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
	metrics    *Metrics
	logger     *zap.Logger

	bridgeAddress common.Address
	bridge        *contracts.CantonBridge

	// Track how far the poller has scanned (for readiness checks)
	mu               sync.RWMutex
	lastScannedBlock uint64
}

// observeRPC records the latency and terminal status of one outbound Ethereum
// RPC call. Use it inline at the call site rather than as a wrapper so the
// caller still owns ctx, result extraction, and error wrapping:
//
//	start := time.Now()
//	header, err := c.client.HeaderByNumber(ctx, nil)
//	c.observeRPC("get_latest_block", start, err)
//	if err != nil { ... }
//
// The method label is a stable lowercase identifier — see the call sites
// throughout this file for the enumerated set.
func (c *Client) observeRPC(method string, start time.Time, err error) {
	status := "ok"
	if err != nil {
		status = "error"
	}
	c.metrics.RPCDuration.WithLabelValues(method).Observe(time.Since(start).Seconds())
	c.metrics.RPCCallsTotal.WithLabelValues(method, status).Inc()
}

// NewClient creates a new Ethereum client.
//
// metrics receives Prometheus observations for every outbound RPC call and
// for the deposit-event poll loop. Pass NewNopMetrics() in tests where metric
// values aren't asserted.
func NewClient(cfg *Config, metrics *Metrics, logger *zap.Logger) (*Client, error) {
	if metrics == nil {
		metrics = NewNopMetrics()
	}
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
		metrics:       metrics,
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
		c.metrics.LastScannedBlock.Set(float64(b))
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
	nonceStart := time.Now()
	nonce, err := c.client.PendingNonceAt(ctx, c.address)
	c.observeRPC("pending_nonce_at", nonceStart, err)
	if err != nil {
		return nil, fmt.Errorf("failed to get nonce: %w", err)
	}

	auth.Nonce = big.NewInt(int64(nonce))
	auth.GasLimit = c.config.GasLimit

	// Set gas price if configured
	if c.config.MaxGasPrice != "" {
		maxGasPrice := new(big.Int)
		maxGasPrice.SetString(c.config.MaxGasPrice, 10)

		gasStart := time.Now()
		gasPrice, err := c.client.SuggestGasPrice(ctx)
		c.observeRPC("suggest_gas_price", gasStart, err)
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
	start := time.Now()
	header, err := c.client.HeaderByNumber(ctx, nil)
	c.observeRPC("get_latest_block", start, err)
	if err != nil {
		return 0, fmt.Errorf("failed to get latest block: %w", err)
	}
	n := header.Number.Uint64()
	c.metrics.LatestBlockSeen.Set(float64(n))
	return n, nil
}

// blockRange is an inclusive [start, end] span of block numbers.
type blockRange struct {
	start uint64
	end   uint64
}

// chunkRange splits the inclusive range [start, end] into consecutive slices of
// at most maxRange blocks each. It returns nil for an empty range (start > end)
// or a zero maxRange. The subtraction-based bound (end-s >= maxRange) avoids the
// uint64 overflow that s+maxRange-1 would hit for very large maxRange values.
func chunkRange(start, end, maxRange uint64) []blockRange {
	if start > end || maxRange == 0 {
		return nil
	}
	chunks := make([]blockRange, 0, (end-start)/maxRange+1)
	for s := start; s <= end; {
		e := end
		if end-s >= maxRange {
			e = s + maxRange - 1
		}
		chunks = append(chunks, blockRange{start: s, end: e})
		if e == ^uint64(0) { // next start (e+1) would wrap to 0
			break
		}
		s = e + 1
	}
	return chunks
}

// WatchDepositEvents polls for deposit events (uses polling for HTTP RPC compatibility).
//
// Each tick's [currentBlock+1, latestBlock] range is walked in slices of at most
// config.MaxBlockRange blocks so requests stay under the provider's per-call cap.
// On a slice failure, currentBlock advances only through the last successful
// slice and the failing range is retried on the next tick.
// After each fully scanned slice — including slices with no deposits — the handler
// is also invoked with a checkpoint event (DepositEvent{Checkpoint: true}) carrying
// the last scanned block, so the caller can persist scan progress and avoid
// re-scanning from the start on restart. The checkpoint rides the same in-order
// handler path as deposits, so it is only delivered after that slice's deposits.
func (c *Client) WatchDepositEvents(
	ctx context.Context,
	fromBlock uint64,
	handler func(*DepositEvent) error,
) error {
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
			// Wrap the tick body in a closure so a single deferred observation
			// records the cycle duration regardless of which branch the tick
			// takes. `continue` inside the for-loop is replaced by `return` to
			// exit the closure; the outer for-select then advances to the next
			// tick.
			func() {
				pollStart := time.Now()
				defer func() {
					c.metrics.EventPollDuration.Observe(time.Since(pollStart).Seconds())
				}()

				// Get latest block
				latestBlock, err := c.GetLatestBlockNumber(ctx)
				if err != nil {
					c.metrics.EventPollFailuresTotal.WithLabelValues("get_latest_block").Inc()
					c.logger.Warn("Failed to get latest block", zap.Error(err))
					return
				}

				if latestBlock <= currentBlock {
					// Still record that we've checked up to this point
					c.setLastScannedBlock(latestBlock)
					return
				}

				// Walk [currentBlock+1, latestBlock] in slices of at most
				// MaxBlockRange blocks so each eth_getLogs request stays under
				// the provider's per-call range cap. Progress advances only
				// through the last successful slice; a failing slice (and
				// everything after it) is retried on the next tick.
				for _, r := range chunkRange(currentBlock+1, latestBlock, c.config.MaxBlockRange) {
					if err := c.scanDepositRange(ctx, r, handler); err != nil {
						return
					}
					// Emit a scan-progress checkpoint after this slice's deposits were
					// handed to the handler (in order), so a restart resumes near here
					// instead of re-scanning from the start. Advance currentBlock only
					// once the checkpoint is accepted, so on failure the same range is
					// re-scanned and re-checkpointed on the next tick rather than having
					// its watermark silently skipped.
					if err := handler(&DepositEvent{BlockNumber: r.end, Checkpoint: true}); err != nil {
						return
					}
					currentBlock = r.end
					c.setLastScannedBlock(currentBlock)
				}
			}()
		}
	}
}

// scanDepositRange filters DepositToCanton events for a single inclusive block
// range and dispatches each to handler. A filter, iterator, or handler error is
// returned so the caller can stop advancing scan progress and retry the range
// on the next tick.
func (c *Client) scanDepositRange(ctx context.Context, r blockRange, handler func(*DepositEvent) error) error {
	opts := &bind.FilterOpts{
		Start:   r.start,
		End:     &r.end,
		Context: ctx,
	}

	filterStart := time.Now()
	iter, err := c.bridge.FilterDepositToCanton(opts, nil, nil, nil)
	c.observeRPC("filter_deposit_events", filterStart, err)
	if err != nil {
		c.metrics.EventPollFailuresTotal.WithLabelValues("filter_events").Inc()
		c.logger.Warn("Failed to filter deposit events",
			zap.Uint64("from_block", r.start),
			zap.Uint64("to_block", r.end),
			zap.Error(err))
		return err
	}
	defer iter.Close()

	for iter.Next() {
		c.metrics.EventsFetchedTotal.Inc()
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
			// The handler only fails on context cancellation (shutdown). Abort
			// the slice without advancing scan progress so the unhandled events
			// are re-scanned on the next tick or restart; downstream inserts are
			// idempotent (ON CONFLICT DO NOTHING), so re-delivery is safe.
			return fmt.Errorf("handle deposit event %s: %w", event.Raw.TxHash.Hex(), err)
		}
	}

	if err := iter.Error(); err != nil {
		c.metrics.EventPollFailuresTotal.WithLabelValues("iterator").Inc()
		c.logger.Warn("Iterator error", zap.Error(err))
		return err
	}
	return nil
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

	start := time.Now()
	tx, err := c.bridge.WithdrawFromCanton(auth, token, recipient, amount, nonce, cantonTxHash)
	c.observeRPC("withdraw_from_canton", start, err)
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
	start := time.Now()
	processed, err := c.bridge.ProcessedCantonTxs(&bind.CallOpts{Context: ctx}, cantonTxHash)
	c.observeRPC("is_withdrawal_processed", start, err)
	return processed, err
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

	start := time.Now()
	tx, err := c.bridge.DepositToCanton(auth, token, amount, cantonRecipient)
	c.observeRPC("deposit_to_canton", start, err)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to submit deposit transaction: %w", err)
	}

	c.logger.Info("Deposit transaction submitted",
		zap.String("tx_hash", tx.Hash().Hex()),
		zap.String("token", token.Hex()),
		zap.String("amount", amount.String()))

	return tx.Hash(), nil
}
