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
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/golang-jwt/jwt/v5"
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

var (
	cwTokenMu     sync.Mutex
	cwCachedToken string
	cwTokenExpiry time.Time
	cwJwtSubject  string
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

	if !*cwDryRun && !*cwForce {
		fmt.Println("Error: Use -force to actually complete withdrawals, or use -dry-run=true to list them")
		os.Exit(1)
	}

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
	fmt.Printf("Party: %s\n", cfg.Canton.RelayerParty)

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

	ctx, err = cwGetAuthContext(ctx, &cfg.Canton.Auth)
	if err != nil {
		fmt.Printf("Failed to get auth context: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("JWT Subject: %s\n\n", cwJwtSubject)

	stateClient := lapiv2.NewStateServiceClient(conn)
	cmdClient := lapiv2.NewCommandServiceClient(conn)

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

	fmt.Println(">>> Getting domain ID...")
	domainID := cfg.Canton.DomainID
	if domainID == "" {
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
		domainID = domainResp.ConnectedSynchronizers[0].SynchronizerId
	}
	fmt.Printf("    Domain ID: %s\n\n", domainID)

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

func cwGetAuthContext(ctx context.Context, auth *config.AuthConfig) (context.Context, error) {
	if auth.ClientID == "" || auth.ClientSecret == "" || auth.Audience == "" || auth.TokenURL == "" {
		return ctx, fmt.Errorf("OAuth2 client credentials not configured")
	}

	token, err := cwGetOAuthToken(auth)
	if err != nil {
		return nil, err
	}

	md := metadata.Pairs("authorization", "Bearer "+token)
	return metadata.NewOutgoingContext(ctx, md), nil
}

func cwGetOAuthToken(auth *config.AuthConfig) (string, error) {
	cwTokenMu.Lock()
	defer cwTokenMu.Unlock()

	now := time.Now()
	if cwCachedToken != "" && now.Before(cwTokenExpiry) {
		return cwCachedToken, nil
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

	cwCachedToken = tokenResp.AccessToken
	cwTokenExpiry = expiry

	if subject, err := cwExtractJWTSubject(tokenResp.AccessToken); err == nil {
		cwJwtSubject = subject
	}

	fmt.Printf("OAuth2 token obtained (expires in %d seconds)\n", tokenResp.ExpiresIn)
	return tokenResp.AccessToken, nil
}

func cwExtractJWTSubject(tokenString string) (string, error) {
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
			templateId := contract.CreatedEvent.TemplateId
			if templateId.ModuleName == "Bridge.Contracts" && templateId.EntityName == "WithdrawalEvent" {
				w := PendingWithdrawal{
					ContractID: contract.CreatedEvent.ContractId,
				}

				fields := contract.CreatedEvent.CreateArguments.Fields
				for _, field := range fields {
					switch field.Label {
					case "userParty":
						if p, ok := field.Value.Sum.(*lapiv2.Value_Party); ok {
							w.UserParty = p.Party
						}
					case "evmDestination":
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
						if v, ok := field.Value.Sum.(*lapiv2.Value_Variant); ok {
							w.Status = v.Variant.Constructor
						}
					}
				}

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
			UserId:         cwJwtSubject,
			ActAs:          []string{issuer},
			Commands:       []*lapiv2.Command{cmd},
		},
	})

	return err
}
