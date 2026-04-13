//go:build ignore

// test-cip56-multi-participant.go — CIP56 token balance test across multiple
// Canton participant nodes and multiple issuers, using external parties for holders.
//
// =============================================================================
// FLOW
// =============================================================================
//
// 1. Connect & Upload DARs
//    Two gRPC clients are created, one per Canton participant.
//    P2 does not have the CIP56 packages from the docker bootstrap (only P1 does).
//    All DAR files are uploaded to P2 so it can validate transactions where it
//    hosts an informee party.
//
// 2. Auto-detect Synchronizer ID
//    A throwaway party is allocated on P1 and a CIP56Manager.create command is
//    submitted. The response carries Transaction.SynchronizerId, which is reused
//    for all subsequent commands — no hard-coded domain ID needed.
//
// 3. Party Allocation
//
//    Participant 1 (P1)                         Participant 2 (P2)
//    ──────────────────────────────────────     ──────────────────────────────
//    IssuerA   (internal)
//    IssuerB   (internal)
//    HolderP1A (external, P1 primary / P2 obs)  ← also observing on P2
//    HolderP1B (external, P1 only)
//                                               HolderP2A (external, P2 only)
//                                               HolderP2B (external, P2 only)
//
//    Internal parties (issuers) are allocated with a standard AllocateParty call.
//    They act as signatories and submit commands through P1's Command Service.
//
//    External parties (holders) each generate their own secp256k1 keypair.
//    Allocation proves key ownership:
//
//      GenerateExternalPartyTopology(hint, publicKey, observingParticipantUids)
//          → Canton returns: TopologyTransactions + MultiHash
//      keypair.SignDER(MultiHash)      ← party proves it owns the private key
//      AllocateExternalParty(TopologyTransactions, DERSignature)
//          → party is registered on the synchronizer under this participant
//
//    HolderP1A is allocated on P1 with P2 listed as an observing participant.
//    This means the synchronizer will also deliver HolderP1A's contracts to P2,
//    enabling balance queries from P2 without a separate migration step.
//
// 4. Token Setup
//    Two independent tokens, each with a dedicated issuer:
//
//      IssuerA  →  CIP56Manager(ALPHA)  →  TokenConfig(ALPHA)
//      IssuerB  →  CIP56Manager(BETA)   →  TokenConfig(BETA)
//
//    CIP56Manager is the minting authority (signatory issuer).
//    TokenConfig wraps it and exposes the IssuerMint choice.
//
// 5. Minting
//    All mints are submitted through P1 (both issuers live there):
//
//      IssuerA  IssuerMint  →  HolderP1A  100 ALPHA
//      IssuerA  IssuerMint  →  HolderP2A  200 ALPHA
//      IssuerB  IssuerMint  →  HolderP1B  300 BETA
//      IssuerB  IssuerMint  →  HolderP2B  400 BETA
//
//    Each mint creates a CIP56Holding contract:
//
//      template CIP56Holding
//        with issuer : Party   -- signatory
//             owner  : Party   -- observer
//             amount : Decimal
//
//    The synchronizer delivers each holding to every participant that hosts
//    one of its stakeholders:
//      P1 receives it  — hosts the issuer (signatory)
//      P2 receives it  — hosts the owner  (observer) for P2 holdings
//
// 6. Balance Verification
//
//    Issuer queries (from P1):
//    IssuerA is the signatory on every ALPHA holding, so P1 stores all of them
//    regardless of which participant the owner is on:
//
//      P1 / IssuerA  →  HolderP1A=100 + HolderP2A=200  =  300 ALPHA
//      P1 / IssuerB  →  HolderP1B=300 + HolderP2B=400  =  700 BETA
//
//    Holder queries (each from their own participant):
//    A participant only stores contracts where one of its hosted parties is a
//    stakeholder, so each holder queries from the participant they live on:
//
//      P1 / HolderP1A  →  100 ALPHA, no BETA
//      P1 / HolderP1B  →  300 BETA,  no ALPHA
//      P2 / HolderP2A  →  200 ALPHA, no BETA
//      P2 / HolderP2B  →  400 BETA,  no ALPHA
//
//    Why P1 sees P2's holders without multi-hosting:
//    P1 does not need to know about HolderP2A directly. When IssuerA mints to
//    HolderP2A, the synchronizer delivers that contract to P1 (issuer=signatory)
//    and P2 (owner=observer). Querying P1 with party=IssuerA therefore returns
//    the full cross-participant balance for ALPHA.
//
//    Cross-participant holder query (HolderP1A via P2):
//    Because HolderP1A was allocated with P2 as an observing participant, the
//    synchronizer also delivers HolderP1A's holdings to P2. This models a
//    real-world scenario where a user's party is hosted on multiple participants
//    and can query their balance from any of them:
//
//      P2 / HolderP1A  →  100 ALPHA  (same holding, delivered to P2 as observer)
//
// =============================================================================
// Prerequisites: docker compose up (Canton running, DARs on participant1)
// =============================================================================
//
// Usage:
//   go run scripts/testing/test-cip56-multi-participant.go \
//     -cip56-package-id c8c6fe7c34d96b88d6471769aae85063c8045783b2a226fd24f8c573603d17c2 \
//     -domain "local::..." \
//     -p1     localhost:5011 \
//     -p2     localhost:5021

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	adminv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2/admin"
	interactivev2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2/interactive"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/ledger"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"
	"github.com/chainsafe/canton-middleware/pkg/keys"
)

// ─── flags ───────────────────────────────────────────────────────────────────

var (
	cip56PackageID = flag.String("cip56-package-id",
		"c8c6fe7c34d96b88d6471769aae85063c8045783b2a226fd24f8c573603d17c2",
		"DAML package ID for CIP56.Token / CIP56.Config templates")

	domainID = flag.String("domain", "",
		"Canton synchronizer ID (auto-detected if empty)")

	p1Addr = flag.String("p1", "localhost:5011", "Participant1 gRPC address")
	p2Addr = flag.String("p2", "localhost:5021", "Participant2 gRPC address")

	tokenURL     = flag.String("token-url", "http://localhost:8088/oauth/token", "OAuth2 token endpoint")
	clientID     = flag.String("client-id", "local-test-client", "OAuth2 client ID")
	clientSecret = flag.String("client-secret", "local-test-secret", "OAuth2 client secret")

	darDir = flag.String("dar-dir", "contracts/canton-erc20/daml",
		"Root directory containing .dar files to upload to participant2 (required for multi-hosting)")

	spliceTransferPackageID = flag.String("splice-transfer-package-id",
		"55ba4deb0ad4662c4168b39859738a0e91388d252286480c7331b3f71a517281",
		"DAML package ID for Splice.Api.Token.TransferInstructionV1 (TransferFactory interface)")
)

// ─── colours ─────────────────────────────────────────────────────────────────

const (
	green = "\033[0;32m"
	red   = "\033[0;31m"
	cyan  = "\033[0;36m"
	reset = "\033[0m"
)

func pass(format string, a ...any) { fmt.Printf(green+"    PASS "+reset+format+"\n", a...) }
func fail(format string, a ...any) { fmt.Fprintf(os.Stderr, red+"    FAIL "+reset+format+"\n", a...) }
func info(format string, a ...any) { fmt.Printf(cyan+"    "+reset+format+"\n", a...) }
func step(msg string)              { fmt.Printf("\n>>> %s\n", msg) }

// ─── externalParty wraps a Canton party ID with its signing keypair ───────────

type externalParty struct {
	PartyID string
	KeyPair *keys.CantonKeyPair
}

// ─── main ────────────────────────────────────────────────────────────────────

func main() {
	flag.Parse()

	fmt.Println(strings.Repeat("=", 70))
	fmt.Println("  CIP56 Multi-Participant / Multi-Issuer Balance Test (External Parties)")
	fmt.Println(strings.Repeat("=", 70))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// ── connect ──────────────────────────────────────────────────────────────
	step("Connecting to participant1 (" + *p1Addr + ")")
	p1, err := newClient(*p1Addr, "http://canton:5011")
	if err != nil {
		log.Fatalf("connect p1: %v", err)
	}
	defer p1.Close()

	step("Connecting to participant2 (" + *p2Addr + ")")
	p2, err := newClient(*p2Addr, "http://canton:5021")
	if err != nil {
		log.Fatalf("connect p2: %v", err)
	}
	defer p2.Close()

	// Upload DARs to P2 so it can vet packages when hosting multi-hosted parties.
	// P1 already has these from the bootstrap; P2 needs them to process transactions
	// where it is an informee (because it co-hosts the holder parties).
	step("Uploading DARs to participant2")
	if err := uploadDARs(ctx, p2, *darDir); err != nil {
		log.Fatalf("upload DARs to P2: %v", err)
	}

	// ── resolve synchronizer ID ───────────────────────────────────────────────
	syncID := *domainID
	if syncID == "" {
		step("Auto-detecting synchronizer ID")
		syncID, err = detectSynchronizerID(ctx, p1)
		if err != nil {
			log.Fatalf("detect domain: %v\n\nHint: pass -domain explicitly", err)
		}
	}
	info("Synchronizer: %s", syncID)

	ts := time.Now().Unix()

	// ── allocate issuers (internal parties on P1) ─────────────────────────────
	step("Allocating internal issuer parties on P1")
	issuerA, err := allocateParty(ctx, p1, fmt.Sprintf("IssuerAlpha%d", ts))
	if err != nil {
		log.Fatalf("allocate IssuerA: %v", err)
	}
	issuerB, err := allocateParty(ctx, p1, fmt.Sprintf("IssuerBeta%d", ts))
	if err != nil {
		log.Fatalf("allocate IssuerB: %v", err)
	}
	info("IssuerA (internal): %s", issuerA)
	info("IssuerB (internal): %s", issuerB)

	// ── allocate holders (external parties with keypairs) ─────────────────────
	step("Allocating external holder parties on P1")
	// HolderP1A is allocated on P1 with P2 listed as an observing participant.
	// P2 must also call AllocateExternalParty with the same topology to add its
	// namespace signature — only then the topology is fully authorized and the
	// synchronizer will deliver HolderP1A's contracts to P2.
	holderP1A, err := allocateExternalParty(ctx, p1, fmt.Sprintf("HolderP1Alpha%d", ts), syncID, p2)
	if err != nil {
		log.Fatalf("allocate HolderP1A: %v", err)
	}
	holderP1B, err := allocateExternalParty(ctx, p1, fmt.Sprintf("HolderP1Beta%d", ts), syncID)
	if err != nil {
		log.Fatalf("allocate HolderP1B: %v", err)
	}
	info("HolderP1A (external): %s", holderP1A.PartyID)
	info("HolderP1B (external): %s", holderP1B.PartyID)

	step("Allocating external holder parties on P2")
	holderP2A, err := allocateExternalParty(ctx, p2, fmt.Sprintf("HolderP2Alpha%d", ts), syncID)
	if err != nil {
		log.Fatalf("allocate HolderP2A: %v", err)
	}
	holderP2B, err := allocateExternalParty(ctx, p2, fmt.Sprintf("HolderP2Beta%d", ts), syncID)
	if err != nil {
		log.Fatalf("allocate HolderP2B: %v", err)
	}
	info("HolderP2A (external, P2 only): %s", holderP2A.PartyID)
	info("HolderP2B (external, P2 only): %s", holderP2B.PartyID)

	// ── create ALPHA token (IssuerA) ──────────────────────────────────────────
	step("IssuerA: creating CIP56Manager for ALPHA")
	alphaManagerCid, err := createManager(ctx, p1, issuerA, syncID, "ALPHA", "Alpha Token")
	if err != nil {
		log.Fatalf("create alpha manager: %v", err)
	}
	info("ALPHA CIP56Manager: %s", alphaManagerCid)

	step("IssuerA: creating TokenConfig for ALPHA")
	alphaConfigCid, err := createTokenConfig(ctx, p1, issuerA, syncID, alphaManagerCid, "ALPHA", "Alpha Token")
	if err != nil {
		log.Fatalf("create alpha config: %v", err)
	}
	info("ALPHA TokenConfig: %s", alphaConfigCid)

	// ── create BETA token (IssuerB) ───────────────────────────────────────────
	step("IssuerB: creating CIP56Manager for BETA")
	betaManagerCid, err := createManager(ctx, p1, issuerB, syncID, "BETA", "Beta Token")
	if err != nil {
		log.Fatalf("create beta manager: %v", err)
	}
	info("BETA CIP56Manager: %s", betaManagerCid)

	step("IssuerB: creating TokenConfig for BETA")
	betaConfigCid, err := createTokenConfig(ctx, p1, issuerB, syncID, betaManagerCid, "BETA", "Beta Token")
	if err != nil {
		log.Fatalf("create beta config: %v", err)
	}
	info("BETA TokenConfig: %s", betaConfigCid)

	// ── mint ──────────────────────────────────────────────────────────────────
	step("Minting ALPHA tokens")
	if err := mint(ctx, p1, issuerA, alphaConfigCid, holderP1A.PartyID, "100.0", syncID); err != nil {
		log.Fatalf("mint ALPHA to HolderP1A: %v", err)
	}
	info("Minted 100.0 ALPHA → HolderP1A")

	if err := mint(ctx, p1, issuerA, alphaConfigCid, holderP2A.PartyID, "200.0", syncID); err != nil {
		log.Fatalf("mint ALPHA to HolderP2A: %v", err)
	}
	info("Minted 200.0 ALPHA → HolderP2A")

	step("Minting BETA tokens")
	if err := mint(ctx, p1, issuerB, betaConfigCid, holderP1B.PartyID, "300.0", syncID); err != nil {
		log.Fatalf("mint BETA to HolderP1B: %v", err)
	}
	info("Minted 300.0 BETA → HolderP1B")

	if err := mint(ctx, p1, issuerB, betaConfigCid, holderP2B.PartyID, "400.0", syncID); err != nil {
		log.Fatalf("mint BETA to HolderP2B: %v", err)
	}
	info("Minted 400.0 BETA → HolderP2B")

	// ── verify ────────────────────────────────────────────────────────────────
	fmt.Println()
	fmt.Println(strings.Repeat("-", 70))
	fmt.Println("  Balance Verification")
	fmt.Println(strings.Repeat("-", 70))

	failures := 0

	step("P1 / IssuerA view: all ALPHA holdings")
	alphaHoldings, err := queryHoldings(ctx, p1, issuerA)
	if err != nil {
		log.Fatalf("query ALPHA (P1): %v", err)
	}
	alphaHoldings = filterByIssuer(alphaHoldings, issuerA)
	check(&failures, len(alphaHoldings) == 2, "expected 2 ALPHA holdings, got %d", len(alphaHoldings))
	check(&failures, sumAmounts(alphaHoldings) == "300", "ALPHA total: expected 300, got %s", sumAmounts(alphaHoldings))
	checkHolderAmount(alphaHoldings, holderP1A.PartyID, "100", &failures)
	checkHolderAmount(alphaHoldings, holderP2A.PartyID, "200", &failures)

	step("P1 / IssuerB view: all BETA holdings")
	betaHoldings, err := queryHoldings(ctx, p1, issuerB)
	if err != nil {
		log.Fatalf("query BETA (P1): %v", err)
	}
	betaHoldings = filterByIssuer(betaHoldings, issuerB)
	check(&failures, len(betaHoldings) == 2, "expected 2 BETA holdings, got %d", len(betaHoldings))
	check(&failures, sumAmounts(betaHoldings) == "700", "BETA total: expected 700, got %s", sumAmounts(betaHoldings))
	checkHolderAmount(betaHoldings, holderP1B.PartyID, "300", &failures)
	checkHolderAmount(betaHoldings, holderP2B.PartyID, "400", &failures)

	step("P1 / HolderP1A view: 100 ALPHA, no BETA")
	p1aH, err := queryHoldings(ctx, p1, holderP1A.PartyID)
	if err != nil {
		log.Fatalf("query HolderP1A: %v", err)
	}
	check(&failures, len(p1aH) == 1, "HolderP1A: expected 1 holding, got %d", len(p1aH))
	checkHolderAmount(p1aH, holderP1A.PartyID, "100", &failures)
	check(&failures, containsNoIssuer(p1aH, issuerB), "HolderP1A should hold no BETA")

	step("P1 / HolderP1B view: 300 BETA, no ALPHA")
	p1bH, err := queryHoldings(ctx, p1, holderP1B.PartyID)
	if err != nil {
		log.Fatalf("query HolderP1B: %v", err)
	}
	check(&failures, len(p1bH) == 1, "HolderP1B: expected 1 holding, got %d", len(p1bH))
	checkHolderAmount(p1bH, holderP1B.PartyID, "300", &failures)
	check(&failures, containsNoIssuer(p1bH, issuerA), "HolderP1B should hold no ALPHA")

	step("P2 / HolderP2A view: 200 ALPHA, no BETA")
	p2aH, err := queryHoldings(ctx, p2, holderP2A.PartyID)
	if err != nil {
		log.Fatalf("query HolderP2A (P2): %v", err)
	}
	check(&failures, len(p2aH) == 1, "HolderP2A on P2: expected 1 holding, got %d", len(p2aH))
	checkHolderAmount(p2aH, holderP2A.PartyID, "200", &failures)
	check(&failures, containsNoIssuer(p2aH, issuerB), "HolderP2A should hold no BETA")

	step("P2 / HolderP2B view: 400 BETA, no ALPHA")
	p2bH, err := queryHoldings(ctx, p2, holderP2B.PartyID)
	if err != nil {
		log.Fatalf("query HolderP2B (P2): %v", err)
	}
	check(&failures, len(p2bH) == 1, "HolderP2B on P2: expected 1 holding, got %d", len(p2bH))
	checkHolderAmount(p2bH, holderP2B.PartyID, "400", &failures)
	check(&failures, containsNoIssuer(p2bH, issuerA), "HolderP2B should hold no ALPHA")

	// HolderP1A was allocated with P2 listed as an observing participant, so the
	// synchronizer also delivers HolderP1A's holdings to P2. This verifies the
	// real-world use case: a party hosted on P1 can query their balance via P2
	// because P2 was registered as an observing participant at allocation time.
	step("P2 / HolderP1A view: 100 ALPHA (cross-participant via observation)")
	p2p1aH, err := queryHoldings(ctx, p2, holderP1A.PartyID)
	if err != nil {
		log.Fatalf("query HolderP1A via P2: %v", err)
	}
	check(&failures, len(p2p1aH) == 1, "HolderP1A on P2: expected 1 holding, got %d", len(p2p1aH))
	checkHolderAmount(p2p1aH, holderP1A.PartyID, "100", &failures)
	check(&failures, containsNoIssuer(p2p1aH, issuerB), "HolderP1A on P2 should hold no BETA")

	// HolderP1B was allocated on P1 only — P2 was not listed as an observing
	// participant. The synchronizer never delivered HolderP1B's holdings to P2,
	// so querying P2 with that party returns nothing. This contrasts with
	// HolderP1A above and shows why the ObservingParticipantUids step matters.
	step("P2 / HolderP1B view: 0 holdings (P2 has no record — not an observer)")
	p2p1bH, err := queryHoldings(ctx, p2, holderP1B.PartyID)
	if err != nil {
		log.Fatalf("query HolderP1B via P2: %v", err)
	}
	check(&failures, len(p2p1bH) == 0, "HolderP1B on P2: expected 0 holdings, got %d", len(p2p1bH))
	if len(p2p1bH) == 0 {
		pass("HolderP1B on P2: confirmed no holdings visible (party not observed by P2)")
	}

	// ── partial transfer: HolderP2B → HolderP1B (150 BETA via P2) ────────────
	//
	// HolderP2B holds 400 BETA on P2. She wants to send 150 BETA to HolderP1B.
	// The transfer uses CIP56TransferFactory (exercised via P2's Interactive
	// Submission API), which atomically:
	//   - archives the input holding (400 BETA)
	//   - creates a receiver holding  (150 BETA → HolderP1B)
	//   - creates a change holding    (250 BETA → HolderP2B)
	//
	// Because HolderP2B is an external party, the transaction must go through
	// the Interactive Submission flow: PrepareSubmission → sign with P2B's key
	// → ExecuteSubmissionAndWait.
	fmt.Println()
	fmt.Println(strings.Repeat("-", 70))
	fmt.Println("  Partial Transfer: HolderP2B → HolderP1B  (150 BETA via P2)")
	fmt.Println(strings.Repeat("-", 70))

	// Step 1: Create a single CIP56TransferFactory on P1 (no audit observers).
	// P2 does not have this contract in its ACS — instead we use explicit
	// disclosure: the factory is fetched from P1 and attached to the
	// PrepareSubmissionRequest so P2 can resolve it without direct P1 access
	// or any observer relationship.
	step("IssuerB: creating CIP56TransferFactory (P1, no observers)")
	factoryCID, err := createTransferFactory(ctx, p1, issuerB, syncID)
	if err != nil {
		log.Fatalf("create transfer factory: %v", err)
	}
	info("CIP56TransferFactory: %s", factoryCID)

	// Step 2: Fetch the factory contract from P1 as a DisclosedContract.
	// The CreatedEventBlob is an opaque Canton-signed blob that P2 can verify
	// and use for the transaction without the contract being in P2's ACS.
	step("Fetching CIP56TransferFactory from P1 for explicit disclosure")
	disclosedFactory, err := fetchDisclosedContract(ctx, p1, issuerB, syncID,
		cip56TemplateID("CIP56.TransferFactory", "CIP56TransferFactory"))
	if err != nil {
		log.Fatalf("fetch disclosed factory: %v", err)
	}
	info("DisclosedContract ready (blob: %d bytes)", len(disclosedFactory.CreatedEventBlob))

	// Step 3: Fetch HolderP2B's current holdings from P2 to get contract IDs.
	step("P2 / HolderP2B: fetching holdings before transfer")
	p2bPreH, err := queryHoldings(ctx, p2, holderP2B.PartyID)
	if err != nil {
		log.Fatalf("query HolderP2B pre-transfer: %v", err)
	}
	if len(p2bPreH) == 0 {
		log.Fatalf("HolderP2B has no holdings to transfer")
	}
	inputCIDs := make([]string, len(p2bPreH))
	for i, h := range p2bPreH {
		inputCIDs[i] = h.ContractID
	}
	info("Input holdings: %d contract(s)", len(inputCIDs))

	// Step 4: Execute the transfer via Interactive Submission on P2.
	// The disclosed factory contract is attached so P2 can resolve it locally.
	// HolderP2B signs the prepared transaction hash with her secp256k1 key.
	step("HolderP2B → HolderP1B: transferring 150 BETA via P2 (explicit disclosure)")
	if err := transferViaInteractive(ctx, p2, &transferArgs{
		SyncID:            syncID,
		FactoryCID:        factoryCID,
		InstrumentAdmin:   issuerB,
		InstrumentID:      "BETA",
		Sender:            holderP2B.PartyID,
		Receiver:          holderP1B.PartyID,
		Amount:            "150",
		InputCIDs:         inputCIDs,
		SignerKey:         holderP2B.KeyPair,
		DisclosedContracts: []*lapiv2.DisclosedContract{disclosedFactory},
	}); err != nil {
		log.Fatalf("transfer BETA P2B→P1B: %v", err)
	}
	info("Transfer complete")

	// Step 4: Verify updated balances.
	step("P2 / HolderP2B view: 250 BETA remaining after transfer")
	p2bPostH, err := queryHoldings(ctx, p2, holderP2B.PartyID)
	if err != nil {
		log.Fatalf("query HolderP2B post-transfer: %v", err)
	}
	check(&failures, len(p2bPostH) == 1, "HolderP2B post-transfer: expected 1 holding, got %d", len(p2bPostH))
	checkHolderAmount(p2bPostH, holderP2B.PartyID, "250", &failures)

	step("P1 / HolderP1B view: 450 BETA total after receiving transfer (2 holdings: 300 mint + 150 received)")
	p1bPostH, err := queryHoldings(ctx, p1, holderP1B.PartyID)
	if err != nil {
		log.Fatalf("query HolderP1B post-transfer: %v", err)
	}
	check(&failures, len(p1bPostH) == 2, "HolderP1B post-transfer: expected 2 holdings (300+150), got %d", len(p1bPostH))
	check(&failures, sumAmounts(p1bPostH) == "450", "HolderP1B post-transfer total: expected 450, got %s", sumAmounts(p1bPostH))

	// ── result ────────────────────────────────────────────────────────────────
	fmt.Println()
	fmt.Println(strings.Repeat("=", 70))
	if failures == 0 {
		fmt.Println(green + "  ALL CHECKS PASSED" + reset)
	} else {
		fmt.Fprintf(os.Stderr, red+"  %d CHECK(S) FAILED"+reset+"\n", failures)
		os.Exit(1)
	}
	fmt.Println(strings.Repeat("=", 70))
}

// ─── Canton client helpers ────────────────────────────────────────────────────

func newClient(addr, audience string) (*ledger.Client, error) {
	cfg := &ledger.Config{
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
	}
	return ledger.New(cfg)
}

// uploadDARs uploads all *.dar files in darDir to the participant so it can
// vet and process transactions containing those templates.
// Errors are logged but not fatal — already-uploaded packages are ignored.
func uploadDARs(ctx context.Context, c *ledger.Client, darDir string) error {
	pkgSvc := adminv2.NewPackageManagementServiceClient(c.Conn())
	authCtx := c.AuthContext(ctx)

	// Use a manual walk instead of Glob to stay stdlib-only.
	var dars []string
	err := filepath.WalkDir(darDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".dar") {
			dars = append(dars, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk DAR directory %s: %w", darDir, err)
	}

	if len(dars) == 0 {
		return fmt.Errorf("no .dar files found under %s", darDir)
	}

	for _, p := range dars {
		data, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read dar %s: %w", p, err)
		}
		_, err = pkgSvc.UploadDarFile(authCtx, &adminv2.UploadDarFileRequest{
			DarFile:       data,
			VettingChange: adminv2.UploadDarFileRequest_VETTING_CHANGE_VET_ALL_PACKAGES,
		})
		if err != nil {
			info("  upload %s: %v (may already exist)", filepath.Base(p), err)
		} else {
			info("  uploaded %s", filepath.Base(p))
		}
	}
	return nil
}

func getParticipantUID(ctx context.Context, c *ledger.Client) (string, error) {
	authCtx := c.AuthContext(ctx)
	resp, err := c.PartyAdmin().GetParticipantId(authCtx, &adminv2.GetParticipantIdRequest{})
	if err != nil {
		return "", fmt.Errorf("get participant id: %w", err)
	}
	return resp.ParticipantId, nil
}

func detectSynchronizerID(ctx context.Context, c *ledger.Client) (string, error) {
	authCtx := c.AuthContext(ctx)

	sub, _ := c.JWTSubject(authCtx)
	if sub == "" {
		sub = "test-user"
	}

	hint := fmt.Sprintf("SyncProbe%d", time.Now().UnixNano())
	allocResp, err := c.PartyAdmin().AllocateParty(authCtx, &adminv2.AllocatePartyRequest{
		PartyIdHint: hint,
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
			Commands: []*lapiv2.Command{
				{
					Command: &lapiv2.Command_Create{
						Create: &lapiv2.CreateCommand{
							TemplateId: cip56TemplateID("CIP56.Token", "CIP56Manager"),
							CreateArguments: &lapiv2.Record{
								Fields: []*lapiv2.RecordField{
									{Label: "issuer", Value: values.PartyValue(probe)},
									{Label: "instrumentId", Value: values.EncodeInstrumentId(probe, "PROBE")},
									{Label: "meta", Value: values.EmptyMetadata()},
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("probe command: %w\n\nHint: pass -domain explicitly (see config.e2e-local.yaml)", err)
	}
	if resp.Transaction == nil || resp.Transaction.SynchronizerId == "" {
		return "", fmt.Errorf("probe returned no synchronizer ID; pass -domain explicitly")
	}
	return resp.Transaction.SynchronizerId, nil
}

// allocateParty allocates an internal (operator-controlled) Canton party.
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

// allocateExternalParty allocates an external party (with its own secp256k1 keypair)
// on the given primary participant. Pass additionalObservers to list other participant
// clients that should also host the party with observation permission — they must
// counter-sign the topology for it to become effective on the synchronizer.
func allocateExternalParty(ctx context.Context, c *ledger.Client, hint, syncID string, additionalObservers ...*ledger.Client) (*externalParty, error) {
	// Collect UIDs of any additional observer participants.
	observingUIDs := make([]string, 0, len(additionalObservers))
	for _, obs := range additionalObservers {
		uid, err := getParticipantUID(ctx, obs)
		if err != nil {
			return nil, fmt.Errorf("get observer participant UID: %w", err)
		}
		observingUIDs = append(observingUIDs, uid)
	}

	kp, err := keys.GenerateCantonKeyPair()
	if err != nil {
		return nil, fmt.Errorf("generate keypair for %q: %w", hint, err)
	}

	spki, err := kp.SPKIPublicKey()
	if err != nil {
		return nil, fmt.Errorf("encode SPKI for %q: %w", hint, err)
	}

	authCtx := c.AuthContext(ctx)

	pubKey := &lapiv2.SigningPublicKey{
		Format:  lapiv2.CryptoKeyFormat_CRYPTO_KEY_FORMAT_DER_X509_SUBJECT_PUBLIC_KEY_INFO,
		KeyData: spki,
		KeySpec: lapiv2.SigningKeySpec_SIGNING_KEY_SPEC_EC_SECP256K1,
	}

	topoResp, err := c.PartyAdmin().GenerateExternalPartyTopology(authCtx, &adminv2.GenerateExternalPartyTopologyRequest{
		Synchronizer:             syncID,
		PartyHint:                hint,
		PublicKey:                pubKey,
		ObservingParticipantUids: observingUIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("generate topology for %q: %w", hint, err)
	}

	derSig, err := kp.SignDER(topoResp.MultiHash)
	if err != nil {
		return nil, fmt.Errorf("sign topology for %q: %w", hint, err)
	}

	multiHashSig := &lapiv2.Signature{
		Format:               lapiv2.SignatureFormat_SIGNATURE_FORMAT_DER,
		Signature:            derSig,
		SignedBy:             topoResp.PublicKeyFingerprint,
		SigningAlgorithmSpec: lapiv2.SigningAlgorithmSpec_SIGNING_ALGORITHM_SPEC_EC_DSA_SHA_256,
	}

	signedTxs := make([]*adminv2.AllocateExternalPartyRequest_SignedTransaction, len(topoResp.TopologyTransactions))
	for i, tx := range topoResp.TopologyTransactions {
		signedTxs[i] = &adminv2.AllocateExternalPartyRequest_SignedTransaction{Transaction: tx}
	}

	allocReq := &adminv2.AllocateExternalPartyRequest{
		Synchronizer:           syncID,
		OnboardingTransactions: signedTxs,
		MultiHashSignatures:    []*lapiv2.Signature{multiHashSig},
	}

	allocResp, err := c.PartyAdmin().AllocateExternalParty(authCtx, allocReq)
	if err != nil {
		return nil, fmt.Errorf("allocate external party %q: %w", hint, err)
	}

	// Each observer participant must also submit AllocateExternalParty with the
	// same topology transactions and party signature so that its namespace key
	// co-signs the PartyToParticipant mapping, making the topology fully effective.
	for _, obs := range additionalObservers {
		obsCtx := obs.AuthContext(ctx)
		if _, err := obs.PartyAdmin().AllocateExternalParty(obsCtx, allocReq); err != nil {
			return nil, fmt.Errorf("observer confirm external party %q: %w", hint, err)
		}
	}

	return &externalParty{PartyID: allocResp.PartyId, KeyPair: kp}, nil
}

// ─── CIP56 contract helpers ───────────────────────────────────────────────────

func createManager(ctx context.Context, c *ledger.Client, issuer, syncID, symbol, name string) (string, error) {
	authCtx := c.AuthContext(ctx)
	sub, _ := c.JWTSubject(authCtx)
	if sub == "" {
		sub = "test-user"
	}
	resp, err := c.Command().SubmitAndWaitForTransaction(authCtx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: syncID,
			CommandId:      fmt.Sprintf("create-manager-%s-%d", symbol, time.Now().UnixNano()),
			UserId:         sub,
			ActAs:          []string{issuer},
			Commands: []*lapiv2.Command{
				{
					Command: &lapiv2.Command_Create{
						Create: &lapiv2.CreateCommand{
							TemplateId: cip56TemplateID("CIP56.Token", "CIP56Manager"),
							CreateArguments: &lapiv2.Record{
								Fields: []*lapiv2.RecordField{
									{Label: "issuer", Value: values.PartyValue(issuer)},
									{Label: "instrumentId", Value: values.EncodeInstrumentId(issuer, symbol)},
									{Label: "meta", Value: values.EncodeMetadata(map[string]string{
										"splice.chainsafe.io/name":   name,
										"splice.chainsafe.io/symbol": symbol,
									})},
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		return "", err
	}
	return findCreated(resp.Transaction, "CIP56Manager")
}

func createTokenConfig(ctx context.Context, c *ledger.Client, issuer, syncID, managerCid, symbol, name string) (string, error) {
	authCtx := c.AuthContext(ctx)
	sub, _ := c.JWTSubject(authCtx)
	if sub == "" {
		sub = "test-user"
	}
	resp, err := c.Command().SubmitAndWaitForTransaction(authCtx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: syncID,
			CommandId:      fmt.Sprintf("create-config-%s-%d", symbol, time.Now().UnixNano()),
			UserId:         sub,
			ActAs:          []string{issuer},
			Commands: []*lapiv2.Command{
				{
					Command: &lapiv2.Command_Create{
						Create: &lapiv2.CreateCommand{
							TemplateId: cip56TemplateID("CIP56.Config", "TokenConfig"),
							CreateArguments: &lapiv2.Record{
								Fields: []*lapiv2.RecordField{
									{Label: "issuer", Value: values.PartyValue(issuer)},
									{Label: "tokenManagerCid", Value: values.ContractIDValue(managerCid)},
									{Label: "instrumentId", Value: values.EncodeInstrumentId(issuer, symbol)},
									{Label: "meta", Value: values.EncodeMetadata(map[string]string{
										"splice.chainsafe.io/name":   name,
										"splice.chainsafe.io/symbol": symbol,
									})},
									{Label: "auditObservers", Value: values.ListValue(nil)},
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		return "", err
	}
	return findCreated(resp.Transaction, "TokenConfig")
}

func mint(ctx context.Context, c *ledger.Client, issuer, configCid, recipient, amount, syncID string) error {
	authCtx := c.AuthContext(ctx)
	sub, _ := c.JWTSubject(authCtx)
	if sub == "" {
		sub = "test-user"
	}
	resp, err := c.Command().SubmitAndWaitForTransaction(authCtx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: syncID,
			CommandId:      fmt.Sprintf("mint-%d", time.Now().UnixNano()),
			UserId:         sub,
			ActAs:          []string{issuer},
			Commands: []*lapiv2.Command{
				{
					Command: &lapiv2.Command_Exercise{
						Exercise: &lapiv2.ExerciseCommand{
							TemplateId: cip56TemplateID("CIP56.Config", "TokenConfig"),
							ContractId: configCid,
							Choice:     "IssuerMint",
							ChoiceArgument: &lapiv2.Value{
								Sum: &lapiv2.Value_Record{
									Record: &lapiv2.Record{
										Fields: []*lapiv2.RecordField{
											{Label: "recipient", Value: values.PartyValue(recipient)},
											{Label: "amount", Value: values.NumericValue(amount)},
											{Label: "eventTime", Value: values.TimestampValue(time.Now())},
											{Label: "eventMeta", Value: values.None()},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		return err
	}
	if _, err = findCreated(resp.Transaction, "CIP56Holding"); err != nil {
		return fmt.Errorf("CIP56Holding not in mint response: %w", err)
	}
	return nil
}

// ─── query + decode ───────────────────────────────────────────────────────────

type holding struct {
	ContractID string
	Issuer     string
	Owner      string
	Amount     string
}

func queryHoldings(ctx context.Context, c *ledger.Client, party string) ([]holding, error) {
	authCtx := c.AuthContext(ctx)
	offset, err := c.GetLedgerEnd(authCtx)
	if err != nil {
		return nil, err
	}
	events, err := c.GetActiveContractsByTemplate(authCtx, offset, []string{party},
		cip56TemplateID("CIP56.Token", "CIP56Holding"))
	if err != nil {
		return nil, err
	}
	out := make([]holding, 0, len(events))
	for _, e := range events {
		f := values.RecordToMap(e.GetCreateArguments())
		out = append(out, holding{
			ContractID: e.ContractId,
			Issuer:     values.Party(f["issuer"]),
			Owner:      values.Party(f["owner"]),
			Amount:     values.Numeric(f["amount"]),
		})
	}
	return out, nil
}

// ─── assertion helpers ────────────────────────────────────────────────────────

func check(failures *int, ok bool, format string, a ...any) bool {
	msg := fmt.Sprintf(format, a...)
	if ok {
		pass("%s", msg)
		return true
	}
	fail("%s", msg)
	*failures++
	return false
}

func checkHolderAmount(holdings []holding, party, expectedWhole string, failures *int) {
	for _, h := range holdings {
		if h.Owner == party {
			got, ok1 := new(big.Rat).SetString(h.Amount)
			exp, ok2 := new(big.Rat).SetString(expectedWhole)
			if !ok1 || !ok2 {
				fail("party %s: failed to parse amounts (got=%q, exp=%q)", shortID(party), h.Amount, expectedWhole)
				*failures++
				return
			}
			check(failures, got.Cmp(exp) == 0,
				"party %s: expected %s, got %s", shortID(party), expectedWhole, h.Amount)
			return
		}
	}
	fail("party %s not found in holdings", shortID(party))
	*failures++
}

func filterByIssuer(holdings []holding, issuer string) []holding {
	var out []holding
	for _, h := range holdings {
		if h.Issuer == issuer {
			out = append(out, h)
		}
	}
	return out
}

func containsNoIssuer(holdings []holding, issuer string) bool {
	for _, h := range holdings {
		if h.Issuer == issuer {
			return false
		}
	}
	return true
}

func sumAmounts(holdings []holding) string {
	total := new(big.Rat)
	for _, h := range holdings {
		if v, ok := new(big.Rat).SetString(h.Amount); ok {
			total.Add(total, v)
		}
	}
	s := total.FloatString(10)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s
}

func shortID(id string) string {
	if len(id) > 20 {
		return id[:20] + "…"
	}
	return id
}

// ─── proto helpers ────────────────────────────────────────────────────────────

func cip56TemplateID(module, entity string) *lapiv2.Identifier {
	return &lapiv2.Identifier{
		PackageId:  *cip56PackageID,
		ModuleName: module,
		EntityName: entity,
	}
}

func findCreated(tx *lapiv2.Transaction, entityName string) (string, error) {
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

// ─── transfer factory helpers ─────────────────────────────────────────────────

// createTransferFactory creates a CIP56TransferFactory contract on the ledger.
// The factory is the entry point for all peer-to-peer token transfers under the
// given admin (issuer). Must be called by the issuer via P1.
func createTransferFactory(ctx context.Context, c *ledger.Client, admin, syncID string, auditObservers ...string) (string, error) {
	authCtx := c.AuthContext(ctx)
	sub, _ := c.JWTSubject(authCtx)
	if sub == "" {
		sub = "test-user"
	}
	resp, err := c.Command().SubmitAndWaitForTransaction(authCtx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: syncID,
			CommandId:      fmt.Sprintf("create-factory-%d", time.Now().UnixNano()),
			UserId:         sub,
			ActAs:          []string{admin},
			Commands: []*lapiv2.Command{
				{
					Command: &lapiv2.Command_Create{
						Create: &lapiv2.CreateCommand{
							TemplateId: &lapiv2.Identifier{
								PackageId:  *cip56PackageID,
								ModuleName: "CIP56.TransferFactory",
								EntityName: "CIP56TransferFactory",
							},
							CreateArguments: &lapiv2.Record{
								Fields: []*lapiv2.RecordField{
									{Label: "admin", Value: values.PartyValue(admin)},
									{Label: "auditObservers", Value: values.ListValue(func() []*lapiv2.Value {
										obs := make([]*lapiv2.Value, len(auditObservers))
										for i, p := range auditObservers {
											obs[i] = values.PartyValue(p)
										}
										return obs
									}())},
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		return "", err
	}
	return findCreated(resp.Transaction, "CIP56TransferFactory")
}

// transferArgs holds the parameters for a peer-to-peer CIP56 token transfer.
type transferArgs struct {
	SyncID             string
	FactoryCID         string
	InstrumentAdmin    string
	InstrumentID       string
	Sender             string
	Receiver           string
	Amount             string
	InputCIDs          []string
	SignerKey          *keys.CantonKeyPair
	DisclosedContracts []*lapiv2.DisclosedContract // optional: contracts not in local ACS
}

// fetchDisclosedContract retrieves a contract from the given participant's ACS
// and returns it as a DisclosedContract ready to be attached to a
// PrepareSubmissionRequest on a different participant that does not have the
// contract in its own ACS (explicit disclosure).
// It calls GetActiveContracts directly with IncludeCreatedEventBlob=true —
// the higher-level GetActiveContractsByTemplate does not request the blob.
func fetchDisclosedContract(ctx context.Context, c *ledger.Client, party, syncID string, templateID *lapiv2.Identifier) (*lapiv2.DisclosedContract, error) {
	authCtx := c.AuthContext(ctx)
	offset, err := c.GetLedgerEnd(authCtx)
	if err != nil {
		return nil, fmt.Errorf("get ledger end: %w", err)
	}

	stream, err := c.State().GetActiveContracts(authCtx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: offset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				party: {
					Cumulative: []*lapiv2.CumulativeFilter{
						{
							IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
								TemplateFilter: &lapiv2.TemplateFilter{
									TemplateId:              templateID,
									IncludeCreatedEventBlob: true,
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
		return nil, fmt.Errorf("get active contracts: %w", err)
	}

	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		if ac := resp.GetActiveContract(); ac != nil && ac.CreatedEvent != nil {
			e := ac.CreatedEvent
			return &lapiv2.DisclosedContract{
				TemplateId:       e.TemplateId,
				ContractId:       e.ContractId,
				CreatedEventBlob: e.CreatedEventBlob,
				SynchronizerId:   syncID,
			}, nil
		}
	}
	return nil, fmt.Errorf("no active contract found for template %s", templateID.EntityName)
}

// transferViaInteractive executes a CIP56 transfer using the Interactive
// Submission API. Since the sender is an external party (no operator-held key),
// the transaction must be prepared by the participant, signed by the party's
// secp256k1 key, and then executed.
func transferViaInteractive(ctx context.Context, c *ledger.Client, args *transferArgs) error {
	authCtx := c.AuthContext(ctx)
	sub, _ := c.JWTSubject(authCtx)
	if sub == "" {
		sub = "test-user"
	}

	now := time.Now().UTC()
	executeBefore := now.Add(time.Hour)

	holdingCidValues := make([]*lapiv2.Value, len(args.InputCIDs))
	for i, cid := range args.InputCIDs {
		holdingCidValues[i] = values.ContractIDValue(cid)
	}

	transfer := &lapiv2.Value{
		Sum: &lapiv2.Value_Record{
			Record: &lapiv2.Record{
				Fields: []*lapiv2.RecordField{
					{Label: "sender", Value: values.PartyValue(args.Sender)},
					{Label: "receiver", Value: values.PartyValue(args.Receiver)},
					{Label: "amount", Value: values.NumericValue(args.Amount)},
					{Label: "instrumentId", Value: values.EncodeInstrumentId(args.InstrumentAdmin, args.InstrumentID)},
					{Label: "requestedAt", Value: values.TimestampValue(now)},
					{Label: "executeBefore", Value: values.TimestampValue(executeBefore)},
					{Label: "inputHoldingCids", Value: values.ListValue(holdingCidValues)},
					{Label: "meta", Value: values.EmptyMetadata()},
				},
			},
		},
	}

	choiceArg := &lapiv2.Value{
		Sum: &lapiv2.Value_Record{
			Record: &lapiv2.Record{
				Fields: []*lapiv2.RecordField{
					{Label: "expectedAdmin", Value: values.PartyValue(args.InstrumentAdmin)},
					{Label: "transfer", Value: transfer},
					{Label: "extraArgs", Value: values.EncodeExtraArgs()},
				},
			},
		},
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  *spliceTransferPackageID,
					ModuleName: "Splice.Api.Token.TransferInstructionV1",
					EntityName: "TransferFactory",
				},
				ContractId:     args.FactoryCID,
				Choice:         "TransferFactory_Transfer",
				ChoiceArgument: choiceArg,
			},
		},
	}

	commands := &lapiv2.Commands{
		SynchronizerId: args.SyncID,
		CommandId:      fmt.Sprintf("transfer-%d", time.Now().UnixNano()),
		UserId:         sub,
		ActAs:          []string{args.Sender},
		Commands:       []*lapiv2.Command{cmd},
	}

	// Prepare: participant builds the transaction and returns the hash to sign.
	prepResp, err := c.Interactive().PrepareSubmission(authCtx, &interactivev2.PrepareSubmissionRequest{
		UserId:             commands.UserId,
		CommandId:          commands.CommandId,
		Commands:           commands.Commands,
		ActAs:              commands.ActAs,
		SynchronizerId:     commands.SynchronizerId,
		DisclosedContracts: args.DisclosedContracts,
	})
	if err != nil {
		return fmt.Errorf("prepare submission: %w", err)
	}

	// Sign: external party signs the transaction hash with its secp256k1 key.
	derSig, err := args.SignerKey.SignDER(prepResp.PreparedTransactionHash)
	if err != nil {
		return fmt.Errorf("sign prepared transaction: %w", err)
	}

	fingerprint, err := args.SignerKey.Fingerprint()
	if err != nil {
		return fmt.Errorf("get signer fingerprint: %w", err)
	}

	partySigs := &interactivev2.PartySignatures{
		Signatures: []*interactivev2.SinglePartySignatures{
			{
				Party: args.Sender,
				Signatures: []*lapiv2.Signature{
					{
						Format:               lapiv2.SignatureFormat_SIGNATURE_FORMAT_DER,
						Signature:            derSig,
						SignedBy:             fingerprint,
						SigningAlgorithmSpec: lapiv2.SigningAlgorithmSpec_SIGNING_ALGORITHM_SPEC_EC_DSA_SHA_256,
					},
				},
			},
		},
	}

	// Execute: submit the signed transaction.
	_, err = c.Interactive().ExecuteSubmissionAndWait(authCtx, &interactivev2.ExecuteSubmissionAndWaitRequest{
		PreparedTransaction:  prepResp.PreparedTransaction,
		PartySignatures:      partySigs,
		SubmissionId:         fmt.Sprintf("transfer-exec-%d", time.Now().UnixNano()),
		UserId:               commands.UserId,
		HashingSchemeVersion: prepResp.HashingSchemeVersion,
	})
	if err != nil {
		return fmt.Errorf("execute submission: %w", err)
	}

	return nil
}
