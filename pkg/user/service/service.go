package service

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/auth"
	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/identity"
	"github.com/chainsafe/canton-middleware/pkg/keys"
	"github.com/chainsafe/canton-middleware/pkg/user"
	"github.com/chainsafe/canton-middleware/pkg/userstore"
)

// Constants for registration operations
const (
	// partyHintLength is the number of characters from EVM address used in party hint
	// Uses first 8 characters after "0x" prefix (e.g., "user_12345678")
	partyHintLength = 8

	// cantonKeySize is the required size for Canton private keys (32 bytes for secp256k1)
	cantonKeySize = 32
)

var (
	ErrUserAlreadyRegistered  = errors.New("user already registered")
	ErrNotWhitelisted         = errors.New("address not whitelisted for registration")
	ErrPartyAlreadyAllocated  = errors.New("canton party already allocated for this user")
	ErrInvalidCantonSignature = errors.New("invalid Canton signature")
	ErrPartyAlreadyRegistered = errors.New("canton party already registered")
)

// Store is the narrow data-access interface for the registration service.
// Defined here to keep registration service decoupled from userstore implementation details.
//
//go:generate mockery --name Store --output mocks --outpkg mocks --filename mock_store.go --with-expecter
type Store interface {
	UserExists(ctx context.Context, evmAddress string) (bool, error)
	IsWhitelisted(ctx context.Context, evmAddress string) (bool, error)
	CreateUser(ctx context.Context, user *user.User) error
	GetUserByCantonPartyID(ctx context.Context, partyID string) (*user.User, error)
	DeleteUser(ctx context.Context, evmAddress string) error
}

// Service defines the interface for the registration business logic
//
//go:generate mockery --name Service --output mocks --outpkg mocks --filename mock_service.go --with-expecter
type Service interface {
	RegisterWeb3User(ctx context.Context, req *user.RegisterRequest) (*user.RegisterResponse, error)
	RegisterCantonNativeUser(ctx context.Context, req *user.RegisterRequest) (*user.RegisterResponse, error)
}

type registrationService struct {
	store                           Store
	cantonClient                    canton.Identity
	logger                          *zap.Logger
	keyCipher                       keys.KeyCipher
	skipCantonSignatureVerification bool
}

// NewService creates a new registration service
func NewService(
	store Store,
	cantonClient canton.Identity,
	keyCipher keys.KeyCipher,
	logger *zap.Logger,
	skipCantonSignatureVerification bool,
) Service {
	return &registrationService{
		store:                           store,
		cantonClient:                    cantonClient,
		logger:                          logger,
		keyCipher:                       keyCipher,
		skipCantonSignatureVerification: skipCantonSignatureVerification,
	}
}

// RegisterWeb3User registers a Web3 user with EIP-191 signature verification.
// This flow generates a Canton party ID and keys for the user on their behalf.
//
// The registration process:
//  1. Verifies EIP-191 signature to prove EVM address ownership
//  2. Checks if user already registered
//  3. Validates address is whitelisted
//  4. Generates Canton keypair
//  5. Allocates external Canton party
//  6. Creates fingerprint mapping on Canton
//  7. Saves user and encrypted keys to database
//
// Returns registration details including Canton party ID and fingerprint.
// On any failure after party allocation, attempts to cleanup database records.
func (s *registrationService) RegisterWeb3User(
	ctx context.Context,
	req *user.RegisterRequest,
) (*user.RegisterResponse, error) {
	// Verify EVM signature
	recoveredAddr, err := auth.VerifyEIP191Signature(req.Message, req.Signature)
	if err != nil {
		return nil, apperrors.BadRequestError(err, "invalid signature")
	}

	evmAddress := auth.NormalizeAddress(recoveredAddr.Hex())
	s.logger.Info("Web3 registration initiated", zap.String("evm_address", evmAddress))

	// Check if user already exists
	exists, err := s.store.UserExists(ctx, evmAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to check user existence: %w", err)
	}
	if exists {
		return nil, apperrors.ConflictError(ErrUserAlreadyRegistered, "user already registered")
	}

	// Check whitelist
	whitelisted, err := s.store.IsWhitelisted(ctx, evmAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to check whitelist: %w", err)
	}
	if !whitelisted {
		return nil, apperrors.ForbiddenError(ErrNotWhitelisted, "address not whitelisted for registration")
	}

	// Compute fingerprint
	fingerprint := auth.ComputeFingerprint(evmAddress)

	// Generate Canton keypair for user
	cantonKeyPair, err := keys.GenerateCantonKeyPair()
	if err != nil {
		return nil, fmt.Errorf("key generation failed: %w", err)
	}

	// Allocate an external Canton party for this user
	partyHint := generatePartyHint(evmAddress)
	spkiKey, err := cantonKeyPair.SPKIPublicKey()
	if err != nil {
		return nil, fmt.Errorf("key encoding failed: %w", err)
	}

	partyResult, err := s.cantonClient.AllocateExternalParty(ctx, partyHint, spkiKey, cantonKeyPair)
	if err != nil {
		if isPartyAlreadyAllocatedError(err) {
			return nil, apperrors.ConflictError(ErrPartyAlreadyAllocated, "Canton party already allocated for this user")
		}
		return nil, fmt.Errorf("party allocation failed: %w", err)
	}

	cantonPartyID := partyResult.PartyID
	// Create fingerprint mapping on Canton
	mapping, err := s.cantonClient.CreateFingerprintMapping(ctx, canton.CreateFingerprintMappingRequest{
		UserParty:   cantonPartyID,
		Fingerprint: fingerprint,
		EvmAddress:  evmAddress,
	})
	if err != nil {
		return nil, fmt.Errorf("fingerprint mapping creation failed: %w", err)
	}

	encryptedPKey, err := s.keyCipher.Encrypt(cantonKeyPair.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt key: %w", err)
	}
	regUser := user.New(
		evmAddress,
		cantonPartyID,
		fingerprint,
		mapping.ContractID,
		encryptedPKey,
	)

	err = s.store.CreateUser(ctx, regUser)
	if err != nil {
		return nil, fmt.Errorf("failed to save user: %w", err)
	}

	return &user.RegisterResponse{
		Party:       cantonPartyID,
		Fingerprint: fingerprint,
		MappingCID:  mapping.ContractID,
		EVMAddress:  evmAddress,
	}, nil
}

// RegisterCantonNativeUser registers a Canton native user (e.g., Loop wallet user).
// These users already have a Canton party ID and are registering to access EVM-compatible features.
//
// The registration process:
//  1. Validates Canton party ID format
//  2. Verifies Canton signature (if not skipped in config)
//  3. Checks if party already registered
//  4. Extracts fingerprint from party ID
//  5. Generates EVM keypair for MetaMask access
//  6. Creates fingerprint mapping on Canton
//  7. Saves user and keys to database
//
// Returns registration details including the generated EVM address and private key.
// The private key allows users to import the account into MetaMask.
func (s *registrationService) RegisterCantonNativeUser(
	ctx context.Context,
	req *user.RegisterRequest,
) (*user.RegisterResponse, error) {
	// Validate Canton party ID format
	if err := auth.ValidateCantonPartyID(req.CantonPartyID); err != nil {
		return nil, apperrors.BadRequestError(err, "invalid canton_party_id")
	}

	// Verify Canton signature (configurable)
	if !s.skipCantonSignatureVerification {
		if req.Message == "" {
			return nil, apperrors.BadRequestError(nil, "message required for Canton signature verification")
		}

		valid, err := auth.VerifyCantonSignature(req.CantonPartyID, req.Message, req.CantonSignature)
		if err != nil {
			return nil, apperrors.BadRequestError(err, "signature verification failed")
		}
		if !valid {
			return nil, apperrors.UnAuthorizedError(ErrInvalidCantonSignature, "invalid Canton signature")
		}
	} else {
		s.logger.Debug("Canton signature verification skipped (development mode)",
			zap.String("party_id", req.CantonPartyID))
	}

	// Check if this exact party ID is already registered
	existingUser, err := s.store.GetUserByCantonPartyID(ctx, req.CantonPartyID)
	if err != nil && !errors.Is(err, userstore.ErrUserNotFound) {
		return nil, fmt.Errorf("failed to check user existence: %w", err)
	}
	if existingUser != nil {
		return nil, apperrors.ConflictError(ErrPartyAlreadyRegistered, "canton party already registered")
	}

	// Extract fingerprint from party ID for mapping
	fingerprint, err := auth.ExtractFingerprintFromPartyID(req.CantonPartyID)
	if err != nil {
		return nil, fmt.Errorf("failed to extract fingerprint: %w", err)
	}

	// Generate EVM keypair for MetaMask access
	evmKeyPair, err := keys.GenerateCantonKeyPair()
	if err != nil {
		return nil, fmt.Errorf("key generation failed: %w", err)
	}

	// Derive EVM address
	evmAddress := keys.DeriveEVMAddressFromPublicKey(evmKeyPair.PublicKey)
	// Create fingerprint mapping on Canton
	mapping, err := s.cantonClient.CreateFingerprintMapping(ctx, canton.CreateFingerprintMappingRequest{
		UserParty:   req.CantonPartyID,
		Fingerprint: fingerprint,
		EvmAddress:  evmAddress,
	})
	if err != nil {
		return nil, fmt.Errorf("fingerprint mapping creation failed: %w", err)
	}

	// Determine which key to store (user-provided or generated)
	keyToStore := selectKeyToStore(req.CantonPrivateKey, evmKeyPair.PrivateKey)
	if keyToStore == nil {
		return nil, apperrors.BadRequestError(nil, fmt.Sprintf("canton_private_key must be a hex-encoded %d-byte key", cantonKeySize))
	}

	encryptedPKey, err := s.keyCipher.Encrypt(keyToStore)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt key: %w", err)
	}
	regUser := user.New(
		evmAddress,
		req.CantonPartyID,
		fingerprint,
		mapping.ContractID,
		encryptedPKey,
	)

	err = s.store.CreateUser(ctx, regUser)
	if err != nil {
		return nil, fmt.Errorf("failed to save user: %w", err)
	}

	return &user.RegisterResponse{
		Party:       req.CantonPartyID,
		Fingerprint: fingerprint,
		MappingCID:  mapping.ContractID,
		EVMAddress:  evmAddress,
		PrivateKey:  evmKeyPair.PrivateKeyHex(),
	}, nil
}

// Helper methods

// generatePartyHint creates a human-readable party hint from EVM address.
// Uses first 8 characters after "0x" prefix (e.g., "user_12345678").
func generatePartyHint(evmAddress string) string {
	if len(evmAddress) < 2+partyHintLength {
		return "user"
	}
	return fmt.Sprintf("user_%s", evmAddress[2:2+partyHintLength])
}

// isPartyAlreadyAllocatedError checks whether Canton returned a gRPC AlreadyExists status.
func isPartyAlreadyAllocatedError(err error) bool {
	if st, ok := status.FromError(err); ok {
		return st.Code() == codes.AlreadyExists
	}
	return false
}

// selectKeyToStore determines which key to store: user-provided or generated.
// Returns nil if user-provided key is invalid (wrong format or size).
// User-provided keys must be hex-encoded 32-byte secp256k1 private keys.
func selectKeyToStore(userProvidedKey string, generatedKey []byte) []byte {
	if userProvidedKey == "" {
		return generatedKey
	}

	// Decode and validate user-provided key
	raw := strings.TrimPrefix(userProvidedKey, "0x")
	decoded, err := hex.DecodeString(raw)
	if err != nil || len(decoded) != cantonKeySize {
		return nil
	}

	return decoded
}
