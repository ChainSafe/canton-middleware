//go:build ignore

// recreate-fingerprint-mappings.go — Recreate FingerprintMapping contracts under the
// current (NEW) issuer party after canton.issuer_party was changed. This restores
// fingerprint→party resolution for every user, which is what brings back USDCx
// (external-token) balances and transfers — those were never stranded, only the
// lookup broke.
//
// It wildcard-lists every FingerprintMapping on the participant and re-creates each
// one whose issuer is NOT the current issuer, under the current issuer, with the SAME
// user party, fingerprint and EVM address. The old issuer is read off each existing
// contract, so no old-issuer party is needed.
//
// Scope is restricted to parties that exist in the userstore DB. There can be more
// FingerprintMapping contracts on-ledger (demo/test/orphan parties from bootstrap or
// shared ledgers) than real registered users, and we only want to re-anchor mappings
// for actual users — so a mapping whose userParty is absent from the DB is skipped.
//
// The config's canton.issuer_party MUST be the current/new issuer:
// CreateFingerprintMapping runs ActAs=that party. Reading every mapping relies on the
// OAuth user's can_read_as_any_party right (same right the mint/list tooling uses).
// The config's database.* must point at the userstore DB.
//
// Migrates every DB user in one run. Idempotent: a fingerprint already mapped under
// the current issuer is skipped, so re-runs are safe. Use -party to scope to one user.
//
// Usage:
//
//	go run scripts/remote/recreate-fingerprint-mappings.go \
//	  -config <config.yaml> \   # canton.issuer_party = current/new issuer
//	  [-party <user-party>] \   # optional: only this user
//	  [-apply]                  # without this it's a dry run

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	cantonclient "github.com/chainsafe/canton-middleware/pkg/cantonsdk/client"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/identity"
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/log"
	"github.com/chainsafe/canton-middleware/pkg/pgutil"
	"github.com/chainsafe/canton-middleware/pkg/userstore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	expcreds "google.golang.org/grpc/experimental/credentials"
	"google.golang.org/grpc/metadata"
)

var (
	configPath = flag.String("config", "", "Path to API server config file (required); canton.issuer_party must be the current/new issuer")
	userParty  = flag.String("party", "", "Optional: only recreate the mapping for this user party (default: all users)")
	apply      = flag.Bool("apply", false, "Perform creates. Without this flag it only reports (dry run).")
)

type mapping struct {
	issuer      string
	userParty   string
	fingerprint string
	evmAddress  string
}

func main() {
	flag.Parse()
	if *configPath == "" {
		fatalf("-config is required")
	}

	cfg, err := config.LoadAPIServer(*configPath)
	if err != nil {
		fatalf("load config: %v", err)
	}
	newIssuer := cfg.Canton.IssuerParty
	if newIssuer == "" {
		fatalf("canton.issuer_party is required in config (must be the current/new issuer)")
	}
	pkgID := cfg.Canton.Identity.PackageID
	if pkgID == "" {
		fatalf("canton.identity.package_id is required in config")
	}

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Recreate FingerprintMappings under the current issuer")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Printf("  Canton:      %s\n", cfg.Canton.Ledger.RPCURL)
	fmt.Printf("  New issuer:  %s\n", newIssuer)
	if *userParty != "" {
		fmt.Printf("  Party:       %s\n", *userParty)
	} else {
		fmt.Printf("  Scope:       ALL users\n")
	}
	fmt.Printf("  Mode:        %s\n", modeLabel(*apply))
	fmt.Println()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// SDK client (current issuer) — used to create the new mappings.
	logger, err := log.NewLogger(cfg.Logging)
	if err != nil {
		fatalf("init logger: %v", err)
	}
	client, err := cantonclient.New(ctx, cfg.Canton, cantonclient.WithLogger(logger))
	if err != nil {
		fatalf("connect to Canton: %v", err)
	}
	defer func() { _ = client.Close() }()

	// Raw connection — wildcard-list every FingerprintMapping (any party as stakeholder).
	conn, err := dialCanton(cfg)
	if err != nil {
		fatalf("dial Canton: %v", err)
	}
	defer conn.Close()
	rctx, _, err := authContext(ctx, cfg.Canton)
	if err != nil {
		fatalf("auth: %v", err)
	}
	all, err := listAllMappings(rctx, lapiv2.NewStateServiceClient(conn), pkgID)
	if err != nil {
		fatalf("list mappings: %v", err)
	}

	// Authoritative set of real users. On-ledger mappings can outnumber registered
	// users (demo/test/orphan parties), and we only re-anchor mappings for parties
	// that exist in the userstore DB.
	bunDB, err := pgutil.ConnectDB(cfg.Database)
	if err != nil {
		fatalf("connect to database: %v", err)
	}
	defer func() { _ = bunDB.Close() }()
	users, err := userstore.NewStore(bunDB).ListUsers(ctx)
	if err != nil {
		fatalf("list users: %v", err)
	}
	dbParties := make(map[string]bool, len(users))
	for _, u := range users {
		if u.CantonPartyID != "" {
			dbParties[u.CantonPartyID] = true
		}
	}
	fmt.Printf("  DB users:    %d\n", len(dbParties))
	fmt.Printf("  On-ledger:   %d mapping(s)\n\n", len(all))

	// Fingerprints already mapped under the current issuer → skip set.
	already := map[string]bool{}
	for _, m := range all {
		if m.issuer == newIssuer {
			already[normFP(m.fingerprint)] = true
		}
	}

	var created, skipped, notInDB int
	for _, m := range all {
		if m.issuer == newIssuer { // already current — nothing to do
			skipped++
			continue
		}
		if m.fingerprint == "" || m.userParty == "" {
			skipped++
			continue
		}
		if !dbParties[m.userParty] { // not a registered user — leave it alone
			notInDB++
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
		evm := m.evmAddress
		if evm == "" {
			evm = "(none)"
		}
		verb := "WOULD CREATE"
		if *apply {
			verb = "CREATING"
		}
		fmt.Printf("  - %s: fp=%s -> party=%s  evm=%s  (was issuer=%s)\n", verb, m.fingerprint, m.userParty, evm, m.issuer)
		if !*apply {
			already[normFP(m.fingerprint)] = true
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
		already[normFP(m.fingerprint)] = true
		created++
	}

	fmt.Println()
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	if *apply {
		fmt.Printf("  Done. Created %d mapping(s); %d skipped (already current / incomplete / filtered); %d not in DB.\n", created, skipped, notInDB)
	} else {
		fmt.Printf("  Dry run. Would create %d mapping(s); %d skipped; %d not in DB. Re-run with -apply.\n", created, skipped, notInDB)
	}
	fmt.Println("══════════════════════════════════════════════════════════════════════")
}

// listAllMappings wildcard-lists every Common.FingerprintAuth.FingerprintMapping
// contract on the participant (requires can_read_as_any_party).
func listAllMappings(ctx context.Context, state lapiv2.StateServiceClient, pkgID string) ([]mapping, error) {
	end, err := state.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		return nil, err
	}
	if end.Offset == 0 {
		return nil, fmt.Errorf("ledger is empty (offset 0)")
	}
	stream, err := state.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: end.Offset,
		EventFormat: &lapiv2.EventFormat{
			FiltersForAnyParty: &lapiv2.Filters{
				Cumulative: []*lapiv2.CumulativeFilter{
					{
						IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
							TemplateFilter: &lapiv2.TemplateFilter{
								TemplateId: &lapiv2.Identifier{
									PackageId:  pkgID,
									ModuleName: "Common.FingerprintAuth",
									EntityName: "FingerprintMapping",
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

	var out []mapping
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		ac := msg.GetActiveContract()
		if ac == nil {
			continue
		}
		f := values.RecordToMap(ac.CreatedEvent.GetCreateArguments())
		out = append(out, mapping{
			issuer:      values.Party(f["issuer"]),
			userParty:   values.Party(f["userParty"]),
			fingerprint: values.Text(f["fingerprint"]),
			// evmAddress is Optional EvmAddress, and EvmAddress is a newtype
			// (single-field record). values.Text can't decode that — unwrap the
			// Optional and the newtype record, else the address round-trips to None.
			evmAddress: values.Text(values.OptionalRecordFields(f["evmAddress"])["value"]),
		})
	}
	return out, nil
}

func dialCanton(cfg *config.APIServer) (*grpc.ClientConn, error) {
	var opts []grpc.DialOption
	if cfg.Canton.Ledger.TLS != nil && cfg.Canton.Ledger.TLS.Enabled {
		tlsConfig := &tls.Config{InsecureSkipVerify: true} //nolint:gosec
		opts = append(opts, grpc.WithTransportCredentials(expcreds.NewTLSWithALPNDisabled(tlsConfig)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	if cfg.Canton.Ledger.MaxMessageSize > 0 {
		opts = append(opts, grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(cfg.Canton.Ledger.MaxMessageSize)))
	}
	target := cfg.Canton.Ledger.RPCURL
	if !strings.Contains(target, "://") {
		target = "dns:///" + target
	}
	return grpc.NewClient(target, opts...)
}

func authContext(ctx context.Context, canton *cantonclient.Config) (context.Context, string, error) {
	if canton.Ledger.Auth == nil || canton.Ledger.Auth.ClientID == "" {
		return ctx, "", nil
	}
	payload := map[string]string{
		"client_id":     canton.Ledger.Auth.ClientID,
		"client_secret": canton.Ledger.Auth.ClientSecret,
		"audience":      canton.Ledger.Auth.Audience,
		"grant_type":    "client_credentials",
	}
	body, _ := json.Marshal(payload)
	resp, err := http.Post(canton.Ledger.Auth.TokenURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("token endpoint %d: %s", resp.StatusCode, respBody)
	}
	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return nil, "", err
	}
	var sub string
	if parts := strings.Split(tokenResp.AccessToken, "."); len(parts) >= 2 {
		padded := parts[1]
		switch len(padded) % 4 {
		case 2:
			padded += "=="
		case 3:
			padded += "="
		}
		if decoded, err := base64.URLEncoding.DecodeString(padded); err == nil {
			var c struct {
				Sub string `json:"sub"`
			}
			_ = json.Unmarshal(decoded, &c)
			sub = c.Sub
		}
	}
	md := metadata.Pairs("authorization", "Bearer "+tokenResp.AccessToken)
	return metadata.NewOutgoingContext(ctx, md), sub, nil
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
