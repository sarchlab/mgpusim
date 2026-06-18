#!/usr/bin/env bash
# Build and run the OpenCL Floyd-Warshall harness on the local GPU
# (intended for the H100 server). Run this ON the H100 server.
#
# Usage: ./build_and_run_floydwarshall.sh <NumNodes> <NumIterations> [iterations]
# Example (matches mgpusim's default -node 16 -iter 0):
#   ./build_and_run_floydwarshall.sh 16 0 20
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KERNEL_CL="$SCRIPT_DIR/../../amd/benchmarks/amdappsdk/floydwarshall/native/FloydWarshall_Kernels.cl"
NUM_NODES="${1:-16}"
NUM_ITER="${2:-0}"
ITER="${3:-20}"

gcc "$SCRIPT_DIR/run_floydwarshall.c" -o "$SCRIPT_DIR/run_floydwarshall" -lOpenCL
"$SCRIPT_DIR/run_floydwarshall" "$KERNEL_CL" "$NUM_NODES" "$NUM_ITER" "$ITER"
