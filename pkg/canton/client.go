package canton

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/chainsafe/canton-middleware/pkg/canton/lapi"
	lapiv1 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v1"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// Client is a wrapper around the Canton gRPC client
type Client struct {
	config *config.CantonConfig
	conn   *grpc.ClientConn
	logger *zap.Logger

	stateService   lapi.StateServiceClient
	commandService lapi.CommandServiceClient
	updateService  lapi.UpdateServiceClient
}

// MintProposalRequest represents a request to mint tokens on Canton
type MintProposalRequest struct {
	Operator        string
	Issuer          string
	Recipient       string
	TokenManagerCID string
	Amount          string
	Reference       string // EVM Tx Hash
}

// BurnEvent represents a burn event on Canton
type BurnEvent struct {
	EventID       string
	TransactionID string
	Operator      string
	Owner         string
	Amount        string
	Destination   string // EVM Address
	Reference     string
}

// NewClient creates a new Canton gRPC client
func NewClient(config *config.CantonConfig, logger *zap.Logger) (*Client, error) {
	var opts []grpc.DialOption

	// Setup TLS if enabled
	if config.TLS.Enabled {
		tlsConfig, err := loadTLSConfig(&config.TLS)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS config: %w", err)
		}
		creds := credentials.NewTLS(tlsConfig)
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// Set max message size
	if config.MaxMessageSize > 0 {
		opts = append(opts, grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(config.MaxMessageSize)))
	}

	// Connect to Canton participant node
	conn, err := grpc.NewClient(config.RPCURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Canton client: %w", err)
	}

	logger.Info("Connected to Canton Network",
		zap.String("rpc_url", config.RPCURL),
		zap.String("ledger_id", config.LedgerID))

	return &Client{
		config:         config,
		conn:           conn,
		logger:         logger,
		stateService:   lapi.NewStateServiceClient(conn),
		commandService: lapi.NewCommandServiceClient(conn),
		updateService:  lapi.NewUpdateServiceClient(conn),
	}, nil
}

// Close closes the connection to the Canton node
func (c *Client) Close() error {
	return c.conn.Close()
}

// GetAuthContext returns a context with the JWT token if configured
func (c *Client) GetAuthContext(ctx context.Context) context.Context {
	// TODO: Implement JWT generation/loading
	if c.config.Auth.JWTSecret != "" {
		md := metadata.Pairs("authorization", "Bearer "+c.config.Auth.JWTSecret)
		return metadata.NewOutgoingContext(ctx, md)
	}
	return ctx
}

// loadTLSConfig loads TLS configuration from files
func loadTLSConfig(tlsCfg *config.TLSConfig) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(tlsCfg.CertFile, tlsCfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load client cert/key: %w", err)
	}

	caCert, err := os.ReadFile(tlsCfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA cert: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to append CA cert")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
	}, nil
}

// StreamTransactions streams transactions from the Canton ledger
func (c *Client) StreamTransactions(ctx context.Context, offset string, filter *lapi.TransactionFilter) (grpc.ServerStreamingClient[lapi.GetUpdatesResponse], error) {
	authCtx := c.GetAuthContext(ctx)

	// Set the starting offset
	var begin *lapi.ParticipantOffset
	if offset == "BEGIN" || offset == "" {
		begin = &lapi.ParticipantOffset{
			Value: &lapi.ParticipantOffset_Boundary{
				Boundary: lapi.ParticipantOffset_PARTICIPANT_BEGIN,
			},
		}
	} else {
		begin = &lapi.ParticipantOffset{
			Value: &lapi.ParticipantOffset_Absolute{Absolute: offset},
		}
	}

	req := &lapi.GetUpdatesRequest{
		BeginExclusive: begin,
		Filter:         filter,
	}

	return c.updateService.GetUpdates(authCtx, req)
}

// SubmitMintProposal submits a mint proposal via WayfinderBridgeConfig
func (c *Client) SubmitMintProposal(ctx context.Context, req *MintProposalRequest) error {
	c.logger.Info("Submitting mint proposal",
		zap.String("reference", req.Reference),
		zap.String("recipient", req.Recipient),
		zap.String("amount", req.Amount))

	// Get WayfinderBridgeConfig contract ID
	configCid, err := c.GetWayfinderBridgeConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to get WayfinderBridgeConfig: %w", err)
	}

	authCtx := c.GetAuthContext(ctx)

	// Create the exercise command
	cmd := &lapiv1.Command{
		Command: &lapiv1.Command_Exercise{
			Exercise: &lapiv1.ExerciseCommand{
				TemplateId: &lapiv1.Identifier{
					PackageId:  c.config.BridgePackageID, // Assuming same package for now, or needs config update
					ModuleName: "Wayfinder.Bridge",
					EntityName: "WayfinderBridgeConfig",
				},
				ContractId:     configCid,
				Choice:         "CreateMintProposal",
				ChoiceArgument: &lapiv1.Value{Sum: &lapiv1.Value_Record{Record: EncodeMintProposalArgs(req)}},
			},
		},
	}

	// Submit command
	_, err = c.commandService.SubmitAndWait(authCtx, &lapi.SubmitAndWaitRequest{
		Commands: &lapi.Commands{
			DomainId:      c.config.DomainID,
			ApplicationId: c.config.ApplicationID,
			CommandId:     generateUUID(),
			ActAs:         []string{c.config.RelayerParty},
			Commands:      []*lapiv1.Command{cmd},
		},
	})

	if err != nil {
		return fmt.Errorf("failed to submit mint proposal: %w", err)
	}

	return nil
}

// GetWayfinderBridgeConfig finds the active WayfinderBridgeConfig contract
func (c *Client) GetWayfinderBridgeConfig(ctx context.Context) (string, error) {
	authCtx := c.GetAuthContext(ctx)

	resp, err := c.stateService.GetActiveContracts(authCtx, &lapi.GetActiveContractsRequest{
		Filter: &lapi.TransactionFilter{
			FiltersByParty: map[string]*lapiv1.Filters{
				c.config.RelayerParty: {
					Inclusive: &lapiv1.InclusiveFilters{
						TemplateIds: []*lapiv1.Identifier{
							{
								PackageId:  c.config.BridgePackageID,
								ModuleName: c.config.BridgeModule,
								EntityName: "WayfinderBridgeConfig",
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to search for config: %w", err)
	}

	// Read the stream
	for {
		msg, err := resp.Recv()
		if err != nil {
			break // EOF or error
		}
		// In V2, ActiveContracts are delivered via ContractEntry oneof
		if contract := msg.GetActiveContract(); contract != nil {
			return contract.CreatedEvent.ContractId, nil
		}
	}

	return "", fmt.Errorf("no active WayfinderBridgeConfig found")
}

// GetLedgerEnd gets the current ledger end offset
func (c *Client) GetLedgerEnd(ctx context.Context) (string, error) {
	authCtx := c.GetAuthContext(ctx)

	resp, err := c.stateService.GetLedgerEnd(authCtx, &lapi.GetLedgerEndRequest{})
	if err != nil {
		return "", fmt.Errorf("failed to get ledger end: %w", err)
	}

	if resp.Offset == nil {
		return "", fmt.Errorf("received empty ledger offset")
	}

	if abs, ok := resp.Offset.Value.(*lapi.ParticipantOffset_Absolute); ok {
		return abs.Absolute, nil
	}

	return "", fmt.Errorf("received non-absolute ledger offset")
}
