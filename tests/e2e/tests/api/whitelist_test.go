//go:build e2e

package api_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/shim"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
)

// whitelistOccurrences counts how many times addr appears across the whole
// whitelist, paging through the cursor-based admin list. Matching is
// case-insensitive since EVM addresses are case-insensitive identifiers. Paging
// keeps the assertion correct regardless of how many other (parallel) tests have
// added addresses to the shared table.
func whitelistOccurrences(ctx context.Context, t *testing.T, api stack.APIServer, addr string) int {
	t.Helper()
	n, cursor := 0, ""
	for {
		page, err := api.ListWhitelist(ctx, cursor, 200)
		if err != nil {
			t.Fatalf("list whitelist: %v", err)
		}
		for _, e := range page.Items {
			if strings.EqualFold(e.EVMAddress, addr) {
				n++
			}
		}
		// Stop when the server says there are no more pages, or defensively if it
		// claims more but gives no cursor to advance with (which would otherwise
		// loop forever on the same page).
		if !page.HasMore || page.NextCursor == "" {
			return n
		}
		cursor = page.NextCursor
	}
}

// TestWhitelist_Add verifies that POST /admin/whitelist adds an address and that
// it then appears in the list.
func TestWhitelist_Add(t *testing.T) {
	t.Parallel()

	sys := presets.NewAPIStack(t)
	ctx := context.Background()
	addr := sys.Accounts.User1.Address.Hex()

	if err := sys.APIServer.WhitelistAddress(ctx, addr); err != nil {
		t.Fatalf("add to whitelist: %v", err)
	}
	if got := whitelistOccurrences(ctx, t, sys.APIServer, addr); got != 1 {
		t.Fatalf("expected %s to appear once in whitelist, got %d", addr, got)
	}
}

// TestWhitelist_AddWhileExists verifies that adding an already-whitelisted
// address is idempotent — it succeeds and does not create a duplicate.
func TestWhitelist_AddWhileExists(t *testing.T) {
	t.Parallel()

	sys := presets.NewAPIStack(t)
	ctx := context.Background()
	addr := sys.Accounts.User1.Address.Hex()

	if err := sys.APIServer.WhitelistAddress(ctx, addr); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if err := sys.APIServer.WhitelistAddress(ctx, addr); err != nil {
		t.Fatalf("second add (while exists) should be idempotent, got: %v", err)
	}
	if got := whitelistOccurrences(ctx, t, sys.APIServer, addr); got != 1 {
		t.Fatalf("expected exactly one entry for %s after duplicate add, got %d", addr, got)
	}
}

// TestWhitelist_Remove verifies that DELETE /admin/whitelist/{address} removes a
// whitelisted address.
func TestWhitelist_Remove(t *testing.T) {
	t.Parallel()

	sys := presets.NewAPIStack(t)
	ctx := context.Background()
	addr := sys.Accounts.User1.Address.Hex()

	if err := sys.APIServer.WhitelistAddress(ctx, addr); err != nil {
		t.Fatalf("add to whitelist: %v", err)
	}
	if err := sys.APIServer.RemoveWhitelistAddress(ctx, addr); err != nil {
		t.Fatalf("remove from whitelist: %v", err)
	}
	if got := whitelistOccurrences(ctx, t, sys.APIServer, addr); got != 0 {
		t.Fatalf("expected %s to be absent after removal, found %d", addr, got)
	}
}

// TestWhitelist_RemoveWhileNotExists verifies that removing an address that was
// never whitelisted returns HTTP 404.
func TestWhitelist_RemoveWhileNotExists(t *testing.T) {
	t.Parallel()

	sys := presets.NewAPIStack(t)
	ctx := context.Background()
	addr := sys.Accounts.User1.Address.Hex() // derived from t.Name(), never added here

	err := sys.APIServer.RemoveWhitelistAddress(ctx, addr)
	var he *shim.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusNotFound {
		t.Fatalf("expected HTTP 404 removing a non-whitelisted address, got %v", err)
	}
}

// TestWhitelist_ListPagination verifies that the cursor and limit query
// parameters are actually applied: limit caps the page size, HasMore/NextCursor
// reflect that more rows exist, and passing the cursor back advances past the
// previous page (no repeats).
func TestWhitelist_ListPagination(t *testing.T) {
	t.Parallel()

	sys := presets.NewAPIStack(t)
	ctx := context.Background()

	// Ensure at least two entries exist so a limit of 1 must spill to a second page.
	for _, addr := range []string{sys.Accounts.User1.Address.Hex(), sys.Accounts.User2.Address.Hex()} {
		if err := sys.APIServer.WhitelistAddress(ctx, addr); err != nil {
			t.Fatalf("add %s: %v", addr, err)
		}
	}

	page1, err := sys.APIServer.ListWhitelist(ctx, "", 1)
	if err != nil {
		t.Fatalf("list page 1: %v", err)
	}
	if len(page1.Items) != 1 {
		t.Fatalf("limit=1 should return exactly one item, got %d", len(page1.Items))
	}
	if !page1.HasMore {
		t.Fatal("expected has_more=true when more than one address is whitelisted")
	}
	if page1.NextCursor != page1.Items[0].EVMAddress {
		t.Fatalf("next_cursor %q should equal the last item %q", page1.NextCursor, page1.Items[0].EVMAddress)
	}

	page2, err := sys.APIServer.ListWhitelist(ctx, page1.NextCursor, 1)
	if err != nil {
		t.Fatalf("list page 2: %v", err)
	}
	if len(page2.Items) != 1 {
		t.Fatalf("limit=1 should return exactly one item on page 2, got %d", len(page2.Items))
	}
	// The cursor is exclusive, so page 2 must not repeat page 1's row.
	if strings.EqualFold(page2.Items[0].EVMAddress, page1.Items[0].EVMAddress) {
		t.Fatalf("cursor did not advance: page 2 repeated %s", page1.Items[0].EVMAddress)
	}
}
