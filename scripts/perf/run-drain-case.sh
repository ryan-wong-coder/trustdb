#!/usr/bin/env bash

set -euo pipefail

if [[ $# -ne 9 ]]; then
  echo "usage: $0 <case> <transport> <endpoint> <metrics-url> <count> <concurrency> <payload-bytes> <proof-level> <output-dir>" >&2
  exit 2
fi

case_name=$1
transport=$2
endpoint=$3
metrics_url=${4%/}
count=$5
concurrency=$6
payload_bytes=$7
proof_level=${8^^}
output_dir=$9

trustdb_bin=${TRUSTDB_BIN:-trustdb}
private_key=${TRUSTDB_PRIVATE_KEY:-client.key}
tenant=${TRUSTDB_TENANT:-perf}
client=${TRUSTDB_CLIENT:-perf-client}
key_id=${TRUSTDB_KEY_ID:-perf-key}
samples=${TRUSTDB_BENCH_SAMPLES:-8}
proof_timeout=${TRUSTDB_PROOF_TIMEOUT:-10m}
drain_timeout_seconds=${TRUSTDB_DRAIN_TIMEOUT_SECONDS:-1200}
semantic_profile=${TRUSTDB_SEMANTIC_PROFILE:-benchmark}
durability_profile=${TRUSTDB_DURABILITY_PROFILE:-unknown}
proof_mode=${TRUSTDB_PROOF_MODE:-async}
record_index_mode=${TRUSTDB_RECORD_INDEX_MODE:-full}

case "$proof_level" in
  L3|L4|L5) ;;
  *) echo "proof level must be L3, L4, or L5" >&2; exit 2 ;;
esac

mkdir -p "$output_dir"
bench_report="$output_dir/bench.json"
bench_stdout="$output_dir/bench.stdout.json"
bench_stderr="$output_dir/bench.stderr.log"
summary_file="$output_dir/drain-summary.json"

fetch_metrics() {
  curl --fail --silent --show-error "$metrics_url/metrics"
}

metric_sum() {
  local snapshot=$1
  local metric=$2
  printf '%s\n' "$snapshot" | awk -v metric="$metric" '
    $1 == metric || index($1, metric "{") == 1 { sum += $2 }
    END { printf "%.0f", sum + 0 }
  '
}

metric_label() {
  local snapshot=$1
  local metric=$2
  local label=$3
  printf '%s\n' "$snapshot" | awk -v metric="$metric" -v label="$label" '
    index($1, metric "{") == 1 && index($1, label) > 0 { sum += $2 }
    END { printf "%.0f", sum + 0 }
  '
}

latest_sth_size() {
  curl --fail --silent "$metrics_url/v1/sth/latest" 2>/dev/null | jq -r '.tree_size // 0' 2>/dev/null || printf '0'
}

elapsed_seconds() {
  awk -v start="$1" -v end="$2" 'BEGIN { printf "%.6f", (end - start) / 1000000000 }'
}

throughput() {
  awk -v records="$1" -v seconds="$2" 'BEGIN { if (seconds > 0) printf "%.3f", records / seconds; else print "0" }'
}

before_metrics=$(fetch_metrics)
before_materialized=$(metric_sum "$before_metrics" trustdb_materialized_records_total)
before_batches=$(metric_sum "$before_metrics" trustdb_batch_size_records_count)
before_sth=$(latest_sth_size)
before_global_published=$(metric_sum "$before_metrics" trustdb_global_log_published_roots_total)
before_anchors=$(metric_sum "$before_metrics" trustdb_anchor_published_total)
started_ns=$(date +%s%N)
deadline=$(( $(date +%s) + drain_timeout_seconds ))

"$trustdb_bin" bench ingest \
  --server "$endpoint" \
  --transport "$transport" \
  --private-key "$private_key" \
  --tenant "$tenant" \
  --client "$client" \
  --key-id "$key_id" \
  --count "$count" \
  --concurrency "$concurrency" \
  --payload-bytes "$payload_bytes" \
  --samples "$samples" \
  --proof-level "$proof_level" \
  --max-proof-level "$proof_level" \
  --proof-timeout "$proof_timeout" \
  --settle 0s \
  --semantic-profile "$semantic_profile" \
  --durability-profile "$durability_profile" \
  --proof-mode "$proof_mode" \
  --record-index-mode "$record_index_mode" \
  --output json \
  --report-file "$bench_report" \
  >"$bench_stdout" 2>"$bench_stderr" &
bench_pid=$!

bench_status=-1
l3_finished_ns=0
l4_finished_ns=0
l5_finished_ns=0
target_batches=0
final_metrics=$before_metrics

while true; do
  now_seconds=$(date +%s)
  if (( now_seconds >= deadline )); then
    kill "$bench_pid" 2>/dev/null || true
    wait "$bench_pid" 2>/dev/null || true
    echo "case $case_name exceeded drain timeout" >&2
    exit 124
  fi

  if (( bench_status == -1 )) && ! kill -0 "$bench_pid" 2>/dev/null; then
    if wait "$bench_pid"; then
      bench_status=0
    else
      bench_status=$?
    fi
    if (( bench_status == 0 )); then
      bench_failures=$(jq -r '(.failed // 0) + (.batch_errors // 0)' "$bench_report")
      if (( bench_failures > 0 )); then
        echo "case $case_name completed with $bench_failures submit or batch errors" >&2
        exit 3
      fi
    fi
  fi

  if ! final_metrics=$(fetch_metrics); then
    sleep 0.1
    continue
  fi
  materialized=$(metric_sum "$final_metrics" trustdb_materialized_records_total)
  batches=$(metric_sum "$final_metrics" trustdb_batch_size_records_count)
  ingest_queue=$(metric_label "$final_metrics" trustdb_queue_depth 'queue="ingest"')
  batch_queue=$(metric_label "$final_metrics" trustdb_queue_depth 'queue="batch"')
  materializer_queue=$(metric_label "$final_metrics" trustdb_queue_depth 'queue="materializer"')
  materializer_in_flight=$(metric_sum "$final_metrics" trustdb_materializer_in_flight)

  if (( l3_finished_ns == 0 )) && \
     (( materialized - before_materialized >= count )) && \
     (( ingest_queue == 0 && batch_queue == 0 && materializer_queue == 0 && materializer_in_flight == 0 )); then
    l3_finished_ns=$(date +%s%N)
    target_batches=$(( batches - before_batches ))
  fi

  if (( l3_finished_ns > 0 )) && [[ "$proof_level" != "L3" ]] && (( l4_finished_ns == 0 )); then
    global_published=$(metric_sum "$final_metrics" trustdb_global_log_published_roots_total)
    if (( global_published - before_global_published >= target_batches )); then
      l4_finished_ns=$(date +%s%N)
    fi
  fi

  if (( l4_finished_ns > 0 )) && [[ "$proof_level" == "L5" ]] && (( l5_finished_ns == 0 )); then
    anchors=$(metric_sum "$final_metrics" trustdb_anchor_published_total)
    anchor_in_flight=$(metric_sum "$final_metrics" trustdb_anchor_in_flight)
    if (( anchors - before_anchors >= target_batches && anchor_in_flight == 0 )); then
      l5_finished_ns=$(date +%s%N)
    fi
  fi

  target_finished_ns=$l3_finished_ns
  [[ "$proof_level" == "L4" ]] && target_finished_ns=$l4_finished_ns
  [[ "$proof_level" == "L5" ]] && target_finished_ns=$l5_finished_ns
  if (( bench_status != -1 && target_finished_ns > 0 )); then
    break
  fi
  sleep 0.1
done

if (( bench_status != 0 )); then
  echo "bench process failed with status $bench_status" >&2
  exit "$bench_status"
fi

finished_ns=$(date +%s%N)
l3_seconds=$(elapsed_seconds "$started_ns" "$l3_finished_ns")
l3_tps=$(throughput "$count" "$l3_seconds")
l4_seconds=0
l4_tps=0
l5_seconds=0
l5_tps=0
if (( l4_finished_ns > 0 )); then
  l4_seconds=$(elapsed_seconds "$started_ns" "$l4_finished_ns")
  l4_tps=$(throughput "$count" "$l4_seconds")
fi
if (( l5_finished_ns > 0 )); then
  l5_seconds=$(elapsed_seconds "$started_ns" "$l5_finished_ns")
  l5_tps=$(throughput "$count" "$l5_seconds")
fi

final_materialized=$(metric_sum "$final_metrics" trustdb_materialized_records_total)
final_batches=$(metric_sum "$final_metrics" trustdb_batch_size_records_count)
final_sth=$(latest_sth_size)
final_global_published=$(metric_sum "$final_metrics" trustdb_global_log_published_roots_total)
final_anchors=$(metric_sum "$final_metrics" trustdb_anchor_published_total)
total_seconds=$(elapsed_seconds "$started_ns" "$finished_ns")

jq -n \
  --arg schema_version "trustdb.bench.drain.v1" \
  --arg case_name "$case_name" \
  --arg transport "$transport" \
  --arg endpoint "$endpoint" \
  --arg proof_level "$proof_level" \
  --argjson count "$count" \
  --argjson concurrency "$concurrency" \
  --argjson payload_bytes "$payload_bytes" \
  --argjson batch_count "$target_batches" \
  --argjson total_seconds "$total_seconds" \
  --argjson l3_seconds "$l3_seconds" \
  --argjson l3_tps "$l3_tps" \
  --argjson l4_seconds "$l4_seconds" \
  --argjson l4_tps "$l4_tps" \
  --argjson l5_seconds "$l5_seconds" \
  --argjson l5_tps "$l5_tps" \
  --argjson materialized_delta "$(( final_materialized - before_materialized ))" \
  --argjson batch_delta "$(( final_batches - before_batches ))" \
  --argjson sth_delta "$(( final_sth - before_sth ))" \
  --argjson global_published_delta "$(( final_global_published - before_global_published ))" \
  --argjson anchor_delta "$(( final_anchors - before_anchors ))" \
  --slurpfile bench "$bench_report" \
  '{
    schema_version: $schema_version,
    case_name: $case_name,
    transport: $transport,
    endpoint: $endpoint,
    proof_level: $proof_level,
    count: $count,
    concurrency: $concurrency,
    payload_bytes: $payload_bytes,
    batch_count: $batch_count,
    total_seconds: $total_seconds,
    l3_materialized_seconds: $l3_seconds,
    l3_materialized_tps: $l3_tps,
    l4_ready_seconds: $l4_seconds,
    l4_ready_tps: $l4_tps,
    l5_ready_seconds: $l5_seconds,
    l5_ready_tps: $l5_tps,
    materialized_delta: $materialized_delta,
    batch_delta: $batch_delta,
    sth_delta: $sth_delta,
    global_published_delta: $global_published_delta,
    anchor_delta: $anchor_delta,
    bench: $bench[0]
  }' | tee "$summary_file"
