//go:build ignore

// allocate-standalone-party.go — Allocate an external party on our participant
// directly via the Canton Ledger API, no middleware DB involved.
//
// Writes all keys and party info to a single text file so the party can be
// accessed/used later via standalone scripts (no middleware required).
//
// Usage:
//   go run scripts/testing/allocate-standalone-party.go \
//     -config config.api-server.devnet-test.yaml \
//     -hint "vinh_test_user" \
//     -out party-credentials.txt

package main

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"time"

	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/client"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/identity"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/keys"

	"go.uber.org/zap"
)

var (
	configPath = flag.String("config", "config.api-server.devnet-test.yaml", "Config file")
	hint       = flag.String("hint", "standalone-test", "Party hint (visible in party ID)")
	outPath    = flag.String("out", "party-credentials.txt", "Output file for party credentials")
)

func main() {
	flag.Parse()

	cfg, err := config.LoadAPIServer(*configPath)
	if err != nil {
		fatalf("load config: %v", err)
	}

	logger, _ := zap.NewDevelopment()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. Generate a fresh keypair
	kp, err := keys.GenerateCantonKeyPair()
	if err != nil {
		fatalf("generate keypair: %v", err)
	}
	spki, err := kp.SPKIPublicKey()
	if err != nil {
		fatalf("spki: %v", err)
	}
	fp, err := kp.Fingerprint()
	if err != nil {
		fatalf("fingerprint: %v", err)
	}

	fmt.Printf("Generated keypair (fingerprint: %s)\n", fp)

	// 2. Connect to Canton (no key resolver — we sign locally)
	cantonClient, err := canton.New(ctx, cfg.Canton, canton.WithLogger(logger))
	if err != nil {
		fatalf("canton client: %v", err)
	}
	defer func() { _ = cantonClient.Close() }()

	// 3. Allocate external party (signing happens in-process via kp)
	signer := externalPartySigner{kp: kp}
	party, err := cantonClient.Identity.AllocateExternalParty(ctx, *hint, spki, signer)
	if err != nil {
		fatalf("AllocateExternalParty: %v", err)
	}
	fmt.Printf("Allocated party: %s\n", party.PartyID)

	// 4. Get participant ID for the file
	participantID, err := cantonClient.Identity.GetParticipantID(ctx)
	if err != nil {
		fatalf("GetParticipantID: %v", err)
	}

	// 5. Write all info to the credentials file
	contents := fmt.Sprintf(`# Canton External Party Credentials
# Generated: %s
# Created via standalone script (no middleware involvement)

[Network]
canton_rpc_url       = %s
synchronizer_id      = %s
participant_id       = %s

[Party]
party_id             = %s
party_hint           = %s
canton_fingerprint   = %s

[Keys]
# secp256k1, 32-byte private key (hex)
private_key_hex      = %s

# Compressed (33-byte) secp256k1 public key (hex)
public_key_hex       = %s

# X.509 SubjectPublicKeyInfo DER (base64) — used for party allocation
spki_public_key_b64  = %s

[OAuth]
# These are the same OAuth credentials our config uses; included here so
# subsequent scripts (e.g. accept transfer) can reach the participant.
client_id            = %s
audience             = %s
token_url            = %s
# client_secret is intentionally omitted — read from config or env
`,
		time.Now().UTC().Format(time.RFC3339),
		cfg.Canton.Ledger.RPCURL,
		cfg.Canton.DomainID,
		participantID,
		party.PartyID,
		*hint,
		fp,
		hex.EncodeToString(kp.PrivateKey),
		hex.EncodeToString(kp.PublicKey),
		base64.StdEncoding.EncodeToString(spki),
		cfg.Canton.Ledger.Auth.ClientID,
		cfg.Canton.Ledger.Auth.Audience,
		cfg.Canton.Ledger.Auth.TokenURL,
	)

	if err := os.WriteFile(*outPath, []byte(contents), 0600); err != nil {
		fatalf("write %s: %v", *outPath, err)
	}

	fmt.Printf("Wrote credentials to %s (mode 0600)\n", *outPath)
	fmt.Println()
	fmt.Println("Share this party ID with the sender:")
	fmt.Printf("  %s\n", party.PartyID)
}

// externalPartySigner adapts CantonKeyPair to the identity.ExternalPartyKey interface.
type externalPartySigner struct {
	kp *keys.CantonKeyPair
}

func (s externalPartySigner) SignDER(message []byte) ([]byte, error) {
	// SignDER hashes the input with SHA-256 then signs and DER-encodes.
	// This matches Canton's EC_DSA_SHA_256 algorithm spec used at allocation.
	return s.kp.SignDER(message)
}

// Fingerprint is unused in this flow but required by interface.
func (s externalPartySigner) Fingerprint() (string, error) {
	return s.kp.Fingerprint()
}

var _ identity.ExternalPartyKey = externalPartySigner{}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FATAL: "+format+"\n", args...)
	os.Exit(1)
}
