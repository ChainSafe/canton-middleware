// SPDX-License-Identifier: Apache-2.0

// Package jwt provides the authentication primitives behind the api-server's
// Sign-In-with-Ethereum (EIP-4361) login flow and the JWT sessions it issues.

package jwt

import (
	"crypto/rsa"
	"fmt"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
	siwe "github.com/spruceid/siwe-go"
)

// Custom JWT claims carried alongside the registered claims.
const (
	// EVMAddressClaim carries the authenticated EVM address.
	EVMAddressClaim = "evm_address"
	// CantonPartyClaim carries the user's Canton party id. It mirrors the standard
	// "sub" claim but is named explicitly so external verifiers (e.g. the indexer)
	// can read the party without relying on "sub" semantics.
	CantonPartyClaim = "canton_party_id"
)

// Claims are the claims minted at login. The Canton party id is both the subject
// and an explicit canton_party_id claim; the EVM address rides alongside so handlers
// can resolve either identity from the token.
type Claims struct {
	EVMAddress    string `json:"evm_address"`
	CantonPartyID string `json:"canton_party_id"`
	gojwt.RegisteredClaims
}

// Issuer mints RS256 JWTs for authenticated users.
type Issuer struct {
	key      *rsa.PrivateKey
	kid      string
	issuer   string
	audience string
	ttl      time.Duration
	now      func() time.Time
}

// NewIssuer creates an Issuer that signs with key, advertising it under kid.
func NewIssuer(key *rsa.PrivateKey, kid, issuer, audience string, ttl time.Duration) *Issuer {
	return &Issuer{
		key:      key,
		kid:      kid,
		issuer:   issuer,
		audience: audience,
		ttl:      ttl,
		now:      time.Now,
	}
}

// Issue mints a signed token for the (evmAddress, cantonPartyID) identity and
// returns it alongside its expiry time.
func (i *Issuer) Issue(evmAddress, cantonPartyID string) (string, time.Time, error) {
	now := i.now()
	exp := now.Add(i.ttl)

	claims := Claims{
		EVMAddress:    evmAddress,
		CantonPartyID: cantonPartyID,
		RegisteredClaims: gojwt.RegisteredClaims{
			Subject:   cantonPartyID,
			Issuer:    i.issuer,
			Audience:  gojwt.ClaimStrings{i.audience},
			IssuedAt:  gojwt.NewNumericDate(now),
			ExpiresAt: gojwt.NewNumericDate(exp),
			ID:        siwe.GenerateNonce(),
		},
	}

	token := gojwt.NewWithClaims(gojwt.SigningMethodRS256, claims)
	token.Header["kid"] = i.kid

	signed, err := token.SignedString(i.key)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return signed, exp, nil
}

// PublicKey returns the public half of the signing key, for JWKS publication and
// in-process validation.
func (i *Issuer) PublicKey() *rsa.PublicKey { return &i.key.PublicKey }

// KeyID returns the kid advertised for the signing key.
func (i *Issuer) KeyID() string { return i.kid }

// JWKS returns the public signing key as a JSON Web Key Set for publication.
func (i *Issuer) JWKS() JWKS { return marshalJWKS(i.kid, &i.key.PublicKey) }
