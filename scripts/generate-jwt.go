// +build ignore

// This script generates a JWT token for Canton Ledger API authentication
// Run with: go run scripts/generate-jwt.go

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"crypto/hmac"
	"crypto/sha256"
)

// JWT Claims for Canton Ledger API
type Claims struct {
	// Standard claims
	Iss string `json:"iss,omitempty"` // Issuer
	Sub string `json:"sub,omitempty"` // Subject (user ID)
	Aud string `json:"aud,omitempty"` // Audience
	Exp int64  `json:"exp,omitempty"` // Expiration time
	Iat int64  `json:"iat,omitempty"` // Issued at

	// Canton-specific claims
	// For Ledger API v2, use these claims:
	ActAs  []string `json:"actAs,omitempty"`  // Parties the token can act as
	ReadAs []string `json:"readAs,omitempty"` // Parties the token can read as
	Admin  bool     `json:"admin,omitempty"`  // Admin access

	// Application ID
	ApplicationId string `json:"applicationId,omitempty"`
}

func main() {
	// Configuration - must match simple-topology.conf
	secret := "devSecret123456789012345678901234"

	// Default party (participant1's party ID)
	// This will be updated once we know the actual party ID
	party := os.Getenv("CANTON_PARTY")
	if party == "" {
		party = "participant1::1220600408d7ce77f61bac5f37187c66bed71e940923cfaa867957b7b7c2f5cd6680"
	}

	appId := os.Getenv("CANTON_APP_ID")
	if appId == "" {
		appId = "canton-middleware"
	}

	// Create claims
	// Canton limits token lifetime - use 1 hour max
	now := time.Now()
	claims := Claims{
		// Remove issuer - Canton's unsafe-jwt-hmac-256 doesn't require identity provider config
		Sub:           "relayer",
		Aud:           "https://daml.com/jwt/aud/participant/participant1",
		Iat:           now.Unix(),
		Exp:           now.Add(1 * time.Hour).Unix(), // 1 hour expiry (Canton may limit this)
		ActAs:         []string{party},
		ReadAs:        []string{party},
		Admin:         true,
		ApplicationId: appId,
	}

	// Generate token
	token, err := generateHMAC256JWT(claims, secret)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating token: %v\n", err)
		os.Exit(1)
	}

	// Output
	fmt.Println("=== Canton Ledger API JWT Token ===")
	fmt.Println()
	fmt.Println("Token:")
	fmt.Println(token)
	fmt.Println()
	fmt.Println("Claims:")
	claimsJSON, _ := json.MarshalIndent(claims, "", "  ")
	fmt.Println(string(claimsJSON))
	fmt.Println()
	fmt.Println("To use this token:")
	fmt.Println("1. Save to a file: echo '" + token + "' > jwt-token.txt")
	fmt.Println("2. Update config.yaml: canton.auth.token_file: jwt-token.txt")
	fmt.Println("3. Or set environment variable: export CANTON_JWT_TOKEN='" + token + "'")
}

func generateHMAC256JWT(claims Claims, secret string) (string, error) {
	// Header
	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	// Base64URL encode
	headerB64 := base64URLEncode(headerJSON)
	claimsB64 := base64URLEncode(claimsJSON)

	// Create signature input
	signInput := headerB64 + "." + claimsB64

	// Sign with HMAC-SHA256
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(signInput))
	signature := h.Sum(nil)
	signatureB64 := base64URLEncode(signature)

	return signInput + "." + signatureB64, nil
}

func base64URLEncode(data []byte) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	// Convert to base64url
	encoded = strings.ReplaceAll(encoded, "+", "-")
	encoded = strings.ReplaceAll(encoded, "/", "_")
	encoded = strings.TrimRight(encoded, "=")
	return encoded
}

