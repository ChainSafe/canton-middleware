//go:build ignore

// list-cip56-holdings.go — Wildcard-list every CIP56.Token.CIP56Holding contract
// visible on the connected Canton participant. Sister of list-token-configs.go
// for verifying that a mint actually produced a Holding on-ledger (rather than
// trusting only the SubmitAndWaitForTransaction response).
//
// Prints: ContractID, Signatories, Observers, TemplateId, plus the full Daml
// CreateArguments record (which contains owner, instrument admin, amount).
//
// Usage:
//
//	go run scripts/remote/list-cip56-holdings.go \
//	  -config <local config file>

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
	"github.com/chainsafe/canton-middleware/pkg/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	expcreds "google.golang.org/grpc/experimental/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
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
	fmt.Println("  List ALL CIP56.Token.CIP56Holding contracts on this participant")
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

	// CIP56Holding template lives under CIP56.Token (Daml module Token in
	// the cip56-token package), unlike TokenConfig which is in CIP56.Config.
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
									ModuleName: "CIP56.Token",
									EntityName: "CIP56Holding",
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

	marshaler := protojson.MarshalOptions{Indent: "    ", EmitUnpopulated: false}
	count := 0
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
		count++
		ev := ac.CreatedEvent
		fmt.Printf("──────────────────────────────────────────────────────────────────────\n")
		fmt.Printf("  ContractID:  %s\n", ev.ContractId)
		fmt.Printf("  Signatories: %v\n", ev.Signatories)
		fmt.Printf("  Observers:   %v\n", ev.Observers)
		if tid := ev.GetTemplateId(); tid != nil {
			fmt.Printf("  TemplateId:  %s:%s:%s\n", tid.PackageId, tid.ModuleName, tid.EntityName)
		}
		if args := ev.GetCreateArguments(); args != nil {
			argsJSON, err := marshaler.Marshal(args)
			if err == nil {
				fmt.Printf("  CreateArguments:\n    %s\n", string(argsJSON))
			}
		}
	}

	fmt.Println("──────────────────────────────────────────────────────────────────────")
	fmt.Printf(">>> %d CIP56Holding contract(s) found on this participant.\n", count)
	if count == 0 {
		fmt.Println()
		fmt.Println("    No CIP56Holding contracts visible. Either:")
		fmt.Println("    - DEMO has never been minted on this participant, OR")
		fmt.Println("    - the OAuth user lacks can_read_as_any_party, OR")
		fmt.Println("    - the CIP56Holding template module path differs in this deployment.")
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
