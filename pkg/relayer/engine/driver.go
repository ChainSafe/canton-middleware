// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/chainsafe/canton-middleware/pkg/relayer"
)

const (
	// defaultStepInterval paces the step loop when the config does not set one.
	defaultStepInterval = 30 * time.Second

	// defaultStepBatchLimit caps how many transfers a single step tick loads.
	defaultStepBatchLimit = 100

	ingestRestartInitialBackoff = 1 * time.Second
	ingestRestartMaxBackoff     = 30 * time.Second
)

// Driver runs the TokenBridge adapter pipelines: one ingest loop per adapter
// source and a single step loop that advances every adapter-owned transfer
// until it reaches a terminal status. It coexists with the legacy single-token
// pipeline, which owns transfers keyed "wayfinder" until its port to an
// adapter (#372); the step loop only touches bridge keys present in the
// registry.
type Driver struct {
	cfg      *relayer.Config
	registry *relayer.Registry
	store    BridgeStore
	metrics  *Metrics
	logger   *zap.Logger

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewDriver creates a driver over the given adapter registry.
func NewDriver(
	cfg *relayer.Config,
	registry *relayer.Registry,
	store BridgeStore,
	metrics *Metrics,
	logger *zap.Logger,
) *Driver {
	return &Driver{
		cfg:      cfg,
		registry: registry,
		store:    store,
		metrics:  metrics,
		logger:   logger,
	}
}

// Start launches the ingest loops and the step loop. It wraps ctx so that
// Stop() can cancel all goroutines.
func (d *Driver) Start(ctx context.Context) error {
	d.logger.Info("Starting bridge driver", zap.Strings("bridges", d.registry.Keys()))
	ctx, d.cancel = context.WithCancel(ctx)

	for _, b := range d.registry.Bridges() {
		sources, err := b.Sources(ctx)
		if err != nil {
			return fmt.Errorf("bridge %s sources: %w", b.Key(), err)
		}
		for _, src := range sources {
			d.wg.Add(1)
			go d.runIngest(ctx, b, src)
		}
	}

	d.wg.Add(1)
	go d.runStepLoop(ctx)

	return nil
}

// Stop cancels all goroutines started by Start and waits for them to finish.
func (d *Driver) Stop() {
	d.logger.Info("Stopping bridge driver")
	if d.cancel != nil {
		d.cancel()
	}
	d.wg.Wait()
	d.logger.Info("Bridge driver stopped")
}

// runStepLoop periodically advances every due adapter-owned transfer.
func (d *Driver) runStepLoop(ctx context.Context) {
	defer d.wg.Done()

	ticker := time.NewTicker(d.stepInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.stepDueTransfers(ctx)
		}
	}
}

// stepDueTransfers loads the transfers due for a step and advances each one.
func (d *Driver) stepDueTransfers(ctx context.Context) {
	keys := d.registry.Keys()
	if len(keys) == 0 {
		return
	}

	transfers, err := d.store.GetSteppableTransfers(ctx, keys, defaultStepBatchLimit)
	if err != nil {
		d.logger.Error("Failed to load steppable transfers", zap.Error(err))
		return
	}

	for _, t := range transfers {
		if ctx.Err() != nil {
			return
		}
		d.stepTransfer(ctx, t)
	}
}

// stepTransfer advances a single transfer one stage and persists the outcome.
func (d *Driver) stepTransfer(ctx context.Context, t *relayer.Transfer) {
	bridge, ok := d.registry.ByKey(t.BridgeKey)
	if !ok {
		// GetSteppableTransfers filters on registry keys, so this indicates a
		// registry/config change mid-flight. Fail the transfer explicitly
		// rather than letting it spin forever.
		errMsg := fmt.Sprintf("no adapter registered for bridge key %q", t.BridgeKey)
		d.logger.Error("Orphaned transfer", zap.String("id", t.ID), zap.String("bridge", t.BridgeKey))
		if err := d.store.UpdateTransferStatus(ctx, t.ID, relayer.TransferStatusFailed, nil, &errMsg); err != nil {
			d.logger.Warn("Failed to mark orphaned transfer as failed", zap.String("id", t.ID), zap.Error(err))
		}
		return
	}

	if d.cfg.MaxRetries > 0 && t.RetryCount >= d.cfg.MaxRetries {
		errMsg := fmt.Sprintf("max retries (%d) exceeded", d.cfg.MaxRetries)
		d.logger.Warn("Transfer exceeded max retries, marking as failed",
			zap.String("id", t.ID),
			zap.String("bridge", t.BridgeKey),
			zap.Int("retry_count", t.RetryCount))
		d.metrics.IncTransferRetries(t.Direction, RetryOutcomeMaxExceeded)
		if err := d.store.UpdateTransferStatus(ctx, t.ID, relayer.TransferStatusFailed, nil, &errMsg); err != nil {
			d.logger.Warn("Failed to mark transfer as failed after max retries", zap.String("id", t.ID), zap.Error(err))
		}
		return
	}

	res, err := bridge.Step(ctx, t)
	if err != nil {
		d.metrics.IncSteps(t.BridgeKey, t.Stage, StepOutcomeError)
		d.logger.Error("Step failed",
			zap.String("id", t.ID),
			zap.String("bridge", t.BridgeKey),
			zap.String("stage", t.Stage),
			zap.Error(err))
		if recErr := d.store.RecordStepError(ctx, t.ID, err.Error(), time.Now().Add(d.retryDelay())); recErr != nil {
			d.logger.Warn("Failed to record step error", zap.String("id", t.ID), zap.Error(recErr))
		}
		return
	}

	if res.Status == "" {
		// An adapter returning no status is a bug; surface it as a step error
		// so the retry/backoff machinery applies instead of silently looping.
		d.metrics.IncSteps(t.BridgeKey, t.Stage, StepOutcomeError)
		d.logger.Error("Adapter returned empty status", zap.String("id", t.ID), zap.String("bridge", t.BridgeKey))
		errMsg := fmt.Sprintf("adapter %q returned empty status", t.BridgeKey)
		if recErr := d.store.RecordStepError(ctx, t.ID, errMsg, time.Now().Add(d.retryDelay())); recErr != nil {
			d.logger.Warn("Failed to record step error", zap.String("id", t.ID), zap.Error(recErr))
		}
		return
	}

	d.metrics.IncSteps(t.BridgeKey, res.Stage, StepOutcomeSuccess)

	retryAfter := res.RetryAfter
	if retryAfter <= 0 {
		retryAfter = d.stepInterval()
	}
	if err := d.store.ApplyStep(ctx, t.ID, res, time.Now().Add(retryAfter)); err != nil {
		d.logger.Warn("Failed to apply step result", zap.String("id", t.ID), zap.Error(err))
		return
	}

	if res.Status.IsTerminal() {
		d.logger.Info("Transfer reached terminal status",
			zap.String("id", t.ID),
			zap.String("bridge", t.BridgeKey),
			zap.String("status", string(res.Status)))
		if res.Status == relayer.TransferStatusCompleted {
			d.metrics.IncTransfersTotal(t.Direction, TransferResultCompleted)
			d.metrics.ObserveTransferAge(t.Direction, time.Since(t.CreatedAt).Seconds())
		}
		return
	}

	d.logger.Debug("Transfer stepped",
		zap.String("id", t.ID),
		zap.String("bridge", t.BridgeKey),
		zap.String("stage", res.Stage))
}

// runIngest streams events from one adapter source into the transfers table.
// Detection only: submission and progress belong to the step loop. The stream
// is restarted with exponential backoff on errors, resuming from the last
// persisted offset.
func (d *Driver) runIngest(ctx context.Context, b relayer.TokenBridge, src relayer.Source) {
	defer d.wg.Done()

	offsetKey := ingestOffsetKey(b.Key(), src.GetChainID())
	backoff := ingestRestartInitialBackoff

	for {
		offset, err := d.loadIngestOffset(ctx, offsetKey)
		if err != nil {
			d.logger.Error("Failed to load ingest offset", zap.String("key", offsetKey), zap.Error(err))
		} else if streamErr := d.consumeStream(ctx, b, src, offsetKey, offset); streamErr == nil {
			backoff = ingestRestartInitialBackoff
		}

		if ctx.Err() != nil {
			return
		}

		d.logger.Warn("Ingest stream stopped; restarting with backoff",
			zap.String("bridge", b.Key()),
			zap.String("chain", src.GetChainID()),
			zap.Duration("restart_in", backoff))

		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		if backoff *= 2; backoff > ingestRestartMaxBackoff {
			backoff = ingestRestartMaxBackoff
		}
	}
}

// consumeStream drains one StreamEvents session. Returns nil when the stream
// closed cleanly and an error when it failed.
func (d *Driver) consumeStream(
	ctx context.Context, b relayer.TokenBridge, src relayer.Source, offsetKey, offset string,
) error {
	eventCh, errCh := src.StreamEvents(ctx, offset)

	for {
		select {
		case event, ok := <-eventCh:
			if !ok {
				return nil
			}
			if event.Checkpoint {
				d.persistIngestOffset(ctx, src, offsetKey, event)
				continue
			}
			d.ingestEvent(ctx, b, src, offsetKey, event)
		case err := <-errCh:
			if err != nil {
				d.logger.Error("Ingest source stream error",
					zap.String("bridge", b.Key()),
					zap.String("chain", src.GetChainID()),
					zap.Error(err))
				return err
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// ingestEvent records a detected event as a pending transfer (idempotent) and
// advances the persisted offset.
func (d *Driver) ingestEvent(
	ctx context.Context, b relayer.TokenBridge, src relayer.Source, offsetKey string, event *relayer.Event,
) {
	transfer := relayer.TransferFromEvent(b.Key(), event)

	inserted, err := d.store.CreateTransfer(ctx, transfer)
	if err != nil {
		d.metrics.IncEventProcessingErrors(src.GetChainID(), StageCreateTransfer)
		d.logger.Error("Failed to create ingested transfer",
			zap.String("event_id", event.ID),
			zap.String("bridge", b.Key()),
			zap.Error(err))
		return
	}
	if inserted {
		d.logger.Info("Ingested transfer",
			zap.String("id", event.ID),
			zap.String("bridge", b.Key()),
			zap.String("direction", string(event.Direction)),
			zap.String("amount", event.Amount))
	} else {
		d.logger.Debug("Event already ingested", zap.String("event_id", event.ID))
	}

	d.persistIngestOffset(ctx, src, offsetKey, event)
}

// persistIngestOffset saves the source's processing position under the
// bridge-scoped chain-state key.
func (d *Driver) persistIngestOffset(ctx context.Context, src relayer.Source, offsetKey string, event *relayer.Event) {
	offset := src.ExtractOffset(event)
	if offset == "" {
		return
	}

	var blockNumber uint64
	if n, err := strconv.ParseUint(offset, 10, 64); err == nil {
		blockNumber = n
	}

	if err := d.store.SetChainState(ctx, offsetKey, blockNumber, offset); err != nil {
		d.logger.Warn("Failed to persist ingest offset",
			zap.String("key", offsetKey),
			zap.String("offset", offset),
			zap.Error(err))
	}
}

// loadIngestOffset returns the stored offset for a bridge-scoped chain key,
// or "" when none is stored yet.
func (d *Driver) loadIngestOffset(ctx context.Context, offsetKey string) (string, error) {
	state, err := d.store.GetChainState(ctx, offsetKey)
	if err != nil {
		return "", err
	}
	if state == nil {
		return "", nil
	}
	return state.Offset, nil
}

func (d *Driver) stepInterval() time.Duration {
	if d.cfg.ProcessingInterval > 0 {
		return d.cfg.ProcessingInterval
	}
	return defaultStepInterval
}

func (d *Driver) retryDelay() time.Duration {
	if d.cfg.RetryDelay > 0 {
		return d.cfg.RetryDelay
	}
	return stuckTransferThreshold
}

// ingestOffsetKey namespaces chain-state rows per bridge so two adapters
// watching the same chain never share a cursor. The legacy pipeline keeps
// its un-namespaced chain keys.
func ingestOffsetKey(bridgeKey, chainID string) string {
	return bridgeKey + ":" + chainID
}
