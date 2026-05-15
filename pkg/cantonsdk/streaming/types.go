// Package streaming provides a reusable, generic Canton ledger streaming client.
//
// It wraps UpdateService.GetUpdates with automatic reconnection, exponential backoff,
// and auth-token invalidation on 401.
package streaming

import (
	"time"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"
)

// TemplateID identifies a DAML template by its package, module, and entity name.
// It is the streaming package's own type — callers do not import lapiv2 directly.
type TemplateID struct {
	PackageID  string
	ModuleName string
	EntityName string
}

// SubscribeRequest configures which templates to stream and from which ledger offset.
type SubscribeRequest struct {
	// FromOffset is the exclusive start offset. Use 0 to start from the beginning.
	FromOffset int64

	// TemplateIDs lists the DAML templates to subscribe to.
	TemplateIDs []TemplateID
}

// LedgerTransaction is a decoded transaction received from the Canton GetUpdates stream.
type LedgerTransaction struct {
	UpdateID      string
	Offset        int64
	EffectiveTime time.Time
	Events        []*LedgerEvent
}

// LedgerEvent is a single created or archived contract event within a transaction.
// All DAML field access goes through typed accessor methods — no lapiv2 types are exposed.
type LedgerEvent struct {
	ContractID   string
	PackageID    string
	ModuleName   string
	TemplateName string

	// IsCreated is true for contract create events, false for archive events.
	IsCreated bool

	// fields holds the pre-decoded DAML record from CreateArguments, keyed by field label.
	// Only populated for created events; nil for archived events.
	fields map[string]*lapiv2.Value
}

// NewLedgerEvent constructs a LedgerEvent with pre-decoded fields.
// Used by tests that need to build events without going through the proto decode path.
// Accepts FieldValue values produced by the Make* constructor functions so that
// callers have no direct dependency on lapiv2.
func NewLedgerEvent(contractID, packageID, moduleName, templateName string, isCreated bool, fields map[string]FieldValue) *LedgerEvent {
	inner := make(map[string]*lapiv2.Value, len(fields))
	for k, v := range fields {
		inner[k] = v.v
	}
	return &LedgerEvent{
		ContractID:   contractID,
		PackageID:    packageID,
		ModuleName:   moduleName,
		TemplateName: templateName,
		IsCreated:    isCreated,
		fields:       inner,
	}
}

// TextField returns the named DAML Text field as a Go string.
// Returns "" when the field is absent or not of type Text.
func (e *LedgerEvent) TextField(name string) string {
	return values.Text(e.fields[name])
}

// PartyField returns the named DAML Party field as a string.
// Returns "" when the field is absent or not of type Party.
func (e *LedgerEvent) PartyField(name string) string {
	return values.Party(e.fields[name])
}

// NumericField returns the named DAML Numeric field as a decimal string (e.g. "1.5").
// Returns "0" when the field is absent or not of type Numeric.
func (e *LedgerEvent) NumericField(name string) string {
	return values.Numeric(e.fields[name])
}

// OptionalTextField returns the inner Text value of a DAML Optional Text field.
// Returns "" for None or when the field is absent.
func (e *LedgerEvent) OptionalTextField(name string) string {
	return values.OptionalText(e.fields[name])
}

// OptionalPartyField returns the inner Party value of a DAML Optional Party field.
// Returns "" for None or when the field is absent.
func (e *LedgerEvent) OptionalPartyField(name string) string {
	return values.OptionalParty(e.fields[name])
}

// IsNone returns true if the named DAML Optional field holds None.
func (e *LedgerEvent) IsNone(name string) bool {
	return values.IsNone(e.fields[name])
}

// TimestampField returns the named DAML Time field as a Go time.Time.
// Returns zero time when the field is absent or not of type Timestamp.
func (e *LedgerEvent) TimestampField(name string) time.Time {
	return values.Timestamp(e.fields[name])
}

// NestedTextField accesses a Text sub-field inside a named DAML Record field.
// Example: event.NestedTextField("instrumentId", "id")
// Returns "" when the outer field is absent or not a Record.
func (e *LedgerEvent) NestedTextField(record, field string) string {
	return values.NestedTextField(e.fields[record], field)
}

// NestedPartyField accesses a Party sub-field inside a named DAML Record field.
// Example: event.NestedPartyField("instrumentId", "admin")
// Returns "" when the outer field is absent or not a Record.
func (e *LedgerEvent) NestedPartyField(record, field string) string {
	return values.NestedPartyField(e.fields[record], field)
}

// NestedNumericField accesses a Numeric sub-field inside a named DAML Record field.
// Example: event.NestedNumericField("transfer", "amount")
// Returns "0" when the outer field is absent, the inner field is missing, or
// the inner field is not a Numeric.
func (e *LedgerEvent) NestedNumericField(record, field string) string {
	return values.Numeric(values.RecordField(e.fields[record])[field])
}

// DoublyNestedPartyField accesses a Party field two records deep.
// Example: event.DoublyNestedPartyField("transfer", "instrumentId", "admin")
// Returns "" when any of the path segments is absent or not a Record.
func (e *LedgerEvent) DoublyNestedPartyField(outer, middle, field string) string {
	mid := values.RecordField(e.fields[outer])
	if mid == nil {
		return ""
	}
	return values.NestedPartyField(mid[middle], field)
}

// DoublyNestedTextField accesses a Text field two records deep.
// Example: event.DoublyNestedTextField("transfer", "instrumentId", "id")
// Returns "" when any of the path segments is absent or not a Record.
func (e *LedgerEvent) DoublyNestedTextField(outer, middle, field string) string {
	mid := values.RecordField(e.fields[outer])
	if mid == nil {
		return ""
	}
	return values.NestedTextField(mid[middle], field)
}

// OptionalMetaLookup looks up a string key within an Optional Metadata field.
// Metadata is encoded as Optional(Record{values: Map Text Text}).
// Returns "" when the Optional is None, the key is absent, or the field is absent.
func (e *LedgerEvent) OptionalMetaLookup(metaField, key string) string {
	inner := values.OptionalRecordFields(e.fields[metaField])
	if inner == nil {
		return ""
	}
	return values.MapLookupText(inner["values"], key)
}
