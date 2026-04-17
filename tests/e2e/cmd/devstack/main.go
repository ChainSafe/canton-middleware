// devstack manages the Docker Compose devstack lifecycle for E2E tests.
//
// Usage:
//
//	go run ./tests/e2e/cmd/devstack <up|down>
//
// up   Starts the E2E devstack and waits for all services to be healthy.
//
//	Sets SKIP_CANTON_SIG_VERIFY=true so Canton native registration tests
//	work without a real Loop wallet signature.
//
// down Tears down the E2E devstack and removes all volumes.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	composeFile = "tests/e2e/docker-compose.e2e.yaml"
	projectName = "canton-e2e"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: devstack <up|down>\n")
		os.Exit(1)
	}
	ctx := context.Background()
	var err error
	switch os.Args[1] {
	case "up":
		err = up(ctx)
	case "down":
		err = down(ctx)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q: want up or down\n", os.Args[1])
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "devstack %s: %v\n", os.Args[1], err)
		os.Exit(1)
	}
}

func up(ctx context.Context) error {
	return compose(ctx, upEnv(),
		"up", "--build", "--wait", "--remove-orphans",
	)
}

func down(ctx context.Context) error {
	return compose(ctx, os.Environ(),
		"down", "-v", "--remove-orphans",
	)
}

// upEnv returns the host environment with SKIP_CANTON_SIG_VERIFY forced to
// "true" so that Canton native registration tests work in the devnet.
func upEnv() []string {
	const key = "SKIP_CANTON_SIG_VERIFY"
	env := os.Environ()
	for i, e := range env {
		if strings.HasPrefix(e, key+"=") {
			env[i] = key + "=true"
			return env
		}
	}
	return append(env, key+"=true")
}

func compose(ctx context.Context, env []string, args ...string) error {
	// #nosec G204 -- args are fixed constants from this file, not user input.
	cmd := exec.CommandContext(ctx, "docker", append([]string{
		"compose", "-f", composeFile, "-p", projectName,
	}, args...)...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.Env = env
	return cmd.Run()
}
