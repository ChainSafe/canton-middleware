package config

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/chainsafe/canton-middleware/pkg/app/http"
	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/client"
	"github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc"
	"github.com/chainsafe/canton-middleware/pkg/log"
	pgdb "github.com/chainsafe/canton-middleware/pkg/pgutil"
	"github.com/chainsafe/canton-middleware/pkg/reconciler"
	"github.com/chainsafe/canton-middleware/pkg/relayer"
	"github.com/chainsafe/canton-middleware/pkg/token"

	"github.com/creasty/defaults"
	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"
)

// APIServer represents the ERC-20 API server configuration
type APIServer struct {
	Server              *http.ServerConfig   `yaml:"server" validate:"required"`
	Database            *pgdb.DatabaseConfig `yaml:"database" validate:"required"`
	Canton              *canton.Config       `yaml:"canton" validate:"required"`
	Token               *token.Config        `yaml:"token" validate:"required"`
	EthRPC              *ethrpc.Config       `yaml:"eth_rpc" validate:"required"`
	JWKS                *JWKS                `yaml:"jwks" default:"-"` // nil by default (feature disabled)
	Logging             *log.Config          `yaml:"logging" validate:"required"`
	Reconciliation      *reconciler.Config   `yaml:"reconciliation" validate:"required"`
	KeyManagement       *KeyManagement       `yaml:"key_management" validate:"required"` // Custodial Canton key settings
	SkipCantonSigVerify bool                 `yaml:"skip_canton_sig_verify" default:"false"`
}

// RelayerServer represents the application configuration for relayer.
type RelayerServer struct {
	Server     *http.ServerConfig   `yaml:"server" validate:"required"`
	Database   *pgdb.DatabaseConfig `yaml:"database" validate:"required"`
	Ethereum   *ethereum.Config     `yaml:"ethereum" validate:"required"`
	Canton     *canton.Config       `yaml:"canton" validate:"required"`
	Bridge     *relayer.Config      `yaml:"bridge" validate:"required"`
	Monitoring *Monitoring          `yaml:"monitoring" validate:"required"`
	Logging    *log.Config          `yaml:"logging" validate:"required"`
}

// Monitoring contains monitoring and metrics settings
type Monitoring struct {
	Enabled        bool   `yaml:"enabled" default:"false"`
	MetricsPort    int    `yaml:"metrics_port" validate:"omitempty,gt=0" default:"9090"`
	HealthCheckURL string `yaml:"health_check_url" default:"/health"`
}

// JWKS contains JWKS configuration for JWT validation
type JWKS struct {
	URL    string `yaml:"url" default:""`
	Issuer string `yaml:"issuer" default:""`
}

// KeyManagement contains settings for custodial Canton key management
type KeyManagement struct {
	// MasterKeyEnv is the environment variable name containing the master encryption key (base64)
	MasterKeyEnv string `yaml:"master_key_env" validate:"required" default:"CANTON_MASTER_KEY"`
	// KeyDerivation specifies how to generate Canton keys: "generate" (random) or "derive" (from EVM + seed)
	KeyDerivation string `yaml:"key_derivation" default:"generate"` // TODO: check usages
}

// LoadAPIServer loads, defaults, and validates API app configuration from file.
func LoadAPIServer(configPath string) (*APIServer, error) {
	var cfg APIServer
	if err := loadConfigFromFile(configPath, &cfg); err != nil {
		return nil, err
	}
	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadRelayerServer loads, defaults, and validates relayer configuration from file.
func LoadRelayerServer(configPath string) (*RelayerServer, error) {
	var cfg RelayerServer
	if err := loadConfigFromFile(configPath, &cfg); err != nil {
		return nil, err
	}
	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func loadConfigFromFile(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file %q: %w", path, err)
	}

	if err = decodeYAMLStrict([]byte(os.ExpandEnv(string(data))), out); err != nil {
		return fmt.Errorf("failed to parse config file %q: %w", path, err)
	}

	if err = defaults.Set(out); err != nil {
		return fmt.Errorf("failed to apply default values for config %q: %w", path, err)
	}
	return nil
}

func decodeYAMLStrict(data []byte, out any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	if err := decoder.Decode(out); err != nil {
		return err
	}
	return nil
}

var startupValidator = newStartupValidator()

func newStartupValidator() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())

	v.RegisterTagNameFunc(func(field reflect.StructField) string {
		yamlTag := field.Tag.Get("yaml")
		if yamlTag == "" {
			return field.Name
		}

		name := strings.SplitN(yamlTag, ",", 2)[0]
		if name == "" {
			return field.Name
		}
		if name == "-" {
			return ""
		}
		return name
	})
	return v
}

func validateConfig(cfg any) error {
	if err := startupValidator.Struct(cfg); err != nil {
		return err
	}
	return nil
}
