#!/usr/bin/env python3
"""
compare_regression.py — Linear-regression-based accuracy evaluation.

For each benchmark, fits linear regressions to both simulator and hardware
timing data (time vs problem_size), then compares the slopes. A slope ratio
close to 1.0 means the simulator scales at the same rate as real hardware.

For multi-parameter benchmarks (fir, kmeans, pagerank, spmv, conv2d), data is
grouped by non-scaling parameters and regression is performed only on the
scaling parameter within each group. Each group is reported as a separate line.

Only LARGE problem sizes are used (where the GPU is fully utilized), avoiding
noise from launch overhead at small sizes.

Usage:
    python3 compare_regression.py --ref mi300a.csv --sim sim_results.csv [--output details.csv]
"""

import argparse
import csv
import math
import re
import sys
from collections import defaultdict
from typing import Dict, List, Optional, Tuple

# =============================================================================
# Problem-size thresholds: only use sizes >= threshold for regression
# =============================================================================
SIZE_THRESHOLDS = {
    "vectoradd": 262144,
    "relu": 262144,
    "bitonicsort": 262144,
    "fastwalshtransform": 262144,
    "fir": 262144,
    "kmeans": 262144,
    "matrixmultiplication": 2048,
    "stencil2d": 2048,
    "matrixtranspose": 2048,
    "simpleconvolution": 2048,
    "floydwarshall": 512,
    "pagerank": 4096,
    "fft1D_512": 2097152,
    "spmv_csr_scalar": 262144,
    "bfs": 131072,
}

# =============================================================================
# Multi-parameter benchmark definitions
# =============================================================================
# For each multi-param benchmark, define how to extract:
#   - scaling_value: the numeric value to regress on (X axis)
#   - group_key: string identifying the non-scaling parameter group
#
# Benchmarks not listed here use the original single-parameter behavior.
# =============================================================================

MULTI_PARAM_BENCHMARKS = {"fir", "kmeans", "pagerank", "spmv_csr_scalar", "conv2d"}


def parse_scaling_and_group(
    kernel_name: str, size_str: str
) -> Optional[Tuple[float, str]]:
    """For multi-parameter benchmarks, extract (scaling_value, group_key).

    Returns None if the size string cannot be parsed.
    """
    s = size_str.strip()

    if kernel_name == "fir":
        # Format: '1024_taps16' → scaling=1024, group='taps=16'
        m = re.match(r"^(\d+)_taps(\d+)$", s)
        if m:
            return (float(m.group(1)), f"taps={m.group(2)}")
        return None

    if kernel_name == "kmeans":
        # Format: 'pts1024_feat8_clus5' → scaling=1024, group='feat=8,clus=5'
        m = re.match(r"^pts(\d+)_feat(\d+)_clus(\d+)$", s)
        if m:
            return (float(m.group(1)), f"feat={m.group(2)},clus={m.group(3)}")
        return None

    if kernel_name == "pagerank":
        # Format: 'nodes4096_sparsity0.5_priters2' → scaling=4096, group='sparsity=0.5,iters=2'
        m = re.match(
            r"^nodes(\d+)_sparsity([0-9.]+)_priters(\d+)$", s
        )
        if m:
            return (
                float(m.group(1)),
                f"sparsity={m.group(2)},iters={m.group(3)}",
            )
        return None

    if kernel_name in ("spmv_csr_scalar", "spmv"):
        # Format: '1024x1024_nnz4096' → scaling=4096, group='dims=1024x1024'
        m = re.match(r"^(\d+x\d+)_nnz(\d+)$", s)
        if m:
            return (float(m.group(2)), f"dims={m.group(1)}")
        return None

    if kernel_name == "conv2d":
        # Format: 'N1_C1_128x128_k3x3_OC3' → scaling=128*128=16384, group='N=1,C=1,k=3x3,OC=3'
        m = re.match(
            r"^N(\d+)_C(\d+)_(\d+)x(\d+)_k(\d+x\d+)_OC(\d+)$", s
        )
        if m:
            h, w = float(m.group(3)), float(m.group(4))
            return (
                h * w,
                f"N={m.group(1)},C={m.group(2)},k={m.group(5)},OC={m.group(6)}",
            )
        return None

    return None


def parse_problem_size(kernel_name: str, size_str: str) -> Optional[float]:
    """Convert a problem_size string to a single numeric value for regression.

    Used for single-parameter benchmarks (those not in MULTI_PARAM_BENCHMARKS).

    Handles formats like:
      '536870912'                    -> 536870912
      '2048x2048'                    -> product (4194304) for 2D benchmarks
      '2048x2048x2048'               -> first dim (2048) for matmul
      'nodes1024'                    -> 1024
      '8_nodes'                      -> 8
      '131072_elements'              -> 131072
      'length=64'                    -> 64
    """
    s = size_str.strip()

    # matrixmultiplication: NxNxN -> first dimension
    if kernel_name == "matrixmultiplication":
        m = re.match(r"^(\d+)x(\d+)x(\d+)$", s)
        if m:
            return float(m.group(1))

    # fft: N_elements
    if kernel_name in ("fft1D_512", "fft"):
        m = re.match(r"^(\d+)_elements$", s)
        if m:
            return float(m.group(1))

    # bfs: nodesN
    if kernel_name == "bfs":
        m = re.match(r"^nodes(\d+)$", s)
        if m:
            return float(m.group(1))

    # floydwarshall: N_nodes
    if kernel_name == "floydwarshall":
        m = re.match(r"^(\d+)_nodes$", s)
        if m:
            return float(m.group(1))

    # nw: length=N
    if kernel_name == "nw":
        m = re.match(r"^length=(\d+)$", s)
        if m:
            return float(m.group(1))

    # im2col: N1_C1_HxW_kKxK_OC3
    if kernel_name == "im2col":
        m = re.search(r"_(\d+)x(\d+)_k(\d+)x(\d+)", s)
        if m:
            return float(m.group(1)) * float(m.group(2))

    # 2D benchmarks: matrixtranspose uses width; others use area
    if kernel_name in ("matrixtranspose",):
        m = re.match(r"^(\d+)x(\d+)$", s)
        if m:
            return float(m.group(1))  # width

    if kernel_name in ("stencil2d", "atax", "bicg", "simpleconvolution"):
        m = re.match(r"^(\d+)x(\d+)$", s)
        if m:
            return float(m.group(1)) * float(m.group(2))

    # Pure numeric
    m = re.match(r"^(\d+)$", s)
    if m:
        return float(m.group(1))

    # NxN format (generic fallback)
    m = re.match(r"^(\d+)x(\d+)$", s)
    if m:
        return float(m.group(1)) * float(m.group(2))

    # Try to extract any leading number
    m = re.match(r"^(\d+(?:\.\d+)?)", s)
    if m:
        return float(m.group(1))

    return None


def get_threshold(kernel_name: str) -> float:
    """Get the minimum problem size threshold for a kernel."""
    return SIZE_THRESHOLDS.get(kernel_name, 0)


def read_csv_data(filepath: str) -> Dict[Tuple[str, str], float]:
    """Read a CSV file and return (kernel, size) -> avg_ms.

    Supports multiple formats:
      - kernel_name, problem_size, iterations, avg_ms, min_ms, max_ms
      - kernel_name, problem_size, sim_time_ms
    """
    data: Dict[Tuple[str, str], List[float]] = defaultdict(list)
    with open(filepath, "r") as f:
        reader = csv.DictReader(f)
        fieldnames = reader.fieldnames or []

        for row in reader:
            kernel = row["kernel_name"].strip()
            size = row["problem_size"].strip()
            key = (kernel, size)

            if "avg_ms" in fieldnames:
                ms = float(row["avg_ms"])
            elif "sim_time_ms" in fieldnames:
                ms = float(row["sim_time_ms"])
            elif "sim_time_sec" in fieldnames:
                ms = float(row["sim_time_sec"]) * 1000.0
            elif "time_ms" in fieldnames:
                ms = float(row["time_ms"])
            elif "time_sec" in fieldnames:
                ms = float(row["time_sec"]) * 1000.0
            else:
                cols = list(row.values())
                try:
                    ms = float(cols[3] if len(cols) > 3 else cols[2])
                except (ValueError, IndexError):
                    continue

            data[key].append(ms)

    # Average duplicates (e.g., BFS with different seeds)
    result = {}
    for key, values in data.items():
        result[key] = sum(values) / len(values)
    return result


def linear_regression(x: List[float], y: List[float]) -> Tuple[float, float, float]:
    """Fit y = slope * x + intercept using least squares.

    Returns (slope, intercept, r_squared).
    """
    n = len(x)
    if n < 2:
        return (0.0, 0.0, 0.0)

    sum_x = sum(x)
    sum_y = sum(y)
    sum_xy = sum(xi * yi for xi, yi in zip(x, y))
    sum_x2 = sum(xi * xi for xi in x)
    mean_y = sum_y / n

    denom = n * sum_x2 - sum_x * sum_x
    if abs(denom) < 1e-30:
        return (0.0, mean_y, 0.0)

    slope = (n * sum_xy - sum_x * sum_y) / denom
    intercept = (sum_y - slope * sum_x) / n

    # R²
    ss_res = sum((yi - (slope * xi + intercept)) ** 2 for xi, yi in zip(x, y))
    ss_tot = sum((yi - mean_y) ** 2 for yi in y)
    r_squared = 1.0 - (ss_res / ss_tot) if ss_tot > 1e-30 else 0.0

    return (slope, intercept, r_squared)


def _do_regression_on_group(
    kernel: str,
    label: str,
    group_key: str,
    ref_data: Dict[Tuple[str, str], float],
    sim_data: Dict[Tuple[str, str], float],
    threshold: float,
    is_multi: bool,
):
    """Run regression for a single (kernel, group) combination.

    For multi-param benchmarks, *group_key* selects data and
    ``parse_scaling_and_group`` extracts the X value.
    For single-param benchmarks, ``parse_problem_size`` is used.

    Returns (result_tuple, detail_rows) or (None, []) if insufficient data.
    """
    ref_sizes = {s: ms for (k, s), ms in ref_data.items() if k == kernel}
    sim_sizes = {s: ms for (k, s), ms in sim_data.items() if k == kernel}
    common_sizes = sorted(set(ref_sizes.keys()) & set(sim_sizes.keys()))
    if not common_sizes:
        return None, []

    filtered = []
    for size_str in common_sizes:
        if is_multi:
            parsed = parse_scaling_and_group(kernel, size_str)
            if parsed is None:
                continue
            scaling_val, gk = parsed
            if gk != group_key:
                continue
            numeric_size = scaling_val
        else:
            numeric_size = parse_problem_size(kernel, size_str)
            if numeric_size is None:
                continue

        if numeric_size < threshold:
            continue
        filtered.append(
            (size_str, numeric_size, ref_sizes[size_str], sim_sizes[size_str])
        )

    if len(filtered) < 3:
        return None, []

    filtered.sort(key=lambda t: t[1])
    x_vals = [t[1] for t in filtered]
    hw_vals = [t[2] for t in filtered]
    sim_vals = [t[3] for t in filtered]

    hw_slope, hw_intercept, hw_r2 = linear_regression(x_vals, hw_vals)
    sim_slope, sim_intercept, sim_r2 = linear_regression(x_vals, sim_vals)

    if abs(hw_slope) > 1e-30:
        slope_ratio = sim_slope / hw_slope
    else:
        slope_ratio = float("inf") if abs(sim_slope) > 1e-30 else 1.0

    result = (label, slope_ratio, sim_slope, hw_slope, sim_r2, hw_r2, len(filtered))

    detail_rows = []
    for size_str, numeric_size, hw_ms, sim_ms in filtered:
        hw_pred = hw_slope * numeric_size + hw_intercept
        sim_pred = sim_slope * numeric_size + sim_intercept
        detail_rows.append(
            {
                "kernel": kernel,
                "group": group_key,
                "label": label,
                "problem_size": size_str,
                "numeric_size": numeric_size,
                "hw_ms": hw_ms,
                "sim_ms": sim_ms,
                "hw_pred_ms": hw_pred,
                "sim_pred_ms": sim_pred,
            }
        )

    return result, detail_rows


def main():
    parser = argparse.ArgumentParser(
        description="Linear-regression-based accuracy evaluation of simulator vs hardware."
    )
    parser.add_argument(
        "--ref",
        required=True,
        help="Path to hardware reference CSV (e.g., mi300a.csv)",
    )
    parser.add_argument(
        "--sim",
        required=True,
        help="Path to simulator results CSV",
    )
    parser.add_argument(
        "--output",
        default=None,
        help="Optional output CSV for per-point details",
    )
    parser.add_argument(
        "--min-fill",
        type=float,
        default=None,
        help="Override minimum problem size threshold (applied to all benchmarks)",
    )
    args = parser.parse_args()

    ref_data = read_csv_data(args.ref)
    sim_data = read_csv_data(args.sim)

    # Collect all kernels
    all_kernels = set()
    for (k, _) in ref_data:
        all_kernels.add(k)
    for (k, _) in sim_data:
        all_kernels.add(k)

    results = []  # (label, slope_ratio, sim_slope, hw_slope, r2_sim, r2_hw, n_points)
    detail_rows = []  # per-point details for --output

    for kernel in sorted(all_kernels):
        threshold = (
            args.min_fill if args.min_fill is not None else get_threshold(kernel)
        )

        if kernel in MULTI_PARAM_BENCHMARKS:
            # Discover all groups present in the data
            groups = set()
            for (k, size_str) in list(ref_data.keys()) + list(sim_data.keys()):
                if k != kernel:
                    continue
                parsed = parse_scaling_and_group(kernel, size_str)
                if parsed is not None:
                    groups.add(parsed[1])

            for group_key in sorted(groups):
                label = f"{kernel}[{group_key}]"
                res, details = _do_regression_on_group(
                    kernel, label, group_key, ref_data, sim_data, threshold, True
                )
                if res is not None:
                    results.append(res)
                    detail_rows.extend(details)
        else:
            # Single-parameter benchmark — original behavior
            label = kernel
            res, details = _do_regression_on_group(
                kernel, label, "", ref_data, sim_data, threshold, False
            )
            if res is not None:
                results.append(res)
                detail_rows.extend(details)

    # Print results table
    print("=" * 120)
    print("LINEAR REGRESSION COMPARISON: Simulator vs Hardware")
    print("=" * 120)
    print(
        f"{'Benchmark':<45s} {'Slope Ratio':>12s} {'Sim Slope':>14s} "
        f"{'HW Slope':>14s} {'R² Sim':>8s} {'R² HW':>8s} {'Points':>8s}"
    )
    print("-" * 120)

    for label, slope_ratio, sim_slope, hw_slope, sim_r2, hw_r2, n_points in results:
        sr_str = f"{slope_ratio:.4f}" if not math.isinf(slope_ratio) else "inf"
        print(
            f"{label:<45s} {sr_str:>12s} {sim_slope:>14.6e} "
            f"{hw_slope:>14.6e} {sim_r2:>8.4f} {hw_r2:>8.4f} {n_points:>8d}"
        )

    # Summary
    if results:
        ratios = [r[1] for r in results if not math.isinf(r[1])]
        if ratios:
            mean_ratio = sum(ratios) / len(ratios)
            sorted_ratios = sorted(ratios)
            n = len(sorted_ratios)
            if n % 2 == 0:
                median_ratio = (
                    sorted_ratios[n // 2 - 1] + sorted_ratios[n // 2]
                ) / 2
            else:
                median_ratio = sorted_ratios[n // 2]

            print()
            print("=" * 120)
            print("SUMMARY")
            print("=" * 120)
            print(f"  Benchmarks analyzed:    {len(results)}")
            print(f"  Mean slope ratio:       {mean_ratio:.4f}")
            print(f"  Median slope ratio:     {median_ratio:.4f}")
            print(
                f"  Ideal slope ratio:      1.0000 (simulator scales identically to hardware)"
            )
            print()
            print("  Interpretation:")
            print(
                "    slope_ratio > 1  =>  simulator time grows FASTER than hardware (sim too slow at large sizes)"
            )
            print(
                "    slope_ratio < 1  =>  simulator time grows SLOWER than hardware (sim too fast at large sizes)"
            )
            print(
                "    slope_ratio ~ 1  =>  simulator scaling matches hardware well"
            )
    else:
        print("\nNo benchmarks had enough data points for regression analysis.")

    # Write output CSV
    if args.output and detail_rows:
        with open(args.output, "w", newline="") as f:
            writer = csv.DictWriter(
                f,
                fieldnames=[
                    "kernel",
                    "group",
                    "label",
                    "problem_size",
                    "numeric_size",
                    "hw_ms",
                    "sim_ms",
                    "hw_pred_ms",
                    "sim_pred_ms",
                ],
            )
            writer.writeheader()
            for row in detail_rows:
                writer.writerow(
                    {
                        "kernel": row["kernel"],
                        "group": row["group"],
                        "label": row["label"],
                        "problem_size": row["problem_size"],
                        "numeric_size": f"{row['numeric_size']:.0f}",
                        "hw_ms": f"{row['hw_ms']:.6f}",
                        "sim_ms": f"{row['sim_ms']:.6f}",
                        "hw_pred_ms": f"{row['hw_pred_ms']:.6f}",
                        "sim_pred_ms": f"{row['sim_pred_ms']:.6f}",
                    }
                )
        print(f"\nDetailed results written to: {args.output}")


if __name__ == "__main__":
    main()
