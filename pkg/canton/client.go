package canton

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// Sentinel errors for balance-related operations
var (
	// ErrInsufficientBalance indicates the user's total balance is less than the required amount
	ErrInsufficientBalance = errors.New("insufficient balance")
	// ErrBalanceFragmented indicates the user has sufficient total balance but it's spread across
	// multiple holdings, none of which individually has enough for the transfer
	ErrBalanceFragmented = errors.New("balance fragmented across multiple holdings: consolidation required")
)

// Client is a wrapper around the Canton gRPC client
type Client struct {
	config *config.CantonConfig
	conn   *grpc.ClientConn
	logger *zap.Logger

	stateService   lapiv2.StateServiceClient
	commandService lapiv2.CommandServiceClient
	updateService  lapiv2.UpdateServiceClient

	// OAuth token cache
	tokenMu     sync.Mutex
	cachedToken string
	tokenExpiry time.Time

	jwtSubject string // Extracted from JWT token
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

	c := &Client{
		config:         config,
		conn:           conn,
		logger:         logger,
		stateService:   lapiv2.NewStateServiceClient(conn),
		commandService: lapiv2.NewCommandServiceClient(conn),
		updateService:  lapiv2.NewUpdateServiceClient(conn),
	}

	// Extract JWT subject if token is configured
	if token, err := c.loadToken(); err == nil && token != "" {
		if subject, err := extractJWTSubject(token); err == nil {
			c.jwtSubject = subject
			logger.Info("Extracted JWT subject", zap.String("subject", subject))
		} else {
			logger.Warn("Failed to extract JWT subject", zap.Error(err))
		}
	}

	return c, nil
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

// invalidateToken clears the cached token to force a refresh on next GetAuthContext call
func (c *Client) invalidateToken() {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	c.cachedToken = ""
	c.tokenExpiry = time.Time{}
}

func (c *Client) loadToken() (string, error) {
	auth := c.config.Auth
	if auth.ClientID == "" || auth.ClientSecret == "" || auth.Audience == "" || auth.TokenURL == "" {
		return "", fmt.Errorf("no auth configured: OAuth2 client credentials are required")
	}

	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	now := time.Now()
	if c.cachedToken != "" && now.Before(c.tokenExpiry) {
		return c.cachedToken, nil
	}

	payload := map[string]string{
		"client_id":     auth.ClientID,
		"client_secret": auth.ClientSecret,
		"audience":      auth.Audience,
		"grant_type":    "client_credentials",
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal OAuth token request: %w", err)
	}

	c.logger.Info("Fetching new OAuth2 access token", zap.String("token_url", auth.TokenURL))

	req, err := http.NewRequest("POST", auth.TokenURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create OAuth token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call OAuth token endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("OAuth token endpoint returned %d: %s", resp.StatusCode, string(b))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode OAuth token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("OAuth token response missing access_token")
	}

	expiry := now.Add(5 * time.Minute)
	if tokenResp.ExpiresIn > 0 {
		leeway := 60
		if tokenResp.ExpiresIn <= leeway {
			leeway = tokenResp.ExpiresIn / 2
		}
		expiry = now.Add(time.Duration(tokenResp.ExpiresIn-leeway) * time.Second)
	}

	c.cachedToken = tokenResp.AccessToken
	c.tokenExpiry = expiry

	c.logger.Info("Fetched new OAuth2 access token",
		zap.String("token_url", auth.TokenURL),
		zap.Int("expires_in", tokenResp.ExpiresIn),
	)

	return tokenResp.AccessToken, nil
}

// extractJWTSubject parses the JWT token and extracts the 'sub' claim
func extractJWTSubject(tokenString string) (string, error) {
	// Parse without validating signature (Canton handles verification)
	token, _, err := jwt.NewParser().ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return "", fmt.Errorf("failed to parse JWT: %w", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("invalid JWT claims")
	}
	sub, ok := claims["sub"].(string)
	if !ok {
		return "", fmt.Errorf("JWT missing 'sub' claim")
	}
	return sub, nil
}

// loadTLSConfig loads TLS configuration from files
// If no cert files are provided, uses system CA pool (standard TLS)
// If cert files are provided, uses mTLS (mutual TLS with client certs)
func loadTLSConfig(tlsCfg *config.TLSConfig) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,           // Skip cert verification for dev - TODO: make configurable
		NextProtos:         []string{"h2"}, // Force HTTP/2 ALPN for grpc-go 1.67+ compatibility
	}

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
			UserId:         c.jwtSubject,
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
	// Normalize fingerprint to have 0x prefix for comparison
	// since API server stores with 0x prefix but Ethereum events may not include it
	normalizedFingerprint := fingerprint
	if !strings.HasPrefix(normalizedFingerprint, "0x") {
		normalizedFingerprint = "0x" + normalizedFingerprint
	}

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
			// Compare with normalized fingerprint (both should have 0x prefix)
			mappingFingerprint := mapping.Fingerprint
			if !strings.HasPrefix(mappingFingerprint, "0x") {
				mappingFingerprint = "0x" + mappingFingerprint
			}
			if mappingFingerprint == normalizedFingerprint {
				return mapping, nil
			}
		}
	}

	return nil, fmt.Errorf("no FingerprintMapping found for fingerprint: %s", normalizedFingerprint)
}

// IsDepositProcessed checks if a deposit with the given EVM tx hash has already been processed
// It looks for existing PendingDeposit or DepositReceipt contracts with matching evmTxHash
func (c *Client) IsDepositProcessed(ctx context.Context, evmTxHash string) (bool, error) {
	authCtx := c.GetAuthContext(ctx)

	// Get current ledger end for active contracts query
	ledgerEndResp, err := c.stateService.GetLedgerEnd(authCtx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		return false, fmt.Errorf("failed to get ledger end: %w", err)
	}
	activeAtOffset := ledgerEndResp.Offset
	if activeAtOffset == 0 {
		return false, nil // Empty ledger, no deposits exist
	}

	// Query for active contracts
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
		return false, fmt.Errorf("failed to query active contracts: %w", err)
	}

	// Search for PendingDeposit or DepositReceipt with matching evmTxHash
	for {
		msg, err := resp.Recv()
		if err != nil {
			break // EOF or error
		}
		if contract := msg.GetActiveContract(); contract != nil {
			templateId := contract.CreatedEvent.TemplateId
			// Check for PendingDeposit or DepositReceipt contracts
			if templateId.ModuleName != "Common.FingerprintAuth" {
				continue
			}
			if templateId.EntityName != "PendingDeposit" && templateId.EntityName != "DepositReceipt" {
				continue
			}

			// Extract evmTxHash from contract arguments
			if contract.CreatedEvent.CreateArguments != nil {
				for _, field := range contract.CreatedEvent.CreateArguments.Fields {
					if field.Label == "evmTxHash" {
						if textVal, ok := field.Value.Sum.(*lapiv2.Value_Text); ok {
							if textVal.Text == evmTxHash {
								c.logger.Debug("Found existing deposit contract",
									zap.String("evm_tx_hash", evmTxHash),
									zap.String("contract_type", templateId.EntityName),
									zap.String("contract_id", contract.CreatedEvent.ContractId))
								return true, nil
							}
						}
					}
				}
			}
		}
	}

	return false, nil
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
			UserId:         c.jwtSubject,
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
			UserId:         c.jwtSubject,
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
			UserId:         c.jwtSubject,
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

	// WithdrawalEvent is in bridge-core package (CorePackageID), not bridge-wayfinder
	corePackageID := c.config.CorePackageID
	if corePackageID == "" {
		corePackageID = c.config.BridgePackageID // fallback for backwards compatibility
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  corePackageID,
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
			UserId:         c.jwtSubject,
			ActAs:          []string{c.config.RelayerParty},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to complete withdrawal: %w", err)
	}

	return nil
}

// =============================================================================
// ERC-20 API SERVER METHODS
// =============================================================================
//
// NOTE: The functions below (GetUserBalance, GetTotalSupply, findHoldingForTransfer,
// GetAllCIP56Holdings) use GetActiveContracts with wildcard filters. Canton Ledger
// API v2 does not support server-side template filtering, so we fetch all contracts
// visible to the relayer party and filter client-side by template ID. This approach
// may not scale well for large contract volumes. The PostgreSQL balance cache in
// the API server mitigates this for read-heavy workloads. For large-scale deployments,
// consider implementing a Canton-side indexer or upgrading when template filtering
// becomes available in the API.
// =============================================================================

// CIP56Holding represents a token holding contract
type CIP56Holding struct {
	ContractID string
	Issuer     string
	Owner      string
	Amount     string
	TokenID    string
}

// GetUserBalance gets the total CIP56Holding balance for a user by fingerprint
func (c *Client) GetUserBalance(ctx context.Context, fingerprint string) (string, error) {
	c.logger.Debug("Getting user balance", zap.String("fingerprint", fingerprint))

	// First, find the FingerprintMapping to get the user's party
	mapping, err := c.GetFingerprintMapping(ctx, fingerprint)
	if err != nil {
		return "0", fmt.Errorf("failed to find user: %w", err)
	}

	// Now find all CIP56Holding contracts owned by this party
	authCtx := c.GetAuthContext(ctx)

	ledgerEndResp, err := c.stateService.GetLedgerEnd(authCtx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		return "0", fmt.Errorf("failed to get ledger end: %w", err)
	}
	activeAtOffset := ledgerEndResp.Offset
	if activeAtOffset == 0 {
		return "0", nil
	}

	// Query for CIP56Holding contracts
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
		return "0", fmt.Errorf("failed to query holdings: %w", err)
	}

	// Sum up all holdings for this user
	totalBalance := "0"
	for {
		msg, err := resp.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			templateId := contract.CreatedEvent.TemplateId
			if templateId.ModuleName != "CIP56.Token" || templateId.EntityName != "CIP56Holding" {
				continue
			}

			// Check if this holding belongs to our user
			fields := recordToMapV2(contract.CreatedEvent.CreateArguments)
			owner, _ := extractPartyV2(fields["owner"])
			if owner != mapping.UserParty {
				continue
			}

			amount, _ := extractNumericV2(fields["amount"])
			if amount != "" {
				// Add to total (simple string addition via decimal library)
				total, _ := addDecimalStrings(totalBalance, amount)
				totalBalance = total
			}
		}
	}

	return totalBalance, nil
}

// GetTotalSupply gets the total supply of CIP56 tokens
func (c *Client) GetTotalSupply(ctx context.Context) (string, error) {
	c.logger.Debug("Getting total token supply")

	authCtx := c.GetAuthContext(ctx)

	ledgerEndResp, err := c.stateService.GetLedgerEnd(authCtx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		return "0", fmt.Errorf("failed to get ledger end: %w", err)
	}
	activeAtOffset := ledgerEndResp.Offset
	if activeAtOffset == 0 {
		return "0", nil
	}

	// Query for all CIP56Holding contracts
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
		return "0", fmt.Errorf("failed to query holdings: %w", err)
	}

	// Sum up all holdings
	totalSupply := "0"
	for {
		msg, err := resp.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			templateId := contract.CreatedEvent.TemplateId
			if templateId.ModuleName != "CIP56.Token" || templateId.EntityName != "CIP56Holding" {
				continue
			}

			fields := recordToMapV2(contract.CreatedEvent.CreateArguments)
			amount, _ := extractNumericV2(fields["amount"])
			if amount != "" {
				total, _ := addDecimalStrings(totalSupply, amount)
				totalSupply = total
			}
		}
	}

	return totalSupply, nil
}

// TransferRequest represents a request to transfer tokens between users
type TransferRequest struct {
	FromFingerprint string
	ToFingerprint   string
	Amount          string
}

// Transfer performs a token transfer using Burn + Mint via CIP56Manager
// Since CIP56Holding.Transfer requires the owner to be the controller,
// and in our issuer-centric model users don't have Canton keys,
// we use the issuer's CIP56Manager to burn from sender and mint to recipient.
func (c *Client) Transfer(ctx context.Context, req *TransferRequest) error {
	c.logger.Info("Executing transfer",
		zap.String("from_fingerprint", req.FromFingerprint),
		zap.String("to_fingerprint", req.ToFingerprint),
		zap.String("amount", req.Amount))

	// Get the sender's mapping
	fromMapping, err := c.GetFingerprintMapping(ctx, req.FromFingerprint)
	if err != nil {
		return fmt.Errorf("sender not found: %w", err)
	}

	// Get the recipient's mapping
	toMapping, err := c.GetFingerprintMapping(ctx, req.ToFingerprint)
	if err != nil {
		return fmt.Errorf("recipient not found: %w", err)
	}

	// Find the sender's holding with sufficient balance
	holdingCid, err := c.findHoldingForTransfer(ctx, fromMapping.UserParty, req.Amount)
	if err != nil {
		return fmt.Errorf("insufficient balance: %w", err)
	}

	// Get the CIP56Manager from WayfinderBridgeConfig
	tokenManagerCid, err := c.getTokenManagerCid(ctx)
	if err != nil {
		return fmt.Errorf("failed to get token manager: %w", err)
	}

	authCtx := c.GetAuthContext(ctx)

	// Step 1: Burn tokens from sender's holding
	burnCmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  c.config.CIP56PackageID,
					ModuleName: "CIP56.Token",
					EntityName: "CIP56Manager",
				},
				ContractId: tokenManagerCid,
				Choice:     "Burn",
				ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: &lapiv2.Record{
					Fields: []*lapiv2.RecordField{
						{Label: "holdingCid", Value: ContractIdValue(holdingCid)},
						{Label: "amount", Value: NumericValue(req.Amount)},
					},
				}}},
			},
		},
	}

	// Step 2: Mint tokens to recipient
	mintCmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  c.config.CIP56PackageID,
					ModuleName: "CIP56.Token",
					EntityName: "CIP56Manager",
				},
				ContractId: tokenManagerCid,
				Choice:     "Mint",
				ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: &lapiv2.Record{
					Fields: []*lapiv2.RecordField{
						{Label: "to", Value: PartyValue(toMapping.UserParty)},
						{Label: "amount", Value: NumericValue(req.Amount)},
					},
				}}},
			},
		},
	}

	// Submit both commands atomically
	_, err = c.commandService.SubmitAndWait(authCtx, &lapiv2.SubmitAndWaitRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: c.config.DomainID,
			CommandId:      generateUUID(),
			UserId:         c.jwtSubject,
			ActAs:          []string{c.config.RelayerParty},
			Commands:       []*lapiv2.Command{burnCmd, mintCmd},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to execute transfer: %w", err)
	}

	c.logger.Info("Transfer completed",
		zap.String("from", fromMapping.UserParty),
		zap.String("to", toMapping.UserParty),
		zap.String("amount", req.Amount))

	return nil
}

// getTokenManagerCid retrieves the CIP56Manager contract ID from WayfinderBridgeConfig
func (c *Client) getTokenManagerCid(ctx context.Context) (string, error) {
	authCtx := c.GetAuthContext(ctx)

	ledgerEndResp, err := c.stateService.GetLedgerEnd(authCtx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		return "", fmt.Errorf("failed to get ledger end: %w", err)
	}

	resp, err := c.stateService.GetActiveContracts(authCtx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: ledgerEndResp.Offset,
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
		return "", fmt.Errorf("failed to query contracts: %w", err)
	}

	for {
		msg, err := resp.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			templateId := contract.CreatedEvent.TemplateId
			if templateId.ModuleName == "Wayfinder.Bridge" && templateId.EntityName == "WayfinderBridgeConfig" {
				fields := recordToMapV2(contract.CreatedEvent.CreateArguments)
				if tokenManagerCid, ok := extractContractIdV2(fields["tokenManagerCid"]); ok {
					return tokenManagerCid, nil
				}
			}
		}
	}

	return "", fmt.Errorf("no WayfinderBridgeConfig found")
}

// findHoldingForTransfer finds a CIP56Holding contract with sufficient balance.
// Returns structured errors to distinguish between insufficient total balance
// and fragmented balance across multiple holdings.
func (c *Client) findHoldingForTransfer(ctx context.Context, ownerParty, requiredAmount string) (string, error) {
	authCtx := c.GetAuthContext(ctx)

	ledgerEndResp, err := c.stateService.GetLedgerEnd(authCtx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		return "", fmt.Errorf("failed to get ledger end: %w", err)
	}
	activeAtOffset := ledgerEndResp.Offset
	if activeAtOffset == 0 {
		return "", fmt.Errorf("%w: no holdings exist", ErrInsufficientBalance)
	}

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
		return "", fmt.Errorf("failed to query holdings: %w", err)
	}

	// Track total balance and individual holdings to provide better error messages
	var totalBalance string = "0"
	var holdingCount int

	// Find a holding with sufficient balance
	for {
		msg, err := resp.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			templateId := contract.CreatedEvent.TemplateId
			if templateId.ModuleName != "CIP56.Token" || templateId.EntityName != "CIP56Holding" {
				continue
			}

			fields := recordToMapV2(contract.CreatedEvent.CreateArguments)
			owner, _ := extractPartyV2(fields["owner"])
			if owner != ownerParty {
				continue
			}

			amount, _ := extractNumericV2(fields["amount"])
			holdingCount++
			totalBalance, _ = addDecimalStrings(totalBalance, amount)

			// Check if this single holding has enough
			if compareDecimalStrings(amount, requiredAmount) >= 0 {
				return contract.CreatedEvent.ContractId, nil
			}
		}
	}

	// No single holding was sufficient - determine why
	if holdingCount == 0 {
		return "", fmt.Errorf("%w: no holdings found for owner", ErrInsufficientBalance)
	}

	// Check if total balance across all holdings is sufficient
	if compareDecimalStrings(totalBalance, requiredAmount) >= 0 {
		// User has enough total balance but it's fragmented
		return "", fmt.Errorf("%w: total balance %s across %d holdings, need %s in single holding",
			ErrBalanceFragmented, totalBalance, holdingCount, requiredAmount)
	}

	// Total balance is genuinely insufficient
	return "", fmt.Errorf("%w: total balance %s, need %s",
		ErrInsufficientBalance, totalBalance, requiredAmount)
}

// GetAllCIP56Holdings retrieves all CIP56Holding contracts from Canton
func (c *Client) GetAllCIP56Holdings(ctx context.Context) ([]*CIP56Holding, error) {
	authCtx := c.GetAuthContext(ctx)

	ledgerEndResp, err := c.stateService.GetLedgerEnd(authCtx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ledger end: %w", err)
	}
	activeAtOffset := ledgerEndResp.Offset
	if activeAtOffset == 0 {
		return []*CIP56Holding{}, nil
	}

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
		return nil, fmt.Errorf("failed to query holdings: %w", err)
	}

	var holdings []*CIP56Holding
	for {
		msg, err := resp.Recv()
		if err != nil {
			break // EOF or error
		}
		if contract := msg.GetActiveContract(); contract != nil {
			templateId := contract.CreatedEvent.TemplateId
			if templateId.ModuleName != "CIP56.Token" || templateId.EntityName != "CIP56Holding" {
				continue
			}

			fields := recordToMapV2(contract.CreatedEvent.CreateArguments)
			issuer, _ := extractPartyV2(fields["issuer"])
			owner, _ := extractPartyV2(fields["owner"])
			amount, _ := extractNumericV2(fields["amount"])

			holdings = append(holdings, &CIP56Holding{
				ContractID: contract.CreatedEvent.ContractId,
				Issuer:     issuer,
				Owner:      owner,
				Amount:     amount,
			})
		}
	}

	return holdings, nil
}

// =============================================================================
// BRIDGE EVENT QUERY METHODS (for reconciliation)
// =============================================================================

// contractDecoder is a function type for decoding contract records into typed events
type contractDecoder[T any] func(contractID string, record *lapiv2.Record) (*T, error)

// getActiveContractsByTemplate is a generic helper that queries active contracts
// filtered by module and entity name, then decodes them using the provided decoder.
func getActiveContractsByTemplate[T any](
	c *Client,
	ctx context.Context,
	moduleName string,
	entityName string,
	decoder contractDecoder[T],
) ([]*T, error) {
	authCtx := c.GetAuthContext(ctx)

	ledgerEndResp, err := c.stateService.GetLedgerEnd(authCtx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ledger end: %w", err)
	}
	activeAtOffset := ledgerEndResp.Offset
	if activeAtOffset == 0 {
		return []*T{}, nil
	}

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
		return nil, fmt.Errorf("failed to query %s.%s contracts: %w", moduleName, entityName, err)
	}

	var results []*T
	contractCount := 0
	for {
		msg, err := resp.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			contractCount++
			templateId := contract.CreatedEvent.TemplateId
			c.logger.Debug("Found active contract",
				zap.String("module", templateId.ModuleName),
				zap.String("entity", templateId.EntityName),
				zap.String("package_id", templateId.PackageId))

			if templateId.ModuleName != moduleName || templateId.EntityName != entityName {
				continue
			}

			decoded, err := decoder(
				contract.CreatedEvent.ContractId,
				contract.CreatedEvent.CreateArguments,
			)
			if err != nil {
				c.logger.Warn("Failed to decode contract",
					zap.String("module", moduleName),
					zap.String("entity", entityName),
					zap.Error(err))
				continue
			}
			results = append(results, decoded)
		}
	}

	c.logger.Debug("getActiveContractsByTemplate completed",
		zap.String("module", moduleName),
		zap.String("entity", entityName),
		zap.Int("total_contracts_scanned", contractCount),
		zap.Int("matching_contracts_found", len(results)))

	return results, nil
}

// GetBridgeMintEvents retrieves all BridgeMintEvent contracts from Canton
// These are created when deposits are processed and tokens are minted
func (c *Client) GetBridgeMintEvents(ctx context.Context) ([]*BridgeMintEvent, error) {
	return getActiveContractsByTemplate(c, ctx, "Bridge.Events", "BridgeMintEvent", DecodeBridgeMintEvent)
}

// GetBridgeBurnEvents retrieves all BridgeBurnEvent contracts from Canton
// These are created when withdrawals are initiated and tokens are burned
func (c *Client) GetBridgeBurnEvents(ctx context.Context) ([]*BridgeBurnEvent, error) {
	return getActiveContractsByTemplate(c, ctx, "Bridge.Events", "BridgeBurnEvent", DecodeBridgeBurnEvent)
}

