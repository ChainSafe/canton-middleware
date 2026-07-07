// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/streaming"
	"github.com/chainsafe/canton-middleware/pkg/indexer"

	"go.uber.org/zap"
)

const (
	tokenTransferEventModule = "CIP56.Events"
	tokenTransferEventEntity = "TokenTransferEvent"

	// Metadata keys for bridge context stored in TokenTransferEvent.meta.values.
	metaKeyExternalTxID    = "bridge.externalTxId"
	metaKeyExternalAddress = "bridge.externalAddress"
	metaKeyFingerprint     = "bridge.fingerprint"

	transferOfferModule = "Utility.Registry.App.V0.Model.Transfer"
	transferOfferEntity = "TransferOffer"

	// Splice token-standard TransferInstruction choices that archive a
	// TransferOffer. The choice name arrives on the consuming exercised event
	// (LEDGER_EFFECTS stream shape) and decides the offer's terminal status:
	// accept settles it ("completed"), withdraw is the sender claiming it back
	// ("canceled"), reject is the receiver declining it ("rejected").
	choiceInstructionAccept   = "TransferInstruction_Accept"
	choiceInstructionWithdraw = "TransferInstruction_Withdraw"
	choiceInstructionReject   = "TransferInstruction_Reject"

	holdingModule = "Utility.Registry.Holding.V0.Holding"
	holdingEntity = "Holding"
)

// NewTokenTransferDecoder returns a decode function for use with streaming.NewStream.
//
// The closure:
//   - skips archived events
//   - checks ModuleName == "CIP56.Events" && TemplateName == "TokenTransferEvent"
//   - applies the FilterModeWhitelist instrument check when mode is FilterModeWhitelist
//   - extracts all fields into a *ParsedEvent
//   - returns nil, false for invalid events (both parties absent, filter miss)
func NewTokenTransferDecoder(
	mode indexer.FilterMode,
	allowed []indexer.InstrumentKey,
	logger *zap.Logger,
) func(*streaming.LedgerTransaction, *streaming.LedgerEvent) (*indexer.ParsedEvent, bool) {
	allowedMap := make(map[indexer.InstrumentKey]struct{}, len(allowed))
	for _, k := range allowed {
		allowedMap[k] = struct{}{}
	}

	return func(tx *streaming.LedgerTransaction, ev *streaming.LedgerEvent) (*indexer.ParsedEvent, bool) {
		if !ev.IsCreated {
			return nil, false
		}
		if ev.ModuleName != tokenTransferEventModule || ev.TemplateName != tokenTransferEventEntity {
			return nil, false
		}

		instrumentID := ev.NestedTextField("instrumentId", "id")
		instrumentAdmin := ev.NestedPartyField("instrumentId", "admin")
		key := indexer.InstrumentKey{Admin: instrumentAdmin, ID: instrumentID}

		if mode == indexer.FilterModeWhitelist {
			if _, ok := allowedMap[key]; !ok {
				logger.Debug("skipping event for unlisted instrument",
					zap.String("instrument_id", instrumentID),
					zap.String("instrument_admin", instrumentAdmin),
					zap.String("contract_id", ev.ContractID),
				)
				return nil, false
			}
		}

		var fromPartyID *string
		if !ev.IsNone("fromParty") {
			v := ev.OptionalPartyField("fromParty")
			if v != "" {
				fromPartyID = &v
			}
		}

		var toPartyID *string
		if !ev.IsNone("toParty") {
			v := ev.OptionalPartyField("toParty")
			if v != "" {
				toPartyID = &v
			}
		}

		var externalTxID *string
		if v := ev.OptionalMetaLookup("meta", metaKeyExternalTxID); v != "" {
			externalTxID = &v
		}

		var externalAddress *string
		if v := ev.OptionalMetaLookup("meta", metaKeyExternalAddress); v != "" {
			externalAddress = &v
		}

		var fingerprint *string
		if v := ev.OptionalMetaLookup("meta", metaKeyFingerprint); v != "" {
			fingerprint = &v
		}

		var et indexer.EventType
		switch {
		case fromPartyID == nil && toPartyID != nil:
			et = indexer.EventMint
		case fromPartyID != nil && toPartyID == nil:
			et = indexer.EventBurn
		case fromPartyID != nil && toPartyID != nil:
			et = indexer.EventTransfer
		default:
			logger.Warn("dropping TokenTransferEvent with both parties absent",
				zap.String("contract_id", ev.ContractID),
				zap.String("tx_id", tx.UpdateID),
				zap.String("instrument_id", instrumentID),
			)
			return nil, false
		}

		return &indexer.ParsedEvent{
			InstrumentID:    instrumentID,
			InstrumentAdmin: instrumentAdmin,
			Issuer:          ev.PartyField("issuer"),
			EventType:       et,
			Amount:          ev.NumericField("amount"),
			FromPartyID:     fromPartyID,
			ToPartyID:       toPartyID,
			ExternalTxID:    externalTxID,
			ExternalAddress: externalAddress,
			Fingerprint:     fingerprint,
			ContractID:      ev.ContractID,
			TxID:            tx.UpdateID,
			LedgerOffset:    tx.Offset,
			Timestamp:       ev.TimestampField("timestamp"),
			EffectiveTime:   tx.EffectiveTime,
		}, true
	}
}

// NewOfferDecoder returns a decode function for TransferOffer CREATED and ARCHIVED
// events, producing an *indexer.Transfer of Kind "offer". Returns nil, false when
// packageID is empty (feature disabled).
//
// On CREATED the transfer is "pending" with all fields populated; on ARCHIVED only
// ContractID/LedgerOffset/CreatedAt, the Archived flag, and the terminal Status
// (derived from the archiving choice) are set — the processor uses ContractID to
// finalize the existing row.
func NewOfferDecoder(
	packageID string, metrics *Metrics, logger *zap.Logger,
) func(*streaming.LedgerTransaction, *streaming.LedgerEvent) (*indexer.Transfer, bool) {
	if packageID == "" {
		return func(*streaming.LedgerTransaction, *streaming.LedgerEvent) (*indexer.Transfer, bool) {
			return nil, false
		}
	}
	return func(tx *streaming.LedgerTransaction, ev *streaming.LedgerEvent) (*indexer.Transfer, bool) {
		// Match by module+entity only. The stream-level filter (buildTemplateFilters)
		// already narrowed the wire to this template; comparing ev.PackageID to a
		// config-supplied value breaks when the config uses a package-name reference
		// (#name) — Canton accepts those in filters but events arrive carrying the
		// resolved package hash, so equality fails. Mirrors the CIP56 decoder.
		if ev.ModuleName != transferOfferModule || ev.TemplateName != transferOfferEntity {
			return nil, false
		}
		transfer := &indexer.Transfer{
			ContractID:   ev.ContractID,
			Kind:         indexer.TransferKindOffer,
			Archived:     !ev.IsCreated,
			LedgerOffset: tx.Offset,
			CreatedAt:    tx.EffectiveTime,
		}
		if !ev.IsCreated {
			transfer.Status = offerTerminalStatus(ev.Choice, ev.ContractID, metrics, logger)
		}
		if ev.IsCreated {
			// TransferOffer CreateArguments: {operator, provider, transfer{...}}.
			// Receiver/sender/amount/instrumentId all live inside the nested transfer record.
			transfer.Status = indexer.TransferStatusPending
			transfer.ToPartyID = ev.NestedPartyField("transfer", "receiver")
			transfer.FromPartyID = ev.NestedPartyField("transfer", "sender")
			transfer.Amount = ev.NestedNumericField("transfer", "amount")
			transfer.InstrumentAdmin = ev.DoublyNestedPartyField("transfer", "instrumentId", "admin")
			transfer.InstrumentID = ev.DoublyNestedTextField("transfer", "instrumentId", "id")
			if exp := ev.NestedTimestampField("transfer", "executeBefore"); !exp.IsZero() {
				transfer.ExpiresAt = &exp
			}
			if transfer.ToPartyID == "" {
				logger.Warn("TransferOffer CREATED decoded with empty receiver — field name mismatch?",
					zap.String("contract_id", ev.ContractID),
					zap.Int64("offset", tx.Offset),
				)
			}
		}
		return transfer, true
	}
}

// offerTerminalStatus maps the choice that archived a TransferOffer to the
// transfer's terminal status: accept settles ("completed"), withdraw is the
// sender claiming the offer back ("canceled"), reject is the receiver declining
// it ("rejected"). Unknown choices — including "" from a stream that does not
// carry choice names — fall back to "completed", preserving the historical
// any-archive-settles behavior. Each fallback increments the
// OfferUnknownArchiveChoices counter (alertable) and logs a warning, since it
// may mean a registry archives offers via a choice this mapping doesn't know
// (e.g. TransferInstruction_Update) and statuses could be mislabeled.
func offerTerminalStatus(choice, contractID string, metrics *Metrics, logger *zap.Logger) string {
	switch choice {
	case choiceInstructionAccept:
		return indexer.TransferStatusCompleted
	case choiceInstructionWithdraw:
		return indexer.TransferStatusCanceled
	case choiceInstructionReject:
		return indexer.TransferStatusRejected
	default:
		metrics.OfferUnknownArchiveChoices.Inc()
		logger.Warn("TransferOffer archived by unrecognized choice — defaulting to completed",
			zap.String("choice", choice),
			zap.String("contract_id", contractID),
		)
		return indexer.TransferStatusCompleted
	}
}

// NewHoldingDecoder returns a decode function for Utility.Registry.Holding.V0.Holding
// CREATED and ARCHIVED events. Returns nil, false when packageID is empty (feature
// disabled). Used so the indexer can maintain indexer_balances for Utility.Registry
// instruments (e.g. USDCx) which do not emit a separate TokenTransferEvent contract.
//
// The Holding template's create_arguments are {operator, provider, registrar, owner,
// instrument{source,id,scheme}, label, amount, lock}. The Splice HoldingV1 view derives
// instrumentId.admin from `registrar` and instrumentId.id from `instrument.id` — the
// decoder mirrors that mapping so balances keyed by (admin, id) line up with the
// per-instrument balance table.
func NewHoldingDecoder(
	packageID string, logger *zap.Logger,
) func(*streaming.LedgerTransaction, *streaming.LedgerEvent) (*indexer.HoldingChange, bool) {
	if packageID == "" {
		return func(*streaming.LedgerTransaction, *streaming.LedgerEvent) (*indexer.HoldingChange, bool) {
			return nil, false
		}
	}
	return func(tx *streaming.LedgerTransaction, ev *streaming.LedgerEvent) (*indexer.HoldingChange, bool) {
		// Match by module+entity only (see NewOfferDecoder comment).
		if ev.ModuleName != holdingModule || ev.TemplateName != holdingEntity {
			return nil, false
		}
		change := &indexer.HoldingChange{
			ContractID:   ev.ContractID,
			IsArchived:   !ev.IsCreated,
			LedgerOffset: tx.Offset,
		}
		if ev.IsCreated {
			change.Owner = ev.PartyField("owner")
			change.InstrumentAdmin = ev.PartyField("registrar")
			change.InstrumentID = ev.NestedTextField("instrument", "id")
			change.Amount = ev.NumericField("amount")
			// `lock` is Optional Lock: Some => the holding is escrowed by an
			// outstanding offer (not spendable), None => freely spendable.
			change.Locked = !ev.IsNone("lock")
			if change.Owner == "" || change.InstrumentID == "" {
				logger.Warn("Holding CREATED decoded with empty owner or instrument — field-name mismatch?",
					zap.String("contract_id", ev.ContractID),
					zap.Int64("offset", tx.Offset),
				)
			}
		}
		return change, true
	}
}

// NewMultiDecoder wraps the TokenTransfer, Offer, and Holding decoders into a single
// any-typed decode function. The Holding decoder is only consulted when both prior
// decoders miss — TokenTransferEvents and TransferOffers never collide with Holding
// templates, so the order is purely a fast-path optimization for the common case.
func NewMultiDecoder(
	transferDecode func(*streaming.LedgerTransaction, *streaming.LedgerEvent) (*indexer.ParsedEvent, bool),
	offerDecode func(*streaming.LedgerTransaction, *streaming.LedgerEvent) (*indexer.Transfer, bool),
	holdingDecode func(*streaming.LedgerTransaction, *streaming.LedgerEvent) (*indexer.HoldingChange, bool),
) func(*streaming.LedgerTransaction, *streaming.LedgerEvent) (any, bool) {
	return func(tx *streaming.LedgerTransaction, ev *streaming.LedgerEvent) (any, bool) {
		if item, ok := transferDecode(tx, ev); ok {
			return item, true
		}
		if item, ok := offerDecode(tx, ev); ok {
			return item, true
		}
		if item, ok := holdingDecode(tx, ev); ok {
			return item, true
		}
		return nil, false
	}
}
