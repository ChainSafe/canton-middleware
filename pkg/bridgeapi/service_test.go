// SPDX-License-Identifier: Apache-2.0

package bridgeapi

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chainsafe/canton-middleware/pkg/relayer"
	"github.com/chainsafe/canton-middleware/pkg/user"
)

const testEVMAddress = "0x00000000000000000000000000000000000000aa"

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
		QuoteTTL:   time.Minute,
		Tokens: map[string]TokenConfig{
			"USDCX": xreserveToken(),
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
	if len(tokens) != 1 || tokens[0].Symbol != "USDCX" || tokens[0].Mechanism != MechanismXReserve {
		t.Fatalf("tokens = %+v", tokens)
	}
}

func TestService_DepositQuote_HappyPath(t *testing.T) {
	svc := newTestService(t, &fakeRelayer{})

	quote, err := svc.DepositQuote(context.Background(), testEVMAddress, &QuoteRequest{Token: "USDCX", Amount: "12.5"})
	if err != nil {
		t.Fatalf("DepositQuote failed: %v", err)
	}
	if quote.ChainID != 1 || len(quote.Steps) != 2 {
		t.Fatalf("quote = %+v, want chain 1 with approve+deposit steps", quote)
	}
	if !strings.HasPrefix(quote.QuoteID, "q_") {
		t.Fatalf("quote id = %q", quote.QuoteID)
	}
	if quote.ExpiresAt.Before(time.Now()) {
		t.Fatalf("quote already expired: %v", quote.ExpiresAt)
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
		{"negative amount", testEVMAddress, &QuoteRequest{Token: "USDCX", Amount: "-1"}},
		{"non-numeric amount", testEVMAddress, &QuoteRequest{Token: "USDCX", Amount: "abc"}},
		{"too many decimals", testEVMAddress, &QuoteRequest{Token: "USDCX", Amount: "1.0000001"}},
		{"unregistered user", "0xdead", &QuoteRequest{Token: "USDCX", Amount: "1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.DepositQuote(ctx, tc.addr, tc.req); err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestService_RegisterDeposit_UsesQuoteParams(t *testing.T) {
	rc := &fakeRelayer{}
	svc := newTestService(t, rc)
	ctx := context.Background()

	quote, err := svc.DepositQuote(ctx, testEVMAddress, &QuoteRequest{Token: "USDCX", Amount: "12.5"})
	if err != nil {
		t.Fatalf("DepositQuote failed: %v", err)
	}

	resp, err := svc.RegisterDeposit(ctx, testEVMAddress, &RegisterDepositRequest{
		QuoteID: quote.QuoteID,
		TxHash:  "0xdeadbeef",
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
	// Every transfer parameter comes from the stored quote, not the caller.
	if reg.BridgeKey != MechanismXReserve || reg.TokenSymbol != "USDCX" ||
		reg.Amount != "12.5" || reg.Recipient != testParty ||
		reg.Sender != testEVMAddress || reg.SourceTxHash != "0xdeadbeef" {
		t.Fatalf("registered = %+v", reg)
	}
	if reg.Direction != relayer.DirectionEthereumToCanton {
		t.Fatalf("direction = %s", reg.Direction)
	}
	if reg.Metadata["quote_id"] != quote.QuoteID {
		t.Fatalf("metadata = %+v", reg.Metadata)
	}
}

func TestService_RegisterDeposit_Guards(t *testing.T) {
	svc := newTestService(t, &fakeRelayer{})
	ctx := context.Background()

	// Unknown quote.
	if _, err := svc.RegisterDeposit(ctx, testEVMAddress, &RegisterDepositRequest{QuoteID: "q_nope", TxHash: "0x1"}); err == nil {
		t.Fatalf("unknown quote should fail")
	}

	// Quote owned by someone else.
	quote, err := svc.DepositQuote(ctx, testEVMAddress, &QuoteRequest{Token: "USDCX", Amount: "1"})
	if err != nil {
		t.Fatalf("DepositQuote failed: %v", err)
	}
	if _, err = svc.RegisterDeposit(ctx, "0xother", &RegisterDepositRequest{QuoteID: quote.QuoteID, TxHash: "0x1"}); err == nil {
		t.Fatalf("foreign quote should fail")
	}
}

func TestQuoteStore_Expiry(t *testing.T) {
	store := newQuoteStore()
	now := time.Now()
	store.now = func() time.Time { return now }

	store.Put(&storedQuote{QuoteID: "q_1", ExpiresAt: now.Add(time.Minute)})

	if _, ok := store.Get("q_1"); !ok {
		t.Fatalf("fresh quote should be retrievable")
	}

	store.now = func() time.Time { return now.Add(2 * time.Minute) }
	if _, ok := store.Get("q_1"); ok {
		t.Fatalf("expired quote should not be retrievable")
	}
}
