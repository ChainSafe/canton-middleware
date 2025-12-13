//go:build ignore

// bridge-activity.go - Display recent Canton bridge activity in a demo-friendly format
//
// Usage:
//   go run scripts/bridge-activity.go -config .test-config.yaml              # Local Docker (after test-bridge.sh)
//   go run scripts/bridge-activity.go -config config.devnet.yaml             # 5North DevNet
//   go run scripts/bridge-activity.go -config config.mainnet.yaml            # Mainnet
//   go run scripts/bridge-activity.go -config .test-config.yaml -limit 10    # Limit results
//   go run scripts/bridge-activity.go -config .test-config.yaml -lookback 500 # Custom lookback
//
// For local testing:
//   1. Run: ./scripts/test-bridge.sh (starts services and creates .test-config.yaml)
//   2. Run: go run scripts/bridge-activity.go -config .test-config.yaml
//
// This script shows a formatted report of:
//   - Recent deposits (EVM → Canton)
//   - Recent withdrawals (Canton → EVM)
//   - Current CIP56 holdings

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
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

var (
	baConfigPath = flag.String("config", "config.yaml", "Path to config file")
	baLimit      = flag.Int("limit", 20, "Number of recent transactions to display")
	baLookback   = flag.Int64("lookback", 1000, "Offset lookback from ledger end")
	baDebug      = flag.Bool("debug", false, "Show debug info about all contracts found")
)

// Token caching
var (
	baTokenMu     sync.Mutex
	baCachedToken string
	baTokenExpiry time.Time
	baJwtSubject  string
)

// Data structures for bridge activity
type DepositInfo struct {
	Offset      int64
	Time        time.Time
	Amount      string
	Recipient   string
	EVMTx       string
	Fingerprint string
	Status      string
}

type WithdrawalInfo struct {
	Offset      int64
	Time        time.Time
	Amount      string
	EVMDest     string
	Status      string
	EVMTx       string
	RequestCID  string
	Fingerprint string
	HoldingCid  string
	RawStatus   string // Pending, Completed, or empty for Request
}

type HoldingInfo struct {
	ContractID string
	Owner      string
	Amount     string
	TokenID    string
}

func main() {
	flag.Parse()

	cfg, err := config.Load(*baConfigPath)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	partyID := cfg.Canton.RelayerParty
	if partyID == "" {
		fmt.Println("Error: canton.relayer_party is required in config")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Connect to Canton
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

	// Set up authentication
	ctx, err = baGetAuthContext(ctx, &cfg.Canton.Auth)
	if err != nil {
		fmt.Printf("Failed to get auth context: %v\n", err)
		os.Exit(1)
	}

	stateClient := lapiv2.NewStateServiceClient(conn)
	updateClient := lapiv2.NewUpdateServiceClient(conn)

	// Get ledger end
	ledgerEndResp, err := stateClient.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		fmt.Printf("Failed to get ledger end: %v\n", err)
		os.Exit(1)
	}

	if ledgerEndResp.Offset == 0 {
		fmt.Println("Ledger is empty - no activity to show.")
		os.Exit(0)
	}

	// Calculate start offset for lookback
	startOffset := ledgerEndResp.Offset - *baLookback
	if startOffset < 0 {
		startOffset = 0
	}

	// Print header
	printHeader(cfg.Canton.RPCURL, partyID, ledgerEndResp.Offset)

	// Query recent transactions for deposits and withdrawals
	deposits, withdrawals, err := queryBridgeActivity(ctx, updateClient, partyID, startOffset, ledgerEndResp.Offset, *baLimit, *baDebug)
	if err != nil {
		fmt.Printf("Failed to query bridge activity: %v\n", err)
		os.Exit(1)
	}

	// Query current holdings
	holdings, err := queryHoldings(ctx, stateClient, partyID, ledgerEndResp.Offset)
	if err != nil {
		fmt.Printf("Failed to query holdings: %v\n", err)
		os.Exit(1)
	}

	// Print deposits section
	printDeposits(deposits)

	// Print withdrawals section
	printWithdrawals(withdrawals)

	// Print holdings section
	printHoldings(holdings)

	// Print summary
	printSummary(len(deposits), len(withdrawals), len(holdings))
}

func printHeader(rpcURL, partyID string, ledgerOffset int64) {
	fmt.Println()
	fmt.Println("======================================================================")
	fmt.Println("CANTON BRIDGE ACTIVITY REPORT")
	fmt.Println("======================================================================")
	fmt.Printf("Network: %s\n", rpcURL)
	fmt.Printf("Party:   %s\n", truncateParty(partyID))
	fmt.Printf("Time:    %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Printf("Ledger:  Offset %d\n", ledgerOffset)
	if baJwtSubject != "" {
		fmt.Printf("JWT:     %s\n", baJwtSubject)
	}
	fmt.Println()
}

func printDeposits(deposits []DepositInfo) {
	fmt.Println("--- RECENT DEPOSITS (EVM → Canton) -----------------------------------")
	if len(deposits) == 0 {
		fmt.Println("No deposits found in the lookback range.")
	} else {
		for i, d := range deposits {
			fmt.Printf("[%d] %s\n", i+1, formatTime(d.Time))
			if d.Amount != "" {
				fmt.Printf("    Amount:      %s PROMPT\n", d.Amount)
			}
			if d.Recipient != "" {
				fmt.Printf("    Recipient:   %s\n", truncateParty(d.Recipient))
			}
			if d.EVMTx != "" {
				fmt.Printf("    EVM Tx:      %s\n", truncateHash(d.EVMTx))
			}
			if d.Fingerprint != "" {
				fmt.Printf("    Fingerprint: %s\n", truncateHash(d.Fingerprint))
			}
			fmt.Printf("    Status:      %s\n", d.Status)
			fmt.Printf("    Offset:      %d\n", d.Offset)
			fmt.Println()
		}
	}
	fmt.Println()
}

func printWithdrawals(withdrawals []WithdrawalInfo) {
	fmt.Println("--- RECENT WITHDRAWALS (Canton → EVM) --------------------------------")
	if len(withdrawals) == 0 {
		fmt.Println("No withdrawals found in the lookback range.")
	} else {
		for i, w := range withdrawals {
			fmt.Printf("[%d] %s\n", i+1, formatTime(w.Time))
			if w.Amount != "" {
				fmt.Printf("    Amount:   %s PROMPT\n", w.Amount)
			}
			if w.EVMDest != "" {
				fmt.Printf("    EVM Dest: %s\n", w.EVMDest)
			}
			fmt.Printf("    Status:   %s\n", w.Status)
			if w.EVMTx != "" {
				fmt.Printf("    EVM Tx:   %s\n", truncateHash(w.EVMTx))
			}
			if w.RequestCID != "" {
				fmt.Printf("    CID:      %s\n", truncateHash(w.RequestCID))
			}
			fmt.Printf("    Offset:   %d\n", w.Offset)
			fmt.Println()
		}
	}
	fmt.Println()
}

func printHoldings(holdings []HoldingInfo) {
	fmt.Println("--- CURRENT HOLDINGS -------------------------------------------------")
	if len(holdings) == 0 {
		fmt.Println("No CIP56Holding contracts found.")
	} else {
		for i, h := range holdings {
			fmt.Printf("[%d] Owner: %s\n", i+1, truncateParty(h.Owner))
			fmt.Printf("    Balance:  %s PROMPT\n", h.Amount)
			fmt.Printf("    Token ID: %s\n", h.TokenID)
			fmt.Printf("    CID:      %s\n", truncateHash(h.ContractID))
			fmt.Println()
		}
	}
	fmt.Println()
}

func printSummary(depositCount, withdrawalCount, holdingCount int) {
	fmt.Println("======================================================================")
	parts := []string{}
	if depositCount > 0 {
		parts = append(parts, fmt.Sprintf("%d deposit(s)", depositCount))
	}
	if withdrawalCount > 0 {
		parts = append(parts, fmt.Sprintf("%d withdrawal(s)", withdrawalCount))
	}
	if holdingCount > 0 {
		parts = append(parts, fmt.Sprintf("%d holding(s)", holdingCount))
	}
	if len(parts) == 0 {
		fmt.Println("Summary: No bridge activity found")
	} else {
		fmt.Printf("Summary: %s\n", strings.Join(parts, ", "))
	}
	fmt.Println("======================================================================")
}

// queryBridgeActivity queries recent transactions for deposit and withdrawal events
func queryBridgeActivity(ctx context.Context, client lapiv2.UpdateServiceClient, party string, fromOffset, toOffset int64, limit int, debug bool) ([]DepositInfo, []WithdrawalInfo, error) {
	req := &lapiv2.GetUpdatesRequest{
		BeginExclusive: fromOffset,
		EndInclusive:   &toOffset,
		UpdateFormat: &lapiv2.UpdateFormat{
			IncludeTransactions: &lapiv2.TransactionFormat{
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
				TransactionShape: lapiv2.TransactionShape_TRANSACTION_SHAPE_ACS_DELTA,
			},
		},
	}

	stream, err := client.GetUpdates(ctx, req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start update stream: %w", err)
	}

	// Track deposits by fingerprint+evmTx to dedupe
	depositMap := make(map[string]*DepositInfo)
	// Track withdrawals by fingerprint+holdingCid to dedupe (keep latest status)
	withdrawalMap := make(map[string]*WithdrawalInfo)

	// Track contract types seen for debug
	contractTypes := make(map[string]int)

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			return nil, nil, fmt.Errorf("stream error: %w", err)
		}

		if tx := msg.GetTransaction(); tx != nil {
			var effectiveAt time.Time
			if tx.EffectiveAt != nil {
				effectiveAt = tx.EffectiveAt.AsTime()
			}

			for _, event := range tx.Events {
				if created := event.GetCreated(); created != nil {
					templateID := created.TemplateId
					if templateID == nil {
						continue
					}

					// Track for debug
					key := templateID.ModuleName + "." + templateID.EntityName
					contractTypes[key]++

					// Debug: show CIP56Holding creations
					if *baDebug && templateID.ModuleName == "CIP56.Token" && templateID.EntityName == "CIP56Holding" {
						amount := ""
						if created.CreateArguments != nil {
							for _, field := range created.CreateArguments.Fields {
								if field.Label == "amount" {
									amount = extractNumeric(field.Value)
								}
							}
						}
						fmt.Printf("    [HOLDING CREATED @ %d]: %s PROMPT, CID: %s\n",
							tx.Offset, amount, truncateHash(created.ContractId))
					}

					// Check for deposit-related contracts
					if isDepositContract(templateID) {
						deposit := parseDepositEvent(created, tx.Offset, effectiveAt)

						// Debug: show raw deposit before deduping
						if *baDebug {
							fmt.Printf("    [RAW DEPOSIT @ %d]: %s PROMPT, EVM: %s, Status: %s\n",
								tx.Offset, deposit.Amount, truncateHash(deposit.EVMTx), deposit.Status)
						}

						// Dedupe by fingerprint+evmTx
						dedupeKey := deposit.Fingerprint + ":" + deposit.EVMTx
						if dedupeKey == ":" {
							dedupeKey = fmt.Sprintf("offset:%d", tx.Offset)
						}
						// Keep the latest (highest offset) or most processed state
						if existing, ok := depositMap[dedupeKey]; ok {
							// Update if this is more processed (Receipt > Pending)
							if deposit.Status == "Completed → minted" || tx.Offset > existing.Offset {
								depositMap[dedupeKey] = &deposit
							}
						} else {
							depositMap[dedupeKey] = &deposit
						}
					}

					// Check for withdrawal-related contracts
					if isWithdrawalContract(templateID) {
						withdrawal := parseWithdrawalEvent(created, tx.Offset, effectiveAt)

						// For deduplication:
						// - WithdrawalRequest: use holdingCid (unique per withdrawal)
						// - WithdrawalEvent: group by offset proximity to request (within 10 offsets)
						var dedupeKey string
						if withdrawal.HoldingCid != "" {
							// This is a Request with holdingCid
							dedupeKey = "holding:" + withdrawal.HoldingCid
						} else {
							// This is an Event - find the nearest Request by offset
							// Events typically follow Request by 3-6 offsets
							foundRequest := false
							for key, existing := range withdrawalMap {
								if strings.HasPrefix(key, "holding:") &&
									withdrawal.Offset > existing.Offset &&
									withdrawal.Offset <= existing.Offset+10 {
									dedupeKey = key
									foundRequest = true
									break
								}
							}
							if !foundRequest {
								// No matching request found, use contract ID
								dedupeKey = "cid:" + withdrawal.RequestCID
							}
						}

						// Keep the most advanced status: Completed > Pending > Request
						if existing, ok := withdrawalMap[dedupeKey]; ok {
							newPriority := withdrawalStatusPriority(withdrawal.RawStatus)
							existingPriority := withdrawalStatusPriority(existing.RawStatus)
							if newPriority > existingPriority {
								// Keep holdingCid from the original request
								if existing.HoldingCid != "" && withdrawal.HoldingCid == "" {
									withdrawal.HoldingCid = existing.HoldingCid
								}
								withdrawalMap[dedupeKey] = &withdrawal
							}
						} else {
							withdrawalMap[dedupeKey] = &withdrawal
						}
					}
				}
			}
		}
	}

	// Convert maps to slices, respecting limit
	var deposits []DepositInfo
	for _, d := range depositMap {
		deposits = append(deposits, *d)
		if len(deposits) >= limit {
			break
		}
	}

	var withdrawals []WithdrawalInfo
	for _, w := range withdrawalMap {
		withdrawals = append(withdrawals, *w)
		if len(withdrawals) >= limit {
			break
		}
	}

	// Sort by offset (most recent first would be nice, but keeping insertion order for now)

	// Print debug info
	if debug && len(contractTypes) > 0 {
		fmt.Println("--- DEBUG: Contract types found in range ---")
		for k, v := range contractTypes {
			fmt.Printf("  %s: %d\n", k, v)
		}
		fmt.Printf("  (Deduped: %d deposits, %d withdrawals)\n", len(deposits), len(withdrawals))
		fmt.Println()
	}

	return deposits, withdrawals, nil
}

func withdrawalStatusPriority(status string) int {
	switch status {
	case "Completed":
		return 3
	case "Pending":
		return 2
	case "Request":
		return 1
	default:
		return 0
	}
}

func isDepositContract(templateID *lapiv2.Identifier) bool {
	module := templateID.ModuleName
	entity := templateID.EntityName
	// PendingDeposit is in Common.FingerprintAuth module
	return (module == "Common.FingerprintAuth" && entity == "PendingDeposit") ||
		entity == "DepositEvent"
}

func isWithdrawalContract(templateID *lapiv2.Identifier) bool {
	module := templateID.ModuleName
	entity := templateID.EntityName
	// WithdrawalRequest and WithdrawalEvent are in Bridge.Contracts module
	return module == "Bridge.Contracts" && (entity == "WithdrawalRequest" || entity == "WithdrawalEvent")
}

func parseDepositEvent(created *lapiv2.CreatedEvent, offset int64, effectiveAt time.Time) DepositInfo {
	deposit := DepositInfo{
		Offset: offset,
		Time:   effectiveAt,
		Status: created.TemplateId.EntityName,
	}

	if created.TemplateId.EntityName == "PendingDeposit" {
		deposit.Status = "Pending → awaiting processing"
	} else if created.TemplateId.EntityName == "DepositEvent" {
		deposit.Status = "Completed → minted"
	}

	if created.CreateArguments != nil {
		for _, field := range created.CreateArguments.Fields {
			switch field.Label {
			case "amount":
				deposit.Amount = extractNumeric(field.Value)
			case "recipient", "owner":
				deposit.Recipient = extractParty(field.Value)
			case "evmTxHash", "txHash":
				deposit.EVMTx = extractText(field.Value)
			case "fingerprint":
				deposit.Fingerprint = extractText(field.Value)
			}
		}
	}

	return deposit
}

func parseWithdrawalEvent(created *lapiv2.CreatedEvent, offset int64, effectiveAt time.Time) WithdrawalInfo {
	withdrawal := WithdrawalInfo{
		Offset:     offset,
		Time:       effectiveAt,
		RequestCID: created.ContractId,
	}

	entityName := created.TemplateId.EntityName
	if entityName == "WithdrawalRequest" {
		withdrawal.Status = "Requested → awaiting processing"
		withdrawal.RawStatus = "Request"
	} else if entityName == "WithdrawalEvent" {
		withdrawal.Status = "Ready → pending EVM release"
		withdrawal.RawStatus = "Pending" // will be updated below if Completed
	}

	if created.CreateArguments != nil {
		for _, field := range created.CreateArguments.Fields {
			switch field.Label {
			case "amount":
				withdrawal.Amount = extractNumeric(field.Value)
			case "evmDestination", "destination":
				withdrawal.EVMDest = extractText(field.Value)
			case "evmTxHash":
				withdrawal.EVMTx = extractText(field.Value)
			case "fingerprint":
				withdrawal.Fingerprint = extractText(field.Value)
			case "status":
				// Check if status is Completed
				if v, ok := field.Value.Sum.(*lapiv2.Value_Variant); ok {
					if v.Variant.Constructor == "Completed" {
						withdrawal.RawStatus = "Completed"
						withdrawal.Status = "Completed → EVM released"
					} else if v.Variant.Constructor == "Pending" {
						withdrawal.RawStatus = "Pending"
						withdrawal.Status = "Ready → pending EVM release"
					}
				}
			case "holdingCid":
				withdrawal.HoldingCid = extractContractId(field.Value)
			}
		}

		// Debug: dump all fields if in verbose mode
		if *baDebug {
			fmt.Printf("    [DEBUG %s fields @ offset %d]:\n", created.TemplateId.EntityName, offset)
			for _, field := range created.CreateArguments.Fields {
				fmt.Printf("      %s = %s\n", field.Label, extractAnyValue(field.Value))
			}
		}
	}

	return withdrawal
}

// queryHoldings queries current CIP56Holding contracts
func queryHoldings(ctx context.Context, client lapiv2.StateServiceClient, party string, offset int64) ([]HoldingInfo, error) {
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

	var holdings []HoldingInfo
	for {
		msg, err := resp.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			templateID := contract.CreatedEvent.TemplateId
			if templateID.ModuleName == "CIP56.Token" && templateID.EntityName == "CIP56Holding" {
				h := HoldingInfo{
					ContractID: contract.CreatedEvent.ContractId,
				}

				fields := contract.CreatedEvent.CreateArguments.Fields

				// Debug: show all field names
				if *baDebug {
					fmt.Printf("    [DEBUG CIP56Holding fields]: ")
					for _, field := range fields {
						fmt.Printf("%s, ", field.Label)
					}
					fmt.Println()
				}

				for _, field := range fields {
					switch field.Label {
					case "owner":
						h.Owner = extractParty(field.Value)
					case "amount":
						h.Amount = extractNumeric(field.Value)
					case "issuer":
						// If no tokenId found elsewhere, use issuer as identifier
						if h.TokenID == "" {
							h.TokenID = truncateParty(extractParty(field.Value))
						}
					case "meta":
						// meta is a record that may contain token metadata
						if tokenId := extractMetaTokenId(field.Value); tokenId != "" {
							h.TokenID = tokenId
						}
					}
				}
				holdings = append(holdings, h)
			}
		}
	}

	return holdings, nil
}

// Auth helpers
func baGetAuthContext(ctx context.Context, auth *config.AuthConfig) (context.Context, error) {
	// Try OAuth2 first
	if auth.ClientID != "" && auth.ClientSecret != "" && auth.Audience != "" && auth.TokenURL != "" {
		token, err := baGetOAuthToken(auth)
		if err != nil {
			return nil, err
		}
		md := metadata.Pairs("authorization", "Bearer "+token)
		return metadata.NewOutgoingContext(ctx, md), nil
	}

	// Fall back to token file
	if auth.TokenFile != "" {
		tokenBytes, err := os.ReadFile(auth.TokenFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read token file: %w", err)
		}
		authToken := strings.TrimSpace(string(tokenBytes))
		md := metadata.Pairs("authorization", "Bearer "+authToken)
		return metadata.NewOutgoingContext(ctx, md), nil
	}

	return ctx, nil
}

func baGetOAuthToken(auth *config.AuthConfig) (string, error) {
	baTokenMu.Lock()
	defer baTokenMu.Unlock()

	now := time.Now()
	if baCachedToken != "" && now.Before(baTokenExpiry) {
		return baCachedToken, nil
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

	baCachedToken = tokenResp.AccessToken
	baTokenExpiry = expiry

	if subject, err := baExtractJWTSubject(tokenResp.AccessToken); err == nil {
		baJwtSubject = subject
	}

	fmt.Printf("OAuth2 token obtained (expires in %d seconds)\n", tokenResp.ExpiresIn)
	return tokenResp.AccessToken, nil
}

func baExtractJWTSubject(tokenString string) (string, error) {
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

// Value extraction helpers
func extractNumeric(v *lapiv2.Value) string {
	if v == nil {
		return ""
	}
	if n, ok := v.Sum.(*lapiv2.Value_Numeric); ok {
		return n.Numeric
	}
	return ""
}

func extractText(v *lapiv2.Value) string {
	if v == nil {
		return ""
	}
	if t, ok := v.Sum.(*lapiv2.Value_Text); ok {
		return t.Text
	}
	return ""
}

func extractContractId(v *lapiv2.Value) string {
	if v == nil {
		return ""
	}
	if c, ok := v.Sum.(*lapiv2.Value_ContractId); ok {
		return c.ContractId
	}
	return ""
}

func extractParty(v *lapiv2.Value) string {
	if v == nil {
		return ""
	}
	if p, ok := v.Sum.(*lapiv2.Value_Party); ok {
		return p.Party
	}
	return ""
}

// extractMetaTokenId extracts token identifier from the meta record field.
func extractMetaTokenId(v *lapiv2.Value) string {
	if v == nil {
		return ""
	}
	// If it's a record, look for common token ID field names
	if rec, ok := v.Sum.(*lapiv2.Value_Record); ok {
		// Debug: show meta record fields
		if *baDebug {
			fmt.Printf("    [DEBUG meta record fields]: ")
			for _, field := range rec.Record.Fields {
				fmt.Printf("%s=%s, ", field.Label, extractAnyValue(field.Value))
			}
			fmt.Println()
		}
		for _, field := range rec.Record.Fields {
			switch field.Label {
			case "tokenId", "id", "instrumentId", "assetId", "symbol", "name":
				if text := extractText(field.Value); text != "" {
					return text
				}
			}
		}
	}
	// Fallback: try direct text extraction
	return extractText(v)
}

func extractAnyValue(v *lapiv2.Value) string {
	if v == nil {
		return "<nil>"
	}
	switch val := v.Sum.(type) {
	case *lapiv2.Value_Text:
		return val.Text
	case *lapiv2.Value_Int64:
		return fmt.Sprintf("%d", val.Int64)
	case *lapiv2.Value_Numeric:
		return val.Numeric
	case *lapiv2.Value_Bool:
		return fmt.Sprintf("%v", val.Bool)
	case *lapiv2.Value_Party:
		return truncateParty(val.Party)
	case *lapiv2.Value_ContractId:
		return truncateHash(val.ContractId)
	case *lapiv2.Value_Timestamp:
		return fmt.Sprintf("ts:%d", val.Timestamp)
	case *lapiv2.Value_Record:
		return "<record>"
	case *lapiv2.Value_List:
		return fmt.Sprintf("<list:%d>", len(val.List.Elements))
	case *lapiv2.Value_Optional:
		if val.Optional.Value == nil {
			return "None"
		}
		return "Some(" + extractAnyValue(val.Optional.Value) + ")"
	case *lapiv2.Value_Variant:
		return fmt.Sprintf("%s(...)", val.Variant.Constructor)
	case *lapiv2.Value_Enum:
		return val.Enum.Constructor
	default:
		return "<unknown>"
	}
}

// Formatting helpers
func truncateParty(s string) string {
	if len(s) <= 50 {
		return s
	}
	// Show prefix::1220... format
	if idx := strings.Index(s, "::"); idx > 0 && idx < len(s)-10 {
		prefix := s[:idx]
		suffix := s[idx+2:]
		if len(suffix) > 12 {
			return prefix + "::" + suffix[:12] + "..."
		}
	}
	return s[:47] + "..."
}

func truncateHash(s string) string {
	if len(s) <= 20 {
		return s
	}
	return s[:17] + "..."
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "Unknown time"
	}
	return t.Format("2006-01-02 15:04:05 UTC")
}
