#!/usr/bin/env python3
"""Validate a terminal-discovery row timeout contract without dispatching work.

The terminal-discovery matrix groups many per-size attempts into one visible
benchmark row.  A candidate operator must only dispatch rows whose worst-case
sequential work fits under the row/job wall-clock cap:

    required_timeout_sec = attempt_count * per_attempt_timeout_sec + row_overhead_sec

This guard is intentionally static: it reads the checked-in shard manifest and
matrix JSON files, reports rows that cannot fit, and never runs simulators or
contacts GitHub.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any, Mapping, Optional, Sequence

import generate_mi300a_terminal_discovery_shards as generator


VALIDATION_SCHEMA = "mgpusim.mi300a_terminal_discovery_timeout_contract_validation"
SCHEMA_VERSION = 1
DEFAULT_MAX_VIOLATIONS = 20
CONTRACT_FORMULA = "required_timeout_sec = attempt_count * per_attempt_timeout_sec + row_overhead_sec"


class TimeoutContractError(Exception):
    """Raised when timeout-contract inputs are malformed."""


# ---------------------------------------------------------------------------
# Generic path/JSON helpers
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
        raise TimeoutContractError(f"{repo_relative(path)}: file not found") from exc
    except json.JSONDecodeError as exc:
        raise TimeoutContractError(f"{repo_relative(path)}: invalid JSON: {exc}") from exc
    if not isinstance(data, Mapping):
        raise TimeoutContractError(f"{repo_relative(path)}: JSON root must be an object")
    return data


def object_at(value: Any, label: str) -> Mapping[str, Any]:
    if not isinstance(value, Mapping):
        raise TimeoutContractError(f"{label} must be an object")
    return value


def list_at(value: Any, label: str) -> list[Any]:
    if not isinstance(value, list):
        raise TimeoutContractError(f"{label} must be a list")
    return value


def as_int(value: Any, label: str) -> int:
    try:
        return generator.as_int(value, label)
    except generator.ShardManifestError as exc:
        raise TimeoutContractError(str(exc)) from exc


def nonempty_str(value: Any, label: str) -> str:
    try:
        return generator.nonempty_str(value, label)
    except generator.ShardManifestError as exc:
        raise TimeoutContractError(str(exc)) from exc


def positive_int_arg(value: str) -> int:
    try:
        parsed = int(value)
    except ValueError as exc:
        raise argparse.ArgumentTypeError(f"{value!r} is not an integer") from exc
    if parsed < 1:
        raise argparse.ArgumentTypeError("value must be a positive integer")
    return parsed


def nonnegative_int_arg(value: str) -> int:
    try:
        parsed = int(value)
    except ValueError as exc:
        raise argparse.ArgumentTypeError(f"{value!r} is not an integer") from exc
    if parsed < 0:
        raise argparse.ArgumentTypeError("value must be a non-negative integer")
    return parsed


# ---------------------------------------------------------------------------
# Timeout-contract validation
# ---------------------------------------------------------------------------


def matrix_path_from_shard(shard: Mapping[str, Any], shard_dir: Path, label: str) -> Path:
    matrix_path = generator.str_or_none(shard.get("matrix_path"))
    if matrix_path is not None:
        return repo_path(matrix_path)
    shard_index = as_int(shard.get("shard_index"), f"{label}.shard_index")
    return shard_dir / generator.matrix_filename(shard_index)


def row_contract_record(
    *,
    matrix_path: Path,
    matrix: Mapping[str, Any],
    row: Mapping[str, Any],
    row_offset: int,
    per_attempt_timeout_sec: int,
    row_timeout_sec: int,
    row_overhead_sec: int,
) -> dict[str, Any]:
    label = f"{repo_relative(matrix_path)}.include[{row_offset}]"
    attempts = [
        object_at(attempt, f"{label}.attempts[{offset}]")
        for offset, attempt in enumerate(list_at(row.get("attempts"), f"{label}.attempts"))
    ]
    attempt_count = as_int(row.get("attempt_count"), f"{label}.attempt_count")
    if attempt_count != len(attempts):
        raise TimeoutContractError(
            f"{label}.attempt_count must match attempts length ({attempt_count} != {len(attempts)})"
        )
    if attempt_count < 1:
        raise TimeoutContractError(f"{label}.attempt_count must be positive")

    source_timeout_values = sorted(
        {
            as_int(
                attempt.get("timeout_sec"),
                f"{label}.attempts[{attempt_offset}].timeout_sec",
            )
            for attempt_offset, attempt in enumerate(attempts)
        }
    )
    plan_indices = [
        as_int(attempt.get("plan_index"), f"{label}.attempts[{offset}].plan_index")
        for offset, attempt in enumerate(attempts)
    ]
    attempt_budget_sec = attempt_count * per_attempt_timeout_sec
    required_timeout_sec = attempt_budget_sec + row_overhead_sec
    slack_sec = row_timeout_sec - required_timeout_sec
    return {
        "matrix_path": repo_relative(matrix_path),
        "shard_index": as_int(
            row.get("shard_index", matrix.get("shard_index")),
            f"{label}.shard_index",
        ),
        "matrix_row_index": row_offset + 1,
        "matrix_row_id": nonempty_str(row.get("matrix_row_id"), f"{label}.matrix_row_id"),
        "benchmark": nonempty_str(row.get("benchmark"), f"{label}.benchmark"),
        "workflow_benchmark": nonempty_str(
            row.get("workflow_benchmark") or row.get("benchmark"),
            f"{label}.workflow_benchmark",
        ),
        "attempt_count": attempt_count,
        "plan_index_start": plan_indices[0],
        "plan_index_end": plan_indices[-1],
        "source_attempt_timeout_sec_values": source_timeout_values,
        "per_attempt_timeout_sec": per_attempt_timeout_sec,
        "attempt_budget_sec": attempt_budget_sec,
        "row_overhead_sec": row_overhead_sec,
        "required_timeout_sec": required_timeout_sec,
        "row_timeout_sec": row_timeout_sec,
        "slack_sec": slack_sec,
        "fits_timeout_contract": slack_sec >= 0,
    }


def collect_row_contracts(
    manifest_path: Path,
    shard_dir: Path,
    *,
    per_attempt_timeout_sec: int,
    row_timeout_sec: int,
    row_overhead_sec: int,
) -> tuple[Mapping[str, Any], list[dict[str, Any]]]:
    manifest = load_json_object(manifest_path)
    if manifest.get("schema_name") != generator.MANIFEST_SCHEMA:
        raise TimeoutContractError(
            f"{repo_relative(manifest_path)}.schema_name must be {generator.MANIFEST_SCHEMA!r}, "
            f"got {manifest.get('schema_name')!r}"
        )

    shards = [
        object_at(shard, f"manifest.shards[{offset}]")
        for offset, shard in enumerate(list_at(manifest.get("shards"), "manifest.shards"))
    ]
    rows: list[dict[str, Any]] = []
    for shard_offset, shard in enumerate(shards):
        shard_label = f"manifest.shards[{shard_offset}]"
        shard_index = as_int(shard.get("shard_index"), f"{shard_label}.shard_index")
        matrix_path = matrix_path_from_shard(shard, shard_dir, shard_label)
        matrix = load_json_object(matrix_path)
        matrix_label = repo_relative(matrix_path)
        matrix_shard_index = as_int(matrix.get("shard_index"), f"{matrix_label}.shard_index")
        if matrix_shard_index != shard_index:
            raise TimeoutContractError(
                f"{matrix_label}.shard_index must match manifest shard_index {shard_index}, "
                f"got {matrix_shard_index}"
            )
        include = [
            object_at(row, f"{matrix_label}.include[{offset}]")
            for offset, row in enumerate(list_at(matrix.get("include"), f"{matrix_label}.include"))
        ]
        declared_rows = as_int(
            matrix.get("visible_matrix_row_count", matrix.get("benchmark_row_count")),
            f"{matrix_label}.visible_matrix_row_count",
        )
        if declared_rows != len(include):
            raise TimeoutContractError(
                f"{matrix_label}.visible_matrix_row_count must match include length "
                f"({declared_rows} != {len(include)})"
            )
        for row_offset, row in enumerate(include):
            rows.append(
                row_contract_record(
                    matrix_path=matrix_path,
                    matrix=matrix,
                    row=row,
                    row_offset=row_offset,
                    per_attempt_timeout_sec=per_attempt_timeout_sec,
                    row_timeout_sec=row_timeout_sec,
                    row_overhead_sec=row_overhead_sec,
                )
            )
    return manifest, rows


def summarize_contract_rows(
    manifest: Mapping[str, Any],
    rows: Sequence[Mapping[str, Any]],
    *,
    manifest_path: Path,
    shard_dir: Path,
    per_attempt_timeout_sec: int,
    row_timeout_sec: int,
    row_overhead_sec: int,
    max_violations: int,
) -> dict[str, Any]:
    if not rows:
        raise TimeoutContractError(
            "terminal discovery timeout contract cannot validate an empty row set"
        )

    violations = [dict(row) for row in rows if not row["fits_timeout_contract"]]
    tightest_row = min(
        rows,
        key=lambda row: (as_int(row["slack_sec"], "row.slack_sec"), str(row["matrix_row_id"])),
    )
    largest_row = max(
        rows,
        key=lambda row: (as_int(row["attempt_count"], "row.attempt_count"), str(row["matrix_row_id"])),
    )
    source_timeout_values = sorted(
        {
            timeout
            for row in rows
            for timeout in list_at(
                row.get("source_attempt_timeout_sec_values"),
                "row.source_attempt_timeout_sec_values",
            )
        }
    )
    max_required_timeout_sec = max(
        as_int(row["required_timeout_sec"], "row.required_timeout_sec") for row in rows
    )
    min_slack_sec = min(
        as_int(row["slack_sec"], "row.slack_sec") for row in rows
    )
    validation_status = "pass" if not violations else "fail"
    return {
        "schema_name": VALIDATION_SCHEMA,
        "schema_version": SCHEMA_VERSION,
        "validation_status": validation_status,
        "validated": validation_status == "pass",
        "dry_run": True,
        "manifest": repo_relative(manifest_path),
        "shard_dir": repo_relative(shard_dir),
        "source_manifest_schema_name": manifest.get("schema_name"),
        "source_manifest_schema_version": manifest.get("schema_version"),
        "contract": {
            "formula": CONTRACT_FORMULA,
            "row_timeout_sec": row_timeout_sec,
            "per_attempt_timeout_sec": per_attempt_timeout_sec,
            "row_overhead_sec": row_overhead_sec,
        },
        "row_count": len(rows),
        "attempt_count": sum(
            as_int(row["attempt_count"], "row.attempt_count") for row in rows
        ),
        "shard_count": len(
            {as_int(row["shard_index"], "row.shard_index") for row in rows}
        ),
        "source_attempt_timeout_sec_values": source_timeout_values,
        "max_attempt_count": as_int(
            largest_row["attempt_count"],
            "largest_row.attempt_count",
        ),
        "max_required_timeout_sec": max_required_timeout_sec,
        "min_slack_sec": min_slack_sec,
        "tightest_row": dict(tightest_row),
        "largest_row": dict(largest_row),
        "violation_count": len(violations),
        "violating_matrix_row_ids": [str(row["matrix_row_id"]) for row in violations],
        "violations_truncated": len(violations) > max_violations,
        "violations": violations[:max_violations],
    }


def validate_timeout_contract(
    manifest_path: Path,
    shard_dir: Path,
    *,
    per_attempt_timeout_sec: int,
    row_timeout_sec: int,
    row_overhead_sec: int,
    max_violations: int = DEFAULT_MAX_VIOLATIONS,
) -> dict[str, Any]:
    manifest, rows = collect_row_contracts(
        manifest_path,
        shard_dir,
        per_attempt_timeout_sec=per_attempt_timeout_sec,
        row_timeout_sec=row_timeout_sec,
        row_overhead_sec=row_overhead_sec,
    )
    return summarize_contract_rows(
        manifest,
        rows,
        manifest_path=manifest_path,
        shard_dir=shard_dir,
        per_attempt_timeout_sec=per_attempt_timeout_sec,
        row_timeout_sec=row_timeout_sec,
        row_overhead_sec=row_overhead_sec,
        max_violations=max_violations,
    )


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Dry-run/static guard for MI300A terminal-discovery row timeout contracts. "
            "The guard fails non-zero when any benchmark row's attempt_count cannot fit "
            "within the configured row/action timeout budget."
        )
    )
    parser.add_argument(
        "--manifest",
        default=str(generator.DEFAULT_MANIFEST),
        help="Path to mi300a_terminal_discovery_shard_manifest.json",
    )
    parser.add_argument(
        "--shard-dir",
        default=str(generator.DEFAULT_GENERATED_DIR),
        help="Directory containing per-shard terminal-discovery matrix JSON files",
    )
    timeout_group = parser.add_mutually_exclusive_group(required=True)
    timeout_group.add_argument(
        "--row-timeout-sec",
        type=positive_int_arg,
        help="Row/job Actions timeout budget in seconds",
    )
    timeout_group.add_argument(
        "--row-timeout-minutes",
        type=positive_int_arg,
        help="Row/job Actions timeout budget in minutes (converted to seconds)",
    )
    parser.add_argument(
        "--per-attempt-timeout-sec",
        type=positive_int_arg,
        required=True,
        help="Effective timeout budget for each sequential per-size attempt in the row",
    )
    parser.add_argument(
        "--row-overhead-sec",
        type=nonnegative_int_arg,
        required=True,
        help="Explicit non-simulator overhead budget per row, e.g. checkout/build/upload slack",
    )
    parser.add_argument(
        "--max-violations",
        type=positive_int_arg,
        default=DEFAULT_MAX_VIOLATIONS,
        help=f"Maximum violating row records to include in the JSON report (default: {DEFAULT_MAX_VIOLATIONS})",
    )
    return parser.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    args = parse_args(argv)
    row_timeout_sec = args.row_timeout_sec
    if row_timeout_sec is None:
        row_timeout_sec = args.row_timeout_minutes * 60

    manifest_path = repo_path(args.manifest)
    shard_dir = repo_path(args.shard_dir)
    try:
        report = validate_timeout_contract(
            manifest_path,
            shard_dir,
            per_attempt_timeout_sec=args.per_attempt_timeout_sec,
            row_timeout_sec=row_timeout_sec,
            row_overhead_sec=args.row_overhead_sec,
            max_violations=args.max_violations,
        )
    except TimeoutContractError as exc:
        print("FAIL: MI300A terminal-discovery timeout-contract guard rejected inputs.", file=sys.stderr)
        print(f"  - {exc}", file=sys.stderr)
        return 1

    print(json.dumps(report, indent=2) + "\n", end="")
    if not report["validated"]:
        print("FAIL: MI300A terminal-discovery timeout-contract guard rejected the row budget.", file=sys.stderr)
        print(
            "  - "
            f"{report['violation_count']} row(s) require more than row_timeout_sec="
            f"{report['contract']['row_timeout_sec']} using "
            f"per_attempt_timeout_sec={report['contract']['per_attempt_timeout_sec']} and "
            f"row_overhead_sec={report['contract']['row_overhead_sec']}",
            file=sys.stderr,
        )
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
