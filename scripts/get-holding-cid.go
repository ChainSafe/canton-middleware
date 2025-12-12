//go:build ignore

// get-holding-cid.go - Get the first CIP56Holding contract ID for withdrawals
// Usage: go run scripts/get-holding-cid.go -config .test-config.yaml

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

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

var configPath = flag.String("config", "config.yaml", "Path to config file")

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
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get contracts: %v\n", err)
		os.Exit(1)
	}

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
				// Print just the contract ID
				fmt.Println(contract.CreatedEvent.ContractId)
				os.Exit(0)
			}
		}
	}

	fmt.Fprintf(os.Stderr, "No CIP56Holding found\n")
	os.Exit(1)
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
