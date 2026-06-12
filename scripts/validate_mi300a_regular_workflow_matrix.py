#!/usr/bin/env python3
"""Validate the maintained full regular MI300A benchmark workflow matrix.

This command is deterministic, static, and read-only. It consumes the checked-in
1416-entry MI300A problem-size discovery plan plus the maintained
`.github/workflows/benchmark.yml` workflow and verifies that the regular runtime
matrix is derived from the plan, grouped by benchmark, and bounded by the
14-way / 7200-second / 21600-second regular workflow contract. It does not contact
GitHub, dispatch/cancel/rerun Actions, inspect logs, download/collect artifacts,
run simulators, calibrate models, or regenerate finishable/provenance/Tier
outputs.
"""

from __future__ import annotations

import argparse
import collections
import json
import re
import sys
from pathlib import Path
from typing import Any, Mapping, Optional, Sequence

import yaml

import materialize_mi300a_regular_workflow_matrix as regular_matrix


VALIDATION_SCHEMA = "mgpusim.mi300a_regular_workflow_matrix_contract_validation"
SCHEMA_VERSION = 1

REPO_ROOT = regular_matrix.REPO_ROOT
DEFAULT_PLAN = regular_matrix.DEFAULT_PLAN
DEFAULT_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "benchmark.yml"

EXPECTED_JOBS = ["validation", "benchmark-tier1", "tier1-summary", "benchmark-tier2", "summary"]
EXPECTED_JOB_NEEDS = {
    "validation": [],
    "benchmark-tier1": ["validation"],
    "tier1-summary": ["validation", "benchmark-tier1"],
    "benchmark-tier2": ["validation", "tier1-summary"],
    "summary": ["validation", "benchmark-tier1", "tier1-summary", "benchmark-tier2"],
}
EXPECTED_WORKFLOW_INPUTS = ["ref"]
EXPECTED_WORKFLOW_EVENTS = ["workflow_dispatch"]
EXPECTED_WORKFLOW_FILES = ["benchmark.yml", "compile_hsaco.yml", "mgpusim_test.yml"]
EXPECTED_TIER_MATRIX_COUNTS = {
    "benchmark-tier1": {"matrix_rows": 19, "plan_entries": 308, "source_plan_index_ranges": ["1-308"]},
    "benchmark-tier2": {"matrix_rows": 63, "plan_entries": 1108, "source_plan_index_ranges": ["309-1416"]},
}
FORBIDDEN_WORKFLOW_SNIPPETS = [
    "mi300a_terminal_discovery.yml",
    "MI300A Terminal Discovery",
    "validate-terminal-discovery",
    "terminal-discovery-shard",
    "collect-terminal-discovery",
    "mi300a_terminal_discovery_shard_manifest.json",
    "mi300a_terminal_discovery_shard_01_matrix.json",
    "validate_mi300a_terminal_discovery_shards.py",
    "collect_mi300a_terminal_discovery_provenance.py",
    "mi300a-terminal-discovery-plan-",
    "github.event.inputs.run_mode",
    "run_mode:",
    "repository_dispatch:",
    "workflow_call:",
]
LEGACY_MATRIX_SNIPPETS = [
    "benchmark-comparison/generated/mi300a_regular_tier1_matrix.json",
    "benchmark-comparison/generated/mi300a_finishable_tier1_matrix.json",
    "benchmark-comparison/generated/mi300a_finishable_tier2_matrix.json",
]
STALE_REGULAR_CONTRACT_SNIPPETS = [
    "MATRIX_TIMEOUT_SEC: ${{ matrix.timeout_sec || '3600' }}",
    '"timeout_sec": env("MATRIX_TIMEOUT_SEC", "3600")',
    'int(metadata["timeout_sec"]) != 3600',
    "timeout_sec must be 3600s for the regular MI300A contract",
    "timeout-minutes: 720",
    "720-min job budget",
]


class ValidationError(Exception):
    """Raised when the maintained regular workflow matrix contract is invalid."""


def repo_relative(path: Path) -> str:
    return regular_matrix.repo_relative(path)


def integer(value: Any, label: str) -> int:
    try:
        return regular_matrix.integer(value, label)
    except regular_matrix.MatrixError as exc:
        raise ValidationError(str(exc)) from exc


def load_workflow_text(workflow_path: Path) -> str:
    try:
        return workflow_path.read_text(encoding="utf-8")
    except FileNotFoundError as exc:
        raise ValidationError(f"{repo_relative(workflow_path)}: file not found") from exc


def load_workflow_yaml(workflow_path: Path) -> dict[str, Any]:
    """Parse the workflow file as YAML for structural field checks.

    Comments and stray substrings are ignored; only the effective key/value
    structure is returned. Raises ValidationError if the file does not parse
    into a mapping.
    """
    text = load_workflow_text(workflow_path)
    try:
        loaded = yaml.safe_load(text)
    except yaml.YAMLError as exc:
        raise ValidationError(
            f"{repo_relative(workflow_path)}: failed to parse YAML ({exc})"
        ) from exc
    if not isinstance(loaded, dict):
        raise ValidationError(
            f"{repo_relative(workflow_path)}: top-level YAML must be a mapping"
        )
    return loaded


def require_job_timeout_minutes(
    workflow_yaml: Mapping[str, Any], job_name: str, expected: int
) -> None:
    """Structurally enforce jobs.<job>.timeout-minutes == expected."""
    jobs = workflow_yaml.get("jobs")
    if not isinstance(jobs, Mapping) or job_name not in jobs:
        raise ValidationError(
            f"benchmark workflow YAML missing job {job_name!r} for "
            f"timeout-minutes: {expected} structural check"
        )
    job_block = jobs[job_name]
    if not isinstance(job_block, Mapping):
        raise ValidationError(
            f"{job_name}: YAML job block must be a mapping for "
            f"timeout-minutes: {expected} structural check"
        )
    actual = job_block.get("timeout-minutes")
    if actual != expected:
        raise ValidationError(
            f"{job_name} must declare 'timeout-minutes: {expected}' "
            f"(got timeout-minutes: {actual!r})"
        )


def require_job_max_parallel(
    workflow_yaml: Mapping[str, Any], job_name: str, expected: int
) -> None:
    """Structurally enforce jobs.<job>.strategy.max-parallel == expected."""
    jobs = workflow_yaml.get("jobs")
    if not isinstance(jobs, Mapping) or job_name not in jobs:
        raise ValidationError(
            f"benchmark workflow YAML missing job {job_name!r} for "
            f"max-parallel: {expected} structural check"
        )
    job_block = jobs[job_name]
    if not isinstance(job_block, Mapping):
        raise ValidationError(
            f"{job_name}: YAML job block must be a mapping for "
            f"max-parallel: {expected} structural check"
        )
    strategy = job_block.get("strategy")
    if not isinstance(strategy, Mapping):
        raise ValidationError(
            f"{job_name} strategy block missing for "
            f"'max-parallel: {expected}' structural check"
        )
    actual = strategy.get("max-parallel")
    if actual != expected:
        raise ValidationError(
            f"{job_name} strategy must declare 'max-parallel: {expected}' "
            f"(got max-parallel: {actual!r})"
        )


def extract_job_order(workflow_text: str) -> list[str]:
    try:
        jobs_text = workflow_text.split("\njobs:\n", 1)[1]
    except IndexError as exc:
        raise ValidationError("benchmark workflow must contain a top-level jobs block") from exc
    return re.findall(r"^  ([A-Za-z0-9_-]+):\n", jobs_text, flags=re.MULTILINE)


def extract_job(workflow_text: str, job_name: str) -> str:
    pattern = re.compile(
        rf"^  {re.escape(job_name)}:\n(?P<body>.*?)(?=^  [A-Za-z0-9_-]+:|\Z)",
        flags=re.MULTILINE | re.DOTALL,
    )
    match = pattern.search(workflow_text)
    if not match:
        raise ValidationError(f"benchmark workflow missing job {job_name!r}")
    return match.group(0)


def extract_step(job_text: str, step_name: str) -> str:
    pattern = re.compile(
        rf"^      - name: {re.escape(step_name)}\n(?P<body>.*?)(?=^      - name: |\Z)",
        flags=re.MULTILINE | re.DOTALL,
    )
    match = pattern.search(job_text)
    if not match:
        raise ValidationError(f"workflow job missing step {step_name!r}")
    return match.group(0)


def workflow_dispatch_inputs(workflow_text: str) -> list[str]:
    pattern = re.compile(r"^  workflow_dispatch:\n(?P<body>.*?)(?=^jobs:)", flags=re.MULTILINE | re.DOTALL)
    match = pattern.search(workflow_text)
    if not match:
        raise ValidationError("benchmark workflow must expose workflow_dispatch")
    block = match.group("body")
    inputs_match = re.search(r"^    inputs:\n(?P<inputs>.*?)(?=^\S|^    [A-Za-z0-9_-]+:|\Z)", block, flags=re.MULTILINE | re.DOTALL)
    if not inputs_match:
        raise ValidationError("workflow_dispatch must define inputs")
    return re.findall(r"^      ([A-Za-z0-9_-]+):\n", inputs_match.group("inputs"), flags=re.MULTILINE)


def workflow_events(workflow_text: str) -> list[str]:
    pattern = re.compile(r"^on:\n(?P<body>.*?)(?=^jobs:)", flags=re.MULTILINE | re.DOTALL)
    match = pattern.search(workflow_text)
    if not match:
        raise ValidationError("benchmark workflow must contain a top-level on block")
    events = re.findall(r"^  ([A-Za-z0-9_-]+):(?:\n|$)", match.group("body"), flags=re.MULTILINE)
    if not events:
        raise ValidationError("benchmark workflow on block must declare events")
    return events


def job_needs(job_text: str) -> list[str]:
    match = re.search(r"^    needs: \[(?P<needs>[^\]]+)\]$", job_text, flags=re.MULTILINE)
    if not match:
        return []
    return [need.strip() for need in match.group("needs").split(",") if need.strip()]


def require_snippets(label: str, text: str, snippets: Sequence[str]) -> None:
    for snippet in snippets:
        if snippet not in text:
            raise ValidationError(f"{label} missing required snippet: {snippet!r}")


def forbid_snippets(label: str, text: str, snippets: Sequence[str]) -> None:
    for snippet in snippets:
        if snippet in text:
            raise ValidationError(f"{label} contains forbidden snippet: {snippet!r}")


def validate_workflow(workflow_path: Path) -> dict[str, Any]:
    workflow_text = load_workflow_text(workflow_path)
    workflow_yaml = load_workflow_yaml(workflow_path)
    job_order = extract_job_order(workflow_text)
    if job_order != EXPECTED_JOBS:
        raise ValidationError(f"benchmark workflow jobs must be {EXPECTED_JOBS!r}, got {job_order!r}")

    events = workflow_events(workflow_text)
    if events != EXPECTED_WORKFLOW_EVENTS:
        raise ValidationError(f"benchmark workflow events must be {EXPECTED_WORKFLOW_EVENTS!r}, got {events!r}")

    inputs = workflow_dispatch_inputs(workflow_text)
    if inputs != EXPECTED_WORKFLOW_INPUTS:
        raise ValidationError(f"workflow_dispatch inputs must be {EXPECTED_WORKFLOW_INPUTS!r}, got {inputs!r}")

    forbid_snippets("benchmark workflow", workflow_text, FORBIDDEN_WORKFLOW_SNIPPETS)
    forbid_snippets("benchmark workflow", workflow_text, LEGACY_MATRIX_SNIPPETS)
    forbid_snippets("benchmark workflow", workflow_text, STALE_REGULAR_CONTRACT_SNIPPETS)

    jobs = {job: extract_job(workflow_text, job) for job in EXPECTED_JOBS}
    for job, expected_needs in EXPECTED_JOB_NEEDS.items():
        actual_needs = job_needs(jobs[job])
        if actual_needs != expected_needs:
            raise ValidationError(f"{job}.needs must be {expected_needs!r}, got {actual_needs!r}")

    validation_job = jobs["validation"]
    materialize_step = extract_step(validation_job, "Materialize full regular matrix from plan")
    require_snippets(
        "full regular matrix materialization step",
        materialize_step,
        [
            "scripts/materialize_mi300a_regular_workflow_matrix.py",
            "--summary regular_workflow_validation_summary.md",
            "--report regular_matrix_validation.json",
        ],
    )
    require_snippets(
        "validation job outputs",
        validation_job,
        [
            "tier1_matrix: ${{ steps.matrix-outputs.outputs.tier1_matrix }}",
            "tier2_matrix: ${{ steps.matrix-outputs.outputs.tier2_matrix }}",
            "tier1_sizes: ${{ steps.matrix-outputs.outputs.tier1_sizes }}",
            "tier2_sizes: ${{ steps.matrix-outputs.outputs.tier2_sizes }}",
        ],
    )

    expected_timeout_minutes = regular_matrix.EXPECTED_BENCHMARK_JOB_TIMEOUT_SEC // 60
    expected_max_parallel = regular_matrix.EXPECTED_MAX_PARALLEL
    tier_contract: dict[str, dict[str, Any]] = {}
    for job in ("benchmark-tier1", "benchmark-tier2"):
        job_text = jobs[job]
        require_job_timeout_minutes(workflow_yaml, job, expected_timeout_minutes)
        require_job_max_parallel(workflow_yaml, job, expected_max_parallel)
        tier_index = 1 if job == "benchmark-tier1" else 2
        require_snippets(
            job,
            job_text,
            [
                f"matrix: ${{{{ fromJson(needs.validation.outputs.tier{tier_index}_matrix) }}}}",
            ],
        )
        tier_contract[job] = {
            "timeout_minutes": expected_timeout_minutes,
            "timeout_sec": regular_matrix.EXPECTED_BENCHMARK_JOB_TIMEOUT_SEC,
            "strategy_max_parallel": expected_max_parallel,
        }

    require_snippets(
        "benchmark-tier2 full-coverage guard",
        jobs["benchmark-tier2"],
        ["if: ${{ always() && needs.validation.result == 'success' }}"],
    )

    tier1_job = jobs["benchmark-tier1"]
    metadata_step = extract_step(tier1_job, "Resolve benchmark metadata")
    run_step = extract_step(tier1_job, "Run benchmark sizes")
    expected_per_simulator_timeout = regular_matrix.EXPECTED_PER_SIMULATOR_TIMEOUT_SEC
    require_snippets(
        "benchmark metadata step",
        metadata_step,
        [
            f"MATRIX_TIMEOUT_SEC: ${{{{ matrix.timeout_sec || '{expected_per_simulator_timeout}' }}}}",
            f'"timeout_sec": env("MATRIX_TIMEOUT_SEC", "{expected_per_simulator_timeout}")',
            f'int(metadata["timeout_sec"]) != {expected_per_simulator_timeout}',
            f"timeout_sec must be {expected_per_simulator_timeout}s for the regular MI300A contract",
        ],
    )
    require_snippets("benchmark run step", run_step, ['TIMEOUT_SEC="${RESOLVED_TIMEOUT_SEC}"', 'timeout "${TIMEOUT_SEC}"'])

    return {
        "workflow_path": repo_relative(workflow_path),
        "workflow_events": events,
        "workflow_dispatch_inputs": inputs,
        "jobs": job_order,
        "job_needs": EXPECTED_JOB_NEEDS,
        "benchmark_tier_contract": tier_contract,
        "tier2_runs_after_validation_for_full_coverage": True,
        "materializer": "scripts/materialize_mi300a_regular_workflow_matrix.py",
        "forbidden_surface_absent": True,
        "legacy_generated_matrix_sources_absent": True,
    }


def runtime_budget_fields(invocation_count: int) -> dict[str, Any]:
    try:
        return regular_matrix.runtime_budget_fields(invocation_count)
    except regular_matrix.MatrixError as exc:
        raise ValidationError(str(exc)) from exc


def require_runtime_budget_fields(
    row: Mapping[str, Any],
    invocation_count: int,
    label: str,
) -> dict[str, Any]:
    expected = runtime_budget_fields(invocation_count)
    for field in regular_matrix.RUNTIME_BUDGET_ROW_FIELDS:
        if row.get(field) != expected[field]:
            raise ValidationError(
                f"{label}.{field} must be {expected[field]!r}, got {row.get(field)!r}"
            )
    return expected


def compact_row(row: Mapping[str, Any]) -> dict[str, Any]:
    sizes = str(row["sizes"]).split()
    plan_entries = integer(
        row["expected_plan_entries"],
        f"{row['benchmark']}.expected_plan_entries",
    )
    if len(sizes) != plan_entries:
        raise ValidationError(f"{row['benchmark']}.sizes token count must match expected_plan_entries")
    budget_fields = require_runtime_budget_fields(row, plan_entries, str(row["benchmark"]))
    return {
        "benchmark": row["benchmark"],
        "artifact_id": row["artifact_id"],
        "plan_entries": plan_entries,
        "size_count": len(sizes),
        "timeout_sec": integer(row["timeout_sec"], f"{row['benchmark']}.timeout_sec"),
        **budget_fields,
        "source_plan_index_ranges": list(row["source_plan_index_ranges"]),
    }


def validate_plan_matrix(plan_path: Path) -> tuple[dict[str, Any], Mapping[str, Sequence[Mapping[str, Any]]]]:
    try:
        plan, entries = regular_matrix.load_plan_entries(plan_path)
        by_job = regular_matrix.group_entries(entries)
        materializer_report = regular_matrix.build_report(plan_path, plan, by_job)
    except regular_matrix.MatrixError as exc:
        raise ValidationError(str(exc)) from exc

    if plan.get("schema_version") != regular_matrix.SCHEMA_VERSION:
        raise ValidationError(f"plan.schema_version must be {regular_matrix.SCHEMA_VERSION}")
    # Source plan timeout metadata is archival; the materializer-owned row
    # timeout fields below define the maintained regular runtime contract.
    if "timeout_sec" in plan:
        integer(plan.get("timeout_sec"), "plan.timeout_sec")
    if plan.get("workflow_jobs") != list(regular_matrix.EXPECTED_WORKFLOW_JOBS):
        raise ValidationError(
            f"plan.workflow_jobs must be {list(regular_matrix.EXPECTED_WORKFLOW_JOBS)!r}, got {plan.get('workflow_jobs')!r}"
        )
    if integer(plan.get("workflow_benchmark_count"), "plan.workflow_benchmark_count") != 82:
        raise ValidationError("plan.workflow_benchmark_count must be 82")
    if plan.get("tier_counts") != {"1": 308, "2": 1108}:
        raise ValidationError("plan.tier_counts must be {'1': 308, '2': 1108}")

    rows_by_benchmark: dict[str, str] = {}
    artifact_ids: set[str] = set()
    coverage_indices: set[int] = set()
    row_details: dict[str, list[dict[str, Any]]] = {}
    for job in regular_matrix.EXPECTED_WORKFLOW_JOBS:
        rows = list(by_job[job])
        expected = EXPECTED_TIER_MATRIX_COUNTS[job]
        if len(rows) != expected["matrix_rows"]:
            raise ValidationError(f"{job} must have {expected['matrix_rows']} per-benchmark rows, got {len(rows)}")
        if sum(integer(row["expected_plan_entries"], f"{job}.{row['benchmark']}.expected_plan_entries") for row in rows) != expected["plan_entries"]:
            raise ValidationError(f"{job} must cover {expected['plan_entries']} plan entries")
        details: list[dict[str, Any]] = []
        for row in rows:
            benchmark = str(row["benchmark"])
            if benchmark in rows_by_benchmark:
                raise ValidationError(f"benchmark {benchmark!r} appears in both {rows_by_benchmark[benchmark]} and {job}")
            rows_by_benchmark[benchmark] = job
            artifact_id = str(row["artifact_id"])
            if artifact_id in artifact_ids:
                raise ValidationError(f"duplicate artifact id {artifact_id!r}")
            artifact_ids.add(artifact_id)
            if integer(row["timeout_sec"], f"{job}.{benchmark}.timeout_sec") != regular_matrix.EXPECTED_PER_SIMULATOR_TIMEOUT_SEC:
                raise ValidationError(
                    f"{job}/{benchmark} timeout_sec must be "
                    f"{regular_matrix.EXPECTED_PER_SIMULATOR_TIMEOUT_SEC}"
                )
            for item in row["source_plan_index_ranges"]:
                coverage_indices.update(regular_matrix._expand_range(str(item)))
            details.append(compact_row(row))
        row_details[job] = details
        try:
            expected_risk = regular_matrix.runtime_budget_risk_summary(rows)
        except regular_matrix.MatrixError as exc:
            raise ValidationError(str(exc)) from exc
        materializer_risk = materializer_report["matrix"]["jobs"][job].get("passability_risk")
        if materializer_risk != expected_risk:
            raise ValidationError(
                f"materializer report {job}.passability_risk must be {expected_risk!r}, "
                f"got {materializer_risk!r}"
            )

    expected_indices = set(range(1, regular_matrix.EXPECTED_ENTRY_COUNT + 1))
    if coverage_indices != expected_indices:
        missing = sorted(expected_indices - coverage_indices)[:10]
        extra = sorted(coverage_indices - expected_indices)[:10]
        raise ValidationError(f"materialized rows must cover plan_index 1..1416 exactly; missing={missing} extra={extra}")

    materializer_jobs = materializer_report["matrix"]["jobs"]
    for job, expected in EXPECTED_TIER_MATRIX_COUNTS.items():
        job_summary = materializer_jobs[job]
        for field, expected_value in expected.items():
            if job_summary[field] != expected_value:
                raise ValidationError(f"materializer report {job}.{field} must be {expected_value!r}")

    return {
        "source_plan": repo_relative(plan_path),
        "plan_schema_name": plan.get("schema_name"),
        "plan_schema_version": plan.get("schema_version"),
        "plan_entry_count": regular_matrix.EXPECTED_ENTRY_COUNT,
        "workflow_benchmark_count": 82,
        "workflow_jobs": list(regular_matrix.EXPECTED_WORKFLOW_JOBS),
        "tier_counts": {"benchmark-tier1": 308, "benchmark-tier2": 1108},
        "matrix": {
            "total_matrix_rows": 82,
            "total_plan_entries": regular_matrix.EXPECTED_ENTRY_COUNT,
            "per_benchmark_grouping": True,
            "unique_benchmark_rows": len(rows_by_benchmark),
            "unique_artifact_ids": len(artifact_ids),
            "jobs": materializer_jobs,
            "rows": row_details,
        },
        "regular_runtime_contract": {
            "strategy_max_parallel": regular_matrix.EXPECTED_MAX_PARALLEL,
            "per_simulator_timeout_sec": regular_matrix.EXPECTED_PER_SIMULATOR_TIMEOUT_SEC,
            "benchmark_job_timeout_sec": regular_matrix.EXPECTED_BENCHMARK_JOB_TIMEOUT_SEC,
            "benchmark_job_timeout_minutes": regular_matrix.EXPECTED_BENCHMARK_JOB_TIMEOUT_SEC // 60,
            "max_invocations_within_job_timeout": regular_matrix.max_invocations_within_job_timeout(),
            "row_budget_risk_is_informational": True,
        },
    }, by_job


def validate_workflow_files(workflows_dir: Path) -> dict[str, Any]:
    workflow_paths = sorted([*workflows_dir.glob("*.yml"), *workflows_dir.glob("*.yaml")])
    workflow_names = [path.name for path in workflow_paths]
    forbidden_files = [
        name
        for name in workflow_names
        if "terminal" in name.lower() or "discovery" in name.lower() or "recovery" in name.lower()
    ]
    if forbidden_files:
        raise ValidationError(f"forbidden terminal-discovery/recovery workflow files present: {forbidden_files}")
    if workflow_names != EXPECTED_WORKFLOW_FILES:
        raise ValidationError(f"workflow files must be exactly {EXPECTED_WORKFLOW_FILES!r}, got {workflow_names!r}")
    return {
        "workflow_files": workflow_names,
        "expected_workflow_files": EXPECTED_WORKFLOW_FILES,
        "terminal_discovery_workflow_files": [],
    }


def build_validation_report(plan_path: Path, workflow_path: Path, workflows_dir: Path) -> dict[str, Any]:
    plan_report, _by_job = validate_plan_matrix(plan_path)
    workflow_report = validate_workflow(workflow_path)
    workflow_files_report = validate_workflow_files(workflows_dir)
    return {
        "schema_name": VALIDATION_SCHEMA,
        "schema_version": SCHEMA_VERSION,
        "status": "pass",
        "runtime_budget_notice": regular_matrix.RUNTIME_BUDGET_NOTICE,
        **plan_report,
        "workflow": workflow_report,
        "workflow_files": workflow_files_report,
        "non_goals": [
            "no terminal-discovery workflow or benchmark dispatch mode",
            "no repository_dispatch or workflow_call benchmark surface",
            "no workflow dispatch/cancel/rerun",
            "no artifact/log inspection, download, or collection",
            "no simulator execution, long simulation, or calibration",
            "no finishable/provenance/Tier regeneration",
            "no terminal finishability completion claim",
        ],
    }


def parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--plan", default=str(DEFAULT_PLAN), help="Path to mi300a_problem_size_discovery_plan.json")
    parser.add_argument("--workflow", default=str(DEFAULT_WORKFLOW), help="Path to .github/workflows/benchmark.yml")
    parser.add_argument(
        "--workflows-dir",
        default=str(DEFAULT_WORKFLOW.parent),
        help="Directory containing permanent GitHub Actions workflow files",
    )
    parser.add_argument("--report", help="Optional path to write the deterministic JSON report")
    return parser.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    args = parse_args(argv)
    try:
        report = build_validation_report(Path(args.plan), Path(args.workflow), Path(args.workflows_dir))
    except ValidationError as exc:
        print("FAIL: regular MI300A workflow matrix contract validation failed.", file=sys.stderr)
        print(f"  - {exc}", file=sys.stderr)
        return 1

    report_text = json.dumps(report, indent=2, sort_keys=True) + "\n"
    if args.report:
        Path(args.report).write_text(report_text, encoding="utf-8")
    print(report_text, end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
