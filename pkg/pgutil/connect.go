package pgutil

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

// ConnectDB creates a connection to the specified database
func ConnectDB(cfg *DatabaseConfig) (*bun.DB, error) {
	if cfg == nil {
		return nil, fmt.Errorf("database config is nil")
	}

	dsn, dbName, err := withSSLMode(cfg.URL, cfg.SSLMode)
	if err != nil {
		return nil, fmt.Errorf("invalid database URL: %w", err)
	}

	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

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
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close() // Close connection to prevent resource leak
		return nil, fmt.Errorf("failed to connect to database %s: %w", dbName, err)
	}

	log.Printf("Successfully connected to database: %s", dbName)
	return db, nil
}

func withSSLMode(rawURL, sslMode string) (string, string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", "", err
	}

	query := parsed.Query()
	if sslMode != "" && query.Get("sslmode") == "" {
		query.Set("sslmode", sslMode)
		parsed.RawQuery = query.Encode()
	}

	dbName := strings.TrimPrefix(parsed.Path, "/")
	if dbName == "" {
		dbName = "<unknown>"
	}

	return parsed.String(), dbName, nil
}
