// SPDX-License-Identifier: Apache-2.0

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

	sharedmetrics "github.com/chainsafe/canton-middleware/internal/metrics"
	apphttp "github.com/chainsafe/canton-middleware/pkg/app/http"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/ledger"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/streaming"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/indexer"
	"github.com/chainsafe/canton-middleware/pkg/indexer/engine"
	indexerservice "github.com/chainsafe/canton-middleware/pkg/indexer/service"
	indexerstore "github.com/chainsafe/canton-middleware/pkg/indexer/store"
	"github.com/chainsafe/canton-middleware/pkg/log"
	"github.com/chainsafe/canton-middleware/pkg/pgutil"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
	// One streaming.Client for the indexer in wildcard mode (FiltersForAnyParty).
	// Wraps GetUpdates with automatic reconnection and OAuth2 token refresh
	// (mirrors bridge/client.go pattern). Requires the Canton auth token to
	// carry CanReadAsAnyParty rights.

	streamClient, err := streaming.New(ledgerClient, streaming.WithLogger(logger))
	if err != nil {
		return fmt.Errorf("create streaming client: %w", err)
	}

	// ── Template identifiers ──────────────────────────────────────────────────

	templateIDs := indexerTemplateIDs(cfg.Indexer)

	// ── Metrics — registered once, injected into processor and store ──────────

	reg := sharedmetrics.WithNamespace(prometheus.DefaultRegisterer, "indexer_server")
	engineMetrics := engine.NewMetrics(reg)
	storeMetrics := indexerstore.NewStoreMetrics(reg)
	httpMetrics := apphttp.NewHTTPMetrics(reg)

	// ── Decoder / Fetcher / Processor (write path) ────────────────────────────

	filterMode, instruments := cfg.Indexer.FilterModeAndKeys()
	transferDecode := engine.NewTokenTransferDecoder(filterMode, instruments, logger)
	offerDecode := engine.NewOfferDecoder(cfg.Indexer.UtilityRegistryPackageID, logger)
	holdingDecode := engine.NewHoldingDecoder(cfg.Indexer.UtilityRegistryHoldingPackageID, logger)
	decode := engine.NewMultiDecoder(transferDecode, offerDecode, holdingDecode)
	fetcher := engine.NewFetcher(streamClient, templateIDs, decode, logger)
	rawStore := indexerstore.NewStore(db)
	store := indexerstore.NewInstrumentedStore(rawStore, storeMetrics)
	processor := engine.NewProcessor(fetcher, store, engineMetrics, logger)

	// ── Service / Router (read path) ──────────────────────────────────────────

	svc := indexerservice.NewService(store, logger)
	router := s.newRouter(svc, httpMetrics, logger)

	// ── Run processor and HTTP servers under one errgroup ─────────────────────
	// The write-path processor and the read-path HTTP server(s) all share gCtx:
	// an OS signal or a fatal error in any of them cancels gCtx and unwinds the
	// rest. g.Wait() blocks until every goroutine has drained, so the deferred
	// ledger/db closes below never race with the still-running processor.

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		logger.Info("Indexer processor starting")
		return processor.Run(gCtx)
	})

	s.registerServers(g, gCtx, router, logger)

	return g.Wait()
}

// registerServers adds the indexer HTTP server and, when monitoring is enabled,
// the metrics server to the shared errgroup. They run on gCtx alongside the
// processor, so a failure in any of them cancels gCtx and unwinds the rest;
// the caller's g.Wait() surfaces the first error.
func (s *Server) registerServers(g *errgroup.Group, gCtx context.Context, router http.Handler, logger *zap.Logger) {
	g.Go(func() error {
		logger.Info("Indexer HTTP server starting",
			zap.String("host", s.cfg.Server.Host),
			zap.Int("port", s.cfg.Server.Port),
		)
		return apphttp.ServeAndWait(gCtx, router, logger, s.cfg.Server)
	})

	if s.cfg.Monitoring != nil && s.cfg.Monitoring.Enabled {
		// Monitoring.Server is enforced non-nil at config load time
		// (`validate:"required_if=Enabled true"`), so no runtime check here —
		// failing late would leak the main HTTP goroutine started above.
		r := chi.NewRouter()
		r.Use(middleware.Recoverer)
		r.Handle("/metrics", promhttp.Handler())

		g.Go(func() error {
			return apphttp.ServeAndWait(gCtx, r, logger, s.cfg.Monitoring.Server)
		})
	}
}

// indexerTemplateIDs builds the streaming template-ID list the fetcher subscribes
// to. CIP-56 TokenTransferEvent is always included; Utility.Registry TransferOffer
// and Holding are appended when their package IDs are configured.
func indexerTemplateIDs(cfg *indexer.Config) []streaming.TemplateID {
	ids := []streaming.TemplateID{{
		PackageID:  cfg.CIP56PackageID,
		ModuleName: "CIP56.Events",
		EntityName: "TokenTransferEvent",
	}}
	if cfg.UtilityRegistryPackageID != "" {
		ids = append(ids, streaming.TemplateID{
			PackageID:  cfg.UtilityRegistryPackageID,
			ModuleName: "Utility.Registry.App.V0.Model.Transfer",
			EntityName: "TransferOffer",
		})
	}
	if cfg.UtilityRegistryHoldingPackageID != "" {
		ids = append(ids, streaming.TemplateID{
			PackageID:  cfg.UtilityRegistryHoldingPackageID,
			ModuleName: "Utility.Registry.Holding.V0.Holding",
			EntityName: "Holding",
		})
	}
	return ids
}

// newRouter builds the chi router with standard middleware, a /health endpoint,
// and the private admin routes under /indexer/v1/admin.
//
// Currently the indexer exposes a single unauthenticated port intended for
// internal/trusted callers (backend services, ops tooling). A public read API
// with JWT authentication will be added in a future iteration on a separate
// route group. Until then, restrict network access to this port at the
// infrastructure level (firewall, private VPC, etc.).
func (s *Server) newRouter(svc indexerservice.Service, metrics *apphttp.HTTPMetrics, logger *zap.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(defaultMiddlewareTimeout))
	r.Use(apphttp.RequestMetricsMiddleware(metrics))

	r.Get(s.cfg.Monitoring.HealthCheckURL, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	indexerservice.RegisterPrivateRoutes(r, svc, logger)

	return r
}
