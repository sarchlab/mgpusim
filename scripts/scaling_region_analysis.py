#!/usr/bin/env python3
"""
Scaling-Region Accuracy Analysis for MGPUSim MI300A Validation.

For each kernel, classifies matched data points as:
  - "overhead": HW time < 2× min HW time for that kernel
    (GPU launch overhead dominates; timing is noisy)
  - "scaling":  HW time ≥ 2× min HW time
    (computation dominates; scaling behaviour is meaningful)

Then computes per-region accuracy metrics:
  - WMAPE  (weighted mean absolute percentage error, weighted by HW time)
  - Spearman ρ  (rank-order correlation across all points in region)
  - Mean scaling error  (avg |sim_ratio/hw_ratio − 1| for consecutive sizes)
  - Point count

Usage:
    python3 scripts/scaling_region_analysis.py                       # summary to stdout
    python3 scripts/scaling_region_analysis.py --csv out.csv         # also write CSV
    python3 scripts/scaling_region_analysis.py --input path/to/comparison_ci.csv
"""

from __future__ import annotations

import argparse
import csv
import sys
from pathlib import Path

import numpy as np


def load_matched_points(csv_path: str | Path) -> list[dict]:
    """Load comparison_ci.csv and return only matched rows (with sim data)."""
    rows = []
    with open(csv_path, newline="") as f:
        reader = csv.DictReader(f)
        for r in reader:
            if r["sim_ms"] and r["sim_ms"].strip():
                rows.append(
                    {
                        "kernel": r["kernel_name"],
                        "problem_size": r["problem_size"],
                        "hw_ms": float(r["real_ms"]),
                        "sim_ms": float(r["sim_ms"]),
                    }
                )
    return rows


def classify_points(
    rows: list[dict], threshold: float = 2.0
) -> list[dict]:
    """Add 'region' key to each row: 'overhead' or 'scaling'."""
    # Compute per-kernel min HW time
    kernel_min: dict[str, float] = {}
    for r in rows:
        k = r["kernel"]
        if k not in kernel_min or r["hw_ms"] < kernel_min[k]:
            kernel_min[k] = r["hw_ms"]

    for r in rows:
        min_t = kernel_min[r["kernel"]]
        r["region"] = "scaling" if r["hw_ms"] >= threshold * min_t else "overhead"
        r["kernel_min_hw"] = min_t
    return rows


def wmape(hw: np.ndarray, sim: np.ndarray) -> float:
    """Weighted mean absolute percentage error."""
    total_hw = hw.sum()
    if total_hw == 0:
        return float("nan")
    return float(np.sum(np.abs(sim - hw)) / total_hw)


def spearman_rho(hw: np.ndarray, sim: np.ndarray) -> float:
    """Spearman rank correlation."""
    if len(hw) < 3:
        return float("nan")
    from scipy.stats import spearmanr  # type: ignore

    rho, _ = spearmanr(hw, sim)
    return float(rho)


def mean_scaling_error_for_kernel(
    kernel_rows: list[dict],
) -> float:
    """Mean |sim_ratio/hw_ratio − 1| for consecutive problem sizes of one kernel."""
    if len(kernel_rows) < 2:
        return float("nan")
    # Sort by HW time (proxy for problem size)
    s = sorted(kernel_rows, key=lambda r: r["hw_ms"])
    errors = []
    for i in range(1, len(s)):
        hw_ratio = s[i]["hw_ms"] / s[i - 1]["hw_ms"] if s[i - 1]["hw_ms"] > 0 else 0
        sim_ratio = (
            s[i]["sim_ms"] / s[i - 1]["sim_ms"] if s[i - 1]["sim_ms"] > 0 else 0
        )
        if hw_ratio > 0:
            errors.append(abs(sim_ratio / hw_ratio - 1))
    return float(np.mean(errors)) if errors else float("nan")


def compute_region_metrics(rows: list[dict], region: str) -> dict:
    """Compute aggregate metrics for a given region."""
    pts = [r for r in rows if r["region"] == region]
    if not pts:
        return {
            "region": region,
            "n_points": 0,
            "n_kernels": 0,
            "wmape": float("nan"),
            "spearman": float("nan"),
            "mean_scaling_error": float("nan"),
        }
    hw = np.array([r["hw_ms"] for r in pts])
    sim = np.array([r["sim_ms"] for r in pts])

    # Per-kernel scaling errors, then average
    kernels_in_region: dict[str, list[dict]] = {}
    for r in pts:
        kernels_in_region.setdefault(r["kernel"], []).append(r)

    kernel_scaling_errors = [
        mean_scaling_error_for_kernel(krows)
        for krows in kernels_in_region.values()
        if len(krows) >= 2
    ]
    mse = float(np.nanmean(kernel_scaling_errors)) if kernel_scaling_errors else float("nan")

    return {
        "region": region,
        "n_points": len(pts),
        "n_kernels": len(kernels_in_region),
        "wmape": wmape(hw, sim),
        "spearman": spearman_rho(hw, sim),
        "mean_scaling_error": mse,
    }


def per_kernel_breakdown(rows: list[dict]) -> list[dict]:
    """Per-kernel point counts and region-specific WMAPE."""
    kernels: dict[str, list[dict]] = {}
    for r in rows:
        kernels.setdefault(r["kernel"], []).append(r)

    result = []
    for k in sorted(kernels):
        krows = kernels[k]
        oh = [r for r in krows if r["region"] == "overhead"]
        sc = [r for r in krows if r["region"] == "scaling"]
        hw_oh = np.array([r["hw_ms"] for r in oh]) if oh else np.array([])
        sim_oh = np.array([r["sim_ms"] for r in oh]) if oh else np.array([])
        hw_sc = np.array([r["hw_ms"] for r in sc]) if sc else np.array([])
        sim_sc = np.array([r["sim_ms"] for r in sc]) if sc else np.array([])

        result.append(
            {
                "kernel": k,
                "total": len(krows),
                "overhead": len(oh),
                "scaling": len(sc),
                "wmape_overhead": wmape(hw_oh, sim_oh) if oh else float("nan"),
                "wmape_scaling": wmape(hw_sc, sim_sc) if sc else float("nan"),
                "spearman_scaling": spearman_rho(hw_sc, sim_sc) if len(sc) >= 3 else float("nan"),
            }
        )
    return result


def fmt(val: float, pct: bool = False) -> str:
    """Format a float nicely."""
    if val != val:  # NaN check
        return "—"
    if pct:
        return f"{val * 100:.1f}%"
    return f"{val:.4f}"


def print_summary(overall: dict, overhead: dict, scaling: dict) -> None:
    """Print aggregate summary table."""
    print("\n" + "=" * 72)
    print("SCALING-REGION ACCURACY ANALYSIS")
    print("=" * 72)
    header = f"{'Region':<12} {'Points':>7} {'Kernels':>8} {'WMAPE':>9} {'Spearman':>10} {'MeanScalErr':>12}"
    print(header)
    print("-" * 72)
    for m in [overall, overhead, scaling]:
        print(
            f"{m['region']:<12} {m['n_points']:>7d} {m['n_kernels']:>8d} "
            f"{fmt(m['wmape'], pct=True):>9s} {fmt(m['spearman']):>10s} "
            f"{fmt(m['mean_scaling_error'], pct=True):>12s}"
        )
    print("-" * 72)
    print()


def print_kernel_table(breakdown: list[dict]) -> None:
    """Print per-kernel breakdown."""
    header = (
        f"{'Kernel':<20} {'Total':>5} {'OH':>4} {'Scal':>5} "
        f"{'WMAPE(OH)':>10} {'WMAPE(Sc)':>10} {'Spear(Sc)':>10}"
    )
    print(header)
    print("-" * 72)
    for b in breakdown:
        print(
            f"{b['kernel']:<20} {b['total']:>5d} {b['overhead']:>4d} "
            f"{b['scaling']:>5d} {fmt(b['wmape_overhead'], pct=True):>10s} "
            f"{fmt(b['wmape_scaling'], pct=True):>10s} "
            f"{fmt(b['spearman_scaling']):>10s}"
        )
    print()


def write_csv_output(
    csv_path: str,
    overall: dict,
    overhead_m: dict,
    scaling_m: dict,
    breakdown: list[dict],
) -> None:
    """Write results to CSV files."""
    # Summary CSV
    with open(csv_path, "w", newline="") as f:
        w = csv.writer(f)
        w.writerow(["region", "n_points", "n_kernels", "wmape", "spearman", "mean_scaling_error"])
        for m in [overall, overhead_m, scaling_m]:
            w.writerow(
                [m["region"], m["n_points"], m["n_kernels"],
                 m["wmape"], m["spearman"], m["mean_scaling_error"]]
            )

    # Per-kernel breakdown CSV
    bk_path = csv_path.replace(".csv", "_per_kernel.csv")
    with open(bk_path, "w", newline="") as f:
        w = csv.writer(f)
        w.writerow(
            ["kernel", "total", "overhead", "scaling",
             "wmape_overhead", "wmape_scaling", "spearman_scaling"]
        )
        for b in breakdown:
            w.writerow(
                [b["kernel"], b["total"], b["overhead"], b["scaling"],
                 b["wmape_overhead"], b["wmape_scaling"], b["spearman_scaling"]]
            )
    print(f"Written: {csv_path}, {bk_path}")


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Scaling-region accuracy analysis for MGPUSim MI300A validation"
    )
    parser.add_argument(
        "--input",
        default="benchmark-comparison/comparison_ci.csv",
        help="Path to comparison_ci.csv (default: benchmark-comparison/comparison_ci.csv)",
    )
    parser.add_argument(
        "--csv",
        default=None,
        help="Optional output CSV path for summary metrics",
    )
    parser.add_argument(
        "--threshold",
        type=float,
        default=2.0,
        help="Overhead threshold multiplier (default: 2.0 — points with HW_time < threshold × min_HW_time are classified as overhead)",
    )
    args = parser.parse_args()

    # Load and classify
    rows = load_matched_points(args.input)
    if not rows:
        print("ERROR: No matched data points found.", file=sys.stderr)
        sys.exit(1)

    rows = classify_points(rows, threshold=args.threshold)

    # Overall metrics (all matched points treated as one region)
    hw_all = np.array([r["hw_ms"] for r in rows])
    sim_all = np.array([r["sim_ms"] for r in rows])
    kernels_all: dict[str, list[dict]] = {}
    for r in rows:
        kernels_all.setdefault(r["kernel"], []).append(r)
    kernel_se_all = [
        mean_scaling_error_for_kernel(krows)
        for krows in kernels_all.values()
        if len(krows) >= 2
    ]
    overall = {
        "region": "all",
        "n_points": len(rows),
        "n_kernels": len(kernels_all),
        "wmape": wmape(hw_all, sim_all),
        "spearman": spearman_rho(hw_all, sim_all),
        "mean_scaling_error": float(np.nanmean(kernel_se_all)) if kernel_se_all else float("nan"),
    }

    overhead_m = compute_region_metrics(rows, "overhead")
    scaling_m = compute_region_metrics(rows, "scaling")

    # Print results
    print_summary(overall, overhead_m, scaling_m)

    breakdown = per_kernel_breakdown(rows)
    print_kernel_table(breakdown)

    # Key findings
    print("KEY FINDINGS:")
    print(f"  • Overhead region: {overhead_m['n_points']} points across {overhead_m['n_kernels']} kernels")
    print(f"  • Scaling region:  {scaling_m['n_points']} points across {scaling_m['n_kernels']} kernels")
    if not np.isnan(scaling_m["wmape"]) and not np.isnan(overhead_m["wmape"]):
        if scaling_m["wmape"] < overhead_m["wmape"]:
            improvement = (1 - scaling_m["wmape"] / overhead_m["wmape"]) * 100
            print(f"  • WMAPE improves by {improvement:.0f}% when focusing on scaling region")
        else:
            print(f"  • WMAPE is similar or worse in scaling region vs overhead")
    if not np.isnan(scaling_m["spearman"]):
        print(f"  • Spearman ρ in scaling region: {scaling_m['spearman']:.4f}")
    print()

    # Write CSV if requested
    if args.csv:
        write_csv_output(args.csv, overall, overhead_m, scaling_m, breakdown)


if __name__ == "__main__":
    main()
