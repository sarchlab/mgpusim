#!/usr/bin/env bash
# Build and run the OpenCL matrix-multiplication harness on the local GPU
# (intended for the H100 server). Run this ON the H100 server.
#
# Usage: ./build_and_run_matrixmultiplication.sh <X> <Y> <Z> [iterations]
# Example (matches mgpusim's scaled-up default -x 320 -y 320 -z 320, chosen
# to keep mgpusim's timing simulation under ~1 minute -- see
# toy_guidebook.md's "Problem sizes" section):
#   ./build_and_run_matrixmultiplication.sh 320 320 320 20
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KERNEL_CL="$SCRIPT_DIR/../../amd/benchmarks/amdappsdk/matrixmultiplication/MatrixMultiplication_Kernels.cl"
X="${1:-320}"
Y="${2:-320}"
Z="${3:-320}"
ITER="${4:-20}"

gcc "$SCRIPT_DIR/run_matrixmultiplication.c" -o "$SCRIPT_DIR/run_matrixmultiplication" -lOpenCL
"$SCRIPT_DIR/run_matrixmultiplication" "$KERNEL_CL" "$X" "$Y" "$Z" "$ITER"
