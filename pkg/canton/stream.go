package canton

import (
	"context"
	"fmt"
	"io"
	"strconv"

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

		// Parse offset - V2 API uses int64
		var beginOffset int64
		if offset == "BEGIN" || offset == "" {
			beginOffset = 0 // 0 means start from beginning
		} else {
			var err error
			beginOffset, err = strconv.ParseInt(offset, 10, 64)
			if err != nil {
				errCh <- fmt.Errorf("invalid offset %s: %w", offset, err)
				return
			}
		}

		// Create V2 update format with template filter
		updateFormat := &lapiv2.UpdateFormat{
			IncludeTransactions: &lapiv2.TransactionFormat{
				EventFormat: &lapiv2.EventFormat{
					FiltersByParty: map[string]*lapiv2.Filters{
						c.config.RelayerParty: {
							Cumulative: []*lapiv2.CumulativeFilter{
								{
									IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
										TemplateFilter: &lapiv2.TemplateFilter{
											TemplateId: &lapiv2.Identifier{
												PackageId:  c.config.BridgePackageID,
												ModuleName: c.config.BridgeModule,
												EntityName: "BurnEvent",
											},
										},
									},
								},
							},
						},
					},
					Verbose: true,
				},
				TransactionShape: lapiv2.TransactionShape_TRANSACTION_SHAPE_ACS_DELTA,
			},
		}

		// Start streaming updates with V2 API
		stream, err := c.updateService.GetUpdates(authCtx, &lapiv2.GetUpdatesRequest{
			BeginExclusive: beginOffset,
			UpdateFormat:   updateFormat,
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
								fmt.Sprintf("%d-%d", createdEvent.Offset, createdEvent.NodeId),
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

// StreamWithdrawalEvents streams WithdrawalEvent contracts from Canton (issuer-centric model)
func (c *Client) StreamWithdrawalEvents(ctx context.Context, offset string) (<-chan *WithdrawalEvent, <-chan error) {
	outCh := make(chan *WithdrawalEvent, 10)
	errCh := make(chan error, 1)

	go func() {
		defer close(outCh)
		defer close(errCh)

		c.logger.Info("Starting Canton withdrawal event stream", zap.String("offset", offset))

		authCtx := c.GetAuthContext(ctx)

		// Parse offset - V2 API uses int64
		var beginOffset int64
		if offset == "BEGIN" || offset == "" {
			beginOffset = 0 // 0 means start from beginning
		} else {
			var err error
			beginOffset, err = strconv.ParseInt(offset, 10, 64)
			if err != nil {
				errCh <- fmt.Errorf("invalid offset %s: %w", offset, err)
				return
			}
		}

		// Create V2 update format with template filter for WithdrawalEvent
		// Note: WithdrawalEvent is in bridge-core package (CorePackageID), not bridge-wayfinder
		corePackageID := c.config.CorePackageID
		if corePackageID == "" {
			corePackageID = c.config.BridgePackageID // fallback for backwards compatibility
		}

		updateFormat := &lapiv2.UpdateFormat{
			IncludeTransactions: &lapiv2.TransactionFormat{
				EventFormat: &lapiv2.EventFormat{
					FiltersByParty: map[string]*lapiv2.Filters{
						c.config.RelayerParty: {
							Cumulative: []*lapiv2.CumulativeFilter{
								{
									IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
										TemplateFilter: &lapiv2.TemplateFilter{
											TemplateId: &lapiv2.Identifier{
												PackageId:  corePackageID,
												ModuleName: "Bridge.Contracts",
												EntityName: "WithdrawalEvent",
											},
										},
									},
								},
							},
						},
					},
					Verbose: true,
				},
				TransactionShape: lapiv2.TransactionShape_TRANSACTION_SHAPE_ACS_DELTA,
			},
		}

		// Start streaming updates with V2 API
		stream, err := c.updateService.GetUpdates(authCtx, &lapiv2.GetUpdatesRequest{
			BeginExclusive: beginOffset,
			UpdateFormat:   updateFormat,
		})
		if err != nil {
			errCh <- fmt.Errorf("failed to start withdrawal stream: %w", err)
			return
		}

		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				errCh <- fmt.Errorf("withdrawal stream error: %w", err)
				return
			}

			// Handle transaction updates
			if tx := resp.GetTransaction(); tx != nil {
				for _, event := range tx.Events {
					if createdEvent := event.GetCreated(); createdEvent != nil {
						// Check if it matches our template
						if createdEvent.TemplateId.EntityName == "WithdrawalEvent" {
							withdrawalEvent, err := DecodeWithdrawalEvent(
								fmt.Sprintf("%d-%d", createdEvent.Offset, createdEvent.NodeId),
								tx.UpdateId,
								createdEvent.ContractId,
								createdEvent.CreateArguments,
							)
							if err != nil {
								c.logger.Error("Failed to decode withdrawal event", zap.Error(err))
								continue
							}

							// Only emit pending withdrawals (not already completed)
							if withdrawalEvent.Status == WithdrawalStatusPending {
								select {
								case outCh <- withdrawalEvent:
								case <-ctx.Done():
									return
								}
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
