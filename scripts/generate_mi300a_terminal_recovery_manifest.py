#!/usr/bin/env python3
"""Generate the static MI300A terminal-discovery recovery manifest.

The manifest is a read-only recovery plan for the plan indices that were not
observed in replacement run 25111815292.  It is derived from the checked-in
1416-entry problem-size discovery plan and the incomplete-coverage report.
It does not run simulators, contact GitHub, dispatch workflows, collect new
outcomes, or regenerate finishable/Tier artifacts.
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
DEFAULT_COVERAGE_DOC = REPO_ROOT / "docs" / "m24_incomplete_terminal_discovery_coverage.md"
DEFAULT_OUTPUT = (
    REPO_ROOT
    / "benchmark-comparison"
    / "generated"
    / "mi300a_terminal_discovery_run_25111815292_recovery_manifest.json"
)

PLAN_SCHEMA = "mgpusim.mi300a_problem_size_discovery_plan"
MANIFEST_SCHEMA = "mgpusim.mi300a_terminal_discovery_recovery_manifest"
SCHEMA_VERSION = 1
EXPECTED_ENTRY_COUNT = 1416
EXPECTED_SOURCE_TIMEOUT_SEC = 3600
RUN_ID = "25111815292"
RUN_URL = "https://github.com/sarchlab/mgpusim-dev/actions/runs/25111815292"
RUN_HEAD_BRANCH = "e31-m23-execute-278-terminal-discovery-relaunch"
RUN_HEAD_SHA = "8ff0b0e1674c6778c3c697ff516abb9b2e4694c1"
PRIOR_ATTEMPT_TIMEOUT_SEC = 600
EXPECTED_MISSING_COUNT = 940
EXPECTED_OBSERVED_COUNT = 476
EXPECTED_PRIOR_STATUS_COUNTS = {"failed": 9, "success": 459, "timeout": 8}
COMPLETE_PRIOR_ROWS = {
    "gelu": "1228",
    "sssp": "1366-1383",
    "dmr": "1402-1416",
}


class RecoveryManifestError(Exception):
    """Raised when recovery manifest generation inputs are invalid."""


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


def file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def load_json_object(path: Path) -> Mapping[str, Any]:
    try:
        with path.open("r", encoding="utf-8") as f:
            data = json.load(f)
    except FileNotFoundError as exc:
        raise RecoveryManifestError(f"{repo_relative(path)}: file not found") from exc
    except json.JSONDecodeError as exc:
        raise RecoveryManifestError(f"{repo_relative(path)}: invalid JSON: {exc}") from exc
    if not isinstance(data, Mapping):
        raise RecoveryManifestError(f"{repo_relative(path)}: JSON root must be an object")
    return data


def as_int(value: Any, label: str) -> int:
    if isinstance(value, bool):
        raise RecoveryManifestError(f"{label} must be an integer, got boolean {value!r}")
    if isinstance(value, int):
        return value
    if isinstance(value, str) and value.strip().isdigit():
        return int(value.strip())
    raise RecoveryManifestError(f"{label} must be an integer, got {value!r}")


def nonempty_str(value: Any, label: str) -> str:
    text = str(value).strip() if value is not None else ""
    if not text:
        raise RecoveryManifestError(f"{label} must be a non-empty string")
    return text


def slugify_identifier(value: str) -> str:
    slug = re.sub(r"[^a-z0-9]+", "-", value.strip().lower()).strip("-")
    return slug or "benchmark"


# ---------------------------------------------------------------------------
# Plan-index ranges
# ---------------------------------------------------------------------------


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


def parse_plan_index_ranges(ranges: Sequence[str], label: str, max_plan_index: int) -> list[int]:
    values: list[int] = []
    seen: set[int] = set()
    for offset, range_text in enumerate(ranges, start=1):
        text = nonempty_str(range_text, f"{label}[{offset}]")
        match = re.fullmatch(r"(\d+)(?:-(\d+))?", text)
        if not match:
            raise RecoveryManifestError(f"{label}[{offset}] has invalid plan-index range {text!r}")
        start = int(match.group(1))
        end = int(match.group(2) or match.group(1))
        if start > end:
            raise RecoveryManifestError(f"{label}[{offset}] has descending range {text!r}")
        if start < 1 or end > max_plan_index:
            raise RecoveryManifestError(
                f"{label}[{offset}] range {text!r} outside plan-index bounds 1-{max_plan_index}"
            )
        for value in range(start, end + 1):
            if value in seen:
                raise RecoveryManifestError(f"{label} duplicates plan_index {value}")
            seen.add(value)
            values.append(value)
    return values


def extract_m24_missing_ranges(m24_doc: Path) -> list[str]:
    try:
        text = m24_doc.read_text(encoding="utf-8")
    except FileNotFoundError as exc:
        raise RecoveryManifestError(f"{repo_relative(m24_doc)}: file not found") from exc
    marker = "Exact missing plan-index ranges observed by the failed provenance gate"
    if marker not in text:
        raise RecoveryManifestError(f"{repo_relative(m24_doc)}: missing range marker")
    after_marker = text.split(marker, 1)[1]
    match = re.search(r"```text\s*(.*?)```", after_marker, flags=re.DOTALL)
    if not match:
        raise RecoveryManifestError(f"{repo_relative(m24_doc)}: missing fenced range block")
    ranges = [part.strip() for part in re.split(r",", match.group(1).replace("\n", " ")) if part.strip()]
    if make_plan_index_ranges(parse_plan_index_ranges(ranges, "m24_missing_plan_index_ranges", EXPECTED_ENTRY_COUNT)) != ranges:
        raise RecoveryManifestError(f"{repo_relative(m24_doc)}: missing ranges are not canonical")
    return ranges


# ---------------------------------------------------------------------------
# Manifest construction
# ---------------------------------------------------------------------------


def load_plan_entries(plan_path: Path) -> tuple[Mapping[str, Any], list[Mapping[str, Any]]]:
    plan = load_json_object(plan_path)
    schema_name = plan.get("schema_name")
    if schema_name != PLAN_SCHEMA:
        raise RecoveryManifestError(f"{repo_relative(plan_path)}: schema_name must be {PLAN_SCHEMA}, got {schema_name!r}")
    entries_value = plan.get("entries")
    if not isinstance(entries_value, list):
        raise RecoveryManifestError(f"{repo_relative(plan_path)}: entries must be a list")
    entries: list[Mapping[str, Any]] = []
    for offset, raw_entry in enumerate(entries_value, start=1):
        if not isinstance(raw_entry, Mapping):
            raise RecoveryManifestError(f"plan.entries[{offset}] must be an object")
        plan_index = as_int(raw_entry.get("plan_index"), f"plan.entries[{offset}].plan_index")
        if plan_index != offset:
            raise RecoveryManifestError(f"plan.entries[{offset}].plan_index must be {offset}, got {plan_index}")
        timeout_sec = as_int(raw_entry.get("timeout_sec"), f"plan.entries[{offset}].timeout_sec")
        if timeout_sec != EXPECTED_SOURCE_TIMEOUT_SEC:
            raise RecoveryManifestError(
                f"plan.entries[{offset}].timeout_sec must be {EXPECTED_SOURCE_TIMEOUT_SEC}, got {timeout_sec}"
            )
        entries.append(raw_entry)
    entry_count = as_int(plan.get("entry_count"), "plan.entry_count")
    if entry_count != EXPECTED_ENTRY_COUNT or len(entries) != EXPECTED_ENTRY_COUNT:
        raise RecoveryManifestError(
            f"{repo_relative(plan_path)}: expected {EXPECTED_ENTRY_COUNT} entries, got entry_count={entry_count} len={len(entries)}"
        )
    return plan, entries


def runnable_unit_id(plan_index: int) -> str:
    return f"mi300a-terminal-discovery-plan-{plan_index:04d}"


def recovery_row_id(workflow_benchmark: str) -> str:
    return f"mi300a-terminal-discovery-recovery-benchmark-{slugify_identifier(workflow_benchmark)}"


def build_recovery_rows(
    entries: Sequence[Mapping[str, Any]], missing_indices: set[int], observed_indices: set[int]
) -> list[dict[str, Any]]:
    by_benchmark: list[tuple[str, list[Mapping[str, Any]]]] = []
    row_by_benchmark: dict[str, list[Mapping[str, Any]]] = {}
    for entry in entries:
        workflow_benchmark = nonempty_str(entry.get("workflow_benchmark"), "plan_entry.workflow_benchmark")
        if workflow_benchmark not in row_by_benchmark:
            row_by_benchmark[workflow_benchmark] = []
            by_benchmark.append((workflow_benchmark, row_by_benchmark[workflow_benchmark]))
        row_by_benchmark[workflow_benchmark].append(entry)

    rows: list[dict[str, Any]] = []
    for workflow_benchmark, source_entries in by_benchmark:
        source_indices = [as_int(entry.get("plan_index"), "plan_entry.plan_index") for entry in source_entries]
        missing_entries = [entry for entry in source_entries if as_int(entry.get("plan_index"), "plan_entry.plan_index") in missing_indices]
        if not missing_entries:
            continue
        first = source_entries[0]
        row_missing_indices = [as_int(entry.get("plan_index"), "plan_entry.plan_index") for entry in missing_entries]
        row_observed_indices = [plan_index for plan_index in source_indices if plan_index in observed_indices]
        rows.append(
            {
                "recovery_row_index": len(rows) + 1,
                "matrix_row_id": recovery_row_id(workflow_benchmark),
                "job_name": f"Benchmark Recovery: {workflow_benchmark} ({len(missing_entries)} missing attempts)",
                "benchmark": workflow_benchmark,
                "workflow_benchmark": workflow_benchmark,
                "workflow_benchmark_name": nonempty_str(
                    first.get("workflow_benchmark_name"), f"{workflow_benchmark}.workflow_benchmark_name"
                ),
                "workflow_job": nonempty_str(first.get("workflow_job"), f"{workflow_benchmark}.workflow_job"),
                "tier": as_int(first.get("tier"), f"{workflow_benchmark}.tier"),
                "tier_name": nonempty_str(first.get("tier_name"), f"{workflow_benchmark}.tier_name"),
                "source_plan_lookup": "source_plan.artifact",
                "source_plan_entry_count": len(source_entries),
                "prior_observed_count": len(row_observed_indices),
                "recovery_attempt_count": len(missing_entries),
                "attempt_count": len(missing_entries),
                "source_plan_index_ranges": make_plan_index_ranges(source_indices),
                "prior_observed_plan_index_ranges": make_plan_index_ranges(row_observed_indices),
                "missing_plan_index_ranges": make_plan_index_ranges(row_missing_indices),
                "missing_plan_index_start": row_missing_indices[0],
                "missing_plan_index_end": row_missing_indices[-1],
                "first_runnable_unit_id": runnable_unit_id(row_missing_indices[0]),
                "last_runnable_unit_id": runnable_unit_id(row_missing_indices[-1]),
            }
        )
    return rows


def complete_prior_rows(entries: Sequence[Mapping[str, Any]], observed_indices: set[int]) -> list[dict[str, Any]]:
    by_benchmark: dict[str, list[int]] = {}
    for entry in entries:
        benchmark = nonempty_str(entry.get("workflow_benchmark"), "plan_entry.workflow_benchmark")
        plan_index = as_int(entry.get("plan_index"), "plan_entry.plan_index")
        by_benchmark.setdefault(benchmark, []).append(plan_index)

    complete_rows: list[dict[str, Any]] = []
    for benchmark, expected_ranges in COMPLETE_PRIOR_ROWS.items():
        row_indices = by_benchmark.get(benchmark)
        if row_indices is None:
            raise RecoveryManifestError(f"complete prior row {benchmark!r} is absent from the source plan")
        if make_plan_index_ranges(row_indices) != [expected_ranges]:
            raise RecoveryManifestError(
                f"complete prior row {benchmark!r} expected ranges {[expected_ranges]}, got {make_plan_index_ranges(row_indices)}"
            )
        if not set(row_indices).issubset(observed_indices):
            raise RecoveryManifestError(f"complete prior row {benchmark!r} is not fully observed in the complement")
        complete_rows.append(
            {
                "benchmark": benchmark,
                "plan_index_ranges": [expected_ranges],
                "observed_count": len(row_indices),
                "recovery_attempt_count": 0,
                "treatment": "prior_incomplete_evidence_only_not_recovery_scope",
            }
        )
    return complete_rows


def load_checked_in_missing_ranges(output_path: Path, m24_doc: Path) -> list[str]:
    """Load frozen missing ranges from the checked-in manifest for check mode."""
    manifest = load_json_object(output_path)
    if manifest.get("schema_name") != MANIFEST_SCHEMA:
        raise RecoveryManifestError(
            f"{repo_relative(output_path)}: schema_name must be {MANIFEST_SCHEMA}, "
            f"got {manifest.get('schema_name')!r}"
        )
    if as_int(manifest.get("schema_version"), "manifest.schema_version") != SCHEMA_VERSION:
        raise RecoveryManifestError(
            f"{repo_relative(output_path)}: schema_version must be {SCHEMA_VERSION}"
        )
    source_evidence = manifest.get("source_evidence")
    if not isinstance(source_evidence, Mapping):
        raise RecoveryManifestError(f"{repo_relative(output_path)}: source_evidence must be an object")
    expected_report = repo_relative(m24_doc)
    if source_evidence.get("m24_coverage_report") != expected_report:
        raise RecoveryManifestError(
            f"{repo_relative(output_path)}: source_evidence.m24_coverage_report must be {expected_report!r}"
        )
    ranges = manifest.get("missing_plan_index_ranges")
    if not isinstance(ranges, list):
        raise RecoveryManifestError(f"{repo_relative(output_path)}: missing_plan_index_ranges must be a list")
    range_strings = [
        nonempty_str(value, "manifest.missing_plan_index_ranges[]")
        for value in ranges
    ]
    parsed_ranges = parse_plan_index_ranges(
        range_strings, "manifest.missing_plan_index_ranges", EXPECTED_ENTRY_COUNT
    )
    if make_plan_index_ranges(parsed_ranges) != range_strings:
        raise RecoveryManifestError(f"{repo_relative(output_path)}: missing ranges are not canonical")
    return range_strings


def build_manifest(
    plan_path: Path,
    m24_doc: Path,
    missing_range_strings: Optional[Sequence[str]] = None,
) -> Mapping[str, Any]:
    plan, entries = load_plan_entries(plan_path)
    if missing_range_strings is None:
        missing_range_strings = extract_m24_missing_ranges(m24_doc)
    missing_values = parse_plan_index_ranges(missing_range_strings, "m24_missing_plan_index_ranges", EXPECTED_ENTRY_COUNT)
    missing_indices = set(missing_values)
    if len(missing_indices) != EXPECTED_MISSING_COUNT:
        raise RecoveryManifestError(f"expected {EXPECTED_MISSING_COUNT} missing plan indices, got {len(missing_indices)}")
    all_indices = set(range(1, EXPECTED_ENTRY_COUNT + 1))
    observed_indices = all_indices - missing_indices
    if len(observed_indices) != EXPECTED_OBSERVED_COUNT:
        raise RecoveryManifestError(f"expected {EXPECTED_OBSERVED_COUNT} observed plan indices, got {len(observed_indices)}")
    rows = build_recovery_rows(entries, missing_indices, observed_indices)
    row_missing_count = sum(row["attempt_count"] for row in rows)
    if row_missing_count != EXPECTED_MISSING_COUNT:
        raise RecoveryManifestError(f"recovery rows contain {row_missing_count} attempts, expected {EXPECTED_MISSING_COUNT}")
    represented_missing: list[int] = []
    for row in rows:
        represented_missing.extend(
            parse_plan_index_ranges(row["missing_plan_index_ranges"], f"{row['benchmark']}.missing_plan_index_ranges", EXPECTED_ENTRY_COUNT)
        )
    # Recovery rows are emitted in workflow_benchmark first-appearance order
    # and each row's missing plan indices are emitted in ascending plan_index
    # order. Workflow benchmarks need not be physically contiguous in the
    # source plan (backprop_adjust_weights interleaved with
    # backprop), so the assertion is on the multiset rather than the
    # concatenated sequence.
    if len(represented_missing) != len(set(represented_missing)):
        raise RecoveryManifestError("recovery rows contain duplicate missing plan indices")
    if sorted(represented_missing) != sorted(missing_indices):
        raise RecoveryManifestError("recovery rows do not exactly cover the missing plan indices")

    observed_range_strings = make_plan_index_ranges(observed_indices)
    tier_counts = plan.get("tier_counts")
    if not isinstance(tier_counts, Mapping):
        raise RecoveryManifestError("plan.tier_counts must be an object")

    return {
        "schema_name": MANIFEST_SCHEMA,
        "schema_version": SCHEMA_VERSION,
        "source_plan": {
            "artifact": repo_relative(plan_path),
            "sha256": file_sha256(plan_path),
            "schema_name": PLAN_SCHEMA,
            "schema_version": as_int(plan.get("schema_version"), "plan.schema_version"),
            "entry_count": EXPECTED_ENTRY_COUNT,
            "timeout_sec": EXPECTED_SOURCE_TIMEOUT_SEC,
            "tier_counts": dict(tier_counts),
        },
        "source_evidence": {
            "m24_coverage_report": repo_relative(m24_doc),
            "run_id": RUN_ID,
            "run_url": RUN_URL,
            "head_branch": RUN_HEAD_BRANCH,
            "head_sha": RUN_HEAD_SHA,
            "run_conclusion": "cancelled",
            "prior_attempt_timeout_sec": PRIOR_ATTEMPT_TIMEOUT_SEC,
            "evidence_policy": "Observed records remain incomplete prior evidence; this manifest contains only unobserved recovery scope.",
        },
        "source_plan_lookup_fields": [
            "problem_size",
            "sizes",
            "size_token",
            "size_label",
            "flag_template",
            "size_label_template",
            "timeout_sec",
            "artifact_id",
            "job_id",
            "extra_flags",
            "gomemlimit",
            "gogc",
        ],
        "generation_policy": {
            "scope_policy": "include only plan indices missing from run 25111815292; do not synthesize terminal outcomes",
            "ordering_policy": "source_plan_order_grouped_by_workflow_benchmark_with_missing_attempts_only",
            "row_policy": "one recovery row per workflow benchmark that has at least one missing plan index",
            "attempt_policy": "each recovery attempt is identified by plan_index; runner metadata is resolved from the source plan",
            "non_goals": [
                "no simulator execution",
                "no workflow dispatch/cancel/rerun",
                "no provenance or finishable manifest regeneration",
                "no terminal finishability completion claim",
            ],
        },
        "entry_count": EXPECTED_MISSING_COUNT,
        "recovery_entry_count": EXPECTED_MISSING_COUNT,
        "attempt_count": EXPECTED_MISSING_COUNT,
        "recovery_attempt_count": EXPECTED_MISSING_COUNT,
        "benchmark_count": len(rows),
        "recovery_row_count": len(rows),
        "timeout_sec": EXPECTED_SOURCE_TIMEOUT_SEC,
        "missing_plan_index_ranges": missing_range_strings,
        "prior_incomplete_evidence": {
            "run_id": RUN_ID,
            "treatment": "incomplete_prior_evidence_only",
            "observed_unique_plan_index_count": EXPECTED_OBSERVED_COUNT,
            "observed_raw_per_plan_outcome_file_count": EXPECTED_OBSERVED_COUNT,
            "duplicate_observed_plan_index_count": 0,
            "extra_observed_plan_index_count": 0,
            "observed_terminal_status_counts": EXPECTED_PRIOR_STATUS_COUNTS,
            "observed_plan_index_ranges": observed_range_strings,
            "complete_logical_rows_excluded_from_recovery": complete_prior_rows(entries, observed_indices),
            "not_complete_terminal_provenance": True,
        },
        "coverage": {
            "expected_plan_index_count": EXPECTED_ENTRY_COUNT,
            "source_plan_index_ranges": [f"1-{EXPECTED_ENTRY_COUNT}"],
            "recovery_missing_plan_index_count": EXPECTED_MISSING_COUNT,
            "recovery_missing_plan_index_ranges": missing_range_strings,
            "prior_observed_plan_index_count": EXPECTED_OBSERVED_COUNT,
            "prior_observed_plan_index_ranges": observed_range_strings,
            "represented_plan_index_count_after_union": EXPECTED_ENTRY_COUNT,
            "duplicate_recovery_plan_index_count": 0,
            "extra_recovery_plan_index_count": 0,
            "missing_recovery_plan_index_count": 0,
        },
        "recovery_rows": rows,
    }


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--plan", default=str(DEFAULT_PLAN), help="Source 1416-entry problem-size discovery plan")
    parser.add_argument("--m24-coverage-doc", default=str(DEFAULT_COVERAGE_DOC), help="incomplete coverage report")
    parser.add_argument("--output", default=str(DEFAULT_OUTPUT), help="Recovery manifest output path")
    parser.add_argument(
        "--check",
        action="store_true",
        help="Validate that --output already matches the generated manifest without rewriting it",
    )
    return parser.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    args = parse_args(argv)
    plan_path = repo_path(args.plan)
    m24_doc = repo_path(args.m24_coverage_doc)
    output_path = repo_path(args.output)
    try:
        missing_range_strings = None
        if args.check and not m24_doc.exists() and output_path.exists():
            missing_range_strings = load_checked_in_missing_ranges(output_path, m24_doc)
        manifest = build_manifest(plan_path, m24_doc, missing_range_strings)
        output_text = json_text(manifest)
        if args.check:
            try:
                current_text = output_path.read_text(encoding="utf-8")
            except FileNotFoundError as exc:
                raise RecoveryManifestError(f"{repo_relative(output_path)}: output file is missing") from exc
            if current_text != output_text:
                raise RecoveryManifestError(f"{repo_relative(output_path)} is not up to date; rerun this script")
            print(
                "OK: recovery manifest is up to date "
                f"missing={manifest['recovery_entry_count']} prior_observed={EXPECTED_OBSERVED_COUNT} "
                f"rows={manifest['recovery_row_count']}"
            )
        else:
            write_json(output_path, manifest)
            print(
                f"Wrote {repo_relative(output_path)}: "
                f"missing={manifest['recovery_entry_count']} prior_observed={EXPECTED_OBSERVED_COUNT} "
                f"rows={manifest['recovery_row_count']}"
            )
    except RecoveryManifestError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
