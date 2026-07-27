// SPDX-License-Identifier: Apache-2.0

package bridgeapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/relayer"
)

const maxRelayerResponseBytes = 1 << 20 // 1MB

// RelayerClient is the api-server's view of the relayer HTTP API: transfer
// registration at initiation time and status reads.
type RelayerClient interface {
	RegisterTransfer(ctx context.Context, req *relayer.RegisterTransferRequest) (*relayer.RegisterTransferResponse, error)
	GetTransfer(ctx context.Context, id string) (*relayer.Transfer, error)
}

// HTTPRelayerClient talks to the relayer's /api/v1 endpoints.
type HTTPRelayerClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewRelayerClient creates a relayer client for the given base URL. A nil
// httpClient falls back to a default client; callers should inject one with
// a timeout.
func NewRelayerClient(baseURL string, httpClient *http.Client) (*HTTPRelayerClient, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("invalid relayer base URL %q", baseURL)
	}
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &HTTPRelayerClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}, nil
}

// RegisterTransfer registers an observer-mechanism transfer with the relayer.
func (c *HTTPRelayerClient) RegisterTransfer(
	ctx context.Context,
	req *relayer.RegisterTransferRequest,
) (*relayer.RegisterTransferResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal register request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+"/api/v1/transfers", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build register request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("register transfer with relayer: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxRelayerResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read relayer response: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("relayer returned %d: %s", resp.StatusCode, payload)
	}

	var out relayer.RegisterTransferResponse
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, fmt.Errorf("parse relayer response: %w", err)
	}
	return &out, nil
}

// GetTransfer fetches a transfer's status from the relayer. Returns a
// ResourceNotFoundError on 404.
func (c *HTTPRelayerClient) GetTransfer(ctx context.Context, id string) (*relayer.Transfer, error) {
	reqURL := c.baseURL + "/api/v1/transfers/" + url.PathEscape(id)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build get-transfer request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("get transfer from relayer: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxRelayerResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read relayer response: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, apperrors.ResourceNotFoundError(nil, "transfer not found")
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("relayer returned %d: %s", resp.StatusCode, payload)
	}

	var out relayer.Transfer
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, fmt.Errorf("parse relayer response: %w", err)
	}
	return &out, nil
}
