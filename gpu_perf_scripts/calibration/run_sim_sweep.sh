#!/usr/bin/env bash
#
# Build the MI300A sample runner for fp32_throughput, sweep a set of
# configurations under the CDNA3 / MI300A timing config, and collect each run's
# kernel_time into a CSV consumed by compare_to_ground_truth.py.
#
# Usage:  run_sim_sweep.sh [output.csv]
#
# NOTE ON COST: every run is a full cycle-accurate simulation. The sweep below
# is deliberately SMALL so the job stays well under the CI timeout. Configs only
# need to MATCH ground-truth configs to be comparable -- the fp32 ground truth
# has num_blocks in {1,32,1024,4096,32768} at threads_per_block=256. Expand the
# arrays once you've measured real runtime on the self-hosted runner.
#
# cache_latency is deferred (it panics in timing mode -- "page not found" in the
# MMU page walk); add a second sweep here once that benchmark is fixed.

set -euo pipefail

OUT="${1:-sim_results.csv}"
export CGO_ENABLED=1   # timing mode records metrics via the SQLite (CGO) recorder

# ---- sweep configuration (EDIT ME) ------------------------------------------
FP32_NUM_BLOCKS=(1 32)                 # must be a subset of ground-truth num_blocks
FP32_FMAS=(4096 16384 65536 262144)    # ground-truth fmas range: 256 .. 1048576
                                       # (larger fmas = throughput-bound = fairer GFLOPS)
COMMON_FLAGS=(-timing -arch cdna3 -gpu mi300a -disable-rtm)
# -----------------------------------------------------------------------------

# Resolve repo root (this script lives in gpu_perf_scripts/calibration/).
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$REPO_ROOT"

BUILD_DIR="$(mktemp -d)"
WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$BUILD_DIR" "$WORK_DIR"' EXIT

echo "Building fp32_throughput sample runner..."
go build -o "$BUILD_DIR/fp32_throughput" ./amd/samples/fp32_throughput/

OUT_ABS="$(cd "$(dirname "$OUT")" && pwd)/$(basename "$OUT")"
echo "benchmark,scaling_param_name,scaling_param_value,num_blocks,threads_per_block,fmas_per_thread,num_accesses,kernel_time_s" > "$OUT_ABS"

# Run a built binary in a clean dir, then read kernel_time (seconds) from the
# metric DB. -metric-file-name gives a deterministic <name>.sqlite3 file.
run_and_extract() {  # $1=binary  $2=tag  ...rest=extra flags
  local bin="$1" tag="$2"; shift 2
  local run_dir; run_dir="$(mktemp -d -p "$WORK_DIR")"
  ( cd "$run_dir" && "$bin" "${COMMON_FLAGS[@]}" -metric-file-name "$tag" "$@" >/dev/null 2>&1 )
  local db="$run_dir/$tag.sqlite3"
  [[ -f "$db" ]] || { echo "  WARN: no metric DB for $tag" >&2; echo ""; return; }
  sqlite3 "$db" "SELECT Value FROM mgpusim_metrics WHERE What='kernel_time' AND Location='Driver' LIMIT 1;"
}

echo "Sweeping fp32_throughput..."
for nb in "${FP32_NUM_BLOCKS[@]}"; do
  for fmas in "${FP32_FMAS[@]}"; do
    tag="fp32_nb${nb}_f${fmas}"
    echo "  $tag"
    kt="$(run_and_extract "$BUILD_DIR/fp32_throughput" "$tag" -num-blocks "$nb" -fmas "$fmas")"
    [[ -n "$kt" ]] && echo "fp32_throughput,fmas_per_thread,${fmas},${nb},256,${fmas},,${kt}" >> "$OUT_ABS"
  done
done

echo "Wrote $OUT_ABS"
cat "$OUT_ABS"
