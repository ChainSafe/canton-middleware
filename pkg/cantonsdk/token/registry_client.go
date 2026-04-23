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

const registryPath = "/registry/transfer-instruction/v1/transfer-factory"

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
type registryDisclosedContract struct {
	ContractID       string `json:"contractId"`
	CreatedEventBlob string `json:"createdEventBlob"` // base64
	TemplateID       string `json:"templateId"`
	SynchronizerID   string `json:"synchronizerId"`
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

	out := make([]*lapiv2.DisclosedContract, 0, len(contracts))
	for _, c := range contracts {
		blob, err := base64.StdEncoding.DecodeString(c.CreatedEventBlob)
		if err != nil {
			return nil, fmt.Errorf("decode created_event_blob for %s: %w", c.ContractID, err)
		}

		domainID := c.SynchronizerID
		if domainID == "" {
			domainID = fallbackDomainID
		}

		out = append(out, &lapiv2.DisclosedContract{
			ContractId:       c.ContractID,
			CreatedEventBlob: blob,
			SynchronizerId:   domainID,
		})
	}
	return out, nil
}

// ConvertChoiceContext parses the registry's choice context JSON into a map suitable for EncodeExtraArgs.
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
