package grpcapi

import (
	"context"
	"testing"

	"github.com/ryan-wong-coder/trustdb/internal/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNewServerNormalizesTypedNilGlobalService(t *testing.T) {
	t.Parallel()

	var global *typedNilGlobalService
	server := NewServer(nil, nil, global, nil, nil)
	if server.Global != nil {
		t.Fatalf("server.Global = %#v, want nil", server.Global)
	}
	_, err := server.GetGlobalProof(context.Background(), &GetGlobalProofRequest{BatchID: "batch-1"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("GetGlobalProof status = %v, want %v err=%v", status.Code(err), codes.FailedPrecondition, err)
	}
}

type typedNilGlobalService struct{}

func (*typedNilGlobalService) LatestSTH(context.Context) (model.SignedTreeHead, bool, error) {
	panic("typed nil global service should be normalized before use")
}

func (*typedNilGlobalService) STH(context.Context, uint64) (model.SignedTreeHead, bool, error) {
	panic("typed nil global service should be normalized before use")
}

func (*typedNilGlobalService) ListSTHs(context.Context, model.TreeHeadListOptions) ([]model.SignedTreeHead, error) {
	panic("typed nil global service should be normalized before use")
}

func (*typedNilGlobalService) ListLeaves(context.Context, model.GlobalLeafListOptions) ([]model.GlobalLogLeaf, error) {
	panic("typed nil global service should be normalized before use")
}

func (*typedNilGlobalService) State(context.Context) (model.GlobalLogState, bool, error) {
	panic("typed nil global service should be normalized before use")
}

func (*typedNilGlobalService) Node(context.Context, uint64, uint64) (model.GlobalLogNode, bool, error) {
	panic("typed nil global service should be normalized before use")
}

func (*typedNilGlobalService) ListNodesAfter(context.Context, uint64, uint64, int) ([]model.GlobalLogNode, error) {
	panic("typed nil global service should be normalized before use")
}

func (*typedNilGlobalService) InclusionProof(context.Context, string, uint64) (model.GlobalLogProof, error) {
	panic("typed nil global service should be normalized before use")
}

func (*typedNilGlobalService) ConsistencyProof(context.Context, uint64, uint64) (model.GlobalConsistencyProof, error) {
	panic("typed nil global service should be normalized before use")
}
