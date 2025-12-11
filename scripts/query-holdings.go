//go:build ignore

// query-holdings.go - Query CIP56Holding contracts for a user on Canton
//
// Usage:
//   go run scripts/query-holdings.go -config config.yaml \
//     -party "BridgeIssuer::122047584945db4991c2954b1e8e673623a43ec80869abf0f8e7531a435ae797ac6e"
//
// If -party is not specified, uses the relayer_party from config.

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

var (
	qhConfigPath = flag.String("config", "config.yaml", "Path to config file")
	qhPartyID    = flag.String("party", "", "Canton Party ID to query holdings for (uses config relayer_party if not specified)")
)

var (
	qhTokenMu     sync.Mutex
	qhCachedToken string
	qhTokenExpiry time.Time
	qhJwtSubject  string
)

type Holding struct {
	ContractID string
	Owner      string
	Amount     string
	TokenID    string
}

func main() {
	flag.Parse()

	cfg, err := config.Load(*qhConfigPath)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	partyID := *qhPartyID
	if partyID == "" {
		partyID = cfg.Canton.RelayerParty
	}

	if partyID == "" {
		fmt.Println("Error: -party is required (or set canton.relayer_party in config)")
		fmt.Println("Usage: go run scripts/query-holdings.go -config config.yaml -party 'PartyID'")
		os.Exit(1)
	}

	fmt.Println("======================================================================")
	fmt.Println("QUERY HOLDINGS - List CIP56Holding contracts on Canton")
	fmt.Println("======================================================================")
	fmt.Printf("Party: %s\n", partyID)

	ctx := context.Background()

	var opts []grpc.DialOption
	if cfg.Canton.TLS.Enabled {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{"h2"},
		}
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

	ctx, err = qhGetAuthContext(ctx, &cfg.Canton.Auth)
	if err != nil {
		fmt.Printf("Failed to get auth context: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("JWT Subject: %s\n\n", qhJwtSubject)

	stateClient := lapiv2.NewStateServiceClient(conn)

	ledgerEndResp, err := stateClient.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		fmt.Printf("Failed to get ledger end: %v\n", err)
		os.Exit(1)
	}
	if ledgerEndResp.Offset == 0 {
		fmt.Println("Error: Ledger is empty.")
		os.Exit(1)
	}

	holdings, err := qhQueryHoldings(ctx, stateClient, partyID, ledgerEndResp.Offset)
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
	fmt.Println("    -evm-destination \"0x...\"")
}

func qhGetAuthContext(ctx context.Context, auth *config.AuthConfig) (context.Context, error) {
	if auth.ClientID == "" || auth.ClientSecret == "" || auth.Audience == "" || auth.TokenURL == "" {
		return ctx, fmt.Errorf("OAuth2 client credentials not configured")
	}

	token, err := qhGetOAuthToken(auth)
	if err != nil {
		return nil, err
	}

	md := metadata.Pairs("authorization", "Bearer "+token)
	return metadata.NewOutgoingContext(ctx, md), nil
}

func qhGetOAuthToken(auth *config.AuthConfig) (string, error) {
	qhTokenMu.Lock()
	defer qhTokenMu.Unlock()

	now := time.Now()
	if qhCachedToken != "" && now.Before(qhTokenExpiry) {
		return qhCachedToken, nil
	}

	payload := map[string]string{
		"client_id":     auth.ClientID,
		"client_secret": auth.ClientSecret,
		"audience":      auth.Audience,
		"grant_type":    "client_credentials",
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal OAuth token request: %w", err)
	}

	fmt.Printf("Fetching OAuth2 access token from %s...\n", auth.TokenURL)

	req, err := http.NewRequest("POST", auth.TokenURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create OAuth token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call OAuth token endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("OAuth token endpoint returned %d: %s", resp.StatusCode, string(b))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode OAuth token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("OAuth token response missing access_token")
	}

	expiry := now.Add(5 * time.Minute)
	if tokenResp.ExpiresIn > 0 {
		leeway := 60
		if tokenResp.ExpiresIn <= leeway {
			leeway = tokenResp.ExpiresIn / 2
		}
		expiry = now.Add(time.Duration(tokenResp.ExpiresIn-leeway) * time.Second)
	}

	qhCachedToken = tokenResp.AccessToken
	qhTokenExpiry = expiry

	if subject, err := qhExtractJWTSubject(tokenResp.AccessToken); err == nil {
		qhJwtSubject = subject
	}

	fmt.Printf("OAuth2 token obtained (expires in %d seconds)\n", tokenResp.ExpiresIn)
	return tokenResp.AccessToken, nil
}

func qhExtractJWTSubject(tokenString string) (string, error) {
	token, _, err := jwt.NewParser().ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return "", fmt.Errorf("failed to parse JWT: %w", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("invalid JWT claims")
	}
	sub, ok := claims["sub"].(string)
	if !ok {
		return "", fmt.Errorf("JWT missing 'sub' claim")
	}
	return sub, nil
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
			templateId := contract.CreatedEvent.TemplateId
			if templateId.ModuleName == "CIP56.Token" && templateId.EntityName == "CIP56Holding" {
				h := Holding{
					ContractID: contract.CreatedEvent.ContractId,
				}

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
