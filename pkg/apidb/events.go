package apidb

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/canton"
)

// BridgeEvent represents a stored bridge event for reconciliation
type BridgeEvent struct {
	ID                   int64      `json:"id"`
	EventType            string     `json:"event_type"`
	ContractID           string     `json:"contract_id"`
	Fingerprint          string     `json:"fingerprint"`
	RecipientFingerprint string     `json:"recipient_fingerprint,omitempty"`
	Amount               string     `json:"amount"`
	EvmTxHash            string     `json:"evm_tx_hash,omitempty"`
	EvmDestination       string     `json:"evm_destination,omitempty"`
	TokenSymbol          string     `json:"token_symbol,omitempty"`
	CantonTimestamp      *time.Time `json:"canton_timestamp,omitempty"`
	ProcessedAt          time.Time  `json:"processed_at"`
}

// ReconciliationState tracks reconciliation progress
type ReconciliationState struct {
	LastProcessedOffset  int64      `json:"last_processed_offset"`
	LastFullReconcileAt  *time.Time `json:"last_full_reconcile_at,omitempty"`
	EventsProcessed      int        `json:"events_processed"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// =============================================================================
// Bridge Event Storage Methods
// =============================================================================

// StoreBridgeMintEvent stores a mint event and updates user balance
func (s *Store) StoreBridgeMintEvent(event *canton.BridgeMintEvent) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check if event already processed
	var exists bool
	err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM bridge_events WHERE contract_id = $1)`, event.ContractID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check event existence: %w", err)
	}
	if exists {
		return nil // Already processed, skip
	}

	// Store the event
	var cantonTimestamp *time.Time
	if !event.Timestamp.IsZero() {
		cantonTimestamp = &event.Timestamp
	}
	_, err = tx.Exec(`
		INSERT INTO bridge_events (event_type, contract_id, fingerprint, amount, evm_tx_hash, token_symbol, canton_timestamp)
		VALUES ('mint', $1, $2, $3, $4, $5, $6)
	`, event.ContractID, event.Fingerprint, event.Amount, event.EvmTxHash, event.TokenSymbol, cantonTimestamp)
	if err != nil {
		return fmt.Errorf("failed to store mint event: %w", err)
	}

	// Update user balance (increment)
	withPrefix, withoutPrefix := normalizeFingerprint(event.Fingerprint)
	_, err = tx.Exec(`
		UPDATE users
		SET balance = COALESCE(balance, 0) + $1::DECIMAL, balance_updated_at = NOW()
		WHERE fingerprint = $2 OR fingerprint = $3
	`, event.Amount, withPrefix, withoutPrefix)
	if err != nil {
		return fmt.Errorf("failed to update user balance: %w", err)
	}

	// Increment events processed counter
	_, err = tx.Exec(`
		UPDATE reconciliation_state 
		SET events_processed = events_processed + 1, updated_at = NOW()
		WHERE id = 1
	`)
	if err != nil {
		return fmt.Errorf("failed to update reconciliation state: %w", err)
	}

	return tx.Commit()
}

// StoreBridgeBurnEvent stores a burn event and updates user balance
func (s *Store) StoreBridgeBurnEvent(event *canton.BridgeBurnEvent) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check if event already processed
	var exists bool
	err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM bridge_events WHERE contract_id = $1)`, event.ContractID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check event existence: %w", err)
	}
	if exists {
		return nil // Already processed, skip
	}

	// Store the event
	var cantonTimestamp *time.Time
	if !event.Timestamp.IsZero() {
		cantonTimestamp = &event.Timestamp
	}
	_, err = tx.Exec(`
		INSERT INTO bridge_events (event_type, contract_id, fingerprint, amount, evm_destination, token_symbol, canton_timestamp)
		VALUES ('burn', $1, $2, $3, $4, $5, $6)
	`, event.ContractID, event.Fingerprint, event.Amount, event.EvmDestination, event.TokenSymbol, cantonTimestamp)
	if err != nil {
		return fmt.Errorf("failed to store burn event: %w", err)
	}

	// Update user balance (decrement)
	withPrefix, withoutPrefix := normalizeFingerprint(event.Fingerprint)
	_, err = tx.Exec(`
		UPDATE users
		SET balance = COALESCE(balance, 0) - $1::DECIMAL, balance_updated_at = NOW()
		WHERE fingerprint = $2 OR fingerprint = $3
	`, event.Amount, withPrefix, withoutPrefix)
	if err != nil {
		return fmt.Errorf("failed to update user balance: %w", err)
	}

	// Increment events processed counter
	_, err = tx.Exec(`
		UPDATE reconciliation_state 
		SET events_processed = events_processed + 1, updated_at = NOW()
		WHERE id = 1
	`)
	if err != nil {
		return fmt.Errorf("failed to update reconciliation state: %w", err)
	}

	return tx.Commit()
}

// =============================================================================
// Reconciliation State Methods
// =============================================================================

// GetReconciliationState returns the current reconciliation state
func (s *Store) GetReconciliationState() (*ReconciliationState, error) {
	state := &ReconciliationState{}
	var lastFullReconcileAt sql.NullTime

	err := s.db.QueryRow(`
		SELECT last_processed_offset, last_full_reconcile_at, events_processed, updated_at
		FROM reconciliation_state
		WHERE id = 1
	`).Scan(&state.LastProcessedOffset, &lastFullReconcileAt, &state.EventsProcessed, &state.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to get reconciliation state: %w", err)
	}

	if lastFullReconcileAt.Valid {
		state.LastFullReconcileAt = &lastFullReconcileAt.Time
	}

	return state, nil
}

// UpdateReconciliationOffset updates the last processed offset
func (s *Store) UpdateReconciliationOffset(offset int64) error {
	_, err := s.db.Exec(`
		UPDATE reconciliation_state
		SET last_processed_offset = $1, updated_at = NOW()
		WHERE id = 1
	`, offset)
	return err
}

// MarkFullReconcileComplete marks that a full reconciliation was completed
func (s *Store) MarkFullReconcileComplete() error {
	_, err := s.db.Exec(`
		UPDATE reconciliation_state
		SET last_full_reconcile_at = NOW(), updated_at = NOW()
		WHERE id = 1
	`)
	return err
}

// =============================================================================
// Event Query Methods
// =============================================================================

// IsEventProcessed checks if a bridge event has already been processed
func (s *Store) IsEventProcessed(contractID string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM bridge_events WHERE contract_id = $1)`, contractID).Scan(&exists)
	return exists, err
}

// GetRecentBridgeEvents returns the most recent bridge events
func (s *Store) GetRecentBridgeEvents(limit int) ([]*BridgeEvent, error) {
	query := `
		SELECT id, event_type, contract_id, fingerprint, recipient_fingerprint, amount, 
		       evm_tx_hash, evm_destination, token_symbol, canton_timestamp, processed_at
		FROM bridge_events
		ORDER BY id DESC
		LIMIT $1
	`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query bridge events: %w", err)
	}
	defer rows.Close()

	var events []*BridgeEvent
	for rows.Next() {
		event := &BridgeEvent{}
		var fingerprint, recipientFingerprint, evmTxHash, evmDestination, tokenSymbol sql.NullString
		var cantonTimestamp sql.NullTime

		err := rows.Scan(
			&event.ID,
			&event.EventType,
			&event.ContractID,
			&fingerprint,
			&recipientFingerprint,
			&event.Amount,
			&evmTxHash,
			&evmDestination,
			&tokenSymbol,
			&cantonTimestamp,
			&event.ProcessedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}

		if fingerprint.Valid {
			event.Fingerprint = fingerprint.String
		}
		if recipientFingerprint.Valid {
			event.RecipientFingerprint = recipientFingerprint.String
		}
		if evmTxHash.Valid {
			event.EvmTxHash = evmTxHash.String
		}
		if evmDestination.Valid {
			event.EvmDestination = evmDestination.String
		}
		if tokenSymbol.Valid {
			event.TokenSymbol = tokenSymbol.String
		}
		if cantonTimestamp.Valid {
			event.CantonTimestamp = &cantonTimestamp.Time
		}

		events = append(events, event)
	}

	return events, nil
}

// GetEventCountByType returns the count of events by type
func (s *Store) GetEventCountByType() (map[string]int, error) {
	query := `SELECT event_type, COUNT(*) FROM bridge_events GROUP BY event_type`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query event counts: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var eventType string
		var count int
		if err := rows.Scan(&eventType, &count); err != nil {
			return nil, fmt.Errorf("failed to scan event count: %w", err)
		}
		counts[eventType] = count
	}

	return counts, nil
}

// ResetUserBalances resets all user balances to 0 (used before full reconciliation)
func (s *Store) ResetUserBalances() error {
	_, err := s.db.Exec(`UPDATE users SET balance = 0, balance_updated_at = NOW()`)
	return err
}

