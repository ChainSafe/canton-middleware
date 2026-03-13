package indexer

import (
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/streaming"

	"go.uber.org/zap"
)

const (
	tokenTransferEventModule = "CIP56.Events"
	tokenTransferEventEntity = "TokenTransferEvent"

	// Metadata keys for bridge context stored in TokenTransferEvent.meta.values.
	metaKeyExternalTxID    = "bridge.externalTxId"
	metaKeyExternalAddress = "bridge.externalAddress"
	metaKeyFingerprint     = "bridge.fingerprint"
)

// Parser decodes streaming.LedgerTransactions into ParsedEvents.
//
// Filtering operates at two distinct layers:
//
//  1. gRPC (template-level): the Fetcher subscribes to CIP56.Events.TokenTransferEvent
//     via TemplateID, reducing network traffic to only that contract type. This is done
//     at the Canton Ledger API level and cannot filter by instrument payload.
//     PackageID="" in the TemplateID enables all-packages mode, so any third-party
//     CIP56-compliant token is automatically included at this layer.
//
//  2. App-level (instrument-level): the Parser further filters by InstrumentKey{Admin, ID}.
//     This is necessary because the gRPC API cannot filter by contract field values.
//     InstrumentKey is the Canton equivalent of an ERC-20 contract address — it uniquely
//     identifies a specific token deployment by its issuer party and token identifier.
type Parser struct {
	mode               FilterMode
	allowedInstruments map[InstrumentKey]struct{}
	logger             *zap.Logger
}

// NewParser creates a new Parser.
//
//   - mode:               FilterModeAll or FilterModeWhitelist.
//   - allowedInstruments: InstrumentKeys to accept (Canton equivalent of ERC-20 contract addresses).
//     Each key is {Admin: issuerPartyID, ID: tokenID}. Both fields must match.
//     Ignored when mode is FilterModeAll.
//   - logger:             caller-provided logger.
func NewParser(mode FilterMode, allowedInstruments []InstrumentKey, logger *zap.Logger) *Parser {
	allowed := make(map[InstrumentKey]struct{}, len(allowedInstruments))
	for _, k := range allowedInstruments {
		allowed[k] = struct{}{}
	}
	return &Parser{
		mode:               mode,
		allowedInstruments: allowed,
		logger:             logger,
	}
}

// Parse extracts and decodes all TokenTransferEvent created-events from tx.
// Returns one ParsedEvent per matched event; events that do not match the template,
// fail the instrument filter, or contain an invalid party combination are dropped.
func (p *Parser) Parse(tx *streaming.LedgerTransaction) []*ParsedEvent {
	out := make([]*ParsedEvent, 0, len(tx.Events))

	for _, ev := range tx.Events {
		if !ev.IsCreated {
			continue // archived events carry no field data — nothing to index
		}
		if ev.ModuleName != tokenTransferEventModule || ev.TemplateName != tokenTransferEventEntity {
			continue
		}

		instrumentID := ev.NestedTextField("instrumentId", "id")
		instrumentAdmin := ev.NestedPartyField("instrumentId", "admin")
		key := InstrumentKey{Admin: instrumentAdmin, ID: instrumentID}

		if !p.instrumentAllowed(key) {
			p.logger.Debug("skipping event for unlisted instrument",
				zap.String("instrument_id", instrumentID),
				zap.String("instrument_admin", instrumentAdmin),
				zap.String("contract_id", ev.ContractID),
			)
			continue
		}

		pe := p.decode(tx, ev, instrumentID)
		if pe == nil {
			continue
		}
		out = append(out, pe)
	}

	return out
}

// decode converts a single TokenTransferEvent LedgerEvent into a ParsedEvent.
// Returns nil when the event contains an invalid party combination (both absent).
func (p *Parser) decode(tx *streaming.LedgerTransaction, ev *streaming.LedgerEvent, instrumentID string) *ParsedEvent {
	fromPartyID := optionalParty(ev, "fromParty")
	toPartyID := optionalParty(ev, "toParty")

	var et EventType
	switch {
	case fromPartyID == nil && toPartyID != nil:
		et = EventMint
	case fromPartyID != nil && toPartyID == nil:
		et = EventBurn
	case fromPartyID != nil && toPartyID != nil:
		et = EventTransfer
	default:
		p.logger.Warn("dropping TokenTransferEvent with both parties absent",
			zap.String("contract_id", ev.ContractID),
			zap.String("tx_id", tx.UpdateID),
			zap.String("instrument_id", instrumentID),
		)
		return nil
	}

	return &ParsedEvent{
		InstrumentID:    instrumentID,
		InstrumentAdmin: ev.NestedPartyField("instrumentId", "admin"),
		Issuer:          ev.PartyField("issuer"),
		EventType:       et,
		Amount:          ev.NumericField("amount"),
		FromPartyID:     fromPartyID,
		ToPartyID:       toPartyID,
		ExternalTxID:    optionalMeta(ev, metaKeyExternalTxID),
		ExternalAddress: optionalMeta(ev, metaKeyExternalAddress),
		Fingerprint:     optionalMeta(ev, metaKeyFingerprint),
		ContractID:      ev.ContractID,
		TxID:            tx.UpdateID,
		LedgerOffset:    tx.Offset,
		Timestamp:       ev.TimestampField("timestamp"),
		EffectiveTime:   tx.EffectiveTime,
	}
}

// instrumentAllowed returns true when the InstrumentKey passes the filter.
func (p *Parser) instrumentAllowed(key InstrumentKey) bool {
	if p.mode == FilterModeAll {
		return true
	}
	_, ok := p.allowedInstruments[key]
	return ok
}

// optionalParty extracts a DAML Optional Party field as *string.
// Returns nil when the field is None.
func optionalParty(ev *streaming.LedgerEvent, name string) *string {
	if ev.IsNone(name) {
		return nil
	}
	v := ev.OptionalPartyField(name)
	if v == "" {
		return nil
	}
	return &v
}

// optionalMeta looks up a bridge metadata key and returns a *string.
// Returns nil when meta is None or the key is absent.
func optionalMeta(ev *streaming.LedgerEvent, key string) *string {
	v := ev.OptionalMetaLookup("meta", key)
	if v == "" {
		return nil
	}
	return &v
}
