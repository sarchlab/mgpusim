#!/usr/bin/env python3
"""Plot kernel-execution-time calibration figures (simulation vs real hardware)
for ANY benchmark, driven entirely by the CSVs (no per-benchmark code).

Inputs:
  --ref       ground-truth CSV: benchmark, scaling_param_name, scaling_param_value,
              non_scaling_json, kernel_ms_mean, n_reps
  --sim-dir   directory searched (recursively) for *_sim_results.csv produced by
              run_sweep.py: same key columns + kernel_time_s
  --out       output dir for the PNGs
  --summary   optional file to append the compact table to ($GITHUB_STEP_SUMMARY)
  --report    optional standalone markdown report with the figures embedded
  --json      optional JSON dump of the figure series + metadata

One figure per (benchmark, non-scaling combo): x = scaling value, y = kernel time
(log-log). The simulation is ALIGNED to real HW by a constant offset (= real-sim
at the smallest shared scaling factor, the anchor) to absorb real HW's unmodeled
fixed overhead; error is RELATIVE over the shared points EXCLUDING the anchor:
MAPE = mean |sim-hw|/hw, sMAPE = mean |sim-hw|/min(sim,hw).
"""

import argparse
import csv
import glob
import json
import math
import os
from collections import defaultdict

import matplotlib
matplotlib.use("Agg")  # headless / CI
import matplotlib.pyplot as plt  # noqa: E402

from calib_common import parse_ns, ns_label, ns_slug, xparse  # noqa: E402

try:
    import seaborn as sns
    sns.set_theme(style="whitegrid", context="talk")
    _PALETTE = sns.color_palette()
    SIM_COLOR, REAL_COLOR = _PALETTE[0], _PALETTE[3]
except ImportError:  # seaborn optional
    SIM_COLOR, REAL_COLOR = "C0", "C3"


def read_groups(path, y_field, y_scale=1.0):
    """benchmark -> {non_scaling_json -> {'scaling': name, 'pts': sorted [(x, y)]}}."""
    groups = defaultdict(lambda: defaultdict(lambda: {"scaling": "", "pts": {}}))
    if not path or not os.path.exists(path):
        return groups
    with open(path, newline="") as f:
        for r in csv.DictReader(f):
            try:
                bench = r["benchmark"]
                ns = r.get("non_scaling_json", "{}") or "{}"
                x = xparse(r["scaling_param_value"])
                y = float(r[y_field]) * y_scale
            except (ValueError, KeyError):
                continue
            g = groups[bench][ns]
            g["scaling"] = r.get("scaling_param_name", "") or g["scaling"]
            g["pts"][x] = y
    out = defaultdict(dict)
    for bench, combos in groups.items():
        for ns, g in combos.items():
            out[bench][ns] = {"scaling": g["scaling"], "pts": sorted(g["pts"].items())}
    return out


def read_sim_dir(sim_dir, y_field="kernel_time_s", y_scale=1000.0):
    """Merge all *_sim_results.csv under sim_dir into one read_groups structure."""
    merged = defaultdict(dict)
    for path in sorted(glob.glob(os.path.join(sim_dir, "**", "*_sim_results.csv"), recursive=True)):
        for bench, combos in read_groups(path, y_field, y_scale).items():
            merged[bench].update(combos)
    return merged


def align_sim_to_real(sim_pts, real_pts):
    """(sim_adj, offset, anchor_x, {mape, smape, n}); see module docstring."""
    real = dict(real_pts)
    sim = dict(sim_pts)
    shared = sorted(x for x in sim if x in real)
    if not shared:
        return [[x, y] for x, y in sim_pts], None, None, {"mape": None, "smape": None, "n": 0, "geomean": None}
    x0 = shared[0]
    offset = real[x0] - sim[x0]
    sim_adj = [[x, y + offset] for x, y in sim_pts]
    mape, smape, logr = [], [], []
    for x in shared:  # raw multiplicative ratio sim/real -- reference only, never the score
        if sim[x] > 0 and real[x] > 0:
            logr.append(math.log(sim[x] / real[x]))
    for x in shared[1:]:
        s, h = sim[x] + offset, real[x]
        if h != 0:
            mape.append(abs(s - h) / h)
        if min(s, h) != 0:
            smape.append(abs(s - h) / min(s, h))
    errs = {"mape": (sum(mape) / len(mape)) if mape else None,
            "smape": (sum(smape) / len(smape)) if smape else None,
            "n": len(shared) - 1,
            "geomean": math.exp(sum(logr) / len(logr)) if logr else None}
    return sim_adj, offset, x0, errs


def _pct(v):
    return f"{v * 100:.1f}%" if v is not None else "n/a"


def _fmtk(k):
    return None if not k else (f"{k:.2f}×" if k < 10 else f"{k:.1f}×")


def error_caption(offset, errs):
    if offset is None:
        return "MAPE / sMAPE = n/a (no matched points)"
    ksfx = f" · k≈{_fmtk(errs.get('geomean'))} (ref)" if errs.get("geomean") else ""
    if errs["mape"] is None and errs["smape"] is None:
        return f"offset {offset:+,.3f} ms · err = n/a (only the anchor matched)" + ksfx
    return (f"offset {offset:+,.3f} ms · MAPE {_pct(errs['mape'])} · "
            f"sMAPE {_pct(errs['smape'])} (n={errs['n']}, anchor excl.)" + ksfx)


def plot_benchmark(benchmark, ref_combos, sim_combos, out_dir):
    """One figure per simulated non-scaling combo. Returns per-figure dicts."""
    results = []
    for ns in sorted(sim_combos):
        sim_pts = sim_combos[ns]["pts"]
        real_pts = ref_combos.get(ns, {}).get("pts", [])
        scaling = sim_combos[ns]["scaling"] or ref_combos.get(ns, {}).get("scaling", "")
        xlabel = f"{scaling} (scaling factor)" if scaling else "scaling factor"
        sim_adj, offset, anchor_x, errs = align_sim_to_real(sim_pts, real_pts)
        label, slug = ns_label(parse_ns(ns)), ns_slug(parse_ns(ns))

        fig, ax = plt.subplots(figsize=(7, 5))
        if sim_adj:
            ax.plot([x for x, _ in sim_adj], [y for _, y in sim_adj],
                    marker="o", color=SIM_COLOR, label="simulation (aligned)")
        if real_pts:
            ax.plot([x for x, _ in real_pts], [y for _, y in real_pts],
                    marker="s", color=REAL_COLOR, label="real hardware")
        if anchor_x is not None:
            ax.plot([anchor_x], [dict(real_pts)[anchor_x]], marker="*", color="black",
                    markersize=15, linestyle="none", zorder=5, label="anchor (excluded)")
        ax.set_xscale("log", base=2)
        ax.set_yscale("log")
        ax.set_xlabel(xlabel)
        ax.set_ylabel("kernel execution time (ms)")
        err_txt = error_caption(offset, errs)
        ax.set_title(f"{benchmark}\n{label}\n{err_txt}", fontsize=11)
        ax.legend(fontsize=9)
        fig.tight_layout()
        path = os.path.join(out_dir, f"{benchmark}_{slug}.png")
        fig.savefig(path, dpi=120)
        plt.close(fig)
        results.append({
            "label": label, "slug": slug, "xlabel": xlabel,
            "mape": errs["mape"], "smape": errs["smape"], "n": errs["n"],
            "geomean": errs["geomean"],
            "offset": offset, "anchor_x": anchor_x, "fig": path,
            "sim": sim_adj, "real": [[x, y] for x, y in real_pts],
        })
        print(f"wrote {path}  (sim={len(sim_adj)}, real={len(real_pts)}, {err_txt})")
    return results


def overall(rows, key):
    vals = [r[key] for r in rows if r.get(key) is not None]
    return (sum(vals) / len(vals)) if vals else None


def _ranks(vals):
    order = sorted(range(len(vals)), key=lambda i: vals[i])
    r = [0] * len(vals)
    for k, i in enumerate(order):
        r[i] = k + 1
    return r


def _pearson(a, b):
    ma, mb = sum(a) / len(a), sum(b) / len(b)
    cov = sum((x - ma) * (y - mb) for x, y in zip(a, b))
    va = math.sqrt(sum((x - ma) ** 2 for x in a))
    vb = math.sqrt(sum((y - mb) ** 2 for y in b))
    return cov / (va * vb) if va and vb else None


def global_metrics(ref, sim, out_dir, report_dir):
    """Cross-benchmark global calibration: rank fidelity + magnitude spread.

    Mirrors the dashboard's client-side computation. Returns (markdown, metrics)
    or (None, None) if there is too little data. Rank correlation grades
    cross-benchmark ORDERING (scale-invariant); geo-sigma is the multiplicative
    spread the single global constant k cannot remove -- neither is gameable by
    per-benchmark fitting.
    """
    logr, scatter = [], []
    by_bench = defaultdict(list)
    for bench, combos in sim.items():
        for ns, g in combos.items():
            real = dict(ref.get(bench, {}).get(ns, {}).get("pts", []))
            for x, s in g["pts"]:
                h = real.get(x)
                if h and s > 0 and h > 0:
                    lr = math.log(s / h)
                    logr.append(lr)
                    scatter.append((h, s, lr))
                    by_bench[bench].append((x, s, h, lr))
    if len(logr) < 2 or len(by_bench) < 2:
        return None, None

    mu = sum(logr) / len(logr)
    geo_k = math.exp(mu)
    geo_std = math.exp(math.sqrt(sum((v - mu) ** 2 for v in logr) / len(logr)))
    per_mean, within_sum, wtot = [], 0.0, 0
    for pts in by_bench.values():
        lrs = [q[3] for q in pts]
        m = sum(lrs) / len(lrs)
        per_mean.append(m)
        within_sum += sum((v - m) ** 2 for v in lrs)
        wtot += len(lrs)
    within = math.exp(math.sqrt(within_sum / wtot))
    bm = sum(per_mean) / len(per_mean)
    between = math.exp(math.sqrt(sum((v - bm) ** 2 for v in per_mean) / len(per_mean)))

    reps = []  # (benchmark, hw_ms, sim_ms) at each benchmark's largest shared config
    for bench, pts in by_bench.items():
        xmax = max(q[0] for q in pts)
        at = [q for q in pts if q[0] == xmax]
        reps.append((bench, sum(q[2] for q in at) / len(at), sum(q[1] for q in at) / len(at)))
    hw = [r[1] for r in reps]
    sm = [r[2] for r in reps]
    rh, rs = _ranks(hw), _ranks(sm)
    spearman = _pearson(rh, rs)
    n = len(reps)
    cc = dd = 0
    for i in range(n):
        for j in range(i + 1, n):
            sgn = (hw[i] - hw[j]) * (sm[i] - sm[j])
            cc += sgn > 0
            dd += sgn < 0
    kendall = (cc - dd) / (n * (n - 1) / 2) if n > 1 else None

    rank_png = os.path.join(out_dir, "_global_rank.png")
    scat_png = os.path.join(out_dir, "_global_scatter.png")
    fig, ax = plt.subplots(figsize=(6, 6))
    for (bench, _, _), a, c in zip(reps, rh, rs):
        offd = abs(a - c) >= 5
        ax.scatter([a], [c], color=(REAL_COLOR if offd else SIM_COLOR), s=45, zorder=3)
        if offd:
            ax.annotate(bench, (a, c), fontsize=7, xytext=(3, 3), textcoords="offset points")
    ax.plot([1, n], [1, n], "--", color="#888", lw=1, zorder=1)
    ax.set_xlabel("real-HW time rank (1 = fastest)")
    ax.set_ylabel("simulation time rank")
    ax.set_title(f"benchmark ranking: sim vs HW (diagonal = correct order)\n"
                 f"Spearman ρ={spearman:.2f} over {n} benchmarks", fontsize=11)
    fig.tight_layout()
    fig.savefig(rank_png, dpi=120)
    plt.close(fig)

    fig, ax = plt.subplots(figsize=(6.5, 6))
    xs = [p[0] for p in scatter]
    ys = [p[1] for p in scatter]
    cmax = max(abs(p[2] / math.log(10)) for p in scatter) or 1.0
    sc = ax.scatter(xs, ys, c=[p[2] / math.log(10) for p in scatter], cmap="RdBu_r",
                    vmin=-cmax, vmax=cmax, s=18)
    lo, hi = min(min(xs), min(ys)), max(max(xs), max(ys))
    ax.plot([lo, hi], [lo, hi], "--", color="#888", lw=1)
    ax.set_xscale("log")
    ax.set_yscale("log")
    ax.set_xlabel("real-HW kernel time (ms)")
    ax.set_ylabel("simulation kernel time (ms)")
    ax.set_title(f"absolute kernel time: sim vs real (y=x ideal)\n"
                 f"geo-σ={geo_std:.1f}× after one global constant", fontsize=11)
    fig.colorbar(sc, ax=ax, label="log10(sim/real)")
    fig.tight_layout()
    fig.savefig(scat_png, dpi=120)
    plt.close(fig)

    metrics = {"spearman": spearman, "kendall": kendall, "geomean_k": geo_k,
               "geo_std": geo_std, "within_factor": within, "between_factor": between,
               "n_benchmarks": n, "n_points": len(logr)}
    pairs = f" (≈{round((kendall + 1) / 2 * 100)}% of pairs ordered correctly)" if kendall is not None else ""

    def rel(p):
        return os.path.relpath(os.path.abspath(p), start=report_dir)

    md = "\n".join([
        "## Global calibration", "",
        "Cross-benchmark view, immune to per-benchmark fitting. **Rank correlation** "
        "grades whether the sim orders workloads by cost like real HW (scale-invariant); "
        "**geo-σ** is the typical multiplicative error left after the single global "
        "constant `k` (so it can't be lowered by per-benchmark fudging). `k` is a "
        "reference, not a score.", "",
        f"- **Spearman ρ = {spearman:.2f}** · **Kendall τ = {kendall:.2f}**{pairs}",
        f"- **geomean k = {geo_k:.2f}×** (single global constant, reference)",
        f"- **geo-σ = {geo_std:.2f}×** after that constant (ideal 1×) — "
        f"shape {within:.2f}× (within-benchmark) / level {between:.2f}× (between-benchmark)", "",
        f"![benchmark ranking]({rel(rank_png)})", "",
        f"![sim vs real absolute]({rel(scat_png)})", "",
    ])
    return md, metrics


def build_summary_table(rows):
    lines = ["| Benchmark | Non-scaling combo | MAPE | sMAPE | k (ref) | Matched pts |",
             "|-----------|-------------------|-----:|------:|--------:|------------:|"]
    n_fig = 0
    for r in rows:
        mape = _pct(r["mape"]) if r["mape"] is not None else "—"
        smape = _pct(r["smape"]) if r["smape"] is not None else "—"
        k = _fmtk(r.get("geomean")) or "—"
        lines.append(f"| {r['benchmark']} | {r['label']} | {mape} | {smape} | {k} | {r['n']} |")
        if r["mape"] is not None or r["smape"] is not None:
            n_fig += 1
    lines.append(f"\n**Overall: MAPE {_pct(overall(rows, 'mape'))} · "
                 f"sMAPE {_pct(overall(rows, 'smape'))}** "
                 f"(mean of per-figure error over {n_fig} figures)")
    return "\n".join(lines)


def build_report_md(rows, report_path, global_md=""):
    report_dir = os.path.dirname(os.path.abspath(report_path))
    out = ["# MI300X Calibration Report", "",
           "Simulated vs. real-hardware **kernel execution time** for the MI300X "
           "(CDNA3) timing model. One figure per (benchmark, non-scaling combo); "
           "x axis is the scaling factor, y axis is kernel time (log-log). The sim "
           "is **aligned** to real HW with a constant `offset` (= real − sim at the "
           "smallest shared scaling factor, the anchor). Errors are RELATIVE over "
           "the shared points **excluding the anchor**: `MAPE` = mean |sim−hw|/hw, "
           "`sMAPE` = mean |sim−hw|/min(sim,hw). `k (ref)` is the geomean sim/real "
           "ratio per figure — a reference for the multiplicative offset, not a score.", "",
           build_summary_table(rows), ""]
    if global_md:
        out += [global_md, ""]
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
    ap.add_argument("--ref", default="gpu_perf_scripts/calibration/mi300x_ground_truth.csv")
    ap.add_argument("--sim-dir", default=".", help="dir searched for *_sim_results.csv")
    ap.add_argument("--out", default="figures")
    ap.add_argument("--summary", default="")
    ap.add_argument("--report", default="")
    ap.add_argument("--json", dest="json_out", default="")
    ap.add_argument("--run-id", default="")
    ap.add_argument("--run-url", default="")
    ap.add_argument("--generated-at", default="")
    args = ap.parse_args()

    os.makedirs(args.out, exist_ok=True)
    ref = read_groups(args.ref, "kernel_ms_mean")
    sim = read_sim_dir(args.sim_dir)

    rows = []
    benchmarks = []
    for benchmark in sorted(sim):
        figs = plot_benchmark(benchmark, ref.get(benchmark, {}), sim[benchmark], args.out)
        benchmarks.append((benchmark, figs))
        for f in figs:
            rows.append({"benchmark": benchmark, "label": f["label"],
                         "mape": f["mape"], "smape": f["smape"], "n": f["n"],
                         "geomean": f["geomean"], "offset": f["offset"], "fig": f["fig"]})

    report_dir = os.path.dirname(os.path.abspath(args.report)) if args.report else os.path.abspath(args.out)
    global_md, gm = global_metrics(ref, sim, args.out, report_dir)

    summary = build_summary_table(rows)
    if gm:
        summary += (f"\n\n**Global:** Spearman ρ {gm['spearman']:.2f} · Kendall τ {gm['kendall']:.2f} · "
                    f"geomean k {gm['geomean_k']:.2f}× (ref) · geo-σ {gm['geo_std']:.2f}× "
                    f"(shape {gm['within_factor']:.2f}× / level {gm['between_factor']:.2f}×)")
    print("\n" + summary)
    if args.summary:
        with open(args.summary, "a") as f:
            f.write(summary + "\n")
    if args.report:
        with open(args.report, "w") as f:
            f.write(build_report_md(rows, args.report, global_md or "") + "\n")
        print(f"\nwrote report {args.report}")
    if args.json_out:
        data = {
            "schema": 1, "run_id": args.run_id, "run_url": args.run_url,
            "generated_at": args.generated_at, "metric": "kernel execution time (ms)",
            "overall_mape": overall(rows, "mape"), "overall_smape": overall(rows, "smape"),
            "global": gm,
            "benchmarks": [
                {"name": name,
                 "xlabel": figs[0]["xlabel"] if figs else "scaling factor",
                 "figures": [{k: f[k] for k in
                              ("label", "slug", "mape", "smape", "n", "offset",
                               "anchor_x", "sim", "real")} for f in figs]}
                for (name, figs) in benchmarks
            ],
        }
        with open(args.json_out, "w") as f:
            json.dump(data, f, indent=2)
        print(f"\nwrote json {args.json_out}")


if __name__ == "__main__":
    main()
