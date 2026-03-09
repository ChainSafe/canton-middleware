// Package bridge implements optional Wayfinder bridge operations for Canton.
//
// It provides deposit/withdrawal flows and bridge-related event queries.
// The bridge client is optional.
package bridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/identity"
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/ledger"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	streamReconnectDelay      = 5 * time.Second
	streamMaxReconnectDelay   = 60 * time.Second
	withdrawalEventChannelCap = 10
)

// Bridge defines bridge operations.
type Bridge interface {
	// IsDepositProcessed returns true if a deposit with the given EVM tx hash already exists as an active
	// PendingDeposit or DepositReceipt contract.
	IsDepositProcessed(ctx context.Context, evmTxHash string) (bool, error)

	// GetWayfinderBridgeConfigCID returns the active WayfinderBridgeConfig contract id.
	GetWayfinderBridgeConfigCID(ctx context.Context) (string, error)

	// CreatePendingDeposit creates a PendingDeposit on Canton from an EVM deposit event.
	CreatePendingDeposit(ctx context.Context, req CreatePendingDepositRequest) (*PendingDeposit, error)

	// ProcessDepositAndMint processes a PendingDeposit and mints tokens (choice on WayfinderBridgeConfig).
	ProcessDepositAndMint(ctx context.Context, req ProcessDepositRequest) (*ProcessedDeposit, error)

	// InitiateWithdrawal creates a WithdrawalRequest for a user (choice on WayfinderBridgeConfig).
	InitiateWithdrawal(ctx context.Context, req InitiateWithdrawalRequest) (string, error)

	// CompleteWithdrawal marks a WithdrawalEvent as completed after the EVM release is finalized.
	CompleteWithdrawal(ctx context.Context, req CompleteWithdrawalRequest) error

	// StreamWithdrawalEvents streams WithdrawalEvent contracts with automatic reconnection.
	StreamWithdrawalEvents(ctx context.Context, offset string) <-chan *WithdrawalEvent

	// GetLatestLedgerOffset returns the ledger end
	GetLatestLedgerOffset(ctx context.Context) (int64, error)
}

// Client implements bridge operations.
type Client struct {
	cfg      *Config
	ledger   ledger.Ledger
	identity identity.Identity
	logger   *zap.Logger
}

// New creates a new bridge client.
func New(cfg *Config, l ledger.Ledger, i identity.Identity, opts ...Option) (*Client, error) {
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	if l == nil {
		return nil, fmt.Errorf("nil ledger client")
	}
	s := applyOptions(opts)

	return &Client{
		cfg:      cfg,
		ledger:   l,
		identity: i,
		logger:   s.logger,
	}, nil
}

func (c *Client) GetWayfinderBridgeConfigCID(ctx context.Context) (string, error) {
	end, err := c.ledger.GetLedgerEnd(ctx)
	if err != nil {
		return "", err
	}
	if end == 0 {
		return "", fmt.Errorf("ledger is empty, no contracts exist")
	}

	tid := &lapiv2.Identifier{
		PackageId:  c.cfg.BridgePackageID,
		ModuleName: c.cfg.BridgeModule,
		EntityName: "WayfinderBridgeConfig",
	}

	events, err := c.ledger.GetActiveContractsByTemplate(ctx, end, []string{c.cfg.RelayerParty}, tid)
	if err != nil {
		return "", fmt.Errorf("query WayfinderBridgeConfig: %w", err)
	}
	if len(events) == 0 {
		return "", fmt.Errorf("no active WayfinderBridgeConfig found")
	}

	return events[0].ContractId, nil
}

func (c *Client) IsDepositProcessed(ctx context.Context, evmTxHash string) (bool, error) {
	if evmTxHash == "" {
		return false, fmt.Errorf("evm tx hash is required")
	}

	end, err := c.ledger.GetLedgerEnd(ctx)
	if err != nil {
		return false, err
	}
	if end == 0 {
		return false, nil
	}

	// We enforce module/entity filtering via template id. This assumes deposits live in the same package/module.
	// If your deposits are in a different package/module, adjust these template IDs accordingly.
	pendingTID := &lapiv2.Identifier{
		PackageId:  c.cfg.BridgePackageID,
		ModuleName: "Common.FingerprintAuth",
		EntityName: "PendingDeposit",
	}
	receiptTID := &lapiv2.Identifier{
		PackageId:  c.cfg.BridgePackageID,
		ModuleName: "Common.FingerprintAuth",
		EntityName: "DepositReceipt",
	}

	check := func(tid *lapiv2.Identifier) (bool, error) {
		var events []*lapiv2.CreatedEvent
		events, err = c.ledger.GetActiveContractsByTemplate(ctx, end, []string{c.cfg.RelayerParty}, tid)
		if err != nil {
			return false, err
		}
		for _, ce := range events {
			fields := values.RecordToMap(ce.CreateArguments)
			if values.Text(fields["evmTxHash"]) == evmTxHash {
				return true, nil
			}
		}
		return false, nil
	}

	ok, err := check(pendingTID)
	if err != nil {
		return false, fmt.Errorf("query PendingDeposit: %w", err)
	}
	if ok {
		return true, nil
	}

	ok, err = check(receiptTID)
	if err != nil {
		return false, fmt.Errorf("query DepositReceipt: %w", err)
	}
	return ok, nil
}

func (c *Client) CreatePendingDeposit(ctx context.Context, req CreatePendingDepositRequest) (*PendingDeposit, error) {
	if err := req.validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	configCID, err := c.GetWayfinderBridgeConfigCID(ctx)
	if err != nil {
		return nil, err
	}

	m, err := c.identity.GetFingerprintMapping(ctx, req.Fingerprint)
	if err != nil {
		return nil, err
	}
	req.Fingerprint = m.Fingerprint // replace with normalized fingerprint

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  c.cfg.BridgePackageID,
					ModuleName: c.cfg.BridgeModule,
					EntityName: "WayfinderBridgeConfig",
				},
				ContractId:     configCID,
				Choice:         "CreatePendingDeposit",
				ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: encodeCreatePendingDepositArgs(req)}},
			},
		},
	}

	resp, err := c.ledger.Command().SubmitAndWaitForTransaction(
		c.ledger.AuthContext(ctx),
		&lapiv2.SubmitAndWaitForTransactionRequest{
			Commands: &lapiv2.Commands{
				SynchronizerId: c.cfg.DomainID,
				CommandId:      uuid.NewString(),
				UserId:         c.cfg.UserID,
				ActAs:          []string{c.cfg.RelayerParty},
				Commands:       []*lapiv2.Command{cmd},
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create pending deposit: %w", err)
	}
	if resp.Transaction == nil {
		return nil, fmt.Errorf("create pending deposit: missing transaction in response")
	}

	for _, e := range resp.Transaction.Events {
		created := e.GetCreated()
		if created == nil || created.TemplateId == nil {
			continue
		}
		if created.TemplateId.ModuleName == "Common.FingerprintAuth" && created.TemplateId.EntityName == "PendingDeposit" {
			return &PendingDeposit{
				ContractID:  created.ContractId,
				MappingCID:  m.ContractID,
				Fingerprint: m.Fingerprint,
				CreatedAt:   created.CreatedAt.AsTime(),
			}, nil
		}
	}

	return nil, fmt.Errorf("PendingDeposit contract not found in response")
}

func (c *Client) ProcessDepositAndMint(ctx context.Context, req ProcessDepositRequest) (*ProcessedDeposit, error) {
	if err := req.validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	configCID, err := c.GetWayfinderBridgeConfigCID(ctx)
	if err != nil {
		return nil, err
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  c.cfg.BridgePackageID,
					ModuleName: c.cfg.BridgeModule,
					EntityName: "WayfinderBridgeConfig",
				},
				ContractId:     configCID,
				Choice:         "ProcessDepositAndMint",
				ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: encodeProcessDepositAndMintArgs(req)}},
			},
		},
	}

	resp, err := c.ledger.Command().SubmitAndWaitForTransaction(
		c.ledger.AuthContext(ctx),
		&lapiv2.SubmitAndWaitForTransactionRequest{
			Commands: &lapiv2.Commands{
				SynchronizerId: c.cfg.DomainID,
				CommandId:      uuid.NewString(),
				UserId:         c.cfg.UserID,
				ActAs:          []string{c.cfg.RelayerParty},
				Commands:       []*lapiv2.Command{cmd},
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("process deposit and mint: %w", err)
	}
	if resp.Transaction == nil {
		return nil, fmt.Errorf("process deposit and mint: missing transaction in response")
	}

	for _, e := range resp.Transaction.Events {
		created := e.GetCreated()
		if created == nil || created.TemplateId == nil {
			continue
		}
		// Mint results in a holding being created.
		if created.TemplateId.ModuleName == "CIP56.Token" && created.TemplateId.EntityName == "CIP56Holding" {
			return &ProcessedDeposit{ContractID: created.ContractId}, nil
		}
	}

	return nil, fmt.Errorf("CIP56Holding contract not found in response")
}

func (c *Client) InitiateWithdrawal(ctx context.Context, req InitiateWithdrawalRequest) (string, error) {
	if err := req.validate(); err != nil {
		return "", fmt.Errorf("invalid request: %w", err)
	}

	configCID, err := c.GetWayfinderBridgeConfigCID(ctx)
	if err != nil {
		return "", err
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  c.cfg.BridgePackageID,
					ModuleName: c.cfg.BridgeModule,
					EntityName: "WayfinderBridgeConfig",
				},
				ContractId:     configCID,
				Choice:         "InitiateWithdrawal",
				ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: encodeInitiateWithdrawalArgs(req)}},
			},
		},
	}

	resp, err := c.ledger.Command().SubmitAndWaitForTransaction(
		c.ledger.AuthContext(ctx),
		&lapiv2.SubmitAndWaitForTransactionRequest{
			Commands: &lapiv2.Commands{
				SynchronizerId: c.cfg.DomainID,
				CommandId:      uuid.NewString(),
				UserId:         c.cfg.UserID,
				ActAs:          []string{c.cfg.RelayerParty},
				Commands:       []*lapiv2.Command{cmd},
			},
		},
	)
	if err != nil {
		return "", fmt.Errorf("initiate withdrawal: %w", err)
	}
	if resp.Transaction == nil {
		return "", fmt.Errorf("initiate withdrawal: missing transaction in response")
	}

	for _, e := range resp.Transaction.Events {
		created := e.GetCreated()
		if created == nil || created.TemplateId == nil {
			continue
		}
		if created.TemplateId.ModuleName == "Bridge.Contracts" && created.TemplateId.EntityName == "WithdrawalRequest" {
			return created.ContractId, nil
		}
	}

	return "", fmt.Errorf("WithdrawalRequest contract not found in response")
}

func (c *Client) CompleteWithdrawal(ctx context.Context, req CompleteWithdrawalRequest) error {
	if err := req.validate(); err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}

	corePkg := c.cfg.effectiveCorePackageID()

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  corePkg,
					ModuleName: "Bridge.Contracts",
					EntityName: "WithdrawalEvent",
				},
				ContractId:     req.WithdrawalEventCID,
				Choice:         "CompleteWithdrawal",
				ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: encodeCompleteWithdrawalArgs(req.EvmTxHash)}},
			},
		},
	}

	_, err := c.ledger.Command().SubmitAndWait(
		c.ledger.AuthContext(ctx),
		&lapiv2.SubmitAndWaitRequest{
			Commands: &lapiv2.Commands{
				SynchronizerId: c.cfg.DomainID,
				CommandId:      uuid.NewString(),
				UserId:         c.cfg.UserID,
				ActAs:          []string{c.cfg.RelayerParty},
				Commands:       []*lapiv2.Command{cmd},
			},
		},
	)
	if err != nil {
		if isAlreadyExistsError(err) { // TODO: verify it works
			return nil
		}
		return fmt.Errorf("complete withdrawal: %w", err)
	}

	return nil
}

func (c *Client) StreamWithdrawalEvents(ctx context.Context, offset string) <-chan *WithdrawalEvent {
	outCh := make(chan *WithdrawalEvent, withdrawalEventChannelCap)

	go func() {
		defer close(outCh)

		currentOffset := offset
		reconnectDelay := streamReconnectDelay

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			err := c.streamWithdrawalEventsOnce(ctx, currentOffset, outCh, &currentOffset)
			if err == nil || errors.Is(err, io.EOF) || ctx.Err() != nil {
				return
			}

			if isAuthError(err) {
				c.ledger.InvalidateToken()
				reconnectDelay = streamReconnectDelay
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(reconnectDelay):
			}

			reconnectDelay = min(reconnectDelay*2, streamMaxReconnectDelay)
		}
	}()

	return outCh
}

func (c *Client) streamWithdrawalEventsOnce(ctx context.Context, offset string, outCh chan<- *WithdrawalEvent, lastOffset *string) error {
	authCtx := c.ledger.AuthContext(ctx)

	beginExclusive, err := parseOffset(offset)
	if err != nil {
		return err
	}

	corePkg := c.cfg.effectiveCorePackageID()

	updateFormat := &lapiv2.UpdateFormat{
		IncludeTransactions: &lapiv2.TransactionFormat{
			EventFormat: &lapiv2.EventFormat{
				FiltersByParty: map[string]*lapiv2.Filters{
					c.cfg.RelayerParty: {
						Cumulative: []*lapiv2.CumulativeFilter{
							{
								IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
									TemplateFilter: &lapiv2.TemplateFilter{
										TemplateId: &lapiv2.Identifier{
											PackageId:  corePkg,
											ModuleName: "Bridge.Contracts",
											EntityName: "WithdrawalEvent",
										},
									},
								},
							},
						},
					},
				},
				Verbose: true,
			},
			TransactionShape: lapiv2.TransactionShape_TRANSACTION_SHAPE_ACS_DELTA,
		},
	}

	stream, err := c.ledger.Update().GetUpdates(authCtx, &lapiv2.GetUpdatesRequest{
		BeginExclusive: beginExclusive,
		UpdateFormat:   updateFormat,
	})
	if err != nil {
		return fmt.Errorf("start withdrawal stream: %w", err)
	}

	for {
		resp, recvErr := stream.Recv()
		if recvErr != nil {
			return recvErr
		}

		tx := resp.GetTransaction()
		if tx == nil {
			continue
		}

		for _, ev := range tx.Events {
			created := ev.GetCreated()
			if created == nil || created.TemplateId == nil {
				continue
			}

			tid := created.TemplateId
			if tid.ModuleName != "Bridge.Contracts" || tid.EntityName != "WithdrawalEvent" {
				continue
			}

			we := decodeWithdrawalEvent(created, tx.UpdateId)

			if we.Status != WithdrawalStatusPending {
				continue
			}

			select {
			case outCh <- we:
				*lastOffset = strconv.FormatInt(created.Offset, 10)
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

func parseOffset(offset string) (int64, error) {
	if offset == "" || offset == "BEGIN" {
		return 0, nil
	}
	n, err := strconv.ParseInt(offset, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid offset %q: %w", offset, err)
	}
	return n, nil
}

// isAuthError checks if the error is an authentication/authorization error that requires token refresh
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

func (c *Client) GetLatestLedgerOffset(ctx context.Context) (int64, error) {
	return c.ledger.GetLedgerEnd(ctx)
}

func isAlreadyExistsError(err error) bool {
	if s, ok := status.FromError(err); ok {
		return s.Code() == codes.AlreadyExists
	}

	return false
}
