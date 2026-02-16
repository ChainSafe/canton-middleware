// Package bridge implements optional Wayfinder bridge operations for Canton.
//
// It provides deposit/withdrawal flows and bridge-related event queries.
// The bridge client is optional.
package bridge

import (
	"context"
	"fmt"
	"strings"

	"github.com/chainsafe/canton-middleware/pkg/canton-sdk/ledger"
	"github.com/chainsafe/canton-middleware/pkg/canton-sdk/values"
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Bridge defines bridge operations.
type Bridge interface {
	// IsDepositProcessed returns true if a deposit with the given EVM tx hash already exists as an active
	// PendingDeposit or DepositReceipt contract.
	IsDepositProcessed(ctx context.Context, evmTxHash string) (bool, error)

	// GetWayfinderBridgeConfigCID returns the active WayfinderBridgeConfig contract id.
	GetWayfinderBridgeConfigCID(ctx context.Context) (string, error)

	// CreatePendingDeposit creates a PendingDeposit on Canton from an EVM deposit event.
	CreatePendingDeposit(ctx context.Context, req CreatePendingDepositRequest) (string, error)

	// ProcessDepositAndMint processes a PendingDeposit and mints tokens (choice on WayfinderBridgeConfig).
	ProcessDepositAndMint(ctx context.Context, req ProcessDepositRequest) (string, error)

	// InitiateWithdrawal creates a WithdrawalRequest for a user (choice on WayfinderBridgeConfig).
	InitiateWithdrawal(ctx context.Context, req InitiateWithdrawalRequest) (string, error)

	// CompleteWithdrawal marks a WithdrawalEvent as completed after the EVM release is finalized.
	CompleteWithdrawal(ctx context.Context, req CompleteWithdrawalRequest) error

	// GetMintEvents returns all active CIP56.Events.MintEvent contracts visible to relayerParty.
	GetMintEvents(ctx context.Context) ([]*MintEvent, error)

	// GetBurnEvents returns all active CIP56.Events.BurnEvent contracts visible to relayerParty.
	GetBurnEvents(ctx context.Context) ([]*BurnEvent, error)
}

// Client implements bridge operations.
type Client struct {
	cfg    *Config
	ledger ledger.Ledger
	logger *zap.Logger
}

// New creates a new bridge client.
func New(cfg Config, l ledger.Ledger, opts ...Option) (*Client, error) {
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	if l == nil {
		return nil, fmt.Errorf("nil ledger client")
	}
	s := applyOptions(opts)

	return &Client{
		cfg:    &cfg,
		ledger: l,
		logger: s.logger,
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
		events, err := c.ledger.GetActiveContractsByTemplate(ctx, end, []string{c.cfg.RelayerParty}, tid)
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

func (c *Client) CreatePendingDeposit(ctx context.Context, req CreatePendingDepositRequest) (string, error) {
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
		return "", fmt.Errorf("create pending deposit: %w", err)
	}
	if resp.Transaction == nil {
		return "", fmt.Errorf("create pending deposit: missing transaction in response")
	}

	for _, e := range resp.Transaction.Events {
		created := e.GetCreated()
		if created == nil || created.TemplateId == nil {
			continue
		}
		if created.TemplateId.ModuleName == "Common.FingerprintAuth" && created.TemplateId.EntityName == "PendingDeposit" {
			return created.ContractId, nil
		}
	}

	return "", fmt.Errorf("PendingDeposit contract not found in response")
}

func (c *Client) ProcessDepositAndMint(ctx context.Context, req ProcessDepositRequest) (string, error) {
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
		return "", fmt.Errorf("process deposit and mint: %w", err)
	}
	if resp.Transaction == nil {
		return "", fmt.Errorf("process deposit and mint: missing transaction in response")
	}

	for _, e := range resp.Transaction.Events {
		created := e.GetCreated()
		if created == nil || created.TemplateId == nil {
			continue
		}
		// Mint results in a holding being created.
		if created.TemplateId.ModuleName == "CIP56.Token" && created.TemplateId.EntityName == "CIP56Holding" {
			return created.ContractId, nil
		}
	}

	return "", fmt.Errorf("CIP56Holding contract not found in response")
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
		// TODO: Match grpc error code without hard-wiring exact text.
		if strings.Contains(strings.ToLower(err.Error()), "already") {
			return nil
		}
		return fmt.Errorf("complete withdrawal: %w", err)
	}

	return nil
}

func (c *Client) GetMintEvents(ctx context.Context) ([]*MintEvent, error) {
	end, err := c.ledger.GetLedgerEnd(ctx)
	if err != nil {
		return nil, err
	}
	if end == 0 {
		return []*MintEvent{}, nil
	}

	tid := &lapiv2.Identifier{
		PackageId:  c.cfg.CIP56PackageID,
		ModuleName: "CIP56.Events",
		EntityName: "MintEvent",
	}

	events, err := c.ledger.GetActiveContractsByTemplate(ctx, end, []string{c.cfg.RelayerParty}, tid)
	if err != nil {
		return nil, fmt.Errorf("query MintEvent: %w", err)
	}

	out := make([]*MintEvent, 0, len(events))
	for _, ce := range events {
		out = append(out, decodeMintEvent(ce))
	}
	return out, nil
}

func (c *Client) GetBurnEvents(ctx context.Context) ([]*BurnEvent, error) {
	end, err := c.ledger.GetLedgerEnd(ctx)
	if err != nil {
		return nil, err
	}
	if end == 0 {
		return []*BurnEvent{}, nil
	}

	tid := &lapiv2.Identifier{
		PackageId:  c.cfg.CIP56PackageID,
		ModuleName: "CIP56.Events",
		EntityName: "BurnEvent",
	}

	events, err := c.ledger.GetActiveContractsByTemplate(ctx, end, []string{c.cfg.RelayerParty}, tid)
	if err != nil {
		return nil, fmt.Errorf("query BurnEvent: %w", err)
	}

	out := make([]*BurnEvent, 0, len(events))
	for _, ce := range events {
		out = append(out, decodeBurnEvent(ce))
	}
	return out, nil
}
