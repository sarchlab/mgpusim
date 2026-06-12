#!/usr/bin/env python3
"""Generate deterministic per-benchmark MI300A terminal discovery matrices.

The checked-in 1416-entry MI300A problem-size discovery plan remains the source
of truth.  This generator builds an offline/operator dispatch surface that keeps
GitHub Actions matrix rows grouped by workflow benchmark.  Each visible matrix
``include`` row represents one workflow benchmark and carries an ordered
``attempts`` list with the per-problem-size terminal discovery metadata.

The generated surface keeps every source plan entry exactly once, keeps each
benchmark group intact, bounds shards by the number of per-size attempts (256 by
default), preserves timeout_sec=3600 on every attempt, and asserts the historic
hotspot/problem-size 48 entry at plan_index 474.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from pathlib import Path
from typing import Any, Iterable, Mapping, Optional, Sequence


REPO_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_PLAN = REPO_ROOT / "benchmark-comparison" / "mi300a_problem_size_discovery_plan.json"
DEFAULT_GENERATED_DIR = REPO_ROOT / "benchmark-comparison" / "generated"
DEFAULT_MANIFEST = DEFAULT_GENERATED_DIR / "mi300a_terminal_discovery_shard_manifest.json"
DEFAULT_MAX_ATTEMPTS_PER_SHARD = 256

PLAN_SCHEMA = "mgpusim.mi300a_problem_size_discovery_plan"
MANIFEST_SCHEMA = "mgpusim.mi300a_terminal_discovery_benchmark_shard_manifest"
MATRIX_SCHEMA = "mgpusim.mi300a_terminal_discovery_benchmark_shard_matrix"
SCHEMA_VERSION = 1
EXPECTED_ENTRY_COUNT = 1416
EXPECTED_TIMEOUT_SEC = 3600
HOTSPOT_KERNEL = "hotspot"
HOTSPOT_PROBLEM_SIZE = "48"
HOTSPOT_PLAN_INDEX = 474

ORDERING_POLICY = (
    "workflow_benchmark_first_appearance_order_with_plan_index_ordered_attempts"
)
SHARDING_POLICY = (
    "contiguous workflow-benchmark groups; never split one workflow benchmark; "
    "start a new shard before adding a benchmark would exceed max_attempts_per_shard"
)
MATRIX_ROW_POLICY = "one_matrix_include_entry_per_workflow_benchmark"
ATTEMPT_POLICY = (
    "each benchmark matrix row carries ordered per-size attempts preserving the "
    "source plan metadata and timeout_sec=3600"
)
MATRIX_FILENAME_TEMPLATE = "mi300a_terminal_discovery_shard_{shard_index:02d}_matrix.json"

REQUIRED_PLAN_ENTRY_STRINGS = (
    "benchmark",
    "problem_size",
    "hardware_kernel_name",
    "hardware_problem_size",
    "workflow_benchmark",
    "workflow_benchmark_name",
    "workflow_job",
    "tier_name",
    "sizes",
    "size_token",
    "size_label",
    "flag_template",
    "size_label_template",
    "job_id",
    "artifact_id",
)
OPTIONAL_RUNNER_FIELDS = ("extra_flags", "gomemlimit", "gogc")
OPTIONAL_MAPPING_FIELDS = ("kernel_to_workflow_mapping_source", "kernel_to_workflow_mapping_reason")
BENCHMARK_GROUP_CONSTANT_FIELDS = (
    "workflow_benchmark",
    "workflow_benchmark_name",
    "workflow_job",
    "tier",
    "tier_name",
    "flag_template",
    "size_label_template",
    *OPTIONAL_RUNNER_FIELDS,
)


class ShardManifestError(Exception):
    """Raised when terminal discovery shard generation inputs are invalid."""


# ---------------------------------------------------------------------------
# Generic path/JSON helpers
# ---------------------------------------------------------------------------


def repo_path(path_text: str) -> Path:
    path = Path(path_text)
    return path if path.is_absolute() else REPO_ROOT / path


def repo_relative(path: Path) -> str:
    try:
        return path.resolve().relative_to(REPO_ROOT).as_posix()
    except ValueError:
        return path.as_posix()


def json_text(data: Mapping[str, Any]) -> str:
    return json.dumps(data, indent=2) + "\n"


def write_json(path: Path, data: Mapping[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json_text(data), encoding="utf-8")


def canonical_sha256(data: Mapping[str, Any]) -> str:
    return hashlib.sha256(json_text(data).encode("utf-8")).hexdigest()


def load_json_object(path: Path) -> Mapping[str, Any]:
    try:
        with path.open("r", encoding="utf-8") as f:
            data = json.load(f)
    except FileNotFoundError as exc:
        raise ShardManifestError(f"{repo_relative(path)}: file not found") from exc
    except json.JSONDecodeError as exc:
        raise ShardManifestError(f"{repo_relative(path)}: invalid JSON: {exc}") from exc
    if not isinstance(data, Mapping):
        raise ShardManifestError(f"{repo_relative(path)}: JSON root must be an object")
    return data


def list_at(value: Any, label: str) -> list[Any]:
    if not isinstance(value, list):
        raise ShardManifestError(f"{label} must be a list")
    return value


def object_at(value: Any, label: str) -> Mapping[str, Any]:
    if not isinstance(value, Mapping):
        raise ShardManifestError(f"{label} must be an object")
    return value


def as_int(value: Any, label: str) -> int:
    if isinstance(value, bool):
        raise ShardManifestError(f"{label} must be an integer, got boolean {value!r}")
    if isinstance(value, int):
        return value
    if isinstance(value, str) and value.strip().isdigit():
        return int(value.strip())
    raise ShardManifestError(f"{label} must be an integer, got {value!r}")


def str_or_none(value: Any) -> Optional[str]:
    if value is None:
        return None
    text = str(value).strip()
    return text if text else None


def nonempty_str(value: Any, label: str) -> str:
    text = str_or_none(value)
    if text is None:
        raise ShardManifestError(f"{label} must be a non-empty string")
    return text


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


def unique_preserve_order(values: Iterable[str]) -> list[str]:
    seen: set[str] = set()
    ordered: list[str] = []
    for value in values:
        if value in seen:
            continue
        seen.add(value)
        ordered.append(value)
    return ordered


def slugify_identifier(value: str) -> str:
    slug = re.sub(r"[^a-z0-9]+", "-", value.strip().lower()).strip("-")
    return slug or "benchmark"


def shard_id(shard_index: int) -> str:
    return f"mi300a-terminal-discovery-shard-{shard_index:02d}"


def matrix_filename(shard_index: int) -> str:
    return MATRIX_FILENAME_TEMPLATE.format(shard_index=shard_index)


def runnable_unit_id(plan_index: int) -> str:
    return f"mi300a-terminal-discovery-plan-{plan_index:04d}"


def matrix_row_id(workflow_benchmark: str) -> str:
    return f"mi300a-terminal-discovery-benchmark-{slugify_identifier(workflow_benchmark)}"


def expected_attempt_name(plan_index: int) -> str:
    return f"Problem-Size Discovery attempt: plan {plan_index}"


def expected_job_name(workflow_benchmark: str, attempt_count: int) -> str:
    return f"Benchmark Discovery: {workflow_benchmark} ({attempt_count} attempts)"


# ---------------------------------------------------------------------------
# Source plan validation and grouping
# ---------------------------------------------------------------------------


def load_and_validate_plan(path: Path) -> tuple[Mapping[str, Any], list[Mapping[str, Any]]]:
    plan = load_json_object(path)
    label = repo_relative(path)

    if plan.get("schema_name") != PLAN_SCHEMA:
        raise ShardManifestError(f"{label}.schema_name must be {PLAN_SCHEMA!r}, got {plan.get('schema_name')!r}")
    if as_int(plan.get("schema_version"), f"{label}.schema_version") != SCHEMA_VERSION:
        raise ShardManifestError(f"{label}.schema_version must be {SCHEMA_VERSION}")
    if as_int(plan.get("timeout_sec"), f"{label}.timeout_sec") != EXPECTED_TIMEOUT_SEC:
        raise ShardManifestError(f"{label}.timeout_sec must be {EXPECTED_TIMEOUT_SEC}")

    raw_entries = list_at(plan.get("entries"), f"{label}.entries")
    declared_entry_count = as_int(plan.get("entry_count"), f"{label}.entry_count")
    if declared_entry_count != len(raw_entries):
        raise ShardManifestError(
            f"{label}.entry_count must match entries length ({declared_entry_count} != {len(raw_entries)})"
        )
    if declared_entry_count != EXPECTED_ENTRY_COUNT:
        raise ShardManifestError(f"{label}.entry_count must be {EXPECTED_ENTRY_COUNT}, got {declared_entry_count}")

    entries: list[Mapping[str, Any]] = []
    seen_job_ids: set[str] = set()
    duplicate_job_ids: set[str] = set()
    seen_artifact_ids: set[str] = set()
    duplicate_artifact_ids: set[str] = set()
    seen_hardware_pairs: set[tuple[str, str]] = set()
    duplicate_hardware_pairs: set[tuple[str, str]] = set()
    hotspot_indices: list[int] = []

    for offset, raw_entry in enumerate(raw_entries):
        entry_label = f"{label}.entries[{offset}]"
        entry = object_at(raw_entry, entry_label)
        plan_index = as_int(entry.get("plan_index"), f"{entry_label}.plan_index")
        expected_index = offset + 1
        if plan_index != expected_index:
            raise ShardManifestError(f"{entry_label}.plan_index must be {expected_index}, got {plan_index}")
        timeout_sec = as_int(entry.get("timeout_sec"), f"{entry_label}.timeout_sec")
        if timeout_sec != EXPECTED_TIMEOUT_SEC:
            raise ShardManifestError(f"{entry_label}.timeout_sec must be {EXPECTED_TIMEOUT_SEC}, got {timeout_sec}")
        tier = as_int(entry.get("tier"), f"{entry_label}.tier")
        if tier not in (1, 2):
            raise ShardManifestError(f"{entry_label}.tier must be 1 or 2, got {tier}")

        for key in REQUIRED_PLAN_ENTRY_STRINGS:
            nonempty_str(entry.get(key), f"{entry_label}.{key}")
        sizes = nonempty_str(entry.get("sizes"), f"{entry_label}.sizes")
        if len(sizes.split()) != 1:
            raise ShardManifestError(f"{entry_label}.sizes must contain exactly one problem-size token")
        if entry.get("size_token") != sizes:
            raise ShardManifestError(f"{entry_label}.size_token must equal sizes")
        if entry.get("size_label") != entry.get("problem_size"):
            raise ShardManifestError(f"{entry_label}.size_label must equal problem_size")

        job_id_value = nonempty_str(entry.get("job_id"), f"{entry_label}.job_id")
        artifact_id_value = nonempty_str(entry.get("artifact_id"), f"{entry_label}.artifact_id")
        if job_id_value in seen_job_ids:
            duplicate_job_ids.add(job_id_value)
        seen_job_ids.add(job_id_value)
        if artifact_id_value in seen_artifact_ids:
            duplicate_artifact_ids.add(artifact_id_value)
        seen_artifact_ids.add(artifact_id_value)

        hardware_pair = (
            nonempty_str(entry.get("hardware_kernel_name"), f"{entry_label}.hardware_kernel_name").lower(),
            nonempty_str(entry.get("hardware_problem_size"), f"{entry_label}.hardware_problem_size"),
        )
        if hardware_pair in seen_hardware_pairs:
            duplicate_hardware_pairs.add(hardware_pair)
        seen_hardware_pairs.add(hardware_pair)
        if hardware_pair == (HOTSPOT_KERNEL, HOTSPOT_PROBLEM_SIZE):
            hotspot_indices.append(plan_index)
        entries.append(entry)

    errors: list[str] = []
    if duplicate_job_ids:
        errors.append(f"duplicate job_id values: {sorted(duplicate_job_ids)}")
    if duplicate_artifact_ids:
        errors.append(f"duplicate artifact_id values: {sorted(duplicate_artifact_ids)}")
    if duplicate_hardware_pairs:
        errors.append(f"duplicate hardware kernel/problem-size pairs: {sorted(duplicate_hardware_pairs)}")
    if hotspot_indices != [HOTSPOT_PLAN_INDEX]:
        errors.append(
            f"expected exactly hotspot/48 at plan_index {HOTSPOT_PLAN_INDEX}, got {hotspot_indices}"
        )
    if errors:
        raise ShardManifestError(f"{label}: invalid discovery plan for terminal shard surface: " + "; ".join(errors))
    return plan, entries


def group_entries_by_workflow_benchmark(entries: Sequence[Mapping[str, Any]]) -> list[dict[str, Any]]:
    if not entries:
        raise ShardManifestError("cannot group an empty discovery plan")

    # Group ordering is deterministic on the workflow_benchmark's first
    # appearance in the source plan; entries within a group are sorted by
    # plan_index so workflow_benchmark blocks need not be physically
    # contiguous in the source plan (backprop_adjust_weights
    # interleaved with backprop entries).
    groups: list[dict[str, Any]] = []
    group_by_name: dict[str, dict[str, Any]] = {}

    for entry in entries:
        plan_index = as_int(entry.get("plan_index"), "entry.plan_index")
        workflow_benchmark = nonempty_str(entry.get("workflow_benchmark"), f"plan {plan_index}.workflow_benchmark")
        group = group_by_name.get(workflow_benchmark)
        if group is None:
            group = {"workflow_benchmark": workflow_benchmark, "entries": []}
            group_by_name[workflow_benchmark] = group
            groups.append(group)
        group["entries"].append(entry)

    for group in groups:
        group_entries = [object_at(entry, "group.entry") for entry in list_at(group.get("entries"), "group.entries")]
        group_entries.sort(key=lambda entry: as_int(entry.get("plan_index"), "entry.plan_index"))
        group["entries"] = group_entries
        first = group_entries[0]
        benchmark = nonempty_str(first.get("workflow_benchmark"), "group.workflow_benchmark")
        for field in BENCHMARK_GROUP_CONSTANT_FIELDS:
            normalized_values = {str_or_none(entry.get(field)) for entry in group_entries}
            if len(normalized_values) != 1:
                raise ShardManifestError(
                    f"workflow_benchmark {benchmark!r} must have one {field} value across grouped attempts, "
                    f"got {sorted(str(value) for value in normalized_values)}"
                )
        plan_indices = [as_int(entry.get("plan_index"), f"{benchmark}.plan_index") for entry in group_entries]
        group["attempt_count"] = len(group_entries)
        group["plan_index_start"] = plan_indices[0]
        group["plan_index_end"] = plan_indices[-1]
        group["plan_index_ranges"] = make_plan_index_ranges(plan_indices)
    return groups


# ---------------------------------------------------------------------------
# Shard/matrix construction
# ---------------------------------------------------------------------------


def partition_benchmark_groups(
    groups: Sequence[Mapping[str, Any]],
    max_attempts_per_shard: int,
) -> list[list[Mapping[str, Any]]]:
    if max_attempts_per_shard < 1:
        raise ShardManifestError("max_attempts_per_shard must be positive")
    if not groups:
        raise ShardManifestError("cannot shard an empty discovery plan")

    shards: list[list[Mapping[str, Any]]] = []
    current: list[Mapping[str, Any]] = []
    current_attempts = 0
    for group in groups:
        benchmark = nonempty_str(group.get("workflow_benchmark"), "group.workflow_benchmark")
        attempts = as_int(group.get("attempt_count"), f"{benchmark}.attempt_count")
        if attempts > max_attempts_per_shard:
            raise ShardManifestError(
                f"workflow_benchmark {benchmark!r} has {attempts} attempts, exceeding "
                f"max_attempts_per_shard {max_attempts_per_shard}"
            )
        if current and current_attempts + attempts > max_attempts_per_shard:
            shards.append(current)
            current = []
            current_attempts = 0
        current.append(group)
        current_attempts += attempts
    if current:
        shards.append(current)
    return shards


def attempt_from_plan_entry(
    entry: Mapping[str, Any],
    *,
    shard_index: int,
    shard_attempt_index: int,
    benchmark_entry_index: int,
    benchmark_attempt_index: int,
    benchmark_row_id: str,
    shard_count: int,
) -> dict[str, Any]:
    plan_index = as_int(entry.get("plan_index"), "entry.plan_index")
    attempt: dict[str, Any] = {
        "shard_index": shard_index,
        "shard_id": shard_id(shard_index),
        "shard_count": shard_count,
        "shard_attempt_index": shard_attempt_index,
        "benchmark_entry_index": benchmark_entry_index,
        "benchmark_attempt_index": benchmark_attempt_index,
        "matrix_row_id": benchmark_row_id,
        "plan_index": plan_index,
        "runnable_unit_id": runnable_unit_id(plan_index),
        "attempt_name": expected_attempt_name(plan_index),
        # matrix.benchmark is intentionally the runnable workflow benchmark.  The
        # original plan/comparison benchmark remains present as plan_benchmark for
        # per-attempt outcome accounting (notably FindMinSAD -> parboil_sad).
        "benchmark": nonempty_str(entry.get("workflow_benchmark"), f"plan {plan_index}.workflow_benchmark"),
        "plan_benchmark": nonempty_str(entry.get("benchmark"), f"plan {plan_index}.benchmark"),
        "problem_size": nonempty_str(entry.get("problem_size"), f"plan {plan_index}.problem_size"),
        "hardware_kernel_name": nonempty_str(
            entry.get("hardware_kernel_name"), f"plan {plan_index}.hardware_kernel_name"
        ),
        "hardware_problem_size": nonempty_str(
            entry.get("hardware_problem_size"), f"plan {plan_index}.hardware_problem_size"
        ),
        "workflow_benchmark": nonempty_str(entry.get("workflow_benchmark"), f"plan {plan_index}.workflow_benchmark"),
        "workflow_benchmark_name": nonempty_str(
            entry.get("workflow_benchmark_name"), f"plan {plan_index}.workflow_benchmark_name"
        ),
        "workflow_job": nonempty_str(entry.get("workflow_job"), f"plan {plan_index}.workflow_job"),
        "tier": as_int(entry.get("tier"), f"plan {plan_index}.tier"),
        "tier_name": nonempty_str(entry.get("tier_name"), f"plan {plan_index}.tier_name"),
        "sizes": nonempty_str(entry.get("sizes"), f"plan {plan_index}.sizes"),
        "size_token": nonempty_str(entry.get("size_token"), f"plan {plan_index}.size_token"),
        "size_label": nonempty_str(entry.get("size_label"), f"plan {plan_index}.size_label"),
        "flag_template": nonempty_str(entry.get("flag_template"), f"plan {plan_index}.flag_template"),
        "size_label_template": nonempty_str(
            entry.get("size_label_template"), f"plan {plan_index}.size_label_template"
        ),
        "timeout_sec": as_int(entry.get("timeout_sec"), f"plan {plan_index}.timeout_sec"),
        "artifact_id": nonempty_str(entry.get("artifact_id"), f"plan {plan_index}.artifact_id"),
        "plan_job_id": nonempty_str(entry.get("job_id"), f"plan {plan_index}.job_id"),
    }
    for key in OPTIONAL_RUNNER_FIELDS:
        value = str_or_none(entry.get(key))
        if value is not None:
            attempt[key] = value
    for key in OPTIONAL_MAPPING_FIELDS:
        value = str_or_none(entry.get(key))
        if value is not None:
            attempt[key] = value
    return attempt


def benchmark_row_from_group(
    group: Mapping[str, Any],
    *,
    shard_index: int,
    shard_entry_index: int,
    shard_count: int,
    shard_attempt_start_index: int,
) -> dict[str, Any]:
    entries = [object_at(entry, "group.entries[]") for entry in list_at(group.get("entries"), "group.entries")]
    first = entries[0]
    workflow_benchmark = nonempty_str(first.get("workflow_benchmark"), "group.workflow_benchmark")
    row_id = matrix_row_id(workflow_benchmark)
    attempts = [
        attempt_from_plan_entry(
            entry,
            shard_index=shard_index,
            shard_attempt_index=shard_attempt_start_index + offset - 1,
            benchmark_entry_index=shard_entry_index,
            benchmark_attempt_index=offset,
            benchmark_row_id=row_id,
            shard_count=shard_count,
        )
        for offset, entry in enumerate(entries, start=1)
    ]
    plan_indices = [as_int(attempt["plan_index"], "attempt.plan_index") for attempt in attempts]
    row: dict[str, Any] = {
        "shard_index": shard_index,
        "shard_id": shard_id(shard_index),
        "shard_count": shard_count,
        "shard_entry_index": shard_entry_index,
        "matrix_row_id": row_id,
        "job_name": expected_job_name(workflow_benchmark, len(attempts)),
        "benchmark": workflow_benchmark,
        "workflow_benchmark": workflow_benchmark,
        "workflow_benchmark_name": nonempty_str(
            first.get("workflow_benchmark_name"), f"{workflow_benchmark}.workflow_benchmark_name"
        ),
        "workflow_job": nonempty_str(first.get("workflow_job"), f"{workflow_benchmark}.workflow_job"),
        "tier": as_int(first.get("tier"), f"{workflow_benchmark}.tier"),
        "tier_name": nonempty_str(first.get("tier_name"), f"{workflow_benchmark}.tier_name"),
        "flag_template": nonempty_str(first.get("flag_template"), f"{workflow_benchmark}.flag_template"),
        "size_label_template": nonempty_str(
            first.get("size_label_template"), f"{workflow_benchmark}.size_label_template"
        ),
        "timeout_sec": EXPECTED_TIMEOUT_SEC,
        "attempt_count": len(attempts),
        "per_size_attempt_count": len(attempts),
        "plan_index_start": plan_indices[0],
        "plan_index_end": plan_indices[-1],
        "plan_index_ranges": make_plan_index_ranges(plan_indices),
        "problem_sizes": [str(attempt["problem_size"]) for attempt in attempts],
        "unique_problem_sizes": unique_preserve_order(str(attempt["problem_size"]) for attempt in attempts),
        "size_tokens": [str(attempt["size_token"]) for attempt in attempts],
        "hardware_kernel_names": unique_preserve_order(str(attempt["hardware_kernel_name"]) for attempt in attempts),
        "hardware_problem_sizes": [str(attempt["hardware_problem_size"]) for attempt in attempts],
        "attempts": attempts,
    }
    for key in OPTIONAL_RUNNER_FIELDS:
        value = str_or_none(first.get(key))
        if value is not None:
            row[key] = value
    for key in OPTIONAL_MAPPING_FIELDS:
        values = unique_preserve_order(
            value for value in (str_or_none(entry.get(key)) for entry in entries) if value is not None
        )
        if len(values) == 1:
            row[key] = values[0]
        elif len(values) > 1:
            row[f"{key}s"] = values
    return row


def attempts_from_rows(rows: Sequence[Mapping[str, Any]], label: str = "rows") -> list[Mapping[str, Any]]:
    attempts: list[Mapping[str, Any]] = []
    for row_offset, row in enumerate(rows):
        row_attempts = list_at(row.get("attempts"), f"{label}[{row_offset}].attempts")
        attempts.extend(object_at(attempt, f"{label}[{row_offset}].attempts[]") for attempt in row_attempts)
    return attempts


def build_matrix_doc(
    shard_groups: Sequence[Mapping[str, Any]],
    *,
    shard_index: int,
    shard_count: int,
    max_attempts_per_shard: int,
    plan_path: Path,
    manifest_path: Path,
) -> dict[str, Any]:
    include: list[dict[str, Any]] = []
    next_shard_attempt_index = 1
    for row_offset, group in enumerate(shard_groups, start=1):
        row = benchmark_row_from_group(
            group,
            shard_index=shard_index,
            shard_entry_index=row_offset,
            shard_count=shard_count,
            shard_attempt_start_index=next_shard_attempt_index,
        )
        include.append(row)
        next_shard_attempt_index += as_int(row.get("attempt_count"), f"shard {shard_index}.row[{row_offset}].attempt_count")

    attempts = attempts_from_rows(include, f"shard {shard_index}.include")
    plan_indices = [as_int(attempt["plan_index"], "attempt.plan_index") for attempt in attempts]
    if len(attempts) > max_attempts_per_shard:
        raise ShardManifestError(
            f"shard {shard_index} has {len(attempts)} attempts, exceeding max_attempts_per_shard {max_attempts_per_shard}"
        )
    if not plan_indices:
        raise ShardManifestError(f"shard {shard_index} would be empty")
    return {
        "schema_name": MATRIX_SCHEMA,
        "schema_version": SCHEMA_VERSION,
        "source_manifest": repo_relative(manifest_path),
        "source_plan": repo_relative(plan_path),
        "shard_index": shard_index,
        "shard_id": shard_id(shard_index),
        "shard_count": shard_count,
        "max_attempts_per_shard": max_attempts_per_shard,
        "entry_count": len(attempts),
        "attempt_count": len(attempts),
        "per_size_attempt_count": len(attempts),
        "benchmark_row_count": len(include),
        "visible_matrix_row_count": len(include),
        "plan_index_start": plan_indices[0],
        "plan_index_end": plan_indices[-1],
        "plan_index_ranges": make_plan_index_ranges(plan_indices),
        "timeout_sec": EXPECTED_TIMEOUT_SEC,
        "ordering_policy": ORDERING_POLICY,
        "sharding_policy": SHARDING_POLICY,
        "matrix_row_policy": MATRIX_ROW_POLICY,
        "attempt_policy": ATTEMPT_POLICY,
        "include": include,
    }


def hotspot_summary_from_attempts(
    attempts: Sequence[Mapping[str, Any]],
    matrix_path_by_shard: Mapping[int, str],
) -> dict[str, Any]:
    matches = [
        attempt
        for attempt in attempts
        if str(attempt.get("hardware_kernel_name", "")).strip().lower() == HOTSPOT_KERNEL
        and str(attempt.get("hardware_problem_size", "")).strip() == HOTSPOT_PROBLEM_SIZE
    ]
    if len(matches) != 1:
        raise ShardManifestError(
            f"expected exactly one hotspot/48 attempt at plan_index {HOTSPOT_PLAN_INDEX}, got {len(matches)}"
        )
    attempt = matches[0]
    plan_index = as_int(attempt.get("plan_index"), "hotspot_48.plan_index")
    if plan_index != HOTSPOT_PLAN_INDEX:
        raise ShardManifestError(
            f"hotspot/48 attempt must be at plan_index {HOTSPOT_PLAN_INDEX}, got {plan_index}"
        )
    timeout_sec = as_int(attempt.get("timeout_sec"), "hotspot_48.timeout_sec")
    if timeout_sec != EXPECTED_TIMEOUT_SEC:
        raise ShardManifestError(f"hotspot/48 timeout_sec must be {EXPECTED_TIMEOUT_SEC}, got {timeout_sec}")
    shard_index = as_int(attempt.get("shard_index"), "hotspot_48.shard_index")
    return {
        "plan_index": HOTSPOT_PLAN_INDEX,
        "shard_index": shard_index,
        "shard_id": str(attempt.get("shard_id")),
        "shard_attempt_index": as_int(attempt.get("shard_attempt_index"), "hotspot_48.shard_attempt_index"),
        "benchmark_entry_index": as_int(attempt.get("benchmark_entry_index"), "hotspot_48.benchmark_entry_index"),
        "benchmark_attempt_index": as_int(attempt.get("benchmark_attempt_index"), "hotspot_48.benchmark_attempt_index"),
        "matrix_path": matrix_path_by_shard[shard_index],
        "matrix_row_id": str(attempt.get("matrix_row_id")),
        "runnable_unit_id": str(attempt.get("runnable_unit_id")),
        "benchmark": str(attempt.get("benchmark")),
        "plan_benchmark": str(attempt.get("plan_benchmark")),
        "problem_size": HOTSPOT_PROBLEM_SIZE,
        "hardware_kernel_name": HOTSPOT_KERNEL,
        "hardware_problem_size": HOTSPOT_PROBLEM_SIZE,
        "sizes": str(attempt.get("sizes")),
        "timeout_sec": timeout_sec,
    }


def build_terminal_discovery_shards(
    plan_path: Path,
    manifest_path: Path,
    shard_dir: Path,
    *,
    max_attempts_per_shard: int = DEFAULT_MAX_ATTEMPTS_PER_SHARD,
) -> tuple[dict[str, Any], list[tuple[Path, dict[str, Any]]]]:
    plan, entries = load_and_validate_plan(plan_path)
    benchmark_groups = group_entries_by_workflow_benchmark(entries)
    shards = partition_benchmark_groups(benchmark_groups, max_attempts_per_shard)
    shard_count = len(shards)

    matrix_docs: list[tuple[Path, dict[str, Any]]] = []
    matrix_path_by_shard: dict[int, str] = {}
    for index, shard_groups in enumerate(shards, start=1):
        matrix_path = shard_dir / matrix_filename(index)
        matrix_doc = build_matrix_doc(
            shard_groups,
            shard_index=index,
            shard_count=shard_count,
            max_attempts_per_shard=max_attempts_per_shard,
            plan_path=plan_path,
            manifest_path=manifest_path,
        )
        matrix_docs.append((matrix_path, matrix_doc))
        matrix_path_by_shard[index] = repo_relative(matrix_path)

    all_rows = [row for _, matrix in matrix_docs for row in matrix["include"]]
    all_attempts = attempts_from_rows(all_rows, "all_rows")
    observed_plan_indices = [as_int(attempt.get("plan_index"), "attempt.plan_index") for attempt in all_attempts]
    expected_plan_indices = set(range(1, len(entries) + 1))
    observed_set = set(observed_plan_indices)
    missing = expected_plan_indices - observed_set
    extra = observed_set - expected_plan_indices
    duplicates = sorted(index for index in observed_set if observed_plan_indices.count(index) > 1)
    if missing or extra or duplicates:
        raise ShardManifestError(
            "terminal discovery benchmark shard matrices must cover all 1416 plan entries exactly once "
            f"(duplicates={make_plan_index_ranges(duplicates)}, "
            f"missing={make_plan_index_ranges(missing)}, extra={make_plan_index_ranges(extra)})"
        )

    benchmark_names = [nonempty_str(row.get("workflow_benchmark"), "row.workflow_benchmark") for row in all_rows]
    duplicate_benchmarks = sorted({name for name in benchmark_names if benchmark_names.count(name) > 1})
    if duplicate_benchmarks:
        raise ShardManifestError(f"workflow benchmark matrix rows must be unique: {duplicate_benchmarks}")

    shard_summaries: list[dict[str, Any]] = []
    for matrix_path, matrix in matrix_docs:
        include = [object_at(row, f"{repo_relative(matrix_path)}.include[]") for row in list_at(matrix.get("include"), f"{repo_relative(matrix_path)}.include")]
        attempts = attempts_from_rows(include, repo_relative(matrix_path))
        plan_indices = [as_int(attempt.get("plan_index"), "attempt.plan_index") for attempt in attempts]
        shard_index = as_int(matrix.get("shard_index"), f"{repo_relative(matrix_path)}.shard_index")
        shard_benchmarks = [nonempty_str(row.get("workflow_benchmark"), "row.workflow_benchmark") for row in include]
        shard_summaries.append(
            {
                "shard_index": shard_index,
                "shard_id": str(matrix.get("shard_id")),
                "matrix_path": repo_relative(matrix_path),
                "matrix_sha256": canonical_sha256(matrix),
                "entry_count": len(attempts),
                "attempt_count": len(attempts),
                "per_size_attempt_count": len(attempts),
                "benchmark_row_count": len(include),
                "visible_matrix_row_count": len(include),
                "timeout_sec": EXPECTED_TIMEOUT_SEC,
                "plan_index_start": plan_indices[0],
                "plan_index_end": plan_indices[-1],
                "plan_index_ranges": make_plan_index_ranges(plan_indices),
                "first_runnable_unit_id": runnable_unit_id(plan_indices[0]),
                "last_runnable_unit_id": runnable_unit_id(plan_indices[-1]),
                "benchmark_start": shard_benchmarks[0],
                "benchmark_end": shard_benchmarks[-1],
                "workflow_benchmarks": shard_benchmarks,
                "contains_hotspot_48": HOTSPOT_PLAN_INDEX in plan_indices,
            }
        )

    attempt_counts = [summary["attempt_count"] for summary in shard_summaries]
    benchmark_row_counts = [summary["benchmark_row_count"] for summary in shard_summaries]
    hotspot_48 = hotspot_summary_from_attempts(all_attempts, matrix_path_by_shard)
    manifest = {
        "schema_name": MANIFEST_SCHEMA,
        "schema_version": SCHEMA_VERSION,
        "source_plan": {
            "artifact": repo_relative(plan_path),
            "schema_name": plan.get("schema_name"),
            "schema_version": plan.get("schema_version"),
            "entry_count": len(entries),
            "timeout_sec": plan.get("timeout_sec"),
            "tier_counts": plan.get("tier_counts"),
        },
        "source_paths": {
            "plan": repo_relative(plan_path),
            "shard_dir": repo_relative(shard_dir),
        },
        "generation_policy": {
            "ordering_policy": ORDERING_POLICY,
            "sharding_policy": SHARDING_POLICY,
            "matrix_row_policy": MATRIX_ROW_POLICY,
            "attempt_policy": ATTEMPT_POLICY,
        },
        "entry_count": len(entries),
        "attempt_count": len(all_attempts),
        "per_size_attempt_count": len(all_attempts),
        # Kept for provenance-collector compatibility: terminal outcome units remain per source plan entry.
        "runnable_unit_count": len(all_attempts),
        "benchmark_count": len(benchmark_groups),
        "visible_matrix_row_count": len(all_rows),
        "timeout_sec": EXPECTED_TIMEOUT_SEC,
        "max_attempts_per_shard": max_attempts_per_shard,
        "shard_count": shard_count,
        "shard_attempt_count_min": min(attempt_counts),
        "shard_attempt_count_max": max(attempt_counts),
        "shard_benchmark_row_count_min": min(benchmark_row_counts),
        "shard_benchmark_row_count_max": max(benchmark_row_counts),
        "plan_index_ranges": make_plan_index_ranges(observed_plan_indices),
        "coverage": {
            "expected_plan_index_count": len(entries),
            "represented_plan_index_count": len(observed_set),
            "attempt_count": len(all_attempts),
            "per_size_attempt_count": len(all_attempts),
            "runnable_unit_count": len(all_attempts),
            "visible_matrix_row_count": len(all_rows),
            "benchmark_count": len(benchmark_groups),
            "duplicate_plan_index_count": len(duplicates),
            "duplicate_plan_index_ranges": make_plan_index_ranges(duplicates),
            "missing_plan_index_count": len(missing),
            "missing_plan_index_ranges": make_plan_index_ranges(missing),
            "extra_plan_index_count": len(extra),
            "extra_plan_index_ranges": make_plan_index_ranges(extra),
        },
        "benchmark_coverage": {
            "source_workflow_benchmark_count": len(benchmark_groups),
            "visible_matrix_row_count": len(all_rows),
            "duplicate_workflow_benchmark_count": len(duplicate_benchmarks),
            "duplicate_workflow_benchmarks": duplicate_benchmarks,
        },
        "hotspot_48": hotspot_48,
        "shards": shard_summaries,
    }
    return manifest, matrix_docs


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Generate deterministic per-benchmark MI300A terminal discovery shard manifests from the "
            "checked-in 1416-entry problem-size discovery plan."
        )
    )
    parser.add_argument("--plan", default=str(DEFAULT_PLAN), help="Path to mi300a_problem_size_discovery_plan.json")
    parser.add_argument(
        "--manifest-output",
        default=str(DEFAULT_MANIFEST),
        help="Output path for the terminal discovery shard manifest JSON",
    )
    parser.add_argument(
        "--shard-dir",
        default=str(DEFAULT_GENERATED_DIR),
        help="Directory for per-shard matrix JSON files",
    )
    parser.add_argument(
        "--max-attempts-per-shard",
        "--max-entries-per-shard",
        dest="max_attempts_per_shard",
        type=int,
        default=DEFAULT_MAX_ATTEMPTS_PER_SHARD,
        help="Maximum per-size attempts per benchmark shard (default: 256); --max-entries-per-shard is a legacy alias",
    )
    return parser.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    args = parse_args(argv)
    plan_path = repo_path(args.plan)
    manifest_path = repo_path(args.manifest_output)
    shard_dir = repo_path(args.shard_dir)
    try:
        manifest, matrix_docs = build_terminal_discovery_shards(
            plan_path,
            manifest_path,
            shard_dir,
            max_attempts_per_shard=args.max_attempts_per_shard,
        )
    except ShardManifestError as exc:
        print("FAIL: MI300A terminal discovery benchmark shard manifest generation rejected input.", file=sys.stderr)
        print(f"  - {exc}", file=sys.stderr)
        return 1

    write_json(manifest_path, manifest)
    for matrix_path, matrix_doc in matrix_docs:
        write_json(matrix_path, matrix_doc)

    print(
        "WROTE MI300A terminal discovery benchmark shard manifests: "
        f"entries={manifest['entry_count']} attempts={manifest['attempt_count']} "
        f"benchmark_rows={manifest['visible_matrix_row_count']} shards={manifest['shard_count']} "
        f"max_attempts_per_shard={manifest['max_attempts_per_shard']} "
        f"shard_attempt_count_max={manifest['shard_attempt_count_max']} "
        f"hotspot_48_plan_index={manifest['hotspot_48']['plan_index']} "
        f"manifest={repo_relative(manifest_path)} shard_dir={repo_relative(shard_dir)}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
