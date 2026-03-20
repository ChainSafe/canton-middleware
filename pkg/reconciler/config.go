package reconciler

import "time"

// Config contains settings for balance reconciliation
type Config struct {
	InitialTimeout time.Duration `yaml:"initial_timeout" default:"2m"`
	Interval       time.Duration `yaml:"interval" default:"5m"`
}
