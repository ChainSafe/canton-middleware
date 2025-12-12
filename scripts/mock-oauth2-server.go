// mock-oauth2-server.go - Simple OAuth2 mock server for local testing
//
// Usage:
//   go run scripts/mock-oauth2-server.go
//
// This server returns JWTs for local Canton testing. The tokens are signed
// with HS256 using a dummy secret (not for production use).

package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	port      = 8088
	jwtSecret = "local-dev-secret-do-not-use-in-production"
)

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func main() {
	http.HandleFunc("/oauth/token", handleToken)
	http.HandleFunc("/health", handleHealth)

	addr := fmt.Sprintf(":%d", port)
	log.Printf("Mock OAuth2 server starting on http://localhost%s", addr)
	log.Printf("POST /oauth/token - Returns JWT signed with HS256")
	log.Printf("GET  /health      - Health check")
	log.Fatal(http.ListenAndServe(addr, nil))
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse form data or JSON body (client_credentials grant)
	contentType := r.Header.Get("Content-Type")
	var clientID, audience string

	if strings.Contains(contentType, "application/json") {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "Failed to parse JSON body", http.StatusBadRequest)
			return
		}
		clientID = body["client_id"]
		audience = body["audience"]
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}
		clientID = r.FormValue("client_id")
		audience = r.FormValue("audience")
	}

	log.Printf("Token request: client_id=%s, audience=%s", clientID, audience)

	// Use client_id as the user ID (this becomes the JWT subject)
	// Canton uses this as the user_id in requests
	userID := clientID
	if userID == "" {
		userID = "local-user"
	}

	// Generate a properly signed JWT
	token := generateSignedJWT(userID, audience)

	resp := tokenResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   86400, // 24 hours
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)

	log.Printf("Issued token for user_id=%s (sub claim)", userID)
}

func generateSignedJWT(userID, audience string) string {
	// JWT Header
	header := map[string]interface{}{
		"alg": "HS256",
		"typ": "JWT",
	}

	// JWT Payload - include all standard claims
	now := time.Now().Unix()
	payload := map[string]interface{}{
		"iss":   "http://localhost:8088",
		"sub":   userID, // This is extracted by Go scripts as jwtSubject
		"aud":   audience,
		"iat":   now,
		"exp":   now + 86400, // 24 hours
		"scope": "daml_ledger_api",
	}

	headerJSON, _ := json.Marshal(header)
	payloadJSON, _ := json.Marshal(payload)

	// Base64url encode
	headerB64 := base64URLEncode(headerJSON)
	payloadB64 := base64URLEncode(payloadJSON)

	// Create signature
	signingInput := headerB64 + "." + payloadB64
	signature := signHS256(signingInput, jwtSecret)

	return signingInput + "." + signature
}

func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func signHS256(input, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(input))
	return base64URLEncode(h.Sum(nil))
}
