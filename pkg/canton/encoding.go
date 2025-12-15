package canton

import (
	"fmt"
	"math/big"
	"time"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
	"github.com/shopspring/decimal"
)

// =============================================================================
// ISSUER-CENTRIC MODEL ENCODING
// =============================================================================

// EncodeRegisterUserArgs encodes arguments for WayfinderBridgeConfig.RegisterUser
func EncodeRegisterUserArgs(req *RegisterUserRequest) *lapiv2.Record {
	fields := []*lapiv2.RecordField{
		{Label: "userParty", Value: PartyValue(req.UserParty)},
		{Label: "fingerprint", Value: TextValue(req.Fingerprint)},
	}

	// Optional EVM address
	if req.EvmAddress != "" {
		fields = append(fields, &lapiv2.RecordField{
			Label: "evmAddress",
			Value: OptionalValue(RecordValue("EvmAddress", TextValue(req.EvmAddress))),
		})
	} else {
		fields = append(fields, &lapiv2.RecordField{
			Label: "evmAddress",
			Value: NoneValue(),
		})
	}

	return &lapiv2.Record{Fields: fields}
}

// EncodeCreatePendingDepositArgs encodes arguments for WayfinderBridgeConfig.CreatePendingDeposit
func EncodeCreatePendingDepositArgs(req *CreatePendingDepositRequest) *lapiv2.Record {
	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "fingerprint", Value: TextValue(req.Fingerprint)},
			{Label: "amount", Value: NumericValue(req.Amount)},
			{Label: "evmTxHash", Value: TextValue(req.EvmTxHash)},
			{Label: "timestamp", Value: TimestampValue(req.Timestamp)},
		},
	}
}

// EncodeProcessDepositAndMintArgs encodes arguments for WayfinderBridgeConfig.ProcessDepositAndMint
func EncodeProcessDepositAndMintArgs(req *ProcessDepositRequest) *lapiv2.Record {
	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "depositCid", Value: ContractIdValue(req.DepositCid)},
			{Label: "mappingCid", Value: ContractIdValue(req.MappingCid)},
		},
	}
}

// EncodeInitiateWithdrawalArgs encodes arguments for WayfinderBridgeConfig.InitiateWithdrawal
func EncodeInitiateWithdrawalArgs(req *InitiateWithdrawalRequest) *lapiv2.Record {
	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "mappingCid", Value: ContractIdValue(req.MappingCid)},
			{Label: "holdingCid", Value: ContractIdValue(req.HoldingCid)},
			{Label: "amount", Value: NumericValue(req.Amount)},
			{Label: "evmDestination", Value: RecordValue("EvmAddress", TextValue(req.EvmDestination))},
		},
	}
}

// EncodeCompleteWithdrawalArgs encodes arguments for WithdrawalEvent.CompleteWithdrawal
func EncodeCompleteWithdrawalArgs(evmTxHash string) *lapiv2.Record {
	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "evmTxHash", Value: TextValue(evmTxHash)},
		},
	}
}

// =============================================================================
// ISSUER-CENTRIC MODEL DECODING
// =============================================================================

// DecodeFingerprintMapping decodes a FingerprintMapping contract
func DecodeFingerprintMapping(contractID string, record *lapiv2.Record) (*FingerprintMapping, error) {
	fields := recordToMap(record)

	issuer, err := extractParty(fields["issuer"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract issuer: %w", err)
	}

	userParty, err := extractParty(fields["userParty"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract userParty: %w", err)
	}

	fingerprint, err := extractText(fields["fingerprint"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract fingerprint: %w", err)
	}

	evmAddress, _ := extractOptionalEvmAddress(fields["evmAddress"])

	return &FingerprintMapping{
		ContractID:  contractID,
		Issuer:      issuer,
		UserParty:   userParty,
		Fingerprint: fingerprint,
		EvmAddress:  evmAddress,
	}, nil
}

// DecodePendingDeposit decodes a PendingDeposit contract
func DecodePendingDeposit(contractID string, record *lapiv2.Record) (*PendingDeposit, error) {
	fields := recordToMap(record)

	issuer, err := extractParty(fields["issuer"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract issuer: %w", err)
	}

	fingerprint, err := extractText(fields["fingerprint"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract fingerprint: %w", err)
	}

	amount, err := extractNumeric(fields["amount"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract amount: %w", err)
	}

	evmTxHash, err := extractText(fields["evmTxHash"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract evmTxHash: %w", err)
	}

	tokenID, err := extractText(fields["tokenId"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract tokenId: %w", err)
	}

	return &PendingDeposit{
		ContractID:  contractID,
		Issuer:      issuer,
		Fingerprint: fingerprint,
		Amount:      amount,
		EvmTxHash:   evmTxHash,
		TokenID:     tokenID,
	}, nil
}

// DecodeWithdrawalEvent decodes a WithdrawalEvent contract (V1 record)
func DecodeWithdrawalEvent(eventID, txID, contractID string, record *lapiv2.Record) (*WithdrawalEvent, error) {
	fields := recordToMap(record)

	issuer, err := extractParty(fields["issuer"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract issuer: %w", err)
	}

	userParty, err := extractParty(fields["userParty"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract userParty: %w", err)
	}

	evmDestination, err := extractEvmAddress(fields["evmDestination"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract evmDestination: %w", err)
	}

	amount, err := extractNumeric(fields["amount"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract amount: %w", err)
	}

	fingerprint, err := extractText(fields["fingerprint"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract fingerprint: %w", err)
	}

	status := extractWithdrawalStatus(fields["status"])

	return &WithdrawalEvent{
		ContractID:     contractID,
		EventID:        eventID,
		TransactionID:  txID,
		Issuer:         issuer,
		UserParty:      userParty,
		EvmDestination: evmDestination,
		Amount:         amount,
		Fingerprint:    fingerprint,
		Status:         status,
	}, nil
}

// DecodeWithdrawalEventV2 decodes a WithdrawalEvent contract from V2 API record
func DecodeWithdrawalEventV2(eventID, txID, contractID string, record *lapiv2.Record) (*WithdrawalEvent, error) {
	fields := recordToMapV2(record)

	issuer, err := extractPartyV2(fields["issuer"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract issuer: %w", err)
	}

	userParty, err := extractPartyV2(fields["userParty"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract userParty: %w", err)
	}

	evmDestination, err := extractEvmAddressV2(fields["evmDestination"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract evmDestination: %w", err)
	}

	amount, err := extractNumericV2(fields["amount"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract amount: %w", err)
	}

	fingerprint, err := extractTextV2(fields["fingerprint"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract fingerprint: %w", err)
	}

	status := extractWithdrawalStatusV2(fields["status"])

	return &WithdrawalEvent{
		ContractID:     contractID,
		EventID:        eventID,
		TransactionID:  txID,
		Issuer:         issuer,
		UserParty:      userParty,
		EvmDestination: evmDestination,
		Amount:         amount,
		Fingerprint:    fingerprint,
		Status:         status,
	}, nil
}

// =============================================================================
// HELPER ENCODING FUNCTIONS
// =============================================================================

func TextValue(s string) *lapiv2.Value {
	return &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: s}}
}

func PartyValue(s string) *lapiv2.Value {
	return &lapiv2.Value{Sum: &lapiv2.Value_Party{Party: s}}
}

func Int64Value(n int64) *lapiv2.Value {
	return &lapiv2.Value{Sum: &lapiv2.Value_Int64{Int64: n}}
}

func NumericValue(s string) *lapiv2.Value {
	return &lapiv2.Value{Sum: &lapiv2.Value_Numeric{Numeric: s}}
}

func ContractIdValue(cid string) *lapiv2.Value {
	return &lapiv2.Value{Sum: &lapiv2.Value_ContractId{ContractId: cid}}
}

func TimestampValue(t time.Time) *lapiv2.Value {
	// DAML timestamps are microseconds since Unix epoch
	return &lapiv2.Value{Sum: &lapiv2.Value_Timestamp{Timestamp: t.UnixMicro()}}
}

func RecordValue(typeName string, fields ...*lapiv2.Value) *lapiv2.Value {
	recordFields := make([]*lapiv2.RecordField, len(fields))
	for i, f := range fields {
		recordFields[i] = &lapiv2.RecordField{Value: f}
	}
	return &lapiv2.Value{
		Sum: &lapiv2.Value_Record{
			Record: &lapiv2.Record{Fields: recordFields},
		},
	}
}

func OptionalValue(v *lapiv2.Value) *lapiv2.Value {
	return &lapiv2.Value{
		Sum: &lapiv2.Value_Optional{
			Optional: &lapiv2.Optional{Value: v},
		},
	}
}

func NoneValue() *lapiv2.Value {
	return &lapiv2.Value{
		Sum: &lapiv2.Value_Optional{
			Optional: &lapiv2.Optional{Value: nil},
		},
	}
}

// =============================================================================
// HELPER EXTRACTION FUNCTIONS
// =============================================================================

func recordToMap(record *lapiv2.Record) map[string]*lapiv2.Value {
	fields := make(map[string]*lapiv2.Value)
	for _, field := range record.Fields {
		fields[field.Label] = field.Value
	}
	return fields
}

func extractText(v *lapiv2.Value) (string, error) {
	if v == nil {
		return "", fmt.Errorf("nil value")
	}
	if t, ok := v.Sum.(*lapiv2.Value_Text); ok {
		return t.Text, nil
	}
	return "", fmt.Errorf("not a text value")
}

func extractParty(v *lapiv2.Value) (string, error) {
	if v == nil {
		return "", fmt.Errorf("nil value")
	}
	if p, ok := v.Sum.(*lapiv2.Value_Party); ok {
		return p.Party, nil
	}
	return "", fmt.Errorf("not a party value")
}

func extractInt64(v *lapiv2.Value) (int64, error) {
	if v == nil {
		return 0, fmt.Errorf("nil value")
	}
	if i, ok := v.Sum.(*lapiv2.Value_Int64); ok {
		return i.Int64, nil
	}
	return 0, fmt.Errorf("not an int64 value")
}

func extractNumeric(v *lapiv2.Value) (string, error) {
	if v == nil {
		return "", fmt.Errorf("nil value")
	}
	if n, ok := v.Sum.(*lapiv2.Value_Numeric); ok {
		return n.Numeric, nil
	}
	return "", fmt.Errorf("not a numeric value")
}

func extractEvmAddress(v *lapiv2.Value) (string, error) {
	if v == nil {
		return "", fmt.Errorf("nil value")
	}
	// EvmAddress is a record with a single "value" field
	if r, ok := v.Sum.(*lapiv2.Value_Record); ok {
		for _, field := range r.Record.Fields {
			if field.Label == "value" {
				return extractText(field.Value)
			}
		}
	}
	return "", fmt.Errorf("not an EvmAddress record")
}

func extractOptionalEvmAddress(v *lapiv2.Value) (string, error) {
	if v == nil {
		return "", nil
	}
	if opt, ok := v.Sum.(*lapiv2.Value_Optional); ok {
		if opt.Optional.Value == nil {
			return "", nil
		}
		return extractEvmAddress(opt.Optional.Value)
	}
	return "", fmt.Errorf("not an optional value")
}

func extractWithdrawalStatus(v *lapiv2.Value) WithdrawalStatus {
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

// =============================================================================
// CONVERSION FUNCTIONS
// =============================================================================

// BigIntToDecimal converts big.Int to Daml decimal string
func BigIntToDecimal(amount *big.Int, decimals int) string {
	d := decimal.NewFromBigInt(amount, int32(-decimals))
	return d.String()
}

// DecimalToBigInt converts Daml decimal string to big.Int
func DecimalToBigInt(s string, decimals int) (*big.Int, error) {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid decimal format: %w", err)
	}
	d = d.Mul(decimal.New(1, int32(decimals)))
	return d.BigInt(), nil
}

// =============================================================================
// V2 API HELPER EXTRACTION FUNCTIONS
// =============================================================================

func recordToMapV2(record *lapiv2.Record) map[string]*lapiv2.Value {
	fields := make(map[string]*lapiv2.Value)
	for _, field := range record.Fields {
		fields[field.Label] = field.Value
	}
	return fields
}

func extractTextV2(v *lapiv2.Value) (string, error) {
	if v == nil {
		return "", fmt.Errorf("nil value")
	}
	if t, ok := v.Sum.(*lapiv2.Value_Text); ok {
		return t.Text, nil
	}
	return "", fmt.Errorf("not a text value")
}

func extractPartyV2(v *lapiv2.Value) (string, error) {
	if v == nil {
		return "", fmt.Errorf("nil value")
	}
	if p, ok := v.Sum.(*lapiv2.Value_Party); ok {
		return p.Party, nil
	}
	return "", fmt.Errorf("not a party value")
}

func extractNumericV2(v *lapiv2.Value) (string, error) {
	if v == nil {
		return "", fmt.Errorf("nil value")
	}
	if n, ok := v.Sum.(*lapiv2.Value_Numeric); ok {
		return n.Numeric, nil
	}
	return "", fmt.Errorf("not a numeric value")
}

func extractEvmAddressV2(v *lapiv2.Value) (string, error) {
	if v == nil {
		return "", fmt.Errorf("nil value")
	}
	// EvmAddress is a record with a single "value" field
	if r, ok := v.Sum.(*lapiv2.Value_Record); ok {
		for _, field := range r.Record.Fields {
			if field.Label == "value" {
				return extractTextV2(field.Value)
			}
		}
	}
	return "", fmt.Errorf("not an EvmAddress record")
}

func extractWithdrawalStatusV2(v *lapiv2.Value) WithdrawalStatus {
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
