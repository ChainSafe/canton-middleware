//go:build ignore

// test-usdcx-indexer.go — End-to-end test for USDCx cross-participant indexing.
//
// Verifies that the Canton indexer (running with FiltersForAnyParty wildcard stream)
// correctly indexes TokenTransferEvents for USDCx, whose issuer (USDCxIssuer) lives
// on participant2 while token holders live on participant1.
//
// This is the key scenario our wildcard stream change unlocks: previously the indexer
// only subscribed to events where BridgeIssuer (P1) was a stakeholder, which excluded
// USDCx events entirely. With FiltersForAnyParty, all TokenTransferEvents on the
// participant are visible regardless of who the issuer is.
//
// =============================================================================
// FLOW
// =============================================================================
//
//  Phase 1 — Bootstrap P2
//    Discovers USDCxIssuer + TokenConfig on P2, or runs the full bootstrap if
//    they don't exist yet.
//
//  Phase 2 — Allocate holders on P1
//    Allocates two external parties (Holder1, Holder2) on participant1 using
//    secp256k1 keypairs. These simulate end-users whose wallets live on P1.
//
//  Phase 3 — Mint USDCx from P2
//    Exercises IssuerMint on TokenConfig from P2, sending 100 USDCx to Holder1
//    and 50 USDCx to Holder2. The synchronizer delivers the resulting
//    CIP56Holding contracts to both P1 and P2.
//
//  Phase 4 — Poll indexer
//    Queries GET /indexer/v1/admin/parties/{partyID}/balances on the local
//    indexer (port 8082) until USDCx balances appear. Times out after 60s.
//
//  Phase 5 — Verify events
//    Queries GET /indexer/v1/admin/parties/{partyID}/events and confirms that
//    MINT events with the correct amounts were recorded in the indexer DB.
//
// =============================================================================
// Prerequisites
// =============================================================================
//
//   docker compose up  (Canton, indexer, mock-oauth2 all running)
//   Indexer must be built with FiltersForAnyParty (streaming.New without WithParty).
//
// Usage:
//
//   go run scripts/testing/test-usdcx-indexer.go
//
// Flags:
//
//   -p1              localhost:5011              Participant1 gRPC address
//   -p2              localhost:5021              Participant2 gRPC address
//   -indexer-url     http://localhost:8082       Indexer HTTP base URL
//   -mint1           100.0                       USDCx to mint to Holder1
//   -mint2           50.0                        USDCx to mint to Holder2
//   -timeout         120s                        Total test timeout
//   -poll-interval   3s                          Indexer poll interval
//   -token-url       http://localhost:8088/oauth/token
//   -client-id       local-test-client
//   -client-secret   local-test-secret
//   -cip56-package-id  c8c6fe7c...              CIP56 package ID

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	adminv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2/admin"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/ledger"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"
	"github.com/chainsafe/canton-middleware/pkg/keys"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
)

// ─── flags ───────────────────────────────────────────────────────────────────

var (
	p1Addr = flag.String("p1", "localhost:5011",
		"Participant1 gRPC address — holders live here")
	p1Audience = flag.String("p1-audience", "http://canton:5011",
		"JWT audience for participant1")
	p2Addr = flag.String("p2", "localhost:5021",
		"Participant2 gRPC address — USDCxIssuer lives here")
	p2Audience = flag.String("p2-audience", "http://canton:5021",
		"JWT audience for participant2")
	indexerURL = flag.String("indexer-url", "http://localhost:8082",
		"Indexer HTTP base URL")
	mint1Amount = flag.String("mint1", "100.0",
		"USDCx to mint to Holder1")
	mint2Amount = flag.String("mint2", "50.0",
		"USDCx to mint to Holder2")
	testTimeout  = flag.Duration("timeout", 120*time.Second, "Total test timeout")
	pollInterval = flag.Duration("poll-interval", 3*time.Second, "Indexer poll interval")
	tokenURL     = flag.String("token-url", "http://localhost:8088/oauth/token",
		"OAuth2 token endpoint")
	clientID     = flag.String("client-id", "local-test-client", "OAuth2 client ID")
	clientSecret = flag.String("client-secret", "local-test-secret", "OAuth2 client secret")
	cip56PkgID = flag.String("cip56-package-id",
		"c8c6fe7c34d96b88d6471769aae85063c8045783b2a226fd24f8c573603d17c2",
		"CIP56 DAML package ID")
	spliceTransferPkgID = flag.String("splice-transfer-package-id",
		"55ba4deb0ad4662c4168b39859738a0e91388d252286480c7331b3f71a517281",
		"Splice.Api.Token.TransferInstructionV1 package ID")
	darDir = flag.String("dar-dir", "contracts/canton-erc20/daml",
		"Root directory containing .dar files to upload to P2 before bootstrapping")
)

// ─── indexer response types ──────────────────────────────────────────────────

type indexerBalance struct {
	PartyID         string `json:"party_id"`
	InstrumentAdmin string `json:"instrument_admin"`
	InstrumentID    string `json:"instrument_id"`
	Amount          string `json:"amount"`
}

type indexerEvent struct {
	ContractID      string `json:"contract_id"`
	EventType       string `json:"event_type"`
	PartyID         string `json:"party_id"`
	InstrumentAdmin string `json:"instrument_admin"`
	InstrumentID    string `json:"instrument_id"`
	Amount          string `json:"amount"`
	LedgerOffset    int64  `json:"ledger_offset"`
	EffectiveAt     string `json:"effective_at"`
}

type balancePage struct {
	Items      []*indexerBalance `json:"items"`
	TotalCount int64             `json:"total_count"`
}

type eventPage struct {
	Items      []*indexerEvent `json:"items"`
	TotalCount int64           `json:"total_count"`
}

// ─── main ────────────────────────────────────────────────────────────────────

func main() {
	flag.Parse()

	sep := strings.Repeat("=", 70)
	fmt.Println(sep)
	fmt.Println("  USDCx Cross-Participant Indexer Test")
	fmt.Println(sep)

	ctx, cancel := context.WithTimeout(context.Background(), *testTimeout)
	defer cancel()

	// ── connect ───────────────────────────────────────────────────────────────

	fmt.Printf("\n>>> Connecting to participant1 (%s)\n", *p1Addr)
	p1, err := newLedgerClient(*p1Addr, *p1Audience)
	if err != nil {
		log.Fatalf("connect p1: %v", err)
	}
	defer p1.Close()

	fmt.Printf(">>> Connecting to participant2 (%s)\n", *p2Addr)
	p2, err := newLedgerClient(*p2Addr, *p2Audience)
	if err != nil {
		log.Fatalf("connect p2: %v", err)
	}
	defer p2.Close()

	// ── Phase 1: discover or bootstrap USDCx on P2 ───────────────────────────

	fmt.Printf("\n%s\n", sep)
	fmt.Println("  Phase 1: Bootstrap P2 — USDCxIssuer + TokenConfig")
	fmt.Println(sep)

	issuer, tokenConfigCID, err := discoverOrBootstrapUSDCx(ctx, p1, p2)
	if err != nil {
		log.Fatalf("bootstrap USDCx on P2: %v", err)
	}
	fmt.Printf("    USDCxIssuer:  %s\n", issuer)
	fmt.Printf("    TokenConfig:  %s\n", tokenConfigCID)

	// Detect synchronizer for Canton commands
	syncID, err := detectSyncID(ctx, p1)
	if err != nil {
		log.Fatalf("detect synchronizer: %v", err)
	}
	fmt.Printf("    Synchronizer: %s\n", syncID)

	// ── Phase 2: allocate external holder parties on P1 ──────────────────────

	fmt.Printf("\n%s\n", sep)
	fmt.Println("  Phase 2: Allocate Holders on P1")
	fmt.Println(sep)

	holder1, err := allocateExternalParty(ctx, p1, fmt.Sprintf("USDCxHolder1-%d", time.Now().Unix()), syncID)
	if err != nil {
		log.Fatalf("allocate Holder1 on P1: %v", err)
	}
	fmt.Printf("    Holder1:  %s\n", holder1.PartyID)

	holder2, err := allocateExternalParty(ctx, p1, fmt.Sprintf("USDCxHolder2-%d", time.Now().Unix()), syncID)
	if err != nil {
		log.Fatalf("allocate Holder2 on P1: %v", err)
	}
	fmt.Printf("    Holder2:  %s\n", holder2.PartyID)

	// ── Phase 3: mint USDCx from P2 ──────────────────────────────────────────

	fmt.Printf("\n%s\n", sep)
	fmt.Println("  Phase 3: Mint USDCx from P2")
	fmt.Println(sep)

	fmt.Printf("    Minting %s USDCx → Holder1...\n", *mint1Amount)
	holding1, err := mintUSDCx(ctx, p2, issuer, tokenConfigCID, holder1.PartyID, *mint1Amount, syncID)
	if err != nil {
		log.Fatalf("mint to Holder1: %v", err)
	}
	fmt.Printf("    CIP56Holding: %s\n", holding1)

	fmt.Printf("    Minting %s USDCx → Holder2...\n", *mint2Amount)
	holding2, err := mintUSDCx(ctx, p2, issuer, tokenConfigCID, holder2.PartyID, *mint2Amount, syncID)
	if err != nil {
		log.Fatalf("mint to Holder2: %v", err)
	}
	fmt.Printf("    CIP56Holding: %s\n", holding2)

	// ── Phase 4: poll indexer for balances ────────────────────────────────────

	fmt.Printf("\n%s\n", sep)
	fmt.Println("  Phase 4: Poll Indexer for USDCx Balances")
	fmt.Println("  (verifies FiltersForAnyParty wildcard stream is working)")
	fmt.Println(sep)
	fmt.Printf("    Indexer: %s\n", *indexerURL)

	fmt.Printf("\n    Waiting for Holder1 balance (expect %s USDCx)...\n", *mint1Amount)
	bal1, err := pollPartyBalance(ctx, holder1.PartyID, "USDCx", *pollInterval)
	if err != nil {
		log.Fatalf("indexer did not index Holder1's USDCx balance: %v\n\n"+
			"    This likely means the indexer is NOT using FiltersForAnyParty.\n"+
			"    Check streaming.New() in pkg/app/indexer/server.go — WithParty must be absent.", err)
	}
	fmt.Printf("    Holder1 balance: %s USDCx  [instrument_admin: %s]\n", bal1.Amount, shortID(bal1.InstrumentAdmin))

	fmt.Printf("\n    Waiting for Holder2 balance (expect %s USDCx)...\n", *mint2Amount)
	bal2, err := pollPartyBalance(ctx, holder2.PartyID, "USDCx", *pollInterval)
	if err != nil {
		log.Fatalf("indexer did not index Holder2's USDCx balance: %v", err)
	}
	fmt.Printf("    Holder2 balance: %s USDCx\n", bal2.Amount)

	// ── Phase 5: verify mint events ───────────────────────────────────────────

	fmt.Printf("\n%s\n", sep)
	fmt.Println("  Phase 5: Verify Mint Events in Indexer")
	fmt.Println(sep)

	events1, err := fetchPartyEvents(holder1.PartyID)
	if err != nil {
		log.Fatalf("fetch events for Holder1: %v", err)
	}
	printEvents("Holder1", events1)

	events2, err := fetchPartyEvents(holder2.PartyID)
	if err != nil {
		log.Fatalf("fetch events for Holder2: %v", err)
	}
	printEvents("Holder2", events2)

	// ── Phase 6: cross-participant transfer P2 → P1 ──────────────────────────

	fmt.Printf("\n%s\n", sep)
	fmt.Println("  Phase 6: Cross-Participant Transfer  P2 (issuer) → P1 (Holder1)")
	fmt.Println("  (sends a TRANSFER event the indexer should catch)")
	fmt.Println(sep)

	fmt.Println("    Minting 20 USDCx → USDCxIssuer (self-mint to create a P2 holding)...")
	issuerHoldingCID, err := mintUSDCx(ctx, p2, issuer, tokenConfigCID, issuer, "20.0", syncID)
	if err != nil {
		log.Fatalf("mint to issuer: %v", err)
	}
	fmt.Printf("    IssuerHolding (P2): %s\n", issuerHoldingCID)

	factoryCID, err := findContractID(ctx, p2, issuer, "CIP56.TransferFactory", "CIP56TransferFactory")
	if err != nil {
		log.Fatalf("find CIP56TransferFactory on P2: %v", err)
	}
	fmt.Printf("    TransferFactory:    %s\n", factoryCID)

	fmt.Printf("    Transferring 10 USDCx: USDCxIssuer (P2) → Holder1 (P1)...\n")
	if err := transferUSDCx(ctx, p2, issuer, factoryCID, issuerHoldingCID, holder1.PartyID, "10.0", syncID); err != nil {
		log.Fatalf("transfer USDCx P2→P1: %v", err)
	}
	fmt.Println("    Transfer submitted")
	fmt.Println()
	fmt.Println("    To verify the TRANSFER event in the indexer:")
	fmt.Printf("    curl %s/indexer/v1/admin/parties/%s/events | jq .\n", *indexerURL, holder1.PartyID)
	fmt.Println()
	fmt.Println("    Look for an event with event_type=TRANSFER, amount=10, instrument_id=USDCx.")

	// ── Summary ───────────────────────────────────────────────────────────────

	fmt.Printf("\n%s\n", sep)
	fmt.Println("  PASS — USDCx cross-participant indexing verified")
	fmt.Println(sep)
	fmt.Println()
	fmt.Printf("  USDCxIssuer (P2):   %s\n", issuer)
	fmt.Printf("  Holder1 (P1):       %s\n", holder1.PartyID)
	fmt.Printf("  Holder2 (P1):       %s\n", holder2.PartyID)
	fmt.Println()
	fmt.Printf("  Holder1 indexed balance: %s USDCx (minted) + 10 USDCx (transferred)\n", bal1.Amount)
	fmt.Printf("  Holder2 indexed balance: %s USDCx\n", bal2.Amount)
	fmt.Println()
	fmt.Println("  The indexer correctly received TokenTransferEvents for a token")
	fmt.Println("  whose issuer (USDCxIssuer) lives on a different Canton participant.")
	fmt.Println(sep)
}

// ─── discover or bootstrap USDCx on P2 ───────────────────────────────────────

func discoverOrBootstrapUSDCx(ctx context.Context, p1, p2 *ledger.Client) (issuer, tokenConfigCID string, err error) {
	fmt.Println("    Searching for USDCxIssuer party on P2...")
	authCtx := p2.AuthContext(ctx)

	partiesResp, err := p2.PartyAdmin().ListKnownParties(authCtx, &adminv2.ListKnownPartiesRequest{})
	if err != nil {
		return "", "", fmt.Errorf("list parties on P2: %w", err)
	}

	var candidates []string
	for _, pd := range partiesResp.PartyDetails {
		if strings.Contains(pd.Party, "USDCxIssuer") {
			candidates = append(candidates, pd.Party)
		}
	}

	if len(candidates) > 0 {
		issuer = candidates[len(candidates)-1]
		fmt.Printf("    Found existing USDCxIssuer: %s\n", shortID(issuer))

		// Look for TokenConfig under the issuer
		tokenConfigCID, err = findTokenConfig(ctx, p2, issuer)
		if err == nil && tokenConfigCID != "" {
			fmt.Printf("    Found existing TokenConfig: %s\n", tokenConfigCID)
			return issuer, tokenConfigCID, nil
		}
		fmt.Println("    TokenConfig not found — creating contracts on existing issuer...")
	} else {
		fmt.Println("    No USDCxIssuer found — running full P2 bootstrap...")
		hint := fmt.Sprintf("USDCxIssuer%d", time.Now().Unix())
		issuer, err = allocateParty(ctx, p2, hint)
		if err != nil {
			return "", "", fmt.Errorf("allocate USDCxIssuer: %w", err)
		}
		fmt.Printf("    Allocated USDCxIssuer: %s\n", shortID(issuer))
	}

	// Upload CIP56 DARs to P2 so it can validate contracts with those templates.
	// P1 already has them from the docker bootstrap; P2 needs them to process
	// transactions where it is an informee (e.g. USDCxIssuer is a signatory).
	fmt.Printf("    Uploading DARs to P2 (from %s)...\n", *darDir)
	if err := uploadDARs(ctx, p2, *darDir); err != nil {
		return "", "", fmt.Errorf("upload DARs to P2: %w", err)
	}

	// Auto-detect synchronizer from P1 (needed for contract creation)
	syncID, err := detectSyncID(ctx, p1)
	if err != nil {
		return "", "", fmt.Errorf("detect synchronizer for P2 bootstrap: %w", err)
	}

	// Create CIP56Manager
	fmt.Println("    Creating CIP56Manager...")
	managerCID, err := createManager(ctx, p2, issuer, syncID)
	if err != nil {
		return "", "", fmt.Errorf("create CIP56Manager: %w", err)
	}

	// Create TokenConfig
	fmt.Println("    Creating TokenConfig...")
	tokenConfigCID, err = createTokenConfig(ctx, p2, issuer, syncID, managerCID)
	if err != nil {
		return "", "", fmt.Errorf("create TokenConfig: %w", err)
	}

	// Create CIP56TransferFactory (needed for future transfers)
	fmt.Println("    Creating CIP56TransferFactory...")
	_, err = createTransferFactory(ctx, p2, issuer, syncID)
	if err != nil {
		// Non-fatal: factory may already exist from a previous run
		fmt.Printf("    [WARN] CIP56TransferFactory: %v (may already exist)\n", err)
	}

	return issuer, tokenConfigCID, nil
}

func findTokenConfig(ctx context.Context, p2 *ledger.Client, issuer string) (string, error) {
	authCtx := p2.AuthContext(ctx)
	offset, err := p2.GetLedgerEnd(authCtx)
	if err != nil {
		return "", err
	}
	events, err := p2.GetActiveContractsByTemplate(authCtx, offset, []string{issuer},
		cip56Identifier("CIP56.Config", "TokenConfig"))
	if err != nil {
		return "", err
	}
	for _, e := range events {
		if values.MetaSymbolFromRecord(e.GetCreateArguments()) == "USDCx" {
			return e.ContractId, nil
		}
	}
	return "", fmt.Errorf("not found")
}

// ─── indexer polling ──────────────────────────────────────────────────────────

// pollPartyBalance polls the indexer every pollInterval until a USDCx balance
// appears for the party, or ctx is canceled.
func pollPartyBalance(ctx context.Context, partyID, instrumentID string, interval time.Duration) (*indexerBalance, error) {
	url := fmt.Sprintf("%s/indexer/v1/admin/parties/%s/balances", *indexerURL, partyID)
	client := &http.Client{Timeout: 10 * time.Second}

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for %s balance for party %s: %w", instrumentID, shortID(partyID), ctx.Err())
		default:
		}

		resp, err := client.Get(url)
		if err == nil {
			var page balancePage
			if json.NewDecoder(resp.Body).Decode(&page) == nil {
				resp.Body.Close()
				for _, b := range page.Items {
					if b.InstrumentID == instrumentID && b.Amount != "0" {
						return b, nil
					}
				}
			} else {
				resp.Body.Close()
			}
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for %s balance for party %s: %w", instrumentID, shortID(partyID), ctx.Err())
		case <-time.After(interval):
		}
	}
}

// fetchPartyEvents queries the indexer for all events for a party.
func fetchPartyEvents(partyID string) ([]*indexerEvent, error) {
	url := fmt.Sprintf("%s/indexer/v1/admin/parties/%s/events", *indexerURL, partyID)
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var page eventPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, err
	}
	return page.Items, nil
}

func printEvents(label string, events []*indexerEvent) {
	fmt.Printf("    %s events (%d total):\n", label, len(events))
	for _, e := range events {
		fmt.Printf("      [%s] %s USDCx  offset=%d  contract=%s\n",
			e.EventType, e.Amount, e.LedgerOffset, shortCID(e.ContractID))
	}
	if len(events) == 0 {
		fmt.Printf("      (no events yet)\n")
	}
}

// ─── transfer helpers ─────────────────────────────────────────────────────────

// findContractID returns the contract ID of the first active contract matching
// the given CIP56 module/entity visible to party.
func findContractID(ctx context.Context, c *ledger.Client, party, module, entity string) (string, error) {
	authCtx := c.AuthContext(ctx)
	offset, err := c.GetLedgerEnd(authCtx)
	if err != nil {
		return "", err
	}
	events, err := c.GetActiveContractsByTemplate(authCtx, offset, []string{party},
		cip56Identifier(module, entity))
	if err != nil {
		return "", err
	}
	if len(events) == 0 {
		return "", fmt.Errorf("%s::%s not found for party %s", module, entity, shortID(party))
	}
	return events[0].ContractId, nil
}

// transferUSDCx exercises TransferFactory_Transfer on P2, moving USDCx from
// the issuer (an internal P2 party) to a recipient on P1. No Interactive
// Submission is needed because the issuer is not an external secp256k1 party.
func transferUSDCx(ctx context.Context, c *ledger.Client, issuer, factoryCID, inputHoldingCID, recipient, amount, syncID string) error {
	authCtx := c.AuthContext(ctx)
	sub, _ := c.JWTSubject(authCtx)
	if sub == "" {
		sub = "test-user"
	}
	now := time.Now().UTC()
	_, err := c.Command().SubmitAndWaitForTransaction(authCtx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: syncID,
			CommandId:      fmt.Sprintf("usdcx-xfer-%d", time.Now().UnixNano()),
			UserId:         sub,
			ActAs:          []string{issuer},
			Commands: []*lapiv2.Command{{
				Command: &lapiv2.Command_Exercise{Exercise: &lapiv2.ExerciseCommand{
					TemplateId: &lapiv2.Identifier{
						PackageId:  *spliceTransferPkgID,
						ModuleName: "Splice.Api.Token.TransferInstructionV1",
						EntityName: "TransferFactory",
					},
					ContractId: factoryCID,
					Choice:     "TransferFactory_Transfer",
					ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: &lapiv2.Record{
						Fields: []*lapiv2.RecordField{
							{Label: "expectedAdmin", Value: values.PartyValue(issuer)},
							{Label: "transfer", Value: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: &lapiv2.Record{
								Fields: []*lapiv2.RecordField{
									{Label: "sender", Value: values.PartyValue(issuer)},
									{Label: "receiver", Value: values.PartyValue(recipient)},
									{Label: "amount", Value: values.NumericValue(amount)},
									{Label: "instrumentId", Value: values.EncodeInstrumentId(issuer, "USDCx")},
									{Label: "requestedAt", Value: values.TimestampValue(now)},
									{Label: "executeBefore", Value: values.TimestampValue(now.Add(time.Hour))},
									{Label: "inputHoldingCids", Value: values.ListValue([]*lapiv2.Value{values.ContractIDValue(inputHoldingCID)})},
									{Label: "meta", Value: values.EmptyMetadata()},
								},
							}}}},
							{Label: "extraArgs", Value: values.EncodeExtraArgs(nil)},
						},
					}}},
				}},
			}},
		},
	})
	return err
}

// ─── Canton helpers ───────────────────────────────────────────────────────────

// uploadDARs walks darDir and uploads every *.dar file to the participant.
// Already-uploaded packages are logged and skipped (not fatal).
func uploadDARs(ctx context.Context, c *ledger.Client, darDir string) error {
	pkgSvc := adminv2.NewPackageManagementServiceClient(c.Conn())
	authCtx := c.AuthContext(ctx)

	var dars []string
	if err := filepath.WalkDir(darDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".dar") {
			dars = append(dars, path)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("walk DAR dir %s: %w", darDir, err)
	}
	if len(dars) == 0 {
		return fmt.Errorf("no .dar files found under %s", darDir)
	}

	for _, p := range dars {
		data, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		_, err = pkgSvc.UploadDarFile(authCtx, &adminv2.UploadDarFileRequest{
			DarFile:       data,
			VettingChange: adminv2.UploadDarFileRequest_VETTING_CHANGE_VET_ALL_PACKAGES,
		})
		if err != nil {
			fmt.Printf("    upload %s: %v (may already exist)\n", filepath.Base(p), err)
		} else {
			fmt.Printf("    uploaded %s\n", filepath.Base(p))
		}
	}
	return nil
}

func newLedgerClient(addr, audience string) (*ledger.Client, error) {
	return ledger.New(&ledger.Config{
		RPCURL:         addr,
		MaxMessageSize: 52428800,
		TLS:            &ledger.TLSConfig{Enabled: false},
		Auth: &ledger.AuthConfig{
			ClientID:     *clientID,
			ClientSecret: *clientSecret,
			Audience:     audience,
			TokenURL:     *tokenURL,
			ExpiryLeeway: 60 * time.Second,
		},
	})
}

// detectSyncID submits a probe transaction on P1 and reads the synchronizer ID.
func detectSyncID(ctx context.Context, c *ledger.Client) (string, error) {
	authCtx := c.AuthContext(ctx)
	sub, _ := c.JWTSubject(authCtx)
	if sub == "" {
		sub = "test-user"
	}
	allocResp, err := c.PartyAdmin().AllocateParty(authCtx, &adminv2.AllocatePartyRequest{
		PartyIdHint: fmt.Sprintf("SyncProbe%d", time.Now().UnixNano()),
	})
	if err != nil {
		return "", fmt.Errorf("allocate probe party: %w", err)
	}
	probe := allocResp.PartyDetails.Party
	resp, err := c.Command().SubmitAndWaitForTransaction(authCtx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			CommandId: fmt.Sprintf("probe-%d", time.Now().UnixNano()),
			UserId:    sub,
			ActAs:     []string{probe},
			Commands: []*lapiv2.Command{{
				Command: &lapiv2.Command_Create{Create: &lapiv2.CreateCommand{
					TemplateId: cip56Identifier("CIP56.Token", "CIP56Manager"),
					CreateArguments: &lapiv2.Record{Fields: []*lapiv2.RecordField{
						{Label: "issuer", Value: values.PartyValue(probe)},
						{Label: "instrumentId", Value: values.EncodeInstrumentId(probe, "PROBE")},
						{Label: "meta", Value: values.EmptyMetadata()},
					}},
				}},
			}},
		},
	})
	if err != nil {
		return "", fmt.Errorf("probe command: %w", err)
	}
	if resp.Transaction == nil || resp.Transaction.SynchronizerId == "" {
		return "", fmt.Errorf("no synchronizer ID in probe response")
	}
	return resp.Transaction.SynchronizerId, nil
}

func allocateParty(ctx context.Context, c *ledger.Client, hint string) (string, error) {
	authCtx := c.AuthContext(ctx)
	resp, err := c.PartyAdmin().AllocateParty(authCtx, &adminv2.AllocatePartyRequest{
		PartyIdHint: hint,
	})
	if err != nil {
		return "", fmt.Errorf("allocate party %q: %w", hint, err)
	}
	return resp.PartyDetails.Party, nil
}

// allocateExternalParty generates a secp256k1 keypair and registers the party on P1.
func allocateExternalParty(ctx context.Context, c *ledger.Client, hint, syncID string) (*externalParty, error) {
	kp, err := keys.GenerateCantonKeyPair()
	if err != nil {
		return nil, fmt.Errorf("generate keypair: %w", err)
	}
	spki, err := kp.SPKIPublicKey()
	if err != nil {
		return nil, fmt.Errorf("encode SPKI: %w", err)
	}
	authCtx := c.AuthContext(ctx)
	pubKey := &lapiv2.SigningPublicKey{
		Format:  lapiv2.CryptoKeyFormat_CRYPTO_KEY_FORMAT_DER_X509_SUBJECT_PUBLIC_KEY_INFO,
		KeyData: spki,
		KeySpec: lapiv2.SigningKeySpec_SIGNING_KEY_SPEC_EC_SECP256K1,
	}
	topoResp, err := c.PartyAdmin().GenerateExternalPartyTopology(authCtx, &adminv2.GenerateExternalPartyTopologyRequest{
		Synchronizer: syncID,
		PartyHint:    hint,
		PublicKey:    pubKey,
	})
	if err != nil {
		return nil, fmt.Errorf("generate topology: %w", err)
	}
	derSig, err := kp.SignDER(topoResp.MultiHash)
	if err != nil {
		return nil, fmt.Errorf("sign topology: %w", err)
	}
	signedTxs := make([]*adminv2.AllocateExternalPartyRequest_SignedTransaction, len(topoResp.TopologyTransactions))
	for i, tx := range topoResp.TopologyTransactions {
		signedTxs[i] = &adminv2.AllocateExternalPartyRequest_SignedTransaction{Transaction: tx}
	}
	allocResp, err := c.PartyAdmin().AllocateExternalParty(authCtx, &adminv2.AllocateExternalPartyRequest{
		Synchronizer:           syncID,
		OnboardingTransactions: signedTxs,
		MultiHashSignatures: []*lapiv2.Signature{{
			Format:               lapiv2.SignatureFormat_SIGNATURE_FORMAT_DER,
			Signature:            derSig,
			SignedBy:             topoResp.PublicKeyFingerprint,
			SigningAlgorithmSpec: lapiv2.SigningAlgorithmSpec_SIGNING_ALGORITHM_SPEC_EC_DSA_SHA_256,
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("allocate external party: %w", err)
	}
	return &externalParty{PartyID: allocResp.PartyId, KeyPair: kp}, nil
}

type externalParty struct {
	PartyID string
	KeyPair *keys.CantonKeyPair
}

// createManager creates a CIP56Manager for USDCx on P2.
func createManager(ctx context.Context, c *ledger.Client, issuer, syncID string) (string, error) {
	authCtx := c.AuthContext(ctx)
	sub, _ := c.JWTSubject(authCtx)
	if sub == "" {
		sub = "test-user"
	}
	resp, err := c.Command().SubmitAndWaitForTransaction(authCtx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: syncID,
			CommandId:      fmt.Sprintf("create-usdcx-mgr-%d", time.Now().UnixNano()),
			UserId:         sub,
			ActAs:          []string{issuer},
			Commands: []*lapiv2.Command{{
				Command: &lapiv2.Command_Create{Create: &lapiv2.CreateCommand{
					TemplateId: cip56Identifier("CIP56.Token", "CIP56Manager"),
					CreateArguments: &lapiv2.Record{Fields: []*lapiv2.RecordField{
						{Label: "issuer", Value: values.PartyValue(issuer)},
						{Label: "instrumentId", Value: values.EncodeInstrumentId(issuer, "USDCx")},
						{Label: "meta", Value: values.EncodeMetadata(map[string]string{
							"splice.chainsafe.io/name":     "USD Coin",
							"splice.chainsafe.io/symbol":   "USDCx",
							"splice.chainsafe.io/decimals": "6",
						})},
					}},
				}},
			}},
		},
	})
	if err != nil {
		return "", err
	}
	return findContractInTx(resp.Transaction, "CIP56Manager")
}

// createTokenConfig creates a TokenConfig for USDCx on P2.
func createTokenConfig(ctx context.Context, c *ledger.Client, issuer, syncID, managerCID string) (string, error) {
	authCtx := c.AuthContext(ctx)
	sub, _ := c.JWTSubject(authCtx)
	if sub == "" {
		sub = "test-user"
	}
	resp, err := c.Command().SubmitAndWaitForTransaction(authCtx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: syncID,
			CommandId:      fmt.Sprintf("create-usdcx-cfg-%d", time.Now().UnixNano()),
			UserId:         sub,
			ActAs:          []string{issuer},
			Commands: []*lapiv2.Command{{
				Command: &lapiv2.Command_Create{Create: &lapiv2.CreateCommand{
					TemplateId: cip56Identifier("CIP56.Config", "TokenConfig"),
					CreateArguments: &lapiv2.Record{Fields: []*lapiv2.RecordField{
						{Label: "issuer", Value: values.PartyValue(issuer)},
						{Label: "tokenManagerCid", Value: values.ContractIDValue(managerCID)},
						{Label: "instrumentId", Value: values.EncodeInstrumentId(issuer, "USDCx")},
						{Label: "meta", Value: values.EncodeMetadata(map[string]string{
							"splice.chainsafe.io/name":     "USD Coin",
							"splice.chainsafe.io/symbol":   "USDCx",
							"splice.chainsafe.io/decimals": "6",
						})},
						{Label: "auditObservers", Value: values.ListValue(nil)},
					}},
				}},
			}},
		},
	})
	if err != nil {
		return "", err
	}
	return findContractInTx(resp.Transaction, "TokenConfig")
}

// createTransferFactory creates a CIP56TransferFactory for USDCx on P2.
func createTransferFactory(ctx context.Context, c *ledger.Client, admin, syncID string) (string, error) {
	authCtx := c.AuthContext(ctx)
	sub, _ := c.JWTSubject(authCtx)
	if sub == "" {
		sub = "test-user"
	}
	resp, err := c.Command().SubmitAndWaitForTransaction(authCtx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: syncID,
			CommandId:      fmt.Sprintf("create-usdcx-factory-%d", time.Now().UnixNano()),
			UserId:         sub,
			ActAs:          []string{admin},
			Commands: []*lapiv2.Command{{
				Command: &lapiv2.Command_Create{Create: &lapiv2.CreateCommand{
					TemplateId: cip56Identifier("CIP56.TransferFactory", "CIP56TransferFactory"),
					CreateArguments: &lapiv2.Record{Fields: []*lapiv2.RecordField{
						{Label: "admin", Value: values.PartyValue(admin)},
						{Label: "auditObservers", Value: values.ListValue(nil)},
					}},
				}},
			}},
		},
	})
	if err != nil {
		return "", err
	}
	return findContractInTx(resp.Transaction, "CIP56TransferFactory")
}

// mintUSDCx exercises IssuerMint on TokenConfig from P2.
func mintUSDCx(ctx context.Context, p2 *ledger.Client, issuer, tokenConfigCID, recipient, amount, syncID string) (string, error) {
	authCtx := p2.AuthContext(ctx)
	sub, _ := p2.JWTSubject(authCtx)
	if sub == "" {
		sub = "test-user"
	}
	resp, err := p2.Command().SubmitAndWaitForTransaction(authCtx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: syncID,
			CommandId:      fmt.Sprintf("mint-usdcx-%d", time.Now().UnixNano()),
			UserId:         sub,
			ActAs:          []string{issuer},
			Commands: []*lapiv2.Command{{
				Command: &lapiv2.Command_Exercise{Exercise: &lapiv2.ExerciseCommand{
					TemplateId: cip56Identifier("CIP56.Config", "TokenConfig"),
					ContractId: tokenConfigCID,
					Choice:     "IssuerMint",
					ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: &lapiv2.Record{
						Fields: []*lapiv2.RecordField{
							{Label: "recipient", Value: values.PartyValue(recipient)},
							{Label: "amount", Value: values.NumericValue(amount)},
							{Label: "eventTime", Value: values.TimestampValue(time.Now())},
							{Label: "eventMeta", Value: values.None()},
						},
					}}},
				}},
			}},
		},
	})
	if err != nil {
		return "", err
	}
	for _, event := range resp.Transaction.Events {
		if c := event.GetCreated(); c != nil && c.TemplateId != nil && c.TemplateId.EntityName == "CIP56Holding" {
			return c.ContractId, nil
		}
	}
	return "", fmt.Errorf("CIP56Holding not found in mint transaction")
}

// ─── proto helpers ────────────────────────────────────────────────────────────

func cip56Identifier(module, entity string) *lapiv2.Identifier {
	return &lapiv2.Identifier{
		PackageId:  *cip56PkgID,
		ModuleName: module,
		EntityName: entity,
	}
}

func findContractInTx(tx *lapiv2.Transaction, entityName string) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("nil transaction")
	}
	for _, event := range tx.Events {
		if created := event.GetCreated(); created != nil {
			if created.TemplateId != nil && created.TemplateId.EntityName == entityName {
				return created.ContractId, nil
			}
		}
	}
	return "", fmt.Errorf("%s not found in transaction events", entityName)
}

// ─── display helpers ──────────────────────────────────────────────────────────

func shortID(id string) string {
	if len(id) > 48 {
		return id[:48] + "…"
	}
	return id
}

func shortCID(cid string) string {
	if len(cid) > 24 {
		return cid[:24] + "…"
	}
	return cid
}
