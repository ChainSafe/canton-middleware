// SPDX-License-Identifier: Apache-2.0

package bridgeapi

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"github.com/chainsafe/canton-middleware/pkg/relayer"
	"github.com/chainsafe/canton-middleware/pkg/user"
)

const (
	testEVMAddress = "0x00000000000000000000000000000000000000aa"
	testTokenSym   = "USDCX"
)

type fakeUserStore struct {
	users map[string]*user.User
}

func (f *fakeUserStore) GetUserByEVMAddress(_ context.Context, evmAddress string) (*user.User, error) {
	u, ok := f.users[evmAddress]
	if !ok {
		return nil, user.ErrUserNotFound
	}
	return u, nil
}

type fakeRelayer struct {
	registered *relayer.RegisterTransferRequest
	transfer   *relayer.Transfer
}

func (f *fakeRelayer) RegisterTransfer(
	_ context.Context, req *relayer.RegisterTransferRequest,
) (*relayer.RegisterTransferResponse, error) {
	f.registered = req
	return &relayer.RegisterTransferResponse{
		Transfer: &relayer.Transfer{ID: req.ID, Status: relayer.TransferStatusPending},
		Created:  true,
	}, nil
}

func (f *fakeRelayer) GetTransfer(context.Context, string) (*relayer.Transfer, error) {
	return f.transfer, nil
}

func testConfig() *Config {
	return &Config{
		RelayerURL: "http://relayer:8080",
		ChainID:    1,
		Tokens: map[string]TokenConfig{
			testTokenSym: xreserveToken(),
		},
	}
}

func newTestService(t *testing.T, rc RelayerClient) Service {
	t.Helper()
	users := &fakeUserStore{users: map[string]*user.User{
		testEVMAddress: {EVMAddress: testEVMAddress, CantonParty: testParty},
	}}
	svc, err := NewService(testConfig(), users, rc, nil, zap.NewNop())
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	return svc
}

func TestService_Tokens(t *testing.T) {
	svc := newTestService(t, &fakeRelayer{})

	tokens := svc.Tokens(context.Background())
	if len(tokens) != 1 || tokens[0].Symbol != testTokenSym || tokens[0].Mechanism != MechanismXReserve {
		t.Fatalf("tokens = %+v", tokens)
	}
}

func TestService_DepositQuote_HappyPath(t *testing.T) {
	svc := newTestService(t, &fakeRelayer{})

	quote, err := svc.DepositQuote(context.Background(), testEVMAddress, &QuoteRequest{Token: testTokenSym, Amount: "12.5"})
	if err != nil {
		t.Fatalf("DepositQuote failed: %v", err)
	}
	if quote.ChainID != 1 || len(quote.Steps) != 2 {
		t.Fatalf("quote = %+v, want chain 1 with approve+deposit steps", quote)
	}
	if quote.Fees.Currency != testTokenSym {
		t.Fatalf("quote fees = %+v", quote.Fees)
	}
}

func TestService_DepositQuote_Validation(t *testing.T) {
	svc := newTestService(t, &fakeRelayer{})
	ctx := context.Background()

	cases := []struct {
		name string
		addr string
		req  *QuoteRequest
	}{
		{"unsupported token", testEVMAddress, &QuoteRequest{Token: "DOGE", Amount: "1"}},
		{"negative amount", testEVMAddress, &QuoteRequest{Token: testTokenSym, Amount: "-1"}},
		{"non-numeric amount", testEVMAddress, &QuoteRequest{Token: testTokenSym, Amount: "abc"}},
		{"too many decimals", testEVMAddress, &QuoteRequest{Token: testTokenSym, Amount: "1.0000001"}},
		{"unregistered user", "0xdead", &QuoteRequest{Token: testTokenSym, Amount: "1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.DepositQuote(ctx, tc.addr, tc.req); err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

const testTxHash = "0x1111111111111111111111111111111111111111111111111111111111111111"

func TestService_RegisterDeposit_DerivesRecipientFromSession(t *testing.T) {
	rc := &fakeRelayer{}
	svc := newTestService(t, rc)

	resp, err := svc.RegisterDeposit(context.Background(), testEVMAddress, &RegisterDepositRequest{
		Token:  testTokenSym,
		Amount: "12.5",
		TxHash: testTxHash,
	})
	if err != nil {
		t.Fatalf("RegisterDeposit failed: %v", err)
	}
	if !resp.Created {
		t.Fatalf("resp = %+v, want created", resp)
	}

	reg := rc.registered
	if reg == nil {
		t.Fatalf("relayer never called")
	}
	// Recipient and mechanism come from the session/config, never the caller;
	// the id is namespaced so it cannot collide with observer ids.
	if reg.BridgeKey != MechanismXReserve || reg.TokenSymbol != testTokenSym ||
		reg.Amount != "12.5" || reg.Recipient != testParty ||
		reg.Sender != testEVMAddress || reg.SourceTxHash != testTxHash {
		t.Fatalf("registered = %+v", reg)
	}
	if reg.ID != MechanismXReserve+":"+testTxHash {
		t.Fatalf("id = %q, want namespaced by mechanism", reg.ID)
	}
	if reg.Direction != relayer.DirectionEthereumToCanton {
		t.Fatalf("direction = %s", reg.Direction)
	}
}

func TestService_RegisterDeposit_Validation(t *testing.T) {
	svc := newTestService(t, &fakeRelayer{})
	ctx := context.Background()

	cases := []struct {
		name string
		addr string
		req  *RegisterDepositRequest
	}{
		{"unsupported token", testEVMAddress, &RegisterDepositRequest{Token: "DOGE", Amount: "1", TxHash: testTxHash}},
		{"bad amount", testEVMAddress, &RegisterDepositRequest{Token: testTokenSym, Amount: "-1", TxHash: testTxHash}},
		{"precision overflow", testEVMAddress, &RegisterDepositRequest{Token: testTokenSym, Amount: "1.0000001", TxHash: testTxHash}},
		{"malformed tx hash", testEVMAddress, &RegisterDepositRequest{Token: testTokenSym, Amount: "1", TxHash: "0xdeadbeef"}},
		{"tx hash with suffix", testEVMAddress, &RegisterDepositRequest{Token: testTokenSym, Amount: "1", TxHash: testTxHash + "-0"}},
		{"unregistered user", "0xdead", &RegisterDepositRequest{Token: testTokenSym, Amount: "1", TxHash: testTxHash}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.RegisterDeposit(ctx, tc.addr, tc.req); err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}
