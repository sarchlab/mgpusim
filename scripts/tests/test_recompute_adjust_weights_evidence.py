"""Tests for adjust_weights selected-run evidence recomputation."""

from __future__ import annotations

import io
import json
import sys
import tempfile
import unittest
from contextlib import redirect_stdout
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPTS_DIR = REPO_ROOT / "scripts"
if str(SCRIPTS_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPTS_DIR))

import recompute_adjust_weights_evidence as evidence  # noqa: E402


def _scratch_root() -> Path:
    scratch = REPO_ROOT / ".repo_tmp"
    scratch.mkdir(exist_ok=True)
    return scratch


class RecomputeAdjustWeightsEvidenceTest(unittest.TestCase):
    def test_fixture_metrics_and_no_result_boundary(self) -> None:
        with tempfile.TemporaryDirectory(dir=_scratch_root()) as tmp:
            tmp_path = Path(tmp)
            csv_path = tmp_path / "comparison.csv"
            tiers_path = tmp_path / "tiers.json"
            csv_path.write_text(
                "\n".join(
                    [
                        "kernel_name,problem_size,real_ms,sim_ms,abs_error_ms,rel_error",
                        "adjust_weights,1,10,8,,",
                        "adjust_weights,2,20,24,,",
                        "adjust_weights,4,40,,,,",
                        "other,1,1,1,,",
                        "",
                    ]
                ),
                encoding="utf-8",
            )
            tiers_path.write_text(
                json.dumps(
                    {
                        "per_benchmark_time_models": {
                            "adjust_weights": {
                                "sim_scale": 0.5,
                                "fixed_time_ms": 1.0,
                                "source": "unit-test affine source",
                            }
                        }
                    }
                ),
                encoding="utf-8",
            )

            result = evidence.build_evidence(csv_path, tiers_path)

        counts = result["sample_counts"]
        self.assertEqual(counts["total_rows"], 3)
        self.assertEqual(counts["completed_samples"], 2)
        self.assertEqual(counts["no_result_samples"], 1)
        self.assertEqual(counts["largest_completed_problem_size"], "2")
        self.assertEqual(counts["first_no_result_problem_size"], "4")
        self.assertTrue(counts["contiguous_no_result_tail"])

        self.assertAlmostEqual(
            result["models"]["raw"]["mean_abs_rel_error"],
            0.2,
        )
        self.assertAlmostEqual(
            result["models"]["fixed_time_only"]["mean_abs_rel_error"],
            0.175,
        )
        self.assertAlmostEqual(
            result["models"]["current_affine"]["mean_abs_rel_error"],
            0.425,
        )
        self.assertAlmostEqual(
            result["models"]["current_affine"]["trend_statistics"][
                "real_ms_vs_adjusted_sim_ms"
            ]["spearman"],
            1.0,
        )
        feasibility = result["feasibility"]
        self.assertEqual(
            feasibility["decision"],
            "raw_or_fixed_time_only_model_meets_target",
        )
        self.assertAlmostEqual(
            feasibility["best_fixed_time_only"]["fixed_time_ms"],
            2.0,
        )
        self.assertAlmostEqual(
            feasibility["best_fixed_time_only"]["mean_abs_rel_error"],
            0.15,
        )

    def test_malformed_csv_reports_missing_required_columns(self) -> None:
        with tempfile.TemporaryDirectory(dir=_scratch_root()) as tmp:
            tmp_path = Path(tmp)
            csv_path = tmp_path / "comparison.csv"
            tiers_path = tmp_path / "tiers.json"
            csv_path.write_text(
                "kernel_name,problem_size,real_ms\nadjust_weights,1,1\n",
                encoding="utf-8",
            )
            tiers_path.write_text(
                json.dumps(
                    {
                        "per_benchmark_time_models": {
                            "adjust_weights": {
                                "sim_scale": 1.0,
                                "fixed_time_ms": 0.0,
                            }
                        }
                    }
                ),
                encoding="utf-8",
            )

            with self.assertRaisesRegex(
                evidence.EvidenceError,
                "missing required columns: sim_ms",
            ):
                evidence.build_evidence(csv_path, tiers_path)

    def test_cli_writes_json_and_markdown_outputs(self) -> None:
        with tempfile.TemporaryDirectory(dir=_scratch_root()) as tmp:
            tmp_path = Path(tmp)
            csv_path = tmp_path / "comparison.csv"
            tiers_path = tmp_path / "tiers.json"
            json_path = tmp_path / "evidence.json"
            md_path = tmp_path / "evidence.md"
            csv_path.write_text(
                "\n".join(
                    [
                        "kernel_name,problem_size,real_ms,sim_ms,abs_error_ms,rel_error",
                        "adjust_weights,1,10,8,,",
                        "adjust_weights,2,20,24,,",
                        "adjust_weights,4,40,,,,",
                        "",
                    ]
                ),
                encoding="utf-8",
            )
            tiers_path.write_text(
                json.dumps(
                    {
                        "per_benchmark_time_models": {
                            "adjust_weights": {
                                "sim_scale": 0.5,
                                "fixed_time_ms": 1.0,
                                "source": "unit-test affine source",
                            }
                        }
                    }
                ),
                encoding="utf-8",
            )

            stdout = io.StringIO()
            with redirect_stdout(stdout):
                rc = evidence.main(
                    [
                        "--csv",
                        str(csv_path),
                        "--tiers",
                        str(tiers_path),
                        "--output-json",
                        str(json_path),
                        "--output-md",
                        str(md_path),
                        "--format",
                        "json",
                    ]
                )

            self.assertEqual(rc, 0)
            self.assertEqual(
                json.loads(stdout.getvalue())["kernel_name"],
                "adjust_weights",
            )
            data = json.loads(json_path.read_text(encoding="utf-8"))
            self.assertEqual(data["kernel_name"], "adjust_weights")
            markdown = md_path.read_text(encoding="utf-8")
            self.assertIn("Mean error", markdown)
            self.assertIn("Feasibility decision", markdown)
            self.assertEqual(
                json.loads(json_path.read_text(encoding="utf-8"))["models"][
                    "current_affine"
                ]["fixed_time_ms"],
                1.0,
            )

    def test_selected_run_feasibility_decision_records_lower_bound(self) -> None:
        result = evidence.build_evidence(
            evidence.DEFAULT_COMPARISON_CSV,
            evidence.DEFAULT_TIER_CONFIG,
            require_tracked_inputs=True,
        )
        feasibility = result["feasibility"]
        best_fixed = feasibility["best_fixed_time_only"]
        shape = feasibility["raw_signed_error_shape"]

        self.assertEqual(
            feasibility["decision"],
            "not_feasible_with_raw_or_fixed_time_only_model",
        )
        self.assertFalse(feasibility["raw_or_configured_fixed_time_below_target"])
        self.assertFalse(feasibility["best_fixed_time_only_below_target"])
        self.assertTrue(feasibility["positive_real_to_sim_trend_preserved"])
        self.assertAlmostEqual(best_fixed["fixed_time_ms"], 0.00406)
        self.assertAlmostEqual(
            best_fixed["mean_abs_rel_error_pct"],
            25.60262507011604,
        )
        self.assertEqual(shape["underpredicted_count"], 8)
        self.assertEqual(shape["overpredicted_count"], 4)
        self.assertEqual(shape["largest_underpredicted_problem_size"], "32768")
        self.assertEqual(shape["first_overpredicted_problem_size"], "65536")

    def test_committed_artifact_matches_tracked_sources(self) -> None:
        artifact = (
            REPO_ROOT
            / "benchmark-comparison"
            / "selected-run-25710089668"
            / "adjust_weights_recomputed_evidence.json"
        )
        expected = evidence.build_evidence(
            evidence.DEFAULT_COMPARISON_CSV,
            evidence.DEFAULT_TIER_CONFIG,
            require_tracked_inputs=True,
        )
        actual = json.loads(artifact.read_text(encoding="utf-8"))
        self.assertEqual(actual, expected)


if __name__ == "__main__":
    unittest.main()
