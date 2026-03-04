#!/bin/bash
# run_all.sh — Run all 22 HIP benchmarks with 20-25 problem sizes each
#
# Sizes form a geometric (log-scale) sweep from the kernel-launch-overhead
# floor up to several seconds per iteration for compute-intensive kernels.
# Constraints are respected per benchmark (power-of-2, multiples of 8/32/64/256/512).
#
# NOTE: rebuild all benchmarks with ./build_all.sh before running so that
#       the CLI size parameters are active.
#
# Usage:
#   ./run_all.sh                    # 10 iterations per size
#   ./run_all.sh --iterations 5     # 5 iterations per size
#   ./run_all.sh --output out.csv

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
# fir  --length N  (multiple of 256; 21 sizes)
# O(N), bandwidth-limited.  ~0.005 ms → ~370 ms.
# ---------------------------------------------------------------------------
run_bench fir \
    "--length 256" \
    "--length 512" \
    "--length 1024" \
    "--length 2048" \
    "--length 4096" \
    "--length 8192" \
    "--length 16384" \
    "--length 32768" \
    "--length 65536" \
    "--length 131072" \
    "--length 262144" \
    "--length 524288" \
    "--length 1048576" \
    "--length 2097152" \
    "--length 4194304" \
    "--length 8388608" \
    "--length 16777216" \
    "--length 33554432" \
    "--length 67108864" \
    "--length 134217728" \
    "--length 268435456"

# ---------------------------------------------------------------------------
# simpleconvolution  --size N  (N×N image, 3×3 mask; 20 sizes)
# O(N²).  ~0.007 ms → ~30 ms.
# ---------------------------------------------------------------------------
run_bench simpleconvolution \
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
# kmeans  --points N  (any; 32 features, 5 clusters; 25 sizes)
# O(N).  ~0.02 ms → ~73 ms.
# ---------------------------------------------------------------------------
run_bench kmeans \
    "--points 1024" \
    "--points 1536" \
    "--points 2048" \
    "--points 3072" \
    "--points 4096" \
    "--points 6144" \
    "--points 8192" \
    "--points 12288" \
    "--points 16384" \
    "--points 24576" \
    "--points 32768" \
    "--points 49152" \
    "--points 65536" \
    "--points 98304" \
    "--points 131072" \
    "--points 196608" \
    "--points 262144" \
    "--points 393216" \
    "--points 524288" \
    "--points 786432" \
    "--points 1048576" \
    "--points 1572864" \
    "--points 2097152" \
    "--points 3145728" \
    "--points 4194304"

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
# bfs  --nodes N  (any; avg degree 6; 21 sizes)
# O(N) per level, ~O(N log N) total.  ~0.3 ms → ~2 s.
# ---------------------------------------------------------------------------
run_bench bfs \
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
    "--nodes 49152" \
    "--nodes 65536" \
    "--nodes 131072" \
    "--nodes 524288" \
    "--nodes 2097152" \
    "--nodes 8388608"

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
# spmv  --rows N  (any; 10 nnz/row; 23 sizes)
# O(N), bandwidth-limited.  ~0.006 ms → ~12 ms.
# ---------------------------------------------------------------------------
run_bench spmv \
    "--rows 1024" \
    "--rows 1536" \
    "--rows 2048" \
    "--rows 3072" \
    "--rows 4096" \
    "--rows 6144" \
    "--rows 8192" \
    "--rows 12288" \
    "--rows 16384" \
    "--rows 24576" \
    "--rows 32768" \
    "--rows 49152" \
    "--rows 65536" \
    "--rows 98304" \
    "--rows 131072" \
    "--rows 196608" \
    "--rows 262144" \
    "--rows 393216" \
    "--rows 524288" \
    "--rows 786432" \
    "--rows 1048576" \
    "--rows 1572864" \
    "--rows 2097152"

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
# conv2d  --size N  (N×N input; 3×3 kernel, 3 output channels; 21 sizes)
# O(N²).  ~0.009 ms → ~900 ms.
# ---------------------------------------------------------------------------
run_bench conv2d \
    "--size 4" \
    "--size 6" \
    "--size 8" \
    "--size 12" \
    "--size 16" \
    "--size 24" \
    "--size 32" \
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
    "--size 4096"

# ---------------------------------------------------------------------------
# im2col  --size N  (N×N input; 3×3 kernel; 21 sizes)
# O(N²).  ~0.007 ms → ~200 ms.
# ---------------------------------------------------------------------------
run_bench im2col \
    "--size 4" \
    "--size 6" \
    "--size 8" \
    "--size 12" \
    "--size 16" \
    "--size 24" \
    "--size 32" \
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
    "--size 4096"

echo ""
echo "Run summary: ${pass} passed, ${fail} failed, ${skip} skipped"
echo "Results written to: ${OUTPUT}"
