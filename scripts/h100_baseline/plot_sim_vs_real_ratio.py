#!/usr/bin/env python3
"""
Plot MGPUSim's simulated-vs-real kernel-time deviation per benchmark, one
bar per GPU model, reading from toy_recording.csv (see compare.py).

A single figure shows, for every benchmark (x-axis groups, sorted
alphabetically) plus a trailing "mean" group, one bar per GPU model
(e.g. H100, A100). Each bar is the relative deviation
    |sim - hardware| / hardware
computed from mgpusim's simulated_real_ratio (sim/hardware) for that
benchmark/GPU -- equivalently |ratio - 1|, since dividing by "hardware"
here is the same as normalizing hardware to 1.0 and sim to the ratio. This
is unsigned: it only says how far off the simulator is, not which
direction, so bigger is worse and 0 is a perfect match.

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

import matplotlib.colors as mcolors
import matplotlib.pyplot as plt

# ---- Figure styling knobs: tweak freely -----------------------------------
FIGSIZE = (9, 7)
TITLE_FONTSIZE = 20
AXIS_LABEL_FONTSIZE = 20 # 14
TICK_LABEL_FONTSIZE = 20 # 12
LEGEND_FONTSIZE = 20 # 12
VALUE_LABEL_FONTSIZE = 10 # 9

# Extra padding (in data units) added above the tallest bar so value labels
# never touch the top of the plot.
Y_PADDING = 0.2

# Name of the trailing summary group (mean across all benchmarks).
MEAN_GROUP_LABEL = "mean"

# Extra horizontal gap (in x-axis units, on top of the normal 1.0 spacing
# between adjacent groups) inserted before the mean group, so it reads
# visually separate from the per-benchmark groups.
AVERAGE_GROUP_EXTRA_GAP = 0.5

# NVIDIA's brand green, and a lighter tint of it. Bars are colored by each
# GPU model's position in --gpu-models: the first model (assumed to be the
# strongest -- H100 by default) gets the full brand green, later models get
# progressively lighter shades. With the default ["H100", "A100"] that's
# just these two colors, H100 dark / A100 light.
NVIDIA_GREEN = "#76B900"
NVIDIA_GREEN_LIGHT = "#C7E59B"
# -----------------------------------------------------------------------------


def gpu_color_shades(gpu_models: list) -> dict:
    """{gpu_model: hex color}, interpolating from NVIDIA_GREEN (first model)
    to NVIDIA_GREEN_LIGHT (last model)."""
    if len(gpu_models) == 1:
        return {gpu_models[0]: NVIDIA_GREEN}
    dark = mcolors.to_rgb(NVIDIA_GREEN)
    light = mcolors.to_rgb(NVIDIA_GREEN_LIGHT)
    n = len(gpu_models)
    shades = {}
    for i, model in enumerate(gpu_models):
        t = i / (n - 1)
        rgb = tuple(d + (l - d) * t for d, l in zip(dark, light))
        shades[model] = mcolors.to_hex(rgb)
    return shades


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


def abs_relative_deviation(ratio: float) -> float:
    """|sim - hardware| / hardware, given hardware normalized to 1.0 and
    sim == ratio. Equivalent to |ratio - 1|. Unsigned: only says how far
    off the simulator is, not whether it's too fast or too slow."""
    return abs(ratio - 1.0)


def plot(rows: list, gpu_models: list, out_path: str) -> None:
    ratios_by_model = {gpu_model: latest_ratio_by_benchmark(rows, gpu_model) for gpu_model in gpu_models}
    benchmarks = sorted({b for ratios in ratios_by_model.values() for b in ratios})

    deviations_by_model = {
        gpu_model: [abs_relative_deviation(ratios_by_model[gpu_model][b]) for b in benchmarks]
        for gpu_model in gpu_models
    }
    mean_by_model = {gpu_model: sum(devs) / len(devs) for gpu_model, devs in deviations_by_model.items()}

    labels = benchmarks + [MEAN_GROUP_LABEL]
    benchmark_x = list(range(len(benchmarks)))
    mean_x = len(benchmarks) - 1 + 1 + AVERAGE_GROUP_EXTRA_GAP
    group_x = benchmark_x + [mean_x]

    colors = gpu_color_shades(gpu_models)

    all_vals = [
        v
        for gpu_model in gpu_models
        for v in deviations_by_model[gpu_model] + [mean_by_model[gpu_model]]
    ]
    label_offset = 0.02 * max(all_vals)

    fig, ax = plt.subplots(figsize=FIGSIZE)

    n_models = len(gpu_models)
    bar_width = 0.8 / n_models

    for i, gpu_model in enumerate(gpu_models):
        vals = deviations_by_model[gpu_model] + [mean_by_model[gpu_model]]
        offset = (i - (n_models - 1) / 2) * bar_width
        bar_x = [xi + offset for xi in group_x]
        bars = ax.bar(bar_x, vals, width=bar_width, label=gpu_model, color=colors[gpu_model])

        for bar, val in zip(bars, vals):
            ax.text(bar.get_x() + bar.get_width() / 2, bar.get_height() + label_offset,
                     f"{val:.3f}", ha="center", va="bottom", fontsize=VALUE_LABEL_FONTSIZE)

    ax.set_ylim(0.0, max(all_vals) + Y_PADDING)

    ax.set_xticks(group_x)
    ax.set_xticklabels(labels, rotation=30, ha="right", fontsize=TICK_LABEL_FONTSIZE)
    ax.tick_params(axis="y", labelsize=TICK_LABEL_FONTSIZE)
    ax.set_title("Use an AMD GPU Simulator to Simulate\nWorkloads on NVIDIA GPUs", fontsize=TITLE_FONTSIZE)
    ax.set_ylabel("Relative Deviation |Sim \u2212 HW| / HW", fontsize=AXIS_LABEL_FONTSIZE)
    ax.legend(fontsize=LEGEND_FONTSIZE, loc="upper right")

    fig.tight_layout()
    fig.savefig(out_path, dpi=300)
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