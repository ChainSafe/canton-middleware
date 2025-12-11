package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/canton"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/db"
	"github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/chainsafe/canton-middleware/pkg/relayer"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

var (
	configPath = flag.String("config", "config.yaml", "Path to configuration file")
)

func main() {
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger, err := config.NewLogger(cfg.Logging)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("Starting Canton-Ethereum Bridge Relayer")

	// Initialize database
	store, err := db.NewStore(cfg.Database.GetConnectionString())
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer store.Close()
	logger.Info("Database connection established")

	// Initialize Canton client
	cantonClient, err := canton.NewClient(&cfg.Canton, logger)
	if err != nil {
		logger.Fatal("Failed to initialize Canton client", zap.Error(err))
	}
	defer cantonClient.Close()

	// Initialize Ethereum client
	ethClient, err := ethereum.NewClient(&cfg.Ethereum, logger)
	if err != nil {
		logger.Fatal("Failed to initialize Ethereum client", zap.Error(err))
	}
	defer ethClient.Close()

	// Start relayer engine first so we can reference it in HTTP handlers
	ctx := context.Background()
	engine := relayer.NewEngine(cfg, cantonClient, ethClient, store, logger)
	if err := engine.Start(ctx); err != nil {
		logger.Fatal("Failed to start relayer engine", zap.Error(err))
	}
	defer engine.Stop()

	// Setup HTTP server for API and metrics
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// Health check endpoint (liveness)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Readiness endpoint - returns 503 until initial sync is complete
	r.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		if !engine.IsReady() {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("NOT_READY"))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("READY"))
	})

	// Metrics endpoint
	if cfg.Monitoring.Enabled {
		r.Handle("/metrics", promhttp.Handler())
		logger.Info("Metrics enabled", zap.Int("port", cfg.Monitoring.MetricsPort))
	}

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/transfers", handleGetTransfers(store, logger))
		r.Get("/transfers/{id}", handleGetTransfer(store, logger))
		r.Get("/status", handleGetStatus(logger))
	})

	// Start HTTP server
	serverAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	server := &http.Server{
		Addr:         serverAddr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("Starting HTTP server", zap.String("address", serverAddr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutdown signal received, gracefully shutting down...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Server shutdown error", zap.Error(err))
	}

	logger.Info("Relayer stopped")
}

func handleGetTransfers(store *db.Store, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		transfers, err := store.ListTransfers(100) // Default limit
		if err != nil {
			logger.Error("Failed to list transfers", zap.Error(err))
			http.Error(w, "Failed to list transfers", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{"transfers": transfers}); err != nil {
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
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(transfer); err != nil {
			logger.Error("Failed to encode response", zap.Error(err))
		}
	}
}

func handleGetStatus(logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{"status": "running"}); err != nil {
			logger.Error("Failed to encode response", zap.Error(err))
		}
	}
}
