#!/usr/bin/env python3
"""Generic MI300A calibration sweep for ONE microbenchmark.

Reads the ground-truth CSV for <benchmark>, runs the matching mgpusim sample in
timing mode at each measured configuration, and writes a sim-results CSV that
plot_calibration.py / fb_publish.py consume. Streams points to Firestore for the
live dashboard when FB_RUN_ID is set (best-effort -- never fails the sweep).

Each benchmark's SPEC maps the ground-truth params to the sample's CLI flags. A
sample may not expose every ground-truth param (e.g. fp64/int32 have no
threads_per_block flag); those params are simply not set, so configs that differ
only in an unsupported param produce identical sim runs -- memoized here, and the
sim curve repeats across that dimension in the report (the honest result).

Usage:  run_sweep.py <benchmark> <out.csv> [--ref CSV]
Env: PER_RUN_TIMEOUT (s, default 1800), SWEEP_TIMEOUT (s, default 86400),
     FB_RUN_ID, FB_EVERY (default 5), GOOGLE_APPLICATION_CREDENTIALS, FB_PY.
"""
import argparse
import csv
import os
import subprocess
import sqlite3
import sys
import tempfile
import time

from calib_common import parse_ns

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.abspath(os.path.join(SCRIPT_DIR, "..", ".."))
COMMON = ["-timing", "-arch", "cdna3", "-gpu", "mi300a", "-disable-rtm"]

# benchmark -> {sample dir, scaling-value flag, ground-truth-param -> CLI flag}.
SPECS = {
    "fp32_throughput":      {"sample": "fp32_throughput",      "scaling_flag": "-fmas",
                             "params": {"num_blocks": "-num-blocks", "threads_per_block": "-threads-per-block"}},
    "fp16_throughput":      {"sample": "fp16_throughput",      "scaling_flag": "-fmas-per-thread",
                             "params": {"num_blocks": "-num-blocks", "threads_per_block": "-threads-per-block"}},
    "fp64_throughput":      {"sample": "fp64_throughput",      "scaling_flag": "-fmas-per-thread",
                             "params": {"num_blocks": "-num-blocks"}},
    "int32_throughput":     {"sample": "int32_throughput",     "scaling_flag": "-mads",
                             "params": {"num_blocks": "-blocks"}},
    # -size is a float32 ELEMENT count; ground truth scales by buffer_size_mb, so
    # convert MiB -> float32 elements (1 MiB / 4 bytes).
    "memory_bandwidth":     {"sample": "memory_bandwidth",     "scaling_flag": "-size",
                             "params": {}, "scaling_xform": lambda v: int(v) * 1024 * 1024 // 4},
    "shared_mem_bandwidth": {"sample": "shared_mem_bandwidth", "scaling_flag": "-inner-iters",
                             "params": {"num_blocks": "-num-blocks"}},
    "cache_latency":        {"sample": "cache_latency",        "scaling_flag": "-array-bytes",
                             "params": {"num_accesses": "-num-accesses", "rng_seed": "-seed"}},
}


def load_configs(ref, benchmark):
    """[(real_ms, scaling_name, scaling_value, non_scaling_dict)] cheapest-first."""
    out = []
    with open(ref, newline="") as f:
        for r in csv.DictReader(f):
            if r["benchmark"] != benchmark:
                continue
            try:
                real = float(r["kernel_ms_mean"])
            except (ValueError, KeyError):
                real = float("inf")
            out.append((real, r["scaling_param_name"], r["scaling_param_value"],
                        parse_ns(r.get("non_scaling_json", "{}"))))
    out.sort(key=lambda c: c[0])
    return out


def sample_args(spec, scaling_value, ns):
    """CLI args the sample supports for this config (unsupported gt params dropped).

    The recorded scaling_param_value stays the ground-truth value (for matching);
    only the CLI flag value is transformed via the spec's optional scaling_xform.
    """
    xform = spec.get("scaling_xform")
    sval = xform(scaling_value) if xform else scaling_value
    args = [spec["scaling_flag"], str(sval)]
    for gt_param, flag in spec["params"].items():
        if gt_param in ns:
            args += [flag, str(ns[gt_param])]
    return args


def run_one(binary, args, work_dir, per_run_timeout):
    """Run the sample under the per-run timeout; return kernel_time (s) or None."""
    run_dir = tempfile.mkdtemp(dir=work_dir)
    cmd = [binary] + COMMON + args + ["-metric-file-name", "t"]
    try:
        subprocess.run(cmd, cwd=run_dir, timeout=per_run_timeout,
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    except subprocess.TimeoutExpired:
        return "timeout"
    except OSError:
        return None
    db = os.path.join(run_dir, "t.sqlite3")
    if not os.path.exists(db):
        return None
    try:
        con = sqlite3.connect(db)
        row = con.execute("SELECT Value FROM mgpusim_metrics "
                          "WHERE What='kernel_time' AND Location='Driver' LIMIT 1;").fetchone()
        con.close()
        return float(row[0]) if row and row[0] is not None else None
    except (sqlite3.Error, ValueError):
        return None


def fb_stream(benchmark, out_csv, ref):
    """Best-effort: publish new CSV rows to Firestore via fb_publish.py."""
    run_id = os.environ.get("FB_RUN_ID")
    if not run_id:
        return
    py = os.environ.get("FB_PY", sys.executable)
    try:
        subprocess.run([py, os.path.join(SCRIPT_DIR, "fb_publish.py"), "publish-points",
                        "--run-id", run_id, "--benchmark", benchmark,
                        "--csv", out_csv, "--ref", ref],
                       timeout=120, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    except (subprocess.SubprocessError, OSError):
        pass


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("benchmark")
    ap.add_argument("out")
    ap.add_argument("--ref", default=os.path.join(SCRIPT_DIR, "mi300a_ground_truth.csv"))
    args = ap.parse_args()

    spec = SPECS.get(args.benchmark)
    if spec is None:
        sys.exit(f"no sweep spec for benchmark '{args.benchmark}'")
    per_run = int(os.environ.get("PER_RUN_TIMEOUT", "1800"))
    budget = int(os.environ.get("SWEEP_TIMEOUT", "86400"))
    every = int(os.environ.get("FB_EVERY", "5"))
    os.environ.setdefault("CGO_ENABLED", "1")  # timing metrics via the SQLite recorder

    build_dir = tempfile.mkdtemp()
    work_dir = tempfile.mkdtemp()
    binary = os.path.join(build_dir, args.benchmark)
    print(f"Building {spec['sample']} ...")
    subprocess.run(["go", "build", "-o", binary, f"./amd/samples/{spec['sample']}/"],
                   cwd=REPO_ROOT, check=True)

    configs = load_configs(args.ref, args.benchmark)
    out_abs = os.path.abspath(args.out)
    with open(out_abs, "w", newline="") as f:
        f.write("benchmark,scaling_param_name,scaling_param_value,non_scaling_json,kernel_time_s\n")

    print(f"Sweeping {args.benchmark}: {len(configs)} configs "
          f"(per-run {per_run}s, budget {budget}s), cheapest-first...")
    start = time.time()
    import json as _json
    cache = {}
    ran = timed_out = failed = 0
    writer_f = open(out_abs, "a", newline="")
    writer = csv.writer(writer_f)
    for i, (real_ms, sname, sval, ns) in enumerate(configs, 1):
        if time.time() - start >= budget:
            print(f"  budget {budget}s reached after {i - 1} configs; stopping early.", flush=True)
            break
        sargs = sample_args(spec, sval, ns)
        key = tuple(sargs)
        cached = key in cache
        kt = cache[key] if cached else run_one(binary, sargs, work_dir, per_run)
        cache[key] = kt
        tag = f"[{i}/{len(configs)}] {sname}={sval} {_json.dumps(ns, separators=(',', ':'))}"
        if isinstance(kt, float):
            writer.writerow([args.benchmark, sname, sval, _json.dumps(ns, separators=(",", ":")), kt])
            writer_f.flush()
            ran += 1
            print(f"  ok      {tag}  sim={kt}s{' (cached)' if cached else ''}", flush=True)
            if ran % every == 0:
                fb_stream(args.benchmark, out_abs, args.ref)
        elif kt == "timeout":
            timed_out += 1
            print(f"  timeout {tag}  (> {per_run}s)", flush=True)
        else:
            failed += 1
            print(f"  FAIL    {tag}", flush=True)
    writer_f.close()
    fb_stream(args.benchmark, out_abs, args.ref)  # final flush
    print(f"Done {args.benchmark}: {ran} ok, {timed_out} timed out, {failed} failed. Wrote {out_abs}")
    # Surface a wholly-broken sweep (e.g. a sample that panics for every config) so
    # the matrix child fails and the run isn't silently marked completed with no
    # points for this benchmark.
    if ran == 0:
        sys.exit(f"ERROR: {args.benchmark} produced no metric for any config "
                 f"({timed_out} timed out, {failed} failed).")


if __name__ == "__main__":
    main()
