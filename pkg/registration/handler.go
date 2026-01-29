package registration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/chainsafe/canton-middleware/pkg/canton"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/keys"
	"go.uber.org/zap"
)

// Handler handles user registration requests
type Handler struct {
	config       *config.APIServerConfig
	db           *apidb.Store
	cantonClient *canton.Client
	keyStore     keys.KeyStore
	logger       *zap.Logger
}

// NewHandler creates a new registration handler with custodial key management
func NewHandler(
	cfg *config.APIServerConfig,
	db *apidb.Store,
	cantonClient *canton.Client,
	keyStore keys.KeyStore,
	logger *zap.Logger,
) *Handler {
	return &Handler{
		config:       cfg,
		db:           db,
		cantonClient: cantonClient,
		keyStore:     keyStore,
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
	ctx := r.Context()

	// Generate Canton keypair for user
	cantonKeyPair, err := keys.GenerateCantonKeyPair()
	if err != nil {
		h.logger.Error("Failed to generate Canton keypair", zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "key generation failed")
		return
	}

	// Allocate a unique Canton party for this user
	partyHint := fmt.Sprintf("user_%s", evmAddress[2:10]) // e.g., "user_f39Fd6e5"
	partyResult, err := h.cantonClient.AllocateParty(ctx, partyHint)
	if err != nil {
		h.logger.Error("Failed to allocate Canton party",
			zap.String("hint", partyHint),
			zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "party allocation failed")
		return
	}
	cantonPartyID := partyResult.PartyID

	h.logger.Info("Allocated Canton party for user",
		zap.String("evm_address", evmAddress),
		zap.String("party_id", cantonPartyID),
		zap.String("public_key", cantonKeyPair.PublicKeyHex()[:32]+"..."))

	// Register fingerprint mapping
	mappingCID, err := h.cantonClient.RegisterUser(ctx, &canton.RegisterUserRequest{
		UserParty:   cantonPartyID,
		Fingerprint: fingerprint,
		EvmAddress:  evmAddress,
	})
	if err != nil {
		h.logger.Error("Failed to register user on Canton",
			zap.String("party", cantonPartyID),
			zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "registration failed")
		return
	}

	// Save user to database
	now := time.Now()
	user := &apidb.User{
		EVMAddress:         evmAddress,
		CantonParty:        cantonPartyID,
		Fingerprint:        fingerprint,
		MappingCID:         mappingCID,
		CantonPartyID:      cantonPartyID,
		CantonKeyCreatedAt: &now,
	}

	if err := h.db.CreateUser(user); err != nil {
		h.logger.Error("Failed to save user", zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "failed to save user")
		return
	}

	// Store the encrypted Canton key
	if h.keyStore != nil {
		if err := h.keyStore.SetUserKey(evmAddress, cantonPartyID, cantonKeyPair.PrivateKey); err != nil {
			h.logger.Error("Failed to store Canton key",
				zap.String("evm_address", evmAddress),
				zap.Error(err))
			// Cleanup: delete the user we just created to maintain consistency
			if delErr := h.db.DeleteUser(evmAddress); delErr != nil {
				h.logger.Error("Failed to cleanup user after key storage failure",
					zap.String("evm_address", evmAddress),
					zap.Error(delErr))
			}
			h.writeError(w, http.StatusInternalServerError, "failed to store Canton key")
			return
		}
	}

	h.logger.Info("User registered",
		zap.String("evm_address", evmAddress),
		zap.String("party", cantonPartyID),
		zap.String("fingerprint", fingerprint))

	// Write success response
	h.writeJSON(w, http.StatusOK, RegisterResponse{
		Party:       cantonPartyID,
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
