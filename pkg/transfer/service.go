package transfer

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/pkg/indexer"
	pkgtoken "github.com/chainsafe/canton-middleware/pkg/token"
	"github.com/chainsafe/canton-middleware/pkg/user"
)

//go:generate mockery --name UserStore --output mocks --outpkg mocks --filename mock_user_store.go --with-expecter
//go:generate mockery --name TransferCache --output mocks --outpkg mocks --filename mock_transfer_cache.go --with-expecter
//go:generate mockery --name PendingOfferLister --output mocks --outpkg mocks --filename mock_pending_offer_lister.go --with-expecter
//go:generate mockery --srcpkg github.com/chainsafe/canton-middleware/pkg/cantonsdk/token --name Token --output mocks --outpkg mocks --filename mock_canton_token.go --with-expecter

// UserStore is the narrow interface for looking up users.
type UserStore interface {
	GetUserByEVMAddress(ctx context.Context, evmAddress string) (*user.User, error)
}

// TransferCache is the interface for caching prepared transfers.
type TransferCache interface {
	Put(transfer *token.PreparedTransfer) error
	GetAndDelete(transferID string) (*token.PreparedTransfer, error)
}

// PendingOfferLister is the narrow slice of indexer/client.Client used by
// ListIncoming. The transfer service treats the indexer as the source of truth
// for pending TransferOffer state instead of querying Canton directly — the
// indexer already maintains `indexer_pending_offers` with all the fields we
// need (sender, amount, instrument), so going through it avoids duplicate
// decode logic in cantonsdk/token.
type PendingOfferLister interface {
	GetPendingOffersForParty(
		ctx context.Context, partyID string, p indexer.Pagination,
	) (*indexer.Page[indexer.PendingOffer], error)
}

// Service is the interface for the non-custodial prepare/execute transfer flow.
type Service interface {
	Prepare(ctx context.Context, senderEVMAddr string, req *PrepareRequest) (*PrepareResponse, error)
	Execute(ctx context.Context, senderEVMAddr string, req *ExecuteRequest) (*ExecuteResponse, error)

	// ListIncoming returns one page of pending inbound TransferOffer details for the
	// user with the given EVM address. This call is unauthenticated — anyone can
	// query any address's pending offers; the response is intentionally minimized
	// (party IDs truncated) to keep that from leaking counterparties.
	ListIncoming(ctx context.Context, evmAddr string, p indexer.Pagination) (*IncomingTransfersList, error)
	// PrepareAccept builds a Canton transaction for accepting an inbound offer.
	PrepareAccept(
		ctx context.Context, evmAddr, contractID string, req *PrepareAcceptRequest,
	) (*PrepareResponse, error)
	// ExecuteAccept completes a previously prepared accept using the client's DER signature.
	ExecuteAccept(ctx context.Context, evmAddr string, req *ExecuteRequest) (*ExecuteResponse, error)
}

// TransferService implements the non-custodial prepare/execute transfer flow.
type TransferService struct {
	cantonToken         token.Token
	userStore           UserStore
	cache               TransferCache
	offerLister         PendingOfferLister
	allowedTokenSymbols map[string]bool
	tokensByInstrument  map[instrumentKey]instrumentMeta
}

// instrumentKey identifies a token by its on-ledger id. Admin is intentionally
// not part of the key: token config carries only InstrumentID, and in practice
// the local issuer is the sole admin per symbol, so id alone disambiguates.
type instrumentKey struct{ id string }

// instrumentMeta is the subset of token.ERC20Token surfaced in incoming-transfer responses.
type instrumentMeta struct {
	contractAddress string
	name            string
	symbol          string
	decimals        int
}

// NewTransferService creates a new TransferService.
// tokenCfg supplies the list of allowed token symbols (used by Prepare) and the
// instrument→EVM-contract mapping (used to enrich ListIncoming responses).
// offerLister is required — the api-server now wires every service to the same
// indexer client at startup, so ListIncoming relies on it being non-nil.
func NewTransferService(
	cantonToken token.Token,
	userStore UserStore,
	cache TransferCache,
	tokenCfg *pkgtoken.Config,
	offerLister PendingOfferLister,
) *TransferService {
	allowed := map[string]bool{}
	byInstrument := map[instrumentKey]instrumentMeta{}
	if tokenCfg != nil {
		for addr, tkn := range tokenCfg.SupportedTokens {
			allowed[tkn.Symbol] = true
			// We don't know the InstrumentAdmin from token config alone, so the lookup key
			// uses only InstrumentID. The Canton ledger may report multiple admins for the
			// same id; in that case the first one wins, which matches the practical case
			// where the local issuer is the only admin for a given symbol.
			byInstrument[instrumentKey{id: tkn.InstrumentID}] = instrumentMeta{
				contractAddress: addr.Hex(),
				name:            tkn.Name,
				symbol:          tkn.Symbol,
				decimals:        tkn.Decimals,
			}
		}
	}
	return &TransferService{
		cantonToken:         cantonToken,
		userStore:           userStore,
		cache:               cache,
		offerLister:         offerLister,
		allowedTokenSymbols: allowed,
		tokensByInstrument:  byInstrument,
	}
}

// Prepare builds a Canton transaction and returns the hash for external signing.
func (s *TransferService) Prepare(ctx context.Context, senderEVMAddr string, req *PrepareRequest) (*PrepareResponse, error) {
	if !s.allowedTokenSymbols[req.Token] {
		return nil, apperrors.BadRequestError(nil, "unsupported token")
	}

	sender, err := s.userStore.GetUserByEVMAddress(ctx, senderEVMAddr)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, apperrors.UnAuthorizedError(err, "user not found")
		}
		return nil, fmt.Errorf("lookup sender: %w", err)
	}
	if sender.KeyMode != user.KeyModeExternal {
		return nil, apperrors.BadRequestError(nil, "prepare/execute API requires key_mode=external")
	}

	recipient, err := s.userStore.GetUserByEVMAddress(ctx, req.To)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, apperrors.BadRequestError(err, "recipient not found")
		}
		return nil, fmt.Errorf("lookup recipient: %w", err)
	}

	pt, err := s.cantonToken.PrepareTransfer(ctx, &token.PrepareTransferRequest{
		FromPartyID: sender.CantonPartyID,
		ToPartyID:   recipient.CantonPartyID,
		Amount:      req.Amount,
		TokenSymbol: req.Token,
	})
	if err != nil {
		if errors.Is(err, token.ErrInsufficientBalance) {
			return nil, apperrors.BadRequestError(err, "insufficient balance")
		}
		return nil, fmt.Errorf("prepare transfer: %w", err)
	}

	if err := s.cache.Put(pt); err != nil {
		return nil, apperrors.GeneralError(fmt.Errorf("too many pending transfers: %w", err))
	}

	return &PrepareResponse{
		TransferID:      pt.TransferID,
		TransactionHash: "0x" + hex.EncodeToString(pt.TransactionHash),
		PartyID:         pt.PartyID,
		ExpiresAt:       pt.ExpiresAt.Format(time.RFC3339),
	}, nil
}

// Execute completes a previously prepared transfer using the client's DER signature.
func (s *TransferService) Execute(ctx context.Context, senderEVMAddr string, req *ExecuteRequest) (*ExecuteResponse, error) {
	sender, err := s.userStore.GetUserByEVMAddress(ctx, senderEVMAddr)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, apperrors.UnAuthorizedError(err, "user not found")
		}
		return nil, fmt.Errorf("lookup sender: %w", err)
	}
	if sender.CantonPublicKeyFingerprint != req.SignedBy {
		return nil, apperrors.ForbiddenError(nil, "signature fingerprint does not match registered key")
	}

	pt, err := s.cache.GetAndDelete(req.TransferID)
	if err != nil {
		if errors.Is(err, ErrTransferNotFound) {
			return nil, apperrors.ResourceNotFoundError(err, "transfer not found")
		}
		if errors.Is(err, ErrTransferExpired) {
			return nil, apperrors.GoneError(err, "transfer expired")
		}
		return nil, fmt.Errorf("retrieve prepared transfer: %w", err)
	}

	sigBytes, err := hex.DecodeString(strings.TrimPrefix(req.Signature, "0x"))
	if err != nil {
		return nil, apperrors.BadRequestError(err, "invalid DER signature")
	}

	err = s.cantonToken.ExecuteTransfer(ctx, &token.ExecuteTransferRequest{
		PreparedTransfer: pt,
		Signature:        sigBytes,
		SignedBy:         req.SignedBy,
	})
	if err != nil {
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.InvalidArgument, codes.PermissionDenied:
				return nil, apperrors.ForbiddenError(err, "signature verification failed")
			}
		}
		return nil, fmt.Errorf("execute transfer: %w", err)
	}

	return &ExecuteResponse{Status: "completed"}, nil
}

// ListIncoming returns one page of pending inbound TransferOffer details for the
// user with the given EVM address. Unauthenticated: callers do not need to prove
// ownership of evmAddr. Data comes from the indexer's `indexer_pending_offers`
// table (already filtered to status=PENDING at the SQL level), so a single
// indexer call serves a single client page — no buffering, no re-aggregation.
func (s *TransferService) ListIncoming(ctx context.Context, evmAddr string, p indexer.Pagination) (*IncomingTransfersList, error) {
	u, err := s.userStore.GetUserByEVMAddress(ctx, evmAddr)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, apperrors.BadRequestError(err, "user not found")
		}
		return nil, fmt.Errorf("lookup user: %w", err)
	}
	if u.KeyMode != user.KeyModeExternal {
		return nil, apperrors.BadRequestError(nil, "incoming transfer API requires key_mode=external")
	}

	result, err := s.offerLister.GetPendingOffersForParty(ctx, u.CantonPartyID, p)
	if err != nil {
		return nil, fmt.Errorf("list pending offers: %w", err)
	}

	// Start with a non-nil empty slice so the JSON response marshals to `[]`
	// even when the page is empty (clients iterate Items directly).
	items := make([]IncomingTransfer, 0, len(result.Items))
	for i := range result.Items {
		o := &result.Items[i]
		// The indexer's pending-offers query already filters to PENDING at the
		// SQL layer, so this is a defensive check in case the contract changes.
		if o.Status != indexer.OfferStatusPending {
			continue
		}
		// Party IDs are truncated server-side because this endpoint is
		// unauthenticated. Surfacing the full fingerprint would let third
		// parties enumerate counterparties (sender→receiver mapping) just
		// by polling addresses; the truncated form keeps enough for the
		// receiver to disambiguate offers while denying enumeration.
		// ContractID and InstrumentAdmin stay intact: ContractID is needed
		// by the accept flow, and InstrumentAdmin is public token-config
		// data (`/tokens` already exposes it).
		item := IncomingTransfer{
			ContractID:      o.ContractID,
			SenderPartyID:   truncatePartyID(o.SenderPartyID),
			ReceiverPartyID: truncatePartyID(o.ReceiverPartyID),
			Amount:          o.Amount,
			InstrumentAdmin: o.InstrumentAdmin,
			InstrumentID:    o.InstrumentID,
		}
		if meta, ok := s.tokensByInstrument[instrumentKey{id: o.InstrumentID}]; ok {
			item.Symbol = meta.symbol
			item.Decimals = meta.decimals
			item.Name = meta.name
			item.ContractAddress = meta.contractAddress
		}
		items = append(items, item)
	}

	hasMore := int64(p.Page*p.Limit) < result.Total
	return &IncomingTransfersList{
		Items:   items,
		Total:   result.Total,
		Page:    p.Page,
		Limit:   p.Limit,
		HasMore: hasMore,
	}, nil
}

// truncatePartyID returns the first and last few characters of a Canton party
// ID with an ellipsis between them — e.g. `user_2dA…4680b7ec`. Used on
// unauthenticated read endpoints so callers see enough to identify an offer
// without being able to enumerate full party identifiers. Returns the input
// unchanged when it is already short enough that truncation would not help.
func truncatePartyID(partyID string) string {
	const head, tail = 8, 8
	if len(partyID) <= head+tail+1 {
		return partyID
	}
	return partyID[:head] + "…" + partyID[len(partyID)-tail:]
}

// PrepareAccept builds a Canton transaction for accepting an inbound offer.
func (s *TransferService) PrepareAccept(
	ctx context.Context, evmAddr, contractID string, req *PrepareAcceptRequest,
) (*PrepareResponse, error) {
	u, err := s.userStore.GetUserByEVMAddress(ctx, evmAddr)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, apperrors.UnAuthorizedError(err, "user not found")
		}
		return nil, fmt.Errorf("lookup user: %w", err)
	}
	if u.KeyMode != user.KeyModeExternal {
		return nil, apperrors.BadRequestError(nil, "incoming transfer API requires key_mode=external")
	}

	pt, err := s.cantonToken.PrepareAcceptTransfer(ctx, u.CantonPartyID, contractID, req.InstrumentAdmin)
	if err != nil {
		return nil, fmt.Errorf("prepare accept: %w", err)
	}

	if err := s.cache.Put(pt); err != nil {
		return nil, apperrors.GeneralError(fmt.Errorf("too many pending transfers: %w", err))
	}

	return &PrepareResponse{
		TransferID:      pt.TransferID,
		TransactionHash: "0x" + hex.EncodeToString(pt.TransactionHash),
		PartyID:         pt.PartyID,
		ExpiresAt:       pt.ExpiresAt.Format(time.RFC3339),
	}, nil
}

// ExecuteAccept completes a previously prepared accept using the client's DER signature.
func (s *TransferService) ExecuteAccept(ctx context.Context, evmAddr string, req *ExecuteRequest) (*ExecuteResponse, error) {
	return s.Execute(ctx, evmAddr, req)
}
