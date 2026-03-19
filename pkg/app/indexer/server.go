// Package indexer implements the runner for the indexer process.
//
// The indexer has two concurrent responsibilities:
//  1. A processor that streams TokenTransferEvents from the Canton ledger via
//     GetUpdates and persists them (events, token supply, balances) into a
//     dedicated PostgreSQL database.
//  2. An HTTP read API that exposes the indexed data under /indexer/v1.
//
// Both halves run under the same context via errgroup so that an OS signal or
// a fatal error in either half cancels the other cleanly.
package indexer

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	apphttp "github.com/chainsafe/canton-middleware/pkg/app/http"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/ledger"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/streaming"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/indexer/engine"
	indexerservice "github.com/chainsafe/canton-middleware/pkg/indexer/service"
	indexerstore "github.com/chainsafe/canton-middleware/pkg/indexer/store"
	"github.com/chainsafe/canton-middleware/pkg/log"
	"github.com/chainsafe/canton-middleware/pkg/pgutil"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

const defaultMiddlewareTimeout = 60 * time.Second

// Server holds the configuration for the indexer process.
type Server struct {
	cfg *config.IndexerServer
}

// NewServer creates a new indexer Server.
func NewServer(cfg *config.IndexerServer) *Server {
	return &Server{cfg: cfg}
}

// Run starts the Canton stream processor and the HTTP read API concurrently.
// It blocks until an OS shutdown signal is received or a fatal error occurs.
func (s *Server) Run() error {
	if s.cfg == nil {
		return fmt.Errorf("nil config")
	}
	cfg := s.cfg

	if cfg.Indexer.Party == "" {
		return fmt.Errorf("indexer.party is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger, err := log.NewLogger(cfg.Logging)
	if err != nil {
		return fmt.Errorf("create logger: %w", err)
	}
	defer func() { _ = logger.Sync() }()

	logger.Info("Starting Canton Indexer")

	// ── Database ──────────────────────────────────────────────────────────────

	db, err := pgutil.ConnectDB(cfg.Database)
	if err != nil {
		return fmt.Errorf("connect indexer db: %w", err)
	}
	defer func() { _ = db.Close() }()
	logger.Info("Database connection established")

	// ── Canton ledger connection ───────────────────────────────────────────────
	// The indexer only needs the low-level ledger gRPC client for streaming —
	// the Identity, Token, and Bridge sub-clients provided by canton.New are
	// not required here.

	ledgerClient, err := ledger.New(cfg.CantonLedger, ledger.WithLogger(logger))
	if err != nil {
		return fmt.Errorf("create ledger client: %w", err)
	}
	defer func() { _ = ledgerClient.Close() }()
	logger.Info("Canton ledger connection established", zap.String("rpc_url", cfg.CantonLedger.RPCURL))

	// ── Streaming client ──────────────────────────────────────────────────────
	// One streaming.Client per indexer party. It wraps GetUpdates with automatic
	// reconnection and OAuth2 token refresh (mirrors bridge/client.go pattern).

	streamClient := streaming.New(ledgerClient, cfg.Indexer.Party, streaming.WithLogger(logger))

	// ── Template identifier ───────────────────────────────────────────────────

	templateID := streaming.TemplateID{
		PackageID:  cfg.Indexer.CIP56PackageID,
		ModuleName: "CIP56.Events",
		EntityName: "TokenTransferEvent",
	}

	// ── Decoder / Fetcher / Processor (write path) ────────────────────────────

	filterMode, instruments := cfg.Indexer.FilterModeAndKeys()
	decode := engine.NewTokenTransferDecoder(filterMode, instruments, logger)
	fetcher := engine.NewFetcher(streamClient, templateID, decode, logger)
	store := indexerstore.NewStore(db)
	processor := engine.NewProcessor(fetcher, store, logger)

	// ── Service / Router (read path) ──────────────────────────────────────────

	svc := indexerservice.NewService(store, logger)
	router := s.newRouter(svc, logger)

	// ── Run both halves concurrently ──────────────────────────────────────────

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		logger.Info("Indexer processor starting", zap.String("party", cfg.Indexer.Party))
		return processor.Run(gctx)
	})

	g.Go(func() error {
		logger.Info("Indexer HTTP server starting",
			zap.String("host", cfg.Server.Host),
			zap.Int("port", cfg.Server.Port),
		)
		return apphttp.ServeAndWait(gctx, router, logger, cfg.Server)
	})

	return g.Wait()
}

// newRouter builds the chi router with standard middleware, a /health endpoint,
// and the private admin routes under /indexer/v1/admin.
//
// Currently the indexer exposes a single unauthenticated port intended for
// internal/trusted callers (backend services, ops tooling). A public read API
// with JWT authentication will be added in a future iteration on a separate
// route group. Until then, restrict network access to this port at the
// infrastructure level (firewall, private VPC, etc.).
func (s *Server) newRouter(svc indexerservice.Service, logger *zap.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(defaultMiddlewareTimeout))

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	indexerservice.RegisterPrivateRoutes(r, svc, logger)

	return r
}
