#!/bin/bash
# run_all.sh -- Execute all benchmarks, collect timing results to SQLite + CSV
#
# Usage:
#   ./run_all.sh                              # defaults: 10 iters, 120s timeout
#   ./run_all.sh --iterations 5
#   ./run_all.sh --timeout 60
#   ./run_all.sh --suite polybench            # run only one suite
#   ./run_all.sh --output results_dir         # output directory
#   ./run_all.sh --db results.sqlite3         # SQLite database path

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
WORKLOADS_DIR="$(dirname "$SCRIPT_DIR")"

ITERATIONS=10
TIMEOUT=120
SUITE=""
OUTPUT_DIR="$WORKLOADS_DIR/results"
DB_PATH=""
RUN_ID="$(date +%Y%m%d-%H%M%S)-$(hostname -s 2>/dev/null || hostname)"

while [ $# -gt 0 ]; do
    case "$1" in
        --iterations) ITERATIONS="$2"; shift 2 ;;
        --timeout)    TIMEOUT="$2"; shift 2 ;;
        --suite)      SUITE="$2"; shift 2 ;;
        --output)     OUTPUT_DIR="$2"; shift 2 ;;
        --db)         DB_PATH="$2"; shift 2 ;;
        -h|--help)
            echo "Usage: $0 [--iterations N] [--timeout SECS] [--suite NAME]"
            echo "          [--output DIR] [--db FILE]"
            exit 0
            ;;
        *) echo "Unknown option: $1" >&2; exit 1 ;;
    esac
done

mkdir -p "$OUTPUT_DIR"
DB_PATH="${DB_PATH:-$OUTPUT_DIR/results.sqlite3}"

if command -v timeout >/dev/null 2>&1; then
    TIMEOUT_BIN="timeout"
elif command -v gtimeout >/dev/null 2>&1; then
    TIMEOUT_BIN="gtimeout"
else
    TIMEOUT_BIN=""
    echo "[WARN] timeout command not found; per-benchmark timeout is disabled" >&2
fi

# --- Detect platform ---
if command -v hipcc &>/dev/null; then
    PLATFORM="hip"
elif command -v nvcc &>/dev/null; then
    PLATFORM="cuda"
else
    echo "ERROR: No GPU runtime found (hipcc or nvcc required)" >&2
    exit 1
fi

echo "=== Run Configuration ==="
echo "  Run ID:     $RUN_ID"
echo "  Platform:   $PLATFORM"
echo "  Iterations: $ITERATIONS"
echo "  Timeout:    ${TIMEOUT}s"
echo "  Output:     $OUTPUT_DIR"
echo "  Database:   $DB_PATH"
echo ""

# --- Collect system info ---
SYSINFO_FILE="$OUTPUT_DIR/sysinfo_${RUN_ID}.json"
bash "$SCRIPT_DIR/collect_sysinfo.sh" --run-id "$RUN_ID" --output "$SYSINFO_FILE"

# --- Initialize SQLite DB ---
sqlite3 "$DB_PATH" < "$SCRIPT_DIR/schema.sql"

# --- Insert run record from sysinfo ---
if command -v python3 &>/dev/null && [ -f "$SYSINFO_FILE" ]; then
    python3 - "$SYSINFO_FILE" "$DB_PATH" <<'PYEOF' || echo "[WARN] Could not insert run record into SQLite"
import json, sys, sqlite3 as s3
sysinfo_file = sys.argv[1]
db_path = sys.argv[2]
with open(sysinfo_file, encoding='utf-8') as f:
    info = json.load(f)
sql = '''INSERT OR REPLACE INTO runs
    (run_id, run_timestamp, hostname, os_name, kernel_version,
     cpu_model, cpu_cores, ram_gb, gpu_model, gpu_count, gpu_vram_gb,
     gpu_driver, compiler, compiler_version, runtime_version, gpu_arch,
     platform)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);'''
vals = [
    info.get('run_id',''), info.get('timestamp',''), info.get('hostname',''),
    info.get('os',''), info.get('kernel',''),
    info.get('cpu',{}).get('model',''), info.get('cpu',{}).get('cores',0),
    info.get('ram_gb',0),
    info.get('gpu',{}).get('model',''), info.get('gpu',{}).get('count',0),
    info.get('gpu',{}).get('vram_gb',0), info.get('gpu',{}).get('driver',''),
    info.get('compiler',''), info.get('compiler_version',''),
    info.get('runtime',''), info.get('gpu',{}).get('arch',''),
    info.get('platform','')
]
db = s3.connect(db_path)
db.execute(sql, vals)
db.commit()
db.close()
PYEOF
fi

append_csv_row() {
    local csv_path="$1"
    shift

    python3 - "$csv_path" "$@" <<'PYEOF'
import csv
import sys

path = sys.argv[1]
fields = sys.argv[2:]

with open(path, 'a', newline='', encoding='utf-8') as out:
    csv.writer(out).writerow(fields)
PYEOF
}

append_result_row() {
    local suite="$1"
    local bench="$2"
    local problem_size="$3"
    local iterations="$4"
    local total_avg_ms="$5"
    local total_min_ms="$6"
    local total_max_ms="$7"
    local total_stddev_ms="$8"
    local status="$9"

    append_csv_row "$CSV_FILE" \
        "$RUN_ID" "$suite" "$bench" "$PLATFORM" "$problem_size" "$iterations" \
        "$total_avg_ms" "$total_min_ms" "$total_max_ms" "$total_stddev_ms" "$status"
}

append_success_rows() {
    local suite="$1"
    local bench="$2"
    local args="$3"
    local output="$4"

    BENCHMARK_OUTPUT="$output" python3 - "$CSV_FILE" "$KERNEL_CSV" "$RUN_ID" "$suite" "$bench" "$PLATFORM" "$args" "$ITERATIONS" <<'PYEOF'
import csv
import io
import math
import sys

results_csv, kernel_csv, run_id, suite, bench, platform, arg_problem_size, default_iterations = sys.argv[1:]


def to_float(val):
    try:
        return float(val)
    except (ValueError, TypeError):
        return 0.0


def unique_ordered(items):
    out = []
    for item in items:
        if item not in out:
            out.append(item)
    return out

import os

raw_output = os.environ.get('BENCHMARK_OUTPUT', '')
reader = csv.reader(io.StringIO(raw_output))
benchmark_rows = []

for row in reader:
    if not row:
        continue

    first = row[0].strip()
    if not first or first.startswith('#') or first == 'kernel_name':
        continue

    if len(row) < 7:
        continue

    kernel_name, problem_size, iterations, avg_ms, min_ms, max_ms, stddev_ms = [cell.strip() for cell in row[:7]]
    benchmark_rows.append((kernel_name, problem_size, iterations, avg_ms, min_ms, max_ms, stddev_ms))

with open(kernel_csv, 'a', newline='', encoding='utf-8') as kf:
    kw = csv.writer(kf)
    for kernel_name, problem_size, iterations, avg_ms, min_ms, max_ms, stddev_ms in benchmark_rows:
        kw.writerow([
            run_id,
            suite,
            bench,
            platform,
            problem_size,
            kernel_name,
            iterations,
            avg_ms,
            min_ms,
            max_ms,
            stddev_ms,
        ])

if benchmark_rows:
    problem_sizes = unique_ordered([row[1] for row in benchmark_rows if row[1]])
    if len(problem_sizes) == 1:
        result_problem_size = problem_sizes[0]
    elif len(problem_sizes) > 1:
        result_problem_size = ';'.join(problem_sizes)
    else:
        result_problem_size = arg_problem_size.strip()

    result_iterations = benchmark_rows[0][2] if benchmark_rows[0][2] else default_iterations

    total_avg = sum(to_float(row[3]) for row in benchmark_rows)
    total_min = sum(to_float(row[4]) for row in benchmark_rows)
    total_max = sum(to_float(row[5]) for row in benchmark_rows)
    total_stddev = math.sqrt(sum(to_float(row[6]) ** 2 for row in benchmark_rows))

    totals = [f"{total_avg:.4f}", f"{total_min:.4f}", f"{total_max:.4f}", f"{total_stddev:.4f}"]
else:
    result_problem_size = arg_problem_size.strip()
    result_iterations = default_iterations
    totals = ['', '', '', '']

with open(results_csv, 'a', newline='', encoding='utf-8') as rf:
    rw = csv.writer(rf)
    rw.writerow([
        run_id,
        suite,
        bench,
        platform,
        result_problem_size,
        result_iterations,
        totals[0],
        totals[1],
        totals[2],
        totals[3],
        'success',
    ])
PYEOF
}

# --- CSV output files ---
CSV_FILE="$OUTPUT_DIR/results_${RUN_ID}.csv"
echo "run_id,suite,benchmark,platform,problem_size,iterations,total_avg_ms,total_min_ms,total_max_ms,total_stddev_ms,status" \
    > "$CSV_FILE"

KERNEL_CSV="$OUTPUT_DIR/kernel_timings_${RUN_ID}.csv"
echo "run_id,suite,benchmark,platform,problem_size,kernel_name,iterations,avg_ms,min_ms,max_ms,stddev_ms" \
    > "$KERNEL_CSV"

# --- Counters ---
pass=0
fail=0
skip=0
timedout=0

run_benchmark() {
    local suite="$1" bench="$2" binary="$3" args="$4"

    if [ ! -x "$binary" ]; then
        echo "[SKIP] $suite/$bench -- binary not found: $binary"
        skip=$((skip + 1))
        append_result_row "$suite" "$bench" "$args" "$ITERATIONS" "" "" "" "" "skip"
        return
    fi

    echo -n "[RUN]  $suite/$bench"
    [ -n "$args" ] && echo -n " $args"
    echo ""

    local exit_code=0
    local output
    local cmd=("$binary" "--iterations" "$ITERATIONS")

    if [ -n "$args" ]; then
        local arg_array=()
        read -r -a arg_array <<< "$args"
        cmd+=("${arg_array[@]}")
    fi

    if [ -n "$TIMEOUT_BIN" ]; then
        output=$($TIMEOUT_BIN "${TIMEOUT}" "${cmd[@]}" 2>&1) || exit_code=$?
    else
        output=$("${cmd[@]}" 2>&1) || exit_code=$?
    fi

    if [ $exit_code -eq 0 ]; then
        append_success_rows "$suite" "$bench" "$args" "$output"
        pass=$((pass + 1))
        echo "[PASS] $suite/$bench"
    elif [ $exit_code -eq 124 ]; then
        echo "[TIMEOUT] $suite/$bench (>${TIMEOUT}s)"
        append_result_row "$suite" "$bench" "$args" "$ITERATIONS" "" "" "" "" "timeout"
        timedout=$((timedout + 1))
    else
        echo "[FAIL] $suite/$bench (exit $exit_code)"
        append_result_row "$suite" "$bench" "$args" "$ITERATIONS" "" "" "" "" "fail"
        fail=$((fail + 1))
    fi
}

# --- Iterate over suites and benchmarks ---
SUITES="${SUITE:-polybench shoc heteromark rodinia parboil lonestar microbench ml_kernels}"

for suite in $SUITES; do
    suite_dir="$WORKLOADS_DIR/$suite"
    [ -d "$suite_dir" ] || continue

    echo ""
    echo "=== Suite: $suite ==="

    for bench_dir in "$suite_dir"/*/; do
        [ -d "$bench_dir" ] || continue
        bench=$(basename "$bench_dir")

        # Skip benchmarks with DISABLED.md
        if [ -f "$bench_dir/DISABLED.md" ]; then
            echo "[SKIP] $suite/$bench -- disabled (see DISABLED.md)"
            skip=$((skip + 1))
            append_result_row "$suite" "$bench" "" "$ITERATIONS" "" "" "" "" "skip"
            continue
        fi

        # Auto-discover binary (Makefiles may produce lowercase names)
        binary=$(find "$bench_dir/$PLATFORM" -maxdepth 1 -type f -executable \
            ! -name '*.cpp' ! -name '*.h' ! -name 'Makefile' 2>/dev/null | head -1)

        # Read parameter sweeps from params.json
        if [ -f "$bench_dir/params.json" ]; then
            param_args=$(python3 - "$bench_dir/params.json" <<'PARAMEOF'
import json, sys
with open(sys.argv[1], encoding='utf-8') as f:
    p = json.load(f)
sp = p.get('scaling_parameter', {})
name = sp.get('name', '')
values = sp.get('values', [])
if name and values:
    for v in values:
        print('--' + name + ' ' + str(v))
PARAMEOF
            ) || param_args=""
            if [ -n "$param_args" ]; then
                while IFS= read -r args; do
                    [ -z "$args" ] && continue
                    run_benchmark "$suite" "$bench" "$binary" "$args"
                done <<< "$param_args"
            else
                run_benchmark "$suite" "$bench" "$binary" ""
            fi
        else
            run_benchmark "$suite" "$bench" "$binary" ""
        fi
    done
done

echo ""
echo "=== Run Summary ==="
echo "  Passed:    $pass"
echo "  Failed:    $fail"
echo "  Timed out: $timedout"
echo "  Skipped:   $skip"
echo "  Results:   $CSV_FILE"
echo "  Kernels:   $KERNEL_CSV"
echo "  Database:  $DB_PATH"
echo "  Sysinfo:   $SYSINFO_FILE"
