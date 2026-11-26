"""Collect hardware kernel timings for a calibration reference.

Three steps, each a subcommand:

  plan   Turn existing reference files into a collection plan.
  run    Run every case of a plan on this machine's GPU, one fresh process
         per repetition, and record the kernel dispatches of each run.
  pack   Pack a collection into the archives build_reference.py reads.

`run` does not build the benchmarks; build GPU Benchmarks first. It can be
interrupted and started again: repetitions that already succeeded are kept.
"""

import argparse
import csv
import json
import os
import platform
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import threading
import time
from pathlib import Path

from build_reference import fixed_parameters

LOG_NAME = "collection-current.log"
DISPATCH_FIELDS = [
    "Kernel_Name",
    "Grid_Size_X",
    "Grid_Size_Y",
    "Grid_Size_Z",
    "Workgroup_Size_X",
    "Workgroup_Size_Y",
    "Workgroup_Size_Z",
    "Start_Timestamp",
    "Duration_ns",
]
SHAPE_FIELDS = DISPATCH_FIELDS[1:7]
# Dispatches that are never the measurement: the suite's clock warmer and the
# runtime's buffer fill. A case can narrow this further with "kernel_pattern".
NOT_MEASURED = re.compile(r"^(bench_warmer_kernel|__amd_rocclr_fillBuffer)")
CLOCK_TOLERANCE = 1.02


# ---- plan -----------------------------------------------------------------


def series_label(parameters):
    """The inverse of build_reference.fixed_parameters."""
    if not parameters:
        return "Default parameters"
    return ", ".join(f"{key}={json.dumps(value)}" for key, value in parameters.items())


def plan_from_references(references):
    plan, seen = [], set()
    for reference in references:
        for series in reference["series"]:
            for case in series["cases"]:
                if case["case"] in seen:
                    raise ValueError(f"Case {case['case']} appears twice")
                seen.add(case["case"])
                plan.append(
                    {
                        "case": case["case"],
                        "benchmark": series["benchmark"],
                        "sample": series["sample"],
                        "axis": series["axis"],
                        "series": series_label(series["parameters"]),
                        "x": case["x"],
                        "args": case["args"],
                        # Name and shape of each measured launch, without its time.
                        "expected_launches": [launch[:-1] for launch in case["launches"]],
                    }
                )
    return plan


# ---- benchmark processes --------------------------------------------------


def detect_platform():
    return "rocm" if Path("/opt/rocm").exists() else "cuda"


def benchmark_binary(root, name, platform_name):
    variants = ["hip"] if platform_name == "rocm" else ["cuda", "hip"]
    for variant in variants:
        for directory in sorted(root.glob(f"tier*/{name}/{variant}")):
            binary = directory / name
            if binary.is_file() and os.access(binary, os.X_OK):
                return binary
    raise FileNotFoundError(
        f"No built {name} binary under {root} for {platform_name}; build GPU Benchmarks first"
    )


def environment_value(value):
    if isinstance(value, bool):
        return "true" if value else "false"
    return str(value)


def case_environment(case, extra):
    parameters = dict(fixed_parameters(case["series"]))
    parameters[case["axis"]] = case["x"]
    parameters.update(extra)
    return {f"BENCH_PARAM_{key}": environment_value(value) for key, value in parameters.items()}


def profiler_dispatches(directory):
    """Kernel dispatches recorded by rocprofv3 --kernel-trace, in start order."""
    rows = []
    for path in sorted(directory.rglob("*kernel_trace.csv")):
        with open(path, newline="") as file:
            rows += [row for row in csv.DictReader(file) if row["Kind"] == "KERNEL_DISPATCH"]
    rows.sort(key=lambda row: int(row["Start_Timestamp"]))
    return [
        {
            "Kernel_Name": row["Kernel_Name"],
            **{field: int(row[field]) for field in SHAPE_FIELDS},
            "Start_Timestamp": int(row["Start_Timestamp"]),
            "Duration_ns": int(row["End_Timestamp"]) - int(row["Start_Timestamp"]),
        }
        for row in rows
    ]


def reported_dispatches(output):
    """Kernel records the benchmark printed: one per kernel, launches summed."""
    dispatches = []
    for line in output.splitlines():
        try:
            record = json.loads(line)
        except ValueError:
            continue
        if isinstance(record, dict) and record.get("type") == "kernel":
            dispatches.append(
                {
                    "Kernel_Name": record["name"],
                    **{field: 0 for field in SHAPE_FIELDS},
                    "Start_Timestamp": len(dispatches),
                    "Duration_ns": round(record["time_ms"] * 1e6),
                }
            )
    return dispatches


def signature(dispatch):
    return [dispatch["Kernel_Name"], *(dispatch[field] for field in SHAPE_FIELDS)]


def measured_dispatches(dispatches, case, timer):
    """The dispatches of one run that are the measurement, or None.

    A case taken from an earlier reference names its launches; they must
    appear in that order among the profiler's dispatches. Any other case keeps
    every dispatch that matches its kernel pattern, or that is not one of the
    suite's own helper kernels.
    """
    expected = case.get("expected_launches")
    if expected and timer == "rocprofv3":
        remaining = iter(dispatches)
        selected = []
        for launch in expected:
            match = next((d for d in remaining if signature(d) == list(launch)), None)
            if match is None:
                return None
            selected.append(match)
        return selected
    pattern = case.get("kernel_pattern")
    if pattern:
        return [d for d in dispatches if re.search(pattern, d["Kernel_Name"])]
    return [d for d in dispatches if not NOT_MEASURED.search(d["Kernel_Name"])]


def write_dispatches(path, dispatches):
    with open(path, "w", newline="") as file:
        writer = csv.DictWriter(file, DISPATCH_FIELDS)
        writer.writeheader()
        writer.writerows(dispatches)


def run_once(binary, environment, timer, directory, timeout):
    """Run one repetition. Returns (status, all dispatches)."""
    command = [str(binary)]
    trace = directory / "profiler"
    if timer == "rocprofv3":
        command = ["rocprofv3", "--kernel-trace", "-f", "csv", "-d", str(trace), "--"] + command
    # Its own session, so that a timeout also ends the benchmark the profiler
    # started: a survivor would keep the GPU busy during the next measurement.
    process = subprocess.Popen(
        command,
        cwd=binary.parent,
        env={**os.environ, **environment},
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        start_new_session=True,
    )
    try:
        output, errors = process.communicate(timeout=timeout)
    except subprocess.TimeoutExpired:
        os.killpg(process.pid, signal.SIGKILL)
        process.communicate()
        return "timeout", []
    (directory / "stdout.txt").write_text(output)
    if process.returncode:
        (directory / "stderr.txt").write_text(errors)
        return "failed", []
    if timer == "rocprofv3":
        dispatches = profiler_dispatches(trace)
        shutil.rmtree(trace, ignore_errors=True)
    else:
        dispatches = reported_dispatches(output)
    write_dispatches(directory / "all-dispatches.csv", dispatches)
    return "ok", dispatches


# ---- clock ----------------------------------------------------------------


def amd_clocks(text):
    """(current, minimum, maximum) MHz of each GFX clock in `amd-smi metric --clock`."""
    return [
        tuple(int(value) for value in match)
        for match in re.findall(
            r"GFX_\d+:\s*CLK:\s*(\d+) MHz\s*MIN_CLK:\s*(\d+) MHz\s*MAX_CLK:\s*(\d+) MHz", text
        )
    ]


def command_output(command):
    try:
        return subprocess.run(
            command, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, timeout=60
        ).stdout
    except (OSError, subprocess.TimeoutExpired) as error:
        return f"{type(error).__name__}: {error}"


class ClockLimit:
    """Caps the AMD GPU core clock for the duration of a collection."""

    def __init__(self, megahertz):
        self.megahertz = megahertz
        self.levels = sorted(Path("/sys/class/drm").glob("card*/device/power_dpm_force_performance_level"))

    def set(self, megahertz):
        output = command_output(["amd-smi", "set", "-L", "sclk", "max", str(megahertz)])
        clocks = amd_clocks(command_output(["amd-smi", "metric", "--clock"]))
        if not clocks or any(maximum != megahertz for _, _, maximum in clocks):
            raise RuntimeError(f"Could not cap the GPU clock at {megahertz} MHz: {output.strip()}")

    def __enter__(self):
        clocks = amd_clocks(command_output(["amd-smi", "metric", "--clock"]))
        if not clocks:
            raise RuntimeError("amd-smi reports no GFX clock; cannot cap the GPU clock")
        self.original = max(maximum for _, _, maximum in clocks)
        self.modes = [(path, path.read_text().strip()) for path in self.levels]
        self.set(self.megahertz)
        return self

    def __exit__(self, *_):
        self.set(self.original)
        # amd-smi leaves the performance level at "manual".
        for path, mode in self.modes:
            path.write_text(mode)


class ClockMonitor(threading.Thread):
    """Samples the GPU core clock and power into a CSV while a collection runs."""

    def __init__(self, path, platform_name, interval):
        super().__init__(daemon=True)
        self.path, self.platform_name, self.interval = path, platform_name, interval
        self.samples, self.lock, self.stopped = [], threading.Lock(), threading.Event()
        self.process = None

    def record(self, megahertz, watts, file):
        now = time.time()
        with self.lock:
            self.samples.append((now, megahertz))
        file.write(f"{now:.3f},{megahertz:.0f},{watts:.1f}\n")

    def run(self):
        with open(self.path, "a") as file:
            if file.tell() == 0:
                file.write("unix_time,core_clock_mhz,power_w\n")
            if self.platform_name == "rocm":
                self.sample_sysfs(file)
            else:
                self.sample_nvidia(file)

    def sample_sysfs(self, file):
        monitors = sorted(Path("/sys/class/drm").glob("card*/device/hwmon/hwmon*/freq1_input"))
        if not monitors:
            return
        clock, power = monitors[0], monitors[0].with_name("power1_input")
        while not self.stopped.wait(self.interval):
            try:
                watts = int(power.read_text()) / 1e6 if power.exists() else 0
                self.record(int(clock.read_text()) / 1e6, watts, file)
            except (OSError, ValueError):
                continue

    def sample_nvidia(self, file):
        try:
            self.process = subprocess.Popen(
                [
                    "nvidia-smi",
                    "--query-gpu=clocks.current.graphics,power.draw",
                    "--format=csv,noheader,nounits",
                    f"-lms={max(1, round(self.interval * 1000))}",
                ],
                stdout=subprocess.PIPE,
                stderr=subprocess.DEVNULL,
                text=True,
            )
        except OSError:
            return
        for line in self.process.stdout:
            try:
                megahertz, watts = (float(value) for value in line.split(","))
            except ValueError:
                continue
            self.record(megahertz, watts, file)

    def maximum(self, start, end):
        """Highest clock sampled in [start, end], and the number of samples."""
        with self.lock:
            values = [megahertz for moment, megahertz in self.samples if start <= moment <= end]
            del self.samples[:-10000]
        return (max(values) if values else None), len(values)

    def stop(self):
        self.stopped.set()
        if self.process:
            self.process.terminate()
        self.join(timeout=5)


# ---- run ------------------------------------------------------------------


def completed(log):
    """(case, repetition) pairs a previous run already collected."""
    done = set()
    if log.exists():
        for line in log.read_text().splitlines():
            row = json.loads(line)
            if row["status"] == "ok":
                done.add((row["case_id"], row["rep"]))
    return done


def environment_record(args, platform_name, timer):
    revision = command_output(["git", "-C", str(args.benchmarks), "rev-parse", "HEAD"]).strip()
    record = {
        "started_at": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
        "host": platform.node(),
        "system": platform.platform(),
        "python": platform.python_version(),
        "platform": platform_name,
        "timer": timer,
        "repetitions": args.reps,
        "timeout_seconds": args.timeout,
        "core_clock_limit_mhz": args.sclk_max,
        "extra_parameters": dict(args.set),
        "benchmarks_revision": revision if re.fullmatch(r"[0-9a-f]{40}", revision) else None,
    }
    if platform_name == "rocm":
        record["amd_smi_version"] = command_output(["amd-smi", "version"]).strip()
        record["clocks"] = command_output(["amd-smi", "metric", "--clock"])
    else:
        record["gpus"] = command_output(["nvidia-smi", "-L"]).strip()
    return record


def collect(args):
    plan = json.loads(args.plan.read_text())
    # Each benchmark runs in its own directory, so relative paths would break.
    args.benchmarks, args.output = args.benchmarks.resolve(), args.output.resolve()
    cases = sorted(
        (
            case
            for case in plan
            if not case.get("unsupported_reason")
            and (not args.benchmark or case["benchmark"] in args.benchmark)
        ),
        key=lambda case: (case["benchmark"], case["series"], case["x"]),
    )
    if not cases:
        raise SystemExit("The plan has no case to collect")
    platform_name = args.platform or detect_platform()
    timer = args.timer or ("rocprofv3" if platform_name == "rocm" else "reported")
    binaries = {
        name: benchmark_binary(args.benchmarks, name, platform_name)
        for name in sorted({case["benchmark"] for case in cases})
    }
    args.output.mkdir(parents=True, exist_ok=True)
    log = args.output / LOG_NAME
    done = completed(log)
    with open(args.output / "environment.jsonl", "a") as file:
        file.write(json.dumps(environment_record(args, platform_name, timer)) + "\n")

    monitor = ClockMonitor(args.output / "clock-monitor.csv", platform_name, args.monitor_interval)
    monitor.start()
    counts, timed_out = {}, set()
    try:
        with open(log, "a") as file:
            for case in cases:
                series = (case["benchmark"], case["series"])
                environment = case_environment(case, dict(args.set))
                for rep in range(1, args.reps + 1):
                    if (case["case"], rep) in done:
                        continue
                    row = {"case_id": case["case"], "rep": rep, "benchmark": case["benchmark"]}
                    if series in timed_out:
                        # Cases run in increasing size; a larger one will not fit either.
                        row.update(status="skipped_after_timeout", measured_dispatches=0)
                    else:
                        directory = (
                            args.output / "dispatch" / case["benchmark"] / str(case["case"]) / str(rep) / "trace"
                        )
                        shutil.rmtree(directory, ignore_errors=True)
                        directory.mkdir(parents=True)
                        start = time.time()
                        status, dispatches = run_once(
                            binaries[case["benchmark"]], environment, timer, directory, args.timeout
                        )
                        end = time.time()
                        measured = measured_dispatches(dispatches, case, timer) if status == "ok" else []
                        if status == "ok" and measured is None:
                            status, measured = "launch_mismatch", []
                        elif status == "ok" and not measured:
                            status = "no_dispatches"
                        if status == "ok":
                            write_dispatches(directory / "measured-dispatches.csv", measured)
                        if status == "timeout":
                            timed_out.add(series)
                        clock, samples = monitor.maximum(start, end)
                        row.update(
                            status=status,
                            measured_dispatches=len(measured),
                            started_at=round(start, 3),
                            seconds=round(end - start, 3),
                            core_clock_mhz_max=clock,
                            core_clock_samples=samples,
                        )
                        if args.sclk_max and clock and clock > args.sclk_max * CLOCK_TOLERANCE:
                            row["core_clock_above_limit"] = True
                    file.write(json.dumps(row) + "\n")
                    file.flush()
                    counts[row["status"]] = counts.get(row["status"], 0) + 1
                    print(f"{case['benchmark']} case {case['case']} rep {rep}: {row['status']}", flush=True)
    finally:
        monitor.stop()
    print(json.dumps(counts, sort_keys=True))
    return 0 if set(counts) <= {"ok"} else 1


def run(args):
    if args.sclk_max:
        with ClockLimit(args.sclk_max):
            return collect(args)
    return collect(args)


# ---- pack -----------------------------------------------------------------


def archive(source, members, target):
    with open(target, "wb") as file:
        tar = subprocess.Popen(["tar", "-cf", "-", "-C", str(source), *members], stdout=subprocess.PIPE)
        zstd = subprocess.run(["zstd", "-q", "-T0", "-c"], stdin=tar.stdout, stdout=file)
        tar.stdout.close()
        if tar.wait() or zstd.returncode:
            raise RuntimeError(f"Could not write {target}")


def pack(args):
    args.archives.mkdir(parents=True, exist_ok=True)
    records = [
        name
        for name in (LOG_NAME, "environment.jsonl", "clock-monitor.csv")
        if (args.collection / name).exists()
    ]
    archive(args.collection, records, args.archives / "collection-complete.tar.zst")
    archive(args.collection, ["dispatch"], args.archives / "dispatch-0001.tar.zst")
    return 0


# ---- command line ---------------------------------------------------------


def parameter(text):
    name, separator, value = text.partition("=")
    if not name or not separator:
        raise argparse.ArgumentTypeError("expected NAME=VALUE")
    return name, value


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    commands = parser.add_subparsers(dest="command", required=True)

    planner = commands.add_parser("plan", help="derive a collection plan from reference files")
    planner.add_argument("references", nargs="+", type=Path)
    planner.add_argument("-o", "--output", type=Path, required=True)

    runner = commands.add_parser("run", help="collect a plan on this machine")
    runner.add_argument("plan", type=Path)
    runner.add_argument("--benchmarks", type=Path, required=True, help="built GPU Benchmarks checkout")
    runner.add_argument("--output", type=Path, required=True, help="collection directory")
    runner.add_argument("--reps", type=int, default=7)
    runner.add_argument("--timeout", type=float, default=600, help="seconds per repetition")
    runner.add_argument("--benchmark", action="append", help="collect only this benchmark (repeatable)")
    runner.add_argument("--platform", choices=["rocm", "cuda"])
    runner.add_argument(
        "--timer",
        choices=["rocprofv3", "reported"],
        help="rocprofv3: profiler dispatch durations (default on ROCm); "
        "reported: the GPU event times the benchmark prints (default on CUDA)",
    )
    runner.add_argument("--sclk-max", type=int, help="cap the AMD GPU core clock at this many MHz")
    runner.add_argument("--set", action="append", default=[], type=parameter, metavar="NAME=VALUE",
                        help="benchmark parameter added to every case (repeatable)")
    runner.add_argument("--monitor-interval", type=float, default=0.1, help="seconds between clock samples")

    packer = commands.add_parser("pack", help="pack a collection for build_reference.py")
    packer.add_argument("collection", type=Path)
    packer.add_argument("archives", type=Path)

    args = parser.parse_args(argv)
    if args.command == "plan":
        import gzip

        references = [json.load(gzip.open(path, "rt")) for path in args.references]
        args.output.write_text(json.dumps(plan_from_references(references), indent=1) + "\n")
        return 0
    if args.command == "run":
        return run(args)
    return pack(args)


if __name__ == "__main__":
    sys.exit(main())
