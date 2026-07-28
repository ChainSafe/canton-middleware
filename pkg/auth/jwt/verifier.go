// SPDX-License-Identifier: Apache-2.0

package jwt

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	siwe "github.com/spruceid/siwe-go"
)

// eip191SigLen is the byte length of a secp256k1 recoverable signature (r||s||v).
const eip191SigLen = 65

// SIWEVerifier validates EIP-4361 login messages against the server's expected
// domain, uri, and chain id, and recovers the signer. It does not track nonces: it
// returns the message's nonce so the caller can enforce single use against its own
// store, keeping this a stateless primitive.
type SIWEVerifier struct {
	domain  string
	uri     string
	chainID int
	now     func() time.Time
}

// NewSIWEVerifier binds a verifier to the expected domain/uri/chain.
func NewSIWEVerifier(domain, uri string, chainID int) *SIWEVerifier {
	return &SIWEVerifier{
		domain:  domain,
		uri:     uri,
		chainID: chainID,
		now:     time.Now,
	}
}

// Verify parses and validates a SIWE message and its signature. On success it
// returns the recovered checksummed EVM address and the message's nonce; the caller
// is responsible for enforcing nonce single use. It performs no state mutation, so a
// failed verification leaves any nonce untouched.
func (v *SIWEVerifier) Verify(rawMessage, signature string) (addr common.Address, nonce string, err error) {
	msg, err := siwe.ParseMessage(rawMessage)
	if err != nil {
		return common.Address{}, "", fmt.Errorf("parse SIWE message: %w", err)
	}

	if got := msg.GetChainID(); got != v.chainID {
		return common.Address{}, "", fmt.Errorf("unexpected chain id: got %d, want %d", got, v.chainID)
	}
	if got := msg.GetURI(); got.String() != v.uri {
		return common.Address{}, "", fmt.Errorf("unexpected uri: got %q, want %q", got.String(), v.uri)
	}

	// Guard the signature length before handing it to siwe-go: its VerifyEIP191
	// indexes sigBytes[64] with no bounds check and panics on a signature shorter
	// than 65 bytes. Reject malformed signatures as an auth failure instead.
	sigBytes, decErr := hexutil.Decode(signature)
	if decErr != nil || len(sigBytes) != eip191SigLen {
		return common.Address{}, "", fmt.Errorf("signature must be a 0x-prefixed 65-byte hex string")
	}

	// Verify signature, domain, and time window (expiration / not-before). Nonce
	// uniqueness is enforced by the caller against its store, not here.
	now := v.now()
	if _, err := msg.Verify(signature, &v.domain, nil, &now); err != nil {
		return common.Address{}, "", fmt.Errorf("verify SIWE signature: %w", err)
	}

	return msg.GetAddress(), msg.GetNonce(), nil
}
