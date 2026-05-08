package custodial

import "time"

// AcceptWorkerConfig configures the background worker that auto-accepts inbound
// USDCx TransferOffers for custodial parties. Omitting this block disables the worker.
type AcceptWorkerConfig struct {
	// IndexerURL is the base URL of the indexer service (e.g. "http://localhost:8081").
	// Required when the worker is enabled.
	IndexerURL   string        `yaml:"indexer_url" validate:"required"`
	PollInterval time.Duration `yaml:"poll_interval" default:"10s"`
}
