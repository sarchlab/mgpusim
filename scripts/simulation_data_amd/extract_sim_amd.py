#!/usr/bin/env python3
"""
Extract MI300X_simulation.csv and MI300X_ground_truth_2.csv from the
`data.json` artifact produced by a "MI300X Calibration" GitHub Actions run
(gpu_perf_scripts/calibration/plot_calibration.py's --json output, bundled
into the `mi300x-calibration-report` artifact).

data.json shape (schema 1):
  {
    "benchmarks": [
      {
        "name": "altis_cfd",
        "xlabel": "size (scaling factor)",
        "figures": [
          {
            "label": "block_size=256, precision=double",  # one non-scaling combo
            "slug": "block_size-256_precision-double",
            "mape": 0.0639, "smape": 0.0700, "n": 10,
            "offset": 0.014826,      # additive shift applied to raw sim time
            "anchor_x": 500,         # scaling value used to fix the offset (excluded from MAPE/n)
            "sim":  [[scaling_value, aligned_sim_time_ms], ...],
            "real": [[scaling_value, real_time_ms], ...]
          }, ...
        ]
      }, ...
    ]
  }

Two things worth knowing before you use the output:

1. `sim` values are ALIGNED (raw_sim + offset), i.e. exactly the blue
   "simulation (aligned)" curve plotted on the dashboard -- NOT the
   simulator's raw un-aligned output. Raw value = aligned - offset (we keep
   `offset_ms` in the CSV so you can back this out per-row if you need it).
2. `real` arrays are longer than `sim` arrays per figure: they carry the
   FULL ground-truth sweep range, while `sim` only has the points the
   simulator actually reached before its per-run/per-benchmark time budget
   cut it off. So MI300X_ground_truth_2.csv will have more rows than
   MI300X_simulation.csv.

`suite` isn't present in data.json (run_sweep.py is only ever called with a
bare benchmark name), so it's filled in here from a static benchmark->suite
map read off the `runs` table distinct (suite, benchmark) pairs in
results.db -- the same source of truth extract_gt_amd.py uses. If a
benchmark name shows up that isn't in the map, suite is left as "UNKNOWN"
and a warning is printed (rather than crashing), since new benchmarks can be
added to the workflow later.

Usage:
    python3 extract_sim_amd.py --json data.json \\
        --sim-out MI300X_simulation.csv \\
        --gt-out MI300X_ground_truth_2.csv
"""

import argparse
import csv
import json
import sys

# suite -> benchmark, from `SELECT DISTINCT suite, benchmark FROM runs` in
# results.db. Only benchmarks actually used by mi300a_calibration.yml need to
# resolve here; the rest of the table is harmless extra coverage.
SUITE_MAP = {
    "altis_cfd": "altis", "altis_raytracing": "altis",
    "cache_latency": "microbench", "empty_kernel": "microbench",
    "fp16_throughput": "microbench", "fp32_throughput": "microbench",
    "fp64_throughput": "microbench", "int32_throughput": "microbench",
    "memory_bandwidth": "microbench", "shared_mem_bandwidth": "microbench",
    "shared_mem_latency": "microbench",
    "parboil_cutcp": "parboil", "parboil_lbm": "parboil", "parboil_sgemm": "parboil",
    "polybench_2dconv": "polybench", "polybench_2mm": "polybench",
    "polybench_3dconv": "polybench", "polybench_3mm": "polybench",
    "polybench_correlation": "polybench", "polybench_fdtd2d": "polybench",
    "polybench_gemm": "polybench", "polybench_jacobi2d": "polybench",
    "polybench_mvt": "polybench", "polybench_syr2k": "polybench",
    "rodinia_backprop": "rodinia", "rodinia_gaussian": "rodinia",
    "rodinia_hotspot": "rodinia", "rodinia_hotspot3d": "rodinia",
    "rodinia_lavamd": "rodinia", "rodinia_lud": "rodinia",
    "rodinia_pathfinder": "rodinia", "rodinia_srad": "rodinia",
    "tango_binomial_options": "tango", "tango_blackscholes": "tango",
}


def scaling_param_name(xlabel: str) -> str:
    """'size (scaling factor)' -> 'size'"""
    suffix = " (scaling factor)"
    return xlabel[: -len(suffix)] if xlabel.endswith(suffix) else xlabel


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--json", default="data.json", help="Path to the run's data.json")
    parser.add_argument("--sim-out", default="MI300X_simulation.csv", help="Output path for simulated data")
    parser.add_argument("--gt-out", default="MI300X_ground_truth_2.csv", help="Output path for the ground-truth data embedded in data.json")
    args = parser.parse_args()

    with open(args.json) as f:
        data = json.load(f)

    sim_rows = []
    gt_rows = []
    unknown_suites = set()

    for bench in data["benchmarks"]:
        name = bench["name"]
        suite = SUITE_MAP.get(name)
        if suite is None:
            suite = "UNKNOWN"
            unknown_suites.add(name)
        sp_name = scaling_param_name(bench["xlabel"])

        for fig in bench["figures"]:
            combo = fig["label"]
            slug = fig["slug"]
            offset = fig["offset"]
            anchor_x = fig["anchor_x"]
            mape = fig["mape"]
            smape = fig["smape"]

            for sp_value, time_ms in fig["sim"]:
                sim_rows.append(
                    (
                        suite, name, combo, slug, sp_name, sp_value,
                        time_ms, offset, anchor_x, (sp_value == anchor_x),
                        mape, smape,
                    )
                )

            for sp_value, time_ms in fig["real"]:
                gt_rows.append((suite, name, combo, slug, sp_name, sp_value, time_ms))

    if unknown_suites:
        print(
            f"WARNING: no suite mapping for: {sorted(unknown_suites)} "
            f"(written as suite='UNKNOWN' -- add them to SUITE_MAP)",
            file=sys.stderr,
        )

    sim_rows.sort(key=lambda r: (r[0], r[1], r[2], float(r[5])))
    gt_rows.sort(key=lambda r: (r[0], r[1], r[2], float(r[5])))

    with open(args.sim_out, "w", newline="") as f:
        writer = csv.writer(f)
        writer.writerow(
            [
                "suite", "benchmark", "non_scaling_combo", "non_scaling_slug",
                "scaling_param_name", "scaling_param_value",
                "sim_time_ms_aligned", "offset_ms", "anchor_x", "is_anchor",
                "mape", "smape",
            ]
        )
        for suite, name, combo, slug, sp_name, sp_value, time_ms, offset, anchor_x, is_anchor, mape, smape in sim_rows:
            writer.writerow(
                [
                    suite, name, combo, slug, sp_name, sp_value,
                    f"{time_ms:.6f}", f"{offset:.6f}", anchor_x, is_anchor,
                    "" if mape is None else f"{mape:.6f}",
                    "" if smape is None else f"{smape:.6f}",
                ]
            )

    with open(args.gt_out, "w", newline="") as f:
        writer = csv.writer(f)
        writer.writerow(
            [
                "suite", "benchmark", "non_scaling_combo", "non_scaling_slug",
                "scaling_param_name", "scaling_param_value", "time_ms_avg",
            ]
        )
        for suite, name, combo, slug, sp_name, sp_value, time_ms in gt_rows:
            writer.writerow([suite, name, combo, slug, sp_name, sp_value, f"{time_ms:.6f}"])

    print(f"Wrote {len(sim_rows)} rows to {args.sim_out} (aligned sim points, {sum(1 for r in sim_rows if r[9])} of them anchors)")
    print(f"Wrote {len(gt_rows)} rows to {args.gt_out} (ground-truth points embedded in data.json)")


if __name__ == "__main__":
    main()