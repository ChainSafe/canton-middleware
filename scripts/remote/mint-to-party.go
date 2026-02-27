//go:build ignore

// mint-to-party.go — Mint DEMO tokens to any Canton party ID on DevNet.
//
// This is a lightweight utility for minting to external parties (e.g. Canton Loop users)
// without requiring database registration.
//
// Usage:
//
//	DATABASE_HOST=localhost go run scripts/remote/mint-to-party.go \
//	  -config config.api-server.devnet.yaml \
//	  -party "PAR::namespace::fingerprint" \
//	  -amount 500

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	expcreds "google.golang.org/grpc/experimental/credentials"
	"google.golang.org/grpc/metadata"
)

var (
	configPath = flag.String("config", "config.api-server.devnet.yaml", "Path to API server config file")
	partyID    = flag.String("party", "", "Canton party ID to mint to (required)")
	amount     = flag.String("amount", "500.0", "Amount to mint")
)

func main() {
	flag.Parse()

	if *partyID == "" {
		fmt.Println("ERROR: -party flag is required")
		flag.Usage()
		os.Exit(1)
	}

	cfg, err := config.LoadAPIServer(*configPath)
	if err != nil {
		fatalf("Failed to load config: %v", err)
	}

	issuer := cfg.Canton.RelayerParty
	if issuer == "" {
		fatalf("canton.relayer_party is required in config")
	}
	domainID := cfg.Canton.DomainID
	if domainID == "" {
		fatalf("canton.domain_id is required in config")
	}
	cip56Pkg := cfg.Canton.CIP56PackageID
	if cip56Pkg == "" {
		fatalf("canton.cip56_package_id is required in config")
	}

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Mint DEMO to Party")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Printf("  Canton:  %s\n", cfg.Canton.RPCURL)
	fmt.Printf("  Issuer:  %s\n", issuer)
	fmt.Printf("  Party:   %s\n", *partyID)
	fmt.Printf("  Amount:  %s DEMO\n", *amount)
	fmt.Println()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Connect to Canton
	conn, err := dialCanton(cfg)
	if err != nil {
		fatalf("Failed to connect to Canton: %v", err)
	}
	defer conn.Close()

	// Get auth context
	ctx, jwtSub, err := authContext(ctx, &cfg.Canton)
	if err != nil {
		fatalf("Failed to get auth token: %v", err)
	}

	stateService := lapiv2.NewStateServiceClient(conn)
	commandService := lapiv2.NewCommandServiceClient(conn)

	// Step 1: Find existing TokenConfig (DEMO)
	fmt.Println(">>> Finding TokenConfig (DEMO)...")
	tokenConfigCID, err := findTokenConfig(ctx, stateService, issuer, cip56Pkg)
	if err != nil {
		fatalf("Failed to find TokenConfig for DEMO: %v", err)
	}
	fmt.Printf("    TokenConfig: %s\n", tokenConfigCID)
	fmt.Println()

	// Step 2: Mint
	fmt.Println(">>> Minting DEMO tokens...")
	holdingCID, err := mintTokens(ctx, commandService, cip56Pkg, tokenConfigCID, *partyID, *amount, issuer, domainID, jwtSub)
	if err != nil {
		fatalf("Failed to mint: %v", err)
	}

	fmt.Println()
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Mint Complete")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Printf("  Holding CID: %s\n", holdingCID)
	fmt.Printf("  Owner:       %s\n", *partyID)
	fmt.Printf("  Amount:      %s DEMO\n", *amount)
	fmt.Println()
}

func dialCanton(cfg *config.APIServerConfig) (*grpc.ClientConn, error) {
	var opts []grpc.DialOption
	if cfg.Canton.TLS.Enabled {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // devnet testing
		}
		opts = append(opts, grpc.WithTransportCredentials(expcreds.NewTLSWithALPNDisabled(tlsConfig)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	if cfg.Canton.MaxMessageSize > 0 {
		opts = append(opts, grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(cfg.Canton.MaxMessageSize)))
	}

	target := cfg.Canton.RPCURL
	if !strings.Contains(target, "://") {
		target = "dns:///" + target
	}
	return grpc.NewClient(target, opts...)
}

func authContext(ctx context.Context, canton *config.CantonConfig) (context.Context, string, error) {
	if canton.Auth.ClientID == "" {
		return ctx, "", nil
	}

	payload := map[string]string{
		"client_id":     canton.Auth.ClientID,
		"client_secret": canton.Auth.ClientSecret,
		"audience":      canton.Auth.Audience,
		"grant_type":    "client_credentials",
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(canton.Auth.TokenURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(respBody))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return nil, "", fmt.Errorf("parse token response: %w", err)
	}

	// Extract JWT subject
	var sub string
	parts := strings.Split(tokenResp.AccessToken, ".")
	if len(parts) >= 2 {
		padded := parts[1]
		switch len(padded) % 4 {
		case 2:
			padded += "=="
		case 3:
			padded += "="
		}
		if decoded, err := base64.URLEncoding.DecodeString(padded); err == nil {
			var claims struct {
				Sub string `json:"sub"`
			}
			json.Unmarshal(decoded, &claims)
			sub = claims.Sub
		}
	}

	md := metadata.Pairs("authorization", "Bearer "+tokenResp.AccessToken)
	return metadata.NewOutgoingContext(ctx, md), sub, nil
}

func findTokenConfig(ctx context.Context, client lapiv2.StateServiceClient, party, packageID string) (string, error) {
	endResp, err := client.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		return "", err
	}
	if endResp.Offset == 0 {
		return "", fmt.Errorf("ledger is empty")
	}

	stream, err := client.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: endResp.Offset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				party: {
					Cumulative: []*lapiv2.CumulativeFilter{
						{
							IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
								TemplateFilter: &lapiv2.TemplateFilter{
									TemplateId: &lapiv2.Identifier{
										PackageId:  packageID,
										ModuleName: "CIP56.Config",
										EntityName: "TokenConfig",
									},
								},
							},
						},
					},
				},
			},
			Verbose: true,
		},
	})
	if err != nil {
		return "", err
	}

	for {
		msg, err := stream.Recv()
		if err != nil {
			break
		}
		if ac := msg.GetActiveContract(); ac != nil {
			if values.MetaSymbolFromRecord(ac.CreatedEvent.GetCreateArguments()) == "DEMO" {
				return ac.CreatedEvent.ContractId, nil
			}
		}
	}
	return "", fmt.Errorf("no TokenConfig found for DEMO")
}

func mintTokens(ctx context.Context, client lapiv2.CommandServiceClient, cip56Pkg, configCID, recipient, amt, issuer, domainID, jwtSub string) (string, error) {
	cmdID := fmt.Sprintf("mint-to-party-%d", time.Now().UnixNano())

	mintArgs := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "recipient", Value: values.PartyValue(recipient)},
			{Label: "amount", Value: &lapiv2.Value{Sum: &lapiv2.Value_Numeric{Numeric: amt}}},
			{Label: "eventTime", Value: &lapiv2.Value{Sum: &lapiv2.Value_Timestamp{Timestamp: time.Now().UnixMicro()}}},
			{Label: "userFingerprint", Value: values.TextValue("external-party")},
			{Label: "evmTxHash", Value: &lapiv2.Value{Sum: &lapiv2.Value_Optional{Optional: &lapiv2.Optional{}}}},
		},
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  cip56Pkg,
					ModuleName: "CIP56.Config",
					EntityName: "TokenConfig",
				},
				ContractId:     configCID,
				Choice:         "IssuerMint",
				ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: mintArgs}},
			},
		},
	}

	resp, err := client.SubmitAndWaitForTransaction(ctx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: domainID,
			CommandId:      cmdID,
			UserId:         jwtSub,
			ActAs:          []string{issuer},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return "", fmt.Errorf("submit failed: %w", err)
	}

	if resp.Transaction != nil {
		for _, event := range resp.Transaction.Events {
			if created := event.GetCreated(); created != nil {
				if created.TemplateId.EntityName == "CIP56Holding" {
					return created.ContractId, nil
				}
			}
		}
	}
	return "", fmt.Errorf("CIP56Holding not found in response")
}

func fatalf(format string, args ...interface{}) {
	fmt.Printf("ERROR: "+format+"\n", args...)
	os.Exit(1)
}
