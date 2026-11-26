"""Plan, run and report release calibration against recorded MI300X dispatches."""

import argparse
import gzip
import json
import math
import re
import sqlite3
import subprocess
import time
from contextlib import closing
from datetime import UTC, datetime
from pathlib import Path

import report_db

BUDGET_SECONDS = 5.5 * 60 * 60
# Database retries end before the 360-minute job timeout, leaving time for
# setup before the series starts and for the artifact uploads after it.
WRITE_DEADLINE_SECONDS = 345 * 60
SERIES_BUDGET_SKIP_REASON = "Series budget expired before this configuration started."
# cr.observations.reason holds at most this many UTF-8 bytes.
REASON_BYTES = 1000
RUN_KEY = re.compile(r"[A-Za-z0-9_-][A-Za-z0-9._-]{0,199}")
SITE_URL = "https://sarchlab-calibration-report.netlify.app/?run="
COMMON = [
    "-arch=cdna3",
    "-gpu=mi300x",
    "-timing",
    "-verify",
    "-disable-rtm",
    "-report-kernel-dispatches",
    "-metric-file-name=timing",
]
UNITS = {
    "array_bytes": "B",
    "buffer_size_mb": "MiB",
    "cacheline_bytes": "B",
    "grid_spacing": "Å",
    "amb_temp": "°C",
}


def read_reference(path):
    with gzip.open(path, "rt") as file:
        return json.load(file)


def matrix_rows(reference, benchmark):
    rows = [
        {"id": i, "benchmark": row["benchmark"], "sample": row["sample"]}
        for i, row in enumerate(reference["series"])
        if not benchmark or row["benchmark"] == benchmark
    ]
    if not 1 <= len(rows) <= 256:
        raise ValueError(f"Expected 1–256 calibration jobs, got {len(rows)}")
    for row in rows:
        if not all(
            re.fullmatch(r"[a-z][a-z0-9_]*", row[key])
            for key in ("sample", "benchmark")
        ):
            raise ValueError(f"Invalid benchmark or sample path for series {row['id']}")
    return rows


def timestamp():
    return datetime.now(UTC).isoformat(timespec="seconds")


def atomic_json(path, data):
    pending = path.with_suffix(path.suffix + ".tmp")
    pending.write_text(json.dumps(data, separators=(",", ":"), allow_nan=False) + "\n")
    pending.replace(path)


def simulated_launches(path):
    if not path.is_file():
        return []
    with closing(sqlite3.connect(path.resolve().as_uri() + "?mode=ro", uri=True)) as db:
        locations = db.execute(
            "SELECT DISTINCT Location FROM mgpusim_kernel_dispatches"
        ).fetchall()
        if len(locations) != 1:
            return []
        return db.execute(
            """SELECT Kernel, GridX, GridY, GridZ, GroupX, GroupY, GroupZ,
                      (EndPS - StartPS) / 1000.0
               FROM mgpusim_kernel_dispatches ORDER BY StartPS, TaskID"""
        ).fetchall()


def reference_clock_flags(reference):
    clock = reference.get("gpu_clock_mhz")
    if clock is not None:
        if type(clock) is not int or not 1 <= clock <= 1_000_000:
            raise ValueError(f"Invalid reference GPU clock in MHz: {clock}")
        return (f"-mi300x-clock-mhz={clock}",)
    return ()


def run_case(binary, case, directory, remaining, *, clock_flags=()):
    directory.mkdir()
    with (directory / "stdout.log").open("w+") as log:
        try:
            process = subprocess.run(
                [str(binary), *COMMON, *clock_flags, *case["args"]],
                cwd=directory,
                stdout=log,
                stderr=subprocess.STDOUT,
                timeout=remaining,
                check=False,
            )
        except subprocess.TimeoutExpired:
            return {
                "status": "timeout",
                "reason": "Series budget expired during this configuration.",
            }
        try:
            launches = simulated_launches(directory / "timing.sqlite3")
        except sqlite3.Error:
            launches = []
        if process.returncode == 0 and launches:
            return {"status": "complete", "launches": launches}
        log.seek(0)
        reason = next((line.strip() for line in log if line.startswith("panic:")), None)
        if (
            reason
            and re.match(r"panic: Benchmark\[\d+\]:", reason)
            and (
                re.search(
                    r"\b(mismatch(?:es)?|verif(?:ication|ied))\b", reason, re.IGNORECASE
                )
                or ("expected" in reason.lower() and "got" in reason.lower())
            )
        ):
            return {
                "status": "verification_failed",
                "reason": reason,
                "launches": launches,
            }
        return {
            "status": "failed",
            "reason": reason
            or f"No usable dispatch trace (exit {process.returncode}).",
        }


def kernel_names(rows):
    names = sorted({row[0] for row in rows})
    if not names:
        return {}
    process = subprocess.run(
        ["c++filt"],
        input="".join(name.removesuffix(".kd") + "\n" for name in names),
        text=True,
        capture_output=True,
        check=True,
    )
    return dict(zip(names, process.stdout.splitlines(), strict=True))


def base_kernel_name(name):
    # ROCprof includes a signature for C symbols that have no mangled signature.
    return name.removesuffix(".kd").split("(", 1)[0].removeprefix("void ").strip()


def same_kernel(hardware, simulated):
    if "(" in hardware and "(" in simulated:
        return hardware.removeprefix("void ") == simulated.removeprefix("void ")
    return base_kernel_name(hardware) == base_kernel_name(simulated)


# These code objects export C symbols; ROCprof records their C++ namespaces.
KERNEL_ALIASES = {
    "altis_cfd": {
        "compute_step_factor_kernel": "cfd_device::compute_step_factor_kernel",
        "compute_flux_kernel": "cfd_device::compute_flux_kernel",
        "rk_update_kernel": "cfd_device::rk_update_kernel",
    },
    "parboil_lbm": {"lbm_collide_stream_kernel": "lbm::lbm_collide_stream_kernel"},
}


def same_dispatch(hardware, simulated, demangled, *, benchmark=""):
    simulated_name = demangled[simulated[0]]
    matches = same_kernel(hardware[0], simulated_name) or (
        KERNEL_ALIASES.get(benchmark, {}).get(simulated_name) == base_kernel_name(hardware[0])
    )
    return matches and list(simulated[1:-1]) == hardware[1:-1]


def cfd_launch_groups(cases):
    # CFD records ten timesteps, each with a copy, a factor calculation, and
    # three Runge–Kutta stages. Validate the whole hardware series before
    # labelling it so that labels are known before any simulation runs.
    stage = ("cfd_device::compute_flux_kernel", "cfd_device::rk_update_kernel")
    step = ("copy_variables_kernel", "cfd_device::compute_step_factor_kernel") + stage * 3
    aliases = KERNEL_ALIASES["altis_cfd"]
    for case in cases:
        hardware = case["launches"]
        names = (base_kernel_name(row[0]) for row in hardware)
        if tuple(aliases.get(name, name) for name in names) != step * 10:
            return ()
        if any(row[1:-1] != hardware[0][1:-1] for row in hardware):
            return ()

    # Keep observed first-timestep effects separate and group the similar
    # stage 1 and 2 timings in the MI300X reference.
    steady = (
        "copy",
        "step_factor",
        "flux_stage_0",
        "update_stage_0",
        "flux_stages_1_2",
        "update_stages_1_2",
        "flux_stages_1_2",
        "update_stages_1_2",
    )
    first = ("copy_first", "step_factor_first", "flux_stage_0_first", *steady[3:])
    return first + steady * 9


ITERATIVE_LAUNCH_PATTERNS = {
    "parboil_lbm": ("num_timesteps", ("lbm::lbm_collide_stream_kernel",)),
    "polybench_fdtd2d": (
        "tmax", ("fdtd_update_ex", "fdtd_update_ey", "fdtd_update_hz"),
    ),
    "polybench_jacobi2d": ("tsteps", ("jacobi2d_kernel", "jacobi2d_copy_kernel")),
}


def iterative_launch_groups(series):
    benchmark = series["benchmark"]
    parameters = series["parameters"]
    tail = False
    if benchmark == "rodinia_pathfinder":
        rows = parameters.get("rows")
        height = parameters.get("pyramid_height")
        if type(rows) is not int or type(height) is not int or rows < 2 or height < 1:
            return ()
        steps, remainder = divmod(rows - 1, height)
        tail = remainder != 0
        count = steps + tail
        kernels = ("dynproc_kernel",)
    else:
        step_count, kernels = ITERATIVE_LAUNCH_PATTERNS[benchmark]
        count = parameters.get(step_count)

    if type(count) is not int or count < 2:
        return ()

    expected = kernels * count
    phases = len(kernels)
    aliases = KERNEL_ALIASES.get(benchmark, {})
    for case in series["cases"]:
        hardware = case["launches"]
        names = (base_kernel_name(row[0]) for row in hardware)
        if tuple(aliases.get(name, name) for name in names) != expected:
            return ()
        if any(row[1:-1] != hardware[i % phases][1:-1] for i, row in enumerate(hardware)):
            return ()

    labels = tuple(kernel.rsplit("::", 1)[-1] for kernel in kernels)
    groups = tuple(f"{label}_first" for label in labels) + labels * (count - 1)
    if tail:
        groups = groups[:-1] + (f"{labels[0]}_tail",)
    return groups


def launch_groups(series):
    # Labels depend only on the hardware reference, so they are known before
    # any simulation runs.
    benchmark = series["benchmark"]
    if benchmark == "altis_cfd":
        return cfd_launch_groups(series["cases"])
    if benchmark == "rodinia_pathfinder" or benchmark in ITERATIVE_LAUNCH_PATTERNS:
        return iterative_launch_groups(series)
    return None


def series_launch_groups(series, definition):
    return launch_groups(series) if "launch_group" in definition["params"] else ()


def run_key(tag, run_id):
    key = f"mgpusim-mi300x-{tag}" if tag else f"mgpusim-mi300x-preview-{run_id}"
    if not RUN_KEY.fullmatch(key):
        raise ValueError(f"Invalid calibration report run key {key!r}")
    return key


def report_header(version, revision):
    return {
        "title": f"MGPUSim {version} MI300X calibration",
        "description": f"Simulation commit {revision}. Each chart compares one dispatch "
        "with its median MI300X timing from GPU Benchmarks v1.1.",
        "live": True,  # Required by the schema; the database derives it from the run state.
        "groups": {"mi300x": {"name": "AMD MI300X"}},
    }


def benchmark_definitions(series_list):
    benchmarks = {}
    for series in series_list:
        bench, axis, fixed = series["benchmark"], series["axis"], series["parameters"]
        params = {key: {"unit": UNITS.get(key, "")} for key in (axis, "launch", *fixed)}
        definition = benchmarks.setdefault(
            bench,
            {
                "scaling_param": axis,
                "params": params,
                "values": {
                    "mi300x": {
                        "name": "MI300X kernel time (median)",
                        "unit": "ns",
                        "role": "hardware_measurement",
                    },
                    "mgpusim": {
                        "name": "MGPUSim kernel time",
                        "unit": "ns",
                        "role": "simulator_output",
                    },
                },
            },
        )
        definition["params"].update(params)
    # Parameters are declared per benchmark. Keep every series of a benchmark
    # ungrouped if any of its reference traces is inconsistent.
    grouped = {}
    for series in series_list:
        groups = launch_groups(series)
        if groups is not None:
            bench = series["benchmark"]
            grouped[bench] = grouped.get(bench, True) and bool(groups)
    for bench, declared in grouped.items():
        if declared:
            benchmarks[bench]["params"]["launch_group"] = {"unit": ""}
    return benchmarks


def queued_progress(series_list):
    total = sum(len(series["cases"]) for series in series_list)
    now = timestamp()
    return {
        "started_at": now,
        "updated_at": now,
        "total": total,
        "running": 0,
        "queued": total,
        "completed": 0,
        "unsuccessful": 0,
    }


def paired_timings(bench, hardware, simulated, demangled):
    # The cache-latency sample launches a primer before the measured
    # pointer chase. The hardware reference records only the latter.
    if (
        bench == "cache_latency"
        and len(hardware) == 1
        and len(simulated) == 2
        and base_kernel_name(hardware[0][0]) == "pointer_chase_kernel"
        and same_dispatch(hardware[0], simulated[0], demangled)
    ):
        simulated = simulated[1:]
    paired = [None] * len(hardware)
    if len(simulated) == len(hardware):
        for index, (real, sim) in enumerate(zip(hardware, simulated)):
            if (
                same_dispatch(real, sim, demangled, benchmark=bench)
                and math.isfinite(sim[-1])
                and sim[-1] > 0
            ):
                paired[index] = sim[-1]
    return paired


def bounded_reason(reason):
    data = reason.encode()
    if len(data) <= REASON_BYTES:
        return reason
    ellipsis = "…"
    kept = data[:REASON_BYTES - len(ellipsis.encode())].decode(errors="ignore")
    return kept + ellipsis


def case_points(series, case, result, launch_groups=()):
    bench, fixed, state = series["benchmark"], series["parameters"], result["status"]
    hardware = case["launches"]
    simulated = result.get("launches", [])
    paired = paired_timings(bench, hardware, simulated, kernel_names(simulated))
    points = []
    for index, real in enumerate(hardware):
        point = {
            "benchmark": bench,
            "group": "mi300x",
            "x": case["x"],
            "non_scaling_params": {
                "launch": f"#{index + 1} {base_kernel_name(real[0])}",
                **fixed,
            },
            "values": {"mi300x": real[-1], "mgpusim": paired[index]},
        }
        if launch_groups:
            point["non_scaling_params"]["launch_group"] = launch_groups[index]
        if state == "complete" and paired[index] is None:
            point["statuses"] = {
                "mgpusim": {
                    "state": "unsupported",
                    "reason": "Hardware and simulated kernel names, order or launch geometry differ.",
                }
            }
        elif state != "complete":
            status = {"state": state, "reason": bounded_reason(result["reason"])}
            if state == "unsupported" and status["reason"] == SERIES_BUDGET_SKIP_REASON:
                status["symbol"] = "↷"
            point["statuses"] = {"mgpusim": status}
        points.append(point)
    return points


def run_series(series, binary, output, key, launch_groups=(), budget=BUDGET_SECONDS, start=None, clock_flags=()):
    start = time.monotonic() if start is None else start
    deadline = start + budget
    write_deadline = start + WRITE_DEADLINE_SECONDS
    output.mkdir(parents=True, exist_ok=True)
    runs = output / "runs"
    runs.mkdir()
    results = {}
    started_at = timestamp()
    status_file = output / "results.json"
    atomic_json(status_file, {"started_at": started_at, "cases": results})
    for case in sorted(series["cases"], key=lambda case: case["x"]):
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            result = {
                "status": "unsupported",
                "reason": SERIES_BUDGET_SKIP_REASON,
            }
        else:
            directory = runs / str(case["case"])
            result = run_case(binary.resolve(), case, directory, remaining, clock_flags=clock_flags)
            # The dispatch rows are already in the results artifact.
            for file in directory.glob("timing.sqlite3*"):
                file.unlink()
        results[str(case["case"])] = result
        atomic_json(status_file, {"started_at": started_at, "cases": results})
        report_db.put_points(
            key, case_points(series, case, result, launch_groups), deadline=write_deadline
        )
        result["published"] = True
        atomic_json(status_file, {"started_at": started_at, "cases": results})
        print(f"{case['x']}: {result['status']}", flush=True)


def final_progress(series_list, results, started_at):
    total = completed = 0
    for series_id, series in series_list.items():
        cases = results.get(series_id, {}).get("cases", {})
        for case in series["cases"]:
            total += 1
            result = cases.get(str(case["case"]), {})
            # A case counts only once the database has its points.
            if result.get("status") != "complete" or not result.get("published"):
                continue
            simulated = result["launches"]
            paired = paired_timings(
                series["benchmark"], case["launches"], simulated, kernel_names(simulated)
            )
            completed += None not in paired
    return {
        "started_at": started_at or None,
        "updated_at": timestamp(),
        "total": total,
        "running": 0,
        "queued": 0,
        "completed": completed,
        "unsuccessful": total - completed,
    }


def selected_series(reference, benchmark):
    return {row["id"]: reference["series"][row["id"]] for row in matrix_rows(reference, benchmark)}


def plan(reference, args):
    report_db.dsn()  # Fail before any work when the secret is missing.
    key = run_key(args.tag, args.run_id)
    rows = matrix_rows(reference, args.benchmark)
    series_list = [reference["series"][row["id"]] for row in rows]
    progress = queued_progress(series_list)
    report_db.open_run(
        key,
        report_header(args.version, args.revision),
        benchmark_definitions(series_list),
        progress,
    )
    print(f"run_key={key}")
    print(f"started_at={progress['started_at']}")
    print(f"url={SITE_URL}{key}")
    print("series=" + json.dumps(rows, separators=(",", ":")))


def sweep(reference, args):
    start = time.monotonic()
    series = reference["series"][args.series]
    # Every series of the benchmark declares the same definition.
    benchmarks = benchmark_definitions(
        [peer for peer in reference["series"] if peer["benchmark"] == series["benchmark"]]
    )
    # Reopens the run if it was marked failed and a failed job is re-run.
    report_db.open_run(
        args.run_key,
        report_header(args.version, args.revision),
        benchmarks,
        queued_progress([series]),
        deadline=start + WRITE_DEADLINE_SECONDS,
    )
    labels = series_launch_groups(series, benchmarks[series["benchmark"]])
    run_series(series, args.binary, args.output, args.run_key, labels, start=start,
               clock_flags=reference_clock_flags(reference))


def finalize(reference, args):
    results = {
        int(path.parent.name.removeprefix("series-")): json.loads(path.read_text())
        for path in args.artifacts.glob("series-*/results.json")
    }
    progress = final_progress(selected_series(reference, args.benchmark), results, args.started_at)
    state = "completed" if args.sweep_result == "success" else "failed"
    report_db.finish_run(args.run_key, state, progress)
    print(f"{state}: {progress['completed']}/{progress['total']} configurations complete")


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument(
        "--reference",
        type=Path,
        default=Path(__file__).with_name("mi300x-v1.1.json.gz"),
    )
    sub = p.add_subparsers(dest="command", required=True)
    plan_parser = sub.add_parser("plan")
    plan_parser.set_defaults(function=plan)
    plan_parser.add_argument("--benchmark")
    plan_parser.add_argument("--tag", default="")
    plan_parser.add_argument("--run-id", required=True)
    run = sub.add_parser("run")
    run.set_defaults(function=sweep)
    run.add_argument("series", type=int)
    run.add_argument("binary", type=Path)
    run.add_argument("output", type=Path)
    run.add_argument("--run-key", required=True)
    for parser in (plan_parser, run):
        parser.add_argument("--version", required=True)
        parser.add_argument("--revision", required=True)
    final = sub.add_parser("finalize")
    final.set_defaults(function=finalize)
    final.add_argument("artifacts", type=Path)
    final.add_argument("--benchmark")
    final.add_argument("--run-key", required=True)
    final.add_argument("--started-at", default="")
    final.add_argument("--sweep-result", required=True)
    args = p.parse_args()
    args.function(read_reference(args.reference), args)


if __name__ == "__main__":
    main()
