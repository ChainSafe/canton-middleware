package config

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/chainsafe/canton-middleware/pkg/app/http"
	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/client"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/ledger"
	"github.com/chainsafe/canton-middleware/pkg/custodial"
	"github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc"
	"github.com/chainsafe/canton-middleware/pkg/indexer"
	"github.com/chainsafe/canton-middleware/pkg/log"
	pgdb "github.com/chainsafe/canton-middleware/pkg/pgutil"
	"github.com/chainsafe/canton-middleware/pkg/reconciler"
	"github.com/chainsafe/canton-middleware/pkg/relayer"
	"github.com/chainsafe/canton-middleware/pkg/token"

	"github.com/creasty/defaults"
	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"
)

// TokenProviderMode selects which backend the token service uses for balance
// and total-supply queries.
type TokenProviderMode string

const (
	// TokenProviderCanton uses live gRPC ACS scans against the Canton ledger.
	// This is the default and requires no additional infrastructure.
	TokenProviderCanton TokenProviderMode = "canton"

	// TokenProviderIndexer reads from the indexer's pre-materialized PostgreSQL
	// tables via the indexer's HTTP admin API.  Requires the indexer process to
	// be running and reachable at IndexerProviderConfig.URL.
	TokenProviderIndexer TokenProviderMode = "indexer"
)

// IndexerProviderConfig holds the settings needed when token_provider.mode is "indexer".
type IndexerProviderConfig struct {
	// URL is the base URL of the indexer's HTTP admin API (e.g. "http://indexer:8082").
	URL string `yaml:"url" validate:"required"`
	// Instruments maps each supported token symbol (InstrumentID) to its Canton
	// instrument admin party.  The indexer keys tokens by {admin, id}, so this
	// mapping is required to translate from the Provider interface's symbol-only
	// calls to the indexer's composite key.
	//
	// Example:
	//   instruments:
	//     DEMO: "admin::abc123@domain"
	//     PROMPT: "issuer::xyz@domain"
	Instruments map[string]string `yaml:"instruments" validate:"required,min=1"`
}

// TokenProviderConfig selects and configures the token data provider.
type TokenProviderConfig struct {
	// Mode selects the provider backend.  Defaults to "canton".
	Mode TokenProviderMode `yaml:"mode" default:"canton" validate:"required,oneof=canton indexer"`
	// Indexer holds settings used when Mode is "indexer".  Must be set when
	// Mode is "indexer"; ignored otherwise.
	Indexer *IndexerProviderConfig `yaml:"indexer"`
}

// APIServer represents the ERC-20 API server configuration
type APIServer struct {
	Server              *http.ServerConfig            `yaml:"server" validate:"required"`
	Database            *pgdb.DatabaseConfig          `yaml:"database" validate:"required"`
	Canton              *canton.Config                `yaml:"canton" validate:"required"`
	Token               *token.Config                 `yaml:"token" validate:"required"`
	TokenProvider       *TokenProviderConfig          `yaml:"token_provider" default:"-"` // omit → defaults to canton mode
	EthRPC              *ethrpc.Config                `yaml:"eth_rpc" validate:"required"`
	JWKS                *JWKS                         `yaml:"jwks" default:"-"`          // nil by default (feature disabled)
	AcceptWorker        *custodial.AcceptWorkerConfig `yaml:"accept_worker" default:"-"` // nil disables the worker
	Logging             *log.Config                   `yaml:"logging" validate:"required"`
	Reconciliation      *reconciler.Config            `yaml:"reconciliation" validate:"required"`
	KeyManagement       *KeyManagement                `yaml:"key_management" validate:"required"`
	SkipCantonSigVerify bool                          `yaml:"skip_canton_sig_verify" default:"false"`
	SkipWhitelistCheck  bool                          `yaml:"skip_whitelist_check" default:"false"`
	CORSOrigins         []string                      `yaml:"cors" default:"[\"*\"]"`
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
	Enabled        bool               `yaml:"enabled" default:"false"`
	Server         *http.ServerConfig `yaml:"server" validate:"required_if=Enabled true"`
	HealthCheckURL string             `yaml:"health_check_url" default:"/health"`
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
	KeyDerivation string `yaml:"key_derivation" default:"generate" validate:"required,oneof=generate derive"`
}

// LoadAPIServer loads, defaults, and validates API app configuration from file.
func LoadAPIServer(configPath string) (*APIServer, error) {
	var cfg APIServer
	if err := loadConfigFromFile(configPath, &cfg); err != nil {
		return nil, err
	}
	if cfg.TokenProvider == nil {
		cfg.TokenProvider = &TokenProviderConfig{Mode: TokenProviderCanton}
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

// IndexerServer represents the configuration for the standalone indexer process.
// It wires a Canton ledger stream (write path) with an HTTP read API.
type IndexerServer struct {
	Server *http.ServerConfig `yaml:"server" validate:"required"`
	// Database holds PostgreSQL connection settings for the indexer DB.
	Database *pgdb.DatabaseConfig `yaml:"database" validate:"required"`
	// CantonLedger holds the Canton participant connection settings. The indexer only
	// needs the ledger gRPC connection (for streaming) — Identity, Token, and
	// Bridge sub-clients are not required.
	CantonLedger *ledger.Config  `yaml:"canton_ledger" validate:"required"`
	Indexer      *indexer.Config `yaml:"indexer" validate:"required"`
	Logging      *log.Config     `yaml:"logging" validate:"required"`
}

// LoadIndexerServer loads, defaults, and validates indexer configuration from file.
func LoadIndexerServer(configPath string) (*IndexerServer, error) {
	var cfg IndexerServer
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

	// NOTE: creasty/defaults cannot distinguish between an explicit zero value in YAML
	// and "not set". For numeric/bool fields with non-zero defaults (e.g. Timeout=10,
	// PoolSize=10, MaxRetries=5), an explicit zero in YAML will be overridden by the
	// default. This is a known limitation; explicit zero values for these fields are
	// not supported.
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

	v.RegisterStructValidation(func(sl validator.StructLevel) {
		cfg := sl.Current().Interface().(TokenProviderConfig)
		if cfg.Mode == TokenProviderIndexer && cfg.Indexer == nil {
			sl.ReportError(cfg.Indexer, "indexer", "Indexer", "required_if_indexer_mode", "")
		}
	}, TokenProviderConfig{})

	return v
}

func validateConfig(cfg any) error {
	if err := startupValidator.Struct(cfg); err != nil {
		return err
	}
	return nil
}
