// SPDX-License-Identifier: Apache-2.0

package whitelist_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/chainsafe/canton-middleware/pkg/user"
	"github.com/chainsafe/canton-middleware/pkg/user/whitelist"
	"github.com/chainsafe/canton-middleware/pkg/user/whitelist/mocks"
)

const lowerAddr = "0xabcdef0123456789abcdef0123456789abcdef01"

func TestService_Add_NormalizesAddress(t *testing.T) {
	ctx := context.Background()
	checksummed := auth.NormalizeAddress(lowerAddr)

	store := mocks.NewStore(t)
	store.EXPECT().AddToWhitelist(ctx, checksummed, "note").Return(nil).Once()

	svc := whitelist.New(store, false)
	require.NoError(t, svc.Add(ctx, lowerAddr, "note"))
}

func TestService_Add_InvalidAddress(t *testing.T) {
	ctx := context.Background()
	store := mocks.NewStore(t) // store must not be called for invalid input

	svc := whitelist.New(store, false)
	err := svc.Add(ctx, "not-an-address", "")
	require.Error(t, err)
	require.True(t, apperrors.Is(err, apperrors.CategoryDataError), "want bad request, got %v", err)
}

func TestService_Remove_NormalizesAddress(t *testing.T) {
	ctx := context.Background()
	checksummed := auth.NormalizeAddress(lowerAddr)

	store := mocks.NewStore(t)
	store.EXPECT().RemoveFromWhitelist(ctx, checksummed).Return(true, nil).Once()

	svc := whitelist.New(store, false)
	require.NoError(t, svc.Remove(ctx, lowerAddr))
}

func TestService_Remove_NotFound(t *testing.T) {
	ctx := context.Background()
	checksummed := auth.NormalizeAddress(lowerAddr)

	store := mocks.NewStore(t)
	store.EXPECT().RemoveFromWhitelist(ctx, checksummed).Return(false, nil).Once()

	svc := whitelist.New(store, false)
	err := svc.Remove(ctx, lowerAddr)
	require.ErrorIs(t, err, whitelist.ErrEntryNotFound)
	require.True(t, apperrors.Is(err, apperrors.CategoryResourceNotFound), "want 404, got %v", err)
}

// TestService_List_Pagination verifies the cursor envelope: the service fetches
// limit+1 from the store, trims to limit, sets HasMore, and derives NextCursor
// from the last returned address.
func TestService_List_Pagination(t *testing.T) {
	ctx := context.Background()
	store := mocks.NewStore(t)

	// limit=2 → service asks the store for 3; store returns 3 → another page exists.
	store.EXPECT().ListWhitelist(ctx, "", 3).Return([]*user.WhitelistEntry{
		{EVMAddress: "0xA"}, {EVMAddress: "0xB"}, {EVMAddress: "0xC"},
	}, nil).Once()

	svc := whitelist.New(store, false)
	page, err := svc.List(ctx, "", 2)
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	require.True(t, page.HasMore)
	require.Equal(t, "0xB", page.NextCursor)
	require.Equal(t, "0xA", page.Items[0].EVMAddress)
}

func TestService_List_LastPage(t *testing.T) {
	ctx := context.Background()
	store := mocks.NewStore(t)

	// Cursor forwarded; store returns fewer than limit+1 → no more pages.
	store.EXPECT().ListWhitelist(ctx, "0xB", 3).Return([]*user.WhitelistEntry{
		{EVMAddress: "0xC"},
	}, nil).Once()

	svc := whitelist.New(store, false)
	page, err := svc.List(ctx, "0xB", 2)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.False(t, page.HasMore)
	require.Empty(t, page.NextCursor)
}

// TestService_List_ClampsLimit verifies the service guards the page size: a
// non-positive limit falls back to DefaultLimit and an oversized one is capped
// at MaxLimit (so the store is always asked for a sane page+1).
func TestService_List_ClampsLimit(t *testing.T) {
	ctx := context.Background()

	t.Run("zero falls back to default", func(t *testing.T) {
		store := mocks.NewStore(t)
		store.EXPECT().ListWhitelist(ctx, "", whitelist.DefaultLimit+1).Return(nil, nil).Once()

		svc := whitelist.New(store, false)
		_, err := svc.List(ctx, "", 0)
		require.NoError(t, err)
	})

	t.Run("oversized capped at max", func(t *testing.T) {
		store := mocks.NewStore(t)
		store.EXPECT().ListWhitelist(ctx, "", whitelist.MaxLimit+1).Return(nil, nil).Once()

		svc := whitelist.New(store, false)
		_, err := svc.List(ctx, "", whitelist.MaxLimit*10)
		require.NoError(t, err)
	})
}

func TestService_List_StoreError(t *testing.T) {
	ctx := context.Background()
	store := mocks.NewStore(t)
	store.EXPECT().ListWhitelist(ctx, "", 51).Return(nil, errors.New("db down")).Once()

	svc := whitelist.New(store, false)
	_, err := svc.List(ctx, "", whitelist.DefaultLimit)
	require.Error(t, err)
}
