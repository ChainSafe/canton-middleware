// SPDX-License-Identifier: Apache-2.0

package jwt

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"fmt"
	"maps"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	gojwt "github.com/golang-jwt/jwt/v5"
	siwe "github.com/spruceid/siwe-go"
)

const (
	testDomain  = "localhost"
	testURI     = "http://localhost"
	testChainID = 31337
	testIssuer  = "canton-middleware"
	testAud     = "canton-middleware-api"
)

func newTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return key
}

// signSIWE builds an EIP-4361 message for the given nonce (with optional overrides
// such as expirationTime), signs it with a freshly generated EOA key, and returns
// the raw message, its 0x-prefixed signature, and the signer's checksummed address.
func signSIWE(t *testing.T, nonce string, opts map[string]any) (raw, signature string, addr common.Address) {
	t.Helper()

	priv, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate EOA key: %v", err)
	}
	addr = crypto.PubkeyToAddress(priv.PublicKey)

	options := map[string]any{
		"chainId":  testChainID,
		"issuedAt": time.Now().UTC().Format(time.RFC3339),
	}
	maps.Copy(options, opts)

	msg, err := siwe.InitMessage(testDomain, addr.Hex(), testURI, nonce, options)
	if err != nil {
		t.Fatalf("init SIWE message: %v", err)
	}
	return msg.String(), signPersonal(t, priv, msg.String()), addr
}

// signPersonal produces an EIP-191 personal_sign signature over message.
func signPersonal(t *testing.T, priv *ecdsa.PrivateKey, message string) string {
	t.Helper()
	prefixed := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(message), message)
	hash := crypto.Keccak256Hash([]byte(prefixed))
	sig, err := crypto.Sign(hash.Bytes(), priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return "0x" + hex.EncodeToString(sig)
}

// mintRS256 signs claims as an RS256 JWT with the given key and kid header. It lets
// validator tests craft tokens with arbitrary claims without going through Issuer.
func mintRS256(t *testing.T, key *rsa.PrivateKey, kid string, claims gojwt.MapClaims) string {
	t.Helper()
	token := gojwt.NewWithClaims(gojwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign RS256 token: %v", err)
	}
	return signed
}
