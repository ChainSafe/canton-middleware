// Package token implements CIP-56 token operations such as mint, burn, transfer,
// and balance queries.
package token

import (
	"context"
	"errors"
	"fmt"

	"github.com/chainsafe/canton-middleware/pkg/canton-sdk/identity"
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton-sdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/canton-sdk/ledger"
	"github.com/chainsafe/canton-middleware/pkg/canton-sdk/values"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Sentinel errors for balance-related operations.
var (
	// ErrInsufficientBalance indicates the owner's total balance is less than the required amount.
	ErrInsufficientBalance = errors.New("insufficient balance")

	// ErrBalanceFragmented indicates the owner has sufficient total balance but it's split across
	// multiple holdings such that no single holding has enough for the transfer.
	ErrBalanceFragmented = errors.New("balance fragmented across multiple holdings: consolidation required")
)

// Token defines CIP-56 token operations.
type Token interface {
	// GetTokenConfigCID returns the active TokenConfig contract ID for the given token symbol.
	GetTokenConfigCID(ctx context.Context, tokenSymbol string) (string, error)

	// Mint mints tokens using TokenConfig.IssuerMint and returns the created holding contract ID.
	Mint(ctx context.Context, req MintRequest) (string, error)

	// Burn burns tokens using TokenConfig.IssuerBurn.
	Burn(ctx context.Context, req BurnRequest) error

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
	cfg      *Config
	ledger   ledger.Ledger
	identity identity.Identity
	logger   *zap.Logger
}

// New creates a new token client.
func New(cfg Config, l ledger.Ledger, id identity.Identity, opts ...Option) (*Client, error) {
	err := cfg.Validate()
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
		cfg:      &cfg,
		ledger:   l,
		identity: id,
		logger:   s.logger,
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
		ModuleName: "CIP56.Config",
		EntityName: "TokenConfig",
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

func (c *Client) Mint(ctx context.Context, req MintRequest) (string, error) {
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
					ModuleName: "CIP56.Config",
					EntityName: "TokenConfig",
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
		if created.TemplateId.ModuleName == "CIP56.Token" && created.TemplateId.EntityName == "CIP56Holding" {
			return created.ContractId, nil
		}
	}

	return "", fmt.Errorf("CIP56Holding contract not found in mint response")
}

func (c *Client) Burn(ctx context.Context, req BurnRequest) error {
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
					ModuleName: "CIP56.Config",
					EntityName: "TokenConfig",
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
	// TODO: check if it supports filtering on request

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
		ModuleName: "CIP56.Token",
		EntityName: "CIP56Holding",
	}

	events, err := c.ledger.GetActiveContractsByTemplate(ctx, end, []string{c.cfg.RelayerParty}, tid)
	if err != nil {
		return nil, fmt.Errorf("query holdings: %w", err)
	}

	out := make([]*Holding, 0)
	for _, ce := range events {
		fields := values.RecordToMap(ce.CreateArguments)
		out = append(out, &Holding{
			ContractID: ce.ContractId,
			Issuer:     values.Party(fields["issuer"]),
			Owner:      values.Party(fields["owner"]),
			Amount:     values.Numeric(fields["amount"]),
			Symbol:     values.MetaSymbol(fields["meta"]),
		})
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
		ModuleName: "CIP56.Token",
		EntityName: "CIP56Holding",
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

	holdingCID, err := c.findHoldingForTransfer(ctx, fromParty, amount, tokenSymbol)
	if err != nil {
		return err
	}

	recipientHolding, err := c.findRecipientHolding(ctx, toParty, tokenSymbol)
	if err != nil {
		return err
	}

	return c.transferHolding(ctx, transferAsUserRequest{
		FromPartyID:              fromParty,
		ToPartyID:                toParty,
		HoldingCID:               holdingCID,
		Amount:                   amount,
		TokenSymbol:              tokenSymbol,
		ExistingRecipientHolding: recipientHolding,
	})
}

type transferAsUserRequest struct {
	FromPartyID string
	ToPartyID   string
	HoldingCID  string
	Amount      string
	TokenSymbol string
	// Existing recipient CIP56Holding CID (for merge), empty if none
	ExistingRecipientHolding string
}

func (c *Client) transferHolding(ctx context.Context, req transferAsUserRequest) error {
	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  c.cfg.CIP56PackageID,
					ModuleName: "CIP56.Token",
					EntityName: "CIP56Holding",
				},
				ContractId: req.HoldingCID,
				Choice:     "Transfer",
				ChoiceArgument: &lapiv2.Value{
					Sum: &lapiv2.Value_Record{
						Record: encodeHoldingTransferArgs(req.ToPartyID, req.Amount, req.ExistingRecipientHolding),
					},
				},
			},
		},
	}

	_, err := c.ledger.Command().SubmitAndWait(c.ledger.AuthContext(ctx), &lapiv2.SubmitAndWaitRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: c.cfg.DomainID,
			CommandId:      uuid.NewString(),
			UserId:         c.cfg.UserID,
			ActAs:          []string{req.FromPartyID},
			ReadAs:         []string{c.cfg.RelayerParty},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return fmt.Errorf("transfer failed: %w", err)
	}

	return nil
}

func (c *Client) findHoldingForTransfer(ctx context.Context, ownerParty, requiredAmount, tokenSymbol string) (string, error) {
	holdings, err := c.GetHoldings(ctx, ownerParty, tokenSymbol)
	if err != nil {
		return "", err
	}
	if len(holdings) == 0 {
		return "", fmt.Errorf("%w: no %s holdings found", ErrInsufficientBalance, tokenSymbol)
	}

	total := "0"
	for _, h := range holdings {
		next, err := addDecimalStrings(total, h.Amount)
		if err != nil {
			return "", err
		}
		total = next

		cmp, err := compareDecimalStrings(h.Amount, requiredAmount)
		if err != nil {
			return "", err
		}
		if cmp >= 0 {
			return h.ContractID, nil
		}
	}

	cmpTotal, err := compareDecimalStrings(total, requiredAmount)
	if err != nil {
		return "", err
	}
	if cmpTotal >= 0 {
		return "", fmt.Errorf("%w: total %s balance %s across %d holdings, need %s in single holding",
			ErrBalanceFragmented, tokenSymbol, total, len(holdings), requiredAmount)
	}

	return "", fmt.Errorf("%w: total %s balance %s, need %s",
		ErrInsufficientBalance, tokenSymbol, total, requiredAmount)
}

func (c *Client) findRecipientHolding(ctx context.Context, recipientParty, tokenSymbol string) (string, error) {
	holdings, err := c.GetHoldings(ctx, recipientParty, tokenSymbol)
	if err != nil {
		return "", err
	}
	if len(holdings) == 0 {
		return "", nil
	}
	return holdings[0].ContractID, nil
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
