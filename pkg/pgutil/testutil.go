package pgutil

import (
	"context"
	"testing"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
)

// SetupTestDB creates a PostgreSQL testcontainer and returns a connection
func SetupTestDB(t *testing.T) (*bun.DB, func()) {
	t.Helper()
	ctx := context.Background()

	// Start PostgreSQL container with wait strategy
	container, err := postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase("test_db"),
		postgres.WithUsername("test_user"),
		postgres.WithPassword("test_pass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}

	// Get connection details
	host, err := container.Host(ctx)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		t.Fatalf("failed to get container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		t.Fatalf("failed to get container port: %v", err)
	}

	// Create database config
	cfg := &config.DatabaseConfig{
		Host:     host,
		Port:     port.Int(),
		User:     "test_user",
		Password: "test_pass",
		Database: "test_db",
		SSLMode:  "disable",
	}

	// Connect to database with retry logic
	var db *bun.DB
	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
		db, err = ConnectDB(cfg)
		if err == nil {
			break
		}
		if i == maxRetries-1 {
			_ = testcontainers.TerminateContainer(container)
			t.Fatalf("failed to connect to test database after %d attempts: %v", maxRetries, err)
		}
		// Exponential backoff: 100ms, 200ms, 400ms, 800ms, 1.6s, 3.2s...
		backoff := time.Duration(100*(1<<uint(i))) * time.Millisecond
		time.Sleep(backoff)
	}

	// Return cleanup function
	cleanup := func() {
		_ = db.Close()
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	}

	return db, cleanup
}

// AssertTableExists checks if a table exists in the database
func AssertTableExists(t *testing.T, db *bun.DB, tableName string) {
	t.Helper()
	ctx := context.Background()

	var exists bool
	err := db.NewSelect().
		ColumnExpr("EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = ? AND table_name = ?)", "public", tableName).
		Scan(ctx, &exists)
	if err != nil {
		t.Fatalf("failed to check if table %s exists: %v", tableName, err)
	}

	if !exists {
		t.Errorf("table %s does not exist", tableName)
	}
}

// AssertIndexExists checks if an index exists in the database
func AssertIndexExists(t *testing.T, db *bun.DB, indexName string) {
	t.Helper()
	ctx := context.Background()

	var exists bool
	err := db.NewSelect().
		ColumnExpr("EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname = ? AND indexname = ?)", "public", indexName).
		Scan(ctx, &exists)
	if err != nil {
		t.Fatalf("failed to check if index %s exists: %v", indexName, err)
	}

	if !exists {
		t.Errorf("index %s does not exist", indexName)
	}
}

// AssertRowCount checks if a table has the expected number of rows
func AssertRowCount(t *testing.T, db *bun.DB, tableName string, expected int) {
	t.Helper()
	ctx := context.Background()

	var count int
	err := db.NewSelect().
		TableExpr("?", bun.Ident(tableName)).
		ColumnExpr("COUNT(*)").
		Scan(ctx, &count)
	if err != nil {
		t.Fatalf("failed to count rows in table %s: %v", tableName, err)
	}

	if count != expected {
		t.Errorf("table %s: expected %d rows, got %d", tableName, expected, count)
	}
}

// AssertTableNotExists checks if a table does not exist in the database
func AssertTableNotExists(t *testing.T, db *bun.DB, tableName string) {
	t.Helper()
	ctx := context.Background()

	var exists bool
	err := db.NewSelect().
		ColumnExpr("EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = ? AND table_name = ?)", "public", tableName).
		Scan(ctx, &exists)
	if err != nil {
		t.Fatalf("failed to check if table %s exists: %v", tableName, err)
	}

	if exists {
		t.Errorf("table %s should not exist but it does", tableName)
	}
}
