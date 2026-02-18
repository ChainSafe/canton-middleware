package registration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
	"github.com/chainsafe/canton-middleware/pkg/auth"
	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/identity"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/keys"
	"github.com/ethereum/go-ethereum/crypto"
	"go.uber.org/zap"
)

// Handler handles user registration requests
type Handler struct {
	config       *config.APIServerConfig
	db           *apidb.Store
	cantonClient canton.Identity
	keyStore     keys.KeyStore
	logger       *zap.Logger
}

// NewHandler creates a new registration handler with custodial key management
func NewHandler(
	cfg *config.APIServerConfig,
	db *apidb.Store,
	cantonClient canton.Identity,
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
// Supports two registration modes:
// 1. Web3 user: signature + message (EIP-191 signature from MetaMask)
// 2. Canton native user: canton_party_id + canton_signature + message (from Loop wallet signMessage)
type RegisterRequest struct {
	// Web3 user registration (EIP-191 signature)
	Signature string `json:"signature,omitempty"`
	Message   string `json:"message,omitempty"`

	// Canton native user registration (Loop wallet signMessage)
	CantonPartyID   string `json:"canton_party_id,omitempty"`
	CantonSignature string `json:"canton_signature,omitempty"`
}

// RegisterResponse represents a registration response
type RegisterResponse struct {
	Party       string `json:"party"`
	Fingerprint string `json:"fingerprint"`
	MappingCID  string `json:"mapping_cid,omitempty"`
	EVMAddress  string `json:"evm_address,omitempty"` // Returned for Canton native users
	PrivateKey  string `json:"private_key,omitempty"` // Returned for Canton native users (for MetaMask import)
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

	// Determine registration type and route accordingly
	// Canton native registration is identified by presence of canton_party_id
	// (signature can be empty if SKIP_CANTON_SIG_VERIFY=true)
	if req.CantonPartyID != "" {
		// Canton native user registration
		h.handleCantonNativeRegistration(w, r, &req)
	} else {
		// Web3 user registration (existing flow)
		h.handleWeb3Registration(w, r, &req)
	}
}

// handleWeb3Registration handles registration for web3 users (EVM signature)
func (h *Handler) handleWeb3Registration(w http.ResponseWriter, r *http.Request, req *RegisterRequest) {
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
	var cantonPartyID string
	if err != nil {
		// Check if party already exists (from previous registration)
		errStr := err.Error()
		if strings.Contains(errStr, "already allocated") || strings.Contains(errStr, "Party already exists") {
			// Party exists - must use ListParties to get the full (non-truncated) party ID
			h.logger.Info("Party already exists, looking up full party ID",
				zap.String("hint", partyHint))

			existingParties, listErr := h.cantonClient.ListParties(ctx)
			if listErr != nil {
				h.logger.Error("Failed to list parties to find existing",
					zap.String("hint", partyHint),
					zap.Error(listErr))
				h.writeError(w, http.StatusInternalServerError, "party lookup failed")
				return
			}

			h.logger.Info("Searching parties list",
				zap.Int("party_count", len(existingParties)))
			for _, p := range existingParties {
				if strings.HasPrefix(p.PartyID, partyHint+"::") {
					cantonPartyID = p.PartyID
					h.logger.Info("Found existing party in list",
						zap.String("party_id", cantonPartyID))
					break
				}
			}

			if cantonPartyID == "" {
				h.logger.Error("Could not find existing party in list",
					zap.String("hint", partyHint))
				h.writeError(w, http.StatusInternalServerError, "party allocation failed")
				return
			}
		} else {
			h.logger.Error("Failed to allocate Canton party",
				zap.String("hint", partyHint),
				zap.Error(err))
			h.writeError(w, http.StatusInternalServerError, "party allocation failed")
			return
		}
	} else {
		cantonPartyID = partyResult.PartyID
	}

	h.logger.Info("Allocated Canton party for user",
		zap.String("evm_address", evmAddress),
		zap.String("party_id", cantonPartyID),
		zap.String("public_key", cantonKeyPair.PublicKeyHex()[:32]+"..."))

	// Grant CanActAs rights to the OAuth client for this party
	// This enables the custodial model: users own their holdings, API server acts on their behalf
	if err = h.cantonClient.GrantActAsParty(ctx, cantonPartyID); err != nil {
		h.logger.Warn("Failed to grant CanActAs rights (transfers may fail)",
			zap.String("party_id", cantonPartyID),
			zap.Error(err))
		// Continue anyway - the right might already exist or can be granted manually
	}

	// Create fingerprint mapping on Canton (direct creation by issuer)
	var mapping *canton.FingerprintMapping
	mapping, err = h.cantonClient.CreateFingerprintMapping(ctx, canton.CreateFingerprintMappingRequest{
		UserParty:   cantonPartyID,
		Fingerprint: fingerprint,
		EvmAddress:  evmAddress,
	})
	if err != nil {
		h.logger.Error("Failed to create FingerprintMapping on Canton",
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
		MappingCID:         mapping.ContractID,
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

	h.logger.Info("Web3 user registered",
		zap.String("evm_address", evmAddress),
		zap.String("party", cantonPartyID),
		zap.String("fingerprint", fingerprint))

	// Write success response
	h.writeJSON(w, http.StatusOK, RegisterResponse{
		Party:       cantonPartyID,
		Fingerprint: fingerprint,
		MappingCID:  mapping.ContractID,
		EVMAddress:  evmAddress,
	})
}

// handleCantonNativeRegistration handles registration for Canton native users (Loop wallet)
// These users already have a Canton party ID - we generate an EVM keypair for MetaMask access
func (h *Handler) handleCantonNativeRegistration(w http.ResponseWriter, r *http.Request, req *RegisterRequest) {
	ctx := r.Context()

	// Validate Canton party ID format
	if err := auth.ValidateCantonPartyID(req.CantonPartyID); err != nil {
		h.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid canton_party_id: %v", err))
		return
	}

	// Verify Canton signature (proves ownership of the party)
	// Can be skipped for local testing with SKIP_CANTON_SIG_VERIFY=true
	skipVerify := os.Getenv("SKIP_CANTON_SIG_VERIFY") == "true"

	if !skipVerify {
		if req.Message == "" {
			h.writeError(w, http.StatusBadRequest, "message required for Canton signature verification")
			return
		}

		valid, err := auth.VerifyCantonSignature(req.CantonPartyID, req.Message, req.CantonSignature)
		if err != nil {
			h.writeError(w, http.StatusUnauthorized, fmt.Sprintf("signature verification failed: %v", err))
			return
		}
		if !valid {
			h.writeError(w, http.StatusUnauthorized, "invalid Canton signature")
			return
		}
	} else {
		h.logger.Warn("Canton signature verification SKIPPED (local testing mode)",
			zap.String("canton_party", req.CantonPartyID))
	}

	// Check if this exact party ID is already registered
	existingUser, err := h.db.GetUserByCantonPartyID(req.CantonPartyID)
	if err != nil {
		h.logger.Error("Failed to check existing user", zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if existingUser != nil {
		h.writeError(w, http.StatusConflict, "Canton party already registered")
		return
	}

	// Extract fingerprint from party ID for mapping
	fingerprint, err := auth.ExtractFingerprintFromPartyID(req.CantonPartyID)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to extract fingerprint: %v", err))
		return
	}

	// Generate EVM keypair for MetaMask access
	evmKeyPair, err := keys.GenerateCantonKeyPair() // Same secp256k1 curve
	if err != nil {
		h.logger.Error("Failed to generate EVM keypair", zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "key generation failed")
		return
	}

	// Derive EVM address from the generated keypair
	evmAddress := deriveEVMAddressFromPublicKey(evmKeyPair.PublicKey)

	h.logger.Info("Generated EVM address for Canton native user",
		zap.String("canton_party", req.CantonPartyID),
		zap.String("evm_address", evmAddress))

	// Grant CanActAs rights to the OAuth client for this party
	// This enables the custodial model: native users can also use MetaMask via the API server
	if err = h.cantonClient.GrantActAsParty(ctx, req.CantonPartyID); err != nil {
		h.logger.Warn("Failed to grant CanActAs rights (transfers may fail)",
			zap.String("party_id", req.CantonPartyID),
			zap.Error(err))
		// Continue anyway - the right might already exist or can be granted manually
	}

	// Create fingerprint mapping on Canton (direct creation by issuer)
	// For Canton native users, the party already exists, we just create the mapping
	var mapping *canton.FingerprintMapping
	mapping, err = h.cantonClient.CreateFingerprintMapping(ctx, canton.CreateFingerprintMappingRequest{
		UserParty:   req.CantonPartyID,
		Fingerprint: fingerprint,
		EvmAddress:  evmAddress,
	})
	if err != nil {
		h.logger.Error("Failed to create FingerprintMapping on Canton",
			zap.String("party", req.CantonPartyID),
			zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "registration failed")
		return
	}

	// Save user to database
	now := time.Now()
	user := &apidb.User{
		EVMAddress:         evmAddress,
		CantonParty:        req.CantonPartyID,
		Fingerprint:        fingerprint,
		MappingCID:         mapping.ContractID,
		CantonPartyID:      req.CantonPartyID,
		CantonKeyCreatedAt: &now,
	}

	if err := h.db.CreateUser(user); err != nil {
		h.logger.Error("Failed to save user", zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "failed to save user")
		return
	}

	// Store the EVM private key (encrypted) so user can download it later for MetaMask
	if h.keyStore != nil {
		if err := h.keyStore.SetUserKey(evmAddress, req.CantonPartyID, evmKeyPair.PrivateKey); err != nil {
			h.logger.Error("Failed to store EVM key",
				zap.String("evm_address", evmAddress),
				zap.Error(err))
			// Cleanup: delete the user we just created
			if delErr := h.db.DeleteUser(evmAddress); delErr != nil {
				h.logger.Error("Failed to cleanup user after key storage failure",
					zap.String("evm_address", evmAddress),
					zap.Error(delErr))
			}
			h.writeError(w, http.StatusInternalServerError, "failed to store EVM key")
			return
		}
	}

	h.logger.Info("Canton native user registered",
		zap.String("canton_party", req.CantonPartyID),
		zap.String("evm_address", evmAddress),
		zap.String("fingerprint", fingerprint))

	// Write success response - include EVM address and private key so user can import to MetaMask
	h.writeJSON(w, http.StatusOK, RegisterResponse{
		Party:       req.CantonPartyID,
		Fingerprint: fingerprint,
		MappingCID:  mapping.ContractID,
		EVMAddress:  evmAddress,
		PrivateKey:  evmKeyPair.PrivateKeyHex(), // For MetaMask import
	})
}

// deriveEVMAddressFromPublicKey derives an Ethereum address from a compressed secp256k1 public key
func deriveEVMAddressFromPublicKey(compressedPubKey []byte) string {
	// Decompress the public key
	pubKey, err := crypto.DecompressPubkey(compressedPubKey)
	if err != nil {
		// Fallback: if decompression fails, hash the compressed key directly
		hash := crypto.Keccak256Hash(compressedPubKey)
		return "0x" + hash.Hex()[26:] // Take last 20 bytes
	}
	// Derive address from uncompressed public key
	addr := crypto.PubkeyToAddress(*pubKey)
	return addr.Hex()
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, ErrorResponse{Error: message})
}
