package canton

import (
	"context"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
)

// MockStateService is a mock implementation of lapi.StateServiceClient
type MockStateService struct {
	GetActiveContractsFunc  func(ctx context.Context, in *lapiv2.GetActiveContractsRequest, opts ...grpc.CallOption) (lapiv2.StateService_GetActiveContractsClient, error)
	GetLedgerEndFunc        func(ctx context.Context, in *lapiv2.GetLedgerEndRequest, opts ...grpc.CallOption) (*lapiv2.GetLedgerEndResponse, error)
	GetConnectedDomainsFunc func(ctx context.Context, in *lapiv2.GetConnectedDomainsRequest, opts ...grpc.CallOption) (*lapiv2.GetConnectedDomainsResponse, error)
}

func (m *MockStateService) GetActiveContracts(ctx context.Context, in *lapiv2.GetActiveContractsRequest, opts ...grpc.CallOption) (lapiv2.StateService_GetActiveContractsClient, error) {
	if m.GetActiveContractsFunc != nil {
		return m.GetActiveContractsFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (m *MockStateService) GetLedgerEnd(ctx context.Context, in *lapiv2.GetLedgerEndRequest, opts ...grpc.CallOption) (*lapiv2.GetLedgerEndResponse, error) {
	if m.GetLedgerEndFunc != nil {
		return m.GetLedgerEndFunc(ctx, in, opts...)
	}
	return &lapiv2.GetLedgerEndResponse{}, nil
}

func (m *MockStateService) GetConnectedDomains(ctx context.Context, in *lapiv2.GetConnectedDomainsRequest, opts ...grpc.CallOption) (*lapiv2.GetConnectedDomainsResponse, error) {
	if m.GetConnectedDomainsFunc != nil {
		return m.GetConnectedDomainsFunc(ctx, in, opts...)
	}
	return &lapiv2.GetConnectedDomainsResponse{}, nil
}

// MockUpdateService is a mock implementation of lapi.UpdateServiceClient
type MockUpdateService struct {
	GetUpdatesFunc              func(ctx context.Context, in *lapiv2.GetUpdatesRequest, opts ...grpc.CallOption) (lapiv2.UpdateService_GetUpdatesClient, error)
	GetUpdateTreesFunc          func(ctx context.Context, in *lapiv2.GetUpdatesRequest, opts ...grpc.CallOption) (lapiv2.UpdateService_GetUpdateTreesClient, error)
	GetTransactionByEventIdFunc func(ctx context.Context, in *lapiv2.GetTransactionByEventIdRequest, opts ...grpc.CallOption) (*lapiv2.GetTransactionResponse, error)
	GetTransactionByIdFunc      func(ctx context.Context, in *lapiv2.GetTransactionByIdRequest, opts ...grpc.CallOption) (*lapiv2.GetTransactionResponse, error)
}

func (m *MockUpdateService) GetUpdates(ctx context.Context, in *lapiv2.GetUpdatesRequest, opts ...grpc.CallOption) (lapiv2.UpdateService_GetUpdatesClient, error) {
	if m.GetUpdatesFunc != nil {
		return m.GetUpdatesFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (m *MockUpdateService) GetUpdateTrees(ctx context.Context, in *lapiv2.GetUpdatesRequest, opts ...grpc.CallOption) (lapiv2.UpdateService_GetUpdateTreesClient, error) {
	if m.GetUpdateTreesFunc != nil {
		return m.GetUpdateTreesFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (m *MockUpdateService) GetTransactionByEventId(ctx context.Context, in *lapiv2.GetTransactionByEventIdRequest, opts ...grpc.CallOption) (*lapiv2.GetTransactionResponse, error) {
	return nil, nil
}

func (m *MockUpdateService) GetTransactionById(ctx context.Context, in *lapiv2.GetTransactionByIdRequest, opts ...grpc.CallOption) (*lapiv2.GetTransactionResponse, error) {
	return nil, nil
}

// MockCommandService is a mock implementation of lapi.CommandServiceClient
type MockCommandService struct {
	SubmitAndWaitFunc func(ctx context.Context, in *lapiv2.SubmitAndWaitRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
}

func (m *MockCommandService) SubmitAndWait(ctx context.Context, in *lapiv2.SubmitAndWaitRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	if m.SubmitAndWaitFunc != nil {
		return m.SubmitAndWaitFunc(ctx, in, opts...)
	}
	return &emptypb.Empty{}, nil
}

// Implement other methods of CommandServiceClient as needed
func (m *MockCommandService) SubmitAndWaitForUpdateId(ctx context.Context, in *lapiv2.SubmitAndWaitRequest, opts ...grpc.CallOption) (*lapiv2.SubmitAndWaitForUpdateIdResponse, error) {
	return nil, nil
}
func (m *MockCommandService) SubmitAndWaitForTransaction(ctx context.Context, in *lapiv2.SubmitAndWaitRequest, opts ...grpc.CallOption) (*lapiv2.SubmitAndWaitForTransactionResponse, error) {
	return nil, nil
}

func (m *MockCommandService) SubmitAndWaitForTransactionTree(ctx context.Context, in *lapiv2.SubmitAndWaitRequest, opts ...grpc.CallOption) (*lapiv2.SubmitAndWaitForTransactionTreeResponse, error) {
	return nil, nil
}

func (m *MockStateService) GetLatestPrunedOffsets(ctx context.Context, in *lapiv2.GetLatestPrunedOffsetsRequest, opts ...grpc.CallOption) (*lapiv2.GetLatestPrunedOffsetsResponse, error) {
	return &lapiv2.GetLatestPrunedOffsetsResponse{}, nil
}

func (m *MockUpdateService) GetTransactionTreeByEventId(ctx context.Context, in *lapiv2.GetTransactionByEventIdRequest, opts ...grpc.CallOption) (*lapiv2.GetTransactionTreeResponse, error) {
	return nil, nil
}

func (m *MockUpdateService) GetTransactionTreeById(ctx context.Context, in *lapiv2.GetTransactionByIdRequest, opts ...grpc.CallOption) (*lapiv2.GetTransactionTreeResponse, error) {
	return nil, nil
}

// MockGetActiveContractsClient is a mock implementation of grpc.ServerStreamingClient[lapiv2.GetActiveContractsResponse]
type MockGetActiveContractsClient struct {
	grpc.ClientStream
	RecvFunc func() (*lapiv2.GetActiveContractsResponse, error)
}

func (m *MockGetActiveContractsClient) Recv() (*lapiv2.GetActiveContractsResponse, error) {
	if m.RecvFunc != nil {
		return m.RecvFunc()
	}
	return nil, io.EOF
}

// MockGetUpdatesClient is a mock implementation of grpc.ServerStreamingClient[lapiv2.GetUpdatesResponse]
type MockGetUpdatesClient struct {
	grpc.ClientStream
	RecvFunc func() (*lapiv2.GetUpdatesResponse, error)
}

func (m *MockGetUpdatesClient) Recv() (*lapiv2.GetUpdatesResponse, error) {
	if m.RecvFunc != nil {
		return m.RecvFunc()
	}
	return nil, io.EOF
}
