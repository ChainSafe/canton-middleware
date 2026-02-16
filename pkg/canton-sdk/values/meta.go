package values

import (
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton-sdk/lapi/v2"
)

// MetaSymbol extracts token symbol from a CIP-56 meta record.
func MetaSymbol(v *lapiv2.Value) string {
	if v == nil {
		return ""
	}
	rec, ok := v.Sum.(*lapiv2.Value_Record)
	if !ok || rec.Record == nil {
		return ""
	}

	fields := RecordToMap(rec.Record)
	return Text(fields["symbol"])
}
