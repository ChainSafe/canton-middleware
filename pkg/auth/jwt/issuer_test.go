// SPDX-License-Identifier: Apache-2.0

package jwt

import (
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

func TestIssuer_IssueSetsClaims(t *testing.T) {
	key := newTestKey(t)
	issuer := NewIssuer(key, "kid-1", testIssuer, testAud, time.Hour)

	before := time.Now()
	token, exp, err := issuer.Issue("0xABC", "party::xyz")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if exp.Before(before.Add(time.Hour - time.Minute)) {
		t.Fatalf("expiry %v is not ~1h out", exp)
	}

	// Parse independently with the issuer's public key (no Validator involved) to
	// assert the token's structure and claims.
	parsed, err := gojwt.Parse(token, func(*gojwt.Token) (any, error) { return &key.PublicKey, nil })
	if err != nil || !parsed.Valid {
		t.Fatalf("parse issued token: valid=%v err=%v", parsed.Valid, err)
	}
	if kid, _ := parsed.Header["kid"].(string); kid != "kid-1" {
		t.Fatalf("kid header = %q, want kid-1", kid)
	}

	claims := parsed.Claims.(gojwt.MapClaims)
	if claims["sub"] != "party::xyz" {
		t.Fatalf("sub = %v, want party::xyz", claims["sub"])
	}
	if claims[CantonPartyClaim] != "party::xyz" {
		t.Fatalf("%s = %v, want party::xyz", CantonPartyClaim, claims[CantonPartyClaim])
	}
	if claims[EVMAddressClaim] != "0xABC" {
		t.Fatalf("%s = %v, want 0xABC", EVMAddressClaim, claims[EVMAddressClaim])
	}
	if claims["iss"] != testIssuer {
		t.Fatalf("iss = %v, want %s", claims["iss"], testIssuer)
	}
	// aud is serialized as a JSON array by jwt.ClaimStrings and parses back as such.
	aud, ok := claims["aud"].([]any)
	if !ok || len(aud) != 1 || aud[0] != testAud {
		t.Fatalf("aud = %v, want [%s]", claims["aud"], testAud)
	}
	if _, ok := claims["jti"]; !ok {
		t.Fatal("expected a jti claim")
	}
}

func TestIssuer_JWKSDescribesSigningKey(t *testing.T) {
	key := newTestKey(t)
	issuer := NewIssuer(key, "kid-1", testIssuer, testAud, time.Hour)

	set := issuer.JWKS()
	if len(set.Keys) != 1 {
		t.Fatalf("JWKS has %d keys, want 1", len(set.Keys))
	}
	k := set.Keys[0]
	if k.Kid != "kid-1" || k.Kty != "RSA" || k.Alg != "RS256" || k.Use != "sig" {
		t.Fatalf("unexpected JWK metadata: %+v", k)
	}
	if k.N == "" || k.E == "" {
		t.Fatal("JWK modulus/exponent must be populated")
	}

	// The published key must reconstruct to the issuer's actual public key.
	pub, err := parseRSAPublicKey(k.N, k.E)
	if err != nil {
		t.Fatalf("parse published key: %v", err)
	}
	if pub.N.Cmp(key.PublicKey.N) != 0 || pub.E != key.PublicKey.E {
		t.Fatal("published JWKS key does not match the signing key")
	}
}
