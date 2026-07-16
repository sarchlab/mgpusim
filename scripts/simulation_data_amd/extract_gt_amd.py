#!/usr/bin/env python3
"""
Build MI300X_ground_truth.csv from a gpubench results.db SQLite file.

For each `run` (= one repetition of one benchmark config), the real GPU
"kernel execution time" is the SUM of that run's kernel_results.time_ms rows
(a run can launch several kernels, e.g. rodinia_backprop launches 6 -- their
individual time_ms values are what's comparable to the simulator's per-kernel
output). We deliberately do NOT use runs.total_time_ms, which includes
non-kernel host-side overhead.

A benchmark configuration is identified by the (suite, benchmark,
non_scaling_params_json) columns, kept separate rather than merged into one
string. `scaling_param_value` selects the problem size being swept within
that configuration (e.g. 32, 64, 128, ...). Each (suite, benchmark,
non_scaling_params_json, scaling_param_value) combo was measured with
repetition = 1/2/3; those repeats are averaged into time_ms_avg.

session_id and tier are carried through from the `runs` table for reference.
Both are constant across a config's 1-3 repetitions (verified against the
data), so each output row's session_id/tier is just that shared value.

Only status='ok' runs are used. Runs with status='timeout' or 'error' are
excluded, as are the rare 'ok' runs that have zero kernel_results rows
(e.g. tensor_core_throughput on this platform, which is a CUDA-only
microbench that no-ops on MI300X/ROCm and reports total_time_ms=0 with no
kernels) -- there's no kernel-level timing to use as ground truth for them.

Usage:
    python3 extract_gt_amd.py [--db results.db] [--out MI300X_ground_truth.csv]
"""

import argparse
import csv
import json
import sqlite3
from collections import defaultdict


def canonical_json(raw: str) -> str:
    """Re-serialize non_scaling_params_json with sorted keys so that two
    logically-identical param sets always group together, even if the JSON
    text happened to be written with keys in a different order somewhere in
    the data."""
    return json.dumps(json.loads(raw), sort_keys=True, separators=(",", ":"))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--db", default="results.db", help="Path to results.db")
    parser.add_argument(
        "--out", default="MI300X_ground_truth.csv", help="Output CSV path"
    )
    args = parser.parse_args()

    conn = sqlite3.connect(args.db)
    cur = conn.cursor()

    # One row per run_id: the run's total kernel-execution time for that
    # single repetition, plus the config columns needed to build the key.
    cur.execute(
        """
        SELECT r.id,
               r.session_id,
               r.tier,
               r.suite,
               r.benchmark,
               r.non_scaling_params_json,
               r.scaling_param_name,
               r.scaling_param_value,
               SUM(k.time_ms) AS kernel_time_ms
        FROM runs r
        JOIN kernel_results k ON k.run_id = r.id
        WHERE r.status = 'ok'
        GROUP BY r.id
        """
    )
    per_run = cur.fetchall()
    conn.close()

    # Group per-run kernel-time sums by the full config (session_id/tier
    # included -- both are constant within a config's repetitions, so they
    # ride along as part of the key) and collect all repetitions for
    # averaging.
    groups = defaultdict(list)
    for _run_id, session_id, tier, suite, benchmark, nsp_json, sp_name, sp_value, kernel_time in per_run:
        nsp_canon = canonical_json(nsp_json)
        key = (session_id, tier, suite, benchmark, nsp_canon, sp_name, sp_value)
        groups[key].append(kernel_time)

    out_rows = []
    for (session_id, tier, suite, benchmark, nsp_canon, sp_name, sp_value), times in groups.items():
        time_ms_avg = sum(times) / len(times)
        out_rows.append(
            (session_id, tier, suite, benchmark, nsp_canon, sp_name, sp_value, time_ms_avg, len(times))
        )

    # Sort for a readable, stable file: by suite/benchmark/params, then
    # numerically by the scaling param value.
    out_rows.sort(key=lambda row: (row[2], row[3], row[4], float(row[6])))

    with open(args.out, "w", newline="") as f:
        writer = csv.writer(f)
        writer.writerow(
            [
                "session_id",
                "tier",
                "suite",
                "benchmark",
                "non_scaling_params_json",
                "scaling_param_name",
                "scaling_param_value",
                "time_ms_avg",
            ]
        )
        for session_id, tier, suite, benchmark, nsp_canon, sp_name, sp_value, time_ms_avg, _n in out_rows:
            writer.writerow(
                [
                    session_id,
                    tier,
                    suite,
                    benchmark,
                    nsp_canon,
                    sp_name,
                    sp_value,
                    f"{time_ms_avg:.6f}",
                ]
            )

    n_partial = sum(1 for row in out_rows if row[8] < 3)
    print(f"Wrote {len(out_rows)} rows to {args.out}")
    print(f"  ({n_partial} of those rows have < 3 repetitions averaged in)")


if __name__ == "__main__":
    main()