package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestEmitBenchIngestResultWritesReportFile(t *testing.T) {
	t.Parallel()

	reportPath := filepath.Join(t.TempDir(), "bench-report.json")
	var out bytes.Buffer
	result := benchIngestResult{
		SchemaVersion:    benchIngestReportSchema,
		Endpoint:         "http://127.0.0.1:8080",
		Transport:        "http",
		Count:            4,
		Concurrency:      2,
		PayloadBytes:     256,
		Submitted:        4,
		ThroughputPerSec: 12.5,
		QuerySamples:     benchQuerySummary{Samples: 1, Ready: 1},
		ProofSamples:     benchProofSummary{Samples: 1, TargetLevel: "L4", Ready: 1},
		Metrics:          []benchMetricDelta{{Name: "trustdb_pebble_wal_bytes_written_total", Delta: 32, After: 32}},
	}

	err := emitBenchIngestResult(&runtimeConfig{out: &out}, benchIngestConfig{
		OutputFormat: "text",
		ReportFile:   reportPath,
	}, result)
	if err != nil {
		t.Fatalf("emitBenchIngestResult() error = %v", err)
	}
	if !strings.Contains(out.String(), "report_file: "+reportPath) {
		t.Fatalf("stdout missing report file hint: %q", out.String())
	}

	loaded, err := readBenchIngestReportFile(reportPath)
	if err != nil {
		t.Fatalf("readBenchIngestReportFile() error = %v", err)
	}
	if loaded.SchemaVersion != benchIngestReportSchema || loaded.Endpoint != result.Endpoint {
		t.Fatalf("loaded report = %+v", loaded)
	}
}

func TestCompareBenchIngestResults(t *testing.T) {
	t.Parallel()

	baseline := benchIngestResult{
		SchemaVersion:    benchIngestReportSchema,
		Endpoint:         "http://baseline",
		Transport:        "http",
		Submitted:        100,
		Failed:           2,
		ThroughputPerSec: 100,
		SubmitLatency:    benchLatencySummary{AvgMs: 12, P95Ms: 20, P99Ms: 25},
		QuerySamples:     benchQuerySummary{Ready: 8, Failed: 1},
		ProofSamples:     benchProofSummary{Ready: 8, Timeouts: 1, Failed: 0},
		Metrics: []benchMetricDelta{
			{Name: "trustdb_pebble_wal_bytes_written_total", Delta: 1024},
			{Name: "trustdb_queue_depth{queue=\"ingest\"}", Delta: 0},
		},
	}
	candidate := benchIngestResult{
		SchemaVersion:    benchIngestReportSchema,
		Endpoint:         "http://candidate",
		Transport:        "grpc",
		Submitted:        120,
		Failed:           1,
		ThroughputPerSec: 150,
		SubmitLatency:    benchLatencySummary{AvgMs: 10, P95Ms: 18, P99Ms: 22},
		QuerySamples:     benchQuerySummary{Ready: 8, Failed: 0},
		ProofSamples:     benchProofSummary{Ready: 8, Timeouts: 0, Failed: 0},
		Metrics: []benchMetricDelta{
			{Name: "trustdb_pebble_wal_bytes_written_total", Delta: 1400},
			{Name: "trustdb_pebble_memtable_size_bytes", Delta: 64},
		},
	}

	result := compareBenchIngestResults("baseline.json", "candidate.json", baseline, candidate)
	if result.SchemaVersion != benchCompareReportSchema {
		t.Fatalf("schema = %q", result.SchemaVersion)
	}
	if result.Summary.ThroughputPerSec.Delta != 50 {
		t.Fatalf("throughput delta = %+v", result.Summary.ThroughputPerSec)
	}
	if result.Summary.ThroughputPerSec.DeltaPct == nil || *result.Summary.ThroughputPerSec.DeltaPct != 50 {
		t.Fatalf("throughput delta pct = %+v", result.Summary.ThroughputPerSec)
	}
	if len(result.Metrics) != 2 {
		t.Fatalf("metrics len = %d, want 2: %+v", len(result.Metrics), result.Metrics)
	}
	if result.Metrics[0].Name != "trustdb_pebble_memtable_size_bytes" || result.Metrics[1].Name != "trustdb_pebble_wal_bytes_written_total" {
		t.Fatalf("metric ordering = %+v", result.Metrics)
	}
}

func TestBenchCompareCommandJSON(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	baselinePath := filepath.Join(tmp, "baseline.json")
	candidatePath := filepath.Join(tmp, "candidate.json")
	if err := writeJSONFile(baselinePath, benchIngestResult{
		Endpoint:         "http://baseline",
		Transport:        "http",
		Submitted:        10,
		ThroughputPerSec: 100,
		SubmitLatency:    benchLatencySummary{AvgMs: 10, P95Ms: 20, P99Ms: 30},
		ProofSamples:     benchProofSummary{Ready: 2},
		Metrics:          []benchMetricDelta{{Name: "trustdb_pebble_wal_bytes_written_total", Delta: 100}},
	}); err != nil {
		t.Fatalf("write baseline report: %v", err)
	}
	if err := writeJSONFile(candidatePath, benchIngestResult{
		SchemaVersion:    benchIngestReportSchema,
		Endpoint:         "http://candidate",
		Transport:        "grpc",
		Submitted:        12,
		ThroughputPerSec: 120,
		SubmitLatency:    benchLatencySummary{AvgMs: 9, P95Ms: 18, P99Ms: 27},
		ProofSamples:     benchProofSummary{Ready: 2},
		Metrics:          []benchMetricDelta{{Name: "trustdb_pebble_wal_bytes_written_total", Delta: 130}},
	}); err != nil {
		t.Fatalf("write candidate report: %v", err)
	}

	var out, errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut)
	cmd.SetArgs([]string{
		"bench", "compare",
		"--baseline", baselinePath,
		"--candidate", candidatePath,
		"--output", "json",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("bench compare execute error = %v stderr=%s", err, errOut.String())
	}

	var result benchCompareResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal(compare output): %v body=%q", err, out.String())
	}
	if result.BaselinePath != baselinePath || result.CandidatePath != candidatePath {
		t.Fatalf("compare paths = %+v", result)
	}
	if result.Baseline.SchemaVersion != benchIngestReportSchema {
		t.Fatalf("baseline schema fallback missing: %+v", result.Baseline)
	}
	if result.Summary.Submitted.Delta != 2 {
		t.Fatalf("submitted delta = %+v", result.Summary.Submitted)
	}
}

func TestEvaluateBenchCompareAssertions(t *testing.T) {
	t.Parallel()

	flags := pflag.NewFlagSet("bench-compare", pflag.ContinueOnError)
	flags.Float64("min-candidate-throughput", 0, "")
	flags.Float64("max-throughput-regression-pct", 0, "")
	flags.Float64("max-duration-regression-pct", 0, "")
	flags.Float64("max-submit-p95-regression-pct", 0, "")
	flags.Float64("max-candidate-submit-p95-ms", 0, "")
	flags.Int("max-candidate-failed", 0, "")
	flags.Int("max-candidate-batch-errors", 0, "")
	flags.Int("max-candidate-query-failed", 0, "")
	flags.Int("max-candidate-proof-timeouts", 0, "")
	flags.Int("max-candidate-proof-failed", 0, "")
	args := []string{
		"--min-candidate-throughput=90",
		"--max-throughput-regression-pct=15",
		"--max-duration-regression-pct=10",
		"--max-submit-p95-regression-pct=25",
		"--max-candidate-submit-p95-ms=18",
		"--max-candidate-failed=1",
		"--max-candidate-batch-errors=0",
		"--max-candidate-query-failed=0",
		"--max-candidate-proof-timeouts=0",
		"--max-candidate-proof-failed=0",
	}
	if err := flags.Parse(args); err != nil {
		t.Fatalf("flags.Parse(): %v", err)
	}
	cmd := &cobra.Command{Use: "compare"}
	cmd.Flags().AddFlagSet(flags)

	result := compareBenchIngestResults("baseline.json", "candidate.json",
		benchIngestResult{
			SchemaVersion:    benchIngestReportSchema,
			Submitted:        10,
			Failed:           0,
			BatchErrors:      0,
			DurationSeconds:  10,
			ThroughputPerSec: 100,
			SubmitLatency:    benchLatencySummary{P95Ms: 10},
			QuerySamples:     benchQuerySummary{Failed: 0},
			ProofSamples:     benchProofSummary{Timeouts: 0, Failed: 0},
		},
		benchIngestResult{
			SchemaVersion:    benchIngestReportSchema,
			Submitted:        10,
			Failed:           2,
			BatchErrors:      1,
			DurationSeconds:  12,
			ThroughputPerSec: 80,
			SubmitLatency:    benchLatencySummary{P95Ms: 20},
			QuerySamples:     benchQuerySummary{Failed: 1},
			ProofSamples:     benchProofSummary{Timeouts: 1, Failed: 1},
		},
	)

	assertions := evaluateBenchCompareAssertions(cmd, benchCompareConfig{
		MinCandidateThroughput:     90,
		MaxThroughputRegressionPct: 15,
		MaxDurationRegressionPct:   10,
		MaxSubmitP95RegressionPct:  25,
		MaxCandidateSubmitP95Ms:    18,
		MaxCandidateFailed:         1,
		MaxCandidateBatchErrors:    0,
		MaxCandidateQueryFailed:    0,
		MaxCandidateProofTimeouts:  0,
		MaxCandidateProofFailed:    0,
	}, result)
	if assertions == nil {
		t.Fatalf("assertions = nil")
	}
	if assertions.Passed {
		t.Fatalf("assertions should fail: %+v", assertions)
	}
	if assertions.FailedCount != len(assertions.Checks) {
		t.Fatalf("failed count = %d, want %d", assertions.FailedCount, len(assertions.Checks))
	}
	if err := benchCompareAssertionsError(assertions); err == nil || !strings.Contains(err.Error(), "assertions failed") {
		t.Fatalf("benchCompareAssertionsError() = %v, want failure", err)
	}
}

func TestBenchCompareCommandJSONWithAssertionsFailure(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	baselinePath := filepath.Join(tmp, "baseline.json")
	candidatePath := filepath.Join(tmp, "candidate.json")
	if err := writeJSONFile(baselinePath, benchIngestResult{
		SchemaVersion:    benchIngestReportSchema,
		Endpoint:         "http://baseline",
		Transport:        "http",
		Submitted:        10,
		Failed:           0,
		ThroughputPerSec: 100,
		SubmitLatency:    benchLatencySummary{P95Ms: 10},
		ProofSamples:     benchProofSummary{Timeouts: 0, Failed: 0},
	}); err != nil {
		t.Fatalf("write baseline report: %v", err)
	}
	if err := writeJSONFile(candidatePath, benchIngestResult{
		SchemaVersion:    benchIngestReportSchema,
		Endpoint:         "http://candidate",
		Transport:        "grpc",
		Submitted:        10,
		Failed:           2,
		ThroughputPerSec: 70,
		SubmitLatency:    benchLatencySummary{P95Ms: 20},
		QuerySamples:     benchQuerySummary{Failed: 1},
		ProofSamples:     benchProofSummary{Timeouts: 1, Failed: 1},
	}); err != nil {
		t.Fatalf("write candidate report: %v", err)
	}

	var out, errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut)
	cmd.SetArgs([]string{
		"bench", "compare",
		"--baseline", baselinePath,
		"--candidate", candidatePath,
		"--output", "json",
		"--max-throughput-regression-pct", "20",
		"--max-candidate-failed", "0",
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("bench compare execute error = nil, want assertion failure")
	}
	if !strings.Contains(err.Error(), "assertions failed") {
		t.Fatalf("bench compare error = %v", err)
	}

	var result benchCompareResult
	if jsonErr := json.Unmarshal(out.Bytes(), &result); jsonErr != nil {
		t.Fatalf("json.Unmarshal(compare output): %v body=%q", jsonErr, out.String())
	}
	if result.Assertions == nil || result.Assertions.Passed {
		t.Fatalf("assertions missing or unexpectedly passed: %+v", result.Assertions)
	}
	if result.Assertions.FailedCount != 2 {
		t.Fatalf("failed count = %d, want 2", result.Assertions.FailedCount)
	}
}
