#!/usr/bin/env python3
"""Validate the retired generated MI300A regular Tier-1 audit matrix.

This command is intentionally static and read-only. It consumes the checked-in
MI300A discovery plan, bounded finishable-size manifest, and retired/offline
``benchmark-comparison/generated/mi300a_regular_tier1_matrix.json`` artifact.
The generated artifact is preserved only for auditability with its historical
600-second per-simulator / 3600-second benchmark-job timeout contract; it is not
the maintained regular workflow matrix, which is validated separately by
``validate_mi300a_regular_workflow_matrix.py`` at 7200 seconds per simulator and
21600 seconds per benchmark-tier job. This command does not query GitHub,
dispatch/cancel/rerun Actions, run simulators, collect artifacts, or regenerate
finishable/provenance evidence.
"""

from __future__ import annotations

import argparse
import collections
import json
import re
import sys
from pathlib import Path
from typing import Any, Iterable, Mapping, Optional, Sequence

import generate_mi300a_finishable_size_manifest as finishable


VALIDATION_SCHEMA = "mgpusim.mi300a_retired_generated_regular_tier1_matrix_audit_validation"
MATRIX_SCHEMA = "mgpusim.mi300a_regular_tier1_matrix"
SCHEMA_VERSION = 1

DEFAULT_MATRIX = finishable.REPO_ROOT / "benchmark-comparison" / "generated" / "mi300a_regular_tier1_matrix.json"
DEFAULT_MANIFEST = finishable.DEFAULT_MANIFEST
DEFAULT_PLAN = finishable.DEFAULT_PLAN

EXPECTED_SOURCE_PROVENANCE = "benchmark-comparison/provenance/mi300a_problem_size_discovery_run_24959959195.json"
EXPECTED_TIER = 1
EXPECTED_TIER_NAME = "tier1"
EXPECTED_RETIRED_MAX_PARALLEL = 14
EXPECTED_RETIRED_PER_SIMULATOR_TIMEOUT_SEC = 600
EXPECTED_RETIRED_BENCHMARK_JOB_TIMEOUT_SEC = 3600
EXPECTED_RETIRED_ROW_OVERHEAD_BUDGET_SEC = 0
EXPECTED_MAINTAINED_MAX_PARALLEL = 14
EXPECTED_MAINTAINED_PER_SIMULATOR_TIMEOUT_SEC = 7200
EXPECTED_MAINTAINED_BENCHMARK_JOB_TIMEOUT_SEC = 21600
EXPECTED_MAINTAINED_BENCHMARK_JOB_TIMEOUT_MINUTES = 360
RETIRED_ARTIFACT_LIFECYCLE_STATUS = "retired_offline_generated_audit_artifact"
MIN_MATRIX_ROWS = 12
MIN_DISTINCT_BENCHMARK_FAMILIES = 10
REQUIRED_NON_GOALS = [
    "no workflow dispatch/cancel/rerun",
    "no simulator execution, long simulation, or calibration",
    "no provenance, finishable manifest, or Tier artifact regeneration",
    "no terminal finishability completion claim",
]
CONFUSING_RETIRED_ARTIFACT_WORDING = (
    "for the maintained mi300a benchmark workflow",
    "used by the maintained benchmark workflow",
    "used by maintained benchmark workflow",
    "maintained regular workflow matrix",
    "maintained regular workflow pruning source",
    "regular workflow probes",
)
CACHE_BANDWIDTH_WORK_SWEEP_BENCHMARKS = ("l1_cache_bw", "l2_cache_bw")
CACHE_BANDWIDTH_EXPECTED_SCALING_AXIS = "num_elements"
CACHE_BANDWIDTH_EXPECTED_FLAG_TEMPLATE = "-num_elements {size}"
CACHE_BANDWIDTH_EXPECTED_SIZE_LABEL_TEMPLATE = "{size}"
CACHE_BANDWIDTH_EXPECTED_TOTAL_READ_BYTES_FORMULA = (
    "total_read_bytes = num_elements * num_iterations * bytes_per_read"
)


class ValidationError(Exception):
    """Raised when the retired generated audit matrix violates its static contract."""


# ---------------------------------------------------------------------------
# Generic helpers
# ---------------------------------------------------------------------------


def repo_path(path_text: str) -> Path:
    path = Path(path_text)
    return path if path.is_absolute() else finishable.REPO_ROOT / path


def repo_relative(path: Path) -> str:
    return finishable.repo_relative(path)


def load_json_object(path: Path) -> Mapping[str, Any]:
    try:
        with path.open("r", encoding="utf-8") as f:
            data = json.load(f)
    except FileNotFoundError as exc:
        raise ValidationError(f"{repo_relative(path)}: file not found") from exc
    except json.JSONDecodeError as exc:
        raise ValidationError(f"{repo_relative(path)}: invalid JSON: {exc}") from exc
    if not isinstance(data, Mapping):
        raise ValidationError(f"{repo_relative(path)}: JSON root must be an object")
    return data


def obj(value: Any, label: str) -> Mapping[str, Any]:
    if not isinstance(value, Mapping):
        raise ValidationError(f"{label} must be an object")
    return value


def seq(value: Any, label: str) -> list[Any]:
    if not isinstance(value, list):
        raise ValidationError(f"{label} must be a list")
    return value


def nonempty_text(value: Any, label: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise ValidationError(f"{label} must be a non-empty string")
    return value


def integer(value: Any, label: str) -> int:
    try:
        return finishable.as_int(value, label)
    except finishable.ManifestError as exc:
        raise ValidationError(str(exc)) from exc


def require_equal(label: str, actual: Any, expected: Any) -> None:
    if actual != expected:
        raise ValidationError(f"{label} must be {expected!r}, got {actual!r}")


def require_true(label: str, actual: Any) -> None:
    if actual is not True:
        raise ValidationError(f"{label} must be true, got {actual!r}")


def make_ranges(values: Iterable[int]) -> list[str]:
    return finishable.make_plan_index_ranges(values)


def parse_ranges(value: Any, label: str, plan_count: int) -> set[int]:
    try:
        return finishable.parse_plan_index_ranges(value, label, plan_count)
    except finishable.ManifestError as exc:
        raise ValidationError(str(exc)) from exc


def size_tokens(row: Mapping[str, Any], label: str) -> list[str]:
    sizes = nonempty_text(row.get("sizes"), f"{label}.sizes").split()
    if not sizes:
        raise ValidationError(f"{label}.sizes must contain at least one token")
    return sizes


def optional_source_value(row: Mapping[str, Any], source: Mapping[str, Any], key: str, label: str) -> None:
    expected = source.get(key)
    if expected is None:
        if key in row:
            raise ValidationError(f"{label}.{key} must be omitted when the source plan entry omits it")
    elif row.get(key) != expected:
        raise ValidationError(f"{label}.{key} must be {expected!r}, got {row.get(key)!r}")


def cache_bandwidth_params_path(benchmark: str) -> Path:
    return finishable.REPO_ROOT / "workloads" / "microbench" / benchmark / "params.json"


# ---------------------------------------------------------------------------
# Source loading and validation
# ---------------------------------------------------------------------------


def load_source_documents(
    plan_path: Path,
    manifest_path: Path,
) -> tuple[Mapping[str, Any], Mapping[str, Any], dict[int, Mapping[str, Any]], dict[int, Mapping[str, Any]]]:
    plan = load_json_object(plan_path)
    manifest = load_json_object(manifest_path)

    require_equal("plan.schema_name", plan.get("schema_name"), "mgpusim.mi300a_problem_size_discovery_plan")
    require_equal("manifest.schema_name", manifest.get("schema_name"), finishable.MANIFEST_SCHEMA)
    require_equal(
        "manifest.source_paths.problem_size_discovery_plan",
        obj(manifest.get("source_paths"), "manifest.source_paths").get("problem_size_discovery_plan"),
        repo_relative(plan_path),
    )

    plan_entries = [
        obj(entry, f"plan.entries[{offset}]")
        for offset, entry in enumerate(seq(plan.get("entries"), "plan.entries"))
    ]
    manifest_entries = [
        obj(entry, f"manifest.entries[{offset}]")
        for offset, entry in enumerate(seq(manifest.get("entries"), "manifest.entries"))
    ]
    expected_count = finishable.EXPECTED_ENTRY_COUNT
    if len(plan_entries) != expected_count:
        raise ValidationError(f"plan.entries length must be {expected_count}, got {len(plan_entries)}")
    if len(manifest_entries) != len(plan_entries):
        raise ValidationError(
            "manifest.entries length must match plan.entries "
            f"({len(manifest_entries)} != {len(plan_entries)})"
        )

    plan_by_index: dict[int, Mapping[str, Any]] = {}
    manifest_by_index: dict[int, Mapping[str, Any]] = {}
    for label, entries, dest in (
        ("plan", plan_entries, plan_by_index),
        ("manifest", manifest_entries, manifest_by_index),
    ):
        for offset, entry in enumerate(entries):
            plan_index = integer(entry.get("plan_index"), f"{label}.entries[{offset}].plan_index")
            if plan_index in dest:
                raise ValidationError(f"{label}.entries duplicate plan_index {plan_index}")
            dest[plan_index] = entry
    expected_indices = set(range(1, expected_count + 1))
    if set(plan_by_index) != expected_indices:
        raise ValidationError("plan.entries must cover plan indices 1..1416 exactly once")
    if set(manifest_by_index) != expected_indices:
        raise ValidationError("manifest.entries must cover plan indices 1..1416 exactly once")
    return plan, manifest, plan_by_index, manifest_by_index


# ---------------------------------------------------------------------------
# Matrix validation
# ---------------------------------------------------------------------------


def normalized_text(value: str) -> str:
    return " ".join(value.lower().split())


def reject_confusing_retired_artifact_wording(label: str, value: Any) -> str:
    text = nonempty_text(value, label)
    normalized = normalized_text(text)
    for phrase in CONFUSING_RETIRED_ARTIFACT_WORDING:
        if phrase in normalized:
            raise ValidationError(
                f"{label} uses confusing maintained-workflow wording for the retired "
                f"generated 600s/3600s audit artifact: {phrase!r}"
            )
    if "active runtime contract" in normalized and "not an active runtime contract" not in normalized:
        raise ValidationError(
            f"{label} must not describe the retired generated 600s/3600s "
            "audit artifact as an active runtime contract"
        )
    return text


def maintained_regular_workflow_contract() -> dict[str, int]:
    return {
        "max_parallel": EXPECTED_MAINTAINED_MAX_PARALLEL,
        "per_simulator_timeout_sec": EXPECTED_MAINTAINED_PER_SIMULATOR_TIMEOUT_SEC,
        "benchmark_job_timeout_sec": EXPECTED_MAINTAINED_BENCHMARK_JOB_TIMEOUT_SEC,
        "benchmark_job_timeout_minutes": EXPECTED_MAINTAINED_BENCHMARK_JOB_TIMEOUT_MINUTES,
    }


def validate_retired_artifact_lifecycle(matrix: Mapping[str, Any]) -> dict[str, Any]:
    lifecycle = obj(matrix.get("artifact_lifecycle"), "matrix.artifact_lifecycle")
    require_equal(
        "retired generated matrix.artifact_lifecycle.status",
        lifecycle.get("status"),
        RETIRED_ARTIFACT_LIFECYCLE_STATUS,
    )

    retirement_note = reject_confusing_retired_artifact_wording(
        "matrix.artifact_lifecycle.retirement_note",
        lifecycle.get("retirement_note"),
    )
    retirement_normalized = normalized_text(retirement_note)
    for phrase in ("retained for audit", "not the maintained regular mi300a workflow contract"):
        if phrase not in retirement_normalized:
            raise ValidationError(
                "matrix.artifact_lifecycle.retirement_note must explicitly state that the "
                "generated artifact is retained for audit and is not the maintained "
                "regular MI300A workflow contract"
            )

    preserved_note = reject_confusing_retired_artifact_wording(
        "matrix.artifact_lifecycle.preserved_runtime_contract_note",
        lifecycle.get("preserved_runtime_contract_note"),
    )
    preserved_normalized = normalized_text(preserved_note)
    for phrase in ("historical 600s", "3600s", "retired artifact"):
        if phrase not in preserved_normalized:
            raise ValidationError(
                "matrix.artifact_lifecycle.preserved_runtime_contract_note must "
                "classify the preserved 600s/3600s values as retired historical audit data"
            )

    current_contract = obj(
        lifecycle.get("current_regular_workflow_contract"),
        "matrix.artifact_lifecycle.current_regular_workflow_contract",
    )
    source = nonempty_text(
        current_contract.get("source"),
        "matrix.artifact_lifecycle.current_regular_workflow_contract.source",
    )
    for snippet in (
        "scripts/materialize_mi300a_regular_workflow_matrix.py",
        ".github/workflows/benchmark.yml",
    ):
        if snippet not in source:
            raise ValidationError(
                "matrix.artifact_lifecycle.current_regular_workflow_contract.source "
                f"must mention {snippet!r}"
            )

    expected_current_contract = maintained_regular_workflow_contract()
    for key, expected in expected_current_contract.items():
        require_equal(
            f"matrix.artifact_lifecycle.current_regular_workflow_contract.{key}",
            integer(
                current_contract.get(key),
                f"matrix.artifact_lifecycle.current_regular_workflow_contract.{key}",
            ),
            expected,
        )

    return {
        "status": lifecycle.get("status"),
        "retired_generated_audit_only": True,
        "not_maintained_regular_workflow_contract": True,
        "preserved_runtime_contract": "historical_600s_per_simulator_3600s_job",
        "current_regular_workflow_contract": expected_current_contract,
    }


def validate_runtime_contract(matrix: Mapping[str, Any]) -> dict[str, int]:
    contract = obj(matrix.get("runtime_contract"), "matrix.runtime_contract")
    values = {
        "max_parallel": integer(contract.get("max_parallel"), "matrix.runtime_contract.max_parallel"),
        "per_simulator_timeout_sec": integer(
            contract.get("per_simulator_timeout_sec"), "matrix.runtime_contract.per_simulator_timeout_sec"
        ),
        "benchmark_job_timeout_sec": integer(
            contract.get("benchmark_job_timeout_sec"), "matrix.runtime_contract.benchmark_job_timeout_sec"
        ),
        "row_overhead_budget_sec": integer(
            contract.get("row_overhead_budget_sec"), "matrix.runtime_contract.row_overhead_budget_sec"
        ),
        "max_simulator_invocations_per_row": integer(
            contract.get("max_simulator_invocations_per_row"),
            "matrix.runtime_contract.max_simulator_invocations_per_row",
        ),
    }
    require_equal(
        "retired generated matrix.runtime_contract.max_parallel",
        values["max_parallel"],
        EXPECTED_RETIRED_MAX_PARALLEL,
    )
    require_equal(
        "retired generated matrix.runtime_contract.per_simulator_timeout_sec",
        values["per_simulator_timeout_sec"],
        EXPECTED_RETIRED_PER_SIMULATOR_TIMEOUT_SEC,
    )
    require_equal(
        "retired generated matrix.runtime_contract.benchmark_job_timeout_sec",
        values["benchmark_job_timeout_sec"],
        EXPECTED_RETIRED_BENCHMARK_JOB_TIMEOUT_SEC,
    )
    require_equal(
        "retired generated matrix.runtime_contract.row_overhead_budget_sec",
        values["row_overhead_budget_sec"],
        EXPECTED_RETIRED_ROW_OVERHEAD_BUDGET_SEC,
    )
    derived_max = values["benchmark_job_timeout_sec"] // values["per_simulator_timeout_sec"]
    if values["max_simulator_invocations_per_row"] > derived_max:
        raise ValidationError(
            "retired generated matrix.runtime_contract.max_simulator_invocations_per_row "
            "exceeds the preserved 3600s audit job budget "
            f"({values['max_simulator_invocations_per_row']} > {derived_max})"
        )
    return values


def validate_selection_policy(matrix: Mapping[str, Any]) -> Mapping[str, Any]:
    policy = obj(matrix.get("selection_policy"), "matrix.selection_policy")
    scope = reject_confusing_retired_artifact_wording(
        "matrix.selection_policy.scope",
        policy.get("scope"),
    )
    scope_normalized = normalized_text(scope)
    for phrase in ("retired", "audit", "not an active runtime contract", "pruning source"):
        if phrase not in scope_normalized:
            raise ValidationError(
                "matrix.selection_policy.scope must classify this as a retired audit "
                "selection, not the maintained regular workflow matrix"
            )
    reject_confusing_retired_artifact_wording(
        "matrix.selection_policy.source_reference",
        policy.get("source_reference"),
    )
    minimum_rows = integer(policy.get("minimum_matrix_rows"), "matrix.selection_policy.minimum_matrix_rows")
    minimum_families = integer(
        policy.get("minimum_distinct_benchmark_families"),
        "matrix.selection_policy.minimum_distinct_benchmark_families",
    )
    if minimum_rows < MIN_MATRIX_ROWS:
        raise ValidationError(f"matrix.selection_policy.minimum_matrix_rows must be at least {MIN_MATRIX_ROWS}")
    if minimum_families < MIN_DISTINCT_BENCHMARK_FAMILIES:
        raise ValidationError(
            "matrix.selection_policy.minimum_distinct_benchmark_families must be at least "
            f"{MIN_DISTINCT_BENCHMARK_FAMILIES}"
        )
    require_true("matrix.selection_policy.small_priority_probe_rows", policy.get("small_priority_probe_rows"))
    non_claim = reject_confusing_retired_artifact_wording(
        "matrix.selection_policy.non_finishable_claim",
        policy.get("non_finishable_claim"),
    ).lower()
    for phrase in ("not", "finishable evidence"):
        if phrase not in non_claim:
            raise ValidationError(
                "matrix.selection_policy.non_finishable_claim must explicitly avoid "
                "finishable-evidence promotion"
            )
    return policy


def validate_cache_bandwidth_axis_contract(
    *,
    plan_by_index: Mapping[int, Mapping[str, Any]],
    include_rows: Sequence[Mapping[str, Any]],
) -> dict[str, Any]:
    """Validate that L1/L2 work-scaling rows cannot regress to a stale axis."""
    benchmarks: dict[str, dict[str, Any]] = {}
    for benchmark in CACHE_BANDWIDTH_WORK_SWEEP_BENCHMARKS:
        params_path = cache_bandwidth_params_path(benchmark)
        params = load_json_object(params_path)
        params_label = repo_relative(params_path)
        require_equal(f"{params_label}.benchmark", params.get("benchmark"), benchmark)

        scaling = obj(
            params.get("scaling_parameter"),
            f"{params_label}.scaling_parameter",
        )
        require_equal(
            f"{params_label}.scaling_parameter.name",
            scaling.get("name"),
            CACHE_BANDWIDTH_EXPECTED_SCALING_AXIS,
        )
        require_equal(
            f"{params_label}.scaling_parameter.scaling_role",
            scaling.get("scaling_role"),
            "work_amount_sweep",
        )
        require_true(
            f"{params_label}.scaling_parameter.work_amount_scaled",
            scaling.get("work_amount_scaled"),
        )

        semantics = obj(
            params.get("validation_semantics"),
            f"{params_label}.validation_semantics",
        )
        require_equal(
            f"{params_label}.validation_semantics.classification",
            semantics.get("classification"),
            "cache_bandwidth_work_sweep",
        )
        require_true(
            f"{params_label}.validation_semantics.work_amount_scaled_by_axis",
            semantics.get("work_amount_scaled_by_axis"),
        )

        work_model = obj(params.get("work_model"), f"{params_label}.work_model")
        require_equal(
            f"{params_label}.work_model.total_read_bytes_formula",
            work_model.get("total_read_bytes_formula"),
            CACHE_BANDWIDTH_EXPECTED_TOTAL_READ_BYTES_FORMULA,
        )

        expected_size_tokens = [
            str(value)
            for value in seq(
                scaling.get("values"),
                f"{params_label}.scaling_parameter.values",
            )
        ]
        if not expected_size_tokens:
            raise ValidationError(
                f"{params_label}.scaling_parameter.values must not be empty"
            )

        plan_entries = [
            entry
            for entry in sorted(
                plan_by_index.values(),
                key=lambda item: integer(item.get("plan_index"), "plan_index"),
            )
            if entry.get("benchmark") == benchmark
            and entry.get("tier") == EXPECTED_TIER
            and entry.get("workflow_job") == "benchmark-tier1"
        ]
        actual_size_tokens = [str(entry.get("size_token")) for entry in plan_entries]
        require_equal(
            f"{benchmark} source-plan size tokens",
            actual_size_tokens,
            expected_size_tokens,
        )
        for entry in plan_entries:
            plan_index = integer(entry.get("plan_index"), f"{benchmark}.plan_index")
            require_equal(
                f"plan.entries[{plan_index}].flag_template",
                entry.get("flag_template"),
                CACHE_BANDWIDTH_EXPECTED_FLAG_TEMPLATE,
            )
            require_equal(
                f"plan.entries[{plan_index}].size_label_template",
                entry.get("size_label_template"),
                CACHE_BANDWIDTH_EXPECTED_SIZE_LABEL_TEMPLATE,
            )

        matrix_rows = [row for row in include_rows if row.get("benchmark") == benchmark]
        if not matrix_rows:
            raise ValidationError(
                "matrix.include must contain a "
                f"{benchmark} row for the cache-bandwidth axis contract"
            )
        for offset, row in enumerate(matrix_rows):
            require_equal(
                f"matrix.include cache-bandwidth {benchmark} row {offset}.flag_template",
                row.get("flag_template"),
                CACHE_BANDWIDTH_EXPECTED_FLAG_TEMPLATE,
            )
            require_equal(
                f"matrix.include cache-bandwidth {benchmark} row {offset}.size_label_template",
                row.get("size_label_template"),
                CACHE_BANDWIDTH_EXPECTED_SIZE_LABEL_TEMPLATE,
            )

        matrix_plan_indices = [
            index
            for row in matrix_rows
            for index in seq(
                row.get("source_plan_indices"),
                f"matrix.include.{benchmark}.source_plan_indices",
            )
        ]
        benchmarks[benchmark] = {
            "status": "pass",
            "params_path": params_label,
            "scaling_axis": CACHE_BANDWIDTH_EXPECTED_SCALING_AXIS,
            "flag_template": CACHE_BANDWIDTH_EXPECTED_FLAG_TEMPLATE,
            "size_label_template": CACHE_BANDWIDTH_EXPECTED_SIZE_LABEL_TEMPLATE,
            "total_read_bytes_formula": CACHE_BANDWIDTH_EXPECTED_TOTAL_READ_BYTES_FORMULA,
            "source_plan_entry_count": len(plan_entries),
            "source_plan_size_count": len(actual_size_tokens),
            "matrix_row_count": len(matrix_rows),
            "matrix_source_plan_index_ranges": make_ranges(
                integer(index, "matrix_source_plan_index")
                for index in matrix_plan_indices
            ),
        }

    return {
        "status": "pass",
        "benchmarks": benchmarks,
    }


def validate_row(
    row: Mapping[str, Any],
    *,
    label: str,
    runtime_contract: Mapping[str, int],
    plan_by_index: Mapping[int, Mapping[str, Any]],
    manifest_by_index: Mapping[int, Mapping[str, Any]],
) -> tuple[list[int], str, str, dict[str, int]]:
    benchmark = nonempty_text(row.get("benchmark"), f"{label}.benchmark")
    family = nonempty_text(row.get("benchmark_family"), f"{label}.benchmark_family")
    artifact_id = nonempty_text(row.get("artifact_id"), f"{label}.artifact_id")
    if not re.fullmatch(r"[A-Za-z0-9_.-]+", artifact_id):
        raise ValidationError(f"{label}.artifact_id contains characters unsupported by artifact names: {artifact_id!r}")
    if row.get("tier") != EXPECTED_TIER_NAME:
        raise ValidationError(f"{label}.tier must be {EXPECTED_TIER_NAME!r}")
    if not (finishable.REPO_ROOT / "amd" / "samples" / benchmark).is_dir():
        raise ValidationError(f"{label}.benchmark has no amd/samples/{benchmark}/ directory")

    sizes = size_tokens(row, label)
    plan_indices = [
        integer(value, f"{label}.source_plan_indices[{offset}]")
        for offset, value in enumerate(
            seq(row.get("source_plan_indices"), f"{label}.source_plan_indices")
        )
    ]
    if len(plan_indices) != len(sizes):
        raise ValidationError(f"{label}.source_plan_indices length must match sizes token count")
    if plan_indices != sorted(plan_indices):
        raise ValidationError(f"{label}.source_plan_indices must be in source-plan order")
    if len(set(plan_indices)) != len(plan_indices):
        raise ValidationError(f"{label}.source_plan_indices must not duplicate plan indices")

    timeout_sec = integer(row.get("timeout_sec"), f"{label}.timeout_sec")
    require_equal(f"{label}.timeout_sec", timeout_sec, runtime_contract["per_simulator_timeout_sec"])
    expected_invocations = integer(
        row.get("expected_simulator_invocations"), f"{label}.expected_simulator_invocations"
    )
    require_equal(f"{label}.expected_simulator_invocations", expected_invocations, len(sizes))
    row_budget = integer(row.get("row_timeout_budget_sec"), f"{label}.row_timeout_budget_sec")
    expected_budget = expected_invocations * runtime_contract["per_simulator_timeout_sec"] + runtime_contract[
        "row_overhead_budget_sec"
    ]
    require_equal(f"{label}.row_timeout_budget_sec", row_budget, expected_budget)
    if expected_budget > runtime_contract["benchmark_job_timeout_sec"]:
        raise ValidationError(
            f"{label} exceeds retired generated audit benchmark job timeout budget: "
            f"{expected_budget}s > {runtime_contract['benchmark_job_timeout_sec']}s"
        )
    if expected_invocations > runtime_contract["max_simulator_invocations_per_row"]:
        raise ValidationError(
            f"{label}.expected_simulator_invocations exceeds max_simulator_invocations_per_row"
        )

    source_size_tokens = [
        nonempty_text(value, f"{label}.source_size_tokens[{offset}]")
        for offset, value in enumerate(seq(row.get("source_size_tokens"), f"{label}.source_size_tokens"))
    ]
    if source_size_tokens != sizes:
        raise ValidationError(f"{label}.source_size_tokens must match sizes tokens")

    expected_outcome_buckets: list[str] = []
    expected_eligibility_statuses: list[str] = []
    for size_offset, (plan_index, size) in enumerate(zip(plan_indices, sizes)):
        plan_entry = plan_by_index.get(plan_index)
        manifest_entry = manifest_by_index.get(plan_index)
        if plan_entry is None or manifest_entry is None:
            raise ValidationError(
                f"{label}.source_plan_indices[{size_offset}] references "
                f"missing plan index {plan_index}"
            )
        source_label = f"{label}.source_plan_indices[{size_offset}]"
        for key in (
            "benchmark",
            "problem_size",
            "workflow_benchmark_name",
            "workflow_job",
            "tier",
            "tier_name",
            "size_token",
            "sizes",
            "flag_template",
            "size_label_template",
        ):
            if manifest_entry.get(key) != plan_entry.get(key):
                raise ValidationError(f"manifest entry {plan_index}.{key} must match the source plan entry")
        require_equal(f"{source_label}.tier", plan_entry.get("tier"), EXPECTED_TIER)
        require_equal(f"{source_label}.tier_name", plan_entry.get("tier_name"), EXPECTED_TIER_NAME)
        require_equal(f"{source_label}.workflow_job", plan_entry.get("workflow_job"), "benchmark-tier1")
        require_equal(f"{source_label}.workflow_benchmark_name", plan_entry.get("workflow_benchmark_name"), benchmark)
        require_equal(f"{source_label}.benchmark_family", plan_entry.get("benchmark"), family)
        require_equal(f"{source_label}.size_token", str(plan_entry.get("size_token")), size)
        require_equal(f"{source_label}.sizes", str(plan_entry.get("sizes")), size)
        require_equal(f"{label}.flag_template", row.get("flag_template"), plan_entry.get("flag_template"))
        require_equal(
            f"{label}.size_label_template",
            row.get("size_label_template"),
            plan_entry.get("size_label_template"),
        )
        optional_source_value(row, plan_entry, "extra_flags", label)
        optional_source_value(row, plan_entry, "gomemlimit", label)
        optional_source_value(row, plan_entry, "gogc", label)
        expected_outcome_buckets.append(
            nonempty_text(
                manifest_entry.get("observed_outcome_bucket"),
                f"manifest.entries[{plan_index}].observed_outcome_bucket",
            )
        )
        expected_eligibility_statuses.append(
            nonempty_text(
                manifest_entry.get("eligibility_status"),
                f"manifest.entries[{plan_index}].eligibility_status",
            )
        )

    declared_buckets = [
        nonempty_text(value, f"{label}.source_observed_outcome_buckets[{offset}]")
        for offset, value in enumerate(
            seq(
                row.get("source_observed_outcome_buckets"),
                f"{label}.source_observed_outcome_buckets",
            )
        )
    ]
    if declared_buckets != expected_outcome_buckets:
        raise ValidationError(f"{label}.source_observed_outcome_buckets must match the manifest source rows")
    declared_statuses = [
        nonempty_text(value, f"{label}.source_eligibility_statuses[{offset}]")
        for offset, value in enumerate(
            seq(row.get("source_eligibility_statuses"), f"{label}.source_eligibility_statuses")
        )
    ]
    if declared_statuses != expected_eligibility_statuses:
        raise ValidationError(f"{label}.source_eligibility_statuses must match the manifest source rows")

    return (
        plan_indices,
        artifact_id,
        family,
        {
            "invocations": expected_invocations,
            "budget_sec": row_budget,
        },
    )


def validate_retired_generated_regular_tier1_matrix(
    *,
    matrix_path: Path,
    manifest_path: Path,
    plan_path: Path,
) -> dict[str, Any]:
    plan, manifest, plan_by_index, manifest_by_index = load_source_documents(plan_path, manifest_path)
    matrix = load_json_object(matrix_path)

    require_equal("matrix.schema_name", matrix.get("schema_name"), MATRIX_SCHEMA)
    require_equal(
        "matrix.schema_version",
        integer(matrix.get("schema_version"), "matrix.schema_version"),
        SCHEMA_VERSION,
    )
    require_equal("matrix.source_manifest", matrix.get("source_manifest"), repo_relative(manifest_path))
    require_equal("matrix.source_plan", matrix.get("source_plan"), repo_relative(plan_path))
    require_equal("matrix.source_provenance", matrix.get("source_provenance"), EXPECTED_SOURCE_PROVENANCE)
    require_equal("matrix.tier", integer(matrix.get("tier"), "matrix.tier"), EXPECTED_TIER)
    require_equal("matrix.tier_name", matrix.get("tier_name"), EXPECTED_TIER_NAME)
    run = obj(matrix.get("run"), "matrix.run")
    manifest_run = obj(manifest.get("run"), "manifest.run")
    for key in ("database_id", "status", "conclusion", "head_branch", "head_sha", "non_terminal_snapshot"):
        require_equal(f"matrix.run.{key}", run.get(key), manifest_run.get(key))
    snapshot_contract = obj(matrix.get("snapshot_contract"), "matrix.snapshot_contract")
    require_true("matrix.snapshot_contract.bounded_snapshot", snapshot_contract.get("bounded_snapshot"))
    claim_scope = nonempty_text(snapshot_contract.get("claim_scope"), "matrix.snapshot_contract.claim_scope").lower()
    if "not final/current finishable-size claims" not in claim_scope:
        raise ValidationError(
            "matrix.snapshot_contract.claim_scope must preserve the bounded "
            "non-terminal snapshot disclaimer"
        )

    runtime_contract = validate_runtime_contract(matrix)
    lifecycle_report = validate_retired_artifact_lifecycle(matrix)
    validate_selection_policy(matrix)

    include_rows = [
        obj(row, f"matrix.include[{offset}]")
        for offset, row in enumerate(seq(matrix.get("include"), "matrix.include"))
    ]
    if len(include_rows) < MIN_MATRIX_ROWS:
        raise ValidationError(
            f"retired generated matrix.include must contain at least {MIN_MATRIX_ROWS} audit rows"
        )
    require_equal("matrix.row_count", integer(matrix.get("row_count"), "matrix.row_count"), len(include_rows))

    seen_plan_indices: set[int] = set()
    seen_artifact_ids: set[str] = set()
    families: list[str] = []
    row_budgets: list[dict[str, Any]] = []
    outcome_bucket_counts: collections.Counter[str] = collections.Counter()
    eligibility_status_counts: collections.Counter[str] = collections.Counter()

    for offset, row in enumerate(include_rows):
        label = f"matrix.include[{offset}]"
        plan_indices, artifact_id, family, budget = validate_row(
            row,
            label=label,
            runtime_contract=runtime_contract,
            plan_by_index=plan_by_index,
            manifest_by_index=manifest_by_index,
        )
        if artifact_id in seen_artifact_ids:
            raise ValidationError(f"{label}.artifact_id duplicates another matrix row: {artifact_id}")
        seen_artifact_ids.add(artifact_id)
        overlap = seen_plan_indices & set(plan_indices)
        if overlap:
            raise ValidationError(f"{label}.source_plan_indices duplicates source plan indices {make_ranges(overlap)}")
        seen_plan_indices.update(plan_indices)
        families.append(family)
        row_budgets.append({"row_index": offset, "artifact_id": artifact_id, **budget})
        outcome_bucket_counts.update(
            str(manifest_by_index[index].get("observed_outcome_bucket"))
            for index in plan_indices
        )
        eligibility_status_counts.update(
            str(manifest_by_index[index].get("eligibility_status"))
            for index in plan_indices
        )

    distinct_families = sorted(set(families))
    if len(distinct_families) < MIN_DISTINCT_BENCHMARK_FAMILIES:
        raise ValidationError(
            "retired generated matrix.include must cover at least "
            f"{MIN_DISTINCT_BENCHMARK_FAMILIES} distinct audit benchmark families"
        )
    require_equal(
        "matrix.distinct_benchmark_family_count",
        integer(matrix.get("distinct_benchmark_family_count"), "matrix.distinct_benchmark_family_count"),
        len(distinct_families),
    )
    declared_families = [
        nonempty_text(item, f"matrix.benchmark_families[{offset}]")
        for offset, item in enumerate(
            seq(matrix.get("benchmark_families"), "matrix.benchmark_families")
        )
    ]
    if declared_families != distinct_families:
        raise ValidationError("matrix.benchmark_families must be sorted and match the included row families")

    source_plan_index_ranges = make_ranges(seen_plan_indices)
    declared_source_ranges = parse_ranges(
        matrix.get("source_plan_index_ranges"),
        "matrix.source_plan_index_ranges",
        finishable.EXPECTED_ENTRY_COUNT,
    )
    if declared_source_ranges != seen_plan_indices:
        raise ValidationError(
            "matrix.source_plan_index_ranges must match row source_plan_indices "
            f"(declared={make_ranges(declared_source_ranges)}, actual={source_plan_index_ranges})"
        )
    entry_count = len(seen_plan_indices)
    require_equal("matrix.entry_count", integer(matrix.get("entry_count"), "matrix.entry_count"), entry_count)

    cache_bandwidth_axis_contract = validate_cache_bandwidth_axis_contract(
        plan_by_index=plan_by_index,
        include_rows=include_rows,
    )

    tier1_source_count = sum(1 for entry in plan_by_index.values() if entry.get("tier") == EXPECTED_TIER)
    return {
        "schema_name": VALIDATION_SCHEMA,
        "schema_version": SCHEMA_VERSION,
        "status": "pass",
        "validation_scope": {
            "artifact_role": "retired_generated_audit_only",
            "not_maintained_regular_workflow_matrix": True,
            "maintained_regular_workflow_validator": "scripts/validate_mi300a_regular_workflow_matrix.py",
        },
        "source_paths": {
            "retired_generated_regular_tier1_matrix": repo_relative(matrix_path),
            "finishable_manifest": repo_relative(manifest_path),
            "problem_size_discovery_plan": repo_relative(plan_path),
            "source_provenance": EXPECTED_SOURCE_PROVENANCE,
        },
        "retired_artifact_lifecycle": lifecycle_report,
        "retired_generated_matrix": {
            "row_count": len(include_rows),
            "entry_count": entry_count,
            "distinct_benchmark_family_count": len(distinct_families),
            "benchmark_families": distinct_families,
            "source_plan_index_ranges": source_plan_index_ranges,
            "artifact_id_count": len(seen_artifact_ids),
        },
        "retired_generated_runtime_contract": dict(runtime_contract),
        "maintained_regular_workflow_contract_reference": maintained_regular_workflow_contract(),
        "source_reference": {
            "plan_entry_count": len(plan_by_index),
            "manifest_entry_count": len(manifest_by_index),
            "tier1_source_entry_count": tier1_source_count,
            "observed_outcome_bucket_counts": dict(sorted(outcome_bucket_counts.items())),
            "eligibility_status_counts": dict(sorted(eligibility_status_counts.items())),
        },
        "retired_row_budget_validation": {
            "all_rows_within_budget": True,
            "max_row_budget_sec": max((row["budget_sec"] for row in row_budgets), default=0),
            "rows": row_budgets,
        },
        "cache_bandwidth_axis_contract": cache_bandwidth_axis_contract,
        "non_goals": REQUIRED_NON_GOALS,
    }


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--matrix",
        default=str(DEFAULT_MATRIX),
        help="Path to the retired generated regular Tier-1 audit matrix JSON",
    )
    parser.add_argument(
        "--manifest",
        default=str(DEFAULT_MANIFEST),
        help="Path to the MI300A finishable-size manifest",
    )
    parser.add_argument(
        "--plan",
        default=str(DEFAULT_PLAN),
        help="Path to the MI300A problem-size discovery plan",
    )
    return parser.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    args = parse_args(argv)
    try:
        report = validate_retired_generated_regular_tier1_matrix(
            matrix_path=repo_path(args.matrix),
            manifest_path=repo_path(args.manifest),
            plan_path=repo_path(args.plan),
        )
    except ValidationError as exc:
        print(
            "FAIL: retired generated MI300A regular Tier-1 audit matrix validation rejected the artifact.",
            file=sys.stderr,
        )
        print(f"  - {exc}", file=sys.stderr)
        return 1
    print(json.dumps(report, indent=2, sort_keys=True) + "\n", end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
