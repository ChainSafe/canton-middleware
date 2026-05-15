//go:build ignore

// create-holder-service-request.go — Create a HolderServiceRequest for a standalone party.
// Initiates the onboarding flow with the USDCx registrar/provider so subsequent
// TransferOffer accepts become possible.
//
// Usage:
//   go run scripts/testing/create-holder-service-request.go \
//     -config config.api-server.devnet-test.yaml \
//     -creds ./party-credentials.txt \
//     -operator "auth0_007c65f857f1c3d599cb6df73775::1220d2d732d042c281cee80f483ab80f3cbaa4782860ed5f4dc228ab03dedd2ee8f9" \
//     -provider "Bridge-Operator::12203042ea66f3ee30a05bd6e4241328f2298c6330e3ac5f27d64b8c9fe9c7646f0e" \
//     [-module "Utility.Registry.App.V0.Model.HolderService"]

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
	hsrPackageID = "#utility-registry-app-v0"
	hsrEntity    = "HolderServiceRequest"
)

var (
	configPath = flag.String("config", "config.api-server.devnet-test.yaml", "Config file")
	credsPath  = flag.String("creds", "./party-credentials.txt", "Party credentials file")
	operator   = flag.String("operator", "auth0_007c65f857f1c3d599cb6df73775::1220d2d732d042c281cee80f483ab80f3cbaa4782860ed5f4dc228ab03dedd2ee8f9", "Operator party ID")
	provider   = flag.String("provider", "Bridge-Operator::12203042ea66f3ee30a05bd6e4241328f2298c6330e3ac5f27d64b8c9fe9c7646f0e", "Provider party ID")
	module     = flag.String("module", "Utility.Registry.App.V0.Service.Holder", "Module containing the HolderServiceRequest template")
	dryRun     = flag.Bool("dry-run", false, "Print what would be submitted without sending")
)

func main() {
	flag.Parse()

	creds, err := loadCredentials(*credsPath)
	if err != nil {
		fatalf("load credentials: %v", err)
	}
	holder := creds["party_id"]
	if holder == "" {
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

	// Create the HolderServiceRequest via Interactive Submission
	createCmd := &lapiv2.Command{
		Command: &lapiv2.Command_Create{
			Create: &lapiv2.CreateCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  hsrPackageID,
					ModuleName: *module,
					EntityName: hsrEntity,
				},
				CreateArguments: &lapiv2.Record{
					Fields: []*lapiv2.RecordField{
						{Label: "holder", Value: values.PartyValue(holder)},
						{Label: "operator", Value: values.PartyValue(*operator)},
						{Label: "provider", Value: values.PartyValue(*provider)},
					},
				},
			},
		},
	}

	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println("  Create HolderServiceRequest")
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Printf("  Holder:   %s\n", holder)
	fmt.Printf("  Operator: %s\n", *operator)
	fmt.Printf("  Provider: %s\n", *provider)
	fmt.Printf("  Template: %s:%s:%s\n", hsrPackageID, *module, hsrEntity)
	fmt.Println()

	if *dryRun {
		fmt.Println("[dry-run] would submit Create command via Interactive Submission")
		return
	}

	authCtx := cantonClient.Ledger.AuthContext(ctx)
	commandID := uuid.NewString()
	prepResp, err := cantonClient.Ledger.Interactive().PrepareSubmission(authCtx, &interactivev2.PrepareSubmissionRequest{
		UserId:         cfg.Canton.Token.UserID,
		CommandId:      commandID,
		Commands:       []*lapiv2.Command{createCmd},
		ActAs:          []string{holder},
		SynchronizerId: cfg.Canton.DomainID,
	})
	if err != nil {
		fatalf("PrepareSubmission: %v", err)
	}

	derSig, err := kp.SignDER(prepResp.PreparedTransactionHash)
	if err != nil {
		fatalf("sign hash: %v", err)
	}
	fingerprint, err := kp.Fingerprint()
	if err != nil {
		fatalf("fingerprint: %v", err)
	}

	if _, err := cantonClient.Ledger.Interactive().ExecuteSubmissionAndWait(authCtx, &interactivev2.ExecuteSubmissionAndWaitRequest{
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
		fatalf("ExecuteSubmission: %v", err)
	}

	fmt.Println("✓ HolderServiceRequest created. Waiting for operator to accept...")
	fmt.Println("  (Re-run scripts/testing/list-all-contracts.go to check for the resulting HolderService)")
}

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
