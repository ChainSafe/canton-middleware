//go:build e2e

package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
)

// TestTransferFactory_ReturnsContractID verifies that POST
// /registry/transfer-instruction/v1/transfer-factory returns a non-empty
// contract_id. This exercises the Splice Registry API endpoint used by Canton
// Loop wallet to discover the active TransferFactory contract.
func TestTransferFactory_ReturnsContractID(t *testing.T) {
	sys := presets.NewAPIStack(t)
	t.Parallel()
	ctx := context.Background()

	resp, err := sys.APIServer.TransferFactory(ctx)
	if err != nil {
		t.Fatalf("POST /registry/.../transfer-factory: %v", err)
	}
	if resp.ContractID == "" {
		t.Fatal("expected non-empty contract_id in TransferFactory response")
	}
}

// TestTransferFactory_MethodNotAllowed verifies that GET
// /registry/transfer-instruction/v1/transfer-factory returns HTTP 405. The
// handler only accepts POST per the Splice Registry API spec.
func TestTransferFactory_MethodNotAllowed(t *testing.T) {
	sys := presets.NewAPIStack(t)
	t.Parallel()
	ctx := context.Background()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		sys.APIServer.Endpoint()+"/registry/transfer-instruction/v1/transfer-factory", nil)
	if err != nil {
		t.Fatalf("build GET request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET transfer-factory: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected HTTP 405 for GET transfer-factory, got %d", resp.StatusCode)
	}
}
