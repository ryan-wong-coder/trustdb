package batch

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/ryan-wong-coder/trustdb/internal/model"
	"github.com/ryan-wong-coder/trustdb/internal/observability"
	"github.com/ryan-wong-coder/trustdb/internal/proofstore"
	"github.com/ryan-wong-coder/trustdb/internal/trusterr"
)

const asyncProofWaitTimeout = 10 * time.Second

func TestServiceCommitsFullBatch(t *testing.T) {
	t.Parallel()

	store := proofstore.LocalStore{Root: t.TempDir()}
	svc := New(fakeEngine{}, store, Options{QueueSize: 4, MaxRecords: 2, MaxDelay: time.Hour}, nil)
	defer svc.Shutdown(context.Background())

	if err := svc.Enqueue(context.Background(), signed("tr1a"), record("tr1a"), accepted("tr1a")); err != nil {
		t.Fatalf("Enqueue(a) error = %v", err)
	}
	if err := svc.Enqueue(context.Background(), signed("tr1b"), record("tr1b"), accepted("tr1b")); err != nil {
		t.Fatalf("Enqueue(b) error = %v", err)
	}

	got := waitForProof(t, svc, "tr1b")
	if got.RecordID != "tr1b" || got.BatchProof.TreeSize != 2 {
		t.Fatalf("Proof() = %+v", got)
	}
	// persistBatch writes bundles before the root, so the bundle becoming
	// visible to Proof() does not yet imply the root file has landed.
	// Poll briefly to close that window instead of asserting immediately.
	root := waitForLatestRoot(t, svc, 2)
	if !bytes.Equal(root.BatchRoot, bytes.Repeat([]byte{9}, 32)) {
		t.Fatalf("LatestRoot() = %+v", root)
	}
}

func TestServiceShutdownFlushesPartialBatch(t *testing.T) {
	t.Parallel()

	store := proofstore.LocalStore{Root: t.TempDir()}
	svc := New(fakeEngine{}, store, Options{QueueSize: 4, MaxRecords: 10, MaxDelay: time.Hour}, nil)
	if err := svc.Enqueue(context.Background(), signed("tr1partial"), record("tr1partial"), accepted("tr1partial")); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	if err := svc.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	got, err := svc.Proof(context.Background(), "tr1partial")
	if err != nil {
		t.Fatalf("Proof() error = %v", err)
	}
	if got.BatchProof.TreeSize != 1 {
		t.Fatalf("Proof() = %+v", got)
	}
}

func TestServiceRejectsFullQueue(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})
	entered := make(chan struct{}, 1)
	svc := New(blockingEngine{block: block, entered: entered}, proofstore.LocalStore{Root: t.TempDir()}, Options{QueueSize: 1, MaxRecords: 1, MaxDelay: time.Hour}, nil)
	defer func() {
		close(block)
		_ = svc.Shutdown(context.Background())
	}()
	if err := svc.Enqueue(context.Background(), signed("tr1a"), record("tr1a"), accepted("tr1a")); err != nil {
		t.Fatalf("Enqueue(a) error = %v", err)
	}
	<-entered
	if err := svc.Enqueue(context.Background(), signed("tr1b"), record("tr1b"), accepted("tr1b")); err != nil {
		t.Fatalf("Enqueue(b) error = %v", err)
	}
	err := svc.Enqueue(context.Background(), signed("tr1c"), record("tr1c"), accepted("tr1c"))
	if trusterr.CodeOf(err) != trusterr.CodeResourceExhausted {
		t.Fatalf("Enqueue(c) code = %s err=%v", trusterr.CodeOf(err), err)
	}
}

// TestServiceAdvancesCheckpointAfterCommit verifies that a successful batch
// commit persists a WAL checkpoint whose LastSequence equals the maximum
// sequence among the committed records, so future restarts can skip those
// records when replaying the WAL.
func TestServiceAdvancesCheckpointAfterCommit(t *testing.T) {
	t.Parallel()

	store := proofstore.LocalStore{Root: t.TempDir()}
	svc := New(fakeEngine{}, store, Options{QueueSize: 4, MaxRecords: 2, MaxDelay: time.Hour}, nil)
	defer svc.Shutdown(context.Background())

	if err := svc.Enqueue(context.Background(), signed("rec-1"), recordWithWAL("rec-1", 10), accepted("rec-1")); err != nil {
		t.Fatalf("Enqueue(a) error = %v", err)
	}
	if err := svc.Enqueue(context.Background(), signed("rec-2"), recordWithWAL("rec-2", 42), accepted("rec-2")); err != nil {
		t.Fatalf("Enqueue(b) error = %v", err)
	}
	waitForProof(t, svc, "rec-2")

	cp := waitForCheckpoint(t, store, 42)
	if cp.SegmentID != 1 || cp.LastOffset != 42*128 {
		t.Fatalf("GetCheckpoint() = %+v", cp)
	}
}

// TestServiceCheckpointMonotonic verifies that advancing with a lower
// sequence does not regress the checkpoint, which protects against
// out-of-order commits during crash recovery (RecoverManifest may process an
// older prepared manifest after newer batches have already advanced the
// checkpoint).
func TestServiceCheckpointMonotonic(t *testing.T) {
	t.Parallel()

	store := proofstore.LocalStore{Root: t.TempDir()}
	svc := New(fakeEngine{}, store, Options{QueueSize: 4, MaxRecords: 1, MaxDelay: time.Hour}, nil)
	defer svc.Shutdown(context.Background())

	if err := svc.Enqueue(context.Background(), signed("rec-high"), recordWithWAL("rec-high", 99), accepted("rec-high")); err != nil {
		t.Fatalf("Enqueue(high) error = %v", err)
	}
	waitForProof(t, svc, "rec-high")

	if err := svc.Enqueue(context.Background(), signed("rec-low"), recordWithWAL("rec-low", 5), accepted("rec-low")); err != nil {
		t.Fatalf("Enqueue(low) error = %v", err)
	}
	waitForProof(t, svc, "rec-low")

	// rec-low committed after rec-high, but checkpoint must remain at 99.
	// Give the worker a moment to observe the PutManifest for rec-low
	// before asserting the checkpoint did not regress.
	time.Sleep(20 * time.Millisecond)
	cp, found, err := store.GetCheckpoint(context.Background())
	if err != nil || !found {
		t.Fatalf("GetCheckpoint() err=%v found=%v", err, found)
	}
	if cp.LastSequence != 99 {
		t.Fatalf("GetCheckpoint() LastSequence = %d, want 99 (never regress)", cp.LastSequence)
	}
	if cp.BatchID == "" {
		t.Fatalf("GetCheckpoint() BatchID empty, want the first batch that advanced to 99")
	}
}

// TestServiceSkipsCheckpointOnZeroSequence keeps the store clean when a batch
// completes with no valid WAL positions (e.g. tests using legacy helpers that
// default to sequence 0). This prevents us from writing a meaningless
// checkpoint that would otherwise confuse downstream replays.
func TestServiceSkipsCheckpointOnZeroSequence(t *testing.T) {
	t.Parallel()

	store := proofstore.LocalStore{Root: t.TempDir()}
	svc := New(fakeEngine{}, store, Options{QueueSize: 4, MaxRecords: 1, MaxDelay: time.Hour}, nil)
	defer svc.Shutdown(context.Background())

	if err := svc.Enqueue(context.Background(), signed("rec"), record("rec"), accepted("rec")); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	waitForProof(t, svc, "rec")

	_, found, err := store.GetCheckpoint(context.Background())
	if err != nil {
		t.Fatalf("GetCheckpoint() error = %v", err)
	}
	if found {
		t.Fatalf("GetCheckpoint() found = true, want no checkpoint when WAL sequences are zero")
	}
}

// TestServiceUpdatesCheckpointGauge verifies that advancing the checkpoint
// also updates the exposed prometheus gauge so dashboards track the batcher's
// progress without polling the proof store.
func TestServiceUpdatesCheckpointGauge(t *testing.T) {
	t.Parallel()

	_, metrics := observability.NewRegistry()
	store := proofstore.LocalStore{Root: t.TempDir()}
	svc := New(fakeEngine{}, store, Options{QueueSize: 4, MaxRecords: 1, MaxDelay: time.Hour}, metrics)
	defer svc.Shutdown(context.Background())

	if err := svc.Enqueue(context.Background(), signed("rec-gauge"), recordWithWAL("rec-gauge", 77), accepted("rec-gauge")); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	waitForProof(t, svc, "rec-gauge")
	waitForCheckpoint(t, store, 77)

	// The gauge is updated inside advanceCheckpoint after the proof store
	// persist succeeds, so a post-proof poll may race. Give the batch
	// worker a short window to publish before asserting.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if testutil.ToFloat64(metrics.WALCheckpointLastSequence) == 77 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := testutil.ToFloat64(metrics.WALCheckpointLastSequence); got != 77 {
		t.Fatalf("wal_checkpoint_last_sequence = %v, want 77", got)
	}
}

// TestServiceInvokesOnCheckpointAdvanced verifies the hook is called with
// each advanced checkpoint and is only called when the checkpoint actually
// moves forward (monotonic advance; the hook is a best-effort observer and
// should not fire for no-op advances).
func TestServiceInvokesOnCheckpointAdvanced(t *testing.T) {
	t.Parallel()

	var (
		hookMu   sync.Mutex
		hookCPs  []model.WALCheckpoint
		hookFire = make(chan struct{}, 8)
	)
	store := proofstore.LocalStore{Root: t.TempDir()}
	svc := New(fakeEngine{}, store, Options{
		QueueSize:  4,
		MaxRecords: 1,
		MaxDelay:   time.Hour,
		OnCheckpointAdvanced: func(_ context.Context, cp model.WALCheckpoint) {
			hookMu.Lock()
			hookCPs = append(hookCPs, cp)
			hookMu.Unlock()
			hookFire <- struct{}{}
		},
	}, nil)
	defer svc.Shutdown(context.Background())

	if err := svc.Enqueue(context.Background(), signed("rec-a"), recordWithWAL("rec-a", 10), accepted("rec-a")); err != nil {
		t.Fatalf("Enqueue(a) error = %v", err)
	}
	waitForProof(t, svc, "rec-a")
	waitForCheckpoint(t, store, 10)
	<-hookFire

	if err := svc.Enqueue(context.Background(), signed("rec-b"), recordWithWAL("rec-b", 20), accepted("rec-b")); err != nil {
		t.Fatalf("Enqueue(b) error = %v", err)
	}
	waitForProof(t, svc, "rec-b")
	waitForCheckpoint(t, store, 20)
	<-hookFire

	// rec-regress has a lower sequence than the existing checkpoint: the
	// service must not invoke the hook for a no-op advance so the prune
	// side does not observe a phantom checkpoint advance.
	if err := svc.Enqueue(context.Background(), signed("rec-regress"), recordWithWAL("rec-regress", 5), accepted("rec-regress")); err != nil {
		t.Fatalf("Enqueue(regress) error = %v", err)
	}
	waitForProof(t, svc, "rec-regress")
	time.Sleep(20 * time.Millisecond)

	hookMu.Lock()
	defer hookMu.Unlock()
	if len(hookCPs) != 2 {
		t.Fatalf("hook fired %d times, want 2 (regress must not advance)", len(hookCPs))
	}
	if hookCPs[0].LastSequence != 10 || hookCPs[1].LastSequence != 20 {
		t.Fatalf("hook checkpoints = %+v, want [10,20]", hookCPs)
	}
}

func waitForCheckpoint(t *testing.T, store proofstore.LocalStore, wantSeq uint64) model.WALCheckpoint {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		cp, found, err := store.GetCheckpoint(context.Background())
		if err == nil && found && cp.LastSequence >= wantSeq {
			return cp
		}
		time.Sleep(5 * time.Millisecond)
	}
	cp, found, err := store.GetCheckpoint(context.Background())
	t.Fatalf("GetCheckpoint() after wait = %+v found=%v err=%v (want LastSequence >= %d)", cp, found, err, wantSeq)
	return model.WALCheckpoint{}
}

// waitForLatestRoot polls LatestRoot until the committed batch has published
// its root (bundles land before the root inside persistBatch, so callers
// that observed a proof cannot assume the root is immediately readable).
func waitForLatestRoot(t *testing.T, svc *Service, wantTreeSize uint64) model.BatchRoot {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		root, err := svc.LatestRoot(context.Background())
		if err == nil && root.TreeSize == wantTreeSize {
			return root
		}
		time.Sleep(5 * time.Millisecond)
	}
	root, err := svc.LatestRoot(context.Background())
	t.Fatalf("LatestRoot() after wait = %+v err=%v (want TreeSize=%d)", root, err, wantTreeSize)
	return model.BatchRoot{}
}

func waitForProof(t *testing.T, svc *Service, recordID string) model.ProofBundle {
	t.Helper()
	deadline := time.Now().Add(asyncProofWaitTimeout)
	for time.Now().Before(deadline) {
		got, err := svc.Proof(context.Background(), recordID)
		if err == nil {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	got, err := svc.Proof(context.Background(), recordID)
	t.Fatalf("Proof(%q) after %s = %+v err=%v lastErr=%v", recordID, asyncProofWaitTimeout, got, err, svc.LastError())
	return model.ProofBundle{}
}

type fakeEngine struct{}

func (fakeEngine) CommitBatch(batchID string, closedAt time.Time, signed []model.SignedClaim, records []model.ServerRecord, accepted []model.AcceptedReceipt) ([]model.ProofBundle, error) {
	out := make([]model.ProofBundle, len(records))
	root := bytes.Repeat([]byte{9}, 32)
	for i := range records {
		out[i] = model.ProofBundle{
			SchemaVersion:   model.SchemaProofBundle,
			RecordID:        records[i].RecordID,
			SignedClaim:     signed[i],
			ServerRecord:    records[i],
			AcceptedReceipt: accepted[i],
			CommittedReceipt: model.CommittedReceipt{
				SchemaVersion: model.SchemaCommittedReceipt,
				RecordID:      records[i].RecordID,
				Status:        "committed",
				BatchID:       batchID,
				LeafIndex:     uint64(i),
				BatchRoot:     root,
				ClosedAtUnixN: closedAt.UnixNano(),
			},
			BatchProof: model.BatchProof{
				TreeAlg:   model.DefaultMerkleTreeAlg,
				LeafIndex: uint64(i),
				TreeSize:  uint64(len(records)),
			},
		}
	}
	return out, nil
}

type blockingEngine struct {
	block   chan struct{}
	entered chan struct{}
}

func (e blockingEngine) CommitBatch(batchID string, closedAt time.Time, signed []model.SignedClaim, records []model.ServerRecord, accepted []model.AcceptedReceipt) ([]model.ProofBundle, error) {
	select {
	case e.entered <- struct{}{}:
	default:
	}
	<-e.block
	return fakeEngine{}.CommitBatch(batchID, closedAt, signed, records, accepted)
}

func signed(recordID string) model.SignedClaim {
	return model.SignedClaim{SchemaVersion: model.SchemaSignedClaim, Claim: model.ClientClaim{IdempotencyKey: recordID}}
}

func record(recordID string) model.ServerRecord {
	return model.ServerRecord{SchemaVersion: model.SchemaServerRecord, RecordID: recordID}
}

func recordWithWAL(recordID string, seq uint64) model.ServerRecord {
	return model.ServerRecord{
		SchemaVersion: model.SchemaServerRecord,
		RecordID:      recordID,
		WAL:           model.WALPosition{SegmentID: 1, Offset: int64(seq) * 128, Sequence: seq},
	}
}

func accepted(recordID string) model.AcceptedReceipt {
	return model.AcceptedReceipt{SchemaVersion: model.SchemaAcceptedReceipt, RecordID: recordID, Status: "accepted"}
}

// TestServiceInitialSeqResumesSuffix locks in the cross-restart fix
// for the "every fresh server emits batch_id ending in -000001"
// regression. With InitialSeq=42, the very first batch this service
// commits must be -000043 (counter is bumped before formatting).
func TestServiceInitialSeqResumesSuffix(t *testing.T) {
	t.Parallel()

	store := proofstore.LocalStore{Root: t.TempDir()}
	svc := New(fakeEngine{}, store, Options{
		QueueSize:  4,
		MaxRecords: 1, // commit immediately, no batching delay
		MaxDelay:   time.Hour,
		InitialSeq: 42,
	}, nil)
	defer svc.Shutdown(context.Background())

	if err := svc.Enqueue(context.Background(), signed("seq-a"), record("seq-a"), accepted("seq-a")); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	root := waitForLatestRoot(t, svc, 1)
	const want = "-000043"
	if !strings.HasSuffix(root.BatchID, want) {
		t.Fatalf("first batch_id = %q, want suffix %q", root.BatchID, want)
	}

	// A second commit on the same Service should keep climbing — the
	// in-memory counter is preserved across commits regardless of
	// where the seed came from.
	if err := svc.Enqueue(context.Background(), signed("seq-b"), record("seq-b"), accepted("seq-b")); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	// Wait for the second bundle to materialize via Proof, then read
	// the corresponding batch root by listing all roots and picking
	// the second one (LatestRoot only returns the newest, which is
	// what we want anyway, but TreeSize is still 1 for either, so
	// we have to disambiguate by batch_id suffix).
	got := waitForProof(t, svc, "seq-b")
	const wantNext = "-000044"
	if !strings.HasSuffix(got.CommittedReceipt.BatchID, wantNext) {
		t.Fatalf("second batch_id = %q, want suffix %q", got.CommittedReceipt.BatchID, wantNext)
	}
}

// TestServiceInitialSeqZeroPreservesLegacyBehaviour ensures the
// default zero value of InitialSeq still produces -000001 on the
// very first batch, matching every existing test/operator.
func TestServiceInitialSeqZeroPreservesLegacyBehaviour(t *testing.T) {
	t.Parallel()

	store := proofstore.LocalStore{Root: t.TempDir()}
	svc := New(fakeEngine{}, store, Options{
		QueueSize:  4,
		MaxRecords: 1,
		MaxDelay:   time.Hour,
	}, nil)
	defer svc.Shutdown(context.Background())

	if err := svc.Enqueue(context.Background(), signed("seq-zero"), record("seq-zero"), accepted("seq-zero")); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	root := waitForLatestRoot(t, svc, 1)
	const want = "-000001"
	if !strings.HasSuffix(root.BatchID, want) {
		t.Fatalf("first batch_id = %q, want suffix %q", root.BatchID, want)
	}
}
