package bridge

import (
	"fmt"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"
)

func decodeWithdrawalEvent(ce *lapiv2.CreatedEvent, txID string) *WithdrawalEvent {
	fields := values.RecordToMap(ce.CreateArguments)

	return &WithdrawalEvent{
		ContractID:     ce.ContractId,
		EventID:        fmt.Sprintf("%d-%d", ce.Offset, ce.NodeId),
		TransactionID:  txID,
		Issuer:         values.Party(fields["issuer"]),
		UserParty:      values.Party(fields["userParty"]),
		EvmDestination: values.Text(fields["evmDestination"]),
		Amount:         values.Numeric(fields["amount"]),
		Fingerprint:    values.Text(fields["fingerprint"]),
		Status:         decodeWithdrawalStatusV2(fields["status"]),
	}
}

func decodeWithdrawalStatusV2(v *lapiv2.Value) WithdrawalStatus {
	if v == nil {
		return WithdrawalStatusPending
	}
	// Status is a variant/enum
	if variant, ok := v.Sum.(*lapiv2.Value_Variant); ok {
		switch variant.Variant.Constructor {
		case "Pending":
			return WithdrawalStatusPending
		case "Completed":
			return WithdrawalStatusCompleted
		case "Failed":
			return WithdrawalStatusFailed
		}
	}
	return WithdrawalStatusPending
}
