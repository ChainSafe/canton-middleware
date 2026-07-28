// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/chainsafe/canton-middleware/pkg/auth/jwt"
	"github.com/chainsafe/canton-middleware/pkg/auth/service/mocks"
	"github.com/chainsafe/canton-middleware/pkg/user"
)

const (
	loginMessage = "localhost wants you to sign in..."
	loginSig     = "0xsignature"
	loginNonce   = "nonce0001"
)

// testAddr is an arbitrary EVM address; loginAddr is its checksummed form, which is
// what the service normalizes to before the store lookup and token issuance.
var (
	testAddr  = common.HexToAddress("0x00000000000000000000000000000000000000ff")
	loginAddr = auth.NormalizeAddress(testAddr.Hex())
)

func newLoginDeps(t *testing.T) (*mocks.Verifier, *mocks.Issuer, *mocks.NonceStore, *mocks.UserLookup) {
	t.Helper()
	return mocks.NewVerifier(t), mocks.NewIssuer(t), mocks.NewNonceStore(t), mocks.NewUserLookup(t)
}

func TestLogin_Success(t *testing.T) {
	v, iss, n, u := newLoginDeps(t)
	exp := time.Unix(1_700_000_000, 0)

	v.EXPECT().Verify(loginMessage, loginSig).Return(testAddr, loginNonce, nil)
	n.EXPECT().Consume(loginNonce).Return(true)
	u.EXPECT().GetUserByEVMAddress(mock.Anything, loginAddr).
		Return(&user.User{EVMAddress: loginAddr, CantonPartyID: "party::abc"}, nil)
	iss.EXPECT().Issue(loginAddr, "party::abc").Return("the-token", exp, nil)

	res, err := New(v, iss, n, u).Login(context.Background(), loginMessage, loginSig)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res.Token != "the-token" {
		t.Fatalf("token = %q, want the-token", res.Token)
	}
	if res.ExpiresAt != exp.Unix() {
		t.Fatalf("expires_at = %d, want %d", res.ExpiresAt, exp.Unix())
	}
}

func TestLogin_InvalidSignature(t *testing.T) {
	v, iss, n, u := newLoginDeps(t)
	// Verification fails; nothing else must be called — in particular the nonce must
	// not be consumed on a bad signature (the mocks fail the test on any extra call).
	v.EXPECT().Verify(loginMessage, loginSig).Return(common.Address{}, "", errors.New("bad signature"))

	_, err := New(v, iss, n, u).Login(context.Background(), loginMessage, loginSig)
	requireUnauthorized(t, err)
}

func TestLogin_NonceRejected(t *testing.T) {
	v, iss, n, u := newLoginDeps(t)
	v.EXPECT().Verify(loginMessage, loginSig).Return(testAddr, loginNonce, nil)
	n.EXPECT().Consume(loginNonce).Return(false) // reused / expired / unknown

	_, err := New(v, iss, n, u).Login(context.Background(), loginMessage, loginSig)
	requireUnauthorized(t, err)
}

func TestLogin_UnregisteredAddress(t *testing.T) {
	v, iss, n, u := newLoginDeps(t)
	v.EXPECT().Verify(loginMessage, loginSig).Return(testAddr, loginNonce, nil)
	n.EXPECT().Consume(loginNonce).Return(true)
	u.EXPECT().GetUserByEVMAddress(mock.Anything, loginAddr).Return(nil, user.ErrUserNotFound)

	_, err := New(v, iss, n, u).Login(context.Background(), loginMessage, loginSig)
	requireUnauthorized(t, err)
}

func TestLogin_MissingCantonParty(t *testing.T) {
	v, iss, n, u := newLoginDeps(t)
	v.EXPECT().Verify(loginMessage, loginSig).Return(testAddr, loginNonce, nil)
	n.EXPECT().Consume(loginNonce).Return(true)
	u.EXPECT().GetUserByEVMAddress(mock.Anything, loginAddr).
		Return(&user.User{EVMAddress: loginAddr}, nil) // CantonPartyID empty

	_, err := New(v, iss, n, u).Login(context.Background(), loginMessage, loginSig)
	requireUnauthorized(t, err)
}

func TestLogin_StoreError(t *testing.T) {
	v, iss, n, u := newLoginDeps(t)
	storeErr := errors.New("db unavailable")
	v.EXPECT().Verify(loginMessage, loginSig).Return(testAddr, loginNonce, nil)
	n.EXPECT().Consume(loginNonce).Return(true)
	u.EXPECT().GetUserByEVMAddress(mock.Anything, loginAddr).Return(nil, storeErr)

	_, err := New(v, iss, n, u).Login(context.Background(), loginMessage, loginSig)
	if !errors.Is(err, storeErr) {
		t.Fatalf("expected wrapped store error, got %v", err)
	}
}

func TestLogin_IssueError(t *testing.T) {
	v, iss, n, u := newLoginDeps(t)
	v.EXPECT().Verify(loginMessage, loginSig).Return(testAddr, loginNonce, nil)
	n.EXPECT().Consume(loginNonce).Return(true)
	u.EXPECT().GetUserByEVMAddress(mock.Anything, loginAddr).
		Return(&user.User{CantonPartyID: "party::abc"}, nil)
	iss.EXPECT().Issue(loginAddr, "party::abc").Return("", time.Time{}, errors.New("sign failure"))

	_, err := New(v, iss, n, u).Login(context.Background(), loginMessage, loginSig)
	if !apperrors.Is(err, apperrors.CategoryGeneralError) {
		t.Fatalf("expected CategoryGeneralError, got %v", err)
	}
}

func TestNonce_Delegates(t *testing.T) {
	v, iss, n, u := newLoginDeps(t)
	n.EXPECT().Issue(loginAddr).Return("fresh-nonce", nil)

	got, err := New(v, iss, n, u).Nonce(loginAddr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "fresh-nonce" {
		t.Fatalf("Nonce() = %q, want fresh-nonce", got)
	}
}

func TestJWKS_Delegates(t *testing.T) {
	v, iss, n, u := newLoginDeps(t)
	want := jwt.JWKS{Keys: []jwt.JWK{{Kid: "kid-1", Kty: "RSA"}}}
	iss.EXPECT().JWKS().Return(want)

	got := New(v, iss, n, u).JWKS()
	if len(got.Keys) != 1 || got.Keys[0].Kid != "kid-1" {
		t.Fatalf("JWKS() = %+v, want %+v", got, want)
	}
}

func requireUnauthorized(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !apperrors.Is(err, apperrors.CategoryUnauthorized) {
		t.Fatalf("expected CategoryUnauthorized, got %v", err)
	}
}
