#!/bin/bash
# run_all.sh — Run all 22 HIP benchmarks across a range of problem sizes
#
# Each benchmark is invoked multiple times with increasing sizes so that
# execution times span from sub-millisecond up to a few seconds.
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

# Write CSV header
echo "kernel_name,problem_size,iterations,avg_ms,min_ms,max_ms" > "$OUTPUT"

pass=0
fail=0
skip=0

# run_bench NAME ARGS...
# Runs ./NAME with --iterations ITERATIONS plus each size args string in turn.
run_bench() {
    local name="$1"
    shift
    if [ ! -x "./${name}" ]; then
        echo "[SKIP] ./${name} not found or not executable"
        skip=$((skip + 1))
        return
    fi
    for args in "$@"; do
        echo "[RUN]  ${name} ${args} (${ITERATIONS} iterations)"
        # shellcheck disable=SC2086
        if output=$(./"${name}" --iterations "${ITERATIONS}" ${args} 2>&1); then
            echo "$output" | grep -v '^kernel_name' >> "$OUTPUT"
            pass=$((pass + 1))
        else
            echo "[FAIL] ${name} ${args}"
            fail=$((fail + 1))
        fi
    done
}

# ---------------------------------------------------------------------------
# vectoradd  --size N  (N total elements; any positive integer)
# ---------------------------------------------------------------------------
run_bench vectoradd \
    "--size 1024" \
    "--size 4096" \
    "--size 16384" \
    "--size 65536" \
    "--size 262144" \
    "--size 1048576"

# ---------------------------------------------------------------------------
# memcopy  --size N  (N megabytes)
# ---------------------------------------------------------------------------
run_bench memcopy \
    "--size 1" \
    "--size 4" \
    "--size 16" \
    "--size 64" \
    "--size 256"

# ---------------------------------------------------------------------------
# matrixtranspose  --size N  (NxN matrix; N must be a multiple of 64)
# ---------------------------------------------------------------------------
run_bench matrixtranspose \
    "--size 256" \
    "--size 512" \
    "--size 1024" \
    "--size 2048" \
    "--size 4096"

# ---------------------------------------------------------------------------
# floydwarshall  --nodes N  (N nodes; N must be a multiple of 8)
# Complexity is O(N^3) kernel launches so sizes are kept modest.
# ---------------------------------------------------------------------------
run_bench floydwarshall \
    "--nodes 64" \
    "--nodes 128" \
    "--nodes 256" \
    "--nodes 512"

# ---------------------------------------------------------------------------
# fastwalshtransform  --size N  (N elements; must be a power of 2, >= 512)
# ---------------------------------------------------------------------------
run_bench fastwalshtransform \
    "--size 1024" \
    "--size 4096" \
    "--size 16384" \
    "--size 65536" \
    "--size 262144"

# ---------------------------------------------------------------------------
# fir  --length N  (N input samples; N must be a multiple of 256)
# ---------------------------------------------------------------------------
run_bench fir \
    "--length 2048" \
    "--length 8192" \
    "--length 32768" \
    "--length 131072" \
    "--length 524288"

# ---------------------------------------------------------------------------
# simpleconvolution  --size N  (NxN image with 3x3 mask)
# ---------------------------------------------------------------------------
run_bench simpleconvolution \
    "--size 128" \
    "--size 256" \
    "--size 512" \
    "--size 1024" \
    "--size 2048"

# ---------------------------------------------------------------------------
# bitonicsort  --size N  (N elements; must be a power of 2)
# ---------------------------------------------------------------------------
run_bench bitonicsort \
    "--size 1024" \
    "--size 4096" \
    "--size 16384" \
    "--size 65536" \
    "--size 262144"

# ---------------------------------------------------------------------------
# kmeans  --points N  (N data points; 32 features, 5 clusters fixed)
# ---------------------------------------------------------------------------
run_bench kmeans \
    "--points 256" \
    "--points 1024" \
    "--points 4096" \
    "--points 16384" \
    "--points 65536"

# ---------------------------------------------------------------------------
# atax  --size N  (NxN matrix)
# ---------------------------------------------------------------------------
run_bench atax \
    "--size 64" \
    "--size 128" \
    "--size 256" \
    "--size 512" \
    "--size 1024"

# ---------------------------------------------------------------------------
# bicg  --size N  (NxN matrix)
# ---------------------------------------------------------------------------
run_bench bicg \
    "--size 64" \
    "--size 128" \
    "--size 256" \
    "--size 512" \
    "--size 1024"

# ---------------------------------------------------------------------------
# relu  --size N  (N elements)
# ---------------------------------------------------------------------------
run_bench relu \
    "--size 16384" \
    "--size 65536" \
    "--size 262144" \
    "--size 1048576" \
    "--size 4194304"

# ---------------------------------------------------------------------------
# pagerank  --nodes N  (N graph nodes; N must be a multiple of 64)
# ---------------------------------------------------------------------------
run_bench pagerank \
    "--nodes 64" \
    "--nodes 256" \
    "--nodes 1024" \
    "--nodes 4096"

# ---------------------------------------------------------------------------
# stencil2d  --size N  (NxN grid; N must be a multiple of 64)
# ---------------------------------------------------------------------------
run_bench stencil2d \
    "--size 128" \
    "--size 256" \
    "--size 512" \
    "--size 1024"

# ---------------------------------------------------------------------------
# bfs  --nodes N  (N graph nodes)
# ---------------------------------------------------------------------------
run_bench bfs \
    "--nodes 256" \
    "--nodes 1024" \
    "--nodes 4096" \
    "--nodes 16384"

# ---------------------------------------------------------------------------
# nw  --size N  (sequence length; N must be a multiple of 64)
# Complexity is O(N^2) kernel launches.
# ---------------------------------------------------------------------------
run_bench nw \
    "--size 64" \
    "--size 128" \
    "--size 256" \
    "--size 512" \
    "--size 1024"

# ---------------------------------------------------------------------------
# fft  --size N  (N complex elements; N must be a multiple of 512)
# ---------------------------------------------------------------------------
run_bench fft \
    "--size 2048" \
    "--size 8192" \
    "--size 32768" \
    "--size 131072" \
    "--size 524288"

# ---------------------------------------------------------------------------
# spmv  --rows N  (N rows in sparse matrix; 10 non-zeros per row fixed)
# ---------------------------------------------------------------------------
run_bench spmv \
    "--rows 256" \
    "--rows 1024" \
    "--rows 4096" \
    "--rows 16384" \
    "--rows 65536"

# ---------------------------------------------------------------------------
# matrixmultiplication  --size N  (NxNxN GEMM; N must be a multiple of 32)
# O(N^3) arithmetic so sizes are kept modest.
# ---------------------------------------------------------------------------
run_bench matrixmultiplication \
    "--size 64" \
    "--size 128" \
    "--size 256" \
    "--size 512"

# ---------------------------------------------------------------------------
# nbody  --bodies N  (N particles; N must be a multiple of 256)
# O(N^2) work per step.
# ---------------------------------------------------------------------------
run_bench nbody \
    "--bodies 256" \
    "--bodies 1024" \
    "--bodies 4096" \
    "--bodies 16384"

# ---------------------------------------------------------------------------
# conv2d  --size N  (NxN input image; 3x3 kernel, 3 output channels fixed)
# ---------------------------------------------------------------------------
run_bench conv2d \
    "--size 28" \
    "--size 56" \
    "--size 112" \
    "--size 224" \
    "--size 448"

# ---------------------------------------------------------------------------
# im2col  --size N  (NxN input image; 3x3 kernel fixed)
# ---------------------------------------------------------------------------
run_bench im2col \
    "--size 28" \
    "--size 56" \
    "--size 112" \
    "--size 224" \
    "--size 448"

echo ""
echo "Run summary: ${pass} passed, ${fail} failed, ${skip} skipped"
echo "Results written to: ${OUTPUT}"
