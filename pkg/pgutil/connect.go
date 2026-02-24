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

	// Build connector using functional options to properly escape special characters
	connector := pgdriver.NewConnector(
		pgdriver.WithNetwork("tcp"),
		pgdriver.WithAddr(fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)),
		pgdriver.WithUser(cfg.User),
		pgdriver.WithPassword(cfg.Password),
		pgdriver.WithDatabase(cfg.Database),
		pgdriver.WithInsecure(cfg.SSLMode == "disable"),
	)

	sqldb := sql.OpenDB(connector)

	db := bun.NewDB(sqldb, pgdialect.New())

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close() // Close connection to prevent resource leak
		return nil, fmt.Errorf("failed to connect to database %s: %w", cfg.Database, err)
	}

	log.Printf("Successfully connected to database: %s", cfg.Database)
	return db, nil
}
