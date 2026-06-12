#!/usr/bin/env python3
"""Collect terminal MI300A discovery outcome artifacts into compact provenance.

Terminal discovery records one terminal outcome JSON record per 1416-entry
problem-size discovery plan row.  Future Actions attempts may upload those
records either as one legacy per-plan artifact or nested inside one visible
benchmark-level artifact.  This collector is offline and purely deterministic:
it reads both shapes, validates exact plan coverage, normalizes terminal
statuses into the finishable-manifest provenance schema, and writes a compact
provenance JSON that can be passed to
``generate_mi300a_finishable_size_manifest.py``.
"""

from __future__ import annotations

import argparse
import collections
from dataclasses import dataclass
import hashlib
import json
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Iterable, Mapping, Optional, Sequence

import generate_mi300a_finishable_size_manifest as finishable


REPO_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_PLAN = finishable.DEFAULT_PLAN
DEFAULT_SHARD_MANIFEST = REPO_ROOT / "benchmark-comparison" / "generated" / "mi300a_terminal_discovery_shard_manifest.json"
DEFAULT_OUTCOMES_ROOT = REPO_ROOT / "terminal-discovery-artifacts"
DEFAULT_OUTPUT = REPO_ROOT / "mi300a_terminal_discovery_provenance.json"

PROVENANCE_SCHEMA = finishable.EXPECTED_PROVENANCE_SCHEMA
PROVENANCE_SCHEMA_VERSION = 3
OUTCOME_SCHEMA = "mgpusim.mi300a_terminal_discovery_outcome"
BENCHMARK_OUTCOME_SCHEMAS = {
    "mgpusim.mi300a_terminal_discovery_benchmark_outcomes",
    "mgpusim.mi300a_terminal_discovery_benchmark_outcome_artifact",
}
NESTED_OUTCOME_KEYS = ("outcomes", "terminal_outcomes", "per_size_outcomes", "attempt_outcomes", "records")
ACCOUNTING_SCHEMA = "mgpusim.mi300a_terminal_outcome_accounting"
VALIDATION_SCHEMA = "mgpusim.mi300a_terminal_discovery_provenance_validation"
SCHEMA_VERSION = 1
EXPECTED_SHARD_MANIFEST_SCHEMAS = {
    "mgpusim.mi300a_terminal_discovery_shard_manifest",
    "mgpusim.mi300a_terminal_discovery_benchmark_shard_manifest",
}
EXPECTED_ENTRY_COUNT = finishable.EXPECTED_ENTRY_COUNT
EXPECTED_TIMEOUT_SEC = finishable.EXPECTED_TIMEOUT_SEC
EXPECTED_BRANCH = "e15-m9-add-terminal-mi300a-discovery-dispatch-surface"

# The workflow step records simulator/runtime state in ``status``.  The compact
# finishable provenance uses the older ``terminal_outcome`` vocabulary.  All
# accepted statuses below are terminal for accounting purposes.
STATUS_TO_TERMINAL_OUTCOME = {
    "success": ("success", "completed_success", "sim_row_emitted", "ok"),
    "timeout": ("timed_out", "timed_out", "no_sim_row", "timeout_3600s"),
    "timed_out": ("timed_out", "timed_out", "no_sim_row", "timeout_3600s"),
    "failed": ("failed", "failed", "no_sim_row", "failed"),
    "failure": ("failed", "failed", "no_sim_row", "failed"),
    "crash": ("failed", "failed", "no_sim_row", "crash"),
    "build_failed": ("failed", "failed", "no_sim_row", "build_failed"),
    "missing_sqlite": ("failed", "failed", "no_sim_row", "missing_sqlite"),
    "missing_kernel_time": ("failed", "failed", "no_sim_row", "missing_kernel_time"),
    "missing_output": ("failed", "failed", "no_sim_row", "missing_output"),
    "skipped": ("skipped", "skipped", "no_sim_row", "skipped"),
    "cancelled": ("cancelled", "cancelled", "no_sim_row", "cancelled"),
    "canceled": ("cancelled", "cancelled", "no_sim_row", "cancelled"),
}
NON_TERMINAL_STATUSES = {"queued", "in_progress", "pending", "waiting", "requested", "non_terminal"}


class CollectionError(Exception):
    """Raised when terminal outcome artifacts cannot form valid provenance."""


@dataclass(frozen=True)
class OutcomeSource:
    """One per-size terminal outcome record and its source artifact metadata."""

    path: Path
    label: str
    outcome: Mapping[str, Any]
    container: Optional[Mapping[str, Any]] = None
    container_label: Optional[str] = None


@dataclass(frozen=True)
class ExpectedSourceIdentity:
    """Requested single-run identity every outcome source must claim."""

    run_id: int
    branch: str
    head_sha: str
    workflow: str
    milestone: str
    collection_run_id: str
    issue: int


@dataclass(frozen=True)
class DeduplicatedOutcomeSources:
    """Terminal outcome sources after deterministic logical de-duplication."""

    selected_by_plan: Mapping[int, OutcomeSource]
    normalized_by_plan: Mapping[int, Mapping[str, Any]]
    raw_source_record_count: int
    raw_source_file_count: int
    raw_shape_counts: Mapping[str, int]
    selected_shape_counts: Mapping[str, int]
    discarded_shape_counts: Mapping[str, int]
    raw_duplicate_plan_indices: Sequence[int]


SOURCE_SHAPE_BENCHMARK_NESTED = "benchmark_nested"
SOURCE_SHAPE_STANDALONE = "standalone"
SOURCE_SHAPE_PREFERENCE = {
    SOURCE_SHAPE_BENCHMARK_NESTED: 0,
    SOURCE_SHAPE_STANDALONE: 1,
}
OUTCOME_SOURCE_DEDUPLICATION_SCHEMA = "mgpusim.mi300a_terminal_outcome_source_deduplication"
OUTCOME_SOURCE_DEDUPLICATION_POLICY = (
    "group terminal discovery outcome records by plan_index after plan/shard validation; "
    "reject duplicates that disagree on terminal/provenance-critical fields; prefer "
    "benchmark-level nested records over standalone per-plan JSON records; tie-break by source label"
)
SOURCE_POLICY_SCHEMA = "mgpusim.mi300a_terminal_source_policy"
SOURCE_POLICY_NAME = "single_run_terminal_source_identity"
SOURCE_POLICY_TEXT = (
    "all terminal outcome source records must identify the same requested/top-level "
    "GitHub Actions run, branch/ref, SHA, workflow, and milestone/issue; "
    "recovery-only artifacts and prior cancelled-run observations cannot be unioned "
    "under one single-run provenance identity"
)
SOURCE_POLICY_REQUIRED_FIELDS = (
    "run_id",
    "branch",
    "head_sha",
    "workflow",
    "milestone",
    "collection_run_id",
    "issue",
)
SOURCE_IDENTITY_INT_ALIASES: tuple[tuple[str, tuple[str, ...]], ...] = (
    ("run_id", ("run_id", "run_database_id", "workflow_run_id")),
    ("issue", ("issue", "issue_id")),
)
SOURCE_IDENTITY_TEXT_ALIASES: tuple[tuple[str, tuple[str, ...]], ...] = (
    ("branch", ("branch", "head_branch", "ref")),
    ("head_sha", ("head_sha", "sha", "commit_sha")),
    ("workflow", ("workflow", "dispatch_workflow")),
    ("milestone", ("milestone",)),
    ("collection_run_id", ("collection_run_id",)),
)


# ---------------------------------------------------------------------------
# Small path / JSON / formatting helpers
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
        raise CollectionError(f"{repo_relative(path)}: file not found") from exc
    except json.JSONDecodeError as exc:
        raise CollectionError(f"{repo_relative(path)}: invalid JSON: {exc}") from exc
    if not isinstance(data, Mapping):
        raise CollectionError(f"{repo_relative(path)}: JSON root must be an object")
    return data


def write_json(path: Path, data: Mapping[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")


def as_int(value: Any, label: str) -> int:
    try:
        return finishable.as_int(value, label)
    except finishable.ManifestError as exc:
        raise CollectionError(str(exc)) from exc


def optional_int(value: Any, label: str) -> Optional[int]:
    if value is None or value == "":
        return None
    return as_int(value, label)


def optional_float(value: Any, label: str) -> Optional[float]:
    if value is None or value == "":
        return None
    try:
        return float(value)
    except (TypeError, ValueError) as exc:
        raise CollectionError(f"{label} must be a number, got {value!r}") from exc


def str_or_none(value: Any) -> Optional[str]:
    return finishable.str_or_none(value)


def nonempty_str(value: Any, label: str) -> str:
    try:
        return finishable.nonempty_str(value, label)
    except finishable.ManifestError as exc:
        raise CollectionError(str(exc)) from exc


def object_at(value: Any, label: str) -> Mapping[str, Any]:
    try:
        return finishable.object_at(value, label)
    except finishable.ManifestError as exc:
        raise CollectionError(str(exc)) from exc


def list_at(value: Any, label: str) -> list[Any]:
    try:
        return finishable.list_at(value, label)
    except finishable.ManifestError as exc:
        raise CollectionError(str(exc)) from exc


def make_ranges(values: Iterable[int]) -> list[str]:
    return finishable.make_plan_index_ranges(values)


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            digest.update(chunk)
    return "sha256:" + digest.hexdigest()


def parse_utc_timestamp(value: Any) -> Optional[datetime]:
    text = str_or_none(value)
    if text is None:
        return None
    try:
        parsed = datetime.fromisoformat(text.replace("Z", "+00:00"))
    except ValueError:
        return None
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=timezone.utc)
    return parsed.astimezone(timezone.utc)


def duration_seconds(started_at: Any, finished_at: Any) -> Optional[int]:
    start = parse_utc_timestamp(started_at)
    finish = parse_utc_timestamp(finished_at)
    if start is None or finish is None:
        return None
    return max(0, int(round((finish - start).total_seconds())))


def zero_inclusive_counts(bucket_sets: Mapping[str, set[int]], buckets: Sequence[str]) -> dict[str, int]:
    return {bucket: len(bucket_sets[bucket]) for bucket in buckets}


# ---------------------------------------------------------------------------
# Source surface / artifact validation
# ---------------------------------------------------------------------------


def load_plan(plan_path: Path) -> tuple[Mapping[str, Any], list[Mapping[str, Any]], dict[int, Mapping[str, Any]]]:
    try:
        return finishable.load_and_validate_plan(plan_path)
    except finishable.ManifestError as exc:
        raise CollectionError(str(exc)) from exc


def load_shard_plan_index_map(shard_manifest_path: Path, plan_count: int) -> dict[int, int]:
    manifest = load_json_object(shard_manifest_path)
    label = repo_relative(shard_manifest_path)
    if manifest.get("schema_name") not in EXPECTED_SHARD_MANIFEST_SCHEMAS:
        expected = ", ".join(sorted(EXPECTED_SHARD_MANIFEST_SCHEMAS))
        raise CollectionError(
            f"{label}.schema_name must be one of {expected}, got {manifest.get('schema_name')!r}"
        )
    if as_int(manifest.get("entry_count"), f"{label}.entry_count") != plan_count:
        raise CollectionError(f"{label}.entry_count must be {plan_count}")
    if as_int(manifest.get("runnable_unit_count"), f"{label}.runnable_unit_count") != plan_count:
        raise CollectionError(f"{label}.runnable_unit_count must be {plan_count}")
    if as_int(manifest.get("timeout_sec"), f"{label}.timeout_sec") != EXPECTED_TIMEOUT_SEC:
        raise CollectionError(f"{label}.timeout_sec must be {EXPECTED_TIMEOUT_SEC}")

    coverage = object_at(manifest.get("coverage"), f"{label}.coverage")
    for key in ("duplicate_plan_index_count", "missing_plan_index_count", "extra_plan_index_count"):
        if as_int(coverage.get(key), f"{label}.coverage.{key}") != 0:
            raise CollectionError(f"{label}.coverage.{key} must be 0")

    hotspot = object_at(manifest.get("hotspot_48"), f"{label}.hotspot_48")
    if as_int(hotspot.get("plan_index"), f"{label}.hotspot_48.plan_index") != 474:
        raise CollectionError(f"{label}.hotspot_48.plan_index must be 474")

    shard_by_plan: dict[int, int] = {}
    shards = list_at(manifest.get("shards"), f"{label}.shards")
    for offset, raw_shard in enumerate(shards):
        shard = object_at(raw_shard, f"{label}.shards[{offset}]")
        shard_index = as_int(shard.get("shard_index"), f"{label}.shards[{offset}].shard_index")
        try:
            plan_indices = finishable.parse_plan_index_ranges(
                shard.get("plan_index_ranges"), f"{label}.shards[{offset}].plan_index_ranges", plan_count
            )
        except finishable.ManifestError as exc:
            raise CollectionError(str(exc)) from exc
        if as_int(shard.get("entry_count"), f"{label}.shards[{offset}].entry_count") != len(plan_indices):
            raise CollectionError(f"{label}.shards[{offset}].entry_count must match plan_index_ranges cardinality")
        for plan_index in plan_indices:
            if plan_index in shard_by_plan:
                raise CollectionError(f"{label}: plan_index {plan_index} appears in multiple shards")
            shard_by_plan[plan_index] = shard_index

    expected = set(range(1, plan_count + 1))
    missing = expected - set(shard_by_plan)
    extra = set(shard_by_plan) - expected
    if missing or extra:
        raise CollectionError(
            "terminal shard manifest must map every plan entry exactly once "
            f"(missing={make_ranges(missing)}, extra={make_ranges(extra)})"
        )
    return shard_by_plan


def outcome_like_path(path: Path) -> bool:
    name = path.name.lower()
    return (
        name.startswith("outcome_mi300a_terminal_discovery_plan_")
        or ("terminal" in name and "discovery" in name and "outcome" in name and name.endswith(".json"))
    )


def candidate_outcome_json_paths(roots: Sequence[Path]) -> list[Path]:
    paths: list[Path] = []
    for root in roots:
        if root.is_file():
            paths.append(root)
        elif root.is_dir():
            # Downloaded Actions artifacts can be one JSON per plan entry (legacy
            # surface) or one JSON per visible benchmark row with nested per-size
            # outcomes.  Scan JSON files under the supplied artifact root and let
            # extract_outcome_sources ignore unrelated metadata JSON.
            paths.extend(sorted(root.glob("**/*.json")))
        else:
            raise CollectionError(f"{repo_relative(root)}: outcome root does not exist")
    return sorted({path.resolve(): path for path in paths}.values(), key=lambda path: path.as_posix())


def inherit_benchmark_outcome_defaults(
    container: Mapping[str, Any], record: Mapping[str, Any]
) -> dict[str, Any]:
    inherited: dict[str, Any] = {}
    if container.get("schema_name") in BENCHMARK_OUTCOME_SCHEMAS:
        inherited.update(
            {
                "schema_name": OUTCOME_SCHEMA,
                "schema_version": container.get("outcome_schema_version", SCHEMA_VERSION),
                "terminal_discovery": True,
                "terminal": True,
            }
        )
    for key in (
        "terminal_discovery",
        "terminal",
        "shard_index",
        "shard_id",
        "shard_count",
        "workflow_benchmark",
        "workflow_benchmark_name",
        "benchmark",
        "timeout_sec",
        "artifact_name",
        "job_url",
        "job_database_id",
        "job_conclusion",
        "run_id",
        "run_database_id",
        "workflow_run_id",
        "run_url",
        "branch",
        "head_branch",
        "ref",
        "head_sha",
        "sha",
        "commit_sha",
        "workflow",
        "dispatch_workflow",
        "workflow_name",
        "milestone",
        "collection_run_id",
        "issue",
        "issue_id",
    ):
        if key in container and key not in inherited:
            inherited[key] = container[key]
    if "benchmark" not in inherited:
        inherited["benchmark"] = container.get("workflow_benchmark_name") or container.get("workflow_benchmark")
    inherited.update(record)
    return inherited


def extract_outcome_sources(
    path: Path,
    expected_source: Optional[ExpectedSourceIdentity] = None,
) -> list[OutcomeSource]:
    label = repo_relative(path)
    data = load_json_object(path)
    if data.get("schema_name") == OUTCOME_SCHEMA:
        return [OutcomeSource(path=path, label=label, outcome=data)]

    nested_key = None
    nested_records: list[Any] | None = None
    for key in NESTED_OUTCOME_KEYS:
        value = data.get(key)
        if isinstance(value, list):
            nested_key = key
            nested_records = value
            break

    benchmark_container = data.get("schema_name") in BENCHMARK_OUTCOME_SCHEMAS
    terminal_discovery_container = data.get("terminal_discovery") is True
    if nested_key is not None and nested_records is not None:
        if terminal_discovery_container and not benchmark_container:
            expected = ", ".join(sorted(BENCHMARK_OUTCOME_SCHEMAS))
            raise CollectionError(
                f"{label}: unrecognized nested terminal_discovery container schema_name "
                f"{data.get('schema_name')!r}; expected schema_name one of {expected} before inheriting "
                "per-size outcomes"
            )
        if benchmark_container or terminal_discovery_container:
            if expected_source is not None:
                validate_present_source_identity(data, label, expected_source)
            sources: list[OutcomeSource] = []
            for offset, raw_record in enumerate(nested_records):
                record_label = f"{label}.{nested_key}[{offset}]"
                record = object_at(raw_record, record_label)
                outcome = inherit_benchmark_outcome_defaults(data, record)
                sources.append(
                    OutcomeSource(
                        path=path,
                        label=record_label,
                        outcome=outcome,
                        container=data,
                        container_label=label,
                    )
                )
            return sources

    if outcome_like_path(path):
        expected = ", ".join(sorted(BENCHMARK_OUTCOME_SCHEMAS | {OUTCOME_SCHEMA}))
        raise CollectionError(
            f"{label}: terminal discovery outcome JSON must use schema_name one of {expected} "
            f"or contain nested per-size outcomes under one of {NESTED_OUTCOME_KEYS}"
        )
    return []


def collect_outcome_sources(
    roots: Sequence[Path],
    expected_source: Optional[ExpectedSourceIdentity] = None,
) -> list[OutcomeSource]:
    paths = candidate_outcome_json_paths(roots)
    sources: list[OutcomeSource] = []
    for path in paths:
        sources.extend(extract_outcome_sources(path, expected_source=expected_source))
    if not sources:
        roots_text = ", ".join(repo_relative(root) for root in roots)
        raise CollectionError(f"no terminal discovery outcome JSON records found under {roots_text}")
    return sources


def normalize_status(status: str, label: str) -> tuple[str, str, str, str]:
    normalized = status.strip().lower()
    if normalized in NON_TERMINAL_STATUSES:
        raise CollectionError(
            f"{label}.status {status!r} is non-terminal; terminal provenance requires completed/failed/"
            "timed_out/skipped/cancelled accounting only"
        )
    try:
        return STATUS_TO_TERMINAL_OUTCOME[normalized]
    except KeyError as exc:
        raise CollectionError(f"{label}.status has unsupported terminal value {status!r}") from exc


def require_requested_source_text(value: Any, field: str) -> str:
    text = str_or_none(value)
    if text is None:
        raise CollectionError(
            f"requested/top-level source identity missing {field}; {SOURCE_POLICY_NAME} requires a strict, "
            "non-empty requested identity for every terminal source-policy field"
        )
    return text


def require_requested_source_int(value: Any, field: str) -> int:
    if str_or_none(value) is None:
        raise CollectionError(
            f"requested/top-level source identity missing {field}; {SOURCE_POLICY_NAME} requires a strict, "
            "non-empty requested identity for every terminal source-policy field"
        )
    return as_int(value, f"requested_source_identity.{field}")


def expected_source_identity(
    *,
    run_id: int,
    head_branch: Optional[str],
    head_sha: Optional[str],
    dispatch_workflow: str,
    milestone: str,
    epoch: str,
    issue: int,
) -> ExpectedSourceIdentity:
    return ExpectedSourceIdentity(
        run_id=require_requested_source_int(run_id, "run_id"),
        branch=require_requested_source_text(head_branch, "branch"),
        head_sha=require_requested_source_text(head_sha, "head_sha"),
        workflow=require_requested_source_text(dispatch_workflow, "workflow"),
        milestone=require_requested_source_text(milestone, "milestone"),
        collection_run_id=require_requested_source_text(epoch, "collection_run_id"),
        issue=require_requested_source_int(issue, "issue"),
    )


def source_alias_values(raw: Mapping[str, Any], keys: Sequence[str]) -> list[tuple[str, str]]:
    values: list[tuple[str, str]] = []
    for key in keys:
        value = str_or_none(raw.get(key))
        if value is not None:
            values.append((key, value))
    return values


def require_source_text(
    raw: Mapping[str, Any],
    keys: Sequence[str],
    field: str,
    expected: Optional[str],
    label: str,
) -> None:
    expected_text = require_requested_source_text(expected, field)
    aliases = source_alias_values(raw, keys)
    if not aliases:
        raise CollectionError(
            f"{label}: source outcome metadata missing {field}; {SOURCE_POLICY_NAME} requires every "
            "terminal outcome record to identify the requested/top-level source identity"
        )
    for key, actual in aliases:
        if actual != expected_text:
            raise CollectionError(
                f"{label}: source outcome metadata conflict for {field} under {SOURCE_POLICY_NAME} "
                f"(alias {key}): source={actual!r} requested/top-level={expected_text!r}"
            )


def require_source_int(
    raw: Mapping[str, Any],
    keys: Sequence[str],
    field: str,
    expected: int,
    label: str,
) -> None:
    aliases = source_alias_values(raw, keys)
    if not aliases:
        raise CollectionError(
            f"{label}: source outcome metadata missing {field}; {SOURCE_POLICY_NAME} requires every "
            "terminal outcome record to identify the requested/top-level source identity"
        )
    for key, actual_text in aliases:
        actual = as_int(actual_text, f"{label}.{key}")
        if actual != expected:
            raise CollectionError(
                f"{label}: source outcome metadata conflict for {field} under {SOURCE_POLICY_NAME} "
                f"(alias {key}): source={actual} requested/top-level={expected}"
            )


def validate_present_source_identity(raw: Mapping[str, Any], label: str, expected: ExpectedSourceIdentity) -> None:
    """Reject non-empty source identity aliases that conflict with the requested run."""

    for field, keys in SOURCE_IDENTITY_INT_ALIASES:
        expected_value = require_requested_source_int(getattr(expected, field), field)
        for key, actual_text in source_alias_values(raw, keys):
            actual = as_int(actual_text, f"{label}.{key}")
            if actual != expected_value:
                raise CollectionError(
                    f"{label}: source outcome metadata conflict for {field} under {SOURCE_POLICY_NAME} "
                    f"(alias {key}): source={actual} requested/top-level={expected_value}"
                )

    for field, keys in SOURCE_IDENTITY_TEXT_ALIASES:
        expected_value = require_requested_source_text(getattr(expected, field), field)
        for key, actual in source_alias_values(raw, keys):
            if actual != expected_value:
                raise CollectionError(
                    f"{label}: source outcome metadata conflict for {field} under {SOURCE_POLICY_NAME} "
                    f"(alias {key}): source={actual!r} requested/top-level={expected_value!r}"
                )


def validate_source_identity(raw: Mapping[str, Any], label: str, expected: ExpectedSourceIdentity) -> None:
    require_source_int(raw, ("run_id", "run_database_id", "workflow_run_id"), "run_id", expected.run_id, label)
    require_source_text(raw, ("branch", "head_branch", "ref"), "branch", expected.branch, label)
    require_source_text(raw, ("head_sha", "sha", "commit_sha"), "head_sha", expected.head_sha, label)
    require_source_text(raw, ("workflow", "dispatch_workflow"), "workflow", expected.workflow, label)
    require_source_text(raw, ("milestone",), "milestone", expected.milestone, label)
    require_source_text(raw, ("collection_run_id",), "collection_run_id", expected.collection_run_id, label)
    require_source_int(raw, ("issue", "issue_id"), "issue", expected.issue, label)


def source_policy_report(
    *,
    expected: ExpectedSourceIdentity,
    raw_source_record_count: int,
    logical_source_record_count: int,
    raw_source_file_count: int,
) -> dict[str, Any]:
    return {
        "schema_name": SOURCE_POLICY_SCHEMA,
        "schema_version": SCHEMA_VERSION,
        "policy": SOURCE_POLICY_NAME,
        "policy_text": SOURCE_POLICY_TEXT,
        "single_run_identity_required": True,
        "reviewed_multi_source_schema": False,
        "multi_source_union_allowed": False,
        "required_identity_fields": list(SOURCE_POLICY_REQUIRED_FIELDS),
        "expected_source_identity": {
            "run_id": expected.run_id,
            "branch": expected.branch,
            "head_sha": expected.head_sha,
            "workflow": expected.workflow,
            "milestone": expected.milestone,
            "collection_run_id": expected.collection_run_id,
            "issue": expected.issue,
        },
        "raw_source_identity_count": 1,
        "raw_source_file_count": raw_source_file_count,
        "raw_source_record_count": raw_source_record_count,
        "logical_source_record_count": logical_source_record_count,
    }


def validate_outcome_against_plan(
    *,
    source: OutcomeSource,
    plan_entry: Mapping[str, Any],
    shard_by_plan: Mapping[int, int],
    expected_source: Optional[ExpectedSourceIdentity] = None,
) -> dict[str, Any]:
    outcome = source.outcome
    path = source.path
    label = source.label
    if outcome.get("schema_name") != OUTCOME_SCHEMA:
        raise CollectionError(f"{label}.schema_name must be {OUTCOME_SCHEMA!r}, got {outcome.get('schema_name')!r}")
    if outcome.get("terminal_discovery") is not True:
        raise CollectionError(f"{label}.terminal_discovery must be true")
    if outcome.get("terminal") is not True:
        raise CollectionError(f"{label}.terminal must be true for terminal provenance collection")
    if expected_source is not None:
        validate_source_identity(outcome, label, expected_source)

    plan_index = as_int(outcome.get("plan_index"), f"{label}.plan_index")
    expected_plan_index = as_int(plan_entry.get("plan_index"), f"plan[{plan_index}].plan_index")
    if plan_index != expected_plan_index:
        raise CollectionError(f"{label}.plan_index must be {expected_plan_index}, got {plan_index}")

    timeout_sec = as_int(outcome.get("timeout_sec"), f"{label}.timeout_sec")
    if timeout_sec != EXPECTED_TIMEOUT_SEC:
        raise CollectionError(f"{label}.timeout_sec must be {EXPECTED_TIMEOUT_SEC}, got {timeout_sec}")

    required_matches = (
        ("benchmark", "workflow_benchmark_name"),
        ("hardware_kernel_name", "hardware_kernel_name"),
        ("problem_size", "problem_size"),
        ("sizes", "sizes"),
    )
    for outcome_key, plan_key in required_matches:
        actual = str(outcome.get(outcome_key, ""))
        expected = str(plan_entry.get(plan_key, ""))
        if actual != expected:
            raise CollectionError(f"{label}.{outcome_key} must match plan {plan_key} ({actual!r} != {expected!r})")

    shard_index = as_int(outcome.get("shard_index"), f"{label}.shard_index")
    expected_shard = shard_by_plan[plan_index]
    if shard_index != expected_shard:
        raise CollectionError(f"{label}.shard_index must be {expected_shard}, got {shard_index}")

    expected_runnable_unit = f"mi300a-terminal-discovery-plan-{plan_index:04d}"
    runnable_unit = str_or_none(outcome.get("runnable_unit_id"))
    if runnable_unit != expected_runnable_unit:
        raise CollectionError(f"{label}.runnable_unit_id must be {expected_runnable_unit!r}, got {runnable_unit!r}")

    status = nonempty_str(outcome.get("status"), f"{label}.status")
    terminal_outcome, bucket, sim_result_state, default_detail = normalize_status(status, f"{label}")
    kernel_time_ms = optional_float(outcome.get("kernel_time_ms"), f"{label}.kernel_time_ms")
    if terminal_outcome == "success" and kernel_time_ms is None:
        raise CollectionError(f"{label}.kernel_time_ms is required for success terminal outcomes")

    return {
        "plan_index": plan_index,
        "status": status.strip().lower(),
        "terminal_outcome": terminal_outcome,
        "bucket": bucket,
        "sim_result_state": sim_result_state,
        "detail": str_or_none(outcome.get("detail")) or default_detail,
        "kernel_time_ms": kernel_time_ms,
        "exit_code": optional_int(outcome.get("exit_code"), f"{label}.exit_code"),
        "started_at": str_or_none(outcome.get("started_at")),
        "finished_at": str_or_none(outcome.get("finished_at")),
        "duration_sec": duration_seconds(outcome.get("started_at"), outcome.get("finished_at")),
        "path": path,
        "source_label": label,
        "container": source.container,
        "container_label": source.container_label,
        "raw": outcome,
    }


def outcome_source_shape(source: OutcomeSource) -> str:
    """Return the representation shape used for deterministic source preference."""

    return SOURCE_SHAPE_BENCHMARK_NESTED if source.container is not None else SOURCE_SHAPE_STANDALONE


def outcome_source_preference_key(source: OutcomeSource) -> tuple[int, str, str]:
    """Prefer benchmark-level nested records, then use stable source labels."""

    shape = outcome_source_shape(source)
    return (SOURCE_SHAPE_PREFERENCE[shape], repo_relative(source.path), source.label)


def _critical_raw_value(raw: Mapping[str, Any], key: str) -> Optional[str]:
    value = str_or_none(raw.get(key))
    return value


def terminal_outcome_conflict_signature(normalized: Mapping[str, Any]) -> dict[str, Any]:
    """Fields that must agree when two source records claim one plan_index.

    Artifact downloads can include the same logical terminal outcome twice: once
    inside a benchmark-level container and once as the legacy standalone per-plan
    JSON.  Those records may be collapsed only when the fields that drive
    terminal accounting and provenance agree after the normal plan/shard checks.
    Source path and representation shape are deliberately excluded because they
    are the dimensions this repair de-duplicates.
    """

    raw = object_at(normalized.get("raw"), "normalized.raw")
    signature: dict[str, Any] = {
        "plan_index": normalized["plan_index"],
        "status": normalized["status"],
        "terminal_outcome": normalized["terminal_outcome"],
        "bucket": normalized["bucket"],
        "sim_result_state": normalized["sim_result_state"],
        "detail": normalized["detail"],
        "kernel_time_ms": normalized["kernel_time_ms"],
        "exit_code": normalized["exit_code"],
        "started_at": normalized["started_at"],
        "finished_at": normalized["finished_at"],
        "duration_sec": normalized["duration_sec"],
        "schema_name": raw.get("schema_name"),
        "terminal_discovery": raw.get("terminal_discovery"),
        "terminal": raw.get("terminal"),
        "timeout_sec": as_int(raw.get("timeout_sec"), "duplicate.timeout_sec"),
        "shard_index": as_int(raw.get("shard_index"), "duplicate.shard_index"),
    }
    for key in (
        "shard_id",
        "shard_count",
        "matrix_row_id",
        "runnable_unit_id",
        "job_url",
        "job_database_id",
        "job_conclusion",
        "benchmark",
        "workflow_benchmark",
        "workflow_benchmark_name",
        "hardware_kernel_name",
        "problem_size",
        "hardware_problem_size",
        "sizes",
        "size_token",
        "size_label",
        "branch",
        "milestone",
        "collection_run_id",
        "issue",
    ):
        signature[key] = _critical_raw_value(raw, key)
    return signature


def signature_difference_keys(left: Mapping[str, Any], right: Mapping[str, Any]) -> list[str]:
    keys = sorted(set(left) | set(right))
    difference_keys: list[str] = []
    for key in keys:
        left_value = left.get(key)
        right_value = right.get(key)
        if left_value == right_value:
            continue
        # Optional provenance adornments can be present on only one artifact
        # shape.  They are conflicts only when both records make different
        # claims for the field.
        if left_value is None or right_value is None:
            continue
        difference_keys.append(key)
    return difference_keys


def deduplicate_outcome_sources(
    *,
    sources: Sequence[OutcomeSource],
    plan_entries: Sequence[Mapping[str, Any]],
    by_index: Mapping[int, Mapping[str, Any]],
    shard_by_plan: Mapping[int, int],
    expected_source: Optional[ExpectedSourceIdentity] = None,
) -> DeduplicatedOutcomeSources:
    """Validate and collapse duplicate terminal outcome representations.

    The exact provenance gate is over logical plan entries, not the raw number of
    JSON representations unpacked from downloaded artifacts.  Each source record
    is still validated against the plan before de-duplication so a bad
    standalone record cannot be hidden by a good benchmark-level record.
    """

    plan_count = len(plan_entries)
    expected_indices = set(range(1, plan_count + 1))
    raw_shape_counts: collections.Counter[str] = collections.Counter()
    grouped: dict[int, list[tuple[OutcomeSource, Mapping[str, Any], Mapping[str, Any]]]] = {}
    extra: set[int] = set()

    for source in sources:
        raw_shape_counts[outcome_source_shape(source)] += 1
        plan_index = as_int(source.outcome.get("plan_index"), f"{source.label}.plan_index")
        if plan_index < 1 or plan_index > plan_count:
            extra.add(plan_index)
            continue
        normalized = validate_outcome_against_plan(
            source=source,
            plan_entry=by_index[plan_index],
            shard_by_plan=shard_by_plan,
            expected_source=expected_source,
        )
        grouped.setdefault(plan_index, []).append((source, normalized, terminal_outcome_conflict_signature(normalized)))

    missing = expected_indices - set(grouped)
    if missing or extra or len(grouped) != plan_count:
        raise CollectionError(
            "terminal provenance requires exactly one outcome per 1416-entry plan entry "
            f"after source de-duplication (logical_observed={len(grouped)}, "
            f"missing={make_ranges(missing)}, extra={make_ranges(extra)})"
        )

    selected_by_plan: dict[int, OutcomeSource] = {}
    normalized_by_plan: dict[int, Mapping[str, Any]] = {}
    selected_shape_counts: collections.Counter[str] = collections.Counter()
    discarded_shape_counts: collections.Counter[str] = collections.Counter()
    raw_duplicate_plan_indices: list[int] = []

    for plan_index in range(1, plan_count + 1):
        records = grouped[plan_index]
        reference_source, _reference_normalized, reference_signature = records[0]
        for source, _normalized, signature in records[1:]:
            difference_keys = signature_difference_keys(reference_signature, signature)
            if difference_keys:
                raise CollectionError(
                    f"conflicting duplicate terminal outcome records for plan_index {plan_index}: "
                    f"{reference_source.label} and {source.label} disagree on "
                    f"terminal/provenance-critical fields {difference_keys}"
                )

        if len(records) > 1:
            raw_duplicate_plan_indices.append(plan_index)

        selected_source, selected_normalized, _signature = sorted(
            records, key=lambda item: outcome_source_preference_key(item[0])
        )[0]
        selected_by_plan[plan_index] = selected_source
        normalized_by_plan[plan_index] = selected_normalized
        selected_shape_counts[outcome_source_shape(selected_source)] += 1
        for source, _normalized, _signature in records:
            if source is not selected_source:
                discarded_shape_counts[outcome_source_shape(source)] += 1

    return DeduplicatedOutcomeSources(
        selected_by_plan=selected_by_plan,
        normalized_by_plan=normalized_by_plan,
        raw_source_record_count=len(sources),
        raw_source_file_count=len({source.path.resolve() for source in sources}),
        raw_shape_counts=dict(sorted(raw_shape_counts.items())),
        selected_shape_counts=dict(sorted(selected_shape_counts.items())),
        discarded_shape_counts=dict(sorted(discarded_shape_counts.items())),
        raw_duplicate_plan_indices=tuple(raw_duplicate_plan_indices),
    )


def outcome_source_deduplication_report(deduplicated: DeduplicatedOutcomeSources) -> dict[str, Any]:
    logical_count = len(deduplicated.selected_by_plan)
    raw_duplicate_count = len(deduplicated.raw_duplicate_plan_indices)
    return {
        "schema_name": OUTCOME_SOURCE_DEDUPLICATION_SCHEMA,
        "schema_version": SCHEMA_VERSION,
        "policy": OUTCOME_SOURCE_DEDUPLICATION_POLICY,
        "preferred_shape": SOURCE_SHAPE_BENCHMARK_NESTED,
        "fallback_shape": SOURCE_SHAPE_STANDALONE,
        "raw_source_file_count": deduplicated.raw_source_file_count,
        "raw_source_record_count": deduplicated.raw_source_record_count,
        "logical_record_count": logical_count,
        "unique_plan_index_count": logical_count,
        "raw_duplicate_plan_index_count": raw_duplicate_count,
        "raw_duplicate_plan_index_ranges": make_ranges(deduplicated.raw_duplicate_plan_indices),
        "deduplicated_source_record_count": deduplicated.raw_source_record_count - logical_count,
        "raw_shape_counts": dict(deduplicated.raw_shape_counts),
        "selected_shape_counts": dict(deduplicated.selected_shape_counts),
        "discarded_shape_counts": dict(deduplicated.discarded_shape_counts),
    }


# ---------------------------------------------------------------------------
# Provenance construction
# ---------------------------------------------------------------------------


def outcome_row(plan_entry: Mapping[str, Any], normalized: Mapping[str, Any]) -> dict[str, Any]:
    return {
        "benchmark": str(plan_entry.get("workflow_benchmark_name")),
        "candidate_size": str(plan_entry.get("sizes")),
        "size_label": str(plan_entry.get("size_label")),
        "terminal_outcome": normalized["terminal_outcome"],
        "sim_result_state": normalized["sim_result_state"],
        "exit_code": "" if normalized["exit_code"] is None else str(normalized["exit_code"]),
        "timeout_sec": str(EXPECTED_TIMEOUT_SEC),
        "detail": normalized["detail"],
        "discovery_plan_index": str(normalized["plan_index"]),
        "discovery_hardware_kernel_name": str(plan_entry.get("hardware_kernel_name")),
        "discovery_hardware_problem_size": str(plan_entry.get("hardware_problem_size")),
        "discovery_artifact_id": str(plan_entry.get("artifact_id")),
    }


def artifact_entry(plan_entry: Mapping[str, Any], normalized: Mapping[str, Any]) -> dict[str, Any]:
    path = normalized["path"]
    raw = normalized["raw"]
    container = normalized.get("container") or {}
    plan_index = normalized["plan_index"]
    artifact_name = (
        str_or_none(raw.get("artifact_name"))
        or str_or_none(container.get("artifact_name"))
        or str_or_none(container.get("name"))
        or path.parent.name
        or f"mi300a-terminal-discovery-plan-{plan_index}"
    )
    return {
        "name": artifact_name,
        "path": normalized.get("source_label") or repo_relative(path),
        "size_in_bytes": path.stat().st_size,
        "expired": False,
        "created_at": normalized["finished_at"] or normalized["started_at"],
        "updated_at": normalized["finished_at"] or normalized["started_at"],
        "digest": sha256_file(path),
        "plan_index": plan_index,
        "benchmark": str(plan_entry.get("hardware_kernel_name")),
        "problem_size": str(plan_entry.get("hardware_problem_size")),
        "artifact_id": str(plan_entry.get("artifact_id")),
    }


def terminal_job_entry(plan_index: int, shard_index: int, normalized: Mapping[str, Any], run_url: Optional[str]) -> dict[str, Any]:
    raw = normalized["raw"]
    # The finishable-manifest provenance schema still keys terminal job names and
    # the historical `shard` field to the legacy 236-entry problem-size shards.
    # Preserve that compatibility while also retaining the benchmark-grouped
    # shard index used by the current terminal-discovery matrix surface.
    legacy_shard = finishable.discovery_shard_for_plan_index(plan_index, EXPECTED_ENTRY_COUNT)
    entry: dict[str, Any] = {
        "plan_index": plan_index,
        "shard": legacy_shard,
        "benchmark_shard": shard_index,
        "name": finishable.expected_discovery_job_name(plan_index, EXPECTED_ENTRY_COUNT),
        "status": "completed",
        "conclusion": str_or_none(raw.get("job_conclusion")) or "success",
        "started_at": normalized["started_at"],
        "completed_at": normalized["finished_at"],
        "duration_sec": normalized["duration_sec"],
        "url": str_or_none(raw.get("job_url")) or (f"{run_url}/attempts/1" if run_url else None),
    }
    job_database_id = optional_int(raw.get("job_database_id"), f"outcome[{plan_index}].job_database_id")
    if job_database_id is not None:
        entry["database_id"] = job_database_id
    return entry


def accounting_record(
    plan_entry: Mapping[str, Any],
    normalized: Mapping[str, Any],
    shard_index: int,
    artifact: Mapping[str, Any],
) -> dict[str, Any]:
    plan_index = normalized["plan_index"]
    return {
        "plan_index": plan_index,
        "runnable_unit_id": f"mi300a-terminal-discovery-plan-{plan_index:04d}",
        "shard_index": shard_index,
        "terminal": True,
        "status": normalized["status"],
        "terminal_outcome": normalized["terminal_outcome"],
        "terminal_outcome_bucket": normalized["bucket"],
        "eligible_for_finishable_manifest": normalized["bucket"] == "completed_success",
        "benchmark": str(plan_entry.get("benchmark")),
        "hardware_kernel_name": str(plan_entry.get("hardware_kernel_name")),
        "problem_size": str(plan_entry.get("problem_size")),
        "timeout_sec": EXPECTED_TIMEOUT_SEC,
        "duration_sec": normalized["duration_sec"],
        "kernel_time_ms": normalized["kernel_time_ms"],
        "exit_code": normalized["exit_code"],
        "outcome_path": normalized.get("source_label") or repo_relative(normalized["path"]),
        "artifact_name": artifact.get("name"),
        "artifact_digest": artifact.get("digest"),
    }


def requested_bucket_document(bucket_sets: Mapping[str, set[int]], observed_at_utc: Optional[str]) -> dict[str, Any]:
    doc: dict[str, Any] = {
        "plan_entry_count": EXPECTED_ENTRY_COUNT,
        "terminal_mode": True,
        "terminal_provenance": True,
    }
    for bucket in finishable.PRIMARY_BUCKETS:
        doc[f"{bucket}_count"] = len(bucket_sets[bucket])
        doc[f"{bucket}_plan_index_ranges"] = make_ranges(bucket_sets[bucket])
    for bucket in finishable.NON_TERMINAL_SUBSTATUS_BUCKETS:
        doc[f"{bucket}_count"] = len(bucket_sets[bucket])
        doc[f"{bucket}_plan_index_ranges"] = make_ranges(bucket_sets[bucket])
    for bucket in finishable.DIAGNOSTIC_BUCKETS:
        doc[f"{bucket}_count"] = len(bucket_sets[bucket])
        doc[f"{bucket}_plan_index_ranges"] = make_ranges(bucket_sets[bucket])
    doc.update(
        {
            "no_results_invented_for_non_terminal_or_missing_rows": True,
            "bounded_non_terminal_snapshot": False,
            "observation_time_utc": observed_at_utc,
        }
    )
    return doc


def build_terminal_provenance(
    *,
    plan_path: Path,
    shard_manifest_path: Path,
    outcome_roots: Sequence[Path],
    run_id: int,
    run_url: Optional[str],
    head_branch: Optional[str],
    head_sha: Optional[str],
    created_at: Optional[str],
    started_at: Optional[str],
    updated_at: Optional[str],
    collected_at: Optional[str],
    workflow_name: str,
    actor: Optional[str],
    milestone: str = "",
    epoch: str = "",
    issue: int = 115,
    dispatch_workflow: str = ".github/workflows/mi300a_terminal_discovery.yml",
    collected_for: str = "terminal_mi300a_discovery_finishable_manifest_regeneration",
) -> tuple[dict[str, Any], dict[str, Any]]:
    plan, plan_entries, by_index = load_plan(plan_path)
    if len(plan_entries) != EXPECTED_ENTRY_COUNT:
        raise CollectionError(f"plan must contain {EXPECTED_ENTRY_COUNT} entries, got {len(plan_entries)}")
    shard_by_plan = load_shard_plan_index_map(shard_manifest_path, len(plan_entries))
    expected_source = expected_source_identity(
        run_id=run_id,
        head_branch=head_branch,
        head_sha=head_sha,
        dispatch_workflow=dispatch_workflow,
        milestone=milestone,
        epoch=epoch,
        issue=issue,
    )
    sources = collect_outcome_sources(outcome_roots, expected_source=expected_source)
    deduplicated_sources = deduplicate_outcome_sources(
        sources=sources,
        plan_entries=plan_entries,
        by_index=by_index,
        shard_by_plan=shard_by_plan,
        expected_source=expected_source,
    )
    deduplication_report = outcome_source_deduplication_report(deduplicated_sources)
    source_policy = source_policy_report(
        expected=expected_source,
        raw_source_record_count=deduplicated_sources.raw_source_record_count,
        logical_source_record_count=deduplication_report["logical_record_count"],
        raw_source_file_count=deduplicated_sources.raw_source_file_count,
    )

    expected_indices = set(range(1, len(plan_entries) + 1))

    bucket_sets: dict[str, set[int]] = {bucket: set() for bucket in (*finishable.PRIMARY_BUCKETS, *finishable.NON_TERMINAL_SUBSTATUS_BUCKETS, *finishable.DIAGNOSTIC_BUCKETS)}
    outcome_rows: list[dict[str, Any]] = []
    artifact_entries: list[dict[str, Any]] = []
    job_entries: list[dict[str, Any]] = []
    accounting_records: list[dict[str, Any]] = []
    status_counts: collections.Counter[str] = collections.Counter()
    terminal_outcome_counts: collections.Counter[str] = collections.Counter()
    job_conclusion_counts: collections.Counter[str] = collections.Counter()

    for plan_index in range(1, len(plan_entries) + 1):
        plan_entry = by_index[plan_index]
        normalized = deduplicated_sources.normalized_by_plan[plan_index]
        bucket = normalized["bucket"]
        bucket_sets[bucket].add(plan_index)
        status_counts[normalized["status"]] += 1
        terminal_outcome_counts[normalized["terminal_outcome"]] += 1
        row = outcome_row(plan_entry, normalized)
        artifact = artifact_entry(plan_entry, normalized)
        job = terminal_job_entry(plan_index, shard_by_plan[plan_index], normalized, run_url)
        job_conclusion_counts[str(job.get("conclusion") or "")] += 1
        outcome_rows.append(row)
        artifact_entries.append(artifact)
        job_entries.append(job)
        accounting_records.append(accounting_record(plan_entry, normalized, shard_by_plan[plan_index], artifact))

    terminal_union = set().union(*(bucket_sets[bucket] for bucket in finishable.TERMINAL_BUCKETS))
    if terminal_union != expected_indices:
        raise CollectionError(
            "terminal outcome buckets must account for every plan entry "
            f"(missing={make_ranges(expected_indices - terminal_union)})"
        )

    observed_at_utc = collected_at or updated_at
    requested = requested_bucket_document(bucket_sets, observed_at_utc)
    reconciliation = finishable.bucket_count_reconciliation(bucket_sets)
    outcome_ranges = {
        outcome: make_ranges(
            int(row["discovery_plan_index"])
            for row in outcome_rows
            if row["terminal_outcome"] == outcome
        )
        for outcome in sorted(terminal_outcome_counts)
    }
    terminal_bucket_ranges = {bucket: make_ranges(bucket_sets[bucket]) for bucket in finishable.TERMINAL_BUCKETS}

    run = {
        "database_id": run_id,
        "url": run_url,
        "workflow_name": workflow_name,
        "name": workflow_name,
        "display_title": workflow_name,
        "event": "workflow_dispatch",
        "head_branch": head_branch,
        "head_sha": head_sha,
        "status": "completed",
        "conclusion": "success",
        "created_at": created_at,
        "started_at": started_at or created_at,
        "updated_at": updated_at or collected_at,
        "attempt": 1,
        "non_terminal_snapshot": False,
        "terminal_discovery": True,
        "terminal_provenance": True,
        "observed_at_utc": observed_at_utc,
    }

    provenance = {
        "schema_name": PROVENANCE_SCHEMA,
        "schema_version": PROVENANCE_SCHEMA_VERSION,
        "purpose": "terminal_mi300a_problem_size_discovery_provenance",
        "milestone": milestone,
        "collection_run_id": epoch,
        "issue": issue,
        "branch": head_branch or EXPECTED_BRANCH,
        "collected_for": collected_for,
        "terminal_discovery": True,
        "source_policy": source_policy,
        "run": run,
        "dispatch": {
            "workflow": dispatch_workflow,
            "ref": head_branch,
            "actor": actor,
        },
        "plan_summary": {
            "artifact": repo_relative(plan_path),
            "schema_name": plan.get("schema_name"),
            "schema_version": plan.get("schema_version"),
            "entry_count": len(plan_entries),
            "timeout_sec": plan.get("timeout_sec"),
            "tier_counts": plan.get("tier_counts"),
        },
        "raw_mi300a_timing_coverage": plan.get("raw_mi300a_timing_coverage"),
        "workflow_summary": {
            "job_count_total": len(plan_entries),
            "non_discovery_job_count": 0,
            "discovery_job_count": len(plan_entries),
            "job_status_counts": {"completed": len(plan_entries)},
            "job_conclusion_counts": dict(sorted(job_conclusion_counts.items())),
            "discovery_job_status_counts": {"completed": len(plan_entries)},
            "discovery_job_conclusion_counts": dict(sorted(job_conclusion_counts.items())),
            "observation_time_utc": observed_at_utc,
            "terminal_mode": True,
        },
        "artifact_summary": {
            "source": "downloaded terminal discovery outcome artifacts",
            "total_count": len(artifact_entries),
            "logical_terminal_outcome_artifact_count": len(artifact_entries),
            "source_file_count": deduplicated_sources.raw_source_file_count,
            "source_outcome_record_count": deduplicated_sources.raw_source_record_count,
            "benchmark_result_artifact_count": len(artifact_entries),
            "plan_index_ranges": make_ranges(expected_indices),
            "outcome_source_deduplication": deduplication_report,
            "entries": artifact_entries,
        },
        "outcome_summary": {
            "source": "terminal discovery outcome JSON artifacts",
            "terminal_outcome_row_count": len(outcome_rows),
            "terminal_outcome_counts": dict(sorted(terminal_outcome_counts.items())),
            "terminal_outcome_plan_index_ranges": outcome_ranges,
            "rows": outcome_rows,
        },
        "requested_plan_outcome_buckets": requested,
        "terminal_outcome_accounting": {
            "schema_name": ACCOUNTING_SCHEMA,
            "schema_version": SCHEMA_VERSION,
            "terminal_mode": True,
            "plan_entry_count": len(plan_entries),
            "record_count": len(accounting_records),
            "timeout_sec": EXPECTED_TIMEOUT_SEC,
            "bucket_counts": zero_inclusive_counts(bucket_sets, finishable.TERMINAL_BUCKETS),
            "terminal_bucket_total": sum(len(bucket_sets[bucket]) for bucket in finishable.TERMINAL_BUCKETS),
            "non_terminal_count": 0,
            "missing_job_count": 0,
            "terminal_outcome_counts": dict(sorted(terminal_outcome_counts.items())),
            "status_counts": dict(sorted(status_counts.items())),
            "plan_index_ranges_by_bucket": terminal_bucket_ranges,
            "coverage": {
                "expected_plan_index_count": len(plan_entries),
                "record_count": len(accounting_records),
                "duplicate_plan_index_count": 0,
                "duplicate_plan_index_ranges": [],
                "missing_plan_index_count": 0,
                "missing_plan_index_ranges": [],
                "extra_plan_index_count": 0,
                "extra_plan_index_ranges": [],
            },
            "records": accounting_records,
        },
        "terminal_discovery_job_summary": {
            "source": "terminal discovery outcome JSON artifacts",
            "completed_job_count": len(job_entries),
            "all_discovery_job_status_counts": {"completed": len(job_entries)},
            "all_discovery_job_conclusion_counts": dict(sorted(job_conclusion_counts.items())),
            "observation_time_utc": observed_at_utc,
            "entries": job_entries,
        },
        "observation": {
            "observed_at_utc": observed_at_utc,
            "source": "terminal_discovery_outcome_artifact_collection",
            "non_terminal_snapshot": False,
            "terminal_mode": True,
        },
    }

    validation_report = validate_terminal_provenance_document(provenance, plan_entries, repo_relative(plan_path))
    validation_report["source_outcome_file_count"] = deduplicated_sources.raw_source_file_count
    validation_report["raw_source_outcome_record_count"] = deduplicated_sources.raw_source_record_count
    validation_report["source_outcome_record_count"] = deduplication_report["logical_record_count"]
    validation_report["logical_source_outcome_record_count"] = deduplication_report["logical_record_count"]
    validation_report["outcome_source_deduplication"] = deduplication_report
    validation_report["output_provenance"] = None
    return provenance, validation_report


# ---------------------------------------------------------------------------
# Provenance validation (also used by the standalone validator script)
# ---------------------------------------------------------------------------


def _record_indices(records: Sequence[Mapping[str, Any]], label: str) -> tuple[set[int], set[int]]:
    seen: set[int] = set()
    duplicates: set[int] = set()
    for offset, record in enumerate(records):
        plan_index = as_int(record.get("plan_index"), f"{label}.records[{offset}].plan_index")
        if plan_index in seen:
            duplicates.add(plan_index)
        seen.add(plan_index)
    return seen, duplicates


def validate_terminal_outcome_accounting(
    provenance: Mapping[str, Any],
    bucket_sets: Mapping[str, set[int]],
    plan_count: int,
    label: str,
) -> dict[str, Any]:
    accounting = object_at(provenance.get("terminal_outcome_accounting"), f"{label}.terminal_outcome_accounting")
    if accounting.get("schema_name") != ACCOUNTING_SCHEMA:
        raise CollectionError(
            f"{label}.terminal_outcome_accounting.schema_name must be {ACCOUNTING_SCHEMA!r}, "
            f"got {accounting.get('schema_name')!r}"
        )
    if accounting.get("terminal_mode") is not True:
        raise CollectionError(f"{label}.terminal_outcome_accounting.terminal_mode must be true")
    if as_int(accounting.get("plan_entry_count"), f"{label}.terminal_outcome_accounting.plan_entry_count") != plan_count:
        raise CollectionError(f"{label}.terminal_outcome_accounting.plan_entry_count must be {plan_count}")

    records = [
        object_at(record, f"{label}.terminal_outcome_accounting.records[{offset}]")
        for offset, record in enumerate(list_at(accounting.get("records"), f"{label}.terminal_outcome_accounting.records"))
    ]
    record_count = as_int(accounting.get("record_count"), f"{label}.terminal_outcome_accounting.record_count")
    if record_count != len(records):
        raise CollectionError(
            f"{label}.terminal_outcome_accounting.record_count must match records length "
            f"({record_count} != {len(records)})"
        )
    if record_count != plan_count:
        raise CollectionError(
            f"{label}.terminal_outcome_accounting must contain one record per 1416-entry plan entry "
            f"({record_count} != {plan_count})"
        )

    seen, duplicates = _record_indices(records, f"{label}.terminal_outcome_accounting")
    expected = set(range(1, plan_count + 1))
    missing = expected - seen
    extra = seen - expected
    if duplicates or missing or extra:
        raise CollectionError(
            f"{label}.terminal_outcome_accounting.records must cover the plan exactly once "
            f"(duplicates={make_ranges(duplicates)}, missing={make_ranges(missing)}, extra={make_ranges(extra)})"
        )

    observed_bucket_counts = {bucket: 0 for bucket in finishable.TERMINAL_BUCKETS}
    observed_bucket_sets = {bucket: set() for bucket in finishable.TERMINAL_BUCKETS}
    for offset, record in enumerate(records):
        record_label = f"{label}.terminal_outcome_accounting.records[{offset}]"
        if record.get("terminal") is not True:
            raise CollectionError(f"{record_label}.terminal must be true")
        plan_index = as_int(record.get("plan_index"), f"{record_label}.plan_index")
        bucket = nonempty_str(record.get("terminal_outcome_bucket"), f"{record_label}.terminal_outcome_bucket")
        if bucket not in observed_bucket_counts:
            raise CollectionError(
                f"{record_label}.terminal_outcome_bucket must be one of {finishable.TERMINAL_BUCKETS}, got {bucket!r}"
            )
        observed_bucket_counts[bucket] += 1
        observed_bucket_sets[bucket].add(plan_index)
        if plan_index not in bucket_sets[bucket]:
            raise CollectionError(
                f"{record_label}.terminal_outcome_bucket {bucket!r} conflicts with requested bucket ranges"
            )

    declared_counts = object_at(accounting.get("bucket_counts"), f"{label}.terminal_outcome_accounting.bucket_counts")
    normalized_declared_counts = {
        bucket: as_int(declared_counts.get(bucket), f"{label}.terminal_outcome_accounting.bucket_counts.{bucket}")
        for bucket in finishable.TERMINAL_BUCKETS
    }
    if normalized_declared_counts != observed_bucket_counts:
        raise CollectionError(
            f"{label}.terminal_outcome_accounting.bucket_counts must match records "
            f"(expected={observed_bucket_counts}, actual={normalized_declared_counts})"
        )
    for bucket in finishable.TERMINAL_BUCKETS:
        if observed_bucket_sets[bucket] != bucket_sets[bucket]:
            raise CollectionError(
                f"{label}.terminal_outcome_accounting.records for {bucket} must match requested ranges "
                f"(expected={make_ranges(bucket_sets[bucket])}, actual={make_ranges(observed_bucket_sets[bucket])})"
            )
    if as_int(accounting.get("terminal_bucket_total"), f"{label}.terminal_outcome_accounting.terminal_bucket_total") != plan_count:
        raise CollectionError(f"{label}.terminal_outcome_accounting.terminal_bucket_total must be {plan_count}")
    if as_int(accounting.get("non_terminal_count"), f"{label}.terminal_outcome_accounting.non_terminal_count") != 0:
        raise CollectionError(f"{label}.terminal_outcome_accounting.non_terminal_count must be 0")
    if as_int(accounting.get("missing_job_count"), f"{label}.terminal_outcome_accounting.missing_job_count") != 0:
        raise CollectionError(f"{label}.terminal_outcome_accounting.missing_job_count must be 0")

    return {
        "record_count": len(records),
        "bucket_counts": observed_bucket_counts,
        "plan_index_ranges_by_bucket": {bucket: make_ranges(observed_bucket_sets[bucket]) for bucket in finishable.TERMINAL_BUCKETS},
    }


def validate_source_policy_document(
    provenance: Mapping[str, Any],
    plan_count: int,
    label: str,
) -> dict[str, Any]:
    policy = object_at(provenance.get("source_policy"), f"{label}.source_policy")
    if policy.get("schema_name") != SOURCE_POLICY_SCHEMA:
        raise CollectionError(
            f"{label}.source_policy.schema_name must be {SOURCE_POLICY_SCHEMA!r}, "
            f"got {policy.get('schema_name')!r}"
        )
    if policy.get("policy") != SOURCE_POLICY_NAME:
        raise CollectionError(f"{label}.source_policy.policy must be {SOURCE_POLICY_NAME!r}")
    if policy.get("single_run_identity_required") is not True:
        raise CollectionError(f"{label}.source_policy.single_run_identity_required must be true")
    if policy.get("reviewed_multi_source_schema") is not False:
        raise CollectionError(f"{label}.source_policy.reviewed_multi_source_schema must be false")
    if policy.get("multi_source_union_allowed") is not False:
        raise CollectionError(f"{label}.source_policy.multi_source_union_allowed must be false")
    identity_count = as_int(policy.get("raw_source_identity_count"), f"{label}.source_policy.raw_source_identity_count")
    if identity_count != 1:
        raise CollectionError(
            f"{label}.source_policy.raw_source_identity_count must be 1 for single-run terminal provenance, "
            f"got {identity_count}"
        )
    logical_count = as_int(policy.get("logical_source_record_count"), f"{label}.source_policy.logical_source_record_count")
    if logical_count != plan_count:
        raise CollectionError(
            f"{label}.source_policy.logical_source_record_count must be {plan_count}, got {logical_count}"
        )

    expected_identity = object_at(policy.get("expected_source_identity"), f"{label}.source_policy.expected_source_identity")
    run = object_at(provenance.get("run"), f"{label}.run")
    dispatch = object_at(provenance.get("dispatch"), f"{label}.dispatch")
    comparisons: tuple[tuple[str, Any, Any], ...] = (
        ("run_id", expected_identity.get("run_id"), run.get("database_id")),
        ("branch", expected_identity.get("branch"), provenance.get("branch") or run.get("head_branch")),
        ("head_sha", expected_identity.get("head_sha"), run.get("head_sha")),
        ("workflow", expected_identity.get("workflow"), dispatch.get("workflow")),
        ("milestone", expected_identity.get("milestone"), provenance.get("milestone")),
        ("collection_run_id", expected_identity.get("collection_run_id"), provenance.get("collection_run_id")),
        ("issue", expected_identity.get("issue"), provenance.get("issue")),
    )
    for field, expected_value, top_level_value in comparisons:
        if field in ("run_id", "issue"):
            expected_int = as_int(expected_value, f"{label}.source_policy.expected_source_identity.{field}")
            top_level_int = as_int(top_level_value, f"{label}.{field}")
            if expected_int != top_level_int:
                raise CollectionError(
                    f"{label}.source_policy expected {field} conflicts with requested/top-level identity "
                    f"({expected_int} != {top_level_int})"
                )
            continue
        if field == "head_sha":
            expected_text = nonempty_str(
                expected_value,
                f"{label}.source_policy.expected_source_identity.{field}",
            )
            top_level_text = nonempty_str(top_level_value, f"{label}.run.head_sha")
        else:
            expected_text = str_or_none(expected_value)
            top_level_text = str_or_none(top_level_value)
        if expected_text != top_level_text:
            raise CollectionError(
                f"{label}.source_policy expected {field} conflicts with requested/top-level identity "
                f"({expected_text!r} != {top_level_text!r})"
            )
    expected_branch = str_or_none(expected_identity.get("branch"))
    for branch_label, branch_value in (
        ("branch", provenance.get("branch")),
        ("run.head_branch", run.get("head_branch")),
        ("dispatch.ref", dispatch.get("ref")),
    ):
        if str_or_none(branch_value) != expected_branch:
            raise CollectionError(
                f"{label}.{branch_label} conflicts with source_policy branch "
                f"({str_or_none(branch_value)!r} != {expected_branch!r})"
            )
    return {
        "policy": SOURCE_POLICY_NAME,
        "raw_source_identity_count": identity_count,
        "logical_source_record_count": logical_count,
    }


def validate_terminal_provenance_document(
    provenance: Mapping[str, Any],
    plan_entries: Sequence[Mapping[str, Any]],
    label: str,
) -> dict[str, Any]:
    if provenance.get("schema_name") != PROVENANCE_SCHEMA:
        raise CollectionError(f"{label}.schema_name must be {PROVENANCE_SCHEMA!r}, got {provenance.get('schema_name')!r}")
    run = object_at(provenance.get("run"), f"{label}.run")
    if run.get("status") != "completed":
        raise CollectionError(f"{label}.run.status must be 'completed' for terminal provenance")
    if run.get("conclusion") != "success":
        raise CollectionError(
            f"{label}.run.conclusion must be 'success' for terminal provenance; "
            "completed non-success/cancelled runs cannot be terminal provenance"
        )
    if run.get("non_terminal_snapshot") is not False:
        raise CollectionError(f"{label}.run.non_terminal_snapshot must be false for terminal provenance")
    source_policy_validation = validate_source_policy_document(provenance, len(plan_entries), label)

    requested = object_at(provenance.get("requested_plan_outcome_buckets"), f"{label}.requested_plan_outcome_buckets")
    if requested.get("terminal_mode") is not True:
        raise CollectionError(f"{label}.requested_plan_outcome_buckets.terminal_mode must be true")
    if requested.get("bounded_non_terminal_snapshot") not in (False, None):
        raise CollectionError(f"{label}.requested_plan_outcome_buckets.bounded_non_terminal_snapshot must be false")

    try:
        bucket_sets = finishable.parse_bucket_sets(provenance, len(plan_entries), label)
    except finishable.ManifestError as exc:
        raise CollectionError(str(exc)) from exc

    disallowed = {
        "non_terminal": bucket_sets["non_terminal"],
        "missing_job": bucket_sets["missing_job"],
        "missing_outcome_row": bucket_sets["missing_outcome_row"],
        "non_terminal_in_progress": bucket_sets["non_terminal_in_progress"],
        "non_terminal_queued": bucket_sets["non_terminal_queued"],
    }
    non_empty_disallowed = {bucket: make_ranges(values) for bucket, values in disallowed.items() if values}
    if non_empty_disallowed:
        raise CollectionError(
            "terminal provenance must not contain non-terminal or missing accounting buckets "
            f"({non_empty_disallowed})"
        )

    expected = set(range(1, len(plan_entries) + 1))
    terminal_indices = set().union(*(bucket_sets[bucket] for bucket in finishable.TERMINAL_BUCKETS))
    if terminal_indices != expected:
        raise CollectionError(
            "terminal provenance must account every plan entry as completed/failed/timed_out/skipped/cancelled "
            f"(missing={make_ranges(expected - terminal_indices)}, extra={make_ranges(terminal_indices - expected)})"
        )

    by_index = {finishable.as_int(entry.get("plan_index"), "plan.plan_index"): entry for entry in plan_entries}
    try:
        outcome_rows = finishable.parse_outcome_rows(provenance, by_index, bucket_sets, label)
        artifacts = finishable.parse_artifacts(provenance, by_index, bucket_sets, label)
        terminal_jobs = finishable.parse_terminal_jobs(provenance, by_index, bucket_sets, label)
    except finishable.ManifestError as exc:
        raise CollectionError(str(exc)) from exc

    if set(outcome_rows) != expected:
        raise CollectionError(
            "terminal outcome rows must include one row per 1416-entry plan entry "
            f"(missing={make_ranges(expected - set(outcome_rows))}, extra={make_ranges(set(outcome_rows) - expected)})"
        )
    if set(artifacts) != expected:
        raise CollectionError(
            "terminal artifact entries must include one entry per 1416-entry plan entry "
            f"(missing={make_ranges(expected - set(artifacts))}, extra={make_ranges(set(artifacts) - expected)})"
        )
    if set(terminal_jobs) != expected:
        raise CollectionError(
            "terminal job entries must include one entry per 1416-entry plan entry "
            f"(missing={make_ranges(expected - set(terminal_jobs))}, extra={make_ranges(set(terminal_jobs) - expected)})"
        )

    accounting_report = validate_terminal_outcome_accounting(provenance, bucket_sets, len(plan_entries), label)
    reconciliation = finishable.bucket_count_reconciliation(bucket_sets)
    if reconciliation["terminal_bucket_total"] != len(plan_entries):
        raise CollectionError(f"terminal_bucket_total must be {len(plan_entries)}")
    if reconciliation["primary_bucket_counts"].get("non_terminal") != 0 or reconciliation["primary_bucket_counts"].get("missing_job") != 0:
        raise CollectionError("terminal primary bucket counts must have zero non_terminal and missing_job")

    hotspot = outcome_rows.get(474)
    if hotspot is None:
        raise CollectionError("terminal provenance must include hotspot/48 outcome row at plan_index 474")

    return {
        "schema_name": VALIDATION_SCHEMA,
        "schema_version": SCHEMA_VERSION,
        "status": "pass",
        "terminal_mode": True,
        "plan_entry_count": len(plan_entries),
        "terminal_outcome_row_count": len(outcome_rows),
        "artifact_entry_count": len(artifacts),
        "terminal_job_entry_count": len(terminal_jobs),
        "terminal_outcome_accounting": accounting_report,
        "source_policy": source_policy_validation,
        "requested_bucket_count_reconciliation": reconciliation,
        "hotspot_48": {
            "plan_index": 474,
            "terminal_outcome": hotspot.get("terminal_outcome"),
            "terminal_outcome_bucket": next(bucket for bucket in finishable.TERMINAL_BUCKETS if 474 in bucket_sets[bucket]),
        },
    }


def validate_terminal_provenance_path(provenance_path: Path, plan_path: Path = DEFAULT_PLAN) -> dict[str, Any]:
    _plan, plan_entries, _by_index = load_plan(plan_path)
    provenance = load_json_object(provenance_path)
    report = validate_terminal_provenance_document(provenance, plan_entries, repo_relative(provenance_path))
    report["source_paths"] = {
        "problem_size_discovery_plan": repo_relative(plan_path),
        "terminal_discovery_provenance": repo_relative(provenance_path),
    }
    return report


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Collect terminal MI300A discovery outcome artifacts into compact provenance."
    )
    parser.add_argument("--plan", default=str(DEFAULT_PLAN), help="Path to mi300a_problem_size_discovery_plan.json")
    parser.add_argument(
        "--shard-manifest",
        default=str(DEFAULT_SHARD_MANIFEST),
        help="Path to mi300a_terminal_discovery_shard_manifest.json",
    )
    parser.add_argument(
        "--outcomes-root",
        action="append",
        default=None,
        help="Directory or file containing per-benchmark or per-plan terminal discovery outcome JSON artifacts (repeatable)",
    )
    parser.add_argument("--output", default=str(DEFAULT_OUTPUT), help="Output compact terminal provenance JSON path")
    parser.add_argument("--run-id", type=int, required=True, help="GitHub Actions run database id for the terminal discovery run")
    parser.add_argument("--run-url", default=None, help="GitHub Actions run URL")
    parser.add_argument("--head-branch", default=EXPECTED_BRANCH, help="Run head branch/ref (required non-empty source identity)")
    parser.add_argument("--head-sha", default=None, help="Run head SHA (required non-empty source identity)")
    parser.add_argument("--created-at", default=None, help="Run created_at timestamp")
    parser.add_argument("--started-at", default=None, help="Run started_at timestamp")
    parser.add_argument("--updated-at", default=None, help="Run updated_at timestamp")
    parser.add_argument("--collected-at", default=None, help="UTC timestamp for this collection/observation")
    parser.add_argument("--workflow-name", default="MI300A Terminal Discovery", help="Workflow name")
    parser.add_argument("--actor", default=None, help="Workflow actor/login")
    parser.add_argument("--milestone", default=None, help="Milestone label to record in collected provenance")
    parser.add_argument("--epoch", default=None, help="Epoch label to record in collected provenance")
    parser.add_argument("--issue", type=int, default=None, help="Issue id to record in collected provenance")
    parser.add_argument(
        "--dispatch-workflow",
        default=".github/workflows/mi300a_terminal_discovery.yml",
        help="Workflow path used for dispatch provenance",
    )
    parser.add_argument(
        "--collected-for",
        default="terminal_mi300a_discovery_finishable_manifest_regeneration",
        help="Purpose string to record in collected provenance",
    )
    return parser.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    args = parse_args(argv)
    outcome_roots = [repo_path(path) for path in (args.outcomes_root or [str(DEFAULT_OUTCOMES_ROOT)])]
    output_path = repo_path(args.output)
    try:
        provenance, report = build_terminal_provenance(
            plan_path=repo_path(args.plan),
            shard_manifest_path=repo_path(args.shard_manifest),
            outcome_roots=outcome_roots,
            run_id=args.run_id,
            run_url=args.run_url,
            head_branch=args.head_branch,
            head_sha=args.head_sha,
            created_at=args.created_at,
            started_at=args.started_at,
            updated_at=args.updated_at,
            collected_at=args.collected_at,
            workflow_name=args.workflow_name,
            actor=args.actor,
            milestone=args.milestone,
            epoch=args.epoch,
            issue=args.issue,
            dispatch_workflow=args.dispatch_workflow,
            collected_for=args.collected_for,
        )
    except CollectionError as exc:
        print("FAIL: MI300A terminal discovery provenance collection rejected artifacts.", file=sys.stderr)
        print(f"  - {exc}", file=sys.stderr)
        return 1

    write_json(output_path, provenance)
    report["output_provenance"] = repo_relative(output_path)
    print(json.dumps(report, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
