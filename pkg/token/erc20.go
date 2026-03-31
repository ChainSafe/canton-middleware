package token

import (
	"context"
	"fmt"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"
)

// ERC20 defines the ERC-20 surface exposed by this package.
type ERC20 interface {
	Name(ctx context.Context) string
	Symbol(ctx context.Context) string
	Decimals(ctx context.Context) uint8
	TotalSupply(ctx context.Context) big.Int
	BalanceOf(ctx context.Context, address common.Address) big.Int
	TransferFrom(ctx context.Context, idempotencyKey string, from, to common.Address, amount big.Int) error
	Approve(ctx context.Context, spender common.Address, amount big.Int) error
	Allowance(ctx context.Context, owner, spender common.Address) big.Int
}

type erc20Impl struct {
	address common.Address
	svc     *Service
}

// NewERC20 creates an ERC20 implementation bound to a contract address.
func NewERC20(address common.Address, tokenService *Service) ERC20 {
	return &erc20Impl{address: address, svc: tokenService}
}

func (e *erc20Impl) Name(ctx context.Context) string {
	name, err := e.svc.getTokenName(ctx, e.address)
	if err != nil {
		return "" // Default to empty string
	}
	return name
}

func (e *erc20Impl) Symbol(ctx context.Context) string {
	symbol, err := e.svc.getTokenSymbol(ctx, e.address)
	if err != nil {
		return "" // Default to empty string
	}
	return symbol
}

func (e *erc20Impl) Decimals(ctx context.Context) uint8 {
	decimals, err := e.svc.getTokenDecimals(ctx, e.address)
	if err != nil {
		return 0 // Default to zero.
	}
	if decimals < 0 || decimals > math.MaxUint8 {
		return 0
	}
	return uint8(decimals)
}

func (e *erc20Impl) TotalSupply(ctx context.Context) big.Int {
	ts, err := e.svc.getTotalSupply(ctx, e.address)
	if err != nil {
		return big.Int{} // Default to zero.
	}
	totalSupply, err := decimalToBigInt(ts, e.Decimals(ctx))
	if err != nil {
		return big.Int{}
	}
	return totalSupply
}

func (e *erc20Impl) BalanceOf(ctx context.Context, address common.Address) big.Int {
	bal, err := e.svc.getBalance(ctx, e.address, address)
	if err != nil {
		return big.Int{} // Default to zero.
	}
	balance, err := decimalToBigInt(bal, e.Decimals(ctx))
	if err != nil {
		return big.Int{}
	}
	return balance
}

func (e *erc20Impl) TransferFrom(ctx context.Context, idempotencyKey string, from, to common.Address, amount big.Int) error {
	return e.svc.transfer(ctx, idempotencyKey, e.address, from, to, bigIntToDecimal(amount, e.Decimals(ctx)))
}

func (erc20Impl) Approve(_ context.Context, _ common.Address, _ big.Int) error {
	return fmt.Errorf("not supported")
}

func (erc20Impl) Allowance(_ context.Context, _, _ common.Address) big.Int {
	return big.Int{}
}

func decimalToBigInt(s string, decimals uint8) (big.Int, error) {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return big.Int{}, fmt.Errorf("invalid decimal format: %w", err)
	}
	d = d.Mul(decimal.New(1, int32(decimals)))
	return *d.BigInt(), nil
}

func bigIntToDecimal(amount big.Int, decimals uint8) string {
	d := decimal.NewFromBigInt(&amount, -int32(decimals))
	return d.String()
}
