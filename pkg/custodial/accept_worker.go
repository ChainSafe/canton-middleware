// Package custodial implements background automation for custodial Canton parties.
package custodial

import (
	"context"
	"time"

	cantontkn "github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/pkg/indexer"
	indexerclient "github.com/chainsafe/canton-middleware/pkg/indexer/client"
	"github.com/chainsafe/canton-middleware/pkg/user"

	"go.uber.org/zap"
)

const acceptWorkerPageLimit = 200

// UserLister is the narrow user-store interface the AcceptWorker needs.
//
//go:generate mockery --name UserLister --output mocks --outpkg mocks --filename mock_user_lister.go --with-expecter
//go:generate mockery --srcpkg github.com/chainsafe/canton-middleware/pkg/cantonsdk/token --name Token --output mocks --outpkg mocks --filename mock_canton_token.go --with-expecter
type UserLister interface {
	ListCustodialUsers(ctx context.Context) ([]*user.User, error)
}

// AcceptWorker polls the indexer for all pending TransferOffers and automatically
// accepts them on behalf of registered custodial parties.
//
// It runs as a background goroutine and stops when ctx is canceled.
// Each poll cycle fetches all PENDING offers in one paginated stream and
// checks each receiver against an in-memory map of custodial party IDs built
// from a single ListUsers call. This is O(1 DB round-trip + P indexer pages)
// per cycle regardless of how many custodial users exist.
//
// A custodial user registered after the ListUsers call at the start of a cycle
// is caught on the next tick — at most one poll-interval delay.
type AcceptWorker struct {
	cantonToken  cantontkn.Token
	userLister   UserLister
	indexer      indexerclient.Client
	pollInterval time.Duration
	logger       *zap.Logger
}

// NewAcceptWorker creates a new AcceptWorker.
func NewAcceptWorker(
	cantonToken cantontkn.Token,
	userLister UserLister,
	indexerClient indexerclient.Client,
	pollInterval time.Duration,
	logger *zap.Logger,
) *AcceptWorker {
	return &AcceptWorker{
		cantonToken:  cantonToken,
		userLister:   userLister,
		indexer:      indexerClient,
		pollInterval: pollInterval,
		logger:       logger,
	}
}

// Run starts the accept worker loop. It blocks until ctx is canceled.
func (w *AcceptWorker) Run(ctx context.Context) {
	w.logger.Info("accept worker started", zap.Duration("poll_interval", w.pollInterval))
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("accept worker stopped")
			return
		case <-ticker.C:
			w.acceptPending(ctx)
		}
	}
}

func (w *AcceptWorker) acceptPending(ctx context.Context) {
	users, err := w.userLister.ListCustodialUsers(ctx)
	if err != nil {
		w.logger.Warn("accept worker: failed to list custodial users", zap.Error(err))
		return
	}
	if len(users) == 0 {
		return
	}

	custodialParties := make(map[string]bool, len(users))
	for _, u := range users {
		custodialParties[u.CantonPartyID] = true
	}

	page := 1
	for {
		result, err := w.indexer.GetAllPendingOffers(ctx, indexer.Pagination{
			Page:  page,
			Limit: acceptWorkerPageLimit,
		})
		if err != nil {
			w.logger.Warn("accept worker: failed to fetch pending offers", zap.Error(err))
			return
		}

		for i := range result.Items {
			offer := result.Items[i]
			if !custodialParties[offer.ReceiverPartyID] {
				continue
			}
			if err := w.cantonToken.AcceptTransferInstruction(
				ctx, offer.ReceiverPartyID, offer.ContractID, offer.InstrumentAdmin,
			); err != nil {
				w.logger.Warn("accept worker: failed to accept offer",
					zap.String("party_id", offer.ReceiverPartyID),
					zap.String("contract_id", offer.ContractID),
					zap.Error(err),
				)
				continue
			}
			w.logger.Info("accept worker: accepted transfer offer",
				zap.String("party_id", offer.ReceiverPartyID),
				zap.String("contract_id", offer.ContractID),
				zap.String("sender", offer.SenderPartyID),
				zap.String("amount", offer.Amount),
			)
		}

		if int64(page*acceptWorkerPageLimit) >= result.Total {
			return
		}
		page++
	}
}
