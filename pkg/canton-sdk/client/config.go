package client

import (
	"github.com/chainsafe/canton-middleware/pkg/canton-sdk/bridge"
	"github.com/chainsafe/canton-middleware/pkg/canton-sdk/identity"
	"github.com/chainsafe/canton-middleware/pkg/canton-sdk/ledger"
	"github.com/chainsafe/canton-middleware/pkg/canton-sdk/token"
)

// Config contains the configuration required to initialize the SDK client.
// It aggregates all sub-component configurations needed by the SDK.
type Config struct {
	Ledger   ledger.Config
	Identity identity.Config
	Token    token.Config
	Bridge   *bridge.Config // optional; nil disables bridge client
}
