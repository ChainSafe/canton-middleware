package migrations

import (
	"context"
	"testing"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"

	apidbdao "github.com/chainsafe/canton-middleware/pkg/apidb/dao"
	"github.com/chainsafe/canton-middleware/pkg/db/dao"
	"github.com/chainsafe/canton-middleware/pkg/migrations/apidb"
	"github.com/chainsafe/canton-middleware/pkg/migrations/relayerdb"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil"
	"github.com/chainsafe/canton-middleware/pkg/userstore"
)

func modelCount(t *testing.T, ctx context.Context, db *bun.DB, model any) int {
	t.Helper()

	count, err := db.NewSelect().Model(model).Count(ctx)
	if err != nil {
		t.Fatalf("count failed for %T: %v", model, err)
	}
	return count
}

func TestAPIDBMigrations_Apply(t *testing.T) {
	db, cleanup := mghelper.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	migrator := migrate.NewMigrator(db, apidb.Migrations)

	if err := migrator.Init(ctx); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	group, err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}
	if group.IsZero() {
		t.Error("expected migrations to run, but none were applied")
	}

	modelCount(t, ctx, db, &userstore.UserDao{})
	modelCount(t, ctx, db, &userstore.WhitelistDao{})
	modelCount(t, ctx, db, &apidbdao.TokenMetricsDao{})
	modelCount(t, ctx, db, &apidbdao.BridgeEventDao{})
	modelCount(t, ctx, db, &apidbdao.ReconciliationStateDao{})
	modelCount(t, ctx, db, &apidbdao.EvmTransactionDao{})
	modelCount(t, ctx, db, &apidbdao.EvmMetaDao{})
	modelCount(t, ctx, db, &apidbdao.EvmLogDao{})
}

func TestRelayerDBMigrations_Apply(t *testing.T) {
	db, cleanup := mghelper.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	migrator := migrate.NewMigrator(db, relayerdb.Migrations)

	if err := migrator.Init(ctx); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	group, err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}
	if group.IsZero() {
		t.Error("expected migrations to run, but none were applied")
	}

	modelCount(t, ctx, db, &dao.TransferDao{})
	modelCount(t, ctx, db, &dao.ChainStateDao{})
	modelCount(t, ctx, db, &dao.NonceStateDao{})
	modelCount(t, ctx, db, &dao.BridgeBalanceDao{})
}

func TestMigrations_Idempotency(t *testing.T) {
	db, cleanup := mghelper.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	migrator := migrate.NewMigrator(db, apidb.Migrations)

	if err := migrator.Init(ctx); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	if _, err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("first Migrate() failed: %v", err)
	}

	group, err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("second Migrate() failed: %v", err)
	}
	if !group.IsZero() {
		t.Error("expected no new migrations on second run")
	}
}

func TestMigrations_Rollback(t *testing.T) {
	db, cleanup := mghelper.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	migrator := migrate.NewMigrator(db, relayerdb.Migrations)

	if err := migrator.Init(ctx); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	if _, err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	group, err := migrator.Rollback(ctx)
	if err != nil {
		t.Fatalf("Rollback() failed: %v", err)
	}
	if group.IsZero() {
		t.Error("expected rollback to process a migration")
	}
}

func TestSeedData_Applied(t *testing.T) {
	db, cleanup := mghelper.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	migrator := migrate.NewMigrator(db, apidb.Migrations)
	if err := migrator.Init(ctx); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	count, err := db.NewSelect().
		Model((*apidbdao.TokenMetricsDao)(nil)).
		Where("token_symbol IN (?)", bun.List([]string{"PROMPT", "DEMO"})).
		Count(ctx)
	if err != nil {
		t.Fatalf("failed to query seeded tokens: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 seeded tokens (PROMPT, DEMO), got %d", count)
	}

	var tokens []apidbdao.TokenMetricsDao
	if err = db.NewSelect().
		Model(&tokens).
		Where("token_symbol IN (?)", bun.List([]string{"PROMPT", "DEMO"})).
		Order("token_symbol ASC").
		Scan(ctx); err != nil {
		t.Fatalf("failed to scan token data: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("expected 2 seeded tokens, got %d", len(tokens))
	}
}

func TestSingletonConstraint_Applied(t *testing.T) {
	db, cleanup := mghelper.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	migrator := migrate.NewMigrator(db, apidb.Migrations)
	if err := migrator.Init(ctx); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	if got := modelCount(t, ctx, db, &apidbdao.ReconciliationStateDao{}); got != 1 {
		t.Fatalf("expected one reconciliation state row, got %d", got)
	}

	_, err := db.NewInsert().
		Model(&apidbdao.ReconciliationStateDao{
			ID:                  2,
			LastProcessedOffset: 0,
			EventsProcessed:     0,
		}).
		Exec(ctx)
	if err == nil {
		t.Fatal("expected singleton constraint violation, insert succeeded")
	}

	if got := modelCount(t, ctx, db, &apidbdao.ReconciliationStateDao{}); got != 1 {
		t.Fatalf("expected one reconciliation state row after violation, got %d", got)
	}
}

func TestSeedData_Idempotency(t *testing.T) {
	db, cleanup := mghelper.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	migrator := migrate.NewMigrator(db, apidb.Migrations)
	if err := migrator.Init(ctx); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("first Migrate() failed: %v", err)
	}

	_, err := db.NewInsert().
		Model(&apidbdao.TokenMetricsDao{
			TokenSymbol: "TEST",
			TotalSupply: "100",
		}).
		Exec(ctx)
	if err != nil {
		t.Fatalf("failed to insert test token: %v", err)
	}

	if _, err = migrator.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate() failed: %v", err)
	}

	count, err := db.NewSelect().
		Model((*apidbdao.TokenMetricsDao)(nil)).
		Where("token_symbol IN (?)", bun.List([]string{"PROMPT", "DEMO"})).
		Count(ctx)
	if err != nil {
		t.Fatalf("failed to query seeded tokens: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 seeded tokens after rerun, got %d", count)
	}
}

func TestEvmMeta_InitialData(t *testing.T) {
	db, cleanup := mghelper.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	migrator := migrate.NewMigrator(db, apidb.Migrations)
	if err := migrator.Init(ctx); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	var meta apidbdao.EvmMetaDao
	err := db.NewSelect().
		Model(&meta).
		Where(`"key" = ?`, "latest_block_number").
		Limit(1).
		Scan(ctx)
	if err != nil {
		t.Fatalf("failed to query evm_meta row: %v", err)
	}
	if meta.Value != "0" {
		t.Errorf("expected latest_block_number = '0', got %q", meta.Value)
	}
}
