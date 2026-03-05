#!/bin/bash
# run_sim_benchmarks.sh — Run MGPUSim benchmarks with MI300A timing config
#                         and collect simulated kernel execution times.
#
# This script builds each benchmark from amd/samples/<name>/, runs it with
# timing simulation using the CDNA3 arch / MI300A GPU config, then extracts
# the kernel execution time from the output SQLite database.
#
# The simulator is deterministic, so only 1 run per configuration is needed.
# Results are written in the same CSV format as mi300a.csv (real hardware).
#
# Usage:
#   ./run_sim_benchmarks.sh                   # default output: sim_results.csv
#   ./run_sim_benchmarks.sh --output out.csv  # custom output file
#   ./run_sim_benchmarks.sh --timeout 300     # per-run wall-clock timeout (sec)
#   ./run_sim_benchmarks.sh --skip-build      # skip go build step
#   ./run_sim_benchmarks.sh --verbose         # show error details
#
# Prerequisites:
#   - Go toolchain installed
#   - sqlite3 CLI installed
#   - python3 or perl (for floating-point conversion)
#
# Known Issues (MI300A timing config):
#   - MMU "page not found" panic for larger problem sizes (varies by benchmark)
#     vectoradd/relu crash at >= ~16K elements
#     fir crashes at all tested sizes
#   - bitonicsort is extremely slow to simulate (>120s even for 1024 elements)
#   - matrixtranspose produces empty metrics with CDNA3/MI300A config
#   - nbody returns identical kernel_time for 256 and 512 particles (possible bug)

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
OUTPUT="sim_results.csv"
TIMEOUT=300       # seconds per benchmark run; 0 = no timeout
SKIP_BUILD=0
VERBOSE=0

while [ $# -gt 0 ]; do
    case "$1" in
        --output)     OUTPUT="$2";     shift 2 ;;
        --timeout)    TIMEOUT="$2";    shift 2 ;;
        --skip-build) SKIP_BUILD=1;    shift ;;
        --verbose)    VERBOSE=1;       shift ;;
        *) echo "Unknown option: $1" >&2; exit 1 ;;
    esac
done

# ---------------------------------------------------------------------------
# Find repo root
# ---------------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Try to find repo root (look for go.mod)
if [ -f "${SCRIPT_DIR}/../go.mod" ]; then
    REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
elif [ -f "./go.mod" ]; then
    REPO_ROOT="$(pwd)"
else
    echo "ERROR: Cannot find repo root (go.mod). Run from repo root or gpu_perf_scripts/." >&2
    exit 1
fi

SAMPLES_DIR="${REPO_ROOT}/amd/samples"
BUILD_DIR="${REPO_ROOT}/_sim_benchmarks"
WORK_DIR=$(mktemp -d)

echo "Repo root:  ${REPO_ROOT}"
echo "Build dir:  ${BUILD_DIR}"
echo "Work dir:   ${WORK_DIR}"
echo "Output:     ${OUTPUT}"
echo "Timeout:    ${TIMEOUT}s"
echo ""

# Cleanup work dir on exit
trap 'rm -rf "${WORK_DIR}"' EXIT

# ---------------------------------------------------------------------------
# Common flags for all timing runs
# ---------------------------------------------------------------------------
COMMON_FLAGS="-timing -arch cdna3 -gpu mi300a -disable-rtm"

# ---------------------------------------------------------------------------
# Counters
# ---------------------------------------------------------------------------
pass=0
fail=0
skip=0
total=0

# ---------------------------------------------------------------------------
# Build a benchmark binary
# ---------------------------------------------------------------------------
build_bench() {
    local name="$1"
    local src_dir="${SAMPLES_DIR}/${name}"

    if [ ! -d "${src_dir}" ]; then
        echo "[SKIP] Source not found: ${src_dir}"
        return 1
    fi

    local bin="${BUILD_DIR}/${name}"
    mkdir -p "${BUILD_DIR}"

    if [ ${SKIP_BUILD} -eq 1 ] && [ -x "${bin}" ]; then
        return 0
    fi

    echo "[BUILD] ${name}..."
    if (cd "${REPO_ROOT}" && go build -o "${bin}" "./amd/samples/${name}/" 2>&1); then
        echo "[BUILD] ${name} OK"
        return 0
    else
        echo "[BUILD] ${name} FAILED"
        return 1
    fi
}

# ---------------------------------------------------------------------------
# Run a single benchmark configuration and extract kernel time
#
# Arguments:
#   $1 - benchmark binary name
#   $2 - CSV kernel_name (for output)
#   $3 - CSV problem_size label (for output)
#   $4... - additional flags for the benchmark
#
# Extracts kernel_time from the Driver row in the SQLite database.
# Converts from seconds to milliseconds for CSV output.
# ---------------------------------------------------------------------------
run_one() {
    local bin_name="$1"
    local csv_name="$2"
    local csv_size="$3"
    shift 3
    local extra_flags="$@"

    local bin="${BUILD_DIR}/${bin_name}"
    total=$((total + 1))

    if [ ! -x "${bin}" ]; then
        echo "[SKIP] Binary not found: ${bin}"
        skip=$((skip + 1))
        return
    fi

    # Clean up any old sqlite files in work dir
    rm -f "${WORK_DIR}"/akita_sim_*.sqlite3
    rm -f "${WORK_DIR}"/akita_data_recording_*.sqlite3

    echo "[RUN]  ${csv_name} ${csv_size}"

    local exit_code=0
    local cmd_output=""

    # Run the benchmark from the work directory
    # Use perl for timeout on macOS (no coreutils timeout)
    # shellcheck disable=SC2086
    if [ "${TIMEOUT}" -gt 0 ] 2>/dev/null; then
        cmd_output=$(cd "${WORK_DIR}" && perl -e '
            use POSIX;
            my $timeout = $ARGV[0];
            shift @ARGV;
            my $pid = fork();
            if ($pid == 0) {
                exec @ARGV;
                exit 127;
            }
            my $killed = 0;
            local $SIG{ALRM} = sub { kill "TERM", $pid; $killed = 1; };
            alarm $timeout;
            waitpid($pid, 0);
            alarm 0;
            if ($killed) { exit 124; }
            exit($? >> 8);
        ' "${TIMEOUT}" "${bin}" ${COMMON_FLAGS} ${extra_flags} 2>&1) || exit_code=$?
    else
        cmd_output=$(cd "${WORK_DIR}" && "${bin}" ${COMMON_FLAGS} ${extra_flags} 2>&1) || exit_code=$?
    fi

    if [ ${exit_code} -eq 124 ]; then
        echo "[TIMEOUT] ${csv_name} ${csv_size} (>${TIMEOUT}s wall clock)"
        fail=$((fail + 1))
        return
    fi

    if [ ${exit_code} -ne 0 ]; then
        echo "[FAIL] ${csv_name} ${csv_size} (exit ${exit_code})"
        if [ ${VERBOSE} -eq 1 ]; then
            echo "${cmd_output}" | tail -3
        fi
        fail=$((fail + 1))
        return
    fi

    # Find the SQLite database
    local db_file=""
    db_file=$(ls "${WORK_DIR}"/akita_sim_*.sqlite3 2>/dev/null | head -1)
    if [ -z "${db_file}" ]; then
        db_file=$(ls "${WORK_DIR}"/akita_data_recording_*.sqlite3 2>/dev/null | head -1)
    fi

    if [ -z "${db_file}" ] || [ ! -f "${db_file}" ]; then
        echo "[FAIL] ${csv_name} ${csv_size} (no SQLite output found)"
        fail=$((fail + 1))
        return
    fi

    # Extract kernel time from the Driver entry (total kernel time)
    local kernel_time_sec=""
    kernel_time_sec=$(sqlite3 "${db_file}" \
        "SELECT Value FROM mgpusim_metrics WHERE What='kernel_time' AND Location='Driver' LIMIT 1;" 2>/dev/null)

    if [ -z "${kernel_time_sec}" ]; then
        # Try GPU CommandProcessor if Driver not found
        kernel_time_sec=$(sqlite3 "${db_file}" \
            "SELECT Value FROM mgpusim_metrics WHERE What='kernel_time' LIMIT 1;" 2>/dev/null)
    fi

    if [ -z "${kernel_time_sec}" ]; then
        echo "[FAIL] ${csv_name} ${csv_size} (no kernel_time in DB)"
        fail=$((fail + 1))
        return
    fi

    # Convert seconds to milliseconds
    # kernel_time_sec can be in scientific notation like 3.834e-06
    local kernel_time_ms=""
    kernel_time_ms=$(python3 -c "print(f'{${kernel_time_sec} * 1000:.4f}')" 2>/dev/null || \
                     perl -e "printf('%.4f', ${kernel_time_sec} * 1000)")

    # For simulator results: avg = min = max (deterministic, 1 iteration)
    echo "${csv_name},${csv_size},1,${kernel_time_ms},${kernel_time_ms},${kernel_time_ms}" >> "${OUTPUT}"
    echo "[OK]   ${csv_name} ${csv_size} -> ${kernel_time_ms} ms (sim kernel time)"
    pass=$((pass + 1))
}

# ---------------------------------------------------------------------------
# Write CSV header
# ---------------------------------------------------------------------------
echo "kernel_name,problem_size,iterations,avg_ms,min_ms,max_ms" > "${OUTPUT}"

# ===========================================================================
# Build benchmarks
# ===========================================================================
echo "=== Building benchmarks ==="

# These are the benchmarks confirmed to work with CDNA3/MI300A timing:
# Excluded (known issues):
#   bitonicsort       - extremely slow (>120s for 1024 elements)
#   fir               - MMU page fault at all sizes
#   matrixtranspose   - produces empty metrics
#   simpleconvolution - extremely slow (>120s for 128x128)
BENCHMARKS_TO_BUILD=(
    vectoradd
    relu
    nbody
    stencil2d
    matrixmultiplication
)

BUILD_FAILED=()
for bench in "${BENCHMARKS_TO_BUILD[@]}"; do
    if ! build_bench "${bench}"; then
        BUILD_FAILED+=("${bench}")
    fi
done

echo ""
if [ ${#BUILD_FAILED[@]} -gt 0 ]; then
    echo "WARNING: Failed to build: ${BUILD_FAILED[*]}"
fi
echo ""

# ===========================================================================
# Run benchmarks
# ===========================================================================
echo "=== Running benchmarks ==="
echo ""

# ---------------------------------------------------------------------------
# vectoradd: -width N -height 1
# Works for sizes up to ~16384. Crashes with MMU page fault at larger sizes.
# ---------------------------------------------------------------------------
echo "--- vectoradd ---"
for size in 1024 2048 4096 8192 16384; do
    run_one vectoradd vectoradd "${size}" "-width ${size} -height 1"
done
echo ""

# ---------------------------------------------------------------------------
# relu: -length N
# Works for sizes up to ~8192. Page fault at ~16384.
# ---------------------------------------------------------------------------
echo "--- relu ---"
for size in 1024 2048 4096 8192; do
    run_one relu relu "${size}" "-length ${size}"
done
echo ""

# ---------------------------------------------------------------------------
# stencil2d: -row N -col N -iter I
# Works well. Uses -row/-col flags (not -x/-y).
# ---------------------------------------------------------------------------
echo "--- stencil2d ---"
for size in 64 128 256 512; do
    run_one stencil2d stencil2d "${size}x${size}" "-row ${size} -col ${size}"
done
echo ""

# ---------------------------------------------------------------------------
# nbody: -particles N
# Works but simulation is slow (~2.5s wall clock for 256 particles).
# ---------------------------------------------------------------------------
echo "--- nbody ---"
for size in 256 512 1024; do
    run_one nbody nbody "${size}_particles" "-particles ${size}"
done
echo ""

# ---------------------------------------------------------------------------
# matrixmultiplication: -x N -y N -z N
# ---------------------------------------------------------------------------
echo "--- matrixmultiplication ---"
for size in 32 64 96 128; do
    run_one matrixmultiplication matrixmultiplication "${size}x${size}x${size}" \
        "-x ${size} -y ${size} -z ${size}"
done
echo ""

# ---------------------------------------------------------------------------
# simpleconvolution: -width W -height H -mask-size M
# NOTE: Disabled by default — extremely slow simulation (>120s for 128x128).
# Uncomment to include if running on a fast machine with high timeout.
# ---------------------------------------------------------------------------
# echo "--- simpleconvolution ---"
# for size in 128 256; do
#     for mask in 3 5 7; do
#         run_one simpleconvolution simpleconvolution "${size}x${size}_mask${mask}" \
#             "-width ${size} -height ${size} -mask-size ${mask}"
#     done
# done
# echo ""

# ===========================================================================
# Summary
# ===========================================================================
echo "=========================================="
echo "Simulation benchmark run complete"
echo "  Passed:    ${pass}"
echo "  Failed:    ${fail}"
echo "  Skipped:   ${skip}"
echo "  Total:     ${total}"
echo "  Output:    ${OUTPUT}"
echo "=========================================="

if [ ${pass} -gt 0 ]; then
    echo ""
    echo "Results:"
    cat "${OUTPUT}"
fi

# Exit with failure if nothing passed
if [ ${pass} -eq 0 ]; then
    exit 1
fi
