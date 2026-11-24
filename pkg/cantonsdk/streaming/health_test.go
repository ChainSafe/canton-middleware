// SPDX-License-Identifier: Apache-2.0

package streaming

import "testing"

func TestStreamHealth(t *testing.T) {
	var h streamHealth
	if h.get() {
		t.Fatal("healthy before any stream")
	}

	first := h.begin()
	if h.get() {
		t.Fatal("healthy before the stream delivered anything")
	}
	h.markHealthy(first)
	if !h.get() {
		t.Fatal("not healthy after markHealthy")
	}

	h.end(first)
	if h.get() {
		t.Fatal("healthy after the stream ended")
	}

	// A timer from the ended run fires late: it must not mark the client healthy.
	h.markHealthy(first)
	if h.get() {
		t.Fatal("stale run marked the client healthy")
	}

	second := h.begin()
	h.markHealthy(first)
	if h.get() {
		t.Fatal("stale run marked the new stream healthy")
	}
	h.markHealthy(second)
	if !h.get() {
		t.Fatal("new stream not healthy after markHealthy")
	}
}
