import json
import tempfile
import unittest
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

            data = report(
                {"series": [SERIES]},
                {0: {"started_at": "2026-11-24T16:00:00Z", "cases": results}},
                "test",
                "sample-sha",
            )
            _, timed_out, skipped = data["data"]
            self.assertNotIn("symbol", timed_out["statuses"]["mgpusim"])
            self.assertEqual(skipped["statuses"]["mgpusim"]["symbol"], "↷")

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
        self.assertNotIn("symbol", unmatched["statuses"]["mgpusim"])
        self.assertEqual(data["progress"]["completed"], 1)

    def test_report_matches_the_lbm_exported_kernel_only_with_matching_geometry(self):
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
                results = {
                    0: {
                        "started_at": "2026-11-24T16:00:00Z",
                        "cases": {"1": {"status": "complete", "launches": [launch]}},
                    }
                }
                data = report({"series": [series]}, results, "test", "sample-sha")
                (point,) = data["data"]
                self.assertEqual(point["values"]["mgpusim"], expected)
                self.assertEqual(data["progress"]["completed"], int(expected is not None))

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
