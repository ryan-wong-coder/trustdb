package sdk

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/ryan-wong-coder/trustdb/internal/grpcapi"
	"github.com/ryan-wong-coder/trustdb/internal/model"
	"github.com/ryan-wong-coder/trustdb/internal/trusterr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestGRPCTransportOperationalEndpoints(t *testing.T) {
	t.Parallel()

	client := newBufconnClient(t, grpcapi.NewServer(nil, grpcTestBatch{}, nil, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("trustdb_ingest_total 1\n"))
	})))

	if status := client.CheckHealth(context.Background()); !status.OK || status.ServerURL != "bufnet" {
		t.Fatalf("health status = %+v", status)
	}
	record, err := client.GetRecord(context.Background(), "tr1record")
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}
	if record.RecordID != "tr1record" || record.BatchID != "batch-1" {
		t.Fatalf("record = %+v", record)
	}
	page, err := client.ListRecords(context.Background(), ListRecordsOptions{Limit: 5, Direction: RecordListDirectionAsc, Query: "hello"})
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}
	if len(page.Records) != 1 || page.Direction != RecordListDirectionAsc {
		t.Fatalf("page = %+v", page)
	}
	roots, err := client.ListRoots(context.Background(), 7)
	if err != nil {
		t.Fatalf("ListRoots: %v", err)
	}
	if len(roots) != 1 || roots[0].BatchID != "batch-1" {
		t.Fatalf("roots = %+v", roots)
	}
	latest, err := client.LatestRoot(context.Background())
	if err != nil {
		t.Fatalf("LatestRoot: %v", err)
	}
	if latest.BatchID != "batch-latest" {
		t.Fatalf("latest = %+v", latest)
	}
	bundle, err := client.GetProofBundle(context.Background(), "tr1record")
	if err != nil {
		t.Fatalf("GetProofBundle: %v", err)
	}
	if bundle.RecordID != "tr1record" {
		t.Fatalf("bundle = %+v", bundle)
	}
	metrics, err := client.MetricsRaw(context.Background())
	if err != nil {
		t.Fatalf("MetricsRaw: %v", err)
	}
	if !strings.Contains(metrics, "trustdb_ingest_total") {
		t.Fatalf("metrics = %q", metrics)
	}
}

func TestGRPCTransportMapsNotFound(t *testing.T) {
	t.Parallel()

	client := newBufconnClient(t, grpcapi.NewServer(nil, grpcTestBatch{}, nil, nil, nil))
	_, err := client.GetRecord(context.Background(), "missing")
	if err == nil {
		t.Fatal("GetRecord error = nil, want not found")
	}
	if !IsNotFound(err) {
		t.Fatalf("IsNotFound(%T %v) = false", err, err)
	}
}

func newBufconnClient(t *testing.T, srv grpcapi.TrustDBServiceServer) *Client {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer(
		grpc.MaxRecvMsgSize(grpcapi.MaxMessageBytes),
		grpc.MaxSendMsgSize(grpcapi.MaxMessageBytes),
	)
	grpcapi.RegisterTrustDBServiceServer(server, srv)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.ForceCodec(grpcapi.Codec()),
			grpc.MaxCallRecvMsgSize(grpcapi.MaxMessageBytes),
			grpc.MaxCallSendMsgSize(grpcapi.MaxMessageBytes),
		),
	)
	if err != nil {
		t.Fatalf("DialContext: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client, err := NewClientWithTransport(NewGRPCTransportFromConn("bufnet", conn))
	if err != nil {
		t.Fatalf("NewClientWithTransport: %v", err)
	}
	return client
}

type grpcTestBatch struct{}

func (grpcTestBatch) Enqueue(context.Context, model.SignedClaim, model.ServerRecord, model.AcceptedReceipt) error {
	return nil
}

func (grpcTestBatch) Proof(context.Context, string) (model.ProofBundle, error) {
	return model.ProofBundle{SchemaVersion: model.SchemaProofBundle, RecordID: "tr1record"}, nil
}

func (grpcTestBatch) RecordIndex(_ context.Context, recordID string) (model.RecordIndex, bool, error) {
	if recordID == "missing" {
		return model.RecordIndex{}, false, nil
	}
	return model.RecordIndex{SchemaVersion: model.SchemaRecordIndex, RecordID: "tr1record", BatchID: "batch-1"}, true, nil
}

func (grpcTestBatch) Records(_ context.Context, opts model.RecordListOptions) ([]model.RecordIndex, error) {
	if opts.Query != "" && opts.Query != "hello" {
		return nil, trusterr.New(trusterr.CodeInvalidArgument, "unexpected query")
	}
	return []model.RecordIndex{{SchemaVersion: model.SchemaRecordIndex, RecordID: "tr1record", BatchID: "batch-1", ReceivedAtUnixN: 10}}, nil
}

func (grpcTestBatch) Roots(context.Context, int) ([]model.BatchRoot, error) {
	return []model.BatchRoot{{SchemaVersion: model.SchemaBatchRoot, BatchID: "batch-1", TreeSize: 1}}, nil
}

func (grpcTestBatch) RootsAfter(context.Context, int64, int) ([]model.BatchRoot, error) {
	return []model.BatchRoot{{SchemaVersion: model.SchemaBatchRoot, BatchID: "batch-2", TreeSize: 2, ClosedAtUnixN: 20}}, nil
}

func (grpcTestBatch) RootsPage(context.Context, model.RootListOptions) ([]model.BatchRoot, error) {
	return []model.BatchRoot{{SchemaVersion: model.SchemaBatchRoot, BatchID: "batch-1", TreeSize: 1, ClosedAtUnixN: 10}}, nil
}

func (grpcTestBatch) LatestRoot(context.Context) (model.BatchRoot, error) {
	return model.BatchRoot{SchemaVersion: model.SchemaBatchRoot, BatchID: "batch-latest", TreeSize: 3}, nil
}

func (grpcTestBatch) Manifest(context.Context, string) (model.BatchManifest, error) {
	return model.BatchManifest{SchemaVersion: model.SchemaBatchManifest, BatchID: "batch-1", TreeSize: 1}, nil
}

func (grpcTestBatch) BatchTreeLeaves(context.Context, model.BatchTreeLeafListOptions) ([]model.BatchTreeLeaf, error) {
	return []model.BatchTreeLeaf{{SchemaVersion: model.SchemaBatchTreeLeaf, BatchID: "batch-1", RecordID: "tr1record", LeafIndex: 0}}, nil
}

func (grpcTestBatch) BatchTreeNodes(context.Context, model.BatchTreeNodeListOptions) ([]model.BatchTreeNode, error) {
	return []model.BatchTreeNode{{SchemaVersion: model.SchemaBatchTreeNode, BatchID: "batch-1", Level: 0, StartIndex: 0, Width: 1}}, nil
}
