package log

// Config contains logging settings
type Config struct {
	Level      string `yaml:"level" validate:"required"`
	Format     string `yaml:"format" validate:"required"`
	OutputPath string `yaml:"output_path" default:"stdout"`
}
