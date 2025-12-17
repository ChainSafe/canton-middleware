//go:build ignore

// initiate-withdrawal.go - Initiate a withdrawal from Canton to EVM
//
// Usage:
//   go run scripts/initiate-withdrawal.go -config config.yaml \
//     -holding-cid "00..." \
//     -amount "50.0" \
//     -evm-destination "0x70997970C51812dc3A010C7d01b50e0d17dc79C8"

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
	"sync"
	"time"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

var (
	iwConfigPath     = flag.String("config", "config.yaml", "Path to config file")
	iwHoldingCid     = flag.String("holding-cid", "", "Contract ID of the CIP56Holding to withdraw from")
	iwAmount         = flag.String("amount", "", "Amount to withdraw (e.g., '50.0')")
	iwEvmDestination = flag.String("evm-destination", "", "EVM address to receive the tokens")
)

var (
	iwTokenMu     sync.Mutex
	iwCachedToken string
	iwTokenExpiry time.Time
	iwJwtSubject  string
)

func main() {
	flag.Parse()

	if *iwHoldingCid == "" || *iwAmount == "" || *iwEvmDestination == "" {
		fmt.Println("Error: -holding-cid, -amount, and -evm-destination are all required")
		fmt.Println("Usage: go run scripts/initiate-withdrawal.go -config config.yaml \\")
		fmt.Println("         -holding-cid '00...' -amount '50.0' -evm-destination '0x...'")
		os.Exit(1)
	}

	if !strings.HasPrefix(*iwEvmDestination, "0x") || len(*iwEvmDestination) != 42 {
		fmt.Println("Error: Invalid EVM destination address. Must be 0x followed by 40 hex chars.")
		os.Exit(1)
	}

	cfg, err := config.Load(*iwConfigPath)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("======================================================================")
	fmt.Println("INITIATE WITHDRAWAL - Canton to EVM")
	fmt.Println("======================================================================")
	fmt.Printf("Holding CID:     %s\n", *iwHoldingCid)
	fmt.Printf("Amount:          %s\n", *iwAmount)
	fmt.Printf("EVM Destination: %s\n", *iwEvmDestination)
	fmt.Printf("Relayer Party:   %s\n", cfg.Canton.RelayerParty)
	fmt.Println()

	ctx := context.Background()

	var opts []grpc.DialOption
	if cfg.Canton.TLS.Enabled {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{"h2"},
		}
		creds := credentials.NewTLS(tlsConfig)
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(cfg.Canton.RPCURL, opts...)
	if err != nil {
		fmt.Printf("Failed to connect to Canton: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	ctx, err = iwGetAuthContext(ctx, &cfg.Canton.Auth)
	if err != nil {
		fmt.Printf("Failed to get auth context: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("JWT Subject: %s\n\n", iwJwtSubject)

	stateClient := lapiv2.NewStateServiceClient(conn)
	cmdClient := lapiv2.NewCommandServiceClient(conn)

	ledgerEndResp, err := stateClient.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		fmt.Printf("Failed to get ledger end: %v\n", err)
		os.Exit(1)
	}
	if ledgerEndResp.Offset == 0 {
		fmt.Println("Error: Ledger is empty.")
		os.Exit(1)
	}

	fmt.Println(">>> Finding WayfinderBridgeConfig...")
	configCid, err := iwFindBridgeConfig(ctx, stateClient, cfg.Canton.RelayerParty, cfg.Canton.BridgePackageID, ledgerEndResp.Offset)
	if err != nil {
		fmt.Printf("Failed to find WayfinderBridgeConfig: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("    Config CID: %s\n\n", configCid)

	fmt.Println(">>> Finding holding owner and FingerprintMapping...")
	owner, err := iwGetHoldingOwner(ctx, stateClient, cfg.Canton.RelayerParty, ledgerEndResp.Offset, *iwHoldingCid)
	if err != nil {
		fmt.Printf("Failed to get holding owner: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("    Holding Owner: %s\n", owner)

	// Look up FingerprintMapping by canton party (owner), not by fingerprint extracted from party ID
	mappingCid, err := iwFindFingerprintMapping(ctx, stateClient, cfg.Canton.RelayerParty, ledgerEndResp.Offset, owner)
	if err != nil {
		fmt.Printf("Failed to find FingerprintMapping: %v\n", err)
		fmt.Println("\nUser must be registered first. Run:")
		fmt.Printf("  go run scripts/register-user.go -config config.yaml -party '%s'\n", owner)
		os.Exit(1)
	}
	fmt.Printf("    Mapping CID: %s\n\n", mappingCid)

	fmt.Println(">>> Getting domain ID...")
	domainID := cfg.Canton.DomainID
	if domainID == "" {
		domainResp, err := stateClient.GetConnectedSynchronizers(ctx, &lapiv2.GetConnectedSynchronizersRequest{
			Party: cfg.Canton.RelayerParty,
		})
		if err != nil {
			fmt.Printf("Failed to get domain ID: %v\n", err)
			os.Exit(1)
		}
		if len(domainResp.ConnectedSynchronizers) == 0 {
			fmt.Println("Error: No connected synchronizers")
			os.Exit(1)
		}
		domainID = domainResp.ConnectedSynchronizers[0].SynchronizerId
	}
	fmt.Printf("    Domain ID: %s\n\n", domainID)

	fmt.Println(">>> Initiating withdrawal...")
	withdrawalRequestCid, err := iwInitiateWithdrawal(
		ctx,
		cmdClient,
		cfg.Canton.RelayerParty,
		cfg.Canton.BridgePackageID,
		domainID,
		configCid,
		mappingCid,
		*iwHoldingCid,
		*iwAmount,
		*iwEvmDestination,
	)
	if err != nil {
		fmt.Printf("Failed to initiate withdrawal: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("    WithdrawalRequest CID: %s\n\n", withdrawalRequestCid)

	fmt.Println(">>> Processing withdrawal (burning tokens)...")
	withdrawalEventCid, err := iwProcessWithdrawal(
		ctx,
		cmdClient,
		cfg.Canton.RelayerParty,
		cfg.Canton.CorePackageID,
		domainID,
		withdrawalRequestCid,
	)
	if err != nil {
		fmt.Printf("Failed to process withdrawal: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("    WithdrawalEvent CID: %s\n\n", withdrawalEventCid)

	fmt.Println("======================================================================")
	fmt.Println("WITHDRAWAL PROCESSED SUCCESSFULLY")
	fmt.Println("======================================================================")
	fmt.Println("The relayer will now:")
	fmt.Println("  1. Detect the WithdrawalEvent on Canton")
	fmt.Println("  2. Submit withdrawFromCanton() transaction on EVM")
	fmt.Println("  3. Mark the withdrawal as complete on Canton")
	fmt.Println()
	fmt.Println("Monitor the relayer logs to see progress.")
}

func iwGetAuthContext(ctx context.Context, auth *config.AuthConfig) (context.Context, error) {
	if auth.ClientID == "" || auth.ClientSecret == "" || auth.Audience == "" || auth.TokenURL == "" {
		return ctx, fmt.Errorf("OAuth2 client credentials not configured")
	}

	token, err := iwGetOAuthToken(auth)
	if err != nil {
		return nil, err
	}

	md := metadata.Pairs("authorization", "Bearer "+token)
	return metadata.NewOutgoingContext(ctx, md), nil
}

func iwGetOAuthToken(auth *config.AuthConfig) (string, error) {
	iwTokenMu.Lock()
	defer iwTokenMu.Unlock()

	now := time.Now()
	if iwCachedToken != "" && now.Before(iwTokenExpiry) {
		return iwCachedToken, nil
	}

	payload := map[string]string{
		"client_id":     auth.ClientID,
		"client_secret": auth.ClientSecret,
		"audience":      auth.Audience,
		"grant_type":    "client_credentials",
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal OAuth token request: %w", err)
	}

	fmt.Printf("Fetching OAuth2 access token from %s...\n", auth.TokenURL)

	req, err := http.NewRequest("POST", auth.TokenURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create OAuth token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call OAuth token endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("OAuth token endpoint returned %d: %s", resp.StatusCode, string(b))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode OAuth token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("OAuth token response missing access_token")
	}

	expiry := now.Add(5 * time.Minute)
	if tokenResp.ExpiresIn > 0 {
		leeway := 60
		if tokenResp.ExpiresIn <= leeway {
			leeway = tokenResp.ExpiresIn / 2
		}
		expiry = now.Add(time.Duration(tokenResp.ExpiresIn-leeway) * time.Second)
	}

	iwCachedToken = tokenResp.AccessToken
	iwTokenExpiry = expiry

	if subject, err := iwExtractJWTSubject(tokenResp.AccessToken); err == nil {
		iwJwtSubject = subject
	}

	fmt.Printf("OAuth2 token obtained (expires in %d seconds)\n", tokenResp.ExpiresIn)
	return tokenResp.AccessToken, nil
}

func iwExtractJWTSubject(tokenString string) (string, error) {
	token, _, err := jwt.NewParser().ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return "", fmt.Errorf("failed to parse JWT: %w", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("invalid JWT claims")
	}
	sub, ok := claims["sub"].(string)
	if !ok {
		return "", fmt.Errorf("JWT missing 'sub' claim")
	}
	return sub, nil
}

func iwFindBridgeConfig(ctx context.Context, client lapiv2.StateServiceClient, party, packageID string, offset int64) (string, error) {
	resp, err := client.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: offset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				party: {
					Cumulative: []*lapiv2.CumulativeFilter{
						{
							IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
								TemplateFilter: &lapiv2.TemplateFilter{
									TemplateId: &lapiv2.Identifier{
										PackageId:  packageID,
										ModuleName: "Wayfinder.Bridge",
										EntityName: "WayfinderBridgeConfig",
									},
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
		return "", err
	}

	for {
		msg, err := resp.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			return contract.CreatedEvent.ContractId, nil
		}
	}
	return "", fmt.Errorf("no WayfinderBridgeConfig found")
}

func iwGetHoldingOwner(ctx context.Context, client lapiv2.StateServiceClient, party string, offset int64, holdingCid string) (string, error) {
	resp, err := client.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: offset,
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
			Verbose: true,
		},
	})
	if err != nil {
		return "", err
	}

	for {
		msg, err := resp.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			if contract.CreatedEvent.ContractId == holdingCid {
				fields := contract.CreatedEvent.CreateArguments.Fields
				for _, field := range fields {
					if field.Label == "owner" {
						if p, ok := field.Value.Sum.(*lapiv2.Value_Party); ok {
							return p.Party, nil
						}
					}
				}
			}
		}
	}
	return "", fmt.Errorf("holding not found: %s", holdingCid)
}

func iwFindFingerprintMapping(ctx context.Context, client lapiv2.StateServiceClient, party string, offset int64, targetCantonParty string) (string, error) {
	resp, err := client.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: offset,
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
			Verbose: true,
		},
	})
	if err != nil {
		return "", err
	}

	for {
		msg, err := resp.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			templateId := contract.CreatedEvent.TemplateId
			if templateId.ModuleName == "Common.FingerprintAuth" && templateId.EntityName == "FingerprintMapping" {
				fields := contract.CreatedEvent.CreateArguments.Fields
				for _, field := range fields {
					// Look up by userParty (the holding owner), not by fingerprint
					if field.Label == "userParty" {
						if p, ok := field.Value.Sum.(*lapiv2.Value_Party); ok {
							if p.Party == targetCantonParty {
								return contract.CreatedEvent.ContractId, nil
							}
						}
					}
				}
			}
		}
	}
	return "", fmt.Errorf("no FingerprintMapping found for user party: %s", targetCantonParty)
}

func iwInitiateWithdrawal(
	ctx context.Context,
	client lapiv2.CommandServiceClient,
	issuer, packageID, domainID, configCid, mappingCid, holdingCid, amount, evmDestination string,
) (string, error) {
	cmdID := fmt.Sprintf("initiate-withdrawal-%s", uuid.New().String())

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  packageID,
					ModuleName: "Wayfinder.Bridge",
					EntityName: "WayfinderBridgeConfig",
				},
				ContractId: configCid,
				Choice:     "InitiateWithdrawal",
				ChoiceArgument: &lapiv2.Value{
					Sum: &lapiv2.Value_Record{
						Record: &lapiv2.Record{
							Fields: []*lapiv2.RecordField{
								{Label: "mappingCid", Value: &lapiv2.Value{Sum: &lapiv2.Value_ContractId{ContractId: mappingCid}}},
								{Label: "holdingCid", Value: &lapiv2.Value{Sum: &lapiv2.Value_ContractId{ContractId: holdingCid}}},
								{Label: "amount", Value: &lapiv2.Value{Sum: &lapiv2.Value_Numeric{Numeric: amount}}},
								{Label: "evmDestination", Value: &lapiv2.Value{
									Sum: &lapiv2.Value_Record{
										Record: &lapiv2.Record{
											Fields: []*lapiv2.RecordField{
												{Label: "value", Value: &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: evmDestination}}},
											},
										},
									},
								}},
							},
						},
					},
				},
			},
		},
	}

	resp, err := client.SubmitAndWaitForTransaction(ctx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: domainID,
			CommandId:      cmdID,
			UserId:         iwJwtSubject,
			ActAs:          []string{issuer},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return "", err
	}

	if resp.Transaction != nil {
		for _, event := range resp.Transaction.Events {
			if created := event.GetCreated(); created != nil {
				templateId := created.TemplateId
				if templateId.ModuleName == "Bridge.Contracts" && templateId.EntityName == "WithdrawalRequest" {
					return created.ContractId, nil
				}
			}
		}
	}

	return "", fmt.Errorf("WithdrawalRequest not found in response")
}

func iwProcessWithdrawal(
	ctx context.Context,
	client lapiv2.CommandServiceClient,
	issuer, packageID, domainID, withdrawalRequestCid string,
) (string, error) {
	cmdID := fmt.Sprintf("process-withdrawal-%s", uuid.New().String())

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  packageID,
					ModuleName: "Bridge.Contracts",
					EntityName: "WithdrawalRequest",
				},
				ContractId:     withdrawalRequestCid,
				Choice:         "ProcessWithdrawal",
				ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: &lapiv2.Record{}}},
			},
		},
	}

	resp, err := client.SubmitAndWaitForTransaction(ctx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: domainID,
			CommandId:      cmdID,
			UserId:         iwJwtSubject,
			ActAs:          []string{issuer},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return "", err
	}

	if resp.Transaction != nil {
		for _, event := range resp.Transaction.Events {
			if created := event.GetCreated(); created != nil {
				templateId := created.TemplateId
				if templateId.ModuleName == "Bridge.Contracts" && templateId.EntityName == "WithdrawalEvent" {
					return created.ContractId, nil
				}
			}
		}
	}

	return "", fmt.Errorf("WithdrawalEvent not found in response")
}
