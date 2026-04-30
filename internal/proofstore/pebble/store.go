// Package pebble provides a Pebble-backed implementation of
// proofstore.Store. Values are CBOR-encoded exactly like the file-based
// LocalStore, so the two backends round-trip identical bytes and can be
// migrated between by copying raw values. The key schema mirrors the
// on-disk layout documented in docs/TRUSTDB_DESIGN.md §17.2.
package pebble

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	pdb "github.com/cockroachdb/pebble"
	"github.com/golang/snappy"

	"github.com/ryan-wong-coder/trustdb/internal/cborx"
	"github.com/ryan-wong-coder/trustdb/internal/model"
	"github.com/ryan-wong-coder/trustdb/internal/trusterr"
)

// maxStoredObjectBytes caps decode input size to guard against corrupt
// values that claim to be multi-gigabyte CBOR payloads. Mirrors the same
// constant in the file backend.
const maxStoredObjectBytes = 64 << 20
const (
	batchArtifactChunkSize       = 1024
	bundleCompressionMinBytes    = 1024
	maxBatchArtifactEncodeWorker = 16
)

var errStopScan = errors.New("stop scan")

const (
	prefixBundle         = "bundle/"
	prefixBundleV2       = "bundle-v2/"
	prefixRecordByID     = "record/by-id/"
	prefixRecordByTime   = "record/by-time/"
	prefixRecordByBatch  = "record/by-batch/"
	prefixRecordByLevel  = "record/by-proof-level/"
	prefixRecordByTenant = "record/by-tenant/"
	prefixRecordByClient = "record/by-client/"
	prefixRecordByHash   = "record/by-content/"
	prefixRecordByToken  = "record/by-storage-token/"
	prefixManifest       = "manifest/"
	prefixRoot           = "root/"
	prefixGlobalLeaf     = "global/leaf/"
	prefixGlobalBatch    = "global/leaf-by-batch/"
	prefixGlobalNode     = "global/node/"
	prefixSTH            = "global/sth/"
	prefixGlobalTile     = "global/tile/"
	prefixGlobalOutbox   = "global/outbox/"
	prefixGlobalStatus   = "global/outbox-status/"
	prefixAnchorOutbox   = "anchor/sth-outbox/"
	prefixAnchorStatus   = "anchor/sth-status/"
	prefixAnchorResult   = "anchor/sth-result/"
	checkpointKey        = "checkpoint/wal"
	globalStateKey       = "global/state/latest"
	rootSortKeyWidth     = 20
)

const (
	schemaStoredProofBundleV2 = "trustdb.pebble-proof-bundle.v2"
	storedBundleCodecSnappy   = "snappy"
)

var recordIndexRefPrefix = []byte("trustdb.record-index-ref.v1\x00")

type storedProofBundleEnvelope struct {
	SchemaVersion string `cbor:"schema_version" json:"schema_version"`
	Codec         string `cbor:"codec" json:"codec"`
	Data          []byte `cbor:"data" json:"data"`
}

type encodedBatchArtifact struct {
	recordID    string
	bundleValue []byte
	index       encodedRecordIndex
}

type encodedRecordIndex struct {
	idx           model.RecordIndex
	value         []byte
	primaryKey    []byte
	secondaryKeys [][]byte
	refValue      []byte
}

// Store is a Pebble-backed proof store. It is safe for concurrent use
// from multiple goroutines; Pebble's internal locking guarantees that
// all Store methods see a linearizable view of the underlying key space.
type Store struct {
	db *pdb.DB

	// closeOnce guards the underlying db.Close so that duplicate
	// Close calls from defers and shutdown hooks cannot panic on a
	// double-free inside Pebble.
	closeOnce sync.Once
	closeErr  error
}

// Open creates or opens a Pebble database at path and wraps it in a
// Store. The caller owns the returned *Store and must call Close to
// release the underlying file locks; Pebble refuses a second Open at
// the same path while the first handle is still live.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, trusterr.New(trusterr.CodeInvalidArgument, "pebble proofstore path is required")
	}
	db, err := pdb.Open(path, &pdb.Options{})
	if err != nil {
		return nil, trusterr.Wrap(trusterr.CodeInternal, "open pebble proofstore", err)
	}
	return &Store{db: db}, nil
}

// Close releases the underlying Pebble database. It is safe to call
// multiple times and from multiple goroutines; subsequent calls return
// the result of the first close.
func (s *Store) Close() error {
	s.closeOnce.Do(func() {
		s.closeErr = s.db.Close()
	})
	return s.closeErr
}

// PebbleMetrics returns a point-in-time snapshot of the underlying
// Pebble engine metrics. The snapshot is cheap to read and safe for
// concurrent use by observability collectors.
func (s *Store) PebbleMetrics() *pdb.Metrics {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Metrics()
}

// bundleKey returns the Pebble key used to store a proof bundle. The
// record_id is written raw because Pebble, unlike the filesystem, has
// no filename escaping constraints.
func bundleKey(recordID string) []byte {
	return append([]byte(prefixBundle), recordID...)
}

func bundleV2Key(recordID string) []byte {
	return append([]byte(prefixBundleV2), recordID...)
}

func recordByIDKey(recordID string) []byte {
	return append([]byte(prefixRecordByID), recordID...)
}

func recordSecondaryPart(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func recordIndexSuffix(receivedAtUnixN int64, recordID string) string {
	return fmt.Sprintf("%0*d/%s", rootSortKeyWidth, receivedAtUnixN, recordID)
}

func recordIndexKey(prefix string, receivedAtUnixN int64, recordID string) []byte {
	return []byte(prefix + recordIndexSuffix(receivedAtUnixN, recordID))
}

func recordIndexUpperTimeKey(prefix string, receivedAtUnixN int64) []byte {
	return []byte(fmt.Sprintf("%s%0*d0", prefix, rootSortKeyWidth, receivedAtUnixN))
}

func recordIndexPrefixes(idx model.RecordIndex) []string {
	prefixes := []string{prefixRecordByTime}
	if idx.BatchID != "" {
		prefixes = append(prefixes, prefixRecordByBatch+recordSecondaryPart(idx.BatchID)+"/")
	}
	if idx.ProofLevel != "" {
		prefixes = append(prefixes, prefixRecordByLevel+recordSecondaryPart(idx.ProofLevel)+"/")
	}
	if idx.TenantID != "" {
		prefixes = append(prefixes, prefixRecordByTenant+recordSecondaryPart(idx.TenantID)+"/")
	}
	if idx.ClientID != "" {
		prefixes = append(prefixes, prefixRecordByClient+recordSecondaryPart(idx.ClientID)+"/")
	}
	if len(idx.ContentHash) > 0 {
		prefixes = append(prefixes, prefixRecordByHash+hex.EncodeToString(idx.ContentHash)+"/")
	}
	for _, token := range model.RecordIndexStorageTokens(idx) {
		prefixes = append(prefixes, prefixRecordByToken+recordSecondaryPart(token)+"/")
	}
	return prefixes
}

func recordIndexKeys(idx model.RecordIndex) [][]byte {
	if idx.RecordID == "" {
		return nil
	}
	keys := [][]byte{recordByIDKey(idx.RecordID)}
	for _, prefix := range recordIndexPrefixes(idx) {
		keys = append(keys, recordIndexKey(prefix, idx.ReceivedAtUnixN, idx.RecordID))
	}
	return keys
}

func manifestKey(batchID string) []byte {
	return append([]byte(prefixManifest), batchID...)
}

// rootKey preserves the same %020d sort-order trick used by the file
// backend's filenames: zero-padding the nanosecond timestamp guarantees
// that lexical byte-order matches time-order so an iterator can read
// roots newest-first with SeekLT + Prev.
func rootKey(closedAtUnixN int64, batchID string) []byte {
	k := make([]byte, 0, len(prefixRoot)+rootSortKeyWidth+1+len(batchID))
	k = append(k, prefixRoot...)
	k = fmt.Appendf(k, "%0*d", rootSortKeyWidth, closedAtUnixN)
	k = append(k, '/')
	k = append(k, batchID...)
	return k
}

func isNotFound(err error) bool {
	return errors.Is(err, pdb.ErrNotFound)
}

// writeCBOR marshals v and writes it at key with Sync durability so the
// write is readable after an immediate crash. The sync flush mirrors
// the writeCBORAtomic + rename guarantee of the file backend.
func (s *Store) writeCBOR(key []byte, v any) error {
	data, err := cborx.Marshal(v)
	if err != nil {
		return err
	}
	if err := s.db.Set(key, data, pdb.Sync); err != nil {
		return err
	}
	return nil
}

// readCBOR fetches key and decodes it into v. Pebble's Get returns
// borrowed bytes that must be copied before the closer runs; the
// cbor decoder copies into v so we can release the closer immediately
// after the decode.
func (s *Store) readCBOR(key []byte, v any) (bool, error) {
	val, closer, err := s.db.Get(key)
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	defer closer.Close()
	if err := cborx.UnmarshalLimit(val, v, maxStoredObjectBytes); err != nil {
		return false, err
	}
	return true, nil
}

func encodeStoredProofBundle(bundle model.ProofBundle) ([]byte, error) {
	if bundle.RecordID == "" {
		return nil, trusterr.New(trusterr.CodeInvalidArgument, "proof bundle record_id is required")
	}
	raw, err := cborx.Marshal(bundle)
	if err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "encode proof bundle", err)
	}
	if len(raw) < bundleCompressionMinBytes {
		return raw, nil
	}
	compressed := snappy.Encode(nil, raw)
	if len(compressed) >= len(raw) {
		return raw, nil
	}
	envelope, err := cborx.Marshal(storedProofBundleEnvelope{
		SchemaVersion: schemaStoredProofBundleV2,
		Codec:         storedBundleCodecSnappy,
		Data:          compressed,
	})
	if err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "encode proof bundle envelope", err)
	}
	return envelope, nil
}

func decodeStoredProofBundle(data []byte) (model.ProofBundle, error) {
	var envelope storedProofBundleEnvelope
	if err := cborx.UnmarshalLimit(data, &envelope, maxStoredObjectBytes); err == nil && envelope.SchemaVersion == schemaStoredProofBundleV2 {
		if envelope.Codec != storedBundleCodecSnappy {
			return model.ProofBundle{}, trusterr.New(trusterr.CodeDataLoss, "unsupported proof bundle codec")
		}
		decodedLen, err := snappy.DecodedLen(envelope.Data)
		if err != nil {
			return model.ProofBundle{}, trusterr.Wrap(trusterr.CodeDataLoss, "decode proof bundle envelope length", err)
		}
		if decodedLen > maxStoredObjectBytes {
			return model.ProofBundle{}, trusterr.New(trusterr.CodeDataLoss, "proof bundle envelope payload too large")
		}
		raw, err := snappy.Decode(nil, envelope.Data)
		if err != nil {
			return model.ProofBundle{}, trusterr.Wrap(trusterr.CodeDataLoss, "decompress proof bundle", err)
		}
		var bundle model.ProofBundle
		if err := cborx.UnmarshalLimit(raw, &bundle, maxStoredObjectBytes); err != nil {
			return model.ProofBundle{}, err
		}
		return bundle, nil
	}
	var bundle model.ProofBundle
	if err := cborx.UnmarshalLimit(data, &bundle, maxStoredObjectBytes); err != nil {
		return model.ProofBundle{}, err
	}
	return bundle, nil
}

func (s *Store) readStoredProofBundle(key []byte) (model.ProofBundle, bool, error) {
	val, closer, err := s.db.Get(key)
	if err != nil {
		if isNotFound(err) {
			return model.ProofBundle{}, false, nil
		}
		return model.ProofBundle{}, false, err
	}
	defer closer.Close()
	bundle, err := decodeStoredProofBundle(val)
	if err != nil {
		return model.ProofBundle{}, false, err
	}
	return bundle, true, nil
}

func encodeRecordIndexArtifact(idx model.RecordIndex) (encodedRecordIndex, error) {
	if idx.RecordID == "" {
		return encodedRecordIndex{}, trusterr.New(trusterr.CodeInvalidArgument, "record index record_id is required")
	}
	idx.ProofLevel = model.RecordIndexProofLevel(idx)
	if idx.SchemaVersion == "" {
		idx.SchemaVersion = model.SchemaRecordIndex
	}
	indexData, err := cborx.Marshal(idx)
	if err != nil {
		return encodedRecordIndex{}, trusterr.Wrap(trusterr.CodeDataLoss, "encode record index", err)
	}
	keys := recordIndexKeys(idx)
	return encodedRecordIndex{
		idx:           idx,
		value:         indexData,
		primaryKey:    keys[0],
		secondaryKeys: keys[1:],
		refValue:      recordIndexRefValue(idx.RecordID),
	}, nil
}

func encodeBatchArtifact(bundle model.ProofBundle) (encodedBatchArtifact, error) {
	bundleValue, err := encodeStoredProofBundle(bundle)
	if err != nil {
		return encodedBatchArtifact{}, err
	}
	index, err := encodeRecordIndexArtifact(model.RecordIndexFromBundle(bundle))
	if err != nil {
		return encodedBatchArtifact{}, err
	}
	return encodedBatchArtifact{recordID: bundle.RecordID, bundleValue: bundleValue, index: index}, nil
}

func encodeBatchArtifacts(ctx context.Context, bundles []model.ProofBundle) ([]encodedBatchArtifact, error) {
	artifacts := make([]encodedBatchArtifact, len(bundles))
	if len(bundles) == 0 {
		return artifacts, nil
	}
	workers := runtime.GOMAXPROCS(0)
	if workers > maxBatchArtifactEncodeWorker {
		workers = maxBatchArtifactEncodeWorker
	}
	if workers > len(bundles) {
		workers = len(bundles)
	}
	jobs := make(chan int)
	errs := make([]error, len(bundles))
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if err := ctx.Err(); err != nil {
					errs[i] = trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore encode batch artifacts canceled", err)
					continue
				}
				artifact, err := encodeBatchArtifact(bundles[i])
				if err != nil {
					errs[i] = err
					continue
				}
				artifacts[i] = artifact
			}
		}()
	}
	for i := range bundles {
		select {
		case jobs <- i:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore encode batch artifacts canceled", ctx.Err())
		}
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore encode batch artifacts canceled", err)
	}
	for i := range errs {
		if errs[i] != nil {
			return nil, errs[i]
		}
	}
	return artifacts, nil
}

func stageSet(batch *pdb.Batch, key, value []byte) error {
	op := batch.SetDeferred(len(key), len(value))
	copy(op.Key, key)
	copy(op.Value, value)
	return op.Finish()
}

func recordIndexRefValue(recordID string) []byte {
	value := make([]byte, 0, len(recordIndexRefPrefix)+len(recordID))
	value = append(value, recordIndexRefPrefix...)
	value = append(value, recordID...)
	return value
}

func decodeRecordIndexRef(value []byte) (string, bool) {
	if !bytes.HasPrefix(value, recordIndexRefPrefix) {
		return "", false
	}
	recordID := string(value[len(recordIndexRefPrefix):])
	return recordID, recordID != ""
}

func (s *Store) readRecordIndexScanValue(value []byte) (model.RecordIndex, error) {
	if recordID, ok := decodeRecordIndexRef(value); ok {
		var idx model.RecordIndex
		found, err := s.readCBOR(recordByIDKey(recordID), &idx)
		if err != nil {
			return model.RecordIndex{}, err
		}
		if !found {
			return model.RecordIndex{}, trusterr.New(trusterr.CodeDataLoss, "record index reference target not found")
		}
		return idx, nil
	}
	var idx model.RecordIndex
	if err := cborx.UnmarshalLimit(value, &idx, maxStoredObjectBytes); err != nil {
		return model.RecordIndex{}, err
	}
	return idx, nil
}

func (s *Store) PutBundle(ctx context.Context, bundle model.ProofBundle) error {
	if err := ctx.Err(); err != nil {
		return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore put bundle canceled", err)
	}
	if bundle.RecordID == "" {
		return trusterr.New(trusterr.CodeInvalidArgument, "proof bundle record_id is required")
	}
	artifact, err := encodeBatchArtifact(bundle)
	if err != nil {
		return err
	}
	var old model.RecordIndex
	oldFound, err := s.readCBOR(recordByIDKey(bundle.RecordID), &old)
	if err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "read existing record index", err)
	}
	batch := s.db.NewBatch()
	defer batch.Close()
	if err := stageSet(batch, bundleV2Key(bundle.RecordID), artifact.bundleValue); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "stage proof bundle", err)
	}
	if err := s.stageEncodedRecordIndexReplace(batch, artifact.index, old, oldFound); err != nil {
		return err
	}
	if err := batch.Commit(pdb.Sync); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "commit proof bundle", err)
	}
	return nil
}

func (s *Store) PutBatchArtifacts(ctx context.Context, bundles []model.ProofBundle, root model.BatchRoot) error {
	if err := ctx.Err(); err != nil {
		return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore put batch artifacts canceled", err)
	}
	if len(bundles) == 0 {
		return trusterr.New(trusterr.CodeInvalidArgument, "proofstore batch artifacts require at least one bundle")
	}
	root, err := normalizeBatchRoot(root, len(bundles))
	if err != nil {
		return err
	}
	artifacts, err := encodeBatchArtifacts(ctx, bundles)
	if err != nil {
		return err
	}
	for start := 0; start < len(artifacts); start += batchArtifactChunkSize {
		end := start + batchArtifactChunkSize
		if end > len(artifacts) {
			end = len(artifacts)
		}
		if err := ctx.Err(); err != nil {
			return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore put batch artifacts canceled", err)
		}
		batch := s.db.NewBatch()
		for i := start; i < end; i++ {
			if err := s.stageEncodedBatchArtifact(batch, artifacts[i]); err != nil {
				_ = batch.Close()
				return err
			}
		}
		if end == len(artifacts) {
			if err := s.stageRoot(batch, root); err != nil {
				_ = batch.Close()
				return err
			}
		}
		if err := batch.Commit(pdb.Sync); err != nil {
			_ = batch.Close()
			return trusterr.Wrap(trusterr.CodeDataLoss, "commit batch artifacts", err)
		}
		if err := batch.Close(); err != nil {
			return trusterr.Wrap(trusterr.CodeDataLoss, "close batch artifacts", err)
		}
	}
	return nil
}

func (s *Store) stageNewBundle(batch *pdb.Batch, bundle model.ProofBundle) error {
	artifact, err := encodeBatchArtifact(bundle)
	if err != nil {
		return err
	}
	return s.stageEncodedBatchArtifact(batch, artifact)
}

func (s *Store) stageEncodedBatchArtifact(batch *pdb.Batch, artifact encodedBatchArtifact) error {
	if err := stageSet(batch, bundleV2Key(artifact.recordID), artifact.bundleValue); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "stage proof bundle", err)
	}
	return s.stageEncodedRecordIndexSet(batch, artifact.index)
}

func (s *Store) PutRecordIndex(ctx context.Context, idx model.RecordIndex) error {
	if err := ctx.Err(); err != nil {
		return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore put record index canceled", err)
	}
	if idx.RecordID == "" {
		return trusterr.New(trusterr.CodeInvalidArgument, "record index record_id is required")
	}
	var old model.RecordIndex
	oldFound, err := s.readCBOR(recordByIDKey(idx.RecordID), &old)
	if err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "read existing record index", err)
	}
	batch := s.db.NewBatch()
	defer batch.Close()
	if err := s.stageRecordIndexReplace(batch, idx, old, oldFound); err != nil {
		return err
	}
	if err := batch.Commit(pdb.Sync); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "commit record index", err)
	}
	return nil
}

func (s *Store) GetBundle(ctx context.Context, recordID string) (model.ProofBundle, error) {
	if err := ctx.Err(); err != nil {
		return model.ProofBundle{}, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore get bundle canceled", err)
	}
	if recordID == "" {
		return model.ProofBundle{}, trusterr.New(trusterr.CodeInvalidArgument, "record_id is required")
	}
	var bundle model.ProofBundle
	bundle, found, err := s.readStoredProofBundle(bundleV2Key(recordID))
	if err != nil {
		return model.ProofBundle{}, trusterr.Wrap(trusterr.CodeDataLoss, "read proof bundle", err)
	}
	if found {
		return bundle, nil
	}
	found, err = s.readCBOR(bundleKey(recordID), &bundle)
	if err != nil {
		return model.ProofBundle{}, trusterr.Wrap(trusterr.CodeDataLoss, "read legacy proof bundle", err)
	}
	if found {
		return bundle, nil
	}
	return model.ProofBundle{}, trusterr.New(trusterr.CodeNotFound, "proof bundle not found")
}

func (s *Store) GetRecordIndex(ctx context.Context, recordID string) (model.RecordIndex, bool, error) {
	if err := ctx.Err(); err != nil {
		return model.RecordIndex{}, false, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore get record index canceled", err)
	}
	if recordID == "" {
		return model.RecordIndex{}, false, trusterr.New(trusterr.CodeInvalidArgument, "record_id is required")
	}
	var idx model.RecordIndex
	found, err := s.readCBOR(recordByIDKey(recordID), &idx)
	if err != nil {
		return model.RecordIndex{}, false, trusterr.Wrap(trusterr.CodeDataLoss, "read record index", err)
	}
	return idx, found, nil
}

func (s *Store) ListRecordIndexes(ctx context.Context, opts model.RecordListOptions) ([]model.RecordIndex, error) {
	if err := ctx.Err(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list record indexes canceled", err)
	}
	limit := normaliseRecordLimit(opts.Limit)
	prefix := recordListPrefix(opts)
	lower, upper := prefixBounds(prefix)
	iter, err := s.db.NewIter(&pdb.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "open record index iterator", err)
	}
	defer iter.Close()

	desc := !strings.EqualFold(opts.Direction, model.RecordListDirectionAsc)
	hasCursor := opts.AfterReceivedAtUnixN != 0 || opts.AfterRecordID != ""
	var ok bool
	if desc {
		if hasCursor {
			ok = iter.SeekLT(recordIndexKey(prefix, opts.AfterReceivedAtUnixN, opts.AfterRecordID))
		} else if opts.ReceivedToUnixN > 0 {
			ok = iter.SeekLT(recordIndexUpperTimeKey(prefix, opts.ReceivedToUnixN))
		} else {
			ok = iter.Last()
		}
	} else if hasCursor {
		ok = iter.SeekGE(recordIndexKey(prefix, opts.AfterReceivedAtUnixN, opts.AfterRecordID))
	} else if opts.ReceivedFromUnixN > 0 {
		ok = iter.SeekGE(recordIndexKey(prefix, opts.ReceivedFromUnixN, ""))
	} else {
		ok = iter.First()
	}

	records := make([]model.RecordIndex, 0, limit)
	for ok {
		if err := ctx.Err(); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list record indexes canceled", err)
		}
		if len(records) >= limit {
			break
		}
		idx, err := s.readRecordIndexScanValue(iter.Value())
		if err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDataLoss, "decode record index", err)
		}
		if recordRangeExhausted(idx, opts, desc) {
			break
		}
		if model.RecordIndexMatchesListOptions(idx, opts) && model.RecordIndexAfterCursor(idx, opts) {
			records = append(records, idx)
		}
		if desc {
			ok = iter.Prev()
		} else {
			ok = iter.Next()
		}
	}
	if err := iter.Error(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "iterate record indexes", err)
	}
	return records, nil
}

func (s *Store) PutRoot(ctx context.Context, root model.BatchRoot) error {
	if err := ctx.Err(); err != nil {
		return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore put root canceled", err)
	}
	root, err := normalizeBatchRoot(root, 0)
	if err != nil {
		return err
	}
	if err := s.writeCBOR(rootKey(root.ClosedAtUnixN, root.BatchID), root); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "write batch root", err)
	}
	return nil
}

func normalizeBatchRoot(root model.BatchRoot, expectedTreeSize int) (model.BatchRoot, error) {
	if root.BatchID == "" {
		return model.BatchRoot{}, trusterr.New(trusterr.CodeInvalidArgument, "batch root batch_id is required")
	}
	if root.SchemaVersion == "" {
		root.SchemaVersion = model.SchemaBatchRoot
	}
	if expectedTreeSize > 0 {
		if root.TreeSize == 0 {
			root.TreeSize = uint64(expectedTreeSize)
		}
		if root.TreeSize != uint64(expectedTreeSize) {
			return model.BatchRoot{}, trusterr.New(trusterr.CodeInvalidArgument, "batch root tree_size does not match bundle count")
		}
	}
	if root.ClosedAtUnixN == 0 {
		root.ClosedAtUnixN = time.Now().UTC().UnixNano()
	}
	return root, nil
}

func (s *Store) stageRoot(batch *pdb.Batch, root model.BatchRoot) error {
	rootData, err := cborx.Marshal(root)
	if err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "encode batch root", err)
	}
	if err := stageSet(batch, rootKey(root.ClosedAtUnixN, root.BatchID), rootData); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "stage batch root", err)
	}
	return nil
}

// rootBounds returns the half-open iterator bounds covering every root
// key. UpperBound uses the next byte after '/' so it captures every
// timestamp suffix without colliding with other prefixes.
func rootBounds() (lower, upper []byte) {
	lower = []byte(prefixRoot)
	// '0' is the byte immediately after '/', so "root0" is the exclusive
	// upper bound for every key that starts with "root/".
	upper = []byte("root0")
	return lower, upper
}

func (s *Store) ListRoots(ctx context.Context, limit int) ([]model.BatchRoot, error) {
	if err := ctx.Err(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list roots canceled", err)
	}
	if limit <= 0 {
		limit = 100
	}
	lower, upper := rootBounds()
	iter, err := s.db.NewIter(&pdb.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "open root iterator", err)
	}
	defer iter.Close()

	capHint := limit
	if capHint > 1024 {
		capHint = 1024
	}
	roots := make([]model.BatchRoot, 0, capHint)
	// Reverse iteration gives newest-first ordering because our root
	// keys are zero-padded nanosecond timestamps.
	for ok := iter.Last(); ok; ok = iter.Prev() {
		if err := ctx.Err(); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list roots canceled", err)
		}
		if len(roots) >= limit {
			break
		}
		var root model.BatchRoot
		if err := cborx.UnmarshalLimit(iter.Value(), &root, maxStoredObjectBytes); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDataLoss, "decode batch root", err)
		}
		roots = append(roots, root)
	}
	if err := iter.Error(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "iterate roots", err)
	}
	return roots, nil
}

func (s *Store) ListRootsAfter(ctx context.Context, afterClosedAtUnixN int64, limit int) ([]model.BatchRoot, error) {
	if err := ctx.Err(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list roots after canceled", err)
	}
	if limit <= 0 {
		limit = 100
	}
	lower, upper := rootBounds()
	iter, err := s.db.NewIter(&pdb.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "open root iterator", err)
	}
	defer iter.Close()
	startKey := rootKey(afterClosedAtUnixN+1, "")
	ok := iter.SeekGE(startKey)
	roots := make([]model.BatchRoot, 0, limit)
	for ; ok; ok = iter.Next() {
		if err := ctx.Err(); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list roots after canceled", err)
		}
		if len(roots) >= limit {
			break
		}
		var root model.BatchRoot
		if err := cborx.UnmarshalLimit(iter.Value(), &root, maxStoredObjectBytes); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDataLoss, "decode batch root", err)
		}
		if root.ClosedAtUnixN <= afterClosedAtUnixN {
			continue
		}
		roots = append(roots, root)
	}
	if err := iter.Error(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "iterate roots after", err)
	}
	return roots, nil
}

func (s *Store) ListRootsPage(ctx context.Context, opts model.RootListOptions) ([]model.BatchRoot, error) {
	if err := ctx.Err(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list roots page canceled", err)
	}
	limit := normaliseRecordLimit(opts.Limit)
	lower, upper := rootBounds()
	iter, err := s.db.NewIter(&pdb.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "open root iterator", err)
	}
	defer iter.Close()

	desc := !strings.EqualFold(opts.Direction, model.RecordListDirectionAsc)
	hasCursor := opts.AfterClosedAtUnixN != 0 || opts.AfterBatchID != ""
	var ok bool
	if desc {
		if hasCursor {
			ok = iter.SeekLT(rootKey(opts.AfterClosedAtUnixN, opts.AfterBatchID))
		} else {
			ok = iter.Last()
		}
	} else if hasCursor {
		ok = iter.SeekGE(rootKey(opts.AfterClosedAtUnixN, opts.AfterBatchID))
	} else {
		ok = iter.First()
	}

	roots := make([]model.BatchRoot, 0, limit)
	for ok {
		if err := ctx.Err(); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list roots page canceled", err)
		}
		if len(roots) >= limit {
			break
		}
		var root model.BatchRoot
		if err := cborx.UnmarshalLimit(iter.Value(), &root, maxStoredObjectBytes); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDataLoss, "decode batch root", err)
		}
		if model.BatchRootAfterCursor(root, opts) {
			roots = append(roots, root)
		}
		if desc {
			ok = iter.Prev()
		} else {
			ok = iter.Next()
		}
	}
	if err := iter.Error(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "iterate roots page", err)
	}
	return roots, nil
}

func (s *Store) LatestRoot(ctx context.Context) (model.BatchRoot, error) {
	roots, err := s.ListRoots(ctx, 1)
	if err != nil {
		return model.BatchRoot{}, err
	}
	if len(roots) == 0 {
		return model.BatchRoot{}, trusterr.New(trusterr.CodeNotFound, "batch root not found")
	}
	return roots[0], nil
}

func (s *Store) PutManifest(ctx context.Context, manifest model.BatchManifest) error {
	if err := ctx.Err(); err != nil {
		return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore put manifest canceled", err)
	}
	if manifest.BatchID == "" {
		return trusterr.New(trusterr.CodeInvalidArgument, "batch manifest batch_id is required")
	}
	if manifest.State != model.BatchStatePrepared && manifest.State != model.BatchStateCommitted {
		return trusterr.New(trusterr.CodeInvalidArgument, "batch manifest state must be prepared or committed")
	}
	if manifest.SchemaVersion == "" {
		manifest.SchemaVersion = model.SchemaBatchManifest
	}
	if err := s.writeCBOR(manifestKey(manifest.BatchID), manifest); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "write batch manifest", err)
	}
	return nil
}

func (s *Store) GetManifest(ctx context.Context, batchID string) (model.BatchManifest, error) {
	if err := ctx.Err(); err != nil {
		return model.BatchManifest{}, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore get manifest canceled", err)
	}
	if batchID == "" {
		return model.BatchManifest{}, trusterr.New(trusterr.CodeInvalidArgument, "batch_id is required")
	}
	var manifest model.BatchManifest
	found, err := s.readCBOR(manifestKey(batchID), &manifest)
	if err != nil {
		return model.BatchManifest{}, trusterr.Wrap(trusterr.CodeDataLoss, "read batch manifest", err)
	}
	if !found {
		return model.BatchManifest{}, trusterr.New(trusterr.CodeNotFound, "batch manifest not found")
	}
	return manifest, nil
}

func manifestBounds() (lower, upper []byte) {
	lower = []byte(prefixManifest)
	// "manifest/" → upper = "manifest0", same "next byte after /" trick.
	upper = []byte("manifest0")
	return lower, upper
}

func (s *Store) ListManifests(ctx context.Context) ([]model.BatchManifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list manifests canceled", err)
	}
	lower, upper := manifestBounds()
	iter, err := s.db.NewIter(&pdb.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "open manifest iterator", err)
	}
	defer iter.Close()

	var manifests []model.BatchManifest
	for ok := iter.First(); ok; ok = iter.Next() {
		if err := ctx.Err(); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list manifests canceled", err)
		}
		var manifest model.BatchManifest
		if err := cborx.UnmarshalLimit(iter.Value(), &manifest, maxStoredObjectBytes); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDataLoss, "decode batch manifest", err)
		}
		manifests = append(manifests, manifest)
	}
	if err := iter.Error(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "iterate manifests", err)
	}
	return manifests, nil
}

func (s *Store) ListManifestsAfter(ctx context.Context, afterBatchID string, limit int) ([]model.BatchManifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list manifests after canceled", err)
	}
	if limit <= 0 {
		limit = 100
	}
	lower, upper := manifestBounds()
	iter, err := s.db.NewIter(&pdb.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "open manifest iterator", err)
	}
	defer iter.Close()

	ok := iter.SeekGE(manifestKey(afterBatchID))
	manifests := make([]model.BatchManifest, 0, limit)
	for ; ok; ok = iter.Next() {
		if err := ctx.Err(); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list manifests after canceled", err)
		}
		if len(manifests) >= limit {
			break
		}
		var manifest model.BatchManifest
		if err := cborx.UnmarshalLimit(iter.Value(), &manifest, maxStoredObjectBytes); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDataLoss, "decode batch manifest", err)
		}
		if afterBatchID != "" && manifest.BatchID <= afterBatchID {
			continue
		}
		manifests = append(manifests, manifest)
	}
	if err := iter.Error(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "iterate manifests after", err)
	}
	return manifests, nil
}

func (s *Store) PutCheckpoint(ctx context.Context, cp model.WALCheckpoint) error {
	if err := ctx.Err(); err != nil {
		return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore put checkpoint canceled", err)
	}
	if cp.SchemaVersion == "" {
		cp.SchemaVersion = model.SchemaWALCheckpoint
	}
	if cp.RecordedAtUnixN == 0 {
		cp.RecordedAtUnixN = time.Now().UTC().UnixNano()
	}
	if err := s.writeCBOR([]byte(checkpointKey), cp); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "write wal checkpoint", err)
	}
	return nil
}

func (s *Store) GetCheckpoint(ctx context.Context) (model.WALCheckpoint, bool, error) {
	if err := ctx.Err(); err != nil {
		return model.WALCheckpoint{}, false, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore get checkpoint canceled", err)
	}
	var cp model.WALCheckpoint
	found, err := s.readCBOR([]byte(checkpointKey), &cp)
	if err != nil {
		return model.WALCheckpoint{}, false, trusterr.Wrap(trusterr.CodeDataLoss, "read wal checkpoint", err)
	}
	if !found {
		return model.WALCheckpoint{}, false, nil
	}
	return cp, true, nil
}

func globalLeafKey(index uint64) []byte {
	return []byte(fmt.Sprintf("%s%0*d", prefixGlobalLeaf, rootSortKeyWidth, index))
}

func globalBatchKey(batchID string) []byte {
	return append([]byte(prefixGlobalBatch), batchID...)
}

func globalNodeKey(level, startIndex uint64) []byte {
	return []byte(fmt.Sprintf("%s%0*d/%0*d", prefixGlobalNode, rootSortKeyWidth, level, rootSortKeyWidth, startIndex))
}

func sthKey(treeSize uint64) []byte {
	return []byte(fmt.Sprintf("%s%0*d", prefixSTH, rootSortKeyWidth, treeSize))
}

func globalTileKey(tile model.GlobalLogTile) []byte {
	return []byte(fmt.Sprintf(
		"%s%0*d/%0*d/%0*d",
		prefixGlobalTile,
		rootSortKeyWidth,
		tile.Level,
		rootSortKeyWidth,
		tile.StartIndex,
		rootSortKeyWidth,
		tile.Width,
	))
}

func globalOutboxKey(batchID string) []byte {
	return append([]byte(prefixGlobalOutbox), batchID...)
}

func globalStatusKey(status string, sortUnixN int64, batchID string) []byte {
	return []byte(fmt.Sprintf("%s%s/%0*d/%s", prefixGlobalStatus, status, rootSortKeyWidth, sortUnixN, batchID))
}

func globalStatusPrefix(status string) string {
	return prefixGlobalStatus + status + "/"
}

func globalStatusSortUnixN(item model.GlobalLogOutboxItem) int64 {
	switch item.Status {
	case model.AnchorStatePending:
		if item.NextAttemptUnixN > 0 {
			return item.NextAttemptUnixN
		}
		return item.EnqueuedAtUnixN
	case model.AnchorStatePublished:
		if item.CompletedAtUnixN > 0 {
			return item.CompletedAtUnixN
		}
		return item.LastAttemptUnixN
	default:
		return item.EnqueuedAtUnixN
	}
}

func anchorOutboxKey(treeSize uint64) []byte {
	return []byte(fmt.Sprintf("%s%0*d", prefixAnchorOutbox, rootSortKeyWidth, treeSize))
}

func anchorStatusKey(status string, sortUnixN int64, treeSize uint64) []byte {
	return []byte(fmt.Sprintf("%s%s/%0*d/%0*d", prefixAnchorStatus, status, rootSortKeyWidth, sortUnixN, rootSortKeyWidth, treeSize))
}

func anchorStatusPrefix(status string) string {
	return prefixAnchorStatus + status + "/"
}

func anchorStatusSortUnixN(item model.STHAnchorOutboxItem) int64 {
	switch item.Status {
	case model.AnchorStatePending:
		if item.NextAttemptUnixN > 0 {
			return item.NextAttemptUnixN
		}
		return item.EnqueuedAtUnixN
	case model.AnchorStatePublished:
		return item.EnqueuedAtUnixN
	case model.AnchorStateFailed:
		return item.EnqueuedAtUnixN
	default:
		return item.EnqueuedAtUnixN
	}
}

func anchorResultKey(treeSize uint64) []byte {
	return []byte(fmt.Sprintf("%s%0*d", prefixAnchorResult, rootSortKeyWidth, treeSize))
}

func (s *Store) PutGlobalLeaf(ctx context.Context, leaf model.GlobalLogLeaf) error {
	if err := ctx.Err(); err != nil {
		return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore put global leaf canceled", err)
	}
	if leaf.BatchID == "" {
		return trusterr.New(trusterr.CodeInvalidArgument, "global log leaf batch_id is required")
	}
	if leaf.SchemaVersion == "" {
		leaf.SchemaVersion = model.SchemaGlobalLogLeaf
	}
	if leaf.AppendedAtUnixN == 0 {
		leaf.AppendedAtUnixN = time.Now().UTC().UnixNano()
	}
	data, err := cborx.Marshal(leaf)
	if err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "encode global leaf", err)
	}
	batch := s.db.NewBatch()
	defer batch.Close()
	if err := batch.Set(globalLeafKey(leaf.LeafIndex), data, nil); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "stage global leaf", err)
	}
	if err := batch.Set(globalBatchKey(leaf.BatchID), data, nil); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "stage global leaf batch index", err)
	}
	if err := batch.Commit(pdb.Sync); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "commit global leaf", err)
	}
	return nil
}

func (s *Store) GetGlobalLeaf(ctx context.Context, index uint64) (model.GlobalLogLeaf, bool, error) {
	if err := ctx.Err(); err != nil {
		return model.GlobalLogLeaf{}, false, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore get global leaf canceled", err)
	}
	var leaf model.GlobalLogLeaf
	found, err := s.readCBOR(globalLeafKey(index), &leaf)
	if err != nil {
		return model.GlobalLogLeaf{}, false, trusterr.Wrap(trusterr.CodeDataLoss, "read global leaf", err)
	}
	return leaf, found, nil
}

func (s *Store) GetGlobalLeafByBatchID(ctx context.Context, batchID string) (model.GlobalLogLeaf, bool, error) {
	if err := ctx.Err(); err != nil {
		return model.GlobalLogLeaf{}, false, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore get global leaf by batch canceled", err)
	}
	if batchID == "" {
		return model.GlobalLogLeaf{}, false, trusterr.New(trusterr.CodeInvalidArgument, "batch_id is required")
	}
	var leaf model.GlobalLogLeaf
	found, err := s.readCBOR(globalBatchKey(batchID), &leaf)
	if err != nil {
		return model.GlobalLogLeaf{}, false, trusterr.Wrap(trusterr.CodeDataLoss, "read global leaf batch index", err)
	}
	return leaf, found, nil
}

func (s *Store) PutGlobalLogNode(ctx context.Context, node model.GlobalLogNode) error {
	if err := ctx.Err(); err != nil {
		return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore put global node canceled", err)
	}
	if node.Width == 0 {
		return trusterr.New(trusterr.CodeInvalidArgument, "global log node width is required")
	}
	if node.SchemaVersion == "" {
		node.SchemaVersion = model.SchemaGlobalLogNode
	}
	if node.CreatedAtUnixN == 0 {
		node.CreatedAtUnixN = time.Now().UTC().UnixNano()
	}
	if err := s.writeCBOR(globalNodeKey(node.Level, node.StartIndex), node); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "write global node", err)
	}
	return nil
}

func (s *Store) GetGlobalLogNode(ctx context.Context, level, startIndex uint64) (model.GlobalLogNode, bool, error) {
	if err := ctx.Err(); err != nil {
		return model.GlobalLogNode{}, false, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore get global node canceled", err)
	}
	var node model.GlobalLogNode
	found, err := s.readCBOR(globalNodeKey(level, startIndex), &node)
	if err != nil {
		return model.GlobalLogNode{}, false, trusterr.Wrap(trusterr.CodeDataLoss, "read global node", err)
	}
	return node, found, nil
}

func (s *Store) ListGlobalLogNodesAfter(ctx context.Context, afterLevel, afterStartIndex uint64, limit int) ([]model.GlobalLogNode, error) {
	if err := ctx.Err(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list global nodes after canceled", err)
	}
	if limit <= 0 {
		limit = 100
	}
	lower, upper := prefixBounds(prefixGlobalNode)
	iter, err := s.db.NewIter(&pdb.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "open global node iterator", err)
	}
	defer iter.Close()

	hasCursor := afterLevel != ^uint64(0) || afterStartIndex != ^uint64(0)
	ok := iter.First()
	if hasCursor {
		ok = iter.SeekGE(globalNodeKey(afterLevel, afterStartIndex))
	}
	nodes := make([]model.GlobalLogNode, 0, limit)
	for ; ok; ok = iter.Next() {
		if err := ctx.Err(); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list global nodes after canceled", err)
		}
		if len(nodes) >= limit {
			break
		}
		var node model.GlobalLogNode
		if err := cborx.UnmarshalLimit(iter.Value(), &node, maxStoredObjectBytes); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDataLoss, "decode global node", err)
		}
		if hasCursor && (node.Level < afterLevel || node.Level == afterLevel && node.StartIndex <= afterStartIndex) {
			continue
		}
		nodes = append(nodes, node)
	}
	if err := iter.Error(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "iterate global nodes after", err)
	}
	return nodes, nil
}

func (s *Store) PutGlobalLogState(ctx context.Context, state model.GlobalLogState) error {
	if err := ctx.Err(); err != nil {
		return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore put global state canceled", err)
	}
	if state.SchemaVersion == "" {
		state.SchemaVersion = model.SchemaGlobalLogState
	}
	if state.UpdatedAtUnixN == 0 {
		state.UpdatedAtUnixN = time.Now().UTC().UnixNano()
	}
	if err := s.writeCBOR([]byte(globalStateKey), state); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "write global state", err)
	}
	return nil
}

func (s *Store) GetGlobalLogState(ctx context.Context) (model.GlobalLogState, bool, error) {
	if err := ctx.Err(); err != nil {
		return model.GlobalLogState{}, false, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore get global state canceled", err)
	}
	var state model.GlobalLogState
	found, err := s.readCBOR([]byte(globalStateKey), &state)
	if err != nil {
		return model.GlobalLogState{}, false, trusterr.Wrap(trusterr.CodeDataLoss, "read global state", err)
	}
	return state, found, nil
}

func (s *Store) ListGlobalLeaves(ctx context.Context) ([]model.GlobalLogLeaf, error) {
	var leaves []model.GlobalLogLeaf
	err := s.scanPrefix(ctx, prefixGlobalLeaf, func(value []byte) error {
		var leaf model.GlobalLogLeaf
		if err := cborx.UnmarshalLimit(value, &leaf, maxStoredObjectBytes); err != nil {
			return err
		}
		leaves = append(leaves, leaf)
		return nil
	})
	if err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "list global leaves", err)
	}
	return leaves, nil
}

func (s *Store) ListGlobalLeavesRange(ctx context.Context, startIndex uint64, limit int) ([]model.GlobalLogLeaf, error) {
	if err := ctx.Err(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list global leaves range canceled", err)
	}
	if limit <= 0 {
		limit = 100
	}
	lower, upper := prefixBounds(prefixGlobalLeaf)
	iter, err := s.db.NewIter(&pdb.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "open global leaf iterator", err)
	}
	defer iter.Close()

	leaves := make([]model.GlobalLogLeaf, 0, limit)
	for ok := iter.SeekGE(globalLeafKey(startIndex)); ok; ok = iter.Next() {
		if err := ctx.Err(); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list global leaves range canceled", err)
		}
		if len(leaves) >= limit {
			break
		}
		var leaf model.GlobalLogLeaf
		if err := cborx.UnmarshalLimit(iter.Value(), &leaf, maxStoredObjectBytes); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDataLoss, "decode global leaf", err)
		}
		leaves = append(leaves, leaf)
	}
	if err := iter.Error(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "iterate global leaves range", err)
	}
	return leaves, nil
}

func (s *Store) ListGlobalLeavesPage(ctx context.Context, opts model.GlobalLeafListOptions) ([]model.GlobalLogLeaf, error) {
	if err := ctx.Err(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list global leaves page canceled", err)
	}
	limit := normaliseRecordLimit(opts.Limit)
	lower, upper := prefixBounds(prefixGlobalLeaf)
	iter, err := s.db.NewIter(&pdb.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "open global leaf iterator", err)
	}
	defer iter.Close()

	desc := !strings.EqualFold(opts.Direction, model.RecordListDirectionAsc)
	var ok bool
	if desc {
		if opts.AfterLeafIndex > 0 {
			ok = iter.SeekLT(globalLeafKey(opts.AfterLeafIndex))
		} else {
			ok = iter.Last()
		}
	} else if opts.AfterLeafIndex > 0 {
		ok = iter.SeekGE(globalLeafKey(opts.AfterLeafIndex))
	} else {
		ok = iter.First()
	}

	leaves := make([]model.GlobalLogLeaf, 0, limit)
	for ok {
		if err := ctx.Err(); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list global leaves page canceled", err)
		}
		if len(leaves) >= limit {
			break
		}
		var leaf model.GlobalLogLeaf
		if err := cborx.UnmarshalLimit(iter.Value(), &leaf, maxStoredObjectBytes); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDataLoss, "decode global log leaf", err)
		}
		if model.Uint64AfterCursor(leaf.LeafIndex, opts.AfterLeafIndex, opts.Direction) {
			leaves = append(leaves, leaf)
		}
		if desc {
			ok = iter.Prev()
		} else {
			ok = iter.Next()
		}
	}
	if err := iter.Error(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "iterate global leaves page", err)
	}
	return leaves, nil
}

func (s *Store) PutSignedTreeHead(ctx context.Context, sth model.SignedTreeHead) error {
	if err := ctx.Err(); err != nil {
		return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore put sth canceled", err)
	}
	if sth.TreeSize == 0 {
		return trusterr.New(trusterr.CodeInvalidArgument, "sth tree_size is required")
	}
	if sth.SchemaVersion == "" {
		sth.SchemaVersion = model.SchemaSignedTreeHead
	}
	if sth.TimestampUnixN == 0 {
		sth.TimestampUnixN = time.Now().UTC().UnixNano()
	}
	if err := s.writeCBOR(sthKey(sth.TreeSize), sth); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "write signed tree head", err)
	}
	return nil
}

func (s *Store) CommitGlobalLogAppend(ctx context.Context, entry model.GlobalLogAppend) error {
	if err := ctx.Err(); err != nil {
		return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore commit global log append canceled", err)
	}
	if entry.Leaf.BatchID == "" {
		return trusterr.New(trusterr.CodeInvalidArgument, "global log append leaf batch_id is required")
	}
	if entry.STH.TreeSize == 0 {
		return trusterr.New(trusterr.CodeInvalidArgument, "global log append STH tree_size is required")
	}
	if entry.Leaf.LeafIndex != entry.STH.TreeSize-1 {
		return trusterr.New(trusterr.CodeInvalidArgument, "global log append STH tree_size must match leaf index")
	}
	if entry.State.TreeSize != entry.STH.TreeSize {
		return trusterr.New(trusterr.CodeInvalidArgument, "global log append state and STH tree_size must match")
	}
	for _, node := range entry.Nodes {
		if node.Width == 0 {
			return trusterr.New(trusterr.CodeInvalidArgument, "global log append node width is required")
		}
	}
	if entry.Leaf.SchemaVersion == "" {
		entry.Leaf.SchemaVersion = model.SchemaGlobalLogLeaf
	}
	if entry.Leaf.AppendedAtUnixN == 0 {
		entry.Leaf.AppendedAtUnixN = time.Now().UTC().UnixNano()
	}
	if entry.State.SchemaVersion == "" {
		entry.State.SchemaVersion = model.SchemaGlobalLogState
	}
	if entry.State.UpdatedAtUnixN == 0 {
		entry.State.UpdatedAtUnixN = time.Now().UTC().UnixNano()
	}
	if entry.STH.SchemaVersion == "" {
		entry.STH.SchemaVersion = model.SchemaSignedTreeHead
	}
	if entry.STH.TimestampUnixN == 0 {
		entry.STH.TimestampUnixN = time.Now().UTC().UnixNano()
	}

	leafData, err := cborx.Marshal(entry.Leaf)
	if err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "encode global log append leaf", err)
	}
	stateData, err := cborx.Marshal(entry.State)
	if err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "encode global log append state", err)
	}
	sthData, err := cborx.Marshal(entry.STH)
	if err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "encode global log append STH", err)
	}

	batch := s.db.NewBatch()
	defer batch.Close()
	if err := batch.Set(globalLeafKey(entry.Leaf.LeafIndex), leafData, nil); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "stage global log append leaf", err)
	}
	if err := batch.Set(globalBatchKey(entry.Leaf.BatchID), leafData, nil); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "stage global log append leaf batch index", err)
	}
	for _, node := range entry.Nodes {
		if node.SchemaVersion == "" {
			node.SchemaVersion = model.SchemaGlobalLogNode
		}
		if node.CreatedAtUnixN == 0 {
			node.CreatedAtUnixN = time.Now().UTC().UnixNano()
		}
		nodeData, err := cborx.Marshal(node)
		if err != nil {
			return trusterr.Wrap(trusterr.CodeDataLoss, "encode global log append node", err)
		}
		if err := batch.Set(globalNodeKey(node.Level, node.StartIndex), nodeData, nil); err != nil {
			return trusterr.Wrap(trusterr.CodeDataLoss, "stage global log append node", err)
		}
	}
	if err := batch.Set([]byte(globalStateKey), stateData, nil); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "stage global log append state", err)
	}
	if err := batch.Set(sthKey(entry.STH.TreeSize), sthData, nil); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "stage global log append STH", err)
	}
	if err := batch.Commit(pdb.Sync); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "commit global log append", err)
	}
	return nil
}

func (s *Store) GetSignedTreeHead(ctx context.Context, treeSize uint64) (model.SignedTreeHead, bool, error) {
	if err := ctx.Err(); err != nil {
		return model.SignedTreeHead{}, false, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore get sth canceled", err)
	}
	if treeSize == 0 {
		return model.SignedTreeHead{}, false, trusterr.New(trusterr.CodeInvalidArgument, "sth tree_size is required")
	}
	var sth model.SignedTreeHead
	found, err := s.readCBOR(sthKey(treeSize), &sth)
	if err != nil {
		return model.SignedTreeHead{}, false, trusterr.Wrap(trusterr.CodeDataLoss, "read signed tree head", err)
	}
	return sth, found, nil
}

func (s *Store) ListSignedTreeHeadsAfter(ctx context.Context, afterTreeSize uint64, limit int) ([]model.SignedTreeHead, error) {
	if err := ctx.Err(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list sth after canceled", err)
	}
	if limit <= 0 {
		limit = 100
	}
	lower, upper := prefixBounds(prefixSTH)
	iter, err := s.db.NewIter(&pdb.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "open sth iterator", err)
	}
	defer iter.Close()

	sths := make([]model.SignedTreeHead, 0, limit)
	for ok := iter.SeekGE(sthKey(afterTreeSize + 1)); ok; ok = iter.Next() {
		if err := ctx.Err(); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list sth after canceled", err)
		}
		if len(sths) >= limit {
			break
		}
		var sth model.SignedTreeHead
		if err := cborx.UnmarshalLimit(iter.Value(), &sth, maxStoredObjectBytes); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDataLoss, "decode signed tree head", err)
		}
		sths = append(sths, sth)
	}
	if err := iter.Error(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "iterate sth after", err)
	}
	return sths, nil
}

func (s *Store) ListSignedTreeHeadsPage(ctx context.Context, opts model.TreeHeadListOptions) ([]model.SignedTreeHead, error) {
	if err := ctx.Err(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list signed tree heads page canceled", err)
	}
	limit := normaliseRecordLimit(opts.Limit)
	lower, upper := prefixBounds(prefixSTH)
	iter, err := s.db.NewIter(&pdb.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "open signed tree head iterator", err)
	}
	defer iter.Close()

	desc := !strings.EqualFold(opts.Direction, model.RecordListDirectionAsc)
	var ok bool
	if desc {
		if opts.AfterTreeSize > 0 {
			ok = iter.SeekLT(sthKey(opts.AfterTreeSize))
		} else {
			ok = iter.Last()
		}
	} else if opts.AfterTreeSize > 0 {
		ok = iter.SeekGE(sthKey(opts.AfterTreeSize))
	} else {
		ok = iter.First()
	}

	sths := make([]model.SignedTreeHead, 0, limit)
	for ok {
		if err := ctx.Err(); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list signed tree heads page canceled", err)
		}
		if len(sths) >= limit {
			break
		}
		var sth model.SignedTreeHead
		if err := cborx.UnmarshalLimit(iter.Value(), &sth, maxStoredObjectBytes); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDataLoss, "decode signed tree head", err)
		}
		if model.Uint64AfterCursor(sth.TreeSize, opts.AfterTreeSize, opts.Direction) {
			sths = append(sths, sth)
		}
		if desc {
			ok = iter.Prev()
		} else {
			ok = iter.Next()
		}
	}
	if err := iter.Error(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "iterate signed tree heads page", err)
	}
	return sths, nil
}

func (s *Store) LatestSignedTreeHead(ctx context.Context) (model.SignedTreeHead, bool, error) {
	if err := ctx.Err(); err != nil {
		return model.SignedTreeHead{}, false, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore latest sth canceled", err)
	}
	lower, upper := prefixBounds(prefixSTH)
	iter, err := s.db.NewIter(&pdb.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return model.SignedTreeHead{}, false, trusterr.Wrap(trusterr.CodeDataLoss, "open sth iterator", err)
	}
	defer iter.Close()
	if !iter.Last() {
		if err := iter.Error(); err != nil {
			return model.SignedTreeHead{}, false, trusterr.Wrap(trusterr.CodeDataLoss, "iterate sth", err)
		}
		return model.SignedTreeHead{}, false, nil
	}
	var sth model.SignedTreeHead
	if err := cborx.UnmarshalLimit(iter.Value(), &sth, maxStoredObjectBytes); err != nil {
		return model.SignedTreeHead{}, false, trusterr.Wrap(trusterr.CodeDataLoss, "decode latest sth", err)
	}
	return sth, true, nil
}

func (s *Store) PutGlobalLogTile(ctx context.Context, tile model.GlobalLogTile) error {
	if err := ctx.Err(); err != nil {
		return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore put global tile canceled", err)
	}
	if tile.SchemaVersion == "" {
		tile.SchemaVersion = model.SchemaGlobalLogTile
	}
	if tile.Width == 0 {
		return trusterr.New(trusterr.CodeInvalidArgument, "global log tile width is required")
	}
	if tile.CreatedAtUnixN == 0 {
		tile.CreatedAtUnixN = time.Now().UTC().UnixNano()
	}
	return s.writeCBOR(globalTileKey(tile), tile)
}

func (s *Store) ListGlobalLogTiles(ctx context.Context) ([]model.GlobalLogTile, error) {
	var tiles []model.GlobalLogTile
	err := s.scanPrefix(ctx, prefixGlobalTile, func(value []byte) error {
		var tile model.GlobalLogTile
		if err := cborx.UnmarshalLimit(value, &tile, maxStoredObjectBytes); err != nil {
			return err
		}
		tiles = append(tiles, tile)
		return nil
	})
	if err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "list global tiles", err)
	}
	return tiles, nil
}

func (s *Store) ListGlobalLogTilesAfter(ctx context.Context, afterLevel, afterStartIndex uint64, limit int) ([]model.GlobalLogTile, error) {
	if err := ctx.Err(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list global tiles after canceled", err)
	}
	if limit <= 0 {
		limit = 100
	}
	lower, upper := prefixBounds(prefixGlobalTile)
	iter, err := s.db.NewIter(&pdb.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "open global tile iterator", err)
	}
	defer iter.Close()

	hasCursor := afterLevel != ^uint64(0) || afterStartIndex != ^uint64(0)
	ok := iter.First()
	if hasCursor {
		start := []byte(fmt.Sprintf("%s%0*d/%0*d/", prefixGlobalTile, rootSortKeyWidth, afterLevel, rootSortKeyWidth, afterStartIndex))
		ok = iter.SeekGE(start)
	}
	tiles := make([]model.GlobalLogTile, 0, limit)
	for ; ok; ok = iter.Next() {
		if err := ctx.Err(); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list global tiles after canceled", err)
		}
		if len(tiles) >= limit {
			break
		}
		var tile model.GlobalLogTile
		if err := cborx.UnmarshalLimit(iter.Value(), &tile, maxStoredObjectBytes); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDataLoss, "decode global tile", err)
		}
		if hasCursor && (tile.Level < afterLevel || tile.Level == afterLevel && tile.StartIndex <= afterStartIndex) {
			continue
		}
		tiles = append(tiles, tile)
	}
	if err := iter.Error(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "iterate global tiles after", err)
	}
	return tiles, nil
}

func (s *Store) EnqueueGlobalLog(ctx context.Context, item model.GlobalLogOutboxItem) error {
	if err := ctx.Err(); err != nil {
		return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore enqueue global log canceled", err)
	}
	if item.BatchID == "" {
		item.BatchID = item.BatchRoot.BatchID
	}
	if item.BatchID == "" {
		return trusterr.New(trusterr.CodeInvalidArgument, "global log outbox batch_id is required")
	}
	if item.SchemaVersion == "" {
		item.SchemaVersion = model.SchemaGlobalLogOutbox
	}
	if item.Status == "" {
		item.Status = model.AnchorStatePending
	}
	if item.EnqueuedAtUnixN == 0 {
		item.EnqueuedAtUnixN = time.Now().UTC().UnixNano()
	}
	key := globalOutboxKey(item.BatchID)
	if _, closer, err := s.db.Get(key); err == nil {
		closer.Close()
		return trusterr.New(trusterr.CodeAlreadyExists, "global log outbox item already exists")
	} else if !isNotFound(err) {
		return trusterr.Wrap(trusterr.CodeDataLoss, "check global log outbox item", err)
	}
	data, err := cborx.Marshal(item)
	if err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "encode global log outbox item", err)
	}
	batch := s.db.NewBatch()
	defer batch.Close()
	if err := batch.Set(key, data, nil); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "stage global log outbox item", err)
	}
	if err := batch.Set(globalStatusKey(item.Status, globalStatusSortUnixN(item), item.BatchID), data, nil); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "stage global log status index", err)
	}
	if err := batch.Commit(pdb.Sync); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "commit global log outbox item", err)
	}
	return nil
}

func (s *Store) ListPendingGlobalLog(ctx context.Context, nowUnixN int64, limit int) ([]model.GlobalLogOutboxItem, error) {
	if limit <= 0 {
		limit = 100
	}
	items := make([]model.GlobalLogOutboxItem, 0, limit)
	err := s.scanPrefix(ctx, globalStatusPrefix(model.AnchorStatePending), func(value []byte) error {
		if len(items) >= limit {
			return errStopScan
		}
		var item model.GlobalLogOutboxItem
		if err := cborx.UnmarshalLimit(value, &item, maxStoredObjectBytes); err != nil {
			return err
		}
		if item.NextAttemptUnixN > nowUnixN {
			return errStopScan
		}
		items = append(items, item)
		return nil
	})
	if err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "list pending global log outbox", err)
	}
	return items, nil
}

func (s *Store) ListGlobalLogOutboxItemsAfter(ctx context.Context, afterBatchID string, limit int) ([]model.GlobalLogOutboxItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list global log outbox after canceled", err)
	}
	if limit <= 0 {
		limit = 100
	}
	lower, upper := prefixBounds(prefixGlobalOutbox)
	iter, err := s.db.NewIter(&pdb.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "open global log outbox iterator", err)
	}
	defer iter.Close()

	items := make([]model.GlobalLogOutboxItem, 0, limit)
	startKey := globalOutboxKey(afterBatchID)
	if afterBatchID == "" {
		startKey = []byte(prefixGlobalOutbox)
	}
	for ok := iter.SeekGE(startKey); ok; ok = iter.Next() {
		if err := ctx.Err(); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list global log outbox after canceled", err)
		}
		if len(items) >= limit {
			break
		}
		var item model.GlobalLogOutboxItem
		if err := cborx.UnmarshalLimit(iter.Value(), &item, maxStoredObjectBytes); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDataLoss, "decode global log outbox item", err)
		}
		if item.BatchID <= afterBatchID {
			continue
		}
		items = append(items, item)
	}
	if err := iter.Error(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "iterate global log outbox after", err)
	}
	return items, nil
}

func (s *Store) GetGlobalLogOutboxItem(ctx context.Context, batchID string) (model.GlobalLogOutboxItem, bool, error) {
	if err := ctx.Err(); err != nil {
		return model.GlobalLogOutboxItem{}, false, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore get global log outbox canceled", err)
	}
	if batchID == "" {
		return model.GlobalLogOutboxItem{}, false, trusterr.New(trusterr.CodeInvalidArgument, "batch_id is required")
	}
	var item model.GlobalLogOutboxItem
	found, err := s.readCBOR(globalOutboxKey(batchID), &item)
	if err != nil {
		return model.GlobalLogOutboxItem{}, false, trusterr.Wrap(trusterr.CodeDataLoss, "read global log outbox item", err)
	}
	return item, found, nil
}

func (s *Store) MarkGlobalLogPublished(ctx context.Context, batchID string, sth model.SignedTreeHead) error {
	item, ok, err := s.GetGlobalLogOutboxItem(ctx, batchID)
	if err != nil {
		return err
	}
	if !ok {
		return trusterr.New(trusterr.CodeNotFound, "global log outbox item not found")
	}
	old := item
	now := time.Now().UTC().UnixNano()
	item.Status = model.AnchorStatePublished
	item.STH = sth
	item.LastErrorMessage = ""
	item.LastAttemptUnixN = now
	item.NextAttemptUnixN = 0
	item.CompletedAtUnixN = now
	if err := s.replaceGlobalLogOutbox(ctx, old, item); err != nil {
		return err
	}
	return s.promoteBatchRecords(ctx, batchID, "L4")
}

func (s *Store) RescheduleGlobalLog(ctx context.Context, batchID string, attempts int, nextAttemptUnixN int64, lastErrorMessage string) error {
	item, ok, err := s.GetGlobalLogOutboxItem(ctx, batchID)
	if err != nil {
		return err
	}
	if !ok {
		return trusterr.New(trusterr.CodeNotFound, "global log outbox item not found")
	}
	old := item
	item.Status = model.AnchorStatePending
	item.Attempts = attempts
	item.NextAttemptUnixN = nextAttemptUnixN
	item.LastErrorMessage = lastErrorMessage
	item.LastAttemptUnixN = time.Now().UTC().UnixNano()
	return s.replaceGlobalLogOutbox(ctx, old, item)
}

func (s *Store) EnqueueSTHAnchor(ctx context.Context, item model.STHAnchorOutboxItem) error {
	if err := ctx.Err(); err != nil {
		return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore enqueue sth anchor canceled", err)
	}
	if item.TreeSize == 0 {
		return trusterr.New(trusterr.CodeInvalidArgument, "sth anchor tree_size is required")
	}
	if item.SchemaVersion == "" {
		item.SchemaVersion = model.SchemaSTHAnchorOutbox
	}
	if item.Status == "" {
		item.Status = model.AnchorStatePending
	}
	if item.EnqueuedAtUnixN == 0 {
		item.EnqueuedAtUnixN = time.Now().UTC().UnixNano()
	}
	key := anchorOutboxKey(item.TreeSize)
	if _, closer, err := s.db.Get(key); err == nil {
		closer.Close()
		return trusterr.New(trusterr.CodeAlreadyExists, "sth anchor outbox item already exists")
	} else if !isNotFound(err) {
		return trusterr.Wrap(trusterr.CodeDataLoss, "check sth anchor outbox item", err)
	}
	data, err := cborx.Marshal(item)
	if err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "encode sth anchor outbox item", err)
	}
	batch := s.db.NewBatch()
	defer batch.Close()
	if err := batch.Set(key, data, nil); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "stage sth anchor outbox item", err)
	}
	if err := batch.Set(anchorStatusKey(item.Status, anchorStatusSortUnixN(item), item.TreeSize), data, nil); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "stage sth anchor status index", err)
	}
	if err := batch.Commit(pdb.Sync); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "commit sth anchor outbox item", err)
	}
	return nil
}

func (s *Store) ListPendingSTHAnchors(ctx context.Context, nowUnixN int64, limit int) ([]model.STHAnchorOutboxItem, error) {
	if limit <= 0 {
		limit = 100
	}
	items := make([]model.STHAnchorOutboxItem, 0, limit)
	err := s.scanPrefix(ctx, anchorStatusPrefix(model.AnchorStatePending), func(value []byte) error {
		if len(items) >= limit {
			return errStopScan
		}
		var item model.STHAnchorOutboxItem
		if err := cborx.UnmarshalLimit(value, &item, maxStoredObjectBytes); err != nil {
			return err
		}
		if item.NextAttemptUnixN > nowUnixN {
			return errStopScan
		}
		items = append(items, item)
		return nil
	})
	if err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "list pending sth anchors", err)
	}
	return items, nil
}

func (s *Store) ListPublishedSTHAnchors(ctx context.Context, limit int) ([]model.STHAnchorOutboxItem, error) {
	if limit <= 0 {
		limit = 100
	}
	items := make([]model.STHAnchorOutboxItem, 0, limit)
	err := s.scanPrefix(ctx, anchorStatusPrefix(model.AnchorStatePublished), func(value []byte) error {
		if len(items) >= limit {
			return errStopScan
		}
		var item model.STHAnchorOutboxItem
		if err := cborx.UnmarshalLimit(value, &item, maxStoredObjectBytes); err != nil {
			return err
		}
		items = append(items, item)
		return nil
	})
	if err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "list published sth anchors", err)
	}
	return items, nil
}

func (s *Store) GetSTHAnchorOutboxItem(ctx context.Context, treeSize uint64) (model.STHAnchorOutboxItem, bool, error) {
	if err := ctx.Err(); err != nil {
		return model.STHAnchorOutboxItem{}, false, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore get sth anchor canceled", err)
	}
	if treeSize == 0 {
		return model.STHAnchorOutboxItem{}, false, trusterr.New(trusterr.CodeInvalidArgument, "tree_size is required")
	}
	var item model.STHAnchorOutboxItem
	found, err := s.readCBOR(anchorOutboxKey(treeSize), &item)
	if err != nil {
		return model.STHAnchorOutboxItem{}, false, trusterr.Wrap(trusterr.CodeDataLoss, "read sth anchor outbox item", err)
	}
	return item, found, nil
}

func (s *Store) ListSTHAnchorOutboxItemsAfter(ctx context.Context, afterTreeSize uint64, limit int) ([]model.STHAnchorOutboxItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list sth anchor outbox after canceled", err)
	}
	if limit <= 0 {
		limit = 100
	}
	lower, upper := prefixBounds(prefixAnchorOutbox)
	iter, err := s.db.NewIter(&pdb.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "open sth anchor outbox iterator", err)
	}
	defer iter.Close()

	items := make([]model.STHAnchorOutboxItem, 0, limit)
	for ok := iter.SeekGE(anchorOutboxKey(afterTreeSize + 1)); ok; ok = iter.Next() {
		if err := ctx.Err(); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list sth anchor outbox after canceled", err)
		}
		if len(items) >= limit {
			break
		}
		var item model.STHAnchorOutboxItem
		if err := cborx.UnmarshalLimit(iter.Value(), &item, maxStoredObjectBytes); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDataLoss, "decode sth anchor outbox item", err)
		}
		items = append(items, item)
	}
	if err := iter.Error(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "iterate sth anchor outbox after", err)
	}
	return items, nil
}

func (s *Store) ListSTHAnchorsPage(ctx context.Context, opts model.AnchorListOptions) ([]model.STHAnchorOutboxItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list sth anchors page canceled", err)
	}
	limit := normaliseRecordLimit(opts.Limit)
	lower, upper := prefixBounds(prefixAnchorOutbox)
	iter, err := s.db.NewIter(&pdb.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "open sth anchor outbox iterator", err)
	}
	defer iter.Close()

	desc := !strings.EqualFold(opts.Direction, model.RecordListDirectionAsc)
	var ok bool
	if desc {
		if opts.AfterTreeSize > 0 {
			ok = iter.SeekLT(anchorOutboxKey(opts.AfterTreeSize))
		} else {
			ok = iter.Last()
		}
	} else if opts.AfterTreeSize > 0 {
		ok = iter.SeekGE(anchorOutboxKey(opts.AfterTreeSize))
	} else {
		ok = iter.First()
	}

	items := make([]model.STHAnchorOutboxItem, 0, limit)
	for ok {
		if err := ctx.Err(); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore list sth anchors page canceled", err)
		}
		if len(items) >= limit {
			break
		}
		var item model.STHAnchorOutboxItem
		if err := cborx.UnmarshalLimit(iter.Value(), &item, maxStoredObjectBytes); err != nil {
			return nil, trusterr.Wrap(trusterr.CodeDataLoss, "decode sth anchor outbox item", err)
		}
		if model.Uint64AfterCursor(item.TreeSize, opts.AfterTreeSize, opts.Direction) {
			items = append(items, item)
		}
		if desc {
			ok = iter.Prev()
		} else {
			ok = iter.Next()
		}
	}
	if err := iter.Error(); err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "iterate sth anchors page", err)
	}
	return items, nil
}

func (s *Store) RescheduleSTHAnchor(ctx context.Context, treeSize uint64, attempts int, nextAttemptUnixN int64, lastErrorMessage string) error {
	item, ok, err := s.GetSTHAnchorOutboxItem(ctx, treeSize)
	if err != nil {
		return err
	}
	if !ok {
		return trusterr.New(trusterr.CodeNotFound, "sth anchor outbox item not found")
	}
	old := item
	item.Status = model.AnchorStatePending
	item.Attempts = attempts
	item.NextAttemptUnixN = nextAttemptUnixN
	item.LastErrorMessage = lastErrorMessage
	item.LastAttemptUnixN = time.Now().UTC().UnixNano()
	return s.replaceSTHAnchorOutbox(ctx, old, item)
}

func (s *Store) MarkSTHAnchorPublished(ctx context.Context, result model.STHAnchorResult) error {
	if err := ctx.Err(); err != nil {
		return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore mark sth anchor published canceled", err)
	}
	if result.TreeSize == 0 {
		return trusterr.New(trusterr.CodeInvalidArgument, "sth anchor result tree_size is required")
	}
	if result.SchemaVersion == "" {
		result.SchemaVersion = model.SchemaSTHAnchorResult
	}
	if result.PublishedAtUnixN == 0 {
		result.PublishedAtUnixN = time.Now().UTC().UnixNano()
	}
	item, ok, err := s.GetSTHAnchorOutboxItem(ctx, result.TreeSize)
	if err != nil {
		return err
	}
	if !ok {
		return trusterr.New(trusterr.CodeNotFound, "sth anchor outbox item not found")
	}
	old := item
	item.Status = model.AnchorStatePublished
	item.LastErrorMessage = ""
	item.LastAttemptUnixN = result.PublishedAtUnixN
	item.NextAttemptUnixN = 0

	resultBytes, err := cborx.Marshal(result)
	if err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "encode sth anchor result", err)
	}
	itemBytes, err := cborx.Marshal(item)
	if err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "encode sth anchor outbox item", err)
	}
	batch := s.db.NewBatch()
	defer batch.Close()
	if err := batch.Set(anchorResultKey(result.TreeSize), resultBytes, nil); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "stage sth anchor result", err)
	}
	if err := batch.Set(anchorOutboxKey(result.TreeSize), itemBytes, nil); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "stage sth anchor outbox item", err)
	}
	if err := batch.Delete(anchorStatusKey(old.Status, anchorStatusSortUnixN(old), old.TreeSize), nil); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "stage old sth anchor status delete", err)
	}
	if err := batch.Set(anchorStatusKey(item.Status, anchorStatusSortUnixN(item), item.TreeSize), itemBytes, nil); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "stage sth anchor status index", err)
	}
	if err := batch.Commit(pdb.Sync); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "commit sth anchor published batch", err)
	}
	leaf, ok, err := s.GetGlobalLeaf(ctx, result.TreeSize-1)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	return s.promoteBatchRecords(ctx, leaf.BatchID, "L5")
}

func (s *Store) MarkSTHAnchorFailed(ctx context.Context, treeSize uint64, lastErrorMessage string) error {
	item, ok, err := s.GetSTHAnchorOutboxItem(ctx, treeSize)
	if err != nil {
		return err
	}
	if !ok {
		return trusterr.New(trusterr.CodeNotFound, "sth anchor outbox item not found")
	}
	old := item
	item.Status = model.AnchorStateFailed
	item.LastErrorMessage = lastErrorMessage
	item.LastAttemptUnixN = time.Now().UTC().UnixNano()
	item.NextAttemptUnixN = 0
	return s.replaceSTHAnchorOutbox(ctx, old, item)
}

func (s *Store) GetSTHAnchorResult(ctx context.Context, treeSize uint64) (model.STHAnchorResult, bool, error) {
	if err := ctx.Err(); err != nil {
		return model.STHAnchorResult{}, false, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore get sth anchor result canceled", err)
	}
	if treeSize == 0 {
		return model.STHAnchorResult{}, false, trusterr.New(trusterr.CodeInvalidArgument, "tree_size is required")
	}
	var result model.STHAnchorResult
	found, err := s.readCBOR(anchorResultKey(treeSize), &result)
	if err != nil {
		return model.STHAnchorResult{}, false, trusterr.Wrap(trusterr.CodeDataLoss, "read sth anchor result", err)
	}
	return result, found, nil
}

func (s *Store) listSTHAnchors(ctx context.Context, limit int, include func(model.STHAnchorOutboxItem) bool) ([]model.STHAnchorOutboxItem, error) {
	if limit <= 0 {
		limit = 100
	}
	items := []model.STHAnchorOutboxItem{}
	err := s.scanPrefix(ctx, prefixAnchorOutbox, func(value []byte) error {
		var item model.STHAnchorOutboxItem
		if err := cborx.UnmarshalLimit(value, &item, maxStoredObjectBytes); err != nil {
			return err
		}
		if include(item) {
			items = append(items, item)
		}
		return nil
	})
	if err != nil {
		return nil, trusterr.Wrap(trusterr.CodeDataLoss, "list sth anchors", err)
	}
	sortSTHAnchorItems(items)
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func sortSTHAnchorItems(items []model.STHAnchorOutboxItem) {
	for i := 1; i < len(items); i++ {
		j := i
		for j > 0 && items[j-1].EnqueuedAtUnixN > items[j].EnqueuedAtUnixN {
			items[j-1], items[j] = items[j], items[j-1]
			j--
		}
	}
}

func recordListPrefix(opts model.RecordListOptions) string {
	switch {
	case len(opts.ContentHash) > 0:
		return prefixRecordByHash + hex.EncodeToString(opts.ContentHash) + "/"
	case model.RecordStorageQueryToken(opts.Query) != "":
		return prefixRecordByToken + recordSecondaryPart(model.RecordStorageQueryToken(opts.Query)) + "/"
	case opts.BatchID != "":
		return prefixRecordByBatch + recordSecondaryPart(opts.BatchID) + "/"
	case opts.ProofLevel != "":
		return prefixRecordByLevel + recordSecondaryPart(opts.ProofLevel) + "/"
	case opts.TenantID != "":
		return prefixRecordByTenant + recordSecondaryPart(opts.TenantID) + "/"
	case opts.ClientID != "":
		return prefixRecordByClient + recordSecondaryPart(opts.ClientID) + "/"
	default:
		return prefixRecordByTime
	}
}

func normaliseRecordLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}

func recordRangeExhausted(idx model.RecordIndex, opts model.RecordListOptions, desc bool) bool {
	if desc {
		return opts.ReceivedFromUnixN > 0 && idx.ReceivedAtUnixN < opts.ReceivedFromUnixN
	}
	return opts.ReceivedToUnixN > 0 && idx.ReceivedAtUnixN > opts.ReceivedToUnixN
}

func (s *Store) stageRecordIndexReplace(batch *pdb.Batch, idx, old model.RecordIndex, oldFound bool) error {
	encoded, err := encodeRecordIndexArtifact(idx)
	if err != nil {
		return err
	}
	return s.stageEncodedRecordIndexReplace(batch, encoded, old, oldFound)
}

func (s *Store) stageEncodedRecordIndexReplace(batch *pdb.Batch, idx encodedRecordIndex, old model.RecordIndex, oldFound bool) error {
	if oldFound {
		for _, key := range recordIndexKeys(old) {
			if err := batch.Delete(key, nil); err != nil {
				return trusterr.Wrap(trusterr.CodeDataLoss, "stage old record index delete", err)
			}
		}
	}
	return s.stageEncodedRecordIndexSet(batch, idx)
}

func (s *Store) stageRecordIndexSet(batch *pdb.Batch, idx model.RecordIndex) error {
	encoded, err := encodeRecordIndexArtifact(idx)
	if err != nil {
		return err
	}
	return s.stageEncodedRecordIndexSet(batch, encoded)
}

func (s *Store) stageEncodedRecordIndexSet(batch *pdb.Batch, idx encodedRecordIndex) error {
	if err := stageSet(batch, idx.primaryKey, idx.value); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "stage record index", err)
	}
	for _, key := range idx.secondaryKeys {
		if err := stageSet(batch, key, idx.refValue); err != nil {
			return trusterr.Wrap(trusterr.CodeDataLoss, "stage secondary record index", err)
		}
	}
	return nil
}

func (s *Store) replaceGlobalLogOutbox(ctx context.Context, old, next model.GlobalLogOutboxItem) error {
	if err := ctx.Err(); err != nil {
		return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore update global log outbox canceled", err)
	}
	data, err := cborx.Marshal(next)
	if err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "encode global log outbox item", err)
	}
	batch := s.db.NewBatch()
	defer batch.Close()
	if err := batch.Set(globalOutboxKey(next.BatchID), data, nil); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "stage global log outbox item", err)
	}
	if old.BatchID != "" && old.Status != "" {
		if err := batch.Delete(globalStatusKey(old.Status, globalStatusSortUnixN(old), old.BatchID), nil); err != nil {
			return trusterr.Wrap(trusterr.CodeDataLoss, "stage old global log status delete", err)
		}
	}
	if err := batch.Set(globalStatusKey(next.Status, globalStatusSortUnixN(next), next.BatchID), data, nil); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "stage global log status index", err)
	}
	if err := batch.Commit(pdb.Sync); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "commit global log outbox update", err)
	}
	return nil
}

func (s *Store) promoteBatchRecords(ctx context.Context, batchID, proofLevel string) error {
	if batchID == "" {
		return nil
	}
	prefix := prefixRecordByBatch + recordSecondaryPart(batchID) + "/"
	updates := make([]recordIndexPromotion, 0, 16)
	err := s.scanPrefix(ctx, prefix, func(value []byte) error {
		idx, err := s.readRecordIndexScanValue(value)
		if err != nil {
			return err
		}
		if model.ProofLevelRank(model.RecordIndexProofLevel(idx)) >= model.ProofLevelRank(proofLevel) {
			return nil
		}
		next := idx
		next.ProofLevel = proofLevel
		updates = append(updates, recordIndexPromotion{old: idx, next: next})
		return nil
	})
	if err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "scan batch record indexes", err)
	}
	return s.commitRecordIndexPromotions(ctx, updates)
}

type recordIndexPromotion struct {
	old  model.RecordIndex
	next model.RecordIndex
}

func (s *Store) commitRecordIndexPromotions(ctx context.Context, updates []recordIndexPromotion) error {
	for start := 0; start < len(updates); start += batchArtifactChunkSize {
		end := start + batchArtifactChunkSize
		if end > len(updates) {
			end = len(updates)
		}
		if err := ctx.Err(); err != nil {
			return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore promote batch records canceled", err)
		}
		batch := s.db.NewBatch()
		for i := start; i < end; i++ {
			if err := s.stageRecordIndexReplace(batch, updates[i].next, updates[i].old, true); err != nil {
				_ = batch.Close()
				return err
			}
		}
		if err := batch.Commit(pdb.Sync); err != nil {
			_ = batch.Close()
			return trusterr.Wrap(trusterr.CodeDataLoss, "commit promoted record indexes", err)
		}
		if err := batch.Close(); err != nil {
			return trusterr.Wrap(trusterr.CodeDataLoss, "close promoted record indexes", err)
		}
	}
	return nil
}

func (s *Store) replaceSTHAnchorOutbox(ctx context.Context, old, next model.STHAnchorOutboxItem) error {
	if err := ctx.Err(); err != nil {
		return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "proofstore update sth anchor canceled", err)
	}
	data, err := cborx.Marshal(next)
	if err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "encode sth anchor outbox item", err)
	}
	batch := s.db.NewBatch()
	defer batch.Close()
	if err := batch.Set(anchorOutboxKey(next.TreeSize), data, nil); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "stage sth anchor outbox item", err)
	}
	if old.TreeSize != 0 && old.Status != "" {
		if err := batch.Delete(anchorStatusKey(old.Status, anchorStatusSortUnixN(old), old.TreeSize), nil); err != nil {
			return trusterr.Wrap(trusterr.CodeDataLoss, "stage old sth anchor status delete", err)
		}
	}
	if err := batch.Set(anchorStatusKey(next.Status, anchorStatusSortUnixN(next), next.TreeSize), data, nil); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "stage sth anchor status index", err)
	}
	if err := batch.Commit(pdb.Sync); err != nil {
		return trusterr.Wrap(trusterr.CodeDataLoss, "commit sth anchor update", err)
	}
	return nil
}

func (s *Store) scanPrefix(ctx context.Context, prefix string, visit func([]byte) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	lower, upper := prefixBounds(prefix)
	iter, err := s.db.NewIter(&pdb.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return err
	}
	defer iter.Close()
	for ok := iter.First(); ok; ok = iter.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := visit(iter.Value()); err != nil {
			if errors.Is(err, errStopScan) {
				return nil
			}
			return err
		}
	}
	return iter.Error()
}

func prefixBounds(prefix string) (lower, upper []byte) {
	lower = []byte(prefix)
	upper = append([]byte(prefix), 0xff)
	return lower, upper
}
