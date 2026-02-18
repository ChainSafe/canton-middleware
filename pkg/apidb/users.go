package apidb

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// TokenType represents a token type for balance operations
type TokenType string

const (
	TokenPrompt TokenType = "prompt" // PROMPT (bridged) token
	TokenDemo   TokenType = "demo"   // DEMO (native) token
)

// tokenColumn returns the database column name for a token type
func tokenColumn(token TokenType) string {
	switch token {
	case TokenPrompt:
		return "prompt_balance"
	case TokenDemo:
		return "demo_balance"
	default:
		return "prompt_balance" // Default to PROMPT for safety
	}
}

// normalizeFingerprint returns both 0x-prefixed and non-prefixed versions of a fingerprint.
// This allows queries to match fingerprints stored in either format.
func normalizeFingerprint(fingerprint string) (withPrefix, withoutPrefix string) {
	if strings.HasPrefix(fingerprint, "0x") {
		return fingerprint, fingerprint[2:]
	}
	return "0x" + fingerprint, fingerprint
}

// User represents a registered EVM user mapped to a Canton party
type User struct {
	ID               int64      `json:"id"`
	EVMAddress       string     `json:"evm_address"`
	CantonParty      string     `json:"canton_party"` // Legacy: relayer party (for backward compat)
	Fingerprint      string     `json:"fingerprint"`
	MappingCID       string     `json:"mapping_cid,omitempty"`
	PromptBalance    string     `json:"prompt_balance"` // PROMPT (bridged) token balance
	DemoBalance      string     `json:"demo_balance"`   // DEMO (native) token balance
	BalanceUpdatedAt *time.Time `json:"balance_updated_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`

	// Custodial Canton key fields (for user-owned holdings)
	CantonPartyID             string     `json:"canton_party_id,omitempty"` // User's own Canton party (from AllocateParty)
	CantonPrivateKeyEncrypted string     `json:"-"`                         // Never expose in JSON
	CantonKeyCreatedAt        *time.Time `json:"canton_key_created_at,omitempty"`
}

// CreateUser creates a new user record
func (s *Store) CreateUser(user *User) error {
	query := `
		INSERT INTO users (evm_address, canton_party, fingerprint, mapping_cid, canton_party_id, canton_private_key_encrypted, canton_key_created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`
	return s.db.QueryRow(
		query,
		user.EVMAddress,
		user.CantonParty,
		user.Fingerprint,
		user.MappingCID,
		sql.NullString{String: user.CantonPartyID, Valid: user.CantonPartyID != ""},
		sql.NullString{String: user.CantonPrivateKeyEncrypted, Valid: user.CantonPrivateKeyEncrypted != ""},
		user.CantonKeyCreatedAt,
	).Scan(&user.ID, &user.CreatedAt)
}

// GetUserByEVMAddress retrieves a user by their EVM address
func (s *Store) GetUserByEVMAddress(evmAddress string) (*User, error) {
	user := &User{}
	var promptBalance sql.NullString
	var demoBalance sql.NullString
	var mappingCID sql.NullString
	var balanceUpdatedAt sql.NullTime
	var cantonPartyID sql.NullString
	var cantonPrivateKeyEncrypted sql.NullString
	var cantonKeyCreatedAt sql.NullTime
	query := `
		SELECT id, evm_address, canton_party, fingerprint, mapping_cid, prompt_balance, demo_balance, balance_updated_at, created_at,
		       canton_party_id, canton_private_key_encrypted, canton_key_created_at
		FROM users
		WHERE evm_address = $1
	`
	err := s.db.QueryRow(query, evmAddress).Scan(
		&user.ID,
		&user.EVMAddress,
		&user.CantonParty,
		&user.Fingerprint,
		&mappingCID,
		&promptBalance,
		&demoBalance,
		&balanceUpdatedAt,
		&user.CreatedAt,
		&cantonPartyID,
		&cantonPrivateKeyEncrypted,
		&cantonKeyCreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if mappingCID.Valid {
		user.MappingCID = mappingCID.String
	}
	if promptBalance.Valid {
		user.PromptBalance = promptBalance.String
	} else {
		user.PromptBalance = "0"
	}
	if demoBalance.Valid {
		user.DemoBalance = demoBalance.String
	} else {
		user.DemoBalance = "0"
	}
	if balanceUpdatedAt.Valid {
		user.BalanceUpdatedAt = &balanceUpdatedAt.Time
	}
	if cantonPartyID.Valid {
		user.CantonPartyID = cantonPartyID.String
	}
	if cantonPrivateKeyEncrypted.Valid {
		user.CantonPrivateKeyEncrypted = cantonPrivateKeyEncrypted.String
	}
	if cantonKeyCreatedAt.Valid {
		user.CantonKeyCreatedAt = &cantonKeyCreatedAt.Time
	}
	return user, nil
}

// GetUserByFingerprint retrieves a user by their fingerprint
// Handles fingerprint with or without 0x prefix
func (s *Store) GetUserByFingerprint(fingerprint string) (*User, error) {
	user := &User{}
	var promptBalance sql.NullString
	var demoBalance sql.NullString
	var mappingCID sql.NullString
	var balanceUpdatedAt sql.NullTime
	var cantonPartyID sql.NullString
	var cantonPrivateKeyEncrypted sql.NullString
	var cantonKeyCreatedAt sql.NullTime

	withPrefix, withoutPrefix := normalizeFingerprint(fingerprint)

	query := `
		SELECT id, evm_address, canton_party, fingerprint, mapping_cid, prompt_balance, demo_balance, balance_updated_at, created_at,
		       canton_party_id, canton_private_key_encrypted, canton_key_created_at
		FROM users
		WHERE fingerprint = $1 OR fingerprint = $2
	`
	err := s.db.QueryRow(query, withPrefix, withoutPrefix).Scan(
		&user.ID,
		&user.EVMAddress,
		&user.CantonParty,
		&user.Fingerprint,
		&mappingCID,
		&promptBalance,
		&demoBalance,
		&balanceUpdatedAt,
		&user.CreatedAt,
		&cantonPartyID,
		&cantonPrivateKeyEncrypted,
		&cantonKeyCreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if mappingCID.Valid {
		user.MappingCID = mappingCID.String
	}
	if promptBalance.Valid {
		user.PromptBalance = promptBalance.String
	} else {
		user.PromptBalance = "0"
	}
	if demoBalance.Valid {
		user.DemoBalance = demoBalance.String
	} else {
		user.DemoBalance = "0"
	}
	if balanceUpdatedAt.Valid {
		user.BalanceUpdatedAt = &balanceUpdatedAt.Time
	}
	if cantonPartyID.Valid {
		user.CantonPartyID = cantonPartyID.String
	}
	if cantonPrivateKeyEncrypted.Valid {
		user.CantonPrivateKeyEncrypted = cantonPrivateKeyEncrypted.String
	}
	if cantonKeyCreatedAt.Valid {
		user.CantonKeyCreatedAt = &cantonKeyCreatedAt.Time
	}
	return user, nil
}

// UpdateUserMappingCID updates the Canton FingerprintMapping contract ID for a user
func (s *Store) UpdateUserMappingCID(evmAddress, mappingCID string) error {
	query := `
		UPDATE users
		SET mapping_cid = $1
		WHERE evm_address = $2
	`
	result, err := s.db.Exec(query, mappingCID, evmAddress)
	if err != nil {
		return fmt.Errorf("failed to update mapping CID: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("user not found: %s", evmAddress)
	}
	return nil
}

// DeleteUser removes a user by EVM address (used for cleanup on failed registration)
func (s *Store) DeleteUser(evmAddress string) error {
	query := `DELETE FROM users WHERE evm_address = $1`
	_, err := s.db.Exec(query, evmAddress)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}

// UserExists checks if a user with the given EVM address exists
func (s *Store) UserExists(evmAddress string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM users WHERE evm_address = $1)`
	err := s.db.QueryRow(query, evmAddress).Scan(&exists)
	return exists, err
}

// GetUserByCantonPartyID retrieves a user by their Canton party ID
func (s *Store) GetUserByCantonPartyID(cantonPartyID string) (*User, error) {
	user := &User{}
	var promptBalance sql.NullString
	var demoBalance sql.NullString
	var mappingCID sql.NullString
	var balanceUpdatedAt sql.NullTime
	var cantonPartyIDNull sql.NullString
	var cantonPrivateKeyEncrypted sql.NullString
	var cantonKeyCreatedAt sql.NullTime
	query := `
		SELECT id, evm_address, canton_party, fingerprint, mapping_cid, prompt_balance, demo_balance, balance_updated_at, created_at,
		       canton_party_id, canton_private_key_encrypted, canton_key_created_at
		FROM users
		WHERE canton_party_id = $1
	`
	err := s.db.QueryRow(query, cantonPartyID).Scan(
		&user.ID,
		&user.EVMAddress,
		&user.CantonParty,
		&user.Fingerprint,
		&mappingCID,
		&promptBalance,
		&demoBalance,
		&balanceUpdatedAt,
		&user.CreatedAt,
		&cantonPartyIDNull,
		&cantonPrivateKeyEncrypted,
		&cantonKeyCreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if mappingCID.Valid {
		user.MappingCID = mappingCID.String
	}
	if promptBalance.Valid {
		user.PromptBalance = promptBalance.String
	} else {
		user.PromptBalance = "0"
	}
	if demoBalance.Valid {
		user.DemoBalance = demoBalance.String
	} else {
		user.DemoBalance = "0"
	}
	if balanceUpdatedAt.Valid {
		user.BalanceUpdatedAt = &balanceUpdatedAt.Time
	}
	if cantonPartyIDNull.Valid {
		user.CantonPartyID = cantonPartyIDNull.String
	}
	if cantonPrivateKeyEncrypted.Valid {
		user.CantonPrivateKeyEncrypted = cantonPrivateKeyEncrypted.String
	}
	if cantonKeyCreatedAt.Valid {
		user.CantonKeyCreatedAt = &cantonKeyCreatedAt.Time
	}
	return user, nil
}

// =============================================================================
// Generic Token Balance Cache Methods
// =============================================================================

// GetBalance returns the cached balance for a user by EVM address and token type
func (s *Store) GetBalance(evmAddress string, token TokenType) (string, error) {
	var balance sql.NullString
	col := tokenColumn(token)
	query := fmt.Sprintf(`SELECT %s FROM users WHERE evm_address = $1`, col)
	err := s.db.QueryRow(query, evmAddress).Scan(&balance)
	if err == sql.ErrNoRows {
		return "0", nil
	}
	if err != nil {
		return "0", fmt.Errorf("failed to get %s balance: %w", token, err)
	}
	if !balance.Valid {
		return "0", nil
	}
	return balance.String, nil
}

// GetBalanceByFingerprint returns the cached balance for a user by fingerprint and token type
// Handles fingerprint with or without 0x prefix
func (s *Store) GetBalanceByFingerprint(fingerprint string, token TokenType) (string, error) {
	withPrefix, withoutPrefix := normalizeFingerprint(fingerprint)

	var balance sql.NullString
	col := tokenColumn(token)
	query := fmt.Sprintf(`SELECT %s FROM users WHERE fingerprint = $1 OR fingerprint = $2`, col)
	err := s.db.QueryRow(query, withPrefix, withoutPrefix).Scan(&balance)
	if err == sql.ErrNoRows {
		return "0", nil
	}
	if err != nil {
		return "0", fmt.Errorf("failed to get %s balance: %w", token, err)
	}
	if !balance.Valid {
		return "0", nil
	}
	return balance.String, nil
}

// UpdateBalance sets the balance for a user (full replacement)
func (s *Store) UpdateBalance(evmAddress, newBalance string, token TokenType) error {
	col := tokenColumn(token)
	query := fmt.Sprintf(`
		UPDATE users
		SET %s = $1, balance_updated_at = NOW()
		WHERE evm_address = $2
	`, col)
	result, err := s.db.Exec(query, newBalance, evmAddress)
	if err != nil {
		return fmt.Errorf("failed to update %s balance: %w", token, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("user not found: %s", evmAddress)
	}
	return nil
}

// UpdateBalanceByFingerprint sets the balance for a user by fingerprint
// Handles fingerprint with or without 0x prefix
func (s *Store) UpdateBalanceByFingerprint(fingerprint, newBalance string, token TokenType) error {
	withPrefix, withoutPrefix := normalizeFingerprint(fingerprint)

	col := tokenColumn(token)
	query := fmt.Sprintf(`
		UPDATE users
		SET %s = $1, balance_updated_at = NOW()
		WHERE fingerprint = $2 OR fingerprint = $3
	`, col)
	result, err := s.db.Exec(query, newBalance, withPrefix, withoutPrefix)
	if err != nil {
		return fmt.Errorf("failed to update %s balance: %w", token, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("user not found for fingerprint: %s", fingerprint)
	}
	return nil
}

// UpdateBalanceByCantonPartyID sets the balance for a user by their Canton party ID
func (s *Store) UpdateBalanceByCantonPartyID(cantonPartyID, newBalance string, token TokenType) error {
	col := tokenColumn(token)
	query := fmt.Sprintf(`
		UPDATE users
		SET %s = $1, balance_updated_at = NOW()
		WHERE canton_party_id = $2
	`, col)
	result, err := s.db.Exec(query, newBalance, cantonPartyID)
	if err != nil {
		return fmt.Errorf("failed to update %s balance: %w", token, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		// Not an error - user may not be registered
		return nil
	}
	return nil
}

// IncrementBalance adds amount to user's balance atomically
func (s *Store) IncrementBalance(evmAddress, amount string, token TokenType) error {
	col := tokenColumn(token)
	query := fmt.Sprintf(`
		UPDATE users
		SET %s = COALESCE(%s, 0) + $1::DECIMAL, balance_updated_at = NOW()
		WHERE evm_address = $2
	`, col, col)
	result, err := s.db.Exec(query, amount, evmAddress)
	if err != nil {
		return fmt.Errorf("failed to increment %s balance: %w", token, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("user not found: %s", evmAddress)
	}
	return nil
}

// IncrementBalanceByFingerprint adds amount to user's balance atomically by fingerprint
// Handles fingerprint with or without 0x prefix
func (s *Store) IncrementBalanceByFingerprint(fingerprint, amount string, token TokenType) error {
	withPrefix, withoutPrefix := normalizeFingerprint(fingerprint)

	col := tokenColumn(token)
	query := fmt.Sprintf(`
		UPDATE users
		SET %s = COALESCE(%s, 0) + $1::DECIMAL, balance_updated_at = NOW()
		WHERE fingerprint = $2 OR fingerprint = $3
	`, col, col)
	result, err := s.db.Exec(query, amount, withPrefix, withoutPrefix)
	if err != nil {
		return fmt.Errorf("failed to increment %s balance: %w", token, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("user not found for fingerprint: %s", fingerprint)
	}
	return nil
}

// DecrementBalance subtracts amount from user's balance atomically
func (s *Store) DecrementBalance(evmAddress, amount string, token TokenType) error {
	col := tokenColumn(token)
	query := fmt.Sprintf(`
		UPDATE users
		SET %s = COALESCE(%s, 0) - $1::DECIMAL, balance_updated_at = NOW()
		WHERE evm_address = $2
	`, col, col)
	result, err := s.db.Exec(query, amount, evmAddress)
	if err != nil {
		return fmt.Errorf("failed to decrement %s balance: %w", token, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("user not found: %s", evmAddress)
	}
	return nil
}

// DecrementBalanceByFingerprint subtracts amount from user's balance atomically by fingerprint
// Handles fingerprint with or without 0x prefix
func (s *Store) DecrementBalanceByFingerprint(fingerprint, amount string, token TokenType) error {
	withPrefix, withoutPrefix := normalizeFingerprint(fingerprint)

	col := tokenColumn(token)
	query := fmt.Sprintf(`
		UPDATE users
		SET %s = COALESCE(%s, 0) - $1::DECIMAL, balance_updated_at = NOW()
		WHERE fingerprint = $2 OR fingerprint = $3
	`, col, col)
	result, err := s.db.Exec(query, amount, withPrefix, withoutPrefix)
	if err != nil {
		return fmt.Errorf("failed to decrement %s balance: %w", token, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("user not found for fingerprint: %s", fingerprint)
	}
	return nil
}

// GetAllUsers returns all registered users with their balances
func (s *Store) GetAllUsers() ([]*User, error) {
	query := `
		SELECT id, evm_address, canton_party, fingerprint, mapping_cid, prompt_balance, demo_balance, balance_updated_at, created_at, canton_party_id
		FROM users
		ORDER BY id
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		user := &User{}
		var promptBalance sql.NullString
		var demoBalance sql.NullString
		var balanceUpdatedAt sql.NullTime
		var cantonPartyID sql.NullString
		err := rows.Scan(
			&user.ID,
			&user.EVMAddress,
			&user.CantonParty,
			&user.Fingerprint,
			&user.MappingCID,
			&promptBalance,
			&demoBalance,
			&balanceUpdatedAt,
			&user.CreatedAt,
			&cantonPartyID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		if promptBalance.Valid {
			user.PromptBalance = promptBalance.String
		} else {
			user.PromptBalance = "0"
		}
		if demoBalance.Valid {
			user.DemoBalance = demoBalance.String
		} else {
			user.DemoBalance = "0"
		}
		if balanceUpdatedAt.Valid {
			user.BalanceUpdatedAt = &balanceUpdatedAt.Time
		}
		if cantonPartyID.Valid {
			user.CantonPartyID = cantonPartyID.String
		}
		users = append(users, user)
	}
	return users, nil
}

// TransferBalanceByFingerprint atomically transfers balance from one user to another.
// Both the decrement and increment are performed in a single database transaction
// to ensure consistency of the balance cache.
// Handles fingerprints with or without 0x prefix.
func (s *Store) TransferBalanceByFingerprint(fromFingerprint, toFingerprint, amount string, token TokenType) error {
	fromWithPrefix, fromWithoutPrefix := normalizeFingerprint(fromFingerprint)
	toWithPrefix, toWithoutPrefix := normalizeFingerprint(toFingerprint)

	col := tokenColumn(token)

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Decrement sender's balance
	decrementQuery := fmt.Sprintf(`
		UPDATE users
		SET %s = COALESCE(%s, 0) - $1::DECIMAL, balance_updated_at = NOW()
		WHERE fingerprint = $2 OR fingerprint = $3
	`, col, col)
	result, err := tx.Exec(decrementQuery, amount, fromWithPrefix, fromWithoutPrefix)
	if err != nil {
		return fmt.Errorf("failed to decrement sender %s balance: %w", token, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected for sender: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("sender not found for fingerprint: %s", fromFingerprint)
	}

	// Increment recipient's balance
	incrementQuery := fmt.Sprintf(`
		UPDATE users
		SET %s = COALESCE(%s, 0) + $1::DECIMAL, balance_updated_at = NOW()
		WHERE fingerprint = $2 OR fingerprint = $3
	`, col, col)
	result, err = tx.Exec(incrementQuery, amount, toWithPrefix, toWithoutPrefix)
	if err != nil {
		return fmt.Errorf("failed to increment recipient %s balance: %w", token, err)
	}
	rows, err = result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected for recipient: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("recipient not found for fingerprint: %s", toFingerprint)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transfer transaction: %w", err)
	}

	return nil
}

// ResetBalances resets all user balances to 0 for a specific token (used before full reconciliation)
func (s *Store) ResetBalances(token TokenType) error {
	col := tokenColumn(token)
	query := fmt.Sprintf(`UPDATE users SET %s = 0, balance_updated_at = NOW()`, col)
	_, err := s.db.Exec(query)
	return err
}

// =============================================================================
// Custodial Canton Key Methods
// =============================================================================

// SetUserCantonKey stores the user's Canton party ID and encrypted private key
func (s *Store) SetUserCantonKey(evmAddress, cantonPartyID, encryptedKey string) error {
	query := `
		UPDATE users
		SET canton_party_id = $1,
		    canton_private_key_encrypted = $2,
		    canton_key_created_at = NOW()
		WHERE evm_address = $3
	`
	result, err := s.db.Exec(query, cantonPartyID, encryptedKey, evmAddress)
	if err != nil {
		return fmt.Errorf("failed to set Canton key: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("user not found: %s", evmAddress)
	}
	return nil
}

// GetUserCantonKey retrieves the encrypted Canton private key for a user
// Returns empty string if user has no Canton key
func (s *Store) GetUserCantonKey(evmAddress string) (cantonPartyID, encryptedKey string, err error) {
	var partyID sql.NullString
	var key sql.NullString
	query := `
		SELECT canton_party_id, canton_private_key_encrypted
		FROM users
		WHERE evm_address = $1
	`
	err = s.db.QueryRow(query, evmAddress).Scan(&partyID, &key)
	if err == sql.ErrNoRows {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("failed to get Canton key: %w", err)
	}
	if partyID.Valid {
		cantonPartyID = partyID.String
	}
	if key.Valid {
		encryptedKey = key.String
	}
	return cantonPartyID, encryptedKey, nil
}

// GetUserCantonKeyByFingerprint retrieves the encrypted Canton private key for a user by fingerprint
func (s *Store) GetUserCantonKeyByFingerprint(fingerprint string) (cantonPartyID, encryptedKey string, err error) {
	withPrefix, withoutPrefix := normalizeFingerprint(fingerprint)

	var partyID sql.NullString
	var key sql.NullString
	query := `
		SELECT canton_party_id, canton_private_key_encrypted
		FROM users
		WHERE fingerprint = $1 OR fingerprint = $2
	`
	err = s.db.QueryRow(query, withPrefix, withoutPrefix).Scan(&partyID, &key)
	if err == sql.ErrNoRows {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("failed to get Canton key: %w", err)
	}
	if partyID.Valid {
		cantonPartyID = partyID.String
	}
	if key.Valid {
		encryptedKey = key.String
	}
	return cantonPartyID, encryptedKey, nil
}

// GetUserCantonKeyByPartyID retrieves the encrypted Canton private key for a user by party ID
func (s *Store) GetUserCantonKeyByPartyID(cantonPartyID string) (encryptedKey string, err error) {
	var key sql.NullString
	query := `
		SELECT canton_private_key_encrypted
		FROM users
		WHERE canton_party_id = $1
	`
	err = s.db.QueryRow(query, cantonPartyID).Scan(&key)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get Canton key by party ID: %w", err)
	}
	if key.Valid {
		encryptedKey = key.String
	}
	return encryptedKey, nil
}

// HasCantonKey checks if a user has a Canton custodial key
func (s *Store) HasCantonKey(evmAddress string) (bool, error) {
	var count int
	query := `
		SELECT COUNT(*) FROM users
		WHERE evm_address = $1 AND canton_party_id IS NOT NULL AND canton_private_key_encrypted IS NOT NULL
	`
	err := s.db.QueryRow(query, evmAddress).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check Canton key: %w", err)
	}
	return count > 0, nil
}

// GetUsersWithoutCantonKey returns all users that don't have Canton keys (for migration)
func (s *Store) GetUsersWithoutCantonKey() ([]*User, error) {
	query := `
		SELECT id, evm_address, canton_party, fingerprint, mapping_cid, prompt_balance, demo_balance, balance_updated_at, created_at,
		       canton_party_id, canton_private_key_encrypted, canton_key_created_at
		FROM users
		WHERE canton_party_id IS NULL OR canton_private_key_encrypted IS NULL
		ORDER BY id
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query users without Canton key: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		user := &User{}
		var promptBalance sql.NullString
		var demoBalance sql.NullString
		var mappingCID sql.NullString
		var balanceUpdatedAt sql.NullTime
		var cantonPartyID sql.NullString
		var cantonPrivateKeyEncrypted sql.NullString
		var cantonKeyCreatedAt sql.NullTime

		err := rows.Scan(
			&user.ID,
			&user.EVMAddress,
			&user.CantonParty,
			&user.Fingerprint,
			&mappingCID,
			&promptBalance,
			&demoBalance,
			&balanceUpdatedAt,
			&user.CreatedAt,
			&cantonPartyID,
			&cantonPrivateKeyEncrypted,
			&cantonKeyCreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		if mappingCID.Valid {
			user.MappingCID = mappingCID.String
		}
		if promptBalance.Valid {
			user.PromptBalance = promptBalance.String
		} else {
			user.PromptBalance = "0"
		}
		if demoBalance.Valid {
			user.DemoBalance = demoBalance.String
		} else {
			user.DemoBalance = "0"
		}
		if balanceUpdatedAt.Valid {
			user.BalanceUpdatedAt = &balanceUpdatedAt.Time
		}
		// Canton key fields will be NULL for users without keys
		users = append(users, user)
	}
	return users, nil
}
