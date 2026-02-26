package apidb

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
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
	LastProcessedOffset int64      `json:"last_processed_offset"`
	LastFullReconcileAt *time.Time `json:"last_full_reconcile_at,omitempty"`
	EventsProcessed     int        `json:"events_processed"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// =============================================================================
// Bridge Event Storage Methods
// =============================================================================

// bridgeEventParams holds the common parameters for storing bridge events
type bridgeEventParams struct {
	eventType      string
	contractID     string
	fingerprint    string
	amount         string
	tokenSymbol    string
	timestamp      time.Time
	evmTxHash      string // Used for mint events
	evmDestination string // Used for burn events
	isCredit       bool   // true = increment balance, false = decrement
}

// storeBridgeEvent is a helper that handles the common logic for storing bridge events
func (s *Store) storeBridgeEvent(params bridgeEventParams) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check if event already processed
	var exists bool
	err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM bridge_events WHERE contract_id = $1)`, params.contractID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check event existence: %w", err)
	}
	if exists {
		return nil // Already processed, skip
	}

	// Store the event
	var cantonTimestamp *time.Time
	if !params.timestamp.IsZero() {
		cantonTimestamp = &params.timestamp
	}
	_, err = tx.Exec(`
		INSERT INTO bridge_events (event_type, contract_id, fingerprint, amount, evm_tx_hash, evm_destination, token_symbol, canton_timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, params.eventType, params.contractID, params.fingerprint, params.amount, nullIfEmpty(params.evmTxHash), nullIfEmpty(params.evmDestination), params.tokenSymbol, cantonTimestamp)
	if err != nil {
		return fmt.Errorf("failed to store %s event: %w", params.eventType, err)
	}

	// Update user PROMPT balance
	withPrefix, withoutPrefix := normalizeFingerprint(params.fingerprint)
	balanceOp := "-"
	if params.isCredit {
		balanceOp = "+"
	}
	_, err = tx.Exec(fmt.Sprintf(`
		UPDATE users
		SET prompt_balance = COALESCE(prompt_balance, 0) %s $1::DECIMAL, balance_updated_at = NOW()
		WHERE fingerprint = $2 OR fingerprint = $3
	`, balanceOp), params.amount, withPrefix, withoutPrefix)
	if err != nil {
		return fmt.Errorf("failed to update user prompt balance: %w", err)
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

// normalizeFingerprint returns both the 0x-prefixed and non-prefixed forms.
func normalizeFingerprint(fingerprint string) (withPrefix, withoutPrefix string) {
	fp := strings.TrimPrefix(fingerprint, "0x")
	return "0x" + fp, fp
}

// nullIfEmpty returns nil for empty strings (for SQL NULL handling)
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// StoreMintEvent stores a mint event and updates user balance
func (s *Store) StoreMintEvent(event *canton.MintEvent) error {
	return s.storeBridgeEvent(bridgeEventParams{
		eventType:   "mint",
		contractID:  event.ContractID,
		fingerprint: event.UserFingerprint,
		amount:      event.Amount,
		tokenSymbol: event.TokenSymbol,
		timestamp:   event.Timestamp,
		evmTxHash:   event.EvmTxHash,
		isCredit:    true,
	})
}

// StoreBurnEvent stores a burn event and updates user balance
func (s *Store) StoreBurnEvent(event *canton.BurnEvent) error {
	return s.storeBridgeEvent(bridgeEventParams{
		eventType:      "burn",
		contractID:     event.ContractID,
		fingerprint:    event.UserFingerprint,
		amount:         event.Amount,
		tokenSymbol:    event.TokenSymbol,
		timestamp:      event.Timestamp,
		evmDestination: event.EvmDestination,
		isCredit:       false,
	})
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

// ClearBridgeEvents removes all entries from the bridge_events table
// This should be called before a full reconciliation to ensure consistency
func (s *Store) ClearBridgeEvents() error {
	_, err := s.db.Exec(`DELETE FROM bridge_events`)
	if err != nil {
		return fmt.Errorf("failed to clear bridge_events: %w", err)
	}
	// Reset the events processed counter
	_, err = s.db.Exec(`
		UPDATE reconciliation_state 
		SET events_processed = 0, updated_at = NOW()
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
