//go:build ignore

// register-user.go - Register a user's fingerprint mapping on Canton
//
// Usage:
//   go run scripts/register-user.go -config config.yaml \
//     -party "Alice::1220abc...def" \
//     -fingerprint "abc...def" \
//     -evm-address "0x..."
//
// For testing with the BridgeIssuer:
//   go run scripts/register-user.go -config config.yaml \
//     -party "BridgeIssuer::122047584945db4991c2954b1e8e673623a43ec80869abf0f8e7531a435ae797ac6e" \
//     -fingerprint "47584945db4991c2954b1e8e673623a43ec80869abf0f8e7531a435ae797ac6e"

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"strings"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

var (
	configPath  = flag.String("config", "config.yaml", "Path to config file")
	partyID     = flag.String("party", "", "Full Canton Party ID (e.g., 'Alice::1220abc...')")
	fingerprint = flag.String("fingerprint", "", "Fingerprint (32-byte hex, without 0x1220 prefix)")
	evmAddress  = flag.String("evm-address", "", "Optional EVM address for withdrawals")
)

func main() {
	flag.Parse()

	if *partyID == "" {
		fmt.Println("Error: -party is required")
		fmt.Println("Usage: go run scripts/register-user.go -config config.yaml -party 'PartyID' -fingerprint 'hex'")
		os.Exit(1)
	}

	// If fingerprint not provided, extract from party ID
	if *fingerprint == "" {
		// Extract fingerprint from party ID (format: "hint::fingerprint")
		parts := strings.Split(*partyID, "::")
		if len(parts) == 2 {
			fp := parts[1]
			// Remove "1220" multihash prefix if present (for bytes32 compatibility)
			if strings.HasPrefix(fp, "1220") && len(fp) == 68 {
				*fingerprint = fp[4:]
			} else {
				*fingerprint = fp
			}
			fmt.Printf("Extracted fingerprint from party ID: %s\n", *fingerprint)
		} else {
			fmt.Println("Error: Could not extract fingerprint from party ID. Please provide -fingerprint")
			os.Exit(1)
		}
	}

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("======================================================================")
	fmt.Println("REGISTER USER - Create FingerprintMapping on Canton")
	fmt.Println("======================================================================")
	fmt.Printf("Party:       %s\n", *partyID)
	fmt.Printf("Fingerprint: %s\n", *fingerprint)
	fmt.Printf("EVM Address: %s\n", *evmAddress)
	fmt.Println()

	ctx := context.Background()

	// Connect to Canton with TLS if enabled
	var opts []grpc.DialOption
	if cfg.Canton.TLS.Enabled {
		tlsConfig := &tls.Config{} // Uses system CA pool
		creds := credentials.NewTLS(tlsConfig)
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(cfg.Canton.RPCURL, opts...)
	if err != nil {
		fmt.Printf("Failed to connect to Canton: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Load JWT token if configured
	if cfg.Canton.Auth.TokenFile != "" {
		tokenBytes, err := os.ReadFile(cfg.Canton.Auth.TokenFile)
		if err != nil {
			fmt.Printf("Failed to read token file: %v\n", err)
			os.Exit(1)
		}
		authToken := strings.TrimSpace(string(tokenBytes))
		md := metadata.Pairs("authorization", "Bearer "+authToken)
		ctx = metadata.NewOutgoingContext(ctx, md)
	}

	stateClient := lapiv2.NewStateServiceClient(conn)
	cmdClient := lapiv2.NewCommandServiceClient(conn)

	// Step 1: Get ledger end offset (required for V2 API)
	ledgerEndResp, err := stateClient.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		fmt.Printf("Failed to get ledger end: %v\n", err)
		os.Exit(1)
	}
	if ledgerEndResp.Offset == 0 {
		fmt.Println("Error: Ledger is empty. Run bootstrap first.")
		os.Exit(1)
	}

	// Step 2: Find WayfinderBridgeConfig
	fmt.Println(">>> Finding WayfinderBridgeConfig...")
	configCid, err := findBridgeConfig(ctx, stateClient, cfg.Canton.RelayerParty, cfg.Canton.BridgePackageID, ledgerEndResp.Offset)
	if err != nil {
		fmt.Printf("Failed to find WayfinderBridgeConfig: %v\n", err)
		fmt.Println("Run: go run scripts/bootstrap-bridge.go first")
		os.Exit(1)
	}
	fmt.Printf("    Config CID: %s\n\n", configCid)

	// Step 3: Check if FingerprintMapping already exists
	fmt.Println(">>> Checking for existing FingerprintMapping...")
	existingCid, err := findFingerprintMapping(ctx, stateClient, cfg.Canton.RelayerParty, cfg.Canton.BridgePackageID, ledgerEndResp.Offset, *fingerprint)
	if err == nil && existingCid != "" {
		fmt.Printf("    [EXISTS] FingerprintMapping already exists: %s\n", existingCid)
		fmt.Println("\n✓ User is already registered!")
		os.Exit(0)
	}

	// Step 4: Get domain ID
	fmt.Println(">>> Getting domain ID...")
	domainResp, err := stateClient.GetConnectedSynchronizers(ctx, &lapiv2.GetConnectedSynchronizersRequest{
		Party: cfg.Canton.RelayerParty,
	})
	if err != nil {
		fmt.Printf("Failed to get domain ID: %v\n", err)
		os.Exit(1)
	}
	if len(domainResp.ConnectedSynchronizers) == 0 {
		fmt.Println("Error: No connected synchronizers")
		os.Exit(1)
	}
	domainID := domainResp.ConnectedSynchronizers[0].SynchronizerId
	fmt.Printf("    Domain ID: %s\n\n", domainID)

	// Step 5: Register user
	fmt.Println(">>> Registering user...")
	mappingCid, err := registerUser(ctx, cmdClient, cfg.Canton.RelayerParty, cfg.Canton.BridgePackageID, domainID, configCid, *partyID, *fingerprint, *evmAddress)
	if err != nil {
		fmt.Printf("Failed to register user: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("    FingerprintMapping CID: %s\n", mappingCid)

	fmt.Println()
	fmt.Println("======================================================================")
	fmt.Println("USER REGISTERED SUCCESSFULLY")
	fmt.Println("======================================================================")
	fmt.Printf("Party:            %s\n", *partyID)
	fmt.Printf("Fingerprint:      %s\n", *fingerprint)
	fmt.Printf("MappingCid:       %s\n", mappingCid)
	fmt.Println()
	fmt.Println("The user can now receive deposits with this fingerprint as bytes32:")
	fmt.Printf("  CANTON_RECIPIENT=\"0x%s\"\n", *fingerprint)
}

func findBridgeConfig(ctx context.Context, client lapiv2.StateServiceClient, party, packageID string, offset int64) (string, error) {
	resp, err := client.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: offset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				party: {
					Cumulative: []*lapiv2.CumulativeFilter{
						{
							IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
								TemplateFilter: &lapiv2.TemplateFilter{
									TemplateId: &lapiv2.Identifier{
										PackageId:  packageID,
										ModuleName: "Wayfinder.Bridge",
										EntityName: "WayfinderBridgeConfig",
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
		msg, err := resp.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			return contract.CreatedEvent.ContractId, nil
		}
	}
	return "", fmt.Errorf("no WayfinderBridgeConfig found")
}

func findFingerprintMapping(ctx context.Context, client lapiv2.StateServiceClient, party, packageID string, offset int64, targetFingerprint string) (string, error) {
	// Note: In the Common package, not bridge-wayfinder
	// Try both package IDs since FingerprintMapping is in common package
	resp, err := client.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: offset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				party: {
					Cumulative: []*lapiv2.CumulativeFilter{
						{
							IdentifierFilter: &lapiv2.CumulativeFilter_WildcardFilter{
								WildcardFilter: &lapiv2.WildcardFilter{},
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
		msg, err := resp.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			// Filter by module and entity name to avoid matching contracts from other modules
			templateId := contract.CreatedEvent.TemplateId
			if templateId.ModuleName == "Common.FingerprintAuth" && templateId.EntityName == "FingerprintMapping" {
				// Check if this mapping is for the target fingerprint
				fields := contract.CreatedEvent.CreateArguments.Fields
				for _, field := range fields {
					if field.Label == "fingerprint" {
						if fp, ok := field.Value.Sum.(*lapiv2.Value_Text); ok {
							if fp.Text == targetFingerprint {
								return contract.CreatedEvent.ContractId, nil
							}
						}
					}
				}
			}
		}
	}
	return "", fmt.Errorf("no FingerprintMapping found for fingerprint: %s", targetFingerprint)
}

func registerUser(ctx context.Context, client lapiv2.CommandServiceClient, issuer, packageID, domainID, configCid, userParty, fingerprint, evmAddress string) (string, error) {
	cmdID := fmt.Sprintf("register-user-%s", uuid.New().String())

	// Build choice arguments
	fields := []*lapiv2.RecordField{
		{Label: "userParty", Value: &lapiv2.Value{Sum: &lapiv2.Value_Party{Party: userParty}}},
		{Label: "fingerprint", Value: &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: fingerprint}}},
	}

	// Add optional EVM address
	if evmAddress != "" {
		fields = append(fields, &lapiv2.RecordField{
			Label: "evmAddress",
			Value: &lapiv2.Value{
				Sum: &lapiv2.Value_Optional{
					Optional: &lapiv2.Optional{
						Value: &lapiv2.Value{
							Sum: &lapiv2.Value_Record{
								Record: &lapiv2.Record{
									Fields: []*lapiv2.RecordField{
										{Label: "value", Value: &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: evmAddress}}},
									},
								},
							},
						},
					},
				},
			},
		})
	} else {
		fields = append(fields, &lapiv2.RecordField{
			Label: "evmAddress",
			Value: &lapiv2.Value{Sum: &lapiv2.Value_Optional{Optional: &lapiv2.Optional{Value: nil}}},
		})
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  packageID,
					ModuleName: "Wayfinder.Bridge",
					EntityName: "WayfinderBridgeConfig",
				},
				ContractId: configCid,
				Choice:     "RegisterUser",
				ChoiceArgument: &lapiv2.Value{
					Sum: &lapiv2.Value_Record{
						Record: &lapiv2.Record{Fields: fields},
					},
				},
			},
		},
	}

	resp, err := client.SubmitAndWaitForTransaction(ctx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: domainID,
			CommandId:      cmdID,
			UserId:         "nKMdSdj49c2BoPDynr6kf3pkLsTghePa@clients", // JWT subject
			ActAs:          []string{issuer},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return "", err
	}

	// Extract FingerprintMapping contract ID
	if resp.Transaction != nil {
		for _, event := range resp.Transaction.Events {
			if created := event.GetCreated(); created != nil {
				templateId := created.TemplateId
				if templateId.ModuleName == "Common.FingerprintAuth" && templateId.EntityName == "FingerprintMapping" {
					return created.ContractId, nil
				}
			}
		}
	}

	return "", fmt.Errorf("FingerprintMapping not found in response")
}
