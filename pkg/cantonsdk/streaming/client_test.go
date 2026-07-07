// SPDX-License-Identifier: Apache-2.0

package streaming

import (
	"testing"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
)

func createdEvent(contractID string, acsDelta bool) *lapiv2.Event {
	return &lapiv2.Event{Event: &lapiv2.Event_Created{Created: &lapiv2.CreatedEvent{
		ContractId: contractID,
		TemplateId: &lapiv2.Identifier{PackageId: "pkg", ModuleName: "Mod", EntityName: "Ent"},
		AcsDelta:   acsDelta,
	}}}
}

func exercisedEvent(contractID, choice string, consuming, acsDelta bool) *lapiv2.Event {
	return &lapiv2.Event{Event: &lapiv2.Event_Exercised{Exercised: &lapiv2.ExercisedEvent{
		ContractId: contractID,
		TemplateId: &lapiv2.Identifier{PackageId: "pkg", ModuleName: "Mod", EntityName: "Ent"},
		Choice:     choice,
		Consuming:  consuming,
		AcsDelta:   acsDelta,
	}}}
}

// TestDecodeLedgerEvent_AcsDeltaGate pins the visibility semantics of the
// LEDGER_EFFECTS stream decode: only events flagged AcsDelta — the ones an
// ACS_DELTA stream would have delivered — are decoded. Witnessed-only events
// and transient contracts (AcsDelta=false) and non-consuming exercises are
// dropped, so balance accounting never sees one end of a contract lifecycle
// without the other.
func TestDecodeLedgerEvent_AcsDeltaGate(t *testing.T) {
	cases := []struct {
		name       string
		ev         *lapiv2.Event
		want       bool // decoded at all
		wantCreate bool
		wantChoice string
	}{
		{"created stakeholder", createdEvent("c1", true), true, true, ""},
		{"created witnessed-only or transient", createdEvent("c2", false), false, false, ""},
		{"consuming exercise on ACS contract", exercisedEvent("c3", "TransferInstruction_Withdraw", true, true), true, false, "TransferInstruction_Withdraw"},
		{"consuming exercise witnessed-only", exercisedEvent("c4", "TransferInstruction_Withdraw", true, false), false, false, ""},
		{"non-consuming exercise", exercisedEvent("c5", "SomeQuery", false, true), false, false, ""},
		{"archived event (ACS_DELTA shape)", &lapiv2.Event{Event: &lapiv2.Event_Archived{Archived: &lapiv2.ArchivedEvent{ContractId: "c6"}}}, false, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			le := decodeLedgerEvent(tc.ev)
			if (le != nil) != tc.want {
				t.Fatalf("decoded=%v, want %v", le != nil, tc.want)
			}
			if le == nil {
				return
			}
			if le.IsCreated != tc.wantCreate {
				t.Fatalf("IsCreated=%v, want %v", le.IsCreated, tc.wantCreate)
			}
			if le.Choice != tc.wantChoice {
				t.Fatalf("Choice=%q, want %q", le.Choice, tc.wantChoice)
			}
		})
	}
}
