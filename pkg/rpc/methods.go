package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/chainsafe/canton-middleware/pkg/canton"
	"go.uber.org/zap"
)

// MethodHandler handles JSON-RPC method dispatch
type MethodHandler struct {
	server *Server
}

// NewMethodHandler creates a new method handler
func NewMethodHandler(server *Server) *MethodHandler {
	return &MethodHandler{server: server}
}

// Methods that require authentication
var authenticatedMethods = map[string]bool{
	"erc20_balanceOf": true,
	"erc20_transfer":  true,
	"user_register":   true, // Requires EVM signature for registration
}

// RequiresAuth returns true if the method requires authentication
func (h *MethodHandler) RequiresAuth(method string) bool {
	return authenticatedMethods[method]
}

// Handle dispatches the method call
func (h *MethodHandler) Handle(ctx context.Context, method string, params json.RawMessage) (interface{}, *Error) {
	switch method {
	case "erc20_name":
		return h.handleName(ctx)
	case "erc20_symbol":
		return h.handleSymbol(ctx)
	case "erc20_decimals":
		return h.handleDecimals(ctx)
	case "erc20_totalSupply":
		return h.handleTotalSupply(ctx)
	case "erc20_balanceOf":
		return h.handleBalanceOf(ctx, params)
	case "erc20_transfer":
		return h.handleTransfer(ctx, params)
	case "user_register":
		return h.handleRegister(ctx)
	default:
		return nil, NewError(MethodNotFound, method)
	}
}

// =============================================================================
// Public Methods (No Auth Required)
// =============================================================================

// handleName returns the token name
func (h *MethodHandler) handleName(ctx context.Context) (interface{}, *Error) {
	return h.server.config.Token.Name, nil
}

// handleSymbol returns the token symbol
func (h *MethodHandler) handleSymbol(ctx context.Context) (interface{}, *Error) {
	return h.server.config.Token.Symbol, nil
}

// handleDecimals returns the token decimals
func (h *MethodHandler) handleDecimals(ctx context.Context) (interface{}, *Error) {
	return h.server.config.Token.Decimals, nil
}

// handleTotalSupply returns the total token supply
func (h *MethodHandler) handleTotalSupply(ctx context.Context) (interface{}, *Error) {
	supply, err := h.server.cantonClient.GetTotalSupply(ctx)
	if err != nil {
		h.server.logger.Error("Failed to get total supply", zap.Error(err))
		return nil, NewError(InternalError, err.Error())
	}
	return &SupplyResult{TotalSupply: supply}, nil
}

// =============================================================================
// Authenticated Methods
// =============================================================================

// handleBalanceOf returns the balance for the authenticated user
func (h *MethodHandler) handleBalanceOf(ctx context.Context, params json.RawMessage) (interface{}, *Error) {
	// Get authenticated user info
	authInfo := auth.AuthInfoFromContext(ctx)
	if authInfo.Fingerprint == "" {
		return nil, NewError(Unauthorized, "user not registered")
	}

	// Get balance from Canton
	balance, err := h.server.cantonClient.GetUserBalance(ctx, authInfo.Fingerprint)
	if err != nil {
		h.server.logger.Error("Failed to get balance",
			zap.String("fingerprint", authInfo.Fingerprint),
			zap.Error(err))
		return nil, NewError(InternalError, err.Error())
	}

	return &BalanceResult{
		Balance: balance,
		Address: authInfo.EVMAddress,
	}, nil
}

// handleTransfer transfers tokens from the authenticated user to another user
func (h *MethodHandler) handleTransfer(ctx context.Context, params json.RawMessage) (interface{}, *Error) {
	// Get authenticated user info
	authInfo := auth.AuthInfoFromContext(ctx)
	if authInfo.Fingerprint == "" {
		return nil, NewError(Unauthorized, "user not registered")
	}

	// Parse params
	var p TransferParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, NewError(InvalidParams, err.Error())
	}

	if p.To == "" {
		return nil, NewError(InvalidParams, "to address is required")
	}
	if p.Amount == "" {
		return nil, NewError(InvalidParams, "amount is required")
	}

	// Validate recipient address
	if !auth.ValidateEVMAddress(p.To) {
		return nil, NewError(InvalidParams, "invalid recipient address")
	}

	// Get recipient's fingerprint
	toAddress := auth.NormalizeAddress(p.To)
	recipient, err := h.server.db.GetUserByEVMAddress(toAddress)
	if err != nil {
		h.server.logger.Error("Failed to get recipient", zap.Error(err))
		return nil, NewError(InternalError, err.Error())
	}
	if recipient == nil {
		return nil, NewError(NotFound, "recipient not registered")
	}

	// Execute transfer
	err = h.server.cantonClient.Transfer(ctx, &canton.TransferRequest{
		FromFingerprint: authInfo.Fingerprint,
		ToFingerprint:   recipient.Fingerprint,
		Amount:          p.Amount,
	})
	if err != nil {
		h.server.logger.Error("Transfer failed",
			zap.String("from", authInfo.EVMAddress),
			zap.String("to", toAddress),
			zap.String("amount", p.Amount),
			zap.Error(err))

		// Check for specific error types
		if isInsufficientFunds(err) {
			return nil, NewError(InsufficientFunds, err.Error())
		}
		return nil, NewError(InternalError, err.Error())
	}

	h.server.logger.Info("Transfer completed",
		zap.String("from", authInfo.EVMAddress),
		zap.String("to", toAddress),
		zap.String("amount", p.Amount))

	return &TransferResult{Success: true}, nil
}

// handleRegister registers a new user
func (h *MethodHandler) handleRegister(ctx context.Context) (interface{}, *Error) {
	// Get EVM address from context (set by EVM signature auth)
	evmAddress, ok := auth.EVMAddressFromContext(ctx)
	if !ok {
		return nil, NewError(Unauthorized, "EVM signature required for registration")
	}

	// Check if user already exists
	exists, err := h.server.db.UserExists(evmAddress)
	if err != nil {
		h.server.logger.Error("Failed to check user existence", zap.Error(err))
		return nil, NewError(InternalError, err.Error())
	}
	if exists {
		return nil, NewError(AlreadyRegistered, "user already registered")
	}

	// Check whitelist
	whitelisted, err := h.server.db.IsWhitelisted(evmAddress)
	if err != nil {
		h.server.logger.Error("Failed to check whitelist", zap.Error(err))
		return nil, NewError(InternalError, err.Error())
	}
	if !whitelisted {
		return nil, NewError(NotWhitelisted, "address not whitelisted for registration")
	}

	// Compute fingerprint
	fingerprint := auth.ComputeFingerprint(evmAddress)

	// Allocate a Canton party via HTTP API
	partyID, err := allocateParty(ctx, h.server.config.Canton.RPCURL)
	if err != nil {
		h.server.logger.Error("Failed to allocate party", zap.Error(err))
		return nil, NewError(InternalError, fmt.Sprintf("failed to allocate party: %v", err))
	}

	// Register the user's fingerprint mapping on Canton
	mappingCID, err := h.server.cantonClient.RegisterUser(ctx, &canton.RegisterUserRequest{
		UserParty:   partyID,
		Fingerprint: fingerprint,
		EvmAddress:  evmAddress,
	})
	if err != nil {
		h.server.logger.Error("Failed to register user on Canton",
			zap.String("party", partyID),
			zap.Error(err))
		return nil, NewError(InternalError, err.Error())
	}

	// Save user to database
	user := &apidb.User{
		EVMAddress:  evmAddress,
		CantonParty: partyID,
		Fingerprint: fingerprint,
		MappingCID:  mappingCID,
	}
	if err := h.server.db.CreateUser(user); err != nil {
		h.server.logger.Error("Failed to save user", zap.Error(err))
		return nil, NewError(InternalError, err.Error())
	}

	h.server.logger.Info("User registered",
		zap.String("evm_address", evmAddress),
		zap.String("party", partyID),
		zap.String("fingerprint", fingerprint))

	return &RegisterResult{
		Party:       partyID,
		Fingerprint: fingerprint,
		MappingCID:  mappingCID,
	}, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// allocateParty allocates a new Canton party via HTTP API
func allocateParty(ctx context.Context, rpcURL string) (string, error) {
	// Convert gRPC URL to HTTP URL
	// Canton gRPC is typically on port 5011, HTTP on 5013
	httpURL := convertToHTTPURL(rpcURL)

	// Generate a unique party hint based on timestamp
	partyHint := fmt.Sprintf("User_%d", time.Now().UnixNano())

	// Create request body
	reqBody := map[string]string{
		"partyIdHint": partyHint,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/v2/parties", httpURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call party allocation API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("party allocation failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var partyResp struct {
		PartyDetails struct {
			Party string `json:"party"`
		} `json:"partyDetails"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&partyResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if partyResp.PartyDetails.Party == "" {
		return "", fmt.Errorf("party allocation returned empty party ID")
	}

	return partyResp.PartyDetails.Party, nil
}

// convertToHTTPURL converts a gRPC URL to HTTP URL
func convertToHTTPURL(grpcURL string) string {
	// Canton gRPC is typically on port 5011, HTTP on 5013
	// Replace port if it looks like the standard gRPC port
	host := grpcURL
	
	// Remove any protocol prefix
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")
	
	// Check for common gRPC ports and convert to HTTP ports
	// Participant 1: gRPC 5011 -> HTTP 5013
	// Participant 2: gRPC 5021 -> HTTP 5023
	if strings.HasSuffix(host, ":5011") {
		host = strings.TrimSuffix(host, ":5011") + ":5013"
	} else if strings.HasSuffix(host, ":5021") {
		host = strings.TrimSuffix(host, ":5021") + ":5023"
	}
	
	return fmt.Sprintf("http://%s", host)
}

// isInsufficientFunds checks if an error indicates insufficient funds
func isInsufficientFunds(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return containsAny(msg, []string{
		"insufficient",
		"balance",
		"no holding",
	})
}

// containsAny checks if s contains any of the substrings
func containsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if contains(s, sub) {
			return true
		}
	}
	return false
}

// contains is a simple case-insensitive contains check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsCI(s, substr))
}

func containsCI(s, substr string) bool {
	s = toLower(s)
	substr = toLower(substr)
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

