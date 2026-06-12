#!/usr/bin/env python3
"""
merge_ci_results.py — Merge CI benchmark results into comparison_ci.csv.

Reads individual benchmark result CSVs from CI artifacts and updates
comparison_ci.csv with sim_ms, abs_error_ms, and rel_error columns.

Usage:
    python3 merge_ci_results.py --artifacts-dir /tmp/ci-results \
        --comparison benchmark-comparison/comparison_ci.csv \
        --output benchmark-comparison/comparison_ci.csv
"""

import argparse
import csv
import os
import sys
from collections import defaultdict


# Mapping from CI benchmark directory name to the kernel_name used in
# the individual results CSV. Most benchmarks output their own name as
# kernel_name, but some differ.
BENCH_DIR_TO_SIM_KERNEL = {
    # Polybench (CamelCase in sim output)
    "2dconvolution": "2DConvolution",
    "3dconvolution": "3DConvolution",
    # FFT
    "fft": "fft1D_512",
    # Parboil: directory names differ from HW/sim kernel names
    "parboil_cutcp": "cutoff_potential",
    "parboil_sgemm": "sgemm_tiled",
    "parboil_histo": "histogram",
    "parboil_lbm": "lbm_stream_collide",
    "parboil_mri_gridding": "gridding_kernel",
    "parboil_mri_q": "ComputeQ",
    "parboil_sad": "ComputeSAD",
    "parboil_spmv": "spmv_csr",
    "parboil_stencil": "stencil_kernel",
    "parboil_tpacf": "tpacf_DD",
    # SHOC: shoc_gemm outputs "gemm" as kernel_name
    "shoc_gemm": "gemm",
    # SpMV standalone
    "spmv": "spmv_csr_scalar",
    # Rodinia: some kernel names differ from directory
    "backprop": "layerforward",
    "backprop_adjust_weights": "adjust_weights",
    "streamcluster": "streamcluster_pgain",
    "bitonicsort": "sort",
    # SHOC: directory names differ from HW kernel names
    "bus_speed_download": "BusSpeedDownload",
    "bus_speed_readback": "BusSpeedReadback",
    "device_memory_read": "DeviceMemory_Read",
    "device_memory_write": "DeviceMemory_Write",
    "max_flops": "MaxFlops",
    # SHOC: md_lj outputs "md" as kernel_name
    "md_lj": "md",
    # Parboil: FindMinSAD extracted from parboil_sad per-kernel timing
    "findminsad": "FindMinSAD",
}

# Mapping from comparison_ci.csv kernel_name to the CI benchmark directory.
# This maps the CSV kernel names to which benchmark directory's results to use.
# All keys are **lowercase** for case-insensitive lookup.
CSV_KERNEL_TO_BENCH_DIR = {
    # Polybench CamelCase
    "2dconvolution": "2dconvolution",
    "3dconvolution": "3dconvolution",
    # FFT
    "fft1d_512": "fft",
    # Parboil: directory-style names in CSV
    "parboil_cutcp": "parboil_cutcp",
    "parboil_sgemm": "parboil_sgemm",
    "parboil_histo": "parboil_histo",
    "parboil_lbm": "parboil_lbm",
    "parboil_mri_gridding": "parboil_mri_gridding",
    "parboil_mri_q": "parboil_mri_q",
    "parboil_sad": "parboil_sad",
    "parboil_spmv": "parboil_spmv",
    "parboil_stencil": "parboil_stencil",
    "parboil_tpacf": "parboil_tpacf",
    # Parboil: HW/sim kernel names (alternate CSV naming)
    "cutoff_potential": "parboil_cutcp",
    "sgemm_tiled": "parboil_sgemm",
    "histogram": "parboil_histo",
    "lbm_stream_collide": "parboil_lbm",
    "gridding_kernel": "parboil_mri_gridding",
    "computeq": "parboil_mri_q",
    "computesad": "parboil_sad",
    "spmv_csr": "parboil_spmv",
    "stencil_kernel": "parboil_stencil",
    "tpacf_dd": "parboil_tpacf",
    "tpacf_dr": "parboil_tpacf",
    "tpacf_rr": "parboil_tpacf",
    # SpMV
    "spmv_csr_scalar": "spmv",
    # SHOC
    "shoc_gemm": "shoc_gemm",
    # Rodinia
    "backprop": "backprop",
    "layerforward": "backprop",
    "adjust_weights": "backprop_adjust_weights",
    "streamcluster": "streamcluster",
    "streamcluster_pgain": "streamcluster",
    # Rodinia: bitonicsort
    "sort": "bitonicsort",
    # SHOC: HW kernel names → CI directory names
    "busspeeddownload": "bus_speed_download",
    "busspeedreadback": "bus_speed_readback",
    "devicememory_read": "device_memory_read",
    "devicememory_write": "device_memory_write",
    "maxflops": "max_flops",
    "md": "md_lj",
    # Parboil: FindMinSAD from parboil_sad per-kernel timing
    "findminsad": "findminsad",
    # All others: csv kernel_name == bench dir name
}


def load_sim_results(artifacts_dir):
    """Load all sim results from CI artifact directories.

    Returns dict: {bench_dir_lower: {problem_size: sim_ms}}

    Results are indexed by the **lowercased** benchmark directory name.
    Additionally, if a ``BENCH_DIR_TO_SIM_KERNEL`` mapping exists, the
    results are also stored under the lowercased sim kernel name so that
    CSV rows using either naming convention can find their data.
    """
    results = {}
    
    for bench_dir in os.listdir(artifacts_dir):
        bench_path = os.path.join(artifacts_dir, bench_dir)
        if not os.path.isdir(bench_path):
            continue
        if bench_dir == "comparison-detailed":
            continue

        # Strip the "benchmark-results-" prefix if present
        bench_name = bench_dir
        if bench_name.startswith("benchmark-results-"):
            bench_name = bench_name[len("benchmark-results-"):]
            
        csv_file = os.path.join(bench_path, f"results_{bench_name}.csv")
        if not os.path.isfile(csv_file):
            # Fallback: CI artifact filenames use underscores even when
            # the directory name uses hyphens (e.g. benchmark-results-l1-cache-bw
            # contains results_l1_cache_bw.csv, not results_l1-cache-bw.csv).
            bench_name_u = bench_name.replace("-", "_")
            csv_file_u = os.path.join(bench_path, f"results_{bench_name_u}.csv")
            if os.path.isfile(csv_file_u):
                bench_name = bench_name_u
                csv_file = csv_file_u
            else:
                continue
            
        bench_results = {}
        with open(csv_file, "r") as f:
            reader = csv.DictReader(f)
            for row in reader:
                size = row["problem_size"].strip()
                try:
                    sim_ms = float(row["avg_ms"])
                    bench_results[size] = sim_ms
                except (ValueError, KeyError):
                    continue
        
        if bench_results:
            # Store under lowercased benchmark name (with prefix stripped)
            dir_lower = bench_name.lower()
            results[dir_lower] = bench_results
            # Also store under the hyphen/underscore alternative so that
            # merge_results can find data regardless of naming convention.
            alt_lower = (
                dir_lower.replace("-", "_")
                if "-" in dir_lower
                else dir_lower.replace("_", "-")
            )
            if alt_lower != dir_lower:
                results[alt_lower] = bench_results
            # Also store under the lowercased sim kernel name (if mapped)
            if bench_name in BENCH_DIR_TO_SIM_KERNEL:
                sim_kernel_lower = BENCH_DIR_TO_SIM_KERNEL[bench_name].lower()
                if sim_kernel_lower != dir_lower:
                    results[sim_kernel_lower] = bench_results
        
        # Check for secondary result files (e.g., results_findminsad.csv
        # in the parboil_sad directory for per-kernel FindMinSAD timing)
        findminsad_csv = os.path.join(bench_path, "results_findminsad.csv")
        if os.path.isfile(findminsad_csv):
            fm_results = {}
            with open(findminsad_csv, "r") as f:
                reader = csv.DictReader(f)
                for row in reader:
                    size = row["problem_size"].strip()
                    try:
                        sim_ms = float(row["avg_ms"])
                        fm_results[size] = sim_ms
                    except (ValueError, KeyError):
                        continue
            if fm_results:
                results["findminsad"] = fm_results
    
    return results


def get_bench_dir_for_csv_kernel(csv_kernel):
    """Map a comparison_ci.csv kernel_name to its CI benchmark directory.

    Performs case-insensitive lookup against ``CSV_KERNEL_TO_BENCH_DIR``.
    Falls back to the lowercased kernel name if no explicit mapping exists.
    """
    lower = csv_kernel.strip().lower()
    if lower in CSV_KERNEL_TO_BENCH_DIR:
        return CSV_KERNEL_TO_BENCH_DIR[lower]
    # Default: csv kernel name (lowercased) == bench dir name
    return lower


def merge_results(comparison_csv, sim_results, output_csv, clear=False):
    """Merge sim results into comparison_ci.csv.
    
    If clear=True, all existing sim_ms/abs_error_ms/rel_error values are
    blanked out first, ensuring only data from the current artifact set
    ends up in the output (single-run guarantee).
    """
    rows = []
    updated_kernels = set()
    total_updated = 0
    
    with open(comparison_csv, "r") as f:
        reader = csv.DictReader(f)
        fieldnames = reader.fieldnames
        
        for row in reader:
            csv_kernel = row["kernel_name"].strip()
            size = row["problem_size"].strip()
            real_ms_str = row["real_ms"].strip()
            
            # Clear existing sim data if requested
            if clear:
                row["sim_ms"] = ""
                row["abs_error_ms"] = ""
                row["rel_error"] = ""
            
            # Try to find sim data for this row (case-insensitive)
            bench_dir = get_bench_dir_for_csv_kernel(csv_kernel)
            bench_dir_lower = bench_dir.lower()
            
            if bench_dir_lower in sim_results and size in sim_results[bench_dir_lower]:
                sim_ms = sim_results[bench_dir_lower][size]
                row["sim_ms"] = f"{sim_ms:.6f}"
                
                if real_ms_str:
                    try:
                        real_ms = float(real_ms_str)
                        abs_err = abs(sim_ms - real_ms)
                        rel_err = (sim_ms - real_ms) / real_ms if real_ms > 0 else 0.0
                        row["abs_error_ms"] = f"{abs_err:.6f}"
                        row["rel_error"] = f"{rel_err:.6f}"
                    except ValueError:
                        pass
                
                updated_kernels.add(csv_kernel)
                total_updated += 1
            
            rows.append(row)
    
    # Write output
    with open(output_csv, "w", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=fieldnames)
        writer.writeheader()
        writer.writerows(rows)
    
    return updated_kernels, total_updated


def print_summary(sim_results, updated_kernels, total_updated, comparison_csv):
    """Print a summary of the merge operation."""
    print(f"\nSim results loaded from {len(sim_results)} benchmarks:")
    for bench, sizes in sorted(sim_results.items()):
        print(f"  {bench}: {len(sizes)} sizes")
    
    print(f"\nUpdated {total_updated} rows across {len(updated_kernels)} kernels:")
    for k in sorted(updated_kernels):
        print(f"  {k}")
    
    # Count total numeric points in output
    with open(comparison_csv, "r") as f:
        reader = csv.DictReader(f)
        total_rows = 0
        numeric_rows = 0
        kernels_with_sim = set()
        for row in reader:
            total_rows += 1
            if row["sim_ms"].strip():
                numeric_rows += 1
                kernels_with_sim.add(row["kernel_name"].strip())
    
    print(f"\nOutput CSV: {total_rows} total rows, {numeric_rows} with sim data")
    print(f"Benchmarks with sim data: {len(kernels_with_sim)}")
    for k in sorted(kernels_with_sim):
        print(f"  {k}")


def main():
    parser = argparse.ArgumentParser(
        description="Merge CI benchmark results into comparison_ci.csv"
    )
    parser.add_argument(
        "--artifacts-dir",
        required=True,
        help="Directory containing CI artifact subdirectories",
    )
    parser.add_argument(
        "--comparison",
        required=True,
        help="Path to comparison_ci.csv",
    )
    parser.add_argument(
        "--output",
        required=True,
        help="Output path for updated comparison_ci.csv",
    )
    parser.add_argument(
        "--clear",
        action="store_true",
        help="Clear all existing sim data before merging (ensures single-run data)",
    )
    args = parser.parse_args()
    
    # Load sim results
    sim_results = load_sim_results(args.artifacts_dir)
    
    if not sim_results:
        print("ERROR: No sim results found!", file=sys.stderr)
        sys.exit(1)
    
    # Merge
    updated_kernels, total_updated = merge_results(
        args.comparison, sim_results, args.output, clear=args.clear
    )
    
    # Summary
    print_summary(sim_results, updated_kernels, total_updated, args.output)


if __name__ == "__main__":
    main()
