package verify

import (
	"strings"
	"testing"

	"github.com/ryan-wong-coder/trustdb/internal/anchor"
	"github.com/ryan-wong-coder/trustdb/internal/model"
)

// newGlobalProofWithSTH returns the minimal GlobalLogProof that
// AnchorConsistency needs: the STH/global root that L5 anchored.
func newGlobalProofWithSTH(treeSize uint64, root []byte) model.GlobalLogProof {
	return model.GlobalLogProof{
		SchemaVersion: model.SchemaGlobalLogProof,
		STH: model.SignedTreeHead{
			SchemaVersion: model.SchemaSignedTreeHead,
			TreeAlg:       model.DefaultMerkleTreeAlg,
			TreeSize:      treeSize,
			RootHash:      root,
		},
	}
}

func TestAnchorConsistencyFileSinkOK(t *testing.T) {
	t.Parallel()
	root := []byte{0xaa, 0xbb, 0xcc}
	proof := newGlobalProofWithSTH(7, root)
	ar := model.STHAnchorResult{
		SchemaVersion: model.SchemaSTHAnchorResult,
		TreeSize:      proof.STH.TreeSize,
		SinkName:      anchor.FileSinkName,
		AnchorID:      anchor.DeterministicFileAnchorID(proof.STH),
		RootHash:      root,
		STH:           proof.STH,
	}
	if err := AnchorConsistency(proof, ar); err != nil {
		t.Fatalf("AnchorConsistency: %v", err)
	}
}

func TestAnchorConsistencyNoopSinkOK(t *testing.T) {
	t.Parallel()
	root := []byte{1, 2, 3}
	proof := newGlobalProofWithSTH(8, root)
	ar := model.STHAnchorResult{
		SchemaVersion: model.SchemaSTHAnchorResult,
		TreeSize:      proof.STH.TreeSize,
		SinkName:      anchor.NoopSinkName,
		AnchorID:      anchor.DeterministicNoopAnchorID(proof.STH),
		RootHash:      root,
		STH:           proof.STH,
	}
	if err := AnchorConsistency(proof, ar); err != nil {
		t.Fatalf("AnchorConsistency: %v", err)
	}
}

func TestAnchorConsistencyUnknownSinkSkipsIDCheck(t *testing.T) {
	t.Parallel()
	// Unknown sinks carry their own proof format; AnchorConsistency
	// must pass as long as tree_size / root_hash agree because the
	// sink-specific verifier runs separately.
	root := []byte{9, 9}
	proof := newGlobalProofWithSTH(9, root)
	ar := model.STHAnchorResult{
		SchemaVersion: model.SchemaSTHAnchorResult,
		TreeSize:      proof.STH.TreeSize,
		SinkName:      "ct-log",
		AnchorID:      "arbitrary-opaque-id-from-ct-log",
		RootHash:      root,
		STH:           proof.STH,
	}
	if err := AnchorConsistency(proof, ar); err != nil {
		t.Fatalf("AnchorConsistency (unknown sink): %v", err)
	}
}

func TestAnchorConsistencyRejectsSchema(t *testing.T) {
	t.Parallel()
	proof := newGlobalProofWithSTH(1, []byte{1})
	ar := model.STHAnchorResult{
		SchemaVersion: "bogus",
		TreeSize:      proof.STH.TreeSize,
		SinkName:      anchor.NoopSinkName,
		AnchorID:      anchor.DeterministicNoopAnchorID(proof.STH),
		RootHash:      []byte{1},
	}
	err := AnchorConsistency(proof, ar)
	if err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("want schema error, got %v", err)
	}
}

func TestAnchorConsistencyRejectsTreeSizeMismatch(t *testing.T) {
	t.Parallel()
	proof := newGlobalProofWithSTH(2, []byte{1})
	ar := model.STHAnchorResult{
		SchemaVersion: model.SchemaSTHAnchorResult,
		TreeSize:      3,
		SinkName:      anchor.NoopSinkName,
		AnchorID:      "noop-sth-3",
		RootHash:      []byte{1},
	}
	err := AnchorConsistency(proof, ar)
	if err == nil || !strings.Contains(err.Error(), "tree_size") {
		t.Fatalf("want tree_size error, got %v", err)
	}
}

func TestAnchorConsistencyRejectsRootMismatch(t *testing.T) {
	t.Parallel()
	proof := newGlobalProofWithSTH(4, []byte{1, 2, 3})
	ar := model.STHAnchorResult{
		SchemaVersion: model.SchemaSTHAnchorResult,
		TreeSize:      proof.STH.TreeSize,
		SinkName:      anchor.NoopSinkName,
		AnchorID:      anchor.DeterministicNoopAnchorID(proof.STH),
		RootHash:      []byte{9, 9, 9},
	}
	err := AnchorConsistency(proof, ar)
	if err == nil || !strings.Contains(err.Error(), "root_hash") {
		t.Fatalf("want root_hash error, got %v", err)
	}
}

func TestAnchorConsistencyRejectsFileAnchorIDTamper(t *testing.T) {
	t.Parallel()
	root := []byte{1, 2}
	proof := newGlobalProofWithSTH(5, root)
	ar := model.STHAnchorResult{
		SchemaVersion: model.SchemaSTHAnchorResult,
		TreeSize:      proof.STH.TreeSize,
		SinkName:      anchor.FileSinkName,
		AnchorID:      "file-tampered-0000000000000000000",
		RootHash:      root,
	}
	err := AnchorConsistency(proof, ar)
	if err == nil || !strings.Contains(err.Error(), "file sink anchor_id") {
		t.Fatalf("want anchor_id mismatch error, got %v", err)
	}
}

func TestAnchorConsistencyRejectsEmptyAnchorID(t *testing.T) {
	t.Parallel()
	proof := newGlobalProofWithSTH(6, []byte{1})
	ar := model.STHAnchorResult{
		SchemaVersion: model.SchemaSTHAnchorResult,
		TreeSize:      proof.STH.TreeSize,
		SinkName:      anchor.FileSinkName,
		RootHash:      []byte{1},
	}
	err := AnchorConsistency(proof, ar)
	if err == nil || !strings.Contains(err.Error(), "anchor_id") {
		t.Fatalf("want anchor_id missing error, got %v", err)
	}
}
