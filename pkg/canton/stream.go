package canton

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	streamReconnectDelay    = 5 * time.Second
	streamMaxReconnectDelay = 60 * time.Second
)

// isAuthError checks if the error is an authentication/authorization error that requires token refresh
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	return st.Code() == codes.Unauthenticated || st.Code() == codes.PermissionDenied
}

// StreamWithdrawalEvents streams WithdrawalEvent contracts from Canton with automatic reconnection on token expiry
func (c *Client) StreamWithdrawalEvents(ctx context.Context, offset string) <-chan *WithdrawalEvent {
	outCh := make(chan *WithdrawalEvent, 10)

	go func() {
		defer close(outCh)

		currentOffset := offset
		reconnectDelay := streamReconnectDelay

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			c.logger.Info("Starting Canton withdrawal event stream", zap.String("offset", currentOffset))

			err := c.streamWithdrawalEventsOnce(ctx, currentOffset, outCh, &currentOffset)
			if err == nil || err == io.EOF {
				return
			}

			if isAuthError(err) {
				c.logger.Warn("Withdrawal stream auth error, refreshing token and reconnecting",
					zap.Error(err),
					zap.String("resume_offset", currentOffset))
				c.invalidateToken()
				reconnectDelay = streamReconnectDelay
			} else {
				c.logger.Error("Withdrawal stream error, reconnecting",
					zap.Error(err),
					zap.String("resume_offset", currentOffset),
					zap.Duration("delay", reconnectDelay))
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(reconnectDelay):
			}

			reconnectDelay = min(reconnectDelay*2, streamMaxReconnectDelay)
		}
	}()

	return outCh
}

func (c *Client) streamWithdrawalEventsOnce(ctx context.Context, offset string, outCh chan<- *WithdrawalEvent, lastOffset *string) error {
	authCtx := c.GetAuthContext(ctx)

	var beginOffset int64
	if offset == "BEGIN" || offset == "" {
		beginOffset = 0
	} else {
		var err error
		beginOffset, err = strconv.ParseInt(offset, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid offset %s: %w", offset, err)
		}
	}

	corePackageID := c.config.CorePackageID
	if corePackageID == "" {
		corePackageID = c.config.BridgePackageID
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

	stream, err := c.updateService.GetUpdates(authCtx, &lapiv2.GetUpdatesRequest{
		BeginExclusive: beginOffset,
		UpdateFormat:   updateFormat,
	})
	if err != nil {
		return fmt.Errorf("failed to start withdrawal stream: %w", err)
	}

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		if tx := resp.GetTransaction(); tx != nil {
			for _, event := range tx.Events {
				if createdEvent := event.GetCreated(); createdEvent != nil {
					*lastOffset = strconv.FormatInt(createdEvent.Offset, 10)

					templateId := createdEvent.TemplateId
					if templateId.ModuleName == "Bridge.Contracts" && templateId.EntityName == "WithdrawalEvent" {
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

						if withdrawalEvent.Status == WithdrawalStatusPending {
							select {
							case outCh <- withdrawalEvent:
							case <-ctx.Done():
								return ctx.Err()
							}
						}
					}
				}
			}
		}
	}
}

func generateUUID() string {
	return uuid.New().String()
}
