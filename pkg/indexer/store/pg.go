package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
	"github.com/uptrace/bun"

	"github.com/chainsafe/canton-middleware/pkg/indexer"
	"github.com/chainsafe/canton-middleware/pkg/indexer/engine"
)

// PGStore is a PostgreSQL-backed store for the indexer, using Bun ORM.
// It satisfies both engine.Store (write path: processor) and service.Store (read path: HTTP).
type PGStore struct {
	db bun.IDB
}

// NewStore creates a new Bun-backed indexer store.
func NewStore(db *bun.DB) *PGStore {
	return &PGStore{db: db}
}

// LatestOffset returns the last persisted ledger offset, or 0 on a fresh start.
func (s *PGStore) LatestOffset(ctx context.Context) (int64, error) {
	dao := new(OffsetDao)
	err := s.db.NewSelect().Model(dao).Where("id = 1").Limit(1).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("latest offset: %w", err)
	}
	return dao.LedgerOffset, nil
}

// RunInTx executes fn inside a single database transaction.
// The Store passed to fn is scoped to that transaction.
func (s *PGStore) RunInTx(ctx context.Context, fn func(ctx context.Context, tx engine.Store) error) error {
	db, ok := s.db.(*bun.DB)
	if !ok {
		return errors.New("RunInTx called on a transaction-scoped store")
	}
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return fn(ctx, &PGStore{db: tx})
	})
}

// InsertEvent persists one ParsedEvent. Returns inserted=false when the event already
// exists (idempotent by ContractID).
func (s *PGStore) InsertEvent(ctx context.Context, event *indexer.ParsedEvent) (bool, error) {
	result, err := s.db.NewInsert().
		Model(toEventDao(event)).
		On("CONFLICT (contract_id) DO NOTHING").
		Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("insert event: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("insert event rows affected: %w", err)
	}
	return n > 0, nil
}

// SaveOffset upserts the single-row offset record.
func (s *PGStore) SaveOffset(ctx context.Context, offset int64) error {
	_, err := s.db.NewInsert().
		Model(&OffsetDao{ID: 1, LedgerOffset: offset}).
		On("CONFLICT (id) DO UPDATE").
		Set("ledger_offset = EXCLUDED.ledger_offset").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("save offset: %w", err)
	}
	return nil
}

// UpsertToken records a token on first observation. Subsequent calls for the same
// composite key (InstrumentAdmin, InstrumentID) are no-ops.
func (s *PGStore) UpsertToken(ctx context.Context, token *indexer.Token) error {
	_, err := s.db.NewInsert().
		Model(&TokenDao{
			InstrumentAdmin: token.InstrumentAdmin,
			InstrumentID:    token.InstrumentID,
			Issuer:          token.Issuer,
			TotalSupply:     "0",
			HolderCount:     0,
			FirstSeenOffset: token.FirstSeenOffset,
			FirstSeenAt:     token.FirstSeenAt,
		}).
		On("CONFLICT (instrument_admin, instrument_id) DO NOTHING").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("upsert token: %w", err)
	}
	return nil
}

// ApplySupplyDelta adds delta (signed decimal string) to a token's TotalSupply.
func (s *PGStore) ApplySupplyDelta(ctx context.Context, instrumentAdmin, instrumentID, delta string) error {
	_, err := s.db.NewUpdate().
		Model((*TokenDao)(nil)).
		Set("total_supply = (total_supply::numeric + ?::numeric)::text", delta).
		Where("instrument_admin = ?", instrumentAdmin).
		Where("instrument_id = ?", instrumentID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("apply supply delta: %w", err)
	}
	return nil
}

// ApplyBalanceDelta adjusts a party's balance by delta (signed decimal string).
// Manages HolderCount on the parent token:
//   - increments when balance transitions from zero to positive
//   - decrements when balance transitions from positive to zero
//
// Must be called within a RunInTx transaction so the three steps are atomic.
func (s *PGStore) ApplyBalanceDelta(ctx context.Context, partyID, instrumentAdmin, instrumentID, delta string) error {
	// Step 1: read current balance (zero if the row doesn't exist yet).
	dao := new(BalanceDao)
	err := s.db.NewSelect().Model(dao).
		Where("party_id = ?", partyID).
		Where("instrument_admin = ?", instrumentAdmin).
		Where("instrument_id = ?", instrumentID).
		Limit(1).Scan(ctx)
	isNew := errors.Is(err, sql.ErrNoRows)
	if err != nil && !isNew {
		return fmt.Errorf("read balance: %w", err)
	}

	oldAmount := decimal.Zero
	if !isNew {
		oldAmount, err = decimal.NewFromString(dao.Amount)
		if err != nil {
			return fmt.Errorf("parse old amount %q: %w", dao.Amount, err)
		}
	}

	// Step 2: compute new balance and upsert.
	d, err := decimal.NewFromString(delta)
	if err != nil {
		return fmt.Errorf("parse delta %q: %w", delta, err)
	}
	newAmount := oldAmount.Add(d)

	_, err = s.db.NewInsert().
		Model(&BalanceDao{
			PartyID:         partyID,
			InstrumentAdmin: instrumentAdmin,
			InstrumentID:    instrumentID,
			Amount:          newAmount.String(),
		}).
		On("CONFLICT (party_id, instrument_admin, instrument_id) DO UPDATE").
		Set("amount = EXCLUDED.amount").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("upsert balance: %w", err)
	}

	// Step 3: update holder_count if the balance crossed zero.
	wasZero := isNew || oldAmount.IsZero()
	isZero := newAmount.IsZero()
	var holderDelta int64
	switch {
	case wasZero && !isZero:
		holderDelta = 1
	case !wasZero && isZero:
		holderDelta = -1
	}
	if holderDelta != 0 {
		_, err = s.db.NewUpdate().
			Model((*TokenDao)(nil)).
			Set("holder_count = holder_count + ?", holderDelta).
			Where("instrument_admin = ?", instrumentAdmin).
			Where("instrument_id = ?", instrumentID).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("update holder count: %w", err)
		}
	}
	return nil
}

// ─── service.Store read-path methods ─────────────────────────────────────────

// GetToken retrieves token metadata by composite key. Returns nil, nil when not found.
func (s *PGStore) GetToken(ctx context.Context, admin, id string) (*indexer.Token, error) {
	dao := new(TokenDao)
	err := s.db.NewSelect().Model(dao).
		Where("instrument_admin = ?", admin).
		Where("instrument_id = ?", id).
		Limit(1).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}
	return fromTokenDao(dao), nil
}

// ListTokens returns a paginated list of all indexed tokens, ordered by first_seen_offset ASC.
func (s *PGStore) ListTokens(ctx context.Context, p indexer.Pagination) ([]*indexer.Token, int64, error) {
	var daos []TokenDao
	q := s.db.NewSelect().Model(&daos).OrderExpr("first_seen_offset ASC")

	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list tokens count: %w", err)
	}
	if err = q.Limit(p.Limit).Offset((p.Page - 1) * p.Limit).Scan(ctx); err != nil {
		return nil, 0, fmt.Errorf("list tokens: %w", err)
	}

	tokens := make([]*indexer.Token, len(daos))
	for i := range daos {
		tokens[i] = fromTokenDao(&daos[i])
	}
	return tokens, int64(total), nil
}

// GetBalance retrieves a single balance record. Returns nil, nil when not found.
func (s *PGStore) GetBalance(ctx context.Context, partyID, admin, id string) (*indexer.Balance, error) {
	dao := new(BalanceDao)
	err := s.db.NewSelect().Model(dao).
		Where("party_id = ?", partyID).
		Where("instrument_admin = ?", admin).
		Where("instrument_id = ?", id).
		Limit(1).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get balance: %w", err)
	}
	return fromBalanceDao(dao), nil
}

// ListBalancesForParty returns a paginated list of all holdings for a given party.
func (s *PGStore) ListBalancesForParty(ctx context.Context, partyID string, p indexer.Pagination) ([]*indexer.Balance, int64, error) {
	var daos []BalanceDao
	q := s.db.NewSelect().Model(&daos).Where("party_id = ?", partyID).
		OrderExpr("instrument_admin ASC, instrument_id ASC")

	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list balances for party count: %w", err)
	}
	if err = q.Limit(p.Limit).Offset((p.Page - 1) * p.Limit).Scan(ctx); err != nil {
		return nil, 0, fmt.Errorf("list balances for party: %w", err)
	}

	balances := make([]*indexer.Balance, len(daos))
	for i := range daos {
		balances[i] = fromBalanceDao(&daos[i])
	}
	return balances, int64(total), nil
}

// ListBalancesForToken returns a paginated list of all holders of a given token.
func (s *PGStore) ListBalancesForToken(ctx context.Context, admin, id string, p indexer.Pagination) ([]*indexer.Balance, int64, error) {
	var daos []BalanceDao
	q := s.db.NewSelect().Model(&daos).
		Where("instrument_admin = ?", admin).
		Where("instrument_id = ?", id).
		OrderExpr("party_id ASC")

	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list balances for token count: %w", err)
	}
	if err = q.Limit(p.Limit).Offset((p.Page - 1) * p.Limit).Scan(ctx); err != nil {
		return nil, 0, fmt.Errorf("list balances for token: %w", err)
	}

	balances := make([]*indexer.Balance, len(daos))
	for i := range daos {
		balances[i] = fromBalanceDao(&daos[i])
	}
	return balances, int64(total), nil
}

// GetEvent retrieves a single event by contract ID. Returns nil, nil when not found.
func (s *PGStore) GetEvent(ctx context.Context, contractID string) (*indexer.ParsedEvent, error) {
	dao := new(EventDao)
	err := s.db.NewSelect().Model(dao).
		Where("contract_id = ?", contractID).
		Limit(1).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get event: %w", err)
	}
	return fromEventDao(dao), nil
}

// ListEvents returns a paginated, ledger_offset-ascending list of events.
// Zero-value EventFilter fields are ignored.
func (s *PGStore) ListEvents(ctx context.Context, f indexer.EventFilter, p indexer.Pagination) ([]*indexer.ParsedEvent, int64, error) {
	var daos []EventDao
	q := s.db.NewSelect().Model(&daos).OrderExpr("ledger_offset ASC")

	if f.InstrumentAdmin != "" {
		q = q.Where("instrument_admin = ?", f.InstrumentAdmin)
	}
	if f.InstrumentID != "" {
		q = q.Where("instrument_id = ?", f.InstrumentID)
	}
	if f.PartyID != "" {
		q = q.Where("(from_party_id = ? OR to_party_id = ?)", f.PartyID, f.PartyID)
	}
	if f.EventType != "" {
		q = q.Where("event_type = ?", string(f.EventType))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list events count: %w", err)
	}
	if err = q.Limit(p.Limit).Offset((p.Page - 1) * p.Limit).Scan(ctx); err != nil {
		return nil, 0, fmt.Errorf("list events: %w", err)
	}

	events := make([]*indexer.ParsedEvent, len(daos))
	for i := range daos {
		events[i] = fromEventDao(&daos[i])
	}
	return events, int64(total), nil
}
