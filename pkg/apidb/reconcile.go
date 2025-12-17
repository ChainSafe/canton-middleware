package apidb

import (
	"context"
	"fmt"
	"strings"
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

// ReconcileAll synchronizes all user balances and total supply from Canton
func (r *Reconciler) ReconcileAll(ctx context.Context) error {
	r.logger.Info("Starting full reconciliation")
	start := time.Now()

	// Get all holdings from Canton
	holdings, err := r.cantonClient.GetAllCIP56Holdings(ctx)
	if err != nil {
		return fmt.Errorf("failed to get holdings from Canton: %w", err)
	}

	// Group holdings by owner party and sum balances
	partyBalances := make(map[string]decimal.Decimal)
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

		if current, ok := partyBalances[holding.Owner]; ok {
			partyBalances[holding.Owner] = current.Add(amount)
		} else {
			partyBalances[holding.Owner] = amount
		}
		totalSupply = totalSupply.Add(amount)
	}

	// Get all users from DB
	users, err := r.db.GetAllUsers()
	if err != nil {
		return fmt.Errorf("failed to get users from DB: %w", err)
	}

	// Update each user's balance
	var updated, mismatches int
	for _, user := range users {
		cantonBalance := decimal.Zero
		if bal, ok := partyBalances[user.CantonParty]; ok {
			cantonBalance = bal
		}

		dbBalance, err := decimal.NewFromString(user.Balance)
		if err != nil {
			r.logger.Warn("Failed to parse user balance", zap.String("evm_address", user.EVMAddress), zap.Error(err))
			continue
		}
		
		if !dbBalance.Equal(cantonBalance) {
			mismatches++
			r.logger.Info("Balance mismatch detected",
				zap.String("evm_address", user.EVMAddress),
				zap.String("db_balance", dbBalance.String()),
				zap.String("canton_balance", cantonBalance.String()))
		}

		// Always update to ensure consistency
		if err := r.db.UpdateUserBalance(user.EVMAddress, cantonBalance.String()); err != nil {
			r.logger.Error("Failed to update user balance",
				zap.String("evm_address", user.EVMAddress),
				zap.Error(err))
			continue
		}
		updated++
	}

	// Update total supply
	if err := r.db.SetTotalSupply(totalSupply.String()); err != nil {
		return fmt.Errorf("failed to update total supply: %w", err)
	}

	// Update reconciliation timestamp
	if err := r.db.UpdateLastReconciled(); err != nil {
		r.logger.Warn("Failed to update last reconciled timestamp", zap.Error(err))
	}

	r.logger.Info("Reconciliation completed",
		zap.Int("users_updated", updated),
		zap.Int("mismatches_found", mismatches),
		zap.Int("holdings_processed", len(holdings)),
		zap.String("total_supply", totalSupply.String()),
		zap.Duration("duration", time.Since(start)))

	return nil
}

// ReconcileUser synchronizes a single user's balance from Canton
func (r *Reconciler) ReconcileUser(ctx context.Context, fingerprint string) error {
	// Normalize fingerprint
	normalizedFingerprint := fingerprint
	if !strings.HasPrefix(normalizedFingerprint, "0x") {
		normalizedFingerprint = "0x" + normalizedFingerprint
	}

	r.logger.Debug("Reconciling user", zap.String("fingerprint", normalizedFingerprint))

	// Get balance from Canton
	balance, err := r.cantonClient.GetUserBalance(ctx, normalizedFingerprint)
	if err != nil {
		return fmt.Errorf("failed to get balance from Canton: %w", err)
	}

	// Update in DB
	if err := r.db.UpdateUserBalanceByFingerprint(normalizedFingerprint, balance); err != nil {
		return fmt.Errorf("failed to update balance in DB: %w", err)
	}

	r.logger.Debug("User balance reconciled",
		zap.String("fingerprint", normalizedFingerprint),
		zap.String("balance", balance))

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
