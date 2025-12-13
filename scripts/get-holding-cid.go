//go:build ignore

// get-holding-cid.go - Get the CIP56Holding with the largest balance for withdrawals
// Usage:
//   go run scripts/get-holding-cid.go -config .test-config.yaml              # Output: contract_id
//   go run scripts/get-holding-cid.go -config .test-config.yaml -with-balance # Output: contract_id balance
//
// Note: When multiple holdings exist (from multiple deposits), this returns the one
// with the LARGEST balance, which is most useful for withdrawals.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

var (
	configPath  = flag.String("config", "config.yaml", "Path to config file")
	withBalance = flag.Bool("with-balance", false, "Also output the holding balance")
)

type holding struct {
	contractID string
	balance    decimal.Decimal
	balanceStr string
}

func main() {
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	partyID := cfg.Canton.RelayerParty
	if partyID == "" {
		fmt.Fprintf(os.Stderr, "Error: canton.relayer_party is required\n")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get OAuth token
	token, err := getOAuthToken(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get OAuth token: %v\n", err)
		os.Exit(1)
	}

	// Connect to Canton
	conn, err := grpc.NewClient(cfg.Canton.RPCURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Add auth to context
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)

	// Get ledger end offset first
	stateClient := lapiv2.NewStateServiceClient(conn)
	
	ledgerEnd, err := stateClient.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get ledger end: %v\n", err)
		os.Exit(1)
	}
	offset := ledgerEnd.Offset

	// Get active contracts
	resp, err := stateClient.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: offset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				partyID: {
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
		fmt.Fprintf(os.Stderr, "Failed to get contracts: %v\n", err)
		os.Exit(1)
	}

	// Collect ALL holdings to find the one with largest balance
	var holdings []holding

	for {
		msg, err := resp.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Stream error: %v\n", err)
			os.Exit(1)
		}

		if contract := msg.GetActiveContract(); contract != nil {
			templateID := contract.CreatedEvent.TemplateId
			if templateID.ModuleName == "CIP56.Token" && templateID.EntityName == "CIP56Holding" {
				contractID := contract.CreatedEvent.ContractId
				balanceStr := "0"
				
				// Extract balance from contract arguments
				if contract.CreatedEvent.CreateArguments != nil {
					for _, field := range contract.CreatedEvent.CreateArguments.Fields {
						if field.Label == "amount" && field.Value != nil {
							if n, ok := field.Value.Sum.(*lapiv2.Value_Numeric); ok {
								balanceStr = n.Numeric
							}
						}
					}
				}
				
				bal, err := decimal.NewFromString(balanceStr)
				if err != nil {
					bal = decimal.Zero
				}
				
				holdings = append(holdings, holding{
					contractID: contractID,
					balance:    bal,
					balanceStr: balanceStr,
				})
			}
		}
	}

	if len(holdings) == 0 {
		fmt.Fprintf(os.Stderr, "No CIP56Holding found\n")
		os.Exit(1)
	}

	// Find the holding with the largest balance
	best := holdings[0]
	for _, h := range holdings[1:] {
		if h.balance.GreaterThan(best.balance) {
			best = h
		}
	}

	if *withBalance {
		fmt.Printf("%s %s\n", best.contractID, best.balanceStr)
	} else {
		fmt.Println(best.contractID)
	}
	os.Exit(0)
}

func getOAuthToken(cfg *config.Config) (string, error) {
	data := fmt.Sprintf("grant_type=client_credentials&client_id=%s&client_secret=%s&audience=%s",
		cfg.Canton.Auth.ClientID,
		cfg.Canton.Auth.ClientSecret,
		cfg.Canton.Auth.Audience)

	resp, err := http.Post(cfg.Canton.Auth.TokenURL, "application/x-www-form-urlencoded", strings.NewReader(data))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.AccessToken, nil
}
