from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPT = REPO_ROOT / "scripts" / "score_curated_validation.py"


class ScoreCuratedValidationTest(unittest.TestCase):
    def run_scorecard(self, summary_path: Path, *extra_args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [
                sys.executable,
                str(SCRIPT),
                "--summary",
                str(summary_path),
                *extra_args,
            ],
            text=True,
            capture_output=True,
            check=False,
            timeout=30,
        )

    def write_summary(
        self,
        path: Path,
        *,
        tier1_avg: float = 0.05,
        tier2_avg: float = 0.15,
        mean_scaling: float = 0.10,
        point_status: str = "pass",
        selected_passing: int | None = None,
        kernels: list[dict] | None = None,
    ) -> None:
        if kernels is None:
            kernels = [
                self.kernel("alpha_probe", tier=1, avg_error=0.05, scaling_error=0.10),
                self.kernel("beta_kernel", tier=2, avg_error=0.15, scaling_error=0.18),
            ]
        selected_total = len(kernels)
        if selected_passing is None:
            selected_passing = selected_total if point_status == "pass" else max(0, selected_total - 1)
        non_compliant = selected_total - selected_passing
        matched_points = sum(kernel["coverage"]["matched_points"] for kernel in kernels)
        summary = {
            "schema_name": "mgpusim.validation_report_summary",
            "schema_version": 1,
            "scope": "curated",
            "set_id": "unit-test-curated-primary",
            "set_version": "test-scorecard",
            "selected_count": selected_total,
            "tier_counts": {
                str(tier): sum(1 for kernel in kernels if kernel["tier"] == tier)
                for tier in (1, 2)
                if any(kernel["tier"] == tier for kernel in kernels)
            },
            "aggregate_metrics": {
                "tier1_mean_avg_error_boosted": tier1_avg,
                "tier2_mean_avg_error_boosted": tier2_avg,
                "mean_scaling_factor_error_boosted": mean_scaling,
            },
            "coverage_status": {
                "compliant": selected_passing,
                "non_compliant": non_compliant,
                "no_sim_data": 0,
            },
            "point_coverage_status": {
                "status": point_status,
                "selected_total": selected_total,
                "selected_with_passing_coverage": selected_passing,
                "totals": {
                    "hardware_points": matched_points,
                    "matched_points": matched_points,
                    "hardware_flat_points": selected_total,
                    "matched_flat_points": selected_total,
                    "hardware_non_flat_points": max(0, matched_points - selected_total),
                    "matched_non_flat_points": max(0, matched_points - selected_total),
                    "trend_points": matched_points,
                },
            },
            "kernels": kernels,
        }
        path.write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n")

    def kernel(
        self,
        name: str,
        *,
        tier: int,
        avg_error: float,
        scaling_error: float | None,
        coverage_compliant: bool = True,
        matched_points: int = 6,
    ) -> dict:
        return {
            "kernel_name": name,
            "tier": tier,
            "category": f"tier{tier}_fixture",
            "coverage": {
                "compliant": coverage_compliant,
                "matched_points": matched_points,
                "matched_flat_points": 1,
                "matched_non_flat_points": max(0, matched_points - 1),
                "hardware_flat_points": 1,
                "hardware_non_flat_points": max(0, matched_points - 1),
                "reason": "OK" if coverage_compliant else "fixture failure",
            },
            "accuracy": {
                "avg_error_boosted": avg_error,
                "scaling_factor_error_boosted": scaling_error,
            },
            "trend": {
                "trend_ratio": None if scaling_error is None else 1.0 + scaling_error,
            },
        }

    def test_pass_fixture_enforce_returns_zero_and_reports_pass(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            summary_path = Path(tmpdir) / "summary.json"
            self.write_summary(summary_path)

            result = self.run_scorecard(summary_path, "--enforce", "--format", "json")

            self.assertEqual(result.returncode, 0, msg=result.stderr)
            scorecard = json.loads(result.stdout)
            self.assertEqual(scorecard["schema_name"], "mgpusim.curated_accuracy_scorecard")
            self.assertEqual(scorecard["schema_version"], 1)
            self.assertEqual(scorecard["coverage_status"], "pass")
            self.assertEqual(scorecard["accuracy_status"], "pass")
            self.assertEqual(scorecard["trend_status"], "pass")
            self.assertEqual(scorecard["overall_status"], "pass")
            self.assertEqual(
                scorecard["aggregate_target_results"]["tier1_avg_error_boosted"]["gap"],
                0.0,
            )

    def test_current_like_failure_is_diagnostic_exit_zero(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            summary_path = Path(tmpdir) / "summary.json"
            self.write_summary(
                summary_path,
                tier1_avg=0.813676029097,
                tier2_avg=0.862090841679,
                mean_scaling=2.785559910429,
                kernels=[
                    self.kernel("slow_probe", tier=1, avg_error=0.70, scaling_error=0.30),
                    self.kernel("trend_bad_kernel", tier=2, avg_error=0.50, scaling_error=2.00),
                ],
            )

            result = self.run_scorecard(summary_path, "--top", "1")

            self.assertEqual(result.returncode, 0, msg=result.stderr)
            self.assertIn("Coverage: PASS", result.stdout)
            self.assertIn("Accuracy: FAIL", result.stdout)
            self.assertIn("Trend/scaling: FAIL", result.stdout)
            self.assertIn("Diagnostic mode: exiting 0", result.stdout)
            self.assertIn("FAIL Tier 1 mean boosted average error: 81.4%", result.stdout)

    def test_unscored_diagnostic_kernel_metrics_are_rankable(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            summary_path = Path(tmpdir) / "summary.json"
            diagnostic_kernel = self.kernel(
                "diagnostic_cache_bw",
                tier=1,
                avg_error=0.05,
                scaling_error=0.10,
            )
            diagnostic_kernel["accuracy"] = {
                "avg_error_boosted": None,
                "avg_error_boosted_diagnostic": 0.25,
                "scaling_factor_error_boosted": None,
                "scaling_factor_error_boosted_diagnostic": 0.40,
                "boosted_metrics_scored": False,
            }
            self.write_summary(
                summary_path,
                tier1_avg=0.05,
                tier2_avg=0.15,
                mean_scaling=0.10,
                kernels=[
                    diagnostic_kernel,
                    self.kernel("scored_kernel", tier=2, avg_error=0.15, scaling_error=0.18),
                ],
            )

            result = self.run_scorecard(summary_path, "--format", "json")

            self.assertEqual(result.returncode, 0, msg=result.stderr)
            scorecard = json.loads(result.stdout)
            ranked = scorecard["ranked_benchmark_gaps"]
            self.assertEqual(ranked[0]["kernel_name"], "diagnostic_cache_bw")
            self.assertEqual(ranked[0]["avg_error_source"], "avg_error_boosted_diagnostic")
            self.assertEqual(
                ranked[0]["scaling_factor_error_source"],
                "scaling_factor_error_boosted_diagnostic",
            )

    def test_enforce_returns_nonzero_for_target_failures(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            summary_path = Path(tmpdir) / "summary.json"
            self.write_summary(
                summary_path,
                tier1_avg=0.25,
                tier2_avg=0.15,
                mean_scaling=0.10,
                kernels=[self.kernel("bad_tier1", tier=1, avg_error=0.25, scaling_error=0.10)],
            )

            result = self.run_scorecard(summary_path, "--enforce")

            self.assertEqual(result.returncode, 2, msg=result.stderr)
            self.assertIn("Enforcement mode: failed targets", result.stdout)
            self.assertIn("FAIL Tier 1 mean boosted average error", result.stdout)

    def test_missing_invalid_and_malformed_summary_handling(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)

            missing = self.run_scorecard(tmp_path / "missing.json")
            self.assertEqual(missing.returncode, 1)
            self.assertIn("summary JSON not found", missing.stderr)

            invalid_path = tmp_path / "invalid.json"
            invalid_path.write_text("{not json\n")
            invalid = self.run_scorecard(invalid_path)
            self.assertEqual(invalid.returncode, 1)
            self.assertIn("invalid summary JSON", invalid.stderr)

            malformed_path = tmp_path / "malformed.json"
            self.write_summary(malformed_path)
            malformed = json.loads(malformed_path.read_text())
            malformed["schema_name"] = "wrong.schema"
            malformed_path.write_text(json.dumps(malformed) + "\n")
            malformed_result = self.run_scorecard(malformed_path)
            self.assertEqual(malformed_result.returncode, 1)
            self.assertIn("schema_name", malformed_result.stderr)

    def test_json_output_and_ranking_are_deterministic(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            summary_path = tmp_path / "summary.json"
            output_path = tmp_path / "scorecard.json"
            self.write_summary(
                summary_path,
                tier1_avg=0.50,
                tier2_avg=0.40,
                mean_scaling=0.80,
                kernels=[
                    self.kernel("zeta", tier=2, avg_error=0.30, scaling_error=0.25),
                    self.kernel("alpha", tier=1, avg_error=0.09, scaling_error=1.00),
                    self.kernel("gamma", tier=2, avg_error=0.50, scaling_error=0.70),
                    self.kernel("beta", tier=2, avg_error=0.50, scaling_error=0.70),
                ],
            )

            first = self.run_scorecard(
                summary_path,
                "--format",
                "json",
                "--json-output",
                str(output_path),
            )
            second = self.run_scorecard(summary_path, "--format", "json")

            self.assertEqual(first.returncode, 0, msg=first.stderr)
            self.assertEqual(second.returncode, 0, msg=second.stderr)
            self.assertEqual(first.stdout, second.stdout)
            self.assertEqual(output_path.read_text(), first.stdout)
            scorecard = json.loads(first.stdout)
            self.assertEqual(
                [entry["kernel_name"] for entry in scorecard["ranked_benchmark_gaps"]],
                ["alpha", "beta", "gamma", "zeta"],
            )
            self.assertEqual(scorecard["ranked_benchmark_gaps"][0]["worst_gap"], 0.8)
            self.assertEqual(scorecard["ranked_benchmark_gaps"][1]["worst_gap"], 0.5)
            self.assertEqual(
                scorecard["aggregate_target_results"]["mean_scaling_factor_error_boosted"]["gap"],
                0.6,
            )


if __name__ == "__main__":
    unittest.main()
