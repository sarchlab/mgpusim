#!/usr/bin/env python3
"""Plot per-configuration execution-time curves: simulation vs real hardware.

One figure per non-scaling-parameter combination (num_blocks, threads_per_block).
X axis = scaling factor (fmas_per_thread), Y axis = execution time (ms). Each
figure has two lines: the simulator and the real MI300A hardware.

  --ref  ground-truth summary CSV (provides kernel_ms_mean)
  --sim  simulator sweep CSV (provides kernel_time_s)
  --out  output directory for the PNGs

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


def read_ref(path):
    """(num_blocks, threads_per_block, fmas) -> real kernel time (ms)."""
    out = {}
    with open(path, newline="") as f:
        for r in csv.DictReader(f):
            if r["benchmark"] != "fp32_throughput":
                continue
            if r["num_blocks"] == "" or r["threads_per_block"] == "":
                continue
            try:
                key = (int(r["num_blocks"]), int(r["threads_per_block"]),
                       int(r["scaling_param_value"]))
                out[key] = float(r["kernel_ms_mean"])
            except ValueError:
                continue
    return out


def read_sim(path):
    """(num_blocks, threads_per_block) -> sorted [(fmas, sim_ms)]."""
    groups = defaultdict(list)
    with open(path, newline="") as f:
        for r in csv.DictReader(f):
            if r["benchmark"] != "fp32_throughput":
                continue
            nb = int(r["num_blocks"])
            tpb = int(r["threads_per_block"])
            fmas = int(r["fmas_per_thread"])
            sim_ms = float(r["kernel_time_s"]) * 1000.0
            groups[(nb, tpb)].append((fmas, sim_ms))
    for k in groups:
        groups[k].sort()
    return groups


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--ref", default="gpu_perf_scripts/calibration/mi300a_ground_truth.csv")
    ap.add_argument("--sim", default="sim_results.csv")
    ap.add_argument("--out", default="figures")
    args = ap.parse_args()

    os.makedirs(args.out, exist_ok=True)
    ref = read_ref(args.ref)
    sim_groups = read_sim(args.sim)

    written = 0
    for (nb, tpb), pts in sorted(sim_groups.items()):
        fmas = [p[0] for p in pts]
        sim_ms = [p[1] for p in pts]
        real = [(f, ref[(nb, tpb, f)]) for f in fmas if (nb, tpb, f) in ref]

        fig, ax = plt.subplots(figsize=(7, 5))
        ax.plot(fmas, sim_ms, marker="o", color=SIM_COLOR, label="simulation")
        if real:
            ax.plot([f for f, _ in real], [m for _, m in real],
                    marker="s", color=REAL_COLOR, label="real hardware")
        ax.set_xscale("log", base=2)
        ax.set_yscale("log")
        ax.set_xlabel("fmas_per_thread (scaling factor)")
        ax.set_ylabel("execution time (ms)")
        ax.set_title(f"fp32_throughput\nnum_blocks={nb}, threads_per_block={tpb}")
        ax.legend()
        fig.tight_layout()
        path = os.path.join(args.out, f"fp32_nb{nb}_tpb{tpb}.png")
        fig.savefig(path, dpi=120)
        plt.close(fig)
        written += 1
        print(f"wrote {path}  ({len(fmas)} sim, {len(real)} real)")

    print(f"wrote {written} figures to {args.out}/")


if __name__ == "__main__":
    main()
