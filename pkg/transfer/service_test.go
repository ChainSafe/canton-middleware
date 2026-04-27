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
	return &TransferService{
		cantonToken:         tok,
		userStore:           store,
		cache:               cache,
		allowedTokenSymbols: map[string]bool{"DEMO": true, "PROMPT": true},
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
