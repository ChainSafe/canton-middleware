// SPDX-License-Identifier: Apache-2.0

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
	metrics      *Metrics
	logger       *zap.Logger
}

// NewAcceptWorker creates a new AcceptWorker.
//
// metrics receives Prometheus observations for cycle duration, per-phase
// errors, and per-offer accept outcomes. Pass NewNopMetrics() in tests where
// metric values aren't asserted.
func NewAcceptWorker(
	cantonToken cantontkn.Token,
	userLister UserLister,
	indexerClient indexerclient.Client,
	pollInterval time.Duration,
	metrics *Metrics,
	logger *zap.Logger,
) *AcceptWorker {
	if metrics == nil {
		metrics = NewNopMetrics()
	}
	return &AcceptWorker{
		cantonToken:  cantonToken,
		userLister:   userLister,
		indexer:      indexerClient,
		pollInterval: pollInterval,
		metrics:      metrics,
		logger:       logger,
	}
}

// Run starts the accept worker loop. It blocks until ctx is canceled.
func (w *AcceptWorker) Run(ctx context.Context) error {
	w.logger.Info("accept worker started", zap.Duration("poll_interval", w.pollInterval))
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("accept worker stopped")
			return nil
		case <-ticker.C:
			w.acceptPending(ctx)
		}
	}
}

func (w *AcceptWorker) acceptPending(ctx context.Context) {
	w.metrics.RunsTotal.Inc()
	start := time.Now()
	defer func() {
		w.metrics.RunDuration.Observe(time.Since(start).Seconds())
	}()

	users, err := w.userLister.ListCustodialUsers(ctx)
	if err != nil {
		w.metrics.ErrorsTotal.WithLabelValues("list_users").Inc()
		w.logger.Warn("accept worker: failed to list custodial users", zap.Error(err))
		return
	}
	w.metrics.CustodialUsers.Set(float64(len(users)))
	if len(users) == 0 {
		// Vacuously successful — there's nothing to accept, but the loop is alive.
		w.metrics.LastSuccessfulRunTimestamp.SetToCurrentTime()
		return
	}

	custodialParties := make(map[string]bool, len(users))
	for _, u := range users {
		custodialParties[u.CantonPartyID] = true
	}

	page := 1
	for {
		result, err := w.indexer.GetPendingTransfers(ctx, indexer.Pagination{
			Page:  page,
			Limit: acceptWorkerPageLimit,
		})
		if err != nil {
			w.metrics.ErrorsTotal.WithLabelValues("fetch_offers").Inc()
			w.logger.Warn("accept worker: failed to fetch pending transfers", zap.Error(err))
			return
		}

		// Update queue-depth gauge once per cycle (on the first successful page).
		// result.Total is the indexer-wide pending count, identical across pages
		// of the same cycle — so writing it every page is redundant but harmless.
		if page == 1 {
			w.metrics.PendingOffers.Set(float64(result.Total))
		}
		w.metrics.OffersFetchedTotal.Add(float64(len(result.Items)))

		for i := range result.Items {
			transfer := result.Items[i]
			// ToPartyID is the offer receiver — the custodial party that must accept.
			if !custodialParties[transfer.ToPartyID] {
				continue
			}
			acceptStart := time.Now()
			err := w.cantonToken.AcceptTransferInstruction(
				ctx, transfer.ToPartyID, transfer.ContractID, transfer.InstrumentAdmin,
			)
			w.metrics.OfferAcceptDuration.Observe(time.Since(acceptStart).Seconds())
			if err != nil {
				w.metrics.OffersAcceptedTotal.WithLabelValues("error").Inc()
				w.logger.Warn("accept worker: failed to accept offer",
					zap.String("party_id", transfer.ToPartyID),
					zap.String("contract_id", transfer.ContractID),
					zap.Error(err),
				)
				continue
			}
			w.metrics.OffersAcceptedTotal.WithLabelValues("success").Inc()
			w.logger.Info("accept worker: accepted transfer offer",
				zap.String("party_id", transfer.ToPartyID),
				zap.String("contract_id", transfer.ContractID),
				zap.String("sender", transfer.FromPartyID),
				zap.String("amount", transfer.Amount),
			)
		}

		if int64(page*acceptWorkerPageLimit) >= result.Total {
			// Reached the end of the pending-offers stream — cycle completed
			// without an unrecoverable error. Per-offer failures don't count
			// against the cycle's success: those are tracked in
			// OffersAcceptedTotal{result="error"}.
			w.metrics.LastSuccessfulRunTimestamp.SetToCurrentTime()
			return
		}
		page++
	}
}
