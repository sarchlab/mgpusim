import json
import os
import shutil
import signal
import stat
import subprocess
import tempfile
import time
import unittest
from pathlib import Path
from unittest.mock import patch

import build_reference
import collect_hardware
from collect_hardware import (
    ClockLimit,
    case_environment,
    gfx_clock_limits,
    main,
    measured_dispatches,
    plan_from_references,
    profiler_dispatches,
    reported_dispatches,
    run_once,
    series_label,
    unselected_look_alikes,
)

SHAPE = [256, 1, 1, 256, 1, 1]
REVISION = "0123456789abcdef0123456789abcdef01234567"
KERNEL = "example_kernel(float*, int)"
REFERENCE = {
    "series": [
        {
            "benchmark": "example",
            "sample": "example",
            "axis": "size",
            "parameters": {"block_size": 256, "pattern": "no_conflict", "pinned": True},
            "cases": [
                {
                    "case": case,
                    "x": size,
                    "args": [f"-size={size}"],
                    "repetitions": 7,
                    "launches": [[KERNEL, *SHAPE, 1000 * size]],
                }
                for case, size in ((7, 64), (8, 128))
            ],
        }
    ]
}

# The benchmark prints its GPU event time; the stand-in profiler records a
# buffer fill, the clock warmer, an untimed primer and the timed kernel, as a
# real run of a self-priming benchmark does.
BENCHMARK = """#!/bin/sh
[ "$BENCH_PARAM_size" = 128 ] && [ -n "$FAIL_LARGE" ] && exit 3
echo "{\\"type\\":\\"kernel\\",\\"name\\":\\"example_kernel\\",\\"time_ms\\":0.$BENCH_PARAM_size}"
"""
TRACE_COLUMNS = (
    "Kind,Kernel_Name,Start_Timestamp,End_Timestamp,"
    "Workgroup_Size_X,Workgroup_Size_Y,Workgroup_Size_Z,Grid_Size_X,Grid_Size_Y,Grid_Size_Z"
)
PROFILER = """#!/bin/sh
[ "$1" = --version ] && { echo "stand-in profiler"; exit 0; }
directory=$5
shift 6
"$@" || exit $?
mkdir -p "$directory/host"
cat > "$directory/host/1_kernel_trace.csv" <<EOF
""" + TRACE_COLUMNS + """
"KERNEL_DISPATCH","example_kernel(float*, int)",4000,$((4000 + ${ZERO_DURATION:+0 * }BENCH_PARAM_size)),256,1,1,256,1,1
"KERNEL_DISPATCH","example_kernel(float*, int)",3000,3999,256,1,1,256,1,1
"KERNEL_DISPATCH","__amd_rocclr_fillBufferUnAligned",1000,1500,256,1,1,256,1,1
"KERNEL_DISPATCH","bench_warmer_kernel(float*, unsigned long long, int)",2000,2600,256,1,1,4096,1,1
EOF
"""
# Remembers the cap in a file and reports it back, unless told that caps do
# not take effect. Like the real tool, a cap leaves the level at "manual".
AMD_SMI = """#!/bin/sh
state={state}
if [ "$1" = set ]; then
  echo "set $5" >> $state.log
  [ -n "$CAP_IGNORED" ] || echo "$5" > $state
  echo manual > {level}
elif [ "$1" = metric ]; then
  for gpu in 0 ${{SECOND_GPU:+1}}; do
    printf 'GPU: %s\\n    CLOCK:\\n        GFX_0:\\n            CLK: 132 MHz\\n' $gpu
    printf '            MIN_CLK: 500 MHz\\n            MAX_CLK: %s MHz\\n' "$(cat $state)"
    [ -z "$UNEQUAL_LIMITS" ] || printf '        GFX_1:\\n            CLK: 1 MHz\\n'
    [ -z "$UNEQUAL_LIMITS" ] || printf '            MIN_CLK: 500 MHz\\n            MAX_CLK: 1900 MHz\\n'
    [ -z "$UNREADABLE_LIMIT" ] || printf '        GFX_1:\\n            CLK: N/A\\n            MAX_CLK: N/A\\n'
  done
else
  echo "stand-in amd-smi"
fi
"""
# One GPU; its clock query yields a single sample and then nothing more.
NVIDIA_SMI = """#!/bin/sh
case "$*" in
  *uuid*) echo "GPU-0123, Stand-in GPU, 1.0" ;;
  *) echo "1755, [N/A]"; exec sleep 60 ;;
esac
"""
CLOCKS = """GPU: 0
    CLOCK:
        GFX_0:
            CLK: 132 MHz
            MIN_CLK: 500 MHz
            MAX_CLK: 1680 MHz
            CLK_LOCKED: DISABLED
        GFX_1:
            CLK: 1679 MHz
            MIN_CLK: 500 MHz
            MAX_CLK: 1680 MHz
        MEM_0:
            CLK: 900 MHz
            MIN_CLK: 900 MHz
            MAX_CLK: 1300 MHz
"""


def executable(path, text):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text)
    path.chmod(path.stat().st_mode | stat.S_IXUSR)


def dispatch(name, start=0, shape=SHAPE):
    fields = dict(zip(collect_hardware.SHAPE_FIELDS, shape))
    return {"Kernel_Name": name, **fields, "Start_Timestamp": start, "Duration_ns": 10 + start}


class PlanTest(unittest.TestCase):
    def test_series_label_round_trips_through_the_reference_builder(self):
        for parameters in ({}, REFERENCE["series"][0]["parameters"], {"flops_per_byte": 0.5}):
            self.assertEqual(build_reference.fixed_parameters(series_label(parameters)), parameters)

    def test_plan_keeps_what_the_builder_and_the_collector_need(self):
        plan = plan_from_references([REFERENCE])
        self.assertEqual([case["case"] for case in plan], [7, 8])
        self.assertEqual(plan[0]["expected_launches"], [[KERNEL, *SHAPE]])
        environment = {
            "BENCH_PARAM_block_size": "256",
            "BENCH_PARAM_pattern": "no_conflict",
            "BENCH_PARAM_pinned": "true",
            "BENCH_PARAM_size": "128",
        }
        self.assertEqual(case_environment(plan[1], None), environment)
        self.assertEqual(
            case_environment(plan[1], "false"), {**environment, "BENCH_PARAM_vector_add_warmup": "false"}
        )

    def test_repeated_case_identifiers_are_rejected(self):
        with self.assertRaises(ValueError):
            plan_from_references([REFERENCE, REFERENCE])


class DispatchTest(unittest.TestCase):
    def test_the_timed_launch_follows_an_untimed_primer_of_the_same_kernel(self):
        dispatches = [dispatch("setup(int)"), dispatch(KERNEL, 1), dispatch(KERNEL, 2)]
        case = {"expected_launches": [[KERNEL, *SHAPE]]}
        measured = measured_dispatches(dispatches, case, "rocprofv3")
        self.assertEqual(measured, [dispatches[2]])
        self.assertEqual(unselected_look_alikes(dispatches, measured), 1)
        self.assertEqual(unselected_look_alikes(dispatches[2:], measured), 0)

    def test_expected_launches_are_matched_in_order_from_the_end(self):
        a, b = "a(int)", "b(int)"
        dispatches = [dispatch(a, 1), dispatch(b, 2), dispatch(a, 3), dispatch(b, 4)]
        select = lambda *names: measured_dispatches(  # noqa: E731
            dispatches, {"expected_launches": [[name, *SHAPE] for name in names]}, "rocprofv3"
        )
        self.assertEqual(select(a, b), dispatches[2:])
        self.assertEqual(select(b, a), dispatches[1:3])
        self.assertEqual(select(a, b, a, b), dispatches)
        self.assertIsNone(select(a, a, a))
        self.assertIsNone(select(b, b, a))
        wrong_shape = {"expected_launches": [[a, 512, 1, 1, 256, 1, 1]]}
        self.assertIsNone(measured_dispatches(dispatches, wrong_shape, "rocprofv3"))

    def test_other_cases_drop_helper_kernels_or_use_their_pattern(self):
        dispatches = [
            dispatch("__amd_rocclr_fillBufferUnAligned"),
            dispatch("bench_warmer_kernel(float*, unsigned long long, int)"),
            dispatch("__amd_rocclr_copyBuffer"),
            dispatch("ic_empty"),
        ]
        self.assertEqual(measured_dispatches(dispatches, {}, "rocprofv3"), dispatches[2:])
        self.assertEqual(measured_dispatches(dispatches, {"kernel_pattern": "^ic_"}, "rocprofv3"), dispatches[3:])
        # Reported times carry no launch shape, so expected launches cannot be matched.
        case = {"expected_launches": [["ic_empty", *SHAPE]]}
        self.assertEqual(measured_dispatches(dispatches[3:], case, "reported"), dispatches[3:])

    def test_reported_dispatches_ignore_everything_but_kernel_records(self):
        output = 'Device: x\n{"type":"kernel","name":"k","time_ms":1.5}\n{"type":"summary","total_time_ms":1.5}\n{'
        self.assertEqual(
            [(d["Kernel_Name"], d["Duration_ns"]) for d in reported_dispatches(output)], [("k", 1500000)]
        )

    def test_every_gfx_clock_of_a_report_has_a_limit_or_none(self):
        self.assertEqual(gfx_clock_limits(CLOCKS), [1680, 1680])
        # A clock that reports no frequency still has its limit read ...
        second = "MAX_CLK: 1680 MHz\n        MEM"
        partly = CLOCKS.replace("CLK: 1679 MHz", "CLK: N/A").replace(second, second.replace("1680", "1900"))
        self.assertEqual(gfx_clock_limits(partly), [1680, 1900])
        # ... and one without a readable limit is not dropped from the list.
        self.assertEqual(gfx_clock_limits(partly.replace("MAX_CLK: 1900 MHz", "MAX_CLK: N/A")), [1680, None])
        self.assertEqual(gfx_clock_limits("FileNotFoundError: amd-smi"), [])


@unittest.skipUnless(shutil.which("zstd"), "needs zstd")
class CollectionTest(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.root)
        self.binary = self.root / "benchmarks/tier1/example/hip/example"
        executable(self.binary, BENCHMARK)
        executable(self.root / "bin/rocprofv3", PROFILER)
        self.plan = self.root / "plan.json"
        self.plan.write_text(json.dumps(plan_from_references([REFERENCE])))
        self.output = self.root / "collection"
        # One AMD GPU: a clock sensor, a power sensor and a performance level.
        device = self.root / "sys/card0/device"
        (device / "hwmon/hwmon0").mkdir(parents=True)
        (device / "hwmon/hwmon0/freq1_input").write_text("1679000000\n")
        (device / "hwmon/hwmon0/power1_input").write_text("250000000\n")
        self.level = device / "power_dpm_force_performance_level"
        self.level.write_text("auto\n")
        self.cap = self.root / "cap"
        self.cap.write_text("2100\n")
        executable(self.root / "bin/amd-smi", AMD_SMI.format(state=self.cap, level=self.level))
        for change in (
            patch.dict(os.environ, {"PATH": f"{self.root / 'bin'}{os.pathsep}{os.environ['PATH']}"}),
            patch.object(collect_hardware, "SYSFS", self.root / "sys"),
            patch.object(collect_hardware, "TELEMETRY_START_SECONDS", 2),
        ):
            change.start()
            self.addCleanup(change.stop)

    def collect(self, *extra):
        return main(
            ["run", str(self.plan), "--benchmarks", str(self.root / "benchmarks"), "--output", str(self.output),
             "--reps", "3", "--platform", "rocm", "--monitor-interval", "0.01",
             *(() if "--no-revision" in extra else ("--benchmarks-revision", REVISION)),
             *(argument for argument in extra if argument != "--no-revision")]
        )

    def log(self):
        return [json.loads(line) for line in (self.output / "collection-current.log").read_text().splitlines()]

    def reference(self, archives="archives"):
        self.assertEqual(main(["pack", str(self.output), str(self.root / archives)]), 0)
        return build_reference.build(json.loads(self.plan.read_text()), self.root / archives)

    def test_profiler_collection_builds_a_reference(self):
        self.assertEqual(self.collect(), 0)
        rows = self.log()
        self.assertEqual([row["status"] for row in rows], ["ok"] * 6)
        # The stand-in profiler shows an untimed primer before the timed launch.
        self.assertEqual({row["unselected_look_alike_dispatches"] for row in rows}, {1})
        [series] = self.reference()
        self.assertEqual(series["parameters"], REFERENCE["series"][0]["parameters"])
        self.assertEqual(
            [(case["case"], case["repetitions"], case["launches"]) for case in series["cases"]],
            [(7, 3, [[KERNEL, *SHAPE, 64]]), (8, 3, [[KERNEL, *SHAPE, 128]])],
        )
        trace = self.output / "dispatch/example/7/1/trace"
        self.assertEqual(len(profiler_dispatches(trace)), 0)  # the raw profiler directory is removed
        self.assertEqual(len((trace / "all-dispatches.csv").read_text().splitlines()), 5)
        record = json.loads((self.output / "environment.jsonl").read_text())
        self.assertEqual(record["rocprofv3_version"], "stand-in profiler")
        self.assertEqual(record["gpu"], "card0 device")
        self.assertEqual(record["benchmarks_revision"], REVISION)

    def test_relative_paths_work_although_benchmarks_run_in_their_own_directory(self):
        previous = os.getcwd()
        self.addCleanup(os.chdir, previous)
        os.chdir(self.root)
        self.assertEqual(
            main(["run", "plan.json", "--benchmarks", "benchmarks", "--output", "collection", "--reps", "1",
                  "--platform", "rocm", "--monitor-interval", "0.01", "--benchmarks-revision", REVISION]),
            0,
        )
        self.assertEqual([row["status"] for row in self.log()], ["ok", "ok"])

    def test_reported_times_are_collected_without_a_profiler(self):
        self.assertEqual(self.collect("--timer", "reported", "--vector-add-warmup", "false"), 0)
        [series] = self.reference()
        # The stand-in benchmark prints 0.64 ms and 0.128 ms.
        self.assertEqual([case["launches"][0][-1] for case in series["cases"]], [640000, 128000])
        record = json.loads((self.output / "environment.jsonl").read_text())
        self.assertEqual(record["vector_add_warmup"], "false")
        self.assertEqual(record["timer"], "reported")

    def test_failures_are_logged_and_only_missing_repetitions_run_again(self):
        with patch.dict(os.environ, {"FAIL_LARGE": "1"}):
            self.assertEqual(self.collect(), 1)
        self.assertEqual([row["status"] for row in self.log()], ["ok"] * 3 + ["failed"] * 3)
        self.assertTrue((self.output / "dispatch/example/8/1/trace/stderr.txt").exists())
        self.assertEqual(self.collect(), 0)
        self.assertEqual([(row["case_id"], row["status"]) for row in self.log()[6:]], [(8, "ok")] * 3)
        [series] = self.reference()
        self.assertEqual([case["repetitions"] for case in series["cases"]], [3, 3])

    def test_a_collection_is_not_continued_with_different_settings(self):
        self.assertEqual(self.collect("--benchmark", "example"), 0)
        rows = len(self.log())
        for changed in (["--vector-add-warmup", "false"], ["--timer", "reported"], ["--sclk-max", "1680"]):
            with self.assertRaisesRegex(SystemExit, "different"):
                self.collect(*changed)
        self.plan.write_text(self.plan.read_text() + "\n")
        with self.assertRaisesRegex(SystemExit, "different plan_sha256"):
            self.collect()
        self.assertEqual(len(self.log()), rows)
        self.assertEqual(self.cap.read_text(), "2100\n")  # refused before the clock was touched

    def test_only_the_plan_and_the_warmer_setting_reach_the_benchmark(self):
        seen = self.root / "seen"
        report = 'echo "[$BENCH_PARAM_num_accesses|$BENCH_PARAM_vector_add_warmup|$BENCH_PARAM_size]"'
        executable(self.binary, f"#!/bin/sh\n{report} >> {seen}\n" + BENCHMARK.split("\n", 1)[1])
        stray = {"BENCH_PARAM_num_accesses": "7", "BENCH_PARAM_vector_add_warmup": "false", "BENCH_PARAM_size": "1"}
        with patch.dict(os.environ, stray):
            self.assertEqual(self.collect("--reps", "1", "--benchmark", "example"), 0)
            self.output = self.root / "warm"
            self.assertEqual(self.collect("--reps", "1", "--vector-add-warmup", "true"), 0)
        self.assertEqual(seen.read_text().split(), ["[||64]", "[||128]", "[|true|64]", "[|true|128]"])
        self.assertEqual(json.loads((self.output / "collection.json").read_text())["vector_add_warmup"], "true")
        with self.assertRaises(SystemExit):  # no general way to set a benchmark parameter
            self.collect("--set", "num_accesses=9")

    def test_the_warmer_setting_cannot_replace_one_the_plan_makes(self):
        plan = plan_from_references([REFERENCE])
        plan[1]["series"] += ", vector_add_warmup=true"
        self.plan.write_text(json.dumps(plan))
        with self.assertRaisesRegex(SystemExit, "Case 8 sets vector_add_warmup itself"):
            self.collect("--vector-add-warmup", "false")
        self.assertFalse(self.output.exists())
        self.assertEqual(self.collect("--reps", "1"), 0)

    def test_a_plan_is_checked_before_anything_is_created(self):
        victim = self.root / "victim/1/trace"
        victim.mkdir(parents=True)
        invalid = (
            ("case", str(self.root / "victim")),  # would become a directory outside the collection
            ("case", "7"),
            ("case", True),
            ("benchmark", "../example"),
            ("series", "block_size"),
            ("series", "block size=1"),
            ("series", "block_size=[1, 2]"),
            ("axis", "si=ze"),
            ("x", None),
            ("kernel_pattern", "("),
            ("expected_launches", [[KERNEL, 256]]),
            ("expected_launches", None),
        )
        for field, value in invalid:
            plan = plan_from_references([REFERENCE])
            plan[0][field] = value
            self.plan.write_text(json.dumps(plan))
            with self.assertRaisesRegex(SystemExit, "invalid case", msg=f"{field}={value!r}"):
                self.collect("--benchmark", "example")
        for plan, message in (({"cases": []}, "not a list of cases"), ([7], "not a list of cases")):
            self.plan.write_text(json.dumps(plan))
            with self.assertRaisesRegex(SystemExit, message):
                self.collect()
        self.assertTrue(victim.exists())
        self.assertFalse(self.output.exists())

    def test_a_case_identifier_cannot_be_used_twice(self):
        plan = plan_from_references([REFERENCE])
        other = {**plan[1], "benchmark": "other", "case": 7}
        self.plan.write_text(json.dumps(plan + [other]))
        # Refused although the filter would leave only one of the two.
        with self.assertRaisesRegex(SystemExit, "more than one case 7"):
            self.collect("--benchmark", "example")
        other["unsupported_reason"] = "not simulated"
        self.plan.write_text(json.dumps(plan + [other]))
        self.assertEqual(self.collect("--reps", "1"), 0)

    def test_requests_that_would_collect_nothing_are_refused(self):
        for arguments, message in (
            (["--reps", "0"], "must be positive"),
            (["--timeout", "0"], "must be positive"),
            (["--monitor-interval", "0"], "must be positive"),
            (["--sclk-max", "0"], "--sclk-max must be positive"),
            (["--benchmark", "example", "--benchmark", "exampel"], "no benchmark named exampel"),
        ):
            with self.assertRaisesRegex(SystemExit, message):
                self.collect(*arguments)
        with patch.object(collect_hardware.shutil, "which", return_value=None):
            with self.assertRaisesRegex(SystemExit, "rocprofv3 is not on the PATH"):
                self.collect()
        self.assertFalse(self.output.exists())
        self.assertFalse(Path(f"{self.cap}.log").exists())

    def test_data_without_its_settings_is_not_adopted(self):
        self.assertEqual(self.collect("--reps", "1"), 0)
        (self.output / "collection.json").unlink()
        rows = self.log()
        for arguments in ([], ["--sclk-max", "1680"]):
            with self.assertRaisesRegex(SystemExit, "collection data but no collection.json"):
                self.collect(*arguments)
        self.assertEqual(self.log(), rows)
        self.assertFalse(Path(f"{self.cap}.log").exists())  # refused before the clock was touched
        self.assertFalse((self.output / "collection.json").exists())

    def test_a_damaged_log_is_refused_before_the_clock_is_touched(self):
        self.assertEqual(self.collect("--reps", "1", "--sclk-max", "1680"), 0)
        log = self.output / "collection-current.log"
        Path(f"{self.cap}.log").unlink()
        rows = log.read_text()
        for damage in ('{"case_id": 7', '{"case_id": 7, "rep": 2}', "[]"):
            log.write_text(rows + damage + "\n")
            with self.assertRaisesRegex(SystemExit, "cannot be read as a collection log"):
                self.collect("--reps", "2", "--sclk-max", "1680")
        self.assertFalse(Path(f"{self.cap}.log").exists())

    def test_the_benchmark_revision_must_be_known(self):
        with self.assertRaisesRegex(SystemExit, "--benchmarks-revision"):
            self.collect("--no-revision")
        with self.assertRaisesRegex(SystemExit, "--benchmarks-revision"):
            self.collect("--no-revision", "--benchmarks-revision", "main")
        self.assertFalse(self.output.exists())

    def test_a_launch_without_a_positive_duration_is_collected_again(self):
        with patch.dict(os.environ, {"ZERO_DURATION": "1"}):
            self.assertEqual(self.collect("--reps", "1"), 1)
        self.assertEqual([row["status"] for row in self.log()], ["invalid_duration"] * 2)
        self.assertFalse((self.output / "dispatch/example/7/1/trace/measured-dispatches.csv").exists())
        self.assertEqual(self.collect("--reps", "1"), 0)
        self.assertEqual([row["status"] for row in self.log()[2:]], ["ok", "ok"])

    def test_a_repetition_that_outlives_the_sampler_is_collected_again(self):
        sensor = self.root / "sys/card0/device/hwmon/hwmon0/freq1_input"
        executable(self.binary, f"#!/bin/sh\necho broken > {sensor}\nsleep 0.3\n" + BENCHMARK.split("\n", 1)[1])
        with self.assertRaisesRegex(RuntimeError, "sampling stopped"):
            self.collect("--reps", "1")
        self.assertEqual(self.log(), [])
        sensor.write_text("1679000000\n")
        executable(self.binary, BENCHMARK)
        self.assertEqual(self.collect("--reps", "1"), 0)
        self.assertEqual([(row["case_id"], row["status"]) for row in self.log()], [(7, "ok"), (8, "ok")])

    def test_a_sampler_that_falls_silent_stops_the_collection(self):
        executable(self.root / "bin/nvidia-smi", NVIDIA_SMI)
        executable(self.binary, "#!/bin/sh\nsleep 0.6\n" + BENCHMARK.split("\n", 1)[1])
        with patch.object(collect_hardware, "TELEMETRY_STALE_SECONDS", 0.3):
            # One sample arrived, so the collection starts; none follows.
            with self.assertRaisesRegex(RuntimeError, "sampling stopped"):
                self.collect("--platform", "cuda", "--reps", "1")
        self.assertEqual(self.log(), [])
        self.assertEqual(len((self.output / "clock-monitor.csv").read_text().splitlines()), 2)
        # A pause counts even when samples arrive again before the benchmark returns.
        resumed = 'echo "1755, 90"; sleep 0.6; while :; do echo "1755, 90"; sleep 0.05; done'
        executable(self.root / "bin/nvidia-smi", NVIDIA_SMI.replace('echo "1755, [N/A]"; exec sleep 60', resumed))
        executable(self.binary, "#!/bin/sh\nsleep 1.2\n" + BENCHMARK.split("\n", 1)[1])
        self.output = self.root / "paused"
        with patch.object(collect_hardware, "TELEMETRY_STALE_SECONDS", 0.3):
            with self.assertRaisesRegex(RuntimeError, "stopped or paused"):
                self.collect("--platform", "cuda", "--reps", "1")
        self.assertEqual(self.log(), [])
        self.assertGreater(len((self.output / "clock-monitor.csv").read_text().splitlines()), 5)
        # With samples arriving, a repetition shorter than their interval is accepted without one.
        executable(self.binary, BENCHMARK)
        self.output = self.root / "sampled"
        self.assertEqual(self.collect("--reps", "1", "--monitor-interval", "1", "--timer", "reported"), 0)
        self.assertEqual({row["core_clock_samples"] for row in self.log()}, {0})

    def test_a_timeout_ends_the_benchmark_and_skips_the_larger_cases(self):
        child = self.root / "child.pid"
        executable(self.binary, f"#!/bin/sh\nsleep 60 &\necho $! > {child}\nwait\n")
        self.assertEqual(self.collect("--timeout", "0.5", "--reps", "2"), 1)
        self.assertEqual(
            [(row["case_id"], row["status"]) for row in self.log()],
            [(7, "timeout"), (7, "skipped_after_timeout"), (8, "skipped_after_timeout"), (8, "skipped_after_timeout")],
        )
        self.assert_gone(child)

    def assert_gone(self, pid_file):
        # The process the benchmark started is gone too, not only the benchmark.
        with self.assertRaises(ProcessLookupError):
            for _ in range(50):
                os.kill(int(pid_file.read_text()), 0)
                time.sleep(0.1)

    def test_an_interrupted_repetition_leaves_no_benchmark_behind(self):
        child = self.root / "child.pid"
        executable(self.binary, f"#!/bin/sh\nsleep 60 &\necho $! > {child}\nwait\n")
        communicate, calls = subprocess.Popen.communicate, []

        def interrupted(process, *args, **kwargs):
            calls.append(process)
            if len(calls) == 1:
                while not child.exists() or not child.read_text().strip():
                    time.sleep(0.01)
                raise KeyboardInterrupt
            return communicate(process, *args, **kwargs)

        directory = self.root / "trace"
        directory.mkdir()
        with patch.object(subprocess.Popen, "communicate", interrupted):
            with self.assertRaises(KeyboardInterrupt):
                run_once(self.binary, {}, "reported", directory, 30)
        self.assert_gone(child)

    def test_a_run_without_the_expected_launch_is_not_a_measurement(self):
        plan = json.loads(self.plan.read_text())
        plan[0]["expected_launches"][0][1] = 512
        self.plan.write_text(json.dumps(plan))
        self.assertEqual(self.collect("--benchmark", "example"), 1)
        self.assertEqual([row["status"] for row in self.log()], ["launch_mismatch"] * 3 + ["ok"] * 3)
        self.assertFalse((self.output / "dispatch/example/7/1/trace/measured-dispatches.csv").exists())

    def test_an_unbuilt_benchmark_stops_the_collection_before_it_starts(self):
        self.binary.unlink()
        with self.assertRaises(FileNotFoundError):
            self.collect()
        self.assertFalse(self.output.exists())

    def test_collection_needs_one_gpu_whose_clock_can_be_sampled(self):
        sensor = self.root / "sys/card0/device/hwmon/hwmon0/freq1_input"
        shutil.copytree(self.root / "sys/card0", self.root / "sys/card1")
        with self.assertRaisesRegex(SystemExit, "exactly one AMD GPU; amd-smi lists 1 and 2 have a clock sensor"):
            self.collect()
        shutil.rmtree(self.root / "sys/card1")
        # Two GPUs of which only one exposes a clock sensor, without any clock cap.
        with patch.dict(os.environ, {"SECOND_GPU": "1"}):
            with self.assertRaisesRegex(SystemExit, "exactly one AMD GPU; amd-smi lists 2 and 1 have"):
                self.collect()
        sensor.unlink()
        with self.assertRaisesRegex(SystemExit, "exactly one AMD GPU; amd-smi lists 1 and 0 have"):
            self.collect()
        self.assertFalse(self.output.exists())
        # A sensor that cannot be read yields no sample.
        sensor.write_text("not a number\n")
        with self.assertRaisesRegex(SystemExit, "cannot be sampled"):
            self.collect()
        self.assertFalse((self.output / "collection-current.log").exists())
        self.assertFalse((self.output / "collection.json").exists())  # nothing binds the directory yet

    def test_the_clock_cap_holds_during_the_collection_and_is_lifted_after(self):
        executable(self.binary, f"#!/bin/sh\ncat {self.cap} >> {self.root}/seen\n" + BENCHMARK.split("\n", 1)[1])
        self.assertEqual(self.collect("--sclk-max", "1680", "--reps", "1"), 0)
        self.assertEqual((self.root / "seen").read_text(), "1680\n1680\n")
        self.assertEqual(Path(f"{self.cap}.log").read_text(), "set 1680\nset 2100\n")
        self.assertEqual(self.cap.read_text(), "2100\n")
        self.assertEqual(self.level.read_text(), "auto")
        self.assertEqual({row["status"] for row in self.log()}, {"ok"})
        record = json.loads((self.output / "environment.jsonl").read_text())
        self.assertEqual(record["core_clock_limit_mhz"], 1680)
        self.assertIn("MAX_CLK: 2100 MHz", record["clocks_before_cap"])  # what was found, not the cap

    def test_no_clock_is_capped_unless_it_is_one_gpu_whose_state_can_be_restored(self):
        refusals = (
            ({"SECOND_GPU": "1"}, "exactly one AMD GPU; amd-smi lists 2"),
            ({"UNEQUAL_LIMITS": "1"}, r"one common limit to restore.*\[2100, 1900\]"),
            ({"UNREADABLE_LIMIT": "1"}, r"one common limit to restore.*\[2100, None\]"),
        )
        for environment, message in refusals:
            with patch.dict(os.environ, environment):
                with self.assertRaisesRegex(SystemExit, message):
                    self.collect("--sclk-max", "1680")
        if os.geteuid():  # root can write anything
            self.level.chmod(0o444)  # readable, but restoring it would fail
            with self.assertRaisesRegex(SystemExit, "performance level that can be written back"):
                self.collect("--sclk-max", "1680")
            self.level.chmod(0o644)
        self.level.write_text("\n")
        with self.assertRaisesRegex(SystemExit, "is empty"):
            self.collect("--sclk-max", "1680")
        self.level.unlink()
        with self.assertRaisesRegex(SystemExit, "one performance level to restore; found 0"):
            self.collect("--sclk-max", "1680")
        with self.assertRaisesRegex(SystemExit, "does not apply on CUDA"):
            self.collect("--sclk-max", "1680", "--platform", "cuda")
        self.assertFalse(Path(f"{self.cap}.log").exists())  # no set command was issued
        self.assertFalse(self.output.exists())

    def test_a_repetition_sampled_above_the_cap_is_not_a_measurement(self):
        sensor = self.root / "sys/card0/device/hwmon/hwmon0/freq1_input"
        executable(self.binary, "#!/bin/sh\nsleep 0.3\n" + BENCHMARK.split("\n", 1)[1])
        capped = ("--sclk-max", "1500", "--benchmark", "example")
        self.assertEqual(self.collect(*capped, "--reps", "1"), 1)  # sampled at 1679 MHz
        rows = self.log()
        self.assertEqual([row["status"] for row in rows], ["clock_above_limit"] * 2)
        self.assertEqual({row["core_clock_mhz_max"] for row in rows}, {1679.0})
        trace = self.output / "dispatch/example/7/1/trace"
        self.assertFalse((trace / "measured-dispatches.csv").exists())
        self.assertTrue((trace / "all-dispatches.csv").exists())  # kept for diagnosis
        with self.assertRaisesRegex(SystemExit, "no successful repetition"):
            main(["pack", str(self.output), str(self.root / "archives")])
        # Within the tolerance of the cap, the same repetition is collected again and counts.
        sensor.write_text("1520000000\n")
        self.assertEqual(self.collect(*capped, "--reps", "2"), 0)
        self.assertEqual(
            [(row["case_id"], row["rep"], row["status"]) for row in self.log()[2:]],
            [(7, 1, "ok"), (7, 2, "ok"), (8, 1, "ok"), (8, 2, "ok")],
        )
        [series] = self.reference()
        self.assertEqual([case["repetitions"] for case in series["cases"]], [2, 2])

    def test_lifting_the_cap_is_not_cut_short_by_another_interrupt(self):
        limit = ClockLimit(1680)
        limit.__enter__()
        self.assertEqual((self.cap.read_text(), self.level.read_text()), ("1680\n", "manual\n"))
        command_output = collect_hardware.command_output

        def interrupted(command):
            if command[1] == "set":
                os.kill(os.getpid(), signal.SIGINT)  # an impatient second Ctrl-C
            return command_output(command)

        with patch.object(collect_hardware, "command_output", interrupted):
            limit.__exit__(None, None, None)
        self.assertEqual((self.cap.read_text(), self.level.read_text()), ("2100\n", "auto"))
        self.assertIs(signal.getsignal(signal.SIGINT), signal.default_int_handler)

    def test_a_cap_that_does_not_take_effect_is_rolled_back_and_nothing_runs(self):
        with patch.dict(os.environ, {"CAP_IGNORED": "1"}):
            with self.assertRaisesRegex(RuntimeError, "Could not set the GPU clock limit to 1680"):
                self.collect("--sclk-max", "1680")
        self.assertEqual(Path(f"{self.cap}.log").read_text(), "set 1680\nset 2100\n")
        self.assertEqual(self.level.read_text(), "auto")
        self.assertFalse((self.output / "collection-current.log").exists())

    def test_packing_never_adds_to_existing_archives(self):
        self.assertEqual(self.collect("--reps", "1"), 0)
        self.reference()
        with self.assertRaisesRegex(SystemExit, "already holds archives"):
            main(["pack", str(self.output), str(self.root / "archives")])
        self.assertEqual(
            sorted(path.name for path in (self.root / "archives").iterdir()),
            ["collection-complete.tar.zst", "dispatch-0001.tar.zst"],
        )
        # A failed pack publishes nothing.
        executable(self.root / "bin/zstd", "#!/bin/sh\nexit 1\n")
        with self.assertRaises(RuntimeError):
            main(["pack", str(self.output), str(self.root / "second")])
        self.assertEqual(list((self.root / "second").iterdir()), [])

    def test_only_a_complete_collection_with_a_measurement_is_packed(self):
        with patch.dict(os.environ, {"FAIL_LARGE": "1"}):
            self.assertEqual(self.collect("--reps", "1", "--benchmark", "example"), 1)
        log = self.output / "collection-current.log"
        rows = log.read_text()
        for name in ("collection-current.log", "collection.json", "environment.jsonl", "clock-monitor.csv"):
            hidden = self.output / f"{name}.hidden"
            (self.output / name).rename(hidden)
            with self.assertRaisesRegex(SystemExit, f"not a complete collection; it has no {name}"):
                main(["pack", str(self.output), str(self.root / "archives")])
            hidden.rename(self.output / name)
        log.write_text(rows.splitlines()[1] + "\n")  # only the failed repetition
        with self.assertRaisesRegex(SystemExit, "no successful repetition"):
            main(["pack", str(self.output), str(self.root / "archives")])
        self.assertFalse((self.root / "archives").exists())
        log.write_text(rows)
        [series] = self.reference()
        self.assertEqual([case["case"] for case in series["cases"]], [7])

if __name__ == "__main__":
    unittest.main()
