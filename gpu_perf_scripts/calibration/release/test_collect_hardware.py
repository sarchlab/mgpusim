import json
import os
import shutil
import stat
import tempfile
import time
import unittest
from pathlib import Path
from unittest.mock import patch

import build_reference
import collect_hardware
from collect_hardware import (
    amd_clocks,
    case_environment,
    main,
    measured_dispatches,
    plan_from_references,
    profiler_dispatches,
    reported_dispatches,
    series_label,
)

SHAPE = [256, 1, 1, 256, 1, 1]
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
                    "launches": [["example_kernel(float*, int)", *SHAPE, 1000 * size]],
                }
                for case, size in ((7, 64), (8, 128))
            ],
        }
    ]
}

# The benchmark prints its GPU event time; the stand-in profiler records a
# buffer fill, the clock warmer and the kernel, as a real run does.
BENCHMARK = """#!/bin/sh
[ "$BENCH_PARAM_size" = 128 ] && [ -n "$FAIL_LARGE" ] && exit 3
echo "{\\"type\\":\\"kernel\\",\\"name\\":\\"example_kernel\\",\\"time_ms\\":0.$BENCH_PARAM_size}"
"""
PROFILER = """#!/bin/sh
directory=$5
shift 6
"$@" || exit $?
mkdir -p "$directory/host"
cat > "$directory/host/1_kernel_trace.csv" <<EOF
"Kind","Kernel_Name","Start_Timestamp","End_Timestamp","Workgroup_Size_X","Workgroup_Size_Y","Workgroup_Size_Z","Grid_Size_X","Grid_Size_Y","Grid_Size_Z"
"KERNEL_DISPATCH","example_kernel(float*, int)",3000,$((3000 + BENCH_PARAM_size)),256,1,1,256,1,1
"KERNEL_DISPATCH","__amd_rocclr_fillBufferUnAligned",1000,1500,256,1,1,256,1,1
"KERNEL_DISPATCH","bench_warmer_kernel(float*, unsigned long long, int)",2000,2600,256,1,1,4096,1,1
EOF
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


class PlanTest(unittest.TestCase):
    def test_series_label_round_trips_through_the_reference_builder(self):
        for parameters in ({}, REFERENCE["series"][0]["parameters"], {"flops_per_byte": 0.5}):
            self.assertEqual(build_reference.fixed_parameters(series_label(parameters)), parameters)

    def test_plan_keeps_what_the_builder_and_the_collector_need(self):
        plan = plan_from_references([REFERENCE])
        self.assertEqual([case["case"] for case in plan], [7, 8])
        self.assertEqual(plan[0]["expected_launches"], [["example_kernel(float*, int)", *SHAPE]])
        self.assertEqual(
            case_environment(plan[1], {"vector_add_warmup": "false"}),
            {
                "BENCH_PARAM_block_size": "256",
                "BENCH_PARAM_pattern": "no_conflict",
                "BENCH_PARAM_pinned": "true",
                "BENCH_PARAM_size": "128",
                "BENCH_PARAM_vector_add_warmup": "false",
            },
        )

    def test_repeated_case_identifiers_are_rejected(self):
        with self.assertRaises(ValueError):
            plan_from_references([REFERENCE, REFERENCE])


class DispatchTest(unittest.TestCase):
    def dispatch(self, name, start=0, shape=SHAPE):
        fields = dict(zip(collect_hardware.SHAPE_FIELDS, shape))
        return {"Kernel_Name": name, **fields, "Start_Timestamp": start, "Duration_ns": 10}

    def test_expected_launches_select_the_measurement_in_order(self):
        kernel = "example_kernel(float*, int)"
        dispatches = [self.dispatch("setup(int)"), self.dispatch(kernel, 1), self.dispatch(kernel, 2)]
        case = {"expected_launches": [[kernel, *SHAPE]]}
        self.assertEqual(measured_dispatches(dispatches, case, "rocprofv3"), [dispatches[1]])
        case = {"expected_launches": [[kernel, *SHAPE]] * 3}
        self.assertIsNone(measured_dispatches(dispatches, case, "rocprofv3"))
        wrong_shape = {"expected_launches": [[kernel, 512, 1, 1, 256, 1, 1]]}
        self.assertIsNone(measured_dispatches(dispatches, wrong_shape, "rocprofv3"))

    def test_other_cases_drop_helper_kernels_or_use_their_pattern(self):
        dispatches = [
            self.dispatch("__amd_rocclr_fillBufferUnAligned"),
            self.dispatch("bench_warmer_kernel(float*, unsigned long long, int)"),
            self.dispatch("__amd_rocclr_copyBuffer"),
            self.dispatch("ic_empty"),
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

    def test_clock_report_is_parsed_per_gfx_clock(self):
        self.assertEqual(amd_clocks(CLOCKS), [(132, 500, 1680), (1679, 500, 1680)])


@unittest.skipUnless(shutil.which("zstd"), "needs zstd")
class CollectionTest(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.root)
        executable(self.root / "benchmarks/tier1/example/hip/example", BENCHMARK)
        executable(self.root / "bin/rocprofv3", PROFILER)
        self.plan = self.root / "plan.json"
        self.plan.write_text(json.dumps(plan_from_references([REFERENCE])))
        self.output = self.root / "collection"
        path = patch.dict(os.environ, {"PATH": f"{self.root / 'bin'}{os.pathsep}{os.environ['PATH']}"})
        path.start()
        self.addCleanup(path.stop)

    def collect(self, *extra):
        return main(
            ["run", str(self.plan), "--benchmarks", str(self.root / "benchmarks"), "--output", str(self.output),
             "--reps", "3", "--platform", "rocm", "--monitor-interval", "0.01", *extra]
        )

    def log(self):
        return [json.loads(line) for line in (self.output / "collection-current.log").read_text().splitlines()]

    def reference(self):
        archives = self.root / "archives"
        self.assertEqual(main(["pack", str(self.output), str(archives)]), 0)
        return build_reference.build(json.loads(self.plan.read_text()), archives)

    def test_profiler_collection_builds_a_reference(self):
        self.assertEqual(self.collect(), 0)
        self.assertEqual([row["status"] for row in self.log()], ["ok"] * 6)
        [series] = self.reference()
        self.assertEqual(series["parameters"], REFERENCE["series"][0]["parameters"])
        self.assertEqual(
            [(case["case"], case["repetitions"], case["launches"]) for case in series["cases"]],
            [
                (7, 3, [["example_kernel(float*, int)", *SHAPE, 64]]),
                (8, 3, [["example_kernel(float*, int)", *SHAPE, 128]]),
            ],
        )
        trace = self.output / "dispatch/example/7/1/trace"
        self.assertEqual(len(profiler_dispatches(trace)), 0)  # the raw profiler directory is removed
        self.assertEqual(len((trace / "all-dispatches.csv").read_text().splitlines()), 4)

    def test_relative_paths_work_although_benchmarks_run_in_their_own_directory(self):
        previous = os.getcwd()
        self.addCleanup(os.chdir, previous)
        os.chdir(self.root)
        self.assertEqual(
            main(["run", "plan.json", "--benchmarks", "benchmarks", "--output", "collection", "--reps", "1",
                  "--platform", "rocm", "--monitor-interval", "0.01"]),
            0,
        )
        self.assertEqual([row["status"] for row in self.log()], ["ok", "ok"])

    def test_reported_times_are_collected_without_a_profiler(self):
        self.assertEqual(self.collect("--timer", "reported", "--set", "vector_add_warmup=false"), 0)
        [series] = self.reference()
        # The stand-in benchmark prints 0.64 ms and 0.128 ms.
        self.assertEqual([case["launches"][0][-1] for case in series["cases"]], [640000, 128000])
        record = json.loads((self.output / "environment.jsonl").read_text())
        self.assertEqual(record["extra_parameters"], {"vector_add_warmup": "false"})
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

    def test_a_timeout_ends_the_benchmark_and_skips_the_larger_cases(self):
        child = self.root / "child.pid"
        executable(
            self.root / "benchmarks/tier1/example/hip/example",
            f"#!/bin/sh\nsleep 60 &\necho $! > {child}\nwait\n",
        )
        self.assertEqual(self.collect("--timeout", "0.5", "--reps", "2"), 1)
        self.assertEqual(
            [(row["case_id"], row["status"]) for row in self.log()],
            [(7, "timeout"), (7, "skipped_after_timeout"), (8, "skipped_after_timeout"), (8, "skipped_after_timeout")],
        )
        # The process the benchmark started is gone too, not only the benchmark.
        with self.assertRaises(ProcessLookupError):
            for _ in range(50):
                os.kill(int(child.read_text()), 0)
                time.sleep(0.1)

    def test_a_run_without_the_expected_launch_is_not_a_measurement(self):
        plan = json.loads(self.plan.read_text())
        plan[0]["expected_launches"][0][1] = 512
        self.plan.write_text(json.dumps(plan))
        self.assertEqual(self.collect("--benchmark", "example"), 1)
        self.assertEqual([row["status"] for row in self.log()], ["launch_mismatch"] * 3 + ["ok"] * 3)
        self.assertFalse((self.output / "dispatch/example/7/1/trace/measured-dispatches.csv").exists())

    def test_an_unbuilt_benchmark_stops_the_collection_before_it_starts(self):
        (self.root / "benchmarks/tier1/example/hip/example").unlink()
        with self.assertRaises(FileNotFoundError):
            self.collect()
        self.assertFalse(self.output.exists())


if __name__ == "__main__":
    unittest.main()
