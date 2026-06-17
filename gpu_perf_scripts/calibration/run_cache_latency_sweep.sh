#!/usr/bin/env bash
#
# Build the MI300A sample runner for cache_latency, sweep a grid of
# configurations under the CDNA3 / MI300A timing config, and collect each run's
# kernel_time into a CSV consumed by the reporting step.
#
# Usage:  run_cache_latency_sweep.sh [output.csv]
#
# cache_latency is a single-thread pointer-chasing latency probe: one work-item
# performs `num_accesses` dependent loads over a randomly shuffled chain that
# fills an array of `array_bytes`. The per-access latency is kernel_time /
# num_accesses; sweeping array_bytes across the cache hierarchy (L1 / L2 / DRAM)
# exposes each level's latency.
#
# COST: each run is a full cycle-accurate simulation. The single dependent-load
# chain makes wall time scale with num_accesses (not array_bytes), so we keep
# num_accesses modest. We still bound cost with two timeouts:
#   * PER_RUN_TIMEOUT  -- kill (and skip) any single config that runs too long.
#   * SWEEP_TIMEOUT    -- stop launching new configs once the budget is spent.
# Configs are ordered smallest-array-first so the cheap-to-simulate (cache-
# resident) points complete before the large DRAM-footprint ones.
#
# A run that times out, errors, or otherwise produces no completed metric is
# recorded as a failure; this script tolerates that and still exits 0, so the
# report job always runs even if some configs fail.

set -euo pipefail

OUT="${1:-cache_latency_sim_results.csv}"
export CGO_ENABLED=1   # timing mode records metrics via the SQLite (CGO) recorder

# ---- sweep configuration (EDIT ME) ------------------------------------------
# Array sizes (bytes) spanning the cache hierarchy, sampled densely enough to
# resolve the latency plateaus AND the transitions between levels (extra points
# clustered around the L1~=32 KB and L2~=4-8 MB boundaries). Classification
# (matching the HIP microbenchmark): <=16 KB -> L1, <=8 MB -> L2, else DRAM.
CL_ARRAY_BYTES=(
  4096 8192 16384 24576 32768 49152 65536 131072 262144 524288
  1048576 2097152 3145728 4194304 6291456 8388608 12582912 16777216 33554432
  67108864 134217728
)
# Dependent loads per run. A single thread chasing N dependent loads in
# cycle-accurate mode costs ~N memory round-trips of wall time; both values are
# run at every array size (256 = quick, 1024 = better-amortized cross-check).
CL_NUM_ACCESSES=(256 1024)
CL_SEED=42
PER_RUN_TIMEOUT=${PER_RUN_TIMEOUT:-900}    # 15 min per individual simulation
SWEEP_TIMEOUT=${SWEEP_TIMEOUT:-6000}       # 100 min for the whole cache sweep
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
echo "benchmark,scaling_param_name,scaling_param_value,array_bytes,num_accesses,kernel_time_s,ns_per_access" > "$OUT_ABS"

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
      ok) ns="$(awk -v k="$kt" -v n="$na" 'BEGIN{ printf "%.4f", k*1e9/n }')"
          echo "cache_latency,array_bytes,${ab},${ab},${na},${kt},${ns}" >> "$OUT_ABS"
          ran=$((ran+1)); echo "  ok      array_bytes=$ab num_accesses=$na (sim ${kt}s, ${ns} ns/access)";;
      timeout) timed_out=$((timed_out+1)); echo "  timeout array_bytes=$ab num_accesses=$na (> ${PER_RUN_TIMEOUT}s, skipped)";;
      *) failed=$((failed+1)); echo "  FAIL    array_bytes=$ab num_accesses=$na (no metric)";;
    esac
  done
done

echo "Done: $ran ok, $timed_out timed out, $failed failed. Wrote $OUT_ABS"
