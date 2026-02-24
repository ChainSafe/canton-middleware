// Package api implements app.Runner for the API server process.
package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
	apphttp "github.com/chainsafe/canton-middleware/pkg/app/http"
	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/client"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc"
	"github.com/chainsafe/canton-middleware/pkg/keys"
	"github.com/chainsafe/canton-middleware/pkg/pgutil"
	reconcilerpkg "github.com/chainsafe/canton-middleware/pkg/reconciler"
	tokenservice "github.com/chainsafe/canton-middleware/pkg/token/service"
	userservice "github.com/chainsafe/canton-middleware/pkg/user/service"
	"github.com/chainsafe/canton-middleware/pkg/userstore"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

const defaultRequestTimeout = 60

// Server holds cfg to init the api server.
type Server struct {
	cfg *config.APIServerConfig
}

type userKeyStore interface {
	GetUserKeyByCantonPartyID(ctx context.Context, decryptor userstore.KeyDecryptor, partyID string) ([]byte, error)
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

	masterKey, err := s.getMasterKey()
	if err != nil {
		return err
	}

	dbBun, err := pgutil.ConnectDB(&cfg.Database)
	if err != nil {
		return err
	}
	defer dbBun.Close()

	userStore := userstore.NewStore(dbBun)
	cipher := keys.NewMasterKeyCipher(masterKey)

	cantonClient, err := s.openCantonClient(ctx, userStore, cipher, logger)
	if err != nil {
		return err
	}
	defer func() { _ = cantonClient.Close() }()

	logger.Info("Connected to Canton", zap.String("rpc_url", cfg.Canton.RPCURL))

	rec := reconcilerpkg.New(db, userStore, cantonClient.Token, logger)
	s.runInitialReconcile(ctx, rec, logger)

	stopReconcile := s.startPeriodicReconcile(rec, logger)
	// We will call stopReconcile explicitly after ServeAndWait returns for deterministic shutdown order.
	// Keep this defer as a safety net.
	defer stopReconcile()

	registrationService := userservice.NewService(
		userStore,
		cantonClient.Identity,
		cipher,
		logger,
		os.Getenv("SKIP_CANTON_SIG_VERIFY") == "true", // TODO: populate in config
	)

	tokenService := tokenservice.NewTokenService(cfg, db, userStore, cantonClient.Token, logger)

	router := s.setupRouter(db, tokenService, userservice.NewLog(registrationService, logger), logger)

	err = apphttp.ServeAndWait(ctx, router, logger, &cfg.Server)

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

func (s *Server) getMasterKey() ([]byte, error) {
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
	return masterKey, nil
}

func (s *Server) openCantonClient(
	ctx context.Context,
	keyStore userKeyStore,
	cipher keys.KeyCipher,
	logger *zap.Logger,
) (*canton.Client, error) {
	keyResolver := func(partyID string) (token.Signer, error) {
		privKey, err := keyStore.GetUserKeyByCantonPartyID(ctx, cipher.Decrypt, partyID)
		if err != nil {
			return nil, fmt.Errorf("key store lookup: %w", err)
		}
		if privKey == nil {
			return nil, fmt.Errorf("no signing key found for party %s", partyID)
		}
		return keys.CantonKeyPairFromPrivateKey(privKey)
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
	reconciler *reconcilerpkg.Reconciler,
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
	reconciler *reconcilerpkg.Reconciler,
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

func (s *Server) setupRouter(
	db *apidb.Store,
	tokenService *tokenservice.TokenService,
	registrationService userservice.Service,
	logger *zap.Logger,
) chi.Router {
	r := chi.NewRouter()

	// Middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(time.Second * defaultRequestTimeout))

	// Health check
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Registration endpoints
	userservice.RegisterRoutes(r, registrationService, logger)

	// Ethereum JSON-RPC endpoints (if enabled)
	if s.cfg.EthRPC.Enabled {
		ethHandler, err := s.createEthHandler(db, tokenService, logger)
		if err != nil {
			logger.Error("Failed to create eth handler", zap.Error(err))
		} else {
			r.Mount("/eth", ethHandler)
		}
	}

	return r
}

func (s *Server) createEthHandler(
	db *apidb.Store,
	tokenService *tokenservice.TokenService,
	logger *zap.Logger,
) (http.Handler, error) {
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
