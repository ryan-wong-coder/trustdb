package globallog

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ryan-wong-coder/trustdb/internal/model"
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
