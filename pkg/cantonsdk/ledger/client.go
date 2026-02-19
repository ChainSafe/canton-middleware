// Package ledger implements the low-level Canton Ledger API client.
//
// It manages gRPC connectivity, TLS configuration, OAuth2 authentication,
// and exposes typed access to Ledger API v2 services including State,
// Command, Update, and Admin services.
package ledger

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	adminv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2/admin"
	interactivev2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2/interactive"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// Ledger defines the public Canton ledger client interface.
//
// It provides authenticated access to Ledger API services and common
// helper operations such as querying the ledger end and retrieving
// active contracts by template.
type Ledger interface {
	// AuthContext attaches authorization metadata to the given context.
	AuthContext(ctx context.Context) context.Context

	// InvalidateToken invalidates the token.
	InvalidateToken()

	// JWTSubject returns the JWT subject ("sub") from the current access token.
	JWTSubject(ctx context.Context) (string, error)

	// State returns the Ledger API StateService client.
	State() lapiv2.StateServiceClient

	// Command returns the Ledger API CommandService client.
	Command() lapiv2.CommandServiceClient

	// Update returns the Ledger API UpdateService client.
	Update() lapiv2.UpdateServiceClient

	// PartyAdmin returns the PartyManagementService client.
	PartyAdmin() adminv2.PartyManagementServiceClient

	// UserAdmin returns the UserManagementService client.
	UserAdmin() adminv2.UserManagementServiceClient

	// Interactive returns the InteractiveSubmissionService client
	// for prepare/sign/execute transaction flows with external parties.
	Interactive() interactivev2.InteractiveSubmissionServiceClient

	// GetLedgerEnd retrieves the current absolute ledger offset.
	GetLedgerEnd(ctx context.Context) (int64, error)

	// GetActiveContractsByTemplate retrieves active contracts filtered
	// by template identifier and visible parties at the given offset.
	GetActiveContractsByTemplate(
		ctx context.Context,
		activeAtOffset int64,
		parties []string,
		templateID *lapiv2.Identifier,
	) ([]*lapiv2.CreatedEvent, error)

	// Conn returns the underlying gRPC client connection.
	Conn() *grpc.ClientConn

	// Close closes the underlying gRPC connection.
	Close() error
}

// Client is the concrete implementation of the Ledger interface.
//
// It encapsulates the gRPC connection, Ledger API service clients,
// and authentication handling.
type Client struct {
	cfg    *Config
	logger *zap.Logger

	conn *grpc.ClientConn

	state   lapiv2.StateServiceClient
	command lapiv2.CommandServiceClient
	update  lapiv2.UpdateServiceClient

	partyAdmin  adminv2.PartyManagementServiceClient
	userAdmin   adminv2.UserManagementServiceClient
	interactive interactivev2.InteractiveSubmissionServiceClient

	auth AuthProvider

	tokenMu     sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

// New creates a new Ledger client using the provided configuration.
func New(cfg *Config, opts ...Option) (*Client, error) {
	s := applyOptions(opts)

	dopts, err := dialOptions(cfg, s.dialOpts)
	if err != nil {
		return nil, err
	}

	conn, err := grpc.NewClient(cfg.RPCURL, dopts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Canton client: %w", err)
	}

	var ap = s.authProvider
	if ap == nil {
		ap = NewOAuthClientCredentialsProvider(cfg.Auth, s.httpClient)
	}

	s.logger.Info("Connected to Canton Network (cantonsdk)",
		zap.String("rpc_url", cfg.RPCURL),
		zap.String("ledger_id", cfg.LedgerID),
	)

	return &Client{
		cfg:         cfg,
		logger:      s.logger,
		conn:        conn,
		state:       lapiv2.NewStateServiceClient(conn),
		command:     lapiv2.NewCommandServiceClient(conn),
		update:      lapiv2.NewUpdateServiceClient(conn),
		partyAdmin:  adminv2.NewPartyManagementServiceClient(conn),
		userAdmin:   adminv2.NewUserManagementServiceClient(conn),
		interactive: interactivev2.NewInteractiveSubmissionServiceClient(conn),
		auth:        ap,
	}, nil
}

func (c *Client) Conn() *grpc.ClientConn { return c.conn }

func (c *Client) Close() error { return c.conn.Close() }

func (c *Client) State() lapiv2.StateServiceClient { return c.state }

func (c *Client) Command() lapiv2.CommandServiceClient { return c.command }

func (c *Client) Update() lapiv2.UpdateServiceClient { return c.update }

func (c *Client) PartyAdmin() adminv2.PartyManagementServiceClient {
	return c.partyAdmin
}

func (c *Client) UserAdmin() adminv2.UserManagementServiceClient {
	return c.userAdmin
}

func (c *Client) Interactive() interactivev2.InteractiveSubmissionServiceClient {
	return c.interactive
}

func (c *Client) AuthContext(ctx context.Context) context.Context {
	token, err := c.loadToken(ctx)
	if err != nil {
		c.logger.Debug("No JWT token configured (OK with wildcard auth)", zap.Error(err))
		return ctx
	}
	if token == "" {
		return ctx
	}
	return metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token))
}

func (c *Client) InvalidateToken() {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	c.cachedToken = ""
	c.tokenExpiry = time.Time{}
}

func (c *Client) JWTSubject(ctx context.Context) (string, error) {
	// Extract JWT subject if token is configured
	token, err := c.loadToken(ctx)
	if err != nil || token == "" {
		return "", fmt.Errorf("error loading JWT token: %w", err)
	}
	subject, err := extractJWTSubject(token)
	if err != nil {
		return "", fmt.Errorf("error extracting JWT subject: %w", err)
	}
	return subject, nil
}

func (c *Client) loadToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	now := time.Now()
	if c.cachedToken != "" && now.Before(c.tokenExpiry) {
		return c.cachedToken, nil
	}

	tok, exp, err := c.auth.Token(ctx)
	if err != nil {
		return "", err
	}
	c.cachedToken = tok
	c.tokenExpiry = exp
	return tok, nil
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

func (c *Client) GetLedgerEnd(ctx context.Context) (int64, error) {
	authCtx := c.AuthContext(ctx)

	resp, err := c.state.GetLedgerEnd(authCtx, &lapiv2.GetLedgerEndRequest{})
	if err != nil {
		return 0, fmt.Errorf("failed to get ledger end: %w", err)
	}
	return resp.Offset, nil
}

func (c *Client) GetActiveContractsByTemplate(
	ctx context.Context,
	activeAtOffset int64,
	parties []string,
	templateID *lapiv2.Identifier,
) ([]*lapiv2.CreatedEvent, error) {
	if activeAtOffset == 0 {
		return nil, fmt.Errorf("ledger is empty, no contracts exist")
	}
	if len(parties) == 0 {
		return nil, fmt.Errorf("at least one party is required")
	}
	if templateID == nil {
		return nil, fmt.Errorf("templateID is required")
	}

	authCtx := c.AuthContext(ctx)

	filtersByParty := make(map[string]*lapiv2.Filters, len(parties))
	for _, p := range parties {
		filtersByParty[p] = &lapiv2.Filters{
			Cumulative: []*lapiv2.CumulativeFilter{
				{
					IdentifierFilter: &lapiv2.CumulativeFilter_TemplateFilter{
						TemplateFilter: &lapiv2.TemplateFilter{
							TemplateId: templateID,
						},
					},
				},
			},
		}
	}

	stream, err := c.state.GetActiveContracts(authCtx, &lapiv2.GetActiveContractsRequest{
		ActiveAtOffset: activeAtOffset,
		EventFormat: &lapiv2.EventFormat{
			FiltersByParty: filtersByParty,
			Verbose:        true,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get active contracts: %w", err)
	}

	var out []*lapiv2.CreatedEvent
	for {
		msg, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("failed to receive active contract: %w", err)
		}
		if ac := msg.GetActiveContract(); ac != nil && ac.CreatedEvent != nil {
			out = append(out, ac.CreatedEvent)
		}
	}

	return out, nil
}
