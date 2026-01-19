package apidb

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

// Store provides database operations for the ERC-20 API server
type Store struct {
	db *sql.DB
}

// NewStore creates a new database store for the API server
func NewStore(connString string) (*Store, error) {
	db, err := sql.Open("postgres", connString)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying database connection for advanced queries
func (s *Store) DB() *sql.DB {
	return s.db
}
