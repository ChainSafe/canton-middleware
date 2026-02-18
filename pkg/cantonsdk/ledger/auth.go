package ledger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	defaultExpiryLeeway = 60 * time.Second
	defaultHTTPTimeout  = 10 * time.Second

	// If token endpoint doesn't give expires_in, use a conservative fallback.
	fallbackTokenTTL = 5 * time.Minute

	// Limit error-body reads so we don't accidentally slurp huge responses.
	maxErrBodyBytes = 4096

	halfDivisor = 2
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
	cfg        *AuthConfig
	httpClient *http.Client
	leeway     time.Duration

	mu     sync.Mutex
	token  string
	expiry time.Time
}

// NewOAuthClientCredentialsProvider creates a new OAuthClientCredentialsProvider instance.
func NewOAuthClientCredentialsProvider(cfg *AuthConfig, httpClient *http.Client) *OAuthClientCredentialsProvider {
	leeway := cfg.ExpiryLeeway
	if leeway == 0 {
		leeway = defaultExpiryLeeway
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}

	return &OAuthClientCredentialsProvider{
		cfg:        cfg,
		httpClient: httpClient,
		leeway:     leeway,
	}
}

func (p *OAuthClientCredentialsProvider) Token(ctx context.Context) (string, time.Time, error) {
	if err := p.cfg.validate(); err != nil {
		return "", time.Time{}, err
	}

	// Fast path: return cached token if still valid.
	p.mu.Lock()
	if p.token != "" && time.Now().Before(p.expiry) {
		tok, exp := p.token, p.expiry
		p.mu.Unlock()
		return tok, exp, nil
	}
	p.mu.Unlock()

	// Fetch without holding the mutex.
	token, expiry, err := p.fetchToken(ctx)
	if err != nil {
		return "", time.Time{}, err
	}

	p.mu.Lock()
	p.token = token
	p.expiry = expiry
	p.mu.Unlock()

	return token, expiry, nil
}

func (p *OAuthClientCredentialsProvider) fetchToken(ctx context.Context) (string, time.Time, error) {
	body, err := p.marshalTokenRequest()
	if err != nil {
		return "", time.Time{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.TokenURL, bytes.NewReader(body))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", time.Time{}, err
		}
		return "", time.Time{}, fmt.Errorf("call token endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, readHTTPError(resp)
	}

	tr, err := decodeTokenResponse(resp.Body)
	if err != nil {
		return "", time.Time{}, err
	}

	now := time.Now()
	expiry := computeRefreshBy(now, tr.ExpiresIn, p.leeway)

	return tr.AccessToken, expiry, nil
}

func (p *OAuthClientCredentialsProvider) marshalTokenRequest() ([]byte, error) {
	payload := map[string]string{
		"client_id":     p.cfg.ClientID,
		"client_secret": p.cfg.ClientSecret,
		"audience":      p.cfg.Audience,
		"grant_type":    "client_credentials",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal token request: %w", err)
	}
	return body, nil
}

func readHTTPError(resp *http.Response) error {
	limited := io.LimitReader(resp.Body, maxErrBodyBytes)

	b, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("token endpoint returned %d and body read failed: %w", resp.StatusCode, err)
	}

	return fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(b))
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

func decodeTokenResponse(r io.Reader) (tokenResponse, error) {
	var tr tokenResponse

	dec := json.NewDecoder(r)
	if err := dec.Decode(&tr); err != nil {
		return tokenResponse{}, fmt.Errorf("decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return tokenResponse{}, fmt.Errorf("token response missing access_token")
	}

	return tr, nil
}

// computeRefreshBy returns a "refresh-by" timestamp, leeway-adjusted.
func computeRefreshBy(now time.Time, expiresInSeconds int, leeway time.Duration) time.Time {
	if expiresInSeconds <= 0 {
		return now.Add(fallbackTokenTTL)
	}

	exp := now.Add(time.Duration(expiresInSeconds) * time.Second)
	refreshBy := exp.Add(-leeway)

	// If leeway overshoots, fall back to a reasonable midpoint.
	if refreshBy.Before(now) {
		half := expiresInSeconds / halfDivisor
		return now.Add(time.Duration(half) * time.Second)
	}

	return refreshBy
}
