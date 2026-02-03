//go:build ignore

// Archive old FingerprintMapping contracts from old common package
// This allows the relayer to create new mappings with the updated package

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

const (
	// Old common package ID that needs to be archived
	oldCommonPackageID = "03f7efaff2e596920c58be98cc60514ee108fcb5c2985f37c83380be9c7c7da2"

	// Canton connection
	cantonRPC = "canton-ledger-api-grpc-dev1.chainsafe.dev:80"
	party     = "daml-autopilot::1220096316d4ea75c021d89123cfd2792cfeac80dfbf90bfbca21bcd8bf1bb40d84c"

	// OAuth
	tokenURL     = "https://dev-2j3m40ajwym1zzaq.eu.auth0.com/oauth/token"
	clientID     = "nKMdSdj49c2BoPDynr6kf3pkLsTghePa"
	clientSecret = os.Getenv("CANTON_AUTH_CLIENT_SECRET") // Set via environment variable
	audience     = "https://canton-ledger-api-dev1.01.chainsafe.dev"
)

func main() {
	ctx := context.Background()

	// Get OAuth token
	token := getToken()

	// Connect to Canton
	conn, err := grpc.Dial(cantonRPC, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)

	stateClient := lapiv2.NewStateServiceClient(conn)
	cmdClient := lapiv2.NewCommandServiceClient(conn)

	// Get ledger end
	ctx2, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	endResp, err := stateClient.GetLedgerEnd(ctx2, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		panic(fmt.Errorf("failed to get ledger end: %v", err))
	}

	fmt.Printf("Ledger end: %d\n", endResp.Offset)
	fmt.Printf("Looking for contracts from old package: %s...\n\n", oldCommonPackageID[:16])

	// Find all FingerprintAuth contracts from old package using wildcard
	type contractInfo struct {
		ID     string
		Module string
		Entity string
	}
	var contractsToArchive []contractInfo

	ctx3, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	resp, err := stateClient.GetActiveContracts(ctx3, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: endResp.Offset,
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
		panic(fmt.Errorf("error querying contracts: %v", err))
	}

	for {
		msg, err := resp.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			c := contract.CreatedEvent
			// Only collect contracts from the old package that are FingerprintAuth templates
			if c.TemplateId.PackageId == oldCommonPackageID &&
				strings.Contains(c.TemplateId.ModuleName, "FingerprintAuth") {
				contractsToArchive = append(contractsToArchive, contractInfo{
					ID:     c.ContractId,
					Module: c.TemplateId.ModuleName,
					Entity: c.TemplateId.EntityName,
				})
			}
		}
	}

	fmt.Printf("Found %d contracts from old package to archive\n", len(contractsToArchive))

	if len(contractsToArchive) == 0 {
		fmt.Println("\nNo contracts from old package found!")
		return
	}

	fmt.Printf("\n=== Archiving %d contracts ===\n", len(contractsToArchive))

	// Get JWT subject from token
	jwtSubject := "nKMdSdj49c2BoPDynr6kf3pkLsTghePa@clients"
	domainID := "global-domain::1220be58c29e65de40bf273be1dc2b266d43a9a002ea5b18955aeef7aac881bb471a"

	// Archive each contract
	for i, c := range contractsToArchive {
		archiveArg := &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: &lapiv2.Record{}}}

		cmd := &lapiv2.Command{
			Command: &lapiv2.Command_Exercise{
				Exercise: &lapiv2.ExerciseCommand{
					TemplateId: &lapiv2.Identifier{
						PackageId:  oldCommonPackageID,
						ModuleName: c.Module,
						EntityName: c.Entity,
					},
					ContractId:     c.ID,
					Choice:         "Archive",
					ChoiceArgument: archiveArg,
				},
			},
		}

		commands := &lapiv2.Commands{
			SynchronizerId: domainID,
			CommandId:      fmt.Sprintf("archive-%d-%d", time.Now().UnixNano(), i),
			UserId:         jwtSubject,
			ActAs:          []string{party},
			Commands:       []*lapiv2.Command{cmd},
		}

		ctx4, cancel := context.WithTimeout(ctx, 30*time.Second)
		_, err := cmdClient.SubmitAndWaitForTransaction(ctx4, &lapiv2.SubmitAndWaitForTransactionRequest{
			Commands: commands,
		})
		cancel()

		if err != nil {
			fmt.Printf("[%d] Failed to archive %s %s: %v\n", i+1, c.Entity, c.ID[:20], err)
		} else {
			fmt.Printf("[%d] Archived %s %s...\n", i+1, c.Entity, c.ID[:20])
		}
	}

	fmt.Println("\nDone! Old contracts archived.")
	fmt.Println("The relayer will now be able to create new FingerprintMapping contracts with the updated package.")
}

func getToken() string {
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
	return tokenResp["access_token"].(string)
}
