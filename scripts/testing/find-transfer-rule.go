//go:build ignore

// find-transfer-rule.go — Search for TransferRule contracts visible to ANY party
// on our participant via filters_for_any_party wildcard query.

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

	cfgpkg "github.com/chainsafe/canton-middleware/pkg/config"
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	expcreds "google.golang.org/grpc/experimental/credentials"
	"google.golang.org/grpc/metadata"
)

var (
	configPath = flag.String("config", "config.api-server.devnet-test.yaml", "Config")
	pkg        = flag.String("pkg", "#utility-registry-app-v0", "Package (name or hex id)")
	module     = flag.String("module", "Utility.Registry.App.V0.Model.Transfer", "Module")
	entity     = flag.String("entity", "TransferRule", "Template entity name")
)

func main() {
	flag.Parse()
	cfg, err := cfgpkg.LoadAPIServer(*configPath)
	if err != nil {
		fatalf("config: %v", err)
	}
	ctx := context.Background()
	token, err := fetchOAuthToken(cfg.Canton.Ledger.Auth.TokenURL, cfg.Canton.Ledger.Auth.ClientID, cfg.Canton.Ledger.Auth.ClientSecret, cfg.Canton.Ledger.Auth.Audience)
	if err != nil {
		fatalf("oauth: %v", err)
	}
	target := cfg.Canton.Ledger.RPCURL
	if !strings.Contains(target, "://") {
		target = "dns:///" + target
	}
	var dialOpts []grpc.DialOption
	if cfg.Canton.Ledger.TLS != nil && cfg.Canton.Ledger.TLS.Enabled {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(expcreds.NewTLSWithALPNDisabled(&tls.Config{MinVersion: tls.VersionTLS12})))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	conn, err := grpc.NewClient(target, dialOpts...)
	if err != nil {
		fatalf("dial: %v", err)
	}
	defer conn.Close()
	authCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)

	stateClient := lapiv2.NewStateServiceClient(conn)
	endCtx, cancel := context.WithTimeout(authCtx, 30*time.Second)
	defer cancel()
	endResp, err := stateClient.GetLedgerEnd(endCtx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		fatalf("GetLedgerEnd: %v", err)
	}

	queryCtx, cancelQ := context.WithTimeout(authCtx, 60*time.Second)
	defer cancelQ()

	// Use filters_for_any_party — wildcard across all parties hosted on participant
	stream, err := stateClient.GetActiveContracts(queryCtx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: endResp.Offset,
		EventFormat: &lapiv2.EventFormat{
			FiltersForAnyParty: &lapiv2.Filters{
				Cumulative: []*lapiv2.CumulativeFilter{
					{
						IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
							TemplateFilter: &lapiv2.TemplateFilter{
								TemplateId: &lapiv2.Identifier{
									PackageId:  *pkg,
									ModuleName: *module,
									EntityName: *entity,
								},
								IncludeCreatedEventBlob: true,
							},
						},
					},
				},
			},
			Verbose: true,
		},
	})
	if err != nil {
		fatalf("GetActiveContracts: %v", err)
	}

	count := 0
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			fatalf("recv: %v", err)
		}
		ce := msg.GetActiveContract()
		if ce == nil || ce.CreatedEvent == nil {
			continue
		}
		count++
		fmt.Printf("\n=== %s #%d ===\n", *entity, count)
		fmt.Printf("CID:  %s\n", ce.CreatedEvent.ContractId)
		fmt.Printf("Pkg:  %s\n", ce.CreatedEvent.TemplateId.PackageId)
		fmt.Printf("Blob: %d bytes (%s...)\n", len(ce.CreatedEvent.CreatedEventBlob), base64.StdEncoding.EncodeToString(ce.CreatedEvent.CreatedEventBlob)[:60])
		if ce.CreatedEvent.CreateArguments != nil {
			for _, f := range ce.CreatedEvent.CreateArguments.Fields {
				v := summarizeValue(f.Value)
				if len(v) > 200 {
					v = v[:200] + "..."
				}
				fmt.Printf("  %-20s = %s\n", f.Label, v)
			}
		}
	}
	fmt.Printf("\n>>> Total %s contracts visible to participant: %d\n", *entity, count)
}

func summarizeValue(v *lapiv2.Value) string {
	if v == nil {
		return "<nil>"
	}
	switch x := v.Sum.(type) {
	case *lapiv2.Value_Text:
		return x.Text
	case *lapiv2.Value_Numeric:
		return x.Numeric
	case *lapiv2.Value_Party:
		return x.Party
	case *lapiv2.Value_ContractId:
		return x.ContractId
	case *lapiv2.Value_Bool:
		return fmt.Sprintf("%v", x.Bool)
	case *lapiv2.Value_Optional:
		if x.Optional == nil || x.Optional.Value == nil {
			return "None"
		}
		return "Some(" + summarizeValue(x.Optional.Value) + ")"
	case *lapiv2.Value_Record:
		if x.Record == nil {
			return "{}"
		}
		fs := values.RecordToMap(x.Record)
		var parts []string
		for k, v := range fs {
			parts = append(parts, fmt.Sprintf("%s=%s", k, summarizeValue(v)))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		return fmt.Sprintf("<%T>", x)
	}
}

func fetchOAuthToken(tokenURL, clientID, clientSecret, audience string) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"client_id": clientID, "client_secret": clientSecret, "audience": audience, "grant_type": "client_credentials",
	})
	resp, err := http.Post(tokenURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token %d: %s", resp.StatusCode, string(respBody))
	}
	var tr struct {
		AccessToken string `json:"access_token"`
	}
	json.Unmarshal(respBody, &tr)
	return tr.AccessToken, nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FATAL: "+format+"\n", args...)
	os.Exit(1)
}
