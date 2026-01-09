package ethereum

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/ethereum/contracts"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"go.uber.org/zap"
)

// Client represents an Ethereum client
type Client struct {
	config     *config.EthereumConfig
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
func NewClient(cfg *config.EthereumConfig, logger *zap.Logger) (*Client, error) {
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

// GetTransactor returns a transaction signer with EIP-1559 gas pricing
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

	// EIP-1559: Get base fee from latest block header
	header, err := c.client.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get block header: %w", err)
	}

	// Get suggested priority fee (tip)
	tip, err := c.client.SuggestGasTipCap(ctx)
	if err != nil {
		// Fallback: 2 gwei tip
		tip = big.NewInt(2_000_000_000)
		c.logger.Warn("Failed to get suggested tip, using fallback",
			zap.String("tip_gwei", "2"),
			zap.Error(err))
	}

	// Minimum tip: 2 gwei to ensure transactions are picked up
	minTip := big.NewInt(2_000_000_000)
	if tip.Cmp(minTip) < 0 {
		tip = minTip
	}

	// maxFeePerGas = 2 * baseFee + tip (allows for 1 block of fee increase)
	baseFee := header.BaseFee
	maxFee := new(big.Int).Mul(baseFee, big.NewInt(2))
	maxFee.Add(maxFee, tip)

	// Cap at configured maximum
	if c.config.MaxGasPrice != "" {
		maxAllowed := new(big.Int)
		maxAllowed.SetString(c.config.MaxGasPrice, 10)
		if maxFee.Cmp(maxAllowed) > 0 {
			c.logger.Warn("Calculated maxFee exceeds limit, capping",
				zap.String("calculated", maxFee.String()),
				zap.String("max_allowed", maxAllowed.String()))
			maxFee = maxAllowed
		}
	}

	auth.GasFeeCap = maxFee // maxFeePerGas
	auth.GasTipCap = tip    // maxPriorityFeePerGas

	c.logger.Debug("EIP-1559 gas parameters",
		zap.String("base_fee_gwei", new(big.Int).Div(baseFee, big.NewInt(1_000_000_000)).String()),
		zap.String("tip_gwei", new(big.Int).Div(tip, big.NewInt(1_000_000_000)).String()),
		zap.String("max_fee_gwei", new(big.Int).Div(maxFee, big.NewInt(1_000_000_000)).String()))

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

// WatchDepositEvents polls for deposit events (uses polling for HTTP RPC compatibility)
func (c *Client) WatchDepositEvents(ctx context.Context, fromBlock uint64, handler func(*DepositEvent) error) error {
	c.logger.Info("Starting deposit event poller", zap.Uint64("from_block", fromBlock))

	currentBlock := fromBlock
	c.setLastScannedBlock(currentBlock)

	ticker := time.NewTicker(c.config.PollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Get latest block
			latestBlock, err := c.GetLatestBlockNumber(ctx)
			if err != nil {
				c.logger.Warn("Failed to get latest block", zap.Error(err))
				continue
			}

			if latestBlock <= currentBlock {
				// Still record that we've checked up to this point
				c.setLastScannedBlock(latestBlock)
				continue
			}

			// Query for events from currentBlock+1 to latestBlock
			opts := &bind.FilterOpts{
				Start:   currentBlock + 1,
				End:     &latestBlock,
				Context: ctx,
			}

			iter, err := c.bridge.FilterDepositToCanton(opts, nil, nil, nil)
			if err != nil {
				c.logger.Warn("Failed to filter deposit events", zap.Error(err))
				continue
			}

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
				c.logger.Warn("Iterator error", zap.Error(err))
			}
			iter.Close()

			// Update scan progress even if there were no events
			currentBlock = latestBlock
			c.setLastScannedBlock(currentBlock)
		}
	}
}

// WithdrawFromCanton submits a withdrawal transaction with signature proof
func (c *Client) WithdrawFromCanton(
	ctx context.Context,
	token common.Address,
	recipient common.Address,
	amount *big.Int,
	withdrawalId [32]byte,
) (common.Hash, error) {
	c.logger.Info("Submitting withdrawal from Canton",
		zap.String("token", token.Hex()),
		zap.String("recipient", recipient.Hex()),
		zap.String("amount", amount.String()),
		zap.String("withdrawal_id", common.Bytes2Hex(withdrawalId[:])))

	// Generate the signature proof
	// The contract verifies: keccak256(abi.encodePacked(token, amount, recipient, withdrawalId, block.chainid, address(this)))
	chainID := big.NewInt(c.config.ChainID)

	// Pack the message: token (20) + amount (32) + recipient (20) + withdrawalId (32) + chainId (32) + bridgeAddr (20)
	messageData := make([]byte, 0, 156)
	messageData = append(messageData, token.Bytes()...)
	messageData = append(messageData, common.LeftPadBytes(amount.Bytes(), 32)...)
	messageData = append(messageData, recipient.Bytes()...)
	messageData = append(messageData, withdrawalId[:]...)
	messageData = append(messageData, common.LeftPadBytes(chainID.Bytes(), 32)...)
	messageData = append(messageData, c.bridgeAddress.Bytes()...)

	// Hash the message
	messageHash := crypto.Keccak256Hash(messageData)

	// Create Ethereum signed message hash: keccak256("\x19Ethereum Signed Message:\n32" + messageHash)
	ethSignedHash := crypto.Keccak256Hash(
		[]byte(fmt.Sprintf("\x19Ethereum Signed Message:\n32")),
		messageHash.Bytes(),
	)

	// Sign the hash
	signature, err := crypto.Sign(ethSignedHash.Bytes(), c.privateKey)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to sign withdrawal proof: %w", err)
	}

	// Ethereum signature format: adjust v value from 0/1 to 27/28
	if signature[64] < 27 {
		signature[64] += 27
	}

	c.logger.Debug("Generated withdrawal proof",
		zap.String("message_hash", messageHash.Hex()),
		zap.String("eth_signed_hash", ethSignedHash.Hex()),
		zap.Int("signature_len", len(signature)))

	auth, err := c.GetTransactor(ctx)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to create transactor: %w", err)
	}

	// Call contract with new signature: (token, amount, recipient, withdrawalId, proof)
	tx, err := c.bridge.WithdrawFromCanton(auth, token, amount, recipient, withdrawalId, signature)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to submit withdrawal transaction: %w", err)
	}

	c.logger.Info("Withdrawal transaction submitted, waiting for confirmation",
		zap.String("tx_hash", tx.Hash().Hex()),
		zap.String("withdrawal_id", common.Bytes2Hex(withdrawalId[:])))

	// Wait for the transaction receipt and verify success
	receipt, err := bind.WaitMined(ctx, c.client, tx)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to wait for withdrawal tx receipt: %w", err)
	}

	// Check if transaction reverted (status 0 = failed, status 1 = success)
	if receipt.Status == 0 {
		c.logger.Error("Withdrawal transaction reverted",
			zap.String("tx_hash", tx.Hash().Hex()),
			zap.Uint64("gas_used", receipt.GasUsed))
		return common.Hash{}, fmt.Errorf("withdrawal transaction reverted (status=0), tx_hash=%s", tx.Hash().Hex())
	}

	c.logger.Info("Withdrawal transaction confirmed",
		zap.String("tx_hash", tx.Hash().Hex()),
		zap.Uint64("block_number", receipt.BlockNumber.Uint64()),
		zap.Uint64("gas_used", receipt.GasUsed))

	return tx.Hash(), nil
}

// IsWithdrawalProcessed checks if a Canton withdrawal has already been processed on EVM
func (c *Client) IsWithdrawalProcessed(ctx context.Context, withdrawalId [32]byte) (bool, error) {
	// Try the new contract function first
	processed, err := c.bridge.IsWithdrawalProcessed(&bind.CallOpts{Context: ctx}, withdrawalId)
	if err == nil {
		return processed, nil
	}
	// Fall back to old function for backwards compatibility
	return c.bridge.ProcessedCantonTxs(&bind.CallOpts{Context: ctx}, withdrawalId)
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
