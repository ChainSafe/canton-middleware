// SPDX-License-Identifier: Apache-2.0

package jwt

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"testing"
)

// b64PEM renders a PEM block and base64-encodes it, matching how the signing key
// is supplied to ParseRSAPrivateKey (base64 so the multi-line PEM survives env
// substitution into a single YAML scalar).
func b64PEM(block *pem.Block) string {
	return base64.StdEncoding.EncodeToString(pem.EncodeToMemory(block))
}

func TestParseRSAPrivateKey_PKCS1(t *testing.T) {
	key := newTestKey(t)
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}

	got, err := ParseRSAPrivateKey(b64PEM(block))
	if err != nil {
		t.Fatalf("parse PKCS#1: %v", err)
	}
	if got.N.Cmp(key.N) != 0 {
		t.Fatal("parsed key does not match original")
	}
}

func TestParseRSAPrivateKey_PKCS8(t *testing.T) {
	key := newTestKey(t)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal PKCS#8: %v", err)
	}
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: der}

	got, err := ParseRSAPrivateKey(b64PEM(block))
	if err != nil {
		t.Fatalf("parse PKCS#8: %v", err)
	}
	if got.N.Cmp(key.N) != 0 {
		t.Fatal("parsed key does not match original")
	}
}

func TestParseRSAPrivateKey_NotBase64(t *testing.T) {
	if _, err := ParseRSAPrivateKey("!!! not base64 !!!"); err == nil {
		t.Fatal("expected error for non-base64 input")
	}
}

func TestParseRSAPrivateKey_NoPEM(t *testing.T) {
	if _, err := ParseRSAPrivateKey(base64.StdEncoding.EncodeToString([]byte("not a pem block"))); err == nil {
		t.Fatal("expected error for input without a PEM block")
	}
}

func TestParseRSAPrivateKey_NonRSA(t *testing.T) {
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate EC key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(ecKey)
	if err != nil {
		t.Fatalf("marshal EC PKCS#8: %v", err)
	}
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: der}

	if _, err := ParseRSAPrivateKey(b64PEM(block)); err == nil {
		t.Fatal("expected error for a non-RSA key")
	}
}
