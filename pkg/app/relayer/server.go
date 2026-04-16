// Package relayer implements app.Runner for the relayer process.
package relayer

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	apphttp "github.com/chainsafe/canton-middleware/pkg/app/http"
	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/client"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/chainsafe/canton-middleware/pkg/log"
	"github.com/chainsafe/canton-middleware/pkg/pgutil"
	relayerengine "github.com/chainsafe/canton-middleware/pkg/relayer/engine"
	relayersvc "github.com/chainsafe/canton-middleware/pkg/relayer/service"
	relayerstore "github.com/chainsafe/canton-middleware/pkg/relayer/store"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

const defaultHTTPMiddlewareTimeout = 60 * time.Second

// Server holds configuration for the relayer process.
type Server struct {
	cfg *config.RelayerServer
}

// NewServer initializes a new relayer Server.
func NewServer(cfg *config.RelayerServer) *Server {
	return &Server{cfg: cfg}
}

// Run starts the relayer engine and the operational HTTP server.
// It blocks until an OS shutdown signal is received or a fatal server error occurs.
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

	logger.Info("Starting Canton-Ethereum Bridge Relayer")

	db, err := pgutil.ConnectDB(cfg.Database)
	if err != nil {
		return fmt.Errorf("connect relayer db: %w", err)
	}
	defer func() { _ = db.Close() }()
	logger.Info("Database connection established")

	// Metrics — registered once, injected into engine and store layers.
	reg := prometheus.DefaultRegisterer
	engineMetrics := relayerengine.NewMetrics(reg)
	storeMetrics := relayerstore.NewStoreMetrics(reg)

	pgStore := relayerstore.NewStore(db)
	store := relayerstore.NewInstrumentedStore(pgStore, storeMetrics)

	cantonClient, err := canton.New(ctx, cfg.Canton, canton.WithLogger(logger))
	if err != nil {
		return fmt.Errorf("initialize canton client: %w", err)
	}
	defer func() { _ = cantonClient.Close() }()

	ethClient, err := ethereum.NewClient(cfg.Ethereum, logger)
	if err != nil {
		return fmt.Errorf("initialize ethereum client: %w", err)
	}
	defer ethClient.Close()

	engine := relayerengine.NewEngine(cfg.Bridge, cantonClient.Bridge, ethClient, store, engineMetrics, logger)

	if err = engine.Start(ctx); err != nil {
		return fmt.Errorf("start relayer engine: %w", err)
	}
	defer engine.Stop()

	router := s.newRouter(store, engine, logger)

	return s.serveAll(ctx, router, logger)
}

// serveAll runs the main HTTP server and, when monitoring is enabled,
// the metrics server. Both share an errgroup context: if either server
// fails the other is cancelled and the first error is returned.
func (s *Server) serveAll(ctx context.Context, router http.Handler, logger *zap.Logger) error {
	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return apphttp.ServeAndWait(gCtx, router, logger, s.cfg.Server)
	})

	if s.cfg.Monitoring != nil && s.cfg.Monitoring.Enabled {
		if s.cfg.Monitoring.Server == nil {
			return fmt.Errorf("monitoring is enabled but server config is nil")
		}

		r := chi.NewRouter()
		r.Use(middleware.Recoverer)
		r.Handle("/metrics", promhttp.Handler())

		g.Go(func() error {
			return apphttp.ServeAndWait(gCtx, r, logger, s.cfg.Monitoring.Server)
		})
	}

	return g.Wait()
}

func (*Server) newRouter(store relayersvc.Store, engine *relayerengine.Engine, logger *zap.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(defaultHTTPMiddlewareTimeout))
	r.Use(middleware.Logger)

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	svc := relayersvc.NewLog(relayersvc.NewService(store), logger)
	relayersvc.RegisterRoutes(r, svc, engine, logger)

	return r
}
