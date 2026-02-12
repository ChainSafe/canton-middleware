package client

import "github.com/chainsafe/canton-middleware/pkg/canton-sdk/ledger"

// Config contains the configuration required to initialize the SDK client.
// It aggregates all sub-component configurations needed by the SDK.
type Config struct {
	Ledger ledger.Config
}
