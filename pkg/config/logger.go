package config

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger creates a new logger based on configuration
func NewLogger(cfg LoggingConfig) (*zap.Logger, error) {
	var zapConfig zap.Config

	if cfg.Format == "json" {
		zapConfig = zap.NewProductionConfig()
	} else {
		zapConfig = zap.NewDevelopmentConfig()
		zapConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	// Set log level
	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		return nil, fmt.Errorf("invalid log level: %w", err)
	}
	zapConfig.Level = zap.NewAtomicLevelAt(level)

	// Set output paths
	if cfg.OutputPath != "" && cfg.OutputPath != "stdout" {
		zapConfig.OutputPaths = []string{cfg.OutputPath}
	}

	logger, err := zapConfig.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build logger: %w", err)
	}

	return logger, nil
}
