#!/usr/bin/env bash
# =============================================================================
# run_tier1_hardware.sh -- Collect tier-1 timing data on real MI300A hardware
# =============================================================================
#
# Builds the tier-1 workload binaries (HIP / hipcc) and runs each (benchmark,
# problem_size) pair from benchmark-comparison/mi300a_problem_size_discovery_plan.json
# (tier == 1 entries). Each binary already times its kernel with hipEvents and
# emits a CSV row of the form:
#
#   kernel_name,problem_size,iterations,avg_ms,min_ms,max_ms,stddev_ms
#
# This script aggregates those rows into two CSV outputs:
#
#   tier1_hardware_timings_<TS>.csv  -- benchmark,problem_size,iterations,
#                                        avg_ms,min_ms,max_ms
#   tier1_comparison_ready.csv       -- kernel_name,problem_size,real_ms
#
# Usage (from repo root):
#   bash scripts/run_tier1_hardware.sh
#   bash scripts/run_tier1_hardware.sh --iterations 5
#   bash scripts/run_tier1_hardware.sh --skip-compute-bound
#   bash scripts/run_tier1_hardware.sh --output-dir /tmp/tier1_hw
#
# Requirements: hipcc (ROCm toolchain), make, python3, bash 4+.
# =============================================================================

set -euo pipefail

# ---- Argument parsing -------------------------------------------------------
ITERATIONS=10
OUTPUT_DIR=""
SKIP_COMPUTE_BOUND=0
PER_RUN_TIMEOUT_SEC=300

usage() {
    cat <<USAGE
Usage: $0 [--iterations N] [--output-dir DIR] [--skip-compute-bound]
          [--per-run-timeout SECS]

  --iterations N         Iterations per (benchmark,size). Default: 10.
  --output-dir DIR       Where to write CSV outputs. Default:
                         workloads/results/tier1_hardware_<TS>/.
  --skip-compute-bound   Skip fp32_fma, fp64_fma, int_mad, sfun_sin,
                         atomic_throughput (these take 4-7h/size in the
                         simulator; on real HW they finish quickly, but the
                         flag exists to mirror simulator-side runs).
  --per-run-timeout S    Wall-clock cap per binary invocation. Default: 300s.
USAGE
}

while [ $# -gt 0 ]; do
    case "$1" in
        --iterations)        ITERATIONS="$2"; shift 2 ;;
        --output-dir)        OUTPUT_DIR="$2"; shift 2 ;;
        --skip-compute-bound) SKIP_COMPUTE_BOUND=1; shift ;;
        --per-run-timeout)   PER_RUN_TIMEOUT_SEC="$2"; shift 2 ;;
        -h|--help)           usage; exit 0 ;;
        *) echo "ERROR: unknown option: $1" >&2; usage; exit 2 ;;
    esac
done

if ! [[ "$ITERATIONS" =~ ^[0-9]+$ ]] || [ "$ITERATIONS" -eq 0 ]; then
    echo "ERROR: --iterations must be a positive integer, got: $ITERATIONS" >&2
    exit 2
fi

# ---- Paths ------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
WORKLOADS_DIR="$REPO_ROOT/workloads"
PLAN_JSON="$REPO_ROOT/benchmark-comparison/mi300a_problem_size_discovery_plan.json"
TS="$(date +%Y%m%d-%H%M%S)"
HOST_TAG="$(hostname -s 2>/dev/null || hostname || echo unknown)"

if [ -z "$OUTPUT_DIR" ]; then
    OUTPUT_DIR="$WORKLOADS_DIR/results/tier1_hardware_${TS}"
fi
mkdir -p "$OUTPUT_DIR"

TIMINGS_CSV="$OUTPUT_DIR/tier1_hardware_timings_${TS}.csv"
COMPARISON_CSV="$OUTPUT_DIR/tier1_comparison_ready.csv"
RAW_LOG="$OUTPUT_DIR/raw_runs_${TS}.log"

# ---- Pre-flight checks ------------------------------------------------------
if ! command -v hipcc >/dev/null 2>&1; then
    cat >&2 <<EOF
ERROR: hipcc not found on PATH.
       This script must be run on a machine with the ROCm toolchain installed
       (e.g. an MI300A node). Install ROCm or load the appropriate module
       before running.
EOF
    exit 1
fi

if [ ! -f "$PLAN_JSON" ]; then
    echo "ERROR: plan JSON not found: $PLAN_JSON" >&2
    exit 1
fi

# Find timeout binary (Linux: timeout; macOS: gtimeout via coreutils).
TIMEOUT_BIN=""
if command -v timeout >/dev/null 2>&1; then
    TIMEOUT_BIN="timeout"
elif command -v gtimeout >/dev/null 2>&1; then
    TIMEOUT_BIN="gtimeout"
fi

# ---- Benchmark mapping table -----------------------------------------------
# Each record:
#   plan_name|workload_subdir|binary_name|flag_name|size_unit
#
# size_unit indicates how to translate the plan's problem_size string into
# the integer the binary expects:
#   raw  -- pass the value as-is (already a number)
#   kb   -- strip "KB" suffix (binary takes --transfer_size_kb)
#   mb   -- strip "MB" suffix (binary takes --array_size_mb)
#
# devicememory_read and devicememory_write share the same binary which emits
# both DeviceMemory_Read and DeviceMemory_Write CSV rows in a single run.
# To avoid running it twice we use a "combo" flag (see run_devicememory()).

read -r -d '' BENCH_TABLE <<'TABLE' || true
atomic_throughput|microbench/atomic_throughput|atomic_throughput|--num_threads|raw
branch_div_50pct|microbench/branch_divergence|branch_divergence|--num_elements|raw
busspeeddownload|shoc/BusSpeedDownload|busspeeddownload|--transfer_size_kb|kb
busspeedreadback|shoc/BusSpeedReadback|busspeedreadback|--transfer_size_kb|kb
fp32_fma|microbench/fp32_throughput|fp32_throughput|--num_ops|raw
fp64_fma|microbench/fp64_throughput|fp64_throughput|--num_ops|raw
global_bw_copy|microbench/global_bandwidth|global_bandwidth|--array_size|raw
int_mad|microbench/int_throughput|int_throughput|--num_ops|raw
l1_cache_bw|microbench/l1_cache_bw|l1_cache_bw|--num_elements|raw
l2_cache_bw|microbench/l2_cache_bw|l2_cache_bw|--num_elements|raw
maxflops|shoc/MaxFlops|maxflops|--num_threads|raw
mem_latency_chase|microbench/mem_latency|mem_latency|--array_size|raw
occupancy_fma|microbench/occupancy_sweep|occupancy_sweep|--num_elements|raw
reduction|shoc/Reduction|reduction|--size|raw
sfun_sin|microbench/sfun_throughput|sfun_throughput|--num_ops|raw
shared_bw|microbench/shared_bandwidth|shared_bandwidth|--array_size|raw
triad|shoc/Triad|triad|--array_size|raw
TABLE

# DeviceMemory is handled separately because its binary emits 2 kernel rows.
DEVMEM_SUBDIR="shoc/DeviceMemory"
DEVMEM_BINARY="devicememory"
DEVMEM_FLAG="--array_size_mb"

COMPUTE_BOUND_SET=" atomic_throughput fp32_fma fp64_fma int_mad sfun_sin "

# Fail before collecting rows if the cache-bandwidth table or checked-in plan
# regresses to the old cache-residency axis. The output CSV has only
# problem_size/real_ms columns, so stale L1/L2 flags would otherwise be hard to
# distinguish after collection.
validate_cache_bw_axis_contract() {
    BENCH_TABLE_TEXT="$BENCH_TABLE" PLAN_JSON="$PLAN_JSON" python3 - <<'PYEOF'
import json
import os
import sys

expected_runner_flags = {
    "l1_cache_bw": "--num_elements",
    "l2_cache_bw": "--num_elements",
}
expected_plan_flag = "-num_elements {size}"

bench_table = {}
for line in os.environ.get("BENCH_TABLE_TEXT", "").splitlines():
    if not line.strip():
        continue
    fields = line.split("|")
    if len(fields) == 5:
        bench_table[fields[0]] = fields[3]

errors = []
for benchmark, expected_flag in expected_runner_flags.items():
    actual_flag = bench_table.get(benchmark)
    if actual_flag != expected_flag:
        errors.append(
            f"{benchmark}: BENCH_TABLE flag {actual_flag!r} != {expected_flag!r}"
        )

with open(os.environ["PLAN_JSON"], encoding="utf-8") as fh:
    plan = json.load(fh)

for benchmark in expected_runner_flags:
    rows = [
        row for row in plan.get("entries", [])
        if row.get("tier") == 1 and row.get("benchmark") == benchmark
    ]
    if not rows:
        errors.append(f"{benchmark}: no tier-1 plan rows found")
        continue
    stale = [
        (row.get("plan_index"), row.get("flag_template"))
        for row in rows
        if row.get("flag_template") != expected_plan_flag
    ]
    if stale:
        preview = ", ".join(f"{idx}:{flag}" for idx, flag in stale[:5])
        errors.append(f"{benchmark}: stale plan flag_template rows: {preview}")

if errors:
    print("ERROR: cache-bandwidth axis contract failed:", file=sys.stderr)
    for error in errors:
        print(f"  - {error}", file=sys.stderr)
    sys.exit(1)
PYEOF
}

validate_cache_bw_axis_contract

# ---- Banner -----------------------------------------------------------------
echo "============================================================"
echo " Tier-1 Hardware Timing Collection"
echo "============================================================"
echo " Repo root          : $REPO_ROOT"
echo " Plan JSON          : $PLAN_JSON"
echo " Iterations / size  : $ITERATIONS"
echo " Per-run timeout    : ${PER_RUN_TIMEOUT_SEC}s"
echo " Skip compute-bound : $SKIP_COMPUTE_BOUND"
echo " Output directory   : $OUTPUT_DIR"
echo " Host               : $HOST_TAG"
echo " Timestamp          : $TS"
echo "============================================================"

# ---- Build all tier-1 workloads --------------------------------------------
echo ""
echo "[BUILD] make all PLATFORM=hip in workloads/microbench"
if ! make -C "$WORKLOADS_DIR/microbench" all PLATFORM=hip 2>&1 | tee -a "$RAW_LOG"; then
    echo "[WARN] microbench build returned non-zero (some benchmarks may have failed)" >&2
fi

echo ""
echo "[BUILD] make all PLATFORM=hip in workloads/shoc"
if ! make -C "$WORKLOADS_DIR/shoc" all PLATFORM=hip 2>&1 | tee -a "$RAW_LOG"; then
    echo "[WARN] shoc build returned non-zero (some benchmarks may have failed)" >&2
fi

# ---- Helpers ----------------------------------------------------------------

# Translate plan problem_size token to integer for the binary's flag.
size_to_int() {
    local size="$1"
    local unit="$2"
    case "$unit" in
        raw) echo "$size" ;;
        kb)  echo "${size%KB}" ;;
        mb)  echo "${size%MB}" ;;
        *)   echo "ERROR: size_to_int: unknown unit '${unit}' for size '${size}'" >&2; exit 1 ;;
    esac
}

# Read tier-1 sizes for a given benchmark from the plan JSON, one per line.
plan_sizes_for() {
    local bench="$1"
    python3 - "$PLAN_JSON" "$bench" <<'PYEOF'
import json, sys
plan_path, bench = sys.argv[1], sys.argv[2]
with open(plan_path, encoding='utf-8') as f:
    plan = json.load(f)
for e in plan.get('entries', []):
    if e.get('tier') == 1 and e.get('benchmark') == bench:
        print(e.get('problem_size', ''))
PYEOF
}

# Run a single (binary, size, flag) invocation, capture CSV rows from stdout,
# and append normalized rows to the two output CSVs.
#
# Args:
#   $1 plan_bench_name (matches expected kernel_name; for devicememory we pass
#                       both kernel names in a comma-separated list).
#   $2 problem_size token from plan (used as the "problem_size" column)
#   $3 binary path
#   $4 flag name (e.g. --array_size)
#   $5 flag value (already translated to int)
#   $6 expected kernel_name regex (one of plan_bench or comma-separated list)
run_one() {
    local plan_bench="$1"
    local problem_size="$2"
    local binary="$3"
    local flag="$4"
    local flag_value="$5"
    local expected_kernels="$6"

    if [ ! -x "$binary" ]; then
        echo "[SKIP] $plan_bench size=$problem_size: binary not built ($binary)"
        SKIPPED=$((SKIPPED + 1))
        return
    fi

    echo "[RUN ] $plan_bench size=$problem_size  ($(basename "$binary") --iterations $ITERATIONS $flag $flag_value)"

    local cmd=("$binary" "--iterations" "$ITERATIONS" "$flag" "$flag_value")
    local exit_code=0
    local output

    if [ -n "$TIMEOUT_BIN" ]; then
        output=$("$TIMEOUT_BIN" "$PER_RUN_TIMEOUT_SEC" "${cmd[@]}" 2>>"$RAW_LOG") || exit_code=$?
    else
        output=$("${cmd[@]}" 2>>"$RAW_LOG") || exit_code=$?
    fi

    if [ "$exit_code" -eq 124 ]; then
        echo "[TIME] $plan_bench size=$problem_size: timed out after ${PER_RUN_TIMEOUT_SEC}s"
        TIMEDOUT=$((TIMEDOUT + 1))
        return
    fi
    if [ "$exit_code" -ne 0 ]; then
        echo "[FAIL] $plan_bench size=$problem_size: exit $exit_code"
        FAILED=$((FAILED + 1))
        return
    fi

    # Append raw output for provenance.
    {
        echo "### $plan_bench size=$problem_size flag=$flag $flag_value"
        echo "$output"
    } >>"$RAW_LOG"

    # Parse the CSV row(s) and emit normalized rows.
    PARSE_OUT=$(
        BENCH_OUTPUT="$output" \
        PLAN_BENCH="$plan_bench" \
        PROBLEM_SIZE="$problem_size" \
        ITERATIONS="$ITERATIONS" \
        EXPECTED_KERNELS="$expected_kernels" \
        TIMINGS_CSV="$TIMINGS_CSV" \
        COMPARISON_CSV="$COMPARISON_CSV" \
        python3 - <<'PYEOF'
import csv
import io
import os
import sys

raw = os.environ.get('BENCH_OUTPUT', '')
plan_bench = os.environ['PLAN_BENCH']
problem_size = os.environ['PROBLEM_SIZE']
iterations = os.environ['ITERATIONS']
expected = [k.strip() for k in os.environ['EXPECTED_KERNELS'].split(',') if k.strip()]
timings_csv = os.environ['TIMINGS_CSV']
comparison_csv = os.environ['COMPARISON_CSV']

# Parse CSV rows from stdout. The runtime emits a header "kernel_name,..."
# followed by one or more data rows.
reader = csv.reader(io.StringIO(raw))
matched = []
for row in reader:
    if not row:
        continue
    first = row[0].strip()
    if not first or first == 'kernel_name' or first.startswith('#'):
        continue
    if len(row) < 7:
        continue
    kernel_name = first
    # Match against the expected kernel name(s).
    if expected and kernel_name not in expected:
        continue
    matched.append(row)

if not matched:
    print('PARSE_FAIL', file=sys.stderr)
    sys.exit(3)

with open(timings_csv, 'a', newline='', encoding='utf-8') as tf, \
     open(comparison_csv, 'a', newline='', encoding='utf-8') as cf:
    tw = csv.writer(tf)
    cw = csv.writer(cf)
    for row in matched:
        kernel_name = row[0].strip()
        # Some binaries emit their own problem_size string (e.g. "64MB").
        # We always use the plan's problem_size for both columns so the
        # comparison pipeline can match on it directly.
        try:
            avg_ms = float(row[3]); min_ms = float(row[4]); max_ms = float(row[5])
        except ValueError:
            continue
        # tier1_hardware_timings: benchmark identifies the workload, kernel
        # in the second/comparison file is the kernel_name (matching the
        # simulator-side comparison key).
        bench_label = plan_bench
        # For DeviceMemory, the plan distinguishes read vs write but the
        # binary emits both kernels in one run; map kernel->plan name.
        if kernel_name == 'DeviceMemory_Read':
            bench_label = 'devicememory_read'
        elif kernel_name == 'DeviceMemory_Write':
            bench_label = 'devicememory_write'
        tw.writerow([bench_label, problem_size, iterations,
                     f'{avg_ms:.6f}', f'{min_ms:.6f}', f'{max_ms:.6f}'])
        cw.writerow([kernel_name, problem_size, f'{avg_ms:.6f}'])

print(f'OK {len(matched)}')
PYEOF
    )
    parse_rc=$?

    if [ $parse_rc -ne 0 ] || [ -z "$PARSE_OUT" ]; then
        echo "[WARN] $plan_bench size=$problem_size: could not parse CSV output (see $RAW_LOG)"
        FAILED=$((FAILED + 1))
        return
    fi
    PASSED=$((PASSED + 1))
}

# Run DeviceMemory once per size (it emits both Read and Write rows).
run_devicememory() {
    local sizes_read sizes_write
    sizes_read="$(plan_sizes_for devicememory_read)"
    sizes_write="$(plan_sizes_for devicememory_write)"
    # Use the union (typically identical).
    local all_sizes
    all_sizes="$(printf '%s\n%s\n' "$sizes_read" "$sizes_write" | awk 'NF' | sort -u)"

    if [ -z "$all_sizes" ]; then
        return
    fi

    local binary="$WORKLOADS_DIR/$DEVMEM_SUBDIR/hip/$DEVMEM_BINARY"
    if [ ! -x "$binary" ]; then
        echo "[SKIP] devicememory_read/write: binary not built ($binary)"
        return
    fi

    while IFS= read -r ps; do
        [ -z "$ps" ] && continue
        local mb
        mb="$(size_to_int "$ps" mb)"
        run_one "devicememory" "$ps" "$binary" "$DEVMEM_FLAG" "$mb" \
                "DeviceMemory_Read,DeviceMemory_Write"
    done <<<"$all_sizes"
}

# ---- Initialise CSV outputs -------------------------------------------------
echo "benchmark,problem_size,iterations,avg_ms,min_ms,max_ms" > "$TIMINGS_CSV"
echo "kernel_name,problem_size,real_ms" > "$COMPARISON_CSV"

PASSED=0
FAILED=0
SKIPPED=0
TIMEDOUT=0

# ---- Iterate plan -----------------------------------------------------------
echo ""
echo "============================================================"
echo " Running benchmarks"
echo "============================================================"

while IFS='|' read -r plan_bench subdir binary_name flag size_unit; do
    [ -z "$plan_bench" ] && continue

    if [ "$SKIP_COMPUTE_BOUND" -eq 1 ] && [[ "$COMPUTE_BOUND_SET" == *" $plan_bench "* ]]; then
        echo "[SKIP] $plan_bench: --skip-compute-bound active"
        continue
    fi

    binary="$WORKLOADS_DIR/$subdir/hip/$binary_name"
    if [ ! -x "$binary" ]; then
        echo "[WARN] $plan_bench: binary missing ($binary); skipping all sizes"
        SKIPPED=$((SKIPPED + 1))
        continue
    fi

    sizes="$(plan_sizes_for "$plan_bench")"
    if [ -z "$sizes" ]; then
        echo "[WARN] $plan_bench: no tier-1 sizes in plan; skipping"
        SKIPPED=$((SKIPPED + 1))
        continue
    fi

    while IFS= read -r ps; do
        [ -z "$ps" ] && continue
        flag_value="$(size_to_int "$ps" "$size_unit")"
        run_one "$plan_bench" "$ps" "$binary" "$flag" "$flag_value" "$plan_bench"
    done <<<"$sizes"

done <<<"$BENCH_TABLE"

# DeviceMemory has its own driver because its binary emits two kernels per run.
run_devicememory

# ---- Summary ----------------------------------------------------------------
TOTAL=$((PASSED + FAILED + SKIPPED + TIMEDOUT))
echo ""
echo "============================================================"
echo " Summary"
echo "============================================================"
echo " Total invocations  : $TOTAL"
echo " Passed             : $PASSED"
echo " Failed             : $FAILED"
echo " Timed out          : $TIMEDOUT"
echo " Skipped            : $SKIPPED"
echo ""
echo " Hardware timings   : $TIMINGS_CSV"
echo " Comparison-ready   : $COMPARISON_CSV"
echo " Raw run log        : $RAW_LOG"
echo "============================================================"

if [ "$PASSED" -eq 0 ]; then
    echo "ERROR: no successful runs; outputs are empty." >&2
    exit 1
fi

exit 0
