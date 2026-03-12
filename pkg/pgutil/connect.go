package pgutil

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

// ConnectDB creates a connection to the specified database
func ConnectDB(cfg *DatabaseConfig) (*bun.DB, error) {
	dsn := cfg.GetConnectionString()
	timeout := time.Duration(cfg.Timeout) * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	connector := pgdriver.NewConnector(
		pgdriver.WithDSN(dsn),
		pgdriver.WithTimeout(timeout),
	)

	sqldb := sql.OpenDB(connector)
	if cfg.PoolSize > 0 {
		sqldb.SetMaxOpenConns(cfg.PoolSize)
		sqldb.SetMaxIdleConns(cfg.PoolSize)
	}

	db := bun.NewDB(sqldb, pgdialect.New())

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close() // Close connection to prevent resource leak
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	log.Printf("Successfully connected to database")
	return db, nil
}
