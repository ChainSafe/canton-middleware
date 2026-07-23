// SPDX-License-Identifier: Apache-2.0

package xreserve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ErrAttestationNotReady means Circle has not (yet) attested the deposit —
// expected during the source-chain finality window (~15 min on Ethereum).
var ErrAttestationNotReady = errors.New("attestation not ready")

// ErrAttestationUnavailable means the attestation service could not be
// reached or answered with a server error. Transient by definition: the
// adapter keeps polling instead of burning transfer retries.
var ErrAttestationUnavailable = errors.New("attestation service unavailable")

const maxAttestationResponseBytes = 1 << 20 // 1MB

// attestationStatusComplete is the terminal attestation status.
const attestationStatusComplete = "complete"

// Attestation is Circle's signed confirmation that a deposit into the
// xReserve contract is final.
type Attestation struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// AttestationClient fetches deposit attestations from Circle's xReserve API.
type AttestationClient interface {
	// GetAttestation returns the attestation for a deposit transaction hash.
	// Returns ErrAttestationNotReady while Circle has not attested and
	// ErrAttestationUnavailable on transient service failures.
	GetAttestation(ctx context.Context, depositTxHash string) (*Attestation, error)
}

// HTTPAttestationClient talks to the xReserve attestation REST API.
//
// The path and response shape follow the devstack attestation stub; Circle's
// production API schema must be confirmed before mainnet enablement (#360).
type HTTPAttestationClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewAttestationClient creates an attestation client for the given base URL.
// A nil httpClient falls back to a default client; callers are expected to
// inject one with a timeout.
func NewAttestationClient(baseURL string, httpClient *http.Client) (*HTTPAttestationClient, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("invalid attestation base URL %q", baseURL)
	}
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &HTTPAttestationClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}, nil
}

// GetAttestation fetches the attestation for a deposit tx hash.
func (c *HTTPAttestationClient) GetAttestation(ctx context.Context, depositTxHash string) (*Attestation, error) {
	reqURL := c.baseURL + "/v1/attestations/" + url.PathEscape(depositTxHash)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build attestation request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAttestationUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAttestationResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %w", ErrAttestationUnavailable, err)
	}

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrAttestationNotReady
	case resp.StatusCode >= http.StatusInternalServerError:
		return nil, fmt.Errorf("%w: status %d: %s", ErrAttestationUnavailable, resp.StatusCode, body)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("attestation service returned %d: %s", resp.StatusCode, body)
	}

	var att Attestation
	if err := json.Unmarshal(body, &att); err != nil {
		return nil, fmt.Errorf("parse attestation response: %w", err)
	}
	if !strings.EqualFold(att.Status, attestationStatusComplete) {
		return nil, ErrAttestationNotReady
	}
	return &att, nil
}
