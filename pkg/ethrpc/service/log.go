package service

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"go.uber.org/zap"

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

func (l *logService) ChainID() hexutil.Uint64 {
	return l.svc.ChainID()
}

func (l *logService) BlockNumber() (n hexutil.Uint64, err error) {
	start := time.Now()
	defer func() {
		fields := []zap.Field{
			zap.String("service", ethServiceName),
			zap.String("method", "BlockNumber"),
			zap.Uint64("block", uint64(n)),
			zap.Duration("duration", time.Since(start)),
		}
		if err != nil {
			l.logger.Error("BlockNumber failed", append(fields, zap.Error(err))...)
			return
		}
		l.logger.Debug("BlockNumber completed", fields...)
	}()
	return l.svc.BlockNumber()
}

func (l *logService) GasPrice() (*hexutil.Big, error) {
	return l.svc.GasPrice()
}

func (l *logService) MaxPriorityFeePerGas() (*hexutil.Big, error) {
	return l.svc.MaxPriorityFeePerGas()
}

func (l *logService) EstimateGas(ctx context.Context, args *ethrpc.CallArgs) (hexutil.Uint64, error) {
	return l.svc.EstimateGas(ctx, args)
}

func (l *logService) GetBalance(ctx context.Context, address common.Address) (bal *hexutil.Big, err error) {
	start := time.Now()
	defer func() {
		fields := []zap.Field{
			zap.String("service", ethServiceName),
			zap.String("method", "getBalance"),
			zap.String("address", address.Hex()),
			zap.Duration("duration", time.Since(start)),
		}
		if err != nil {
			l.logger.Error("getBalance failed", append(fields, zap.Error(err))...)
			return
		}
		l.logger.Debug("getBalance completed", fields...)
	}()
	return l.svc.GetBalance(ctx, address)
}

func (l *logService) GetTransactionCount(ctx context.Context, address common.Address) (hexutil.Uint64, error) {
	return l.svc.GetTransactionCount(ctx, address)
}

func (l *logService) GetCode(ctx context.Context, address common.Address) (hexutil.Bytes, error) {
	return l.svc.GetCode(ctx, address)
}

func (l *logService) Syncing() bool {
	return l.svc.Syncing()
}

func (l *logService) SendRawTransaction(ctx context.Context, data hexutil.Bytes) (hash common.Hash, err error) {
	start := time.Now()

	l.logger.Info("SendRawTransaction started",
		zap.String("service", ethServiceName),
		zap.String("method", "SendRawTransaction"),
		zap.Int("data_len", len(data)),
	)

	defer func() {
		fields := []zap.Field{
			zap.String("service", ethServiceName),
			zap.String("method", "SendRawTransaction"),
			zap.String("tx_hash", hash.Hex()),
			zap.Duration("duration", time.Since(start)),
		}
		if err != nil {
			l.logger.Error("SendRawTransaction failed", append(fields, zap.Error(err))...)
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
			l.logger.Error("GetTransactionReceipt failed", append(fields, zap.Error(err))...)
			return
		}
		l.logger.Debug("GetTransactionReceipt completed", fields...)
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
			l.logger.Error("GetTransactionByHash failed", append(fields, zap.Error(err))...)
			return
		}
		l.logger.Debug("GetTransactionByHash completed", fields...)
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
			l.logger.Error("Call failed", append(fields, zap.Error(err))...)
			return
		}
		l.logger.Debug("Call completed", fields...)
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
			l.logger.Error("GetLogs failed", append(fields, zap.Error(err))...)
			return
		}
		l.logger.Debug("GetLogs completed", fields...)
	}()
	return l.svc.GetLogs(ctx, query)
}

func (l *logService) GetBlockByNumber(ctx context.Context, blockNr ethrpc.BlockNumberOrHash, fullTx bool) (*ethrpc.RPCBlock, error) {
	return l.svc.GetBlockByNumber(ctx, blockNr, fullTx)
}

func (l *logService) GetBlockByHash(ctx context.Context, hash common.Hash, fullTx bool) (*ethrpc.RPCBlock, error) {
	return l.svc.GetBlockByHash(ctx, hash, fullTx)
}
