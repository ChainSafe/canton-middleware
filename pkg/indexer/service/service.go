package service

import (
	"context"

	"go.uber.org/zap"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/chainsafe/canton-middleware/pkg/indexer"
)

const (
	MaxLimit     = 200
	DefaultLimit = 50
)

//go:generate mockery --name ReadinessChecker --output mocks --outpkg mocks --filename mock_readiness_checker.go --with-expecter

// ReadinessChecker reports whether the indexer has caught up to the ledger.
// The engine.Processor implements this interface.
type ReadinessChecker interface {
	Ready() bool
}

//go:generate mockery --name Store --output mocks --outpkg mocks --filename mock_store.go --with-expecter
type Store interface {
	GetToken(ctx context.Context, admin, id string) (*indexer.Token, error)
	ListTokens(ctx context.Context, p indexer.Pagination) ([]*indexer.Token, int64, error)
	GetBalance(ctx context.Context, partyID, admin, id string) (*indexer.Balance, error)
	ListBalancesForParty(ctx context.Context, partyID string, p indexer.Pagination) ([]*indexer.Balance, int64, error)
	ListBalancesForToken(ctx context.Context, admin, id string, p indexer.Pagination) ([]*indexer.Balance, int64, error)
	// ListEvents returns events in ascending ledger_offset order (immutable audit trail).
	// Zero-value EventFilter fields are ignored (no filter applied).
	ListEvents(ctx context.Context, f indexer.EventFilter, p indexer.Pagination) ([]*indexer.ParsedEvent, int64, error)
	// GetEvent looks up a single event by its unique contract ID.
	GetEvent(ctx context.Context, contractID string) (*indexer.ParsedEvent, error)
	// GetAllowance is a stub — AllowanceContract indexing is deferred to a later phase.
	GetAllowance(ctx context.Context, owner, spender, admin, id string) (string, error)
}

//go:generate mockery --name Service --output mocks --outpkg mocks --filename mock_service.go --with-expecter
type Service interface {
	// Token queries
	GetToken(ctx context.Context, admin, id string) (*indexer.Token, error)
	ListTokens(ctx context.Context, p indexer.Pagination) (*indexer.Page[*indexer.Token], error)

	// ERC-20 analogs
	TotalSupply(ctx context.Context, admin, id string) (string, error)                       // totalSupply()
	BalanceOf(ctx context.Context, partyID, admin, id string) (string, error)                // balanceOf(address)
	Allowance(ctx context.Context, owner, spender, admin, id string) (string, error)         // allowance() — stub

	// Rich balance queries (beyond ERC-20)
	GetBalance(ctx context.Context, partyID, admin, id string) (*indexer.Balance, error)
	ListBalancesForParty(ctx context.Context, partyID string, p indexer.Pagination) (*indexer.Page[*indexer.Balance], error)
	ListBalancesForToken(ctx context.Context, admin, id string, p indexer.Pagination) (*indexer.Page[*indexer.Balance], error)

	// Audit trail (immutable, ordered by ledger_offset ASC)
	GetEvent(ctx context.Context, contractID string) (*indexer.ParsedEvent, error)
	ListTokenEvents(ctx context.Context, admin, id string, f indexer.EventFilter, p indexer.Pagination) (*indexer.Page[*indexer.ParsedEvent], error)
	ListPartyEvents(ctx context.Context, partyID string, f indexer.EventFilter, p indexer.Pagination) (*indexer.Page[*indexer.ParsedEvent], error)
}

// NewService creates a new indexer Service backed by store.
func NewService(store Store, logger *zap.Logger) Service {
	return &svc{store: store, logger: logger}
}

type svc struct {
	store  Store
	logger *zap.Logger
}

// requesterParty reads the canton_party_id injected by jwtMiddleware.
// The middleware guarantees the value is always present when this is called.
func requesterParty(ctx context.Context) (string, error) {
	party, ok := auth.CantonPartyFromContext(ctx)
	if !ok || party == "" {
		return "", apperrors.UnAuthorizedError(nil, "missing canton_party_id")
	}
	return party, nil
}

func (s *svc) GetToken(ctx context.Context, admin, id string) (*indexer.Token, error) {
	if _, err := requesterParty(ctx); err != nil {
		return nil, err
	}
	t, err := s.store.GetToken(ctx, admin, id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, apperrors.ResourceNotFoundError(nil, "token not found")
	}
	return t, nil
}

func (s *svc) ListTokens(ctx context.Context, p indexer.Pagination) (*indexer.Page[*indexer.Token], error) {
	if _, err := requesterParty(ctx); err != nil {
		return nil, err
	}
	items, total, err := s.store.ListTokens(ctx, p)
	if err != nil {
		return nil, err
	}
	return &indexer.Page[*indexer.Token]{Items: items, Total: total, Page: p.Page, Limit: p.Limit}, nil
}

func (s *svc) TotalSupply(ctx context.Context, admin, id string) (string, error) {
	if _, err := requesterParty(ctx); err != nil {
		return "", err
	}
	t, err := s.store.GetToken(ctx, admin, id)
	if err != nil {
		return "", err
	}
	if t == nil {
		return "", apperrors.ResourceNotFoundError(nil, "token not found")
	}
	return t.TotalSupply, nil
}

func (s *svc) BalanceOf(ctx context.Context, partyID, admin, id string) (string, error) {
	requester, err := requesterParty(ctx)
	if err != nil {
		return "", err
	}
	if requester != partyID && requester != admin {
		return "", apperrors.ForbiddenError(nil, "access denied")
	}
	b, err := s.store.GetBalance(ctx, partyID, admin, id)
	if err != nil {
		return "", err
	}
	if b == nil {
		return "", apperrors.ResourceNotFoundError(nil, "balance not found")
	}
	return b.Amount, nil
}

func (s *svc) Allowance(ctx context.Context, owner, spender, admin, id string) (string, error) {
	requester, err := requesterParty(ctx)
	if err != nil {
		return "", err
	}
	if requester != owner && requester != admin {
		return "", apperrors.ForbiddenError(nil, "access denied")
	}
	return s.store.GetAllowance(ctx, owner, spender, admin, id)
}

func (s *svc) GetBalance(ctx context.Context, partyID, admin, id string) (*indexer.Balance, error) {
	requester, err := requesterParty(ctx)
	if err != nil {
		return nil, err
	}
	if requester != partyID && requester != admin {
		return nil, apperrors.ForbiddenError(nil, "access denied")
	}
	b, err := s.store.GetBalance(ctx, partyID, admin, id)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, apperrors.ResourceNotFoundError(nil, "balance not found")
	}
	return b, nil
}

func (s *svc) ListBalancesForParty(ctx context.Context, partyID string, p indexer.Pagination) (*indexer.Page[*indexer.Balance], error) {
	requester, err := requesterParty(ctx)
	if err != nil {
		return nil, err
	}
	if requester != partyID {
		return nil, apperrors.ForbiddenError(nil, "access denied")
	}
	items, total, err := s.store.ListBalancesForParty(ctx, partyID, p)
	if err != nil {
		return nil, err
	}
	return &indexer.Page[*indexer.Balance]{Items: items, Total: total, Page: p.Page, Limit: p.Limit}, nil
}

func (s *svc) ListBalancesForToken(ctx context.Context, admin, id string, p indexer.Pagination) (*indexer.Page[*indexer.Balance], error) {
	requester, err := requesterParty(ctx)
	if err != nil {
		return nil, err
	}
	if requester != admin {
		return nil, apperrors.ForbiddenError(nil, "access denied")
	}
	items, total, err := s.store.ListBalancesForToken(ctx, admin, id, p)
	if err != nil {
		return nil, err
	}
	return &indexer.Page[*indexer.Balance]{Items: items, Total: total, Page: p.Page, Limit: p.Limit}, nil
}

func (s *svc) GetEvent(ctx context.Context, contractID string) (*indexer.ParsedEvent, error) {
	if _, err := requesterParty(ctx); err != nil {
		return nil, err
	}
	e, err := s.store.GetEvent(ctx, contractID)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, apperrors.ResourceNotFoundError(nil, "event not found")
	}
	return e, nil
}

func (s *svc) ListTokenEvents(ctx context.Context, admin, id string, f indexer.EventFilter, p indexer.Pagination) (*indexer.Page[*indexer.ParsedEvent], error) {
	if _, err := requesterParty(ctx); err != nil {
		return nil, err
	}
	f.InstrumentAdmin = admin
	f.InstrumentID = id
	items, total, err := s.store.ListEvents(ctx, f, p)
	if err != nil {
		return nil, err
	}
	return &indexer.Page[*indexer.ParsedEvent]{Items: items, Total: total, Page: p.Page, Limit: p.Limit}, nil
}

func (s *svc) ListPartyEvents(ctx context.Context, partyID string, f indexer.EventFilter, p indexer.Pagination) (*indexer.Page[*indexer.ParsedEvent], error) {
	requester, err := requesterParty(ctx)
	if err != nil {
		return nil, err
	}
	if requester != partyID {
		return nil, apperrors.ForbiddenError(nil, "access denied")
	}
	f.PartyID = partyID
	items, total, err := s.store.ListEvents(ctx, f, p)
	if err != nil {
		return nil, err
	}
	return &indexer.Page[*indexer.ParsedEvent]{Items: items, Total: total, Page: p.Page, Limit: p.Limit}, nil
}
