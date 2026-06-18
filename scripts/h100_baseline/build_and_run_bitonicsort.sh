#!/usr/bin/env bash
# Build and run the OpenCL bitonic-sort harness on the local GPU
# (intended for the H100 server). Run this ON the H100 server.
#
# Usage: ./build_and_run_bitonicsort.sh <Length> <OrderAscending 0|1> [iterations]
# Example (matches mgpusim's default -length 1024 -order-asc=true):
#   ./build_and_run_bitonicsort.sh 1024 1 20
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KERNEL_CL="$SCRIPT_DIR/../../amd/benchmarks/amdappsdk/bitonicsort/kernels.cl"
LENGTH="${1:-1024}"
ORDER_ASC="${2:-1}"
ITER="${3:-20}"

gcc "$SCRIPT_DIR/run_bitonicsort.c" -o "$SCRIPT_DIR/run_bitonicsort" -lOpenCL
"$SCRIPT_DIR/run_bitonicsort" "$KERNEL_CL" "$LENGTH" "$ORDER_ASC" "$ITER"
