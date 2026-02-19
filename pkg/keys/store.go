package keys

import (
	"fmt"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
)

// KeyStore provides an interface for retrieving and storing Canton keys
type KeyStore interface {
	// GetUserKey retrieves and decrypts the Canton private key for a user
	// Returns 32-byte secp256k1 private key
	GetUserKey(evmAddress string) (partyID string, privateKey []byte, err error)

	// GetUserKeyByFingerprint retrieves and decrypts the Canton private key by fingerprint
	// Returns 32-byte secp256k1 private key
	GetUserKeyByFingerprint(fingerprint string) (partyID string, privateKey []byte, err error)

	// GetUserKeyByPartyID retrieves and decrypts the Canton private key by Canton party ID.
	// Returns 32-byte secp256k1 private key. Used by Interactive Submission to sign on behalf of external parties.
	GetUserKeyByPartyID(cantonPartyID string) (privateKey []byte, err error)

	// SetUserKey encrypts and stores the Canton key for a user
	// Expects 32-byte secp256k1 private key
	SetUserKey(evmAddress, cantonPartyID string, privateKey []byte) error

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
func (s *PostgresKeyStore) GetUserKey(evmAddress string) (string, []byte, error) {
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
func (s *PostgresKeyStore) GetUserKeyByFingerprint(fingerprint string) (string, []byte, error) {
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

// GetUserKeyByPartyID retrieves and decrypts the Canton private key by Canton party ID
func (s *PostgresKeyStore) GetUserKeyByPartyID(cantonPartyID string) ([]byte, error) {
	encryptedKey, err := s.db.GetUserCantonKeyByPartyID(cantonPartyID)
	if err != nil {
		return nil, fmt.Errorf("failed to get encrypted key: %w", err)
	}
	if encryptedKey == "" {
		return nil, nil
	}

	privateKey, err := DecryptPrivateKey(encryptedKey, s.masterKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt key: %w", err)
	}

	return privateKey, nil
}

// SetUserKey encrypts and stores the Canton key for a user
func (s *PostgresKeyStore) SetUserKey(evmAddress, cantonPartyID string, privateKey []byte) error {
	if len(privateKey) != 32 {
		return fmt.Errorf("private key must be 32 bytes (secp256k1)")
	}

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
	privateKey []byte // 32-byte secp256k1 private key
}

// NewMemoryKeyStore creates a new in-memory key store (for testing)
func NewMemoryKeyStore(masterKey []byte) *MemoryKeyStore {
	return &MemoryKeyStore{
		keys:      make(map[string]*userKeyEntry),
		masterKey: masterKey,
	}
}

// GetUserKey retrieves the Canton private key for a user (from memory)
func (s *MemoryKeyStore) GetUserKey(evmAddress string) (string, []byte, error) {
	entry, ok := s.keys[evmAddress]
	if !ok {
		return "", nil, nil
	}
	return entry.partyID, entry.privateKey, nil
}

// GetUserKeyByFingerprint is not supported by MemoryKeyStore
func (s *MemoryKeyStore) GetUserKeyByFingerprint(fingerprint string) (string, []byte, error) {
	return "", nil, fmt.Errorf("GetUserKeyByFingerprint not supported by MemoryKeyStore")
}

// GetUserKeyByPartyID retrieves the Canton private key by party ID (in memory)
func (s *MemoryKeyStore) GetUserKeyByPartyID(cantonPartyID string) ([]byte, error) {
	for _, entry := range s.keys {
		if entry.partyID == cantonPartyID {
			return entry.privateKey, nil
		}
	}
	return nil, nil
}

// SetUserKey stores the Canton key for a user (in memory)
func (s *MemoryKeyStore) SetUserKey(evmAddress, cantonPartyID string, privateKey []byte) error {
	if len(privateKey) != 32 {
		return fmt.Errorf("private key must be 32 bytes (secp256k1)")
	}
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

// ResolveKeyPairByPartyID looks up a party's private key from the store and
// reconstructs a full CantonKeyPair. Returns nil, nil if the key is not found.
func ResolveKeyPairByPartyID(ks KeyStore, partyID string) (*CantonKeyPair, error) {
	privKey, err := ks.GetUserKeyByPartyID(partyID)
	if err != nil {
		return nil, fmt.Errorf("key store lookup: %w", err)
	}
	if privKey == nil {
		return nil, fmt.Errorf("no signing key found for party %s", partyID)
	}
	return CantonKeyPairFromPrivateKey(privKey)
}
