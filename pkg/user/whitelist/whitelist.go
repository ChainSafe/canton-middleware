// SPDX-License-Identifier: Apache-2.0

// Package whitelist is the registration-whitelist domain. Its Service provides
// two things from one place:
//
//   - the authorization gate (Checker.IsWhitelisted) shared by the
//     registration service and the Ethereum JSON-RPC facade. The skip decision
//     (skip_whitelist_check) is baked into the Service at construction time, so
//     consumers only ever ask "is this address allowed?" without carrying their
//     own skip flag and branch; and
//   - the admin operations (Manager: add, remove, cursor-paginated list) exposed
//     under /admin/whitelist.
package whitelist

import (
	"context"
	"errors"
	"fmt"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/chainsafe/canton-middleware/pkg/user"
)

// Cursor pagination bounds for List, mirroring the token list endpoint.
const (
	DefaultLimit = 50
	MaxLimit     = 200
)

// ErrEntryNotFound is returned by Remove when the address was not whitelisted.
var ErrEntryNotFound = errors.New("whitelist entry not found")

// Store is the persistence the whitelist needs. It is satisfied by the user
// store. ListWhitelist returns up to limit entries after cursor (the previous
// page's last evm_address; empty starts from the beginning).
//
//go:generate mockery --name Store --output mocks --outpkg mocks --filename mock_store.go --with-expecter
type Store interface {
	IsWhitelisted(ctx context.Context, evmAddress string) (bool, error)
	AddToWhitelist(ctx context.Context, evmAddress, note string) error
	RemoveFromWhitelist(ctx context.Context, evmAddress string) (removed bool, err error)
	ListWhitelist(ctx context.Context, cursor string, limit int) ([]*user.WhitelistEntry, error)
}

// Checker reports whether an EVM address is permitted. It is the narrow gate
// consumed by the registration service and the eth-rpc facade.
//
//go:generate mockery --name Checker --output mocks --outpkg mocks --filename mock_checker.go --with-expecter
type Checker interface {
	IsWhitelisted(ctx context.Context, evmAddress string) (bool, error)
}

// Manager is the admin surface (add/remove/list) consumed by the HTTP layer.
//
//go:generate mockery --name Manager --output mocks --outpkg mocks --filename mock_manager.go --with-expecter
type Manager interface {
	Add(ctx context.Context, evmAddress, note string) error
	Remove(ctx context.Context, evmAddress string) error
	List(ctx context.Context, cursor string, limit int) (*Page, error)
}

// Page is the cursor-paginated whitelist listing returned by List.
type Page struct {
	Items      []user.WhitelistEntry `json:"items"`
	NextCursor string                `json:"next_cursor,omitempty"`
	HasMore    bool                  `json:"has_more"`
}

// Service is the whitelist domain. When skip is true the gate authorizes every
// address (skip_whitelist_check); the admin operations are unaffected.
type Service struct {
	store Store
	skip  bool
}

// Compile-time checks that Service satisfies both consumer interfaces.
var (
	_ Checker = (*Service)(nil)
	_ Manager = (*Service)(nil)
)

// New returns a whitelist Service backed by store. When skip is true every
// address is authorized by IsWhitelisted.
func New(store Store, skip bool) *Service {
	return &Service{store: store, skip: skip}
}

// IsWhitelisted reports whether evmAddress may register/transact. With skip
// enabled it always returns true without touching the store.
func (s *Service) IsWhitelisted(ctx context.Context, evmAddress string) (bool, error) {
	if s.skip {
		return true, nil
	}
	return s.store.IsWhitelisted(ctx, evmAddress)
}

// Add validates the address and upserts it (with an optional note). It is
// idempotent: re-adding an address updates its note rather than erroring, so
// callers can ensure-present without a prior read.
func (s *Service) Add(ctx context.Context, evmAddress, note string) error {
	addr, err := normalizeAddress(evmAddress)
	if err != nil {
		return err
	}
	if storeErr := s.store.AddToWhitelist(ctx, addr, note); storeErr != nil {
		return apperrors.GeneralError(fmt.Errorf("add to whitelist: %w", storeErr))
	}
	return nil
}

// Remove validates the address and removes it, returning a not-found error when
// the address was not whitelisted so the operator gets clear feedback.
func (s *Service) Remove(ctx context.Context, evmAddress string) error {
	addr, err := normalizeAddress(evmAddress)
	if err != nil {
		return err
	}
	removed, storeErr := s.store.RemoveFromWhitelist(ctx, addr)
	if storeErr != nil {
		return apperrors.GeneralError(fmt.Errorf("remove from whitelist: %w", storeErr))
	}
	if !removed {
		return apperrors.ResourceNotFoundError(ErrEntryNotFound, "address not whitelisted")
	}
	return nil
}

// List returns one cursor-delimited page of whitelisted addresses, ordered by
// address. It fetches one extra row to determine HasMore without a separate
// count query; NextCursor is the last returned address (pass it back as cursor
// for the next page). limit is clamped to [1, MaxLimit] so a non-HTTP caller
// can't request a zero/negative page (which would yield an empty page that still
// reports HasMore and loop a paging client forever) or an unbounded one.
func (s *Service) List(ctx context.Context, cursor string, limit int) (*Page, error) {
	if limit < 1 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	rows, err := s.store.ListWhitelist(ctx, cursor, limit+1)
	if err != nil {
		return nil, apperrors.GeneralError(fmt.Errorf("list whitelist: %w", err))
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	items := make([]user.WhitelistEntry, len(rows))
	for i, e := range rows {
		items[i] = *e
	}

	page := &Page{Items: items, HasMore: hasMore}
	if hasMore && len(items) > 0 {
		page.NextCursor = items[len(items)-1].EVMAddress
	}
	return page, nil
}

// normalizeAddress validates an EVM address and returns its EIP-55 form.
// Validation matters because auth.NormalizeAddress (common.HexToAddress) silently
// coerces malformed input rather than rejecting it.
func normalizeAddress(evmAddress string) (string, error) {
	if !auth.ValidateEVMAddress(evmAddress) {
		return "", apperrors.BadRequestError(nil, "valid evm address is required")
	}
	return auth.NormalizeAddress(evmAddress), nil
}
