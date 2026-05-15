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
// Sender-side endpoint:
//   POST /registry/transfer-instruction/v1/transfer-factory
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
// Receiver-side accept endpoint:
//   POST /api/token-standard/v0/registrars/{registrar}/registry/transfer-instruction/v1/{cid}/choice-contexts/accept
//
//   Request body: { "meta": {}, "excludeDebugFields": false }
//
//   Response body:
//     { "choiceContextData": { "values": {
//           "utility.digitalasset.com/transfer-rule":            {"tag":"AV_ContractId","value":"<cid>"},
//           "utility.digitalasset.com/instrument-configuration": {"tag":"AV_ContractId","value":"<cid>"},
//           "utility.digitalasset.com/sender-credentials":       {"tag":"AV_List","value":[]},
//           "utility.digitalasset.com/receiver-credentials":     {"tag":"AV_List","value":[]} } },
//       "disclosedContracts": [...] }
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
	// registryAppPkgID is the utility_registry_app_v0 package ID — AllocationFactory lives here.
	registryAppPkgID = flag.String("registry-app-package-id", "7a75ef6e69f69395a4e60919e228528bb8f3881150ccfde3f31bcc73864b18ab", "utility_registry_app_v0 package ID")
	// registryPkgID is the utility_registry_v0 package ID — TransferRule and InstrumentConfiguration live here.
	registryPkgID = flag.String("registry-package-id", "a236e8e22a3b5f199e37d5554e82bafd2df688f901de02b00be3964bdfa8c1ab", "utility_registry_v0 package ID")
	cacheTTL      = flag.Duration("cache-ttl", 60*time.Second, "Cache TTL for ACS queries (0 = query on every request)")
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

// AcceptContextResponse is returned by the receiver-side accept endpoint.
// The JSON shape must match what accept-via-interface.go and the SDK client expect.
type AcceptContextResponse struct {
	ChoiceContextData  acceptChoiceContextData `json:"choiceContextData"`
	DisclosedContracts []DisclosedContract     `json:"disclosedContracts"`
}

// acceptChoiceContextData holds the AnyValue map consumed by TransferInstruction_Accept.
type acceptChoiceContextData struct {
	Values map[string]any `json:"values"`
}

type errorBody struct {
	Error string `json:"error"`
}

// ─── Caches ───────────────────────────────────────────────────────────────────

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

type acceptContextCache struct {
	mu        sync.RWMutex
	resp      *AcceptContextResponse
	expiresAt time.Time
	ttl       time.Duration
}

func (c *acceptContextCache) load() *AcceptContextResponse {
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

func (c *acceptContextCache) store(resp *AcceptContextResponse) {
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
	p2          *ledger.Client
	issuer      string
	domain      string
	cache       *factoryCache
	acceptCache *acceptContextCache
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
		p2:          p2,
		issuer:      issuer,
		domain:      domain,
		cache:       &factoryCache{ttl: *cacheTTL},
		acceptCache: &acceptContextCache{ttl: *cacheTTL},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/registry/transfer-instruction/v1/transfer-factory", srv.handleTransferFactory)
	// Receiver-side accept context endpoint — matches DA's registrar URL pattern.
	mux.HandleFunc("/api/token-standard/v0/registrars/", srv.handleAcceptContext)
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

// ─── Sender-side handler ──────────────────────────────────────────────────────

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
		log.Printf("ERROR: query AllocationFactory: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorBody{Error: "failed to retrieve transfer factory"})
		return
	}

	s.cache.store(resp)
	writeJSON(w, http.StatusOK, resp)
}

// queryFactory queries P2's ACS for the active AllocationFactory and the
// InstrumentConfiguration referenced by it. AllocationFactory's
// TransferFactory_Transfer choice requires the sender-side context entries
// (instrument-configuration + empty sender-credentials list) and the
// InstrumentConfiguration contract to be disclosed so a participant that
// doesn't hold it in its own ACS (e.g. P1 when sender is on P2's-issuer-token)
// can still fetch it during interpretation.
func (s *server) queryFactory(ctx context.Context) (*RegistryResponse, error) {
	authCtx := s.p2.AuthContext(ctx)

	end, err := s.p2.GetLedgerEnd(authCtx)
	if err != nil {
		return nil, fmt.Errorf("get ledger end: %w", err)
	}
	if end == 0 {
		return nil, fmt.Errorf("ledger is empty")
	}

	factoryTID := &lapiv2.Identifier{
		PackageId:  *registryAppPkgID,
		ModuleName: "Utility.Registry.App.V0.Service.AllocationFactory",
		EntityName: "AllocationFactory",
	}
	instrumentConfigTID := &lapiv2.Identifier{
		PackageId:  *registryPkgID,
		ModuleName: "Utility.Registry.V0.Configuration.Instrument",
		EntityName: "InstrumentConfiguration",
	}

	factoryCID, factoryBlob, err := s.fetchContractFromACS(authCtx, end, factoryTID)
	if err != nil {
		return nil, fmt.Errorf("AllocationFactory: %w", err)
	}
	configCID, configBlob, err := s.fetchContractFromACS(authCtx, end, instrumentConfigTID)
	if err != nil {
		return nil, fmt.Errorf("InstrumentConfiguration: %w", err)
	}

	templateIDStr := func(id *lapiv2.Identifier) string {
		return fmt.Sprintf("%s:%s:%s", id.PackageId, id.ModuleName, id.EntityName)
	}
	return &RegistryResponse{
		FactoryID:    factoryCID,
		TransferKind: "transfer",
		ChoiceContext: map[string]any{
			"values": map[string]any{
				"utility.digitalasset.com/instrument-configuration": map[string]string{
					"tag": "AV_ContractId", "value": configCID,
				},
				"utility.digitalasset.com/sender-credentials": map[string]any{
					"tag": "AV_List", "value": []any{},
				},
			},
		},
		DisclosedContracts: []DisclosedContract{
			{
				ContractID:       factoryCID,
				CreatedEventBlob: base64.StdEncoding.EncodeToString(factoryBlob),
				TemplateID:       templateIDStr(factoryTID),
				SynchronizerID:   s.domain,
			},
			{
				ContractID:       configCID,
				CreatedEventBlob: base64.StdEncoding.EncodeToString(configBlob),
				TemplateID:       templateIDStr(instrumentConfigTID),
				SynchronizerID:   s.domain,
			},
		},
	}, nil
}

// ─── Receiver-side handler ────────────────────────────────────────────────────

// handleAcceptContext implements:
//
//	POST /api/token-standard/v0/registrars/{registrar}/registry/transfer-instruction/v1/{cid}/choice-contexts/accept
//
// Returns the choiceContextData (TransferRule + InstrumentConfiguration contract IDs as
// AV_ContractId AnyValues) and their createdEventBlobs as disclosedContracts.
// The {cid} path parameter is accepted but not used — context is instrument-level,
// not per-offer, for the local devstack with a single instrument.
func (s *server) handleAcceptContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorBody{Error: "method not allowed"})
		return
	}

	// Path: /api/token-standard/v0/registrars/{registrar}/registry/transfer-instruction/v1/{cid}/choice-contexts/accept
	// After stripping the registered prefix "/api/token-standard/v0/registrars/" the remainder is:
	// {registrar}/registry/transfer-instruction/v1/{cid}/choice-contexts/accept
	remainder := strings.TrimPrefix(r.URL.Path, "/api/token-standard/v0/registrars/")
	parts := strings.SplitN(remainder, "/", 8)
	// parts[0]=registrar, [1]=registry, [2]=transfer-instruction, [3]=v1, [4]=cid,
	// [5]=choice-contexts, [6]=accept
	if len(parts) < 7 || parts[1] != "registry" || parts[5] != "choice-contexts" || parts[6] != "accept" {
		writeJSON(w, http.StatusNotFound, errorBody{Error: "not found"})
		return
	}
	registrar := parts[0]

	if registrar != s.issuer {
		writeJSON(w, http.StatusBadRequest, errorBody{
			Error: fmt.Sprintf("registrar %q not managed by this registry", registrar),
		})
		return
	}

	// Drain request body (ignored — request fields not used for local single-instrument setup)
	_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 1<<20))
	_ = r.Body.Close()

	if resp := s.acceptCache.load(); resp != nil {
		writeJSON(w, http.StatusOK, resp)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	resp, err := s.queryAcceptContext(ctx)
	if err != nil {
		log.Printf("ERROR: query accept context: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorBody{Error: "failed to build accept context"})
		return
	}

	s.acceptCache.store(resp)
	writeJSON(w, http.StatusOK, resp)
}

// queryAcceptContext fetches TransferRule and InstrumentConfiguration from P2's
// ACS and builds the AcceptContextResponse. Both contracts are disclosed so the
// receiver's participant can verify them without holding them in its own ACS.
func (s *server) queryAcceptContext(ctx context.Context) (*AcceptContextResponse, error) {
	authCtx := s.p2.AuthContext(ctx)

	end, err := s.p2.GetLedgerEnd(authCtx)
	if err != nil {
		return nil, fmt.Errorf("get ledger end: %w", err)
	}
	if end == 0 {
		return nil, fmt.Errorf("ledger is empty")
	}

	transferRuleTID := &lapiv2.Identifier{
		PackageId:  *registryPkgID,
		ModuleName: "Utility.Registry.V0.Rule.Transfer",
		EntityName: "TransferRule",
	}
	instrumentConfigTID := &lapiv2.Identifier{
		PackageId:  *registryPkgID,
		ModuleName: "Utility.Registry.V0.Configuration.Instrument",
		EntityName: "InstrumentConfiguration",
	}

	transferRuleCID, transferRuleBlob, err := s.fetchContractFromACS(authCtx, end, transferRuleTID)
	if err != nil {
		return nil, fmt.Errorf("TransferRule: %w", err)
	}

	instrumentConfigCID, instrumentConfigBlob, err := s.fetchContractFromACS(authCtx, end, instrumentConfigTID)
	if err != nil {
		return nil, fmt.Errorf("InstrumentConfiguration: %w", err)
	}

	templateIDStr := func(id *lapiv2.Identifier) string {
		return fmt.Sprintf("%s:%s:%s", id.PackageId, id.ModuleName, id.EntityName)
	}

	return &AcceptContextResponse{
		ChoiceContextData: acceptChoiceContextData{
			Values: map[string]any{
				"utility.digitalasset.com/transfer-rule": map[string]string{
					"tag": "AV_ContractId", "value": transferRuleCID,
				},
				"utility.digitalasset.com/instrument-configuration": map[string]string{
					"tag": "AV_ContractId", "value": instrumentConfigCID,
				},
				"utility.digitalasset.com/sender-credentials": map[string]any{
					"tag": "AV_List", "value": []any{},
				},
				"utility.digitalasset.com/receiver-credentials": map[string]any{
					"tag": "AV_List", "value": []any{},
				},
			},
		},
		DisclosedContracts: []DisclosedContract{
			{
				ContractID:       transferRuleCID,
				CreatedEventBlob: base64.StdEncoding.EncodeToString(transferRuleBlob),
				TemplateID:       templateIDStr(transferRuleTID),
				SynchronizerID:   s.domain,
			},
			{
				ContractID:       instrumentConfigCID,
				CreatedEventBlob: base64.StdEncoding.EncodeToString(instrumentConfigBlob),
				TemplateID:       templateIDStr(instrumentConfigTID),
				SynchronizerID:   s.domain,
			},
		},
	}, nil
}

// ─── Shared ACS helper ────────────────────────────────────────────────────────

// fetchContractFromACS queries P2's ACS for the first active contract matching tid,
// using FiltersForAnyParty so it works regardless of which party hosts the contract.
// IncludeCreatedEventBlob is set so the blob can be returned as a disclosedContract.
func (s *server) fetchContractFromACS(authCtx context.Context, end int64, tid *lapiv2.Identifier) (contractID string, blob []byte, err error) {
	stream, err := s.p2.State().GetActiveContracts(authCtx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: end,
		EventFormat: &lapiv2.EventFormat{
			FiltersForAnyParty: &lapiv2.Filters{
				Cumulative: []*lapiv2.CumulativeFilter{{
					IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
						TemplateFilter: &lapiv2.TemplateFilter{
							TemplateId:              tid,
							IncludeCreatedEventBlob: true,
						},
					},
				}},
			},
			Verbose: false,
		},
	})
	if err != nil {
		return "", nil, fmt.Errorf("get active contracts (%s): %w", tid.EntityName, err)
	}

	for {
		msg, recvErr := stream.Recv()
		if recvErr != nil {
			if errors.Is(recvErr, io.EOF) {
				break
			}
			return "", nil, fmt.Errorf("recv (%s): %w", tid.EntityName, recvErr)
		}
		if ac := msg.GetActiveContract(); ac != nil && ac.CreatedEvent != nil {
			return ac.CreatedEvent.ContractId, ac.CreatedEvent.CreatedEventBlob, nil
		}
	}
	return "", nil, fmt.Errorf("%s not found on P2 ACS (has bootstrap-usdcx run?)", tid.EntityName)
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
