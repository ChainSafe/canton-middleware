package indexer

import "context"

// Store defines the persistence contract for the indexer Processor.
//
// The key invariant: offset and events from the same LedgerTransaction must be
// written atomically. This guarantees that after a restart the processor resumes
// from a consistent point — no event is lost and no event is double-written.
//
// The Bun-backed implementation lives in pkg/indexer/store.
//
//go:generate mockery --name Store --output engine/mocks --outpkg mocks --filename mock_store.go --with-expecter
type Store interface {
	// LatestOffset returns the last successfully persisted ledger offset.
	// Returns 0 and no error when no offset has been stored yet (fresh start).
	// Called once at startup, outside any transaction.
	LatestOffset(ctx context.Context) (int64, error)

	// RunInTx executes fn inside a single database transaction.
	// On success fn's return value is nil and the transaction is committed.
	// On any error the transaction is rolled back and the error is returned.
	// The Store passed to fn is scoped to the transaction — all methods on it
	// participate in the same underlying DB transaction.
	RunInTx(ctx context.Context, fn func(ctx context.Context, tx Store) error) error

	// SaveBatch persists a batch of ParsedEvents and advances the stored ledger offset.
	// Duplicate events (same ContractID) are silently skipped via ON CONFLICT DO NOTHING.
	// When events is empty the offset is still advanced to skip no-op transactions on restart.
	SaveBatch(ctx context.Context, offset int64, events []*ParsedEvent) error

	// UpsertToken records a token deployment on first observation.
	// Subsequent calls for the same {InstrumentAdmin, InstrumentID} are no-ops
	// (ON CONFLICT DO NOTHING).
	UpsertToken(ctx context.Context, token *Token) error

	// ApplyBalanceDelta adjusts a party's token balance by delta (signed decimal string).
	// The balance row is created at zero if it does not yet exist, then delta is added.
	// The store must also update Token.HolderCount atomically:
	//   - increment when a party's balance transitions from zero to positive
	//   - decrement when a party's balance transitions from positive to zero
	ApplyBalanceDelta(ctx context.Context, partyID, instrumentAdmin, instrumentID, delta string) error

	// ApplySupplyDelta adjusts a token's TotalSupply by delta (signed decimal string).
	// Called once per mint (+amount) or burn (-amount). Transfer events must not call this.
	ApplySupplyDelta(ctx context.Context, instrumentAdmin, instrumentID, delta string) error
}
