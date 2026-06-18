#!/usr/bin/env python3
"""Rebuild the calibration dashboard's run index from the published run data.

Scans <runs_dir>/<run_id>/data.json (each written by `plot_calibration.py --json`)
and emits an index.json listing every run, newest first, so the static dashboard
can populate its run picker without server-side code.

  build_index.py <runs_dir> [-o index.json]

Rebuilding from the actual files (rather than appending) keeps the index
self-healing: a run whose folder was removed simply drops out next publish.
"""

import argparse
import json
import os
import sys


def collect(runs_dir):
    runs = []
    if not os.path.isdir(runs_dir):
        return runs
    for run_id in os.listdir(runs_dir):
        data_path = os.path.join(runs_dir, run_id, "data.json")
        if not os.path.isfile(data_path):
            continue
        try:
            with open(data_path) as f:
                d = json.load(f)
        except (OSError, ValueError) as e:
            print(f"WARNING: skipping {data_path}: {e}", file=sys.stderr)
            continue
        n_fig = sum(len(b.get("figures", [])) for b in d.get("benchmarks", []))
        runs.append({
            "run_id": d.get("run_id") or run_id,
            "run_url": d.get("run_url", ""),
            "generated_at": d.get("generated_at", ""),
            "overall_mape": d.get("overall_mape"),
            "overall_smape": d.get("overall_smape"),
            "n_figures": n_fig,
            "data": f"runs/{run_id}/data.json",
        })
    # Newest first: sort by timestamp then run id, both descending.
    runs.sort(key=lambda r: (r["generated_at"], str(r["run_id"])), reverse=True)
    return runs


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("runs_dir")
    ap.add_argument("-o", "--out", default="", help="write here instead of stdout")
    args = ap.parse_args()

    index = {"schema": 1, "runs": collect(args.runs_dir)}
    text = json.dumps(index, indent=2)
    if args.out:
        with open(args.out, "w") as f:
            f.write(text + "\n")
        print(f"wrote {args.out} ({len(index['runs'])} runs)", file=sys.stderr)
    else:
        print(text)


if __name__ == "__main__":
    main()
