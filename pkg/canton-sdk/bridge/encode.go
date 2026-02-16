package bridge

import (
	"time"

	"github.com/chainsafe/canton-middleware/pkg/canton-sdk/values"
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
)

func encodeCreatePendingDepositArgs(req CreatePendingDepositRequest) *lapiv2.Record {
	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "fingerprint", Value: values.TextValue(req.Fingerprint)},
			{Label: "amount", Value: values.NumericValue(req.Amount)},
			{Label: "evmTxHash", Value: values.TextValue(req.EvmTxHash)},
			{Label: "eventTime", Value: values.TimestampValue(time.Now())},
		},
	}
}

func encodeProcessDepositAndMintArgs(req ProcessDepositRequest) *lapiv2.Record {
	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "depositCid", Value: values.ContractIDValue(req.DepositCID)},
			{Label: "mappingCid", Value: values.ContractIDValue(req.MappingCID)},
			{Label: "eventTime", Value: values.TimestampValue(time.Now())},
		},
	}
}

func encodeInitiateWithdrawalArgs(req InitiateWithdrawalRequest) *lapiv2.Record {
	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "mappingCid", Value: values.ContractIDValue(req.MappingCID)},
			{Label: "holdingCid", Value: values.ContractIDValue(req.HoldingCID)},
			{Label: "amount", Value: values.NumericValue(req.Amount)},
			{Label: "evmDestination", Value: values.TextValue(req.EvmDestination)},
			{Label: "eventTime", Value: values.TimestampValue(time.Now())},
		},
	}
}

func encodeCompleteWithdrawalArgs(evmTxHash string) *lapiv2.Record {
	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "evmTxHash", Value: values.TextValue(evmTxHash)},
			{Label: "eventTime", Value: values.TimestampValue(time.Now())},
		},
	}
}
