package log

// Config contains logging settings
type Config struct {
	Level      string `yaml:"level" validate:"required,oneof=debug info warn error dpanic panic fatal"`
	Format     string `yaml:"format" validate:"required,oneof=json console"`
	OutputPath string `yaml:"output_path" default:"stdout"`
}
