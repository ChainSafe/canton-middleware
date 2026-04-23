package service

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"go.uber.org/zap"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/chainsafe/canton-middleware/pkg/user"
	"github.com/chainsafe/canton-middleware/pkg/user/service/mocks"
)

const testMessage = "register-me"

func signEIP191Message(t *testing.T) (string, string) {
	t.Helper()

	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() failed: %v", err)
	}

	prefixedMessage := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(testMessage), testMessage)
	hash := crypto.Keccak256Hash([]byte(prefixedMessage))

	signature, err := crypto.Sign(hash.Bytes(), privateKey)
	if err != nil {
		t.Fatalf("Sign() failed: %v", err)
	}

	address := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	return auth.NormalizeAddress(address), "0x" + hex.EncodeToString(signature)
}

func TestRegistrationService_RegisterWeb3User_UserAlreadyRegistered(t *testing.T) {
	ctx := context.Background()
	evmAddress, signature := signEIP191Message(t)

	storeMock := mocks.NewStore(t)
	storeMock.EXPECT().IsWhitelisted(ctx, evmAddress).Return(true, nil).Once()
	storeMock.EXPECT().UserExists(ctx, evmAddress).Return(true, nil).Once()

	svc := NewService(storeMock, nil, nil, zap.NewNop(), false, false, nil)

	_, err := svc.RegisterWeb3User(ctx, &user.RegisterRequest{
		Message:   testMessage,
		Signature: signature,
	})
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	if !errors.Is(err, ErrUserAlreadyRegistered) {
		t.Fatalf("expected ErrUserAlreadyRegistered, got %v", err)
	}
	if !apperrors.Is(err, apperrors.CategoryDataConflict) {
		t.Fatalf("expected CategoryDataConflict, got %v", err)
	}
}

func TestRegistrationService_RegisterWeb3User_NotWhitelisted(t *testing.T) {
	ctx := context.Background()
	evmAddress, signature := signEIP191Message(t)

	storeMock := mocks.NewStore(t)
	storeMock.EXPECT().IsWhitelisted(ctx, evmAddress).Return(false, nil).Once()

	svc := NewService(storeMock, nil, nil, zap.NewNop(), false, false, nil)

	_, err := svc.RegisterWeb3User(ctx, &user.RegisterRequest{
		Message:   testMessage,
		Signature: signature,
	})
	if err == nil {
		t.Fatal("expected forbidden error, got nil")
	}
	if !errors.Is(err, ErrNotWhitelisted) {
		t.Fatalf("expected ErrNotWhitelisted, got %v", err)
	}
	if !apperrors.Is(err, apperrors.CategoryForbidden) {
		t.Fatalf("expected CategoryForbidden, got %v", err)
	}
}

func TestPrepareExternalRegistration_UserAlreadyExists(t *testing.T) {
	ctx := context.Background()
	evmAddress, signature := signEIP191Message(t)

	storeMock := mocks.NewStore(t)
	storeMock.EXPECT().UserExists(ctx, evmAddress).Return(true, nil).Once()

	svc := NewService(storeMock, nil, nil, zap.NewNop(), false, false, nil)

	_, err := svc.PrepareExternalRegistration(ctx, &user.RegisterRequest{
		Message:         testMessage,
		Signature:       signature,
		CantonPublicKey: "02deadbeef",
	})
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	if !errors.Is(err, ErrUserAlreadyRegistered) {
		t.Fatalf("expected ErrUserAlreadyRegistered, got %v", err)
	}
	if !apperrors.Is(err, apperrors.CategoryDataConflict) {
		t.Fatalf("expected CategoryDataConflict, got %v", err)
	}
}

func TestPrepareExternalRegistration_NotWhitelisted(t *testing.T) {
	ctx := context.Background()
	evmAddress, signature := signEIP191Message(t)

	storeMock := mocks.NewStore(t)
	storeMock.EXPECT().UserExists(ctx, evmAddress).Return(false, nil).Once()
	storeMock.EXPECT().IsWhitelisted(ctx, evmAddress).Return(false, nil).Once()

	svc := NewService(storeMock, nil, nil, zap.NewNop(), false, false, nil)

	_, err := svc.PrepareExternalRegistration(ctx, &user.RegisterRequest{
		Message:         testMessage,
		Signature:       signature,
		CantonPublicKey: "02deadbeef",
	})
	if err == nil {
		t.Fatal("expected forbidden error, got nil")
	}
	if !errors.Is(err, ErrNotWhitelisted) {
		t.Fatalf("expected ErrNotWhitelisted, got %v", err)
	}
	if !apperrors.Is(err, apperrors.CategoryForbidden) {
		t.Fatalf("expected CategoryForbidden, got %v", err)
	}
}

func TestRegistrationService_RegisterCantonNativeUser_StoreError(t *testing.T) {
	ctx := context.Background()
	partyID := "party::aabb"
	storeErr := errors.New("db unavailable")

	storeMock := mocks.NewStore(t)
	storeMock.EXPECT().GetUserByCantonPartyID(ctx, partyID).Return(nil, storeErr).Once()

	svc := NewService(storeMock, nil, nil, zap.NewNop(), true, false, nil)

	_, err := svc.RegisterCantonNativeUser(ctx, &user.RegisterRequest{
		CantonPartyID: partyID,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to check user existence") {
		t.Fatalf("expected wrapped user-existence error, got %v", err)
	}
	if !errors.Is(err, storeErr) {
		t.Fatalf("expected store error to be wrapped, got %v", err)
	}
}

func TestRegistrationService_RegisterCantonNativeUser_PartyAlreadyRegistered(t *testing.T) {
	ctx := context.Background()
	partyID := "party::aabb"

	storeMock := mocks.NewStore(t)
	storeMock.EXPECT().GetUserByCantonPartyID(ctx, partyID).Return(&user.User{CantonPartyID: partyID}, nil).Once()

	svc := NewService(storeMock, nil, nil, zap.NewNop(), true, false, nil)

	_, err := svc.RegisterCantonNativeUser(ctx, &user.RegisterRequest{
		CantonPartyID: partyID,
	})
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	if !errors.Is(err, ErrPartyAlreadyRegistered) {
		t.Fatalf("expected ErrPartyAlreadyRegistered, got %v", err)
	}
	if !apperrors.Is(err, apperrors.CategoryDataConflict) {
		t.Fatalf("expected CategoryDataConflict, got %v", err)
	}
}
