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

	sharedmetrics "github.com/chainsafe/canton-middleware/internal/metrics"
	apphttp "github.com/chainsafe/canton-middleware/pkg/app/http"
	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/client"
	cantontkn "github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/pkg/config"
	ethrpcminer "github.com/chainsafe/canton-middleware/pkg/ethrpc/miner"
	ethrpc "github.com/chainsafe/canton-middleware/pkg/ethrpc/service"
	ethrpcstore "github.com/chainsafe/canton-middleware/pkg/ethrpc/store"
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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
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

	// Metrics — registered once, injected into store wrappers and router middleware.
	reg := sharedmetrics.WithNamespace(prometheus.DefaultRegisterer, "api_server")

	userStore := userstore.NewInstrumentedStore(
		userstore.NewStore(dbBun),
		userstore.NewStoreMetrics(reg),
	)
	cipher := keys.NewMasterKeyCipher(masterKey)

	metrics := apphttp.NewHTTPMetrics(reg)

	cantonClient, err := s.openCantonClient(ctx, userStore, cipher, reg, logger)
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

	topologyCache := userservice.NewTopologyCache(topologyCacheTTL)
	go topologyCache.Start(ctx)

	registrationService := userservice.NewService(
		userStore,
		cantonClient.Identity,
		cipher,
		logger,
		cfg.SkipCantonSigVerify,
		topologyCache,
	)

	tokenDataProvider := tokenprovider.NewCanton(cantonClient.Token)
	tokenService := token.NewTokenService(cfg.Token, tokenDataProvider, userStore, cantonClient.Token)
	evmStore := ethrpcstore.NewInstrumentedStore(
		ethrpcstore.NewStore(dbBun),
		ethrpcstore.NewStoreMetrics(reg),
	)

	transferCache := transfer.NewPreparedTransferCache(transferCacheTTL, transferCacheMaxSize)
	go transferCache.Start(ctx)
	instrumentedCache := transfer.NewInstrumentedCache(transferCache, transfer.NewCacheMetrics(reg))
	transferSvc := transfer.NewTransferService(cantonClient.Token, userStore, instrumentedCache, tokenSymbols(cfg.Token))
	regSvcLog := userservice.NewLog(registrationService, logger)
	transferSvcLog := transfer.NewLog(transferSvc, logger)

	s.startEthRPCMinerIfEnabled(ctx, evmStore, reg, logger)

	router := s.setupRouter(evmStore, cantonClient, tokenService, regSvcLog, transferSvcLog, metrics, logger)

	err = s.serveAll(ctx, router, logger)

	// Stop background work before deferred DB/client closes kick in.
	stopReconcile()

	return err
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
	reg sharedmetrics.NamespacedRegisterer,
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
		canton.WithPrometheusRegisterer(reg),
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

func (s *Server) startEthRPCMinerIfEnabled(
	ctx context.Context,
	evmStore *ethrpcstore.InstrumentedStore,
	reg sharedmetrics.NamespacedRegisterer,
	logger *zap.Logger,
) {
	if !s.cfg.EthRPC.Enabled {
		return
	}
	m := ethrpcminer.New(
		evmStore,
		s.cfg.EthRPC.ChainID, s.cfg.EthRPC.GasLimit,
		s.cfg.EthRPC.MinerMaxTxsPerBlock, s.cfg.EthRPC.MinerInterval,
		ethrpcminer.NewMetrics(reg),
		logger,
	)
	go m.Start(ctx)
}

// serveAll runs the main HTTP server and, when monitoring is enabled,
// the metrics server. Both share an errgroup context: if either server
// fails the other is canceled and the first error is returned.
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

func (s *Server) setupRouter(
	evmStore ethrpc.Store,
	cantonClient *canton.Client,
	tokenService *token.Service,
	registrationService userservice.Service,
	transferSvc transfer.Service,
	metrics *apphttp.HTTPMetrics,
	logger *zap.Logger,
) chi.Router {
	r := chi.NewRouter()

	// Middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(time.Second * defaultRequestTimeout))
	r.Use(apphttp.RequestMetricsMiddleware(metrics))

	// Health check
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

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
