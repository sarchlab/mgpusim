#!/usr/bin/env python3
"""Generate a static MI300A terminal-discovery recovery dry-run plan.

The dry-run plan is derived from the checked-in recovery manifest for
replacement run 25111815292.  It expands the 940 missing plan indices into
single-attempt recovery rows, then partitions those rows into reproducible
operator batches that satisfy the static 14-way / 600-second row / 3600-second
action-layer contract.  It does not run simulators, contact GitHub, dispatch or
mutate workflows, collect outcomes, start calibration, or regenerate
provenance/finishable/Tier artifacts.
"""

from __future__ import annotations

import argparse
import json
import math
import sys
from pathlib import Path
from typing import Any, Iterable, Mapping, Optional, Sequence

import generate_mi300a_terminal_recovery_manifest as recovery


REPO_ROOT = recovery.REPO_ROOT
DEFAULT_RECOVERY_MANIFEST = recovery.DEFAULT_OUTPUT
DEFAULT_OUTPUT = (
    REPO_ROOT
    / "benchmark-comparison"
    / "generated"
    / "mi300a_terminal_discovery_run_25111815292_recovery_dry_run_plan.json"
)

DRY_RUN_SCHEMA = "mgpusim.mi300a_terminal_discovery_recovery_dry_run_plan"
SCHEMA_VERSION = 1
DEFAULT_MAX_PARALLEL = 14
DEFAULT_ROW_TIMEOUT_SEC = 600
DEFAULT_PER_ATTEMPT_TIMEOUT_SEC = 600
DEFAULT_ACTION_TIMEOUT_SEC = 3600
DEFAULT_ROW_OVERHEAD_SEC = 0
CONTRACT_CAPS = {
    "max_parallel": DEFAULT_MAX_PARALLEL,
    "per_attempt_timeout_sec": DEFAULT_PER_ATTEMPT_TIMEOUT_SEC,
    "row_timeout_sec": DEFAULT_ROW_TIMEOUT_SEC,
    "action_timeout_sec": DEFAULT_ACTION_TIMEOUT_SEC,
}
EXPECTED_RECOVERY_ROW_COUNT = 79

ROW_TIMEOUT_FORMULA = "required_timeout_sec = attempts_per_row * per_attempt_timeout_sec + row_overhead_sec"
BATCH_TIMEOUT_FORMULA = "batch_required_timeout_sec = ceil(row_count / max_parallel) * row_timeout_sec"


class RecoveryDryRunPlanError(Exception):
    """Raised when recovery dry-run plan generation inputs are invalid."""


# ---------------------------------------------------------------------------
# Generic helpers
# ---------------------------------------------------------------------------


def repo_path(path_text: str) -> Path:
    return recovery.repo_path(path_text)


def repo_relative(path: Path) -> str:
    return recovery.repo_relative(path)


def json_text(data: Mapping[str, Any]) -> str:
    return json.dumps(data, indent=2) + "\n"


def write_json(path: Path, data: Mapping[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json_text(data), encoding="utf-8")


def load_json_object(path: Path) -> Mapping[str, Any]:
    try:
        return recovery.load_json_object(path)
    except recovery.RecoveryManifestError as exc:
        raise RecoveryDryRunPlanError(str(exc)) from exc


def file_sha256(path: Path) -> str:
    return recovery.file_sha256(path)


def as_int(value: Any, label: str) -> int:
    try:
        return recovery.as_int(value, label)
    except recovery.RecoveryManifestError as exc:
        raise RecoveryDryRunPlanError(str(exc)) from exc


def nonempty_str(value: Any, label: str) -> str:
    try:
        return recovery.nonempty_str(value, label)
    except recovery.RecoveryManifestError as exc:
        raise RecoveryDryRunPlanError(str(exc)) from exc


def object_at(value: Any, label: str) -> Mapping[str, Any]:
    if not isinstance(value, Mapping):
        raise RecoveryDryRunPlanError(f"{label} must be an object")
    return value


def list_at(value: Any, label: str) -> list[Any]:
    if not isinstance(value, list):
        raise RecoveryDryRunPlanError(f"{label} must be a list")
    return value


def positive_int_arg(value: str) -> int:
    try:
        parsed = int(value)
    except ValueError as exc:
        raise argparse.ArgumentTypeError(f"{value!r} is not an integer") from exc
    if parsed < 1:
        raise argparse.ArgumentTypeError("value must be a positive integer")
    return parsed


def nonnegative_int_arg(value: str) -> int:
    try:
        parsed = int(value)
    except ValueError as exc:
        raise argparse.ArgumentTypeError(f"{value!r} is not an integer") from exc
    if parsed < 0:
        raise argparse.ArgumentTypeError("value must be a non-negative integer")
    return parsed


def parse_plan_index_ranges(ranges: Sequence[str], label: str, max_plan_index: int) -> list[int]:
    try:
        return recovery.parse_plan_index_ranges(ranges, label, max_plan_index)
    except recovery.RecoveryManifestError as exc:
        raise RecoveryDryRunPlanError(str(exc)) from exc


def make_plan_index_ranges(values: Iterable[int]) -> list[str]:
    return recovery.make_plan_index_ranges(values)


def runnable_unit_id(plan_index: int) -> str:
    return recovery.runnable_unit_id(plan_index)


# ---------------------------------------------------------------------------
# Input loading and validation
# ---------------------------------------------------------------------------


def load_plan_entries(plan_path: Path) -> tuple[Mapping[str, Any], list[Mapping[str, Any]]]:
    try:
        return recovery.load_plan_entries(plan_path)
    except recovery.RecoveryManifestError as exc:
        raise RecoveryDryRunPlanError(str(exc)) from exc


def recovery_manifest_source_plan_path(manifest: Mapping[str, Any]) -> Path:
    source_plan = object_at(manifest.get("source_plan"), "manifest.source_plan")
    artifact = nonempty_str(source_plan.get("artifact"), "manifest.source_plan.artifact")
    return repo_path(artifact)


def reject_contract_cap_overrides(
    *,
    max_parallel: int,
    per_attempt_timeout_sec: int,
    row_timeout_sec: int,
    action_timeout_sec: int,
) -> None:
    requested_values = {
        "max_parallel": max_parallel,
        "per_attempt_timeout_sec": per_attempt_timeout_sec,
        "row_timeout_sec": row_timeout_sec,
        "action_timeout_sec": action_timeout_sec,
    }
    for label, requested in requested_values.items():
        cap = CONTRACT_CAPS[label]
        if requested > cap:
            raise RecoveryDryRunPlanError(
                f"{label}={requested} exceeds recovery dry-run operator contract cap {label}<={cap}"
            )


def validate_contract(
    *,
    max_parallel: int,
    per_attempt_timeout_sec: int,
    row_timeout_sec: int,
    action_timeout_sec: int,
    row_overhead_sec: int,
) -> tuple[int, int, int]:
    reject_contract_cap_overrides(
        max_parallel=max_parallel,
        per_attempt_timeout_sec=per_attempt_timeout_sec,
        row_timeout_sec=row_timeout_sec,
        action_timeout_sec=action_timeout_sec,
    )
    row_required_timeout_sec = per_attempt_timeout_sec + row_overhead_sec
    if row_required_timeout_sec > row_timeout_sec:
        raise RecoveryDryRunPlanError(
            "single-attempt recovery rows do not fit the row timeout: "
            f"per_attempt_timeout_sec={per_attempt_timeout_sec} + row_overhead_sec={row_overhead_sec} "
            f"> row_timeout_sec={row_timeout_sec}"
        )
    max_parallel_waves_per_batch = action_timeout_sec // row_timeout_sec
    if max_parallel_waves_per_batch < 1:
        raise RecoveryDryRunPlanError(
            "action timeout cannot fit even one row-timeout wave: "
            f"action_timeout_sec={action_timeout_sec}, row_timeout_sec={row_timeout_sec}"
        )
    max_rows_per_batch = max_parallel * max_parallel_waves_per_batch
    return row_required_timeout_sec, max_parallel_waves_per_batch, max_rows_per_batch


def manifest_plan_indices(
    manifest: Mapping[str, Any], *, label: str, key: str, expected_count: int
) -> list[int]:
    ranges = [
        nonempty_str(range_text, f"{label}.{key}[{offset}]")
        for offset, range_text in enumerate(list_at(manifest.get(key), f"{label}.{key}"))
    ]
    values = parse_plan_index_ranges(ranges, f"{label}.{key}", recovery.EXPECTED_ENTRY_COUNT)
    if len(values) != expected_count:
        raise RecoveryDryRunPlanError(
            f"{label}.{key} covers {len(values)} plan indices, expected {expected_count}"
        )
    if make_plan_index_ranges(values) != ranges:
        raise RecoveryDryRunPlanError(f"{label}.{key} ranges are not canonical")
    return values


def collect_recovery_scope(
    manifest: Mapping[str, Any]
) -> tuple[list[int], list[int], dict[int, Mapping[str, Any]]]:
    missing_values = manifest_plan_indices(
        manifest,
        label="manifest",
        key="missing_plan_index_ranges",
        expected_count=recovery.EXPECTED_MISSING_COUNT,
    )
    prior = object_at(
        manifest.get("prior_incomplete_evidence"),
        "manifest.prior_incomplete_evidence",
    )
    observed_ranges_label = "manifest.prior_incomplete_evidence.observed_plan_index_ranges"
    prior_ranges = [
        nonempty_str(range_text, f"{observed_ranges_label}[{offset}]")
        for offset, range_text in enumerate(
            list_at(prior.get("observed_plan_index_ranges"), observed_ranges_label)
        )
    ]
    prior_values = parse_plan_index_ranges(
        prior_ranges,
        "manifest.prior_incomplete_evidence.observed_plan_index_ranges",
        recovery.EXPECTED_ENTRY_COUNT,
    )
    if len(prior_values) != recovery.EXPECTED_OBSERVED_COUNT:
        raise RecoveryDryRunPlanError(
            "manifest.prior_incomplete_evidence.observed_plan_index_ranges covers "
            f"{len(prior_values)} plan indices, expected {recovery.EXPECTED_OBSERVED_COUNT}"
        )

    missing_set = set(missing_values)
    prior_set = set(prior_values)
    overlap = sorted(missing_set & prior_set)
    if overlap:
        raise RecoveryDryRunPlanError(
            "recovery scope overlaps prior observed plan indices: "
            f"{make_plan_index_ranges(overlap)}"
        )
    expected_all = set(range(1, recovery.EXPECTED_ENTRY_COUNT + 1))
    represented_all = missing_set | prior_set
    if represented_all != expected_all:
        missing_from_union = sorted(expected_all - represented_all)
        extra_in_union = sorted(represented_all - expected_all)
        raise RecoveryDryRunPlanError(
            "manifest recovery/prior union does not cover exactly the source plan: "
            f"missing={make_plan_index_ranges(missing_from_union)} extra={make_plan_index_ranges(extra_in_union)}"
        )

    rows = [
        object_at(row, f"manifest.recovery_rows[{offset}]")
        for offset, row in enumerate(list_at(manifest.get("recovery_rows"), "manifest.recovery_rows"))
    ]
    if len(rows) != EXPECTED_RECOVERY_ROW_COUNT:
        raise RecoveryDryRunPlanError(
            f"manifest.recovery_rows length must be {EXPECTED_RECOVERY_ROW_COUNT}, "
            f"got {len(rows)}"
        )

    row_by_plan_index: dict[int, Mapping[str, Any]] = {}
    ordered_from_rows: list[int] = []
    for row_offset, row in enumerate(rows, start=1):
        row_label = f"manifest.recovery_rows[{row_offset}]"
        declared_index = as_int(
            row.get("recovery_row_index"),
            f"{row_label}.recovery_row_index",
        )
        if declared_index != row_offset:
            raise RecoveryDryRunPlanError(
                f"{row_label}.recovery_row_index must be {row_offset}, "
                f"got {declared_index}"
            )
        row_ranges = [
            nonempty_str(
                range_text,
                f"{row_label}.missing_plan_index_ranges[{range_offset}]",
            )
            for range_offset, range_text in enumerate(
                list_at(
                    row.get("missing_plan_index_ranges"),
                    f"{row_label}.missing_plan_index_ranges",
                )
            )
        ]
        row_indices = parse_plan_index_ranges(
            row_ranges,
            f"{row_label}.missing_plan_index_ranges",
            recovery.EXPECTED_ENTRY_COUNT,
        )
        declared_count = as_int(
            row.get("recovery_attempt_count"),
            f"{row_label}.recovery_attempt_count",
        )
        if declared_count != len(row_indices):
            raise RecoveryDryRunPlanError(
                f"{row_label}.recovery_attempt_count must match missing ranges "
                f"({declared_count} != {len(row_indices)})"
            )
        for plan_index in row_indices:
            if plan_index not in missing_set:
                raise RecoveryDryRunPlanError(
                    f"{row_label} includes plan_index {plan_index}, "
                    "which is not in manifest.missing_plan_index_ranges"
                )
            if plan_index in row_by_plan_index:
                raise RecoveryDryRunPlanError(f"manifest.recovery_rows duplicate plan_index {plan_index}")
            row_by_plan_index[plan_index] = row
            ordered_from_rows.append(plan_index)

    # Recovery rows follow workflow_benchmark first-appearance order; within
    # each row plan indices are ascending. Workflow benchmarks need not be
    # physically contiguous in the source plan (
    # backprop_adjust_weights interleaved with backprop), so we assert on the
    # multiset rather than the concatenated sequence.
    if sorted(ordered_from_rows) != sorted(missing_set):
        raise RecoveryDryRunPlanError(
            "manifest.recovery_rows do not exactly cover the manifest "
            "missing plan indices"
        )
    return ordered_from_rows, prior_values, row_by_plan_index


def load_and_validate_inputs(
    recovery_manifest_path: Path,
) -> tuple[
    Mapping[str, Any],
    Mapping[str, Any],
    list[Mapping[str, Any]],
    list[int],
    list[int],
    dict[int, Mapping[str, Any]],
]:
    manifest = load_json_object(recovery_manifest_path)
    if manifest.get("schema_name") != recovery.MANIFEST_SCHEMA:
        raise RecoveryDryRunPlanError(
            f"{repo_relative(recovery_manifest_path)}.schema_name must be {recovery.MANIFEST_SCHEMA!r}, "
            f"got {manifest.get('schema_name')!r}"
        )
    if as_int(manifest.get("schema_version"), "manifest.schema_version") != recovery.SCHEMA_VERSION:
        raise RecoveryDryRunPlanError(
            f"{repo_relative(recovery_manifest_path)}.schema_version must be {recovery.SCHEMA_VERSION}"
        )
    recovery_entry_count = as_int(
        manifest.get("recovery_entry_count"),
        "manifest.recovery_entry_count",
    )
    if recovery_entry_count != recovery.EXPECTED_MISSING_COUNT:
        raise RecoveryDryRunPlanError(
            f"manifest.recovery_entry_count must be {recovery.EXPECTED_MISSING_COUNT}"
        )

    plan_path = recovery_manifest_source_plan_path(manifest)
    source_plan = object_at(manifest.get("source_plan"), "manifest.source_plan")
    expected_sha = nonempty_str(source_plan.get("sha256"), "manifest.source_plan.sha256")
    actual_sha = file_sha256(plan_path)
    if actual_sha != expected_sha:
        raise RecoveryDryRunPlanError(
            f"{repo_relative(plan_path)} sha256 {actual_sha} does not match "
            f"recovery manifest source_plan.sha256 {expected_sha}"
        )
    plan, entries = load_plan_entries(plan_path)
    ordered_missing, prior_observed, row_by_plan_index = collect_recovery_scope(manifest)
    return manifest, plan, entries, ordered_missing, prior_observed, row_by_plan_index


# ---------------------------------------------------------------------------
# Plan construction
# ---------------------------------------------------------------------------


def ordered_unique(values: Iterable[str]) -> list[str]:
    seen: set[str] = set()
    ordered: list[str] = []
    for value in values:
        if value in seen:
            continue
        seen.add(value)
        ordered.append(value)
    return ordered



def build_dry_run_rows(
    entries_by_plan_index: Mapping[int, Mapping[str, Any]],
    ordered_missing: Sequence[int],
    row_by_plan_index: Mapping[int, Mapping[str, Any]],
    *,
    max_parallel: int,
    max_rows_per_batch: int,
    per_attempt_timeout_sec: int,
    row_timeout_sec: int,
    row_overhead_sec: int,
    row_required_timeout_sec: int,
) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    for row_index, plan_index in enumerate(ordered_missing, start=1):
        entry = entries_by_plan_index.get(plan_index)
        if entry is None:
            raise RecoveryDryRunPlanError(f"source plan does not contain missing plan_index {plan_index}")
        source_recovery_row = row_by_plan_index[plan_index]
        source_timeout_sec = as_int(entry.get("timeout_sec"), f"plan_entry[{plan_index}].timeout_sec")
        if source_timeout_sec != recovery.EXPECTED_SOURCE_TIMEOUT_SEC:
            raise RecoveryDryRunPlanError(
                f"plan_entry[{plan_index}].timeout_sec must be {recovery.EXPECTED_SOURCE_TIMEOUT_SEC}, "
                f"got {source_timeout_sec}"
            )
        batch_index = ((row_index - 1) // max_rows_per_batch) + 1
        batch_row_index = ((row_index - 1) % max_rows_per_batch) + 1
        parallel_wave_index = ((batch_row_index - 1) // max_parallel) + 1
        parallel_wave_slot = ((batch_row_index - 1) % max_parallel) + 1
        plan_entry_label = f"plan_entry[{plan_index}]"
        source_recovery_label = f"source_recovery_row[{plan_index}]"
        workflow_benchmark = nonempty_str(
            entry.get("workflow_benchmark"),
            f"{plan_entry_label}.workflow_benchmark",
        )
        source_ranges_label = f"{source_recovery_label}.missing_plan_index_ranges"
        source_ranges = [
            nonempty_str(range_text, f"{source_ranges_label}[{offset}]")
            for offset, range_text in enumerate(
                list_at(source_recovery_row.get("missing_plan_index_ranges"), source_ranges_label)
            )
        ]
        row_slack_sec = row_timeout_sec - row_required_timeout_sec
        rows.append(
            {
                "dry_run_row_index": row_index,
                "recovery_batch_index": batch_index,
                "batch_row_index": batch_row_index,
                "parallel_wave_index": parallel_wave_index,
                "parallel_wave_slot": parallel_wave_slot,
                "matrix_row_id": f"mi300a-terminal-discovery-recovery-plan-{plan_index:04d}",
                "job_name": f"Recovery Dry Run: plan {plan_index:04d} {workflow_benchmark}",
                "benchmark": workflow_benchmark,
                "workflow_benchmark": workflow_benchmark,
                "workflow_benchmark_name": nonempty_str(
                    entry.get("workflow_benchmark_name"),
                    f"{plan_entry_label}.workflow_benchmark_name",
                ),
                "workflow_job": nonempty_str(
                    entry.get("workflow_job"),
                    f"{plan_entry_label}.workflow_job",
                ),
                "tier": as_int(entry.get("tier"), f"{plan_entry_label}.tier"),
                "tier_name": nonempty_str(
                    entry.get("tier_name"),
                    f"{plan_entry_label}.tier_name",
                ),
                "plan_index": plan_index,
                "plan_index_ranges": [str(plan_index)],
                "runnable_unit_id": runnable_unit_id(plan_index),
                "attempt_count": 1,
                "per_size_attempt_count": 1,
                "source_plan_timeout_sec": source_timeout_sec,
                "per_attempt_timeout_sec": per_attempt_timeout_sec,
                "row_timeout_sec": row_timeout_sec,
                "row_overhead_sec": row_overhead_sec,
                "required_timeout_sec": row_required_timeout_sec,
                "row_slack_sec": row_slack_sec,
                "fits_timeout_contract": row_slack_sec >= 0,
                "source_recovery_row_index": as_int(
                    source_recovery_row.get("recovery_row_index"),
                    f"{source_recovery_label}.recovery_row_index",
                ),
                "source_recovery_matrix_row_id": nonempty_str(
                    source_recovery_row.get("matrix_row_id"),
                    f"{source_recovery_label}.matrix_row_id",
                ),
                "source_recovery_missing_plan_index_ranges": source_ranges,
                "plan_benchmark": nonempty_str(entry.get("benchmark"), f"{plan_entry_label}.benchmark"),
                "problem_size": nonempty_str(entry.get("problem_size"), f"{plan_entry_label}.problem_size"),
                "hardware_kernel_name": nonempty_str(
                    entry.get("hardware_kernel_name"), f"{plan_entry_label}.hardware_kernel_name"
                ),
                "hardware_problem_size": nonempty_str(
                    entry.get("hardware_problem_size"), f"{plan_entry_label}.hardware_problem_size"
                ),
                "sizes": nonempty_str(entry.get("sizes"), f"{plan_entry_label}.sizes"),
                "size_token": nonempty_str(entry.get("size_token"), f"{plan_entry_label}.size_token"),
                "size_label": nonempty_str(entry.get("size_label"), f"{plan_entry_label}.size_label"),
                "flag_template": nonempty_str(entry.get("flag_template"), f"{plan_entry_label}.flag_template"),
                "size_label_template": nonempty_str(
                    entry.get("size_label_template"),
                    f"{plan_entry_label}.size_label_template",
                ),
                "artifact_id": nonempty_str(entry.get("artifact_id"), f"{plan_entry_label}.artifact_id"),
                "job_id": nonempty_str(entry.get("job_id"), f"{plan_entry_label}.job_id"),
                "extra_flags": entry.get("extra_flags"),
                "gomemlimit": entry.get("gomemlimit"),
                "gogc": entry.get("gogc"),
                "kernel_to_workflow_mapping_source": nonempty_str(
                    entry.get("kernel_to_workflow_mapping_source"),
                    f"plan_entry[{plan_index}].kernel_to_workflow_mapping_source",
                ),
                "kernel_to_workflow_mapping_reason": entry.get("kernel_to_workflow_mapping_reason"),
                "dry_run_only": True,
                "recovery_only": True,
            }
        )
    return rows


def build_batches(
    rows: Sequence[Mapping[str, Any]],
    *,
    max_parallel: int,
    row_timeout_sec: int,
    action_timeout_sec: int,
    per_attempt_timeout_sec: int,
    row_overhead_sec: int,
    row_required_timeout_sec: int,
    max_rows_per_batch: int,
) -> list[dict[str, Any]]:
    batches: list[dict[str, Any]] = []
    for start in range(0, len(rows), max_rows_per_batch):
        batch_rows = [dict(row) for row in rows[start : start + max_rows_per_batch]]
        batch_index = len(batches) + 1
        plan_indices = [
            as_int(row.get("plan_index"), f"batch[{batch_index}].row.plan_index")
            for row in batch_rows
        ]
        wave_count = math.ceil(len(batch_rows) / max_parallel)
        batch_required_timeout_sec = wave_count * row_timeout_sec
        action_slack_sec = action_timeout_sec - batch_required_timeout_sec
        if action_slack_sec < 0:
            raise RecoveryDryRunPlanError(
                f"batch {batch_index} requires {batch_required_timeout_sec}s, "
                f"exceeding action_timeout_sec={action_timeout_sec}"
            )
        workflow_benchmarks = ordered_unique(
            nonempty_str(
                row.get("workflow_benchmark"),
                f"batch[{batch_index}].row.workflow_benchmark",
            )
            for row in batch_rows
        )
        first_row = batch_rows[0]
        last_row = batch_rows[-1]
        batches.append(
            {
                "recovery_batch_index": batch_index,
                "batch_id": f"mi300a-terminal-discovery-recovery-dry-run-batch-{batch_index:02d}",
                "job_name": f"Recovery Dry Run Batch {batch_index:02d} ({len(batch_rows)} single-attempt rows)",
                "dry_run_only": True,
                "recovery_only": True,
                "row_count": len(batch_rows),
                "attempt_count": len(batch_rows),
                "per_size_attempt_count": len(batch_rows),
                "dry_run_row_index_start": as_int(first_row.get("dry_run_row_index"), "first_row.dry_run_row_index"),
                "dry_run_row_index_end": as_int(last_row.get("dry_run_row_index"), "last_row.dry_run_row_index"),
                "max_parallel": max_parallel,
                "parallel_wave_count": wave_count,
                "parallel_wave_slot_count": max_parallel,
                "per_attempt_timeout_sec": per_attempt_timeout_sec,
                "row_timeout_sec": row_timeout_sec,
                "row_overhead_sec": row_overhead_sec,
                "row_required_timeout_sec": row_required_timeout_sec,
                "action_timeout_sec": action_timeout_sec,
                "batch_required_timeout_sec": batch_required_timeout_sec,
                "action_slack_sec": action_slack_sec,
                "fits_action_timeout_contract": True,
                "plan_index_start": plan_indices[0],
                "plan_index_end": plan_indices[-1],
                "plan_index_ranges": make_plan_index_ranges(plan_indices),
                "first_matrix_row_id": nonempty_str(first_row.get("matrix_row_id"), "first_row.matrix_row_id"),
                "last_matrix_row_id": nonempty_str(last_row.get("matrix_row_id"), "last_row.matrix_row_id"),
                "first_runnable_unit_id": runnable_unit_id(plan_indices[0]),
                "last_runnable_unit_id": runnable_unit_id(plan_indices[-1]),
                "benchmark_count": len(workflow_benchmarks),
                "workflow_benchmarks": workflow_benchmarks,
            }
        )
    return batches


def summarize_coverage(
    *,
    planned_indices: Sequence[int],
    missing_indices: Sequence[int],
    prior_observed_indices: Sequence[int],
    manifest: Mapping[str, Any],
) -> dict[str, Any]:
    planned_set = set(planned_indices)
    missing_set = set(missing_indices)
    prior_set = set(prior_observed_indices)
    duplicate_count = len(planned_indices) - len(planned_set)
    extra = sorted(planned_set - missing_set)
    missing = sorted(missing_set - planned_set)
    prior_overlap = sorted(planned_set & prior_set)
    represented_after_union = planned_set | prior_set
    return {
        "expected_plan_index_count": recovery.EXPECTED_ENTRY_COUNT,
        "source_plan_index_ranges": [f"1-{recovery.EXPECTED_ENTRY_COUNT}"],
        "recovery_missing_plan_index_count": len(missing_indices),
        "recovery_missing_plan_index_ranges": list_at(
            manifest.get("missing_plan_index_ranges"), "manifest.missing_plan_index_ranges"
        ),
        "dry_run_plan_index_count": len(planned_indices),
        "dry_run_plan_index_ranges": make_plan_index_ranges(planned_indices),
        "prior_observed_plan_index_count": len(prior_observed_indices),
        "prior_observed_plan_index_ranges": make_plan_index_ranges(prior_observed_indices),
        "represented_plan_index_count_after_union": len(represented_after_union),
        "duplicate_dry_run_plan_index_count": duplicate_count,
        "extra_dry_run_plan_index_count": len(extra),
        "extra_dry_run_plan_index_ranges": make_plan_index_ranges(extra),
        "missing_dry_run_plan_index_count": len(missing),
        "missing_dry_run_plan_index_ranges": make_plan_index_ranges(missing),
        "prior_observed_overlap_count": len(prior_overlap),
        "prior_observed_overlap_ranges": make_plan_index_ranges(prior_overlap),
        "complete_prior_rows_excluded_from_recovery": list_at(
            object_at(manifest.get("prior_incomplete_evidence"), "manifest.prior_incomplete_evidence").get(
                "complete_logical_rows_excluded_from_recovery"
            ),
            "manifest.prior_incomplete_evidence.complete_logical_rows_excluded_from_recovery",
        ),
    }


def build_plan(
    recovery_manifest_path: Path,
    *,
    max_parallel: int = DEFAULT_MAX_PARALLEL,
    per_attempt_timeout_sec: int = DEFAULT_PER_ATTEMPT_TIMEOUT_SEC,
    row_timeout_sec: int = DEFAULT_ROW_TIMEOUT_SEC,
    action_timeout_sec: int = DEFAULT_ACTION_TIMEOUT_SEC,
    row_overhead_sec: int = DEFAULT_ROW_OVERHEAD_SEC,
) -> Mapping[str, Any]:
    row_required_timeout_sec, max_parallel_waves_per_batch, max_rows_per_batch = validate_contract(
        max_parallel=max_parallel,
        per_attempt_timeout_sec=per_attempt_timeout_sec,
        row_timeout_sec=row_timeout_sec,
        action_timeout_sec=action_timeout_sec,
        row_overhead_sec=row_overhead_sec,
    )
    (
        manifest,
        plan,
        entries,
        ordered_missing,
        prior_observed,
        row_by_plan_index,
    ) = load_and_validate_inputs(recovery_manifest_path)
    entries_by_plan_index = {as_int(entry.get("plan_index"), "plan_entry.plan_index"): entry for entry in entries}
    rows = build_dry_run_rows(
        entries_by_plan_index,
        ordered_missing,
        row_by_plan_index,
        max_parallel=max_parallel,
        max_rows_per_batch=max_rows_per_batch,
        per_attempt_timeout_sec=per_attempt_timeout_sec,
        row_timeout_sec=row_timeout_sec,
        row_overhead_sec=row_overhead_sec,
        row_required_timeout_sec=row_required_timeout_sec,
    )
    batches = build_batches(
        rows,
        max_parallel=max_parallel,
        row_timeout_sec=row_timeout_sec,
        action_timeout_sec=action_timeout_sec,
        per_attempt_timeout_sec=per_attempt_timeout_sec,
        row_overhead_sec=row_overhead_sec,
        row_required_timeout_sec=row_required_timeout_sec,
        max_rows_per_batch=max_rows_per_batch,
    )
    planned_indices = [as_int(row.get("plan_index"), "dry_run_row.plan_index") for row in rows]
    coverage = summarize_coverage(
        planned_indices=planned_indices,
        missing_indices=ordered_missing,
        prior_observed_indices=prior_observed,
        manifest=manifest,
    )
    if any(
        as_int(coverage[key], f"coverage.{key}") != 0
        for key in [
            "duplicate_dry_run_plan_index_count",
            "extra_dry_run_plan_index_count",
            "missing_dry_run_plan_index_count",
            "prior_observed_overlap_count",
        ]
    ):
        raise RecoveryDryRunPlanError("dry-run plan coverage does not exactly match recovery-only missing scope")
    represented_count = as_int(
        coverage["represented_plan_index_count_after_union"],
        "coverage.represented_plan_index_count_after_union",
    )
    if represented_count != recovery.EXPECTED_ENTRY_COUNT:
        raise RecoveryDryRunPlanError("dry-run/prior union does not represent all 1416 source plan entries")

    source_evidence = object_at(manifest.get("source_evidence"), "manifest.source_evidence")
    source_plan = object_at(manifest.get("source_plan"), "manifest.source_plan")
    prior = object_at(manifest.get("prior_incomplete_evidence"), "manifest.prior_incomplete_evidence")
    max_batch_required_timeout_sec = max(
        as_int(batch.get("batch_required_timeout_sec"), "batch.batch_required_timeout_sec") for batch in batches
    )
    min_action_slack_sec = min(as_int(batch.get("action_slack_sec"), "batch.action_slack_sec") for batch in batches)
    return {
        "schema_name": DRY_RUN_SCHEMA,
        "schema_version": SCHEMA_VERSION,
        "dry_run": True,
        "recovery_only": True,
        "source_recovery_manifest": {
            "artifact": repo_relative(recovery_manifest_path),
            "sha256": file_sha256(recovery_manifest_path),
            "schema_name": manifest.get("schema_name"),
            "schema_version": as_int(manifest.get("schema_version"), "manifest.schema_version"),
            "run_id": nonempty_str(source_evidence.get("run_id"), "manifest.source_evidence.run_id"),
            "run_url": nonempty_str(source_evidence.get("run_url"), "manifest.source_evidence.run_url"),
            "head_branch": nonempty_str(source_evidence.get("head_branch"), "manifest.source_evidence.head_branch"),
            "head_sha": nonempty_str(source_evidence.get("head_sha"), "manifest.source_evidence.head_sha"),
            "run_conclusion": nonempty_str(
                source_evidence.get("run_conclusion"),
                "manifest.source_evidence.run_conclusion",
            ),
            "recovery_entry_count": as_int(manifest.get("recovery_entry_count"), "manifest.recovery_entry_count"),
            "recovery_row_count": as_int(manifest.get("recovery_row_count"), "manifest.recovery_row_count"),
        },
        "source_plan": {
            "artifact": nonempty_str(source_plan.get("artifact"), "manifest.source_plan.artifact"),
            "sha256": nonempty_str(source_plan.get("sha256"), "manifest.source_plan.sha256"),
            "schema_name": plan.get("schema_name"),
            "schema_version": as_int(plan.get("schema_version"), "plan.schema_version"),
            "entry_count": recovery.EXPECTED_ENTRY_COUNT,
            "timeout_sec": recovery.EXPECTED_SOURCE_TIMEOUT_SEC,
            "tier_counts": dict(object_at(plan.get("tier_counts"), "plan.tier_counts")),
        },
        "source_plan_lookup_fields": list_at(
            manifest.get("source_plan_lookup_fields"),
            "manifest.source_plan_lookup_fields",
        ),
        "operator_contract": {
            "requested_contract": "14-way parallelism, 600-second row/run limit, 3600-second action-layer limit",
            "max_parallel": max_parallel,
            "attempts_per_matrix_row": 1,
            "per_attempt_timeout_sec": per_attempt_timeout_sec,
            "benchmark_run_timeout_sec": per_attempt_timeout_sec,
            "row_timeout_sec": row_timeout_sec,
            "row_overhead_sec": row_overhead_sec,
            "row_required_timeout_sec": row_required_timeout_sec,
            "action_timeout_sec": action_timeout_sec,
            "max_parallel_waves_per_batch": max_parallel_waves_per_batch,
            "max_rows_per_batch": max_rows_per_batch,
            "row_timeout_formula": ROW_TIMEOUT_FORMULA,
            "batch_timeout_formula": BATCH_TIMEOUT_FORMULA,
            "row_timeout_contract_status": "pass",
            "action_timeout_contract_status": "pass",
        },
        "generation_policy": {
            "scope_policy": (
                "include only the 940 plan indices missing from run 25111815292 "
                "in the recovery manifest"
            ),
            "ordering_policy": (
                "source_plan_order_from_recovery_manifest_rows_then_"
                "one_single-attempt_row_per_missing_plan_index"
            ),
            "batching_policy": (
                "contiguous dry-run rows; each batch has at most "
                "max_parallel * floor(action_timeout_sec / row_timeout_sec) rows"
            ),
            "matrix_row_policy": "one dry-run matrix row per missing plan index",
            "attempt_policy": (
                "each dry-run matrix row is exactly one terminal-discovery "
                "attempt resolved from the source plan"
            ),
            "non_goals": [
                "no simulator execution or simulation campaign",
                "no workflow dispatch/cancel/rerun",
                "no calibration start or parameter sweep",
                "no provenance, finishable manifest, or Tier artifact regeneration",
                "no terminal finishability completion claim",
                "no permanent visible Actions surface change",
            ],
        },
        "dry_run_matrix_row_template": {
            "matrix_row_id": "mi300a-terminal-discovery-recovery-plan-{plan_index:04d}",
            "runnable_unit_id": "mi300a-terminal-discovery-plan-{plan_index:04d}",
            "source_plan_lookup": "source_plan.artifact entries[plan_index - 1]",
            "source_recovery_manifest_lookup": "source_recovery_manifest.artifact recovery_rows containing plan_index",
            "attempt_count": 1,
            "per_size_attempt_count": 1,
            "per_attempt_timeout_sec": per_attempt_timeout_sec,
            "row_timeout_sec": row_timeout_sec,
            "row_overhead_sec": row_overhead_sec,
            "row_required_timeout_sec": row_required_timeout_sec,
            "dry_run_only": True,
            "recovery_only": True,
        },
        "recovery_entry_count": len(rows),
        "recovery_attempt_count": len(rows),
        "dry_run_matrix_row_count": len(rows),
        "attempt_count": len(rows),
        "per_size_attempt_count": len(rows),
        "recovery_benchmark_count": len(
            ordered_unique(
                nonempty_str(
                    row.get("workflow_benchmark"),
                    "dry_run_row.workflow_benchmark",
                )
                for row in rows
            )
        ),
        "source_recovery_row_count": as_int(manifest.get("recovery_row_count"), "manifest.recovery_row_count"),
        "recovery_batch_count": len(batches),
        "prior_observed_plan_index_count": as_int(
            prior.get("observed_unique_plan_index_count"),
            "manifest.prior_incomplete_evidence.observed_unique_plan_index_count",
        ),
        "prior_observed_treatment": nonempty_str(
            prior.get("treatment"),
            "manifest.prior_incomplete_evidence.treatment",
        ),
        "prior_observed_excluded_from_recovery": True,
        "timeout_contract_summary": {
            "row_violation_count": 0,
            "batch_violation_count": 0,
            "max_batch_required_timeout_sec": max_batch_required_timeout_sec,
            "min_action_slack_sec": min_action_slack_sec,
            "max_row_required_timeout_sec": row_required_timeout_sec,
            "min_row_slack_sec": row_timeout_sec - row_required_timeout_sec,
        },
        "coverage": coverage,
        "batches": batches,
    }


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--recovery-manifest",
        default=str(DEFAULT_RECOVERY_MANIFEST),
        help="Checked-in recovery manifest for run 25111815292",
    )
    parser.add_argument("--output", default=str(DEFAULT_OUTPUT), help="Recovery dry-run plan output path")
    parser.add_argument(
        "--max-parallel",
        type=positive_int_arg,
        default=DEFAULT_MAX_PARALLEL,
        help=f"Maximum recovery rows running concurrently (default: {DEFAULT_MAX_PARALLEL})",
    )
    parser.add_argument(
        "--per-attempt-timeout-sec",
        type=positive_int_arg,
        default=DEFAULT_PER_ATTEMPT_TIMEOUT_SEC,
        help=f"Per-attempt benchmark run timeout in seconds (default: {DEFAULT_PER_ATTEMPT_TIMEOUT_SEC})",
    )
    parser.add_argument(
        "--row-timeout-sec",
        type=positive_int_arg,
        default=DEFAULT_ROW_TIMEOUT_SEC,
        help=f"Row/job timeout in seconds (default: {DEFAULT_ROW_TIMEOUT_SEC})",
    )
    parser.add_argument(
        "--action-timeout-sec",
        type=positive_int_arg,
        default=DEFAULT_ACTION_TIMEOUT_SEC,
        help=f"Overall action-layer timeout in seconds for one recovery batch (default: {DEFAULT_ACTION_TIMEOUT_SEC})",
    )
    parser.add_argument(
        "--row-overhead-sec",
        type=nonnegative_int_arg,
        default=DEFAULT_ROW_OVERHEAD_SEC,
        help=f"Explicit per-row non-simulator overhead budget in seconds (default: {DEFAULT_ROW_OVERHEAD_SEC})",
    )
    parser.add_argument(
        "--check",
        action="store_true",
        help="Validate that --output already matches the generated dry-run plan without rewriting it",
    )
    return parser.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    args = parse_args(argv)
    recovery_manifest_path = repo_path(args.recovery_manifest)
    output_path = repo_path(args.output)
    try:
        plan = build_plan(
            recovery_manifest_path,
            max_parallel=args.max_parallel,
            per_attempt_timeout_sec=args.per_attempt_timeout_sec,
            row_timeout_sec=args.row_timeout_sec,
            action_timeout_sec=args.action_timeout_sec,
            row_overhead_sec=args.row_overhead_sec,
        )
        output_text = json_text(plan)
        if args.check:
            try:
                current_text = output_path.read_text(encoding="utf-8")
            except FileNotFoundError as exc:
                raise RecoveryDryRunPlanError(f"{repo_relative(output_path)}: output file is missing") from exc
            if current_text != output_text:
                raise RecoveryDryRunPlanError(f"{repo_relative(output_path)} is not up to date; rerun this script")
            print(
                "OK: recovery dry-run plan is up to date "
                f"attempts={plan['recovery_attempt_count']} rows={plan['dry_run_matrix_row_count']} "
                f"batches={plan['recovery_batch_count']} max_parallel={plan['operator_contract']['max_parallel']}"
            )
        else:
            write_json(output_path, plan)
            print(
                f"Wrote {repo_relative(output_path)}: "
                f"attempts={plan['recovery_attempt_count']} rows={plan['dry_run_matrix_row_count']} "
                f"batches={plan['recovery_batch_count']} max_parallel={plan['operator_contract']['max_parallel']} "
                f"row_timeout_sec={plan['operator_contract']['row_timeout_sec']} "
                f"action_timeout_sec={plan['operator_contract']['action_timeout_sec']}"
            )
    except RecoveryDryRunPlanError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
