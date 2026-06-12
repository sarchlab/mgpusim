#!/usr/bin/env python3
"""Validate the checked-in MI300A finishable-size manifest.

This is a static guard for MI300A finishable-size evidence.  It consumes the
checked-in 1416-entry problem-size discovery plan, compact discovery outcome
provenance, generated finishable-size manifest, compact summary, and Tier 1/2
matrix views.  It does not query GitHub or run simulations.

The validator intentionally re-derives the expected manifest in memory from the
plan/provenance evidence, compares the checked-in JSON outputs byte-for-byte at
the object level, and performs focused coverage/provenance checks with clear
failure messages for common bad states: duplicate/missing plan entries, omitted
hotspot/48, non-success rows marked eligible, incomplete terminal outcome rows,
and invented outcomes for non-terminal or missing jobs.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any, Mapping, Optional, Sequence

import generate_mi300a_finishable_size_manifest as generator


VALIDATION_SCHEMA = "mgpusim.mi300a_finishable_size_manifest_validation"
SCHEMA_VERSION = 1


class ValidationError(Exception):
    """Raised when a checked-in manifest artifact violates the finishable-size manifest contract."""


# ---------------------------------------------------------------------------
# JSON/path helpers
# ---------------------------------------------------------------------------


def repo_path(path_text: str) -> Path:
    path = Path(path_text)
    return path if path.is_absolute() else generator.REPO_ROOT / path


def repo_relative(path: Path) -> str:
    return generator.repo_relative(path)


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


def object_at(value: Any, label: str) -> Mapping[str, Any]:
    if not isinstance(value, Mapping):
        raise ValidationError(f"{label} must be an object")
    return value


def list_at(value: Any, label: str) -> list[Any]:
    if not isinstance(value, list):
        raise ValidationError(f"{label} must be a list")
    return value


def as_int(value: Any, label: str) -> int:
    try:
        return generator.as_int(value, label)
    except generator.ManifestError as exc:
        raise ValidationError(str(exc)) from exc


def range_set(raw_ranges: Any, label: str, plan_count: int) -> set[int]:
    try:
        return generator.parse_plan_index_ranges(raw_ranges, label, plan_count)
    except generator.ManifestError as exc:
        raise ValidationError(str(exc)) from exc


def make_ranges(values: set[int] | list[int]) -> list[str]:
    return generator.make_plan_index_ranges(values)


def strict_count_mapping(raw_value: Any, label: str, required_keys: Sequence[str]) -> dict[str, int]:
    raw = object_at(raw_value, label)
    keys = {str(key) for key in raw}
    required = set(required_keys)
    missing = sorted(required - keys)
    unexpected = sorted(keys - required)
    if missing or unexpected:
        raise ValidationError(f"{label} must contain exactly {sorted(required)} (missing={missing}, unexpected={unexpected})")
    return {key: as_int(raw[key], f"{label}.{key}") for key in required_keys}


def validate_bucket_count_reconciliation(
    reconciliation: Mapping[str, Any],
    *,
    label: str,
    plan_count: int,
) -> dict[str, Any]:
    primary_counts = strict_count_mapping(
        reconciliation.get("primary_bucket_counts"),
        f"{label}.primary_bucket_counts",
        generator.PRIMARY_BUCKETS,
    )
    diagnostic_counts = strict_count_mapping(
        reconciliation.get("diagnostic_bucket_counts"),
        f"{label}.diagnostic_bucket_counts",
        generator.DIAGNOSTIC_BUCKETS,
    )
    terminal_counts = strict_count_mapping(
        reconciliation.get("terminal_bucket_counts"),
        f"{label}.terminal_bucket_counts",
        generator.TERMINAL_BUCKETS,
    )
    substatus_counts = strict_count_mapping(
        reconciliation.get("non_terminal_substatus_bucket_counts"),
        f"{label}.non_terminal_substatus_bucket_counts",
        generator.NON_TERMINAL_SUBSTATUS_BUCKETS,
    )

    primary_total = as_int(reconciliation.get("primary_bucket_total"), f"{label}.primary_bucket_total")
    if primary_total != sum(primary_counts.values()) or primary_total != plan_count:
        raise ValidationError(
            f"{label}.primary_bucket_total must reconcile the 1416-entry plan "
            f"({primary_total} != sum {sum(primary_counts.values())} != plan_count {plan_count})"
        )
    terminal_total = as_int(reconciliation.get("terminal_bucket_total"), f"{label}.terminal_bucket_total")
    if terminal_total != sum(terminal_counts.values()):
        raise ValidationError(
            f"{label}.terminal_bucket_total must equal terminal bucket counts "
            f"({terminal_total} != {sum(terminal_counts.values())})"
        )
    expected_terminal_counts = {bucket: primary_counts[bucket] for bucket in generator.TERMINAL_BUCKETS}
    if terminal_counts != expected_terminal_counts:
        raise ValidationError(
            f"{label}.terminal_bucket_counts must mirror terminal primary buckets "
            f"(expected={expected_terminal_counts}, actual={terminal_counts})"
        )
    substatus_total = as_int(
        reconciliation.get("non_terminal_substatus_bucket_total"), f"{label}.non_terminal_substatus_bucket_total"
    )
    if substatus_total != sum(substatus_counts.values()):
        raise ValidationError(
            f"{label}.non_terminal_substatus_bucket_total must equal non-terminal substatus bucket counts "
            f"({substatus_total} != {sum(substatus_counts.values())})"
        )
    if substatus_total not in (0, primary_counts["non_terminal"]):
        raise ValidationError(
            f"{label}.non_terminal_substatus_bucket_total must be zero or equal non_terminal count "
            f"({substatus_total} != {primary_counts['non_terminal']})"
        )
    diagnostic_total = as_int(reconciliation.get("diagnostic_bucket_total"), f"{label}.diagnostic_bucket_total")
    if diagnostic_total != sum(diagnostic_counts.values()):
        raise ValidationError(
            f"{label}.diagnostic_bucket_total must equal diagnostic bucket counts "
            f"({diagnostic_total} != {sum(diagnostic_counts.values())})"
        )
    return {
        "primary_bucket_counts": primary_counts,
        "terminal_bucket_counts": terminal_counts,
        "non_terminal_substatus_bucket_counts": substatus_counts,
        "diagnostic_bucket_counts": diagnostic_counts,
        "primary_bucket_total": primary_total,
        "terminal_bucket_total": terminal_total,
        "non_terminal_substatus_bucket_total": substatus_total,
        "diagnostic_bucket_total": diagnostic_total,
    }


# ---------------------------------------------------------------------------
# Difference/report helpers
# ---------------------------------------------------------------------------


def compact_json(value: Any, *, max_len: int = 220) -> str:
    text = json.dumps(value, sort_keys=True, separators=(",", ":"))
    if len(text) <= max_len:
        return text
    return text[: max_len - 3] + "..."


def first_difference(expected: Any, actual: Any, path: str = "$") -> Optional[tuple[str, Any, Any]]:
    if isinstance(expected, Mapping) and isinstance(actual, Mapping):
        expected_keys = set(expected)
        actual_keys = set(actual)
        if expected_keys != actual_keys:
            missing = sorted(str(key) for key in expected_keys - actual_keys)
            unexpected = sorted(str(key) for key in actual_keys - expected_keys)
            return f"{path} keys", {"missing": missing}, {"unexpected": unexpected}
        for key in sorted(expected_keys, key=str):
            diff = first_difference(expected[key], actual[key], f"{path}.{key}")
            if diff is not None:
                return diff
        return None
    if isinstance(expected, list) and isinstance(actual, list):
        if len(expected) != len(actual):
            return f"{path}.length", len(expected), len(actual)
        for index, (expected_item, actual_item) in enumerate(zip(expected, actual)):
            diff = first_difference(expected_item, actual_item, f"{path}[{index}]")
            if diff is not None:
                return diff
        return None
    if expected != actual:
        return path, expected, actual
    return None


def require_equal(name: str, expected: Any, actual: Any) -> None:
    if expected == actual:
        return
    diff = first_difference(expected, actual)
    if diff is None:
        raise ValidationError(f"{name} differs from regenerated evidence")
    path, expected_value, actual_value = diff
    raise ValidationError(
        f"{name} differs from regenerated evidence at {path}: "
        f"expected {compact_json(expected_value)}, got {compact_json(actual_value)}"
    )


# ---------------------------------------------------------------------------
# Focused manifest checks
# ---------------------------------------------------------------------------


def validate_manifest_coverage(manifest: Mapping[str, Any], plan_entries: Sequence[Mapping[str, Any]]) -> list[Mapping[str, Any]]:
    label = "manifest"
    if manifest.get("schema_name") != generator.MANIFEST_SCHEMA:
        raise ValidationError(
            f"{label}.schema_name must be {generator.MANIFEST_SCHEMA!r}, got {manifest.get('schema_name')!r}"
        )
    if as_int(manifest.get("schema_version"), f"{label}.schema_version") != generator.SCHEMA_VERSION:
        raise ValidationError(f"{label}.schema_version must be {generator.SCHEMA_VERSION}")
    entry_count = as_int(manifest.get("entry_count"), f"{label}.entry_count")
    if entry_count != generator.EXPECTED_ENTRY_COUNT:
        raise ValidationError(
            f"{label}.entry_count must be the 1416-entry discovery plan, got {entry_count}"
        )
    source_plan = object_at(manifest.get("source_plan"), f"{label}.source_plan")
    source_plan_count = as_int(source_plan.get("entry_count"), f"{label}.source_plan.entry_count")
    if source_plan_count != generator.EXPECTED_ENTRY_COUNT:
        raise ValidationError(
            f"{label}.source_plan.entry_count must be the 1416-entry discovery plan, got {source_plan_count}"
        )
    entries = [object_at(entry, f"{label}.entries[{offset}]") for offset, entry in enumerate(list_at(manifest.get("entries"), f"{label}.entries"))]
    plan_count = len(plan_entries)
    if len(entries) != plan_count:
        raise ValidationError(f"{label}.entries length must be {plan_count}, got {len(entries)}")

    seen: set[int] = set()
    duplicates: set[int] = set()
    ordered_indices: list[int] = []
    for offset, entry in enumerate(entries):
        plan_index = as_int(entry.get("plan_index"), f"{label}.entries[{offset}].plan_index")
        ordered_indices.append(plan_index)
        if plan_index in seen:
            duplicates.add(plan_index)
        seen.add(plan_index)
    expected_indices = set(range(1, plan_count + 1))
    missing = expected_indices - seen
    extra = seen - expected_indices
    if duplicates or missing or extra:
        raise ValidationError(
            "manifest entries must cover exactly the 1416-entry plan once "
            f"(duplicates={make_ranges(duplicates)}, missing={make_ranges(missing)}, extra={make_ranges(extra)})"
        )
    expected_order = list(range(1, plan_count + 1))
    if ordered_indices != expected_order:
        for offset, (actual, expected) in enumerate(zip(ordered_indices, expected_order)):
            if actual != expected:
                raise ValidationError(
                    f"manifest entries must be ordered by plan_index; entries[{offset}].plan_index "
                    f"is {actual}, expected {expected}"
                )
        raise ValidationError("manifest entries must be ordered by plan_index")
    return entries


def validate_hotspot_48(entries: Sequence[Mapping[str, Any]]) -> dict[str, Any]:
    hotspot_entries = [
        entry
        for entry in entries
        if str(entry.get("hardware_kernel_name")) == "hotspot" and str(entry.get("hardware_problem_size")) == "48"
    ]
    if len(hotspot_entries) != 1 or as_int(hotspot_entries[0].get("plan_index"), "hotspot_48.plan_index") != 474:
        raise ValidationError(
            "manifest must include exactly one hotspot/48 entry at plan_index 474 "
            f"(found_plan_indices={make_ranges([as_int(entry.get('plan_index'), 'hotspot_48.plan_index') for entry in hotspot_entries])})"
        )
    hotspot = hotspot_entries[0]
    required_values = {
        "benchmark": "hotspot",
        "problem_size": "48",
        "kernel": "hotspot",
        "tier": 2,
        "workflow_job": "benchmark-tier2",
        "timeout_sec": generator.EXPECTED_TIMEOUT_SEC,
    }
    for key, expected in required_values.items():
        actual = hotspot.get(key)
        if actual != expected:
            raise ValidationError(f"hotspot/48 manifest entry field {key!r} must be {expected!r}, got {actual!r}")
    return {
        "plan_index": 474,
        "benchmark": hotspot.get("benchmark"),
        "problem_size": hotspot.get("problem_size"),
        "eligible_for_linear_workflow": bool(hotspot.get("eligible_for_linear_workflow")),
        "observed_outcome_bucket": hotspot.get("observed_outcome_bucket"),
        "observed_outcome": hotspot.get("observed_outcome"),
        "duration_sec": hotspot.get("duration_sec"),
    }


def validate_manifest_matches_plan(entries: Sequence[Mapping[str, Any]], plan_entries: Sequence[Mapping[str, Any]]) -> None:
    field_pairs = (
        ("benchmark", "benchmark"),
        ("problem_size", "problem_size"),
        ("hardware_kernel_name", "hardware_kernel_name"),
        ("hardware_problem_size", "hardware_problem_size"),
        ("tier", "tier"),
        ("tier_name", "tier_name"),
        ("workflow_job", "workflow_job"),
        ("workflow_benchmark_name", "workflow_benchmark_name"),
        ("sizes", "sizes"),
        ("size_token", "size_token"),
        ("size_label", "size_label"),
        ("timeout_sec", "timeout_sec"),
        ("flag_template", "flag_template"),
        ("size_label_template", "size_label_template"),
        ("expected_artifact_id", "artifact_id"),
    )
    for offset, (entry, plan_entry) in enumerate(zip(entries, plan_entries)):
        plan_index = offset + 1
        for manifest_key, plan_key in field_pairs:
            if entry.get(manifest_key) != plan_entry.get(plan_key):
                raise ValidationError(
                    f"manifest.entries[{offset}].{manifest_key} must match plan entry {plan_index} {plan_key} "
                    f"({entry.get(manifest_key)!r} != {plan_entry.get(plan_key)!r})"
                )


def validate_eligibility_policy(entries: Sequence[Mapping[str, Any]]) -> None:
    eligible_indices: list[int] = []
    excluded_indices: list[int] = []
    for offset, entry in enumerate(entries):
        label = f"manifest.entries[{offset}]"
        plan_index = as_int(entry.get("plan_index"), f"{label}.plan_index")
        eligible = bool(entry.get("eligible_for_linear_workflow"))
        status = str(entry.get("observed_job_status") or "")
        conclusion = str(entry.get("observed_job_conclusion") or "")
        bucket = str(entry.get("observed_outcome_bucket") or "")
        outcome = str(entry.get("observed_outcome") or "")
        terminal_outcome = str(entry.get("terminal_outcome") or "")
        duration = entry.get("duration_sec")
        duration_ok = duration is None or as_int(duration, f"{label}.duration_sec") <= generator.EXPECTED_TIMEOUT_SEC
        job_evidence = entry.get("discovery_job_database_id") is not None or bool(entry.get("discovery_job_name"))
        artifact_evidence = entry.get("artifact_database_id") is not None or bool(entry.get("artifact_name"))
        success_evidence = (
            bucket == "completed_success"
            and status == "completed"
            and conclusion == "success"
            and outcome == "success"
            and terminal_outcome == "success"
            and job_evidence
            and artifact_evidence
            and entry.get("outcome_timeout_sec") == generator.EXPECTED_TIMEOUT_SEC
            and duration_ok
        )
        if eligible:
            eligible_indices.append(plan_index)
            if not success_evidence:
                raise ValidationError(
                    f"{label} is eligible without complete completed/success outcome evidence "
                    f"(bucket={bucket!r}, status={status!r}, conclusion={conclusion!r}, outcome={outcome!r})"
                )
            if entry.get("exclusion_reason") is not None or entry.get("eligibility_status") != "eligible":
                raise ValidationError(f"{label} eligible row must have eligibility_status='eligible' and no exclusion_reason")
        else:
            excluded_indices.append(plan_index)
            if entry.get("eligibility_status") != "excluded":
                raise ValidationError(f"{label} excluded row must have eligibility_status='excluded'")
            if success_evidence and entry.get("exclusion_reason") is None:
                raise ValidationError(f"{label} has success evidence but is excluded without an exclusion_reason")
    if len(eligible_indices) + len(excluded_indices) != generator.EXPECTED_ENTRY_COUNT:
        raise ValidationError("eligibility policy did not classify every manifest entry")


def validate_snapshot_contract(label: str, document: Mapping[str, Any]) -> None:
    run = object_at(document.get("run"), f"{label}.run")
    snapshot_contract = object_at(document.get("snapshot_contract"), f"{label}.snapshot_contract")
    non_terminal_snapshot = bool(run.get("non_terminal_snapshot"))
    contract_non_terminal = snapshot_contract.get("non_terminal_snapshot") is True
    if contract_non_terminal != non_terminal_snapshot:
        raise ValidationError(
            f"{label}.snapshot_contract.non_terminal_snapshot must match {label}.run.non_terminal_snapshot"
        )
    if not non_terminal_snapshot:
        if "observation" in document:
            observation = object_at(document.get("observation"), f"{label}.observation")
            if observation.get("non_terminal_snapshot") not in (False, None):
                raise ValidationError(f"{label}.observation.non_terminal_snapshot must be false for terminal/current provenance")
        if snapshot_contract.get("bounded_snapshot") is not False:
            raise ValidationError(f"{label}.snapshot_contract.bounded_snapshot must be false for terminal/non-bounded provenance")
        if snapshot_contract.get("terminal_provenance") is not True:
            raise ValidationError(
                f"{label}.snapshot_contract.terminal_provenance must be true for terminal/non-bounded provenance"
            )
        claim_scope = str(snapshot_contract.get("claim_scope") or "").lower()
        if "terminal discovery evidence" not in claim_scope:
            raise ValidationError(
                f"{label}.snapshot_contract.claim_scope for terminal provenance must mention terminal discovery evidence"
            )
        return

    if snapshot_contract.get("bounded_snapshot") is not True:
        raise ValidationError(f"{label}.snapshot_contract.bounded_snapshot must be true for non-terminal snapshots")
    if not str(snapshot_contract.get("observation_time_utc") or ""):
        raise ValidationError(f"{label}.snapshot_contract.observation_time_utc is required for non-terminal snapshots")
    claim_scope = str(snapshot_contract.get("claim_scope") or "").lower()
    required_phrase = "not final/current finishable-size claims"
    if required_phrase not in claim_scope:
        raise ValidationError(
            f"{label}.snapshot_contract.claim_scope for non-terminal snapshots must state rows are "
            f"{required_phrase!r}"
        )

    if "observation" in document:
        observation = object_at(document.get("observation"), f"{label}.observation")
        if observation.get("non_terminal_snapshot") is not True:
            raise ValidationError(f"{label}.observation.non_terminal_snapshot must be true for non-terminal snapshots")


def validate_snapshot_contracts(
    manifest: Mapping[str, Any], summary: Mapping[str, Any], tier1: Mapping[str, Any], tier2: Mapping[str, Any]
) -> None:
    for label, document in (("manifest", manifest), ("summary", summary), ("tier1", tier1), ("tier2", tier2)):
        validate_snapshot_contract(label, document)


def validate_summary_ranges(
    manifest: Mapping[str, Any],
    summary: Mapping[str, Any],
    tier1: Mapping[str, Any],
    tier2: Mapping[str, Any],
    entries: Sequence[Mapping[str, Any]],
) -> None:
    manifest_summary = object_at(manifest.get("summary"), "manifest.summary")
    summary_summary = object_at(summary.get("summary"), "summary.summary")
    if manifest_summary != summary_summary:
        raise ValidationError("manifest.summary must exactly match compact summary.summary")
    plan_count = len(entries)
    eligible = {as_int(entry.get("plan_index"), "entry.plan_index") for entry in entries if entry.get("eligible_for_linear_workflow")}
    excluded = {as_int(entry.get("plan_index"), "entry.plan_index") for entry in entries if not entry.get("eligible_for_linear_workflow")}
    declared_eligible = range_set(manifest_summary.get("eligible_plan_index_ranges"), "manifest.summary.eligible_plan_index_ranges", plan_count)
    declared_excluded = range_set(manifest_summary.get("excluded_plan_index_ranges"), "manifest.summary.excluded_plan_index_ranges", plan_count)
    if declared_eligible != eligible:
        raise ValidationError(
            "manifest.summary.eligible_plan_index_ranges must match eligible rows "
            f"(declared={make_ranges(declared_eligible)}, actual={make_ranges(eligible)})"
        )
    if declared_excluded != excluded:
        raise ValidationError(
            "manifest.summary.excluded_plan_index_ranges must match excluded rows "
            f"(declared={make_ranges(declared_excluded)}, actual={make_ranges(excluded)})"
        )
    if eligible & excluded or eligible | excluded != set(range(1, plan_count + 1)):
        raise ValidationError("eligible/excluded ranges must partition all 1416 plan entries")

    reconciliation = validate_bucket_count_reconciliation(
        object_at(
            manifest_summary.get("requested_bucket_count_reconciliation"),
            "manifest.summary.requested_bucket_count_reconciliation",
        ),
        label="manifest.summary.requested_bucket_count_reconciliation",
        plan_count=plan_count,
    )
    observed_primary_counts = {bucket: 0 for bucket in generator.PRIMARY_BUCKETS}
    for entry in entries:
        bucket = str(entry.get("observed_outcome_bucket") or "")
        if bucket not in observed_primary_counts:
            raise ValidationError(f"manifest entry has unsupported observed_outcome_bucket {bucket!r}")
        observed_primary_counts[bucket] += 1
    if reconciliation["primary_bucket_counts"] != observed_primary_counts:
        raise ValidationError(
            "manifest.summary.requested_bucket_count_reconciliation.primary_bucket_counts must match manifest rows "
            f"(expected={observed_primary_counts}, actual={reconciliation['primary_bucket_counts']})"
        )

    primary_ranges = object_at(
        manifest_summary.get("requested_primary_bucket_plan_index_ranges"),
        "manifest.summary.requested_primary_bucket_plan_index_ranges",
    )
    primary_range_seen: set[int] = set()
    primary_range_overlaps: set[int] = set()
    for bucket in generator.PRIMARY_BUCKETS:
        declared = range_set(
            primary_ranges.get(bucket, []),
            f"manifest.summary.requested_primary_bucket_plan_index_ranges.{bucket}",
            plan_count,
        )
        if len(declared) != reconciliation["primary_bucket_counts"][bucket]:
            raise ValidationError(
                f"manifest.summary.requested_primary_bucket_plan_index_ranges.{bucket} cardinality must match bucket count "
                f"({len(declared)} != {reconciliation['primary_bucket_counts'][bucket]})"
            )
        primary_range_overlaps |= primary_range_seen & declared
        primary_range_seen |= declared
    expected_plan_indices = set(range(1, plan_count + 1))
    if primary_range_overlaps or primary_range_seen != expected_plan_indices:
        raise ValidationError(
            "manifest.summary.requested_primary_bucket_plan_index_ranges must cover the 1416-entry plan exactly once "
            f"(overlaps={make_ranges(primary_range_overlaps)}, missing={make_ranges(expected_plan_indices - primary_range_seen)})"
        )

    diagnostic_ranges = object_at(
        manifest_summary.get("requested_diagnostic_bucket_plan_index_ranges"),
        "manifest.summary.requested_diagnostic_bucket_plan_index_ranges",
    )
    diagnostic_sets: dict[str, set[int]] = {}
    for bucket in generator.DIAGNOSTIC_BUCKETS:
        declared = range_set(
            diagnostic_ranges.get(bucket, []),
            f"manifest.summary.requested_diagnostic_bucket_plan_index_ranges.{bucket}",
            plan_count,
        )
        diagnostic_sets[bucket] = declared
        if len(declared) != reconciliation["diagnostic_bucket_counts"][bucket]:
            raise ValidationError(
                f"manifest.summary.requested_diagnostic_bucket_plan_index_ranges.{bucket} cardinality must match bucket count "
                f"({len(declared)} != {reconciliation['diagnostic_bucket_counts'][bucket]})"
            )
    completed_success_ranges = range_set(
        primary_ranges.get("completed_success", []),
        "manifest.summary.requested_primary_bucket_plan_index_ranges.completed_success",
        plan_count,
    )
    for bucket in ("missing_artifact_for_completed_success_job", "missing_outcome_row_for_completed_success_job"):
        if not diagnostic_sets[bucket].issubset(completed_success_ranges):
            raise ValidationError(
                f"manifest.summary.requested_diagnostic_bucket_plan_index_ranges.{bucket} must be a subset of completed_success"
            )
    actual_missing_outcome_rows = {
        as_int(entry.get("plan_index"), "entry.plan_index") for entry in entries if entry.get("terminal_outcome") is None
    }
    if diagnostic_sets["missing_outcome_row"] != actual_missing_outcome_rows:
        raise ValidationError(
            "manifest.summary.requested_diagnostic_bucket_plan_index_ranges.missing_outcome_row must match rows without terminal outcomes "
            f"(declared={make_ranges(diagnostic_sets['missing_outcome_row'])}, actual={make_ranges(actual_missing_outcome_rows)})"
        )

    for tier_name, tier_view, tier_value in (("tier1", tier1, 1), ("tier2", tier2, 2)):
        if tier_view.get("schema_name") != generator.TIER_VIEW_SCHEMA:
            raise ValidationError(f"{tier_name}.schema_name must be {generator.TIER_VIEW_SCHEMA!r}")
        if as_int(tier_view.get("tier"), f"{tier_name}.tier") != tier_value:
            raise ValidationError(f"{tier_name}.tier must be {tier_value}")
        tier_entries = [entry for entry in entries if entry.get("tier") == tier_value]
        tier_eligible = {as_int(entry.get("plan_index"), "entry.plan_index") for entry in tier_entries if entry.get("eligible_for_linear_workflow")}
        declared = range_set(tier_view.get("eligible_plan_index_ranges"), f"{tier_name}.eligible_plan_index_ranges", plan_count)
        if declared != tier_eligible:
            raise ValidationError(
                f"{tier_name}.eligible_plan_index_ranges must match eligible tier rows "
                f"(declared={make_ranges(declared)}, actual={make_ranges(tier_eligible)})"
            )


# ---------------------------------------------------------------------------
# Focused provenance outcome checks
# ---------------------------------------------------------------------------


def validate_terminal_or_non_bounded_claim_scope(
    provenance: Mapping[str, Any],
    bucket_sets: Mapping[str, set[int]],
    plan_count: int,
    label: str,
) -> None:
    run = object_at(provenance.get("run"), f"{label}.run")
    requested = object_at(provenance.get("requested_plan_outcome_buckets"), f"{label}.requested_plan_outcome_buckets")
    non_terminal_snapshot = bool(run.get("non_terminal_snapshot"))
    terminal_claim = (
        requested.get("terminal_mode") is True
        or requested.get("terminal_provenance") is True
        or provenance.get("terminal_outcome_accounting") is not None
    )
    non_bounded_claim = not non_terminal_snapshot
    if not (terminal_claim or non_bounded_claim):
        return

    failures: list[str] = []
    if provenance.get("terminal_outcome_accounting") is None:
        failures.append(f"{label}.terminal_outcome_accounting is required")
    non_empty_buckets = {
        bucket: make_ranges(bucket_sets[bucket])
        for bucket in generator.NON_BOUNDED_BLOCKING_BUCKETS
        if bucket_sets[bucket]
    }
    if non_empty_buckets:
        failures.append(
            "non-terminal/queued/in-progress/missing buckets are allowed only for explicit "
            f"bounded non-terminal snapshots (non_empty={non_empty_buckets})"
        )
    if failures:
        raise ValidationError(
            "terminal/non-bounded finishable evidence claims require complete terminal_outcome_accounting; "
            + "; ".join(failures)
        )

    try:
        generator.validate_terminal_outcome_accounting(provenance, bucket_sets, plan_count, label)
    except generator.ManifestError as exc:
        raise ValidationError(str(exc)) from exc


def validate_provenance_outcome_integrity(provenance: Mapping[str, Any], plan_count: int, label: str) -> dict[str, Any]:
    try:
        bucket_sets = generator.parse_bucket_sets(provenance, plan_count, label)
    except generator.ManifestError as exc:
        raise ValidationError(str(exc)) from exc

    requested_reconciliation = validate_bucket_count_reconciliation(
        generator.bucket_count_reconciliation(bucket_sets),
        label=f"{label}.requested_plan_outcome_buckets",
        plan_count=plan_count,
    )

    requested = object_at(provenance.get("requested_plan_outcome_buckets"), f"{label}.requested_plan_outcome_buckets")
    if requested.get("no_results_invented_for_non_terminal_or_missing_rows") is not True:
        raise ValidationError(
            f"{label}.requested_plan_outcome_buckets.no_results_invented_for_non_terminal_or_missing_rows must be true"
        )
    validate_terminal_or_non_bounded_claim_scope(provenance, bucket_sets, plan_count, label)
    terminal_mode = (
        requested.get("terminal_mode") is True
        or requested.get("terminal_provenance") is True
        or provenance.get("terminal_outcome_accounting") is not None
    )
    if terminal_mode:
        if provenance.get("terminal_outcome_accounting") is None:
            raise ValidationError(f"{label}.terminal_outcome_accounting is required for terminal-mode provenance")
        terminal_indices = set().union(*(bucket_sets[bucket] for bucket in generator.TERMINAL_BUCKETS))
        expected_indices = set(range(1, plan_count + 1))
        disallowed = {
            "non_terminal": bucket_sets["non_terminal"],
            "missing_job": bucket_sets["missing_job"],
            "missing_outcome_row": bucket_sets["missing_outcome_row"],
            "non_terminal_in_progress": bucket_sets["non_terminal_in_progress"],
            "non_terminal_queued": bucket_sets["non_terminal_queued"],
        }
        non_empty_disallowed = {bucket: make_ranges(values) for bucket, values in disallowed.items() if values}
        if non_empty_disallowed or terminal_indices != expected_indices:
            raise ValidationError(
                "terminal provenance must account every plan entry as completed/failed/timed_out/skipped/cancelled "
                f"with no non-terminal or missing buckets (disallowed={non_empty_disallowed}, "
                f"missing={make_ranges(expected_indices - terminal_indices)})"
            )

    outcome_summary = object_at(provenance.get("outcome_summary"), f"{label}.outcome_summary")
    outcome_rows = [
        object_at(row, f"{label}.outcome_summary.rows[{offset}]")
        for offset, row in enumerate(list_at(outcome_summary.get("rows"), f"{label}.outcome_summary.rows"))
    ]
    row_indices = {
        as_int(row.get("discovery_plan_index"), f"{label}.outcome_summary.rows[{offset}].discovery_plan_index")
        for offset, row in enumerate(outcome_rows)
    }
    terminal_indices = set().union(*(bucket_sets[bucket] for bucket in generator.TERMINAL_BUCKETS))
    missing_rows = terminal_indices - row_indices
    fabricated_rows = row_indices - terminal_indices
    if missing_rows or fabricated_rows:
        raise ValidationError(
            "terminal outcome rows must exactly match terminal requested buckets "
            f"(missing={make_ranges(missing_rows)}, fabricated={make_ranges(fabricated_rows)})"
        )
    fabricated_non_terminal = row_indices & (bucket_sets["non_terminal"] | bucket_sets["missing_job"])
    if fabricated_non_terminal:
        raise ValidationError(
            "fabricated terminal outcome rows for non-terminal or missing jobs: "
            f"{make_ranges(fabricated_non_terminal)}"
        )

    expected_row_count = as_int(
        outcome_summary.get("terminal_outcome_row_count"), f"{label}.outcome_summary.terminal_outcome_row_count"
    )
    if expected_row_count != len(outcome_rows):
        raise ValidationError(
            f"{label}.outcome_summary.terminal_outcome_row_count must match rows length "
            f"({expected_row_count} != {len(outcome_rows)})"
        )

    declared_outcome_ranges = object_at(
        outcome_summary.get("terminal_outcome_plan_index_ranges", {}),
        f"{label}.outcome_summary.terminal_outcome_plan_index_ranges",
    )
    observed_by_outcome: dict[str, set[int]] = {}
    for row in outcome_rows:
        outcome = str(row.get("terminal_outcome") or "")
        observed_by_outcome.setdefault(outcome, set()).add(as_int(row.get("discovery_plan_index"), "outcome.discovery_plan_index"))
    for outcome, observed_indices in observed_by_outcome.items():
        declared = range_set(
            declared_outcome_ranges.get(outcome, []),
            f"{label}.outcome_summary.terminal_outcome_plan_index_ranges.{outcome}",
            plan_count,
        )
        if declared != observed_indices:
            raise ValidationError(
                f"{label}.outcome_summary.terminal_outcome_plan_index_ranges.{outcome} must match rows "
                f"(declared={make_ranges(declared)}, actual={make_ranges(observed_indices)})"
            )
    for outcome, raw_ranges in declared_outcome_ranges.items():
        if str(outcome) not in observed_by_outcome:
            declared = range_set(
                raw_ranges,
                f"{label}.outcome_summary.terminal_outcome_plan_index_ranges.{outcome}",
                plan_count,
            )
            if declared:
                raise ValidationError(
                    f"{label}.outcome_summary.terminal_outcome_plan_index_ranges has undeclared outcome {outcome!r}"
                )

    artifact_summary = object_at(provenance.get("artifact_summary"), f"{label}.artifact_summary")
    artifact_entries = [
        object_at(entry, f"{label}.artifact_summary.entries[{offset}]")
        for offset, entry in enumerate(list_at(artifact_summary.get("entries"), f"{label}.artifact_summary.entries"))
    ]
    artifact_indices = {
        as_int(entry.get("plan_index"), f"{label}.artifact_summary.entries[{offset}].plan_index")
        for offset, entry in enumerate(artifact_entries)
    }
    declared_artifact_indices = range_set(
        artifact_summary.get("plan_index_ranges", []), f"{label}.artifact_summary.plan_index_ranges", plan_count
    )
    if declared_artifact_indices != artifact_indices:
        raise ValidationError(
            f"{label}.artifact_summary.plan_index_ranges must match artifact entries "
            f"(declared={make_ranges(declared_artifact_indices)}, actual={make_ranges(artifact_indices)})"
        )
    for count_key in ("total_count", "benchmark_result_artifact_count"):
        if count_key in artifact_summary:
            count = as_int(artifact_summary.get(count_key), f"{label}.artifact_summary.{count_key}")
            if count != len(artifact_entries):
                raise ValidationError(
                    f"{label}.artifact_summary.{count_key} must match artifact entries length "
                    f"({count} != {len(artifact_entries)})"
                )
    if artifact_indices != row_indices:
        raise ValidationError(
            "artifact entries must match terminal outcome rows because outcome rows are parsed from result artifacts "
            f"(missing_artifacts={make_ranges(row_indices - artifact_indices)}, fabricated_artifacts={make_ranges(artifact_indices - row_indices)})"
        )

    terminal_job_summary = object_at(
        provenance.get("terminal_discovery_job_summary"), f"{label}.terminal_discovery_job_summary"
    )
    terminal_job_entries = [
        object_at(entry, f"{label}.terminal_discovery_job_summary.entries[{offset}]")
        for offset, entry in enumerate(
            list_at(terminal_job_summary.get("entries"), f"{label}.terminal_discovery_job_summary.entries")
        )
    ]
    terminal_job_indices = {
        as_int(entry.get("plan_index"), f"{label}.terminal_discovery_job_summary.entries[{offset}].plan_index")
        for offset, entry in enumerate(terminal_job_entries)
    }
    if terminal_job_indices != row_indices:
        raise ValidationError(
            "terminal job entries must match terminal outcome rows "
            f"(missing_jobs={make_ranges(row_indices - terminal_job_indices)}, fabricated_jobs={make_ranges(terminal_job_indices - row_indices)})"
        )
    completed_job_count = as_int(
        terminal_job_summary.get("completed_job_count"), f"{label}.terminal_discovery_job_summary.completed_job_count"
    )
    if completed_job_count != len(terminal_job_entries):
        raise ValidationError(
            f"{label}.terminal_discovery_job_summary.completed_job_count must match entries length "
            f"({completed_job_count} != {len(terminal_job_entries)})"
        )

    workflow_summary = object_at(provenance.get("workflow_summary"), f"{label}.workflow_summary")
    discovery_job_count = as_int(workflow_summary.get("discovery_job_count"), f"{label}.workflow_summary.discovery_job_count")
    missing_job_count = len(bucket_sets["missing_job"])
    if discovery_job_count != plan_count - missing_job_count:
        raise ValidationError(
            f"{label}.workflow_summary.discovery_job_count must equal plan_count - missing_job_count "
            f"({discovery_job_count} != {plan_count - missing_job_count})"
        )
    status_counts = object_at(workflow_summary.get("discovery_job_status_counts"), f"{label}.workflow_summary.discovery_job_status_counts")
    expected_status_counts = {
        "completed": len(terminal_indices),
        "in_progress": len(bucket_sets["non_terminal_in_progress"]),
        "queued": len(bucket_sets["non_terminal_queued"]),
    }
    normalized_status_counts = {str(key): as_int(value, f"{label}.workflow_summary.discovery_job_status_counts.{key}") for key, value in status_counts.items()}
    if normalized_status_counts != {key: value for key, value in expected_status_counts.items() if value or key in normalized_status_counts}:
        raise ValidationError(
            f"{label}.workflow_summary.discovery_job_status_counts must match requested outcome buckets "
            f"(expected={expected_status_counts}, actual={normalized_status_counts})"
        )
    all_status_counts = object_at(
        terminal_job_summary.get("all_discovery_job_status_counts"),
        f"{label}.terminal_discovery_job_summary.all_discovery_job_status_counts",
    )
    normalized_all_status = {str(key): as_int(value, f"{label}.terminal_discovery_job_summary.all_discovery_job_status_counts.{key}") for key, value in all_status_counts.items()}
    if normalized_all_status != normalized_status_counts:
        raise ValidationError(
            f"{label}.terminal_discovery_job_summary.all_discovery_job_status_counts must match workflow_summary.discovery_job_status_counts"
        )

    workflow_conclusions = object_at(
        workflow_summary.get("discovery_job_conclusion_counts"), f"{label}.workflow_summary.discovery_job_conclusion_counts"
    )
    terminal_conclusions = object_at(
        terminal_job_summary.get("all_discovery_job_conclusion_counts"),
        f"{label}.terminal_discovery_job_summary.all_discovery_job_conclusion_counts",
    )
    normalized_workflow_conclusions = {str(key): as_int(value, f"{label}.workflow_summary.discovery_job_conclusion_counts.{key}") for key, value in workflow_conclusions.items()}
    normalized_terminal_conclusions = {str(key): as_int(value, f"{label}.terminal_discovery_job_summary.all_discovery_job_conclusion_counts.{key}") for key, value in terminal_conclusions.items()}
    if normalized_terminal_conclusions != normalized_workflow_conclusions:
        raise ValidationError(
            f"{label}.terminal_discovery_job_summary.all_discovery_job_conclusion_counts must match workflow_summary.discovery_job_conclusion_counts"
        )

    return {
        "terminal_outcome_row_count": len(outcome_rows),
        "terminal_plan_index_ranges": make_ranges(terminal_indices),
        "artifact_entry_count": len(artifact_entries),
        "terminal_job_entry_count": len(terminal_job_entries),
        "discovery_job_status_counts": normalized_status_counts,
        "requested_bucket_count_reconciliation": requested_reconciliation,
    }


# ---------------------------------------------------------------------------
# Top-level validation/reporting
# ---------------------------------------------------------------------------


def validate_paths(
    *,
    plan_path: Path,
    provenance_path: Path,
    manifest_path: Path,
    tier1_path: Path,
    tier2_path: Path,
    summary_path: Path,
) -> dict[str, Any]:
    try:
        _plan, plan_entries, _by_index = generator.load_and_validate_plan(plan_path)
        provenance = generator.load_and_validate_provenance(provenance_path, plan_path, len(plan_entries))
    except generator.ManifestError as exc:
        raise ValidationError(str(exc)) from exc

    provenance_report = validate_provenance_outcome_integrity(
        provenance, len(plan_entries), repo_relative(provenance_path)
    )

    try:
        expected_manifest, expected_tier1, expected_tier2, expected_summary = generator.build_manifest(
            plan_path,
            provenance_path,
            manifest_path,
            tier1_path,
            tier2_path,
            summary_path,
        )
    except generator.ManifestError as exc:
        raise ValidationError(str(exc)) from exc

    manifest = load_json_object(manifest_path)
    tier1 = load_json_object(tier1_path)
    tier2 = load_json_object(tier2_path)
    summary = load_json_object(summary_path)

    entries = validate_manifest_coverage(manifest, plan_entries)
    hotspot_report = validate_hotspot_48(entries)
    validate_manifest_matches_plan(entries, plan_entries)
    validate_eligibility_policy(entries)
    validate_summary_ranges(manifest, summary, tier1, tier2, entries)
    validate_snapshot_contracts(manifest, summary, tier1, tier2)

    require_equal(repo_relative(manifest_path), expected_manifest, manifest)
    require_equal(repo_relative(tier1_path), expected_tier1, tier1)
    require_equal(repo_relative(tier2_path), expected_tier2, tier2)
    require_equal(repo_relative(summary_path), expected_summary, summary)

    manifest_summary = object_at(manifest.get("summary"), "manifest.summary")
    return {
        "schema_name": VALIDATION_SCHEMA,
        "schema_version": SCHEMA_VERSION,
        "status": "pass",
        "source_paths": {
            "problem_size_discovery_plan": repo_relative(plan_path),
            "discovery_provenance": repo_relative(provenance_path),
            "finishable_manifest": repo_relative(manifest_path),
            "tier1_view": repo_relative(tier1_path),
            "tier2_view": repo_relative(tier2_path),
            "summary": repo_relative(summary_path),
        },
        "plan_entry_count": len(plan_entries),
        "manifest_entry_count": as_int(manifest.get("entry_count"), "manifest.entry_count"),
        "eligible_entry_count": as_int(manifest_summary.get("eligible_entry_count"), "manifest.summary.eligible_entry_count"),
        "excluded_entry_count": as_int(manifest_summary.get("excluded_entry_count"), "manifest.summary.excluded_entry_count"),
        "eligible_plan_index_ranges": manifest_summary.get("eligible_plan_index_ranges"),
        "observed_outcome_bucket_counts": manifest_summary.get("observed_outcome_bucket_counts"),
        "tier_counts": manifest_summary.get("tier_counts"),
        "hotspot_48": hotspot_report,
        "provenance": provenance_report,
    }


def parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Validate deterministic MI300A finishable-size manifest, summary, and Tier 1/Tier 2 views."
    )
    parser.add_argument("--plan", default=str(generator.DEFAULT_PLAN), help="Path to mi300a_problem_size_discovery_plan.json")
    parser.add_argument(
        "--provenance",
        default=str(generator.DEFAULT_PROVENANCE),
        help="Path to compact discovery run outcome/provenance JSON",
    )
    parser.add_argument("--manifest", default=str(generator.DEFAULT_MANIFEST), help="Path to finishable-size manifest JSON")
    parser.add_argument("--tier1", default=str(generator.DEFAULT_TIER1_VIEW), help="Path to Tier 1 finishable matrix view")
    parser.add_argument("--tier2", default=str(generator.DEFAULT_TIER2_VIEW), help="Path to Tier 2 finishable matrix view")
    parser.add_argument("--summary", default=str(generator.DEFAULT_SUMMARY), help="Path to compact finishable-size summary JSON")
    return parser.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    args = parse_args(argv)
    try:
        report = validate_paths(
            plan_path=repo_path(args.plan),
            provenance_path=repo_path(args.provenance),
            manifest_path=repo_path(args.manifest),
            tier1_path=repo_path(args.tier1),
            tier2_path=repo_path(args.tier2),
            summary_path=repo_path(args.summary),
        )
    except ValidationError as exc:
        print("FAIL: MI300A finishable-size manifest validation rejected input.", file=sys.stderr)
        print(f"  - {exc}", file=sys.stderr)
        return 1

    print(json.dumps(report, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
