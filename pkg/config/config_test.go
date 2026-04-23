package config

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestLoadDefaultConfigFiles(t *testing.T) {
	setDefaultConfigEnv(t)

	t.Run("api docker", func(t *testing.T) {
		if _, err := LoadAPIServer(defaultConfigPath(t, "config.api-server.docker.yaml")); err != nil {
			t.Fatalf("load api docker config: %v", err)
		}
	})

	t.Run("api local-devnet", func(t *testing.T) {
		if _, err := LoadAPIServer(defaultConfigPath(t, "config.api-server.local-devnet.yaml")); err != nil {
			t.Fatalf("load api local-devnet config: %v", err)
		}
	})

	t.Run("relayer docker", func(t *testing.T) {
		if _, err := LoadRelayerServer(defaultConfigPath(t, "config.relayer.docker.yaml")); err != nil {
			t.Fatalf("load relayer docker config: %v", err)
		}
	})

	t.Run("relayer local-devnet", func(t *testing.T) {
		if _, err := LoadRelayerServer(defaultConfigPath(t, "config.relayer.local-devnet.yaml")); err != nil {
			t.Fatalf("load relayer local-devnet config: %v", err)
		}
	})

	t.Run("api mainnet", func(t *testing.T) {
		if _, err := LoadAPIServer(defaultConfigPath(t, "config.api-server.mainnet.yaml")); err != nil {
			t.Fatalf("load api mainnet config: %v", err)
		}
	})

	t.Run("relayer mainnet", func(t *testing.T) {
		if _, err := LoadRelayerServer(defaultConfigPath(t, "config.relayer.mainnet.yaml")); err != nil {
			t.Fatalf("load relayer mainnet config: %v", err)
		}
	})
}

func TestLoadConfig_EnvSubstitution(t *testing.T) {
	t.Setenv("TEST_API_DB_URL", "postgres://postgres:pass@localhost:5432/erc20_api")
	t.Setenv("TEST_API_DOMAIN_ID", "global-domain::1220test")
	t.Setenv("TEST_API_ISSUER", "issuer::1220test")
	t.Setenv("TEST_API_CLIENT_ID", "env-client-id")
	t.Setenv("TEST_API_CLIENT_SECRET", "env-client-secret")

	cfg, err := LoadAPIServer(testConfigPath(t, "env-substitution.api.yaml"))
	if err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}

	if cfg.Database.URL != "postgres://postgres:pass@localhost:5432/erc20_api" {
		t.Fatalf("expected env-expanded database.url, got %q", cfg.Database.URL)
	}
	if cfg.Canton.DomainID != "global-domain::1220test" {
		t.Fatalf("expected env-expanded canton.domain_id, got %q", cfg.Canton.DomainID)
	}
	if cfg.Canton.IssuerParty != "issuer::1220test" {
		t.Fatalf("expected env-expanded canton.issuer_party, got %q", cfg.Canton.IssuerParty)
	}
	if cfg.Canton.Ledger.Auth.ClientID != "env-client-id" {
		t.Fatalf("expected env-expanded auth.client_id, got %q", cfg.Canton.Ledger.Auth.ClientID)
	}
	if cfg.Canton.Ledger.Auth.ClientSecret != "env-client-secret" {
		t.Fatalf("expected env-expanded auth.client_secret, got %q", cfg.Canton.Ledger.Auth.ClientSecret)
	}
}

func TestLoadConfig_MissingEnvVariable(t *testing.T) {
	_, err := LoadAPIServer(testConfigPath(t, "missing-env.api.yaml"))
	if err == nil {
		t.Fatal("expected validation error after empty env expansion, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "required") {
		t.Fatalf("expected required validation error, got: %v", err)
	}
}

func TestLoadAPIServer_AppliesDefaults(t *testing.T) {
	cfg, err := LoadAPIServer(testConfigPath(t, "minimal.api.yaml"))
	if err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}

	if cfg.Server.ReadTimeout != 15*time.Second {
		t.Fatalf("read_timeout default mismatch: got %s", cfg.Server.ReadTimeout)
	}
	if cfg.Server.WriteTimeout != 15*time.Second {
		t.Fatalf("write_timeout default mismatch: got %s", cfg.Server.WriteTimeout)
	}
	if cfg.Server.IdleTimeout != 60*time.Second {
		t.Fatalf("idle_timeout default mismatch: got %s", cfg.Server.IdleTimeout)
	}
	if cfg.Server.ShutdownTimeout != 30*time.Second {
		t.Fatalf("shutdown_timeout default mismatch: got %s", cfg.Server.ShutdownTimeout)
	}

	if cfg.Database.Timeout != 10 {
		t.Fatalf("database.timeout default mismatch: got %d", cfg.Database.Timeout)
	}
	if cfg.Database.PoolSize != 10 {
		t.Fatalf("database.pool_size default mismatch: got %d", cfg.Database.PoolSize)
	}

	if cfg.Canton.Ledger.LedgerID != "" {
		t.Fatalf("ledger_id default mismatch: got %q", cfg.Canton.Ledger.LedgerID)
	}
	if cfg.Canton.Ledger.MaxMessageSize != 52428800 {
		t.Fatalf("max_inbound_message_size default mismatch: got %d", cfg.Canton.Ledger.MaxMessageSize)
	}
	if cfg.Canton.Ledger.Auth.ExpiryLeeway != 60*time.Second {
		t.Fatalf("auth.expiry_leeway default mismatch: got %s", cfg.Canton.Ledger.Auth.ExpiryLeeway)
	}

	if cfg.Token.NativeBalanceWei != "1000000000000000000000" {
		t.Fatalf("token.native_balance_wei default mismatch: got %q", cfg.Token.NativeBalanceWei)
	}

	if cfg.EthRPC.Enabled {
		t.Fatal("eth_rpc.enabled default mismatch: expected false")
	}
	if cfg.EthRPC.GasPriceWei != "1000000000" {
		t.Fatalf("eth_rpc.gas_price_wei default mismatch: got %q", cfg.EthRPC.GasPriceWei)
	}
	if cfg.EthRPC.GasLimit != 21000 {
		t.Fatalf("eth_rpc.gas_limit default mismatch: got %d", cfg.EthRPC.GasLimit)
	}
	if cfg.EthRPC.NativeBalanceWei != "1000000000000000000000" {
		t.Fatalf("eth_rpc.native_balance_wei default mismatch: got %q", cfg.EthRPC.NativeBalanceWei)
	}
	if cfg.EthRPC.RequestTimeout != 30*time.Second {
		t.Fatalf("eth_rpc.request_timeout default mismatch: got %s", cfg.EthRPC.RequestTimeout)
	}

	if cfg.Logging.OutputPath != "stdout" {
		t.Fatalf("logging.output_path default mismatch: got %q", cfg.Logging.OutputPath)
	}
	if cfg.Reconciliation.InitialTimeout != 2*time.Minute {
		t.Fatalf("reconciliation.initial_timeout default mismatch: got %s", cfg.Reconciliation.InitialTimeout)
	}
	if cfg.Reconciliation.Interval != 5*time.Minute {
		t.Fatalf("reconciliation.interval default mismatch: got %s", cfg.Reconciliation.Interval)
	}

	if cfg.KeyManagement.MasterKeyEnv != "CANTON_MASTER_KEY" {
		t.Fatalf("key_management.master_key_env default mismatch: got %q", cfg.KeyManagement.MasterKeyEnv)
	}
	if cfg.KeyManagement.KeyDerivation != "generate" {
		t.Fatalf("key_management.key_derivation default mismatch: got %q", cfg.KeyManagement.KeyDerivation)
	}

	if cfg.JWKS != nil {
		t.Fatal("jwks should default to nil when omitted")
	}
	if cfg.Canton.Bridge != nil {
		t.Fatal("canton.bridge should default to nil when omitted")
	}
}

func TestLoadRelayerServer_AppliesDefaults(t *testing.T) {
	cfg, err := LoadRelayerServer(testConfigPath(t, "minimal.relayer.yaml"))
	if err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}

	if cfg.Server.ReadTimeout != 15*time.Second {
		t.Fatalf("read_timeout default mismatch: got %s", cfg.Server.ReadTimeout)
	}
	if cfg.Server.WriteTimeout != 15*time.Second {
		t.Fatalf("write_timeout default mismatch: got %s", cfg.Server.WriteTimeout)
	}
	if cfg.Server.IdleTimeout != 60*time.Second {
		t.Fatalf("idle_timeout default mismatch: got %s", cfg.Server.IdleTimeout)
	}
	if cfg.Server.ShutdownTimeout != 30*time.Second {
		t.Fatalf("shutdown_timeout default mismatch: got %s", cfg.Server.ShutdownTimeout)
	}

	if cfg.Database.Timeout != 10 {
		t.Fatalf("database.timeout default mismatch: got %d", cfg.Database.Timeout)
	}
	if cfg.Database.PoolSize != 10 {
		t.Fatalf("database.pool_size default mismatch: got %d", cfg.Database.PoolSize)
	}

	if cfg.Ethereum.WSUrl != "" {
		t.Fatalf("ethereum.ws_url default mismatch: got %q", cfg.Ethereum.WSUrl)
	}
	if cfg.Ethereum.TokenContract != "" {
		t.Fatalf("ethereum.token_contract default mismatch: got %q", cfg.Ethereum.TokenContract)
	}
	if cfg.Ethereum.ConfirmationBlocks != 12 {
		t.Fatalf("ethereum.confirmation_blocks default mismatch: got %d", cfg.Ethereum.ConfirmationBlocks)
	}
	if cfg.Ethereum.MaxGasPrice != "" {
		t.Fatalf("ethereum.max_gas_price default mismatch: got %q", cfg.Ethereum.MaxGasPrice)
	}
	if cfg.Ethereum.StartBlock != 0 {
		t.Fatalf("ethereum.start_block default mismatch: got %d", cfg.Ethereum.StartBlock)
	}
	if cfg.Ethereum.LookbackBlocks != 1000 {
		t.Fatalf("ethereum.lookback_blocks default mismatch: got %d", cfg.Ethereum.LookbackBlocks)
	}

	if cfg.Canton.Ledger.MaxMessageSize != 52428800 {
		t.Fatalf("max_inbound_message_size default mismatch: got %d", cfg.Canton.Ledger.MaxMessageSize)
	}
	if cfg.Canton.Ledger.Auth.ExpiryLeeway != 60*time.Second {
		t.Fatalf("auth.expiry_leeway default mismatch: got %s", cfg.Canton.Ledger.Auth.ExpiryLeeway)
	}
	if cfg.Canton.Bridge != nil {
		t.Fatal("canton.bridge should default to nil when omitted")
	}

	if cfg.Bridge.MaxRetries != 5 {
		t.Fatalf("bridge.max_retries default mismatch: got %d", cfg.Bridge.MaxRetries)
	}
	if cfg.Bridge.RetryDelay != 60*time.Second {
		t.Fatalf("bridge.retry_delay default mismatch: got %s", cfg.Bridge.RetryDelay)
	}
	if cfg.Bridge.ProcessingInterval != 30*time.Second {
		t.Fatalf("bridge.processing_interval default mismatch: got %s", cfg.Bridge.ProcessingInterval)
	}
	if cfg.Bridge.CantonStartBlock != 0 {
		t.Fatalf("bridge.canton_start_block default mismatch: got %d", cfg.Bridge.CantonStartBlock)
	}
	if cfg.Bridge.CantonLookback != 1000 {
		t.Fatalf("bridge.canton_lookback_blocks default mismatch: got %d", cfg.Bridge.CantonLookback)
	}
	if cfg.Bridge.EthStartBlock != 0 {
		t.Fatalf("bridge.eth_start_block default mismatch: got %d", cfg.Bridge.EthStartBlock)
	}
	if cfg.Bridge.EthLookbackBlocks != 1000 {
		t.Fatalf("bridge.eth_lookback_blocks default mismatch: got %d", cfg.Bridge.EthLookbackBlocks)
	}

	if cfg.Monitoring.Enabled {
		t.Fatal("monitoring.enabled default mismatch: expected false")
	}
	if cfg.Monitoring.Server != nil {
		t.Fatal("monitoring.server should be nil when monitoring is disabled")
	}
	if cfg.Monitoring.HealthCheckURL != "/health" {
		t.Fatalf("monitoring.health_check_url default mismatch: got %q", cfg.Monitoring.HealthCheckURL)
	}
	if cfg.Logging.OutputPath != "stdout" {
		t.Fatalf("logging.output_path default mismatch: got %q", cfg.Logging.OutputPath)
	}
}

func TestLoadAPIServer_RequiredValidation(t *testing.T) {
	path := testConfigPath(t, "missing-required.api.yaml")

	_, err := LoadAPIServer(path)
	if err == nil {
		t.Fatal("expected required validation error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "database") {
		t.Fatalf("expected missing database error, got: %v", err)
	}
}

func TestLoadAPIServer_InvalidDatabaseURL(t *testing.T) {
	path := testConfigPath(t, "invalid-database-url.api.yaml")

	_, err := LoadAPIServer(path)
	if err == nil {
		t.Fatal("expected database URL validation error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "database.url") {
		t.Fatalf("expected database.url validation error, got: %v", err)
	}
}

func TestLoadRelayerServer_RequiredValidation(t *testing.T) {
	path := testConfigPath(t, "missing-required.relayer.yaml")

	_, err := LoadRelayerServer(path)
	if err == nil {
		t.Fatal("expected required validation error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "eth_token_contract") {
		t.Fatalf("expected missing eth_token_contract error, got: %v", err)
	}
}

func TestLoadAPIServer_RejectsUnknownField(t *testing.T) {
	path := testConfigPath(t, "unknown-field.api.yaml")

	_, err := LoadAPIServer(path)
	if err == nil {
		t.Fatal("expected unknown field error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unknown_field") {
		t.Fatalf("expected unknown_field in error, got: %v", err)
	}
}

func TestLoadRelayerServer_RejectsUnknownField(t *testing.T) {
	path := testConfigPath(t, "unknown-field.relayer.yaml")

	_, err := LoadRelayerServer(path)
	if err == nil {
		t.Fatal("expected unknown field error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unknown_field") {
		t.Fatalf("expected unknown_field in error, got: %v", err)
	}
}

func TestLoadRelayerServer_MonitoringEnabledRequiresServer(t *testing.T) {
	t.Run("enabled without server fails", func(t *testing.T) {
		_, err := LoadRelayerServer(testConfigPath(t, "monitoring-enabled-no-server.relayer.yaml"))
		if err == nil {
			t.Fatal("expected validation error when monitoring.enabled=true and server is nil, got nil")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "server") {
			t.Fatalf("expected error mentioning server, got: %v", err)
		}
	})

	t.Run("enabled with server passes", func(t *testing.T) {
		cfg, err := LoadRelayerServer(testConfigPath(t, "monitoring-enabled-with-server.relayer.yaml"))
		if err != nil {
			t.Fatalf("expected valid config, got error: %v", err)
		}
		if !cfg.Monitoring.Enabled {
			t.Fatal("monitoring.enabled should be true")
		}
		if cfg.Monitoring.Server == nil {
			t.Fatal("monitoring.server should not be nil")
		}
		if cfg.Monitoring.Server.Port != 9090 {
			t.Fatalf("monitoring.server.port mismatch: got %d, want 9090", cfg.Monitoring.Server.Port)
		}
	})

	t.Run("disabled without server passes", func(t *testing.T) {
		cfg, err := LoadRelayerServer(testConfigPath(t, "minimal.relayer.yaml"))
		if err != nil {
			t.Fatalf("expected valid config, got error: %v", err)
		}
		if cfg.Monitoring.Enabled {
			t.Fatal("monitoring.enabled should be false")
		}
	})
}

func TestLoadConfig_FileErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		_, err := LoadAPIServer("/non-existent/config.yaml")
		if err == nil {
			t.Fatal("expected read-file error, got nil")
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		path := testConfigPath(t, "invalid.yaml")
		_, err := LoadRelayerServer(path)
		if err == nil {
			t.Fatal("expected yaml parse error, got nil")
		}
	})
}

func testConfigPath(t *testing.T, fileName string) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	return filepath.Join(filepath.Dir(file), "tests", fileName)
}

func defaultConfigPath(t *testing.T, fileName string) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	return filepath.Join(filepath.Dir(file), "defaults", fileName)
}

func setDefaultConfigEnv(t *testing.T) {
	t.Helper()

	t.Setenv("API_SERVER_DATABASE_URL", "postgres://postgres:pass@localhost:5432/erc20_api")
	t.Setenv("RELAYER_DATABASE_URL", "postgres://postgres:pass@localhost:5432/relayer")
	t.Setenv("CANTON_DOMAIN_ID", "global-domain::1220test")
	t.Setenv("CANTON_ISSUER_PARTY", "issuer::1220test")
	t.Setenv("CANTON_LEDGER_RPC_URL", "canton-ledger.example:443")
	t.Setenv("CANTON_AUTH_CLIENT_ID", "test-client-id")
	t.Setenv("CANTON_AUTH_CLIENT_SECRET", "test-client-secret")
	t.Setenv("CANTON_AUTH_AUDIENCE", "https://canton-ledger.example")
	t.Setenv("CANTON_AUTH_TOKEN_URL", "https://auth.example/oauth/token")
	t.Setenv("CANTON_IDENTITY_PACKAGE_ID", "identity-package-id")
	t.Setenv("CANTON_CIP56_PACKAGE_ID", "cip56-package-id")
	t.Setenv("CANTON_SPLICE_TRANSFER_PACKAGE_ID", "splice-transfer-package-id")
	t.Setenv("CANTON_SPLICE_HOLDING_PACKAGE_ID", "splice-holding-package-id")
	t.Setenv("CANTON_BRIDGE_PACKAGE_ID", "bridge-package-id")
	t.Setenv("CANTON_USDCX_INSTRUMENT_ADMIN", "Bridge-Operator::1220test")
	t.Setenv("ETHEREUM_RPC_URL", "https://eth.example")
	t.Setenv("ETHEREUM_WS_URL", "wss://eth.example/ws")
	t.Setenv("ETHEREUM_CHAIN_ID", "1")
	t.Setenv("ETHEREUM_BRIDGE_CONTRACT", "0x1111111111111111111111111111111111111111")
	t.Setenv("ETHEREUM_TOKEN_CONTRACT", "0x2222222222222222222222222222222222222222")
	t.Setenv("ETHEREUM_RELAYER_PRIVATE_KEY", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	t.Setenv("SEPOLIA_RPC_URL", "https://sepolia.example")
	t.Setenv("SEPOLIA_WS_URL", "wss://sepolia.example/ws")
}
