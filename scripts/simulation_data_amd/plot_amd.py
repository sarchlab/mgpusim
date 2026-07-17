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

# ---------------------------------------------------------------------------
# Tweakable presentation settings -- kept up top so they're easy to find.
# ---------------------------------------------------------------------------
FONT_SIZE_TITLE = 20 # 22
FONT_SIZE_AXIS_LABEL = 20 # 18
FONT_SIZE_TICK = 20 # 13
FONT_SIZE_ANNOTATION = 12 #  11

BAR_COLOR = "#E67E22"       # warm orange
MEAN_BAR_COLOR = "#E67E22"  # same color; change if you want the Mean bar to stand out

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

FIGSIZE = (18, 9)
DPI = 300
BAR_WIDTH = 0.6
GAP_BEFORE_MEAN = 1.5       # extra x-axis units of empty space before the Mean bar
LABEL_ROTATION = 45

TITLE = "Simulator Performance on AMD MI300X GPU"
YLABEL = "Relative Error  |Sim \u2212 Real| / Real"


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

    # ---- plotting ----
    n = len(bench_labels)
    x = list(range(n))
    mean_x = n - 1 + GAP_BEFORE_MEAN + 1  # one bar-width slot after the last benchmark, plus the gap

    fig, ax = plt.subplots(figsize=FIGSIZE)

    ax.bar(x, bench_values, width=BAR_WIDTH, color=BAR_COLOR, zorder=3)
    ax.bar([mean_x], [mean_value], width=BAR_WIDTH, color=MEAN_BAR_COLOR, zorder=3)

    for xi, v in zip(x, bench_values):
        ax.text(xi, v, f"{v:.3f}", ha="center", va="bottom", fontsize=FONT_SIZE_ANNOTATION, zorder=4)
    ax.text(mean_x, mean_value, f"{mean_value:.3f}", ha="center", va="bottom", fontsize=FONT_SIZE_ANNOTATION, zorder=4)

    ax.set_xticks(x + [mean_x])
    ax.set_xticklabels(bench_labels + ["mean"], rotation=LABEL_ROTATION, ha="right", fontsize=FONT_SIZE_TICK)
    ax.tick_params(axis="y", labelsize=FONT_SIZE_TICK)

    ax.set_title(TITLE, fontsize=FONT_SIZE_TITLE, pad=16)
    ax.set_ylabel(YLABEL, fontsize=FONT_SIZE_AXIS_LABEL)

    ax.yaxis.grid(True, linestyle="--", alpha=0.4, zorder=0)
    ax.set_axisbelow(True)
    for spine in ("top", "right"):
        ax.spines[spine].set_visible(False)

    fig.tight_layout()
    fig.savefig(args.out, dpi=DPI)
    print(f"Wrote {args.out}  ({n} benchmarks + 1 mean bar)")


if __name__ == "__main__":
    main()