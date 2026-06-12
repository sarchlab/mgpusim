#!/bin/bash
# build_all.sh -- Build all GPU benchmarks
#
# Usage:
#   ./build_all.sh                      # auto-detect platform
#   ./build_all.sh --platform hip       # force HIP
#   ./build_all.sh --platform cuda      # force CUDA
#   ./build_all.sh --suite polybench    # build only one suite
#   ./build_all.sh --dry-run            # show what would be built

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
WORKLOADS_DIR="$(dirname "$SCRIPT_DIR")"

PLATFORM=""
SUITE=""
DRY_RUN=false

while [ $# -gt 0 ]; do
    case "$1" in
        --platform) PLATFORM="$2"; shift 2 ;;
        --suite)    SUITE="$2"; shift 2 ;;
        --dry-run)  DRY_RUN=true; shift ;;
        -h|--help)
            echo "Usage: $0 [--platform hip|cuda] [--suite NAME] [--dry-run]"
            exit 0
            ;;
        *) echo "Unknown option: $1" >&2; exit 1 ;;
    esac
done

# --- Platform detection ---
if [ -z "$PLATFORM" ]; then
    if command -v hipcc &>/dev/null; then
        PLATFORM="hip"
    elif command -v nvcc &>/dev/null; then
        PLATFORM="cuda"
    else
        echo "ERROR: No GPU compiler found (hipcc or nvcc required)" >&2
        exit 1
    fi
fi

# --- GPU architecture detection ---
if [ "$PLATFORM" = "hip" ]; then
    COMPILER="hipcc"
    GPU_ARCH="${GPU_ARCH:-$(rocminfo 2>/dev/null \
        | grep -m1 'Name:.*gfx' | awk '{print $2}' || echo gfx942)}"
elif [ "$PLATFORM" = "cuda" ]; then
    COMPILER="nvcc"
    GPU_ARCH="${GPU_ARCH:-$(nvidia-smi --query-gpu=compute_cap \
        --format=csv,noheader 2>/dev/null | head -1 \
        | tr -d '.' | sed 's/^/sm_/' || echo sm_80)}"
else
    echo "ERROR: Unknown platform '$PLATFORM' (must be 'hip' or 'cuda')" >&2
    exit 1
fi

echo "=== Build Configuration ==="
echo "  Platform: $PLATFORM"
echo "  Compiler: $COMPILER"
echo "  GPU arch: $GPU_ARCH"
echo ""

if [ "$DRY_RUN" = true ]; then
    echo "[DRY RUN] Would run: make -C $WORKLOADS_DIR PLATFORM=$PLATFORM GPU_ARCH=$GPU_ARCH"
    if [ -n "$SUITE" ]; then
        echo "  Target: suite-$SUITE"
    else
        echo "  Target: all"
    fi
    exit 0
fi

cd "$WORKLOADS_DIR"

if [ -n "$SUITE" ]; then
    make suite-"$SUITE" PLATFORM="$PLATFORM" GPU_ARCH="$GPU_ARCH"
else
    make all PLATFORM="$PLATFORM" GPU_ARCH="$GPU_ARCH"
fi
