#!/bin/bash
# run_all.sh — Run all 22 HIP benchmarks with multi-dimensional size sweeps
#
# 1D benchmarks (single size parameter):
#   vectoradd, memcopy, matrixtranspose, floydwarshall, fastwalshtransform,
#   bitonicsort, atax, bicg, relu, pagerank, stencil2d, nw, fft,
#   matrixmultiplication, nbody
#
# 2D benchmarks (Cartesian product of two parameters):
#   fir           length × taps         (5×4 = 20 combos)
#   simpleconvolution  size × mask      (6×4 = 24 combos)
#   kmeans        points × features     (5×4 = 20 combos)
#   bfs           nodes × degree        (5×4 = 20 combos)
#   spmv          rows × nnz_per_row    (5×4 = 20 combos)
#   conv2d        size × mask           (8×3 = 24 combos)
#   im2col        size × mask           (8×3 = 24 combos)
#
# NOTE: rebuild all benchmarks with ./build_all.sh before running so that
#       the CLI size parameters are active.
#
# Usage:
#   ./run_all.sh                          # 10 iterations, 120s timeout
#   ./run_all.sh --iterations 5           # 5 iterations per size
#   ./run_all.sh --timeout 60             # kill any run exceeding 60s
#   ./run_all.sh --timeout 0              # disable timeout
#   ./run_all.sh --output out.csv

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

ITERATIONS=10
OUTPUT="results.csv"
TIMEOUT=120   # seconds per benchmark run; 0 = no timeout

while [ $# -gt 0 ]; do
    case "$1" in
        --iterations) ITERATIONS="$2"; shift 2 ;;
        --output)     OUTPUT="$2";     shift 2 ;;
        --timeout)    TIMEOUT="$2";    shift 2 ;;
        *) echo "Unknown option: $1" >&2; exit 1 ;;
    esac
done

echo "kernel_name,problem_size,iterations,avg_ms,min_ms,max_ms" > "$OUTPUT"

pass=0
fail=0
skip=0
timed_out=0

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
        exit_code=0
        if [ "${TIMEOUT}" -gt 0 ]; then
            output=$(timeout "${TIMEOUT}" ./"${name}" --iterations "${ITERATIONS}" ${args} 2>&1) || exit_code=$?
        else
            output=$(./"${name}" --iterations "${ITERATIONS}" ${args} 2>&1) || exit_code=$?
        fi
        if [ ${exit_code} -eq 0 ]; then
            echo "$output" | grep -v '^kernel_name' >> "$OUTPUT"
            pass=$((pass + 1))
        elif [ ${exit_code} -eq 124 ]; then
            echo "[TIMEOUT] ${name} ${args} (>${TIMEOUT}s)"
            timed_out=$((timed_out + 1))
        else
            echo "[FAIL] ${name} ${args} (exit ${exit_code})"
            fail=$((fail + 1))
        fi
    done
}

# ---------------------------------------------------------------------------
# vectoradd  --size N  (any N; 20 sizes, powers of 2 from 1K to 512M)
# O(N), bandwidth-limited.  ~0.005 ms → ~600 ms on MI300A.
# ---------------------------------------------------------------------------
run_bench vectoradd \
    "--size 1024" \
    "--size 2048" \
    "--size 4096" \
    "--size 8192" \
    "--size 16384" \
    "--size 32768" \
    "--size 65536" \
    "--size 131072" \
    "--size 262144" \
    "--size 524288" \
    "--size 1048576" \
    "--size 2097152" \
    "--size 4194304" \
    "--size 8388608" \
    "--size 16777216" \
    "--size 33554432" \
    "--size 67108864" \
    "--size 134217728" \
    "--size 268435456" \
    "--size 536870912"

# ---------------------------------------------------------------------------
# memcopy  --size N  (N megabytes; 10 sizes → 30 CSV rows: h2d + d2h + d2d)
# Bandwidth-limited.  ~0.1 ms → ~90 ms per variant.
# ---------------------------------------------------------------------------
run_bench memcopy \
    "--size 1" \
    "--size 2" \
    "--size 4" \
    "--size 8" \
    "--size 16" \
    "--size 32" \
    "--size 64" \
    "--size 128" \
    "--size 256" \
    "--size 512"

# ---------------------------------------------------------------------------
# matrixtranspose  --size N  (N×N; N multiple of 64; 20 sizes)
# O(N²), bandwidth-limited.  ~0.006 ms → ~25 ms.
# ---------------------------------------------------------------------------
run_bench matrixtranspose \
    "--size 64" \
    "--size 128" \
    "--size 192" \
    "--size 256" \
    "--size 384" \
    "--size 512" \
    "--size 768" \
    "--size 1024" \
    "--size 1536" \
    "--size 2048" \
    "--size 3072" \
    "--size 4096" \
    "--size 6144" \
    "--size 8192" \
    "--size 12288" \
    "--size 16384" \
    "--size 24576" \
    "--size 32768" \
    "--size 49152" \
    "--size 65536"

# ---------------------------------------------------------------------------
# floydwarshall  --nodes N  (N multiple of 8; 23 sizes)
# O(N³).  ~0.001 ms → ~3 s.
# ---------------------------------------------------------------------------
run_bench floydwarshall \
    "--nodes 8" \
    "--nodes 16" \
    "--nodes 24" \
    "--nodes 32" \
    "--nodes 48" \
    "--nodes 64" \
    "--nodes 96" \
    "--nodes 128" \
    "--nodes 160" \
    "--nodes 192" \
    "--nodes 256" \
    "--nodes 320" \
    "--nodes 384" \
    "--nodes 512" \
    "--nodes 640" \
    "--nodes 768" \
    "--nodes 1024" \
    "--nodes 1280" \
    "--nodes 1536" \
    "--nodes 2048" \
    "--nodes 2560" \
    "--nodes 3072" \
    "--nodes 4096"

# ---------------------------------------------------------------------------
# fastwalshtransform  --size N  (power of 2, >=512; 20 sizes)
# O(N log N).  ~0.04 ms → ~2.5 s.
# ---------------------------------------------------------------------------
run_bench fastwalshtransform \
    "--size 512" \
    "--size 1024" \
    "--size 2048" \
    "--size 4096" \
    "--size 8192" \
    "--size 16384" \
    "--size 32768" \
    "--size 65536" \
    "--size 131072" \
    "--size 262144" \
    "--size 524288" \
    "--size 1048576" \
    "--size 2097152" \
    "--size 4194304" \
    "--size 8388608" \
    "--size 16777216" \
    "--size 33554432" \
    "--size 67108864" \
    "--size 134217728" \
    "--size 268435456"

# ---------------------------------------------------------------------------
# fir  --length N --taps T  (length: multiple of 256; 2D: 5×4 = 20 combos)
# O(N·T), bandwidth/compute.  Sweeps signal length × filter order.
# ---------------------------------------------------------------------------
for fir_len in 1024 4096 16384 65536 262144; do
    for fir_taps in 8 16 32 64; do
        run_bench fir "--length ${fir_len} --taps ${fir_taps}"
    done
done

# ---------------------------------------------------------------------------
# simpleconvolution  --size N --mask M  (2D: 6×4 = 24 combos)
# O(N²·M²).  Sweeps image resolution × convolution kernel size (odd only).
# ---------------------------------------------------------------------------
for sc_size in 128 256 512 1024 2048 4096; do
    for sc_mask in 3 5 7 9; do
        run_bench simpleconvolution "--size ${sc_size} --mask ${sc_mask}"
    done
done

# ---------------------------------------------------------------------------
# bitonicsort  --size N  (power of 2; 15 sizes — algorithm constraint)
# O(N log²N).  ~0.2 ms → ~3.6 s.
# ---------------------------------------------------------------------------
run_bench bitonicsort \
    "--size 1024" \
    "--size 2048" \
    "--size 4096" \
    "--size 8192" \
    "--size 16384" \
    "--size 32768" \
    "--size 65536" \
    "--size 131072" \
    "--size 262144" \
    "--size 524288" \
    "--size 1048576" \
    "--size 2097152" \
    "--size 4194304" \
    "--size 8388608" \
    "--size 16777216"

# ---------------------------------------------------------------------------
# kmeans  --points N --features F --clusters K  (2D: 5×4 = 20 combos)
# O(N·F·K).  Sweeps data points × feature dimensionality (clusters=5 fixed).
# ---------------------------------------------------------------------------
for km_pts in 1024 4096 16384 65536 262144; do
    for km_feat in 8 16 32 64; do
        run_bench kmeans "--points ${km_pts} --features ${km_feat}"
    done
done

# ---------------------------------------------------------------------------
# atax  --size N  (N×N matrix; 20 sizes)
# O(N²).  ~0.05 ms → ~2.4 s.
# ---------------------------------------------------------------------------
run_bench atax \
    "--size 48" \
    "--size 64" \
    "--size 96" \
    "--size 128" \
    "--size 192" \
    "--size 256" \
    "--size 384" \
    "--size 512" \
    "--size 768" \
    "--size 1024" \
    "--size 1536" \
    "--size 2048" \
    "--size 3072" \
    "--size 4096" \
    "--size 6144" \
    "--size 8192" \
    "--size 12288" \
    "--size 16384" \
    "--size 24576" \
    "--size 32768"

# ---------------------------------------------------------------------------
# bicg  --size N  (N×N matrix; 20 sizes)
# O(N²).  ~0.05 ms → ~2.1 s.
# ---------------------------------------------------------------------------
run_bench bicg \
    "--size 48" \
    "--size 64" \
    "--size 96" \
    "--size 128" \
    "--size 192" \
    "--size 256" \
    "--size 384" \
    "--size 512" \
    "--size 768" \
    "--size 1024" \
    "--size 1536" \
    "--size 2048" \
    "--size 3072" \
    "--size 4096" \
    "--size 6144" \
    "--size 8192" \
    "--size 12288" \
    "--size 16384" \
    "--size 24576" \
    "--size 32768"

# ---------------------------------------------------------------------------
# relu  --size N  (any; int index, max ~1B; 21 sizes)
# O(N), bandwidth-limited.  ~0.005 ms → ~80 ms.
# ---------------------------------------------------------------------------
run_bench relu \
    "--size 1024" \
    "--size 2048" \
    "--size 4096" \
    "--size 8192" \
    "--size 16384" \
    "--size 32768" \
    "--size 65536" \
    "--size 131072" \
    "--size 262144" \
    "--size 524288" \
    "--size 1048576" \
    "--size 2097152" \
    "--size 4194304" \
    "--size 8388608" \
    "--size 16777216" \
    "--size 33554432" \
    "--size 67108864" \
    "--size 134217728" \
    "--size 268435456" \
    "--size 536870912" \
    "--size 1073741824"

# ---------------------------------------------------------------------------
# pagerank  --nodes N  (multiple of 64; 50% dense random graph; 19 sizes)
# O(N²).  ~0.02 ms → ~4.5 s.
# ---------------------------------------------------------------------------
run_bench pagerank \
    "--nodes 64" \
    "--nodes 128" \
    "--nodes 192" \
    "--nodes 256" \
    "--nodes 384" \
    "--nodes 512" \
    "--nodes 768" \
    "--nodes 1024" \
    "--nodes 1536" \
    "--nodes 2048" \
    "--nodes 3072" \
    "--nodes 4096" \
    "--nodes 6144" \
    "--nodes 8192" \
    "--nodes 12288" \
    "--nodes 16384" \
    "--nodes 24576" \
    "--nodes 32768" \
    "--nodes 49152"

# ---------------------------------------------------------------------------
# stencil2d  --size N  (N×N grid; multiple of 64; 20 sizes)
# O(N²), bandwidth-limited.  ~0.006 ms → ~100 ms.
# ---------------------------------------------------------------------------
run_bench stencil2d \
    "--size 64" \
    "--size 128" \
    "--size 192" \
    "--size 256" \
    "--size 384" \
    "--size 512" \
    "--size 768" \
    "--size 1024" \
    "--size 1536" \
    "--size 2048" \
    "--size 3072" \
    "--size 4096" \
    "--size 6144" \
    "--size 8192" \
    "--size 12288" \
    "--size 16384" \
    "--size 24576" \
    "--size 32768" \
    "--size 49152" \
    "--size 65536"

# ---------------------------------------------------------------------------
# bfs  --nodes N --degree D  (2D: 5×4 = 20 combos)
# Sweeps graph size × average vertex degree (edge density).
# ---------------------------------------------------------------------------
for bfs_nodes in 1024 4096 16384 65536 262144; do
    for bfs_deg in 4 8 16 32; do
        run_bench bfs "--nodes ${bfs_nodes} --degree ${bfs_deg}"
    done
done

# ---------------------------------------------------------------------------
# nw  --size N  (multiple of 64; 24 sizes)
# O(N²) kernel launches.  ~0.05 ms → ~820 ms.
# ---------------------------------------------------------------------------
run_bench nw \
    "--size 64" \
    "--size 128" \
    "--size 192" \
    "--size 256" \
    "--size 320" \
    "--size 384" \
    "--size 448" \
    "--size 512" \
    "--size 640" \
    "--size 768" \
    "--size 896" \
    "--size 1024" \
    "--size 1280" \
    "--size 1536" \
    "--size 1792" \
    "--size 2048" \
    "--size 2560" \
    "--size 3072" \
    "--size 3584" \
    "--size 4096" \
    "--size 5120" \
    "--size 6144" \
    "--size 7168" \
    "--size 8192"

# ---------------------------------------------------------------------------
# fft  --size N  (multiple of 512; 21 sizes, powers of 2)
# O(N), bandwidth-limited.  ~0.008 ms → ~32 ms.
# ---------------------------------------------------------------------------
run_bench fft \
    "--size 512" \
    "--size 1024" \
    "--size 2048" \
    "--size 4096" \
    "--size 8192" \
    "--size 16384" \
    "--size 32768" \
    "--size 65536" \
    "--size 131072" \
    "--size 262144" \
    "--size 524288" \
    "--size 1048576" \
    "--size 2097152" \
    "--size 4194304" \
    "--size 8388608" \
    "--size 16777216" \
    "--size 33554432" \
    "--size 67108864" \
    "--size 134217728" \
    "--size 268435456" \
    "--size 536870912"

# ---------------------------------------------------------------------------
# spmv  --rows N --nnz K  (2D: 5×4 = 20 combos)
# O(N·K), bandwidth-limited.  Sweeps matrix rows × non-zeros per row.
# ---------------------------------------------------------------------------
for spmv_rows in 1024 4096 16384 65536 262144; do
    for spmv_nnz in 4 8 16 32; do
        run_bench spmv "--rows ${spmv_rows} --nnz ${spmv_nnz}"
    done
done

# ---------------------------------------------------------------------------
# matrixmultiplication  --size N  (multiple of 32; 22 sizes)
# O(N³).  ~0.001 ms → ~6 s.
# ---------------------------------------------------------------------------
run_bench matrixmultiplication \
    "--size 32" \
    "--size 64" \
    "--size 96" \
    "--size 128" \
    "--size 160" \
    "--size 192" \
    "--size 256" \
    "--size 320" \
    "--size 384" \
    "--size 512" \
    "--size 640" \
    "--size 768" \
    "--size 1024" \
    "--size 1280" \
    "--size 1536" \
    "--size 2048" \
    "--size 2560" \
    "--size 3072" \
    "--size 4096" \
    "--size 5120" \
    "--size 6144" \
    "--size 8192"

# ---------------------------------------------------------------------------
# nbody  --bodies N  (multiple of 256; 22 sizes)
# O(N²).  ~0.18 ms → ~3 s.
# ---------------------------------------------------------------------------
run_bench nbody \
    "--bodies 256" \
    "--bodies 512" \
    "--bodies 768" \
    "--bodies 1024" \
    "--bodies 1280" \
    "--bodies 1536" \
    "--bodies 2048" \
    "--bodies 2560" \
    "--bodies 3072" \
    "--bodies 4096" \
    "--bodies 5120" \
    "--bodies 6144" \
    "--bodies 8192" \
    "--bodies 10240" \
    "--bodies 12288" \
    "--bodies 16384" \
    "--bodies 24576" \
    "--bodies 32768" \
    "--bodies 49152" \
    "--bodies 65536" \
    "--bodies 98304" \
    "--bodies 131072"

# ---------------------------------------------------------------------------
# conv2d  --size N --mask M  (2D: 8×3 = 24 combos)
# O(N²·M²).  Sweeps input resolution × convolution kernel size.
# ---------------------------------------------------------------------------
for c2d_size in 8 16 32 64 128 256 512 1024; do
    for c2d_mask in 3 5 7; do
        run_bench conv2d "--size ${c2d_size} --mask ${c2d_mask}"
    done
done

# ---------------------------------------------------------------------------
# im2col  --size N --mask M  (2D: 8×3 = 24 combos)
# O(N²·M²).  Sweeps input resolution × kernel size.
# ---------------------------------------------------------------------------
for i2c_size in 8 16 32 64 128 256 512 1024; do
    for i2c_mask in 3 5 7; do
        run_bench im2col "--size ${i2c_size} --mask ${i2c_mask}"
    done
done

echo ""
echo "Run summary: ${pass} passed, ${fail} failed, ${timed_out} timed out, ${skip} skipped"
echo "Results written to: ${OUTPUT}"
