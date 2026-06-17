#!/usr/bin/env python3
"""Compare MGPUSim (MI300A) microbenchmark results against real-hardware ground truth.

Reads the ground-truth summary CSV and the simulator sweep CSV, derives GFLOPS
for each simulated fp32_throughput point, and reports the ABSOLUTE error
(sim - real, in GFLOPS) per point plus the mean absolute error (MAE). Renders
the markdown report from report_template.md. Stdlib only.

  --ref        ground-truth summary CSV (summarize_ground_truth.py output)
  --sim        fp32_throughput simulator sweep CSV (run_sim_sweep.sh output)
  --cache-sim  cache_latency simulator sweep CSV (run_cache_latency_sweep.sh
               output); optional. Rendered sim-only (no ground truth yet).

Sim GFLOPS = num_blocks * threads_per_block * fmas_per_thread * 2 / kernel_time.
Points are matched on (num_blocks, threads_per_block, fmas_per_thread).

cache_latency has no committed ground truth yet, so its section is sim-only; it
renders an explanatory note when the sweep produced no successful points.
"""

import argparse
import csv
import os
from datetime import datetime, timezone


def read_ref(path):
    """(num_blocks, threads_per_block, fmas) -> real GFLOPS mean."""
    ref = {}
    with open(path, newline="") as f:
        for r in csv.DictReader(f):
            if r["benchmark"] != "fp32_throughput":
                continue
            key = (int(r["num_blocks"]), int(r["threads_per_block"]),
                   int(r["scaling_param_value"]))
            ref[key] = float(r["metric_mean"])
    return ref


def read_sim(path):
    with open(path, newline="") as f:
        return [r for r in csv.DictReader(f) if r["benchmark"] == "fp32_throughput"]


def read_cache_sim(path):
    """Rows from run_cache_latency_sweep.sh; [] if the file is missing/empty."""
    if not path or not os.path.exists(path):
        return []
    with open(path, newline="") as f:
        return [r for r in csv.DictReader(f) if r["benchmark"] == "cache_latency"]


def classify_level(array_bytes):
    """Match the HIP microbenchmark's cache-level buckets (by array size)."""
    kb = array_bytes / 1024.0
    if kb <= 16:
        return "L1"
    if kb <= 8192:
        return "L2"
    return "DRAM"


def render_cache_rows(cache_rows):
    """sim-only rows: array size, cache level, accesses, ns/access."""
    if not cache_rows:
        return ("| _no successful points — the cache_latency sweep produced no "
                "data (check the sweep job log)_ |  |  |  |  |")
    rows = []
    for r in sorted(cache_rows, key=lambda r: (int(r["num_accesses"]),
                                               int(r["array_bytes"]))):
        ab = int(r["array_bytes"])
        na = int(r["num_accesses"])
        ns = float(r["ns_per_access"])
        kib = ab / 1024.0
        size = f"{kib:,.0f} KB" if kib < 1024 else f"{kib / 1024:,.0f} MB"
        rows.append(f"| {size} | {classify_level(ab)} | {na} | "
                    f"{ns:,.2f} | {float(r['kernel_time_s']) * 1e3:,.4f} |")
    return "\n".join(rows)


def build_points(ref, sim_rows):
    points = []
    for r in sim_rows:
        nb = int(r["num_blocks"])
        tpb = int(r["threads_per_block"])
        fmas = int(r["fmas_per_thread"])
        kt = float(r["kernel_time_s"])
        sim = nb * tpb * fmas * 2.0 / kt / 1e9  # GFLOPS
        real = ref.get((nb, tpb, fmas))
        points.append({
            "num_blocks": nb, "tpb": tpb, "fmas": fmas, "sim": sim, "real": real,
            "abs_err": None if real is None else sim - real,
        })
    points.sort(key=lambda p: (p["num_blocks"], p["tpb"], p["fmas"]))
    return points


def aggregate(points):
    matched = [p for p in points if p["abs_err"] is not None]
    if not matched:
        return {"n": 0, "mae": None, "max_ae": None, "max_at": "—"}
    abs_errs = [abs(p["abs_err"]) for p in matched]
    worst = max(matched, key=lambda p: abs(p["abs_err"]))
    n = len(matched)
    return {
        "n": n,
        "mae": sum(abs_errs) / n,
        "max_ae": max(abs_errs),
        "max_at": (f"num_blocks={worst['num_blocks']}, "
                   f"tpb={worst['tpb']}, fmas={worst['fmas']}"),
    }


def g(v):
    return "—" if v is None else f"{v:,.2f}"


def render_rows(points):
    out = []
    for p in points:
        if p["abs_err"] is None:
            err, note = "—", "no ground-truth match"
        else:
            err = f"{p['abs_err']:+,.2f}"
            note = "sim high" if p["abs_err"] > 0 else "sim low"
        out.append(f"| {p['num_blocks']} | {p['tpb']} | {p['fmas']} | "
                   f"{g(p['real'])} | {g(p['sim'])} | {err} | {note} |")
    return "\n".join(out) if out else "| _no fp32 points_ |  |  |  |  |  |  |"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--ref", default="gpu_perf_scripts/calibration/mi300a_ground_truth.csv")
    ap.add_argument("--sim", default="sim_results.csv")
    ap.add_argument("--cache-sim", default="",
                    help="cache_latency sweep CSV (optional, sim-only)")
    ap.add_argument("--template",
                    default="gpu_perf_scripts/calibration/report_template.md")
    ap.add_argument("--out", default="calibration_report.md")
    args = ap.parse_args()

    if not os.path.exists(args.ref):
        raise SystemExit(f"ground-truth CSV not found: {args.ref}")

    points = build_points(read_ref(args.ref), read_sim(args.sim))
    agg = aggregate(points)
    cache_rows = read_cache_sim(args.cache_sim)

    with open(args.template) as f:
        report = f.read()

    repl = {
        "GENERATED_AT": datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M UTC"),
        "GIT_SHA": os.environ.get("GITHUB_SHA", "local")[:12],
        "SIM_CONFIG": "-timing -arch cdna3 -gpu mi300a",
        "REF": os.path.basename(args.ref),
        "FP32_MAE": g(agg["mae"]),
        "FP32_MAXAE": g(agg["max_ae"]),
        "FP32_MAXAE_AT": agg["max_at"],
        "FP32_N": str(agg["n"]),
        "SUMMARY_ROWS": (f"| fp32_throughput | GFLOPS | {agg['n']} | "
                         f"{g(agg['mae'])} | {g(agg['max_ae'])} |\n"
                         f"| cache_latency | ns/access | {len(cache_rows)} | "
                         f"— | — |"),
        "FP32_ROWS": render_rows(points),
        "CACHE_N": str(len(cache_rows)),
        "CACHE_ROWS": render_cache_rows(cache_rows),
    }
    for k, v in repl.items():
        report = report.replace("{{" + k + "}}", v)

    with open(args.out, "w") as f:
        f.write(report)

    print(f"fp32_throughput: n={agg['n']} MAE={g(agg['mae'])} GFLOPS "
          f"max|err|={g(agg['max_ae'])} GFLOPS")
    print(f"cache_latency: n={len(cache_rows)} sim points (sim-only)")
    print(f"wrote {args.out}")


if __name__ == "__main__":
    main()
