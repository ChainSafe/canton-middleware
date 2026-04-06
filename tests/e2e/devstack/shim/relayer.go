//go:build e2e

package shim

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/relayer"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
)

// RelayerShim implements stack.Relayer via HTTP.
type RelayerShim struct {
	endpoint string
	client   *http.Client
}

// NewRelayer returns a RelayerShim for the relayer endpoint in the manifest.
func NewRelayer(manifest *stack.ServiceManifest) *RelayerShim {
	return &RelayerShim{
		endpoint: manifest.RelayerHTTP,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (r *RelayerShim) Endpoint() string { return r.endpoint }

// Health returns nil when GET /health responds with 200.
func (r *RelayerShim) Health(ctx context.Context) error {
	return r.getOK(ctx, "/health")
}

// IsReady returns true when GET /ready responds with 200.
func (r *RelayerShim) IsReady(ctx context.Context) bool {
	return r.getOK(ctx, "/ready") == nil
}

// ListTransfers returns all transfers from GET /api/v1/transfers.
func (r *RelayerShim) ListTransfers(ctx context.Context) ([]*relayer.Transfer, error) {
	var resp struct {
		Transfers []*relayer.Transfer `json:"transfers"`
	}
	if err := r.get(ctx, "/api/v1/transfers", &resp); err != nil {
		return nil, err
	}
	return resp.Transfers, nil
}

// GetTransfer returns a single transfer by ID from GET /api/v1/transfers/{id}.
func (r *RelayerShim) GetTransfer(ctx context.Context, id string) (*relayer.Transfer, error) {
	var transfer relayer.Transfer
	if err := r.get(ctx, fmt.Sprintf("/api/v1/transfers/%s", id), &transfer); err != nil {
		return nil, err
	}
	return &transfer, nil
}

// Status returns the relayer status string from GET /api/v1/status.
func (r *RelayerShim) Status(ctx context.Context) (string, error) {
	var resp struct {
		Status string `json:"status"`
	}
	if err := r.get(ctx, "/api/v1/status", &resp); err != nil {
		return "", err
	}
	return resp.Status, nil
}

func (r *RelayerShim) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.endpoint+path, nil)
	if err != nil {
		return err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("GET %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (r *RelayerShim) getOK(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.endpoint+path, nil)
	if err != nil {
		return err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d", path, resp.StatusCode)
	}
	return nil
}
