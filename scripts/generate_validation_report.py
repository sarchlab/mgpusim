#!/usr/bin/env python3
"""Generate docs/validation_report.md from the selected-run comparison CSV.

This script is the single source of truth for the validation report.
It classifies data points into flat (overhead-dominated) and non-flat
(compute/memory-dominated) regions, checks compliance with the minimum
point requirements, fits per-benchmark boosters, computes average error
and scaling factor error, generates scaling figures, and writes the
final Markdown report.

Usage (from repo root):
    python3 scripts/generate_validation_report.py

By default, all-scope reports read the committed selected-run artifact at
benchmark-comparison/selected-run-25619929396/comparison_ci.detailed.csv.
Curated-scope reports keep the current top-level comparison_ci.csv default.

Policy reference: docs/validation_point_selection.md
"""

import argparse
import csv
import importlib
import json
import math
import os
import re
import struct
import sys
import tempfile
import zlib
from collections import Counter, defaultdict
from pathlib import Path


try:
    from validate_mi300a_evaluation_set import (
        ValidationError as ManifestValidationError,
        build_summary as build_manifest_summary,
        compute_coverage_stats as compute_manifest_coverage_stats,
        load_comparison_csv as load_manifest_comparison_csv,
        load_manifest as load_manifest_json,
        validate_coverage as validate_manifest_coverage,
        validate_manifest_shape,
    )
except ImportError:  # pragma: no cover - supports package-style imports in tests.
    from scripts.validate_mi300a_evaluation_set import (
        ValidationError as ManifestValidationError,
        build_summary as build_manifest_summary,
        compute_coverage_stats as compute_manifest_coverage_stats,
        load_comparison_csv as load_manifest_comparison_csv,
        load_manifest as load_manifest_json,
        validate_coverage as validate_manifest_coverage,
        validate_manifest_shape,
    )

try:
    import materialize_mi300a_regular_workflow_matrix as regular_matrix
except ImportError:  # pragma: no cover - supports package-style imports in tests.
    from scripts import materialize_mi300a_regular_workflow_matrix as regular_matrix

REPO_ROOT = Path(__file__).resolve().parent.parent
CSV_PATH = REPO_ROOT / "benchmark-comparison" / "comparison_ci.csv"
SELECTED_RUN_COMPARISON_PATH = (
    REPO_ROOT
    / "benchmark-comparison"
    / "selected-run-25619929396"
    / "comparison_ci.detailed.csv"
)
REPORT_PATH = REPO_ROOT / "docs" / "validation_report.md"
CURATED_REPORT_PATH = REPO_ROOT / "docs" / "validation_report_curated.md"
FIGURES_DIR = REPO_ROOT / "docs" / "figures"
DEFAULT_MANIFEST_PATH = REPO_ROOT / "benchmark-comparison" / "mi300a_evaluation_set.json"
BENCHMARK_TIERS_PATH = REPO_ROOT / "scripts" / "benchmark_tiers.json"
CURATED_SUMMARY_PATH = REPO_ROOT / "benchmark-comparison" / "mi300a_evaluation_set_summary.json"
REGULAR_MATRIX_REPORT_PATH = REPO_ROOT / "regular_matrix_validation.json"
REGULAR_PLAN_PATH = REPO_ROOT / "benchmark-comparison" / "mi300a_problem_size_discovery_plan.json"
REGULAR_COVERAGE_JSON_PATH = Path("regular_artifact_coverage_summary.json")
REGULAR_COVERAGE_MARKDOWN_PATH = Path("regular_artifact_coverage_summary.md")
REGULAR_COVERAGE_SCHEMA_NAME = "mgpusim.mi300a_regular_artifact_coverage_summary"
REGULAR_COVERAGE_SCHEMA_VERSION = 1
REGULAR_STAGE_NAMES = ("validation", "benchmark-tier1", "tier1-summary", "benchmark-tier2")
REGULAR_CSV_NAMES = ("simulation", "comparison", "regression")
REGULAR_REQUIRED_CSV_COLUMNS = {
    "simulation": (
        "kernel_name",
        "problem_size",
        "iterations",
        "avg_ms",
        "min_ms",
        "max_ms",
    ),
    "comparison": (
        "kernel_name",
        "problem_size",
        "real_ms",
        "sim_ms",
        "abs_error_ms",
        "rel_error",
    ),
    "regression": (
        "kernel",
        "group",
        "label",
        "problem_size",
        "numeric_size",
        "hw_ms",
        "sim_ms",
    ),
}
REGULAR_COMPLETE_LABEL = "complete_regular_evidence"
REGULAR_PARTIAL_LABEL = "partial_diagnostic_regular_evidence"
REGULAR_INSUFFICIENT_LABEL = "insufficient_regular_evidence"
CURATED_FIGURE_PREFIX = "curated_"
CURATED_DIAGNOSTIC_POLICY = (
    "Curated primary benchmarks are the tuning/evaluation surface for this "
    "report. Non-selected broad-suite kernels remain secondary diagnostics and "
    "regression signals; they are excluded here unless promoted into the "
    "curated manifest."
)

class PlottingDependencyError(RuntimeError):
    """Raised when validation-report figure generation cannot import matplotlib."""


_PLOTTING_MODULES = None
_PLOTTING_IMPORT_ERROR = None


def load_plotting_modules():
    """Return matplotlib plotting modules or raise PlottingDependencyError."""
    global _PLOTTING_MODULES, _PLOTTING_IMPORT_ERROR
    if _PLOTTING_MODULES is False:
        raise PlottingDependencyError(plotting_required_message())
    if _PLOTTING_MODULES is not None:
        return _PLOTTING_MODULES

    try:
        matplotlib = importlib.import_module("matplotlib")
        matplotlib.use("Agg")
        plt = importlib.import_module("matplotlib.pyplot")
        patches = importlib.import_module("matplotlib.patches")
    except Exception as exc:  # pragma: no cover - exercised via subprocess tests.
        _PLOTTING_MODULES = False
        _PLOTTING_IMPORT_ERROR = exc
        raise PlottingDependencyError(plotting_required_message()) from exc

    _PLOTTING_MODULES = {"plt": plt, "Patch": patches.Patch}
    return _PLOTTING_MODULES


def plotting_unavailable_reason():
    """Return a stable description of a matplotlib import failure."""
    if _PLOTTING_IMPORT_ERROR is None:
        return "matplotlib could not be imported"
    return str(_PLOTTING_IMPORT_ERROR) or type(_PLOTTING_IMPORT_ERROR).__name__


def plotting_required_message():
    """Return the fail-loud message for missing plotting dependencies."""
    return (
        "matplotlib is required to generate validation report figures; "
        "install matplotlib before running scripts/generate_validation_report.py "
        f"({plotting_unavailable_reason()})."
    )


# Minimum point requirements (from validation_point_selection.md §3)
MIN_FLAT_POINTS = 1
MIN_NONFLAT_POINTS = 5

# Tier classification (19 Tier 1 / 66 Tier 2)
# Tier 1: microbenchmarks that probe specific system parameters
# Tier 2: algorithm kernels (default for any kernel not listed below)
TIER1_BENCHMARKS = frozenset([
    "mem_latency_chase",
    "l1_cache_bw",
    "l2_cache_bw",
    "shared_bw",
    "global_bw_copy",
    "devicememory_read",
    "devicememory_write",
    "busspeeddownload",
    "busspeedreadback",
    "maxflops",
    "fp32_fma",
    "fp64_fma",
    "int_mad",
    "sfun_sin",
    "occupancy_fma",
    "atomic_throughput",
    "branch_div_50pct",
    "triad",
    "reduction",
])

# Accuracy targets per tier
TIER1_AVG_ERR_TARGET = 0.10   # ≤ 10%
TIER2_AVG_ERR_TARGET = 0.20   # ≤ 20%


def get_tier(kernel_name: str) -> int:
    """Return tier (1 or 2) for a kernel using the broad-suite default map."""
    return 1 if kernel_name in TIER1_BENCHMARKS else 2


def lookup_tier(kernel_name: str, tier_lookup=None) -> int:
    """Return manifest-provided tier when available, else broad-suite tier."""
    if tier_lookup and kernel_name in tier_lookup:
        return tier_lookup[kernel_name]
    return get_tier(kernel_name)


def parse_args(argv=None):
    parser = argparse.ArgumentParser(
        description=(
            "Generate the MI300A validation report from comparison_ci.csv. "
            "Default all scope reads the selected-run comparison artifact; "
            "curated scope filters to mi300a_evaluation_set.json."
        )
    )
    parser.add_argument(
        "--scope",
        choices=("all", "curated"),
        default="all",
        help="Report scope to generate (default: all).",
    )
    parser.add_argument(
        "--csv",
        default=None,
        help=(
            "Path to comparison CSV. Defaults to "
            "benchmark-comparison/selected-run-25619929396/comparison_ci.detailed.csv "
            "for --scope all and benchmark-comparison/comparison_ci.csv for "
            "--scope curated."
        ),
    )
    parser.add_argument(
        "--manifest",
        default=str(DEFAULT_MANIFEST_PATH),
        help=(
            "Curated-set manifest path for --scope curated "
            "(default: benchmark-comparison/mi300a_evaluation_set.json)."
        ),
    )
    parser.add_argument(
        "--output",
        default=None,
        help=(
            "Markdown report output path. Defaults to docs/validation_report.md "
            "for --scope all and docs/validation_report_curated.md for --scope curated."
        ),
    )
    parser.add_argument(
        "--summary-json",
        default=None,
        help=(
            "Optional deterministic summary JSON path. Curated scope defaults to "
            "benchmark-comparison/mi300a_evaluation_set_summary.json."
        ),
    )
    parser.add_argument(
        "--figures-dir",
        default=str(FIGURES_DIR),
        help="Directory for generated figures (default: docs/figures).",
    )
    parser.add_argument(
        "--regular-artifact-coverage",
        "--regular-artifact-coverage-summary",
        action="store_true",
        dest="regular_artifact_coverage",
        help=(
            "Generate the deterministic repo-only regular workflow artifact "
            "coverage summary instead of the validation accuracy report."
        ),
    )
    parser.add_argument(
        "--regular-matrix-report",
        default=None,
        help=(
            "Path to regular_matrix_validation.json. Defaults to the checked-in "
            "repo report when present, otherwise materializes expectations from --regular-plan."
        ),
    )
    parser.add_argument(
        "--regular-plan",
        default=str(REGULAR_PLAN_PATH),
        help=(
            "Path to mi300a_problem_size_discovery_plan.json used only when no "
            "regular matrix report is available."
        ),
    )
    parser.add_argument(
        "--regular-sim-csv",
        "--regular-simulation-csv",
        default="sim_results_ci.csv",
        dest="regular_sim_csv",
        help="Merged simulator CSV to count (default: sim_results_ci.csv).",
    )
    parser.add_argument(
        "--regular-comparison-csv",
        default="comparison_ci.csv",
        help="Comparison CSV to count (default: comparison_ci.csv).",
    )
    parser.add_argument(
        "--regular-regression-csv",
        default="regression_ci.csv",
        help="Regression CSV to count (default: regression_ci.csv).",
    )
    parser.add_argument(
        "--regular-stage-result",
        "--upstream-stage-result",
        action="append",
        default=[],
        metavar="STAGE=RESULT",
        help=(
            "Required upstream stage result, repeated for validation, benchmark-tier1, "
            "tier1-summary, and benchmark-tier2. Missing stages are treated as unknown."
        ),
    )
    parser.add_argument(
        "--regular-coverage-json",
        "--regular-artifact-coverage-json",
        default=str(REGULAR_COVERAGE_JSON_PATH),
        help=(
            "Path to write deterministic regular artifact coverage JSON "
            "(default: regular_artifact_coverage_summary.json)."
        ),
    )
    parser.add_argument(
        "--regular-coverage-md",
        "--regular-coverage-markdown",
        "--regular-artifact-coverage-md",
        default=str(REGULAR_COVERAGE_MARKDOWN_PATH),
        help=(
            "Path to write deterministic regular artifact coverage Markdown "
            "(default: regular_artifact_coverage_summary.md)."
        ),
    )
    return parser.parse_args(argv)


def default_csv_path(scope: str) -> Path:
    """Return the implicit comparison CSV for the requested report scope."""
    return CSV_PATH if scope == "curated" else SELECTED_RUN_COMPARISON_PATH


def resolve_csv_path(scope: str, csv_arg) -> Path:
    """Return the explicit --csv path or the scope-specific default."""
    return Path(csv_arg) if csv_arg else default_csv_path(scope)


def default_output_path(scope: str) -> Path:
    return CURATED_REPORT_PATH if scope == "curated" else REPORT_PATH


def default_summary_path(scope: str):
    return CURATED_SUMMARY_PATH if scope == "curated" else None


def repo_relative(path: Path) -> str:
    try:
        return path.resolve().relative_to(REPO_ROOT).as_posix()
    except ValueError:
        return path.as_posix()


def figure_link_prefix(output_path: Path, figures_dir: Path) -> str:
    rel = os.path.relpath(figures_dir, output_path.parent)
    return "" if rel == "." else rel.replace(os.sep, "/")


PNG_SIGNATURE = b"\x89PNG\r\n\x1a\n"
PNG_COLOR_TYPE_SAMPLES = {0: 1, 2: 3, 3: 1, 4: 2, 6: 4}
PNG_RENDERER_DRIFT_MAX_CHANNEL_DELTA = 1
PNG_RENDERER_DRIFT_MAX_CHANGED_FRACTION = 0.0001


def write_text_if_changed(path: Path, text: str, encoding="utf-8") -> bool:
    """Write text only when content differs; return True when rewritten."""
    path = Path(path)
    try:
        if path.read_text(encoding=encoding) == text:
            return False
    except FileNotFoundError:
        pass
    path.write_text(text, encoding=encoding)
    return True


def png_files_materially_equal(existing_path: Path, candidate_path: Path) -> bool:
    """Return True when two PNGs render the same pixels despite byte drift."""
    try:
        existing_bytes = Path(existing_path).read_bytes()
        candidate_bytes = Path(candidate_path).read_bytes()
    except OSError:
        return False

    if existing_bytes == candidate_bytes:
        return True
    if (
        Path(existing_path).suffix.lower() != ".png"
        or Path(candidate_path).suffix.lower() != ".png"
    ):
        return False

    try:
        existing_payload = _png_material_payload(existing_bytes)
        candidate_payload = _png_material_payload(candidate_bytes)
    except (ValueError, zlib.error, struct.error):
        return False
    return _png_material_payloads_equal(existing_payload, candidate_payload)


def _png_material_payloads_equal(existing_payload, candidate_payload) -> bool:
    existing_ihdr, existing_chunks, existing_pixels = existing_payload
    candidate_ihdr, candidate_chunks, candidate_pixels = candidate_payload
    if existing_ihdr != candidate_ihdr or existing_chunks != candidate_chunks:
        return False
    if existing_pixels == candidate_pixels:
        return True
    return _png_pixels_within_renderer_drift(existing_pixels, candidate_pixels)


def _png_pixels_within_renderer_drift(
    existing_pixels: bytes, candidate_pixels: bytes
) -> bool:
    if len(existing_pixels) != len(candidate_pixels):
        return False
    allowed_changed = max(
        1,
        int(len(existing_pixels) * PNG_RENDERER_DRIFT_MAX_CHANGED_FRACTION),
    )
    changed = 0
    for existing, candidate in zip(existing_pixels, candidate_pixels):
        if existing == candidate:
            continue
        if abs(existing - candidate) > PNG_RENDERER_DRIFT_MAX_CHANNEL_DELTA:
            return False
        changed += 1
        if changed > allowed_changed:
            return False
    return True


def save_figure_if_materially_changed(
    fig, output_path: Path, **savefig_kwargs
) -> bool:
    """Save a figure unless an existing PNG already has identical pixels."""
    output_path = Path(output_path)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    suffix = output_path.suffix or ".tmp"
    with tempfile.NamedTemporaryFile(
        dir=output_path.parent,
        prefix=f".{output_path.name}.",
        suffix=suffix,
        delete=False,
    ) as tmp:
        tmp_path = Path(tmp.name)

    try:
        fig.savefig(tmp_path, **savefig_kwargs)
        if output_path.exists() and png_files_materially_equal(
            output_path, tmp_path
        ):
            tmp_path.unlink()
            return False
        os.replace(tmp_path, output_path)
        return True
    finally:
        if tmp_path.exists():
            tmp_path.unlink()


def _png_material_payload(data: bytes):
    """Return PNG fields that determine generated figure appearance."""
    if not data.startswith(PNG_SIGNATURE):
        raise ValueError("not a PNG file")

    pos = len(PNG_SIGNATURE)
    ihdr = None
    idat_chunks = []
    appearance_chunks = []
    while pos < len(data):
        if pos + 8 > len(data):
            raise ValueError("truncated PNG chunk header")
        length = struct.unpack(">I", data[pos:pos + 4])[0]
        chunk_type = data[pos + 4:pos + 8]
        chunk_start = pos + 8
        chunk_end = chunk_start + length
        crc_end = chunk_end + 4
        if crc_end > len(data):
            raise ValueError("truncated PNG chunk payload")
        chunk_data = data[chunk_start:chunk_end]

        if chunk_type == b"IHDR":
            ihdr = struct.unpack(">IIBBBBB", chunk_data)
        elif chunk_type == b"IDAT":
            idat_chunks.append(chunk_data)
        elif chunk_type in (b"PLTE", b"tRNS"):
            appearance_chunks.append((chunk_type, chunk_data))
        elif chunk_type == b"IEND":
            break
        pos = crc_end

    if ihdr is None:
        raise ValueError("PNG missing IHDR")
    if not idat_chunks:
        raise ValueError("PNG missing IDAT")

    raw_scanlines = zlib.decompress(b"".join(idat_chunks))
    return (
        ihdr,
        tuple(appearance_chunks),
        _png_unfiltered_pixels(ihdr, raw_scanlines),
    )


def _png_unfiltered_pixels(ihdr, raw_scanlines: bytes) -> bytes:
    width, height, bit_depth, color_type, compression, filter_method, interlace = (
        ihdr
    )
    if (
        compression != 0
        or filter_method != 0
        or interlace != 0
        or color_type not in PNG_COLOR_TYPE_SAMPLES
    ):
        return raw_scanlines
    samples = PNG_COLOR_TYPE_SAMPLES[color_type]

    bits_per_pixel = samples * bit_depth
    row_length = (width * bits_per_pixel + 7) // 8
    bytes_per_pixel = max(1, (bits_per_pixel + 7) // 8)
    expected_length = height * (row_length + 1)
    if len(raw_scanlines) != expected_length:
        raise ValueError("PNG scanline length does not match IHDR")

    prior = bytearray(row_length)
    pixels = bytearray(row_length * height)
    src_pos = 0
    dst_pos = 0
    for _ in range(height):
        filter_type = raw_scanlines[src_pos]
        src_pos += 1
        row = bytearray(raw_scanlines[src_pos:src_pos + row_length])
        src_pos += row_length
        for idx, value in enumerate(row):
            left = row[idx - bytes_per_pixel] if idx >= bytes_per_pixel else 0
            up = prior[idx]
            up_left = prior[idx - bytes_per_pixel] if idx >= bytes_per_pixel else 0
            if filter_type == 0:
                decoded = value
            elif filter_type == 1:
                decoded = value + left
            elif filter_type == 2:
                decoded = value + up
            elif filter_type == 3:
                decoded = value + ((left + up) // 2)
            elif filter_type == 4:
                decoded = value + _png_paeth_predictor(left, up, up_left)
            else:
                raise ValueError(f"unsupported PNG filter type {filter_type}")
            row[idx] = decoded & 0xFF
        pixels[dst_pos:dst_pos + row_length] = row
        dst_pos += row_length
        prior = row
    return bytes(pixels)


def _png_paeth_predictor(left: int, up: int, up_left: int) -> int:
    estimate = left + up - up_left
    distance_left = abs(estimate - left)
    distance_up = abs(estimate - up)
    distance_up_left = abs(estimate - up_left)
    if distance_left <= distance_up and distance_left <= distance_up_left:
        return left
    if distance_up <= distance_up_left:
        return up
    return up_left


class RegularCoverageError(Exception):
    """Raised when regular artifact coverage inputs are malformed."""


def regular_int(value, label: str) -> int:
    """Parse a non-negative integer field from regular coverage metadata."""
    if isinstance(value, bool):
        raise RegularCoverageError(f"{label} must be an integer, got boolean")
    try:
        parsed = int(value)
    except (TypeError, ValueError) as exc:
        raise RegularCoverageError(f"{label} must be an integer, got {value!r}") from exc
    if parsed < 0:
        raise RegularCoverageError(f"{label} must be non-negative, got {parsed}")
    return parsed


def load_json_object_for_regular_coverage(path: Path):
    """Load a JSON object for the regular artifact coverage path."""
    try:
        with path.open("r", encoding="utf-8") as f:
            data = json.load(f)
    except FileNotFoundError as exc:
        raise RegularCoverageError(f"{repo_relative(path)} not found") from exc
    except json.JSONDecodeError as exc:
        raise RegularCoverageError(f"{repo_relative(path)} invalid JSON: {exc}") from exc
    if not isinstance(data, dict):
        raise RegularCoverageError(f"{repo_relative(path)} JSON root must be an object")
    return data


def regular_expectations_from_matrix_report(report_path: Path):
    """Return expected regular row counts from a matrix validation report."""
    report = load_json_object_for_regular_coverage(report_path)
    if report.get("schema_name") != regular_matrix.VALIDATION_SCHEMA:
        raise RegularCoverageError(
            f"{repo_relative(report_path)} schema_name must be {regular_matrix.VALIDATION_SCHEMA!r}"
        )
    if report.get("status") != "pass":
        raise RegularCoverageError(
            f"{repo_relative(report_path)} status must be 'pass', got {report.get('status')!r}"
        )
    matrix = report.get("matrix")
    if not isinstance(matrix, dict):
        raise RegularCoverageError(f"{repo_relative(report_path)}.matrix must be an object")
    jobs = matrix.get("jobs")
    if not isinstance(jobs, dict):
        raise RegularCoverageError(f"{repo_relative(report_path)}.matrix.jobs must be an object")

    job_expectations = {}
    for job in ("benchmark-tier1", "benchmark-tier2"):
        job_data = jobs.get(job)
        if not isinstance(job_data, dict):
            raise RegularCoverageError(f"{repo_relative(report_path)}.matrix.jobs.{job} must be an object")
        job_expectations[job] = {
            "matrix_rows": regular_int(job_data.get("matrix_rows"), f"matrix.jobs.{job}.matrix_rows"),
            "expected_rows": regular_int(job_data.get("plan_entries"), f"matrix.jobs.{job}.plan_entries"),
        }

    total_matrix_rows = regular_int(matrix.get("total_matrix_rows"), "matrix.total_matrix_rows")
    total_rows = regular_int(matrix.get("total_plan_entries"), "matrix.total_plan_entries")
    computed_matrix_rows = sum(job["matrix_rows"] for job in job_expectations.values())
    computed_rows = sum(job["expected_rows"] for job in job_expectations.values())
    if total_matrix_rows != computed_matrix_rows:
        raise RegularCoverageError(
            f"matrix.total_matrix_rows is {total_matrix_rows}, expected {computed_matrix_rows} from tier jobs"
        )
    if total_rows != computed_rows:
        raise RegularCoverageError(
            f"matrix.total_plan_entries is {total_rows}, expected {computed_rows} from tier jobs"
        )

    return {
        "source_kind": "matrix_report",
        "matrix_report": repo_relative(report_path),
        "source_plan": report.get("source_plan"),
        "tier1_rows": job_expectations["benchmark-tier1"]["expected_rows"],
        "tier2_rows": job_expectations["benchmark-tier2"]["expected_rows"],
        "total_rows": total_rows,
        "tier1_matrix_rows": job_expectations["benchmark-tier1"]["matrix_rows"],
        "tier2_matrix_rows": job_expectations["benchmark-tier2"]["matrix_rows"],
        "total_matrix_rows": total_matrix_rows,
        "jobs": job_expectations,
    }


def regular_expectations_from_plan(plan_path: Path):
    """Materialize expected regular row counts from the checked-in plan."""
    try:
        plan, entries = regular_matrix.load_plan_entries(plan_path)
        by_job = regular_matrix.group_entries(entries)
        report = regular_matrix.build_report(plan_path, plan, by_job)
    except regular_matrix.MatrixError as exc:
        raise RegularCoverageError(str(exc)) from exc

    tmp_report_path = REGULAR_MATRIX_REPORT_PATH
    expectations = {
        "source_kind": "materialized_plan",
        "matrix_report": None,
        "source_plan": repo_relative(plan_path),
        "tier1_rows": report["matrix"]["jobs"]["benchmark-tier1"]["plan_entries"],
        "tier2_rows": report["matrix"]["jobs"]["benchmark-tier2"]["plan_entries"],
        "total_rows": report["matrix"]["total_plan_entries"],
        "tier1_matrix_rows": report["matrix"]["jobs"]["benchmark-tier1"]["matrix_rows"],
        "tier2_matrix_rows": report["matrix"]["jobs"]["benchmark-tier2"]["matrix_rows"],
        "total_matrix_rows": report["matrix"]["total_matrix_rows"],
        "jobs": {
            "benchmark-tier1": {
                "matrix_rows": report["matrix"]["jobs"]["benchmark-tier1"]["matrix_rows"],
                "expected_rows": report["matrix"]["jobs"]["benchmark-tier1"]["plan_entries"],
            },
            "benchmark-tier2": {
                "matrix_rows": report["matrix"]["jobs"]["benchmark-tier2"]["matrix_rows"],
                "expected_rows": report["matrix"]["jobs"]["benchmark-tier2"]["plan_entries"],
            },
        },
    }
    if tmp_report_path.exists():
        expectations["checked_in_matrix_report"] = repo_relative(tmp_report_path)
    return expectations


def load_regular_expectations(matrix_report_arg, plan_path: Path):
    """Load expected regular Tier 1/Tier 2 row counts for coverage summaries."""
    if matrix_report_arg:
        matrix_report_path = Path(matrix_report_arg)
        if not matrix_report_path.exists():
            raise RegularCoverageError(f"{repo_relative(matrix_report_path)} not found")
        return regular_expectations_from_matrix_report(matrix_report_path)

    if REGULAR_MATRIX_REPORT_PATH.exists():
        return regular_expectations_from_matrix_report(REGULAR_MATRIX_REPORT_PATH)
    return regular_expectations_from_plan(plan_path)


def build_regular_workflow_contract(plan_path: Path = REGULAR_PLAN_PATH):
    """Return the maintained regular workflow contract for report context."""
    expectations = regular_expectations_from_plan(Path(plan_path))
    return {
        "source_plan": expectations["source_plan"],
        "tier1_matrix_rows": expectations["tier1_matrix_rows"],
        "tier1_rows": expectations["tier1_rows"],
        "tier2_matrix_rows": expectations["tier2_matrix_rows"],
        "tier2_rows": expectations["tier2_rows"],
        "total_matrix_rows": expectations["total_matrix_rows"],
        "total_rows": expectations["total_rows"],
        "per_simulator_timeout_sec": regular_matrix.EXPECTED_PER_SIMULATOR_TIMEOUT_SEC,
        "benchmark_job_timeout_sec": regular_matrix.EXPECTED_BENCHMARK_JOB_TIMEOUT_SEC,
        "max_parallel": regular_matrix.EXPECTED_MAX_PARALLEL,
        "max_invocations_within_job_timeout": regular_matrix.max_invocations_within_job_timeout(),
        "workflow_order": "Validation -> Tier 1 -> Tier-1 summary -> Tier 2 -> Summary/report",
    }


def parse_regular_stage_results(stage_args):
    """Parse repeated STAGE=RESULT inputs and default missing stages to unknown."""
    results = {stage: "unknown" for stage in REGULAR_STAGE_NAMES}
    seen = set()
    for raw in stage_args or []:
        if "=" not in raw:
            raise RegularCoverageError(f"stage result {raw!r} must use STAGE=RESULT")
        stage, result = raw.split("=", 1)
        stage = stage.strip()
        result = result.strip().lower() or "unknown"
        if stage not in REGULAR_STAGE_NAMES:
            raise RegularCoverageError(
                f"unknown regular stage {stage!r}; expected one of {', '.join(REGULAR_STAGE_NAMES)}"
            )
        if stage in seen:
            raise RegularCoverageError(f"duplicate regular stage result for {stage}")
        seen.add(stage)
        results[stage] = result
    return [
        {
            "stage": stage,
            "result": results[stage],
            "success": results[stage] == "success",
        }
        for stage in REGULAR_STAGE_NAMES
    ]


def normalize_regular_csv_header(header):
    """Return normalized CSV header columns for regular artifact recognition."""
    normalized = []
    for item in header:
        normalized.append(str(item).lstrip("\ufeff").strip().lower())
    return normalized


def missing_regular_csv_header_columns(name: str, header):
    """Return required regular CSV columns absent from the observed header."""
    required_columns = REGULAR_REQUIRED_CSV_COLUMNS[name]
    observed_columns = set(normalize_regular_csv_header(header))
    return [column for column in required_columns if column not in observed_columns]


def inspect_regular_csv(name: str, path: Path, expected_rows: int):
    """Count observed data rows in one regular artifact CSV."""
    record = {
        "name": name,
        "path": repo_relative(path),
        "exists": path.exists(),
        "header_columns": [],
        "required_header_columns": list(REGULAR_REQUIRED_CSV_COLUMNS[name]),
        "missing_header_columns": list(REGULAR_REQUIRED_CSV_COLUMNS[name]),
        "header_recognized": False,
        "data_rows": 0,
        "expected_rows": expected_rows,
        "required_for_complete": True,
        "matches_expected": False,
        "status": "missing_csv",
    }
    if not path.exists():
        return record

    try:
        with path.open("r", encoding="utf-8", errors="replace", newline="") as f:
            reader = csv.reader(f)
            header = next(reader, None)
            if header is None:
                record["status"] = "missing_header"
                return record
            record["header_columns"] = [str(item) for item in header]
            record["missing_header_columns"] = missing_regular_csv_header_columns(name, header)
            record["header_recognized"] = not record["missing_header_columns"]
            record["data_rows"] = sum(1 for _ in reader)
    except OSError as exc:
        raise RegularCoverageError(f"could not read {repo_relative(path)}: {exc}") from exc

    if not record["header_recognized"]:
        record["status"] = "malformed_header"
    elif record["data_rows"] == expected_rows:
        record["matches_expected"] = True
        record["status"] = "matches_expected"
    elif record["data_rows"] == 0 and name in ("comparison", "regression"):
        record["status"] = "empty_reference_fallback_csv"
    elif record["data_rows"] == 0:
        record["status"] = "empty_csv"
    else:
        record["status"] = "row_count_mismatch"
    return record


def regular_coverage_reasons(stage_records, observed_records):
    """Build deterministic reasons explaining incomplete regular evidence."""
    reasons = []
    for stage in stage_records:
        if stage["success"]:
            continue
        reasons.append(f"upstream stage {stage['stage']} result is {stage['result']}")

    for name in REGULAR_CSV_NAMES:
        record = observed_records[name]
        path = record["path"]
        data_rows = record["data_rows"]
        expected_rows = record["expected_rows"]
        status = record["status"]
        if status == "matches_expected":
            continue
        if status == "missing_csv":
            reasons.append(f"{name} CSV {path} is missing")
        elif status == "missing_header":
            reasons.append(f"{name} CSV {path} is empty or missing a header")
        elif status == "malformed_header":
            missing_columns = ", ".join(record["missing_header_columns"])
            reasons.append(
                f"{name} CSV {path} header is not recognizable; "
                f"missing required header columns: {missing_columns}"
            )
        elif status == "empty_reference_fallback_csv":
            reasons.append(
                f"{name} CSV {path} has 0 data rows; this is an empty reference fallback, "
                "not complete comparison/regression evidence"
            )
        elif status == "empty_csv":
            reasons.append(f"{name} CSV {path} has 0 data rows; expected {expected_rows}")
        else:
            reasons.append(f"{name} CSV {path} has {data_rows} data rows; expected {expected_rows}")
    return reasons


def classify_regular_evidence(stage_records, observed_records):
    """Return the deterministic regular evidence label and completion boolean."""
    stages_success = all(stage["success"] for stage in stage_records)
    csvs_match = all(record["matches_expected"] for record in observed_records.values())
    if stages_success and csvs_match:
        return REGULAR_COMPLETE_LABEL, True

    severe_statuses = {
        "missing_csv",
        "missing_header",
        "malformed_header",
        "empty_csv",
        "empty_reference_fallback_csv",
    }
    if any(record["status"] in severe_statuses for record in observed_records.values()):
        return REGULAR_INSUFFICIENT_LABEL, False
    return REGULAR_PARTIAL_LABEL, False


def build_regular_artifact_coverage_summary(
    *,
    matrix_report_arg=None,
    plan_path=REGULAR_PLAN_PATH,
    sim_csv=Path("sim_results_ci.csv"),
    comparison_csv=Path("comparison_ci.csv"),
    regression_csv=Path("regression_ci.csv"),
    stage_args=None,
):
    """Build deterministic JSON data for the regular artifact coverage CLI."""
    expectations = load_regular_expectations(matrix_report_arg, Path(plan_path))
    expected_total_rows = expectations["total_rows"]
    stage_records = parse_regular_stage_results(stage_args or [])
    observed_records = {
        "simulation": inspect_regular_csv("simulation", Path(sim_csv), expected_total_rows),
        "comparison": inspect_regular_csv("comparison", Path(comparison_csv), expected_total_rows),
        "regression": inspect_regular_csv("regression", Path(regression_csv), expected_total_rows),
    }
    reasons = regular_coverage_reasons(stage_records, observed_records)
    evidence_label, complete = classify_regular_evidence(stage_records, observed_records)

    return {
        "schema_name": REGULAR_COVERAGE_SCHEMA_NAME,
        "schema_version": REGULAR_COVERAGE_SCHEMA_VERSION,
        "evidence_label": evidence_label,
        "complete_regular_evidence": complete,
        "expected": {
            "source_kind": expectations["source_kind"],
            "matrix_report": expectations.get("matrix_report"),
            "checked_in_matrix_report": expectations.get("checked_in_matrix_report"),
            "source_plan": expectations.get("source_plan"),
            "tier1_rows": expectations["tier1_rows"],
            "tier2_rows": expectations["tier2_rows"],
            "total_rows": expectations["total_rows"],
            "tier1_matrix_rows": expectations["tier1_matrix_rows"],
            "tier2_matrix_rows": expectations["tier2_matrix_rows"],
            "total_matrix_rows": expectations["total_matrix_rows"],
            "jobs": expectations["jobs"],
        },
        "upstream_stages": stage_records,
        "observed": observed_records,
        "reasons": reasons,
    }


def generate_regular_artifact_coverage_markdown(summary) -> str:
    """Render regular artifact coverage JSON data as deterministic Markdown."""
    expected = summary["expected"]
    observed = summary["observed"]
    lines = [
        "# MI300A Regular Artifact Coverage Summary",
        "",
        f"**Evidence label:** `{summary['evidence_label']}`  ",
        f"**Complete regular evidence:** `{str(summary['complete_regular_evidence']).lower()}`",
        "",
        "This repo-only summary counts checked-in/materialized regular workflow expectations "
        "against observed CSV artifact rows. It does not dispatch workflows, run simulators, "
        "collect artifacts, or fabricate outcomes.",
        "",
        "## Label interpretation",
        "",
        (
            f"- `{REGULAR_COMPLETE_LABEL}`: every required upstream stage succeeded and "
            "simulation, comparison, and regression CSV data-row counts all match the "
            "expected regular row count. This is complete regular artifact coverage "
            "only, not finishability/provenance completion or proof of simulator "
            "accuracy."
        ),
        (
            f"- `{REGULAR_PARTIAL_LABEL}`: required CSVs are present with recognizable "
            "headers but one or more upstream stages are non-success/unknown or a "
            "non-empty CSV row count does not match. Treat the artifacts as partial "
            "diagnostic regular evidence."
        ),
        (
            f"- `{REGULAR_INSUFFICIENT_LABEL}`: a required CSV is missing, lacks a "
            "recognizable header, is empty, or comparison/regression used an empty "
            "reference fallback. There is not enough observed artifact evidence to "
            "claim regular coverage."
        ),
        "",
        "## Expected regular rows",
        "",
        "| Tier | Matrix rows | Expected data rows |",
        "|---|---:|---:|",
        (
            f"| benchmark-tier1 | {expected['tier1_matrix_rows']} | "
            f"{expected['tier1_rows']} |"
        ),
        (
            f"| benchmark-tier2 | {expected['tier2_matrix_rows']} | "
            f"{expected['tier2_rows']} |"
        ),
        f"| total | {expected['total_matrix_rows']} | {expected['total_rows']} |",
        "",
        "## Upstream stage results",
        "",
        "| Stage | Result | Success required for complete evidence |",
        "|---|---|---:|",
    ]
    for stage in summary["upstream_stages"]:
        lines.append(
            f"| {stage['stage']} | `{stage['result']}` | "
            f"{str(stage['success']).lower()} |"
        )

    lines.extend([
        "",
        "## Observed CSV row coverage",
        "",
        "| Artifact | CSV path | Data rows | Expected rows | Status |",
        "|---|---|---:|---:|---|",
    ])
    for name in REGULAR_CSV_NAMES:
        record = observed[name]
        lines.append(
            f"| {name} | `{record['path']}` | {record['data_rows']} | "
            f"{record['expected_rows']} | `{record['status']}` |"
        )

    lines.extend(["", "## Evidence reasons", ""])
    if summary["reasons"]:
        for reason in summary["reasons"]:
            lines.append(f"- {reason}")
    else:
        lines.append("- All upstream stages succeeded and all required CSV row counts matched expected rows.")
    lines.append("")
    return "\n".join(lines)


def run_regular_artifact_coverage(args) -> int:
    """CLI entry point for regular artifact coverage summaries."""
    try:
        summary = build_regular_artifact_coverage_summary(
            matrix_report_arg=args.regular_matrix_report,
            plan_path=Path(args.regular_plan),
            sim_csv=Path(args.regular_sim_csv),
            comparison_csv=Path(args.regular_comparison_csv),
            regression_csv=Path(args.regular_regression_csv),
            stage_args=args.regular_stage_result,
        )
    except RegularCoverageError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1

    markdown = generate_regular_artifact_coverage_markdown(summary)
    json_text = json.dumps(summary, indent=2, sort_keys=True) + "\n"

    json_path = Path(args.regular_coverage_json) if args.regular_coverage_json else None
    if json_path is not None:
        json_path.parent.mkdir(parents=True, exist_ok=True)
        json_path.write_text(json_text, encoding="utf-8")
    markdown_path = Path(args.regular_coverage_md) if args.regular_coverage_md else None
    if markdown_path is not None:
        markdown_path.parent.mkdir(parents=True, exist_ok=True)
        markdown_path.write_text(markdown, encoding="utf-8")

    print(json_text, end="")
    return 0


def load_and_validate_curated_manifest(manifest_path: Path, csv_path: Path):
    """Validate curated manifest/CSV pair and return manifest, entries, summary."""
    rows = load_manifest_comparison_csv(csv_path)
    manifest = load_manifest_json(manifest_path)
    entries = validate_manifest_shape(manifest)
    stats_by_kernel = compute_manifest_coverage_stats(rows)
    validate_manifest_coverage(entries, rows, stats_by_kernel)
    summary = build_manifest_summary(manifest, entries, rows, stats_by_kernel)
    return manifest, entries, summary


def filter_to_kernels(all_rows, matched_by_kernel, hw_by_kernel, selected_kernels):
    """Filter loaded CSV structures to a manifest-selected kernel set."""
    selected = set(selected_kernels)
    filtered_rows = [row for row in all_rows if row["kernel_name"] in selected]
    filtered_matched = {}
    filtered_hw = {}
    for kernel in selected_kernels:
        if kernel in matched_by_kernel or kernel in hw_by_kernel:
            filtered_matched[kernel] = list(matched_by_kernel.get(kernel, []))
            filtered_hw[kernel] = list(hw_by_kernel.get(kernel, []))
    return filtered_rows, filtered_matched, filtered_hw


def round_metric(value, digits=12):
    if value is None:
        return None
    try:
        numeric = float(value)
    except (TypeError, ValueError):
        return None
    if not math.isfinite(numeric):
        return None
    return round(numeric, digits)


def load_benchmark_tiers_config(path: Path = BENCHMARK_TIERS_PATH):
    """Load report-side benchmark tier metadata.

    Missing or malformed metadata is treated as absent so the default all-suite
    report remains reproducible from the comparison CSV alone.
    """
    try:
        with Path(path).open(encoding="utf-8") as fh:
            data = json.load(fh)
    except (FileNotFoundError, OSError, json.JSONDecodeError):
        return {}
    return data if isinstance(data, dict) else {}


def _load_metadata_section(data: dict, section: str):
    """Return one benchmark_tiers.json metadata section keyed by kernel."""
    regions = data.get(section, {}) or {}
    if not isinstance(regions, dict):
        return {}
    return {
        str(kernel): dict(metadata)
        for kernel, metadata in regions.items()
        if str(kernel) and isinstance(metadata, dict)
    }


def load_hardware_flat_region_metadata(path: Path = BENCHMARK_TIERS_PATH):
    """Return archived hardware-flat region metadata keyed by kernel name."""
    data = load_benchmark_tiers_config(path)
    return _load_metadata_section(data, "hardware_flat_regions")


def load_cache_bandwidth_work_sweep_metadata(path: Path = BENCHMARK_TIERS_PATH):
    """Return cache-bandwidth work-scaling metadata keyed by kernel name."""
    data = load_benchmark_tiers_config(path)
    return _load_metadata_section(data, "cache_bandwidth_work_sweeps")


def load_report_classification_metadata(path: Path = BENCHMARK_TIERS_PATH):
    """Return all report-side classification metadata keyed by kernel name.

    L1/L2 cache-bandwidth kernels intentionally have two independent metadata
    records: an archived selected-run fixed-work hardware-flat gate, and the
    current work-scaling/value-changing gate.  Keeping both records lets loaded
    rows choose the correct report semantics from provenance and row evidence.
    """
    combined = {}
    for kernel, metadata in load_hardware_flat_region_metadata(path).items():
        combined.setdefault(kernel, {})["hardware_flat_region"] = metadata
    for kernel, metadata in load_cache_bandwidth_work_sweep_metadata(path).items():
        combined.setdefault(kernel, {})["work_scaling_region"] = metadata
    return combined


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def parse_sort_key(s: str):
    """Numeric sort key for problem_size strings."""
    if re.fullmatch(r"\d+", s):
        return (0, int(s), s)
    m = re.fullmatch(r"(\d+)(?:x(\d+))?(?:x(\d+))?", s)
    if m:
        dims = [int(d) for d in m.groups() if d is not None]
        product = 1
        for d in dims:
            product *= d
        return (0, product, s)
    m = re.match(r"(\d+)", s)
    if m:
        return (0, int(m.group(1)), s)
    return (1, 0, s)


def spearman(x, y):
    """Spearman rank correlation. Returns None if n < 3."""
    n = len(x)
    if n < 3:
        return None

    def rank(v):
        idx = sorted(range(n), key=lambda i: v[i])
        r = [0.0] * n
        for i, j in enumerate(idx):
            r[j] = i + 1
        return r

    rx, ry = rank(x), rank(y)
    d2 = sum((rx[i] - ry[i]) ** 2 for i in range(n))
    return 1 - 6 * d2 / (n * (n * n - 1))


def wmape(points, use_adjusted=False):
    """Weighted Mean Absolute Percentage Error."""
    sim_key = "adjusted_sim" if use_adjusted else "sim"
    ae = sum(abs(p.get(sim_key, p["sim"]) - p["real"]) for p in points)
    r = sum(p["real"] for p in points)
    return ae / r if r > 0 else None


def avg_error(points, use_adjusted=False):
    """Mean of per-point |sim - hw| / hw."""
    sim_key = "adjusted_sim" if use_adjusted else "sim"
    if not points:
        return None
    errs = []
    for p in points:
        if p["real"] > 0:
            s = p.get(sim_key, p["sim"])
            errs.append(abs(s - p["real"]) / p["real"])
    return sum(errs) / len(errs) if errs else None


def parse_input_size(s: str) -> float:
    """Parse a problem_size string into a numeric input size.

    Handles formats like: '1024', '256x256', '128x128x128',
    'M1024', '1024KB', '128MB', etc.
    """
    s = s.strip()

    # Strip leading letter prefix (e.g., 'M1024' -> '1024')
    m = re.match(r"^[A-Za-z]+(\d.*)$", s)
    if m:
        s = m.group(1)

    # Handle KB/MB suffixes
    m = re.fullmatch(r"(\d+(?:\.\d+)?)\s*KB", s, re.IGNORECASE)
    if m:
        return float(m.group(1)) * 1024

    m = re.fullmatch(r"(\d+(?:\.\d+)?)\s*MB", s, re.IGNORECASE)
    if m:
        return float(m.group(1)) * 1024 * 1024

    # Handle NxNxN
    m = re.fullmatch(r"(\d+)(?:x(\d+))?(?:x(\d+))?", s)
    if m:
        dims = [int(d) for d in m.groups() if d is not None]
        product = 1
        for d in dims:
            product *= d
        return float(product)

    # Pure number
    m = re.match(r"(\d+(?:\.\d+)?)", s)
    if m:
        return float(m.group(1))

    return 0.0


def fit_booster(points):
    """Fit a constant booster c that minimises Σ|sim + c - hw|.

    Uses the median of (hw - sim) as the L1-optimal constant.
    Returns the booster value.
    """
    if not points:
        return 0.0
    residuals = sorted(p["real"] - p["sim"] for p in points)
    n = len(residuals)
    if n % 2 == 1:
        return residuals[n // 2]
    return (residuals[n // 2 - 1] + residuals[n // 2]) / 2


def apply_booster(points, booster):
    """Add adjusted_sim = sim + booster to each point."""
    for p in points:
        p["adjusted_sim"] = p["sim"] + booster


def linear_regression(x, y):
    """Simple OLS linear regression. Returns (slope, intercept)."""
    n = len(x)
    if n < 2:
        return None, None
    sx = sum(x)
    sy = sum(y)
    sxx = sum(xi * xi for xi in x)
    sxy = sum(xi * yi for xi, yi in zip(x, y))
    denom = n * sxx - sx * sx
    if abs(denom) < 1e-15:
        return None, None
    slope = (n * sxy - sx * sy) / denom
    intercept = (sy - slope * sx) / n
    return slope, intercept


def scaling_factor_error(points, use_adjusted=False):
    """Compute scaling factor (slope) error between HW and sim.

    Performs linear regression of time vs input_size for both HW and sim.
    Returns (hw_slope, sim_slope, error) or (None, None, None).
    """
    sim_key = "adjusted_sim" if use_adjusted else "sim"
    if len(points) < 3:
        return None, None, None

    sizes = [parse_input_size(p["size"]) for p in points]
    hw_times = [p["real"] for p in points]
    sim_times = [p.get(sim_key, p["sim"]) for p in points]

    hw_slope, _ = linear_regression(sizes, hw_times)
    sim_slope, _ = linear_regression(sizes, sim_times)

    if hw_slope is None or sim_slope is None or abs(hw_slope) < 1e-15:
        return hw_slope, sim_slope, None

    err = abs(sim_slope - hw_slope) / abs(hw_slope)
    return hw_slope, sim_slope, err


def fmt_pct(v):
    """Format a fraction as percentage string."""
    if v is None:
        return "—"
    return f"{v * 100:.1f}%"


def fmt_rho(v):
    """Format Spearman rho."""
    if v is None:
        return "—"
    return f"{v:+.4f}"


def fmt_booster(v):
    """Format booster value."""
    if v is None or v == 0.0:
        return "—"
    return f"{v:+.4f} ms"


def fmt_slope(v):
    """Format slope value."""
    if v is None:
        return "—"
    return f"{v:.4e}"


def fmt_bool(value):
    """Format a tri-state boolean for Markdown tables."""
    if value is True:
        return "yes"
    if value is False:
        return "no"
    return "unknown"


def report_repeat_count_parameter_names(meta):
    """Return repeat-count aliases from report/work metadata, when present."""
    names = meta.get("repeat_count_parameter_names")
    if not isinstance(names, dict):
        work_amount = meta.get("work_amount")
        if isinstance(work_amount, dict):
            names = work_amount.get("repeat_count_parameter_names")
    return names if isinstance(names, dict) else {}


def fmt_total_read_bytes(meta):
    """Format cache-bandwidth total-read work metadata for Markdown tables."""
    formula = meta.get("total_read_bytes_formula")
    default_bytes = meta.get("default_total_read_bytes")
    if not formula:
        return "—"

    repeat_names = report_repeat_count_parameter_names(meta)
    parts = [str(formula)]
    if default_bytes is not None:
        default_label = "HIP/CUDA default" if repeat_names.get("hip_cuda") else "default"
        parts.append(f"{default_label} {default_bytes} B")

    aliases = []
    if repeat_names.get("hip_cuda"):
        aliases.append(f"HIP/CUDA `{repeat_names['hip_cuda']}`")
    if repeat_names.get("go_simulator"):
        aliases.append(f"Go simulator `{repeat_names['go_simulator']}`")
    if aliases:
        parts.append("repeat count " + ", ".join(aliases))
    return "; ".join(parts)


def fmt_implementation_semantics(meta):
    """Format cache-bandwidth implementation footprint metadata."""
    implementation = meta.get("implementation_semantics")
    if not isinstance(implementation, dict):
        if meta.get("work_amount_scaled_by_axis") is False:
            return "archived fixed-work cache-residency footprint"
        return "—"

    parts = []
    hip = implementation.get("hip_cuda_workload")
    if isinstance(hip, dict):
        if hip.get("working_set_model") == "fixed_total_working_set_size":
            hip_text = "HIP/CUDA fixed working_set_size"
            if hip.get("working_set_size_bytes") is not None:
                hip_text += f" {hip['working_set_size_bytes']} B"
        else:
            hip_text = "HIP/CUDA " + str(hip.get("working_set_model") or "workload")
        if hip.get("repeat_parameter"):
            hip_text += f"; repeat {hip['repeat_parameter']}"
        parts.append(hip_text)

    simulator = implementation.get("go_simulator")
    if isinstance(simulator, dict):
        if simulator.get("working_set_model") == "per_256_thread_read_group_reuse":
            elems = simulator.get("read_group_elements")
            footprint = simulator.get("read_group_footprint_bytes")
            if elems and footprint:
                sim_text = f"Go simulator {elems}-thread read groups ({footprint} B each)"
            else:
                sim_text = "Go simulator per-read-group reuse"
        else:
            sim_text = "Go simulator " + str(
                simulator.get("working_set_model") or "workload"
            )
        if simulator.get("input_bytes_formula"):
            sim_text += f"; input bytes = {simulator['input_bytes_formula']}"
        if simulator.get("repeat_parameter"):
            sim_text += f"; repeat {simulator['repeat_parameter']}"
        parts.append(sim_text)

    return "; ".join(parts) if parts else "—"


def fmt_scored_pct(m, scored_key, diagnostic_key):
    """Format a scored percentage, showing diagnostics when excluded."""
    if m.get(scored_key) is not None:
        return fmt_pct(m[scored_key])
    diagnostic = m.get(diagnostic_key)
    if diagnostic is None:
        return "—"
    return f"not scored (diag {fmt_pct(diagnostic)})"


def fmt_ratio(value):
    """Format a multiplicative ratio for Markdown tables."""
    if value is None:
        return "—"
    return f"{value:.3f}×"


def loaded_row_summary_from_report_class(report_class):
    """Return the loaded-row summary attached to a metadata/value gate."""
    report_class = report_class or {}
    for gate_key in ("value_change_gate", "metadata_gate", "archived_metadata_gate"):
        gate = report_class.get(gate_key)
        if isinstance(gate, dict) and isinstance(gate.get("loaded_row_summary"), dict):
            return gate["loaded_row_summary"]
    return {}


def primary_evidence_gate(report_class):
    """Return the most relevant report gate name and payload for evidence text."""
    report_class = report_class or {}
    if isinstance(report_class.get("value_change_gate"), dict):
        return "value_change_gate", report_class["value_change_gate"]
    if isinstance(report_class.get("metadata_gate"), dict):
        return "metadata_gate", report_class["metadata_gate"]
    if isinstance(report_class.get("archived_metadata_gate"), dict):
        return "archived_metadata_gate", report_class["archived_metadata_gate"]
    return None, {}


def fmt_evidence_gate_status(report_class):
    """Format the active metadata/value-change gate status for Markdown."""
    gate_name, gate = primary_evidence_gate(report_class)
    if not gate_name or not gate.get("required"):
        return "—"

    reason = gate.get("reason") or "OK"
    if gate.get("passed"):
        if report_class.get("metric_policy") == "diagnostic_only_constant_work_flat_region":
            return "passed archived flat gate; fixed-work diagnostic only"
        return "passed value-change gate"
    return reason


def fmt_fixed_offset_implication(metric):
    """Describe whether the constant booster is scored or diagnostic only."""
    booster = fmt_booster(metric.get("booster"))
    scored = metric.get("avg_err_adj")
    diagnostic = metric.get("avg_err_adj_diagnostic")
    if scored is not None:
        return f"booster {booster}; scored avg err {fmt_pct(scored)}"
    if diagnostic is not None:
        return f"booster {booster}; diagnostic avg err {fmt_pct(diagnostic)}, not scored"
    return f"booster {booster}; no matched diagnostic"


def fmt_shape_diagnostics(metric):
    """Format trend/shape diagnostics, including unscored slopes."""
    return (
        f"ρ {fmt_rho(metric.get('rho'))}; "
        f"HW slope {fmt_slope(metric.get('hw_slope'))}; "
        f"sim slope {fmt_slope(metric.get('sim_slope'))}; "
        f"diag scaling err {fmt_pct(metric.get('sf_err_adj_diagnostic'))}"
    )


def cache_bandwidth_evidence_summaries(classified, kernel_metrics):
    """Return report rows for metadata-gated L1/L2 cache-bandwidth evidence."""
    rows = []
    for kernel in ("l1_cache_bw", "l2_cache_bw"):
        if kernel not in classified or kernel not in kernel_metrics:
            continue
        c = classified[kernel]
        metric = kernel_metrics[kernel]
        report_class = c.get("report_classification", {})
        if not report_class.get("has_metadata"):
            continue
        summary = loaded_row_summary_from_report_class(report_class)
        hw_rows = summary.get("hardware_rows", c["hw_flat_count"] + c["hw_nonflat_count"])
        sim_rows = summary.get("sim_matched_rows", len(c["all"]))
        ratio = summary.get("hardware_value_change_ratio")
        if ratio is None:
            hw_min = summary.get("hardware_min_ms")
            hw_max = summary.get("hardware_max_ms")
            if hw_min and hw_max:
                ratio = hw_max / hw_min
        rows.append({
            "kernel": kernel,
            "classification": report_class.get("display_label", "—"),
            "hardware_rows": hw_rows,
            "sim_completed_rows": sim_rows,
            "hardware_flat_rows": c["hw_flat_count"],
            "hardware_nonflat_rows": c["hw_nonflat_count"],
            "hardware_value_change_ratio": ratio,
            "raw_avg_error": metric.get("avg_err"),
            "raw_wmape": metric.get("wmape_all"),
            "fixed_offset_implication": fmt_fixed_offset_implication(metric),
            "shape_diagnostics": fmt_shape_diagnostics(metric),
            "evidence_status": fmt_evidence_gate_status(report_class),
        })
    return rows


def kernel_evidence_summary(c, metric):
    """Build a deterministic machine-readable evidence summary for one kernel."""
    report_class = c.get("report_classification", {})
    row_summary = loaded_row_summary_from_report_class(report_class)
    gate_name, gate = primary_evidence_gate(report_class)
    hw_rows = row_summary.get("hardware_rows", c["hw_flat_count"] + c["hw_nonflat_count"])
    sim_rows = row_summary.get("sim_matched_rows", len(c["all"]))

    return {
        "hardware_rows": hw_rows,
        "sim_completed_rows": sim_rows,
        "hardware_flat_rows": c["hw_flat_count"],
        "hardware_non_flat_rows": c["hw_nonflat_count"],
        "hardware_min_ms": round_metric(row_summary.get("hardware_min_ms")),
        "hardware_max_ms": round_metric(row_summary.get("hardware_max_ms")),
        "flat_threshold_ms": round_metric(row_summary.get("flat_threshold_ms")),
        "hardware_value_change_ratio": round_metric(
            row_summary.get("hardware_value_change_ratio")
            if row_summary.get("hardware_value_change_ratio") is not None
            else (
                row_summary.get("hardware_max_ms") / row_summary.get("hardware_min_ms")
                if row_summary.get("hardware_min_ms") and row_summary.get("hardware_max_ms")
                else None
            )
        ),
        "sim_value_change_ratio": round_metric(row_summary.get("sim_value_change_ratio")),
        "raw_avg_error": round_metric(metric.get("avg_err")),
        "raw_wmape": round_metric(metric.get("wmape_all")),
        "booster_ms": round_metric(metric.get("booster")),
        "avg_error_boosted_diagnostic": round_metric(metric.get("avg_err_adj_diagnostic")),
        "avg_error_boosted_scored": round_metric(metric.get("avg_err_adj")),
        "boosted_metrics_scored": bool(metric.get("boosted_metrics_scored", True)),
        "shape": {
            "spearman_rho": round_metric(metric.get("rho")),
            "hardware_slope": round_metric(metric.get("hw_slope")),
            "sim_slope": round_metric(metric.get("sim_slope")),
            "sim_slope_boosted": round_metric(metric.get("sim_slope_adj")),
            "scaling_factor_error_diagnostic": round_metric(
                metric.get("sf_err_adj_diagnostic")
            ),
            "scaling_factor_error_scored": round_metric(metric.get("sf_err_adj")),
            "scaling_informative": bool(metric.get("scaling_informative", True)),
        },
        "gate": {
            "name": gate_name,
            "required": bool(gate.get("required", False)) if gate else False,
            "passed": bool(gate.get("passed", False)) if gate else None,
            "reason": gate.get("reason") if gate else None,
        },
    }


def report_classification_label(c):
    """Return the concise report classification label for a classified kernel."""
    return c.get("report_classification", {}).get("display_label", "threshold flat/non-flat regions")


def report_classification_table_label(c):
    """Return a compact table label, suppressing default classifications."""
    classification = c.get("report_classification", {})
    if classification.get("has_metadata") or not classification.get("boosted_metrics_scored", True):
        return classification.get("display_label", "diagnostic only")
    return "—"


def point_region_label(point, classified_kernel):
    """Return the per-point region label used in detailed report rows."""
    is_flat = point["real"] <= classified_kernel["threshold"]
    if not is_flat:
        return "**non-flat**"
    classification = classified_kernel.get("report_classification", {})
    if classification.get("true_hardware_flat"):
        return classification.get("row_region_label", "true HW-flat")
    if classification.get("metric_policy") == "diagnostic_until_value_changing_rows":
        return "flat (value-change proof pending)"
    return "flat"


# ---------------------------------------------------------------------------
# Data loading and classification
# ---------------------------------------------------------------------------

def load_data(csv_path=CSV_PATH):
    """Load CSV, return (all_rows, matched_points_by_kernel, hw_points_by_kernel)."""
    all_rows = []
    matched_by_kernel = defaultdict(list)
    hw_by_kernel = defaultdict(list)

    with open(csv_path) as f:
        for row in csv.DictReader(f):
            all_rows.append(row)
            kernel = row["kernel_name"]
            real = row.get("real_ms", "").strip()
            sim = row.get("sim_ms", "").strip()

            if real:
                hw_by_kernel[kernel].append({
                    "kernel": kernel,
                    "size": row["problem_size"],
                    "real": float(real),
                })

            if real and sim:
                matched_by_kernel[kernel].append({
                    "kernel": kernel,
                    "size": row["problem_size"],
                    "real": float(real),
                    "sim": float(sim),
                })

    return all_rows, dict(matched_by_kernel), dict(hw_by_kernel)


def infer_selected_run_id(source) -> str | None:
    """Infer a selected-run ID from a provenance/data-source path when present."""
    if source is None:
        return None
    match = re.search(r"(?:^|[/\\])selected-run-(\d+)(?:[/\\]|$)", str(source))
    return match.group(1) if match else None


def build_classification_provenance(csv_path: Path | str | None):
    """Return report provenance used to gate report-only flat classifications."""
    if csv_path is None:
        return {}
    data_source = repo_relative(Path(csv_path))
    return {
        "data_source": data_source,
        "selected_run_id": infer_selected_run_id(data_source),
    }


def _has_metadata_value(metadata, key):
    value = metadata.get(key)
    return value is not None and value != ""


def _numeric_metadata_value(metadata, key):
    if not _has_metadata_value(metadata, key):
        return None
    try:
        return float(metadata[key])
    except (TypeError, ValueError):
        return None


def _int_metadata_value(metadata, key):
    if not _has_metadata_value(metadata, key):
        return None
    try:
        return int(metadata[key])
    except (TypeError, ValueError):
        return None


def _source_path_exists(source):
    if not source:
        return False
    source = str(source)
    if re.match(r"https?://", source):
        return True
    path = Path(source)
    if not path.is_absolute():
        path = REPO_ROOT / path
    return path.exists()


def _loaded_report_source(report_provenance) -> str:
    """Return the actual loaded report CSV/source path when provenance has it."""
    report_provenance = report_provenance or {}
    return str(
        report_provenance.get("data_source")
        or report_provenance.get("source")
        or ""
    )


def _selected_run_artifact_exists(run_id):
    if not run_id or not str(run_id).isdigit():
        return False
    selected_dir = REPO_ROOT / "benchmark-comparison" / f"selected-run-{run_id}"
    run_view = selected_dir / "run-view.json"
    comparison = selected_dir / "comparison_ci.detailed.csv"
    if not run_view.exists() or not comparison.exists():
        return False
    try:
        metadata = json.loads(run_view.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return False
    return str(metadata.get("databaseId")) == str(run_id)


def summarize_loaded_flat_region_rows(hw_pts, matched_pts):
    """Summarize loaded report rows for hardware-flat metadata validation."""
    real_times = [p["real"] for p in hw_pts]
    sim_times = [p["sim"] for p in matched_pts]
    problem_sizes = [parse_input_size(p["size"]) for p in hw_pts]
    summary = {
        "hardware_rows": len(hw_pts),
        "sim_matched_rows": len(matched_pts),
        "problem_size_min": min(problem_sizes) if problem_sizes else None,
        "problem_size_max": max(problem_sizes) if problem_sizes else None,
        "hardware_min_ms": min(real_times) if real_times else None,
        "hardware_max_ms": max(real_times) if real_times else None,
        "sim_min_ms": min(sim_times) if sim_times else None,
        "sim_max_ms": max(sim_times) if sim_times else None,
    }
    summary["flat_threshold_ms"] = (
        2 * summary["hardware_min_ms"]
        if summary["hardware_min_ms"] is not None else None
    )
    return summary


def _check_equal(checks, name, actual, expected):
    passed = actual == expected
    checks.append({
        "name": name,
        "passed": passed,
        "actual": actual,
        "expected": expected,
    })
    return passed


def _check_close(checks, name, actual, expected, *, abs_tol=1e-6):
    passed = (
        actual is not None
        and expected is not None
        and math.isclose(float(actual), float(expected), rel_tol=1e-9, abs_tol=abs_tol)
    )
    checks.append({
        "name": name,
        "passed": passed,
        "actual": actual,
        "expected": expected,
    })
    return passed


def evaluate_hardware_flat_metadata_gate(
    kernel,
    metadata,
    hw_pts,
    matched_pts,
    *,
    all_flat,
    report_provenance=None,
):
    """Validate report-only hardware-flat metadata against loaded rows/provenance."""
    metadata = metadata or {}
    classification = str(metadata.get("classification") or "")
    constant_work_axis = metadata.get("work_amount_scaled_by_axis") is False
    required = classification == "true_hardware_flat_region" and constant_work_axis
    gate = {
        "required": required,
        "passed": True,
        "checks": [],
        "reason": "not report-classified hardware-flat metadata",
    }
    if not required:
        return gate

    checks = gate["checks"]
    report_provenance = report_provenance or {}
    row_summary = summarize_loaded_flat_region_rows(hw_pts, matched_pts)

    def add_check(name, passed, actual=None, expected=None):
        checks.append({
            "name": name,
            "passed": bool(passed),
            "actual": actual,
            "expected": expected,
        })
        return bool(passed)

    add_check("loaded_hardware_rows_present", bool(hw_pts), len(hw_pts), ">0")
    add_check("loaded_hardware_rows_all_flat", bool(hw_pts) and all_flat, all_flat, True)

    metadata_run_id = str(metadata.get("selected_run_id") or "")
    current_run_id = str(
        report_provenance.get("selected_run_id")
        or infer_selected_run_id(report_provenance.get("data_source"))
        or ""
    )
    if metadata_run_id:
        if current_run_id:
            _check_equal(
                checks,
                "selected_run_id",
                current_run_id,
                metadata_run_id,
            )
        else:
            add_check(
                "selected_run_id",
                _selected_run_artifact_exists(metadata_run_id),
                metadata_run_id,
                "checked-in selected-run artifact",
            )
    else:
        add_check("selected_run_id", False, None, "metadata selected_run_id")

    metadata_source = str(metadata.get("source") or "")
    current_source = _loaded_report_source(report_provenance)
    if metadata_source:
        if current_source:
            _check_equal(checks, "source/data_source", current_source, metadata_source)
        else:
            add_check(
                "source",
                _source_path_exists(metadata_source),
                metadata_source,
                "existing source/provenance path",
            )
    else:
        add_check("source", False, None, "metadata source")

    for field in ("hardware_rows", "sim_matched_rows"):
        if _has_metadata_value(metadata, field):
            expected = _int_metadata_value(metadata, field)
            if expected is None:
                add_check(field, False, metadata.get(field), "integer metadata")
            else:
                _check_equal(checks, field, row_summary[field], expected)

    for field in ("problem_size_min", "problem_size_max"):
        if _has_metadata_value(metadata, field):
            expected = _numeric_metadata_value(metadata, field)
            if expected is None:
                add_check(field, False, metadata.get(field), "numeric metadata")
            else:
                _check_close(checks, field, row_summary[field], expected, abs_tol=1e-9)

    for field in (
        "hardware_min_ms",
        "hardware_max_ms",
        "flat_threshold_ms",
        "sim_min_ms",
        "sim_max_ms",
    ):
        if _has_metadata_value(metadata, field):
            expected = _numeric_metadata_value(metadata, field)
            if expected is None:
                add_check(field, False, metadata.get(field), "numeric metadata")
            else:
                _check_close(checks, field, row_summary[field], expected)

    failed = [check["name"] for check in checks if not check["passed"]]
    gate["passed"] = not failed
    gate["reason"] = "OK" if not failed else "metadata gate failed: " + ", ".join(failed)
    gate["loaded_row_summary"] = row_summary
    return gate


def _normalize_kernel_report_metadata(metadata):
    """Normalize legacy or combined per-kernel report metadata.

    Historical callers pass a single hardware-flat metadata dict.  Newer
    benchmark_tiers.json entries may carry separate archived hardware-flat and
    current work-scaling records for the same kernel.
    """
    if not isinstance(metadata, dict) or not metadata:
        return {}
    if any(
        key in metadata
        for key in (
            "hardware_flat_region",
            "archived_hardware_flat_region",
            "work_scaling_region",
            "cache_bandwidth_work_sweep",
        )
    ):
        normalized = {}
        flat = metadata.get("hardware_flat_region") or metadata.get("archived_hardware_flat_region")
        work = metadata.get("work_scaling_region") or metadata.get("cache_bandwidth_work_sweep")
        if isinstance(flat, dict):
            normalized["hardware_flat_region"] = dict(flat)
        if isinstance(work, dict):
            normalized["work_scaling_region"] = dict(work)
        return normalized

    classification = str(metadata.get("classification") or "")
    if (
        classification == "cache_bandwidth_work_sweep"
        or metadata.get("work_amount_scaled_by_axis") is True
        or metadata.get("work_amount_scaled") is True
    ):
        return {"work_scaling_region": dict(metadata)}
    return {"hardware_flat_region": dict(metadata)}


def _metadata_work_amount(metadata):
    work_amount = metadata.get("work_amount")
    return work_amount if isinstance(work_amount, dict) else {}


def _metadata_total_read_bytes_formula(metadata):
    work_amount = _metadata_work_amount(metadata)
    return (
        metadata.get("total_read_bytes_formula")
        or work_amount.get("total_read_bytes_formula")
        or work_amount.get("formula")
    )


def _proof_config(metadata):
    proof = metadata.get("value_change_proof")
    return proof if isinstance(proof, dict) else {}


def _proof_int(metadata, key, default):
    proof = _proof_config(metadata)
    value = proof.get(key, metadata.get(key, default))
    try:
        return int(value)
    except (TypeError, ValueError):
        return default


def _proof_float(metadata, key, default):
    proof = _proof_config(metadata)
    value = proof.get(key, metadata.get(key, default))
    try:
        parsed = float(value)
    except (TypeError, ValueError):
        return default
    return parsed if math.isfinite(parsed) else default


def summarize_loaded_work_scaling_rows(hw_pts, matched_pts):
    """Summarize loaded rows used to prove work-scaling/value-changing data."""
    summary = summarize_loaded_flat_region_rows(hw_pts, matched_pts)
    real_times = [p["real"] for p in hw_pts]
    sim_times = [p["sim"] for p in matched_pts]
    problem_sizes = [parse_input_size(p["size"]) for p in matched_pts]
    summary["matched_problem_size_count"] = len(set(problem_sizes))
    summary["hardware_value_change_ratio"] = (
        max(real_times) / min(real_times)
        if real_times and min(real_times) > 0 else None
    )
    summary["sim_value_change_ratio"] = (
        max(sim_times) / min(sim_times)
        if sim_times and min(sim_times) > 0 else None
    )
    if len(matched_pts) >= 3:
        sizes = [parse_input_size(p["size"]) for p in matched_pts]
        matched_hw_times = [p["real"] for p in matched_pts]
        hw_slope, _ = linear_regression(sizes, matched_hw_times)
        sim_slope, _ = linear_regression(sizes, sim_times)
    else:
        hw_slope = sim_slope = None
    summary["matched_hw_slope"] = hw_slope
    summary["matched_sim_slope"] = sim_slope
    return summary


def evaluate_work_scaling_metadata_gate(kernel, metadata, hw_pts, matched_pts):
    """Validate work-scaling metadata before scoring/trending loaded L1/L2 rows."""
    metadata = metadata or {}
    classification = str(metadata.get("classification") or "")
    work_amount_scaled = (
        metadata.get("work_amount_scaled_by_axis") is True
        or metadata.get("work_amount_scaled") is True
    )
    required = classification == "cache_bandwidth_work_sweep" and work_amount_scaled
    gate = {
        "required": required,
        "passed": True,
        "checks": [],
        "reason": "not report-classified work-scaling metadata",
    }
    if not required:
        return gate

    checks = gate["checks"]
    row_summary = summarize_loaded_work_scaling_rows(hw_pts, matched_pts)
    min_matched = _proof_int(metadata, "minimum_matched_rows", 3)
    min_problem_sizes = _proof_int(metadata, "minimum_distinct_problem_sizes", min_matched)
    value_ratio_threshold = _proof_float(
        metadata,
        "hardware_min_to_max_ratio_exclusive",
        _proof_float(metadata, "hardware_value_change_min_ratio", 2.0),
    )

    def add_check(name, passed, actual=None, expected=None):
        checks.append({
            "name": name,
            "passed": bool(passed),
            "actual": actual,
            "expected": expected,
        })
        return bool(passed)

    add_check(
        "work_amount_scaled_by_axis",
        work_amount_scaled,
        metadata.get("work_amount_scaled_by_axis"),
        True,
    )
    add_check(
        "total_read_bytes_defined",
        bool(_metadata_total_read_bytes_formula(metadata)),
        _metadata_total_read_bytes_formula(metadata),
        "total_read_bytes formula",
    )
    add_check("loaded_hardware_rows_present", bool(hw_pts), len(hw_pts), ">0")
    add_check(
        "loaded_matched_rows_min",
        len(matched_pts) >= min_matched,
        len(matched_pts),
        f">={min_matched}",
    )
    add_check(
        "loaded_distinct_problem_sizes_min",
        row_summary["matched_problem_size_count"] >= min_problem_sizes,
        row_summary["matched_problem_size_count"],
        f">={min_problem_sizes}",
    )
    ratio = row_summary["hardware_value_change_ratio"]
    add_check(
        "loaded_hardware_value_changing",
        ratio is not None and ratio > value_ratio_threshold,
        ratio,
        f">{value_ratio_threshold:g}x min hardware time",
    )

    failed = [check["name"] for check in checks if not check["passed"]]
    gate["passed"] = not failed
    gate["reason"] = "OK" if not failed else "value-change proof failed: " + ", ".join(failed)
    gate["loaded_row_summary"] = row_summary
    return gate


def build_report_classification(kernel, metadata, *, all_flat, no_flat):
    """Return report-side interpretation for a benchmark's flat/scaling rows."""
    metadata = metadata or {}
    has_metadata = bool(metadata)
    classification = str(metadata.get("classification") or "")
    scaling_axis_role = str(metadata.get("scaling_axis_role") or "")
    work_amount_scaled = metadata.get("work_amount_scaled_by_axis")
    if work_amount_scaled is None:
        work_amount_scaled = metadata.get("work_amount_scaled")
    constant_work_axis = work_amount_scaled is False
    true_hardware_flat = classification == "true_hardware_flat_region"
    work_scaling_sweep = (
        classification == "cache_bandwidth_work_sweep"
        and work_amount_scaled is True
    )
    value_change_gate = metadata.get("_value_change_gate") or {}
    value_change_proven = bool(
        work_scaling_sweep
        and value_change_gate.get("required")
        and value_change_gate.get("passed")
    )

    if true_hardware_flat and constant_work_axis:
        display_label = "true HW-flat / constant-work cache-residency sweep"
        row_region_label = "true HW-flat (constant-work)"
        metric_policy = "diagnostic_only_constant_work_flat_region"
        scaling_informative = False
    elif true_hardware_flat:
        display_label = "true hardware-flat region"
        row_region_label = "true HW-flat"
        metric_policy = "scored"
        scaling_informative = True
    elif work_scaling_sweep and value_change_proven:
        display_label = "cache bandwidth work-scaling sweep"
        row_region_label = "work-scaling"
        metric_policy = "scored"
        scaling_informative = True
    elif work_scaling_sweep:
        display_label = "cache bandwidth work-scaling sweep (pending value-change proof)"
        row_region_label = "value-change proof pending"
        metric_policy = "diagnostic_until_value_changing_rows"
        scaling_informative = False
    elif all_flat:
        display_label = "auto hardware-flat region"
        row_region_label = "flat"
        metric_policy = "scored"
        scaling_informative = True
    elif no_flat:
        display_label = "no distinct flat region"
        row_region_label = "non-flat"
        metric_policy = "scored"
        scaling_informative = True
    else:
        display_label = "threshold flat/non-flat regions"
        row_region_label = "flat"
        metric_policy = "scored"
        scaling_informative = True

    boosted_metrics_scored = metric_policy == "scored"
    work_amount = _metadata_work_amount(metadata)

    return {
        "kernel": kernel,
        "has_metadata": has_metadata,
        "classification": classification or (
            "auto_hardware_flat_region" if all_flat else "threshold_flat_nonflat"
        ),
        "display_label": display_label,
        "row_region_label": row_region_label,
        "metric_policy": metric_policy,
        "boosted_metrics_scored": boosted_metrics_scored,
        "scaling_informative": scaling_informative,
        "true_hardware_flat": true_hardware_flat,
        "constant_work_axis": constant_work_axis,
        "work_scaling_sweep": work_scaling_sweep,
        "value_change_proven": value_change_proven,
        "scaling_axis": metadata.get("scaling_axis"),
        "scaling_axis_role": scaling_axis_role or None,
        "work_amount_scaled_by_axis": work_amount_scaled,
        "work_amount_scaled": metadata.get("work_amount_scaled"),
        "work_amount": work_amount or None,
        "repeat_count_parameter_names": work_amount.get("repeat_count_parameter_names"),
        "total_read_bytes_formula": _metadata_total_read_bytes_formula(metadata),
        "default_total_read_bytes": work_amount.get("default_total_read_bytes"),
        "implementation_semantics": metadata.get("implementation_semantics"),
        "calibration_policy": metadata.get("calibration_policy"),
        "selected_run_id": metadata.get("selected_run_id"),
        "source": metadata.get("source"),
        "reason": metadata.get("reason"),
    }


def classify_points(
    hw_points,
    matched_points,
    flat_region_metadata=None,
    report_provenance=None,
):
    """Classify matched points into flat / non-flat regions.

    Optional report metadata annotates archived cache-residency sweeps and
    current cache-bandwidth work-scaling sweeps. The raw region split is
    preserved; metadata only affects report-side interpretation and scoring
    policy after provenance or value-changing row gates pass.

    Returns dict per kernel:
        {kernel: {flat: [...], nonflat: [...], min_hw: float,
                  threshold: float, all_flat: bool, no_flat: bool,
                  hw_flat_count: int, hw_nonflat_count: int,
                  report_classification: dict}}
    """
    result = {}
    flat_region_metadata = flat_region_metadata or {}
    for kernel in sorted(set(list(hw_points.keys()) + list(matched_points.keys()))):
        hw_pts = hw_points.get(kernel, [])
        m_pts = matched_points.get(kernel, [])

        if not hw_pts:
            continue

        min_hw = min(p["real"] for p in hw_pts)
        threshold = 2 * min_hw

        hw_flat = [p for p in hw_pts if p["real"] <= threshold]
        hw_nonflat = [p for p in hw_pts if p["real"] > threshold]

        flat = [p for p in m_pts if p["real"] <= threshold]
        nonflat = [p for p in m_pts if p["real"] > threshold]

        all_flat = len(hw_nonflat) == 0
        no_flat = len(hw_flat) <= 1 and not all_flat
        metadata = _normalize_kernel_report_metadata(flat_region_metadata.get(kernel))
        flat_metadata = metadata.get("hardware_flat_region")
        work_metadata = metadata.get("work_scaling_region")
        metadata_gate = evaluate_hardware_flat_metadata_gate(
            kernel,
            flat_metadata,
            hw_pts,
            m_pts,
            all_flat=all_flat,
            report_provenance=report_provenance,
        )

        active_gate_name = None
        report_classification = None
        if metadata_gate["required"] and metadata_gate["passed"]:
            report_classification = build_report_classification(
                kernel,
                flat_metadata,
                all_flat=all_flat,
                no_flat=no_flat,
            )
            active_gate_name = "metadata_gate"
        elif work_metadata:
            value_change_gate = evaluate_work_scaling_metadata_gate(
                kernel,
                work_metadata,
                hw_pts,
                m_pts,
            )
            work_classification_metadata = dict(work_metadata)
            work_classification_metadata["_value_change_gate"] = value_change_gate
            report_classification = build_report_classification(
                kernel,
                work_classification_metadata,
                all_flat=all_flat,
                no_flat=no_flat,
            )
            report_classification["value_change_gate"] = value_change_gate
            active_gate_name = "metadata_gate"
            if metadata_gate["required"]:
                report_classification["archived_metadata_gate"] = metadata_gate
        else:
            classification_metadata = flat_metadata
            if metadata_gate["required"] and not metadata_gate["passed"]:
                classification_metadata = None
            report_classification = build_report_classification(
                kernel,
                classification_metadata,
                all_flat=all_flat,
                no_flat=no_flat,
            )
            if metadata_gate["required"]:
                active_gate_name = "metadata_gate"

        if active_gate_name == "metadata_gate":
            if "value_change_gate" in report_classification:
                report_classification["metadata_gate"] = report_classification["value_change_gate"]
            elif metadata_gate["required"]:
                report_classification["metadata_gate"] = metadata_gate

        result[kernel] = {
            "flat": flat,
            "nonflat": nonflat,
            "all": m_pts,
            "min_hw": min_hw,
            "threshold": threshold,
            "all_flat": all_flat,
            "no_flat": no_flat,
            "hw_flat_count": len(hw_flat),
            "hw_nonflat_count": len(hw_nonflat),
            "report_classification": report_classification,
        }

    return result


def check_compliance(classified):
    """Check each kernel against minimum point requirements.

    Returns dict: {kernel: {flat_ok, nonflat_ok, compliant, reason}}
    """
    result = {}
    for kernel, c in classified.items():
        has_flat_region = c["hw_flat_count"] > 0 and not c["no_flat"]
        has_nonflat_region = c["hw_nonflat_count"] > 0

        flat_ok = (not has_flat_region) or (len(c["flat"]) >= MIN_FLAT_POINTS)
        nonflat_ok = (not has_nonflat_region) or (
            len(c["nonflat"]) >= MIN_NONFLAT_POINTS
        )

        compliant = flat_ok and nonflat_ok

        reasons = []
        if not flat_ok:
            reasons.append(
                f"flat: {len(c['flat'])}/{MIN_FLAT_POINTS}"
            )
        if not nonflat_ok:
            reasons.append(
                f"non-flat: {len(c['nonflat'])}/{MIN_NONFLAT_POINTS}"
            )

        result[kernel] = {
            "flat_ok": flat_ok,
            "nonflat_ok": nonflat_ok,
            "compliant": compliant,
            "reason": "; ".join(reasons) if reasons else "OK",
        }

    return result


# ---------------------------------------------------------------------------
# Accuracy computation
# ---------------------------------------------------------------------------

def compute_kernel_metrics(classified):
    """Compute per-kernel metrics. Returns dict of dicts.

    Fits per-benchmark booster and computes both raw and booster-adjusted
    metrics including average error and scaling factor error.
    """
    metrics = {}
    for kernel, c in classified.items():
        m = {}
        classification = c.get("report_classification", {})
        boosted_scored = classification.get("boosted_metrics_scored", True)
        scaling_informative = classification.get("scaling_informative", True)
        m["n_total"] = len(c["all"])
        m["n_flat"] = len(c["flat"])
        m["n_nonflat"] = len(c["nonflat"])
        m["boosted_metrics_scored"] = boosted_scored
        m["scaling_informative"] = scaling_informative

        # Fit booster (median of hw - sim). For report-classified constant-work
        # hardware-flat sweeps this value is retained only as a diagnostic.
        booster = fit_booster(c["all"])
        m["booster"] = booster
        apply_booster(c["all"], booster)
        apply_booster(c["flat"], booster)
        apply_booster(c["nonflat"], booster)

        # Raw WMAPE (without booster)
        m["wmape_all"] = wmape(c["all"]) if c["all"] else None
        m["wmape_flat"] = wmape(c["flat"]) if c["flat"] else None
        m["wmape_nonflat"] = wmape(c["nonflat"]) if c["nonflat"] else None

        # Boosted WMAPE. Keep diagnostic values even when they are excluded from
        # scored aggregate metrics.
        wmape_all_adj = wmape(c["all"], use_adjusted=True) if c["all"] else None
        wmape_nonflat_adj = wmape(c["nonflat"], use_adjusted=True) if c["nonflat"] else None
        m["wmape_all_adj_diagnostic"] = wmape_all_adj
        m["wmape_nonflat_adj_diagnostic"] = wmape_nonflat_adj
        m["wmape_all_adj"] = wmape_all_adj if boosted_scored else None
        m["wmape_nonflat_adj"] = wmape_nonflat_adj if boosted_scored else None

        # Average error (primary metric for pass/fail)
        avg_err_adj = avg_error(c["all"], use_adjusted=True) if c["all"] else None
        avg_err_nonflat_adj = avg_error(c["nonflat"], use_adjusted=True) if c["nonflat"] else None
        m["avg_err"] = avg_error(c["all"]) if c["all"] else None
        m["avg_err_adj_diagnostic"] = avg_err_adj
        m["avg_err_adj"] = avg_err_adj if boosted_scored else None
        m["avg_err_nonflat"] = avg_error(c["nonflat"]) if c["nonflat"] else None
        m["avg_err_nonflat_adj_diagnostic"] = avg_err_nonflat_adj
        m["avg_err_nonflat_adj"] = avg_err_nonflat_adj if boosted_scored else None

        # Scaling factor error (time vs input_size regression). Constant-work
        # cache-residency sweeps keep diagnostic slopes but are not target-scored.
        hw_slope, sim_slope, sf_err = scaling_factor_error(c["all"])
        _, sim_slope_adj, sf_err_adj = scaling_factor_error(
            c["all"], use_adjusted=True
        )
        m["hw_slope"] = hw_slope
        m["sim_slope"] = sim_slope
        m["sim_slope_adj"] = sim_slope_adj
        m["sf_err"] = sf_err
        m["sf_err_adj_diagnostic"] = sf_err_adj
        m["sf_err_adj"] = sf_err_adj if scaling_informative else None

        # Spearman rho
        if len(c["all"]) >= 3:
            pts = sorted(c["all"], key=lambda p: p["real"])
            m["rho"] = spearman(
                [p["real"] for p in pts], [p["sim"] for p in pts]
            )
        else:
            m["rho"] = None

        if len(c["nonflat"]) >= 3:
            pts = sorted(c["nonflat"], key=lambda p: p["real"])
            m["rho_nonflat"] = spearman(
                [p["real"] for p in pts], [p["sim"] for p in pts]
            )
        else:
            m["rho_nonflat"] = None

        # Sim/HW ratio stats
        if c["all"]:
            ratios = [
                p["sim"] / p["real"] for p in c["all"] if p["real"] > 0
            ]
            m["avg_ratio"] = sum(ratios) / len(ratios) if ratios else None
        else:
            m["avg_ratio"] = None

        metrics[kernel] = m

    return metrics


def compute_aggregate_metrics(classified, kernel_metrics, compliance_map=None, tier_lookup=None):
    """Compute aggregate metrics across all kernels."""
    all_pts = []
    flat_pts = []
    nonflat_pts = []
    for c in classified.values():
        all_pts.extend(c["all"])
        flat_pts.extend(c["flat"])
        nonflat_pts.extend(c["nonflat"])

    unscored_boosted = [
        k for k, m in kernel_metrics.items()
        if m.get("n_total", 0) > 0 and not m.get("boosted_metrics_scored", True)
    ]
    noninformative_scaling = [
        k for k, m in kernel_metrics.items()
        if m.get("n_total", 0) > 0 and not m.get("scaling_informative", True)
    ]

    agg = {
        "total_kernels": len(classified),
        "matched_kernels": sum(
            1 for c in classified.values() if len(c["all"]) > 0
        ),
        "total_points": len(all_pts),
        "flat_points": len(flat_pts),
        "nonflat_points": len(nonflat_pts),
        "wmape_all": wmape(all_pts),
        "wmape_flat": wmape(flat_pts) if flat_pts else None,
        "wmape_nonflat": wmape(nonflat_pts) if nonflat_pts else None,
        "wmape_nonflat_adj": wmape(nonflat_pts, use_adjusted=True) if nonflat_pts else None,
        "n_boosted_unscored": len(unscored_boosted),
        "boosted_unscored_kernels": sorted(unscored_boosted),
        "n_noninformative_scaling": len(noninformative_scaling),
        "noninformative_scaling_kernels": sorted(noninformative_scaling),
    }

    # Aggregate average error (with booster)
    adj_errs = [
        m["avg_err_adj"] for m in kernel_metrics.values()
        if m["avg_err_adj"] is not None
    ]
    agg["mean_avg_err_adj"] = (
        sum(adj_errs) / len(adj_errs) if adj_errs else None
    )
    agg["n_avg_err_benchmarks"] = len(adj_errs)

    # Aggregate scaling factor error (with booster)
    sf_errs = [
        m["sf_err_adj"] for m in kernel_metrics.values()
        if m["sf_err_adj"] is not None
    ]
    agg["mean_sf_err_adj"] = (
        sum(sf_errs) / len(sf_errs) if sf_errs else None
    )
    agg["n_sf_benchmarks"] = len(sf_errs)

    if len(all_pts) >= 3:
        pts = sorted(all_pts, key=lambda p: p["real"])
        agg["rho_all"] = spearman(
            [p["real"] for p in pts], [p["sim"] for p in pts]
        )
    else:
        agg["rho_all"] = None

    # ── Tier-separated metrics ──
    for tier_num, tier_target in [(1, TIER1_AVG_ERR_TARGET), (2, TIER2_AVG_ERR_TARGET)]:
        prefix = f"tier{tier_num}"
        tier_kernels = [
            k for k in kernel_metrics
            if lookup_tier(k, tier_lookup) == tier_num and kernel_metrics[k]["avg_err_adj"] is not None
        ]
        tier_errs = [kernel_metrics[k]["avg_err_adj"] for k in tier_kernels]
        tier_sf_errs = [
            kernel_metrics[k]["sf_err_adj"] for k in tier_kernels
            if kernel_metrics[k]["sf_err_adj"] is not None
        ]
        tier_compliant = sum(
            1 for k in classified
            if lookup_tier(k, tier_lookup) == tier_num and compliance_map.get(k, {}).get("compliant", False)
        ) if compliance_map else 0
        tier_total = sum(1 for k in classified if lookup_tier(k, tier_lookup) == tier_num)

        agg[f"{prefix}_count"] = tier_total
        agg[f"{prefix}_with_data"] = len(tier_kernels)
        agg[f"{prefix}_mean_avg_err_adj"] = (
            sum(tier_errs) / len(tier_errs) if tier_errs else None
        )
        agg[f"{prefix}_mean_sf_err_adj"] = (
            sum(tier_sf_errs) / len(tier_sf_errs) if tier_sf_errs else None
        )
        agg[f"{prefix}_target"] = tier_target
        agg[f"{prefix}_compliant"] = tier_compliant

    return agg


# ---------------------------------------------------------------------------
# Figure generation
# ---------------------------------------------------------------------------

def generate_figures(all_rows, classified, figures_dir=FIGURES_DIR, filename_prefix=""):
    """Generate per-kernel scaling figures. Returns list of (kernel, filename)."""
    plotting = load_plotting_modules()
    plt = plotting["plt"]
    figures_dir.mkdir(parents=True, exist_ok=True)

    # Build full HW + sim data from all_rows for plotting.
    hw_data = defaultdict(list)
    sim_data = defaultdict(list)
    for row in all_rows:
        kernel = row["kernel_name"]
        real = row.get("real_ms", "").strip()
        sim = row.get("sim_ms", "").strip()
        size = row["problem_size"]
        if real:
            hw_data[kernel].append((size, float(real)))
        if sim:
            sim_data[kernel].append((size, float(sim)))

    generated = []

    for kernel in sorted(classified.keys()):
        c = classified[kernel]
        if len(c["all"]) < 2:
            continue

        hw_pts = sorted(hw_data.get(kernel, []), key=lambda t: parse_sort_key(t[0]))
        sim_pts = sorted(sim_data.get(kernel, []), key=lambda t: parse_sort_key(t[0]))

        if not hw_pts or not sim_pts:
            continue

        fname = f"{filename_prefix}{kernel.lower()}_scaling.png"
        # Build unified x-axis
        all_sizes = list(dict.fromkeys(
            [t[0] for t in hw_pts] + [t[0] for t in sim_pts]
        ))
        all_sizes.sort(key=parse_sort_key)
        size_to_x = {s: i for i, s in enumerate(all_sizes)}

        fig, ax = plt.subplots(figsize=(9, 5.5))

        # Plot HW line
        hw_x = [size_to_x[t[0]] for t in hw_pts]
        hw_y = [t[1] for t in hw_pts]
        ax.plot(hw_x, hw_y, color="#2563eb", linestyle="-", marker="o",
                label="Hardware (MI300A)", markersize=5, linewidth=1.5)

        # Plot Sim line
        sim_x = [size_to_x[t[0]] for t in sim_pts]
        sim_y = [t[1] for t in sim_pts]
        ax.plot(sim_x, sim_y, color="#dc2626", linestyle="--", marker="s",
                label="Simulator (MGPUSim)", markersize=5, linewidth=1.5)

        # Shade flat region
        threshold = c["threshold"]
        if not c["all_flat"] and not c["no_flat"]:
            # Find the rightmost x where HW is still in flat region
            flat_x_max = -1
            for s, t in hw_pts:
                if t <= threshold:
                    flat_x_max = max(flat_x_max, size_to_x[s])
            if flat_x_max >= 0:
                ax.axvspan(-0.5, flat_x_max + 0.5, alpha=0.08, color="orange",
                           label="Flat region (overhead-dominated)")
        elif c.get("report_classification", {}).get("true_hardware_flat"):
            ax.axhspan(
                min(hw_y),
                max(hw_y),
                alpha=0.06,
                color="orange",
                label="True hardware-flat selected region",
            )

        if not c.get("report_classification", {}).get("scaling_informative", True):
            policy = c.get("report_classification", {}).get("metric_policy")
            note = "constant-work cache-residency sweep\nboosted/trend metrics diagnostic only"
            if policy == "diagnostic_until_value_changing_rows":
                note = "work-scaling metadata\nvalue-change proof pending"
            ax.text(
                0.02,
                0.98,
                note,
                transform=ax.transAxes,
                va="top",
                ha="left",
                fontsize=8,
                bbox={
                    "boxstyle": "round",
                    "facecolor": "white",
                    "alpha": 0.8,
                    "edgecolor": "#f59e0b",
                },
            )

        ax.set_xticks(range(len(all_sizes)))
        ax.set_xticklabels(all_sizes, rotation=45, ha="right", fontsize=7)
        ax.set_xlabel("Problem Size")
        ax.set_ylabel("Execution Time (ms)")
        ax.set_title(f"{kernel}", fontsize=13, fontweight="bold")
        ax.legend(fontsize=8)
        ax.grid(True, alpha=0.3)

        fig.tight_layout()
        save_figure_if_materially_changed(
            fig, figures_dir / fname, dpi=150, bbox_inches="tight"
        )
        plt.close(fig)
        generated.append((kernel, fname))

    return generated


# ---------------------------------------------------------------------------
# Slowdown figures & trend metric
# ---------------------------------------------------------------------------

def generate_slowdown_figures(classified, figures_dir=FIGURES_DIR, filename_prefix="", tier_lookup=None):
    """Generate slowdown bar chart and sim-vs-hw scatter plot.

    Returns dict with summary statistics for embedding in the report.
    """
    import statistics

    # Collect per-benchmark median slowdown and raw (real, sim) pairs.
    tier1_benchmarks_data = {}  # kernel -> list of sim/real ratios
    tier2_benchmarks_data = {}
    all_real = []
    all_sim = []
    all_tiers = []

    for kernel, c in classified.items():
        pts = c["all"]
        if not pts:
            continue
        ratios = []
        for point in pts:
            if point["real"] > 0:
                ratios.append(point["sim"] / point["real"])
                all_real.append(point["real"])
                all_sim.append(point["sim"])
                all_tiers.append(lookup_tier(kernel, tier_lookup))
        if ratios:
            tier = lookup_tier(kernel, tier_lookup)
            target = tier1_benchmarks_data if tier == 1 else tier2_benchmarks_data
            target[kernel] = ratios

    t1_names = sorted(tier1_benchmarks_data.keys())
    t2_names = sorted(tier2_benchmarks_data.keys())
    t1_medians = [statistics.median(tier1_benchmarks_data[k]) for k in t1_names]
    t2_medians = [statistics.median(tier2_benchmarks_data[k]) for k in t2_names]

    slowdown_bar_chart = f"{filename_prefix}slowdown_bar_chart.png"
    sim_vs_hw_scatter = f"{filename_prefix}sim_vs_hw_scatter.png"

    plotting = load_plotting_modules()
    plt = plotting["plt"]
    Patch = plotting["Patch"]
    figures_dir.mkdir(parents=True, exist_ok=True)

    # -- Bar chart: median slowdown per benchmark, grouped by tier --
    fig, ax = plt.subplots(figsize=(12, 6))

    labels = t1_names + t2_names
    medians = t1_medians + t2_medians
    colors = ["#2563eb"] * len(t1_names) + ["#ea580c"] * len(t2_names)

    if labels:
        x_pos = range(len(labels))
        ax.bar(x_pos, medians, color=colors, edgecolor="white", linewidth=0.5)
        ax.axhline(y=1.0, color="black", linestyle="--", linewidth=1,
                   label="Parity (1×)")
        ax.set_yscale("log")
        ax.set_xticks(list(x_pos))
        ax.set_xticklabels(labels, rotation=60, ha="right", fontsize=7)
        ax.set_ylabel("Median Slowdown (sim_time / hw_time)")
        ax.set_title("Simulation Slowdown Factor by Benchmark", fontsize=13,
                      fontweight="bold")
        # Custom legend entries
        legend_elements = [
            Patch(facecolor="#2563eb", label="Tier 1"),
            Patch(facecolor="#ea580c", label="Tier 2"),
        ]
        ax.legend(handles=legend_elements, fontsize=9)
        ax.grid(True, alpha=0.3, axis="y")

    fig.tight_layout()
    save_figure_if_materially_changed(
        fig, figures_dir / slowdown_bar_chart, dpi=150, bbox_inches="tight"
    )
    plt.close(fig)

    # -- Scatter plot: sim_ms vs real_ms, log-log --
    fig, ax = plt.subplots(figsize=(8, 8))

    t1_real = [r for r, t in zip(all_real, all_tiers) if t == 1]
    t1_sim = [s for s, t in zip(all_sim, all_tiers) if t == 1]
    t2_real = [r for r, t in zip(all_real, all_tiers) if t == 2]
    t2_sim = [s for s, t in zip(all_sim, all_tiers) if t == 2]

    if t1_real:
        ax.scatter(t1_real, t1_sim, c="#2563eb", alpha=0.6, s=30,
                   label="Tier 1", zorder=3)
    if t2_real:
        ax.scatter(t2_real, t2_sim, c="#ea580c", alpha=0.6, s=30,
                   label="Tier 2", zorder=3)

    if all_real:
        lo = min(min(all_real), min(all_sim)) * 0.5
        hi = max(max(all_real), max(all_sim)) * 2.0
        ax.plot([lo, hi], [lo, hi], "k--", linewidth=1, label="1:1 reference",
                zorder=2)
        ax.set_xlim(lo, hi)
        ax.set_ylim(lo, hi)

    ax.set_xscale("log")
    ax.set_yscale("log")
    ax.set_xlabel("Hardware Time (ms)")
    ax.set_ylabel("Simulator Time (ms)")
    ax.set_title("Simulator vs Hardware Execution Time", fontsize=13,
                  fontweight="bold")
    ax.legend(fontsize=9)
    ax.grid(True, alpha=0.3, which="both")
    ax.set_aspect("equal", adjustable="box")

    fig.tight_layout()
    save_figure_if_materially_changed(
        fig, figures_dir / sim_vs_hw_scatter, dpi=150, bbox_inches="tight"
    )
    plt.close(fig)

    # -- Summary statistics --
    all_medians_t1 = t1_medians if t1_medians else []
    all_medians_t2 = t2_medians if t2_medians else []
    all_medians = all_medians_t1 + all_medians_t2

    summary = {
        "t1_median_slowdown": statistics.median(all_medians_t1) if all_medians_t1 else None,
        "t1_mean_slowdown": (sum(all_medians_t1) / len(all_medians_t1)) if all_medians_t1 else None,
        "t2_median_slowdown": statistics.median(all_medians_t2) if all_medians_t2 else None,
        "t2_mean_slowdown": (sum(all_medians_t2) / len(all_medians_t2)) if all_medians_t2 else None,
        "all_median_slowdown": statistics.median(all_medians) if all_medians else None,
        "all_mean_slowdown": (sum(all_medians) / len(all_medians)) if all_medians else None,
        "slowdown_bar_chart": slowdown_bar_chart,
        "sim_vs_hw_scatter": sim_vs_hw_scatter,
    }
    return summary


def compute_trend_metric(classified, tier_lookup=None):
    """Compute trend-based accuracy metric for benchmarks with ≥3 data points.

    For each qualifying benchmark, computes:
        trend_ratio = sim_slope / hw_slope
    where slopes come from linear regression of time vs numeric input size.

    A ratio near 1.0 means the simulator correctly captures scaling behaviour.

    Returns list of dicts: [{kernel, tier, hw_slope, sim_slope, trend_ratio}]
    """
    results = []
    for kernel, c in sorted(classified.items()):
        if not c.get("report_classification", {}).get("scaling_informative", True):
            continue
        pts = c["all"]
        if len(pts) < 3:
            continue

        sizes = [parse_input_size(p["size"]) for p in pts]
        hw_times = [p["real"] for p in pts]
        sim_times = [p["sim"] for p in pts]

        hw_slope, _ = linear_regression(sizes, hw_times)
        sim_slope, _ = linear_regression(sizes, sim_times)

        if hw_slope is None or sim_slope is None:
            continue
        if abs(hw_slope) < 1e-15:
            continue

        trend_ratio = sim_slope / hw_slope
        results.append({
            "kernel": kernel,
            "tier": lookup_tier(kernel, tier_lookup),
            "hw_slope": hw_slope,
            "sim_slope": sim_slope,
            "trend_ratio": trend_ratio,
        })

    return results


# ---------------------------------------------------------------------------
# Report generation
# ---------------------------------------------------------------------------

def render_regular_workflow_contract(contract):
    """Render the maintained regular workflow contract for the Markdown report."""
    tier1 = contract["tier1_matrix_rows"]
    tier2 = contract["tier2_matrix_rows"]
    tier1_rows = contract["tier1_rows"]
    tier2_rows = contract["tier2_rows"]
    total_rows = contract["total_rows"]
    per_sim_sec = contract["per_simulator_timeout_sec"]
    job_sec = contract["benchmark_job_timeout_sec"]
    max_parallel = contract["max_parallel"]
    job_minutes = job_sec // 60
    return [
        "## Regular Workflow Contract",
        "",
        (
            "This report uses the comparison CSV for accuracy metrics; the "
            "maintained regular MI300A workflow contract remains the static "
            f"{tier1 + tier2}-benchmark matrix: {tier1}-benchmark Tier 1 / "
            f"{tier1_rows} plan entries plus {tier2}-benchmark Tier 2 / "
            f"{tier2_rows} plan entries ({total_rows} entries total)."
        ),
        "",
        (
            f"Workflow order is {contract['workflow_order']}. Runtime bounds are "
            f"{per_sim_sec}s per simulator invocation (`timeout_sec: {per_sim_sec}`), "
            f"{job_minutes}min/{job_sec}s per benchmark-tier job "
            f"(`timeout-minutes: {job_minutes}`), and `max-parallel: {max_parallel}`."
        ),
        "",
    ]


def render_report_provenance_contract(report_context, data_source):
    """Return Markdown that explains the all-scope CSV provenance contract."""
    if report_context.get("scope") != "all":
        return []

    top_level_source = repo_relative(CSV_PATH)
    selected_source = repo_relative(SELECTED_RUN_COMPARISON_PATH)
    lines = ["## Report Provenance", ""]

    if report_context.get("csv_explicit"):
        lines.extend([
            "This all-scope report was generated with an explicit `--csv` override. "
            f"The current source is `{data_source}`; the `Data source` header and "
            "`Raw data` reference below are the authoritative CSV provenance for "
            "this output.",
            "",
            "Use this mode when deliberately targeting current or top-level data such "
            f"as `{top_level_source}`. Do not infer selected-run provenance from an "
            "explicit-CSV all-scope report unless that explicit source is itself a "
            "selected-run artifact.",
            "",
        ])
        return lines

    lines.extend([
        "Running `python3 scripts/generate_validation_report.py` with no arguments "
        "uses the default `--scope all` contract and reads the committed "
        f"selected-run artifact `{selected_source}`.",
        "",
        "Use an explicit `--csv` override only when deliberately regenerating from "
        f"another comparison CSV, including current top-level data such as "
        f"`{top_level_source}`. The resulting report, summary, and figures must "
        "preserve that explicit CSV path as their provenance.",
        "",
    ])
    return lines


def generate_report(classified, compliance, kernel_metrics, agg, figures,
                    slowdown_summary=None, trend_results=None, report_context=None):
    """Generate the Markdown report for either all-suite or curated scope."""
    report_context = report_context or {}
    tier_lookup = report_context.get("tier_lookup")
    title = report_context.get("title", "MGPUSim MI300A Validation Report")
    data_source = report_context.get("data_source", "benchmark-comparison/comparison_ci.csv")
    raw_data_source = report_context.get("raw_data_source", data_source)
    figure_dir_link = report_context.get("figure_dir_link", "figures")
    line_break = "<br>"

    def fig_link(filename):
        if not figure_dir_link:
            return filename
        return f"{figure_dir_link}/{filename}"

    lines = []

    # ── Header ──
    lines.append(f"# {title}")
    lines.append("")
    lines.append(f"**Hardware reference:** AMD Instinct MI300A{line_break}")
    lines.append(f"**Data source:** `{data_source}`{line_break}")
    lines.append(
        f"**Policy:** [Validation Point Selection](validation_point_selection.md){line_break}"
    )
    if report_context.get("scope_label"):
        lines.append(f"**Scope:** {report_context['scope_label']}{line_break}")
    if report_context.get("set_id"):
        lines.append(
            f"**Curated set:** `{report_context['set_id']}` "
            f"version `{report_context.get('set_version', 'unknown')}`{line_break}"
        )
    lines.append(
        f"**Matched:** {agg['matched_kernels']} kernels, "
        f"{agg['total_points']} data points "
        f"({agg['flat_points']} flat, {agg['nonflat_points']} non-flat)"
    )
    lines.append("")
    lines.append(
        "*This report is auto-generated by "
        "`scripts/generate_validation_report.py`. Do not edit manually.*"
    )
    lines.append("")

    lines.extend(render_report_provenance_contract(report_context, data_source))

    regular_contract = report_context.get("regular_workflow_contract")
    if regular_contract:
        lines.extend(render_regular_workflow_contract(regular_contract))

    if report_context.get("scope") == "curated":
        lines.append("## Curated Primary Evaluation Set")
        lines.append("")
        lines.append(
            f"Selected benchmarks: **{report_context['selected_count']}** "
            f"(Tier 1: **{report_context['tier_counts'].get('1', 0)}**, "
            f"Tier 2: **{report_context['tier_counts'].get('2', 0)}**)."
        )
        coverage = report_context.get("point_coverage_status", {})
        totals = coverage.get("totals", {})
        if coverage:
            full_count = coverage.get("selected_with_passing_coverage", 0)
            total_count = coverage.get("selected_total", report_context["selected_count"])
            partial_count = coverage.get("selected_with_partial_coverage", 0)
            lines.append(
                f"Coverage status: **{coverage.get('status', 'unknown')}** "
                f"with {full_count}/{total_count} manifest-full coverage benchmark(s), "
                f"{partial_count} partial current-data benchmark(s), "
                f"{totals.get('matched_points', 0)}/{totals.get('hardware_points', 0)} "
                f"matched points, {totals.get('matched_flat_points', 0)}/"
                f"{totals.get('hardware_flat_points', 0)} flat, "
                f"{totals.get('matched_non_flat_points', 0)}/"
                f"{totals.get('hardware_non_flat_points', 0)} non-flat, "
                f"and {totals.get('trend_points', 0)} trend points."
            )
            if coverage.get("partial_kernels"):
                partial = ", ".join(f"`{kernel}`" for kernel in coverage["partial_kernels"])
                lines.append(
                    "Partial entries are retained for historical/category continuity "
                    f"but are not counted as full point-coverage passes: {partial}."
                )
        lines.append("")
        lines.append("### Category Counts")
        lines.append("")
        lines.append("| Category | Benchmarks |")
        lines.append("|----------|-----------:|")
        for category, count in sorted(report_context.get("category_counts", {}).items()):
            lines.append(f"| {category} | {count} |")
        lines.append("")
        lines.append(
            f"> **Broad-suite diagnostics policy:** "
            f"{report_context.get('diagnostic_policy', CURATED_DIAGNOSTIC_POLICY)}"
        )
        lines.append("")

    # ── 1. Aggregate Accuracy ──
    lines.append("---")
    lines.append("")
    lines.append("## 1. Aggregate Accuracy")
    lines.append("")
    lines.append("### Primary Metrics (scored per-benchmark booster)")
    lines.append("")
    lines.append("| Metric | Value | Target |")
    lines.append("|--------|------:|-------:|")
    lines.append(
        f"| **Mean Average Error (scored booster)** "
        f"| {fmt_pct(agg['mean_avg_err_adj'])} "
        f"({agg['n_avg_err_benchmarks']} benchmarks) | ≤ 20% |"
    )
    lines.append(
        f"| **Mean Scaling Factor Error** "
        f"| {fmt_pct(agg['mean_sf_err_adj'])} "
        f"({agg['n_sf_benchmarks']} benchmarks) | ≤ 20% |"
    )
    lines.append("")
    lines.append(
        "> **Average error** measures per-point accuracy after removing "
        "systematic constant offset (booster) only for benchmarks whose "
        "scaling axis is informative. **Scaling factor error** measures "
        "whether the simulator correctly predicts how performance scales "
        "with input size — this is what academic papers rely on."
    )
    if agg.get("n_boosted_unscored"):
        excluded = ", ".join(agg.get("boosted_unscored_kernels", []))
        lines.append(
            f"> **Excluded from boosted targets:** {agg['n_boosted_unscored']} "
            f"metadata-gated benchmark(s): {excluded}. These are either archived "
            "constant-work cache-residency sweeps or work-scaling rows still "
            "pending loaded value-change proof; raw and diagnostic boosted values "
            "remain in the detailed tables."
        )
    lines.append("")
    lines.append("### Tier-Separated Metrics")
    lines.append("")
    lines.append("| Tier | Benchmarks | Avg Error (scored booster) | Target | Scaling Err | Compliant |")
    lines.append("|------|-----------|--------------------:|-------:|------------:|----------:|")
    for tier_num, tier_label in [(1, "Tier 1 (microbenchmarks)"), (2, "Tier 2 (algorithm kernels)")]:
        prefix = f"tier{tier_num}"
        lines.append(
            f"| {tier_label} "
            f"| {agg[f'{prefix}_with_data']}/{agg[f'{prefix}_count']} "
            f"| {fmt_pct(agg[f'{prefix}_mean_avg_err_adj'])} "
            f"| ≤ {agg[f'{prefix}_target'] * 100:.0f}% "
            f"| {fmt_pct(agg[f'{prefix}_mean_sf_err_adj'])} "
            f"| {agg[f'{prefix}_compliant']}/{agg[f'{prefix}_count']} |"
        )
    lines.append("")
    lines.append("### Supporting Metrics")
    lines.append("")
    lines.append("| Region | Points | WMAPE (raw) | WMAPE (with booster) |")
    lines.append("|--------|-------:|------------:|---------------------:|")
    lines.append(
        f"| Non-flat | {agg['nonflat_points']} "
        f"| {fmt_pct(agg['wmape_nonflat'])} "
        f"| {fmt_pct(agg['wmape_nonflat_adj'])} |"
    )
    lines.append(
        f"| Flat | {agg['flat_points']} "
        f"| {fmt_pct(agg['wmape_flat'])} | — |"
    )
    lines.append(
        f"| All combined | {agg['total_points']} "
        f"| {fmt_pct(agg['wmape_all'])} | — |"
    )
    lines.append(
        f"| Spearman ρ (all) | — | {fmt_rho(agg['rho_all'])} | — |"
    )
    lines.append("")

    classified_flat_regions = [
        (kernel, classified[kernel])
        for kernel in sorted(classified)
        if classified[kernel].get("report_classification", {}).get("has_metadata")
    ]
    if classified_flat_regions:
        lines.append("### Hardware-Flat / Work-Scaling Sweep Classification")
        lines.append("")
        lines.append(
            "These report-side annotations preserve the raw CSV rows while "
            "separating archived fixed-work cache-residency plateaus from "
            "current cache-bandwidth work-scaling evidence. Archived selected-run "
            "rows become diagnostic-only only when their provenance and row-summary "
            "gate passes. Work-scaled rows are scored/trended only after loaded "
            "hardware rows prove non-flat/value-changing behavior. For current "
            "L1 rows, HIP/CUDA keeps a fixed `working_set_size`, while the Go "
            "simulator keeps each 256-thread read group L1-resident and lets "
            "total input allocation grow with `num_elements`."
        )
        lines.append("")
        lines.append(
            "| Benchmark | Report class | Axis | Axis role | Work scaled? "
            "| Total read bytes | Implementation footprint | HW flat/non-flat "
            "| Matched pts | Metric policy | Source |"
        )
        lines.append(
            "|-----------|--------------|------|-----------|-------------:"
            "|------------------|--------------------------|----------------:"
            "|------------:|---------------|--------|"
        )
        for kernel, c in classified_flat_regions:
            meta = c["report_classification"]
            source = meta.get("source") or "metadata"
            if meta.get("selected_run_id"):
                source = f"{source}; run {meta['selected_run_id']}"
            policy = "scored"
            if meta.get("metric_policy") == "diagnostic_until_value_changing_rows":
                policy = "diagnostic until loaded rows prove value-changing behavior"
            elif not meta.get("boosted_metrics_scored", True):
                policy = "diagnostic only; excluded from boosted/trend targets"
            lines.append(
                f"| {kernel} "
                f"| {meta['display_label']} "
                f"| {meta.get('scaling_axis') or 'unknown'} "
                f"| {meta.get('scaling_axis_role') or 'unknown'} "
                f"| {fmt_bool(meta.get('work_amount_scaled_by_axis'))} "
                f"| {fmt_total_read_bytes(meta)} "
                f"| {fmt_implementation_semantics(meta)} "
                f"| {c['hw_flat_count']}/{c['hw_nonflat_count']} "
                f"| {len(c['all'])} "
                f"| {policy} "
                f"| {source} |"
            )
        lines.append("")

    cache_evidence_rows = cache_bandwidth_evidence_summaries(classified, kernel_metrics)
    if cache_evidence_rows:
        lines.append("### Cache-Bandwidth Evidence Snapshot")
        lines.append("")
        lines.append(
            "This snapshot keeps L1/L2 cache-bandwidth evidence explicit: "
            "row counts and raw errors are always from loaded CSV rows; "
            "constant-offset boosters are diagnostic unless the metadata gate marks "
            "the row scored; trend/shape metrics are shown even when excluded from "
            "aggregate targets."
        )
        lines.append("")
        lines.append(
            "| Benchmark | Report class | HW rows | Completed sim rows "
            "| HW flat/non-flat | HW max/min | Raw avg/W | Fixed-offset implication "
            "| Trend/shape diagnostics | Evidence status |"
        )
        lines.append(
            "|-----------|--------------|--------:|-------------------:"
            "|----------------:|-----------:|-----------:|--------------------------"
            "|-------------------------|-----------------|"
        )
        for row in cache_evidence_rows:
            lines.append(
                f"| {row['kernel']} "
                f"| {row['classification']} "
                f"| {row['hardware_rows']} "
                f"| {row['sim_completed_rows']} "
                f"| {row['hardware_flat_rows']}/{row['hardware_nonflat_rows']} "
                f"| {fmt_ratio(row['hardware_value_change_ratio'])} "
                f"| {fmt_pct(row['raw_avg_error'])}/{fmt_pct(row['raw_wmape'])} "
                f"| {row['fixed_offset_implication']} "
                f"| {row['shape_diagnostics']} "
                f"| {row['evidence_status']} |"
            )
        lines.append("")

    # ── Performance Comparison ──
    lines.append("---")
    lines.append("")
    lines.append("## Performance Comparison")
    lines.append("")
    slowdown_bar = "slowdown_bar_chart.png"
    sim_vs_hw = "sim_vs_hw_scatter.png"
    if slowdown_summary:
        slowdown_bar = slowdown_summary.get("slowdown_bar_chart", slowdown_bar)
        sim_vs_hw = slowdown_summary.get("sim_vs_hw_scatter", sim_vs_hw)
    lines.append(f"![Slowdown Bar Chart]({fig_link(slowdown_bar)})")
    lines.append("")
    lines.append(f"![Sim vs HW Scatter]({fig_link(sim_vs_hw)})")
    lines.append("")
    if slowdown_summary:
        lines.append("### Slowdown Summary Statistics")
        lines.append("")
        lines.append("| Tier | Median Slowdown | Mean Slowdown |")
        lines.append("|------|----------------:|--------------:|")

        def _fmt_sd(v):
            return f"{v:.3f}×" if v is not None else "—"

        lines.append(
            f"| Tier 1 (microbenchmarks) "
            f"| {_fmt_sd(slowdown_summary['t1_median_slowdown'])} "
            f"| {_fmt_sd(slowdown_summary['t1_mean_slowdown'])} |"
        )
        lines.append(
            f"| Tier 2 (algorithm kernels) "
            f"| {_fmt_sd(slowdown_summary['t2_median_slowdown'])} "
            f"| {_fmt_sd(slowdown_summary['t2_mean_slowdown'])} |"
        )
        lines.append(
            f"| **All** "
            f"| {_fmt_sd(slowdown_summary['all_median_slowdown'])} "
            f"| {_fmt_sd(slowdown_summary['all_mean_slowdown'])} |"
        )
        lines.append("")
    lines.append("")

    # ── Trend Accuracy ──
    lines.append("---")
    lines.append("")
    lines.append("## Trend Accuracy")
    lines.append("")
    lines.append(
        "Trend ratio = `sim_slope / hw_slope` for benchmarks with ≥ 3 data "
        "points and an informative work-scaling axis. A value near 1.0 "
        "means the simulator correctly captures how performance scales with "
        "input size. Report-classified constant-work cache-residency sweeps "
        "and work-scaling rows pending loaded value-change proof are omitted "
        "from trend scoring."
    )
    lines.append("")
    if trend_results:
        lines.append(
            "| Benchmark | Tier | HW Slope | Sim Slope | Trend Ratio |"
        )
        lines.append(
            "|-----------|-----:|---------:|----------:|------------:|"
        )
        for tr in trend_results:
            lines.append(
                f"| {tr['kernel']} "
                f"| {tr['tier']} "
                f"| {fmt_slope(tr['hw_slope'])} "
                f"| {fmt_slope(tr['sim_slope'])} "
                f"| {tr['trend_ratio']:.4f} |"
            )
        lines.append("")

        # Mean trend ratio per tier
        t1_ratios = [t["trend_ratio"] for t in trend_results if t["tier"] == 1]
        t2_ratios = [t["trend_ratio"] for t in trend_results if t["tier"] == 2]
        lines.append("### Mean Trend Ratio by Tier")
        lines.append("")
        lines.append("| Tier | Benchmarks | Mean Trend Ratio |")
        lines.append("|------|----------:|-----------------:|")
        if t1_ratios:
            lines.append(
                f"| Tier 1 | {len(t1_ratios)} "
                f"| {sum(t1_ratios) / len(t1_ratios):.4f} |"
            )
        else:
            lines.append("| Tier 1 | 0 | — |")
        if t2_ratios:
            lines.append(
                f"| Tier 2 | {len(t2_ratios)} "
                f"| {sum(t2_ratios) / len(t2_ratios):.4f} |"
            )
        else:
            lines.append("| Tier 2 | 0 | — |")
        lines.append("")
    else:
        lines.append("*No benchmarks with ≥ 3 data points available.*")
        lines.append("")

    # ── 2. Compliance Summary ──
    lines.append("---")
    lines.append("")
    lines.append("## 2. Point Coverage Compliance")
    lines.append("")
    n_compliant = sum(1 for v in compliance.values() if v["compliant"])
    n_noncompliant = sum(1 for v in compliance.values() if not v["compliant"])
    n_nosim = sum(
        1 for k, c in classified.items() if len(c["all"]) == 0
    )
    lines.append(
        f"**{n_compliant}** compliant, "
        f"**{n_noncompliant}** non-compliant, "
        f"**{n_nosim}** with no sim data."
    )
    lines.append("")
    lines.append(
        "Policy requires ≥ 1 flat point (if flat region exists) and "
        "≥ 5 non-flat points (if non-flat region exists)."
    )
    if report_context.get("scope") == "curated":
        lines.append(
            "For curated scope, the manifest-full/partial status above follows "
            "the per-entry manifest expectations; this table remains the broad "
            "flat/non-flat report-compliance view for the loaded rows."
        )
    lines.append("")

    if n_noncompliant > 0:
        lines.append("### Non-Compliant Benchmarks")
        lines.append("")
        lines.append(
            "| Benchmark | Flat Pts | Non-flat Pts | HW Flat | HW Non-flat | Gap |"
        )
        lines.append("|-----------|----------|-------------|---------|-------------|-----|")
        for k in sorted(compliance.keys()):
            v = compliance[k]
            if not v["compliant"]:
                c = classified[k]
                lines.append(
                    f"| {k} | {len(c['flat'])} | {len(c['nonflat'])} "
                    f"| {c['hw_flat_count']} | {c['hw_nonflat_count']} "
                    f"| {v['reason']} |"
                )
        lines.append("")

    # ── 3. Per-Benchmark Accuracy ──
    lines.append("---")
    lines.append("")
    lines.append("## 3. Per-Benchmark Accuracy")
    lines.append("")
    lines.append(
        "Scored rows are sorted by average error with booster (primary metric). "
        "Non-scored diagnostic rows and benchmarks without data are sorted to the end."
    )
    lines.append("")

    # Separate kernels with data from those without
    has_data = [
        k for k in sorted(classified.keys())
        if len(classified[k]["all"]) > 0
    ]

    # Sort by adjusted average error
    def sort_key(k):
        m = kernel_metrics[k]
        if m["avg_err_adj"] is not None:
            return (0, m["avg_err_adj"])
        if m["wmape_all"] is not None:
            return (1, m["wmape_all"])
        return (2, 0)

    has_data.sort(key=sort_key)

    lines.append(
        "| Benchmark | Tier | Pts | Scaling class | Avg Err | Avg Err (boosted) "
        "| Scaling Err | Booster | ρ | Compliant |"
    )
    lines.append(
        "|-----------|-----:|----:|---------------|--------:|------------------:"
        "|------------:|--------:|--:|-----------|"
    )

    for k in has_data:
        m = kernel_metrics[k]
        c = compliance.get(k, {})
        classified_kernel = classified[k]
        comp_str = "✅" if c.get("compliant", False) else "⚠️"
        tier = lookup_tier(k, tier_lookup)
        lines.append(
            f"| {k} "
            f"| {tier} "
            f"| {m['n_total']} "
            f"| {report_classification_table_label(classified_kernel)} "
            f"| {fmt_pct(m['avg_err'])} "
            f"| {fmt_scored_pct(m, 'avg_err_adj', 'avg_err_adj_diagnostic')} "
            f"| {fmt_scored_pct(m, 'sf_err_adj', 'sf_err_adj_diagnostic')} "
            f"| {fmt_booster(m['booster'])} "
            f"| {fmt_rho(m['rho'])} "
            f"| {comp_str} |"
        )

    lines.append("")

    # ── 4. Per-Benchmark Data Tables ──
    lines.append("---")
    lines.append("")
    lines.append("## 4. Per-Benchmark Data Tables & Figures")
    lines.append("")
    lines.append(
        "Each section shows the raw data points, region classification, "
        "and a scaling figure. Orange-shaded area in figures indicates "
        "the flat (overhead-dominated) region."
    )
    lines.append("")

    fig_map = {k.lower(): f for k, f in figures}

    for k in sorted(classified.keys()):
        c = classified[k]
        m = kernel_metrics[k]
        comp = compliance.get(k, {})

        if len(c["all"]) == 0:
            continue

        lines.append(f"### {k}")
        lines.append("")

        # Summary line
        comp_str = "✅ Compliant" if comp.get("compliant") else f"⚠️ Non-compliant ({comp.get('reason', '')})"
        report_class = c.get("report_classification", {})
        avg_boosted_text = fmt_scored_pct(m, "avg_err_adj", "avg_err_adj_diagnostic")
        sf_text = fmt_scored_pct(m, "sf_err_adj", "sf_err_adj_diagnostic")
        lines.append(
            f"**Points:** {m['n_total']} total "
            f"({m['n_flat']} flat, {m['n_nonflat']} non-flat){line_break}"
        )
        if report_class.get("has_metadata") or not report_class.get("boosted_metrics_scored", True):
            lines.append(f"**Report classification:** {report_classification_label(c)}{line_break}")
        if report_class.get("work_scaling_sweep"):
            axis = report_class.get("scaling_axis") or "the configured axis"
            lines.append(
                f"**Work axis:** `{axis}` scales total reads: "
                f"{fmt_total_read_bytes(report_class)}.{line_break}"
            )
            footprint = fmt_implementation_semantics(report_class)
            if footprint != "—":
                lines.append(f"**Implementation footprint:** {footprint}{line_break}")
        if not report_class.get("boosted_metrics_scored", True):
            axis = report_class.get("scaling_axis") or "the configured axis"
            role = report_class.get("scaling_axis_role") or "cache-residency sweep"
            if report_class.get("metric_policy") == "diagnostic_until_value_changing_rows":
                lines.append(
                    f"**Metric policy:** booster and trend/scaling are diagnostic only; "
                    f"excluded from aggregate targets until loaded `{axis}` rows "
                    f"prove non-flat/value-changing work-scaling behavior.{line_break}"
                )
            else:
                lines.append(
                    f"**Metric policy:** booster and trend/scaling are diagnostic only; "
                    f"excluded from aggregate targets because `{axis}` is a {role} "
                    f"with fixed work amount.{line_break}"
                )
        lines.append(
            f"**Avg Error:** {fmt_pct(m['avg_err'])} (raw), "
            f"{avg_boosted_text} (with booster){line_break}"
        )
        lines.append(
            f"**Scaling Factor Error:** {sf_text} · "
            f"**Booster:** {fmt_booster(m['booster'])}{line_break}"
        )
        lines.append(
            f"**HW Slope:** {fmt_slope(m['hw_slope'])} · "
            f"**Sim Slope:** {fmt_slope(m['sim_slope_adj'])}{line_break}"
        )
        lines.append(f"**ρ:** {fmt_rho(m['rho'])} · **Status:** {comp_str}")
        lines.append("")

        # Data table
        pts_sorted = sorted(c["all"], key=lambda p: parse_sort_key(p["size"]))
        lines.append(
            "| Size | HW (ms) | Sim (ms) | Sim+Booster (ms) "
            "| Error (raw) | Error (boosted) | Region |"
        )
        lines.append(
            "|------|--------:|---------:|------------------:"
            "|------------:|----------------:|--------|"
        )
        for p in pts_sorted:
            err = (p["sim"] - p["real"]) / p["real"] if p["real"] > 0 else 0
            adj = p.get("adjusted_sim", p["sim"])
            err_adj = (adj - p["real"]) / p["real"] if p["real"] > 0 else 0
            region = point_region_label(p, c)
            lines.append(
                f"| {p['size']} "
                f"| {p['real']:.4f} "
                f"| {p['sim']:.4f} "
                f"| {adj:.4f} "
                f"| {err:+.1%} "
                f"| {err_adj:+.1%} "
                f"| {region} |"
            )
        lines.append("")

        # Figure
        fname = fig_map.get(k.lower())
        if fname:
            lines.append(f"![{k} scaling]({fig_link(fname)})")
            lines.append("")

    # ── 5. Methodology ──
    lines.append("---")
    lines.append("")
    lines.append("## 5. Methodology")
    lines.append("")
    lines.append("### Region Classification")
    lines.append("")
    lines.append(
        "Each data point is classified based on hardware execution time:"
    )
    lines.append("")
    lines.append(
        "- **Flat region:** `hw_time ≤ 2 × min(hw_time)` for that kernel. "
        "Kernel launch overhead or a hardware plateau dominates."
    )
    lines.append(
        "- **Non-flat region:** `hw_time > 2 × min(hw_time)`. "
        "Compute/memory behaviour dominates."
    )
    lines.append(
        "- **Report-classified hardware-flat sweeps:** benchmarks may carry "
        "metadata identifying an archived true hardware-flat selected region "
        "whose x-axis was a cache-residency/footprint sweep rather than a "
        "work-amount sweep. The diagnostic-only policy applies only when the "
        "loaded report rows still match the selected-run provenance and "
        "row count/min/max checks."
    )
    lines.append(
        "- **Report-classified cache-bandwidth work sweeps:** current L1/L2 "
        "metadata marks `num_elements` as a work-amount axis and defines total "
        "read bytes. The repeat count is named `num_iterations` in HIP/CUDA "
        "workloads and `num_repeats` in Go simulator runs. For L1, HIP/CUDA "
        "keeps the total `working_set_size` fixed, while the Go simulator uses "
        "per-256-thread read-group reuse and a `num_elements`-scaled input "
        "allocation. These rows are scored and trended only when loaded hardware "
        "rows prove non-flat/value-changing behavior; otherwise the rows remain "
        "diagnostic pending value-change proof."
    )
    lines.append("")
    lines.append("### Per-Benchmark Booster")
    lines.append("")
    lines.append(
        "Each scored benchmark receives a constant booster `c` (fit by median "
        "of `hw - sim`) added to all simulated points. This absorbs "
        "systematic offsets from unmodeled overheads (kernel launch, "
        "driver stack, etc.) without changing the simulator itself. "
        "For report-classified constant-work hardware-flat sweeps, or "
        "work-scaling sweeps pending loaded value-change proof, the booster "
        "value is shown only as a diagnostic and is excluded from aggregate "
        "boosted accuracy and trend/scaling targets."
    )
    lines.append("")
    lines.append("### Primary Metrics")
    lines.append("")
    lines.append(
        "- **Average Error (scored booster):** Mean of per-point "
        "`|sim + c - hw| / hw` for informative scaling axes. Target: "
        "≤ 20% (Tier 2), ≤ 10% (Tier 1)."
    )
    lines.append(
        "- **Scaling Factor Error:** Linear regression of `time` vs "
        "`input_size` for both HW and boosted-sim on informative scaling axes. "
        "Error = `|slope_sim - slope_hw| / |slope_hw|`. Target: ≤ 20% average."
    )
    lines.append("")
    lines.append("### Supporting Metrics")
    lines.append("")
    lines.append(
        "- **WMAPE** (Weighted Mean Absolute Percentage Error): "
        "`Σ|sim - hw| / Σ hw`"
    )
    lines.append(
        "- **Spearman ρ**: Rank correlation between HW and sim times "
        "(requires ≥ 3 points)"
    )
    lines.append("")
    lines.append("### Minimum Point Requirements")
    lines.append("")
    lines.append(
        "- **Tier 2:** ≥ 1 flat point + ≥ 5 non-flat points"
    )
    lines.append(
        "- **Tier 1:** ≥ 2 points per auto-detected regime, ≥ 5 total "
        "(subject to author review)"
    )
    if report_context.get("scope") == "curated":
        lines.append(
            "- **Curated partial entries:** retained manifest entries with "
            "`coverage_status=partial` are not counted as full point-coverage "
            "passes; their current CSV gaps are reported explicitly."
        )
    lines.append("")
    lines.append("### Accuracy Targets")
    lines.append("")
    lines.append(
        "| Tier | Avg Error (scored booster) | Scaling Factor Error |"
    )
    lines.append(
        "|------|--------------------:|---------------------:|"
    )
    lines.append(
        "| Tier 1 (microbenchmarks) | ≤ 10% | ≤ 20% |"
    )
    lines.append(
        "| Tier 2 (algorithm kernels) | ≤ 20% | ≤ 20% |"
    )
    lines.append("")
    lines.append("### References")
    lines.append("")
    lines.append(
        "- **Point selection policy:** "
        "[validation_point_selection.md](validation_point_selection.md)"
    )
    lines.append(
        f"- **Raw data:** `{raw_data_source}`"
    )
    lines.append(
        "- **Generator:** `scripts/generate_validation_report.py`"
    )
    lines.append("")

    return markdown_without_trailing_whitespace(lines)


def markdown_without_trailing_whitespace(lines):
    """Join generated Markdown lines after trimming trailing spaces and tabs."""
    return "\n".join(line.rstrip(" \t") for line in lines)


def build_report_summary(
    scope,
    csv_path,
    output_path,
    figures_dir,
    classified,
    compliance,
    kernel_metrics,
    agg,
    figures,
    slowdown_summary,
    trend_results,
    manifest=None,
    manifest_path=None,
    entries=None,
    manifest_summary=None,
    tier_lookup=None,
    category_map=None,
):
    """Build a deterministic machine-readable summary for report outputs."""
    entries = entries or []
    category_map = category_map or {}
    tier_lookup = tier_lookup or {}

    if entries:
        tier_counts = Counter(str(entry["tier"]) for entry in entries)
        category_counts = Counter(entry["category"] for entry in entries)
        kernel_order = [entry["kernel_name"] for entry in entries]
    else:
        tier_counts = Counter(str(lookup_tier(k, tier_lookup)) for k in classified)
        category_counts = Counter(category_map.get(k, "unclassified") for k in classified)
        kernel_order = sorted(classified.keys())

    trend_by_kernel = {entry["kernel"]: entry for entry in (trend_results or [])}
    manifest_kernel_summary = {
        entry["kernel_name"]: entry
        for entry in (manifest_summary or {}).get("kernels", [])
    }
    figure_files = [repo_relative(figures_dir / fname) for _, fname in figures]
    if slowdown_summary:
        for key in ("slowdown_bar_chart", "sim_vs_hw_scatter"):
            if slowdown_summary.get(key):
                figure_files.append(repo_relative(figures_dir / slowdown_summary[key]))

    kernel_summaries = []
    for kernel in kernel_order:
        if kernel not in classified:
            continue
        c = classified[kernel]
        m = kernel_metrics[kernel]
        comp = compliance.get(kernel, {})
        trend = trend_by_kernel.get(kernel, {})
        report_class = c.get("report_classification", {})
        manifest_kernel = manifest_kernel_summary.get(kernel, {})
        kernel_summaries.append({
            "kernel_name": kernel,
            "tier": lookup_tier(kernel, tier_lookup),
            "category": category_map.get(kernel),
            "classification": {
                "report_classification": report_class.get("classification"),
                "display_label": report_class.get("display_label"),
                "metric_policy": report_class.get("metric_policy"),
                "boosted_metrics_scored": report_class.get("boosted_metrics_scored", True),
                "scaling_informative": report_class.get("scaling_informative", True),
                "scaling_axis": report_class.get("scaling_axis"),
                "scaling_axis_role": report_class.get("scaling_axis_role"),
                "work_amount_scaled_by_axis": report_class.get("work_amount_scaled_by_axis"),
                "work_amount_scaled": report_class.get("work_amount_scaled"),
                "work_amount": report_class.get("work_amount"),
                "repeat_count_parameter_names": report_class.get("repeat_count_parameter_names"),
                "total_read_bytes_formula": report_class.get("total_read_bytes_formula"),
                "default_total_read_bytes": report_class.get("default_total_read_bytes"),
                "implementation_semantics": report_class.get("implementation_semantics"),
                "value_change_proven": report_class.get("value_change_proven"),
                "selected_run_id": report_class.get("selected_run_id"),
                "source": report_class.get("source"),
            },
            "coverage": {
                "matched_points": len(c["all"]),
                "matched_flat_points": len(c["flat"]),
                "matched_non_flat_points": len(c["nonflat"]),
                "hardware_flat_points": c["hw_flat_count"],
                "hardware_non_flat_points": c["hw_nonflat_count"],
                "compliant": bool(comp.get("compliant", False)),
                "reason": comp.get("reason", ""),
                "manifest_status": manifest_kernel.get("coverage_status"),
                "point_coverage_ready": manifest_kernel.get("point_coverage_ready"),
                "coverage_gaps": manifest_kernel.get("coverage_gaps", []),
            },
            "evidence": kernel_evidence_summary(c, m),
            "accuracy": {
                "avg_error": round_metric(m["avg_err"]),
                "avg_error_boosted": round_metric(m["avg_err_adj"]),
                "avg_error_boosted_diagnostic": round_metric(m["avg_err_adj_diagnostic"]),
                "wmape": round_metric(m["wmape_all"]),
                "wmape_nonflat": round_metric(m["wmape_nonflat"]),
                "scaling_factor_error_boosted": round_metric(m["sf_err_adj"]),
                "scaling_factor_error_boosted_diagnostic": round_metric(m["sf_err_adj_diagnostic"]),
                "booster_ms": round_metric(m["booster"]),
                "boosted_metrics_scored": bool(m.get("boosted_metrics_scored", True)),
                "scaling_informative": bool(m.get("scaling_informative", True)),
                "spearman_rho": round_metric(m["rho"]),
                "avg_sim_hw_ratio": round_metric(m["avg_ratio"]),
            },
            "trend": {
                "hw_slope": round_metric(trend.get("hw_slope")),
                "sim_slope": round_metric(trend.get("sim_slope")),
                "trend_ratio": round_metric(trend.get("trend_ratio")),
            },
        })

    trend_tier_counts = Counter(str(entry["tier"]) for entry in (trend_results or []))
    trend_ratios_by_tier = {}
    for tier in ("1", "2"):
        ratios = [
            entry["trend_ratio"] for entry in (trend_results or [])
            if str(entry["tier"]) == tier
        ]
        trend_ratios_by_tier[tier] = round_metric(sum(ratios) / len(ratios)) if ratios else None

    summary = {
        "schema_name": "mgpusim.validation_report_summary",
        "schema_version": 1,
        "scope": scope,
        "data_source": repo_relative(csv_path),
        "report_path": repo_relative(output_path),
        "figure_files": sorted(figure_files),
        "selected_count": len(entries) if entries else len(classified),
        "tier_counts": dict(sorted(tier_counts.items(), key=lambda item: item[0])),
        "category_counts": dict(sorted(category_counts.items(), key=lambda item: item[0])),
        "aggregate_metrics": {
            "matched_kernels": agg["matched_kernels"],
            "total_points": agg["total_points"],
            "flat_points": agg["flat_points"],
            "nonflat_points": agg["nonflat_points"],
            "mean_avg_error_boosted": round_metric(agg["mean_avg_err_adj"]),
            "mean_scaling_factor_error_boosted": round_metric(agg["mean_sf_err_adj"]),
            "avg_error_boosted_benchmarks": agg["n_avg_err_benchmarks"],
            "scaling_factor_error_boosted_benchmarks": agg["n_sf_benchmarks"],
            "boosted_unscored_kernels": agg["boosted_unscored_kernels"],
            "noninformative_scaling_kernels": agg["noninformative_scaling_kernels"],
            "nonflat_wmape": round_metric(agg["wmape_nonflat"]),
            "flat_wmape": round_metric(agg["wmape_flat"]),
            "overall_wmape": round_metric(agg["wmape_all"]),
            "spearman_rho_all": round_metric(agg["rho_all"]),
            "tier1_mean_avg_error_boosted": round_metric(agg["tier1_mean_avg_err_adj"]),
            "tier2_mean_avg_error_boosted": round_metric(agg["tier2_mean_avg_err_adj"]),
            "tier1_mean_scaling_factor_error_boosted": round_metric(agg["tier1_mean_sf_err_adj"]),
            "tier2_mean_scaling_factor_error_boosted": round_metric(agg["tier2_mean_sf_err_adj"]),
        },
        "coverage_status": {
            "compliant": sum(1 for value in compliance.values() if value.get("compliant")),
            "non_compliant": sum(1 for value in compliance.values() if not value.get("compliant")),
            "no_sim_data": sum(1 for c in classified.values() if len(c["all"]) == 0),
        },
        "slowdown_summary": {
            key: round_metric(value)
            for key, value in (slowdown_summary or {}).items()
            if key not in ("slowdown_bar_chart", "sim_vs_hw_scatter")
        },
        "trend_summary": {
            "kernels_with_trend_data": len(trend_results or []),
            "tier_counts": dict(sorted(trend_tier_counts.items(), key=lambda item: item[0])),
            "mean_trend_ratio_by_tier": trend_ratios_by_tier,
        },
        "kernels": kernel_summaries,
    }

    if manifest is not None:
        summary.update({
            "manifest_path": repo_relative(manifest_path or DEFAULT_MANIFEST_PATH),
            "set_id": manifest["set_id"],
            "set_version": manifest["set_version"],
            "manifest_schema_name": manifest["schema_name"],
            "manifest_schema_version": manifest["schema_version"],
            "broad_suite_diagnostics_policy": CURATED_DIAGNOSTIC_POLICY,
        })
    if manifest_summary is not None:
        summary["point_coverage_status"] = manifest_summary["point_coverage_status"]
        summary["validator_aggregate_accuracy_trend"] = manifest_summary[
            "aggregate_accuracy_trend"
        ]
        summary["validator_accuracy_trend_fields"] = manifest_summary[
            "accuracy_trend_fields"
        ]

    return summary


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main(argv=None):
    args = parse_args(argv)
    if args.regular_artifact_coverage:
        return run_regular_artifact_coverage(args)

    scope = args.scope
    csv_path = resolve_csv_path(scope, args.csv)
    manifest_path = Path(args.manifest)
    output_path = Path(args.output) if args.output else default_output_path(scope)
    summary_path = Path(args.summary_json) if args.summary_json else default_summary_path(scope)
    figures_dir = Path(args.figures_dir)

    if not csv_path.exists():
        print(f"ERROR: {csv_path} not found", file=sys.stderr)
        return 1

    manifest = None
    entries = []
    manifest_summary = None
    selected_kernels = None
    tier_lookup = None
    category_map = {}
    figure_prefix = ""

    if scope == "curated":
        print(f"Validating curated manifest {manifest_path}...")
        try:
            manifest, entries, manifest_summary = load_and_validate_curated_manifest(
                manifest_path, csv_path
            )
        except ManifestValidationError as exc:
            print(f"ERROR: {exc}", file=sys.stderr)
            return 1
        selected_kernels = [entry["kernel_name"] for entry in entries]
        tier_lookup = {entry["kernel_name"]: entry["tier"] for entry in entries}
        category_map = {entry["kernel_name"]: entry["category"] for entry in entries}
        figure_prefix = CURATED_FIGURE_PREFIX
        print(
            f"  curated set {manifest['set_id']} version {manifest['set_version']} "
            f"with {len(selected_kernels)} selected kernels"
        )

    print(f"Reading {csv_path}...")
    all_rows, matched_by_kernel, hw_by_kernel = load_data(csv_path)

    if selected_kernels is not None:
        all_rows, matched_by_kernel, hw_by_kernel = filter_to_kernels(
            all_rows, matched_by_kernel, hw_by_kernel, selected_kernels
        )
        print(f"  filtered to {len(selected_kernels)} manifest-selected kernels")

    flat_region_metadata = load_report_classification_metadata()
    report_provenance = build_classification_provenance(csv_path)
    print("Classifying points into flat/non-flat regions...")
    classified = classify_points(
        hw_by_kernel,
        matched_by_kernel,
        flat_region_metadata=flat_region_metadata,
        report_provenance=report_provenance,
    )

    print("Checking compliance...")
    compliance = check_compliance(classified)
    n_compliant = sum(1 for v in compliance.values() if v["compliant"])
    n_noncompliant = sum(1 for v in compliance.values() if not v["compliant"])
    print(f"  {n_compliant} compliant, {n_noncompliant} non-compliant")

    print("Computing metrics...")
    kernel_metrics = compute_kernel_metrics(classified)
    agg = compute_aggregate_metrics(
        classified, kernel_metrics, compliance, tier_lookup=tier_lookup
    )
    print(f"  Mean Avg Error (scored booster): {fmt_pct(agg['mean_avg_err_adj'])}")
    print(f"  Mean Scaling Factor Err:  {fmt_pct(agg['mean_sf_err_adj'])}")
    if agg.get("n_boosted_unscored"):
        print(
            "  Boosted metrics unscored: "
            f"{agg['n_boosted_unscored']} "
            f"({', '.join(agg['boosted_unscored_kernels'])})"
        )
    print(f"  Non-flat WMAPE: {fmt_pct(agg['wmape_nonflat'])}")
    print(f"  Flat WMAPE: {fmt_pct(agg['wmape_flat'])}")
    print(f"  Overall WMAPE: {fmt_pct(agg['wmape_all'])}")

    try:
        load_plotting_modules()
    except PlottingDependencyError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1

    print("Generating figures...")
    figures = generate_figures(
        all_rows, classified, figures_dir=figures_dir, filename_prefix=figure_prefix
    )
    print(f"  {len(figures)} figures available in {figures_dir}")

    print("Generating slowdown figures...")
    slowdown_summary = generate_slowdown_figures(
        classified,
        figures_dir=figures_dir,
        filename_prefix=figure_prefix,
        tier_lookup=tier_lookup,
    )
    print(
        f"  {slowdown_summary['slowdown_bar_chart']} and "
        f"{slowdown_summary['sim_vs_hw_scatter']} available"
    )

    print("Computing trend metric...")
    trend_results = compute_trend_metric(classified, tier_lookup=tier_lookup)
    print(f"  {len(trend_results)} benchmarks with trend data")

    tier_counts = (
        manifest_summary["tier_counts"] if manifest_summary else
        dict(sorted(Counter(str(lookup_tier(k, tier_lookup)) for k in classified).items()))
    )
    category_counts = (
        manifest_summary["category_counts"] if manifest_summary else
        dict(sorted(Counter(category_map.get(k, "unclassified") for k in classified).items()))
    )
    report_context = {
        "scope": scope,
        "scope_label": (
            "Curated MI300A primary evaluation set" if scope == "curated" else None
        ),
        "title": (
            "MGPUSim MI300A Curated Validation Report" if scope == "curated"
            else "MGPUSim MI300A Validation Report"
        ),
        "data_source": repo_relative(csv_path),
        "csv_explicit": args.csv is not None,
        "figure_dir_link": figure_link_prefix(output_path, figures_dir),
        "tier_lookup": tier_lookup,
        "category_map": category_map,
        "selected_count": len(entries) if entries else len(classified),
        "tier_counts": tier_counts,
        "category_counts": category_counts,
        "diagnostic_policy": CURATED_DIAGNOSTIC_POLICY,
        "regular_workflow_contract": (
            build_regular_workflow_contract(REGULAR_PLAN_PATH) if scope == "all" else None
        ),
    }
    if manifest is not None:
        report_context.update({
            "set_id": manifest["set_id"],
            "set_version": manifest["set_version"],
            "point_coverage_status": manifest_summary["point_coverage_status"],
        })

    print("Writing report...")
    report = generate_report(
        classified, compliance, kernel_metrics, agg, figures,
        slowdown_summary=slowdown_summary,
        trend_results=trend_results,
        report_context=report_context,
    )
    output_path.parent.mkdir(parents=True, exist_ok=True)
    report_written = write_text_if_changed(output_path, report)
    report_status = "Written" if report_written else "Unchanged"
    print(f"  {report_status} {output_path}")

    summary = build_report_summary(
        scope,
        csv_path,
        output_path,
        figures_dir,
        classified,
        compliance,
        kernel_metrics,
        agg,
        figures,
        slowdown_summary,
        trend_results,
        manifest=manifest,
        manifest_path=manifest_path,
        entries=entries,
        manifest_summary=manifest_summary,
        tier_lookup=tier_lookup,
        category_map=category_map,
    )
    if summary_path is not None:
        summary_path.parent.mkdir(parents=True, exist_ok=True)
        summary_text = json.dumps(summary, indent=2, sort_keys=True) + "\n"
        summary_written = write_text_if_changed(summary_path, summary_text)
        summary_status = "Written" if summary_written else "Unchanged"
        print(f"  {summary_status} {summary_path}")

    # Summary
    print()
    print("=" * 60)
    print("VALIDATION REPORT SUMMARY")
    print("=" * 60)
    if manifest is not None:
        print(f"  Scope:                  curated")
        print(f"  Curated set:            {manifest['set_id']} ({manifest['set_version']})")
        print(f"  Selected kernels:       {len(entries)}")
        print(
            f"  Tier counts:            "
            f"Tier 1={tier_counts.get('1', 0)}, Tier 2={tier_counts.get('2', 0)}"
        )
        point_status = manifest_summary["point_coverage_status"]
        print(
            f"  Coverage status:        "
            f"{point_status['status']} "
            f"({point_status['selected_with_passing_coverage']}/"
            f"{point_status['selected_total']} manifest-full, "
            f"{point_status.get('selected_with_partial_coverage', 0)} partial)"
        )
    else:
        print(f"  Scope:                  all")
    print(f"  Kernels with data:      {agg['matched_kernels']}")
    print(f"  Total matched points:   {agg['total_points']}")
    print(f"    Flat:                 {agg['flat_points']}")
    print(f"    Non-flat:             {agg['nonflat_points']}")
    print(f"  Mean Avg Err (scored):  {fmt_pct(agg['mean_avg_err_adj'])}")
    print(f"  Mean Scaling Factor Err:{fmt_pct(agg['mean_sf_err_adj'])}")
    print(f"  Non-flat WMAPE:         {fmt_pct(agg['wmape_nonflat'])}")
    print(f"  Overall WMAPE:          {fmt_pct(agg['wmape_all'])}")
    print(f"  Compliant:              {n_compliant}")
    print(f"  Non-compliant:          {n_noncompliant}")
    print()
    print("  --- Tier-Separated ---")
    for tier_num in (1, 2):
        prefix = f"tier{tier_num}"
        target = agg[f"{prefix}_target"]
        print(
            f"  Tier {tier_num}: "
            f"Avg Err={fmt_pct(agg[f'{prefix}_mean_avg_err_adj'])} "
            f"(target ≤{target * 100:.0f}%), "
            f"SF Err={fmt_pct(agg[f'{prefix}_mean_sf_err_adj'])}, "
            f"compliant={agg[f'{prefix}_compliant']}/{agg[f'{prefix}_count']}"
        )
    print("=" * 60)

    return 0


if __name__ == "__main__":
    sys.exit(main())
