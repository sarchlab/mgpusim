#!/usr/bin/env python3
"""
Compare H100 real-hardware kernel time against mgpusim's simulated kernel
time/cycles for the same benchmark, to illustrate the gap between an
AMD-ISA-targeted simulator's prediction and real NVIDIA hardware behavior.

mgpusim (amd/samples/runner) always writes results to a sqlite3 file (name
set by -metric-file-name, default "metrics") in a table `mgpusim_metrics`
with columns (Location, What, Value, Unit). The row with
What == "kernel_time" and Location == "Driver" is the end-to-end simulated
kernel execution time in seconds.

Inputs:
  --h100-line  stdout line from run_matrixmultiplication, e.g.:
               "device=NVIDIA H100 80GB HBM3 X=64 Y=64 Z=64 iterations=20 avg_kernel_seconds=0.000123456"
  --sim-db     mgpusim sqlite3 metrics file (default: metrics.sqlite3)
  --freq-mhz   GPU clock frequency in MHz, used for both sides of the
               comparison: it should match both (a) the frequency you set
               in the mgpusim platform builder (e.g. r9nano's `freq` field)
               and (b) the real GPU's clock as reported by
               `nvidia-smi -q -d CLOCK` (e.g. 1755 for the H100 PCIe's SM
               clock). Required, since cycle counts are meaningless without
               a specific frequency to convert from/to seconds.
"""
import argparse
import re
import sqlite3
import sys


def parse_h100_line(line: str) -> float:
    m = re.search(r"avg_kernel_seconds=([0-9.eE+-]+)", line)
    if not m:
        raise ValueError(f"could not parse H100 timing from: {line!r}")
    return float(m.group(1))


def parse_sim_kernel_time(db_path: str) -> float:
    con = sqlite3.connect(db_path)
    try:
        row = con.execute(
            "SELECT Value FROM mgpusim_metrics WHERE What = 'kernel_time' "
            "AND Location = 'Driver' LIMIT 1"
        ).fetchone()
    finally:
        con.close()
    if row is None:
        raise ValueError(f"no Driver/kernel_time row found in {db_path}")
    return float(row[0])


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--h100-line", required=True)
    ap.add_argument("--sim-db", default="metrics.sqlite3")
    ap.add_argument("--freq-mhz", "--freq_mhz", dest="freq_mhz", type=float, required=True)
    args = ap.parse_args()

    freq_hz = args.freq_mhz * 1e6

    h100_seconds = parse_h100_line(args.h100_line)
    h100_cycles = h100_seconds * freq_hz

    sim_seconds = parse_sim_kernel_time(args.sim_db)
    sim_cycles = sim_seconds * freq_hz

    ratio = sim_seconds / h100_seconds if h100_seconds else float("inf")

    label_w = 48
    print(f"{'(1) GPU frequency (MHz)':<{label_w}}{args.freq_mhz:.1f}")
    print(f"{'(2) H100 kernel time (real)':<{label_w}}{h100_seconds:.9e} s")
    print(f"{'(3) H100 cycle count (calculated)':<{label_w}}{h100_cycles:.1f} cycles")
    print(f"{'(4) mgpusim simulated kernel time (real)':<{label_w}}{sim_seconds:.9e} s")
    print(f"{'(5) mgpusim simulated cycle count (calculated)':<{label_w}}{sim_cycles:.1f} cycles")
    print(f"{'(6) simulated-time / real-time ratio':<{label_w}}{ratio:.1f}x")


if __name__ == "__main__":
    sys.exit(main())
