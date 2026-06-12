#!/usr/bin/env python3
"""
analyze_regressions.py — Regression analysis with 1pp significance threshold.

Reads a comparison CSV (e.g., comparison_ci.csv) produced by the CI benchmark
workflow and reports:
  1. Raw regression count: data points where sim error > baseline error.
  2. Significant regression count: data points where the absolute error
     increased by more than 1 percentage point (1pp) compared to a baseline.

When no --baseline is provided, regressions are defined as points where the
simulator over-estimates execution time (sim > real, i.e., positive relative
error). The "1pp significance" filter then counts only points whose absolute
relative error exceeds 1pp (0.01).

When --baseline is provided (a second comparison CSV from a prior run),
regressions are points where |error_new| > |error_baseline|, and significant
regressions are those where the increase exceeds 1pp.

Usage:
    # Without baseline (absolute analysis):
    python3 scripts/mi300a_acceptance/analyze_regressions.py \\
        --csv comparison_ci.csv

    # With baseline (differential analysis):
    python3 scripts/mi300a_acceptance/analyze_regressions.py \\
        --csv comparison_ci.csv \\
        --baseline comparison_baseline.csv

    # Custom significance threshold (in percentage points):
    python3 scripts/mi300a_acceptance/analyze_regressions.py \\
        --csv comparison_ci.csv \\
        --threshold 2.0

Exit codes:
    0: success (metrics printed)
    1: error (missing file, no data, etc.)
"""

import argparse
import csv
import sys
from typing import Dict, List, Optional, Tuple


def read_comparison_csv(
    path: str,
) -> Dict[Tuple[str, str], float]:
    """Read comparison_ci.csv and return {(kernel, size): rel_error}.

    Expects columns: kernel_name, problem_size, real_ms, sim_ms, abs_error_ms, rel_error
    """
    data: Dict[Tuple[str, str], float] = {}
    with open(path, newline="") as fh:
        reader = csv.DictReader(fh)
        for row in reader:
            kernel = row.get("kernel_name", "").strip()
            size = row.get("problem_size", "").strip()
            rel_str = row.get("rel_error", "").strip()
            if not kernel or not size or not rel_str:
                continue
            try:
                rel_error = float(rel_str)
            except ValueError:
                continue
            data[(kernel, size)] = rel_error
    return data


def analyze_absolute(
    data: Dict[Tuple[str, str], float],
    threshold_pp: float,
) -> dict:
    """Analyze regressions without a baseline.

    A "regression" is any point with positive relative error (sim slower than HW).
    A "significant regression" is one where |rel_error| > threshold_pp / 100.
    """
    threshold = threshold_pp / 100.0  # convert pp to fraction
    total = len(data)
    raw_regressions = 0
    significant_regressions = 0
    significant_details: List[Tuple[str, str, float]] = []

    for (kernel, size), rel_error in sorted(data.items()):
        abs_err = abs(rel_error)
        if rel_error > 0:  # sim is slower than real
            raw_regressions += 1
            if abs_err > threshold:
                significant_regressions += 1
                significant_details.append((kernel, size, rel_error))

    return {
        "total": total,
        "raw_regressions": raw_regressions,
        "raw_pct": raw_regressions / total * 100 if total else 0,
        "significant_regressions": significant_regressions,
        "significant_pct": significant_regressions / total * 100 if total else 0,
        "threshold_pp": threshold_pp,
        "details": significant_details,
    }


def analyze_differential(
    new_data: Dict[Tuple[str, str], float],
    baseline_data: Dict[Tuple[str, str], float],
    threshold_pp: float,
) -> dict:
    """Analyze regressions relative to a baseline run.

    A "regression" is any point where |error_new| > |error_baseline|.
    A "significant regression" is where the increase exceeds threshold_pp / 100.
    """
    threshold = threshold_pp / 100.0
    common_keys = set(new_data.keys()) & set(baseline_data.keys())
    total = len(common_keys)
    raw_regressions = 0
    significant_regressions = 0
    significant_details: List[Tuple[str, str, float, float, float]] = []

    for key in sorted(common_keys):
        new_err = abs(new_data[key])
        base_err = abs(baseline_data[key])
        delta = new_err - base_err

        if delta > 0:
            raw_regressions += 1
            if delta > threshold:
                significant_regressions += 1
                significant_details.append(
                    (key[0], key[1], new_data[key], baseline_data[key], delta)
                )

    return {
        "total": total,
        "raw_regressions": raw_regressions,
        "raw_pct": raw_regressions / total * 100 if total else 0,
        "significant_regressions": significant_regressions,
        "significant_pct": significant_regressions / total * 100 if total else 0,
        "threshold_pp": threshold_pp,
        "details": significant_details,
        "mode": "differential",
    }


def print_report(results: dict) -> None:
    """Print formatted regression analysis report."""
    W = 72
    print("=" * W)
    print(" MI300A Regression Analysis Report".center(W))
    print("=" * W)

    mode = results.get("mode", "absolute")
    total = results["total"]
    threshold_pp = results["threshold_pp"]

    print(f"\n  Mode:                    {mode}")
    print(f"  Total data points:       {total}")
    print(f"  Significance threshold:  {threshold_pp}pp")
    print()
    print(f"  Raw regressions:         {results['raw_regressions']}/{total}"
          f" ({results['raw_pct']:.1f}%)")
    print(f"  Significant regressions: {results['significant_regressions']}/{total}"
          f" ({results['significant_pct']:.1f}%)"
          f"  [>{threshold_pp}pp increase]")

    # Show top significant regressions
    details = results.get("details", [])
    if details:
        print(f"\n{'─' * W}")
        print(f"  Top significant regressions (showing up to 20):")
        print(f"{'─' * W}")

        if mode == "differential":
            print(f"  {'Kernel':<25s} {'Size':<20s} {'New Err':>10s} {'Base Err':>10s} {'Delta':>10s}")
            print(f"  {'-'*25} {'-'*20} {'-'*10} {'-'*10} {'-'*10}")
            for item in sorted(details, key=lambda x: -abs(x[4]))[:20]:
                kernel, size, new_err, base_err, delta = item
                print(f"  {kernel:<25s} {size:<20s} {new_err:>+9.1%} {base_err:>+9.1%} {delta:>+9.1%}")
        else:
            print(f"  {'Kernel':<25s} {'Size':<20s} {'Rel Error':>12s}")
            print(f"  {'-'*25} {'-'*20} {'-'*12}")
            for item in sorted(details, key=lambda x: -abs(x[2]))[:20]:
                kernel, size, rel_error = item
                print(f"  {kernel:<25s} {size:<20s} {rel_error:>+11.1%}")

    print(f"\n{'=' * W}")
    pass_fail = "PASS" if results["significant_pct"] < 50.0 else "NEEDS REVIEW"
    print(f"  Result: {pass_fail}")
    print(f"  (significant regressions < 50% of total = PASS)")
    print(f"{'=' * W}")


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Regression analysis with 1pp significance threshold."
    )
    parser.add_argument(
        "--csv",
        required=True,
        help="Path to comparison_ci.csv from the CI benchmark workflow.",
    )
    parser.add_argument(
        "--baseline",
        default=None,
        help="Path to a baseline comparison CSV for differential analysis.",
    )
    parser.add_argument(
        "--threshold",
        type=float,
        default=1.0,
        help="Significance threshold in percentage points (default: 1.0pp).",
    )
    args = parser.parse_args()

    # Read current data
    try:
        new_data = read_comparison_csv(args.csv)
    except FileNotFoundError:
        print(f"ERROR: File not found: {args.csv}", file=sys.stderr)
        sys.exit(1)

    if not new_data:
        print("ERROR: No valid data points in comparison CSV.", file=sys.stderr)
        sys.exit(1)

    if args.baseline:
        try:
            baseline_data = read_comparison_csv(args.baseline)
        except FileNotFoundError:
            print(f"ERROR: Baseline file not found: {args.baseline}", file=sys.stderr)
            sys.exit(1)

        if not baseline_data:
            print("ERROR: No valid data points in baseline CSV.", file=sys.stderr)
            sys.exit(1)

        results = analyze_differential(new_data, baseline_data, args.threshold)
    else:
        results = analyze_absolute(new_data, args.threshold)

    print_report(results)


if __name__ == "__main__":
    main()
