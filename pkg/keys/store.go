package keys

import (
	"crypto/ed25519"
	"fmt"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
)

// KeyStore provides an interface for retrieving and storing Canton keys
type KeyStore interface {
	// GetUserKey retrieves and decrypts the Canton private key for a user
	GetUserKey(evmAddress string) (partyID string, privateKey ed25519.PrivateKey, err error)

	// GetUserKeyByFingerprint retrieves and decrypts the Canton private key by fingerprint
	GetUserKeyByFingerprint(fingerprint string) (partyID string, privateKey ed25519.PrivateKey, err error)

	// SetUserKey encrypts and stores the Canton key for a user
	SetUserKey(evmAddress, cantonPartyID string, privateKey ed25519.PrivateKey) error

	// HasUserKey checks if a user has a Canton key
	HasUserKey(evmAddress string) (bool, error)
}

// PostgresKeyStore implements KeyStore using PostgreSQL via apidb.Store
type PostgresKeyStore struct {
	db        *apidb.Store
	masterKey []byte
}

// NewPostgresKeyStore creates a new PostgresKeyStore
func NewPostgresKeyStore(db *apidb.Store, masterKey []byte) (*PostgresKeyStore, error) {
	if len(masterKey) != 32 {
		return nil, fmt.Errorf("master key must be 32 bytes (AES-256)")
	}
	return &PostgresKeyStore{
		db:        db,
		masterKey: masterKey,
	}, nil
}

// GetUserKey retrieves and decrypts the Canton private key for a user
func (s *PostgresKeyStore) GetUserKey(evmAddress string) (string, ed25519.PrivateKey, error) {
	partyID, encryptedKey, err := s.db.GetUserCantonKey(evmAddress)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get encrypted key: %w", err)
	}
	if encryptedKey == "" {
		return "", nil, nil // User doesn't have a Canton key yet
	}

	privateKey, err := DecryptPrivateKey(encryptedKey, s.masterKey)
	if err != nil {
		return "", nil, fmt.Errorf("failed to decrypt key: %w", err)
	}

	return partyID, privateKey, nil
}

// GetUserKeyByFingerprint retrieves and decrypts the Canton private key by fingerprint
func (s *PostgresKeyStore) GetUserKeyByFingerprint(fingerprint string) (string, ed25519.PrivateKey, error) {
	partyID, encryptedKey, err := s.db.GetUserCantonKeyByFingerprint(fingerprint)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get encrypted key: %w", err)
	}
	if encryptedKey == "" {
		return "", nil, nil // User doesn't have a Canton key yet
	}

	privateKey, err := DecryptPrivateKey(encryptedKey, s.masterKey)
	if err != nil {
		return "", nil, fmt.Errorf("failed to decrypt key: %w", err)
	}

	return partyID, privateKey, nil
}

// SetUserKey encrypts and stores the Canton key for a user
func (s *PostgresKeyStore) SetUserKey(evmAddress, cantonPartyID string, privateKey ed25519.PrivateKey) error {
	encryptedKey, err := EncryptPrivateKey(privateKey, s.masterKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt key: %w", err)
	}

	if err := s.db.SetUserCantonKey(evmAddress, cantonPartyID, encryptedKey); err != nil {
		return fmt.Errorf("failed to store key: %w", err)
	}

	return nil
}

// HasUserKey checks if a user has a Canton key
func (s *PostgresKeyStore) HasUserKey(evmAddress string) (bool, error) {
	return s.db.HasCantonKey(evmAddress)
}

// MemoryKeyStore implements KeyStore using in-memory storage (for testing)
type MemoryKeyStore struct {
	keys      map[string]*userKeyEntry
	masterKey []byte
}

type userKeyEntry struct {
	partyID    string
	privateKey ed25519.PrivateKey
}

// NewMemoryKeyStore creates a new in-memory key store (for testing)
func NewMemoryKeyStore(masterKey []byte) *MemoryKeyStore {
	return &MemoryKeyStore{
		keys:      make(map[string]*userKeyEntry),
		masterKey: masterKey,
	}
}

// GetUserKey retrieves the Canton private key for a user (from memory)
func (s *MemoryKeyStore) GetUserKey(evmAddress string) (string, ed25519.PrivateKey, error) {
	entry, ok := s.keys[evmAddress]
	if !ok {
		return "", nil, nil
	}
	return entry.partyID, entry.privateKey, nil
}

// GetUserKeyByFingerprint is not supported by MemoryKeyStore
func (s *MemoryKeyStore) GetUserKeyByFingerprint(fingerprint string) (string, ed25519.PrivateKey, error) {
	return "", nil, fmt.Errorf("GetUserKeyByFingerprint not supported by MemoryKeyStore")
}

// SetUserKey stores the Canton key for a user (in memory)
func (s *MemoryKeyStore) SetUserKey(evmAddress, cantonPartyID string, privateKey ed25519.PrivateKey) error {
	s.keys[evmAddress] = &userKeyEntry{
		partyID:    cantonPartyID,
		privateKey: privateKey,
	}
	return nil
}

// HasUserKey checks if a user has a Canton key (in memory)
func (s *MemoryKeyStore) HasUserKey(evmAddress string) (bool, error) {
	_, ok := s.keys[evmAddress]
	return ok, nil
}
