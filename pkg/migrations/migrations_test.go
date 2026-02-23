package migrations

import (
	"context"
	"testing"

	"github.com/chainsafe/canton-middleware/pkg/apidb/dao"
	"github.com/chainsafe/canton-middleware/pkg/migrations/apidb"
	"github.com/chainsafe/canton-middleware/pkg/migrations/relayerdb"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"
)

func TestAPIDBMigrations_Apply(t *testing.T) {
	db, cleanup := mghelper.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	migrator := migrate.NewMigrator(db, apidb.Migrations)

	// Initialize migration system
	err := migrator.Init(ctx)
	if err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	// Run all migrations up
	group, err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}
	if group.IsZero() {
		t.Error("Expected migrations to run, but none were applied")
	}

	// Verify all expected tables exist
	expectedTables := []string{
		"users",
		"whitelist",
		"token_metrics",
		"bridge_events",
		"reconciliation_state",
		"evm_transactions",
		"evm_meta",
		"evm_logs",
		"bun_migrations",
	}

	for _, table := range expectedTables {
		mghelper.AssertTableExists(t, db, table)
	}

	// Verify indexes exist for users table
	mghelper.AssertIndexExists(t, db, "idx_users_fingerprint")
	mghelper.AssertIndexExists(t, db, "idx_users_canton_party_id")

	// Verify indexes exist for transfers table (if applicable)
	// Note: Some migrations create additional indexes
}

func TestRelayerDBMigrations_Apply(t *testing.T) {
	db, cleanup := mghelper.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	migrator := migrate.NewMigrator(db, relayerdb.Migrations)

	// Initialize migration system
	err := migrator.Init(ctx)
	if err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	// Run all migrations up
	group, err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}
	if group.IsZero() {
		t.Error("Expected migrations to run, but none were applied")
	}

	// Verify all expected tables exist
	expectedTables := []string{
		"transfers",
		"chain_state",
		"nonce_state",
		"bridge_balances",
		"bun_migrations",
	}

	for _, table := range expectedTables {
		mghelper.AssertTableExists(t, db, table)
	}

	// Verify indexes exist for transfers table
	mghelper.AssertIndexExists(t, db, "idx_transfers_status")
	mghelper.AssertIndexExists(t, db, "idx_transfers_direction")
	mghelper.AssertIndexExists(t, db, "idx_transfers_source_tx_hash")
}

func TestMigrations_Idempotency(t *testing.T) {
	db, cleanup := mghelper.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	migrator := migrate.NewMigrator(db, apidb.Migrations)

	// Initialize
	err := migrator.Init(ctx)
	if err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	// Run migrations first time
	_, err = migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("First Migrate() failed: %v", err)
	}

	// Run migrations second time - should not fail
	group, err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Second Migrate() failed: %v", err)
	}

	// Should return zero group (no new migrations)
	if !group.IsZero() {
		t.Error("Expected no new migrations on second run")
	}

	// Verify tables still exist
	mghelper.AssertTableExists(t, db, "users")
	mghelper.AssertTableExists(t, db, "token_metrics")
}

func TestMigrations_Rollback(t *testing.T) {
	db, cleanup := mghelper.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Use relayerdb for simpler test (fewer migrations)
	migrator := migrate.NewMigrator(db, relayerdb.Migrations)

	// Initialize
	err := migrator.Init(ctx)
	if err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	// Run migrations up
	_, err = migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	// Verify tables exist
	mghelper.AssertTableExists(t, db, "transfers")
	mghelper.AssertTableExists(t, db, "chain_state")

	// Rollback last migration group (all migrations run in one group by Migrate())
	group, err := migrator.Rollback(ctx)
	if err != nil {
		t.Fatalf("Rollback() failed: %v", err)
	}
	if group.IsZero() {
		t.Error("Expected rollback to process a migration")
	}

	// Verify all tables are dropped (entire migration group is rolled back)
	mghelper.AssertTableNotExists(t, db, "bridge_balances")
	mghelper.AssertTableNotExists(t, db, "nonce_state")
	mghelper.AssertTableNotExists(t, db, "chain_state")
	mghelper.AssertTableNotExists(t, db, "transfers")
}

func TestSeedData_Applied(t *testing.T) {
	db, cleanup := mghelper.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	migrator := migrate.NewMigrator(db, apidb.Migrations)

	// Initialize and run migrations
	err := migrator.Init(ctx)
	if err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	_, err = migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	// Verify seed data inserted
	mghelper.AssertRowCount(t, db, "token_metrics", 2)

	// Verify PROMPT and DEMO tokens exist
	count, err := db.NewSelect().
		Model((*dao.TokenMetricsDao)(nil)).
		ModelTableExpr("token_metrics").
		Where("token_symbol IN (?)", bun.In([]string{"PROMPT", "DEMO"})).
		Count(ctx)
	if err != nil {
		t.Fatalf("Failed to query seed data: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 seed tokens (PROMPT, DEMO), got %d", count)
	}

	// Verify token values
	var tokens []struct {
		TokenSymbol string `bun:"token_symbol"`
		TotalSupply string `bun:"total_supply"`
	}
	err = db.NewSelect().
		TableExpr("token_metrics").
		Column("token_symbol", "total_supply").
		Order("token_symbol ASC").
		Scan(ctx, &tokens)
	if err != nil {
		t.Fatalf("Failed to query token data: %v", err)
	}

	if len(tokens) != 2 {
		t.Fatalf("Expected 2 tokens, got %d", len(tokens))
	}

	// DEMO should be first alphabetically
	// Note: NUMERIC(38,18) returns "0.000000000000000000" for value 0
	if tokens[0].TokenSymbol != "DEMO" {
		t.Errorf("Expected DEMO as first token, got %s", tokens[0].TokenSymbol)
	}
	if tokens[0].TotalSupply != "0" && tokens[0].TotalSupply != "0.000000000000000000" {
		t.Errorf("Expected DEMO supply to be 0, got %s", tokens[0].TotalSupply)
	}

	// PROMPT should be second
	if tokens[1].TokenSymbol != "PROMPT" {
		t.Errorf("Expected PROMPT as second token, got %s", tokens[1].TokenSymbol)
	}
	if tokens[1].TotalSupply != "0" && tokens[1].TotalSupply != "0.000000000000000000" {
		t.Errorf("Expected PROMPT supply to be 0, got %s", tokens[1].TotalSupply)
	}
}

func TestSingletonConstraint_Applied(t *testing.T) {
	db, cleanup := mghelper.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	migrator := migrate.NewMigrator(db, apidb.Migrations)

	// Initialize and run migrations
	err := migrator.Init(ctx)
	if err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	_, err = migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	// Verify reconciliatÏion_state table exists
	mghelper.AssertTableExists(t, db, "reconciliation_state")

	// Verify initial row inserted
	mghelper.AssertRowCount(t, db, "reconciliation_state", 1)

	// Verify singleton constraint exists
	var hasConstraint bool
	query := `
		SELECT EXISTS (
			SELECT 1 FROM pg_constraint
			WHERE conname = 'singleton_check'
			AND conrelid = 'reconciliation_state'::regclass
		)
	`
	err = db.NewRaw(query).Scan(ctx, &hasConstraint)
	if err != nil {
		t.Fatalf("Failed to check constraint: %v", err)
	}
	if !hasConstraint {
		t.Error("singleton_check constraint does not exist")
	}

	// Try to insert a second row with id != 1 - should fail due to constraint
	_, err = db.NewInsert().
		Model(&dao.ReconciliationStateDao{
			ID:                  2,
			LastProcessedOffset: 0,
			EventsProcessed:     0,
		}).
		ModelTableExpr("reconciliation_state").
		Exec(ctx)
	if err == nil {
		t.Error("Expected insert with id != 1 to fail due to singleton constraint, but it succeeded")
	}

	// Verify still only 1 row
	mghelper.AssertRowCount(t, db, "reconciliation_state", 1)

	// Verify the row has id = 1
	var result struct {
		ID int `bun:"id"`
	}
	err = db.NewSelect().
		TableExpr("reconciliation_state").
		Column("id").
		Scan(ctx, &result)
	if err != nil {
		t.Fatalf("Failed to query reconciliation_state: %v", err)
	}
	if result.ID != 1 {
		t.Errorf("Expected reconciliation_state.id = 1, got %d", result.ID)
	}
}

func TestSeedData_Idempotency(t *testing.T) {
	db, cleanup := mghelper.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	migrator := migrate.NewMigrator(db, apidb.Migrations)

	// Initialize and run migrations
	err := migrator.Init(ctx)
	if err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	_, err = migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("First Migrate() failed: %v", err)
	}

	// Verify initial seed data
	mghelper.AssertRowCount(t, db, "token_metrics", 2)

	// Manually insert a token to test ON CONFLICT behavior
	_, err = db.NewInsert().
		Model(&dao.TokenMetricsDao{
			TokenSymbol: "TEST",
			TotalSupply: "100",
		}).
		ModelTableExpr("token_metrics").
		Exec(ctx)
	if err != nil {
		t.Fatalf("Failed to insert test token: %v", err)
	}
	mghelper.AssertRowCount(t, db, "token_metrics", 3)

	// Run seed migration again by running the entire up migration
	// This should not fail and should not duplicate PROMPT/DEMO
	_, err = migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Second Migrate() failed: %v", err)
	}

	// Verify still have 3 rows (PROMPT, DEMO, TEST)
	mghelper.AssertRowCount(t, db, "token_metrics", 3)

	// Verify PROMPT and DEMO still exist once each
	count, err := db.NewSelect().
		Model((*dao.TokenMetricsDao)(nil)).
		ModelTableExpr("token_metrics").
		Where("token_symbol IN (?)", bun.In([]string{"PROMPT", "DEMO"})).
		Count(ctx)
	if err != nil {
		t.Fatalf("Failed to query seed data: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 seed tokens after re-run, got %d", count)
	}
}

func TestEvmMeta_InitialData(t *testing.T) {
	db, cleanup := mghelper.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	migrator := migrate.NewMigrator(db, apidb.Migrations)

	// Initialize and run migrations
	err := migrator.Init(ctx)
	if err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	_, err = migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	// Verify evm_meta table exists
	mghelper.AssertTableExists(t, db, "evm_meta")

	// Verify initial data inserted
	mghelper.AssertRowCount(t, db, "evm_meta", 1)

	// Verify latest_block_number = 0
	var result struct {
		Value string `bun:"value"`
	}
	err = db.NewSelect().
		TableExpr("evm_meta").
		Column("value").
		Where("key = ?", "latest_block_number").
		Scan(ctx, &result)
	if err != nil {
		t.Fatalf("Failed to query evm_meta: %v", err)
	}
	if result.Value != "0" {
		t.Errorf("Expected latest_block_number = '0', got '%s'", result.Value)
	}
}
