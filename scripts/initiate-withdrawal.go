//go:build ignore

// initiate-withdrawal.go - Initiate a withdrawal from Canton to EVM
//
// Usage:
//   go run scripts/initiate-withdrawal.go -config config.yaml \
//     -holding-cid "00..." \
//     -amount "50.0" \
//     -evm-destination "0x70997970C51812dc3A010C7d01b50e0d17dc79C8"

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	iwConfigPath     = flag.String("config", "config.yaml", "Path to config file")
	iwHoldingCid     = flag.String("holding-cid", "", "Contract ID of the CIP56Holding to withdraw from")
	iwAmount         = flag.String("amount", "", "Amount to withdraw (e.g., '50.0')")
	iwEvmDestination = flag.String("evm-destination", "", "EVM address to receive the tokens")
)

func main() {
	flag.Parse()

	if *iwHoldingCid == "" || *iwAmount == "" || *iwEvmDestination == "" {
		fmt.Println("Error: -holding-cid, -amount, and -evm-destination are all required")
		fmt.Println("Usage: go run scripts/initiate-withdrawal.go -config config.yaml \\")
		fmt.Println("         -holding-cid '00...' -amount '50.0' -evm-destination '0x...'")
		os.Exit(1)
	}

	// Validate EVM address
	if !strings.HasPrefix(*iwEvmDestination, "0x") || len(*iwEvmDestination) != 42 {
		fmt.Println("Error: Invalid EVM destination address. Must be 0x followed by 40 hex chars.")
		os.Exit(1)
	}

	// Load config
	cfg, err := config.Load(*iwConfigPath)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("======================================================================")
	fmt.Println("INITIATE WITHDRAWAL - Canton to EVM")
	fmt.Println("======================================================================")
	fmt.Printf("Holding CID:     %s\n", *iwHoldingCid)
	fmt.Printf("Amount:          %s\n", *iwAmount)
	fmt.Printf("EVM Destination: %s\n", *iwEvmDestination)
	fmt.Println()

	ctx := context.Background()

	// Connect to Canton
	conn, err := grpc.NewClient(cfg.Canton.RPCURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Printf("Failed to connect to Canton: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	stateClient := lapiv2.NewStateServiceClient(conn)
	cmdClient := lapiv2.NewCommandServiceClient(conn)

	// Get ledger end offset
	ledgerEndResp, err := stateClient.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		fmt.Printf("Failed to get ledger end: %v\n", err)
		os.Exit(1)
	}
	if ledgerEndResp.Offset == 0 {
		fmt.Println("Error: Ledger is empty.")
		os.Exit(1)
	}

	// Step 1: Find WayfinderBridgeConfig
	fmt.Println(">>> Finding WayfinderBridgeConfig...")
	configCid, err := iwFindBridgeConfig(ctx, stateClient, cfg.Canton.RelayerParty, cfg.Canton.BridgePackageID, ledgerEndResp.Offset)
	if err != nil {
		fmt.Printf("Failed to find WayfinderBridgeConfig: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("    Config CID: %s\n\n", configCid)

	// Step 2: Find FingerprintMapping for the holding owner
	fmt.Println(">>> Finding holding owner and FingerprintMapping...")
	owner, err := iwGetHoldingOwner(ctx, stateClient, cfg.Canton.RelayerParty, ledgerEndResp.Offset, *iwHoldingCid)
	if err != nil {
		fmt.Printf("Failed to get holding owner: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("    Holding Owner: %s\n", owner)

	// Extract fingerprint from owner party ID
	fingerprint := iwExtractFingerprint(owner)
	fmt.Printf("    Fingerprint: %s\n", fingerprint)

	mappingCid, err := iwFindFingerprintMapping(ctx, stateClient, cfg.Canton.RelayerParty, ledgerEndResp.Offset, fingerprint)
	if err != nil {
		fmt.Printf("Failed to find FingerprintMapping: %v\n", err)
		fmt.Println("\nUser must be registered first. Run:")
		fmt.Printf("  go run scripts/register-user.go -config config.yaml -party '%s'\n", owner)
		os.Exit(1)
	}
	fmt.Printf("    Mapping CID: %s\n\n", mappingCid)

	// Step 3: Get domain ID
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

	// Step 4: Initiate withdrawal (creates WithdrawalRequest)
	fmt.Println(">>> Initiating withdrawal...")
	withdrawalRequestCid, err := iwInitiateWithdrawal(
		ctx,
		cmdClient,
		cfg.Canton.RelayerParty,
		cfg.Canton.BridgePackageID,
		domainID,
		configCid,
		mappingCid,
		*iwHoldingCid,
		*iwAmount,
		*iwEvmDestination,
	)
	if err != nil {
		fmt.Printf("Failed to initiate withdrawal: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("    WithdrawalRequest CID: %s\n\n", withdrawalRequestCid)

	// Step 5: Process withdrawal (burns tokens and creates WithdrawalEvent)
	// Note: WithdrawalRequest is in bridge-core package, not bridge-wayfinder
	fmt.Println(">>> Processing withdrawal (burning tokens)...")
	withdrawalEventCid, err := iwProcessWithdrawal(
		ctx,
		cmdClient,
		cfg.Canton.RelayerParty,
		cfg.Canton.CorePackageID, // Use core package for WithdrawalRequest
		domainID,
		withdrawalRequestCid,
	)
	if err != nil {
		fmt.Printf("Failed to process withdrawal: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("    WithdrawalEvent CID: %s\n\n", withdrawalEventCid)

	fmt.Println("======================================================================")
	fmt.Println("WITHDRAWAL PROCESSED SUCCESSFULLY")
	fmt.Println("======================================================================")
	fmt.Printf("WithdrawalRequest CID: %s\n", withdrawalRequestCid)
	fmt.Printf("WithdrawalEvent CID:   %s\n", withdrawalEventCid)
	fmt.Println()
	fmt.Println("The relayer will now:")
	fmt.Println("  1. Detect the WithdrawalEvent on Canton")
	fmt.Println("  2. Submit withdrawFromCanton() transaction on EVM")
	fmt.Println("  3. Mark the withdrawal as complete on Canton")
	fmt.Println()
	fmt.Println("Monitor the relayer logs to see progress.")
}

func iwFindBridgeConfig(ctx context.Context, client lapiv2.StateServiceClient, party, packageID string, offset int64) (string, error) {
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

func iwGetHoldingOwner(ctx context.Context, client lapiv2.StateServiceClient, party string, offset int64, holdingCid string) (string, error) {
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
			if contract.CreatedEvent.ContractId == holdingCid {
				fields := contract.CreatedEvent.CreateArguments.Fields
				for _, field := range fields {
					if field.Label == "owner" {
						if p, ok := field.Value.Sum.(*lapiv2.Value_Party); ok {
							return p.Party, nil
						}
					}
				}
			}
		}
	}
	return "", fmt.Errorf("holding not found: %s", holdingCid)
}

func iwFindFingerprintMapping(ctx context.Context, client lapiv2.StateServiceClient, party string, offset int64, targetFingerprint string) (string, error) {
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

func iwExtractFingerprint(partyID string) string {
	parts := strings.Split(partyID, "::")
	if len(parts) == 2 {
		fp := parts[1]
		// Remove "1220" multihash prefix if present
		if strings.HasPrefix(fp, "1220") && len(fp) == 68 {
			return fp[4:]
		}
		return fp
	}
	return partyID
}

func iwInitiateWithdrawal(
	ctx context.Context,
	client lapiv2.CommandServiceClient,
	issuer, packageID, domainID, configCid, mappingCid, holdingCid, amount, evmDestination string,
) (string, error) {
	cmdID := fmt.Sprintf("initiate-withdrawal-%s", uuid.New().String())

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  packageID,
					ModuleName: "Wayfinder.Bridge",
					EntityName: "WayfinderBridgeConfig",
				},
				ContractId: configCid,
				Choice:     "InitiateWithdrawal",
				ChoiceArgument: &lapiv2.Value{
					Sum: &lapiv2.Value_Record{
						Record: &lapiv2.Record{
							Fields: []*lapiv2.RecordField{
								{Label: "mappingCid", Value: &lapiv2.Value{Sum: &lapiv2.Value_ContractId{ContractId: mappingCid}}},
								{Label: "holdingCid", Value: &lapiv2.Value{Sum: &lapiv2.Value_ContractId{ContractId: holdingCid}}},
								{Label: "amount", Value: &lapiv2.Value{Sum: &lapiv2.Value_Numeric{Numeric: amount}}},
								{Label: "evmDestination", Value: &lapiv2.Value{
									Sum: &lapiv2.Value_Record{
										Record: &lapiv2.Record{
											Fields: []*lapiv2.RecordField{
												{Label: "value", Value: &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: evmDestination}}},
											},
										},
									},
								}},
							},
						},
					},
				},
			},
		},
	}

	resp, err := client.SubmitAndWaitForTransaction(ctx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: domainID,
			CommandId:      cmdID,
			UserId:         "bridge-operator",
			ActAs:          []string{issuer},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return "", err
	}

	// Extract WithdrawalRequest contract ID
	if resp.Transaction != nil {
		for _, event := range resp.Transaction.Events {
			if created := event.GetCreated(); created != nil {
				templateId := created.TemplateId
				if templateId.ModuleName == "Bridge.Contracts" && templateId.EntityName == "WithdrawalRequest" {
					return created.ContractId, nil
				}
			}
		}
	}

	return "", fmt.Errorf("WithdrawalRequest not found in response")
}

func iwProcessWithdrawal(
	ctx context.Context,
	client lapiv2.CommandServiceClient,
	issuer, packageID, domainID, withdrawalRequestCid string,
) (string, error) {
	cmdID := fmt.Sprintf("process-withdrawal-%s", uuid.New().String())

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  packageID,
					ModuleName: "Bridge.Contracts",
					EntityName: "WithdrawalRequest",
				},
				ContractId:     withdrawalRequestCid,
				Choice:         "ProcessWithdrawal",
				ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: &lapiv2.Record{}}},
			},
		},
	}

	resp, err := client.SubmitAndWaitForTransaction(ctx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: domainID,
			CommandId:      cmdID,
			UserId:         "bridge-operator",
			ActAs:          []string{issuer},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return "", err
	}

	// Extract WithdrawalEvent contract ID
	if resp.Transaction != nil {
		for _, event := range resp.Transaction.Events {
			if created := event.GetCreated(); created != nil {
				templateId := created.TemplateId
				if templateId.ModuleName == "Bridge.Contracts" && templateId.EntityName == "WithdrawalEvent" {
					return created.ContractId, nil
				}
			}
		}
	}

	return "", fmt.Errorf("WithdrawalEvent not found in response")
}
