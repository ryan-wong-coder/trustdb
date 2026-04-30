package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/ryan-wong-coder/trustdb/internal/cborx"
	"github.com/ryan-wong-coder/trustdb/internal/ingest"
	"github.com/ryan-wong-coder/trustdb/internal/model"
	"github.com/ryan-wong-coder/trustdb/internal/trusterr"
)

func TestSubmitClaim(t *testing.T) {
	t.Parallel()

	p := processorFunc(func(ctx context.Context, signed model.SignedClaim) (model.ServerRecord, model.AcceptedReceipt, bool, error) {
		return model.ServerRecord{RecordID: "tr1http"}, model.AcceptedReceipt{RecordID: "tr1http", Status: "accepted"}, false, nil
	})
	svc := ingest.New(p, ingest.Options{QueueSize: 1, Workers: 1}, nil)
	defer svc.Shutdown(context.Background())
	handler := New(svc, nil)
	body, err := cborx.Marshal(model.SignedClaim{SchemaVersion: model.SchemaSignedClaim})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/claims", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not json: %v", err)
	}
	if got["record_id"] != "tr1http" || got["status"] != "accepted" {
		t.Fatalf("response = %#v", got)
	}
	if got["proof_level"] != "L2" {
		t.Fatalf("proof_level = %#v", got["proof_level"])
	}
}

func TestSubmitClaimEnqueuesBatch(t *testing.T) {
	t.Parallel()

	p := processorFunc(func(ctx context.Context, signed model.SignedClaim) (model.ServerRecord, model.AcceptedReceipt, bool, error) {
		return model.ServerRecord{RecordID: "tr1batch"}, model.AcceptedReceipt{RecordID: "tr1batch", Status: "accepted"}, false, nil
	})
	svc := ingest.New(p, ingest.Options{QueueSize: 1, Workers: 1}, nil)
	defer svc.Shutdown(context.Background())
	batchSvc := &fakeBatchService{}
	handler := New(svc, nil, batchSvc)
	body, err := cborx.Marshal(model.SignedClaim{SchemaVersion: model.SchemaSignedClaim})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/claims", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var got submitClaimResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not json: %v", err)
	}
	if !got.BatchEnqueued {
		t.Fatalf("BatchEnqueued = false response=%+v", got)
	}
	if batchSvc.enqueuedRecordID() != "tr1batch" {
		t.Fatalf("enqueued record id = %s", batchSvc.enqueuedRecordID())
	}
}

func TestSubmitClaimIdempotentReplaySkipsBatch(t *testing.T) {
	t.Parallel()

	p := processorFunc(func(ctx context.Context, signed model.SignedClaim) (model.ServerRecord, model.AcceptedReceipt, bool, error) {
		return model.ServerRecord{RecordID: "tr1replay"}, model.AcceptedReceipt{RecordID: "tr1replay", Status: "accepted"}, true, nil
	})
	svc := ingest.New(p, ingest.Options{QueueSize: 1, Workers: 1}, nil)
	defer svc.Shutdown(context.Background())
	batchSvc := &fakeBatchService{}
	handler := New(svc, nil, batchSvc)
	body, err := cborx.Marshal(model.SignedClaim{SchemaVersion: model.SchemaSignedClaim})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/claims", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("idempotent replay status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got submitClaimResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not json: %v", err)
	}
	if !got.Idempotent {
		t.Fatalf("Idempotent = false, want true response=%+v", got)
	}
	if got.BatchEnqueued {
		t.Fatalf("BatchEnqueued = true, idempotent replay must not re-enqueue")
	}
	if id := batchSvc.enqueuedRecordID(); id != "" {
		t.Fatalf("batch enqueued record id = %q, want empty", id)
	}
}

func TestSubmitClaimIdempotencyConflictReturns409(t *testing.T) {
	t.Parallel()

	p := processorFunc(func(ctx context.Context, signed model.SignedClaim) (model.ServerRecord, model.AcceptedReceipt, bool, error) {
		return model.ServerRecord{}, model.AcceptedReceipt{}, false, trusterr.New(trusterr.CodeAlreadyExists, "idempotency_key conflict")
	})
	svc := ingest.New(p, ingest.Options{QueueSize: 1, Workers: 1}, nil)
	defer svc.Shutdown(context.Background())
	handler := New(svc, nil)
	body, err := cborx.Marshal(model.SignedClaim{SchemaVersion: model.SchemaSignedClaim})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/claims", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	var got errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not json: %v", err)
	}
	if got.Code != trusterr.CodeAlreadyExists {
		t.Fatalf("response code = %s, want %s", got.Code, trusterr.CodeAlreadyExists)
	}
}

func TestSubmitClaimRejectsBadCBOR(t *testing.T) {
	t.Parallel()

	svc := ingest.New(processorFunc(nil), ingest.Options{QueueSize: 1, Workers: 1}, nil)
	defer svc.Shutdown(context.Background())
	handler := New(svc, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/claims", strings.NewReader("not cbor"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHealthAndMetrics(t *testing.T) {
	t.Parallel()

	metricsHandler, _ := MetricsHandler()
	handler := New(nil, metricsHandler)
	for _, path := range []string{"/healthz", "/metrics"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, rec.Code)
		}
	}
}

func TestProofAndRootEndpoints(t *testing.T) {
	t.Parallel()

	batchSvc := &fakeBatchService{
		proof: model.ProofBundle{
			SchemaVersion: model.SchemaProofBundle,
			RecordID:      "tr1proof",
			BatchProof:    model.BatchProof{TreeSize: 1},
		},
		roots: []model.BatchRoot{
			{SchemaVersion: model.SchemaBatchRoot, BatchID: "batch-a", TreeSize: 1},
		},
	}
	handler := New(nil, nil, batchSvc)

	req := httptest.NewRequest(http.MethodGet, "/v1/proofs/tr1proof", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("proof status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/roots/latest", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("latest root status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/roots?limit=1", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("roots status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRecordEndpoints(t *testing.T) {
	t.Parallel()

	batchSvc := &fakeBatchService{
		records: []model.RecordIndex{
			{SchemaVersion: model.SchemaRecordIndex, RecordID: "rec-3", TenantID: "tenant-b", BatchID: "batch-2", ProofLevel: "L5", ReceivedAtUnixN: 300, ContentHash: bytes.Repeat([]byte{3}, 32), StorageURI: "file:///vault/report-final.pdf"},
			{SchemaVersion: model.SchemaRecordIndex, RecordID: "rec-2", TenantID: "tenant-a", BatchID: "batch-1", ProofLevel: "L4", ReceivedAtUnixN: 200, ContentHash: bytes.Repeat([]byte{2}, 32), StorageURI: "file:///vault/screenshot-alpha.png"},
			{SchemaVersion: model.SchemaRecordIndex, RecordID: "rec-1", TenantID: "tenant-a", BatchID: "batch-1", ProofLevel: "L3", ReceivedAtUnixN: 100, ContentHash: bytes.Repeat([]byte{1}, 32), StorageURI: "file:///vault/notes.txt"},
		},
	}
	handler := New(nil, nil, batchSvc)

	req := httptest.NewRequest(http.MethodGet, "/v1/records?limit=2", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("records status = %d body=%s", rec.Code, rec.Body.String())
	}
	var page recordsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode records response: %v", err)
	}
	if len(page.Records) != 2 || page.Records[0].RecordID != "rec-3" || page.NextCursor == "" {
		t.Fatalf("records page = %+v", page)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/records?limit=2&cursor="+page.NextCursor, nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("records next status = %d body=%s", rec.Code, rec.Body.String())
	}
	var next recordsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &next); err != nil {
		t.Fatalf("decode next records response: %v", err)
	}
	if len(next.Records) != 1 || next.Records[0].RecordID != "rec-1" {
		t.Fatalf("next records page = %+v", next)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/records/rec-2", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("record index status = %d body=%s", rec.Code, rec.Body.String())
	}
	var idx model.RecordIndex
	if err := json.Unmarshal(rec.Body.Bytes(), &idx); err != nil {
		t.Fatalf("decode record index: %v", err)
	}
	if idx.RecordID != "rec-2" || idx.BatchID != "batch-1" {
		t.Fatalf("record index = %+v", idx)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/records?q=screenshot&limit=10", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("records q status = %d body=%s", rec.Code, rec.Body.String())
	}
	var byQuery recordsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &byQuery); err != nil {
		t.Fatalf("decode q records response: %v", err)
	}
	if len(byQuery.Records) != 1 || byQuery.Records[0].RecordID != "rec-2" {
		t.Fatalf("q records page = %+v", byQuery)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/records?content_hash="+strings.Repeat("02", 32)+"&limit=10", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("records content_hash status = %d body=%s", rec.Code, rec.Body.String())
	}
	var byHash recordsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &byHash); err != nil {
		t.Fatalf("decode hash records response: %v", err)
	}
	if len(byHash.Records) != 1 || byHash.Records[0].RecordID != "rec-2" {
		t.Fatalf("hash records page = %+v", byHash)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/records?received_from=150&received_to=250&limit=10", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("records range status = %d body=%s", rec.Code, rec.Body.String())
	}
	var byRange recordsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &byRange); err != nil {
		t.Fatalf("decode range records response: %v", err)
	}
	if len(byRange.Records) != 1 || byRange.Records[0].RecordID != "rec-2" {
		t.Fatalf("range records page = %+v", byRange)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/records?level=L5&limit=10", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("records level status = %d body=%s", rec.Code, rec.Body.String())
	}
	var byLevel recordsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &byLevel); err != nil {
		t.Fatalf("decode level records response: %v", err)
	}
	if len(byLevel.Records) != 1 || byLevel.Records[0].RecordID != "rec-3" {
		t.Fatalf("level records page = %+v", byLevel)
	}
}

func TestGlobalAndAnchorListEndpoints(t *testing.T) {
	t.Parallel()

	handler := NewWithGlobalAndAnchors(nil, nil, &fakeBatchService{}, fakeGlobalService{
		sths: []model.SignedTreeHead{
			{SchemaVersion: model.SchemaSignedTreeHead, TreeSize: 3, RootHash: []byte{3}},
			{SchemaVersion: model.SchemaSignedTreeHead, TreeSize: 2, RootHash: []byte{2}},
			{SchemaVersion: model.SchemaSignedTreeHead, TreeSize: 1, RootHash: []byte{1}},
		},
		leaves: []model.GlobalLogLeaf{
			{SchemaVersion: model.SchemaGlobalLogLeaf, BatchID: "batch-3", LeafIndex: 2},
			{SchemaVersion: model.SchemaGlobalLogLeaf, BatchID: "batch-2", LeafIndex: 1},
			{SchemaVersion: model.SchemaGlobalLogLeaf, BatchID: "batch-1", LeafIndex: 0},
		},
	}, fakeAnchorService{
		items: []model.STHAnchorOutboxItem{
			{SchemaVersion: model.SchemaSTHAnchorOutbox, TreeSize: 3, Status: model.AnchorStatePending},
			{SchemaVersion: model.SchemaSTHAnchorOutbox, TreeSize: 2, Status: model.AnchorStatePublished},
			{SchemaVersion: model.SchemaSTHAnchorOutbox, TreeSize: 1, Status: model.AnchorStatePublished},
		},
		results: map[uint64]model.STHAnchorResult{
			2: {SchemaVersion: model.SchemaSTHAnchorResult, TreeSize: 2, AnchorID: "anchor-2", SinkName: "ots"},
			1: {SchemaVersion: model.SchemaSTHAnchorResult, TreeSize: 1, AnchorID: "anchor-1", SinkName: "ots"},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/sth?limit=2", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sth list status = %d body=%s", rec.Code, rec.Body.String())
	}
	var sthsPage sthsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &sthsPage); err != nil {
		t.Fatalf("decode sth page: %v", err)
	}
	if len(sthsPage.STHs) != 2 || sthsPage.STHs[0].TreeSize != 3 || sthsPage.NextCursor == "" {
		t.Fatalf("sth page = %+v", sthsPage)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/global-log/leaves?limit=2", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("global leaves status = %d body=%s", rec.Code, rec.Body.String())
	}
	var leavesPage globalLeavesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &leavesPage); err != nil {
		t.Fatalf("decode leaves page: %v", err)
	}
	if len(leavesPage.Leaves) != 2 || leavesPage.Leaves[0].LeafIndex != 2 || leavesPage.NextCursor == "" {
		t.Fatalf("leaves page = %+v", leavesPage)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/anchors/sth?limit=2", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("anchor list status = %d body=%s", rec.Code, rec.Body.String())
	}
	var anchorsPage anchorsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &anchorsPage); err != nil {
		t.Fatalf("decode anchor page: %v", err)
	}
	if len(anchorsPage.Anchors) != 2 || anchorsPage.Anchors[0].TreeSize != 3 || anchorsPage.NextCursor == "" {
		t.Fatalf("anchor page = %+v", anchorsPage)
	}
}

func TestGlobalRoutesAreNotRegisteredWithoutGlobalService(t *testing.T) {
	t.Parallel()

	handler := NewWithGlobalAndAnchors(nil, nil, &fakeBatchService{}, nil, nil)
	for _, path := range []string{
		"/v1/sth/latest",
		"/v1/sth",
		"/v1/global-log/leaves",
		"/v1/global-log/inclusion/batch-1",
		"/v1/global-log/consistency?from=1&to=2",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404 body=%s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestGlobalRoutesAreNotRegisteredWithTypedNilGlobalService(t *testing.T) {
	t.Parallel()

	var global *fakeGlobalService
	handler := NewWithGlobalAndAnchors(nil, nil, &fakeBatchService{}, global, nil)
	for _, path := range []string{
		"/v1/sth/latest",
		"/v1/sth",
		"/v1/global-log/leaves",
		"/v1/global-log/inclusion/batch-1",
		"/v1/global-log/consistency?from=1&to=2",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404 body=%s", path, rec.Code, rec.Body.String())
		}
	}
}

type processorFunc func(context.Context, model.SignedClaim) (model.ServerRecord, model.AcceptedReceipt, bool, error)

func (f processorFunc) Submit(ctx context.Context, signed model.SignedClaim) (model.ServerRecord, model.AcceptedReceipt, bool, error) {
	if f == nil {
		return model.ServerRecord{}, model.AcceptedReceipt{}, false, nil
	}
	return f(ctx, signed)
}

type fakeBatchService struct {
	mu       sync.Mutex
	enqueued string
	proof    model.ProofBundle
	roots    []model.BatchRoot
	records  []model.RecordIndex
}

func (f *fakeBatchService) Enqueue(ctx context.Context, signed model.SignedClaim, record model.ServerRecord, accepted model.AcceptedReceipt) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enqueued = record.RecordID
	return nil
}

func (f *fakeBatchService) Proof(ctx context.Context, recordID string) (model.ProofBundle, error) {
	if f.proof.RecordID == "" {
		f.proof = model.ProofBundle{SchemaVersion: model.SchemaProofBundle, RecordID: recordID}
	}
	return f.proof, nil
}

func (f *fakeBatchService) RecordIndex(ctx context.Context, recordID string) (model.RecordIndex, bool, error) {
	for _, idx := range f.records {
		if idx.RecordID == recordID {
			return idx, true, nil
		}
	}
	return model.RecordIndex{}, false, nil
}

func (f *fakeBatchService) Records(ctx context.Context, opts model.RecordListOptions) ([]model.RecordIndex, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}
	records := append([]model.RecordIndex(nil), f.records...)
	desc := !strings.EqualFold(opts.Direction, model.RecordListDirectionAsc)
	sort.Slice(records, func(i, j int) bool {
		cmp := model.CompareRecordPosition(records[i].ReceivedAtUnixN, records[i].RecordID, records[j].ReceivedAtUnixN, records[j].RecordID)
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
	out := make([]model.RecordIndex, 0, limit)
	for _, idx := range records {
		if !model.RecordIndexMatchesListOptions(idx, opts) || !model.RecordIndexAfterCursor(idx, opts) {
			continue
		}
		out = append(out, idx)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (f *fakeBatchService) Roots(ctx context.Context, limit int) ([]model.BatchRoot, error) {
	return f.roots, nil
}

func (f *fakeBatchService) RootsAfter(ctx context.Context, afterClosedAtUnixN int64, limit int) ([]model.BatchRoot, error) {
	out := make([]model.BatchRoot, 0, len(f.roots))
	for _, root := range f.roots {
		if root.ClosedAtUnixN > afterClosedAtUnixN {
			out = append(out, root)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (f *fakeBatchService) RootsPage(ctx context.Context, opts model.RootListOptions) ([]model.BatchRoot, error) {
	roots := append([]model.BatchRoot(nil), f.roots...)
	desc := !strings.EqualFold(opts.Direction, model.RecordListDirectionAsc)
	sort.Slice(roots, func(i, j int) bool {
		cmp := model.CompareBatchRootPosition(roots[i].ClosedAtUnixN, roots[i].BatchID, roots[j].ClosedAtUnixN, roots[j].BatchID)
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
	out := make([]model.BatchRoot, 0, opts.Limit)
	for _, root := range roots {
		if !model.BatchRootAfterCursor(root, opts) {
			continue
		}
		out = append(out, root)
		if len(out) >= opts.Limit {
			break
		}
	}
	return out, nil
}

func (f *fakeBatchService) LatestRoot(ctx context.Context) (model.BatchRoot, error) {
	if len(f.roots) == 0 {
		return model.BatchRoot{}, nil
	}
	return f.roots[0], nil
}

func (f *fakeBatchService) enqueuedRecordID() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.enqueued
}

type fakeGlobalService struct {
	sths   []model.SignedTreeHead
	leaves []model.GlobalLogLeaf
}

func (f fakeGlobalService) LatestSTH(context.Context) (model.SignedTreeHead, bool, error) {
	if len(f.sths) == 0 {
		return model.SignedTreeHead{}, false, nil
	}
	return f.sths[0], true, nil
}

func (f fakeGlobalService) STH(_ context.Context, treeSize uint64) (model.SignedTreeHead, bool, error) {
	for _, sth := range f.sths {
		if sth.TreeSize == treeSize {
			return sth, true, nil
		}
	}
	return model.SignedTreeHead{}, false, nil
}

func (f fakeGlobalService) ListSTHs(_ context.Context, opts model.TreeHeadListOptions) ([]model.SignedTreeHead, error) {
	sths := append([]model.SignedTreeHead(nil), f.sths...)
	desc := !strings.EqualFold(opts.Direction, model.RecordListDirectionAsc)
	sort.Slice(sths, func(i, j int) bool {
		cmp := model.CompareUint64Position(sths[i].TreeSize, sths[j].TreeSize)
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
	out := make([]model.SignedTreeHead, 0, opts.Limit)
	for _, sth := range sths {
		if !model.Uint64AfterCursor(sth.TreeSize, opts.AfterTreeSize, opts.Direction) {
			continue
		}
		out = append(out, sth)
		if len(out) >= opts.Limit {
			break
		}
	}
	return out, nil
}

func (f fakeGlobalService) ListLeaves(_ context.Context, opts model.GlobalLeafListOptions) ([]model.GlobalLogLeaf, error) {
	leaves := append([]model.GlobalLogLeaf(nil), f.leaves...)
	desc := !strings.EqualFold(opts.Direction, model.RecordListDirectionAsc)
	sort.Slice(leaves, func(i, j int) bool {
		cmp := model.CompareUint64Position(leaves[i].LeafIndex, leaves[j].LeafIndex)
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
	out := make([]model.GlobalLogLeaf, 0, opts.Limit)
	for _, leaf := range leaves {
		if !model.Uint64AfterCursor(leaf.LeafIndex, opts.AfterLeafIndex, opts.Direction) {
			continue
		}
		out = append(out, leaf)
		if len(out) >= opts.Limit {
			break
		}
	}
	return out, nil
}

func (f fakeGlobalService) InclusionProof(context.Context, string, uint64) (model.GlobalLogProof, error) {
	return model.GlobalLogProof{}, nil
}

func (f fakeGlobalService) ConsistencyProof(context.Context, uint64, uint64) (model.GlobalConsistencyProof, error) {
	return model.GlobalConsistencyProof{}, nil
}

type fakeAnchorService struct {
	items   []model.STHAnchorOutboxItem
	results map[uint64]model.STHAnchorResult
}

func (f fakeAnchorService) AnchorResult(_ context.Context, treeSize uint64) (model.STHAnchorResult, bool, error) {
	result, ok := f.results[treeSize]
	return result, ok, nil
}

func (f fakeAnchorService) AnchorStatus(_ context.Context, treeSize uint64) (model.STHAnchorOutboxItem, bool, error) {
	for _, item := range f.items {
		if item.TreeSize == treeSize {
			return item, true, nil
		}
	}
	return model.STHAnchorOutboxItem{}, false, nil
}

func (f fakeAnchorService) Anchors(_ context.Context, opts model.AnchorListOptions) ([]model.STHAnchorOutboxItem, error) {
	items := append([]model.STHAnchorOutboxItem(nil), f.items...)
	desc := !strings.EqualFold(opts.Direction, model.RecordListDirectionAsc)
	sort.Slice(items, func(i, j int) bool {
		cmp := model.CompareUint64Position(items[i].TreeSize, items[j].TreeSize)
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
	out := make([]model.STHAnchorOutboxItem, 0, opts.Limit)
	for _, item := range items {
		if !model.Uint64AfterCursor(item.TreeSize, opts.AfterTreeSize, opts.Direction) {
			continue
		}
		out = append(out, item)
		if len(out) >= opts.Limit {
			break
		}
	}
	return out, nil
}
