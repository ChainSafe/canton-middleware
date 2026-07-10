// SPDX-License-Identifier: Apache-2.0

package auth

import "time"

// Config configures Sign-In with Ethereum (EIP-4361) login and the JWTs it issues
// for read endpoints. It is always required; read endpoints are gated by a bearer
// token. The matching public key is published at /.well-known/jwks.json so other
// services (e.g. the indexer) can validate tokens without a shared secret.
type Config struct {
	// PrivateKey is the RSA signing key as a base64-encoded PEM (PKCS#1 or PKCS#8).
	// Supply it via env substitution — private_key: "${JWT_PRIVATE_KEY}" — where the
	// env holds `base64 < key.pem`. It is base64-encoded so the (multi-line) PEM
	// survives being expanded into a single YAML scalar.
	PrivateKey string `yaml:"private_key" validate:"required"`
	// KeyID is the JWKS "kid" advertised for the signing key.
	KeyID string `yaml:"kid" validate:"required" default:"default"`
	// Issuer is the JWT "iss" claim (and the value validators check).
	Issuer string `yaml:"issuer" validate:"required" default:"canton-middleware"`
	// Audience is the JWT "aud" claim.
	Audience string `yaml:"audience" validate:"required" default:"canton-middleware-api"`
	// TokenTTL is how long an issued JWT stays valid.
	TokenTTL time.Duration `yaml:"token_ttl" validate:"required,gt=0" default:"30m"`
	// NonceTTL is how long a login nonce stays valid before it must be re-fetched.
	NonceTTL time.Duration `yaml:"nonce_ttl" validate:"required,gt=0" default:"5m"`
	// Domain is the EIP-4361 domain the SIWE message must bind to (e.g. "app.example.com").
	Domain string `yaml:"domain" validate:"required"`
	// URI is the EIP-4361 uri the SIWE message must bind to (e.g. "https://app.example.com").
	URI string `yaml:"uri" validate:"required"`
	// ChainID is the EIP-155 chain id the SIWE message must declare.
	ChainID int `yaml:"chain_id" validate:"required,gt=0"`
}
