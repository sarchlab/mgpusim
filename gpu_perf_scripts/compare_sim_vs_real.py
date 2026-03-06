#!/usr/bin/env python3
"""
compare_sim_vs_real.py — Compare simulator output against MI300A reference data.

Usage:
    python3 compare_sim_vs_real.py --ref <reference_csv> --sim <simulator_csv> [--output <output_csv>]

Reference CSV format (gpu_perf_scripts/mi300a.csv):
    kernel_name,problem_size,iterations,avg_ms,min_ms,max_ms

Simulator CSV format (expected):
    kernel_name,problem_size,sim_time_ms
    (or: kernel_name,problem_size,iterations,avg_ms,min_ms,max_ms)

The script:
  1. Reads both CSVs and matches rows on (kernel_name, problem_size).
  2. Computes per-benchmark relative error: |sim - real| / real
  3. Outputs per-benchmark breakdown, per-kernel summary, and overall stats.
  4. Optionally writes detailed results to a CSV.
"""

import argparse
import csv
import sys
from collections import defaultdict
from dataclasses import dataclass, field
from typing import Dict, List, Optional, Tuple


@dataclass
class BenchmarkPoint:
    kernel_name: str
    problem_size: str
    real_ms: float
    sim_ms: Optional[float] = None
    abs_error_ms: Optional[float] = None
    rel_error: Optional[float] = None


@dataclass
class KernelSummary:
    kernel_name: str
    num_points: int = 0
    num_matched: int = 0
    total_rel_error: float = 0.0
    max_rel_error: float = 0.0
    max_rel_error_size: str = ""
    avg_rel_error: float = 0.0
    points: List[BenchmarkPoint] = field(default_factory=list)


def read_reference_csv(filepath: str) -> Dict[Tuple[str, str], float]:
    """Read reference CSV and return dict of (kernel, size) -> avg_ms.

    For duplicates (e.g., BFS with same node count but different graph),
    we keep the first occurrence.
    """
    data = {}
    with open(filepath, "r") as f:
        reader = csv.DictReader(f)
        for row in reader:
            key = (row["kernel_name"].strip(), row["problem_size"].strip())
            avg_ms = float(row["avg_ms"])
            if key not in data:
                data[key] = avg_ms
            else:
                # Average duplicate entries (e.g., BFS with different seeds)
                data[key] = (data[key] + avg_ms) / 2.0
    return data


def read_simulator_csv(filepath: str) -> Dict[Tuple[str, str], float]:
    """Read simulator output CSV. Supports multiple formats:

    Format 1: kernel_name,problem_size,sim_time_ms
    Format 2: kernel_name,problem_size,iterations,avg_ms,min_ms,max_ms
    Format 3: kernel_name,problem_size,sim_time_sec (auto-detected if < 0.001)
    """
    data = {}
    with open(filepath, "r") as f:
        reader = csv.DictReader(f)
        fieldnames = reader.fieldnames
        if fieldnames is None:
            print("ERROR: Simulator CSV has no header row.", file=sys.stderr)
            sys.exit(1)

        for row in reader:
            kernel = row["kernel_name"].strip()
            size = row["problem_size"].strip()
            key = (kernel, size)

            if "sim_time_ms" in fieldnames:
                sim_ms = float(row["sim_time_ms"])
            elif "avg_ms" in fieldnames:
                sim_ms = float(row["avg_ms"])
            elif "sim_time_sec" in fieldnames:
                sim_ms = float(row["sim_time_sec"]) * 1000.0
            elif "time_ms" in fieldnames:
                sim_ms = float(row["time_ms"])
            elif "time_sec" in fieldnames:
                sim_ms = float(row["time_sec"]) * 1000.0
            else:
                # Try to use the third column
                cols = list(row.values())
                try:
                    sim_ms = float(cols[2])
                except (ValueError, IndexError):
                    print(
                        f"WARNING: Cannot parse sim time for {key}",
                        file=sys.stderr,
                    )
                    continue

            data[key] = sim_ms
    return data


def compute_errors(
    ref_data: Dict[Tuple[str, str], float],
    sim_data: Dict[Tuple[str, str], float],
) -> Tuple[List[BenchmarkPoint], Dict[str, KernelSummary]]:
    """Compute per-point and per-kernel error metrics."""

    all_points = []
    kernel_summaries: Dict[str, KernelSummary] = {}

    # Group reference data by kernel
    kernels = defaultdict(list)
    for (kernel, size), real_ms in sorted(ref_data.items()):
        kernels[kernel].append((size, real_ms))

    for kernel in sorted(kernels.keys()):
        summary = KernelSummary(kernel_name=kernel)
        for size, real_ms in kernels[kernel]:
            point = BenchmarkPoint(
                kernel_name=kernel, problem_size=size, real_ms=real_ms
            )
            summary.num_points += 1

            key = (kernel, size)
            if key in sim_data:
                sim_ms = sim_data[key]
                point.sim_ms = sim_ms
                point.abs_error_ms = abs(sim_ms - real_ms)
                if real_ms > 0:
                    point.rel_error = point.abs_error_ms / real_ms
                else:
                    point.rel_error = float("inf") if sim_ms > 0 else 0.0

                summary.num_matched += 1
                summary.total_rel_error += point.rel_error
                if point.rel_error > summary.max_rel_error:
                    summary.max_rel_error = point.rel_error
                    summary.max_rel_error_size = size

            summary.points.append(point)
            all_points.append(point)

        if summary.num_matched > 0:
            summary.avg_rel_error = summary.total_rel_error / summary.num_matched

        kernel_summaries[kernel] = summary

    return all_points, kernel_summaries


def analyze_reference_only(
    ref_filepath: str, ref120_filepath: Optional[str] = None
):
    """Analyze reference data patterns without simulator data."""
    ref_data = read_reference_csv(ref_filepath)
    ref120_data = None
    if ref120_filepath:
        ref120_data = read_reference_csv(ref120_filepath)

    # Group by kernel
    kernels = defaultdict(list)
    for (kernel, size), real_ms in sorted(ref_data.items()):
        kernels[kernel].append((size, real_ms))

    print("=" * 80)
    print("REFERENCE DATA ANALYSIS — MI300A (240 CU)")
    print("=" * 80)

    for kernel in sorted(kernels.keys()):
        sizes = kernels[kernel]
        print(f"\n--- {kernel} ({len(sizes)} data points) ---")
        for size, ms in sizes:
            line = f"  {size:>40s}: {ms:>12.4f} ms"
            if ref120_data:
                key = (kernel, size)
                if key in ref120_data:
                    ms120 = ref120_data[key]
                    ratio = ms / ms120 if ms120 > 0 else float("inf")
                    speedup = ms120 / ms if ms > 0 else float("inf")
                    line += f"  | 120CU: {ms120:>12.4f} ms  | ratio(240/120): {ratio:.3f}  | speedup: {speedup:.2f}x"
            print(line)

    if ref120_data:
        print("\n" + "=" * 80)
        print("CU SCALING ANALYSIS (240 CU vs 120 CU)")
        print("=" * 80)
        print(
            "\nBenchmarks where time halves with 2x CUs = compute-bound (speedup ~2x)"
        )
        print("Benchmarks where time stays same = memory-bound (speedup ~1x)")
        print()

        kernel_scaling = {}
        for kernel in sorted(kernels.keys()):
            speedups = []
            for size, ms240 in kernels[kernel]:
                key = (kernel, size)
                if key in ref120_data and ms240 > 0.01:  # skip tiny values
                    ms120 = ref120_data[key]
                    if ms240 > 0:
                        speedup = ms120 / ms240
                        speedups.append(speedup)
            if speedups:
                avg_speedup = sum(speedups) / len(speedups)
                # Use only large problem sizes for classification
                large_speedups = speedups[-max(1, len(speedups) // 3) :]
                avg_large_speedup = sum(large_speedups) / len(large_speedups)
                kernel_scaling[kernel] = (avg_speedup, avg_large_speedup)

        # Classify
        compute_bound = []
        memory_bound = []
        mixed = []
        for kernel, (avg_sp, avg_large_sp) in sorted(
            kernel_scaling.items(), key=lambda x: -x[1][1]
        ):
            if avg_large_sp > 1.5:
                compute_bound.append((kernel, avg_sp, avg_large_sp))
            elif avg_large_sp < 1.15:
                memory_bound.append((kernel, avg_sp, avg_large_sp))
            else:
                mixed.append((kernel, avg_sp, avg_large_sp))

        print("COMPUTE-BOUND (large-size speedup > 1.5x with 2x CUs):")
        for k, avg, large in compute_bound:
            print(f"  {k:>30s}: avg speedup={avg:.2f}x, large-size speedup={large:.2f}x")

        print("\nMEMORY-BOUND (large-size speedup < 1.15x with 2x CUs):")
        for k, avg, large in memory_bound:
            print(f"  {k:>30s}: avg speedup={avg:.2f}x, large-size speedup={large:.2f}x")

        print("\nMIXED (1.15x - 1.5x speedup):")
        for k, avg, large in mixed:
            print(f"  {k:>30s}: avg speedup={avg:.2f}x, large-size speedup={large:.2f}x")


def print_comparison_results(
    all_points: List[BenchmarkPoint],
    kernel_summaries: Dict[str, KernelSummary],
):
    """Print formatted comparison results."""
    matched_points = [p for p in all_points if p.sim_ms is not None]
    unmatched_points = [p for p in all_points if p.sim_ms is None]

    print("=" * 100)
    print("SIMULATOR vs REFERENCE COMPARISON")
    print("=" * 100)

    # Per-kernel summary
    print(f"\n{'Kernel':<30s} {'Matched':>8s} {'Total':>6s} {'Avg Err':>10s} {'Max Err':>10s} {'Max Err Size':<30s}")
    print("-" * 100)

    for kernel in sorted(kernel_summaries.keys()):
        s = kernel_summaries[kernel]
        if s.num_matched > 0:
            print(
                f"{s.kernel_name:<30s} {s.num_matched:>8d} {s.num_points:>6d} "
                f"{s.avg_rel_error:>9.1%} {s.max_rel_error:>9.1%} {s.max_rel_error_size:<30s}"
            )
        else:
            print(
                f"{s.kernel_name:<30s} {s.num_matched:>8d} {s.num_points:>6d} "
                f"{'N/A':>10s} {'N/A':>10s} {'':30s}"
            )

    # Overall stats
    if matched_points:
        all_errors = [p.rel_error for p in matched_points if p.rel_error is not None]
        overall_avg = sum(all_errors) / len(all_errors)
        overall_max = max(all_errors)
        overall_median = sorted(all_errors)[len(all_errors) // 2]
        within_10 = sum(1 for e in all_errors if e <= 0.10) / len(all_errors)
        within_25 = sum(1 for e in all_errors if e <= 0.25) / len(all_errors)
        within_50 = sum(1 for e in all_errors if e <= 0.50) / len(all_errors)

        print("\n" + "=" * 100)
        print("OVERALL STATISTICS")
        print("=" * 100)
        print(f"  Total reference points:  {len(all_points)}")
        print(f"  Matched points:          {len(matched_points)}")
        print(f"  Unmatched points:        {len(unmatched_points)}")
        print(f"  Average relative error:  {overall_avg:.1%}")
        print(f"  Median relative error:   {overall_median:.1%}")
        print(f"  Maximum relative error:  {overall_max:.1%}")
        print(f"  Within 10% error:        {within_10:.1%}")
        print(f"  Within 25% error:        {within_25:.1%}")
        print(f"  Within 50% error:        {within_50:.1%}")
    else:
        print("\nNo matching data points found between reference and simulator.")

    # Detailed per-point table for matched points
    if matched_points:
        print("\n" + "=" * 100)
        print("DETAILED PER-POINT COMPARISON (matched points)")
        print("=" * 100)
        print(
            f"{'Kernel':<25s} {'Size':<30s} {'Real(ms)':>12s} {'Sim(ms)':>12s} "
            f"{'Abs Err':>12s} {'Rel Err':>10s}"
        )
        print("-" * 100)
        for p in matched_points:
            print(
                f"{p.kernel_name:<25s} {p.problem_size:<30s} {p.real_ms:>12.4f} "
                f"{p.sim_ms:>12.4f} {p.abs_error_ms:>12.4f} {p.rel_error:>9.1%}"
            )


def write_output_csv(
    filepath: str,
    all_points: List[BenchmarkPoint],
):
    """Write detailed comparison results to CSV."""
    with open(filepath, "w", newline="") as f:
        writer = csv.writer(f)
        writer.writerow(
            [
                "kernel_name",
                "problem_size",
                "real_ms",
                "sim_ms",
                "abs_error_ms",
                "rel_error",
            ]
        )
        for p in all_points:
            writer.writerow(
                [
                    p.kernel_name,
                    p.problem_size,
                    f"{p.real_ms:.6f}",
                    f"{p.sim_ms:.6f}" if p.sim_ms is not None else "",
                    f"{p.abs_error_ms:.6f}" if p.abs_error_ms is not None else "",
                    f"{p.rel_error:.6f}" if p.rel_error is not None else "",
                ]
            )
    print(f"\nDetailed results written to: {filepath}")


def main():
    parser = argparse.ArgumentParser(
        description="Compare simulator output against MI300A reference data."
    )
    parser.add_argument(
        "--ref",
        required=True,
        help="Path to reference CSV (e.g., gpu_perf_scripts/mi300a.csv)",
    )
    parser.add_argument(
        "--sim",
        default=None,
        help="Path to simulator output CSV. If omitted, only reference analysis is printed.",
    )
    parser.add_argument(
        "--ref120",
        default=None,
        help="Path to 120 CU reference CSV for CU scaling analysis.",
    )
    parser.add_argument(
        "--output",
        default=None,
        help="Path to write detailed comparison CSV.",
    )
    parser.add_argument(
        "--analyze-only",
        action="store_true",
        help="Only analyze reference data (no simulator comparison).",
    )
    args = parser.parse_args()

    if args.analyze_only or args.sim is None:
        analyze_reference_only(args.ref, args.ref120)
        return

    # Full comparison mode
    ref_data = read_reference_csv(args.ref)
    sim_data = read_simulator_csv(args.sim)

    all_points, kernel_summaries = compute_errors(ref_data, sim_data)
    print_comparison_results(all_points, kernel_summaries)

    if args.output:
        write_output_csv(args.output, all_points)


if __name__ == "__main__":
    main()
