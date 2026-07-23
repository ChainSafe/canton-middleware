// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/chainsafe/canton-middleware/pkg/auth/jwt"
)

const loginServiceName = "LoginService"

// logService wraps Service with logging of the login call. Nonce and JWKS are
// trivial accessors and pass through unlogged; Login is the meaningful operation,
// so its entry/exit, duration, and errors are recorded (never the message or
// signature, which are sensitive).
type logService struct {
	svc    Service
	logger *zap.Logger
}

// NewLog creates a logging decorator for the login Service.
func NewLog(svc Service, logger *zap.Logger) Service {
	return &logService{svc: svc, logger: logger}
}

func (ls *logService) Nonce(address string) (string, error) { return ls.svc.Nonce(address) }

func (ls *logService) JWKS() jwt.JWKS { return ls.svc.JWKS() }

func (ls *logService) Login(ctx context.Context, message, signature string) (resp *auth.LoginResponse, err error) {
	start := time.Now()

	ls.logger.Info("Login started",
		zap.String("service", loginServiceName),
		zap.String("method", "Login"),
	)

	defer func() {
		duration := time.Since(start)

		if err != nil {
			ls.logger.Error("Login failed",
				zap.String("service", loginServiceName),
				zap.String("method", "Login"),
				zap.Duration("duration", duration),
				zap.Error(err),
			)
		} else {
			fields := []zap.Field{
				zap.String("service", loginServiceName),
				zap.String("method", "Login"),
				zap.Duration("duration", duration),
			}
			// resp is non-nil on the nil-error path, but guard defensively: this
			// decorator wraps an interface any implementation could satisfy.
			if resp != nil {
				fields = append(fields, zap.Int64("expires_at", resp.ExpiresAt))
			}
			ls.logger.Info("Login completed", fields...)
		}
	}()

	return ls.svc.Login(ctx, message, signature)
}
