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
Env: PER_RUN_TIMEOUT (s, default 3600), SWEEP_TIMEOUT (s, default 86400),
     FB_RUN_ID, FB_EVERY (default 5), GOOGLE_APPLICATION_CREDENTIALS, FB_PY.
"""
import argparse
import csv
import heapq
from contextlib import closing
import json
import math
import os
from pathlib import Path
import re
import subprocess
import sqlite3
import sys
import tempfile
import time

from calib_common import parse_ns, xparse
from export_live_report import parse_parameters

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.abspath(os.path.join(SCRIPT_DIR, "..", ".."))
COMMON = ["-timing", "-arch", "cdna3", "-gpu", "mi300x", "-disable-rtm", "-verify"]

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
#
# KERNEL-FIDELITY CAVEATS (from diffing MGPUSim native kernels vs current
# gpu_benchmarks .hip; the recent gpu_benchmarks updates are harness-only, so
# these are pre-existing port divergences, not staleness):
#  - heteromark_{aes,fir,pagerank}: GENUINE kernel drift -> DISABLED above.
#  - memory_bandwidth: ground truth is host hipMemcpy D2D (no kernel); the sim
#    proxies it with a copy kernel -- same D2D direction, different mechanism.
#  - fp64_throughput, polybench_gramschmidt, rodinia_hotspot3d, npb_ep,
#    parboil_cutcp: intentional, documented CDNA3/gfx942 emulator workarounds
#    (memory-sourced operands / serial-reduction-instead-of-atomics / z-loop /
#    per-thread-output / dropped-unused-param). Numerically equivalent, but the
#    timing profile of the adapted step is not a faithful match -- read with care.
#  - shared_mem_bandwidth: kernel logic matches but LDS footprint differs
#    (gpu_benchmarks 48 KB vs sim 2 KB) -> different occupancy; consider aligning.
SPECS = {
    # --- Tier-1 microbenchmarks ---
    "fp32_throughput":      {"sample": "fp32_throughput",      "scaling_flag": "-fmas",
                             "params": {"num_blocks": "-num-blocks", "threads_per_block": "-threads-per-block"}},
    "fp16_throughput":      {"sample": "fp16_throughput",      "scaling_flag": "-fmas-per-thread",
                             "params": {"num_blocks": "-num-blocks", "threads_per_block": "-threads-per-block"}},
    "fp64_throughput":      {"sample": "fp64_throughput",      "scaling_flag": "-fmas-per-thread",
                             "params": {"num_blocks": "-num-blocks", "threads_per_block": "-threads-per-block"}},
    "int32_throughput":     {"sample": "int32_throughput",     "scaling_flag": "-mads",
                             "params": {"num_blocks": "-blocks", "threads_per_block": "-threads-per-block"}},
    # -size is a float32 ELEMENT count; ground truth scales by buffer_size_mb, so
    # convert MiB -> float32 elements (1 MiB / 4 bytes).
    "memory_bandwidth":     {"sample": "memory_bandwidth",     "scaling_flag": "-size",
                             "params": {}, "scaling_xform": lambda v: int(v) * 1024 * 1024 // 4},
    # The original "shared_mem_bandwidth" kernel was actually a single-accumulator
    # dependent chain (measures LDS latency, not bandwidth); it is now
    # shared_mem_latency and keeps the existing ground truth.
    # block_size is now a real swept param (the kernel matches the HW: 48 KB
    # LDS, block size as a runtime arg). access_pattern is intentionally NOT
    # passed: the conflict kernel's runtime arithmetic still trips CDNA3-emulator
    # gaps, so the sim runs no_conflict only for now (the sample default);
    # conflict ground-truth rows dedup onto the no_conflict sim until the
    # emulator is hardened for the conflict kernel.
    "shared_mem_latency":   {"sample": "shared_mem_latency",   "scaling_flag": "-inner-iters",
                             "params": {"num_blocks": "-num-blocks", "block_size": "-block-size"}},
    # Multi-accumulator shared_mem_bandwidth kernel (independent accumulators,
    # many LDS reads in flight -> measures throughput, not latency). block_size is
    # now a real swept param: the kernel matches the HW (32 KB LDS footprint, block
    # size a runtime arg, reads masked to block_size*UNROLL). access_pattern is NOT
    # passed -- the conflict kernel hangs the CDNA3 emulator (same as
    # shared_mem_latency), so the sim runs no_conflict only until that is fixed.
    "shared_mem_bandwidth": {"sample": "shared_mem_bandwidth", "scaling_flag": "-inner-iters",
                             "params": {"num_blocks": "-num-blocks", "block_size": "-block-size"}},
    # Pass the real benchmark's params so the sim builds the same cacheline-strided
    # chain and derives the same timed-access count (measure_laps x lines, clamped).
    "cache_latency":        {"sample": "cache_latency",        "scaling_flag": "-array-bytes",
                             "params": {"cacheline_bytes": "-cacheline-bytes",
                                        "measure_laps": "-measure-laps", "rng_seed": "-seed"}},
    # Launch-overhead microbench. The GT's kernel_ms_mean for empty_kernel is the
    # real launch_sync_us (per-launch round-trip; see summarize_ground_truth.py),
    # so the sim's single empty launch (kernel_time = the model's per-launch
    # overhead) is the comparable quantity.
    "empty_kernel":         {"sample": "empty_kernel",         "scaling_flag": "-num-blocks",
                             "params": {"block_size": "-block-size"}},

    # --- Tier-2: altis ---
    "altis_cfd":            {"sample": "altis_cfd",            "scaling_flag": "-size",  "params": {}},
    "altis_raytracing":     {"sample": "altis_raytracing",    "scaling_flag": "-width",
                             "params": {"height": "-height", "spheres": "-spheres"}},

    # --- Tier-2: heteromark --- DISABLED pending re-port.
    # The MGPUSim aes/fir/pagerank ports run materially DIFFERENT kernel code than
    # the current gpu_benchmarks ground truth (verified by kernel diff): pagerank
    # is CSR SpMV warp-per-row in the sim vs thread-per-vertex power-method
    # upstream (different algorithm); aes uses a different AES-256 formulation; fir
    # uses a history buffer vs shared-mem coefficients. Calibrating against these
    # would compare different kernels. Re-extract the kernels from current
    # gpu_benchmarks tier2/{heteromark_aes,heteromark_fir,heteromark_pagerank}.hip
    # and recompile the HSACO before re-enabling. Mappings kept for that work:
    #   "heteromark_aes":      {"sample": "aes",      "scaling_flag": "-length", "params": {}},
    #   "heteromark_fir":      {"sample": "fir",      "scaling_flag": "-length", "params": {"num_taps": "-taps"}},
    #   "heteromark_pagerank": {"sample": "pagerank", "scaling_flag": "-node",   "params": {"pr_iterations": "-iterations"}},

    # --- Tier-2: npb ---
    # npb_ep DISABLED: real ep_kernel is atomic-contention-bound
    # (atomicAdd into a ~10-bin histogram -> serializes -> scales ~linearly with
    # N). The CDNA3/gfx942 emulator has no global atomics, so the port uses
    # per-thread output (zero contention) and the sim is flat across the whole
    # size sweep -- the trend is unrecoverable here. Re-enable once atomics land.
    # See https://github.com/sarchlab/mgpusim/issues/278
    # "npb_ep":               {"sample": "npb_ep",              "scaling_flag": "-size",   "params": {}},

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
    # gramschmidt HANGS in CDNA3 timing mode (stalls even at n=1, in
    # gram_norm_finish/gram_normalize -- a timing-core stall, not the serial
    # reduction). Excluded from the CI matrix. ALSO atomic-dependent: the real
    # gram_norm does atomicAdd(nrm_buf, ...), which the CDNA3 emulator can't model
    # (see https://github.com/sarchlab/mgpusim/issues/278), so it stays dropped on
    # both counts. Kept here only for manual runs.
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
    cost = s * ns.get("num_blocks", 1) * ns.get("threads_per_block", 1)
    # Iteration-count params multiply the per-element work, so fold them in too:
    # this keeps the ordering (and the early-stop) monotonic in real sim time when
    # two configs share a scaling value but differ in iteration count.
    for k in ("num_iterations", "iterations", "tsteps", "tmax", "num_timesteps",
              "steps", "passes", "trials", "measure_laps", "inner_iters"):
        v = ns.get(k)
        if v is None:
            continue
        try:
            cost *= max(1, abs(int(v)))
        except (ValueError, TypeError):
            pass
    return cost


def series_key(config):
    _, axis, _, params = config
    return axis, json.dumps(params, sort_keys=True, separators=(",", ":"))


def ordered_configs(configs):
    """Prefer low-cost work while visiting each series in increasing X order."""
    series = {}
    for config in configs:
        series.setdefault(series_key(config), []).append(config)
    groups = [iter(sorted(group, key=lambda c: xparse(c[2]))) for group in series.values()]
    ready = []
    for i, group in enumerate(groups):
        config = next(group)
        heapq.heappush(ready, (sim_cost(config[2], config[3]), i, config))
    while ready:
        _, i, config = heapq.heappop(ready)
        yield config
        following = next(groups[i], None)
        if following is not None:
            heapq.heappush(ready, (sim_cost(following[2], following[3]), i, following))


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
    return list(ordered_configs(out))


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


def recorded_kernel_time(db):
    if not db.is_file():
        return None
    try:
        with closing(sqlite3.connect(db.as_uri() + "?mode=ro", uri=True)) as con:
            rows = con.execute(
                "SELECT Value FROM mgpusim_metrics WHERE What='kernel_time' AND Location='Driver'"
            ).fetchall()
        if len(rows) == 1:
            value = float(rows[0][0])
            if math.isfinite(value) and value > 0:
                return value
    except (sqlite3.Error, TypeError, ValueError):
        pass
    return None


def verification_failure(log):
    reason = None
    for line in log:
        match = re.match(r"^panic: Benchmark\[\d+\]: (.+)", line)
        if match:
            reason = match[1].strip()
    if reason and (
        re.search(r"\b(mismatch(?:es)?|verif(?:ication|ied))\b", reason, re.IGNORECASE)
        or ("expected" in reason.lower() and "got" in reason.lower())
    ):
        return reason
    return None


def run_one(binary, args, work_dir, per_run_timeout):
    """Capture the exit status and any persisted, usable Driver kernel time."""
    with tempfile.TemporaryDirectory(dir=work_dir) as directory:
        run_dir = Path(directory)
        cmd = [binary, *COMMON, *args, "-metric-file-name", "t"]
        try:
            with (run_dir / "stdout.log").open("w+") as log:
                process = subprocess.run(cmd, cwd=run_dir, timeout=per_run_timeout,
                                         stdout=log, stderr=subprocess.STDOUT)
                timing = recorded_kernel_time(run_dir / "t.sqlite3")
                if process.returncode == 0:
                    if timing is not None:
                        return {"status": "complete", "kernel_time_s": timing}
                    return {"status": "failed", "error": "Driver kernel time was not recorded"}
                log.seek(0)
                mismatch = verification_failure(log)
                if mismatch:
                    result = {"status": "failed", "failure_kind": "verification_failed",
                              "failure_reason": mismatch}
                    if timing is not None:
                        result["kernel_time_s"] = timing
                    return result
                return {"status": "failed", "error": f"Simulator exited with status {process.returncode}"}
        except subprocess.TimeoutExpired:
            return {"status": "timeout", "error": f"Simulator exceeded {per_run_timeout} seconds"}
        except OSError as error:
            return {"status": "failed", "error": str(error)}


def sweep_points(configs, command_for, execute, stop_on_timeout=True):
    cutoffs = {}
    cache = {}
    for index, config in enumerate(configs):
        _, axis, value, params = config
        x = xparse(value)
        series = series_key(config)
        if stop_on_timeout and series in cutoffs and x > cutoffs[series]:
            yield index, {"status": "skipped_after_timeout",
                          "error": f"A smaller point timed out at {axis}={cutoffs[series]}."}
            continue
        command = tuple(command_for(config))
        result = cache.get(command)
        if result is None:
            result = execute(index, command)
            if result["status"] != "timeout":
                cache[command] = result
        if result["status"] == "timeout":
            cutoffs.setdefault(series, x)
        yield index, result


def write_snapshot(path, points, start, live=True):
    if path is None:
        return
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_text(json.dumps({
        "started_at": start, "updated_at": time.time(), "cases": points, "live": live,
    }, allow_nan=False) + "\n")
    temporary.replace(path)


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
    ap.add_argument("--snapshot", type=Path, help="write live status and timing JSON for the report")
    ap.add_argument("--plan", type=Path, help="JSON list of explicit calibration cases")
    ap.add_argument("--reference", type=Path, help="JSON mapping plan case IDs to hardware measurements")
    args = ap.parse_args()
    if bool(args.plan) != bool(args.reference):
        ap.error("--plan and --reference must be provided together")

    spec = SPECS.get(args.benchmark)
    if spec is None and args.plan is None:
        ap.error(f"no sweep spec for benchmark '{args.benchmark}'")
    per_run = int(os.environ.get("PER_RUN_TIMEOUT", "3600"))
    budget = int(os.environ.get("SWEEP_TIMEOUT", "86400"))
    every = int(os.environ.get("FB_EVERY", "5"))
    stop_on_timeout = os.environ.get("SWEEP_STOP_ON_TIMEOUT", "1") != "0"
    os.environ.setdefault("CGO_ENABLED", "1")

    if args.plan:
        planned = [row for row in json.loads(args.plan.read_text())
                   if row["benchmark"] == args.benchmark and not row.get("unsupported_reason")]
        samples = {row["sample"] for row in planned}
        if len(samples) != 1:
            ap.error("--plan must specify one sample for the selected benchmark")
        sample = samples.pop()
        if not re.fullmatch(r"[a-z0-9_]+", sample):
            ap.error("invalid sample directory")
        refs = json.loads(args.reference.read_text())
        commands = {}
        cases = []
        for row in planned:
            measurement = refs[str(row["case"])]
            real = measurement["hardware_ms"]
            params = parse_parameters(row["series"])
            config = (real if real is not None else math.inf, row["axis"], str(row["x"]), params)
            key = (*series_key(config), config[2])
            if key in commands:
                ap.error(f"duplicate configuration in plan: {key}")
            cases.append(config)
            commands[key] = row["args"]
        configs = list(ordered_configs(cases))
        kind = refs[str(planned[0]["case"])]["kind"]

        def command_for(config):
            return commands[(*series_key(config), config[2])]
    else:
        sample = spec["sample"]
        configs = load_configs(args.ref, args.benchmark)
        kind = "transfer" if args.benchmark == "memory_bandwidth" else "kernel"

        def command_for(config):
            return sample_args(spec, config[2], config[3])

    out_abs = os.path.abspath(args.out)
    points = [
        {"benchmark": args.benchmark, "axis": axis, "x": xparse(value),
         "non_scaling_params": params, "kind": kind, "status": "queued",
         "hardware_ms": real_ms if math.isfinite(real_ms) else None}
        for real_ms, axis, value, params in configs
    ]

    completed = 0
    with (tempfile.TemporaryDirectory() as directory,
          open(out_abs, "w", newline="") as output):
        writer = csv.writer(output)
        writer.writerow(["benchmark", "scaling_param_name", "scaling_param_value",
                         "non_scaling_json", "kernel_time_s"])
        output.flush()
        binary = os.path.join(directory, args.benchmark)
        print(f"Building {sample} ...", flush=True)
        subprocess.run(["go", "build", "-o", binary, f"./amd/samples/{sample}/"],
                       cwd=REPO_ROOT, check=True)

        start = time.time()
        deadline = time.monotonic() + budget
        write_snapshot(args.snapshot, points, start)
        def execute(index, flags):
            points[index]["status"] = "running"
            write_snapshot(args.snapshot, points, start)
            return run_one(binary, flags, directory, per_run)

        results = sweep_points(configs, command_for, execute, stop_on_timeout)
        for _ in configs:
            if time.monotonic() >= deadline:
                break
            index, result = next(results)
            point = points[index]
            point.update(result)
            write_snapshot(args.snapshot, points, start)
            _, axis, value, params = configs[index]
            print(f"[{index + 1}/{len(configs)}] {axis}={value} {params}: {point['status']}",
                  flush=True)
            if point["status"] == "complete":
                writer.writerow([args.benchmark, axis, value, json.dumps(params), point["kernel_time_s"]])
                output.flush()
                completed += 1
                if not args.plan and completed % every == 0:
                    fb_stream(args.benchmark, out_abs, args.ref)

        write_snapshot(args.snapshot, points, start, live=False)

    if not args.plan:
        fb_stream(args.benchmark, out_abs, args.ref)
    if completed == 0:
        sys.exit(f"ERROR: {args.benchmark} produced no verified timing.")


if __name__ == "__main__":
    main()
