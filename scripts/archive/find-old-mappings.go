//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
)

func main() {
	ctx := context.Background()

	// Get OAuth token
	tokenURL := "https://dev-2j3m40ajwym1zzaq.eu.auth0.com/oauth/token"
	clientID := "nKMdSdj49c2BoPDynr6kf3pkLsTghePa"
	clientSecret := os.Getenv("CANTON_AUTH_CLIENT_SECRET") // Set via environment variable
	audience := "https://canton-ledger-api-dev1.01.chainsafe.dev"

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("audience", audience)

	resp, err := http.Post(tokenURL, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var tokenResp map[string]interface{}
	json.Unmarshal(body, &tokenResp)
	token := tokenResp["access_token"].(string)

	// Connect to Canton (port 80 - TLS termination at load balancer)
	conn, err := grpc.Dial("canton-ledger-api-grpc-dev1.chainsafe.dev:80", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// Query active contracts with template name containing FingerprintMapping
	client := lapiv2.NewStateServiceClient(conn)
	party := "daml-autopilot::1220096316d4ea75c021d89123cfd2792cfeac80dfbf90bfbca21bcd8bf1bb40d84c"

	fmt.Println("Querying for FingerprintMapping and PendingDeposit contracts...")

	// Old package ID from error: 03f7efaf... (full ID needed)
	// Let's query with a wildcard to find all FingerprintAuth contracts
	// The old package is common-v2, let's search for the template specifically

	// First get ledger end
	endResp, err := client.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		panic(fmt.Errorf("failed to get ledger end: %v", err))
	}
	activeAtOffset := endResp.Offset

	// Query for FingerprintMapping template
	resp2, err := client.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: activeAtOffset,
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
		},
	})
	if err != nil {
		panic(err)
	}

	count := 0
	for {
		msg, err := resp2.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			c := contract.CreatedEvent
			if c != nil && strings.Contains(c.TemplateId.ModuleName, "FingerprintAuth") {
				count++
				fmt.Printf("\n[%d] Contract: %s\n", count, c.ContractId)
				fmt.Printf("    Template: %s:%s\n", c.TemplateId.ModuleName, c.TemplateId.EntityName)
				fmt.Printf("    Package: %s\n", c.TemplateId.PackageId)
				// Print payload to see fingerprint
				if c.CreateArguments != nil && len(c.CreateArguments.Fields) > 0 {
					for _, f := range c.CreateArguments.Fields {
						if f.Label == "userFingerprint" || f.Label == "fingerprint" {
							fmt.Printf("    Fingerprint: %v\n", f.Value)
						}
					}
				}
			}
		}
	}
	fmt.Printf("\nTotal FingerprintAuth contracts: %d\n", count)
}
