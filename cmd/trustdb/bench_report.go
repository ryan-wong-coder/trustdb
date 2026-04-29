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
	benchCompareReportSchema = "trustdb.bench.compare.v2"
)

type benchCompareConfig struct {
	BaselinePath               string
	CandidatePath              string
	OutputFormat               string
	MinCandidateThroughput     float64
	MaxThroughputRegressionPct float64
	MaxDurationRegressionPct   float64
	MaxSubmitP95RegressionPct  float64
	MaxCandidateSubmitP95Ms    float64
	MaxCandidateFailed         int
	MaxCandidateBatchErrors    int
	MaxCandidateQueryFailed    int
	MaxCandidateProofTimeouts  int
	MaxCandidateProofFailed    int
}

type benchCompareResult struct {
	SchemaVersion string                  `json:"schema_version"`
	BaselinePath  string                  `json:"baseline_path"`
	CandidatePath string                  `json:"candidate_path"`
	Baseline      benchCompareMetadata    `json:"baseline"`
	Candidate     benchCompareMetadata    `json:"candidate"`
	Summary       benchCompareSummary     `json:"summary"`
	Metrics       []benchMetricComparison `json:"metrics,omitempty"`
	Assertions    *benchCompareAssertions `json:"assertions,omitempty"`
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

type benchCompareAssertions struct {
	Passed      bool                `json:"passed"`
	FailedCount int                 `json:"failed_count"`
	Checks      []benchCompareCheck `json:"checks"`
}

type benchCompareCheck struct {
	Name       string  `json:"name"`
	Passed     bool    `json:"passed"`
	Actual     float64 `json:"actual"`
	Limit      float64 `json:"limit"`
	Comparator string  `json:"comparator"`
	Unit       string  `json:"unit,omitempty"`
	Message    string  `json:"message,omitempty"`
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
			result.Assertions = evaluateBenchCompareAssertions(cmd, cfg, result)
			if cfg.OutputFormat == "text" {
				writeBenchCompareText(rt.out, result)
				return benchCompareAssertionsError(result.Assertions)
			}
			if err := rt.writeJSON(result); err != nil {
				return err
			}
			return benchCompareAssertionsError(result.Assertions)
		},
	}
	cmd.Flags().StringVar(&cfg.BaselinePath, "baseline", "", "baseline ingest bench JSON report path")
	cmd.Flags().StringVar(&cfg.CandidatePath, "candidate", "", "candidate ingest bench JSON report path")
	cmd.Flags().StringVar(&cfg.OutputFormat, "output", "text", "output format: text or json")
	cmd.Flags().Float64Var(&cfg.MinCandidateThroughput, "min-candidate-throughput", 0, "fail if candidate throughput_per_sec is lower than this value")
	cmd.Flags().Float64Var(&cfg.MaxThroughputRegressionPct, "max-throughput-regression-pct", 0, "fail if candidate throughput regresses more than this percentage versus baseline")
	cmd.Flags().Float64Var(&cfg.MaxDurationRegressionPct, "max-duration-regression-pct", 0, "fail if candidate duration_seconds increases more than this percentage versus baseline")
	cmd.Flags().Float64Var(&cfg.MaxSubmitP95RegressionPct, "max-submit-p95-regression-pct", 0, "fail if candidate submit_p95_ms increases more than this percentage versus baseline")
	cmd.Flags().Float64Var(&cfg.MaxCandidateSubmitP95Ms, "max-candidate-submit-p95-ms", 0, "fail if candidate submit_p95_ms exceeds this absolute value")
	cmd.Flags().IntVar(&cfg.MaxCandidateFailed, "max-candidate-failed", 0, "fail if candidate failed submissions exceeds this value")
	cmd.Flags().IntVar(&cfg.MaxCandidateBatchErrors, "max-candidate-batch-errors", 0, "fail if candidate batch_errors exceeds this value")
	cmd.Flags().IntVar(&cfg.MaxCandidateQueryFailed, "max-candidate-query-failed", 0, "fail if candidate query_failed exceeds this value")
	cmd.Flags().IntVar(&cfg.MaxCandidateProofTimeouts, "max-candidate-proof-timeouts", 0, "fail if candidate proof_timeouts exceeds this value")
	cmd.Flags().IntVar(&cfg.MaxCandidateProofFailed, "max-candidate-proof-failed", 0, "fail if candidate proof_failed exceeds this value")
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
	if result.Assertions != nil {
		fmt.Fprintln(w, "assertions:")
		fmt.Fprintf(w, "  passed: %t\n", result.Assertions.Passed)
		fmt.Fprintf(w, "  failed_count: %d\n", result.Assertions.FailedCount)
		for _, check := range result.Assertions.Checks {
			status := "PASS"
			if !check.Passed {
				status = "FAIL"
			}
			fmt.Fprintf(
				w,
				"  [%s] %s actual=%s limit=%s comparator=%s",
				status,
				check.Name,
				formatBenchNumber(check.Actual, 2),
				formatBenchNumber(check.Limit, 2),
				check.Comparator,
			)
			if check.Unit != "" {
				fmt.Fprintf(w, " unit=%s", check.Unit)
			}
			if check.Message != "" {
				fmt.Fprintf(w, " message=%q", check.Message)
			}
			fmt.Fprintln(w)
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

func evaluateBenchCompareAssertions(cmd *cobra.Command, cfg benchCompareConfig, result benchCompareResult) *benchCompareAssertions {
	var checks []benchCompareCheck
	addMax := func(flagName, name string, actual, limit float64, unit string) {
		if !cmd.Flags().Changed(flagName) {
			return
		}
		passed := actual <= limit
		checks = append(checks, benchCompareCheck{
			Name:       name,
			Passed:     passed,
			Actual:     actual,
			Limit:      limit,
			Comparator: "<=",
			Unit:       unit,
			Message:    benchCompareCheckMessage(name, passed, actual, limit, "<=", unit),
		})
	}
	addMin := func(flagName, name string, actual, limit float64, unit string) {
		if !cmd.Flags().Changed(flagName) {
			return
		}
		passed := actual >= limit
		checks = append(checks, benchCompareCheck{
			Name:       name,
			Passed:     passed,
			Actual:     actual,
			Limit:      limit,
			Comparator: ">=",
			Unit:       unit,
			Message:    benchCompareCheckMessage(name, passed, actual, limit, ">=", unit),
		})
	}

	addMin("min-candidate-throughput", "candidate_throughput_per_sec", result.Candidate.ThroughputPerSec, cfg.MinCandidateThroughput, "ops/s")
	if delta := result.Summary.ThroughputPerSec.DeltaPct; delta != nil {
		addMax("max-throughput-regression-pct", "throughput_regression_pct", math.Max(0, -*delta), cfg.MaxThroughputRegressionPct, "pct")
	} else if cmd.Flags().Changed("max-throughput-regression-pct") {
		addMax("max-throughput-regression-pct", "throughput_regression_pct", 0, cfg.MaxThroughputRegressionPct, "pct")
	}
	if delta := result.Summary.DurationSeconds.DeltaPct; delta != nil {
		addMax("max-duration-regression-pct", "duration_regression_pct", math.Max(0, *delta), cfg.MaxDurationRegressionPct, "pct")
	} else if cmd.Flags().Changed("max-duration-regression-pct") {
		addMax("max-duration-regression-pct", "duration_regression_pct", 0, cfg.MaxDurationRegressionPct, "pct")
	}
	if delta := result.Summary.SubmitP95Ms.DeltaPct; delta != nil {
		addMax("max-submit-p95-regression-pct", "submit_p95_regression_pct", math.Max(0, *delta), cfg.MaxSubmitP95RegressionPct, "pct")
	} else if cmd.Flags().Changed("max-submit-p95-regression-pct") {
		addMax("max-submit-p95-regression-pct", "submit_p95_regression_pct", 0, cfg.MaxSubmitP95RegressionPct, "pct")
	}
	addMax("max-candidate-submit-p95-ms", "candidate_submit_p95_ms", result.Summary.SubmitP95Ms.Candidate, cfg.MaxCandidateSubmitP95Ms, "ms")
	addMax("max-candidate-failed", "candidate_failed", float64(result.Candidate.Failed), float64(cfg.MaxCandidateFailed), "count")
	addMax("max-candidate-batch-errors", "candidate_batch_errors", float64(result.Candidate.BatchErrors), float64(cfg.MaxCandidateBatchErrors), "count")
	addMax("max-candidate-query-failed", "candidate_query_failed", result.Summary.QueryFailed.Candidate, float64(cfg.MaxCandidateQueryFailed), "count")
	addMax("max-candidate-proof-timeouts", "candidate_proof_timeouts", result.Summary.ProofTimeouts.Candidate, float64(cfg.MaxCandidateProofTimeouts), "count")
	addMax("max-candidate-proof-failed", "candidate_proof_failed", result.Summary.ProofFailed.Candidate, float64(cfg.MaxCandidateProofFailed), "count")

	if len(checks) == 0 {
		return nil
	}
	out := &benchCompareAssertions{
		Passed: true,
		Checks: checks,
	}
	for _, check := range checks {
		if check.Passed {
			continue
		}
		out.Passed = false
		out.FailedCount++
	}
	return out
}

func benchCompareCheckMessage(name string, passed bool, actual, limit float64, comparator, unit string) string {
	if passed {
		return ""
	}
	suffix := ""
	if unit != "" {
		suffix = " " + unit
	}
	return fmt.Sprintf("%s %.2f%s is not %s %.2f%s", name, actual, suffix, comparator, limit, suffix)
}

func benchCompareAssertionsError(assertions *benchCompareAssertions) error {
	if assertions == nil || assertions.Passed {
		return nil
	}
	failed := make([]string, 0, assertions.FailedCount)
	for _, check := range assertions.Checks {
		if check.Passed {
			continue
		}
		failed = append(failed, check.Name)
	}
	return fmt.Errorf("bench compare assertions failed: %s", strings.Join(failed, ", "))
}
