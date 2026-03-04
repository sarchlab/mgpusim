#!/bin/bash
# run_all.sh — Run all 22 HIP benchmarks and collect results into results.csv
#
# Usage:
#   ./run_all.sh                    # run with default 10 iterations
#   ./run_all.sh --iterations 50    # run with 50 iterations
#   ./run_all.sh --output out.csv   # write to out.csv instead of results.csv

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

ITERATIONS=10
OUTPUT="results.csv"

# Parse arguments
while [ $# -gt 0 ]; do
    case "$1" in
        --iterations)
            ITERATIONS="$2"
            shift 2
            ;;
        --output)
            OUTPUT="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1" >&2
            echo "Usage: $0 [--iterations N] [--output FILE]" >&2
            exit 1
            ;;
    esac
done

BENCHMARKS=(
    vectoradd
    memcopy
    matrixtranspose
    floydwarshall
    fastwalshtransform
    fir
    simpleconvolution
    bitonicsort
    kmeans
    atax
    bicg
    relu
    pagerank
    stencil2d
    bfs
    nw
    fft
    spmv
    matrixmultiplication
    nbody
    conv2d
    im2col
)

# Write CSV header
echo "kernel_name,problem_size,iterations,avg_ms,min_ms,max_ms" > "$OUTPUT"

pass=0
fail=0
skip=0

for name in "${BENCHMARKS[@]}"; do
    if [ ! -x "./${name}" ]; then
        echo "[SKIP] ./${name} not found or not executable"
        skip=$((skip + 1))
        continue
    fi

    echo "[RUN]  ${name} (${ITERATIONS} iterations)"
    # Each benchmark prints CSV header + data rows. We skip the header line.
    if output=$(./"${name}" --iterations "${ITERATIONS}" 2>&1); then
        # Strip the CSV header line ("kernel_name,...") and append data rows
        echo "$output" | grep -v '^kernel_name' >> "$OUTPUT"
        pass=$((pass + 1))
    else
        echo "[FAIL] ${name}"
        fail=$((fail + 1))
    fi
done

echo ""
echo "Run summary: ${pass} passed, ${fail} failed, ${skip} skipped (of ${#BENCHMARKS[@]} total)"
echo "Results written to: ${OUTPUT}"
