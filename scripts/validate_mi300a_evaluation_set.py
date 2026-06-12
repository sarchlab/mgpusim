#!/usr/bin/env python3
"""Validate and summarize the curated MI300A evaluation benchmark set.

The validator intentionally stays independent of report/figure generation.  It
checks that the machine-readable manifest is well formed, that every selected
kernel exists in ``benchmark-comparison/comparison_ci.csv``, and that both the
manifest evidence and current CSV data satisfy each benchmark's coverage
expectation.  On success it emits a deterministic, concise summary for the
curated set.
"""

from __future__ import annotations

import argparse
import csv
import json
import math
import re
import sys
from collections import Counter, defaultdict
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterable


REPO_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_CSV = REPO_ROOT / "benchmark-comparison" / "comparison_ci.csv"
DEFAULT_MANIFEST = REPO_ROOT / "benchmark-comparison" / "mi300a_evaluation_set.json"

REQUIRED_CSV_COLUMNS = ("kernel_name", "problem_size", "real_ms", "sim_ms")
REQUIRED_TOP_LEVEL_KEYS = (
    "schema_version",
    "schema_name",
    "set_id",
    "set_version",
    "data_source",
    "benchmarks",
)
REQUIRED_BENCHMARK_KEYS = (
    "kernel_name",
    "tier",
    "category",
    "rationale",
    "coverage_expectation",
    "coverage_evidence",
)
EVIDENCE_INT_FIELDS = (
    "hardware_points",
    "matched_points",
    "hardware_flat_points",
    "matched_flat_points",
    "hardware_non_flat_points",
    "matched_non_flat_points",
    "trend_points",
)
TIER1_POLICY = "tier1_microbenchmark_regime_proxy_v1"
TIER2_POLICY = "tier2_flat_nonflat_v1"
PARTIAL_COVERAGE_POLICY = "partial_current_data_v1"
COVERAGE_STATUS_FULL = "full"
COVERAGE_STATUS_PARTIAL = "partial"
COVERAGE_STATUSES = (COVERAGE_STATUS_FULL, COVERAGE_STATUS_PARTIAL)
TIER1_EXPECTATION_FIELDS = ("min_matched_points", "min_trend_points")
TIER2_EXPECTATION_FIELDS = (
    "min_flat_points_if_hw_flat_exists",
    "min_non_flat_points_if_hw_non_flat_exists",
    "min_trend_points",
)
FLOAT_FIELDS = (
    "avg_abs_rel_error",
    "wmape",
    "mean_signed_rel_error",
    "spearman_rho",
    "trend_ratio",
    "trend_error",
    "hw_slope",
    "sim_slope",
    "avg_sim_hw_ratio",
)


class ValidationError(Exception):
    """Raised when the manifest/CSV pair does not validate."""


@dataclass(frozen=True)
class ComparisonRow:
    kernel_name: str
    problem_size: str
    real_ms: float | None
    sim_ms: float | None


@dataclass(frozen=True)
class CoverageStats:
    hardware_points: int
    matched_points: int
    hardware_flat_points: int
    matched_flat_points: int
    hardware_non_flat_points: int
    matched_non_flat_points: int
    trend_points: int

    def to_dict(self) -> dict[str, int]:
        return {
            "hardware_points": self.hardware_points,
            "matched_points": self.matched_points,
            "hardware_flat_points": self.hardware_flat_points,
            "matched_flat_points": self.matched_flat_points,
            "hardware_non_flat_points": self.hardware_non_flat_points,
            "matched_non_flat_points": self.matched_non_flat_points,
            "trend_points": self.trend_points,
        }


ZERO_COVERAGE = CoverageStats(
    hardware_points=0,
    matched_points=0,
    hardware_flat_points=0,
    matched_flat_points=0,
    hardware_non_flat_points=0,
    matched_non_flat_points=0,
    trend_points=0,
)


# ---------------------------------------------------------------------------
# Parsing helpers
# ---------------------------------------------------------------------------


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Validate benchmark-comparison/mi300a_evaluation_set.json against "
            "benchmark-comparison/comparison_ci.csv and emit a curated-set summary"
        )
    )
    parser.add_argument(
        "--csv",
        default=str(DEFAULT_CSV),
        help="Path to comparison_ci.csv (default: benchmark-comparison/comparison_ci.csv)",
    )
    parser.add_argument(
        "--manifest",
        default=str(DEFAULT_MANIFEST),
        help=(
            "Path to mi300a_evaluation_set.json "
            "(default: benchmark-comparison/mi300a_evaluation_set.json)"
        ),
    )
    parser.add_argument(
        "--summary-format",
        choices=("text", "json"),
        default="text",
        help="Format to print to stdout after validation succeeds",
    )
    parser.add_argument(
        "--summary-json",
        default=None,
        help="Optional path to also write the deterministic JSON summary",
    )
    return parser.parse_args(argv)


def parse_optional_ms(value: str | None, field_name: str, line_no: int) -> float | None:
    if value is None or value.strip() == "":
        return None
    try:
        parsed = float(value)
    except ValueError as exc:
        raise ValidationError(
            f"malformed CSV: line {line_no} has non-numeric {field_name}={value!r}"
        ) from exc
    if not math.isfinite(parsed):
        raise ValidationError(
            f"malformed CSV: line {line_no} has non-finite {field_name}={value!r}"
        )
    if parsed < 0:
        raise ValidationError(
            f"malformed CSV: line {line_no} has negative {field_name}={value!r}"
        )
    if field_name == "real_ms" and parsed == 0:
        raise ValidationError(f"malformed CSV: line {line_no} has zero real_ms")
    return parsed


def load_comparison_csv(csv_path: Path) -> list[ComparisonRow]:
    if not csv_path.is_file():
        raise ValidationError(f"comparison CSV not found: {csv_path}")

    with csv_path.open("r", newline="", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        if reader.fieldnames is None:
            raise ValidationError("malformed CSV: missing header")
        missing = [col for col in REQUIRED_CSV_COLUMNS if col not in reader.fieldnames]
        if missing:
            raise ValidationError(
                "malformed CSV: missing required columns: " + ", ".join(missing)
            )

        rows: list[ComparisonRow] = []
        for line_no, row in enumerate(reader, start=2):
            kernel_name = row.get("kernel_name", "").strip()
            problem_size = row.get("problem_size", "").strip()
            if not kernel_name:
                raise ValidationError(f"malformed CSV: line {line_no} has empty kernel_name")
            if not problem_size:
                raise ValidationError(f"malformed CSV: line {line_no} has empty problem_size")

            real_ms = parse_optional_ms(row.get("real_ms"), "real_ms", line_no)
            sim_ms = parse_optional_ms(row.get("sim_ms"), "sim_ms", line_no)
            rows.append(
                ComparisonRow(
                    kernel_name=kernel_name,
                    problem_size=problem_size,
                    real_ms=real_ms,
                    sim_ms=sim_ms,
                )
            )

    if not rows:
        raise ValidationError("malformed CSV: comparison CSV has no data rows")
    return rows


def load_manifest(manifest_path: Path) -> dict[str, Any]:
    if not manifest_path.is_file():
        raise ValidationError(f"manifest not found: {manifest_path}")
    try:
        data = json.loads(manifest_path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise ValidationError(f"malformed manifest: invalid JSON: {exc}") from exc
    if not isinstance(data, dict):
        raise ValidationError("malformed manifest: top-level JSON value must be an object")
    return data


def require_nonempty_str(value: Any, path: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise ValidationError(f"malformed manifest: {path} must be a non-empty string")
    return value.strip()


def require_nonnegative_int(value: Any, path: str) -> int:
    if isinstance(value, bool) or not isinstance(value, int) or value < 0:
        raise ValidationError(f"malformed manifest: {path} must be a non-negative integer")
    return value


def require_bool(value: Any, path: str) -> bool:
    if not isinstance(value, bool):
        raise ValidationError(f"malformed manifest: {path} must be a boolean")
    return value


def validate_manifest_shape(manifest: dict[str, Any]) -> list[dict[str, Any]]:
    missing = [key for key in REQUIRED_TOP_LEVEL_KEYS if key not in manifest]
    if missing:
        raise ValidationError(
            "malformed manifest: missing top-level keys: " + ", ".join(missing)
        )

    for key in ("schema_name", "set_id", "set_version", "data_source"):
        require_nonempty_str(manifest.get(key), key)
    require_nonnegative_int(manifest.get("schema_version"), "schema_version")

    benchmarks = manifest.get("benchmarks")
    if not isinstance(benchmarks, list) or not benchmarks:
        raise ValidationError("malformed manifest: benchmarks must be a non-empty list")

    seen: set[str] = set()
    for idx, entry in enumerate(benchmarks):
        prefix = f"benchmarks[{idx}]"
        if not isinstance(entry, dict):
            raise ValidationError(f"malformed manifest: {prefix} must be an object")
        missing_entry_keys = [key for key in REQUIRED_BENCHMARK_KEYS if key not in entry]
        if missing_entry_keys:
            raise ValidationError(
                f"malformed manifest: {prefix} missing required keys: "
                + ", ".join(missing_entry_keys)
            )

        kernel_name = require_nonempty_str(entry.get("kernel_name"), f"{prefix}.kernel_name")
        if kernel_name in seen:
            raise ValidationError(
                f"malformed manifest: duplicate selected kernel {kernel_name!r}"
            )
        seen.add(kernel_name)

        tier = entry.get("tier")
        if isinstance(tier, bool) or tier not in (1, 2):
            raise ValidationError(f"malformed manifest: {prefix}.tier must be 1 or 2")
        require_nonempty_str(entry.get("category"), f"{prefix}.category")
        require_nonempty_str(entry.get("rationale"), f"{prefix}.rationale")

        expectation = entry.get("coverage_expectation")
        if not isinstance(expectation, dict):
            raise ValidationError(
                f"malformed manifest: {prefix}.coverage_expectation must be an object"
            )
        policy = require_nonempty_str(
            expectation.get("policy"), f"{prefix}.coverage_expectation.policy"
        )
        if tier == 1:
            if policy != TIER1_POLICY:
                raise ValidationError(
                    f"malformed manifest: {prefix}.coverage_expectation.policy "
                    f"must be {TIER1_POLICY!r} for tier 1"
                )
            expectation_fields = TIER1_EXPECTATION_FIELDS
        else:
            if policy != TIER2_POLICY:
                raise ValidationError(
                    f"malformed manifest: {prefix}.coverage_expectation.policy "
                    f"must be {TIER2_POLICY!r} for tier 2"
                )
            expectation_fields = TIER2_EXPECTATION_FIELDS
        missing_expectation = [field for field in expectation_fields if field not in expectation]
        if missing_expectation:
            raise ValidationError(
                f"malformed manifest: {prefix}.coverage_expectation missing required fields: "
                + ", ".join(missing_expectation)
            )
        for field in expectation_fields:
            require_nonnegative_int(expectation.get(field), f"{prefix}.coverage_expectation.{field}")
        partial_allowed = expectation.get("partial_coverage_allowed", False)
        if not isinstance(partial_allowed, bool):
            raise ValidationError(
                f"malformed manifest: {prefix}.coverage_expectation.partial_coverage_allowed "
                "must be a boolean"
            )
        if partial_allowed:
            partial_policy = require_nonempty_str(
                expectation.get("partial_coverage_policy"),
                f"{prefix}.coverage_expectation.partial_coverage_policy",
            )
            if partial_policy != PARTIAL_COVERAGE_POLICY:
                raise ValidationError(
                    f"malformed manifest: {prefix}.coverage_expectation.partial_coverage_policy "
                    f"must be {PARTIAL_COVERAGE_POLICY!r} when partial coverage is allowed"
                )

        evidence = entry.get("coverage_evidence")
        if not isinstance(evidence, dict):
            raise ValidationError(
                f"malformed manifest: {prefix}.coverage_evidence must be an object"
            )
        missing_evidence = [field for field in EVIDENCE_INT_FIELDS if field not in evidence]
        if missing_evidence:
            raise ValidationError(
                f"malformed manifest: {prefix}.coverage_evidence missing required fields: "
                + ", ".join(missing_evidence)
            )
        for field in EVIDENCE_INT_FIELDS:
            require_nonnegative_int(evidence.get(field), f"{prefix}.coverage_evidence.{field}")
        point_ready = require_bool(
            evidence.get("point_coverage_ready"),
            f"{prefix}.coverage_evidence.point_coverage_ready",
        )
        coverage_status = evidence.get("coverage_status", COVERAGE_STATUS_FULL)
        if coverage_status not in COVERAGE_STATUSES:
            raise ValidationError(
                f"malformed manifest: {prefix}.coverage_evidence.coverage_status "
                f"must be one of {COVERAGE_STATUSES!r}"
            )
        if coverage_status == COVERAGE_STATUS_PARTIAL:
            if not partial_allowed:
                raise ValidationError(
                    f"malformed manifest: {prefix}.coverage_evidence.coverage_status "
                    "cannot be partial unless coverage_expectation.partial_coverage_allowed is true"
                )
            if point_ready:
                raise ValidationError(
                    f"malformed manifest: {prefix}.coverage_evidence.point_coverage_ready "
                    "must be false for partial coverage entries"
                )
            require_nonempty_str(
                evidence.get("partial_coverage_reason"),
                f"{prefix}.coverage_evidence.partial_coverage_reason",
            )
            gap_entries = evidence.get("coverage_gaps")
            if not isinstance(gap_entries, list) or not gap_entries:
                raise ValidationError(
                    f"malformed manifest: {prefix}.coverage_evidence.coverage_gaps "
                    "must be a non-empty list for partial coverage entries"
                )
            for gap_idx, gap in enumerate(gap_entries):
                gap_prefix = f"{prefix}.coverage_evidence.coverage_gaps[{gap_idx}]"
                if not isinstance(gap, dict):
                    raise ValidationError(f"malformed manifest: {gap_prefix} must be an object")
                require_nonempty_str(gap.get("field"), f"{gap_prefix}.field")
                require_nonnegative_int(gap.get("observed"), f"{gap_prefix}.observed")
                require_nonnegative_int(gap.get("required"), f"{gap_prefix}.required")
        elif not point_ready:
            raise ValidationError(
                f"malformed manifest: {prefix}.coverage_evidence.point_coverage_ready "
                "can only be false when coverage_status is partial"
            )

    return benchmarks


# ---------------------------------------------------------------------------
# Coverage and metric helpers
# ---------------------------------------------------------------------------


def parse_input_size(size: str) -> float | None:
    """Parse a benchmark problem-size label into a numeric value for trends."""
    stripped = size.strip()
    if not stripped:
        return None

    prefixed = re.match(r"^[A-Za-z]+(\d.*)$", stripped)
    if prefixed:
        stripped = prefixed.group(1)

    kb_match = re.fullmatch(r"(\d+(?:\.\d+)?)\s*KB", stripped, re.IGNORECASE)
    if kb_match:
        return float(kb_match.group(1)) * 1024

    mb_match = re.fullmatch(r"(\d+(?:\.\d+)?)\s*MB", stripped, re.IGNORECASE)
    if mb_match:
        return float(mb_match.group(1)) * 1024 * 1024

    dims_match = re.fullmatch(r"(\d+)(?:x(\d+))?(?:x(\d+))?", stripped)
    if dims_match:
        product = 1
        for dim in dims_match.groups():
            if dim is not None:
                product *= int(dim)
        return float(product)

    numeric_prefix = re.match(r"(\d+(?:\.\d+)?)", stripped)
    if numeric_prefix:
        return float(numeric_prefix.group(1))

    return None


def linear_regression(x_values: list[float], y_values: list[float]) -> tuple[float | None, float | None]:
    if len(x_values) < 2:
        return None, None
    n = len(x_values)
    sx = sum(x_values)
    sy = sum(y_values)
    sxx = sum(x * x for x in x_values)
    sxy = sum(x * y for x, y in zip(x_values, y_values))
    denom = n * sxx - sx * sx
    if abs(denom) < 1e-15:
        return None, None
    slope = (n * sxy - sx * sy) / denom
    intercept = (sy - slope * sx) / n
    return slope, intercept


def spearman(x_values: list[float], y_values: list[float]) -> float | None:
    n = len(x_values)
    if n < 3:
        return None

    def rank(values: list[float]) -> list[float]:
        ordered = sorted(range(n), key=lambda idx: (values[idx], idx))
        ranks = [0.0] * n
        for rank_idx, original_idx in enumerate(ordered, start=1):
            ranks[original_idx] = float(rank_idx)
        return ranks

    x_ranks = rank(x_values)
    y_ranks = rank(y_values)
    d2_sum = sum((x_ranks[i] - y_ranks[i]) ** 2 for i in range(n))
    return 1 - 6 * d2_sum / (n * (n * n - 1))


def matched_rows(rows: Iterable[ComparisonRow]) -> list[ComparisonRow]:
    return [row for row in rows if row.real_ms is not None and row.sim_ms is not None]


def compute_coverage_stats(rows: list[ComparisonRow]) -> dict[str, CoverageStats]:
    rows_by_kernel: dict[str, list[ComparisonRow]] = defaultdict(list)
    for row in rows:
        rows_by_kernel[row.kernel_name].append(row)

    stats: dict[str, CoverageStats] = {}
    for kernel_name, kernel_rows in rows_by_kernel.items():
        hw_rows = [row for row in kernel_rows if row.real_ms is not None]
        matched = matched_rows(kernel_rows)
        if not hw_rows:
            stats[kernel_name] = ZERO_COVERAGE
            continue

        min_real = min(row.real_ms for row in hw_rows if row.real_ms is not None)
        threshold = 2 * min_real
        hw_flat = [row for row in hw_rows if row.real_ms is not None and row.real_ms <= threshold]
        hw_nonflat = [row for row in hw_rows if row.real_ms is not None and row.real_ms > threshold]
        matched_flat = [
            row for row in matched if row.real_ms is not None and row.real_ms <= threshold
        ]
        matched_nonflat = [
            row for row in matched if row.real_ms is not None and row.real_ms > threshold
        ]
        trend_points = sum(1 for row in matched if parse_input_size(row.problem_size) is not None)
        stats[kernel_name] = CoverageStats(
            hardware_points=len(hw_rows),
            matched_points=len(matched),
            hardware_flat_points=len(hw_flat),
            matched_flat_points=len(matched_flat),
            hardware_non_flat_points=len(hw_nonflat),
            matched_non_flat_points=len(matched_nonflat),
            trend_points=trend_points,
        )

    return stats


def round_float(value: float | None, digits: int = 6) -> float | None:
    if value is None or not math.isfinite(value):
        return None
    return round(value, digits)


def compute_accuracy_trend_metrics(rows: list[ComparisonRow]) -> dict[str, float | None]:
    matched = matched_rows(rows)
    if not matched:
        return {field: None for field in FLOAT_FIELDS}

    abs_errors = [
        abs(row.sim_ms - row.real_ms) / row.real_ms
        for row in matched
        if row.real_ms is not None and row.sim_ms is not None and row.real_ms > 0
    ]
    signed_errors = [
        (row.sim_ms - row.real_ms) / row.real_ms
        for row in matched
        if row.real_ms is not None and row.sim_ms is not None and row.real_ms > 0
    ]
    abs_error_ms = [
        abs(row.sim_ms - row.real_ms)
        for row in matched
        if row.real_ms is not None and row.sim_ms is not None
    ]
    real_sum = sum(row.real_ms for row in matched if row.real_ms is not None)
    ratios = [
        row.sim_ms / row.real_ms
        for row in matched
        if row.real_ms is not None and row.sim_ms is not None and row.real_ms > 0
    ]

    trend_ratio: float | None = None
    trend_error: float | None = None
    hw_slope: float | None = None
    sim_slope: float | None = None
    parsed_sizes = [parse_input_size(row.problem_size) for row in matched]
    if len(matched) >= 3 and all(size is not None for size in parsed_sizes):
        sizes = [float(size) for size in parsed_sizes if size is not None]
        hw_times = [row.real_ms for row in matched if row.real_ms is not None]
        sim_times = [row.sim_ms for row in matched if row.sim_ms is not None]
        hw_slope, _ = linear_regression(sizes, hw_times)
        sim_slope, _ = linear_regression(sizes, sim_times)
        if hw_slope is not None and sim_slope is not None and abs(hw_slope) >= 1e-15:
            trend_ratio = sim_slope / hw_slope
            trend_error = abs(sim_slope - hw_slope) / abs(hw_slope)

    rho: float | None = None
    if len(matched) >= 3:
        real_values = [row.real_ms for row in matched if row.real_ms is not None]
        sim_values = [row.sim_ms for row in matched if row.sim_ms is not None]
        rho = spearman(real_values, sim_values)

    metrics = {
        "avg_abs_rel_error": sum(abs_errors) / len(abs_errors) if abs_errors else None,
        "wmape": sum(abs_error_ms) / real_sum if real_sum > 0 else None,
        "mean_signed_rel_error": (
            sum(signed_errors) / len(signed_errors) if signed_errors else None
        ),
        "spearman_rho": rho,
        "trend_ratio": trend_ratio,
        "trend_error": trend_error,
        "hw_slope": hw_slope,
        "sim_slope": sim_slope,
        "avg_sim_hw_ratio": sum(ratios) / len(ratios) if ratios else None,
    }
    return {key: round_float(value) for key, value in metrics.items()}


def expectation_gap_records(
    tier: int, expectation: dict[str, Any], stats: CoverageStats
) -> list[dict[str, int | str]]:
    """Return deterministic full-coverage gaps for one stats record."""
    gaps: list[dict[str, int | str]] = []

    def add_gap(field: str, observed: int, required: int) -> None:
        gaps.append({"field": field, "observed": observed, "required": required})

    if tier == 1:
        min_matched = expectation["min_matched_points"]
        min_trend = expectation["min_trend_points"]
        if stats.matched_points < min_matched:
            add_gap("matched_points", stats.matched_points, min_matched)
        if stats.trend_points < min_trend:
            add_gap("trend_points", stats.trend_points, min_trend)
    else:
        min_flat = expectation["min_flat_points_if_hw_flat_exists"]
        min_nonflat = expectation["min_non_flat_points_if_hw_non_flat_exists"]
        min_trend = expectation["min_trend_points"]
        if stats.hardware_flat_points > 0 and stats.matched_flat_points < min_flat:
            add_gap("matched_flat_points", stats.matched_flat_points, min_flat)
        if stats.hardware_non_flat_points > 0 and stats.matched_non_flat_points < min_nonflat:
            add_gap("matched_non_flat_points", stats.matched_non_flat_points, min_nonflat)
        if stats.trend_points < min_trend:
            add_gap("trend_points", stats.trend_points, min_trend)
    return gaps


def gap_records_to_failures(
    kernel_name: str, gaps: list[dict[str, int | str]], source_label: str
) -> list[str]:
    return [
        f"coverage failure for {kernel_name}: {source_label} "
        f"{gap['field']} {gap['observed']}/{gap['required']}"
        for gap in gaps
    ]


def check_expectation(
    kernel_name: str,
    tier: int,
    expectation: dict[str, Any],
    stats: CoverageStats,
    source_label: str,
) -> list[str]:
    gaps = expectation_gap_records(tier, expectation, stats)
    return gap_records_to_failures(kernel_name, gaps, source_label)


def evidence_to_stats(evidence: dict[str, Any]) -> CoverageStats:
    return CoverageStats(
        hardware_points=evidence["hardware_points"],
        matched_points=evidence["matched_points"],
        hardware_flat_points=evidence["hardware_flat_points"],
        matched_flat_points=evidence["matched_flat_points"],
        hardware_non_flat_points=evidence["hardware_non_flat_points"],
        matched_non_flat_points=evidence["matched_non_flat_points"],
        trend_points=evidence["trend_points"],
    )


def is_partial_coverage_entry(entry: dict[str, Any]) -> bool:
    return entry["coverage_evidence"].get("coverage_status") == COVERAGE_STATUS_PARTIAL


def manifest_gap_records(evidence: dict[str, Any]) -> list[dict[str, int | str]]:
    return [
        {
            "field": gap["field"],
            "observed": gap["observed"],
            "required": gap["required"],
        }
        for gap in evidence.get("coverage_gaps", [])
    ]


def validate_coverage(
    entries: list[dict[str, Any]], rows: list[ComparisonRow], stats_by_kernel: dict[str, CoverageStats]
) -> None:
    all_csv_kernels = {row.kernel_name for row in rows}
    errors: list[str] = []

    for entry in entries:
        kernel_name = entry["kernel_name"]
        if kernel_name not in all_csv_kernels:
            errors.append(f"unknown selected kernel: {kernel_name}")
            continue

        current_stats = stats_by_kernel.get(kernel_name, ZERO_COVERAGE)
        evidence = entry["coverage_evidence"]
        evidence_stats = evidence_to_stats(evidence)
        expectation = entry["coverage_expectation"]
        tier = entry["tier"]

        current_dict = current_stats.to_dict()
        evidence_dict = evidence_stats.to_dict()
        for field in EVIDENCE_INT_FIELDS:
            if evidence_dict[field] > current_dict[field]:
                errors.append(
                    f"coverage evidence for {kernel_name} overstates current data: "
                    f"{field} evidence={evidence_dict[field]} current={current_dict[field]}"
                )

        evidence_gaps = expectation_gap_records(tier, expectation, evidence_stats)
        current_gaps = expectation_gap_records(tier, expectation, current_stats)
        if is_partial_coverage_entry(entry):
            for field in EVIDENCE_INT_FIELDS:
                if evidence_dict[field] != current_dict[field]:
                    errors.append(
                        f"partial coverage evidence for {kernel_name} must match current data: "
                        f"{field} evidence={evidence_dict[field]} current={current_dict[field]}"
                    )
            if not current_gaps:
                errors.append(
                    f"coverage failure for {kernel_name}: partial coverage entry has "
                    "no current CSV coverage gaps"
                )
            if evidence_gaps != current_gaps:
                errors.append(
                    f"coverage gaps for {kernel_name} do not match current data: "
                    f"manifest evidence gaps={evidence_gaps} current={current_gaps}"
                )
            declared_gaps = manifest_gap_records(evidence)
            if declared_gaps != current_gaps:
                errors.append(
                    f"coverage_gaps for {kernel_name} do not match current data: "
                    f"manifest={declared_gaps} current={current_gaps}"
                )
            continue

        if not evidence["point_coverage_ready"]:
            errors.append(
                f"coverage failure for {kernel_name}: "
                "coverage_evidence.point_coverage_ready is false"
            )
        errors.extend(gap_records_to_failures(kernel_name, evidence_gaps, "manifest evidence"))
        errors.extend(gap_records_to_failures(kernel_name, current_gaps, "current CSV"))

    if errors:
        raise ValidationError("validation failed:\n- " + "\n- ".join(errors))


# ---------------------------------------------------------------------------
# Summary construction/rendering
# ---------------------------------------------------------------------------


def rows_by_kernel(rows: list[ComparisonRow]) -> dict[str, list[ComparisonRow]]:
    grouped: dict[str, list[ComparisonRow]] = defaultdict(list)
    for row in rows:
        grouped[row.kernel_name].append(row)
    return grouped


def summarize_entry(
    entry: dict[str, Any],
    current_stats: CoverageStats,
    kernel_rows: list[ComparisonRow],
) -> dict[str, Any]:
    metrics = compute_accuracy_trend_metrics(kernel_rows)
    gaps = expectation_gap_records(entry["tier"], entry["coverage_expectation"], current_stats)
    status = COVERAGE_STATUS_PARTIAL if is_partial_coverage_entry(entry) else "pass"
    return {
        "kernel_name": entry["kernel_name"],
        "tier": entry["tier"],
        "category": entry["category"],
        "coverage_status": status,
        "point_coverage_ready": entry["coverage_evidence"]["point_coverage_ready"],
        "partial_coverage_reason": entry["coverage_evidence"].get("partial_coverage_reason"),
        "coverage_gaps": gaps,
        "coverage": current_stats.to_dict(),
        "accuracy_trend": metrics,
    }


def average_available(values: Iterable[float | None]) -> float | None:
    present = [value for value in values if value is not None]
    if not present:
        return None
    return round_float(sum(present) / len(present))


def build_summary(
    manifest: dict[str, Any],
    entries: list[dict[str, Any]],
    rows: list[ComparisonRow],
    stats_by_kernel: dict[str, CoverageStats],
) -> dict[str, Any]:
    grouped_rows = rows_by_kernel(rows)
    kernel_summaries = [
        summarize_entry(
            entry,
            stats_by_kernel.get(entry["kernel_name"], ZERO_COVERAGE),
            grouped_rows.get(entry["kernel_name"], []),
        )
        for entry in entries
    ]

    tier_counts = Counter(str(entry["tier"]) for entry in entries)
    category_counts = Counter(entry["category"] for entry in entries)
    total_coverage = CoverageStats(
        hardware_points=sum(k["coverage"]["hardware_points"] for k in kernel_summaries),
        matched_points=sum(k["coverage"]["matched_points"] for k in kernel_summaries),
        hardware_flat_points=sum(k["coverage"]["hardware_flat_points"] for k in kernel_summaries),
        matched_flat_points=sum(k["coverage"]["matched_flat_points"] for k in kernel_summaries),
        hardware_non_flat_points=sum(
            k["coverage"]["hardware_non_flat_points"] for k in kernel_summaries
        ),
        matched_non_flat_points=sum(
            k["coverage"]["matched_non_flat_points"] for k in kernel_summaries
        ),
        trend_points=sum(k["coverage"]["trend_points"] for k in kernel_summaries),
    )

    available_fields = [
        field
        for field in FLOAT_FIELDS
        if any(kernel["accuracy_trend"].get(field) is not None for kernel in kernel_summaries)
    ]

    aggregate_accuracy_trend = {
        "mean_avg_abs_rel_error": average_available(
            kernel["accuracy_trend"].get("avg_abs_rel_error") for kernel in kernel_summaries
        ),
        "mean_wmape": average_available(
            kernel["accuracy_trend"].get("wmape") for kernel in kernel_summaries
        ),
        "mean_trend_ratio": average_available(
            kernel["accuracy_trend"].get("trend_ratio") for kernel in kernel_summaries
        ),
        "mean_trend_error": average_available(
            kernel["accuracy_trend"].get("trend_error") for kernel in kernel_summaries
        ),
        "kernels_with_trend_ratio": sum(
            1 for kernel in kernel_summaries if kernel["accuracy_trend"].get("trend_ratio") is not None
        ),
    }

    coverage_status_counts = Counter(kernel["coverage_status"] for kernel in kernel_summaries)
    selected_passing = coverage_status_counts.get("pass", 0)
    selected_partial = coverage_status_counts.get(COVERAGE_STATUS_PARTIAL, 0)
    overall_coverage_status = "pass" if selected_passing == len(entries) else "partial"
    partial_kernels = [
        kernel["kernel_name"]
        for kernel in kernel_summaries
        if kernel["coverage_status"] == COVERAGE_STATUS_PARTIAL
    ]

    return {
        "schema_name": manifest["schema_name"],
        "schema_version": manifest["schema_version"],
        "set_id": manifest["set_id"],
        "set_version": manifest["set_version"],
        "data_source": manifest["data_source"],
        "selected_count": len(entries),
        "tier_counts": dict(sorted(tier_counts.items(), key=lambda item: item[0])),
        "category_counts": dict(sorted(category_counts.items(), key=lambda item: item[0])),
        "point_coverage_status": {
            "status": overall_coverage_status,
            "selected_with_passing_coverage": selected_passing,
            "selected_with_partial_coverage": selected_partial,
            "selected_total": len(entries),
            "coverage_status_counts": dict(sorted(coverage_status_counts.items())),
            "partial_kernels": partial_kernels,
            "totals": total_coverage.to_dict(),
        },
        "accuracy_trend_fields": available_fields,
        "aggregate_accuracy_trend": aggregate_accuracy_trend,
        "kernels": kernel_summaries,
    }


def fmt_optional_float(value: float | None) -> str:
    if value is None:
        return "n/a"
    return f"{value:.6g}"


def render_counts(prefix: str, counts: dict[str, int]) -> str:
    rendered = ", ".join(f"{key}={value}" for key, value in counts.items())
    return f"{prefix}: {rendered if rendered else 'none'}"


def render_text_summary(summary: dict[str, Any]) -> str:
    point_status = summary["point_coverage_status"]
    totals = point_status["totals"]
    lines = [
        (
            "VALIDATION_OK "
            f"curated_set={summary['set_id']} "
            f"version={summary['set_version']} "
            f"selected={summary['selected_count']} "
            f"coverage={summary['point_coverage_status']['status']}"
        ),
        render_counts("tier_counts", summary["tier_counts"]),
        render_counts("category_counts", summary["category_counts"]),
        (
            "point_coverage: "
            f"full={point_status['selected_with_passing_coverage']}/"
            f"{point_status['selected_total']} "
            f"partial={point_status.get('selected_with_partial_coverage', 0)} "
            f"matched={totals['matched_points']}/{totals['hardware_points']} "
            f"flat={totals['matched_flat_points']}/{totals['hardware_flat_points']} "
            f"non_flat={totals['matched_non_flat_points']}/{totals['hardware_non_flat_points']} "
            f"trend_points={totals['trend_points']}"
        ),
        "accuracy_trend_fields: "
        + (", ".join(summary["accuracy_trend_fields"]) or "none"),
        (
            "aggregate_accuracy_trend: "
            f"mean_avg_abs_rel_error="
            f"{fmt_optional_float(summary['aggregate_accuracy_trend']['mean_avg_abs_rel_error'])} "
            f"mean_wmape={fmt_optional_float(summary['aggregate_accuracy_trend']['mean_wmape'])} "
            f"mean_trend_ratio="
            f"{fmt_optional_float(summary['aggregate_accuracy_trend']['mean_trend_ratio'])} "
            f"mean_trend_error="
            f"{fmt_optional_float(summary['aggregate_accuracy_trend']['mean_trend_error'])} "
            f"kernels_with_trend_ratio="
            f"{summary['aggregate_accuracy_trend']['kernels_with_trend_ratio']}"
        ),
        "kernel_summary:",
    ]

    for kernel in summary["kernels"]:
        coverage = kernel["coverage"]
        metrics = kernel["accuracy_trend"]
        gap_text = ",".join(
            f"{gap['field']}={gap['observed']}/{gap['required']}"
            for gap in kernel.get("coverage_gaps", [])
        ) or "none"
        lines.append(
            "  "
            f"{kernel['kernel_name']} "
            f"tier={kernel['tier']} "
            f"category={kernel['category']} "
            f"coverage={kernel['coverage_status']} "
            f"matched={coverage['matched_points']}/{coverage['hardware_points']} "
            f"flat={coverage['matched_flat_points']}/{coverage['hardware_flat_points']} "
            f"non_flat={coverage['matched_non_flat_points']}/{coverage['hardware_non_flat_points']} "
            f"gaps={gap_text} "
            f"avg_abs_rel_error={fmt_optional_float(metrics['avg_abs_rel_error'])} "
            f"wmape={fmt_optional_float(metrics['wmape'])} "
            f"spearman_rho={fmt_optional_float(metrics['spearman_rho'])} "
            f"trend_ratio={fmt_optional_float(metrics['trend_ratio'])} "
            f"trend_error={fmt_optional_float(metrics['trend_error'])}"
        )
    return "\n".join(lines) + "\n"


def validate_and_summarize(csv_path: Path, manifest_path: Path) -> dict[str, Any]:
    rows = load_comparison_csv(csv_path)
    manifest = load_manifest(manifest_path)
    entries = validate_manifest_shape(manifest)
    stats_by_kernel = compute_coverage_stats(rows)
    validate_coverage(entries, rows, stats_by_kernel)
    return build_summary(manifest, entries, rows, stats_by_kernel)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    try:
        summary = validate_and_summarize(Path(args.csv), Path(args.manifest))
    except ValidationError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1

    json_summary = json.dumps(summary, indent=2, sort_keys=True)
    if args.summary_json is not None:
        Path(args.summary_json).write_text(json_summary + "\n", encoding="utf-8")

    if args.summary_format == "json":
        print(json_summary)
    else:
        print(render_text_summary(summary), end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
