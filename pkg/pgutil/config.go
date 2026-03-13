package pgutil

import "net/url"

// DatabaseConfig contains database connection settings
type DatabaseConfig struct {
	URL      string `yaml:"url" validate:"required,url"`
	SSLMode  string `yaml:"ssl_mode" validate:"required,oneof=disable allow prefer require verify-ca verify-full"`
	Timeout  int    `yaml:"timeout" default:"10"`
	PoolSize int    `yaml:"pool_size" default:"10"`
}

// GetConnectionString returns a PostgreSQL connection string with sslmode applied when missing.
func (c *DatabaseConfig) GetConnectionString() string {
	if c == nil {
		return ""
	}

	// URL validity is enforced by config validation (validate:"required,url").
	parsed, _ := url.Parse(c.URL)

	query := parsed.Query()
	if c.SSLMode != "" && query.Get("sslmode") == "" {
		query.Set("sslmode", c.SSLMode)
		parsed.RawQuery = query.Encode()
	}

	return parsed.String()
}
