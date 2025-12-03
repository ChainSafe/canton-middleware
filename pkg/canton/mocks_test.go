package canton

import (
	"context"
	"io"

	"google.golang.org/grpc"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
)

// MockStateService is a mock implementation of lapi.StateServiceClient
type MockStateService struct {
	GetActiveContractsFunc        func(ctx context.Context, in *lapiv2.GetActiveContractsRequest, opts ...grpc.CallOption) (lapiv2.StateService_GetActiveContractsClient, error)
	GetLedgerEndFunc              func(ctx context.Context, in *lapiv2.GetLedgerEndRequest, opts ...grpc.CallOption) (*lapiv2.GetLedgerEndResponse, error)
	GetConnectedSynchronizersFunc func(ctx context.Context, in *lapiv2.GetConnectedSynchronizersRequest, opts ...grpc.CallOption) (*lapiv2.GetConnectedSynchronizersResponse, error)
	GetLatestPrunedOffsetsFunc    func(ctx context.Context, in *lapiv2.GetLatestPrunedOffsetsRequest, opts ...grpc.CallOption) (*lapiv2.GetLatestPrunedOffsetsResponse, error)
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

func (m *MockStateService) GetConnectedSynchronizers(ctx context.Context, in *lapiv2.GetConnectedSynchronizersRequest, opts ...grpc.CallOption) (*lapiv2.GetConnectedSynchronizersResponse, error) {
	if m.GetConnectedSynchronizersFunc != nil {
		return m.GetConnectedSynchronizersFunc(ctx, in, opts...)
	}
	return &lapiv2.GetConnectedSynchronizersResponse{}, nil
}

func (m *MockStateService) GetLatestPrunedOffsets(ctx context.Context, in *lapiv2.GetLatestPrunedOffsetsRequest, opts ...grpc.CallOption) (*lapiv2.GetLatestPrunedOffsetsResponse, error) {
	if m.GetLatestPrunedOffsetsFunc != nil {
		return m.GetLatestPrunedOffsetsFunc(ctx, in, opts...)
	}
	return &lapiv2.GetLatestPrunedOffsetsResponse{}, nil
}

// MockUpdateService is a mock implementation of lapi.UpdateServiceClient
type MockUpdateService struct {
	GetUpdatesFunc        func(ctx context.Context, in *lapiv2.GetUpdatesRequest, opts ...grpc.CallOption) (lapiv2.UpdateService_GetUpdatesClient, error)
	GetUpdateByOffsetFunc func(ctx context.Context, in *lapiv2.GetUpdateByOffsetRequest, opts ...grpc.CallOption) (*lapiv2.GetUpdateResponse, error)
	GetUpdateByIdFunc     func(ctx context.Context, in *lapiv2.GetUpdateByIdRequest, opts ...grpc.CallOption) (*lapiv2.GetUpdateResponse, error)
}

func (m *MockUpdateService) GetUpdates(ctx context.Context, in *lapiv2.GetUpdatesRequest, opts ...grpc.CallOption) (lapiv2.UpdateService_GetUpdatesClient, error) {
	if m.GetUpdatesFunc != nil {
		return m.GetUpdatesFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (m *MockUpdateService) GetUpdateByOffset(ctx context.Context, in *lapiv2.GetUpdateByOffsetRequest, opts ...grpc.CallOption) (*lapiv2.GetUpdateResponse, error) {
	if m.GetUpdateByOffsetFunc != nil {
		return m.GetUpdateByOffsetFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (m *MockUpdateService) GetUpdateById(ctx context.Context, in *lapiv2.GetUpdateByIdRequest, opts ...grpc.CallOption) (*lapiv2.GetUpdateResponse, error) {
	if m.GetUpdateByIdFunc != nil {
		return m.GetUpdateByIdFunc(ctx, in, opts...)
	}
	return nil, nil
}

// MockCommandService is a mock implementation of lapi.CommandServiceClient
type MockCommandService struct {
	SubmitAndWaitFunc                func(ctx context.Context, in *lapiv2.SubmitAndWaitRequest, opts ...grpc.CallOption) (*lapiv2.SubmitAndWaitResponse, error)
	SubmitAndWaitForTransactionFunc  func(ctx context.Context, in *lapiv2.SubmitAndWaitForTransactionRequest, opts ...grpc.CallOption) (*lapiv2.SubmitAndWaitForTransactionResponse, error)
	SubmitAndWaitForReassignmentFunc func(ctx context.Context, in *lapiv2.SubmitAndWaitForReassignmentRequest, opts ...grpc.CallOption) (*lapiv2.SubmitAndWaitForReassignmentResponse, error)
}

func (m *MockCommandService) SubmitAndWait(ctx context.Context, in *lapiv2.SubmitAndWaitRequest, opts ...grpc.CallOption) (*lapiv2.SubmitAndWaitResponse, error) {
	if m.SubmitAndWaitFunc != nil {
		return m.SubmitAndWaitFunc(ctx, in, opts...)
	}
	return &lapiv2.SubmitAndWaitResponse{}, nil
}

func (m *MockCommandService) SubmitAndWaitForTransaction(ctx context.Context, in *lapiv2.SubmitAndWaitForTransactionRequest, opts ...grpc.CallOption) (*lapiv2.SubmitAndWaitForTransactionResponse, error) {
	if m.SubmitAndWaitForTransactionFunc != nil {
		return m.SubmitAndWaitForTransactionFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (m *MockCommandService) SubmitAndWaitForReassignment(ctx context.Context, in *lapiv2.SubmitAndWaitForReassignmentRequest, opts ...grpc.CallOption) (*lapiv2.SubmitAndWaitForReassignmentResponse, error) {
	if m.SubmitAndWaitForReassignmentFunc != nil {
		return m.SubmitAndWaitForReassignmentFunc(ctx, in, opts...)
	}
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
