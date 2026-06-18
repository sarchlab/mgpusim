#!/usr/bin/env bash
# Build and run the OpenCL fast-Walsh-transform harness on the local GPU
# (intended for the H100 server). Run this ON the H100 server.
#
# Usage: ./build_and_run_fastwalshtransform.sh <Length> [iterations]
# Example (matches mgpusim's scaled-up default -length 65536, chosen to
# keep mgpusim's timing simulation under ~1 minute -- see toy_guidebook.md's
# "Problem sizes" section):
#   ./build_and_run_fastwalshtransform.sh 65536 20
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KERNEL_CL="$SCRIPT_DIR/../../amd/benchmarks/amdappsdk/fastwalshtransform/native/FastWalshTransform_Kernels.cl"
LENGTH="${1:-65536}"
ITER="${2:-20}"

gcc "$SCRIPT_DIR/run_fastwalshtransform.c" -o "$SCRIPT_DIR/run_fastwalshtransform" -lOpenCL
"$SCRIPT_DIR/run_fastwalshtransform" "$KERNEL_CL" "$LENGTH" "$ITER"
