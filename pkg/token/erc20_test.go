package token_test

import (
	"context"
	"errors"
	"math/big"
	"testing"

	apperr "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/token"
	"github.com/chainsafe/canton-middleware/pkg/token/mocks"
	"github.com/chainsafe/canton-middleware/pkg/user"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ─── Test addresses ───────────────────────────────────────────────────────────

var (
	promptAddr      = common.HexToAddress("0x1000000000000000000000000000000000000001")
	demoAddr        = common.HexToAddress("0x2000000000000000000000000000000000000002")
	unsupportedAddr = common.HexToAddress("0x9999999999999999999999999999999999999999")
)

// ─── Shared helpers ───────────────────────────────────────────────────────────

func newCfg() *token.Config {
	cfg := token.NewConfig("5000000000000000000") // 5 ETH in wei
	cfg.AddToken(promptAddr, token.ERC20Token{Name: "Prompt Token", Symbol: "PROMPT", Decimals: 18})
	cfg.AddToken(demoAddr, token.ERC20Token{Name: "Demo Token", Symbol: "DEMO", Decimals: 18})
	return cfg
}

func promptUser() *user.User {
	return &user.User{Fingerprint: "fpA"}
}

func demoUser() *user.User {
	return &user.User{Fingerprint: "fpB"}
}

// ─── TestERC20_Name ───────────────────────────────────────────────────────────

func TestERC20_Name(t *testing.T) {
	ctx := context.Background()

	t.Run("supported contract returns configured name", func(t *testing.T) {
		svc := token.NewTokenService(newCfg(), nil, nil, nil)
		erc20 := token.NewERC20(promptAddr, svc)

		assert.Equal(t, "Prompt Token", erc20.Name(ctx))
	})

	t.Run("unsupported contract returns empty string", func(t *testing.T) {
		// Silent failure; correctness enforced by ethrpc guard
		svc := token.NewTokenService(newCfg(), nil, nil, nil)
		erc20 := token.NewERC20(unsupportedAddr, svc)

		assert.Empty(t, erc20.Name(ctx))
	})
}

// ─── TestERC20_Symbol ─────────────────────────────────────────────────────────

func TestERC20_Symbol(t *testing.T) {
	ctx := context.Background()

	t.Run("supported contract returns configured symbol", func(t *testing.T) {
		svc := token.NewTokenService(newCfg(), nil, nil, nil)
		erc20 := token.NewERC20(promptAddr, svc)

		assert.Equal(t, "PROMPT", erc20.Symbol(ctx))
	})

	t.Run("unsupported contract returns empty string", func(t *testing.T) {
		svc := token.NewTokenService(newCfg(), nil, nil, nil)
		erc20 := token.NewERC20(unsupportedAddr, svc)

		assert.Empty(t, erc20.Symbol(ctx))
	})
}

// ─── TestERC20_Decimals ───────────────────────────────────────────────────────

func TestERC20_Decimals(t *testing.T) {
	ctx := context.Background()

	t.Run("supported contract returns configured decimals", func(t *testing.T) {
		svc := token.NewTokenService(newCfg(), nil, nil, nil)
		erc20 := token.NewERC20(promptAddr, svc)

		assert.Equal(t, uint8(18), erc20.Decimals(ctx))
	})

	t.Run("decimals value 255 (MaxUint8) is preserved", func(t *testing.T) {
		cfg := newCfg()
		addr255 := common.HexToAddress("0x3000000000000000000000000000000000000003")
		cfg.AddToken(addr255, token.ERC20Token{Name: "T255", Symbol: "T255", Decimals: 255})
		svc := token.NewTokenService(cfg, nil, nil, nil)
		erc20 := token.NewERC20(addr255, svc)

		assert.Equal(t, uint8(255), erc20.Decimals(ctx))
	})

	t.Run("decimals value 256 (> MaxUint8) returns zero", func(t *testing.T) {
		cfg := newCfg()
		addr256 := common.HexToAddress("0x4000000000000000000000000000000000000004")
		cfg.AddToken(addr256, token.ERC20Token{Name: "T256", Symbol: "T256", Decimals: 256})
		svc := token.NewTokenService(cfg, nil, nil, nil)
		erc20 := token.NewERC20(addr256, svc)

		assert.Equal(t, uint8(0), erc20.Decimals(ctx))
	})

	t.Run("unsupported contract returns zero", func(t *testing.T) {
		svc := token.NewTokenService(newCfg(), nil, nil, nil)
		erc20 := token.NewERC20(unsupportedAddr, svc)

		assert.Equal(t, uint8(0), erc20.Decimals(ctx))
	})
}

// ─── TestERC20_TotalSupply ────────────────────────────────────────────────────

func TestERC20_TotalSupply(t *testing.T) {
	ctx := context.Background()

	t.Run("supported contract returns scaled supply", func(t *testing.T) {
		provider := mocks.NewProvider(t)
		provider.EXPECT().GetTotalSupply(mock.Anything, "PROMPT").Return("1000", nil)

		svc := token.NewTokenService(newCfg(), provider, nil, nil)
		erc20 := token.NewERC20(promptAddr, svc)

		supply := erc20.TotalSupply(ctx)
		// 1000 * 10^18
		expected := new(big.Int).Mul(big.NewInt(1000), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
		assert.Equal(t, 0, expected.Cmp(&supply))
	})

	t.Run("provider error returns zero big.Int", func(t *testing.T) {
		provider := mocks.NewProvider(t)
		provider.EXPECT().GetTotalSupply(mock.Anything, "PROMPT").Return("", errors.New("provider down"))

		svc := token.NewTokenService(newCfg(), provider, nil, nil)
		erc20 := token.NewERC20(promptAddr, svc)

		// Silent failure
		supply := erc20.TotalSupply(ctx)
		assert.Equal(t, big.Int{}, supply)
	})

	t.Run("invalid decimal from provider returns zero", func(t *testing.T) {
		provider := mocks.NewProvider(t)
		provider.EXPECT().GetTotalSupply(mock.Anything, "PROMPT").Return("not-a-number", nil)

		svc := token.NewTokenService(newCfg(), provider, nil, nil)
		erc20 := token.NewERC20(promptAddr, svc)

		supply := erc20.TotalSupply(ctx)
		assert.Equal(t, big.Int{}, supply)
	})

	t.Run("unsupported contract returns zero when no provider configured", func(t *testing.T) {
		svc := token.NewTokenService(newCfg(), nil, nil, nil)
		erc20 := token.NewERC20(unsupportedAddr, svc)

		supply := erc20.TotalSupply(ctx)
		assert.Equal(t, big.Int{}, supply)
	})
}

// ─── TestERC20_BalanceOf ──────────────────────────────────────────────────────

func TestERC20_BalanceOf(t *testing.T) {
	ctx := context.Background()
	accountAddr := common.HexToAddress("0xAAAA000000000000000000000000000000000001")

	t.Run("PROMPT token: provider balance is scaled", func(t *testing.T) {
		userStore := mocks.NewUserStore(t)
		userStore.EXPECT().GetUserByEVMAddress(mock.Anything, accountAddr.Hex()).Return(promptUser(), nil)

		provider := mocks.NewProvider(t)
		provider.EXPECT().GetBalance(mock.Anything, "PROMPT", promptUser().Fingerprint).Return("100", nil)

		svc := token.NewTokenService(newCfg(), provider, userStore, nil)
		erc20 := token.NewERC20(promptAddr, svc)

		bal := erc20.BalanceOf(ctx, accountAddr)
		// 100 * 10^18
		expected := new(big.Int).Mul(big.NewInt(100), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
		assert.Equal(t, 0, expected.Cmp(&bal))
	})

	t.Run("DEMO token: provider balance is scaled", func(t *testing.T) {
		userStore := mocks.NewUserStore(t)
		userStore.EXPECT().GetUserByEVMAddress(mock.Anything, accountAddr.Hex()).Return(demoUser(), nil)

		provider := mocks.NewProvider(t)
		provider.EXPECT().GetBalance(mock.Anything, "DEMO", demoUser().Fingerprint).Return("50", nil)

		svc := token.NewTokenService(newCfg(), provider, userStore, nil)
		erc20 := token.NewERC20(demoAddr, svc)

		bal := erc20.BalanceOf(ctx, accountAddr)
		// 50 * 10^18
		expected := new(big.Int).Mul(big.NewInt(50), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
		assert.Equal(t, 0, expected.Cmp(&bal))
	})

	t.Run("user not found returns zero balance", func(t *testing.T) {
		userStore := mocks.NewUserStore(t)
		userStore.EXPECT().GetUserByEVMAddress(mock.Anything, accountAddr.Hex()).Return(nil, user.ErrUserNotFound)

		provider := mocks.NewProvider(t)

		svc := token.NewTokenService(newCfg(), provider, userStore, nil)
		erc20 := token.NewERC20(promptAddr, svc)

		bal := erc20.BalanceOf(ctx, accountAddr)
		assert.Equal(t, big.Int{}, bal)
	})

	t.Run("provider error returns zero balance (silent failure)", func(t *testing.T) {
		userStore := mocks.NewUserStore(t)
		userStore.EXPECT().GetUserByEVMAddress(mock.Anything, accountAddr.Hex()).Return(promptUser(), nil)

		provider := mocks.NewProvider(t)
		provider.EXPECT().GetBalance(mock.Anything, "PROMPT", promptUser().Fingerprint).Return("0", errors.New("timeout"))

		svc := token.NewTokenService(newCfg(), provider, userStore, nil)
		erc20 := token.NewERC20(promptAddr, svc)

		bal := erc20.BalanceOf(ctx, accountAddr)
		assert.Equal(t, big.Int{}, bal)
	})

	t.Run("unsupported contract returns zero (provider error)", func(t *testing.T) {
		userStore := mocks.NewUserStore(t)
		provider := mocks.NewProvider(t)

		svc := token.NewTokenService(newCfg(), provider, userStore, nil)
		erc20 := token.NewERC20(unsupportedAddr, svc)

		bal := erc20.BalanceOf(ctx, accountAddr)
		assert.Equal(t, big.Int{}, bal)
	})
}

// ─── TestERC20_TransferFrom ───────────────────────────────────────────────────

func TestERC20_TransferFrom(t *testing.T) {
	ctx := context.Background()
	fromAddr := common.HexToAddress("0xAAAA000000000000000000000000000000000001")
	toAddr := common.HexToAddress("0xBBBB000000000000000000000000000000000002")
	// 1 PROMPT = 1 * 10^18 smallest units
	amount := *new(big.Int).Mul(big.NewInt(1), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))

	t.Run("success executes canton transfer", func(t *testing.T) {
		userStore := mocks.NewUserStore(t)
		userStore.EXPECT().GetUserByEVMAddress(mock.Anything, fromAddr.Hex()).Return(promptUser(), nil)
		userStore.EXPECT().GetUserByEVMAddress(mock.Anything, toAddr.Hex()).Return(demoUser(), nil)

		cantonToken := mocks.NewToken(t)
		cantonToken.EXPECT().TransferByFingerprint(mock.Anything, mock.Anything, promptUser().Fingerprint, demoUser().Fingerprint, "1", "PROMPT").Return(nil)

		svc := token.NewTokenService(newCfg(), nil, userStore, cantonToken)
		erc20 := token.NewERC20(promptAddr, svc)

		err := erc20.TransferFrom(ctx, "test-cmd", fromAddr, toAddr, amount)
		require.NoError(t, err)
	})

	t.Run("success: transfer does not require local balance sync", func(t *testing.T) {
		userStore := mocks.NewUserStore(t)
		userStore.EXPECT().GetUserByEVMAddress(mock.Anything, fromAddr.Hex()).Return(promptUser(), nil)
		userStore.EXPECT().GetUserByEVMAddress(mock.Anything, toAddr.Hex()).Return(demoUser(), nil)

		cantonToken := mocks.NewToken(t)
		cantonToken.EXPECT().TransferByFingerprint(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, "PROMPT").Return(nil)

		svc := token.NewTokenService(newCfg(), nil, userStore, cantonToken)
		erc20 := token.NewERC20(promptAddr, svc)

		err := erc20.TransferFrom(ctx, "test-cmd", fromAddr, toAddr, amount)
		require.NoError(t, err)
	})

	t.Run("unsupported contract returns error", func(t *testing.T) {
		svc := token.NewTokenService(newCfg(), nil, nil, nil)
		erc20 := token.NewERC20(unsupportedAddr, svc)

		err := erc20.TransferFrom(ctx, "test-cmd", fromAddr, toAddr, amount)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "token not supported")
		assert.True(t, apperr.Is(err, apperr.CategoryDataError))
	})

	t.Run("sender not found returns error", func(t *testing.T) {
		userStore := mocks.NewUserStore(t)
		userStore.EXPECT().GetUserByEVMAddress(mock.Anything, fromAddr.Hex()).Return(nil, user.ErrUserNotFound)

		svc := token.NewTokenService(newCfg(), nil, userStore, nil)
		erc20 := token.NewERC20(promptAddr, svc)

		err := erc20.TransferFrom(ctx, "test-cmd", fromAddr, toAddr, amount)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get sender")
		assert.True(t, apperr.Is(err, apperr.CategoryDataError))
	})

	t.Run("recipient not found returns error", func(t *testing.T) {
		userStore := mocks.NewUserStore(t)
		userStore.EXPECT().GetUserByEVMAddress(mock.Anything, fromAddr.Hex()).Return(promptUser(), nil)
		userStore.EXPECT().GetUserByEVMAddress(mock.Anything, toAddr.Hex()).Return(nil, user.ErrUserNotFound)

		svc := token.NewTokenService(newCfg(), nil, userStore, nil)
		erc20 := token.NewERC20(promptAddr, svc)

		err := erc20.TransferFrom(ctx, "test-cmd", fromAddr, toAddr, amount)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get recipient")
		assert.True(t, apperr.Is(err, apperr.CategoryDataError))
	})

	t.Run("canton transfer failure returns error", func(t *testing.T) {
		userStore := mocks.NewUserStore(t)
		userStore.EXPECT().GetUserByEVMAddress(mock.Anything, fromAddr.Hex()).Return(promptUser(), nil)
		userStore.EXPECT().GetUserByEVMAddress(mock.Anything, toAddr.Hex()).Return(demoUser(), nil)

		cantonToken := mocks.NewToken(t)
		cantonToken.EXPECT().TransferByFingerprint(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, "PROMPT").Return(errors.New("ledger down"))

		svc := token.NewTokenService(newCfg(), nil, userStore, cantonToken)
		erc20 := token.NewERC20(promptAddr, svc)

		err := erc20.TransferFrom(ctx, "test-cmd", fromAddr, toAddr, amount)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "canton transfer failed")
		assert.True(t, apperr.Is(err, apperr.CategoryDependencyFailure))
	})
}

// ─── TestERC20_Approve ────────────────────────────────────────────────────────

func TestERC20_Approve(t *testing.T) {
	ctx := context.Background()
	spender := common.HexToAddress("0xAAAA000000000000000000000000000000000001")
	amount := *big.NewInt(100)

	t.Run("always returns not-supported error", func(t *testing.T) {
		svc := token.NewTokenService(newCfg(), nil, nil, nil)
		erc20 := token.NewERC20(promptAddr, svc)

		err := erc20.Approve(ctx, spender, amount)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not supported")
	})
}

// ─── TestERC20_Allowance ──────────────────────────────────────────────────────

func TestERC20_Allowance(t *testing.T) {
	ctx := context.Background()
	owner := common.HexToAddress("0xAAAA000000000000000000000000000000000001")
	spender := common.HexToAddress("0xBBBB000000000000000000000000000000000002")

	t.Run("always returns zero", func(t *testing.T) {
		svc := token.NewTokenService(newCfg(), nil, nil, nil)
		erc20 := token.NewERC20(promptAddr, svc)

		allowance := erc20.Allowance(ctx, owner, spender)
		assert.Equal(t, big.Int{}, allowance)
	})
}
