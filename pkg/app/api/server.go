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

	apphttp "github.com/chainsafe/canton-middleware/pkg/app/http"
	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/client"
	cantontkn "github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/pkg/config"
	ethrpcminer "github.com/chainsafe/canton-middleware/pkg/ethrpc/miner"
	ethrpc "github.com/chainsafe/canton-middleware/pkg/ethrpc/service"
	ethrpcstore "github.com/chainsafe/canton-middleware/pkg/ethrpc/store"
	indexerclient "github.com/chainsafe/canton-middleware/pkg/indexer/client"
	"github.com/chainsafe/canton-middleware/pkg/keys"
	"github.com/chainsafe/canton-middleware/pkg/log"
	"github.com/chainsafe/canton-middleware/pkg/pgutil"
	"github.com/chainsafe/canton-middleware/pkg/reconciler"
	reconcilerstore "github.com/chainsafe/canton-middleware/pkg/reconciler/store"
	"github.com/chainsafe/canton-middleware/pkg/registry"
	"github.com/chainsafe/canton-middleware/pkg/token"
	tokenprovider "github.com/chainsafe/canton-middleware/pkg/token/provider"
	"github.com/chainsafe/canton-middleware/pkg/transfer"
	userservice "github.com/chainsafe/canton-middleware/pkg/user/service"
	"github.com/chainsafe/canton-middleware/pkg/userstore"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/uptrace/bun"
	"go.uber.org/zap"
)

const (
	defaultRequestTimeout = 60
	topologyCacheTTL      = 5 * time.Minute
	transferCacheTTL      = 2 * time.Minute
	transferCacheMaxSize  = 10000
)

// Server holds cfg to init the api server.
type Server struct {
	cfg *config.APIServer
}

type userKeyStore interface {
	GetUserKeyByCantonPartyID(ctx context.Context, decryptor userstore.KeyDecryptor, partyID string) ([]byte, error)
}

// NewServer initializes new api server.
func NewServer(cfg *config.APIServer) *Server {
	return &Server{cfg: cfg}
}

func (s *Server) Run() error {
	if s.cfg == nil {
		return fmt.Errorf("api server config is nil")
	}
	cfg := s.cfg

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger, err := log.NewLogger(cfg.Logging)
	if err != nil {
		return fmt.Errorf("setup logger: %w", err)
	}
	defer func() { _ = logger.Sync() }()

	logger.Info("Starting API server",
		zap.String("host", cfg.Server.Host),
		zap.Int("port", cfg.Server.Port),
	)

	masterKey, err := s.getMasterKey()
	if err != nil {
		return err
	}

	dbBun, err := pgutil.ConnectDB(cfg.Database)
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

	logger.Info("Connected to Canton")

	recStore := reconcilerstore.NewStore(dbBun)
	rec := reconciler.New(recStore, userStore, cantonClient.Token, logger)
	s.runInitialReconcile(ctx, rec, logger)

	stopReconcile := s.startPeriodicReconcile(rec, logger)
	// We will call stopReconcile explicitly after ServeAndWait returns for deterministic shutdown order.
	// Keep this defer as a safety net.
	defer stopReconcile()

	svcs, err := initServices(ctx, cfg, dbBun, cantonClient, cipher, logger)
	if err != nil {
		return err
	}

	if cfg.AcceptWorker != nil {
		ic, icErr := indexerclient.New(cfg.AcceptWorker.IndexerURL, nil)
		if icErr != nil {
			return fmt.Errorf("create accept worker indexer client: %w", icErr)
		}
		worker := transfer.NewAcceptWorker(
			cantonClient.Token,
			userStore,
			ic,
			cfg.AcceptWorker.PollInterval,
			logger,
		)
		go worker.Run(ctx)
		logger.Info("accept worker started",
			zap.String("indexer_url", cfg.AcceptWorker.IndexerURL),
			zap.Duration("poll_interval", cfg.AcceptWorker.PollInterval),
		)
	}

	router := s.setupRouter(svcs.evmStore, cantonClient, svcs.tokenService, svcs.regSvc, svcs.transferSvc, logger)

	err = apphttp.ServeAndWait(ctx, router, logger, cfg.Server)

	// Stop background work before deferred DB/client closes kick in.
	stopReconcile()

	return err
}

// buildTokenProvider constructs the token data provider according to the
// configured mode.  canton is the default; indexer reads from the indexer's
// pre-materialized HTTP API instead of issuing live gRPC ACS scans.
func buildTokenProvider(cfg *config.APIServer, cantonToken cantontkn.Token) (token.Provider, error) {
	switch cfg.TokenProvider.Mode {
	case config.TokenProviderIndexer:
		ic := cfg.TokenProvider.Indexer
		if ic == nil {
			return nil, fmt.Errorf("token_provider.indexer config is required when mode is %q", config.TokenProviderIndexer)
		}
		c, err := indexerclient.New(ic.URL, nil)
		if err != nil {
			return nil, fmt.Errorf("create indexer client: %w", err)
		}
		return tokenprovider.NewIndexer(c, ic.Instruments), nil
	default: // TokenProviderCanton
		return tokenprovider.NewCanton(cantonToken), nil
	}
}

type services struct {
	evmStore     ethrpc.Store
	tokenService *token.Service
	regSvc       userservice.Service
	transferSvc  transfer.Service
}

func initServices(
	ctx context.Context,
	cfg *config.APIServer,
	dbBun *bun.DB,
	cantonClient *canton.Client,
	cipher keys.KeyCipher,
	logger *zap.Logger,
) (*services, error) {
	userStore := userstore.NewStore(dbBun)

	topologyCache := userservice.NewTopologyCache(topologyCacheTTL)
	go topologyCache.Start(ctx)

	registrationService := userservice.NewService(
		userStore,
		cantonClient.Identity,
		cipher,
		logger,
		cfg.SkipCantonSigVerify,
		cfg.SkipWhitelistCheck,
		topologyCache,
	)

	tokenDataProvider, err := buildTokenProvider(cfg, cantonClient.Token)
	if err != nil {
		return nil, fmt.Errorf("build token provider: %w", err)
	}

	evmStore := ethrpcstore.NewStore(dbBun)

	transferCache := transfer.NewPreparedTransferCache(transferCacheTTL, transferCacheMaxSize)
	go transferCache.Start(ctx)

	if cfg.EthRPC.Enabled {
		m := ethrpcminer.New(evmStore, cfg.EthRPC.ChainID, cfg.EthRPC.GasLimit, cfg.EthRPC.MinerMaxTxsPerBlock, cfg.EthRPC.MinerInterval, logger)
		go m.Start(ctx)
	}

	return &services{
		evmStore:     evmStore,
		tokenService: token.NewTokenService(cfg.Token, tokenDataProvider, userStore, cantonClient.Token),
		regSvc:       userservice.NewLog(registrationService, logger),
		transferSvc:  transfer.NewLog(transfer.NewTransferService(cantonClient.Token, userStore, transferCache, tokenSymbols(cfg.Token)), logger),
	}, nil
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
	keyResolver := func(partyID string) (cantontkn.Signer, error) {
		privKey, err := keyStore.GetUserKeyByCantonPartyID(ctx, cipher.Decrypt, partyID)
		if err != nil {
			return nil, fmt.Errorf("key store lookup: %w", err)
		}
		if privKey == nil {
			return nil, fmt.Errorf("no signing key found for party %s", partyID)
		}
		return keys.CantonKeyPairFromPrivateKey(privKey)
	}

	client, err := canton.New(
		ctx,
		s.cfg.Canton,
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
	reconciler *reconciler.Reconciler,
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
	reconciler *reconciler.Reconciler,
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
	evmStore ethrpc.Store,
	cantonClient *canton.Client,
	tokenService *token.Service,
	registrationService userservice.Service,
	transferSvc transfer.Service,
	logger *zap.Logger,
) chi.Router {
	r := chi.NewRouter()

	// Middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(time.Second * defaultRequestTimeout))
	r.Use(apphttp.CORSMiddleware(s.cfg.CORSOrigins))

	// Health check
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Supported tokens metadata
	token.RegisterRoutes(r, tokenService, logger)

	// Registration endpoints
	userservice.RegisterRoutes(r, registrationService, logger)

	// Non-custodial transfer endpoints (prepare/execute)
	transfer.RegisterRoutes(r, transferSvc, logger)

	registryHandler := registry.NewHandler(cantonClient.Token, logger)
	r.Handle("/registry/transfer-instruction/v1/transfer-factory", registryHandler)
	logger.Info("Splice Registry API enabled",
		zap.String("path", "/registry/transfer-instruction/v1/transfer-factory"))

	// Ethereum JSON-RPC endpoints (if enabled)
	if s.cfg.EthRPC.Enabled {
		coreEthSvc := ethrpc.NewService(s.cfg.EthRPC, evmStore, tokenService)
		ethrpc.RegisterRoutes(r, ethrpc.NewLog(coreEthSvc, logger), s.cfg.EthRPC.RequestTimeout, logger)
	}

	return r
}

// tokenSymbols extracts the unique symbol strings from the token config.
func tokenSymbols(cfg *token.Config) []string {
	seen := make(map[string]bool, len(cfg.SupportedTokens))
	var symbols []string
	for _, tkn := range cfg.SupportedTokens {
		if !seen[tkn.Symbol] {
			seen[tkn.Symbol] = true
			symbols = append(symbols, tkn.Symbol)
		}
	}
	return symbols
}
