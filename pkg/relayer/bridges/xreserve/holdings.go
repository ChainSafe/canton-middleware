// SPDX-License-Identifier: Apache-2.0

package xreserve

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
)

// HoldingLister is the narrow slice of token.Token the adapter needs to
// observe a recipient's USDCx holdings on Canton.
//
// Reads are authorized by the query's party filter combined with the
// relayer's Canton auth token, so the relayer user must carry read rights
// over user parties (e.g. CanReadAsAnyParty, as the indexer already requires).
type HoldingLister interface {
	GetHoldingsByParty(ctx context.Context, ownerParty, instrumentID string) ([]*token.Holding, error)
}

// partyBalance sums the party's holdings of the given instrument (matched on
// both instrument id and admin party) as a decimal in token units. Locked
// holdings count: a freshly minted amount is a balance increase regardless of
// later locks.
func partyBalance(
	ctx context.Context,
	holdings HoldingLister,
	ownerParty, instrumentAdmin, instrumentID string,
) (decimal.Decimal, error) {
	list, err := holdings.GetHoldingsByParty(ctx, ownerParty, instrumentID)
	if err != nil {
		return decimal.Zero, fmt.Errorf("list holdings for %s: %w", ownerParty, err)
	}

	total := decimal.Zero
	for _, h := range list {
		if h.InstrumentAdmin != instrumentAdmin {
			continue
		}
		amount, parseErr := decimal.NewFromString(h.Amount)
		if parseErr != nil {
			return decimal.Zero, fmt.Errorf("parse holding %s amount %q: %w", h.ContractID, h.Amount, parseErr)
		}
		total = total.Add(amount)
	}
	return total, nil
}
