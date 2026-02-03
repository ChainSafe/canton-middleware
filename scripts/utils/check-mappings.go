//go:build ignore

// Check FingerprintMapping contracts on Canton

package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"gopkg.in/yaml.v3"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
)

type Config struct {
	Canton struct {
		RPCURL       string `yaml:"rpc_url"`
		RelayerParty string `yaml:"relayer_party"`
		TLS          struct {
			Enabled bool `yaml:"enabled"`
		} `yaml:"tls"`
		Auth struct {
			ClientID     string `yaml:"client_id"`
			ClientSecret string `yaml:"client_secret"`
			Audience     string `yaml:"audience"`
			TokenURL     string `yaml:"token_url"`
		} `yaml:"auth"`
	} `yaml:"canton"`
}

func main() {
	// Load config
	data, _ := os.ReadFile("config.mainnet.yaml")
	var cfg Config
	yaml.Unmarshal(data, &cfg)

	// Connect
	var opts []grpc.DialOption
	if cfg.Canton.TLS.Enabled {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(cfg.Canton.RPCURL, opts...)
	if err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer conn.Close()

	// Get OAuth token
	token, err := getToken(&cfg)
	if err != nil {
		fmt.Printf("Auth error: %v\n", err)
		return
	}

	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
	stateService := lapiv2.NewStateServiceClient(conn)

	// Get ledger end
	ledgerEnd, _ := stateService.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})

	fmt.Println("=== FingerprintMapping Contracts on Mainnet ===")
	fmt.Printf("Ledger offset: %d\n\n", ledgerEnd.Offset)

	// Query all contracts
	resp, err := stateService.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: ledgerEnd.Offset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				cfg.Canton.RelayerParty: {
					Cumulative: []*lapiv2.CumulativeFilter{{
						IdentifierFilter: &lapiv2.CumulativeFilter_WildcardFilter{
							WildcardFilter: &lapiv2.WildcardFilter{},
						},
					}},
				},
			},
			Verbose: true,
		},
	})
	if err != nil {
		fmt.Printf("Query error: %v\n", err)
		return
	}

	mappingCount := 0
	for {
		msg, err := resp.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			templateId := contract.CreatedEvent.TemplateId
			if templateId.ModuleName == "Common.FingerprintAuth" && templateId.EntityName == "FingerprintMapping" {
				mappingCount++
				fmt.Printf("[%d] FingerprintMapping\n", mappingCount)
				fmt.Printf("    CID: %s...\n", contract.CreatedEvent.ContractId[:40])

				// Extract fingerprint
				if contract.CreatedEvent.CreateArguments != nil {
					for _, f := range contract.CreatedEvent.CreateArguments.Fields {
						if f.Label == "fingerprint" {
							if t, ok := f.Value.Sum.(*lapiv2.Value_Text); ok {
								fmt.Printf("    Fingerprint: %s\n", t.Text)
							}
						}
					}
				}
				fmt.Println()
			}
		}
	}

	if mappingCount == 0 {
		fmt.Println("No FingerprintMapping contracts found!")
		fmt.Println("\nThis means users haven't been registered on mainnet Canton yet.")
		fmt.Println("The API server might have cached data from a different network.")
	} else {
		fmt.Printf("Found %d FingerprintMapping contract(s)\n", mappingCount)
	}
}

func getToken(cfg *Config) (string, error) {
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", cfg.Canton.Auth.ClientID)
	data.Set("client_secret", cfg.Canton.Auth.ClientSecret)
	data.Set("audience", cfg.Canton.Auth.Audience)

	req, _ := http.NewRequest("POST", cfg.Canton.Auth.TokenURL, strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	json.Unmarshal(body, &tokenResp)
	return tokenResp.AccessToken, nil
}
