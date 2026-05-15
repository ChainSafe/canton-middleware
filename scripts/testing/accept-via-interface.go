//go:build ignore

// accept-via-interface.go — Accept a USDCx TransferOffer by exercising the
// Splice CIP-56 TransferInstruction interface choice TransferInstruction_Accept.
//
// This script:
//  1. Loads standalone external party credentials.
//  2. Finds pending TransferOffers visible to that party.
//  3. For each offer, calls the USDCx registrar's per-instruction choice-context
//     endpoint to get the choiceContextData (TransferRule + InstrumentConfig
//     contract IDs etc.) and the disclosedContracts needed for visibility.
//  4. Encodes the choiceContext as Daml `Map Text AnyValue` and the
//     disclosedContracts as proto, then exercises TransferInstruction_Accept
//     via the Interactive Submission API.
//
// Usage:
//   go run scripts/testing/accept-via-interface.go \
//     -config config.api-server.devnet-test.yaml \
//     -creds ./party-credentials.txt

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/client"
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	interactivev2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2/interactive"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/keys"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	// Concrete template (for ACS lookup of pending offers)
	transferOfferPackageID = "#utility-registry-app-v0"
	transferOfferModule    = "Utility.Registry.App.V0.Model.Transfer"
	transferOfferEntity    = "TransferOffer"

	// Splice TransferInstruction interface (for the Accept choice)
	transferInstrPackageID = "#splice-api-token-transfer-instruction-v1"
	transferInstrModule    = "Splice.Api.Token.TransferInstructionV1"
	transferInstrEntity    = "TransferInstruction"
	acceptChoice           = "TransferInstruction_Accept"
)

var (
	configPath   = flag.String("config", "config.api-server.devnet-test.yaml", "Config")
	credsPath    = flag.String("creds", "./party-credentials.txt", "Credentials file")
	registryHost = flag.String("registry-host", "https://api.utilities.digitalasset-dev.com", "Registrar API base URL")
	registrar    = flag.String("registrar", "decentralized-usdc-interchain-rep::1220d420ba8f168d63157f610e6593dca072bbd79ff90a830efc345ed4348a816de7", "Registrar party ID")
	dryRun       = flag.Bool("dry-run", false, "Print the registry response and command without submitting")
	debug        = flag.Bool("debug", true, "Print registry response JSON")
)

func main() {
	flag.Parse()

	creds, err := loadCredentials(*credsPath)
	if err != nil {
		fatalf("load credentials: %v", err)
	}
	holder := creds["party_id"]
	privBytes, err := hex.DecodeString(creds["private_key_hex"])
	if err != nil {
		fatalf("decode private key: %v", err)
	}
	kp, err := keys.CantonKeyPairFromPrivateKey(privBytes)
	if err != nil {
		fatalf("reconstruct keypair: %v", err)
	}

	cfg, err := config.LoadAPIServer(*configPath)
	if err != nil {
		fatalf("load config: %v", err)
	}
	logger, _ := zap.NewDevelopment()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cantonClient, err := canton.New(ctx, cfg.Canton, canton.WithLogger(logger))
	if err != nil {
		fatalf("canton client: %v", err)
	}
	defer func() { _ = cantonClient.Close() }()

	offers, err := findOffers(ctx, cantonClient, holder)
	if err != nil {
		fatalf("find offers: %v", err)
	}
	fmt.Printf(">>> Found %d TransferOffer(s) for %s\n", len(offers), holder)
	if len(offers) == 0 {
		return
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}

	for i, cid := range offers {
		fmt.Printf("\n--- Offer #%d ---\n  CID: %s\n", i+1, cid)

		regResp, raw, err := fetchAcceptContext(ctx, httpClient, *registryHost, *registrar, cid)
		if err != nil {
			fmt.Printf("  ERROR fetching choice-context: %v\n", err)
			continue
		}
		if *debug {
			fmt.Printf("  --- registry response ---\n%s\n  -------------------------\n", indentJSON(raw))
		}

		ctxValue, err := encodeChoiceContextRecord(regResp.ChoiceContextData.Values)
		if err != nil {
			fmt.Printf("  ERROR encoding choice context: %v\n", err)
			continue
		}
		extraArgs := buildExtraArgsValue(ctxValue)

		disclosed, err := buildDisclosedContracts(regResp.DisclosedContracts, cfg.Canton.DomainID)
		if err != nil {
			fmt.Printf("  ERROR building disclosed contracts: %v\n", err)
			continue
		}
		fmt.Printf("  Disclosed contracts: %d\n", len(disclosed))

		if *dryRun {
			fmt.Println("  [dry-run] would exercise TransferInstruction_Accept")
			continue
		}

		if err := acceptViaInterface(ctx, cantonClient, cfg, holder, kp, cid, extraArgs, disclosed); err != nil {
			fmt.Printf("  ERROR: %v\n", err)
			continue
		}
		fmt.Println("  ACCEPTED")
	}
}

func findOffers(ctx context.Context, c *canton.Client, partyID string) ([]string, error) {
	end, err := c.Ledger.GetLedgerEnd(ctx)
	if err != nil {
		return nil, err
	}
	if end == 0 {
		return nil, nil
	}
	tid := &lapiv2.Identifier{
		PackageId: transferOfferPackageID, ModuleName: transferOfferModule, EntityName: transferOfferEntity,
	}
	events, err := c.Ledger.GetActiveContractsByTemplate(ctx, end, []string{partyID}, tid)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, ce := range events {
		out = append(out, ce.ContractId)
	}
	return out, nil
}

// ---------- Registry ----------

type acceptContextResponse struct {
	ChoiceContextData struct {
		Values map[string]json.RawMessage `json:"values"`
	} `json:"choiceContextData"`
	DisclosedContracts []registryDisclosedContract `json:"disclosedContracts"`
}

type registryDisclosedContract struct {
	ContractID       string          `json:"contractId"`
	CreatedEventBlob string          `json:"createdEventBlob"`
	TemplateID       json.RawMessage `json:"templateId"`
	SynchronizerID   string          `json:"synchronizerId"`
}

func fetchAcceptContext(ctx context.Context, hc *http.Client, host, registrarParty, instructionCID string) (*acceptContextResponse, []byte, error) {
	url := fmt.Sprintf(
		"%s/api/token-standard/v0/registrars/%s/registry/transfer-instruction/v1/%s/choice-contexts/accept",
		strings.TrimRight(host, "/"), registrarParty, instructionCID,
	)
	body := []byte(`{"meta":{},"excludeDebugFields":false}`)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := hc.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("registry POST %s: %w", url, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, raw, fmt.Errorf("registry returned %d: %s", resp.StatusCode, string(raw))
	}

	var out acceptContextResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, raw, fmt.Errorf("parse registry response: %w", err)
	}
	return &out, raw, nil
}

// ---------- AnyValue encoding ----------

// anyValueJSON is the {"tag": "...", "value": ...} envelope used by Daml-LF JSON
// for the AnyValue ADT.
type anyValueJSON struct {
	Tag   string          `json:"tag"`
	Value json.RawMessage `json:"value"`
}

func encodeAnyValue(raw json.RawMessage) (*lapiv2.Value, error) {
	var av anyValueJSON
	if err := json.Unmarshal(raw, &av); err != nil {
		return nil, fmt.Errorf("parse AnyValue: %w", err)
	}
	var inner *lapiv2.Value
	switch av.Tag {
	case "AV_ContractId":
		var s string
		if err := json.Unmarshal(av.Value, &s); err != nil {
			return nil, err
		}
		inner = values.ContractIDValue(s)
	case "AV_Text":
		var s string
		if err := json.Unmarshal(av.Value, &s); err != nil {
			return nil, err
		}
		inner = values.TextValue(s)
	case "AV_Party":
		var s string
		if err := json.Unmarshal(av.Value, &s); err != nil {
			return nil, err
		}
		inner = values.PartyValue(s)
	case "AV_Bool":
		var b bool
		if err := json.Unmarshal(av.Value, &b); err != nil {
			return nil, err
		}
		inner = &lapiv2.Value{Sum: &lapiv2.Value_Bool{Bool: b}}
	case "AV_Int":
		var n json.Number
		if err := json.Unmarshal(av.Value, &n); err != nil {
			return nil, err
		}
		i, err := n.Int64()
		if err != nil {
			return nil, fmt.Errorf("AV_Int: %w", err)
		}
		inner = &lapiv2.Value{Sum: &lapiv2.Value_Int64{Int64: i}}
	case "AV_Decimal":
		var s string
		if err := json.Unmarshal(av.Value, &s); err != nil {
			return nil, err
		}
		inner = values.NumericValue(s)
	case "AV_List":
		var items []json.RawMessage
		if err := json.Unmarshal(av.Value, &items); err != nil {
			return nil, err
		}
		elems := make([]*lapiv2.Value, 0, len(items))
		for _, it := range items {
			v, err := encodeAnyValue(it)
			if err != nil {
				return nil, err
			}
			elems = append(elems, v)
		}
		inner = values.ListValue(elems)
	case "AV_Map":
		// Map is encoded as a list of (Text, AnyValue) tuples
		var items []struct {
			Key   string          `json:"_1"`
			Value json.RawMessage `json:"_2"`
		}
		if err := json.Unmarshal(av.Value, &items); err != nil {
			return nil, err
		}
		elems := make([]*lapiv2.Value, 0, len(items))
		for _, it := range items {
			v, err := encodeAnyValue(it.Value)
			if err != nil {
				return nil, err
			}
			elems = append(elems, &lapiv2.Value{
				Sum: &lapiv2.Value_Record{
					Record: &lapiv2.Record{
						Fields: []*lapiv2.RecordField{
							{Label: "_1", Value: values.TextValue(it.Key)},
							{Label: "_2", Value: v},
						},
					},
				},
			})
		}
		inner = values.ListValue(elems)
	default:
		return nil, fmt.Errorf("unsupported AnyValue tag: %s (raw=%s)", av.Tag, string(raw))
	}
	return &lapiv2.Value{
		Sum: &lapiv2.Value_Variant{
			Variant: &lapiv2.Variant{
				Constructor: av.Tag,
				Value:       inner,
			},
		},
	}, nil
}

// encodeChoiceContextRecord builds the Splice ChoiceContext record:
//
//	ChoiceContext { values: TextMap AnyValue }
func encodeChoiceContextRecord(vals map[string]json.RawMessage) (*lapiv2.Value, error) {
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	entries := make([]*lapiv2.TextMap_Entry, 0, len(vals))
	for _, k := range keys {
		v, err := encodeAnyValue(vals[k])
		if err != nil {
			return nil, fmt.Errorf("encode key %q: %w", k, err)
		}
		entries = append(entries, &lapiv2.TextMap_Entry{Key: k, Value: v})
	}
	return &lapiv2.Value{
		Sum: &lapiv2.Value_Record{
			Record: &lapiv2.Record{
				Fields: []*lapiv2.RecordField{
					{
						Label: "values",
						Value: &lapiv2.Value{
							Sum: &lapiv2.Value_TextMap{
								TextMap: &lapiv2.TextMap{Entries: entries},
							},
						},
					},
				},
			},
		},
	}, nil
}

func buildExtraArgsValue(ctxValue *lapiv2.Value) *lapiv2.Value {
	return &lapiv2.Value{
		Sum: &lapiv2.Value_Record{
			Record: &lapiv2.Record{
				Fields: []*lapiv2.RecordField{
					{Label: "context", Value: ctxValue},
					{Label: "meta", Value: values.EmptyMetadata()},
				},
			},
		},
	}
}

// ---------- DisclosedContracts ----------

// templateID may be a string "<pkg>:Module:Entity" or a {packageId, moduleName, entityName} object.
func parseTemplateID(raw json.RawMessage) (*lapiv2.Identifier, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && s != "" {
		parts := strings.SplitN(s, ":", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("templateId %q not in pkg:module:entity form", s)
		}
		return &lapiv2.Identifier{PackageId: parts[0], ModuleName: parts[1], EntityName: parts[2]}, nil
	}
	var obj struct {
		PackageID  string `json:"packageId"`
		ModuleName string `json:"moduleName"`
		EntityName string `json:"entityName"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("parse templateId: %w", err)
	}
	return &lapiv2.Identifier{PackageId: obj.PackageID, ModuleName: obj.ModuleName, EntityName: obj.EntityName}, nil
}

func buildDisclosedContracts(rcs []registryDisclosedContract, fallbackDomain string) ([]*lapiv2.DisclosedContract, error) {
	out := make([]*lapiv2.DisclosedContract, 0, len(rcs))
	for _, c := range rcs {
		blob, err := base64.StdEncoding.DecodeString(c.CreatedEventBlob)
		if err != nil {
			return nil, fmt.Errorf("decode blob for %s: %w", c.ContractID, err)
		}
		tid, err := parseTemplateID(c.TemplateID)
		if err != nil {
			return nil, err
		}
		domain := c.SynchronizerID
		if domain == "" {
			domain = fallbackDomain
		}
		out = append(out, &lapiv2.DisclosedContract{
			TemplateId:       tid,
			ContractId:       c.ContractID,
			CreatedEventBlob: blob,
			SynchronizerId:   domain,
		})
	}
	return out, nil
}

// ---------- Submit ----------

func acceptViaInterface(
	ctx context.Context,
	c *canton.Client,
	cfg *config.APIServer,
	holder string,
	kp *keys.CantonKeyPair,
	contractID string,
	extraArgs *lapiv2.Value,
	disclosed []*lapiv2.DisclosedContract,
) error {
	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  transferInstrPackageID,
					ModuleName: transferInstrModule,
					EntityName: transferInstrEntity,
				},
				ContractId: contractID,
				Choice:     acceptChoice,
				ChoiceArgument: &lapiv2.Value{
					Sum: &lapiv2.Value_Record{
						Record: &lapiv2.Record{
							Fields: []*lapiv2.RecordField{
								{Label: "extraArgs", Value: extraArgs},
							},
						},
					},
				},
			},
		},
	}

	authCtx := c.Ledger.AuthContext(ctx)
	prepResp, err := c.Ledger.Interactive().PrepareSubmission(authCtx, &interactivev2.PrepareSubmissionRequest{
		UserId:             cfg.Canton.Token.UserID,
		CommandId:          uuid.NewString(),
		Commands:           []*lapiv2.Command{cmd},
		ActAs:              []string{holder},
		SynchronizerId:     cfg.Canton.DomainID,
		DisclosedContracts: disclosed,
	})
	if err != nil {
		return fmt.Errorf("PrepareSubmission: %w", err)
	}

	derSig, err := kp.SignDER(prepResp.PreparedTransactionHash)
	if err != nil {
		return fmt.Errorf("sign: %w", err)
	}
	fingerprint, _ := kp.Fingerprint()

	if _, err := c.Ledger.Interactive().ExecuteSubmissionAndWait(authCtx, &interactivev2.ExecuteSubmissionAndWaitRequest{
		PreparedTransaction: prepResp.PreparedTransaction,
		PartySignatures: &interactivev2.PartySignatures{
			Signatures: []*interactivev2.SinglePartySignatures{
				{
					Party: holder,
					Signatures: []*lapiv2.Signature{
						{
							Format:               lapiv2.SignatureFormat_SIGNATURE_FORMAT_DER,
							Signature:            derSig,
							SignedBy:             fingerprint,
							SigningAlgorithmSpec: lapiv2.SigningAlgorithmSpec_SIGNING_ALGORITHM_SPEC_EC_DSA_SHA_256,
						},
					},
				},
			},
		},
		SubmissionId:         uuid.NewString(),
		UserId:               cfg.Canton.Token.UserID,
		HashingSchemeVersion: prepResp.HashingSchemeVersion,
	}); err != nil {
		return fmt.Errorf("ExecuteSubmission: %w", err)
	}
	return nil
}

// ---------- Helpers ----------

func indentJSON(raw []byte) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "  ", "  "); err != nil {
		return string(raw)
	}
	return buf.String()
}

func loadCredentials(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		out[strings.TrimSpace(line[:idx])] = strings.TrimSpace(line[idx+1:])
	}
	return out, s.Err()
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FATAL: "+format+"\n", args...)
	os.Exit(1)
}
