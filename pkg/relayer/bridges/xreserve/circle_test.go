// SPDX-License-Identifier: Apache-2.0

package xreserve

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func attestationServer(t *testing.T, status int, body string) *HTTPAttestationClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/attestations/0xdeposit" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	client, err := NewAttestationClient(srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("NewAttestationClient failed: %v", err)
	}
	return client
}

func TestAttestationClient_Complete(t *testing.T) {
	client := attestationServer(t, http.StatusOK, `{"id":"att-1","status":"complete"}`)

	att, err := client.GetAttestation(context.Background(), "0xdeposit")
	if err != nil {
		t.Fatalf("GetAttestation failed: %v", err)
	}
	if att.ID != "att-1" {
		t.Fatalf("attestation id = %q, want att-1", att.ID)
	}
}

func TestAttestationClient_PendingStatus_NotReady(t *testing.T) {
	client := attestationServer(t, http.StatusOK, `{"id":"att-1","status":"pending"}`)

	if _, err := client.GetAttestation(context.Background(), "0xdeposit"); !errors.Is(err, ErrAttestationNotReady) {
		t.Fatalf("err = %v, want ErrAttestationNotReady", err)
	}
}

func TestAttestationClient_NotFound_NotReady(t *testing.T) {
	client := attestationServer(t, http.StatusNotFound, `{}`)

	if _, err := client.GetAttestation(context.Background(), "0xdeposit"); !errors.Is(err, ErrAttestationNotReady) {
		t.Fatalf("err = %v, want ErrAttestationNotReady", err)
	}
}

func TestAttestationClient_ServerError_Unavailable(t *testing.T) {
	client := attestationServer(t, http.StatusServiceUnavailable, "down")

	if _, err := client.GetAttestation(context.Background(), "0xdeposit"); !errors.Is(err, ErrAttestationUnavailable) {
		t.Fatalf("err = %v, want ErrAttestationUnavailable", err)
	}
}

func TestAttestationClient_ClientError_IsHardError(t *testing.T) {
	client := attestationServer(t, http.StatusBadRequest, "bad request")

	_, err := client.GetAttestation(context.Background(), "0xdeposit")
	if err == nil || errors.Is(err, ErrAttestationNotReady) || errors.Is(err, ErrAttestationUnavailable) {
		t.Fatalf("err = %v, want a hard error", err)
	}
}

func TestAttestationClient_ConnectionRefused_Unavailable(t *testing.T) {
	client, err := NewAttestationClient("http://127.0.0.1:1", nil)
	if err != nil {
		t.Fatalf("NewAttestationClient failed: %v", err)
	}

	if _, err = client.GetAttestation(context.Background(), "0xdeposit"); !errors.Is(err, ErrAttestationUnavailable) {
		t.Fatalf("err = %v, want ErrAttestationUnavailable", err)
	}
}

func TestNewAttestationClient_RejectsInvalidURL(t *testing.T) {
	for _, bad := range []string{"", "not-a-url", "ftp://host", "http://"} {
		if _, err := NewAttestationClient(bad, nil); err == nil {
			t.Fatalf("NewAttestationClient(%q) should fail", bad)
		}
	}
}
