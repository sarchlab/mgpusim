#!/usr/bin/env python3
"""Summarize mi300x_ground_truth.db into a compact, committable CSV.

The full SQLite DB stores every individual repetition across all benchmarks plus
large indexes. Calibration only needs the per-configuration aggregate (mean
per-run TOTAL kernel time over the reps), which is a few thousand rows -- so this
emits that summary and the repo commits the small CSV instead of the multi-MB DB.

The CSV is benchmark-agnostic: the per-config non-scaling parameters are kept
verbatim as a JSON column (`non_scaling_json`), so any benchmark's parameter set
is captured without bespoke columns. One row per (benchmark, scaling point,
non-scaling combo); `kernel_ms_mean` is the mean over reps of each run's TOTAL
kernel time (sum of all kernel launches), matching the simulator's cumulative
`kernel_time` -- this is what the sweep calibrates against.

The DB itself is NOT committed; keep it wherever you collect ground truth and
re-run this script to refresh the CSV when new measurements land.

Usage:
    summarize_ground_truth.py --db /path/to/mi300x_ground_truth.db \
        --out gpu_perf_scripts/calibration/mi300x_ground_truth.csv
"""

import argparse
import csv
import sqlite3

# The committed CSV is the COMPLETE real-hardware record: every benchmark in the
# DB is summarized, including ones with no sim runner yet. Which benchmarks are
# actually simulated/calibrated is decided by run_sweep.py's SPECS and the CI
# matrix, not here.
EXCLUDE = set()

# Benchmarks whose meaningful real value is a per-launch METRIC, not the total
# kernel time. empty_kernel reports launch_sync_us (the real launch+synchronize
# round-trip per launch); the sim's single-launch kernel_time is the model's
# per-launch overhead, directly comparable. Value: (metric_name, scale_to_ms).
METRIC_AS_KERNEL_MS = {"empty_kernel": ("launch_sync_us", 1e-3)}

COLUMNS = [
    "benchmark", "scaling_param_name", "scaling_param_value",
    "non_scaling_json", "kernel_ms_mean", "n_reps",
]


def summarize(conn):
    """One aggregated row per distinct (benchmark, scaling point, non-scaling combo).

    kernel_ms_mean is the mean, over the ok repetitions, of each run's TOTAL kernel
    time -- the SUM of every kernel launch in that run. This matches the simulator's
    `kernel_time` metric (cumulative GPU busy time across all launches). The earlier
    AVG(kr.time_ms) averaged individual kernel times, which understated the real
    value for multi-kernel benchmarks by ~the kernel count -- and averaging across
    what are often DIFFERENT kernels has no physical meaning."""
    metric_benches = tuple(METRIC_AS_KERNEL_MS)
    placeholders = ",".join("?" * len(metric_benches)) or "''"
    rows = conn.execute(
        f"""
        SELECT benchmark, scaling_param_name, scaling_param_value,
               non_scaling_params_json,
               AVG(run_total_ms),   -- mean over reps of each run's total kernel time
               COUNT(*)             -- number of ok reps
        FROM (
            SELECT r.benchmark, r.scaling_param_name, r.scaling_param_value,
                   r.non_scaling_params_json,
                   SUM(kr.time_ms) AS run_total_ms
            FROM runs r
            JOIN kernel_results kr ON kr.run_id = r.id
            WHERE r.status = 'ok' AND r.benchmark NOT IN ({placeholders})
            GROUP BY r.id
        )
        GROUP BY benchmark, scaling_param_name, scaling_param_value,
                 non_scaling_params_json
        """,
        metric_benches,
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

    # Per-launch-metric benchmarks (e.g. empty_kernel): kernel_ms_mean is the mean
    # of a recorded metric (e.g. launch_sync_us), scaled to ms, not a kernel total.
    for bench, (metric_name, scale) in METRIC_AS_KERNEL_MS.items():
        if bench in EXCLUDE:
            continue
        for spn, spv, ns_json, val, n in conn.execute(
            """
            SELECT r.scaling_param_name, r.scaling_param_value,
                   r.non_scaling_params_json, AVG(m.metric_value), COUNT(*)
            FROM runs r
            JOIN metrics m ON m.run_id = r.id
            WHERE r.status = 'ok' AND r.benchmark = ? AND m.metric_name = ?
            GROUP BY r.scaling_param_name, r.scaling_param_value,
                     r.non_scaling_params_json
            """,
            (bench, metric_name),
        ).fetchall():
            out.append({
                "benchmark": bench,
                "scaling_param_name": spn,
                "scaling_param_value": spv,
                "non_scaling_json": ns_json or "{}",
                "kernel_ms_mean": round(val * scale, 6) if val is not None else "",
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
    ap.add_argument("--out", default="gpu_perf_scripts/calibration/mi300x_ground_truth.csv")
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
