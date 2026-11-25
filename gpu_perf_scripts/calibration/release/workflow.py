"""Prepare, run and report release calibration against recorded MI300X dispatches."""

import argparse
import gzip
import json
import math
import re
import sqlite3
import subprocess
import time
from collections import Counter
from contextlib import closing
from datetime import UTC, datetime
from pathlib import Path

BUDGET_SECONDS = 5.5 * 60 * 60
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


def fixed_parameters(label):
    def parse(value):
        try:
            return json.loads(value)
        except ValueError:
            return value

    if label in {"默认参数", "Default parameters", ""}:
        return {}
    return {
        key: parse(value)
        for key, value in (pair.split("=", 1) for pair in label.split(", "))
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


def run_case(binary, case, directory, remaining):
    directory.mkdir()
    with (directory / "stdout.log").open("w+") as log:
        try:
            process = subprocess.run(
                [str(binary), *COMMON, *case["args"]],
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


def run_series(series, binary, output, budget=BUDGET_SECONDS):
    output.mkdir(parents=True, exist_ok=True)
    runs = output / "runs"
    runs.mkdir()
    results = {}
    start = timestamp()
    status_file = output / "results.json"
    deadline = time.monotonic() + budget
    atomic_json(status_file, {"started_at": start, "cases": results})
    for case in sorted(series["cases"], key=lambda case: case["x"]):
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            result = {
                "status": "unsupported",
                "reason": "Series budget expired before this configuration started.",
            }
        else:
            directory = runs / str(case["case"])
            result = run_case(binary.resolve(), case, directory, remaining)
            # The dispatch rows are already in the results artifact.
            for file in directory.glob("timing.sqlite3*"):
                file.unlink()
        results[str(case["case"])] = result
        atomic_json(status_file, {"started_at": start, "cases": results})
        print(f"{case['x']}: {result['status']}", flush=True)


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


def same_dispatch(hardware, simulated, demangled, *, benchmark=""):
    simulated_name = demangled[simulated[0]]
    if benchmark == "altis_cfd" and simulated_name in CFD_KERNEL_ALIASES:
        # The simulator's gfx942 code object exports these CFD kernels as
        # C symbols, while ROCprof records their names in cfd_device.
        matches = base_kernel_name(hardware[0]) == CFD_KERNEL_ALIASES[simulated_name]
    else:
        matches = same_kernel(hardware[0], simulated_name)
    return matches and list(simulated[1:-1]) == hardware[1:-1]


CFD_KERNEL_ALIASES = {
    "compute_step_factor_kernel": "cfd_device::compute_step_factor_kernel",
    "compute_flux_kernel": "cfd_device::compute_flux_kernel",
    "rk_update_kernel": "cfd_device::rk_update_kernel",
}


def cfd_launch_groups(hardware):
    # CFD records ten timesteps, each with a copy, a factor calculation, and
    # three Runge–Kutta stages. Keep the first-timestep effects observed in the
    # MI300X reference separate and group the similar stage 1 and 2 timings.
    stage = ("cfd_device::compute_flux_kernel", "cfd_device::rk_update_kernel")
    step = ("copy_variables_kernel", "cfd_device::compute_step_factor_kernel") + stage * 3
    if tuple(base_kernel_name(row[0]) for row in hardware) != step * 10:
        return ()
    if any(row[1:-1] != hardware[0][1:-1] for row in hardware):
        return ()

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


def report(reference, results, version, revision, *, live=False):
    durations = []
    all_simulated = (
        row
        for series in results.values()
        for case in series["cases"].values()
        for row in case.get("launches", [])
    )
    demangled = kernel_names(all_simulated)
    counts = Counter()
    benchmarks = {}
    points = []
    for group_id, series in enumerate(reference["series"]):
        if series.get("selection_excluded"):
            continue
        group_results = results.get(group_id, {})
        if group_results:
            durations.append(group_results["started_at"])
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
        for case in series["cases"]:
            if live and not group_results:
                counts["queued"] += 1
                continue
            run = group_results.get("cases", {}).get(
                str(case["case"]),
                {
                    "status": "failed",
                    "reason": "Series job ended without producing a result for this configuration.",
                },
            )
            state = run["status"]
            hardware = case["launches"]
            simulated = run.get("launches", [])
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
            counts[
                "unsupported" if state == "complete" and None in paired else state
            ] += 1
            launch_groups = ()
            if bench == "altis_cfd" and (not simulated or None not in paired):
                launch_groups = cfd_launch_groups(hardware)
                if launch_groups:
                    definition["params"]["launch_group"] = {"unit": ""}
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
                    point["statuses"] = {
                        "mgpusim": {"state": state, "reason": run["reason"]}
                    }
                points.append(point)
    total = sum(counts.values())
    return {
        "report": {
            "title": f"MGPUSim {version} MI300X calibration",
            "description": f"Simulation commit {revision}. Each chart compares one dispatch "
            "with its median MI300X timing from GPU Benchmarks v1.1.",
            "live": live,
            "groups": {"mi300x": {"name": "AMD MI300X"}},
        },
        "progress": {
            "started_at": min(durations, default=None),
            "updated_at": timestamp(),
            "total": total,
            "running": 0,
            "queued": counts["queued"],
            "completed": counts["complete"],
            "unsuccessful": total - counts["complete"] - counts["queued"],
        },
        "benchmarks": benchmarks,
        "data": points,
    }


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument(
        "--reference",
        type=Path,
        default=Path(__file__).with_name("mi300x-v1.1.json.gz"),
    )
    sub = p.add_subparsers(dest="command", required=True)
    matrix = sub.add_parser("matrix")
    matrix.add_argument("--benchmark")
    run = sub.add_parser("run")
    run.add_argument("series", type=int)
    run.add_argument("binary", type=Path)
    run.add_argument("output", type=Path)
    args = p.parse_args()
    reference = read_reference(args.reference)
    if args.command == "matrix":
        print(json.dumps(matrix_rows(reference, args.benchmark), separators=(",", ":")))
    else:
        run_series(reference["series"][args.series], args.binary, args.output)


if __name__ == "__main__":
    main()
