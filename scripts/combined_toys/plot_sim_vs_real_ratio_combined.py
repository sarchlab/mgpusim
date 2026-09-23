#!/usr/bin/env python3
"""
Plot MGPUSim's (AMD GPU simulator) and BeyondV's (NVIDIA simulator)
simulated-vs-real kernel-time deviation side by side, one benchmark group
per x-axis tick, two bars per group.

The AMD-simulator series is read from toy_recording.csv (H100 rows only --
see plot_sim_vs_real_ratio.py in scripts/h100_baseline/, this is that same
data with A100 dropped). The NVIDIA-simulator series is a fixed/hardcoded
dataset (see NVIDIA_SIM_DATA below -- copied from
plot_sim_vs_real_ratio_fixed.py in scripts/mgpusim_nvidia_on_toy/).

A single figure shows, for every benchmark (x-axis groups, sorted
alphabetically) plus a trailing "mean" group, two bars: the AMD GPU
simulator (gray) and the BeyondV-generated NVIDIA simulator (blue). Each
bar is the relative deviation
    |sim - hardware| / hardware
i.e. |simulated_cycle / hardware_cycle - 1|. This is unsigned: it only
says how far off the simulator is, not which direction, so bigger is
worse and 0 is a perfect match.

Usage:
  python3 plot_sim_vs_real_ratio_combined.py [--csv toy_recording.csv]
                                              [--out sim_vs_real_ratio_combined.png]
"""
import argparse
import csv
from pathlib import Path

import matplotlib.pyplot as plt
from matplotlib.ticker import FormatStrFormatter, MultipleLocator

# ---- Fixed data: BeyondV-generated NVIDIA simulator, one row per benchmark
# profile_cycle = HW (hardware-profiled cycle count)
# predict_cycle = Sim (simulator-predicted cycle count)
NVIDIA_SIM_DATA = [
    {"benchmark": "fastwalshtransform",   "profile_cycle": 19343, "predict_cycle": 20066},
    {"benchmark": "matrixmultiplication", "profile_cycle": 35071, "predict_cycle": 42211},
    {"benchmark": "floydwarshall",        "profile_cycle": 9884,  "predict_cycle": 7779},
    {"benchmark": "simpleconvolution",    "profile_cycle": 11271, "predict_cycle": 13931},
    {"benchmark": "bitonicsort",          "profile_cycle": 77338, "predict_cycle": 78158},
    {"benchmark": "matrixtranspose",      "profile_cycle": 12979, "predict_cycle": 10682},
]

# The AMD-simulator series is read from this CSV's H100 rows only.
AMD_SIM_CSV_GPU_MODEL = "H100"

# ---- Figure styling knobs: tweak freely -----------------------------------
# Wide/short ("fat"), matching draw_amd_result.py / plot_nvidia.py's FIGSIZE.
FIGSIZE = (18, 7)
AXIS_LABEL_FONTSIZE = 30#20
TICK_LABEL_FONTSIZE = 30#20
LEGEND_FONTSIZE = 25#18
VALUE_LABEL_FONTSIZE = 30#10

# Fixed y-axis ceiling. Bars whose true value exceeds this are drawn capped
# at Y_MAX, with their real value printed as text above the cap (same idea
# as the "2.5x"/"2.7x" overflow labels in the reference figure). Bars that
# don't exceed it get no value label at all, matching the reference figure.
Y_MAX = 0.50

# Fixed tick spacing so labels always land exactly on 0.0, 0.1, ..., Y_MAX
# (the last one sitting exactly at the top of the frame) regardless of data.
Y_TICK_STEP = 0.1
Y_TICK_FORMAT = "%.1f"

# Name of the trailing summary group (mean across all benchmarks).
MEAN_GROUP_LABEL = "mean"

# Gap (in x-axis units, on top of the normal 1.0 spacing between adjacent
# groups) inserted before the mean group. The mean group is set apart by
# its shaded background band (MEAN_BAND_COLOR) rather than by whitespace,
# so this only needs to be enough that the band doesn't fuse with the last
# benchmark group -- matching draw_amd_result.py / plot_nvidia.py.
GAP_BEFORE_MEAN = 0.0

AMD_SIM_LABEL = "AMD GPU simulator (Before BeyondV)"
AMD_SIM_COLOR = "#7F7F7F"  # gray

NVIDIA_SIM_LABEL = "BeyondV NVIDIA simulator"
NVIDIA_SIM_COLOR = "#1F77B4"  # blue

BAR_EDGE_COLOR = "black"
BAR_EDGE_WIDTH = 1.2
BORDER_LINEWIDTH = 1.8
GRID_COLOR = "#CCCCCC"
GRID_LINEWIDTH = BORDER_LINEWIDTH * 0.6

MEAN_BAND_COLOR = "#D9D9D9"  # light gray band behind the mean group
MEAN_BAND_HALF_WIDTH = 0.45  # half-width (in x-axis units) of the shaded band

VLINE_COLOR = "#DDDDDD"
VLINE_WIDTH = 0.9

BAR_WIDTH = 0.2

# For bars clipped at Y_MAX: the bar's own color pokes a little past the top
# frame line (drawn with clip_on=False, no bottom border, so it reads as a
# continuation of the bar breaking through the box) and then a white gap
# (bordered top/bottom, like a torn edge) separates that poke from the value
# label -- together reading as "this bar escapes past the frame".
CLIP_POKE_FRACTION = 0.05
CLIP_POKE_HEIGHT = CLIP_POKE_FRACTION * Y_MAX
CLIP_GAP_FRACTION = 0.035
CLIP_GAP_HEIGHT = CLIP_GAP_FRACTION * Y_MAX

FONT_FAMILY = "serif"
FONT_SERIF_STACK = ["Nimbus Roman", "Liberation Serif", "DejaVu Serif"]
# -----------------------------------------------------------------------------


def abs_relative_deviation(sim_cycle: float, hw_cycle: float) -> float:
    """|sim - hardware| / hardware, i.e. |sim_cycle/hw_cycle - 1|. Unsigned:
    only says how far off the simulator is, not whether it's too fast or
    too slow."""
    return abs(sim_cycle / hw_cycle - 1.0)


def load_amd_sim_deviations(csv_path: str, gpu_model: str) -> dict:
    """Returns {benchmark: deviation} from toy_recording.csv's
    simulated_real_ratio column, keeping the last row seen per benchmark
    for `gpu_model`."""
    with open(csv_path, newline="") as f:
        rows = list(csv.DictReader(f))
    result = {}
    for row in rows:
        if row["gpu_model"] != gpu_model:
            continue
        ratio = float(row["simulated_real_ratio"])
        result[row["benchmark"]] = abs(ratio - 1.0)
    return result


def load_nvidia_sim_deviations() -> dict:
    return {
        r["benchmark"]: abs_relative_deviation(r["predict_cycle"], r["profile_cycle"])
        for r in NVIDIA_SIM_DATA
    }


def plot(amd_deviations: dict, nvidia_deviations: dict, out_path: str) -> None:
    plt.rcParams["font.family"] = FONT_FAMILY
    plt.rcParams["font.serif"] = FONT_SERIF_STACK

    benchmarks = sorted(set(amd_deviations) | set(nvidia_deviations))

    amd_vals = [amd_deviations[b] for b in benchmarks]
    nvidia_vals = [nvidia_deviations[b] for b in benchmarks]
    amd_mean = sum(amd_vals) / len(amd_vals)
    nvidia_mean = sum(nvidia_vals) / len(nvidia_vals)

    labels = benchmarks + [MEAN_GROUP_LABEL]
    amd_vals = amd_vals + [amd_mean]
    nvidia_vals = nvidia_vals + [nvidia_mean]

    benchmark_x = list(range(len(benchmarks)))
    mean_x = len(benchmarks) - 1 + 1 + GAP_BEFORE_MEAN
    group_x = benchmark_x + [mean_x]

    # Bars are drawn capped at Y_MAX; the true value is still printed above
    # the cap so nothing is visually lost, it's just clipped on the axis.
    amd_draw_vals = [min(v, Y_MAX) for v in amd_vals]
    nvidia_draw_vals = [min(v, Y_MAX) for v in nvidia_vals]

    label_offset = 0.02 * Y_MAX

    fig, ax = plt.subplots(figsize=FIGSIZE)
    ax.set_axisbelow(True)

    # Shaded band behind the mean group, drawn first so bars sit on top.
    ax.axvspan(mean_x - MEAN_BAND_HALF_WIDTH, mean_x + MEAN_BAND_HALF_WIDTH,
               color=MEAN_BAND_COLOR, zorder=0)

    ax.yaxis.grid(True, linestyle="--", color=GRID_COLOR, linewidth=GRID_LINEWIDTH, zorder=1)

    # Light vertical separators at the midpoint between every pair of
    # adjacent groups (including the gap into the mean group).
    for xi, xj in zip(group_x[:-1], group_x[1:]):
        ax.axvline((xi + xj) / 2, color=VLINE_COLOR, linewidth=VLINE_WIDTH, zorder=1)

    bar_width = BAR_WIDTH
    amd_x = [xi - bar_width / 2 for xi in group_x]
    nvidia_x = [xi + bar_width / 2 for xi in group_x]

    amd_bars = ax.bar(amd_x, amd_draw_vals, width=bar_width, label=AMD_SIM_LABEL, color=AMD_SIM_COLOR,
                       edgecolor=BAR_EDGE_COLOR, linewidth=BAR_EDGE_WIDTH, zorder=3)
    nvidia_bars = ax.bar(nvidia_x, nvidia_draw_vals, width=bar_width, label=NVIDIA_SIM_LABEL, color=NVIDIA_SIM_COLOR,
                          edgecolor=BAR_EDGE_COLOR, linewidth=BAR_EDGE_WIDTH, zorder=3)

    # Bars that got clipped: color pokes past the frame top (no bottom edge,
    # so it visually fuses with the bar below it), then a white gap (torn
    # edge, top+bottom border) breaks it from the value label above.
    for bars, vals, color in [(amd_bars, amd_vals, AMD_SIM_COLOR), (nvidia_bars, nvidia_vals, NVIDIA_SIM_COLOR)]:
        for bar, val in zip(bars, vals):
            if val <= Y_MAX:
                continue
            cx, cw = bar.get_x() + bar.get_width() / 2, bar.get_width()
            # Poke: same fill color, no edge of its own (so it visually
            # continues straight out of the bar below), just re-stroke its
            # left/right sides in black to match the bar's own side edges.
            ax.bar(cx, CLIP_POKE_HEIGHT, bottom=Y_MAX, width=cw, color=color,
                   edgecolor="none", zorder=4, clip_on=False)
            ax.plot([cx - cw / 2, cx - cw / 2], [Y_MAX, Y_MAX + CLIP_POKE_HEIGHT],
                    color=BAR_EDGE_COLOR, linewidth=BAR_EDGE_WIDTH, zorder=4, clip_on=False)
            ax.plot([cx + cw / 2, cx + cw / 2], [Y_MAX, Y_MAX + CLIP_POKE_HEIGHT],
                    color=BAR_EDGE_COLOR, linewidth=BAR_EDGE_WIDTH, zorder=4, clip_on=False)
            gap_bottom = Y_MAX + CLIP_POKE_HEIGHT
            ax.bar(cx, CLIP_GAP_HEIGHT, bottom=gap_bottom, width=cw, color="white",
                   edgecolor=BAR_EDGE_COLOR, linewidth=BAR_EDGE_WIDTH, zorder=5, clip_on=False)
            ax.text(cx, gap_bottom + CLIP_GAP_HEIGHT + label_offset, f"{val:.3f}",
                     ha="center", va="bottom", fontsize=VALUE_LABEL_FONTSIZE,
                     clip_on=False, zorder=6)

    ax.set_ylim(0.0, Y_MAX)

    ax.set_xticks(group_x)
    ax.set_xticklabels(labels, rotation=15, ha="right", fontsize=TICK_LABEL_FONTSIZE)
    ax.yaxis.set_major_locator(MultipleLocator(Y_TICK_STEP))
    ax.yaxis.set_major_formatter(FormatStrFormatter(Y_TICK_FORMAT))
    ax.tick_params(axis="y", labelsize=TICK_LABEL_FONTSIZE)
    ax.set_ylabel("ARE  |Sim − HW| / HW", fontsize=AXIS_LABEL_FONTSIZE)

    for spine in ax.spines.values():
        spine.set_linewidth(BORDER_LINEWIDTH)
    ax.tick_params(width=BORDER_LINEWIDTH * 0.7)

    ax.legend(fontsize=LEGEND_FONTSIZE, loc="lower left", bbox_to_anchor=(0.0, 1.36),
              ncol=2, frameon=True, edgecolor="black")

    ax.set_xlim(group_x[0] - 0.8, mean_x + 0.8)

    fig.tight_layout()
    fig.savefig(out_path, dpi=500, bbox_inches="tight")
    print(f"wrote {out_path}")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--csv", default=str(Path(__file__).resolve().parent / "toy_recording.csv"))
    ap.add_argument("--out", default=str(Path(__file__).resolve().parent / "motivation_sim_vs_real_ratio_combined.png"))
    args = ap.parse_args()

    amd_deviations = load_amd_sim_deviations(args.csv, AMD_SIM_CSV_GPU_MODEL)
    nvidia_deviations = load_nvidia_sim_deviations()
    plot(amd_deviations, nvidia_deviations, args.out)


if __name__ == "__main__":
    main()