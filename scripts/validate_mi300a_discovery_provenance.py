#!/usr/bin/env python3
"""Static guard for MI300A discovery provenance."""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path
from typing import Any, Mapping, Optional, Sequence


REPO_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_PLAN = REPO_ROOT / "benchmark-comparison" / "mi300a_problem_size_discovery_plan.json"
SCHEMA_NAME = "mgpusim.mi300a_discovery_provenance_static_check"
SCHEMA_VERSION = 1

EXPECTED_BRANCH = "e5-m3-2-refresh-1416-entry-discovery-evidence"
EXPECTED_ENTRY_COUNT = 1416
EXPECTED_TIMEOUT_SEC = 3600

RUN_ID_RE = re.compile(r"^mi300a_problem_size_discovery_run_(?P<run_id>\d+)\.json$")
FULL_SHA_RE = re.compile(r"^[0-9a-f]{40}$", re.IGNORECASE)


# ---------------------------------------------------------------------------
# Small validation helpers
# ---------------------------------------------------------------------------


def repo_path(path_text: str) -> Path:
    path = Path(path_text)
    return path if path.is_absolute() else REPO_ROOT / path


def repo_relative(path: Path) -> str:
    try:
        return path.resolve().relative_to(REPO_ROOT).as_posix()
    except ValueError:
        return path.as_posix()


def load_object(path: Path, errors: list[str]) -> Optional[Mapping[str, Any]]:
    label = repo_relative(path)
    try:
        with path.open("r", encoding="utf-8") as f:
            data = json.load(f)
    except FileNotFoundError:
        errors.append(f"{label}: file not found")
        return None
    except json.JSONDecodeError as exc:
        errors.append(f"{label}: invalid JSON: {exc}")
        return None
    if not isinstance(data, Mapping):
        errors.append(f"{label}: JSON must be an object")
        return None
    return data


def as_object(value: Any, label: str, errors: list[str]) -> Optional[Mapping[str, Any]]:
    if not isinstance(value, Mapping):
        errors.append(f"{label} must be an object")
        return None
    return value


def as_int(value: Any, label: str, errors: list[str]) -> Optional[int]:
    if isinstance(value, bool):
        errors.append(f"{label} must be an integer, got boolean {value!r}")
        return None
    if isinstance(value, int):
        return value
    if isinstance(value, str) and value.strip().isdigit():
        return int(value.strip())
    errors.append(f"{label} must be an integer, got {value!r}")
    return None


def nonempty(value: Any, label: str, errors: list[str]) -> Optional[str]:
    if not isinstance(value, str) or not value.strip():
        errors.append(f"{label} must be a non-empty string")
        return None
    return value.strip()


def require_equal(actual: Any, expected: Any, label: str, errors: list[str]) -> None:
    if actual != expected:
        errors.append(f"{label} must be {expected!r}, got {actual!r}")


def first_count(mapping: Mapping[str, Any], keys: Sequence[str], label: str, errors: list[str]) -> Optional[int]:
    for key in keys:
        if key in mapping:
            return as_int(mapping.get(key), f"{label}.{key}", errors)
    errors.append(f"{label} must include one of: {', '.join(keys)}")
    return None


def coverage_object(data: Mapping[str, Any], label: str, errors: list[str]) -> Optional[Mapping[str, Any]]:
    top_level = data.get("raw_mi300a_timing_coverage")
    if isinstance(top_level, Mapping):
        return top_level
    plan_summary = data.get("plan_summary")
    if isinstance(plan_summary, Mapping) and isinstance(plan_summary.get("raw_mi300a_timing_coverage"), Mapping):
        return plan_summary["raw_mi300a_timing_coverage"]
    errors.append(f"{label} must include raw_mi300a_timing_coverage")
    return None


def coverage_counts(
    data: Mapping[str, Any],
    label: str,
    expected: Mapping[str, Any],
    errors: list[str],
) -> Optional[dict[str, int]]:
    coverage = coverage_object(data, label, errors)
    if coverage is None:
        return None

    counts = {
        "raw_unique": first_count(
            coverage,
            ("raw_unique_kernel_problem_size_count", "raw_unique", "unique_count"),
            f"{label}.raw_mi300a_timing_coverage",
            errors,
        ),
        "represented": first_count(
            coverage,
            (
                "plan_represented_unique_kernel_problem_size_count",
                "represented_unique_kernel_problem_size_count",
                "represented_count",
                "represented",
            ),
            f"{label}.raw_mi300a_timing_coverage",
            errors,
        ),
        "missing": first_count(
            coverage,
            ("missing_raw_measured_rows", "missing_count", "missing"),
            f"{label}.raw_mi300a_timing_coverage",
            errors,
        ),
        "skipped": first_count(
            coverage,
            ("skipped_exception_count", "skipped_count", "skipped"),
            f"{label}.raw_mi300a_timing_coverage",
            errors,
        ),
    }
    if any(value is None for value in counts.values()):
        return None

    require_equal(counts["raw_unique"], expected["expected_raw_unique_count"], f"{label}.raw_mi300a_timing_coverage.raw_unique", errors)
    require_equal(counts["represented"], expected["expected_represented_count"], f"{label}.raw_mi300a_timing_coverage.represented", errors)
    require_equal(counts["missing"], expected["expected_missing_count"], f"{label}.raw_mi300a_timing_coverage.missing", errors)
    require_equal(counts["skipped"], expected["expected_skipped_count"], f"{label}.raw_mi300a_timing_coverage.skipped", errors)
    if counts["represented"] + counts["skipped"] != counts["raw_unique"]:
        errors.append(
            f"{label}.raw_mi300a_timing_coverage represented plus skipped must equal raw_unique "
            f"({counts['represented']} + {counts['skipped']} != {counts['raw_unique']})"
        )
    return counts  # type: ignore[return-value]


# ---------------------------------------------------------------------------
# Contract checks
# ---------------------------------------------------------------------------


def validate_plan(plan_path: Path, expected: Mapping[str, Any], errors: list[str]) -> Optional[dict[str, Any]]:
    plan = load_object(plan_path, errors)
    if plan is None:
        return None
    label = repo_relative(plan_path)

    entries = plan.get("entries")
    if not isinstance(entries, list):
        errors.append(f"{label}: entries must be a list")
        entries = []

    entry_count = as_int(plan.get("entry_count"), f"{label}.entry_count", errors)
    if entry_count is not None:
        require_equal(entry_count, expected["expected_entry_count"], f"{label}.entry_count", errors)
        if entries and entry_count != len(entries):
            errors.append(f"{label}.entry_count must match entries length ({entry_count} != {len(entries)})")

    counts = coverage_counts(plan, label, expected, errors)

    hotspot_entries = [
        entry
        for entry in entries
        if isinstance(entry, Mapping)
        and str(entry.get("hardware_kernel_name") or entry.get("benchmark") or "").strip().lower() == "hotspot"
        and str(entry.get("hardware_problem_size") or entry.get("problem_size") or "").strip() == "48"
    ]
    if len(hotspot_entries) != 1:
        errors.append(f"{label}: expected exactly one hotspot/48 discovery entry, got {len(hotspot_entries)}")
        return None
    hotspot = hotspot_entries[0]
    plan_index = as_int(hotspot.get("plan_index"), f"{label}.hotspot_48.plan_index", errors)
    hardware_csv_line = as_int(hotspot.get("hardware_csv_line"), f"{label}.hotspot_48.hardware_csv_line", errors)
    require_equal(hotspot.get("sizes"), "48", f"{label}.hotspot_48.sizes", errors)
    require_equal(hotspot.get("timeout_sec"), expected["expected_timeout_sec"], f"{label}.hotspot_48.timeout_sec", errors)

    if None in (entry_count, counts, plan_index, hardware_csv_line):
        return None
    return {
        "path": label,
        "entry_count": entry_count,
        "raw_unique": counts["raw_unique"],
        "raw_represented": counts["represented"],
        "raw_missing": counts["missing"],
        "raw_skipped": counts["skipped"],
        "hotspot_48_plan_index": plan_index,
        "hotspot_48_hardware_csv_line": hardware_csv_line,
        "hotspot_48_sizes": str(hotspot.get("sizes")),
    }


def validate_plan_summary(
    data: Mapping[str, Any],
    label: str,
    plan_path: Path,
    expected: Mapping[str, Any],
    errors: list[str],
) -> Optional[int]:
    plan_summary = as_object(data.get("plan_summary"), f"{label}.plan_summary", errors)
    if plan_summary is None:
        return None

    artifact = nonempty(plan_summary.get("artifact"), f"{label}.plan_summary.artifact", errors)
    if artifact is not None:
        require_equal(artifact, repo_relative(plan_path), f"{label}.plan_summary.artifact", errors)

    entry_count = as_int(plan_summary.get("entry_count"), f"{label}.plan_summary.entry_count", errors)
    if entry_count is not None:
        require_equal(entry_count, expected["expected_entry_count"], f"{label}.plan_summary.entry_count", errors)

    first_plan_index = as_int(plan_summary.get("first_plan_index"), f"{label}.plan_summary.first_plan_index", errors)
    if first_plan_index is not None:
        require_equal(first_plan_index, 1, f"{label}.plan_summary.first_plan_index", errors)
    last_plan_index = as_int(plan_summary.get("last_plan_index"), f"{label}.plan_summary.last_plan_index", errors)
    if last_plan_index is not None:
        require_equal(last_plan_index, expected["expected_entry_count"], f"{label}.plan_summary.last_plan_index", errors)
    timeout_sec = as_int(plan_summary.get("timeout_sec"), f"{label}.plan_summary.timeout_sec", errors)
    if timeout_sec is not None:
        require_equal(timeout_sec, expected["expected_timeout_sec"], f"{label}.plan_summary.timeout_sec", errors)
    return entry_count


def validate_run(data: Mapping[str, Any], path: Path, expected: Mapping[str, Any], errors: list[str]) -> dict[str, Any]:
    label = repo_relative(path)
    run = as_object(data.get("run"), f"{label}.run", errors)
    if run is None:
        return {"run_id": None, "head_branch": None, "head_sha": None, "status": "", "conclusion": ""}

    run_id = as_int(run.get("database_id"), f"{label}.run.database_id", errors)
    if run_id is not None and run_id <= 0:
        errors.append(f"{label}.run.database_id must be positive, got {run_id!r}")
    filename_match = RUN_ID_RE.match(path.name)
    if filename_match and run_id is not None and int(filename_match.group("run_id")) != run_id:
        errors.append(f"{label}.run.database_id must match filename run id {filename_match.group('run_id')}, got {run_id}")

    head_branch = nonempty(run.get("head_branch"), f"{label}.run.head_branch", errors)
    if head_branch is not None:
        require_equal(head_branch, expected["expected_branch"], f"{label}.run.head_branch", errors)

    head_sha = nonempty(run.get("head_sha"), f"{label}.run.head_sha", errors)
    if head_sha is not None:
        if not FULL_SHA_RE.match(head_sha):
            errors.append(f"{label}.run.head_sha must be a full 40-character hex SHA, got {head_sha!r}")
        if expected.get("expected_head_sha") is not None:
            require_equal(head_sha, expected["expected_head_sha"], f"{label}.run.head_sha", errors)

    require_equal(run.get("event"), "workflow_dispatch", f"{label}.run.event", errors)
    nonempty(run.get("workflow_name"), f"{label}.run.workflow_name", errors)
    nonempty(run.get("created_at"), f"{label}.run.created_at", errors)
    status = nonempty(run.get("status"), f"{label}.run.status", errors) or ""
    conclusion = run.get("conclusion", "")
    if conclusion is None:
        conclusion = ""
    elif not isinstance(conclusion, str):
        errors.append(f"{label}.run.conclusion must be a string when present")
        conclusion = ""
    url = nonempty(run.get("url"), f"{label}.run.url", errors)
    if url is not None and run_id is not None and str(run_id) not in url:
        errors.append(f"{label}.run.url must contain run id {run_id}, got {url!r}")

    return {"run_id": run_id, "head_branch": head_branch, "head_sha": head_sha, "status": status, "conclusion": conclusion}


def validate_dispatch(data: Mapping[str, Any], path: Path, expected: Mapping[str, Any], run: Mapping[str, Any], errors: list[str]) -> None:
    label = repo_relative(path)
    dispatch = as_object(data.get("dispatch"), f"{label}.dispatch", errors)
    if dispatch is None:
        return

    command = nonempty(dispatch.get("command"), f"{label}.dispatch.command", errors)
    if command is not None:
        if "run_mode=problem-size-discovery" not in command:
            errors.append(f"{label}.dispatch.command must include run_mode=problem-size-discovery")
        if expected["expected_branch"] not in command:
            errors.append(f"{label}.dispatch.command must include branch {expected['expected_branch']!r}")

    selected = None
    for key in ("selected_run_id", "post_dispatch_selected_run_id", "run_id"):
        if key in dispatch:
            selected = as_int(dispatch.get(key), f"{label}.dispatch.{key}", errors)
            break
    if selected is None:
        errors.append(f"{label}.dispatch must include selected_run_id or post_dispatch_selected_run_id matching run.database_id")
    elif run.get("run_id") is not None and selected != run["run_id"]:
        errors.append(f"{label}.dispatch selected run id must be {run['run_id']}, got {selected}")

    dispatch_sha = dispatch.get("dispatched_at_head_sha")
    if dispatch_sha is not None:
        if not isinstance(dispatch_sha, str) or not dispatch_sha.strip():
            errors.append(f"{label}.dispatch.dispatched_at_head_sha must be a non-empty string when present")
        elif run.get("head_sha") is not None and dispatch_sha.strip() != run["head_sha"]:
            errors.append(f"{label}.dispatch.dispatched_at_head_sha must equal run.head_sha ({dispatch_sha!r} != {run['head_sha']!r})")


def validate_provenance(path: Path, plan_path: Path, expected: Mapping[str, Any], errors: list[str]) -> Optional[dict[str, Any]]:
    data = load_object(path, errors)
    if data is None:
        return None
    label = repo_relative(path)

    entry_count = validate_plan_summary(data, label, plan_path, expected, errors)
    counts = coverage_counts(data, label, expected, errors)
    run = validate_run(data, path, expected, errors)
    validate_dispatch(data, path, expected, run, errors)

    if None in (entry_count, counts, run.get("run_id"), run.get("head_branch"), run.get("head_sha")):
        return None
    return {
        "path": label,
        "run_id": run["run_id"],
        "head_branch": run["head_branch"],
        "head_sha": run["head_sha"],
        "status": run["status"],
        "conclusion": run["conclusion"],
        "entry_count": entry_count,
        "raw_unique": counts["raw_unique"],
        "raw_represented": counts["represented"],
        "raw_missing": counts["missing"],
        "raw_skipped": counts["skipped"],
    }


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Validate MI300A discovery provenance against the 1416-entry contract."
    )
    parser.add_argument("--provenance", nargs="+", required=True)
    parser.add_argument("--plan", default=str(DEFAULT_PLAN))
    parser.add_argument("--expected-branch", default=EXPECTED_BRANCH)
    parser.add_argument("--expected-head-sha")
    parser.add_argument("--expected-entry-count", type=int, default=EXPECTED_ENTRY_COUNT)
    parser.add_argument("--expected-raw-unique-count", type=int)
    parser.add_argument("--expected-represented-count", type=int)
    parser.add_argument("--expected-missing-count", type=int, default=0)
    parser.add_argument("--expected-skipped-count", type=int, default=0)
    parser.add_argument("--expected-timeout-sec", type=int, default=EXPECTED_TIMEOUT_SEC)
    return parser.parse_args(argv)


def expected_from_args(args: argparse.Namespace) -> dict[str, Any]:
    raw_unique = args.expected_raw_unique_count if args.expected_raw_unique_count is not None else args.expected_entry_count
    represented = args.expected_represented_count
    if represented is None:
        represented = raw_unique - args.expected_skipped_count
    return {
        "expected_branch": args.expected_branch,
        "expected_head_sha": args.expected_head_sha,
        "expected_entry_count": args.expected_entry_count,
        "expected_raw_unique_count": raw_unique,
        "expected_represented_count": represented,
        "expected_missing_count": args.expected_missing_count,
        "expected_skipped_count": args.expected_skipped_count,
        "expected_timeout_sec": args.expected_timeout_sec,
    }


def main(argv: Optional[Sequence[str]] = None) -> int:
    args = parse_args(argv)
    expected = expected_from_args(args)
    plan_path = repo_path(args.plan)
    provenance_paths = sorted({repo_path(path) for path in args.provenance}, key=repo_relative)

    errors: list[str] = []
    plan_summary = validate_plan(plan_path, expected, errors)
    provenance = [
        summary
        for summary in (validate_provenance(path, plan_path, expected, errors) for path in provenance_paths)
        if summary is not None
    ]

    if errors:
        print("FAIL: MI300A discovery provenance static check rejected input.", file=sys.stderr)
        for error in errors:
            print(f"  - {error}", file=sys.stderr)
        return 1

    json.dump(
        {
            "schema_name": SCHEMA_NAME,
            "schema_version": SCHEMA_VERSION,
            "expected": expected,
            "plan": plan_summary,
            "provenance_count": len(provenance),
            "provenance": provenance,
        },
        sys.stdout,
        indent=2,
        sort_keys=False,
    )
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
