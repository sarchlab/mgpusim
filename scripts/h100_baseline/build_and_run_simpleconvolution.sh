#!/usr/bin/env bash
# Build and run the OpenCL simple-convolution harness on the local GPU
# (intended for the H100 server). Run this ON the H100 server.
#
# Usage: ./build_and_run_simpleconvolution.sh <Width> <Height> <MaskSize> [iterations]
# Example (matches mgpusim's scaled-up default -width 510 -height 510
# -mask-size 3, chosen to keep mgpusim's timing simulation under ~1 minute
# -- see toy_guidebook.md's "Problem sizes" section):
#   ./build_and_run_simpleconvolution.sh 510 510 3 20
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KERNEL_CL="$SCRIPT_DIR/../../amd/benchmarks/amdappsdk/simpleconvolution/SimpleConvolution_Kernels.cl"
WIDTH="${1:-510}"
HEIGHT="${2:-510}"
MASK_SIZE="${3:-3}"
ITER="${4:-20}"

gcc "$SCRIPT_DIR/run_simpleconvolution.c" -o "$SCRIPT_DIR/run_simpleconvolution" -lOpenCL
"$SCRIPT_DIR/run_simpleconvolution" "$KERNEL_CL" "$WIDTH" "$HEIGHT" "$MASK_SIZE" "$ITER"
