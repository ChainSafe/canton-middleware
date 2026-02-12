// Package identity implements Canton identity operations such as party management
// and fingerprint-to-party mapping.
package identity

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/chainsafe/canton-middleware/pkg/canton-sdk/ledger"
	"github.com/chainsafe/canton-middleware/pkg/canton-sdk/values"
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
	adminv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2/admin"
	"go.uber.org/zap"
)

// Identity defines identity and party management operations.
type Identity interface {
	AllocateParty(ctx context.Context, hint string) (*AllocatePartyResult, error)
	ListParties(ctx context.Context) ([]*AllocatePartyResult, error)
	GetParticipantID(ctx context.Context) (string, error)

	CreateFingerprintMapping(ctx context.Context, req CreateFingerprintMappingRequest) (string, error)
	GetFingerprintMapping(ctx context.Context, fingerprint string) (*FingerprintMapping, error)

	GrantCanActAs(ctx context.Context, partyID string) error
}

type settings struct {
	logger *zap.Logger
}

// Option configures the identity client.
type Option func(*settings)

// WithLogger sets a custom logger for the identity client.
func WithLogger(l *zap.Logger) Option {
	return func(s *settings) { s.logger = l }
}

func applyOptions(opts []Option) settings {
	s := settings{logger: zap.NewNop()}
	for _, opt := range opts {
		if opt != nil {
			opt(&s)
		}
	}
	return s
}

// Client implements the Identity interface.
type Client struct {
	cfg    Config
	ledger ledger.Ledger
	logger *zap.Logger
}

// New creates a new identity client.
func New(cfg Config, l ledger.Ledger, opts ...Option) (*Client, error) {
	if l == nil {
		return nil, fmt.Errorf("nil ledger client")
	}
	s := applyOptions(opts)
	return &Client{cfg: cfg, ledger: l, logger: s.logger}, nil
}

func (c *Client) AllocateParty(ctx context.Context, hint string) (*AllocatePartyResult, error) {
	authCtx := c.ledger.AuthContext(ctx)

	req := &adminv2.AllocatePartyRequest{
		PartyIdHint:    hint,
		SynchronizerId: c.cfg.DomainID,
	}

	resp, err := c.ledger.PartyAdmin().AllocateParty(authCtx, req)
	if err != nil {
		return nil, fmt.Errorf("allocate party: %w", err)
	}
	if resp.PartyDetails == nil {
		return nil, fmt.Errorf("allocate party returned nil party details")
	}

	return &AllocatePartyResult{
		PartyID: resp.PartyDetails.Party,
		IsLocal: resp.PartyDetails.IsLocal,
	}, nil
}

func (c *Client) ListParties(ctx context.Context) ([]*AllocatePartyResult, error) {
	authCtx := c.ledger.AuthContext(ctx)

	var out []*AllocatePartyResult
	pageToken := ""

	for {
		resp, err := c.ledger.PartyAdmin().ListKnownParties(authCtx, &adminv2.ListKnownPartiesRequest{
			PageSize:  1000,
			PageToken: pageToken,
		})
		if err != nil {
			return nil, fmt.Errorf("list parties: %w", err)
		}

		for _, p := range resp.PartyDetails {
			out = append(out, &AllocatePartyResult{PartyID: p.Party, IsLocal: p.IsLocal})
		}

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	return out, nil
}

func (c *Client) GetParticipantID(ctx context.Context) (string, error) {
	authCtx := c.ledger.AuthContext(ctx)

	resp, err := c.ledger.PartyAdmin().GetParticipantId(authCtx, &adminv2.GetParticipantIdRequest{})
	if err != nil {
		return "", fmt.Errorf("get participant id: %w", err)
	}

	return resp.ParticipantId, nil
}

func (c *Client) CreateFingerprintMapping(ctx context.Context, req CreateFingerprintMappingRequest) (string, error) {
	if c.cfg.UserID == "" {
		return "", fmt.Errorf("user id is required for command submission")
	}

	packageID := c.cfg.CommonPackageID
	if packageID == "" {
		packageID = c.cfg.BridgePackageID
	}
	if packageID == "" {
		return "", fmt.Errorf("package id is required for FingerprintMapping")
	}

	authCtx := c.ledger.AuthContext(ctx)

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Create{
			Create: &lapiv2.CreateCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  packageID,
					ModuleName: "Common.FingerprintAuth",
					EntityName: "FingerprintMapping",
				},
				CreateArguments: encodeFingerprintMappingCreate(
					c.cfg.RelayerParty,
					req.UserParty,
					req.Fingerprint,
					req.EvmAddress,
				),
			},
		},
	}

	resp, err := c.ledger.Command().SubmitAndWaitForTransaction(authCtx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: c.cfg.DomainID,
			CommandId:      values.UUID(),
			UserId:         c.cfg.UserID,
			ActAs:          []string{c.cfg.RelayerParty},
			ReadAs:         []string{req.UserParty},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return "", fmt.Errorf("create fingerprint mapping: %w", err)
	}

	if resp.Transaction != nil {
		for _, e := range resp.Transaction.Events {
			if created := e.GetCreated(); created != nil {
				if created.TemplateId.ModuleName == "Common.FingerprintAuth" && created.TemplateId.EntityName == "FingerprintMapping" {
					return created.ContractId, nil
				}
			}
		}
	}

	return "", fmt.Errorf("fingerprint mapping contract not found in response")
}

func (c *Client) GetFingerprintMapping(ctx context.Context, fingerprint string) (*FingerprintMapping, error) {
	fp := fingerprint
	if !strings.HasPrefix(fp, "0x") {
		fp = "0x" + fp
	}

	authCtx := c.ledger.AuthContext(ctx)

	end, err := c.ledger.GetLedgerEnd(authCtx)
	if err != nil {
		return nil, err
	}
	if end == 0 {
		return nil, fmt.Errorf("ledger is empty, no contracts exist")
	}

	var filter *lapiv2.CumulativeFilter
	if c.cfg.CommonPackageID != "" {
		filter = &lapiv2.CumulativeFilter{
			IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
				TemplateFilter: &lapiv2.TemplateFilter{
					TemplateId: &lapiv2.Identifier{
						PackageId:  c.cfg.CommonPackageID,
						ModuleName: "Common.FingerprintAuth",
						EntityName: "FingerprintMapping",
					},
				},
			},
		}
	} else {
		filter = &lapiv2.CumulativeFilter{
			IdentifierFilter: &lapiv2.CumulativeFilter_WildcardFilter{
				WildcardFilter: &lapiv2.WildcardFilter{},
			},
		}
	}

	stream, err := c.ledger.State().GetActiveContracts(authCtx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: end,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				c.cfg.RelayerParty: {Cumulative: []*lapiv2.CumulativeFilter{filter}},
			},
			Verbose: true,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query fingerprint mappings: %w", err)
	}

	for {
		msg, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			break
		}

		ac := msg.GetActiveContract()
		if ac == nil || ac.CreatedEvent == nil {
			continue
		}

		tid := ac.CreatedEvent.TemplateId
		if tid.ModuleName != "Common.FingerprintAuth" || tid.EntityName != "FingerprintMapping" {
			continue
		}

		fields := values.RecordToMap(ac.CreatedEvent.CreateArguments)

		mfp := values.Text(fields["fingerprint"])
		if !strings.HasPrefix(mfp, "0x") {
			mfp = "0x" + mfp
		}
		if mfp != fp {
			continue
		}

		return &FingerprintMapping{
			ContractID:  ac.CreatedEvent.ContractId,
			Issuer:      values.Party(fields["issuer"]),
			UserParty:   values.Party(fields["userParty"]),
			Fingerprint: mfp,
			EvmAddress:  values.Text(fields["evmAddress"]),
		}, nil
	}

	return nil, fmt.Errorf("no FingerprintMapping found for fingerprint: %s", fp)
}

func (c *Client) GrantCanActAs(ctx context.Context, partyID string) error {
	if c.cfg.UserID == "" {
		return fmt.Errorf("user id is required for rights management")
	}

	authCtx := c.ledger.AuthContext(ctx)

	right := &adminv2.Right{
		Kind: &adminv2.Right_CanActAs_{
			CanActAs: &adminv2.Right_CanActAs{Party: partyID},
		},
	}

	_, err := c.ledger.UserAdmin().GrantUserRights(authCtx, &adminv2.GrantUserRightsRequest{
		UserId: c.cfg.UserID,
		Rights: []*adminv2.Right{right},
	})
	if err != nil {
		if strings.Contains(err.Error(), "already") {
			return nil
		}
		return fmt.Errorf("grant can act as: %w", err)
	}

	return nil
}
