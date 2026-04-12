//go:build e2e

package bridge_test

import (
	"os"
	"testing"

	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
)

func TestMain(m *testing.M) {
	os.Exit(presets.DoMain(m))
}
