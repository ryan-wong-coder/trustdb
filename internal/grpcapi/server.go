package grpcapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	"github.com/ryan-wong-coder/trustdb/internal/httpapi"
	"github.com/ryan-wong-coder/trustdb/internal/ingest"
	"github.com/ryan-wong-coder/trustdb/internal/model"
	"github.com/ryan-wong-coder/trustdb/internal/prooflevel"
	"github.com/ryan-wong-coder/trustdb/internal/trusterr"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	Ingest         *ingest.Service
	Batch          httpapi.BatchService
	Global         httpapi.GlobalLogService
	Anchors        httpapi.AnchorService
	MetricsHandler http.Handler
}

func NewServer(
	ingestSvc *ingest.Service,
	batchSvc httpapi.BatchService,
	globalSvc httpapi.GlobalLogService,
	anchorSvc httpapi.AnchorService,
	metrics http.Handler,
) *Server {
	return &Server{Ingest: ingestSvc, Batch: batchSvc, Global: globalSvc, Anchors: anchorSvc, MetricsHandler: metrics}
}

func (s *Server) Health(context.Context, *HealthRequest) (*HealthResponse, error) {
	return &HealthResponse{OK: true}, nil
}

func (s *Server) SubmitClaim(ctx context.Context, req *SubmitClaimRequest) (*SubmitClaimResponse, error) {
	if s.Ingest == nil {
		return nil, toStatusError(trusterr.New(trusterr.CodeFailedPrecondition, "ingest service is not configured"))
	}
	record, accepted, idempotent, err := s.Ingest.Submit(ctx, req.SignedClaim)
	if err != nil {
		return nil, toStatusError(err)
	}
	batchEnqueued := false
	batchErr := ""
	if s.Batch != nil && !idempotent {
		if err := s.Batch.Enqueue(context.WithoutCancel(ctx), req.SignedClaim, record, accepted); err != nil {
			batchErr = err.Error()
		} else {
			batchEnqueued = true
		}
	}
	return &SubmitClaimResponse{
		RecordID:        record.RecordID,
		Status:          accepted.Status,
		ProofLevel:      prooflevel.L2.String(),
		Idempotent:      idempotent,
		BatchEnqueued:   batchEnqueued,
		BatchError:      batchErr,
		ServerRecord:    record,
		AcceptedReceipt: accepted,
	}, nil
}

func (s *Server) GetRecord(ctx context.Context, req *GetRecordRequest) (*GetRecordResponse, error) {
	if s.Batch == nil {
		return nil, toStatusError(trusterr.New(trusterr.CodeFailedPrecondition, "batch service is not configured"))
	}
	idx, ok, err := s.Batch.RecordIndex(ctx, req.RecordID)
	if err != nil {
		return nil, toStatusError(err)
	}
	if !ok {
		return nil, toStatusError(trusterr.New(trusterr.CodeNotFound, "record index not found"))
	}
	return &GetRecordResponse{Record: idx}, nil
}

func (s *Server) ListRecords(ctx context.Context, req *ListRecordsRequest) (*ListRecordsResponse, error) {
	if s.Batch == nil {
		return nil, toStatusError(trusterr.New(trusterr.CodeFailedPrecondition, "batch service is not configured"))
	}
	opts, err := recordListOptions(req)
	if err != nil {
		return nil, toStatusError(err)
	}
	records, err := s.Batch.Records(ctx, opts)
	if err != nil {
		return nil, toStatusError(err)
	}
	next := ""
	if len(records) == opts.Limit {
		next = encodeRecordCursor(records[len(records)-1])
	}
	return &ListRecordsResponse{Records: records, Limit: opts.Limit, Direction: opts.Direction, NextCursor: next}, nil
}

func (s *Server) GetProofBundle(ctx context.Context, req *GetProofBundleRequest) (*GetProofBundleResponse, error) {
	if s.Batch == nil {
		return nil, toStatusError(trusterr.New(trusterr.CodeFailedPrecondition, "batch service is not configured"))
	}
	bundle, err := s.Batch.Proof(ctx, req.RecordID)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &GetProofBundleResponse{RecordID: bundle.RecordID, ProofLevel: prooflevel.L3.String(), ProofBundle: bundle}, nil
}

func (s *Server) ListRoots(ctx context.Context, req *ListRootsRequest) (*ListRootsResponse, error) {
	if s.Batch == nil {
		return nil, toStatusError(trusterr.New(trusterr.CodeFailedPrecondition, "batch service is not configured"))
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		return nil, toStatusError(trusterr.New(trusterr.CodeInvalidArgument, "limit must be between 1 and 1000"))
	}
	var (
		roots []model.BatchRoot
		err   error
	)
	if req.After > 0 {
		roots, err = s.Batch.RootsAfter(ctx, req.After, limit)
	} else {
		roots, err = s.Batch.Roots(ctx, limit)
	}
	if err != nil {
		return nil, toStatusError(err)
	}
	next := ""
	if req.After > 0 && len(roots) == limit {
		next = strconv.FormatInt(roots[len(roots)-1].ClosedAtUnixN, 10)
	}
	return &ListRootsResponse{Roots: roots, Limit: limit, NextCursor: next}, nil
}

func (s *Server) LatestRoot(ctx context.Context, _ *LatestRootRequest) (*LatestRootResponse, error) {
	if s.Batch == nil {
		return nil, toStatusError(trusterr.New(trusterr.CodeFailedPrecondition, "batch service is not configured"))
	}
	root, err := s.Batch.LatestRoot(ctx)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &LatestRootResponse{Root: root}, nil
}

func (s *Server) LatestSTH(ctx context.Context, _ *LatestSTHRequest) (*LatestSTHResponse, error) {
	if s.Global == nil {
		return nil, toStatusError(trusterr.New(trusterr.CodeFailedPrecondition, "global log service is not configured"))
	}
	sth, ok, err := s.Global.LatestSTH(ctx)
	if err != nil {
		return nil, toStatusError(err)
	}
	if !ok {
		return nil, toStatusError(trusterr.New(trusterr.CodeNotFound, "signed tree head not found"))
	}
	return &LatestSTHResponse{STH: sth}, nil
}

func (s *Server) GetSTH(ctx context.Context, req *GetSTHRequest) (*GetSTHResponse, error) {
	if s.Global == nil {
		return nil, toStatusError(trusterr.New(trusterr.CodeFailedPrecondition, "global log service is not configured"))
	}
	if req.TreeSize == 0 {
		return nil, toStatusError(trusterr.New(trusterr.CodeInvalidArgument, "tree_size must be a positive integer"))
	}
	sth, ok, err := s.Global.STH(ctx, req.TreeSize)
	if err != nil {
		return nil, toStatusError(err)
	}
	if !ok {
		return nil, toStatusError(trusterr.New(trusterr.CodeNotFound, "signed tree head not found"))
	}
	return &GetSTHResponse{STH: sth}, nil
}

func (s *Server) GetGlobalProof(ctx context.Context, req *GetGlobalProofRequest) (*GetGlobalProofResponse, error) {
	if s.Global == nil {
		return nil, toStatusError(trusterr.New(trusterr.CodeFailedPrecondition, "global log service is not configured"))
	}
	proof, err := s.Global.InclusionProof(ctx, req.BatchID, req.TreeSize)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &GetGlobalProofResponse{Proof: proof}, nil
}

func (s *Server) GetAnchor(ctx context.Context, req *GetAnchorRequest) (*GetAnchorResponse, error) {
	if s.Anchors == nil {
		return nil, toStatusError(trusterr.New(trusterr.CodeFailedPrecondition, "anchor service is not configured"))
	}
	if req.TreeSize == 0 {
		return nil, toStatusError(trusterr.New(trusterr.CodeInvalidArgument, "tree_size must be a positive integer"))
	}
	item, itemOK, err := s.Anchors.AnchorStatus(ctx, req.TreeSize)
	if err != nil {
		return nil, toStatusError(err)
	}
	result, resultOK, err := s.Anchors.AnchorResult(ctx, req.TreeSize)
	if err != nil {
		return nil, toStatusError(err)
	}
	if !itemOK && !resultOK {
		return nil, toStatusError(trusterr.New(trusterr.CodeNotFound, "anchor not found for STH"))
	}
	resp := &GetAnchorResponse{TreeSize: req.TreeSize, ProofLevel: prooflevel.L5.String()}
	switch {
	case resultOK:
		resp.Status = model.AnchorStatePublished
		resp.Result = &result
	case itemOK:
		resp.Status = item.Status
	default:
		resp.Status = "unknown"
	}
	if itemOK {
		resp.Outbox = &item
	}
	return resp, nil
}

func (s *Server) Metrics(ctx context.Context, _ *MetricsRequest) (*MetricsResponse, error) {
	if s.MetricsHandler == nil {
		return nil, toStatusError(trusterr.New(trusterr.CodeFailedPrecondition, "metrics handler is not configured"))
	}
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	s.MetricsHandler.ServeHTTP(rr, req)
	if rr.Code < 200 || rr.Code >= 300 {
		return nil, toStatusError(trusterr.New(trusterr.CodeInternal, fmt.Sprintf("metrics handler returned http %d", rr.Code)))
	}
	return &MetricsResponse{Text: rr.Body.String()}, nil
}

func recordListOptions(req *ListRecordsRequest) (model.RecordListOptions, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		return model.RecordListOptions{}, trusterr.New(trusterr.CodeInvalidArgument, "limit must be between 1 and 1000")
	}
	direction := req.Direction
	switch direction {
	case "":
		direction = model.RecordListDirectionDesc
	case model.RecordListDirectionAsc, model.RecordListDirectionDesc:
	default:
		return model.RecordListOptions{}, trusterr.New(trusterr.CodeInvalidArgument, "direction must be asc or desc")
	}
	opts := model.RecordListOptions{
		Limit:             limit,
		Direction:         direction,
		BatchID:           req.BatchID,
		TenantID:          req.TenantID,
		ClientID:          req.ClientID,
		Query:             strings.TrimSpace(req.Query),
		ReceivedFromUnixN: req.ReceivedFromUnixN,
		ReceivedToUnixN:   req.ReceivedToUnixN,
	}
	if opts.BatchID == "" && strings.HasPrefix(strings.ToLower(opts.Query), "batch-") {
		opts.BatchID = opts.Query
		opts.Query = ""
	}
	hashRaw := strings.TrimSpace(req.ContentHashHex)
	if hashRaw == "" && looksLikeHexSHA256(opts.Query) {
		hashRaw = opts.Query
		opts.Query = ""
	}
	if hashRaw != "" {
		hash, err := parseSHA256Hex(hashRaw)
		if err != nil {
			return model.RecordListOptions{}, err
		}
		opts.ContentHash = hash
	}
	if opts.Query != "" && model.RecordStorageQueryToken(opts.Query) == "" {
		return model.RecordListOptions{}, trusterr.New(trusterr.CodeInvalidArgument, "q must contain at least two letters or digits")
	}
	if opts.ReceivedFromUnixN > 0 && opts.ReceivedToUnixN > 0 && opts.ReceivedFromUnixN > opts.ReceivedToUnixN {
		return model.RecordListOptions{}, trusterr.New(trusterr.CodeInvalidArgument, "received_from must be <= received_to")
	}
	if req.Cursor != "" {
		cursor, err := decodeRecordCursor(req.Cursor)
		if err != nil {
			return model.RecordListOptions{}, err
		}
		opts.AfterReceivedAtUnixN = cursor.ReceivedAtUnixN
		opts.AfterRecordID = cursor.RecordID
	}
	return opts, nil
}

func looksLikeHexSHA256(raw string) bool {
	raw = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(raw)), "sha256:")
	if len(raw) != 64 {
		return false
	}
	for _, r := range raw {
		if r < '0' || r > '9' && r < 'a' || r > 'f' {
			return false
		}
	}
	return true
}

func parseSHA256Hex(raw string) ([]byte, error) {
	raw = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(raw)), "sha256:")
	if len(raw) != 64 {
		return nil, trusterr.New(trusterr.CodeInvalidArgument, "content_hash must be a 64-character sha256 hex string")
	}
	hash, err := hex.DecodeString(raw)
	if err != nil || len(hash) != 32 {
		return nil, trusterr.New(trusterr.CodeInvalidArgument, "content_hash must be a valid sha256 hex string")
	}
	return hash, nil
}

type recordCursor struct {
	ReceivedAtUnixN int64  `json:"t"`
	RecordID        string `json:"r"`
}

func encodeRecordCursor(idx model.RecordIndex) string {
	data, err := json.Marshal(recordCursor{ReceivedAtUnixN: idx.ReceivedAtUnixN, RecordID: idx.RecordID})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeRecordCursor(raw string) (recordCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return recordCursor{}, trusterr.New(trusterr.CodeInvalidArgument, "cursor is invalid")
	}
	var cursor recordCursor
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&cursor); err != nil || cursor.RecordID == "" {
		return recordCursor{}, trusterr.New(trusterr.CodeInvalidArgument, "cursor is invalid")
	}
	return cursor, nil
}

func toStatusError(err error) error {
	if err == nil {
		return nil
	}
	return status.Error(grpcCode(trusterr.CodeOf(err)), err.Error())
}

func grpcCode(code trusterr.Code) codes.Code {
	switch code {
	case trusterr.CodeInvalidArgument:
		return codes.InvalidArgument
	case trusterr.CodeAlreadyExists:
		return codes.AlreadyExists
	case trusterr.CodeFailedPrecondition:
		return codes.FailedPrecondition
	case trusterr.CodeNotFound:
		return codes.NotFound
	case trusterr.CodeResourceExhausted:
		return codes.ResourceExhausted
	case trusterr.CodeDeadlineExceeded:
		return codes.DeadlineExceeded
	case trusterr.CodeDataLoss:
		return codes.DataLoss
	default:
		return codes.Internal
	}
}
