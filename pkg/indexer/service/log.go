package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/chainsafe/canton-middleware/pkg/indexer"
)

const indexerServiceName = "IndexerService"

// logService wraps Service with automatic logging of all method calls.
type logService struct {
	svc    Service
	logger *zap.Logger
}

// NewLog creates a logging decorator for the indexer Service.
func NewLog(svc Service, logger *zap.Logger) Service {
	return &logService{svc: svc, logger: logger}
}

func (ls *logService) GetToken(ctx context.Context, admin, id string) (t *indexer.Token, err error) {
	start := time.Now()
	ls.logger.Info("GetToken started",
		zap.String("service", indexerServiceName),
		zap.String("admin", admin),
		zap.String("id", id),
	)
	defer func() {
		if err != nil {
			ls.logger.Error("GetToken failed",
				zap.String("service", indexerServiceName),
				zap.String("admin", admin),
				zap.String("id", id),
				zap.Duration("duration", time.Since(start)),
				zap.Error(err),
			)
		} else {
			ls.logger.Info("GetToken completed",
				zap.String("service", indexerServiceName),
				zap.String("admin", admin),
				zap.String("id", id),
				zap.Duration("duration", time.Since(start)),
			)
		}
	}()
	return ls.svc.GetToken(ctx, admin, id)
}

func (ls *logService) ListTokens(ctx context.Context, p indexer.Pagination) (page *indexer.Page[*indexer.Token], err error) {
	start := time.Now()
	ls.logger.Info("ListTokens started",
		zap.String("service", indexerServiceName),
		zap.Int("page", p.Page),
		zap.Int("limit", p.Limit),
	)
	defer func() {
		if err != nil {
			ls.logger.Error("ListTokens failed",
				zap.String("service", indexerServiceName),
				zap.Duration("duration", time.Since(start)),
				zap.Error(err),
			)
		} else {
			ls.logger.Info("ListTokens completed",
				zap.String("service", indexerServiceName),
				zap.Int64("total", page.Total),
				zap.Duration("duration", time.Since(start)),
			)
		}
	}()
	return ls.svc.ListTokens(ctx, p)
}

func (ls *logService) TotalSupply(ctx context.Context, admin, id string) (supply string, err error) {
	start := time.Now()
	ls.logger.Info("TotalSupply started",
		zap.String("service", indexerServiceName),
		zap.String("admin", admin),
		zap.String("id", id),
	)
	defer func() {
		if err != nil {
			ls.logger.Error("TotalSupply failed",
				zap.String("service", indexerServiceName),
				zap.String("admin", admin),
				zap.String("id", id),
				zap.Duration("duration", time.Since(start)),
				zap.Error(err),
			)
		} else {
			ls.logger.Info("TotalSupply completed",
				zap.String("service", indexerServiceName),
				zap.String("admin", admin),
				zap.String("id", id),
				zap.String("total_supply", supply),
				zap.Duration("duration", time.Since(start)),
			)
		}
	}()
	return ls.svc.TotalSupply(ctx, admin, id)
}

func (ls *logService) BalanceOf(ctx context.Context, partyID, admin, id string) (amount string, err error) {
	start := time.Now()
	ls.logger.Info("BalanceOf started",
		zap.String("service", indexerServiceName),
		zap.String("party_id", partyID),
		zap.String("admin", admin),
		zap.String("id", id),
	)
	defer func() {
		if err != nil {
			ls.logger.Error("BalanceOf failed",
				zap.String("service", indexerServiceName),
				zap.String("party_id", partyID),
				zap.Duration("duration", time.Since(start)),
				zap.Error(err),
			)
		} else {
			ls.logger.Info("BalanceOf completed",
				zap.String("service", indexerServiceName),
				zap.String("party_id", partyID),
				zap.Duration("duration", time.Since(start)),
			)
		}
	}()
	return ls.svc.BalanceOf(ctx, partyID, admin, id)
}

func (ls *logService) Allowance(ctx context.Context, owner, spender, admin, id string) (amount string, err error) {
	start := time.Now()
	ls.logger.Info("Allowance started",
		zap.String("service", indexerServiceName),
		zap.String("owner", owner),
		zap.String("spender", spender),
		zap.String("admin", admin),
		zap.String("id", id),
	)
	defer func() {
		if err != nil {
			ls.logger.Error("Allowance failed",
				zap.String("service", indexerServiceName),
				zap.String("owner", owner),
				zap.Duration("duration", time.Since(start)),
				zap.Error(err),
			)
		} else {
			ls.logger.Info("Allowance completed",
				zap.String("service", indexerServiceName),
				zap.String("owner", owner),
				zap.Duration("duration", time.Since(start)),
			)
		}
	}()
	return ls.svc.Allowance(ctx, owner, spender, admin, id)
}

func (ls *logService) GetBalance(ctx context.Context, partyID, admin, id string) (b *indexer.Balance, err error) {
	start := time.Now()
	ls.logger.Info("GetBalance started",
		zap.String("service", indexerServiceName),
		zap.String("party_id", partyID),
		zap.String("admin", admin),
		zap.String("id", id),
	)
	defer func() {
		if err != nil {
			ls.logger.Error("GetBalance failed",
				zap.String("service", indexerServiceName),
				zap.String("party_id", partyID),
				zap.Duration("duration", time.Since(start)),
				zap.Error(err),
			)
		} else {
			ls.logger.Info("GetBalance completed",
				zap.String("service", indexerServiceName),
				zap.String("party_id", partyID),
				zap.Duration("duration", time.Since(start)),
			)
		}
	}()
	return ls.svc.GetBalance(ctx, partyID, admin, id)
}

func (ls *logService) ListBalancesForParty(ctx context.Context, partyID string, p indexer.Pagination) (page *indexer.Page[*indexer.Balance], err error) {
	start := time.Now()
	ls.logger.Info("ListBalancesForParty started",
		zap.String("service", indexerServiceName),
		zap.String("party_id", partyID),
		zap.Int("page", p.Page),
		zap.Int("limit", p.Limit),
	)
	defer func() {
		if err != nil {
			ls.logger.Error("ListBalancesForParty failed",
				zap.String("service", indexerServiceName),
				zap.String("party_id", partyID),
				zap.Duration("duration", time.Since(start)),
				zap.Error(err),
			)
		} else {
			ls.logger.Info("ListBalancesForParty completed",
				zap.String("service", indexerServiceName),
				zap.String("party_id", partyID),
				zap.Int64("total", page.Total),
				zap.Duration("duration", time.Since(start)),
			)
		}
	}()
	return ls.svc.ListBalancesForParty(ctx, partyID, p)
}

func (ls *logService) ListBalancesForToken(ctx context.Context, admin, id string, p indexer.Pagination) (page *indexer.Page[*indexer.Balance], err error) {
	start := time.Now()
	ls.logger.Info("ListBalancesForToken started",
		zap.String("service", indexerServiceName),
		zap.String("admin", admin),
		zap.String("id", id),
		zap.Int("page", p.Page),
		zap.Int("limit", p.Limit),
	)
	defer func() {
		if err != nil {
			ls.logger.Error("ListBalancesForToken failed",
				zap.String("service", indexerServiceName),
				zap.String("admin", admin),
				zap.String("id", id),
				zap.Duration("duration", time.Since(start)),
				zap.Error(err),
			)
		} else {
			ls.logger.Info("ListBalancesForToken completed",
				zap.String("service", indexerServiceName),
				zap.String("admin", admin),
				zap.String("id", id),
				zap.Int64("total", page.Total),
				zap.Duration("duration", time.Since(start)),
			)
		}
	}()
	return ls.svc.ListBalancesForToken(ctx, admin, id, p)
}

func (ls *logService) GetEvent(ctx context.Context, contractID string) (e *indexer.ParsedEvent, err error) {
	start := time.Now()
	ls.logger.Info("GetEvent started",
		zap.String("service", indexerServiceName),
		zap.String("contract_id", contractID),
	)
	defer func() {
		if err != nil {
			ls.logger.Error("GetEvent failed",
				zap.String("service", indexerServiceName),
				zap.String("contract_id", contractID),
				zap.Duration("duration", time.Since(start)),
				zap.Error(err),
			)
		} else {
			ls.logger.Info("GetEvent completed",
				zap.String("service", indexerServiceName),
				zap.String("contract_id", contractID),
				zap.Duration("duration", time.Since(start)),
			)
		}
	}()
	return ls.svc.GetEvent(ctx, contractID)
}

func (ls *logService) ListTokenEvents(ctx context.Context, admin, id string, f indexer.EventFilter, p indexer.Pagination) (page *indexer.Page[*indexer.ParsedEvent], err error) {
	start := time.Now()
	ls.logger.Info("ListTokenEvents started",
		zap.String("service", indexerServiceName),
		zap.String("admin", admin),
		zap.String("id", id),
		zap.Int("page", p.Page),
		zap.Int("limit", p.Limit),
	)
	defer func() {
		if err != nil {
			ls.logger.Error("ListTokenEvents failed",
				zap.String("service", indexerServiceName),
				zap.String("admin", admin),
				zap.String("id", id),
				zap.Duration("duration", time.Since(start)),
				zap.Error(err),
			)
		} else {
			ls.logger.Info("ListTokenEvents completed",
				zap.String("service", indexerServiceName),
				zap.String("admin", admin),
				zap.String("id", id),
				zap.Int64("total", page.Total),
				zap.Duration("duration", time.Since(start)),
			)
		}
	}()
	return ls.svc.ListTokenEvents(ctx, admin, id, f, p)
}

func (ls *logService) ListPartyEvents(ctx context.Context, partyID string, f indexer.EventFilter, p indexer.Pagination) (page *indexer.Page[*indexer.ParsedEvent], err error) {
	start := time.Now()
	ls.logger.Info("ListPartyEvents started",
		zap.String("service", indexerServiceName),
		zap.String("party_id", partyID),
		zap.Int("page", p.Page),
		zap.Int("limit", p.Limit),
	)
	defer func() {
		if err != nil {
			ls.logger.Error("ListPartyEvents failed",
				zap.String("service", indexerServiceName),
				zap.String("party_id", partyID),
				zap.Duration("duration", time.Since(start)),
				zap.Error(err),
			)
		} else {
			ls.logger.Info("ListPartyEvents completed",
				zap.String("service", indexerServiceName),
				zap.String("party_id", partyID),
				zap.Int64("total", page.Total),
				zap.Duration("duration", time.Since(start)),
			)
		}
	}()
	return ls.svc.ListPartyEvents(ctx, partyID, f, p)
}
