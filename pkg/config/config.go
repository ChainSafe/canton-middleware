package config

import (
	"fmt"
	"os"

	"github.com/chainsafe/canton-middleware/pkg/app/http"
	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/client"
	"github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc"
	"github.com/chainsafe/canton-middleware/pkg/log"
	pgdb "github.com/chainsafe/canton-middleware/pkg/pgutil"
	"github.com/chainsafe/canton-middleware/pkg/reconciler"
	"github.com/chainsafe/canton-middleware/pkg/relayer"
	"github.com/chainsafe/canton-middleware/pkg/token"

	"gopkg.in/yaml.v3"
)

// APIServer represents the ERC-20 API server configuration
type APIServer struct {
	Server         *http.ServerConfig   `yaml:"server"`
	Database       *pgdb.DatabaseConfig `yaml:"database"`
	Canton         *canton.Config       `yaml:"canton"`
	Token          *token.Config        `yaml:"token"`
	EthRPC         *ethrpc.Config       `yaml:"eth_rpc"`
	JWKS           *JWKS                `yaml:"jwks"`
	Logging        *log.Config          `yaml:"logging"`
	Reconciliation *reconciler.Config   `yaml:"reconciliation"`
	KeyManagement  *KeyManagement       `yaml:"key_management"` // Custodial Canton key settings
}

// RelayerServer represents the application configuration for relayer.
type RelayerServer struct {
	Server     *http.ServerConfig   `yaml:"server"`
	Database   *pgdb.DatabaseConfig `yaml:"database"`
	Ethereum   *ethereum.Config     `yaml:"ethereum"`
	Canton     *canton.Config       `yaml:"canton"`
	Bridge     *relayer.Config      `yaml:"bridge"`
	Monitoring *Monitoring          `yaml:"monitoring"`
	Logging    *log.Config          `yaml:"logging"`
}

// Monitoring contains monitoring and metrics settings
type Monitoring struct {
	Enabled        bool   `yaml:"enabled"`
	MetricsPort    int    `yaml:"metrics_port"`
	HealthCheckURL string `yaml:"health_check_url"`
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

// LoadAPIServer loads API app configuration from file
func LoadAPIServer(configPath string) (*APIServer, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	var config APIServer
	if err = yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	return &config, nil
}

// LoadRelayerServer loads configuration from file and environment variables for relayer app.
func LoadRelayerServer(configPath string) (*RelayerServer, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	var config RelayerServer
	if err = yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	return &config, nil
}
