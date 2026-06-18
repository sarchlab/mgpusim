#!/usr/bin/env bash
# Build and run an mgpusim sample in timing mode, producing a sqlite3 metrics
# file that compare.py / batch_compare.py can read.
#
# Usage: ./run_mgpusim.sh <sample-dir-under-amd/samples> <out-name> [extra flags...]
# Example:
#   ./run_mgpusim.sh matrixmultiplication mm_64 -x 64 -y 64 -z 64
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
SAMPLE="$1"; OUT_NAME="$2"; shift 2

SAMPLE_DIR="$REPO_ROOT/amd/samples/$SAMPLE"
cd "$SAMPLE_DIR"
go build -o "/tmp/mgpusim_$SAMPLE" .
"/tmp/mgpusim_$SAMPLE" -timing -report-all -disable-rtm -metric-file-name "$OUT_NAME" "$@"
mv "$SAMPLE_DIR/$OUT_NAME.sqlite3" "$SCRIPT_DIR/$OUT_NAME.sqlite3"
echo "wrote $SCRIPT_DIR/$OUT_NAME.sqlite3"
