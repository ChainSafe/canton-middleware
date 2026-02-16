package bridge

import (
	"github.com/chainsafe/canton-middleware/pkg/canton-sdk/values"
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
)

func decodeMintEvent(ce *lapiv2.CreatedEvent) *MintEvent {
	fields := values.RecordToMap(ce.CreateArguments)

	return &MintEvent{
		ContractID:     ce.ContractId,
		Issuer:         values.Party(fields["issuer"]),
		Recipient:      values.Party(fields["recipient"]),
		Amount:         values.Numeric(fields["amount"]),
		HoldingCid:     values.ContractID(fields["holdingCid"]),
		TokenSymbol:    values.Text(fields["tokenSymbol"]),
		EvmTxHash:      values.Text(fields["evmTxHash"]),
		Fingerprint:    values.Text(fields["userFingerprint"]),
		Timestamp:      values.Timestamp(fields["timestamp"]),
		AuditObservers: values.PartyList(fields["auditObservers"]),
	}
}

func decodeBurnEvent(ce *lapiv2.CreatedEvent) *BurnEvent {
	fields := values.RecordToMap(ce.CreateArguments)

	return &BurnEvent{
		ContractID:     ce.ContractId,
		Issuer:         values.Party(fields["issuer"]),
		BurnedFrom:     values.Party(fields["burnedFrom"]),
		Amount:         values.Numeric(fields["amount"]),
		EvmDestination: values.Text(fields["evmDestination"]),
		TokenSymbol:    values.Text(fields["tokenSymbol"]),
		Fingerprint:    values.Text(fields["userFingerprint"]),
		Timestamp:      values.Timestamp(fields["timestamp"]),
		AuditObservers: values.PartyList(fields["auditObservers"]),
	}
}
