package client

import (
	"context"
	"strings"
	"time"

	"google.golang.org/grpc"
)

// rpcObserverInterceptor returns a gRPC unary client interceptor that records
// the duration of each Canton ledger RPC call via the provided observer callback.
func rpcObserverInterceptor(obs func(method string, elapsed time.Duration)) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		fullMethod string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		start := time.Now()
		err := invoker(ctx, fullMethod, req, reply, cc, opts...)
		obs(shortMethodName(fullMethod), time.Since(start))
		return err
	}
}

// shortMethodName converts a full gRPC method path to a concise label.
//
//	"/com.digitalasset.canton.v2.CommandService/SubmitAndWait"
//	→ "CommandService/SubmitAndWait"
func shortMethodName(full string) string {
	s := strings.TrimPrefix(full, "/")
	if idx := strings.LastIndex(s, "."); idx >= 0 {
		s = s[idx+1:]
	}
	return s
}
