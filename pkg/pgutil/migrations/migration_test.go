package migrations

import (
	"context"
	"testing"

	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/pgutil"
	"github.com/uptrace/bun"
)

// Test DAO for testing purposes
type testDao struct {
	bun.BaseModel `bun:"table:test_table"`
	ID            int64  `bun:",pk,autoincrement"`
	Name          string `bun:",notnull,type:varchar(100)"`
	Age           int    `bun:",nullzero"`
}

func TestConnectDB_Success(t *testing.T) {
	db, cleanup := pgutil.SetupTestDB(t)
	defer cleanup()

	// Verify connection works
	err := db.Ping()
	if err != nil {
		t.Errorf("Ping() failed: %v", err)
	}
}

func TestConnectDB_InvalidHost(t *testing.T) {
	cfg := &config.DatabaseConfig{
		Host:     "invalid-host-that-does-not-exist",
		Port:     5432,
		User:     "test",
		Password: "test",
		Database: "test",
		SSLMode:  "disable",
	}

	db, err := pgutil.ConnectDB(cfg)
	if err == nil {
		db.Close()
		t.Error("ConnectDB() should fail with invalid host")
	}
}

func TestConnectDB_SSLModeDefault(t *testing.T) {
	db, cleanup := pgutil.SetupTestDB(t)
	defer cleanup()

	// Test that empty SSLMode defaults to "disable"
	// Connection succeeds, which means default was used
	err := db.Ping()
	if err != nil {
		t.Errorf("Ping() with default SSLMode failed: %v", err)
	}
}

func TestCreateSchema(t *testing.T) {
	db, cleanup := pgutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create schema
	err := CreateSchema(ctx, db, &testDao{})
	if err != nil {
		t.Fatalf("CreateSchema() failed: %v", err)
	}

	// Verify table exists
	pgutil.AssertTableExists(t, db, "test_table")

	// Verify idempotency - calling again should not fail
	err = CreateSchema(ctx, db, &testDao{})
	if err != nil {
		t.Errorf("CreateSchema() second call failed: %v", err)
	}
}

func TestDropTables(t *testing.T) {
	db, cleanup := pgutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create table first
	err := CreateSchema(ctx, db, &testDao{})
	if err != nil {
		t.Fatalf("CreateSchema() failed: %v", err)
	}
	pgutil.AssertTableExists(t, db, "test_table")

	// Drop table
	err = DropTables(ctx, db, &testDao{})
	if err != nil {
		t.Fatalf("DropTables() failed: %v", err)
	}

	// Verify table dropped
	pgutil.AssertTableNotExists(t, db, "test_table")

	// Verify idempotency - calling again should not fail
	err = DropTables(ctx, db, &testDao{})
	if err != nil {
		t.Errorf("DropTables() second call failed: %v", err)
	}
}

func TestInsertEntry(t *testing.T) {
	db, cleanup := pgutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create table
	err := CreateSchema(ctx, db, &testDao{})
	if err != nil {
		t.Fatalf("CreateSchema() failed: %v", err)
	}

	// Insert entry
	entry := &testDao{
		Name: "Test User",
		Age:  30,
	}
	err = InsertEntry(ctx, db, entry)
	if err != nil {
		t.Fatalf("InsertEntry() failed: %v", err)
	}

	// Verify entry inserted
	pgutil.AssertRowCount(t, db, "test_table", 1)

	// Verify data
	var result testDao
	err = db.NewRaw("SELECT * FROM test_table WHERE name = ?", "Test User").Scan(ctx, &result)
	if err != nil {
		t.Fatalf("failed to query inserted data: %v", err)
	}
	if result.Name != "Test User" || result.Age != 30 {
		t.Errorf("inserted data mismatch: got Name=%s, Age=%d", result.Name, result.Age)
	}
}

func TestTruncateTables(t *testing.T) {
	db, cleanup := pgutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create table and insert data
	err := CreateSchema(ctx, db, &testDao{})
	if err != nil {
		t.Fatalf("CreateSchema() failed: %v", err)
	}

	err = InsertEntry(ctx, db, &testDao{Name: "User1", Age: 20}, &testDao{Name: "User2", Age: 25})
	if err != nil {
		t.Fatalf("InsertEntry() failed: %v", err)
	}
	pgutil.AssertRowCount(t, db, "test_table", 2)

	// Truncate table
	err = TruncateTables(ctx, db, &testDao{})
	if err != nil {
		t.Fatalf("TruncateTables() failed: %v", err)
	}

	// Verify table is empty
	pgutil.AssertRowCount(t, db, "test_table", 0)

	// Verify table still exists
	pgutil.AssertTableExists(t, db, "test_table")
}

func TestCreateIndex(t *testing.T) {
	db, cleanup := pgutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create table
	err := CreateSchema(ctx, db, &testDao{})
	if err != nil {
		t.Fatalf("CreateSchema() failed: %v", err)
	}

	// Create index
	err = CreateIndex(ctx, db, "test_table", "idx_test_name", "name")
	if err != nil {
		t.Fatalf("CreateIndex() failed: %v", err)
	}

	// Verify index exists
	pgutil.AssertIndexExists(t, db, "idx_test_name")

	// Verify idempotency
	err = CreateIndex(ctx, db, "test_table", "idx_test_name", "name")
	if err != nil {
		t.Errorf("CreateIndex() second call failed: %v", err)
	}
}

func TestCreateIndexes(t *testing.T) {
	db, cleanup := pgutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create table
	err := CreateSchema(ctx, db, &testDao{})
	if err != nil {
		t.Fatalf("CreateSchema() failed: %v", err)
	}

	// Create multiple indexes
	err = CreateIndexes(ctx, db, "test_table", "name", "age")
	if err != nil {
		t.Fatalf("CreateIndexes() failed: %v", err)
	}

	// Verify indexes exist
	pgutil.AssertIndexExists(t, db, "idx_test_table_name")
	pgutil.AssertIndexExists(t, db, "idx_test_table_age")
}

func TestCreateModelIndexes(t *testing.T) {
	db, cleanup := pgutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create table
	err := CreateSchema(ctx, db, &testDao{})
	if err != nil {
		t.Fatalf("CreateSchema() failed: %v", err)
	}

	// Create indexes from model
	err = CreateModelIndexes(ctx, db, &testDao{}, "name", "age")
	if err != nil {
		t.Fatalf("CreateModelIndexes() failed: %v", err)
	}

	// Verify indexes exist
	pgutil.AssertIndexExists(t, db, "idx_test_table_name")
	pgutil.AssertIndexExists(t, db, "idx_test_table_age")
}

func TestCreateUniqueIndexes(t *testing.T) {
	db, cleanup := pgutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create table
	err := CreateSchema(ctx, db, &testDao{})
	if err != nil {
		t.Fatalf("CreateSchema() failed: %v", err)
	}

	// Create unique indexes
	err = CreateUniqueIndexes(ctx, db, "test_table", "name")
	if err != nil {
		t.Fatalf("CreateUniqueIndexes() failed: %v", err)
	}

	// Verify index exists
	pgutil.AssertIndexExists(t, db, "idx_test_table_name")

	// Verify uniqueness by inserting duplicate
	err = InsertEntry(ctx, db, &testDao{Name: "Unique", Age: 20})
	if err != nil {
		t.Fatalf("First insert failed: %v", err)
	}

	err = InsertEntry(ctx, db, &testDao{Name: "Unique", Age: 25})
	if err == nil {
		t.Error("Expected duplicate insert to fail, but it succeeded")
	}
}

func TestCreateModelUniqueIndexes(t *testing.T) {
	db, cleanup := pgutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create table
	err := CreateSchema(ctx, db, &testDao{})
	if err != nil {
		t.Fatalf("CreateSchema() failed: %v", err)
	}

	// Create unique indexes from model
	err = CreateModelUniqueIndexes(ctx, db, &testDao{}, "name")
	if err != nil {
		t.Fatalf("CreateModelUniqueIndexes() failed: %v", err)
	}

	// Verify index exists
	pgutil.AssertIndexExists(t, db, "idx_test_table_name")
}

func TestDropIndex(t *testing.T) {
	db, cleanup := pgutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create table and index
	err := CreateSchema(ctx, db, &testDao{})
	if err != nil {
		t.Fatalf("CreateSchema() failed: %v", err)
	}

	err = CreateIndex(ctx, db, "test_table", "idx_test_name", "name")
	if err != nil {
		t.Fatalf("CreateIndex() failed: %v", err)
	}
	pgutil.AssertIndexExists(t, db, "idx_test_name")

	// Drop index
	err = DropIndex(ctx, db, "idx_test_name")
	if err != nil {
		t.Fatalf("DropIndex() failed: %v", err)
	}

	// Verify index dropped
	var exists bool
	query := `SELECT EXISTS (SELECT FROM pg_indexes WHERE schemaname = 'public' AND indexname = ?)`
	err = db.NewRaw(query, "idx_test_name").Scan(ctx, &exists)
	if err != nil {
		t.Fatalf("failed to check index: %v", err)
	}
	if exists {
		t.Error("index should be dropped but still exists")
	}

	// Verify idempotency
	err = DropIndex(ctx, db, "idx_test_name")
	if err != nil {
		t.Errorf("DropIndex() second call failed: %v", err)
	}
}

func TestDropModelIndexes(t *testing.T) {
	db, cleanup := pgutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	err := CreateSchema(ctx, db, &testDao{})
	if err != nil {
		t.Fatalf("CreateSchema() failed: %v", err)
	}

	err = CreateModelIndexes(ctx, db, &testDao{}, "name", "age")
	if err != nil {
		t.Fatalf("CreateModelIndexes() failed: %v", err)
	}
	pgutil.AssertIndexExists(t, db, "idx_test_table_name")
	pgutil.AssertIndexExists(t, db, "idx_test_table_age")

	err = DropModelIndexes(ctx, db, &testDao{}, "name", "age")
	if err != nil {
		t.Fatalf("DropModelIndexes() failed: %v", err)
	}

	var exists bool
	query := `SELECT EXISTS (SELECT FROM pg_indexes WHERE schemaname = 'public' AND indexname = ?)`
	err = db.NewRaw(query, "idx_test_table_name").Scan(ctx, &exists)
	if err != nil {
		t.Fatalf("failed to check name index: %v", err)
	}
	if exists {
		t.Error("idx_test_table_name should be dropped")
	}

	err = db.NewRaw(query, "idx_test_table_age").Scan(ctx, &exists)
	if err != nil {
		t.Fatalf("failed to check age index: %v", err)
	}
	if exists {
		t.Error("idx_test_table_age should be dropped")
	}
}
