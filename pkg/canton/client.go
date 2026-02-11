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
	adminv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2/admin"
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

	stateService           lapiv2.StateServiceClient
	commandService         lapiv2.CommandServiceClient
	updateService          lapiv2.UpdateServiceClient
	partyManagementService adminv2.PartyManagementServiceClient
	userManagementService  adminv2.UserManagementServiceClient

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
		config:                 config,
		conn:                   conn,
		logger:                 logger,
		stateService:           lapiv2.NewStateServiceClient(conn),
		commandService:         lapiv2.NewCommandServiceClient(conn),
		updateService:          lapiv2.NewUpdateServiceClient(conn),
		partyManagementService: adminv2.NewPartyManagementServiceClient(conn),
		userManagementService:  adminv2.NewUserManagementServiceClient(conn),
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
// PARTY MANAGEMENT METHODS (for custodial key model)
// =============================================================================

// AllocatePartyResult contains the result of allocating a new Canton party
type AllocatePartyResult struct {
	PartyID string // The full party ID (e.g., "user_0x1234::participant123")
	IsLocal bool   // Whether the party is local to this participant
}

// AllocateParty allocates a new Canton party for a user.
// The hint is used to create a human-readable party ID prefix.
// Returns the allocated party details.
func (c *Client) AllocateParty(ctx context.Context, hint string) (*AllocatePartyResult, error) {
	c.logger.Info("Allocating new Canton party",
		zap.String("hint", hint),
		zap.String("synchronizer_id", c.config.DomainID))

	authCtx := c.GetAuthContext(ctx)

	req := &adminv2.AllocatePartyRequest{
		PartyIdHint:    hint,
		SynchronizerId: c.config.DomainID,
	}

	resp, err := c.partyManagementService.AllocateParty(authCtx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate party: %w", err)
	}

	if resp.PartyDetails == nil {
		return nil, fmt.Errorf("AllocateParty returned nil party details")
	}

	c.logger.Info("Allocated new Canton party",
		zap.String("party_id", resp.PartyDetails.Party),
		zap.Bool("is_local", resp.PartyDetails.IsLocal))

	return &AllocatePartyResult{
		PartyID: resp.PartyDetails.Party,
		IsLocal: resp.PartyDetails.IsLocal,
	}, nil
}

// ListParties returns all parties known to this participant (paginates through all results)
func (c *Client) ListParties(ctx context.Context) ([]*AllocatePartyResult, error) {
	authCtx := c.GetAuthContext(ctx)

	var results []*AllocatePartyResult
	pageToken := ""

	for {
		resp, err := c.partyManagementService.ListKnownParties(authCtx, &adminv2.ListKnownPartiesRequest{
			PageSize:  1000,
			PageToken: pageToken,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list parties: %w", err)
		}

		for _, p := range resp.PartyDetails {
			results = append(results, &AllocatePartyResult{
				PartyID: p.Party,
				IsLocal: p.IsLocal,
			})
		}

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	return results, nil
}

// GetParticipantID returns the participant ID of the connected Canton node
func (c *Client) GetParticipantID(ctx context.Context) (string, error) {
	authCtx := c.GetAuthContext(ctx)

	resp, err := c.partyManagementService.GetParticipantId(authCtx, &adminv2.GetParticipantIdRequest{})
	if err != nil {
		return "", fmt.Errorf("failed to get participant ID: %w", err)
	}

	return resp.ParticipantId, nil
}

// =============================================================================
// ISSUER-CENTRIC MODEL METHODS
// =============================================================================

// CreateFingerprintMappingDirect creates a FingerprintMapping using direct Create command
// The issuer has signatory rights on FingerprintMapping, so no bridge config is needed.
func (c *Client) CreateFingerprintMappingDirect(ctx context.Context, req *RegisterUserRequest) (string, error) {
	c.logger.Info("Creating FingerprintMapping directly",
		zap.String("user_party", req.UserParty),
		zap.String("fingerprint", req.Fingerprint))

	authCtx := c.GetAuthContext(ctx)

	// Determine the package ID for Common.FingerprintAuth
	// Try CommonPackageID first, fall back to BridgePackageID
	packageID := c.config.CommonPackageID
	if packageID == "" {
		packageID = c.config.BridgePackageID
	}
	if packageID == "" {
		return "", fmt.Errorf("no package ID configured for FingerprintMapping (set common_package_id or bridge_package_id)")
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Create{
			Create: &lapiv2.CreateCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  packageID,
					ModuleName: "Common.FingerprintAuth",
					EntityName: "FingerprintMapping",
				},
				CreateArguments: EncodeFingerprintMappingCreate(
					c.config.RelayerParty,
					req.UserParty,
					req.Fingerprint,
					req.EvmAddress,
				),
			},
		},
	}

	resp, err := c.commandService.SubmitAndWaitForTransaction(authCtx, &lapiv2.SubmitAndWaitForTransactionRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: c.config.DomainID,
			CommandId:      generateUUID(),
			UserId:         c.jwtSubject,
			ActAs:          []string{c.config.RelayerParty},
			ReadAs:         []string{req.UserParty}, // userParty is observer
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to create FingerprintMapping: %w", err)
	}

	// Extract the created FingerprintMapping contract ID from response
	if resp.Transaction != nil {
		for _, event := range resp.Transaction.Events {
			if created := event.GetCreated(); created != nil {
				templateId := created.TemplateId
				if templateId.ModuleName == "Common.FingerprintAuth" && templateId.EntityName == "FingerprintMapping" {
					c.logger.Info("FingerprintMapping created successfully",
						zap.String("contract_id", created.ContractId),
						zap.String("user_party", req.UserParty))
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

	// Build filter for FingerprintMapping contracts using TemplateFilter
	resp, err := c.stateService.GetActiveContracts(authCtx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: activeAtOffset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				c.config.RelayerParty: {
					Cumulative: templateFilterWithFallback(c.config.CommonPackageID, "Common.FingerprintAuth", "FingerprintMapping"),
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

	// Query for PendingDeposit and DepositReceipt contracts using TemplateFilter
	var depositFilters []*lapiv2.CumulativeFilter
	if c.config.CommonPackageID != "" {
		depositFilters = []*lapiv2.CumulativeFilter{
			templateFilter(c.config.CommonPackageID, "Common.FingerprintAuth", "PendingDeposit"),
			templateFilter(c.config.CommonPackageID, "Common.FingerprintAuth", "DepositReceipt"),
		}
	} else {
		depositFilters = []*lapiv2.CumulativeFilter{
			{IdentifierFilter: &lapiv2.CumulativeFilter_WildcardFilter{WildcardFilter: &lapiv2.WildcardFilter{}}},
		}
	}
	resp, err := c.stateService.GetActiveContracts(authCtx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: activeAtOffset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				c.config.RelayerParty: {
					Cumulative: depositFilters,
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
			// Extract evmTxHash from contract arguments
			if contract.CreatedEvent.CreateArguments != nil {
				for _, field := range contract.CreatedEvent.CreateArguments.Fields {
					if field.Label == "evmTxHash" {
						if textVal, ok := field.Value.Sum.(*lapiv2.Value_Text); ok {
							if textVal.Text == evmTxHash {
							c.logger.Debug("Found existing deposit contract",
								zap.String("evm_tx_hash", evmTxHash),
								zap.String("contract_type", contract.CreatedEvent.TemplateId.EntityName),
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

// cip56HoldingFilter returns a TemplateFilter for CIP56.Token.CIP56Holding contracts.
// Falls back to WildcardFilter if cip56_package_id is not configured.
func (c *Client) cip56HoldingFilter() []*lapiv2.CumulativeFilter {
	return templateFilterWithFallback(c.config.CIP56PackageID, "CIP56.Token", "CIP56Holding")
}

// tokenConfigFilter returns a TemplateFilter for CIP56.Config.TokenConfig contracts.
// Falls back to WildcardFilter if cip56_package_id is not configured.
func (c *Client) tokenConfigFilter() []*lapiv2.CumulativeFilter {
	return templateFilterWithFallback(c.config.CIP56PackageID, "CIP56.Config", "TokenConfig")
}

// templateFilterWithFallback returns a TemplateFilter if packageID is set,
// or a WildcardFilter as fallback. This prevents silent failures when a
// package ID is not configured -- the wildcard will still return results
// (albeit less efficiently) rather than matching nothing.
func templateFilterWithFallback(packageID, moduleName, entityName string) []*lapiv2.CumulativeFilter {
	if packageID == "" {
		return []*lapiv2.CumulativeFilter{
			{
				IdentifierFilter: &lapiv2.CumulativeFilter_WildcardFilter{
					WildcardFilter: &lapiv2.WildcardFilter{},
				},
			},
		}
	}
	return []*lapiv2.CumulativeFilter{templateFilter(packageID, moduleName, entityName)}
}

// templateFilter builds a CumulativeFilter with a TemplateFilter for server-side contract filtering.
func templateFilter(packageID, moduleName, entityName string) *lapiv2.CumulativeFilter {
	return &lapiv2.CumulativeFilter{
		IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
			TemplateFilter: &lapiv2.TemplateFilter{
				TemplateId: &lapiv2.Identifier{
					PackageId:  packageID,
					ModuleName: moduleName,
					EntityName: entityName,
				},
			},
		},
	}
}


// CIP56Holding represents a token holding contract
type CIP56Holding struct {
	ContractID string
	Issuer     string
	Owner      string
	Amount     string
	TokenID    string
	Symbol     string // Token symbol from metadata ("DEMO" or "PROMPT")
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

	// Query for CIP56Holding contracts using TemplateFilter
	resp, err := c.stateService.GetActiveContracts(authCtx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: activeAtOffset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				c.config.RelayerParty: {
					Cumulative: c.cip56HoldingFilter(),
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

	// Query for all CIP56Holding contracts using TemplateFilter
	resp, err := c.stateService.GetActiveContracts(authCtx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: activeAtOffset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				c.config.RelayerParty: {
					Cumulative: c.cip56HoldingFilter(),
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

// getTokenConfigCidFromBridge retrieves the TokenConfig contract ID from WayfinderBridgeConfig
func (c *Client) getTokenConfigCidFromBridge(ctx context.Context) (string, error) {
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
					Cumulative: templateFilterWithFallback(c.config.BridgePackageID, "Wayfinder.Bridge", "WayfinderBridgeConfig"),
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
			fields := recordToMapV2(contract.CreatedEvent.CreateArguments)
			if tokenConfigCid, ok := extractContractIdV2(fields["tokenConfigCid"]); ok {
				return tokenConfigCid, nil
			}
		}
	}

	return "", fmt.Errorf("no WayfinderBridgeConfig found")
}

// findHoldingForTransfer finds a CIP56Holding contract with sufficient balance for a given token.
// Filters by owner party and token symbol from metadata.
// Returns structured errors to distinguish between insufficient total balance
// and fragmented balance across multiple holdings.
func (c *Client) findHoldingForTransfer(ctx context.Context, ownerParty, requiredAmount, tokenSymbol string) (string, error) {
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
					Cumulative: c.cip56HoldingFilter(),
				},
			},
			Verbose: true,
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to query holdings: %w", err)
	}

	var totalBalance string = "0"
	var holdingCount int

	for {
		msg, err := resp.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			fields := recordToMapV2(contract.CreatedEvent.CreateArguments)
			owner, _ := extractPartyV2(fields["owner"])
			if owner != ownerParty {
				continue
			}

			// Filter by token symbol from metadata
			symbol := extractHoldingSymbol(fields)
			if symbol != tokenSymbol {
				continue
			}

			amount, _ := extractNumericV2(fields["amount"])
			holdingCount++
			totalBalance, _ = addDecimalStrings(totalBalance, amount)

			if compareDecimalStrings(amount, requiredAmount) >= 0 {
				return contract.CreatedEvent.ContractId, nil
			}
		}
	}

	if holdingCount == 0 {
		return "", fmt.Errorf("%w: no %s holdings found for owner", ErrInsufficientBalance, tokenSymbol)
	}

	if compareDecimalStrings(totalBalance, requiredAmount) >= 0 {
		return "", fmt.Errorf("%w: total %s balance %s across %d holdings, need %s in single holding",
			ErrBalanceFragmented, tokenSymbol, totalBalance, holdingCount, requiredAmount)
	}

	return "", fmt.Errorf("%w: total %s balance %s, need %s",
		ErrInsufficientBalance, tokenSymbol, totalBalance, requiredAmount)
}

// findRecipientHolding finds an existing CIP56Holding for the recipient party and token symbol.
// Returns the contract ID if found, empty string if none exists.
func (c *Client) findRecipientHolding(ctx context.Context, recipientParty, tokenSymbol string) (string, error) {
	authCtx := c.GetAuthContext(ctx)

	ledgerEndResp, err := c.stateService.GetLedgerEnd(authCtx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		return "", fmt.Errorf("failed to get ledger end: %w", err)
	}
	if ledgerEndResp.Offset == 0 {
		return "", nil
	}

	resp, err := c.stateService.GetActiveContracts(authCtx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: ledgerEndResp.Offset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				c.config.RelayerParty: {
					Cumulative: c.cip56HoldingFilter(),
				},
			},
			Verbose: true,
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to query holdings: %w", err)
	}

	for {
		msg, err := resp.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			fields := recordToMapV2(contract.CreatedEvent.CreateArguments)
			owner, _ := extractPartyV2(fields["owner"])
			if owner != recipientParty {
				continue
			}
			symbol := extractHoldingSymbol(fields)
			if symbol == tokenSymbol {
				return contract.CreatedEvent.ContractId, nil
			}
		}
	}

	return "", nil // No existing holding found
}

// extractHoldingSymbol extracts the token symbol from a CIP56Holding's metadata
func extractHoldingSymbol(fields map[string]*lapiv2.Value) string {
	if metaVal, ok := fields["meta"]; ok {
		if metaRecord := metaVal.GetRecord(); metaRecord != nil {
			for _, metaField := range metaRecord.Fields {
				if metaField.Label == "symbol" {
					if s := metaField.Value.GetText(); s != "" {
						return s
					}
				}
			}
		}
	}
	return ""
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
					Cumulative: c.cip56HoldingFilter(),
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
			fields := recordToMapV2(contract.CreatedEvent.CreateArguments)
			issuer, _ := extractPartyV2(fields["issuer"])
			owner, _ := extractPartyV2(fields["owner"])
			amount, _ := extractNumericV2(fields["amount"])

			// Extract symbol from metadata
			var symbol string
			if metaVal, ok := fields["meta"]; ok {
				if metaRecord := metaVal.GetRecord(); metaRecord != nil {
					for _, metaField := range metaRecord.Fields {
						if metaField.Label == "symbol" {
							if s := metaField.Value.GetText(); s != "" {
								symbol = s
							}
						}
					}
				}
			}

			holdings = append(holdings, &CIP56Holding{
				ContractID: contract.CreatedEvent.ContractId,
				Issuer:     issuer,
				Owner:      owner,
				Amount:     amount,
				Symbol:     symbol,
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

	// NOTE: This generic helper uses WildcardFilter because it doesn't know
	// which package the module/entity belongs to. Client-side filtering is
	// applied below. Consider adding a packageID parameter if performance matters.
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

// GetMintEvents retrieves all MintEvent contracts from Canton (CIP56.Events.MintEvent)
// These are created when tokens are minted (native or bridge deposits)
func (c *Client) GetMintEvents(ctx context.Context) ([]*MintEvent, error) {
	return getActiveContractsByTemplate(c, ctx, "CIP56.Events", "MintEvent", DecodeMintEvent)
}

// GetBurnEvents retrieves all BurnEvent contracts from Canton (CIP56.Events.BurnEvent)
// These are created when tokens are burned (native or bridge withdrawals)
func (c *Client) GetBurnEvents(ctx context.Context) ([]*BurnEvent, error) {
	return getActiveContractsByTemplate(c, ctx, "CIP56.Events", "BurnEvent", DecodeBurnEvent)
}

// =============================================================================
// UNIFIED TOKEN METHODS (via CIP56.Config.TokenConfig)
// =============================================================================

// GetTokenConfig finds an active TokenConfig contract by matching token symbol in metadata.
// Returns the contract ID of the matching TokenConfig.
func (c *Client) GetTokenConfig(ctx context.Context, tokenSymbol string) (string, error) {
	authCtx := c.GetAuthContext(ctx)

	ledgerEndResp, err := c.stateService.GetLedgerEnd(authCtx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		return "", fmt.Errorf("failed to get ledger end: %w", err)
	}
	if ledgerEndResp.Offset == 0 {
		return "", fmt.Errorf("ledger is empty, no contracts exist")
	}

	resp, err := c.stateService.GetActiveContracts(authCtx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: ledgerEndResp.Offset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: map[string]*lapiv2.Filters{
				c.config.RelayerParty: {
					Cumulative: c.tokenConfigFilter(),
				},
			},
			Verbose: true,
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to search for TokenConfig: %w", err)
	}

	for {
		msg, err := resp.Recv()
		if err != nil {
			break
		}
		if contract := msg.GetActiveContract(); contract != nil {
			// Check if meta.symbol matches
			fields := recordToMapV2(contract.CreatedEvent.CreateArguments)
			if metaVal, ok := fields["meta"]; ok {
				if metaRecord := metaVal.GetRecord(); metaRecord != nil {
					for _, metaField := range metaRecord.Fields {
						if metaField.Label == "symbol" {
							if s := metaField.Value.GetText(); s == tokenSymbol {
								return contract.CreatedEvent.ContractId, nil
							}
						}
					}
				}
			}
		}
	}

	return "", fmt.Errorf("no active TokenConfig found for symbol %s", tokenSymbol)
}

// TokenMintRequest represents a request to mint tokens via TokenConfig.IssuerMint
type TokenMintRequest struct {
	RecipientParty  string // Canton party to mint to
	Amount          string // Amount to mint (decimal string)
	UserFingerprint string // EVM fingerprint of the recipient
	TokenSymbol     string // Token symbol (e.g., "DEMO", "PROMPT")
	ConfigCid       string // TokenConfig contract ID (optional, will be fetched if empty)
	EvmTxHash       string // Optional: set for bridge deposits, empty for native mints
}

// TokenMint mints tokens to a user via TokenConfig.IssuerMint
func (c *Client) TokenMint(ctx context.Context, req *TokenMintRequest) (string, error) {
	c.logger.Info("Minting tokens via TokenConfig",
		zap.String("recipient", req.RecipientParty),
		zap.String("amount", req.Amount),
		zap.String("symbol", req.TokenSymbol),
		zap.String("fingerprint", req.UserFingerprint))

	configCid := req.ConfigCid
	if configCid == "" {
		var err error
		configCid, err = c.GetTokenConfig(ctx, req.TokenSymbol)
		if err != nil {
			return "", fmt.Errorf("failed to get TokenConfig for %s: %w", req.TokenSymbol, err)
		}
	}

	authCtx := c.GetAuthContext(ctx)

	// Build IssuerMint choice arguments (includes optional evmTxHash)
	evmTxHashValue := NoneValue()
	if req.EvmTxHash != "" {
		evmTxHashValue = OptionalValue(TextValue(req.EvmTxHash))
	}

	mintArgs := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "recipient", Value: PartyValue(req.RecipientParty)},
			{Label: "amount", Value: NumericValue(req.Amount)},
			{Label: "eventTime", Value: TimestampValue(time.Now())},
			{Label: "userFingerprint", Value: TextValue(req.UserFingerprint)},
			{Label: "evmTxHash", Value: evmTxHashValue},
		},
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  c.config.CIP56PackageID,
					ModuleName: "CIP56.Config",
					EntityName: "TokenConfig",
				},
				ContractId:     configCid,
				Choice:         "IssuerMint",
				ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: mintArgs}},
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
		return "", fmt.Errorf("failed to mint %s tokens: %w", req.TokenSymbol, err)
	}

	if resp.Transaction != nil {
		for _, event := range resp.Transaction.Events {
			if created := event.GetCreated(); created != nil {
				templateId := created.TemplateId
				if templateId.ModuleName == "CIP56.Token" && templateId.EntityName == "CIP56Holding" {
					c.logger.Info("Tokens minted successfully",
						zap.String("holding_cid", created.ContractId),
						zap.String("recipient", req.RecipientParty),
						zap.String("amount", req.Amount),
						zap.String("symbol", req.TokenSymbol))
					return created.ContractId, nil
				}
			}
		}
	}

	return "", fmt.Errorf("CIP56Holding contract not found in mint response")
}

// TokenBurn burns tokens via TokenConfig.IssuerBurn
// Used for cleanup/reset operations and bridge withdrawals
func (c *Client) TokenBurn(ctx context.Context, holdingCid, amount, tokenSymbol, userFingerprint, evmDestination string) error {
	c.logger.Info("Burning tokens via TokenConfig",
		zap.String("holding_cid", holdingCid),
		zap.String("amount", amount),
		zap.String("symbol", tokenSymbol))

	configCid, err := c.GetTokenConfig(ctx, tokenSymbol)
	if err != nil {
		return fmt.Errorf("failed to get TokenConfig for %s: %w", tokenSymbol, err)
	}

	authCtx := c.GetAuthContext(ctx)

	evmDestValue := NoneValue()
	if evmDestination != "" {
		evmDestValue = OptionalValue(TextValue(evmDestination))
	}

	burnArgs := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "holdingCid", Value: ContractIdValue(holdingCid)},
			{Label: "amount", Value: NumericValue(amount)},
			{Label: "eventTime", Value: TimestampValue(time.Now())},
			{Label: "userFingerprint", Value: TextValue(userFingerprint)},
			{Label: "evmDestination", Value: evmDestValue},
		},
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  c.config.CIP56PackageID,
					ModuleName: "CIP56.Config",
					EntityName: "TokenConfig",
				},
				ContractId:     configCid,
				Choice:         "IssuerBurn",
				ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: burnArgs}},
			},
		},
	}

	_, err = c.commandService.SubmitAndWait(authCtx, &lapiv2.SubmitAndWaitRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: c.config.DomainID,
			CommandId:      generateUUID(),
			UserId:         c.jwtSubject,
			ActAs:          []string{c.config.RelayerParty},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to burn %s holding: %w", tokenSymbol, err)
	}

	c.logger.Info("Tokens burned successfully",
		zap.String("holding_cid", holdingCid),
		zap.String("amount", amount),
		zap.String("symbol", tokenSymbol))

	return nil
}

// =============================================================================
// CUSTODIAL TRANSFER METHODS (for user-owned holdings)
// =============================================================================

// TransferAsUserRequest represents a request to transfer tokens using the owner-controlled Transfer choice
type TransferAsUserRequest struct {
	FromPartyID            string // User's Canton party ID (must be the holding owner)
	ToPartyID              string // Recipient's Canton party ID
	HoldingCID             string // The CIP56Holding contract ID to transfer from
	Amount                 string // Amount to transfer
	TokenSymbol            string // "PROMPT" or "DEMO"
	FromFingerprint        string // For logging/audit
	ToFingerprint          string // For logging/audit
	ExistingRecipientHolding string // Existing recipient CIP56Holding CID (for merge), empty if none
}

// TransferAsUser performs a token transfer using the CIP56Holding.Transfer choice.
// This is the owner-controlled transfer for user-owned holdings (custodial mode).
// Passes existingRecipientHolding to prevent fragmentation.
func (c *Client) TransferAsUser(ctx context.Context, req *TransferAsUserRequest) error {
	c.logger.Info("Executing transfer as user (owner-controlled)",
		zap.String("from_party", req.FromPartyID),
		zap.String("to_party", req.ToPartyID),
		zap.String("holding_cid", req.HoldingCID),
		zap.String("amount", req.Amount),
		zap.String("token", req.TokenSymbol),
		zap.String("existing_recipient_holding", req.ExistingRecipientHolding))

	authCtx := c.GetAuthContext(ctx)

	// Build existingRecipientHolding as Optional (ContractId CIP56Holding)
	existingHoldingValue := NoneValue()
	if req.ExistingRecipientHolding != "" {
		existingHoldingValue = OptionalValue(ContractIdValue(req.ExistingRecipientHolding))
	}

	// CIP56Holding.Transfer takes: to, value, existingRecipientHolding, complianceRulesCid, complianceProofCid
	transferArgs := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "to", Value: PartyValue(req.ToPartyID)},
			{Label: "value", Value: NumericValue(req.Amount)},
			{Label: "existingRecipientHolding", Value: existingHoldingValue},
			{Label: "complianceRulesCid", Value: NoneValue()},
			{Label: "complianceProofCid", Value: NoneValue()},
		},
	}

	cmd := &lapiv2.Command{
		Command: &lapiv2.Command_Exercise{
			Exercise: &lapiv2.ExerciseCommand{
				TemplateId: &lapiv2.Identifier{
					PackageId:  c.config.CIP56PackageID,
					ModuleName: "CIP56.Token",
					EntityName: "CIP56Holding",
				},
				ContractId:     req.HoldingCID,
				Choice:         "Transfer",
				ChoiceArgument: &lapiv2.Value{Sum: &lapiv2.Value_Record{Record: transferArgs}},
			},
		},
	}

	// Act as the user's party (custodial) with readAs relayer (to see recipient holdings)
	_, err := c.commandService.SubmitAndWait(authCtx, &lapiv2.SubmitAndWaitRequest{
		Commands: &lapiv2.Commands{
			SynchronizerId: c.config.DomainID,
			CommandId:      generateUUID(),
			UserId:         c.jwtSubject,
			ActAs:          []string{req.FromPartyID},
			ReadAs:         []string{c.config.RelayerParty},
			Commands:       []*lapiv2.Command{cmd},
		},
	})
	if err != nil {
		return fmt.Errorf("transfer failed: %w", err)
	}

	c.logger.Info("Transfer completed (user-owned)",
		zap.String("from", req.FromFingerprint),
		zap.String("to", req.ToFingerprint),
		zap.String("amount", req.Amount),
		zap.String("token", req.TokenSymbol))

	return nil
}

// TransferAsUserByFingerprint performs a token transfer looking up users by fingerprint.
// Resolves fingerprints to party IDs, finds the holding, looks up recipient holding for merge.
func (c *Client) TransferAsUserByFingerprint(ctx context.Context, fromFingerprint, toFingerprint, amount, tokenSymbol string) error {
	c.logger.Info("Executing transfer by fingerprint",
		zap.String("from_fingerprint", fromFingerprint),
		zap.String("to_fingerprint", toFingerprint),
		zap.String("amount", amount),
		zap.String("token", tokenSymbol))

	fromMapping, err := c.GetFingerprintMapping(ctx, fromFingerprint)
	if err != nil {
		return fmt.Errorf("sender not found: %w", err)
	}

	toMapping, err := c.GetFingerprintMapping(ctx, toFingerprint)
	if err != nil {
		return fmt.Errorf("recipient not found: %w", err)
	}

	// Find sender's holding with sufficient balance (unified, symbol-aware)
	holdingCID, err := c.findHoldingForTransfer(ctx, fromMapping.UserParty, amount, tokenSymbol)
	if err != nil {
		return fmt.Errorf("insufficient balance: %w", err)
	}

	// Find recipient's existing holding for merge (prevents fragmentation)
	recipientHolding, err := c.findRecipientHolding(ctx, toMapping.UserParty, tokenSymbol)
	if err != nil {
		c.logger.Warn("Failed to find recipient holding for merge, proceeding without", zap.Error(err))
	}

	return c.TransferAsUser(ctx, &TransferAsUserRequest{
		FromPartyID:              fromMapping.UserParty,
		ToPartyID:                toMapping.UserParty,
		HoldingCID:               holdingCID,
		Amount:                   amount,
		TokenSymbol:              tokenSymbol,
		FromFingerprint:          fromFingerprint,
		ToFingerprint:            toFingerprint,
		ExistingRecipientHolding: recipientHolding,
	})
}

// TransferByPartyID performs a token transfer using party IDs directly.
// Finds the sender's holding and recipient's existing holding for merge.
func (c *Client) TransferByPartyID(ctx context.Context, fromParty, toParty, amount, tokenSymbol string) error {
	c.logger.Info("Executing transfer by party ID",
		zap.String("from_party", fromParty),
		zap.String("to_party", toParty),
		zap.String("amount", amount),
		zap.String("token", tokenSymbol))

	holdingCID, err := c.findHoldingForTransfer(ctx, fromParty, amount, tokenSymbol)
	if err != nil {
		return fmt.Errorf("insufficient balance or holding not found: %w", err)
	}

	recipientHolding, err := c.findRecipientHolding(ctx, toParty, tokenSymbol)
	if err != nil {
		c.logger.Warn("Failed to find recipient holding for merge, proceeding without", zap.Error(err))
	}

	return c.TransferAsUser(ctx, &TransferAsUserRequest{
		FromPartyID:              fromParty,
		ToPartyID:                toParty,
		HoldingCID:               holdingCID,
		Amount:                   amount,
		TokenSymbol:              tokenSymbol,
		FromFingerprint:          "",
		ToFingerprint:            "",
		ExistingRecipientHolding: recipientHolding,
	})
}

// GrantCanActAs grants the OAuth client (this API server) CanActAs rights for the given party.
// This is called during user registration to enable the custodial model where the API server
// can submit transactions on behalf of the user's party.
// This maintains user ownership (the user's party owns the holdings) while allowing the
// API server to act as a trusted intermediary after the user authorizes via MetaMask signature.
func (c *Client) GrantCanActAs(ctx context.Context, partyID string) error {
	if c.jwtSubject == "" {
		return fmt.Errorf("JWT subject not available - cannot determine user ID for rights grant")
	}

	c.logger.Info("Granting CanActAs rights to OAuth client",
		zap.String("party_id", partyID),
		zap.String("oauth_user", c.jwtSubject))

	authCtx := c.GetAuthContext(ctx)

	// Create the CanActAs right for this party
	right := &adminv2.Right{
		Kind: &adminv2.Right_CanActAs_{
			CanActAs: &adminv2.Right_CanActAs{
				Party: partyID,
			},
		},
	}

	// Grant the right to the OAuth client user
	resp, err := c.userManagementService.GrantUserRights(authCtx, &adminv2.GrantUserRightsRequest{
		UserId: c.jwtSubject,
		Rights: []*adminv2.Right{right},
	})
	if err != nil {
		// Check if it's because the right already exists (not an error)
		if strings.Contains(err.Error(), "already") {
			c.logger.Info("CanActAs right already exists",
				zap.String("party_id", partyID),
				zap.String("oauth_user", c.jwtSubject))
			return nil
		}
		return fmt.Errorf("failed to grant CanActAs rights: %w", err)
	}

	if len(resp.NewlyGrantedRights) > 0 {
		c.logger.Info("CanActAs rights granted successfully",
			zap.String("party_id", partyID),
			zap.String("oauth_user", c.jwtSubject),
			zap.Int("newly_granted", len(resp.NewlyGrantedRights)))
	} else {
		c.logger.Info("CanActAs right already existed",
			zap.String("party_id", partyID),
			zap.String("oauth_user", c.jwtSubject))
	}

	return nil
}

// GetJWTSubject returns the OAuth client's user ID (JWT subject claim)
func (c *Client) GetJWTSubject() string {
	return c.jwtSubject
}
