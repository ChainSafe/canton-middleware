// SPDX-License-Identifier: Apache-2.0

package jwt

import "testing"

func TestMarshalJWKS_RoundTrip(t *testing.T) {
	key := newTestKey(t)

	set := marshalJWKS("kid-1", &key.PublicKey)
	if len(set.Keys) != 1 {
		t.Fatalf("got %d keys, want 1", len(set.Keys))
	}
	jwk := set.Keys[0]
	if jwk.Kid != "kid-1" || jwk.Kty != "RSA" || jwk.Alg != "RS256" || jwk.Use != "sig" {
		t.Fatalf("unexpected JWK metadata: %+v", jwk)
	}

	pub, err := parseRSAPublicKey(jwk.N, jwk.E)
	if err != nil {
		t.Fatalf("parseRSAPublicKey: %v", err)
	}
	if pub.N.Cmp(key.PublicKey.N) != 0 {
		t.Fatal("modulus did not round-trip")
	}
	if pub.E != key.PublicKey.E {
		t.Fatalf("exponent = %d, want %d", pub.E, key.PublicKey.E)
	}
}

func TestParseRSAPublicKey_RejectsBadBase64(t *testing.T) {
	if _, err := parseRSAPublicKey("!!!not-base64!!!", "AQAB"); err == nil {
		t.Fatal("expected error for invalid modulus encoding")
	}
}
