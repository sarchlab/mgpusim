#!/usr/bin/env python3
"""Validate the checked-in per-benchmark MI300A terminal discovery surface.

The validator re-derives the deterministic shard manifest and matrix JSON files
from ``mi300a_problem_size_discovery_plan.json``, compares checked-in artifacts
against that regeneration, and performs focused coverage checks: all 1416 source
plan entries appear exactly once as per-size attempts, visible matrix ``include``
rows are grouped one per workflow benchmark, each benchmark row preserves its
ordered per-size attempt metadata, no shard exceeds the configured per-attempt
bound, hotspot/problem-size 48 remains at plan_index 474, and every attempt
preserves timeout_sec=3600.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
from pathlib import Path
from typing import Any, Mapping, Optional, Sequence

import generate_mi300a_terminal_discovery_shards as generator


VALIDATION_SCHEMA = "mgpusim.mi300a_terminal_discovery_benchmark_shard_validation"
SCHEMA_VERSION = 1
PROBLEM_SIZE_LEVEL_ROW_KEYS = frozenset(
    {
        "plan_index",
        "problem_size",
        "hardware_problem_size",
        "sizes",
        "size_token",
        "size_label",
        "runnable_unit_id",
        "attempt_name",
        "artifact_id",
        "plan_job_id",
        "shard_attempt_index",
        "benchmark_attempt_index",
    }
)


class ValidationError(Exception):
    """Raised when terminal discovery shard artifacts violate the contract."""


# ---------------------------------------------------------------------------
# Path/JSON helpers
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
    except generator.ShardManifestError as exc:
        raise ValidationError(str(exc)) from exc


def nonempty_str(value: Any, label: str) -> str:
    try:
        return generator.nonempty_str(value, label)
    except generator.ShardManifestError as exc:
        raise ValidationError(str(exc)) from exc


def str_or_none(value: Any) -> Optional[str]:
    return generator.str_or_none(value)


def make_ranges(values: set[int] | list[int]) -> list[str]:
    return generator.make_plan_index_ranges(values)


def unique_preserve_order(values: Sequence[str]) -> list[str]:
    return generator.unique_preserve_order(values)


def canonical_text(data: Mapping[str, Any]) -> str:
    return generator.json_text(data)


def canonical_sha256(data: Mapping[str, Any]) -> str:
    return hashlib.sha256(canonical_text(data).encode("utf-8")).hexdigest()


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
        for key in expected:
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
        raise ValidationError(f"{name} differs from regenerated terminal discovery benchmark shard surface")
    path, expected_value, actual_value = diff
    raise ValidationError(
        f"{name} differs from regenerated terminal discovery benchmark shard surface at {path}: "
        f"expected {compact_json(expected_value)}, got {compact_json(actual_value)}"
    )


def require_canonical_file(path: Path, expected: Mapping[str, Any], label: str) -> None:
    expected_text = canonical_text(expected)
    actual_text = path.read_text(encoding="utf-8")
    if actual_text != expected_text:
        raise ValidationError(
            f"{label} is not canonical JSON output; regenerate with "
            "scripts/generate_mi300a_terminal_discovery_shards.py"
        )


def validate_exact_matrix_file_set(
    shard_dir: Path,
    expected_matrices: Sequence[tuple[Path, Mapping[str, Any]]],
) -> None:
    expected_paths = {matrix_path.resolve() for matrix_path, _matrix in expected_matrices}
    actual_paths = {path.resolve() for path in shard_dir.glob("mi300a_terminal_discovery_shard_*_matrix.json")}
    missing = sorted(repo_relative(path) for path in expected_paths - actual_paths)
    extra = sorted(repo_relative(path) for path in actual_paths - expected_paths)
    if missing or extra:
        raise ValidationError(
            "terminal discovery shard directory must contain exactly the regenerated benchmark shard matrix files "
            f"(missing={missing}, extra={extra})"
        )


# ---------------------------------------------------------------------------
# Focused surface validation
# ---------------------------------------------------------------------------


def validate_manifest_header(
    manifest: Mapping[str, Any],
    *,
    plan_path: Path,
    shard_dir: Path,
    max_attempts_per_shard: int,
    benchmark_count: int,
) -> None:
    if manifest.get("schema_name") != generator.MANIFEST_SCHEMA:
        raise ValidationError(
            f"manifest.schema_name must be {generator.MANIFEST_SCHEMA!r}, got {manifest.get('schema_name')!r}"
        )
    if as_int(manifest.get("schema_version"), "manifest.schema_version") != generator.SCHEMA_VERSION:
        raise ValidationError(f"manifest.schema_version must be {generator.SCHEMA_VERSION}")
    if as_int(manifest.get("entry_count"), "manifest.entry_count") != generator.EXPECTED_ENTRY_COUNT:
        raise ValidationError(f"manifest.entry_count must be {generator.EXPECTED_ENTRY_COUNT}")
    for key in ("attempt_count", "per_size_attempt_count", "runnable_unit_count"):
        if as_int(manifest.get(key), f"manifest.{key}") != generator.EXPECTED_ENTRY_COUNT:
            raise ValidationError(f"manifest.{key} must be {generator.EXPECTED_ENTRY_COUNT}")
    if as_int(manifest.get("benchmark_count"), "manifest.benchmark_count") != benchmark_count:
        raise ValidationError(f"manifest.benchmark_count must be {benchmark_count}")
    if as_int(manifest.get("visible_matrix_row_count"), "manifest.visible_matrix_row_count") != benchmark_count:
        raise ValidationError(f"manifest.visible_matrix_row_count must be {benchmark_count}")
    if as_int(manifest.get("timeout_sec"), "manifest.timeout_sec") != generator.EXPECTED_TIMEOUT_SEC:
        raise ValidationError(f"manifest.timeout_sec must be {generator.EXPECTED_TIMEOUT_SEC}")
    if as_int(manifest.get("max_attempts_per_shard"), "manifest.max_attempts_per_shard") != max_attempts_per_shard:
        raise ValidationError(f"manifest.max_attempts_per_shard must be {max_attempts_per_shard}")

    source_plan = object_at(manifest.get("source_plan"), "manifest.source_plan")
    if source_plan.get("artifact") != repo_relative(plan_path):
        raise ValidationError(
            f"manifest.source_plan.artifact must be {repo_relative(plan_path)!r}, got {source_plan.get('artifact')!r}"
        )
    if as_int(source_plan.get("entry_count"), "manifest.source_plan.entry_count") != generator.EXPECTED_ENTRY_COUNT:
        raise ValidationError(f"manifest.source_plan.entry_count must be {generator.EXPECTED_ENTRY_COUNT}")
    if as_int(source_plan.get("timeout_sec"), "manifest.source_plan.timeout_sec") != generator.EXPECTED_TIMEOUT_SEC:
        raise ValidationError(f"manifest.source_plan.timeout_sec must be {generator.EXPECTED_TIMEOUT_SEC}")

    source_paths = object_at(manifest.get("source_paths"), "manifest.source_paths")
    if source_paths.get("plan") != repo_relative(plan_path):
        raise ValidationError(f"manifest.source_paths.plan must be {repo_relative(plan_path)!r}")
    if source_paths.get("shard_dir") != repo_relative(shard_dir):
        raise ValidationError(f"manifest.source_paths.shard_dir must be {repo_relative(shard_dir)!r}")

    generation_policy = object_at(manifest.get("generation_policy"), "manifest.generation_policy")
    expected_policy = {
        "ordering_policy": generator.ORDERING_POLICY,
        "sharding_policy": generator.SHARDING_POLICY,
        "matrix_row_policy": generator.MATRIX_ROW_POLICY,
        "attempt_policy": generator.ATTEMPT_POLICY,
    }
    for key, expected in expected_policy.items():
        if generation_policy.get(key) != expected:
            raise ValidationError(f"manifest.generation_policy.{key} must be {expected!r}")

    coverage = object_at(manifest.get("coverage"), "manifest.coverage")
    expected_zero_fields = (
        "duplicate_plan_index_count",
        "missing_plan_index_count",
        "extra_plan_index_count",
    )
    for key in expected_zero_fields:
        if as_int(coverage.get(key), f"manifest.coverage.{key}") != 0:
            raise ValidationError(f"manifest.coverage.{key} must be 0")
    for key in (
        "expected_plan_index_count",
        "represented_plan_index_count",
        "attempt_count",
        "per_size_attempt_count",
        "runnable_unit_count",
    ):
        if as_int(coverage.get(key), f"manifest.coverage.{key}") != generator.EXPECTED_ENTRY_COUNT:
            raise ValidationError(f"manifest.coverage.{key} must be {generator.EXPECTED_ENTRY_COUNT}")
    for key in ("benchmark_count", "visible_matrix_row_count"):
        if as_int(coverage.get(key), f"manifest.coverage.{key}") != benchmark_count:
            raise ValidationError(f"manifest.coverage.{key} must be {benchmark_count}")

    benchmark_coverage = object_at(manifest.get("benchmark_coverage"), "manifest.benchmark_coverage")
    if as_int(benchmark_coverage.get("source_workflow_benchmark_count"), "manifest.benchmark_coverage.source_workflow_benchmark_count") != benchmark_count:
        raise ValidationError(f"manifest.benchmark_coverage.source_workflow_benchmark_count must be {benchmark_count}")
    if as_int(benchmark_coverage.get("visible_matrix_row_count"), "manifest.benchmark_coverage.visible_matrix_row_count") != benchmark_count:
        raise ValidationError(f"manifest.benchmark_coverage.visible_matrix_row_count must be {benchmark_count}")
    if as_int(benchmark_coverage.get("duplicate_workflow_benchmark_count"), "manifest.benchmark_coverage.duplicate_workflow_benchmark_count") != 0:
        raise ValidationError("manifest.benchmark_coverage.duplicate_workflow_benchmark_count must be 0")


def plan_entry_by_index(plan_entries: Sequence[Mapping[str, Any]]) -> dict[int, Mapping[str, Any]]:
    by_index: dict[int, Mapping[str, Any]] = {}
    for entry in plan_entries:
        index = as_int(entry.get("plan_index"), "plan_entry.plan_index")
        by_index[index] = entry
    return by_index


def plan_groups_by_benchmark(plan_entries: Sequence[Mapping[str, Any]]) -> dict[str, list[Mapping[str, Any]]]:
    try:
        groups = generator.group_entries_by_workflow_benchmark(plan_entries)
    except generator.ShardManifestError as exc:
        raise ValidationError(str(exc)) from exc
    return {
        nonempty_str(group.get("workflow_benchmark"), "group.workflow_benchmark"): [
            object_at(entry, "group.entries[]") for entry in list_at(group.get("entries"), "group.entries")
        ]
        for group in groups
    }


def require_attempt_matches_plan(attempt: Mapping[str, Any], plan_entry: Mapping[str, Any], label: str) -> None:
    field_pairs = (
        ("plan_benchmark", "benchmark"),
        ("problem_size", "problem_size"),
        ("hardware_kernel_name", "hardware_kernel_name"),
        ("hardware_problem_size", "hardware_problem_size"),
        ("workflow_benchmark", "workflow_benchmark"),
        ("workflow_benchmark_name", "workflow_benchmark_name"),
        ("workflow_job", "workflow_job"),
        ("tier", "tier"),
        ("tier_name", "tier_name"),
        ("sizes", "sizes"),
        ("size_token", "size_token"),
        ("size_label", "size_label"),
        ("flag_template", "flag_template"),
        ("size_label_template", "size_label_template"),
        ("timeout_sec", "timeout_sec"),
        ("artifact_id", "artifact_id"),
        ("plan_job_id", "job_id"),
    )
    for attempt_key, plan_key in field_pairs:
        if attempt.get(attempt_key) != plan_entry.get(plan_key):
            raise ValidationError(
                f"{label}.{attempt_key} must match source plan {plan_key} "
                f"({attempt.get(attempt_key)!r} != {plan_entry.get(plan_key)!r})"
            )
    if attempt.get("benchmark") != plan_entry.get("workflow_benchmark"):
        raise ValidationError(
            f"{label}.benchmark must be the runnable workflow benchmark "
            f"{plan_entry.get('workflow_benchmark')!r}, got {attempt.get('benchmark')!r}"
        )
    for key in generator.OPTIONAL_RUNNER_FIELDS:
        expected = generator.str_or_none(plan_entry.get(key))
        actual = generator.str_or_none(attempt.get(key))
        if expected != actual:
            raise ValidationError(f"{label}.{key} must match source plan ({actual!r} != {expected!r})")
    for key in generator.OPTIONAL_MAPPING_FIELDS:
        expected = generator.str_or_none(plan_entry.get(key))
        actual = generator.str_or_none(attempt.get(key))
        if expected != actual:
            raise ValidationError(f"{label}.{key} must match source plan ({actual!r} != {expected!r})")


def require_row_matches_plan_group(row: Mapping[str, Any], group_entries: Sequence[Mapping[str, Any]], label: str) -> None:
    first = group_entries[0]
    benchmark = nonempty_str(first.get("workflow_benchmark"), f"{label}.workflow_benchmark")
    row_expected_pairs: tuple[tuple[str, Any], ...] = (
        ("benchmark", benchmark),
        ("workflow_benchmark", benchmark),
        ("workflow_benchmark_name", first.get("workflow_benchmark_name")),
        ("workflow_job", first.get("workflow_job")),
        ("tier", first.get("tier")),
        ("tier_name", first.get("tier_name")),
        ("flag_template", first.get("flag_template")),
        ("size_label_template", first.get("size_label_template")),
        ("timeout_sec", generator.EXPECTED_TIMEOUT_SEC),
        ("attempt_count", len(group_entries)),
        ("per_size_attempt_count", len(group_entries)),
    )
    for key, expected in row_expected_pairs:
        if row.get(key) != expected:
            raise ValidationError(f"{label}.{key} must be {expected!r}, got {row.get(key)!r}")

    plan_indices = [as_int(entry.get("plan_index"), f"{label}.plan_index") for entry in group_entries]
    if as_int(row.get("plan_index_start"), f"{label}.plan_index_start") != plan_indices[0]:
        raise ValidationError(f"{label}.plan_index_start must match the first grouped attempt")
    if as_int(row.get("plan_index_end"), f"{label}.plan_index_end") != plan_indices[-1]:
        raise ValidationError(f"{label}.plan_index_end must match the last grouped attempt")
    expected_ranges = make_ranges(plan_indices)
    if row.get("plan_index_ranges") != expected_ranges:
        raise ValidationError(f"{label}.plan_index_ranges must be {expected_ranges!r}")

    expected_problem_sizes = [str(entry.get("problem_size")) for entry in group_entries]
    if row.get("problem_sizes") != expected_problem_sizes:
        raise ValidationError(f"{label}.problem_sizes must preserve grouped source problem sizes")
    expected_unique_problem_sizes = unique_preserve_order(expected_problem_sizes)
    if row.get("unique_problem_sizes") != expected_unique_problem_sizes:
        raise ValidationError(f"{label}.unique_problem_sizes must be {expected_unique_problem_sizes!r}")
    expected_size_tokens = [str(entry.get("size_token")) for entry in group_entries]
    if row.get("size_tokens") != expected_size_tokens:
        raise ValidationError(f"{label}.size_tokens must preserve grouped source size tokens")
    expected_hardware_kernel_names = unique_preserve_order([str(entry.get("hardware_kernel_name")) for entry in group_entries])
    if row.get("hardware_kernel_names") != expected_hardware_kernel_names:
        raise ValidationError(f"{label}.hardware_kernel_names must be {expected_hardware_kernel_names!r}")
    expected_hardware_problem_sizes = [str(entry.get("hardware_problem_size")) for entry in group_entries]
    if row.get("hardware_problem_sizes") != expected_hardware_problem_sizes:
        raise ValidationError(f"{label}.hardware_problem_sizes must preserve grouped source hardware problem sizes")

    for key in generator.OPTIONAL_RUNNER_FIELDS:
        expected = generator.str_or_none(first.get(key))
        actual = generator.str_or_none(row.get(key))
        if expected != actual:
            raise ValidationError(f"{label}.{key} must match source plan ({actual!r} != {expected!r})")
    for key in generator.OPTIONAL_MAPPING_FIELDS:
        values = unique_preserve_order(
            [value for value in (generator.str_or_none(entry.get(key)) for entry in group_entries) if value is not None]
        )
        if len(values) == 0:
            if key in row or f"{key}s" in row:
                raise ValidationError(f"{label}.{key} must be absent when source plan has no values")
        elif len(values) == 1:
            if row.get(key) != values[0]:
                raise ValidationError(f"{label}.{key} must be {values[0]!r}")
        else:
            plural_key = f"{key}s"
            if row.get(plural_key) != values:
                raise ValidationError(f"{label}.{plural_key} must be {values!r}")


def validate_matrix_docs(
    matrices: Sequence[tuple[Path, Mapping[str, Any]]],
    *,
    manifest: Mapping[str, Any],
    manifest_path: Path,
    plan_path: Path,
    plan_entries: Sequence[Mapping[str, Any]],
    max_attempts_per_shard: int,
) -> dict[str, Any]:
    shards = [
        object_at(shard, f"manifest.shards[{offset}]")
        for offset, shard in enumerate(list_at(manifest.get("shards"), "manifest.shards"))
    ]
    if as_int(manifest.get("shard_count"), "manifest.shard_count") != len(shards):
        raise ValidationError("manifest.shard_count must match manifest.shards length")
    if len(shards) != len(matrices):
        raise ValidationError(f"manifest.shards length must be {len(matrices)}, got {len(shards)}")

    plan_by_index = plan_entry_by_index(plan_entries)
    groups_by_benchmark = plan_groups_by_benchmark(plan_entries)
    expected_benchmark_order = list(groups_by_benchmark)
    observed_indices: list[int] = []
    duplicate_indices: set[int] = set()
    seen_indices: set[int] = set()
    seen_unit_ids: set[str] = set()
    duplicate_unit_ids: set[str] = set()
    seen_row_ids: set[str] = set()
    duplicate_row_ids: set[str] = set()
    observed_benchmarks: list[str] = []
    hotspot_attempts: list[Mapping[str, Any]] = []
    shard_attempt_counts: dict[str, int] = {}
    shard_benchmark_row_counts: dict[str, int] = {}
    shard_ranges: dict[str, list[str]] = {}
    matrix_hash_expectations: list[tuple[int, Path, str]] = []

    for expected_shard_index, ((matrix_path, matrix), shard_summary) in enumerate(zip(matrices, shards), start=1):
        label = repo_relative(matrix_path)
        if matrix.get("schema_name") != generator.MATRIX_SCHEMA:
            raise ValidationError(f"{label}.schema_name must be {generator.MATRIX_SCHEMA!r}")
        if as_int(matrix.get("schema_version"), f"{label}.schema_version") != generator.SCHEMA_VERSION:
            raise ValidationError(f"{label}.schema_version must be {generator.SCHEMA_VERSION}")
        shard_index = as_int(matrix.get("shard_index"), f"{label}.shard_index")
        if shard_index != expected_shard_index:
            raise ValidationError(f"{label}.shard_index must be {expected_shard_index}, got {shard_index}")
        expected_id = generator.shard_id(shard_index)
        if matrix.get("shard_id") != expected_id:
            raise ValidationError(f"{label}.shard_id must be {expected_id!r}")
        if matrix.get("source_manifest") != repo_relative(manifest_path):
            raise ValidationError(f"{label}.source_manifest must be {repo_relative(manifest_path)!r}")
        if matrix.get("source_plan") != repo_relative(plan_path):
            raise ValidationError(f"{label}.source_plan must be {repo_relative(plan_path)!r}")
        if as_int(matrix.get("max_attempts_per_shard"), f"{label}.max_attempts_per_shard") != max_attempts_per_shard:
            raise ValidationError(f"{label}.max_attempts_per_shard must be {max_attempts_per_shard}")
        if as_int(matrix.get("timeout_sec"), f"{label}.timeout_sec") != generator.EXPECTED_TIMEOUT_SEC:
            raise ValidationError(f"{label}.timeout_sec must be {generator.EXPECTED_TIMEOUT_SEC}")
        for key, expected in (
            ("ordering_policy", generator.ORDERING_POLICY),
            ("sharding_policy", generator.SHARDING_POLICY),
            ("matrix_row_policy", generator.MATRIX_ROW_POLICY),
            ("attempt_policy", generator.ATTEMPT_POLICY),
        ):
            if matrix.get(key) != expected:
                raise ValidationError(f"{label}.{key} must be {expected!r}")

        include = [
            object_at(row, f"{label}.include[{offset}]")
            for offset, row in enumerate(list_at(matrix.get("include"), f"{label}.include"))
        ]
        matrix_attempts = [
            object_at(attempt, f"{label}.include[{row_offset}].attempts[{attempt_offset}]")
            for row_offset, row in enumerate(include)
            for attempt_offset, attempt in enumerate(list_at(row.get("attempts"), f"{label}.include[{row_offset}].attempts"))
        ]
        attempt_count = as_int(matrix.get("attempt_count"), f"{label}.attempt_count")
        if attempt_count != len(matrix_attempts):
            raise ValidationError(f"{label}.attempt_count must match nested attempts length")
        for key in ("entry_count", "per_size_attempt_count"):
            if as_int(matrix.get(key), f"{label}.{key}") != len(matrix_attempts):
                raise ValidationError(f"{label}.{key} must match nested attempts length")
        for key in ("benchmark_row_count", "visible_matrix_row_count"):
            if as_int(matrix.get(key), f"{label}.{key}") != len(include):
                raise ValidationError(f"{label}.{key} must match include length")
        if len(matrix_attempts) > max_attempts_per_shard:
            raise ValidationError(
                f"{label}.attempt_count {len(matrix_attempts)} exceeds max_attempts_per_shard {max_attempts_per_shard}"
            )

        summary_path = nonempty_str(shard_summary.get("matrix_path"), f"manifest.shards[{expected_shard_index - 1}].matrix_path")
        if summary_path != repo_relative(matrix_path):
            raise ValidationError(
                f"manifest.shards[{expected_shard_index - 1}].matrix_path must be {repo_relative(matrix_path)!r}, got {summary_path!r}"
            )

        matrix_plan_indices: list[int] = []
        expected_shard_attempt_index = 1
        shard_benchmarks: list[str] = []
        # workflow_benchmark blocks need not be physically contiguous in the
        # source plan (backprop_adjust_weights interleaved with
        # backprop), so plan_index ordering is asserted per benchmark row
        # rather than across the whole shard.
        for row_offset, row in enumerate(include, start=1):
            row_label = f"{label}.include[{row_offset - 1}]"
            if as_int(row.get("shard_index"), f"{row_label}.shard_index") != shard_index:
                raise ValidationError(f"{row_label}.shard_index must be {shard_index}")
            if row.get("shard_id") != expected_id:
                raise ValidationError(f"{row_label}.shard_id must be {expected_id!r}")
            if as_int(row.get("shard_count"), f"{row_label}.shard_count") != len(matrices):
                raise ValidationError(f"{row_label}.shard_count must be {len(matrices)}")
            if as_int(row.get("shard_entry_index"), f"{row_label}.shard_entry_index") != row_offset:
                raise ValidationError(f"{row_label}.shard_entry_index must be {row_offset}")
            forbidden_row_keys = sorted(PROBLEM_SIZE_LEVEL_ROW_KEYS.intersection(row))
            if forbidden_row_keys:
                raise ValidationError(
                    f"{row_label} must be one visible workflow-benchmark row, not a problem-size-level row; "
                    f"forbidden per-size keys={forbidden_row_keys}"
                )
            benchmark = nonempty_str(row.get("workflow_benchmark"), f"{row_label}.workflow_benchmark")
            shard_benchmarks.append(benchmark)
            observed_benchmarks.append(benchmark)
            if benchmark not in groups_by_benchmark:
                raise ValidationError(f"{row_label}.workflow_benchmark references unknown source benchmark {benchmark!r}")
            row_id = nonempty_str(row.get("matrix_row_id"), f"{row_label}.matrix_row_id")
            expected_row_id = generator.matrix_row_id(benchmark)
            if row_id != expected_row_id:
                raise ValidationError(f"{row_label}.matrix_row_id must be {expected_row_id!r}, got {row_id!r}")
            if row_id in seen_row_ids:
                duplicate_row_ids.add(row_id)
            seen_row_ids.add(row_id)
            expected_job_name = generator.expected_job_name(benchmark, len(groups_by_benchmark[benchmark]))
            if row.get("job_name") != expected_job_name:
                raise ValidationError(f"{row_label}.job_name must be {expected_job_name!r}")
            timeout_sec = as_int(row.get("timeout_sec"), f"{row_label}.timeout_sec")
            if timeout_sec != generator.EXPECTED_TIMEOUT_SEC:
                raise ValidationError(f"{row_label}.timeout_sec must be {generator.EXPECTED_TIMEOUT_SEC}, got {timeout_sec}")
            require_row_matches_plan_group(row, groups_by_benchmark[benchmark], row_label)

            attempts = [
                object_at(attempt, f"{row_label}.attempts[{offset}]")
                for offset, attempt in enumerate(list_at(row.get("attempts"), f"{row_label}.attempts"))
            ]
            if len(attempts) != len(groups_by_benchmark[benchmark]):
                raise ValidationError(f"{row_label}.attempts length must match source benchmark group")
            row_plan_indices: list[int] = []
            for attempt_offset, attempt in enumerate(attempts, start=1):
                attempt_label = f"{row_label}.attempts[{attempt_offset - 1}]"
                if as_int(attempt.get("shard_index"), f"{attempt_label}.shard_index") != shard_index:
                    raise ValidationError(f"{attempt_label}.shard_index must be {shard_index}")
                if attempt.get("shard_id") != expected_id:
                    raise ValidationError(f"{attempt_label}.shard_id must be {expected_id!r}")
                if as_int(attempt.get("shard_count"), f"{attempt_label}.shard_count") != len(matrices):
                    raise ValidationError(f"{attempt_label}.shard_count must be {len(matrices)}")
                if as_int(attempt.get("shard_attempt_index"), f"{attempt_label}.shard_attempt_index") != expected_shard_attempt_index:
                    raise ValidationError(f"{attempt_label}.shard_attempt_index must be {expected_shard_attempt_index}")
                expected_shard_attempt_index += 1
                if as_int(attempt.get("benchmark_entry_index"), f"{attempt_label}.benchmark_entry_index") != row_offset:
                    raise ValidationError(f"{attempt_label}.benchmark_entry_index must be {row_offset}")
                if as_int(attempt.get("benchmark_attempt_index"), f"{attempt_label}.benchmark_attempt_index") != attempt_offset:
                    raise ValidationError(f"{attempt_label}.benchmark_attempt_index must be {attempt_offset}")
                if attempt.get("matrix_row_id") != row_id:
                    raise ValidationError(f"{attempt_label}.matrix_row_id must be {row_id!r}")
                plan_index = as_int(attempt.get("plan_index"), f"{attempt_label}.plan_index")
                matrix_plan_indices.append(plan_index)
                row_plan_indices.append(plan_index)
                observed_indices.append(plan_index)
                if plan_index in seen_indices:
                    duplicate_indices.add(plan_index)
                seen_indices.add(plan_index)
                unit_id = nonempty_str(attempt.get("runnable_unit_id"), f"{attempt_label}.runnable_unit_id")
                if unit_id in seen_unit_ids:
                    duplicate_unit_ids.add(unit_id)
                seen_unit_ids.add(unit_id)
                expected_unit_id = generator.runnable_unit_id(plan_index)
                if unit_id != expected_unit_id:
                    raise ValidationError(f"{attempt_label}.runnable_unit_id must be {expected_unit_id!r}, got {unit_id!r}")
                expected_attempt_name = generator.expected_attempt_name(plan_index)
                if attempt.get("attempt_name") != expected_attempt_name:
                    raise ValidationError(f"{attempt_label}.attempt_name must be {expected_attempt_name!r}")
                attempt_timeout_sec = as_int(attempt.get("timeout_sec"), f"{attempt_label}.timeout_sec")
                if attempt_timeout_sec != generator.EXPECTED_TIMEOUT_SEC:
                    raise ValidationError(f"{attempt_label}.timeout_sec must be {generator.EXPECTED_TIMEOUT_SEC}, got {attempt_timeout_sec}")
                sizes = nonempty_str(attempt.get("sizes"), f"{attempt_label}.sizes")
                if len(sizes.split()) != 1:
                    raise ValidationError(f"{attempt_label}.sizes must contain exactly one problem-size token")
                if plan_index not in plan_by_index:
                    raise ValidationError(f"{attempt_label}.plan_index references unknown source plan entry {plan_index}")
                require_attempt_matches_plan(attempt, plan_by_index[plan_index], attempt_label)
                if (
                    str(attempt.get("hardware_kernel_name", "")).strip().lower() == generator.HOTSPOT_KERNEL
                    and str(attempt.get("hardware_problem_size", "")).strip() == generator.HOTSPOT_PROBLEM_SIZE
                ):
                    hotspot_attempts.append(attempt)

            if row_plan_indices != sorted(row_plan_indices):
                raise ValidationError(
                    f"{row_label}.attempts must be ordered by ascending plan_index"
                )
        if matrix_plan_indices:
            if as_int(matrix.get("plan_index_start"), f"{label}.plan_index_start") != matrix_plan_indices[0]:
                raise ValidationError(f"{label}.plan_index_start must match first attempt plan_index")
            if as_int(matrix.get("plan_index_end"), f"{label}.plan_index_end") != matrix_plan_indices[-1]:
                raise ValidationError(f"{label}.plan_index_end must match last attempt plan_index")
            expected_ranges = make_ranges(matrix_plan_indices)
            if matrix.get("plan_index_ranges") != expected_ranges:
                raise ValidationError(f"{label}.plan_index_ranges must be {expected_ranges!r}")
            if shard_summary.get("plan_index_ranges") != expected_ranges:
                raise ValidationError(
                    f"manifest.shards[{expected_shard_index - 1}].plan_index_ranges must be {expected_ranges!r}"
                )
            if as_int(shard_summary.get("entry_count"), f"manifest.shards[{expected_shard_index - 1}].entry_count") != len(matrix_plan_indices):
                raise ValidationError(f"manifest.shards[{expected_shard_index - 1}].entry_count must match shard attempts")
            for key in ("attempt_count", "per_size_attempt_count"):
                if as_int(shard_summary.get(key), f"manifest.shards[{expected_shard_index - 1}].{key}") != len(matrix_plan_indices):
                    raise ValidationError(f"manifest.shards[{expected_shard_index - 1}].{key} must match shard attempts")
            for key in ("benchmark_row_count", "visible_matrix_row_count"):
                if as_int(shard_summary.get(key), f"manifest.shards[{expected_shard_index - 1}].{key}") != len(include):
                    raise ValidationError(f"manifest.shards[{expected_shard_index - 1}].{key} must match shard matrix rows")
            if shard_summary.get("workflow_benchmarks") != shard_benchmarks:
                raise ValidationError(f"manifest.shards[{expected_shard_index - 1}].workflow_benchmarks must match matrix rows")
            if shard_summary.get("benchmark_start") != shard_benchmarks[0]:
                raise ValidationError(f"manifest.shards[{expected_shard_index - 1}].benchmark_start must match first row")
            if shard_summary.get("benchmark_end") != shard_benchmarks[-1]:
                raise ValidationError(f"manifest.shards[{expected_shard_index - 1}].benchmark_end must match last row")
        expected_sha = nonempty_str(
            shard_summary.get("matrix_sha256"), f"manifest.shards[{expected_shard_index - 1}].matrix_sha256"
        )
        matrix_hash_expectations.append((expected_shard_index, matrix_path, expected_sha))
        shard_attempt_counts[str(shard_index)] = len(matrix_plan_indices)
        shard_benchmark_row_counts[str(shard_index)] = len(include)
        shard_ranges[str(shard_index)] = make_ranges(matrix_plan_indices)

    expected_indices = set(plan_by_index)
    observed_set = set(observed_indices)
    missing = expected_indices - observed_set
    extra = observed_set - expected_indices
    if duplicate_indices or missing or extra:
        raise ValidationError(
            "terminal discovery benchmark shard matrices must cover all 1416 plan entries exactly once "
            f"(duplicates={make_ranges(duplicate_indices)}, missing={make_ranges(missing)}, extra={make_ranges(extra)})"
        )
    if duplicate_unit_ids:
        raise ValidationError(f"runnable_unit_id values must be unique, duplicates={sorted(duplicate_unit_ids)}")
    if duplicate_row_ids:
        raise ValidationError(f"matrix_row_id values must be unique, duplicates={sorted(duplicate_row_ids)}")
    if observed_benchmarks != expected_benchmark_order:
        raise ValidationError("visible matrix rows must follow source workflow_benchmark order exactly once")
    if len(hotspot_attempts) != 1:
        found = [as_int(attempt.get("plan_index"), "hotspot.plan_index") for attempt in hotspot_attempts]
        raise ValidationError(
            "terminal discovery benchmark shard surface must include exactly one hotspot/48 attempt at "
            f"plan_index {generator.HOTSPOT_PLAN_INDEX} (found_plan_indices={make_ranges(found)})"
        )
    hotspot = hotspot_attempts[0]
    if as_int(hotspot.get("plan_index"), "hotspot_48.plan_index") != generator.HOTSPOT_PLAN_INDEX:
        raise ValidationError(
            "terminal discovery benchmark shard surface must include exactly one hotspot/48 attempt at "
            f"plan_index {generator.HOTSPOT_PLAN_INDEX}"
        )
    if as_int(hotspot.get("timeout_sec"), "hotspot_48.timeout_sec") != generator.EXPECTED_TIMEOUT_SEC:
        raise ValidationError(f"hotspot/48 timeout_sec must be {generator.EXPECTED_TIMEOUT_SEC}")

    hotspot_summary = object_at(manifest.get("hotspot_48"), "manifest.hotspot_48")
    if as_int(hotspot_summary.get("plan_index"), "manifest.hotspot_48.plan_index") != generator.HOTSPOT_PLAN_INDEX:
        raise ValidationError(f"manifest.hotspot_48.plan_index must be {generator.HOTSPOT_PLAN_INDEX}")
    if as_int(hotspot_summary.get("timeout_sec"), "manifest.hotspot_48.timeout_sec") != generator.EXPECTED_TIMEOUT_SEC:
        raise ValidationError(f"manifest.hotspot_48.timeout_sec must be {generator.EXPECTED_TIMEOUT_SEC}")
    if hotspot_summary.get("matrix_row_id") != generator.matrix_row_id("hotspot"):
        raise ValidationError("manifest.hotspot_48.matrix_row_id must identify the hotspot benchmark row")

    for expected_shard_index, matrix_path, expected_sha in matrix_hash_expectations:
        actual_sha = hashlib.sha256(matrix_path.read_text(encoding="utf-8").encode("utf-8")).hexdigest()
        if expected_sha != actual_sha:
            raise ValidationError(
                f"manifest.shards[{expected_shard_index - 1}].matrix_sha256 must match {repo_relative(matrix_path)}"
            )

    return {
        "shard_attempt_counts": shard_attempt_counts,
        "shard_benchmark_row_counts": shard_benchmark_row_counts,
        "shard_plan_index_ranges": shard_ranges,
        "hotspot_48": dict(hotspot_summary),
    }


# ---------------------------------------------------------------------------
# Top-level validation
# ---------------------------------------------------------------------------


def validate_terminal_discovery_shards(
    plan_path: Path,
    manifest_path: Path,
    shard_dir: Path,
    *,
    max_attempts_per_shard: int,
) -> dict[str, Any]:
    try:
        _plan, plan_entries = generator.load_and_validate_plan(plan_path)
        benchmark_groups = generator.group_entries_by_workflow_benchmark(plan_entries)
        expected_manifest, expected_matrices = generator.build_terminal_discovery_shards(
            plan_path,
            manifest_path,
            shard_dir,
            max_attempts_per_shard=max_attempts_per_shard,
        )
    except generator.ShardManifestError as exc:
        raise ValidationError(str(exc)) from exc

    validate_exact_matrix_file_set(shard_dir, expected_matrices)
    actual_manifest = load_json_object(manifest_path)
    actual_matrices = [(matrix_path, load_json_object(matrix_path)) for matrix_path, _matrix in expected_matrices]

    validate_manifest_header(
        actual_manifest,
        plan_path=plan_path,
        shard_dir=shard_dir,
        max_attempts_per_shard=max_attempts_per_shard,
        benchmark_count=len(benchmark_groups),
    )
    focused = validate_matrix_docs(
        actual_matrices,
        manifest=actual_manifest,
        manifest_path=manifest_path,
        plan_path=plan_path,
        plan_entries=plan_entries,
        max_attempts_per_shard=max_attempts_per_shard,
    )

    require_equal("terminal discovery benchmark shard manifest", expected_manifest, actual_manifest)
    require_canonical_file(manifest_path, expected_manifest, "terminal discovery benchmark shard manifest")
    for (matrix_path, expected_matrix), (_actual_path, actual_matrix) in zip(expected_matrices, actual_matrices):
        require_equal(repo_relative(matrix_path), expected_matrix, actual_matrix)
        require_canonical_file(matrix_path, expected_matrix, repo_relative(matrix_path))
        # The manifest hash is over the canonical JSON text the generator writes.
        expected_summary = expected_manifest["shards"][as_int(expected_matrix["shard_index"], "matrix.shard_index") - 1]
        if expected_summary["matrix_sha256"] != canonical_sha256(expected_matrix):
            raise ValidationError(f"internal expected hash mismatch for {repo_relative(matrix_path)}")

    return {
        "schema_name": VALIDATION_SCHEMA,
        "schema_version": SCHEMA_VERSION,
        "validated": True,
        "manifest": repo_relative(manifest_path),
        "source_plan": repo_relative(plan_path),
        "entry_count": generator.EXPECTED_ENTRY_COUNT,
        "attempt_count": generator.EXPECTED_ENTRY_COUNT,
        "per_size_attempt_count": generator.EXPECTED_ENTRY_COUNT,
        "runnable_unit_count": generator.EXPECTED_ENTRY_COUNT,
        "benchmark_count": len(benchmark_groups),
        "visible_matrix_row_count": len(benchmark_groups),
        "problem_size_level_row_count": 0,
        "timeout_sec": generator.EXPECTED_TIMEOUT_SEC,
        "max_attempts_per_shard": max_attempts_per_shard,
        "shard_count": len(expected_matrices),
        "shard_attempt_count_max": max(focused["shard_attempt_counts"].values()),
        "shard_benchmark_row_count_max": max(focused["shard_benchmark_row_counts"].values()),
        "shard_attempt_counts": focused["shard_attempt_counts"],
        "shard_benchmark_row_counts": focused["shard_benchmark_row_counts"],
        "shard_plan_index_ranges": focused["shard_plan_index_ranges"],
        "hotspot_48": focused["hotspot_48"],
    }


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Validate deterministic per-benchmark MI300A terminal discovery shard manifests against the "
            "1416-entry problem-size discovery plan."
        )
    )
    parser.add_argument("--plan", default=str(generator.DEFAULT_PLAN), help="Path to mi300a_problem_size_discovery_plan.json")
    parser.add_argument(
        "--manifest",
        default=str(generator.DEFAULT_MANIFEST),
        help="Path to mi300a_terminal_discovery_shard_manifest.json",
    )
    parser.add_argument(
        "--shard-dir",
        default=str(generator.DEFAULT_GENERATED_DIR),
        help="Directory containing per-shard matrix JSON files",
    )
    parser.add_argument(
        "--max-attempts-per-shard",
        "--max-entries-per-shard",
        dest="max_attempts_per_shard",
        type=int,
        default=generator.DEFAULT_MAX_ATTEMPTS_PER_SHARD,
        help="Maximum per-size attempts per benchmark shard (default: 256); --max-entries-per-shard is a legacy alias",
    )
    return parser.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    args = parse_args(argv)
    plan_path = repo_path(args.plan)
    manifest_path = repo_path(args.manifest)
    shard_dir = repo_path(args.shard_dir)
    try:
        report = validate_terminal_discovery_shards(
            plan_path,
            manifest_path,
            shard_dir,
            max_attempts_per_shard=args.max_attempts_per_shard,
        )
    except ValidationError as exc:
        print("FAIL: MI300A terminal discovery benchmark shard validation rejected artifacts.", file=sys.stderr)
        print(f"  - {exc}", file=sys.stderr)
        return 1

    print(json.dumps(report, indent=2) + "\n", end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
