package sproof

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wowtrust/trustdb/internal/anchor/fiscobcos"
	"github.com/wowtrust/trustdb/internal/cryptosuite"
	"github.com/wowtrust/trustdb/internal/globallog"
	"github.com/wowtrust/trustdb/internal/model"
	"github.com/wowtrust/trustdb/internal/trusterr"
)

func TestNewRejectsUnavailableGeneration(t *testing.T) {
	t.Parallel()

	if _, err := New(vectorProof().ProofBundle, Options{ExportedAtUnixN: 1_700_000_000_000_000_000}); trusterr.CodeOf(err) != trusterr.CodeFailedPrecondition || !strings.Contains(err.Error(), "sproof v1 writer is retired") {
		t.Fatalf("New() code=%s error=%v", trusterr.CodeOf(err), err)
	}
}

func TestValidateRejectsAnchorWithoutGlobalProof(t *testing.T) {
	t.Parallel()

	proof := vectorProof()
	proof.AnchorResult = &model.STHAnchorResult{
		SchemaVersion: model.SchemaSTHAnchorResult,
		TreeSize:      1,
		AnchorID:      "anchor-1",
	}
	if err := Validate(proof); err == nil || !strings.Contains(err.Error(), "requires global_proof") {
		t.Fatalf("Validate() error = %v, want global_proof requirement", err)
	}
}

func TestValidateRejectsDriftedEnvelopeMetadata(t *testing.T) {
	t.Parallel()

	proof := vectorProof()
	proof.RecordID = "other-record"
	if err := Validate(proof); err == nil || !strings.Contains(err.Error(), "record_id mismatch") {
		t.Fatalf("Validate() error = %v, want record_id mismatch", err)
	}

	proof = vectorProof()
	proof.ProofLevel = "L5"
	if err := Validate(proof); err == nil || !strings.Contains(err.Error(), "proof_level") {
		t.Fatalf("Validate() error = %v, want proof_level mismatch", err)
	}
}

func TestValidateStrictlyDecodesFISCOBCOSProviderProof(t *testing.T) {
	t.Parallel()

	proof := vectorProof()
	proof.ProofLevel = "L5"
	proof.NodeID = "node-1"
	proof.LogID = "log-1"
	proof.ProofBundle.NodeID = proof.NodeID
	proof.ProofBundle.LogID = proof.LogID
	proof.ProofBundle.CommittedReceipt = model.CommittedReceipt{
		SchemaVersion: model.SchemaCommittedReceipt, CryptoSuite: cryptosuite.INTLV1,
		BatchID: "batch-1", BatchRoot: make([]byte, 32), ClosedAtUnixN: 7,
	}
	proof.ProofBundle.CryptoSuite = cryptosuite.INTLV1
	proof.ProofBundle.BatchProof = model.BatchProof{TreeSize: 1}
	leaf := model.GlobalLogLeaf{
		SchemaVersion: model.SchemaGlobalLogLeaf, CryptoSuite: cryptosuite.INTLV1, NodeID: proof.NodeID, LogID: proof.LogID,
		BatchID: "batch-1", BatchRoot: make([]byte, 32), BatchTreeSize: 1, BatchClosedAtUnixN: 7,
	}
	leafHash, err := globallog.HashLeaf(leaf)
	if err != nil {
		t.Fatal(err)
	}
	sth := model.SignedTreeHead{
		SchemaVersion: model.SchemaSignedTreeHead, CryptoSuite: cryptosuite.INTLV1, TreeAlg: cryptosuite.MerkleRFC6962SHA256,
		TreeSize: 1, RootHash: leafHash, TimestampUnixN: 8, NodeID: proof.NodeID, LogID: proof.LogID,
		Signature: model.Signature{Alg: cryptosuite.SignatureEd25519, KeyID: "server", Signature: []byte{1}},
	}
	proof.GlobalProof = &model.GlobalLogProof{
		SchemaVersion: model.SchemaGlobalLogProof, CryptoSuite: cryptosuite.INTLV1, NodeID: proof.NodeID, LogID: proof.LogID,
		BatchID: "batch-1", LeafIndex: 0, LeafHash: leafHash, TreeSize: 1, STH: sth,
	}
	proof.AnchorResult = &model.STHAnchorResult{
		SchemaVersion: model.SchemaSTHAnchorResult, CryptoSuite: cryptosuite.INTLV1, NodeID: proof.NodeID, LogID: proof.LogID,
		TreeSize: 1, SinkName: fiscobcos.SinkName, AnchorID: strings.Repeat("0", 64),
		RootHash: leafHash, STH: sth, Proof: []byte{0xa1, 0x61, 0x78, 0x01}, PublishedAtUnixN: 9,
	}
	if err := Validate(proof); err == nil || !strings.Contains(err.Error(), "FISCO BCOS") {
		t.Fatalf("Validate() error = %v, want strict FISCO BCOS proof decoding failure", err)
	}
}

func TestSProofV1L3VectorIsRejected(t *testing.T) {
	t.Parallel()

	vectorPath := filepath.Join("..", "..", "test", "vectors", "sproof-v1-l3.cbor")
	fixture, err := os.ReadFile(vectorPath)
	if err != nil {
		t.Fatalf("read vector: %v", err)
	}
	if _, err := Unmarshal(fixture); err == nil {
		t.Fatal("Unmarshal(v1 vector) error = nil, want retired generation rejection")
	}
}

func TestReadFileRejectsOversizedSingleProof(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "oversized.sproof")
	if err := os.WriteFile(path, make([]byte, MaxBytes+1), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := ReadFile(path); err == nil {
		t.Fatal("ReadFile() error = nil, want oversized invalid proof rejection")
	}
}

func TestWriteFileRejectsUnavailableGeneration(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "proof.sproof")
	if err := WriteFile(path, vectorProof()); trusterr.CodeOf(err) != trusterr.CodeFailedPrecondition {
		t.Fatalf("WriteFile() code=%s error=%v", trusterr.CodeOf(err), err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("retired writer produced output: %v", err)
	}
}

func TestWriteFileCleansTemporaryFileOnFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "existing-dir")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := WriteFile(target, vectorProof()); err == nil {
		t.Fatal("WriteFile() error = nil, want failure when target is a directory")
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat(target) error = %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("target was replaced; mode=%s", info.Mode())
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".existing-dir.*.tmp"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files left behind: %v", matches)
	}
}

func vectorProof() model.SingleProof {
	return model.SingleProof{
		SchemaVersion:   model.SchemaSingleProof,
		FormatVersion:   FormatVersion,
		RecordID:        "rec-vector-1",
		ProofLevel:      "L3",
		ExportedAtUnixN: 1_700_000_000_000_000_000,
		ProofBundle: model.ProofBundle{
			SchemaVersion: model.SchemaProofBundle,
			RecordID:      "rec-vector-1",
		},
	}
}
