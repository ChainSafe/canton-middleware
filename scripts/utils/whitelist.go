//go:build ignore

// whitelist.go — Interactive EVM address whitelisting utility.
//
// Reads all config values automatically from the running Docker stack
// (same approach as fund-wallet.go). No config flags required.
//
// Usage (interactive):
//
//	go run scripts/utils/whitelist.go
//
// Usage (non-interactive):
//
//	go run scripts/utils/whitelist.go -address 0xABCD... -note "operator"

package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/pgutil"
	"github.com/chainsafe/canton-middleware/pkg/userstore"
	_ "github.com/lib/pq"
)

const (
	containerName = "erc20-api-server"
	resolvedCfg   = "/app/state/api-server-config.yaml"
)

func main() {
	// Parse optional flags for non-interactive use.
	var addrFlag, noteFlag string
	for i, arg := range os.Args[1:] {
		switch {
		case arg == "-address" || arg == "--address":
			if i+1 < len(os.Args[1:]) {
				addrFlag = os.Args[i+2]
			}
		case strings.HasPrefix(arg, "-address="):
			addrFlag = strings.TrimPrefix(arg, "-address=")
		case strings.HasPrefix(arg, "--address="):
			addrFlag = strings.TrimPrefix(arg, "--address=")
		case arg == "-note" || arg == "--note":
			if i+1 < len(os.Args[1:]) {
				noteFlag = os.Args[i+2]
			}
		case strings.HasPrefix(arg, "-note="):
			noteFlag = strings.TrimPrefix(arg, "-note=")
		case strings.HasPrefix(arg, "--note="):
			noteFlag = strings.TrimPrefix(arg, "--note=")
		}
	}

	fmt.Println("════════════════════════════════════════════════════════════════════")
	fmt.Println("  Whitelist Manager")
	fmt.Println("════════════════════════════════════════════════════════════════════")
	fmt.Println()

	cfg, err := loadConfigFromDocker()
	if err != nil {
		fatalf("failed to load config from Docker: %v", err)
	}

	fmt.Println(">>> Connecting to database...")
	bunDB, err := pgutil.ConnectDB(cfg.Database)
	if err != nil {
		fatalf("failed to connect to database: %v", err)
	}
	defer bunDB.Close()
	store := userstore.NewStore(bunDB)
	fmt.Println("    Connected to PostgreSQL")
	fmt.Println()

	ctx := context.Background()
	reader := bufio.NewReader(os.Stdin)

	rawAddr := strings.TrimSpace(addrFlag)
	rawNote := strings.TrimSpace(noteFlag)

	if rawAddr == "" {
		fmt.Print("Enter EVM address to whitelist: ")
		line, _ := reader.ReadString('\n')
		rawAddr = strings.TrimSpace(line)
	}
	if rawAddr == "" {
		fatalf("address is required")
	}

	normalized := auth.NormalizeAddress(rawAddr)
	if normalized == "0x0000000000000000000000000000000000000000" {
		fatalf("invalid EVM address: %q", rawAddr)
	}
	if normalized != rawAddr {
		fmt.Printf("  Normalized: %s\n", normalized)
	}

	already, err := store.IsWhitelisted(ctx, normalized)
	if err != nil {
		fatalf("failed to check whitelist: %v", err)
	}

	if err := store.AddToWhitelist(ctx, normalized, rawNote); err != nil {
		fatalf("failed to add to whitelist: %v", err)
	}

	fmt.Println()
	if already {
		fmt.Printf("  Updated:  %s\n", normalized)
	} else {
		fmt.Printf("  Added:    %s\n", normalized)
	}
	if rawNote != "" {
		fmt.Printf("  Note:     %s\n", rawNote)
	}
	fmt.Println()
	fmt.Println("════════════════════════════════════════════════════════════════════")
	fmt.Println("  Done")
	fmt.Println("════════════════════════════════════════════════════════════════════")
}

// loadConfigFromDocker pulls config values from the running Docker stack:
// - env vars (DATABASE_URL, auth credentials) from the api-server container
// - resolved YAML (domain_id, issuer_party) from the container's state volume
// Docker-internal hostnames are rewritten to localhost for host-side access.
func loadConfigFromDocker() (*config.APIServer, error) {
	// 1. Read env vars from the running container and set them locally so
	//    config.LoadAPIServer can expand ${VAR} placeholders via os.ExpandEnv.
	envOut, err := exec.Command("docker", "inspect", containerName,
		"--format", "{{range .Config.Env}}{{println .}}{{end}}").Output()
	if err != nil {
		return nil, fmt.Errorf("docker inspect %s failed: %w\n(is the Docker stack running?)", containerName, err)
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
	//    This file already has domain_id and issuer_party substituted by the
	//    bootstrap script; remaining ${VAR} placeholders are expanded by
	//    config.LoadAPIServer via os.ExpandEnv using the env vars set above.
	yamlOut, err := exec.Command("docker", "exec", containerName, "cat", resolvedCfg).Output()
	if err != nil {
		return nil, fmt.Errorf("docker exec cat %s failed: %w", resolvedCfg, err)
	}

	// Rewrite docker-internal service hostnames to localhost for host access.
	resolved := string(yamlOut)
	resolved = strings.ReplaceAll(resolved, "canton:5011", "localhost:5011")
	resolved = strings.ReplaceAll(resolved, "mock-oauth2:8088", "localhost:8088")

	// 3. Write patched YAML to a temp file and load via the standard loader.
	tmp, err := os.CreateTemp("", "whitelist-*.yaml")
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

func fatalf(format string, args ...any) {
	fmt.Printf("ERROR: "+format+"\n", args...)
	os.Exit(1)
}
