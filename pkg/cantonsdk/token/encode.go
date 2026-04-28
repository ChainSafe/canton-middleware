package token

import (
	"time"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"
)

func encodeIssuerMintArgs(req *MintRequest) *lapiv2.Record {
	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "recipient", Value: values.PartyValue(req.RecipientParty)},
			{Label: "amount", Value: values.NumericValue(req.Amount)},
			{Label: "eventTime", Value: values.TimestampValue(time.Now())},
			{Label: "eventMeta", Value: values.EncodeOptionalMetadata(req.EventMeta)},
		},
	}
}

func encodeIssuerBurnArgs(req *BurnRequest) *lapiv2.Record {
	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "holdingCid", Value: values.ContractIDValue(req.HoldingCID)},
			{Label: "amount", Value: values.NumericValue(req.Amount)},
			{Label: "eventTime", Value: values.TimestampValue(time.Now())},
			{Label: "eventMeta", Value: values.EncodeOptionalMetadata(req.EventMeta)},
		},
	}
}

// encodeTransferFactoryTransferArgs encodes the Splice TransferFactory_Transfer choice arguments.
// The choice is exercised on the TransferFactory interface of a CIP56TransferFactory contract.
func encodeTransferFactoryTransferArgs(
	expectedAdmin string,
	sender string,
	receiver string,
	amount string,
	instrumentAdmin string,
	instrumentID string,
	requestedAt time.Time,
	executeBefore time.Time,
	inputHoldingCIDs []string,
	choiceContext map[string]string,
) *lapiv2.Record {
	holdingCidValues := make([]*lapiv2.Value, len(inputHoldingCIDs))
	for i, cid := range inputHoldingCIDs {
		holdingCidValues[i] = values.ContractIDValue(cid)
	}

	transfer := &lapiv2.Value{
		Sum: &lapiv2.Value_Record{
			Record: &lapiv2.Record{
				Fields: []*lapiv2.RecordField{
					{Label: "sender", Value: values.PartyValue(sender)},
					{Label: "receiver", Value: values.PartyValue(receiver)},
					{Label: "amount", Value: values.NumericValue(amount)},
					{Label: "instrumentId", Value: values.EncodeInstrumentId(instrumentAdmin, instrumentID)},
					{Label: "requestedAt", Value: values.TimestampValue(requestedAt)},
					{Label: "executeBefore", Value: values.TimestampValue(executeBefore)},
					{Label: "inputHoldingCids", Value: values.ListValue(holdingCidValues)},
					{Label: "meta", Value: values.EmptyMetadata()},
				},
			},
		},
	}

	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "expectedAdmin", Value: values.PartyValue(expectedAdmin)},
			{Label: "transfer", Value: transfer},
			{Label: "extraArgs", Value: values.EncodeExtraArgs(choiceContext)},
		},
	}
}
