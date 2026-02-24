// Package api implements app.Runner for the API server process.
package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
	"github.com/chainsafe/canton-middleware/pkg/app/httpserver"
	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/client"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc"
	"github.com/chainsafe/canton-middleware/pkg/keys"
	"github.com/chainsafe/canton-middleware/pkg/registration"
	"github.com/chainsafe/canton-middleware/pkg/registry"
	"github.com/chainsafe/canton-middleware/pkg/service"

	"go.uber.org/zap"
)

// Server holds cfg to init the api server.
type Server struct {
	cfg *config.APIServerConfig
}

// NewServer initializes new api server.
func NewServer(cfg *config.APIServerConfig) *Server {
	return &Server{cfg: cfg}
}

func (s *Server) Run() error {
	if s.cfg == nil {
		return fmt.Errorf("api server config is nil")
	}
	cfg := s.cfg

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger, err := config.NewLogger(cfg.Logging)
	if err != nil {
		return fmt.Errorf("setup logger: %w", err)
	}
	defer func() { _ = logger.Sync() }()

	logger.Info("Starting API server",
		zap.String("host", cfg.Server.Host),
		zap.Int("port", cfg.Server.Port),
	)

	db, err := s.openDB(logger)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	keyStore, err := s.openKeyStore(db)
	if err != nil {
		return err
	}
	logger.Info("Custodial key management initialized")

	cantonClient, err := s.openCantonClient(ctx, keyStore, logger)
	if err != nil {
		return err
	}
	defer func() { _ = cantonClient.Close() }()

	logger.Info("Connected to Canton", zap.String("rpc_url", cfg.Canton.RPCURL))

	reconciler := apidb.NewReconciler(db, cantonClient.Token, logger)
	s.runInitialReconcile(ctx, reconciler, logger)

	stopReconcile := s.startPeriodicReconcile(reconciler, logger)
	// We will call stopReconcile explicitly after ServeAndWait returns for deterministic shutdown order.
	// Keep this defer as a safety net.
	defer stopReconcile()

	tokenService := service.NewTokenService(cfg, db, cantonClient.Token, logger)

	ethHandler, err := s.maybeCreateEthHandler(db, tokenService, logger)
	if err != nil {
		return err
	}

	mux := s.newRouter(db, cantonClient, keyStore, ethHandler, logger)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	httpServer := newHTTPServer(addr, mux, cfg.Server)

	err = httpserver.ServeAndWait(ctx, logger, httpServer, cfg.Shutdown.Timeout)

	// Stop background work before deferred DB/client closes kick in.
	stopReconcile()

	return err
}

func (s *Server) openDB(logger *zap.Logger) (*apidb.Store, error) {
	dbConnStr := s.cfg.Database.GetConnectionString()
	db, err := apidb.NewStore(dbConnStr)
	if err != nil {
		return nil, fmt.Errorf("connect db: %w", err)
	}
	logger.Info("Connected to database",
		zap.String("host", s.cfg.Database.Host),
		zap.String("database", s.cfg.Database.Database),
	)
	return db, nil
}

func (s *Server) openKeyStore(db *apidb.Store) (*keys.PostgresKeyStore, error) {
	masterKeyStr := os.Getenv(s.cfg.KeyManagement.MasterKeyEnv)
	if masterKeyStr == "" {
		return nil, fmt.Errorf(
			"canton master key not set: env=%s (hint: openssl rand -base64 32)",
			s.cfg.KeyManagement.MasterKeyEnv,
		)
	}

	masterKey, err := keys.MasterKeyFromBase64(masterKeyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid canton master key: %w", err)
	}

	keyStore, err := keys.NewPostgresKeyStore(db, masterKey)
	if err != nil {
		return nil, fmt.Errorf("create keystore: %w", err)
	}

	return keyStore, nil
}

func (s *Server) openCantonClient(
	ctx context.Context,
	keyStore *keys.PostgresKeyStore,
	logger *zap.Logger,
) (*canton.Client, error) {
	keyResolver := func(partyID string) (token.Signer, error) {
		return keys.ResolveKeyPairByPartyID(keyStore, partyID)
	}

	client, err := canton.NewFromAppConfig(
		ctx,
		&s.cfg.Canton,
		canton.WithLogger(logger),
		canton.WithKeyResolver(keyResolver),
	)
	if err != nil {
		return nil, fmt.Errorf("create canton client: %w", err)
	}
	return client, nil
}

func (s *Server) runInitialReconcile(
	ctx context.Context,
	reconciler *apidb.Reconciler,
	logger *zap.Logger,
) {
	if s.cfg.Reconciliation.InitialTimeout <= 0 {
		return
	}

	logger.Info("Running initial balance reconciliation",
		zap.Duration("timeout", s.cfg.Reconciliation.InitialTimeout),
	)

	startupCtx, cancel := context.WithTimeout(ctx, s.cfg.Reconciliation.InitialTimeout)
	defer cancel()

	if err := reconciler.ReconcileAll(startupCtx); err != nil {
		logger.Warn("Initial reconciliation failed (will retry periodically)", zap.Error(err))
		return
	}

	logger.Info("Initial balance reconciliation completed")
}

func (s *Server) startPeriodicReconcile(
	reconciler *apidb.Reconciler,
	logger *zap.Logger,
) func() {
	if s.cfg.Reconciliation.Interval <= 0 {
		return func() {}
	}

	logger.Info("Starting periodic reconciliation", zap.Duration("interval", s.cfg.Reconciliation.Interval))
	reconciler.StartPeriodicReconciliation(s.cfg.Reconciliation.Interval)

	// Return stopper for deterministic shutdown ordering.
	return func() { reconciler.Stop() }
}

func (s *Server) maybeCreateEthHandler(
	db *apidb.Store,
	tokenService *service.TokenService,
	logger *zap.Logger,
) (http.Handler, error) {
	if !s.cfg.EthRPC.Enabled {
		return nil, nil
	}

	ethSrv, err := ethrpc.NewServer(s.cfg, db, tokenService, logger)
	if err != nil {
		return nil, fmt.Errorf("create eth json-rpc server: %w", err)
	}

	logger.Info("Ethereum JSON-RPC endpoint enabled",
		zap.String("path", "/eth"),
		zap.Uint64("chain_id", s.cfg.EthRPC.ChainID),
		zap.String("token_address", s.cfg.EthRPC.TokenAddress),
	)

	return ethSrv, nil
}

func (s *Server) newRouter(
	db *apidb.Store,
	cantonClient *canton.Client,
	keyStore *keys.PostgresKeyStore,
	ethHandler http.Handler,
	logger *zap.Logger,
) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
			logger.Warn("failed to write health check response", zap.Error(err))
		}
	})

	registrationHandler := registration.NewHandler(s.cfg, db, cantonClient.Identity, keyStore, logger)
	mux.Handle("/register", registrationHandler)
	logger.Info("Registration endpoint enabled", zap.String("path", "/register"))

	registryHandler := registry.NewHandler(cantonClient.Token, logger)
	mux.Handle("/registry/transfer-instruction/v1/transfer-factory", registryHandler)
	logger.Info("Splice Registry API enabled",
		zap.String("path", "/registry/transfer-instruction/v1/transfer-factory"))

	if ethHandler != nil {
		mux.Handle("/eth", ethHandler)
	}

	return mux
}

func newHTTPServer(addr string, handler http.Handler, sc config.ServerConfig) *http.Server {
	return &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  sc.ReadTimeout,
		WriteTimeout: sc.WriteTimeout,
		IdleTimeout:  sc.IdleTimeout,
	}
}
