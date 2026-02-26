package token

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type Native interface {
	GetBalance(ctx context.Context, address common.Address) (big.Int, error)
	Transfer(ctx context.Context, from, to common.Address, amount big.Int) error
}

type nativeImpl struct {
	svc *Service
}

func NewNative(svc *Service) Native {
	return &nativeImpl{svc: svc}
}

func (n *nativeImpl) GetBalance(ctx context.Context, address common.Address) (big.Int, error) {
	// TODO: This logic is confusing - either return not supported or implement it.
	isRegistered, err := n.svc.isUserRegistered(ctx, address)
	if err != nil || !isRegistered {
		return big.Int{}, err
	}
	bal, _ := new(big.Int).SetString(n.svc.cfg.NativeBalanceWei, 10)
	return *bal, nil
}

func (n *nativeImpl) Transfer(ctx context.Context, from, to common.Address, amount big.Int) error {
	return fmt.Errorf("native token transfer not supported")
}
