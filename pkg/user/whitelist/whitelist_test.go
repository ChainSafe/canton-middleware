// SPDX-License-Identifier: Apache-2.0

package whitelist_test

import (
	"context"
	"errors"
	"testing"

	"github.com/chainsafe/canton-middleware/pkg/user/whitelist"
	"github.com/chainsafe/canton-middleware/pkg/user/whitelist/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestIsWhitelisted_SkipAllowsEveryoneWithoutConsultingStore(t *testing.T) {
	// No expectations set: if skip=true ever consults the store, the mock fails
	// on an unexpected call.
	store := mocks.NewStore(t)

	svc := whitelist.New(store, true)

	ok, err := svc.IsWhitelisted(context.Background(), "0xabc")
	require.NoError(t, err)
	assert.True(t, ok, "skip mode must allow every address")
}

func TestIsWhitelisted_EnforceDelegatesToStore(t *testing.T) {
	store := mocks.NewStore(t)
	store.EXPECT().IsWhitelisted(mock.Anything, "0xabc").Return(true, nil).Once()

	svc := whitelist.New(store, false)

	ok, err := svc.IsWhitelisted(context.Background(), "0xabc")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestIsWhitelisted_EnforcePropagatesDenialAndError(t *testing.T) {
	wantErr := errors.New("db down")
	store := mocks.NewStore(t)
	store.EXPECT().IsWhitelisted(mock.Anything, mock.Anything).Return(false, wantErr).Once()

	svc := whitelist.New(store, false)

	ok, err := svc.IsWhitelisted(context.Background(), "0xabc")
	require.ErrorIs(t, err, wantErr)
	assert.False(t, ok)
}
