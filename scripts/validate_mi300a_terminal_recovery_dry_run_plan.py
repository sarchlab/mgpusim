#!/usr/bin/env python3
"""Validate the static MI300A recovery dry-run plan before dispatch.

The validator consumes the checked-in recovery dry-run plan and the
recovery manifest for replacement run 25111815292.  It proves, without running
simulators or contacting GitHub, that the dry-run matrix covers exactly the 940
missing plan indices once, keeps the 476 previously observed entries as
incomplete prior evidence outside the recovery scope, and satisfies the
per-row-timeout 14-way / 600-second row / 3600-second action-layer budget
contract.
"""

from __future__ import annotations

import argparse
import json
import math
import sys
from pathlib import Path
from typing import Any, Iterable, Mapping, Optional, Sequence

import generate_mi300a_terminal_recovery_dry_run_plan as dry_run
import generate_mi300a_terminal_recovery_manifest as recovery_manifest


VALIDATION_SCHEMA = "mgpusim.mi300a_terminal_discovery_recovery_dry_run_validation"
SCHEMA_VERSION = 1

EXPECTED_PLAN_INDEX_COUNT = recovery_manifest.EXPECTED_ENTRY_COUNT
EXPECTED_MISSING_COUNT = recovery_manifest.EXPECTED_MISSING_COUNT
EXPECTED_PRIOR_OBSERVED_COUNT = recovery_manifest.EXPECTED_OBSERVED_COUNT
EXPECTED_RUN_ID = recovery_manifest.RUN_ID
EXPECTED_RUN_CONCLUSION = "cancelled"
EXPECTED_MAX_PARALLEL = dry_run.DEFAULT_MAX_PARALLEL
EXPECTED_PER_ATTEMPT_TIMEOUT_SEC = dry_run.DEFAULT_PER_ATTEMPT_TIMEOUT_SEC
EXPECTED_ROW_TIMEOUT_SEC = dry_run.DEFAULT_ROW_TIMEOUT_SEC
EXPECTED_ACTION_TIMEOUT_SEC = dry_run.DEFAULT_ACTION_TIMEOUT_SEC
EXPECTED_ROW_OVERHEAD_SEC = dry_run.DEFAULT_ROW_OVERHEAD_SEC
EXPECTED_ATTEMPTS_PER_ROW = 1
REQUIRED_NON_GOALS = {
    "no workflow dispatch/cancel/rerun",
    "no provenance, finishable manifest, or Tier artifact regeneration",
    "no terminal finishability completion claim",
}


class ValidationError(Exception):
    """Raised when the recovery dry-run plan violates the static contract."""


# ---------------------------------------------------------------------------
# Generic helpers
# ---------------------------------------------------------------------------


def repo_path(path_text: str) -> Path:
    return dry_run.repo_path(path_text)


def repo_relative(path: Path) -> str:
    return dry_run.repo_relative(path)


def file_sha256(path: Path) -> str:
    try:
        return dry_run.file_sha256(path)
    except FileNotFoundError as exc:
        raise ValidationError(f"{repo_relative(path)}: file not found") from exc


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


def integer(value: Any, label: str) -> int:
    try:
        return dry_run.as_int(value, label)
    except dry_run.RecoveryDryRunPlanError as exc:
        raise ValidationError(str(exc)) from exc


def text(value: Any, label: str) -> str:
    try:
        return dry_run.nonempty_str(value, label)
    except dry_run.RecoveryDryRunPlanError as exc:
        raise ValidationError(str(exc)) from exc


def ranges(value: Any, label: str) -> list[int]:
    range_texts = [text(item, f"{label}[{offset}]") for offset, item in enumerate(seq(value, label))]
    try:
        parsed = dry_run.parse_plan_index_ranges(range_texts, label, EXPECTED_PLAN_INDEX_COUNT)
    except dry_run.RecoveryDryRunPlanError as exc:
        raise ValidationError(str(exc)) from exc
    if dry_run.make_plan_index_ranges(parsed) != range_texts:
        raise ValidationError(f"{label} ranges must be canonical")
    return parsed


def make_ranges(values: Iterable[int]) -> list[str]:
    return dry_run.make_plan_index_ranges(values)


def bool_true(value: Any, label: str) -> None:
    if value is not True:
        raise ValidationError(f"{label} must be true")


def require_equal(label: str, actual: Any, expected: Any) -> None:
    if actual != expected:
        raise ValidationError(f"{label} must be {expected!r}, got {actual!r}")


def require_int(label: str, actual: Any, expected: int) -> None:
    parsed = integer(actual, label)
    if parsed != expected:
        raise ValidationError(f"{label} must be {expected}, got {parsed}")


def require_int_fields(parent_label: str, source: Mapping[str, Any], expected: Mapping[str, int]) -> None:
    for key, value in expected.items():
        require_int(f"{parent_label}.{key}", source.get(key), value)


def compact_json(value: Any, *, max_len: int = 180) -> str:
    encoded = json.dumps(value, sort_keys=True, separators=(",", ":"))
    return encoded if len(encoded) <= max_len else encoded[: max_len - 3] + "..."


def first_difference(expected: Any, actual: Any, path: str = "$") -> Optional[tuple[str, Any, Any]]:
    if isinstance(expected, Mapping) and isinstance(actual, Mapping):
        if set(expected) != set(actual):
            return (
                f"{path} keys",
                {"missing": sorted(str(key) for key in set(expected) - set(actual))},
                {"unexpected": sorted(str(key) for key in set(actual) - set(expected))},
            )
        for key in expected:
            diff = first_difference(expected[key], actual[key], f"{path}.{key}")
            if diff is not None:
                return diff
        return None
    if isinstance(expected, list) and isinstance(actual, list):
        if len(expected) != len(actual):
            return f"{path}.length", len(expected), len(actual)
        for index, (left, right) in enumerate(zip(expected, actual)):
            diff = first_difference(left, right, f"{path}[{index}]")
            if diff is not None:
                return diff
        return None
    return None if expected == actual else (path, expected, actual)


def require_rederived_plan(plan: Mapping[str, Any], manifest_path: Path, plan_path: Path) -> None:
    try:
        expected = dry_run.build_plan(
            manifest_path,
            max_parallel=EXPECTED_MAX_PARALLEL,
            per_attempt_timeout_sec=EXPECTED_PER_ATTEMPT_TIMEOUT_SEC,
            row_timeout_sec=EXPECTED_ROW_TIMEOUT_SEC,
            action_timeout_sec=EXPECTED_ACTION_TIMEOUT_SEC,
            row_overhead_sec=EXPECTED_ROW_OVERHEAD_SEC,
        )
    except dry_run.RecoveryDryRunPlanError as exc:
        raise ValidationError(f"could not rederive static recovery dry-run plan: {exc}") from exc
    if expected != plan:
        diff = first_difference(expected, plan)
        if diff is None:
            raise ValidationError("checked-in dry-run plan differs from regenerated static dry-run plan")
        path, expected_value, actual_value = diff
        raise ValidationError(
            "checked-in dry-run plan differs from regenerated static dry-run plan "
            f"at {path}: expected {compact_json(expected_value)}, got {compact_json(actual_value)}"
        )
    if plan_path.read_text(encoding="utf-8") != dry_run.json_text(expected):
        raise ValidationError(f"{repo_relative(plan_path)} is not the canonical static recovery dry-run plan")


# ---------------------------------------------------------------------------
# Focused validation
# ---------------------------------------------------------------------------


def validate_manifest_scope(
    manifest: Mapping[str, Any],
) -> tuple[list[int], list[int], list[Mapping[str, Any]], str, str]:
    require_equal("manifest.schema_name", manifest.get("schema_name"), recovery_manifest.MANIFEST_SCHEMA)
    require_int("manifest.schema_version", manifest.get("schema_version"), recovery_manifest.SCHEMA_VERSION)
    require_int_fields(
        "manifest",
        manifest,
        {
            "entry_count": EXPECTED_MISSING_COUNT,
            "recovery_entry_count": EXPECTED_MISSING_COUNT,
            "attempt_count": EXPECTED_MISSING_COUNT,
            "recovery_attempt_count": EXPECTED_MISSING_COUNT,
        },
    )

    source = obj(manifest.get("source_evidence"), "manifest.source_evidence")
    run_id = text(source.get("run_id"), "manifest.source_evidence.run_id")
    run_conclusion = text(source.get("run_conclusion"), "manifest.source_evidence.run_conclusion")
    require_equal("manifest.source_evidence.run_id", run_id, EXPECTED_RUN_ID)
    require_equal("manifest.source_evidence.run_conclusion", run_conclusion, EXPECTED_RUN_CONCLUSION)
    require_int(
        "manifest.source_evidence.prior_attempt_timeout_sec",
        source.get("prior_attempt_timeout_sec"),
        EXPECTED_PER_ATTEMPT_TIMEOUT_SEC,
    )

    source_plan = obj(manifest.get("source_plan"), "manifest.source_plan")
    source_plan_path = repo_path(text(source_plan.get("artifact"), "manifest.source_plan.artifact"))
    require_equal("manifest.source_plan.sha256", source_plan.get("sha256"), file_sha256(source_plan_path))
    require_int_fields(
        "manifest.source_plan",
        source_plan,
        {"entry_count": EXPECTED_PLAN_INDEX_COUNT, "timeout_sec": recovery_manifest.EXPECTED_SOURCE_TIMEOUT_SEC},
    )

    missing = ranges(manifest.get("missing_plan_index_ranges"), "manifest.missing_plan_index_ranges")
    if len(missing) != EXPECTED_MISSING_COUNT:
        raise ValidationError(f"manifest missing scope must contain {EXPECTED_MISSING_COUNT} entries, got {len(missing)}")

    prior = obj(manifest.get("prior_incomplete_evidence"), "manifest.prior_incomplete_evidence")
    require_equal(
        "manifest.prior_incomplete_evidence.treatment",
        prior.get("treatment"),
        "incomplete_prior_evidence_only",
    )
    bool_true(
        prior.get("not_complete_terminal_provenance"),
        "manifest.prior_incomplete_evidence.not_complete_terminal_provenance",
    )
    require_int_fields(
        "manifest.prior_incomplete_evidence",
        prior,
        {
            "observed_unique_plan_index_count": EXPECTED_PRIOR_OBSERVED_COUNT,
            "observed_raw_per_plan_outcome_file_count": EXPECTED_PRIOR_OBSERVED_COUNT,
            "duplicate_observed_plan_index_count": 0,
            "extra_observed_plan_index_count": 0,
        },
    )
    prior_indices = ranges(
        prior.get("observed_plan_index_ranges"),
        "manifest.prior_incomplete_evidence.observed_plan_index_ranges",
    )
    if len(prior_indices) != EXPECTED_PRIOR_OBSERVED_COUNT:
        raise ValidationError(
            f"manifest prior observed scope must contain {EXPECTED_PRIOR_OBSERVED_COUNT} entries, "
            f"got {len(prior_indices)}"
        )

    missing_set = set(missing)
    prior_set = set(prior_indices)
    if missing_set & prior_set:
        raise ValidationError(
            "manifest missing recovery scope overlaps prior observed evidence: "
            f"{make_ranges(sorted(missing_set & prior_set))}"
        )
    all_indices = set(range(1, EXPECTED_PLAN_INDEX_COUNT + 1))
    if missing_set | prior_set != all_indices:
        represented = missing_set | prior_set
        raise ValidationError(
            "manifest missing/prior union does not represent exactly all source plan indices: "
            f"missing={make_ranges(sorted(all_indices - represented))} "
            f"extra={make_ranges(sorted(represented - all_indices))}"
        )

    complete_rows = [
        obj(row, f"manifest.prior_incomplete_evidence.complete_logical_rows_excluded_from_recovery[{offset}]")
        for offset, row in enumerate(
            seq(
                prior.get("complete_logical_rows_excluded_from_recovery"),
                "manifest.prior_incomplete_evidence.complete_logical_rows_excluded_from_recovery",
            )
        )
    ]
    for offset, row in enumerate(complete_rows):
        label = f"manifest.prior_incomplete_evidence.complete_logical_rows_excluded_from_recovery[{offset}]"
        require_equal(f"{label}.treatment", row.get("treatment"), "prior_incomplete_evidence_only_not_recovery_scope")
        require_int(f"{label}.recovery_attempt_count", row.get("recovery_attempt_count"), 0)
        row_indices = set(ranges(row.get("plan_index_ranges"), f"{label}.plan_index_ranges"))
        if not row_indices.issubset(prior_set) or row_indices & missing_set:
            raise ValidationError(f"{label}.plan_index_ranges must be prior-only and outside recovery scope")

    coverage = obj(manifest.get("coverage"), "manifest.coverage")
    require_int_fields(
        "manifest.coverage",
        coverage,
        {
            "expected_plan_index_count": EXPECTED_PLAN_INDEX_COUNT,
            "recovery_missing_plan_index_count": EXPECTED_MISSING_COUNT,
            "prior_observed_plan_index_count": EXPECTED_PRIOR_OBSERVED_COUNT,
            "represented_plan_index_count_after_union": EXPECTED_PLAN_INDEX_COUNT,
            "duplicate_recovery_plan_index_count": 0,
            "extra_recovery_plan_index_count": 0,
            "missing_recovery_plan_index_count": 0,
        },
    )
    return missing, prior_indices, complete_rows, run_id, run_conclusion


def validate_plan_header(plan: Mapping[str, Any], manifest: Mapping[str, Any], manifest_path: Path) -> None:
    require_equal("plan.schema_name", plan.get("schema_name"), dry_run.DRY_RUN_SCHEMA)
    require_int("plan.schema_version", plan.get("schema_version"), dry_run.SCHEMA_VERSION)
    bool_true(plan.get("dry_run"), "plan.dry_run")
    bool_true(plan.get("recovery_only"), "plan.recovery_only")

    source_manifest = obj(plan.get("source_recovery_manifest"), "plan.source_recovery_manifest")
    require_equal("plan.source_recovery_manifest.artifact", source_manifest.get("artifact"), repo_relative(manifest_path))
    require_equal("plan.source_recovery_manifest.sha256", source_manifest.get("sha256"), file_sha256(manifest_path))
    require_equal("plan.source_recovery_manifest.schema_name", source_manifest.get("schema_name"), manifest.get("schema_name"))
    require_int("plan.source_recovery_manifest.schema_version", source_manifest.get("schema_version"), recovery_manifest.SCHEMA_VERSION)
    require_equal("plan.source_recovery_manifest.run_id", source_manifest.get("run_id"), EXPECTED_RUN_ID)
    require_equal("plan.source_recovery_manifest.run_conclusion", source_manifest.get("run_conclusion"), EXPECTED_RUN_CONCLUSION)

    source_plan = obj(plan.get("source_plan"), "plan.source_plan")
    manifest_source_plan = obj(manifest.get("source_plan"), "manifest.source_plan")
    require_equal("plan.source_plan.artifact", source_plan.get("artifact"), manifest_source_plan.get("artifact"))
    require_equal("plan.source_plan.sha256", source_plan.get("sha256"), manifest_source_plan.get("sha256"))
    require_int_fields(
        "plan.source_plan",
        source_plan,
        {"entry_count": EXPECTED_PLAN_INDEX_COUNT, "timeout_sec": recovery_manifest.EXPECTED_SOURCE_TIMEOUT_SEC},
    )
    require_int_fields(
        "plan",
        plan,
        {
            "recovery_entry_count": EXPECTED_MISSING_COUNT,
            "recovery_attempt_count": EXPECTED_MISSING_COUNT,
            "dry_run_matrix_row_count": EXPECTED_MISSING_COUNT,
            "attempt_count": EXPECTED_MISSING_COUNT,
            "per_size_attempt_count": EXPECTED_MISSING_COUNT,
            "source_recovery_row_count": dry_run.EXPECTED_RECOVERY_ROW_COUNT,
            "prior_observed_plan_index_count": EXPECTED_PRIOR_OBSERVED_COUNT,
        },
    )
    require_equal("plan.prior_observed_treatment", plan.get("prior_observed_treatment"), "incomplete_prior_evidence_only")
    bool_true(plan.get("prior_observed_excluded_from_recovery"), "plan.prior_observed_excluded_from_recovery")

    non_goal_items = seq(
        obj(plan.get("generation_policy"), "plan.generation_policy").get("non_goals"),
        "plan.generation_policy.non_goals",
    )
    non_goals = {text(item, f"plan.generation_policy.non_goals[{offset}]") for offset, item in enumerate(non_goal_items)}
    missing_non_goals = sorted(REQUIRED_NON_GOALS - non_goals)
    if missing_non_goals:
        raise ValidationError(f"plan.generation_policy.non_goals missing {missing_non_goals}")


def validate_budget_contract(plan: Mapping[str, Any]) -> dict[str, Any]:
    expected_waves = EXPECTED_ACTION_TIMEOUT_SEC // EXPECTED_ROW_TIMEOUT_SEC
    expected_max_rows = EXPECTED_MAX_PARALLEL * expected_waves
    row_required = EXPECTED_PER_ATTEMPT_TIMEOUT_SEC + EXPECTED_ROW_OVERHEAD_SEC
    contract = obj(plan.get("operator_contract"), "plan.operator_contract")
    require_int_fields(
        "plan.operator_contract",
        contract,
        {
            "max_parallel": EXPECTED_MAX_PARALLEL,
            "attempts_per_matrix_row": EXPECTED_ATTEMPTS_PER_ROW,
            "per_attempt_timeout_sec": EXPECTED_PER_ATTEMPT_TIMEOUT_SEC,
            "benchmark_run_timeout_sec": EXPECTED_PER_ATTEMPT_TIMEOUT_SEC,
            "row_timeout_sec": EXPECTED_ROW_TIMEOUT_SEC,
            "row_overhead_sec": EXPECTED_ROW_OVERHEAD_SEC,
            "row_required_timeout_sec": row_required,
            "action_timeout_sec": EXPECTED_ACTION_TIMEOUT_SEC,
            "max_parallel_waves_per_batch": expected_waves,
            "max_rows_per_batch": expected_max_rows,
        },
    )
    require_equal("plan.operator_contract.row_timeout_contract_status", contract.get("row_timeout_contract_status"), "pass")
    require_equal("plan.operator_contract.action_timeout_contract_status", contract.get("action_timeout_contract_status"), "pass")

    template = obj(plan.get("dry_run_matrix_row_template"), "plan.dry_run_matrix_row_template")
    require_int_fields(
        "plan.dry_run_matrix_row_template",
        template,
        {
            "attempt_count": 1,
            "per_size_attempt_count": 1,
            "per_attempt_timeout_sec": EXPECTED_PER_ATTEMPT_TIMEOUT_SEC,
            "row_timeout_sec": EXPECTED_ROW_TIMEOUT_SEC,
            "row_overhead_sec": EXPECTED_ROW_OVERHEAD_SEC,
            "row_required_timeout_sec": row_required,
        },
    )
    bool_true(template.get("dry_run_only"), "plan.dry_run_matrix_row_template.dry_run_only")
    bool_true(template.get("recovery_only"), "plan.dry_run_matrix_row_template.recovery_only")

    summary = obj(plan.get("timeout_contract_summary"), "plan.timeout_contract_summary")
    require_int_fields(
        "plan.timeout_contract_summary",
        summary,
        {"row_violation_count": 0, "batch_violation_count": 0},
    )
    max_batch_required = integer(summary.get("max_batch_required_timeout_sec"), "plan.timeout_contract_summary.max_batch_required_timeout_sec")
    max_row_required = integer(summary.get("max_row_required_timeout_sec"), "plan.timeout_contract_summary.max_row_required_timeout_sec")
    min_action_slack = integer(summary.get("min_action_slack_sec"), "plan.timeout_contract_summary.min_action_slack_sec")
    min_row_slack = integer(summary.get("min_row_slack_sec"), "plan.timeout_contract_summary.min_row_slack_sec")
    if max_batch_required > EXPECTED_ACTION_TIMEOUT_SEC or max_row_required > EXPECTED_ROW_TIMEOUT_SEC:
        raise ValidationError("timeout_contract_summary exceeds the requested per-row-timeout row/action caps")
    if min_action_slack < 0 or min_row_slack < 0:
        raise ValidationError("timeout_contract_summary slack values must be non-negative")

    return {
        "max_parallel": EXPECTED_MAX_PARALLEL,
        "attempts_per_matrix_row": EXPECTED_ATTEMPTS_PER_ROW,
        "per_attempt_timeout_sec": EXPECTED_PER_ATTEMPT_TIMEOUT_SEC,
        "benchmark_run_timeout_sec": EXPECTED_PER_ATTEMPT_TIMEOUT_SEC,
        "row_timeout_sec": EXPECTED_ROW_TIMEOUT_SEC,
        "row_overhead_sec": EXPECTED_ROW_OVERHEAD_SEC,
        "row_required_timeout_sec": row_required,
        "action_timeout_sec": EXPECTED_ACTION_TIMEOUT_SEC,
        "max_parallel_waves_per_batch": expected_waves,
        "max_rows_per_batch": expected_max_rows,
        "row_violation_count": 0,
        "batch_violation_count": 0,
        "max_batch_required_timeout_sec": max_batch_required,
        "min_action_slack_sec": min_action_slack,
        "max_row_required_timeout_sec": max_row_required,
        "min_row_slack_sec": min_row_slack,
    }


def collect_batch_indices(plan: Mapping[str, Any]) -> tuple[list[int], list[int], list[int], set[str]]:
    expected_waves = EXPECTED_ACTION_TIMEOUT_SEC // EXPECTED_ROW_TIMEOUT_SEC
    expected_max_rows = EXPECTED_MAX_PARALLEL * expected_waves
    expected_batch_count = math.ceil(EXPECTED_MISSING_COUNT / expected_max_rows)
    require_int("plan.recovery_batch_count", plan.get("recovery_batch_count"), expected_batch_count)
    batches = [obj(batch, f"plan.batches[{offset}]") for offset, batch in enumerate(seq(plan.get("batches"), "plan.batches"))]
    if len(batches) != expected_batch_count:
        raise ValidationError(f"plan.batches length must be {expected_batch_count}, got {len(batches)}")

    all_indices: list[int] = []
    row_counts: list[int] = []
    required_seconds: list[int] = []
    workflow_benchmarks: set[str] = set()
    next_row_index = 1
    for offset, batch in enumerate(batches, start=1):
        label = f"plan.batches[{offset - 1}]"
        require_int(f"{label}.recovery_batch_index", batch.get("recovery_batch_index"), offset)
        bool_true(batch.get("dry_run_only"), f"{label}.dry_run_only")
        bool_true(batch.get("recovery_only"), f"{label}.recovery_only")
        indices = ranges(batch.get("plan_index_ranges"), f"{label}.plan_index_ranges")
        row_count = len(indices)
        if row_count < 1:
            raise ValidationError(f"{label}.plan_index_ranges must contain at least one recovery plan index")
        if indices != sorted(indices):
            raise ValidationError(f"{label}.plan_index_ranges must be in source-plan order")
        wave_count = math.ceil(row_count / EXPECTED_MAX_PARALLEL)
        required = wave_count * EXPECTED_ROW_TIMEOUT_SEC
        slack = EXPECTED_ACTION_TIMEOUT_SEC - required
        require_int_fields(
            label,
            batch,
            {
                "row_count": row_count,
                "attempt_count": row_count,
                "per_size_attempt_count": row_count,
                "dry_run_row_index_start": next_row_index,
                "dry_run_row_index_end": next_row_index + row_count - 1,
                "max_parallel": EXPECTED_MAX_PARALLEL,
                "parallel_wave_count": wave_count,
                "parallel_wave_slot_count": EXPECTED_MAX_PARALLEL,
                "per_attempt_timeout_sec": EXPECTED_PER_ATTEMPT_TIMEOUT_SEC,
                "row_timeout_sec": EXPECTED_ROW_TIMEOUT_SEC,
                "row_overhead_sec": EXPECTED_ROW_OVERHEAD_SEC,
                "row_required_timeout_sec": EXPECTED_PER_ATTEMPT_TIMEOUT_SEC + EXPECTED_ROW_OVERHEAD_SEC,
                "action_timeout_sec": EXPECTED_ACTION_TIMEOUT_SEC,
                "batch_required_timeout_sec": required,
                "action_slack_sec": slack,
                "plan_index_start": indices[0],
                "plan_index_end": indices[-1],
            },
        )
        if row_count > expected_max_rows or wave_count > expected_waves or slack < 0:
            raise ValidationError(f"{label} exceeds the per-row-timeout recovery batch budget")
        bool_true(batch.get("fits_action_timeout_contract"), f"{label}.fits_action_timeout_contract")
        for benchmark in seq(batch.get("workflow_benchmarks"), f"{label}.workflow_benchmarks"):
            workflow_benchmarks.add(text(benchmark, f"{label}.workflow_benchmarks[]"))
        next_row_index += row_count
        all_indices.extend(indices)
        row_counts.append(row_count)
        required_seconds.append(required)
    return all_indices, row_counts, required_seconds, workflow_benchmarks


def validate_coverage(
    plan: Mapping[str, Any],
    missing: Sequence[int],
    prior: Sequence[int],
    complete_rows: Sequence[Mapping[str, Any]],
    batch_indices: Sequence[int],
    workflow_benchmarks: set[str],
) -> dict[str, Any]:
    coverage = obj(plan.get("coverage"), "plan.coverage")
    require_int_fields(
        "plan.coverage",
        coverage,
        {
            "expected_plan_index_count": EXPECTED_PLAN_INDEX_COUNT,
            "recovery_missing_plan_index_count": EXPECTED_MISSING_COUNT,
            "dry_run_plan_index_count": EXPECTED_MISSING_COUNT,
            "prior_observed_plan_index_count": EXPECTED_PRIOR_OBSERVED_COUNT,
            "represented_plan_index_count_after_union": EXPECTED_PLAN_INDEX_COUNT,
            "duplicate_dry_run_plan_index_count": 0,
            "extra_dry_run_plan_index_count": 0,
            "missing_dry_run_plan_index_count": 0,
            "prior_observed_overlap_count": 0,
        },
    )
    coverage_missing = ranges(coverage.get("recovery_missing_plan_index_ranges"), "plan.coverage.recovery_missing_plan_index_ranges")
    coverage_dry = ranges(coverage.get("dry_run_plan_index_ranges"), "plan.coverage.dry_run_plan_index_ranges")
    coverage_prior = ranges(coverage.get("prior_observed_plan_index_ranges"), "plan.coverage.prior_observed_plan_index_ranges")
    if list(coverage_missing) != list(missing):
        raise ValidationError("plan.coverage.recovery_missing_plan_index_ranges must match manifest missing scope")
    if list(coverage_prior) != list(prior):
        raise ValidationError("plan.coverage.prior_observed_plan_index_ranges must match manifest prior observed evidence")
    if len(batch_indices) != len(set(batch_indices)):
        raise ValidationError("dry-run recovery batches must not duplicate plan indices")
    if list(batch_indices) != list(coverage_dry):
        raise ValidationError("dry-run batch plan indices must match plan.coverage.dry_run_plan_index_ranges")

    dry_set = set(batch_indices)
    missing_set = set(missing)
    prior_set = set(prior)
    overlap = sorted(dry_set & prior_set)
    if overlap:
        raise ValidationError(f"dry-run recovery plan overlaps prior observed incomplete evidence: {make_ranges(overlap)}")
    extra = sorted(dry_set - missing_set)
    absent = sorted(missing_set - dry_set)
    if extra or absent:
        raise ValidationError(
            "dry-run recovery plan must equal manifest missing_plan_index_ranges: "
            f"extra={make_ranges(extra)} missing={make_ranges(absent)}"
        )
    represented = dry_set | prior_set
    if represented != set(range(1, EXPECTED_PLAN_INDEX_COUNT + 1)):
        raise ValidationError("dry-run/prior union must represent exactly all 1416 source plan entries")

    complete_benchmarks = {text(row.get("benchmark"), "complete_prior_row.benchmark") for row in complete_rows}
    if complete_benchmarks & workflow_benchmarks:
        raise ValidationError(
            "complete prior logical rows must remain excluded from recovery batches: "
            f"{sorted(complete_benchmarks & workflow_benchmarks)}"
        )
    if seq(coverage.get("complete_prior_rows_excluded_from_recovery"), "plan.coverage.complete_prior_rows_excluded_from_recovery") != list(complete_rows):
        raise ValidationError("plan.coverage.complete_prior_rows_excluded_from_recovery must match manifest prior rows")

    return {
        "source_plan_index_count": EXPECTED_PLAN_INDEX_COUNT,
        "missing_plan_index_count": len(missing),
        "dry_run_plan_index_count": len(batch_indices),
        "dry_run_unique_plan_index_count": len(set(batch_indices)),
        "prior_observed_plan_index_count": len(prior),
        "prior_observed_unique_plan_index_count": len(set(prior)),
        "duplicate_dry_run_plan_index_count": len(batch_indices) - len(set(batch_indices)),
        "extra_dry_run_plan_index_count": len(extra),
        "missing_dry_run_plan_index_count": len(absent),
        "prior_observed_overlap_count": len(overlap),
        "represented_plan_index_count_after_union": len(represented),
        "dry_run_matches_manifest_missing_scope": True,
        "prior_observed_excluded_from_recovery": True,
        "prior_observed_treatment": "incomplete_prior_evidence_only",
        "not_complete_terminal_provenance": True,
        "dry_run_plan_index_ranges": make_ranges(batch_indices),
        "prior_observed_plan_index_ranges": make_ranges(prior),
        "complete_prior_rows_excluded_from_recovery": [dict(row) for row in complete_rows],
    }


def validate_recovery_dry_run_plan(plan_path: Path, manifest_path: Path) -> dict[str, Any]:
    manifest = load_json_object(manifest_path)
    plan = load_json_object(plan_path)
    missing, prior, complete_rows, run_id, run_conclusion = validate_manifest_scope(manifest)
    validate_plan_header(plan, manifest, manifest_path)
    budget = validate_budget_contract(plan)
    batch_indices, batch_row_counts, batch_required_seconds, workflow_benchmarks = collect_batch_indices(plan)
    coverage = validate_coverage(plan, missing, prior, complete_rows, batch_indices, workflow_benchmarks)
    require_rederived_plan(plan, manifest_path, plan_path)

    budget.update(
        {
            "recovery_batch_count": len(batch_row_counts),
            "batch_row_counts": batch_row_counts,
            "batch_required_timeout_sec": batch_required_seconds,
            "max_batch_row_count": max(batch_row_counts),
            "all_batches_fit_action_timeout": True,
            "all_batches_max_parallel_at_most_14": True,
        }
    )
    non_goals = seq(obj(plan.get("generation_policy"), "plan.generation_policy").get("non_goals"), "plan.generation_policy.non_goals")
    return {
        "schema_name": VALIDATION_SCHEMA,
        "schema_version": SCHEMA_VERSION,
        "validation_status": "pass",
        "validated": True,
        "dry_run": True,
        "pre_dispatch_static_validation": True,
        "plan": repo_relative(plan_path),
        "plan_sha256": file_sha256(plan_path),
        "recovery_manifest": repo_relative(manifest_path),
        "recovery_manifest_sha256": file_sha256(manifest_path),
        "source_run_id": run_id,
        "source_run_conclusion": run_conclusion,
        "coverage": coverage,
        "budget_contract": budget,
        "rederived_plan_matches_checked_in": True,
        "canonical_plan_matches_checked_in": True,
        "non_goals": [text(item, "plan.generation_policy.non_goals[]") for item in non_goals],
    }


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--dry-run-plan",
        default=str(dry_run.DEFAULT_OUTPUT),
        help="Path to the checked-in recovery dry-run plan JSON",
    )
    parser.add_argument(
        "--recovery-manifest",
        default=str(dry_run.DEFAULT_RECOVERY_MANIFEST),
        help="Path to the checked-in recovery manifest JSON",
    )
    return parser.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    args = parse_args(argv)
    try:
        report = validate_recovery_dry_run_plan(repo_path(args.dry_run_plan), repo_path(args.recovery_manifest))
    except ValidationError as exc:
        print("FAIL: MI300A recovery dry-run static validation rejected the plan.", file=sys.stderr)
        print(f"  - {exc}", file=sys.stderr)
        return 1
    print(json.dumps(report, indent=2) + "\n", end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
