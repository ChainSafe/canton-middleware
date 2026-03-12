package http

import "time"

// ServerConfig contains HTTP server settings
type ServerConfig struct {
	Host            string        `yaml:"host" validate:"required"`
	Port            int           `yaml:"port" validate:"required,gt=0"`
	ReadTimeout     time.Duration `yaml:"read_timeout" default:"15s"`
	WriteTimeout    time.Duration `yaml:"write_timeout" default:"15s"`
	IdleTimeout     time.Duration `yaml:"idle_timeout" default:"60s"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" default:"30s"`
}
