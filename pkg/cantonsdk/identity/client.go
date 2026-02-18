// Package identity implements Canton identity operations such as party management
// and fingerprint-to-party mapping.
package identity

import (
	"context"
	"fmt"
	"strings"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	adminv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2/admin"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/ledger"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const listKnownPartiesPageSize = 1000

// ExternalPartyKey holds the public key and signing capability needed to
// allocate an external party. Implemented by keys.CantonKeyPair.
type ExternalPartyKey interface {
	SignHashDER(hash []byte) ([]byte, error)
}

// Identity defines identity and party management operations.
type Identity interface {
	AllocateParty(ctx context.Context, hint string) (*Party, error)
	AllocateExternalParty(ctx context.Context, hint string, compressedPubKey []byte, signer ExternalPartyKey) (*Party, error)
	ListParties(ctx context.Context) ([]*Party, error) // TODO: add iterator
	GetParticipantID(ctx context.Context) (string, error)

	CreateFingerprintMapping(ctx context.Context, req CreateFingerprintMappingRequest) (*FingerprintMapping, error)
	GetFingerprintMapping(ctx context.Context, fingerprint string) (*FingerprintMapping, error)

	GrantActAsParty(ctx context.Context, partyID string) error
}

// Client implements the Identity interface.
type Client struct {
	cfg    *Config
	ledger ledger.Ledger
	logger *zap.Logger
}

// New creates a new identity client.
func New(cfg *Config, l ledger.Ledger, opts ...Option) (*Client, error) {
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}
	if l == nil {
		return nil, fmt.Errorf("nil ledger client")
	}
	s := applyOptions(opts)
	return &Client{cfg: cfg, ledger: l, logger: s.logger}, nil
}

func (c *Client) AllocateParty(ctx context.Context, hint string) (*Party, error) {
	authCtx := c.ledger.AuthContext(ctx)

	req := &adminv2.AllocatePartyRequest{
		PartyIdHint:    hint,
		SynchronizerId: c.cfg.DomainID,
	}

	resp, err := c.ledger.PartyAdmin().AllocateParty(authCtx, req)
	if err != nil {
		return nil, fmt.Errorf("error allocating party: %w", err)
	}
	if resp.PartyDetails == nil {
		return nil, fmt.Errorf("allocate party returned nil party details")
	}

	return &Party{
		PartyID: resp.PartyDetails.Party,
		IsLocal: resp.PartyDetails.IsLocal,
	}, nil
}

// AllocateExternalParty creates an external party using the Interactive Submission flow.
// External parties have no practical limit (unlike internal parties which are capped at ~200).
// compressedPubKey is the 33-byte compressed secp256k1 public key.
// signer provides the SignHashDER capability for signing the topology multi-hash.
func (c *Client) AllocateExternalParty(ctx context.Context, hint string, compressedPubKey []byte, signer ExternalPartyKey) (*Party, error) {
	authCtx := c.ledger.AuthContext(ctx)

	pubKey := &lapiv2.SigningPublicKey{
		Format:  lapiv2.CryptoKeyFormat_CRYPTO_KEY_FORMAT_RAW,
		KeyData: compressedPubKey,
		KeySpec: lapiv2.SigningKeySpec_SIGNING_KEY_SPEC_EC_SECP256K1,
	}

	topoResp, err := c.ledger.PartyAdmin().GenerateExternalPartyTopology(authCtx, &adminv2.GenerateExternalPartyTopologyRequest{
		Synchronizer: c.cfg.DomainID,
		PartyHint:    hint,
		PublicKey:    pubKey,
	})
	if err != nil {
		return nil, fmt.Errorf("generate external party topology: %w", err)
	}

	derSig, err := signer.SignHashDER(topoResp.MultiHash)
	if err != nil {
		return nil, fmt.Errorf("sign topology multi-hash: %w", err)
	}

	multiHashSig := &lapiv2.Signature{
		Format:              lapiv2.SignatureFormat_SIGNATURE_FORMAT_DER,
		Signature:           derSig,
		SignedBy:            topoResp.PublicKeyFingerprint,
		SigningAlgorithmSpec: lapiv2.SigningAlgorithmSpec_SIGNING_ALGORITHM_SPEC_EC_DSA_SHA_256,
	}

	signedTxs := make([]*adminv2.AllocateExternalPartyRequest_SignedTransaction, len(topoResp.TopologyTransactions))
	for i, tx := range topoResp.TopologyTransactions {
		signedTxs[i] = &adminv2.AllocateExternalPartyRequest_SignedTransaction{
			Transaction: tx,
		}
	}

	allocResp, err := c.ledger.PartyAdmin().AllocateExternalParty(authCtx, &adminv2.AllocateExternalPartyRequest{
		Synchronizer:           c.cfg.DomainID,
		OnboardingTransactions: signedTxs,
		MultiHashSignatures:    []*lapiv2.Signature{multiHashSig},
	})
	if err != nil {
		return nil, fmt.Errorf("allocate external party: %w", err)
	}

	c.logger.Info("Allocated external party",
		zap.String("party_id", allocResp.PartyId),
		zap.String("hint", hint),
		zap.String("key_fingerprint", topoResp.PublicKeyFingerprint))

	return &Party{
		PartyID: allocResp.PartyId,
		IsLocal: false,
	}, nil
}

func (c *Client) ListParties(ctx context.Context) ([]*Party, error) {
	authCtx := c.ledger.AuthContext(ctx)

	var out []*Party
	pageToken := ""

	for {
		resp, err := c.ledger.PartyAdmin().ListKnownParties(authCtx, &adminv2.ListKnownPartiesRequest{
			PageSize:  listKnownPartiesPageSize,
			PageToken: pageToken,
		})
		if err != nil {
			return nil, fmt.Errorf("error listing parties: %w", err)
		}

		for _, p := range resp.PartyDetails {
			out = append(out, &Party{PartyID: p.Party, IsLocal: p.IsLocal})
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
		return "", fmt.Errorf("error getting participant id: %w", err)
	}

	return resp.ParticipantId, nil
}

func (c *Client) CreateFingerprintMapping(ctx context.Context, req CreateFingerprintMappingRequest) (*FingerprintMapping, error) {
	if err := req.validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	authCtx := c.ledger.AuthContext(ctx)
	packageID := c.cfg.GetPackageID()
	module := "Common.FingerprintAuth"
	entity := "FingerprintMapping"

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Create{
			Create: &lapiv2.CreateCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  packageID,
					ModuleName: module,
					EntityName: entity,
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
			CommandId:      uuid.NewString(),
			UserId:         c.cfg.UserID,
			ActAs:          []string{c.cfg.RelayerParty},
			ReadAs:         []string{req.UserParty},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error creating fingerprint mapping: %w", err)
	}

	if resp.Transaction != nil {
		for _, e := range resp.Transaction.Events {
			if created := e.GetCreated(); created != nil {
				if created.TemplateId.ModuleName == module && created.TemplateId.EntityName == entity {
					return fingerprintMappingFromCreateEvent(created), nil
				}
			}
		}
	}

	return nil, fmt.Errorf("fingerprint mapping contract not found in response")
}

func (c *Client) GetFingerprintMapping(ctx context.Context, fingerprint string) (*FingerprintMapping, error) {
	fp := normalizeFingerprint(fingerprint)

	end, err := c.ledger.GetLedgerEnd(ctx)
	if err != nil {
		return nil, err
	}
	if end == 0 {
		return nil, fmt.Errorf("ledger is empty, no contracts exist")
	}

	tid := &lapiv2.Identifier{
		PackageId:  c.cfg.GetPackageID(),
		ModuleName: "Common.FingerprintAuth",
		EntityName: "FingerprintMapping",
	}

	events, err := c.ledger.GetActiveContractsByTemplate(ctx, end, []string{c.cfg.RelayerParty}, tid)
	if err != nil {
		return nil, err
	}

	for _, ce := range events {
		m := fingerprintMappingFromCreateEvent(ce)
		if m != nil && m.Fingerprint == fp {
			return m, nil
		}
	}

	return nil, fmt.Errorf("no FingerprintMapping found for fingerprint: %s", fp)
}

func fingerprintMappingFromCreateEvent(event *lapiv2.CreatedEvent) *FingerprintMapping {
	fields := values.RecordToMap(event.CreateArguments)
	mfp := normalizeFingerprint(values.Text(fields["fingerprint"]))

	return &FingerprintMapping{
		ContractID:  event.ContractId,
		Issuer:      values.Party(fields["issuer"]),
		UserParty:   values.Party(fields["userParty"]),
		Fingerprint: mfp,
		EvmAddress:  values.Text(fields["evmAddress"]),
	}
}

func (c *Client) GrantActAsParty(ctx context.Context, partyID string) error {
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
		if isAlreadyExistsError(err) { // TODO: need to verify this works
			return nil
		}
		return fmt.Errorf("grant can act as: %w", err)
	}

	return nil
}

func isAlreadyExistsError(err error) bool {
	if s, ok := status.FromError(err); ok {
		return s.Code() == codes.AlreadyExists
	}

	return false
}

func normalizeFingerprint(fingerprint string) string {
	if !strings.HasPrefix(fingerprint, "0x") {
		fingerprint = "0x" + fingerprint
	}
	return fingerprint
}
