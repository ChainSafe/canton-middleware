// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/chainsafe/canton-middleware/pkg/relayer"
)

// PGStore is a PostgreSQL-backed store for the relayer, backed by Bun ORM.
type PGStore struct {
	db *bun.DB
}

// NewStore creates a new Bun-backed relayer store.
func NewStore(db *bun.DB) *PGStore {
	return &PGStore{db: db}
}

// CreateTransfer inserts a new transfer record. Returns true if newly inserted,
// false if it already existed (ON CONFLICT DO NOTHING).
func (s *PGStore) CreateTransfer(ctx context.Context, t *relayer.Transfer) (bool, error) {
	dao := toTransferDao(t)
	result, err := s.db.NewInsert().
		Model(dao).
		On("CONFLICT (id) DO NOTHING").
		Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("create transfer: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("create transfer rows affected: %w", err)
	}
	return n > 0, nil
}

// GetTransfer retrieves a transfer by ID. Returns nil, nil when not found.
func (s *PGStore) GetTransfer(ctx context.Context, id string) (*relayer.Transfer, error) {
	dao := new(TransferDao)
	err := s.db.NewSelect().
		Model(dao).
		Where("id = ?", id).
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get transfer: %w", err)
	}
	return fromTransferDao(dao), nil
}

// UpdateTransferStatus updates the status, optional destination tx hash, and optional error message.
func (s *PGStore) UpdateTransferStatus(
	ctx context.Context,
	id string,
	status relayer.TransferStatus,
	destTxHash *string,
	errMsg *string,
) error {
	now := time.Now()
	q := s.db.NewUpdate().
		Model((*TransferDao)(nil)).
		Set("status = ?", status).
		Set("destination_tx_hash = ?", destTxHash).
		Set("error_message = ?", errMsg).
		Set("updated_at = ?", now).
		Where("id = ?", id)

	if status == relayer.TransferStatusCompleted {
		q = q.Set("completed_at = ?", now)
	}

	result, err := q.Exec(ctx)
	if err != nil {
		return fmt.Errorf("update transfer status: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update transfer status rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("transfer %s not found", id)
	}
	return nil
}

// IncrementRetryCount atomically increments the retry count for a transfer.
func (s *PGStore) IncrementRetryCount(ctx context.Context, id string) error {
	result, err := s.db.NewUpdate().
		Model((*TransferDao)(nil)).
		Set("retry_count = retry_count + 1").
		Set("updated_at = ?", time.Now()).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("increment retry count: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("increment retry count rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("transfer %s not found", id)
	}
	return nil
}

// GetPendingTransfers returns all pending transfers for a given direction.
func (s *PGStore) GetPendingTransfers(
	ctx context.Context,
	direction relayer.TransferDirection,
) ([]*relayer.Transfer, error) {
	var daos []TransferDao
	err := s.db.NewSelect().
		Model(&daos).
		Where("direction = ?", direction).
		Where("status = ?", relayer.TransferStatusPending).
		OrderExpr("created_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("get pending transfers: %w", err)
	}

	transfers := make([]*relayer.Transfer, 0, len(daos))
	for i := range daos {
		transfers = append(transfers, fromTransferDao(&daos[i]))
	}
	return transfers, nil
}

// ListTransfers returns the most recently created transfers up to limit.
func (s *PGStore) ListTransfers(ctx context.Context, limit int) ([]*relayer.Transfer, error) {
	var daos []TransferDao
	err := s.db.NewSelect().
		Model(&daos).
		OrderExpr("created_at DESC").
		Limit(limit).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("list transfers: %w", err)
	}

	transfers := make([]*relayer.Transfer, 0, len(daos))
	for i := range daos {
		transfers = append(transfers, fromTransferDao(&daos[i]))
	}
	return transfers, nil
}

// GetSteppableTransfers returns non-terminal transfers owned by the given
// bridge keys that are due for a step (next_step_at unset or elapsed),
// oldest first.
func (s *PGStore) GetSteppableTransfers(
	ctx context.Context,
	bridgeKeys []string,
	limit int,
) ([]*relayer.Transfer, error) {
	if len(bridgeKeys) == 0 {
		return nil, nil
	}

	var daos []TransferDao
	err := s.db.NewSelect().
		Model(&daos).
		Where("bridge_key IN (?)", bun.List(bridgeKeys)).
		Where("status IN (?)", bun.List([]string{
			string(relayer.TransferStatusPending),
			string(relayer.TransferStatusInProgress),
		})).
		Where("(next_step_at IS NULL OR next_step_at <= ?)", time.Now()).
		OrderExpr("created_at ASC").
		Limit(limit).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("get steppable transfers: %w", err)
	}

	transfers := make([]*relayer.Transfer, 0, len(daos))
	for i := range daos {
		transfers = append(transfers, fromTransferDao(&daos[i]))
	}
	return transfers, nil
}

// ApplyStep persists the outcome of a successful TokenBridge.Step call:
// status, stage, optional destination tx hash, merged metadata, and the next
// step time (cleared for terminal statuses). A successful step clears any
// previous error message.
func (s *PGStore) ApplyStep(ctx context.Context, id string, res relayer.StepResult, nextStepAt time.Time) error {
	now := time.Now()
	q := s.db.NewUpdate().
		Model((*TransferDao)(nil)).
		Set("status = ?", res.Status).
		Set("stage = ?", res.Stage).
		Set("error_message = NULL").
		Set("updated_at = ?", now).
		Where("id = ?", id)

	if res.DestTxHash != nil {
		q = q.Set("destination_tx_hash = ?", res.DestTxHash)
	}
	if len(res.Metadata) > 0 {
		buf, err := json.Marshal(res.Metadata)
		if err != nil {
			return fmt.Errorf("apply step: marshal metadata: %w", err)
		}
		q = q.Set("metadata = COALESCE(metadata, '{}'::jsonb) || ?::jsonb", string(buf))
	}

	if res.Status.IsTerminal() {
		q = q.Set("next_step_at = NULL")
		if res.Status == relayer.TransferStatusCompleted {
			q = q.Set("completed_at = ?", now)
		}
	} else {
		q = q.Set("next_step_at = ?", nextStepAt)
	}

	result, err := q.Exec(ctx)
	if err != nil {
		return fmt.Errorf("apply step: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("apply step rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("transfer %s not found", id)
	}
	return nil
}

// RecordStepError increments retry_count, stores the error message, and
// schedules the next step attempt.
func (s *PGStore) RecordStepError(ctx context.Context, id string, errMsg string, nextStepAt time.Time) error {
	result, err := s.db.NewUpdate().
		Model((*TransferDao)(nil)).
		Set("retry_count = retry_count + 1").
		Set("error_message = ?", errMsg).
		Set("next_step_at = ?", nextStepAt).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("record step error: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("record step error rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("transfer %s not found", id)
	}
	return nil
}

// GetChainState retrieves the last processed offset for a chain. Returns nil, nil when not found.
func (s *PGStore) GetChainState(ctx context.Context, chainID string) (*relayer.ChainState, error) {
	dao := new(ChainStateDao)
	err := s.db.NewSelect().
		Model(dao).
		Where("chain_id = ?", chainID).
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get chain state: %w", err)
	}
	return fromChainStateDao(dao), nil
}

// SetChainState upserts the last processed offset for a chain.
func (s *PGStore) SetChainState(ctx context.Context, chainID string, blockNumber uint64, offset string) error {
	dao := &ChainStateDao{
		ChainID:       chainID,
		LastBlock:     blockNumber,
		LastBlockHash: offset,
		UpdatedAt:     time.Now(),
	}
	_, err := s.db.NewInsert().
		Model(dao).
		On("CONFLICT (chain_id) DO UPDATE").
		Set("last_block = EXCLUDED.last_block").
		Set("last_block_hash = EXCLUDED.last_block_hash").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("set chain state: %w", err)
	}
	return nil
}
