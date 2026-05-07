package transfer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/chainsafe/canton-middleware/pkg/indexer"
	indexermocks "github.com/chainsafe/canton-middleware/pkg/indexer/client/mocks"
	"github.com/chainsafe/canton-middleware/pkg/transfer/mocks"
	"github.com/chainsafe/canton-middleware/pkg/user"
)

func custodialUser() *user.User {
	return &user.User{
		EVMAddress:    "0xcccccccccccccccccccccccccccccccccccccccc",
		CantonPartyID: "custodial-party::abc",
		KeyMode:       user.KeyModeCustodial,
	}
}

func externalUser() *user.User {
	return &user.User{
		EVMAddress:    "0xdddddddddddddddddddddddddddddddddddddddd",
		CantonPartyID: "external-party::def",
		KeyMode:       user.KeyModeExternal,
	}
}

const testInstrumentAdmin = "admin-party::zzz"

func pendingOffer(contractID string) indexer.PendingOffer {
	return indexer.PendingOffer{
		ContractID:      contractID,
		Status:          indexer.OfferStatusPending,
		ReceiverPartyID: "custodial-party::abc",
		SenderPartyID:   "sender-party::xyz",
		InstrumentAdmin: testInstrumentAdmin,
		InstrumentID:    "USDCX",
		Amount:          "100.0",
		LedgerOffset:    42,
	}
}

func TestAcceptWorker_SkipsExternalUsers(t *testing.T) {
	tok := mocks.NewToken(t)
	lister := mocks.NewUserLister(t)
	ic := indexermocks.NewClient(t)

	// Only external user returned — no offers should be fetched
	lister.EXPECT().ListUsers(mock.Anything).Return([]*user.User{externalUser()}, nil)

	worker := NewAcceptWorker(tok, lister, ic, time.Hour, zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	worker.acceptPending(ctx)
}

func TestAcceptWorker_AcceptsSingleOffer(t *testing.T) {
	tok := mocks.NewToken(t)
	lister := mocks.NewUserLister(t)
	ic := indexermocks.NewClient(t)

	offer := pendingOffer("contract-1")
	page := &indexer.Page[indexer.PendingOffer]{Items: []indexer.PendingOffer{offer}, Total: 1, Page: 1, Limit: 200}

	lister.EXPECT().ListUsers(mock.Anything).Return([]*user.User{custodialUser()}, nil)
	ic.EXPECT().GetPendingOffersForParty(mock.Anything, "custodial-party::abc", indexer.Pagination{Page: 1, Limit: 200}).
		Return(page, nil)
	tok.EXPECT().AcceptTransferInstruction(mock.Anything, "custodial-party::abc", "contract-1", "admin-party::zzz").
		Return(nil)

	worker := NewAcceptWorker(tok, lister, ic, time.Hour, zap.NewNop())
	worker.acceptPending(context.Background())
}

func TestAcceptWorker_LogsAndContinuesOnAcceptError(t *testing.T) {
	tok := mocks.NewToken(t)
	lister := mocks.NewUserLister(t)
	ic := indexermocks.NewClient(t)

	offer1 := pendingOffer("contract-1")
	offer2 := pendingOffer("contract-2")
	page := &indexer.Page[indexer.PendingOffer]{Items: []indexer.PendingOffer{offer1, offer2}, Total: 2, Page: 1, Limit: 200}

	lister.EXPECT().ListUsers(mock.Anything).Return([]*user.User{custodialUser()}, nil)
	ic.EXPECT().GetPendingOffersForParty(mock.Anything, mock.Anything, mock.Anything).Return(page, nil)
	// First offer fails (e.g. already accepted double-accept)
	tok.EXPECT().AcceptTransferInstruction(mock.Anything, mock.Anything, "contract-1", mock.Anything).
		Return(errors.New("ALREADY_EXISTS"))
	// Second offer should still be attempted
	tok.EXPECT().AcceptTransferInstruction(mock.Anything, mock.Anything, "contract-2", mock.Anything).
		Return(nil)

	worker := NewAcceptWorker(tok, lister, ic, time.Hour, zap.NewNop())
	worker.acceptPending(context.Background())
}

func TestAcceptWorker_LogsAndContinuesOnIndexerError(t *testing.T) {
	tok := mocks.NewToken(t)
	lister := mocks.NewUserLister(t)
	ic := indexermocks.NewClient(t)

	lister.EXPECT().ListUsers(mock.Anything).Return([]*user.User{custodialUser()}, nil)
	ic.EXPECT().GetPendingOffersForParty(mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("indexer unavailable"))

	worker := NewAcceptWorker(tok, lister, ic, time.Hour, zap.NewNop())
	// Must not panic or call AcceptTransferInstruction
	worker.acceptPending(context.Background())
}

func TestAcceptWorker_StopsOnContextCancel(t *testing.T) {
	tok := mocks.NewToken(t)
	lister := mocks.NewUserLister(t)
	ic := indexermocks.NewClient(t)

	lister.EXPECT().ListUsers(mock.Anything).Maybe().Return([]*user.User{}, nil)

	worker := NewAcceptWorker(tok, lister, ic, 50*time.Millisecond, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop after context cancellation")
	}
}

func TestAcceptWorker_PaginatesAllOffers(t *testing.T) {
	tok := mocks.NewToken(t)
	lister := mocks.NewUserLister(t)
	ic := indexermocks.NewClient(t)

	offer1 := pendingOffer("contract-1")
	offer2 := pendingOffer("contract-2")

	// Total is 400, limit 200 → 2 pages
	page1 := &indexer.Page[indexer.PendingOffer]{Items: []indexer.PendingOffer{offer1}, Total: 400, Page: 1, Limit: 200}
	page2 := &indexer.Page[indexer.PendingOffer]{Items: []indexer.PendingOffer{offer2}, Total: 400, Page: 2, Limit: 200}

	lister.EXPECT().ListUsers(mock.Anything).Return([]*user.User{custodialUser()}, nil)
	ic.EXPECT().GetPendingOffersForParty(mock.Anything, "custodial-party::abc", indexer.Pagination{Page: 1, Limit: 200}).
		Return(page1, nil)
	ic.EXPECT().GetPendingOffersForParty(mock.Anything, "custodial-party::abc", indexer.Pagination{Page: 2, Limit: 200}).
		Return(page2, nil)
	tok.EXPECT().AcceptTransferInstruction(mock.Anything, mock.Anything, "contract-1", mock.Anything).Return(nil)
	tok.EXPECT().AcceptTransferInstruction(mock.Anything, mock.Anything, "contract-2", mock.Anything).Return(nil)

	worker := NewAcceptWorker(tok, lister, ic, time.Hour, zap.NewNop())
	worker.acceptPending(context.Background())
}

func TestAcceptWorker_ListUsersError(t *testing.T) {
	tok := mocks.NewToken(t)
	lister := mocks.NewUserLister(t)
	ic := indexermocks.NewClient(t)

	lister.EXPECT().ListUsers(mock.Anything).Return(nil, errors.New("db down"))

	worker := NewAcceptWorker(tok, lister, ic, time.Hour, zap.NewNop())
	require.NotPanics(t, func() { worker.acceptPending(context.Background()) })
}
