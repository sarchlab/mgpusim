#!/usr/bin/env python3
"""Materialize the maintained regular MI300A workflow matrices from the plan.

This helper is intentionally static and read-only. It consumes the checked-in
MI300A problem-size discovery plan and emits compact per-benchmark matrix rows
for the maintained `.github/workflows/benchmark.yml` regular workflow. It does
not contact GitHub, dispatch/cancel/rerun Actions, run simulators, collect
artifacts, or regenerate finishable/provenance/Tier outputs.
"""

from __future__ import annotations

import argparse
import collections
import json
import os
import re
import sys
from pathlib import Path
from typing import Any, Iterable, Mapping, Optional, Sequence


REPO_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_PLAN = REPO_ROOT / "benchmark-comparison" / "mi300a_problem_size_discovery_plan.json"
DEFAULT_SUMMARY = Path("regular_workflow_validation_summary.md")
DEFAULT_REPORT = Path("regular_matrix_validation.json")

PLAN_SCHEMA = "mgpusim.mi300a_problem_size_discovery_plan"
VALIDATION_SCHEMA = "mgpusim.mi300a_regular_workflow_matrix_validation"
SCHEMA_VERSION = 1
EXPECTED_ENTRY_COUNT = 1416
EXPECTED_WORKFLOW_JOBS = ("benchmark-tier1", "benchmark-tier2")
EXPECTED_TIER_BY_WORKFLOW_JOB = {"benchmark-tier1": 1, "benchmark-tier2": 2}
EXPECTED_MAX_PARALLEL = 14
EXPECTED_PER_SIMULATOR_TIMEOUT_SEC = 7200
EXPECTED_BENCHMARK_JOB_TIMEOUT_SEC = 21600
ARTIFACT_ID_RE = re.compile(r"^[A-Za-z0-9_.-]+$")
RUNTIME_BUDGET_ROW_FIELDS = (
    "expected_simulator_invocations",
    "planned_invocation_count",
    "per_invocation_timeout_sec",
    "row_timeout_budget_sec",
    "benchmark_job_timeout_sec",
    "row_timeout_budget_to_job_timeout_ratio",
    "exceeds_benchmark_job_timeout",
    "max_invocations_within_job_timeout",
)
RUNTIME_BUDGET_NOTICE = (
    "Runtime-budget/passability-risk visibility is informational only; it is not "
    "a hard pass/fail gate and is not a simulator outcome."
)


class MatrixError(Exception):
    """Raised when the checked-in MI300A regular matrix source is invalid."""


def repo_relative(path: Path) -> str:
    try:
        return path.resolve().relative_to(REPO_ROOT).as_posix()
    except ValueError:
        return path.as_posix()


def load_json_object(path: Path) -> Mapping[str, Any]:
    try:
        with path.open("r", encoding="utf-8") as f:
            data = json.load(f)
    except FileNotFoundError as exc:
        raise MatrixError(f"{repo_relative(path)}: file not found") from exc
    except json.JSONDecodeError as exc:
        raise MatrixError(f"{repo_relative(path)}: invalid JSON: {exc}") from exc
    if not isinstance(data, Mapping):
        raise MatrixError(f"{repo_relative(path)}: JSON root must be an object")
    return data


def nonempty_text(value: Any, label: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise MatrixError(f"{label} must be a non-empty string")
    return value


def integer(value: Any, label: str) -> int:
    if isinstance(value, bool):
        raise MatrixError(f"{label} must be an integer, got boolean")
    try:
        result = int(value)
    except (TypeError, ValueError) as exc:
        raise MatrixError(f"{label} must be an integer, got {value!r}") from exc
    return result


def as_list(value: Any, label: str) -> list[Any]:
    if not isinstance(value, list):
        raise MatrixError(f"{label} must be a list")
    return value


def make_ranges(values: Iterable[int]) -> list[str]:
    sorted_values = sorted(set(values))
    if not sorted_values:
        return []
    ranges: list[str] = []
    start = prev = sorted_values[0]
    for value in sorted_values[1:]:
        if value == prev + 1:
            prev = value
            continue
        ranges.append(str(start) if start == prev else f"{start}-{prev}")
        start = prev = value
    ranges.append(str(start) if start == prev else f"{start}-{prev}")
    return ranges


def safe_artifact_id(benchmark: str) -> str:
    artifact_id = benchmark.replace("_", "-")
    if not ARTIFACT_ID_RE.fullmatch(artifact_id):
        raise MatrixError(f"artifact id for benchmark {benchmark!r} is not artifact-safe: {artifact_id!r}")
    return artifact_id


def max_invocations_within_job_timeout() -> int:
    return EXPECTED_BENCHMARK_JOB_TIMEOUT_SEC // EXPECTED_PER_SIMULATOR_TIMEOUT_SEC


def runtime_budget_fields(invocation_count: int) -> dict[str, Any]:
    if invocation_count < 0:
        raise MatrixError(f"invocation_count must be non-negative, got {invocation_count}")
    row_timeout_budget_sec = invocation_count * EXPECTED_PER_SIMULATOR_TIMEOUT_SEC
    benchmark_job_timeout_sec = EXPECTED_BENCHMARK_JOB_TIMEOUT_SEC
    return {
        "expected_simulator_invocations": invocation_count,
        "planned_invocation_count": invocation_count,
        "per_invocation_timeout_sec": EXPECTED_PER_SIMULATOR_TIMEOUT_SEC,
        "row_timeout_budget_sec": row_timeout_budget_sec,
        "benchmark_job_timeout_sec": benchmark_job_timeout_sec,
        "row_timeout_budget_to_job_timeout_ratio": round(
            row_timeout_budget_sec / benchmark_job_timeout_sec,
            6,
        ),
        "exceeds_benchmark_job_timeout": row_timeout_budget_sec > benchmark_job_timeout_sec,
        "max_invocations_within_job_timeout": max_invocations_within_job_timeout(),
    }


def compact_row(row: Mapping[str, Any]) -> dict[str, Any]:
    benchmark = row.get("benchmark", "<unknown>")
    invocation_count = integer(
        row.get("expected_plan_entries"),
        f"{benchmark}.expected_plan_entries",
    )
    budget_fields = runtime_budget_fields(invocation_count)
    return {
        "benchmark": row["benchmark"],
        "artifact_id": row["artifact_id"],
        "plan_entries": invocation_count,
        "size_count": len(str(row["sizes"]).split()),
        "timeout_sec": integer(row["timeout_sec"], f"{row['benchmark']}.timeout_sec"),
        **budget_fields,
        "source_plan_index_ranges": list(row["source_plan_index_ranges"]),
    }


def runtime_budget_risk_summary(rows: Sequence[Mapping[str, Any]]) -> dict[str, Any]:
    invocation_counts = [
        integer(
            row.get("expected_simulator_invocations"),
            f"{row.get('benchmark', '<unknown>')}.expected_simulator_invocations",
        )
        for row in rows
    ]
    row_timeout_budgets = [count * EXPECTED_PER_SIMULATOR_TIMEOUT_SEC for count in invocation_counts]
    max_invocations = max_invocations_within_job_timeout()
    rows_exceeding_timeout = sum(
        budget > EXPECTED_BENCHMARK_JOB_TIMEOUT_SEC
        for budget in row_timeout_budgets
    )
    return {
        "per_invocation_timeout_sec": EXPECTED_PER_SIMULATOR_TIMEOUT_SEC,
        "benchmark_job_timeout_sec": EXPECTED_BENCHMARK_JOB_TIMEOUT_SEC,
        "max_invocations_within_job_timeout": max_invocations,
        "rows_exceeding_benchmark_job_timeout": rows_exceeding_timeout,
        "rows_within_benchmark_job_timeout": len(invocation_counts) - rows_exceeding_timeout,
        "planned_invocations": sum(invocation_counts),
        "planned_invocations_over_job_timeout_capacity": sum(
            max(0, count - max_invocations)
            for count in invocation_counts
        ),
        "max_planned_invocation_count": max(invocation_counts, default=0),
        "max_row_timeout_budget_sec": max(row_timeout_budgets, default=0),
        "max_row_timeout_budget_to_job_timeout_ratio": round(
            max(row_timeout_budgets, default=0) / EXPECTED_BENCHMARK_JOB_TIMEOUT_SEC,
            6,
        ),
        "informational_only": True,
    }


def load_plan_entries(plan_path: Path) -> tuple[Mapping[str, Any], list[Mapping[str, Any]]]:
    plan = load_json_object(plan_path)
    if plan.get("schema_name") != PLAN_SCHEMA:
        raise MatrixError(f"{repo_relative(plan_path)} schema_name must be {PLAN_SCHEMA!r}")
    entries = [
        entry
        for entry in as_list(plan.get("entries"), "plan.entries")
    ]
    if len(entries) != EXPECTED_ENTRY_COUNT:
        raise MatrixError(f"plan.entries must contain {EXPECTED_ENTRY_COUNT} entries, got {len(entries)}")
    if integer(plan.get("entry_count"), "plan.entry_count") != EXPECTED_ENTRY_COUNT:
        raise MatrixError(f"plan.entry_count must be {EXPECTED_ENTRY_COUNT}")
    # The discovery plan is an archival coverage source; its historical timeout
    # metadata is not the maintained regular runtime contract. The materialized
    # matrix rows below normalize to EXPECTED_PER_SIMULATOR_TIMEOUT_SEC.
    if "timeout_sec" in plan:
        integer(plan.get("timeout_sec"), "plan.timeout_sec")

    seen_indices: set[int] = set()
    for offset, raw_entry in enumerate(entries):
        if not isinstance(raw_entry, Mapping):
            raise MatrixError(f"plan.entries[{offset}] must be an object")
        entry = raw_entry
        plan_index = integer(entry.get("plan_index"), f"plan.entries[{offset}].plan_index")
        if plan_index in seen_indices:
            raise MatrixError(f"plan.entries duplicate plan_index {plan_index}")
        seen_indices.add(plan_index)
        for key in (
            "workflow_job",
            "workflow_benchmark",
            "workflow_benchmark_name",
            "benchmark",
            "size_token",
            "flag_template",
            "size_label_template",
            "tier_name",
        ):
            nonempty_text(entry.get(key), f"plan.entries[{offset}].{key}")
        workflow_job = str(entry.get("workflow_job"))
        if workflow_job not in EXPECTED_WORKFLOW_JOBS:
            raise MatrixError(
                f"plan.entries[{offset}].workflow_job must be one of {EXPECTED_WORKFLOW_JOBS}, "
                f"got {entry.get('workflow_job')!r}"
            )
        if entry.get("workflow_benchmark") != entry.get("workflow_benchmark_name"):
            raise MatrixError(
                f"plan.entries[{offset}].workflow_benchmark must match workflow_benchmark_name"
            )
        expected_tier = EXPECTED_TIER_BY_WORKFLOW_JOB[workflow_job]
        if integer(entry.get("tier"), f"plan.entries[{offset}].tier") != expected_tier:
            raise MatrixError(f"plan.entries[{offset}].tier must be {expected_tier} for {workflow_job}")
        if entry.get("tier_name") != f"tier{expected_tier}":
            raise MatrixError(f"plan.entries[{offset}].tier_name must be 'tier{expected_tier}'")
        if "timeout_sec" in entry:
            integer(entry.get("timeout_sec"), f"plan.entries[{offset}].timeout_sec")
        benchmark_dir = REPO_ROOT / "amd" / "samples" / str(entry.get("workflow_benchmark_name"))
        if not benchmark_dir.is_dir():
            raise MatrixError(f"plan.entries[{offset}] has no {repo_relative(benchmark_dir)} directory")

    expected_indices = set(range(1, EXPECTED_ENTRY_COUNT + 1))
    if seen_indices != expected_indices:
        raise MatrixError("plan.entries must cover plan_index 1..1416 exactly once")
    return plan, entries


def group_entries(entries: Sequence[Mapping[str, Any]]) -> dict[str, list[dict[str, Any]]]:
    grouped: dict[tuple[str, str], list[Mapping[str, Any]]] = collections.OrderedDict()
    for entry in entries:
        key = (str(entry["workflow_job"]), str(entry["workflow_benchmark_name"]))
        grouped.setdefault(key, []).append(entry)

    by_job: dict[str, list[dict[str, Any]]] = {job: [] for job in EXPECTED_WORKFLOW_JOBS}
    artifact_ids: set[str] = set()
    for (workflow_job, benchmark), group in grouped.items():
        first = group[0]
        for field in (
            "workflow_benchmark",
            "tier",
            "tier_name",
            "flag_template",
            "size_label_template",
            "extra_flags",
            "gomemlimit",
            "gogc",
        ):
            values = {entry.get(field) for entry in group}
            if len(values) != 1:
                raise MatrixError(f"{workflow_job}/{benchmark} has inconsistent {field} values")
        artifact_id = safe_artifact_id(benchmark)
        if artifact_id in artifact_ids:
            raise MatrixError(f"duplicate matrix artifact_id {artifact_id}")
        artifact_ids.add(artifact_id)
        source_indices = [integer(entry["plan_index"], "plan_index") for entry in group]
        if source_indices != sorted(source_indices):
            raise MatrixError(f"{workflow_job}/{benchmark} source plan indices are not ordered")
        sizes = [str(entry["size_token"]) for entry in group]
        invocation_count = len(group)
        row: dict[str, Any] = {
            "benchmark": benchmark,
            "sizes": " ".join(sizes),
            "flag_template": str(first["flag_template"]),
            "size_label_template": str(first["size_label_template"]),
            "timeout_sec": EXPECTED_PER_SIMULATOR_TIMEOUT_SEC,
            "artifact_id": artifact_id,
            "expected_plan_entries": invocation_count,
            **runtime_budget_fields(invocation_count),
            "source_plan_index_ranges": make_ranges(source_indices),
        }
        for optional_key in ("extra_flags", "gomemlimit", "gogc"):
            value = first.get(optional_key)
            if value is not None:
                row[optional_key] = value
        by_job[workflow_job].append(row)
    return by_job


def build_report(
    plan_path: Path,
    plan: Mapping[str, Any],
    by_job: Mapping[str, Sequence[Mapping[str, Any]]],
) -> dict[str, Any]:
    job_summaries: dict[str, dict[str, Any]] = {}
    row_details: dict[str, list[dict[str, Any]]] = {}
    total_rows = 0
    total_entries = 0
    for job in EXPECTED_WORKFLOW_JOBS:
        rows = list(by_job[job])
        row_count = len(rows)
        entry_count = sum(integer(row.get("expected_plan_entries"), f"{job}.expected_plan_entries") for row in rows)
        total_rows += row_count
        total_entries += entry_count
        row_details[job] = [compact_row(row) for row in rows]
        job_summaries[job] = {
            "matrix_rows": row_count,
            "plan_entries": entry_count,
            "max_plan_entries_per_row": max(
                (
                    integer(row.get("expected_plan_entries"), "expected_plan_entries")
                    for row in rows
                ),
                default=0,
            ),
            "passability_risk": runtime_budget_risk_summary(rows),
            "source_plan_index_ranges": make_ranges(
                index
                for row in rows
                for item in row.get("source_plan_index_ranges", [])
                for index in _expand_range(str(item))
            ),
        }
    if total_entries != EXPECTED_ENTRY_COUNT:
        raise MatrixError(f"materialized matrix covers {total_entries} plan entries, expected {EXPECTED_ENTRY_COUNT}")
    return {
        "schema_name": VALIDATION_SCHEMA,
        "schema_version": SCHEMA_VERSION,
        "status": "pass",
        "runtime_budget_notice": RUNTIME_BUDGET_NOTICE,
        "source_plan": repo_relative(plan_path),
        "plan_schema_name": plan.get("schema_name"),
        "plan_entry_count": EXPECTED_ENTRY_COUNT,
        "workflow_jobs": list(EXPECTED_WORKFLOW_JOBS),
        "regular_runtime_contract": {
            "strategy_max_parallel": EXPECTED_MAX_PARALLEL,
            "per_simulator_timeout_sec": EXPECTED_PER_SIMULATOR_TIMEOUT_SEC,
            "benchmark_job_timeout_sec": EXPECTED_BENCHMARK_JOB_TIMEOUT_SEC,
            "benchmark_job_timeout_minutes": EXPECTED_BENCHMARK_JOB_TIMEOUT_SEC // 60,
            "max_invocations_within_job_timeout": max_invocations_within_job_timeout(),
            "row_budget_risk_is_informational": True,
        },
        "matrix": {
            "total_matrix_rows": total_rows,
            "total_plan_entries": total_entries,
            "per_benchmark_grouping": True,
            "jobs": job_summaries,
            "rows": row_details,
        },
        "non_goals": [
            "no terminal-discovery mode or workflow surface",
            "no repository_dispatch or workflow_call surface",
            "no workflow dispatch/cancel/rerun",
            "no artifact/log collection",
            "no simulator execution, long simulation, or calibration",
            "no finishable/provenance/Tier regeneration",
            "no terminal finishability completion claim",
        ],
    }


def _expand_range(item: str) -> list[int]:
    if "-" not in item:
        return [int(item)]
    start, end = item.split("-", 1)
    return list(range(int(start), int(end) + 1))


def write_summary(path: Path, report: Mapping[str, Any]) -> None:
    jobs = report["matrix"]["jobs"]  # type: ignore[index]
    lines = [
        "# MI300A Regular Workflow Matrix Validation",
        "",
        f"Plan: `{report['source_plan']}`",
        "",
        (
            "Regular runtime contract: benchmark-tier matrices run with "
            f"`max-parallel: {EXPECTED_MAX_PARALLEL}`; each individual simulator "
            f"invocation is capped at `{EXPECTED_PER_SIMULATOR_TIMEOUT_SEC}` "
            "seconds; each benchmark-tier job is capped at "
            f"`timeout-minutes: {EXPECTED_BENCHMARK_JOB_TIMEOUT_SEC // 60}` "
            f"({EXPECTED_BENCHMARK_JOB_TIMEOUT_SEC} seconds)."
        ),
        "",
        (
            f"| Workflow job | Matrix rows | Plan entries | Rows over "
            f"{EXPECTED_BENCHMARK_JOB_TIMEOUT_SEC // 3600}h budget | "
            "Max invocations/row | Max row budget | Source plan index ranges |"
        ),
        "|---|---:|---:|---:|---:|---:|---|",
    ]
    for job in EXPECTED_WORKFLOW_JOBS:
        summary = jobs[job]
        risk = summary["passability_risk"]
        ranges = ", ".join(summary["source_plan_index_ranges"])
        lines.append(
            f"| {job} | {summary['matrix_rows']} | {summary['plan_entries']} | "
            f"{risk['rows_exceeding_benchmark_job_timeout']} | "
            f"{risk['max_planned_invocation_count']} | "
            f"{risk['max_row_timeout_budget_sec']}s | `{ranges}` |"
        )
    lines.extend(
        [
            "",
            f"Total matrix rows: `{report['matrix']['total_matrix_rows']}`.",  # type: ignore[index]
            f"Total covered plan entries: `{report['matrix']['total_plan_entries']}`.",  # type: ignore[index]
            "",
            RUNTIME_BUDGET_NOTICE,
            "",
            "This validation is static/read-only and does not claim terminal finishability completion.",
        ]
    )
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def write_github_outputs(
    path: Optional[Path],
    by_job: Mapping[str, Sequence[Mapping[str, Any]]],
    report: Mapping[str, Any],
) -> None:
    outputs = {
        "tier1_matrix": json.dumps({"include": list(by_job["benchmark-tier1"])}, separators=(",", ":")),
        "tier2_matrix": json.dumps({"include": list(by_job["benchmark-tier2"])}, separators=(",", ":")),
        "tier1_count": str(report["matrix"]["jobs"]["benchmark-tier1"]["matrix_rows"]),  # type: ignore[index]
        "tier2_count": str(report["matrix"]["jobs"]["benchmark-tier2"]["matrix_rows"]),  # type: ignore[index]
        "tier1_sizes": str(report["matrix"]["jobs"]["benchmark-tier1"]["plan_entries"]),  # type: ignore[index]
        "tier2_sizes": str(report["matrix"]["jobs"]["benchmark-tier2"]["plan_entries"]),  # type: ignore[index]
        "regular_matrix_rows": str(report["matrix"]["total_matrix_rows"]),  # type: ignore[index]
        "regular_plan_entries": str(report["matrix"]["total_plan_entries"]),  # type: ignore[index]
    }
    if path is None:
        return
    with path.open("a", encoding="utf-8") as f:
        for key, value in outputs.items():
            if "\n" in value:
                raise MatrixError(f"GitHub output {key} unexpectedly contains a newline")
            f.write(f"{key}={value}\n")


def materialize(
    *,
    plan_path: Path,
    summary_path: Path,
    report_path: Path,
    github_output_path: Optional[Path],
) -> dict[str, Any]:
    plan, entries = load_plan_entries(plan_path)
    by_job = group_entries(entries)
    report = build_report(plan_path, plan, by_job)
    report_path.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    write_summary(summary_path, report)
    write_github_outputs(github_output_path, by_job, report)
    return report


def parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--plan", default=str(DEFAULT_PLAN), help="Path to mi300a_problem_size_discovery_plan.json")
    parser.add_argument("--summary", default=str(DEFAULT_SUMMARY), help="Path to write the Markdown validation summary")
    parser.add_argument("--report", default=str(DEFAULT_REPORT), help="Path to write the JSON validation report")
    parser.add_argument(
        "--github-output",
        default=os.environ.get("GITHUB_OUTPUT"),
        help="Optional GitHub output file to append matrix outputs to",
    )
    return parser.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    args = parse_args(argv)
    github_output_path = Path(args.github_output) if args.github_output else None
    try:
        report = materialize(
            plan_path=Path(args.plan),
            summary_path=Path(args.summary),
            report_path=Path(args.report),
            github_output_path=github_output_path,
        )
    except MatrixError as exc:
        print("FAIL: regular MI300A workflow matrix validation failed.", file=sys.stderr)
        print(f"  - {exc}", file=sys.stderr)
        return 1
    print(json.dumps(report, indent=2, sort_keys=True) + "\n", end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
