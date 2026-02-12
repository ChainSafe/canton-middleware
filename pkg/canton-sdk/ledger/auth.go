package ledger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// AuthProvider defines how the ledger client obtains
// and refreshes authentication tokens.
type AuthProvider interface {
	// Token returns a valid access token and its expiry time.
	// Implementations must cache and refresh tokens as needed.
	Token(ctx context.Context) (token string, expiry time.Time, err error)
}

// OAuthClientCredentialsProvider implements AuthProvider
// using the OAuth2 client credentials flow.
type OAuthClientCredentialsProvider struct {
	cfg        AuthConfig
	httpClient *http.Client
	leeway     time.Duration

	mu     sync.Mutex
	token  string
	expiry time.Time
}

// NewOAuthClientCredentialsProvider creates a new OAuthClientCredentialsProvider instance.
func NewOAuthClientCredentialsProvider(cfg AuthConfig, httpClient *http.Client) *OAuthClientCredentialsProvider {
	leeway := cfg.ExpiryLeeway
	if leeway == 0 {
		leeway = 60 * time.Second
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &OAuthClientCredentialsProvider{
		cfg:        cfg,
		httpClient: httpClient,
		leeway:     leeway,
	}
}

func (p *OAuthClientCredentialsProvider) Token(ctx context.Context) (string, time.Time, error) {
	if p.cfg.ClientID == "" || p.cfg.ClientSecret == "" || p.cfg.Audience == "" || p.cfg.TokenURL == "" {
		return "", time.Time{}, fmt.Errorf("no auth configured: OAuth2 client credentials are required")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	if p.token != "" && now.Before(p.expiry) {
		return p.token, p.expiry, nil
	}

	payload := map[string]string{
		"client_id":     p.cfg.ClientID,
		"client_secret": p.cfg.ClientSecret,
		"audience":      p.cfg.Audience,
		"grant_type":    "client_credentials",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("marshal token request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.TokenURL, bytes.NewReader(body))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("call token endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", time.Time{}, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(b))
	}

	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", time.Time{}, fmt.Errorf("decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", time.Time{}, fmt.Errorf("token response missing access_token")
	}

	expiry := now.Add(5 * time.Minute)
	if tr.ExpiresIn > 0 {
		exp := now.Add(time.Duration(tr.ExpiresIn) * time.Second)
		expiry = exp.Add(-p.leeway)
		if expiry.Before(now) {
			expiry = now.Add(time.Duration(tr.ExpiresIn/2) * time.Second)
		}
	}

	p.token = tr.AccessToken
	p.expiry = expiry
	return p.token, p.expiry, nil
}
