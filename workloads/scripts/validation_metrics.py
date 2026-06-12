#!/usr/bin/env python3
"""
validation_metrics.py — 3-tier validation dashboard for MGPUSim.

Reads comparison_ci.csv and computes per-kernel Spearman rank-correlation,
speedup ratios, and bottleneck-category coverage.  Outputs a formatted
dashboard showing Tier 1/2/3 pass/fail.

Data-quality filters:
  - Tier 1: exclude kernels where real_ms coefficient of variation < --min-cv
    (launch-overhead / profiler noise dominated).
  - Tier 2: exclude flat-data kernels (real execution time < 5 µs for all
    data points).  Requires at least 2 matched data points.

Usage:
    python3 workloads/scripts/validation_metrics.py \
        --csv benchmark-comparison/comparison_ci.csv

Requirements: Python 3.8+, scipy (for scipy.stats.spearmanr).
"""

import argparse
import csv
import math
import re
import sys
from collections import defaultdict
from typing import Dict, List, Tuple

from scipy.stats import spearmanr

# ──────────────────────────────────────────────────────────────────────────────
# Bottleneck-category mapping (12 categories from accuracy-metrics.md)
# Keys are category names.  Values are lists of lowercased kernel_name
# substrings / prefixes that appear in comparison_ci.csv.
# A kernel may match multiple patterns; the *first* match in iteration
# order wins.  Categories with no patterns are "not covered".
# ──────────────────────────────────────────────────────────────────────────────
BOTTLENECK_CATEGORIES: Dict[str, List[str]] = {
    "Compute-bound (FP32)": [
        "2dconvolution", "3dconvolution", "atax", "bicg",
        "mvt", "gemver", "gesummv", "fdtd2d", "srad",
        "gemm", "sgemm", "matmul", "fir",
        "correlation", "covariance",
        "hotspot", "lavamd", "lbm", "s3d",
        "stencil_kernel", "fft",
        "2mm", "3mm", "syr2k", "syrk",
        "gramschmidt", "lud", "gaussian",
        "bs", "md", "ep",
        "maxflops", "fp32_fma", "occupancy_fma",
    ],
    "Compute-bound (FP64)": [
        "fp64_fma",
    ],
    "Compute-bound (INT)": [
        "parboil_sad", "computesad", "findminsad", "sad",
        "hist", "histogram", "huffman", "md5hash",
        "int_mad", "bitonicsort", "sort",
    ],
    "Memory bandwidth (streaming)": [
        "triad", "global_bw_copy",
        "devicememory_read", "devicememory_write",
        "busspeeddownload", "busspeedreadback",
    ],
    "Memory bandwidth (gather/scatter)": [
        "nn", "adjust_weights", "spmv_csr",
    ],
    "Cache-sensitive (L1)": [
        "tiled_gemm", "l1_cache_bw", "shared_bw",
    ],
    "Cache-sensitive (L2)": [
        "l2_cache_bw", "mem_latency_chase",
    ],
    "Kernel launch overhead": [],
    "Synchronization": [
        "reduction", "dotproduct", "prefixsum", "scan",
    ],
    "Mixed compute+memory": [
        "gelu", "relu", "softmax", "maxpool",
        "layernorm", "layerforward", "rope",
        "naive_attention", "fused_swiglu",
    ],
    "Irregular access": [
        "bfs", "pagerank", "sssp", "bh",
    ],
    "Multi-kernel": [],
}

# Reverse look-up: kernel_name -> category (first match wins).
def _classify_kernel(kernel_name: str) -> str:
    """Return the bottleneck category for *kernel_name*, or 'Uncategorised'."""
    kn = kernel_name.lower()
    for category, patterns in BOTTLENECK_CATEGORIES.items():
        for pat in patterns:
            if kn == pat or kn.startswith(pat):
                return category
    return "Uncategorised"


# ──────────────────────────────────────────────────────────────────────────────
# Helpers
# ──────────────────────────────────────────────────────────────────────────────

def _parse_problem_size(ps: str) -> float:
    """Extract the first number from a problem_size string.

    Examples:
        '1024'       -> 1024.0
        '1024x1024'  -> 1024.0
        '16384-elem' -> 16384.0
    """
    m = re.search(r"(\d+(?:\.\d+)?)", ps)
    if m:
        return float(m.group(1))
    raise ValueError(f"Cannot parse numeric value from problem_size={ps!r}")


def _read_comparison_csv(path: str) -> Dict[str, List[Tuple[float, float, float]]]:
    """Read comparison_ci.csv and return {kernel_name: [(size, real_ms, sim_ms), ...]}.

    Only rows where *both* real_ms and sim_ms are non-empty are included.
    """
    kernels: Dict[str, List[Tuple[float, float, float]]] = defaultdict(list)
    with open(path, newline="") as fh:
        reader = csv.DictReader(fh)
        for row in reader:
            kname = row["kernel_name"].strip().lower()
            real_s = row.get("real_ms", "").strip()
            sim_s = row.get("sim_ms", "").strip()
            if not real_s or not sim_s:
                continue
            try:
                real_ms = float(real_s)
                sim_ms = float(sim_s)
                size = _parse_problem_size(row["problem_size"].strip())
            except (ValueError, KeyError):
                continue
            if real_ms > 0 and sim_ms > 0:
                kernels[kname].append((size, real_ms, sim_ms))
    # Sort each kernel's entries by problem size
    for kname in kernels:
        kernels[kname].sort(key=lambda t: t[0])
    return dict(kernels)


# ──────────────────────────────────────────────────────────────────────────────
# Metric computations
# ──────────────────────────────────────────────────────────────────────────────

def compute_cv(values: List[float]) -> float:
    """Coefficient of variation: std / mean.

    Returns 0.0 if mean is zero or there are fewer than 2 values.
    """
    n = len(values)
    if n < 2:
        return 0.0
    mean = sum(values) / n
    if mean == 0:
        return 0.0
    variance = sum((v - mean) ** 2 for v in values) / (n - 1)  # sample variance
    std = math.sqrt(variance)
    return std / mean


def compute_spearman(points: List[Tuple[float, float, float]]) -> float:
    """Spearman ρ between real_ms and sim_ms columns."""
    reals = [p[1] for p in points]
    sims = [p[2] for p in points]
    rho, _ = spearmanr(reals, sims)
    return rho


def compute_speedup_ratio(points: List[Tuple[float, float, float]]) -> float:
    """Speedup ratio: sim_speedup / real_speedup.

    Points must be sorted by problem size.  Uses the smallest and largest
    problem sizes:
        sim_speedup  = sim_ms(largest)  / sim_ms(smallest)
        real_speedup = real_ms(largest) / real_ms(smallest)
        speedup_ratio = sim_speedup / real_speedup
    """
    if len(points) < 2:
        return float("nan")
    smallest = points[0]
    largest = points[-1]
    if smallest[1] == 0 or smallest[2] == 0:
        return float("nan")
    real_speedup = largest[1] / smallest[1]
    sim_speedup = largest[2] / smallest[2]
    if real_speedup == 0:
        return float("nan")
    return sim_speedup / real_speedup


# ──────────────────────────────────────────────────────────────────────────────
# Tier evaluation
# ──────────────────────────────────────────────────────────────────────────────

# Flat-data threshold: kernels where all real_ms < 5 µs (0.005 ms)
_FLAT_DATA_THRESHOLD_MS = 0.005


def evaluate_tiers(
    kernels: Dict[str, List[Tuple[float, float, float]]],
    min_points: int = 3,
    min_cv: float = 0.02,
) -> dict:
    """Compute all three tiers and return a results dict.

    Filters applied:
      - Tier 1: kernels with real_ms CV < min_cv are excluded.
                 Requires at least *min_points* (default 3) matched data points.
      - Tier 2: flat-data kernels (all real_ms < 5 µs) are excluded.
                 Requires at least 2 matched data points.
    """

    # ── Per-kernel metrics ───────────────────────────────────────────────
    spearman_results: Dict[str, float] = {}
    speedup_ratio_results: Dict[str, float] = {}
    kernel_categories: Dict[str, str] = {}
    kernel_cv: Dict[str, float] = {}
    kernel_flat: Dict[str, bool] = {}  # True if all real_ms < 5 µs

    for kname, pts in kernels.items():
        kernel_categories[kname] = _classify_kernel(kname)
        reals = [p[1] for p in pts]
        cv = compute_cv(reals)
        kernel_cv[kname] = cv
        kernel_flat[kname] = all(r < _FLAT_DATA_THRESHOLD_MS for r in reals)

        # Tier 1: need >= min_points data points (default 3)
        if len(pts) >= min_points:
            spearman_results[kname] = compute_spearman(pts)

        # Tier 2: need >= 2 data points
        if len(pts) >= 2:
            speedup_ratio_results[kname] = compute_speedup_ratio(pts)

    # ── Tier 1: Spearman ρ ≥ 0.7 for ≥80% of eligible kernels ───────
    # Exclude kernels with low CV (real timing is flat / noise-dominated)
    tier1_excluded: Dict[str, str] = {}  # kname -> reason
    tier1_eligible: Dict[str, float] = {}
    for k, v in spearman_results.items():
        if math.isnan(v):
            continue
        if kernel_cv.get(k, 0.0) < min_cv:
            tier1_excluded[k] = "low CV"
            continue
        tier1_eligible[k] = v

    tier1_pass_kernels = {k for k, v in tier1_eligible.items() if v >= 0.7}
    tier1_total = len(tier1_eligible)
    tier1_passing = len(tier1_pass_kernels)
    tier1_frac = tier1_passing / tier1_total if tier1_total else 0.0
    tier1_ok = tier1_frac >= 0.80

    # ── Tier 2: speedup ratio ∈ [0.5, 2.0] for ≥55% of eligible kernels
    # Exclude flat-data kernels (all real_ms < 5 µs)
    tier2_excluded: Dict[str, str] = {}  # kname -> reason
    tier2_eligible: Dict[str, float] = {}
    for k, v in speedup_ratio_results.items():
        if math.isnan(v):
            continue
        if kernel_flat.get(k, False):
            tier2_excluded[k] = "flat data (<5\u00b5s)"
            continue
        tier2_eligible[k] = v

    tier2_pass_kernels = {k for k, v in tier2_eligible.items() if 0.5 <= v <= 2.0}
    tier2_total = len(tier2_eligible)
    tier2_passing = len(tier2_pass_kernels)
    tier2_frac = tier2_passing / tier2_total if tier2_total else 0.0
    tier2_ok = tier2_frac >= 0.55

    # ── Tier 3: ≥8/12 categories with ≥1 kernel having <100% error ──
    # A kernel has "<100% error" if its median absolute percentage error
    # (|sim_ms - real_ms| / real_ms) is below 1.0 (100%).
    # Categories without benchmarks are counted as NOT COVERED.
    cat_pass: Dict[str, bool] = {}
    cat_kernels: Dict[str, List[str]] = defaultdict(list)
    for kname, pts in kernels.items():
        cat = kernel_categories.get(kname, _classify_kernel(kname))
        if cat == "Uncategorised":
            continue
        errors = [abs(p[2] / p[1] - 1.0) for p in pts if p[1] > 0]
        if not errors:
            continue
        cat_kernels[cat].append(kname)
        median_error = sorted(errors)[len(errors) // 2]
        if median_error < 1.0:
            cat_pass[cat] = True
        elif cat not in cat_pass:
            cat_pass[cat] = False

    tier3_cats_total = 12
    tier3_cats_pass = sum(1 for v in cat_pass.values() if v)
    tier3_ok = tier3_cats_pass >= 8

    return {
        "tier1": {
            "pass": tier1_ok,
            "passing": tier1_passing,
            "total": tier1_total,
            "fraction": tier1_frac,
            "details": {k: spearman_results[k] for k in sorted(tier1_eligible)},
            "excluded": {k: tier1_excluded[k] for k in sorted(tier1_excluded)},
            "excluded_values": {k: spearman_results[k] for k in sorted(tier1_excluded)},
        },
        "tier2": {
            "pass": tier2_ok,
            "passing": tier2_passing,
            "total": tier2_total,
            "fraction": tier2_frac,
            "details": {k: speedup_ratio_results[k] for k in sorted(tier2_eligible)},
            "excluded": {k: tier2_excluded[k] for k in sorted(tier2_excluded)},
            "excluded_values": {k: speedup_ratio_results[k] for k in sorted(tier2_excluded)},
        },
        "tier3": {
            "pass": tier3_ok,
            "cats_pass": tier3_cats_pass,
            "cats_total": tier3_cats_total,
            "category_status": {
                cat: cat_pass.get(cat, False)
                for cat in BOTTLENECK_CATEGORIES
            },
            "category_kernels": dict(cat_kernels),
        },
        "kernel_categories": kernel_categories,
        "kernel_cv": kernel_cv,
    }


# ──────────────────────────────────────────────────────────────────────────────
# Dashboard output
# ──────────────────────────────────────────────────────────────────────────────

def _tag(ok: bool) -> str:
    return "\u2705 PASS" if ok else "\u274c FAIL"


def print_dashboard(results: dict) -> None:
    """Print a human-readable 3-tier validation dashboard."""
    W = 72
    print("=" * W)
    print(" MGPUSim 3-Tier Validation Dashboard".center(W))
    print("=" * W)

    # ── Tier 1 ────────────────────────────────────────────────────────
    t1 = results["tier1"]
    print(f"\n{'\u2500' * W}")
    print(f"  Tier 1 \u2014 Rank-order accuracy (Spearman \u03c1 \u2265 0.7)   [{_tag(t1['pass'])}]")
    print(f"  Threshold: \u226580% of kernels | Result: {t1['passing']}/{t1['total']}"
          f" ({t1['fraction']:.0%})")
    print(f"{'\u2500' * W}")
    for kname, rho in t1["details"].items():
        status = "\u2713" if rho >= 0.7 else "\u2717"
        print(f"    {status}  {kname:<35s}  \u03c1 = {rho:+.4f}")
    # Show excluded kernels
    for kname, reason in t1["excluded"].items():
        val = t1["excluded_values"].get(kname, float("nan"))
        print(f"    -  {kname:<35s}  \u03c1 = {val:+.4f}  (excluded: {reason})")

    # ── Tier 2 ────────────────────────────────────────────────────────
    t2 = results["tier2"]
    print(f"\n{'\u2500' * W}")
    print(f"  Tier 2 \u2014 Speedup ratio \u2208 [0.5, 2.0]               [{_tag(t2['pass'])}]")
    print(f"  Threshold: \u226555% of kernels | Result: {t2['passing']}/{t2['total']}"
          f" ({t2['fraction']:.0%})")
    print(f"{'\u2500' * W}")
    for kname, ratio in t2["details"].items():
        status = "\u2713" if 0.5 <= ratio <= 2.0 else "\u2717"
        print(f"    {status}  {kname:<35s}  speedup_ratio = {ratio:.4f}")
    # Show excluded kernels
    for kname, reason in t2["excluded"].items():
        val = t2["excluded_values"].get(kname, float("nan"))
        print(f"    -  {kname:<35s}  speedup_ratio = {val:.4f}  (excluded: {reason})")

    # ── Tier 3 ────────────────────────────────────────────────────────
    t3 = results["tier3"]
    print(f"\n{'\u2500' * W}")
    print(f"  Tier 3 \u2014 Bottleneck-category coverage              [{_tag(t3['pass'])}]")
    print(f"  Threshold: \u22658/12 categories | Result: {t3['cats_pass']}/{t3['cats_total']}")
    print(f"{'\u2500' * W}")
    for cat, ok in t3["category_status"].items():
        kns = t3["category_kernels"].get(cat, [])
        if not kns:
            status = "-"
            label = "NOT COVERED"
        elif ok:
            status = "\u2713"
            label = "PASS (<100% err)"
        else:
            status = "\u2717"
            label = "FAIL (\u2265100% err)"
        kns_str = ", ".join(sorted(kns)[:5])
        if len(kns) > 5:
            kns_str += f", \u2026 (+{len(kns)-5})"
        print(f"    {status}  {cat:<35s}  {label:<18s}  [{kns_str}]")

    # ── Overall ───────────────────────────────────────────────────────
    overall = t1["pass"] and t2["pass"] and t3["pass"]
    print(f"\n{'=' * W}")
    print(f"  Overall: {_tag(overall)}")
    print(f"{'=' * W}\n")


# ──────────────────────────────────────────────────────────────────────────────
# Main
# ──────────────────────────────────────────────────────────────────────────────

def main() -> None:
    parser = argparse.ArgumentParser(
        description="3-tier validation dashboard for MGPUSim."
    )
    parser.add_argument(
        "--csv",
        default="benchmark-comparison/comparison_ci.csv",
        help="Path to comparison_ci.csv (default: benchmark-comparison/comparison_ci.csv)",
    )
    parser.add_argument(
        "--min-points",
        type=int,
        default=3,
        help="Minimum matched data points per kernel for Tier 1 (default: 3)",
    )
    parser.add_argument(
        "--min-cv",
        type=float,
        default=0.02,
        help="Minimum coefficient of variation of real_ms to include kernel in Tier 1 (default: 0.02)",
    )
    args = parser.parse_args()

    kernels = _read_comparison_csv(args.csv)
    if not kernels:
        print("ERROR: No matched (real_ms, sim_ms) rows found.", file=sys.stderr)
        sys.exit(1)

    total_kernels = len(kernels)
    eligible = sum(1 for pts in kernels.values() if len(pts) >= args.min_points)
    print(f"Loaded {sum(len(v) for v in kernels.values())} matched data points "
          f"across {total_kernels} kernels ({eligible} with \u2265{args.min_points} points).\n")

    results = evaluate_tiers(
        kernels,
        min_points=args.min_points,
        min_cv=args.min_cv,
    )
    print_dashboard(results)


if __name__ == "__main__":
    main()
