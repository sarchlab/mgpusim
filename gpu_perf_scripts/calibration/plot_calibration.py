#!/usr/bin/env python3
"""Plot kernel-execution-time calibration figures: simulation vs real hardware.

The report is figures only (no prose). One figure per non-scaling-parameter
combination, with two lines -- simulator and real MI300A hardware:

  benchmark        non-scaling combo -> one figure   x axis (scaling factor)
  ---------------- -------------------------------    -----------------------
  fp32_throughput  (num_blocks, threads_per_block)    fmas_per_thread
  cache_latency    (num_accesses,)                    array_bytes

Y axis is kernel execution time in ms (real from the ground-truth CSV's
kernel_ms_mean; sim from kernel_time_s x 1000). For each figure we also report
the average error = mean |sim - real| kernel time (ms) over the points present
in BOTH lines; per-figure values go in the figure title and a compact summary
is written to --summary / stdout.

  --ref        ground-truth summary CSV (provides kernel_ms_mean)
  --fp32-sim   fp32_throughput sweep CSV (run_sim_sweep.sh output)
  --cache-sim  cache_latency sweep CSV (run_cache_latency_sweep.sh output)
  --out        output directory for the PNGs
  --summary    optional file to append the average-error summary to

Uses seaborn for styling when available, falling back to plain matplotlib.
"""

import argparse
import csv
import os
from collections import defaultdict

import matplotlib
matplotlib.use("Agg")  # headless / CI
import matplotlib.pyplot as plt  # noqa: E402

try:
    import seaborn as sns
    sns.set_theme(style="whitegrid", context="talk")
    _PALETTE = sns.color_palette()
    SIM_COLOR, REAL_COLOR = _PALETTE[0], _PALETTE[3]
except ImportError:  # seaborn optional
    SIM_COLOR, REAL_COLOR = "C0", "C3"


# Per-benchmark plotting spec. `nonscaling` are the figure-key columns; `scaling`
# is the x-axis column in the sim CSV (matched against the ground truth's
# scaling_param_value); `label` formats the non-scaling combo for titles/files.
SPECS = {
    "fp32_throughput": {
        "nonscaling": ["num_blocks", "threads_per_block"],
        "scaling": "fmas_per_thread",
        "xlabel": "fmas_per_thread (scaling factor)",
        "label": lambda k: f"num_blocks={k[0]}, threads_per_block={k[1]}",
        "slug": lambda k: f"nb{k[0]}_tpb{k[1]}",
    },
    "cache_latency": {
        "nonscaling": ["num_accesses"],
        "scaling": "array_bytes",
        "xlabel": "array_bytes (scaling factor)",
        "label": lambda k: f"num_accesses={k[0]}",
        "slug": lambda k: f"na{k[0]}",
    },
}


def read_ref(path, benchmark, nonscaling):
    """(nonscaling-tuple) -> sorted [(scaling_value, real_kernel_ms)]."""
    groups = defaultdict(list)
    with open(path, newline="") as f:
        for r in csv.DictReader(f):
            if r["benchmark"] != benchmark:
                continue
            try:
                key = tuple(int(r[c]) for c in nonscaling)
                x = int(r["scaling_param_value"])
                y = float(r["kernel_ms_mean"])
            except (ValueError, KeyError):
                continue
            groups[key].append((x, y))
    for k in groups:
        groups[k].sort()
    return groups


def read_sim(path, benchmark, nonscaling, scaling):
    """(nonscaling-tuple) -> sorted [(scaling_value, sim_kernel_ms)]."""
    groups = defaultdict(list)
    if not path or not os.path.exists(path):
        return groups
    with open(path, newline="") as f:
        for r in csv.DictReader(f):
            if r["benchmark"] != benchmark:
                continue
            try:
                key = tuple(int(r[c]) for c in nonscaling)
                x = int(r[scaling])
                y = float(r["kernel_time_s"]) * 1000.0  # s -> ms
            except (ValueError, KeyError):
                continue
            groups[key].append((x, y))
    for k in groups:
        groups[k].sort()
    return groups


def avg_abs_error(sim_pts, real_pts):
    """Mean |sim - real| kernel time (ms) over shared x; (mae, n_matched)."""
    real = dict(real_pts)
    errs = [abs(y - real[x]) for x, y in sim_pts if x in real]
    if not errs:
        return None, 0
    return sum(errs) / len(errs), len(errs)


def plot_benchmark(benchmark, spec, ref_groups, sim_groups, out_dir):
    """One figure per simulated non-scaling combo. Returns [(label, mae, n), ...].

    Iterates the combos we actually simulated (sim_groups): each figure is a
    sim-vs-real comparison, so a combo with no sim data is not a figure.
    """
    results = []
    keys = sorted(sim_groups)
    for key in keys:
        sim_pts = sim_groups.get(key, [])
        real_pts = ref_groups.get(key, [])
        mae, n = avg_abs_error(sim_pts, real_pts)
        label = spec["label"](key)

        fig, ax = plt.subplots(figsize=(7, 5))
        if sim_pts:
            ax.plot([x for x, _ in sim_pts], [y for _, y in sim_pts],
                    marker="o", color=SIM_COLOR, label="simulation")
        if real_pts:
            ax.plot([x for x, _ in real_pts], [y for _, y in real_pts],
                    marker="s", color=REAL_COLOR, label="real hardware")
        ax.set_xscale("log", base=2)
        ax.set_yscale("log")
        ax.set_xlabel(spec["xlabel"])
        ax.set_ylabel("kernel execution time (ms)")
        err_txt = f"avg |err| = {mae:,.3f} ms (n={n})" if mae is not None \
            else "avg |err| = n/a (no matched points)"
        ax.set_title(f"{benchmark}\n{label}\n{err_txt}", fontsize=12)
        ax.legend()
        fig.tight_layout()
        path = os.path.join(out_dir, f"{benchmark}_{spec['slug'](key)}.png")
        fig.savefig(path, dpi=120)
        plt.close(fig)
        results.append((label, mae, n))
        print(f"wrote {path}  (sim={len(sim_pts)}, real={len(real_pts)}, {err_txt})")
    return results


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--ref", default="gpu_perf_scripts/calibration/mi300a_ground_truth.csv")
    ap.add_argument("--fp32-sim", default="fp32_sim_results.csv")
    ap.add_argument("--cache-sim", default="cache_latency_sim_results.csv")
    ap.add_argument("--out", default="figures")
    ap.add_argument("--summary", default="")
    args = ap.parse_args()

    os.makedirs(args.out, exist_ok=True)
    sim_paths = {"fp32_throughput": args.fp32_sim, "cache_latency": args.cache_sim}

    summary_lines = ["| Benchmark | Non-scaling combo | Avg |err| (ms) | Matched pts |",
                     "|-----------|-------------------|---------------:|------------:|"]
    all_maes = []
    for benchmark, spec in SPECS.items():
        ref_groups = read_ref(args.ref, benchmark, spec["nonscaling"])
        sim_groups = read_sim(sim_paths[benchmark], benchmark,
                              spec["nonscaling"], spec["scaling"])
        for label, mae, n in plot_benchmark(benchmark, spec, ref_groups,
                                            sim_groups, args.out):
            cell = f"{mae:,.3f}" if mae is not None else "—"
            summary_lines.append(f"| {benchmark} | {label} | {cell} | {n} |")
            if mae is not None:
                all_maes.append(mae)

    overall = sum(all_maes) / len(all_maes) if all_maes else None
    overall_txt = f"{overall:,.3f} ms" if overall is not None else "n/a"
    summary_lines.append(f"\n**Overall average error: {overall_txt}** "
                         f"(mean of per-figure avg |err| over {len(all_maes)} figures)")

    summary = "\n".join(summary_lines)
    print("\n" + summary)
    if args.summary:
        with open(args.summary, "a") as f:
            f.write(summary + "\n")


if __name__ == "__main__":
    main()
