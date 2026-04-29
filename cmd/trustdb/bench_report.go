package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

const (
	benchIngestReportSchema  = "trustdb.bench.ingest.v1"
	benchCompareReportSchema = "trustdb.bench.compare.v1"
)

type benchCompareConfig struct {
	BaselinePath  string
	CandidatePath string
	OutputFormat  string
}

type benchCompareResult struct {
	SchemaVersion string                  `json:"schema_version"`
	BaselinePath  string                  `json:"baseline_path"`
	CandidatePath string                  `json:"candidate_path"`
	Baseline      benchCompareMetadata    `json:"baseline"`
	Candidate     benchCompareMetadata    `json:"candidate"`
	Summary       benchCompareSummary     `json:"summary"`
	Metrics       []benchMetricComparison `json:"metrics,omitempty"`
}

type benchCompareMetadata struct {
	SchemaVersion    string  `json:"schema_version"`
	Endpoint         string  `json:"endpoint"`
	Transport        string  `json:"transport"`
	Count            int     `json:"count"`
	Concurrency      int     `json:"concurrency"`
	PayloadBytes     int     `json:"payload_bytes"`
	Submitted        int     `json:"submitted"`
	Failed           int     `json:"failed"`
	BatchErrors      int     `json:"batch_errors"`
	DurationSeconds  float64 `json:"duration_seconds"`
	ThroughputPerSec float64 `json:"throughput_per_sec"`
}

type benchCompareSummary struct {
	Submitted        benchNumberComparison `json:"submitted"`
	Failed           benchNumberComparison `json:"failed"`
	BatchErrors      benchNumberComparison `json:"batch_errors"`
	DurationSeconds  benchNumberComparison `json:"duration_seconds"`
	ThroughputPerSec benchNumberComparison `json:"throughput_per_sec"`
	SubmitAvgMs      benchNumberComparison `json:"submit_avg_ms"`
	SubmitP95Ms      benchNumberComparison `json:"submit_p95_ms"`
	SubmitP99Ms      benchNumberComparison `json:"submit_p99_ms"`
	QueryReady       benchNumberComparison `json:"query_ready"`
	QueryFailed      benchNumberComparison `json:"query_failed"`
	ProofReady       benchNumberComparison `json:"proof_ready"`
	ProofTimeouts    benchNumberComparison `json:"proof_timeouts"`
	ProofFailed      benchNumberComparison `json:"proof_failed"`
}

type benchNumberComparison struct {
	Baseline  float64  `json:"baseline"`
	Candidate float64  `json:"candidate"`
	Delta     float64  `json:"delta"`
	DeltaPct  *float64 `json:"delta_pct,omitempty"`
}

type benchMetricComparison struct {
	Name           string   `json:"name"`
	BaselineDelta  float64  `json:"baseline_delta"`
	CandidateDelta float64  `json:"candidate_delta"`
	Delta          float64  `json:"delta"`
	DeltaPct       *float64 `json:"delta_pct,omitempty"`
}

func newBenchCompareCommand(rt *runtimeConfig) *cobra.Command {
	var cfg benchCompareConfig
	cmd := &cobra.Command{
		Use:   "compare",
		Short: "Compare two persisted ingest benchmark reports",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.BaselinePath = strings.TrimSpace(cfg.BaselinePath)
			cfg.CandidatePath = strings.TrimSpace(cfg.CandidatePath)
			cfg.OutputFormat = strings.ToLower(strings.TrimSpace(cfg.OutputFormat))
			if cfg.BaselinePath == "" {
				return usageError("bench compare requires --baseline")
			}
			if cfg.CandidatePath == "" {
				return usageError("bench compare requires --candidate")
			}
			if cfg.OutputFormat == "" {
				cfg.OutputFormat = "text"
			}
			if cfg.OutputFormat != "text" && cfg.OutputFormat != "json" {
				return usageError("bench compare --output must be json or text")
			}

			baseline, err := readBenchIngestReportFile(cfg.BaselinePath)
			if err != nil {
				return err
			}
			candidate, err := readBenchIngestReportFile(cfg.CandidatePath)
			if err != nil {
				return err
			}

			result := compareBenchIngestResults(cfg.BaselinePath, cfg.CandidatePath, baseline, candidate)
			if cfg.OutputFormat == "text" {
				writeBenchCompareText(rt.out, result)
				return nil
			}
			return rt.writeJSON(result)
		},
	}
	cmd.Flags().StringVar(&cfg.BaselinePath, "baseline", "", "baseline ingest bench JSON report path")
	cmd.Flags().StringVar(&cfg.CandidatePath, "candidate", "", "candidate ingest bench JSON report path")
	cmd.Flags().StringVar(&cfg.OutputFormat, "output", "text", "output format: text or json")
	return cmd
}

func emitBenchIngestResult(rt *runtimeConfig, cfg benchIngestConfig, result benchIngestResult) error {
	if result.SchemaVersion == "" {
		result.SchemaVersion = benchIngestReportSchema
	}
	if cfg.ReportFile != "" {
		if err := writeJSONFile(cfg.ReportFile, result); err != nil {
			return err
		}
	}
	if cfg.OutputFormat == "text" {
		writeBenchIngestText(rt.out, result)
		if cfg.ReportFile != "" {
			fmt.Fprintf(rt.out, "report_file: %s\n", cfg.ReportFile)
		}
		return nil
	}
	return rt.writeJSON(result)
}

func readBenchIngestReportFile(path string) (benchIngestResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return benchIngestResult{}, err
	}
	var result benchIngestResult
	if err := json.Unmarshal(data, &result); err != nil {
		return benchIngestResult{}, err
	}
	if result.SchemaVersion == "" {
		result.SchemaVersion = benchIngestReportSchema
	}
	return result, nil
}

func compareBenchIngestResults(baselinePath, candidatePath string, baseline, candidate benchIngestResult) benchCompareResult {
	if baseline.SchemaVersion == "" {
		baseline.SchemaVersion = benchIngestReportSchema
	}
	if candidate.SchemaVersion == "" {
		candidate.SchemaVersion = benchIngestReportSchema
	}
	return benchCompareResult{
		SchemaVersion: benchCompareReportSchema,
		BaselinePath:  baselinePath,
		CandidatePath: candidatePath,
		Baseline:      benchCompareMetadataFromResult(baseline),
		Candidate:     benchCompareMetadataFromResult(candidate),
		Summary: benchCompareSummary{
			Submitted:        benchNumberDelta(float64(baseline.Submitted), float64(candidate.Submitted)),
			Failed:           benchNumberDelta(float64(baseline.Failed), float64(candidate.Failed)),
			BatchErrors:      benchNumberDelta(float64(baseline.BatchErrors), float64(candidate.BatchErrors)),
			DurationSeconds:  benchNumberDelta(baseline.DurationSeconds, candidate.DurationSeconds),
			ThroughputPerSec: benchNumberDelta(baseline.ThroughputPerSec, candidate.ThroughputPerSec),
			SubmitAvgMs:      benchNumberDelta(baseline.SubmitLatency.AvgMs, candidate.SubmitLatency.AvgMs),
			SubmitP95Ms:      benchNumberDelta(baseline.SubmitLatency.P95Ms, candidate.SubmitLatency.P95Ms),
			SubmitP99Ms:      benchNumberDelta(baseline.SubmitLatency.P99Ms, candidate.SubmitLatency.P99Ms),
			QueryReady:       benchNumberDelta(float64(baseline.QuerySamples.Ready), float64(candidate.QuerySamples.Ready)),
			QueryFailed:      benchNumberDelta(float64(baseline.QuerySamples.Failed), float64(candidate.QuerySamples.Failed)),
			ProofReady:       benchNumberDelta(float64(baseline.ProofSamples.Ready), float64(candidate.ProofSamples.Ready)),
			ProofTimeouts:    benchNumberDelta(float64(baseline.ProofSamples.Timeouts), float64(candidate.ProofSamples.Timeouts)),
			ProofFailed:      benchNumberDelta(float64(baseline.ProofSamples.Failed), float64(candidate.ProofSamples.Failed)),
		},
		Metrics: compareBenchMetricDeltas(baseline.Metrics, candidate.Metrics),
	}
}

func benchCompareMetadataFromResult(result benchIngestResult) benchCompareMetadata {
	return benchCompareMetadata{
		SchemaVersion:    result.SchemaVersion,
		Endpoint:         result.Endpoint,
		Transport:        result.Transport,
		Count:            result.Count,
		Concurrency:      result.Concurrency,
		PayloadBytes:     result.PayloadBytes,
		Submitted:        result.Submitted,
		Failed:           result.Failed,
		BatchErrors:      result.BatchErrors,
		DurationSeconds:  result.DurationSeconds,
		ThroughputPerSec: result.ThroughputPerSec,
	}
}

func compareBenchMetricDeltas(baseline, candidate []benchMetricDelta) []benchMetricComparison {
	baselineMap := make(map[string]float64, len(baseline))
	for _, metric := range baseline {
		baselineMap[metric.Name] = metric.Delta
	}
	candidateMap := make(map[string]float64, len(candidate))
	for _, metric := range candidate {
		candidateMap[metric.Name] = metric.Delta
	}
	keys := make([]string, 0, len(baselineMap)+len(candidateMap))
	seen := make(map[string]struct{}, len(baselineMap)+len(candidateMap))
	for name := range baselineMap {
		keys = append(keys, name)
		seen[name] = struct{}{}
	}
	for name := range candidateMap {
		if _, ok := seen[name]; ok {
			continue
		}
		keys = append(keys, name)
	}
	sort.Strings(keys)
	out := make([]benchMetricComparison, 0, len(keys))
	for _, name := range keys {
		b := baselineMap[name]
		c := candidateMap[name]
		if b == 0 && c == 0 {
			continue
		}
		out = append(out, benchMetricComparison{
			Name:           name,
			BaselineDelta:  b,
			CandidateDelta: c,
			Delta:          c - b,
			DeltaPct:       benchDeltaPct(c-b, b),
		})
	}
	return out
}

func benchNumberDelta(baseline, candidate float64) benchNumberComparison {
	return benchNumberComparison{
		Baseline:  baseline,
		Candidate: candidate,
		Delta:     candidate - baseline,
		DeltaPct:  benchDeltaPct(candidate-baseline, baseline),
	}
}

func benchDeltaPct(delta, baseline float64) *float64 {
	if baseline == 0 {
		return nil
	}
	value := (delta / baseline) * 100
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}
	return &value
}

func writeBenchCompareText(w io.Writer, result benchCompareResult) {
	fmt.Fprintf(w, "baseline: %s\n", result.BaselinePath)
	fmt.Fprintf(w, "candidate: %s\n", result.CandidatePath)
	fmt.Fprintf(w, "baseline_endpoint: %s\n", result.Baseline.Endpoint)
	fmt.Fprintf(w, "candidate_endpoint: %s\n", result.Candidate.Endpoint)
	fmt.Fprintln(w, "summary:")
	writeBenchComparisonLine(w, "submitted", result.Summary.Submitted, 0)
	writeBenchComparisonLine(w, "failed", result.Summary.Failed, 0)
	writeBenchComparisonLine(w, "batch_errors", result.Summary.BatchErrors, 0)
	writeBenchComparisonLine(w, "duration_seconds", result.Summary.DurationSeconds, 3)
	writeBenchComparisonLine(w, "throughput_per_sec", result.Summary.ThroughputPerSec, 2)
	writeBenchComparisonLine(w, "submit_avg_ms", result.Summary.SubmitAvgMs, 2)
	writeBenchComparisonLine(w, "submit_p95_ms", result.Summary.SubmitP95Ms, 2)
	writeBenchComparisonLine(w, "submit_p99_ms", result.Summary.SubmitP99Ms, 2)
	writeBenchComparisonLine(w, "query_ready", result.Summary.QueryReady, 0)
	writeBenchComparisonLine(w, "query_failed", result.Summary.QueryFailed, 0)
	writeBenchComparisonLine(w, "proof_ready", result.Summary.ProofReady, 0)
	writeBenchComparisonLine(w, "proof_timeouts", result.Summary.ProofTimeouts, 0)
	writeBenchComparisonLine(w, "proof_failed", result.Summary.ProofFailed, 0)
	if len(result.Metrics) > 0 {
		fmt.Fprintln(w, "metrics:")
		for _, metric := range result.Metrics {
			fmt.Fprintf(
				w,
				"  %s baseline_delta=%s candidate_delta=%s delta=%s %s\n",
				metric.Name,
				formatBenchNumber(metric.BaselineDelta, 6),
				formatBenchNumber(metric.CandidateDelta, 6),
				formatSignedBenchNumber(metric.Delta, 6),
				formatBenchPct(metric.DeltaPct),
			)
		}
	}
}

func writeBenchComparisonLine(w io.Writer, name string, cmp benchNumberComparison, decimals int) {
	fmt.Fprintf(
		w,
		"  %s: baseline=%s candidate=%s delta=%s %s\n",
		name,
		formatBenchNumber(cmp.Baseline, decimals),
		formatBenchNumber(cmp.Candidate, decimals),
		formatSignedBenchNumber(cmp.Delta, decimals),
		formatBenchPct(cmp.DeltaPct),
	)
}

func formatBenchNumber(value float64, decimals int) string {
	return fmt.Sprintf("%.*f", decimals, value)
}

func formatSignedBenchNumber(value float64, decimals int) string {
	return fmt.Sprintf("%+.*f", decimals, value)
}

func formatBenchPct(value *float64) string {
	if value == nil {
		return "(n/a)"
	}
	return fmt.Sprintf("(%+.2f%%)", *value)
}
