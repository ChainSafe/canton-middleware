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

// ListIncoming wraps the service method with logging.
func (ls *logService) ListIncoming(ctx context.Context, evmAddr string) (resp *ListIncomingResponse, err error) {
	start := time.Now()
	ls.logger.Info("ListIncoming started",
		zap.String("service", transferServiceName),
		zap.String("method", "ListIncoming"),
		zap.String("evm_addr", evmAddr),
	)
	defer func() {
		duration := time.Since(start)
		if err != nil {
			ls.logger.Error("ListIncoming failed",
				zap.String("service", transferServiceName),
				zap.String("method", "ListIncoming"),
				zap.Duration("duration", duration),
				zap.Error(err),
			)
		} else {
			ls.logger.Info("ListIncoming completed",
				zap.String("service", transferServiceName),
				zap.String("method", "ListIncoming"),
				zap.Int("total", resp.Total),
				zap.Duration("duration", duration),
			)
		}
	}()
	return ls.svc.ListIncoming(ctx, evmAddr)
}

// PrepareAccept wraps the service method with logging.
func (ls *logService) PrepareAccept(
	ctx context.Context, evmAddr, contractID string, req *PrepareAcceptRequest,
) (resp *PrepareResponse, err error) {
	start := time.Now()
	ls.logger.Info("PrepareAccept started",
		zap.String("service", transferServiceName),
		zap.String("method", "PrepareAccept"),
		zap.String("evm_addr", evmAddr),
		zap.String("contract_id", contractID),
		zap.String("instrument_admin", req.InstrumentAdmin),
	)
	defer func() {
		duration := time.Since(start)
		if err != nil {
			ls.logger.Error("PrepareAccept failed",
				zap.String("service", transferServiceName),
				zap.String("method", "PrepareAccept"),
				zap.String("contract_id", contractID),
				zap.Duration("duration", duration),
				zap.Error(err),
			)
		} else {
			ls.logger.Info("PrepareAccept completed",
				zap.String("service", transferServiceName),
				zap.String("method", "PrepareAccept"),
				zap.String("transfer_id", resp.TransferID),
				zap.String("party_id", resp.PartyID),
				zap.Duration("duration", duration),
			)
		}
	}()
	return ls.svc.PrepareAccept(ctx, evmAddr, contractID, req)
}

// ExecuteAccept wraps the service method with logging.
func (ls *logService) ExecuteAccept(
	ctx context.Context, evmAddr string, req *ExecuteRequest,
) (resp *ExecuteResponse, err error) {
	start := time.Now()
	ls.logger.Info("ExecuteAccept started",
		zap.String("service", transferServiceName),
		zap.String("method", "ExecuteAccept"),
		zap.String("evm_addr", evmAddr),
		zap.String("transfer_id", req.TransferID),
		zap.String("signature", redactSignature(req.Signature)),
		zap.String("signed_by", req.SignedBy),
	)
	defer func() {
		duration := time.Since(start)
		if err != nil {
			ls.logger.Error("ExecuteAccept failed",
				zap.String("service", transferServiceName),
				zap.String("method", "ExecuteAccept"),
				zap.String("transfer_id", req.TransferID),
				zap.Duration("duration", duration),
				zap.Error(err),
			)
		} else {
			ls.logger.Info("ExecuteAccept completed",
				zap.String("service", transferServiceName),
				zap.String("method", "ExecuteAccept"),
				zap.String("transfer_id", req.TransferID),
				zap.String("status", resp.Status),
				zap.Duration("duration", duration),
			)
		}
	}()
	return ls.svc.ExecuteAccept(ctx, evmAddr, req)
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
