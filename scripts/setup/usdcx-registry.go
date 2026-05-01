//go:build ignore

// usdcx-registry.go — Devstack Splice Transfer Factory Registry for USDCx.
//
// Implements the Splice Transfer Factory Registry API, exactly as an external
// token issuer (e.g., Circle for real USDCx) would operate it in production.
// When migrating to testnet or mainnet, replace the
// canton.token.external_tokens[<issuer-party>].registry_url value in the
// api-server config with the issuer's live endpoint — no other code changes
// are required.
//
// Protocol: POST /registry/transfer-instruction/v1/transfer-factory
//
//   Request body (RegistryRequest):
//     { "expectedAdmin": "<party>", "transfer": { "sender": "...", ... } }
//
//   Response body (RegistryResponse):
//     { "factoryId": "...", "transferKind": "transfer",
//       "choiceContext": null,
//       "disclosedContracts": [{ "contractId": "...",
//                                "createdEventBlob": "<base64>",
//                                "templateId": "<pkg>:<module>:<entity>",
//                                "synchronizerId": "..." }] }
//
// These types must stay in sync with pkg/cantonsdk/token/registry_client.go.

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
	"sync"
	"time"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/ledger"
)

var (
	p2Grpc       = flag.String("p2", "canton:5021", "Participant2 gRPC address")
	p2HTTP       = flag.String("p2-http", "http://canton:5023", "Participant2 HTTP API for discovery")
	p2Audience   = flag.String("p2-audience", "http://canton:5021", "JWT audience for participant2")
	tokenURL     = flag.String("token-url", "http://mock-oauth2:8088/oauth/token", "OAuth2 token endpoint")
	clientID     = flag.String("client-id", "local-test-client", "OAuth2 client ID")
	clientSecret = flag.String("client-secret", "local-test-secret", "OAuth2 client secret")
	listenPort   = flag.String("port", "8090", "HTTP port to listen on")
	cip56PkgID   = flag.String("cip56-package-id", "c8c6fe7c34d96b88d6471769aae85063c8045783b2a226fd24f8c573603d17c2", "CIP56 package ID")
	cacheTTL     = flag.Duration("cache-ttl", 60*time.Second, "Factory cache TTL (0 = query on every request)")
)

// ─── API types ───────────────────────────────────────────────────────────────
// These must match the JSON shapes in pkg/cantonsdk/token/registry_client.go.

// RegistryRequest is the POST body sent by RegistryClient.GetTransferFactory.
type RegistryRequest struct {
	ExpectedAdmin string         `json:"expectedAdmin"`
	Transfer      TransferDetail `json:"transfer"`
}

// TransferDetail carries the transfer parameters sent by the caller.
type TransferDetail struct {
	Sender           string   `json:"sender"`
	Receiver         string   `json:"receiver"`
	Amount           string   `json:"amount"`
	InstrumentID     string   `json:"instrumentId"`
	InputHoldingCIDs []string `json:"inputHoldingCids"`
}

// RegistryResponse is the response body parsed by RegistryClient.GetTransferFactory.
type RegistryResponse struct {
	FactoryID          string              `json:"factoryId"`
	TransferKind       string              `json:"transferKind"`
	ChoiceContext      any                 `json:"choiceContext"`
	DisclosedContracts []DisclosedContract `json:"disclosedContracts"`
}

// DisclosedContract matches the registryDisclosedContract shape in registry_client.go.
type DisclosedContract struct {
	ContractID       string `json:"contractId"`
	CreatedEventBlob string `json:"createdEventBlob"` // base64-encoded raw blob
	TemplateID       string `json:"templateId"`        // "<packageId>:<module>:<entity>"
	SynchronizerID   string `json:"synchronizerId"`
}

type errorBody struct {
	Error string `json:"error"`
}

// ─── Cache ───────────────────────────────────────────────────────────────────

type factoryCache struct {
	mu        sync.RWMutex
	resp      *RegistryResponse
	expiresAt time.Time
	ttl       time.Duration
}

func (c *factoryCache) load() *RegistryResponse {
	if c.ttl == 0 {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.resp != nil && time.Now().Before(c.expiresAt) {
		return c.resp
	}
	return nil
}

func (c *factoryCache) store(resp *RegistryResponse) {
	if c.ttl == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resp = resp
	c.expiresAt = time.Now().Add(c.ttl)
}

// ─── Server ──────────────────────────────────────────────────────────────────

type server struct {
	p2     *ledger.Client
	issuer string
	domain string
	cache  *factoryCache
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

	srv := &server{
		p2:     p2,
		issuer: issuer,
		domain: domain,
		cache:  &factoryCache{ttl: *cacheTTL},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/registry/transfer-instruction/v1/transfer-factory", srv.handleTransferFactory)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	addr := ":" + *listenPort
	log.Printf(">>> USDCx Registry listening on %s (issuer: %s, cache TTL: %s)", addr, issuer, *cacheTTL)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

// handleTransferFactory implements POST /registry/transfer-instruction/v1/transfer-factory.
func (s *server) handleTransferFactory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorBody{Error: "method not allowed"})
		return
	}

	defer r.Body.Close()
	var req RegistryRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "invalid request body: " + err.Error()})
		return
	}

	if req.ExpectedAdmin != s.issuer {
		writeJSON(w, http.StatusBadRequest, errorBody{
			Error: fmt.Sprintf("expectedAdmin %q does not match issuer", req.ExpectedAdmin),
		})
		return
	}

	if resp := s.cache.load(); resp != nil {
		writeJSON(w, http.StatusOK, resp)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	resp, err := s.queryFactory(ctx)
	if err != nil {
		log.Printf("ERROR: query CIP56TransferFactory: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorBody{Error: "failed to retrieve transfer factory"})
		return
	}

	s.cache.store(resp)
	writeJSON(w, http.StatusOK, resp)
}

// queryFactory queries P2's ACS for the active CIP56TransferFactory and builds
// the RegistryResponse. The IncludeCreatedEventBlob flag is required so the
// caller can disclose the factory contract to a Canton node that doesn't hold it
// in its own ACS (Canton DisclosedContracts mechanism).
func (s *server) queryFactory(ctx context.Context) (*RegistryResponse, error) {
	authCtx := s.p2.AuthContext(ctx)

	end, err := s.p2.GetLedgerEnd(authCtx)
	if err != nil {
		return nil, fmt.Errorf("get ledger end: %w", err)
	}
	if end == 0 {
		return nil, fmt.Errorf("ledger is empty")
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
					Cumulative: []*lapiv2.CumulativeFilter{{
						IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
							TemplateFilter: &lapiv2.TemplateFilter{
								TemplateId:              tid,
								IncludeCreatedEventBlob: true,
							},
						},
					}},
				},
			},
			Verbose: true,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get active contracts: %w", err)
	}

	var events []*lapiv2.CreatedEvent
	for {
		msg, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("receive stream: %w", err)
		}
		if ac := msg.GetActiveContract(); ac != nil && ac.CreatedEvent != nil {
			events = append(events, ac.CreatedEvent)
		}
	}

	if len(events) == 0 {
		return nil, fmt.Errorf("CIP56TransferFactory not found for issuer %s", s.issuer)
	}
	if len(events) > 1 {
		log.Printf("WARN: %d CIP56TransferFactory contracts found, using first (cid=%s)", len(events), events[0].ContractId)
	}

	ev := events[0]
	templateID := fmt.Sprintf("%s:%s:%s", tid.PackageId, tid.ModuleName, tid.EntityName)

	return &RegistryResponse{
		FactoryID:    ev.ContractId,
		TransferKind: "transfer",
		ChoiceContext: nil,
		DisclosedContracts: []DisclosedContract{{
			ContractID:       ev.ContractId,
			CreatedEventBlob: base64.StdEncoding.EncodeToString(ev.CreatedEventBlob),
			TemplateID:       templateID,
			SynchronizerID:   s.domain,
		}},
	}, nil
}

// ─── Discovery helpers ────────────────────────────────────────────────────────

// discoverIssuer lists parties on P2 and returns the first one with the
// USDCxIssuer:: prefix.
func discoverIssuer(ctx context.Context, p2HTTPURL string) (string, error) {
	url := strings.TrimRight(p2HTTPURL, "/") + "/v2/parties"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
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
	return "", fmt.Errorf("no USDCxIssuer party found on P2 (is bootstrap complete?)")
}

// discoverDomain returns the synchronizer ID from P2's connected-synchronizers endpoint.
func discoverDomain(ctx context.Context, p2HTTPURL string) (string, error) {
	url := strings.TrimRight(p2HTTPURL, "/") + "/v2/state/connected-synchronizers"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
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

// ─── Helpers ──────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("WARN: write JSON response: %v", err)
	}
}
