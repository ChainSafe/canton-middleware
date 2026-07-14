// SPDX-License-Identifier: Apache-2.0

package transfer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/pkg/indexer"
	"github.com/chainsafe/canton-middleware/pkg/transfer/mocks"
	"github.com/chainsafe/canton-middleware/pkg/user"
)

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

// newTestService builds a TransferService wired to a fresh IndexerReader
// mock. The api-server now requires the indexer to be configured, so the
// production constructor refuses to operate without an offer lister; tests
// pass `t` so an unused mock fails cleanly if a test accidentally invokes it.
func newTestService(t *testing.T, tok *mocks.Token, store *mocks.UserStore, cache *mocks.TransferCache) *TransferService {
	return newTestServiceWithOffers(tok, store, cache, mocks.NewIndexerReader(t))
}

func newTestServiceWithOffers(
	tok *mocks.Token,
	store *mocks.UserStore,
	cache *mocks.TransferCache,
	offers *mocks.IndexerReader,
) *TransferService {
	return &TransferService{
		cantonToken:         tok,
		userStore:           store,
		cache:               cache,
		offerLister:         offers,
		allowedTokenSymbols: map[string]bool{"DEMO": true, "PROMPT": true, "USDCx": true},
		// USDCx is the externally transferable token, mirroring the deploy
		// configs; DEMO/PROMPT are internal-only. Tests that exercise the
		// external path assign svc.partyRegistry themselves.
		externalTokenSymbols: map[string]bool{"USDCx": true},
		tokensByInstrument: map[instrumentKey]instrumentMeta{
			{id: "DEMO"}: {
				contractAddress: "0x1111111111111111111111111111111111111111",
				name:            "Demo Token",
				symbol:          "DEMO",
				decimals:        18,
			},
		},
	}
}

func assertServiceErrorCategory(t *testing.T, err error, cat apperrors.Category) {
	t.Helper()
	require.Error(t, err)
	require.True(t, apperrors.Is(err, cat), "expected category %v, got: %v", cat, err)
}

// --- Prepare tests ---

func TestTransferService_Prepare_Success(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()
	recipient := recipientUser()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()
	store.EXPECT().GetUserByEVMAddress(ctx, recipient.EVMAddress).Return(recipient, nil).Once()

	prepared := &token.PreparedTransfer{
		TransferID:      "txn-123",
		TransactionHash: []byte{0xde, 0xad},
		PartyID:         sender.CantonPartyID,
		ExpiresAt:       time.Now().Add(2 * time.Minute),
	}

	tok := mocks.NewToken(t)
	tok.EXPECT().PrepareTransfer(ctx, &token.PrepareTransferRequest{
		FromPartyID: sender.CantonPartyID,
		ToPartyID:   recipient.CantonPartyID,
		Amount:      "100.5",
		TokenSymbol: "DEMO",
		Validity:    time.Hour,
	}).Return(prepared, nil).Once()

	cache := mocks.NewTransferCache(t)
	cache.EXPECT().Put(prepared).Return(nil).Once()

	svc := newTestService(t, tok, store, cache)
	resp, err := svc.Prepare(ctx, sender.EVMAddress, &PrepareRequest{
		To:              recipient.EVMAddress,
		Amount:          "100.5",
		Token:           "DEMO",
		ValiditySeconds: 3600,
	})

	require.NoError(t, err)
	assert.Equal(t, "txn-123", resp.TransferID)
	assert.Equal(t, "0xdead", resp.TransactionHash)
	assert.Equal(t, sender.CantonPartyID, resp.PartyID)
	assert.NotEmpty(t, resp.ExpiresAt)
}

func TestTransferService_Prepare_SenderNotFound(t *testing.T) {
	ctx := context.Background()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa").
		Return(nil, user.ErrUserNotFound).Once()

	svc := newTestService(t, mocks.NewToken(t), store, mocks.NewTransferCache(t))

	_, err := svc.Prepare(ctx, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", &PrepareRequest{
		To:              "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Amount:          "10",
		Token:           "DEMO",
		ValiditySeconds: 3600,
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryUnauthorized)
}

func TestTransferService_Prepare_SenderNotExternal(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()
	sender.KeyMode = user.KeyModeCustodial

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	svc := newTestService(t, mocks.NewToken(t), store, mocks.NewTransferCache(t))

	_, err := svc.Prepare(ctx, sender.EVMAddress, &PrepareRequest{
		To:              "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Amount:          "10",
		Token:           "DEMO",
		ValiditySeconds: 3600,
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
}

func TestTransferService_Prepare_UnsupportedToken(t *testing.T) {
	svc := newTestService(t, mocks.NewToken(t), mocks.NewUserStore(t), mocks.NewTransferCache(t))

	_, err := svc.Prepare(context.Background(), senderUser().EVMAddress, &PrepareRequest{
		To:     "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Amount: "10",
		Token:  "BOGUS",
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
}

func TestTransferService_Prepare_RequiresValidity(t *testing.T) {
	// validity_seconds is mandatory; a zero/negative value is rejected before any
	// store lookup, so no user mock is needed.
	svc := newTestService(t, mocks.NewToken(t), mocks.NewUserStore(t), mocks.NewTransferCache(t))
	_, err := svc.Prepare(context.Background(), senderUser().EVMAddress, &PrepareRequest{
		To:     "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Amount: "10",
		Token:  "DEMO",
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
}

func TestTransferService_Prepare_ValidityTooLarge(t *testing.T) {
	// A validity_seconds large enough to overflow the nanosecond time.Duration is
	// rejected before any store lookup rather than silently wrapping to a small
	// (early-expiring) duration.
	svc := newTestService(t, mocks.NewToken(t), mocks.NewUserStore(t), mocks.NewTransferCache(t))
	_, err := svc.Prepare(context.Background(), senderUser().EVMAddress, &PrepareRequest{
		To:              "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Amount:          "10",
		Token:           "DEMO",
		ValiditySeconds: maxValiditySeconds + 1,
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
}

// validExternalPartyID is a syntactically valid Canton party id (hint::hex)
// for a party not registered in the middleware.
const validExternalPartyID = "alice::1220abcdef0123456789"

func TestTransferService_Prepare_ToPartyID_Success(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()

	// The recipient party id resolves to a registered user, so an internal-only
	// token like DEMO may be sent to it.
	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()
	store.EXPECT().GetUserByCantonPartyID(ctx, validExternalPartyID).Return(recipientUser(), nil).Once()

	prepared := &token.PreparedTransfer{
		TransferID:      "txn-ext-1",
		TransactionHash: []byte{0xbe, 0xef},
		PartyID:         sender.CantonPartyID,
		ExpiresAt:       time.Now().Add(2 * time.Minute),
	}

	tok := mocks.NewToken(t)
	tok.EXPECT().PrepareTransfer(ctx, &token.PrepareTransferRequest{
		FromPartyID: sender.CantonPartyID,
		ToPartyID:   validExternalPartyID,
		Amount:      "10",
		TokenSymbol: "DEMO",
		Validity:    time.Hour,
	}).Return(prepared, nil).Once()

	cache := mocks.NewTransferCache(t)
	cache.EXPECT().Put(prepared).Return(nil).Once()

	svc := newTestService(t, tok, store, cache)
	resp, err := svc.Prepare(ctx, sender.EVMAddress, &PrepareRequest{
		ToPartyID:       validExternalPartyID,
		Amount:          "10",
		Token:           "DEMO",
		ValiditySeconds: 3600,
	})

	require.NoError(t, err)
	assert.Equal(t, "txn-ext-1", resp.TransferID)
	assert.Equal(t, sender.CantonPartyID, resp.PartyID)
}

func TestTransferService_Prepare_ToPartyID_Invalid(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	svc := newTestService(t, mocks.NewToken(t), store, mocks.NewTransferCache(t))
	_, err := svc.Prepare(ctx, sender.EVMAddress, &PrepareRequest{
		ToPartyID:       "not-a-party-id",
		Amount:          "10",
		Token:           "DEMO",
		ValiditySeconds: 3600,
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
}

func TestTransferService_Prepare_ToPartyID_Self(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()
	sender.CantonPartyID = "sender::1220deadbeef" // valid hex so it passes party-id validation

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	svc := newTestService(t, mocks.NewToken(t), store, mocks.NewTransferCache(t))
	_, err := svc.Prepare(ctx, sender.EVMAddress, &PrepareRequest{
		ToPartyID:       sender.CantonPartyID,
		Amount:          "10",
		Token:           "DEMO",
		ValiditySeconds: 3600,
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
}

func TestTransferService_Prepare_InternalToken_UnregisteredParty_Rejected(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()

	// DEMO is internal-only: a party id that is not a registered user is rejected
	// before any Canton call.
	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()
	store.EXPECT().GetUserByCantonPartyID(ctx, validExternalPartyID).Return(nil, user.ErrUserNotFound).Once()

	svc := newTestService(t, mocks.NewToken(t), store, mocks.NewTransferCache(t))
	_, err := svc.Prepare(ctx, sender.EVMAddress, &PrepareRequest{
		ToPartyID:       validExternalPartyID,
		Amount:          "10",
		Token:           "DEMO",
		ValiditySeconds: 3600,
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
	assert.Contains(t, err.Error(), "does not support transfers to external parties")
}

func TestTransferService_Prepare_ExternalToken_ExternalParty_Success(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()

	// USDCx allows unregistered recipients as long as the party is known to the
	// participant's topology.
	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()
	store.EXPECT().GetUserByCantonPartyID(ctx, validExternalPartyID).Return(nil, user.ErrUserNotFound).Once()

	registry := mocks.NewPartyRegistry(t)
	registry.EXPECT().PartyExists(ctx, validExternalPartyID).Return(true, nil).Once()

	prepared := &token.PreparedTransfer{
		TransferID:      "txn-usdcx-1",
		TransactionHash: []byte{0xbe, 0xef},
		PartyID:         sender.CantonPartyID,
		ExpiresAt:       time.Now().Add(2 * time.Minute),
	}
	tok := mocks.NewToken(t)
	tok.EXPECT().PrepareTransfer(ctx, &token.PrepareTransferRequest{
		FromPartyID: sender.CantonPartyID,
		ToPartyID:   validExternalPartyID,
		Amount:      "10",
		TokenSymbol: "USDCx",
		Validity:    time.Hour,
	}).Return(prepared, nil).Once()

	cache := mocks.NewTransferCache(t)
	cache.EXPECT().Put(prepared).Return(nil).Once()

	svc := newTestService(t, tok, store, cache)
	svc.partyRegistry = registry
	resp, err := svc.Prepare(ctx, sender.EVMAddress, &PrepareRequest{
		ToPartyID:       validExternalPartyID,
		Amount:          "10",
		Token:           "USDCx",
		ValiditySeconds: 3600,
	})

	require.NoError(t, err)
	assert.Equal(t, "txn-usdcx-1", resp.TransferID)
}

func TestTransferService_Prepare_ExternalToken_UnknownParty_Rejected(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()
	store.EXPECT().GetUserByCantonPartyID(ctx, validExternalPartyID).Return(nil, user.ErrUserNotFound).Once()

	registry := mocks.NewPartyRegistry(t)
	registry.EXPECT().PartyExists(ctx, validExternalPartyID).Return(false, nil).Once()

	svc := newTestService(t, mocks.NewToken(t), store, mocks.NewTransferCache(t))
	svc.partyRegistry = registry
	_, err := svc.Prepare(ctx, sender.EVMAddress, &PrepareRequest{
		ToPartyID:       validExternalPartyID,
		Amount:          "10",
		Token:           "USDCx",
		ValiditySeconds: 3600,
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
	assert.Contains(t, err.Error(), "not known on the network")
}

func TestTransferService_Prepare_ExternalToken_PartyLookupFailure(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()
	store.EXPECT().GetUserByCantonPartyID(ctx, validExternalPartyID).Return(nil, user.ErrUserNotFound).Once()

	registry := mocks.NewPartyRegistry(t)
	registry.EXPECT().PartyExists(ctx, validExternalPartyID).
		Return(false, grpcstatus.Error(codes.Unavailable, "participant down")).Once()

	svc := newTestService(t, mocks.NewToken(t), store, mocks.NewTransferCache(t))
	svc.partyRegistry = registry
	_, err := svc.Prepare(ctx, sender.EVMAddress, &PrepareRequest{
		ToPartyID:       validExternalPartyID,
		Amount:          "10",
		Token:           "USDCx",
		ValiditySeconds: 3600,
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryDependencyFailure)
}

func TestTransferService_Prepare_LedgerRejection_NotInternalError(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()
	store.EXPECT().GetUserByCantonPartyID(ctx, validExternalPartyID).Return(recipientUser(), nil).Once()

	// A ledger NOT_FOUND (e.g. party disappeared between validation and
	// submission) must map to a 400-shaped error, not a 500.
	tok := mocks.NewToken(t)
	tok.EXPECT().PrepareTransfer(ctx, mock.Anything).
		Return(nil, grpcstatus.Error(codes.NotFound, "unknown informee party")).Once()

	svc := newTestService(t, tok, store, mocks.NewTransferCache(t))
	_, err := svc.Prepare(ctx, sender.EVMAddress, &PrepareRequest{
		ToPartyID:       validExternalPartyID,
		Amount:          "10",
		Token:           "DEMO",
		ValiditySeconds: 3600,
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
}

func TestTransferService_Prepare_BothRecipientForms_Rejected(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	svc := newTestService(t, mocks.NewToken(t), store, mocks.NewTransferCache(t))
	_, err := svc.Prepare(ctx, sender.EVMAddress, &PrepareRequest{
		To:              recipientUser().EVMAddress,
		ToPartyID:       validExternalPartyID,
		Amount:          "10",
		Token:           "DEMO",
		ValiditySeconds: 3600,
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
}

func TestTransferService_Prepare_NoRecipient_Rejected(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	svc := newTestService(t, mocks.NewToken(t), store, mocks.NewTransferCache(t))
	_, err := svc.Prepare(ctx, sender.EVMAddress, &PrepareRequest{
		Amount:          "10",
		Token:           "DEMO",
		ValiditySeconds: 3600,
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
}

// --- SendCustodial tests ---

func custodialSender() *user.User {
	return &user.User{
		EVMAddress:    "0xcccccccccccccccccccccccccccccccccccccccc",
		CantonPartyID: "sender::1220deadbeef",
		KeyMode:       user.KeyModeCustodial,
	}
}

func TestTransferService_SendCustodial_Success(t *testing.T) {
	ctx := context.Background()
	sender := custodialSender()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()
	store.EXPECT().GetUserByCantonPartyID(ctx, validExternalPartyID).Return(recipientUser(), nil).Once()

	tok := mocks.NewToken(t)
	// idempotencyKey is a freshly generated UUID, so match any string there.
	tok.EXPECT().TransferByPartyID(ctx, mock.Anything, sender.CantonPartyID, validExternalPartyID, "10", "DEMO", time.Hour).
		Return(nil).Once()

	svc := newTestService(t, tok, store, mocks.NewTransferCache(t))
	resp, err := svc.SendCustodial(ctx, sender.EVMAddress, &CustodialTransferRequest{
		ToPartyID:       validExternalPartyID,
		Amount:          "10",
		Token:           "DEMO",
		ValiditySeconds: 3600,
	})

	require.NoError(t, err)
	assert.Equal(t, "submitted", resp.Status)
}

func TestTransferService_SendCustodial_RequiresCustodial(t *testing.T) {
	ctx := context.Background()
	sender := senderUser() // key_mode=external

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	svc := newTestService(t, mocks.NewToken(t), store, mocks.NewTransferCache(t))
	_, err := svc.SendCustodial(ctx, sender.EVMAddress, &CustodialTransferRequest{
		ToPartyID:       validExternalPartyID,
		Amount:          "10",
		Token:           "DEMO",
		ValiditySeconds: 3600,
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
}

func TestTransferService_SendCustodial_InvalidPartyID(t *testing.T) {
	// Party-id validation happens before any store lookup, so no user mock is needed.
	svc := newTestService(t, mocks.NewToken(t), mocks.NewUserStore(t), mocks.NewTransferCache(t))
	_, err := svc.SendCustodial(context.Background(), custodialSender().EVMAddress, &CustodialTransferRequest{
		ToPartyID:       "0xdeadbeef", // missing hint::fingerprint form
		Amount:          "10",
		Token:           "DEMO",
		ValiditySeconds: 3600,
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
}

func TestTransferService_SendCustodial_RequiresValidity(t *testing.T) {
	// validity_seconds is mandatory; rejected before any store lookup.
	svc := newTestService(t, mocks.NewToken(t), mocks.NewUserStore(t), mocks.NewTransferCache(t))
	_, err := svc.SendCustodial(context.Background(), custodialSender().EVMAddress, &CustodialTransferRequest{
		ToPartyID: validExternalPartyID,
		Amount:    "10",
		Token:     "DEMO",
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
}

func TestTransferService_SendCustodial_UserNotFound(t *testing.T) {
	ctx := context.Background()
	sender := custodialSender()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(nil, user.ErrUserNotFound).Once()

	svc := newTestService(t, mocks.NewToken(t), store, mocks.NewTransferCache(t))
	_, err := svc.SendCustodial(ctx, sender.EVMAddress, &CustodialTransferRequest{
		ToPartyID:       validExternalPartyID,
		Amount:          "10",
		Token:           "DEMO",
		ValiditySeconds: 3600,
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryUnauthorized)
}

func TestTransferService_SendCustodial_InsufficientBalance(t *testing.T) {
	ctx := context.Background()
	sender := custodialSender()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()
	store.EXPECT().GetUserByCantonPartyID(ctx, validExternalPartyID).Return(recipientUser(), nil).Once()

	tok := mocks.NewToken(t)
	tok.EXPECT().TransferByPartyID(ctx, mock.Anything, sender.CantonPartyID, validExternalPartyID, "10", "DEMO", time.Hour).
		Return(token.ErrInsufficientBalance).Once()

	svc := newTestService(t, tok, store, mocks.NewTransferCache(t))
	_, err := svc.SendCustodial(ctx, sender.EVMAddress, &CustodialTransferRequest{
		ToPartyID:       validExternalPartyID,
		Amount:          "10",
		Token:           "DEMO",
		ValiditySeconds: 3600,
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
}

func TestTransferService_SendCustodial_InternalToken_UnregisteredParty_Rejected(t *testing.T) {
	ctx := context.Background()
	sender := custodialSender()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()
	store.EXPECT().GetUserByCantonPartyID(ctx, validExternalPartyID).Return(nil, user.ErrUserNotFound).Once()

	svc := newTestService(t, mocks.NewToken(t), store, mocks.NewTransferCache(t))
	_, err := svc.SendCustodial(ctx, sender.EVMAddress, &CustodialTransferRequest{
		ToPartyID:       validExternalPartyID,
		Amount:          "10",
		Token:           "PROMPT",
		ValiditySeconds: 3600,
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
	assert.Contains(t, err.Error(), "does not support transfers to external parties")
}

func TestTransferService_SendCustodial_ExternalToken_ExternalParty_Success(t *testing.T) {
	ctx := context.Background()
	sender := custodialSender()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()
	store.EXPECT().GetUserByCantonPartyID(ctx, validExternalPartyID).Return(nil, user.ErrUserNotFound).Once()

	registry := mocks.NewPartyRegistry(t)
	registry.EXPECT().PartyExists(ctx, validExternalPartyID).Return(true, nil).Once()

	tok := mocks.NewToken(t)
	tok.EXPECT().TransferByPartyID(ctx, mock.Anything, sender.CantonPartyID, validExternalPartyID, "10", "USDCx", time.Hour).
		Return(nil).Once()

	svc := newTestService(t, tok, store, mocks.NewTransferCache(t))
	svc.partyRegistry = registry
	resp, err := svc.SendCustodial(ctx, sender.EVMAddress, &CustodialTransferRequest{
		ToPartyID:       validExternalPartyID,
		Amount:          "10",
		Token:           "USDCx",
		ValiditySeconds: 3600,
	})

	require.NoError(t, err)
	assert.Equal(t, "submitted", resp.Status)
}

func TestValidatePartyID(t *testing.T) {
	cases := []struct {
		name    string
		partyID string
		wantErr bool
	}{
		{"valid", "alice::1220abcdef0123456789", false},
		{"no separator", "alice1220abcdef", true},
		{"empty hint", "::1220abcdef", true},
		{"empty fingerprint", "alice::", true},
		{"non-hex fingerprint", "alice::nothex", true},
		{"evm address", "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePartyID(tc.partyID)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// --- Execute tests ---

func TestTransferService_Execute_Success(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	pt := &token.PreparedTransfer{
		TransferID:      "txn-456",
		TransactionHash: []byte{0xca, 0xfe},
		PartyID:         sender.CantonPartyID,
		ExpiresAt:       time.Now().Add(2 * time.Minute),
	}

	cache := mocks.NewTransferCache(t)
	cache.EXPECT().GetAndDelete("txn-456").Return(pt, nil).Once()

	tok := mocks.NewToken(t)
	tok.EXPECT().ExecuteTransfer(ctx, mock.MatchedBy(func(req *token.ExecuteTransferRequest) bool {
		return req.PreparedTransfer == pt && req.SignedBy == sender.CantonPublicKeyFingerprint
	})).Return(nil).Once()

	svc := newTestService(t, tok, store, cache)

	resp, err := svc.Execute(ctx, sender.EVMAddress, &ExecuteRequest{
		TransferID: "txn-456",
		Signature:  "0xdeadbeef",
		SignedBy:   sender.CantonPublicKeyFingerprint,
	})

	require.NoError(t, err)
	assert.Equal(t, "completed", resp.Status)
}

func TestTransferService_Execute_TransferNotFound(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	cache := mocks.NewTransferCache(t)
	cache.EXPECT().GetAndDelete("nonexistent").Return(nil, ErrTransferNotFound).Once()

	svc := newTestService(t, mocks.NewToken(t), store, cache)

	_, err := svc.Execute(ctx, sender.EVMAddress, &ExecuteRequest{
		TransferID: "nonexistent",
		Signature:  "0xab",
		SignedBy:   sender.CantonPublicKeyFingerprint,
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryResourceNotFound)
}

func TestTransferService_Execute_TransferExpired(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	cache := mocks.NewTransferCache(t)
	cache.EXPECT().GetAndDelete("expired-txn").Return(nil, ErrTransferExpired).Once()

	svc := newTestService(t, mocks.NewToken(t), store, cache)

	_, err := svc.Execute(ctx, sender.EVMAddress, &ExecuteRequest{
		TransferID: "expired-txn",
		Signature:  "0xab",
		SignedBy:   sender.CantonPublicKeyFingerprint,
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryGone)
}

func TestTransferService_Execute_InvalidSignature_ReturnsForbidden(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	pt := &token.PreparedTransfer{TransferID: "txn-sig-fail"}
	cache := mocks.NewTransferCache(t)
	cache.EXPECT().GetAndDelete("txn-sig-fail").Return(pt, nil).Once()

	cantonErr := grpcstatus.Error(codes.InvalidArgument, "signature verification failed")
	tok := mocks.NewToken(t)
	tok.EXPECT().ExecuteTransfer(ctx, mock.Anything).Return(cantonErr).Once()

	svc := newTestService(t, tok, store, cache)

	_, err := svc.Execute(ctx, sender.EVMAddress, &ExecuteRequest{
		TransferID: "txn-sig-fail",
		Signature:  "0xdeadbeef",
		SignedBy:   sender.CantonPublicKeyFingerprint,
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryForbidden)
}

// --- ListIncoming tests ---

func TestTransferService_ListIncoming_Success(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	offers := mocks.NewIndexerReader(t)
	// Sender/receiver party IDs are intentionally long-form here so the test
	// exercises the server-side truncation that ListIncoming applies.
	const (
		longSender   = "party::sender-aliceXXXXXXXXXXXXXXXXX"
		longReceiver = "party::receiverXXXXXXXXXXXXXXXXXXXXX"
	)
	reqPagination := indexer.Pagination{Page: 1, Limit: 50}
	offers.EXPECT().GetTransfers(ctx, sender.CantonPartyID, indexer.TransferQuery{Role: indexer.TransferRoleReceiver, Status: indexer.TransferStatusPending}, reqPagination).
		Return(&indexer.Page[indexer.Transfer]{
			Items: []indexer.Transfer{
				{
					ContractID:      "cid-1",
					Kind:            indexer.TransferKindOffer,
					Status:          indexer.TransferStatusPending,
					FromPartyID:     longSender,
					ToPartyID:       longReceiver,
					Amount:          "10.0",
					InstrumentAdmin: "admin::issuer",
					InstrumentID:    "DEMO",
				},
				{
					ContractID:      "cid-2",
					Kind:            indexer.TransferKindOffer,
					Status:          indexer.TransferStatusPending,
					FromPartyID:     "party::sender-bob",
					ToPartyID:       longReceiver,
					Amount:          "5.5",
					InstrumentAdmin: "admin::issuer",
					InstrumentID:    "UNKNOWN",
				},
			},
			Total: 2,
			Page:  1,
			Limit: 50,
		}, nil).Once()

	svc := newTestServiceWithOffers(mocks.NewToken(t), store, mocks.NewTransferCache(t), offers)

	resp, err := svc.ListIncoming(ctx, sender.EVMAddress, reqPagination)
	require.NoError(t, err)
	require.Len(t, resp.Items, 2)
	assert.False(t, resp.HasMore)
	assert.Equal(t, int64(2), resp.Total)
	assert.Equal(t, 1, resp.Page)
	assert.Equal(t, 50, resp.Limit)

	// Truncation: 8 head + "…" + 8 tail. Verify both the format (one ellipsis
	// inside) and that the original full IDs do NOT leak into the response.
	assert.Equal(t, "cid-1", resp.Items[0].ContractID)
	assert.NotEqual(t, longSender, resp.Items[0].SenderPartyID)
	assert.NotEqual(t, longReceiver, resp.Items[0].ReceiverPartyID)
	assert.Contains(t, resp.Items[0].SenderPartyID, "…")
	assert.Equal(t, longSender[:8], resp.Items[0].SenderPartyID[:8])
	assert.Equal(t, longSender[len(longSender)-8:], resp.Items[0].SenderPartyID[len(resp.Items[0].SenderPartyID)-8:])
	assert.Equal(t, "10.0", resp.Items[0].Amount)
	assert.Equal(t, "DEMO", resp.Items[0].InstrumentID)
	assert.Equal(t, "DEMO", resp.Items[0].Symbol)
	assert.Equal(t, 18, resp.Items[0].Decimals)
	assert.Equal(t, "0x1111111111111111111111111111111111111111", resp.Items[0].ContractAddress)

	// UNKNOWN instrument: token-metadata fields stay empty. Short sender stays
	// untouched because truncation only kicks in past ~17 characters.
	assert.Equal(t, "cid-2", resp.Items[1].ContractID)
	assert.Equal(t, "party::sender-bob", resp.Items[1].SenderPartyID)
	assert.Empty(t, resp.Items[1].Symbol)
	assert.Empty(t, resp.Items[1].ContractAddress)
}

func TestTransferService_ListIncoming_HasMore(t *testing.T) {
	// Verify HasMore is computed correctly when the page does not cover the
	// total: requesting page 1 with limit 2 from a total of 5 should set
	// HasMore=true so clients know to keep paging.
	ctx := context.Background()
	sender := senderUser()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	offers := mocks.NewIndexerReader(t)
	offers.EXPECT().GetTransfers(ctx, sender.CantonPartyID, indexer.TransferQuery{Role: indexer.TransferRoleReceiver, Status: indexer.TransferStatusPending}, indexer.Pagination{Page: 1, Limit: 2}).
		Return(&indexer.Page[indexer.Transfer]{
			Items: []indexer.Transfer{
				{ContractID: "cid-1", Status: indexer.TransferStatusPending, InstrumentID: "DEMO"},
				{ContractID: "cid-2", Status: indexer.TransferStatusPending, InstrumentID: "DEMO"},
			},
			Total: 5,
			Page:  1,
			Limit: 2,
		}, nil).Once()

	svc := newTestServiceWithOffers(mocks.NewToken(t), store, mocks.NewTransferCache(t), offers)
	resp, err := svc.ListIncoming(ctx, sender.EVMAddress, indexer.Pagination{Page: 1, Limit: 2})
	require.NoError(t, err)
	assert.True(t, resp.HasMore)
	assert.Equal(t, int64(5), resp.Total)
	assert.Equal(t, 1, resp.Page)
	assert.Equal(t, 2, resp.Limit)
}

func TestTransferService_ListIncoming_EmptyReturnsEmptySlice(t *testing.T) {
	// Regression for the Gemini review: a nil items slice marshals to `null`
	// instead of `[]`, which trips client list-iteration code. Make sure an
	// indexer page with zero results still surfaces an initialized slice.
	ctx := context.Background()
	sender := senderUser()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	offers := mocks.NewIndexerReader(t)
	offers.EXPECT().GetTransfers(ctx, sender.CantonPartyID, mock.Anything, mock.Anything).
		Return(&indexer.Page[indexer.Transfer]{
			Items: []indexer.Transfer{},
			Total: 0,
			Page:  1,
			Limit: 200,
		}, nil).Once()

	svc := newTestServiceWithOffers(mocks.NewToken(t), store, mocks.NewTransferCache(t), offers)
	resp, err := svc.ListIncoming(ctx, sender.EVMAddress, indexer.Pagination{Page: 1, Limit: 200})
	require.NoError(t, err)
	require.NotNil(t, resp.Items)
	assert.Empty(t, resp.Items)
}

func TestTransferService_ListIncoming_UserNotFound(t *testing.T) {
	ctx := context.Background()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, senderUser().EVMAddress).Return(nil, user.ErrUserNotFound).Once()

	svc := newTestServiceWithOffers(mocks.NewToken(t), store, mocks.NewTransferCache(t), mocks.NewIndexerReader(t))

	_, err := svc.ListIncoming(ctx, senderUser().EVMAddress, indexer.Pagination{Page: 1, Limit: 50})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
}

func TestTransferService_ListIncoming_CustodialUserRejected(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()
	sender.KeyMode = user.KeyModeCustodial

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	svc := newTestServiceWithOffers(mocks.NewToken(t), store, mocks.NewTransferCache(t), mocks.NewIndexerReader(t))

	_, err := svc.ListIncoming(ctx, sender.EVMAddress, indexer.Pagination{Page: 1, Limit: 50})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
}

// --- PrepareAccept tests ---

func TestTransferService_PrepareAccept_Success(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()
	const contractID = "offer-contract-1"
	const instrumentAdmin = "admin::zzz"

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	pt := &token.PreparedTransfer{
		TransferID:      "accept-txn-1",
		TransactionHash: []byte{0xab, 0xcd},
		PartyID:         sender.CantonPartyID,
		ExpiresAt:       time.Now().Add(2 * time.Minute),
	}

	tok := mocks.NewToken(t)
	tok.EXPECT().PrepareAcceptTransfer(ctx, sender.CantonPartyID, contractID, instrumentAdmin).
		Return(pt, nil).Once()

	cache := mocks.NewTransferCache(t)
	cache.EXPECT().Put(pt).Return(nil).Once()

	svc := newTestService(t, tok, store, cache)

	resp, err := svc.PrepareAccept(ctx, sender.EVMAddress, contractID, &PrepareAcceptRequest{
		InstrumentAdmin: instrumentAdmin,
	})

	require.NoError(t, err)
	assert.Equal(t, "accept-txn-1", resp.TransferID)
	assert.Equal(t, "0xabcd", resp.TransactionHash)
	assert.Equal(t, sender.CantonPartyID, resp.PartyID)
}

func TestTransferService_PrepareAccept_UserNotFound(t *testing.T) {
	ctx := context.Background()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, senderUser().EVMAddress).Return(nil, user.ErrUserNotFound).Once()

	svc := newTestService(t, mocks.NewToken(t), store, mocks.NewTransferCache(t))

	_, err := svc.PrepareAccept(ctx, senderUser().EVMAddress, "cid-1", &PrepareAcceptRequest{
		InstrumentAdmin: "admin::zzz",
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryUnauthorized)
}

func TestTransferService_PrepareAccept_CustodialUserRejected(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()
	sender.KeyMode = user.KeyModeCustodial

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	svc := newTestService(t, mocks.NewToken(t), store, mocks.NewTransferCache(t))

	_, err := svc.PrepareAccept(ctx, sender.EVMAddress, "cid-1", &PrepareAcceptRequest{
		InstrumentAdmin: "admin::zzz",
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
}

// --- ExecuteAccept tests ---

func TestTransferService_ExecuteAccept_DelegatesToExecute(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	pt := &token.PreparedTransfer{
		TransferID:      "accept-exec-1",
		TransactionHash: []byte{0xff},
		PartyID:         sender.CantonPartyID,
		ExpiresAt:       time.Now().Add(2 * time.Minute),
	}

	cache := mocks.NewTransferCache(t)
	cache.EXPECT().GetAndDelete("accept-exec-1").Return(pt, nil).Once()

	tok := mocks.NewToken(t)
	tok.EXPECT().ExecuteTransfer(ctx, mock.MatchedBy(func(req *token.ExecuteTransferRequest) bool {
		return req.PreparedTransfer == pt && req.SignedBy == sender.CantonPublicKeyFingerprint
	})).Return(nil).Once()

	svc := newTestService(t, tok, store, cache)

	resp, err := svc.ExecuteAccept(ctx, sender.EVMAddress, &ExecuteRequest{
		TransferID: "accept-exec-1",
		Signature:  "0xdeadbeef",
		SignedBy:   sender.CantonPublicKeyFingerprint,
	})

	require.NoError(t, err)
	assert.Equal(t, "completed", resp.Status)
}

// --- ListOutgoing tests ---

func TestTransferService_ListOutgoing_Success(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()
	expires := time.Now().Add(time.Hour).UTC()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	offers := mocks.NewIndexerReader(t)
	page := indexer.Pagination{Page: 1, Limit: 50}
	offers.EXPECT().GetTransfers(ctx, sender.CantonPartyID,
		indexer.TransferQuery{Role: indexer.TransferRoleSender, Status: indexer.TransferStatusPending}, page).
		Return(&indexer.Page[indexer.Transfer]{
			Items: []indexer.Transfer{
				{
					ContractID:      "out-1",
					Kind:            indexer.TransferKindOffer,
					Status:          indexer.TransferStatusPending,
					FromPartyID:     sender.CantonPartyID,
					ToPartyID:       "party::receiverXXXXXXXXXXXXXXXXXXXXX",
					Amount:          "7.5",
					InstrumentAdmin: "admin::issuer",
					InstrumentID:    "DEMO",
					ExpiresAt:       &expires,
				},
			},
			Total: 1, Page: 1, Limit: 50,
		}, nil).Once()

	svc := newTestServiceWithOffers(mocks.NewToken(t), store, mocks.NewTransferCache(t), offers)
	resp, err := svc.ListOutgoing(ctx, sender.EVMAddress, indexer.TransferStatusPending, page)
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "out-1", resp.Items[0].ContractID)
	assert.Equal(t, "pending", resp.Items[0].Status)
	assert.NotEmpty(t, resp.Items[0].ExpiresAt)
	// Counterparty (receiver) is truncated; instrument metadata enriched from config.
	assert.Contains(t, resp.Items[0].ReceiverPartyID, "…")
	assert.Equal(t, "DEMO", resp.Items[0].Symbol)
	assert.Equal(t, 18, resp.Items[0].Decimals)
	assert.False(t, resp.HasMore)
}

func TestTransferService_ListOutgoing_UserNotFound(t *testing.T) {
	ctx := context.Background()
	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, senderUser().EVMAddress).Return(nil, user.ErrUserNotFound).Once()

	svc := newTestServiceWithOffers(mocks.NewToken(t), store, mocks.NewTransferCache(t), mocks.NewIndexerReader(t))
	_, err := svc.ListOutgoing(ctx, senderUser().EVMAddress, "", indexer.Pagination{Page: 1, Limit: 50})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
}

// --- ListCompleted tests ---

func TestTransferService_ListCompleted_Success(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()
	ts := time.Now().UTC()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	offers := mocks.NewIndexerReader(t)
	page := indexer.Pagination{Page: 1, Limit: 50}
	offers.EXPECT().GetTransfers(ctx, sender.CantonPartyID,
		indexer.TransferQuery{Role: indexer.TransferRoleAny, Status: indexer.TransferStatusCompleted}, page).
		Return(&indexer.Page[indexer.Transfer]{
			Items: []indexer.Transfer{
				{ // our-token settled transfer (direct)
					ContractID:      "ev-1",
					Kind:            indexer.TransferKindDirect,
					Status:          indexer.TransferStatusCompleted,
					FromPartyID:     sender.CantonPartyID,
					ToPartyID:       "party::receiverXXXXXXXXXXXXXXXXXXXXX",
					Amount:          "3",
					InstrumentAdmin: "admin::issuer",
					InstrumentID:    "DEMO",
					TxID:            "tx-1",
					CreatedAt:       ts,
				},
				{ // USDCx settled transfer (offer-based)
					ContractID:      "of-1",
					Kind:            indexer.TransferKindOffer,
					Status:          indexer.TransferStatusCompleted,
					FromPartyID:     "party::senderXXXXXXXXXXXXXXXXXXXXXXX",
					ToPartyID:       sender.CantonPartyID,
					Amount:          "9",
					InstrumentAdmin: "circle::admin",
					InstrumentID:    "USDC",
					CreatedAt:       ts,
				},
			},
			Total: 2, Page: 1, Limit: 50,
		}, nil).Once()

	svc := newTestServiceWithOffers(mocks.NewToken(t), store, mocks.NewTransferCache(t), offers)
	resp, err := svc.ListCompleted(ctx, sender.EVMAddress, page)
	require.NoError(t, err)
	require.Len(t, resp.Items, 2)

	assert.Equal(t, "direct", resp.Items[0].Kind)
	assert.Equal(t, "tx-1", resp.Items[0].TxID)
	assert.Equal(t, "DEMO", resp.Items[0].Symbol) // enriched from config
	assert.Contains(t, resp.Items[0].ToPartyID, "…")

	assert.Equal(t, "offer", resp.Items[1].Kind)
	assert.Empty(t, resp.Items[1].TxID)   // offers carry no tx id
	assert.Empty(t, resp.Items[1].Symbol) // USDC not in test config -> not enriched
	assert.Equal(t, "USDC", resp.Items[1].InstrumentID)
}

func TestTransferService_ListCompleted_UserNotFound(t *testing.T) {
	ctx := context.Background()
	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, senderUser().EVMAddress).Return(nil, user.ErrUserNotFound).Once()

	svc := newTestServiceWithOffers(mocks.NewToken(t), store, mocks.NewTransferCache(t), mocks.NewIndexerReader(t))
	_, err := svc.ListCompleted(ctx, senderUser().EVMAddress, indexer.Pagination{Page: 1, Limit: 50})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
}

// --- Withdraw (claim back) tests ---

const withdrawCID = "offer-cid-1"

// withdrawableOffer is an offer-kind transfer the given sender owns, in a state that
// can be claimed back. The claim-back lookup queries by sender role, so ownership is
// implied by the party the mock is keyed on.
func withdrawableOffer(sender *user.User) indexer.Transfer {
	return indexer.Transfer{
		ContractID:      withdrawCID,
		Kind:            indexer.TransferKindOffer,
		Status:          indexer.TransferStatusExpired,
		FromPartyID:     sender.CantonPartyID,
		ToPartyID:       "external::1220abcd",
		InstrumentAdmin: "usdc::admin",
		InstrumentID:    "USDCx",
		Amount:          "10",
	}
}

func expectGetTransfer(offers *mocks.IndexerReader, ctx context.Context, offer *indexer.Transfer) {
	offers.EXPECT().GetTransfer(ctx, withdrawCID).Return(offer, nil).Once()
}

func TestTransferService_PrepareWithdraw_Success(t *testing.T) {
	ctx := context.Background()
	sender := senderUser() // external key mode

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	offer := withdrawableOffer(sender)
	offers := mocks.NewIndexerReader(t)
	expectGetTransfer(offers, ctx, &offer)

	prepared := &token.PreparedTransfer{
		TransferID: "wd-1", TransactionHash: []byte{0x01, 0x02},
		PartyID: sender.CantonPartyID, ExpiresAt: time.Now().Add(time.Hour),
	}
	tok := mocks.NewToken(t)
	tok.EXPECT().PrepareWithdrawTransfer(ctx, sender.CantonPartyID, withdrawCID, "usdc::admin").
		Return(prepared, nil).Once()

	cache := mocks.NewTransferCache(t)
	cache.EXPECT().Put(prepared).Return(nil).Once()

	svc := newTestServiceWithOffers(tok, store, cache, offers)
	resp, err := svc.PrepareWithdraw(ctx, sender.EVMAddress, withdrawCID)
	require.NoError(t, err)
	assert.Equal(t, "wd-1", resp.TransferID)
	assert.Equal(t, sender.CantonPartyID, resp.PartyID)
}

func TestTransferService_WithdrawCustodial_Success(t *testing.T) {
	ctx := context.Background()
	sender := custodialSender()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	offer := withdrawableOffer(sender)
	offer.Status = indexer.TransferStatusPending // a still-pending offer is claimable too
	offers := mocks.NewIndexerReader(t)
	expectGetTransfer(offers, ctx, &offer)

	tok := mocks.NewToken(t)
	tok.EXPECT().WithdrawTransferInstruction(ctx, sender.CantonPartyID, withdrawCID, "usdc::admin").
		Return(nil).Once()

	svc := newTestServiceWithOffers(tok, store, mocks.NewTransferCache(t), offers)
	resp, err := svc.WithdrawCustodial(ctx, sender.EVMAddress, withdrawCID)
	require.NoError(t, err)
	assert.Equal(t, "submitted", resp.Status)
}

func TestTransferService_PrepareWithdraw_NotFound(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	offers := mocks.NewIndexerReader(t)
	offers.EXPECT().GetTransfer(ctx, "missing-cid").
		Return(nil, apperrors.ResourceNotFoundError(nil, "transfer not found")).Once()

	svc := newTestServiceWithOffers(mocks.NewToken(t), store, mocks.NewTransferCache(t), offers)
	_, err := svc.PrepareWithdraw(ctx, sender.EVMAddress, "missing-cid")
	assertServiceErrorCategory(t, err, apperrors.CategoryResourceNotFound)
}

func TestTransferService_PrepareWithdraw_ForeignOffer(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	// An offer sent by a different party must not be claimable by this caller; it is
	// reported as not-found so callers can't probe foreign offers by contract id.
	foreign := withdrawableOffer(sender)
	foreign.FromPartyID = "party::someone-else"
	offers := mocks.NewIndexerReader(t)
	expectGetTransfer(offers, ctx, &foreign)

	svc := newTestServiceWithOffers(mocks.NewToken(t), store, mocks.NewTransferCache(t), offers)
	_, err := svc.PrepareWithdraw(ctx, sender.EVMAddress, withdrawCID)
	assertServiceErrorCategory(t, err, apperrors.CategoryResourceNotFound)
}

func TestTransferService_PrepareWithdraw_RequiresExternal(t *testing.T) {
	ctx := context.Background()
	sender := custodialSender() // custodial cannot use the prepare/execute withdraw

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	svc := newTestServiceWithOffers(mocks.NewToken(t), store, mocks.NewTransferCache(t), mocks.NewIndexerReader(t))
	_, err := svc.PrepareWithdraw(ctx, sender.EVMAddress, withdrawCID)
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
}

func TestTransferService_WithdrawCustodial_NotAnOffer(t *testing.T) {
	ctx := context.Background()
	sender := custodialSender()

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	// A direct (CIP-56) transfer is not an offer and cannot be claimed back.
	direct := withdrawableOffer(sender)
	direct.Kind = indexer.TransferKindDirect
	direct.Status = indexer.TransferStatusCompleted
	offers := mocks.NewIndexerReader(t)
	expectGetTransfer(offers, ctx, &direct)

	svc := newTestServiceWithOffers(mocks.NewToken(t), store, mocks.NewTransferCache(t), offers)
	_, err := svc.WithdrawCustodial(ctx, sender.EVMAddress, withdrawCID)
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
}
