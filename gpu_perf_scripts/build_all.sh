#!/bin/bash
# build_all.sh — Compile all 22 HIP benchmarks for gfx942 (AMD MI300A)
#
# Usage:
#   ./build_all.sh              # build all benchmarks
#   ./build_all.sh vectoradd    # build only vectoradd

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

HIPCC="${HIPCC:-hipcc}"
ARCH="${GPU_ARCH:-gfx942}"
CFLAGS="-O2 --offload-arch=${ARCH}"

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

build_one() {
    local name="$1"
    local src="${name}.cpp"
    if [ ! -f "$src" ]; then
        echo "[SKIP] ${src} not found"
        return 0
    fi
    echo "[BUILD] ${name}"
    ${HIPCC} ${CFLAGS} -o "${name}" "${src}"
}

# If arguments given, build only those; otherwise build all.
if [ $# -gt 0 ]; then
    for name in "$@"; do
        build_one "$name"
    done
else
    pass=0
    fail=0
    skip=0
    for name in "${BENCHMARKS[@]}"; do
        if [ ! -f "${name}.cpp" ]; then
            skip=$((skip + 1))
            echo "[SKIP] ${name}.cpp not found"
            continue
        fi
        if build_one "$name"; then
            pass=$((pass + 1))
        else
            fail=$((fail + 1))
            echo "[FAIL] ${name}"
        fi
    done
    echo ""
    echo "Build summary: ${pass} passed, ${fail} failed, ${skip} skipped (of ${#BENCHMARKS[@]} total)"
fi
