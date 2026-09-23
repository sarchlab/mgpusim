#!/usr/bin/env python3
"""
Bar chart of per-benchmark relative error between the (aligned) simulator
output and real MI300X hardware, one bar per {suite}-{benchmark}, plus a
trailing "Mean" bar.

Inputs are the two CSVs produced by extract_sim_amd.py:
  --sim  MI300X_simulation.csv     (has sim_time_ms_aligned, is_anchor, ...)
  --gt   MI300X_ground_truth_2.csv (has time_ms_avg)

Method (confirmed with the user):
  - Match sim/real points on (suite, benchmark, non_scaling_slug, scaling_param_value).
  - Anchor points (is_anchor=True -- the point each figure's offset was
    solved to match exactly) are EXCLUDED, matching how the dashboard's own
    MAPE/sMAPE are computed.
  - Per-point relative error = |sim - real| / real.
  - Each benchmark's bar = mean of its points' relative errors (division
    done per-point first, then averaged -- not mean(sim)/mean(real)).
  - The trailing "Mean" bar = unweighted mean of the per-benchmark bar
    values (every benchmark counts equally, regardless of how many points
    it has).
  - empty_kernel is excluded by default (its real-hardware ground truth
    looks unreliable -- see DEFAULT_EXCLUDE_BENCHMARKS below). Any
    suite-benchmark with no matching points between the sim/gt tables, or
    with zero non-anchor points, is skipped and reported via a WARNING
    rather than silently included or erroring. Use --exclude to drop more.
  - Suites with fewer than MIN_BENCHMARKS_PER_SUITE surviving benchmarks
    (default 3) are dropped entirely, e.g. altis/tango currently have only
    1 surviving benchmark each.
  - x-axis labels are "{suite}-{benchmark}", but when benchmark repeats its
    own suite name (e.g. suite=polybench, benchmark=polybench_2dconv) that
    repeat is stripped: "polybench-2dconv" instead of
    "polybench-polybench_2dconv". Labels are sorted alphabetically by this
    final (deduped) string. The trailing bar is labeled "mean" (lowercase,
    to match).

Usage:
    python3 draw_amd_result.py --sim MI300X_simulation.csv --gt MI300X_ground_truth_2.csv \\
        --out mi300x_relative_error.png
"""

import argparse
import csv
from collections import defaultdict

import matplotlib.pyplot as plt
from matplotlib.ticker import MultipleLocator, FormatStrFormatter

# ---------------------------------------------------------------------------
# Tweakable presentation settings -- kept up top so they're easy to find.
# Styled to match a reference paper's theme: green bars, no in-plot title,
# no value labels on bars, a shaded background band behind the summary bar,
# a full box frame, fixed 0.25-interval y-ticks, and a serif font.
# ---------------------------------------------------------------------------
# Try Times New Roman first (what the reference figure appears to use),
# falling back through progressively more available serif fonts so this
# doesn't silently render in the wrong typeface if Times isn't installed.
FONT_FAMILY = "serif"
FONT_SERIF_STACK = ["Nimbus Roman", "Liberation Serif", "DejaVu Serif"] # "Times New Roman", 

FONT_SIZE_AXIS_LABEL = 16#14
FONT_SIZE_TICK = 16#13

BAR_COLOR = "#F5B183" # "#00B050"        # rgb(0,176,80), sampled from the reference figure
MEAN_BAR_COLOR = "#F5B183"  # same green; the shaded band behind it is what sets it apart
BAR_EDGE_COLOR = "black"
BAR_EDGE_WIDTH = 1.2         # reference bars have a visible outline

SHOW_TITLE = False           # reference chart has no in-plot title
SHOW_VALUE_LABELS = False    # reference chart has no numbers printed above bars

MEAN_BAND_COLOR = "#D9D9D9"  # light gray band behind the summary bar
MEAN_BAND_HALF_WIDTH = 0.45  # half-width (in x-axis units) of the shaded band; shrunk to match the tighter GAP_BEFORE_MEAN

Y_TICK_STEP = 0.25           # fixed tick spacing, matching the reference's 0.00/0.25/0.50/0.75/1.00
Y_TICK_FORMAT = "%.2f"

BORDER_LINEWIDTH = 1.8       # thickness of the box frame and gridlines, to match the reference's bolder look

# Benchmarks excluded from the chart entirely (known-bad ground truth, etc).
# empty_kernel: results.db's real-hardware time for it is ~47-50ms flat
# regardless of num_blocks (looks like kernel-launch/JIT cold-start being
# captured instead of steady-state execution time), which disagrees with
# the checked-in mi300x_ground_truth.csv (sub-ms, scales with num_blocks)
# used to build data.json -- see the cross-validation from the prior step.
# Its relative-error numbers aren't trustworthy either way, so it's dropped.
DEFAULT_EXCLUDE_BENCHMARKS = {"empty_kernel"}

# Suites with fewer than this many surviving benchmarks (after the above
# exclusions) are dropped entirely -- too few bars to say anything about
# "this suite" as a group.
MIN_BENCHMARKS_PER_SUITE = 3

FIGSIZE = (18, 4)
DPI = 300
BAR_WIDTH = 0.35
# Now that the mean bar is set apart by its shaded background band rather
# than by whitespace, it doesn't need much extra gap -- just enough that
# the band doesn't visually fuse with the last benchmark bar.
GAP_BEFORE_MEAN = 0.0
LABEL_ROTATION = 25

TITLE = "Simulator Performance on AMD MI300X GPU"
YLABEL = "|Sim − HW| / HW"

# Light vertical separators between adjacent bars (including the gap into
# the mean bar), matching the reference figure's per-bar gridlines.
VLINE_COLOR = "#DDDDDD"
VLINE_WIDTH = 0.9


def load_sim(path):
    """suite,benchmark,non_scaling_slug,scaling_param_value -> aligned sim time_ms (non-anchor rows only).
    Also returns the full set of (suite, benchmark) pairs seen in the file
    (anchor rows included), so callers can detect benchmarks that end up
    with zero non-anchor points."""
    points = {}
    all_benchmarks = set()
    with open(path, newline="") as f:
        for row in csv.DictReader(f):
            all_benchmarks.add((row["suite"], row["benchmark"]))
            if row["is_anchor"] == "True":
                continue
            key = (row["suite"], row["benchmark"], row["non_scaling_slug"], float(row["scaling_param_value"]))
            points[key] = float(row["sim_time_ms_aligned"])
    return points, all_benchmarks


def load_gt(path):
    """suite,benchmark,non_scaling_slug,scaling_param_value -> real time_ms_avg."""
    points = {}
    with open(path, newline="") as f:
        for row in csv.DictReader(f):
            key = (row["suite"], row["benchmark"], row["non_scaling_slug"], float(row["scaling_param_value"]))
            points[key] = float(row["time_ms_avg"])
    return points


def format_label(suite, benchmark):
    """'polybench', 'polybench_2dconv' -> 'polybench-2dconv' (drop the
    benchmark's redundant repeat of its own suite name). Benchmarks that
    don't repeat the suite name (e.g. 'microbench', 'cache_latency') are
    left as-is: 'microbench-cache_latency'."""
    prefix = suite + "_"
    short = benchmark[len(prefix):] if benchmark.startswith(prefix) else benchmark
    return f"{suite}-{short}"


def plot(bench_labels, bench_values, mean_value, out_path):
    """Split out from main() so the styling can be smoke-tested with mock
    data independent of CSV loading."""
    if FONT_FAMILY:
        plt.rcParams["font.family"] = FONT_FAMILY
        plt.rcParams["font.serif"] = FONT_SERIF_STACK

    n = len(bench_labels)
    x = list(range(n))
    mean_x = n - 1 + GAP_BEFORE_MEAN + 1  # one bar-width slot after the last benchmark, plus the gap

    fig, ax = plt.subplots(figsize=FIGSIZE)

    # Shaded band behind the summary bar, drawn first so bars sit on top.
    ax.axvspan(mean_x - MEAN_BAND_HALF_WIDTH, mean_x + MEAN_BAND_HALF_WIDTH,
               color=MEAN_BAND_COLOR, zorder=0)

    # Light vertical separators at the midpoint between every pair of
    # adjacent bars (including the shorter gap into the mean bar), sitting
    # above the shaded band but below the bars themselves.
    all_x = x + [mean_x]
    for xi, xj in zip(all_x[:-1], all_x[1:]):
        ax.axvline((xi + xj) / 2, color=VLINE_COLOR, linewidth=VLINE_WIDTH, zorder=1)

    ax.bar(x, bench_values, width=BAR_WIDTH, color=BAR_COLOR,
           edgecolor=BAR_EDGE_COLOR, linewidth=BAR_EDGE_WIDTH, zorder=3)
    ax.bar([mean_x], [mean_value], width=BAR_WIDTH, color=MEAN_BAR_COLOR,
           edgecolor=BAR_EDGE_COLOR, linewidth=BAR_EDGE_WIDTH, zorder=3)

    if SHOW_VALUE_LABELS:
        for xi, v in zip(x, bench_values):
            ax.text(xi, v, f"{v:.3f}", ha="center", va="bottom", fontsize=FONT_SIZE_TICK, zorder=4)
        ax.text(mean_x, mean_value, f"{mean_value:.3f}", ha="center", va="bottom", fontsize=FONT_SIZE_TICK, zorder=4)

    ax.set_xticks(x + [mean_x])
    ax.set_xticklabels(bench_labels + ["mean"], rotation=LABEL_ROTATION, ha="right", fontsize=FONT_SIZE_TICK)
    ax.tick_params(axis="y", labelsize=FONT_SIZE_TICK)

    if SHOW_TITLE:
        ax.set_title(TITLE, fontsize=FONT_SIZE_AXIS_LABEL + 4, pad=16)
    ax.set_ylabel(YLABEL, fontsize=FONT_SIZE_AXIS_LABEL)

    ax.yaxis.set_major_locator(MultipleLocator(Y_TICK_STEP))
    ax.yaxis.set_major_formatter(FormatStrFormatter(Y_TICK_FORMAT))
    ax.yaxis.grid(True, linestyle="--", color="#BBBBBB", linewidth=BORDER_LINEWIDTH * 0.6, alpha=0.8, zorder=1)
    ax.set_axisbelow(True)

    # Full box frame (reference style), instead of hiding top/right spines.
    for spine in ax.spines.values():
        spine.set_visible(True)
        spine.set_color("black")
        spine.set_linewidth(BORDER_LINEWIDTH)

    ax.tick_params(width=BORDER_LINEWIDTH * 0.7)

    ax.set_xlim(-0.8, mean_x + 0.8)

    fig.tight_layout()
    fig.savefig(out_path, dpi=DPI)
    print(f"Wrote {out_path}  ({n} benchmarks + 1 mean bar)")


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--sim", required=True, help="Path to MI300X_simulation.csv")
    parser.add_argument("--gt", required=True, help="Path to MI300X_ground_truth_2.csv")
    parser.add_argument("--out", default="mi300x_relative_error.png", help="Output image path")
    parser.add_argument(
        "--exclude",
        default="",
        help=f"Comma-separated benchmark names to exclude in addition to the defaults ({sorted(DEFAULT_EXCLUDE_BENCHMARKS)})",
    )
    args = parser.parse_args()

    exclude = set(DEFAULT_EXCLUDE_BENCHMARKS) | {b.strip() for b in args.exclude.split(",") if b.strip()}

    sim_points, sim_benchmarks = load_sim(args.sim)
    gt_points = load_gt(args.gt)

    if exclude:
        sim_points = {k: v for k, v in sim_points.items() if k[1] not in exclude}
        sim_benchmarks = {(s, b) for (s, b) in sim_benchmarks if b not in exclude}
        print(f"Excluding benchmark(s): {sorted(exclude)}")

    errors_by_bench = defaultdict(list)
    unmatched = 0
    for key, sim_val in sim_points.items():
        real_val = gt_points.get(key)
        if real_val is None:
            # No matching point in the ground-truth table for this
            # suite/benchmark/combo/scaling_value -- skip it rather than
            # erroring; the benchmark-level "dropped" check below reports it
            # if this leaves the whole benchmark with nothing to plot.
            unmatched += 1
            continue
        suite, benchmark, _slug, _sp_val = key
        errors_by_bench[(suite, benchmark)].append(abs(sim_val - real_val) / real_val)

    if unmatched:
        print(f"WARNING: {unmatched} non-anchor sim points had no matching ground-truth point; skipped.")

    dropped = sorted(format_label(s, b) for s, b in sim_benchmarks if (s, b) not in errors_by_bench)
    if dropped:
        print(
            f"WARNING: {len(dropped)} benchmark(s) have zero non-anchor points "
            f"(every sim point for them was the anchor) and are OMITTED from the chart entirely: {dropped}"
        )

    # Drop whole suites that have too few surviving benchmarks to say
    # anything meaningful as a group (e.g. altis/tango with only 1 each
    # after the exclusions above).
    suite_counts = defaultdict(int)
    for (suite, _benchmark) in errors_by_bench:
        suite_counts[suite] += 1
    small_suites = {suite for suite, cnt in suite_counts.items() if cnt < MIN_BENCHMARKS_PER_SUITE}
    if small_suites:
        removed = sorted(format_label(s, b) for (s, b) in errors_by_bench if s in small_suites)
        print(
            f"WARNING: suite(s) {sorted(small_suites)} have fewer than "
            f"{MIN_BENCHMARKS_PER_SUITE} surviving benchmarks and are OMITTED entirely: {removed}"
        )
        errors_by_bench = {k: v for k, v in errors_by_bench.items() if k[0] not in small_suites}

    # One value per benchmark: mean of that benchmark's per-point relative errors.
    bench_mean = {
        format_label(suite, benchmark): sum(errs) / len(errs)
        for (suite, benchmark), errs in errors_by_bench.items()
    }
    bench_labels = sorted(bench_mean.keys())
    bench_values = [bench_mean[label] for label in bench_labels]

    # Overall Mean bar: unweighted mean of the per-benchmark bar values.
    mean_value = sum(bench_values) / len(bench_values)

    plot(bench_labels, bench_values, mean_value, args.out)


if __name__ == "__main__":
    main()