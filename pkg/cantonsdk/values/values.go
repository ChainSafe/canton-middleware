// Package values provides helper utilities for working with
// Canton Ledger API value types.
package values

import (
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
)

// RecordToMap converts a Ledger API record into a map keyed by field label.
// Fields without labels are ignored.
func RecordToMap(r *lapiv2.Record) map[string]*lapiv2.Value {
	out := make(map[string]*lapiv2.Value)
	if r == nil {
		return out
	}
	for _, f := range r.Fields {
		if f.Label == "" {
			continue
		}
		out[f.Label] = f.Value
	}
	return out
}
