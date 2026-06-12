#!/usr/bin/env python3
"""Preflight the bounded MI300A terminal-discovery recovery operator path.

This command consumes the checked-in recovery dry-run plan for cancelled run
25111815292 and emits a deterministic operator preflight/dry-run report.  It is
read-only: it does not dispatch, cancel, or rerun workflows; it does not run
simulators; and it does not regenerate provenance, finishable, or Tier artifacts.
"""

from __future__ import annotations

import argparse
from collections import Counter
import json
from pathlib import Path
import subprocess
import sys
from typing import Any, Mapping, Optional, Sequence

import generate_mi300a_terminal_recovery_dry_run_plan as dry_run
import validate_mi300a_terminal_recovery_dry_run_plan as dry_run_validator


OPERATOR_SCHEMA = "mgpusim.mi300a_terminal_discovery_recovery_operator_preflight"
SCHEMA_VERSION = 1
EXPECTED_OPERATOR_BRANCH: Optional[str] = None
EXPECTED_SOURCE_RUN_ID = str(dry_run_validator.EXPECTED_RUN_ID)
EXPECTED_DRY_RUN_PLAN_SHA256 = "247ae3055306393c0345ed8de105be2e79d52caea54f2b78f9c38f6221e48932"
EXPECTED_RECOVERY_MANIFEST_SHA256 = "d3e6ce707d6d9699e4906dd7386adbee4318886ca7c07264085f79cb30df729c"
EXPECTED_BATCH_COUNT = 12
EXPECTED_SOURCE_PLAN_INDEX_COUNT = dry_run_validator.EXPECTED_PLAN_INDEX_COUNT
EXPECTED_MISSING_PLAN_INDEX_COUNT = dry_run_validator.EXPECTED_MISSING_COUNT
EXPECTED_PRIOR_OBSERVED_COUNT = dry_run_validator.EXPECTED_PRIOR_OBSERVED_COUNT

CONTRACT_CAPS = {
    "max_parallel": dry_run_validator.EXPECTED_MAX_PARALLEL,
    "per_attempt_timeout_sec": dry_run_validator.EXPECTED_PER_ATTEMPT_TIMEOUT_SEC,
    "row_timeout_sec": dry_run_validator.EXPECTED_ROW_TIMEOUT_SEC,
    "action_timeout_sec": dry_run_validator.EXPECTED_ACTION_TIMEOUT_SEC,
}

FORBIDDEN_SIDE_EFFECTS = [
    "workflow_dispatch",
    "workflow_cancel",
    "workflow_rerun",
    "simulator_execution",
    "calibration_start",
    "provenance_regeneration",
    "finishable_manifest_regeneration",
    "tier_artifact_regeneration",
    "github_workflow_file_modification",
]


class OperatorPreflightError(Exception):
    """Raised when the guarded recovery operator preflight cannot proceed."""


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
        raise OperatorPreflightError(f"{repo_relative(path)}: file not found") from exc


def load_json_object(path: Path) -> Mapping[str, Any]:
    try:
        return dry_run_validator.load_json_object(path)
    except dry_run_validator.ValidationError as exc:
        raise OperatorPreflightError(str(exc)) from exc


def as_int(value: Any, label: str) -> int:
    try:
        return dry_run.as_int(value, label)
    except dry_run.RecoveryDryRunPlanError as exc:
        raise OperatorPreflightError(str(exc)) from exc


def nonempty_str(value: Any, label: str) -> str:
    try:
        return dry_run.nonempty_str(value, label)
    except dry_run.RecoveryDryRunPlanError as exc:
        raise OperatorPreflightError(str(exc)) from exc


def list_at(value: Any, label: str) -> list[Any]:
    if not isinstance(value, list):
        raise OperatorPreflightError(f"{label} must be a list")
    return value


def object_at(value: Any, label: str) -> Mapping[str, Any]:
    if not isinstance(value, Mapping):
        raise OperatorPreflightError(f"{label} must be an object")
    return value


def positive_int_arg(value: str) -> int:
    return dry_run.positive_int_arg(value)


def parse_plan_index_ranges(ranges: Sequence[str], label: str) -> list[int]:
    try:
        return dry_run.parse_plan_index_ranges(ranges, label, EXPECTED_SOURCE_PLAN_INDEX_COUNT)
    except dry_run.RecoveryDryRunPlanError as exc:
        raise OperatorPreflightError(str(exc)) from exc


def make_plan_index_ranges(values: Sequence[int]) -> list[str]:
    return dry_run.make_plan_index_ranges(values)


# ---------------------------------------------------------------------------
# Guard validation
# ---------------------------------------------------------------------------


def validate_static_plan(plan_path: Path, manifest_path: Path) -> dict[str, Any]:
    try:
        return dry_run_validator.validate_recovery_dry_run_plan(plan_path, manifest_path)
    except dry_run_validator.ValidationError as exc:
        raise OperatorPreflightError(f"recovery dry-run plan validation failed: {exc}") from exc


def require_expected_hashes(plan_path: Path, manifest_path: Path) -> tuple[str, str]:
    plan_sha = file_sha256(plan_path)
    if plan_sha != EXPECTED_DRY_RUN_PLAN_SHA256:
        raise OperatorPreflightError(
            "stale or regenerated recovery dry-run plan sha256 mismatch: "
            f"expected {EXPECTED_DRY_RUN_PLAN_SHA256}, got {plan_sha}"
        )

    manifest_sha = file_sha256(manifest_path)
    if manifest_sha != EXPECTED_RECOVERY_MANIFEST_SHA256:
        raise OperatorPreflightError(
            "stale or regenerated recovery manifest sha256 mismatch: "
            f"expected {EXPECTED_RECOVERY_MANIFEST_SHA256}, got {manifest_sha}"
        )
    return plan_sha, manifest_sha


def git_output(args: Sequence[str], label: str) -> str:
    try:
        result = subprocess.run(
            ["git", "-C", str(dry_run.REPO_ROOT), *args],
            text=True,
            capture_output=True,
            check=False,
            timeout=10,
        )
    except FileNotFoundError as exc:
        raise OperatorPreflightError(
            "unable to read operator git provenance: git executable not found"
        ) from exc
    except subprocess.TimeoutExpired as exc:
        raise OperatorPreflightError(f"unable to read {label} from git: command timed out") from exc

    if result.returncode != 0:
        details = (result.stderr or result.stdout).strip()
        if details:
            details = " ".join(details.split())
            raise OperatorPreflightError(f"unable to read {label} from git: {details}")
        raise OperatorPreflightError(f"unable to read {label} from git: exit {result.returncode}")

    value = result.stdout.strip()
    if not value:
        raise OperatorPreflightError(f"unable to read {label} from git: empty output")
    return value


def normalize_expected_operator_branch(expected_operator_branch: Optional[str]) -> Optional[str]:
    if expected_operator_branch is None:
        return None

    normalized = expected_operator_branch.strip()
    if not normalized:
        raise OperatorPreflightError("expected operator branch must not be empty")
    return normalized


def normalize_expected_head_sha(expected_head_sha: Optional[str]) -> Optional[str]:
    if expected_head_sha is None:
        return None

    normalized = expected_head_sha.strip().lower()
    if not normalized:
        raise OperatorPreflightError("expected operator HEAD SHA must not be empty")
    if len(normalized) != 40 or any(char not in "0123456789abcdef" for char in normalized):
        raise OperatorPreflightError(
            "expected operator HEAD SHA must be a 40-character hexadecimal git commit SHA"
        )
    return normalized


def expected_operator_branch_values(
    supplied_expected_branch: Optional[str],
) -> list[tuple[str, str]]:
    expectations: list[tuple[str, str]] = []
    configured_branch = normalize_expected_operator_branch(EXPECTED_OPERATOR_BRANCH)
    supplied_branch = normalize_expected_operator_branch(supplied_expected_branch)

    if configured_branch is not None:
        expectations.append(("configured", configured_branch))
    if supplied_branch is not None and supplied_branch != configured_branch:
        expectations.append(("supplied", supplied_branch))
    return expectations


def read_operator_git_provenance(
    expected_head_sha: Optional[str] = None,
    *,
    expected_operator_branch: Optional[str] = None,
) -> dict[str, Optional[str]]:
    current_branch = git_output(["branch", "--show-current"], "current git branch")
    actual_head_sha = git_output(["rev-parse", "--verify", "HEAD"], "current git HEAD SHA").lower()
    if len(actual_head_sha) != 40 or any(
        char not in "0123456789abcdef" for char in actual_head_sha
    ):
        raise OperatorPreflightError(
            f"current git HEAD SHA is not a 40-character hexadecimal commit SHA: {actual_head_sha}"
        )

    branch_expectations = expected_operator_branch_values(expected_operator_branch)
    expected_head_sha = normalize_expected_head_sha(expected_head_sha)
    for expectation_source, branch in branch_expectations:
        if current_branch != branch:
            raise OperatorPreflightError(
                f"operator branch mismatch ({expectation_source} expectation): "
                f"expected {branch}, got {current_branch}"
            )
    if expected_head_sha is not None and actual_head_sha != expected_head_sha:
        raise OperatorPreflightError(
            "operator HEAD SHA mismatch: "
            f"expected {expected_head_sha}, got {actual_head_sha}"
        )

    expected_operator_branch_value = (
        branch_expectations[-1][1] if branch_expectations else None
    )
    return {
        "expected_operator_branch": expected_operator_branch_value,
        "current_branch": current_branch,
        "operator_head_sha": actual_head_sha,
        "expected_operator_head_sha": expected_head_sha,
    }


def requested_contract_from_args(args: argparse.Namespace) -> dict[str, int]:
    return {
        "max_parallel": args.max_parallel,
        "per_attempt_timeout_sec": args.per_attempt_timeout_sec,
        "row_timeout_sec": args.row_timeout_sec,
        "action_timeout_sec": args.action_timeout_sec,
    }


def validate_requested_contract(requested: Mapping[str, int]) -> dict[str, int]:
    for field, cap in CONTRACT_CAPS.items():
        value = as_int(requested.get(field), field)
        if value > cap:
            raise OperatorPreflightError(
                f"{field}={value} exceeds bounded recovery operator contract cap {field}<={cap}"
            )

    for field, expected in CONTRACT_CAPS.items():
        value = as_int(requested.get(field), field)
        if value != expected:
            raise OperatorPreflightError(
                f"{field}={value} does not match checked-in recovery dry-run plan contract {field}={expected}"
            )
    return dict(requested)


def require_core_validation_report(validation: Mapping[str, Any]) -> None:
    coverage = object_at(validation.get("coverage"), "validation.coverage")
    budget = object_at(validation.get("budget_contract"), "validation.budget_contract")
    expected_counts = {
        "coverage.source_plan_index_count": (
            coverage.get("source_plan_index_count"),
            EXPECTED_SOURCE_PLAN_INDEX_COUNT,
        ),
        "coverage.missing_plan_index_count": (
            coverage.get("missing_plan_index_count"),
            EXPECTED_MISSING_PLAN_INDEX_COUNT,
        ),
        "coverage.dry_run_plan_index_count": (
            coverage.get("dry_run_plan_index_count"),
            EXPECTED_MISSING_PLAN_INDEX_COUNT,
        ),
        "coverage.dry_run_unique_plan_index_count": (
            coverage.get("dry_run_unique_plan_index_count"),
            EXPECTED_MISSING_PLAN_INDEX_COUNT,
        ),
        "coverage.prior_observed_plan_index_count": (
            coverage.get("prior_observed_plan_index_count"),
            EXPECTED_PRIOR_OBSERVED_COUNT,
        ),
        "coverage.prior_observed_unique_plan_index_count": (
            coverage.get("prior_observed_unique_plan_index_count"),
            EXPECTED_PRIOR_OBSERVED_COUNT,
        ),
        "budget.recovery_batch_count": (budget.get("recovery_batch_count"), EXPECTED_BATCH_COUNT),
        "budget.max_parallel": (budget.get("max_parallel"), CONTRACT_CAPS["max_parallel"]),
        "budget.attempts_per_matrix_row": (budget.get("attempts_per_matrix_row"), 1),
        "budget.per_attempt_timeout_sec": (
            budget.get("per_attempt_timeout_sec"),
            CONTRACT_CAPS["per_attempt_timeout_sec"],
        ),
        "budget.row_timeout_sec": (budget.get("row_timeout_sec"), CONTRACT_CAPS["row_timeout_sec"]),
        "budget.action_timeout_sec": (
            budget.get("action_timeout_sec"),
            CONTRACT_CAPS["action_timeout_sec"],
        ),
    }
    for label, (actual, expected) in expected_counts.items():
        parsed = as_int(actual, label)
        if parsed != expected:
            raise OperatorPreflightError(f"{label} must be {expected}, got {parsed}")

    if coverage.get("prior_observed_treatment") != "incomplete_prior_evidence_only":
        raise OperatorPreflightError(
            "coverage.prior_observed_treatment must remain incomplete_prior_evidence_only"
        )
    if validation.get("canonical_plan_matches_checked_in") is not True:
        raise OperatorPreflightError("canonical dry-run plan check did not pass")
    if validation.get("rederived_plan_matches_checked_in") is not True:
        raise OperatorPreflightError("rederived dry-run plan check did not pass")


# ---------------------------------------------------------------------------
# Batch selection
# ---------------------------------------------------------------------------


def parse_batch_token(token: str, label: str) -> list[int] | str:
    stripped = token.strip()
    if not stripped:
        raise OperatorPreflightError(f"{label} contains an empty batch selection")
    if stripped == "all":
        return "all"
    if "-" in stripped and not stripped.startswith("-"):
        start_text, end_text = stripped.split("-", 1)
        try:
            start = int(start_text)
            end = int(end_text)
        except ValueError as exc:
            raise OperatorPreflightError(f"{label} has invalid batch range {stripped!r}") from exc
        if start > end:
            raise OperatorPreflightError(f"{label} has descending batch range {stripped!r}")
        return list(range(start, end + 1))
    try:
        return [int(stripped)]
    except ValueError as exc:
        raise OperatorPreflightError(f"{label} has invalid batch selection {stripped!r}") from exc


def expand_batch_specs(batch_specs: Sequence[str]) -> tuple[list[int], bool]:
    if not batch_specs:
        return [], True

    expanded: list[int] = []
    saw_all = False
    for spec_index, spec in enumerate(batch_specs):
        for part_index, token in enumerate(spec.split(",")):
            parsed = parse_batch_token(token, f"batch selection {spec_index + 1}.{part_index + 1}")
            if parsed == "all":
                saw_all = True
            else:
                expanded.extend(parsed)

    if saw_all and expanded:
        raise OperatorPreflightError("batch selection 'all' cannot be combined with explicit batch indices")
    return expanded, saw_all


def select_batch_indices(batch_specs: Sequence[str], batch_count: int) -> list[int]:
    expanded, saw_all = expand_batch_specs(batch_specs)
    if saw_all:
        return list(range(1, batch_count + 1))

    if not expanded:
        return list(range(1, batch_count + 1))

    duplicate_values = sorted(value for value, count in Counter(expanded).items() if count > 1)
    if duplicate_values:
        raise OperatorPreflightError(
            "duplicate recovery batch selections: "
            f"{', '.join(str(value) for value in duplicate_values)}"
        )

    out_of_scope = sorted(value for value in expanded if value < 1 or value > batch_count)
    if out_of_scope:
        raise OperatorPreflightError(
            "out-of-scope recovery batch selections: "
            f"{', '.join(str(value) for value in out_of_scope)}; valid batch range is 1-{batch_count}"
        )
    return sorted(expanded)


def batch_map(plan: Mapping[str, Any]) -> dict[int, Mapping[str, Any]]:
    raw_batches = list_at(plan.get("batches"), "plan.batches")
    batches = [object_at(batch, f"plan.batches[{offset}]") for offset, batch in enumerate(raw_batches)]
    if len(batches) != EXPECTED_BATCH_COUNT:
        raise OperatorPreflightError(f"plan.batches length must be {EXPECTED_BATCH_COUNT}, got {len(batches)}")

    by_index: dict[int, Mapping[str, Any]] = {}
    for offset, batch in enumerate(batches):
        batch_index = as_int(batch.get("recovery_batch_index"), f"plan.batches[{offset}].recovery_batch_index")
        if batch_index in by_index:
            raise OperatorPreflightError(f"plan.batches duplicate recovery_batch_index {batch_index}")
        by_index[batch_index] = batch
    expected = set(range(1, EXPECTED_BATCH_COUNT + 1))
    if set(by_index) != expected:
        raise OperatorPreflightError(
            "plan.batches must enumerate exactly recovery_batch_index 1-"
            f"{EXPECTED_BATCH_COUNT}"
        )
    return by_index


def batch_plan_indices(batch: Mapping[str, Any], label: str) -> list[int]:
    range_texts = [
        nonempty_str(item, f"{label}.plan_index_ranges[{offset}]")
        for offset, item in enumerate(list_at(batch.get("plan_index_ranges"), f"{label}.plan_index_ranges"))
    ]
    indices = parse_plan_index_ranges(range_texts, f"{label}.plan_index_ranges")
    row_count = as_int(batch.get("row_count"), f"{label}.row_count")
    if len(indices) != row_count:
        raise OperatorPreflightError(f"{label}.plan_index_ranges count must match row_count")
    return indices


def summarize_selected_batches(
    plan: Mapping[str, Any],
    selected_indices: Sequence[int],
) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    batches_by_index = batch_map(plan)
    selected_batches: list[dict[str, Any]] = []
    selected_plan_indices: list[int] = []
    selected_row_count = 0
    selected_attempt_count = 0

    for batch_index in selected_indices:
        batch = batches_by_index[batch_index]
        label = f"plan.batches[{batch_index - 1}]"
        indices = batch_plan_indices(batch, label)
        row_count = as_int(batch.get("row_count"), f"{label}.row_count")
        attempt_count = as_int(batch.get("attempt_count"), f"{label}.attempt_count")
        per_size_attempt_count = as_int(batch.get("per_size_attempt_count"), f"{label}.per_size_attempt_count")
        if attempt_count != row_count or per_size_attempt_count != row_count:
            raise OperatorPreflightError(f"{label} must keep one attempt per selected recovery row")

        selected_row_count += row_count
        selected_attempt_count += attempt_count
        selected_plan_indices.extend(indices)
        selected_batches.append(
            {
                "recovery_batch_index": batch_index,
                "batch_id": nonempty_str(batch.get("batch_id"), f"{label}.batch_id"),
                "dry_run_only": batch.get("dry_run_only") is True,
                "recovery_only": batch.get("recovery_only") is True,
                "row_count": row_count,
                "attempt_count": attempt_count,
                "per_size_attempt_count": per_size_attempt_count,
                "plan_index_count": len(indices),
                "dry_run_row_index_start": as_int(
                    batch.get("dry_run_row_index_start"),
                    f"{label}.dry_run_row_index_start",
                ),
                "dry_run_row_index_end": as_int(
                    batch.get("dry_run_row_index_end"),
                    f"{label}.dry_run_row_index_end",
                ),
                "max_parallel": as_int(batch.get("max_parallel"), f"{label}.max_parallel"),
                "parallel_wave_count": as_int(
                    batch.get("parallel_wave_count"),
                    f"{label}.parallel_wave_count",
                ),
                "per_attempt_timeout_sec": as_int(
                    batch.get("per_attempt_timeout_sec"),
                    f"{label}.per_attempt_timeout_sec",
                ),
                "row_timeout_sec": as_int(batch.get("row_timeout_sec"), f"{label}.row_timeout_sec"),
                "action_timeout_sec": as_int(
                    batch.get("action_timeout_sec"),
                    f"{label}.action_timeout_sec",
                ),
                "batch_required_timeout_sec": as_int(
                    batch.get("batch_required_timeout_sec"),
                    f"{label}.batch_required_timeout_sec",
                ),
                "action_slack_sec": as_int(batch.get("action_slack_sec"), f"{label}.action_slack_sec"),
                "plan_index_ranges": make_plan_index_ranges(indices),
                "first_runnable_unit_id": nonempty_str(
                    batch.get("first_runnable_unit_id"),
                    f"{label}.first_runnable_unit_id",
                ),
                "last_runnable_unit_id": nonempty_str(
                    batch.get("last_runnable_unit_id"),
                    f"{label}.last_runnable_unit_id",
                ),
                "would_dispatch_workflow": False,
                "would_run_simulator": False,
            }
        )

    duplicate_plan_index_count = len(selected_plan_indices) - len(set(selected_plan_indices))
    if duplicate_plan_index_count:
        duplicate_indices = sorted(
            value for value, count in Counter(selected_plan_indices).items() if count > 1
        )
        raise OperatorPreflightError(
            "selected recovery batches duplicate plan indices: "
            f"{', '.join(str(value) for value in duplicate_indices[:20])}"
        )

    summary = {
        "selected_batch_indices": list(selected_indices),
        "selected_batch_count": len(selected_indices),
        "available_batch_count": EXPECTED_BATCH_COUNT,
        "all_batches_selected": len(selected_indices) == EXPECTED_BATCH_COUNT,
        "selected_row_count": selected_row_count,
        "selected_attempt_count": selected_attempt_count,
        "selected_plan_index_count": len(selected_plan_indices),
        "selected_unique_plan_index_count": len(set(selected_plan_indices)),
        "duplicate_selected_plan_index_count": duplicate_plan_index_count,
        "selected_plan_index_ranges": make_plan_index_ranges(selected_plan_indices),
    }
    return selected_batches, summary


# ---------------------------------------------------------------------------
# Report construction
# ---------------------------------------------------------------------------


def build_contract_report(requested_contract: Mapping[str, int], budget: Mapping[str, Any]) -> dict[str, Any]:
    return {
        "requested_max_parallel": requested_contract["max_parallel"],
        "requested_per_attempt_timeout_sec": requested_contract["per_attempt_timeout_sec"],
        "requested_row_timeout_sec": requested_contract["row_timeout_sec"],
        "requested_action_timeout_sec": requested_contract["action_timeout_sec"],
        "max_parallel": as_int(budget.get("max_parallel"), "budget.max_parallel"),
        "attempts_per_matrix_row": as_int(
            budget.get("attempts_per_matrix_row"),
            "budget.attempts_per_matrix_row",
        ),
        "per_attempt_timeout_sec": as_int(
            budget.get("per_attempt_timeout_sec"),
            "budget.per_attempt_timeout_sec",
        ),
        "benchmark_run_timeout_sec": as_int(
            budget.get("benchmark_run_timeout_sec"),
            "budget.benchmark_run_timeout_sec",
        ),
        "row_timeout_sec": as_int(budget.get("row_timeout_sec"), "budget.row_timeout_sec"),
        "row_cap_sec": CONTRACT_CAPS["row_timeout_sec"],
        "row_required_timeout_sec": as_int(
            budget.get("row_required_timeout_sec"),
            "budget.row_required_timeout_sec",
        ),
        "action_timeout_sec": as_int(
            budget.get("action_timeout_sec"),
            "budget.action_timeout_sec",
        ),
        "action_cap_sec": CONTRACT_CAPS["action_timeout_sec"],
        "max_parallel_waves_per_batch": as_int(
            budget.get("max_parallel_waves_per_batch"),
            "budget.max_parallel_waves_per_batch",
        ),
        "max_rows_per_batch": as_int(
            budget.get("max_rows_per_batch"),
            "budget.max_rows_per_batch",
        ),
        "row_violation_count": as_int(
            budget.get("row_violation_count"),
            "budget.row_violation_count",
        ),
        "batch_violation_count": as_int(
            budget.get("batch_violation_count"),
            "budget.batch_violation_count",
        ),
        "max_batch_required_timeout_sec": as_int(
            budget.get("max_batch_required_timeout_sec"),
            "budget.max_batch_required_timeout_sec",
        ),
        "min_action_slack_sec": as_int(
            budget.get("min_action_slack_sec"),
            "budget.min_action_slack_sec",
        ),
        "recovery_batch_count": as_int(
            budget.get("recovery_batch_count"),
            "budget.recovery_batch_count",
        ),
        "batch_row_counts": list_at(budget.get("batch_row_counts"), "budget.batch_row_counts"),
        "batch_required_timeout_sec": list_at(
            budget.get("batch_required_timeout_sec"),
            "budget.batch_required_timeout_sec",
        ),
        "all_batches_fit_action_timeout": budget.get("all_batches_fit_action_timeout") is True,
        "all_batches_max_parallel_at_most_14": budget.get("all_batches_max_parallel_at_most_14") is True,
    }


def build_coverage_report(coverage: Mapping[str, Any], budget: Mapping[str, Any]) -> dict[str, Any]:
    return {
        "source_plan_index_count": as_int(
            coverage.get("source_plan_index_count"),
            "coverage.source_plan_index_count",
        ),
        "missing_plan_index_count": as_int(
            coverage.get("missing_plan_index_count"),
            "coverage.missing_plan_index_count",
        ),
        "dry_run_plan_index_count": as_int(
            coverage.get("dry_run_plan_index_count"),
            "coverage.dry_run_plan_index_count",
        ),
        "dry_run_unique_plan_index_count": as_int(
            coverage.get("dry_run_unique_plan_index_count"),
            "coverage.dry_run_unique_plan_index_count",
        ),
        "prior_observed_plan_index_count": as_int(
            coverage.get("prior_observed_plan_index_count"),
            "coverage.prior_observed_plan_index_count",
        ),
        "prior_observed_unique_plan_index_count": as_int(
            coverage.get("prior_observed_unique_plan_index_count"),
            "coverage.prior_observed_unique_plan_index_count",
        ),
        "prior_observed_treatment": nonempty_str(
            coverage.get("prior_observed_treatment"),
            "coverage.prior_observed_treatment",
        ),
        "prior_observed_excluded_from_recovery": coverage.get("prior_observed_excluded_from_recovery") is True,
        "not_complete_terminal_provenance": coverage.get("not_complete_terminal_provenance") is True,
        "duplicate_dry_run_plan_index_count": as_int(
            coverage.get("duplicate_dry_run_plan_index_count"),
            "coverage.duplicate_dry_run_plan_index_count",
        ),
        "extra_dry_run_plan_index_count": as_int(
            coverage.get("extra_dry_run_plan_index_count"),
            "coverage.extra_dry_run_plan_index_count",
        ),
        "missing_dry_run_plan_index_count": as_int(
            coverage.get("missing_dry_run_plan_index_count"),
            "coverage.missing_dry_run_plan_index_count",
        ),
        "prior_observed_overlap_count": as_int(
            coverage.get("prior_observed_overlap_count"),
            "coverage.prior_observed_overlap_count",
        ),
        "represented_plan_index_count_after_union": as_int(
            coverage.get("represented_plan_index_count_after_union"),
            "coverage.represented_plan_index_count_after_union",
        ),
        "recovery_batch_count": as_int(
            budget.get("recovery_batch_count"),
            "budget.recovery_batch_count",
        ),
        "dry_run_matches_manifest_missing_scope": coverage.get("dry_run_matches_manifest_missing_scope") is True,
        "dry_run_plan_index_ranges": list_at(
            coverage.get("dry_run_plan_index_ranges"),
            "coverage.dry_run_plan_index_ranges",
        ),
        "prior_observed_plan_index_ranges": list_at(
            coverage.get("prior_observed_plan_index_ranges"),
            "coverage.prior_observed_plan_index_ranges",
        ),
        "complete_prior_rows_excluded_from_recovery": list_at(
            coverage.get("complete_prior_rows_excluded_from_recovery"),
            "coverage.complete_prior_rows_excluded_from_recovery",
        ),
    }


def build_operator_report(
    *,
    mode: str,
    plan_path: Path,
    manifest_path: Path,
    batch_specs: Sequence[str],
    requested_contract: Mapping[str, int],
    expected_operator_branch: Optional[str] = None,
    expected_head_sha: Optional[str] = None,
) -> dict[str, Any]:
    operator_git = read_operator_git_provenance(
        expected_operator_branch=expected_operator_branch,
        expected_head_sha=expected_head_sha,
    )
    validated_contract = validate_requested_contract(requested_contract)
    validation = validate_static_plan(plan_path, manifest_path)
    require_core_validation_report(validation)
    plan_sha, manifest_sha = require_expected_hashes(plan_path, manifest_path)
    plan = load_json_object(plan_path)

    coverage = object_at(validation.get("coverage"), "validation.coverage")
    budget = object_at(validation.get("budget_contract"), "validation.budget_contract")
    selected_indices = select_batch_indices(batch_specs, EXPECTED_BATCH_COUNT)
    selected_batches, selection_summary = summarize_selected_batches(plan, selected_indices)

    source_recovery_manifest = object_at(
        plan.get("source_recovery_manifest"),
        "plan.source_recovery_manifest",
    )
    source_run_id = nonempty_str(
        source_recovery_manifest.get("run_id"),
        "plan.source_recovery_manifest.run_id",
    )
    if source_run_id != EXPECTED_SOURCE_RUN_ID:
        raise OperatorPreflightError(
            f"plan.source_recovery_manifest.run_id must be {EXPECTED_SOURCE_RUN_ID}, got {source_run_id}"
        )
    requested_selection = list(batch_specs) if batch_specs else ["all"]
    return {
        "schema_name": OPERATOR_SCHEMA,
        "schema_version": SCHEMA_VERSION,
        "operator_branch": operator_git["current_branch"],
        "expected_operator_branch": operator_git["expected_operator_branch"],
        "current_branch": operator_git["current_branch"],
        "operator_head_sha": operator_git["operator_head_sha"],
        "current_head_sha": operator_git["operator_head_sha"],
        "expected_operator_head_sha": operator_git["expected_operator_head_sha"],
        "operator_git": operator_git,
        "mode": mode,
        "status": "pass",
        "validated": True,
        "preflight": True,
        "dry_run": True,
        "read_only": True,
        "dispatch_performed": False,
        "simulator_execution_performed": False,
        "artifact_regeneration_performed": False,
        "workflow_modification_performed": False,
        "plan": {
            "artifact": repo_relative(plan_path),
            "sha256": plan_sha,
            "expected_sha256": EXPECTED_DRY_RUN_PLAN_SHA256,
            "schema_name": plan.get("schema_name"),
            "schema_version": as_int(plan.get("schema_version"), "plan.schema_version"),
            "canonical_matches_checked_in": True,
            "rederived_matches_checked_in": True,
        },
        "recovery_manifest": {
            "artifact": repo_relative(manifest_path),
            "sha256": manifest_sha,
            "expected_sha256": EXPECTED_RECOVERY_MANIFEST_SHA256,
        },
        "source_run": {
            "run_id": source_run_id,
            "run_conclusion": nonempty_str(
                source_recovery_manifest.get("run_conclusion"),
                "plan.source_recovery_manifest.run_conclusion",
            ),
            "head_branch": nonempty_str(
                source_recovery_manifest.get("head_branch"),
                "plan.source_recovery_manifest.head_branch",
            ),
            "head_sha": nonempty_str(
                source_recovery_manifest.get("head_sha"),
                "plan.source_recovery_manifest.head_sha",
            ),
        },
        "coverage": build_coverage_report(coverage, budget),
        "operator_contract": build_contract_report(validated_contract, budget),
        "batch_selection": {
            "requested": requested_selection,
            **selection_summary,
        },
        "selected_batches": selected_batches,
        "guardrails": {
            "pinned_plan_sha256": True,
            "pinned_recovery_manifest_sha256": True,
            "canonical_plan_matches_checked_in": True,
            "rederived_plan_matches_checked_in": True,
            "bounded_contract_matches_checked_in": True,
            "actual_operator_branch_verified": True,
            "actual_operator_head_sha_recorded": True,
            "expected_operator_head_sha_verified": (
                operator_git["expected_operator_head_sha"] is not None
            ),
            "duplicate_batch_selection_rejected": True,
            "out_of_scope_batch_selection_rejected": True,
            "over_cap_overrides_rejected": True,
            "forbidden_side_effects": FORBIDDEN_SIDE_EFFECTS,
        },
        "non_goals": list_at(validation.get("non_goals"), "validation.non_goals"),
    }


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "command",
        nargs="?",
        choices=["preflight", "dry-run"],
        default="preflight",
        help="Report mode. Both modes are read-only and perform no dispatch (default: preflight).",
    )
    parser.add_argument(
        "--dry-run-plan",
        default=str(dry_run.DEFAULT_OUTPUT),
        help="Checked-in run 25111815292 recovery dry-run plan JSON",
    )
    parser.add_argument(
        "--recovery-manifest",
        default=str(dry_run.DEFAULT_RECOVERY_MANIFEST),
        help="Checked-in run 25111815292 recovery manifest JSON",
    )
    parser.add_argument(
        "--batch",
        dest="batch_specs",
        action="append",
        default=[],
        metavar="N_OR_RANGE",
        help="Recovery batch selection; may be repeated. Ranges use START-END. Default: all.",
    )
    parser.add_argument(
        "--batches",
        dest="batch_specs",
        action="append",
        metavar="SPEC",
        help="Comma-separated recovery batch selections. Use 'all' or ranges like 1-3,5.",
    )
    parser.add_argument(
        "--all-batches",
        action="store_true",
        help="Explicitly select all 12 checked-in recovery batches.",
    )
    parser.add_argument(
        "--max-parallel",
        type=positive_int_arg,
        default=CONTRACT_CAPS["max_parallel"],
        help="Requested maximum parallel recovery rows; cannot exceed or differ from checked-in cap 14.",
    )
    parser.add_argument(
        "--per-attempt-timeout-sec",
        type=positive_int_arg,
        default=CONTRACT_CAPS["per_attempt_timeout_sec"],
        help="Requested per-attempt cap; cannot exceed or differ from checked-in cap 600.",
    )
    parser.add_argument(
        "--row-timeout-sec",
        type=positive_int_arg,
        default=CONTRACT_CAPS["row_timeout_sec"],
        help="Requested row cap; cannot exceed or differ from checked-in cap 600.",
    )
    parser.add_argument(
        "--action-timeout-sec",
        type=positive_int_arg,
        default=CONTRACT_CAPS["action_timeout_sec"],
        help="Requested action-layer cap; cannot exceed or differ from checked-in cap 3600.",
    )
    parser.add_argument(
        "--expected-branch",
        default=None,
        help=(
            "Optional expected current git branch; "
            "preflight fails if it does not match the repo."
        ),
    )
    parser.add_argument(
        "--expected-head-sha",
        default=None,
        help=(
            "Optional expected current git HEAD SHA; "
            "preflight fails if it does not match the repo."
        ),
    )
    args = parser.parse_args(argv)
    if args.all_batches:
        if args.batch_specs:
            parser.error("--all-batches cannot be combined with --batch/--batches")
        args.batch_specs = ["all"]
    return args


def main(argv: Optional[Sequence[str]] = None) -> int:
    args = parse_args(argv)
    try:
        report = build_operator_report(
            mode=args.command,
            plan_path=repo_path(args.dry_run_plan),
            manifest_path=repo_path(args.recovery_manifest),
            batch_specs=args.batch_specs,
            requested_contract=requested_contract_from_args(args),
            expected_operator_branch=args.expected_branch,
            expected_head_sha=args.expected_head_sha,
        )
    except OperatorPreflightError as exc:
        print("FAIL: MI300A recovery operator preflight rejected the request.", file=sys.stderr)
        print(f"  - {exc}", file=sys.stderr)
        return 1
    print(json.dumps(report, indent=2) + "\n", end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
