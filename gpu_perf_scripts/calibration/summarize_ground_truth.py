#!/usr/bin/env python3
"""Summarize mi300a_ground_truth.db into a compact, committable CSV.

The full SQLite DB is ~9 MB: it stores every individual repetition (21k+ rows
across 7 benchmarks -- atomic_operations alone is 56%) plus ~5 MB of indexes.
Calibration only needs the per-configuration aggregate (mean over the 7 reps),
which is a few hundred rows. This script emits that summary so the repo can
commit the small CSV instead of the multi-megabyte DB.

The DB itself is NOT committed; keep it wherever you collect ground truth and
re-run this script to refresh the CSV when new measurements land.

Usage:
    summarize_ground_truth.py --db /path/to/mi300a_ground_truth.db \
        --out gpu_perf_scripts/calibration/mi300a_ground_truth.csv
"""

import argparse
import csv
import sqlite3

# benchmark -> ground-truth metric stored in the `metrics` table.
SPECS = {
    "fp32_throughput": "GFLOPS",
    "cache_latency": "latency_ns",
}

COLUMNS = [
    "benchmark", "scaling_param_name", "scaling_param_value",
    "num_blocks", "threads_per_block", "num_accesses",
    "metric_name", "metric_mean", "kernel_ms_mean", "n_reps",
]


def summarize(conn, benchmark, metric_name):
    """One aggregated row per distinct (config, scaling point)."""
    # Mean metric value + rep count, grouped by the full config key.
    metric_rows = conn.execute(
        """
        SELECT r.scaling_param_name,
               r.scaling_param_value,
               CAST(json_extract(r.non_scaling_params_json, '$.num_blocks') AS INTEGER),
               CAST(json_extract(r.non_scaling_params_json, '$.threads_per_block') AS INTEGER),
               CAST(json_extract(r.non_scaling_params_json, '$.num_accesses') AS INTEGER),
               AVG(m.metric_value),
               COUNT(*)
        FROM runs r
        JOIN metrics m ON m.run_id = r.id AND m.metric_name = ?
        WHERE r.benchmark = ? AND r.status = 'ok'
        GROUP BY 1, 2, 3, 4, 5
        """,
        (metric_name, benchmark),
    ).fetchall()

    # Mean per-kernel time (ms) for the same config keys, for context.
    kernel_ms = {}
    for spn, spv, nb, tpb, na, kms in conn.execute(
        """
        SELECT r.scaling_param_name,
               r.scaling_param_value,
               CAST(json_extract(r.non_scaling_params_json, '$.num_blocks') AS INTEGER),
               CAST(json_extract(r.non_scaling_params_json, '$.threads_per_block') AS INTEGER),
               CAST(json_extract(r.non_scaling_params_json, '$.num_accesses') AS INTEGER),
               AVG(kr.time_ms)
        FROM runs r
        JOIN kernel_results kr ON kr.run_id = r.id
        WHERE r.benchmark = ? AND r.status = 'ok'
        GROUP BY 1, 2, 3, 4, 5
        """,
        (benchmark,),
    ).fetchall():
        kernel_ms[(spn, spv, nb, tpb, na)] = kms

    out = []
    for spn, spv, nb, tpb, na, mean, n in metric_rows:
        out.append({
            "benchmark": benchmark,
            "scaling_param_name": spn,
            "scaling_param_value": spv,
            "num_blocks": "" if nb is None else nb,
            "threads_per_block": "" if tpb is None else tpb,
            "num_accesses": "" if na is None else na,
            "metric_name": metric_name,
            "metric_mean": round(mean, 4),
            "kernel_ms_mean": round(kernel_ms.get((spn, spv, nb, tpb, na), float("nan")), 6),
            "n_reps": n,
        })
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", required=True)
    ap.add_argument("--out", default="gpu_perf_scripts/calibration/mi300a_ground_truth.csv")
    args = ap.parse_args()

    conn = sqlite3.connect(args.db)
    rows = []
    for benchmark, metric in SPECS.items():
        rows.extend(summarize(conn, benchmark, metric))
    conn.close()

    rows.sort(key=lambda r: (
        r["benchmark"],
        int(r["num_blocks"]) if r["num_blocks"] != "" else 0,
        int(r["threads_per_block"]) if r["threads_per_block"] != "" else 0,
        int(r["scaling_param_value"]),
    ))

    with open(args.out, "w", newline="") as f:
        w = csv.DictWriter(f, fieldnames=COLUMNS)
        w.writeheader()
        w.writerows(rows)

    print(f"wrote {len(rows)} rows to {args.out}")


if __name__ == "__main__":
    main()
