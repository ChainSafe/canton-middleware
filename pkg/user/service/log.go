package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/chainsafe/canton-middleware/pkg/user"
)

const serviceName = "RegistrationService"

const (
	logMessageMaxLen     = 50
	signatureDisplaySize = 16
)

// logService wraps Service with automatic logging of all method calls
type logService struct {
	svc    Service
	logger *zap.Logger
}

// NewLog creates a logging decorator for the registration Service.
// It logs method entry/exit, duration, errors, and sanitized request/response data.
func NewLog(svc Service, logger *zap.Logger) Service {
	return &logService{
		svc:    svc,
		logger: logger,
	}
}

// RegisterWeb3User wraps the service method with logging
func (ls *logService) RegisterWeb3User(
	ctx context.Context,
	req *user.RegisterRequest,
) (resp *user.RegisterResponse, err error) {
	start := time.Now()

	// Log method entry
	ls.logger.Info("RegisterWeb3User started",
		zap.String("service", serviceName),
		zap.String("method", "RegisterWeb3User"),
		zap.String("message", truncateString(req.Message, logMessageMaxLen)),
		zap.String("signature", redactSignature(req.Signature)),
	)

	// Execute the actual service method
	defer func() {
		duration := time.Since(start)

		if err != nil {
			// Log error case
			ls.logger.Error("RegisterWeb3User failed",
				zap.String("service", serviceName),
				zap.String("method", "RegisterWeb3User"),
				zap.Duration("duration", duration),
				zap.Error(err),
			)
		} else {
			// Log success case
			ls.logger.Info("RegisterWeb3User completed",
				zap.String("service", serviceName),
				zap.String("method", "RegisterWeb3User"),
				zap.String("evm_address", resp.EVMAddress),
				zap.String("party_id", resp.Party),
				zap.String("fingerprint", resp.Fingerprint),
				zap.String("mapping_cid", resp.MappingCID),
				zap.Duration("duration", duration),
			)
		}
	}()

	return ls.svc.RegisterWeb3User(ctx, req)
}

// RegisterCantonNativeUser wraps the service method with logging
func (ls *logService) RegisterCantonNativeUser(
	ctx context.Context,
	req *user.RegisterRequest,
) (resp *user.RegisterResponse, err error) {
	start := time.Now()

	// Log method entry
	ls.logger.Info("RegisterCantonNativeUser started",
		zap.String("service", serviceName),
		zap.String("method", "RegisterCantonNativeUser"),
		zap.String("canton_party_id", req.CantonPartyID),
		zap.String("message", truncateString(req.Message, logMessageMaxLen)),
		zap.String("canton_signature", redactSignature(req.CantonSignature)),
		zap.Bool("has_private_key", req.CantonPrivateKey != ""),
	)

	// Execute the actual service method
	defer func() {
		duration := time.Since(start)

		if err != nil {
			// Log error case
			ls.logger.Error("RegisterCantonNativeUser failed",
				zap.String("service", serviceName),
				zap.String("method", "RegisterCantonNativeUser"),
				zap.String("canton_party_id", req.CantonPartyID),
				zap.Duration("duration", duration),
				zap.Error(err),
			)
		} else {
			// Log success case
			ls.logger.Info("RegisterCantonNativeUser completed",
				zap.String("service", serviceName),
				zap.String("method", "RegisterCantonNativeUser"),
				zap.String("canton_party_id", req.CantonPartyID),
				zap.String("evm_address", resp.EVMAddress),
				zap.String("fingerprint", resp.Fingerprint),
				zap.String("mapping_cid", resp.MappingCID),
				zap.Bool("private_key_returned", resp.PrivateKey != ""),
				zap.Duration("duration", duration),
			)
		}
	}()

	return ls.svc.RegisterCantonNativeUser(ctx, req)
}

// Helper functions for sensitive data redaction

// truncateString limits string length for logging to prevent log spam
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// redactSignature redacts signature data to show only metadata
// Signatures are sensitive and should not be logged in full
func redactSignature(sig string) string {
	if sig == "" {
		return "<empty>"
	}
	sigLen := len(sig)
	if sigLen > signatureDisplaySize {
		// Show first 8 and last 4 characters with length
		return fmt.Sprintf("%s...%s (%d bytes)", sig[:8], sig[sigLen-4:], sigLen)
	}
	// For very short signatures, just show length
	return fmt.Sprintf("<%d bytes>", sigLen)
}
