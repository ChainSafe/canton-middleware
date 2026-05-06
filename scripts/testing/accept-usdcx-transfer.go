//go:build ignore

// accept-usdcx-transfer.go — Accept pending USDCx TransferOffers for a standalone party.
//
// Reads party credentials from a text file produced by allocate-standalone-party.go,
// finds pending TransferOffer contracts where this party is the receiver, and exercises
// TransferOffer_Accept on each via Canton's Interactive Submission API.
//
// Usage:
//   go run scripts/testing/accept-usdcx-transfer.go \
//     -config config.api-server.devnet-test.yaml \
//     -creds ./party-credentials.txt \
//     [-dry-run]

package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
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
	transferOfferPackageID = "#utility-registry-app-v0"
	transferOfferModule    = "Utility.Registry.App.V0.Model.Transfer"
	transferOfferEntity    = "TransferOffer"
	transferOfferAccept    = "TransferOffer_Accept"
)

var (
	configPath = flag.String("config", "config.api-server.devnet-test.yaml", "Config file")
	credsPath  = flag.String("creds", "./party-credentials.txt", "Party credentials file")
	dryRun     = flag.Bool("dry-run", false, "Print what would be exercised without submitting")
)

func main() {
	flag.Parse()

	creds, err := loadCredentials(*credsPath)
	if err != nil {
		fatalf("load credentials: %v", err)
	}
	fmt.Printf(">>> Loaded credentials for party: %s\n", creds["party_id"])

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

	partyID := creds["party_id"]
	if partyID == "" {
		fatalf("party_id missing from credentials")
	}

	// Reconstruct keypair from hex-encoded private key
	privBytes, err := hex.DecodeString(creds["private_key_hex"])
	if err != nil {
		fatalf("decode private key: %v", err)
	}
	kp, err := keys.CantonKeyPairFromPrivateKey(privBytes)
	if err != nil {
		fatalf("reconstruct keypair: %v", err)
	}

	// Find pending TransferOffer contracts where this party is the receiver
	offers, err := findTransferOffers(ctx, cantonClient, partyID)
	if err != nil {
		fatalf("find offers: %v", err)
	}
	fmt.Printf(">>> Found %d TransferOffer contract(s) for this party\n", len(offers))

	if len(offers) == 0 {
		fmt.Println("No offers to accept. Exiting.")
		return
	}

	for i, o := range offers {
		fmt.Printf("\n--- Offer #%d ---\n", i+1)
		fmt.Printf("  CID:    %s\n", o.contractID)
		fmt.Printf("  Sender: %s\n", o.sender)
		fmt.Printf("  Amount: %s %s\n", o.amount, o.instrumentID)

		if *dryRun {
			fmt.Println("  [dry-run] would exercise TransferOffer_Accept")
			continue
		}

		if err := exerciseAccept(ctx, cantonClient, cfg, partyID, kp, o.contractID); err != nil {
			fmt.Printf("  ERROR accepting: %v\n", err)
			continue
		}
		fmt.Println("  ACCEPTED ✓")
	}
}

type offerInfo struct {
	contractID   string
	sender       string
	amount       string
	instrumentID string
}

// findTransferOffers queries the ACS for all TransferOffer contracts visible to partyID.
func findTransferOffers(ctx context.Context, c *canton.Client, partyID string) ([]offerInfo, error) {
	end, err := c.Ledger.GetLedgerEnd(ctx)
	if err != nil {
		return nil, err
	}
	if end == 0 {
		return nil, nil
	}

	tid := &lapiv2.Identifier{
		PackageId:  transferOfferPackageID,
		ModuleName: transferOfferModule,
		EntityName: transferOfferEntity,
	}
	events, err := c.Ledger.GetActiveContractsByTemplate(ctx, end, []string{partyID}, tid)
	if err != nil {
		return nil, err
	}

	var out []offerInfo
	for _, ce := range events {
		fields := values.RecordToMap(ce.CreateArguments)
		transferRec, ok := fields["transfer"].Sum.(*lapiv2.Value_Record)
		if !ok || transferRec.Record == nil {
			continue
		}
		tFields := values.RecordToMap(transferRec.Record)
		_, instrumentID := values.DecodeInstrumentId(tFields["instrumentId"])
		out = append(out, offerInfo{
			contractID:   ce.ContractId,
			sender:       values.Party(tFields["sender"]),
			amount:       values.Numeric(tFields["amount"]),
			instrumentID: instrumentID,
		})
	}
	return out, nil
}

// exerciseAccept submits an Interactive Submission to exercise TransferOffer_Accept.
func exerciseAccept(
	ctx context.Context,
	c *canton.Client,
	cfg *config.APIServer,
	partyID string,
	kp *keys.CantonKeyPair,
	contractID string,
) error {
	// Build the exercise command. Choice arg is empty for now (most likely);
	// if Canton rejects, the error will tell us the expected schema.
	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  transferOfferPackageID,
					ModuleName: transferOfferModule,
					EntityName: transferOfferEntity,
				},
				ContractId: contractID,
				Choice:     transferOfferAccept,
				ChoiceArgument: &lapiv2.Value{
					Sum: &lapiv2.Value_Record{
						Record: &lapiv2.Record{Fields: nil}, // empty record
					},
				},
			},
		},
	}

	authCtx := c.Ledger.AuthContext(ctx)
	commandID := uuid.NewString()

	// PrepareSubmission as the external party. ActAs the user, no ReadAs.
	prepResp, err := c.Ledger.Interactive().PrepareSubmission(authCtx, &interactivev2.PrepareSubmissionRequest{
		UserId:         cfg.Canton.Token.UserID,
		CommandId:      commandID,
		Commands:       []*lapiv2.Command{cmd},
		ActAs:          []string{partyID},
		SynchronizerId: cfg.Canton.DomainID,
	})
	if err != nil {
		return fmt.Errorf("PrepareSubmission: %w", err)
	}

	// Sign the prepared transaction hash with the party's key
	derSig, err := kp.SignHashDER(prepResp.PreparedTransactionHash)
	if err != nil {
		return fmt.Errorf("sign hash: %w", err)
	}
	fingerprint, err := kp.Fingerprint()
	if err != nil {
		return fmt.Errorf("fingerprint: %w", err)
	}

	partySigs := &interactivev2.PartySignatures{
		Signatures: []*interactivev2.SinglePartySignatures{
			{
				Party: partyID,
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
	}

	if _, err := c.Ledger.Interactive().ExecuteSubmissionAndWait(authCtx, &interactivev2.ExecuteSubmissionAndWaitRequest{
		PreparedTransaction:  prepResp.PreparedTransaction,
		PartySignatures:      partySigs,
		SubmissionId:         uuid.NewString(),
		UserId:               cfg.Canton.Token.UserID,
		HashingSchemeVersion: prepResp.HashingSchemeVersion,
	}); err != nil {
		return fmt.Errorf("ExecuteSubmission: %w", err)
	}

	return nil
}

// loadCredentials reads the simple key=value text file produced by allocate-standalone-party.go.
func loadCredentials(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		k := strings.TrimSpace(line[:idx])
		v := strings.TrimSpace(line[idx+1:])
		out[k] = v
	}
	return out, scanner.Err()
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FATAL: "+format+"\n", args...)
	os.Exit(1)
}
