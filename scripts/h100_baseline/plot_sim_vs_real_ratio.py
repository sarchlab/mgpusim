#!/usr/bin/env python3
"""
Plot Simulator/Hardware kernel-time ratio per benchmark, one subplot per GPU
model, reading from toy_recording.csv (see compare.py).

Each subplot shows, for every benchmark (x-axis groups, sorted
alphabetically), two bars: "Hardware" (always normalized to 1.0) and
"Simulator" (mgpusim's simulated_real_ratio for that benchmark/GPU). This
visualizes how far mgpusim's prediction sits from real hardware, side by
side across GPU models (e.g. H100 vs. A100).

If toy_recording.csv has multiple rows for the same (gpu_model, benchmark)
pair, the most recent row (last in the file) is used.

Usage:
  python3 plot_sim_vs_real_ratio.py [--csv toy_recording.csv]
                                     [--gpu-models H100 A100]
                                     [--out sim_vs_real_ratio.png]
"""
import argparse
import csv
from pathlib import Path

import matplotlib.pyplot as plt


def load_rows(csv_path: str) -> list:
    with open(csv_path, newline="") as f:
        return list(csv.DictReader(f))


def latest_ratio_by_benchmark(rows: list, gpu_model: str) -> dict:
    """Returns {benchmark: simulated_real_ratio}, keeping the last row seen
    per benchmark for the given gpu_model."""
    result = {}
    for row in rows:
        if row["gpu_model"] != gpu_model:
            continue
        result[row["benchmark"]] = float(row["simulated_real_ratio"])
    return result


def plot(rows: list, gpu_models: list, out_path: str) -> None:
    fig, axes = plt.subplots(1, len(gpu_models), figsize=(7 * len(gpu_models), 5), sharey=True)
    if len(gpu_models) == 1:
        axes = [axes]

    bar_width = 0.35

    for ax, gpu_model in zip(axes, gpu_models):
        ratios = latest_ratio_by_benchmark(rows, gpu_model)
        benchmarks = sorted(ratios.keys())

        x = range(len(benchmarks))
        hardware_vals = [1.0] * len(benchmarks)
        sim_vals = [ratios[b] for b in benchmarks]

        hw_x = [i - bar_width / 2 for i in x]
        sim_x = [i + bar_width / 2 for i in x]

        ax.bar(hw_x, hardware_vals, width=bar_width, label="Hardware")
        bars = ax.bar(sim_x, sim_vals, width=bar_width, label="Simulator")

        for bar, val in zip(bars, sim_vals):
            ax.text(bar.get_x() + bar.get_width() / 2, bar.get_height() + 0.02,
                     f"{val:.2f}", ha="center", va="bottom", fontsize=9)

        ax.axhline(1.0, color="black", linewidth=0.8, linestyle="--")
        ax.set_xticks(list(x))
        ax.set_xticklabels(benchmarks, rotation=30, ha="right")
        ax.set_title(f"NVIDIA {gpu_model} GPU vs. MGPUSIM Simulator")
        ax.set_ylabel("Simulator/Hardware Ratio")
        ax.legend()

    fig.tight_layout()
    fig.savefig(out_path, dpi=150)
    print(f"wrote {out_path}")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--csv", default=str(Path(__file__).resolve().parent / "toy_recording.csv"))
    ap.add_argument("--gpu-models", nargs="+", default=["H100", "A100"])
    ap.add_argument("--out", default=str(Path(__file__).resolve().parent / "sim_vs_real_ratio.png"))
    args = ap.parse_args()

    rows = load_rows(args.csv)
    plot(rows, args.gpu_models, args.out)


if __name__ == "__main__":
    main()
