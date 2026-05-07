package custodial

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/chainsafe/canton-middleware/pkg/custodial/mocks"
	"github.com/chainsafe/canton-middleware/pkg/indexer"
	indexermocks "github.com/chainsafe/canton-middleware/pkg/indexer/client/mocks"
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

func allOffersPage(offers ...indexer.PendingOffer) *indexer.Page[indexer.PendingOffer] {
	return &indexer.Page[indexer.PendingOffer]{
		Items: offers,
		Total: int64(len(offers)),
		Page:  1,
		Limit: acceptWorkerPageLimit,
	}
}

func TestAcceptWorker_SkipsExternalUsers(t *testing.T) {
	tok := mocks.NewToken(t)
	lister := mocks.NewUserLister(t)
	ic := indexermocks.NewClient(t)

	// Only external user — custodialParties map is empty so worker returns early
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
	page := allOffersPage(offer)

	lister.EXPECT().ListUsers(mock.Anything).Return([]*user.User{custodialUser()}, nil)
	ic.EXPECT().GetAllPendingOffers(mock.Anything, indexer.Pagination{Page: 1, Limit: acceptWorkerPageLimit}).
		Return(page, nil)
	tok.EXPECT().AcceptTransferInstruction(mock.Anything, "custodial-party::abc", "contract-1", testInstrumentAdmin).
		Return(nil)

	worker := NewAcceptWorker(tok, lister, ic, time.Hour, zap.NewNop())
	worker.acceptPending(context.Background())
}

func TestAcceptWorker_SkipsOffersForNonCustodialParties(t *testing.T) {
	tok := mocks.NewToken(t)
	lister := mocks.NewUserLister(t)
	ic := indexermocks.NewClient(t)

	// Offer for a party not in our custodialParties map
	nonCustodialOffer := indexer.PendingOffer{
		ContractID:      "contract-external",
		ReceiverPartyID: "external-party::def",
		InstrumentAdmin: testInstrumentAdmin,
	}
	custodialOffer := pendingOffer("contract-custodial")
	page := allOffersPage(nonCustodialOffer, custodialOffer)
	page.Total = 2

	lister.EXPECT().ListUsers(mock.Anything).Return([]*user.User{custodialUser()}, nil)
	ic.EXPECT().GetAllPendingOffers(mock.Anything, mock.Anything).Return(page, nil)
	// Only the custodial offer should be accepted
	tok.EXPECT().AcceptTransferInstruction(mock.Anything, "custodial-party::abc", "contract-custodial", testInstrumentAdmin).
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
	page := &indexer.Page[indexer.PendingOffer]{
		Items: []indexer.PendingOffer{offer1, offer2},
		Total: 2,
		Page:  1,
		Limit: acceptWorkerPageLimit,
	}

	lister.EXPECT().ListUsers(mock.Anything).Return([]*user.User{custodialUser()}, nil)
	ic.EXPECT().GetAllPendingOffers(mock.Anything, mock.Anything).Return(page, nil)
	// First offer fails
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
	ic.EXPECT().GetAllPendingOffers(mock.Anything, mock.Anything).
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
	page1 := &indexer.Page[indexer.PendingOffer]{Items: []indexer.PendingOffer{offer1}, Total: 400, Page: 1, Limit: acceptWorkerPageLimit}
	page2 := &indexer.Page[indexer.PendingOffer]{Items: []indexer.PendingOffer{offer2}, Total: 400, Page: 2, Limit: acceptWorkerPageLimit}

	lister.EXPECT().ListUsers(mock.Anything).Return([]*user.User{custodialUser()}, nil)
	ic.EXPECT().GetAllPendingOffers(mock.Anything, indexer.Pagination{Page: 1, Limit: acceptWorkerPageLimit}).
		Return(page1, nil)
	ic.EXPECT().GetAllPendingOffers(mock.Anything, indexer.Pagination{Page: 2, Limit: acceptWorkerPageLimit}).
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
