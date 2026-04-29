package grpcapi

import (
	"context"

	"google.golang.org/grpc"
)

type TrustDBServiceServer interface {
	Health(context.Context, *HealthRequest) (*HealthResponse, error)
	SubmitClaim(context.Context, *SubmitClaimRequest) (*SubmitClaimResponse, error)
	GetRecord(context.Context, *GetRecordRequest) (*GetRecordResponse, error)
	ListRecords(context.Context, *ListRecordsRequest) (*ListRecordsResponse, error)
	GetProofBundle(context.Context, *GetProofBundleRequest) (*GetProofBundleResponse, error)
	ListRoots(context.Context, *ListRootsRequest) (*ListRootsResponse, error)
	LatestRoot(context.Context, *LatestRootRequest) (*LatestRootResponse, error)
	ListSTHs(context.Context, *ListSTHsRequest) (*ListSTHsResponse, error)
	LatestSTH(context.Context, *LatestSTHRequest) (*LatestSTHResponse, error)
	GetSTH(context.Context, *GetSTHRequest) (*GetSTHResponse, error)
	ListGlobalLeaves(context.Context, *ListGlobalLeavesRequest) (*ListGlobalLeavesResponse, error)
	GetGlobalProof(context.Context, *GetGlobalProofRequest) (*GetGlobalProofResponse, error)
	ListAnchors(context.Context, *ListAnchorsRequest) (*ListAnchorsResponse, error)
	GetAnchor(context.Context, *GetAnchorRequest) (*GetAnchorResponse, error)
	Metrics(context.Context, *MetricsRequest) (*MetricsResponse, error)
}

func RegisterTrustDBServiceServer(s grpc.ServiceRegistrar, srv TrustDBServiceServer) {
	s.RegisterService(&TrustDBService_ServiceDesc, srv)
}

var TrustDBService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: ServiceName,
	HandlerType: (*TrustDBServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{MethodName: "Health", Handler: _TrustDB_Health_Handler},
		{MethodName: "SubmitClaim", Handler: _TrustDB_SubmitClaim_Handler},
		{MethodName: "GetRecord", Handler: _TrustDB_GetRecord_Handler},
		{MethodName: "ListRecords", Handler: _TrustDB_ListRecords_Handler},
		{MethodName: "GetProofBundle", Handler: _TrustDB_GetProofBundle_Handler},
		{MethodName: "ListRoots", Handler: _TrustDB_ListRoots_Handler},
		{MethodName: "LatestRoot", Handler: _TrustDB_LatestRoot_Handler},
		{MethodName: "ListSTHs", Handler: _TrustDB_ListSTHs_Handler},
		{MethodName: "LatestSTH", Handler: _TrustDB_LatestSTH_Handler},
		{MethodName: "GetSTH", Handler: _TrustDB_GetSTH_Handler},
		{MethodName: "ListGlobalLeaves", Handler: _TrustDB_ListGlobalLeaves_Handler},
		{MethodName: "GetGlobalProof", Handler: _TrustDB_GetGlobalProof_Handler},
		{MethodName: "ListAnchors", Handler: _TrustDB_ListAnchors_Handler},
		{MethodName: "GetAnchor", Handler: _TrustDB_GetAnchor_Handler},
		{MethodName: "Metrics", Handler: _TrustDB_Metrics_Handler},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "trustdb/v1/cbor",
}

func unaryHandler[Req any](
	srv any,
	ctx context.Context,
	dec func(any) error,
	interceptor grpc.UnaryServerInterceptor,
	method string,
	call func(TrustDBServiceServer, context.Context, *Req) (any, error),
) (any, error) {
	in := new(Req)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return call(srv.(TrustDBServiceServer), ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/" + ServiceName + "/" + method}
	handler := func(ctx context.Context, req any) (any, error) {
		return call(srv.(TrustDBServiceServer), ctx, req.(*Req))
	}
	return interceptor(ctx, in, info, handler)
}

func _TrustDB_Health_Handler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return unaryHandler[HealthRequest](srv, ctx, dec, interceptor, "Health", func(s TrustDBServiceServer, ctx context.Context, req *HealthRequest) (any, error) {
		return s.Health(ctx, req)
	})
}

func _TrustDB_SubmitClaim_Handler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return unaryHandler[SubmitClaimRequest](srv, ctx, dec, interceptor, "SubmitClaim", func(s TrustDBServiceServer, ctx context.Context, req *SubmitClaimRequest) (any, error) {
		return s.SubmitClaim(ctx, req)
	})
}

func _TrustDB_GetRecord_Handler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return unaryHandler[GetRecordRequest](srv, ctx, dec, interceptor, "GetRecord", func(s TrustDBServiceServer, ctx context.Context, req *GetRecordRequest) (any, error) {
		return s.GetRecord(ctx, req)
	})
}

func _TrustDB_ListRecords_Handler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return unaryHandler[ListRecordsRequest](srv, ctx, dec, interceptor, "ListRecords", func(s TrustDBServiceServer, ctx context.Context, req *ListRecordsRequest) (any, error) {
		return s.ListRecords(ctx, req)
	})
}

func _TrustDB_GetProofBundle_Handler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return unaryHandler[GetProofBundleRequest](srv, ctx, dec, interceptor, "GetProofBundle", func(s TrustDBServiceServer, ctx context.Context, req *GetProofBundleRequest) (any, error) {
		return s.GetProofBundle(ctx, req)
	})
}

func _TrustDB_ListRoots_Handler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return unaryHandler[ListRootsRequest](srv, ctx, dec, interceptor, "ListRoots", func(s TrustDBServiceServer, ctx context.Context, req *ListRootsRequest) (any, error) {
		return s.ListRoots(ctx, req)
	})
}

func _TrustDB_LatestRoot_Handler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return unaryHandler[LatestRootRequest](srv, ctx, dec, interceptor, "LatestRoot", func(s TrustDBServiceServer, ctx context.Context, req *LatestRootRequest) (any, error) {
		return s.LatestRoot(ctx, req)
	})
}

func _TrustDB_ListSTHs_Handler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return unaryHandler[ListSTHsRequest](srv, ctx, dec, interceptor, "ListSTHs", func(s TrustDBServiceServer, ctx context.Context, req *ListSTHsRequest) (any, error) {
		return s.ListSTHs(ctx, req)
	})
}

func _TrustDB_LatestSTH_Handler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return unaryHandler[LatestSTHRequest](srv, ctx, dec, interceptor, "LatestSTH", func(s TrustDBServiceServer, ctx context.Context, req *LatestSTHRequest) (any, error) {
		return s.LatestSTH(ctx, req)
	})
}

func _TrustDB_GetSTH_Handler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return unaryHandler[GetSTHRequest](srv, ctx, dec, interceptor, "GetSTH", func(s TrustDBServiceServer, ctx context.Context, req *GetSTHRequest) (any, error) {
		return s.GetSTH(ctx, req)
	})
}

func _TrustDB_ListGlobalLeaves_Handler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return unaryHandler[ListGlobalLeavesRequest](srv, ctx, dec, interceptor, "ListGlobalLeaves", func(s TrustDBServiceServer, ctx context.Context, req *ListGlobalLeavesRequest) (any, error) {
		return s.ListGlobalLeaves(ctx, req)
	})
}

func _TrustDB_GetGlobalProof_Handler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return unaryHandler[GetGlobalProofRequest](srv, ctx, dec, interceptor, "GetGlobalProof", func(s TrustDBServiceServer, ctx context.Context, req *GetGlobalProofRequest) (any, error) {
		return s.GetGlobalProof(ctx, req)
	})
}

func _TrustDB_ListAnchors_Handler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return unaryHandler[ListAnchorsRequest](srv, ctx, dec, interceptor, "ListAnchors", func(s TrustDBServiceServer, ctx context.Context, req *ListAnchorsRequest) (any, error) {
		return s.ListAnchors(ctx, req)
	})
}

func _TrustDB_GetAnchor_Handler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return unaryHandler[GetAnchorRequest](srv, ctx, dec, interceptor, "GetAnchor", func(s TrustDBServiceServer, ctx context.Context, req *GetAnchorRequest) (any, error) {
		return s.GetAnchor(ctx, req)
	})
}

func _TrustDB_Metrics_Handler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return unaryHandler[MetricsRequest](srv, ctx, dec, interceptor, "Metrics", func(s TrustDBServiceServer, ctx context.Context, req *MetricsRequest) (any, error) {
		return s.Metrics(ctx, req)
	})
}
