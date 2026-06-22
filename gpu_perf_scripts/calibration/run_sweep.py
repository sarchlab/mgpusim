#!/usr/bin/env python3
"""Generic MI300X calibration sweep for ONE microbenchmark.

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

from calib_common import parse_ns, xparse

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.abspath(os.path.join(SCRIPT_DIR, "..", ".."))
COMMON = ["-timing", "-arch", "cdna3", "-gpu", "mi300x", "-disable-rtm"]

# benchmark -> {sample dir, scaling-value flag, ground-truth-param -> CLI flag}.
#
# Only benchmarks whose sim sample is a FAITHFUL port of the ground-truth kernel
# are listed (same benchmark package). Ground-truth benchmarks with no sim runner
# (atomic_operations, cache_bandwidth, empty_kernel, pcie_bandwidth,
# tensor_core_throughput, warp_shuffle, altis_gups/particlefilter, chai_*,
# graph_*, lonestar_*, npb_cg/is/mg, parboil_histogram/spmv) or only a
# different-kernel sample (cuda_* vs nbody/matrixtranspose, shoc_* vs the generic
# fft/stencil2d/spmv/sort samples, polybench_atax/bicg, rodinia_bfs/kmeans/nw) are
# intentionally omitted -- calibrating against a different kernel is meaningless.
# Most samples expose only their scaling dimension; unsupported ground-truth
# params (block_size, precision, ...) are simply not passed (the sim curve then
# repeats across that dimension -- the honest result).
SPECS = {
    # --- Tier-1 microbenchmarks ---
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

    # --- Tier-2: altis ---
    "altis_cfd":            {"sample": "altis_cfd",            "scaling_flag": "-size",  "params": {}},
    "altis_raytracing":     {"sample": "altis_raytracing",    "scaling_flag": "-width",
                             "params": {"height": "-height", "spheres": "-spheres"}},

    # --- Tier-2: heteromark (sim samples aes/fir/pagerank are the heteromark ports) ---
    "heteromark_aes":       {"sample": "aes",                 "scaling_flag": "-length", "params": {}},
    "heteromark_fir":       {"sample": "fir",                 "scaling_flag": "-length", "params": {"num_taps": "-taps"}},
    "heteromark_pagerank":  {"sample": "pagerank",            "scaling_flag": "-node",   "params": {"pr_iterations": "-iterations"}},

    # --- Tier-2: npb ---
    "npb_ep":               {"sample": "npb_ep",              "scaling_flag": "-size",   "params": {}},

    # --- Tier-2: parboil ---
    "parboil_cutcp":        {"sample": "parboil_cutcp",       "scaling_flag": "-num-atoms",
                             "params": {"grid_spacing": "-grid-spacing", "cutoff_radius": "-cutoff"}},
    "parboil_lbm":          {"sample": "parboil_lbm",         "scaling_flag": "-grid",
                             "params": {"num_timesteps": "-timesteps", "tau": "-tau"}},
    "parboil_sgemm":        {"sample": "parboil_sgemm",       "scaling_flag": "-size",   "params": {}},
    # parboil_stencil hangs in CDNA3 timing mode (known timing-core bug); kept here
    # for manual runs but excluded from the CI matrix so it can't burn a runner.
    "parboil_stencil":      {"sample": "parboil_stencil",     "scaling_flag": "-size",
                             "params": {"num_timesteps": "-timesteps"}},

    # --- Tier-2: polybench ---
    "polybench_2dconv":      {"sample": "polybench_2dconv",      "scaling_flag": "-size", "params": {}},
    "polybench_2mm":         {"sample": "polybench_2mm",         "scaling_flag": "-size", "params": {}},
    "polybench_3dconv":      {"sample": "polybench_3dconv",      "scaling_flag": "-size", "params": {"filter_size": "-filter-size"}},
    "polybench_3mm":         {"sample": "polybench_3mm",         "scaling_flag": "-size", "params": {}},
    "polybench_correlation": {"sample": "polybench_correlation", "scaling_flag": "-size", "params": {}},
    "polybench_fdtd2d":      {"sample": "polybench_fdtd2d",      "scaling_flag": "-size", "params": {"tmax": "-tmax"}},
    "polybench_gemm":        {"sample": "polybench_gemm",        "scaling_flag": "-size", "params": {}},
    "polybench_gramschmidt": {"sample": "polybench_gramschmidt", "scaling_flag": "-m",    "params": {"n": "-n"}},
    "polybench_jacobi2d":    {"sample": "polybench_jacobi2d",    "scaling_flag": "-size", "params": {"tsteps": "-tsteps"}},
    "polybench_mvt":         {"sample": "polybench_mvt",         "scaling_flag": "-size", "params": {}},
    "polybench_syr2k":       {"sample": "polybench_syr2k",       "scaling_flag": "-size", "params": {"inner_size": "-inner-size"}},

    # --- Tier-2: rodinia ---
    "rodinia_backprop":   {"sample": "rodinia_backprop",   "scaling_flag": "-input",     "params": {"hidden": "-hidden", "output": "-output"}},
    "rodinia_gaussian":   {"sample": "rodinia_gaussian",   "scaling_flag": "-size",      "params": {}},
    "rodinia_hotspot":    {"sample": "rodinia_hotspot",    "scaling_flag": "-size",      "params": {"num_iterations": "-iterations"}},
    "rodinia_hotspot3d":  {"sample": "rodinia_hotspot3d",  "scaling_flag": "-size",      "params": {"amb_temp": "-amb-temp", "num_iterations": "-iterations"}},
    "rodinia_lavamd":     {"sample": "rodinia_lavamd",     "scaling_flag": "-num-boxes", "params": {"particles_per_box": "-particles-per-box"}},
    "rodinia_lud":        {"sample": "rodinia_lud",        "scaling_flag": "-size",      "params": {}},
    "rodinia_pathfinder": {"sample": "rodinia_pathfinder", "scaling_flag": "-cols",      "params": {"rows": "-rows"}},
    "rodinia_srad":       {"sample": "rodinia_srad",       "scaling_flag": "-size",      "params": {"num_iterations": "-iterations"}},

    # --- Tier-2: tango ---
    "tango_binomial_options": {"sample": "tango_binomial_options", "scaling_flag": "-options", "params": {"steps": "-steps"}},
    "tango_blackscholes":     {"sample": "tango_blackscholes",     "scaling_flag": "-size",    "params": {}},
}


def sim_cost(scaling_value, ns):
    """Rough proxy for how expensive a config is to SIMULATE: total dynamic work
    ~= grid size x per-thread iterations = num_blocks * threads_per_block * scaling.

    Ordering by real HARDWARE time is work-blind -- HW kernel time floors at
    ~0.5 ms regardless of size, so the "cheapest real" configs include ones that
    are enormous to cycle-accurately simulate (e.g. 4096 blocks x 64 threads).
    Those burn the full PER_RUN_TIMEOUT producing nothing and starve the sweep
    (the dashboard stays empty). Ordering by simulated work instead streams the
    genuinely fast configs first; the expensive tail (which may time out) runs
    last. Benchmarks that don't pass threads_per_block collapse those configs via
    memoization, so the proxy still orders the distinct sims correctly.
    """
    try:
        s = abs(xparse(scaling_value))
    except (ValueError, TypeError):
        s = 1
    return s * ns.get("num_blocks", 1) * ns.get("threads_per_block", 1)


def load_configs(ref, benchmark):
    """[(real_ms, scaling_name, scaling_value, non_scaling_dict)] cheapest-to-SIMULATE first."""
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
    out.sort(key=lambda c: sim_cost(c[2], c[3]))
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
    ap.add_argument("--ref", default=os.path.join(SCRIPT_DIR, "mi300x_ground_truth.csv"))
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
          f"(per-run {per_run}s, budget {budget}s), cheapest-to-simulate first...")
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
