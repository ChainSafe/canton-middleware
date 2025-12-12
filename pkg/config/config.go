package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config represents the application configuration
type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	Database   DatabaseConfig   `mapstructure:"database"`
	Ethereum   EthereumConfig   `mapstructure:"ethereum"`
	Canton     CantonConfig     `mapstructure:"canton"`
	Bridge     BridgeConfig     `mapstructure:"bridge"`
	Monitoring MonitoringConfig `mapstructure:"monitoring"`
	Logging    LoggingConfig    `mapstructure:"logging"`
}

// ServerConfig contains HTTP server settings
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

// DatabaseConfig contains database connection settings
type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Database string `mapstructure:"database"`
	SSLMode  string `mapstructure:"ssl_mode"`
}

// EthereumConfig contains Ethereum client settings
type EthereumConfig struct {
	RPCURL             string        `mapstructure:"rpc_url"`
	WSUrl              string        `mapstructure:"ws_url"`
	ChainID            int64         `mapstructure:"chain_id"`
	BridgeContract     string        `mapstructure:"bridge_contract"`
	TokenContract      string        `mapstructure:"token_contract"`
	RelayerPrivateKey  string        `mapstructure:"relayer_private_key"`
	ConfirmationBlocks int           `mapstructure:"confirmation_blocks"`
	GasLimit           uint64        `mapstructure:"gas_limit"`
	MaxGasPrice        string        `mapstructure:"max_gas_price"`
	PollingInterval    time.Duration `mapstructure:"polling_interval"`
	StartBlock         int64         `mapstructure:"start_block"`
	LookbackBlocks     int64         `mapstructure:"lookback_blocks"`
}

// CantonConfig contains Canton Network client settings
type CantonConfig struct {
	RPCURL             string        `mapstructure:"rpc_url"`
	LedgerID           string        `mapstructure:"ledger_id"`
	DomainID           string        `mapstructure:"domain_id"`
	ApplicationID      string        `mapstructure:"application_id"`
	ChainID            string        `mapstructure:"chain_id"`
	BridgeContract     string        `mapstructure:"bridge_contract"`
	RelayerParty       string        `mapstructure:"relayer_party"`
	BridgePackageID    string        `mapstructure:"bridge_package_id"`
	CorePackageID      string        `mapstructure:"core_package_id"`
	CIP56PackageID     string        `mapstructure:"cip56_package_id"`
	BridgeModule       string        `mapstructure:"bridge_module"`
	RelayerPrivateKey  string        `mapstructure:"relayer_private_key"`
	ConfirmationBlocks int           `mapstructure:"confirmation_blocks"`
	PollingInterval    time.Duration `mapstructure:"polling_interval"`
	StartBlock         int64         `mapstructure:"start_block"`
	LookbackBlocks     int64         `mapstructure:"lookback_blocks"`
	TLS                TLSConfig     `mapstructure:"tls"`
	Auth               AuthConfig    `mapstructure:"auth"`
	DedupDuration      time.Duration `mapstructure:"dedup_duration"`
	MaxMessageSize     int           `mapstructure:"max_inbound_message_size"`
}

// TLSConfig holds TLS configuration
type TLSConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
	CAFile   string `mapstructure:"ca_file"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	JWTIssuer    string `mapstructure:"jwt_issuer"`
	TokenFile    string `mapstructure:"token_file"`
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	Audience     string `mapstructure:"audience"`
	TokenURL     string `mapstructure:"token_url"`
}

// BridgeConfig contains bridge operation settings
type BridgeConfig struct {
	MaxTransferAmount  string        `mapstructure:"max_transfer_amount"`
	MinTransferAmount  string        `mapstructure:"min_transfer_amount"`
	RateLimitPerHour   int           `mapstructure:"rate_limit_per_hour"`
	MaxRetries         int           `mapstructure:"max_retries"`
	RetryDelay         time.Duration `mapstructure:"retry_delay"`
	ProcessingInterval time.Duration `mapstructure:"processing_interval"`
}

// MonitoringConfig contains monitoring and metrics settings
type MonitoringConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	MetricsPort    int    `mapstructure:"metrics_port"`
	HealthCheckURL string `mapstructure:"health_check_url"`
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"`
	OutputPath string `mapstructure:"output_path"`
}

// Load loads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")
	viper.AutomaticEnv()

	// Set defaults
	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := validate(&config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}

func setDefaults() {
	// Server defaults
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8080)

	// Database defaults
	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("database.ssl_mode", "disable")

	// Ethereum defaults
	viper.SetDefault("ethereum.confirmation_blocks", 12)
	viper.SetDefault("ethereum.gas_limit", 300000)
	viper.SetDefault("ethereum.polling_interval", "15s")
	viper.SetDefault("ethereum.start_block", 0)
	viper.SetDefault("ethereum.lookback_blocks", 1000)

	// Canton defaults
	viper.SetDefault("canton.confirmation_blocks", 1)
	viper.SetDefault("canton.polling_interval", "10s")
	viper.SetDefault("canton.start_block", 0)
	viper.SetDefault("canton.lookback_blocks", 1000)

	// Bridge defaults
	viper.SetDefault("bridge.max_retries", 3)
	viper.SetDefault("bridge.retry_delay", "1m")
	viper.SetDefault("bridge.processing_interval", "30s")
	viper.SetDefault("bridge.rate_limit_per_hour", 100)

	// Monitoring defaults
	viper.SetDefault("monitoring.enabled", true)
	viper.SetDefault("monitoring.metrics_port", 9090)

	// Logging defaults
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.format", "json")
	viper.SetDefault("logging.output_path", "stdout")
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
