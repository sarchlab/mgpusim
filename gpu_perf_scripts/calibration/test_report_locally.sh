#!/usr/bin/env bash
#
# Iterate on the MI300A calibration REPORT (plot_calibration.py) WITHOUT paying
# for the ~2-hour cycle-accurate sweep.
#
# The two sweep jobs upload their sim-result CSVs as artifacts on every workflow
# run (`fp32-sim-results`, `cache-latency-sim-results`). This script grabs those
# CSVs from a finished run and feeds them to the exact same plotting step the CI
# `report` job runs, against the committed ground truth -- so you get the figures
# and the job-summary table locally in seconds and can iterate on the report.
#
# Usage:
#   test_report_locally.sh [RUN_ID] [-o OUT_DIR] [-d DATA_DIR] [--web] [--open]
#
#   RUN_ID        GitHub Actions run id to pull sim CSVs from.
#                 Default: 27708217340 (the first full fp32 + cache_latency run).
#   -d DATA_DIR   Use sim CSVs already on disk instead of downloading. DATA_DIR is
#                 searched recursively for fp32_sim_results.csv and
#                 cache_latency_sim_results.csv (either/both may be absent).
#   -o OUT_DIR    Where to write figures/ + summary.md.
#                 Default: ${TMPDIR}/mi300a_report_test/out_<RUN_ID>.
#   --web         Also build the interactive gh-pages dashboard locally (exact
#                 same index.html + data.json the Pages site uses) and serve it,
#                 so you can preview the site before enabling GitHub Pages.
#   --port N      Port for --web's local server (default 8000).
#   --open        Open the result: with --web the dashboard URL, otherwise OUT_DIR.
#
# Requires: python3 with matplotlib + seaborn; gh (authenticated) only when
# downloading (i.e. when -d is not given).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REF_CSV="$SCRIPT_DIR/mi300a_ground_truth.csv"
PLOTTER="$SCRIPT_DIR/plot_calibration.py"
WEB_DIR="$SCRIPT_DIR/web"

RUN_ID="27708217340"
DATA_DIR=""
OUT_DIR=""
DO_OPEN=0
DO_WEB=0
PORT=8000

# First bare (non-flag) argument is the run id; flags may appear in any order.
args=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o) OUT_DIR="$2"; shift 2 ;;
    -d) DATA_DIR="$2"; shift 2 ;;
    --web) DO_WEB=1; shift ;;
    --port) PORT="$2"; shift 2 ;;
    --open) DO_OPEN=1; shift ;;
    -h|--help) sed -n '2,33p' "${BASH_SOURCE[0]}"; exit 0 ;;
    -*) echo "unknown flag: $1" >&2; exit 2 ;;
    *) args+=("$1"); shift ;;
  esac
done
[[ ${#args[@]} -gt 0 ]] && RUN_ID="${args[0]}"

CACHE_ROOT="${TMPDIR:-/tmp}/mi300a_report_test"
[[ -z "$OUT_DIR" ]] && OUT_DIR="$CACHE_ROOT/out_${RUN_ID}"

# ---- locate the sim-result CSVs ---------------------------------------------
if [[ -z "$DATA_DIR" ]]; then
  command -v gh >/dev/null 2>&1 || { echo "ERROR: gh not found; install it or pass -d DATA_DIR." >&2; exit 1; }
  DATA_DIR="$CACHE_ROOT/run_${RUN_ID}"
  if [[ -d "$DATA_DIR" ]] && find "$DATA_DIR" -name '*_sim_results.csv' | grep -q .; then
    echo "Using cached artifacts in $DATA_DIR (delete to re-download)."
  else
    echo "Downloading sim-result artifacts from run $RUN_ID ..."
    mkdir -p "$DATA_DIR"
    # Only the lightweight CSV artifacts -- not the multi-MB figures.
    gh run download "$RUN_ID" -D "$DATA_DIR" \
      -n fp32-sim-results -n cache-latency-sim-results 2>/dev/null \
      || gh run download "$RUN_ID" -D "$DATA_DIR"   # fall back to all artifacts
  fi
fi
[[ -d "$DATA_DIR" ]] || { echo "ERROR: data dir not found: $DATA_DIR" >&2; exit 1; }

find_csv() {  # $1=basename -> first match under DATA_DIR, or ""
  find "$DATA_DIR" -type f -name "$1" 2>/dev/null | head -1
}
FP32_CSV="$(find_csv fp32_sim_results.csv)"
CACHE_CSV="$(find_csv cache_latency_sim_results.csv)"

# plot_calibration.py skips a sim source whose path does not exist, so a missing
# CSV just drops that benchmark's figures rather than erroring. Point absent ones
# at a non-existent path to make that explicit.
: "${FP32_CSV:=$DATA_DIR/__missing_fp32.csv}"
: "${CACHE_CSV:=$DATA_DIR/__missing_cache.csv}"
echo "  fp32 sim CSV : ${FP32_CSV/#$DATA_DIR\//}"
echo "  cache sim CSV: ${CACHE_CSV/#$DATA_DIR\//}"

# ---- render the report ------------------------------------------------------
mkdir -p "$OUT_DIR"
SUMMARY="$OUT_DIR/summary.md"
REPORT="$OUT_DIR/report.md"
: > "$SUMMARY"   # truncate any previous summary so reruns don't append

# --web also emits data.json into a local mirror of the gh-pages layout
# (<site>/calibration/...) so the dashboard renders exactly what Pages would.
SITE="$OUT_DIR/site"
CAL="$SITE/calibration"
PLOT_ARGS=(
  --ref "$REF_CSV"
  --fp32-sim "$FP32_CSV"
  --cache-sim "$CACHE_CSV"
  --out "$OUT_DIR/figures"
  --summary "$SUMMARY"
  --report "$REPORT"
)
if [[ "$DO_WEB" == "1" ]]; then
  mkdir -p "$CAL/runs/$RUN_ID"
  PLOT_ARGS+=(
    --json "$CAL/runs/$RUN_ID/data.json"
    --run-id "$RUN_ID"
    --run-url "https://github.com/sarchlab/mgpusim/actions/runs/$RUN_ID"
    --generated-at "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  )
fi

echo "Rendering report into $OUT_DIR ..."
python3 "$PLOTTER" "${PLOT_ARGS[@]}"

echo
echo "==================== job-summary preview ===================="
cat "$SUMMARY"
echo
echo "============================================================="
echo "Figures : $OUT_DIR/figures"
echo "Summary : $SUMMARY  (compact table -> CI job summary)"
echo "Report  : $REPORT  (figures embedded -> open in a markdown viewer)"

if [[ "$DO_WEB" == "1" ]]; then
  cp "$WEB_DIR/index.html" "$CAL/index.html"
  python3 "$WEB_DIR/build_index.py" "$CAL/runs" -o "$CAL/index.json"
  URL="http://localhost:${PORT}/calibration/?run=${RUN_ID}"
  echo "Dashboard: $URL"
  echo "(serving $SITE ; press Ctrl-C to stop)"
  if [[ "$DO_OPEN" == "1" ]]; then
    if command -v open >/dev/null 2>&1; then open "$URL" || true
    elif command -v xdg-open >/dev/null 2>&1; then xdg-open "$URL" || true
    fi
  fi
  exec python3 -m http.server "$PORT" --directory "$SITE"
fi

if [[ "$DO_OPEN" == "1" ]]; then
  if command -v open >/dev/null 2>&1; then open "$OUT_DIR"
  elif command -v xdg-open >/dev/null 2>&1; then xdg-open "$OUT_DIR"
  fi
fi
