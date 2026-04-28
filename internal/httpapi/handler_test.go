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
			{SchemaVersion: model.SchemaRecordIndex, RecordID: "rec-3", TenantID: "tenant-b", BatchID: "batch-2", ReceivedAtUnixN: 300, ContentHash: bytes.Repeat([]byte{3}, 32), StorageURI: "file:///vault/report-final.pdf"},
			{SchemaVersion: model.SchemaRecordIndex, RecordID: "rec-2", TenantID: "tenant-a", BatchID: "batch-1", ReceivedAtUnixN: 200, ContentHash: bytes.Repeat([]byte{2}, 32), StorageURI: "file:///vault/screenshot-alpha.png"},
			{SchemaVersion: model.SchemaRecordIndex, RecordID: "rec-1", TenantID: "tenant-a", BatchID: "batch-1", ReceivedAtUnixN: 100, ContentHash: bytes.Repeat([]byte{1}, 32), StorageURI: "file:///vault/notes.txt"},
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
