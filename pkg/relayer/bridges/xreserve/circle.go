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

// ErrAttestationUnavailable means the service was unreachable or 5xx'd —
// transient, so the adapter keeps polling instead of burning retries.
var ErrAttestationUnavailable = errors.New("attestation service unavailable")

const maxAttestationResponseBytes = 1 << 20 // 1MB

// attestationStatusComplete is the terminal attestation status.
const attestationStatusComplete = "complete"

// ErrReleasePending means Circle has not yet released the burned amount.
var ErrReleasePending = errors.New("release pending")

// isTransientStatus reports statuses worth retrying rather than failing on.
func isTransientStatus(code int) bool {
	return code >= http.StatusInternalServerError ||
		code == http.StatusTooManyRequests ||
		code == http.StatusRequestTimeout
}

// Attestation is Circle's signed confirmation that a deposit into the
// xReserve contract is final.
type Attestation struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// burnStatusReleased is the terminal burn status.
const burnStatusReleased = "released"

// BurnStatus reports the destination-chain release of a Canton burn.
type BurnStatus struct {
	Status string `json:"status"`
	TxHash string `json:"tx_hash"`
}

// AttestationClient is the Circle xReserve API surface the adapter polls:
// deposit attestations inbound, burn releases outbound.
type AttestationClient interface {
	// GetAttestation returns the attestation for a deposit transaction hash.
	// Returns ErrAttestationNotReady while Circle has not attested and
	// ErrAttestationUnavailable on transient service failures.
	GetAttestation(ctx context.Context, depositTxHash string) (*Attestation, error)
	// GetBurnStatus returns the release status for a burn request id.
	// Returns ErrReleasePending until Circle has released and
	// ErrAttestationUnavailable on transient service failures.
	GetBurnStatus(ctx context.Context, burnRequestID string) (*BurnStatus, error)
}

// HTTPAttestationClient talks to the xReserve attestation REST API.
// Paths/shapes match the devstack stub; confirm against Circle before mainnet (#360).
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
	var att Attestation
	reqURL := c.baseURL + "/v1/attestations/" + url.PathEscape(depositTxHash)
	if err := c.getJSON(ctx, reqURL, ErrAttestationNotReady, &att); err != nil {
		return nil, err
	}
	if !strings.EqualFold(att.Status, attestationStatusComplete) {
		return nil, ErrAttestationNotReady
	}
	return &att, nil
}

// GetBurnStatus fetches the release status for a burn request id.
func (c *HTTPAttestationClient) GetBurnStatus(ctx context.Context, burnRequestID string) (*BurnStatus, error) {
	var st BurnStatus
	reqURL := c.baseURL + "/v1/burns/" + url.PathEscape(burnRequestID)
	if err := c.getJSON(ctx, reqURL, ErrReleasePending, &st); err != nil {
		return nil, err
	}
	if !strings.EqualFold(st.Status, burnStatusReleased) {
		return nil, ErrReleasePending
	}
	return &st, nil
}

// getJSON GETs reqURL and decodes the 200 response into out. 404 maps to
// notFoundErr (both polling endpoints treat it as "not yet"), 5xx and
// transport failures to ErrAttestationUnavailable.
func (c *HTTPAttestationClient) getJSON(ctx context.Context, reqURL string, notFoundErr error, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrAttestationUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAttestationResponseBytes))
	if err != nil {
		return fmt.Errorf("%w: read response: %w", ErrAttestationUnavailable, err)
	}

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return notFoundErr
	case isTransientStatus(resp.StatusCode):
		// 5xx, 429 (rate limit), 408 (request timeout): transient. Keep
		// polling rather than burning a transfer retry on a healthy transfer.
		return fmt.Errorf("%w: status %d: %s", ErrAttestationUnavailable, resp.StatusCode, body)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("xreserve api returned %d: %s", resp.StatusCode, body)
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	return nil
}
