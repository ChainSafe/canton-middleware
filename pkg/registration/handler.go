package registration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/chainsafe/canton-middleware/pkg/canton"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"go.uber.org/zap"
)

// Handler handles user registration requests
type Handler struct {
	config       *config.APIServerConfig
	db           *apidb.Store
	cantonClient *canton.Client
	logger       *zap.Logger
}

// NewHandler creates a new registration handler
func NewHandler(
	cfg *config.APIServerConfig,
	db *apidb.Store,
	cantonClient *canton.Client,
	logger *zap.Logger,
) *Handler {
	return &Handler{
		config:       cfg,
		db:           db,
		cantonClient: cantonClient,
		logger:       logger,
	}
}

// RegisterRequest represents a registration request
type RegisterRequest struct {
	Signature string `json:"signature"`
	Message   string `json:"message"`
}

// RegisterResponse represents a registration response
type RegisterResponse struct {
	Party       string `json:"party"`
	Fingerprint string `json:"fingerprint"`
	MappingCID  string `json:"mapping_cid"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// ServeHTTP handles HTTP requests
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Read request body
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "failed to read request")
		return
	}

	// Parse request
	var req RegisterRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Verify the signature (either from request body or headers)
	signature := req.Signature
	message := req.Message

	// Try headers if not in body
	if signature == "" {
		signature = r.Header.Get("X-Signature")
		message = r.Header.Get("X-Message")
	}

	if signature == "" || message == "" {
		h.writeError(w, http.StatusUnauthorized, "signature and message required")
		return
	}

	// Verify EVM signature
	recoveredAddr, err := auth.VerifyEIP191Signature(message, signature)
	if err != nil {
		h.writeError(w, http.StatusUnauthorized, fmt.Sprintf("invalid signature: %v", err))
		return
	}

	evmAddress := auth.NormalizeAddress(recoveredAddr.Hex())

	// Check if user already exists
	exists, err := h.db.UserExists(evmAddress)
	if err != nil {
		h.logger.Error("Failed to check user existence", zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if exists {
		h.writeError(w, http.StatusConflict, "user already registered")
		return
	}

	// Check whitelist
	whitelisted, err := h.db.IsWhitelisted(evmAddress)
	if err != nil {
		h.logger.Error("Failed to check whitelist", zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if !whitelisted {
		h.writeError(w, http.StatusForbidden, "address not whitelisted for registration")
		return
	}

	// Compute fingerprint
	fingerprint := auth.ComputeFingerprint(evmAddress)

	// Get the relayer party ID
	partyID := h.config.Canton.RelayerParty
	if partyID == "" {
		h.writeError(w, http.StatusInternalServerError, "relayer_party not configured")
		return
	}

	// Register the user's fingerprint mapping on Canton
	ctx := r.Context()
	mappingCID, err := h.cantonClient.RegisterUser(ctx, &canton.RegisterUserRequest{
		UserParty:   partyID,
		Fingerprint: fingerprint,
		EvmAddress:  evmAddress,
	})
	if err != nil {
		h.logger.Error("Failed to register user on Canton",
			zap.String("party", partyID),
			zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "registration failed")
		return
	}

	// Save user to database
	user := &apidb.User{
		EVMAddress:  evmAddress,
		CantonParty: partyID,
		Fingerprint: fingerprint,
		MappingCID:  mappingCID,
	}
	if err := h.db.CreateUser(user); err != nil {
		h.logger.Error("Failed to save user", zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "failed to save user")
		return
	}

	h.logger.Info("User registered",
		zap.String("evm_address", evmAddress),
		zap.String("party", partyID),
		zap.String("fingerprint", fingerprint))

	// Write success response
	h.writeJSON(w, http.StatusOK, RegisterResponse{
		Party:       partyID,
		Fingerprint: fingerprint,
		MappingCID:  mappingCID,
	})
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, ErrorResponse{Error: message})
}
