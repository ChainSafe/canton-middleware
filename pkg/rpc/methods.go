package rpc

import (
	"context"
	"encoding/json"
	"errors"

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

// handleTotalSupply returns the total token supply from cache
func (h *MethodHandler) handleTotalSupply(ctx context.Context) (interface{}, *Error) {
	// Read from DB cache
	supply, err := h.server.db.GetTotalSupply()
	if err != nil {
		h.server.logger.Error("Failed to get total supply from cache", zap.Error(err))
		return nil, NewError(InternalError, err.Error())
	}
	return &SupplyResult{TotalSupply: supply}, nil
}

// =============================================================================
// Authenticated Methods
// =============================================================================

// handleBalanceOf returns the balance for the authenticated user from cache
func (h *MethodHandler) handleBalanceOf(ctx context.Context, params json.RawMessage) (interface{}, *Error) {
	// Get authenticated user info
	authInfo := auth.AuthInfoFromContext(ctx)
	if authInfo.Fingerprint == "" {
		return nil, NewError(Unauthorized, "user not registered")
	}

	// Get balance from DB cache
	balance, err := h.server.db.GetUserBalanceByFingerprint(authInfo.Fingerprint)
	if err != nil {
		h.server.logger.Error("Failed to get balance from cache",
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

	// Execute transfer on Canton
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

	// Update balance cache atomically for both sender and recipient
	if err := h.server.db.TransferBalanceByFingerprint(authInfo.Fingerprint, recipient.Fingerprint, p.Amount); err != nil {
		h.server.logger.Warn("Failed to update balance cache",
			zap.String("from_fingerprint", authInfo.Fingerprint),
			zap.String("to_fingerprint", recipient.Fingerprint),
			zap.String("amount", p.Amount),
			zap.Error(err))
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

	// Issuer-centric model: all users share the relayer party
	// Users are differentiated by their EVM address/fingerprint via FingerprintMapping
	partyID := h.server.config.Canton.RelayerParty
	if partyID == "" {
		return nil, NewError(InternalError, "relayer_party not configured - required for issuer-centric model")
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

// isInsufficientFunds checks if an error indicates insufficient funds using structured error types.
// Uses errors.Is() to check for sentinel errors from the canton package.
func isInsufficientFunds(err error) bool {
	if err == nil {
		return false
	}
	// Check for structured sentinel errors first
	if errors.Is(err, canton.ErrInsufficientBalance) || errors.Is(err, canton.ErrBalanceFragmented) {
		return true
	}
	return false
}
