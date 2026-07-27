// SPDX-License-Identifier: Apache-2.0

package bridgeapi

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"go.uber.org/zap"

	"github.com/chainsafe/canton-middleware/pkg/ethereum"
)

// MechanismXReserve quotes deposits through Circle's xReserve contract. The
// value doubles as the relayer TokenBridge key.
const MechanismXReserve = "xreserve"

// xReserveABI covers the single call the quoter encodes. The signature
// follows Circle's published xReserve interface (deposit quickstart /
// evm-xreserve-contracts); confirming it against the mainnet deployment is a
// #360 work item.
const xReserveABI = `[{
	"type": "function",
	"name": "depositToRemote",
	"stateMutability": "nonpayable",
	"inputs": [
		{"name": "value", "type": "uint256"},
		{"name": "remoteDomain", "type": "uint32"},
		{"name": "remoteRecipient", "type": "bytes32"},
		{"name": "localToken", "type": "address"},
		{"name": "maxFee", "type": "uint256"},
		{"name": "hookData", "type": "bytes"}
	],
	"outputs": []
}]`

// DepositQuoter builds the unsigned transaction steps for one mechanism.
type DepositQuoter interface {
	// DepositSteps returns the ordered transactions that move baseAmount
	// (token base units) from sender's EVM address to recipientParty on
	// Canton.
	DepositSteps(
		ctx context.Context,
		token TokenConfig,
		baseAmount *big.Int,
		sender common.Address,
		recipientParty string,
	) ([]TxStep, error)
}

// xReserveQuoter encodes approve + depositToRemote for xreserve tokens.
type xReserveQuoter struct {
	erc20     abi.ABI
	xreserve  abi.ABI
	allowance AllowanceChecker // nil: always include the approve step
	logger    *zap.Logger
}

func newXReserveQuoter(allowance AllowanceChecker, logger *zap.Logger) (*xReserveQuoter, error) {
	erc20, err := abi.JSON(strings.NewReader(ethereum.ERC20ABI))
	if err != nil {
		return nil, fmt.Errorf("parse erc20 abi: %w", err)
	}
	xr, err := abi.JSON(strings.NewReader(xReserveABI))
	if err != nil {
		return nil, fmt.Errorf("parse xreserve abi: %w", err)
	}
	return &xReserveQuoter{erc20: erc20, xreserve: xr, allowance: allowance, logger: logger}, nil
}

func (q *xReserveQuoter) DepositSteps(
	ctx context.Context,
	token TokenConfig,
	baseAmount *big.Int,
	sender common.Address,
	recipientParty string,
) ([]TxStep, error) {
	if token.XReserve == nil {
		return nil, fmt.Errorf("token %s: missing xreserve config", token.EVMAddress)
	}

	tokenAddr := common.HexToAddress(token.EVMAddress)
	contract := common.HexToAddress(token.XReserve.Contract)

	const base10 = 10
	maxFee := big.NewInt(0)
	if token.XReserve.MaxFee != "" {
		parsed, ok := new(big.Int).SetString(token.XReserve.MaxFee, base10)
		if !ok {
			return nil, fmt.Errorf("invalid xreserve max_fee %q", token.XReserve.MaxFee)
		}
		maxFee = parsed
	}

	var steps []TxStep

	if q.needsApprove(ctx, tokenAddr, sender, contract, baseAmount) {
		approveData, packErr := q.erc20.Pack("approve", contract, baseAmount)
		if packErr != nil {
			return nil, fmt.Errorf("encode approve: %w", packErr)
		}
		steps = append(steps, TxStep{
			Kind:  StepKindApprove,
			To:    tokenAddr.Hex(),
			Data:  hexutil.Encode(approveData),
			Value: "0",
		})
	}

	// Canton recipient encoding per Circle's deposit quickstart:
	// remoteRecipient = keccak256(partyId), full party id in hookData.
	remoteRecipient := crypto.Keccak256Hash([]byte(recipientParty))
	depositData, err := q.xreserve.Pack(
		"depositToRemote",
		baseAmount,
		token.XReserve.RemoteDomain,
		remoteRecipient,
		tokenAddr,
		maxFee,
		[]byte(recipientParty),
	)
	if err != nil {
		return nil, fmt.Errorf("encode depositToRemote: %w", err)
	}
	steps = append(steps, TxStep{
		Kind:  StepKindDeposit,
		To:    contract.Hex(),
		Data:  hexutil.Encode(depositData),
		Value: "0",
	})

	return steps, nil
}

// needsApprove checks the current allowance when a checker is configured, and
// falls back to including the approve step on any failure (a redundant approve
// is safe, a missing one strands the deposit).
//
// Emits a single approve(amount) — fine for USDC (non-zero -> non-zero is
// allowed). A USDT-style token would need an approve(0) reset added first.
func (q *xReserveQuoter) needsApprove(
	ctx context.Context, token, owner, spender common.Address, amount *big.Int,
) bool {
	if q.allowance == nil {
		return true
	}
	current, err := q.allowance.Allowance(ctx, token, owner, spender)
	if err != nil {
		q.logger.Warn("Allowance check failed, including approve step",
			zap.String("token", token.Hex()), zap.Error(err))
		return true
	}
	return current.Cmp(amount) < 0
}
