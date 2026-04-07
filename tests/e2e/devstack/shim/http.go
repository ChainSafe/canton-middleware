//go:build e2e

package shim

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// httpClient is a thin wrapper around *http.Client shared by all HTTP-based
// shims. It binds a base endpoint and provides get/getOK/post helpers so
// each shim only holds business logic.
type httpClient struct {
	endpoint string
	client   *http.Client
}

// getOK performs GET <endpoint><path> and returns an error if the response
// status is not 200. The response body is discarded.
func (h *httpClient) getOK(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.endpoint+path, nil)
	if err != nil {
		return err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d", path, resp.StatusCode)
	}
	return nil
}

// get performs GET <endpoint><path>[?query] and JSON-decodes the response into out.
// query may be nil.
func (h *httpClient) get(ctx context.Context, path string, query url.Values, out any) error {
	u := h.endpoint + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("GET %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// post performs POST <endpoint><path> with a JSON-encoded body and decodes the
// response into out. sig and msg are set as X-Signature / X-Message headers
// when non-empty (required by the transfer endpoints). out may be nil.
func (h *httpClient) post(ctx context.Context, path, sig, msg string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if sig != "" {
		req.Header.Set("X-Signature", sig)
		req.Header.Set("X-Message", msg)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: status %d: %s", path, resp.StatusCode, string(raw))
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response from %s: %w", path, err)
		}
	}
	return nil
}
