#!/usr/bin/env bash
# =============================================================================
# run_tier1.sh — Run Tier 1 (fast) benchmarks for quick local validation
# =============================================================================
#
# Tier 1 benchmarks have max sim_ms < 0.05 ms, meaning they simulate quickly.
# Use this script for rapid iteration during development.
#
# Usage:
#   cd <repo_root>
#   bash benchmark-comparison/run_tier1.sh
#
# Prerequisites:
#   - Go built: go build ./...
#   - Binaries in amd/samples/<benchmark>/
# =============================================================================

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GO="${GO:-go}"

# Tier 1 benchmarks (kernel_name → CI benchmark directory mapping)
# These all have max sim_ms < 0.05 ms in comparison_ci.csv
TIER1_BENCHMARKS=(
    "2dconvolution"       # 2dconvolution kernel
    "3dconvolution"       # 3dconvolution kernel
    "bs"                  # bs kernel
    "ep"                  # ep kernel
    "gemm"                # gemm kernel
    "nn"                  # nn kernel
    "reduction"           # reduction kernel
    "scan"                # scan kernel
    "spmv"                # spmv_csr kernel
    "triad"               # triad kernel
    "2mm"                 # 2mm kernel
    "hotspot"             # hotspot kernel
    "dwt2d"               # dwt2d kernel
    # Note: sgemm_tiled maps to parboil_sgemm, computeq maps to parboil_mri_q
    "parboil_sgemm"       # sgemm_tiled kernel
    "parboil_mri_q"       # computeq kernel
)

echo "============================================================"
echo "  Tier 1 Benchmark Quick Run"
echo "  ${#TIER1_BENCHMARKS[@]} benchmarks"
echo "============================================================"
echo ""

PASS=0
FAIL=0
SKIP=0

for bench in "${TIER1_BENCHMARKS[@]}"; do
    # Find the benchmark directory
    bench_dir=""
    if [[ -d "$REPO_ROOT/amd/samples/$bench" ]]; then
        bench_dir="$REPO_ROOT/amd/samples/$bench"
    elif [[ "$bench" == parboil_* ]]; then
        parboil_name="${bench#parboil_}"
        if [[ -d "$REPO_ROOT/amd/samples/parboil/$parboil_name" ]]; then
            bench_dir="$REPO_ROOT/amd/samples/parboil/$parboil_name"
        fi
    fi

    if [[ -z "$bench_dir" ]]; then
        echo "  SKIP  $bench (directory not found)"
        ((SKIP++))
        continue
    fi

    printf "  %-25s " "$bench"

    # Build
    if ! (cd "$bench_dir" && timeout 60 $GO build ./... 2>/dev/null); then
        echo "FAIL (build error)"
        ((FAIL++))
        continue
    fi

    # Find the binary
    binary=$(find "$bench_dir" -maxdepth 1 -type f -perm +111 -not -name "*.go" -not -name "*.sh" 2>/dev/null | head -1)
    if [[ -z "$binary" ]]; then
        # Try the directory name as binary
        binary="$bench_dir/$(basename "$bench_dir")"
    fi

    if [[ ! -x "$binary" ]]; then
        echo "SKIP (no binary)"
        ((SKIP++))
        continue
    fi

    # Run with smallest size, timing mode, 30s timeout
    if timeout 30 "$binary" -timing -report-all 2>/dev/null | grep -q "kernel_time\|total_time"; then
        echo "PASS"
        ((PASS++))
    else
        # Try without flags
        if timeout 30 "$binary" -timing 2>/dev/null; then
            echo "PASS"
            ((PASS++))
        else
            echo "FAIL"
            ((FAIL++))
        fi
    fi
done

echo ""
echo "============================================================"
echo "  Results: $PASS pass, $FAIL fail, $SKIP skip"
echo "============================================================"
