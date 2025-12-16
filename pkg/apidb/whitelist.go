package apidb

import (
	"database/sql"
	"time"
)

// WhitelistEntry represents an allowed EVM address
type WhitelistEntry struct {
	EVMAddress string    `json:"evm_address"`
	Note       string    `json:"note,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// IsWhitelisted checks if an EVM address is whitelisted
func (s *Store) IsWhitelisted(evmAddress string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM whitelist WHERE evm_address = $1)`
	err := s.db.QueryRow(query, evmAddress).Scan(&exists)
	return exists, err
}

// AddToWhitelist adds an EVM address to the whitelist
func (s *Store) AddToWhitelist(evmAddress, note string) error {
	query := `
		INSERT INTO whitelist (evm_address, note)
		VALUES ($1, $2)
		ON CONFLICT (evm_address) DO UPDATE SET note = $2
	`
	_, err := s.db.Exec(query, evmAddress, note)
	return err
}

// RemoveFromWhitelist removes an EVM address from the whitelist
func (s *Store) RemoveFromWhitelist(evmAddress string) error {
	query := `DELETE FROM whitelist WHERE evm_address = $1`
	_, err := s.db.Exec(query, evmAddress)
	return err
}

// GetWhitelistEntry retrieves a whitelist entry by EVM address
func (s *Store) GetWhitelistEntry(evmAddress string) (*WhitelistEntry, error) {
	entry := &WhitelistEntry{}
	query := `
		SELECT evm_address, note, created_at
		FROM whitelist
		WHERE evm_address = $1
	`
	err := s.db.QueryRow(query, evmAddress).Scan(
		&entry.EVMAddress,
		&entry.Note,
		&entry.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return entry, nil
}

// ListWhitelist returns all whitelisted addresses
func (s *Store) ListWhitelist() ([]*WhitelistEntry, error) {
	query := `SELECT evm_address, note, created_at FROM whitelist ORDER BY created_at DESC`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*WhitelistEntry
	for rows.Next() {
		entry := &WhitelistEntry{}
		if err := rows.Scan(&entry.EVMAddress, &entry.Note, &entry.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

