import gzip
import json
import math
import os
import tempfile
import unittest
from argparse import Namespace
from collections import Counter
from pathlib import Path
from unittest.mock import call, patch

import psycopg
import psycopg.errors

import report_db
from workflow import (
    BUDGET_SECONDS,
    HARDWARE_VALUE_NAME,
    REFERENCES,
    SERIES_BUDGET_SKIP_REASON,
    WRITE_DEADLINE_SECONDS,
    benchmark_definitions,
    case_points,
    cfd_launch_groups,
    final_progress,
    finalize,
    matrix_rows,
    read_reference,
    run_key,
    run_series,
    series_launch_groups,
    sweep,
)

SHAPE = [64, 64, 1, 16, 16, 1]
HARDWARE = ["kernel(float*)", *SHAPE, 2000]
SIMULATED = ["kernel", *SHAPE, 1500]
SERIES = {
    "benchmark": "example",
    "sample": "example",
    "axis": "size",
    "parameters": {"block_size": 16},
    "cases": [{"case": i, "x": i * 64, "launches": [HARDWARE]} for i in range(1, 4)],
}
CFD_NAMES = [
    "copy_variables_kernel",
    "cfd_device::compute_step_factor_kernel",
    "cfd_device::compute_flux_kernel",
    "cfd_device::rk_update_kernel",
    "cfd_device::compute_flux_kernel",
    "cfd_device::rk_update_kernel",
    "cfd_device::compute_flux_kernel",
    "cfd_device::rk_update_kernel",
] * 10


def point_for(series, launches, status="complete"):
    case = series["cases"][0]
    (point,) = case_points(series, case, {"status": status, "launches": launches})
    return point


class PointTest(unittest.TestCase):
    def test_points_match_launches_and_keep_failed_verification_timing(self):
        case = SERIES["cases"][0]
        (verified,) = case_points(SERIES, case, {"status": "complete", "launches": [SIMULATED]})
        self.assertEqual(verified["values"], {"mi300x": 2000, "mgpusim": 1500})
        self.assertEqual(
            verified["non_scaling_params"], {"launch": "#1 kernel", "block_size": 16}
        )
        self.assertNotIn("statuses", verified)

        (failed,) = case_points(SERIES, case, {
            "status": "verification_failed", "reason": "Mismatch", "launches": [SIMULATED],
        })
        self.assertEqual(failed["values"]["mgpusim"], 1500)
        self.assertEqual(failed["statuses"]["mgpusim"]["state"], "verification_failed")

        unmatched = point_for(SERIES, [["other_kernel", *SHAPE, 1500]])
        self.assertIsNone(unmatched["values"]["mgpusim"])
        self.assertEqual(unmatched["statuses"]["mgpusim"]["state"], "unsupported")
        self.assertNotIn("symbol", unmatched["statuses"]["mgpusim"])

    def test_case_parameters_are_added_to_those_of_the_series(self):
        cases = [
            {**case, "parameters": {"ops_per_iter": ops}}
            for case, ops in zip(SERIES["cases"], (2, 4, 4))
        ]
        series = {**SERIES, "cases": cases}
        point = point_for(series, [SIMULATED])
        self.assertEqual(
            point["non_scaling_params"],
            {"launch": "#1 kernel", "block_size": 16, "ops_per_iter": 2},
        )
        params = benchmark_definitions([series])["example"]["params"]
        self.assertEqual(list(params), ["size", "launch", "block_size", "ops_per_iter"])

    def test_long_reasons_are_bounded_without_splitting_characters(self):
        reason = "panic: " + "é" * 600  # 1,207 UTF-8 bytes
        result = {"status": "failed", "reason": reason}
        (point,) = case_points(SERIES, SERIES["cases"][0], result)
        sent = point["statuses"]["mgpusim"]["reason"]
        self.assertLessEqual(len(sent.encode()), 1000)
        self.assertTrue(sent.endswith("…"))
        self.assertTrue(reason.startswith(sent[:-1]))
        self.assertEqual(result["reason"], reason)
        short = {"status": "failed", "reason": "é" * 500}  # exactly 1,000 bytes
        (point,) = case_points(SERIES, SERIES["cases"][0], short)
        self.assertEqual(point["statuses"]["mgpusim"]["reason"], short["reason"])

    def test_unsuccessful_cases_keep_their_status(self):
        case = SERIES["cases"][0]
        (timeout,) = case_points(SERIES, case, {"status": "timeout", "reason": "Budget expired."})
        self.assertIsNone(timeout["values"]["mgpusim"])
        self.assertEqual(timeout["values"]["mi300x"], 2000)
        self.assertEqual(
            timeout["statuses"]["mgpusim"], {"state": "timeout", "reason": "Budget expired."}
        )
        (skipped,) = case_points(
            SERIES, case, {"status": "unsupported", "reason": SERIES_BUDGET_SKIP_REASON}
        )
        self.assertEqual(skipped["statuses"]["mgpusim"]["symbol"], "↷")

    def test_lbm_exported_kernel_matches_only_with_matching_geometry(self):
        hardware = [
            "lbm::lbm_collide_stream_kernel(float const*, float*, lbm::Layout)",
            *SHAPE,
            2000,
        ]
        simulated = ["lbm_collide_stream_kernel", *SHAPE, 1500]
        different_geometry = ["lbm_collide_stream_kernel", 32, *SHAPE[1:], 1500]
        for benchmark, launch, expected in (
            ("parboil_lbm", simulated, 1500),
            ("parboil_lbm", different_geometry, None),
            ("example", simulated, None),
        ):
            with self.subTest(benchmark=benchmark, launch=launch):
                series = {
                    **SERIES,
                    "benchmark": benchmark,
                    "cases": [{"case": 1, "x": 64, "launches": [hardware]}],
                }
                self.assertEqual(point_for(series, [launch])["values"]["mgpusim"], expected)

    def test_only_the_cache_latency_primer_is_omitted(self):
        geometry = [1, 1, 1, 1, 1, 1]
        hardware = ["pointer_chase_kernel(unsigned int const*)", *geometry, 2000]
        primer = ["pointer_chase_kernel", *geometry, 100]
        measured = ["pointer_chase_kernel", *geometry, 1500]
        different_geometry = ["pointer_chase_kernel", 2, *geometry[1:], 100]
        other_kernel = ["other_kernel", *geometry, 100]
        for benchmark, launches, expected in (
            ("cache_latency", [primer, measured], 1500),
            ("cache_latency", [measured], 1500),
            ("cache_latency", [primer, primer, measured], None),
            ("cache_latency", [different_geometry, measured], None),
            ("cache_latency", [primer, different_geometry], None),
            ("cache_latency", [other_kernel, measured], None),
            ("cache_latency", [primer, other_kernel], None),
            ("example", [primer, measured], None),
        ):
            with self.subTest(benchmark=benchmark, launches=launches):
                series = {
                    **SERIES,
                    "benchmark": benchmark,
                    "cases": [{"case": 1, "x": 64, "launches": [hardware]}],
                }
                point = point_for(series, launches)
                self.assertEqual(point["values"]["mgpusim"], expected)
                if expected is None:
                    self.assertEqual(point["statuses"]["mgpusim"]["state"], "unsupported")
                else:
                    self.assertNotIn("statuses", point)


class CfdLaunchGroupTest(unittest.TestCase):
    def setUp(self):
        self.hardware = [[f"{name}(int)", *SHAPE, 2000] for name in CFD_NAMES]
        self.series = {
            **SERIES,
            "benchmark": "altis_cfd",
            "cases": [
                {"case": 1, "x": 64, "launches": self.hardware},
                {"case": 2, "x": 128, "launches": self.hardware},
            ],
        }

    def test_labels_depend_only_on_hardware(self):
        labels = cfd_launch_groups(self.series["cases"])
        self.assertEqual(
            Counter(labels),
            {
                "copy_first": 1,
                "copy": 9,
                "step_factor_first": 1,
                "step_factor": 9,
                "flux_stage_0_first": 1,
                "flux_stage_0": 9,
                "flux_stages_1_2": 20,
                "update_stage_0": 10,
                "update_stages_1_2": 20,
            },
        )
        params = benchmark_definitions([self.series])["altis_cfd"]["params"]
        self.assertEqual(list(params), ["size", "launch", "block_size", "launch_group"])

        simulated = [
            [name.removeprefix("cfd_device::"), *SHAPE, 1500 + i]
            for i, name in enumerate(CFD_NAMES)
        ]
        mismatched = [["different_kernel", *SHAPE, 1500], *simulated[1:]]
        for result in (
            {"status": "complete", "launches": simulated},
            {"status": "complete", "launches": mismatched},
            {"status": "complete", "launches": simulated[:-1]},
            {"status": "timeout", "reason": "Budget expired."},
        ):
            points = case_points(self.series, self.series["cases"][0], result, labels)
            self.assertEqual(len(points), 80)
            self.assertEqual(len({p["non_scaling_params"]["launch"] for p in points}), 80)
            self.assertEqual(
                [p["non_scaling_params"]["launch_group"] for p in points], list(labels)
            )

        points = case_points(
            self.series, self.series["cases"][0], {"status": "complete", "launches": simulated},
            labels,
        )
        self.assertEqual([p["values"]["mgpusim"] for p in points], list(range(1500, 1580)))
        points = case_points(
            self.series, self.series["cases"][0], {"status": "complete", "launches": mismatched},
            labels,
        )
        self.assertEqual(points[0]["statuses"]["mgpusim"]["state"], "unsupported")

        # Unqualified hardware symbols still match through the aliases.
        unqualified = [[f"{name.removeprefix('cfd_device::')}(int)", *SHAPE, 2000]
                       for name in CFD_NAMES]
        self.assertEqual(cfd_launch_groups([{"launches": unqualified}]), labels)

    def test_a_hardware_mismatch_drops_the_labels_of_the_benchmark(self):
        for index, value in ((0, "different_kernel(int)"), (1, 128)):
            with self.subTest(index=index):
                changed = [list(row) for row in self.hardware]
                changed[5][index] = value
                series = {
                    **self.series,
                    "cases": [*self.series["cases"], {"case": 3, "x": 256, "launches": changed}],
                }
                self.assertEqual(cfd_launch_groups(series["cases"]), ())
                self.assertNotIn(
                    "launch_group", benchmark_definitions([series])["altis_cfd"]["params"]
                )
                # Every series of the benchmark shares the declaration.
                self.assertNotIn(
                    "launch_group",
                    benchmark_definitions([self.series, series])["altis_cfd"]["params"],
                )


class IterativeLaunchGroupTest(unittest.TestCase):
    def test_repeated_dispatches_are_grouped_without_merging_them(self):
        examples = (
            (
                "rodinia_pathfinder", {"rows": 8, "pyramid_height": 3},
                ("dynproc_kernel",) * 3,
                ("dynproc_kernel_first", "dynproc_kernel", "dynproc_kernel_tail"),
            ),
            (
                "parboil_lbm", {"num_timesteps": 2},
                ("lbm::lbm_collide_stream_kernel",) * 2,
                ("lbm_collide_stream_kernel_first", "lbm_collide_stream_kernel"),
            ),
            (
                "polybench_fdtd2d", {"tmax": 2},
                ("fdtd_update_ex", "fdtd_update_ey", "fdtd_update_hz") * 2,
                (
                    "fdtd_update_ex_first", "fdtd_update_ey_first",
                    "fdtd_update_hz_first", "fdtd_update_ex", "fdtd_update_ey",
                    "fdtd_update_hz",
                ),
            ),
            (
                "polybench_jacobi2d", {"tsteps": 2},
                ("jacobi2d_kernel", "jacobi2d_copy_kernel") * 2,
                (
                    "jacobi2d_kernel_first", "jacobi2d_copy_kernel_first",
                    "jacobi2d_kernel", "jacobi2d_copy_kernel",
                ),
            ),
        )
        for bench, parameters, kernels, groups in examples:
            with self.subTest(benchmark=bench):
                hardware = [[f"{name}(int)", *SHAPE, 2000 + i] for i, name in enumerate(kernels)]
                simulated = [[name.split("::")[-1], *SHAPE, 1500 + i] for i, name in enumerate(kernels)]
                cases = [
                    {"case": 1, "x": 64, "launches": hardware},
                    {"case": 2, "x": 128, "launches": hardware},
                ]
                series = {**SERIES, "benchmark": bench, "parameters": parameters, "cases": cases}
                definition = benchmark_definitions([series])[bench]
                self.assertIn("launch_group", definition["params"])
                labels = series_launch_groups(series, definition)
                points = [
                    *case_points(series, cases[0], {"status": "complete", "launches": simulated},
                                 labels),
                    *case_points(series, cases[1], {"status": "timeout", "reason": "Budget expired."},
                                 labels),
                ]
                self.assertEqual(
                    [p["non_scaling_params"]["launch_group"] for p in points], list(groups) * 2
                )
                self.assertEqual(
                    [p["values"]["mi300x"] for p in points], [row[-1] for row in hardware] * 2
                )
                self.assertEqual(
                    [p["values"]["mgpusim"] for p in points],
                    [row[-1] for row in simulated] + [None] * len(kernels),
                )
                self.assertFalse(any("statuses" in p for p in points[:len(kernels)]))
                self.assertEqual(
                    [p["non_scaling_params"]["launch"] for p in points[:len(kernels)]],
                    [f"#{i + 1} {name}" for i, name in enumerate(kernels)],
                )

    def test_groups_fall_back_when_a_reference_series_is_inconsistent(self):
        hardware = [["lbm_collide_stream_kernel", *SHAPE, 2000] for _ in range(2)]
        first = {
            **SERIES, "benchmark": "parboil_lbm", "parameters": {"num_timesteps": 2},
            "cases": [{"case": 1, "x": 64, "launches": hardware}],
        }
        other_shape = ["lbm_collide_stream_kernel", 32, *SHAPE[1:], 2000]
        second = {
            **first, "cases": [{"case": 2, "x": 128, "launches": [hardware[0], other_shape]}],
        }
        definition = benchmark_definitions([first, second])["parboil_lbm"]
        self.assertNotIn("launch_group", definition["params"])
        for series in (first, second):
            labels = series_launch_groups(series, definition)
            self.assertEqual(labels, ())
            points = case_points(series, series["cases"][0], {"status": "timeout", "reason": "x"},
                                 labels)
            self.assertEqual(len(points), 2)
            for point in points:
                self.assertNotIn("launch_group", point["non_scaling_params"])


class ReferenceTest(unittest.TestCase):
    def test_series_of_several_references_are_concatenated_in_order(self):
        first = {**SERIES, "benchmark": "first"}
        second = {**SERIES, "benchmark": "second", "hardware": "Other machine (median)"}
        with tempfile.TemporaryDirectory() as folder:
            paths = []
            for name, series in (("a", [first, first]), ("b", [second])):
                paths.append(Path(folder) / f"{name}.json.gz")
                with gzip.open(paths[-1], "wt") as file:
                    json.dump({"benchmark_revision": name, "series": series}, file)
            reference = read_reference(paths)
        self.assertEqual(
            [(row["id"], row["benchmark"]) for row in matrix_rows(reference, "")],
            [(0, "first"), (1, "first"), (2, "second")],
        )
        self.assertEqual(matrix_rows(reference, "second"), [
            {"id": 2, "benchmark": "second", "sample": "example"},
        ])
        definitions = benchmark_definitions(reference["series"])
        names = {name: row["values"]["mi300x"]["name"] for name, row in definitions.items()}
        self.assertEqual(
            names, {"first": HARDWARE_VALUE_NAME, "second": "Other machine (median)"}
        )

    def test_checked_in_references_keep_the_v12_series_first(self):
        self.assertEqual(
            [path.name for path in REFERENCES],
            ["mi300x-v1.2.json.gz", "mi300x-code-fetch-latency.json.gz",
             "mi300x-loop-kernels.json.gz"],
        )
        with gzip.open(REFERENCES[0], "rt") as file:
            main = json.load(file)
        self.assertEqual(main["benchmark_revision"], "7f2c5552a43b16ad95b90fed88e61f0c80c5236b")
        self.assertEqual(main["core_clock_limit_mhz"], 1680)
        self.assertEqual(main["timer"], "rocprofv3")
        v12 = main["series"]
        self.assertEqual(sum(len(row["cases"]) for row in v12), 2861)
        series = read_reference(REFERENCES)["series"]
        self.assertEqual(series[: len(v12)], v12)
        fetch = read_reference(REFERENCES[1:2])["series"]
        self.assertEqual(series[len(v12): len(v12) + len(fetch)], fetch)
        loops = series[len(v12) + len(fetch):]
        self.assertEqual(
            {row["benchmark"] for row in loops}, {"instruction_cost", "loop_wait"}
        )
        self.assertFalse(
            {"code_fetch_latency", "instruction_cost", "loop_wait"}
            & {row["benchmark"] for row in v12}
        )
        self.assertEqual(sum(len(row["cases"]) for row in loops), 3128)
        for row in loops:
            for case in row["cases"]:
                ((name, *shape, nanoseconds),) = case["launches"]
                self.assertTrue(name.startswith(("ic_", "lw_")))
                threads = row["parameters"]["threads_per_block"]
                self.assertEqual(shape, [threads, 1, 1, threads, 1, 1])
                self.assertGreater(nanoseconds, 0)
                self.assertIn(f"-iterations={case['x']}", case["args"])
                self.assertEqual(set(case["parameters"]), {"ops_per_iter"} | (
                    {"dependency"} if row["benchmark"] == "instruction_cost" else {"layout"}
                ))
        self.assertEqual([row["parameters"]["loops"] for row in fetch], [1, 4, 8, 16])
        for row in fetch:
            sizes = [2**k for k in range(4, 17)]
            self.assertEqual([case["x"] for case in row["cases"]], sizes)
            for case in row["cases"]:
                ((name, *shape, nanoseconds),) = case["launches"]
                self.assertEqual(name.split("(")[0], f"fetch_kernel_{case['x']}")
                self.assertEqual(shape, [64, 1, 1, 64, 1, 1])
                self.assertGreater(nanoseconds, 0)
                loops = row["parameters"]["loops"]
                self.assertEqual(case["args"], [f"-body-ops={case['x']}", f"-loops={loops}"])
        cases = [case["case"] for row in series for case in row["cases"]]
        self.assertEqual(len(cases), len(set(cases)))
        self.assertLessEqual(len(matrix_rows({"series": series}, "")), 256)


class RunTest(unittest.TestCase):
    def test_run_key(self):
        self.assertEqual(run_key("v4.1.0", "123"), "mgpusim-mi300x-v4.1.0")
        self.assertEqual(run_key("", "123"), "mgpusim-mi300x-preview-123")
        for tag in ("v1/2", "a b", "x" * 200):
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                run_key(tag, "123")

    def test_each_case_is_written_once_as_soon_as_it_ends(self):
        with tempfile.TemporaryDirectory() as folder:
            output = Path(folder) / "series"
            complete = {"status": "complete", "launches": [SIMULATED]}
            expired = {"status": "timeout", "reason": "Series budget expired."}
            written = []

            def put_points(key, points, deadline):
                self.assertEqual(deadline, WRITE_DEADLINE_SECONDS)
                results = json.loads((output / "results.json").read_text())["cases"]
                self.assertFalse(any("published" in r for r in list(results.values())[-1:]))
                written.append((key, points, len(results)))

            with (
                patch("workflow.time.monotonic", side_effect=[0, 0, 7, 10]),
                patch("workflow.run_case", side_effect=[complete, expired]) as run,
                patch("workflow.report_db.put_points", side_effect=put_points),
            ):
                run_series(SERIES, Path("sample"), output, "key", budget=10)
            self.assertEqual([c.args[-1] for c in run.call_args_list], [10, 3])
            results = json.loads((output / "results.json").read_text())["cases"]
            self.assertEqual(results["1"]["launches"], [SIMULATED])
            self.assertEqual(results["2"]["status"], "timeout")
            self.assertEqual(results["3"]["status"], "unsupported")
            self.assertTrue(all(result["published"] for result in results.values()))

            self.assertEqual([key for key, _, _ in written], ["key"] * 3)
            # Each write happens after its result is saved and before the next case.
            self.assertEqual([saved for _, _, saved in written], [1, 2, 3])
            for (_, points, _), case in zip(written, SERIES["cases"]):
                self.assertEqual([p["x"] for p in points], [case["x"]])
            self.assertNotIn("statuses", written[0][1][0])
            self.assertEqual(written[1][1][0]["statuses"]["mgpusim"]["state"], "timeout")
            self.assertEqual(written[2][1][0]["statuses"]["mgpusim"]["symbol"], "↷")

    def test_a_failed_write_fails_the_series(self):
        with tempfile.TemporaryDirectory() as folder:
            output = Path(folder) / "series"
            complete = {"status": "complete", "launches": [SIMULATED]}
            with (
                patch("workflow.run_case", return_value=complete) as run,
                patch("workflow.report_db.put_points", side_effect=psycopg.OperationalError()),
                self.assertRaises(psycopg.OperationalError),
            ):
                run_series(SERIES, Path("sample"), output, "key")
            self.assertEqual(run.call_count, 1)
            # The saved result is not published, so finalize counts it unsuccessful.
            results = json.loads((output / "results.json").read_text())
            self.assertEqual(results["cases"]["1"]["status"], "complete")
            progress = final_progress({0: SERIES}, {0: results}, "")
            self.assertEqual((progress["completed"], progress["unsuccessful"]), (0, 3))

    def test_the_series_clock_starts_before_the_run_is_opened(self):
        args = Namespace(series=0, binary=Path("sample"), output=Path("out"), run_key="key",
                         version="v", revision="r")
        with (
            patch("workflow.time.monotonic", return_value=1000),
            patch("workflow.report_db.open_run") as open_run,
            patch("workflow.run_series") as run,
        ):
            sweep({"series": [SERIES]}, args)
        self.assertEqual(open_run.call_args.kwargs, {"deadline": 1000 + WRITE_DEADLINE_SECONDS})
        self.assertEqual(open_run.call_args.args[2], benchmark_definitions([SERIES]))
        self.assertEqual(run.call_args.kwargs, {"start": 1000})
        self.assertLess(BUDGET_SECONDS, WRITE_DEADLINE_SECONDS)

    def test_finalize_counts_configurations_and_sets_the_state(self):
        other = {**SERIES, "cases": [{"case": 9, "x": 64, "launches": [HARDWARE]}]}
        reference = {"series": [SERIES, other]}
        with tempfile.TemporaryDirectory() as folder:
            artifact = Path(folder) / "series-0"
            artifact.mkdir()
            (artifact / "results.json").write_text(json.dumps({
                "started_at": "2026-11-24T16:00:00Z",
                "cases": {
                    "1": {"status": "complete", "launches": [SIMULATED], "published": True},
                    "2": {"status": "complete", "launches": [["other", *SHAPE, 1]],
                          "published": True},
                },
            }))
            for result, state in (("success", "completed"), ("failure", "failed")):
                with (
                    self.subTest(result=result),
                    patch("workflow.report_db.finish_run") as finish,
                ):
                    finalize(reference, Namespace(
                        artifacts=Path(folder), benchmark=None, run_key="key",
                        started_at="2026-11-24T15:00:00+00:00", sweep_result=result,
                    ))
                    ((key, written_state, progress),) = [c.args for c in finish.call_args_list]
                    self.assertEqual((key, written_state), ("key", state))
                    # Series 1 uploaded no artifact; case 3 of series 0 has no result.
                    self.assertEqual(
                        {k: v for k, v in progress.items() if k != "updated_at"},
                        {
                            "started_at": "2026-11-24T15:00:00+00:00",
                            "total": 4,
                            "running": 0,
                            "queued": 0,
                            "completed": 1,
                            "unsuccessful": 3,
                        },
                    )
        self.assertEqual(final_progress({}, {}, "")["started_at"], None)


class Connection:
    def __init__(self, outcome, log):
        self.outcome, self.log = outcome, log

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False

    def execute(self, sql, params=None):
        if sql == report_db.STATEMENT_TIMEOUT:
            return
        self.log.append(sql)
        if self.outcome:
            raise self.outcome


class ReportDbTest(unittest.TestCase):
    def setUp(self):
        patcher = patch.dict(os.environ, {"CALIBRATION_REPORT_DSN": "postgresql://test"})
        patcher.start()
        self.addCleanup(patcher.stop)

    def attempt(self, outcomes, clock=None, deadline=math.inf, sleep=None):
        log, delays = [], []
        outcomes = iter(outcomes)

        def connect(dsn, **options):
            self.assertEqual(dsn, "postgresql://test")
            self.assertEqual(options, report_db.CONNECT_OPTIONS)
            outcome = next(outcomes)
            if isinstance(outcome, type) and outcome is psycopg.OperationalError:
                raise outcome("could not connect")
            return Connection(outcome, log)

        error = None
        try:
            report_db.call(
                "SELECT cr.put_points(%s, %s::json)", "key", [], connect=connect,
                sleep=sleep or delays.append, clock=clock or (lambda: 0), deadline=deadline,
            )
        except (psycopg.Error, TimeoutError) as caught:
            error = caught
        return error, log, delays

    def test_connection_level_errors_are_retried_with_backoff(self):
        outcomes = [psycopg.OperationalError, psycopg.errors.ConnectionFailure("broken"),
                    psycopg.errors.AdminShutdown("shutdown")] * 3 + [None]
        error, log, delays = self.attempt(outcomes)
        self.assertIsNone(error)
        self.assertEqual(len(log), 7)
        self.assertEqual(delays, [1, 2, 4, 8, 16, 32, 60, 60, 60])

    def test_validation_and_state_errors_are_final(self):
        for outcome in (
            psycopg.errors.InvalidParameterValue("points must be an array"),
            psycopg.errors.ObjectNotInPrerequisiteState("run is completed"),
            psycopg.errors.NoDataFound("run does not exist"),
        ):
            with self.subTest(outcome=type(outcome).__name__):
                error, log, delays = self.attempt([outcome, None])
                self.assertIs(error, outcome)
                self.assertEqual((len(log), delays), (1, []))

    def test_retries_stop_after_the_deadline(self):
        now = [0]

        def clock():
            return now[0]

        def connect_failures():
            while True:
                now[0] += 100
                yield psycopg.OperationalError

        error, _, delays = self.attempt(connect_failures(), clock)
        self.assertIsInstance(error, psycopg.OperationalError)
        self.assertLessEqual(len(delays) * 100, report_db.RETRY_SECONDS + 100)
        self.assertGreater(len(delays), 0)

    def test_retries_end_at_the_job_deadline(self):
        now = [0]

        def clock():
            return now[0]

        def connect_failures():
            while True:
                now[0] += 10
                yield psycopg.OperationalError

        error, _, delays = self.attempt(connect_failures(), clock, deadline=100)
        self.assertIsInstance(error, psycopg.OperationalError)
        # The seventh attempt fails at 70; a 60 s wait would pass the deadline.
        self.assertEqual(delays, [1, 2, 4, 8, 16, 32])
        self.assertEqual(now[0], 70)

    def test_an_expired_deadline_opens_no_connection(self):
        error, log, _ = self.attempt([], lambda: 100, deadline=100)
        self.assertIsInstance(error, TimeoutError)
        self.assertEqual(log, [])

    def test_no_attempt_starts_after_a_sleep_passes_the_deadline(self):
        now = [0]
        connections = []

        def overshoot(delay):
            now[0] += 50

        def outcomes():
            while True:
                connections.append(now[0])
                yield psycopg.OperationalError

        error, _, _ = self.attempt(outcomes(), lambda: now[0], deadline=10, sleep=overshoot)
        self.assertIsInstance(error, TimeoutError)
        self.assertEqual(connections, [0])

    def test_no_attempt_starts_after_a_sleep_passes_the_retry_window(self):
        now = [0]
        connections = []
        failure = psycopg.errors.ConnectionFailure("broken")

        def overshoot(delay):
            now[0] = report_db.RETRY_SECONDS + 1

        def outcomes():
            while True:
                connections.append(now[0])
                yield failure

        error, _, _ = self.attempt(outcomes(), lambda: now[0], deadline=10 ** 9, sleep=overshoot)
        self.assertIs(error, failure)
        self.assertEqual(connections, [0])

    def test_statement_timeouts_are_retried(self):
        error, log, delays = self.attempt([psycopg.errors.QueryCanceled("timeout"), None])
        self.assertIsNone(error)
        self.assertEqual((len(log), delays), (2, [1]))

    def test_missing_dsn_fails_clearly(self):
        with patch.dict(os.environ, {"CALIBRATION_REPORT_DSN": " "}):
            with self.assertRaisesRegex(RuntimeError, "CALIBRATION_REPORT_DSN"):
                report_db.open_run("key", {}, {}, {})

    def test_calls_use_the_ingest_functions(self):
        with patch("report_db.call") as sent:
            report_db.put_points("key", [{"x": 1}], deadline=5)
            report_db.finish_run("key", "completed", {"total": 1})
        self.assertEqual(sent.call_args_list, [
            call("SELECT cr.put_points(%s, %s::json)", "key", [{"x": 1}], deadline=5),
            call("SELECT cr.finish_run(%s, %s, %s::json)", "key", "completed", {"total": 1}),
        ])


if __name__ == "__main__":
    unittest.main()
