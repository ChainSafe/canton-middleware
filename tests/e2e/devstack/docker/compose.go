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

// Start brings up the full stack via:
//
//	docker compose -f <file> -p <project> up --build --wait --remove-orphans
//
// All combined output is streamed to os.Stderr. Returns an error immediately
// if any container fails to reach a healthy state — no retries.
func (c *ComposeOrchestrator) Start(ctx context.Context) error {
	cmd := exec.CommandContext(ctx,
		"docker", "compose",
		"-f", c.composeFile,
		"-p", c.projectName,
		"up", "--build", "--wait", "--remove-orphans",
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose up failed: %w", err)
	}
	return nil
}

// Stop tears down the stack and removes all volumes via:
//
//	docker compose -f <file> -p <project> down -v --remove-orphans
//
// All combined output is streamed to os.Stderr. Leaves no dangling volumes.
func (c *ComposeOrchestrator) Stop(ctx context.Context) error {
	cmd := exec.CommandContext(ctx,
		"docker", "compose",
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
