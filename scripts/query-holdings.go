//go:build ignore

// query-holdings.go - Query CIP56Holding contracts for a user on Canton
//
// Usage:
//   go run scripts/query-holdings.go -config config.yaml \
//     -party "BridgeIssuer::122047584945db4991c2954b1e8e673623a43ec80869abf0f8e7531a435ae797ac6e"

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	qhConfigPath = flag.String("config", "config.yaml", "Path to config file")
	qhPartyID    = flag.String("party", "", "Canton Party ID to query holdings for")
)

type Holding struct {
	ContractID string
	Owner      string
	Amount     string
	TokenID    string
}

func main() {
	flag.Parse()

	if *qhPartyID == "" {
		fmt.Println("Error: -party is required")
		fmt.Println("Usage: go run scripts/query-holdings.go -config config.yaml -party 'PartyID'")
		os.Exit(1)
	}

	// Load config
	cfg, err := config.Load(*qhConfigPath)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("======================================================================")
	fmt.Println("QUERY HOLDINGS - List CIP56Holding contracts on Canton")
	fmt.Println("======================================================================")
	fmt.Printf("Party: %s\n\n", *qhPartyID)

	ctx := context.Background()

	// Connect to Canton
	conn, err := grpc.NewClient(cfg.Canton.RPCURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Printf("Failed to connect to Canton: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	stateClient := lapiv2.NewStateServiceClient(conn)

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

	// Query holdings
	holdings, err := qhQueryHoldings(ctx, stateClient, *qhPartyID, ledgerEndResp.Offset)
	if err != nil {
		fmt.Printf("Failed to query holdings: %v\n", err)
		os.Exit(1)
	}

	if len(holdings) == 0 {
		fmt.Println("No CIP56Holding contracts found for this party.")
		fmt.Println("\nTo create holdings, run a deposit from EVM to Canton.")
		os.Exit(0)
	}

	fmt.Printf("Found %d holding(s):\n\n", len(holdings))
	for i, h := range holdings {
		fmt.Printf("Holding #%d:\n", i+1)
		fmt.Printf("  Contract ID: %s\n", h.ContractID)
		fmt.Printf("  Owner:       %s\n", h.Owner)
		fmt.Printf("  Amount:      %s\n", h.Amount)
		fmt.Printf("  Token ID:    %s\n", h.TokenID)
		fmt.Println()
	}

	fmt.Println("======================================================================")
	fmt.Println("To initiate a withdrawal, use:")
	fmt.Println("  go run scripts/initiate-withdrawal.go -config config.yaml \\")
	fmt.Printf("    -holding-cid \"%s\" \\\n", holdings[0].ContractID)
	fmt.Println("    -amount \"50.0\" \\")
	fmt.Println("    -evm-destination \"0x70997970C51812dc3A010C7d01b50e0d17dc79C8\"")
}

func qhQueryHoldings(ctx context.Context, client lapiv2.StateServiceClient, party string, offset int64) ([]Holding, error) {
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

	var holdings []Holding
	for {
		msg, err := resp.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			// Filter by module and entity name to avoid matching contracts from other modules
			templateId := contract.CreatedEvent.TemplateId
			if templateId.ModuleName == "CIP56.Token" && templateId.EntityName == "CIP56Holding" {
				h := Holding{
					ContractID: contract.CreatedEvent.ContractId,
				}

				// Extract fields from the contract
				fields := contract.CreatedEvent.CreateArguments.Fields
				for _, field := range fields {
					switch field.Label {
					case "owner":
						if p, ok := field.Value.Sum.(*lapiv2.Value_Party); ok {
							h.Owner = p.Party
						}
					case "amount":
						if n, ok := field.Value.Sum.(*lapiv2.Value_Numeric); ok {
							h.Amount = n.Numeric
						}
					case "tokenId":
						if t, ok := field.Value.Sum.(*lapiv2.Value_Text); ok {
							h.TokenID = t.Text
						}
					}
				}
				holdings = append(holdings, h)
			}
		}
	}

	return holdings, nil
}
