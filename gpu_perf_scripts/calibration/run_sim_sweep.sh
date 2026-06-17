#!/usr/bin/env bash
#
# Build the MI300A sample runner for fp32_throughput, sweep a grid of
# configurations under the CDNA3 / MI300A timing config, and collect each run's
# kernel_time into a CSV consumed by compare_to_ground_truth.py / plot_calibration.py.
#
# Usage:  run_sim_sweep.sh [output.csv]
#
# The fp32 kernel takes the work-group size as an explicit argument, so the sweep
# varies all three ground-truth dimensions: num_blocks, threads_per_block, and
# fmas_per_thread (the scaling factor).
#
# COST: every run is a full cycle-accurate simulation whose wall time scales with
# the total FMA work (num_blocks * threads_per_block * fmas_per_thread); the most
# expensive corners can take much longer than others. Rather than guess, we bound
# cost with two timeouts:
#   * PER_RUN_TIMEOUT  -- kill (and skip) any single config that runs too long.
#   * SWEEP_TIMEOUT    -- stop launching new configs once the whole sweep budget
#                         is spent. Configs are ordered by real-hardware execution
#                         time (shortest first, read from the ground-truth CSV) so
#                         the budget completes the cheap-to-simulate configs first.
# Per-run timeouts need `timeout`/`gtimeout` on PATH (present on the Linux CI
# runner); without it, runs are not individually time-capped.

set -euo pipefail

OUT="${1:-sim_results.csv}"
export CGO_ENABLED=1   # timing mode records metrics via the SQLite (CGO) recorder

# ---- sweep configuration (EDIT ME) ------------------------------------------
FP32_NUM_BLOCKS=(1 32 1024)
FP32_THREADS_PER_BLOCK=(1 32 64 256 1024)
FP32_FMAS=(256 1024 4096 16384 65536 262144 1048576)
PER_RUN_TIMEOUT=${PER_RUN_TIMEOUT:-600}    # 10 min per individual simulation
SWEEP_TIMEOUT=${SWEEP_TIMEOUT:-7200}       # 120 min for the whole fp32 sweep
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

echo "Building fp32_throughput sample runner..."
go build -o "$BUILD_DIR/fp32_throughput" ./amd/samples/fp32_throughput/

OUT_ABS="$(cd "$(dirname "$OUT")" && pwd)/$(basename "$OUT")"
echo "benchmark,scaling_param_name,scaling_param_value,num_blocks,threads_per_block,fmas_per_thread,num_accesses,kernel_time_s" > "$OUT_ABS"

# Run a built binary (under the per-run timeout) in a clean dir, then read
# kernel_time (seconds) from the metric DB. A timed-out/failed run leaves no
# completed metric. Echoes one of: "ok <seconds>", "timeout", or "fail".
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

# Order configs by real-hardware execution time (shortest first). Join the
# cartesian product of the sweep arrays against the ground-truth CSV (awk reads
# the CSV in BEGIN, then annotates each combo with its real kernel_ms_mean), then
# numeric-sort ascending. Configs with no ground-truth entry get +inf -> run last.
REF_CSV="$SCRIPT_DIR/mi300a_ground_truth.csv"
[[ -f "$REF_CSV" ]] || echo "NOTE: ground-truth CSV not found ($REF_CSV); sweep will be unordered." >&2

build_combos() {
  local nb tpb fmas
  for nb in "${FP32_NUM_BLOCKS[@]}"; do
    for tpb in "${FP32_THREADS_PER_BLOCK[@]}"; do
      for fmas in "${FP32_FMAS[@]}"; do echo "$nb,$tpb,$fmas"; done
    done
  done
}

# Lines: "<real_ms> <nb> <tpb> <fmas>", ascending by real_ms.
SORTED="$(build_combos | awk -F, -v ref="$REF_CSV" '
  BEGIN {
    while ((getline line < ref) > 0) {
      split(line, f, ",")
      if (f[1] == "fp32_throughput" && f[4] != "" && f[5] != "")
        real[f[4]"|"f[5]"|"f[3]] = f[9]   # nb|tpb|fmas -> kernel_ms_mean
    }
  }
  { k = $1"|"$2"|"$3; ms = (k in real) ? real[k] : 1e18; printf "%s %s %s %s\n", ms, $1, $2, $3 }
' | sort -g -k1,1)"

echo "Sweeping fp32_throughput (per-run ${PER_RUN_TIMEOUT}s, sweep ${SWEEP_TIMEOUT}s), shortest ground-truth time first..."
SWEEP_START=$(date +%s)
ran=0; timed_out=0; failed=0
while read -r real_ms nb tpb fmas; do
  [[ -n "${nb:-}" ]] || continue
  elapsed=$(( $(date +%s) - SWEEP_START ))
  if (( elapsed >= SWEEP_TIMEOUT )); then
    echo "  sweep budget ${SWEEP_TIMEOUT}s reached (${elapsed}s elapsed); stopping early."
    break
  fi
  tag="fp32_nb${nb}_t${tpb}_f${fmas}"
  result="$(run_and_extract "$BUILD_DIR/fp32_throughput" "$tag" \
        -num-blocks "$nb" -threads-per-block "$tpb" -fmas "$fmas")"
  status="${result%% *}"; kt="${result#* }"
  case "$status" in
    ok) echo "fp32_throughput,fmas_per_thread,${fmas},${nb},${tpb},${fmas},,${kt}" >> "$OUT_ABS"
        ran=$((ran+1)); echo "  ok      nb=$nb tpb=$tpb fmas=$fmas (real ${real_ms}ms, sim ${kt}s)";;
    timeout) timed_out=$((timed_out+1)); echo "  timeout nb=$nb tpb=$tpb fmas=$fmas (> ${PER_RUN_TIMEOUT}s, skipped)";;
    *) failed=$((failed+1)); echo "  FAIL    nb=$nb tpb=$tpb fmas=$fmas";;
  esac
done <<< "$SORTED"

echo "Done: $ran ok, $timed_out timed out, $failed failed. Wrote $OUT_ABS"
