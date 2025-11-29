package canton

import (
	"context"
	"io"

	"github.com/chainsafe/canton-middleware/pkg/canton/lapi"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// MockStateService is a mock implementation of lapi.StateServiceClient
type MockStateService struct {
	lapi.StateServiceClient
	GetActiveContractsFunc func(ctx context.Context, in *lapi.GetActiveContractsRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[lapi.GetActiveContractsResponse], error)
	GetLedgerEndFunc       func(ctx context.Context, in *lapi.GetLedgerEndRequest, opts ...grpc.CallOption) (*lapi.GetLedgerEndResponse, error)
}

func (m *MockStateService) GetActiveContracts(ctx context.Context, in *lapi.GetActiveContractsRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[lapi.GetActiveContractsResponse], error) {
	if m.GetActiveContractsFunc != nil {
		return m.GetActiveContractsFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (m *MockStateService) GetLedgerEnd(ctx context.Context, in *lapi.GetLedgerEndRequest, opts ...grpc.CallOption) (*lapi.GetLedgerEndResponse, error) {
	if m.GetLedgerEndFunc != nil {
		return m.GetLedgerEndFunc(ctx, in, opts...)
	}
	return &lapi.GetLedgerEndResponse{}, nil
}

// MockGetActiveContractsClient is a mock implementation of grpc.ServerStreamingClient[lapi.GetActiveContractsResponse]
type MockGetActiveContractsClient struct {
	grpc.ServerStreamingClient[lapi.GetActiveContractsResponse]
	RecvFunc func() (*lapi.GetActiveContractsResponse, error)
}

func (m *MockGetActiveContractsClient) Recv() (*lapi.GetActiveContractsResponse, error) {
	if m.RecvFunc != nil {
		return m.RecvFunc()
	}
	return nil, io.EOF
}

// MockUpdateService is a mock implementation of lapi.UpdateServiceClient
type MockUpdateService struct {
	lapi.UpdateServiceClient
	GetUpdatesFunc func(ctx context.Context, in *lapi.GetUpdatesRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[lapi.GetUpdatesResponse], error)
}

func (m *MockUpdateService) GetUpdates(ctx context.Context, in *lapi.GetUpdatesRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[lapi.GetUpdatesResponse], error) {
	if m.GetUpdatesFunc != nil {
		return m.GetUpdatesFunc(ctx, in, opts...)
	}
	return nil, nil
}

// MockGetUpdatesClient is a mock implementation of grpc.ServerStreamingClient[lapi.GetUpdatesResponse]
type MockGetUpdatesClient struct {
	grpc.ServerStreamingClient[lapi.GetUpdatesResponse]
	RecvFunc func() (*lapi.GetUpdatesResponse, error)
}

func (m *MockGetUpdatesClient) Recv() (*lapi.GetUpdatesResponse, error) {
	if m.RecvFunc != nil {
		return m.RecvFunc()
	}
	return nil, io.EOF
}

// MockCommandService is a mock implementation of lapi.CommandServiceClient
type MockCommandService struct {
	SubmitAndWaitFunc func(ctx context.Context, in *lapi.SubmitAndWaitRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
}

func (m *MockCommandService) SubmitAndWait(ctx context.Context, in *lapi.SubmitAndWaitRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	if m.SubmitAndWaitFunc != nil {
		return m.SubmitAndWaitFunc(ctx, in, opts...)
	}
	return &emptypb.Empty{}, nil
}

// Implement other methods of CommandServiceClient as needed
func (m *MockCommandService) SubmitAndWaitForUpdateId(ctx context.Context, in *lapi.SubmitAndWaitRequest, opts ...grpc.CallOption) (*lapi.SubmitAndWaitForUpdateIdResponse, error) {
	return nil, nil
}
func (m *MockCommandService) SubmitAndWaitForTransaction(ctx context.Context, in *lapi.SubmitAndWaitRequest, opts ...grpc.CallOption) (*lapi.SubmitAndWaitForTransactionResponse, error) {
	return nil, nil
}
func (m *MockCommandService) SubmitAndWaitForTransactionTree(ctx context.Context, in *lapi.SubmitAndWaitRequest, opts ...grpc.CallOption) (*lapi.SubmitAndWaitForTransactionTreeResponse, error) {
	return nil, nil
}
