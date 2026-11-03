// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"go.uber.org/zap"

	apperr "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc"
)

const ethServiceName = "EthService"

// logService wraps Service with automatic method-level logging.
type logService struct {
	svc    Service
	logger *zap.Logger
}

// NewLog creates a logging decorator for Service.
// Returns svc unchanged if logger is nil.
func NewLog(svc Service, logger *zap.Logger) Service {
	if logger == nil {
		return svc
	}
	return &logService{svc: svc, logger: logger}
}

// logErr emits a method's failure log line at a severity chosen by error kind,
// so routine client-side rejections don't masquerade as server failures. Wallets
// like MetaMask constantly probe the facade — polling stale imported tokens,
// calling unsupported methods, sending contract-creation-shaped calls — and
// every such probe surfaces here as a client-category error (bad request,
// not-supported, forbidden, …). Logging those at ERROR buries the genuine faults
// among noise.
//
//	server/internal → Error (store, Canton, or other downstream failure)
//	client error    → Warn when warnOnClientErr, else Debug
//
// SendRawTransaction passes warnOnClientErr=true: a rejected transfer attempt
// (e.g. a non-whitelisted sender) is worth an operator's attention without being
// an Error, whereas read-path probes (Call) stay at Debug.
func (l *logService) logErr(failedMsg string, warnOnClientErr bool, err error, fields []zap.Field) {
	fields = append(fields, zap.Error(err))
	switch {
	case apperr.IsInternalError(err):
		l.logger.Error(failedMsg, fields...)
	case warnOnClientErr:
		l.logger.Warn(failedMsg, fields...)
	default:
		l.logger.Debug(failedMsg, fields...)
	}
}

func (l *logService) ChainID(ctx context.Context) (chainID hexutil.Uint64) {
	start := time.Now()
	defer func() {
		l.logger.Info("ChainID completed",
			zap.String("service", ethServiceName),
			zap.String("method", "ChainID"),
			zap.Uint64("chain_id", uint64(chainID)),
			zap.Duration("duration", time.Since(start)),
		)
	}()
	chainID = l.svc.ChainID(ctx)
	return chainID
}

func (l *logService) BlockNumber(ctx context.Context) (n hexutil.Uint64, err error) {
	start := time.Now()
	defer func() {
		fields := []zap.Field{
			zap.String("service", ethServiceName),
			zap.String("method", "BlockNumber"),
			zap.Uint64("block", uint64(n)),
			zap.Duration("duration", time.Since(start)),
		}
		if err != nil {
			l.logErr("BlockNumber failed", false, err, fields)
			return
		}
		l.logger.Info("BlockNumber completed", fields...)
	}()
	return l.svc.BlockNumber(ctx)
}

func (l *logService) GasPrice(ctx context.Context) (price *hexutil.Big, err error) {
	start := time.Now()
	defer func() {
		fields := []zap.Field{
			zap.String("service", ethServiceName),
			zap.String("method", "GasPrice"),
			zap.Bool("has_price", price != nil),
			zap.Duration("duration", time.Since(start)),
		}
		if err != nil {
			l.logErr("GasPrice failed", false, err, fields)
			return
		}
		l.logger.Info("GasPrice completed", fields...)
	}()
	return l.svc.GasPrice(ctx)
}

func (l *logService) MaxPriorityFeePerGas(ctx context.Context) (fee *hexutil.Big, err error) {
	start := time.Now()
	defer func() {
		fields := []zap.Field{
			zap.String("service", ethServiceName),
			zap.String("method", "MaxPriorityFeePerGas"),
			zap.Bool("has_fee", fee != nil),
			zap.Duration("duration", time.Since(start)),
		}
		if err != nil {
			l.logErr("MaxPriorityFeePerGas failed", false, err, fields)
			return
		}
		l.logger.Info("MaxPriorityFeePerGas completed", fields...)
	}()
	return l.svc.MaxPriorityFeePerGas(ctx)
}

func (l *logService) EstimateGas(ctx context.Context, args *ethrpc.CallArgs) (gas hexutil.Uint64, err error) {
	start := time.Now()
	to := ""
	if args != nil && args.To != nil {
		to = args.To.Hex()
	}

	defer func() {
		fields := []zap.Field{
			zap.String("service", ethServiceName),
			zap.String("method", "EstimateGas"),
			zap.String("to", to),
			zap.Uint64("gas", uint64(gas)),
			zap.Duration("duration", time.Since(start)),
		}
		if err != nil {
			l.logErr("EstimateGas failed", false, err, fields)
			return
		}
		l.logger.Info("EstimateGas completed", fields...)
	}()
	return l.svc.EstimateGas(ctx, args)
}

func (l *logService) GetBalance(ctx context.Context, address common.Address) (bal *hexutil.Big, err error) {
	start := time.Now()
	defer func() {
		fields := []zap.Field{
			zap.String("service", ethServiceName),
			zap.String("method", "GetBalance"),
			zap.String("address", address.Hex()),
			zap.Duration("duration", time.Since(start)),
		}
		if err != nil {
			l.logErr("GetBalance failed", false, err, fields)
			return
		}
		l.logger.Info("GetBalance completed", fields...)
	}()
	return l.svc.GetBalance(ctx, address)
}

func (l *logService) GetTransactionCount(ctx context.Context, address common.Address) (count hexutil.Uint64, err error) {
	start := time.Now()
	defer func() {
		fields := []zap.Field{
			zap.String("service", ethServiceName),
			zap.String("method", "GetTransactionCount"),
			zap.String("address", address.Hex()),
			zap.Uint64("count", uint64(count)),
			zap.Duration("duration", time.Since(start)),
		}
		if err != nil {
			l.logErr("GetTransactionCount failed", false, err, fields)
			return
		}
		l.logger.Info("GetTransactionCount completed", fields...)
	}()
	return l.svc.GetTransactionCount(ctx, address)
}

func (l *logService) GetCode(ctx context.Context, address common.Address) (code hexutil.Bytes, err error) {
	start := time.Now()
	defer func() {
		fields := []zap.Field{
			zap.String("service", ethServiceName),
			zap.String("method", "GetCode"),
			zap.String("address", address.Hex()),
			zap.Int("code_len", len(code)),
			zap.Duration("duration", time.Since(start)),
		}
		if err != nil {
			l.logErr("GetCode failed", false, err, fields)
			return
		}
		l.logger.Info("GetCode completed", fields...)
	}()
	return l.svc.GetCode(ctx, address)
}

func (l *logService) Syncing(ctx context.Context) (syncing bool) {
	start := time.Now()
	defer func() {
		l.logger.Info("Syncing completed",
			zap.String("service", ethServiceName),
			zap.String("method", "Syncing"),
			zap.Bool("syncing", syncing),
			zap.Duration("duration", time.Since(start)),
		)
	}()
	syncing = l.svc.Syncing(ctx)
	return syncing
}

func (l *logService) SendRawTransaction(ctx context.Context, data hexutil.Bytes) (hash common.Hash, err error) {
	start := time.Now()

	defer func() {
		fields := []zap.Field{
			zap.String("service", ethServiceName),
			zap.String("method", "SendRawTransaction"),
			zap.String("tx_hash", hash.Hex()),
			zap.Duration("duration", time.Since(start)),
		}
		if err != nil {
			// warnOnClientErr: a rejected transfer (e.g. non-whitelisted sender)
			// is operator-relevant but not a server fault, so it logs at Warn.
			l.logErr("SendRawTransaction failed", true, err, fields)
			return
		}
		l.logger.Info("SendRawTransaction completed", fields...)
	}()

	return l.svc.SendRawTransaction(ctx, data)
}

func (l *logService) GetTransactionReceipt(ctx context.Context, hash common.Hash) (receipt *ethrpc.RPCReceipt, err error) {
	start := time.Now()
	defer func() {
		fields := []zap.Field{
			zap.String("service", ethServiceName),
			zap.String("method", "GetTransactionReceipt"),
			zap.String("tx_hash", hash.Hex()),
			zap.Bool("found", receipt != nil),
			zap.Duration("duration", time.Since(start)),
		}
		if err != nil {
			l.logErr("GetTransactionReceipt failed", false, err, fields)
			return
		}
		l.logger.Info("GetTransactionReceipt completed", fields...)
	}()
	return l.svc.GetTransactionReceipt(ctx, hash)
}

func (l *logService) GetTransactionByHash(ctx context.Context, hash common.Hash) (tx *ethrpc.RPCTransaction, err error) {
	start := time.Now()
	defer func() {
		fields := []zap.Field{
			zap.String("service", ethServiceName),
			zap.String("method", "GetTransactionByHash"),
			zap.String("tx_hash", hash.Hex()),
			zap.Bool("found", tx != nil),
			zap.Duration("duration", time.Since(start)),
		}
		if err != nil {
			l.logErr("GetTransactionByHash failed", false, err, fields)
			return
		}
		l.logger.Info("GetTransactionByHash completed", fields...)
	}()
	return l.svc.GetTransactionByHash(ctx, hash)
}

func (l *logService) Call(ctx context.Context, args *ethrpc.CallArgs) (result hexutil.Bytes, err error) {
	start := time.Now()

	to := ""
	if args != nil && args.To != nil {
		to = args.To.Hex()
	}

	defer func() {
		fields := []zap.Field{
			zap.String("service", ethServiceName),
			zap.String("method", "Call"),
			zap.String("to", to),
			zap.Duration("duration", time.Since(start)),
		}
		if err != nil {
			// Read-path probe failures (unknown method, unsupported/stale token)
			// are routine wallet behavior, not faults — keep them at Debug.
			l.logErr("Call failed", false, err, fields)
			return
		}
		l.logger.Info("Call completed", fields...)
	}()

	return l.svc.Call(ctx, args)
}

func (l *logService) GetLogs(ctx context.Context, query ethrpc.FilterQuery) (logs []*types.Log, err error) {
	start := time.Now()
	defer func() {
		fields := []zap.Field{
			zap.String("service", ethServiceName),
			zap.String("method", "GetLogs"),
			zap.Int("count", len(logs)),
			zap.Duration("duration", time.Since(start)),
		}
		if err != nil {
			l.logErr("GetLogs failed", false, err, fields)
			return
		}
		l.logger.Info("GetLogs completed", fields...)
	}()
	return l.svc.GetLogs(ctx, query)
}

func (l *logService) GetBlockByNumber(
	ctx context.Context,
	blockNr ethrpc.BlockNumberOrHash,
	fullTx bool,
) (block *ethrpc.RPCBlock, err error) {
	start := time.Now()
	defer func() {
		fields := []zap.Field{
			zap.String("service", ethServiceName),
			zap.String("method", "GetBlockByNumber"),
			zap.Bool("full_tx", fullTx),
			zap.Bool("found", block != nil),
			zap.Duration("duration", time.Since(start)),
		}
		if blockNr.BlockNumber != nil {
			fields = append(fields, zap.Uint64("block_number", uint64(*blockNr.BlockNumber)))
		}
		if blockNr.BlockHash != nil {
			fields = append(fields, zap.String("block_hash", blockNr.BlockHash.Hex()))
		}
		if err != nil {
			l.logErr("GetBlockByNumber failed", false, err, fields)
			return
		}
		l.logger.Info("GetBlockByNumber completed", fields...)
	}()
	return l.svc.GetBlockByNumber(ctx, blockNr, fullTx)
}

func (l *logService) GetBlockByHash(
	ctx context.Context,
	hash common.Hash,
	fullTx bool,
) (block *ethrpc.RPCBlock, err error) {
	start := time.Now()
	defer func() {
		fields := []zap.Field{
			zap.String("service", ethServiceName),
			zap.String("method", "GetBlockByHash"),
			zap.String("block_hash", hash.Hex()),
			zap.Bool("full_tx", fullTx),
			zap.Bool("found", block != nil),
			zap.Duration("duration", time.Since(start)),
		}
		if err != nil {
			l.logErr("GetBlockByHash failed", false, err, fields)
			return
		}
		l.logger.Info("GetBlockByHash completed", fields...)
	}()
	return l.svc.GetBlockByHash(ctx, hash, fullTx)
}
