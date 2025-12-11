//go:build ignore

// cleanup-withdrawals.go - Clean up stale pending WithdrawalEvent contracts on Canton
//
// This script finds all pending WithdrawalEvent contracts and marks them as completed
// with a cleanup marker. This is useful for cleaning up stale withdrawals from previous
// test runs (e.g., when switching from Anvil to Sepolia).
//
// Usage:
//   go run scripts/cleanup-withdrawals.go -config config.devnet.yaml
//
// Options:
//   -dry-run    List pending withdrawals without completing them (default: true)
//   -force      Actually complete the withdrawals (use with caution)

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
	cwConfigPath = flag.String("config", "config.devnet.yaml", "Path to config file")
	cwDryRun     = flag.Bool("dry-run", true, "List pending withdrawals without completing them")
	cwForce      = flag.Bool("force", false, "Actually complete the withdrawals")
)

type PendingWithdrawal struct {
	ContractID     string
	UserParty      string
	EvmDestination string
	Amount         string
	Fingerprint    string
	Status         string
}

func main() {
	flag.Parse()

	// Safety check: require explicit -force to actually complete withdrawals
	if !*cwDryRun && !*cwForce {
		fmt.Println("Error: Use -force to actually complete withdrawals, or use -dry-run=true to list them")
		os.Exit(1)
	}

	// Load config
	cfg, err := config.Load(*cwConfigPath)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("======================================================================")
	fmt.Println("CLEANUP STALE WITHDRAWALS - Mark pending WithdrawalEvents as complete")
	fmt.Println("======================================================================")
	if *cwDryRun {
		fmt.Println("MODE: Dry-run (listing only, use -dry-run=false -force to execute)")
	} else {
		fmt.Println("MODE: EXECUTING - Will mark withdrawals as complete!")
	}
	fmt.Printf("Config: %s\n", *cwConfigPath)
	fmt.Printf("Party: %s\n\n", cfg.Canton.RelayerParty)

	ctx := context.Background()

	// Connect to Canton with TLS if enabled
	var opts []grpc.DialOption
	if cfg.Canton.TLS.Enabled {
		tlsConfig := &tls.Config{}
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

	// Get ledger end offset (required for V2 API)
	ledgerEndResp, err := stateClient.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		fmt.Printf("Failed to get ledger end: %v\n", err)
		os.Exit(1)
	}
	if ledgerEndResp.Offset == 0 {
		fmt.Println("Error: Ledger is empty.")
		os.Exit(1)
	}
	fmt.Printf("Ledger offset: %d\n\n", ledgerEndResp.Offset)

	// Query pending withdrawals
	fmt.Println(">>> Querying pending WithdrawalEvent contracts...")
	withdrawals, err := cwQueryPendingWithdrawals(ctx, stateClient, cfg.Canton.RelayerParty, ledgerEndResp.Offset)
	if err != nil {
		fmt.Printf("Failed to query withdrawals: %v\n", err)
		os.Exit(1)
	}

	if len(withdrawals) == 0 {
		fmt.Println("\n✓ No pending WithdrawalEvent contracts found. Nothing to clean up!")
		os.Exit(0)
	}

	fmt.Printf("\nFound %d pending withdrawal(s):\n\n", len(withdrawals))
	for i, w := range withdrawals {
		fmt.Printf("Withdrawal #%d:\n", i+1)
		fmt.Printf("  Contract ID:     %s\n", w.ContractID)
		fmt.Printf("  User Party:      %s\n", w.UserParty)
		fmt.Printf("  EVM Destination: %s\n", w.EvmDestination)
		fmt.Printf("  Amount:          %s\n", w.Amount)
		fmt.Printf("  Fingerprint:     %s\n", w.Fingerprint)
		fmt.Printf("  Status:          %s\n", w.Status)
		fmt.Println()
	}

	if *cwDryRun {
		fmt.Println("======================================================================")
		fmt.Println("DRY-RUN COMPLETE")
		fmt.Println("======================================================================")
		fmt.Printf("Found %d pending withdrawal(s) that would be marked as complete.\n", len(withdrawals))
		fmt.Println()
		fmt.Println("To actually clean them up, run:")
		fmt.Printf("  go run scripts/cleanup-withdrawals.go -config %s -dry-run=false -force\n", *cwConfigPath)
		os.Exit(0)
	}

	// Get domain ID
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

	// Complete each pending withdrawal
	fmt.Println(">>> Completing stale withdrawals...")
	completed := 0
	failed := 0
	for i, w := range withdrawals {
		cleanupTxHash := fmt.Sprintf("cleanup-stale-%d-0x%s", i, strings.Repeat("0", 60))
		fmt.Printf("\n[%d/%d] Completing withdrawal %s...\n", i+1, len(withdrawals), w.ContractID[:20]+"...")

		err := cwCompleteWithdrawal(
			ctx,
			cmdClient,
			cfg.Canton.RelayerParty,
			cfg.Canton.CorePackageID,
			domainID,
			w.ContractID,
			cleanupTxHash,
		)
		if err != nil {
			fmt.Printf("    ✗ Failed: %v\n", err)
			failed++
		} else {
			fmt.Printf("    ✓ Completed with cleanup marker: %s\n", cleanupTxHash[:40]+"...")
			completed++
		}
	}

	fmt.Println()
	fmt.Println("======================================================================")
	fmt.Println("CLEANUP COMPLETE")
	fmt.Println("======================================================================")
	fmt.Printf("Completed: %d\n", completed)
	fmt.Printf("Failed:    %d\n", failed)
	fmt.Printf("Total:     %d\n", len(withdrawals))
	fmt.Println()
	if completed > 0 {
		fmt.Println("The relayer should no longer try to process these old withdrawals.")
	}
}

func cwQueryPendingWithdrawals(ctx context.Context, client lapiv2.StateServiceClient, party string, offset int64) ([]PendingWithdrawal, error) {
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
		return nil, err
	}

	var withdrawals []PendingWithdrawal
	for {
		msg, err := resp.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			// Filter for WithdrawalEvent contracts
			templateId := contract.CreatedEvent.TemplateId
			if templateId.ModuleName == "Bridge.Contracts" && templateId.EntityName == "WithdrawalEvent" {
				w := PendingWithdrawal{
					ContractID: contract.CreatedEvent.ContractId,
				}

				// Extract fields from the contract
				fields := contract.CreatedEvent.CreateArguments.Fields
				for _, field := range fields {
					switch field.Label {
					case "userParty":
						if p, ok := field.Value.Sum.(*lapiv2.Value_Party); ok {
							w.UserParty = p.Party
						}
					case "evmDestination":
						// EvmAddress is a record with a "value" field
						if r, ok := field.Value.Sum.(*lapiv2.Value_Record); ok {
							for _, f := range r.Record.Fields {
								if f.Label == "value" {
									if t, ok := f.Value.Sum.(*lapiv2.Value_Text); ok {
										w.EvmDestination = t.Text
									}
								}
							}
						}
					case "amount":
						if n, ok := field.Value.Sum.(*lapiv2.Value_Numeric); ok {
							w.Amount = n.Numeric
						}
					case "fingerprint":
						if t, ok := field.Value.Sum.(*lapiv2.Value_Text); ok {
							w.Fingerprint = t.Text
						}
					case "status":
						// Status is a variant
						if v, ok := field.Value.Sum.(*lapiv2.Value_Variant); ok {
							w.Status = v.Variant.Constructor
						}
					}
				}

				// Only include pending withdrawals
				if w.Status == "Pending" {
					withdrawals = append(withdrawals, w)
				}
			}
		}
	}

	return withdrawals, nil
}

func cwCompleteWithdrawal(
	ctx context.Context,
	client lapiv2.CommandServiceClient,
	issuer, packageID, domainID, withdrawalEventCid, evmTxHash string,
) error {
	cmdID := fmt.Sprintf("cleanup-withdrawal-%s", uuid.New().String())

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  packageID,
					ModuleName: "Bridge.Contracts",
					EntityName: "WithdrawalEvent",
				},
				ContractId: withdrawalEventCid,
				Choice:     "CompleteWithdrawal",
				ChoiceArgument: &lapiv2.Value{
					Sum: &lapiv2.Value_Record{
						Record: &lapiv2.Record{
							Fields: []*lapiv2.RecordField{
								{Label: "evmTxHash", Value: &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: evmTxHash}}},
							},
						},
					},
				},
			},
		},
	}

	_, err := client.SubmitAndWait(ctx, &lapiv2.SubmitAndWaitRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: domainID,
			CommandId:      cmdID,
			UserId:         "RSrzTpeADIJU4QHlWkr0xtmm2mgZ5Epb@clients", // JWT subject
			ActAs:          []string{issuer},
			Commands:       []*lapiv2.Command{cmd},
		},
	})

	return err
}
