// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// TestLoadKubernetesConfigFiles keeps the configs shipped with deployments/kubernetes
// loadable by the strict loaders, so the manifests cannot drift from the config structs.
func TestLoadKubernetesConfigFiles(t *testing.T) {
	setDefaultConfigEnv(t)
	t.Setenv("INDEXER_DATABASE_URL", "postgres://postgres:pass@localhost:5432/canton_indexer")

	t.Run("api-server", func(t *testing.T) {
		if _, err := LoadAPIServer(kubernetesConfigPath(t, "api-server.yaml")); err != nil {
			t.Fatalf("load kubernetes api-server config: %v", err)
		}
	})

	// The USDCx entries ship commented out; uncommented, they must still load.
	t.Run("api-server with USDCx", func(t *testing.T) {
		src, err := os.ReadFile(kubernetesConfigPath(t, "api-server.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "api-server.yaml")
		if err = os.WriteFile(path, uncommentConfigLines(string(src)), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadAPIServer(path)
		if err != nil {
			t.Fatalf("load kubernetes api-server config with USDCx: %v", err)
		}
		usdcx, ok := cfg.Token.SupportedTokens[common.HexToAddress("0xC1A0000000000000000000000000000000000001")]
		if !ok || !usdcx.ExternalTransfer {
			t.Fatalf("USDCx supported token missing or not external_transfer: %+v", usdcx)
		}
		if _, ok := cfg.TokenProvider.Indexer.Instruments["USDCx"]; !ok {
			t.Fatal("USDCx missing from token_provider.indexer.instruments")
		}
		if len(cfg.Canton.Token.ExternalTokens) != 1 {
			t.Fatalf("want 1 external token, got %d", len(cfg.Canton.Token.ExternalTokens))
		}
	})

	t.Run("indexer", func(t *testing.T) {
		if _, err := LoadIndexerServer(kubernetesConfigPath(t, "indexer.yaml")); err != nil {
			t.Fatalf("load kubernetes indexer config: %v", err)
		}
	})

	t.Run("relayer", func(t *testing.T) {
		if _, err := LoadRelayerServer(kubernetesConfigPath(t, "relayer.yaml")); err != nil {
			t.Fatalf("load kubernetes relayer config: %v", err)
		}
	})
}

func kubernetesConfigPath(t *testing.T, fileName string) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "deployments", "kubernetes", "config", fileName)
}

// commentedKey matches a commented-out YAML key line ("# key:" or "# \"key\":"), not prose comments.
var commentedKey = regexp.MustCompile(`^(\s*)# (\s*"?[\w.:-]+"?:.*)$`)

func uncommentConfigLines(src string) []byte {
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		lines[i] = commentedKey.ReplaceAllString(line, "$1$2")
	}
	return []byte(strings.Join(lines, "\n"))
}
