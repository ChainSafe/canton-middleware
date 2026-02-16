package token

import (
	"time"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton-sdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/canton-sdk/values"
)

func encodeIssuerMintArgs(req MintRequest) *lapiv2.Record {
	evmTx := values.None()
	if req.EvmTxHash != "" {
		evmTx = values.Optional(values.TextValue(req.EvmTxHash))
	}

	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "recipient", Value: values.PartyValue(req.RecipientParty)},
			{Label: "amount", Value: values.NumericValue(req.Amount)},
			{Label: "eventTime", Value: values.TimestampValue(time.Now())},
			{Label: "userFingerprint", Value: values.TextValue(req.UserFingerprint)},
			{Label: "evmTxHash", Value: evmTx},
		},
	}
}

func encodeIssuerBurnArgs(req BurnRequest) *lapiv2.Record {
	evmDest := values.None()
	if req.EvmDestination != "" {
		evmDest = values.Optional(values.TextValue(req.EvmDestination))
	}

	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "holdingCid", Value: values.ContractIDValue(req.HoldingCID)},
			{Label: "amount", Value: values.NumericValue(req.Amount)},
			{Label: "eventTime", Value: values.TimestampValue(time.Now())},
			{Label: "userFingerprint", Value: values.TextValue(req.UserFingerprint)},
			{Label: "evmDestination", Value: evmDest},
		},
	}
}

func encodeHoldingTransferArgs(toParty, amount, existingRecipientHolding string) *lapiv2.Record {
	existing := values.None()
	if existingRecipientHolding != "" {
		existing = values.Optional(values.ContractIDValue(existingRecipientHolding))
	}

	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "to", Value: values.PartyValue(toParty)},
			{Label: "value", Value: values.NumericValue(amount)},
			{Label: "existingRecipientHolding", Value: existing},
			{Label: "complianceRulesCid", Value: values.None()},
			{Label: "complianceProofCid", Value: values.None()},
		},
	}
}
