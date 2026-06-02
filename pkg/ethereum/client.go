// SPDX-License-Identifier: Apache-2.0

package ethereum

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/ethereum/contracts"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"go.uber.org/zap"
)

// Client represents an Ethereum client
type Client struct {
	config     *Config
	client     *ethclient.Client
	wsClient   *ethclient.Client
	privateKey *ecdsa.PrivateKey
	address    common.Address
	logger     *zap.Logger

	bridgeAddress common.Address
	bridge        *contracts.CantonBridge

	// Track how far the poller has scanned (for readiness checks)
	mu               sync.RWMutex
	lastScannedBlock uint64
}

// NewClient creates a new Ethereum client
func NewClient(cfg *Config, logger *zap.Logger) (*Client, error) {
	// Connect to Ethereum RPC
	client, err := ethclient.Dial(cfg.RPCURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Ethereum RPC: %w", err)
	}

	// Connect to WebSocket for event streaming (optional)
	var wsClient *ethclient.Client
	if cfg.WSUrl != "" {
		wsClient, err = ethclient.Dial(cfg.WSUrl)
		if err != nil {
			logger.Warn("Failed to connect to Ethereum WebSocket, falling back to polling",
				zap.Error(err))
		}
	}

	// Load private key
	privateKey, err := crypto.HexToECDSA(cfg.RelayerPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load private key: %w", err)
	}

	address := crypto.PubkeyToAddress(privateKey.PublicKey)
	bridgeAddress := common.HexToAddress(cfg.BridgeContract)

	// Load bridge contract
	bridge, err := contracts.NewCantonBridge(bridgeAddress, client)
	if err != nil {
		return nil, fmt.Errorf("failed to load bridge contract: %w", err)
	}

	logger.Info("Connected to Ethereum",
		zap.Int64("chain_id", cfg.ChainID),
		zap.String("rpc_url", cfg.RPCURL),
		zap.String("bridge_contract", bridgeAddress.Hex()),
		zap.String("relayer_address", address.Hex()))

	return &Client{
		config:        cfg,
		client:        client,
		wsClient:      wsClient,
		privateKey:    privateKey,
		address:       address,
		bridgeAddress: bridgeAddress,
		bridge:        bridge,
		logger:        logger,
	}, nil
}

// Close closes the Ethereum clients
func (c *Client) Close() {
	if c.client != nil {
		c.client.Close()
	}
	if c.wsClient != nil {
		c.wsClient.Close()
	}
}

// GetLastScannedBlock returns the highest block number the poller has scanned.
func (c *Client) GetLastScannedBlock() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastScannedBlock
}

func (c *Client) setLastScannedBlock(b uint64) {
	c.mu.Lock()
	if b > c.lastScannedBlock {
		c.lastScannedBlock = b
	}
	c.mu.Unlock()
}

// GetTransactor returns a transaction signer
func (c *Client) GetTransactor(ctx context.Context) (*bind.TransactOpts, error) {
	chainID := big.NewInt(c.config.ChainID)

	auth, err := bind.NewKeyedTransactorWithChainID(c.privateKey, chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to create transactor: %w", err)
	}

	// Get nonce
	nonce, err := c.client.PendingNonceAt(ctx, c.address)
	if err != nil {
		return nil, fmt.Errorf("failed to get nonce: %w", err)
	}

	auth.Nonce = big.NewInt(int64(nonce))
	auth.GasLimit = c.config.GasLimit

	// Set gas price if configured
	if c.config.MaxGasPrice != "" {
		maxGasPrice := new(big.Int)
		maxGasPrice.SetString(c.config.MaxGasPrice, 10)

		gasPrice, err := c.client.SuggestGasPrice(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to suggest gas price: %w", err)
		}

		if gasPrice.Cmp(maxGasPrice) > 0 {
			c.logger.Warn("Suggested gas price exceeds maximum",
				zap.String("suggested", gasPrice.String()),
				zap.String("max", maxGasPrice.String()))
			auth.GasPrice = maxGasPrice
		} else {
			auth.GasPrice = gasPrice
		}
	}

	return auth, nil
}

// GetLatestBlockNumber gets the latest block number
func (c *Client) GetLatestBlockNumber(ctx context.Context) (uint64, error) {
	header, err := c.client.HeaderByNumber(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to get latest block: %w", err)
	}
	return header.Number.Uint64(), nil
}

// WatchDepositEvents polls for deposit events (uses polling for HTTP RPC compatibility).
//
// Each tick is split into chunks of at most config.MaxBlockRange blocks so that
// requests stay under the provider's per-call block-range cap. If a chunk fails,
// the poller advances currentBlock only up to the last successful chunk and
// retries the failing range on the next tick — preventing an oversized range
// from permanently stalling progress.
func (c *Client) WatchDepositEvents(ctx context.Context, fromBlock uint64, handler func(*DepositEvent) error) error {
	c.logger.Info("Starting deposit event poller",
		zap.Uint64("from_block", fromBlock),
		zap.Uint64("max_block_range", c.config.MaxBlockRange))

	currentBlock := fromBlock
	c.setLastScannedBlock(currentBlock)

	ticker := time.NewTicker(c.config.PollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			latestBlock, err := c.GetLatestBlockNumber(ctx)
			if err != nil {
				c.logger.Warn("Failed to get latest block", zap.Error(err))
				continue
			}

			if latestBlock <= currentBlock {
				c.setLastScannedBlock(latestBlock)
				continue
			}

			for _, chunk := range chunkRange(currentBlock+1, latestBlock, c.config.MaxBlockRange) {
				if err := c.scanDepositChunk(ctx, chunk.start, chunk.end, handler); err != nil {
					c.logger.Warn("Failed to scan deposit chunk; will retry next tick",
						zap.Error(err),
						zap.Uint64("from", chunk.start),
						zap.Uint64("to", chunk.end))
					break
				}
				currentBlock = chunk.end
				c.setLastScannedBlock(currentBlock)
			}
		}
	}
}

// scanDepositChunk filters DepositToCanton events for an inclusive [from,to] range.
func (c *Client) scanDepositChunk(ctx context.Context, from, to uint64, handler func(*DepositEvent) error) error {
	opts := &bind.FilterOpts{
		Start:   from,
		End:     &to,
		Context: ctx,
	}

	iter, err := c.bridge.FilterDepositToCanton(opts, nil, nil, nil)
	if err != nil {
		return fmt.Errorf("filter deposit events [%d,%d]: %w", from, to, err)
	}
	defer iter.Close()

	for iter.Next() {
		event := iter.Event
		depositEvent := &DepositEvent{
			Token:           event.Token,
			Sender:          event.Sender,
			CantonRecipient: event.CantonRecipient,
			Amount:          event.Amount,
			Nonce:           event.Nonce,
			BlockNumber:     event.Raw.BlockNumber,
			TxHash:          event.Raw.TxHash,
			LogIndex:        event.Raw.Index,
		}

		if err := handler(depositEvent); err != nil {
			c.logger.Error("Failed to handle deposit event",
				zap.Error(err),
				zap.String("tx_hash", event.Raw.TxHash.Hex()))
		}
	}

	if err := iter.Error(); err != nil {
		return fmt.Errorf("deposit iterator [%d,%d]: %w", from, to, err)
	}
	return nil
}

type blockRange struct{ start, end uint64 }

// chunkRange splits [start,end] inclusive into contiguous chunks of at most maxRange blocks each.
// Returns nil if start > end or maxRange == 0.
func chunkRange(start, end, maxRange uint64) []blockRange {
	if start > end || maxRange == 0 {
		return nil
	}
	chunks := make([]blockRange, 0, (end-start)/maxRange+1)
	for s := start; s <= end; {
		e := min(s+maxRange-1, end)
		chunks = append(chunks, blockRange{start: s, end: e})
		if e == ^uint64(0) {
			break
		}
		s = e + 1
	}
	return chunks
}

// WithdrawFromCanton submits a withdrawal transaction
func (c *Client) WithdrawFromCanton(
	ctx context.Context,
	token common.Address,
	recipient common.Address,
	amount *big.Int,
	nonce *big.Int,
	cantonTxHash [32]byte,
) (common.Hash, error) {
	c.logger.Info("Submitting withdrawal from Canton",
		zap.String("token", token.Hex()),
		zap.String("recipient", recipient.Hex()),
		zap.String("amount", amount.String()),
		zap.Uint64("nonce", nonce.Uint64()))

	auth, err := c.GetTransactor(ctx)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to create transactor: %w", err)
	}

	tx, err := c.bridge.WithdrawFromCanton(auth, token, recipient, amount, nonce, cantonTxHash)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to submit withdrawal transaction: %w", err)
	}

	c.logger.Info("Withdrawal transaction submitted",
		zap.String("tx_hash", tx.Hash().Hex()),
		zap.Uint64("nonce", nonce.Uint64()))

	return tx.Hash(), nil
}

// IsWithdrawalProcessed checks if a Canton withdrawal has already been processed on EVM
func (c *Client) IsWithdrawalProcessed(ctx context.Context, cantonTxHash [32]byte) (bool, error) {
	return c.bridge.ProcessedCantonTxs(&bind.CallOpts{Context: ctx}, cantonTxHash)
}

// DepositToCanton submits a deposit transaction (for testing)
func (c *Client) DepositToCanton(
	ctx context.Context,
	token common.Address,
	amount *big.Int,
	cantonRecipient [32]byte,
) (common.Hash, error) {
	auth, err := c.GetTransactor(ctx)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to create transactor: %w", err)
	}

	tx, err := c.bridge.DepositToCanton(auth, token, amount, cantonRecipient)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to submit deposit transaction: %w", err)
	}

	c.logger.Info("Deposit transaction submitted",
		zap.String("tx_hash", tx.Hash().Hex()),
		zap.String("token", token.Hex()),
		zap.String("amount", amount.String()))

	return tx.Hash(), nil
}
