package pgutil

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	"github.com/chainsafe/canton-middleware/pkg/config"
)

// ConnectDB creates a connection to the specified database
func ConnectDB(cfg *config.DatabaseConfig) (*bun.DB, error) {
	ctx := context.Background()
	// Use default sslmode if not specified
	sslmode := cfg.SSLMode
	if sslmode == "" {
		sslmode = "disable"
	}

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.Database,
		sslmode,
	)

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))

	db := bun.NewDB(sqldb, pgdialect.New())

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close() // Close connection to prevent resource leak
		return nil, fmt.Errorf("failed to connect to database %s: %w", cfg.Database, err)
	}

	log.Printf("Successfully connected to database: %s", cfg.Database)
	return db, nil
}
