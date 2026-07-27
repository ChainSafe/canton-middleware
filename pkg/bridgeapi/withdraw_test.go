// SPDX-License-Identifier: Apache-2.0

package bridgeapi

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	cantontkn "github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/pkg/relayer"
	"github.com/chainsafe/canton-middleware/pkg/user"
)

const (
	testExternalAddr  = "0x00000000000000000000000000000000000000ee"
	testCustodialAddr = "0x00000000000000000000000000000000000000cc"
	testEVMRecipient  = "0x1111111111111111111111111111111111111111"
	testFingerprint   = "1220fingerprint"
)

func withdrawUsers() map[string]*user.User {
	return map[string]*user.User{
		testExternalAddr: {
			EVMAddress:                 testExternalAddr,
			CantonParty:                "party::external",
			KeyMode:                    user.KeyModeExternal,
			CantonPublicKeyFingerprint: testFingerprint,
		},
		testCustodialAddr: {
			EVMAddress:  testCustodialAddr,
			CantonParty: "party::custodial",
			KeyMode:     user.KeyModeCustodial,
		},
	}
}

func preparedBurnTransfer() *cantontkn.PreparedTransfer {
	return &cantontkn.PreparedTransfer{
		TransferID:      "burn-1",
		TransactionHash: []byte{0xde, 0xad},
		PartyID:         "party::external",
		ExpiresAt:       time.Now().Add(time.Minute),
	}
}

func withdrawReq() *WithdrawRequest {
	return &WithdrawRequest{Token: "USDCX", Amount: testAmount, Recipient: testEVMRecipient}
}

func TestWithdrawPrepare_ExternalKey(t *testing.T) {
	burner := &fakeBurner{prepared: preparedBurnTransfer()}
	svc := newTestServiceWithUsers(t, &fakeRelayer{}, burner, withdrawUsers())

	resp, err := svc.WithdrawPrepare(context.Background(), testExternalAddr, withdrawReq())
	if err != nil {
		t.Fatalf("WithdrawPrepare failed: %v", err)
	}
	if resp.TransferID != "burn-1" || resp.TransactionHash != "0x"+hex.EncodeToString([]byte{0xde, 0xad}) {
		t.Fatalf("resp = %+v", resp)
	}

	req := burner.burnReq
	if req == nil {
		t.Fatalf("PrepareBurn never called")
	}
	if req.PartyID != "party::external" || req.TokenSymbol != "USDCx" ||
		req.InstrumentAdmin != "circle::admin" || req.EvmRecipient != testEVMRecipient ||
		req.Amount != testAmount || req.RequestID == "" {
		t.Fatalf("burn request = %+v", req)
	}
}

func TestWithdrawPrepare_CustodialUserRejected(t *testing.T) {
	svc := newTestServiceWithUsers(t, &fakeRelayer{}, &fakeBurner{}, withdrawUsers())

	if _, err := svc.WithdrawPrepare(context.Background(), testCustodialAddr, withdrawReq()); err == nil {
		t.Fatalf("custodial user on prepare endpoint should fail")
	}
}

func TestWithdrawPrepare_Validation(t *testing.T) {
	svc := newTestServiceWithUsers(t, &fakeRelayer{}, &fakeBurner{prepared: preparedBurnTransfer()}, withdrawUsers())
	ctx := context.Background()

	cases := []struct {
		name   string
		mutate func(*WithdrawRequest)
	}{
		{"unsupported token", func(r *WithdrawRequest) { r.Token = "DOGE" }},
		{"bad amount", func(r *WithdrawRequest) { r.Amount = "-3" }},
		{"bad recipient", func(r *WithdrawRequest) { r.Recipient = "not-an-address" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := withdrawReq()
			tc.mutate(req)
			if _, err := svc.WithdrawPrepare(ctx, testExternalAddr, req); err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestWithdrawExecute_SubmitsAndRegisters(t *testing.T) {
	burner := &fakeBurner{prepared: preparedBurnTransfer()}
	rc := &fakeRelayer{}
	svc := newTestServiceWithUsers(t, rc, burner, withdrawUsers())
	ctx := context.Background()

	prep, err := svc.WithdrawPrepare(ctx, testExternalAddr, withdrawReq())
	if err != nil {
		t.Fatalf("WithdrawPrepare failed: %v", err)
	}

	resp, err := svc.WithdrawExecute(ctx, testExternalAddr, &WithdrawExecuteRequest{
		TransferID: prep.TransferID,
		Signature:  "0xabcdef",
		SignedBy:   testFingerprint,
	})
	if err != nil {
		t.Fatalf("WithdrawExecute failed: %v", err)
	}
	if !resp.Created {
		t.Fatalf("resp = %+v", resp)
	}
	if burner.executed == nil || burner.executed.SignedBy != testFingerprint {
		t.Fatalf("ExecuteTransfer not called correctly: %+v", burner.executed)
	}

	reg := rc.registered
	if reg == nil {
		t.Fatalf("relayer never called")
	}
	if reg.Direction != relayer.DirectionCantonToEthereum || reg.BridgeKey != MechanismXReserve {
		t.Fatalf("registered = %+v", reg)
	}
	if reg.Metadata[metaBurnRequestID] == "" || reg.SourceTxHash != reg.Metadata[metaBurnRequestID] {
		t.Fatalf("burn request id not propagated: %+v", reg)
	}
	if reg.Recipient != testEVMRecipient || reg.Sender != "party::external" || reg.Amount != testAmount {
		t.Fatalf("registered = %+v", reg)
	}

	// The prepared burn is single-use.
	if _, err = svc.WithdrawExecute(ctx, testExternalAddr, &WithdrawExecuteRequest{
		TransferID: prep.TransferID, Signature: "0xabcdef", SignedBy: testFingerprint,
	}); err == nil {
		t.Fatalf("replayed execute should fail")
	}
}

func TestWithdrawExecute_Guards(t *testing.T) {
	burner := &fakeBurner{prepared: preparedBurnTransfer()}
	svc := newTestServiceWithUsers(t, &fakeRelayer{}, burner, withdrawUsers())
	ctx := context.Background()

	// Unknown transfer id.
	if _, err := svc.WithdrawExecute(ctx, testExternalAddr, &WithdrawExecuteRequest{
		TransferID: "nope", Signature: "0x1", SignedBy: testFingerprint,
	}); err == nil {
		t.Fatalf("unknown transfer should fail")
	}

	// Foreign owner.
	prep, err := svc.WithdrawPrepare(ctx, testExternalAddr, withdrawReq())
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	if _, err = svc.WithdrawExecute(ctx, testCustodialAddr, &WithdrawExecuteRequest{
		TransferID: prep.TransferID, Signature: "0x1", SignedBy: testFingerprint,
	}); err == nil {
		t.Fatalf("foreign owner should fail")
	}

	// Fingerprint mismatch.
	prep, err = svc.WithdrawPrepare(ctx, testExternalAddr, withdrawReq())
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	if _, err = svc.WithdrawExecute(ctx, testExternalAddr, &WithdrawExecuteRequest{
		TransferID: prep.TransferID, Signature: "0x1", SignedBy: "1220other",
	}); err == nil {
		t.Fatalf("fingerprint mismatch should fail")
	}
}

func TestWithdrawCustodial_BurnsAndRegisters(t *testing.T) {
	burner := &fakeBurner{}
	rc := &fakeRelayer{}
	svc := newTestServiceWithUsers(t, rc, burner, withdrawUsers())

	resp, err := svc.WithdrawCustodial(context.Background(), testCustodialAddr, withdrawReq())
	if err != nil {
		t.Fatalf("WithdrawCustodial failed: %v", err)
	}
	if !resp.Created || burner.customCalls != 1 {
		t.Fatalf("resp = %+v, burn calls = %d", resp, burner.customCalls)
	}
	if burner.burnReq.PartyID != "party::custodial" {
		t.Fatalf("burn request = %+v", burner.burnReq)
	}
	if rc.registered == nil || rc.registered.Sender != "party::custodial" {
		t.Fatalf("registered = %+v", rc.registered)
	}
}

func TestWithdrawCustodial_ExternalUserRejected(t *testing.T) {
	svc := newTestServiceWithUsers(t, &fakeRelayer{}, &fakeBurner{}, withdrawUsers())

	if _, err := svc.WithdrawCustodial(context.Background(), testExternalAddr, withdrawReq()); err == nil {
		t.Fatalf("external-key user on custodial endpoint should fail")
	}
}
