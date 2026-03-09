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
	"github.com/chainsafe/canton-middleware/pkg/pgutil"
	"github.com/chainsafe/canton-middleware/pkg/relayer"
	relayersvc "github.com/chainsafe/canton-middleware/pkg/relayer/service"
	relayerstore "github.com/chainsafe/canton-middleware/pkg/relayer/store"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

const defaultHTTPMiddlewareTimeout = 60 * time.Second

// Server holds configuration for the relayer process.
type Server struct {
	cfg *config.Config
}

// NewServer initializes a new relayer Server.
func NewServer(cfg *config.Config) *Server {
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

	logger, err := config.NewLogger(cfg.Logging)
	if err != nil {
		return fmt.Errorf("create logger: %w", err)
	}
	defer func() { _ = logger.Sync() }()

	logger.Info("Starting Canton-Ethereum Bridge Relayer")

	db, err := pgutil.ConnectDB(&cfg.Database)
	if err != nil {
		return fmt.Errorf("connect relayer db: %w", err)
	}
	defer func() { _ = db.Close() }()
	logger.Info("Database connection established")

	store := relayerstore.NewStore(db)

	cantonClient, err := canton.NewFromAppConfig(ctx, &cfg.Canton, canton.WithLogger(logger))
	if err != nil {
		return fmt.Errorf("initialize canton client: %w", err)
	}
	defer func() { _ = cantonClient.Close() }()

	ethClient, err := ethereum.NewClient(&cfg.Ethereum, logger)
	if err != nil {
		return fmt.Errorf("initialize ethereum client: %w", err)
	}
	ethClient.Close()

	engine := relayer.NewEngine(cfg, cantonClient.Bridge, ethClient, store, logger)

	if err = engine.Start(ctx); err != nil {
		return fmt.Errorf("start relayer engine: %w", err)
	}
	defer engine.Stop()

	router := s.newRouter(store, engine, logger)

	return apphttp.ServeAndWait(ctx, router, logger, &cfg.Server)
}

func (s *Server) newRouter(store relayer.BridgeStore, engine *relayer.Engine, logger *zap.Logger) http.Handler {
	cfg := s.cfg

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

	if cfg.Monitoring.Enabled {
		r.Handle("/metrics", promhttp.Handler())
		logger.Info("Metrics enabled", zap.String("path", "/metrics"))
	}

	svc := relayersvc.NewLog(relayersvc.NewService(store), logger)
	relayersvc.RegisterRoutes(r, svc, engine, logger)

	return r
}
