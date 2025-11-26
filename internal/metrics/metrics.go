package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// TransfersTotal counts total transfers by direction and status
	TransfersTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bridge_transfers_total",
			Help: "Total number of bridge transfers",
		},
		[]string{"direction", "status"},
	)

	// TransferDuration tracks transfer processing time
	TransferDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "bridge_transfer_duration_seconds",
			Help:    "Transfer processing duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"direction"},
	)

	// TransferAmount tracks the amount of tokens transferred
	TransferAmount = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "bridge_transfer_amount",
			Help:    "Amount of tokens transferred",
			Buckets: []float64{0.001, 0.01, 0.1, 1, 10, 100, 1000, 10000},
		},
		[]string{"direction", "token"},
	)

	// BlocksProcessed counts blocks processed on each chain
	BlocksProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bridge_blocks_processed_total",
			Help: "Total number of blocks processed",
		},
		[]string{"chain"},
	)

	// EventsDetected counts events detected on each chain
	EventsDetected = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bridge_events_detected_total",
			Help: "Total number of bridge events detected",
		},
		[]string{"chain", "event_type"},
	)

	// TransactionsSent counts transactions sent to each chain
	TransactionsSent = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bridge_transactions_sent_total",
			Help: "Total number of transactions sent",
		},
		[]string{"chain", "status"},
	)

	// BridgeBalance tracks current bridge balances
	BridgeBalance = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bridge_balance",
			Help: "Current bridge balance by chain and token",
		},
		[]string{"chain", "token"},
	)

	// PendingTransfers tracks number of pending transfers
	PendingTransfers = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bridge_pending_transfers",
			Help: "Number of pending transfers by direction",
		},
		[]string{"direction"},
	)

	// ErrorsTotal counts errors by type
	ErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bridge_errors_total",
			Help: "Total number of errors",
		},
		[]string{"component", "error_type"},
	)

	// GasUsed tracks gas used for Ethereum transactions
	GasUsed = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "bridge_gas_used",
			Help:    "Gas used for Ethereum transactions",
			Buckets: []float64{21000, 50000, 100000, 200000, 300000, 500000},
		},
		[]string{"operation"},
	)

	// LastProcessedBlock tracks the last processed block number
	LastProcessedBlock = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bridge_last_processed_block",
			Help: "Last processed block number by chain",
		},
		[]string{"chain"},
	)
)
