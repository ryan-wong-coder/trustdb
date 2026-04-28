package observability

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsRegisterAndExpose(t *testing.T) {
	t.Parallel()

	reg, metrics := NewRegistry()
	metrics.IngestRequests.WithLabelValues("accepted").Inc()
	metrics.IngestRejected.WithLabelValues("RESOURCE_EXHAUSTED").Add(2)
	metrics.QueueDepth.WithLabelValues("ingest").Set(7)

	expected := `
# HELP trustdb_ingest_rejected_total Total rejected ingest requests by reason.
# TYPE trustdb_ingest_rejected_total counter
trustdb_ingest_rejected_total{reason="RESOURCE_EXHAUSTED"} 2
# HELP trustdb_ingest_requests_total Total ingest requests by result.
# TYPE trustdb_ingest_requests_total counter
trustdb_ingest_requests_total{result="accepted"} 1
# HELP trustdb_queue_depth Current queue depth by queue name.
# TYPE trustdb_queue_depth gauge
trustdb_queue_depth{queue="ingest"} 7
`
	if err := testutil.GatherAndCompare(
		reg,
		strings.NewReader(expected),
		"trustdb_ingest_requests_total",
		"trustdb_ingest_rejected_total",
		"trustdb_queue_depth",
	); err != nil {
		t.Fatalf("GatherAndCompare() error = %v", err)
	}

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	Handler(reg).ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("metrics handler status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "trustdb_ingest_requests_total") {
		t.Fatalf("metrics handler body missing trustdb metric")
	}
}

func TestWALCheckpointAndReplayMetrics(t *testing.T) {
	t.Parallel()

	reg, metrics := NewRegistry()
	metrics.WALCheckpointLastSequence.Set(42)
	metrics.WALReplayRecords.WithLabelValues("skipped").Add(5)
	metrics.WALReplayRecords.WithLabelValues("replayed").Add(2)
	metrics.WALReplayRecords.WithLabelValues("recovered").Add(1)

	expected := `
# HELP trustdb_wal_checkpoint_last_sequence Highest WAL sequence that a committed batch has advanced the checkpoint to.
# TYPE trustdb_wal_checkpoint_last_sequence gauge
trustdb_wal_checkpoint_last_sequence 42
# HELP trustdb_wal_replay_records_total WAL records handled during startup replay, broken down by outcome.
# TYPE trustdb_wal_replay_records_total counter
trustdb_wal_replay_records_total{result="recovered"} 1
trustdb_wal_replay_records_total{result="replayed"} 2
trustdb_wal_replay_records_total{result="skipped"} 5
`
	if err := testutil.GatherAndCompare(
		reg,
		strings.NewReader(expected),
		"trustdb_wal_checkpoint_last_sequence",
		"trustdb_wal_replay_records_total",
	); err != nil {
		t.Fatalf("GatherAndCompare() error = %v", err)
	}
}

// TestWALSegmentMetrics exercises the segment-rotation metric family. The
// gauges are set directly (they are pushed by serve on rotation/prune) while
// the counter is accumulated as prune removes bytes. We assert registration,
// help text, and a small value matrix to pin down the exposition format.
func TestWALSegmentMetrics(t *testing.T) {
	t.Parallel()

	reg, metrics := NewRegistry()
	metrics.WALActiveSegmentID.Set(7)
	metrics.WALSegmentsTotal.Set(4)
	metrics.WALBytesPrunedTotal.Add(1024)
	metrics.WALBytesPrunedTotal.Add(2048)

	expected := `
# HELP trustdb_wal_active_segment_id Id of the WAL segment the writer is currently appending to.
# TYPE trustdb_wal_active_segment_id gauge
trustdb_wal_active_segment_id 7
# HELP trustdb_wal_bytes_pruned_total Cumulative bytes reclaimed from pruned WAL segments since process start.
# TYPE trustdb_wal_bytes_pruned_total counter
trustdb_wal_bytes_pruned_total 3072
# HELP trustdb_wal_segments_total Number of WAL segment files currently present on disk.
# TYPE trustdb_wal_segments_total gauge
trustdb_wal_segments_total 4
`
	if err := testutil.GatherAndCompare(
		reg,
		strings.NewReader(expected),
		"trustdb_wal_active_segment_id",
		"trustdb_wal_segments_total",
		"trustdb_wal_bytes_pruned_total",
	); err != nil {
		t.Fatalf("GatherAndCompare() error = %v", err)
	}
}
