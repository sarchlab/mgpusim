#!/usr/bin/env python3
"""Generate per-kernel scaling figures (HW vs Sim) from comparison_ci.csv.

Reads benchmark-comparison/comparison_ci.csv and produces one PNG per kernel
(where ≥2 rows have both real_ms and sim_ms populated) into docs/figures/.

Usage:
    python3 scripts/generate_scaling_figures.py
"""

import os
import re
import sys

import matplotlib
matplotlib.use("Agg")  # non-interactive backend
import matplotlib.pyplot as plt
import pandas as pd

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
CSV_PATH = os.path.join(REPO_ROOT, "benchmark-comparison", "comparison_ci.csv")
FIGURES_DIR = os.path.join(REPO_ROOT, "docs", "figures")


def parse_sort_key(s: str):
    """Return a numeric sort key for a problem_size string.

    Handles pure integers ('1024'), dimension strings ('1024x1024'),
    and strings with units ('128KB', 'rows128_...'). Falls back to
    lexicographic sorting for unrecognised formats.
    """
    # Pure integer
    if re.fullmatch(r"\d+", s):
        return (0, int(s), s)

    # NxN or NxNxN  — sort by product of dimensions
    m = re.fullmatch(r"(\d+)(?:x(\d+))?(?:x(\d+))?", s)
    if m:
        dims = [int(d) for d in m.groups() if d is not None]
        product = 1
        for d in dims:
            product *= d
        return (0, product, s)

    # Leading number with suffix (e.g. '128KB', '128MB')
    m = re.match(r"(\d+)", s)
    if m:
        return (0, int(m.group(1)), s)

    # Fallback: lexicographic
    return (1, 0, s)


def main():
    os.makedirs(FIGURES_DIR, exist_ok=True)

    df = pd.read_csv(CSV_PATH)

    # Identify kernels with ≥2 rows where BOTH real_ms and sim_ms are present
    matched = df.dropna(subset=["real_ms", "sim_ms"])
    kernel_counts = matched.groupby("kernel_name").size()
    eligible_kernels = sorted(kernel_counts[kernel_counts >= 2].index)

    if not eligible_kernels:
        print("No kernels with ≥2 matched data points found.")
        sys.exit(0)

    generated = []

    for kernel in eligible_kernels:
        kdf = df[df["kernel_name"] == kernel].copy()

        # Separate HW-only and Sim-only points, plus matched
        hw_rows = kdf.dropna(subset=["real_ms"]).copy()
        sim_rows = kdf.dropna(subset=["sim_ms"]).copy()

        # Sort by problem_size numerically where possible
        hw_rows["_sort"] = hw_rows["problem_size"].apply(parse_sort_key)
        hw_rows = hw_rows.sort_values("_sort").reset_index(drop=True)

        sim_rows["_sort"] = sim_rows["problem_size"].apply(parse_sort_key)
        sim_rows = sim_rows.sort_values("_sort").reset_index(drop=True)

        fig, ax = plt.subplots(figsize=(8, 5))

        # X-axis: use integer index for evenly-spaced ticks
        hw_labels = hw_rows["problem_size"].tolist()
        sim_labels = sim_rows["problem_size"].tolist()

        # Build a unified label list (sorted) for x positioning
        all_labels_set = list(dict.fromkeys(hw_labels + sim_labels))  # preserve order
        # Re-sort using parse_sort_key
        all_labels_set.sort(key=parse_sort_key)
        label_to_x = {label: i for i, label in enumerate(all_labels_set)}

        hw_x = [label_to_x[l] for l in hw_labels]
        sim_x = [label_to_x[l] for l in sim_labels]

        ax.plot(hw_x, hw_rows["real_ms"].tolist(), color="blue", linestyle="-",
                marker="o", label="Hardware (MI300A)", markersize=5)
        ax.plot(sim_x, sim_rows["sim_ms"].tolist(), color="red", linestyle="--",
                marker="s", label="Simulator (MGPUSim)", markersize=5)

        ax.set_xticks(range(len(all_labels_set)))
        ax.set_xticklabels(all_labels_set, rotation=45, ha="right", fontsize=7)
        ax.set_xlabel("Problem Size")
        ax.set_ylabel("Execution Time (ms)")
        ax.set_title(kernel)
        ax.legend()
        ax.grid(True, alpha=0.3)

        fig.tight_layout()
        fname = f"{kernel.lower()}_scaling.png"
        fig.savefig(os.path.join(FIGURES_DIR, fname), dpi=150, bbox_inches="tight")
        plt.close(fig)
        generated.append((kernel, fname))
        print(f"  ✓ {fname}")

    print(f"\nGenerated {len(generated)} figures in {FIGURES_DIR}")
    return generated


if __name__ == "__main__":
    main()
