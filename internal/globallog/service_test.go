package globallog

import (
	"bytes"
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ryan-wong-coder/trustdb/internal/model"
	"github.com/ryan-wong-coder/trustdb/internal/proofstore"
	"github.com/ryan-wong-coder/trustdb/internal/trustcrypto"
)

func newTestService(t testing.TB) (*Service, proofstore.LocalStore) {
	t.Helper()
	_, priv, err := trustcrypto.GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key: %v", err)
	}
	store := proofstore.LocalStore{Root: t.TempDir()}
	svc, err := New(Options{
		Store:      store,
		LogID:      "test-log",
		KeyID:      "test-key",
		PrivateKey: priv,
		Clock:      func() time.Time { return time.Unix(100, 0).UTC() },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc, store
}

func batchRoot(id string, seed byte) model.BatchRoot {
	root := bytes.Repeat([]byte{seed}, 32)
	return model.BatchRoot{
		SchemaVersion: model.SchemaBatchRoot,
		BatchID:       id,
		BatchRoot:     root,
		TreeSize:      uint64(seed),
		ClosedAtUnixN: int64(seed),
	}
}

func TestAppendBatchRootProducesStableSTHAndInclusionProof(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, store := newTestService(t)

	var latest model.SignedTreeHead
	for _, root := range []model.BatchRoot{
		batchRoot("b1", 1),
		batchRoot("b2", 2),
		batchRoot("b3", 3),
	} {
		sth, err := svc.AppendBatchRoot(ctx, root)
		if err != nil {
			t.Fatalf("AppendBatchRoot(%s): %v", root.BatchID, err)
		}
		latest = sth
	}
	if latest.TreeSize != 3 {
		t.Fatalf("latest tree_size = %d, want 3", latest.TreeSize)
	}
	again, err := svc.AppendBatchRoot(ctx, batchRoot("b2", 2))
	if err != nil {
		t.Fatalf("AppendBatchRoot duplicate: %v", err)
	}
	if again.TreeSize != 2 {
		t.Fatalf("duplicate append returned tree_size=%d, want original STH size 2", again.TreeSize)
	}
	leaves, err := store.ListGlobalLeaves(ctx)
	if err != nil {
		t.Fatalf("ListGlobalLeaves: %v", err)
	}
	if len(leaves) != 3 {
		t.Fatalf("global leaves = %d, want 3", len(leaves))
	}

	proof, err := svc.InclusionProof(ctx, "b2", latest.TreeSize)
	if err != nil {
		t.Fatalf("InclusionProof: %v", err)
	}
	if !VerifyInclusion(proof) {
		t.Fatal("VerifyInclusion returned false")
	}
	if proof.STH.TreeSize != latest.TreeSize || !bytes.Equal(proof.STH.RootHash, latest.RootHash) {
		t.Fatalf("proof STH = %+v, want latest %+v", proof.STH, latest)
	}
}

func TestCompactHistoryPreservesInclusionProof(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, store := newTestService(t)
	for _, root := range []model.BatchRoot{
		batchRoot("b1", 1),
		batchRoot("b2", 2),
		batchRoot("b3", 3),
	} {
		if _, err := svc.AppendBatchRoot(ctx, root); err != nil {
			t.Fatalf("AppendBatchRoot: %v", err)
		}
	}
	before, err := svc.InclusionProof(ctx, "b1", 3)
	if err != nil {
		t.Fatalf("InclusionProof before compact: %v", err)
	}
	written, err := svc.CompactHistory(ctx, 2)
	if err != nil {
		t.Fatalf("CompactHistory: %v", err)
	}
	if written != 2 {
		t.Fatalf("tiles written = %d, want 2", written)
	}
	tiles, err := store.ListGlobalLogTiles(ctx)
	if err != nil {
		t.Fatalf("ListGlobalLogTiles: %v", err)
	}
	if len(tiles) != 2 {
		t.Fatalf("tiles = %d, want 2", len(tiles))
	}
	after, err := svc.InclusionProof(ctx, "b1", 3)
	if err != nil {
		t.Fatalf("InclusionProof after compact: %v", err)
	}
	if !bytes.Equal(before.LeafHash, after.LeafHash) || !bytes.Equal(before.STH.RootHash, after.STH.RootHash) {
		t.Fatalf("proof changed after compact: before=%+v after=%+v", before, after)
	}
}

func TestConsistencyProofMatchesSmallTreeReference(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, _ := newTestService(t)
	for i := 1; i <= 9; i++ {
		if _, err := svc.AppendBatchRoot(ctx, batchRoot("b"+string(rune('0'+i)), byte(i))); err != nil {
			t.Fatalf("AppendBatchRoot(%d): %v", i, err)
		}
	}
	leaves, err := svc.leafHashes(ctx, 9)
	if err != nil {
		t.Fatalf("leafHashes: %v", err)
	}
	for from := uint64(1); from <= 9; from++ {
		got, err := svc.ConsistencyProof(ctx, from, 9)
		if err != nil {
			t.Fatalf("ConsistencyProof(%d,9): %v", from, err)
		}
		want, err := consistencyProof(leaves, from)
		if err != nil {
			t.Fatalf("reference consistencyProof(%d): %v", from, err)
		}
		if len(got.AuditPath) != len(want) {
			t.Fatalf("path len for from=%d got=%d want=%d", from, len(got.AuditPath), len(want))
		}
		for i := range want {
			if !bytes.Equal(got.AuditPath[i], want[i]) {
				t.Fatalf("path[%d] for from=%d changed", i, from)
			}
		}
	}
}

func TestConsistencyProofUsesIndexedNodesInsteadOfFullLeafScan(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, store := newTestService(t)
	const treeSize = 512
	for i := 1; i <= treeSize; i++ {
		if _, err := svc.AppendBatchRoot(ctx, batchRoot("b"+strconv.Itoa(i), byte(i%255+1))); err != nil {
			t.Fatalf("AppendBatchRoot(%d): %v", i, err)
		}
	}

	counting := &countingGlobalStore{LocalStore: store}
	reader, err := NewReader(counting)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	proof, err := reader.ConsistencyProof(ctx, 256, treeSize)
	if err != nil {
		t.Fatalf("ConsistencyProof: %v", err)
	}
	if len(proof.AuditPath) == 0 {
		t.Fatal("expected non-empty consistency path")
	}
	if got := counting.leafReads.Load(); got > 8 {
		t.Fatalf("ConsistencyProof read %d leaves for tree_size=%d; want indexed-node path, not a full scan", got, treeSize)
	}
}

func BenchmarkConsistencyProofLargeTree(b *testing.B) {
	ctx := context.Background()
	svc, _ := newTestService(b)
	const treeSize = 8192
	for i := 1; i <= treeSize; i++ {
		if _, err := svc.AppendBatchRoot(ctx, batchRoot("bench-"+strconv.Itoa(i), byte(i%255+1))); err != nil {
			b.Fatalf("AppendBatchRoot(%d): %v", i, err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.ConsistencyProof(ctx, 4096, treeSize); err != nil {
			b.Fatalf("ConsistencyProof: %v", err)
		}
	}
}

type countingGlobalStore struct {
	proofstore.LocalStore
	leafReads atomic.Int64
}

func (s *countingGlobalStore) GetGlobalLeaf(ctx context.Context, index uint64) (model.GlobalLogLeaf, bool, error) {
	s.leafReads.Add(1)
	return s.LocalStore.GetGlobalLeaf(ctx, index)
}
