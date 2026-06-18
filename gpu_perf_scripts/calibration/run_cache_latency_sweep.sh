#!/usr/bin/env bash
#
# Build the MI300A sample runner for cache_latency, run it under the CDNA3 /
# MI300A timing config at the SAME configurations measured on real hardware, and
# collect each run's kernel_time into a CSV consumed by the reporting step.
#
# Usage:  run_cache_latency_sweep.sh [output.csv]
#
# cache_latency is a single-thread pointer-chasing latency probe: one work-item
# performs `num_accesses` dependent loads over a randomly shuffled chain that
# fills an array of `array_bytes`. Sweeping array_bytes across the cache
# hierarchy (L1 / L2 / DRAM) exposes each level's latency; num_accesses is held
# at the real benchmark's value so the kernel TIME is directly comparable.
#
# COST: each run is a full cycle-accurate simulation whose wall time scales with
# num_accesses x per-access latency. At the real benchmark's 2,000,000 accesses
# this is expensive, especially for the large DRAM-footprint arrays. We bound it
# with two timeouts:
#   * PER_RUN_TIMEOUT  -- kill (and skip) any single config that runs too long.
#   * SWEEP_TIMEOUT    -- stop launching new configs once the budget is spent.
# Configs are ordered smallest-array-first so the cheap-to-simulate (cache-
# resident) points always complete before the large DRAM-footprint ones.
#
# A run that times out, errors, or otherwise produces no completed metric is
# recorded as a failure; this script tolerates that and still exits 0, so the
# report job always runs even if some configs fail.

set -euo pipefail

OUT="${1:-cache_latency_sim_results.csv}"
export CGO_ENABLED=1   # timing mode records metrics via the SQLite (CGO) recorder

# ---- sweep configuration (EDIT ME) ------------------------------------------
# Array sizes (bytes) -- exactly the 19 sizes measured on real hardware (see
# mi300a_ground_truth.csv: cache_latency, scaling_param=array_bytes), so every
# simulated point pairs with a real one for the kernel-time comparison.
CL_ARRAY_BYTES=(
  4096 8192 16384 32768 65536 131072 262144 524288 1048576 2097152
  4194304 8388608 16777216 33554432 67108864 134217728 268435456 536870912 1073741824
)
# Dependent loads per run -- MUST match the real benchmark (2,000,000) so the
# kernel times are directly comparable. This is ~2000x the prior throwaway
# value; a single thread chasing 2M dependent loads in cycle-accurate mode is
# expensive (wall time scales with accesses x per-access latency), so the large
# DRAM-footprint arrays may hit PER_RUN_TIMEOUT and be skipped. Arrays are run
# smallest-first so the cache-resident points always complete.
CL_NUM_ACCESSES=(2000000)
CL_SEED=42
PER_RUN_TIMEOUT=${PER_RUN_TIMEOUT:-1800}   # 30 min per individual simulation
SWEEP_TIMEOUT=${SWEEP_TIMEOUT:-86400}      # 24 h budget to launch new configs
COMMON_FLAGS=(-timing -arch cdna3 -gpu mi300a -disable-rtm)
# -----------------------------------------------------------------------------

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$REPO_ROOT"

BUILD_DIR="$(mktemp -d)"
WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$BUILD_DIR" "$WORK_DIR"' EXIT

# Portable per-run timeout wrapper.
TIMEOUT_BIN=""
if command -v timeout >/dev/null 2>&1; then TIMEOUT_BIN="timeout"
elif command -v gtimeout >/dev/null 2>&1; then TIMEOUT_BIN="gtimeout"
else echo "NOTE: no timeout/gtimeout on PATH; per-run timeout disabled." >&2; fi

with_timeout() {  # $1=secs ...cmd
  local secs="$1"; shift
  if [[ -n "$TIMEOUT_BIN" ]]; then "$TIMEOUT_BIN" "$secs" "$@"; else "$@"; fi
}

echo "Building cache_latency sample runner..."
go build -o "$BUILD_DIR/cache_latency" ./amd/samples/cache_latency/

OUT_ABS="$(cd "$(dirname "$OUT")" && pwd)/$(basename "$OUT")"
echo "benchmark,scaling_param_name,scaling_param_value,array_bytes,num_accesses,kernel_time_s" > "$OUT_ABS"

# Run a built binary (under the per-run timeout) in a clean dir, then read
# kernel_time (seconds) from the metric DB. A timed-out/failed/panicked run
# leaves no completed metric. Echoes one of: "ok <seconds>", "timeout", "fail".
# (Status is returned via stdout, not a variable, because the caller invokes
# this inside $(...) -- a subshell whose variable assignments would be lost.)
run_and_extract() {  # $1=binary  $2=tag  ...rest=extra flags
  local bin="$1" tag="$2"; shift 2
  local run_dir; run_dir="$(mktemp -d -p "$WORK_DIR")"
  local rc=0
  ( cd "$run_dir" && with_timeout "$PER_RUN_TIMEOUT" \
      "$bin" "${COMMON_FLAGS[@]}" -metric-file-name "$tag" "$@" >/dev/null 2>&1 ) || rc=$?
  local db="$run_dir/$tag.sqlite3"
  local kt=""
  if [[ -f "$db" ]]; then
    kt="$(sqlite3 "$db" "SELECT Value FROM mgpusim_metrics WHERE What='kernel_time' AND Location='Driver' LIMIT 1;" 2>/dev/null)"
  fi
  if [[ -n "$kt" ]]; then echo "ok $kt"
  elif [[ "$rc" == "124" || "$rc" == "137" ]]; then echo "timeout"
  else echo "fail"; fi
}

echo "Sweeping cache_latency (per-run ${PER_RUN_TIMEOUT}s, sweep ${SWEEP_TIMEOUT}s), smallest array first..."
SWEEP_START=$(date +%s)
ran=0; timed_out=0; failed=0
for na in "${CL_NUM_ACCESSES[@]}"; do
  for ab in "${CL_ARRAY_BYTES[@]}"; do
    elapsed=$(( $(date +%s) - SWEEP_START ))
    if (( elapsed >= SWEEP_TIMEOUT )); then
      echo "  sweep budget ${SWEEP_TIMEOUT}s reached (${elapsed}s elapsed); stopping early."
      break 2
    fi
    tag="cl_b${ab}_n${na}"
    result="$(run_and_extract "$BUILD_DIR/cache_latency" "$tag" \
          -array-bytes "$ab" -num-accesses "$na" -seed "$CL_SEED")"
    status="${result%% *}"; kt="${result#* }"
    case "$status" in
      ok) echo "cache_latency,array_bytes,${ab},${ab},${na},${kt}" >> "$OUT_ABS"
          ran=$((ran+1)); echo "  ok      array_bytes=$ab num_accesses=$na (sim kernel ${kt}s)";;
      timeout) timed_out=$((timed_out+1)); echo "  timeout array_bytes=$ab num_accesses=$na (> ${PER_RUN_TIMEOUT}s, skipped)";;
      *) failed=$((failed+1)); echo "  FAIL    array_bytes=$ab num_accesses=$na (no metric)";;
    esac
  done
done

echo "Done: $ran ok, $timed_out timed out, $failed failed. Wrote $OUT_ABS"
