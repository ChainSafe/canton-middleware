// SPDX-License-Identifier: Apache-2.0

package token_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/chainsafe/canton-middleware/pkg/token"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── TestNative_GetBalance ────────────────────────────────────────────────────

func TestNative_GetBalance(t *testing.T) {
	ctx := context.Background()
	addr := common.HexToAddress("0xAAAA000000000000000000000000000000000001")

	// The native coin is synthetic and always reports a zero balance, regardless
	// of registration — gas is also fixed at 0, so this still passes MetaMask's
	// pre-flight check for the zero-value ERC-20 transfers this facade supports.
	t.Run("always returns zero balance", func(t *testing.T) {
		svc := token.NewTokenService(newCfg(), nil, nil, nil)
		native := token.NewNative(svc)

		bal, err := native.GetBalance(ctx, addr)
		require.NoError(t, err)
		assert.Equal(t, big.Int{}, bal)
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
