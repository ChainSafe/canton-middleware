// SPDX-License-Identifier: Apache-2.0

package token

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// Native defines the native-token surface exposed by this package.
type Native interface {
	GetBalance(ctx context.Context, address common.Address) (big.Int, error)
	Transfer(ctx context.Context, from, to common.Address, amount big.Int) error
}

type nativeImpl struct {
	svc *Service
}

// NewNative creates a Native implementation.
func NewNative(svc *Service) Native {
	return &nativeImpl{svc: svc}
}

// GetBalance always reports a zero native balance. The native coin is synthetic
// — there is no real gas token — so 0 is the honest value and avoids showing a
// confusing fake balance in MetaMask. Gas is also fixed at 0 (see the ethrpc
// service), so MetaMask's `balance >= value + gasLimit*gasPrice` pre-flight
// check still passes as `0 >= 0` for the zero-value ERC-20 transfers this facade
// supports.
func (*nativeImpl) GetBalance(_ context.Context, _ common.Address) (big.Int, error) {
	return big.Int{}, nil
}

func (*nativeImpl) Transfer(_ context.Context, _, _ common.Address, _ big.Int) error {
	return fmt.Errorf("native token transfer not supported")
}
