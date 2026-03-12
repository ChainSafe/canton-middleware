package pgutil

// DatabaseConfig contains database connection settings
type DatabaseConfig struct {
	URL      string `yaml:"url" validate:"required"`
	SSLMode  string `yaml:"ssl_mode" validate:"required"`
	Timeout  int    `yaml:"timeout" default:"10"`
	PoolSize int    `yaml:"pool_size" default:"10"`
}

// GetConnectionString returns a PostgreSQL connection string
func (c *DatabaseConfig) GetConnectionString() string {
	return c.URL
}
