package token

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
)

const (
	registryPath        = "/registry/transfer-instruction/v1/transfer-factory"
	acceptContextPathFmt = "/api/token-standard/v0/registrars/%s/registry/transfer-instruction/v1/%s/choice-contexts/accept"
)

// RegistryClient calls the Splice Transfer Factory Registry API to discover
// transfer factories for external tokens (e.g., USDCx).
type RegistryClient struct {
	httpClient *http.Client
}

// NewRegistryClient creates a new registry client.
func NewRegistryClient(httpClient *http.Client) *RegistryClient {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &RegistryClient{httpClient: httpClient}
}

// RegistryRequest is the POST body for the Transfer Factory Registry API.
type RegistryRequest struct {
	ExpectedAdmin string                 `json:"expectedAdmin"`
	Transfer      RegistryTransferDetail `json:"transfer"`
	ExtraArgs     map[string]any         `json:"extraArgs,omitempty"`
}

// RegistryTransferDetail contains the transfer parameters for registry lookup.
type RegistryTransferDetail struct {
	Sender           string   `json:"sender"`
	Receiver         string   `json:"receiver"`
	Amount           string   `json:"amount"`
	InstrumentID     string   `json:"instrumentId"`
	InputHoldingCIDs []string `json:"inputHoldingCids"`
}

// RegistryResponse is the response from the Transfer Factory Registry API.
type RegistryResponse struct {
	FactoryID          string          `json:"factoryId"`
	TransferKind       string          `json:"transferKind"`
	ChoiceContext      json.RawMessage `json:"choiceContext"`
	DisclosedContracts json.RawMessage `json:"disclosedContracts"`
}

// registryDisclosedContract is the JSON shape of a disclosed contract from the registry.
// TemplateID is json.RawMessage because registries may return it as a string
// ("pkg:module:entity") or as an object ({packageId, moduleName, entityName}).
type registryDisclosedContract struct {
	ContractID       string          `json:"contractId"`
	CreatedEventBlob string          `json:"createdEventBlob"` // base64
	TemplateID       json.RawMessage `json:"templateId"`
	SynchronizerID   string          `json:"synchronizerId"`
}

// GetTransferFactory calls the registry to discover the transfer factory for an external token.
func (rc *RegistryClient) GetTransferFactory(ctx context.Context, registryBaseURL string, req *RegistryRequest) (*RegistryResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal registry request: %w", err)
	}

	url := strings.TrimRight(registryBaseURL, "/") + registryPath
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create registry request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := rc.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("registry request failed: %w", err)
	}
	defer resp.Body.Close()

	const maxResponseBytes = 1 << 20 // 1 MB
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read registry response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result RegistryResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse registry response: %w", err)
	}

	return &result, nil
}

// ConvertDisclosedContracts parses the registry's disclosed contracts JSON into proto messages.
func ConvertDisclosedContracts(raw json.RawMessage, fallbackDomainID string) ([]*lapiv2.DisclosedContract, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var contracts []registryDisclosedContract
	if err := json.Unmarshal(raw, &contracts); err != nil {
		return nil, fmt.Errorf("parse disclosed contracts: %w", err)
	}
	return convertDisclosedContractSlice(contracts, fallbackDomainID)
}

// convertDisclosedContractSlice converts a slice of registry contracts into proto messages.
func convertDisclosedContractSlice(contracts []registryDisclosedContract, fallbackDomainID string) ([]*lapiv2.DisclosedContract, error) {
	out := make([]*lapiv2.DisclosedContract, 0, len(contracts))
	for _, c := range contracts {
		blob, err := base64.StdEncoding.DecodeString(c.CreatedEventBlob)
		if err != nil {
			return nil, fmt.Errorf("decode created_event_blob for %s: %w", c.ContractID, err)
		}
		tid, err := parseTemplateID(c.TemplateID)
		if err != nil {
			return nil, fmt.Errorf("parse templateId for %s: %w", c.ContractID, err)
		}
		domainID := c.SynchronizerID
		if domainID == "" {
			domainID = fallbackDomainID
		}
		out = append(out, &lapiv2.DisclosedContract{
			ContractId:       c.ContractID,
			CreatedEventBlob: blob,
			SynchronizerId:   domainID,
			TemplateId:       tid,
		})
	}
	return out, nil
}

// parseTemplateID parses a templateId that is either a "pkg:module:entity" string
// or a {packageId, moduleName, entityName} JSON object. Returns nil for empty input.
func parseTemplateID(raw json.RawMessage) (*lapiv2.Identifier, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && s != "" {
		parts := strings.SplitN(s, ":", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("templateId %q not in pkg:module:entity form", s)
		}
		return &lapiv2.Identifier{PackageId: parts[0], ModuleName: parts[1], EntityName: parts[2]}, nil
	}
	var obj struct {
		PackageID  string `json:"packageId"`
		ModuleName string `json:"moduleName"`
		EntityName string `json:"entityName"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("parse templateId: %w", err)
	}
	return &lapiv2.Identifier{PackageId: obj.PackageID, ModuleName: obj.ModuleName, EntityName: obj.EntityName}, nil
}

// ConvertChoiceContext parses the registry's choice context JSON into a map suitable for EncodeExtraArgs.
// Returns nil for null or empty input (AllocationFactory returns null).
func ConvertChoiceContext(raw json.RawMessage) (map[string]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse choice context: %w", err)
	}
	return m, nil
}

// GetAcceptChoiceContext calls the registrar's accept choice-context endpoint for a pending
// TransferInstruction. Returns the choiceContextData (AnyValue map) and disclosed contracts
// needed to exercise TransferInstruction_Accept.
func (rc *RegistryClient) GetAcceptChoiceContext(ctx context.Context, registryBaseURL, registrarParty, instructionCID string) (*AcceptContextResponse, error) {
	path := fmt.Sprintf(acceptContextPathFmt, registrarParty, instructionCID)
	url := strings.TrimRight(registryBaseURL, "/") + path
	body := []byte(`{"meta":{},"excludeDebugFields":false}`)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create accept context request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := rc.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("accept context request failed: %w", err)
	}
	defer resp.Body.Close()

	const maxResponseBytes = 1 << 20
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read accept context response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("accept context returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result AcceptContextResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse accept context response: %w", err)
	}
	return &result, nil
}
