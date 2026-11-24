from contextlib import closing
import sqlite3
import subprocess
import tempfile
import unittest
from unittest.mock import patch

from export_live_report import convert
from run_sweep import ordered_configs, run_one, sweep_points


class SweepTests(unittest.TestCase):
    def test_only_larger_points_in_timed_out_series_are_skipped(self):
        configs = [(1, "size", str(x), {"block": block})
                   for x, block in [(32, 8), (8, 8), (16, 16), (16, 8), (32, 16)]]
        configs = list(ordered_configs(configs))
        launched = []

        def run(_, command):
            launched.append(command)
            if command == ("8", 8):
                return {"status": "timeout"}
            if command == ("16", 16):
                return {"status": "failed"}
            return {"status": "complete", "kernel_time_s": 1e-6}

        results = {(configs[i][2], configs[i][3]["block"]): row["status"]
                    for i, row in sweep_points(configs, lambda c: (c[2], c[3]["block"]), run)}
        self.assertEqual(launched, [("8", 8), ("16", 16), ("32", 16)])
        self.assertEqual(results, {("8", 8): "timeout", ("16", 8): "skipped_after_timeout",
                                   ("32", 8): "skipped_after_timeout", ("16", 16): "failed",
                                   ("32", 16): "complete"})

    def test_verification_failures_retain_only_available_timing(self):
        for has_timing in (False, True):
            with self.subTest(has_timing=has_timing), tempfile.TemporaryDirectory() as directory:
                def simulate(command, cwd, stdout, **_):
                    stdout.write("panic: Benchmark[0]: Mismatch at 10, expected 3, but got 4\n")
                    if has_timing:
                        with closing(sqlite3.connect(cwd / "t.sqlite3")) as db, db:
                            db.execute("CREATE TABLE mgpusim_metrics (What, Location, Value)")
                            db.execute("INSERT INTO mgpusim_metrics VALUES ('kernel_time', 'Driver', 0.003)")
                    return subprocess.CompletedProcess(command, 2)

                with patch("run_sweep.subprocess.run", side_effect=simulate):
                    result = run_one("/binary", [], directory, 1)
                row = {"benchmark": "b", "axis": "size", "x": 1, "non_scaling_params": {},
                       "kind": "kernel", "hardware_ms": 2, **result}
                point = convert({"cases": [row]})["data"][0]
                self.assertEqual(point["values"]["mgpusim"], 3_000_000 if has_timing else None)
                self.assertEqual(point["statuses"]["mgpusim"]["state"], "verification_failed")

    def test_skip_exports_a_marker_with_an_explanation(self):
        row = {"benchmark": "b", "axis": "size", "x": 2, "series": "block=8",
               "kind": "kernel", "status": "skipped_after_timeout", "error": "size=1 timed out"}
        point = convert({"cases": [row]})["data"][0]
        self.assertEqual(point["non_scaling_params"], {"block": 8})
        self.assertEqual(point["statuses"]["mgpusim"]["symbol"], "↷")
        self.assertIn("size=1", point["statuses"]["mgpusim"]["reason"])


if __name__ == "__main__":
    unittest.main()
