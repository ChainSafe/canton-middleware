// SPDX-License-Identifier: Apache-2.0

package jwt

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestSIWEVerifier_Success(t *testing.T) {
	verifier := NewSIWEVerifier(testDomain, testURI, testChainID)

	raw, sig, addr := signSIWE(t, "testnonce0001", nil)

	gotAddr, gotNonce, err := verifier.Verify(raw, sig)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if gotAddr != addr {
		t.Fatalf("recovered %s, want %s", gotAddr, addr)
	}
	if gotNonce != "testnonce0001" {
		t.Fatalf("returned nonce %q, want testnonce0001", gotNonce)
	}
}

func TestSIWEVerifier_WrongDomainRejected(t *testing.T) {
	verifier := NewSIWEVerifier("evil.example.com", testURI, testChainID)

	raw, sig, _ := signSIWE(t, "testnonce0001", nil)
	if _, _, err := verifier.Verify(raw, sig); err == nil {
		t.Fatal("domain mismatch must be rejected")
	}
}

func TestSIWEVerifier_WrongChainRejected(t *testing.T) {
	verifier := NewSIWEVerifier(testDomain, testURI, 1) // expects mainnet

	raw, sig, _ := signSIWE(t, "testnonce0001", nil) // signs with testChainID
	if _, _, err := verifier.Verify(raw, sig); err == nil {
		t.Fatal("chain id mismatch must be rejected")
	}
}

func TestSIWEVerifier_WrongURIRejected(t *testing.T) {
	verifier := NewSIWEVerifier(testDomain, "http://other.example", testChainID)

	raw, sig, _ := signSIWE(t, "testnonce0001", nil)
	if _, _, err := verifier.Verify(raw, sig); err == nil {
		t.Fatal("uri mismatch must be rejected")
	}
}

func TestSIWEVerifier_ExpiredMessageRejected(t *testing.T) {
	verifier := NewSIWEVerifier(testDomain, testURI, testChainID)

	raw, sig, _ := signSIWE(t, "testnonce0001", map[string]any{
		"expirationTime": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
	})
	if _, _, err := verifier.Verify(raw, sig); err == nil {
		t.Fatal("expired SIWE message must be rejected")
	}
}

func TestSIWEVerifier_TamperedSignatureRejected(t *testing.T) {
	verifier := NewSIWEVerifier(testDomain, testURI, testChainID)

	raw, _, _ := signSIWE(t, "testnonce0001", nil)

	// A signature from a different signer over the same message.
	other, _ := crypto.GenerateKey()
	badSig := signPersonal(t, other, raw)

	if _, _, err := verifier.Verify(raw, badSig); err == nil {
		t.Fatal("signature from a non-matching signer must be rejected")
	}
}

func TestSIWEVerifier_MalformedSignatureRejected(t *testing.T) {
	verifier := NewSIWEVerifier(testDomain, testURI, testChainID)

	raw, _, _ := signSIWE(t, "testnonce0001", nil)

	// A too-short signature must be rejected as an auth failure, not panic
	// (siwe-go's VerifyEIP191 indexes sigBytes[64] with no bounds check).
	if _, _, err := verifier.Verify(raw, "0x00"); err == nil {
		t.Fatal("malformed (short) signature must be rejected")
	}
}
