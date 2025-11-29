package canton

import (
	"context"
	"fmt"
	"io"

	"github.com/chainsafe/canton-middleware/pkg/canton/lapi"
	lapiv1 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v1"
	"go.uber.org/zap"
)

// StreamBurnEvents streams BurnEvent events from Canton
func (c *Client) StreamBurnEvents(ctx context.Context, startOffset string) (<-chan *BurnEvent, <-chan error) {
	burnCh := make(chan *BurnEvent, 10)
	errCh := make(chan error, 1)

	go func() {
		defer close(burnCh)
		defer close(errCh)

		c.logger.Info("Starting Canton burn event stream", zap.String("offset", startOffset))

		authCtx := c.GetAuthContext(ctx)

		// Create the transaction filter
		filter := &lapi.TransactionFilter{
			FiltersByParty: map[string]*lapiv1.Filters{
				c.config.RelayerParty: {
					Inclusive: &lapiv1.InclusiveFilters{
						TemplateIds: []*lapiv1.Identifier{
							{
								PackageId:  c.config.BridgePackageID,
								ModuleName: c.config.BridgeModule,
								EntityName: "BurnEvent",
							},
						},
					},
				},
			},
		}

		// Set the starting offset
		var begin *lapi.ParticipantOffset
		if startOffset == "BEGIN" || startOffset == "" {
			begin = &lapi.ParticipantOffset{
				Value: &lapi.ParticipantOffset_Boundary{
					Boundary: lapi.ParticipantOffset_PARTICIPANT_BEGIN,
				},
			}
		} else {
			begin = &lapi.ParticipantOffset{
				Value: &lapi.ParticipantOffset_Absolute{
					Absolute: startOffset,
				},
			}
		}

		// Start streaming updates
		stream, err := c.updateService.GetUpdates(authCtx, &lapi.GetUpdatesRequest{
			BeginExclusive: begin,
			Filter:         filter,
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

			// Handle transaction updates
			if tx := resp.GetTransaction(); tx != nil {
				for _, event := range tx.Events {
					if createdEvent := event.GetCreated(); createdEvent != nil {
						// Check if it matches our template
						if createdEvent.TemplateId.EntityName == "BurnEvent" {
							burnEvent, err := DecodeBurnEvent(
								createdEvent.EventId,
								tx.UpdateId,
								createdEvent.CreateArguments,
							)
							if err != nil {
								c.logger.Error("Failed to decode burn event", zap.Error(err))
								continue
							}

							select {
							case burnCh <- burnEvent:
							case <-ctx.Done():
								return
							}
						}
					}
				}
			}
		}
	}()

	return burnCh, errCh
}

func generateUUID() string {
	// Simple UUID generation
	// In production, use github.com/google/uuid
	return fmt.Sprintf("%d", 1234567890)
}
