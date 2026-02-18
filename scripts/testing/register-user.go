//go:build ignore

// register-user.go - Register a user's fingerprint mapping on Canton
//
// Usage:
//   go run scripts/register-user.go -config config.yaml \
//     -party "Alice::1220abc...def" \
//     -fingerprint "abc...def" \
//     -evm-address "0x..."
//
// For testing with the BridgeIssuer (uses config relayer_party by default):
//   go run scripts/register-user.go -config config.yaml

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

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

var (
	ruConfigPath  = flag.String("config", "config.yaml", "Path to config file")
	ruPartyID     = flag.String("party", "", "Full Canton Party ID (uses config relayer_party if not specified)")
	ruFingerprint = flag.String("fingerprint", "", "Fingerprint (32-byte hex, without 0x1220 prefix)")
	ruEvmAddress  = flag.String("evm-address", "", "Optional EVM address for withdrawals")
)

var (
	ruTokenMu     sync.Mutex
	ruCachedToken string
	ruTokenExpiry time.Time
	ruJwtSubject  string
)

func main() {
	flag.Parse()

	cfg, err := config.Load(*ruConfigPath)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	partyID := *ruPartyID
	if partyID == "" {
		partyID = cfg.Canton.RelayerParty
	}

	if partyID == "" {
		fmt.Println("Error: -party is required (or set canton.relayer_party in config)")
		fmt.Println("Usage: go run scripts/register-user.go -config config.yaml -party 'PartyID' -fingerprint 'hex'")
		os.Exit(1)
	}

	fingerprint := *ruFingerprint
	if fingerprint == "" {
		parts := strings.Split(partyID, "::")
		if len(parts) == 2 {
			fp := parts[1]
			if strings.HasPrefix(fp, "1220") && len(fp) == 68 {
				fingerprint = fp[4:]
			} else {
				fingerprint = fp
			}
			fmt.Printf("Extracted fingerprint from party ID: %s\n", fingerprint)
		} else {
			fmt.Println("Error: Could not extract fingerprint from party ID. Please provide -fingerprint")
			os.Exit(1)
		}
	}

	fmt.Println("======================================================================")
	fmt.Println("REGISTER USER - Create FingerprintMapping on Canton")
	fmt.Println("======================================================================")
	fmt.Printf("Party:       %s\n", partyID)
	fmt.Printf("Fingerprint: %s\n", fingerprint)
	fmt.Printf("EVM Address: %s\n", *ruEvmAddress)
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

	ctx, err = ruGetAuthContext(ctx, &cfg.Canton.Auth)
	if err != nil {
		fmt.Printf("Failed to get auth context: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("JWT Subject: %s\n\n", ruJwtSubject)

	stateClient := lapiv2.NewStateServiceClient(conn)
	cmdClient := lapiv2.NewCommandServiceClient(conn)

	ledgerEndResp, err := stateClient.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		fmt.Printf("Failed to get ledger end: %v\n", err)
		os.Exit(1)
	}
	if ledgerEndResp.Offset == 0 {
		fmt.Println("Error: Ledger is empty. Run bootstrap first.")
		os.Exit(1)
	}

	fmt.Println(">>> Checking for existing FingerprintMapping...")
	// Use common_package_id with fallback to bridge_package_id for FingerprintMapping
	commonPkgID := cfg.Canton.CommonPackageID
	if commonPkgID == "" {
		commonPkgID = cfg.Canton.BridgePackageID
	}
	existingCid, err := ruFindFingerprintMapping(ctx, stateClient, cfg.Canton.RelayerParty, commonPkgID, ledgerEndResp.Offset, fingerprint)
	if err == nil && existingCid != "" {
		fmt.Printf("    [EXISTS] FingerprintMapping already exists: %s\n", existingCid)
		fmt.Println("\n✓ User is already registered!")
		os.Exit(0)
	}

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

	fmt.Println(">>> Creating FingerprintMapping directly...")
	mappingCid, err := ruCreateFingerprintMapping(ctx, cmdClient, cfg.Canton.RelayerParty, commonPkgID, domainID, partyID, fingerprint, *ruEvmAddress)
	if err != nil {
		fmt.Printf("Failed to register user: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("    FingerprintMapping CID: %s\n", mappingCid)

	fmt.Println()
	fmt.Println("======================================================================")
	fmt.Println("USER REGISTERED SUCCESSFULLY")
	fmt.Println("======================================================================")
	fmt.Printf("Party:            %s\n", partyID)
	fmt.Printf("Fingerprint:      %s\n", fingerprint)
	fmt.Printf("MappingCid:       %s\n", mappingCid)
	fmt.Println()
	fmt.Println("The user can now receive deposits with this fingerprint as bytes32:")
	fmt.Printf("  CANTON_RECIPIENT=\"0x%s\"\n", fingerprint)
}

func ruGetAuthContext(ctx context.Context, auth *config.AuthConfig) (context.Context, error) {
	if auth.ClientID == "" || auth.ClientSecret == "" || auth.Audience == "" || auth.TokenURL == "" {
		return ctx, fmt.Errorf("OAuth2 client credentials not configured")
	}

	token, err := ruGetOAuthToken(auth)
	if err != nil {
		return nil, err
	}

	md := metadata.Pairs("authorization", "Bearer "+token)
	return metadata.NewOutgoingContext(ctx, md), nil
}

func ruGetOAuthToken(auth *config.AuthConfig) (string, error) {
	ruTokenMu.Lock()
	defer ruTokenMu.Unlock()

	now := time.Now()
	if ruCachedToken != "" && now.Before(ruTokenExpiry) {
		return ruCachedToken, nil
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

	ruCachedToken = tokenResp.AccessToken
	ruTokenExpiry = expiry

	if subject, err := ruExtractJWTSubject(tokenResp.AccessToken); err == nil {
		ruJwtSubject = subject
	}

	fmt.Printf("OAuth2 token obtained (expires in %d seconds)\n", tokenResp.ExpiresIn)
	return tokenResp.AccessToken, nil
}

func ruExtractJWTSubject(tokenString string) (string, error) {
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

func ruFindFingerprintMapping(ctx context.Context, client lapiv2.StateServiceClient, party, packageID string, offset int64, targetFingerprint string) (string, error) {
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
					if field.Label == "fingerprint" {
						if fp, ok := field.Value.Sum.(*lapiv2.Value_Text); ok {
							if fp.Text == targetFingerprint {
								return contract.CreatedEvent.ContractId, nil
							}
						}
					}
				}
			}
		}
	}
	return "", fmt.Errorf("no FingerprintMapping found for fingerprint: %s", targetFingerprint)
}

// ruCreateFingerprintMapping creates a FingerprintMapping contract directly via CreateCommand.
// The issuer has signatory rights on FingerprintMapping, so no bridge config is needed.
func ruCreateFingerprintMapping(ctx context.Context, client lapiv2.CommandServiceClient, issuer, packageID, domainID, userParty, fingerprint, evmAddress string) (string, error) {
	cmdID := fmt.Sprintf("register-user-%s", uuid.New().String())

	fields := []*lapiv2.RecordField{
		{Label: "issuer", Value: &lapiv2.Value{Sum: &lapiv2.Value_Party{Party: issuer}}},
		{Label: "userParty", Value: &lapiv2.Value{Sum: &lapiv2.Value_Party{Party: userParty}}},
		{Label: "fingerprint", Value: &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: fingerprint}}},
	}

	if evmAddress != "" {
		fields = append(fields, &lapiv2.RecordField{
			Label: "evmAddress",
			Value: &lapiv2.Value{
				Sum: &lapiv2.Value_Optional{
					Optional: &lapiv2.Optional{
						Value: &lapiv2.Value{
							Sum: &lapiv2.Value_Record{
								Record: &lapiv2.Record{
									Fields: []*lapiv2.RecordField{
										{Label: "value", Value: &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: evmAddress}}},
									},
								},
							},
						},
					},
				},
			},
		})
	} else {
		fields = append(fields, &lapiv2.RecordField{
			Label: "evmAddress",
			Value: &lapiv2.Value{Sum: &lapiv2.Value_Optional{Optional: &lapiv2.Optional{Value: nil}}},
		})
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Create{
			Create: &lapiv2.CreateCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  packageID,
					ModuleName: "Common.FingerprintAuth",
					EntityName: "FingerprintMapping",
				},
				CreateArguments: &lapiv2.Record{Fields: fields},
			},
		},
	}

	resp, err := client.SubmitAndWaitForTransaction(ctx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: domainID,
			CommandId:      cmdID,
			UserId:         ruJwtSubject,
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
				if templateId.ModuleName == "Common.FingerprintAuth" && templateId.EntityName == "FingerprintMapping" {
					return created.ContractId, nil
				}
			}
		}
	}

	return "", fmt.Errorf("FingerprintMapping not found in response")
}
