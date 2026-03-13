package streaming

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/ledger"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	reconnectBaseDelay = 5 * time.Second
	reconnectMaxDelay  = 60 * time.Second
	txChannelCap       = 100
)

// Streamer is the interface for opening a live Canton ledger stream.
// *Client satisfies this interface.
type Streamer interface {
	Subscribe(ctx context.Context, req SubscribeRequest, lastOffset *int64) <-chan *LedgerTransaction
}

// Client wraps UpdateService.GetUpdates with automatic reconnection and auth handling.
// It mirrors the streaming pattern established in pkg/cantonsdk/bridge/client.go.
type Client struct {
	ledger ledger.Ledger
	party  string
	logger *zap.Logger
}

// New creates a new streaming Client for the given ledger and party.
func New(l ledger.Ledger, party string, opts ...Option) *Client {
	s := applyOptions(opts)
	return &Client{
		ledger: l,
		party:  party,
		logger: s.logger,
	}
}

// Subscribe opens a live stream against the Canton ledger and returns a read-only channel
// of decoded transactions. It reconnects automatically with exponential backoff (5s → 60s)
// on transient errors, and invalidates the auth token on 401/403.
//
// lastOffset is updated atomically after each received transaction so that reconnects
// resume from the last safely received point. The caller is responsible for persisting
// lastOffset to the database (the processor does this atomically with event writes).
//
// The returned channel is closed when ctx is cancelled or a terminal error occurs
// (io.EOF, context cancellation).
func (c *Client) Subscribe(
	ctx context.Context,
	req SubscribeRequest,
	lastOffset *int64,
) <-chan *LedgerTransaction {
	out := make(chan *LedgerTransaction, txChannelCap)

	go func() {
		defer close(out)

		reconnectDelay := reconnectBaseDelay

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			err := c.runStream(ctx, req.FromOffset, req.TemplateIDs, lastOffset, out)
			if err == nil || errors.Is(err, io.EOF) || ctx.Err() != nil {
				return
			}

			if isAuthError(err) {
				c.ledger.InvalidateToken()
				reconnectDelay = reconnectBaseDelay
			}

			// Advance FromOffset to where the stream last successfully delivered a
			// transaction so the next connection resumes from the correct position.
			req.FromOffset = atomic.LoadInt64(lastOffset)

			c.logger.Warn("canton stream disconnected, reconnecting",
				zap.Error(err),
				zap.Int64("resume_offset", req.FromOffset),
				zap.Duration("backoff", reconnectDelay),
			)

			select {
			case <-ctx.Done():
				return
			case <-time.After(reconnectDelay):
			}

			reconnectDelay = min(reconnectDelay*2, reconnectMaxDelay)
		}
	}()

	return out
}

// runStream opens a single GetUpdates stream and forwards transactions to out until
// the stream ends or ctx is cancelled. It updates lastOffset atomically on each
// received transaction.
func (c *Client) runStream(
	ctx context.Context,
	fromOffset int64,
	templateIDs []TemplateID,
	lastOffset *int64,
	out chan<- *LedgerTransaction,
) error {
	authCtx := c.ledger.AuthContext(ctx)

	stream, err := c.ledger.Update().GetUpdates(authCtx, &lapiv2.GetUpdatesRequest{
		BeginExclusive: fromOffset,
		UpdateFormat: &lapiv2.UpdateFormat{
			IncludeTransactions: &lapiv2.TransactionFormat{
				EventFormat: &lapiv2.EventFormat{
					FiltersByParty: map[string]*lapiv2.Filters{
						c.party: buildTemplateFilters(templateIDs),
					},
					Verbose: true,
				},
				TransactionShape: lapiv2.TransactionShape_TRANSACTION_SHAPE_ACS_DELTA,
			},
		},
	})
	if err != nil {
		if isAuthError(err) {
			c.ledger.InvalidateToken()
		}
		return fmt.Errorf("open canton stream: %w", err)
	}

	for {
		resp, err := stream.Recv()
		if err != nil {
			if isAuthError(err) {
				c.ledger.InvalidateToken()
			}
			return err
		}

		tx := resp.GetTransaction()
		if tx == nil {
			// Checkpoint or topology update — nothing to index.
			continue
		}

		lt := decodeLedgerTransaction(tx)
		atomic.StoreInt64(lastOffset, lt.Offset)

		select {
		case out <- lt:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// buildTemplateFilters constructs the Filters value for a set of TemplateIDs.
//
// This is the gRPC-level (template-level) filter. It controls which contract types
// are delivered by the Canton Ledger API — reducing bandwidth to only the requested
// templates. It is NOT an instrument filter: it cannot filter by contract field values
// such as instrumentId. Instrument filtering (by InstrumentKey{Admin, ID}) happens
// downstream in the parser after events are received.
//
// When TemplateID.PackageID is empty the filter matches the template across all
// deployed package versions. Setting PackageID="" for CIP56.Events.TokenTransferEvent
// enables indexing of any third-party CIP56-compliant token regardless of which
// package version it was deployed with.
func buildTemplateFilters(templateIDs []TemplateID) *lapiv2.Filters {
	cumulative := make([]*lapiv2.CumulativeFilter, 0, len(templateIDs))
	for _, tid := range templateIDs {
		cumulative = append(cumulative, &lapiv2.CumulativeFilter{
			IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
				TemplateFilter: &lapiv2.TemplateFilter{
					TemplateId: &lapiv2.Identifier{
						PackageId:  tid.PackageID, // empty = match all package versions
						ModuleName: tid.ModuleName,
						EntityName: tid.EntityName,
					},
				},
			},
		})
	}
	return &lapiv2.Filters{Cumulative: cumulative}
}

// decodeLedgerTransaction converts a proto Transaction into a LedgerTransaction.
func decodeLedgerTransaction(tx *lapiv2.Transaction) *LedgerTransaction {
	lt := &LedgerTransaction{
		UpdateID:      tx.GetUpdateId(),
		Offset:        tx.GetOffset(),
		EffectiveTime: tx.GetEffectiveAt().AsTime(),
		Events:        make([]*LedgerEvent, 0, len(tx.Events)),
	}
	for _, ev := range tx.Events {
		if le := decodeLedgerEvent(ev); le != nil {
			lt.Events = append(lt.Events, le)
		}
	}
	return lt
}

// decodeLedgerEvent converts a proto Event to a LedgerEvent.
// For created events the DAML CreateArguments are pre-decoded into LedgerEvent.fields
// so that callers never need to import lapiv2 directly.
// Returns nil for event kinds the indexer does not process (e.g. exercised events).
func decodeLedgerEvent(ev *lapiv2.Event) *LedgerEvent {
	if created := ev.GetCreated(); created != nil {
		le := &LedgerEvent{
			ContractID: created.GetContractId(),
			IsCreated:  true,
			fields:     values.RecordToMap(created.GetCreateArguments()),
		}
		if tid := created.GetTemplateId(); tid != nil {
			le.PackageID = tid.GetPackageId()
			le.ModuleName = tid.GetModuleName()
			le.TemplateName = tid.GetEntityName()
		}
		return le
	}

	if archived := ev.GetArchived(); archived != nil {
		le := &LedgerEvent{
			ContractID: archived.GetContractId(),
			IsCreated:  false,
		}
		if tid := archived.GetTemplateId(); tid != nil {
			le.PackageID = tid.GetPackageId()
			le.ModuleName = tid.GetModuleName()
			le.TemplateName = tid.GetEntityName()
		}
		return le
	}

	return nil
}

// isAuthError returns true if err signals authentication or authorisation failure.
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	return st.Code() == codes.Unauthenticated || st.Code() == codes.PermissionDenied
}
