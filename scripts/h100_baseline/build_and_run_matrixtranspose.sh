#!/usr/bin/env bash
# Build and run the OpenCL matrix-transpose harness on the local GPU
# (intended for the H100 server). Run this ON the H100 server.
#
# Usage: ./build_and_run_matrixtranspose.sh <Width> [iterations]
# Example (matches mgpusim's default -width 256):
#   ./build_and_run_matrixtranspose.sh 256 20
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KERNEL_CL="$SCRIPT_DIR/../../amd/benchmarks/amdappsdk/matrixtranspose/native/MatrixTranspose_Kernels.cl"
WIDTH="${1:-256}"
ITER="${2:-20}"

gcc "$SCRIPT_DIR/run_matrixtranspose.c" -o "$SCRIPT_DIR/run_matrixtranspose" -lOpenCL
"$SCRIPT_DIR/run_matrixtranspose" "$KERNEL_CL" "$WIDTH" "$ITER"
