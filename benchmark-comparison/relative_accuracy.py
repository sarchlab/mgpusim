#!/usr/bin/env python3
"""Relative accuracy metrics for simulator benchmarks.

Instead of absolute error (WMAPE), this script computes relative metrics
that measure whether the simulator correctly captures performance *trends*:

1. Spearman Rank Correlation — does sim rank benchmarks correctly?
2. Per-benchmark Scaling Accuracy — does sim capture how time grows with size?
3. Relative Speedup Error — does sim correctly predict speedup from small→large?
"""

import csv
import math
import os
import statistics
import sys


def parse_size_key(problem_size: str) -> float:
    """Extract a numeric sort key from a problem_size string.

    Handles formats like '1024x1024', '1024', '1024x1024x3', etc.
    Returns the product of all numeric components.
    """
    parts = problem_size.lower().replace("x", " ").split()
    product = 1.0
    for p in parts:
        try:
            product *= float(p)
        except ValueError:
            return float("inf")
    return product


def spearman_rank_correlation(x_ranks, y_ranks):
    """Compute Spearman's rank correlation coefficient."""
    n = len(x_ranks)
    if n < 3:
        return float("nan")
    d_sq_sum = sum((a - b) ** 2 for a, b in zip(x_ranks, y_ranks))
    rho = 1.0 - (6.0 * d_sq_sum) / (n * (n * n - 1))
    return rho


def rank_values(values):
    """Return ranks (1-based) for a list of values, handling ties by averaging."""
    indexed = sorted(enumerate(values), key=lambda t: t[1])
    ranks = [0.0] * len(values)
    i = 0
    while i < len(indexed):
        j = i
        while j < len(indexed) and indexed[j][1] == indexed[i][1]:
            j += 1
        avg_rank = (i + j + 1) / 2.0  # average rank for tied group
        for k in range(i, j):
            ranks[indexed[k][0]] = avg_rank
        i = j
    return ranks


def coefficient_of_variation(values):
    """Return the coefficient of variation (std/mean) for a list of values."""
    if len(values) < 2:
        return 0.0
    mean = statistics.mean(values)
    if mean == 0:
        return 0.0
    return statistics.stdev(values) / abs(mean)


def detect_data_quality_issues(group):
    """Detect data quality issues in a kernel's datapoints.

    Returns a list of flags (empty if no issues):
      - 'FLAT REAL DATA': real values have very low variation (CV < 0.1),
        meaning any ranking is noise.
      - 'NON-MONOTONIC REAL': the Spearman correlation between problem size
        and real runtime is < 0.5, meaning real data doesn't scale with size
        as expected.

    These flags indicate that per-kernel Spearman ρ is unreliable and
    should be excluded from the aggregate average.
    """
    flags = []
    real_vals = [r["real_ms"] for r in group]

    # Check 1: Coefficient of variation of real values
    cv = coefficient_of_variation(real_vals)
    if cv < 0.1:
        flags.append("FLAT REAL DATA")

    # Check 2: Spearman correlation between size and real runtime
    # If real data doesn't scale with size, per-kernel ρ is meaningless
    if len(group) >= 3:
        size_keys = [r["size_key"] for r in group]
        size_ranks = rank_values(size_keys)
        real_ranks = rank_values(real_vals)
        size_vs_real_rho = spearman_rank_correlation(size_ranks, real_ranks)
        if not math.isnan(size_vs_real_rho) and size_vs_real_rho < 0.5:
            if "FLAT REAL DATA" not in flags:
                flags.append("NON-MONOTONIC REAL")

    return flags


def main():
    csv_path = os.path.join(
        os.path.dirname(os.path.abspath(__file__)), "comparison_ci.csv"
    )
    if not os.path.exists(csv_path):
        print(f"ERROR: {csv_path} not found")
        sys.exit(1)

    with open(csv_path) as f:
        reader = csv.DictReader(f)
        rows = list(reader)

    # Filter to rows with both real and sim data
    valid = []
    for r in rows:
        if r["sim_ms"] and r["real_ms"]:
            try:
                sim = float(r["sim_ms"])
                real = float(r["real_ms"])
                if sim > 0 and real > 0:
                    valid.append(
                        {
                            "kernel": r["kernel_name"],
                            "size": r["problem_size"],
                            "real_ms": real,
                            "sim_ms": sim,
                            "size_key": parse_size_key(r["problem_size"]),
                        }
                    )
            except ValueError:
                pass

    if not valid:
        print("ERROR: No valid rows found")
        sys.exit(1)

    # ─── 1. Spearman Rank Correlation ───────────────────────────────────
    # Across ALL datapoints: does sim correctly rank which are faster/slower?
    real_vals = [r["real_ms"] for r in valid]
    sim_vals = [r["sim_ms"] for r in valid]
    real_ranks = rank_values(real_vals)
    sim_ranks = rank_values(sim_vals)
    overall_rho = spearman_rank_correlation(real_ranks, sim_ranks)

    # Also compute per-kernel Spearman (for kernels with ≥3 sizes)
    kernels = sorted(set(r["kernel"] for r in valid))
    kernel_groups = {}
    for r in valid:
        kernel_groups.setdefault(r["kernel"], []).append(r)

    per_kernel_rho = {}
    kernel_dq_flags = {}  # data quality flags per kernel
    for k in kernels:
        group = sorted(kernel_groups[k], key=lambda r: r["size_key"])
        if len(group) >= 3:
            kr = rank_values([r["real_ms"] for r in group])
            ks = rank_values([r["sim_ms"] for r in group])
            per_kernel_rho[k] = spearman_rank_correlation(kr, ks)
            flags = detect_data_quality_issues(group)
            if flags:
                kernel_dq_flags[k] = flags

    # ─── 2. Per-benchmark Scaling Accuracy ──────────────────────────────
    # For consecutive sizes: ratio_sim / ratio_real → ideally 1.0
    scaling_errors = {}
    for k in kernels:
        group = sorted(kernel_groups[k], key=lambda r: r["size_key"])
        if len(group) < 2:
            continue
        ratios = []
        for i in range(1, len(group)):
            real_ratio = group[i]["real_ms"] / group[i - 1]["real_ms"]
            sim_ratio = group[i]["sim_ms"] / group[i - 1]["sim_ms"]
            if real_ratio > 0:
                ratios.append(sim_ratio / real_ratio)
        if ratios:
            mean_ratio = sum(ratios) / len(ratios)
            mean_abs_err = sum(abs(r - 1.0) for r in ratios) / len(ratios)
            scaling_errors[k] = {
                "mean_ratio": mean_ratio,
                "mean_abs_err": mean_abs_err,
                "n_pairs": len(ratios),
            }

    # ─── 3. Relative Speedup Error ──────────────────────────────────────
    # Compare smallest vs largest: sim_speedup vs real_speedup
    speedup_errors = {}
    for k in kernels:
        group = sorted(kernel_groups[k], key=lambda r: r["size_key"])
        if len(group) < 2:
            continue
        smallest = group[0]
        largest = group[-1]
        real_speedup = largest["real_ms"] / smallest["real_ms"]
        sim_speedup = largest["sim_ms"] / smallest["sim_ms"]
        if real_speedup > 0:
            speedup_errors[k] = {
                "real_speedup": real_speedup,
                "sim_speedup": sim_speedup,
                "ratio": sim_speedup / real_speedup,
                "smallest": smallest["size"],
                "largest": largest["size"],
            }

    # ─── Print Report ───────────────────────────────────────────────────
    print("=" * 72)
    print("  RELATIVE ACCURACY METRICS REPORT")
    print("=" * 72)
    print()
    print(f"  Data: {len(valid)} valid datapoints across {len(kernels)} kernels")
    print()

    # Section 1
    print("─" * 72)
    print("  1. SPEARMAN RANK CORRELATION")
    print("─" * 72)
    print(f"  Overall (all {len(valid)} points): ρ = {overall_rho:.4f}")
    print(f"  (1.0 = perfect rank agreement, 0.0 = no correlation)")
    print()
    if per_kernel_rho:
        print(f"  Per-kernel (≥3 sizes, {len(per_kernel_rho)} kernels):")
        sorted_rho = sorted(per_kernel_rho.items(), key=lambda t: t[1])
        for k, rho in sorted_rho:
            n = len(kernel_groups[k])
            dq = kernel_dq_flags.get(k, [])
            flag = ""
            if dq:
                flag = "  [DATA QUALITY: " + ", ".join(dq) + "] — excluded"
            elif rho < 0.7:
                flag = " ⚠"
            print(f"    {k:25s}  ρ = {rho:+.4f}  (n={n}){flag}")

        # Compute average ρ excluding data-quality-flagged kernels
        clean_rho = {
            k: v for k, v in per_kernel_rho.items() if k not in kernel_dq_flags
        }
        avg_rho_all = sum(per_kernel_rho.values()) / len(per_kernel_rho)
        good = sum(1 for v in per_kernel_rho.values() if v >= 0.9)
        print()
        if kernel_dq_flags:
            print(
                f"  Data quality exclusions: {len(kernel_dq_flags)} kernel(s) "
                f"({', '.join(sorted(kernel_dq_flags.keys()))})"
            )
            for k, flags in sorted(kernel_dq_flags.items()):
                real_vals = [r["real_ms"] for r in kernel_groups[k]]
                cv = coefficient_of_variation(real_vals)
                print(f"    {k}: {', '.join(flags)} (CV={cv:.4f})")
            print()
        if clean_rho:
            avg_rho_clean = sum(clean_rho.values()) / len(clean_rho)
            good_clean = sum(1 for v in clean_rho.values() if v >= 0.9)
            print(f"  Average per-kernel ρ (all):        {avg_rho_all:.4f}")
            print(
                f"  Average per-kernel ρ (filtered):   {avg_rho_clean:.4f}  "
                f"({len(clean_rho)} kernels)"
            )
            print(f"  Kernels with ρ ≥ 0.9: {good_clean}/{len(clean_rho)}")
        else:
            print(f"  Average per-kernel ρ: {avg_rho_all:.4f}")
            print(f"  Kernels with ρ ≥ 0.9: {good}/{len(per_kernel_rho)}")
    print()

    # Section 2
    print("─" * 72)
    print("  2. PER-BENCHMARK SCALING ACCURACY")
    print("─" * 72)
    print("  For consecutive sizes: sim_ratio / real_ratio (ideal = 1.0)")
    print()
    if scaling_errors:
        sorted_se = sorted(
            scaling_errors.items(), key=lambda t: t[1]["mean_abs_err"], reverse=True
        )
        print(f"  {'Kernel':25s} {'Mean Ratio':>12s} {'Mean |Err|':>12s} {'Pairs':>6s}")
        print(f"  {'─' * 25} {'─' * 12} {'─' * 12} {'─' * 6}")
        for k, v in sorted_se:
            flag = " ⚠" if v["mean_abs_err"] > 0.5 else ""
            print(
                f"  {k:25s} {v['mean_ratio']:12.4f} {v['mean_abs_err']:12.4f} {v['n_pairs']:6d}{flag}"
            )
        overall_mae = sum(v["mean_abs_err"] for v in scaling_errors.values()) / len(
            scaling_errors
        )
        print()
        print(f"  Overall mean |scaling error|: {overall_mae:.4f}")
        good_scale = sum(
            1 for v in scaling_errors.values() if v["mean_abs_err"] < 0.2
        )
        print(
            f"  Kernels with mean |err| < 0.2: {good_scale}/{len(scaling_errors)}"
        )
    print()

    # Section 3
    print("─" * 72)
    print("  3. RELATIVE SPEEDUP ERROR (smallest → largest)")
    print("─" * 72)
    print("  sim_speedup / real_speedup (ideal = 1.0)")
    print()
    if speedup_errors:
        sorted_sp = sorted(
            speedup_errors.items(),
            key=lambda t: abs(t[1]["ratio"] - 1.0),
            reverse=True,
        )
        print(
            f"  {'Kernel':25s} {'Real Spdup':>11s} {'Sim Spdup':>11s} {'Ratio':>8s}  Sizes"
        )
        print(f"  {'─' * 25} {'─' * 11} {'─' * 11} {'─' * 8}  {'─' * 20}")
        for k, v in sorted_sp:
            flag = " ⚠" if abs(v["ratio"] - 1.0) > 0.5 else ""
            print(
                f"  {k:25s} {v['real_speedup']:11.3f} {v['sim_speedup']:11.3f} {v['ratio']:8.4f}  {v['smallest']}→{v['largest']}{flag}"
            )
        overall_ratio_err = sum(
            abs(v["ratio"] - 1.0) for v in speedup_errors.values()
        ) / len(speedup_errors)
        print()
        print(f"  Overall mean |speedup ratio - 1|: {overall_ratio_err:.4f}")
        good_sp = sum(
            1 for v in speedup_errors.values() if abs(v["ratio"] - 1.0) < 0.3
        )
        print(
            f"  Kernels with |ratio-1| < 0.3: {good_sp}/{len(speedup_errors)}"
        )
    print()

    print("=" * 72)
    print("  SUMMARY")
    print("=" * 72)
    print(f"  Spearman ρ (overall):         {overall_rho:.4f}")
    if per_kernel_rho:
        avg_rho_all = sum(per_kernel_rho.values()) / len(per_kernel_rho)
        clean_rho = {
            k: v for k, v in per_kernel_rho.items() if k not in kernel_dq_flags
        }
        if clean_rho and kernel_dq_flags:
            avg_rho_clean = sum(clean_rho.values()) / len(clean_rho)
            print(f"  Spearman ρ (per-kernel avg):  {avg_rho_clean:.4f}  "
                  f"(filtered; {avg_rho_all:.4f} incl. data-quality kernels)")
        else:
            print(f"  Spearman ρ (per-kernel avg):  {avg_rho_all:.4f}")
    if scaling_errors:
        overall_mae = sum(v["mean_abs_err"] for v in scaling_errors.values()) / len(
            scaling_errors
        )
        print(f"  Mean scaling |error|:         {overall_mae:.4f}")
    if speedup_errors:
        overall_ratio_err = sum(
            abs(v["ratio"] - 1.0) for v in speedup_errors.values()
        ) / len(speedup_errors)
        print(f"  Mean speedup |ratio - 1|:     {overall_ratio_err:.4f}")
    print()


if __name__ == "__main__":
    main()
