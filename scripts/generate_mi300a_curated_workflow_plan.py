#!/usr/bin/env python3
"""Generate the historical curated MI300A workflow-plan contract.

The curated-primary lane was superseded by the current linear finishable-size
benchmark workflow.  This tool is retained only for offline/historical audit of
the archived curated plan.  When invoked with the current default
``.github/workflows/benchmark.yml`` (which intentionally has no full-mode
matrices), it validates and re-emits the archived curated plan instead of
treating the live workflow as a source of full-mode entries.  Passing an
explicit archived or fixture workflow can still rebuild the plan from full-mode
``benchmark-tier1``/``benchmark-tier2`` matrices.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from collections import Counter, defaultdict
from dataclasses import dataclass
from pathlib import Path
from typing import Any


REPO_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_MANIFEST = REPO_ROOT / "benchmark-comparison" / "mi300a_evaluation_set.json"
DEFAULT_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "benchmark.yml"
DEFAULT_PLAN_OUTPUT = (
    REPO_ROOT / "benchmark-comparison" / "archive" / "mi300a_curated_workflow_plan.json"
)
DEFAULT_ARCHIVED_PLAN = DEFAULT_PLAN_OUTPUT

PLAN_SCHEMA_NAME = "mgpusim.mi300a_curated_workflow_plan"
PLAN_SCHEMA_VERSION = 1
ORDERING_POLICY = "tier1_then_tier2_preserve_manifest_order"
FULL_MODE_WORKFLOW_JOBS = ("benchmark-tier1", "benchmark-tier2")
ARCHIVAL_FALLBACK_POLICY = (
    "current_default_workflow_has_no_legacy_full_mode_matrix; "
    "validate_and_emit_archived_curated_workflow_plan"
)

# Explicit differences between manifest/CSV kernel names and workflow benchmark
# directory names.  All non-identity mappings must be listed here so future
# workflow changes cannot silently remap curated benchmarks.
SUPPORTED_ALIASES: dict[str, tuple[str, str]] = {
    "busspeeddownload": (
        "bus_speed_download",
        "Workflow uses the SHOC amd/samples/bus_speed_download directory name; "
        "the curated manifest and comparison CSV use the MI300A hardware kernel "
        "name busspeeddownload.",
    ),
    "busspeedreadback": (
        "bus_speed_readback",
        "Workflow uses the SHOC amd/samples/bus_speed_readback directory name; "
        "the curated manifest and comparison CSV use the MI300A hardware kernel "
        "name busspeedreadback.",
    ),
    "devicememory_write": (
        "device_memory_write",
        "Workflow uses the SHOC amd/samples/device_memory_write directory name; "
        "the curated manifest and comparison CSV use the MI300A hardware kernel "
        "name devicememory_write.",
    ),
    "sgemm_tiled": (
        "parboil_sgemm",
        "Workflow runs the Parboil SGEMM sample as amd/samples/parboil_sgemm; "
        "that sample emits the MI300A comparison kernel name sgemm_tiled.",
    ),
}

REQUIRED_MANIFEST_TOP_LEVEL_KEYS = (
    "schema_name",
    "schema_version",
    "set_id",
    "set_version",
    "benchmarks",
)
REQUIRED_MANIFEST_BENCHMARK_KEYS = ("kernel_name", "tier", "category")
REQUIRED_WORKFLOW_ENTRY_KEYS = (
    "benchmark",
    "tier",
    "sizes",
    "flag_template",
    "size_label_template",
)


class PlanError(Exception):
    """Raised when the manifest/workflow pair cannot produce a valid plan."""


@dataclass(frozen=True)
class ManifestBenchmark:
    kernel_name: str
    tier: int
    category: str
    manifest_index: int


@dataclass(frozen=True)
class WorkflowBenchmark:
    benchmark: str
    tier: str
    sizes: tuple[str, ...]
    flag_template: str
    size_label_template: str
    timeout_sec: int | None
    extra_flags: str | None
    gomemlimit: str | None
    gogc: str | None
    job_name: str
    line_no: int


# ---------------------------------------------------------------------------
# CLI and generic helpers
# ---------------------------------------------------------------------------


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Validate/re-emit the historical curated MI300A workflow-plan JSON. "
            "The current default benchmark workflow is linear and has no legacy "
            "full-mode matrices, so the default repository invocation uses the "
            "archived curated plan instead of validating live workflow entries."
        )
    )
    parser.add_argument(
        "--manifest",
        default=str(DEFAULT_MANIFEST),
        help="Path to mi300a_evaluation_set.json (default: benchmark-comparison/mi300a_evaluation_set.json).",
    )
    parser.add_argument(
        "--workflow",
        default=str(DEFAULT_WORKFLOW),
        help=(
            "Path to benchmark.yml. The default is the current linear workflow; "
            "because it intentionally has no legacy full-mode matrix, default "
            "repository invocations fall back to --archived-plan."
        ),
    )
    parser.add_argument(
        "--archived-plan",
        default=str(DEFAULT_ARCHIVED_PLAN),
        help=(
            "Checked-in archived curated workflow plan to validate/re-emit when "
            "the default linear workflow no longer contains the legacy full-mode "
            "matrix (default: benchmark-comparison/archive/"
            "mi300a_curated_workflow_plan.json)."
        ),
    )
    parser.add_argument(
        "--output",
        default=None,
        help=(
            "Optional path to write the deterministic plan JSON. The preferred "
            f"repository path is {repo_relative(DEFAULT_PLAN_OUTPUT)}."
        ),
    )
    return parser.parse_args(argv)


def repo_relative(path: Path) -> str:
    try:
        return path.resolve().relative_to(REPO_ROOT).as_posix()
    except ValueError:
        return path.as_posix()


def load_json_object(path: Path, label: str) -> dict[str, Any]:
    if not path.is_file():
        raise PlanError(f"{label} not found: {path}")
    try:
        loaded = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise PlanError(
            f"invalid {label} JSON: {path}: {exc.msg} at line {exc.lineno} column {exc.colno}"
        ) from exc
    if not isinstance(loaded, dict):
        raise PlanError(f"malformed {label}: top-level JSON value must be an object")
    return loaded


def require_mapping(value: Any, path: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        raise PlanError(f"malformed manifest: {path} must be an object")
    return value


def require_nonempty_string(value: Any, path: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise PlanError(f"malformed manifest: {path} must be a non-empty string")
    return value.strip()


def require_tier(value: Any, path: str) -> int:
    if isinstance(value, bool) or value not in (1, 2):
        raise PlanError(f"malformed manifest: {path} must be integer tier 1 or 2")
    return int(value)


def json_text(data: dict[str, Any]) -> str:
    return json.dumps(data, indent=2, sort_keys=False) + "\n"


# ---------------------------------------------------------------------------
# Manifest loading and validation
# ---------------------------------------------------------------------------


def load_manifest(path: Path) -> tuple[dict[str, Any], list[ManifestBenchmark]]:
    manifest = load_json_object(path, "manifest")

    missing_top = [key for key in REQUIRED_MANIFEST_TOP_LEVEL_KEYS if key not in manifest]
    if missing_top:
        raise PlanError(
            "malformed manifest: missing top-level keys: " + ", ".join(missing_top)
        )

    require_nonempty_string(manifest.get("schema_name"), "schema_name")
    schema_version = manifest.get("schema_version")
    if isinstance(schema_version, bool) or not isinstance(schema_version, int) or schema_version < 0:
        raise PlanError("malformed manifest: schema_version must be a non-negative integer")
    require_nonempty_string(manifest.get("set_id"), "set_id")
    require_nonempty_string(manifest.get("set_version"), "set_version")

    raw_benchmarks = manifest.get("benchmarks")
    if not isinstance(raw_benchmarks, list) or not raw_benchmarks:
        raise PlanError("malformed manifest: benchmarks must be a non-empty list")

    benchmarks: list[ManifestBenchmark] = []
    seen: Counter[str] = Counter()
    for idx, raw_entry in enumerate(raw_benchmarks, start=1):
        entry_path = f"benchmarks[{idx - 1}]"
        entry = require_mapping(raw_entry, entry_path)
        missing = [key for key in REQUIRED_MANIFEST_BENCHMARK_KEYS if key not in entry]
        if missing:
            raise PlanError(
                f"malformed manifest: {entry_path} missing keys: " + ", ".join(missing)
            )

        kernel_name = require_nonempty_string(entry.get("kernel_name"), f"{entry_path}.kernel_name")
        tier = require_tier(entry.get("tier"), f"{entry_path}.tier")
        category = require_nonempty_string(entry.get("category"), f"{entry_path}.category")
        seen[kernel_name] += 1
        benchmarks.append(
            ManifestBenchmark(
                kernel_name=kernel_name,
                tier=tier,
                category=category,
                manifest_index=idx,
            )
        )

    duplicates = sorted(name for name, count in seen.items() if count > 1)
    if duplicates:
        raise PlanError(
            "malformed manifest: duplicate selected kernel_name entries: "
            + ", ".join(duplicates)
        )

    return manifest, benchmarks


# ---------------------------------------------------------------------------
# Workflow parsing and validation
# ---------------------------------------------------------------------------


def load_workflow_entries(path: Path) -> list[WorkflowBenchmark]:
    if not path.is_file():
        raise PlanError(f"workflow not found: {path}")
    text = path.read_text(encoding="utf-8")
    lines = text.splitlines()

    entries: list[WorkflowBenchmark] = []
    for job_name in FULL_MODE_WORKFLOW_JOBS:
        start, end = workflow_job_line_range(lines, job_name)
        entries.extend(parse_workflow_job_entries(lines[start:end], job_name, line_offset=start))

    if not entries:
        raise PlanError(
            "workflow has no full-mode benchmark entries in jobs: "
            + ", ".join(FULL_MODE_WORKFLOW_JOBS)
            + "; this is expected for the current linear finishable-size workflow. "
            "Use the archived-plan fallback or pass an explicit legacy/fixture workflow "
            "when regenerating this historical curated plan."
        )
    return entries


def same_path(left: Path, right: Path) -> bool:
    return left.resolve() == right.resolve()


def workflow_error_allows_archival_fallback(error: PlanError) -> bool:
    message = str(error)
    return "workflow has no full-mode benchmark entries" in message or "workflow job not found:" in message


def should_use_archived_plan_fallback(
    manifest_path: Path,
    workflow_path: Path,
    archived_plan_path: Path,
    workflow_error: PlanError,
) -> bool:
    if not workflow_error_allows_archival_fallback(workflow_error):
        return False
    if not archived_plan_path.is_file():
        return False
    return same_path(manifest_path, DEFAULT_MANIFEST) and same_path(workflow_path, DEFAULT_WORKFLOW)


def load_and_validate_archived_plan(
    archived_plan_path: Path,
    manifest: dict[str, Any],
    manifest_benchmarks: list[ManifestBenchmark],
    manifest_path: Path,
    workflow_path: Path,
) -> dict[str, Any]:
    archived_plan = load_json_object(archived_plan_path, "archived curated workflow plan")
    expected_source_paths = {
        "manifest": repo_relative(manifest_path),
        "workflow": repo_relative(workflow_path),
    }
    if archived_plan.get("source_paths") != expected_source_paths:
        raise PlanError(
            "archived curated workflow plan source_paths do not match the requested "
            f"default inputs under {ARCHIVAL_FALLBACK_POLICY}: expected "
            f"{expected_source_paths!r}, got {archived_plan.get('source_paths')!r}"
        )
    validate_archived_plan(archived_plan, manifest, manifest_benchmarks)
    return archived_plan


def validate_archived_plan(
    plan: dict[str, Any],
    manifest: dict[str, Any],
    manifest_benchmarks: list[ManifestBenchmark],
) -> None:
    expected_manifest = {
        "schema_name": manifest["schema_name"],
        "schema_version": manifest["schema_version"],
        "set_id": manifest["set_id"],
        "set_version": manifest["set_version"],
    }
    required_top_level = (
        "schema_name",
        "schema_version",
        "manifest",
        "source_paths",
        "workflow_jobs",
        "ordering_policy",
        "entry_count",
        "tier_counts",
        "alias_count",
        "aliases",
        "benchmarks",
    )
    missing = [key for key in required_top_level if key not in plan]
    if missing:
        raise PlanError(
            "archived curated workflow plan missing top-level keys under "
            f"{ARCHIVAL_FALLBACK_POLICY}: " + ", ".join(missing)
        )
    if plan.get("schema_name") != PLAN_SCHEMA_NAME:
        raise PlanError(
            "archived curated workflow plan schema_name mismatch under "
            f"{ARCHIVAL_FALLBACK_POLICY}: expected {PLAN_SCHEMA_NAME!r}, "
            f"got {plan.get('schema_name')!r}"
        )
    if plan.get("schema_version") != PLAN_SCHEMA_VERSION:
        raise PlanError(
            "archived curated workflow plan schema_version mismatch under "
            f"{ARCHIVAL_FALLBACK_POLICY}: expected {PLAN_SCHEMA_VERSION!r}, "
            f"got {plan.get('schema_version')!r}"
        )
    if plan.get("manifest") != expected_manifest:
        raise PlanError(
            "archived curated workflow plan manifest metadata mismatch under "
            f"{ARCHIVAL_FALLBACK_POLICY}: expected {expected_manifest!r}, "
            f"got {plan.get('manifest')!r}"
        )
    if plan.get("workflow_jobs") != list(FULL_MODE_WORKFLOW_JOBS):
        raise PlanError(
            "archived curated workflow plan workflow_jobs mismatch under "
            f"{ARCHIVAL_FALLBACK_POLICY}: expected {list(FULL_MODE_WORKFLOW_JOBS)!r}, "
            f"got {plan.get('workflow_jobs')!r}"
        )
    if plan.get("ordering_policy") != ORDERING_POLICY:
        raise PlanError(
            "archived curated workflow plan ordering_policy mismatch under "
            f"{ARCHIVAL_FALLBACK_POLICY}: expected {ORDERING_POLICY!r}, "
            f"got {plan.get('ordering_policy')!r}"
        )

    entries = plan.get("benchmarks")
    if not isinstance(entries, list):
        raise PlanError("archived curated workflow plan benchmarks must be a list")
    aliases = plan.get("aliases")
    if not isinstance(aliases, list):
        raise PlanError("archived curated workflow plan aliases must be a list")
    if plan.get("entry_count") != len(entries):
        raise PlanError(
            "archived curated workflow plan entry_count mismatch under "
            f"{ARCHIVAL_FALLBACK_POLICY}: expected {len(entries)!r}, "
            f"got {plan.get('entry_count')!r}"
        )

    ordered_manifest = sorted(
        manifest_benchmarks,
        key=lambda entry: (entry.tier, entry.manifest_index),
    )
    if len(entries) != len(ordered_manifest):
        raise PlanError(
            "archived curated workflow plan benchmark count mismatch under "
            f"{ARCHIVAL_FALLBACK_POLICY}: expected {len(ordered_manifest)!r}, "
            f"got {len(entries)!r}"
        )

    tier_counts: Counter[int] = Counter()
    expected_aliases: list[dict[str, str]] = []
    seen_names: set[str] = set()
    for plan_index, (manifest_entry, raw_plan_entry) in enumerate(
        zip(ordered_manifest, entries),
        start=1,
    ):
        if not isinstance(raw_plan_entry, dict):
            raise PlanError(
                f"archived curated workflow plan benchmarks[{plan_index - 1}] must be an object"
            )
        validate_archived_plan_entry(raw_plan_entry, manifest_entry, plan_index)
        seen_names.add(manifest_entry.kernel_name)
        tier_counts[manifest_entry.tier] += 1

        workflow_name, alias_reason = resolve_workflow_name(manifest_entry.kernel_name)
        if alias_reason is not None:
            expected_aliases.append(
                {
                    "manifest_kernel_name": manifest_entry.kernel_name,
                    "workflow_benchmark_name": workflow_name,
                    "alias_reason": alias_reason,
                }
            )

    if len(seen_names) != len(manifest_benchmarks):
        raise PlanError("archived curated workflow plan has duplicate manifest kernel names")
    expected_tier_counts = {
        "1": tier_counts.get(1, 0),
        "2": tier_counts.get(2, 0),
    }
    if plan.get("tier_counts") != expected_tier_counts:
        raise PlanError(
            "archived curated workflow plan tier_counts mismatch under "
            f"{ARCHIVAL_FALLBACK_POLICY}: expected {expected_tier_counts!r}, "
            f"got {plan.get('tier_counts')!r}"
        )
    if plan.get("alias_count") != len(expected_aliases):
        raise PlanError(
            "archived curated workflow plan alias_count mismatch under "
            f"{ARCHIVAL_FALLBACK_POLICY}: expected {len(expected_aliases)!r}, "
            f"got {plan.get('alias_count')!r}"
        )
    if aliases != expected_aliases:
        raise PlanError(
            "archived curated workflow plan aliases mismatch under "
            f"{ARCHIVAL_FALLBACK_POLICY}: expected {expected_aliases!r}, got {aliases!r}"
        )


def validate_archived_plan_entry(
    entry: dict[str, Any],
    manifest_entry: ManifestBenchmark,
    plan_index: int,
) -> None:
    required_entry_keys = (
        "plan_index",
        "manifest_index",
        "manifest_kernel_name",
        "workflow_benchmark_name",
        "workflow_job",
        "workflow_line",
        "tier",
        "category",
        "sizes",
        "size_count",
        "flag_template",
        "size_label_template",
        "alias_reason",
    )
    missing = [key for key in required_entry_keys if key not in entry]
    if missing:
        raise PlanError(
            f"archived curated workflow plan entry {plan_index} missing keys: "
            + ", ".join(missing)
        )

    workflow_name, alias_reason = resolve_workflow_name(manifest_entry.kernel_name)
    expected_values = {
        "plan_index": plan_index,
        "manifest_index": manifest_entry.manifest_index,
        "manifest_kernel_name": manifest_entry.kernel_name,
        "workflow_benchmark_name": workflow_name,
        "tier": manifest_entry.tier,
        "category": manifest_entry.category,
        "alias_reason": alias_reason,
    }
    mismatches = [
        f"{key}: expected {expected!r}, got {entry.get(key)!r}"
        for key, expected in expected_values.items()
        if entry.get(key) != expected
    ]
    if mismatches:
        raise PlanError(
            f"archived curated workflow plan entry {plan_index} does not match manifest/alias contract: "
            + "; ".join(mismatches)
        )

    if entry.get("workflow_job") not in FULL_MODE_WORKFLOW_JOBS:
        raise PlanError(
            f"archived curated workflow plan entry {plan_index} has unsupported workflow_job "
            f"{entry.get('workflow_job')!r}"
        )
    workflow_line = entry.get("workflow_line")
    if isinstance(workflow_line, bool) or not isinstance(workflow_line, int) or workflow_line <= 0:
        raise PlanError(
            f"archived curated workflow plan entry {plan_index} has invalid workflow_line "
            f"{workflow_line!r}"
        )
    sizes = entry.get("sizes")
    if (
        not isinstance(sizes, list)
        or not sizes
        or any(not isinstance(size, str) or not size.strip() for size in sizes)
    ):
        raise PlanError(
            f"archived curated workflow plan entry {plan_index} sizes must be a non-empty string list"
        )
    if entry.get("size_count") != len(sizes):
        raise PlanError(
            f"archived curated workflow plan entry {plan_index} size_count mismatch: "
            f"expected {len(sizes)!r}, got {entry.get('size_count')!r}"
        )
    for string_key in ("flag_template", "size_label_template"):
        value = entry.get(string_key)
        if not isinstance(value, str) or not value.strip():
            raise PlanError(
                f"archived curated workflow plan entry {plan_index} has empty {string_key}"
            )
    if "{size}" not in entry["flag_template"]:
        raise PlanError(
            f"archived curated workflow plan entry {plan_index} flag_template lacks {{size}} placeholder"
        )
    if "timeout_sec" in entry:
        timeout_sec = entry["timeout_sec"]
        if isinstance(timeout_sec, bool) or not isinstance(timeout_sec, int) or timeout_sec <= 0:
            raise PlanError(
                f"archived curated workflow plan entry {plan_index} has invalid timeout_sec "
                f"{timeout_sec!r}"
            )
    for optional_key in ("extra_flags", "gomemlimit", "gogc"):
        if optional_key in entry:
            value = entry[optional_key]
            if not isinstance(value, str) or not value.strip():
                raise PlanError(
                    f"archived curated workflow plan entry {plan_index} has empty {optional_key}"
                )


def workflow_job_line_range(lines: list[str], job_name: str) -> tuple[int, int]:
    start: int | None = None
    job_header = re.compile(rf"^  {re.escape(job_name)}:\s*$")
    next_job_header = re.compile(r"^  [A-Za-z0-9_-]+:\s*$")

    for idx, line in enumerate(lines):
        if job_header.match(line):
            start = idx + 1
            break
    if start is None:
        raise PlanError(f"workflow job not found: {job_name}")

    end = len(lines)
    for idx in range(start, len(lines)):
        if next_job_header.match(lines[idx]):
            end = idx
            break
    return start, end


def parse_workflow_job_entries(
    lines: list[str], job_name: str, *, line_offset: int
) -> list[WorkflowBenchmark]:
    raw_entries: list[dict[str, Any]] = []
    current: dict[str, Any] | None = None
    current_indent: int | None = None
    entry_re = re.compile(r"^(?P<indent>\s*)-\s+benchmark:\s*(?P<value>.+?)\s*$")
    key_re = re.compile(r"^(?P<indent>\s+)(?P<key>[A-Za-z0-9_]+):\s*(?P<value>.+?)\s*$")

    for idx, line in enumerate(lines, start=1):
        benchmark_match = entry_re.match(line)
        if benchmark_match:
            if current is not None:
                raw_entries.append(current)
            current_indent = len(benchmark_match.group("indent"))
            current = {
                "benchmark": parse_yaml_scalar(benchmark_match.group("value")),
                "_line_no": line_offset + idx,
                "_job_name": job_name,
            }
            continue

        if current is None or current_indent is None:
            continue
        if not line.strip() or line.lstrip().startswith("#"):
            continue

        key_match = key_re.match(line)
        if not key_match:
            continue
        key_indent = len(key_match.group("indent"))
        if key_indent <= current_indent:
            continue
        current[key_match.group("key")] = parse_yaml_scalar(key_match.group("value"))

    if current is not None:
        raw_entries.append(current)

    return [validate_workflow_entry(raw_entry) for raw_entry in raw_entries]


def parse_yaml_scalar(raw_value: str) -> str:
    value = raw_value.strip()
    if len(value) >= 2 and value[0] == value[-1] and value[0] in {"'", '"'}:
        return value[1:-1]
    return value


def validate_workflow_entry(raw_entry: dict[str, Any]) -> WorkflowBenchmark:
    line_no = raw_entry["_line_no"]
    job_name = raw_entry["_job_name"]
    missing = [key for key in REQUIRED_WORKFLOW_ENTRY_KEYS if key not in raw_entry]
    if missing:
        raise PlanError(
            f"malformed workflow: benchmark entry at line {line_no} missing keys: "
            + ", ".join(missing)
        )

    benchmark = require_workflow_string(raw_entry["benchmark"], line_no, "benchmark")
    tier = require_workflow_string(raw_entry["tier"], line_no, "tier")
    if tier not in ("tier1", "tier2"):
        raise PlanError(
            f"malformed workflow: benchmark {benchmark!r} at line {line_no} has unsupported tier {tier!r}"
        )

    sizes_raw = require_workflow_string(raw_entry["sizes"], line_no, "sizes")
    sizes = tuple(sizes_raw.split())
    if not sizes:
        raise PlanError(
            f"malformed workflow: benchmark {benchmark!r} at line {line_no} has no sizes"
        )

    flag_template = require_workflow_string(raw_entry["flag_template"], line_no, "flag_template")
    size_label_template = require_workflow_string(
        raw_entry["size_label_template"], line_no, "size_label_template"
    )
    timeout_sec = parse_optional_timeout(raw_entry.get("timeout_sec"), benchmark, line_no)
    extra_flags = parse_optional_workflow_string(
        raw_entry.get("extra_flags"), benchmark, line_no, "extra_flags"
    )
    gomemlimit = parse_optional_workflow_string(
        raw_entry.get("gomemlimit"), benchmark, line_no, "gomemlimit"
    )
    gogc = parse_optional_workflow_string(raw_entry.get("gogc"), benchmark, line_no, "gogc")

    return WorkflowBenchmark(
        benchmark=benchmark,
        tier=tier,
        sizes=sizes,
        flag_template=flag_template,
        size_label_template=size_label_template,
        timeout_sec=timeout_sec,
        extra_flags=extra_flags,
        gomemlimit=gomemlimit,
        gogc=gogc,
        job_name=job_name,
        line_no=line_no,
    )


def require_workflow_string(value: Any, line_no: int, key: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise PlanError(
            f"malformed workflow: benchmark entry at line {line_no} has empty {key}"
        )
    return value.strip()


def parse_optional_workflow_string(
    value: Any, benchmark: str, line_no: int, key: str
) -> str | None:
    if value is None:
        return None
    if not isinstance(value, str) or not value.strip():
        raise PlanError(
            f"malformed workflow: benchmark {benchmark!r} at line {line_no} has empty {key}"
        )
    return value.strip()


def parse_optional_timeout(value: Any, benchmark: str, line_no: int) -> int | None:
    raw = parse_optional_workflow_string(value, benchmark, line_no, "timeout_sec")
    if raw is None:
        return None
    if not re.fullmatch(r"[0-9]+", raw):
        raise PlanError(
            f"malformed workflow: benchmark {benchmark!r} at line {line_no} has non-integer timeout_sec {raw!r}"
        )
    parsed = int(raw)
    if parsed <= 0:
        raise PlanError(
            f"malformed workflow: benchmark {benchmark!r} at line {line_no} has non-positive timeout_sec {raw!r}"
        )
    return parsed


# ---------------------------------------------------------------------------
# Plan construction
# ---------------------------------------------------------------------------


def build_plan(
    manifest: dict[str, Any],
    manifest_benchmarks: list[ManifestBenchmark],
    workflow_entries: list[WorkflowBenchmark],
    manifest_path: Path,
    workflow_path: Path,
) -> dict[str, Any]:
    workflow_by_name: dict[str, list[WorkflowBenchmark]] = defaultdict(list)
    for entry in workflow_entries:
        workflow_by_name[entry.benchmark].append(entry)

    resolved: dict[str, tuple[ManifestBenchmark, WorkflowBenchmark, str | None]] = {}
    errors: list[str] = []
    for manifest_entry in manifest_benchmarks:
        workflow_name, alias_reason = resolve_workflow_name(manifest_entry.kernel_name)
        matches = workflow_by_name.get(workflow_name, [])
        if not matches:
            errors.append(missing_mapping_error(manifest_entry.kernel_name, workflow_name, workflow_entries))
            continue
        if len(matches) > 1:
            locations = ", ".join(f"{entry.job_name}:line {entry.line_no}" for entry in matches)
            errors.append(
                f"selected kernel {manifest_entry.kernel_name!r} maps to duplicate workflow "
                f"benchmark {workflow_name!r} entries ({locations})"
            )
            continue

        workflow_entry = matches[0]
        expected_tier = f"tier{manifest_entry.tier}"
        if workflow_entry.tier != expected_tier:
            errors.append(
                f"selected kernel {manifest_entry.kernel_name!r} tier mismatch: manifest tier "
                f"{manifest_entry.tier} expects workflow tier {expected_tier!r}, but "
                f"{workflow_entry.benchmark!r} at line {workflow_entry.line_no} is "
                f"{workflow_entry.tier!r}"
            )
            continue

        resolved[manifest_entry.kernel_name] = (manifest_entry, workflow_entry, alias_reason)

    if errors:
        raise PlanError("curated workflow plan validation failed:\n- " + "\n- ".join(errors))

    ordered = sorted(
        (resolved[entry.kernel_name] for entry in manifest_benchmarks),
        key=lambda item: (item[0].tier, item[0].manifest_index),
    )

    plan_entries: list[dict[str, Any]] = []
    alias_entries: list[dict[str, str]] = []
    tier_counts: Counter[int] = Counter()
    for plan_index, (manifest_entry, workflow_entry, alias_reason) in enumerate(ordered, start=1):
        tier_counts[manifest_entry.tier] += 1
        plan_entry: dict[str, Any] = {
            "plan_index": plan_index,
            "manifest_index": manifest_entry.manifest_index,
            "manifest_kernel_name": manifest_entry.kernel_name,
            "workflow_benchmark_name": workflow_entry.benchmark,
            "workflow_job": workflow_entry.job_name,
            "workflow_line": workflow_entry.line_no,
            "tier": manifest_entry.tier,
            "category": manifest_entry.category,
            "sizes": list(workflow_entry.sizes),
            "size_count": len(workflow_entry.sizes),
            "flag_template": workflow_entry.flag_template,
            "size_label_template": workflow_entry.size_label_template,
            "alias_reason": alias_reason,
        }
        if workflow_entry.timeout_sec is not None:
            plan_entry["timeout_sec"] = workflow_entry.timeout_sec
        if workflow_entry.extra_flags is not None:
            plan_entry["extra_flags"] = workflow_entry.extra_flags
        if workflow_entry.gomemlimit is not None:
            plan_entry["gomemlimit"] = workflow_entry.gomemlimit
        if workflow_entry.gogc is not None:
            plan_entry["gogc"] = workflow_entry.gogc
        plan_entries.append(plan_entry)

        if alias_reason is not None:
            alias_entries.append(
                {
                    "manifest_kernel_name": manifest_entry.kernel_name,
                    "workflow_benchmark_name": workflow_entry.benchmark,
                    "alias_reason": alias_reason,
                }
            )

    return {
        "schema_name": PLAN_SCHEMA_NAME,
        "schema_version": PLAN_SCHEMA_VERSION,
        "manifest": {
            "schema_name": manifest["schema_name"],
            "schema_version": manifest["schema_version"],
            "set_id": manifest["set_id"],
            "set_version": manifest["set_version"],
        },
        "source_paths": {
            "manifest": repo_relative(manifest_path),
            "workflow": repo_relative(workflow_path),
        },
        "workflow_jobs": list(FULL_MODE_WORKFLOW_JOBS),
        "ordering_policy": ORDERING_POLICY,
        "entry_count": len(plan_entries),
        "tier_counts": {
            "1": tier_counts.get(1, 0),
            "2": tier_counts.get(2, 0),
        },
        "alias_count": len(alias_entries),
        "aliases": alias_entries,
        "benchmarks": plan_entries,
    }


def resolve_workflow_name(kernel_name: str) -> tuple[str, str | None]:
    alias = SUPPORTED_ALIASES.get(kernel_name)
    if alias is None:
        return kernel_name, None
    return alias


def normalized_name(name: str) -> str:
    return re.sub(r"[^A-Za-z0-9]", "", name).lower()


def missing_mapping_error(
    kernel_name: str, workflow_name: str, workflow_entries: list[WorkflowBenchmark]
) -> str:
    if kernel_name in SUPPORTED_ALIASES:
        return (
            f"selected kernel {kernel_name!r} uses supported alias target {workflow_name!r}, "
            "but that workflow benchmark is not present exactly once in the full-mode matrix"
        )

    normalized_kernel = normalized_name(kernel_name)
    normalized_candidates = sorted(
        {
            entry.benchmark
            for entry in workflow_entries
            if normalized_name(entry.benchmark) == normalized_kernel
        }
    )
    if normalized_candidates:
        return (
            f"selected kernel {kernel_name!r} has no exact workflow benchmark and matches "
            f"normalized workflow candidate(s) {normalized_candidates}; add an explicit "
            "supported alias before relying on this mapping"
        )

    return (
        f"selected kernel {kernel_name!r} has no matching workflow benchmark {workflow_name!r} "
        "and no supported alias rule"
    )


def generate_plan(
    manifest_path: Path,
    workflow_path: Path,
    archived_plan_path: Path = DEFAULT_ARCHIVED_PLAN,
) -> dict[str, Any]:
    manifest, manifest_benchmarks = load_manifest(manifest_path)
    try:
        workflow_entries = load_workflow_entries(workflow_path)
    except PlanError as exc:
        if should_use_archived_plan_fallback(
            manifest_path,
            workflow_path,
            archived_plan_path,
            exc,
        ):
            return load_and_validate_archived_plan(
                archived_plan_path,
                manifest,
                manifest_benchmarks,
                manifest_path,
                workflow_path,
            )
        raise
    return build_plan(manifest, manifest_benchmarks, workflow_entries, manifest_path, workflow_path)


# ---------------------------------------------------------------------------
# Entrypoint
# ---------------------------------------------------------------------------


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    manifest_path = Path(args.manifest)
    workflow_path = Path(args.workflow)
    archived_plan_path = Path(args.archived_plan)

    try:
        plan = generate_plan(manifest_path, workflow_path, archived_plan_path)
    except PlanError as exc:
        print(str(exc), file=sys.stderr)
        return 1

    rendered = json_text(plan)
    if args.output:
        output_path = Path(args.output)
        output_path.parent.mkdir(parents=True, exist_ok=True)
        output_path.write_text(rendered, encoding="utf-8")

    print(rendered, end="")
    return 0


if __name__ == "__main__":
    sys.exit(main())
