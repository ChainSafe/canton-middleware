package config

import (
	"fmt"
	"os"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc"
	"github.com/chainsafe/canton-middleware/pkg/reconciler"
	"github.com/chainsafe/canton-middleware/pkg/relayer"
	"github.com/chainsafe/canton-middleware/pkg/token"
	"gopkg.in/yaml.v3"
)

// APIApp represents the ERC-20 API server configuration
type APIApp struct {
	Server         *Server            `yaml:"server"`
	Database       *Database          `yaml:"database"`
	Canton         *CantonConfig      `yaml:"canton"`
	Token          *token.Config      `yaml:"token"`
	EthRPC         *ethrpc.Config     `yaml:"eth_rpc"`
	JWKS           *JWKS              `yaml:"jwks"`
	Logging        *Logging           `yaml:"logging"`
	Reconciliation *reconciler.Config `yaml:"reconciliation"`
	KeyManagement  *KeyManagement     `yaml:"key_management"` // Custodial Canton key settings
}

// RelayerApp represents the application configuration for relayer.
type RelayerApp struct {
	Server     *Server               `yaml:"server"`
	Database   *Database             `yaml:"database"`
	Ethereum   *ethereum.Config      `yaml:"ethereum"`
	Canton     *CantonConfig         `yaml:"canton"`
	Bridge     *relayer.BridgeConfig `yaml:"bridge"`
	Monitoring *Monitoring           `yaml:"monitoring"`
	Logging    *Logging              `yaml:"logging"`
}

// Server contains HTTP server settings
type Server struct {
	Host            string        `yaml:"host"`
	Port            int           `yaml:"port"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	IdleTimeout     time.Duration `yaml:"idle_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

// Database contains database connection settings
// TODO: refactor to connection url
type Database struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
	SSLMode  string `yaml:"ssl_mode"`
}

// GetConnectionString returns a PostgreSQL connection string
func (c *Database) GetConnectionString() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Database, c.SSLMode,
	)
}

// Canton contains Canton Network client settings
type Canton struct {
	RPCURL                  string        `yaml:"rpc_url"`
	LedgerID                string        `yaml:"ledger_id"`
	DomainID                string        `yaml:"domain_id"`
	ApplicationID           string        `yaml:"application_id"`
	ChainID                 string        `yaml:"chain_id"`
	BridgeContract          string        `yaml:"bridge_contract"`
	RelayerParty            string        `yaml:"relayer_party"`
	BridgePackageID         string        `yaml:"bridge_package_id"`
	CorePackageID           string        `yaml:"core_package_id"`
	CIP56PackageID          string        `yaml:"cip56_package_id"`
	CommonPackageID         string        `yaml:"common_package_id"`
	SpliceTransferPackageID string        `yaml:"splice_transfer_package_id"`
	BridgeModule            string        `yaml:"bridge_module"`
	RelayerPrivateKey       string        `yaml:"relayer_private_key"`
	ConfirmationBlocks      int           `yaml:"confirmation_blocks"`
	PollingInterval         time.Duration `yaml:"polling_interval"`
	StartBlock              int64         `yaml:"start_block"`
	LookbackBlocks          int64         `yaml:"lookback_blocks"`
	TLS                     *TLSConfig    `yaml:"tls"`
	Auth                    *Auth         `yaml:"auth"`
	DedupDuration           time.Duration `yaml:"dedup_duration"`
	MaxMessageSize          int           `yaml:"max_inbound_message_size"`
}

// TLSConfig holds TLS configuration
type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
	CAFile   string `yaml:"ca_file"`
}

// Auth holds authentication configuration
type Auth struct {
	JWTIssuer    string `yaml:"jwt_issuer"`
	TokenFile    string `yaml:"token_file"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	Audience     string `yaml:"audience"`
	TokenURL     string `yaml:"token_url"`
}

// Monitoring contains monitoring and metrics settings
type Monitoring struct {
	Enabled        bool   `yaml:"enabled"`
	MetricsPort    int    `yaml:"metrics_port"`
	HealthCheckURL string `yaml:"health_check_url"`
}

// Logging contains logging settings
type Logging struct {
	Level      string `yaml:"level"`
	Format     string `yaml:"format"`
	OutputPath string `yaml:"output_path"`
}

// JWKS contains JWKS configuration for JWT validation
type JWKS struct {
	URL    string `yaml:"url"`
	Issuer string `yaml:"issuer"`
}

// KeyManagement contains settings for custodial Canton key management
type KeyManagement struct {
	// MasterKeyEnv is the environment variable name containing the master encryption key (base64)
	MasterKeyEnv string `yaml:"master_key_env"`
	// KeyDerivation specifies how to generate Canton keys: "generate" (random) or "derive" (from EVM + seed)
	KeyDerivation string `yaml:"key_derivation"` // TODO: check usages
}

// LoadAPIApp loads API app configuration from file
func LoadAPIApp(configPath string) (*APIApp, error) {
	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var config APIApp
	if err = yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	return &config, nil
}

// LoadRelayerApp loads configuration from file and environment variables for relayer app.
func LoadRelayerApp(configPath string) (*RelayerApp, error) {
	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var config RelayerApp
	if err = yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}
