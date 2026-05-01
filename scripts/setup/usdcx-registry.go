//go:build ignore

// usdcx-registry.go — Devstack Splice Transfer Factory Registry for USDCx.
//
// In production this service is run by Circle (the USDCx issuer). In the local
// devstack the USDCxIssuer party lives on participant2. This binary connects to
// P2, discovers the CIP56TransferFactory, and serves the Splice Transfer Factory
// Registry API so that P1's api-server can call it during PrepareTransfer.
//
// Endpoint: POST /registry/transfer-instruction/v1/transfer-factory
//
// Usage (docker-compose):
//
//	/app/usdcx-registry \
//	  -p2              canton:5021 \
//	  -p2-http         http://canton:5023 \
//	  -p2-audience     http://canton:5021 \
//	  -token-url       http://mock-oauth2:8088/oauth/token \
//	  -client-id       local-test-client \
//	  -client-secret   local-test-secret \
//	  -port            8090

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/ledger"
)

var (
	p2Grpc       = flag.String("p2", "canton:5021", "Participant2 gRPC address")
	p2HTTP       = flag.String("p2-http", "http://canton:5023", "Participant2 HTTP API for party/domain discovery")
	p2Audience   = flag.String("p2-audience", "http://canton:5021", "JWT audience for participant2")
	tokenURL     = flag.String("token-url", "http://mock-oauth2:8088/oauth/token", "OAuth2 token endpoint")
	clientID     = flag.String("client-id", "local-test-client", "OAuth2 client ID")
	clientSecret = flag.String("client-secret", "local-test-secret", "OAuth2 client secret")
	listenPort   = flag.String("port", "8090", "HTTP port to listen on")
	cip56PkgID   = flag.String("cip56-package-id", "c8c6fe7c34d96b88d6471769aae85063c8045783b2a226fd24f8c573603d17c2", "CIP56 package ID")
)

// registryResponse matches the JSON format expected by pkg/cantonsdk/token.RegistryClient.
type registryResponse struct {
	FactoryID          string              `json:"factoryId"`
	TransferKind       string              `json:"transferKind"`
	ChoiceContext      any                 `json:"choiceContext"`
	DisclosedContracts []disclosedContract `json:"disclosedContracts"`
}

type disclosedContract struct {
	ContractID       string `json:"contractId"`
	CreatedEventBlob string `json:"createdEventBlob"` // base64-encoded raw blob
	SynchronizerID   string `json:"synchronizerId"`
}

type server struct {
	p2     *ledger.Client
	issuer string
	domain string
}

func main() {
	flag.Parse()

	ctx := context.Background()

	log.Println(">>> USDCx Registry: connecting to participant2...")
	p2, err := ledger.New(&ledger.Config{
		RPCURL:         *p2Grpc,
		MaxMessageSize: 52428800,
		TLS:            &ledger.TLSConfig{Enabled: false},
		Auth: &ledger.AuthConfig{
			ClientID:     *clientID,
			ClientSecret: *clientSecret,
			Audience:     *p2Audience,
			TokenURL:     *tokenURL,
			ExpiryLeeway: 60 * time.Second,
		},
	})
	if err != nil {
		log.Fatalf("connect to P2: %v", err)
	}
	defer p2.Close()

	log.Println(">>> USDCx Registry: discovering USDCxIssuer party...")
	issuer, err := discoverIssuer(ctx, *p2HTTP)
	if err != nil {
		log.Fatalf("discover USDCxIssuer party: %v", err)
	}
	log.Printf("    USDCxIssuer: %s", issuer)

	log.Println(">>> USDCx Registry: discovering domain ID...")
	domain, err := discoverDomain(ctx, *p2HTTP)
	if err != nil {
		log.Fatalf("discover domain ID: %v", err)
	}
	log.Printf("    Domain: %s", domain)

	srv := &server{p2: p2, issuer: issuer, domain: domain}

	http.HandleFunc("/registry/transfer-instruction/v1/transfer-factory", srv.handleTransferFactory)
	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	addr := ":" + *listenPort
	log.Printf(">>> USDCx Registry listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

func (s *server) handleTransferFactory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	contractID, blob, err := s.queryFactory(ctx)
	if err != nil {
		log.Printf("ERROR: query CIP56TransferFactory: %v", err)
		http.Error(w, fmt.Sprintf("failed to query factory: %v", err), http.StatusInternalServerError)
		return
	}

	resp := registryResponse{
		FactoryID:    contractID,
		TransferKind: "transfer",
		ChoiceContext: nil,
		DisclosedContracts: []disclosedContract{
			{
				ContractID:       contractID,
				CreatedEventBlob: base64.StdEncoding.EncodeToString(blob),
				SynchronizerID:   s.domain,
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("ERROR: encode response: %v", err)
	}
}

func (s *server) queryFactory(ctx context.Context) (contractID string, blob []byte, err error) {
	authCtx := s.p2.AuthContext(ctx)

	end, err := s.p2.GetLedgerEnd(authCtx)
	if err != nil {
		return "", nil, fmt.Errorf("get ledger end: %w", err)
	}
	if end == 0 {
		return "", nil, fmt.Errorf("ledger is empty")
	}

	tid := &lapiv2.Identifier{
		PackageId:  *cip56PkgID,
		ModuleName: "CIP56.TransferFactory",
		EntityName: "CIP56TransferFactory",
	}

	stream, err := s.p2.State().GetActiveContracts(authCtx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: end,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				s.issuer: {
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
		return "", nil, fmt.Errorf("get active contracts: %w", err)
	}

	for {
		msg, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", nil, fmt.Errorf("receive: %w", err)
		}
		if ac := msg.GetActiveContract(); ac != nil && ac.CreatedEvent != nil {
			return ac.CreatedEvent.ContractId, ac.CreatedEvent.CreatedEventBlob, nil
		}
	}

	return "", nil, fmt.Errorf("CIP56TransferFactory not found for issuer %s", s.issuer)
}

// discoverIssuer lists parties on P2 and returns the first one matching USDCxIssuer::.
func discoverIssuer(ctx context.Context, p2HTTPURL string) (string, error) {
	url := strings.TrimRight(p2HTTPURL, "/") + "/v2/parties"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var body struct {
		PartyDetails []struct {
			Party string `json:"party"`
		} `json:"partyDetails"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("parse parties response: %w", err)
	}

	for _, pd := range body.PartyDetails {
		if strings.HasPrefix(pd.Party, "USDCxIssuer::") {
			return pd.Party, nil
		}
	}
	return "", fmt.Errorf("no USDCxIssuer party found on P2")
}

// discoverDomain returns the synchronizer ID from P2's connected synchronizers.
func discoverDomain(ctx context.Context, p2HTTPURL string) (string, error) {
	url := strings.TrimRight(p2HTTPURL, "/") + "/v2/state/connected-synchronizers"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var body struct {
		ConnectedSynchronizers []struct {
			SynchronizerID string `json:"synchronizerId"`
		} `json:"connectedSynchronizers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("parse synchronizers response: %w", err)
	}

	if len(body.ConnectedSynchronizers) == 0 {
		return "", fmt.Errorf("P2 not connected to any synchronizer")
	}
	return body.ConnectedSynchronizers[0].SynchronizerID, nil
}
