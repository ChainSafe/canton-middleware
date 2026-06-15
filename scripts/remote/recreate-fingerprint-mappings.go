//go:build ignore

// recreate-fingerprint-mappings.go — Recreate FingerprintMapping contracts under a
// NEW issuer party after canton.issuer_party was changed. This restores
// fingerprint→party resolution for every user, which is what brings back USDCx
// (external-token) balances and transfers — those were never stranded, only the
// lookup broke.
//
// It reads the OLD mappings straight off the ledger (they persist, signed by the old
// issuer) and re-creates each one under the new issuer with the SAME user party,
// fingerprint and EVM address. No userstore DB needed.
//
// The config's canton.issuer_party MUST be the NEW issuer: CreateFingerprintMapping
// runs ActAs=that party. Reading the old mappings relies on the OAuth user's
// can_read_as_any_party right (same right the mint/list tooling uses).
//
// Idempotent: a fingerprint that already has a mapping under the new issuer is skipped.
//
// Usage:
//
//	go run scripts/remote/recreate-fingerprint-mappings.go \
//	  -config <config.yaml> \        # canton.issuer_party = NEW issuer
//	  -old-issuer 'OLD_ISSUER::1220...' \
//	  [-apply]                        # without this it's a dry run

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	cantonclient "github.com/chainsafe/canton-middleware/pkg/cantonsdk/client"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/identity"
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/log"
)

var (
	configPath = flag.String("config", "", "Path to API server config file (required); canton.issuer_party must be the NEW issuer")
	oldIssuer  = flag.String("old-issuer", "", "OLD issuer party whose FingerprintMappings to recreate (required)")
	userParty  = flag.String("party", "", "Optional: only recreate the mapping for this user party")
	apply      = flag.Bool("apply", false, "Perform creates. Without this flag it only reports (dry run).")
)

type mapping struct {
	userParty   string
	fingerprint string
	evmAddress  string
}

func main() {
	flag.Parse()
	if *configPath == "" {
		fatalf("-config is required")
	}
	if *oldIssuer == "" {
		fatalf("-old-issuer is required")
	}

	cfg, err := config.LoadAPIServer(*configPath)
	if err != nil {
		fatalf("load config: %v", err)
	}
	newIssuer := cfg.Canton.IssuerParty
	if newIssuer == "" {
		fatalf("canton.issuer_party is required in config (must be the NEW issuer)")
	}
	if newIssuer == *oldIssuer {
		fatalf("-old-issuer equals canton.issuer_party (%s); nothing to migrate", newIssuer)
	}

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Recreate FingerprintMappings under the NEW issuer")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Printf("  Canton:      %s\n", cfg.Canton.Ledger.RPCURL)
	fmt.Printf("  Old issuer:  %s\n", *oldIssuer)
	fmt.Printf("  New issuer:  %s\n", newIssuer)
	if *userParty != "" {
		fmt.Printf("  Party:       %s\n", *userParty)
	}
	fmt.Printf("  Mode:        %s\n", modeLabel(*apply))
	fmt.Println()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	logger, err := log.NewLogger(cfg.Logging)
	if err != nil {
		fatalf("init logger: %v", err)
	}
	client, err := cantonclient.New(ctx, cfg.Canton, cantonclient.WithLogger(logger))
	if err != nil {
		fatalf("connect to Canton: %v", err)
	}
	defer func() { _ = client.Close() }()

	end, err := client.Ledger.GetLedgerEnd(ctx)
	if err != nil {
		fatalf("get ledger end: %v", err)
	}
	tid := &lapiv2.Identifier{
		PackageId:  client.Identity.PackageID(),
		ModuleName: "Common.FingerprintAuth",
		EntityName: "FingerprintMapping",
	}

	// Old mappings to migrate (old issuer is signatory → sees them all).
	oldEvents, err := client.Ledger.GetActiveContractsByTemplate(ctx, end, []string{*oldIssuer}, tid)
	if err != nil {
		fatalf("list old mappings: %v", err)
	}
	// Fingerprints already mapped under the new issuer → skip set.
	newEvents, err := client.Ledger.GetActiveContractsByTemplate(ctx, end, []string{newIssuer}, tid)
	if err != nil {
		fatalf("list new mappings: %v", err)
	}
	already := map[string]bool{}
	for _, e := range newEvents {
		already[normFP(values.Text(values.RecordToMap(e.CreateArguments)["fingerprint"]))] = true
	}

	var created, skipped int
	for _, e := range oldEvents {
		m := decodeMapping(e)
		if m.fingerprint == "" || m.userParty == "" {
			skipped++
			continue
		}
		if *userParty != "" && m.userParty != *userParty {
			skipped++
			continue
		}
		if already[normFP(m.fingerprint)] {
			skipped++
			continue
		}
		fmt.Printf("  - map fp=%s -> party=%s  evm=%s\n", m.fingerprint, m.userParty, m.evmAddress)
		if !*apply {
			created++
			continue
		}
		if _, err := client.Identity.CreateFingerprintMapping(ctx, identity.CreateFingerprintMappingRequest{
			UserParty:   m.userParty,
			Fingerprint: m.fingerprint,
			EvmAddress:  m.evmAddress,
		}); err != nil {
			fatalf("create mapping fp=%s party=%s: %v", m.fingerprint, m.userParty, err)
		}
		// Guard against duplicate old contracts for the same fingerprint in one run.
		already[normFP(m.fingerprint)] = true
		created++
	}

	fmt.Println()
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	if *apply {
		fmt.Printf("  Done. Created %d mapping(s); %d skipped (already mapped / incomplete).\n", created, skipped)
	} else {
		fmt.Printf("  Dry run. Would create %d mapping(s); %d skipped. Re-run with -apply.\n", created, skipped)
	}
	fmt.Println("══════════════════════════════════════════════════════════════════════")
}

func decodeMapping(e *lapiv2.CreatedEvent) mapping {
	f := values.RecordToMap(e.CreateArguments)
	return mapping{
		userParty:   values.Party(f["userParty"]),
		fingerprint: values.Text(f["fingerprint"]),
		evmAddress:  values.Text(f["evmAddress"]),
	}
}

func normFP(s string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(s), "0x"))
}

func modeLabel(apply bool) string {
	if apply {
		return "APPLY (mappings will be created)"
	}
	return "dry run (no writes)"
}

func fatalf(format string, args ...interface{}) {
	fmt.Printf("ERROR: "+format+"\n", args...)
	os.Exit(1)
}
