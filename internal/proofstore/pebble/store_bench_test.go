package pebble

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	pdb "github.com/cockroachdb/pebble"

	"github.com/ryan-wong-coder/trustdb/internal/cborx"
	"github.com/ryan-wong-coder/trustdb/internal/model"
)

func BenchmarkPebblePutBatchArtifacts1024(b *testing.B) {
	benchmarkPebblePutBatchArtifacts(b, 1024)
}

func BenchmarkPebblePutBatchArtifacts8192(b *testing.B) {
	benchmarkPebblePutBatchArtifacts(b, 8192)
}

func benchmarkPebblePutBatchArtifacts(b *testing.B, n int) {
	store, err := Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = store.Close() })
	bundles := syntheticProofBundles(n)
	root := model.BatchRoot{
		SchemaVersion: model.SchemaBatchRoot,
		BatchID:       "bench-batch",
		BatchRoot:     bytes.Repeat([]byte{1}, 32),
		TreeSize:      uint64(len(bundles)),
		ClosedAtUnixN: 1_000,
	}
	ctx := context.Background()

	b.ReportAllocs()
	for b.Loop() {
		if err := store.PutBatchArtifacts(ctx, bundles, root); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPebbleGetBundleV2(b *testing.B) {
	store, err := Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = store.Close() })
	bundle := syntheticCompressibleProofBundle("bench-record-v2")
	if err := store.PutBundle(context.Background(), bundle); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		got, err := store.GetBundle(context.Background(), bundle.RecordID)
		if err != nil {
			b.Fatal(err)
		}
		if got.RecordID == "" {
			b.Fatal("empty proof bundle")
		}
	}
}

func TestStorePutBundleWritesCompressedV2AndRoundTrips(t *testing.T) {
	t.Parallel()

	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	bundle := syntheticCompressibleProofBundle("tr-compressed")
	if err := store.PutBundle(context.Background(), bundle); err != nil {
		t.Fatalf("PutBundle: %v", err)
	}
	val, closer, err := store.db.Get(bundleV2Key(bundle.RecordID))
	if err != nil {
		t.Fatalf("get v2 bundle key: %v", err)
	}
	var envelope storedProofBundleEnvelope
	if err := cborx.UnmarshalLimit(val, &envelope, maxStoredObjectBytes); err != nil {
		_ = closer.Close()
		t.Fatalf("decode v2 envelope: %v", err)
	}
	_ = closer.Close()
	if envelope.SchemaVersion != schemaStoredProofBundleV2 || envelope.Codec != storedBundleCodecSnappy {
		t.Fatalf("envelope = %+v", envelope)
	}
	got, err := store.GetBundle(context.Background(), bundle.RecordID)
	if err != nil {
		t.Fatalf("GetBundle: %v", err)
	}
	if got.RecordID != bundle.RecordID || len(got.BatchProof.AuditPath) != len(bundle.BatchProof.AuditPath) {
		t.Fatalf("round trip = %+v", got)
	}
}

func TestStoreGetBundleReadsLegacyBundle(t *testing.T) {
	t.Parallel()

	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	bundle := syntheticProofBundles(1)[0]
	data, err := cborx.Marshal(bundle)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := store.db.Set(bundleKey(bundle.RecordID), data, pdb.Sync); err != nil {
		t.Fatalf("write legacy bundle: %v", err)
	}
	got, err := store.GetBundle(context.Background(), bundle.RecordID)
	if err != nil {
		t.Fatalf("GetBundle: %v", err)
	}
	if got.RecordID != bundle.RecordID || got.CommittedReceipt.BatchID != bundle.CommittedReceipt.BatchID {
		t.Fatalf("legacy round trip = %+v", got)
	}
}

func TestStoreSecondaryRecordIndexesUseRefs(t *testing.T) {
	t.Parallel()

	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	bundle := syntheticProofBundles(1)[0]
	if err := store.PutBundle(context.Background(), bundle); err != nil {
		t.Fatalf("PutBundle: %v", err)
	}
	iter, err := store.db.NewIter(&pdb.IterOptions{
		LowerBound: []byte(prefixRecordByBatch),
		UpperBound: []byte("record/by-batch0"),
	})
	if err != nil {
		t.Fatalf("NewIter: %v", err)
	}
	defer iter.Close()
	if ok := iter.First(); !ok {
		t.Fatalf("missing batch secondary index: %v", iter.Error())
	}
	recordID, ok := decodeRecordIndexRef(iter.Value())
	if !ok || recordID != bundle.RecordID {
		t.Fatalf("secondary index value is not a record ref: id=%q ok=%v raw=%x", recordID, ok, iter.Value())
	}
	records, err := store.ListRecordIndexes(context.Background(), model.RecordListOptions{BatchID: bundle.CommittedReceipt.BatchID})
	if err != nil {
		t.Fatalf("ListRecordIndexes: %v", err)
	}
	if len(records) != 1 || records[0].RecordID != bundle.RecordID {
		t.Fatalf("records = %+v", records)
	}
}

func syntheticProofBundles(n int) []model.ProofBundle {
	bundles := make([]model.ProofBundle, n)
	for i := range bundles {
		recordID := fmt.Sprintf("bench-record-%04d", i)
		bundles[i] = model.ProofBundle{
			SchemaVersion: model.SchemaProofBundle,
			RecordID:      recordID,
			SignedClaim: model.SignedClaim{
				SchemaVersion: model.SchemaSignedClaim,
				Claim: model.ClientClaim{
					SchemaVersion: model.SchemaClientClaim,
					TenantID:      "bench-tenant",
					ClientID:      "bench-client",
					KeyID:         "bench-key",
					Content: model.Content{
						HashAlg:       model.DefaultHashAlg,
						ContentHash:   bytes.Repeat([]byte{byte(i % 251)}, 32),
						ContentLength: 1024,
						StorageURI:    "bench://" + recordID,
					},
					Metadata: model.Metadata{EventType: "bench.synthetic"},
				},
			},
			ServerRecord: model.ServerRecord{
				SchemaVersion:   model.SchemaServerRecord,
				RecordID:        recordID,
				TenantID:        "bench-tenant",
				ClientID:        "bench-client",
				KeyID:           "bench-key",
				ReceivedAtUnixN: int64(1_000 + i),
				WAL:             model.WALPosition{SegmentID: 1, Offset: int64(i * 512), Sequence: uint64(i + 1)},
			},
			CommittedReceipt: model.CommittedReceipt{
				SchemaVersion: model.SchemaCommittedReceipt,
				RecordID:      recordID,
				BatchID:       "bench-batch",
				LeafIndex:     uint64(i),
				BatchRoot:     bytes.Repeat([]byte{1}, 32),
				ClosedAtUnixN: 1_000,
			},
			BatchProof: model.BatchProof{
				TreeAlg:   model.DefaultMerkleTreeAlg,
				LeafIndex: uint64(i),
				TreeSize:  uint64(n),
				AuditPath: [][]byte{bytes.Repeat([]byte{byte((i + 1) % 251)}, 32)},
			},
		}
	}
	return bundles
}

func syntheticCompressibleProofBundle(recordID string) model.ProofBundle {
	bundle := syntheticProofBundles(1)[0]
	bundle.RecordID = recordID
	bundle.SignedClaim.Signature.Signature = bytes.Repeat([]byte{7}, 4096)
	bundle.AcceptedReceipt.ServerSig.Signature = bytes.Repeat([]byte{8}, 4096)
	bundle.CommittedReceipt.ServerSig.Signature = bytes.Repeat([]byte{9}, 4096)
	bundle.BatchProof.AuditPath = make([][]byte, 128)
	for i := range bundle.BatchProof.AuditPath {
		bundle.BatchProof.AuditPath[i] = bytes.Repeat([]byte{byte(i % 8)}, 32)
	}
	return bundle
}
