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

func TestNew_SkipAllowsEveryoneWithoutConsultingSource(t *testing.T) {
	// No expectations set: if New(skip=true) ever consults the source, the mock
	// fails on an unexpected call.
	src := mocks.NewChecker(t)

	c := whitelist.New(src, true)

	ok, err := c.IsWhitelisted(context.Background(), "0xabc")
	require.NoError(t, err)
	assert.True(t, ok, "skip mode must allow every address")
}

func TestNew_EnforceDelegatesToSource(t *testing.T) {
	src := mocks.NewChecker(t)
	src.EXPECT().IsWhitelisted(mock.Anything, "0xabc").Return(true, nil).Once()

	c := whitelist.New(src, false)

	ok, err := c.IsWhitelisted(context.Background(), "0xabc")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestNew_EnforcePropagatesDenialAndError(t *testing.T) {
	wantErr := errors.New("db down")
	src := mocks.NewChecker(t)
	src.EXPECT().IsWhitelisted(mock.Anything, mock.Anything).Return(false, wantErr).Once()

	c := whitelist.New(src, false)

	ok, err := c.IsWhitelisted(context.Background(), "0xabc")
	require.ErrorIs(t, err, wantErr)
	assert.False(t, ok)
}
