import contextlib
import copy
import io
import json
import os
import sys
import unittest
from unittest.mock import patch

import collect_hardware
from collect_hardware import plan_from_references
from test_collect_hardware import BENCHMARK, KERNEL, REFERENCE, SHAPE, CollectionFixture, executable

try:
    import psycopg

    import hardware_report
    import report_db
except ImportError:
    psycopg = None

DSN = "postgresql://writer:s3cret@db.example/reports"
REPORT = ("--report-run", "run-1", "--report-device", "dev=Test Device")
FIXED = {"block_size": 256, "pattern": "no_conflict", "pinned": True}


def point(size, value, **status):
    result = {
        "benchmark": "example",
        "group": "dev",
        "x": size,
        "non_scaling_params": {"launch": "#1 example_kernel", **FIXED},
        "values": {"dev": value},
    }
    if status:
        result["statuses"] = {"dev": status}
    return result


class ReportOptionsTest(CollectionFixture):
    """Refusals that need neither the report database nor its client library."""

    def test_the_report_options_are_checked_before_anything_is_created(self):
        refusals = (
            (REPORT[:2], {"CALIBRATION_REPORT_DSN": DSN}, "--report-run needs --report-device"),
            (REPORT[2:], {"CALIBRATION_REPORT_DSN": DSN}, "--report-device needs --report-run"),
            (REPORT, {"CALIBRATION_REPORT_DSN": " "}, "CALIBRATION_REPORT_DSN"),
        )
        for options, environment, message in refusals:
            with patch.dict(os.environ, environment):
                with self.assertRaisesRegex(SystemExit, message):
                    self.collect(*options)
        # Without the database client the module that talks to the report cannot be loaded.
        with patch.dict(os.environ, {"CALIBRATION_REPORT_DSN": DSN}):
            with patch.dict(sys.modules, {"hardware_report": None}):
                with self.assertRaisesRegex(SystemExit, "--report-run needs psycopg"):
                    self.collect(*REPORT)
        with contextlib.redirect_stderr(io.StringIO()), self.assertRaises(SystemExit):
            self.collect("--report-run", "run-1", "--report-device", "no name")
        self.assertFalse(self.output.exists())

    def test_a_collection_without_a_report_does_not_load_the_database_client(self):
        with patch.dict(sys.modules, {"hardware_report": None, "report_db": None}):
            self.assertEqual(self.collect("--reps", "1"), 0)
        record = json.loads((self.output / "environment.jsonl").read_text())
        self.assertIsNone(record["report_run"])


@unittest.skipUnless(psycopg, "needs psycopg")
class ReportTest(CollectionFixture):
    def setUp(self):
        super().setUp()
        self.calls, self.failure = [], None

        def call(sql, *arguments, deadline):
            self.calls.append((sql.split("(")[0].split(".")[1], *copy.deepcopy(arguments)))
            if self.failure:
                raise self.failure

        for change in (
            patch.dict(os.environ, {"CALIBRATION_REPORT_DSN": DSN}),
            patch.object(report_db, "call", call),
        ):
            change.start()
            self.addCleanup(change.stop)

    def progress(self, call):
        return {key: call[-1][key] for key in ("total", "completed", "unsuccessful", "running", "queued")}

    def test_a_collection_is_shown_case_by_case_and_completed(self):
        self.assertEqual(self.collect(*REPORT), 0)
        opened, first, second, finished = self.calls
        self.assertEqual(opened[:2], ("open_run", "run-1"))
        self.assertEqual(opened[2]["title"], "Test Device hardware collection")
        self.assertEqual(opened[2]["groups"], {"dev": {"name": "Test Device"}})
        self.assertIn("3 repetitions per case, rocprofv3 timer, core clock not capped", opened[2]["description"])
        self.assertEqual(
            opened[3],
            {
                "example": {
                    "scaling_param": "size",
                    "params": {name: {"unit": ""} for name in ("size", "launch", *FIXED)},
                    "values": {
                        "dev": {
                            "name": "Test Device kernel time (median)",
                            "unit": "ns",
                            "role": "hardware_measurement",
                        }
                    },
                }
            },
        )
        zero = {"total": 2, "completed": 0, "unsuccessful": 0, "running": 0, "queued": 2}
        self.assertEqual(self.progress(opened), zero)
        # The stand-in profiler reports a launch of as many nanoseconds as the case's size.
        self.assertEqual(first[:3], ("put_points", "run-1", [point(64, 64)]))
        self.assertEqual(self.progress(first), {**zero, "completed": 1, "queued": 1})
        self.assertEqual(second[:3], ("put_points", "run-1", [point(128, 128)]))
        self.assertEqual(finished[:3], ("finish_run", "run-1", "completed"))
        self.assertEqual(self.progress(finished), {**zero, "completed": 2, "queued": 0})
        self.assertRegex(opened[4]["started_at"], r"^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\+00:00$")
        record = json.loads((self.output / "environment.jsonl").read_text())
        self.assertEqual(record["report_run"], "run-1")

    def test_a_case_without_a_measurement_says_why_and_a_later_invocation_completes_the_run(self):
        with patch.dict(os.environ, {"FAIL_LARGE": "1"}):
            self.assertEqual(self.collect(*REPORT), 1)
        _, _, failed, finished = self.calls
        self.assertEqual(failed[2], [point(128, None, state="failed", reason="failed")])
        self.assertEqual(finished[2], "failed")
        self.assertEqual(
            self.progress(finished), {"total": 2, "completed": 1, "unsuccessful": 1, "running": 0, "queued": 0}
        )
        del self.calls[:]
        self.assertEqual(self.collect(*REPORT), 0)
        opened, resent, repaired, finished = self.calls
        # The case finished before is sent again; the failed one is waiting, not unsuccessful.
        self.assertEqual(
            self.progress(opened), {"total": 2, "completed": 1, "unsuccessful": 0, "running": 0, "queued": 1}
        )
        self.assertEqual(resent[2], [point(64, 64)])
        self.assertEqual(repaired[2], [point(128, 128)])
        self.assertEqual(finished[2], "completed")

    def test_some_successful_repetitions_give_a_median_but_not_a_complete_case(self):
        flag = self.root / "failed-once"
        fail_once = f"#!/bin/sh\n[ -e {flag} ] || {{ touch {flag}; exit 3; }}\n"
        executable(self.binary, fail_once + BENCHMARK.split("\n", 1)[1])
        self.assertEqual(self.collect(*REPORT, "--benchmark", "example"), 1)
        self.assertEqual(self.calls[1][2], [point(64, 64)])
        self.assertEqual(self.calls[-1][2], "failed")
        self.assertEqual(self.progress(self.calls[-1])["unsuccessful"], 1)

    def test_an_invocation_limited_to_a_benchmark_does_not_complete_the_run(self):
        reference = copy.deepcopy(REFERENCE)
        other = copy.deepcopy(reference["series"][0])
        other.update(benchmark="other", sample="other", cases=[{**other["cases"][0], "case": 9}])
        reference["series"].append(other)
        self.plan.write_text(json.dumps(plan_from_references([reference])))
        executable(self.root / "benchmarks/tier1/other/hip/other", BENCHMARK)
        self.assertEqual(self.collect(*REPORT, "--benchmark", "example"), 0)
        self.assertEqual(sorted(self.calls[0][3]), ["example", "other"])
        self.assertEqual(self.calls[-1][2], "failed")
        self.assertEqual(
            self.progress(self.calls[-1]), {"total": 3, "completed": 2, "unsuccessful": 0, "running": 0, "queued": 1}
        )
        del self.calls[:]
        self.assertEqual(self.collect(*REPORT, "--benchmark", "other"), 0)
        opened, resent, last, finished = self.calls
        self.assertEqual(self.progress(opened)["completed"], 2)
        self.assertEqual([entry["x"] for entry in resent[2]], [64, 128])
        self.assertEqual(last[2][0]["benchmark"], "other")
        self.assertEqual(finished[2], "completed")

    def test_an_interrupted_collection_leaves_the_run_failed(self):
        with patch.object(collect_hardware, "run_once", side_effect=KeyboardInterrupt):
            with self.assertRaises(KeyboardInterrupt):
                self.collect(*REPORT)
        self.assertEqual([call[0] for call in self.calls], ["open_run", "finish_run"])
        self.assertEqual(self.calls[-1][2], "failed")

    def test_a_report_that_cannot_be_written_does_not_stop_the_collection(self):
        self.failure = psycopg.ProgrammingError(f'missing "=" after "{DSN}" in connection info string')
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            self.assertEqual(self.collect(*REPORT), 0)
        self.assertEqual([call[0] for call in self.calls], ["open_run"])  # nothing is sent after the failure
        self.assertEqual([row["status"] for row in self.log()], ["ok"] * 6)
        self.assertIn("The calibration report is not updated any more. ProgrammingError", output.getvalue())
        self.assertNotIn("s3cret", output.getvalue())
        self.assertNotIn("db.example", output.getvalue())

    def test_points_without_a_usable_median(self):
        report = hardware_report.HardwareReport("run-1", ("dev", "Test Device"), [], 3, "")
        case = plan_from_references([REFERENCE])[0]
        dispatch = dict(zip(collect_hardware.DISPATCH_FIELDS, (KERNEL, *SHAPE, 0, 10)))
        self.assertEqual(report.points(case, [], "skipped_after_timeout")[0]["statuses"]["dev"]["state"], "timeout")
        [different] = report.points(case, [[dispatch], [{**dispatch, "Grid_Size_X": 512}]], "")
        self.assertEqual(
            different["statuses"], {"dev": {"state": "failed", "reason": "the repetitions measured different launches"}}
        )
        # No name is made up for a launch nobody has seen: such a point would outlive a later success.
        self.assertEqual(report.points({**case, "expected_launches": []}, [], "failed"), [])
        [even] = report.points(case, [[dispatch], [{**dispatch, "Duration_ns": 13, "Start_Timestamp": 7}]], "")
        self.assertEqual(even["values"], {"dev": 11.5})
        self.assertEqual(
            hardware_report.base_kernel_name("void example_kernel(float*, int) [clone .kd]"), "example_kernel"
        )
        self.assertEqual(hardware_report.base_kernel_name("ic_empty.kd"), "ic_empty")


if __name__ == "__main__":
    unittest.main()
