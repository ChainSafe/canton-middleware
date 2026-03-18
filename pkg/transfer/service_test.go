package transfer

import (
	"context"
	"testing"
	"time"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/pkg/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mocks ---

type mockUserStore struct {
	users map[string]*user.User
	err   error
}

func (m *mockUserStore) GetUserByEVMAddress(_ context.Context, evmAddress string) (*user.User, error) {
	if m.err != nil {
		return nil, m.err
	}
	u, ok := m.users[evmAddress]
	if !ok {
		return nil, user.ErrUserNotFound
	}
	return u, nil
}

type mockTransferCache struct {
	stored  map[string]*token.PreparedTransfer
	putErr  error
	getErr  error
	started bool
}

func newMockCache() *mockTransferCache {
	return &mockTransferCache{stored: make(map[string]*token.PreparedTransfer)}
}

func (m *mockTransferCache) Put(t *token.PreparedTransfer) error {
	if m.putErr != nil {
		return m.putErr
	}
	m.stored[t.TransferID] = t
	return nil
}

func (m *mockTransferCache) GetAndDelete(transferID string) (*token.PreparedTransfer, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	t, ok := m.stored[transferID]
	if !ok {
		return nil, token.ErrTransferNotFound
	}
	delete(m.stored, transferID)
	return t, nil
}

func (m *mockTransferCache) Start(_ context.Context) {
	m.started = true
}

// mockToken implements token.Token. Only PrepareTransfer and ExecuteTransfer
// carry real logic; every other method panics if called.
type mockToken struct {
	prepareResult *token.PreparedTransfer
	prepareErr    error
	executeErr    error
}

func (m *mockToken) GetTokenConfigCID(context.Context, string) (string, error) {
	panic("not implemented")
}
func (m *mockToken) Mint(context.Context, *token.MintRequest) (string, error) {
	panic("not implemented")
}
func (m *mockToken) Burn(context.Context, *token.BurnRequest) error {
	panic("not implemented")
}
func (m *mockToken) GetHoldings(context.Context, string, string) ([]*token.Holding, error) {
	panic("not implemented")
}
func (m *mockToken) GetAllHoldings(context.Context) ([]*token.Holding, error) {
	panic("not implemented")
}
func (m *mockToken) GetBalanceByFingerprint(context.Context, string, string) (string, error) {
	panic("not implemented")
}
func (m *mockToken) GetTotalSupply(context.Context, string) (string, error) {
	panic("not implemented")
}
func (m *mockToken) TransferByFingerprint(context.Context, string, string, string, string) error {
	panic("not implemented")
}
func (m *mockToken) TransferByPartyID(context.Context, string, string, string, string) error {
	panic("not implemented")
}
func (m *mockToken) GetTokenTransferEvents(context.Context) ([]*token.TokenTransferEvent, error) {
	panic("not implemented")
}
func (m *mockToken) GetTransferFactory(context.Context) (*token.TransferFactoryInfo, error) {
	panic("not implemented")
}

func (m *mockToken) PrepareTransfer(_ context.Context, _ *token.PrepareTransferRequest) (*token.PreparedTransfer, error) {
	if m.prepareErr != nil {
		return nil, m.prepareErr
	}
	return m.prepareResult, nil
}

func (m *mockToken) ExecuteTransfer(_ context.Context, _ *token.ExecuteTransferRequest) error {
	return m.executeErr
}

// --- helpers ---

func senderUser() *user.User {
	return &user.User{
		EVMAddress:                 "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		CantonPartyID:              "party::sender",
		KeyMode:                    user.KeyModeExternal,
		CantonPublicKeyFingerprint: "fingerprint-sender",
	}
}

func recipientUser() *user.User {
	return &user.User{
		EVMAddress:    "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		CantonPartyID: "party::recipient",
		KeyMode:       user.KeyModeExternal,
	}
}

func newTestService(tok *mockToken, store *mockUserStore, cache *mockTransferCache) *TransferService {
	svc := &TransferService{
		cantonToken:         tok,
		userStore:           store,
		cache:               cache,
		allowedTokenSymbols: map[string]bool{"DEMO": true, "PROMPT": true},
	}
	return svc
}

func assertServiceErrorCategory(t *testing.T, err error, cat apperrors.Category) {
	t.Helper()
	require.Error(t, err)
	require.True(t, apperrors.Is(err, cat), "expected category %v, got: %v", cat, err)
}

// --- Prepare tests ---

func TestTransferService_Prepare_Success(t *testing.T) {
	sender := senderUser()
	recipient := recipientUser()

	store := &mockUserStore{users: map[string]*user.User{
		sender.EVMAddress:    sender,
		recipient.EVMAddress: recipient,
	}}
	cache := newMockCache()
	tok := &mockToken{
		prepareResult: &token.PreparedTransfer{
			TransferID:      "txn-123",
			TransactionHash: []byte{0xde, 0xad},
			PartyID:         sender.CantonPartyID,
			ExpiresAt:       time.Now().Add(2 * time.Minute),
		},
	}

	svc := newTestService(tok, store, cache)
	resp, err := svc.Prepare(context.Background(), sender.EVMAddress, &PrepareRequest{
		To:     recipient.EVMAddress,
		Amount: "100.5",
		Token:  "DEMO",
	})

	require.NoError(t, err)
	assert.Equal(t, "txn-123", resp.TransferID)
	assert.Equal(t, "0xdead", resp.TransactionHash)
	assert.Equal(t, sender.CantonPartyID, resp.PartyID)
	assert.NotEmpty(t, resp.ExpiresAt)

	// Verify it was cached.
	_, ok := cache.stored["txn-123"]
	assert.True(t, ok, "prepared transfer should be in cache")
}

func TestTransferService_Prepare_MissingFields(t *testing.T) {
	svc := newTestService(&mockToken{}, &mockUserStore{users: map[string]*user.User{}}, newMockCache())

	tests := []struct {
		name string
		req  *PrepareRequest
	}{
		{"missing to", &PrepareRequest{Amount: "1", Token: "DEMO"}},
		{"missing amount", &PrepareRequest{To: "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Token: "DEMO"}},
		{"missing token", &PrepareRequest{To: "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Amount: "1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Prepare(context.Background(), "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", tt.req)
			assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
		})
	}
}

func TestTransferService_Prepare_SenderNotFound(t *testing.T) {
	store := &mockUserStore{users: map[string]*user.User{}} // empty store
	svc := newTestService(&mockToken{}, store, newMockCache())

	_, err := svc.Prepare(context.Background(), "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", &PrepareRequest{
		To:     "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Amount: "10",
		Token:  "DEMO",
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryUnauthorized)
}

func TestTransferService_Prepare_SenderNotExternal(t *testing.T) {
	sender := senderUser()
	sender.KeyMode = user.KeyModeCustodial

	store := &mockUserStore{users: map[string]*user.User{sender.EVMAddress: sender}}
	svc := newTestService(&mockToken{}, store, newMockCache())

	_, err := svc.Prepare(context.Background(), sender.EVMAddress, &PrepareRequest{
		To:     "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Amount: "10",
		Token:  "DEMO",
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
}

func TestTransferService_Prepare_UnsupportedToken(t *testing.T) {
	sender := senderUser()
	store := &mockUserStore{users: map[string]*user.User{sender.EVMAddress: sender}}
	svc := newTestService(&mockToken{}, store, newMockCache())

	_, err := svc.Prepare(context.Background(), sender.EVMAddress, &PrepareRequest{
		To:     "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Amount: "10",
		Token:  "BOGUS",
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
}

// --- Execute tests ---

func TestTransferService_Execute_Success(t *testing.T) {
	sender := senderUser()
	store := &mockUserStore{users: map[string]*user.User{sender.EVMAddress: sender}}
	cache := newMockCache()

	pt := &token.PreparedTransfer{
		TransferID:      "txn-456",
		TransactionHash: []byte{0xca, 0xfe},
		PartyID:         sender.CantonPartyID,
		ExpiresAt:       time.Now().Add(2 * time.Minute),
	}
	cache.stored[pt.TransferID] = pt

	tok := &mockToken{}
	svc := newTestService(tok, store, cache)

	resp, err := svc.Execute(context.Background(), sender.EVMAddress, &ExecuteRequest{
		TransferID: "txn-456",
		Signature:  "0xdeadbeef",
		SignedBy:   sender.CantonPublicKeyFingerprint,
	})

	require.NoError(t, err)
	assert.Equal(t, "completed", resp.Status)

	// Verify the transfer was removed from cache.
	_, ok := cache.stored["txn-456"]
	assert.False(t, ok, "transfer should be removed from cache after execute")
}

func TestTransferService_Execute_MissingFields(t *testing.T) {
	svc := newTestService(&mockToken{}, &mockUserStore{users: map[string]*user.User{}}, newMockCache())

	tests := []struct {
		name string
		req  *ExecuteRequest
	}{
		{"missing transfer_id", &ExecuteRequest{Signature: "0xab", SignedBy: "fp"}},
		{"missing signature", &ExecuteRequest{TransferID: "t1", SignedBy: "fp"}},
		{"missing signed_by", &ExecuteRequest{TransferID: "t1", Signature: "0xab"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Execute(context.Background(), "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", tt.req)
			assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
		})
	}
}

func TestTransferService_Execute_TransferNotFound(t *testing.T) {
	sender := senderUser()
	store := &mockUserStore{users: map[string]*user.User{sender.EVMAddress: sender}}
	cache := newMockCache() // empty cache

	svc := newTestService(&mockToken{}, store, cache)

	_, err := svc.Execute(context.Background(), sender.EVMAddress, &ExecuteRequest{
		TransferID: "nonexistent",
		Signature:  "0xab",
		SignedBy:   sender.CantonPublicKeyFingerprint,
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryResourceNotFound)
}

func TestTransferService_Execute_TransferExpired(t *testing.T) {
	sender := senderUser()
	store := &mockUserStore{users: map[string]*user.User{sender.EVMAddress: sender}}
	cache := newMockCache()
	cache.getErr = token.ErrTransferExpired

	svc := newTestService(&mockToken{}, store, cache)

	_, err := svc.Execute(context.Background(), sender.EVMAddress, &ExecuteRequest{
		TransferID: "expired-txn",
		Signature:  "0xab",
		SignedBy:   sender.CantonPublicKeyFingerprint,
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryGone)
}
