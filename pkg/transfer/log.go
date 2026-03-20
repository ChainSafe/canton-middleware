package transfer

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

const (
	transferServiceName  = "TransferService"
	signatureDisplaySize = 16
)

// logService wraps Service with automatic logging of all method calls.
type logService struct {
	svc    Service
	logger *zap.Logger
}

// NewLog creates a logging decorator for the transfer Service.
// It logs method entry/exit, duration, errors, and sanitized request/response data.
func NewLog(svc Service, logger *zap.Logger) Service {
	return &logService{
		svc:    svc,
		logger: logger,
	}
}

// Prepare wraps the service method with logging.
func (ls *logService) Prepare(
	ctx context.Context,
	senderEVMAddr string,
	req *PrepareRequest,
) (resp *PrepareResponse, err error) {
	start := time.Now()

	ls.logger.Info("Prepare started",
		zap.String("service", transferServiceName),
		zap.String("method", "Prepare"),
		zap.String("sender", senderEVMAddr),
		zap.String("to", req.To),
		zap.String("amount", req.Amount),
		zap.String("token", req.Token),
	)

	defer func() {
		duration := time.Since(start)

		if err != nil {
			ls.logger.Error("Prepare failed",
				zap.String("service", transferServiceName),
				zap.String("method", "Prepare"),
				zap.String("sender", senderEVMAddr),
				zap.Duration("duration", duration),
				zap.Error(err),
			)
		} else {
			ls.logger.Info("Prepare completed",
				zap.String("service", transferServiceName),
				zap.String("method", "Prepare"),
				zap.String("sender", senderEVMAddr),
				zap.String("transfer_id", resp.TransferID),
				zap.String("party_id", resp.PartyID),
				zap.String("expires_at", resp.ExpiresAt),
				zap.Duration("duration", duration),
			)
		}
	}()

	return ls.svc.Prepare(ctx, senderEVMAddr, req)
}

// Execute wraps the service method with logging.
func (ls *logService) Execute(
	ctx context.Context,
	senderEVMAddr string,
	req *ExecuteRequest,
) (resp *ExecuteResponse, err error) {
	start := time.Now()

	ls.logger.Info("Execute started",
		zap.String("service", transferServiceName),
		zap.String("method", "Execute"),
		zap.String("sender", senderEVMAddr),
		zap.String("transfer_id", req.TransferID),
		zap.String("signature", redactSignature(req.Signature)),
		zap.String("signed_by", req.SignedBy),
	)

	defer func() {
		duration := time.Since(start)

		if err != nil {
			ls.logger.Error("Execute failed",
				zap.String("service", transferServiceName),
				zap.String("method", "Execute"),
				zap.String("sender", senderEVMAddr),
				zap.String("transfer_id", req.TransferID),
				zap.Duration("duration", duration),
				zap.Error(err),
			)
		} else {
			ls.logger.Info("Execute completed",
				zap.String("service", transferServiceName),
				zap.String("method", "Execute"),
				zap.String("sender", senderEVMAddr),
				zap.String("transfer_id", req.TransferID),
				zap.String("status", resp.Status),
				zap.Duration("duration", duration),
			)
		}
	}()

	return ls.svc.Execute(ctx, senderEVMAddr, req)
}

// redactSignature redacts signature data to show only metadata.
// Signatures are sensitive and should not be logged in full.
func redactSignature(sig string) string {
	if sig == "" {
		return "<empty>"
	}
	sigLen := len(sig)
	if sigLen > signatureDisplaySize {
		return fmt.Sprintf("%s...%s (%d bytes)", sig[:8], sig[sigLen-4:], sigLen)
	}
	return fmt.Sprintf("<%d bytes>", sigLen)
}
