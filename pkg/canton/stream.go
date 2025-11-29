package canton

import (
	"context"
	"fmt"
	"io"

	lapiv1 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v1"
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// StreamBurnEvents streams BurnEvent events from Canton
func (c *Client) StreamBurnEvents(ctx context.Context, offset string) (<-chan *BurnEvent, <-chan error) {
	outCh := make(chan *BurnEvent, 10)
	errCh := make(chan error, 1)

	go func() {
		defer close(outCh)
		defer close(errCh)

		c.logger.Info("Starting Canton burn event stream", zap.String("offset", offset))

		authCtx := c.GetAuthContext(ctx)

		// Create the transaction filter
		filter := &lapiv2.TransactionFilter{
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
		var begin *lapiv2.ParticipantOffset
		if offset == "BEGIN" || offset == "" {
			begin = &lapiv2.ParticipantOffset{
				Value: &lapiv2.ParticipantOffset_Boundary{
					Boundary: lapiv2.ParticipantOffset_PARTICIPANT_BEGIN,
				},
			}
		} else {
			begin = &lapiv2.ParticipantOffset{
				Value: &lapiv2.ParticipantOffset_Absolute{
					Absolute: offset,
				},
			}
		}

		// Start streaming updates
		stream, err := c.updateService.GetUpdates(authCtx, &lapiv2.GetUpdatesRequest{
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
							case outCh <- burnEvent:
							case <-ctx.Done():
								return
							}
						}
					}
				}
			}
		}
	}()

	return outCh, errCh
}

func generateUUID() string {
	return uuid.New().String()
}
