package canton

import (
	"context"
	"fmt"
	"io"

	"github.com/chainsafe/canton-middleware/pkg/canton/lapi"
	"go.uber.org/zap"
)

// StreamDeposits streams DepositRequest events from Canton
func (c *Client) StreamDeposits(ctx context.Context, startOffset string) (<-chan *DepositRequest, <-chan error) {
	depositCh := make(chan *DepositRequest, 10)
	errCh := make(chan error, 1)

	go func() {
		defer close(depositCh)
		defer close(errCh)

		c.logger.Info("Starting Canton deposit stream", zap.String("offset", startOffset))

		authCtx := c.GetAuthContext(ctx)

		// Create the transaction filter
		filter := &lapi.TransactionFilter{
			FiltersByParty: map[string]*lapi.Filters{
				c.config.RelayerParty: {
					Inclusive: &lapi.InclusiveFilters{
						TemplateIds: []*lapi.Identifier{
							{
								PackageId:  c.config.BridgePackageID,
								ModuleName: c.config.BridgeModule,
								EntityName: "DepositRequest",
							},
						},
					},
				},
			},
		}

		// Set the starting offset
		var begin *lapi.LedgerOffset
		if startOffset == "BEGIN" || startOffset == "" {
			begin = &lapi.LedgerOffset{
				Value: &lapi.LedgerOffset_Boundary{
					Boundary: lapi.LedgerOffset_LEDGER_BEGIN,
				},
			}
		} else {
			begin = &lapi.LedgerOffset{
				Value: &lapi.LedgerOffset_Absolute{
					Absolute: startOffset,
				},
			}
		}

		// Start streaming transactions
		stream, err := c.transactionService.GetTransactions(authCtx, &lapi.GetTransactionsRequest{
			LedgerId: c.config.LedgerID,
			Begin:    begin,
			Filter:   filter,
		})
		if err != nil {
			errCh <- fmt.Errorf("failed to start stream: %w", err)
			return
		}

		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				errCh <- fmt.Errorf("stream error: %w", err)
				return
			}

			for _, tx := range resp.Transactions {
				for _, event := range tx.Events {
					if createdEvent, ok := event.Event.(*lapi.Event_Created); ok {
						// Check if it matches our template
						if createdEvent.Created.TemplateId.EntityName == "DepositRequest" {
							deposit, err := DecodeDepositRequest(
								createdEvent.Created.EventId,
								tx.TransactionId,
								createdEvent.Created.CreateArguments,
							)
							if err != nil {
								c.logger.Error("Failed to decode deposit", zap.Error(err))
								continue
							}

							select {
							case depositCh <- deposit:
							case <-ctx.Done():
								return
							}
						}
					}
				}
			}
		}
	}()

	return depositCh, errCh
}

func generateUUID() string {
	// Simple UUID generation
	// In production, use github.com/google/uuid
	return fmt.Sprintf("%d", 1234567890)
}
