//go:build e2e

// Package docker manages the Docker Compose lifecycle and service discovery
// for the E2E test framework. It is a thin wrapper around the Docker CLI —
// no testcontainers dependency.
package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ComposeOrchestrator manages the Docker Compose stack lifecycle for E2E tests.
// It wraps the Docker CLI directly; all output is forwarded to os.Stderr so CI
// logs capture container startup failures.
type ComposeOrchestrator struct {
	composeFile string
	projectName string
}

// NewOrchestrator returns a ComposeOrchestrator for the given compose file and
// project name. composeFile should be a path relative to the repo root (e.g.
// "tests/e2e/docker-compose.e2e.yaml"). projectName is passed as -p to every
// docker compose invocation (e.g. "canton-e2e").
func NewOrchestrator(composeFile, projectName string) *ComposeOrchestrator {
	return &ComposeOrchestrator{
		composeFile: composeFile,
		projectName: projectName,
	}
}

func dockerComposeCommand(ctx context.Context, args ...string) *exec.Cmd {
	// #nosec G204 -- The E2E harness passes fixed Docker Compose args from test configuration, not shell-expanded user input.
	return exec.CommandContext(ctx, "docker", append([]string{"compose"}, args...)...)
}

// Start brings up the full stack via:
//
//	docker compose -f <file> -p <project> up --build --wait --remove-orphans
//
// All combined output is streamed to os.Stderr. Returns an error immediately
// if any container fails to reach a healthy state — no retries.
//
// SKIP_CANTON_SIG_VERIFY is forced to "true" for the E2E environment so that
// Canton native user registration tests can run without a real Loop wallet
// signature. The local devnet has no production Canton parties, so skipping
// the sig check is safe and intentional for testing purposes.
func (c *ComposeOrchestrator) Start(ctx context.Context) error {
	cmd := dockerComposeCommand(ctx,
		"-f", c.composeFile,
		"-p", c.projectName,
		"up", "--build", "--wait", "--remove-orphans",
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	// Inherit the host environment and override the Canton sig-verify flag so
	// that Canton native registration tests work without a real Loop wallet.
	cmd.Env = e2eEnv()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose up failed: %w", err)
	}
	return nil
}

// e2eEnv returns the host environment with SKIP_CANTON_SIG_VERIFY forced to
// "true". Canton native registration tests require signature verification to be
// disabled; any other value will cause those tests to fail.
func e2eEnv() []string {
	const key = "SKIP_CANTON_SIG_VERIFY"
	env := os.Environ()
	for i, e := range env {
		if strings.HasPrefix(e, key+"=") {
			if e != key+"=true" {
				fmt.Fprintf(os.Stderr, "WARNING: %s is set to %q; forcing to \"true\" for E2E tests\n", key, strings.TrimPrefix(e, key+"="))
				env[i] = key + "=true"
			}
			return env
		}
	}
	return append(env, key+"=true")
}

// Stop tears down the stack and removes all volumes via:
//
//	docker compose -f <file> -p <project> down -v --remove-orphans
//
// All combined output is streamed to os.Stderr. Leaves no dangling volumes.
func (c *ComposeOrchestrator) Stop(ctx context.Context) error {
	cmd := dockerComposeCommand(ctx,
		"-f", c.composeFile,
		"-p", c.projectName,
		"down", "-v", "--remove-orphans",
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose down failed: %w", err)
	}
	return nil
}
