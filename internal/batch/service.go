package batch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ryan-wong-coder/trustdb/internal/model"
	"github.com/ryan-wong-coder/trustdb/internal/observability"
	"github.com/ryan-wong-coder/trustdb/internal/trusterr"
)

type Engine interface {
	CommitBatch(batchID string, closedAt time.Time, signed []model.SignedClaim, records []model.ServerRecord, accepted []model.AcceptedReceipt) ([]model.ProofBundle, error)
}

type Store interface {
	PutBundle(context.Context, model.ProofBundle) error
	GetBundle(context.Context, string) (model.ProofBundle, error)
	GetRecordIndex(context.Context, string) (model.RecordIndex, bool, error)
	ListRecordIndexes(context.Context, model.RecordListOptions) ([]model.RecordIndex, error)
	PutRoot(context.Context, model.BatchRoot) error
	ListRoots(context.Context, int) ([]model.BatchRoot, error)
	ListRootsAfter(context.Context, int64, int) ([]model.BatchRoot, error)
	LatestRoot(context.Context) (model.BatchRoot, error)
	PutManifest(context.Context, model.BatchManifest) error
	GetManifest(context.Context, string) (model.BatchManifest, error)
	ListManifests(context.Context) ([]model.BatchManifest, error)
	PutCheckpoint(context.Context, model.WALCheckpoint) error
	GetCheckpoint(context.Context) (model.WALCheckpoint, bool, error)
}

type Accepted struct {
	Signed   model.SignedClaim
	Record   model.ServerRecord
	Accepted model.AcceptedReceipt
}

type Options struct {
	QueueSize  int
	MaxRecords int
	MaxDelay   time.Duration
	// InitialSeq seeds the in-memory batch sequence counter so that
	// batch_id suffixes keep increasing across restarts. Callers
	// typically derive it from the latest persisted BatchRoot via
	// ParseBatchSeq; leaving it at 0 means "first batch starts at
	// -000001", which is the legacy behaviour from before we
	// persisted the counter.
	//
	// The counter is still process-local — it disambiguates batches
	// inside the same nanosecond, which the single-goroutine worker
	// can't actually produce, so the field is mainly cosmetic. But
	// without restoring it, every server restart resets the suffix
	// to -000001 and the human reading the proof bundle thinks two
	// unrelated batches share an ID.
	InitialSeq uint64
	// OnCheckpointAdvanced is called after a successful advanceCheckpoint
	// step with the newly persisted checkpoint. It runs on the batch
	// worker goroutine so it must not block on IO that could stall
	// subsequent batches — wire async prune or metric updates here, not
	// synchronous network calls. Errors from the hook are not returned
	// because checkpoint advancement is a best-effort optimization.
	OnCheckpointAdvanced func(context.Context, model.WALCheckpoint)
	// OnBatchCommitted fires after a batch is fully persisted (manifest
	// committed, bundles + root written, checkpoint advanced) with the
	// BatchRoot that was just stored. The serve command uses this hook only
	// to persist a durable global-log outbox event and trigger the separate
	// outbox worker; global append and L5 anchoring must remain outside the
	// batch goroutine. The hook must not block on slow IO for the same
	// reason as OnCheckpointAdvanced.
	OnBatchCommitted func(context.Context, model.BatchRoot)
}

type Service struct {
	engine  Engine
	store   Store
	metrics *observability.Metrics
	queue   chan Accepted
	opts    Options

	mu      sync.RWMutex
	closed  bool
	lastErr error
	wg      sync.WaitGroup
	seq     uint64
}

func New(engine Engine, store Store, opts Options, metrics *observability.Metrics) *Service {
	if opts.QueueSize <= 0 {
		opts.QueueSize = 1024
	}
	if opts.MaxRecords <= 0 {
		opts.MaxRecords = 1024
	}
	if opts.MaxDelay <= 0 {
		opts.MaxDelay = 500 * time.Millisecond
	}
	s := &Service{
		engine:  engine,
		store:   store,
		metrics: metrics,
		queue:   make(chan Accepted, opts.QueueSize),
		opts:    opts,
		seq:     opts.InitialSeq,
	}
	s.wg.Add(1)
	go s.worker()
	return s
}

func (s *Service) Enqueue(ctx context.Context, signed model.SignedClaim, record model.ServerRecord, accepted model.AcceptedReceipt) error {
	if err := ctx.Err(); err != nil {
		return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "batch enqueue canceled", err)
	}
	item := Accepted{Signed: signed, Record: record, Accepted: accepted}
	return s.enqueue(ctx, item, false)
}

func (s *Service) EnqueueRecovered(ctx context.Context, item Accepted) error {
	if err := ctx.Err(); err != nil {
		return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "batch enqueue canceled", err)
	}
	return s.enqueue(ctx, item, true)
}

func (s *Service) enqueue(ctx context.Context, item Accepted, wait bool) error {
	for {
		s.mu.RLock()
		if s.closed {
			s.mu.RUnlock()
			return trusterr.New(trusterr.CodeFailedPrecondition, "batch service is shutting down")
		}
		select {
		case s.queue <- item:
			s.setQueueDepth()
			s.mu.RUnlock()
			return nil
		default:
			s.mu.RUnlock()
		}

		if !wait {
			return trusterr.New(trusterr.CodeResourceExhausted, "batch queue is full")
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "batch enqueue canceled", ctx.Err())
		}
	}
}

func (s *Service) Proof(ctx context.Context, recordID string) (model.ProofBundle, error) {
	if s.store == nil {
		return model.ProofBundle{}, trusterr.New(trusterr.CodeFailedPrecondition, "proof store is not configured")
	}
	return s.store.GetBundle(ctx, recordID)
}

func (s *Service) RecordIndex(ctx context.Context, recordID string) (model.RecordIndex, bool, error) {
	if s.store == nil {
		return model.RecordIndex{}, false, trusterr.New(trusterr.CodeFailedPrecondition, "proof store is not configured")
	}
	return s.store.GetRecordIndex(ctx, recordID)
}

func (s *Service) Records(ctx context.Context, opts model.RecordListOptions) ([]model.RecordIndex, error) {
	if s.store == nil {
		return nil, trusterr.New(trusterr.CodeFailedPrecondition, "proof store is not configured")
	}
	return s.store.ListRecordIndexes(ctx, opts)
}

func (s *Service) Roots(ctx context.Context, limit int) ([]model.BatchRoot, error) {
	if s.store == nil {
		return nil, trusterr.New(trusterr.CodeFailedPrecondition, "proof store is not configured")
	}
	return s.store.ListRoots(ctx, limit)
}

func (s *Service) RootsAfter(ctx context.Context, afterClosedAtUnixN int64, limit int) ([]model.BatchRoot, error) {
	if s.store == nil {
		return nil, trusterr.New(trusterr.CodeFailedPrecondition, "proof store is not configured")
	}
	return s.store.ListRootsAfter(ctx, afterClosedAtUnixN, limit)
}

func (s *Service) LatestRoot(ctx context.Context) (model.BatchRoot, error) {
	if s.store == nil {
		return model.BatchRoot{}, trusterr.New(trusterr.CodeFailedPrecondition, "proof store is not configured")
	}
	return s.store.LatestRoot(ctx)
}

func (s *Service) Manifests(ctx context.Context) ([]model.BatchManifest, error) {
	if s.store == nil {
		return nil, trusterr.New(trusterr.CodeFailedPrecondition, "proof store is not configured")
	}
	return s.store.ListManifests(ctx)
}

func (s *Service) LastError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastErr
}

func (s *Service) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		close(s.queue)
	}
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		s.setQueueDepth()
		return nil
	case <-ctx.Done():
		return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "batch shutdown timed out", ctx.Err())
	}
}

func (s *Service) worker() {
	defer s.wg.Done()

	batch := make([]Accepted, 0, s.opts.MaxRecords)
	timer := time.NewTimer(s.opts.MaxDelay)
	if !timer.Stop() {
		<-timer.C
	}
	var timerC <-chan time.Time
	startTimer := func() {
		timer.Reset(s.opts.MaxDelay)
		timerC = timer.C
	}
	stopTimer := func() {
		if timerC == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timerC = nil
	}

	for {
		select {
		case item, ok := <-s.queue:
			if !ok {
				stopTimer()
				s.commit(batch)
				return
			}
			s.setQueueDepth()
			batch = append(batch, item)
			if len(batch) == 1 {
				startTimer()
			}
			if len(batch) >= s.opts.MaxRecords {
				stopTimer()
				s.commit(batch)
				batch = batch[:0]
			}
		case <-timerC:
			timerC = nil
			s.commit(batch)
			batch = batch[:0]
		}
	}
}

func (s *Service) commit(items []Accepted) {
	if len(items) == 0 {
		return
	}
	if s.engine == nil || s.store == nil {
		s.setLastError(trusterr.New(trusterr.CodeFailedPrecondition, "batch engine and store are required"))
		return
	}
	start := time.Now().UTC()
	batchID := s.nextBatchID(start)
	if err := s.persistBatch(context.Background(), batchID, start, items); err != nil {
		s.setLastError(err)
		return
	}
	if s.metrics != nil {
		s.metrics.BatchSizeRecords.Observe(float64(len(items)))
		s.metrics.BatchCommitLatency.Observe(time.Since(start).Seconds())
	}
	s.setLastError(nil)
}

// persistBatch writes a batch using the prepared -> bundles/root -> committed
// manifest protocol so that a crash between steps is recoverable from the WAL
// by rebuilding the deterministic outputs and replaying the remaining writes.
func (s *Service) persistBatch(ctx context.Context, batchID string, closedAt time.Time, items []Accepted) error {
	signed := make([]model.SignedClaim, len(items))
	records := make([]model.ServerRecord, len(items))
	accepted := make([]model.AcceptedReceipt, len(items))
	recordIDs := make([]string, len(items))
	for i := range items {
		signed[i] = items[i].Signed
		records[i] = items[i].Record
		accepted[i] = items[i].Accepted
		recordIDs[i] = items[i].Record.RecordID
	}

	bundles, err := s.engine.CommitBatch(batchID, closedAt, signed, records, accepted)
	if err != nil {
		return trusterr.Wrap(trusterr.CodeInternal, "commit batch", err)
	}
	if len(bundles) != len(items) {
		return trusterr.New(trusterr.CodeInternal, "commit batch returned inconsistent proof count")
	}

	manifest := model.BatchManifest{
		SchemaVersion:   model.SchemaBatchManifest,
		BatchID:         batchID,
		State:           model.BatchStatePrepared,
		TreeAlg:         model.DefaultMerkleTreeAlg,
		TreeSize:        uint64(len(bundles)),
		BatchRoot:       bundles[0].CommittedReceipt.BatchRoot,
		RecordIDs:       recordIDs,
		WALRange:        walRangeFor(items),
		ClosedAtUnixN:   closedAt.UnixNano(),
		PreparedAtUnixN: closedAt.UnixNano(),
	}
	if err := s.store.PutManifest(ctx, manifest); err != nil {
		return err
	}
	root, err := s.writeBundlesAndRoot(ctx, batchID, bundles)
	if err != nil {
		return err
	}
	manifest.State = model.BatchStateCommitted
	manifest.CommittedAtUnixN = time.Now().UTC().UnixNano()
	if err := s.store.PutManifest(ctx, manifest); err != nil {
		return err
	}
	// Advance the WAL checkpoint as a best-effort optimization for the next
	// restart. A failed write here never breaks correctness: replay can
	// always fall back to scanning the whole WAL and consulting manifests.
	if err := s.advanceCheckpoint(ctx, manifest); err != nil {
		s.setLastError(err)
	}
	s.fireOnBatchCommitted(ctx, root)
	return nil
}

func (s *Service) writeBundlesAndRoot(ctx context.Context, batchID string, bundles []model.ProofBundle) (model.BatchRoot, error) {
	for i := range bundles {
		if err := s.store.PutBundle(ctx, bundles[i]); err != nil {
			return model.BatchRoot{}, err
		}
	}
	root := model.BatchRoot{
		SchemaVersion: model.SchemaBatchRoot,
		BatchID:       batchID,
		BatchRoot:     bundles[0].CommittedReceipt.BatchRoot,
		TreeSize:      uint64(len(bundles)),
		ClosedAtUnixN: bundles[0].CommittedReceipt.ClosedAtUnixN,
	}
	if err := s.store.PutRoot(ctx, root); err != nil {
		return model.BatchRoot{}, err
	}
	return root, nil
}

// RecoverManifest replays a prepared manifest by rebuilding its bundles from
// the supplied items (looked up from the WAL by caller) and then writing
// bundles, root, and a committed manifest. Re-running this on an already
// committed manifest is a no-op for callers, and crash-resuming after partial
// writes converges to the same final state because every step uses the same
// deterministic inputs.
func (s *Service) RecoverManifest(ctx context.Context, manifest model.BatchManifest, items []Accepted) error {
	if manifest.State == model.BatchStateCommitted {
		return nil
	}
	if manifest.State != model.BatchStatePrepared {
		return trusterr.New(trusterr.CodeFailedPrecondition, fmt.Sprintf("unknown batch manifest state: %s", manifest.State))
	}
	if len(items) != len(manifest.RecordIDs) {
		return trusterr.New(trusterr.CodeFailedPrecondition, fmt.Sprintf("recovered items (%d) do not match manifest record count (%d)", len(items), len(manifest.RecordIDs)))
	}
	for i, rid := range manifest.RecordIDs {
		if items[i].Record.RecordID != rid {
			return trusterr.New(trusterr.CodeFailedPrecondition, fmt.Sprintf("recovered item %d record_id mismatch: got %s, want %s", i, items[i].Record.RecordID, rid))
		}
	}

	signed := make([]model.SignedClaim, len(items))
	records := make([]model.ServerRecord, len(items))
	accepted := make([]model.AcceptedReceipt, len(items))
	for i := range items {
		signed[i] = items[i].Signed
		records[i] = items[i].Record
		accepted[i] = items[i].Accepted
	}
	closedAt := time.Unix(0, manifest.ClosedAtUnixN).UTC()
	bundles, err := s.engine.CommitBatch(manifest.BatchID, closedAt, signed, records, accepted)
	if err != nil {
		return trusterr.Wrap(trusterr.CodeInternal, "rebuild batch during recovery", err)
	}
	if len(bundles) != len(manifest.RecordIDs) {
		return trusterr.New(trusterr.CodeInternal, "recovered bundle count mismatch")
	}
	root, err := s.writeBundlesAndRoot(ctx, manifest.BatchID, bundles)
	if err != nil {
		return err
	}
	manifest.State = model.BatchStateCommitted
	if manifest.CommittedAtUnixN == 0 {
		manifest.CommittedAtUnixN = time.Now().UTC().UnixNano()
	}
	if err := s.store.PutManifest(ctx, manifest); err != nil {
		return err
	}
	if err := s.advanceCheckpoint(ctx, manifest); err != nil {
		s.setLastError(err)
	}
	s.fireOnBatchCommitted(ctx, root)
	return nil
}

// fireOnBatchCommitted runs the commit hook in a panic-safe wrapper so a buggy
// observer cannot crash the batch worker. It is intentionally synchronous only
// for bounded local side effects such as durable outbox enqueue; slow global
// append, external notary calls, or network IO belong in a separate worker.
func (s *Service) fireOnBatchCommitted(ctx context.Context, root model.BatchRoot) {
	if s.opts.OnBatchCommitted == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			s.setLastError(trusterr.New(trusterr.CodeInternal,
				fmt.Sprintf("OnBatchCommitted panic: %v", r)))
		}
	}()
	s.opts.OnBatchCommitted(ctx, root)
}

// advanceCheckpoint moves the WAL checkpoint forward to cover every record
// inside manifest. The checkpoint is always advanced monotonically, so a
// stale read (concurrent commits, retries, recovery passes) never regresses
// it. Persisting the checkpoint is a best-effort optimization and a failure
// is surfaced as LastError so operators can investigate without rolling back
// the commit.
func (s *Service) advanceCheckpoint(ctx context.Context, manifest model.BatchManifest) error {
	to := manifest.WALRange.To
	if to.Sequence == 0 {
		return nil
	}
	existing, found, err := s.store.GetCheckpoint(ctx)
	if err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "load wal checkpoint", err)
	}
	if found && existing.LastSequence >= to.Sequence {
		return nil
	}
	cp := model.WALCheckpoint{
		SchemaVersion:   model.SchemaWALCheckpoint,
		SegmentID:       to.SegmentID,
		LastSequence:    to.Sequence,
		LastOffset:      to.Offset,
		BatchID:         manifest.BatchID,
		RecordedAtUnixN: time.Now().UTC().UnixNano(),
	}
	if err := s.store.PutCheckpoint(ctx, cp); err != nil {
		return err
	}
	if s.metrics != nil {
		s.metrics.WALCheckpointLastSequence.Set(float64(cp.LastSequence))
	}
	if s.opts.OnCheckpointAdvanced != nil {
		// Hook runs synchronously on the batch worker; see Options doc
		// for the tradeoff. Panics are recovered so a buggy prune hook
		// cannot take down the batcher.
		defer func() {
			if r := recover(); r != nil {
				s.setLastError(trusterr.New(trusterr.CodeInternal,
					fmt.Sprintf("OnCheckpointAdvanced panic: %v", r)))
			}
		}()
		s.opts.OnCheckpointAdvanced(ctx, cp)
	}
	return nil
}

func walRangeFor(items []Accepted) model.WALRange {
	if len(items) == 0 {
		return model.WALRange{}
	}
	from := items[0].Record.WAL
	to := items[0].Record.WAL
	for i := 1; i < len(items); i++ {
		pos := items[i].Record.WAL
		if pos.Sequence < from.Sequence {
			from = pos
		}
		if pos.Sequence > to.Sequence {
			to = pos
		}
	}
	return model.WALRange{From: from, To: to}
}

func (s *Service) nextBatchID(now time.Time) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	return fmt.Sprintf("batch-%d-%06d", now.UTC().UnixNano(), s.seq)
}

func (s *Service) setLastError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastErr = err
}

func (s *Service) setQueueDepth() {
	if s.metrics != nil {
		s.metrics.QueueDepth.WithLabelValues("batch").Set(float64(len(s.queue)))
	}
}
