#!/usr/bin/env bash
# Build and run the OpenCL matrix-multiplication harness on the local GPU
# (intended for the H100 server). Run this ON the H100 server.
#
# Usage: ./build_and_run_matrixmultiplication.sh <X> <Y> <Z> [iterations]
# Example (matches mgpusim's default -x 64 -y 64 -z 64):
#   ./build_and_run_matrixmultiplication.sh 64 64 64 20
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KERNEL_CL="$SCRIPT_DIR/../../amd/benchmarks/amdappsdk/matrixmultiplication/MatrixMultiplication_Kernels.cl"
X="${1:-64}"
Y="${2:-64}"
Z="${3:-64}"
ITER="${4:-20}"

gcc "$SCRIPT_DIR/run_matrixmultiplication.c" -o "$SCRIPT_DIR/run_matrixmultiplication" -lOpenCL
"$SCRIPT_DIR/run_matrixmultiplication" "$KERNEL_CL" "$X" "$Y" "$Z" "$ITER"
