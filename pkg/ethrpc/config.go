// SPDX-License-Identifier: Apache-2.0

package ethrpc

import (
	"time"
)

// Config contains Ethereum JSON-RPC facade settings for MetaMask compatibility
type Config struct {
	Enabled             bool          `yaml:"enabled" default:"false"`
	ChainID             uint64        `yaml:"chain_id" validate:"required_if=Enabled true"`
	GasPriceWei         string        `yaml:"gas_price_wei" default:"1000000000"`
	GasLimit            uint64        `yaml:"gas_limit" default:"21000"`
	NativeBalanceWei    string        `yaml:"native_balance_wei" default:"1000000000000000000000"`
	RequestTimeout      time.Duration `yaml:"request_timeout"  default:"30s"`
	MinerInterval       time.Duration `yaml:"miner_interval"   default:"2s"`
	MinerMaxTxsPerBlock int           `yaml:"miner_max_txs_per_block" default:"500"`
	// SubmitterInterval controls how often the submitter drains pending
	// mempool entries by calling Canton. Kept tight by default so that
	// eth_sendRawTransaction → on-Canton latency stays close to Canton's
	// own commit time even though the HTTP call returns immediately.
	SubmitterInterval time.Duration `yaml:"submitter_interval" default:"500ms"`
	// SubmitterBatchSize caps the number of pending entries fetched in a
	// single submitter tick (0 = unlimited). Bounded so a backlog never
	// loads the entire pending queue into memory; the next tick picks up
	// whatever is left.
	SubmitterBatchSize int `yaml:"submitter_batch_size" default:"100"`
	// SubmitterConcurrency is the number of pending entries the submitter
	// processes in parallel within one tick. Canton commits typically take
	// 5-15s, so 10 parallel transfers give ~10x throughput vs sequential
	// without hammering Canton or saturating the gRPC connection.
	SubmitterConcurrency int `yaml:"submitter_concurrency" default:"10"`
}
