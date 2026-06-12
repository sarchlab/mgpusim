import csv
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPT = REPO_ROOT / "scripts" / "validate_mi300a_evaluation_set.py"


class ValidateMI300AEvaluationSetTest(unittest.TestCase):
    def run_validator(
        self, csv_path: Path, manifest_path: Path, *extra_args: str
    ) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [
                sys.executable,
                str(SCRIPT),
                "--csv",
                str(csv_path),
                "--manifest",
                str(manifest_path),
                *extra_args,
            ],
            text=True,
            capture_output=True,
            check=False,
        )

    def write_csv(self, path: Path, kernel: str = "valid_kernel", real_values=None) -> None:
        if real_values is None:
            real_values = [1.0, 3.0, 4.0, 5.0, 6.0, 7.0]
        with path.open("w", newline="") as f:
            writer = csv.writer(f)
            writer.writerow(
                ["kernel_name", "problem_size", "real_ms", "sim_ms", "abs_error_ms", "rel_error"]
            )
            for idx, real_ms in enumerate(real_values, start=1):
                sim_ms = real_ms * 1.1
                writer.writerow(
                    [
                        kernel,
                        str(idx),
                        f"{real_ms:.6f}",
                        f"{sim_ms:.6f}",
                        f"{abs(sim_ms - real_ms):.6f}",
                        f"{(sim_ms - real_ms) / real_ms:.6f}",
                    ]
                )

    def write_manifest(
        self,
        path: Path,
        kernel: str = "valid_kernel",
        hardware_points: int = 6,
        matched_points: int = 6,
        hardware_flat_points: int = 1,
        matched_flat_points: int = 1,
        hardware_non_flat_points: int = 5,
        matched_non_flat_points: int = 5,
        trend_points: int = 6,
    ) -> None:
        manifest = {
            "schema_version": 1,
            "schema_name": "mgpusim.mi300a_evaluation_set",
            "set_id": "unit-test-curated-set",
            "set_version": "test",
            "data_source": "fixture/comparison_ci.csv",
            "benchmarks": [
                {
                    "kernel_name": kernel,
                    "tier": 2,
                    "category": "unit_test_category",
                    "rationale": "Small deterministic fixture for validator unit tests.",
                    "coverage_expectation": {
                        "policy": "tier2_flat_nonflat_v1",
                        "min_flat_points_if_hw_flat_exists": 1,
                        "min_non_flat_points_if_hw_non_flat_exists": 5,
                        "min_trend_points": 3,
                    },
                    "coverage_evidence": {
                        "hardware_points": hardware_points,
                        "matched_points": matched_points,
                        "hardware_flat_points": hardware_flat_points,
                        "matched_flat_points": matched_flat_points,
                        "hardware_non_flat_points": hardware_non_flat_points,
                        "matched_non_flat_points": matched_non_flat_points,
                        "point_coverage_ready": True,
                        "trend_points": trend_points,
                    },
                }
            ],
        }
        path.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n")

    def write_partial_manifest(
        self,
        path: Path,
        *,
        kernel: str = "valid_kernel",
        hardware_points: int = 5,
        matched_points: int = 5,
        hardware_flat_points: int = 1,
        matched_flat_points: int = 1,
        hardware_non_flat_points: int = 4,
        matched_non_flat_points: int = 4,
        trend_points: int = 5,
    ) -> None:
        manifest = {
            "schema_version": 1,
            "schema_name": "mgpusim.mi300a_evaluation_set",
            "set_id": "unit-test-curated-set",
            "set_version": "partial-test",
            "data_source": "fixture/comparison_ci.csv",
            "benchmarks": [
                {
                    "kernel_name": kernel,
                    "tier": 2,
                    "category": "unit_test_category",
                    "rationale": "Partial deterministic fixture for validator unit tests.",
                    "coverage_expectation": {
                        "policy": "tier2_flat_nonflat_v1",
                        "min_flat_points_if_hw_flat_exists": 1,
                        "min_non_flat_points_if_hw_non_flat_exists": 5,
                        "min_trend_points": 3,
                        "partial_coverage_allowed": True,
                        "partial_coverage_policy": "partial_current_data_v1",
                    },
                    "coverage_evidence": {
                        "hardware_points": hardware_points,
                        "matched_points": matched_points,
                        "hardware_flat_points": hardware_flat_points,
                        "matched_flat_points": matched_flat_points,
                        "hardware_non_flat_points": hardware_non_flat_points,
                        "matched_non_flat_points": matched_non_flat_points,
                        "point_coverage_ready": False,
                        "coverage_status": "partial",
                        "partial_coverage_reason": "Fixture intentionally has too few non-flat points.",
                        "coverage_gaps": [
                            {
                                "field": "matched_non_flat_points",
                                "observed": matched_non_flat_points,
                                "required": 5,
                            }
                        ],
                        "trend_points": trend_points,
                    },
                }
            ],
        }
        path.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n")

    def test_accepts_valid_fixture_and_prints_json_summary(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            csv_path = tmp_path / "comparison_ci.csv"
            manifest_path = tmp_path / "mi300a_evaluation_set.json"
            self.write_csv(csv_path)
            self.write_manifest(manifest_path)

            result = self.run_validator(csv_path, manifest_path, "--summary-format", "json")

            self.assertEqual(result.returncode, 0, msg=result.stderr)
            summary = json.loads(result.stdout)
            self.assertEqual(summary["selected_count"], 1)
            self.assertEqual(summary["tier_counts"], {"2": 1})
            self.assertEqual(summary["point_coverage_status"]["status"], "pass")
            self.assertIn("avg_abs_rel_error", summary["accuracy_trend_fields"])
            self.assertIn("trend_ratio", summary["accuracy_trend_fields"])

    def test_accepts_explicit_partial_current_coverage_without_marking_ready(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            csv_path = tmp_path / "comparison_ci.csv"
            manifest_path = tmp_path / "mi300a_evaluation_set.json"
            self.write_csv(csv_path, real_values=[1.0, 3.0, 4.0, 5.0, 6.0])
            self.write_partial_manifest(manifest_path)

            result = self.run_validator(csv_path, manifest_path, "--summary-format", "json")

            self.assertEqual(result.returncode, 0, msg=result.stderr)
            summary = json.loads(result.stdout)
            point_status = summary["point_coverage_status"]
            self.assertEqual(point_status["status"], "partial")
            self.assertEqual(point_status["selected_with_passing_coverage"], 0)
            self.assertEqual(point_status["selected_with_partial_coverage"], 1)
            self.assertEqual(point_status["partial_kernels"], ["valid_kernel"])
            self.assertEqual(summary["kernels"][0]["coverage_status"], "partial")
            self.assertFalse(summary["kernels"][0]["point_coverage_ready"])
            self.assertEqual(
                summary["kernels"][0]["coverage_gaps"],
                [{"field": "matched_non_flat_points", "observed": 4, "required": 5}],
            )

    def test_rejects_partial_coverage_with_stale_gap_accounting(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            csv_path = tmp_path / "comparison_ci.csv"
            manifest_path = tmp_path / "mi300a_evaluation_set.json"
            self.write_csv(csv_path, real_values=[1.0, 3.0, 4.0, 5.0, 6.0])
            self.write_partial_manifest(manifest_path, matched_non_flat_points=3)

            result = self.run_validator(csv_path, manifest_path)

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("partial coverage evidence", result.stderr)
            self.assertIn("coverage_gaps", result.stderr)

    def test_rejects_malformed_manifest_entry(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            csv_path = tmp_path / "comparison_ci.csv"
            manifest_path = tmp_path / "mi300a_evaluation_set.json"
            self.write_csv(csv_path)
            self.write_manifest(manifest_path)
            manifest = json.loads(manifest_path.read_text())
            del manifest["benchmarks"][0]["coverage_evidence"]
            manifest_path.write_text(json.dumps(manifest) + "\n")

            result = self.run_validator(csv_path, manifest_path)

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("malformed manifest", result.stderr)
            self.assertIn("coverage_evidence", result.stderr)

    def test_rejects_unknown_selected_kernel(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            csv_path = tmp_path / "comparison_ci.csv"
            manifest_path = tmp_path / "mi300a_evaluation_set.json"
            self.write_csv(csv_path, kernel="known_kernel")
            self.write_manifest(manifest_path, kernel="missing_kernel")

            result = self.run_validator(csv_path, manifest_path)

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("unknown selected kernel: missing_kernel", result.stderr)

    def test_rejects_coverage_failure(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            csv_path = tmp_path / "comparison_ci.csv"
            manifest_path = tmp_path / "mi300a_evaluation_set.json"
            self.write_csv(csv_path, real_values=[1.0, 3.0, 4.0, 5.0, 6.0])
            self.write_manifest(
                manifest_path,
                hardware_points=5,
                matched_points=5,
                hardware_flat_points=1,
                matched_flat_points=1,
                hardware_non_flat_points=4,
                matched_non_flat_points=4,
                trend_points=5,
            )

            result = self.run_validator(csv_path, manifest_path)

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("coverage failure for valid_kernel", result.stderr)
            self.assertIn("matched_non_flat_points 4/5", result.stderr)


if __name__ == "__main__":
    unittest.main()
