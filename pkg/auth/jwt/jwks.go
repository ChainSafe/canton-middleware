// SPDX-License-Identifier: Apache-2.0

package jwt

import (
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"math/big"
)

// JWK is a single JSON Web Key (RSA public key) per RFC 7517.
type JWK struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// JWKS is a JSON Web Key Set, the document served at /.well-known/jwks.json.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// marshalJWKS renders an RSA public key as a single-key JWKS document. It is the
// inverse of parseRSAPublicKey: modulus and exponent are base64url-encoded (no
// padding) per RFC 7518.
func marshalJWKS(kid string, pub *rsa.PublicKey) JWKS {
	eBytes := big.NewInt(int64(pub.E)).Bytes()
	return JWKS{Keys: []JWK{{
		Kty: "RSA",
		Use: "sig",
		Alg: "RS256",
		Kid: kid,
		N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(eBytes),
	}}}
}

// parseRSAPublicKey reconstructs an RSA public key from base64url-encoded modulus
// and exponent (the "n" and "e" members of a JWK).
func parseRSAPublicKey(nStr, eStr string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nStr)
	if err != nil {
		return nil, fmt.Errorf("decode modulus: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eStr)
	if err != nil {
		return nil, fmt.Errorf("decode exponent: %w", err)
	}

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: int(new(big.Int).SetBytes(eBytes).Int64()),
	}, nil
}
