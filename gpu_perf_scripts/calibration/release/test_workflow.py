import json
import tempfile
import unittest
from collections import Counter
from pathlib import Path
from unittest.mock import patch

from workflow import report, run_series

SHAPE = [64, 64, 1, 16, 16, 1]
HARDWARE = ["kernel(float*)", *SHAPE, 2000]
SIMULATED = ["kernel", *SHAPE, 1500]
SERIES = {
    "benchmark": "example",
    "axis": "size",
    "parameters": {"block_size": 16},
    "cases": [{"case": i, "x": i * 64, "launches": [HARDWARE]} for i in range(1, 4)],
}


class ReleaseCalibrationTest(unittest.TestCase):
    def test_budget_applies_to_the_series_and_preserves_completed_points(self):
        with tempfile.TemporaryDirectory() as folder:
            output = Path(folder) / "series"
            complete = {"status": "complete", "launches": [SIMULATED]}
            expired = {"status": "timeout", "reason": "Series budget expired."}
            with (
                patch("workflow.time.monotonic", side_effect=[0, 0, 7, 10]),
                patch("workflow.run_case", side_effect=[complete, expired]) as run,
            ):
                run_series(SERIES, Path("sample"), output, budget=10)
            self.assertEqual([call.args[-1] for call in run.call_args_list], [10, 3])
            results = json.loads((output / "results.json").read_text())["cases"]
            self.assertEqual(results["1"]["launches"], [SIMULATED])
            self.assertEqual(results["2"]["status"], "timeout")
            self.assertEqual(results["3"]["status"], "unsupported")

    def test_report_matches_launches_and_keeps_failed_verification_timing(self):
        results = {
            0: {
                "started_at": "2026-11-24T16:00:00Z",
                "cases": {
                    "1": {"status": "complete", "launches": [SIMULATED]},
                    "2": {
                        "status": "verification_failed",
                        "reason": "Mismatch",
                        "launches": [SIMULATED],
                    },
                    "3": {
                        "status": "complete",
                        "launches": [["other_kernel", *SHAPE, 1500]],
                    },
                },
            }
        }
        data = report({"series": [SERIES]}, results, "test", "sample-sha")
        verified, verification_failed, unmatched = data["data"]
        self.assertEqual(verified["values"]["mgpusim"], 1500)
        self.assertEqual(verification_failed["values"]["mgpusim"], 1500)
        self.assertEqual(
            verification_failed["statuses"]["mgpusim"]["state"], "verification_failed"
        )
        self.assertIsNone(unmatched["values"]["mgpusim"])
        self.assertEqual(unmatched["statuses"]["mgpusim"]["state"], "unsupported")
        self.assertEqual(data["progress"]["completed"], 1)

    def test_cfd_report_labels_launch_groups_without_merging_dispatches(self):
        names = [
            "copy_variables_kernel",
            "cfd_device::compute_step_factor_kernel",
            "cfd_device::compute_flux_kernel",
            "cfd_device::rk_update_kernel",
            "cfd_device::compute_flux_kernel",
            "cfd_device::rk_update_kernel",
            "cfd_device::compute_flux_kernel",
            "cfd_device::rk_update_kernel",
        ] * 10
        hardware = [[f"{name}(int)", *SHAPE, 2000] for name in names]
        simulated = [
            [name.removeprefix("cfd_device::"), *SHAPE, 1500 + i]
            for i, name in enumerate(names)
        ]
        cases = [{"case": 1, "x": 64, "launches": hardware}]
        series = {**SERIES, "benchmark": "altis_cfd", "cases": cases}
        results = {
            0: {
                "started_at": "2026-11-24T16:00:00Z",
                "cases": {"1": {"status": "complete", "launches": simulated}},
            }
        }

        data = report({"series": [series]}, results, "test", "sample-sha")
        points = data["data"]
        self.assertEqual(len(points), 80)
        self.assertEqual(
            len({point["non_scaling_params"]["launch"] for point in points}), 80
        )
        self.assertEqual(
            [point["values"]["mgpusim"] for point in points], list(range(1500, 1580))
        )
        self.assertEqual(
            Counter(point["non_scaling_params"]["launch_group"] for point in points),
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

        for changed_rows, index, changed_value in (
            (hardware, 0, "different_kernel"),
            (hardware, 1, 128),
            (simulated, 0, "different_kernel"),
        ):
            side = "hardware" if changed_rows is hardware else "simulated"
            with self.subTest(side=side, index=index):
                previous = changed_rows[0][index]
                changed_rows[0][index] = changed_value
                try:
                    ungrouped = report({"series": [series]}, results, "test", "sample-sha")
                finally:
                    changed_rows[0][index] = previous
                self.assertEqual(len(ungrouped["data"]), 80)
                self.assertNotIn(
                    "launch_group", ungrouped["benchmarks"]["altis_cfd"]["params"]
                )
                self.assertTrue(all(
                    "launch_group" not in point["non_scaling_params"]
                    for point in ungrouped["data"]
                ))

    def test_report_omits_only_the_cache_latency_primer(self):
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
                results = {
                    0: {
                        "started_at": "2026-11-24T16:00:00Z",
                        "cases": {"1": {"status": "complete", "launches": launches}},
                    }
                }
                data = report({"series": [series]}, results, "test", "sample-sha")
                (point,) = data["data"]
                self.assertEqual(point["values"]["mgpusim"], expected)
                if expected is None:
                    self.assertEqual(point["statuses"]["mgpusim"]["state"], "unsupported")
                else:
                    self.assertNotIn("statuses", point)
                    self.assertEqual(data["progress"]["completed"], 1)


if __name__ == "__main__":
    unittest.main()
