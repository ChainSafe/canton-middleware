// Package token implements CIP-56 token operations such as mint, burn, transfer,
// and balance queries.
package token

import (
	"context"
	"errors"
	"fmt"
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
)

// Token defines CIP-56 token operations.
type Token interface {
	// GetTokenConfigCID returns the active TokenConfig contract ID for the given token symbol.
	GetTokenConfigCID(ctx context.Context, tokenSymbol string) (string, error)

	// Mint mints tokens using TokenConfig.IssuerMint and returns the created holding contract ID.
	Mint(ctx context.Context, req *MintRequest) (string, error)

	// Burn burns tokens using TokenConfig.IssuerBurn.
	Burn(ctx context.Context, req *BurnRequest) error

	// GetHoldings returns all CIP56Holding contracts for the owner and token symbol.
	GetHoldings(ctx context.Context, ownerParty string, tokenSymbol string) ([]*Holding, error)

	// GetAllHoldings GetHoldings returns all CIP56Holding contracts.
	GetAllHoldings(ctx context.Context) ([]*Holding, error) // TODO: use pagination

	// GetBalanceByFingerprint returns the owner's total balance (sum of holdings) for the token symbol.
	GetBalanceByFingerprint(ctx context.Context, fingerprint string, tokenSymbol string) (string, error)

	// GetTotalSupply returns the total supply (sum across all holdings) for the token symbol.
	GetTotalSupply(ctx context.Context, tokenSymbol string) (string, error)

	// TransferByFingerprint transfers tokens by resolving fingerprints to parties.
	TransferByFingerprint(ctx context.Context, fromFingerprint, toFingerprint, amount, tokenSymbol string) error

	// TransferByPartyID transfers tokens by party IDs.
	TransferByPartyID(ctx context.Context, fromParty, toParty, amount, tokenSymbol string) error

	// GetMintEvents returns all active CIP56.Events.MintEvent contracts visible to relayerParty.
	GetMintEvents(ctx context.Context) ([]*MintEvent, error)

	// GetBurnEvents returns all active CIP56.Events.BurnEvent contracts visible to relayerParty.
	GetBurnEvents(ctx context.Context) ([]*BurnEvent, error)
}

// Client implements CIP-56 token operations.
type Client struct {
	cfg         *Config
	ledger      ledger.Ledger
	identity    identity.Identity
	keyResolver KeyResolver
	logger      *zap.Logger
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
		cfg:         cfg,
		ledger:      l,
		identity:    id,
		keyResolver: s.keyResolver,
		logger:      s.logger,
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

	events, err := c.ledger.GetActiveContractsByTemplate(ctx, end, []string{c.cfg.RelayerParty}, tid)
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
			ActAs:          []string{c.cfg.RelayerParty},
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
			ActAs:          []string{c.cfg.RelayerParty},
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

	allHoldings, err := c.GetAllHoldings(ctx)
	if err != nil {
		return nil, err
	}

	validHoldings := make([]*Holding, 0)
	for _, h := range allHoldings {
		if h.Owner != ownerParty || h.Symbol != tokenSymbol {
			continue
		}
		validHoldings = append(validHoldings, h)
	}

	return validHoldings, nil
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

	events, err := c.ledger.GetActiveContractsByTemplate(ctx, end, []string{c.cfg.RelayerParty}, tid)
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
	return c.getBalanceByPartyID(ctx, m.UserParty, tokenSymbol)
}

func (c *Client) getBalanceByPartyID(ctx context.Context, partyID string, tokenSymbol string) (string, error) {
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

	events, err := c.ledger.GetActiveContractsByTemplate(ctx, end, []string{c.cfg.RelayerParty}, tid)
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

func (c *Client) TransferByFingerprint(ctx context.Context, fromFingerprint, toFingerprint, amount, tokenSymbol string) error {
	fromMap, err := c.identity.GetFingerprintMapping(ctx, fromFingerprint)
	if err != nil {
		return fmt.Errorf("sender not found: %w", err)
	}
	toMap, err := c.identity.GetFingerprintMapping(ctx, toFingerprint)
	if err != nil {
		return fmt.Errorf("recipient not found: %w", err)
	}

	return c.TransferByPartyID(ctx, fromMap.UserParty, toMap.UserParty, amount, tokenSymbol)
}

func (c *Client) TransferByPartyID(ctx context.Context, fromParty, toParty, amount, tokenSymbol string) error {
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

	factoryCID, err := c.getTransferFactoryCID(ctx)
	if err != nil {
		return err
	}

	return c.transferViaFactory(ctx, &transferFactoryRequest{
		FromPartyID:      fromParty,
		ToPartyID:        toParty,
		Amount:           amount,
		InstrumentAdmin:  selected.InstrumentAdmin,
		InstrumentID:     selected.InstrumentID,
		InputHoldingCIDs: selected.CIDs,
		FactoryCID:       factoryCID,
	})
}

type transferFactoryRequest struct {
	FromPartyID      string
	ToPartyID        string
	Amount           string
	InstrumentAdmin  string
	InstrumentID     string
	InputHoldingCIDs []string
	FactoryCID       string
}

func (c *Client) transferViaFactory(ctx context.Context, req *transferFactoryRequest) error {
	if c.keyResolver == nil {
		return fmt.Errorf("transfer failed: no key resolver configured (required for Interactive Submission)")
	}

	signerKey, err := c.keyResolver(req.FromPartyID)
	if err != nil {
		return fmt.Errorf("transfer failed: cannot resolve signing key for party %s: %w", req.FromPartyID, err)
	}

	now := time.Now().UTC()

	cmd := &lapiv2.Command{
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
						),
					},
				},
			},
		},
	}

	commands := &lapiv2.Commands{
		SynchronizerId: c.cfg.DomainID,
		CommandId:      uuid.NewString(),
		UserId:         c.cfg.UserID,
		ActAs:          []string{req.FromPartyID},
		ReadAs:         []string{c.cfg.RelayerParty},
		Commands:       []*lapiv2.Command{cmd},
	}

	return c.prepareAndExecuteAsUser(ctx, commands, signerKey, req.FromPartyID)
}

func (c *Client) getTransferFactoryCID(ctx context.Context) (string, error) {
	end, err := c.ledger.GetLedgerEnd(ctx)
	if err != nil {
		return "", err
	}
	if end == 0 {
		return "", fmt.Errorf("ledger is empty, no contracts exist")
	}

	tid := &lapiv2.Identifier{
		PackageId:  c.cfg.CIP56PackageID,
		ModuleName: moduleTransferFactory,
		EntityName: entityTransferFactory,
	}

	events, err := c.ledger.GetActiveContractsByTemplate(ctx, end, []string{c.cfg.RelayerParty}, tid)
	if err != nil {
		return "", fmt.Errorf("query transfer factory: %w", err)
	}
	if len(events) == 0 {
		return "", fmt.Errorf("no active CIP56TransferFactory found")
	}
	if len(events) > 1 {
		c.logger.Warn("multiple CIP56TransferFactory contracts found, using first",
			zap.Int("count", len(events)),
			zap.String("selected_cid", events[0].ContractId))
	}

	return events[0].ContractId, nil
}

// prepareAndExecuteAsUser uses the Interactive Submission API to submit a
// transaction on behalf of an external party. It prepares the transaction,
// signs the hash with the party's private key, and executes it.
func (c *Client) prepareAndExecuteAsUser(ctx context.Context, commands *lapiv2.Commands, signerKey Signer, partyID string) error {
	authCtx := c.ledger.AuthContext(ctx)

	prepResp, err := c.ledger.Interactive().PrepareSubmission(authCtx, &interactivev2.PrepareSubmissionRequest{
		UserId:         commands.UserId,
		CommandId:      commands.CommandId,
		Commands:       commands.Commands,
		ActAs:          commands.ActAs,
		ReadAs:         commands.ReadAs,
		SynchronizerId: commands.SynchronizerId,
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

func (c *Client) GetMintEvents(ctx context.Context) ([]*MintEvent, error) {
	return getEvents(
		ctx,
		c.ledger,
		c.cfg.CIP56PackageID,
		c.cfg.RelayerParty,
		"MintEvent",
		decodeMintEvent,
	)
}

func (c *Client) GetBurnEvents(ctx context.Context) ([]*BurnEvent, error) {
	return getEvents(
		ctx,
		c.ledger,
		c.cfg.CIP56PackageID,
		c.cfg.RelayerParty,
		"BurnEvent",
		decodeBurnEvent,
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
