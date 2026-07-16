package globallog

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/ryan-wong-coder/trustdb/internal/model"
	"github.com/ryan-wong-coder/trustdb/internal/observability"
	"github.com/ryan-wong-coder/trustdb/internal/proofstore"
	"github.com/ryan-wong-coder/trustdb/internal/trustcrypto"
)

func TestOutboxWorkerReschedulesAppendFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := proofstore.LocalStore{Root: t.TempDir()}
	root := model.BatchRoot{
		SchemaVersion: model.SchemaBatchRoot,
		BatchID:       "batch-retry",
		BatchRoot:     bytes.Repeat([]byte{0x42}, 32),
		TreeSize:      1,
		ClosedAtUnixN: 1,
	}
	if err := store.EnqueueGlobalLog(ctx, model.GlobalLogOutboxItem{
		SchemaVersion: model.SchemaGlobalLogOutbox,
		BatchID:       root.BatchID,
		BatchRoot:     root,
		Status:        model.AnchorStatePending,
	}); err != nil {
		t.Fatalf("EnqueueGlobalLog: %v", err)
	}
	readerOnly, err := NewReader(store)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	worker := NewOutboxWorker(OutboxConfig{
		Store:          store,
		Global:         readerOnly,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		Clock:          func() time.Time { return time.Unix(100, 0).UTC() },
	})
	worker.tick(ctx)

	item, ok, err := store.GetGlobalLogOutboxItem(ctx, root.BatchID)
	if err != nil || !ok {
		t.Fatalf("GetGlobalLogOutboxItem ok=%v err=%v", ok, err)
	}
	if item.Status != model.AnchorStatePending || item.Attempts != 1 || item.NextAttemptUnixN == 0 {
		t.Fatalf("item not rescheduled correctly: %+v", item)
	}
	if !strings.Contains(item.LastErrorMessage, "signer") {
		t.Fatalf("last_error = %q, want signer failure", item.LastErrorMessage)
	}
}

func TestOutboxWorkerPublishesAndCallsSTHHook(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := proofstore.LocalStore{Root: t.TempDir()}
	root := model.BatchRoot{
		SchemaVersion: model.SchemaBatchRoot,
		BatchID:       "batch-success",
		BatchRoot:     bytes.Repeat([]byte{0x24}, 32),
		TreeSize:      1,
		ClosedAtUnixN: 1,
	}
	if err := store.EnqueueGlobalLog(ctx, model.GlobalLogOutboxItem{
		SchemaVersion: model.SchemaGlobalLogOutbox,
		BatchID:       root.BatchID,
		BatchRoot:     root,
		Status:        model.AnchorStatePending,
	}); err != nil {
		t.Fatalf("EnqueueGlobalLog: %v", err)
	}
	_, priv, err := trustcrypto.GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key: %v", err)
	}
	svc, err := New(Options{
		Store:      store,
		LogID:      "outbox-test",
		KeyID:      "outbox-key",
		PrivateKey: priv,
		Clock:      func() time.Time { return time.Unix(200, 0).UTC() },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var hookedTreeSize uint64
	worker := NewOutboxWorker(OutboxConfig{
		Store:  store,
		Global: svc,
		OnSTH: func(_ context.Context, sth model.SignedTreeHead) {
			hookedTreeSize = sth.TreeSize
		},
	})
	worker.tick(ctx)

	item, ok, err := store.GetGlobalLogOutboxItem(ctx, root.BatchID)
	if err != nil || !ok {
		t.Fatalf("GetGlobalLogOutboxItem ok=%v err=%v", ok, err)
	}
	if item.Status != model.AnchorStatePublished || item.STH.TreeSize != 1 {
		t.Fatalf("item not published correctly: %+v", item)
	}
	if hookedTreeSize != 1 {
		t.Fatalf("OnSTH tree_size=%d, want 1", hookedTreeSize)
	}
	if _, ok, err := store.GetGlobalLeafByBatchID(ctx, root.BatchID); err != nil || !ok {
		t.Fatalf("GetGlobalLeafByBatchID ok=%v err=%v", ok, err)
	}
}

func TestOutboxWorkerAtomicAnchorPathTriggersExistingOutbox(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := &anchorBatchStore{LocalStore: proofstore.LocalStore{Root: t.TempDir()}}
	root := model.BatchRoot{
		SchemaVersion: model.SchemaBatchRoot,
		BatchID:       "batch-anchor-atomic",
		BatchRoot:     bytes.Repeat([]byte{0x51}, 32),
		TreeSize:      1,
		ClosedAtUnixN: 1,
	}
	if err := store.EnqueueGlobalLog(ctx, model.GlobalLogOutboxItem{
		SchemaVersion: model.SchemaGlobalLogOutbox,
		BatchID:       root.BatchID,
		BatchRoot:     root,
		Status:        model.AnchorStatePending,
	}); err != nil {
		t.Fatalf("EnqueueGlobalLog: %v", err)
	}
	_, priv, err := trustcrypto.GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key: %v", err)
	}
	svc, err := New(Options{
		Store:      store,
		LogID:      "outbox-anchor-test",
		KeyID:      "outbox-anchor-key",
		PrivateKey: priv,
		Clock:      func() time.Time { return time.Unix(300, 0).UTC() },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	metrics := observability.NewMetrics()
	anchorsReady := 0
	legacyHookCalls := 0
	worker := NewOutboxWorker(OutboxConfig{
		Store:        store,
		Global:       svc,
		AnchorOutbox: true,
		Metrics:      metrics,
		OnAnchorsReady: func() {
			anchorsReady++
		},
		OnSTHs: func(context.Context, []model.SignedTreeHead) {
			legacyHookCalls++
		},
	})
	worker.tick(ctx)

	if anchorsReady != 1 {
		t.Fatalf("OnAnchorsReady calls = %d, want 1", anchorsReady)
	}
	if legacyHookCalls != 0 {
		t.Fatalf("OnSTHs calls = %d, want 0", legacyHookCalls)
	}
	if got := testutil.ToFloat64(metrics.GlobalLogPublished); got != 1 {
		t.Fatalf("published roots metric = %v, want 1", got)
	}
	anchorItem, ok, err := store.GetSTHAnchorOutboxItem(ctx, 1)
	if err != nil || !ok || anchorItem.Status != model.AnchorStatePending {
		t.Fatalf("anchor item ok=%v err=%v item=%+v", ok, err, anchorItem)
	}
}

type anchorBatchStore struct {
	proofstore.LocalStore
}

func (s *anchorBatchStore) MarkGlobalLogPublishedBatchWithAnchors(ctx context.Context, batchIDs []string, sths []model.SignedTreeHead, anchors []model.STHAnchorOutboxItem) error {
	for i := range batchIDs {
		if err := s.MarkGlobalLogPublished(ctx, batchIDs[i], sths[i]); err != nil {
			return err
		}
		if err := s.EnqueueSTHAnchor(ctx, anchors[i]); err != nil {
			return err
		}
	}
	return nil
}
