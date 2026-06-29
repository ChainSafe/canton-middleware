// SPDX-License-Identifier: Apache-2.0

package transfer

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
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
//go:generate mockery --name IndexerReader --output mocks --outpkg mocks --filename mock_indexer_reader.go --with-expecter
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

// IndexerReader is the slice of indexer/client.Client the transfer service uses
// to read TransferOffer state. The indexer is the source of truth for the offer
// lifecycle (incoming/outgoing/expired/accepted), so the service reads from it
// rather than re-decoding Canton contracts.
type IndexerReader interface {
	GetTransfers(
		ctx context.Context, partyID string, query indexer.TransferQuery, p indexer.Pagination,
	) (*indexer.Page[indexer.Transfer], error)
	// GetTransfer returns a single transfer by its contract id (used to validate a
	// claim-back request directly, without scanning a party's transfer list).
	GetTransfer(ctx context.Context, contractID string) (*indexer.Transfer, error)
}

// Service is the interface for the non-custodial prepare/execute transfer flow.
type Service interface {
	Prepare(ctx context.Context, senderEVMAddr string, req *PrepareRequest) (*PrepareResponse, error)
	Execute(ctx context.Context, senderEVMAddr string, req *ExecuteRequest) (*ExecuteResponse, error)

	// SendCustodial performs a single-call, server-signed transfer for a custodial
	// user to an arbitrary recipient party id (the middleware holds the user's
	// Canton key, so there is no client prepare/execute round-trip).
	SendCustodial(ctx context.Context, senderEVMAddr string, req *CustodialTransferRequest) (*ExecuteResponse, error)

	// ListIncoming returns one page of pending inbound TransferOffer details for the
	// user with the given EVM address. This call is unauthenticated — anyone can
	// query any address's pending offers; the response is intentionally minimized
	// (party IDs truncated) to keep that from leaking counterparties.
	ListIncoming(ctx context.Context, evmAddr string, p indexer.Pagination) (*IncomingTransfersList, error)
	// ListOutgoing returns one page of the user's outbound transfers filtered
	// by status (pending / expired / completed / all). Like ListIncoming it is
	// unauthenticated and truncates party IDs.
	ListOutgoing(ctx context.Context, evmAddr string, status string, p indexer.Pagination) (*OutgoingTransfersList, error)
	// ListCompleted returns one page of the user's settled transfers across all
	// tokens (TokenTransferEvents and accepted TransferOffers), newest first.
	ListCompleted(ctx context.Context, evmAddr string, p indexer.Pagination) (*CompletedTransfersList, error)
	// PrepareAccept builds a Canton transaction for accepting an inbound offer.
	PrepareAccept(
		ctx context.Context, evmAddr, contractID string, req *PrepareAcceptRequest,
	) (*PrepareResponse, error)
	// ExecuteAccept completes a previously prepared accept using the client's DER signature.
	ExecuteAccept(ctx context.Context, evmAddr string, req *ExecuteRequest) (*ExecuteResponse, error)
	// PrepareWithdraw builds a Canton transaction for a non-custodial sender to claim
	// back (withdraw) a pending/expired offer they sent. Complete it via Execute.
	PrepareWithdraw(ctx context.Context, evmAddr, contractID string) (*PrepareResponse, error)
	// WithdrawCustodial claims back a pending/expired offer for a custodial sender in a
	// single server-signed call.
	WithdrawCustodial(ctx context.Context, evmAddr, contractID string) (*ExecuteResponse, error)
}

// TransferService implements the non-custodial prepare/execute transfer flow.
type TransferService struct {
	cantonToken         token.Token
	userStore           UserStore
	cache               TransferCache
	offerLister         IndexerReader
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
	offerLister IndexerReader,
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

// maxValiditySeconds bounds validity_seconds so that converting it to a
// time.Duration (nanoseconds, int64) cannot overflow. Above this, the
// multiplication by time.Second would wrap and could yield a small/negative
// duration, silently expiring the offer early.
const maxValiditySeconds = math.MaxInt64 / int64(time.Second)

// validityDuration validates the request's validity_seconds and converts it to a
// time.Duration. It rejects non-positive and overflow-prone values with a 400.
func validityDuration(seconds int64) (time.Duration, error) {
	if seconds <= 0 {
		return 0, apperrors.BadRequestError(nil, "validity_seconds must be a positive number")
	}
	if seconds > maxValiditySeconds {
		return 0, apperrors.BadRequestError(nil, "validity_seconds is too large")
	}
	return time.Duration(seconds) * time.Second, nil
}

// Prepare builds a Canton transaction and returns the hash for external signing.
func (s *TransferService) Prepare(ctx context.Context, senderEVMAddr string, req *PrepareRequest) (*PrepareResponse, error) {
	if !s.allowedTokenSymbols[req.Token] {
		return nil, apperrors.BadRequestError(nil, "unsupported token")
	}
	validity, err := validityDuration(req.ValiditySeconds)
	if err != nil {
		return nil, err
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

	// Exactly one recipient form is allowed; reject an ambiguous request rather
	// than silently preferring one (the HTTP handler enforces this too, but the
	// service must not resolve an ambiguous request when called directly).
	if (req.To == "") == (req.ToPartyID == "") {
		return nil, apperrors.BadRequestError(nil, "exactly one of to or to_party_id is required")
	}

	// Resolve the recipient party id. When the caller supplies a raw party id we
	// use it directly (it may be a party not registered in the middleware, e.g.
	// hosted on an external participant node); otherwise we look up the EVM
	// address of a registered user.
	toPartyID := req.ToPartyID
	if toPartyID == "" {
		recipient, lookupErr := s.userStore.GetUserByEVMAddress(ctx, req.To)
		if lookupErr != nil {
			if errors.Is(lookupErr, user.ErrUserNotFound) {
				return nil, apperrors.BadRequestError(lookupErr, "recipient not found")
			}
			return nil, fmt.Errorf("lookup recipient: %w", lookupErr)
		}
		toPartyID = recipient.CantonPartyID
	} else if vErr := validatePartyID(toPartyID); vErr != nil {
		return nil, apperrors.BadRequestError(vErr, "invalid recipient party id")
	}
	if toPartyID == sender.CantonPartyID {
		return nil, apperrors.BadRequestError(nil, "cannot transfer to self")
	}

	pt, err := s.cantonToken.PrepareTransfer(ctx, &token.PrepareTransferRequest{
		FromPartyID: sender.CantonPartyID,
		ToPartyID:   toPartyID,
		Amount:      req.Amount,
		TokenSymbol: req.Token,
		Validity:    validity,
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

// SendCustodial performs a single-call, server-signed transfer for a custodial
// user to an arbitrary recipient party id. The middleware holds the custodial
// user's Canton key, so it both prepares and executes the transfer (no client
// round-trip). The recipient must accept the resulting TransferOffer on their
// own participant node; this call only creates and submits the offer.
func (s *TransferService) SendCustodial(
	ctx context.Context, senderEVMAddr string, req *CustodialTransferRequest,
) (*ExecuteResponse, error) {
	if !s.allowedTokenSymbols[req.Token] {
		return nil, apperrors.BadRequestError(nil, "unsupported token")
	}
	validity, err := validityDuration(req.ValiditySeconds)
	if err != nil {
		return nil, err
	}

	if err = validatePartyID(req.ToPartyID); err != nil {
		return nil, apperrors.BadRequestError(err, "invalid recipient party id")
	}

	sender, err := s.userStore.GetUserByEVMAddress(ctx, senderEVMAddr)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, apperrors.UnAuthorizedError(err, "user not found")
		}
		return nil, fmt.Errorf("lookup sender: %w", err)
	}
	if sender.KeyMode != user.KeyModeCustodial {
		return nil, apperrors.BadRequestError(nil, "this endpoint requires key_mode=custodial")
	}
	if req.ToPartyID == sender.CantonPartyID {
		return nil, apperrors.BadRequestError(nil, "cannot transfer to self")
	}

	// The middleware signs server-side, so prepare+execute happen in one call.
	// A fresh idempotency key is used as the Canton command id per request.
	err = s.cantonToken.TransferByPartyID(
		ctx, uuid.NewString(), sender.CantonPartyID, req.ToPartyID, req.Amount, req.Token, validity,
	)
	if err != nil {
		if errors.Is(err, token.ErrInsufficientBalance) {
			return nil, apperrors.BadRequestError(err, "insufficient balance")
		}
		return nil, fmt.Errorf("transfer: %w", err)
	}

	return &ExecuteResponse{Status: "submitted"}, nil
}

// validatePartyID does a lightweight syntactic check of a Canton party id, which
// has the form "<hint>::<fingerprint>" where the fingerprint is a hex-encoded
// multihash. It rejects obvious garbage (e.g. an EVM address pasted by mistake);
// an id that is well-formed but unroutable is surfaced by Canton at submission.
func validatePartyID(partyID string) error {
	hint, fingerprint, ok := strings.Cut(partyID, "::")
	if !ok || hint == "" || fingerprint == "" {
		return fmt.Errorf("party id must be of the form <hint>::<fingerprint>")
	}
	if _, err := hex.DecodeString(fingerprint); err != nil {
		return fmt.Errorf("party id fingerprint must be hex-encoded")
	}
	return nil
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

	result, err := s.offerLister.GetTransfers(ctx, u.CantonPartyID,
		indexer.TransferQuery{Role: indexer.TransferRoleReceiver, Status: indexer.TransferStatusPending}, p)
	if err != nil {
		return nil, fmt.Errorf("list pending transfers: %w", err)
	}

	// Start with a non-nil empty slice so the JSON response marshals to `[]`
	// even when the page is empty (clients iterate Items directly).
	items := make([]IncomingTransfer, 0, len(result.Items))
	for i := range result.Items {
		o := &result.Items[i]
		// The indexer's query already filters to pending at the SQL layer, so
		// this is a defensive check in case the contract changes.
		if o.Status != indexer.TransferStatusPending {
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
			SenderPartyID:   truncatePartyID(o.FromPartyID),
			ReceiverPartyID: truncatePartyID(o.ToPartyID),
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

// ListOutgoing returns one page of the user's outbound transfers filtered
// by status. Data comes from the indexer (role=sender); party IDs are truncated
// like ListIncoming since the endpoint is unauthenticated.
func (s *TransferService) ListOutgoing(
	ctx context.Context, evmAddr string, status string, p indexer.Pagination,
) (*OutgoingTransfersList, error) {
	u, err := s.userStore.GetUserByEVMAddress(ctx, evmAddr)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, apperrors.BadRequestError(err, "user not found")
		}
		return nil, fmt.Errorf("lookup user: %w", err)
	}

	result, err := s.offerLister.GetTransfers(ctx, u.CantonPartyID,
		indexer.TransferQuery{Role: indexer.TransferRoleSender, Status: status}, p)
	if err != nil {
		return nil, fmt.Errorf("list outgoing transfers: %w", err)
	}

	items := make([]OutgoingTransfer, 0, len(result.Items))
	for i := range result.Items {
		o := &result.Items[i]
		item := OutgoingTransfer{
			ContractID:      o.ContractID,
			SenderPartyID:   truncatePartyID(o.FromPartyID),
			ReceiverPartyID: truncatePartyID(o.ToPartyID),
			Amount:          o.Amount,
			InstrumentAdmin: o.InstrumentAdmin,
			InstrumentID:    o.InstrumentID,
			Status:          o.Status,
		}
		if o.ExpiresAt != nil {
			item.ExpiresAt = o.ExpiresAt.Format(time.RFC3339)
		}
		if meta, ok := s.tokensByInstrument[instrumentKey{id: o.InstrumentID}]; ok {
			item.Symbol = meta.symbol
			item.Decimals = meta.decimals
			item.Name = meta.name
			item.ContractAddress = meta.contractAddress
		}
		items = append(items, item)
	}

	return &OutgoingTransfersList{
		Items:   items,
		Total:   result.Total,
		Page:    p.Page,
		Limit:   p.Limit,
		HasMore: int64(p.Page*p.Limit) < result.Total,
	}, nil
}

// ListCompleted returns one page of the user's settled transfers across all
// tokens. Data comes from the indexer's generalized completed-transfers query;
// party IDs are truncated like the other read endpoints since this is unauthenticated.
func (s *TransferService) ListCompleted(
	ctx context.Context, evmAddr string, p indexer.Pagination,
) (*CompletedTransfersList, error) {
	u, err := s.userStore.GetUserByEVMAddress(ctx, evmAddr)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, apperrors.BadRequestError(err, "user not found")
		}
		return nil, fmt.Errorf("lookup user: %w", err)
	}

	result, err := s.offerLister.GetTransfers(ctx, u.CantonPartyID,
		indexer.TransferQuery{Role: indexer.TransferRoleAny, Status: indexer.TransferStatusCompleted}, p)
	if err != nil {
		return nil, fmt.Errorf("list completed transfers: %w", err)
	}

	items := make([]CompletedTransfer, 0, len(result.Items))
	for i := range result.Items {
		t := &result.Items[i]
		item := CompletedTransfer{
			ContractID:      t.ContractID,
			Kind:            t.Kind,
			Status:          t.Status,
			FromPartyID:     truncatePartyID(t.FromPartyID),
			ToPartyID:       truncatePartyID(t.ToPartyID),
			Amount:          t.Amount,
			InstrumentAdmin: t.InstrumentAdmin,
			InstrumentID:    t.InstrumentID,
			Timestamp:       t.CreatedAt.Format(time.RFC3339),
			TxID:            t.TxID,
		}
		if meta, ok := s.tokensByInstrument[instrumentKey{id: t.InstrumentID}]; ok {
			item.Symbol = meta.symbol
			item.Decimals = meta.decimals
			item.Name = meta.name
			item.ContractAddress = meta.contractAddress
		}
		items = append(items, item)
	}

	return &CompletedTransfersList{
		Items:   items,
		Total:   result.Total,
		Page:    p.Page,
		Limit:   p.Limit,
		HasMore: int64(p.Page*p.Limit) < result.Total,
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

// claimableTransfer validates a claim-back request against the indexer before any
// Canton call. It looks the offer up directly by contract id and confirms the caller
// owns it (is the sender), it is an offer (not a direct CIP-56 transfer), and it is
// still withdrawable (pending or expired, not already completed). Returns the offer so
// callers can use its InstrumentAdmin for the on-ledger withdraw. Errors: 404 when no
// such offer exists for the caller, 400 when it is not a withdrawable offer.
func (s *TransferService) claimableTransfer(
	ctx context.Context, callerParty, contractID string,
) (*indexer.Transfer, error) {
	t, err := s.offerLister.GetTransfer(ctx, contractID)
	if err != nil {
		// The indexer client maps a missing contract id to a not-found app error;
		// propagate it (and any other lookup failure) as-is.
		return nil, err
	}
	// Ownership: only the sender may claim back. Report a missing/foreign offer as
	// not-found so callers can't probe other parties' offers by contract id.
	if t.FromPartyID != callerParty {
		return nil, apperrors.ResourceNotFoundError(nil, "no claimable offer found for this contract id")
	}
	if t.Kind != indexer.TransferKindOffer {
		return nil, apperrors.BadRequestError(nil, "only offer-based transfers can be claimed back")
	}
	if t.Status == indexer.TransferStatusCompleted {
		return nil, apperrors.BadRequestError(nil, "transfer already completed; nothing to claim back")
	}
	return t, nil
}

// PrepareWithdraw builds a Canton transaction for a non-custodial sender to claim back
// (withdraw) a pending or expired offer they sent. Returns the hash to sign; complete
// it via the standard Execute endpoint.
func (s *TransferService) PrepareWithdraw(ctx context.Context, evmAddr, contractID string) (*PrepareResponse, error) {
	u, err := s.userStore.GetUserByEVMAddress(ctx, evmAddr)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, apperrors.UnAuthorizedError(err, "user not found")
		}
		return nil, fmt.Errorf("lookup user: %w", err)
	}
	if u.KeyMode != user.KeyModeExternal {
		return nil, apperrors.BadRequestError(nil, "withdraw prepare/execute API requires key_mode=external")
	}

	offer, err := s.claimableTransfer(ctx, u.CantonPartyID, contractID)
	if err != nil {
		return nil, err
	}

	pt, err := s.cantonToken.PrepareWithdrawTransfer(ctx, u.CantonPartyID, contractID, offer.InstrumentAdmin)
	if err != nil {
		return nil, mapWithdrawErr(err)
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

// WithdrawCustodial claims back a pending or expired offer for a custodial sender in a
// single server-signed call (the middleware holds the user's Canton key).
func (s *TransferService) WithdrawCustodial(ctx context.Context, evmAddr, contractID string) (*ExecuteResponse, error) {
	u, err := s.userStore.GetUserByEVMAddress(ctx, evmAddr)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, apperrors.UnAuthorizedError(err, "user not found")
		}
		return nil, fmt.Errorf("lookup user: %w", err)
	}
	if u.KeyMode != user.KeyModeCustodial {
		return nil, apperrors.BadRequestError(nil, "this endpoint requires key_mode=custodial")
	}

	offer, err := s.claimableTransfer(ctx, u.CantonPartyID, contractID)
	if err != nil {
		return nil, err
	}

	if err := s.cantonToken.WithdrawTransferInstruction(ctx, u.CantonPartyID, contractID, offer.InstrumentAdmin); err != nil {
		return nil, mapWithdrawErr(err)
	}
	return &ExecuteResponse{Status: "submitted"}, nil
}

// mapWithdrawErr maps a Canton/registry withdraw failure to an HTTP-shaped error.
// A receiver who accepted first (or any state where the instruction is no longer
// active) surfaces as 409 Conflict; everything else is a generic dependency error.
func mapWithdrawErr(err error) error {
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.NotFound, codes.FailedPrecondition, codes.Aborted, codes.AlreadyExists:
			return apperrors.ConflictError(err, "offer is no longer claimable (it may have been accepted or already withdrawn)")
		}
	}
	return fmt.Errorf("withdraw transfer: %w", err)
}
