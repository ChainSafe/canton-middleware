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

// NewOfferDecoder returns a decode function for TransferOffer CREATED and ARCHIVED events.
// Returns nil, false when packageID is empty (feature disabled).
func NewOfferDecoder(packageID string) func(*streaming.LedgerTransaction, *streaming.LedgerEvent) (*indexer.PendingOffer, bool) {
	if packageID == "" {
		return func(*streaming.LedgerTransaction, *streaming.LedgerEvent) (*indexer.PendingOffer, bool) {
			return nil, false
		}
	}
	return func(tx *streaming.LedgerTransaction, ev *streaming.LedgerEvent) (*indexer.PendingOffer, bool) {
		if ev.PackageID != packageID || ev.ModuleName != transferOfferModule || ev.TemplateName != transferOfferEntity {
			return nil, false
		}
		offer := &indexer.PendingOffer{
			ContractID:   ev.ContractID,
			IsArchived:   !ev.IsCreated,
			LedgerOffset: tx.Offset,
			CreatedAt:    tx.EffectiveTime,
		}
		if ev.IsCreated {
			// TransferOffer has top-level sender/receiver/amount fields and an
			// instrumentId: InstrumentId{ admin, id } record. Field names may
			// need adjustment once tested against the real devnet template.
			offer.ReceiverPartyID = ev.PartyField("receiver")
			offer.SenderPartyID = ev.PartyField("sender")
			offer.Amount = ev.NumericField("amount")
			offer.InstrumentAdmin = ev.NestedPartyField("instrumentId", "admin")
			offer.InstrumentID = ev.NestedTextField("instrumentId", "id")
		}
		return offer, true
	}
}

// NewMultiDecoder wraps a TokenTransfer decoder and an Offer decoder into a single any-typed decode function.
func NewMultiDecoder(
	transferDecode func(*streaming.LedgerTransaction, *streaming.LedgerEvent) (*indexer.ParsedEvent, bool),
	offerDecode func(*streaming.LedgerTransaction, *streaming.LedgerEvent) (*indexer.PendingOffer, bool),
) func(*streaming.LedgerTransaction, *streaming.LedgerEvent) (any, bool) {
	return func(tx *streaming.LedgerTransaction, ev *streaming.LedgerEvent) (any, bool) {
		if item, ok := transferDecode(tx, ev); ok {
			return item, true
		}
		if item, ok := offerDecode(tx, ev); ok {
			return item, true
		}
		return nil, false
	}
}
