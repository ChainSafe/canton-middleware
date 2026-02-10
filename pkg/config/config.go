package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Database   DatabaseConfig   `yaml:"database"`
	Ethereum   EthereumConfig   `yaml:"ethereum"`
	Canton     CantonConfig     `yaml:"canton"`
	Bridge     BridgeConfig     `yaml:"bridge"`
	Monitoring MonitoringConfig `yaml:"monitoring"`
	Logging    LoggingConfig    `yaml:"logging"`
}

// ServerConfig contains HTTP server settings
type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

// DatabaseConfig contains database connection settings
type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
	SSLMode  string `yaml:"ssl_mode"`
}

// EthereumConfig contains Ethereum client settings
type EthereumConfig struct {
	RPCURL             string        `yaml:"rpc_url"`
	WSUrl              string        `yaml:"ws_url"`
	ChainID            int64         `yaml:"chain_id"`
	BridgeContract     string        `yaml:"bridge_contract"`
	TokenContract      string        `yaml:"token_contract"`
	RelayerPrivateKey  string        `yaml:"relayer_private_key"`
	ConfirmationBlocks int           `yaml:"confirmation_blocks"`
	GasLimit           uint64        `yaml:"gas_limit"`
	MaxGasPrice        string        `yaml:"max_gas_price"`
	PollingInterval    time.Duration `yaml:"polling_interval"`
	StartBlock         int64         `yaml:"start_block"`
	LookbackBlocks     int64         `yaml:"lookback_blocks"`
}

// CantonConfig contains Canton Network client settings
type CantonConfig struct {
	RPCURL               string        `yaml:"rpc_url"`
	LedgerID             string        `yaml:"ledger_id"`
	DomainID             string        `yaml:"domain_id"`
	ApplicationID        string        `yaml:"application_id"`
	ChainID              string        `yaml:"chain_id"`
	BridgeContract       string        `yaml:"bridge_contract"`
	RelayerParty         string        `yaml:"relayer_party"`
	BridgePackageID      string        `yaml:"bridge_package_id"`
	CorePackageID        string        `yaml:"core_package_id"`
	CIP56PackageID       string        `yaml:"cip56_package_id"`
	CommonPackageID      string        `yaml:"common_package_id"`       // Package ID for common DAR (FingerprintMapping)
	BridgeModule         string        `yaml:"bridge_module"`
	RelayerPrivateKey    string        `yaml:"relayer_private_key"`
	ConfirmationBlocks   int           `yaml:"confirmation_blocks"`
	PollingInterval      time.Duration `yaml:"polling_interval"`
	StartBlock           int64         `yaml:"start_block"`
	LookbackBlocks       int64         `yaml:"lookback_blocks"`
	TLS                  TLSConfig     `yaml:"tls"`
	Auth                 AuthConfig    `yaml:"auth"`
	DedupDuration        time.Duration `yaml:"dedup_duration"`
	MaxMessageSize       int           `yaml:"max_inbound_message_size"`
}

// TLSConfig holds TLS configuration
type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
	CAFile   string `yaml:"ca_file"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	JWTIssuer    string `yaml:"jwt_issuer"`
	TokenFile    string `yaml:"token_file"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	Audience     string `yaml:"audience"`
	TokenURL     string `yaml:"token_url"`
}

// BridgeConfig contains bridge operation settings
type BridgeConfig struct {
	MaxTransferAmount  string        `yaml:"max_transfer_amount"`
	MinTransferAmount  string        `yaml:"min_transfer_amount"`
	RateLimitPerHour   int           `yaml:"rate_limit_per_hour"`
	MaxRetries         int           `yaml:"max_retries"`
	RetryDelay         time.Duration `yaml:"retry_delay"`
	ProcessingInterval time.Duration `yaml:"processing_interval"`
}

// MonitoringConfig contains monitoring and metrics settings
type MonitoringConfig struct {
	Enabled        bool   `yaml:"enabled"`
	MetricsPort    int    `yaml:"metrics_port"`
	HealthCheckURL string `yaml:"health_check_url"`
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	Level      string `yaml:"level"`
	Format     string `yaml:"format"`
	OutputPath string `yaml:"output_path"`
}

// =============================================================================
// API SERVER CONFIG
// =============================================================================

// APIServerConfig represents the ERC-20 API server configuration
type APIServerConfig struct {
	Server         ServerConfig         `yaml:"server"`
	Database       DatabaseConfig       `yaml:"database"`
	Canton         CantonConfig         `yaml:"canton"`
	Token          TokenConfig          `yaml:"token"`
	DemoToken      TokenConfig          `yaml:"demo_token"` // DEMO token metadata (native)
	EthRPC         EthRPCConfig         `yaml:"eth_rpc"`
	JWKS           JWKSConfig           `yaml:"jwks"`
	Logging        LoggingConfig        `yaml:"logging"`
	Reconciliation ReconciliationConfig `yaml:"reconciliation"`
	Shutdown       ShutdownConfig       `yaml:"shutdown"`
	KeyManagement  KeyManagementConfig  `yaml:"key_management"` // Custodial Canton key settings
}

// EthRPCConfig contains Ethereum JSON-RPC facade settings for MetaMask compatibility
type EthRPCConfig struct {
	Enabled          bool          `yaml:"enabled"`
	ChainID          uint64        `yaml:"chain_id"`
	TokenAddress     string        `yaml:"token_address"`      // PROMPT token address
	DemoTokenAddress string        `yaml:"demo_token_address"` // DEMO token address (native)
	GasPriceWei      string        `yaml:"gas_price_wei"`
	GasLimit         uint64        `yaml:"gas_limit"`
	NativeBalanceWei string        `yaml:"native_balance_wei"`
	RequestTimeout   time.Duration `yaml:"request_timeout"`
}

// TokenConfig contains ERC-20 token metadata
type TokenConfig struct {
	Name     string `yaml:"name"`
	Symbol   string `yaml:"symbol"`
	Decimals int    `yaml:"decimals"`
}

// JWKSConfig contains JWKS configuration for JWT validation
type JWKSConfig struct {
	URL    string `yaml:"url"`
	Issuer string `yaml:"issuer"`
}

// ReconciliationConfig contains settings for balance reconciliation
type ReconciliationConfig struct {
	InitialTimeout time.Duration `yaml:"initial_timeout"`
	Interval       time.Duration `yaml:"interval"`
}

// ShutdownConfig contains graceful shutdown settings
type ShutdownConfig struct {
	Timeout time.Duration `yaml:"timeout"`
}

// KeyManagementConfig contains settings for custodial Canton key management
type KeyManagementConfig struct {
	// MasterKeyEnv is the environment variable name containing the master encryption key (base64)
	MasterKeyEnv string `yaml:"master_key_env"`
	// KeyDerivation specifies how to generate Canton keys: "generate" (random) or "derive" (from EVM + seed)
	KeyDerivation string `yaml:"key_derivation"`
}

// GetTokenConfig returns the TokenConfig for a given token symbol
func (c *APIServerConfig) GetTokenConfig(symbol string) *TokenConfig {
	switch symbol {
	case "DEMO":
		return &c.DemoToken
	default:
		return &c.Token
	}
}

// LoadAPIServer loads API server configuration from file
func LoadAPIServer(configPath string) (*APIServerConfig, error) {
	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var config APIServerConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Set defaults
	setAPIServerDefaults(&config)

	// Override with environment variables
	overrideAPIServerEnv(&config)

	// Validate
	if err := validateAPIServer(&config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}

func setAPIServerDefaults(config *APIServerConfig) {
	// Server defaults
	if config.Server.Host == "" {
		config.Server.Host = "0.0.0.0"
	}
	if config.Server.Port == 0 {
		config.Server.Port = 8081
	}

	// Database defaults
	if config.Database.Host == "" {
		config.Database.Host = "localhost"
	}
	if config.Database.Port == 0 {
		config.Database.Port = 5432
	}
	if config.Database.SSLMode == "" {
		config.Database.SSLMode = "disable"
	}
	if config.Database.Database == "" {
		config.Database.Database = "erc20_api"
	}

	// Token defaults (PROMPT - bridge token)
	if config.Token.Name == "" {
		config.Token.Name = "PROMPT"
	}
	if config.Token.Symbol == "" {
		config.Token.Symbol = "PROMPT"
	}
	if config.Token.Decimals == 0 {
		config.Token.Decimals = 18
	}

	// DemoToken defaults (DEMO - native token)
	if config.DemoToken.Name == "" {
		config.DemoToken.Name = "Demo Token"
	}
	if config.DemoToken.Symbol == "" {
		config.DemoToken.Symbol = "DEMO"
	}
	if config.DemoToken.Decimals == 0 {
		config.DemoToken.Decimals = 18
	}

	// Eth RPC defaults
	if config.EthRPC.ChainID == 0 {
		config.EthRPC.ChainID = 31337
	}
	if config.EthRPC.GasPriceWei == "" {
		config.EthRPC.GasPriceWei = "1000000000"
	}
	if config.EthRPC.GasLimit == 0 {
		config.EthRPC.GasLimit = 21000
	}
	if config.EthRPC.NativeBalanceWei == "" {
		config.EthRPC.NativeBalanceWei = "1000000000000000000000"
	}
	if config.EthRPC.RequestTimeout == 0 {
		config.EthRPC.RequestTimeout = 30 * time.Second
	}
	// Default DEMO token address (synthetic address for native token)
	if config.EthRPC.DemoTokenAddress == "" {
		config.EthRPC.DemoTokenAddress = "0xDE30000000000000000000000000000000000001"
	}

	// Logging defaults
	if config.Logging.Level == "" {
		config.Logging.Level = "info"
	}
	if config.Logging.Format == "" {
		config.Logging.Format = "json"
	}
	if config.Logging.OutputPath == "" {
		config.Logging.OutputPath = "stdout"
	}

	// Reconciliation defaults
	if config.Reconciliation.InitialTimeout == 0 {
		config.Reconciliation.InitialTimeout = 2 * time.Minute
	}
	if config.Reconciliation.Interval == 0 {
		config.Reconciliation.Interval = 5 * time.Minute
	}

	// Shutdown defaults
	if config.Shutdown.Timeout == 0 {
		config.Shutdown.Timeout = 30 * time.Second
	}

	// KeyManagement defaults
	if config.KeyManagement.MasterKeyEnv == "" {
		config.KeyManagement.MasterKeyEnv = "CANTON_MASTER_KEY"
	}
	if config.KeyManagement.KeyDerivation == "" {
		config.KeyManagement.KeyDerivation = "generate"
	}
}

func overrideAPIServerEnv(config *APIServerConfig) {
	// Server
	if v := os.Getenv("SERVER_HOST"); v != "" {
		config.Server.Host = v
	}
	if v := os.Getenv("SERVER_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			config.Server.Port = port
		}
	}

	// Database
	if v := os.Getenv("DATABASE_HOST"); v != "" {
		config.Database.Host = v
	}
	if v := os.Getenv("DATABASE_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			config.Database.Port = port
		}
	}
	if v := os.Getenv("DATABASE_USER"); v != "" {
		config.Database.User = v
	}
	if v := os.Getenv("DATABASE_PASSWORD"); v != "" {
		config.Database.Password = v
	}
	if v := os.Getenv("DATABASE_DATABASE"); v != "" {
		config.Database.Database = v
	}
	if v := os.Getenv("DATABASE_SSL_MODE"); v != "" {
		config.Database.SSLMode = v
	}

	// Canton
	if v := os.Getenv("CANTON_RPC_URL"); v != "" {
		config.Canton.RPCURL = v
	}
	if v := os.Getenv("CANTON_RELAYER_PRIVATE_KEY"); v != "" {
		config.Canton.RelayerPrivateKey = v
	}

	// Logging
	if v := os.Getenv("LOGGING_LEVEL"); v != "" {
		config.Logging.Level = v
	}
}

func validateAPIServer(config *APIServerConfig) error {
	if config.Database.Host == "" {
		return fmt.Errorf("database.host is required")
	}
	if config.Canton.RPCURL == "" {
		return fmt.Errorf("canton.rpc_url is required")
	}
	return nil
}

// Load loads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Set defaults
	setDefaults(&config)

	// Override with environment variables
	overrideEnv(&config)

	// Validate
	if err := validate(&config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}

func setDefaults(config *Config) {
	// Server defaults
	if config.Server.Host == "" {
		config.Server.Host = "0.0.0.0"
	}
	if config.Server.Port == 0 {
		config.Server.Port = 8080
	}

	// Database defaults
	if config.Database.Host == "" {
		config.Database.Host = "localhost"
	}
	if config.Database.Port == 0 {
		config.Database.Port = 5432
	}
	if config.Database.SSLMode == "" {
		config.Database.SSLMode = "disable"
	}

	// Ethereum defaults
	if config.Ethereum.ConfirmationBlocks == 0 {
		config.Ethereum.ConfirmationBlocks = 12
	}
	if config.Ethereum.GasLimit == 0 {
		config.Ethereum.GasLimit = 300000
	}
	if config.Ethereum.PollingInterval == 0 {
		config.Ethereum.PollingInterval = 15 * time.Second
	}
	if config.Ethereum.LookbackBlocks == 0 {
		config.Ethereum.LookbackBlocks = 1000
	}

	// Canton defaults
	if config.Canton.ConfirmationBlocks == 0 {
		config.Canton.ConfirmationBlocks = 1
	}
	if config.Canton.PollingInterval == 0 {
		config.Canton.PollingInterval = 10 * time.Second
	}
	if config.Canton.LookbackBlocks == 0 {
		config.Canton.LookbackBlocks = 1000
	}

	// Bridge defaults
	if config.Bridge.MaxRetries == 0 {
		config.Bridge.MaxRetries = 3
	}
	if config.Bridge.RetryDelay == 0 {
		config.Bridge.RetryDelay = 1 * time.Minute
	}
	if config.Bridge.ProcessingInterval == 0 {
		config.Bridge.ProcessingInterval = 30 * time.Second
	}
	if config.Bridge.RateLimitPerHour == 0 {
		config.Bridge.RateLimitPerHour = 100
	}

	// Monitoring defaults
	if config.Monitoring.MetricsPort == 0 {
		config.Monitoring.MetricsPort = 9090
	}

	// Logging defaults
	if config.Logging.Level == "" {
		config.Logging.Level = "info"
	}
	if config.Logging.Format == "" {
		config.Logging.Format = "json"
	}
	if config.Logging.OutputPath == "" {
		config.Logging.OutputPath = "stdout"
	}
}

func overrideEnv(config *Config) {
	// Server
	if v := os.Getenv("SERVER_HOST"); v != "" {
		config.Server.Host = v
	}
	if v := os.Getenv("SERVER_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			config.Server.Port = port
		}
	}

	// Database
	if v := os.Getenv("DATABASE_HOST"); v != "" {
		config.Database.Host = v
	}
	if v := os.Getenv("DATABASE_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			config.Database.Port = port
		}
	}
	if v := os.Getenv("DATABASE_USER"); v != "" {
		config.Database.User = v
	}
	if v := os.Getenv("DATABASE_PASSWORD"); v != "" {
		config.Database.Password = v
	}
	if v := os.Getenv("DATABASE_DATABASE"); v != "" {
		config.Database.Database = v
	}
	if v := os.Getenv("DATABASE_SSL_MODE"); v != "" {
		config.Database.SSLMode = v
	}

	// Ethereum
	if v := os.Getenv("ETHEREUM_RPC_URL"); v != "" {
		config.Ethereum.RPCURL = v
	}
	if v := os.Getenv("ETHEREUM_RELAYER_PRIVATE_KEY"); v != "" {
		config.Ethereum.RelayerPrivateKey = v
	}

	// Canton
	if v := os.Getenv("CANTON_RPC_URL"); v != "" {
		config.Canton.RPCURL = v
	}
	if v := os.Getenv("CANTON_RELAYER_PRIVATE_KEY"); v != "" {
		config.Canton.RelayerPrivateKey = v
	}

	// Logging
	if v := os.Getenv("LOGGING_LEVEL"); v != "" {
		config.Logging.Level = v
	}
}

func validate(config *Config) error {
	if config.Database.Host == "" {
		return fmt.Errorf("database.host is required")
	}
	if config.Ethereum.RPCURL == "" {
		return fmt.Errorf("ethereum.rpc_url is required")
	}
	if config.Canton.RPCURL == "" {
		return fmt.Errorf("canton.rpc_url is required")
	}
	if config.Ethereum.BridgeContract == "" {
		return fmt.Errorf("ethereum.bridge_contract is required")
	}
	if config.Canton.BridgeContract == "" {
		return fmt.Errorf("canton.bridge_contract is required")
	}
	return nil
}

// GetDatabaseConnectionString returns a PostgreSQL connection string
func (c *DatabaseConfig) GetConnectionString() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Database, c.SSLMode,
	)
}

// GetAPIConnectionString returns a PostgreSQL connection string for the API database (erc20_api)
// This is used by the relayer to update the balance cache
func (c *DatabaseConfig) GetAPIConnectionString() string {
	if c.Host == "" {
		return ""
	}
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, "erc20_api", c.SSLMode,
	)
}
