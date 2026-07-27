// SPDX-License-Identifier: Apache-2.0

package bridgeapi

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	goethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/chainsafe/canton-middleware/pkg/ethereum"
)

// AllowanceChecker reads an ERC-20 allowance from the EVM chain.
type AllowanceChecker interface {
	Allowance(ctx context.Context, token, owner, spender common.Address) (*big.Int, error)
}

// ethAllowanceChecker checks allowances via eth_call against an RPC node.
type ethAllowanceChecker struct {
	client *ethclient.Client
	erc20  abi.ABI
}

// NewAllowanceChecker dials the given EVM RPC URL for allowance reads.
func NewAllowanceChecker(rpcURL string) (AllowanceChecker, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("dial evm rpc: %w", err)
	}
	erc20, err := abi.JSON(strings.NewReader(ethereum.ERC20ABI))
	if err != nil {
		return nil, fmt.Errorf("parse erc20 abi: %w", err)
	}
	return &ethAllowanceChecker{client: client, erc20: erc20}, nil
}

func (c *ethAllowanceChecker) Allowance(
	ctx context.Context, token, owner, spender common.Address,
) (*big.Int, error) {
	data, err := c.erc20.Pack("allowance", owner, spender)
	if err != nil {
		return nil, fmt.Errorf("encode allowance: %w", err)
	}

	out, err := c.client.CallContract(ctx, goethereum.CallMsg{To: &token, Data: data}, nil)
	if err != nil {
		return nil, fmt.Errorf("call allowance: %w", err)
	}

	results, err := c.erc20.Unpack("allowance", out)
	if err != nil || len(results) != 1 {
		return nil, fmt.Errorf("decode allowance: %w", err)
	}
	allowance, ok := results[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("decode allowance: unexpected type %T", results[0])
	}
	return allowance, nil
}
