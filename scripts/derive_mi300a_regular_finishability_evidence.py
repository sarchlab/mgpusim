#!/usr/bin/env python3
"""Derive terminal MI300A regular-workflow finishability evidence.

The command consumes already-downloaded GitHub Actions artifacts for one selected
regular MI300A benchmark run.  It validates that the selected source run is a
terminal GitHub Actions run, joins the materialized regular matrix to benchmark
result CSV artifacts, and emits deterministic JSON and/or Markdown evidence.

It does not contact GitHub, dispatch workflows, run simulators, or fabricate
per-size outcomes.  If staged artifacts contain no per-size failure, timeout, or
cancellation marker for a missing result row, the conservative per-size status
is ``no-result``.
"""

from __future__ import annotations

import argparse
import csv
import json
import re
import sys
from collections import Counter, defaultdict
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Iterable, Mapping, Optional, Sequence

REPO_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_PLAN = REPO_ROOT / "benchmark-comparison" / "mi300a_problem_size_discovery_plan.json"

EVIDENCE_SCHEMA_NAME = "mgpusim.mi300a_regular_finishability_evidence"
EVIDENCE_SCHEMA_VERSION = 2
STATUS_VALUES = ("pass", "fail", "timeout", "cancelled", "no-result")
CLASSIFICATION_VALUES = ("pass", "no-result", "failure", "cancelled")
REGULAR_PER_INVOCATION_TIMEOUT_SEC = 7200
REGULAR_BENCHMARK_JOB_TIMEOUT_SEC = 21600
MERGED_SIM_RESULTS_CANDIDATES = (
    "sim_results_ci.csv",
    "extracted/benchmark-comparison/sim_results_ci.csv",
)
MERGED_SIM_RESULTS_KERNEL_ALIASES = {
    ("bitonicsort", "sort"): ("bitonicsort",),
    ("sssp", "sssp"): ("sssp_relax_kernel",),
    ("bh", "bh"): ("compute_forces_kernel",),
    ("dmr", "dmr"): ("identify_bad_triangles",),
}
EXPLICIT_RESULT_STATUS_COLUMNS = (
    "finishability_status",
    "terminal_status",
    "status",
    "outcome",
    "result",
)
PASS_RESULT_STATUSES = {
    "ok",
    "pass",
    "passed",
    "success",
    "successful",
    "complete",
    "completed",
    "completed_success",
}
FAILURE_RESULT_STATUSES = {
    "error",
    "fail",
    "failed",
    "failure",
    "crash",
    "build_failed",
    "missing_kernel_time",
    "missing_output",
    "missing_sqlite",
}
NO_RESULT_STATUSES = {"missing", "no_result", "no-result", "skip", "skipped"}
TIMEOUT_RESULT_STATUSES = {"timeout", "timed_out", "timed-out"}
CANCELLED_RESULT_STATUSES = {"cancelled", "canceled"}


class EvidenceError(ValueError):
    """Raised when selected-run finishability evidence is invalid."""


def _clean_text(value: object) -> Optional[str]:
    if value is None:
        return None
    text = str(value).strip()
    return text or None


def _json_object(path: Path) -> dict:
    try:
        raw = json.loads(path.read_text(encoding="utf-8"))
    except (json.JSONDecodeError, OSError) as exc:
        raise EvidenceError(f"failed to read JSON object {path}: {exc}") from exc
    if not isinstance(raw, dict):
        raise EvidenceError(f"{path} must contain a JSON object")
    return raw


def _metadata_value(raw: Mapping[str, object], *keys: str) -> Optional[str]:
    for key in keys:
        value = _clean_text(raw.get(key))
        if value is not None:
            return value
    nested = raw.get("run")
    if isinstance(nested, Mapping):
        for key in keys:
            value = _clean_text(nested.get(key))
            if value is not None:
                return value
    return None


def _metadata_bool(raw: Mapping[str, object], key: str) -> bool:
    value = raw.get(key)
    if isinstance(value, bool):
        return value
    nested = raw.get("run")
    if isinstance(nested, Mapping):
        nested_value = nested.get(key)
        if isinstance(nested_value, bool):
            return nested_value
    return False


def load_run_metadata(path: Path, expected_run_id: str | None, expected_head_sha: str | None) -> dict:
    """Load and validate terminal selected-run metadata."""
    raw = _json_object(path)
    run_id = _metadata_value(raw, "databaseId", "database_id", "run_id", "workflow_run_id")
    head_sha = _metadata_value(raw, "headSha", "head_sha", "sha", "commit_sha")
    status = (_metadata_value(raw, "status") or "").lower()
    conclusion = (_metadata_value(raw, "conclusion") or "").lower()
    non_terminal_snapshot = _metadata_bool(raw, "non_terminal_snapshot")

    if not run_id or not head_sha:
        raise EvidenceError(
            f"finishability evidence source {path} requires selected run ID and head SHA"
        )
    if expected_run_id is not None and run_id != str(expected_run_id):
        raise EvidenceError(
            f"finishability evidence source run {run_id} does not match expected run "
            f"{expected_run_id}"
        )
    if expected_head_sha is not None and head_sha != expected_head_sha:
        raise EvidenceError(
            f"finishability evidence source head SHA {head_sha} does not match expected "
            f"head SHA {expected_head_sha}"
        )
    if status != "completed" or non_terminal_snapshot:
        snapshot_note = "; non_terminal_snapshot=true" if non_terminal_snapshot else ""
        raise EvidenceError(
            f"finishability evidence rejected: selected run {run_id} is not terminal "
            f"(status={status or 'missing'}{snapshot_note})"
        )

    return {
        "run_id": run_id,
        "head_sha": head_sha,
        "head_branch": _metadata_value(raw, "headBranch", "head_branch", "branch"),
        "workflow_name": _metadata_value(raw, "workflowName", "workflow_name", "name"),
        "status": status,
        "conclusion": conclusion,
        "event": _metadata_value(raw, "event"),
        "created_at": _metadata_value(raw, "createdAt", "created_at"),
        "updated_at": _metadata_value(raw, "updatedAt", "updated_at"),
        "url": _metadata_value(raw, "url", "html_url"),
        "metadata_path": str(path),
        "terminal": True,
        "non_terminal_snapshot": False,
    }


def _parse_time(value: str | None) -> datetime | None:
    if not value:
        return None
    text = value.strip()
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"
    try:
        return datetime.fromisoformat(text)
    except ValueError:
        return None


def _duration_seconds(started_at: str | None, completed_at: str | None) -> int | None:
    start = _parse_time(started_at)
    end = _parse_time(completed_at)
    if start is None or end is None:
        return None
    if start.tzinfo is None:
        start = start.replace(tzinfo=timezone.utc)
    if end.tzinfo is None:
        end = end.replace(tzinfo=timezone.utc)
    return int((end - start).total_seconds())


def load_jobs(path: Path) -> tuple[list[dict], dict[tuple[str, str], dict]]:
    """Load GitHub job metadata and index benchmark jobs by tier/benchmark."""
    raw = _json_object(path)
    jobs_raw = raw.get("jobs")
    if not isinstance(jobs_raw, list):
        raise EvidenceError(f"{path} must contain a jobs list")

    jobs: list[dict] = []
    benchmark_jobs: dict[tuple[str, str], dict] = {}
    pattern = re.compile(r"^Tier (?P<tier>[12]) Bench: (?P<benchmark>.+)$")
    for job in jobs_raw:
        if not isinstance(job, Mapping):
            continue
        steps = []
        for step in job.get("steps") or []:
            if not isinstance(step, Mapping):
                continue
            steps.append({
                "name": _clean_text(step.get("name")),
                "number": step.get("number"),
                "status": _clean_text(step.get("status")),
                "conclusion": _clean_text(step.get("conclusion")),
                "started_at": _clean_text(step.get("startedAt")),
                "completed_at": _clean_text(step.get("completedAt")),
                "duration_sec": _duration_seconds(
                    _clean_text(step.get("startedAt")),
                    _clean_text(step.get("completedAt")),
                ),
            })
        record = {
            "name": _clean_text(job.get("name")),
            "database_id": job.get("databaseId") or job.get("id"),
            "status": _clean_text(job.get("status")),
            "conclusion": _clean_text(job.get("conclusion")),
            "started_at": _clean_text(job.get("startedAt")),
            "completed_at": _clean_text(job.get("completedAt")),
            "duration_sec": _duration_seconds(
                _clean_text(job.get("startedAt")),
                _clean_text(job.get("completedAt")),
            ),
            "url": _clean_text(job.get("url")),
            "steps": steps,
        }
        jobs.append(record)
        name = record["name"] or ""
        match = pattern.match(name)
        if match:
            key = (f"benchmark-tier{match.group('tier')}", match.group("benchmark"))
            benchmark_jobs[key] = record
    jobs.sort(key=lambda item: (str(item.get("started_at") or ""), str(item.get("name") or "")))
    return jobs, benchmark_jobs


def _expand_ranges(ranges: Sequence[str]) -> set[int]:
    out: set[int] = set()
    for item in ranges:
        text = str(item).strip()
        if not text:
            continue
        if "-" in text:
            left, right = text.split("-", 1)
            start = int(left)
            end = int(right)
            out.update(range(start, end + 1))
        else:
            out.add(int(text))
    return out


def load_plan_entries(path: Path) -> list[dict]:
    raw = _json_object(path)
    entries = raw.get("entries")
    if not isinstance(entries, list):
        raise EvidenceError(f"{path} must contain an entries list")
    records = []
    for entry in entries:
        if not isinstance(entry, Mapping):
            continue
        records.append({
            "plan_index": int(entry["plan_index"]),
            "workflow_job": str(entry["workflow_job"]),
            "workflow_benchmark": str(entry["workflow_benchmark"]),
            "tier": int(entry["tier"]),
            "tier_name": str(entry.get("tier_name") or f"tier{entry['tier']}"),
            "hardware_kernel_name": str(entry["hardware_kernel_name"]),
            "hardware_problem_size": str(entry["hardware_problem_size"]),
            "problem_size": str(entry["problem_size"]),
            "size_label": str(entry["size_label"]),
            "timeout_sec": int(entry.get("timeout_sec") or REGULAR_PER_INVOCATION_TIMEOUT_SEC),
        })
    records.sort(key=lambda item: item["plan_index"])
    return records


def load_matrix_rows(path: Path) -> list[dict]:
    raw = _json_object(path)
    matrix = raw.get("matrix")
    rows = matrix.get("rows") if isinstance(matrix, Mapping) else None
    if not isinstance(rows, Mapping):
        raise EvidenceError(f"{path} must contain matrix.rows")
    out: list[dict] = []
    for workflow_job in ("benchmark-tier1", "benchmark-tier2"):
        job_rows = rows.get(workflow_job)
        if not isinstance(job_rows, list):
            raise EvidenceError(f"{path} missing matrix.rows.{workflow_job}")
        for row in job_rows:
            if not isinstance(row, Mapping):
                continue
            ranges = [str(item) for item in row.get("source_plan_index_ranges") or []]
            indices = _expand_ranges(ranges)
            out.append({
                "workflow_job": workflow_job,
                "benchmark": str(row["benchmark"]),
                "artifact_id": str(row.get("artifact_id") or row["benchmark"]),
                "source_plan_index_ranges": ranges,
                "plan_indices": sorted(indices),
                "plan_entries": int(row.get("plan_entries") or len(indices)),
                "size_count": int(row.get("size_count") or len(indices)),
                "timeout_sec": int(row.get("timeout_sec") or REGULAR_PER_INVOCATION_TIMEOUT_SEC),
                "per_invocation_timeout_sec": int(
                    row.get("per_invocation_timeout_sec") or REGULAR_PER_INVOCATION_TIMEOUT_SEC
                ),
                "benchmark_job_timeout_sec": int(
                    row.get("benchmark_job_timeout_sec") or REGULAR_BENCHMARK_JOB_TIMEOUT_SEC
                ),
                "planned_invocation_count": int(
                    row.get("planned_invocation_count") or len(indices)
                ),
                "exceeds_benchmark_job_timeout": bool(row.get("exceeds_benchmark_job_timeout")),
            })
    return out


def _normalize_kernel(value: str) -> str:
    return str(value).strip().lower()


@dataclass(frozen=True)
class BenchmarkResults:
    """Observed result rows for one workflow benchmark."""

    csv_paths: tuple[str, ...]
    size_rows: Mapping[str, int]
    kernel_size_rows: Mapping[tuple[str, str], int]
    size_evidence: Mapping[str, Mapping[str, object]]
    kernel_size_evidence: Mapping[tuple[str, str], Mapping[str, object]]


@dataclass(frozen=True)
class LoadedBenchmarkResults:
    """Observed result rows plus the staged source layout used to load them."""

    by_benchmark: Mapping[str, BenchmarkResults]
    source_kind: str
    source_paths: tuple[str, ...]


def _normalized_status_token(value: str) -> str:
    return re.sub(r"[^a-z0-9]+", "_", value.strip().lower()).strip("_")


def _row_value_case_insensitive(
    row: Mapping[str, object],
    names: Sequence[str],
) -> Optional[str]:
    values = {str(key).strip().lower(): value for key, value in row.items()}
    for name in names:
        value = _clean_text(values.get(name.lower()))
        if value is not None:
            return value
    return None


def _result_row_evidence(row: Mapping[str, object]) -> dict[str, object]:
    """Return explicit per-size evidence encoded by one result CSV row."""
    raw_status = _row_value_case_insensitive(row, EXPLICIT_RESULT_STATUS_COLUMNS)
    if raw_status is None:
        return {
            "status": "pass",
            "classification": "pass",
            "failure_kind": None,
            "explicit_status": None,
        }

    token = _normalized_status_token(raw_status)
    if token in PASS_RESULT_STATUSES:
        status = "pass"
        classification = "pass"
        failure_kind = None
    elif token in NO_RESULT_STATUSES:
        status = "no-result"
        classification = "no-result"
        failure_kind = None
    elif token in TIMEOUT_RESULT_STATUSES:
        status = "timeout"
        classification = "failure"
        failure_kind = "timeout"
    elif token in FAILURE_RESULT_STATUSES:
        status = "fail"
        classification = "failure"
        failure_kind = token
    elif token in CANCELLED_RESULT_STATUSES:
        status = "cancelled"
        classification = "cancelled"
        failure_kind = None
    else:
        status = "fail"
        classification = "failure"
        failure_kind = f"unrecognized_explicit_status:{token or 'empty'}"

    return {
        "status": status,
        "classification": classification,
        "failure_kind": failure_kind,
        "explicit_status": raw_status,
    }


def _classification_priority(classification: str) -> int:
    return {"pass": 0, "failure": 1, "cancelled": 2}.get(classification, -1)


def _merge_observed_result(
    bucket: dict[object, dict[str, object]],
    key: object,
    row_evidence: Mapping[str, object],
    csv_path: Path,
    *,
    source_kind: str = "benchmark_result_csv_row",
) -> None:
    record = bucket.setdefault(
        key,
        {
            "status": row_evidence["status"],
            "classification": row_evidence["classification"],
            "failure_kind": row_evidence.get("failure_kind"),
            "explicit_status_values": set(),
            "source_row_count": 0,
            "csv_paths": set(),
            "source_kind": source_kind,
        },
    )
    record["source_row_count"] = int(record["source_row_count"]) + 1
    record["csv_paths"].add(str(csv_path))
    explicit_status = row_evidence.get("explicit_status")
    if explicit_status is not None:
        record["explicit_status_values"].add(str(explicit_status))

    current = str(record["classification"])
    candidate = str(row_evidence["classification"])
    if _classification_priority(candidate) > _classification_priority(current):
        record["status"] = row_evidence["status"]
        record["classification"] = candidate
        record["failure_kind"] = row_evidence.get("failure_kind")


def _freeze_observed_evidence(
    evidence: Mapping[object, Mapping[str, object]],
) -> dict[object, dict[str, object]]:
    frozen: dict[object, dict[str, object]] = {}
    for key, record in evidence.items():
        frozen[key] = {
            "status": record["status"],
            "classification": record["classification"],
            "failure_kind": record.get("failure_kind"),
            "explicit_status_values": sorted(record["explicit_status_values"]),
            "source_row_count": int(record["source_row_count"]),
            "csv_paths": tuple(sorted(record["csv_paths"])),
            "source_kind": record.get("source_kind") or "benchmark_result_csv_row",
        }
    return frozen


def _load_extracted_benchmark_results(
    extracted_dir: Path,
    matrix_rows: Sequence[Mapping[str, object]],
) -> dict[str, BenchmarkResults]:
    """Load benchmark result CSV evidence from extracted artifacts."""
    artifact_to_benchmark = {
        str(row["artifact_id"]): str(row["benchmark"])
        for row in matrix_rows
    }
    by_benchmark: dict[str, dict[str, object]] = {}
    for csv_path in sorted(extracted_dir.glob("benchmark-results-*/results_*.csv")):
        parent = csv_path.parent.name
        artifact_id = parent.removeprefix("benchmark-results-")
        benchmark = artifact_to_benchmark.get(artifact_id)
        if benchmark is None:
            # Fallback for fixtures or legacy artifacts with direct benchmark slug.
            stem = csv_path.stem.removeprefix("results_")
            benchmark = stem if stem != "findminsad" else "parboil_sad"
        bucket = by_benchmark.setdefault(
            benchmark,
            {
                "paths": [],
                "size_rows": Counter(),
                "kernel_size_rows": Counter(),
                "size_evidence": {},
                "kernel_size_evidence": {},
            },
        )
        bucket["paths"].append(str(csv_path))
        try:
            with csv_path.open(newline="", encoding="utf-8", errors="replace") as fh:
                reader = csv.DictReader(fh)
                if reader.fieldnames is None:
                    continue
                for row in reader:
                    size = _clean_text(row.get("problem_size"))
                    kernel = _clean_text(row.get("kernel_name"))
                    if not size or not kernel:
                        continue
                    normalized_kernel = _normalize_kernel(kernel)
                    row_evidence = _result_row_evidence(row)
                    bucket["size_rows"][size] += 1
                    bucket["kernel_size_rows"][(normalized_kernel, size)] += 1
                    _merge_observed_result(
                        bucket["size_evidence"],
                        size,
                        row_evidence,
                        csv_path,
                    )
                    _merge_observed_result(
                        bucket["kernel_size_evidence"],
                        (normalized_kernel, size),
                        row_evidence,
                        csv_path,
                    )
        except OSError as exc:
            raise EvidenceError(f"failed to read benchmark result CSV {csv_path}: {exc}") from exc

    out: dict[str, BenchmarkResults] = {}
    for benchmark, bucket in by_benchmark.items():
        out[benchmark] = BenchmarkResults(
            csv_paths=tuple(sorted(bucket["paths"])),
            size_rows=dict(sorted(bucket["size_rows"].items())),
            kernel_size_rows=dict(sorted(bucket["kernel_size_rows"].items())),
            size_evidence=_freeze_observed_evidence(bucket["size_evidence"]),
            kernel_size_evidence=_freeze_observed_evidence(
                bucket["kernel_size_evidence"]
            ),
        )
    return out


def _candidate_merged_sim_kernel_names(entry: Mapping[str, object]) -> tuple[str, ...]:
    """Return normalized kernel names that can represent sim_results_ci rows."""
    workflow_benchmark = _normalize_kernel(str(entry["workflow_benchmark"]))
    hardware_kernel = _normalize_kernel(str(entry["hardware_kernel_name"]))
    candidates = [hardware_kernel]
    for alias in MERGED_SIM_RESULTS_KERNEL_ALIASES.get(
        (workflow_benchmark, hardware_kernel),
        (),
    ):
        normalized_alias = _normalize_kernel(alias)
        if normalized_alias not in candidates:
            candidates.append(normalized_alias)
    return tuple(candidates)


def _load_merged_sim_results(
    sim_results_csv: Path,
    plan_entries: Sequence[Mapping[str, object]],
) -> dict[str, BenchmarkResults]:
    """Load checked-in merged simulator timing evidence by joining it to the plan."""
    entry_lookup: dict[tuple[str, str], list[Mapping[str, object]]] = defaultdict(list)
    for entry in plan_entries:
        size = str(entry["size_label"])
        for kernel in _candidate_merged_sim_kernel_names(entry):
            entry_lookup[(kernel, size)].append(entry)

    by_benchmark: dict[str, dict[str, object]] = {}
    try:
        with sim_results_csv.open(newline="", encoding="utf-8", errors="replace") as fh:
            reader = csv.DictReader(fh)
            if reader.fieldnames is None:
                return {}
            for row in reader:
                size = _clean_text(row.get("problem_size"))
                kernel = _clean_text(row.get("kernel_name"))
                if not size or not kernel:
                    continue
                matched_entries = entry_lookup.get((_normalize_kernel(kernel), size), [])
                if not matched_entries:
                    continue
                row_evidence = _result_row_evidence(row)
                for entry in matched_entries:
                    benchmark = str(entry["workflow_benchmark"])
                    bucket = by_benchmark.setdefault(
                        benchmark,
                        {
                            "paths": [],
                            "size_rows": Counter(),
                            "kernel_size_rows": Counter(),
                            "size_evidence": {},
                            "kernel_size_evidence": {},
                        },
                    )
                    if not bucket["paths"]:
                        bucket["paths"].append(str(sim_results_csv))
                    normalized_plan_kernel = _normalize_kernel(
                        str(entry["hardware_kernel_name"])
                    )
                    bucket["size_rows"][size] += 1
                    bucket["kernel_size_rows"][(normalized_plan_kernel, size)] += 1
                    _merge_observed_result(
                        bucket["size_evidence"],
                        size,
                        row_evidence,
                        sim_results_csv,
                        source_kind="merged_sim_results_ci_row",
                    )
                    _merge_observed_result(
                        bucket["kernel_size_evidence"],
                        (normalized_plan_kernel, size),
                        row_evidence,
                        sim_results_csv,
                        source_kind="merged_sim_results_ci_row",
                    )
    except OSError as exc:
        raise EvidenceError(
            f"failed to read merged simulator result CSV {sim_results_csv}: {exc}"
        ) from exc

    out: dict[str, BenchmarkResults] = {}
    for benchmark, bucket in by_benchmark.items():
        out[benchmark] = BenchmarkResults(
            csv_paths=tuple(sorted(bucket["paths"])),
            size_rows=dict(sorted(bucket["size_rows"].items())),
            kernel_size_rows=dict(sorted(bucket["kernel_size_rows"].items())),
            size_evidence=_freeze_observed_evidence(bucket["size_evidence"]),
            kernel_size_evidence=_freeze_observed_evidence(
                bucket["kernel_size_evidence"]
            ),
        )
    return out


def _find_merged_sim_results_csv(artifact_root: Path) -> Path | None:
    for relative in MERGED_SIM_RESULTS_CANDIDATES:
        candidate = artifact_root / relative
        if candidate.exists():
            return candidate
    return None


def load_benchmark_results(
    artifact_root: Path,
    matrix_rows: Sequence[Mapping[str, object]],
    plan_entries: Sequence[Mapping[str, object]],
) -> LoadedBenchmarkResults:
    """Load result CSV evidence from a full extraction or curated selected-run root."""
    extracted_dir = artifact_root / "extracted"
    extracted_results = _load_extracted_benchmark_results(extracted_dir, matrix_rows)
    if extracted_results:
        paths = sorted(
            {
                path
                for result in extracted_results.values()
                for path in result.csv_paths
            }
        )
        return LoadedBenchmarkResults(
            by_benchmark=extracted_results,
            source_kind="extracted_benchmark_result_csvs",
            source_paths=tuple(paths),
        )

    sim_results_csv = _find_merged_sim_results_csv(artifact_root)
    if sim_results_csv is not None:
        merged_results = _load_merged_sim_results(sim_results_csv, plan_entries)
        return LoadedBenchmarkResults(
            by_benchmark=merged_results,
            source_kind="merged_sim_results_ci_csv",
            source_paths=(str(sim_results_csv),),
        )

    return LoadedBenchmarkResults(
        by_benchmark={},
        source_kind="missing_result_csvs",
        source_paths=(),
    )


def _status_counts(records: Iterable[Mapping[str, object]]) -> dict[str, int]:
    counts = Counter(str(record.get("status")) for record in records)
    return {status: counts.get(status, 0) for status in STATUS_VALUES}


def _classification_counts(records: Iterable[Mapping[str, object]]) -> dict[str, int]:
    counts = Counter(str(record.get("classification")) for record in records)
    return {status: counts.get(status, 0) for status in CLASSIFICATION_VALUES}


def _run_step(job: Mapping[str, object] | None) -> Mapping[str, object] | None:
    if not job:
        return None
    for step in job.get("steps") or []:
        if isinstance(step, Mapping) and step.get("name") == "Run benchmark sizes":
            return step
    return None


def _job_basis(job: Mapping[str, object] | None) -> dict:
    if not job:
        return {
            "job_name": None,
            "job_database_id": None,
            "job_status": "missing",
            "job_conclusion": "missing",
            "job_started_at": None,
            "job_completed_at": None,
            "job_duration_sec": None,
            "job_terminal": False,
            "job_url": None,
            "run_benchmark_sizes_step_status": "missing",
            "run_benchmark_sizes_step_conclusion": "missing",
            "run_benchmark_sizes_step_started_at": None,
            "run_benchmark_sizes_step_completed_at": None,
            "run_benchmark_sizes_step_duration_sec": None,
        }
    step = _run_step(job)
    job_status = _clean_text(job.get("status"))
    job_conclusion = _clean_text(job.get("conclusion"))
    return {
        "job_name": job.get("name"),
        "job_database_id": job.get("database_id"),
        "job_status": job_status,
        "job_conclusion": job_conclusion,
        "job_started_at": job.get("started_at"),
        "job_completed_at": job.get("completed_at"),
        "job_duration_sec": job.get("duration_sec"),
        "job_terminal": job_status == "completed" or bool(job_conclusion),
        "job_url": job.get("url"),
        "run_benchmark_sizes_step_status": step.get("status") if step else None,
        "run_benchmark_sizes_step_conclusion": step.get("conclusion") if step else None,
        "run_benchmark_sizes_step_started_at": step.get("started_at") if step else None,
        "run_benchmark_sizes_step_completed_at": step.get("completed_at") if step else None,
        "run_benchmark_sizes_step_duration_sec": step.get("duration_sec") if step else None,
    }


def _size_has_multiple_kernels(entries_for_row: Sequence[Mapping[str, object]]) -> dict[str, bool]:
    kernels_by_size: dict[str, set[str]] = defaultdict(set)
    for entry in entries_for_row:
        kernels_by_size[str(entry["size_label"])].add(
            _normalize_kernel(str(entry["hardware_kernel_name"]))
        )
    return {size: len(kernels) > 1 for size, kernels in kernels_by_size.items()}


def _entry_observed_evidence(
    entry: Mapping[str, object],
    row_entries: Sequence[Mapping[str, object]],
    benchmark_results: BenchmarkResults | None,
) -> Mapping[str, object] | None:
    if benchmark_results is None:
        return None
    size = str(entry["size_label"])
    multi_kernel_size = _size_has_multiple_kernels(row_entries).get(size, False)
    if multi_kernel_size:
        key = (_normalize_kernel(str(entry["hardware_kernel_name"])), size)
        return benchmark_results.kernel_size_evidence.get(key)
    return benchmark_results.size_evidence.get(size)


def _job_terminal_classification(job_basis: Mapping[str, object]) -> str:
    values = [
        _normalized_status_token(str(job_basis.get("job_conclusion") or "")),
        _normalized_status_token(
            str(job_basis.get("run_benchmark_sizes_step_conclusion") or "")
        ),
    ]
    if any(value in CANCELLED_RESULT_STATUSES for value in values):
        return "cancelled"
    if any(
        value in FAILURE_RESULT_STATUSES or value in TIMEOUT_RESULT_STATUSES
        for value in values
    ):
        return "failure"
    if any(value in PASS_RESULT_STATUSES for value in values):
        return "pass"
    return "no-result"


def _row_evidence_classification(status_counts: Mapping[str, int]) -> str:
    if status_counts.get("cancelled", 0):
        return "cancelled"
    if status_counts.get("fail", 0) or status_counts.get("timeout", 0):
        return "failure"
    if status_counts.get("no-result", 0):
        return "no-result"
    if status_counts.get("pass", 0):
        return "pass"
    return "no-result"


def _entry_provenance(
    observed_evidence: Mapping[str, object] | None,
    job_basis: Mapping[str, object],
) -> dict[str, object]:
    if observed_evidence is None:
        source = "missing_result_row"
        result_csv_paths: list[str] = []
        matched_rows = 0
        explicit_values: list[str] = []
        failure_kind = None
    else:
        source = str(observed_evidence.get("source_kind") or "benchmark_result_csv_row")
        result_csv_paths = list(observed_evidence.get("csv_paths") or [])
        matched_rows = int(observed_evidence.get("source_row_count") or 0)
        explicit_values = list(observed_evidence.get("explicit_status_values") or [])
        failure_kind = observed_evidence.get("failure_kind")

    return {
        "source": source,
        "result_csv_paths": result_csv_paths,
        "matched_result_row_count": matched_rows,
        "explicit_status_values": explicit_values,
        "failure_kind": failure_kind,
        "job_database_id": job_basis.get("job_database_id"),
        "job_name": job_basis.get("job_name"),
        "job_status": job_basis.get("job_status"),
        "job_conclusion": job_basis.get("job_conclusion"),
        "job_terminal": job_basis.get("job_terminal"),
        "run_benchmark_sizes_step_status": job_basis.get(
            "run_benchmark_sizes_step_status"
        ),
        "run_benchmark_sizes_step_conclusion": job_basis.get(
            "run_benchmark_sizes_step_conclusion"
        ),
    }


def _observed_source_label(observed_evidence: Mapping[str, object]) -> str:
    source_kind = str(observed_evidence.get("source_kind") or "benchmark_result_csv_row")
    if source_kind == "merged_sim_results_ci_row":
        return "merged simulator timing CSV row"
    return "benchmark result CSV row"


def _entry_basis(
    entry: Mapping[str, object],
    classification: str,
    observed_evidence: Mapping[str, object] | None,
    job_basis: Mapping[str, object],
) -> str:
    if observed_evidence is not None:
        paths = ", ".join(observed_evidence.get("csv_paths") or ["<missing>"])
        source_label = _observed_source_label(observed_evidence)
        if classification == "pass":
            return (
                f"{source_label} emitted for size "
                f"{entry['size_label']} in {paths}"
            )
        explicit_values = ", ".join(
            observed_evidence.get("explicit_status_values") or ["<missing>"]
        )
        failure_kind = observed_evidence.get("failure_kind") or "none"
        return (
            f"explicit per-size {classification} marker for size "
            f"{entry['size_label']} in {paths}; source={source_label}; "
            f"explicit_status={explicit_values}; failure_kind={failure_kind}"
        )

    conclusion = job_basis.get("job_conclusion") or "missing"
    step_conclusion = job_basis.get("run_benchmark_sizes_step_conclusion") or "missing"
    return (
        "no matching result or simulator timing CSV row in staged artifacts; "
        f"benchmark job conclusion={conclusion}, run step conclusion={step_conclusion}; "
        "missing rows remain no-result unless explicit per-size evidence marks "
        "pass, failure, timeout, or cancellation"
    )


def _per_job_summary(row_records: Sequence[Mapping[str, object]]) -> list[dict[str, object]]:
    summaries = []
    for row in row_records:
        basis = row["job_basis"]
        status_counts = row["status_counts"]
        summaries.append({
            "workflow_job": row["workflow_job"],
            "tier": row["tier"],
            "benchmark": row["benchmark"],
            "artifact_id": row["artifact_id"],
            "plan_entries": row["plan_entries"],
            "size_count": row["size_count"],
            "terminal_classification": row["terminal_classification"],
            "evidence_classification": _row_evidence_classification(status_counts),
            "status_counts": status_counts,
            "classification_counts": row["classification_counts"],
            "result_csv_paths": row["result_csv_paths"],
            "source_plan_index_ranges": row["source_plan_index_ranges"],
            "job_terminal": basis.get("job_terminal"),
            "job_status": basis.get("job_status"),
            "job_conclusion": basis.get("job_conclusion"),
            "run_benchmark_sizes_step_status": basis.get(
                "run_benchmark_sizes_step_status"
            ),
            "run_benchmark_sizes_step_conclusion": basis.get(
                "run_benchmark_sizes_step_conclusion"
            ),
            "provenance": {
                "job_database_id": basis.get("job_database_id"),
                "job_name": basis.get("job_name"),
                "job_url": basis.get("job_url"),
                "job_started_at": basis.get("job_started_at"),
                "job_completed_at": basis.get("job_completed_at"),
                "job_duration_sec": basis.get("job_duration_sec"),
                "run_benchmark_sizes_step_started_at": basis.get(
                    "run_benchmark_sizes_step_started_at"
                ),
                "run_benchmark_sizes_step_completed_at": basis.get(
                    "run_benchmark_sizes_step_completed_at"
                ),
                "run_benchmark_sizes_step_duration_sec": basis.get(
                    "run_benchmark_sizes_step_duration_sec"
                ),
            },
        })
    return summaries


def _per_size_summary(row_records: Sequence[Mapping[str, object]]) -> list[dict[str, object]]:
    summaries = []
    for row in row_records:
        basis = row["job_basis"]
        for entry in row["entries"]:
            summaries.append({
                "plan_index": entry["plan_index"],
                "workflow_job": entry["workflow_job"],
                "tier": entry["tier"],
                "tier_name": entry["tier_name"],
                "benchmark": entry["benchmark"],
                "hardware_kernel_name": entry["hardware_kernel_name"],
                "problem_size": entry["problem_size"],
                "size_label": entry["size_label"],
                "status": entry["status"],
                "classification": entry["classification"],
                "terminal_classification": row["terminal_classification"],
                "job_status": basis.get("job_status"),
                "job_conclusion": basis.get("job_conclusion"),
                "job_terminal": basis.get("job_terminal"),
                "run_benchmark_sizes_step_conclusion": basis.get(
                    "run_benchmark_sizes_step_conclusion"
                ),
                "provenance": entry["provenance"],
            })
    return summaries


def derive_evidence(
    *,
    artifact_root: Path,
    plan_path: Path,
    run_metadata_path: Path,
    jobs_path: Path,
    matrix_report_path: Path,
    expected_run_id: str | None = None,
    expected_head_sha: str | None = None,
) -> dict:
    """Build deterministic finishability evidence from staged artifacts."""
    run = load_run_metadata(run_metadata_path, expected_run_id, expected_head_sha)
    jobs, benchmark_jobs = load_jobs(jobs_path)
    plan_entries = load_plan_entries(plan_path)
    plan_by_index = {entry["plan_index"]: entry for entry in plan_entries}
    matrix_rows = load_matrix_rows(matrix_report_path)
    loaded_results = load_benchmark_results(artifact_root, matrix_rows, plan_entries)
    results = loaded_results.by_benchmark

    row_records = []
    all_entry_records = []
    for row in matrix_rows:
        workflow_job = str(row["workflow_job"])
        benchmark = str(row["benchmark"])
        row_entries = [plan_by_index[index] for index in row["plan_indices"]]
        row_results = results.get(benchmark)
        job = benchmark_jobs.get((workflow_job, benchmark))
        basis = _job_basis(job)
        entry_records = []
        for entry in row_entries:
            observed = _entry_observed_evidence(entry, row_entries, row_results)
            if observed is None:
                status = "no-result"
                classification = "no-result"
            else:
                status = str(observed["status"])
                classification = str(observed["classification"])
            provenance = _entry_provenance(observed, basis)
            record = {
                "plan_index": entry["plan_index"],
                "workflow_job": workflow_job,
                "tier": entry["tier"],
                "tier_name": entry["tier_name"],
                "benchmark": benchmark,
                "hardware_kernel_name": entry["hardware_kernel_name"],
                "problem_size": entry["hardware_problem_size"],
                "size_label": entry["size_label"],
                "status": status,
                "classification": classification,
                "basis": _entry_basis(entry, classification, observed, basis),
                "provenance": provenance,
                "result_csv_paths": provenance["result_csv_paths"],
                "matched_result_row_count": provenance["matched_result_row_count"],
                "explicit_status_values": provenance["explicit_status_values"],
                "failure_kind": provenance["failure_kind"],
            }
            entry_records.append(record)
            all_entry_records.append(record)

        row_counts = _status_counts(entry_records)
        row_classification_counts = _classification_counts(entry_records)
        terminal_classification = _job_terminal_classification(basis)
        row_records.append({
            "workflow_job": workflow_job,
            "tier": 1 if workflow_job == "benchmark-tier1" else 2,
            "benchmark": benchmark,
            "artifact_id": row["artifact_id"],
            "source_plan_index_ranges": row["source_plan_index_ranges"],
            "plan_entries": row["plan_entries"],
            "size_count": row["size_count"],
            "timeout_sec": row["timeout_sec"],
            "per_invocation_timeout_sec": row["per_invocation_timeout_sec"],
            "benchmark_job_timeout_sec": row["benchmark_job_timeout_sec"],
            "planned_invocation_count": row["planned_invocation_count"],
            "exceeds_benchmark_job_timeout": row["exceeds_benchmark_job_timeout"],
            "job_basis": basis,
            "terminal_classification": terminal_classification,
            "evidence_classification": _row_evidence_classification(row_counts),
            "result_csv_paths": list(row_results.csv_paths) if row_results else [],
            "status_counts": row_counts,
            "classification_counts": row_classification_counts,
            "entries": entry_records,
        })

    job_conclusions = Counter(str(job.get("conclusion") or "missing") for job in jobs)
    benchmark_job_conclusions = Counter(
        str(job.get("conclusion") or "missing") for job in benchmark_jobs.values()
    )
    total_counts = _status_counts(all_entry_records)
    total_classification_counts = _classification_counts(all_entry_records)
    rows_by_tier = Counter(str(row["workflow_job"]) for row in row_records)
    entries_by_tier = Counter(str(entry["workflow_job"]) for entry in all_entry_records)
    terminal_job_counts_raw = Counter(
        str(row["terminal_classification"]) for row in row_records
    )
    terminal_job_classification_counts = {
        status: terminal_job_counts_raw.get(status, 0)
        for status in CLASSIFICATION_VALUES
    }
    source_paths = {
        "artifact_root": str(artifact_root),
        "plan": str(plan_path),
        "run_metadata": str(run_metadata_path),
        "jobs": str(jobs_path),
        "regular_matrix_report": str(matrix_report_path),
        "benchmark_results_dir": str(artifact_root / "extracted"),
        "result_source_kind": loaded_results.source_kind,
        "result_source_paths": list(loaded_results.source_paths),
    }
    per_job = _per_job_summary(row_records)
    per_size = _per_size_summary(row_records)

    return {
        "schema_name": EVIDENCE_SCHEMA_NAME,
        "schema_version": EVIDENCE_SCHEMA_VERSION,
        "source": source_paths,
        "provenance": {
            "terminal_run": {
                "run_id": run["run_id"],
                "head_sha": run["head_sha"],
                "status": run["status"],
                "conclusion": run.get("conclusion"),
                "terminal": run["terminal"],
                "non_terminal_snapshot": run["non_terminal_snapshot"],
                "metadata_path": run["metadata_path"],
            },
            "source_paths": source_paths,
            "result_matching_policy": (
                "join each regular plan entry to staged per-benchmark result CSV rows "
                "when present, otherwise to the checked-in merged sim_results_ci.csv "
                "timing rows; match by problem size, and by kernel plus size when "
                "one matrix row contains multiple kernels for the same size"
            ),
            "missing_row_policy": (
                "missing planned rows remain no-result unless explicit per-size "
                "artifact evidence marks pass, failure, timeout, or cancellation"
            ),
        },
        "classification_policy": {
            "status_values": list(STATUS_VALUES),
            "classification_values": list(CLASSIFICATION_VALUES),
            "pass": (
                "a staged per-benchmark result CSV row or checked-in merged simulator "
                "timing row was emitted for the matrix entry's problem size"
            ),
            "fail": (
                "reserved for explicit per-size non-timeout failure markers; missing "
                "rows are not relabeled as fail from job-level failure alone"
            ),
            "failure": (
                "per-size classification for explicit failure or timeout markers; "
                "job-level failure is recorded separately as terminal_classification"
            ),
            "cancelled": (
                "per-size classification only when explicit cancellation evidence is "
                "present; job-level cancellation is recorded separately"
            ),
            "timeout": (
                "reserved for explicit per-size timeout markers; timeout markers map "
                "to failure classification with failure_kind=timeout"
            ),
            "no-result": (
                "no matching result or simulator timing CSV row was present; with "
                "artifacts-only evidence and no logs, missing rows are not relabeled "
                "as fail or timeout"
            ),
        },
        "run": run,
        "terminal": {
            "run_terminal": run["terminal"],
            "run_status": run["status"],
            "run_conclusion": run.get("conclusion"),
            "terminal_job_classification_counts": terminal_job_classification_counts,
        },
        "job_basis": {
            "total_jobs": len(jobs),
            "job_conclusions": dict(sorted(job_conclusions.items())),
            "benchmark_jobs": len(benchmark_jobs),
            "benchmark_job_conclusions": dict(sorted(benchmark_job_conclusions.items())),
            "benchmark_job_terminal_classification_counts": terminal_job_classification_counts,
        },
        "matrix": {
            "regular_matrix_rows": len(row_records),
            "regular_plan_entries": len(all_entry_records),
            "rows_by_workflow_job": dict(sorted(rows_by_tier.items())),
            "entries_by_workflow_job": dict(sorted(entries_by_tier.items())),
        },
        "status_counts": total_counts,
        "classification_counts": total_classification_counts,
        "summaries": {
            "per_job": per_job,
            "per_size": per_size,
            "terminal_job_classification_counts": terminal_job_classification_counts,
            "classification_counts": total_classification_counts,
        },
        "per_job_summary": per_job,
        "per_size_summary": per_size,
        "rows": row_records,
    }


def _format_ranges(ranges: Sequence[str]) -> str:
    return ", ".join(f"`{item}`" for item in ranges)


def _status_count_text(counts: Mapping[str, int]) -> str:
    return ", ".join(f"{status}={counts.get(status, 0)}" for status in STATUS_VALUES)


def _classification_count_text(counts: Mapping[str, int]) -> str:
    return ", ".join(
        f"{status}={counts.get(status, 0)}" for status in CLASSIFICATION_VALUES
    )


def _count_map_text(counts: Mapping[str, int]) -> str:
    return ", ".join(f"{key}={counts[key]}" for key in sorted(counts))


def render_markdown(report: Mapping[str, object]) -> str:
    """Render human-readable deterministic evidence Markdown."""
    run = report["run"]
    job_basis = report["job_basis"]
    matrix = report["matrix"]
    source = report["source"]
    status_counts = report["status_counts"]
    classification_counts = report.get("classification_counts", {})
    terminal_counts = report.get("terminal", {}).get(
        "terminal_job_classification_counts",
        {},
    )
    result_source_paths = ", ".join(
        f"`{path}`" for path in source.get("result_source_paths") or ["<none>"]
    )
    lines = [
        f"# MI300A Regular Run {run['run_id']} Finishability Evidence",
        "",
        "This document records terminal, artifacts-only finishability evidence for the",
        "selected regular MI300A benchmark run. It does not contact GitHub, dispatch",
        "workflows, rerun simulators, or infer timeout/crash labels from absent logs.",
        "",
        "## Source run",
        "",
        f"- Run ID: `{run['run_id']}`",
        f"- Head SHA: `{run['head_sha']}`",
        f"- Head branch: `{run.get('head_branch') or 'unknown'}`",
        f"- Workflow: `{run.get('workflow_name') or 'unknown'}`",
        f"- Terminal status/conclusion: `{run['status']}` / `{run.get('conclusion') or 'unknown'}`",
        f"- Run URL: {run.get('url') or 'unknown'}",
        f"- Metadata: `{source['run_metadata']}`",
        "",
        "## Artifact and job basis",
        "",
        f"- Artifact root: `{source['artifact_root']}`",
        f"- Regular matrix report: `{source['regular_matrix_report']}`",
        f"- Jobs metadata: `{source['jobs']}`",
        f"- Plan: `{source['plan']}`",
        f"- Result source kind: `{source.get('result_source_kind') or 'unknown'}`",
        f"- Result source paths: {result_source_paths}",
        f"- Total jobs in metadata: `{job_basis['total_jobs']}` "
        f"({_count_map_text(job_basis['job_conclusions'])})",
        f"- Benchmark jobs: `{job_basis['benchmark_jobs']}` "
        f"({_count_map_text(job_basis['benchmark_job_conclusions'])})",
        "- Benchmark job terminal classifications: "
        f"{_classification_count_text(terminal_counts)}",
        f"- Regular matrix rows: `{matrix['regular_matrix_rows']}`; "
        f"plan entries: `{matrix['regular_plan_entries']}`",
        "",
        "## Classification policy",
        "",
        "- Per-size `status` is derived only from explicit per-size artifacts:",
        "`pass` for an emitted benchmark result row or checked-in merged",
        "simulator timing row, `fail`/`timeout`/`cancelled` only for explicit",
        "per-size markers, and `no-result` when no matching row exists.",
        "- Per-size `classification` uses the bounded vocabulary",
        "`pass`, `no-result`, `failure`, `cancelled`; explicit timeout markers",
        "are classified as `failure` with `failure_kind=timeout`.",
        "- Job-level failure or cancellation is recorded as",
        "`terminal_classification` in per-job summaries. It does not relabel",
        "missing per-size rows away from `no-result`.",
        "",
        "## Overall per-size status counts",
        "",
        "| Status | Count |",
        "| --- | ---: |",
    ]
    for status in STATUS_VALUES:
        lines.append(f"| {status} | {status_counts.get(status, 0)} |")

    lines.extend([
        "",
        "## Overall per-size classification counts",
        "",
        "| Classification | Count |",
        "| --- | ---: |",
    ])
    for status in CLASSIFICATION_VALUES:
        lines.append(f"| {status} | {classification_counts.get(status, 0)} |")

    lines.extend([
        "",
        "## Per-job summary",
        "",
        "| Workflow job | Benchmark | Plan index ranges | Terminal class | "
        "Evidence class | Job conclusion | Run-step conclusion | Status counts |",
        "| --- | --- | --- | --- | --- | --- | --- | --- |",
    ])
    for row in report["rows"]:
        basis = row["job_basis"]
        lines.append(
            f"| {row['workflow_job']} | {row['benchmark']} | "
            f"{_format_ranges(row['source_plan_index_ranges'])} | "
            f"`{row.get('terminal_classification')}` | "
            f"`{row.get('evidence_classification')}` | "
            f"`{basis.get('job_conclusion')}` | "
            f"`{basis.get('run_benchmark_sizes_step_conclusion')}` | "
            f"{_status_count_text(row['status_counts'])} |"
        )

    lines.extend([
        "",
        "## Per-size summary",
        "",
        "| Plan index | Workflow job | Benchmark | Kernel | Problem size | "
        "Status | Classification | Terminal class | Matched rows | Basis |",
        "| ---: | --- | --- | --- | --- | --- | --- | --- | ---: | --- |",
    ])
    for row in report["rows"]:
        for entry in row["entries"]:
            basis = str(entry["basis"]).replace("|", "\\|")
            lines.append(
                f"| {entry['plan_index']} | {entry['workflow_job']} | "
                f"{entry['benchmark']} | {entry['hardware_kernel_name']} | "
                f"{entry['problem_size']} | `{entry['status']}` | "
                f"`{entry['classification']}` | "
                f"`{row.get('terminal_classification')}` | "
                f"{entry.get('matched_result_row_count', 0)} | {basis} |"
            )

    lines.append("")
    return "\n".join(lines)


def _default_path(
    artifact_root: Path,
    primary_relative: str,
    *fallback_relatives: str,
) -> Path:
    """Resolve a default artifact path while preserving the legacy primary layout."""
    for relative in (primary_relative, *fallback_relatives):
        candidate = artifact_root / relative
        if candidate.exists():
            return candidate
    return artifact_root / primary_relative


def parse_args(argv: Iterable[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--artifact-root", required=True, help="selected run artifact root")
    parser.add_argument("--plan", default=str(DEFAULT_PLAN), help="regular problem-size plan JSON")
    parser.add_argument(
        "--run-metadata-json",
        default=None,
        help=(
            "selected run metadata JSON "
            "(default: inventory/run-view.json, then run-view.json)"
        ),
    )
    parser.add_argument(
        "--jobs-json",
        default=None,
        help="selected run jobs JSON (default: inventory/jobs.json, then jobs.json)",
    )
    parser.add_argument(
        "--regular-matrix-report",
        default=None,
        help=(
            "regular matrix validation JSON (default: "
            "extracted/validation-summary/regular_matrix_validation.json, "
            "then regular_matrix_validation.json)"
        ),
    )
    parser.add_argument("--expected-run-id", default=None, help="required selected run ID")
    parser.add_argument("--expected-head-sha", default=None, help="required selected head SHA")
    parser.add_argument("--output-json", default=None, help="write deterministic JSON report")
    parser.add_argument("--output-md", default=None, help="write deterministic Markdown report")
    parser.add_argument(
        "--markdown-only",
        action="store_true",
        help="print Markdown to stdout instead of JSON when no output file is requested",
    )
    return parser.parse_args(list(argv) if argv is not None else None)


def main(argv: Iterable[str] | None = None) -> int:
    args = parse_args(argv)
    artifact_root = Path(args.artifact_root)
    run_metadata_path = Path(args.run_metadata_json) if args.run_metadata_json else _default_path(
        artifact_root, "inventory/run-view.json", "run-view.json"
    )
    jobs_path = Path(args.jobs_json) if args.jobs_json else _default_path(
        artifact_root, "inventory/jobs.json", "jobs.json"
    )
    matrix_report_path = Path(args.regular_matrix_report) if args.regular_matrix_report else _default_path(
        artifact_root,
        "extracted/validation-summary/regular_matrix_validation.json",
        "regular_matrix_validation.json",
    )

    try:
        report = derive_evidence(
            artifact_root=artifact_root,
            plan_path=Path(args.plan),
            run_metadata_path=run_metadata_path,
            jobs_path=jobs_path,
            matrix_report_path=matrix_report_path,
            expected_run_id=args.expected_run_id,
            expected_head_sha=args.expected_head_sha,
        )
    except EvidenceError as exc:
        print(f"FAIL: MI300A regular finishability evidence rejected input: {exc}", file=sys.stderr)
        return 2

    json_text = json.dumps(report, indent=2, sort_keys=True) + "\n"
    markdown = None
    if args.output_md or args.markdown_only:
        markdown = render_markdown(report)

    if args.output_json:
        output_json = Path(args.output_json)
        output_json.parent.mkdir(parents=True, exist_ok=True)
        output_json.write_text(json_text, encoding="utf-8")
    if args.output_md:
        output_md = Path(args.output_md)
        output_md.parent.mkdir(parents=True, exist_ok=True)
        output_md.write_text(markdown if markdown is not None else render_markdown(report), encoding="utf-8")

    if args.markdown_only and not args.output_md:
        print(markdown if markdown is not None else render_markdown(report), end="")
    elif not args.output_json:
        print(json_text, end="")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
