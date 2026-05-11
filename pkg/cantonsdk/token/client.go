// Package token implements CIP-56 token operations such as mint, burn, transfer,
// and balance queries.
package token

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/identity"
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	interactivev2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2/interactive"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/ledger"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ErrInsufficientBalance indicates the owner's total unlocked balance is less than the required amount.
var ErrInsufficientBalance = errors.New("insufficient balance")

// ErrTransferFactoryNotFound indicates no active CIP56TransferFactory contract exists on the ledger.
var ErrTransferFactoryNotFound = errors.New("no active CIP56TransferFactory found")

const (
	defaultTransferValidity = time.Hour

	moduleConfig          = "CIP56.Config"
	entityTokenConfig     = "TokenConfig"
	moduleToken           = "CIP56.Token"
	entityHolding         = "CIP56Holding"
	moduleTransferFactory = "CIP56.TransferFactory"
	entityTransferFactory = "CIP56TransferFactory"
	moduleEvents          = "CIP56.Events"

	spliceTransferModule  = "Splice.Api.Token.TransferInstructionV1"
	spliceTransferFactory = "TransferFactory"
	transferInstrEntity   = "TransferInstruction"
	acceptChoice          = "TransferInstruction_Accept"

	spliceHoldingModule = "Splice.Api.Token.HoldingV1"
	spliceHoldingEntity = "Holding"

	moduleTransferOffer = "Utility.Registry.App.V0.Model.Transfer"
	entityTransferOffer = "TransferOffer"
)

// Token defines CIP-56 token operations.
type Token interface {
	// GetTokenConfigCID returns the active TokenConfig contract ID for the given token symbol.
	GetTokenConfigCID(ctx context.Context, tokenSymbol string) (string, error)

	// Mint mints tokens using TokenConfig.IssuerMint and returns the created holding contract ID.
	Mint(ctx context.Context, req *MintRequest) (string, error)

	// Burn burns tokens using TokenConfig.IssuerBurn.
	Burn(ctx context.Context, req *BurnRequest) error

	// GetHoldings returns holdings for the owner and token symbol.
	// Delegates to GetHoldingsByParty using the Splice HoldingV1 interface.
	GetHoldings(ctx context.Context, ownerParty string, tokenSymbol string) ([]*Holding, error)

	// GetHoldingsByParty queries all Splice HoldingV1 holdings visible to the given party,
	// optionally filtered by instrumentID (empty string returns all instruments).
	// This is the unified query path for all Splice-compliant tokens (CIP-56 and external).
	GetHoldingsByParty(ctx context.Context, ownerParty, instrumentID string) ([]*Holding, error)

	// GetAllHoldings returns all CIP56Holding contracts queried as IssuerParty.
	// Used by the indexer and totalSupply — does NOT use the unified HoldingV1 path.
	GetAllHoldings(ctx context.Context) ([]*Holding, error) // TODO: use pagination

	// GetBalanceByFingerprint returns the owner's total balance (sum of holdings) for the token symbol.
	GetBalanceByFingerprint(ctx context.Context, fingerprint string, tokenSymbol string) (string, error)

	// GetBalanceByPartyID returns the owner's total balance (sum of holdings) for the token symbol.
	GetBalanceByPartyID(ctx context.Context, partyID string, tokenSymbol string) (string, error)

	// GetTotalSupply returns the total supply (sum across all holdings) for the token symbol.
	GetTotalSupply(ctx context.Context, tokenSymbol string) (string, error)

	// TransferByFingerprint transfers tokens by resolving fingerprints to parties.
	// idempotencyKey is used as the Canton CommandId for idempotent submission.
	TransferByFingerprint(ctx context.Context, idempotencyKey, fromFingerprint, toFingerprint, amount, tokenSymbol string) error

	// TransferByPartyID transfers tokens by party IDs using Interactive Submission.
	// idempotencyKey is used as the Canton CommandId for idempotent submission.
	// Requires a KeyResolver configured via WithKeyResolver (external/secp256k1 parties only).
	TransferByPartyID(ctx context.Context, idempotencyKey, fromParty, toParty, amount, tokenSymbol string) error

	// TransferInternalByPartyID transfers tokens for an internal Canton party using
	// regular command submission (SubmitAndWaitForTransaction). Unlike TransferByPartyID,
	// no KeyResolver is required — the Canton node holds the signing key. Use this for
	// issuer or bridge parties that are not registered with an external secp256k1 key.
	// idempotencyKey is used as the Canton CommandId for idempotent submission.
	TransferInternalByPartyID(ctx context.Context, idempotencyKey, fromParty, toParty, amount, tokenSymbol string) error

	// GetTokenTransferEvents returns all active CIP56.Events.TokenTransferEvent contracts visible to relayerParty.
	GetTokenTransferEvents(ctx context.Context) ([]*TokenTransferEvent, error)

	// GetTransferFactory returns the active CIP56TransferFactory contract ID and its
	// CreatedEventBlob for explicit contract disclosure by external wallets (Splice Registry API).
	GetTransferFactory(ctx context.Context) (*TransferFactoryInfo, error)

	// PrepareTransfer builds a Canton transaction for a non-custodial transfer and returns
	// the hash that the client must sign externally.
	PrepareTransfer(ctx context.Context, req *PrepareTransferRequest) (*PreparedTransfer, error)

	// ExecuteTransfer completes a previously prepared transfer using the client's DER signature.
	ExecuteTransfer(ctx context.Context, req *ExecuteTransferRequest) error

	// FindPendingInboundTransferInstructions returns contract IDs of active TransferOffer contracts
	// where partyID is an observer (i.e. the receiver). Used by the accept worker to find offers
	// to accept on behalf of custodial parties.
	FindPendingInboundTransferInstructions(ctx context.Context, partyID string) ([]string, error)

	// AcceptTransferInstruction accepts a pending inbound transfer for a custodial party.
	// Calls the registrar's accept choice-context endpoint (keyed by instrumentAdmin), encodes
	// the AnyValue choiceContext, and exercises TransferInstruction_Accept via SubmitAndWait.
	AcceptTransferInstruction(ctx context.Context, partyID, instructionCID, instrumentAdmin string) error

	// PrepareAcceptTransfer builds a Canton transaction for accepting a pending inbound
	// transfer instruction and returns the hash that the client must sign externally.
	// Use ExecuteTransfer to complete the accept once the client has signed.
	PrepareAcceptTransfer(
		ctx context.Context, partyID, instructionCID, instrumentAdmin string,
	) (*PreparedTransfer, error)
}

// Client implements CIP-56 token operations.
type Client struct {
	cfg            *Config
	ledger         ledger.Ledger
	identity       identity.Identity
	keyResolver    KeyResolver
	registryClient *RegistryClient
	logger         *zap.Logger
}

// New creates a new token client.
func New(cfg *Config, l ledger.Ledger, id identity.Identity, opts ...Option) (*Client, error) {
	err := cfg.validate()
	if err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	if l == nil {
		return nil, fmt.Errorf("nil ledger client")
	}
	if id == nil {
		return nil, fmt.Errorf("nil identity client")
	}

	s := applyOptions(opts)
	return &Client{
		cfg:            cfg,
		ledger:         l,
		identity:       id,
		keyResolver:    s.keyResolver,
		registryClient: s.registryClient,
		logger:         s.logger,
	}, nil
}

func (c *Client) GetTokenConfigCID(ctx context.Context, tokenSymbol string) (string, error) {
	end, err := c.ledger.GetLedgerEnd(ctx)
	if err != nil {
		return "", err
	}
	if end == 0 {
		return "", fmt.Errorf("ledger is empty, no contracts exist")
	}

	tid := &lapiv2.Identifier{
		PackageId:  c.cfg.CIP56PackageID,
		ModuleName: moduleConfig,
		EntityName: entityTokenConfig,
	}

	events, err := c.ledger.GetActiveContractsByTemplate(ctx, end, []string{c.cfg.IssuerParty}, tid)
	if err != nil {
		return "", fmt.Errorf("error getting contracts: %w", err)
	}

	for _, ce := range events {
		fields := values.RecordToMap(ce.CreateArguments)
		if values.MetaSymbol(fields["meta"]) == tokenSymbol {
			return ce.ContractId, nil
		}
	}

	return "", fmt.Errorf("no active TokenConfig found for symbol %s", tokenSymbol)
}

func (c *Client) Mint(ctx context.Context, req *MintRequest) (string, error) {
	err := req.validate()
	if err != nil {
		return "", fmt.Errorf("invalid request: %w", err)
	}

	cid := req.ConfigCID
	if cid == "" {
		cid, err = c.GetTokenConfigCID(ctx, req.TokenSymbol)
		if err != nil {
			return "", err
		}
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  c.cfg.CIP56PackageID,
					ModuleName: moduleConfig,
					EntityName: entityTokenConfig,
				},
				ContractId:     cid,
				Choice:         "IssuerMint",
				ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: encodeIssuerMintArgs(req)}},
			},
		},
	}

	resp, err := c.ledger.Command().SubmitAndWaitForTransaction(c.ledger.AuthContext(ctx), &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: c.cfg.DomainID,
			CommandId:      uuid.NewString(),
			UserId:         c.cfg.UserID,
			ActAs:          []string{c.cfg.IssuerParty},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return "", fmt.Errorf("mint tokens: %w", err)
	}

	if resp.Transaction == nil {
		return "", fmt.Errorf("mint tokens: missing transaction in response")
	}

	for _, e := range resp.Transaction.Events {
		created := e.GetCreated()
		if created == nil || created.TemplateId == nil {
			continue
		}
		if created.TemplateId.ModuleName == moduleToken && created.TemplateId.EntityName == entityHolding {
			return created.ContractId, nil
		}
	}

	return "", fmt.Errorf("CIP56Holding contract not found in mint response")
}

func (c *Client) Burn(ctx context.Context, req *BurnRequest) error {
	err := req.validate()
	if err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}

	configCID, err := c.GetTokenConfigCID(ctx, req.TokenSymbol)
	if err != nil {
		return err
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  c.cfg.CIP56PackageID,
					ModuleName: moduleConfig,
					EntityName: entityTokenConfig,
				},
				ContractId:     configCID,
				Choice:         "IssuerBurn",
				ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: encodeIssuerBurnArgs(req)}},
			},
		},
	}

	_, err = c.ledger.Command().SubmitAndWait(c.ledger.AuthContext(ctx), &lapiv2.SubmitAndWaitRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: c.cfg.DomainID,
			CommandId:      uuid.NewString(),
			UserId:         c.cfg.UserID,
			ActAs:          []string{c.cfg.IssuerParty},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return fmt.Errorf("burn tokens: %w", err)
	}

	return nil
}

func (c *Client) GetHoldings(ctx context.Context, ownerParty string, tokenSymbol string) ([]*Holding, error) {
	if ownerParty == "" {
		return nil, fmt.Errorf("owner party is required")
	}
	if tokenSymbol == "" {
		return nil, fmt.Errorf("token symbol is required")
	}
	return c.GetHoldingsByParty(ctx, ownerParty, tokenSymbol)
}

// GetHoldingsByParty queries all Splice HoldingV1 holdings visible to the given party.
// This is the unified query path for all Splice-compliant tokens (CIP-56 and external like USDCx).
// If instrumentID is non-empty, results are filtered to that instrument.
// TODO: add unit tests with mocked ledger client (happy path, filtering, errors).
func (c *Client) GetHoldingsByParty(ctx context.Context, ownerParty, instrumentID string) ([]*Holding, error) {
	if ownerParty == "" {
		return nil, fmt.Errorf("owner party is required")
	}

	end, err := c.ledger.GetLedgerEnd(ctx)
	if err != nil {
		return nil, err
	}
	if end == 0 {
		return []*Holding{}, nil
	}

	iid := &lapiv2.Identifier{
		PackageId:  c.cfg.SpliceHoldingPackageID,
		ModuleName: spliceHoldingModule,
		EntityName: spliceHoldingEntity,
	}

	// GetActiveContractsByInterface returns CreatedEvents with create_arguments populated
	// (Required field per Canton Ledger API v2 proto), so decodeHolding works identically
	// for both template-based and interface-based queries.
	events, err := c.ledger.GetActiveContractsByInterface(ctx, end, []string{ownerParty}, iid)
	if err != nil {
		return nil, fmt.Errorf("query holdings by party: %w", err)
	}

	out := make([]*Holding, 0, len(events))
	for _, ce := range events {
		h := decodeHolding(ce)
		if h.Owner != ownerParty {
			continue
		}
		if instrumentID != "" && h.InstrumentID != instrumentID {
			continue
		}
		out = append(out, h)
	}
	return out, nil
}

func (c *Client) GetAllHoldings(ctx context.Context) ([]*Holding, error) {
	end, err := c.ledger.GetLedgerEnd(ctx)
	if err != nil {
		return nil, err
	}
	if end == 0 {
		return []*Holding{}, nil
	}

	tid := &lapiv2.Identifier{
		PackageId:  c.cfg.CIP56PackageID,
		ModuleName: moduleToken,
		EntityName: entityHolding,
	}

	events, err := c.ledger.GetActiveContractsByTemplate(ctx, end, []string{c.cfg.IssuerParty}, tid)
	if err != nil {
		return nil, fmt.Errorf("query holdings: %w", err)
	}

	out := make([]*Holding, 0, len(events))
	for _, ce := range events {
		out = append(out, decodeHolding(ce))
	}
	return out, nil
}

func (c *Client) GetBalanceByFingerprint(ctx context.Context, fingerprint string, tokenSymbol string) (string, error) {
	m, err := c.identity.GetFingerprintMapping(ctx, fingerprint)
	if err != nil {
		return "0", err
	}
	return c.GetBalanceByPartyID(ctx, m.UserParty, tokenSymbol)
}

func (c *Client) GetBalanceByPartyID(ctx context.Context, partyID string, tokenSymbol string) (string, error) {
	holdings, err := c.GetHoldings(ctx, partyID, tokenSymbol)
	if err != nil {
		return "0", err
	}

	total := "0"
	for _, h := range holdings {
		next, err := addDecimalStrings(total, h.Amount)
		if err != nil {
			return "0", err
		}
		total = next
	}

	return total, nil
}

func (c *Client) GetTotalSupply(ctx context.Context, tokenSymbol string) (string, error) {
	if tokenSymbol == "" {
		return "0", fmt.Errorf("token symbol is required")
	}

	end, err := c.ledger.GetLedgerEnd(ctx)
	if err != nil {
		return "0", err
	}
	if end == 0 {
		return "0", nil
	}

	tid := &lapiv2.Identifier{
		PackageId:  c.cfg.CIP56PackageID,
		ModuleName: moduleToken,
		EntityName: entityHolding,
	}

	events, err := c.ledger.GetActiveContractsByTemplate(ctx, end, []string{c.cfg.IssuerParty}, tid)
	if err != nil {
		return "0", fmt.Errorf("query holdings: %w", err)
	}

	total := "0"
	for _, ce := range events {
		fields := values.RecordToMap(ce.CreateArguments)
		if values.MetaSymbol(fields["meta"]) != tokenSymbol {
			continue
		}

		next, err := addDecimalStrings(total, values.Numeric(fields["amount"]))
		if err != nil {
			return "0", err
		}
		total = next
	}

	return total, nil
}

func (c *Client) TransferInternalByPartyID(ctx context.Context, idempotencyKey, fromParty, toParty, amount, tokenSymbol string) error {
	if idempotencyKey == "" {
		return fmt.Errorf("idempotencyKey is required")
	}
	if fromParty == "" || toParty == "" {
		return fmt.Errorf("from/to party is required")
	}
	if amount == "" {
		return fmt.Errorf("amount is required")
	}
	if tokenSymbol == "" {
		return fmt.Errorf("token symbol is required")
	}

	holdings, err := c.GetHoldings(ctx, fromParty, tokenSymbol)
	if err != nil {
		return err
	}
	selected, err := selectHoldingsForTransfer(holdings, amount)
	if err != nil {
		return fmt.Errorf("select holdings for transfer: %w", err)
	}

	// Try CIP56TransferFactory first; if not found, fall back to AllocationFactory.
	// AllocationFactory (used by external tokens like USDCx) requires InstrumentConfiguration
	// and TransferRule CIDs in the choice context for TransferFactory_Transfer.
	var factoryCID string
	var anyValueCtx AcceptChoiceContext
	factoryInfo, factoryErr := c.GetTransferFactory(ctx)
	if factoryErr == nil {
		factoryCID = factoryInfo.ContractID
	} else if errors.Is(factoryErr, ErrTransferFactoryNotFound) {
		allocInfo, allocErr := c.GetAllocationFactory(ctx)
		if allocErr != nil {
			return allocErr
		}
		factoryCID = allocInfo.ContractID
		if c.cfg.UtilityRegistryPackageID != "" {
			anyValueCtx, err = c.getUtilityRegistryChoiceContext(ctx)
			if err != nil {
				return fmt.Errorf("get utility registry choice context: %w", err)
			}
		}
	} else {
		return factoryErr
	}

	req := &transferFactoryRequest{
		CommandID:        idempotencyKey,
		FromPartyID:      fromParty,
		ToPartyID:        toParty,
		Amount:           amount,
		InstrumentAdmin:  selected.InstrumentAdmin,
		InstrumentID:     selected.InstrumentID,
		InputHoldingCIDs: selected.CIDs,
		FactoryCID:       factoryCID,
		AnyValueContext:  anyValueCtx,
	}

	cmd, err := c.buildTransferCommand(req)
	if err != nil {
		return err
	}
	authCtx := c.ledger.AuthContext(ctx)
	_, err = c.ledger.Command().SubmitAndWaitForTransaction(authCtx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: c.cfg.DomainID,
			CommandId:      idempotencyKey,
			UserId:         c.cfg.UserID,
			ActAs:          []string{fromParty},
			ReadAs:         []string{c.cfg.IssuerParty},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	return err
}

func (c *Client) TransferByFingerprint(ctx context.Context, idempotencyKey, fromFingerprint,
	toFingerprint, amount, tokenSymbol string) error {
	fromMap, err := c.identity.GetFingerprintMapping(ctx, fromFingerprint)
	if err != nil {
		return fmt.Errorf("sender not found: %w", err)
	}
	toMap, err := c.identity.GetFingerprintMapping(ctx, toFingerprint)
	if err != nil {
		return fmt.Errorf("recipient not found: %w", err)
	}

	return c.TransferByPartyID(ctx, idempotencyKey, fromMap.UserParty, toMap.UserParty, amount, tokenSymbol)
}

func (c *Client) TransferByPartyID(ctx context.Context, idempotencyKey, fromParty, toParty, amount, tokenSymbol string) error {
	if idempotencyKey == "" {
		return fmt.Errorf("idempotencyKey is required")
	}
	if fromParty == "" || toParty == "" {
		return fmt.Errorf("from/to party is required")
	}
	if amount == "" {
		return fmt.Errorf("amount is required")
	}
	if tokenSymbol == "" {
		return fmt.Errorf("token symbol is required")
	}

	holdings, err := c.GetHoldings(ctx, fromParty, tokenSymbol)
	if err != nil {
		return err
	}
	selected, err := selectHoldingsForTransfer(holdings, amount)
	if err != nil {
		return fmt.Errorf("select holdings for transfer: %w", err)
	}

	req := &transferFactoryRequest{
		CommandID:        idempotencyKey,
		FromPartyID:      fromParty,
		ToPartyID:        toParty,
		Amount:           amount,
		InstrumentAdmin:  selected.InstrumentAdmin,
		InstrumentID:     selected.InstrumentID,
		InputHoldingCIDs: selected.CIDs,
	}

	if err := c.resolveTransferFactory(ctx, req); err != nil {
		return err
	}

	return c.transferViaFactory(ctx, req)
}

type transferFactoryRequest struct {
	CommandID        string
	FromPartyID      string
	ToPartyID        string
	Amount           string
	InstrumentAdmin  string
	InstrumentID     string
	InputHoldingCIDs []string
	FactoryCID       string
	ChoiceContext    map[string]string
	// AnyValueContext holds the choice context when values need AnyValue encoding
	// (required for AllocationFactory's TransferFactory_Transfer choice).
	AnyValueContext    AcceptChoiceContext
	DisclosedContracts []*lapiv2.DisclosedContract
	IsExternal         bool
}

func (c *Client) transferViaFactory(ctx context.Context, req *transferFactoryRequest) error {
	if c.keyResolver == nil {
		return fmt.Errorf("transfer failed: no key resolver configured (required for Interactive Submission)")
	}

	signerKey, err := c.keyResolver(req.FromPartyID)
	if err != nil {
		return fmt.Errorf("transfer failed: cannot resolve signing key for party %s: %w", req.FromPartyID, err)
	}

	cmd, err := c.buildTransferCommand(req)
	if err != nil {
		return err
	}

	readAs := []string{c.cfg.IssuerParty}
	if req.IsExternal {
		readAs = nil
	}

	commands := &lapiv2.Commands{
		SynchronizerId:     c.cfg.DomainID,
		CommandId:          req.CommandID,
		UserId:             c.cfg.UserID,
		ActAs:              []string{req.FromPartyID},
		ReadAs:             readAs,
		Commands:           []*lapiv2.Command{cmd},
		DisclosedContracts: req.DisclosedContracts,
	}

	return c.prepareAndExecuteAsUser(ctx, commands, signerKey, req.FromPartyID)
}

// buildTransferCommand creates the exercise command for a TransferFactory_Transfer.
// Shared between custodial (transferViaFactory) and non-custodial (PrepareTransfer) paths.
// Returns an error only when AnyValueContext encoding fails (invalid AnyValue tags).
func (c *Client) buildTransferCommand(req *transferFactoryRequest) (*lapiv2.Command, error) {
	// Backdate requestedAt by 5s to avoid `Transfer requestedAt must be in the past`
	// when the host clock is slightly ahead of the Canton ledger clock. The
	// AllocationFactory_TransferInternal choice asserts requestedAt < ledger time
	// (assertDeadlineExceeded), so a now() value that the ledger sees as future
	// fails interpretation.
	now := time.Now().UTC().Add(-5 * time.Second)

	// For AllocationFactory tokens the choice context must be encoded as TextMap AnyValue.
	// For CIP56TransferFactory tokens (empty context) the standard TextMap Text encoding is used.
	var extraArgsValue *lapiv2.Value
	if len(req.AnyValueContext.Values) > 0 {
		ctxValue, err := encodeChoiceContextRecord(req.AnyValueContext)
		if err != nil {
			return nil, fmt.Errorf("encode choice context: %w", err)
		}
		extraArgsValue = &lapiv2.Value{
			Sum: &lapiv2.Value_Record{
				Record: &lapiv2.Record{
					Fields: []*lapiv2.RecordField{
						{Label: "context", Value: ctxValue},
						{Label: "meta", Value: values.EmptyMetadata()},
					},
				},
			},
		}
	} else {
		extraArgsValue = values.EncodeExtraArgs(req.ChoiceContext)
	}

	return &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  c.cfg.SpliceTransferPackageID,
					ModuleName: spliceTransferModule,
					EntityName: spliceTransferFactory,
				},
				ContractId: req.FactoryCID,
				Choice:     "TransferFactory_Transfer",
				ChoiceArgument: &lapiv2.Value{
					Sum: &lapiv2.Value_Record{
						Record: encodeTransferFactoryTransferArgs(
							req.InstrumentAdmin,
							req.FromPartyID,
							req.ToPartyID,
							req.Amount,
							req.InstrumentAdmin,
							req.InstrumentID,
							now,
							now.Add(defaultTransferValidity),
							req.InputHoldingCIDs,
							extraArgsValue,
						),
					},
				},
			},
		},
	}, nil
}

func (c *Client) getTransferFactoryCID(ctx context.Context) (string, error) {
	info, err := c.GetTransferFactory(ctx)
	if err == nil {
		return info.ContractID, nil
	}
	if !errors.Is(err, ErrTransferFactoryNotFound) {
		return "", err
	}
	// Fallback: query via the Splice TransferFactory interface, which is
	// implemented by both CIP56TransferFactory and AllocationFactory. This
	// avoids hardcoding the concrete template's package ID.
	info, err = c.GetAllocationFactory(ctx)
	if err != nil {
		return "", err
	}
	return info.ContractID, nil
}

// GetAllocationFactory finds any active contract implementing the Splice
// TransferFactory interface. Used as a fallback when no CIP56TransferFactory
// exists (e.g. tokens using AllocationFactory from utility_registry_app_v0).
// Uses an interface filter keyed on SpliceTransferPackageID so the concrete
// template's package ID never needs to be known.
func (c *Client) GetAllocationFactory(ctx context.Context) (*TransferFactoryInfo, error) {
	ifaceID := &lapiv2.Identifier{
		PackageId:  c.cfg.SpliceTransferPackageID,
		ModuleName: spliceTransferModule,
		EntityName: spliceTransferFactory,
	}
	filter := &lapiv2.CumulativeFilter{
		IdentifierFilter: &lapiv2.CumulativeFilter_InterfaceFilter{
			InterfaceFilter: &lapiv2.InterfaceFilter{
				InterfaceId:             ifaceID,
				IncludeCreatedEventBlob: true,
			},
		},
	}
	events, err := c.queryFactoryContracts(ctx, filter, "TransferFactory interface")
	if err != nil {
		return nil, err
	}
	ev := events[0]
	return &TransferFactoryInfo{
		ContractID:       ev.ContractId,
		CreatedEventBlob: ev.CreatedEventBlob,
		TemplateID: TemplateIdentifier{
			PackageID:  ifaceID.PackageId,
			ModuleName: ifaceID.ModuleName,
			EntityName: ifaceID.EntityName,
		},
	}, nil
}

// jsonRawString encodes a Go string as a JSON string literal (e.g. `"foo"` → `"\"foo\""`).
// Used to build json.RawMessage values for AnyValue fields.
func jsonRawString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

// getUtilityRegistryChoiceContext queries the ACS for InstrumentConfiguration and
// TransferRule contract IDs and returns them as an AcceptChoiceContext with AV_ContractId
// values. Required when exercising TransferFactory_Transfer on an AllocationFactory.
//
// AllocationFactory's TransferFactory_Transfer choice requires four context entries:
// instrument-configuration, transfer-rule, sender-credentials, receiver-credentials.
// The credential entries are empty AV_List values for the local devstack (no credentials
// are issued); they must still be present or DAML interpretation fails with
// "Missing context entry for: utility.digitalasset.com/sender-credentials".
func (c *Client) getUtilityRegistryChoiceContext(ctx context.Context) (AcceptChoiceContext, error) {
	authCtx := c.ledger.AuthContext(ctx)
	end, err := c.ledger.GetLedgerEnd(authCtx)
	if err != nil {
		return AcceptChoiceContext{}, fmt.Errorf("get ledger end: %w", err)
	}

	configCID, err := c.queryContractCIDFromACS(authCtx, end, &lapiv2.Identifier{
		PackageId:  c.cfg.UtilityRegistryPackageID,
		ModuleName: "Utility.Registry.V0.Configuration.Instrument",
		EntityName: "InstrumentConfiguration",
	})
	if err != nil {
		return AcceptChoiceContext{}, fmt.Errorf("InstrumentConfiguration: %w", err)
	}

	ruleCID, err := c.queryContractCIDFromACS(authCtx, end, &lapiv2.Identifier{
		PackageId:  c.cfg.UtilityRegistryPackageID,
		ModuleName: "Utility.Registry.V0.Rule.Transfer",
		EntityName: "TransferRule",
	})
	if err != nil {
		return AcceptChoiceContext{}, fmt.Errorf("TransferRule: %w", err)
	}

	avCID := func(cid string) AnyValue {
		return AnyValue{Tag: "AV_ContractId", Value: jsonRawString(cid)}
	}
	emptyList := AnyValue{Tag: "AV_List", Value: json.RawMessage("[]")}
	return AcceptChoiceContext{
		Values: map[string]AnyValue{
			"utility.digitalasset.com/instrument-configuration": avCID(configCID),
			"utility.digitalasset.com/transfer-rule":            avCID(ruleCID),
			"utility.digitalasset.com/sender-credentials":       emptyList,
			"utility.digitalasset.com/receiver-credentials":     emptyList,
		},
	}, nil
}

// queryContractCIDFromACS fetches the first active contract matching tid from the ACS,
// using FiltersForAnyParty so it works regardless of which party hosts the contract.
func (c *Client) queryContractCIDFromACS(ctx context.Context, end int64, tid *lapiv2.Identifier) (string, error) {
	stream, err := c.ledger.State().GetActiveContracts(ctx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: end,
		EventFormat: &lapiv2.EventFormat{
			FiltersForAnyParty: &lapiv2.Filters{
				Cumulative: []*lapiv2.CumulativeFilter{{
					IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
						TemplateFilter: &lapiv2.TemplateFilter{
							TemplateId:              tid,
							IncludeCreatedEventBlob: false,
						},
					},
				}},
			},
			Verbose: false,
		},
	})
	if err != nil {
		return "", fmt.Errorf("get active contracts (%s): %w", tid.EntityName, err)
	}

	for {
		msg, recvErr := stream.Recv()
		if recvErr != nil {
			if errors.Is(recvErr, io.EOF) {
				break
			}
			return "", fmt.Errorf("recv (%s): %w", tid.EntityName, recvErr)
		}
		if ac := msg.GetActiveContract(); ac != nil && ac.CreatedEvent != nil {
			return ac.CreatedEvent.ContractId, nil
		}
	}
	return "", fmt.Errorf("%s not found on ACS", tid.EntityName)
}

// queryFactoryContracts streams active contracts for the issuer party matching
// the given filter. Returns ErrTransferFactoryNotFound when none are found.
// Shared by GetTransferFactory and GetAllocationFactory.
func (c *Client) queryFactoryContracts(ctx context.Context, filter *lapiv2.CumulativeFilter, label string) ([]*lapiv2.CreatedEvent, error) {
	end, err := c.ledger.GetLedgerEnd(ctx)
	if err != nil {
		return nil, err
	}
	if end == 0 {
		return nil, fmt.Errorf("ledger is empty, no contracts exist")
	}

	authCtx := c.ledger.AuthContext(ctx)
	filtersByParty := map[string]*lapiv2.Filters{
		c.cfg.IssuerParty: {
			Cumulative: []*lapiv2.CumulativeFilter{filter},
		},
	}

	stream, err := c.ledger.State().GetActiveContracts(authCtx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: end,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: filtersByParty,
			Verbose:        true,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", label, err)
	}

	var events []*lapiv2.CreatedEvent
	for {
		msg, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("receive %s contract: %w", label, err)
		}
		if ac := msg.GetActiveContract(); ac != nil && ac.CreatedEvent != nil {
			events = append(events, ac.CreatedEvent)
		}
	}

	if len(events) == 0 {
		return nil, ErrTransferFactoryNotFound
	}
	if len(events) > 1 {
		c.logger.Warn("multiple factory contracts found, using first",
			zap.String("filter", label),
			zap.Int("count", len(events)),
			zap.String("selected_cid", events[0].ContractId))
	}
	return events, nil
}

// resolveTransferFactory fills in factory info on the request by routing based on InstrumentAdmin.
// For our tokens (InstrumentAdmin == IssuerParty): uses local ACS query.
// For external tokens: calls the Transfer Factory Registry API.
func (c *Client) resolveTransferFactory(ctx context.Context, req *transferFactoryRequest) error {
	if req.InstrumentAdmin == c.cfg.IssuerParty {
		cid, err := c.getTransferFactoryCID(ctx)
		if err != nil {
			return err
		}
		req.FactoryCID = cid
		return nil
	}

	// External token — use registry
	if c.registryClient == nil {
		return fmt.Errorf("no registry client configured for external token transfers")
	}
	extCfg, ok := c.cfg.ExternalTokens[req.InstrumentAdmin]
	if !ok {
		return fmt.Errorf("unsupported external token issuer: %s", req.InstrumentAdmin)
	}

	regResp, err := c.registryClient.GetTransferFactory(ctx, extCfg.RegistryURL, &RegistryRequest{
		ExpectedAdmin: req.InstrumentAdmin,
		Transfer: RegistryTransferDetail{
			Sender:           req.FromPartyID,
			Receiver:         req.ToPartyID,
			Amount:           req.Amount,
			InstrumentID:     req.InstrumentID,
			InputHoldingCIDs: req.InputHoldingCIDs,
		},
	})
	if err != nil {
		return fmt.Errorf("registry lookup for %s: %w", req.InstrumentAdmin, err)
	}

	req.FactoryCID = regResp.FactoryID
	req.IsExternal = true

	// Try the AnyValue shape first (AllocationFactory-style registries). Fall back
	// to the legacy TextMap-of-Text shape only when no AnyValue context is present
	// — never mix the two on one request.
	anyCtx, err := ConvertAnyValueChoiceContext(regResp.ChoiceContext)
	if err != nil {
		return fmt.Errorf("convert anyvalue choice context: %w", err)
	}
	if len(anyCtx.Values) > 0 {
		req.AnyValueContext = anyCtx
	} else {
		req.ChoiceContext, err = ConvertChoiceContext(regResp.ChoiceContext)
		if err != nil {
			return fmt.Errorf("convert choice context: %w", err)
		}
	}

	req.DisclosedContracts, err = ConvertDisclosedContracts(regResp.DisclosedContracts, c.cfg.DomainID)
	if err != nil {
		return fmt.Errorf("convert disclosed contracts: %w", err)
	}

	return nil
}

func (c *Client) GetTransferFactory(ctx context.Context) (*TransferFactoryInfo, error) {
	tid := &lapiv2.Identifier{
		PackageId:  c.cfg.CIP56PackageID,
		ModuleName: moduleTransferFactory,
		EntityName: entityTransferFactory,
	}
	filter := &lapiv2.CumulativeFilter{
		IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
			TemplateFilter: &lapiv2.TemplateFilter{
				TemplateId:              tid,
				IncludeCreatedEventBlob: true,
			},
		},
	}
	events, err := c.queryFactoryContracts(ctx, filter, "CIP56TransferFactory")
	if err != nil {
		return nil, err
	}
	ev := events[0]
	return &TransferFactoryInfo{
		ContractID:       ev.ContractId,
		CreatedEventBlob: ev.CreatedEventBlob,
		TemplateID: TemplateIdentifier{
			PackageID:  tid.PackageId,
			ModuleName: tid.ModuleName,
			EntityName: tid.EntityName,
		},
	}, nil
}

// prepareAndExecuteAsUser uses the Interactive Submission API to submit a
// transaction on behalf of an external party. It prepares the transaction,
// signs the hash with the party's private key, and executes it.
func (c *Client) prepareAndExecuteAsUser(ctx context.Context, commands *lapiv2.Commands, signerKey Signer, partyID string) error {
	authCtx := c.ledger.AuthContext(ctx)

	prepResp, err := c.ledger.Interactive().PrepareSubmission(authCtx, &interactivev2.PrepareSubmissionRequest{
		UserId:             commands.UserId,
		CommandId:          commands.CommandId,
		Commands:           commands.Commands,
		ActAs:              commands.ActAs,
		ReadAs:             commands.ReadAs,
		SynchronizerId:     commands.SynchronizerId,
		DisclosedContracts: commands.DisclosedContracts,
	})
	if err != nil {
		return fmt.Errorf("prepare submission: %w", err)
	}

	derSig, err := signerKey.SignDER(prepResp.PreparedTransactionHash)
	if err != nil {
		return fmt.Errorf("sign prepared transaction: %w", err)
	}

	fingerprint, err := signerKey.Fingerprint()
	if err != nil {
		return fmt.Errorf("get signer fingerprint: %w", err)
	}

	partySigs := &interactivev2.PartySignatures{
		Signatures: []*interactivev2.SinglePartySignatures{
			{
				Party: partyID,
				Signatures: []*lapiv2.Signature{
					{
						Format:               lapiv2.SignatureFormat_SIGNATURE_FORMAT_DER,
						Signature:            derSig,
						SignedBy:             fingerprint,
						SigningAlgorithmSpec: lapiv2.SigningAlgorithmSpec_SIGNING_ALGORITHM_SPEC_EC_DSA_SHA_256,
					},
				},
			},
		},
	}

	_, err = c.ledger.Interactive().ExecuteSubmissionAndWait(authCtx, &interactivev2.ExecuteSubmissionAndWaitRequest{
		PreparedTransaction:  prepResp.PreparedTransaction,
		PartySignatures:      partySigs,
		SubmissionId:         uuid.NewString(),
		UserId:               commands.UserId,
		HashingSchemeVersion: prepResp.HashingSchemeVersion,
	})
	if err != nil {
		return fmt.Errorf("execute submission: %w", err)
	}

	return nil
}

type selectedHoldings struct {
	CIDs            []string
	InstrumentAdmin string
	InstrumentID    string
}

// selectHoldingsForTransfer selects holdings whose combined value covers the
// required transfer amount. With multi-input TransferFactory, fragmentation
// is no longer an issue -- we just accumulate holdings until we have enough.
func selectHoldingsForTransfer(holdings []*Holding, requiredAmount string) (*selectedHoldings, error) {
	if len(holdings) == 0 {
		return nil, fmt.Errorf("%w: no holdings found", ErrInsufficientBalance)
	}

	result := &selectedHoldings{}
	total := "0"
	for _, h := range holdings {
		if h.Locked {
			continue
		}
		if len(result.CIDs) == 0 {
			result.InstrumentAdmin = h.InstrumentAdmin
			result.InstrumentID = h.InstrumentID
		} else if h.InstrumentAdmin != result.InstrumentAdmin || h.InstrumentID != result.InstrumentID {
			continue
		}
		result.CIDs = append(result.CIDs, h.ContractID)
		next, err := addDecimalStrings(total, h.Amount)
		if err != nil {
			return nil, err
		}
		total = next

		cmp, err := compareDecimalStrings(total, requiredAmount)
		if err != nil {
			return nil, err
		}
		if cmp >= 0 {
			return result, nil
		}
	}

	return nil, fmt.Errorf("%w: total unlocked balance %s, need %s",
		ErrInsufficientBalance, total, requiredAmount)
}

func (c *Client) PrepareTransfer(ctx context.Context, req *PrepareTransferRequest) (*PreparedTransfer, error) {
	if err := req.validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	holdings, err := c.GetHoldings(ctx, req.FromPartyID, req.TokenSymbol)
	if err != nil {
		return nil, err
	}
	selected, err := selectHoldingsForTransfer(holdings, req.Amount)
	if err != nil {
		return nil, fmt.Errorf("select holdings for transfer: %w", err)
	}

	factoryReq := &transferFactoryRequest{
		FromPartyID:      req.FromPartyID,
		ToPartyID:        req.ToPartyID,
		Amount:           req.Amount,
		InstrumentAdmin:  selected.InstrumentAdmin,
		InstrumentID:     selected.InstrumentID,
		InputHoldingCIDs: selected.CIDs,
	}

	if resolveErr := c.resolveTransferFactory(ctx, factoryReq); resolveErr != nil {
		return nil, resolveErr
	}

	cmd, err := c.buildTransferCommand(factoryReq)
	if err != nil {
		return nil, err
	}

	readAs := []string{c.cfg.IssuerParty}
	if factoryReq.IsExternal {
		readAs = nil
	}

	commands := &lapiv2.Commands{
		SynchronizerId:     c.cfg.DomainID,
		CommandId:          uuid.NewString(),
		UserId:             c.cfg.UserID,
		ActAs:              []string{req.FromPartyID},
		ReadAs:             readAs,
		Commands:           []*lapiv2.Command{cmd},
		DisclosedContracts: factoryReq.DisclosedContracts,
	}

	authCtx := c.ledger.AuthContext(ctx)
	prepResp, err := c.ledger.Interactive().PrepareSubmission(authCtx, &interactivev2.PrepareSubmissionRequest{
		UserId:             commands.UserId,
		CommandId:          commands.CommandId,
		Commands:           commands.Commands,
		ActAs:              commands.ActAs,
		ReadAs:             commands.ReadAs,
		SynchronizerId:     commands.SynchronizerId,
		DisclosedContracts: commands.DisclosedContracts,
	})
	if err != nil {
		return nil, fmt.Errorf("prepare submission: %w", err)
	}

	pt := &PreparedTransfer{
		TransferID:           uuid.NewString(),
		TransactionHash:      prepResp.PreparedTransactionHash,
		PreparedTransaction:  prepResp.PreparedTransaction,
		HashingSchemeVersion: prepResp.HashingSchemeVersion,
		PartyID:              req.FromPartyID,
	}

	c.logger.Info("Prepared non-custodial transfer",
		zap.String("transfer_id", pt.TransferID),
		zap.String("from_party", req.FromPartyID),
		zap.String("to_party", req.ToPartyID),
		zap.String("amount", req.Amount),
		zap.String("token", req.TokenSymbol))

	return pt, nil
}

func (c *Client) ExecuteTransfer(ctx context.Context, req *ExecuteTransferRequest) error {
	if err := req.validate(); err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}

	pt := req.PreparedTransfer
	authCtx := c.ledger.AuthContext(ctx)

	partySigs := &interactivev2.PartySignatures{
		Signatures: []*interactivev2.SinglePartySignatures{
			{
				Party: pt.PartyID,
				Signatures: []*lapiv2.Signature{
					{
						Format:               lapiv2.SignatureFormat_SIGNATURE_FORMAT_DER,
						Signature:            req.Signature,
						SignedBy:             req.SignedBy,
						SigningAlgorithmSpec: lapiv2.SigningAlgorithmSpec_SIGNING_ALGORITHM_SPEC_EC_DSA_SHA_256,
					},
				},
			},
		},
	}

	_, err := c.ledger.Interactive().ExecuteSubmissionAndWait(authCtx, &interactivev2.ExecuteSubmissionAndWaitRequest{
		PreparedTransaction:  pt.PreparedTransaction,
		PartySignatures:      partySigs,
		SubmissionId:         uuid.NewString(),
		UserId:               c.cfg.UserID,
		HashingSchemeVersion: pt.HashingSchemeVersion,
	})
	if err != nil {
		return fmt.Errorf("execute submission: %w", err)
	}

	c.logger.Info("Executed non-custodial transfer",
		zap.String("transfer_id", pt.TransferID),
		zap.String("party", pt.PartyID))

	return nil
}

func (c *Client) FindPendingInboundTransferInstructions(ctx context.Context, partyID string) ([]string, error) {
	if partyID == "" {
		return nil, fmt.Errorf("partyID is required")
	}
	if c.cfg.UtilityRegistryAppPackageID == "" {
		return nil, fmt.Errorf("utility_registry_app_package_id not configured")
	}

	end, err := c.ledger.GetLedgerEnd(ctx)
	if err != nil {
		return nil, err
	}
	if end == 0 {
		return nil, nil
	}

	tid := &lapiv2.Identifier{
		PackageId:  c.cfg.UtilityRegistryAppPackageID,
		ModuleName: moduleTransferOffer,
		EntityName: entityTransferOffer,
	}

	events, err := c.ledger.GetActiveContractsByTemplate(ctx, end, []string{partyID}, tid)
	if err != nil {
		return nil, fmt.Errorf("query pending TransferOffers: %w", err)
	}

	out := make([]string, 0, len(events))
	for _, ce := range events {
		out = append(out, ce.ContractId)
	}
	return out, nil
}

func (c *Client) AcceptTransferInstruction(ctx context.Context, partyID, instructionCID, instrumentAdmin string) error {
	if partyID == "" || instructionCID == "" || instrumentAdmin == "" {
		return fmt.Errorf("partyID, instructionCID, and instrumentAdmin are required")
	}
	if c.registryClient == nil {
		return fmt.Errorf("no registry client configured for accept")
	}
	extCfg, ok := c.cfg.ExternalTokens[instrumentAdmin]
	if !ok {
		return fmt.Errorf("unsupported external token issuer: %s", instrumentAdmin)
	}

	acceptResp, err := c.registryClient.GetAcceptChoiceContext(ctx, extCfg.RegistryURL, instrumentAdmin, instructionCID)
	if err != nil {
		return fmt.Errorf("get accept choice context: %w", err)
	}

	ctxValue, err := encodeChoiceContextRecord(acceptResp.ChoiceContextData)
	if err != nil {
		return fmt.Errorf("encode choice context: %w", err)
	}

	extraArgs := &lapiv2.Value{
		Sum: &lapiv2.Value_Record{
			Record: &lapiv2.Record{
				Fields: []*lapiv2.RecordField{
					{Label: "context", Value: ctxValue},
					{Label: "meta", Value: values.EmptyMetadata()},
				},
			},
		},
	}

	disclosed, err := convertDisclosedContractSlice(acceptResp.DisclosedContracts, c.cfg.DomainID)
	if err != nil {
		return fmt.Errorf("convert disclosed contracts: %w", err)
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  c.cfg.SpliceTransferPackageID,
					ModuleName: spliceTransferModule,
					EntityName: transferInstrEntity,
				},
				ContractId: instructionCID,
				Choice:     acceptChoice,
				ChoiceArgument: &lapiv2.Value{
					Sum: &lapiv2.Value_Record{
						Record: &lapiv2.Record{
							Fields: []*lapiv2.RecordField{
								{Label: "extraArgs", Value: extraArgs},
							},
						},
					},
				},
			},
		},
	}

	// External parties (the only kind this api-server creates for custodial users)
	// can't be submitted as via plain SubmitAndWait — the participant doesn't hold
	// the signing key. Use Interactive Submission with the user's stored key.
	if c.keyResolver == nil {
		return fmt.Errorf("accept transfer instruction: no key resolver configured " +
			"(custodial accept requires the api-server to hold the user's signing key)")
	}
	signerKey, err := c.keyResolver(partyID)
	if err != nil {
		return fmt.Errorf("accept transfer instruction: resolve signing key for %s: %w", partyID, err)
	}

	commands := &lapiv2.Commands{
		SynchronizerId:     c.cfg.DomainID,
		CommandId:          uuid.NewString(),
		UserId:             c.cfg.UserID,
		ActAs:              []string{partyID},
		Commands:           []*lapiv2.Command{cmd},
		DisclosedContracts: disclosed,
	}
	if err := c.prepareAndExecuteAsUser(ctx, commands, signerKey, partyID); err != nil {
		return fmt.Errorf("accept transfer instruction: %w", err)
	}
	return nil
}

func (c *Client) PrepareAcceptTransfer(
	ctx context.Context, partyID, instructionCID, instrumentAdmin string,
) (*PreparedTransfer, error) {
	if partyID == "" || instructionCID == "" || instrumentAdmin == "" {
		return nil, fmt.Errorf("partyID, instructionCID, and instrumentAdmin are required")
	}
	if c.registryClient == nil {
		return nil, fmt.Errorf("no registry client configured for accept")
	}
	extCfg, ok := c.cfg.ExternalTokens[instrumentAdmin]
	if !ok {
		return nil, fmt.Errorf("unsupported external token issuer: %s", instrumentAdmin)
	}

	acceptResp, err := c.registryClient.GetAcceptChoiceContext(ctx, extCfg.RegistryURL, instrumentAdmin, instructionCID)
	if err != nil {
		return nil, fmt.Errorf("get accept choice context: %w", err)
	}

	ctxValue, err := encodeChoiceContextRecord(acceptResp.ChoiceContextData)
	if err != nil {
		return nil, fmt.Errorf("encode choice context: %w", err)
	}

	extraArgs := &lapiv2.Value{
		Sum: &lapiv2.Value_Record{
			Record: &lapiv2.Record{
				Fields: []*lapiv2.RecordField{
					{Label: "context", Value: ctxValue},
					{Label: "meta", Value: values.EmptyMetadata()},
				},
			},
		},
	}

	disclosed, err := convertDisclosedContractSlice(acceptResp.DisclosedContracts, c.cfg.DomainID)
	if err != nil {
		return nil, fmt.Errorf("convert disclosed contracts: %w", err)
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  c.cfg.SpliceTransferPackageID,
					ModuleName: spliceTransferModule,
					EntityName: transferInstrEntity,
				},
				ContractId: instructionCID,
				Choice:     acceptChoice,
				ChoiceArgument: &lapiv2.Value{
					Sum: &lapiv2.Value_Record{
						Record: &lapiv2.Record{
							Fields: []*lapiv2.RecordField{
								{Label: "extraArgs", Value: extraArgs},
							},
						},
					},
				},
			},
		},
	}

	authCtx := c.ledger.AuthContext(ctx)
	prepResp, err := c.ledger.Interactive().PrepareSubmission(authCtx, &interactivev2.PrepareSubmissionRequest{
		UserId:             c.cfg.UserID,
		CommandId:          uuid.NewString(),
		Commands:           []*lapiv2.Command{cmd},
		ActAs:              []string{partyID},
		SynchronizerId:     c.cfg.DomainID,
		DisclosedContracts: disclosed,
	})
	if err != nil {
		return nil, fmt.Errorf("prepare accept submission: %w", err)
	}

	pt := &PreparedTransfer{
		TransferID:           uuid.NewString(),
		TransactionHash:      prepResp.PreparedTransactionHash,
		PreparedTransaction:  prepResp.PreparedTransaction,
		HashingSchemeVersion: prepResp.HashingSchemeVersion,
		PartyID:              partyID,
		ExpiresAt:            time.Now().Add(defaultTransferValidity),
	}

	c.logger.Info("prepared non-custodial accept",
		zap.String("transfer_id", pt.TransferID),
		zap.String("party_id", partyID),
		zap.String("contract_id", instructionCID),
	)

	return pt, nil
}

func (c *Client) GetTokenTransferEvents(ctx context.Context) ([]*TokenTransferEvent, error) {
	return getEvents(
		ctx,
		c.ledger,
		c.cfg.CIP56PackageID,
		c.cfg.IssuerParty,
		"TokenTransferEvent",
		decodeTokenTransferEvent,
	)
}

func getEvents[T any](
	ctx context.Context,
	ldr ledger.Ledger,
	cip56PackageID string,
	relayerParty string,
	eventName string,
	decode func(*lapiv2.CreatedEvent) T,
) ([]T, error) {
	end, err := ldr.GetLedgerEnd(ctx)
	if err != nil {
		return nil, err
	}
	if end == 0 {
		return []T{}, nil
	}

	tid := &lapiv2.Identifier{
		PackageId:  cip56PackageID,
		ModuleName: moduleEvents,
		EntityName: eventName,
	}

	events, err := ldr.GetActiveContractsByTemplate(ctx, end, []string{relayerParty}, tid)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", eventName, err)
	}

	out := make([]T, 0, len(events))
	for _, ce := range events {
		out = append(out, decode(ce))
	}

	return out, nil
}
