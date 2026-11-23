// SPDX-License-Identifier: Apache-2.0

package config

import (
	"path/filepath"
	"runtime"
	"testing"
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
