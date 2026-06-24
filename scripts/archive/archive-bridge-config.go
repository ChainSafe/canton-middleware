//go:build ignore

// archive-bridge-config.go — Surgically archive a single WayfinderBridgeConfig.
//
// This is a narrowly-scoped alternative to scripts/archive/archive-cip56.go, which
// archives a broad set (CIP56Holding balances, TokenConfig, FingerprintMapping, ...).
// This script ONLY ever queries and archives the Wayfinder.Bridge:WayfinderBridgeConfig
// template — it cannot touch holdings, token configs, or fingerprint mappings.
//
// Use it to retire a stale WayfinderBridgeConfig (e.g. one bound to an old
// bridge-wayfinder package) so bootstrap-bridge creates a fresh one and the relayer
// binds to the freshly-uploaded package.
//
// Usage:
//
//	# 1) List every WayfinderBridgeConfig for the issuer, with its package id:
//	go run scripts/archive/archive-bridge-config.go -config <api-server.yaml>
//
//	# 2) Archive ONLY the stale one, by contract id (dry-run prints the plan):
//	go run scripts/archive/archive-bridge-config.go -config <api-server.yaml> -cid <contract-id>
//	go run scripts/archive/archive-bridge-config.go -config <api-server.yaml> -cid <contract-id> -archive
//
// Flags:
//
//	-config   path to the api-server config (provides issuer party, ledger conn, auth)
//	-cid      the exact WayfinderBridgeConfig contract id to archive (required to archive)
//	-archive  actually archive (default is a dry run)
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/ledger"
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"
	"github.com/chainsafe/canton-middleware/pkg/config"
)

// The only template this script ever touches.
const (
	bridgeModule = "Wayfinder.Bridge"
	bridgeEntity = "WayfinderBridgeConfig"
	// Package-name reference: matches every deployed bridge-wayfinder version, so
	// both the old and new config show up in the listing.
	bridgePackageNameRef = "#bridge-wayfinder"
)

var (
	tokenMu     sync.Mutex
	cachedToken string
	tokenExpiry time.Time
	jwtSubject  string
)

type bridgeConfig struct {
	ContractID     string
	PackageID      string
	Issuer         string
	TokenConfigCID string
}

func main() {
	configPath := flag.String("config", "", "Path to the api-server config (required)")
	cid := flag.String("cid", "", "Exact WayfinderBridgeConfig contract id to archive")
	doArchive := flag.Bool("archive", false, "Actually archive (default: dry run)")
	flag.Parse()

	if *configPath == "" {
		flag.Usage()
		log.Fatal("-config is required")
	}

	cfg, err := config.LoadAPIServer(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	issuer := cfg.Canton.IssuerParty
	domainID := cfg.Canton.DomainID

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	conn, err := dial(cfg)
	if err != nil {
		log.Fatalf("connect to Canton: %v", err)
	}
	defer conn.Close()

	ctx, err = getAuthContext(ctx, cfg.Canton.Ledger.Auth)
	if err != nil {
		log.Fatalf("auth: %v", err)
	}

	stateService := lapiv2.NewStateServiceClient(conn)

	// Always list first — this is the only template queried, so nothing else can
	// ever be selected.
	configs, err := listBridgeConfigs(ctx, stateService, issuer)
	if err != nil {
		log.Fatalf("list WayfinderBridgeConfig: %v", err)
	}

	fmt.Printf("Issuer:  %s\n", issuer)
	fmt.Printf("Found %d active WayfinderBridgeConfig contract(s):\n\n", len(configs))
	for _, c := range configs {
		marker := ""
		if *cid != "" && c.ContractID == *cid {
			marker = "  <-- selected for archive"
		}
		fmt.Printf("  contract_id:     %s%s\n", c.ContractID, marker)
		fmt.Printf("  package_id:      %s\n", c.PackageID)
		fmt.Printf("  tokenConfigCid:  %s\n", c.TokenConfigCID)
		fmt.Println()
	}

	if *cid == "" {
		fmt.Println("No -cid given: listing only. Re-run with -cid <id> (and -archive) to archive one.")
		return
	}

	// Guard: the cid must be one of the listed WayfinderBridgeConfigs. Since we only
	// ever query that template, this makes it impossible to archive anything else.
	var target *bridgeConfig
	for i := range configs {
		if configs[i].ContractID == *cid {
			target = &configs[i]
			break
		}
	}
	if target == nil {
		log.Fatalf("contract id %q is not an active WayfinderBridgeConfig for issuer %s; refusing to archive", *cid, issuer)
	}

	if !*doArchive {
		fmt.Printf("DRY RUN: would archive WayfinderBridgeConfig %s (package %s)\n", target.ContractID, target.PackageID)
		fmt.Println("Re-run with -archive to perform it.")
		return
	}

	commandService := lapiv2.NewCommandServiceClient(conn)
	if err := archiveBridgeConfig(ctx, commandService, issuer, domainID, target); err != nil {
		log.Fatalf("archive failed: %v", err)
	}
	fmt.Printf("✓ Archived WayfinderBridgeConfig %s\n", target.ContractID)
	fmt.Println("Next: re-run bootstrap-bridge so a fresh config is created under the new package.")
}

func dial(cfg *config.APIServer) (*grpc.ClientConn, error) {
	var opts []grpc.DialOption
	if cfg.Canton.Ledger.TLS != nil && cfg.Canton.Ledger.TLS.Enabled {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	target := cfg.Canton.Ledger.RPCURL
	if !strings.Contains(target, "://") {
		target = "dns:///" + target
	}
	return grpc.NewClient(target, opts...)
}

func listBridgeConfigs(ctx context.Context, client lapiv2.StateServiceClient, party string) ([]bridgeConfig, error) {
	endResp, err := client.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		return nil, fmt.Errorf("get ledger end: %w", err)
	}
	if endResp.Offset == 0 {
		return nil, nil
	}

	resp, err := client.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: endResp.Offset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				party: {
					Cumulative: []*lapiv2.CumulativeFilter{{
						IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
							TemplateFilter: &lapiv2.TemplateFilter{
								TemplateId: &lapiv2.Identifier{
									PackageId:  bridgePackageNameRef,
									ModuleName: bridgeModule,
									EntityName: bridgeEntity,
								},
							},
						},
					}},
				},
			},
			Verbose: true,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get active contracts: %w", err)
	}

	var out []bridgeConfig
	for {
		msg, err := resp.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("recv active contracts: %w", err)
		}
		ac := msg.GetActiveContract()
		if ac == nil || ac.CreatedEvent == nil {
			continue
		}
		ce := ac.CreatedEvent
		bc := bridgeConfig{ContractID: ce.ContractId}
		if tid := ce.GetTemplateId(); tid != nil {
			bc.PackageID = tid.PackageId
		}
		fields := values.RecordToMap(ce.CreateArguments)
		bc.Issuer = values.Party(fields["issuer"])
		bc.TokenConfigCID = values.ContractID(fields["tokenConfigCid"])
		out = append(out, bc)
	}
	return out, nil
}

func archiveBridgeConfig(ctx context.Context, client lapiv2.CommandServiceClient, actAs, domainID string, c *bridgeConfig) error {
	archiveArg := &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: &lapiv2.Record{
		RecordId: &lapiv2.Identifier{PackageId: c.PackageID, ModuleName: "DA.Internal.Template", EntityName: "Archive"},
		Fields:   []*lapiv2.RecordField{},
	}}}

	cmd := &lapiv2.Command{Command: &lapiv2.Command_Exercise{Exercise: &lapiv2.ExerciseCommand{
		TemplateId:     &lapiv2.Identifier{PackageId: c.PackageID, ModuleName: bridgeModule, EntityName: bridgeEntity},
		ContractId:     c.ContractID,
		Choice:         "Archive",
		ChoiceArgument: archiveArg,
	}}}

	_, err := client.SubmitAndWaitForTransaction(ctx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: domainID,
			CommandId:      fmt.Sprintf("archive-bridge-config-%d", time.Now().UnixNano()),
			UserId:         jwtSubject,
			ActAs:          []string{actAs},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	return err
}

func getAuthContext(ctx context.Context, auth *ledger.AuthConfig) (context.Context, error) {
	if auth == nil || auth.ClientID == "" || auth.ClientSecret == "" || auth.Audience == "" || auth.TokenURL == "" {
		return ctx, nil
	}
	token, err := getOAuthToken(auth)
	if err != nil {
		return nil, err
	}
	return metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token)), nil
}

func getOAuthToken(auth *ledger.AuthConfig) (string, error) {
	tokenMu.Lock()
	defer tokenMu.Unlock()

	if cachedToken != "" && time.Now().Before(tokenExpiry) {
		return cachedToken, nil
	}

	body, err := json.Marshal(map[string]string{
		"client_id":     auth.ClientID,
		"client_secret": auth.ClientSecret,
		"audience":      auth.Audience,
		"grant_type":    "client_credentials",
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest("POST", auth.TokenURL, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OAuth failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &tok); err != nil {
		return "", err
	}
	cachedToken = tok.AccessToken
	tokenExpiry = time.Now().Add(time.Duration(tok.ExpiresIn-60) * time.Second)
	if sub, err := extractJWTSubject(tok.AccessToken); err == nil {
		jwtSubject = sub
	}
	return tok.AccessToken, nil
}

func extractJWTSubject(tokenString string) (string, error) {
	t, _, err := jwt.NewParser().ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return "", err
	}
	claims, ok := t.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("invalid JWT claims")
	}
	sub, ok := claims["sub"].(string)
	if !ok {
		return "", fmt.Errorf("JWT missing 'sub'")
	}
	return sub, nil
}
