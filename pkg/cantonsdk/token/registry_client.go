// SPDX-License-Identifier: Apache-2.0

package token

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
)

const (
	registryPathFmt = "/api/token-standard/v0/registrars/%s/registry/transfer-instruction/v1/transfer-factory"
	// choiceContextPathFmt is the registrar's per-instruction choice-context
	// endpoint. The final %s is the action ("accept" or "withdraw").
	choiceContextPathFmt = "/api/token-standard/v0/registrars/%s/registry/transfer-instruction/v1/%s/choice-contexts/%s"
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
// Shape matches the Splice token-standard spec as hosted by DA's utilities API
// (e.g. api.utilities.digitalasset-dev.com): choice arguments are wrapped in
// `choiceArguments`, with a sibling `excludeDebugFields` flag.
type RegistryRequest struct {
	ChoiceArguments    ChoiceArguments `json:"choiceArguments"`
	ExcludeDebugFields bool            `json:"excludeDebugFields"`
}

// ChoiceArguments wraps the on-ledger TransferFactory_Transfer choice arguments
// alongside the off-ledger extraArgs (context + meta) consumed by the registry.
type ChoiceArguments struct {
	ExpectedAdmin string                 `json:"expectedAdmin"`
	Transfer      RegistryTransferDetail `json:"transfer"`
	ExtraArgs     ExtraArgs              `json:"extraArgs"`
}

// ExtraArgs carries the off-ledger context and meta AnyValue maps required
// by the AllocationFactory's TransferFactory_Transfer choice.
type ExtraArgs struct {
	Context AnyValueMap `json:"context"`
	Meta    AnyValueMap `json:"meta"`
}

// AnyValueMap models the `{"values": {...}}` AnyValue container that wraps
// metadata and context maps in the Splice token-standard JSON.
type AnyValueMap struct {
	Values map[string]any `json:"values"`
}

// RegistryTransferDetail contains the transfer parameters for registry lookup.
// `executeBefore` and `requestedAt` are RFC3339 timestamps; the registry uses
// them to assert the transfer falls within the issuer's validity window.
type RegistryTransferDetail struct {
	Sender           string        `json:"sender"`
	Receiver         string        `json:"receiver"`
	Amount           string        `json:"amount"`
	InstrumentID     InstrumentRef `json:"instrumentId"`
	InputHoldingCIDs []string      `json:"inputHoldingCids"`
	Meta             AnyValueMap   `json:"meta"`
	ExecuteBefore    string        `json:"executeBefore"`
	RequestedAt      string        `json:"requestedAt"`
}

// InstrumentRef is the structured instrument identifier the registry expects:
// the issuer admin party plus the instrument id within that issuer's namespace.
type InstrumentRef struct {
	Admin string `json:"admin"`
	ID    string `json:"id"`
}

// RegistryResponse is the registry response as consumed by the SDK.
// `ChoiceContext` holds the AnyValue map (i.e. `choiceContext.choiceContextData`
// from the wire shape) and `DisclosedContracts` holds the array — both lifted
// out of the wire envelope by GetTransferFactory so the downstream converters
// (ConvertAnyValueChoiceContext / ConvertDisclosedContracts) can consume them
// directly.
type RegistryResponse struct {
	FactoryID          string          `json:"factoryId"`
	TransferKind       string          `json:"transferKind"`
	ChoiceContext      json.RawMessage `json:"choiceContext"`
	DisclosedContracts json.RawMessage `json:"disclosedContracts"`
}

// registryWireResponse mirrors DA's hosted token-standard registry response,
// where the AnyValue choice context and the disclosed contracts are both
// nested inside `choiceContext` (matching the receiver-side AcceptContextResponse).
type registryWireResponse struct {
	FactoryID     string          `json:"factoryId"`
	TransferKind  string          `json:"transferKind"`
	ChoiceContext json.RawMessage `json:"choiceContext"`
}

type registryWireChoiceContext struct {
	ChoiceContextData  json.RawMessage `json:"choiceContextData"`
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
// registrarParty is the issuer's party ID under whose namespace the registry is mounted
// (DA's hosted multi-registrar API multiplexes registrars by URL path).
func (rc *RegistryClient) GetTransferFactory(
	ctx context.Context, registryBaseURL, registrarParty string, req *RegistryRequest,
) (*RegistryResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal registry request: %w", err)
	}

	reqURL := strings.TrimRight(registryBaseURL, "/") + fmt.Sprintf(registryPathFmt, url.PathEscape(registrarParty))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
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

	var wire registryWireResponse
	if err := json.Unmarshal(respBody, &wire); err != nil {
		return nil, fmt.Errorf("parse registry response: %w", err)
	}

	// DA's hosted registry wraps `choiceContextData` and `disclosedContracts`
	// inside `choiceContext` — lift them out so callers see the legacy flat
	// shape the SDK's converters already understand. A null/absent
	// choiceContext leaves both fields empty, which the converters short-circuit.
	var inner registryWireChoiceContext
	if len(wire.ChoiceContext) > 0 && string(wire.ChoiceContext) != jsonNull {
		if err := json.Unmarshal(wire.ChoiceContext, &inner); err != nil {
			return nil, fmt.Errorf("parse choiceContext wrapper: %w", err)
		}
	}

	return &RegistryResponse{
		FactoryID:          wire.FactoryID,
		TransferKind:       wire.TransferKind,
		ChoiceContext:      inner.ChoiceContextData,
		DisclosedContracts: inner.DisclosedContracts,
	}, nil
}

// ConvertDisclosedContracts parses the registry's disclosed contracts JSON into proto messages.
func ConvertDisclosedContracts(raw json.RawMessage, fallbackDomainID string) ([]*lapiv2.DisclosedContract, error) {
	if len(raw) == 0 || string(raw) == jsonNull {
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

// jsonNull is the literal JSON null token. Registry endpoints commonly send this
// for absent choice contexts; both converters short-circuit on it.
const jsonNull = "null"

// ConvertChoiceContext parses the registry's choice context JSON into a map suitable for EncodeExtraArgs.
// Returns nil for null or empty input. Used for the legacy TextMap-of-Text shape.
// AllocationFactory-based registries return the richer AnyValue shape, which is parsed by
// ConvertAnyValueChoiceContext instead.
func ConvertChoiceContext(raw json.RawMessage) (map[string]string, error) {
	if len(raw) == 0 || string(raw) == jsonNull {
		return nil, nil
	}
	// Treat the AnyValue shape (`{"values": {...}}`) as absent for the legacy
	// converter — callers using AllocationFactory should branch to
	// ConvertAnyValueChoiceContext when the JSON looks like an AnyValue map.
	var peek struct {
		Values json.RawMessage `json:"values"`
	}
	if err := json.Unmarshal(raw, &peek); err == nil && len(peek.Values) > 0 {
		return nil, nil
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse choice context: %w", err)
	}
	return m, nil
}

// ConvertAnyValueChoiceContext parses an AnyValue-shaped registry choice context
// (`{"values": {key: {tag, value}, ...}}`) into AcceptChoiceContext. Used when the
// registry returns context entries for AllocationFactory's TransferFactory_Transfer
// choice (instrument-configuration as AV_ContractId, sender-credentials as AV_List).
// Returns an empty context when raw is null or doesn't carry the AnyValue shape.
func ConvertAnyValueChoiceContext(raw json.RawMessage) (AcceptChoiceContext, error) {
	if len(raw) == 0 || string(raw) == jsonNull {
		return AcceptChoiceContext{}, nil
	}
	var ctx AcceptChoiceContext
	if err := json.Unmarshal(raw, &ctx); err != nil {
		return AcceptChoiceContext{}, fmt.Errorf("parse anyvalue choice context: %w", err)
	}
	return ctx, nil
}

// GetAcceptChoiceContext calls the registrar's accept choice-context endpoint for a pending
// TransferInstruction. Returns the choiceContextData (AnyValue map) and disclosed contracts
// needed to exercise TransferInstruction_Accept.
func (rc *RegistryClient) GetAcceptChoiceContext(
	ctx context.Context, registryBaseURL, registrarParty, instructionCID string,
) (*AcceptContextResponse, error) {
	return rc.getChoiceContext(ctx, registryBaseURL, registrarParty, instructionCID, "accept")
}

// GetWithdrawChoiceContext calls the registrar's withdraw choice-context endpoint for a
// TransferInstruction. Returns the choiceContextData and disclosed contracts needed to
// exercise TransferInstruction_Withdraw (sender reclaims a pending/expired offer).
func (rc *RegistryClient) GetWithdrawChoiceContext(
	ctx context.Context, registryBaseURL, registrarParty, instructionCID string,
) (*AcceptContextResponse, error) {
	return rc.getChoiceContext(ctx, registryBaseURL, registrarParty, instructionCID, "withdraw")
}

// getChoiceContext fetches a per-instruction choice-context for the given action
// ("accept" or "withdraw"). The response shape is identical across actions, so both
// the accept and withdraw flows share this. action is interpolated into the registrar
// endpoint path and the error messages.
func (rc *RegistryClient) getChoiceContext(
	ctx context.Context, registryBaseURL, registrarParty, instructionCID, action string,
) (*AcceptContextResponse, error) {
	path := fmt.Sprintf(choiceContextPathFmt, registrarParty, instructionCID, action)
	url := strings.TrimRight(registryBaseURL, "/") + path
	body := []byte(`{"meta":{},"excludeDebugFields":false}`)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create %s context request: %w", action, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := rc.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s context request failed: %w", action, err)
	}
	defer resp.Body.Close()

	const maxResponseBytes = 1 << 20
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read %s context response: %w", action, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s context returned %d: %s", action, resp.StatusCode, string(respBody))
	}

	var result AcceptContextResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse %s context response: %w", action, err)
	}
	return &result, nil
}
