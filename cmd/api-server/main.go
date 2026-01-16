package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
	"github.com/chainsafe/canton-middleware/pkg/canton"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc"
	"github.com/chainsafe/canton-middleware/pkg/rpc"
	"github.com/chainsafe/canton-middleware/pkg/service"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.LoadAPIServer(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Setup logger
	logger, err := setupLogger(cfg.Logging.Level, cfg.Logging.Format)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("Starting ERC-20 API Server",
		zap.String("config", *configPath),
		zap.String("host", cfg.Server.Host),
		zap.Int("port", cfg.Server.Port))

	// Connect to database
	dbConnStr := cfg.Database.GetConnectionString()
	db, err := apidb.NewStore(dbConnStr)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer db.Close()
	logger.Info("Connected to database",
		zap.String("host", cfg.Database.Host),
		zap.String("database", cfg.Database.Database))

	// Create Canton client
	cantonClient, err := canton.NewClient(&cfg.Canton, logger)
	if err != nil {
		logger.Fatal("Failed to create Canton client", zap.Error(err))
	}
	defer cantonClient.Close()
	logger.Info("Connected to Canton",
		zap.String("rpc_url", cfg.Canton.RPCURL))

	// Create and start reconciler for balance cache
	reconciler := apidb.NewReconciler(db, cantonClient, logger)

	// Run initial reconciliation on startup
	logger.Info("Running initial balance reconciliation...",
		zap.Duration("timeout", cfg.Reconciliation.InitialTimeout))
	startupCtx, startupCancel := context.WithTimeout(context.Background(), cfg.Reconciliation.InitialTimeout)
	if err := reconciler.ReconcileAll(startupCtx); err != nil {
		logger.Warn("Initial reconciliation failed (will retry periodically)", zap.Error(err))
	} else {
		logger.Info("Initial balance reconciliation completed")
	}
	startupCancel()

	// Start periodic reconciliation
	logger.Info("Starting periodic reconciliation", zap.Duration("interval", cfg.Reconciliation.Interval))
	reconciler.StartPeriodicReconciliation(cfg.Reconciliation.Interval)
	defer reconciler.Stop()

	// Create shared token service
	tokenService := service.NewTokenService(cfg, db, cantonClient, logger)

	// Create RPC server
	rpcServer := rpc.NewServer(cfg, db, cantonClient, logger)

	// Create HTTP mux for multiple endpoints
	mux := http.NewServeMux()
	mux.Handle("/rpc", rpcServer)
	mux.Handle("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))

	// Create Ethereum JSON-RPC server if enabled
	if cfg.EthRPC.Enabled {
		ethServer, err := ethrpc.NewServer(cfg, db, tokenService, logger)
		if err != nil {
			logger.Fatal("Failed to create Eth JSON-RPC server", zap.Error(err))
		}
		mux.Handle("/eth", ethServer)
		logger.Info("Ethereum JSON-RPC endpoint enabled at /eth",
			zap.Uint64("chain_id", cfg.EthRPC.ChainID),
			zap.String("token_address", cfg.EthRPC.TokenAddress))
	}

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		logger.Info("HTTP server listening", zap.String("address", addr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("HTTP server error", zap.Error(err))
		}
	}()

	// Wait for shutdown signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Info("Shutting down server...", zap.Duration("timeout", cfg.Shutdown.Timeout))

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Shutdown.Timeout)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("Server shutdown error", zap.Error(err))
	}

	logger.Info("Server stopped")
}

// setupLogger creates a configured zap logger
func setupLogger(level, format string) (*zap.Logger, error) {
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		// Default to InfoLevel if parsing fails or level is invalid
		_, _ = fmt.Fprintf(os.Stderr, "Invalid log level, defaulting to InfoLevel. {%v}", zap.Error(err))
		zapLevel = zapcore.InfoLevel
	}

	var cfg zap.Config
	if format == "json" {
		cfg = zap.NewProductionConfig()
	} else {
		cfg = zap.NewDevelopmentConfig()
	}
	cfg.Level = zap.NewAtomicLevelAt(zapLevel)

	return cfg.Build()
}
