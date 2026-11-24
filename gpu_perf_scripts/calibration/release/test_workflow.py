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


if __name__ == "__main__":
    unittest.main()
