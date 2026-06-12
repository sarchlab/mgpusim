#!/usr/bin/env python3
"""Validate and summarize the bounded MI300A finishable evaluation set.

This static CLI consumes the checked-in finishable-size manifest and the
eligible-only Tier 1/Tier 2 matrix views.  It derives the small post-cleanup
MI300A evaluation surface, validates the checked-in evaluation manifest and
summary against that derivation, and emits deterministic JSON or text output.

It does not query GitHub, dispatch CI, run simulations, or inspect simulator
outputs.  The source finishable manifest remains the authority for the bounded
snapshot disclaimer: the 23 included entries completed successfully within the
3600s problem-size discovery budget, while timed-out/non-terminal exclusions are
observation-time evidence only and are not permanent unfinishability claims.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any, Iterable, Mapping, Optional, Sequence

import generate_mi300a_finishable_size_manifest as finishable


REPO_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_FINISHABLE_MANIFEST = REPO_ROOT / "benchmark-comparison" / "mi300a_finishable_size_manifest.json"
DEFAULT_TIER1_VIEW = REPO_ROOT / "benchmark-comparison" / "generated" / "mi300a_finishable_tier1_matrix.json"
DEFAULT_TIER2_VIEW = REPO_ROOT / "benchmark-comparison" / "generated" / "mi300a_finishable_tier2_matrix.json"
DEFAULT_EVALUATION_MANIFEST = REPO_ROOT / "benchmark-comparison" / "mi300a_finishable_evaluation_set.json"
DEFAULT_EVALUATION_SUMMARY = REPO_ROOT / "benchmark-comparison" / "mi300a_finishable_evaluation_set_summary.json"

EVALUATION_SCHEMA = "mgpusim.mi300a_finishable_evaluation_set"
SUMMARY_SCHEMA = "mgpusim.mi300a_finishable_evaluation_summary"
SCHEMA_VERSION = 1
SET_ID = "mi300a-bounded-finishable-evaluation"
SET_VERSION = "2026.04.26-run24959959195"

EXPECTED_DISCOVERY_ENTRY_COUNT = 1416
EXPECTED_FINISHABLE_ENTRY_COUNT = 1416
EXPECTED_ELIGIBLE_ENTRY_COUNT = 23
EXPECTED_EXCLUDED_ENTRY_COUNT = 1393
EXPECTED_TIMEOUT_SEC = 3600
EXPECTED_OBSERVED_OUTCOME_BUCKET_COUNTS = {
    "completed_success": 23,
    "non_terminal": 1373,
    "timed_out": 20,
}
EXPECTED_PRIMARY_BUCKET_COUNTS = {
    "completed_success": 23,
    "failed": 0,
    "timed_out": 20,
    "skipped": 0,
    "cancelled": 0,
    "non_terminal": 1373,
    "missing_job": 0,
}
EXPECTED_TIER_ELIGIBLE_COUNTS = {1: 6, 2: 17}
EXPECTED_TIER_MATRIX_COUNTS = {1: 2, 2: 3}
EXPECTED_TIER_TOTAL_COUNTS = {1: 308, 2: 1108}
EXPECTED_ELIGIBLE_PLAN_INDEX_RANGES = ["1", "3", "6", "9", "237-238", "473-482", "720-722", "955-958"]


class ValidationError(Exception):
    """Raised when the bounded evaluation surface does not validate."""


# ---------------------------------------------------------------------------
# Basic JSON/path helpers
# ---------------------------------------------------------------------------


def repo_path(path_text: str) -> Path:
    path = Path(path_text)
    return path if path.is_absolute() else REPO_ROOT / path


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
        raise ValidationError(f"{repo_relative(path)}: file not found") from exc
    except json.JSONDecodeError as exc:
        raise ValidationError(f"{repo_relative(path)}: invalid JSON: {exc}") from exc
    if not isinstance(data, Mapping):
        raise ValidationError(f"{repo_relative(path)}: JSON root must be an object")
    return data


def write_json(path: Path, data: Mapping[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")


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
        return finishable.as_int(value, label)
    except finishable.ManifestError as exc:
        raise ValidationError(str(exc)) from exc


def optional_int(value: Any, label: str) -> Optional[int]:
    try:
        return finishable.optional_int(value, label)
    except finishable.ManifestError as exc:
        raise ValidationError(str(exc)) from exc


def nonempty_str(value: Any, label: str) -> str:
    text = str(value).strip() if value is not None else ""
    if not text:
        raise ValidationError(f"{label} must be a non-empty string")
    return text


def str_or_none(value: Any) -> Optional[str]:
    if value is None:
        return None
    text = str(value).strip()
    return text if text else None


def parse_ranges(raw_ranges: Any, label: str, plan_count: int = EXPECTED_DISCOVERY_ENTRY_COUNT) -> set[int]:
    try:
        return finishable.parse_plan_index_ranges(raw_ranges, label, plan_count)
    except finishable.ManifestError as exc:
        raise ValidationError(str(exc)) from exc


def make_ranges(values: Iterable[int]) -> list[str]:
    return finishable.make_plan_index_ranges(values)


def normalized_int_mapping(raw_value: Any, label: str) -> dict[str, int]:
    raw = object_at(raw_value, label)
    return {str(key): as_int(value, f"{label}.{key}") for key, value in raw.items()}


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
        raise ValidationError(f"{name} differs from derived finishable evaluation surface")
    path, expected_value, actual_value = diff
    raise ValidationError(
        f"{name} differs from derived finishable evaluation surface at {path}: "
        f"expected {compact_json(expected_value)}, got {compact_json(actual_value)}"
    )


# ---------------------------------------------------------------------------
# Source finishable manifest / Tier-view validation
# ---------------------------------------------------------------------------


def validate_snapshot_contract(snapshot_contract: Mapping[str, Any], label: str) -> None:
    if snapshot_contract.get("bounded_snapshot") is not True:
        raise ValidationError(f"{label}.bounded_snapshot must be true")
    if snapshot_contract.get("non_terminal_snapshot") is not True:
        raise ValidationError(f"{label}.non_terminal_snapshot must be true for run 24959959195")
    claim_scope = nonempty_str(snapshot_contract.get("claim_scope"), f"{label}.claim_scope")
    required_phrase = "not final/current finishable-size claims"
    if required_phrase not in claim_scope:
        raise ValidationError(f"{label}.claim_scope must preserve bounded-snapshot disclaimer: {required_phrase!r}")
    nonempty_str(snapshot_contract.get("observation_time_utc"), f"{label}.observation_time_utc")


def validate_source_finishable_manifest(
    manifest: Mapping[str, Any],
    *,
    finishable_manifest_path: Path,
) -> tuple[list[Mapping[str, Any]], dict[int, Mapping[str, Any]], dict[str, Any]]:
    label = repo_relative(finishable_manifest_path)
    if manifest.get("schema_name") != finishable.MANIFEST_SCHEMA:
        raise ValidationError(f"{label}.schema_name must be {finishable.MANIFEST_SCHEMA!r}")
    if as_int(manifest.get("schema_version"), f"{label}.schema_version") != finishable.SCHEMA_VERSION:
        raise ValidationError(f"{label}.schema_version must be {finishable.SCHEMA_VERSION}")
    if as_int(manifest.get("entry_count"), f"{label}.entry_count") != EXPECTED_FINISHABLE_ENTRY_COUNT:
        raise ValidationError(f"{label}.entry_count must preserve the 1416-entry finishable manifest contract")

    source_plan = object_at(manifest.get("source_plan"), f"{label}.source_plan")
    if as_int(source_plan.get("entry_count"), f"{label}.source_plan.entry_count") != EXPECTED_DISCOVERY_ENTRY_COUNT:
        raise ValidationError(f"{label}.source_plan.entry_count must preserve the 1416-entry discovery plan contract")
    if as_int(source_plan.get("timeout_sec"), f"{label}.source_plan.timeout_sec") != EXPECTED_TIMEOUT_SEC:
        raise ValidationError(f"{label}.source_plan.timeout_sec must be {EXPECTED_TIMEOUT_SEC}")

    source_paths = object_at(manifest.get("source_paths"), f"{label}.source_paths")
    nonempty_str(source_paths.get("problem_size_discovery_plan"), f"{label}.source_paths.problem_size_discovery_plan")
    nonempty_str(source_paths.get("discovery_provenance"), f"{label}.source_paths.discovery_provenance")

    run = object_at(manifest.get("run"), f"{label}.run")
    if as_int(run.get("database_id"), f"{label}.run.database_id") != 24959959195:
        raise ValidationError(f"{label}.run.database_id must remain the accepted bounded discovery run 24959959195")
    if run.get("non_terminal_snapshot") is not True:
        raise ValidationError(f"{label}.run.non_terminal_snapshot must be true for the bounded source snapshot")

    snapshot_contract = object_at(manifest.get("snapshot_contract"), f"{label}.snapshot_contract")
    validate_snapshot_contract(snapshot_contract, f"{label}.snapshot_contract")

    summary = object_at(manifest.get("summary"), f"{label}.summary")
    summary_entry_count = as_int(summary.get("entry_count"), f"{label}.summary.entry_count")
    if summary_entry_count != EXPECTED_FINISHABLE_ENTRY_COUNT:
        raise ValidationError(f"{label}.summary.entry_count must be {EXPECTED_FINISHABLE_ENTRY_COUNT}")
    eligible_count = as_int(summary.get("eligible_entry_count"), f"{label}.summary.eligible_entry_count")
    excluded_count = as_int(summary.get("excluded_entry_count"), f"{label}.summary.excluded_entry_count")
    if eligible_count != EXPECTED_ELIGIBLE_ENTRY_COUNT or excluded_count != EXPECTED_EXCLUDED_ENTRY_COUNT:
        raise ValidationError(
            f"{label}.summary must preserve 23 eligible / 1393 excluded entries "
            f"(got eligible={eligible_count}, excluded={excluded_count})"
        )
    eligible_ranges = list_at(summary.get("eligible_plan_index_ranges"), f"{label}.summary.eligible_plan_index_ranges")
    if eligible_ranges != EXPECTED_ELIGIBLE_PLAN_INDEX_RANGES:
        raise ValidationError(
            f"{label}.summary.eligible_plan_index_ranges must remain {EXPECTED_ELIGIBLE_PLAN_INDEX_RANGES}, "
            f"got {eligible_ranges}"
        )
    observed_counts = normalized_int_mapping(
        summary.get("observed_outcome_bucket_counts"), f"{label}.summary.observed_outcome_bucket_counts"
    )
    if observed_counts != EXPECTED_OBSERVED_OUTCOME_BUCKET_COUNTS:
        raise ValidationError(
            f"{label}.summary.observed_outcome_bucket_counts must preserve "
            f"23 completed_success / 20 timed_out / 1373 non_terminal entries, got {observed_counts}"
        )
    reconciliation = object_at(
        summary.get("requested_bucket_count_reconciliation"),
        f"{label}.summary.requested_bucket_count_reconciliation",
    )
    primary_counts = normalized_int_mapping(
        reconciliation.get("primary_bucket_counts"),
        f"{label}.summary.requested_bucket_count_reconciliation.primary_bucket_counts",
    )
    if primary_counts != EXPECTED_PRIMARY_BUCKET_COUNTS:
        raise ValidationError(
            f"{label}.summary.requested_bucket_count_reconciliation.primary_bucket_counts must preserve "
            f"{EXPECTED_PRIMARY_BUCKET_COUNTS}, got {primary_counts}"
        )
    if as_int(reconciliation.get("primary_bucket_total"), f"{label}.summary.requested_bucket_count_reconciliation.primary_bucket_total") != EXPECTED_DISCOVERY_ENTRY_COUNT:
        raise ValidationError(f"{label}.summary.requested_bucket_count_reconciliation.primary_bucket_total must be 1416")

    tier_counts = object_at(summary.get("tier_counts"), f"{label}.summary.tier_counts")
    for tier in (1, 2):
        tier_summary = object_at(tier_counts.get(str(tier)), f"{label}.summary.tier_counts.{tier}")
        if as_int(tier_summary.get("total"), f"{label}.summary.tier_counts.{tier}.total") != EXPECTED_TIER_TOTAL_COUNTS[tier]:
            raise ValidationError(f"{label}.summary.tier_counts.{tier}.total drifted")
        if as_int(tier_summary.get("eligible"), f"{label}.summary.tier_counts.{tier}.eligible") != EXPECTED_TIER_ELIGIBLE_COUNTS[tier]:
            raise ValidationError(f"{label}.summary.tier_counts.{tier}.eligible drifted")

    entries = [
        object_at(entry, f"{label}.entries[{offset}]")
        for offset, entry in enumerate(list_at(manifest.get("entries"), f"{label}.entries"))
    ]
    if len(entries) != EXPECTED_FINISHABLE_ENTRY_COUNT:
        raise ValidationError(f"{label}.entries length must be {EXPECTED_FINISHABLE_ENTRY_COUNT}, got {len(entries)}")

    eligible_entries: list[Mapping[str, Any]] = []
    eligible_by_index: dict[int, Mapping[str, Any]] = {}
    for offset, entry in enumerate(entries):
        entry_label = f"{label}.entries[{offset}]"
        plan_index = as_int(entry.get("plan_index"), f"{entry_label}.plan_index")
        if plan_index != offset + 1:
            raise ValidationError(f"{entry_label}.plan_index must be ordered and equal {offset + 1}, got {plan_index}")
        if not bool(entry.get("eligible_for_linear_workflow")):
            continue
        if entry.get("eligibility_status") != "eligible" or entry.get("exclusion_reason") is not None:
            raise ValidationError(f"{entry_label} eligible row must have eligibility_status='eligible' and no exclusion_reason")
        if str(entry.get("observed_outcome_bucket")) != "completed_success":
            raise ValidationError(f"{entry_label}.observed_outcome_bucket must be completed_success for evaluation entries")
        if str(entry.get("observed_outcome")) != "success" or str(entry.get("terminal_outcome")) != "success":
            raise ValidationError(f"{entry_label} must have successful observed and terminal outcomes")
        if str(entry.get("observed_job_status")) != "completed" or str(entry.get("observed_job_conclusion")) != "success":
            raise ValidationError(f"{entry_label} must have completed/success job evidence")
        duration_sec = as_int(entry.get("duration_sec"), f"{entry_label}.duration_sec")
        if duration_sec > EXPECTED_TIMEOUT_SEC:
            raise ValidationError(f"{entry_label}.duration_sec exceeds {EXPECTED_TIMEOUT_SEC}")
        if as_int(entry.get("timeout_sec"), f"{entry_label}.timeout_sec") != EXPECTED_TIMEOUT_SEC:
            raise ValidationError(f"{entry_label}.timeout_sec must be {EXPECTED_TIMEOUT_SEC}")
        if as_int(entry.get("outcome_timeout_sec"), f"{entry_label}.outcome_timeout_sec") != EXPECTED_TIMEOUT_SEC:
            raise ValidationError(f"{entry_label}.outcome_timeout_sec must be {EXPECTED_TIMEOUT_SEC}")
        nonempty_str(entry.get("benchmark"), f"{entry_label}.benchmark")
        nonempty_str(entry.get("kernel"), f"{entry_label}.kernel")
        nonempty_str(entry.get("problem_size"), f"{entry_label}.problem_size")
        nonempty_str(entry.get("hardware_real_ms"), f"{entry_label}.hardware_real_ms")
        if plan_index in eligible_by_index:
            raise ValidationError(f"duplicate eligible plan_index {plan_index}")
        eligible_by_index[plan_index] = entry
        eligible_entries.append(entry)

    eligible_indices = [as_int(entry["plan_index"], "entry.plan_index") for entry in eligible_entries]
    if len(eligible_entries) != EXPECTED_ELIGIBLE_ENTRY_COUNT:
        raise ValidationError(f"source finishable manifest must expose 23 eligible entries, got {len(eligible_entries)}")
    if make_ranges(eligible_indices) != EXPECTED_ELIGIBLE_PLAN_INDEX_RANGES:
        raise ValidationError(
            "source finishable manifest eligible entry rows do not match the accepted eligible ranges "
            f"{EXPECTED_ELIGIBLE_PLAN_INDEX_RANGES}: got {make_ranges(eligible_indices)}"
        )

    source_contracts = {
        "discovery_plan_entry_count": EXPECTED_DISCOVERY_ENTRY_COUNT,
        "finishable_manifest_entry_count": EXPECTED_FINISHABLE_ENTRY_COUNT,
        "eligible_entry_count": EXPECTED_ELIGIBLE_ENTRY_COUNT,
        "excluded_entry_count": EXPECTED_EXCLUDED_ENTRY_COUNT,
        "completed_success_entry_count": EXPECTED_OBSERVED_OUTCOME_BUCKET_COUNTS["completed_success"],
        "timed_out_entry_count": EXPECTED_OBSERVED_OUTCOME_BUCKET_COUNTS["timed_out"],
        "non_terminal_entry_count": EXPECTED_OBSERVED_OUTCOME_BUCKET_COUNTS["non_terminal"],
        "eligible_plan_index_ranges": EXPECTED_ELIGIBLE_PLAN_INDEX_RANGES,
        "observed_outcome_bucket_counts": EXPECTED_OBSERVED_OUTCOME_BUCKET_COUNTS,
        "requested_primary_bucket_counts": EXPECTED_PRIMARY_BUCKET_COUNTS,
        "requested_primary_bucket_total": EXPECTED_DISCOVERY_ENTRY_COUNT,
        "terminal_bucket_total": as_int(reconciliation.get("terminal_bucket_total"), f"{label}.summary.requested_bucket_count_reconciliation.terminal_bucket_total"),
    }
    return eligible_entries, eligible_by_index, source_contracts


def validate_tier_view(
    tier_view: Mapping[str, Any],
    *,
    tier: int,
    tier_path: Path,
    finishable_manifest_path: Path,
    source_provenance_path: str,
    eligible_by_index: Mapping[int, Mapping[str, Any]],
) -> list[dict[str, Any]]:
    label = repo_relative(tier_path)
    if tier_view.get("schema_name") != finishable.TIER_VIEW_SCHEMA:
        raise ValidationError(f"{label}.schema_name must be {finishable.TIER_VIEW_SCHEMA!r}")
    if as_int(tier_view.get("schema_version"), f"{label}.schema_version") != finishable.SCHEMA_VERSION:
        raise ValidationError(f"{label}.schema_version must be {finishable.SCHEMA_VERSION}")
    if as_int(tier_view.get("tier"), f"{label}.tier") != tier:
        raise ValidationError(f"{label}.tier must be {tier}")
    if tier_view.get("tier_name") != f"tier{tier}":
        raise ValidationError(f"{label}.tier_name must be 'tier{tier}'")
    if tier_view.get("eligible_only") is not True:
        raise ValidationError(f"{label}.eligible_only must be true")
    if tier_view.get("source_manifest") != repo_relative(finishable_manifest_path):
        raise ValidationError(
            f"{label}.source_manifest must point to {repo_relative(finishable_manifest_path)!r}, "
            f"got {tier_view.get('source_manifest')!r}"
        )
    if tier_view.get("source_provenance") != source_provenance_path:
        raise ValidationError(
            f"{label}.source_provenance must point to {source_provenance_path!r}, got {tier_view.get('source_provenance')!r}"
        )

    expected_tier_indices = {
        plan_index for plan_index, entry in eligible_by_index.items() if as_int(entry.get("tier"), f"entry[{plan_index}].tier") == tier
    }
    declared_ranges = list_at(tier_view.get("eligible_plan_index_ranges"), f"{label}.eligible_plan_index_ranges")
    declared_indices = parse_ranges(declared_ranges, f"{label}.eligible_plan_index_ranges")
    if declared_indices != expected_tier_indices:
        raise ValidationError(
            f"{label}.eligible_plan_index_ranges must match source eligible tier {tier} rows "
            f"(declared={make_ranges(declared_indices)}, actual={make_ranges(expected_tier_indices)})"
        )
    eligible_count = as_int(tier_view.get("eligible_entry_count"), f"{label}.eligible_entry_count")
    if eligible_count != len(expected_tier_indices) or eligible_count != EXPECTED_TIER_ELIGIBLE_COUNTS[tier]:
        raise ValidationError(f"{label}.eligible_entry_count must be {EXPECTED_TIER_ELIGIBLE_COUNTS[tier]}")

    include_rows = [
        object_at(row, f"{label}.include[{offset}]")
        for offset, row in enumerate(list_at(tier_view.get("include"), f"{label}.include"))
    ]
    if len(include_rows) != EXPECTED_TIER_MATRIX_COUNTS[tier]:
        raise ValidationError(f"{label}.include must contain {EXPECTED_TIER_MATRIX_COUNTS[tier]} matrix rows")

    seen_indices: set[int] = set()
    groups: list[dict[str, Any]] = []
    for offset, row in enumerate(include_rows):
        row_label = f"{label}.include[{offset}]"
        row_indices = parse_ranges(row.get("finishable_plan_index_ranges"), f"{row_label}.finishable_plan_index_ranges")
        if not row_indices:
            raise ValidationError(f"{row_label}.finishable_plan_index_ranges must not be empty")
        overlap = seen_indices & row_indices
        if overlap:
            raise ValidationError(f"{row_label}.finishable_plan_index_ranges duplicates plan indices {make_ranges(overlap)}")
        extra = row_indices - expected_tier_indices
        if extra:
            raise ValidationError(f"{row_label}.finishable_plan_index_ranges contains non-eligible tier indices {make_ranges(extra)}")
        seen_indices.update(row_indices)
        source_entries = [eligible_by_index[index] for index in sorted(row_indices)]
        benchmarks = {str(entry.get("benchmark")) for entry in source_entries}
        kernels = {str(entry.get("kernel")) for entry in source_entries}
        if len(benchmarks) != 1 or len(kernels) != 1:
            raise ValidationError(f"{row_label} must not mix benchmarks/kernels")
        benchmark = next(iter(benchmarks))
        kernel = next(iter(kernels))
        if row.get("benchmark") != benchmark:
            raise ValidationError(f"{row_label}.benchmark must be {benchmark!r}, got {row.get('benchmark')!r}")
        if row.get("tier") != f"tier{tier}":
            raise ValidationError(f"{row_label}.tier must be 'tier{tier}'")
        expected_sizes = " ".join(str(entry.get("sizes")) for entry in source_entries)
        if row.get("sizes") != expected_sizes:
            raise ValidationError(f"{row_label}.sizes must match source eligible sizes {expected_sizes!r}")
        for key in ("flag_template", "size_label_template", "timeout_sec"):
            expected_value = source_entries[0].get(key)
            if row.get(key) != expected_value:
                raise ValidationError(f"{row_label}.{key} must be {expected_value!r}, got {row.get(key)!r}")
        for key in ("extra_flags", "gomemlimit", "gogc"):
            expected_value = source_entries[0].get(key)
            if expected_value is None:
                if key in row:
                    raise ValidationError(f"{row_label}.{key} must be omitted when source entries do not set it")
            elif row.get(key) != expected_value:
                raise ValidationError(f"{row_label}.{key} must be {expected_value!r}, got {row.get(key)!r}")
        groups.append(build_benchmark_group(row, source_entries, tier))

    if seen_indices != expected_tier_indices:
        raise ValidationError(
            f"{label}.include rows must cover exactly eligible tier {tier} indices "
            f"(missing={make_ranges(expected_tier_indices - seen_indices)}, extra={make_ranges(seen_indices - expected_tier_indices)})"
        )
    return groups


# ---------------------------------------------------------------------------
# Evaluation manifest / summary construction
# ---------------------------------------------------------------------------


def build_evaluation_entry(entry: Mapping[str, Any], *, source_provenance_path: str) -> dict[str, Any]:
    plan_index = as_int(entry.get("plan_index"), "entry.plan_index")
    return {
        "plan_index": plan_index,
        "benchmark": str(entry.get("benchmark")),
        "kernel": str(entry.get("kernel")),
        "tier": as_int(entry.get("tier"), f"entry[{plan_index}].tier"),
        "tier_name": str(entry.get("tier_name")),
        "problem_size": str(entry.get("problem_size")),
        "hardware_real_ms": str(entry.get("hardware_real_ms")),
        "workflow": {
            "workflow_job": str(entry.get("workflow_job")),
            "workflow_benchmark": str(entry.get("workflow_benchmark")),
            "workflow_benchmark_name": str(entry.get("workflow_benchmark_name")),
            "sizes": str(entry.get("sizes")),
            "size_token": str(entry.get("size_token")),
            "size_label": str(entry.get("size_label")),
            "flag_template": str(entry.get("flag_template")),
            "size_label_template": str(entry.get("size_label_template")),
            "extra_flags": str_or_none(entry.get("extra_flags")),
            "gomemlimit": str_or_none(entry.get("gomemlimit")),
            "gogc": str_or_none(entry.get("gogc")),
            "timeout_sec": as_int(entry.get("timeout_sec"), f"entry[{plan_index}].timeout_sec"),
        },
        "source_ci": {
            "provenance_path": source_provenance_path,
            "run_id": as_int(entry.get("run_id"), f"entry[{plan_index}].run_id"),
            "run_url": str_or_none(entry.get("run_url")),
            "run_status": str_or_none(entry.get("run_status")),
            "run_conclusion": str_or_none(entry.get("run_conclusion")) or "",
            "run_head_branch": str_or_none(entry.get("run_head_branch")),
            "run_head_sha": str_or_none(entry.get("run_head_sha")),
            "discovery_shard": as_int(entry.get("discovery_shard"), f"entry[{plan_index}].discovery_shard"),
            "discovery_job_name": str_or_none(entry.get("discovery_job_name")),
            "discovery_job_database_id": as_int(
                entry.get("discovery_job_database_id"), f"entry[{plan_index}].discovery_job_database_id"
            ),
            "discovery_job_url": str_or_none(entry.get("discovery_job_url")),
            "artifact_database_id": as_int(entry.get("artifact_database_id"), f"entry[{plan_index}].artifact_database_id"),
            "artifact_name": str_or_none(entry.get("artifact_name")),
            "artifact_digest": str_or_none(entry.get("artifact_digest")),
        },
        "observed": {
            "duration_sec": as_int(entry.get("duration_sec"), f"entry[{plan_index}].duration_sec"),
            "outcome_bucket": str(entry.get("observed_outcome_bucket")),
            "outcome": str(entry.get("observed_outcome")),
            "job_status": str(entry.get("observed_job_status")),
            "job_conclusion": str(entry.get("observed_job_conclusion")),
            "terminal_outcome": str(entry.get("terminal_outcome")),
            "outcome_timeout_sec": as_int(entry.get("outcome_timeout_sec"), f"entry[{plan_index}].outcome_timeout_sec"),
        },
    }


def build_benchmark_group(matrix_row: Mapping[str, Any], source_entries: Sequence[Mapping[str, Any]], tier: int) -> dict[str, Any]:
    source_entries = sorted(source_entries, key=lambda entry: as_int(entry.get("plan_index"), "entry.plan_index"))
    plan_indices = [as_int(entry.get("plan_index"), "entry.plan_index") for entry in source_entries]
    benchmark = str(source_entries[0].get("benchmark"))
    kernel = str(source_entries[0].get("kernel"))
    matrix: dict[str, Any] = {
        "benchmark": matrix_row.get("benchmark"),
        "tier": matrix_row.get("tier"),
        "sizes": matrix_row.get("sizes"),
        "flag_template": matrix_row.get("flag_template"),
        "size_label_template": matrix_row.get("size_label_template"),
        "timeout_sec": matrix_row.get("timeout_sec"),
        "finishable_plan_index_ranges": list(matrix_row.get("finishable_plan_index_ranges", [])),
    }
    for key in ("extra_flags", "gomemlimit", "gogc"):
        if key in matrix_row:
            matrix[key] = matrix_row[key]
    return {
        "benchmark": benchmark,
        "kernel": kernel,
        "tier": tier,
        "tier_name": f"tier{tier}",
        "entry_count": len(source_entries),
        "plan_index_ranges": make_ranges(plan_indices),
        "problem_sizes": [str(entry.get("problem_size")) for entry in source_entries],
        "matrix": matrix,
        "entries": [
            {
                "plan_index": as_int(entry.get("plan_index"), "entry.plan_index"),
                "problem_size": str(entry.get("problem_size")),
                "hardware_real_ms": str(entry.get("hardware_real_ms")),
                "duration_sec": as_int(entry.get("duration_sec"), "entry.duration_sec"),
                "observed_outcome": str(entry.get("observed_outcome")),
            }
            for entry in source_entries
        ],
    }


def tier_counts_from_groups(groups: Sequence[Mapping[str, Any]], tier_views: Mapping[int, Mapping[str, Any]]) -> dict[str, Any]:
    tier_counts: dict[str, Any] = {}
    for tier in (1, 2):
        tier_groups = [group for group in groups if as_int(group.get("tier"), "group.tier") == tier]
        view = tier_views[tier]
        entry_count = sum(as_int(group.get("entry_count"), "group.entry_count") for group in tier_groups)
        tier_counts[f"tier{tier}"] = {
            "entry_count": entry_count,
            "benchmark_count": len(tier_groups),
            "matrix_entry_count": len(list_at(view.get("include"), f"tier{tier}.include")),
            "eligible_plan_index_ranges": list(view.get("eligible_plan_index_ranges", [])),
        }
    return tier_counts


def build_evaluation_manifest(
    *,
    finishable_manifest: Mapping[str, Any],
    finishable_manifest_path: Path,
    tier1_view: Mapping[str, Any],
    tier1_path: Path,
    tier2_view: Mapping[str, Any],
    tier2_path: Path,
) -> dict[str, Any]:
    eligible_entries, eligible_by_index, source_contracts = validate_source_finishable_manifest(
        finishable_manifest,
        finishable_manifest_path=finishable_manifest_path,
    )
    source_paths = object_at(finishable_manifest.get("source_paths"), "finishable_manifest.source_paths")
    source_provenance_path = nonempty_str(source_paths.get("discovery_provenance"), "source_paths.discovery_provenance")

    tier_groups: list[dict[str, Any]] = []
    tier_groups.extend(
        validate_tier_view(
            tier1_view,
            tier=1,
            tier_path=tier1_path,
            finishable_manifest_path=finishable_manifest_path,
            source_provenance_path=source_provenance_path,
            eligible_by_index=eligible_by_index,
        )
    )
    tier_groups.extend(
        validate_tier_view(
            tier2_view,
            tier=2,
            tier_path=tier2_path,
            finishable_manifest_path=finishable_manifest_path,
            source_provenance_path=source_provenance_path,
            eligible_by_index=eligible_by_index,
        )
    )

    grouped_indices = {
        as_int(row.get("plan_index"), "group.entries.plan_index")
        for group in tier_groups
        for row in list_at(group.get("entries"), "group.entries")
    }
    eligible_indices = {as_int(entry.get("plan_index"), "entry.plan_index") for entry in eligible_entries}
    if grouped_indices != eligible_indices:
        raise ValidationError(
            "Tier-view groups must reconcile exactly to source eligible entries "
            f"(missing={make_ranges(eligible_indices - grouped_indices)}, extra={make_ranges(grouped_indices - eligible_indices)})"
        )

    snapshot_contract = dict(object_at(finishable_manifest.get("snapshot_contract"), "finishable_manifest.snapshot_contract"))
    bounded_snapshot_disclaimer = (
        "This evaluation surface includes only completed-success entries from the bounded 3600s discovery snapshot. "
        "Timed-out and non-terminal source rows remain visible only in source-contract counts and must not be described "
        "as permanent unfinishable claims without new terminal evidence."
    )
    run = dict(object_at(finishable_manifest.get("run"), "finishable_manifest.run"))
    observation = dict(object_at(finishable_manifest.get("observation"), "finishable_manifest.observation"))
    source_path_block = {
        "finishable_manifest": repo_relative(finishable_manifest_path),
        "tier1_view": repo_relative(tier1_path),
        "tier2_view": repo_relative(tier2_path),
        "problem_size_discovery_plan": source_paths.get("problem_size_discovery_plan"),
        "discovery_provenance": source_provenance_path,
    }
    tier_counts = tier_counts_from_groups(tier_groups, {1: tier1_view, 2: tier2_view})
    entries = [build_evaluation_entry(entry, source_provenance_path=source_provenance_path) for entry in eligible_entries]
    return {
        "schema_name": EVALUATION_SCHEMA,
        "schema_version": SCHEMA_VERSION,
        "set_id": SET_ID,
        "set_version": SET_VERSION,
        "status": "current_bounded_finishable_surface",
        "selection_basis": {
            "collection_run_id": "",
            "source": "bounded finishable-size manifest plus checked-in eligible-only Tier views",
            "policy": "include every source entry with eligible_for_linear_workflow=true and no others",
        },
        "source_paths": source_path_block,
        "source_run": {
            "database_id": run.get("database_id"),
            "url": run.get("url"),
            "workflow_name": run.get("workflow_name"),
            "event": run.get("event"),
            "head_branch": run.get("head_branch"),
            "head_sha": run.get("head_sha"),
            "status": run.get("status"),
            "conclusion": run.get("conclusion") or "",
            "non_terminal_snapshot": bool(run.get("non_terminal_snapshot")),
            "observed_at_utc": run.get("observed_at_utc"),
        },
        "observation": {
            "observed_at_utc": observation.get("observed_at_utc"),
            "source": observation.get("source"),
            "non_terminal_snapshot": bool(observation.get("non_terminal_snapshot")),
        },
        "snapshot_contract": snapshot_contract,
        "bounded_snapshot_disclaimer": bounded_snapshot_disclaimer,
        "source_contracts": source_contracts,
        "selection": {
            "eligible_only": True,
            "source_eligibility_field": "eligible_for_linear_workflow",
            "excluded_eligible_entries": [],
            "included_entry_count": len(entries),
            "included_plan_index_ranges": make_ranges(eligible_indices),
            "non_included_source_rows_policy": (
                "all non-included source rows are excluded in the finishable-size manifest; the evaluation set does not "
                "promote timed-out or non-terminal rows"
            ),
        },
        "entry_count": len(entries),
        "benchmark_count": len(tier_groups),
        "tier_counts": tier_counts,
        "benchmarks": tier_groups,
        "entries": entries,
    }


def build_summary(evaluation_manifest: Mapping[str, Any], *, evaluation_manifest_path: Path) -> dict[str, Any]:
    entries = [object_at(entry, f"evaluation.entries[{offset}]") for offset, entry in enumerate(list_at(evaluation_manifest.get("entries"), "evaluation.entries"))]
    benchmarks = [
        object_at(group, f"evaluation.benchmarks[{offset}]")
        for offset, group in enumerate(list_at(evaluation_manifest.get("benchmarks"), "evaluation.benchmarks"))
    ]
    benchmark_summaries: list[dict[str, Any]] = []
    for group in benchmarks:
        group_entries = [object_at(row, "benchmark.entries[]") for row in list_at(group.get("entries"), "benchmark.entries")]
        durations = [as_int(row.get("duration_sec"), "benchmark.entries.duration_sec") for row in group_entries]
        benchmark_summaries.append(
            {
                "benchmark": group.get("benchmark"),
                "kernel": group.get("kernel"),
                "tier": group.get("tier"),
                "tier_name": group.get("tier_name"),
                "entry_count": group.get("entry_count"),
                "plan_index_ranges": group.get("plan_index_ranges"),
                "problem_sizes": group.get("problem_sizes"),
                "duration_sec_min": min(durations),
                "duration_sec_max": max(durations),
            }
        )

    source_contracts = object_at(evaluation_manifest.get("source_contracts"), "evaluation.source_contracts")
    return {
        "schema_name": SUMMARY_SCHEMA,
        "schema_version": SCHEMA_VERSION,
        "validation_status": "pass",
        "source_evaluation_manifest": repo_relative(evaluation_manifest_path),
        "set_id": evaluation_manifest.get("set_id"),
        "set_version": evaluation_manifest.get("set_version"),
        "source_paths": evaluation_manifest.get("source_paths"),
        "source_run": evaluation_manifest.get("source_run"),
        "snapshot_contract": evaluation_manifest.get("snapshot_contract"),
        "bounded_snapshot_disclaimer": evaluation_manifest.get("bounded_snapshot_disclaimer"),
        "source_contracts": source_contracts,
        "selection": evaluation_manifest.get("selection"),
        "entry_count": len(entries),
        "benchmark_count": len(benchmarks),
        "tier_counts": evaluation_manifest.get("tier_counts"),
        "benchmark_summaries": benchmark_summaries,
    }


# ---------------------------------------------------------------------------
# Checked-in evaluation manifest/summary validation
# ---------------------------------------------------------------------------


def validate_evaluation_manifest_shape(
    manifest: Mapping[str, Any],
    *,
    expected_eligible_indices: set[int],
    label: str,
) -> None:
    if manifest.get("schema_name") != EVALUATION_SCHEMA:
        raise ValidationError(f"{label}.schema_name must be {EVALUATION_SCHEMA!r}")
    if as_int(manifest.get("schema_version"), f"{label}.schema_version") != SCHEMA_VERSION:
        raise ValidationError(f"{label}.schema_version must be {SCHEMA_VERSION}")
    selection = object_at(manifest.get("selection"), f"{label}.selection")
    if selection.get("eligible_only") is not True:
        raise ValidationError(f"{label}.selection.eligible_only must be true")
    exclusions = list_at(selection.get("excluded_eligible_entries"), f"{label}.selection.excluded_eligible_entries")
    for offset, exclusion in enumerate(exclusions):
        item = object_at(exclusion, f"{label}.selection.excluded_eligible_entries[{offset}]")
        plan_index = as_int(item.get("plan_index"), f"{label}.selection.excluded_eligible_entries[{offset}].plan_index")
        if plan_index not in expected_eligible_indices:
            raise ValidationError(f"{label}.selection.excluded_eligible_entries[{offset}] references non-eligible plan_index {plan_index}")
        nonempty_str(item.get("justification"), f"{label}.selection.excluded_eligible_entries[{offset}].justification")
    if exclusions:
        raise ValidationError(
            f"{label}.selection.excluded_eligible_entries must be empty for the current finishable surface; "
            "future exclusions require regenerating the expected manifest from an explicit policy"
        )

    entries = [object_at(entry, f"{label}.entries[{offset}]") for offset, entry in enumerate(list_at(manifest.get("entries"), f"{label}.entries"))]
    actual_indices = {as_int(entry.get("plan_index"), f"{label}.entries[].plan_index") for entry in entries}
    if actual_indices != expected_eligible_indices:
        raise ValidationError(
            f"{label}.entries must include all and only eligible finishable entries "
            f"(missing={make_ranges(expected_eligible_indices - actual_indices)}, extra={make_ranges(actual_indices - expected_eligible_indices)})"
        )
    if as_int(manifest.get("entry_count"), f"{label}.entry_count") != len(entries):
        raise ValidationError(f"{label}.entry_count must equal entries length")
    validate_snapshot_contract(object_at(manifest.get("snapshot_contract"), f"{label}.snapshot_contract"), f"{label}.snapshot_contract")


def validate_paths(
    *,
    finishable_manifest_path: Path,
    tier1_path: Path,
    tier2_path: Path,
    evaluation_manifest_path: Path,
    evaluation_summary_path: Path,
    update: bool = False,
) -> dict[str, Any]:
    finishable_manifest = load_json_object(finishable_manifest_path)
    tier1_view = load_json_object(tier1_path)
    tier2_view = load_json_object(tier2_path)
    expected_manifest = build_evaluation_manifest(
        finishable_manifest=finishable_manifest,
        finishable_manifest_path=finishable_manifest_path,
        tier1_view=tier1_view,
        tier1_path=tier1_path,
        tier2_view=tier2_view,
        tier2_path=tier2_path,
    )
    expected_summary = build_summary(expected_manifest, evaluation_manifest_path=evaluation_manifest_path)

    if update:
        write_json(evaluation_manifest_path, expected_manifest)
        write_json(evaluation_summary_path, expected_summary)

    actual_manifest = load_json_object(evaluation_manifest_path)
    expected_eligible_indices = {as_int(entry.get("plan_index"), "expected.entries[].plan_index") for entry in expected_manifest["entries"]}
    validate_evaluation_manifest_shape(
        actual_manifest,
        expected_eligible_indices=expected_eligible_indices,
        label=repo_relative(evaluation_manifest_path),
    )
    require_equal(repo_relative(evaluation_manifest_path), expected_manifest, actual_manifest)

    actual_summary = load_json_object(evaluation_summary_path)
    require_equal(repo_relative(evaluation_summary_path), expected_summary, actual_summary)
    return expected_summary


def format_text_summary(summary: Mapping[str, Any]) -> str:
    source_contracts = object_at(summary.get("source_contracts"), "summary.source_contracts")
    tier_counts = object_at(summary.get("tier_counts"), "summary.tier_counts")
    source_run = object_at(summary.get("source_run"), "summary.source_run")
    lines = [
        "MI300A finishable evaluation set validation: pass",
        f"set={summary.get('set_id')} version={summary.get('set_version')}",
        f"source_run={source_run.get('database_id')} observed_at={source_run.get('observed_at_utc')} bounded_snapshot={source_run.get('non_terminal_snapshot')}",
        (
            "contract: "
            f"discovery_entries={source_contracts.get('discovery_plan_entry_count')} "
            f"finishable_entries={source_contracts.get('finishable_manifest_entry_count')} "
            f"eligible={source_contracts.get('eligible_entry_count')} "
            f"timed_out={source_contracts.get('timed_out_entry_count')} "
            f"non_terminal={source_contracts.get('non_terminal_entry_count')}"
        ),
        f"entries={summary.get('entry_count')} benchmarks={summary.get('benchmark_count')}",
    ]
    for tier_name in ("tier1", "tier2"):
        tier = object_at(tier_counts.get(tier_name), f"summary.tier_counts.{tier_name}")
        lines.append(
            f"{tier_name}: entries={tier.get('entry_count')} benchmarks={tier.get('benchmark_count')} "
            f"matrix_rows={tier.get('matrix_entry_count')} ranges={','.join(tier.get('eligible_plan_index_ranges', []))}"
        )
    lines.append(str(summary.get("bounded_snapshot_disclaimer")))
    return "\n".join(lines) + "\n"


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Validate and summarize the bounded finishable MI300A evaluation set."
    )
    parser.add_argument(
        "--finishable-manifest",
        default=str(DEFAULT_FINISHABLE_MANIFEST),
        help="Path to mi300a_finishable_size_manifest.json",
    )
    parser.add_argument("--tier1", default=str(DEFAULT_TIER1_VIEW), help="Path to Tier 1 finishable matrix view")
    parser.add_argument("--tier2", default=str(DEFAULT_TIER2_VIEW), help="Path to Tier 2 finishable matrix view")
    parser.add_argument(
        "--evaluation-manifest",
        default=str(DEFAULT_EVALUATION_MANIFEST),
        help="Path to mi300a_finishable_evaluation_set.json",
    )
    parser.add_argument(
        "--summary",
        default=str(DEFAULT_EVALUATION_SUMMARY),
        help="Path to mi300a_finishable_evaluation_set_summary.json",
    )
    parser.add_argument(
        "--summary-format",
        choices=("json", "text"),
        default="json",
        help="Format to print to stdout after validation succeeds",
    )
    parser.add_argument(
        "--update",
        action="store_true",
        help="Write the derived evaluation manifest and summary before validating them",
    )
    return parser.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    args = parse_args(argv)
    try:
        summary = validate_paths(
            finishable_manifest_path=repo_path(args.finishable_manifest),
            tier1_path=repo_path(args.tier1),
            tier2_path=repo_path(args.tier2),
            evaluation_manifest_path=repo_path(args.evaluation_manifest),
            evaluation_summary_path=repo_path(args.summary),
            update=bool(args.update),
        )
    except ValidationError as exc:
        print("FAIL: MI300A finishable evaluation set validation rejected input.", file=sys.stderr)
        print(f"  - {exc}", file=sys.stderr)
        return 1

    if args.summary_format == "json":
        print(json.dumps(summary, indent=2, sort_keys=True))
    else:
        print(format_text_summary(summary), end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
