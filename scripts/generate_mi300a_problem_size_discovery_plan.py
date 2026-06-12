#!/usr/bin/env python3
"""Generate the MI300A 3600s problem-size discovery plan.

The plan is derived from existing repository contracts:

* ``benchmark-comparison/comparison_ci.csv`` supplies the historical MI300A
  hardware rows used by the comparison/reporting path.  A row is in scope when
  ``real_ms`` is non-empty.
* ``workloads/results/kernel_timings_20260317-075319-odyssey.csv`` is the raw
  MI300A timing contract that discovery coverage reconciles against.  Any unique
  measured ``kernel_name``/``problem_size`` pair missing from ``comparison_ci`` is
  added directly from this raw timing file.
* ``workloads/results/results_20260317-075319-odyssey.csv`` supplies raw success
  rows for benchmark/problem sizes where kernel and benchmark names align.
* A legacy full-mode ``benchmark.yml`` can supply runnable benchmark metadata
  when this tool is used against an archived/fixture workflow.  The current
  checked-in workflow is the maintained linear regular-workflow-only path and
  intentionally no longer contains those legacy matrices, so the default
  repository invocation reconciles and re-emits the checked-in 1416-entry
  archival discovery plan instead of rebuilding it from ``benchmark.yml``.

Each emitted entry is one unique hardware ``kernel_name``/``problem_size`` pair
mapped to one runnable workflow benchmark with a single size token for the runner
loop. Discovery intentionally overrides every per-entry timeout to 3600 seconds.
"""

from __future__ import annotations

import argparse
import csv
import hashlib
import importlib.util
import json
import math
import re
import sys
from collections import Counter, defaultdict
from dataclasses import dataclass, replace
from pathlib import Path
from typing import Any, Dict, Iterable, List, Optional, Sequence, Tuple


REPO_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_COMPARISON = REPO_ROOT / "benchmark-comparison" / "comparison_ci.csv"
DEFAULT_RAW_KERNEL_TIMINGS = REPO_ROOT / "workloads" / "results" / "kernel_timings_20260317-075319-odyssey.csv"
DEFAULT_RAW_RESULTS = REPO_ROOT / "workloads" / "results" / "results_20260317-075319-odyssey.csv"
DEFAULT_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "benchmark.yml"
DEFAULT_MERGE_SCRIPT = REPO_ROOT / "benchmark-comparison" / "merge_ci_results.py"
DEFAULT_PLAN_OUTPUT = REPO_ROOT / "benchmark-comparison" / "mi300a_problem_size_discovery_plan.json"
DEFAULT_ARCHIVED_PLAN = DEFAULT_PLAN_OUTPUT

PLAN_SCHEMA_NAME = "mgpusim.mi300a_problem_size_discovery_plan"
PLAN_SCHEMA_VERSION = 1
DISCOVERY_TIMEOUT_SEC = 3600
FULL_MODE_WORKFLOW_JOBS = ("benchmark-tier1", "benchmark-tier2")
ORDERING_POLICY = "tier_then_full_mode_workflow_order_then_natural_problem_size_then_kernel"
SELECTION_POLICY = (
    "comparison_ci_rows_with_nonempty_real_ms_plus_missing_raw_kernel_timings_"
    "mapped_to_full_mode_workflow_template"
)
ARCHIVAL_FALLBACK_POLICY = (
    "current_default_workflow_has_no_legacy_full_mode_matrix; "
    "reconcile_and_emit_checked_in_1416_entry_discovery_plan"
)

REQUIRED_COMPARISON_COLUMNS = ("kernel_name", "problem_size", "real_ms")
REQUIRED_RAW_KERNEL_TIMING_COLUMNS = (
    "suite",
    "benchmark",
    "platform",
    "problem_size",
    "kernel_name",
    "avg_ms",
)
REQUIRED_RAW_RESULTS_COLUMNS = (
    "suite",
    "benchmark",
    "platform",
    "problem_size",
    "total_avg_ms",
    "status",
)
REQUIRED_WORKFLOW_ENTRY_KEYS = (
    "benchmark",
    "tier",
    "sizes",
    "flag_template",
    "size_label_template",
)
OPTIONAL_WORKFLOW_RUNTIME_KEYS = ("extra_flags", "gomemlimit", "gogc")

# merge_ci_results.py maps FindMinSAD to a secondary result pseudo-name because
# it reads results_findminsad.csv during merge.  For discovery we need the
# runnable workflow benchmark that produces that secondary result file.
PLAN_ONLY_KERNEL_TO_WORKFLOW_BENCHMARK = {
    "findminsad": (
        "parboil_sad",
        "FindMinSAD is emitted as a secondary per-kernel result by the runnable parboil_sad workflow benchmark.",
    ),
}


class PlanError(Exception):
    """Raised when the discovery plan cannot be generated or validated."""


@dataclass(frozen=True)
class WorkflowEntry:
    benchmark: str
    tier_name: str
    tier: int
    sizes: Tuple[str, ...]
    flag_template: str
    size_label_template: str
    workflow_timeout_sec: Optional[int]
    extra_flags: Optional[str]
    gomemlimit: Optional[str]
    gogc: Optional[str]
    workflow_job: str
    workflow_line: int
    workflow_matrix_index: int = 0


@dataclass(frozen=True)
class HardwareRow:
    csv_line: int
    kernel_name: str
    problem_size: str
    real_ms: str
    source: str = "comparison_ci"
    raw_benchmark: Optional[str] = None
    raw_suite: Optional[str] = None


@dataclass(frozen=True)
class RawResultRow:
    csv_line: int
    benchmark: str
    problem_size: str
    total_avg_ms: str
    suite: str


@dataclass(frozen=True)
class ResolvedRow:
    hardware: HardwareRow
    workflow: WorkflowEntry
    size_token: str
    size_token_source: str
    mapping_source: str
    mapping_reason: Optional[str]


# ---------------------------------------------------------------------------
# CLI and generic helpers
# ---------------------------------------------------------------------------


def parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Validate MI300A hardware rows against archived legacy full-mode "
            "benchmark metadata and emit the deterministic 3600s problem-size "
            "discovery plan. With the current maintained linear regular-workflow-only "
            "path, the default repository invocation reconciles/re-emits the checked-in "
            "archival plan instead of rebuilding from benchmark.yml."
        )
    )
    parser.add_argument(
        "--comparison",
        default=str(DEFAULT_COMPARISON),
        help="Path to comparison_ci.csv (default: benchmark-comparison/comparison_ci.csv).",
    )
    parser.add_argument(
        "--raw-kernel-timings",
        default=str(DEFAULT_RAW_KERNEL_TIMINGS),
        help=(
            "Path to the tracked raw MI300A kernel timing CSV used for discovery coverage "
            "reconciliation (default: workloads/results/kernel_timings_20260317-075319-odyssey.csv)."
        ),
    )
    parser.add_argument(
        "--raw-results",
        default=str(DEFAULT_RAW_RESULTS),
        help=(
            "Path to the tracked raw MI300A benchmark results CSV used where kernel and benchmark "
            "names align (default: workloads/results/results_20260317-075319-odyssey.csv)."
        ),
    )
    parser.add_argument(
        "--workflow",
        default=str(DEFAULT_WORKFLOW),
        help="Path to benchmark.yml (default: .github/workflows/benchmark.yml).",
    )
    parser.add_argument(
        "--merge-script",
        default=str(DEFAULT_MERGE_SCRIPT),
        help=(
            "Path to merge_ci_results.py for CSV-kernel-to-workflow-benchmark aliases "
            "(default: benchmark-comparison/merge_ci_results.py)."
        ),
    )
    parser.add_argument(
        "--archived-plan",
        default=str(DEFAULT_ARCHIVED_PLAN),
        help=(
            "Checked-in archival discovery plan to validate/re-emit when the default "
            "maintained linear regular workflow no longer contains the legacy full-mode "
            "matrix (default: benchmark-comparison/mi300a_problem_size_discovery_plan.json)."
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


def json_text(data: Dict[str, Any]) -> str:
    return json.dumps(data, indent=2, sort_keys=False) + "\n"


def require_nonempty_string(value: Any, label: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise PlanError(f"{label} must be a non-empty string")
    return value.strip()


# ---------------------------------------------------------------------------
# Workflow matrix parsing
# ---------------------------------------------------------------------------


def load_workflow_entries(path: Path) -> List[WorkflowEntry]:
    if not path.is_file():
        raise PlanError(f"workflow not found: {path}")
    lines = path.read_text(encoding="utf-8").splitlines()

    entries: List[WorkflowEntry] = []
    for job_name in FULL_MODE_WORKFLOW_JOBS:
        start, end = workflow_job_line_range(lines, job_name)
        entries.extend(parse_workflow_job_entries(lines[start:end], job_name, line_offset=start))

    if not entries:
        raise PlanError(
            "workflow has no full-mode benchmark entries in jobs: "
            + ", ".join(FULL_MODE_WORKFLOW_JOBS)
        )

    indexed_entries: List[WorkflowEntry] = []
    for idx, entry in enumerate(entries, start=1):
        indexed_entries.append(replace(entry, workflow_matrix_index=idx))
    return indexed_entries


def workflow_job_line_range(lines: List[str], job_name: str) -> Tuple[int, int]:
    start = None  # type: Optional[int]
    job_header = re.compile(r"^  " + re.escape(job_name) + r":\s*$")
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
    lines: List[str], job_name: str, line_offset: int
) -> List[WorkflowEntry]:
    raw_entries = []  # type: List[Dict[str, Any]]
    current = None  # type: Optional[Dict[str, Any]]
    current_indent = None  # type: Optional[int]
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


def validate_workflow_entry(raw_entry: Dict[str, Any]) -> WorkflowEntry:
    line_no = int(raw_entry["_line_no"])
    job_name = str(raw_entry["_job_name"])
    missing = [key for key in REQUIRED_WORKFLOW_ENTRY_KEYS if key not in raw_entry]
    if missing:
        raise PlanError(
            f"malformed workflow: benchmark entry at line {line_no} missing keys: "
            + ", ".join(missing)
        )

    benchmark = require_workflow_string(raw_entry["benchmark"], line_no, "benchmark")
    tier_name = require_workflow_string(raw_entry["tier"], line_no, "tier")
    if tier_name not in ("tier1", "tier2"):
        raise PlanError(
            f"malformed workflow: benchmark {benchmark!r} at line {line_no} has unsupported tier {tier_name!r}"
        )
    tier = int(tier_name.replace("tier", ""))

    sizes_raw = require_workflow_string(raw_entry["sizes"], line_no, "sizes")
    sizes = tuple(sizes_raw.split())
    if not sizes:
        raise PlanError(f"malformed workflow: benchmark {benchmark!r} at line {line_no} has no sizes")
    for token in sizes:
        if len(token.split()) != 1:
            raise PlanError(
                f"malformed workflow: benchmark {benchmark!r} at line {line_no} has non-single-token size {token!r}"
            )

    flag_template = require_workflow_string(raw_entry["flag_template"], line_no, "flag_template")
    size_label_template = require_workflow_string(
        raw_entry["size_label_template"], line_no, "size_label_template"
    )
    workflow_timeout_sec = parse_optional_timeout(raw_entry.get("timeout_sec"), benchmark, line_no)
    extra_flags = parse_optional_workflow_string(raw_entry.get("extra_flags"), benchmark, line_no, "extra_flags")
    gomemlimit = parse_optional_workflow_string(raw_entry.get("gomemlimit"), benchmark, line_no, "gomemlimit")
    gogc = parse_optional_workflow_string(raw_entry.get("gogc"), benchmark, line_no, "gogc")

    return WorkflowEntry(
        benchmark=benchmark,
        tier_name=tier_name,
        tier=tier,
        sizes=sizes,
        flag_template=flag_template,
        size_label_template=size_label_template,
        workflow_timeout_sec=workflow_timeout_sec,
        extra_flags=extra_flags,
        gomemlimit=gomemlimit,
        gogc=gogc,
        workflow_job=job_name,
        workflow_line=line_no,
    )


def require_workflow_string(value: Any, line_no: int, key: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise PlanError(f"malformed workflow: benchmark entry at line {line_no} has empty {key}")
    return value.strip()


def parse_optional_workflow_string(
    value: Any, benchmark: str, line_no: int, key: str
) -> Optional[str]:
    if value is None:
        return None
    if not isinstance(value, str) or not value.strip():
        raise PlanError(f"malformed workflow: benchmark {benchmark!r} at line {line_no} has empty {key}")
    return value.strip()


def parse_optional_timeout(value: Any, benchmark: str, line_no: int) -> Optional[int]:
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
# Source data and alias mapping
# ---------------------------------------------------------------------------


def load_hardware_rows(path: Path) -> List[HardwareRow]:
    if not path.is_file():
        raise PlanError(f"comparison CSV not found: {path}")

    rows: List[HardwareRow] = []
    with path.open(newline="", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        if reader.fieldnames is None:
            raise PlanError(f"comparison CSV has no header: {path}")
        missing_columns = [key for key in REQUIRED_COMPARISON_COLUMNS if key not in reader.fieldnames]
        if missing_columns:
            raise PlanError(
                "comparison CSV missing required columns: " + ", ".join(missing_columns)
            )

        for line_no, row in enumerate(reader, start=2):
            real_ms = (row.get("real_ms") or "").strip()
            if not real_ms:
                continue
            kernel_name = (row.get("kernel_name") or "").strip()
            problem_size = (row.get("problem_size") or "").strip()
            if not kernel_name or not problem_size:
                raise PlanError(
                    f"comparison CSV row {line_no} has non-empty real_ms but missing kernel_name/problem_size"
                )
            try:
                parsed_real_ms = float(real_ms)
            except ValueError as exc:
                raise PlanError(
                    f"comparison CSV row {line_no} has non-numeric real_ms {real_ms!r}"
                ) from exc
            if not math.isfinite(parsed_real_ms) or parsed_real_ms < 0:
                raise PlanError(
                    f"comparison CSV row {line_no} has non-finite or negative real_ms {real_ms!r}"
                )
            rows.append(
                HardwareRow(
                    csv_line=line_no,
                    kernel_name=kernel_name,
                    problem_size=problem_size,
                    real_ms=real_ms,
                )
            )

    if not rows:
        raise PlanError(f"comparison CSV has no rows with non-empty real_ms: {path}")
    return rows


def load_raw_kernel_timing_rows(path: Path) -> List[HardwareRow]:
    if not path.is_file():
        raise PlanError(f"raw kernel timing CSV not found: {path}")

    rows: List[HardwareRow] = []
    with path.open(newline="", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        if reader.fieldnames is None:
            raise PlanError(f"raw kernel timing CSV has no header: {path}")
        missing_columns = [key for key in REQUIRED_RAW_KERNEL_TIMING_COLUMNS if key not in reader.fieldnames]
        if missing_columns:
            raise PlanError(
                "raw kernel timing CSV missing required columns: " + ", ".join(missing_columns)
            )

        for line_no, row in enumerate(reader, start=2):
            avg_ms = (row.get("avg_ms") or "").strip()
            if not avg_ms:
                continue
            kernel_name = (row.get("kernel_name") or "").strip()
            problem_size = (row.get("problem_size") or "").strip()
            benchmark = (row.get("benchmark") or "").strip()
            suite = (row.get("suite") or "").strip()
            if not kernel_name or not problem_size:
                raise PlanError(
                    f"raw kernel timing CSV row {line_no} has non-empty avg_ms but missing kernel_name/problem_size"
                )
            try:
                parsed_avg_ms = float(avg_ms)
            except ValueError as exc:
                raise PlanError(
                    f"raw kernel timing CSV row {line_no} has non-numeric avg_ms {avg_ms!r}"
                ) from exc
            if not math.isfinite(parsed_avg_ms) or parsed_avg_ms < 0:
                raise PlanError(
                    f"raw kernel timing CSV row {line_no} has non-finite or negative avg_ms {avg_ms!r}"
                )
            rows.append(
                HardwareRow(
                    csv_line=line_no,
                    kernel_name=kernel_name,
                    problem_size=problem_size,
                    real_ms=avg_ms,
                    source="raw_kernel_timings",
                    raw_benchmark=benchmark or None,
                    raw_suite=suite or None,
                )
            )

    if not rows:
        raise PlanError(f"raw kernel timing CSV has no measured avg_ms rows: {path}")
    return rows


def load_raw_result_rows(path: Path) -> List[RawResultRow]:
    if not path.is_file():
        raise PlanError(f"raw results CSV not found: {path}")

    rows: List[RawResultRow] = []
    with path.open(newline="", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        if reader.fieldnames is None:
            raise PlanError(f"raw results CSV has no header: {path}")
        missing_columns = [key for key in REQUIRED_RAW_RESULTS_COLUMNS if key not in reader.fieldnames]
        if missing_columns:
            raise PlanError("raw results CSV missing required columns: " + ", ".join(missing_columns))

        for line_no, row in enumerate(reader, start=2):
            status = (row.get("status") or "").strip().lower()
            total_avg_ms = (row.get("total_avg_ms") or "").strip()
            if status != "success" or not total_avg_ms:
                continue
            benchmark = (row.get("benchmark") or "").strip()
            problem_size = (row.get("problem_size") or "").strip()
            suite = (row.get("suite") or "").strip()
            if not benchmark or not problem_size:
                raise PlanError(
                    f"raw results CSV row {line_no} is success but missing benchmark/problem_size"
                )
            try:
                parsed_total_avg_ms = float(total_avg_ms)
            except ValueError as exc:
                raise PlanError(
                    f"raw results CSV row {line_no} has non-numeric total_avg_ms {total_avg_ms!r}"
                ) from exc
            if not math.isfinite(parsed_total_avg_ms) or parsed_total_avg_ms < 0:
                raise PlanError(
                    f"raw results CSV row {line_no} has non-finite or negative total_avg_ms {total_avg_ms!r}"
                )
            rows.append(
                RawResultRow(
                    csv_line=line_no,
                    benchmark=benchmark,
                    problem_size=problem_size,
                    total_avg_ms=total_avg_ms,
                    suite=suite,
                )
            )

    if not rows:
        raise PlanError(f"raw results CSV has no successful measured rows: {path}")
    return rows


def hardware_pair_key(row: HardwareRow) -> Tuple[str, str]:
    return row.kernel_name.strip().lower(), row.problem_size.strip()


def result_pair_key(row: RawResultRow) -> Tuple[str, str]:
    return row.benchmark.strip().lower(), row.problem_size.strip()


def unique_hardware_rows(rows: Iterable[HardwareRow]) -> Dict[Tuple[str, str], HardwareRow]:
    unique: Dict[Tuple[str, str], HardwareRow] = {}
    for row in rows:
        unique.setdefault(hardware_pair_key(row), row)
    return unique


def merge_comparison_with_raw_timings(
    comparison_rows: List[HardwareRow], raw_kernel_rows: List[HardwareRow]
) -> Tuple[List[HardwareRow], Dict[str, int]]:
    comparison_keys = {hardware_pair_key(row) for row in comparison_rows}
    raw_unique = unique_hardware_rows(raw_kernel_rows)
    supplemental_rows = [row for key, row in raw_unique.items() if key not in comparison_keys]
    merged_rows = list(comparison_rows) + supplemental_rows
    return merged_rows, {
        "comparison_measured_row_count": len(comparison_rows),
        "raw_kernel_timing_measured_row_count": len(raw_kernel_rows),
        "raw_unique_kernel_problem_size_count": len(raw_unique),
        "raw_duplicate_kernel_problem_size_row_count": len(raw_kernel_rows) - len(raw_unique),
        "supplemental_raw_kernel_timing_rows_added": len(supplemental_rows),
    }


def validate_raw_results_reconciliation(
    raw_kernel_rows: List[HardwareRow], raw_result_rows: List[RawResultRow]
) -> Dict[str, int]:
    result_pairs = {result_pair_key(row) for row in raw_result_rows}
    same_benchmark_unique: Dict[Tuple[str, str], HardwareRow] = {}
    for row in raw_kernel_rows:
        benchmark = (row.raw_benchmark or "").strip()
        if benchmark and benchmark.lower() == row.kernel_name.strip().lower():
            same_benchmark_unique.setdefault(hardware_pair_key(row), row)

    missing = [row for key, row in same_benchmark_unique.items() if key not in result_pairs]
    if missing:
        details = "; ".join(
            f"raw kernel line {row.csv_line}: {row.kernel_name!r}/{row.problem_size!r}"
            for row in missing[:20]
        )
        if len(missing) > 20:
            details += f"; ... and {len(missing) - 20} more"
        raise PlanError(
            "raw results reconciliation failed: same-name kernel timing rows lack successful "
            f"benchmark result rows in the raw results CSV: {details}"
        )

    return {
        "raw_results_success_row_count": len(raw_result_rows),
        "raw_results_unique_success_benchmark_problem_size_count": len(result_pairs),
        "same_benchmark_kernel_result_success_checks": len(same_benchmark_unique),
        "missing_result_success_for_same_benchmark_kernel_rows": 0,
    }


def validate_plan_covers_raw_timings(
    plan: Dict[str, Any], raw_kernel_rows: List[HardwareRow], skipped_exception_count: int = 0
) -> Dict[str, int]:
    raw_unique = unique_hardware_rows(raw_kernel_rows)
    represented_pairs = {
        (str(entry.get("hardware_kernel_name", "")).strip().lower(), str(entry.get("hardware_problem_size", "")).strip())
        for entry in plan.get("entries", [])
        if isinstance(entry, dict)
    }
    missing_keys = sorted(set(raw_unique) - represented_pairs)
    if missing_keys:
        details = "; ".join(
            f"raw kernel line {raw_unique[key].csv_line}: {raw_unique[key].kernel_name!r}/{raw_unique[key].problem_size!r}"
            for key in missing_keys[:20]
        )
        if len(missing_keys) > 20:
            details += f"; ... and {len(missing_keys) - 20} more"
        raise PlanError(
            "raw kernel timing coverage reconciliation failed: measured raw kernel/problem-size "
            f"rows are not represented in the discovery plan or skipped by exception: {details}"
        )
    accounted = len(represented_pairs & set(raw_unique)) + skipped_exception_count
    if accounted != len(raw_unique):
        raise PlanError(
            "raw kernel timing coverage reconciliation failed: represented plan entries plus "
            f"skipped exceptions ({accounted}) do not match unique raw measured rows ({len(raw_unique)})"
        )
    return {
        "plan_represented_unique_kernel_problem_size_count": len(represented_pairs & set(raw_unique)),
        "missing_raw_measured_rows": 0,
        "skipped_exception_count": skipped_exception_count,
    }


def same_path(left: Path, right: Path) -> bool:
    return left.resolve() == right.resolve()


def workflow_error_allows_archival_fallback(error: PlanError) -> bool:
    message = str(error)
    return "workflow has no full-mode benchmark entries" in message or "workflow job not found:" in message


def should_use_archived_plan_fallback(
    comparison_path: Path,
    raw_kernel_timings_path: Path,
    raw_results_path: Path,
    workflow_path: Path,
    merge_script_path: Path,
    archived_plan_path: Path,
    workflow_error: PlanError,
) -> bool:
    if not workflow_error_allows_archival_fallback(workflow_error):
        return False
    if not archived_plan_path.is_file():
        return False
    default_source_paths = (
        (comparison_path, DEFAULT_COMPARISON),
        (raw_kernel_timings_path, DEFAULT_RAW_KERNEL_TIMINGS),
        (raw_results_path, DEFAULT_RAW_RESULTS),
        (workflow_path, DEFAULT_WORKFLOW),
        (merge_script_path, DEFAULT_MERGE_SCRIPT),
    )
    return all(same_path(path, default_path) for path, default_path in default_source_paths)


def load_and_validate_archived_plan(
    archived_plan_path: Path,
    comparison_path: Path,
    raw_kernel_timings_path: Path,
    raw_results_path: Path,
    workflow_path: Path,
    merge_script_path: Path,
    hardware_rows: List[HardwareRow],
    raw_kernel_rows: List[HardwareRow],
    raw_merge_summary: Dict[str, int],
    raw_results_summary: Dict[str, int],
) -> Dict[str, Any]:
    try:
        archived_plan = json.loads(archived_plan_path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise PlanError(f"archived discovery plan is not valid JSON: {archived_plan_path}: {exc}") from exc
    if not isinstance(archived_plan, dict):
        raise PlanError(f"archived discovery plan must be a JSON object: {archived_plan_path}")

    expected_source_paths = {
        "comparison_csv": repo_relative(comparison_path),
        "raw_kernel_timings_csv": repo_relative(raw_kernel_timings_path),
        "raw_results_csv": repo_relative(raw_results_path),
        "workflow": repo_relative(workflow_path),
        "merge_mapping": repo_relative(merge_script_path),
    }
    if archived_plan.get("source_paths") != expected_source_paths:
        raise PlanError(
            "archived discovery plan source_paths do not match the requested default inputs: "
            f"expected {expected_source_paths!r}, got {archived_plan.get('source_paths')!r}"
        )

    return reconcile_archived_plan_with_current_inputs(
        archived_plan,
        hardware_rows,
        raw_kernel_rows,
        raw_merge_summary,
        raw_results_summary,
    )


def reconcile_archived_plan_with_current_inputs(
    archived_plan: Dict[str, Any],
    hardware_rows: List[HardwareRow],
    raw_kernel_rows: List[HardwareRow],
    raw_merge_summary: Dict[str, int],
    raw_results_summary: Dict[str, int],
) -> Dict[str, Any]:
    validate_plan(archived_plan)
    current_hardware_by_key = unique_hardware_rows_or_error(
        hardware_rows,
        "current comparison/raw hardware inputs",
    )
    archived_entries = archived_plan.get("entries", [])
    archived_entries_by_key: Dict[Tuple[str, str], Dict[str, Any]] = {}
    archived_order: List[Tuple[str, str]] = []
    duplicate_archive_keys: List[Tuple[str, str]] = []

    for entry_index, entry in enumerate(archived_entries, start=1):
        if not isinstance(entry, dict):
            raise PlanError(
                f"archived discovery plan entry {entry_index} must be a JSON object"
            )
        key = plan_entry_hardware_pair_key(entry, entry_index)
        if key in archived_entries_by_key:
            duplicate_archive_keys.append(key)
            continue
        archived_entries_by_key[key] = entry
        archived_order.append(key)

    if duplicate_archive_keys:
        raise PlanError(
            "archived discovery plan has duplicate hardware kernel/problem-size entries: "
            + format_pair_details(duplicate_archive_keys)
        )

    current_keys = set(current_hardware_by_key)
    archived_keys = set(archived_entries_by_key)
    missing_template_keys = sorted(current_keys - archived_keys)
    if missing_template_keys:
        raise PlanError(
            "archived discovery plan cannot reconcile current MI300A timing inputs because runnable "
            "metadata is missing for: "
            + format_pair_details(missing_template_keys)
        )

    reconciled = dict(archived_plan)
    reconciled_entries: List[Dict[str, Any]] = []
    for key in archived_order:
        hardware = current_hardware_by_key.get(key)
        if hardware is None:
            continue
        reconciled_entries.append(
            reconcile_archived_entry(archived_entries_by_key[key], hardware)
        )

    for plan_index, entry in enumerate(reconciled_entries, start=1):
        entry["plan_index"] = plan_index

    reconciled["entries"] = reconciled_entries
    update_plan_summaries(
        reconciled,
        raw_kernel_rows,
        raw_merge_summary,
        raw_results_summary,
    )
    validate_plan(reconciled)
    return reconciled


def unique_hardware_rows_or_error(
    rows: Iterable[HardwareRow], label: str
) -> Dict[Tuple[str, str], HardwareRow]:
    unique: Dict[Tuple[str, str], HardwareRow] = {}
    duplicates: List[Tuple[str, str]] = []
    for row in rows:
        key = hardware_pair_key(row)
        if key in unique:
            duplicates.append(key)
            continue
        unique[key] = row
    if duplicates:
        raise PlanError(
            f"{label} contain duplicate kernel/problem-size rows: "
            + format_pair_details(duplicates)
        )
    return unique


def plan_entry_hardware_pair_key(
    entry: Dict[str, Any], entry_index: int
) -> Tuple[str, str]:
    kernel_name = require_plan_entry_string(entry, "hardware_kernel_name", entry_index)
    problem_size = require_plan_entry_string(entry, "hardware_problem_size", entry_index)
    return kernel_name.lower(), problem_size


def require_plan_entry_string(
    entry: Dict[str, Any], key: str, entry_index: int
) -> str:
    value = entry.get(key)
    if not isinstance(value, str) or not value.strip():
        raise PlanError(f"archived discovery plan entry {entry_index} has empty {key}")
    return value.strip()


def reconcile_archived_entry(
    entry: Dict[str, Any], hardware: HardwareRow
) -> Dict[str, Any]:
    reconciled = dict(entry)
    reconciled["benchmark"] = hardware.kernel_name
    reconciled["problem_size"] = hardware.problem_size
    reconciled["hardware_kernel_name"] = hardware.kernel_name
    reconciled["hardware_problem_size"] = hardware.problem_size
    reconciled["hardware_real_ms"] = hardware.real_ms
    reconciled["hardware_csv_line"] = hardware.csv_line
    reconciled["size_label"] = hardware.problem_size

    workflow_benchmark = str(reconciled.get("workflow_benchmark") or "").strip()
    identity_key = hardware.kernel_name + "\0" + hardware.problem_size
    reconciled["job_id"] = make_safe_id(
        "mi300a-discovery",
        hardware.kernel_name,
        hardware.problem_size,
        identity_key,
    )
    reconciled["artifact_id"] = make_safe_id(
        "mi300a-discovery-artifact",
        workflow_benchmark,
        hardware.kernel_name,
        hardware.problem_size,
        identity_key,
    )
    return reconciled


def update_plan_summaries(
    plan: Dict[str, Any],
    raw_kernel_rows: List[HardwareRow],
    raw_merge_summary: Dict[str, int],
    raw_results_summary: Dict[str, int],
) -> None:
    entries = plan.get("entries", [])
    if not isinstance(entries, list):
        raise PlanError(
            "problem-size discovery plan validation failed: entries must be a list"
        )

    dict_entries = [entry for entry in entries if isinstance(entry, dict)]
    plan["entry_count"] = len(entries)
    plan["hardware_row_count"] = len(entries)
    plan["hardware_kernel_count"] = len(
        {entry.get("hardware_kernel_name") for entry in dict_entries}
    )
    plan["workflow_benchmark_count"] = len(
        {entry.get("workflow_benchmark") for entry in dict_entries}
    )
    plan["tier_counts"] = dict(
        sorted(Counter(str(entry.get("tier")) for entry in dict_entries).items())
    )
    plan["size_token_source_counts"] = dict(
        sorted(
            Counter(str(entry.get("size_token_source")) for entry in dict_entries).items()
        )
    )
    plan["kernel_to_workflow_mapping_source_counts"] = dict(
        sorted(
            Counter(
                str(entry.get("kernel_to_workflow_mapping_source"))
                for entry in dict_entries
            ).items()
        )
    )

    raw_plan_summary = validate_plan_covers_raw_timings(plan, raw_kernel_rows)
    plan["raw_mi300a_timing_coverage"] = {
        **plan.get("raw_mi300a_timing_coverage", {}),
        **raw_merge_summary,
        **raw_results_summary,
        **raw_plan_summary,
    }
    validation = dict(plan.get("validation", {}))
    validation["missing_raw_measured_rows"] = raw_plan_summary["missing_raw_measured_rows"]
    validation["skipped_exception_count"] = raw_plan_summary["skipped_exception_count"]
    plan["validation"] = validation


def format_pair_details(keys: Iterable[Tuple[str, str]], limit: int = 20) -> str:
    ordered = sorted(keys)
    details = "; ".join(
        f"{kernel!r}/{problem_size!r}" for kernel, problem_size in ordered[:limit]
    )
    if len(ordered) > limit:
        details += f"; ... and {len(ordered) - limit} more"
    return details


def load_csv_kernel_mapping(path: Path) -> Dict[str, str]:
    if not path.is_file():
        raise PlanError(f"merge mapping script not found: {path}")
    spec = importlib.util.spec_from_file_location("_mi300a_merge_ci_results_mapping", str(path))
    if spec is None or spec.loader is None:
        raise PlanError(f"could not load merge mapping script: {path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)  # type: ignore[union-attr]
    raw_mapping = getattr(module, "CSV_KERNEL_TO_BENCH_DIR", None)
    if not isinstance(raw_mapping, dict):
        raise PlanError(f"merge mapping script does not define CSV_KERNEL_TO_BENCH_DIR: {path}")

    mapping: Dict[str, str] = {}
    for key, value in raw_mapping.items():
        if not isinstance(key, str) or not isinstance(value, str) or not key.strip() or not value.strip():
            raise PlanError(f"merge mapping script has malformed CSV_KERNEL_TO_BENCH_DIR entry: {key!r} -> {value!r}")
        mapping[key.strip().lower()] = value.strip()
    return mapping


def resolve_workflow_benchmark_name(
    hardware_kernel_name: str, csv_kernel_mapping: Dict[str, str]
) -> Tuple[str, str, Optional[str]]:
    key = hardware_kernel_name.strip().lower()
    if key in PLAN_ONLY_KERNEL_TO_WORKFLOW_BENCHMARK:
        workflow_benchmark, reason = PLAN_ONLY_KERNEL_TO_WORKFLOW_BENCHMARK[key]
        return workflow_benchmark, "plan_override", reason
    if key in csv_kernel_mapping:
        return (
            csv_kernel_mapping[key],
            "benchmark-comparison/merge_ci_results.py:CSV_KERNEL_TO_BENCH_DIR",
            None,
        )
    return key, "identity_lowercase", None


# ---------------------------------------------------------------------------
# Size-token rendering/inference
# ---------------------------------------------------------------------------


def render_size_label(entry: WorkflowEntry, token: str) -> str:
    if "{p1}" in entry.flag_template:
        parts = token.split("_")
        replacements = {
            "p1": parts[0] if len(parts) > 0 else "",
            "p2": parts[1] if len(parts) > 1 else "",
            "p3": parts[2] if len(parts) > 2 else "",
        }
        label = entry.size_label_template
        for key, value in replacements.items():
            label = label.replace("{" + key + "}", value)
        return label
    if ":" in token:
        label_size = token.split(":", 1)[1]
        return entry.size_label_template.replace("{size}", label_size)
    return entry.size_label_template.replace("{size}", token)


def label_token_map(entry: WorkflowEntry) -> Dict[str, str]:
    labels: Dict[str, str] = {}
    duplicates: Dict[str, List[str]] = defaultdict(list)
    for token in entry.sizes:
        label = render_size_label(entry, token)
        if label in labels:
            duplicates[label].extend([labels[label], token])
        labels[label] = token
    if duplicates:
        details = "; ".join(
            f"{label!r}: {sorted(set(tokens))}" for label, tokens in sorted(duplicates.items())
        )
        raise PlanError(
            f"workflow benchmark {entry.benchmark!r} at line {entry.workflow_line} has duplicate rendered labels: {details}"
        )
    return labels


def infer_size_token(entry: WorkflowEntry, problem_size: str, known_label_tokens: Dict[str, str]) -> Tuple[str, str]:
    if problem_size in known_label_tokens:
        return known_label_tokens[problem_size], "workflow_matrix_sizes"

    if any(":" in token for token in entry.sizes):
        raise PlanError(
            f"problem_size {problem_size!r} is not present in colon-encoded workflow sizes for {entry.benchmark!r}; "
            "cannot infer the runner's actual_value:label_value token safely"
        )

    token = infer_token_from_label_template(entry, problem_size)
    if not token or len(token.split()) != 1:
        raise PlanError(
            f"problem_size {problem_size!r} for workflow benchmark {entry.benchmark!r} did not produce a single runner size token"
        )
    rendered = render_size_label(entry, token)
    if rendered != problem_size:
        raise PlanError(
            f"inferred size token {token!r} for workflow benchmark {entry.benchmark!r} renders label {rendered!r}, "
            f"not expected problem_size {problem_size!r}"
        )
    return token, "inferred_from_size_label_template"


def infer_token_from_label_template(entry: WorkflowEntry, problem_size: str) -> str:
    if "{p1}" in entry.flag_template:
        return infer_compound_token_from_template(entry, problem_size)
    if "{size}" not in entry.size_label_template:
        raise PlanError(
            f"workflow benchmark {entry.benchmark!r} has no {{size}} placeholder in size_label_template "
            f"{entry.size_label_template!r}"
        )
    return infer_size_placeholder_from_template(entry.size_label_template, problem_size)


def infer_size_placeholder_from_template(template: str, label: str) -> str:
    regex_parts: List[str] = ["^"]
    seen = False
    pos = 0
    placeholder_re = re.compile(r"\{size\}")
    for match in placeholder_re.finditer(template):
        regex_parts.append(re.escape(template[pos:match.start()]))
        if seen:
            regex_parts.append(r"(?P=size)")
        else:
            regex_parts.append(r"(?P<size>.+?)")
            seen = True
        pos = match.end()
    regex_parts.append(re.escape(template[pos:]))
    regex_parts.append("$")
    match = re.match("".join(regex_parts), label)
    if not match:
        raise PlanError(f"problem_size {label!r} does not match size_label_template {template!r}")
    return match.group("size")


def infer_compound_token_from_template(entry: WorkflowEntry, label: str) -> str:
    template = entry.size_label_template
    regex_parts: List[str] = ["^"]
    seen: Dict[str, bool] = {}
    pos = 0
    placeholder_re = re.compile(r"\{(p[123])\}")
    for match in placeholder_re.finditer(template):
        regex_parts.append(re.escape(template[pos:match.start()]))
        name = match.group(1)
        if name in seen:
            regex_parts.append(r"(?P=" + name + r")")
        else:
            regex_parts.append(r"(?P<" + name + r">.+?)")
            seen[name] = True
        pos = match.end()
    regex_parts.append(re.escape(template[pos:]))
    regex_parts.append("$")

    match = re.match("".join(regex_parts), label)
    if not match:
        raise PlanError(f"problem_size {label!r} does not match size_label_template {template!r}")

    required_parts = sorted(set(re.findall(r"\{(p[123])\}", entry.flag_template)))
    missing = [name for name in required_parts if name not in seen]
    if missing:
        raise PlanError(
            f"problem_size {label!r} matches {template!r}, but flag_template {entry.flag_template!r} "
            "requires placeholder(s) not recoverable from the label: "
            + ", ".join(missing)
        )

    return "_".join(match.group(name) for name in required_parts)


# ---------------------------------------------------------------------------
# Plan construction and validation
# ---------------------------------------------------------------------------


def generate_plan(
    comparison_path: Path,
    raw_kernel_timings_path: Path,
    raw_results_path: Path,
    workflow_path: Path,
    merge_script_path: Path,
    archived_plan_path: Path = DEFAULT_ARCHIVED_PLAN,
) -> Dict[str, Any]:
    comparison_rows = load_hardware_rows(comparison_path)
    raw_kernel_rows = load_raw_kernel_timing_rows(raw_kernel_timings_path)
    raw_result_rows = load_raw_result_rows(raw_results_path)
    hardware_rows, raw_merge_summary = merge_comparison_with_raw_timings(comparison_rows, raw_kernel_rows)
    raw_results_summary = validate_raw_results_reconciliation(raw_kernel_rows, raw_result_rows)
    try:
        workflow_entries = load_workflow_entries(workflow_path)
    except PlanError as exc:
        if should_use_archived_plan_fallback(
            comparison_path,
            raw_kernel_timings_path,
            raw_results_path,
            workflow_path,
            merge_script_path,
            archived_plan_path,
            exc,
        ):
            return load_and_validate_archived_plan(
                archived_plan_path,
                comparison_path,
                raw_kernel_timings_path,
                raw_results_path,
                workflow_path,
                merge_script_path,
                hardware_rows,
                raw_kernel_rows,
                raw_merge_summary,
                raw_results_summary,
            )
        raise
    csv_kernel_mapping = load_csv_kernel_mapping(merge_script_path)
    resolved_rows = resolve_rows(hardware_rows, workflow_entries, csv_kernel_mapping)
    plan = build_plan(
        resolved_rows,
        comparison_path,
        raw_kernel_timings_path,
        raw_results_path,
        workflow_path,
        merge_script_path,
        raw_merge_summary,
        raw_results_summary,
    )
    raw_plan_summary = validate_plan_covers_raw_timings(plan, raw_kernel_rows)
    plan["raw_mi300a_timing_coverage"].update(raw_plan_summary)
    plan["validation"]["missing_raw_measured_rows"] = raw_plan_summary["missing_raw_measured_rows"]
    plan["validation"]["skipped_exception_count"] = raw_plan_summary["skipped_exception_count"]
    validate_plan(plan)
    return plan


def resolve_rows(
    hardware_rows: List[HardwareRow],
    workflow_entries: List[WorkflowEntry],
    csv_kernel_mapping: Dict[str, str],
) -> List[ResolvedRow]:
    workflow_by_benchmark: Dict[str, List[WorkflowEntry]] = defaultdict(list)
    for entry in workflow_entries:
        workflow_by_benchmark[entry.benchmark.lower()].append(entry)

    duplicate_workflow_names = {
        name: entries for name, entries in workflow_by_benchmark.items() if len(entries) > 1
    }
    if duplicate_workflow_names:
        details = []
        for name, entries in sorted(duplicate_workflow_names.items()):
            locations = ", ".join(f"{entry.workflow_job}:line {entry.workflow_line}" for entry in entries)
            details.append(f"{name!r} ({locations})")
        raise PlanError("workflow has duplicate full-mode benchmark names: " + "; ".join(details))

    labels_by_workflow = {entry.benchmark.lower(): label_token_map(entry) for entry in workflow_entries}

    resolved: List[ResolvedRow] = []
    errors: List[str] = []
    for row in hardware_rows:
        workflow_name, mapping_source, mapping_reason = resolve_workflow_benchmark_name(
            row.kernel_name, csv_kernel_mapping
        )
        matches = workflow_by_benchmark.get(workflow_name.lower(), [])
        if not matches:
            errors.append(
                f"line {row.csv_line}: hardware row {row.kernel_name!r}/{row.problem_size!r} maps to "
                f"workflow benchmark {workflow_name!r}, but that benchmark is absent from full-mode jobs "
                f"{list(FULL_MODE_WORKFLOW_JOBS)!r}"
            )
            continue
        workflow_entry = matches[0]
        try:
            size_token, size_token_source = infer_size_token(
                workflow_entry,
                row.problem_size,
                labels_by_workflow[workflow_entry.benchmark.lower()],
            )
        except PlanError as exc:
            errors.append(
                f"line {row.csv_line}: hardware row {row.kernel_name!r}/{row.problem_size!r} maps to "
                f"workflow benchmark {workflow_entry.benchmark!r}, but no runnable size token could be resolved: {exc}"
            )
            continue

        resolved.append(
            ResolvedRow(
                hardware=row,
                workflow=workflow_entry,
                size_token=size_token,
                size_token_source=size_token_source,
                mapping_source=mapping_source,
                mapping_reason=mapping_reason,
            )
        )

    if errors:
        raise PlanError("problem-size discovery mapping failed:\n- " + "\n- ".join(errors))
    return resolved


def build_plan(
    resolved_rows: List[ResolvedRow],
    comparison_path: Path,
    raw_kernel_timings_path: Path,
    raw_results_path: Path,
    workflow_path: Path,
    merge_script_path: Path,
    raw_merge_summary: Dict[str, int],
    raw_results_summary: Dict[str, int],
) -> Dict[str, Any]:
    ordered = sorted(
        resolved_rows,
        key=lambda item: (
            item.workflow.tier,
            item.workflow.workflow_matrix_index,
            natural_sort_key(item.hardware.problem_size),
            item.hardware.kernel_name.lower(),
            item.hardware.csv_line,
        ),
    )

    entries: List[Dict[str, Any]] = []
    tier_counts: Counter = Counter()
    token_source_counts: Counter = Counter()
    mapping_source_counts: Counter = Counter()
    workflow_benchmarks = set()
    hardware_kernels = set()

    for plan_index, item in enumerate(ordered, start=1):
        hardware = item.hardware
        workflow = item.workflow
        tier_counts[str(workflow.tier)] += 1
        token_source_counts[item.size_token_source] += 1
        mapping_source_counts[item.mapping_source] += 1
        workflow_benchmarks.add(workflow.benchmark)
        hardware_kernels.add(hardware.kernel_name)

        identity_key = hardware.kernel_name + "\0" + hardware.problem_size
        job_id = make_safe_id("mi300a-discovery", hardware.kernel_name, hardware.problem_size, identity_key)
        artifact_id = make_safe_id(
            "mi300a-discovery-artifact",
            workflow.benchmark,
            hardware.kernel_name,
            hardware.problem_size,
            identity_key,
        )

        entry: Dict[str, Any] = {
            "plan_index": plan_index,
            "benchmark": hardware.kernel_name,
            "problem_size": hardware.problem_size,
            "hardware_kernel_name": hardware.kernel_name,
            "hardware_problem_size": hardware.problem_size,
            "hardware_real_ms": hardware.real_ms,
            "hardware_csv_line": hardware.csv_line,
            "workflow_benchmark": workflow.benchmark,
            "workflow_benchmark_name": workflow.benchmark,
            "workflow_job": workflow.workflow_job,
            "workflow_line": workflow.workflow_line,
            "workflow_matrix_index": workflow.workflow_matrix_index,
            "tier": workflow.tier,
            "tier_name": workflow.tier_name,
            "sizes": item.size_token,
            "size_token": item.size_token,
            "size_label": hardware.problem_size,
            "size_token_source": item.size_token_source,
            "flag_template": workflow.flag_template,
            "size_label_template": workflow.size_label_template,
            "timeout_sec": DISCOVERY_TIMEOUT_SEC,
            "job_id": job_id,
            "artifact_id": artifact_id,
            "kernel_to_workflow_mapping_source": item.mapping_source,
            "kernel_to_workflow_mapping_reason": item.mapping_reason,
        }
        # Discovery jobs intentionally ignore any source full-mode timeout override;
        # every emitted runnable entry carries timeout_sec=3600 and no alternate
        # timeout field.
        for optional_key in OPTIONAL_WORKFLOW_RUNTIME_KEYS:
            value = getattr(workflow, optional_key)
            if value is not None:
                entry[optional_key] = value
        entries.append(entry)

    mapping_overrides = [
        {
            "hardware_kernel_name": kernel,
            "workflow_benchmark_name": workflow_benchmark,
            "reason": reason,
        }
        for kernel, (workflow_benchmark, reason) in sorted(PLAN_ONLY_KERNEL_TO_WORKFLOW_BENCHMARK.items())
    ]

    return {
        "schema_name": PLAN_SCHEMA_NAME,
        "schema_version": PLAN_SCHEMA_VERSION,
        "source_paths": {
            "comparison_csv": repo_relative(comparison_path),
            "raw_kernel_timings_csv": repo_relative(raw_kernel_timings_path),
            "raw_results_csv": repo_relative(raw_results_path),
            "workflow": repo_relative(workflow_path),
            "merge_mapping": repo_relative(merge_script_path),
        },
        "workflow_jobs": list(FULL_MODE_WORKFLOW_JOBS),
        "selection_policy": SELECTION_POLICY,
        "ordering_policy": ORDERING_POLICY,
        "timeout_sec": DISCOVERY_TIMEOUT_SEC,
        "entry_count": len(entries),
        "hardware_row_count": len(entries),
        "hardware_kernel_count": len(hardware_kernels),
        "workflow_benchmark_count": len(workflow_benchmarks),
        "tier_counts": dict(sorted(tier_counts.items())),
        "size_token_source_counts": dict(sorted(token_source_counts.items())),
        "kernel_to_workflow_mapping_source_counts": dict(sorted(mapping_source_counts.items())),
        "mapping_override_count": len(mapping_overrides),
        "mapping_overrides": mapping_overrides,
        "raw_mi300a_timing_coverage": {
            "kernel_timings_csv": repo_relative(raw_kernel_timings_path),
            "results_csv": repo_relative(raw_results_path),
            **raw_merge_summary,
            **raw_results_summary,
            "plan_represented_unique_kernel_problem_size_count": 0,
            "missing_raw_measured_rows": 0,
            "skipped_exception_count": 0,
        },
        "skipped_exception_records": [],
        "validation": {
            "unmapped_hardware_rows": 0,
            "duplicate_job_ids": 0,
            "non_3600_timeouts": 0,
            "non_single_size_entries": 0,
            "missing_raw_measured_rows": 0,
            "skipped_exception_count": 0,
        },
        "entries": entries,
    }


def validate_plan(plan: Dict[str, Any]) -> None:
    entries = plan.get("entries")
    if not isinstance(entries, list):
        raise PlanError(
            "problem-size discovery plan validation failed: entries must be a list"
        )

    errors: List[str] = []
    if plan.get("schema_name") != PLAN_SCHEMA_NAME:
        errors.append("schema_name is invalid")
    if plan.get("schema_version") != PLAN_SCHEMA_VERSION:
        errors.append("schema_version is invalid")
    if plan.get("timeout_sec") != DISCOVERY_TIMEOUT_SEC:
        errors.append(f"top-level timeout_sec must be {DISCOVERY_TIMEOUT_SEC}")
    if plan.get("entry_count") != len(entries):
        errors.append("entry_count does not match entries length")

    raw_coverage = plan.get("raw_mi300a_timing_coverage")
    if not isinstance(raw_coverage, dict):
        errors.append("raw_mi300a_timing_coverage must be present")
    else:
        if raw_coverage.get("missing_raw_measured_rows") != 0:
            errors.append("raw_mi300a_timing_coverage.missing_raw_measured_rows must be 0")
        if raw_coverage.get("missing_result_success_for_same_benchmark_kernel_rows") != 0:
            errors.append(
                "raw_mi300a_timing_coverage.missing_result_success_for_same_benchmark_kernel_rows must be 0"
            )
        represented = raw_coverage.get("plan_represented_unique_kernel_problem_size_count")
        skipped = raw_coverage.get("skipped_exception_count")
        unique = raw_coverage.get("raw_unique_kernel_problem_size_count")
        if not all(isinstance(value, int) for value in (represented, skipped, unique)):
            errors.append("raw_mi300a_timing_coverage represented/skipped/unique counts must be integers")
        elif represented + skipped != unique:
            errors.append(
                "raw_mi300a_timing_coverage represented plus skipped counts must equal unique raw rows"
            )

    job_ids: Counter = Counter()
    artifact_ids: Counter = Counter()
    for idx, entry in enumerate(entries, start=1):
        prefix = f"entries[{idx - 1}]"
        if entry.get("plan_index") != idx:
            errors.append(f"{prefix}.plan_index must be {idx}")
        timeout_sec = entry.get("timeout_sec")
        if timeout_sec != DISCOVERY_TIMEOUT_SEC:
            errors.append(f"{prefix}.timeout_sec must be {DISCOVERY_TIMEOUT_SEC}, got {timeout_sec!r}")
        sizes = entry.get("sizes")
        size_token = entry.get("size_token")
        if not isinstance(sizes, str) or not sizes.strip() or len(sizes.split()) != 1:
            errors.append(f"{prefix}.sizes must be one non-empty runner token string")
        if size_token != sizes:
            errors.append(f"{prefix}.size_token must equal sizes")
        if entry.get("size_label") != entry.get("problem_size"):
            errors.append(f"{prefix}.size_label must equal problem_size")
        job_id = entry.get("job_id")
        artifact_id = entry.get("artifact_id")
        if not isinstance(job_id, str) or not is_safe_id(job_id):
            errors.append(f"{prefix}.job_id is not job-safe: {job_id!r}")
        else:
            job_ids[job_id] += 1
        if not isinstance(artifact_id, str) or not is_safe_id(artifact_id):
            errors.append(f"{prefix}.artifact_id is not artifact-safe: {artifact_id!r}")
        else:
            artifact_ids[artifact_id] += 1

    duplicate_job_ids = sorted(job_id for job_id, count in job_ids.items() if count > 1)
    if duplicate_job_ids:
        errors.append("duplicate job_id values: " + ", ".join(duplicate_job_ids))
    duplicate_artifact_ids = sorted(artifact_id for artifact_id, count in artifact_ids.items() if count > 1)
    if duplicate_artifact_ids:
        errors.append("duplicate artifact_id values: " + ", ".join(duplicate_artifact_ids))

    if errors:
        raise PlanError("problem-size discovery plan validation failed:\n- " + "\n- ".join(errors))


def natural_sort_key(value: str) -> Tuple[Tuple[int, Any, str], ...]:
    parts = []  # type: List[Tuple[int, Any, str]]
    number_re = re.compile(r"[0-9]+(?:\.[0-9]+)?(?:e[+-]?[0-9]+)?", re.IGNORECASE)
    pos = 0
    for match in number_re.finditer(value):
        if match.start() > pos:
            text = value[pos:match.start()].lower()
            if text:
                parts.append((1, text, text))
        raw_number = match.group(0)
        try:
            number_value = float(raw_number)
        except ValueError:
            parts.append((1, raw_number.lower(), raw_number.lower()))
        else:
            parts.append((0, number_value, raw_number))
        pos = match.end()
    if pos < len(value):
        text = value[pos:].lower()
        if text:
            parts.append((1, text, text))
    return tuple(parts)


def safe_slug(value: str) -> str:
    slug = re.sub(r"[^A-Za-z0-9]+", "-", value.lower()).strip("-")
    slug = re.sub(r"-+", "-", slug)
    return slug or "x"


def make_safe_id(prefix: str, *parts: str) -> str:
    if not parts:
        raise ValueError("make_safe_id requires at least one identity part")
    identity = parts[-1]
    human_parts = parts[:-1]
    digest = hashlib.sha256(identity.encode("utf-8")).hexdigest()[:12]
    raw = "-".join([prefix] + [safe_slug(part) for part in human_parts] + [digest])
    return safe_slug(raw)


def is_safe_id(value: str) -> bool:
    return bool(re.fullmatch(r"[a-z0-9][a-z0-9-]*", value))


# ---------------------------------------------------------------------------
# Entrypoint
# ---------------------------------------------------------------------------


def main(argv: Optional[Sequence[str]] = None) -> int:
    args = parse_args(argv)
    comparison_path = Path(args.comparison)
    raw_kernel_timings_path = Path(args.raw_kernel_timings)
    raw_results_path = Path(args.raw_results)
    workflow_path = Path(args.workflow)
    merge_script_path = Path(args.merge_script)
    archived_plan_path = Path(args.archived_plan)

    try:
        plan = generate_plan(
            comparison_path,
            raw_kernel_timings_path,
            raw_results_path,
            workflow_path,
            merge_script_path,
            archived_plan_path,
        )
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
    raise SystemExit(main())
