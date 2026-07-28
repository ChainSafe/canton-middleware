// SPDX-License-Identifier: Apache-2.0

// Package service implements the Sign-In with Ethereum (EIP-4361) login flow: it
// issues single-use nonces, verifies signed SIWE messages, and mints the JWTs that
// authenticate read endpoints. The cryptographic primitives (JWT issuer, SIWE
// verifier, JWKS) live in pkg/auth/jwt and the nonce store in
// pkg/auth/service/nonce_provider; this package orchestrates them and binds the
// authenticated address to a registered user's Canton party.
package service

import (
	"context"
	"errors"
	"time"

	"github.com/ethereum/go-ethereum/common"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/chainsafe/canton-middleware/pkg/auth/jwt"
	"github.com/chainsafe/canton-middleware/pkg/user"
)

// The login service depends only on the narrow interfaces below, declared at the
// consumer so they can be mocked in isolation. Concrete implementations come from
// pkg/auth/jwt (Verifier, Issuer), pkg/auth/service/nonce_provider (NonceStore),
// and pkg/userstore (UserLookup).

//go:generate mockery --name Service --output mocks --outpkg mocks --filename mock_service.go --with-expecter
//go:generate mockery --name Verifier --output mocks --outpkg mocks --filename mock_verifier.go --with-expecter
//go:generate mockery --name Issuer --output mocks --outpkg mocks --filename mock_issuer.go --with-expecter
//go:generate mockery --name NonceStore --output mocks --outpkg mocks --filename mock_nonce_store.go --with-expecter
//go:generate mockery --name UserLookup --output mocks --outpkg mocks --filename mock_user_lookup.go --with-expecter

// UserLookup resolves a registered user by EVM address so login can bind the token
// to the user's Canton party. Satisfied by *userstore.Store.
type UserLookup interface {
	GetUserByEVMAddress(ctx context.Context, evmAddress string) (*user.User, error)
}

// NonceStore issues and consumes single-use login nonces keyed by address. Consume
// must be atomic so a given nonce satisfies at most one login. Implementations live
// under nonce_provider (e.g. nonceprovider.InMemory).
type NonceStore interface {
	// Issue returns a nonce for address, valid for the store's TTL. Calling it again
	// for an address that still holds a live nonce returns the same value. It may
	// return an error if the store is at capacity.
	Issue(address string) (string, error)
	// Consume returns true exactly once for a live, previously-issued nonce and
	// removes it; false for unknown, expired, or already-consumed nonces.
	Consume(nonce string) bool
}

// Verifier validates a signed SIWE login message and returns the recovered EVM
// address and the message nonce. Satisfied by *jwt.SIWEVerifier.
type Verifier interface {
	Verify(message, signature string) (common.Address, string, error)
}

// Issuer mints session JWTs and publishes the signing key set. Satisfied by
// *jwt.Issuer.
type Issuer interface {
	Issue(evmAddress, cantonPartyID string) (string, time.Time, error)
	JWKS() jwt.JWKS
}

// Service orchestrates the SIWE login flow.
type Service interface {
	// Nonce issues a single-use login nonce for the given EVM address.
	Nonce(address string) (string, error)
	// JWKS returns the public signing key set for token validation by other services.
	JWKS() jwt.JWKS
	// Login verifies a signed SIWE message and, if the recovered address belongs to
	// a registered user, issues a JWT bound to that user's Canton party.
	Login(ctx context.Context, message, signature string) (*auth.LoginResponse, error)
}

type loginService struct {
	verifier Verifier
	issuer   Issuer
	nonces   NonceStore
	users    UserLookup
}

// New builds a login Service from its collaborators.
func New(
	verifier Verifier,
	issuer Issuer,
	nonces NonceStore,
	users UserLookup,
) Service {
	return &loginService{
		verifier: verifier,
		issuer:   issuer,
		nonces:   nonces,
		users:    users,
	}
}

func (s *loginService) Nonce(address string) (string, error) { return s.nonces.Issue(address) }

func (s *loginService) JWKS() jwt.JWKS { return s.issuer.JWKS() }

func (s *loginService) Login(ctx context.Context, message, signature string) (*auth.LoginResponse, error) {
	addr, nonce, err := s.verifier.Verify(message, signature)
	if err != nil {
		return nil, apperrors.UnAuthorizedError(err, "invalid sign-in message or signature")
	}
	// Enforce single-use only after the signature checks out, so a bad signature
	// cannot burn a victim's outstanding nonce.
	if !s.nonces.Consume(nonce) {
		return nil, apperrors.UnAuthorizedError(nil, "nonce is unknown, expired, or already used")
	}
	evmAddress := auth.NormalizeAddress(addr.Hex())

	usr, err := s.users.GetUserByEVMAddress(ctx, evmAddress)
	if err != nil && !errors.Is(err, user.ErrUserNotFound) {
		return nil, err
	}
	if usr == nil {
		return nil, apperrors.UnAuthorizedError(nil, "address is not registered")
	}
	if usr.CantonPartyID == "" {
		return nil, apperrors.UnAuthorizedError(nil, "user has no Canton party id")
	}

	token, exp, err := s.issuer.Issue(evmAddress, usr.CantonPartyID)
	if err != nil {
		return nil, apperrors.GeneralError(err)
	}

	return &auth.LoginResponse{Token: token, ExpiresAt: exp.Unix()}, nil
}
