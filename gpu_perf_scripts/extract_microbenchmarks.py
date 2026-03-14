#!/usr/bin/env python3
"""
Extract microbenchmark comparison metrics from CI comparison CSV.

Reads comparison_ci.csv and extracts three key measurements:
  1. Kernel Launch Overhead (vectoradd/relu at smallest sizes)
  2. Per-Kernel Overhead (bitonicsort at small sizes)
  3. Memory Throughput / Effective Bandwidth (vectoradd at large sizes)

Usage:
    python extract_microbenchmarks.py [CSV_PATH] [--output FILE]

Examples:
    python extract_microbenchmarks.py comparison_ci.csv
    python extract_microbenchmarks.py /tmp/latest_ci/benchmark-comparison/comparison_ci.csv --output report.txt
"""

import argparse
import csv
import math
import sys
from typing import Optional


CLOCK_GHZ = 1.8  # MI300A clock speed in GHz


def read_csv(path: str) -> list[dict]:
    """Read comparison CSV and return list of row dicts."""
    rows = []
    with open(path, newline="") as f:
        reader = csv.DictReader(f)
        for row in reader:
            rows.append(row)
    return rows


def safe_float(val: str) -> Optional[float]:
    """Convert string to float, returning None for empty strings."""
    if val is None or val.strip() == "":
        return None
    return float(val)


def extract_launch_overhead(rows: list[dict]) -> list[dict]:
    """
    Extract kernel launch overhead from vectoradd and relu at size 1024.

    The simulation time at the smallest problem size approximates the
    launch overhead floor, since computation is negligible.
    """
    results = []
    for kernel in ["vectoradd", "relu"]:
        for row in rows:
            if row["kernel_name"] == kernel and row["problem_size"] == "1024":
                real_ms = safe_float(row["real_ms"])
                sim_ms = safe_float(row["sim_ms"])
                if real_ms is not None and sim_ms is not None:
                    results.append({
                        "kernel": kernel,
                        "size": 1024,
                        "real_ms": real_ms,
                        "sim_ms": sim_ms,
                        "ratio": sim_ms / real_ms,
                        "sim_cycles": sim_ms * CLOCK_GHZ * 1e6,
                        "real_cycles": real_ms * CLOCK_GHZ * 1e6,
                    })
    return results


def extract_per_kernel_overhead(rows: list[dict]) -> list[dict]:
    """
    Extract per-kernel overhead from bitonicsort at small sizes (1024-4096).

    Bitonicsort launches O(log^2 N) kernels. At small sizes, the per-kernel
    dispatch overhead dominates total runtime.
    """
    results = []
    target_sizes = {"1024", "2048", "4096"}
    for row in rows:
        if row["kernel_name"] == "bitonicsort" and row["problem_size"] in target_sizes:
            real_ms = safe_float(row["real_ms"])
            sim_ms = safe_float(row["sim_ms"])
            if real_ms is not None and sim_ms is not None:
                n = int(row["problem_size"])
                log_n = int(math.log2(n))
                # bitonicsort: log2(N) * (log2(N) + 1) / 2 kernel launches
                num_kernels = log_n * (log_n + 1) // 2
                results.append({
                    "size": n,
                    "real_ms": real_ms,
                    "sim_ms": sim_ms,
                    "ratio": sim_ms / real_ms,
                    "num_kernels": num_kernels,
                    "per_kernel_real_us": (real_ms * 1000) / num_kernels,
                    "per_kernel_sim_us": (sim_ms * 1000) / num_kernels,
                })
    results.sort(key=lambda x: x["size"])
    return results


def extract_memory_bandwidth(rows: list[dict]) -> list[dict]:
    """
    Extract effective memory bandwidth from vectoradd at large sizes.

    vectoradd: C[i] = A[i] + B[i]
    Data moved = 3 arrays * N * 4 bytes (2 reads + 1 write, float32).
    Effective BW = data_bytes / time_seconds.
    """
    results = []
    for row in rows:
        if row["kernel_name"] == "vectoradd":
            real_ms = safe_float(row["real_ms"])
            sim_ms = safe_float(row["sim_ms"])
            try:
                n = int(row["problem_size"])
            except ValueError:
                continue
            if n >= 1048576 and real_ms is not None and sim_ms is not None:
                data_bytes = n * 12  # 3 arrays * 4 bytes
                data_mb = data_bytes / (1024 * 1024)
                real_bw_gbs = (data_bytes / (real_ms / 1000)) / 1e9
                sim_bw_gbs = (data_bytes / (sim_ms / 1000)) / 1e9
                results.append({
                    "size": n,
                    "data_mb": data_mb,
                    "real_ms": real_ms,
                    "sim_ms": sim_ms,
                    "real_bw_gbs": real_bw_gbs,
                    "sim_bw_gbs": sim_bw_gbs,
                    "bw_ratio": real_bw_gbs / sim_bw_gbs,
                })
    results.sort(key=lambda x: x["size"])
    return results


def format_report(
    launch: list[dict],
    per_kernel: list[dict],
    bandwidth: list[dict],
) -> str:
    """Format all extracted metrics into a readable report."""
    lines = []
    lines.append("=" * 78)
    lines.append("MICROBENCHMARK COMPARISON: Simulator vs Real Hardware")
    lines.append("=" * 78)
    lines.append("")

    # Section 1: Launch overhead
    lines.append("1. KERNEL LAUNCH OVERHEAD")
    lines.append("-" * 78)
    lines.append(f"{'Kernel':<12} {'Size':>6} {'Real (ms)':>10} {'Sim (ms)':>10} "
                 f"{'Ratio':>7} {'Sim Cycles':>12} {'Real Cycles':>12}")
    lines.append("-" * 78)
    for r in launch:
        lines.append(
            f"{r['kernel']:<12} {r['size']:>6} {r['real_ms']:>10.4f} {r['sim_ms']:>10.4f} "
            f"{r['ratio']:>7.2f}x {r['sim_cycles']:>12,.0f} {r['real_cycles']:>12,.0f}"
        )
    if launch:
        avg_sim = sum(r["sim_ms"] for r in launch) / len(launch)
        avg_real = sum(r["real_ms"] for r in launch) / len(launch)
        lines.append("")
        lines.append(f"  Average launch overhead: Sim={avg_sim:.4f} ms "
                     f"({avg_sim * CLOCK_GHZ * 1e6:,.0f} cycles), "
                     f"Real={avg_real:.4f} ms ({avg_real * CLOCK_GHZ * 1e6:,.0f} cycles)")
    lines.append("")

    # Section 2: Per-kernel overhead
    lines.append("2. PER-KERNEL OVERHEAD (bitonicsort)")
    lines.append("-" * 78)
    lines.append(f"{'Size':>8} {'Real (ms)':>10} {'Sim (ms)':>10} {'Ratio':>7} "
                 f"{'Kernels':>8} {'Per-K Real (µs)':>16} {'Per-K Sim (µs)':>16}")
    lines.append("-" * 78)
    for r in per_kernel:
        lines.append(
            f"{r['size']:>8} {r['real_ms']:>10.4f} {r['sim_ms']:>10.4f} {r['ratio']:>7.2f}x "
            f"{r['num_kernels']:>8} {r['per_kernel_real_us']:>16.2f} {r['per_kernel_sim_us']:>16.2f}"
        )
    if per_kernel:
        avg_pk_sim = sum(r["per_kernel_sim_us"] for r in per_kernel) / len(per_kernel)
        avg_pk_real = sum(r["per_kernel_real_us"] for r in per_kernel) / len(per_kernel)
        lines.append("")
        lines.append(f"  Average per-kernel overhead: Sim={avg_pk_sim:.2f} µs, "
                     f"Real={avg_pk_real:.2f} µs")
    lines.append("")

    # Section 3: Bandwidth
    lines.append("3. MEMORY THROUGHPUT / EFFECTIVE BANDWIDTH (vectoradd)")
    lines.append("-" * 78)
    lines.append(f"{'Size':>12} {'Data (MB)':>10} {'Real (ms)':>10} {'Sim (ms)':>10} "
                 f"{'Real BW':>10} {'Sim BW':>10} {'Ratio':>7}")
    lines.append(f"{'':>12} {'':>10} {'':>10} {'':>10} "
                 f"{'(GB/s)':>10} {'(GB/s)':>10} {'(R/S)':>7}")
    lines.append("-" * 78)
    for r in bandwidth:
        lines.append(
            f"{r['size']:>12,} {r['data_mb']:>10.2f} {r['real_ms']:>10.4f} {r['sim_ms']:>10.4f} "
            f"{r['real_bw_gbs']:>10.1f} {r['sim_bw_gbs']:>10.1f} {r['bw_ratio']:>7.2f}x"
        )
    lines.append("")

    # Section 4: Summary
    lines.append("4. SUMMARY")
    lines.append("=" * 78)
    lines.append(f"{'Parameter':<30} {'Sim Value':<20} {'Real Value':<20} "
                 f"{'Ratio':>7} {'Notes'}")
    lines.append("-" * 78)

    if launch:
        avg_sim = sum(r["sim_ms"] for r in launch) / len(launch)
        avg_real = sum(r["real_ms"] for r in launch) / len(launch)
        sim_cyc = avg_sim * CLOCK_GHZ * 1e6
        real_cyc = avg_real * CLOCK_GHZ * 1e6
        lines.append(
            f"{'Kernel launch overhead':<30} "
            f"{f'{avg_sim:.4f} ms ({sim_cyc:,.0f}c)':<20} "
            f"{f'{avg_real:.4f} ms ({real_cyc:,.0f}c)':<20} "
            f"{avg_sim / avg_real:>6.2f}x "
            f"vectoradd/relu @ 1024"
        )

    if per_kernel:
        avg_pk_sim = sum(r["per_kernel_sim_us"] for r in per_kernel) / len(per_kernel)
        avg_pk_real = sum(r["per_kernel_real_us"] for r in per_kernel) / len(per_kernel)
        lines.append(
            f"{'Per-kernel overhead':<30} "
            f"{f'{avg_pk_sim:.2f} µs':<20} "
            f"{f'{avg_pk_real:.2f} µs':<20} "
            f"{avg_pk_sim / avg_pk_real:>6.2f}x "
            f"bitonicsort 1024-4096"
        )

    if bandwidth:
        r0 = bandwidth[0]
        sim_bw = r0["sim_bw_gbs"]
        real_bw = r0["real_bw_gbs"]
        bw_size = r0["size"]
        lines.append(
            f"{'Effective bandwidth':<30} "
            f"{f'{sim_bw:.1f} GB/s':<20} "
            f"{f'{real_bw:.1f} GB/s':<20} "
            f"{sim_bw / real_bw:>6.2f}x "
            f"vectoradd N={bw_size:,}"
        )

    lines.append("=" * 78)
    lines.append("")
    return "\n".join(lines)


def main():
    parser = argparse.ArgumentParser(
        description="Extract microbenchmark comparison metrics from CI CSV data."
    )
    parser.add_argument(
        "csv_path",
        nargs="?",
        default="comparison_ci.csv",
        help="Path to comparison_ci.csv (default: comparison_ci.csv)",
    )
    parser.add_argument(
        "--output", "-o",
        help="Write output to file instead of stdout",
    )
    args = parser.parse_args()

    try:
        rows = read_csv(args.csv_path)
    except FileNotFoundError:
        print(f"Error: CSV file not found: {args.csv_path}", file=sys.stderr)
        sys.exit(1)

    launch = extract_launch_overhead(rows)
    per_kernel = extract_per_kernel_overhead(rows)
    bandwidth = extract_memory_bandwidth(rows)

    if not launch and not per_kernel and not bandwidth:
        print("Warning: No matching data found in CSV.", file=sys.stderr)

    report = format_report(launch, per_kernel, bandwidth)

    if args.output:
        with open(args.output, "w") as f:
            f.write(report)
        print(f"Report written to {args.output}")
    else:
        print(report)


if __name__ == "__main__":
    main()
