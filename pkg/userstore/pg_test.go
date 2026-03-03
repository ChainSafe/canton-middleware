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

	"github.com/shopspring/decimal"
	"github.com/uptrace/bun/driver/pgdriver"

	"github.com/chainsafe/canton-middleware/pkg/pgutil"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
	"github.com/chainsafe/canton-middleware/pkg/token"
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

func assertDecimalEqual(t *testing.T, got, want string) {
	t.Helper()

	gotDec, err := decimal.NewFromString(got)
	if err != nil {
		t.Fatalf("failed to parse got decimal %q: %v", got, err)
	}
	wantDec, err := decimal.NewFromString(want)
	if err != nil {
		t.Fatalf("failed to parse want decimal %q: %v", want, err)
	}
	if !gotDec.Equal(wantDec) {
		t.Fatalf("decimal mismatch: got %s want %s", gotDec.String(), wantDec.String())
	}
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
	u.PromptBalance = "12.5"
	u.DemoBalance = "3"
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
	if !errors.Is(err, ErrUserNotFound) {
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

func TestUserPGStore_BalanceOperations(t *testing.T) {
	ctx, s := setupStore(t)

	from := newTestUser("0x9999999999999999999999999999999999999999", "party::from", "0xabc123")
	to := newTestUser("0x8888888888888888888888888888888888888888", "party::to", "def456")
	if err := s.CreateUser(ctx, from); err != nil {
		t.Fatalf("CreateUser(from) failed: %v", err)
	}
	if err := s.CreateUser(ctx, to); err != nil {
		t.Fatalf("CreateUser(to) failed: %v", err)
	}

	if err := s.UpdateBalanceByCantonPartyID(ctx, from.CantonPartyID, "100", token.Prompt); err != nil {
		t.Fatalf("UpdateBalanceByCantonPartyID(prompt) failed: %v", err)
	}
	if err := s.UpdateBalanceByCantonPartyID(ctx, from.CantonPartyID, "7", token.Demo); err != nil {
		t.Fatalf("UpdateBalanceByCantonPartyID(demo) failed: %v", err)
	}

	updatedFrom, err := s.GetUserByCantonPartyID(ctx, from.CantonPartyID)
	if err != nil {
		t.Fatalf("GetUserByCantonPartyID(from) failed: %v", err)
	}
	assertDecimalEqual(t, updatedFrom.PromptBalance, "100")
	assertDecimalEqual(t, updatedFrom.DemoBalance, "7")

	err = s.IncrementBalanceByFingerprint(ctx, "abc123", "25.5", token.Prompt)
	if err != nil {
		t.Fatalf("IncrementBalanceByFingerprint() failed: %v", err)
	}
	updatedFrom, err = s.GetUserByCantonPartyID(ctx, from.CantonPartyID)
	if err != nil {
		t.Fatalf("GetUserByCantonPartyID(from) failed: %v", err)
	}
	assertDecimalEqual(t, updatedFrom.PromptBalance, "125.5")

	err = s.DecrementBalanceByEVMAddress(ctx, from.EVMAddress, "5.5", token.Prompt)
	if err != nil {
		t.Fatalf("DecrementBalanceByEVMAddress() failed: %v", err)
	}
	updatedFrom, err = s.GetUserByCantonPartyID(ctx, from.CantonPartyID)
	if err != nil {
		t.Fatalf("GetUserByCantonPartyID(from) failed: %v", err)
	}
	assertDecimalEqual(t, updatedFrom.PromptBalance, "120")

	err = s.TransferBalanceByFingerprint(ctx, "abc123", "0xdef456", "20", token.Prompt)
	if err != nil {
		t.Fatalf("TransferBalanceByFingerprint() failed: %v", err)
	}

	updatedFrom, err = s.GetUserByCantonPartyID(ctx, from.CantonPartyID)
	if err != nil {
		t.Fatalf("GetUserByCantonPartyID(from) failed: %v", err)
	}
	updatedTo, err := s.GetUserByCantonPartyID(ctx, to.CantonPartyID)
	if err != nil {
		t.Fatalf("GetUserByCantonPartyID(to) failed: %v", err)
	}
	assertDecimalEqual(t, updatedFrom.PromptBalance, "100")
	assertDecimalEqual(t, updatedTo.PromptBalance, "20")

	err = s.ResetBalances(ctx, token.Prompt)
	if err != nil {
		t.Fatalf("ResetBalances(prompt) failed: %v", err)
	}
	updatedFrom, err = s.GetUserByCantonPartyID(ctx, from.CantonPartyID)
	if err != nil {
		t.Fatalf("GetUserByCantonPartyID(from) failed: %v", err)
	}
	updatedTo, err = s.GetUserByCantonPartyID(ctx, to.CantonPartyID)
	if err != nil {
		t.Fatalf("GetUserByCantonPartyID(to) failed: %v", err)
	}
	assertDecimalEqual(t, updatedFrom.PromptBalance, "0")
	assertDecimalEqual(t, updatedTo.PromptBalance, "0")
	assertDecimalEqual(t, updatedFrom.DemoBalance, "7")
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
	if !errors.Is(err, ErrKeyNotFound) {
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
