// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/chainsafe/canton-middleware/pkg/relayer"
)

const serviceName = "RelayerService"

// logService wraps Service with automatic logging of all method calls.
type logService struct {
	svc    Service
	logger *zap.Logger
}

// NewLog creates a logging decorator for the relayer Service.
// It logs method entry/exit, duration, and errors.
func NewLog(svc Service, logger *zap.Logger) Service {
	return &logService{svc: svc, logger: logger}
}

func (ls *logService) ListTransfers(ctx context.Context, limit int) (transfers []*relayer.Transfer, err error) {
	start := time.Now()
	ls.logger.Info("ListTransfers started",
		zap.String("service", serviceName),
		zap.Int("limit", limit),
	)
	defer func() {
		duration := time.Since(start)
		if err != nil {
			ls.logger.Error("ListTransfers failed",
				zap.String("service", serviceName),
				zap.Duration("duration", duration),
				zap.Error(err),
			)
		} else {
			ls.logger.Info("ListTransfers completed",
				zap.String("service", serviceName),
				zap.Int("count", len(transfers)),
				zap.Duration("duration", duration),
			)
		}
	}()
	return ls.svc.ListTransfers(ctx, limit)
}

func (ls *logService) RegisterTransfer(
	ctx context.Context,
	req *relayer.RegisterTransferRequest,
) (resp *relayer.RegisterTransferResponse, err error) {
	start := time.Now()
	ls.logger.Info("RegisterTransfer started",
		zap.String("service", serviceName),
		zap.String("bridge_key", req.BridgeKey),
		zap.String("token", req.TokenSymbol),
		zap.String("source_tx_hash", req.SourceTxHash),
	)
	defer func() {
		duration := time.Since(start)
		if err != nil {
			ls.logger.Error("RegisterTransfer failed",
				zap.String("service", serviceName),
				zap.String("source_tx_hash", req.SourceTxHash),
				zap.Duration("duration", duration),
				zap.Error(err),
			)
		} else {
			ls.logger.Info("RegisterTransfer completed",
				zap.String("service", serviceName),
				zap.String("id", resp.Transfer.ID),
				zap.Bool("created", resp.Created),
				zap.Duration("duration", duration),
			)
		}
	}()
	return ls.svc.RegisterTransfer(ctx, req)
}

func (ls *logService) GetTransfer(ctx context.Context, id string) (transfer *relayer.Transfer, err error) {
	start := time.Now()
	ls.logger.Info("GetTransfer started",
		zap.String("service", serviceName),
		zap.String("id", id),
	)
	defer func() {
		duration := time.Since(start)
		if err != nil {
			ls.logger.Error("GetTransfer failed",
				zap.String("service", serviceName),
				zap.String("id", id),
				zap.Duration("duration", duration),
				zap.Error(err),
			)
		} else {
			ls.logger.Info("GetTransfer completed",
				zap.String("service", serviceName),
				zap.String("id", id),
				zap.Bool("found", transfer != nil),
				zap.Duration("duration", duration),
			)
		}
	}()
	return ls.svc.GetTransfer(ctx, id)
}
