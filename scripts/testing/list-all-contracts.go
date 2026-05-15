//go:build ignore

// list-all-contracts.go — List ALL active contracts visible to a party (wildcard filter).
// Diagnostic to verify whether any contracts at all are on our participant for this party.

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
	"time"

	cfgpkg "github.com/chainsafe/canton-middleware/pkg/config"
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	expcreds "google.golang.org/grpc/experimental/credentials"
	"google.golang.org/grpc/metadata"
)

var (
	configPath = flag.String("config", "config.api-server.devnet-test.yaml", "Path to API server config file")
	partyID    = flag.String("party", "", "Canton party ID to query (required)")
	limit      = flag.Int("limit", 50, "Max contracts to print")
)

func main() {
	flag.Parse()
	if *partyID == "" {
		fatalf("-party flag is required")
	}

	cfg, err := cfgpkg.LoadAPIServer(*configPath)
	if err != nil {
		fatalf("Failed to load config: %v", err)
	}

	ctx := context.Background()

	token, err := fetchOAuthToken(
		cfg.Canton.Ledger.Auth.TokenURL,
		cfg.Canton.Ledger.Auth.ClientID,
		cfg.Canton.Ledger.Auth.ClientSecret,
		cfg.Canton.Ledger.Auth.Audience,
	)
	if err != nil {
		fatalf("OAuth: %v", err)
	}

	target := cfg.Canton.Ledger.RPCURL
	if !strings.Contains(target, "://") {
		target = "dns:///" + target
	}

	var dialOpts []grpc.DialOption
	if cfg.Canton.Ledger.TLS != nil && cfg.Canton.Ledger.TLS.Enabled {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(
			expcreds.NewTLSWithALPNDisabled(&tls.Config{MinVersion: tls.VersionTLS12}),
		))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	conn, err := grpc.NewClient(target, dialOpts...)
	if err != nil {
		fatalf("dial: %v", err)
	}
	defer conn.Close()

	authCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)

	// First get ledger end
	stateClient := lapiv2.NewStateServiceClient(conn)
	endCtx, cancelEnd := context.WithTimeout(authCtx, 30*time.Second)
	defer cancelEnd()
	endResp, err := stateClient.GetLedgerEnd(endCtx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		fatalf("GetLedgerEnd: %v", err)
	}

	fmt.Printf(">>> Ledger end: %d\n", endResp.Offset)

	// Wildcard query — all contracts where this party is a stakeholder
	queryCtx, cancelQuery := context.WithTimeout(authCtx, 60*time.Second)
	defer cancelQuery()

	stream, err := stateClient.GetActiveContracts(queryCtx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: endResp.Offset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				*partyID: {
					Cumulative: []*lapiv2.CumulativeFilter{
						{
							IdentifierFilter: &lapiv2.CumulativeFilter_WildcardFilter{
								WildcardFilter: &lapiv2.WildcardFilter{
									IncludeCreatedEventBlob: false,
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
		fatalf("GetActiveContracts: %v", err)
	}

	count := 0
	templateCounts := map[string]int{}
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			fatalf("stream recv: %v", err)
		}
		entry := msg.GetActiveContract()
		if entry == nil {
			continue
		}
		ce := entry.CreatedEvent
		if ce == nil {
			continue
		}
		count++
		key := ""
		if ce.TemplateId != nil {
			key = fmt.Sprintf("%s:%s", ce.TemplateId.ModuleName, ce.TemplateId.EntityName)
		} else {
			key = "(no template)"
		}
		templateCounts[key]++

		if count <= *limit {
			fmt.Printf("\n#%d %s\n  CID:  %s\n", count, key, ce.ContractId)
			if ce.TemplateId != nil {
				fmt.Printf("  Pkg:  %s\n", ce.TemplateId.PackageId)
			}
			if ce.CreateArguments != nil {
				for _, f := range ce.CreateArguments.Fields {
					fmt.Printf("  Arg:  %-18s = %s\n", f.Label, summarizeValue(f.Value))
				}
			}
		}
	}

	fmt.Printf("\n>>> Total contracts visible to %s: %d\n", *partyID, count)
	fmt.Println("\nTemplate breakdown:")
	for k, v := range templateCounts {
		fmt.Printf("  %s: %d\n", k, v)
	}
}

func fetchOAuthToken(tokenURL, clientID, clientSecret, audience string) (string, error) {
	payload := map[string]string{
		"client_id":     clientID,
		"client_secret": clientSecret,
		"audience":      audience,
		"grant_type":    "client_credentials",
	}
	body, _ := json.Marshal(payload)
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
	if err := json.Unmarshal(respBody, &tr); err != nil {
		return "", err
	}
	return tr.AccessToken, nil
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
	case *lapiv2.Value_Int64:
		return fmt.Sprintf("%d", x.Int64)
	case *lapiv2.Value_Bool:
		return fmt.Sprintf("%v", x.Bool)
	case *lapiv2.Value_Party:
		return x.Party
	case *lapiv2.Value_ContractId:
		return x.ContractId
	case *lapiv2.Value_Timestamp:
		return fmt.Sprintf("%d", x.Timestamp)
	case *lapiv2.Value_Date:
		return fmt.Sprintf("%d", x.Date)
	case *lapiv2.Value_Optional:
		if x.Optional == nil || x.Optional.Value == nil {
			return "None"
		}
		return "Some(" + summarizeValue(x.Optional.Value) + ")"
	case *lapiv2.Value_Record:
		if x.Record == nil {
			return "<empty record>"
		}
		var parts []string
		for _, f := range x.Record.Fields {
			parts = append(parts, fmt.Sprintf("%s=%s", f.Label, summarizeValue(f.Value)))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case *lapiv2.Value_List:
		if x.List == nil {
			return "[]"
		}
		var parts []string
		for _, e := range x.List.Elements {
			parts = append(parts, summarizeValue(e))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		return fmt.Sprintf("<%T>", x)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FATAL: "+format+"\n", args...)
	os.Exit(1)
}
