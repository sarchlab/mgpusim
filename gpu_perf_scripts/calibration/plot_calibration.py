#!/usr/bin/env python3
"""Plot kernel-execution-time calibration figures: simulation vs real hardware.

The report is figures only (no prose). One figure per non-scaling-parameter
combination, with two lines -- simulator and real MI300A hardware:

  benchmark        non-scaling combo -> one figure   x axis (scaling factor)
  ---------------- -------------------------------    -----------------------
  fp32_throughput  (num_blocks, threads_per_block)    fmas_per_thread
  cache_latency    (num_accesses,)                    array_bytes

Y axis is kernel execution time in ms (real from the ground-truth CSV's
kernel_ms_mean; sim from kernel_time_s x 1000). Real HW carries a constant
per-launch overhead the timing model does not capture, so each simulation series
is first ALIGNED to real HW: a constant offset = real - sim at the smallest
shared scaling factor is added to every sim point, making that smallest point
match exactly. The average error = mean |sim_aligned - real| (ms) over the shared
points EXCLUDING that anchor; per-figure values go in the figure title and a
compact summary is written to --summary / stdout.

  --ref        ground-truth summary CSV (provides kernel_ms_mean)
  --fp32-sim   fp32_throughput sweep CSV (run_sim_sweep.sh output)
  --cache-sim  cache_latency sweep CSV (run_cache_latency_sweep.sh output)
  --out        output directory for the PNGs
  --summary    optional file to append the average-error summary to

Uses seaborn for styling when available, falling back to plain matplotlib.
"""

import argparse
import csv
import json
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


def align_sim_to_real(sim_pts, real_pts):
    """Absorb real hardware's unmodeled fixed overhead, then score the rest.

    Real HW has a constant per-launch overhead the timing model does not capture.
    We estimate it as the gap at the smallest shared scaling factor x0 and add it
    as a constant to EVERY simulation point, so the sim line is shifted up to meet
    real HW and the smallest point matches exactly (by construction).

    Returns (sim_adj, offset, anchor_x, errs):
      sim_adj   -- [[x, y + offset], ...] (the aligned simulation series)
      offset    -- real(x0) - sim(x0) at the smallest shared x0 (the overhead)
      anchor_x  -- x0
      errs      -- {"mape": .., "smape": .., "n": ..} RELATIVE errors (fractions)
                   over shared x EXCLUDING the anchor:
                     MAPE  = mean |sim_adj - real| / real
                     sMAPE = mean |sim_adj - real| / min(sim_adj, real)
    With no shared x, sim is returned unshifted and offset/anchor_x are None.
    """
    real = dict(real_pts)
    sim = dict(sim_pts)
    shared = sorted(x for x in sim if x in real)
    if not shared:
        return [[x, y] for x, y in sim_pts], None, None, {"mape": None, "smape": None, "n": 0}
    x0 = shared[0]
    offset = real[x0] - sim[x0]
    sim_adj = [[x, y + offset] for x, y in sim_pts]
    mape, smape = [], []
    for x in shared[1:]:
        s, h = sim[x] + offset, real[x]
        if h != 0:
            mape.append(abs(s - h) / h)
        if min(s, h) != 0:
            smape.append(abs(s - h) / min(s, h))
    errs = {
        "mape": (sum(mape) / len(mape)) if mape else None,
        "smape": (sum(smape) / len(smape)) if smape else None,
        "n": len(shared) - 1,
    }
    return sim_adj, offset, x0, errs


def _pct(v):
    return f"{v * 100:.1f}%" if v is not None else "n/a"


def error_caption(offset, errs):
    """Human-readable per-figure error line shared by the PNG title and report."""
    if offset is None:
        return "MAPE / sMAPE = n/a (no matched points)"
    if errs["mape"] is None and errs["smape"] is None:
        return f"offset {offset:+,.3f} ms · err = n/a (only the anchor matched)"
    return (f"offset {offset:+,.3f} ms · MAPE {_pct(errs['mape'])} · "
            f"sMAPE {_pct(errs['smape'])} (n={errs['n']}, anchor excl.)")


def plot_benchmark(benchmark, spec, ref_groups, sim_groups, out_dir):
    """One figure per simulated combo. Returns a list of per-figure dicts:
    {label, slug, mae, n, offset, anchor_x, fig, sim, real} -- `sim` is the
    overhead-aligned simulation series ([[x, y_ms], ...]); `real` is unchanged.

    Iterates the combos we actually simulated (sim_groups): each figure is a
    sim-vs-real comparison, so a combo with no sim data is not a figure.
    """
    results = []
    for key in sorted(sim_groups):
        sim_pts = sim_groups.get(key, [])
        real_pts = ref_groups.get(key, [])
        sim_adj, offset, anchor_x, errs = align_sim_to_real(sim_pts, real_pts)
        label = spec["label"](key)

        fig, ax = plt.subplots(figsize=(7, 5))
        if sim_adj:
            ax.plot([x for x, _ in sim_adj], [y for _, y in sim_adj],
                    marker="o", color=SIM_COLOR, label="simulation (aligned)")
        if real_pts:
            ax.plot([x for x, _ in real_pts], [y for _, y in real_pts],
                    marker="s", color=REAL_COLOR, label="real hardware")
        if anchor_x is not None:
            ax.plot([anchor_x], [dict(real_pts)[anchor_x]], marker="*",
                    color="black", markersize=15, linestyle="none", zorder=5,
                    label="anchor (excluded)")
        ax.set_xscale("log", base=2)
        ax.set_yscale("log")
        ax.set_xlabel(spec["xlabel"])
        ax.set_ylabel("kernel execution time (ms)")
        err_txt = error_caption(offset, errs)
        ax.set_title(f"{benchmark}\n{label}\n{err_txt}", fontsize=11)
        ax.legend(fontsize=9)
        fig.tight_layout()
        path = os.path.join(out_dir, f"{benchmark}_{spec['slug'](key)}.png")
        fig.savefig(path, dpi=120)
        plt.close(fig)
        results.append({
            "label": label, "slug": spec["slug"](key),
            "mape": errs["mape"], "smape": errs["smape"], "n": errs["n"],
            "offset": offset, "anchor_x": anchor_x,
            "fig": path,
            "sim": sim_adj,
            "real": [[x, y] for x, y in real_pts],
        })
        print(f"wrote {path}  (sim={len(sim_adj)}, real={len(real_pts)}, {err_txt})")
    return results


def overall(rows, key):
    """Mean of a per-figure relative metric (key in {'mape','smape'}), or None."""
    vals = [r[key] for r in rows if r.get(key) is not None]
    return (sum(vals) / len(vals)) if vals else None


def build_summary_table(rows):
    """Compact markdown table + overall error line (for the CI job summary)."""
    lines = ["| Benchmark | Non-scaling combo | MAPE | sMAPE | Matched pts |",
             "|-----------|-------------------|-----:|------:|------------:|"]
    n_fig = 0
    for r in rows:
        mape = _pct(r["mape"]) if r["mape"] is not None else "—"
        smape = _pct(r["smape"]) if r["smape"] is not None else "—"
        lines.append(f"| {r['benchmark']} | {r['label']} | {mape} | {smape} | {r['n']} |")
        if r["mape"] is not None or r["smape"] is not None:
            n_fig += 1
    lines.append(f"\n**Overall: MAPE {_pct(overall(rows, 'mape'))} · "
                 f"sMAPE {_pct(overall(rows, 'smape'))}** "
                 f"(mean of per-figure error over {n_fig} figures)")
    return "\n".join(lines)


def build_report_md(rows, report_path):
    """Full visual report: summary table followed by every figure embedded inline.

    Image links are written relative to the report file's directory so the
    markdown renders both in a local preview and when the report ships next to
    figures/ in the CI artifact.
    """
    report_dir = os.path.dirname(os.path.abspath(report_path))
    out = ["# MI300A Calibration Report", "",
           "Simulated vs. real-hardware **kernel execution time** for the MI300A "
           "(CDNA3) timing model. One figure per non-scaling-parameter combination; "
           "x axis is the scaling factor, y axis is kernel time (log-log). The "
           "simulation is **aligned** to real HW by adding a constant `offset` "
           "(= real − sim at the smallest shared scaling factor, the anchor) to "
           "absorb real HW's unmodeled fixed overhead. Errors are RELATIVE, over "
           "the shared points **excluding the anchor**: `MAPE` = mean |sim−hw|/hw, "
           "`sMAPE` = mean |sim−hw|/min(sim,hw).", "",
           build_summary_table(rows), ""]
    current = None
    for r in rows:
        if r["benchmark"] != current:
            current = r["benchmark"]
            out += ["", f"## {current}", ""]
        err = error_caption(r.get("offset"),
                            {"mape": r["mape"], "smape": r["smape"], "n": r["n"]})
        out += [f"### {r['label']}", "", f"_{err}_", ""]
        if r["fig"]:
            rel = os.path.relpath(os.path.abspath(r["fig"]), start=report_dir)
            out += [f"![{r['benchmark']} — {r['label']}]({rel})", ""]
    return "\n".join(out)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--ref", default="gpu_perf_scripts/calibration/mi300a_ground_truth.csv")
    ap.add_argument("--fp32-sim", default="fp32_sim_results.csv")
    ap.add_argument("--cache-sim", default="cache_latency_sim_results.csv")
    ap.add_argument("--out", default="figures")
    ap.add_argument("--summary", default="",
                    help="append the compact table to this file (e.g. $GITHUB_STEP_SUMMARY)")
    ap.add_argument("--report", default="",
                    help="write a standalone markdown report with the figures embedded inline")
    ap.add_argument("--json", dest="json_out", default="",
                    help="write the figure series + metadata as JSON (consumed by the web dashboard)")
    ap.add_argument("--run-id", default="", help="run identifier recorded in the JSON")
    ap.add_argument("--run-url", default="", help="run URL recorded in the JSON")
    ap.add_argument("--generated-at", default="", help="ISO timestamp recorded in the JSON")
    args = ap.parse_args()

    os.makedirs(args.out, exist_ok=True)
    sim_paths = {"fp32_throughput": args.fp32_sim, "cache_latency": args.cache_sim}

    rows = []
    benchmarks = []   # (name, spec, [figure dicts]) -- preserves SPECS order
    for benchmark, spec in SPECS.items():
        ref_groups = read_ref(args.ref, benchmark, spec["nonscaling"])
        sim_groups = read_sim(sim_paths[benchmark], benchmark,
                              spec["nonscaling"], spec["scaling"])
        figs = plot_benchmark(benchmark, spec, ref_groups, sim_groups, args.out)
        benchmarks.append((benchmark, spec, figs))
        for f in figs:
            rows.append({"benchmark": benchmark, "label": f["label"],
                         "mape": f["mape"], "smape": f["smape"], "n": f["n"],
                         "offset": f["offset"], "fig": f["fig"]})

    summary = build_summary_table(rows)
    print("\n" + summary)
    if args.summary:
        with open(args.summary, "a") as f:
            f.write(summary + "\n")
    if args.report:
        md = build_report_md(rows, args.report)
        with open(args.report, "w") as f:
            f.write(md + "\n")
        print(f"\nwrote report {args.report}")
    if args.json_out:
        data = {
            "schema": 1,
            "run_id": args.run_id,
            "run_url": args.run_url,
            "generated_at": args.generated_at,
            "metric": "kernel execution time (ms)",
            "overall_mape": overall(rows, "mape"),
            "overall_smape": overall(rows, "smape"),
            "benchmarks": [
                {
                    "name": name,
                    "xlabel": spec["xlabel"],
                    "figures": [{k: f[k] for k in
                                 ("label", "slug", "mape", "smape", "n", "offset",
                                  "anchor_x", "sim", "real")}
                                for f in figs],
                }
                for (name, spec, figs) in benchmarks
            ],
        }
        with open(args.json_out, "w") as f:
            json.dump(data, f, indent=2)
        print(f"\nwrote json {args.json_out}")


if __name__ == "__main__":
    main()
