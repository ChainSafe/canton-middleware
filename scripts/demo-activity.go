//go:build ignore

// demo-activity.go - Display DEMO token holdings and transactions on Canton
//
// Usage:
//   go run scripts/demo-activity.go -config config.devnet.yaml           # DevNet
//   go run scripts/demo-activity.go -config .test-config.yaml            # Local Docker
//   go run scripts/demo-activity.go -config config.devnet.yaml -debug    # Show raw contract data
//
// This script shows:
//   - Active CIP56Holdings (DEMO token balances)
//   - MintEvents (token minting history)
//   - BurnEvents (token burning history)
//   - TransferEvents (token transfer history)
//   - NativeTokenConfig (token configuration)
//   - CIP56Manager (token manager contract)

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const maxMessageSize = 50 * 1024 * 1024 // 50MB

var (
	configPath = flag.String("config", "config.devnet.yaml", "Path to config file")
	debug      = flag.Bool("debug", false, "Show debug info about all contracts found")
	showAll    = flag.Bool("all", false, "Show all events (not just recent)")
)

// Token caching
var (
	tokenMu     sync.Mutex
	cachedToken string
	tokenExpiry time.Time
)

// Holding represents a DEMO token holding
type Holding struct {
	ContractID string
	Owner      string
	Issuer     string
	Amount     string
	TokenName  string
	TokenSymbol string
	Decimals   string
	CreatedAt  time.Time
	Offset     int64
}

// Event represents a token event
type Event struct {
	Type       string // MintEvent, BurnEvent, TransferEvent
	ContractID string
	Owner      string
	From       string
	To         string
	Amount     string
	CreatedAt  time.Time
	Offset     int64
}

// TokenConfig represents NativeTokenConfig
type TokenConfig struct {
	ContractID string
	Issuer     string
	Name       string
	Symbol     string
	Decimals   string
	ManagerCID string
	CreatedAt  time.Time
}

func main() {
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	partyID := cfg.Canton.RelayerParty
	if partyID == "" {
		fmt.Println("Error: canton.relayer_party is required in config")
		os.Exit(1)
	}

	cip56Pkg := cfg.Canton.CIP56PackageID
	nativePkg := cfg.Canton.NativeTokenPackageID

	if cip56Pkg == "" || nativePkg == "" {
		fmt.Println("Error: cip56_package_id and native_token_package_id are required in config")
		os.Exit(1)
	}

	// Connect to Canton
	conn, err := connectToCanton(cfg)
	if err != nil {
		fmt.Printf("Failed to connect to Canton: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	ctx := context.Background()
	if cfg.Canton.Auth.TokenURL != "" {
		token, err := getOAuthToken(cfg)
		if err != nil {
			fmt.Printf("Failed to get OAuth token: %v\n", err)
			os.Exit(1)
		}
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	}

	stateClient := lapiv2.NewStateServiceClient(conn)

	// Get ledger end
	endResp, err := stateClient.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		fmt.Printf("Failed to get ledger end: %v\n", err)
		os.Exit(1)
	}
	ledgerEnd := endResp.Offset

	// Print header
	fmt.Println()
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  DEMO Token Activity on Canton")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("  Canton RPC:     %s\n", cfg.Canton.RPCURL)
	fmt.Printf("  Party:          %s...\n", partyID[:50])
	fmt.Printf("  Ledger Offset:  %d\n", ledgerEnd)
	fmt.Printf("  CIP56 Package:  %s...\n", cip56Pkg[:16])
	fmt.Printf("  Native Package: %s...\n", nativePkg[:16])
	fmt.Println()

	// Query holdings
	holdings := queryHoldings(ctx, stateClient, partyID, cip56Pkg, ledgerEnd)
	
	// Query token config
	tokenConfigs := queryTokenConfigs(ctx, stateClient, partyID, nativePkg, ledgerEnd)
	
	// Query events
	mintEvents := queryEvents(ctx, stateClient, partyID, nativePkg, "MintEvent", ledgerEnd)
	burnEvents := queryEvents(ctx, stateClient, partyID, nativePkg, "BurnEvent", ledgerEnd)
	transferEvents := queryEvents(ctx, stateClient, partyID, nativePkg, "TransferEvent", ledgerEnd)

	// Print Token Config
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Native Token Configuration")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	if len(tokenConfigs) == 0 {
		fmt.Println("  No NativeTokenConfig found")
	} else {
		for _, tc := range tokenConfigs {
			fmt.Printf("  Token:      %s (%s)\n", tc.Name, tc.Symbol)
			fmt.Printf("  Decimals:   %s\n", tc.Decimals)
			fmt.Printf("  Contract:   %s...\n", tc.ContractID[:50])
			fmt.Printf("  Created:    %s\n", tc.CreatedAt.Format(time.RFC3339))
			fmt.Println()
		}
	}

	// Print Holdings
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Active CIP56Holdings (DEMO Token Balances)")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	
	if len(holdings) == 0 {
		fmt.Println("  No holdings found")
	} else {
		var totalAmount float64
		fmt.Printf("  %-6s  %-12s  %-50s  %s\n", "IDX", "AMOUNT", "CONTRACT ID", "CREATED")
		fmt.Println("  ──────────────────────────────────────────────────────────────────────")
		
		for i, h := range holdings {
			var amt float64
			fmt.Sscanf(h.Amount, "%f", &amt)
			totalAmount += amt
			
			fmt.Printf("  %-6d  %-12s  %s...  %s\n", 
				i+1,
				fmt.Sprintf("%.2f", amt),
				h.ContractID[:50],
				h.CreatedAt.Format("2006-01-02 15:04:05"))
		}
		
		fmt.Println("  ──────────────────────────────────────────────────────────────────────")
		fmt.Printf("  TOTAL:  %.2f DEMO across %d holdings\n", totalAmount, len(holdings))
	}
	fmt.Println()

	// Print Events Summary
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Token Events Summary")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("  Mint Events:     %d\n", len(mintEvents))
	fmt.Printf("  Burn Events:     %d\n", len(burnEvents))
	fmt.Printf("  Transfer Events: %d\n", len(transferEvents))
	fmt.Println()

	// Print recent events
	if len(mintEvents) > 0 {
		fmt.Println("  Recent Mint Events:")
		fmt.Println("  ─────────────────────────────────────────────────────────────────")
		limit := 5
		if len(mintEvents) < limit {
			limit = len(mintEvents)
		}
		for i := 0; i < limit; i++ {
			e := mintEvents[i]
			fmt.Printf("    %s: %.4f DEMO minted\n", e.CreatedAt.Format("2006-01-02 15:04:05"), parseAmount(e.Amount))
		}
		fmt.Println()
	}

	if len(transferEvents) > 0 {
		fmt.Println("  Recent Transfer Events:")
		fmt.Println("  ─────────────────────────────────────────────────────────────────")
		limit := 5
		if len(transferEvents) < limit {
			limit = len(transferEvents)
		}
		for i := 0; i < limit; i++ {
			e := transferEvents[i]
			fmt.Printf("    %s: %.4f DEMO transferred\n", e.CreatedAt.Format("2006-01-02 15:04:05"), parseAmount(e.Amount))
		}
		fmt.Println()
	}

	// Print summary
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Summary")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	
	var totalHoldings float64
	for _, h := range holdings {
		totalHoldings += parseAmount(h.Amount)
	}
	
	var totalMinted float64
	for _, e := range mintEvents {
		totalMinted += parseAmount(e.Amount)
	}
	
	var totalBurned float64
	for _, e := range burnEvents {
		totalBurned += parseAmount(e.Amount)
	}
	
	fmt.Printf("  Total Holdings:    %.2f DEMO\n", totalHoldings)
	fmt.Printf("  Total Minted:      %.2f DEMO\n", totalMinted)
	fmt.Printf("  Total Burned:      %.2f DEMO\n", totalBurned)
	fmt.Printf("  Net Supply:        %.2f DEMO\n", totalMinted-totalBurned)
	fmt.Println()
}

func parseAmount(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

func connectToCanton(cfg *config.Config) (*grpc.ClientConn, error) {
	var opts []grpc.DialOption

	if cfg.Canton.TLS.Enabled {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	opts = append(opts, grpc.WithDefaultCallOptions(
		grpc.MaxCallRecvMsgSize(maxMessageSize),
	))

	return grpc.Dial(cfg.Canton.RPCURL, opts...)
}

func getOAuthToken(cfg *config.Config) (string, error) {
	tokenMu.Lock()
	defer tokenMu.Unlock()

	if cachedToken != "" && time.Now().Before(tokenExpiry) {
		return cachedToken, nil
	}

	payload := map[string]string{
		"client_id":     cfg.Canton.Auth.ClientID,
		"client_secret": cfg.Canton.Auth.ClientSecret,
		"audience":      cfg.Canton.Auth.Audience,
		"grant_type":    "client_credentials",
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(cfg.Canton.Auth.TokenURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("OAuth error: %s", string(data))
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}

	cachedToken = result.AccessToken
	tokenExpiry = time.Now().Add(time.Duration(result.ExpiresIn-60) * time.Second)

	// Extract subject from JWT for debugging
	token, _, _ := new(jwt.Parser).ParseUnverified(cachedToken, jwt.MapClaims{})
	if token != nil {
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			if sub, ok := claims["sub"].(string); ok {
				fmt.Printf("  JWT Subject:    %s\n", sub)
			}
		}
	}

	return cachedToken, nil
}

func queryHoldings(ctx context.Context, client lapiv2.StateServiceClient, party, packageID string, offset int64) []Holding {
	filter := &lapiv2.EventFormat{
		FiltersByParty: map[string]*lapiv2.Filters{
			party: {
				Cumulative: []*lapiv2.CumulativeFilter{
					{
						IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
							TemplateFilter: &lapiv2.TemplateFilter{
								TemplateId: &lapiv2.Identifier{
									PackageId:  packageID,
									ModuleName: "CIP56.Token",
									EntityName: "CIP56Holding",
								},
							},
						},
					},
				},
			},
		},
	}

	stream, err := client.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: offset,
		EventFormat:    filter,
	})
	if err != nil {
		fmt.Printf("  Error querying holdings: %v\n", err)
		return nil
	}

	var holdings []Holding
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("  Error receiving holdings: %v\n", err)
			break
		}

		if resp.GetActiveContract() != nil {
			event := resp.GetActiveContract().GetCreatedEvent()
			if event != nil {
				h := Holding{
					ContractID: event.ContractId,
					Offset:     event.Offset,
				}

				// Parse create arguments
				fields := event.GetCreateArguments().GetFields()
				if len(fields) >= 4 {
					h.Issuer = fields[0].GetValue().GetParty()
					h.Owner = fields[1].GetValue().GetParty()
					h.Amount = fields[2].GetValue().GetNumeric()
					
					// Token info is in field 3
					tokenFields := fields[3].GetValue().GetRecord().GetFields()
					if len(tokenFields) >= 3 {
						h.TokenName = tokenFields[0].GetValue().GetText()
						h.TokenSymbol = tokenFields[1].GetValue().GetText()
						h.Decimals = fmt.Sprintf("%d", tokenFields[2].GetValue().GetInt64())
					}
				}

				if event.CreatedAt != nil {
					h.CreatedAt = event.CreatedAt.AsTime()
				}

				holdings = append(holdings, h)
			}
		}
	}

	// Sort by created time descending
	sort.Slice(holdings, func(i, j int) bool {
		return holdings[i].CreatedAt.After(holdings[j].CreatedAt)
	})

	return holdings
}

func queryTokenConfigs(ctx context.Context, client lapiv2.StateServiceClient, party, packageID string, offset int64) []TokenConfig {
	filter := &lapiv2.EventFormat{
		FiltersByParty: map[string]*lapiv2.Filters{
			party: {
				Cumulative: []*lapiv2.CumulativeFilter{
					{
						IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
							TemplateFilter: &lapiv2.TemplateFilter{
								TemplateId: &lapiv2.Identifier{
									PackageId:  packageID,
									ModuleName: "Native.Token",
									EntityName: "NativeTokenConfig",
								},
							},
						},
					},
				},
			},
		},
	}

	stream, err := client.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: offset,
		EventFormat:    filter,
	})
	if err != nil {
		return nil
	}

	var configs []TokenConfig
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}

		if resp.GetActiveContract() != nil {
			event := resp.GetActiveContract().GetCreatedEvent()
			if event != nil {
				tc := TokenConfig{
					ContractID: event.ContractId,
				}

				fields := event.GetCreateArguments().GetFields()
				if len(fields) >= 4 {
					tc.Issuer = fields[0].GetValue().GetParty()
					
					tokenFields := fields[1].GetValue().GetRecord().GetFields()
					if len(tokenFields) >= 3 {
						tc.Name = tokenFields[0].GetValue().GetText()
						tc.Symbol = tokenFields[1].GetValue().GetText()
						tc.Decimals = fmt.Sprintf("%d", tokenFields[2].GetValue().GetInt64())
					}
					
					tc.ManagerCID = fields[2].GetValue().GetContractId()
				}

				if event.CreatedAt != nil {
					tc.CreatedAt = event.CreatedAt.AsTime()
				}

				configs = append(configs, tc)
			}
		}
	}

	return configs
}

func queryEvents(ctx context.Context, client lapiv2.StateServiceClient, party, packageID, eventType string, offset int64) []Event {
	filter := &lapiv2.EventFormat{
		FiltersByParty: map[string]*lapiv2.Filters{
			party: {
				Cumulative: []*lapiv2.CumulativeFilter{
					{
						IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
							TemplateFilter: &lapiv2.TemplateFilter{
								TemplateId: &lapiv2.Identifier{
									PackageId:  packageID,
									ModuleName: "Native.Events",
									EntityName: eventType,
								},
							},
						},
					},
				},
			},
		},
	}

	stream, err := client.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: offset,
		EventFormat:    filter,
	})
	if err != nil {
		return nil
	}

	var events []Event
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}

		if resp.GetActiveContract() != nil {
			created := resp.GetActiveContract().GetCreatedEvent()
			if created != nil {
				e := Event{
					Type:       eventType,
					ContractID: created.ContractId,
					Offset:     created.Offset,
				}

				fields := created.GetCreateArguments().GetFields()
				
				// Parse based on event type
				// MintEvent: issuer, recipient, amount, holdingCid, tokenSymbol, timestamp, auditObservers, userFingerprint
				// BurnEvent: issuer, owner, amount, holdingCid, tokenSymbol, timestamp, auditObservers, userFingerprint
				// TransferEvent: from, to, amount, holdingCid, tokenSymbol, timestamp, auditObservers
				switch eventType {
				case "MintEvent":
					if len(fields) >= 3 {
						e.Owner = fields[1].GetValue().GetParty()  // recipient
						e.Amount = fields[2].GetValue().GetNumeric()
					}
				case "BurnEvent":
					if len(fields) >= 3 {
						e.Owner = fields[1].GetValue().GetParty()  // owner
						e.Amount = fields[2].GetValue().GetNumeric()
					}
				case "TransferEvent":
					if len(fields) >= 3 {
						e.From = fields[0].GetValue().GetParty()
						e.To = fields[1].GetValue().GetParty()
						e.Amount = fields[2].GetValue().GetNumeric()
					}
				}

				if created.CreatedAt != nil {
					e.CreatedAt = created.CreatedAt.AsTime()
				}

				events = append(events, e)
			}
		}
	}

	// Sort by created time descending
	sort.Slice(events, func(i, j int) bool {
		return events[i].CreatedAt.After(events[j].CreatedAt)
	})

	return events
}
