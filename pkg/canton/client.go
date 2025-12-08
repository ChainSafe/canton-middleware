package canton

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strconv"
	"strings"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
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

	stateService   lapiv2.StateServiceClient
	commandService lapiv2.CommandServiceClient
	updateService  lapiv2.UpdateServiceClient
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
		stateService:   lapiv2.NewStateServiceClient(conn),
		commandService: lapiv2.NewCommandServiceClient(conn),
		updateService:  lapiv2.NewUpdateServiceClient(conn),
	}, nil
}

// Close closes the connection to the Canton node
func (c *Client) Close() error {
	return c.conn.Close()
}

// GetAuthContext returns a context with the JWT token if configured
// When using wildcard auth, no token is needed and this returns the original context
func (c *Client) GetAuthContext(ctx context.Context) context.Context {
	token, err := c.loadToken()
	if err != nil {
		// Not an error with wildcard auth - just means no token configured
		c.logger.Debug("No JWT token configured (OK with wildcard auth)", zap.Error(err))
		return ctx
	}

	if token != "" {
		md := metadata.Pairs("authorization", "Bearer "+token)
		return metadata.NewOutgoingContext(ctx, md)
	}
	return ctx
}

func (c *Client) loadToken() (string, error) {
	// If token file is provided, read from file
	if c.config.Auth.TokenFile != "" {
		tokenBytes, err := os.ReadFile(c.config.Auth.TokenFile)
		if err != nil {
			return "", fmt.Errorf("failed to read token file: %w", err)
		}
		// Trim whitespace/newlines - critical for JWT tokens!
		token := strings.TrimSpace(string(tokenBytes))
		return token, nil
	}

	return "", fmt.Errorf("no token file or token provided")
}

// loadTLSConfig loads TLS configuration from files
// If no cert files are provided, uses system CA pool (standard TLS)
// If cert files are provided, uses mTLS (mutual TLS with client certs)
func loadTLSConfig(tlsCfg *config.TLSConfig) (*tls.Config, error) {
	tlsConfig := &tls.Config{}

	// If client cert/key provided, load them (mTLS)
	if tlsCfg.CertFile != "" && tlsCfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(tlsCfg.CertFile, tlsCfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client cert/key: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// If CA file provided, use it; otherwise use system CA pool
	if tlsCfg.CAFile != "" {
		caCert, err := os.ReadFile(tlsCfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA cert: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to append CA cert")
		}
		tlsConfig.RootCAs = caCertPool
	}
	// If no CA file, tlsConfig.RootCAs = nil uses system CA pool

	return tlsConfig, nil
}

// StreamTransactions streams transactions from the Canton ledger
// V2 API: offset is int64 and uses UpdateFormat instead of TransactionFilter
func (c *Client) StreamTransactions(ctx context.Context, offset string, updateFormat *lapiv2.UpdateFormat) (grpc.ServerStreamingClient[lapiv2.GetUpdatesResponse], error) {
	authCtx := c.GetAuthContext(ctx)

	// Parse offset - V2 API uses int64
	var beginOffset int64
	if offset == "BEGIN" || offset == "" {
		beginOffset = 0 // 0 means start from beginning
	} else {
		var err error
		beginOffset, err = strconv.ParseInt(offset, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid offset %s: %w", offset, err)
		}
	}

	req := &lapiv2.GetUpdatesRequest{
		BeginExclusive: beginOffset,
		UpdateFormat:   updateFormat,
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
	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  c.config.BridgePackageID, // Assuming same package for now, or needs config update
					ModuleName: "Wayfinder.Bridge",
					EntityName: "WayfinderBridgeConfig",
				},
				ContractId:     configCid,
				Choice:         "CreateMintProposal",
				ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: EncodeMintProposalArgs(req)}},
			},
		},
	}

	// Submit command
	_, err = c.commandService.SubmitAndWait(authCtx, &lapiv2.SubmitAndWaitRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: c.config.DomainID,
			CommandId:      generateUUID(),
			UserId:         "bridge-relayer",
			ActAs:          []string{c.config.RelayerParty},
			Commands:       []*lapiv2.Command{cmd},
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

	// V2 API requires ActiveAtOffset - get current ledger end
	ledgerEndResp, err := c.stateService.GetLedgerEnd(authCtx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		return "", fmt.Errorf("failed to get ledger end: %w", err)
	}
	activeAtOffset := ledgerEndResp.Offset
	if activeAtOffset == 0 {
		return "", fmt.Errorf("ledger is empty, no contracts exist")
	}

	// V2 API: GetActiveContracts uses EventFormat with FiltersByParty and Cumulative filters
	resp, err := c.stateService.GetActiveContracts(authCtx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: activeAtOffset,
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
										EntityName: "WayfinderBridgeConfig",
									},
								},
							},
						},
					},
				},
			},
			Verbose: true,
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

	resp, err := c.stateService.GetLedgerEnd(authCtx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		return "", fmt.Errorf("failed to get ledger end: %w", err)
	}

	// V2 API: Offset is int64 directly
	// 0 = empty ledger, positive = absolute offset
	if resp.Offset == 0 {
		return "BEGIN", nil
	}

	return strconv.FormatInt(resp.Offset, 10), nil
}

// =============================================================================
// ISSUER-CENTRIC MODEL METHODS
// =============================================================================

// RegisterUser creates a FingerprintMapping for a new user
// Called after allocating a party via AllocateParty API
func (c *Client) RegisterUser(ctx context.Context, req *RegisterUserRequest) (string, error) {
	c.logger.Info("Registering user fingerprint mapping",
		zap.String("user_party", req.UserParty),
		zap.String("fingerprint", req.Fingerprint))

	configCid, err := c.GetWayfinderBridgeConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get WayfinderBridgeConfig: %w", err)
	}

	authCtx := c.GetAuthContext(ctx)

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  c.config.BridgePackageID,
					ModuleName: "Wayfinder.Bridge",
					EntityName: "WayfinderBridgeConfig",
				},
				ContractId:     configCid,
				Choice:         "RegisterUser",
				ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: EncodeRegisterUserArgs(req)}},
			},
		},
	}

	resp, err := c.commandService.SubmitAndWaitForTransaction(authCtx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: c.config.DomainID,
			CommandId:      generateUUID(),
			UserId:         "bridge-relayer",
			ActAs:          []string{c.config.RelayerParty},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to register user: %w", err)
	}

	// Extract the created FingerprintMapping contract ID from response
	if resp.Transaction != nil {
		for _, event := range resp.Transaction.Events {
			if created := event.GetCreated(); created != nil {
				templateId := created.TemplateId
				if templateId.ModuleName == "Common.FingerprintAuth" && templateId.EntityName == "FingerprintMapping" {
					return created.ContractId, nil
				}
			}
		}
	}

	return "", fmt.Errorf("FingerprintMapping contract not found in response")
}

// GetFingerprintMapping finds a FingerprintMapping by fingerprint
func (c *Client) GetFingerprintMapping(ctx context.Context, fingerprint string) (*FingerprintMapping, error) {
	authCtx := c.GetAuthContext(ctx)

	// V2 API requires ActiveAtOffset - get current ledger end
	ledgerEndResp, err := c.stateService.GetLedgerEnd(authCtx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ledger end: %w", err)
	}
	activeAtOffset := ledgerEndResp.Offset
	if activeAtOffset == 0 {
		return nil, fmt.Errorf("ledger is empty, no contracts exist")
	}

	// Use wildcard filter to find FingerprintMapping contracts
	// FingerprintMapping is in the 'common' package which has a different package ID
	// than bridge-wayfinder, so we use a wildcard and filter by entity name
	resp, err := c.stateService.GetActiveContracts(authCtx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: activeAtOffset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				c.config.RelayerParty: {
					Cumulative: []*lapiv2.CumulativeFilter{
						{
							IdentifierFilter: &lapiv2.CumulativeFilter_WildcardFilter{
								WildcardFilter: &lapiv2.WildcardFilter{},
							},
						},
					},
				},
			},
			Verbose: true,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search for FingerprintMapping: %w", err)
	}

	// Read through the stream to find the matching FingerprintMapping
	for {
		msg, err := resp.Recv()
		if err != nil {
			break // EOF or error
		}
		if contract := msg.GetActiveContract(); contract != nil {
			// Filter by module and entity name since we're using wildcard
			templateId := contract.CreatedEvent.TemplateId
			if templateId.ModuleName != "Common.FingerprintAuth" || templateId.EntityName != "FingerprintMapping" {
				continue
			}
			mapping, err := DecodeFingerprintMapping(
				contract.CreatedEvent.ContractId,
				contract.CreatedEvent.CreateArguments,
			)
			if err != nil {
				c.logger.Warn("Failed to decode FingerprintMapping", zap.Error(err))
				continue
			}
			if mapping.Fingerprint == fingerprint {
				return mapping, nil
			}
		}
	}

	return nil, fmt.Errorf("no FingerprintMapping found for fingerprint: %s", fingerprint)
}

// CreatePendingDeposit creates a PendingDeposit from an EVM deposit event
func (c *Client) CreatePendingDeposit(ctx context.Context, req *CreatePendingDepositRequest) (string, error) {
	c.logger.Info("Creating pending deposit",
		zap.String("fingerprint", req.Fingerprint),
		zap.String("amount", req.Amount),
		zap.String("evm_tx_hash", req.EvmTxHash))

	configCid, err := c.GetWayfinderBridgeConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get WayfinderBridgeConfig: %w", err)
	}

	authCtx := c.GetAuthContext(ctx)

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  c.config.BridgePackageID,
					ModuleName: "Wayfinder.Bridge",
					EntityName: "WayfinderBridgeConfig",
				},
				ContractId:     configCid,
				Choice:         "CreatePendingDeposit",
				ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: EncodeCreatePendingDepositArgs(req)}},
			},
		},
	}

	resp, err := c.commandService.SubmitAndWaitForTransaction(authCtx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: c.config.DomainID,
			CommandId:      generateUUID(),
			UserId:         "bridge-relayer",
			ActAs:          []string{c.config.RelayerParty},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to create pending deposit: %w", err)
	}

	// Extract the created PendingDeposit contract ID
	if resp.Transaction != nil {
		for _, event := range resp.Transaction.Events {
			if created := event.GetCreated(); created != nil {
				templateId := created.TemplateId
				if templateId.ModuleName == "Common.FingerprintAuth" && templateId.EntityName == "PendingDeposit" {
					return created.ContractId, nil
				}
			}
		}
	}

	return "", fmt.Errorf("PendingDeposit contract not found in response")
}

// ProcessDeposit processes a pending deposit and mints tokens
func (c *Client) ProcessDeposit(ctx context.Context, req *ProcessDepositRequest) (string, error) {
	c.logger.Info("Processing deposit and minting tokens",
		zap.String("deposit_cid", req.DepositCid),
		zap.String("mapping_cid", req.MappingCid))

	configCid, err := c.GetWayfinderBridgeConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get WayfinderBridgeConfig: %w", err)
	}

	authCtx := c.GetAuthContext(ctx)

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  c.config.BridgePackageID,
					ModuleName: "Wayfinder.Bridge",
					EntityName: "WayfinderBridgeConfig",
				},
				ContractId:     configCid,
				Choice:         "ProcessDepositAndMint",
				ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: EncodeProcessDepositAndMintArgs(req)}},
			},
		},
	}

	resp, err := c.commandService.SubmitAndWaitForTransaction(authCtx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: c.config.DomainID,
			CommandId:      generateUUID(),
			UserId:         "bridge-relayer",
			ActAs:          []string{c.config.RelayerParty},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to process deposit: %w", err)
	}

	// Extract the created CIP56Holding contract ID
	if resp.Transaction != nil {
		for _, event := range resp.Transaction.Events {
			if created := event.GetCreated(); created != nil {
				templateId := created.TemplateId
				if templateId.ModuleName == "CIP56.Token" && templateId.EntityName == "CIP56Holding" {
					return created.ContractId, nil
				}
			}
		}
	}

	return "", fmt.Errorf("CIP56Holding contract not found in response")
}

// InitiateWithdrawal starts a withdrawal on behalf of a user
func (c *Client) InitiateWithdrawal(ctx context.Context, req *InitiateWithdrawalRequest) (string, error) {
	c.logger.Info("Initiating withdrawal",
		zap.String("mapping_cid", req.MappingCid),
		zap.String("holding_cid", req.HoldingCid),
		zap.String("amount", req.Amount),
		zap.String("evm_destination", req.EvmDestination))

	configCid, err := c.GetWayfinderBridgeConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get WayfinderBridgeConfig: %w", err)
	}

	authCtx := c.GetAuthContext(ctx)

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  c.config.BridgePackageID,
					ModuleName: "Wayfinder.Bridge",
					EntityName: "WayfinderBridgeConfig",
				},
				ContractId:     configCid,
				Choice:         "InitiateWithdrawal",
				ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: EncodeInitiateWithdrawalArgs(req)}},
			},
		},
	}

	resp, err := c.commandService.SubmitAndWaitForTransaction(authCtx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: c.config.DomainID,
			CommandId:      generateUUID(),
			UserId:         "bridge-relayer",
			ActAs:          []string{c.config.RelayerParty},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to initiate withdrawal: %w", err)
	}

	// Extract the created WithdrawalRequest contract ID
	if resp.Transaction != nil {
		for _, event := range resp.Transaction.Events {
			if created := event.GetCreated(); created != nil {
				templateId := created.TemplateId
				if templateId.ModuleName == "Bridge.Contracts" && templateId.EntityName == "WithdrawalRequest" {
					return created.ContractId, nil
				}
			}
		}
	}

	return "", fmt.Errorf("WithdrawalRequest contract not found in response")
}

// CompleteWithdrawal marks a withdrawal as complete after EVM release
func (c *Client) CompleteWithdrawal(ctx context.Context, req *CompleteWithdrawalRequest) error {
	c.logger.Info("Completing withdrawal",
		zap.String("withdrawal_event_cid", req.WithdrawalEventCid),
		zap.String("evm_tx_hash", req.EvmTxHash))

	authCtx := c.GetAuthContext(ctx)

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  c.config.BridgePackageID,
					ModuleName: "Bridge.Contracts",
					EntityName: "WithdrawalEvent",
				},
				ContractId:     req.WithdrawalEventCid,
				Choice:         "CompleteWithdrawal",
				ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: EncodeCompleteWithdrawalArgs(req.EvmTxHash)}},
			},
		},
	}

	_, err := c.commandService.SubmitAndWait(authCtx, &lapiv2.SubmitAndWaitRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: c.config.DomainID,
			CommandId:      generateUUID(),
			UserId:         "bridge-relayer",
			ActAs:          []string{c.config.RelayerParty},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to complete withdrawal: %w", err)
	}

	return nil
}
