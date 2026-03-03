package userstore

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uptrace/bun/driver/pgdriver"

	"github.com/chainsafe/canton-middleware/pkg/pgutil"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
	"github.com/chainsafe/canton-middleware/pkg/user"
)

const plainKey = "plain-key"

func setupStore(t *testing.T) (context.Context, *pgStore) {
	t.Helper()
	requireDockerAccess(t)

	ctx := context.Background()
	db, cleanup := pgutil.SetupTestDB(t)
	t.Cleanup(cleanup)

	if err := mghelper.CreateSchema(ctx, db, &UserDao{}, &WhitelistDao{}); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	return ctx, NewStore(db)
}

func requireDockerAccess(t *testing.T) {
	t.Helper()

	candidates := []string{
		"/var/run/docker.sock",
		filepath.Join(os.Getenv("HOME"), ".docker/run/docker.sock"),
	}

	for _, sock := range candidates {
		if sock == "" {
			continue
		}
		if _, err := os.Stat(sock); err != nil {
			continue
		}
		conn, err := (&net.Dialer{}).DialContext(context.Background(), "unix", sock)
		if err == nil {
			_ = conn.Close()
			return
		}
	}

	t.Skip("docker daemon socket is not accessible; skipping testcontainer-backed userstore tests")
}

func newTestUser(evmAddress, partyID, fingerprint string) *user.User {
	return user.New(evmAddress, partyID, fingerprint, "mapping-cid", "encrypted-key")
}

func TestUserPGStore_CreateUserAndConstraints(t *testing.T) {
	ctx, s := setupStore(t)

	u := newTestUser("0x1111111111111111111111111111111111111111", "party::alice", "0xaaa")
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser() failed: %v", err)
	}

	exists, err := s.UserExists(ctx, u.EVMAddress)
	if err != nil {
		t.Fatalf("UserExists() failed: %v", err)
	}
	if !exists {
		t.Fatalf("expected user to exist")
	}

	dup := newTestUser(u.EVMAddress, "party::other", "0xbbb")
	err = s.CreateUser(ctx, dup)
	if err == nil {
		t.Fatalf("expected duplicate EVM address to fail")
	}
	var pgErr pgdriver.Error
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected postgres error type, got: %v", err)
	}
	if !pgErr.IntegrityViolation() {
		t.Fatalf("expected unique violation SQLSTATE=23505, got %s (%v)", pgErr.Field('C'), err)
	}

	tooLongEVM := newTestUser("0x"+strings.Repeat("a", 41), "party::long", "0xccc")
	err = s.CreateUser(ctx, tooLongEVM)
	if err == nil {
		t.Fatalf("expected oversized evm_address to fail")
	}
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected postgres error type, got: %v", err)
	}
	if pgErr.Field('C') != "22001" {
		t.Fatalf("expected value-too-long SQLSTATE=22001, got %s (%v)", pgErr.Field('C'), err)
	}

	tooLongFingerprint := newTestUser("0x2222222222222222222222222222222222222222", "party::fp", strings.Repeat("f", 129))
	err = s.CreateUser(ctx, tooLongFingerprint)
	if err == nil {
		t.Fatalf("expected oversized fingerprint to fail")
	}
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected postgres error type, got: %v", err)
	}
	if pgErr.Field('C') != "22001" {
		t.Fatalf("expected value-too-long SQLSTATE=22001, got %s (%v)", pgErr.Field('C'), err)
	}
}

func TestUserPGStore_GetUserLookupsAndDelete(t *testing.T) {
	ctx, s := setupStore(t)

	u := newTestUser("0x3333333333333333333333333333333333333333", "party::carol", "0xf1")
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser() failed: %v", err)
	}

	byEVM, err := s.GetUserByEVMAddress(ctx, u.EVMAddress)
	if err != nil {
		t.Fatalf("GetUserByEVMAddress() failed: %v", err)
	}
	if byEVM.CantonPartyID != u.CantonPartyID {
		t.Fatalf("party mismatch: got %s want %s", byEVM.CantonPartyID, u.CantonPartyID)
	}

	byParty, err := s.GetUserByCantonPartyID(ctx, u.CantonPartyID)
	if err != nil {
		t.Fatalf("GetUserByCantonPartyID() failed: %v", err)
	}
	if byParty.EVMAddress != u.EVMAddress {
		t.Fatalf("evm mismatch: got %s want %s", byParty.EVMAddress, u.EVMAddress)
	}

	byFingerprint, err := s.GetUserByFingerprint(ctx, u.Fingerprint)
	if err != nil {
		t.Fatalf("GetUserByFingerprint() failed: %v", err)
	}
	if byFingerprint.Fingerprint != u.Fingerprint {
		t.Fatalf("fingerprint mismatch: got %s want %s", byFingerprint.Fingerprint, u.Fingerprint)
	}

	_, err = s.GetUserByEVMAddress(ctx, "0x4444444444444444444444444444444444444444")
	if !errors.Is(err, user.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}

	if err := s.DeleteUser(ctx, u.EVMAddress); err != nil {
		t.Fatalf("DeleteUser() failed: %v", err)
	}
	if err := s.DeleteUser(ctx, u.EVMAddress); err != nil {
		t.Fatalf("DeleteUser() should be idempotent, got: %v", err)
	}
}

func TestUserPGStore_ListUsers(t *testing.T) {
	ctx, s := setupStore(t)

	users := []*user.User{
		newTestUser("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "party::u1", "0x1"),
		newTestUser("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "party::u2", "0x2"),
	}

	for _, u := range users {
		if err := s.CreateUser(ctx, u); err != nil {
			t.Fatalf("CreateUser() failed: %v", err)
		}
	}

	got, err := s.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers() failed: %v", err)
	}
	if len(got) != len(users) {
		t.Fatalf("unexpected user count: got %d want %d", len(got), len(users))
	}
}

func TestUserPGStore_IsWhitelisted(t *testing.T) {
	ctx, s := setupStore(t)

	addr := "0x1234567890123456789012345678901234567890"
	ok, err := s.IsWhitelisted(ctx, addr)
	if err != nil {
		t.Fatalf("IsWhitelisted() failed: %v", err)
	}
	if ok {
		t.Fatalf("expected address to be non-whitelisted")
	}

	note := "test"
	entry := &WhitelistDao{
		EVMAddress: addr,
		Note:       &note,
	}
	if _, err = s.db.NewInsert().Model(entry).Exec(ctx); err != nil {
		t.Fatalf("failed to insert whitelist entry: %v", err)
	}

	ok, err = s.IsWhitelisted(ctx, addr)
	if err != nil {
		t.Fatalf("IsWhitelisted() failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected address to be whitelisted")
	}
}

func TestUserPGStore_GetUserKeyMethods(t *testing.T) {
	ctx, s := setupStore(t)

	withKey := newTestUser("0x7777777777777777777777777777777777777777", "party::k1", "0xk1")
	withKey.CantonPrivateKeyEncrypted = "ciphertext"
	if err := s.CreateUser(ctx, withKey); err != nil {
		t.Fatalf("CreateUser(withKey) failed: %v", err)
	}

	withoutKey := newTestUser("0x6666666666666666666666666666666666666666", "party::k2", "0xk2")
	withoutKey.CantonPrivateKeyEncrypted = ""
	if err := s.CreateUser(ctx, withoutKey); err != nil {
		t.Fatalf("CreateUser(withoutKey) failed: %v", err)
	}

	decryptor := func(encrypted string) ([]byte, error) {
		if encrypted != "ciphertext" {
			return nil, fmt.Errorf("unexpected encrypted value: %s", encrypted)
		}
		return []byte(plainKey), nil
	}

	key, err := s.GetUserKeyByCantonPartyID(ctx, decryptor, withKey.CantonPartyID)
	if err != nil {
		t.Fatalf("GetUserKeyByCantonPartyID() failed: %v", err)
	}
	if string(key) != plainKey {
		t.Fatalf("unexpected decrypted key: %q", string(key))
	}

	key, err = s.GetUserKeyByEVMAddress(ctx, decryptor, withKey.EVMAddress)
	if err != nil {
		t.Fatalf("GetUserKeyByEVMAddress() failed: %v", err)
	}
	if string(key) != plainKey {
		t.Fatalf("unexpected decrypted key: %q", string(key))
	}

	key, err = s.GetUserKeyByFingerprint(ctx, decryptor, withKey.Fingerprint)
	if err != nil {
		t.Fatalf("GetUserKeyByFingerprint() failed: %v", err)
	}
	if string(key) != plainKey {
		t.Fatalf("unexpected decrypted key: %q", string(key))
	}

	key, err = s.GetUserKeyByCantonPartyID(ctx, decryptor, withoutKey.CantonPartyID)
	if err != nil {
		t.Fatalf("GetUserKeyByCantonPartyID(withoutKey) failed: %v", err)
	}
	if key != nil {
		t.Fatalf("expected nil key for empty encrypted value, got %q", string(key))
	}

	_, err = s.GetUserKeyByCantonPartyID(ctx, decryptor, "party::missing")
	if !errors.Is(err, user.ErrKeyNotFound) {
		t.Fatalf("expected ErrKeyNotFound, got %v", err)
	}

	decryptErr := errors.New("decrypt failed")
	badDecryptor := func(string) ([]byte, error) {
		return nil, decryptErr
	}
	_, err = s.GetUserKeyByCantonPartyID(ctx, badDecryptor, withKey.CantonPartyID)
	if err == nil {
		t.Fatalf("expected decrypt error")
	}
	if !errors.Is(err, decryptErr) {
		t.Fatalf("expected decrypt error to be wrapped with errors.Is, got %v", err)
	}
}
