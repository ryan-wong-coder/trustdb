# TrustDB Optimized Single-Node Performance Report

![AI generated TrustDB performance overview](assets/perf-v3-ai-overview.png)

Report date: 2026-04-30
Run ID: `perf-v3-20260430T043651Z`
Code path: optimized bench v3 / Merkle / Pebble artifact / WAL observability / SDK HTTP transport branch
Primary goal: validate current single-node ingest behavior while separating accepted submission, immediate query visibility, and proof-ready query visibility.

The image above is an AI-generated visual overview for the report. The tables and metric charts below are generated from the recorded benchmark and matrix outputs and are the source of truth.

## Executive Summary

The optimized TrustDB build completed the CVM matrix without submission failures, batch errors, proof failures, proof timeouts, or post-proof query failures.

| Result | HTTP matrix | gRPC matrix | Combined |
| --- | ---: | ---: | ---: |
| Submitted records | 185,000 | 65,000 | 250,000 |
| Submit failures | 0 | 0 | 0 |
| Batch errors | 0 | 0 | 0 |
| Proof timeouts | 0 | 0 | 0 |
| Proof failures | 0 | 0 | 0 |
| Post-proof query failures | 0 | 0 | 0 |
| Immediate query misses | 2 / 160 samples | 2 / 80 samples | 4 / 240 samples |

The four immediate query misses are asynchronous visibility observations: a record was accepted, but its L3/L4 proof and record index were not yet visible at the exact immediate-read sample point. After waiting for proof readiness, all sampled records were readable.

![TrustDB throughput chart](assets/perf-v3-throughput.png)

## Test Environment

| Item | Value |
| --- | --- |
| Cloud host | Tencent Cloud CVM |
| Region / zone | Nanjing / Nanjing Zone 1 |
| Instance type | `SA5.8XLARGE64` |
| CPU / memory | 32 vCPU / 64 GiB |
| OS image | TencentOS Server 4 for x86_64 |
| System disk | 100 GiB enhanced SSD cloud disk |
| Data disk | Not attached |
| TrustDB service | `trustdb-perf.service`, active |
| Health check | `{"ok":true}` |
| Run data size | 1.9 GiB under the run directory |
| Go runtime for microbenchmarks | Go 1.26.2 linux/amd64 |
| CPU reported by Go benchmarks | AMD EPYC 9754 128-Core Processor |

The report intentionally omits server passwords, keys, and public endpoint credentials.

## Measurement Semantics

Bench reports use `trustdb.bench.ingest.v3`.

| Metric | Meaning |
| --- | --- |
| `submit_duration_seconds` / `submit_throughput_per_sec` | Accepted-receipt submission path before asynchronous proof readiness. |
| `immediate_query_samples` | `GetRecord` samples taken immediately after submission. Misses here measure asynchronous visibility delay. |
| `proof_wait_duration_seconds` / `proof_samples` | Wait for the configured proof target, this run used L4. |
| `post_proof_query_samples` | `GetRecord` samples after proof readiness. This is the correct correctness signal for proof-ready reads. |
| `throughput_per_sec` | End-to-end compatible field including submit, proof wait, post-proof checks, and settle time. |

This split prevents an accepted asynchronous write from being misclassified as a business failure.

![TrustDB visibility chart](assets/perf-v3-visibility.png)

## Matrix Results

### HTTP

| Case | Records | Concurrency | Payload | Endpoint throughput | Submit throughput | Submit p95 | Immediate misses | Post-proof failures |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `p1k-c8` | 10,000 | 8 | 1 KiB | 2,485.09/s | 10,088.53/s | 5 ms | 0 | 0 |
| `p1k-c16` | 20,000 | 16 | 1 KiB | 4,340.70/s | 12,779.60/s | 5 ms | 0 | 0 |
| `p1k-c32` | 30,000 | 32 | 1 KiB | 2,735.06/s | 14,728.02/s | 10 ms | 0 | 0 |
| `p1k-c64` | 30,000 | 64 | 1 KiB | 1,488.97/s | 19,570.34/s | 10 ms | 1 | 0 |
| `p1k-c128` | 30,000 | 128 | 1 KiB | 1,600.20/s | 26,237.86/s | 10 ms | 0 | 0 |
| `p4k-c32` | 20,000 | 32 | 4 KiB | 1,636.06/s | 16,403.38/s | 5 ms | 0 | 0 |
| `p4k-c64` | 20,000 | 64 | 4 KiB | 1,829.18/s | 20,121.75/s | 10 ms | 0 | 0 |
| `p16k-c32` | 10,000 | 32 | 16 KiB | 898.30/s | 16,496.42/s | 5 ms | 0 | 0 |
| `p16k-c64` | 10,000 | 64 | 16 KiB | 1,984.24/s | 21,055.81/s | 10 ms | 0 | 0 |
| `p64k-c32` | 5,000 | 32 | 64 KiB | 678.74/s | 17,054.48/s | 5 ms | 1 | 0 |

HTTP matrix aggregate:

| Metric | Value |
| --- | ---: |
| Average endpoint throughput | 1,967.65/s |
| Average submit throughput | 17,453.62/s |
| Fastest endpoint case | `p1k-c16`, 4,340.70/s |
| Fastest submit case | `p1k-c128`, 26,237.86/s |
| Worst submit p95 | 10 ms |

### gRPC

| Case | Records | Concurrency | Payload | Endpoint throughput | Submit throughput | Submit p95 | Immediate misses | Post-proof failures |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `p1k-c16` | 10,000 | 16 | 1 KiB | 2,708.57/s | 15,120.64/s | 5 ms | 0 | 0 |
| `p1k-c32` | 20,000 | 32 | 1 KiB | 2,537.51/s | 18,641.82/s | 5 ms | 0 | 0 |
| `p1k-c64` | 20,000 | 64 | 1 KiB | 1,272.46/s | 24,294.90/s | 10 ms | 1 | 0 |
| `p4k-c32` | 10,000 | 32 | 4 KiB | 887.78/s | 19,521.52/s | 5 ms | 0 | 0 |
| `p16k-c32` | 5,000 | 32 | 16 KiB | 1,074.05/s | 17,649.55/s | 5 ms | 1 | 0 |

gRPC matrix aggregate:

| Metric | Value |
| --- | ---: |
| Average endpoint throughput | 1,696.07/s |
| Average submit throughput | 19,045.69/s |
| Fastest endpoint case | `p1k-c16`, 2,708.57/s |
| Fastest submit case | `p1k-c64`, 24,294.90/s |
| Worst submit p95 | 10 ms |

## Hot-Path Microbenchmarks

![TrustDB microbenchmark chart](assets/perf-v3-microbench.png)

| Benchmark | Mean time | Mean bytes/op | Mean allocs/op |
| --- | ---: | ---: | ---: |
| `BenchmarkCommitBatchSynthetic1024` | 32.51 ms/op | 3.92 MB/op | 21,526 |
| `BenchmarkPebblePutBatchArtifacts1024` | 36.12 ms/op | 35.48 MB/op | 237,146 |
| `BenchmarkPebblePutBatchArtifacts8192` | 295.30 ms/op | 282.19 MB/op | 1,895,782 |
| `BenchmarkPebbleGetBundleV2` | 41.48 us/op | 41.63 KiB/op | 163 |
| `BenchmarkWALAppendGroup` | 3.77 us/op | 1,152 B/op | 1 |

The optimized path keeps WAL append overhead small while preserving the default group-fsync durability boundary. The remaining heavy allocation area is proof artifact persistence, especially large batch writes. That path is now chunked and compressed, but it still has room for further memory-pressure work.

## What The Results Mean

TrustDB is currently healthy for single-node proof-ready ingest testing on this CVM class:

- The accepted submission path sustains roughly 17K to 19K records/s on average across HTTP and gRPC matrices.
- End-to-end proof-ready throughput is lower, as expected, because it includes proof wait, record query validation, and settle time.
- L4 proof readiness completed without timeout across all sampled records.
- Post-proof record reads were fully successful, which is the key correctness signal for this asynchronous architecture.
- Submit p95 stayed within 5 ms to 10 ms across the matrix.

The most important interpretation change is that immediate query misses are no longer reported as generic query failures. They describe asynchronous visibility delay between L2 acceptance and L3/L4 proof/index readiness.

## Current Optimization Notes

This run validates the following optimized implementation areas:

- Bench schema v3 separates immediate visibility from proof-ready visibility.
- Merkle proof generation avoids repeated subtree recomputation and supports batch proof generation.
- Pebble proof artifacts use a v2 compressed envelope with legacy read fallback.
- Pebble batch artifact writes use 1024-record sync chunks and shared encoding paths.
- Secondary record indexes store compact references while preserving legacy read compatibility.
- WAL append and fsync metrics are wired into the writer path without changing default durability semantics.
- SDK and benchmark HTTP transports use larger connection pools while keeping `WithHTTPClient` overrides.
- Batch stage metrics expose collect, build, artifacts, checkpoint, manifest, and outbox boundaries.

## Remaining Capacity Risks

The current test host has no dedicated data disk. WAL, Pebble WAL, table flushes, proof artifacts, and compaction all share the 100 GiB system disk. For longer runs or stricter production SLOs, the next hardware test should place WAL and Pebble data on a dedicated high-performance data disk.

The next software optimization target is memory reduction in `PutBatchArtifacts`, especially bundle/index encoding for large batches. A second target is proof-ready latency under large payloads, where HTTP `p64k-c32` showed a proof wait average of 253.82 ms with one sampled record waiting about 4.04 seconds before becoming proof-ready.

## Validation Commands

The optimized branch was validated with:

```powershell
go test -p 1 ./...
go test -p 1 -tags=e2e ./cmd/trustdb
go test -race -tags='integration e2e' ./...
```

Remote microbenchmarks used:

```bash
go test -run '^$' -bench 'BenchmarkCommitBatchSynthetic1024' -benchmem -count=6 ./internal/app
go test -run '^$' -bench 'BenchmarkPebblePutBatchArtifacts1024|BenchmarkPebblePutBatchArtifacts8192|BenchmarkPebbleGetBundleV2' -benchmem -count=6 ./internal/proofstore/pebble
go test -run '^$' -bench 'BenchmarkWALAppendGroup' -benchmem -count=6 ./internal/wal
```

Primary local report artifacts were retained under:

```text
.localdeploy/perf-v3-20260430T043651Z/reports/
```
