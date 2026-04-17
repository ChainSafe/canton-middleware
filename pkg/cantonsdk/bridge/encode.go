package bridge

import (
	"time"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"
)

func encodeCreatePendingDepositArgs(req CreatePendingDepositRequest) *lapiv2.Record {
	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "fingerprint", Value: values.TextValue(req.Fingerprint)},
			{Label: "amount", Value: values.NumericValue(req.Amount)},
			{Label: "evmTxHash", Value: values.TextValue(req.EvmTxHash)},
			{Label: "timestamp", Value: values.TimestampValue(time.Now())},
		},
	}
}

func encodeProcessDepositAndMintArgs(req ProcessDepositRequest) *lapiv2.Record {
	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "depositCid", Value: values.ContractIDValue(req.DepositCID)},
			{Label: "mappingCid", Value: values.ContractIDValue(req.MappingCID)},
			{Label: "timestamp", Value: values.TimestampValue(time.Now())},
		},
	}
}

func encodeInitiateWithdrawalArgs(req InitiateWithdrawalRequest) *lapiv2.Record {
	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "mappingCid", Value: values.ContractIDValue(req.MappingCID)},
			{Label: "holdingCid", Value: values.ContractIDValue(req.HoldingCID)},
			{Label: "amount", Value: values.NumericValue(req.Amount)},
			{Label: "evmDestination", Value: values.NewtypeValue(values.TextValue(req.EvmDestination))},
		},
	}
}

// encodeProcessWithdrawalArgs encodes the argument record for the
// Bridge.Contracts.WithdrawalRequest:ProcessWithdrawal choice.
// DAML signature: ProcessWithdrawal : Time -> ContractId WithdrawalEvent
// The choice takes a single `timestamp : Time` field.
func encodeProcessWithdrawalArgs() *lapiv2.Record {
	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "timestamp", Value: values.TimestampValue(time.Now())},
		},
	}
}

func encodeCompleteWithdrawalArgs(evmTxHash string) *lapiv2.Record {
	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "evmTxHash", Value: values.TextValue(evmTxHash)},
			{Label: "timestamp", Value: values.TimestampValue(time.Now())},
		},
	}
}
