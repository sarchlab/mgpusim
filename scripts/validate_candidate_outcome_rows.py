#!/usr/bin/env python3
"""Validate structured terminal outcome rows for focused single-candidate artifacts."""

from __future__ import annotations

import argparse
import csv
import sys
from pathlib import Path


REQUIRED_COLUMNS = [
    "benchmark",
    "candidate_size",
    "size_label",
    "terminal_outcome",
    "sim_result_state",
    "exit_code",
    "timeout_sec",
    "detail",
]


ALLOWED_TERMINAL_OUTCOMES = {
    "success",
    "completed",
    "timeout",
    "timed_out",
    "crash",
    "crashed",
    "failed",
    "missing_output",
    "missing_kernel_time",
    "skipped",
    "cancelled",
}


ALLOWED_SIM_RESULT_STATES = {"sim_row_emitted", "no_sim_row"}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Validate terminal outcome CSV emitted by focused single-candidate runs"
    )
    parser.add_argument("--csv", required=True, help="Path to candidate outcome CSV")
    parser.add_argument(
        "--expected-attempts",
        type=int,
        default=None,
        help="Expected number of attempted candidates (optional)",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    csv_path = Path(args.csv)

    if not csv_path.is_file():
        print(f"ERROR: outcome CSV not found: {csv_path}", file=sys.stderr)
        return 1

    with csv_path.open("r", newline="") as f:
        reader = csv.DictReader(f)
        if reader.fieldnames is None:
            print("ERROR: outcome CSV missing header", file=sys.stderr)
            return 1

        missing_columns = [c for c in REQUIRED_COLUMNS if c not in reader.fieldnames]
        if missing_columns:
            print(
                "ERROR: outcome CSV missing required columns: "
                + ", ".join(missing_columns),
                file=sys.stderr,
            )
            return 1

        rows = list(reader)

    if not rows:
        print(
            "ERROR: outcome CSV has no terminal rows (header-only artifact).",
            file=sys.stderr,
        )
        return 1

    if args.expected_attempts is not None and len(rows) != args.expected_attempts:
        print(
            f"ERROR: expected {args.expected_attempts} terminal rows, found {len(rows)}",
            file=sys.stderr,
        )
        return 1

    seen_candidate_keys: set[tuple[str, str, str]] = set()

    for idx, row in enumerate(rows, start=2):
        for col in ("benchmark", "candidate_size", "size_label", "terminal_outcome", "sim_result_state"):
            if not row.get(col, "").strip():
                print(
                    f"ERROR: row {idx} has empty required value for '{col}'",
                    file=sys.stderr,
                )
                return 1

        candidate_key = (
            row["benchmark"].strip(),
            row["candidate_size"].strip(),
            row["size_label"].strip(),
        )
        if candidate_key in seen_candidate_keys:
            print(
                "ERROR: duplicate terminal rows for candidate "
                f"benchmark={candidate_key[0]} candidate_size={candidate_key[1]} size_label={candidate_key[2]}",
                file=sys.stderr,
            )
            return 1
        seen_candidate_keys.add(candidate_key)

        terminal_outcome = row["terminal_outcome"].strip()
        if terminal_outcome not in ALLOWED_TERMINAL_OUTCOMES:
            print(
                f"ERROR: row {idx} has unsupported terminal_outcome '{terminal_outcome}'",
                file=sys.stderr,
            )
            return 1

        sim_state = row["sim_result_state"].strip()
        if sim_state not in ALLOWED_SIM_RESULT_STATES:
            print(
                f"ERROR: row {idx} has unsupported sim_result_state '{sim_state}'",
                file=sys.stderr,
            )
            return 1

    expected = args.expected_attempts if args.expected_attempts is not None else "unspecified"
    print(
        f"VALIDATION_OK outcome_csv={csv_path} terminal_rows={len(rows)} expected_attempts={expected}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
