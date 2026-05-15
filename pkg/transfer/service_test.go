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

func newTestService(tok *mocks.Token, store *mocks.UserStore, cache *mocks.TransferCache) *TransferService {
	return newTestServiceWithOffers(tok, store, cache, nil)
}

func newTestServiceWithOffers(
	tok *mocks.Token,
	store *mocks.UserStore,
	cache *mocks.TransferCache,
	offers *mocks.PendingOfferLister,
) *TransferService {
	// Assigning a typed nil *mocks.PendingOfferLister to the interface field would
	// produce a non-nil interface value, hiding the "no indexer configured" branch.
	// Use a real nil interface when no mock is supplied.
	var offerLister PendingOfferLister
	if offers != nil {
		offerLister = offers
	}
	return &TransferService{
		cantonToken:         tok,
		userStore:           store,
		cache:               cache,
		offerLister:         offerLister,
		allowedTokenSymbols: map[string]bool{"DEMO": true, "PROMPT": true},
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
	}).Return(prepared, nil).Once()

	cache := mocks.NewTransferCache(t)
	cache.EXPECT().Put(prepared).Return(nil).Once()

	svc := newTestService(tok, store, cache)
	resp, err := svc.Prepare(ctx, sender.EVMAddress, &PrepareRequest{
		To:     recipient.EVMAddress,
		Amount: "100.5",
		Token:  "DEMO",
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

	svc := newTestService(mocks.NewToken(t), store, mocks.NewTransferCache(t))

	_, err := svc.Prepare(ctx, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", &PrepareRequest{
		To:     "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Amount: "10",
		Token:  "DEMO",
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryUnauthorized)
}

func TestTransferService_Prepare_SenderNotExternal(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()
	sender.KeyMode = user.KeyModeCustodial

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	svc := newTestService(mocks.NewToken(t), store, mocks.NewTransferCache(t))

	_, err := svc.Prepare(ctx, sender.EVMAddress, &PrepareRequest{
		To:     "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Amount: "10",
		Token:  "DEMO",
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
}

func TestTransferService_Prepare_UnsupportedToken(t *testing.T) {
	svc := newTestService(mocks.NewToken(t), mocks.NewUserStore(t), mocks.NewTransferCache(t))

	_, err := svc.Prepare(context.Background(), senderUser().EVMAddress, &PrepareRequest{
		To:     "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Amount: "10",
		Token:  "BOGUS",
	})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
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

	svc := newTestService(tok, store, cache)

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

	svc := newTestService(mocks.NewToken(t), store, cache)

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

	svc := newTestService(mocks.NewToken(t), store, cache)

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

	svc := newTestService(tok, store, cache)

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

	offers := mocks.NewPendingOfferLister(t)
	// Sender/receiver party IDs are intentionally long-form here so the test
	// exercises the server-side truncation that ListIncoming applies.
	const (
		longSender   = "party::sender-aliceXXXXXXXXXXXXXXXXX"
		longReceiver = "party::receiverXXXXXXXXXXXXXXXXXXXXX"
	)
	reqPagination := indexer.Pagination{Page: 1, Limit: 50}
	offers.EXPECT().GetPendingOffersForParty(ctx, sender.CantonPartyID, reqPagination).
		Return(&indexer.Page[indexer.PendingOffer]{
			Items: []indexer.PendingOffer{
				{
					ContractID:      "cid-1",
					Status:          indexer.OfferStatusPending,
					SenderPartyID:   longSender,
					ReceiverPartyID: longReceiver,
					Amount:          "10.0",
					InstrumentAdmin: "admin::issuer",
					InstrumentID:    "DEMO",
				},
				{
					ContractID:      "cid-2",
					Status:          indexer.OfferStatusPending,
					SenderPartyID:   "party::sender-bob",
					ReceiverPartyID: longReceiver,
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

	offers := mocks.NewPendingOfferLister(t)
	offers.EXPECT().GetPendingOffersForParty(ctx, sender.CantonPartyID, indexer.Pagination{Page: 1, Limit: 2}).
		Return(&indexer.Page[indexer.PendingOffer]{
			Items: []indexer.PendingOffer{
				{ContractID: "cid-1", Status: indexer.OfferStatusPending, InstrumentID: "DEMO"},
				{ContractID: "cid-2", Status: indexer.OfferStatusPending, InstrumentID: "DEMO"},
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

	offers := mocks.NewPendingOfferLister(t)
	offers.EXPECT().GetPendingOffersForParty(ctx, sender.CantonPartyID, mock.Anything).
		Return(&indexer.Page[indexer.PendingOffer]{
			Items: []indexer.PendingOffer{},
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

	svc := newTestServiceWithOffers(mocks.NewToken(t), store, mocks.NewTransferCache(t), mocks.NewPendingOfferLister(t))

	_, err := svc.ListIncoming(ctx, senderUser().EVMAddress, indexer.Pagination{Page: 1, Limit: 50})
	assertServiceErrorCategory(t, err, apperrors.CategoryDataError)
}

func TestTransferService_ListIncoming_IndexerNotConfigured(t *testing.T) {
	svc := newTestService(mocks.NewToken(t), mocks.NewUserStore(t), mocks.NewTransferCache(t))

	_, err := svc.ListIncoming(context.Background(), senderUser().EVMAddress, indexer.Pagination{Page: 1, Limit: 50})
	assertServiceErrorCategory(t, err, apperrors.CategoryGeneralError)
}

func TestTransferService_ListIncoming_CustodialUserRejected(t *testing.T) {
	ctx := context.Background()
	sender := senderUser()
	sender.KeyMode = user.KeyModeCustodial

	store := mocks.NewUserStore(t)
	store.EXPECT().GetUserByEVMAddress(ctx, sender.EVMAddress).Return(sender, nil).Once()

	svc := newTestServiceWithOffers(mocks.NewToken(t), store, mocks.NewTransferCache(t), mocks.NewPendingOfferLister(t))

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

	svc := newTestService(tok, store, cache)

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

	svc := newTestService(mocks.NewToken(t), store, mocks.NewTransferCache(t))

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

	svc := newTestService(mocks.NewToken(t), store, mocks.NewTransferCache(t))

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

	svc := newTestService(tok, store, cache)

	resp, err := svc.ExecuteAccept(ctx, sender.EVMAddress, &ExecuteRequest{
		TransferID: "accept-exec-1",
		Signature:  "0xdeadbeef",
		SignedBy:   sender.CantonPublicKeyFingerprint,
	})

	require.NoError(t, err)
	assert.Equal(t, "completed", resp.Status)
}
