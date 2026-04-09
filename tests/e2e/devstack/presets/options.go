//go:build e2e

package presets

const (
	defaultComposeFile = "tests/e2e/docker-compose.e2e.yaml"
	defaultProjectName = "canton-e2e"
)

// options holds the configurable parameters for the devstack.
type options struct {
	composeFile string
	projectName string
}

// Option is a functional option for the devstack presets.
type Option func(*options)

// WithComposeFile overrides the default Docker Compose file path.
func WithComposeFile(path string) Option {
	return func(o *options) {
		o.composeFile = path
	}
}

// WithProjectName overrides the default Docker Compose project name.
func WithProjectName(name string) Option {
	return func(o *options) {
		o.projectName = name
	}
}

func applyOptions(opts []Option) options {
	o := options{
		composeFile: defaultComposeFile,
		projectName: defaultProjectName,
	}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}
