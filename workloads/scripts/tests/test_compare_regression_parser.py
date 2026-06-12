import csv
import importlib.util
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


SCRIPT_PATH = Path(__file__).resolve().parents[1] / "compare_regression.py"
FIXTURE_DIR = Path(__file__).resolve().parents[1] / "fixtures" / "compare_regression"


spec = importlib.util.spec_from_file_location("compare_regression", SCRIPT_PATH)
assert spec and spec.loader
compare_regression = importlib.util.module_from_spec(spec)
spec.loader.exec_module(compare_regression)


class CompareRegressionParserTest(unittest.TestCase):
    def test_supported_timing_columns_skip_blank_and_non_numeric(self):
        for column_name, scale in compare_regression.TIMING_COLUMN_SCALES:
            with self.subTest(column_name=column_name):
                with tempfile.TemporaryDirectory() as tmpdir:
                    csv_path = Path(tmpdir) / f"{column_name}.csv"
                    csv_path.write_text(
                        "kernel_name,problem_size,{}\n"
                        "kernel_a,256,1.5\n"
                        "kernel_b,512,\n"
                        "kernel_c,1024,not_numeric\n".format(column_name),
                        encoding="utf-8",
                    )

                    data, skipped_rows = compare_regression.read_csv_data(
                        str(csv_path), source_name=column_name
                    )

                    self.assertEqual(data, {("kernel_a", "256"): 1.5 * scale})
                    self.assertEqual(len(skipped_rows), 2)

                    reasons = {row["reason"] for row in skipped_rows}
                    self.assertIn(f"blank {column_name}", reasons)
                    self.assertIn(f"non-numeric {column_name}", reasons)

    def test_cli_logs_diagnostics_and_writes_non_empty_output_csv(self):
        ref_csv = FIXTURE_DIR / "ref_vectoradd.csv.fixture"
        sim_csv = FIXTURE_DIR / "sim_vectoradd_sparse.csv.fixture"

        with tempfile.TemporaryDirectory() as tmpdir:
            output_csv = Path(tmpdir) / "regression_out.csv"
            cmd = [
                sys.executable,
                str(SCRIPT_PATH),
                "--ref",
                str(ref_csv),
                "--sim",
                str(sim_csv),
                "--output",
                str(output_csv),
            ]

            completed = subprocess.run(
                cmd,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                check=False,
            )

            self.assertEqual(
                completed.returncode,
                0,
                msg=f"stdout:\n{completed.stdout}\nstderr:\n{completed.stderr}",
            )
            self.assertTrue(output_csv.exists())

            with output_csv.open("r", encoding="utf-8", newline="") as handle:
                rows = list(csv.DictReader(handle))
            self.assertGreater(len(rows), 0)

            stderr = completed.stderr
            self.assertIn("[compare_regression] Skipped rows: 2", stderr)
            self.assertIn("kernel_name=vectoradd", stderr)
            self.assertIn("reason=blank avg_ms", stderr)
            self.assertIn("reason=non-numeric avg_ms", stderr)
            self.assertIn("raw_value=<blank>", stderr)


if __name__ == "__main__":
    unittest.main()
