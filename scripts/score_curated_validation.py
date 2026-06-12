#!/usr/bin/env python3
"""Build a deterministic target-gap scorecard for the curated MI300A summary.

The scorecard consumes the generated curated validation summary JSON instead of
re-reading simulator data.  By default it is diagnostic: it reports whether the
curated set has coverage and target accuracy/trend gaps, but exits 0 so tuning
work can inspect the current baseline.  Use ``--enforce`` when the same target
checks should be a nonzero gate.
"""

from __future__ import annotations

import argparse
import json
import math
import sys
from pathlib import Path
from typing import Any


REPO_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_SUMMARY = REPO_ROOT / "benchmark-comparison" / "mi300a_evaluation_set_summary.json"

SCORECARD_SCHEMA_NAME = "mgpusim.curated_accuracy_scorecard"
SCORECARD_SCHEMA_VERSION = 1
TARGET_POLICY = "mi300a_curated_accuracy_targets_v1"

TIER1_AVG_ERROR_TARGET = 0.10
TIER2_AVG_ERROR_TARGET = 0.20
MEAN_SCALING_FACTOR_ERROR_TARGET = 0.20

SOURCE_SCHEMA_NAME = "mgpusim.validation_report_summary"
SOURCE_SCHEMA_VERSION = 1
SOURCE_SCOPE = "curated"

AGGREGATE_RESULT_ORDER = (
    "coverage",
    "tier1_avg_error_boosted",
    "tier2_avg_error_boosted",
    "mean_scaling_factor_error_boosted",
)


class ScorecardError(Exception):
    """Raised when the summary cannot be scored."""


# ---------------------------------------------------------------------------
# Parsing and validation helpers
# ---------------------------------------------------------------------------


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Score the curated MI300A validation summary against coverage, "
            "Tier 1/Tier 2 accuracy, and scaling/trend targets."
        )
    )
    parser.add_argument(
        "--summary",
        default=str(DEFAULT_SUMMARY),
        help=(
            "Path to mi300a_evaluation_set_summary.json "
            "(default: benchmark-comparison/mi300a_evaluation_set_summary.json)."
        ),
    )
    parser.add_argument(
        "--format",
        choices=("text", "json"),
        default="text",
        help="Output format for stdout (default: text).",
    )
    parser.add_argument(
        "--json-output",
        default=None,
        help="Optional path to also write the deterministic scorecard JSON.",
    )
    parser.add_argument(
        "--top",
        type=int,
        default=10,
        help="Number of ranked benchmark gaps to show in text output (default: 10).",
    )
    parser.add_argument(
        "--enforce",
        action="store_true",
        help="Exit nonzero when any coverage/accuracy/trend target fails.",
    )
    return parser.parse_args(argv)


def repo_relative(path: Path) -> str:
    try:
        return path.resolve().relative_to(REPO_ROOT).as_posix()
    except ValueError:
        return path.as_posix()


def round_metric(value: float | None, digits: int = 12) -> float | None:
    if value is None:
        return None
    if not math.isfinite(value):
        return None
    return round(value, digits)


def as_mapping(value: Any, path: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        raise ScorecardError(f"malformed summary: {path} must be an object")
    return value


def as_list(value: Any, path: str) -> list[Any]:
    if not isinstance(value, list):
        raise ScorecardError(f"malformed summary: {path} must be a list")
    return value


def as_string(value: Any, path: str) -> str:
    if not isinstance(value, str) or not value:
        raise ScorecardError(f"malformed summary: {path} must be a non-empty string")
    return value


def as_int(value: Any, path: str, *, allow_zero: bool = True) -> int:
    if isinstance(value, bool) or not isinstance(value, int):
        raise ScorecardError(f"malformed summary: {path} must be an integer")
    if value < 0 or (value == 0 and not allow_zero):
        raise ScorecardError(f"malformed summary: {path} must be positive")
    return value


def as_bool(value: Any, path: str) -> bool:
    if not isinstance(value, bool):
        raise ScorecardError(f"malformed summary: {path} must be a boolean")
    return value


def as_number(value: Any, path: str) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ScorecardError(f"malformed summary: {path} must be numeric")
    parsed = float(value)
    if not math.isfinite(parsed):
        raise ScorecardError(f"malformed summary: {path} must be finite")
    return parsed


def as_optional_number(value: Any, path: str) -> float | None:
    if value is None:
        return None
    return as_number(value, path)


def load_summary(summary_path: Path) -> dict[str, Any]:
    if not summary_path.is_file():
        raise ScorecardError(f"summary JSON not found: {summary_path}")
    try:
        loaded = json.loads(summary_path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise ScorecardError(
            f"invalid summary JSON: {summary_path}: {exc.msg} at line {exc.lineno} column {exc.colno}"
        ) from exc
    return as_mapping(loaded, "root")


def validate_summary_shape(summary: dict[str, Any]) -> None:
    schema_name = summary.get("schema_name")
    if schema_name != SOURCE_SCHEMA_NAME:
        raise ScorecardError(
            f"malformed summary: schema_name must be {SOURCE_SCHEMA_NAME!r}, got {schema_name!r}"
        )
    schema_version = summary.get("schema_version")
    if schema_version != SOURCE_SCHEMA_VERSION:
        raise ScorecardError(
            "malformed summary: schema_version must be "
            f"{SOURCE_SCHEMA_VERSION}, got {schema_version!r}"
        )
    scope = summary.get("scope")
    if scope != SOURCE_SCOPE:
        raise ScorecardError(
            f"malformed summary: scope must be {SOURCE_SCOPE!r}, got {scope!r}"
        )

    as_string(summary.get("set_id"), "set_id")
    as_string(summary.get("set_version"), "set_version")
    as_mapping(summary.get("aggregate_metrics"), "aggregate_metrics")
    as_mapping(summary.get("coverage_status"), "coverage_status")
    as_mapping(summary.get("point_coverage_status"), "point_coverage_status")
    as_list(summary.get("kernels"), "kernels")

    aggregate = summary["aggregate_metrics"]
    for field in (
        "tier1_mean_avg_error_boosted",
        "tier2_mean_avg_error_boosted",
        "mean_scaling_factor_error_boosted",
    ):
        as_number(aggregate.get(field), f"aggregate_metrics.{field}")

    coverage_status = summary["coverage_status"]
    for field in ("compliant", "non_compliant", "no_sim_data"):
        as_int(coverage_status.get(field), f"coverage_status.{field}")

    point_status = summary["point_coverage_status"]
    as_string(point_status.get("status"), "point_coverage_status.status")
    as_int(point_status.get("selected_total"), "point_coverage_status.selected_total")
    as_int(
        point_status.get("selected_with_passing_coverage"),
        "point_coverage_status.selected_with_passing_coverage",
    )
    totals = as_mapping(point_status.get("totals"), "point_coverage_status.totals")
    for field in (
        "hardware_points",
        "matched_points",
        "hardware_flat_points",
        "matched_flat_points",
        "hardware_non_flat_points",
        "matched_non_flat_points",
        "trend_points",
    ):
        as_int(totals.get(field), f"point_coverage_status.totals.{field}")

    for idx, kernel in enumerate(summary["kernels"]):
        kpath = f"kernels[{idx}]"
        entry = as_mapping(kernel, kpath)
        as_string(entry.get("kernel_name"), f"{kpath}.kernel_name")
        tier = entry.get("tier")
        if tier not in (1, 2):
            raise ScorecardError(f"malformed summary: {kpath}.tier must be 1 or 2")
        if entry.get("category") is not None:
            as_string(entry.get("category"), f"{kpath}.category")
        coverage = as_mapping(entry.get("coverage"), f"{kpath}.coverage")
        as_bool(coverage.get("compliant"), f"{kpath}.coverage.compliant")
        as_int(coverage.get("matched_points"), f"{kpath}.coverage.matched_points")
        accuracy = as_mapping(entry.get("accuracy"), f"{kpath}.accuracy")
        avg_error = as_optional_number(
            accuracy.get("avg_error_boosted"), f"{kpath}.accuracy.avg_error_boosted"
        )
        avg_error_diagnostic = as_optional_number(
            accuracy.get("avg_error_boosted_diagnostic"),
            f"{kpath}.accuracy.avg_error_boosted_diagnostic",
        )
        if avg_error is None and avg_error_diagnostic is None:
            raise ScorecardError(
                f"malformed summary: {kpath}.accuracy must provide "
                "avg_error_boosted or avg_error_boosted_diagnostic"
            )
        as_optional_number(
            accuracy.get("scaling_factor_error_boosted"),
            f"{kpath}.accuracy.scaling_factor_error_boosted",
        )
        as_optional_number(
            accuracy.get("scaling_factor_error_boosted_diagnostic"),
            f"{kpath}.accuracy.scaling_factor_error_boosted_diagnostic",
        )


def status_for(actual: float, target: float) -> str:
    return "pass" if actual <= target else "fail"


def target_gap(actual: float | None, target: float) -> float | None:
    if actual is None:
        return None
    return round_metric(max(0.0, actual - target))


def signed_gap(actual: float | None, target: float) -> float | None:
    if actual is None:
        return None
    return round_metric(actual - target)


# ---------------------------------------------------------------------------
# Scorecard construction
# ---------------------------------------------------------------------------


def build_coverage_result(summary: dict[str, Any]) -> dict[str, Any]:
    point_status = summary["point_coverage_status"]
    coverage_status = summary["coverage_status"]
    selected_total = as_int(point_status["selected_total"], "point_coverage_status.selected_total")
    selected_passing = as_int(
        point_status["selected_with_passing_coverage"],
        "point_coverage_status.selected_with_passing_coverage",
    )
    missing_selected = max(0, selected_total - selected_passing)
    non_compliant = as_int(coverage_status["non_compliant"], "coverage_status.non_compliant")
    no_sim_data = as_int(coverage_status["no_sim_data"], "coverage_status.no_sim_data")
    status = (
        "pass"
        if point_status["status"] == "pass"
        and missing_selected == 0
        and non_compliant == 0
        and no_sim_data == 0
        else "fail"
    )
    return {
        "status": status,
        "actual": point_status["status"],
        "target": "pass",
        "selected_total": selected_total,
        "selected_with_passing_coverage": selected_passing,
        "non_compliant": non_compliant,
        "no_sim_data": no_sim_data,
        "gap": missing_selected + non_compliant + no_sim_data,
    }


def build_numeric_result(
    *,
    actual: float,
    target: float,
    metric_name: str,
    label: str,
) -> dict[str, Any]:
    return {
        "status": status_for(actual, target),
        "metric": metric_name,
        "label": label,
        "actual": round_metric(actual),
        "target": round_metric(target),
        "gap": target_gap(actual, target),
        "signed_gap": signed_gap(actual, target),
    }


def build_aggregate_target_results(summary: dict[str, Any]) -> dict[str, Any]:
    aggregate = summary["aggregate_metrics"]
    results = {
        "coverage": build_coverage_result(summary),
        "tier1_avg_error_boosted": build_numeric_result(
            actual=as_number(
                aggregate["tier1_mean_avg_error_boosted"],
                "aggregate_metrics.tier1_mean_avg_error_boosted",
            ),
            target=TIER1_AVG_ERROR_TARGET,
            metric_name="tier1_mean_avg_error_boosted",
            label="Tier 1 mean boosted average error",
        ),
        "tier2_avg_error_boosted": build_numeric_result(
            actual=as_number(
                aggregate["tier2_mean_avg_error_boosted"],
                "aggregate_metrics.tier2_mean_avg_error_boosted",
            ),
            target=TIER2_AVG_ERROR_TARGET,
            metric_name="tier2_mean_avg_error_boosted",
            label="Tier 2 mean boosted average error",
        ),
        "mean_scaling_factor_error_boosted": build_numeric_result(
            actual=as_number(
                aggregate["mean_scaling_factor_error_boosted"],
                "aggregate_metrics.mean_scaling_factor_error_boosted",
            ),
            target=MEAN_SCALING_FACTOR_ERROR_TARGET,
            metric_name="mean_scaling_factor_error_boosted",
            label="Mean scaling-factor/trend error",
        ),
    }
    # Preserve insertion order for text callers and make missing/extra mistakes obvious.
    return {name: results[name] for name in AGGREGATE_RESULT_ORDER}


def build_ranked_benchmark_gaps(summary: dict[str, Any]) -> list[dict[str, Any]]:
    ranked: list[dict[str, Any]] = []
    seen: set[str] = set()
    for idx, kernel in enumerate(summary["kernels"]):
        entry = as_mapping(kernel, f"kernels[{idx}]")
        kernel_name = as_string(entry["kernel_name"], f"kernels[{idx}].kernel_name")
        if kernel_name in seen:
            raise ScorecardError(f"malformed summary: duplicate kernel entry: {kernel_name}")
        seen.add(kernel_name)

        tier = entry["tier"]
        avg_target = TIER1_AVG_ERROR_TARGET if tier == 1 else TIER2_AVG_ERROR_TARGET
        accuracy = as_mapping(entry["accuracy"], f"kernels[{idx}].accuracy")
        avg_error = as_optional_number(
            accuracy.get("avg_error_boosted"),
            f"kernels[{idx}].accuracy.avg_error_boosted",
        )
        avg_error_source = "avg_error_boosted"
        if avg_error is None:
            avg_error = as_optional_number(
                accuracy.get("avg_error_boosted_diagnostic"),
                f"kernels[{idx}].accuracy.avg_error_boosted_diagnostic",
            )
            avg_error_source = "avg_error_boosted_diagnostic"
        if avg_error is None:
            raise ScorecardError(
                f"malformed summary: kernels[{idx}].accuracy must provide "
                "avg_error_boosted or avg_error_boosted_diagnostic"
            )
        scaling_error = as_optional_number(
            accuracy.get("scaling_factor_error_boosted"),
            f"kernels[{idx}].accuracy.scaling_factor_error_boosted",
        )
        scaling_error_source = "scaling_factor_error_boosted"
        if scaling_error is None:
            scaling_error = as_optional_number(
                accuracy.get("scaling_factor_error_boosted_diagnostic"),
                f"kernels[{idx}].accuracy.scaling_factor_error_boosted_diagnostic",
            )
            if scaling_error is not None:
                scaling_error_source = "scaling_factor_error_boosted_diagnostic"
        avg_gap = target_gap(avg_error, avg_target)
        scaling_gap = target_gap(scaling_error, MEAN_SCALING_FACTOR_ERROR_TARGET)
        gap_values = {
            "avg_error_boosted": avg_gap if avg_gap is not None else 0.0,
            "scaling_factor_error_boosted": scaling_gap if scaling_gap is not None else 0.0,
        }
        # Prefer the accuracy gap when values tie exactly; otherwise rank by size.
        worst_gap_metric, worst_gap = max(
            gap_values.items(), key=lambda item: (item[1], 1 if item[0] == "avg_error_boosted" else 0)
        )

        coverage = as_mapping(entry["coverage"], f"kernels[{idx}].coverage")
        ranked.append(
            {
                "kernel_name": kernel_name,
                "tier": tier,
                "category": entry.get("category"),
                "coverage_compliant": as_bool(
                    coverage["compliant"], f"kernels[{idx}].coverage.compliant"
                ),
                "matched_points": as_int(
                    coverage["matched_points"], f"kernels[{idx}].coverage.matched_points"
                ),
                "avg_error_boosted": round_metric(avg_error),
                "avg_error_source": avg_error_source,
                "avg_error_target": round_metric(avg_target),
                "avg_error_gap": avg_gap,
                "scaling_factor_error_boosted": round_metric(scaling_error),
                "scaling_factor_error_source": scaling_error_source,
                "scaling_factor_error_target": round_metric(MEAN_SCALING_FACTOR_ERROR_TARGET),
                "scaling_factor_error_gap": scaling_gap,
                "worst_gap": round_metric(worst_gap),
                "worst_gap_metric": worst_gap_metric,
            }
        )

    ranked.sort(key=lambda entry: (-entry["worst_gap"], entry["kernel_name"]))
    for rank, entry in enumerate(ranked, start=1):
        entry["rank"] = rank
    return ranked


def build_scorecard(summary: dict[str, Any], summary_path: Path) -> dict[str, Any]:
    validate_summary_shape(summary)
    aggregate_target_results = build_aggregate_target_results(summary)
    ranked_gaps = build_ranked_benchmark_gaps(summary)

    coverage_status = aggregate_target_results["coverage"]["status"]
    accuracy_result_names = ("tier1_avg_error_boosted", "tier2_avg_error_boosted")
    accuracy_status = (
        "pass"
        if all(aggregate_target_results[name]["status"] == "pass" for name in accuracy_result_names)
        else "fail"
    )
    trend_status = aggregate_target_results["mean_scaling_factor_error_boosted"]["status"]
    overall_status = (
        "pass"
        if coverage_status == "pass" and accuracy_status == "pass" and trend_status == "pass"
        else "fail"
    )

    point_totals = summary["point_coverage_status"]["totals"]
    return {
        "schema_name": SCORECARD_SCHEMA_NAME,
        "schema_version": SCORECARD_SCHEMA_VERSION,
        "target_policy": TARGET_POLICY,
        "source_path": repo_relative(summary_path),
        "source_schema_name": summary["schema_name"],
        "source_schema_version": summary["schema_version"],
        "scope": summary["scope"],
        "set_id": summary["set_id"],
        "set_version": summary["set_version"],
        "selected_count": summary.get("selected_count"),
        "tier_counts": summary.get("tier_counts", {}),
        "matched_points": point_totals["matched_points"],
        "hardware_points": point_totals["hardware_points"],
        "coverage_status": coverage_status,
        "accuracy_status": accuracy_status,
        "trend_status": trend_status,
        "overall_status": overall_status,
        "aggregate_target_results": aggregate_target_results,
        "ranked_benchmark_gaps": ranked_gaps,
    }


# ---------------------------------------------------------------------------
# Formatting and CLI
# ---------------------------------------------------------------------------


def fmt_pct(value: float | None) -> str:
    if value is None:
        return "n/a"
    return f"{value * 100:.1f}%"


def fmt_status(status: str) -> str:
    return status.upper()


def format_target_line(name: str, result: dict[str, Any]) -> str:
    if name == "coverage":
        return (
            f"  {fmt_status(result['status']):4} coverage: "
            f"{result['selected_with_passing_coverage']}/{result['selected_total']} selected compliant, "
            f"non-compliant={result['non_compliant']}, no-sim={result['no_sim_data']} "
            f"(target {result['target']})"
        )
    return (
        f"  {fmt_status(result['status']):4} {result['label']}: "
        f"{fmt_pct(result['actual'])} (target ≤ {fmt_pct(result['target'])}, "
        f"gap {fmt_pct(result['gap'])})"
    )


def format_text(scorecard: dict[str, Any], *, top: int, enforce: bool) -> str:
    lines = [
        "Curated MI300A accuracy scorecard",
        f"Source: {scorecard['source_path']}",
        f"Set: {scorecard['set_id']} ({scorecard['set_version']})",
        (
            f"Coverage: {fmt_status(scorecard['coverage_status'])} "
            f"({scorecard['matched_points']}/{scorecard['hardware_points']} matched points)"
        ),
        (
            f"Accuracy: {fmt_status(scorecard['accuracy_status'])}; "
            f"Trend/scaling: {fmt_status(scorecard['trend_status'])}; "
            f"Overall: {fmt_status(scorecard['overall_status'])}"
        ),
        "",
        "Aggregate target results:",
    ]
    for name in AGGREGATE_RESULT_ORDER:
        lines.append(format_target_line(name, scorecard["aggregate_target_results"][name]))

    ranked = scorecard["ranked_benchmark_gaps"]
    top = max(0, top)
    shown = ranked[:top]
    lines.extend(["", f"Top {len(shown)} benchmark target gaps:"])
    for entry in shown:
        lines.append(
            f"  {entry['rank']:>2}. {entry['kernel_name']} (Tier {entry['tier']}): "
            f"worst {entry['worst_gap_metric']} gap {fmt_pct(entry['worst_gap'])}; "
            f"avg gap {fmt_pct(entry['avg_error_gap'])}, "
            f"scaling gap {fmt_pct(entry['scaling_factor_error_gap'])}"
        )

    if scorecard["overall_status"] != "pass" and not enforce:
        lines.extend(
            [
                "",
                "Diagnostic mode: exiting 0 despite failed targets. Use --enforce to gate on this scorecard.",
            ]
        )
    elif scorecard["overall_status"] != "pass" and enforce:
        lines.extend(["", "Enforcement mode: failed targets will return a nonzero exit code."])
    else:
        lines.extend(["", "All curated coverage/accuracy/trend targets pass."])
    return "\n".join(lines) + "\n"


def dump_json(scorecard: dict[str, Any]) -> str:
    return json.dumps(scorecard, indent=2, sort_keys=True) + "\n"


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    if args.top < 0:
        print("ERROR: --top must be non-negative", file=sys.stderr)
        return 1

    summary_path = Path(args.summary)
    try:
        summary = load_summary(summary_path)
        scorecard = build_scorecard(summary, summary_path)
    except ScorecardError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1

    json_text = dump_json(scorecard)
    if args.json_output:
        output_path = Path(args.json_output)
        output_path.parent.mkdir(parents=True, exist_ok=True)
        output_path.write_text(json_text, encoding="utf-8")

    if args.format == "json":
        sys.stdout.write(json_text)
    else:
        sys.stdout.write(format_text(scorecard, top=args.top, enforce=args.enforce))

    if args.enforce and scorecard["overall_status"] != "pass":
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
