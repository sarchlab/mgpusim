#!/usr/bin/env python3
"""Validate workload output artifacts produced by scripts/run_all.sh."""

from __future__ import annotations

import argparse
import csv
import json
import sys
from pathlib import Path

ALLOWED_STATUS = {"success", "fail", "timeout", "skip"}

EXPECTED_RESULTS_HEADER = [
    "run_id",
    "suite",
    "benchmark",
    "platform",
    "problem_size",
    "iterations",
    "total_avg_ms",
    "total_min_ms",
    "total_max_ms",
    "total_stddev_ms",
    "status",
]

EXPECTED_KERNEL_HEADER = [
    "run_id",
    "suite",
    "benchmark",
    "platform",
    "problem_size",
    "kernel_name",
    "iterations",
    "avg_ms",
    "min_ms",
    "max_ms",
    "stddev_ms",
]


def _validate_csv_header_and_width(path: Path, expected_header: list[str], label: str) -> list[str]:
    errors: list[str] = []

    with path.open("r", encoding="utf-8", newline="") as f:
        reader = csv.reader(f)
        try:
            header = next(reader)
        except StopIteration:
            return [f"{label}: {path} is empty"]

        if header != expected_header:
            errors.append(
                f"{label}: unexpected header in {path}.\n"
                f"  expected: {expected_header}\n"
                f"  found:    {header}"
            )

        expected_width = len(header)
        for line_num, row in enumerate(reader, start=2):
            if len(row) != expected_width:
                errors.append(
                    f"{label}: row width mismatch in {path}:{line_num} "
                    f"(expected {expected_width}, found {len(row)})"
                )

    return errors


def validate_results(path: Path) -> list[str]:
    errors = _validate_csv_header_and_width(path, EXPECTED_RESULTS_HEADER, "results")

    with path.open("r", encoding="utf-8", newline="") as f:
        reader = csv.reader(f)
        header = next(reader, [])
        if not header:
            return errors

        try:
            status_idx = header.index("status")
        except ValueError:
            errors.append(f"results: missing status column in {path}")
            return errors

        for line_num, row in enumerate(reader, start=2):
            if len(row) <= status_idx:
                continue
            status = row[status_idx].strip()
            if status not in ALLOWED_STATUS:
                errors.append(
                    f"results: invalid status '{status}' in {path}:{line_num}; "
                    f"expected one of {sorted(ALLOWED_STATUS)}"
                )

    return errors


def validate_kernel(path: Path) -> list[str]:
    return _validate_csv_header_and_width(path, EXPECTED_KERNEL_HEADER, "kernel_timings")


def validate_sysinfo(path: Path) -> list[str]:
    errors: list[str] = []

    try:
        with path.open("r", encoding="utf-8") as f:
            payload = json.load(f)
    except Exception as exc:  # pragma: no cover - surfaced to user
        return [f"sysinfo: failed to parse {path}: {exc}"]

    if not isinstance(payload, dict):
        errors.append(f"sysinfo: expected top-level JSON object in {path}")

    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--results", type=Path, help="Path to results_*.csv")
    parser.add_argument("--kernel", type=Path, help="Path to kernel_timings_*.csv")
    parser.add_argument("--sysinfo", type=Path, help="Path to sysinfo_*.json")

    args = parser.parse_args()

    selected = [args.results, args.kernel, args.sysinfo]
    if not any(selected):
        parser.error("provide at least one of --results/--kernel/--sysinfo")

    errors: list[str] = []

    if args.results:
        if not args.results.exists():
            errors.append(f"results: file not found: {args.results}")
        else:
            errors.extend(validate_results(args.results))

    if args.kernel:
        if not args.kernel.exists():
            errors.append(f"kernel_timings: file not found: {args.kernel}")
        else:
            errors.extend(validate_kernel(args.kernel))

    if args.sysinfo:
        if not args.sysinfo.exists():
            errors.append(f"sysinfo: file not found: {args.sysinfo}")
        else:
            errors.extend(validate_sysinfo(args.sysinfo))

    if errors:
        for err in errors:
            print(f"[ERROR] {err}", file=sys.stderr)
        return 1

    checked = []
    if args.results:
        checked.append(f"results={args.results}")
    if args.kernel:
        checked.append(f"kernel={args.kernel}")
    if args.sysinfo:
        checked.append(f"sysinfo={args.sysinfo}")

    print(f"[OK] Output validation passed ({', '.join(checked)})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
