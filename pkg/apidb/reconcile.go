package apidb

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/canton"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// Reconciler handles synchronization between Canton ledger state and DB cache
type Reconciler struct {
	db           *Store
	cantonClient *canton.Client
	logger       *zap.Logger

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewReconciler creates a new reconciler
func NewReconciler(db *Store, cantonClient *canton.Client, logger *zap.Logger) *Reconciler {
	return &Reconciler{
		db:           db,
		cantonClient: cantonClient,
		logger:       logger,
		stopCh:       make(chan struct{}),
	}
}

// ReconcileAll synchronizes total supply from Canton.
// NOTE: User balances are NOT reconciled from Canton because in the issuer-centric model,
// all holdings are owned by the same party (issuer) and individual user balances cannot
// be determined from on-chain data. User balances are tracked via transaction history
// (deposits, withdrawals, transfers) which is the source of truth.
func (r *Reconciler) ReconcileAll(ctx context.Context) error {
	r.logger.Info("Starting total supply reconciliation")
	start := time.Now()

	// Get all holdings from Canton to calculate total supply
	holdings, err := r.cantonClient.GetAllCIP56Holdings(ctx)
	if err != nil {
		return fmt.Errorf("failed to get holdings from Canton: %w", err)
	}

	// Calculate total supply from all holdings
	totalSupply := decimal.Zero
	for _, holding := range holdings {
		amount, err := decimal.NewFromString(holding.Amount)
		if err != nil {
			r.logger.Warn("Failed to parse holding amount",
				zap.String("owner", holding.Owner),
				zap.String("amount", holding.Amount),
				zap.Error(err))
			continue
		}
		totalSupply = totalSupply.Add(amount)
	}

	// Update total supply from Canton (this is authoritative)
	if err := r.db.SetTotalSupply(totalSupply.String()); err != nil {
		return fmt.Errorf("failed to update total supply: %w", err)
	}

	// Update reconciliation timestamp
	if err := r.db.UpdateLastReconciled(); err != nil {
		r.logger.Warn("Failed to update last reconciled timestamp", zap.Error(err))
	}

	r.logger.Info("Total supply reconciliation completed",
		zap.Int("holdings_processed", len(holdings)),
		zap.String("total_supply", totalSupply.String()),
		zap.Duration("duration", time.Since(start)))

	return nil
}

// StartPeriodicReconciliation starts a background goroutine that reconciles periodically
func (r *Reconciler) StartPeriodicReconciliation(interval time.Duration) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		r.logger.Info("Started periodic reconciliation", zap.Duration("interval", interval))

		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				if err := r.ReconcileAll(ctx); err != nil {
					r.logger.Error("Periodic reconciliation failed", zap.Error(err))
				}
				cancel()
			case <-r.stopCh:
				r.logger.Info("Stopping periodic reconciliation")
				return
			}
		}
	}()
}

// Stop stops the periodic reconciliation
func (r *Reconciler) Stop() {
	close(r.stopCh)
	r.wg.Wait()
}
