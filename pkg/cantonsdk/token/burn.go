// SPDX-License-Identifier: Apache-2.0

package token

import (
	"context"
	"fmt"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"
)

// xReserve burn (USDCx outbound): the user party exercises
// BridgeUserAgreement_Burn on their own BridgeUserAgreement contract; Circle
// validates the burn and releases the asset on the destination chain. The
// factory contract id, choice context, and disclosed contracts come from the
// registrar's burn-mint registry endpoint.
//
// The choice name and argument field names follow DA's devnet xReserve
// documentation and the devstack stub; they must be confirmed against the
// production utility-bridge DAR before mainnet enablement (#360).
const (
	burnChoice          = "BridgeUserAgreement_Burn"
	burnAgreementEntity = "BridgeUserAgreement"
)

// findBridgeUserAgreement locates the party's BridgeUserAgreement contract.
// Its absence means the party has not been onboarded for bridging.
func (c *Client) findBridgeUserAgreement(ctx context.Context, partyID string) (string, error) {
	end, err := c.ledger.GetLedgerEnd(ctx)
	if err != nil {
		return "", fmt.Errorf("get ledger end: %w", err)
	}
	if end == 0 {
		return "", fmt.Errorf("ledger is empty")
	}

	tid := &lapiv2.Identifier{
		PackageId:  c.cfg.BurnMintPackageID,
		ModuleName: c.cfg.BurnMintModule,
		EntityName: burnAgreementEntity,
	}
	events, err := c.ledger.GetActiveContractsByTemplate(ctx, end, []string{partyID}, tid)
	if err != nil {
		return "", fmt.Errorf("query BridgeUserAgreement contracts: %w", err)
	}
	if len(events) == 0 {
		return "", fmt.Errorf("party %s has no BridgeUserAgreement (bridge onboarding required)", partyID)
	}
	return events[0].ContractId, nil
}

// buildBurnCommand assembles the BridgeUserAgreement_Burn exercise: holdings
// selection, burn-mint factory + choice context from the registrar, and the
// choice argument. Returns the command, disclosed contracts, and the
// agreement contract id.
func (c *Client) buildBurnCommand(
	ctx context.Context, req *PrepareBurnRequest,
) (*lapiv2.Command, []*lapiv2.DisclosedContract, string, error) {
	if c.cfg.BurnMintPackageID == "" || c.cfg.BurnMintModule == "" {
		return nil, nil, "", fmt.Errorf("burn is not configured: set burn_mint_package_id and burn_mint_module")
	}
	if c.registryClient == nil {
		return nil, nil, "", fmt.Errorf("no registry client configured for burn")
	}
	extCfg, ok := c.cfg.ExternalTokens[req.InstrumentAdmin]
	if !ok {
		return nil, nil, "", fmt.Errorf("unsupported external token issuer: %s", req.InstrumentAdmin)
	}

	agreementCID, err := c.findBridgeUserAgreement(ctx, req.PartyID)
	if err != nil {
		return nil, nil, "", err
	}

	holdings, err := c.GetHoldings(ctx, req.PartyID, req.TokenSymbol)
	if err != nil {
		return nil, nil, "", fmt.Errorf("list holdings: %w", err)
	}
	selected, err := selectHoldingsForTransfer(holdings, req.Amount)
	if err != nil {
		return nil, nil, "", err
	}

	factoryResp, err := c.registryClient.GetBurnMintFactory(
		ctx, extCfg.RegistryURL, req.InstrumentAdmin, &RegistryRequest{ExcludeDebugFields: true})
	if err != nil {
		return nil, nil, "", fmt.Errorf("get burn-mint factory: %w", err)
	}

	anyCtx, err := ConvertAnyValueChoiceContext(factoryResp.ChoiceContext)
	if err != nil {
		return nil, nil, "", fmt.Errorf("convert burn choice context: %w", err)
	}
	ctxValue, err := encodeChoiceContextRecord(anyCtx)
	if err != nil {
		return nil, nil, "", fmt.Errorf("encode burn choice context: %w", err)
	}
	extraArgs := &lapiv2.Value{
		Sum: &lapiv2.Value_Record{
			Record: &lapiv2.Record{
				Fields: []*lapiv2.RecordField{
					{Label: "context", Value: ctxValue},
					{Label: "meta", Value: values.EmptyMetadata()},
				},
			},
		},
	}

	disclosed, err := ConvertDisclosedContracts(factoryResp.DisclosedContracts, c.cfg.DomainID)
	if err != nil {
		return nil, nil, "", fmt.Errorf("convert disclosed contracts: %w", err)
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  c.cfg.BurnMintPackageID,
					ModuleName: c.cfg.BurnMintModule,
					EntityName: burnAgreementEntity,
				},
				ContractId: agreementCID,
				Choice:     burnChoice,
				ChoiceArgument: &lapiv2.Value{
					Sum: &lapiv2.Value_Record{
						Record: encodeBurnArgs(req, factoryResp.FactoryID, selected.CIDs, extraArgs),
					},
				},
			},
		},
	}
	return cmd, disclosed, agreementCID, nil
}

// encodeBurnArgs builds the BridgeUserAgreement_Burn choice argument. Field
// names are pinned to the devstack stub (see package comment above).
func encodeBurnArgs(
	req *PrepareBurnRequest, factoryCID string, holdingCIDs []string, extraArgs *lapiv2.Value,
) *lapiv2.Record {
	holdingValues := make([]*lapiv2.Value, len(holdingCIDs))
	for i, cid := range holdingCIDs {
		holdingValues[i] = values.ContractIDValue(cid)
	}

	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "factoryCid", Value: values.ContractIDValue(factoryCID)},
			{Label: "destinationDomain", Value: values.Int64Value(req.DestinationDomain)},
			{Label: "amount", Value: values.NumericValue(req.Amount)},
			{Label: "recipient", Value: values.TextValue(req.EvmRecipient)},
			{Label: "requestId", Value: values.TextValue(req.RequestID)},
			{Label: "inputHoldingCids", Value: values.ListValue(holdingValues)},
			{Label: "extraArgs", Value: extraArgs},
		},
	}
}

func (req *PrepareBurnRequest) validate() error {
	if req.PartyID == "" || req.TokenSymbol == "" || req.Amount == "" ||
		req.InstrumentAdmin == "" || req.EvmRecipient == "" || req.RequestID == "" {
		return fmt.Errorf("party_id, token_symbol, amount, instrument_admin, evm_recipient, and request_id are required")
	}
	return nil
}

// PrepareBurn builds a BridgeUserAgreement_Burn for a non-custodial party and
// returns the hash the party signs externally. Use ExecuteTransfer to
// complete it.
func (c *Client) PrepareBurn(ctx context.Context, req *PrepareBurnRequest) (*PreparedTransfer, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	cmd, disclosed, agreementCID, err := c.buildBurnCommand(ctx, req)
	if err != nil {
		return nil, err
	}
	return c.prepareInstructionTx(ctx, req.PartyID, agreementCID, cmd, disclosed)
}

// BurnByPartyID burns on behalf of a custodial party, signing with the
// middleware-held key via Interactive Submission.
func (c *Client) BurnByPartyID(ctx context.Context, req *PrepareBurnRequest) error {
	if err := req.validate(); err != nil {
		return err
	}
	cmd, disclosed, _, err := c.buildBurnCommand(ctx, req)
	if err != nil {
		return err
	}
	return c.exerciseInstructionAsCustodial(ctx, req.PartyID, cmd, disclosed)
}
