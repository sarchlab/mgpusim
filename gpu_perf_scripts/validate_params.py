#!/usr/bin/env python3
"""
validate_params.py — Compare microbenchmark CSV results against hardcoded
                     simulator parameters from mi300a/builder.go.

Reads one or more microbenchmark CSV outputs and compares measured values
against known simulator configuration values.

Usage:
    python validate_params.py \
        --cache-latency cache_results.csv \
        --tlb tlb_results.csv \
        --membw membw_results.csv \
        --launch launch_results.csv

All arguments are optional — the script handles whichever CSVs are provided.

Exit code: 0 if all checks pass, 1 if any check fails.
"""

import argparse
import csv
import sys
from typing import Optional

# ── Simulator Parameters (from mi300a/builder.go) ───────────────────────────

SIM_PARAMS = {
    # Clock
    "gpu_freq_mhz": 1700,

    # L1V Cache
    "l1v_cache_size_kb_per_cu": 32,
    "l1v_bank_latency_cycles": 7,

    # L2 Cache
    "l2_cache_size_mb_total": 32,
    "l2_bank_latency_cycles": 6,

    # DRAM
    "dram_freq_mhz": 1000,
    "dram_pipeline_depth": 5,
    "dram_row_miss_delay_cycles": 52,

    # Kernel launch overhead (cycles)
    "kernel_launch_first_cycles": 5400,
    "kernel_launch_subsequent_cycles": 1800,

    # L1V TLB
    "l1v_tlb_sets": 4,
    "l1v_tlb_ways": 64,
    "l1v_tlb_entries": 256,       # 4 × 64
    "l1v_tlb_coverage_kb": 1024,  # 256 × 4 KB = 1 MB

    # L2 TLB
    "l2_tlb_sets": 64,
    "l2_tlb_ways": 64,
    "l2_tlb_entries": 4096,        # 64 × 64
    "l2_tlb_coverage_kb": 16384,   # 4096 × 4 KB = 16 MB

    # Page size
    "log2_page_size": 12,
    "page_size_kb": 4,

    # CU count
    "num_shader_arrays": 20,
    "cus_per_shader_array": 6,
    "num_cus": 120,
}

GPU_FREQ_GHZ = SIM_PARAMS["gpu_freq_mhz"] / 1000.0  # 1.7 GHz

# ── Tolerance ────────────────────────────────────────────────────────────────

# A measured value is considered PASS if:
#   ratio = measured / expected
#   RATIO_LO <= ratio <= RATIO_HI
RATIO_LO = 0.5
RATIO_HI = 2.0


# ── Result container ────────────────────────────────────────────────────────

class CheckResult:
    """One comparison result between measured and expected."""

    def __init__(self, category: str, parameter: str, measured, expected,
                 unit: str = "", note: str = ""):
        self.category = category
        self.parameter = parameter
        self.measured = measured
        self.expected = expected
        self.unit = unit
        self.note = note
        if expected and expected != 0:
            self.ratio = measured / expected
            self.diff = measured - expected
            self.passed = RATIO_LO <= self.ratio <= RATIO_HI
        else:
            self.ratio = None
            self.diff = None
            self.passed = False


# ── CSV parsers ──────────────────────────────────────────────────────────────

def _read_csv_skip_comments(path: str) -> list[dict]:
    """Read a CSV file, skipping lines starting with '#'."""
    rows = []
    with open(path, newline="") as f:
        lines = [line for line in f if not line.startswith("#")]
    reader = csv.DictReader(lines)
    for row in reader:
        rows.append(row)
    return rows


def validate_cache_latency(path: str) -> list[CheckResult]:
    """
    Parse micro_cache_latency CSV.

    CSV columns: level,array_size_kb,iterations,avg_ns,estimated_cycles

    Compare:
      - L1 measured latency (cycles) vs sim L1V bank latency
      - L2 measured latency (cycles) vs sim L2 bank latency
    """
    rows = _read_csv_skip_comments(path)
    results = []

    # Aggregate by level
    level_cycles: dict[str, list[float]] = {}
    for row in rows:
        level = row["level"].strip()
        cycles = float(row["estimated_cycles"])
        level_cycles.setdefault(level, []).append(cycles)

    for level, cycle_list in level_cycles.items():
        avg_cycles = sum(cycle_list) / len(cycle_list)

        if level.upper() == "L1":
            results.append(CheckResult(
                category="Cache Latency",
                parameter="L1 latency (cycles)",
                measured=avg_cycles,
                expected=SIM_PARAMS["l1v_bank_latency_cycles"],
                unit="cycles",
                note=f"avg over {len(cycle_list)} array sizes",
            ))
        elif level.upper() == "L2":
            results.append(CheckResult(
                category="Cache Latency",
                parameter="L2 latency (cycles)",
                measured=avg_cycles,
                expected=SIM_PARAMS["l2_bank_latency_cycles"],
                unit="cycles",
                note=f"avg over {len(cycle_list)} array sizes",
            ))
        elif level.upper() == "DRAM":
            # Compare DRAM latency against row-miss delay
            results.append(CheckResult(
                category="Cache Latency",
                parameter="DRAM latency (cycles)",
                measured=avg_cycles,
                expected=SIM_PARAMS["dram_row_miss_delay_cycles"],
                unit="cycles",
                note=f"avg over {len(cycle_list)} array sizes; compare to row-miss delay",
            ))

    return results


def validate_tlb(path: str) -> list[CheckResult]:
    """
    Parse micro_tlb CSV.

    CSV columns: test_name,num_pages,stride_kb,iterations,avg_ns_per_access,estimated_cycles

    Compare TLB hit/miss latency patterns against sim TLB parameters.
    """
    rows = _read_csv_skip_comments(path)
    results = []

    # Group by test_name
    test_data: dict[str, list[dict]] = {}
    for row in rows:
        name = row["test_name"].strip()
        test_data.setdefault(name, []).append(row)

    # Extract TLB-hit baseline vs L1-miss and L2-miss latencies
    baseline_cycles = None
    l1_miss_cycles = None
    l2_miss_cycles = None

    for name, rows_for_test in test_data.items():
        avg_cycles = sum(float(r["estimated_cycles"]) for r in rows_for_test) / len(rows_for_test)

        if "hit" in name.lower() or "baseline" in name.lower():
            baseline_cycles = avg_cycles
        elif "l1" in name.lower() and "miss" in name.lower():
            l1_miss_cycles = avg_cycles
        elif "l2" in name.lower() and "miss" in name.lower():
            l2_miss_cycles = avg_cycles

    # Report TLB miss penalties
    if baseline_cycles is not None and l1_miss_cycles is not None:
        l1_tlb_miss_penalty = l1_miss_cycles - baseline_cycles
        results.append(CheckResult(
            category="TLB",
            parameter="L1 TLB miss penalty (cycles)",
            measured=l1_tlb_miss_penalty,
            expected=SIM_PARAMS["l2_bank_latency_cycles"],  # L2 cache serves the page walk
            unit="cycles",
            note="L1 TLB miss cycles minus TLB-hit baseline",
        ))

    if baseline_cycles is not None and l2_miss_cycles is not None:
        l2_tlb_miss_penalty = l2_miss_cycles - baseline_cycles
        results.append(CheckResult(
            category="TLB",
            parameter="L2 TLB miss penalty (cycles)",
            measured=l2_tlb_miss_penalty,
            expected=SIM_PARAMS["dram_row_miss_delay_cycles"],
            unit="cycles",
            note="L2 TLB miss cycles minus TLB-hit baseline",
        ))

    # Report raw latency values too
    if baseline_cycles is not None:
        results.append(CheckResult(
            category="TLB",
            parameter="TLB-hit access latency (cycles)",
            measured=baseline_cycles,
            expected=SIM_PARAMS["l1v_bank_latency_cycles"],
            unit="cycles",
            note="Baseline with TLB hits (should be ~L1 cache latency)",
        ))

    return results


def validate_membw(path: str) -> list[CheckResult]:
    """
    Parse micro_membw CSV.

    CSV columns: operation,array_size,iterations,avg_ms,min_ms,max_ms,avg_gbps

    Compare measured bandwidth against theoretical DRAM bandwidth.
    """
    rows = _read_csv_skip_comments(path)
    results = []

    # Group by operation
    op_bw: dict[str, list[float]] = {}
    for row in rows:
        op = row["operation"].strip()
        bw = float(row["avg_gbps"])
        op_bw.setdefault(op, []).append(bw)

    # Theoretical peak bandwidth for MI300A:
    # DRAM freq 1 GHz, 8 channels, 128-bit per channel ≈ 128 GB/s (simplified)
    # The simulator's effective BW depends on many factors; use a rough
    # estimate from DRAM freq and pipeline depth.
    # More accurate: MI300A HBM3 spec = 5.3 TB/s (8 stacks), but simulator
    # models a simplified memory subsystem.
    dram_freq_ghz = SIM_PARAMS["dram_freq_mhz"] / 1000.0

    for op, bw_list in op_bw.items():
        peak_bw = max(bw_list)
        avg_bw = sum(bw_list) / len(bw_list)

        results.append(CheckResult(
            category="Memory BW",
            parameter=f"{op} peak BW (GB/s)",
            measured=peak_bw,
            expected=dram_freq_ghz * 128,  # rough estimate
            unit="GB/s",
            note=f"best of {len(bw_list)} sizes; DRAM freq={SIM_PARAMS['dram_freq_mhz']}MHz",
        ))

    return results


def validate_launch(path: str) -> list[CheckResult]:
    """
    Parse micro_launch CSV.

    CSV columns: test,launches,iterations,total_avg_ms,per_launch_avg_us

    Compare measured launch overhead against sim kernel launch overhead.
    """
    rows = _read_csv_skip_comments(path)
    results = []

    for row in rows:
        test_name = row["test"].strip()
        per_launch_us = float(row["per_launch_avg_us"])

        # Convert to cycles: per_launch_us * gpu_freq_ghz * 1000
        # (1 µs × 1.7 GHz = 1700 cycles)
        per_launch_cycles = per_launch_us * GPU_FREQ_GHZ * 1000.0

        # For batch_async, the first kernel has higher overhead
        if "batch" in test_name.lower() or "async" in test_name.lower():
            expected_cycles = SIM_PARAMS["kernel_launch_subsequent_cycles"]
            label = "subsequent"
        elif "sync" in test_name.lower() and "small" not in test_name.lower():
            expected_cycles = SIM_PARAMS["kernel_launch_subsequent_cycles"]
            label = "sync-per-launch"
        else:
            expected_cycles = SIM_PARAMS["kernel_launch_first_cycles"]
            label = "first/cold"

        results.append(CheckResult(
            category="Launch Overhead",
            parameter=f"{test_name} per-launch overhead (cycles)",
            measured=per_launch_cycles,
            expected=expected_cycles,
            unit="cycles",
            note=f"measured {per_launch_us:.2f} µs; expected ~{label}",
        ))

    return results


# ── Reporting ────────────────────────────────────────────────────────────────

def print_sim_params():
    """Print the reference simulator parameters."""
    print("=" * 78)
    print("SIMULATOR REFERENCE PARAMETERS (mi300a/builder.go)")
    print("=" * 78)
    param_groups = [
        ("Clock", [
            ("GPU Frequency", f"{SIM_PARAMS['gpu_freq_mhz']} MHz"),
        ]),
        ("L1V Cache", [
            ("Size per CU", f"{SIM_PARAMS['l1v_cache_size_kb_per_cu']} KB"),
            ("Bank latency", f"{SIM_PARAMS['l1v_bank_latency_cycles']} cycles"),
        ]),
        ("L2 Cache", [
            ("Total size", f"{SIM_PARAMS['l2_cache_size_mb_total']} MB"),
            ("Bank latency", f"{SIM_PARAMS['l2_bank_latency_cycles']} cycles"),
        ]),
        ("DRAM", [
            ("Frequency", f"{SIM_PARAMS['dram_freq_mhz']} MHz"),
            ("Pipeline depth", f"{SIM_PARAMS['dram_pipeline_depth']}"),
            ("Row miss delay", f"{SIM_PARAMS['dram_row_miss_delay_cycles']} cycles"),
        ]),
        ("Kernel Launch", [
            ("First launch", f"{SIM_PARAMS['kernel_launch_first_cycles']} cycles"),
            ("Subsequent", f"{SIM_PARAMS['kernel_launch_subsequent_cycles']} cycles"),
        ]),
        ("TLB", [
            ("L1 TLB", f"{SIM_PARAMS['l1v_tlb_sets']}×{SIM_PARAMS['l1v_tlb_ways']}="
                        f"{SIM_PARAMS['l1v_tlb_entries']} entries "
                        f"({SIM_PARAMS['l1v_tlb_coverage_kb']} KB coverage)"),
            ("L2 TLB", f"{SIM_PARAMS['l2_tlb_sets']}×{SIM_PARAMS['l2_tlb_ways']}="
                        f"{SIM_PARAMS['l2_tlb_entries']} entries "
                        f"({SIM_PARAMS['l2_tlb_coverage_kb']} KB coverage)"),
            ("Page size", f"{SIM_PARAMS['page_size_kb']} KB "
                          f"(log2={SIM_PARAMS['log2_page_size']})"),
        ]),
        ("Compute", [
            ("Shader arrays", f"{SIM_PARAMS['num_shader_arrays']}"),
            ("CUs per SA", f"{SIM_PARAMS['cus_per_shader_array']}"),
            ("Total CUs", f"{SIM_PARAMS['num_cus']}"),
        ]),
    ]
    for group_name, params in param_groups:
        print(f"\n  {group_name}:")
        for name, value in params:
            print(f"    {name:25s} {value}")
    print()


def print_results(results: list[CheckResult]) -> bool:
    """Print a summary table.  Returns True if all passed."""
    if not results:
        print("  (no results)\n")
        return True

    # Column widths
    cat_w = max(len(r.category) for r in results)
    par_w = max(len(r.parameter) for r in results)

    hdr = (f"  {'Category':<{cat_w}}  {'Parameter':<{par_w}}  "
           f"{'Measured':>12}  {'Expected':>12}  {'Ratio':>8}  "
           f"{'Diff':>12}  {'Status':>6}  Note")
    sep = "  " + "-" * (len(hdr) - 2)

    print(sep)
    print(hdr)
    print(sep)

    all_pass = True
    for r in results:
        status = "PASS" if r.passed else "FAIL"
        if not r.passed:
            all_pass = False

        ratio_str = f"{r.ratio:.3f}" if r.ratio is not None else "N/A"
        diff_str = f"{r.diff:+.2f}" if r.diff is not None else "N/A"
        meas_str = f"{r.measured:.2f}" if isinstance(r.measured, float) else str(r.measured)
        exp_str = f"{r.expected:.2f}" if isinstance(r.expected, float) else str(r.expected)

        print(f"  {r.category:<{cat_w}}  {r.parameter:<{par_w}}  "
              f"{meas_str:>12}  {exp_str:>12}  {ratio_str:>8}  "
              f"{diff_str:>12}  {status:>6}  {r.note}")

    print(sep)
    return all_pass


# ── Main ─────────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(
        description="Compare microbenchmark CSV results against simulator parameters.",
    )
    parser.add_argument("--cache-latency", metavar="CSV",
                        help="micro_cache_latency output CSV")
    parser.add_argument("--tlb", metavar="CSV",
                        help="micro_tlb output CSV")
    parser.add_argument("--membw", metavar="CSV",
                        help="micro_membw output CSV")
    parser.add_argument("--launch", metavar="CSV",
                        help="micro_launch output CSV")
    args = parser.parse_args()

    if not any([args.cache_latency, args.tlb, args.membw, args.launch]):
        parser.print_help()
        print("\nError: provide at least one CSV file.", file=sys.stderr)
        sys.exit(2)

    print_sim_params()

    all_results: list[CheckResult] = []

    if args.cache_latency:
        print(f"── Cache Latency: {args.cache_latency}")
        res = validate_cache_latency(args.cache_latency)
        all_results.extend(res)

    if args.tlb:
        print(f"── TLB: {args.tlb}")
        res = validate_tlb(args.tlb)
        all_results.extend(res)

    if args.membw:
        print(f"── Memory Bandwidth: {args.membw}")
        res = validate_membw(args.membw)
        all_results.extend(res)

    if args.launch:
        print(f"── Launch Overhead: {args.launch}")
        res = validate_launch(args.launch)
        all_results.extend(res)

    print()
    print("=" * 78)
    print("VALIDATION RESULTS")
    print("=" * 78)
    print(f"  Acceptable ratio range: [{RATIO_LO:.1f}x, {RATIO_HI:.1f}x]")
    print()

    all_pass = print_results(all_results)

    n_pass = sum(1 for r in all_results if r.passed)
    n_fail = sum(1 for r in all_results if not r.passed)
    n_total = len(all_results)

    print()
    print(f"  Total checks: {n_total}  |  PASS: {n_pass}  |  FAIL: {n_fail}")
    if all_pass:
        print("  ✅ ALL CHECKS PASSED")
    else:
        print("  ❌ SOME CHECKS FAILED")
    print()

    sys.exit(0 if all_pass else 1)


if __name__ == "__main__":
    main()
