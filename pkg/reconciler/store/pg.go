package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"

	"github.com/uptrace/bun"
)

const defaultTokenSymbol = "PROMPT"

// PGStore is a PostgreSQL-backed reconciler store.
type PGStore struct {
	db *bun.DB
}

// NewStore creates a new PostgreSQL-backed reconciler store.
func NewStore(db *bun.DB) *PGStore {
	return &PGStore{db: db}
}

// SetBalanceByCantonPartyID sets an absolute balance for a user/token pair.
// It upserts based on (canton_party_id, token_symbol) in user_token_balances.
func (s *PGStore) SetBalanceByCantonPartyID(ctx context.Context, partyID, tokenSymbol, balance string) error {
	symbol, err := normalizeTokenSymbol(tokenSymbol)
	if err != nil {
		return err
	}
	partyID = strings.TrimSpace(partyID)
	if partyID == "" {
		return fmt.Errorf("party id is required")
	}

	row := &UserTokenBalanceDao{
		CantonPartyID: &partyID,
		TokenSymbol:   symbol,
		Balance:       balance,
	}

	_, err = s.db.NewInsert().
		Model(row).
		On("CONFLICT (canton_party_id, token_symbol) DO UPDATE").
		Set("balance = EXCLUDED.balance").
		Set("updated_at = NOW()").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("set balance for party %s token %s: %w", partyID, symbol, err)
	}

	return nil
}

// IncrementBalanceByFingerprint increments a user's token balance by amount.
func (s *PGStore) IncrementBalanceByFingerprint(ctx context.Context, fingerprint, amount, tokenSymbol string) error {
	symbol, err := normalizeTokenSymbol(tokenSymbol)
	if err != nil {
		return err
	}

	withPrefix, _ := normalizeFingerprint(fingerprint)
	if withPrefix == "" {
		return fmt.Errorf("fingerprint is required")
	}

	row := &UserTokenBalanceDao{
		Fingerprint: &withPrefix,
		TokenSymbol: symbol,
		Balance:     amount,
	}

	_, err = s.db.NewInsert().
		Model(row).
		On("CONFLICT (fingerprint, token_symbol) DO UPDATE").
		Set("balance = COALESCE(utb.balance, 0) + EXCLUDED.balance").
		Set("updated_at = NOW()").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("increment balance for fingerprint %s token %s: %w", fingerprint, symbol, err)
	}

	return nil
}

// DecrementBalanceByEVMAddress decrements a user's token balance by amount.
func (s *PGStore) DecrementBalanceByEVMAddress(ctx context.Context, evmAddress, amount, tokenSymbol string) error {
	symbol, err := normalizeTokenSymbol(tokenSymbol)
	if err != nil {
		return err
	}
	evmAddress = strings.TrimSpace(evmAddress)
	if evmAddress == "" {
		return fmt.Errorf("evm address is required")
	}

	row := &UserTokenBalanceDao{
		EVMAddress:  &evmAddress,
		TokenSymbol: symbol,
		Balance:     amount,
	}

	_, err = s.db.NewInsert().
		Model(row).
		On("CONFLICT (evm_address, token_symbol) DO UPDATE").
		Set("balance = COALESCE(utb.balance, 0) - EXCLUDED.balance").
		Set("updated_at = NOW()").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("decrement balance for evm address %s token %s: %w", evmAddress, symbol, err)
	}

	return nil
}

// ResetBalancesByTokenSymbol resets all balances for the given token symbol to zero.
func (s *PGStore) ResetBalancesByTokenSymbol(ctx context.Context, tokenSymbol string) error {
	symbol, err := normalizeTokenSymbol(tokenSymbol)
	if err != nil {
		return err
	}

	_, err = s.db.NewUpdate().
		Model((*UserTokenBalanceDao)(nil)).
		Set("balance = '0'").
		Set("updated_at = NOW()").
		Where("token_symbol = ?", symbol).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("reset balances for token %s: %w", symbol, err)
	}

	return nil
}

// GetBalanceByFingerprint returns the stored user balance for token and fingerprint.
func (s *PGStore) GetBalanceByFingerprint(ctx context.Context, fingerprint, tokenSymbol string) (string, error) {
	symbol, err := normalizeTokenSymbol(tokenSymbol)
	if err != nil {
		return "0", err
	}
	withPrefix, withoutPrefix := normalizeFingerprint(fingerprint)
	if withPrefix == "" {
		return "0", fmt.Errorf("fingerprint is required")
	}

	row := new(UserTokenBalanceDao)
	err = s.db.NewSelect().
		Model(row).
		Column("balance").
		Where("token_symbol = ?", symbol).
		Where("(fingerprint = ? OR fingerprint = ?)", withPrefix, withoutPrefix).
		Order("updated_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "0", nil
		}
		return "0", fmt.Errorf("get balance by fingerprint %s token %s: %w", fingerprint, symbol, err)
	}

	return row.Balance, nil
}

// GetBalanceByEVMAddress returns the stored user balance for token and EVM address.
func (s *PGStore) GetBalanceByEVMAddress(ctx context.Context, evmAddress, tokenSymbol string) (string, error) {
	symbol, err := normalizeTokenSymbol(tokenSymbol)
	if err != nil {
		return "0", err
	}
	evmAddress = strings.TrimSpace(evmAddress)
	if evmAddress == "" {
		return "0", fmt.Errorf("evm address is required")
	}

	row := new(UserTokenBalanceDao)
	err = s.db.NewSelect().
		Model(row).
		Column("balance").
		Where("token_symbol = ?", symbol).
		Where("evm_address = ?", evmAddress).
		Order("updated_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "0", nil
		}
		return "0", fmt.Errorf("get balance by evm address %s token %s: %w", evmAddress, symbol, err)
	}

	return row.Balance, nil
}

// SetTotalSupply sets the total supply for a token.
func (s *PGStore) SetTotalSupply(ctx context.Context, tokenSymbol, value string) error {
	symbol, err := normalizeTokenSymbol(tokenSymbol)
	if err != nil {
		return err
	}

	_, err = s.db.NewInsert().
		Model(&TokenMetricsDao{TokenSymbol: symbol, TotalSupply: value}).
		On("CONFLICT (token_symbol) DO UPDATE").
		Set("total_supply = EXCLUDED.total_supply").
		Set("updated_at = NOW()").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("set total supply for %s: %w", symbol, err)
	}

	return nil
}

// UpdateLastReconciled updates the last reconciled timestamp for a token.
func (s *PGStore) UpdateLastReconciled(ctx context.Context, tokenSymbol string) error {
	symbol, err := normalizeTokenSymbol(tokenSymbol)
	if err != nil {
		return err
	}

	_, err = s.db.NewUpdate().
		Model((*TokenMetricsDao)(nil)).
		Set("last_reconciled_at = NOW()").
		Set("updated_at = NOW()").
		Where("token_symbol = ?", symbol).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("update last reconciled for %s: %w", symbol, err)
	}

	return nil
}

// IsEventProcessed checks if a bridge event was already processed.
func (s *PGStore) IsEventProcessed(ctx context.Context, contractID string) (bool, error) {
	exists, err := s.db.NewSelect().
		Model((*BridgeEventDao)(nil)).
		Where("contract_id = ?", contractID).
		Exists(ctx)
	if err != nil {
		return false, fmt.Errorf("check processed event %s: %w", contractID, err)
	}
	return exists, nil
}

// StoreMintEvent stores a mint event and updates user balances atomically.
func (s *PGStore) StoreMintEvent(ctx context.Context, event *canton.MintEvent) error {
	if event == nil {
		return fmt.Errorf("mint event is required")
	}

	return s.storeBridgeEvent(ctx, &bridgeEventParams{
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

// StoreBurnEvent stores a burn event and updates user balances atomically.
func (s *PGStore) StoreBurnEvent(ctx context.Context, event *canton.BurnEvent) error {
	if event == nil {
		return fmt.Errorf("burn event is required")
	}

	return s.storeBridgeEvent(ctx, &bridgeEventParams{
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

// MarkFullReconcileComplete marks reconciliation completion timestamp.
func (s *PGStore) MarkFullReconcileComplete(ctx context.Context) error {
	_, err := s.db.NewUpdate().
		Model((*ReconciliationStateDao)(nil)).
		Set("last_full_reconcile_at = NOW()").
		Set("updated_at = NOW()").
		Where("id = 1").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("mark full reconcile complete: %w", err)
	}
	return nil
}

// ClearBridgeEvents clears all bridge events and resets processed counter.
func (s *PGStore) ClearBridgeEvents(ctx context.Context) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewDelete().Model((*BridgeEventDao)(nil)).Where("1 = 1").Exec(ctx); err != nil {
			return fmt.Errorf("clear bridge events: %w", err)
		}

		if _, err := tx.NewUpdate().
			Model((*ReconciliationStateDao)(nil)).
			Set("events_processed = 0").
			Set("updated_at = NOW()").
			Where("id = 1").
			Exec(ctx); err != nil {
			return fmt.Errorf("reset reconciliation state counter: %w", err)
		}

		return nil
	})
}

// GetReconciliationState returns the current reconciliation status.
func (s *PGStore) GetReconciliationState(ctx context.Context) (*ReconciliationState, error) {
	dao := new(ReconciliationStateDao)
	err := s.db.NewSelect().
		Model(dao).
		Where("id = 1").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("get reconciliation state: %w", err)
	}

	return &ReconciliationState{
		LastProcessedOffset: dao.LastProcessedOffset,
		LastFullReconcileAt: dao.LastFullReconcileAt,
		EventsProcessed:     dao.EventsProcessed,
		UpdatedAt:           dao.UpdatedAt,
	}, nil
}

// UpdateReconciliationOffset updates the reconciliation offset cursor.
func (s *PGStore) UpdateReconciliationOffset(ctx context.Context, offset int64) error {
	_, err := s.db.NewUpdate().
		Model((*ReconciliationStateDao)(nil)).
		Set("last_processed_offset = ?", offset).
		Set("updated_at = NOW()").
		Where("id = 1").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("update reconciliation offset: %w", err)
	}
	return nil
}

type bridgeEventParams struct {
	eventType      string
	contractID     string
	fingerprint    string
	amount         string
	tokenSymbol    string
	timestamp      time.Time
	evmTxHash      string
	evmDestination string
	isCredit       bool
}

func (s *PGStore) storeBridgeEvent(ctx context.Context, params *bridgeEventParams) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		event := &BridgeEventDao{
			EventType:       params.eventType,
			ContractID:      params.contractID,
			Fingerprint:     stringPtrOrNil(params.fingerprint),
			Amount:          params.amount,
			EvmTxHash:       stringPtrOrNil(params.evmTxHash),
			EvmDestination:  stringPtrOrNil(params.evmDestination),
			TokenSymbol:     stringPtrOrNil(params.tokenSymbol),
			CantonTimestamp: timePtrOrNil(params.timestamp),
		}
		result, err := tx.NewInsert().
			Model(event).
			On("CONFLICT (contract_id) DO NOTHING").
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("store %s event: %w", params.eventType, err)
		}
		rowsAffected, rowsErr := result.RowsAffected()
		if rowsErr == nil && rowsAffected == 0 {
			// Already processed.
			return nil
		}

		tokenSymbol := params.tokenSymbol
		if strings.TrimSpace(tokenSymbol) == "" {
			tokenSymbol = defaultTokenSymbol
		}
		symbol, err := normalizeTokenSymbol(tokenSymbol)
		if err != nil {
			return err
		}

		op := "-"
		if params.isCredit {
			op = "+"
		}

		withPrefix, _ := normalizeFingerprint(params.fingerprint)
		if withPrefix == "" {
			return fmt.Errorf("event fingerprint is required")
		}

		balanceExpr := "COALESCE(utb.balance, 0) + EXCLUDED.balance"
		if op == "-" {
			balanceExpr = "COALESCE(utb.balance, 0) - EXCLUDED.balance"
		}

		row := &UserTokenBalanceDao{
			Fingerprint: &withPrefix,
			TokenSymbol: symbol,
			Balance:     params.amount,
		}
		if _, err = tx.NewInsert().
			Model(row).
			On("CONFLICT (fingerprint, token_symbol) DO UPDATE").
			Set("balance = " + balanceExpr).
			Set("updated_at = NOW()").
			Exec(ctx); err != nil {
			return fmt.Errorf("update user token balance: %w", err)
		}

		if _, err = tx.NewUpdate().
			Model((*ReconciliationStateDao)(nil)).
			Set("events_processed = COALESCE(events_processed, 0) + 1").
			Set("updated_at = NOW()").
			Where("id = 1").
			Exec(ctx); err != nil {
			return fmt.Errorf("update reconciliation state: %w", err)
		}

		return nil
	})
}

func normalizeTokenSymbol(tokenSymbol string) (string, error) {
	s := strings.ToUpper(strings.TrimSpace(tokenSymbol))
	if s == "" {
		return "", fmt.Errorf("token symbol is required")
	}
	return s, nil
}

func normalizeFingerprint(fingerprint string) (withPrefix, withoutPrefix string) {
	fp := strings.TrimSpace(fingerprint)
	fp = strings.TrimPrefix(fp, "0x")
	if fp == "" {
		return "", ""
	}
	return "0x" + fp, fp
}

func stringPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func timePtrOrNil(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
