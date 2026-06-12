#!/usr/bin/env python3
"""Recompute selected-run adjust_weights evidence from tracked report inputs.

The comparison CSV remains the source of raw simulator timings.  This helper
computes raw, fixed-time-only, and configured affine report-time metrics for the
``adjust_weights`` rows without rewriting any historical CSVs.
"""

from __future__ import annotations

import argparse
import csv
import hashlib
import json
import math
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable, Mapping, Sequence


REPO_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_COMPARISON_CSV = (
    REPO_ROOT
    / "benchmark-comparison"
    / "selected-run-25710089668"
    / "comparison_ci.detailed.csv"
)
DEFAULT_TIER_CONFIG = REPO_ROOT / "scripts" / "benchmark_tiers.json"
DEFAULT_KERNEL = "adjust_weights"

REQUIRED_COLUMNS = ("kernel_name", "problem_size", "real_ms", "sim_ms")
SCHEMA_NAME = "mgpusim.adjust_weights_selected_run_evidence"
SCHEMA_VERSION = 2
TARGET_MEAN_ABS_REL_ERROR = 0.20


@dataclass(frozen=True)
class Sample:
    """One selected-run comparison row for the target kernel."""

    problem_size: str
    numeric_problem_size: float
    real_ms: float
    sim_ms: float | None

    @property
    def completed(self) -> bool:
        return self.sim_ms is not None


@dataclass(frozen=True)
class TimeModel:
    """Affine report-time model applied to raw simulator time."""

    name: str
    sim_scale: float
    fixed_time_ms: float
    source: str

    def adjusted_ms(self, sim_ms: float) -> float:
        return self.sim_scale * sim_ms + self.fixed_time_ms


class EvidenceError(ValueError):
    """Raised when evidence cannot be recomputed from the supplied files."""


def parse_args(argv: Sequence[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Recompute adjust_weights raw/fixed/affine selected-run evidence "
            "from comparison_ci.detailed.csv and scripts/benchmark_tiers.json."
        )
    )
    parser.add_argument(
        "--csv",
        default=str(DEFAULT_COMPARISON_CSV),
        help=(
            "Comparison CSV to read "
            "(default: benchmark-comparison/selected-run-25710089668/"
            "comparison_ci.detailed.csv)."
        ),
    )
    parser.add_argument(
        "--tiers",
        default=str(DEFAULT_TIER_CONFIG),
        help="Tier/calibration JSON to read (default: scripts/benchmark_tiers.json).",
    )
    parser.add_argument(
        "--kernel",
        default=DEFAULT_KERNEL,
        help="Kernel name to summarize (default: adjust_weights).",
    )
    parser.add_argument(
        "--output-json",
        default=None,
        help="Optional path for deterministic JSON evidence.",
    )
    parser.add_argument(
        "--output-md",
        default=None,
        help="Optional path for deterministic Markdown evidence.",
    )
    parser.add_argument(
        "--format",
        choices=("markdown", "json"),
        default="markdown",
        help="stdout format when writing the recomputed evidence (default: markdown).",
    )
    parser.add_argument(
        "--require-tracked-inputs",
        action="store_true",
        help="Fail unless the comparison CSV and tier JSON are tracked by git.",
    )
    return parser.parse_args(argv)


def build_evidence(
    comparison_csv: Path,
    tier_config: Path,
    *,
    kernel: str = DEFAULT_KERNEL,
    require_tracked_inputs: bool = False,
) -> dict:
    """Return deterministic evidence computed from *comparison_csv* and *tier_config*."""

    comparison_csv = Path(comparison_csv)
    tier_config = Path(tier_config)
    tracked_inputs = []
    if require_tracked_inputs:
        tracked_inputs = assert_tracked_inputs((comparison_csv, tier_config))

    samples = load_kernel_samples(comparison_csv, kernel)
    affine = load_affine_model(tier_config, kernel)
    fixed_only = TimeModel(
        name="fixed_time_only",
        sim_scale=1.0,
        fixed_time_ms=affine.fixed_time_ms,
        source=(
            "fixed-time-only sensitivity using the configured fixed_time_ms "
            "with sim_scale=1"
        ),
    )
    raw = TimeModel(name="raw", sim_scale=1.0, fixed_time_ms=0.0, source="raw sim_ms")

    completed = sorted(
        (sample for sample in samples if sample.completed),
        key=lambda sample: sample.numeric_problem_size,
    )
    no_result = sorted(
        (sample for sample in samples if not sample.completed),
        key=lambda sample: sample.numeric_problem_size,
    )
    if not completed:
        raise EvidenceError(f"no completed {kernel!r} rows with populated sim_ms")

    models = [raw, fixed_only, affine]
    model_metrics = {
        model.name: compute_model_metrics(completed, model) for model in models
    }
    best_fixed_only = best_fixed_time_only_model(completed)
    best_fixed_only_metrics = compute_model_metrics(completed, best_fixed_only)
    feasibility = build_feasibility_decision(
        completed,
        model_metrics,
        best_fixed_only_metrics,
    )

    return {
        "schema": SCHEMA_NAME,
        "schema_version": SCHEMA_VERSION,
        "kernel_name": kernel,
        "source_files": {
            "comparison_csv": repo_relative(comparison_csv),
            "comparison_csv_sha256": sha256_file(comparison_csv),
            "tier_config": repo_relative(tier_config),
            "tier_config_sha256": sha256_file(tier_config),
            "tracked_input_check": {
                "required": require_tracked_inputs,
                "passed": require_tracked_inputs,
                "tracked_inputs": tracked_inputs,
            },
        },
        "sample_counts": sample_counts(samples, completed, no_result),
        "models": model_metrics,
        "feasibility": feasibility,
        "completed_samples": completed_sample_rows(completed, models),
        "no_result_samples": [sample_row(sample) for sample in no_result],
    }


def load_kernel_samples(path: Path, kernel: str) -> list[Sample]:
    """Load all rows for *kernel* from a comparison CSV."""

    try:
        with Path(path).open(newline="", encoding="utf-8") as handle:
            reader = csv.DictReader(handle)
            if reader.fieldnames is None:
                raise EvidenceError(f"{path} is empty or missing a CSV header")
            missing = [column for column in REQUIRED_COLUMNS if column not in reader.fieldnames]
            if missing:
                raise EvidenceError(
                    f"{path} is missing required columns: {', '.join(missing)}"
                )
            samples = [
                parse_sample(row, path, reader.line_num)
                for row in reader
                if row.get("kernel_name") == kernel
            ]
    except OSError as exc:
        raise EvidenceError(f"could not read comparison CSV {path}: {exc}") from exc

    if not samples:
        raise EvidenceError(f"no rows found for kernel {kernel!r} in {path}")
    return samples


def parse_sample(row: Mapping[str, str], path: Path, line_number: int) -> Sample:
    problem_size = (row.get("problem_size") or "").strip()
    if not problem_size:
        raise EvidenceError(f"{path}:{line_number}: problem_size is blank")
    numeric_problem_size = finite_float(problem_size, path, line_number, "problem_size")
    real_ms = finite_float(row.get("real_ms"), path, line_number, "real_ms")
    if real_ms <= 0:
        raise EvidenceError(f"{path}:{line_number}: real_ms must be positive")

    raw_sim_ms = (row.get("sim_ms") or "").strip()
    sim_ms = None
    if raw_sim_ms:
        sim_ms = finite_float(raw_sim_ms, path, line_number, "sim_ms")
        if sim_ms <= 0:
            raise EvidenceError(f"{path}:{line_number}: sim_ms must be positive when set")

    return Sample(
        problem_size=problem_size,
        numeric_problem_size=numeric_problem_size,
        real_ms=real_ms,
        sim_ms=sim_ms,
    )


def load_affine_model(path: Path, kernel: str) -> TimeModel:
    """Load the configured report-time affine model for *kernel*."""

    try:
        data = json.loads(Path(path).read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise EvidenceError(f"could not parse tier config {path}: {exc}") from exc
    except OSError as exc:
        raise EvidenceError(f"could not read tier config {path}: {exc}") from exc

    models = data.get("per_benchmark_time_models")
    if not isinstance(models, dict) or kernel not in models:
        raise EvidenceError(
            f"tier config {path} does not define per_benchmark_time_models[{kernel!r}]"
        )
    raw_model = models[kernel]
    if not isinstance(raw_model, dict):
        raise EvidenceError(f"tier config model for {kernel!r} must be an object")

    sim_scale = finite_config_float(raw_model.get("sim_scale"), path, kernel, "sim_scale")
    fixed_time_ms = finite_config_float(
        raw_model.get("fixed_time_ms"), path, kernel, "fixed_time_ms"
    )
    if sim_scale < 0:
        raise EvidenceError(f"tier config model for {kernel!r} has negative sim_scale")
    source = str(raw_model.get("source") or "per_benchmark_time_models")
    return TimeModel(
        name="current_affine",
        sim_scale=sim_scale,
        fixed_time_ms=fixed_time_ms,
        source=source,
    )


def finite_float(value: object, path: Path, line_number: int, field_name: str) -> float:
    try:
        parsed = float(str(value).strip())
    except (TypeError, ValueError) as exc:
        raise EvidenceError(f"{path}:{line_number}: {field_name} must be numeric") from exc
    if not math.isfinite(parsed):
        raise EvidenceError(f"{path}:{line_number}: {field_name} must be finite")
    return parsed


def finite_config_float(value: object, path: Path, kernel: str, field_name: str) -> float:
    try:
        parsed = float(value)
    except (TypeError, ValueError) as exc:
        raise EvidenceError(
            f"tier config {path} model for {kernel!r} field {field_name!r} must be numeric"
        ) from exc
    if not math.isfinite(parsed):
        raise EvidenceError(
            f"tier config {path} model for {kernel!r} field {field_name!r} must be finite"
        )
    return parsed


def compute_model_metrics(samples: Sequence[Sample], model: TimeModel) -> dict:
    predictions = [model.adjusted_ms(assert_sim(sample)) for sample in samples]
    real = [sample.real_ms for sample in samples]
    sizes = [sample.numeric_problem_size for sample in samples]
    signed_rel = [(pred - hw) / hw for pred, hw in zip(predictions, real)]
    abs_rel = [abs(value) for value in signed_rel]
    abs_ms = [abs(pred - hw) for pred, hw in zip(predictions, real)]

    return {
        "sim_scale": model.sim_scale,
        "fixed_time_ms": model.fixed_time_ms,
        "source": model.source,
        "completed_samples": len(samples),
        "mean_abs_rel_error": mean(abs_rel),
        "mean_abs_rel_error_pct": 100.0 * mean(abs_rel),
        "max_abs_rel_error": max(abs_rel),
        "max_abs_rel_error_pct": 100.0 * max(abs_rel),
        "mean_signed_rel_error": mean(signed_rel),
        "mean_signed_rel_error_pct": 100.0 * mean(signed_rel),
        "mean_abs_error_ms": mean(abs_ms),
        "max_abs_error_ms": max(abs_ms),
        "trend_statistics": {
            "real_ms_vs_adjusted_sim_ms": correlation_pair(real, predictions),
            "problem_size_vs_adjusted_sim_ms": correlation_pair(sizes, predictions),
            "problem_size_vs_abs_rel_error": correlation_pair(sizes, abs_rel),
            "problem_size_vs_signed_rel_error": correlation_pair(sizes, signed_rel),
        },
    }


def best_fixed_time_only_model(samples: Sequence[Sample]) -> TimeModel:
    """Return the non-negative fixed-time-only model with minimum mean error."""

    deltas = [sample.real_ms - assert_sim(sample) for sample in samples]
    weights = [1.0 / sample.real_ms for sample in samples]
    fixed_time_ms = max(0.0, weighted_median(deltas, weights))
    return TimeModel(
        name="best_fixed_time_only",
        sim_scale=1.0,
        fixed_time_ms=fixed_time_ms,
        source=(
            "Minimum mean absolute relative error non-negative fixed-time-only "
            "offset over completed selected-run samples; raw sim_ms values are unchanged."
        ),
    )


def weighted_median(values: Sequence[float], weights: Sequence[float]) -> float:
    """Return a deterministic weighted median for positive finite weights."""

    if len(values) != len(weights):
        raise EvidenceError("weighted median inputs must have equal length")
    if not values:
        raise EvidenceError("cannot compute weighted median of an empty sequence")

    pairs = sorted(zip(values, weights), key=lambda pair: pair[0])
    total_weight = sum(weight for _value, weight in pairs)
    if total_weight <= 0 or not math.isfinite(total_weight):
        raise EvidenceError("weighted median weights must have a positive finite sum")

    cumulative = 0.0
    half_weight = total_weight / 2.0
    for value, weight in pairs:
        if weight <= 0 or not math.isfinite(weight):
            raise EvidenceError("weighted median weights must be positive and finite")
        cumulative += weight
        if cumulative >= half_weight:
            return value

    return pairs[-1][0]


def build_feasibility_decision(
    samples: Sequence[Sample],
    model_metrics: Mapping[str, dict],
    best_fixed_only_metrics: dict,
) -> dict:
    """Summarize whether raw or fixed-time-only reporting can meet the target."""

    raw_metrics = model_metrics["raw"]
    fixed_metrics = model_metrics["fixed_time_only"]
    affine_metrics = model_metrics["current_affine"]
    raw_or_configured_fixed_below_target = any(
        metrics["mean_abs_rel_error"] < TARGET_MEAN_ABS_REL_ERROR
        for metrics in (raw_metrics, fixed_metrics)
    )
    best_fixed_below_target = (
        best_fixed_only_metrics["mean_abs_rel_error"] < TARGET_MEAN_ABS_REL_ERROR
    )
    positive_trend_preserved = all(
        has_positive_real_to_sim_trend(metrics)
        for metrics in (
            raw_metrics,
            fixed_metrics,
            affine_metrics,
            best_fixed_only_metrics,
        )
    )
    feasible = (
        raw_or_configured_fixed_below_target or best_fixed_below_target
    ) and positive_trend_preserved

    if feasible:
        decision = "raw_or_fixed_time_only_model_meets_target"
        blocking_behavior = "At least one raw or fixed-time-only model meets the target."
        recommended_limitation = "No raw/fixed-time-only limitation is required."
    else:
        decision = "not_feasible_with_raw_or_fixed_time_only_model"
        blocking_behavior = (
            "Raw adjust_weights timing underpredicts the small-size launch floor and "
            "overpredicts the large-size per-work slope; a single fixed-time offset "
            "cannot correct both regimes."
        )
        recommended_limitation = (
            "Report adjust_weights as an affine report-time calibration. Raw sim_ms "
            "rows remain unchanged, and raw or fixed-time-only mean error is not below "
            "20% on the retained selected-run evidence."
        )

    return {
        "target_mean_abs_rel_error": TARGET_MEAN_ABS_REL_ERROR,
        "target_mean_abs_rel_error_pct": 100.0 * TARGET_MEAN_ABS_REL_ERROR,
        "raw_or_configured_fixed_time_below_target": (
            raw_or_configured_fixed_below_target
        ),
        "best_fixed_time_only_below_target": best_fixed_below_target,
        "positive_real_to_sim_trend_preserved": positive_trend_preserved,
        "decision": decision,
        "blocking_model_behavior": blocking_behavior,
        "recommended_limitation": recommended_limitation,
        "raw_signed_error_shape": raw_signed_error_shape(samples),
        "best_fixed_time_only": best_fixed_only_metrics,
    }


def has_positive_real_to_sim_trend(metrics: Mapping[str, object]) -> bool:
    """Return true when real_ms to adjusted_sim_ms correlations are positive."""

    trend = metrics["trend_statistics"]["real_ms_vs_adjusted_sim_ms"]
    return all(value is None or value > 0 for value in trend.values())


def raw_signed_error_shape(samples: Sequence[Sample]) -> dict:
    """Describe where raw simulator timing changes signed-error direction."""

    signed_rows = []
    for sample in sorted(samples, key=lambda item: item.numeric_problem_size):
        raw_signed_error = (assert_sim(sample) - sample.real_ms) / sample.real_ms
        signed_rows.append((sample, raw_signed_error))

    underpredicted = [sample for sample, error in signed_rows if error < 0]
    overpredicted = [sample for sample, error in signed_rows if error > 0]
    exact = [sample for sample, error in signed_rows if error == 0]

    return {
        "underpredicted_count": len(underpredicted),
        "overpredicted_count": len(overpredicted),
        "exact_count": len(exact),
        "largest_underpredicted_problem_size": (
            underpredicted[-1].problem_size if underpredicted else None
        ),
        "first_overpredicted_problem_size": (
            overpredicted[0].problem_size if overpredicted else None
        ),
    }


def correlation_pair(x_values: Sequence[float], y_values: Sequence[float]) -> dict:
    return {
        "pearson": pearson_correlation(x_values, y_values),
        "spearman": spearman_correlation(x_values, y_values),
    }


def pearson_correlation(
    x_values: Sequence[float], y_values: Sequence[float]
) -> float | None:
    if len(x_values) != len(y_values):
        raise EvidenceError("correlation inputs must have equal length")
    if len(x_values) < 2:
        return None
    x_mean = mean(x_values)
    y_mean = mean(y_values)
    x_delta = [value - x_mean for value in x_values]
    y_delta = [value - y_mean for value in y_values]
    x_norm = math.sqrt(sum(value * value for value in x_delta))
    y_norm = math.sqrt(sum(value * value for value in y_delta))
    if x_norm == 0.0 or y_norm == 0.0:
        return None
    return sum(x * y for x, y in zip(x_delta, y_delta)) / (x_norm * y_norm)


def spearman_correlation(
    x_values: Sequence[float], y_values: Sequence[float]
) -> float | None:
    if len(x_values) != len(y_values):
        raise EvidenceError("correlation inputs must have equal length")
    if len(x_values) < 2:
        return None
    return pearson_correlation(rank_values(x_values), rank_values(y_values))


def rank_values(values: Sequence[float]) -> list[float]:
    """Return average ranks (1-based) so ties have stable Spearman handling."""

    order = sorted(range(len(values)), key=lambda index: values[index])
    ranks = [0.0] * len(values)
    start = 0
    while start < len(order):
        stop = start + 1
        while stop < len(order) and values[order[stop]] == values[order[start]]:
            stop += 1
        average_rank = (start + 1 + stop) / 2.0
        for offset in range(start, stop):
            ranks[order[offset]] = average_rank
        start = stop
    return ranks


def sample_counts(
    all_samples: Sequence[Sample],
    completed: Sequence[Sample],
    no_result: Sequence[Sample],
) -> dict:
    completed_sizes = [sample.problem_size for sample in completed]
    no_result_sizes = [sample.problem_size for sample in no_result]
    largest_completed = completed[-1] if completed else None
    first_no_result = no_result[0] if no_result else None
    contiguous_no_result_tail = None
    if largest_completed is not None and first_no_result is not None:
        contiguous_no_result_tail = all(
            sample.numeric_problem_size < first_no_result.numeric_problem_size
            for sample in completed
        ) and all(
            sample.numeric_problem_size >= first_no_result.numeric_problem_size
            for sample in no_result
        )

    return {
        "total_rows": len(all_samples),
        "completed_samples": len(completed),
        "no_result_samples": len(no_result),
        "completed_problem_sizes": completed_sizes,
        "no_result_problem_sizes": no_result_sizes,
        "largest_completed_problem_size": (
            largest_completed.problem_size if largest_completed is not None else None
        ),
        "first_no_result_problem_size": (
            first_no_result.problem_size if first_no_result is not None else None
        ),
        "contiguous_no_result_tail": contiguous_no_result_tail,
        "boundary_summary": boundary_summary(largest_completed, first_no_result, no_result),
    }


def boundary_summary(
    largest_completed: Sample | None,
    first_no_result: Sample | None,
    no_result: Sequence[Sample],
) -> str:
    if largest_completed is None:
        return "no completed rows with populated sim_ms"
    if first_no_result is None:
        return "all planned rows have populated sim_ms"
    count = len(no_result)
    size_word = "size" if count == 1 else "sizes"
    return (
        f"completed through problem_size {largest_completed.problem_size}; "
        f"no-result starts at {first_no_result.problem_size} and covers "
        f"{count} larger planned {size_word}"
    )


def completed_sample_rows(samples: Sequence[Sample], models: Sequence[TimeModel]) -> list[dict]:
    rows = []
    for sample in samples:
        row = sample_row(sample)
        row["models"] = {}
        for model in models:
            predicted = model.adjusted_ms(assert_sim(sample))
            signed = (predicted - sample.real_ms) / sample.real_ms
            row["models"][model.name] = {
                "adjusted_sim_ms": predicted,
                "signed_rel_error": signed,
                "signed_rel_error_pct": 100.0 * signed,
                "abs_rel_error": abs(signed),
                "abs_rel_error_pct": 100.0 * abs(signed),
                "abs_error_ms": abs(predicted - sample.real_ms),
            }
        rows.append(row)
    return rows


def sample_row(sample: Sample) -> dict:
    return {
        "problem_size": sample.problem_size,
        "real_ms": sample.real_ms,
        "sim_ms": sample.sim_ms,
    }


def assert_sim(sample: Sample) -> float:
    if sample.sim_ms is None:  # defensive; completed samples are prefiltered.
        raise EvidenceError(f"problem_size {sample.problem_size} has no sim_ms")
    return sample.sim_ms


def mean(values: Iterable[float]) -> float:
    values = list(values)
    if not values:
        raise EvidenceError("cannot compute mean of an empty sequence")
    return sum(values) / len(values)


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    try:
        with Path(path).open("rb") as handle:
            for chunk in iter(lambda: handle.read(1024 * 1024), b""):
                digest.update(chunk)
    except OSError as exc:
        raise EvidenceError(f"could not hash {path}: {exc}") from exc
    return digest.hexdigest()


def assert_tracked_inputs(paths: Sequence[Path]) -> list[str]:
    tracked = []
    for path in paths:
        rel_path = repo_relative(path)
        if rel_path.startswith("../") or rel_path == "..":
            raise EvidenceError(f"input is outside the repository: {path}")
        result = subprocess.run(
            ["git", "-C", str(REPO_ROOT), "ls-files", "--error-unmatch", "--", rel_path],
            check=False,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        if result.returncode != 0:
            detail = result.stderr.strip() or result.stdout.strip()
            raise EvidenceError(f"input is not tracked by git: {rel_path} ({detail})")
        tracked.append(rel_path)
    return tracked


def repo_relative(path: Path) -> str:
    path = Path(path)
    try:
        return path.resolve().relative_to(REPO_ROOT).as_posix()
    except ValueError:
        return path.as_posix()


def render_markdown(evidence: Mapping[str, object]) -> str:
    source_files = evidence["source_files"]
    counts = evidence["sample_counts"]
    models = evidence["models"]
    lines = [
        f"# {evidence['kernel_name']} selected-run recomputed evidence",
        "",
        "Inputs are read-only source files; raw comparison CSV rows are not rewritten.",
        "",
        "## Provenance",
        "",
        f"- Comparison CSV: `{source_files['comparison_csv']}` "
        f"(sha256 `{source_files['comparison_csv_sha256']}`)",
        f"- Tier config: `{source_files['tier_config']}` "
        f"(sha256 `{source_files['tier_config_sha256']}`)",
    ]
    tracking = source_files["tracked_input_check"]
    if tracking["required"]:
        tracked = ", ".join(f"`{path}`" for path in tracking["tracked_inputs"])
        lines.append(f"- Git-tracked input check: passed for {tracked}")
    else:
        lines.append("- Git-tracked input check: not requested by this invocation")

    affine = models["current_affine"]
    lines.extend(
        [
            "",
            "## Summary",
            "",
            f"- Rows for kernel: **{counts['total_rows']}**",
            f"- Completed rows with `sim_ms`: **{counts['completed_samples']}**",
            f"- `no-result` rows: **{counts['no_result_samples']}**",
            f"- Boundary: {counts['boundary_summary']}.",
            "- Current affine model from tier config: "
            f"`sim_scale={format_number(affine['sim_scale'])}`, "
            f"`fixed_time_ms={format_number(affine['fixed_time_ms'])}`.",
            f"- Affine source: {affine['source']}",
            "",
            "| Model | sim_scale | fixed_time_ms | Mean error | Max error | "
            "Mean signed error | Mean abs error (ms) | Pearson real→sim | "
            "Spearman real→sim | Pearson size→abs error | Spearman size→abs error |",
            "| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |",
        ]
    )
    for model_key, label in (
        ("raw", "Raw"),
        ("fixed_time_only", "Fixed-time-only"),
        ("current_affine", "Current affine"),
    ):
        metrics = models[model_key]
        real_trend = metrics["trend_statistics"]["real_ms_vs_adjusted_sim_ms"]
        error_trend = metrics["trend_statistics"]["problem_size_vs_abs_rel_error"]
        lines.append(
            "| "
            f"{label} | {format_number(metrics['sim_scale'])} | "
            f"{format_number(metrics['fixed_time_ms'])} | "
            f"{format_pct(metrics['mean_abs_rel_error_pct'])} | "
            f"{format_pct(metrics['max_abs_rel_error_pct'])} | "
            f"{format_pct(metrics['mean_signed_rel_error_pct'])} | "
            f"{format_number(metrics['mean_abs_error_ms'])} | "
            f"{format_correlation(real_trend['pearson'])} | "
            f"{format_correlation(real_trend['spearman'])} | "
            f"{format_correlation(error_trend['pearson'])} | "
            f"{format_correlation(error_trend['spearman'])} |"
        )

    feasibility = evidence["feasibility"]
    best_fixed = feasibility["best_fixed_time_only"]
    shape = feasibility["raw_signed_error_shape"]
    lines.extend(
        [
            "",
            "## Feasibility decision",
            "",
            "- Target for raw or fixed-time-only mean error: "
            f"**<{format_pct(feasibility['target_mean_abs_rel_error_pct'])}**.",
            "- Current raw mean error: "
            f"**{format_pct(models['raw']['mean_abs_rel_error_pct'])}**; "
            "configured fixed-time-only mean error: "
            f"**{format_pct(models['fixed_time_only']['mean_abs_rel_error_pct'])}**.",
            "- Best non-negative fixed-time-only lower bound: "
            f"**{format_pct(best_fixed['mean_abs_rel_error_pct'])}** mean error "
            f"at `fixed_time_ms={format_number(best_fixed['fixed_time_ms'])}` "
            f"(max {format_pct(best_fixed['max_abs_rel_error_pct'])}).",
            "- Raw signed-error shape: underpredicts "
            f"{shape['underpredicted_count']} completed sizes through "
            f"`{shape['largest_underpredicted_problem_size']}` and overpredicts "
            f"{shape['overpredicted_count']} completed sizes starting at "
            f"`{shape['first_overpredicted_problem_size']}`.",
            "- Positive real-to-sim trend preserved: "
            f"`{str(feasibility['positive_real_to_sim_trend_preserved']).lower()}`.",
            "- Decision: "
            f"`{feasibility['decision']}`. "
            f"{feasibility['blocking_model_behavior']}",
            "- Recommended limitation wording: "
            f"{feasibility['recommended_limitation']}",
        ]
    )

    completed_sizes = ", ".join(
        f"`{size}`" for size in counts["completed_problem_sizes"]
    )
    no_result_sizes = ", ".join(
        f"`{size}`" for size in counts["no_result_problem_sizes"]
    )
    lines.extend(
        [
            "",
            "## Problem-size boundary",
            "",
            f"- Completed problem sizes: {completed_sizes}",
            f"- No-result problem sizes: {no_result_sizes}",
            f"- Contiguous no-result tail: `{str(counts['contiguous_no_result_tail']).lower()}`",
            "",
            "## Completed sample details",
            "",
            "| problem_size | real_ms | raw_sim_ms | raw error | fixed-only sim_ms | "
            "fixed-only error | affine sim_ms | affine error |",
            "| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |",
        ]
    )
    for sample in evidence["completed_samples"]:
        raw = sample["models"]["raw"]
        fixed = sample["models"]["fixed_time_only"]
        affine_sample = sample["models"]["current_affine"]
        lines.append(
            "| "
            f"{sample['problem_size']} | "
            f"{format_number(sample['real_ms'])} | "
            f"{format_number(sample['sim_ms'])} | "
            f"{format_pct(raw['abs_rel_error_pct'])} | "
            f"{format_number(fixed['adjusted_sim_ms'])} | "
            f"{format_pct(fixed['abs_rel_error_pct'])} | "
            f"{format_number(affine_sample['adjusted_sim_ms'])} | "
            f"{format_pct(affine_sample['abs_rel_error_pct'])} |"
        )
    lines.append("")
    return "\n".join(lines)


def evidence_to_json(evidence: Mapping[str, object]) -> str:
    return json.dumps(evidence, indent=2) + "\n"


def format_number(value: object) -> str:
    if value is None:
        return "—"
    numeric = float(value)
    return f"{numeric:.12g}"


def format_pct(value: object) -> str:
    if value is None:
        return "—"
    return f"{float(value):.2f}%"


def format_correlation(value: object) -> str:
    if value is None:
        return "—"
    return f"{float(value):.6f}"


def write_text(path: Path, content: str) -> None:
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


def main(argv: Sequence[str] | None = None) -> int:
    args = parse_args(argv)
    try:
        evidence = build_evidence(
            Path(args.csv),
            Path(args.tiers),
            kernel=args.kernel,
            require_tracked_inputs=args.require_tracked_inputs,
        )
        json_text = evidence_to_json(evidence)
        markdown_text = render_markdown(evidence)
        if args.output_json:
            write_text(Path(args.output_json), json_text)
        if args.output_md:
            write_text(Path(args.output_md), markdown_text)
        if args.format == "json":
            sys.stdout.write(json_text)
        else:
            sys.stdout.write(markdown_text)
    except EvidenceError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":  # pragma: no cover
    sys.exit(main())
