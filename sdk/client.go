package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ryan-wong-coder/trustdb/internal/cborx"
	"github.com/ryan-wong-coder/trustdb/internal/model"
	"github.com/ryan-wong-coder/trustdb/internal/sproof"
)

const defaultHTTPTimeout = 15 * time.Second

type Client struct {
	baseURL    string
	httpClient *http.Client
	userAgent  string
}

type Option func(*Client)

func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		if client != nil {
			c.httpClient = client
		}
	}
}

func WithUserAgent(userAgent string) Option {
	return func(c *Client) {
		c.userAgent = strings.TrimSpace(userAgent)
	}
}

func NewClient(baseURL string, opts ...Option) (*Client, error) {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return nil, errors.New("sdk: server url is empty")
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("sdk: parse server url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("sdk: server url must include scheme and host: %s", trimmed)
	}
	c := &Client{
		baseURL: strings.TrimRight(trimmed, "/"),
		httpClient: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
		userAgent: "trustdb-go-sdk",
	}
	for _, apply := range opts {
		apply(c)
	}
	return c, nil
}

func (c *Client) Health(ctx context.Context) error {
	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.getJSON(ctx, "/healthz", nil, &out); err != nil {
		return err
	}
	if !out.OK {
		return &Error{Op: "health", Message: "server returned ok=false"}
	}
	return nil
}

func (c *Client) SubmitFile(ctx context.Context, raw io.Reader, id Identity, opts FileClaimOptions) (SubmitResult, error) {
	signed, err := BuildSignedFileClaim(raw, id, opts)
	if err != nil {
		return SubmitResult{}, err
	}
	result, err := c.SubmitSignedClaim(ctx, signed)
	if err != nil {
		return SubmitResult{}, err
	}
	result.SignedClaim = signed
	return result, nil
}

func (c *Client) SubmitSignedClaim(ctx context.Context, signed SignedClaim) (SubmitResult, error) {
	body, err := cborx.Marshal(signed)
	if err != nil {
		return SubmitResult{}, err
	}
	var env submitClaimEnvelope
	if err := c.doJSON(ctx, http.MethodPost, "/v1/claims", nil, bytes.NewReader(body), "application/cbor", &env); err != nil {
		return SubmitResult{}, err
	}
	return SubmitResult{
		RecordID:        env.RecordID,
		Status:          env.Status,
		ProofLevel:      env.ProofLevel,
		Idempotent:      env.Idempotent,
		BatchEnqueued:   env.BatchEnqueued,
		BatchError:      env.BatchError,
		ServerRecord:    env.ServerRecord,
		AcceptedReceipt: env.AcceptedReceipt,
		SignedClaim:     signed,
	}, nil
}

func (c *Client) GetRecord(ctx context.Context, recordID string) (RecordIndex, error) {
	var idx model.RecordIndex
	if err := c.getJSON(ctx, "/v1/records/"+url.PathEscape(recordID), nil, &idx); err != nil {
		return RecordIndex{}, err
	}
	if idx.RecordID == "" {
		return RecordIndex{}, &Error{Op: "get record", Message: "server returned empty record index"}
	}
	return idx, nil
}

func (c *Client) ListRecords(ctx context.Context, opts ListRecordsOptions) (RecordPage, error) {
	values := url.Values{}
	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}
	values.Set("limit", strconv.Itoa(limit))
	direction := opts.Direction
	if direction == "" {
		direction = model.RecordListDirectionDesc
	}
	values.Set("direction", direction)
	setQuery(values, "cursor", opts.Cursor)
	setQuery(values, "batch_id", opts.BatchID)
	setQuery(values, "tenant_id", opts.TenantID)
	setQuery(values, "client_id", opts.ClientID)
	setQuery(values, "q", opts.Query)
	setQuery(values, "content_hash", opts.ContentHashHex)
	if opts.ReceivedFromUnixN > 0 {
		values.Set("received_from", strconv.FormatInt(opts.ReceivedFromUnixN, 10))
	}
	if opts.ReceivedToUnixN > 0 {
		values.Set("received_to", strconv.FormatInt(opts.ReceivedToUnixN, 10))
	}
	var env recordsEnvelope
	if err := c.getJSON(ctx, "/v1/records", values, &env); err != nil {
		return RecordPage{}, err
	}
	records := make([]RecordIndex, 0, len(env.Records))
	records = append(records, env.Records...)
	return RecordPage{
		Records:    records,
		Limit:      env.Limit,
		Direction:  env.Direction,
		NextCursor: env.NextCursor,
	}, nil
}

func (c *Client) GetProofBundle(ctx context.Context, recordID string) (ProofBundle, error) {
	var env proofEnvelope
	if err := c.getJSON(ctx, "/v1/proofs/"+url.PathEscape(recordID), nil, &env); err != nil {
		return ProofBundle{}, err
	}
	if env.ProofBundle.RecordID == "" {
		return ProofBundle{}, &Error{Op: "get proof bundle", Message: "server returned empty proof bundle"}
	}
	return env.ProofBundle, nil
}

func (c *Client) GetGlobalProof(ctx context.Context, batchID string) (GlobalLogProof, error) {
	var proof model.GlobalLogProof
	if err := c.getJSON(ctx, "/v1/global-log/inclusion/"+url.PathEscape(batchID), nil, &proof); err != nil {
		return GlobalLogProof{}, err
	}
	return proof, nil
}

func (c *Client) GetAnchor(ctx context.Context, treeSize uint64) (AnchorStatus, error) {
	var env anchorEnvelope
	if err := c.getJSON(ctx, "/v1/anchors/sth/"+strconv.FormatUint(treeSize, 10), nil, &env); err != nil {
		return AnchorStatus{}, err
	}
	return AnchorStatus{
		TreeSize: env.TreeSize,
		Status:   env.Status,
		Result:   env.Result,
	}, nil
}

func (c *Client) LatestSTH(ctx context.Context) (SignedTreeHead, error) {
	var sth model.SignedTreeHead
	if err := c.getJSON(ctx, "/v1/sth/latest", nil, &sth); err != nil {
		return SignedTreeHead{}, err
	}
	return sth, nil
}

func (c *Client) GetSTH(ctx context.Context, treeSize uint64) (SignedTreeHead, error) {
	var sth model.SignedTreeHead
	if err := c.getJSON(ctx, "/v1/sth/"+strconv.FormatUint(treeSize, 10), nil, &sth); err != nil {
		return SignedTreeHead{}, err
	}
	return sth, nil
}

func (c *Client) ExportSingleProof(ctx context.Context, recordID string) (SingleProof, error) {
	bundle, err := c.GetProofBundle(ctx, recordID)
	if err != nil {
		return SingleProof{}, err
	}
	opts := sproof.Options{ExportedAtUnixN: time.Now().UTC().UnixNano()}
	global, err := c.GetGlobalProof(ctx, bundle.CommittedReceipt.BatchID)
	if err != nil {
		if !IsUnavailable(err) {
			return SingleProof{}, fmt.Errorf("sdk: fetch global proof: %w", err)
		}
		return sproof.New(bundle, opts)
	}
	opts.GlobalProof = &global
	anchor, err := c.GetAnchor(ctx, global.STH.TreeSize)
	if err != nil {
		if !IsUnavailable(err) {
			return SingleProof{}, fmt.Errorf("sdk: fetch anchor: %w", err)
		}
		return sproof.New(bundle, opts)
	}
	opts.AnchorResult = anchor.Result
	return sproof.New(bundle, opts)
}

func (c *Client) WriteSingleProofFile(ctx context.Context, recordID, path string) error {
	proof, err := c.ExportSingleProof(ctx, recordID)
	if err != nil {
		return err
	}
	return sproof.WriteFile(path, proof)
}

func (c *Client) getJSON(ctx context.Context, path string, query url.Values, dst any) error {
	return c.doJSON(ctx, http.MethodGet, path, query, nil, "", dst)
}

func (c *Client) doJSON(
	ctx context.Context,
	method string,
	path string,
	query url.Values,
	body io.Reader,
	contentType string,
	dst any,
) error {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return &Error{Op: method, URL: endpoint, Err: err}
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &Error{Op: method, URL: endpoint, Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeHTTPError(method, endpoint, resp)
	}
	if dst == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return &Error{Op: method, URL: endpoint, Err: fmt.Errorf("decode json: %w", err)}
	}
	return nil
}

func decodeHTTPError(method, endpoint string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	var env struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &env); err == nil && (env.Code != "" || env.Message != "") {
		return &Error{
			Op:         method,
			URL:        endpoint,
			StatusCode: resp.StatusCode,
			Code:       env.Code,
			Message:    env.Message,
		}
	}
	return &Error{
		Op:         method,
		URL:        endpoint,
		StatusCode: resp.StatusCode,
		Message:    strings.TrimSpace(string(body)),
	}
}

func setQuery(values url.Values, name, value string) {
	if strings.TrimSpace(value) != "" {
		values.Set(name, value)
	}
}

type submitClaimEnvelope struct {
	RecordID        string          `json:"record_id"`
	Status          string          `json:"status"`
	ProofLevel      string          `json:"proof_level"`
	Idempotent      bool            `json:"idempotent"`
	BatchEnqueued   bool            `json:"batch_enqueued"`
	BatchError      string          `json:"batch_error,omitempty"`
	ServerRecord    ServerRecord    `json:"server_record"`
	AcceptedReceipt AcceptedReceipt `json:"accepted_receipt"`
}

type proofEnvelope struct {
	RecordID    string      `json:"record_id"`
	ProofLevel  string      `json:"proof_level"`
	ProofBundle ProofBundle `json:"proof_bundle"`
}

type recordsEnvelope struct {
	Records    []RecordIndex `json:"records"`
	Limit      int           `json:"limit"`
	Direction  string        `json:"direction"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

type anchorEnvelope struct {
	TreeSize uint64           `json:"tree_size"`
	Status   string           `json:"status"`
	Result   *STHAnchorResult `json:"result,omitempty"`
}
