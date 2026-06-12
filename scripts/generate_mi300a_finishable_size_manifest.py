#!/usr/bin/env python3
"""Generate the MI300A finishable-size manifest from problem-size discovery evidence.

The generator is intentionally deterministic and evidence-only: it reconciles the
checked-in 1416-entry problem-size discovery plan with compact CI outcome
provenance for the discovery run, then emits one manifest row per plan entry.
Rows are eligible for the future linear workflow only when the compact evidence
shows a successful terminal discovery outcome under the 3600s per-size budget.
All non-terminal, missing, failed, timed-out, skipped, and cancelled entries stay
visible as explicit exclusions.
"""

from __future__ import annotations

import argparse
import collections
import json
import math
import re
import sys
from pathlib import Path
from typing import Any, Iterable, Mapping, Optional, Sequence


REPO_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_PLAN = REPO_ROOT / "benchmark-comparison" / "mi300a_problem_size_discovery_plan.json"
DEFAULT_PROVENANCE = (
    REPO_ROOT
    / "benchmark-comparison"
    / "provenance"
    / "mi300a_problem_size_discovery_run_24959959195.json"
)
DEFAULT_MANIFEST = REPO_ROOT / "benchmark-comparison" / "mi300a_finishable_size_manifest.json"
DEFAULT_TIER1_VIEW = REPO_ROOT / "benchmark-comparison" / "generated" / "mi300a_finishable_tier1_matrix.json"
DEFAULT_TIER2_VIEW = REPO_ROOT / "benchmark-comparison" / "generated" / "mi300a_finishable_tier2_matrix.json"
DEFAULT_SUMMARY = REPO_ROOT / "benchmark-comparison" / "mi300a_finishable_size_summary.json"

MANIFEST_SCHEMA = "mgpusim.mi300a_finishable_size_manifest"
SUMMARY_SCHEMA = "mgpusim.mi300a_finishable_size_summary"
TIER_VIEW_SCHEMA = "mgpusim.mi300a_finishable_tier_matrix_view"
TERMINAL_ACCOUNTING_SCHEMA = "mgpusim.mi300a_terminal_outcome_accounting"
SOURCE_POLICY_SCHEMA = "mgpusim.mi300a_terminal_source_policy"
SOURCE_POLICY_NAME = "single_run_terminal_source_identity"
SCHEMA_VERSION = 1
EXPECTED_PLAN_SCHEMA = "mgpusim.mi300a_problem_size_discovery_plan"
EXPECTED_PROVENANCE_SCHEMA = "mgpusim.mi300a_problem_size_discovery_provenance"
EXPECTED_ENTRY_COUNT = 1416
EXPECTED_TIMEOUT_SEC = 3600
DISCOVERY_SHARD_COUNT = 6

PRIMARY_BUCKETS = (
    "completed_success",
    "failed",
    "timed_out",
    "skipped",
    "cancelled",
    "non_terminal",
    "missing_job",
)
TERMINAL_BUCKETS = ("completed_success", "failed", "timed_out", "skipped", "cancelled")
NON_TERMINAL_SUBSTATUS_BUCKETS = ("non_terminal_in_progress", "non_terminal_queued")
NON_BOUNDED_BLOCKING_BUCKETS = (
    "non_terminal",
    "non_terminal_in_progress",
    "non_terminal_queued",
    "missing_job",
    "missing_outcome_row",
)
DIAGNOSTIC_BUCKETS = (
    "missing_artifact_for_completed_success_job",
    "missing_outcome_row_for_completed_success_job",
    "missing_outcome_row",
)
OUTCOME_TO_PRIMARY_BUCKET = {
    "success": "completed_success",
    "timeout": "timed_out",
    "timed_out": "timed_out",
    "cancelled": "cancelled",
    "canceled": "cancelled",
    "skipped": "skipped",
    "crash": "failed",
    "failure": "failed",
    "failed": "failed",
    "missing_output": "failed",
    "missing_kernel_time": "failed",
}


class ManifestError(Exception):
    """Raised when manifest generation inputs violate the evidence contract."""


# ---------------------------------------------------------------------------
# Basic helpers
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
        raise ManifestError(f"{repo_relative(path)}: file not found") from exc
    except json.JSONDecodeError as exc:
        raise ManifestError(f"{repo_relative(path)}: invalid JSON: {exc}") from exc
    if not isinstance(data, Mapping):
        raise ManifestError(f"{repo_relative(path)}: JSON root must be an object")
    return data


def write_json(path: Path, data: Mapping[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")


def as_int(value: Any, label: str) -> int:
    if isinstance(value, bool):
        raise ManifestError(f"{label} must be an integer, got boolean {value!r}")
    if isinstance(value, int):
        return value
    if isinstance(value, str) and value.strip().isdigit():
        return int(value.strip())
    raise ManifestError(f"{label} must be an integer, got {value!r}")


def optional_int(value: Any, label: str) -> Optional[int]:
    if value is None or value == "":
        return None
    return as_int(value, label)


def str_or_none(value: Any) -> Optional[str]:
    if value is None:
        return None
    text = str(value).strip()
    return text if text else None


def nonempty_str(value: Any, label: str) -> str:
    text = str_or_none(value)
    if text is None:
        raise ManifestError(f"{label} must be a non-empty string")
    return text


def object_at(value: Any, label: str) -> Mapping[str, Any]:
    if not isinstance(value, Mapping):
        raise ManifestError(f"{label} must be an object")
    return value


def list_at(value: Any, label: str) -> list[Any]:
    if not isinstance(value, list):
        raise ManifestError(f"{label} must be a list")
    return value


def stringify_counts(counter: Mapping[Any, int]) -> dict[str, int]:
    return {str(key): int(counter[key]) for key in sorted(counter, key=lambda item: str(item))}


def bucket_count_reconciliation(bucket_sets: Mapping[str, set[int]]) -> dict[str, Any]:
    """Return explicit zero-inclusive bucket counts for provenance/output checks.

    Primary buckets partition the 1416-entry discovery plan.  Diagnostic buckets
    are overlays used to make missing artifact/outcome evidence visible; they do
    not need to partition the plan, but their counts are still emitted so stale
    provenance drift cannot hide in omitted zero-valued fields.
    """

    primary_counts = {bucket: len(bucket_sets[bucket]) for bucket in PRIMARY_BUCKETS}
    terminal_counts = {bucket: primary_counts[bucket] for bucket in TERMINAL_BUCKETS}
    diagnostic_counts = {bucket: len(bucket_sets[bucket]) for bucket in DIAGNOSTIC_BUCKETS}
    non_terminal_substatus_counts = {bucket: len(bucket_sets[bucket]) for bucket in NON_TERMINAL_SUBSTATUS_BUCKETS}
    return {
        "primary_bucket_counts": primary_counts,
        "primary_bucket_total": sum(primary_counts.values()),
        "terminal_bucket_counts": terminal_counts,
        "terminal_bucket_total": sum(terminal_counts.values()),
        "non_terminal_substatus_bucket_counts": non_terminal_substatus_counts,
        "non_terminal_substatus_bucket_total": sum(non_terminal_substatus_counts.values()),
        "diagnostic_bucket_counts": diagnostic_counts,
        "diagnostic_bucket_total": sum(diagnostic_counts.values()),
    }


# ---------------------------------------------------------------------------
# Range helpers
# ---------------------------------------------------------------------------


_RANGE_RE = re.compile(r"^(?P<start>\d+)(?:-(?P<end>\d+))?$")
_JOB_NAME_RE = re.compile(r"^Problem-Size Discovery (?P<shard>\d+): plan (?P<plan_index>\d+)$")


def parse_plan_index_ranges(raw_ranges: Any, label: str, plan_count: int) -> set[int]:
    ranges = list_at(raw_ranges, label)
    values: set[int] = set()
    for offset, raw_range in enumerate(ranges):
        if not isinstance(raw_range, str):
            raise ManifestError(f"{label}[{offset}] must be a string range, got {raw_range!r}")
        text = raw_range.strip()
        match = _RANGE_RE.match(text)
        if match is None:
            raise ManifestError(f"{label}[{offset}] has invalid range syntax {raw_range!r}")
        start = int(match.group("start"))
        end = int(match.group("end") or start)
        if start < 1 or end > plan_count or start > end:
            raise ManifestError(
                f"{label}[{offset}] range {raw_range!r} is outside valid plan indices 1..{plan_count}"
            )
        overlap = values.intersection(range(start, end + 1))
        if overlap:
            sample = sorted(overlap)[:5]
            raise ManifestError(f"{label}[{offset}] overlaps an earlier range at plan indices {sample}")
        values.update(range(start, end + 1))
    return values


def make_plan_index_ranges(values: Iterable[int]) -> list[str]:
    ordered = sorted(set(values))
    if not ordered:
        return []
    ranges: list[str] = []
    start = prev = ordered[0]
    for value in ordered[1:]:
        if value == prev + 1:
            prev = value
            continue
        ranges.append(f"{start}-{prev}" if start != prev else str(start))
        start = prev = value
    ranges.append(f"{start}-{prev}" if start != prev else str(start))
    return ranges


def discovery_shard_for_plan_index(plan_index: int, plan_count: int) -> int:
    chunk_size = max(1, math.ceil(plan_count / DISCOVERY_SHARD_COUNT))
    return min(DISCOVERY_SHARD_COUNT, ((plan_index - 1) // chunk_size) + 1)


def expected_discovery_job_name(plan_index: int, plan_count: int) -> str:
    return f"Problem-Size Discovery {discovery_shard_for_plan_index(plan_index, plan_count)}: plan {plan_index}"


# ---------------------------------------------------------------------------
# Input validation/parsing
# ---------------------------------------------------------------------------


def load_and_validate_plan(path: Path) -> tuple[Mapping[str, Any], list[Mapping[str, Any]], dict[int, Mapping[str, Any]]]:
    plan = load_json_object(path)
    label = repo_relative(path)
    if plan.get("schema_name") != EXPECTED_PLAN_SCHEMA:
        raise ManifestError(f"{label}.schema_name must be {EXPECTED_PLAN_SCHEMA!r}, got {plan.get('schema_name')!r}")

    entries = list_at(plan.get("entries"), f"{label}.entries")
    entry_count = as_int(plan.get("entry_count"), f"{label}.entry_count")
    if entry_count != len(entries):
        raise ManifestError(f"{label}.entry_count must match entries length ({entry_count} != {len(entries)})")
    if entry_count != EXPECTED_ENTRY_COUNT:
        raise ManifestError(f"{label}.entry_count must be {EXPECTED_ENTRY_COUNT}, got {entry_count}")

    by_index: dict[int, Mapping[str, Any]] = {}
    hotspot_48_indices: list[int] = []
    for offset, raw_entry in enumerate(entries):
        entry_label = f"{label}.entries[{offset}]"
        entry = object_at(raw_entry, entry_label)
        plan_index = as_int(entry.get("plan_index"), f"{entry_label}.plan_index")
        expected_index = offset + 1
        if plan_index != expected_index:
            raise ManifestError(f"{entry_label}.plan_index must be {expected_index}, got {plan_index}")
        timeout_sec = as_int(entry.get("timeout_sec"), f"{entry_label}.timeout_sec")
        if timeout_sec != EXPECTED_TIMEOUT_SEC:
            raise ManifestError(f"{entry_label}.timeout_sec must be {EXPECTED_TIMEOUT_SEC}, got {timeout_sec}")
        sizes = nonempty_str(entry.get("sizes"), f"{entry_label}.sizes")
        if len(sizes.split()) != 1:
            raise ManifestError(f"{entry_label}.sizes must contain exactly one problem-size token")
        for required_key in (
            "benchmark",
            "problem_size",
            "hardware_kernel_name",
            "hardware_problem_size",
            "workflow_job",
            "workflow_benchmark_name",
            "tier",
            "tier_name",
            "artifact_id",
            "flag_template",
            "size_label_template",
        ):
            nonempty_str(entry.get(required_key), f"{entry_label}.{required_key}")
        tier = as_int(entry.get("tier"), f"{entry_label}.tier")
        if tier not in (1, 2):
            raise ManifestError(f"{entry_label}.tier must be 1 or 2, got {tier}")
        key = (str(entry.get("hardware_kernel_name", "")).lower(), str(entry.get("hardware_problem_size", "")))
        if key == ("hotspot", "48"):
            hotspot_48_indices.append(plan_index)
        by_index[plan_index] = entry

    if hotspot_48_indices != [474]:
        raise ManifestError(f"{label}: expected exactly hotspot/48 at plan_index 474, got {hotspot_48_indices}")
    return plan, [object_at(entry, f"{label}.entries[{i}]") for i, entry in enumerate(entries)], by_index


def load_and_validate_provenance(path: Path, plan_path: Path, plan_count: int) -> Mapping[str, Any]:
    provenance = load_json_object(path)
    label = repo_relative(path)
    if provenance.get("schema_name") != EXPECTED_PROVENANCE_SCHEMA:
        raise ManifestError(
            f"{label}.schema_name must be {EXPECTED_PROVENANCE_SCHEMA!r}, got {provenance.get('schema_name')!r}"
        )
    run = object_at(provenance.get("run"), f"{label}.run")
    run_id = as_int(run.get("database_id"), f"{label}.run.database_id")
    if run_id <= 0:
        raise ManifestError(f"{label}.run.database_id must be positive, got {run_id}")
    run_status = nonempty_str(run.get("status"), f"{label}.run.status")
    non_terminal_snapshot = bool(run.get("non_terminal_snapshot"))
    if run_status == "completed":
        if non_terminal_snapshot:
            raise ManifestError(f"{label}.run.non_terminal_snapshot must be false when run.status is 'completed'")
    elif not non_terminal_snapshot:
        raise ManifestError(
            f"{label}.run.non_terminal_snapshot must be true when run.status is non-terminal ({run_status!r})"
        )
    plan_summary = object_at(provenance.get("plan_summary"), f"{label}.plan_summary")
    artifact = nonempty_str(plan_summary.get("artifact"), f"{label}.plan_summary.artifact")
    if artifact != repo_relative(plan_path):
        raise ManifestError(f"{label}.plan_summary.artifact must be {repo_relative(plan_path)!r}, got {artifact!r}")
    entry_count = as_int(plan_summary.get("entry_count"), f"{label}.plan_summary.entry_count")
    if entry_count != plan_count:
        raise ManifestError(f"{label}.plan_summary.entry_count must be {plan_count}, got {entry_count}")
    timeout_sec = as_int(plan_summary.get("timeout_sec"), f"{label}.plan_summary.timeout_sec")
    if timeout_sec != EXPECTED_TIMEOUT_SEC:
        raise ManifestError(f"{label}.plan_summary.timeout_sec must be {EXPECTED_TIMEOUT_SEC}, got {timeout_sec}")
    return provenance


def parse_bucket_sets(provenance: Mapping[str, Any], plan_count: int, label: str) -> dict[str, set[int]]:
    requested = object_at(provenance.get("requested_plan_outcome_buckets"), f"{label}.requested_plan_outcome_buckets")
    declared_plan_count = as_int(
        requested.get("plan_entry_count"), f"{label}.requested_plan_outcome_buckets.plan_entry_count"
    )
    if declared_plan_count != plan_count:
        raise ManifestError(
            f"{label}.requested_plan_outcome_buckets.plan_entry_count must be {plan_count}, "
            f"got {declared_plan_count}"
        )
    bucket_sets: dict[str, set[int]] = {}
    for bucket in (*PRIMARY_BUCKETS, *NON_TERMINAL_SUBSTATUS_BUCKETS, *DIAGNOSTIC_BUCKETS):
        ranges_key = f"{bucket}_plan_index_ranges"
        count_key = f"{bucket}_count"
        raw_ranges = requested.get(ranges_key, [])
        values = parse_plan_index_ranges(raw_ranges, f"{label}.requested_plan_outcome_buckets.{ranges_key}", plan_count)
        if count_key in requested:
            count = as_int(requested.get(count_key), f"{label}.requested_plan_outcome_buckets.{count_key}")
            if count != len(values):
                raise ManifestError(
                    f"{label}.requested_plan_outcome_buckets.{count_key} must match range cardinality "
                    f"({count} != {len(values)})"
                )
        bucket_sets[bucket] = values

    all_plan_indices = set(range(1, plan_count + 1))
    primary_seen: dict[int, str] = {}
    overlaps: list[str] = []
    for bucket in PRIMARY_BUCKETS:
        for plan_index in bucket_sets[bucket]:
            previous = primary_seen.setdefault(plan_index, bucket)
            if previous != bucket:
                overlaps.append(f"plan {plan_index}: {previous} and {bucket}")
    if overlaps:
        raise ManifestError("primary outcome buckets overlap: " + "; ".join(overlaps[:10]))
    missing = all_plan_indices - set(primary_seen)
    extra = set(primary_seen) - all_plan_indices
    if missing or extra:
        raise ManifestError(
            "primary outcome buckets must cover every plan entry exactly once "
            f"(missing={make_plan_index_ranges(missing)}, extra={make_plan_index_ranges(extra)})"
        )

    non_terminal = bucket_sets["non_terminal"]
    substatus_union: set[int] = set()
    for bucket in NON_TERMINAL_SUBSTATUS_BUCKETS:
        values = bucket_sets[bucket]
        if not values.issubset(non_terminal):
            raise ManifestError(
                f"{label}.requested_plan_outcome_buckets.{bucket}_plan_index_ranges must be a subset of non_terminal "
                f"(extra={make_plan_index_ranges(values - non_terminal)})"
            )
        if substatus_union & values:
            raise ManifestError(f"non-terminal substatus buckets overlap at {make_plan_index_ranges(substatus_union & values)}")
        substatus_union |= values
    if substatus_union and substatus_union != non_terminal:
        raise ManifestError(
            "non_terminal_in_progress + non_terminal_queued ranges must cover non_terminal ranges "
            f"(missing={make_plan_index_ranges(non_terminal - substatus_union)})"
        )

    # Completed-success diagnostics are narrower overlays, not primary outcome buckets.
    for bucket in ("missing_artifact_for_completed_success_job", "missing_outcome_row_for_completed_success_job"):
        values = bucket_sets[bucket]
        if not values.issubset(bucket_sets["completed_success"]):
            raise ManifestError(
                f"{label}.requested_plan_outcome_buckets.{bucket}_plan_index_ranges must be a subset of completed_success "
                f"(extra={make_plan_index_ranges(values - bucket_sets['completed_success'])})"
            )

    terminal_mode = requested.get("terminal_mode") is True or requested.get("terminal_provenance") is True
    if terminal_mode:
        disallowed = {
            "non_terminal": bucket_sets["non_terminal"],
            "missing_job": bucket_sets["missing_job"],
            "missing_outcome_row": bucket_sets["missing_outcome_row"],
            "non_terminal_in_progress": bucket_sets["non_terminal_in_progress"],
            "non_terminal_queued": bucket_sets["non_terminal_queued"],
        }
        non_empty = {bucket: make_plan_index_ranges(values) for bucket, values in disallowed.items() if values}
        if non_empty:
            raise ManifestError(
                "terminal-mode provenance must account every plan entry as completed/failed/timed_out/skipped/cancelled; "
                f"non-terminal or missing buckets are not allowed ({non_empty})"
            )
        terminal_indices = set().union(*(bucket_sets[bucket] for bucket in TERMINAL_BUCKETS))
        if terminal_indices != all_plan_indices:
            raise ManifestError(
                "terminal-mode provenance terminal buckets must cover every plan entry exactly once "
                f"(missing={make_plan_index_ranges(all_plan_indices - terminal_indices)}, "
                f"extra={make_plan_index_ranges(terminal_indices - all_plan_indices)})"
            )
    return bucket_sets


def parse_outcome_rows(
    provenance: Mapping[str, Any],
    by_index: Mapping[int, Mapping[str, Any]],
    bucket_sets: Mapping[str, set[int]],
    label: str,
) -> dict[int, Mapping[str, Any]]:
    outcome_summary = object_at(provenance.get("outcome_summary"), f"{label}.outcome_summary")
    rows = list_at(outcome_summary.get("rows"), f"{label}.outcome_summary.rows")
    expected_row_count = as_int(
        outcome_summary.get("terminal_outcome_row_count"), f"{label}.outcome_summary.terminal_outcome_row_count"
    )
    if expected_row_count != len(rows):
        raise ManifestError(
            f"{label}.outcome_summary.terminal_outcome_row_count must match rows length "
            f"({expected_row_count} != {len(rows)})"
        )

    by_plan_index: dict[int, Mapping[str, Any]] = {}
    observed_counts: collections.Counter[str] = collections.Counter()
    for offset, raw_row in enumerate(rows):
        row_label = f"{label}.outcome_summary.rows[{offset}]"
        row = object_at(raw_row, row_label)
        plan_index = as_int(row.get("discovery_plan_index"), f"{row_label}.discovery_plan_index")
        if plan_index not in by_index:
            raise ManifestError(f"{row_label}.discovery_plan_index references unknown plan index {plan_index}")
        if plan_index in by_plan_index:
            raise ManifestError(f"duplicate terminal outcome row for discovery_plan_index {plan_index}")
        plan_entry = by_index[plan_index]
        artifact_id = nonempty_str(row.get("discovery_artifact_id"), f"{row_label}.discovery_artifact_id")
        if artifact_id != plan_entry.get("artifact_id"):
            raise ManifestError(
                f"{row_label}.discovery_artifact_id must match plan artifact_id {plan_entry.get('artifact_id')!r}, "
                f"got {artifact_id!r}"
            )
        outcome_timeout = as_int(row.get("timeout_sec"), f"{row_label}.timeout_sec")
        if outcome_timeout != plan_entry.get("timeout_sec"):
            raise ManifestError(
                f"{row_label}.timeout_sec must match plan timeout_sec {plan_entry.get('timeout_sec')!r}, got {outcome_timeout}"
            )
        terminal_outcome = nonempty_str(row.get("terminal_outcome"), f"{row_label}.terminal_outcome")
        observed_counts[terminal_outcome] += 1
        expected_bucket = OUTCOME_TO_PRIMARY_BUCKET.get(terminal_outcome)
        if expected_bucket is None:
            raise ManifestError(f"{row_label}.terminal_outcome has unsupported value {terminal_outcome!r}")
        if plan_index not in bucket_sets[expected_bucket]:
            raise ManifestError(
                f"{row_label}.terminal_outcome {terminal_outcome!r} conflicts with requested bucket "
                f"{expected_bucket} for plan_index {plan_index}"
            )
        by_plan_index[plan_index] = row

    declared_counts = object_at(
        outcome_summary.get("terminal_outcome_counts", {}), f"{label}.outcome_summary.terminal_outcome_counts"
    )
    for outcome, count in observed_counts.items():
        declared = as_int(declared_counts.get(outcome, 0), f"{label}.outcome_summary.terminal_outcome_counts.{outcome}")
        if declared != count:
            raise ManifestError(
                f"{label}.outcome_summary.terminal_outcome_counts.{outcome} must be {count}, got {declared}"
            )
    declared_extra = sorted(str(key) for key in declared_counts if str(key) not in observed_counts and as_int(declared_counts[key], f"{label}.outcome_summary.terminal_outcome_counts.{key}") != 0)
    if declared_extra:
        raise ManifestError(f"{label}.outcome_summary.terminal_outcome_counts has undeclared row values {declared_extra}")

    row_plan_indices = set(by_plan_index)
    terminal_plan_indices = set().union(*(bucket_sets[bucket] for bucket in TERMINAL_BUCKETS))
    missing_terminal_rows = terminal_plan_indices - row_plan_indices
    if missing_terminal_rows:
        raise ManifestError(
            "terminal outcome rows must cover every terminal requested bucket "
            f"(missing={make_plan_index_ranges(missing_terminal_rows)})"
        )
    unexpected_terminal_rows = row_plan_indices - terminal_plan_indices
    if unexpected_terminal_rows:
        raise ManifestError(
            "terminal outcome rows must not reference non-terminal or missing-job plan entries "
            f"(extra={make_plan_index_ranges(unexpected_terminal_rows)})"
        )
    actual_missing_outcome_rows = set(by_index) - row_plan_indices
    declared_missing_outcome_rows = bucket_sets["missing_outcome_row"]
    if actual_missing_outcome_rows != declared_missing_outcome_rows:
        raise ManifestError(
            "missing_outcome_row ranges must equal plan entries without terminal outcome rows "
            f"(declared={make_plan_index_ranges(declared_missing_outcome_rows)}, "
            f"actual={make_plan_index_ranges(actual_missing_outcome_rows)})"
        )

    for plan_index in sorted(bucket_sets["completed_success"]):
        row = by_plan_index.get(plan_index)
        if row is None or row.get("terminal_outcome") != "success":
            raise ManifestError(f"completed_success plan_index {plan_index} is missing a success terminal outcome row")
    return by_plan_index


def parse_artifacts(
    provenance: Mapping[str, Any],
    by_index: Mapping[int, Mapping[str, Any]],
    bucket_sets: Mapping[str, set[int]],
    label: str,
) -> dict[int, Mapping[str, Any]]:
    artifact_summary = object_at(provenance.get("artifact_summary"), f"{label}.artifact_summary")
    entries = list_at(artifact_summary.get("entries"), f"{label}.artifact_summary.entries")
    by_plan_index: dict[int, Mapping[str, Any]] = {}
    for offset, raw_entry in enumerate(entries):
        entry_label = f"{label}.artifact_summary.entries[{offset}]"
        entry = object_at(raw_entry, entry_label)
        plan_index = as_int(entry.get("plan_index"), f"{entry_label}.plan_index")
        if plan_index not in by_index:
            raise ManifestError(f"{entry_label}.plan_index references unknown plan index {plan_index}")
        if plan_index in by_plan_index:
            raise ManifestError(f"duplicate artifact entry for plan_index {plan_index}")
        artifact_id = nonempty_str(entry.get("artifact_id"), f"{entry_label}.artifact_id")
        if artifact_id != by_index[plan_index].get("artifact_id"):
            raise ManifestError(
                f"{entry_label}.artifact_id must match plan artifact_id {by_index[plan_index].get('artifact_id')!r}, "
                f"got {artifact_id!r}"
            )
        by_plan_index[plan_index] = entry

    for plan_index in sorted(bucket_sets["completed_success"]):
        if plan_index not in by_plan_index:
            raise ManifestError(f"completed_success plan_index {plan_index} is missing artifact provenance")
    return by_plan_index


def parse_terminal_jobs(
    provenance: Mapping[str, Any],
    by_index: Mapping[int, Mapping[str, Any]],
    bucket_sets: Mapping[str, set[int]],
    label: str,
) -> dict[int, Mapping[str, Any]]:
    summary = provenance.get("terminal_discovery_job_summary")
    if summary is None:
        raise ManifestError(
            f"{label}.terminal_discovery_job_summary is required so terminal rows carry actual job provenance"
        )

    summary_obj = object_at(summary, f"{label}.terminal_discovery_job_summary")
    entries = list_at(summary_obj.get("entries"), f"{label}.terminal_discovery_job_summary.entries")
    by_plan_index: dict[int, Mapping[str, Any]] = {}
    for offset, raw_entry in enumerate(entries):
        entry_label = f"{label}.terminal_discovery_job_summary.entries[{offset}]"
        entry = object_at(raw_entry, entry_label)
        plan_index = as_int(entry.get("plan_index"), f"{entry_label}.plan_index")
        if plan_index not in by_index:
            raise ManifestError(f"{entry_label}.plan_index references unknown plan index {plan_index}")
        if plan_index in by_plan_index:
            raise ManifestError(f"duplicate terminal discovery job entry for plan_index {plan_index}")
        status = nonempty_str(entry.get("status"), f"{entry_label}.status")
        if status != "completed":
            raise ManifestError(f"{entry_label}.status must be 'completed', got {status!r}")
        name = nonempty_str(entry.get("name"), f"{entry_label}.name")
        expected_name = expected_discovery_job_name(plan_index, len(by_index))
        if name != expected_name:
            raise ManifestError(f"{entry_label}.name must be {expected_name!r}, got {name!r}")
        match = _JOB_NAME_RE.match(name)
        if match is None:
            raise ManifestError(f"{entry_label}.name does not match problem-size discovery job naming")
        shard = as_int(entry.get("shard"), f"{entry_label}.shard")
        if shard != discovery_shard_for_plan_index(plan_index, len(by_index)):
            raise ManifestError(f"{entry_label}.shard must match plan index shard, got {shard}")
        conclusion = str_or_none(entry.get("conclusion")) or ""
        if plan_index in bucket_sets["completed_success"] and conclusion != "success":
            raise ManifestError(f"{entry_label}.conclusion must be 'success' for completed_success plan_index {plan_index}")
        duration_sec = optional_int(entry.get("duration_sec"), f"{entry_label}.duration_sec")
        if duration_sec is not None and duration_sec < 0:
            raise ManifestError(f"{entry_label}.duration_sec must be non-negative, got {duration_sec}")
        by_plan_index[plan_index] = entry

    completed_indices = set().union(*(bucket_sets[bucket] for bucket in TERMINAL_BUCKETS))
    if not set(by_plan_index).issubset(completed_indices):
        raise ManifestError(
            "terminal_discovery_job_summary entries must be a subset of terminal requested buckets "
            f"(extra={make_plan_index_ranges(set(by_plan_index) - completed_indices)})"
        )
    missing_terminal_jobs = completed_indices - set(by_plan_index)
    if missing_terminal_jobs:
        raise ManifestError(
            "terminal requested plan entries are missing terminal job provenance "
            f"(missing={make_plan_index_ranges(missing_terminal_jobs)})"
        )
    completed_count = as_int(
        summary_obj.get("completed_job_count"), f"{label}.terminal_discovery_job_summary.completed_job_count"
    )
    if completed_count != len(by_plan_index):
        raise ManifestError(
            f"{label}.terminal_discovery_job_summary.completed_job_count must match entries length "
            f"({completed_count} != {len(by_plan_index)})"
        )
    return by_plan_index


def validate_terminal_outcome_accounting(
    provenance: Mapping[str, Any],
    bucket_sets: Mapping[str, set[int]],
    plan_count: int,
    label: str,
) -> None:
    """Validate terminal accounting records when present.

    Historical bounded snapshots do not carry this section.  Any terminal
    or non-bounded finishable evidence claim must carry it, and when present it
    must contain exactly one terminal record per 1416-entry plan entry that
    agrees with the requested terminal bucket ranges used for manifest
    regeneration.
    """

    accounting = provenance.get("terminal_outcome_accounting")
    if accounting is None:
        return
    accounting_obj = object_at(accounting, f"{label}.terminal_outcome_accounting")
    if accounting_obj.get("schema_name") != TERMINAL_ACCOUNTING_SCHEMA:
        raise ManifestError(
            f"{label}.terminal_outcome_accounting.schema_name must be {TERMINAL_ACCOUNTING_SCHEMA!r}, "
            f"got {accounting_obj.get('schema_name')!r}"
        )
    if as_int(accounting_obj.get("schema_version"), f"{label}.terminal_outcome_accounting.schema_version") != SCHEMA_VERSION:
        raise ManifestError(f"{label}.terminal_outcome_accounting.schema_version must be {SCHEMA_VERSION}")
    if accounting_obj.get("terminal_mode") is not True:
        raise ManifestError(f"{label}.terminal_outcome_accounting.terminal_mode must be true")
    declared_plan_count = as_int(
        accounting_obj.get("plan_entry_count"), f"{label}.terminal_outcome_accounting.plan_entry_count"
    )
    if declared_plan_count != plan_count:
        raise ManifestError(f"{label}.terminal_outcome_accounting.plan_entry_count must be {plan_count}")
    records = list_at(accounting_obj.get("records"), f"{label}.terminal_outcome_accounting.records")
    record_count = as_int(accounting_obj.get("record_count"), f"{label}.terminal_outcome_accounting.record_count")
    if record_count != len(records) or record_count != plan_count:
        raise ManifestError(
            f"{label}.terminal_outcome_accounting.record_count must be one record per 1416-entry plan entry "
            f"({record_count} != {len(records)} != {plan_count})"
        )

    expected_indices = set(range(1, plan_count + 1))
    seen: set[int] = set()
    duplicates: set[int] = set()
    observed_by_bucket = {bucket: set() for bucket in TERMINAL_BUCKETS}
    for offset, raw_record in enumerate(records):
        record_label = f"{label}.terminal_outcome_accounting.records[{offset}]"
        record = object_at(raw_record, record_label)
        plan_index = as_int(record.get("plan_index"), f"{record_label}.plan_index")
        if plan_index in seen:
            duplicates.add(plan_index)
        seen.add(plan_index)
        if record.get("terminal") is not True:
            raise ManifestError(f"{record_label}.terminal must be true")
        bucket = nonempty_str(record.get("terminal_outcome_bucket"), f"{record_label}.terminal_outcome_bucket")
        if bucket not in observed_by_bucket:
            raise ManifestError(f"{record_label}.terminal_outcome_bucket has unsupported value {bucket!r}")
        observed_by_bucket[bucket].add(plan_index)

    missing = expected_indices - seen
    extra = seen - expected_indices
    if duplicates or missing or extra:
        raise ManifestError(
            f"{label}.terminal_outcome_accounting.records must cover the plan exactly once "
            f"(duplicates={make_plan_index_ranges(duplicates)}, missing={make_plan_index_ranges(missing)}, "
            f"extra={make_plan_index_ranges(extra)})"
        )
    for bucket in TERMINAL_BUCKETS:
        if observed_by_bucket[bucket] != bucket_sets[bucket]:
            raise ManifestError(
                f"{label}.terminal_outcome_accounting.records for {bucket} must match requested bucket ranges "
                f"(expected={make_plan_index_ranges(bucket_sets[bucket])}, "
                f"actual={make_plan_index_ranges(observed_by_bucket[bucket])})"
            )
    terminal_total = as_int(
        accounting_obj.get("terminal_bucket_total"), f"{label}.terminal_outcome_accounting.terminal_bucket_total"
    )
    if terminal_total != plan_count:
        raise ManifestError(f"{label}.terminal_outcome_accounting.terminal_bucket_total must be {plan_count}")
    for key in ("non_terminal_count", "missing_job_count"):
        if as_int(accounting_obj.get(key), f"{label}.terminal_outcome_accounting.{key}") != 0:
            raise ManifestError(f"{label}.terminal_outcome_accounting.{key} must be 0")


def terminal_claim_requested(provenance: Mapping[str, Any], label: str) -> bool:
    requested = object_at(provenance.get("requested_plan_outcome_buckets"), f"{label}.requested_plan_outcome_buckets")
    return (
        requested.get("terminal_mode") is True
        or requested.get("terminal_provenance") is True
        or provenance.get("terminal_outcome_accounting") is not None
    )


def validate_expected_source_int(
    expected_identity: Mapping[str, Any],
    field: str,
    top_level_value: Any,
    top_level_label: str,
    label: str,
) -> None:
    expected_value = as_int(
        expected_identity.get(field),
        f"{label}.source_policy.expected_source_identity.{field}",
    )
    actual_value = as_int(top_level_value, f"{label}.{top_level_label}")
    if expected_value != actual_value:
        raise ManifestError(
            f"{label}.source_policy expected {field} conflicts with requested/top-level identity at "
            f"{top_level_label} ({expected_value} != {actual_value})"
        )


def validate_expected_source_text(
    expected_identity: Mapping[str, Any],
    field: str,
    top_level_value: Any,
    top_level_label: str,
    label: str,
) -> None:
    expected_value = str_or_none(expected_identity.get(field))
    actual_value = str_or_none(top_level_value)
    if expected_value != actual_value:
        raise ManifestError(
            f"{label}.source_policy expected {field} conflicts with requested/top-level identity at "
            f"{top_level_label} ({expected_value!r} != {actual_value!r})"
        )


def validate_optional_source_int(
    expected_identity: Mapping[str, Any],
    field: str,
    top_level_value: Any,
    top_level_label: str,
    label: str,
) -> None:
    expected_value = optional_int(
        expected_identity.get(field),
        f"{label}.source_policy.expected_source_identity.{field}",
    )
    actual_value = optional_int(top_level_value, f"{label}.{top_level_label}")
    if expected_value != actual_value:
        raise ManifestError(
            f"{label}.source_policy expected {field} conflicts with requested/top-level identity at "
            f"{top_level_label} ({expected_value!r} != {actual_value!r})"
        )


def validate_expected_source_required_text(
    expected_identity: Mapping[str, Any],
    field: str,
    top_level_value: Any,
    top_level_label: str,
    label: str,
) -> None:
    expected_value = nonempty_str(
        expected_identity.get(field),
        f"{label}.source_policy.expected_source_identity.{field}",
    )
    actual_value = nonempty_str(top_level_value, f"{label}.{top_level_label}")
    if expected_value != actual_value:
        raise ManifestError(
            f"{label}.source_policy expected {field} conflicts with requested/top-level identity at "
            f"{top_level_label} ({expected_value!r} != {actual_value!r})"
        )


def validate_expected_source_identity_matches_provenance(
    provenance: Mapping[str, Any],
    policy: Mapping[str, Any],
    label: str,
) -> None:
    expected_identity = object_at(
        policy.get("expected_source_identity"),
        f"{label}.source_policy.expected_source_identity",
    )
    run = object_at(provenance.get("run"), f"{label}.run")
    dispatch = object_at(provenance.get("dispatch"), f"{label}.dispatch")
    validate_expected_source_int(
        expected_identity,
        "run_id",
        run.get("database_id"),
        "run.database_id",
        label,
    )
    for top_level_label, top_level_value in (
        ("branch", provenance.get("branch")),
        ("run.head_branch", run.get("head_branch")),
        ("dispatch.ref", dispatch.get("ref")),
    ):
        validate_expected_source_text(
            expected_identity,
            "branch",
            top_level_value,
            top_level_label,
            label,
        )
    validate_expected_source_required_text(
        expected_identity,
        "head_sha",
        run.get("head_sha"),
        "run.head_sha",
        label,
    )
    validate_expected_source_text(
        expected_identity,
        "workflow",
        dispatch.get("workflow"),
        "dispatch.workflow",
        label,
    )
    validate_expected_source_text(
        expected_identity,
        "milestone",
        provenance.get("milestone"),
        "milestone",
        label,
    )
    validate_expected_source_text(
        expected_identity,
        "collection_run_id",
        provenance.get("collection_run_id"),
        "collection_run_id",
        label,
    )
    validate_optional_source_int(
        expected_identity,
        "issue",
        provenance.get("issue"),
        "issue",
        label,
    )


def validate_single_run_source_policy(provenance: Mapping[str, Any], plan_count: int, label: str) -> None:
    policy = object_at(provenance.get("source_policy"), f"{label}.source_policy")
    if policy.get("schema_name") != SOURCE_POLICY_SCHEMA:
        raise ManifestError(f"{label}.source_policy.schema_name must be {SOURCE_POLICY_SCHEMA!r}")
    if policy.get("policy") != SOURCE_POLICY_NAME:
        raise ManifestError(f"{label}.source_policy.policy must be {SOURCE_POLICY_NAME!r}")
    if policy.get("single_run_identity_required") is not True:
        raise ManifestError(f"{label}.source_policy.single_run_identity_required must be true")
    if policy.get("reviewed_multi_source_schema") is not False:
        raise ManifestError(f"{label}.source_policy.reviewed_multi_source_schema must be false")
    if policy.get("multi_source_union_allowed") is not False:
        raise ManifestError(f"{label}.source_policy.multi_source_union_allowed must be false")
    if as_int(policy.get("raw_source_identity_count"), f"{label}.source_policy.raw_source_identity_count") != 1:
        raise ManifestError(f"{label}.source_policy.raw_source_identity_count must be 1")
    logical_count = as_int(policy.get("logical_source_record_count"), f"{label}.source_policy.logical_source_record_count")
    if logical_count != plan_count:
        raise ManifestError(f"{label}.source_policy.logical_source_record_count must be {plan_count}")
    validate_expected_source_identity_matches_provenance(provenance, policy, label)
    run = object_at(provenance.get("run"), f"{label}.run")
    if run.get("conclusion") != "success":
        raise ManifestError(
            f"{label}.run.conclusion must be 'success' before finishable/Tier regeneration from terminal provenance"
        )


def non_empty_blocking_bucket_ranges(bucket_sets: Mapping[str, set[int]]) -> dict[str, list[str]]:
    return {
        bucket: make_plan_index_ranges(bucket_sets[bucket])
        for bucket in NON_BOUNDED_BLOCKING_BUCKETS
        if bucket_sets[bucket]
    }


def enforce_finishable_claim_scope(
    provenance: Mapping[str, Any],
    bucket_sets: Mapping[str, set[int]],
    plan_count: int,
    label: str,
) -> None:
    """Reject relabeled bounded snapshots before emitting non-bounded claims.

    Bounded snapshots are allowed to contain non-terminal, queued,
    in-progress, missing-job, and missing-outcome buckets only while the source
    run is explicitly labeled as a bounded non-terminal snapshot.  Any terminal
    or otherwise non-bounded finishable-size claim must instead carry complete
    terminal outcome accounting for all 1416 plan entries.
    """

    run = object_at(provenance.get("run"), f"{label}.run")
    non_terminal_snapshot = bool(run.get("non_terminal_snapshot"))
    terminal_claim = terminal_claim_requested(provenance, label)
    non_bounded_claim = not non_terminal_snapshot
    if not (terminal_claim or non_bounded_claim):
        return

    failures: list[str] = []
    if provenance.get("terminal_outcome_accounting") is None:
        failures.append(f"{label}.terminal_outcome_accounting is required")

    non_empty_buckets = non_empty_blocking_bucket_ranges(bucket_sets)
    if non_empty_buckets:
        failures.append(
            "non-terminal/queued/in-progress/missing buckets are allowed only for explicit "
            f"bounded non-terminal snapshots (non_empty={non_empty_buckets})"
        )

    if failures:
        raise ManifestError(
            "terminal/non-bounded finishable evidence claims require complete terminal_outcome_accounting; "
            + "; ".join(failures)
        )

    if terminal_claim:
        validate_single_run_source_policy(provenance, plan_count, label)
    validate_terminal_outcome_accounting(provenance, bucket_sets, plan_count, label)


# ---------------------------------------------------------------------------
# Manifest construction
# ---------------------------------------------------------------------------


def primary_bucket_for(plan_index: int, bucket_sets: Mapping[str, set[int]]) -> str:
    for bucket in PRIMARY_BUCKETS:
        if plan_index in bucket_sets[bucket]:
            return bucket
    raise AssertionError(f"plan_index {plan_index} missing primary bucket after validation")


def observed_status_for(plan_index: int, primary_bucket: str, bucket_sets: Mapping[str, set[int]], job: Optional[Mapping[str, Any]]) -> str:
    status = str_or_none(job.get("status") if job else None)
    if status:
        return status
    if primary_bucket in TERMINAL_BUCKETS:
        return "completed"
    if primary_bucket == "missing_job":
        return "missing"
    if plan_index in bucket_sets["non_terminal_in_progress"]:
        return "in_progress"
    if plan_index in bucket_sets["non_terminal_queued"]:
        return "queued"
    if primary_bucket == "non_terminal":
        return "non_terminal"
    return primary_bucket


def observed_conclusion_for(primary_bucket: str, job: Optional[Mapping[str, Any]]) -> str:
    conclusion = str_or_none(job.get("conclusion") if job else None)
    if conclusion is not None:
        return conclusion
    return {
        "completed_success": "success",
        "failed": "failure",
        "timed_out": "timed_out",
        "skipped": "skipped",
        "cancelled": "cancelled",
        "non_terminal": "",
        "missing_job": "missing",
    }[primary_bucket]


def observed_outcome_for(primary_bucket: str, outcome_row: Optional[Mapping[str, Any]]) -> str:
    terminal = str_or_none(outcome_row.get("terminal_outcome") if outcome_row else None)
    if terminal is not None:
        return terminal
    return {
        "completed_success": "success",
        "failed": "failed",
        "timed_out": "timeout",
        "skipped": "skipped",
        "cancelled": "cancelled",
        "non_terminal": "non_terminal",
        "missing_job": "missing_job",
    }[primary_bucket]


def exclusion_reason_for(
    *,
    plan_index: int,
    primary_bucket: str,
    observed_status: str,
    observed_conclusion: str,
    observed_outcome: str,
    timeout_sec: int,
    duration_sec: Optional[int],
    bucket_sets: Mapping[str, set[int]],
    has_outcome_row: bool,
    has_artifact: bool,
) -> Optional[str]:
    if plan_index in bucket_sets["missing_artifact_for_completed_success_job"] or (
        primary_bucket == "completed_success" and not has_artifact
    ):
        return "missing_artifact_for_completed_success_job"
    if plan_index in bucket_sets["missing_outcome_row_for_completed_success_job"] or (
        primary_bucket == "completed_success" and not has_outcome_row
    ):
        return "missing_outcome_row_for_completed_success_job"
    if primary_bucket == "completed_success":
        if observed_status != "completed":
            return f"completed_success_job_status_{observed_status}"
        if observed_conclusion != "success":
            return f"completed_success_job_conclusion_{observed_conclusion or 'empty'}"
        if observed_outcome != "success":
            return f"completed_success_terminal_outcome_{observed_outcome}"
        if timeout_sec > EXPECTED_TIMEOUT_SEC:
            return f"timeout_budget_over_{EXPECTED_TIMEOUT_SEC}s"
        if duration_sec is not None and duration_sec > EXPECTED_TIMEOUT_SEC:
            return f"duration_over_{EXPECTED_TIMEOUT_SEC}s"
        return None
    if primary_bucket == "non_terminal":
        if observed_status in ("queued", "in_progress"):
            return f"non_terminal_{observed_status}"
        return "non_terminal"
    return {
        "failed": "completed_failure",
        "timed_out": "timed_out",
        "skipped": "skipped",
        "cancelled": "cancelled",
        "missing_job": "missing_job",
    }[primary_bucket]


def manifest_entry(
    plan_entry: Mapping[str, Any],
    plan_count: int,
    run: Mapping[str, Any],
    bucket_sets: Mapping[str, set[int]],
    outcome_row: Optional[Mapping[str, Any]],
    artifact: Optional[Mapping[str, Any]],
    job: Optional[Mapping[str, Any]],
) -> dict[str, Any]:
    plan_index = as_int(plan_entry.get("plan_index"), "plan_entry.plan_index")
    timeout_sec = as_int(plan_entry.get("timeout_sec"), f"plan[{plan_index}].timeout_sec")
    primary_bucket = primary_bucket_for(plan_index, bucket_sets)
    observed_status = observed_status_for(plan_index, primary_bucket, bucket_sets, job)
    observed_conclusion = observed_conclusion_for(primary_bucket, job)
    observed_outcome = observed_outcome_for(primary_bucket, outcome_row)
    duration_sec = optional_int(job.get("duration_sec"), f"job[{plan_index}].duration_sec") if job else None
    has_artifact = artifact is not None
    has_outcome_row = outcome_row is not None
    exclusion_reason = exclusion_reason_for(
        plan_index=plan_index,
        primary_bucket=primary_bucket,
        observed_status=observed_status,
        observed_conclusion=observed_conclusion,
        observed_outcome=observed_outcome,
        timeout_sec=timeout_sec,
        duration_sec=duration_sec,
        bucket_sets=bucket_sets,
        has_outcome_row=has_outcome_row,
        has_artifact=has_artifact,
    )
    eligible = exclusion_reason is None
    shard = optional_int(job.get("shard"), f"job[{plan_index}].shard") if job else discovery_shard_for_plan_index(plan_index, plan_count)

    return {
        "plan_index": plan_index,
        "benchmark": str(plan_entry.get("benchmark")),
        "kernel": str(plan_entry.get("hardware_kernel_name")),
        "problem_size": str(plan_entry.get("problem_size")),
        "hardware_kernel_name": str(plan_entry.get("hardware_kernel_name")),
        "hardware_problem_size": str(plan_entry.get("hardware_problem_size")),
        "hardware_real_ms": str_or_none(plan_entry.get("hardware_real_ms")),
        "tier": as_int(plan_entry.get("tier"), f"plan[{plan_index}].tier"),
        "tier_name": str(plan_entry.get("tier_name")),
        "workflow_job": str(plan_entry.get("workflow_job")),
        "workflow_benchmark": str(plan_entry.get("workflow_benchmark")),
        "workflow_benchmark_name": str(plan_entry.get("workflow_benchmark_name")),
        "workflow_matrix_index": optional_int(plan_entry.get("workflow_matrix_index"), f"plan[{plan_index}].workflow_matrix_index"),
        "sizes": str(plan_entry.get("sizes")),
        "size_token": str(plan_entry.get("size_token")),
        "size_label": str(plan_entry.get("size_label")),
        "flag_template": str(plan_entry.get("flag_template")),
        "size_label_template": str(plan_entry.get("size_label_template")),
        "extra_flags": str_or_none(plan_entry.get("extra_flags")),
        "gomemlimit": str_or_none(plan_entry.get("gomemlimit")),
        "gogc": str_or_none(plan_entry.get("gogc")),
        "timeout_sec": timeout_sec,
        "run_id": as_int(run.get("database_id"), "run.database_id"),
        "run_url": str_or_none(run.get("url")),
        "run_status": str_or_none(run.get("status")),
        "run_conclusion": str_or_none(run.get("conclusion")) or "",
        "run_head_branch": str_or_none(run.get("head_branch")),
        "run_head_sha": str_or_none(run.get("head_sha")),
        "discovery_shard": shard,
        "discovery_job_name": str_or_none(job.get("name")) if job else expected_discovery_job_name(plan_index, plan_count),
        "discovery_job_database_id": optional_int(job.get("database_id"), f"job[{plan_index}].database_id") if job else None,
        "discovery_job_url": str_or_none(job.get("url")) if job else None,
        "discovery_job_started_at": str_or_none(job.get("started_at")) if job else None,
        "discovery_job_completed_at": str_or_none(job.get("completed_at")) if job else None,
        "duration_sec": duration_sec,
        "observed_job_status": observed_status,
        "observed_job_conclusion": observed_conclusion,
        "observed_outcome_bucket": primary_bucket,
        "observed_outcome": observed_outcome,
        "terminal_outcome": str_or_none(outcome_row.get("terminal_outcome")) if outcome_row else None,
        "sim_result_state": str_or_none(outcome_row.get("sim_result_state")) if outcome_row else None,
        "exit_code": str_or_none(outcome_row.get("exit_code")) if outcome_row else None,
        "outcome_detail": str_or_none(outcome_row.get("detail")) if outcome_row else None,
        "outcome_timeout_sec": optional_int(outcome_row.get("timeout_sec"), f"outcome[{plan_index}].timeout_sec") if outcome_row else None,
        "expected_artifact_id": str(plan_entry.get("artifact_id")),
        "artifact_database_id": optional_int(artifact.get("id"), f"artifact[{plan_index}].id") if artifact else None,
        "artifact_name": str_or_none(artifact.get("name")) if artifact else None,
        "artifact_digest": str_or_none(artifact.get("digest")) if artifact else None,
        "artifact_created_at": str_or_none(artifact.get("created_at")) if artifact else None,
        "artifact_expired": bool(artifact.get("expired")) if artifact else None,
        "eligible_for_linear_workflow": eligible,
        "eligibility_status": "eligible" if eligible else "excluded",
        "exclusion_reason": exclusion_reason,
    }


def summarize_entries(entries: Sequence[Mapping[str, Any]], bucket_sets: Mapping[str, set[int]]) -> dict[str, Any]:
    eligible_indices = [as_int(entry["plan_index"], "entry.plan_index") for entry in entries if entry.get("eligible_for_linear_workflow")]
    excluded_indices = [as_int(entry["plan_index"], "entry.plan_index") for entry in entries if not entry.get("eligible_for_linear_workflow")]
    tier_summary: dict[str, dict[str, Any]] = {}
    for tier in (1, 2):
        tier_entries = [entry for entry in entries if entry.get("tier") == tier]
        tier_eligible = [entry for entry in tier_entries if entry.get("eligible_for_linear_workflow")]
        tier_excluded = [entry for entry in tier_entries if not entry.get("eligible_for_linear_workflow")]
        tier_summary[str(tier)] = {
            "total": len(tier_entries),
            "eligible": len(tier_eligible),
            "excluded": len(tier_excluded),
            "eligible_plan_index_ranges": make_plan_index_ranges(as_int(entry["plan_index"], "entry.plan_index") for entry in tier_eligible),
            "excluded_plan_index_ranges": make_plan_index_ranges(as_int(entry["plan_index"], "entry.plan_index") for entry in tier_excluded),
        }

    return {
        "entry_count": len(entries),
        "eligible_entry_count": len(eligible_indices),
        "excluded_entry_count": len(excluded_indices),
        "eligible_plan_index_ranges": make_plan_index_ranges(eligible_indices),
        "excluded_plan_index_ranges": make_plan_index_ranges(excluded_indices),
        "tier_counts": tier_summary,
        "observed_outcome_bucket_counts": stringify_counts(collections.Counter(str(entry.get("observed_outcome_bucket")) for entry in entries)),
        "observed_outcome_counts": stringify_counts(collections.Counter(str(entry.get("observed_outcome")) for entry in entries)),
        "observed_job_status_counts": stringify_counts(collections.Counter(str(entry.get("observed_job_status")) for entry in entries)),
        "observed_job_conclusion_counts": stringify_counts(collections.Counter(str(entry.get("observed_job_conclusion")) for entry in entries)),
        "eligibility_status_counts": stringify_counts(collections.Counter(str(entry.get("eligibility_status")) for entry in entries)),
        "exclusion_reason_counts": stringify_counts(
            collections.Counter(str(entry.get("exclusion_reason")) for entry in entries if entry.get("exclusion_reason") is not None)
        ),
        "requested_bucket_count_reconciliation": bucket_count_reconciliation(bucket_sets),
        "requested_primary_bucket_plan_index_ranges": {
            bucket: make_plan_index_ranges(bucket_sets[bucket]) for bucket in PRIMARY_BUCKETS
        },
        "requested_diagnostic_bucket_plan_index_ranges": {
            bucket: make_plan_index_ranges(bucket_sets[bucket]) for bucket in DIAGNOSTIC_BUCKETS
        },
    }


def build_tier_matrix(
    entries: Sequence[Mapping[str, Any]],
    tier: int,
    *,
    source_manifest_path: Path,
    source_provenance_path: Path,
    run: Mapping[str, Any],
    snapshot_contract: Mapping[str, Any],
) -> dict[str, Any]:
    tier_entries = [entry for entry in entries if entry.get("tier") == tier]
    eligible_entries = [entry for entry in tier_entries if entry.get("eligible_for_linear_workflow")]
    excluded_entries = [entry for entry in tier_entries if not entry.get("eligible_for_linear_workflow")]
    groups: collections.OrderedDict[tuple[Any, ...], list[Mapping[str, Any]]] = collections.OrderedDict()
    for entry in eligible_entries:
        key = (
            entry.get("workflow_benchmark_name"),
            entry.get("tier_name"),
            entry.get("flag_template"),
            entry.get("size_label_template"),
            entry.get("extra_flags"),
            entry.get("gomemlimit"),
            entry.get("gogc"),
            entry.get("timeout_sec"),
        )
        groups.setdefault(key, []).append(entry)

    include: list[dict[str, Any]] = []
    for (
        benchmark,
        tier_name,
        flag_template,
        size_label_template,
        extra_flags,
        gomemlimit,
        gogc,
        timeout_sec,
    ), rows in groups.items():
        rows = sorted(rows, key=lambda row: as_int(row["plan_index"], "entry.plan_index"))
        matrix_row: dict[str, Any] = {
            "benchmark": benchmark,
            "tier": tier_name,
            "sizes": " ".join(str(row.get("sizes")) for row in rows),
            "flag_template": flag_template,
            "size_label_template": size_label_template,
            "timeout_sec": timeout_sec,
            "finishable_plan_index_ranges": make_plan_index_ranges(as_int(row["plan_index"], "entry.plan_index") for row in rows),
        }
        if extra_flags is not None:
            matrix_row["extra_flags"] = extra_flags
        if gomemlimit is not None:
            matrix_row["gomemlimit"] = gomemlimit
        if gogc is not None:
            matrix_row["gogc"] = gogc
        include.append(matrix_row)
    return {
        "schema_name": TIER_VIEW_SCHEMA,
        "schema_version": SCHEMA_VERSION,
        "source_manifest": repo_relative(source_manifest_path),
        "source_provenance": repo_relative(source_provenance_path),
        "run": {
            "database_id": run.get("database_id"),
            "status": run.get("status"),
            "conclusion": run.get("conclusion") or "",
            "head_branch": run.get("head_branch"),
            "head_sha": run.get("head_sha"),
            "non_terminal_snapshot": bool(run.get("non_terminal_snapshot")),
        },
        "snapshot_contract": dict(snapshot_contract),
        "tier": tier,
        "tier_name": f"tier{tier}",
        "eligible_only": True,
        "eligible_entry_count": len(eligible_entries),
        "excluded_entry_count": len(excluded_entries),
        "eligible_plan_index_ranges": make_plan_index_ranges(
            as_int(entry["plan_index"], "entry.plan_index") for entry in eligible_entries
        ),
        "excluded_plan_index_ranges": make_plan_index_ranges(
            as_int(entry["plan_index"], "entry.plan_index") for entry in excluded_entries
        ),
        "include": include,
    }


def build_manifest(
    plan_path: Path,
    provenance_path: Path,
    manifest_path: Path,
    tier1_path: Path,
    tier2_path: Path,
    summary_path: Path,
) -> tuple[dict[str, Any], dict[str, Any], dict[str, Any], dict[str, Any]]:
    plan, plan_entries, by_index = load_and_validate_plan(plan_path)
    provenance = load_and_validate_provenance(provenance_path, plan_path, len(plan_entries))
    label = repo_relative(provenance_path)
    bucket_sets = parse_bucket_sets(provenance, len(plan_entries), label)
    enforce_finishable_claim_scope(provenance, bucket_sets, len(plan_entries), label)
    outcome_rows = parse_outcome_rows(provenance, by_index, bucket_sets, label)
    artifacts = parse_artifacts(provenance, by_index, bucket_sets, label)
    terminal_jobs = parse_terminal_jobs(provenance, by_index, bucket_sets, label)
    if set(outcome_rows) - set(artifacts):
        raise ManifestError(
            "artifact entries must cover every terminal outcome row "
            f"(missing_artifacts={make_plan_index_ranges(set(outcome_rows) - set(artifacts))})"
        )
    run = object_at(provenance.get("run"), f"{label}.run")

    observation = object_at(provenance.get("observation", {}), f"{label}.observation")
    observed_at_utc = str_or_none(observation.get("observed_at_utc")) or str_or_none(run.get("observed_at_utc"))
    non_terminal_snapshot = bool(run.get("non_terminal_snapshot"))
    terminal_provenance = terminal_claim_requested(provenance, label)
    if non_terminal_snapshot and observed_at_utc is None:
        raise ManifestError(f"{label}: non-terminal snapshots must record observation.observed_at_utc")
    if non_terminal_snapshot:
        snapshot_contract = {
            "observation_time_utc": observed_at_utc,
            "non_terminal_snapshot": non_terminal_snapshot,
            "bounded_snapshot": True,
            "claim_scope": (
                "immutable observation-time evidence snapshot only; queued/in_progress/missing-outcome rows are "
                "not final/current finishable-size claims while the source workflow remains non-terminal"
            ),
            "source": str_or_none(observation.get("source")),
        }
    else:
        snapshot_contract = {
            "observation_time_utc": observed_at_utc,
            "non_terminal_snapshot": False,
            "bounded_snapshot": False,
            "terminal_provenance": terminal_provenance,
            "claim_scope": (
                "terminal discovery evidence regenerated from compact provenance; existing checked-in "
                "bounded snapshot artifacts remain historical unless explicitly regenerated from this terminal provenance"
            ),
            "source": str_or_none(observation.get("source")),
        }

    entries = [
        manifest_entry(
            plan_entry,
            len(plan_entries),
            run,
            bucket_sets,
            outcome_rows.get(as_int(plan_entry["plan_index"], "plan_entry.plan_index")),
            artifacts.get(as_int(plan_entry["plan_index"], "plan_entry.plan_index")),
            terminal_jobs.get(as_int(plan_entry["plan_index"], "plan_entry.plan_index")),
        )
        for plan_entry in plan_entries
    ]
    summary = summarize_entries(entries, bucket_sets)
    tier1_view = build_tier_matrix(
        entries,
        1,
        source_manifest_path=manifest_path,
        source_provenance_path=provenance_path,
        run=run,
        snapshot_contract=snapshot_contract,
    )
    tier2_view = build_tier_matrix(
        entries,
        2,
        source_manifest_path=manifest_path,
        source_provenance_path=provenance_path,
        run=run,
        snapshot_contract=snapshot_contract,
    )

    tier_view_summary = {
        "tier1": {
            "path": repo_relative(tier1_path),
            "eligible_entry_count": summary["tier_counts"]["1"]["eligible"],
            "matrix_entry_count": len(tier1_view["include"]),
        },
        "tier2": {
            "path": repo_relative(tier2_path),
            "eligible_entry_count": summary["tier_counts"]["2"]["eligible"],
            "matrix_entry_count": len(tier2_view["include"]),
        },
    }

    manifest = {
        "schema_name": MANIFEST_SCHEMA,
        "schema_version": SCHEMA_VERSION,
        "source_paths": {
            "problem_size_discovery_plan": repo_relative(plan_path),
            "discovery_provenance": repo_relative(provenance_path),
        },
        "source_plan": {
            "schema_name": plan.get("schema_name"),
            "schema_version": plan.get("schema_version"),
            "entry_count": plan.get("entry_count"),
            "tier_counts": plan.get("tier_counts"),
            "timeout_sec": plan.get("timeout_sec"),
        },
        "run": {
            "database_id": run.get("database_id"),
            "url": run.get("url"),
            "workflow_name": run.get("workflow_name"),
            "event": run.get("event"),
            "head_branch": run.get("head_branch"),
            "head_sha": run.get("head_sha"),
            "status": run.get("status"),
            "conclusion": run.get("conclusion") or "",
            "created_at": run.get("created_at"),
            "updated_at": run.get("updated_at"),
            "non_terminal_snapshot": bool(run.get("non_terminal_snapshot")),
            "observed_at_utc": observed_at_utc,
        },
        "observation": {
            "observed_at_utc": observed_at_utc,
            "source": snapshot_contract["source"],
            "non_terminal_snapshot": non_terminal_snapshot,
        },
        "snapshot_contract": snapshot_contract,
        "eligibility_policy": {
            "timeout_sec": EXPECTED_TIMEOUT_SEC,
            "eligible_when": "completed_success terminal_outcome=success with completed/success job evidence and no missing artifact/outcome diagnostics; duration_sec must be <= timeout_sec when available",
            "excluded_when": "non-terminal, missing, failed, timed-out, skipped, cancelled, or completed-success rows with missing artifact/outcome diagnostics",
            "hardware_timing_guesses_used": False,
        },
        "entry_count": len(entries),
        "summary": summary,
        "tier_views": tier_view_summary,
        "entries": entries,
    }
    compact_summary = {
        "schema_name": SUMMARY_SCHEMA,
        "schema_version": SCHEMA_VERSION,
        "source_manifest": repo_relative(manifest_path),
        "source_paths": manifest["source_paths"],
        "run": manifest["run"],
        "observation": manifest["observation"],
        "snapshot_contract": manifest["snapshot_contract"],
        "eligibility_policy": manifest["eligibility_policy"],
        "entry_count": len(entries),
        "summary": summary,
        "tier_views": tier_view_summary,
    }
    return manifest, tier1_view, tier2_view, compact_summary


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Generate deterministic MI300A finishable-size manifest and Tier 1/Tier 2 views."
    )
    parser.add_argument("--plan", default=str(DEFAULT_PLAN), help="Path to mi300a_problem_size_discovery_plan.json")
    parser.add_argument(
        "--provenance",
        default=str(DEFAULT_PROVENANCE),
        help="Path to compact discovery run outcome/provenance JSON",
    )
    parser.add_argument("--output", default=str(DEFAULT_MANIFEST), help="Manifest output JSON path")
    parser.add_argument("--tier1-output", default=str(DEFAULT_TIER1_VIEW), help="Tier 1 finishable matrix view path")
    parser.add_argument("--tier2-output", default=str(DEFAULT_TIER2_VIEW), help="Tier 2 finishable matrix view path")
    parser.add_argument("--summary-output", default=str(DEFAULT_SUMMARY), help="Compact summary output JSON path")
    return parser.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    args = parse_args(argv)
    plan_path = repo_path(args.plan)
    provenance_path = repo_path(args.provenance)
    manifest_path = repo_path(args.output)
    tier1_path = repo_path(args.tier1_output)
    tier2_path = repo_path(args.tier2_output)
    summary_path = repo_path(args.summary_output)

    try:
        manifest, tier1_view, tier2_view, summary = build_manifest(
            plan_path,
            provenance_path,
            manifest_path,
            tier1_path,
            tier2_path,
            summary_path,
        )
    except ManifestError as exc:
        print("FAIL: MI300A finishable-size manifest generation rejected input.", file=sys.stderr)
        print(f"  - {exc}", file=sys.stderr)
        return 1

    write_json(manifest_path, manifest)
    write_json(tier1_path, tier1_view)
    write_json(tier2_path, tier2_view)
    write_json(summary_path, summary)
    print(
        "WROTE MI300A finishable-size manifest: "
        f"entries={manifest['entry_count']} eligible={summary['summary']['eligible_entry_count']} "
        f"excluded={summary['summary']['excluded_entry_count']} "
        f"manifest={repo_relative(manifest_path)} "
        f"tier1={repo_relative(tier1_path)} "
        f"tier2={repo_relative(tier2_path)} "
        f"summary={repo_relative(summary_path)}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
