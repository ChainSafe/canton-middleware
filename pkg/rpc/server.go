package rpc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/chainsafe/canton-middleware/pkg/canton"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"go.uber.org/zap"
)

// Server handles JSON-RPC requests for the ERC-20 API
type Server struct {
	config       *config.APIServerConfig
	db           *apidb.Store
	cantonClient *canton.Client
	jwtValidator *auth.JWTValidator
	logger       *zap.Logger
	handler      *MethodHandler
}

// NewServer creates a new RPC server
func NewServer(
	cfg *config.APIServerConfig,
	db *apidb.Store,
	cantonClient *canton.Client,
	logger *zap.Logger,
) *Server {
	var jwtValidator *auth.JWTValidator
	if cfg.JWKS.URL != "" {
		jwtValidator = auth.NewJWTValidator(cfg.JWKS.URL, cfg.JWKS.Issuer)
	}

	s := &Server{
		config:       cfg,
		db:           db,
		cantonClient: cantonClient,
		jwtValidator: jwtValidator,
		logger:       logger,
	}

	// Create method handler
	s.handler = NewMethodHandler(s)

	return s
}

// ServeHTTP handles HTTP requests
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Health check endpoint
	if r.URL.Path == "/health" {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
		return
	}

	// Only handle POST to /rpc
	if r.URL.Path != "/rpc" {
		http.NotFound(w, r)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		s.writeError(w, nil, NewError(ParseError, "failed to read request"))
		return
	}

	// Parse JSON-RPC request
	var req Request
	if err := json.Unmarshal(body, &req); err != nil {
		s.writeError(w, nil, NewError(ParseError, err.Error()))
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		s.writeError(w, req.ID, NewError(InvalidRequest, err.Error()))
		return
	}

	// Check if method requires authentication
	requiresAuth := s.handler.RequiresAuth(req.Method)

	// Create context with request info
	ctx := r.Context()

	// Authenticate if required
	if requiresAuth {
		authCtx, err := s.authenticate(ctx, r, req.Method)
		if err != nil {
			s.logger.Warn("Authentication failed",
				zap.String("method", req.Method),
				zap.Error(err))
			s.writeError(w, req.ID, NewError(Unauthorized, err.Error()))
			return
		}
		ctx = authCtx
	}

	// Handle the method
	result, rpcErr := s.handler.Handle(ctx, req.Method, req.Params)
	if rpcErr != nil {
		s.writeError(w, req.ID, rpcErr)
		return
	}

	// Write success response
	s.writeResponse(w, SuccessResponse(req.ID, result))
}

// authenticate verifies the request authentication
func (s *Server) authenticate(ctx context.Context, r *http.Request, method string) (context.Context, error) {
	// Try EVM signature authentication first (primary for ERC-20 API)
	signature := r.Header.Get("X-Signature")
	message := r.Header.Get("X-Message")

	if signature != "" && message != "" {
		return s.authenticateEVM(ctx, signature, message)
	}

	// Try JWT authentication if configured
	if s.jwtValidator != nil && s.jwtValidator.IsConfigured() {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			return s.authenticateJWT(ctx, token)
		}
	}

	return nil, &AuthError{Message: "no valid authentication provided"}
}

// authenticateEVM verifies EVM signature and returns authenticated context
func (s *Server) authenticateEVM(ctx context.Context, signature, message string) (context.Context, error) {
	// Verify the signature
	recoveredAddr, err := auth.VerifyEIP191Signature(message, signature)
	if err != nil {
		return nil, &AuthError{Message: "invalid signature: " + err.Error()}
	}

	evmAddress := auth.NormalizeAddress(recoveredAddr.Hex())

	// Look up the user in the database
	user, err := s.db.GetUserByEVMAddress(evmAddress)
	if err != nil {
		return nil, &AuthError{Message: "database error: " + err.Error()}
	}

	// For registration, user might not exist yet - that's OK
	// The method handler will check if user exists for non-registration methods
	if user != nil {
		ctx = auth.WithAuthInfo(ctx, &auth.AuthInfo{
			EVMAddress:  user.EVMAddress,
			CantonParty: user.CantonParty,
			Fingerprint: user.Fingerprint,
			UserID:      user.ID,
		})
	} else {
		// User not registered yet - only include EVM address
		ctx = auth.WithEVMAddress(ctx, evmAddress)
	}

	return ctx, nil
}

// authenticateJWT verifies JWT token and returns authenticated context
func (s *Server) authenticateJWT(ctx context.Context, token string) (context.Context, error) {
	claims, err := s.jwtValidator.ValidateToken(token)
	if err != nil {
		return nil, &AuthError{Message: "invalid token: " + err.Error()}
	}

	// Extract EVM address from claims (assuming it's stored there)
	evmAddress, ok := claims["evm_address"].(string)
	if !ok {
		return nil, &AuthError{Message: "token missing evm_address claim"}
	}

	// Look up user
	user, err := s.db.GetUserByEVMAddress(evmAddress)
	if err != nil {
		return nil, &AuthError{Message: "database error: " + err.Error()}
	}

	if user != nil {
		ctx = auth.WithAuthInfo(ctx, &auth.AuthInfo{
			EVMAddress:  user.EVMAddress,
			CantonParty: user.CantonParty,
			Fingerprint: user.Fingerprint,
			UserID:      user.ID,
		})
	} else {
		ctx = auth.WithEVMAddress(ctx, evmAddress)
	}

	return ctx, nil
}

// writeResponse writes a JSON-RPC response
func (s *Server) writeResponse(w http.ResponseWriter, resp *Response) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// writeError writes a JSON-RPC error response
func (s *Server) writeError(w http.ResponseWriter, id interface{}, err *Error) {
	s.writeResponse(w, ErrorResponse(id, err))
}

// AuthError represents an authentication error
type AuthError struct {
	Message string
}

func (e *AuthError) Error() string {
	return e.Message
}

