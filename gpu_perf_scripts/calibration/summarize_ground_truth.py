#!/usr/bin/env python3
"""Summarize mi300a_ground_truth.db into a compact, committable CSV.

The full SQLite DB stores every individual repetition across all benchmarks plus
large indexes. Calibration only needs the per-configuration aggregate (mean
kernel time over the reps), which is a few thousand rows -- so this emits that
summary and the repo commits the small CSV instead of the multi-megabyte DB.

The CSV is benchmark-agnostic: the per-config non-scaling parameters are kept
verbatim as a JSON column (`non_scaling_json`), so any benchmark's parameter set
is captured without bespoke columns. One row per (benchmark, scaling point,
non-scaling combo); `kernel_ms_mean` (mean real kernel time, from kernel_results)
is what the sweep calibrates the simulator against.

The DB itself is NOT committed; keep it wherever you collect ground truth and
re-run this script to refresh the CSV when new measurements land.

Usage:
    summarize_ground_truth.py --db /path/to/mi300a_ground_truth.db \
        --out gpu_perf_scripts/calibration/mi300a_ground_truth.csv
"""

import argparse
import csv
import sqlite3

# Benchmarks intentionally left out of calibration (no usable sim runner and/or
# explicitly excluded). Everything else in the DB is summarized.
EXCLUDE = {"atomic_operations", "shoc_scan", "shoc_triad"}

COLUMNS = [
    "benchmark", "scaling_param_name", "scaling_param_value",
    "non_scaling_json", "kernel_ms_mean", "n_reps",
]


def summarize(conn):
    """One aggregated row per distinct (benchmark, scaling point, non-scaling combo);
    kernel_ms_mean is the mean real kernel time over the ok repetitions."""
    rows = conn.execute(
        """
        SELECT r.benchmark,
               r.scaling_param_name,
               r.scaling_param_value,
               r.non_scaling_params_json,
               AVG(kr.time_ms),
               COUNT(DISTINCT r.id)
        FROM runs r
        JOIN kernel_results kr ON kr.run_id = r.id
        WHERE r.status = 'ok'
        GROUP BY r.benchmark, r.scaling_param_name, r.scaling_param_value,
                 r.non_scaling_params_json
        """
    ).fetchall()
    out = []
    for bench, spn, spv, ns_json, kms, n in rows:
        if bench in EXCLUDE:
            continue
        out.append({
            "benchmark": bench,
            "scaling_param_name": spn,
            "scaling_param_value": spv,
            "non_scaling_json": ns_json or "{}",
            "kernel_ms_mean": round(kms, 6) if kms is not None else "",
            "n_reps": n,
        })
    return out


def _scale_key(v):
    try:
        return float(v)
    except (TypeError, ValueError):
        return float("inf")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", required=True)
    ap.add_argument("--out", default="gpu_perf_scripts/calibration/mi300a_ground_truth.csv")
    args = ap.parse_args()

    conn = sqlite3.connect(args.db)
    rows = summarize(conn)
    conn.close()

    rows.sort(key=lambda r: (r["benchmark"], r["non_scaling_json"],
                             _scale_key(r["scaling_param_value"])))

    with open(args.out, "w", newline="") as f:
        w = csv.DictWriter(f, fieldnames=COLUMNS)
        w.writeheader()
        w.writerows(rows)

    benches = sorted({r["benchmark"] for r in rows})
    print(f"wrote {len(rows)} rows ({len(benches)} benchmarks) to {args.out}")
    print("benchmarks:", ", ".join(benches))


if __name__ == "__main__":
    main()
