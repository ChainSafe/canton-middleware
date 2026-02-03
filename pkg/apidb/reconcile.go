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

// ReconcileAll synchronizes total supply and user balances from Canton.
// This method:
// 1. Calculates total supply from all CIP56Holding contracts
// 2. Updates registered users' balances based on their Canton party holdings
//
// User balances are reconciled from Canton holdings, which catches ALL balance changes
// including transfers made directly on the Canton ledger (e.g., by native Canton users).
func (r *Reconciler) ReconcileAll(ctx context.Context) error {
	r.logger.Info("Starting full reconciliation (supply + user balances)")
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

	// Also reconcile user balances from holdings
	if err := r.ReconcileUserBalancesFromHoldings(ctx); err != nil {
		r.logger.Error("Failed to reconcile user balances from holdings", zap.Error(err))
		// Don't return error - total supply reconciliation succeeded
	}

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

// =============================================================================
// Holdings-Based Balance Reconciliation
// =============================================================================

// ReconcileUserBalancesFromHoldings queries all CIP56Holding contracts from Canton
// and updates registered users' balances in the database. This catches ALL balance
// changes including transfers made directly on the Canton ledger.
func (r *Reconciler) ReconcileUserBalancesFromHoldings(ctx context.Context) error {
	r.logger.Info("Starting holdings-based balance reconciliation")
	start := time.Now()

	// Get all holdings from Canton
	holdings, err := r.cantonClient.GetAllCIP56Holdings(ctx)
	if err != nil {
		return fmt.Errorf("failed to get holdings from Canton: %w", err)
	}

	// Build a map of party -> token -> total balance
	// Structure: map[partyID]map[tokenSymbol]decimal.Decimal
	partyBalances := make(map[string]map[string]decimal.Decimal)

	for _, holding := range holdings {
		if holding.Owner == "" || holding.Amount == "" {
			continue
		}

		amount, err := decimal.NewFromString(holding.Amount)
		if err != nil {
			r.logger.Warn("Failed to parse holding amount",
				zap.String("owner", holding.Owner),
				zap.String("amount", holding.Amount),
				zap.Error(err))
			continue
		}

		// Determine token type from symbol (default to PROMPT if unknown)
		symbol := holding.Symbol
		if symbol == "" {
			symbol = "PROMPT" // Default for backward compatibility
		}

		// Initialize party's balance map if needed
		if _, ok := partyBalances[holding.Owner]; !ok {
			partyBalances[holding.Owner] = make(map[string]decimal.Decimal)
		}

		// Add to existing balance for this party and token
		current := partyBalances[holding.Owner][symbol]
		partyBalances[holding.Owner][symbol] = current.Add(amount)
	}

	// Get all registered users to update their balances
	users, err := r.db.GetAllUsers()
	if err != nil {
		return fmt.Errorf("failed to get users: %w", err)
	}

	var updatedCount int
	for _, user := range users {
		// Skip users without a Canton party ID
		if user.CantonPartyID == "" {
			continue
		}

		// Get this user's holdings
		userHoldings, hasHoldings := partyBalances[user.CantonPartyID]

		// Update DEMO balance
		demoBalance := decimal.Zero
		if hasHoldings {
			if bal, ok := userHoldings["DEMO"]; ok {
				demoBalance = bal
			}
		}
		if err := r.db.UpdateBalanceByCantonPartyID(user.CantonPartyID, demoBalance.String(), TokenDemo); err != nil {
			r.logger.Warn("Failed to update DEMO balance",
				zap.String("party_id", user.CantonPartyID),
				zap.Error(err))
		}

		// Update PROMPT balance
		promptBalance := decimal.Zero
		if hasHoldings {
			if bal, ok := userHoldings["PROMPT"]; ok {
				promptBalance = bal
			}
		}
		if err := r.db.UpdateBalanceByCantonPartyID(user.CantonPartyID, promptBalance.String(), TokenPrompt); err != nil {
			r.logger.Warn("Failed to update PROMPT balance",
				zap.String("party_id", user.CantonPartyID),
				zap.Error(err))
		}

		updatedCount++
		r.logger.Debug("Updated user balances from holdings",
			zap.String("party_id", user.CantonPartyID),
			zap.String("demo_balance", demoBalance.String()),
			zap.String("prompt_balance", promptBalance.String()))
	}

	r.logger.Info("Holdings-based balance reconciliation completed",
		zap.Int("holdings_processed", len(holdings)),
		zap.Int("parties_with_holdings", len(partyBalances)),
		zap.Int("users_updated", updatedCount),
		zap.Duration("duration", time.Since(start)))

	return nil
}

// =============================================================================
// Event-Based Reconciliation (using Bridge Audit Events)
// =============================================================================

// ReconcileFromBridgeEvents fetches all bridge events from Canton and reconciles
// user balances based on mint/burn events. This provides an authoritative
// source of truth from the Canton ledger for user balances.
// Note: Transfers are internal Canton operations and don't affect bridge reconciliation.
func (r *Reconciler) ReconcileFromBridgeEvents(ctx context.Context) error {
	r.logger.Info("Starting event-based balance reconciliation")
	start := time.Now()

	var mintCount, burnCount int

	// Get all bridge mint events
	mintEvents, err := r.cantonClient.GetBridgeMintEvents(ctx)
	if err != nil {
		return fmt.Errorf("failed to get mint events: %w", err)
	}

	// Process mint events
	for _, event := range mintEvents {
		// Check if already processed
		processed, err := r.db.IsEventProcessed(event.ContractID)
		if err != nil {
			r.logger.Warn("Failed to check if event processed", zap.String("contract_id", event.ContractID), zap.Error(err))
			continue
		}
		if processed {
			continue
		}

		if err := r.db.StoreBridgeMintEvent(event); err != nil {
			r.logger.Warn("Failed to store mint event",
				zap.String("contract_id", event.ContractID),
				zap.String("fingerprint", event.Fingerprint),
				zap.Error(err))
			continue
		}
		mintCount++
		r.logger.Debug("Processed mint event",
			zap.String("fingerprint", event.Fingerprint),
			zap.String("amount", event.Amount),
			zap.String("evm_tx_hash", event.EvmTxHash))
	}

	// Get all bridge burn events
	burnEvents, err := r.cantonClient.GetBridgeBurnEvents(ctx)
	if err != nil {
		return fmt.Errorf("failed to get burn events: %w", err)
	}

	// Process burn events
	for _, event := range burnEvents {
		processed, err := r.db.IsEventProcessed(event.ContractID)
		if err != nil {
			r.logger.Warn("Failed to check if event processed", zap.String("contract_id", event.ContractID), zap.Error(err))
			continue
		}
		if processed {
			continue
		}

		if err := r.db.StoreBridgeBurnEvent(event); err != nil {
			r.logger.Warn("Failed to store burn event",
				zap.String("contract_id", event.ContractID),
				zap.String("fingerprint", event.Fingerprint),
				zap.Error(err))
			continue
		}
		burnCount++
		r.logger.Debug("Processed burn event",
			zap.String("fingerprint", event.Fingerprint),
			zap.String("amount", event.Amount))
	}

	// Mark full reconcile complete
	if err := r.db.MarkFullReconcileComplete(); err != nil {
		r.logger.Warn("Failed to mark reconcile complete", zap.Error(err))
	}

	r.logger.Info("Event-based reconciliation completed",
		zap.Int("mint_events", mintCount),
		zap.Int("burn_events", burnCount),
		zap.Duration("duration", time.Since(start)))

	return nil
}

// FullBalanceReconciliation performs a complete balance reset and rebuild
// from bridge events. This should be used sparingly, e.g., on startup or
// when data inconsistencies are detected.
//
// This function:
// 1. Resets all user balances to 0
// 2. Clears the bridge_events table (to prevent double-counting)
// 3. Calls ReconcileFromBridgeEvents to rebuild both balances and event log
//
// Note: Only mint/burn events are considered since transfers are internal Canton operations.
func (r *Reconciler) FullBalanceReconciliation(ctx context.Context) error {
	r.logger.Info("Starting FULL balance reconciliation (resetting all balances and events)")
	start := time.Now()

	// Step 1: Reset all user balances to 0
	if err := r.db.ResetBalances(TokenPrompt); err != nil {
		return fmt.Errorf("failed to reset user balances: %w", err)
	}
	r.logger.Debug("Reset all user balances to 0")

	// Step 2: Clear the bridge_events table to prevent double-counting
	// when ReconcileFromBridgeEvents runs
	if err := r.db.ClearBridgeEvents(); err != nil {
		return fmt.Errorf("failed to clear bridge events: %w", err)
	}
	r.logger.Debug("Cleared bridge_events table")

	// Step 3: Run incremental reconciliation which will:
	// - Fetch all events from Canton
	// - Store them in bridge_events table
	// - Update user balances accordingly
	if err := r.ReconcileFromBridgeEvents(ctx); err != nil {
		return fmt.Errorf("failed to reconcile from bridge events: %w", err)
	}

	r.logger.Info("FULL balance reconciliation completed",
		zap.Duration("duration", time.Since(start)))

	return nil
}

// GetReconciliationStatus returns the current reconciliation state
func (r *Reconciler) GetReconciliationStatus(ctx context.Context) (*ReconciliationState, error) {
	return r.db.GetReconciliationState()
}
