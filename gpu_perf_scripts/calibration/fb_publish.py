#!/usr/bin/env python3
"""Publish MI300X calibration run status + per-point results to Firestore, for the
live dashboard.

BEST-EFFORT BY DESIGN: any Firestore/credential error prints a warning and exits 0,
so the live-dashboard plumbing can never fail the actual calibration sweep.

Auth: GOOGLE_APPLICATION_CREDENTIALS -> service-account JSON (Firebase Admin SDK,
which bypasses security rules, so the dashboard rules can stay public-read/no-write).

Firestore layout:
  runs/{run_id}                      {status, started_at, updated_at, run_url,
                                      overall_mape, overall_smape}
  runs/{run_id}/points/{slug__xN}    {benchmark, label, slug, x, sim_ms, real_ms}

Subcommands:
  mark-run       --run-id ID --status {in_progress,completed,failed}
                 [--run-url URL] [--from-json data.json]   # pulls overall_mape/smape
  publish-points --run-id ID --benchmark NAME --csv sweep.csv --ref ground_truth.csv
                 # uploads only NEW rows since the last call (state: <csv>.fbpos),
                 # joining the ground truth for real_ms. Idempotent (deterministic
                 # doc ids), so re-runs never duplicate.
"""

import argparse
import csv
import json
import os
import sys

from calib_common import parse_ns, ns_label, ns_slug, xparse


def warn(msg):
    print(f"WARNING (fb_publish): {msg}", file=sys.stderr)


def get_db():
    import firebase_admin
    from firebase_admin import credentials, firestore
    if not firebase_admin._apps:
        firebase_admin.initialize_app(credentials.ApplicationDefault())
    return firestore.client()


def server_ts():
    from firebase_admin import firestore
    return firestore.SERVER_TIMESTAMP


def load_real(ref_path, benchmark):
    """(slug, x) -> kernel_ms_mean from the ground-truth CSV; slug from non_scaling_json."""
    real = {}
    if not os.path.exists(ref_path):
        return real
    with open(ref_path, newline="") as f:
        for r in csv.DictReader(f):
            if r["benchmark"] != benchmark:
                continue
            try:
                slug = ns_slug(parse_ns(r.get("non_scaling_json", "{}")))
                real[(slug, xparse(r["scaling_param_value"]))] = float(r["kernel_ms_mean"])
            except (KeyError, ValueError):
                continue
    return real


def publish_points(run_id, benchmark, csv_path, ref_path):
    if not os.path.exists(csv_path):
        return
    rows = list(csv.DictReader(open(csv_path, newline="")))
    pos_path = csv_path + ".fbpos"
    start = 0
    if os.path.exists(pos_path):
        try:
            start = int(open(pos_path).read().strip() or "0")
        except ValueError:
            start = 0
    new = rows[start:]
    if not new:
        return
    real = load_real(ref_path, benchmark)
    db = get_db()
    coll = db.collection("runs").document(run_id).collection("points")
    batch = db.batch()
    n = 0
    for r in new:
        try:
            ns = parse_ns(r.get("non_scaling_json", "{}"))
            label, slug = ns_label(ns), ns_slug(ns)
            x = xparse(r["scaling_param_value"])
            sim_ms = float(r["kernel_time_s"]) * 1000.0
        except (KeyError, ValueError) as e:
            warn(f"skipping row {r}: {e}")
            continue
        # Include benchmark in the id: several throughput benchmarks share the
        # same (slug, x), so a bare slug__x would let them overwrite each other in
        # the shared runs/{run_id}/points collection.
        batch.set(coll.document(f"{benchmark}__{slug}__x{x}"), {
            "benchmark": benchmark, "label": label, "slug": slug,
            "scaling": r.get("scaling_param_name", ""),
            "x": x, "sim_ms": sim_ms, "real_ms": real.get((slug, x)),
        })
        n += 1
    batch.commit()
    with open(pos_path, "w") as f:
        f.write(str(len(rows)))
    print(f"published {n} {benchmark} point(s) to runs/{run_id}")


def mark_run(run_id, status, run_url=None, from_json=None):
    data = {"status": status, "updated_at": server_ts()}
    if status == "in_progress":
        data["started_at"] = server_ts()
    if status == "completed":
        data["completed_at"] = server_ts()
    if run_url:
        data["run_url"] = run_url
    if from_json and os.path.exists(from_json):
        try:
            j = json.load(open(from_json))
            data["overall_mape"] = j.get("overall_mape")
            data["overall_smape"] = j.get("overall_smape")
            # generated_at gives a stable ordering key for runs that were never
            # marked in_progress live (e.g. backfilled history), where started_at
            # is absent.
            if j.get("generated_at"):
                data["generated_at"] = j["generated_at"]
        except (OSError, ValueError) as e:
            warn(f"could not read {from_json}: {e}")
    get_db().collection("runs").document(run_id).set(data, merge=True)
    print(f"marked runs/{run_id} {status}")


def main():
    ap = argparse.ArgumentParser()
    sub = ap.add_subparsers(dest="cmd", required=True)
    m = sub.add_parser("mark-run")
    m.add_argument("--run-id", required=True)
    m.add_argument("--status", required=True,
                   choices=["in_progress", "completed", "failed"])
    m.add_argument("--run-url", default=None)
    m.add_argument("--from-json", default=None)
    p = sub.add_parser("publish-points")
    p.add_argument("--run-id", required=True)
    p.add_argument("--benchmark", required=True)
    p.add_argument("--csv", required=True)
    p.add_argument("--ref", required=True)
    args = ap.parse_args()

    try:
        if args.cmd == "mark-run":
            mark_run(args.run_id, args.status, args.run_url, args.from_json)
        else:
            publish_points(args.run_id, args.benchmark, args.csv, args.ref)
    except Exception as e:  # best-effort: never break the sweep
        warn(f"{type(e).__name__}: {e}")
        sys.exit(0)


if __name__ == "__main__":
    main()
