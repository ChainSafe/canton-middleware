//go:build ignore

// extract-loop-info.go — Extract integration data for Canton Loop.
//
// Prints InstrumentID, TransferFactory details, holdings, and package IDs
// that Canton Loop needs for explicit disclosure and token transfers.
//
// Usage:
//
//	DATABASE_HOST=localhost go run scripts/remote/extract-loop-info.go \
//	  -config config.api-server.devnet.yaml \
//	  -party "PAR::namespace::fingerprint"

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	expcreds "google.golang.org/grpc/experimental/credentials"
	"google.golang.org/grpc/metadata"
)

var (
	configPath = flag.String("config", "config.api-server.devnet.yaml", "Path to API server config file")
	partyID    = flag.String("party", "", "Canton party ID to query holdings for (optional)")
)

func main() {
	flag.Parse()

	cfg, err := config.LoadAPIServer(*configPath)
	if err != nil {
		fatalf("Failed to load config: %v", err)
	}

	issuer := cfg.Canton.RelayerParty
	if issuer == "" {
		fatalf("canton.relayer_party is required in config")
	}
	cip56Pkg := cfg.Canton.CIP56PackageID
	if cip56Pkg == "" {
		fatalf("canton.cip56_package_id is required in config")
	}

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Canton Loop Integration Data")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Printf("  Canton:  %s\n", cfg.Canton.RPCURL)
	fmt.Printf("  Issuer:  %s\n", issuer)
	if *partyID != "" {
		fmt.Printf("  Party:   %s\n", *partyID)
	}
	fmt.Println()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	conn, err := dialCanton(cfg)
	if err != nil {
		fatalf("Failed to connect to Canton: %v", err)
	}
	defer conn.Close()

	ctx, _, err = authContext(ctx, &cfg.Canton)
	if err != nil {
		fatalf("Failed to get auth token: %v", err)
	}

	stateService := lapiv2.NewStateServiceClient(conn)

	// === InstrumentID ===
	instrumentAdmin := cfg.Canton.InstrumentAdmin
	if instrumentAdmin == "" {
		instrumentAdmin = issuer
	}
	instrumentID := cfg.Canton.InstrumentID
	if instrumentID == "" {
		instrumentID = "DEMO"
	}

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  1. InstrumentID")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Printf("  admin: %s\n", instrumentAdmin)
	fmt.Printf("  id:    %s\n", instrumentID)
	fmt.Println()

	// === TransferFactory ===
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  2. TransferFactory (for explicit disclosure)")
	fmt.Println("══════════════════════════════════════════════════════════════════════")

	factory, err := getTransferFactory(ctx, stateService, issuer, cip56Pkg)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
	} else {
		fmt.Printf("  contract_id:        %s\n", factory.contractID)
		fmt.Printf("  template_id:        %s:%s:%s\n", cip56Pkg, "CIP56.TransferFactory", "CIP56TransferFactory")
		fmt.Printf("  created_event_blob: %s\n", base64.StdEncoding.EncodeToString(factory.createdEventBlob))
	}
	fmt.Println()

	// === Holdings ===
	if *partyID != "" {
		fmt.Println("══════════════════════════════════════════════════════════════════════")
		fmt.Println("  3. Holdings for Party")
		fmt.Println("══════════════════════════════════════════════════════════════════════")

		holdings, err := getHoldings(ctx, stateService, issuer, cip56Pkg, *partyID)
		if err != nil {
			fmt.Printf("  ERROR: %v\n", err)
		} else if len(holdings) == 0 {
			fmt.Println("  (no holdings found)")
		} else {
			for i, h := range holdings {
				fmt.Printf("  [%d] contract_id: %s\n", i+1, h.contractID)
				fmt.Printf("      owner:       %s\n", h.owner)
				fmt.Printf("      amount:      %s\n", h.amount)
				fmt.Printf("      symbol:      %s\n", h.symbol)
				fmt.Printf("      locked:      %v\n", h.locked)
			}
		}
		fmt.Println()
	}

	// === Package IDs ===
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  4. Package IDs")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Printf("  cip56_package_id:          %s\n", cfg.Canton.CIP56PackageID)
	fmt.Printf("  splice_holding_package_id: %s\n", cfg.Canton.SpliceHoldingPackageID)
	fmt.Printf("  splice_transfer_package_id:%s\n", cfg.Canton.SpliceTransferPackageID)
	fmt.Printf("  common_package_id:         %s\n", cfg.Canton.CommonPackageID)
	fmt.Printf("  bridge_package_id:         %s\n", cfg.Canton.BridgePackageID)
	fmt.Println()

	// === JSON summary for easy sharing ===
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  JSON Summary (copy-paste for Dinh)")
	fmt.Println("══════════════════════════════════════════════════════════════════════")

	summary := map[string]interface{}{
		"instrument_id": map[string]string{
			"admin": instrumentAdmin,
			"id":    instrumentID,
		},
		"package_ids": map[string]string{
			"cip56":           cfg.Canton.CIP56PackageID,
			"splice_holding":  cfg.Canton.SpliceHoldingPackageID,
			"splice_transfer": cfg.Canton.SpliceTransferPackageID,
		},
	}

	if factory != nil {
		summary["transfer_factory"] = map[string]string{
			"contract_id":        factory.contractID,
			"created_event_blob": base64.StdEncoding.EncodeToString(factory.createdEventBlob),
			"template_id":        fmt.Sprintf("%s:CIP56.TransferFactory:CIP56TransferFactory", cip56Pkg),
		}
	}

	jsonBytes, _ := json.MarshalIndent(summary, "  ", "  ")
	fmt.Printf("  %s\n", string(jsonBytes))
	fmt.Println()
}

type transferFactoryResult struct {
	contractID       string
	createdEventBlob []byte
}

func getTransferFactory(ctx context.Context, client lapiv2.StateServiceClient, party, packageID string) (*transferFactoryResult, error) {
	endResp, err := client.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		return nil, err
	}
	if endResp.Offset == 0 {
		return nil, fmt.Errorf("ledger is empty")
	}

	tid := &lapiv2.Identifier{
		PackageId:  packageID,
		ModuleName: "CIP56.TransferFactory",
		EntityName: "CIP56TransferFactory",
	}

	stream, err := client.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: endResp.Offset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				party: {
					Cumulative: []*lapiv2.CumulativeFilter{
						{
							IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
								TemplateFilter: &lapiv2.TemplateFilter{
									TemplateId:              tid,
									IncludeCreatedEventBlob: true,
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

	for {
		msg, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		if ac := msg.GetActiveContract(); ac != nil && ac.CreatedEvent != nil {
			return &transferFactoryResult{
				contractID:       ac.CreatedEvent.ContractId,
				createdEventBlob: ac.CreatedEvent.CreatedEventBlob,
			}, nil
		}
	}
	return nil, fmt.Errorf("no CIP56TransferFactory found")
}

type holdingResult struct {
	contractID string
	owner      string
	amount     string
	symbol     string
	locked     bool
}

func getHoldings(ctx context.Context, client lapiv2.StateServiceClient, issuer, packageID, party string) ([]*holdingResult, error) {
	endResp, err := client.GetLedgerEnd(ctx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		return nil, err
	}
	if endResp.Offset == 0 {
		return nil, nil
	}

	stream, err := client.GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: endResp.Offset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				issuer: {
					Cumulative: []*lapiv2.CumulativeFilter{
						{
							IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
								TemplateFilter: &lapiv2.TemplateFilter{
									TemplateId: &lapiv2.Identifier{
										PackageId:  packageID,
										ModuleName: "CIP56.Token",
										EntityName: "CIP56Holding",
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
		return nil, err
	}

	var results []*holdingResult
	for {
		msg, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		if ac := msg.GetActiveContract(); ac != nil && ac.CreatedEvent != nil {
			fields := values.RecordToMap(ac.CreatedEvent.CreateArguments)
			owner := values.Party(fields["owner"])

			// Filter by party if specified
			if owner != party {
				continue
			}

			meta := values.DecodeMetadata(fields["meta"])
			results = append(results, &holdingResult{
				contractID: ac.CreatedEvent.ContractId,
				owner:      owner,
				amount:     values.Numeric(fields["amount"]),
				symbol:     meta[values.MetaKeySymbol],
				locked:     !values.IsNone(fields["lock"]),
			})
		}
	}
	return results, nil
}

func dialCanton(cfg *config.APIServerConfig) (*grpc.ClientConn, error) {
	var opts []grpc.DialOption
	if cfg.Canton.TLS.Enabled {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // devnet testing
		}
		opts = append(opts, grpc.WithTransportCredentials(expcreds.NewTLSWithALPNDisabled(tlsConfig)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	if cfg.Canton.MaxMessageSize > 0 {
		opts = append(opts, grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(cfg.Canton.MaxMessageSize)))
	}

	target := cfg.Canton.RPCURL
	if !strings.Contains(target, "://") {
		target = "dns:///" + target
	}
	return grpc.NewClient(target, opts...)
}

func authContext(ctx context.Context, canton *config.CantonConfig) (context.Context, string, error) {
	if canton.Auth.ClientID == "" {
		return ctx, "", nil
	}

	payload := map[string]string{
		"client_id":     canton.Auth.ClientID,
		"client_secret": canton.Auth.ClientSecret,
		"audience":      canton.Auth.Audience,
		"grant_type":    "client_credentials",
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(canton.Auth.TokenURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(respBody))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return nil, "", fmt.Errorf("parse token response: %w", err)
	}

	var sub string
	parts := strings.Split(tokenResp.AccessToken, ".")
	if len(parts) >= 2 {
		padded := parts[1]
		switch len(padded) % 4 {
		case 2:
			padded += "=="
		case 3:
			padded += "="
		}
		if decoded, err := base64.URLEncoding.DecodeString(padded); err == nil {
			var claims struct {
				Sub string `json:"sub"`
			}
			json.Unmarshal(decoded, &claims)
			sub = claims.Sub
		}
	}

	md := metadata.Pairs("authorization", "Bearer "+tokenResp.AccessToken)
	return metadata.NewOutgoingContext(ctx, md), sub, nil
}

func fatalf(format string, args ...interface{}) {
	fmt.Printf("ERROR: "+format+"\n", args...)
	os.Exit(1)
}
