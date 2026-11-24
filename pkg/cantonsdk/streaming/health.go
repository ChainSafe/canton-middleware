// SPDX-License-Identifier: Apache-2.0

package streaming

import (
	"sync"
	"time"
)

// streamHealthyAfter is how long a new stream must stay open without error before it counts as
// connected, since an idle ledger may send nothing. Auth and dial errors arrive well before this.
const streamHealthyAfter = 10 * time.Second

// streamHealth tracks whether the current stream is connected. Each stream run gets a new
// generation, so a timer left over from an ended run can't mark a later one healthy.
type streamHealth struct {
	mu      sync.Mutex
	gen     uint64
	healthy bool
}

func (h *streamHealth) begin() uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.gen++
	h.healthy = false
	return h.gen
}

func (h *streamHealth) markHealthy(gen uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if gen == h.gen {
		h.healthy = true
	}
}

func (h *streamHealth) end(gen uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if gen == h.gen {
		h.gen++
		h.healthy = false
	}
}

func (h *streamHealth) get() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.healthy
}
