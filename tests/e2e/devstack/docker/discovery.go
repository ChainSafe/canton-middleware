//go:build e2e

package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
)

const (
	anvilPort      = 8545
	cantonGRPCPort = 5011
	cantonHTTPPort = 5013
	apiServerPort  = 8081
	relayerPort    = 8080
	indexerPort    = 8082
	mockOAuth2Port = 8088
	postgresPort   = 5432
)

// ServiceDiscovery resolves running container ports and reads the bootstrap
// deploy manifest to produce a fully populated stack.ServiceManifest.
type ServiceDiscovery struct {
	projectName string
}

// NewServiceDiscovery returns a ServiceDiscovery scoped to the given Docker
// Compose project (e.g. "canton-e2e").
func NewServiceDiscovery(projectName string) *ServiceDiscovery {
	return &ServiceDiscovery{projectName: projectName}
}

// deployManifest mirrors the JSON written by scripts/setup/docker-bootstrap.sh
// to /tmp/e2e-deploy.json inside the e2e-deploy volume.
type deployManifest struct {
	PromptToken           string `json:"prompt_token"`
	CantonBridge          string `json:"canton_bridge"`
	PromptInstrumentAdmin string `json:"prompt_instrument_admin"`
	PromptInstrumentID    string `json:"prompt_instrument_id"`
	DemoInstrumentAdmin   string `json:"demo_instrument_admin"`
	DemoInstrumentID      string `json:"demo_instrument_id"`
}

// Manifest resolves all service endpoints and contract addresses from the
// running Docker Compose project and returns a fully populated ServiceManifest.
//
// All subprocess calls (docker compose port, docker inspect, docker compose run)
// are issued concurrently via errgroup to minimize wall-clock time at test-suite
// startup.
//
// DSNs are read directly from each service's own environment variables
// (RELAYER_DATABASE_URL, API_SERVER_DATABASE_URL, INDEXER_DATABASE_URL) via
// docker inspect, with the internal hostname replaced by the published
// localhost:PORT. This avoids hardcoding credentials or database names.
func (d *ServiceDiscovery) Manifest(ctx context.Context) (*stack.ServiceManifest, error) {
	// Phase 1: resolve all endpoints and the postgres host in parallel.
	var (
		anvilRPC     string
		cantonGRPC   string
		cantonHTTP   string
		apiHTTP      string
		relayerHTTP  string
		indexerHTTP  string
		oauthHTTP    string
		postgresHost string
		dm           *deployManifest
	)

	g1, gctx := errgroup.WithContext(ctx)
	g1.Go(func() (err error) { anvilRPC, err = d.httpEndpoint(gctx, "anvil", anvilPort); return })
	g1.Go(func() (err error) { cantonGRPC, err = d.tcpEndpoint(gctx, "canton", cantonGRPCPort); return })
	g1.Go(func() (err error) { cantonHTTP, err = d.httpEndpoint(gctx, "canton", cantonHTTPPort); return })
	g1.Go(func() (err error) { apiHTTP, err = d.httpEndpoint(gctx, "api-server", apiServerPort); return })
	g1.Go(func() (err error) { relayerHTTP, err = d.httpEndpoint(gctx, "relayer", relayerPort); return })
	g1.Go(func() (err error) { indexerHTTP, err = d.httpEndpoint(gctx, "indexer", indexerPort); return })
	g1.Go(func() (err error) { oauthHTTP, err = d.httpEndpoint(gctx, "mock-oauth2", mockOAuth2Port); return })
	g1.Go(func() (err error) { postgresHost, err = d.publishedPort(gctx, "postgres", postgresPort); return })
	g1.Go(func() (err error) { dm, err = d.readDeployManifest(gctx); return })
	if err := g1.Wait(); err != nil {
		return nil, err
	}

	apiDSN, err := d.serviceDSN(ctx, "api-server", "API_SERVER_DATABASE_URL", postgresHost)
	if err != nil {
		return nil, err
	}

	return &stack.ServiceManifest{
		AnvilRPC:              anvilRPC,
		CantonGRPC:            cantonGRPC,
		CantonHTTP:            cantonHTTP,
		APIHTTP:               apiHTTP,
		RelayerHTTP:           relayerHTTP,
		IndexerHTTP:           indexerHTTP,
		OAuthHTTP:             oauthHTTP,
		APIDatabaseDSN:        apiDSN,
		PromptTokenAddr:       dm.PromptToken,
		BridgeAddr:            dm.CantonBridge,
		PromptInstrumentAdmin: dm.PromptInstrumentAdmin,
		PromptInstrumentID:    dm.PromptInstrumentID,
		DemoInstrumentAdmin:   dm.DemoInstrumentAdmin,
		DemoInstrumentID:      dm.DemoInstrumentID,
	}, nil
}

// serviceDSN reads the named environment variable from the running service
// container via docker inspect, then rewrites the host to the published
// postgresHost (localhost:PORT) so the DSN is usable from outside Docker.
//
// docker compose uses the service name ("postgres") as the hostname inside the
// container network; we replace it with the resolved external address.
func (d *ServiceDiscovery) serviceDSN(ctx context.Context, service, envVar, postgresHost string) (string, error) {
	raw, err := d.containerEnv(ctx, service, envVar)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parsing %s from %s: %w", envVar, service, err)
	}
	u.Host = postgresHost
	return u.String(), nil
}

// containerEnv returns the value of an environment variable from the running
// container for the given Compose service. It uses docker inspect on the
// container ID returned by `docker compose ps -q`:
//
//	docker inspect <id> --format '{{range .Config.Env}}{{println .}}{{end}}'
func (d *ServiceDiscovery) containerEnv(ctx context.Context, service, key string) (string, error) {
	// Resolve container ID.
	psCmd := dockerComposeCommand(ctx,
		"-p", d.projectName,
		"ps", "-q", service,
	)
	var psOut, psErr bytes.Buffer
	psCmd.Stdout = &psOut
	psCmd.Stderr = &psErr
	if err := psCmd.Run(); err != nil {
		return "", fmt.Errorf("docker compose ps -q %s: %w — %s", service, err, psErr.String())
	}
	// strings.Fields splits on all whitespace, handling both the single-ID case
	// and the multi-ID case (scaled replicas or orphaned containers from a prior
	// run). We take the first ID; docker inspect expects exactly one argument.
	ids := strings.Fields(psOut.String())
	if len(ids) == 0 {
		return "", fmt.Errorf("no running container found for service %q", service)
	}
	containerID := ids[0]

	// Read all env vars from the container.
	inspectCmd := exec.CommandContext(ctx,
		"docker", "inspect", containerID,
		"--format", "{{range .Config.Env}}{{println .}}{{end}}",
	)
	var inspectOut, inspectErr bytes.Buffer
	inspectCmd.Stdout = &inspectOut
	inspectCmd.Stderr = &inspectErr
	if err := inspectCmd.Run(); err != nil {
		return "", fmt.Errorf("docker inspect %s: %w — %s", containerID, err, inspectErr.String())
	}

	prefix := key + "="
	for _, line := range strings.Split(inspectOut.String(), "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix), nil
		}
	}
	return "", fmt.Errorf("env var %q not found in container %s (%s)", key, containerID, service)
}

// httpEndpoint returns "http://host:port" for the given service and container port.
func (d *ServiceDiscovery) httpEndpoint(ctx context.Context, service string, containerPort int) (string, error) {
	hostPort, err := d.publishedPort(ctx, service, containerPort)
	if err != nil {
		return "", err
	}
	return "http://" + hostPort, nil
}

// tcpEndpoint returns "host:port" (no scheme) for the given service and container port.
func (d *ServiceDiscovery) tcpEndpoint(ctx context.Context, service string, containerPort int) (string, error) {
	return d.publishedPort(ctx, service, containerPort)
}

// publishedPort executes `docker compose -p <project> port <service> <port>`
// and returns the resolved "host:port" string (e.g. "0.0.0.0:54321" →
// "localhost:54321").
func (d *ServiceDiscovery) publishedPort(ctx context.Context, service string, containerPort int) (string, error) {
	cmd := dockerComposeCommand(ctx,
		"-p", d.projectName,
		"port", service, fmt.Sprintf("%d", containerPort),
	)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker compose port %s %d: %w — %s", service, containerPort, err, errBuf.String())
	}
	raw := strings.TrimSpace(out.String())
	if raw == "" {
		return "", fmt.Errorf("docker compose port %s %d: no output", service, containerPort)
	}
	// docker compose port outputs "0.0.0.0:PORT" or ":::PORT" — normalise to
	// localhost:PORT so connections work on all platforms.
	if idx := strings.LastIndex(raw, ":"); idx >= 0 {
		raw = "localhost:" + raw[idx+1:]
	}
	return raw, nil
}

// readDeployManifest reads /tmp/e2e-deploy.json by running a short-lived
// bootstrap container. The bootstrap service already has the e2e-deploy volume
// mounted at /tmp in the compose definition, so docker compose run inherits it:
//
//	docker compose -p <project> run --rm bootstrap cat /tmp/e2e-deploy.json
func (d *ServiceDiscovery) readDeployManifest(ctx context.Context) (*deployManifest, error) {
	cmd := dockerComposeCommand(ctx,
		"-p", d.projectName,
		"run", "--rm",
		"bootstrap",
		"cat", "/tmp/e2e-deploy.json",
	)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("reading e2e-deploy.json: %w — %s", err, errBuf.String())
	}
	var dm deployManifest
	if err := json.Unmarshal(out.Bytes(), &dm); err != nil {
		return nil, fmt.Errorf("parsing e2e-deploy.json: %w", err)
	}
	return &dm, nil
}
