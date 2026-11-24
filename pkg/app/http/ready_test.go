// SPDX-License-Identifier: Apache-2.0

package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func TestReadyHandler(t *testing.T) {
	ok := ReadinessCheck{Name: "database", Check: func(context.Context) error { return nil }}
	failing := ReadinessCheck{Name: "ledger", Check: func(context.Context) error { return errors.New("dial: secret detail") }}

	tests := []struct {
		name     string
		checks   []ReadinessCheck
		wantCode int
		wantBody string
	}{
		{"no checks", nil, http.StatusOK, "READY"},
		{"all pass", []ReadinessCheck{ok, ok}, http.StatusOK, "READY"},
		{"one fails", []ReadinessCheck{ok, failing}, http.StatusServiceUnavailable, "NOT_READY: ledger"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ReadyHandler(zap.NewNop(), tt.checks...)(rec, httptest.NewRequest(http.MethodGet, ReadyPath, http.NoBody))

			if rec.Code != tt.wantCode {
				t.Errorf("code = %d, want %d", rec.Code, tt.wantCode)
			}
			if rec.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}
