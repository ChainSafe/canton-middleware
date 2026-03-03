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

func (l *logService) ChainID() (chainID hexutil.Uint64) {
	start := time.Now()
	defer func() {
		l.logger.Info("ChainID completed",
			zap.String("service", ethServiceName),
			zap.String("method", "ChainID"),
			zap.Uint64("chain_id", uint64(chainID)),
			zap.Duration("duration", time.Since(start)),
		)
	}()
	chainID = l.svc.ChainID()
	return chainID
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
		l.logger.Info("BlockNumber completed", fields...)
	}()
	return l.svc.BlockNumber()
}

func (l *logService) GasPrice() (price *hexutil.Big, err error) {
	start := time.Now()
	defer func() {
		fields := []zap.Field{
			zap.String("service", ethServiceName),
			zap.String("method", "GasPrice"),
			zap.Bool("has_price", price != nil),
			zap.Duration("duration", time.Since(start)),
		}
		if err != nil {
			l.logger.Error("GasPrice failed", append(fields, zap.Error(err))...)
			return
		}
		l.logger.Info("GasPrice completed", fields...)
	}()
	return l.svc.GasPrice()
}

func (l *logService) MaxPriorityFeePerGas() (fee *hexutil.Big, err error) {
	start := time.Now()
	defer func() {
		fields := []zap.Field{
			zap.String("service", ethServiceName),
			zap.String("method", "MaxPriorityFeePerGas"),
			zap.Bool("has_fee", fee != nil),
			zap.Duration("duration", time.Since(start)),
		}
		if err != nil {
			l.logger.Error("MaxPriorityFeePerGas failed", append(fields, zap.Error(err))...)
			return
		}
		l.logger.Info("MaxPriorityFeePerGas completed", fields...)
	}()
	return l.svc.MaxPriorityFeePerGas()
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
			l.logger.Error("EstimateGas failed", append(fields, zap.Error(err))...)
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
			l.logger.Error("GetBalance failed", append(fields, zap.Error(err))...)
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
			l.logger.Error("GetTransactionCount failed", append(fields, zap.Error(err))...)
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
			l.logger.Error("GetCode failed", append(fields, zap.Error(err))...)
			return
		}
		l.logger.Info("GetCode completed", fields...)
	}()
	return l.svc.GetCode(ctx, address)
}

func (l *logService) Syncing() (syncing bool) {
	start := time.Now()
	defer func() {
		l.logger.Info("Syncing completed",
			zap.String("service", ethServiceName),
			zap.String("method", "Syncing"),
			zap.Bool("syncing", syncing),
			zap.Duration("duration", time.Since(start)),
		)
	}()
	syncing = l.svc.Syncing()
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
			l.logger.Error("GetTransactionByHash failed", append(fields, zap.Error(err))...)
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
			l.logger.Error("Call failed", append(fields, zap.Error(err))...)
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
			l.logger.Error("GetLogs failed", append(fields, zap.Error(err))...)
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
			l.logger.Error("GetBlockByNumber failed", append(fields, zap.Error(err))...)
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
			l.logger.Error("GetBlockByHash failed", append(fields, zap.Error(err))...)
			return
		}
		l.logger.Info("GetBlockByHash completed", fields...)
	}()
	return l.svc.GetBlockByHash(ctx, hash, fullTx)
}
