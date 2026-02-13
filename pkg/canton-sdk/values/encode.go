package values

import (
	"time"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
)

// TextValue returns a text ledger value.
func TextValue(v string) *lapiv2.Value {
	return &lapiv2.Value{
		Sum: &lapiv2.Value_Text{
			Text: v,
		},
	}
}

// PartyValue returns a party ledger value.
func PartyValue(v string) *lapiv2.Value {
	return &lapiv2.Value{
		Sum: &lapiv2.Value_Party{
			Party: v,
		},
	}
}

// NumericValue returns a numeric ledger value.
func NumericValue(v string) *lapiv2.Value {
	return &lapiv2.Value{
		Sum: &lapiv2.Value_Numeric{
			Numeric: v,
		},
	}
}

// ContractIDValue returns a contract ID ledger value.
func ContractIDValue(v string) *lapiv2.Value {
	return &lapiv2.Value{
		Sum: &lapiv2.Value_ContractId{
			ContractId: v,
		},
	}
}

// TimestampValue returns a timestamp ledger value.
func TimestampValue(t time.Time) *lapiv2.Value {
	return &lapiv2.Value{
		Sum: &lapiv2.Value_Timestamp{
			Timestamp: t.UnixMicro(),
		},
	}
}

// None returns an empty optional value.
func None() *lapiv2.Value {
	return &lapiv2.Value{
		Sum: &lapiv2.Value_Optional{
			Optional: nil,
		},
	}
}

// Optional wraps an optional value.
func Optional(v *lapiv2.Value) *lapiv2.Value {
	return &lapiv2.Value{
		Sum: &lapiv2.Value_Optional{
			Optional: &lapiv2.Optional{Value: v},
		},
	}
}
