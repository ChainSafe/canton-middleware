//go:build ignore

// send-usdcx-direct.go — Send USDCx from a standalone external party, bypassing
// the middleware's hardcoded 1-hour offer validity so the receiver has effectively
// unlimited time to accept.
//
// Mirrors accept-via-interface.go in structure: loads party credentials, calls
// the registrar's HTTP API to get the transfer factory + choice context +
// disclosed contracts, encodes the AnyValue-shaped choice context inline, then
// exercises Splice.Api.Token.TransferInstructionV1:TransferFactory_Transfer
// via Interactive Submission with a far-future executeBefore.
//
// Usage:
//   go run scripts/testing/send-usdcx-direct.go \
//     -config config.api-server.devnet-test.yaml \
//     -creds ./party-credentials.txt \
//     -to "damlcopilot-receiver::1220096316d4ea75c021d89123cfd2792cfeac80dfbf90bfbca21bcd8bf1bb40d84c" \
//     -amount "2"

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
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/keys"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	// Splice TransferFactory interface — the choice argument shape matches the
	// SDK's encodeTransferFactoryTransferArgs in pkg/cantonsdk/token/encode.go.
	transferFactoryPackageID = "#splice-api-token-transfer-instruction-v1"
	transferFactoryModule    = "Splice.Api.Token.TransferInstructionV1"
	transferFactoryEntity    = "TransferFactory"
	transferChoice           = "TransferFactory_Transfer"
)

var (
	configPath   = flag.String("config", "config.api-server.devnet-test.yaml", "Config file")
	credsPath    = flag.String("creds", "./party-credentials.txt", "Party credentials file")
	toParty      = flag.String("to", "", "Receiver party ID (required)")
	amount       = flag.String("amount", "2", "Amount to send (decimal string)")
	instrumentID = flag.String("instrument", "USDCx", "Instrument ID (token symbol on Canton)")
	registryHost = flag.String("registry-host", "https://api.utilities.digitalasset-dev.com", "Registrar API base URL")
	registrar    = flag.String("registrar", "decentralized-usdc-interchain-rep::1220d420ba8f168d63157f610e6593dca072bbd79ff90a830efc345ed4348a816de7", "Registrar / instrument admin party ID")
	expiry       = flag.String("expiry", "2099-12-31T23:59:59Z", "Offer executeBefore (RFC3339). Default ≈ no time limit")
	dryRun       = flag.Bool("dry-run", false, "Print the registry response + plan, do not submit")
	debug        = flag.Bool("debug", true, "Print registry response JSON")
)

func main() {
	flag.Parse()
	if *toParty == "" {
		fatalf("-to is required")
	}

	creds, err := loadCredentials(*credsPath)
	if err != nil {
		fatalf("load credentials: %v", err)
	}
	sender := creds["party_id"]
	if sender == "" {
		fatalf("party_id missing from credentials")
	}
	privBytes, err := hex.DecodeString(creds["private_key_hex"])
	if err != nil {
		fatalf("decode private key: %v", err)
	}
	kp, err := keys.CantonKeyPairFromPrivateKey(privBytes)
	if err != nil {
		fatalf("reconstruct keypair: %v", err)
	}
	fp, _ := kp.Fingerprint()

	executeBefore, err := time.Parse(time.RFC3339, *expiry)
	if err != nil {
		fatalf("parse -expiry: %v", err)
	}

	fmt.Printf(">>> sender party       : %s\n", sender)
	fmt.Printf(">>> sender fingerprint : %s\n", fp)
	fmt.Printf(">>> receiver party     : %s\n", *toParty)
	fmt.Printf(">>> amount             : %s %s\n", *amount, *instrumentID)
	fmt.Printf(">>> executeBefore      : %s (%s)\n", executeBefore.Format(time.RFC3339), humanizeDuration(time.Until(executeBefore)))

	cfg, err := config.LoadAPIServer(*configPath)
	if err != nil {
		fatalf("load config: %v", err)
	}
	logger, _ := zap.NewDevelopment()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	cantonClient, err := canton.New(ctx, cfg.Canton, canton.WithLogger(logger))
	if err != nil {
		fatalf("canton client: %v", err)
	}
	defer func() { _ = cantonClient.Close() }()

	holdings, err := cantonClient.Token.GetHoldingsByParty(ctx, sender, *instrumentID)
	if err != nil {
		fatalf("get holdings: %v", err)
	}
	fmt.Printf("\n>>> sender %s holdings: %d contract(s)\n", *instrumentID, len(holdings))
	for i, h := range holdings {
		fmt.Printf("    #%d  amount=%s  locked=%v  cid=%s\n", i+1, h.Amount, h.Locked, h.ContractID)
	}

	inputCIDs, total, err := selectHoldings(holdings, *amount)
	if err != nil {
		fatalf("select holdings: %v", err)
	}
	fmt.Printf(">>> selected %d holding(s) totaling %s for amount %s\n", len(inputCIDs), total, *amount)

	httpClient := &http.Client{Timeout: 30 * time.Second}
	regResp, raw, err := fetchTransferFactory(ctx, httpClient, *registryHost, *registrar, sender, *toParty, *amount, *instrumentID, inputCIDs, executeBefore)
	if err != nil {
		fatalf("registry transfer-factory: %v", err)
	}
	if *debug {
		fmt.Printf("\n  --- registry response ---\n%s\n  -------------------------\n", indentJSON(raw))
	}
	fmt.Printf(">>> factoryId          : %s\n", regResp.FactoryID)
	fmt.Printf(">>> transferKind       : %s\n", regResp.TransferKind)
	fmt.Printf(">>> disclosed contracts: %d\n", len(regResp.ChoiceContext.DisclosedContracts))

	ctxValue, err := encodeChoiceContextRecord(regResp.ChoiceContext.ChoiceContextData.Values)
	if err != nil {
		fatalf("encode choice context: %v", err)
	}
	extraArgs := buildExtraArgsValue(ctxValue)

	disclosed, err := buildDisclosedContracts(regResp.ChoiceContext.DisclosedContracts, cfg.Canton.DomainID)
	if err != nil {
		fatalf("build disclosed contracts: %v", err)
	}

	if *dryRun {
		fmt.Println("\n[dry-run] would exercise TransferFactory_Transfer — skipping submission")
		return
	}

	t0 := time.Now()
	offerCID, err := submitTransfer(ctx, cantonClient, cfg, sender, kp, regResp.FactoryID, *toParty, *amount, *instrumentID, inputCIDs, executeBefore, extraArgs, disclosed)
	if err != nil {
		fatalf("submit transfer: %v", err)
	}
	fmt.Printf("\n>>> SUCCESS in %s\n>>> TransferOffer CID: %s\n>>> receiver must exercise TransferInstruction_Accept before %s to settle\n",
		time.Since(t0).Round(time.Millisecond), offerCID, executeBefore.Format(time.RFC3339))
}

// ---------- holdings selection ----------

func selectHoldings(holdings []*token.Holding, amount string) ([]string, string, error) {
	// Greedy: sort by amount descending, take until total >= amount.
	// Skip locked holdings (already committed elsewhere).
	available := make([]*token.Holding, 0, len(holdings))
	for _, h := range holdings {
		if h.Locked {
			continue
		}
		available = append(available, h)
	}
	sort.Slice(available, func(i, j int) bool {
		return parseDecimal(available[i].Amount) > parseDecimal(available[j].Amount)
	})
	target := parseDecimal(amount)
	var total float64
	var cids []string
	for _, h := range available {
		cids = append(cids, h.ContractID)
		total += parseDecimal(h.Amount)
		if total >= target {
			return cids, fmt.Sprintf("%g", total), nil
		}
	}
	return nil, "", fmt.Errorf("insufficient balance: have %g, need %s", total, amount)
}

func parseDecimal(s string) float64 {
	var f float64
	_, _ = fmt.Sscanf(s, "%f", &f)
	return f
}

// ---------- Registry: transfer-factory ----------

type transferFactoryResponse struct {
	FactoryID     string                       `json:"factoryId"`
	TransferKind  string                       `json:"transferKind"`
	ChoiceContext transferFactoryInnerResponse `json:"choiceContext"`
}

type transferFactoryInnerResponse struct {
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

func fetchTransferFactory(
	ctx context.Context, hc *http.Client, host, registrarParty, sender, receiver, amount, instrumentID string,
	inputHoldingCIDs []string, executeBefore time.Time,
) (*transferFactoryResponse, []byte, error) {
	now := time.Now().UTC()
	emptyValues := map[string]any{"values": map[string]any{}}

	body, err := json.Marshal(map[string]any{
		"choiceArguments": map[string]any{
			"expectedAdmin": registrarParty,
			"transfer": map[string]any{
				"sender":   sender,
				"receiver": receiver,
				"amount":   amount,
				"instrumentId": map[string]any{
					"admin": registrarParty,
					"id":    instrumentID,
				},
				"inputHoldingCids": inputHoldingCIDs,
				"meta":             emptyValues,
				"requestedAt":      now.Format("2006-01-02T15:04:05.000Z"),
				"executeBefore":    executeBefore.UTC().Format("2006-01-02T15:04:05.000Z"),
			},
			"extraArgs": map[string]any{
				"context": emptyValues,
				"meta":    emptyValues,
			},
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf(
		"%s/api/token-standard/v0/registrars/%s/registry/transfer-instruction/v1/transfer-factory",
		strings.TrimRight(host, "/"), registrarParty,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := hc.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, raw, fmt.Errorf("registry returned %d: %s", resp.StatusCode, string(raw))
	}

	var out transferFactoryResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, raw, fmt.Errorf("parse registry response: %w", err)
	}
	return &out, raw, nil
}

// ---------- AnyValue encoding (identical to accept-via-interface.go) ----------

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
						Value: &lapiv2.Value{Sum: &lapiv2.Value_TextMap{TextMap: &lapiv2.TextMap{Entries: entries}}},
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

func submitTransfer(
	ctx context.Context, c *canton.Client, cfg *config.APIServer,
	sender string, kp *keys.CantonKeyPair,
	factoryCID, receiver, amount, instrumentID string,
	inputHoldingCIDs []string, executeBefore time.Time,
	extraArgs *lapiv2.Value, disclosed []*lapiv2.DisclosedContract,
) (string, error) {
	// Backdate requestedAt by 5s to dodge ledger-time skew (matches SDK behavior).
	now := time.Now().UTC().Add(-5 * time.Second)

	holdingCidValues := make([]*lapiv2.Value, len(inputHoldingCIDs))
	for i, cid := range inputHoldingCIDs {
		holdingCidValues[i] = values.ContractIDValue(cid)
	}

	transferRec := &lapiv2.Value{
		Sum: &lapiv2.Value_Record{
			Record: &lapiv2.Record{
				Fields: []*lapiv2.RecordField{
					{Label: "sender", Value: values.PartyValue(sender)},
					{Label: "receiver", Value: values.PartyValue(receiver)},
					{Label: "amount", Value: values.NumericValue(amount)},
					{Label: "instrumentId", Value: values.EncodeInstrumentId(*registrar, instrumentID)},
					{Label: "requestedAt", Value: values.TimestampValue(now)},
					{Label: "executeBefore", Value: values.TimestampValue(executeBefore)},
					{Label: "inputHoldingCids", Value: values.ListValue(holdingCidValues)},
					{Label: "meta", Value: values.EmptyMetadata()},
				},
			},
		},
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  transferFactoryPackageID,
					ModuleName: transferFactoryModule,
					EntityName: transferFactoryEntity,
				},
				ContractId: factoryCID,
				Choice:     transferChoice,
				ChoiceArgument: &lapiv2.Value{
					Sum: &lapiv2.Value_Record{
						Record: &lapiv2.Record{
							Fields: []*lapiv2.RecordField{
								{Label: "expectedAdmin", Value: values.PartyValue(*registrar)},
								{Label: "transfer", Value: transferRec},
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
		ActAs:              []string{sender},
		SynchronizerId:     cfg.Canton.DomainID,
		DisclosedContracts: disclosed,
	})
	if err != nil {
		return "", fmt.Errorf("PrepareSubmission: %w", err)
	}
	derSig, err := kp.SignDER(prepResp.PreparedTransactionHash)
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}
	fingerprint, _ := kp.Fingerprint()

	execResp, err := c.Ledger.Interactive().ExecuteSubmissionAndWait(authCtx, &interactivev2.ExecuteSubmissionAndWaitRequest{
		PreparedTransaction: prepResp.PreparedTransaction,
		PartySignatures: &interactivev2.PartySignatures{
			Signatures: []*interactivev2.SinglePartySignatures{
				{
					Party: sender,
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
	})
	if err != nil {
		return "", fmt.Errorf("ExecuteSubmission: %w", err)
	}
	_ = execResp
	// ExecuteSubmissionAndWait doesn't return the created contract id directly;
	// the caller can find the resulting TransferOffer via ACS query as the receiver.
	return "<see ACS for receiver " + receiver + ">", nil
}

// ---------- Helpers ----------

func indentJSON(raw []byte) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "  ", "  "); err != nil {
		return string(raw)
	}
	return buf.String()
}

func humanizeDuration(d time.Duration) string {
	if d <= 0 {
		return "in the past!"
	}
	days := int(d.Hours() / 24)
	if days > 365 {
		return fmt.Sprintf("~%d years from now", days/365)
	}
	if days > 0 {
		return fmt.Sprintf("%d days from now", days)
	}
	return d.Round(time.Second).String() + " from now"
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
