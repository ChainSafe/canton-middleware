//go:build ignore

// list-token-configs.go — Wildcard-list every CIP56.Config.TokenConfig contract
// visible on the connected Canton participant. Use this when mint-to-party.go's
// targeted lookup ("find TokenConfig for DEMO under issuer X") returns nothing
// and you need to discover (a) whether any TokenConfig exists at all, (b) which
// issuer party owns it, (c) the actual symbol stored in metadata.
//
// Mirrors the dial / auth pattern of scripts/remote/mint-to-party.go, but uses
// EventFormat.FiltersForAnyParty so the search isn't constrained to a single
// stakeholder. Requires the OAuth user to have can_read_as_any_party (or
// participant_admin) on the participant.
//
// Usage:
//
//	go run scripts/remote/list-token-configs.go \
//	  -config <local config file with prod1 OAuth + endpoints>
//
// Often invoked indirectly by scripts/remote/mint-demo-prod1.sh's debug fallback.

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
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	expcreds "google.golang.org/grpc/experimental/credentials"
	"google.golang.org/grpc/metadata"
)

var configPath = flag.String("config", "", "Path to API server config file (required)")

func main() {
	flag.Parse()
	if *configPath == "" {
		fmt.Println("ERROR: -config flag is required")
		flag.Usage()
		os.Exit(1)
	}

	cfg, err := config.LoadAPIServer(*configPath)
	if err != nil {
		fatalf("Failed to load config: %v", err)
	}

	cip56Pkg := cfg.Canton.Token.CIP56PackageID
	if cip56Pkg == "" {
		fatalf("canton.token.cip56_package_id is required in config")
	}

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  List ALL CIP56.Config.TokenConfig contracts on this participant")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Printf("  Canton:    %s\n", cfg.Canton.Ledger.RPCURL)
	fmt.Printf("  cip56_pkg: %s\n", cip56Pkg)
	fmt.Println()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	conn, err := dialCanton(cfg)
	if err != nil {
		fatalf("Failed to connect to Canton: %v", err)
	}
	defer conn.Close()

	ctx, _, err = authContext(ctx, cfg.Canton)
	if err != nil {
		fatalf("Failed to get auth token: %v", err)
	}

	state := lapiv2.NewStateServiceClient(conn)

	endResp, err := state.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		fatalf("GetLedgerEnd failed: %v", err)
	}
	if endResp.Offset == 0 {
		fatalf("ledger is empty (offset 0)")
	}

	// Wildcard ACS query: every party the OAuth user can read as, every
	// TokenConfig template. Requires can_read_as_any_party on the user.
	stream, err := state.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: endResp.Offset,
		EventFormat: &lapiv2.EventFormat{
			FiltersForAnyParty: &lapiv2.Filters{
				Cumulative: []*lapiv2.CumulativeFilter{
					{
						IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
							TemplateFilter: &lapiv2.TemplateFilter{
								TemplateId: &lapiv2.Identifier{
									PackageId:  cip56Pkg,
									ModuleName: "CIP56.Config",
									EntityName: "TokenConfig",
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
		fatalf("GetActiveContracts failed: %v", err)
	}

	count := 0
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Stream usually ends with EOF or a benign close.
			break
		}
		ac := msg.GetActiveContract()
		if ac == nil {
			continue
		}
		count++
		ev := ac.CreatedEvent
		symbol := values.MetaSymbolFromRecord(ev.GetCreateArguments())
		fmt.Printf("──────────────────────────────────────────────────────────────────────\n")
		fmt.Printf("  ContractID:  %s\n", ev.ContractId)
		fmt.Printf("  Symbol:      %s\n", symbol)
		fmt.Printf("  Signatories: %v\n", ev.Signatories)
		fmt.Printf("  Observers:   %v\n", ev.Observers)
		if tid := ev.GetTemplateId(); tid != nil {
			fmt.Printf("  TemplateId:  %s:%s:%s\n", tid.PackageId, tid.ModuleName, tid.EntityName)
		}
	}

	fmt.Println("──────────────────────────────────────────────────────────────────────")
	fmt.Printf(">>> %d TokenConfig contract(s) found on this participant.\n", count)
	if count == 0 {
		fmt.Println()
		fmt.Println("    No TokenConfig contracts at all. Either:")
		fmt.Println("    - DEMO/PROMPT have never been bootstrapped here, OR")
		fmt.Println("    - the participant the api-server connects to differs from")
		fmt.Println("      the one where bootstrap-demo.go ran, OR")
		fmt.Println("    - the OAuth user lacks can_read_as_any_party.")
	}
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

func fatalf(format string, args ...interface{}) {
	fmt.Printf("ERROR: "+format+"\n", args...)
	os.Exit(1)
}
