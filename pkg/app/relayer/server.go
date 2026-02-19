// Package relayer implements app.Runner for the relayer process.
package relayer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
	"github.com/chainsafe/canton-middleware/pkg/app/httpserver"
	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/client"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/db"
	"github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/chainsafe/canton-middleware/pkg/relayer"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// TODO: take these from config
const (
	defaultGracefulShutdownTimeout = 30 * time.Second
	defaultHTTPMiddlewareTimeout   = 60 * time.Second
	defaultHTTPReadTimeout         = 15 * time.Second
	defaultHTTPWriteTimeout        = 15 * time.Second
	defaultHTTPIdleTimeout         = 60 * time.Second

	defaultLimitForListTransfer = 100
)

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

	store, err := db.NewStore(cfg.Database.GetConnectionString())
	if err != nil {
		return fmt.Errorf("connect relayer db: %w", err)
	}
	defer func() { _ = store.Close() }()
	logger.Info("Database connection established")

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

	apiStore, cleanupAPIDB, err := s.maybeOpenAPIDB(logger)
	if err != nil {
		apiStore = nil
	}
	if cleanupAPIDB != nil {
		defer cleanupAPIDB()
	}

	engine := relayer.NewEngine(cfg, cantonClient.Bridge, ethClient, store, logger)
	if apiStore != nil {
		engine.SetAPIDB(apiStore)
	}

	if err := engine.Start(ctx); err != nil {
		return fmt.Errorf("start relayer engine: %w", err)
	}
	defer engine.Stop()

	router := s.newRouter(store, engine, logger)

	serverAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	httpServer := newHTTPServer(serverAddr, router)

	return httpserver.ServeAndWait(ctx, logger, httpServer, defaultGracefulShutdownTimeout)
}

func (s *Server) maybeOpenAPIDB(logger *zap.Logger) (*apidb.Store, func(), error) {
	apiDBConnStr := s.cfg.Database.GetAPIConnectionString()
	if apiDBConnStr == "" {
		return nil, nil, nil
	}

	apiStore, err := apidb.NewStore(apiDBConnStr)
	if err != nil {
		logger.Warn("Failed to connect to API database (balance cache disabled)", zap.Error(err))
		return nil, nil, err
	}

	logger.Info("API database connection established (balance cache enabled)")
	cleanup := func() { _ = apiStore.Close() }
	return apiStore, cleanup, nil
}

func (s *Server) newRouter(store *db.Store, engine *relayer.Engine, logger *zap.Logger) http.Handler {
	cfg := s.cfg

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(defaultHTTPMiddlewareTimeout))

	// NOTE: chi's middleware.Logger logs to stdlib.
	// Keep it temporarily if access logs are useful; replace with zap-based middleware later.
	r.Use(middleware.Logger)

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	r.Get("/ready", func(w http.ResponseWriter, _ *http.Request) {
		if !engine.IsReady() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("NOT_READY"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("READY"))
	})

	if cfg.Monitoring.Enabled {
		r.Handle("/metrics", promhttp.Handler())
		logger.Info("Metrics enabled", zap.String("path", "/metrics"))
	}

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/transfers", handleGetTransfers(store, logger))
		r.Get("/transfers/{id}", handleGetTransfer(store, logger))
		r.Get("/status", handleGetStatus(logger))
	})

	return r
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  defaultHTTPReadTimeout,
		WriteTimeout: defaultHTTPWriteTimeout,
		IdleTimeout:  defaultHTTPIdleTimeout,
	}
}

func handleGetTransfers(store *db.Store, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		transfers, err := store.ListTransfers(defaultLimitForListTransfer)
		if err != nil {
			logger.Error("Failed to list transfers", zap.Error(err))
			http.Error(w, "failed to list transfers", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"transfers": transfers}); err != nil {
			logger.Error("Failed to encode response", zap.Error(err))
		}
	}
}

func handleGetTransfer(store *db.Store, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		transfer, err := store.GetTransfer(id)
		if err != nil {
			logger.Error("Failed to get transfer", zap.Error(err), zap.String("id", id))
			http.Error(w, "transfer not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(transfer); err != nil {
			logger.Error("Failed to encode response", zap.Error(err))
		}
	}
}

func handleGetStatus(logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"status": "running"}); err != nil {
			logger.Error("Failed to encode response", zap.Error(err))
		}
	}
}
