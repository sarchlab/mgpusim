#!/usr/bin/env bash
# Build and run the OpenCL bitonic-sort harness on the local GPU
# (intended for the H100 server). Run this ON the H100 server.
#
# Usage: ./build_and_run_bitonicsort.sh <Length> <OrderAscending 0|1> [iterations]
# Example (matches mgpusim's scaled-up default -length 32768 -order-asc=true,
# chosen to keep mgpusim's timing simulation under ~1 minute -- see
# toy_guidebook.md's "Problem sizes" section):
#   ./build_and_run_bitonicsort.sh 32768 1 20
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KERNEL_CL="$SCRIPT_DIR/../../amd/benchmarks/amdappsdk/bitonicsort/kernels.cl"
LENGTH="${1:-32768}"
ORDER_ASC="${2:-1}"
ITER="${3:-20}"

gcc "$SCRIPT_DIR/run_bitonicsort.c" -o "$SCRIPT_DIR/run_bitonicsort" -lOpenCL
"$SCRIPT_DIR/run_bitonicsort" "$KERNEL_CL" "$LENGTH" "$ORDER_ASC" "$ITER"
