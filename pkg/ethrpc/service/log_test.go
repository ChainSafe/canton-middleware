// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	apperr "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc/service"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc/service/mocks"
)

// observedLog wraps a mock Service with the logging decorator, capturing every
// emitted entry so a test can assert the severity chosen for a given error.
func observedLog(t *testing.T) (*mocks.Service, service.Service, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	svc := mocks.NewService(t)
	return svc, service.NewLog(svc, zap.New(core)), logs
}

// TestLogService_Severity locks in that client-side rejections are logged below
// ERROR — read-path probes at Debug, transfer attempts at Warn — while genuine
// server faults stay at ERROR. This keeps routine wallet probing from drowning
// real failures in the logs.
func TestLogService_Severity(t *testing.T) {
	t.Run("Call client error is Debug, not Error", func(t *testing.T) {
		svc, logSvc, logs := observedLog(t)
		svc.EXPECT().Call(mock.Anything, mock.Anything).
			Return(nil, apperr.BadRequestError(nil, "unknown method")).Once()

		_, err := logSvc.Call(context.Background(), nil)
		require.Error(t, err)

		entries := logs.FilterMessage("Call failed").All()
		require.Len(t, entries, 1)
		assert.Equal(t, zapcore.DebugLevel, entries[0].Level)
	})

	t.Run("SendRawTransaction client error is Warn and carries the detail", func(t *testing.T) {
		svc, logSvc, logs := observedLog(t)
		svc.EXPECT().SendRawTransaction(mock.Anything, mock.Anything).
			Return(common.Hash{}, apperr.ForbiddenError(
				errors.New("sender not whitelisted: 0xabc"), "sender not whitelisted: 0xabc")).Once()

		_, err := logSvc.SendRawTransaction(context.Background(), hexutil.Bytes{0x01})
		require.Error(t, err)

		entries := logs.FilterMessage("SendRawTransaction failed").All()
		require.Len(t, entries, 1)
		assert.Equal(t, zapcore.WarnLevel, entries[0].Level)
		// The rejected address must reach the log, not just the generic message.
		assert.Contains(t, entries[0].ContextMap()["error"], "sender not whitelisted: 0xabc")
	})

	t.Run("server fault stays at Error", func(t *testing.T) {
		svc, logSvc, logs := observedLog(t)
		svc.EXPECT().SendRawTransaction(mock.Anything, mock.Anything).
			Return(common.Hash{}, apperr.DependencyError(errors.New("db down"), "insert mempool entry")).Once()

		_, err := logSvc.SendRawTransaction(context.Background(), hexutil.Bytes{0x01})
		require.Error(t, err)

		entries := logs.FilterMessage("SendRawTransaction failed").All()
		require.Len(t, entries, 1)
		assert.Equal(t, zapcore.ErrorLevel, entries[0].Level)
	})
}
