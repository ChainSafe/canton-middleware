package auth

import (
	"context"
)

// Context keys for authentication data
type contextKey string

const (
	// ContextKeyEVMAddress is the context key for the authenticated EVM address
	ContextKeyEVMAddress contextKey = "evm_address"
	// ContextKeyCantonParty is the context key for the user's Canton party
	ContextKeyCantonParty contextKey = "canton_party"
	// ContextKeyFingerprint is the context key for the user's fingerprint
	ContextKeyFingerprint contextKey = "fingerprint"
	// ContextKeyUserID is the context key for the user's database ID
	ContextKeyUserID contextKey = "user_id"
)

// WithEVMAddress adds the EVM address to the context
func WithEVMAddress(ctx context.Context, address string) context.Context {
	return context.WithValue(ctx, ContextKeyEVMAddress, address)
}

// EVMAddressFromContext retrieves the EVM address from the context
func EVMAddressFromContext(ctx context.Context) (string, bool) {
	addr, ok := ctx.Value(ContextKeyEVMAddress).(string)
	return addr, ok
}

// WithCantonParty adds the Canton party to the context
func WithCantonParty(ctx context.Context, party string) context.Context {
	return context.WithValue(ctx, ContextKeyCantonParty, party)
}

// CantonPartyFromContext retrieves the Canton party from the context
func CantonPartyFromContext(ctx context.Context) (string, bool) {
	party, ok := ctx.Value(ContextKeyCantonParty).(string)
	return party, ok
}

// WithFingerprint adds the fingerprint to the context
func WithFingerprint(ctx context.Context, fingerprint string) context.Context {
	return context.WithValue(ctx, ContextKeyFingerprint, fingerprint)
}

// FingerprintFromContext retrieves the fingerprint from the context
func FingerprintFromContext(ctx context.Context) (string, bool) {
	fp, ok := ctx.Value(ContextKeyFingerprint).(string)
	return fp, ok
}

// WithUserID adds the user ID to the context
func WithUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, ContextKeyUserID, userID)
}

// UserIDFromContext retrieves the user ID from the context
func UserIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(ContextKeyUserID).(int64)
	return id, ok
}

// AuthInfo contains all authentication information for a request
type AuthInfo struct {
	EVMAddress  string
	CantonParty string
	Fingerprint string
	UserID      int64
}

// WithAuthInfo adds all authentication info to the context
func WithAuthInfo(ctx context.Context, info *AuthInfo) context.Context {
	ctx = WithEVMAddress(ctx, info.EVMAddress)
	ctx = WithCantonParty(ctx, info.CantonParty)
	ctx = WithFingerprint(ctx, info.Fingerprint)
	ctx = WithUserID(ctx, info.UserID)
	return ctx
}

// AuthInfoFromContext retrieves all authentication info from the context
func AuthInfoFromContext(ctx context.Context) *AuthInfo {
	info := &AuthInfo{}
	info.EVMAddress, _ = EVMAddressFromContext(ctx)
	info.CantonParty, _ = CantonPartyFromContext(ctx)
	info.Fingerprint, _ = FingerprintFromContext(ctx)
	info.UserID, _ = UserIDFromContext(ctx)
	return info
}

