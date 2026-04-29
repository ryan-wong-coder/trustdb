package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	prommodel "github.com/prometheus/common/model"
	"github.com/ryan-wong-coder/trustdb/internal/trusterr"
	"github.com/ryan-wong-coder/trustdb/sdk"
	"github.com/spf13/cobra"
)

func newBenchCommand(rt *runtimeConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bench",
		Short: "Benchmark ingest and proof/query paths against a TrustDB server",
	}
	cmd.AddCommand(newBenchIngestCommand(rt))
	return cmd
}

func newBenchIngestCommand(rt *runtimeConfig) *cobra.Command {
	var cfg benchIngestConfig
	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Generate synthetic claims and measure ingest, proof, and query performance",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Endpoint = strings.TrimSpace(cfg.Endpoint)
			cfg.Transport = strings.ToLower(strings.TrimSpace(cfg.Transport))
			cfg.PrivateKeyPath = stringOrConfig(cmd, rt, "private-key", cfg.PrivateKeyPath, "keys.client_private")
			cfg.Identity = sdk.Identity{
				TenantID: stringValue(cmd, rt, "tenant", "tenant"),
				ClientID: stringValue(cmd, rt, "client", "client"),
				KeyID:    stringValue(cmd, rt, "key-id", "key_id"),
			}
			if cfg.Endpoint == "" {
				return usageError("bench ingest requires --server")
			}
			if cfg.Transport == "" {
				cfg.Transport = "http"
			}
			if cfg.Transport != "http" && cfg.Transport != "grpc" {
				return usageError("bench ingest --transport must be http or grpc")
			}
			if cfg.PrivateKeyPath == "" {
				return usageError("bench ingest requires --private-key")
			}
			if cfg.Identity.ClientID == "" || cfg.Identity.KeyID == "" {
				return usageError("bench ingest requires --client and --key-id")
			}
			if cfg.Count <= 0 {
				return usageError("bench ingest --count must be > 0")
			}
			if cfg.Concurrency <= 0 {
				return usageError("bench ingest --concurrency must be > 0")
			}
			if cfg.PayloadBytes <= 0 {
				return usageError("bench ingest --payload-bytes must be > 0")
			}
			if cfg.Samples < 0 {
				return usageError("bench ingest --samples must be >= 0")
			}
			if cfg.ProofLevel == "" {
				cfg.ProofLevel = sdk.ProofLevelL4
			}
			switch cfg.ProofLevel {
			case sdk.ProofLevelL3, sdk.ProofLevelL4, sdk.ProofLevelL5:
			default:
				return usageError("bench ingest --proof-level must be L3, L4, or L5")
			}
			if cfg.ProofTimeout <= 0 {
				return usageError("bench ingest --proof-timeout must be > 0")
			}
			if cfg.ProgressEvery < 0 {
				return usageError("bench ingest --progress-every must be >= 0")
			}
			if cfg.EventType == "" {
				cfg.EventType = "bench.synthetic"
			}
			if cfg.Source == "" {
				cfg.Source = "trustdb-bench"
			}
			if cfg.OutputFormat == "" {
				cfg.OutputFormat = "json"
			}
			if cfg.OutputFormat != "json" && cfg.OutputFormat != "text" {
				return usageError("bench ingest --output must be json or text")
			}

			priv, err := readPrivateKey(cfg.PrivateKeyPath)
			if err != nil {
				return err
			}
			cfg.Identity.PrivateKey = priv

			client, err := newBenchSDKClient(cfg.Transport, cfg.Endpoint)
			if err != nil {
				return err
			}
			defer client.Close()

			ctx := cmd.Context()
			result, err := runBenchIngest(ctx, rt, client, cfg)
			if err != nil {
				return err
			}
			if cfg.OutputFormat == "text" {
				writeBenchIngestText(rt.out, result)
				return nil
			}
			return rt.writeJSON(result)
		},
	}
	cmd.Flags().StringVar(&cfg.Endpoint, "server", "", "TrustDB server HTTP base URL or gRPC target")
	cmd.Flags().StringVar(&cfg.Transport, "transport", "http", "transport: http or grpc")
	addCommonIdentityFlags(cmd)
	cmd.Flags().StringVar(&cfg.PrivateKeyPath, "private-key", "", "client private key")
	cmd.Flags().IntVar(&cfg.Count, "count", 1000, "number of synthetic claims to submit")
	cmd.Flags().IntVar(&cfg.Concurrency, "concurrency", 16, "number of concurrent submit workers")
	cmd.Flags().IntVar(&cfg.PayloadBytes, "payload-bytes", 1024, "payload size in bytes per synthetic claim")
	cmd.Flags().IntVar(&cfg.ProgressEvery, "progress-every", 1000, "log progress every N completed submits; 0 disables progress logs")
	cmd.Flags().IntVar(&cfg.Samples, "samples", 8, "number of successful records to sample for record/proof queries after ingest")
	cmd.Flags().StringVar(&cfg.ProofLevel, "proof-level", sdk.ProofLevelL4, "target proof level to wait for in samples: L3, L4, or L5")
	cmd.Flags().DurationVar(&cfg.ProofTimeout, "proof-timeout", 45*time.Second, "maximum wait per sampled record for target proof level")
	cmd.Flags().DurationVar(&cfg.Settle, "settle", 3*time.Second, "extra settle time before final metric snapshot")
	cmd.Flags().StringVar(&cfg.EventType, "event-type", "bench.synthetic", "metadata.event_type for synthetic claims")
	cmd.Flags().StringVar(&cfg.Source, "source", "trustdb-bench", "metadata.source for synthetic claims")
	cmd.Flags().StringVar(&cfg.OutputFormat, "output", "json", "output format: json or text")
	return cmd
}

type benchIngestConfig struct {
	Endpoint       string
	Transport      string
	PrivateKeyPath string
	Identity       sdk.Identity
	Count          int
	Concurrency    int
	PayloadBytes   int
	ProgressEvery  int
	Samples        int
	ProofLevel     string
	ProofTimeout   time.Duration
	Settle         time.Duration
	EventType      string
	Source         string
	OutputFormat   string
}

type benchIngestResult struct {
	Endpoint         string              `json:"endpoint"`
	Transport        string              `json:"transport"`
	Count            int                 `json:"count"`
	Concurrency      int                 `json:"concurrency"`
	PayloadBytes     int                 `json:"payload_bytes"`
	StartedAt        time.Time           `json:"started_at"`
	FinishedAt       time.Time           `json:"finished_at"`
	DurationSeconds  float64             `json:"duration_seconds"`
	Submitted        int                 `json:"submitted"`
	Failed           int                 `json:"failed"`
	BatchErrors      int                 `json:"batch_errors"`
	ThroughputPerSec float64             `json:"throughput_per_sec"`
	SubmitLatency    benchLatencySummary `json:"submit_latency"`
	QuerySamples     benchQuerySummary   `json:"query_samples"`
	ProofSamples     benchProofSummary   `json:"proof_samples"`
	Metrics          []benchMetricDelta  `json:"metrics"`
	ErrorSamples     []string            `json:"error_samples,omitempty"`
	Records          []benchRecordSample `json:"records,omitempty"`
}

type benchLatencySummary struct {
	Count int64   `json:"count"`
	AvgMs float64 `json:"avg_ms"`
	MinMs float64 `json:"min_ms"`
	P50Ms float64 `json:"p50_ms"`
	P95Ms float64 `json:"p95_ms"`
	P99Ms float64 `json:"p99_ms"`
	MaxMs float64 `json:"max_ms"`
}

type benchQuerySummary struct {
	Samples int                 `json:"samples"`
	Ready   int                 `json:"ready"`
	Failed  int                 `json:"failed"`
	Latency benchLatencySummary `json:"latency"`
}

type benchProofSummary struct {
	Samples     int                 `json:"samples"`
	TargetLevel string              `json:"target_level"`
	Ready       int                 `json:"ready"`
	Timeouts    int                 `json:"timeouts"`
	Failed      int                 `json:"failed"`
	Latency     benchLatencySummary `json:"latency"`
}

type benchMetricDelta struct {
	Name   string  `json:"name"`
	Before float64 `json:"before"`
	After  float64 `json:"after"`
	Delta  float64 `json:"delta"`
}

type benchRecordSample struct {
	RecordID string `json:"record_id"`
	BatchID  string `json:"batch_id,omitempty"`
}

type benchSubmitOutcome struct {
	RecordID   string
	Latency    time.Duration
	Err        error
	BatchError string
}

func runBenchIngest(ctx context.Context, rt *runtimeConfig, client *sdk.Client, cfg benchIngestConfig) (benchIngestResult, error) {
	if status := client.CheckHealth(ctx); !status.OK {
		return benchIngestResult{}, &sdk.Error{Op: "bench health", URL: cfg.Endpoint, StatusCode: status.StatusCode, Message: status.Error}
	}

	beforeMetrics, err := fetchBenchMetrics(ctx, client)
	if err != nil {
		rt.logger.Warn().Err(err).Msg("bench could not snapshot initial metrics")
	}

	started := time.Now().UTC()
	submitStats := newBenchLatencyHistogram()
	recordStats := newBenchLatencyHistogram()
	proofStats := newBenchLatencyHistogram()
	jobCh := make(chan int)
	outcomeCh := make(chan benchSubmitOutcome, cfg.Concurrency)
	sampleMu := sync.Mutex{}
	samples := make([]benchRecordSample, 0, cfg.Samples)
	errorSamples := make([]string, 0, 5)
	var completed atomic.Int64
	var wg sync.WaitGroup
	payloadPool := sync.Pool{
		New: func() any {
			return make([]byte, cfg.PayloadBytes)
		},
	}
	runID := strconv.FormatInt(time.Now().UTC().UnixNano(), 36)

	for worker := 0; worker < cfg.Concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for seq := range jobCh {
				if ctx.Err() != nil {
					return
				}
				start := time.Now()
				buf := payloadPool.Get().([]byte)
				if cap(buf) < cfg.PayloadBytes {
					buf = make([]byte, cfg.PayloadBytes)
				}
				buf = buf[:cfg.PayloadBytes]
				fillBenchPayload(buf, seq)
				result, err := client.SubmitFile(ctx, bytes.NewReader(buf), cfg.Identity, sdk.FileClaimOptions{
					ProducedAt:     time.Now().UTC(),
					Nonce:          benchNonce(seq),
					IdempotencyKey: fmt.Sprintf("bench-%s-%d", runID, seq),
					MediaType:      "application/octet-stream",
					StorageURI:     fmt.Sprintf("bench://%s/%d.bin", runID, seq),
					EventType:      cfg.EventType,
					Source:         cfg.Source,
					CustomMetadata: map[string]string{"bench_seq": strconv.Itoa(seq)},
				})
				payloadPool.Put(buf)
				outcome := benchSubmitOutcome{Latency: time.Since(start), Err: err}
				if err == nil {
					outcome.RecordID = result.RecordID
					outcome.BatchError = strings.TrimSpace(result.BatchError)
				}
				select {
				case outcomeCh <- outcome:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	go func() {
		for i := 0; i < cfg.Count; i++ {
			select {
			case jobCh <- i:
			case <-ctx.Done():
				close(jobCh)
				return
			}
		}
		close(jobCh)
	}()
	go func() {
		wg.Wait()
		close(outcomeCh)
	}()

	submitted := 0
	failed := 0
	batchErrors := 0
	for outcome := range outcomeCh {
		completedNow := completed.Add(1)
		if outcome.Err != nil {
			failed++
			if len(errorSamples) < cap(errorSamples) {
				errorSamples = append(errorSamples, outcome.Err.Error())
			}
		} else {
			submitted++
			submitStats.Observe(outcome.Latency)
			if outcome.BatchError != "" {
				batchErrors++
				if len(errorSamples) < cap(errorSamples) {
					errorSamples = append(errorSamples, outcome.BatchError)
				}
			}
			sampleMu.Lock()
			if len(samples) < cfg.Samples {
				samples = append(samples, benchRecordSample{RecordID: outcome.RecordID})
			}
			sampleMu.Unlock()
		}
		if cfg.ProgressEvery > 0 && completedNow%int64(cfg.ProgressEvery) == 0 {
			rt.logger.Info().
				Int64("completed", completedNow).
				Int("submitted", submitted).
				Int("failed", failed).
				Msg("bench ingest progress")
		}
	}

	queryReady := 0
	queryFailed := 0
	proofReady := 0
	proofFailed := 0
	proofTimeouts := 0
	for i := range samples {
		recordStart := time.Now()
		record, err := client.GetRecord(ctx, samples[i].RecordID)
		if err != nil {
			queryFailed++
			if len(errorSamples) < cap(errorSamples) {
				errorSamples = append(errorSamples, fmt.Sprintf("get record %s: %v", samples[i].RecordID, err))
			}
		} else {
			queryReady++
			recordStats.Observe(time.Since(recordStart))
			samples[i].BatchID = record.BatchID
		}

		if cfg.ProofLevel == "" {
			continue
		}
		proofStart := time.Now()
		waitErr := waitForBenchProofLevel(ctx, client, samples[i].RecordID, cfg.ProofLevel, cfg.ProofTimeout)
		switch {
		case waitErr == nil:
			proofReady++
			proofStats.Observe(time.Since(proofStart))
		case errors.Is(waitErr, errBenchProofTimeout):
			proofTimeouts++
			if len(errorSamples) < cap(errorSamples) {
				errorSamples = append(errorSamples, fmt.Sprintf("proof timeout %s", samples[i].RecordID))
			}
		default:
			proofFailed++
			if len(errorSamples) < cap(errorSamples) {
				errorSamples = append(errorSamples, fmt.Sprintf("proof wait %s: %v", samples[i].RecordID, waitErr))
			}
		}
	}

	if cfg.Settle > 0 {
		select {
		case <-time.After(cfg.Settle):
		case <-ctx.Done():
			return benchIngestResult{}, trusterr.Wrap(trusterr.CodeDeadlineExceeded, "bench ingest canceled during settle", ctx.Err())
		}
	}

	afterMetrics, err := fetchBenchMetrics(ctx, client)
	if err != nil {
		rt.logger.Warn().Err(err).Msg("bench could not snapshot final metrics")
	}

	finished := time.Now().UTC()
	duration := finished.Sub(started)
	result := benchIngestResult{
		Endpoint:        cfg.Endpoint,
		Transport:       cfg.Transport,
		Count:           cfg.Count,
		Concurrency:     cfg.Concurrency,
		PayloadBytes:    cfg.PayloadBytes,
		StartedAt:       started,
		FinishedAt:      finished,
		DurationSeconds: duration.Seconds(),
		Submitted:       submitted,
		Failed:          failed,
		BatchErrors:     batchErrors,
		SubmitLatency:   submitStats.Summary(),
		QuerySamples:    benchQuerySummary{Samples: len(samples), Ready: queryReady, Failed: queryFailed, Latency: recordStats.Summary()},
		ProofSamples:    benchProofSummary{Samples: len(samples), TargetLevel: cfg.ProofLevel, Ready: proofReady, Timeouts: proofTimeouts, Failed: proofFailed, Latency: proofStats.Summary()},
		Metrics:         diffBenchMetrics(beforeMetrics, afterMetrics),
		ErrorSamples:    errorSamples,
		Records:         samples,
	}
	if duration > 0 {
		result.ThroughputPerSec = float64(submitted) / duration.Seconds()
	}
	return result, nil
}

func newBenchSDKClient(transport, endpoint string) (*sdk.Client, error) {
	switch strings.ToLower(strings.TrimSpace(transport)) {
	case "http":
		return sdk.NewClient(endpoint)
	case "grpc":
		return sdk.NewGRPCClient(endpoint)
	default:
		return nil, usageError("bench ingest --transport must be http or grpc")
	}
}

var errBenchProofTimeout = errors.New("bench proof timeout")

func waitForBenchProofLevel(ctx context.Context, client *sdk.Client, recordID, level string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "bench proof wait canceled", ctx.Err())
		}
		proof, err := client.ExportSingleProof(ctx, recordID)
		if err == nil && benchProofReachedLevel(proof, level) {
			return nil
		}
		select {
		case <-time.After(50 * time.Millisecond):
		case <-ctx.Done():
			return trusterr.Wrap(trusterr.CodeDeadlineExceeded, "bench proof wait canceled", ctx.Err())
		}
	}
	return errBenchProofTimeout
}

func benchProofReachedLevel(proof sdk.SingleProof, level string) bool {
	switch level {
	case sdk.ProofLevelL3:
		return proof.ProofBundle.RecordID != ""
	case sdk.ProofLevelL4:
		return proof.ProofBundle.RecordID != "" && proof.GlobalProof != nil
	case sdk.ProofLevelL5:
		return proof.ProofBundle.RecordID != "" && proof.GlobalProof != nil && proof.AnchorResult != nil
	default:
		return false
	}
}

func fillBenchPayload(buf []byte, seq int) {
	seed := uint64(seq+1) * 0x9e3779b97f4a7c15
	for i := range buf {
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		buf[i] = byte(seed >> 56)
	}
}

func benchNonce(seq int) []byte {
	nonce := make([]byte, 16)
	binary.BigEndian.PutUint64(nonce[:8], uint64(seq+1))
	binary.BigEndian.PutUint64(nonce[8:], ^uint64(seq+1))
	return nonce
}

var benchMetricPrefixes = []string{
	"trustdb_ingest_requests_total",
	"trustdb_ingest_rejected_total",
	"trustdb_wal_append_latency_seconds",
	"trustdb_wal_fsync_latency_seconds",
	"trustdb_batch_commit_latency_seconds",
	"trustdb_batch_size_records",
	"trustdb_merkle_build_latency_seconds",
	"trustdb_anchor_published_total",
	"trustdb_anchor_attempts_total",
	"trustdb_anchor_pending_total",
	"trustdb_queue_depth",
	"trustdb_wal_checkpoint_last_sequence",
	"trustdb_wal_replay_records_total",
	"trustdb_wal_active_segment_id",
	"trustdb_wal_segments_total",
	"trustdb_wal_bytes_pruned_total",
	"trustdb_pebble_",
	"go_goroutines",
	"go_memstats_alloc_bytes",
	"go_memstats_heap_alloc_bytes",
	"go_memstats_heap_inuse_bytes",
}

func fetchBenchMetrics(ctx context.Context, client *sdk.Client) (map[string]float64, error) {
	raw, err := client.MetricsRaw(ctx)
	if err != nil {
		return nil, err
	}
	return parseBenchMetrics(raw)
}

func parseBenchMetrics(raw string) (map[string]float64, error) {
	parser := expfmt.NewTextParser(prommodel.UTF8Validation)
	families, err := parser.TextToMetricFamilies(strings.NewReader(raw))
	if err != nil {
		return nil, err
	}
	out := make(map[string]float64)
	for name, family := range families {
		if !benchMetricWanted(name) {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := benchMetricLabels(metric.GetLabel())
			switch family.GetType() {
			case dto.MetricType_COUNTER:
				out[benchMetricKey(name, labels)] = metric.GetCounter().GetValue()
			case dto.MetricType_GAUGE:
				out[benchMetricKey(name, labels)] = metric.GetGauge().GetValue()
			case dto.MetricType_UNTYPED:
				out[benchMetricKey(name, labels)] = metric.GetUntyped().GetValue()
			case dto.MetricType_HISTOGRAM:
				h := metric.GetHistogram()
				out[benchMetricKey(name+"_count", labels)] = float64(h.GetSampleCount())
				out[benchMetricKey(name+"_sum", labels)] = h.GetSampleSum()
			case dto.MetricType_SUMMARY:
				s := metric.GetSummary()
				out[benchMetricKey(name+"_count", labels)] = float64(s.GetSampleCount())
				out[benchMetricKey(name+"_sum", labels)] = s.GetSampleSum()
			}
		}
	}
	return out, nil
}

func benchMetricWanted(name string) bool {
	for _, prefix := range benchMetricPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func benchMetricLabels(labels []*dto.LabelPair) string {
	if len(labels) == 0 {
		return ""
	}
	parts := make([]string, 0, len(labels))
	for _, label := range labels {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, label.GetName(), label.GetValue()))
	}
	sort.Strings(parts)
	return "{" + strings.Join(parts, ",") + "}"
}

func benchMetricKey(name, labels string) string {
	return name + labels
}

func diffBenchMetrics(before, after map[string]float64) []benchMetricDelta {
	if len(before) == 0 && len(after) == 0 {
		return nil
	}
	keys := make([]string, 0, len(before)+len(after))
	seen := make(map[string]struct{}, len(before)+len(after))
	for k := range before {
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	for k := range after {
		if _, ok := seen[k]; ok {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]benchMetricDelta, 0, len(keys))
	for _, key := range keys {
		b := before[key]
		a := after[key]
		out = append(out, benchMetricDelta{Name: key, Before: b, After: a, Delta: a - b})
	}
	return out
}

type benchLatencyHistogram struct {
	count     int64
	sum       time.Duration
	min       time.Duration
	max       time.Duration
	bounds    []time.Duration
	bucketHit []int64
}

func newBenchLatencyHistogram() *benchLatencyHistogram {
	bounds := []time.Duration{
		100 * time.Microsecond,
		250 * time.Microsecond,
		500 * time.Microsecond,
		1 * time.Millisecond,
		2 * time.Millisecond,
		5 * time.Millisecond,
		10 * time.Millisecond,
		20 * time.Millisecond,
		50 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
		500 * time.Millisecond,
		1 * time.Second,
		2 * time.Second,
		5 * time.Second,
		10 * time.Second,
		30 * time.Second,
		60 * time.Second,
	}
	return &benchLatencyHistogram{
		min:       time.Duration(math.MaxInt64),
		bounds:    bounds,
		bucketHit: make([]int64, len(bounds)+1),
	}
}

func (h *benchLatencyHistogram) Observe(d time.Duration) {
	if d < 0 {
		d = 0
	}
	h.count++
	h.sum += d
	if d < h.min {
		h.min = d
	}
	if d > h.max {
		h.max = d
	}
	for i, bound := range h.bounds {
		if d <= bound {
			h.bucketHit[i]++
			return
		}
	}
	h.bucketHit[len(h.bucketHit)-1]++
}

func (h *benchLatencyHistogram) Summary() benchLatencySummary {
	if h.count == 0 {
		return benchLatencySummary{}
	}
	return benchLatencySummary{
		Count: h.count,
		AvgMs: durationMillis(time.Duration(int64(h.sum) / h.count)),
		MinMs: durationMillis(h.min),
		P50Ms: durationMillis(h.quantile(0.50)),
		P95Ms: durationMillis(h.quantile(0.95)),
		P99Ms: durationMillis(h.quantile(0.99)),
		MaxMs: durationMillis(h.max),
	}
}

func (h *benchLatencyHistogram) quantile(q float64) time.Duration {
	if h.count == 0 {
		return 0
	}
	target := int64(math.Ceil(float64(h.count) * q))
	if target < 1 {
		target = 1
	}
	var seen int64
	for i, hits := range h.bucketHit {
		seen += hits
		if seen >= target {
			if i < len(h.bounds) {
				return h.bounds[i]
			}
			return h.max
		}
	}
	return h.max
}

func durationMillis(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

func writeBenchIngestText(w io.Writer, result benchIngestResult) {
	fmt.Fprintf(w, "endpoint: %s\n", result.Endpoint)
	fmt.Fprintf(w, "transport: %s\n", result.Transport)
	fmt.Fprintf(w, "count: %d\n", result.Count)
	fmt.Fprintf(w, "concurrency: %d\n", result.Concurrency)
	fmt.Fprintf(w, "payload_bytes: %d\n", result.PayloadBytes)
	fmt.Fprintf(w, "submitted: %d\n", result.Submitted)
	fmt.Fprintf(w, "failed: %d\n", result.Failed)
	fmt.Fprintf(w, "batch_errors: %d\n", result.BatchErrors)
	fmt.Fprintf(w, "duration_seconds: %.3f\n", result.DurationSeconds)
	fmt.Fprintf(w, "throughput_per_sec: %.2f\n", result.ThroughputPerSec)
	fmt.Fprintf(w, "submit_latency_ms: avg=%.2f min=%.2f p50=%.2f p95=%.2f p99=%.2f max=%.2f\n",
		result.SubmitLatency.AvgMs,
		result.SubmitLatency.MinMs,
		result.SubmitLatency.P50Ms,
		result.SubmitLatency.P95Ms,
		result.SubmitLatency.P99Ms,
		result.SubmitLatency.MaxMs,
	)
	fmt.Fprintf(w, "record_query_samples: total=%d ready=%d failed=%d avg_ms=%.2f p95_ms=%.2f\n",
		result.QuerySamples.Samples,
		result.QuerySamples.Ready,
		result.QuerySamples.Failed,
		result.QuerySamples.Latency.AvgMs,
		result.QuerySamples.Latency.P95Ms,
	)
	fmt.Fprintf(w, "proof_samples: total=%d target=%s ready=%d timeouts=%d failed=%d avg_ms=%.2f p95_ms=%.2f\n",
		result.ProofSamples.Samples,
		result.ProofSamples.TargetLevel,
		result.ProofSamples.Ready,
		result.ProofSamples.Timeouts,
		result.ProofSamples.Failed,
		result.ProofSamples.Latency.AvgMs,
		result.ProofSamples.Latency.P95Ms,
	)
	if len(result.Metrics) > 0 {
		fmt.Fprintln(w, "metrics:")
		for _, metric := range result.Metrics {
			fmt.Fprintf(w, "  %s delta=%.6f after=%.6f\n", metric.Name, metric.Delta, metric.After)
		}
	}
	if len(result.ErrorSamples) > 0 {
		fmt.Fprintln(w, "errors:")
		for _, msg := range result.ErrorSamples {
			fmt.Fprintf(w, "  - %s\n", msg)
		}
	}
}
