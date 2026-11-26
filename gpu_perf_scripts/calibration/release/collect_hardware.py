"""Collect hardware kernel timings for a calibration reference.

Three steps, each a subcommand:

  plan   Turn existing reference files into a collection plan.
  run    Run every case of a plan on this machine's GPU, one fresh process
         per repetition, and record the kernel dispatches of each run.
  pack   Pack a collection into the archives build_reference.py reads.

`run` does not build the benchmarks; build GPU Benchmarks first. It needs a
machine with exactly one GPU. It can be interrupted and started again with the
same settings: repetitions that already succeeded are kept.
"""

import argparse
import contextlib
import csv
import gzip
import hashlib
import json
import os
import platform
import re
import shutil
import signal
import subprocess
import sys
import threading
import time
from pathlib import Path

from build_reference import fixed_parameters

LOG_NAME = "collection-current.log"
IDENTITY_NAME = "collection.json"
# What a collection consists of, beside its dispatch tree.
RECORD_NAMES = [LOG_NAME, IDENTITY_NAME, "environment.jsonl", "clock-monitor.csv"]
ARCHIVE_NAMES = {"records": "collection-complete.tar.zst", "dispatches": "dispatch-0001.tar.zst"}
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
WARMUP = "vector_add_warmup"
CLOCK_TOLERANCE = 1.02
TELEMETRY_START_SECONDS = 10
# Sampling has stopped when the last sample is older than this many sampling
# intervals, and than this many seconds.
TELEMETRY_STALE_INTERVALS = 20
TELEMETRY_STALE_SECONDS = 2
SYSFS = Path("/sys/class/drm")


# ---- plan -----------------------------------------------------------------


def series_label(parameters):
    """The inverse of build_reference.fixed_parameters."""
    if not parameters:
        return "Default parameters"
    return ", ".join(f"{key}={json.dumps(value)}" for key, value in parameters.items())


def repeated_case(cases):
    """An identifier that more than one of the cases carries, or None."""
    seen = set()
    for case in cases:
        if case["case"] in seen:
            return case["case"]
        seen.add(case["case"])
    return None


def plan_from_references(references):
    plan = [
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
        for reference in references
        for series in reference["series"]
        for case in series["cases"]
    ]
    if repeated_case(plan) is not None:
        raise ValueError(f"Case {repeated_case(plan)} appears twice")
    return plan


def case_parameters(case):
    return {**fixed_parameters(case["series"]), case["axis"]: case["x"]}


def collectable(case):
    """Whether a plan entry can be run and filed as it is written.

    Its identifier and benchmark become directory names and its parameters
    environment variables, so all of them are checked before anything runs.
    """
    try:
        parameters = case_parameters(case)
        re.compile(case.get("kernel_pattern", ""))
        return (
            type(case["case"]) is int
            and all(
                isinstance(name, str) and re.fullmatch(r"\w+", name) for name in [case["benchmark"], *parameters]
            )
            and all(type(value) in (bool, int, float, str) for value in parameters.values())
            and all(
                isinstance(launch, list) and len(launch) == len(SHAPE_FIELDS) + 1
                for launch in case.get("expected_launches", [])
            )
        )
    except (AttributeError, KeyError, TypeError, ValueError, re.error):
        return False


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


def case_environment(case, warmup):
    """The benchmark parameters of a case; `warmup` None leaves the suite's default."""
    parameters = case_parameters(case)
    if warmup is not None:
        parameters[WARMUP] = warmup
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

    A case taken from an earlier reference names its launches. They are
    matched, in order, against the last dispatches of that name and shape: a
    benchmark that primes with its own kernel launches the untimed primer
    first (cache_latency does). Any other case keeps every dispatch that
    matches its kernel pattern, or that is not one of the suite's helpers.
    """
    expected = case.get("expected_launches")
    if expected and timer == "rocprofv3":
        remaining = reversed(dispatches)
        selected = []
        for launch in reversed(expected):
            match = next((d for d in remaining if signature(d) == list(launch)), None)
            if match is None:
                return None
            selected.append(match)
        return selected[::-1]
    pattern = case.get("kernel_pattern")
    if pattern:
        return [d for d in dispatches if re.search(pattern, d["Kernel_Name"])]
    return [d for d in dispatches if not NOT_MEASURED.search(d["Kernel_Name"])]


def unselected_look_alikes(dispatches, measured):
    """Dispatches with a measured launch's name and shape that were not selected."""
    signatures = [signature(dispatch) for dispatch in measured]
    return sum(signature(dispatch) in signatures for dispatch in dispatches) - len(measured)


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
    # Only the collector decides the workload, never the caller's shell.
    inherited = {key: value for key, value in os.environ.items() if not key.startswith("BENCH_PARAM_")}
    # Its own session, so that the benchmark the profiler started can be ended
    # with it: a survivor would keep the GPU busy during the next measurement.
    process = subprocess.Popen(
        command,
        cwd=binary.parent,
        env={**inherited, **environment},
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        start_new_session=True,
    )
    try:
        output, errors = process.communicate(timeout=timeout)
    except BaseException as error:  # a timeout, Ctrl-C or SIGTERM
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        process.communicate()
        if isinstance(error, subprocess.TimeoutExpired):
            return "timeout", []
        raise
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


# ---- the GPU, its clock and its telemetry ---------------------------------


def command_output(command):
    try:
        return subprocess.run(
            command, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, timeout=60
        ).stdout
    except (OSError, subprocess.TimeoutExpired) as error:
        return f"{type(error).__name__}: {error}"


def amd_clock_report():
    return command_output(["amd-smi", "metric", "--clock"])


def the_gpu(platform_name):
    """Identity of the machine's only GPU and, on ROCm, its clock sensor.

    Launch, clock cap and telemetry must all concern the same device. With one
    GPU that needs no device selection anywhere, so more than one is refused.
    """
    if platform_name == "rocm":
        listed = len(re.findall(r"^GPU: *\d+", amd_clock_report(), re.MULTILINE))
        sensors = sorted(SYSFS.glob("card*/device/hwmon/hwmon*/freq1_input"))
        if listed != 1 or len(sensors) != 1:
            raise SystemExit(
                f"Collection needs exactly one AMD GPU; amd-smi lists {listed} "
                f"and {len(sensors)} have a clock sensor"
            )
        device = sensors[0].parent.parent.parent
        return f"{device.parent.name} {device.resolve().name}", sensors[0]
    rows = [
        line.strip()
        for line in command_output(
            ["nvidia-smi", "--query-gpu=uuid,name,driver_version", "--format=csv,noheader"]
        ).splitlines()
        if line.strip()
    ]
    if len(rows) != 1 or not rows[0].startswith("GPU-"):
        raise SystemExit(f"Collection needs exactly one NVIDIA GPU; nvidia-smi reports: {rows}")
    return rows[0], None


def gfx_clock_limits(report):
    """Maximum MHz of every GFX clock in `amd-smi metric --clock`; None where it cannot be read."""
    sections = re.split(r"^\s*(?=(?:[A-Z]+_\d+|GPU):)", report, flags=re.MULTILINE)
    limits = []
    for section in sections:
        if re.match(r"GFX_\d+:", section):
            limit = re.search(r"^\s*MAX_CLK:\s*(\d+) MHz\s*$", section, re.MULTILINE)
            limits.append(int(limit.group(1)) if limit else None)
    return limits


class ClockLimit:
    """Caps the AMD GPU core clock for the duration of a collection.

    Creating one changes nothing; it refuses a clock state that could not be
    put back. Entering applies the cap and leaving restores what was found.
    """

    def __init__(self, megahertz):
        self.megahertz = megahertz
        limits = gfx_clock_limits(amd_clock_report())
        levels = sorted(SYSFS.glob("card*/device/power_dpm_force_performance_level"))
        if not limits or None in limits or len(set(limits)) != 1:
            raise SystemExit(
                f"A clock cap needs every GFX clock to report one common limit to restore; amd-smi reports {limits}"
            )
        if len(levels) != 1:
            raise SystemExit(f"A clock cap needs one performance level to restore; found {len(levels)}")
        self.original, self.clocks, self.level = limits[0], len(limits), levels[0]
        try:
            # Opened the way restoring it will need, but without changing it.
            with open(self.level, "r+") as control:
                self.mode = control.read().strip()
        except OSError as error:
            raise SystemExit(f"A clock cap needs a performance level that can be written back: {error}") from None
        if not self.mode:
            raise SystemExit(f"A clock cap needs a performance level to restore; {self.level} is empty")

    def apply(self, megahertz):
        output = command_output(["amd-smi", "set", "-L", "sclk", "max", str(megahertz)])
        if gfx_clock_limits(amd_clock_report()) != [megahertz] * self.clocks:
            raise RuntimeError(f"Could not set the GPU clock limit to {megahertz} MHz: {output.strip()}")

    def restore(self):
        # A second Ctrl-C or SIGTERM must not leave the GPU capped.
        handlers = {number: signal.signal(number, signal.SIG_IGN) for number in (signal.SIGINT, signal.SIGTERM)}
        try:
            self.apply(self.original)
        finally:
            try:
                # amd-smi leaves the performance level at "manual".
                self.level.write_text(self.mode)
            finally:
                for number, handler in handlers.items():
                    signal.signal(number, handler)

    def __enter__(self):
        try:
            self.apply(self.megahertz)
        except BaseException:
            # The cap may be in place although it could not be confirmed.
            self.restore()
            raise
        return self

    def __exit__(self, *_):
        self.restore()


class ClockMonitor(threading.Thread):
    """Samples the GPU core clock and power into a CSV while a collection runs."""

    def __init__(self, path, sensor, interval):
        super().__init__(daemon=True)
        self.path, self.sensor, self.interval = path, sensor, interval
        self.samples, self.lock = [], threading.Lock()
        self.stopped, self.sampled = threading.Event(), threading.Event()
        self.process, self.latest, self.interrupted = None, None, False

    def stale(self, moment):
        """Whether the sample before `moment` is too old for sampling to count as continuous."""
        limit = max(TELEMETRY_STALE_SECONDS, TELEMETRY_STALE_INTERVALS * self.interval)
        return self.latest is not None and moment - self.latest > limit

    def record(self, megahertz, watts, file):
        """One sample; `watts` is None when the GPU reports no power."""
        now = time.time()
        with self.lock:
            self.samples.append((now, megahertz))
            # Remembered: a later sample does not make up for the gap before it.
            self.interrupted = self.interrupted or self.stale(time.monotonic())
            self.latest = time.monotonic()
        file.write(f"{now:.3f},{megahertz:.0f},{'' if watts is None else f'{watts:.1f}'}\n")
        self.sampled.set()

    def run(self):
        with open(self.path, "a") as file:
            if file.tell() == 0:
                file.write("unix_time,core_clock_mhz,power_w\n")
            if self.sensor:
                self.sample_sysfs(file)
            else:
                self.sample_nvidia(file)

    def sample_sysfs(self, file):
        power = self.sensor.with_name("power1_input")
        power = power if power.exists() else None
        while not self.stopped.is_set():
            watts = int(power.read_text()) / 1e6 if power else None
            self.record(int(self.sensor.read_text()) / 1e6, watts, file)
            self.stopped.wait(self.interval)

    def sample_nvidia(self, file):
        command = [
            "nvidia-smi",
            "--query-gpu=clocks.current.graphics,power.draw",
            "--format=csv,noheader,nounits",
            f"-lms={max(1, round(self.interval * 1000))}",
        ]
        with subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True) as self.process:
            for line in self.process.stdout:
                megahertz, _, watts = line.partition(",")
                try:
                    watts = float(watts)
                except ValueError:
                    watts = None
                self.record(float(megahertz), watts, file)

    def begin(self):
        """Start sampling; a collection without telemetry does not start."""
        self.start()
        if not self.sampled.wait(TELEMETRY_START_SECONDS):
            self.stop()
            raise SystemExit("The GPU clock cannot be sampled; refusing to collect without telemetry")

    def check(self):
        """Fail unless sampling has been continuous since it began and still is."""
        with self.lock:
            interrupted = self.interrupted or self.stale(time.monotonic())
        if interrupted or not self.is_alive():
            raise RuntimeError("GPU clock sampling stopped or paused; the collection is not monitored any more")

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


def planned_cases(args, plan):
    """The cases of the plan this invocation collects, in running order."""
    if not isinstance(plan, list) or not all(isinstance(case, dict) for case in plan):
        raise SystemExit("The plan is not a list of cases")
    supported = [case for case in plan if not case.get("unsupported_reason")]
    for case in supported:
        if not collectable(case):
            raise SystemExit(
                f"The plan has an invalid case {case.get('case')!r}: a case needs an integer identifier, "
                "a benchmark and parameters with plain names and values, launches of a name and six sizes, "
                "and a valid kernel pattern"
            )
        if args.vector_add_warmup is not None and WARMUP in case_parameters(case):
            raise SystemExit(f"Case {case['case']} sets {WARMUP} itself; --vector-add-warmup would replace it")
    # A second entry would clear and overwrite the traces of the first.
    if repeated_case(supported) is not None:
        raise SystemExit(f"The plan has more than one case {repeated_case(supported)}")
    unknown = sorted(set(args.benchmark or ()) - {case["benchmark"] for case in supported})
    if unknown:
        raise SystemExit(f"The plan has no benchmark named {', '.join(unknown)}")
    cases = [case for case in supported if not args.benchmark or case["benchmark"] in args.benchmark]
    if not cases:
        raise SystemExit("The plan has no case to collect")
    try:
        # Increasing size within a series: after a timeout the rest of it is skipped.
        return sorted(cases, key=lambda case: (case["benchmark"], case["series"], case["x"]))
    except TypeError:
        raise SystemExit("The cases of one series must have scaling values of one type") from None


def prepare(args):
    """Check everything that can be checked; nothing is created or changed here."""
    if args.reps < 1 or args.timeout <= 0 or args.monitor_interval <= 0:
        raise SystemExit("--reps, --timeout and --monitor-interval must be positive")
    if args.sclk_max is not None and args.sclk_max <= 0:
        raise SystemExit("--sclk-max must be positive")
    # Each benchmark runs in its own directory, so relative paths would break.
    args.benchmarks, args.output = args.benchmarks.resolve(), args.output.resolve()
    plan_text = args.plan.read_bytes()
    cases = planned_cases(args, json.loads(plan_text))
    platform_name = args.platform or detect_platform()
    if args.sclk_max is not None and platform_name != "rocm":
        raise SystemExit("--sclk-max caps an AMD GPU clock; it does not apply on CUDA")
    timer = args.timer or ("rocprofv3" if platform_name == "rocm" else "reported")
    if timer == "rocprofv3" and not shutil.which("rocprofv3"):
        raise SystemExit("rocprofv3 is not on the PATH")
    binaries = {
        name: benchmark_binary(args.benchmarks, name, platform_name)
        for name in sorted({case["benchmark"] for case in cases})
    }
    gpu, sensor = the_gpu(platform_name)
    revision = args.benchmarks_revision or command_output(
        ["git", "-C", str(args.benchmarks), "rev-parse", "HEAD"]
    ).strip()
    if not re.fullmatch(r"[0-9a-f]{40}", revision):
        raise SystemExit(
            f"Cannot tell the GPU Benchmarks revision of {args.benchmarks}; "
            "pass --benchmarks-revision with the full commit hash"
        )
    # What must not change between the invocations that fill one collection.
    identity = {
        "plan_sha256": hashlib.sha256(plan_text).hexdigest(),
        "host": platform.node(),
        "gpu": gpu,
        "platform": platform_name,
        "timer": timer,
        "core_clock_limit_mhz": args.sclk_max,
        "vector_add_warmup": args.vector_add_warmup,
        "benchmarks_revision": revision,
    }
    record = args.output / IDENTITY_NAME
    if record.exists():
        previous = json.loads(record.read_text())
        changed = sorted(key for key in {*identity, *previous} if identity.get(key) != previous.get(key))
        if changed:
            raise SystemExit(
                f"{args.output} holds a collection made with a different {', '.join(changed)}; "
                "use a new output directory"
            )
    elif (args.output / LOG_NAME).exists() or (args.output / "dispatch").exists():
        # Its repetitions would be taken for ones made with the settings above.
        raise SystemExit(f"{args.output} holds collection data but no {IDENTITY_NAME}; use a new output directory")
    try:
        done = completed(args.output / LOG_NAME)
    except (KeyError, TypeError, ValueError):
        raise SystemExit(f"{args.output / LOG_NAME} cannot be read as a collection log") from None
    return {
        "cases": cases,
        "binaries": binaries,
        "identity": identity,
        "done": done,
        "sensor": sensor,
        "invocation": invocation_record(args, identity),
        # Refuses now if the clock state could not be restored later.
        "limit": ClockLimit(args.sclk_max) if args.sclk_max is not None else None,
    }


def invocation_record(args, identity):
    """Taken before a clock cap is applied, so it shows the limits found."""
    record = {
        **identity,
        "started_at": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
        "system": platform.platform(),
        "python": platform.python_version(),
        "repetitions": args.reps,
        "timeout_seconds": args.timeout,
    }
    if identity["platform"] == "rocm":
        record["amd_smi_version"] = command_output(["amd-smi", "version"]).strip()
        record["rocprofv3_version"] = command_output(["rocprofv3", "--version"]).strip()
        record["clocks_before_cap"] = amd_clock_report()
    return record


def collect(args, monitor, cases, binaries, identity, done, invocation):
    """Run the cases. Telemetry is running and a requested clock cap is in place."""
    record, log = args.output / IDENTITY_NAME, args.output / LOG_NAME
    if not record.exists():
        record.write_text(json.dumps(identity, indent=1) + "\n")
    with open(args.output / "environment.jsonl", "a") as file:
        file.write(json.dumps(invocation) + "\n")
    timer, cap = identity["timer"], identity["core_clock_limit_mhz"]
    counts, timed_out = {}, set()
    with open(log, "a") as file:
        for case in cases:
            series = (case["benchmark"], case["series"])
            environment = case_environment(case, identity[WARMUP])
            for rep in range(1, args.reps + 1):
                if (case["case"], rep) in done:
                    continue
                row = {"case_id": case["case"], "rep": rep, "benchmark": case["benchmark"]}
                if series in timed_out:
                    # Cases run in increasing size; a larger one will not fit either.
                    row.update(status="skipped_after_timeout", measured_dispatches=0)
                else:
                    directory = args.output / "dispatch" / case["benchmark"] / str(case["case"]) / str(rep) / "trace"
                    shutil.rmtree(directory, ignore_errors=True)
                    directory.mkdir(parents=True)
                    start = time.time()
                    status, dispatches = run_once(
                        binaries[case["benchmark"]], environment, timer, directory, args.timeout
                    )
                    end = time.time()
                    # A repetition that outlived the sampler is not recorded and runs again.
                    monitor.check()
                    measured = measured_dispatches(dispatches, case, timer) if status == "ok" else []
                    if status == "ok" and measured is None:
                        status, measured = "launch_mismatch", []
                    elif status == "ok" and not measured:
                        status = "no_dispatches"
                    elif status == "ok" and any(d["Duration_ns"] <= 0 for d in measured):
                        # build_reference.py rejects such a launch.
                        status = "invalid_duration"
                    clock, samples = monitor.maximum(start, end)
                    if status == "ok" and cap is not None and clock is not None and clock > cap * CLOCK_TOLERANCE:
                        # It did not run under the cap, so it is collected again.
                        status = "clock_above_limit"
                    if status == "ok":
                        write_dispatches(directory / "measured-dispatches.csv", measured)
                    if status == "timeout":
                        timed_out.add(series)
                    row.update(
                        status=status,
                        measured_dispatches=len(measured),
                        started_at=round(start, 3),
                        seconds=round(end - start, 3),
                        # A repetition shorter than the sampling interval can have no sample.
                        core_clock_mhz_max=clock,
                        core_clock_samples=samples,
                    )
                    look_alikes = unselected_look_alikes(dispatches, measured)
                    if look_alikes:
                        row["unselected_look_alike_dispatches"] = look_alikes
                file.write(json.dumps(row) + "\n")
                file.flush()
                counts[row["status"]] = counts.get(row["status"], 0) + 1
                print(f"{case['benchmark']} case {case['case']} rep {rep}: {row['status']}", flush=True)
    print(json.dumps(counts, sort_keys=True))
    return 0 if set(counts) <= {"ok"} else 1


def interrupt(*_):
    raise KeyboardInterrupt


def run(args):
    prepared = prepare(args)
    limit = prepared.pop("limit") or contextlib.nullcontext()
    sensor = prepared.pop("sensor")
    # SIGTERM takes the path of Ctrl-C: end the benchmark, then lift the cap.
    # A forced kill of the collector (SIGKILL) skips both.
    previous = signal.signal(signal.SIGTERM, interrupt)
    try:
        args.output.mkdir(parents=True, exist_ok=True)
        monitor = ClockMonitor(args.output / "clock-monitor.csv", sensor, args.monitor_interval)
        # Telemetry first: without it the clock is not touched either.
        monitor.begin()
        try:
            with limit:
                return collect(args, monitor, **prepared)
        finally:
            monitor.stop()
    finally:
        signal.signal(signal.SIGTERM, previous)


# ---- pack -----------------------------------------------------------------


def archive(source, members, target):
    with open(target, "wb") as file:
        tar = subprocess.Popen(["tar", "-cf", "-", "-C", str(source), *members], stdout=subprocess.PIPE)
        zstd = subprocess.run(["zstd", "-q", "-T0", "-c"], stdin=tar.stdout, stdout=file)
        tar.stdout.close()
        if tar.wait() or zstd.returncode:
            raise RuntimeError(f"Could not write {target}")


def pack(args):
    missing = [name for name in [*RECORD_NAMES, "dispatch"] if not (args.collection / name).exists()]
    if missing:
        raise SystemExit(f"{args.collection} is not a complete collection; it has no {', '.join(missing)}")
    if not completed(args.collection / LOG_NAME):
        raise SystemExit(f"{args.collection} has no successful repetition to pack")
    for tool in ("tar", "zstd"):
        if not shutil.which(tool):
            raise SystemExit(f"{tool} is not on the PATH")
    if args.archives.exists() and any(args.archives.glob("*.tar.zst")):
        # build_reference.py reads every dispatch archive it finds there.
        raise SystemExit(f"{args.archives} already holds archives; pack into a new directory")
    args.archives.mkdir(parents=True, exist_ok=True)
    members = {"records": RECORD_NAMES, "dispatches": ["dispatch"]}
    partial = {kind: args.archives / (name + ".partial") for kind, name in ARCHIVE_NAMES.items()}
    try:
        for kind, path in partial.items():
            archive(args.collection, members[kind], path)
        # Publish only once both are complete.
        for kind, path in partial.items():
            os.replace(path, args.archives / ARCHIVE_NAMES[kind])
    finally:
        for path in partial.values():
            if path.exists():
                path.unlink()
    return 0


# ---- command line ---------------------------------------------------------


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    commands = parser.add_subparsers(dest="command", required=True)

    planner = commands.add_parser("plan", help="derive a collection plan from reference files")
    planner.add_argument("references", nargs="+", type=Path)
    planner.add_argument("-o", "--output", type=Path, required=True)

    runner = commands.add_parser("run", help="collect a plan on this machine")
    runner.add_argument("plan", type=Path)
    runner.add_argument("--benchmarks", type=Path, required=True, help="built GPU Benchmarks checkout")
    runner.add_argument("--benchmarks-revision", help="its commit hash, when it is not a git checkout")
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
    runner.add_argument(
        "--vector-add-warmup",
        choices=["true", "false"],
        help="run every case with the suite's clock warmer on or off (default: the suite's own default)",
    )
    runner.add_argument("--monitor-interval", type=float, default=0.1, help="seconds between clock samples")

    packer = commands.add_parser("pack", help="pack a collection for build_reference.py")
    packer.add_argument("collection", type=Path)
    packer.add_argument("archives", type=Path, help="new directory for the archives")

    args = parser.parse_args(argv)
    if args.command == "plan":
        references = [json.load(gzip.open(path, "rt")) for path in args.references]
        args.output.write_text(json.dumps(plan_from_references(references), indent=1) + "\n")
        return 0
    if args.command == "run":
        return run(args)
    return pack(args)


if __name__ == "__main__":
    sys.exit(main())
