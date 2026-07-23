// SPDX-License-Identifier: Apache-2.0

package xreserve

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
	"github.com/chainsafe/canton-middleware/pkg/relayer"
)

// fakeAttester returns a fixed attestation result.
type fakeAttester struct {
	att   *Attestation
	err   error
	calls int
}

func (f *fakeAttester) GetAttestation(context.Context, string) (*Attestation, error) {
	f.calls++
	return f.att, f.err
}

// fakeHoldings returns its fixed holdings list filtered by owner.
type fakeHoldings struct {
	holdings []*token.Holding
	err      error
}

func (f *fakeHoldings) GetHoldingsByParty(_ context.Context, ownerParty, _ string) ([]*token.Holding, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]*token.Holding, 0, len(f.holdings))
	for _, h := range f.holdings {
		if h.Owner == ownerParty {
			out = append(out, h)
		}
	}
	return out, nil
}

const (
	testSymbol          = "USDCX"
	testInstrumentAdmin = "circle::admin"
	testInstrumentID    = "USDCx"
	testRecipient       = "party::recipient"
)

func testTokens() map[string]relayer.TokenConfig {
	return map[string]relayer.TokenConfig{
		testSymbol: {
			Mechanism:  Mechanism,
			EVMAddress: "0xusdc",
			Decimals:   6,
			XReserve: &relayer.XReserveConfig{
				AttestationURL:          "http://attestation.local",
				InstrumentAdmin:         testInstrumentAdmin,
				InstrumentID:            testInstrumentID,
				AttestationPollInterval: 30 * time.Second,
				MintPollInterval:        5 * time.Second,
			},
		},
	}
}

func newTestBridge(t *testing.T, circle AttestationClient, holdings HoldingLister) *Bridge {
	t.Helper()
	b, err := New(testTokens(), holdings, zap.NewNop(), WithAttestationClient(circle))
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	return b
}

func holding(amount string) *token.Holding {
	return &token.Holding{
		ContractID:      "cid-" + amount,
		Owner:           testRecipient,
		Amount:          amount,
		InstrumentAdmin: testInstrumentAdmin,
		InstrumentID:    testInstrumentID,
	}
}

func depositTransfer(stage string, metadata map[string]string) *relayer.Transfer {
	return &relayer.Transfer{
		ID:           "0xdeposit",
		BridgeKey:    Mechanism,
		TokenSymbol:  testSymbol,
		Direction:    relayer.DirectionEthereumToCanton,
		Status:       relayer.TransferStatusPending,
		Stage:        stage,
		SourceTxHash: "0xdeposit",
		Amount:       "12.5",
		Recipient:    testRecipient,
		Metadata:     metadata,
	}
}

func TestBridge_KeyAndSources(t *testing.T) {
	b := newTestBridge(t, &fakeAttester{}, &fakeHoldings{})

	if b.Key() != Mechanism {
		t.Fatalf("Key() = %q, want %q", b.Key(), Mechanism)
	}
	sources, err := b.Sources(context.Background())
	if err != nil || len(sources) != 0 {
		t.Fatalf("Sources() = %v, %v; want empty (observer mechanism)", sources, err)
	}
}

func TestBridge_New_RequiresXReserveBlock(t *testing.T) {
	tokens := testTokens()
	tc := tokens[testSymbol]
	tc.XReserve = nil
	tokens[testSymbol] = tc

	if _, err := New(tokens, &fakeHoldings{}, zap.NewNop(), WithAttestationClient(&fakeAttester{})); err == nil {
		t.Fatalf("New without xreserve block should fail")
	}
}

func TestBridge_New_RequiresTokens(t *testing.T) {
	empty := map[string]relayer.TokenConfig{}
	if _, err := New(empty, &fakeHoldings{}, zap.NewNop(), WithAttestationClient(&fakeAttester{})); err == nil {
		t.Fatalf("New without tokens should fail")
	}
}

func TestBridge_Step_FirstStep_SnapshotsBaseline(t *testing.T) {
	holdings := &fakeHoldings{holdings: []*token.Holding{
		holding("100"),
		holding("2.5"),
		// Different admin: same instrument id from another registrar must not count.
		{ContractID: "cid-x", Owner: testRecipient, Amount: "999", InstrumentAdmin: "other::admin", InstrumentID: testInstrumentID},
	}}

	b := newTestBridge(t, &fakeAttester{}, holdings)
	res, err := b.Step(context.Background(), depositTransfer("", nil))
	if err != nil {
		t.Fatalf("Step failed: %v", err)
	}

	if res.Status != relayer.TransferStatusInProgress || res.Stage != StageAwaitingAttestation {
		t.Fatalf("status/stage = %s/%s, want in_progress/%s", res.Status, res.Stage, StageAwaitingAttestation)
	}
	if res.Metadata[metaBaselineBalance] != "102.5" {
		t.Fatalf("baseline = %q, want 102.5", res.Metadata[metaBaselineBalance])
	}
	if res.RetryAfter != 30*time.Second {
		t.Fatalf("RetryAfter = %v, want 30s", res.RetryAfter)
	}
}

func TestBridge_Step_AttestationNotReady_KeepsPolling(t *testing.T) {
	circle := &fakeAttester{err: ErrAttestationNotReady}

	b := newTestBridge(t, circle, &fakeHoldings{})
	res, err := b.Step(context.Background(), depositTransfer(StageAwaitingAttestation, map[string]string{metaBaselineBalance: "0"}))
	if err != nil {
		t.Fatalf("Step failed: %v", err)
	}
	if res.Status != relayer.TransferStatusInProgress || res.Stage != StageAwaitingAttestation {
		t.Fatalf("status/stage = %s/%s, want in_progress/%s", res.Status, res.Stage, StageAwaitingAttestation)
	}
	if circle.calls != 1 {
		t.Fatalf("GetAttestation called %d times, want 1", circle.calls)
	}
}

func TestBridge_Step_AttestationUnavailable_KeepsPollingWithoutError(t *testing.T) {
	circle := &fakeAttester{err: errors.Join(ErrAttestationUnavailable, errors.New("503"))}

	b := newTestBridge(t, circle, &fakeHoldings{})
	res, err := b.Step(context.Background(), depositTransfer(StageAwaitingAttestation, map[string]string{metaBaselineBalance: "0"}))
	if err != nil {
		t.Fatalf("transient unavailability must not be a step error, got: %v", err)
	}
	if res.Stage != StageAwaitingAttestation {
		t.Fatalf("stage = %s, want %s", res.Stage, StageAwaitingAttestation)
	}
}

func TestBridge_Step_AttestationReady_MovesToAwaitingMint(t *testing.T) {
	circle := &fakeAttester{att: &Attestation{ID: "att-1", Status: "complete"}}

	b := newTestBridge(t, circle, &fakeHoldings{})
	res, err := b.Step(context.Background(), depositTransfer(StageAwaitingAttestation, map[string]string{metaBaselineBalance: "100"}))
	if err != nil {
		t.Fatalf("Step failed: %v", err)
	}
	if res.Status != relayer.TransferStatusInProgress || res.Stage != StageAwaitingMint {
		t.Fatalf("status/stage = %s/%s, want in_progress/%s", res.Status, res.Stage, StageAwaitingMint)
	}
	if res.Metadata[metaAttestationID] != "att-1" {
		t.Fatalf("attestation id = %q, want att-1", res.Metadata[metaAttestationID])
	}
	if res.RetryAfter != 5*time.Second {
		t.Fatalf("RetryAfter = %v, want 5s", res.RetryAfter)
	}
}

func TestBridge_Step_MintNotArrived_KeepsWaiting(t *testing.T) {
	holdings := &fakeHoldings{holdings: []*token.Holding{holding("100")}}

	b := newTestBridge(t, &fakeAttester{}, holdings)
	res, err := b.Step(context.Background(), depositTransfer(StageAwaitingMint, map[string]string{metaBaselineBalance: "100"}))
	if err != nil {
		t.Fatalf("Step failed: %v", err)
	}
	if res.Status != relayer.TransferStatusInProgress || res.Stage != StageAwaitingMint {
		t.Fatalf("status/stage = %s/%s, want in_progress/%s", res.Status, res.Stage, StageAwaitingMint)
	}
}

func TestBridge_Step_MintArrived_Completes(t *testing.T) {
	holdings := &fakeHoldings{holdings: []*token.Holding{holding("100"), holding("12.5")}}

	b := newTestBridge(t, &fakeAttester{}, holdings)
	res, err := b.Step(context.Background(), depositTransfer(StageAwaitingMint, map[string]string{metaBaselineBalance: "100"}))
	if err != nil {
		t.Fatalf("Step failed: %v", err)
	}
	if res.Status != relayer.TransferStatusCompleted || res.Stage != StageMinted {
		t.Fatalf("status/stage = %s/%s, want completed/%s", res.Status, res.Stage, StageMinted)
	}
}

func TestBridge_Step_UnknownToken_Fails(t *testing.T) {
	b := newTestBridge(t, &fakeAttester{}, &fakeHoldings{})

	transfer := depositTransfer("", nil)
	transfer.TokenSymbol = "UNKNOWN"
	if _, err := b.Step(context.Background(), transfer); err == nil {
		t.Fatalf("Step with unconfigured token should fail")
	}
}

func TestBridge_Step_UnknownStage_Fails(t *testing.T) {
	b := newTestBridge(t, &fakeAttester{}, &fakeHoldings{})

	if _, err := b.Step(context.Background(), depositTransfer("bogus", nil)); err == nil {
		t.Fatalf("Step with unknown stage should fail")
	}
}

func TestBridge_Step_WithdrawalsNotSupportedYet(t *testing.T) {
	b := newTestBridge(t, &fakeAttester{}, &fakeHoldings{})

	transfer := depositTransfer("", nil)
	transfer.Direction = relayer.DirectionCantonToEthereum
	if _, err := b.Step(context.Background(), transfer); err == nil {
		t.Fatalf("withdrawal Step should fail until #359 lands")
	}
}
