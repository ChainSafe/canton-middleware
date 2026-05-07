package transfer

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
type UserLister interface {
	ListUsers(ctx context.Context) ([]*user.User, error)
}

// AcceptWorker polls the indexer for pending inbound TransferOffers and automatically
// accepts them on behalf of custodial parties.
//
// It runs as a background goroutine and stops when ctx is canceled.
// On restart, the indexer returns only PENDING offers (accepted ones are filtered out),
// so double-accept attempts are handled by Canton which the worker logs and skips.
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
	users, err := w.userLister.ListUsers(ctx)
	if err != nil {
		w.logger.Warn("accept worker: failed to list users", zap.Error(err))
		return
	}

	for _, u := range users {
		if u.KeyMode != user.KeyModeCustodial {
			continue
		}
		w.acceptForParty(ctx, u.CantonPartyID)
	}
}

func (w *AcceptWorker) acceptForParty(ctx context.Context, partyID string) {
	page := 1
	for {
		result, err := w.indexer.GetPendingOffersForParty(ctx, partyID, indexer.Pagination{
			Page:  page,
			Limit: acceptWorkerPageLimit,
		})
		if err != nil {
			w.logger.Warn("accept worker: failed to fetch pending offers",
				zap.String("party_id", partyID),
				zap.Error(err),
			)
			return
		}

		for i := range result.Items {
			offer := result.Items[i]
			if err := w.cantonToken.AcceptTransferInstruction(ctx, partyID, offer.ContractID, offer.InstrumentAdmin); err != nil {
				w.logger.Warn("accept worker: failed to accept offer",
					zap.String("party_id", partyID),
					zap.String("contract_id", offer.ContractID),
					zap.Error(err),
				)
				continue
			}
			w.logger.Info("accept worker: accepted transfer offer",
				zap.String("party_id", partyID),
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
