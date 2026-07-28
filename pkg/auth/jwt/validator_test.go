// SPDX-License-Identifier: Apache-2.0

package jwt

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

func validClaims() gojwt.MapClaims {
	return gojwt.MapClaims{
		"iss": testIssuer,
		"sub": "party::xyz",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
}

func TestValidator_InProcessKey_Accepts(t *testing.T) {
	key := newTestKey(t)
	token := mintRS256(t, key, "kid-1", validClaims())

	claims, err := NewValidatorWithKey("kid-1", &key.PublicKey, testIssuer).ValidateToken(token)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if claims["sub"] != "party::xyz" {
		t.Fatalf("sub = %v, want party::xyz", claims["sub"])
	}
}

func TestValidator_RejectsWrongAlgorithm(t *testing.T) {
	key := newTestKey(t)
	// HS256 token — the validator must reject non-RSA methods (alg-confusion guard).
	hs := gojwt.NewWithClaims(gojwt.SigningMethodHS256, validClaims())
	hs.Header["kid"] = "kid-1"
	signed, err := hs.SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("sign HS256: %v", err)
	}

	if _, err := NewValidatorWithKey("kid-1", &key.PublicKey, testIssuer).ValidateToken(signed); err == nil {
		t.Fatal("HS256 token must be rejected")
	}
}

func TestValidator_RejectsUnknownKid(t *testing.T) {
	key := newTestKey(t)
	token := mintRS256(t, key, "other-kid", validClaims())

	if _, err := NewValidatorWithKey("kid-1", &key.PublicKey, testIssuer).ValidateToken(token); err == nil {
		t.Fatal("token with unknown kid must be rejected")
	}
}

func TestValidator_RejectsWrongIssuer(t *testing.T) {
	key := newTestKey(t)
	claims := validClaims()
	claims["iss"] = "someone-else"
	token := mintRS256(t, key, "kid-1", claims)

	if _, err := NewValidatorWithKey("kid-1", &key.PublicKey, testIssuer).ValidateToken(token); err == nil {
		t.Fatal("token with wrong issuer must be rejected")
	}
}

func TestValidator_RejectsExpiredToken(t *testing.T) {
	key := newTestKey(t)
	claims := validClaims()
	claims["exp"] = time.Now().Add(-time.Hour).Unix()
	token := mintRS256(t, key, "kid-1", claims)

	if _, err := NewValidatorWithKey("kid-1", &key.PublicKey, testIssuer).ValidateToken(token); err == nil {
		t.Fatal("expired token must be rejected")
	}
}

func TestValidator_FetchesKeysFromJWKS(t *testing.T) {
	key := newTestKey(t)
	set := marshalJWKS("kid-remote", &key.PublicKey)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(set)
	}))
	defer srv.Close()

	token := mintRS256(t, key, "kid-remote", validClaims())

	claims, err := NewValidator(srv.URL, testIssuer).ValidateToken(token)
	if err != nil {
		t.Fatalf("validate via JWKS: %v", err)
	}
	if claims["sub"] != "party::xyz" {
		t.Fatalf("sub = %v, want party::xyz", claims["sub"])
	}
}

func TestValidator_IsConfigured(t *testing.T) {
	key := newTestKey(t)
	if !NewValidatorWithKey("kid-1", &key.PublicKey, testIssuer).IsConfigured() {
		t.Fatal("in-process validator should report configured")
	}
	if !NewValidator("http://jwks.example/keys", testIssuer).IsConfigured() {
		t.Fatal("JWKS validator should report configured")
	}
	if (&Validator{}).IsConfigured() {
		t.Fatal("empty validator should report not configured")
	}
}
