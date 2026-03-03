package token_test

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/chainsafe/canton-middleware/pkg/token"
	"github.com/chainsafe/canton-middleware/pkg/token/mocks"
	"github.com/chainsafe/canton-middleware/pkg/user"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── TestNative_GetBalance ────────────────────────────────────────────────────

func TestNative_GetBalance(t *testing.T) {
	ctx := context.Background()
	addr := common.HexToAddress("0xAAAA000000000000000000000000000000000001")

	t.Run("registered user returns configured native balance", func(t *testing.T) {
		userStore := mocks.NewUserStore(t)
		userStore.EXPECT().GetUserByEVMAddress(ctx, addr.Hex()).Return(&user.User{}, nil)

		svc := token.NewTokenService(newCfg(), nil, userStore, nil)
		native := token.NewNative(svc)

		bal, err := native.GetBalance(ctx, addr)
		require.NoError(t, err)
		expected, _ := new(big.Int).SetString("5000000000000000000", 10)
		assert.Equal(t, 0, expected.Cmp(&bal))
	})

	t.Run("unregistered user (ErrUserNotFound) returns zero and nil error", func(t *testing.T) {
		userStore := mocks.NewUserStore(t)
		userStore.EXPECT().GetUserByEVMAddress(ctx, addr.Hex()).Return(nil, user.ErrUserNotFound)

		svc := token.NewTokenService(newCfg(), nil, userStore, nil)
		native := token.NewNative(svc)

		bal, err := native.GetBalance(ctx, addr)
		require.NoError(t, err)
		assert.Equal(t, big.Int{}, bal)
	})

	t.Run("DB error propagates as error", func(t *testing.T) {
		userStore := mocks.NewUserStore(t)
		userStore.EXPECT().GetUserByEVMAddress(ctx, addr.Hex()).Return(nil, errors.New("db timeout"))

		svc := token.NewTokenService(newCfg(), nil, userStore, nil)
		native := token.NewNative(svc)

		_, err := native.GetBalance(ctx, addr)
		require.Error(t, err)
	})

	t.Run("invalid NativeBalanceWei panics", func(t *testing.T) {
		userStore := mocks.NewUserStore(t)
		userStore.EXPECT().GetUserByEVMAddress(ctx, addr.Hex()).Return(&user.User{}, nil)

		// Override NativeBalanceWei with an unparseable string.
		cfg := newCfg()
		cfg.NativeBalanceWei = "not-a-number"
		svc := token.NewTokenService(cfg, nil, userStore, nil)
		native := token.NewNative(svc)

		// nil dereference in *bal when SetString returns nil
		assert.Panics(t, func() { native.GetBalance(ctx, addr) }) //nolint:errcheck // panic assertion intentionally ignores returned error
	})
}

// ─── TestNative_Transfer ──────────────────────────────────────────────────────

func TestNative_Transfer(t *testing.T) {
	ctx := context.Background()
	from := common.HexToAddress("0xAAAA000000000000000000000000000000000001")
	to := common.HexToAddress("0xBBBB000000000000000000000000000000000002")

	t.Run("always returns not-supported error", func(t *testing.T) {
		svc := token.NewTokenService(newCfg(), nil, nil, nil)
		native := token.NewNative(svc)

		err := native.Transfer(ctx, from, to, *big.NewInt(1))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "native token transfer not supported")
	})
}
