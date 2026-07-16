#!/usr/bin/env python3
"""
Plot MGPUSim's simulated-vs-real kernel-time deviation per benchmark, one
bar per benchmark, using a fixed/hardcoded dataset (see BENCHMARK_DATA
below) instead of reading a CSV.

Unlike plot_sim_vs_real_ratio.py, there is only one GPU model here, so
each benchmark group has a single bar and no legend is needed.

A single figure shows, for every benchmark (x-axis groups, sorted
alphabetically) plus a trailing "mean" group, one bar. Each bar is the
relative deviation
    |sim - hardware| / hardware
computed as |predict_cycle / profile_cycle - 1|, where profile_cycle is
the hardware-measured cycle count and predict_cycle is MGPUSim's
simulated cycle count. This is unsigned: it only says how far off the
simulator is, not which direction, so bigger is worse and 0 is a
perfect match.

Usage:
  python3 plot_sim_vs_real_ratio_fixed.py [--out sim_vs_real_ratio.png]
"""
import argparse
from pathlib import Path

import matplotlib.pyplot as plt

# ---- Fixed data: one row per benchmark ------------------------------------
# profile_cycle = HW (hardware-profiled cycle count)
# predict_cycle = Sim (MGPUSim-predicted cycle count)
BENCHMARK_DATA = [
    {"benchmark": "fastwalshtransform",   "profile_cycle": 19343, "predict_cycle": 20066},
    {"benchmark": "matrixmultiplication", "profile_cycle": 35071, "predict_cycle": 42211},
    {"benchmark": "floydwarshall",        "profile_cycle": 9884,  "predict_cycle": 7779},
    {"benchmark": "simpleconvolution",    "profile_cycle": 11271, "predict_cycle": 13931},
    {"benchmark": "bitonicsort",          "profile_cycle": 77338, "predict_cycle": 78158},
    {"benchmark": "matrixtranspose",      "profile_cycle": 12979, "predict_cycle": 10682},
]

# ---- Figure styling knobs: tweak freely -----------------------------------
FIGSIZE = (9, 7)
TITLE_FONTSIZE = 20
AXIS_LABEL_FONTSIZE = 20
TICK_LABEL_FONTSIZE = 20
VALUE_LABEL_FONTSIZE = 10

# Extra padding (in data units) added above the tallest bar so value labels
# never touch the top of the plot.
Y_PADDING = 0.2

# Name of the trailing summary group (mean across all benchmarks).
MEAN_GROUP_LABEL = "mean"

# Extra horizontal gap (in x-axis units, on top of the normal 1.0 spacing
# between adjacent groups) inserted before the mean group, so it reads
# visually separate from the per-benchmark groups.
AVERAGE_GROUP_EXTRA_GAP = 0.5

# NVIDIA's brand green. Only one series here, so just one color.
NVIDIA_GREEN = "orange" # "#76B900"
# -----------------------------------------------------------------------------


def abs_relative_deviation(profile_cycle: float, predict_cycle: float) -> float:
    """|sim - hardware| / hardware, i.e. |predict_cycle/profile_cycle - 1|.
    Unsigned: only says how far off the simulator is, not whether it's too
    fast or too slow."""
    ratio = predict_cycle / profile_cycle
    return abs(ratio - 1.0)


def plot(data: list, out_path: str) -> None:
    rows = sorted(data, key=lambda r: r["benchmark"])
    benchmarks = [r["benchmark"] for r in rows]
    deviations = [abs_relative_deviation(r["profile_cycle"], r["predict_cycle"]) for r in rows]
    mean_deviation = sum(deviations) / len(deviations)

    labels = benchmarks + [MEAN_GROUP_LABEL]
    vals = deviations + [mean_deviation]

    benchmark_x = list(range(len(benchmarks)))
    mean_x = len(benchmarks) - 1 + 1 + AVERAGE_GROUP_EXTRA_GAP
    group_x = benchmark_x + [mean_x]

    label_offset = 0.02 * max(vals)

    fig, ax = plt.subplots(figsize=FIGSIZE)

    bars = ax.bar(group_x, vals, width=0.6, color=NVIDIA_GREEN)
    for bar, val in zip(bars, vals):
        ax.text(bar.get_x() + bar.get_width() / 2, bar.get_height() + label_offset,
                 f"{val:.3f}", ha="center", va="bottom", fontsize=VALUE_LABEL_FONTSIZE)

    ax.set_ylim(0.0, max(vals) + Y_PADDING)

    ax.set_xticks(group_x)
    ax.set_xticklabels(labels, rotation=30, ha="right", fontsize=TICK_LABEL_FONTSIZE)
    ax.tick_params(axis="y", labelsize=TICK_LABEL_FONTSIZE)
    ax.set_title("Use MGPUSim-NVIDIA Simulator to Simulate\nWorkloads", fontsize=TITLE_FONTSIZE)
    ax.set_ylabel("Relative Deviation |Sim \u2212 HW| / HW", fontsize=AXIS_LABEL_FONTSIZE)

    fig.tight_layout()
    fig.savefig(out_path, dpi=300)
    print(f"wrote {out_path}")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default=str(Path(__file__).resolve().parent / "sim_vs_real_ratio.png"))
    args = ap.parse_args()

    plot(BENCHMARK_DATA, args.out)


if __name__ == "__main__":
    main()
