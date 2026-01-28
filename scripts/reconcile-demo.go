//go:build ignore

// reconcile-demo.go - Reconcile DEMO token balances from Canton to database
//
// This script queries Canton for all MintEvent, BurnEvent, and TransferEvent
// contracts, calculates balances per fingerprint, and updates the database.
//
// Usage:
//   go run scripts/reconcile-demo.go -config config.local-devnet.yaml
//   go run scripts/reconcile-demo.go -config config.local-devnet.yaml -dry-run

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/config"
	_ "github.com/lib/pq"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

var configPath = flag.String("config", "config.local-devnet.yaml", "Path to config file")
var dryRun = flag.Bool("dry-run", false, "Show what would be updated without making changes")

const maxMessageSize = 52428800 // 50MB

func main() {
	flag.Parse()

	cfg, err := config.LoadAPIServer(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  DEMO Token Balance Reconciliation")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Printf("  Canton RPC: %s\n", cfg.Canton.RPCURL)
	fmt.Printf("  Database:   %s@%s/%s\n", cfg.Database.User, cfg.Database.Host, cfg.Database.Database)
	fmt.Printf("  Dry Run:    %v\n", *dryRun)
	fmt.Println()

	// Get OAuth token
	var token string
	if cfg.Canton.Auth.ClientID != "" {
		fmt.Println(">>> Fetching OAuth token...")
		token, err = fetchOAuthToken(&cfg.Canton.Auth)
		if err != nil {
			log.Fatalf("Failed to get OAuth token: %v", err)
		}
		fmt.Println("    Token obtained")
	}

	// Connect to Canton
	conn, stateClient, err := connectToCanton(ctx, cfg)
	if err != nil {
		log.Fatalf("Failed to connect to Canton: %v", err)
	}
	defer conn.Close()

	// Create auth context
	authCtx := ctx
	if token != "" {
		md := metadata.Pairs("authorization", "Bearer "+token)
		authCtx = metadata.NewOutgoingContext(ctx, md)
	}

	// Get ledger end
	ledgerEndResp, err := stateClient.GetLedgerEnd(authCtx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		log.Fatalf("Failed to get ledger end: %v", err)
	}
	fmt.Printf("  Ledger Offset: %d\n\n", ledgerEndResp.Offset)

	// Calculate balances from events
	balances := make(map[string]decimal.Decimal) // fingerprint -> balance

	// Query MintEvents
	fmt.Println(">>> Querying MintEvents...")
	mints, err := queryMintEvents(authCtx, stateClient, cfg, ledgerEndResp.Offset)
	if err != nil {
		log.Fatalf("Failed to query mint events: %v", err)
	}
	for _, m := range mints {
		balances[m.Fingerprint] = balances[m.Fingerprint].Add(m.Amount)
		fmt.Printf("    + %s DEMO to %s...\n", m.Amount.StringFixed(2), truncateFP(m.Fingerprint))
	}
	fmt.Printf("    Found %d mint events\n\n", len(mints))

	// Query BurnEvents
	fmt.Println(">>> Querying BurnEvents...")
	burns, err := queryBurnEvents(authCtx, stateClient, cfg, ledgerEndResp.Offset)
	if err != nil {
		log.Fatalf("Failed to query burn events: %v", err)
	}
	for _, b := range burns {
		balances[b.Fingerprint] = balances[b.Fingerprint].Sub(b.Amount)
		fmt.Printf("    - %s DEMO from %s...\n", b.Amount.StringFixed(2), truncateFP(b.Fingerprint))
	}
	fmt.Printf("    Found %d burn events\n\n", len(burns))

	// Query TransferEvents
	fmt.Println(">>> Querying TransferEvents...")
	transfers, err := queryTransferEvents(authCtx, stateClient, cfg, ledgerEndResp.Offset)
	if err != nil {
		log.Fatalf("Failed to query transfer events: %v", err)
	}
	for _, t := range transfers {
		balances[t.SenderFingerprint] = balances[t.SenderFingerprint].Sub(t.Amount)
		balances[t.RecipientFingerprint] = balances[t.RecipientFingerprint].Add(t.Amount)
		fmt.Printf("    %s DEMO: %s... -> %s...\n", t.Amount.StringFixed(2),
			truncateFP(t.SenderFingerprint), truncateFP(t.RecipientFingerprint))
	}
	fmt.Printf("    Found %d transfer events\n\n", len(transfers))

	// Print calculated balances
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Calculated Balances (from Canton events)")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	totalSupply := decimal.Zero
	for fp, bal := range balances {
		if bal.GreaterThan(decimal.Zero) {
			fmt.Printf("  %s: %s DEMO\n", fp, bal.StringFixed(2))
			totalSupply = totalSupply.Add(bal)
		}
	}
	fmt.Printf("\n  Total Supply: %s DEMO\n\n", totalSupply.StringFixed(2))

	if *dryRun {
		fmt.Println("[DRY RUN] Would update database with above balances")
		return
	}

	// Update database
	fmt.Println(">>> Updating database...")
	if err := updateDatabase(cfg, balances); err != nil {
		log.Fatalf("Failed to update database: %v", err)
	}

	fmt.Println()
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  RECONCILIATION COMPLETE")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
}

func truncateFP(fp string) string {
	if len(fp) > 16 {
		return fp[:16]
	}
	return fp
}

type MintEvent struct {
	Fingerprint string
	Amount      decimal.Decimal
}

type BurnEvent struct {
	Fingerprint string
	Amount      decimal.Decimal
}

type TransferEvent struct {
	SenderFingerprint    string
	RecipientFingerprint string
	Amount               decimal.Decimal
}

func connectToCanton(ctx context.Context, cfg *config.APIServerConfig) (*grpc.ClientConn, lapiv2.StateServiceClient, error) {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithDefaultCallOptions(
		grpc.MaxCallRecvMsgSize(maxMessageSize),
		grpc.MaxCallSendMsgSize(maxMessageSize),
	))

	if cfg.Canton.TLS.Enabled {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.DialContext(ctx, cfg.Canton.RPCURL, opts...)
	if err != nil {
		return nil, nil, err
	}

	return conn, lapiv2.NewStateServiceClient(conn), nil
}

func fetchOAuthToken(auth *config.AuthConfig) (string, error) {
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", auth.ClientID)
	data.Set("client_secret", auth.ClientSecret)
	data.Set("audience", auth.Audience)

	resp, err := http.Post(auth.TokenURL, "application/x-www-form-urlencoded", bytes.NewBufferString(data.Encode()))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	return result.AccessToken, nil
}

func queryMintEvents(ctx context.Context, client lapiv2.StateServiceClient, cfg *config.APIServerConfig, offset int64) ([]MintEvent, error) {
	resp, err := client.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: offset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				cfg.Canton.RelayerParty: {
					Cumulative: []*lapiv2.CumulativeFilter{
						{
							IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
								TemplateFilter: &lapiv2.TemplateFilter{
									TemplateId: &lapiv2.Identifier{
										PackageId:  cfg.Canton.NativeTokenPackageID,
										ModuleName: "Native.Events",
										EntityName: "MintEvent",
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
		return nil, err
	}

	var events []MintEvent
	for {
		msg, err := resp.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			templateId := contract.CreatedEvent.TemplateId
			if templateId.ModuleName != "Native.Events" || templateId.EntityName != "MintEvent" {
				continue
			}

			fields := contract.CreatedEvent.CreateArguments.Fields
			var fingerprint string
			var amount decimal.Decimal

			for _, f := range fields {
				switch f.Label {
				case "userFingerprint":
					fingerprint = f.Value.GetText()
				case "amount":
					amount, _ = decimal.NewFromString(f.Value.GetNumeric())
				}
			}

			if fingerprint != "" && !amount.IsZero() {
				events = append(events, MintEvent{
					Fingerprint: strings.ToLower(fingerprint),
					Amount:      amount,
				})
			}
		}
	}

	return events, nil
}

func queryBurnEvents(ctx context.Context, client lapiv2.StateServiceClient, cfg *config.APIServerConfig, offset int64) ([]BurnEvent, error) {
	resp, err := client.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: offset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				cfg.Canton.RelayerParty: {
					Cumulative: []*lapiv2.CumulativeFilter{
						{
							IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
								TemplateFilter: &lapiv2.TemplateFilter{
									TemplateId: &lapiv2.Identifier{
										PackageId:  cfg.Canton.NativeTokenPackageID,
										ModuleName: "Native.Events",
										EntityName: "BurnEvent",
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
		return nil, err
	}

	var events []BurnEvent
	for {
		msg, err := resp.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			templateId := contract.CreatedEvent.TemplateId
			if templateId.ModuleName != "Native.Events" || templateId.EntityName != "BurnEvent" {
				continue
			}

			fields := contract.CreatedEvent.CreateArguments.Fields
			var fingerprint string
			var amount decimal.Decimal

			for _, f := range fields {
				switch f.Label {
				case "userFingerprint":
					fingerprint = f.Value.GetText()
				case "amount":
					amount, _ = decimal.NewFromString(f.Value.GetNumeric())
				}
			}

			if fingerprint != "" && !amount.IsZero() {
				events = append(events, BurnEvent{
					Fingerprint: strings.ToLower(fingerprint),
					Amount:      amount,
				})
			}
		}
	}

	return events, nil
}

func queryTransferEvents(ctx context.Context, client lapiv2.StateServiceClient, cfg *config.APIServerConfig, offset int64) ([]TransferEvent, error) {
	resp, err := client.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: offset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				cfg.Canton.RelayerParty: {
					Cumulative: []*lapiv2.CumulativeFilter{
						{
							IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
								TemplateFilter: &lapiv2.TemplateFilter{
									TemplateId: &lapiv2.Identifier{
										PackageId:  cfg.Canton.NativeTokenPackageID,
										ModuleName: "Native.Events",
										EntityName: "TransferEvent",
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
		return nil, err
	}

	var events []TransferEvent
	for {
		msg, err := resp.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			templateId := contract.CreatedEvent.TemplateId
			if templateId.ModuleName != "Native.Events" || templateId.EntityName != "TransferEvent" {
				continue
			}

			fields := contract.CreatedEvent.CreateArguments.Fields
			var senderFp, recipientFp string
			var amount decimal.Decimal

			for _, f := range fields {
				switch f.Label {
				case "senderFingerprint":
					senderFp = f.Value.GetText()
				case "recipientFingerprint":
					recipientFp = f.Value.GetText()
				case "amount":
					amount, _ = decimal.NewFromString(f.Value.GetNumeric())
				}
			}

			if senderFp != "" && recipientFp != "" && !amount.IsZero() {
				events = append(events, TransferEvent{
					SenderFingerprint:    strings.ToLower(senderFp),
					RecipientFingerprint: strings.ToLower(recipientFp),
					Amount:               amount,
				})
			}
		}
	}

	return events, nil
}

func updateDatabase(cfg *config.APIServerConfig, balances map[string]decimal.Decimal) error {
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User,
		cfg.Database.Password, cfg.Database.Database, cfg.Database.SSLMode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return err
	}
	defer db.Close()

	for fingerprint, balance := range balances {
		if balance.LessThanOrEqual(decimal.Zero) {
			continue
		}

		result, err := db.Exec(`
			UPDATE users 
			SET demo_balance = $1, balance_updated_at = NOW()
			WHERE LOWER(fingerprint) = LOWER($2)
		`, balance.String(), fingerprint)
		if err != nil {
			fmt.Printf("    [WARN] Failed to update %s: %v\n", truncateFP(fingerprint), err)
			continue
		}

		rows, _ := result.RowsAffected()
		if rows > 0 {
			fmt.Printf("    Updated %s...: %s DEMO\n", truncateFP(fingerprint), balance.StringFixed(2))
		} else {
			fmt.Printf("    [SKIP] No user with fingerprint %s... (issuer or unregistered)\n", truncateFP(fingerprint))
		}
	}

	return nil
}
