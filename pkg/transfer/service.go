package transfer

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc"
	tokenconfig "github.com/chainsafe/canton-middleware/pkg/token"
	"github.com/chainsafe/canton-middleware/pkg/user"
)

//go:generate mockery --name UserStore --output mocks --outpkg mocks --filename mock_user_store.go --with-expecter
//go:generate mockery --name TransferCache --output mocks --outpkg mocks --filename mock_transfer_cache.go --with-expecter
//go:generate mockery --srcpkg github.com/chainsafe/canton-middleware/pkg/cantonsdk/token --name Token --output mocks --outpkg mocks --filename mock_canton_token.go --with-expecter

// UserStore is the narrow interface for looking up users.
type UserStore interface {
	GetUserByEVMAddress(ctx context.Context, evmAddress string) (*user.User, error)
}

// TransferCache is the interface for caching prepared transfers.
type TransferCache interface {
	Put(transfer *token.PreparedTransfer) error
	GetAndDelete(transferID string) (*token.PreparedTransfer, error)
}

// EvmLogStore is the narrow store interface needed to write synthetic EVM Transfer
// logs for non-custodial transfers so they appear in eth_getLogs / the Activity page.
type EvmLogStore interface {
	NewBlock(ctx context.Context, chainID uint64) (ethrpc.PendingBlock, error)
	GetEvmTransactionCount(ctx context.Context, fromAddress string) (uint64, error)
}

// Service is the interface for the non-custodial prepare/execute transfer flow.
type Service interface {
	Prepare(ctx context.Context, senderEVMAddr string, req *PrepareRequest) (*PrepareResponse, error)
	Execute(ctx context.Context, senderEVMAddr string, req *ExecuteRequest) (*ExecuteResponse, error)
}

// transferEventTopic is the keccak256 hash of Transfer(address,address,uint256).
var transferEventTopic = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))

const evmWordSize = 32

// evmTransferMeta holds the EVM-layer data captured at Prepare time so Execute
// can write a synthetic Transfer log without re-deriving it from the Canton types.
type evmTransferMeta struct {
	senderEVMAddr    string
	recipientEVMAddr string
	contractAddress  string
	amountData       []byte // big.Int.Bytes() of transfer amount in token units
	callData         []byte // ABI-encoded transfer(address,uint256) calldata
}

type tokenEntry struct {
	address  common.Address
	decimals int
}

// TransferService implements the non-custodial prepare/execute transfer flow.
type TransferService struct {
	cantonToken         token.Token
	userStore           UserStore
	cache               TransferCache
	allowedTokenSymbols map[string]bool
	tokensBySymbol      map[string]tokenEntry // symbol (upper) → contract address + decimals
	evmMeta             sync.Map              // transferID → *evmTransferMeta; cleared on Execute
	evmStore            EvmLogStore           // nil when EthRPC is disabled
	chainID             uint64
	gasLimit            uint64
	logger              *zap.Logger
}

// NewTransferService creates a new TransferService.
// allowedSymbols defines the set of valid token symbols (e.g. "DEMO", "PROMPT").
// evmStore may be nil; when non-nil a synthetic EVM Transfer log is written after every
// successful Execute so the Activity page reflects non-custodial transfers.
func NewTransferService(
	cantonToken token.Token,
	userStore UserStore,
	cache TransferCache,
	allowedSymbols []string,
	tokenCfg *tokenconfig.Config,
	evmStore EvmLogStore,
	chainID uint64,
	gasLimit uint64,
	logger *zap.Logger,
) *TransferService {
	allowed := make(map[string]bool, len(allowedSymbols))
	for _, s := range allowedSymbols {
		allowed[s] = true
	}
	bySymbol := make(map[string]tokenEntry, len(tokenCfg.SupportedTokens))
	for addr, tok := range tokenCfg.SupportedTokens {
		bySymbol[strings.ToUpper(tok.Symbol)] = tokenEntry{address: addr, decimals: tok.Decimals}
	}
	return &TransferService{
		cantonToken:         cantonToken,
		userStore:           userStore,
		cache:               cache,
		allowedTokenSymbols: allowed,
		tokensBySymbol:      bySymbol,
		evmStore:            evmStore,
		chainID:             chainID,
		gasLimit:            gasLimit,
		logger:              logger,
	}
}

// Prepare builds a Canton transaction and returns the hash for external signing.
func (s *TransferService) Prepare(ctx context.Context, senderEVMAddr string, req *PrepareRequest) (*PrepareResponse, error) {
	if !s.allowedTokenSymbols[req.Token] {
		return nil, apperrors.BadRequestError(nil, "unsupported token")
	}

	sender, err := s.userStore.GetUserByEVMAddress(ctx, senderEVMAddr)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, apperrors.UnAuthorizedError(err, "user not found")
		}
		return nil, fmt.Errorf("lookup sender: %w", err)
	}
	if sender.KeyMode != user.KeyModeExternal {
		return nil, apperrors.BadRequestError(nil, "prepare/execute API requires key_mode=external")
	}

	recipient, err := s.userStore.GetUserByEVMAddress(ctx, req.To)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, apperrors.BadRequestError(err, "recipient not found")
		}
		return nil, fmt.Errorf("lookup recipient: %w", err)
	}

	pt, err := s.cantonToken.PrepareTransfer(ctx, &token.PrepareTransferRequest{
		FromPartyID: sender.CantonPartyID,
		ToPartyID:   recipient.CantonPartyID,
		Amount:      req.Amount,
		TokenSymbol: req.Token,
	})
	if err != nil {
		if errors.Is(err, token.ErrInsufficientBalance) {
			return nil, apperrors.BadRequestError(err, "insufficient balance")
		}
		return nil, fmt.Errorf("prepare transfer: %w", err)
	}

	if err := s.cache.Put(pt); err != nil {
		return nil, apperrors.GeneralError(fmt.Errorf("too many pending transfers: %w", err))
	}

	// Store EVM metadata for use in Execute → writeEvmTransferLog.
	if s.evmStore != nil {
		if meta, metaErr := s.buildEvmMeta(senderEVMAddr, req); metaErr == nil {
			s.evmMeta.Store(pt.TransferID, meta)
		}
	}

	return &PrepareResponse{
		TransferID:      pt.TransferID,
		TransactionHash: "0x" + hex.EncodeToString(pt.TransactionHash),
		PartyID:         pt.PartyID,
		ExpiresAt:       pt.ExpiresAt.Format(time.RFC3339),
	}, nil
}

// Execute completes a previously prepared transfer using the client's DER signature.
func (s *TransferService) Execute(ctx context.Context, senderEVMAddr string, req *ExecuteRequest) (*ExecuteResponse, error) {
	sender, err := s.userStore.GetUserByEVMAddress(ctx, senderEVMAddr)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, apperrors.UnAuthorizedError(err, "user not found")
		}
		return nil, fmt.Errorf("lookup sender: %w", err)
	}
	if sender.CantonPublicKeyFingerprint != req.SignedBy {
		return nil, apperrors.ForbiddenError(nil, "signature fingerprint does not match registered key")
	}

	pt, err := s.cache.GetAndDelete(req.TransferID)
	if err != nil {
		if errors.Is(err, ErrTransferNotFound) {
			return nil, apperrors.ResourceNotFoundError(err, "transfer not found")
		}
		if errors.Is(err, ErrTransferExpired) {
			return nil, apperrors.GoneError(err, "transfer expired")
		}
		return nil, fmt.Errorf("retrieve prepared transfer: %w", err)
	}

	sigBytes, err := hex.DecodeString(strings.TrimPrefix(req.Signature, "0x"))
	if err != nil {
		return nil, apperrors.BadRequestError(err, "invalid DER signature")
	}

	err = s.cantonToken.ExecuteTransfer(ctx, &token.ExecuteTransferRequest{
		PreparedTransfer: pt,
		Signature:        sigBytes,
		SignedBy:         req.SignedBy,
	})
	if err != nil {
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.InvalidArgument, codes.PermissionDenied:
				return nil, apperrors.ForbiddenError(err, "signature verification failed")
			}
		}
		return nil, fmt.Errorf("execute transfer: %w", err)
	}

	// Canton transfer succeeded. Write a synthetic EVM log so eth_getLogs /
	// the Activity page reflects this non-custodial transfer.
	if s.evmStore != nil {
		meta, loaded := s.evmMeta.LoadAndDelete(req.TransferID)
		if loaded {
			if logErr := s.writeEvmTransferLog(ctx, pt.TransactionHash, meta.(*evmTransferMeta)); logErr != nil {
				s.logger.Warn("failed to write synthetic EVM log for non-custodial transfer",
					zap.String("transfer_id", req.TransferID),
					zap.Error(logErr),
				)
			}
		}
	}

	return &ExecuteResponse{Status: "completed"}, nil
}

// buildEvmMeta constructs the EVM metadata for a transfer at Prepare time.
func (s *TransferService) buildEvmMeta(senderEVMAddr string, req *PrepareRequest) (*evmTransferMeta, error) {
	entry, ok := s.tokensBySymbol[strings.ToUpper(req.Token)]
	if !ok {
		return nil, fmt.Errorf("token not found: %s", req.Token)
	}
	amount, err := parseTokenAmount(req.Amount, entry.decimals)
	if err != nil {
		return nil, err
	}
	recipientAddr := common.HexToAddress(req.To)
	return &evmTransferMeta{
		senderEVMAddr:    strings.ToLower(senderEVMAddr),
		recipientEVMAddr: strings.ToLower(req.To),
		contractAddress:  entry.address.Hex(),
		amountData:       amount.Bytes(),
		callData:         encodeERC20Transfer(recipientAddr, amount),
	}, nil
}

// writeEvmTransferLog persists a synthetic ERC-20 Transfer log and EVM transaction
// for a completed non-custodial transfer. Errors are non-fatal; the Canton transfer
// already succeeded.
func (s *TransferService) writeEvmTransferLog(ctx context.Context, txHash []byte, meta *evmTransferMeta) error {
	nonce, err := s.evmStore.GetEvmTransactionCount(ctx, meta.senderEVMAddr)
	if err != nil {
		return fmt.Errorf("get nonce: %w", err)
	}

	block, err := s.evmStore.NewBlock(ctx, s.chainID)
	if err != nil {
		return fmt.Errorf("open block: %w", err)
	}
	defer block.Abort(ctx) //nolint:errcheck // safe: Abort is no-op after Finalize

	contractAddr := common.HexToAddress(meta.contractAddress)
	fromAddr := common.HexToAddress(meta.senderEVMAddr)
	toAddr := common.HexToAddress(meta.recipientEVMAddr)
	fromTopic := common.BytesToHash(common.LeftPadBytes(fromAddr.Bytes(), evmWordSize))
	toTopic := common.BytesToHash(common.LeftPadBytes(toAddr.Bytes(), evmWordSize))
	amountData := common.LeftPadBytes(meta.amountData, evmWordSize)
	blockTimestamp := uint64(time.Now().Unix()) //nolint:gosec

	evmTx := &ethrpc.EvmTransaction{
		TxHash:      txHash,
		FromAddress: meta.senderEVMAddr,
		ToAddress:   meta.contractAddress,
		Nonce:       nonce,
		Input:       meta.callData,
		ValueWei:    "0",
		Status:      1,
		BlockNumber: block.Number(),
		BlockHash:   block.Hash(),
		TxIndex:     0,
		GasUsed:     s.gasLimit,
	}
	if err = block.AddEvmTransaction(ctx, evmTx); err != nil {
		return fmt.Errorf("add evm transaction: %w", err)
	}

	evmLog := &ethrpc.EvmLog{
		TxHash:         txHash,
		LogIndex:       0,
		Address:        contractAddr.Bytes(),
		Topics:         [][]byte{transferEventTopic.Bytes(), fromTopic.Bytes(), toTopic.Bytes()},
		Data:           amountData,
		BlockNumber:    block.Number(),
		BlockHash:      block.Hash(),
		TxIndex:        0,
		Removed:        false,
		BlockTimestamp: blockTimestamp,
	}
	if err = block.AddEvmLog(ctx, evmLog); err != nil {
		return fmt.Errorf("add evm log: %w", err)
	}

	return block.Finalize(ctx)
}

// parseTokenAmount converts a decimal string (e.g. "100.5") to a raw integer
// scaled by 10^decimals.
func parseTokenAmount(amountStr string, decimals int) (*big.Int, error) {
	parts := strings.SplitN(strings.TrimSpace(amountStr), ".", 2)
	whole := strings.ReplaceAll(parts[0], ",", "")
	frac := ""
	if len(parts) == 2 {
		frac = parts[1]
	}
	if len(frac) > decimals {
		return nil, fmt.Errorf("too many decimal places in %q", amountStr)
	}
	frac += strings.Repeat("0", decimals-len(frac))
	result := new(big.Int)
	if _, ok := result.SetString(whole+frac, 10); !ok {
		return nil, fmt.Errorf("invalid amount %q", amountStr)
	}
	return result, nil
}

// encodeERC20Transfer builds ABI calldata for transfer(address,uint256).
// Selector 0xa9059cbb = keccak256("transfer(address,uint256)")[:4].
func encodeERC20Transfer(to common.Address, amount *big.Int) []byte {
	selector := []byte{0xa9, 0x05, 0x9c, 0xbb}
	toBytes := common.LeftPadBytes(to.Bytes(), evmWordSize)
	amountBytes := common.LeftPadBytes(amount.Bytes(), evmWordSize)
	calldata := make([]byte, 0, 4+evmWordSize+evmWordSize)
	calldata = append(calldata, selector...)
	calldata = append(calldata, toBytes...)
	calldata = append(calldata, amountBytes...)
	return calldata
}
