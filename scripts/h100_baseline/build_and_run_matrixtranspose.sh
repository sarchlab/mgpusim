#!/usr/bin/env bash
# Build and run the OpenCL matrix-transpose harness on the local GPU
# (intended for the H100 server). Run this ON the H100 server.
#
# Usage: ./build_and_run_matrixtranspose.sh <Width> [iterations]
# Example (matches mgpusim's scaled-up default -width 1024, chosen to keep
# mgpusim's timing simulation under ~1 minute -- see toy_guidebook.md's
# "Problem sizes" section):
#   ./build_and_run_matrixtranspose.sh 1024 20
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KERNEL_CL="$SCRIPT_DIR/../../amd/benchmarks/amdappsdk/matrixtranspose/native/MatrixTranspose_Kernels.cl"
WIDTH="${1:-1024}"
ITER="${2:-20}"

gcc "$SCRIPT_DIR/run_matrixtranspose.c" -o "$SCRIPT_DIR/run_matrixtranspose" -lOpenCL
"$SCRIPT_DIR/run_matrixtranspose" "$KERNEL_CL" "$WIDTH" "$ITER"
