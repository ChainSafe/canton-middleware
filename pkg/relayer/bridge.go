// SPDX-License-Identifier: Apache-2.0

package relayer

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// Source streams bridge events from a chain for a TokenBridge adapter.
// It mirrors the legacy engine source contract so existing implementations
// can be ported without change.
//
//go:generate mockery --name Source --output mocks --outpkg mocks --filename mock_source.go --with-expecter
type Source interface {
	StreamEvents(ctx context.Context, offset string) (<-chan *Event, <-chan error)
	GetChainID() string
	// ExtractOffset returns the offset string to persist after processing event.
	// Returns "" when no offset should be saved for this event.
	ExtractOffset(event *Event) string
}

// StepResult is the outcome of a single TokenBridge.Step call.
type StepResult struct {
	// Status is the transfer status after this step. Required.
	Status TransferStatus
	// Stage is the mechanism-defined progress marker (e.g. "awaiting_attestation").
	Stage string
	// DestTxHash, when set, records the destination-chain transaction hash.
	DestTxHash *string
	// Metadata is merged into the transfer's stored metadata.
	Metadata map[string]string
	// RetryAfter hints when the driver should step this transfer again.
	// Zero means the driver's default processing interval.
	RetryAfter time.Duration
}

// TokenBridge is a bridging-mechanism adapter. A transfer is a durable record
// advanced by an idempotent Step call until it reaches a terminal status;
// mechanisms differ only in their stage sequences.
//
// Executor mechanisms (we perform the bridge) expose Sources so missed events
// never strand funds. Observer mechanisms (an external party performs the
// bridge, e.g. Circle xReserve) return no sources: their transfers are
// registered at initiation time and Step only tracks progress.
//
//go:generate mockery --name TokenBridge --output mocks --outpkg mocks --filename mock_token_bridge.go --with-expecter
type TokenBridge interface {
	// Key is the stable mechanism identifier stored on every transfer row.
	Key() string
	// Sources returns the event streams that detect new transfer intents.
	// May be empty for observer mechanisms.
	Sources(ctx context.Context) ([]Source, error)
	// Step advances one transfer one stage. It must be idempotent and must
	// not block on external latency: report the current state and a
	// RetryAfter hint instead.
	Step(ctx context.Context, t *Transfer) (StepResult, error)
}

// Registry holds the configured TokenBridge adapters keyed by bridge key.
type Registry struct {
	bridges map[string]TokenBridge
	keys    []string
}

// NewRegistry creates an empty adapter registry.
func NewRegistry() *Registry {
	return &Registry{bridges: make(map[string]TokenBridge)}
}

// Register adds a TokenBridge to the registry. It rejects empty and duplicate keys.
func (r *Registry) Register(b TokenBridge) error {
	key := b.Key()
	if key == "" {
		return fmt.Errorf("token bridge key must not be empty")
	}
	if _, exists := r.bridges[key]; exists {
		return fmt.Errorf("token bridge %q already registered", key)
	}
	r.bridges[key] = b
	r.keys = append(r.keys, key)
	sort.Strings(r.keys)
	return nil
}

// ByKey returns the TokenBridge registered under key.
func (r *Registry) ByKey(key string) (TokenBridge, bool) {
	b, ok := r.bridges[key]
	return b, ok
}

// Keys returns the registered bridge keys in deterministic order.
func (r *Registry) Keys() []string {
	keys := make([]string, len(r.keys))
	copy(keys, r.keys)
	return keys
}

// Bridges returns the registered adapters in deterministic key order.
func (r *Registry) Bridges() []TokenBridge {
	bridges := make([]TokenBridge, 0, len(r.keys))
	for _, key := range r.keys {
		bridges = append(bridges, r.bridges[key])
	}
	return bridges
}

// TransferFromEvent builds the initial transfer row for an event ingested
// from a TokenBridge source.
func TransferFromEvent(bridgeKey string, e *Event) *Transfer {
	return &Transfer{
		ID:                e.ID,
		BridgeKey:         bridgeKey,
		TokenSymbol:       e.TokenSymbol,
		Direction:         e.Direction,
		Status:            TransferStatusPending,
		SourceChain:       e.SourceChain,
		DestinationChain:  e.DestinationChain,
		SourceTxHash:      e.SourceTxHash,
		TokenAddress:      e.TokenAddress,
		Amount:            e.Amount,
		Sender:            e.Sender,
		Recipient:         e.Recipient,
		Nonce:             e.Nonce,
		SourceBlockNumber: e.SourceBlockNumber,
		CreatedAt:         time.Now(),
	}
}
