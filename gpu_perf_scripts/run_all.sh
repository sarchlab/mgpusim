#!/bin/bash
# run_all.sh — Run all 22 HIP benchmarks across a range of problem sizes
#
# Sizes are chosen so that execution times span from sub-millisecond up to
# a few seconds.  Compute-intensive kernels (floydwarshall, bitonicsort,
# nbody, matrixmultiplication, atax, bicg, nw, pagerank) can reach the
# 1–5 s range.  Bandwidth-limited kernels are scaled to practical memory
# limits (~100–500 ms ceiling on MI300A).
#
# NOTE: rebuild all benchmarks with ./build_all.sh before running so that
#       the new CLI size parameters are active.
#
# Usage:
#   ./run_all.sh                    # 10 iterations per size
#   ./run_all.sh --iterations 5     # 5 iterations per size
#   ./run_all.sh --output out.csv   # write to out.csv

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

ITERATIONS=10
OUTPUT="results.csv"

while [ $# -gt 0 ]; do
    case "$1" in
        --iterations) ITERATIONS="$2"; shift 2 ;;
        --output)     OUTPUT="$2";     shift 2 ;;
        *) echo "Unknown option: $1" >&2; exit 1 ;;
    esac
done

echo "kernel_name,problem_size,iterations,avg_ms,min_ms,max_ms" > "$OUTPUT"

pass=0
fail=0
skip=0

# run_bench NAME ARGS...
# Each ARGS string (e.g. "--size 1024") is passed verbatim after word-splitting.
run_bench() {
    local name="$1"; shift
    if [ ! -x "./${name}" ]; then
        echo "[SKIP] ./${name} not found or not executable"
        skip=$((skip + 1))
        return
    fi
    for args in "$@"; do
        echo "[RUN]  ${name} ${args}"
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
# vectoradd  --size N  (N elements; any positive integer)
# Bandwidth-limited O(N).  Expected: ~0.05 ms → ~300 ms on MI300A.
# ---------------------------------------------------------------------------
run_bench vectoradd \
    "--size 262144" \
    "--size 4194304" \
    "--size 67108864" \
    "--size 268435456"

# ---------------------------------------------------------------------------
# memcopy  --size N  (N megabytes)
# PCIe / HBM limited.  Expected: ~0.3 ms → ~90 ms (H2D).
# ---------------------------------------------------------------------------
run_bench memcopy \
    "--size 16" \
    "--size 64" \
    "--size 256" \
    "--size 1024"

# ---------------------------------------------------------------------------
# matrixtranspose  --size N  (NxN; N must be a multiple of 64)
# Bandwidth-limited O(N²).  Expected: ~0.006 ms → ~25 ms.
# ---------------------------------------------------------------------------
run_bench matrixtranspose \
    "--size 1024" \
    "--size 4096" \
    "--size 16384" \
    "--size 65536"

# ---------------------------------------------------------------------------
# floydwarshall  --nodes N  (N must be a multiple of 8)
# O(N³) passes.  Expected: ~0.09 ms → ~3 s.
# ---------------------------------------------------------------------------
run_bench floydwarshall \
    "--nodes 128" \
    "--nodes 256" \
    "--nodes 512" \
    "--nodes 1024" \
    "--nodes 2048" \
    "--nodes 4096"

# ---------------------------------------------------------------------------
# fastwalshtransform  --size N  (power of 2, >= 512)
# O(N log N).  Expected: ~4 ms → ~2 s.
# ---------------------------------------------------------------------------
run_bench fastwalshtransform \
    "--size 65536" \
    "--size 1048576" \
    "--size 8388608" \
    "--size 33554432"

# ---------------------------------------------------------------------------
# fir  --length N  (multiple of 256)
# Bandwidth-limited O(N).  Expected: ~0.05 ms → ~370 ms.
# ---------------------------------------------------------------------------
run_bench fir \
    "--length 65536" \
    "--length 524288" \
    "--length 4194304" \
    "--length 33554432"

# ---------------------------------------------------------------------------
# simpleconvolution  --size N  (NxN image, 3x3 mask)
# O(N²).  Expected: ~0.03 ms → ~30 ms.
# ---------------------------------------------------------------------------
run_bench simpleconvolution \
    "--size 1024" \
    "--size 4096" \
    "--size 16384" \
    "--size 32768"

# ---------------------------------------------------------------------------
# bitonicsort  --size N  (power of 2)
# O(N log²N).  Expected: ~6 ms → ~3.6 s.
# ---------------------------------------------------------------------------
run_bench bitonicsort \
    "--size 65536" \
    "--size 1048576" \
    "--size 4194304" \
    "--size 16777216"

# ---------------------------------------------------------------------------
# kmeans  --points N  (any; 32 features, 5 clusters fixed)
# O(N).  Expected: ~0.07 ms → ~290 ms.
# ---------------------------------------------------------------------------
run_bench kmeans \
    "--points 8192" \
    "--points 131072" \
    "--points 2097152" \
    "--points 16777216"

# ---------------------------------------------------------------------------
# atax  --size N  (NxN matrix)
# O(N²).  Expected: ~0.6 ms → ~2.4 s.
# ---------------------------------------------------------------------------
run_bench atax \
    "--size 512" \
    "--size 2048" \
    "--size 8192" \
    "--size 16384" \
    "--size 32768"

# ---------------------------------------------------------------------------
# bicg  --size N  (NxN matrix)
# O(N²).  Expected: ~0.5 ms → ~2.1 s.
# ---------------------------------------------------------------------------
run_bench bicg \
    "--size 512" \
    "--size 2048" \
    "--size 8192" \
    "--size 16384" \
    "--size 32768"

# ---------------------------------------------------------------------------
# relu  --size N  (any; uses int index so max ~1B)
# Bandwidth-limited O(N).  Expected: ~0.08 ms → ~80 ms.
# ---------------------------------------------------------------------------
run_bench relu \
    "--size 1048576" \
    "--size 16777216" \
    "--size 268435456" \
    "--size 1073741824"

# ---------------------------------------------------------------------------
# pagerank  --nodes N  (multiple of 64; 50% dense graph)
# O(N²).  Expected: ~0.3 ms → ~4.5 s.
# ---------------------------------------------------------------------------
run_bench pagerank \
    "--nodes 256" \
    "--nodes 1024" \
    "--nodes 8192" \
    "--nodes 32768"

# ---------------------------------------------------------------------------
# stencil2d  --size N  (NxN grid; multiple of 64)
# Bandwidth-limited O(N²).  Expected: ~0.006 ms → ~25 ms.
# ---------------------------------------------------------------------------
run_bench stencil2d \
    "--size 512" \
    "--size 2048" \
    "--size 8192" \
    "--size 32768"

# ---------------------------------------------------------------------------
# bfs  --nodes N  (any; avg degree 6)
# O(N).  Expected: ~1 ms → ~2 s.
# ---------------------------------------------------------------------------
run_bench bfs \
    "--nodes 4096" \
    "--nodes 65536" \
    "--nodes 1048576" \
    "--nodes 8388608"

# ---------------------------------------------------------------------------
# nw  --size N  (multiple of 64; sequence alignment)
# O(N²) kernel launches.  Expected: ~0.2 ms → ~820 ms.
# ---------------------------------------------------------------------------
run_bench nw \
    "--size 128" \
    "--size 512" \
    "--size 2048" \
    "--size 4096" \
    "--size 8192"

# ---------------------------------------------------------------------------
# fft  --size N  (multiple of 512)
# Bandwidth-limited O(N).  Expected: ~0.008 ms → ~32 ms.
# ---------------------------------------------------------------------------
run_bench fft \
    "--size 131072" \
    "--size 2097152" \
    "--size 33554432" \
    "--size 536870912"

# ---------------------------------------------------------------------------
# spmv  --rows N  (any; 10 nnz/row fixed)
# O(N).  Expected: ~0.1 ms → ~245 ms.
# ---------------------------------------------------------------------------
run_bench spmv \
    "--rows 16384" \
    "--rows 262144" \
    "--rows 4194304" \
    "--rows 33554432"

# ---------------------------------------------------------------------------
# matrixmultiplication  --size N  (multiple of 32; NxNxN GEMM)
# O(N³).  Expected: ~0.18 ms → ~6 s.
# ---------------------------------------------------------------------------
run_bench matrixmultiplication \
    "--size 256" \
    "--size 512" \
    "--size 1024" \
    "--size 2048" \
    "--size 4096" \
    "--size 8192"

# ---------------------------------------------------------------------------
# nbody  --bodies N  (multiple of 256)
# O(N²).  Expected: ~2.9 ms → ~3 s.
# ---------------------------------------------------------------------------
run_bench nbody \
    "--bodies 1024" \
    "--bodies 4096" \
    "--bodies 16384" \
    "--bodies 65536" \
    "--bodies 131072"

# ---------------------------------------------------------------------------
# conv2d  --size N  (NxN input; 3x3 kernel, 3 output channels)
# O(N²).  Expected: ~0.04 ms → ~900 ms.
# ---------------------------------------------------------------------------
run_bench conv2d \
    "--size 56" \
    "--size 224" \
    "--size 896" \
    "--size 2048" \
    "--size 4096"

# ---------------------------------------------------------------------------
# im2col  --size N  (NxN input; 3x3 kernel)
# O(N²).  Expected: ~0.01 ms → ~200 ms.
# ---------------------------------------------------------------------------
run_bench im2col \
    "--size 56" \
    "--size 224" \
    "--size 896" \
    "--size 2048" \
    "--size 4096"

echo ""
echo "Run summary: ${pass} passed, ${fail} failed, ${skip} skipped"
echo "Results written to: ${OUTPUT}"
