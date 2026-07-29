// SPDX-License-Identifier: Apache-2.0

package bridgeapi

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"go.uber.org/zap"

	"github.com/chainsafe/canton-middleware/pkg/ethereum"
)

//nolint:gosec // public Sepolia contract addresses, not credentials
const (
	testTokenAddr    = "0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238"
	testContractAddr = "0x008888878f94C0d87defdf0B07f46B93C1934442"
	testParty        = "party::1220abcdef"
)

type fakeAllowance struct {
	allowance *big.Int
	err       error
}

func (f *fakeAllowance) Allowance(context.Context, common.Address, common.Address, common.Address) (*big.Int, error) {
	return f.allowance, f.err
}

func xreserveToken() TokenConfig {
	return TokenConfig{
		Mechanism:  MechanismXReserve,
		EVMAddress: testTokenAddr,
		Decimals:   6,
		XReserve: &XReserveTokenConfig{
			Contract:        testContractAddr,
			RemoteDomain:    10001,
			MaxFee:          "50000",
			InstrumentAdmin: "circle::admin",
			InstrumentID:    "USDCx",
		},
	}
}

func newQuoterForTest(t *testing.T, allowance AllowanceChecker) *xReserveQuoter {
	t.Helper()
	q, err := newXReserveQuoter(allowance, zap.NewNop())
	if err != nil {
		t.Fatalf("newXReserveQuoter failed: %v", err)
	}
	return q
}

func TestXReserveQuoter_EncodesApproveAndDeposit(t *testing.T) {
	q := newQuoterForTest(t, nil) // nil checker: approve always included
	sender := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	amount := big.NewInt(12_500_000) // 12.5 USDC in base units

	steps, err := q.DepositSteps(context.Background(), xreserveToken(), amount, sender, testParty)
	if err != nil {
		t.Fatalf("DepositSteps failed: %v", err)
	}
	if len(steps) != 2 || steps[0].Kind != StepKindApprove || steps[1].Kind != StepKindDeposit {
		t.Fatalf("steps = %+v, want [approve deposit]", steps)
	}

	// Approve: spender is the xReserve contract, value is the exact amount.
	if steps[0].To != common.HexToAddress(testTokenAddr).Hex() {
		t.Fatalf("approve.to = %s, want token address", steps[0].To)
	}
	erc20, err := abi.JSON(strings.NewReader(ethereum.ERC20ABI))
	if err != nil {
		t.Fatalf("parse erc20 abi: %v", err)
	}
	approveData := hexutil.MustDecode(steps[0].Data)
	approveArgs, err := erc20.Methods["approve"].Inputs.Unpack(approveData[4:])
	if err != nil {
		t.Fatalf("unpack approve: %v", err)
	}
	if spender := approveArgs[0].(common.Address); spender != common.HexToAddress(testContractAddr) {
		t.Fatalf("approve spender = %s, want xreserve contract", spender.Hex())
	}
	if value := approveArgs[1].(*big.Int); value.Cmp(amount) != 0 {
		t.Fatalf("approve value = %s, want %s", value, amount)
	}

	// Deposit: recipient hashing + hookData carry the Canton party.
	if steps[1].To != common.HexToAddress(testContractAddr).Hex() {
		t.Fatalf("deposit.to = %s, want xreserve contract", steps[1].To)
	}
	xr, err := abi.JSON(strings.NewReader(xReserveABI))
	if err != nil {
		t.Fatalf("parse xreserve abi: %v", err)
	}
	depositData := hexutil.MustDecode(steps[1].Data)
	depositArgs, err := xr.Methods["depositToRemote"].Inputs.Unpack(depositData[4:])
	if err != nil {
		t.Fatalf("unpack depositToRemote: %v", err)
	}
	if value := depositArgs[0].(*big.Int); value.Cmp(amount) != 0 {
		t.Fatalf("deposit value = %s, want %s", value, amount)
	}
	if domain := depositArgs[1].(uint32); domain != 10001 {
		t.Fatalf("remoteDomain = %d, want 10001", domain)
	}
	recipient := depositArgs[2].([32]byte)
	if common.Hash(recipient) != crypto.Keccak256Hash([]byte(testParty)) {
		t.Fatalf("remoteRecipient is not keccak256(partyId)")
	}
	if localToken := depositArgs[3].(common.Address); localToken != common.HexToAddress(testTokenAddr) {
		t.Fatalf("localToken = %s, want token address", localToken.Hex())
	}
	if maxFee := depositArgs[4].(*big.Int); maxFee.Cmp(big.NewInt(50000)) != 0 {
		t.Fatalf("maxFee = %s, want 50000", maxFee)
	}
	if hookData := depositArgs[5].([]byte); string(hookData) != testParty {
		t.Fatalf("hookData = %q, want the full party id", hookData)
	}
}

func TestXReserveQuoter_SkipsApproveWhenAllowanceSufficient(t *testing.T) {
	q := newQuoterForTest(t, &fakeAllowance{allowance: big.NewInt(100_000_000)})

	steps, err := q.DepositSteps(
		context.Background(), xreserveToken(), big.NewInt(12_500_000),
		common.HexToAddress("0xaa"), testParty)
	if err != nil {
		t.Fatalf("DepositSteps failed: %v", err)
	}
	if len(steps) != 1 || steps[0].Kind != StepKindDeposit {
		t.Fatalf("steps = %+v, want [deposit] only", steps)
	}
}

func TestXReserveQuoter_IncludesApproveWhenAllowanceLow(t *testing.T) {
	q := newQuoterForTest(t, &fakeAllowance{allowance: big.NewInt(1)})

	steps, err := q.DepositSteps(
		context.Background(), xreserveToken(), big.NewInt(12_500_000),
		common.HexToAddress("0xaa"), testParty)
	if err != nil {
		t.Fatalf("DepositSteps failed: %v", err)
	}
	if len(steps) != 2 || steps[0].Kind != StepKindApprove {
		t.Fatalf("steps = %+v, want approve first", steps)
	}
}

func TestXReserveQuoter_AllowanceFailure_FallsBackToApprove(t *testing.T) {
	q := newQuoterForTest(t, &fakeAllowance{err: errors.New("rpc down")})

	steps, err := q.DepositSteps(
		context.Background(), xreserveToken(), big.NewInt(1),
		common.HexToAddress("0xaa"), testParty)
	if err != nil {
		t.Fatalf("allowance failure must not fail the quote: %v", err)
	}
	if len(steps) != 2 || steps[0].Kind != StepKindApprove {
		t.Fatalf("steps = %+v, want a redundant-but-safe approve", steps)
	}
}
