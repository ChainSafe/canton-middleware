// SPDX-License-Identifier: Apache-2.0

package bridgeapi

import (
	"context"
	"testing"

	"go.uber.org/zap"

	cantontkn "github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
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
	// seen models the relayer's idempotent CreateTransfer: a repeated id
	// returns created=false, as the real ON CONFLICT DO NOTHING would.
	seen         map[string]bool
	registerErr  error
	registerHits int
}

func (f *fakeRelayer) RegisterTransfer(
	_ context.Context, req *relayer.RegisterTransferRequest,
) (*relayer.RegisterTransferResponse, error) {
	f.registerHits++
	if f.registerErr != nil {
		return nil, f.registerErr
	}
	f.registered = req
	if f.seen == nil {
		f.seen = map[string]bool{}
	}
	created := !f.seen[req.ID]
	f.seen[req.ID] = true
	return &relayer.RegisterTransferResponse{
		Transfer: &relayer.Transfer{ID: req.ID, Status: relayer.TransferStatusPending},
		Created:  created,
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

// fakeBurner records burn calls and returns canned results.
type fakeBurner struct {
	prepared    *cantontkn.PreparedTransfer
	prepareErr  error
	burnReq     *cantontkn.PrepareBurnRequest
	executed    *cantontkn.ExecuteTransferRequest
	burnErr     error
	executeErr  error
	customCalls int
}

func (f *fakeBurner) PrepareBurn(_ context.Context, req *cantontkn.PrepareBurnRequest) (*cantontkn.PreparedTransfer, error) {
	f.burnReq = req
	return f.prepared, f.prepareErr
}

func (f *fakeBurner) BurnByPartyID(_ context.Context, req *cantontkn.PrepareBurnRequest) error {
	f.customCalls++
	f.burnReq = req
	return f.burnErr
}

func (f *fakeBurner) ExecuteTransfer(_ context.Context, req *cantontkn.ExecuteTransferRequest) error {
	f.executed = req
	return f.executeErr
}

func newTestService(t *testing.T, rc RelayerClient) Service {
	t.Helper()
	return newTestServiceWithUsers(t, rc, &fakeBurner{}, map[string]*user.User{
		testEVMAddress: {EVMAddress: testEVMAddress, CantonParty: testParty},
	})
}

func newTestServiceWithUsers(t *testing.T, rc RelayerClient, burner CantonBurner, users map[string]*user.User) Service {
	t.Helper()
	svc, err := NewService(testConfig(), &fakeUserStore{users: users}, rc, nil, burner, zap.NewNop())
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

	quote, err := svc.DepositQuote(context.Background(), testEVMAddress, &QuoteRequest{Token: testTokenSym, Amount: testAmount})
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
		Amount: testAmount,
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
		reg.Amount != testAmount || reg.Recipient != testParty ||
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

const testAmount = "12.5"
