package main

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ryan-wong-coder/trustdb/internal/model"
	"github.com/ryan-wong-coder/trustdb/internal/prooflevel"
	"github.com/ryan-wong-coder/trustdb/sdk"
)

type httpClient struct {
	sdk *sdk.Client
}

func newHTTPClient(base string) (*httpClient, error) {
	client, err := sdk.NewClient(base)
	if err != nil {
		return nil, err
	}
	return &httpClient{sdk: client}, nil
}

type ServerError = sdk.Error

type HealthStatus struct {
	OK         bool   `json:"ok"`
	ServerURL  string `json:"server_url"`
	RTTMillis  int64  `json:"rtt_millis"`
	Error      string `json:"error,omitempty"`
	StatusCode int    `json:"status_code,omitempty"`
}

func (c *httpClient) health(ctx context.Context) HealthStatus {
	status := c.sdk.CheckHealth(ctx)
	return HealthStatus{
		OK:         status.OK,
		ServerURL:  status.ServerURL,
		RTTMillis:  status.RTTMillis,
		Error:      status.Error,
		StatusCode: status.StatusCode,
	}
}

// submitClaimResult mirrors the server's submit response so the UI can inspect
// batch_enqueued / idempotent flags without making a second call.
type submitClaimResult = sdk.SubmitResult

func (c *httpClient) submitSignedClaim(ctx context.Context, signed model.SignedClaim) (submitClaimResult, error) {
	return c.sdk.SubmitSignedClaim(ctx, signed)
}

func (c *httpClient) getProof(ctx context.Context, recordID string) (model.ProofBundle, error) {
	return c.sdk.GetProofBundle(ctx, recordID)
}

func (c *httpClient) getRecordIndex(ctx context.Context, recordID string) (model.RecordIndex, error) {
	return c.sdk.GetRecord(ctx, recordID)
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
		direction = sdk.RecordListDirectionDesc
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

	page, err := c.sdk.ListRecords(ctx, sdk.ListRecordsOptions{
		Limit:          limit,
		Direction:      direction,
		Cursor:         opts.Cursor,
		BatchID:        opts.BatchID,
		TenantID:       opts.TenantID,
		ClientID:       opts.ClientID,
		Query:          query,
		ContentHashHex: contentHash,
	})
	if err != nil {
		return RecordPage{}, err
	}
	items := make([]LocalRecord, 0, len(page.Records))
	for _, idx := range page.Records {
		items = append(items, localRecordFromIndex(idx))
	}
	total := opts.Offset + len(items)
	if page.NextCursor != "" {
		total++
	}
	return RecordPage{
		Items:      items,
		Total:      total,
		Limit:      limit,
		Offset:     opts.Offset,
		HasMore:    page.NextCursor != "",
		NextCursor: page.NextCursor,
		Source:     "server",
		TotalExact: page.NextCursor == "",
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
	proofLevel := prooflevel.L2
	var committed *model.CommittedReceipt
	if idx.BatchID != "" {
		proofLevel = prooflevel.L3
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
		ProofLevel:       proofLevel.String(),
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
	return c.sdk.GetGlobalProof(ctx, batchID)
}

func (c *httpClient) getAnchor(ctx context.Context, treeSize uint64) (anchorEnvelope, error) {
	status, err := c.sdk.GetAnchor(ctx, treeSize)
	if err != nil {
		if sdk.IsUnavailable(err) {
			return anchorEnvelope{TreeSize: treeSize, Status: "unavailable"}, nil
		}
		return anchorEnvelope{}, err
	}
	return anchorEnvelope{TreeSize: status.TreeSize, Status: status.Status, Result: status.Result}, nil
}

func (c *httpClient) listRoots(ctx context.Context, limit int) ([]model.BatchRoot, error) {
	return c.sdk.ListRoots(ctx, limit)
}

func (c *httpClient) latestRoot(ctx context.Context) (model.BatchRoot, error) {
	return c.sdk.LatestRoot(ctx)
}

func (c *httpClient) metricsRaw(ctx context.Context) (string, error) {
	return c.sdk.MetricsRaw(ctx)
}

func (c *httpClient) exportSingleProof(ctx context.Context, recordID string) (model.SingleProof, error) {
	return c.sdk.ExportSingleProof(ctx, recordID)
}
