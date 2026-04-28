package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ryan-wong-coder/trustdb/internal/model"
)

// httpTimeout bounds each request against the server. Submit + proof
// queries are small (<1MB) so a short timeout gives a clear failure
// mode when the server is down or unreachable.
const httpTimeout = 15 * time.Second

type httpClient struct {
	base  string
	inner *http.Client
}

func newHTTPClient(base string) (*httpClient, error) {
	trimmed := strings.TrimSpace(base)
	if trimmed == "" {
		return nil, fmt.Errorf("server url is empty")
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("parse server url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("server url must include scheme and host: %s", trimmed)
	}
	return &httpClient{base: strings.TrimRight(trimmed, "/"), inner: &http.Client{Timeout: httpTimeout}}, nil
}

func (c *httpClient) url(p string, parts ...string) string {
	// Manual assembly is safer than a single url.Parse here because
	// record ids contain characters (e.g. lower-case base32) that we
	// want percent-encoded while the static prefix stays untouched.
	out := c.base + p
	for _, part := range parts {
		out += url.PathEscape(part)
	}
	return out
}

type ServerError struct {
	StatusCode int
	Code       string `json:"code"`
	Message    string `json:"message"`
}

func (e *ServerError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("http %d: %s", e.StatusCode, e.Message)
}

// decodeError tries to parse the server's structured error envelope
// (trusterr.Code + message) and falls back to the raw body if the
// response isn't JSON, so the UI always surfaces *something*
// actionable instead of a blank "request failed".
func decodeError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	var se ServerError
	if err := json.Unmarshal(body, &se); err == nil && (se.Code != "" || se.Message != "") {
		se.StatusCode = resp.StatusCode
		return &se
	}
	return &ServerError{StatusCode: resp.StatusCode, Message: strings.TrimSpace(string(body))}
}

func (c *httpClient) getJSON(ctx context.Context, endpoint string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.inner.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return decodeError(resp)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

type HealthStatus struct {
	OK         bool   `json:"ok"`
	ServerURL  string `json:"server_url"`
	RTTMillis  int64  `json:"rtt_millis"`
	Error      string `json:"error,omitempty"`
	StatusCode int    `json:"status_code,omitempty"`
}

func (c *httpClient) health(ctx context.Context) HealthStatus {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/healthz", nil)
	if err != nil {
		return HealthStatus{ServerURL: c.base, Error: err.Error()}
	}
	resp, err := c.inner.Do(req)
	rtt := time.Since(start).Milliseconds()
	if err != nil {
		return HealthStatus{ServerURL: c.base, Error: err.Error(), RTTMillis: rtt}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return HealthStatus{ServerURL: c.base, Error: "unhealthy", StatusCode: resp.StatusCode, RTTMillis: rtt}
	}
	return HealthStatus{OK: true, ServerURL: c.base, RTTMillis: rtt}
}

// submitClaimResult mirrors the server's submitClaimResponse so the
// UI can inspect batch_enqueued / idempotent flags without making a
// second call.
type submitClaimResult struct {
	RecordID        string                `json:"record_id"`
	Status          string                `json:"status"`
	ProofLevel      string                `json:"proof_level"`
	Idempotent      bool                  `json:"idempotent"`
	BatchEnqueued   bool                  `json:"batch_enqueued"`
	BatchError      string                `json:"batch_error,omitempty"`
	ServerRecord    model.ServerRecord    `json:"server_record"`
	AcceptedReceipt model.AcceptedReceipt `json:"accepted_receipt"`
}

func (c *httpClient) submitClaimCBOR(ctx context.Context, body []byte) (submitClaimResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/v1/claims", bytes.NewReader(body))
	if err != nil {
		return submitClaimResult{}, err
	}
	req.Header.Set("Content-Type", "application/cbor")
	resp, err := c.inner.Do(req)
	if err != nil {
		return submitClaimResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return submitClaimResult{}, decodeError(resp)
	}
	var out submitClaimResult
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return submitClaimResult{}, fmt.Errorf("decode submit response: %w", err)
	}
	return out, nil
}

type proofEnvelope struct {
	RecordID    string            `json:"record_id"`
	ProofLevel  string            `json:"proof_level"`
	ProofBundle model.ProofBundle `json:"proof_bundle"`
}

type recordsEnvelope struct {
	Records    []model.RecordIndex `json:"records"`
	Limit      int                 `json:"limit"`
	Direction  string              `json:"direction"`
	NextCursor string              `json:"next_cursor,omitempty"`
}

func (c *httpClient) getProof(ctx context.Context, recordID string) (model.ProofBundle, error) {
	endpoint := c.url("/v1/proofs/", recordID)
	var env proofEnvelope
	if err := c.getJSON(ctx, endpoint, &env); err != nil {
		return model.ProofBundle{}, err
	}
	if env.ProofBundle.RecordID == "" {
		return model.ProofBundle{}, fmt.Errorf("server returned empty proof bundle")
	}
	return env.ProofBundle, nil
}

func (c *httpClient) getRecordIndex(ctx context.Context, recordID string) (model.RecordIndex, error) {
	var idx model.RecordIndex
	if err := c.getJSON(ctx, c.url("/v1/records/", recordID), &idx); err != nil {
		return model.RecordIndex{}, err
	}
	if idx.RecordID == "" {
		return model.RecordIndex{}, fmt.Errorf("server returned empty record index")
	}
	return idx, nil
}

func (c *httpClient) listRecordIndexes(ctx context.Context, opts RecordPageOptions) (RecordPage, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	direction := strings.TrimSpace(opts.Direction)
	if direction == "" {
		direction = "desc"
	}

	query := strings.TrimSpace(opts.Query)
	contentHash := ""
	if query != "" {
		if looksLikeRecordID(query) {
			idx, err := c.getRecordIndex(ctx, query)
			if err != nil {
				var se *ServerError
				if errors.As(err, &se) && se.StatusCode == http.StatusNotFound {
					return RecordPage{Items: nil, Limit: limit, Offset: opts.Offset, Source: "server", TotalExact: true}, nil
				}
				return RecordPage{}, err
			}
			return RecordPage{
				Items:      []LocalRecord{localRecordFromIndex(idx)},
				Total:      1,
				Limit:      limit,
				Offset:     opts.Offset,
				Source:     "server",
				TotalExact: true,
			}, nil
		}
		if strings.HasPrefix(query, "batch-") && opts.BatchID == "" {
			opts.BatchID = query
			query = ""
		}
		if looksLikeSHA256Hex(query) {
			contentHash = strings.TrimPrefix(strings.ToLower(query), "sha256:")
			query = ""
		}
	}

	values := url.Values{}
	values.Set("limit", strconv.Itoa(limit))
	values.Set("direction", direction)
	if opts.Cursor != "" {
		values.Set("cursor", opts.Cursor)
	}
	if opts.BatchID != "" {
		values.Set("batch_id", opts.BatchID)
	}
	if opts.TenantID != "" {
		values.Set("tenant_id", opts.TenantID)
	}
	if opts.ClientID != "" {
		values.Set("client_id", opts.ClientID)
	}
	if contentHash != "" {
		values.Set("content_hash", contentHash)
	}
	if query != "" {
		values.Set("q", query)
	}
	endpoint := c.base + "/v1/records?" + values.Encode()
	var env recordsEnvelope
	if err := c.getJSON(ctx, endpoint, &env); err != nil {
		return RecordPage{}, err
	}
	items := make([]LocalRecord, 0, len(env.Records))
	for _, idx := range env.Records {
		items = append(items, localRecordFromIndex(idx))
	}
	total := opts.Offset + len(items)
	if env.NextCursor != "" {
		total++
	}
	return RecordPage{
		Items:      items,
		Total:      total,
		Limit:      limit,
		Offset:     opts.Offset,
		HasMore:    env.NextCursor != "",
		NextCursor: env.NextCursor,
		Source:     "server",
		TotalExact: env.NextCursor == "",
	}, nil
}

func looksLikeRecordID(query string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(query)), "tr1")
}

func looksLikeSHA256Hex(query string) bool {
	query = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(query)), "sha256:")
	if len(query) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(query)
	return err == nil && len(decoded) == 32
}

func localRecordFromIndex(idx model.RecordIndex) LocalRecord {
	proofLevel := "L2"
	var committed *model.CommittedReceipt
	if idx.BatchID != "" {
		proofLevel = "L3"
		committed = &model.CommittedReceipt{
			SchemaVersion: model.SchemaCommittedReceipt,
			RecordID:      idx.RecordID,
			Status:        "committed",
			BatchID:       idx.BatchID,
			LeafIndex:     idx.BatchLeafIndex,
			ClosedAtUnixN: idx.BatchClosedAtUnixN,
		}
	}
	submittedAt := time.Unix(0, idx.ReceivedAtUnixN).UTC()
	if idx.ReceivedAtUnixN == 0 && idx.BatchClosedAtUnixN != 0 {
		submittedAt = time.Unix(0, idx.BatchClosedAtUnixN).UTC()
	}
	rec := LocalRecord{
		RecordID:         idx.RecordID,
		FilePath:         idx.StorageURI,
		FileName:         displayNameFromStorageURI(idx),
		ContentHashHex:   hex.EncodeToString(idx.ContentHash),
		ContentLength:    idx.ContentLength,
		MediaType:        idx.MediaType,
		EventType:        idx.EventType,
		Source:           idx.Source,
		TenantID:         idx.TenantID,
		ClientID:         idx.ClientID,
		KeyID:            idx.KeyID,
		ProofLevel:       proofLevel,
		BatchID:          idx.BatchID,
		CommittedReceipt: committed,
	}
	setLocalRecordSubmittedAt(&rec, submittedAt)
	setLocalRecordLastSyncedAt(&rec, time.Now().UTC())
	return rec
}

func displayNameFromStorageURI(idx model.RecordIndex) string {
	if strings.TrimSpace(idx.FileName) != "" {
		return strings.TrimSpace(idx.FileName)
	}
	raw := strings.TrimSpace(idx.StorageURI)
	if raw == "" {
		if idx.RecordID != "" {
			return idx.RecordID
		}
		return "remote-record"
	}
	withoutQuery := raw
	if cut := strings.IndexAny(withoutQuery, "?#"); cut >= 0 {
		withoutQuery = withoutQuery[:cut]
	}
	withoutQuery = strings.TrimRight(strings.ReplaceAll(withoutQuery, "\\", "/"), "/")
	if slash := strings.LastIndex(withoutQuery, "/"); slash >= 0 && slash < len(withoutQuery)-1 {
		return withoutQuery[slash+1:]
	}
	if withoutQuery != "" {
		return withoutQuery
	}
	return idx.RecordID
}

type anchorEnvelope struct {
	TreeSize uint64                     `json:"tree_size"`
	Status   string                     `json:"status"`
	Result   *model.STHAnchorResult     `json:"result,omitempty"`
	Outbox   *model.STHAnchorOutboxItem `json:"outbox,omitempty"`
}

func (c *httpClient) getGlobalProof(ctx context.Context, batchID string) (model.GlobalLogProof, error) {
	endpoint := c.url("/v1/global-log/inclusion/", batchID)
	var proof model.GlobalLogProof
	if err := c.getJSON(ctx, endpoint, &proof); err != nil {
		return model.GlobalLogProof{}, err
	}
	return proof, nil
}

func (c *httpClient) getAnchor(ctx context.Context, treeSize uint64) (anchorEnvelope, error) {
	endpoint := c.url("/v1/anchors/sth/", fmt.Sprintf("%d", treeSize))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return anchorEnvelope{}, err
	}
	resp, err := c.inner.Do(req)
	if err != nil {
		return anchorEnvelope{}, err
	}
	defer resp.Body.Close()
	// 404 / 412 are "no anchor yet" — legitimate states, not errors.
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusPreconditionFailed {
		return anchorEnvelope{TreeSize: treeSize, Status: "unavailable"}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return anchorEnvelope{}, decodeError(resp)
	}
	var env anchorEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return anchorEnvelope{}, fmt.Errorf("decode anchor response: %w", err)
	}
	return env, nil
}

type rootsEnvelope struct {
	Roots []model.BatchRoot `json:"roots"`
}

func (c *httpClient) listRoots(ctx context.Context, limit int) ([]model.BatchRoot, error) {
	if limit <= 0 {
		limit = 100
	}
	endpoint := c.base + "/v1/roots?limit=" + strconv.Itoa(limit)
	var env rootsEnvelope
	if err := c.getJSON(ctx, endpoint, &env); err != nil {
		return nil, err
	}
	return env.Roots, nil
}

func (c *httpClient) latestRoot(ctx context.Context) (model.BatchRoot, error) {
	var root model.BatchRoot
	if err := c.getJSON(ctx, c.base+"/v1/roots/latest", &root); err != nil {
		return model.BatchRoot{}, err
	}
	return root, nil
}

func (c *httpClient) metricsRaw(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/metrics", nil)
	if err != nil {
		return "", err
	}
	resp, err := c.inner.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", decodeError(resp)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
