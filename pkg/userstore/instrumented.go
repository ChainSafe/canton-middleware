package userstore

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/chainsafe/canton-middleware/pkg/user"
)

// InstrumentedStore wraps a Store and records Prometheus metrics for every
// database operation.
type InstrumentedStore struct {
	inner   Store
	metrics *StoreMetrics
}

// NewInstrumentedStore returns a metrics-instrumented wrapper around the given Store.
func NewInstrumentedStore(inner Store, metrics *StoreMetrics) *InstrumentedStore {
	return &InstrumentedStore{inner: inner, metrics: metrics}
}

// Compile-time check that InstrumentedStore implements Store.
var _ Store = (*InstrumentedStore)(nil)

func (s *InstrumentedStore) CreateUser(ctx context.Context, usr *user.User) error {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpCreateUser))
	defer timer.ObserveDuration()

	err := s.inner.CreateUser(ctx, usr)
	if err != nil {
		s.metrics.IncErrors(OpCreateUser)
	}
	return err
}

func (s *InstrumentedStore) GetUserByEVMAddress(ctx context.Context, evmAddress string) (*user.User, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpGetUserByEVMAddress))
	defer timer.ObserveDuration()

	u, err := s.inner.GetUserByEVMAddress(ctx, evmAddress)
	if err != nil {
		s.metrics.IncErrors(OpGetUserByEVMAddress)
	}
	return u, err
}

func (s *InstrumentedStore) GetUserByCantonPartyID(ctx context.Context, partyID string) (*user.User, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpGetUserByCantonPartyID))
	defer timer.ObserveDuration()

	u, err := s.inner.GetUserByCantonPartyID(ctx, partyID)
	if err != nil {
		s.metrics.IncErrors(OpGetUserByCantonPartyID)
	}
	return u, err
}

func (s *InstrumentedStore) GetUserByFingerprint(ctx context.Context, fingerprint string) (*user.User, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpGetUserByFingerprint))
	defer timer.ObserveDuration()

	u, err := s.inner.GetUserByFingerprint(ctx, fingerprint)
	if err != nil {
		s.metrics.IncErrors(OpGetUserByFingerprint)
	}
	return u, err
}

func (s *InstrumentedStore) UserExists(ctx context.Context, evmAddress string) (bool, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpUserExists))
	defer timer.ObserveDuration()

	exists, err := s.inner.UserExists(ctx, evmAddress)
	if err != nil {
		s.metrics.IncErrors(OpUserExists)
	}
	return exists, err
}

func (s *InstrumentedStore) DeleteUser(ctx context.Context, evmAddress string) error {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpDeleteUser))
	defer timer.ObserveDuration()

	err := s.inner.DeleteUser(ctx, evmAddress)
	if err != nil {
		s.metrics.IncErrors(OpDeleteUser)
	}
	return err
}

func (s *InstrumentedStore) ListUsers(ctx context.Context) ([]*user.User, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpListUsers))
	defer timer.ObserveDuration()

	users, err := s.inner.ListUsers(ctx)
	if err != nil {
		s.metrics.IncErrors(OpListUsers)
	}
	return users, err
}

func (s *InstrumentedStore) IsWhitelisted(ctx context.Context, evmAddress string) (bool, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpIsWhitelisted))
	defer timer.ObserveDuration()

	ok, err := s.inner.IsWhitelisted(ctx, evmAddress)
	if err != nil {
		s.metrics.IncErrors(OpIsWhitelisted)
	}
	return ok, err
}

func (s *InstrumentedStore) AddToWhitelist(ctx context.Context, evmAddress, note string) error {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpAddToWhitelist))
	defer timer.ObserveDuration()

	err := s.inner.AddToWhitelist(ctx, evmAddress, note)
	if err != nil {
		s.metrics.IncErrors(OpAddToWhitelist)
	}
	return err
}

func (s *InstrumentedStore) GetUserKeyByCantonPartyID(
	ctx context.Context, decryptor KeyDecryptor, partyID string,
) ([]byte, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpGetKeyByCantonPartyID))
	defer timer.ObserveDuration()

	key, err := s.inner.GetUserKeyByCantonPartyID(ctx, decryptor, partyID)
	if err != nil {
		s.metrics.IncErrors(OpGetKeyByCantonPartyID)
	}
	return key, err
}

func (s *InstrumentedStore) GetUserKeyByEVMAddress(
	ctx context.Context, decryptor KeyDecryptor, evmAddress string,
) ([]byte, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpGetKeyByEVMAddress))
	defer timer.ObserveDuration()

	key, err := s.inner.GetUserKeyByEVMAddress(ctx, decryptor, evmAddress)
	if err != nil {
		s.metrics.IncErrors(OpGetKeyByEVMAddress)
	}
	return key, err
}

func (s *InstrumentedStore) GetUserKeyByFingerprint(
	ctx context.Context, decryptor KeyDecryptor, fingerprint string,
) ([]byte, error) {
	timer := prometheus.NewTimer(s.metrics.ObserveQueryDuration(OpGetKeyByFingerprint))
	defer timer.ObserveDuration()

	key, err := s.inner.GetUserKeyByFingerprint(ctx, decryptor, fingerprint)
	if err != nil {
		s.metrics.IncErrors(OpGetKeyByFingerprint)
	}
	return key, err
}
