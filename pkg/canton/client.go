package canton

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/chainsafe/canton-middleware/pkg/canton/lapi"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

// Client represents a Canton Network gRPC client
type Client struct {
	config *config.CantonConfig
	conn   *grpc.ClientConn
	logger *zap.Logger

	transactionService     lapi.TransactionServiceClient
	commandService         lapi.CommandServiceClient
	activeContractsService lapi.ActiveContractsServiceClient
	ledgerIdentityService  lapi.LedgerIdentityServiceClient
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
		opts = append(opts, grpc.WithInsecure())
	}

	// Set max message size
	if config.MaxMessageSize > 0 {
		opts = append(opts, grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(config.MaxMessageSize)))
	}

	// Connect to Canton participant node
	conn, err := grpc.Dial(config.RPCURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial Canton node: %w", err)
	}

	logger.Info("Connected to Canton Network",
		zap.String("rpc_url", config.RPCURL),
		zap.String("ledger_id", config.LedgerID))

	return &Client{
		config:                 config,
		conn:                   conn,
		logger:                 logger,
		transactionService:     lapi.NewTransactionServiceClient(conn),
		commandService:         lapi.NewCommandServiceClient(conn),
		activeContractsService: lapi.NewActiveContractsServiceClient(conn),
		ledgerIdentityService:  lapi.NewLedgerIdentityServiceClient(conn),
	}, nil
}

// Close closes the gRPC connection
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// GetAuthContext returns a context with JWT authorization
func (c *Client) GetAuthContext(ctx context.Context) context.Context {
	// Generate JWT token with actAs and readAs claims
	token := c.generateJWT()
	md := metadata.Pairs("authorization", "Bearer "+token)
	return metadata.NewOutgoingContext(ctx, md)
}

// generateJWT generates a JWT token for Canton authentication
// TODO: Implement proper JWT generation with HS256/RS256
func (c *Client) generateJWT() string {
	// Placeholder - in production, generate a proper JWT with:
	// - actAs: [c.config.RelayerParty]
	// - readAs: [c.config.RelayerParty]
	// - exp: time.Now().Add(1 hour)
	// - iss: c.config.Auth.JWTIssuer
	return c.config.Auth.JWTSecret
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

// Placeholder methods - will be implemented once protobufs are generated

// StreamTransactions streams transactions from Canton
func (c *Client) StreamTransactions(ctx context.Context, offset string) error {
	c.logger.Info("Starting transaction stream", zap.String("offset", offset))
	// TODO: Implement using TransactionService.GetTransactions
	return fmt.Errorf("not implemented - protobuf generation required")
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
	cmd := &lapi.Command{
		Command: &lapi.Command_Exercise{
			Exercise: &lapi.ExerciseCommand{
				TemplateId: &lapi.Identifier{
					PackageId:  c.config.BridgePackageID, // Assuming same package for now, or needs config update
					ModuleName: "Wayfinder.Bridge",
					EntityName: "WayfinderBridgeConfig",
				},
				ContractId:     configCid,
				Choice:         "CreateMintProposal",
				ChoiceArgument: &lapi.Value{Sum: &lapi.Value_Record{Record: EncodeMintProposalArgs(req)}},
			},
		},
	}

	// Submit command
	_, err = c.commandService.SubmitAndWait(authCtx, &lapi.SubmitAndWaitRequest{
		Commands: &lapi.Commands{
			LedgerId:      c.config.LedgerID,
			ApplicationId: c.config.ApplicationID,
			CommandId:     generateUUID(),
			ActAs:         []string{c.config.RelayerParty},
			Commands:      []*lapi.Command{cmd},
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

	// Search for WayfinderBridgeConfig
	// Note: In a real implementation, we might want to cache this or be more specific with queries
	// For now, we assume there's one valid config visible to the relayer
	resp, err := c.activeContractsService.GetActiveContracts(authCtx, &lapi.GetActiveContractsRequest{
		LedgerId: c.config.LedgerID,
		Filter: &lapi.TransactionFilter{
			FiltersByParty: map[string]*lapi.Filters{
				c.config.RelayerParty: {
					Inclusive: &lapi.InclusiveFilters{
						TemplateIds: []*lapi.Identifier{
							{
								PackageId:  c.config.BridgePackageID, // TODO: Make sure this matches Wayfinder package
								ModuleName: "Wayfinder.Bridge",
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
	// We just take the first one we find for now
	for {
		msg, err := resp.Recv()
		if err != nil {
			// EOF or error
			break
		}
		for _, event := range msg.ActiveContracts {
			return event.ContractId, nil
		}
	}

	return "", fmt.Errorf("no active WayfinderBridgeConfig found")
}

// GetLedgerEnd gets the current ledger end offset
func (c *Client) GetLedgerEnd(ctx context.Context) (string, error) {
	authCtx := c.GetAuthContext(ctx)

	resp, err := c.transactionService.GetLedgerEnd(authCtx, &lapi.GetLedgerEndRequest{
		LedgerId: c.config.LedgerID,
	})
	if err != nil {
		return "", fmt.Errorf("failed to get ledger end: %w", err)
	}

	if resp.Offset == nil {
		return "", fmt.Errorf("received empty ledger offset")
	}

	if abs, ok := resp.Offset.Value.(*lapi.LedgerOffset_Absolute); ok {
		return abs.Absolute, nil
	}

	return "", fmt.Errorf("received non-absolute ledger offset")
}
