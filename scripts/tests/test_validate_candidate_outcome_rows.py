import csv
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPT = REPO_ROOT / "scripts" / "validate_candidate_outcome_rows.py"


class ValidateCandidateOutcomeRowsTest(unittest.TestCase):
    def run_validator(self, csv_path: Path, *extra_args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [sys.executable, str(SCRIPT), "--csv", str(csv_path), *extra_args],
            text=True,
            capture_output=True,
            check=False,
        )

    def test_accepts_single_terminal_row(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            csv_path = Path(tmpdir) / "candidate_outcomes_parboil_stencil.csv"
            with csv_path.open("w", newline="") as f:
                writer = csv.writer(f)
                writer.writerow(
                    [
                        "benchmark",
                        "candidate_size",
                        "size_label",
                        "terminal_outcome",
                        "sim_result_state",
                        "exit_code",
                        "timeout_sec",
                        "detail",
                    ]
                )
                writer.writerow(
                    [
                        "parboil_stencil",
                        "88",
                        "88",
                        "timeout",
                        "no_sim_row",
                        "124",
                        "21600",
                        "timeout_21600s",
                    ]
                )

            result = self.run_validator(csv_path, "--expected-attempts", "1")
            self.assertEqual(result.returncode, 0, msg=result.stderr)
            self.assertIn("VALIDATION_OK", result.stdout)

    def test_rejects_header_only_csv(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            csv_path = Path(tmpdir) / "candidate_outcomes_covariance.csv"
            with csv_path.open("w", newline="") as f:
                writer = csv.writer(f)
                writer.writerow(
                    [
                        "benchmark",
                        "candidate_size",
                        "size_label",
                        "terminal_outcome",
                        "sim_result_state",
                        "exit_code",
                        "timeout_sec",
                        "detail",
                    ]
                )

            result = self.run_validator(csv_path, "--expected-attempts", "1")
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("header-only artifact", result.stderr)

    def test_rejects_duplicate_candidate_rows(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            csv_path = Path(tmpdir) / "candidate_outcomes_parboil_stencil.csv"
            with csv_path.open("w", newline="") as f:
                writer = csv.writer(f)
                writer.writerow(
                    [
                        "benchmark",
                        "candidate_size",
                        "size_label",
                        "terminal_outcome",
                        "sim_result_state",
                        "exit_code",
                        "timeout_sec",
                        "detail",
                    ]
                )
                writer.writerow(
                    [
                        "parboil_stencil",
                        "88",
                        "88",
                        "cancelled",
                        "no_sim_row",
                        "143",
                        "21600",
                        "preseed_cancelled_fallback",
                    ]
                )
                writer.writerow(
                    [
                        "parboil_stencil",
                        "88",
                        "88",
                        "timeout",
                        "no_sim_row",
                        "124",
                        "21600",
                        "timeout_21600s",
                    ]
                )

            result = self.run_validator(csv_path, "--expected-attempts", "2")
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("duplicate terminal rows", result.stderr)


if __name__ == "__main__":
    unittest.main()
