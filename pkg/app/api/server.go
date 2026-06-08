// SPDX-License-Identifier: Apache-2.0

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
	"github.com/chainsafe/canton-middleware/pkg/custodial"
	ethrpcminer "github.com/chainsafe/canton-middleware/pkg/ethrpc/miner"
	ethrpc "github.com/chainsafe/canton-middleware/pkg/ethrpc/service"
	ethrpcstore "github.com/chainsafe/canton-middleware/pkg/ethrpc/store"
	ethrpcsubmitter "github.com/chainsafe/canton-middleware/pkg/ethrpc/submitter"
	indexerclient "github.com/chainsafe/canton-middleware/pkg/indexer/client"
	"github.com/chainsafe/canton-middleware/pkg/keys"
	"github.com/chainsafe/canton-middleware/pkg/log"
	"github.com/chainsafe/canton-middleware/pkg/pgutil"
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
	"github.com/uptrace/bun"
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

	// The indexer is now a hard dependency: the token provider (when in
	// indexer mode), the accept worker, and the pending-offers list endpoint
	// all read from it. Build the HTTP client once and share — separate
	// instances would just open redundant idle connections to the same host.
	indexerClient, err := buildIndexerClient(cfg, reg)
	if err != nil {
		return err
	}

	// All long-lived goroutines — background workers and HTTP servers — run
	// under a single errgroup tied to gCtx. A signal (ctx) or any server error
	// cancels gCtx, unwinding every goroutine; g.Wait() then blocks until they
	// have all drained, so the deferred cantonClient/dbBun closes below never
	// race with in-flight worker calls.
	g, gCtx := errgroup.WithContext(ctx)

	svcs, err := initServices(gCtx, g, cfg, dbBun, userStore, cantonClient, indexerClient, cipher, reg, logger)
	if err != nil {
		// initServices may have already started workers on g (the caches, and on
		// later failures the miner/submitter). Cancel the group's context and wait
		// for them to exit before returning — otherwise they outlive the deferred
		// dbBun/cantonClient closes below and leak.
		stop()
		_ = g.Wait()
		return err
	}

	if cfg.AcceptWorker != nil {
		worker := custodial.NewAcceptWorker(
			cantonClient.Token,
			userStore,
			indexerClient,
			cfg.AcceptWorker.PollInterval,
			custodial.NewMetrics(reg),
			logger,
		)
		g.Go(func() error { return worker.Run(gCtx) })
		logger.Info("accept worker started",
			zap.Duration("poll_interval", cfg.AcceptWorker.PollInterval),
		)
	}

	router := s.setupRouter(svcs.evmStore, cantonClient, svcs.tokenService, svcs.regSvc, svcs.transferSvc, metrics, logger)

	s.registerServers(g, gCtx, router, logger)

	return g.Wait()
}

// buildIndexerClient creates the single indexer HTTP client used by every
// part of the api-server that talks to the indexer. The URL is read from
// token_provider.indexer.url, falling back to accept_worker.indexer_url, since
// both are aliases for the same indexer service in every deployment we ship —
// the dual configuration exists for historical reasons and is consolidated
// here so the rest of the code never has to pick between them.
//
// An indexer is now required (the pending-offers endpoint backs onto it), so
// startup fails fast if neither location is configured. The returned client is
// wrapped with metrics so all outbound indexer calls are observed.
func buildIndexerClient(cfg *config.APIServer, reg sharedmetrics.NamespacedRegisterer) (indexerclient.Client, error) {
	url := ""
	if cfg.TokenProvider != nil && cfg.TokenProvider.Indexer != nil {
		url = cfg.TokenProvider.Indexer.URL
	}
	if url == "" && cfg.AcceptWorker != nil {
		url = cfg.AcceptWorker.IndexerURL
	}
	if url == "" {
		return nil, fmt.Errorf("indexer URL is required: set token_provider.indexer.url or accept_worker.indexer_url")
	}
	c, err := indexerclient.New(url, nil)
	if err != nil {
		return nil, fmt.Errorf("create indexer client (%s): %w", url, err)
	}
	return indexerclient.NewInstrumentedClient(c, indexerclient.NewMetrics(reg)), nil
}

// buildTokenProvider constructs the token data provider according to the
// configured mode. canton is the default; indexer reads from the indexer's
// pre-materialized HTTP API instead of issuing live gRPC ACS scans. The
// indexer-mode branch reuses the shared client built by buildIndexerClient.
func buildTokenProvider(cfg *config.APIServer, cantonToken cantontkn.Token, indexerClient indexerclient.Client) (token.Provider, error) {
	if cfg.TokenProvider.Mode != config.TokenProviderIndexer {
		return tokenprovider.NewCanton(cantonToken), nil
	}
	if cfg.TokenProvider.Indexer == nil {
		return nil, fmt.Errorf("token_provider.indexer config is required when mode is %q", config.TokenProviderIndexer)
	}
	return tokenprovider.NewIndexer(indexerClient, cfg.TokenProvider.Indexer.Instruments), nil
}

type services struct {
	evmStore     ethrpc.Store
	tokenService *token.Service
	regSvc       userservice.Service
	transferSvc  transfer.Service
}

func initServices(
	gCtx context.Context,
	g *errgroup.Group,
	cfg *config.APIServer,
	dbBun *bun.DB,
	userStore userstore.Store,
	cantonClient *canton.Client,
	indexerClient indexerclient.Client,
	cipher keys.KeyCipher,
	reg sharedmetrics.NamespacedRegisterer,
	logger *zap.Logger,
) (*services, error) {
	topologyCache := userservice.NewTopologyCache(topologyCacheTTL)
	g.Go(func() error { return topologyCache.Start(gCtx) })

	registrationService := userservice.NewService(
		userStore,
		cantonClient.Identity,
		cipher,
		logger,
		cfg.SkipCantonSigVerify,
		cfg.SkipWhitelistCheck,
		topologyCache,
	)

	tokenDataProvider, err := buildTokenProvider(cfg, cantonClient.Token, indexerClient)
	if err != nil {
		return nil, fmt.Errorf("build token provider: %w", err)
	}

	evmStore := ethrpcstore.NewInstrumentedStore(
		ethrpcstore.NewStore(dbBun),
		ethrpcstore.NewStoreMetrics(reg),
	)

	transferCache := transfer.NewPreparedTransferCache(transferCacheTTL, transferCacheMaxSize)
	g.Go(func() error { return transferCache.Start(gCtx) })
	instrumentedCache := transfer.NewInstrumentedCache(transferCache, transfer.NewCacheMetrics(reg))

	tokenService := token.NewTokenService(cfg.Token, tokenDataProvider, userStore, cantonClient.Token)

	if cfg.EthRPC.Enabled {
		m := ethrpcminer.New(
			evmStore,
			cfg.EthRPC.ChainID, cfg.EthRPC.GasLimit,
			cfg.EthRPC.MinerMaxTxsPerBlock, cfg.EthRPC.MinerInterval,
			ethrpcminer.NewMetrics(reg),
			logger,
		)
		g.Go(func() error { return m.Start(gCtx) })

		// Async submitter: drives pending mempool entries → completed/failed by
		// calling Canton. SendRawTransaction returns the tx hash immediately
		// after the pending insert; this worker is what actually moves money.
		// Runs SubmitterConcurrency transfers in parallel; each Canton call is
		// bounded by a package-level timeout so a hung gRPC call can't drain
		// the pool.
		sub := ethrpcsubmitter.New(
			evmStore,
			tokenService,
			cfg.EthRPC.SubmitterInterval,
			cfg.EthRPC.SubmitterBatchSize,
			cfg.EthRPC.SubmitterConcurrency,
			ethrpcsubmitter.NewMetrics(reg),
			logger,
		)
		g.Go(func() error { return sub.Start(gCtx) })
	}

	transferSvc := transfer.NewTransferService(cantonClient.Token, userStore, instrumentedCache, cfg.Token, indexerClient)
	return &services{
		evmStore:     evmStore,
		tokenService: tokenService,
		regSvc:       userservice.NewLog(registrationService, logger),
		transferSvc:  transfer.NewLog(transferSvc, logger),
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

// registerServers adds the main HTTP server and, when monitoring is enabled,
// the metrics server to the shared errgroup. They run on gCtx alongside the
// background workers, so a failure in either server cancels gCtx and unwinds
// everything; the caller's g.Wait() surfaces the first error.
func (s *Server) registerServers(g *errgroup.Group, gCtx context.Context, router http.Handler, logger *zap.Logger) {
	g.Go(func() error {
		return apphttp.ServeAndWait(gCtx, router, logger, s.cfg.Server)
	})

	if s.cfg.Monitoring != nil && s.cfg.Monitoring.Enabled {
		r := chi.NewRouter()
		r.Use(middleware.Recoverer)
		r.Handle("/metrics", promhttp.Handler())

		g.Go(func() error {
			return apphttp.ServeAndWait(gCtx, r, logger, s.cfg.Monitoring.Server)
		})
	}
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
	r.Use(apphttp.CORSMiddleware(s.cfg.CORSOrigins))

	// Health check
	r.Get(s.cfg.Monitoring.HealthCheckURL, func(w http.ResponseWriter, _ *http.Request) {
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
