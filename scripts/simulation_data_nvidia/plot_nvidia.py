#!/usr/bin/env python3
"""
Bar chart of per-benchmark relative error between simulated and real
NVIDIA H100 kernel cycle counts, one bar per {suite}-{benchmark}, plus a
trailing "mean" bar. Sibling of draw_amd_result.py, adapted for H100_raw.csv
(a single raw table -- no separate aligned-sim/ground-truth files).

Every run produces TWO images: a "_not_aligned" version (raw sim vs real,
no correction) and an "_aligned" version (see ALIGN RULE below).

Input: H100_raw.csv, columns
    profile_type, suite, benchmark, param_json, ground_truth_cycle, simulation_cycle
(produced by extract_raw_data_from_web_json.py)

ALIGN RULE (confirmed with the user):
  Within each (suite, benchmark) group, the point with the SMALLEST
  ground_truth_cycle is the "anchor". offset = anchor's (simulation_cycle -
  ground_truth_cycle). This offset is subtracted from every OTHER point's
  simulation_cycle in that group before computing its relative error:
      aligned_sim = simulation_cycle - offset
      error = |aligned_sim - ground_truth_cycle| / ground_truth_cycle
  The anchor point itself is exactly correct by construction (error == 0
  after alignment) and is EXCLUDED from that benchmark's average, same as
  the anchor-exclusion rule in draw_amd_result.py. A benchmark whose group
  has only one point total (nothing besides the anchor) contributes zero
  points to the aligned average and is dropped, reported via a WARNING.
  The "_not_aligned" version skips all of this: plain
  |simulation_cycle - ground_truth_cycle| / ground_truth_cycle per point,
  no anchor, no exclusions.

Method (both versions, once each has its list of per-point errors):
  - Each benchmark's bar = mean of its points' relative errors, pooled
    across every param_json combo for that (suite, benchmark) (division
    done per-point first, then averaged).
  - BRUTE-FORCE OUTLIER FILTER: any benchmark whose bar value exceeds
    FILTER_MAX_RATIO (default 1.05) is dropped from the chart entirely and
    reported via a WARNING with its actual value, so the y-axis stays
    readable for everything else. Applied independently to each version
    (a benchmark can survive in one and not the other).
  - The trailing "mean" bar = unweighted mean of the *surviving* per-
    benchmark bar values.
  - Suites with fewer than MIN_BENCHMARKS_PER_SUITE surviving benchmarks
    are dropped entirely.
  - x-axis labels are "{suite}-{benchmark}", with a redundant "{suite}_"
    prefix in the benchmark name stripped if present.

Usage:
    python3 plot_nvidia.py --csv H100_raw.csv --out h100_relative_error.png
    (writes h100_relative_error_aligned.png and h100_relative_error_not_aligned.png)
"""

import argparse
import csv
from collections import defaultdict
from pathlib import Path

import matplotlib.pyplot as plt
from matplotlib.ticker import MultipleLocator, FormatStrFormatter

# ---------------------------------------------------------------------------
# Tweakable presentation settings -- kept up top so they're easy to find.
# Styled to match draw_amd_result.py's theme: green bars with a black
# outline, no in-plot title, no value labels on bars, a shaded background
# band behind the summary bar, a full box frame, fixed 0.25-interval
# y-ticks, light vertical separators between bars, and a serif font.
# ---------------------------------------------------------------------------
FONT_FAMILY = "serif"
FONT_SERIF_STACK = ["Nimbus Roman", "Liberation Serif", "DejaVu Serif"]  # "Times New Roman",

FONT_SIZE_AXIS_LABEL = 16#14
FONT_SIZE_TICK = 16#13

BAR_COLOR = "#00B050"        # rgb(0,176,80), matching draw_amd_result.py
MEAN_BAR_COLOR = "#00B050"   # same green; the shaded band behind it is what sets it apart
BAR_EDGE_COLOR = "black"
BAR_EDGE_WIDTH = 1.2

SHOW_TITLE = False           # reference chart has no in-plot title
SHOW_VALUE_LABELS = False    # reference chart has no numbers printed above bars

MEAN_BAND_COLOR = "#D9D9D9"  # light gray band behind the summary bar
MEAN_BAND_HALF_WIDTH = 0.45  # half-width (in x-axis units) of the shaded band

Y_TICK_STEP = 0.25           # fixed tick spacing: 0.00/0.25/0.50/0.75/1.00/...
Y_TICK_FORMAT = "%.2f"

BORDER_LINEWIDTH = 1.8       # thickness of the box frame, gridlines, and ticks

# Light vertical separators between adjacent bars (including the gap into
# the mean bar).
VLINE_COLOR = "#DDDDDD"
VLINE_WIDTH = 0.9

# Benchmarks excluded from the chart entirely, in addition to whatever
# --exclude adds. Empty by default -- nothing here has a known
# data-quality problem; extreme benchmarks are instead caught by
# FILTER_MAX_RATIO below.
DEFAULT_EXCLUDE_BENCHMARKS = set()

# Brute-force outlier filter: any benchmark whose bar value (mean per-point
# relative error) exceeds this is dropped entirely and reported.
FILTER_MAX_RATIO = 1.05

# Suites with fewer than this many surviving benchmarks (after the above
# filters) are dropped entirely -- too few bars to say anything about "this
# suite" as a group.
MIN_BENCHMARKS_PER_SUITE = 3

FIGSIZE = (18, 4)   # matches draw_amd_result.py's default for visual consistency
                     # between the two companion figures; widen back out (e.g.
                     # to the previous (16, 9)) if this dataset's bar count
                     # makes labels too cramped at 18-wide.
DPI = 300
BAR_WIDTH = 0.35
# Now that the mean bar is set apart by its shaded background band rather
# than by whitespace, it doesn't need extra gap.
GAP_BEFORE_MEAN = 0.0
LABEL_ROTATION = 25         # kept steeper than the AMD chart's 30 -- more/longer labels to fit

TITLE = "Simulator Performance on NVIDIA H100 GPU"
YLABEL = "|Sim − HW| / HW"


def format_label(suite, benchmark):
    """'polybench', 'polybench_2dconv' -> 'polybench-2dconv' (drop the
    benchmark's redundant repeat of its own suite name), matching
    draw_amd_result.py. No-op for this dataset's naming, kept for
    consistency / in case that changes."""
    prefix = suite + "_"
    short = benchmark[len(prefix):] if benchmark.startswith(prefix) else benchmark
    return f"{suite}-{short}"


def load_raw_points(path):
    """(suite, benchmark) -> list of (ground_truth_cycle, simulation_cycle,
    param_json) tuples, one per row, pooled across every param_json combo.
    Also returns the set of distinct profile_types seen."""
    raw_by_bench = defaultdict(list)
    profile_types = set()
    with open(path, newline="") as f:
        for row in csv.DictReader(f):
            profile_types.add(row["profile_type"])
            gt = float(row["ground_truth_cycle"])
            sim = float(row["simulation_cycle"])
            raw_by_bench[(row["suite"], row["benchmark"])].append((gt, sim, row["param_json"]))
    return raw_by_bench, profile_types


def compute_errors(raw_by_bench, aligned):
    """(suite, benchmark) -> list of per-point relative errors.

    aligned=False: plain |sim - gt| / gt for every point.
    aligned=True: per the ALIGN RULE docstring above -- offset each group
    to its smallest-ground-truth point, exclude that anchor point from the
    result. Groups with only 1 point produce an empty list (dropped by the
    caller, with a warning)."""
    errors_by_bench = {}
    for key, points in raw_by_bench.items():
        if not aligned:
            errors_by_bench[key] = [abs(sim - gt) / gt for gt, sim, _param in points]
            continue

        anchor_idx = min(range(len(points)), key=lambda i: (points[i][0], points[i][2]))
        anchor_gt, anchor_sim, _anchor_param = points[anchor_idx]
        offset = anchor_sim - anchor_gt
        errs = []
        for i, (gt, sim, _param) in enumerate(points):
            if i == anchor_idx:
                continue
            aligned_sim = sim - offset
            errs.append(abs(aligned_sim - gt) / gt)
        errors_by_bench[key] = errs
    return errors_by_bench


def build_bars(errors_by_bench, exclude, filter_max_ratio, tag):
    """Apply exclude / outlier-filter / min-benchmarks-per-suite to a
    {(suite,benchmark): [errors]} dict and return (bench_labels,
    bench_values, mean_value), printing WARNINGs (prefixed with `tag`, e.g.
    "[aligned]") for everything dropped along the way."""
    if exclude:
        errors_by_bench = {k: v for k, v in errors_by_bench.items() if k[1] not in exclude}
        print(f"[{tag}] Excluding benchmark(s): {sorted(exclude)}")

    empty = sorted(format_label(s, b) for (s, b), errs in errors_by_bench.items() if not errs)
    if empty:
        print(
            f"[{tag}] WARNING: {len(empty)} benchmark(s) have zero non-anchor points "
            f"(every point for them was the anchor) and are OMITTED entirely: {empty}"
        )

    # One value per benchmark: mean of that benchmark's per-point relative errors.
    bench_mean = {k: sum(errs) / len(errs) for k, errs in errors_by_bench.items() if errs}

    # Brute-force outlier filter.
    over_limit = {k: v for k, v in bench_mean.items() if v > filter_max_ratio}
    if over_limit:
        removed = sorted(f"{format_label(s, b)}={v:.3f}" for (s, b), v in over_limit.items())
        print(
            f"[{tag}] WARNING: {len(over_limit)} benchmark(s) exceed --filter-max-ratio "
            f"({filter_max_ratio}) and are OMITTED entirely: {removed}"
        )
        bench_mean = {k: v for k, v in bench_mean.items() if k not in over_limit}

    # Drop whole suites that have too few surviving benchmarks.
    suite_counts = defaultdict(int)
    for (suite, _benchmark) in bench_mean:
        suite_counts[suite] += 1
    small_suites = {suite for suite, cnt in suite_counts.items() if cnt < MIN_BENCHMARKS_PER_SUITE}
    if small_suites:
        removed = sorted(format_label(s, b) for (s, b) in bench_mean if s in small_suites)
        print(
            f"[{tag}] WARNING: suite(s) {sorted(small_suites)} have fewer than "
            f"{MIN_BENCHMARKS_PER_SUITE} surviving benchmarks and are OMITTED entirely: {removed}"
        )
        bench_mean = {k: v for k, v in bench_mean.items() if k[0] not in small_suites}

    if not bench_mean:
        raise SystemExit(f"[{tag}] Nothing left to plot after filtering -- loosen --filter-max-ratio or --exclude.")

    labeled = {format_label(suite, benchmark): v for (suite, benchmark), v in bench_mean.items()}
    bench_labels = sorted(labeled.keys())
    bench_values = [labeled[label] for label in bench_labels]
    mean_value = sum(bench_values) / len(bench_values)
    return bench_labels, bench_values, mean_value


def plot(bench_labels, bench_values, mean_value, title, out_path):
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
    # adjacent bars (including the gap into the mean bar).
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
        ax.set_title(title, fontsize=FONT_SIZE_AXIS_LABEL + 4, pad=16)
    ax.set_ylabel(YLABEL, fontsize=FONT_SIZE_AXIS_LABEL)

    ax.yaxis.set_major_locator(MultipleLocator(Y_TICK_STEP))
    ax.yaxis.set_major_formatter(FormatStrFormatter(Y_TICK_FORMAT))
    ax.yaxis.grid(True, linestyle="--", color="#BBBBBB", linewidth=BORDER_LINEWIDTH * 0.6, alpha=0.8, zorder=1)
    ax.set_axisbelow(True)

    # Full box frame, instead of hiding top/right spines.
    for spine in ax.spines.values():
        spine.set_visible(True)
        spine.set_color("black")
        spine.set_linewidth(BORDER_LINEWIDTH)

    ax.tick_params(width=BORDER_LINEWIDTH * 0.7)

    ax.set_xlim(-0.8, mean_x + 0.8)

    fig.tight_layout()
    fig.savefig(out_path, dpi=DPI)
    plt.close(fig)
    print(f"Wrote {out_path}  ({n} benchmarks + 1 mean bar)")


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--csv", required=True, help="Path to H100_raw.csv")
    parser.add_argument("--out", default="h100_relative_error.png", help="Base output image path; '_aligned' / '_not_aligned' is inserted before the extension")
    parser.add_argument(
        "--exclude",
        default="",
        help="Comma-separated benchmark names to exclude in addition to the defaults (none by default)",
    )
    parser.add_argument(
        "--filter-max-ratio",
        type=float,
        default=FILTER_MAX_RATIO,
        help=f"Drop any benchmark whose mean relative error exceeds this (default {FILTER_MAX_RATIO})",
    )
    args = parser.parse_args()

    exclude = set(DEFAULT_EXCLUDE_BENCHMARKS) | {b.strip() for b in args.exclude.split(",") if b.strip()}

    raw_by_bench, profile_types = load_raw_points(args.csv)
    if len(profile_types) > 1:
        print(f"NOTE: multiple profile_types found and pooled together: {sorted(profile_types)}")

    out_path = Path(args.out)
    variants = [
        ("not_aligned", False, TITLE + "", out_path.with_name(f"{out_path.stem}_not_aligned{out_path.suffix}")),
        ("aligned", True, TITLE + "", out_path.with_name(f"{out_path.stem}{out_path.suffix}")),  # _aligned
    ]

    for tag, aligned, title, path in variants:
        errors_by_bench = compute_errors(raw_by_bench, aligned=aligned)
        bench_labels, bench_values, mean_value = build_bars(errors_by_bench, exclude, args.filter_max_ratio, tag)
        plot(bench_labels, bench_values, mean_value, title, path)


if __name__ == "__main__":
    main()