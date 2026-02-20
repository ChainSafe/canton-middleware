//go:build ignore

// Check actual user holdings from Canton directly (bypasses API cache)

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"gopkg.in/yaml.v3"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
)

type Config struct {
	Canton struct {
		RPCURL       string `yaml:"rpc_url"`
		RelayerParty string `yaml:"relayer_party"`
		Auth         struct {
			ClientID     string `yaml:"client_id"`
			ClientSecret string `yaml:"client_secret"`
			Audience     string `yaml:"audience"`
			TokenURL     string `yaml:"token_url"`
		} `yaml:"auth"`
	} `yaml:"canton"`
}

type UserMapping struct {
	Fingerprint string
	Party       string
	EvmAddress  string
}

func main() {
	configFile := "config.mainnet.yaml"
	if len(os.Args) > 1 {
		configFile = os.Args[1]
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		fmt.Printf("Failed to read config: %v\n", err)
		return
	}
	var cfg Config
	yaml.Unmarshal(data, &cfg)

	token := getToken(cfg.Canton.Auth.TokenURL, cfg.Canton.Auth.ClientID, cfg.Canton.Auth.ClientSecret, cfg.Canton.Auth.Audience)
	if token == "" {
		fmt.Println("Failed to get OAuth token")
		return
	}

	conn, err := grpc.NewClient(cfg.Canton.RPCURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		return
	}
	defer conn.Close()

	stateService := lapiv2.NewStateServiceClient(conn)
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)

	resp, err := stateService.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		fmt.Printf("Failed to get ledger end: %v\n", err)
		return
	}

	fmt.Println("=== User Holdings on Canton (Direct Query) ===")
	fmt.Printf("Config: %s\n", configFile)
	fmt.Printf("Ledger offset: %d\n\n", resp.Offset)

	// Step 1: Get all FingerprintMappings to know which party belongs to which user
	mappings := make(map[string]*UserMapping) // party -> mapping
	contracts, _ := stateService.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: resp.Offset,
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

	mappingCount := 0
	for {
		msg, err := contracts.Recv()
		if err != nil {
			break
		}
		if c := msg.GetActiveContract(); c != nil {
			tid := c.CreatedEvent.TemplateId
			if tid.ModuleName == "Common.FingerprintAuth" && tid.EntityName == "FingerprintMapping" {
				mappingCount++
				fields := recordToMap(c.CreatedEvent.CreateArguments)
				fp := extractText(fields["fingerprint"])
				userParty := extractParty(fields["userParty"])
				evmAddr := ""
				if opt := fields["evmAddress"]; opt != nil {
					if optVal := opt.GetOptional(); optVal != nil && optVal.Value != nil {
						if rec := optVal.Value.GetRecord(); rec != nil {
							for _, f := range rec.Fields {
								if f.Label == "evmAddress" {
									evmAddr = extractText(f.Value)
								}
							}
						}
					}
				}
				mappings[userParty] = &UserMapping{
					Fingerprint: fp,
					Party:       userParty,
					EvmAddress:  evmAddr,
				}
			}
		}
	}

	fmt.Printf("Found %d FingerprintMappings (%d unique parties)\n\n", mappingCount, len(mappings))

	// Step 2: Get all CIP56Holdings and sum by owner party
	partyBalances := make(map[string]decimal.Decimal) // party -> balance
	totalSupply := decimal.Zero

	contracts2, _ := stateService.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: resp.Offset,
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

	holdingCount := 0
	for {
		msg, err := contracts2.Recv()
		if err != nil {
			break
		}
		if c := msg.GetActiveContract(); c != nil {
			tid := c.CreatedEvent.TemplateId
			if tid.ModuleName == "CIP56.Token" && tid.EntityName == "CIP56Holding" {
				holdingCount++
				fields := recordToMap(c.CreatedEvent.CreateArguments)
				owner := extractParty(fields["owner"])
				amountStr := extractNumeric(fields["amount"])

				amount, err := decimal.NewFromString(amountStr)
				if err != nil {
					continue
				}

				totalSupply = totalSupply.Add(amount)

				if current, ok := partyBalances[owner]; ok {
					partyBalances[owner] = current.Add(amount)
				} else {
					partyBalances[owner] = amount
				}
			}
		}
	}

	fmt.Printf("Found %d CIP56Holding contracts\n", holdingCount)
	fmt.Printf("Total Supply: %s PROMPT\n\n", totalSupply.String())

	// Check if multiple users share the same party (issuer-centric model issue)
	if mappingCount > len(mappings) {
		fmt.Println("⚠️  WARNING: Multiple users share the same Canton party!")
		fmt.Println("   In this setup, individual user balances CANNOT be determined")
		fmt.Println("   because all holdings are owned by the shared party.")
		fmt.Println()
	}

	// Step 3: Map holdings to users
	fmt.Println("--- User Balances (by Party) ---")
	for party, mapping := range mappings {
		balance := decimal.Zero
		if bal, ok := partyBalances[party]; ok {
			balance = bal
		}
		shortFp := mapping.Fingerprint
		if len(shortFp) > 20 {
			shortFp = shortFp[:20] + "..."
		}
		if mapping.EvmAddress != "" {
			fmt.Printf("✓ EVM: %s\n", mapping.EvmAddress)
		} else {
			fmt.Printf("✓ Fingerprint: %s\n", shortFp)
		}
		fmt.Printf("  Party: %s...\n", truncate(party, 50))
		fmt.Printf("  Holdings owned by this party: %s PROMPT\n\n", balance.String())
	}

	// Show any holdings that don't belong to registered users
	var unmappedTotal decimal.Decimal
	for party, balance := range partyBalances {
		if _, ok := mappings[party]; !ok {
			unmappedTotal = unmappedTotal.Add(balance)
		}
	}
	if !unmappedTotal.IsZero() {
		fmt.Printf("--- Holdings by other parties: %s PROMPT ---\n", unmappedTotal.String())
	}

	// Summary
	fmt.Println("=== Summary ===")
	fmt.Printf("Total Supply: %s PROMPT\n", totalSupply.String())
	fmt.Printf("Registered Users: %d (mapped to %d unique parties)\n", mappingCount, len(mappings))
	if mappingCount > len(mappings) {
		fmt.Println("\nNOTE: Individual user balances are indistinguishable on-chain")
		fmt.Println("because multiple users share the same Canton party.")
	}
}

func getToken(tokenURL, clientID, clientSecret, audience string) string {
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"audience":      {audience},
	}
	resp, err := http.Post(tokenURL, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		AccessToken string `json:"access_token"`
	}
	json.Unmarshal(body, &result)
	return result.AccessToken
}

func recordToMap(r *lapiv2.Record) map[string]*lapiv2.Value {
	m := make(map[string]*lapiv2.Value)
	if r != nil {
		for _, f := range r.Fields {
			m[f.Label] = f.Value
		}
	}
	return m
}

func extractParty(v *lapiv2.Value) string {
	if v != nil {
		return v.GetParty()
	}
	return ""
}

func extractNumeric(v *lapiv2.Value) string {
	if v != nil {
		return v.GetNumeric()
	}
	return ""
}

func extractText(v *lapiv2.Value) string {
	if v != nil {
		return v.GetText()
	}
	return ""
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
