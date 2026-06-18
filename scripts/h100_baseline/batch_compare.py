#!/usr/bin/env python3
"""
Drive steps (2) and (3) of the H100-vs-mgpusim pipeline for every benchmark
listed in benchmarks.yaml, and print a summary table.

Step (1) (running each benchmark's harness on the H100 server) is external
to this script by design -- this script only runs the mgpusim side, since
running on the H100 must happen on that physical machine. Pass in the H100
results via --h100-results (a small text file, one line per benchmark, see
format below), produced by running each h100_harnesses/<name> binary on the
H100 server and saving its stdout line.

--h100-results format (one benchmark per line):
  matrixmultiplication: device=NVIDIA H100 80GB HBM3 X=64 Y=64 Z=64 iterations=20 avg_kernel_seconds=0.000123456

Usage:
  python3 batch_compare.py --h100-results h100_results.txt [--config benchmarks.yaml]
"""
import argparse
import re
import subprocess
import sys
from pathlib import Path

import yaml  # pip install pyyaml

from compare import parse_h100_line, parse_sim_kernel_time

SCRIPT_DIR = Path(__file__).resolve().parent
REPO_ROOT = SCRIPT_DIR.parent.parent


def load_h100_results(path: str) -> dict:
    results = {}
    for line in Path(path).read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        name, _, rest = line.partition(":")
        results[name.strip()] = parse_h100_line(rest.strip())
    return results


def run_sim(bench: dict) -> float:
    sample_dir = REPO_ROOT / "amd" / "samples" / bench["sample"]
    binary = f"/tmp/mgpusim_{bench['sample']}"
    subprocess.run(["go", "build", "-o", binary, "."], cwd=sample_dir, check=True)

    db_name = f"{bench['name']}_metrics"
    subprocess.run(
        [binary, "-timing", "-report-all", "-disable-rtm", "-metric-file-name", db_name]
        + bench.get("sim_args", []),
        cwd=sample_dir,
        check=True,
    )
    db_path = sample_dir / f"{db_name}.sqlite3"
    sim_seconds = parse_sim_kernel_time(str(db_path))
    db_path.unlink()
    return sim_seconds


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--h100-results", required=True)
    ap.add_argument("--config", default=str(SCRIPT_DIR / "benchmarks.yaml"))
    ap.add_argument("--freq-hz", type=float, default=1e9)
    args = ap.parse_args()

    config = yaml.safe_load(Path(args.config).read_text())
    h100_results = load_h100_results(args.h100_results)

    print(f"{'benchmark':<24}{'H100 (s)':>14}{'mgpusim (s)':>14}{'cycles':>12}{'ratio':>10}")
    for bench in config["benchmarks"]:
        name = bench["name"]
        if name not in h100_results:
            print(f"{name:<24} (skipped: no H100 result provided)")
            continue
        h100_seconds = h100_results[name]
        sim_seconds = run_sim(bench)
        sim_cycles = sim_seconds * args.freq_hz
        ratio = sim_seconds / h100_seconds if h100_seconds else float("inf")
        print(f"{name:<24}{h100_seconds:>14.3e}{sim_seconds:>14.3e}{sim_cycles:>12.0f}{ratio:>9.1f}x")


if __name__ == "__main__":
    sys.exit(main())
