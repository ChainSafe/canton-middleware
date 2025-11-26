package canton

import (
	"context"
	"io"

	"github.com/chainsafe/canton-middleware/pkg/canton/lapi"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// MockTransactionService is a mock implementation of lapi.TransactionServiceClient
type MockTransactionService struct {
	GetTransactionsFunc func(ctx context.Context, in *lapi.GetTransactionsRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[lapi.GetTransactionsResponse], error)
	GetLedgerEndFunc    func(ctx context.Context, in *lapi.GetLedgerEndRequest, opts ...grpc.CallOption) (*lapi.GetLedgerEndResponse, error)
}

func (m *MockTransactionService) GetTransactions(ctx context.Context, in *lapi.GetTransactionsRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[lapi.GetTransactionsResponse], error) {
	if m.GetTransactionsFunc != nil {
		return m.GetTransactionsFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (m *MockTransactionService) GetLedgerEnd(ctx context.Context, in *lapi.GetLedgerEndRequest, opts ...grpc.CallOption) (*lapi.GetLedgerEndResponse, error) {
	if m.GetLedgerEndFunc != nil {
		return m.GetLedgerEndFunc(ctx, in, opts...)
	}
	return &lapi.GetLedgerEndResponse{}, nil
}

// Implement other methods of TransactionServiceClient as needed (returning nil/errors for now)
func (m *MockTransactionService) GetTransactionTrees(ctx context.Context, in *lapi.GetTransactionsRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[lapi.GetTransactionTreesResponse], error) {
	return nil, nil
}
func (m *MockTransactionService) GetTransactionByEventId(ctx context.Context, in *lapi.GetTransactionByEventIdRequest, opts ...grpc.CallOption) (*lapi.GetTransactionResponse, error) {
	return nil, nil
}
func (m *MockTransactionService) GetTransactionById(ctx context.Context, in *lapi.GetTransactionByIdRequest, opts ...grpc.CallOption) (*lapi.GetTransactionResponse, error) {
	return nil, nil
}
func (m *MockTransactionService) GetFlatTransactionByEventId(ctx context.Context, in *lapi.GetTransactionByEventIdRequest, opts ...grpc.CallOption) (*lapi.GetFlatTransactionResponse, error) {
	return nil, nil
}
func (m *MockTransactionService) GetFlatTransactionById(ctx context.Context, in *lapi.GetTransactionByIdRequest, opts ...grpc.CallOption) (*lapi.GetFlatTransactionResponse, error) {
	return nil, nil
}
func (m *MockTransactionService) GetLatestPrunedOffsets(ctx context.Context, in *lapi.GetLatestPrunedOffsetsRequest, opts ...grpc.CallOption) (*lapi.GetLatestPrunedOffsetsResponse, error) {
	return nil, nil
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
func (m *MockCommandService) SubmitAndWaitForTransactionId(ctx context.Context, in *lapi.SubmitAndWaitRequest, opts ...grpc.CallOption) (*lapi.SubmitAndWaitForTransactionIdResponse, error) {
	return nil, nil
}
func (m *MockCommandService) SubmitAndWaitForTransaction(ctx context.Context, in *lapi.SubmitAndWaitRequest, opts ...grpc.CallOption) (*lapi.SubmitAndWaitForTransactionResponse, error) {
	return nil, nil
}
func (m *MockCommandService) SubmitAndWaitForTransactionTree(ctx context.Context, in *lapi.SubmitAndWaitRequest, opts ...grpc.CallOption) (*lapi.SubmitAndWaitForTransactionTreeResponse, error) {
	return nil, nil
}

// MockGetTransactionsClient is a mock implementation of grpc.ServerStreamingClient[lapi.GetTransactionsResponse]
type MockGetTransactionsClient struct {
	grpc.ClientStream
	RecvFunc func() (*lapi.GetTransactionsResponse, error)
}

func (m *MockGetTransactionsClient) Recv() (*lapi.GetTransactionsResponse, error) {
	if m.RecvFunc != nil {
		return m.RecvFunc()
	}
	return nil, io.EOF
}
