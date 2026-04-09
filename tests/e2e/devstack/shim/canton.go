//go:build e2e

package shim

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
)

var _ stack.Canton = (*CantonShim)(nil)

// CantonShim implements stack.Canton.
type CantonShim struct {
	grpcEndpoint string
	httpEndpoint string
	client       *http.Client
}

// NewCanton returns a CantonShim wired to the endpoints in the manifest.
func NewCanton(manifest *stack.ServiceManifest) *CantonShim {
	return &CantonShim{
		grpcEndpoint: manifest.CantonGRPC,
		httpEndpoint: manifest.CantonHTTP,
		client:       &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *CantonShim) GRPCEndpoint() string { return c.grpcEndpoint }
func (c *CantonShim) HTTPEndpoint() string { return c.httpEndpoint }

// IsHealthy returns true when the Canton HTTP JSON API responds with 200. It
// does not block — callers should use WaitForCanton in the DSL for polling.
func (c *CantonShim) IsHealthy(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/version", c.httpEndpoint), nil)
	if err != nil {
		return false
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
