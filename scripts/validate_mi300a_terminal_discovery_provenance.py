#!/usr/bin/env python3
"""Validate compact terminal MI300A discovery provenance.

This validator is the offline guard for terminal discovery collection output.
It requires one terminal outcome accounting record, one outcome row, one artifact
entry, and one terminal job entry for every row in the 1416-entry MI300A
problem-size discovery plan.  Terminal mode rejects non-terminal and missing-job
buckets; all plan entries must be accounted as completed-success, failed,
timed-out, skipped, or cancelled.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Optional, Sequence

import collect_mi300a_terminal_discovery_provenance as collector


def repo_path(path_text: str) -> Path:
    return collector.repo_path(path_text)


def parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Validate terminal MI300A discovery provenance coverage/accounting."
    )
    parser.add_argument("--plan", default=str(collector.DEFAULT_PLAN), help="Path to mi300a_problem_size_discovery_plan.json")
    parser.add_argument("--provenance", required=True, help="Path to collected terminal discovery provenance JSON")
    return parser.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    args = parse_args(argv)
    try:
        report = collector.validate_terminal_provenance_path(
            provenance_path=repo_path(args.provenance),
            plan_path=repo_path(args.plan),
        )
    except collector.CollectionError as exc:
        print("FAIL: MI300A terminal discovery provenance validation rejected input.", file=sys.stderr)
        print(f"  - {exc}", file=sys.stderr)
        return 1

    print(json.dumps(report, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
