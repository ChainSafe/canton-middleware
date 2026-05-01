// Package dockerconfig loads the api-server config from a running Docker stack.
// Used by developer utility scripts that need database / Canton connectivity
// without a manually specified config path.
package dockerconfig

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/chainsafe/canton-middleware/pkg/config"
)

const (
	// ContainerName is the api-server container used as the config source.
	ContainerName = "erc20-api-server"

	resolvedCfg = "/app/state/api-server-config.yaml"
)

// Load pulls config from the running Docker stack:
//   - env vars are read from the api-server container via docker inspect and
//     exported into the current process so ${VAR} placeholders expand correctly
//   - the bootstrap-resolved YAML is fetched via docker exec cat and docker-
//     internal hostnames are rewritten to localhost for host-side access
func Load() (*config.APIServer, error) {
	// 1. Read env vars from the running container.
	envOut, err := exec.Command("docker", "inspect", ContainerName,
		"--format", "{{range .Config.Env}}{{println .}}{{end}}").Output()
	if err != nil {
		return nil, fmt.Errorf("docker inspect %s failed: %w\n(is the Docker stack running?)", ContainerName, err)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(envOut)), "\n") {
		if idx := strings.IndexByte(line, '='); idx > 0 {
			os.Setenv(line[:idx], line[idx+1:])
		}
	}

	// Fix DATABASE_URL: container uses postgres hostname, host machine uses localhost.
	if dbURL := os.Getenv("API_SERVER_DATABASE_URL"); dbURL != "" {
		os.Setenv("API_SERVER_DATABASE_URL", strings.ReplaceAll(dbURL, "@postgres:", "@localhost:"))
	}

	// 2. Read the bootstrap-resolved config YAML from the shared state volume.
	yamlOut, err := exec.Command("docker", "exec", ContainerName, "cat", resolvedCfg).Output()
	if err != nil {
		return nil, fmt.Errorf("docker exec cat %s failed: %w", resolvedCfg, err)
	}

	// Rewrite docker-internal service hostnames to localhost for host access.
	resolved := string(yamlOut)
	resolved = strings.ReplaceAll(resolved, "canton:5011", "localhost:5011")
	resolved = strings.ReplaceAll(resolved, "mock-oauth2:8088", "localhost:8088")

	// 3. Write patched YAML to a temp file and load via the standard loader.
	tmp, err := os.CreateTemp("", "devtool-*.yaml")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(resolved); err != nil {
		return nil, err
	}
	tmp.Close()

	return config.LoadAPIServer(tmp.Name())
}
