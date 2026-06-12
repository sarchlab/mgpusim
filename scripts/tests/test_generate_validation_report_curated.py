import csv
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPT = REPO_ROOT / "scripts" / "generate_validation_report.py"
SELECTED_RUN_COMPARISON_REFERENCE = (
    "benchmark-comparison/selected-run-25619929396/comparison_ci.detailed.csv"
)
TOP_LEVEL_COMPARISON_REFERENCE = "benchmark-comparison/comparison_ci.csv"
sys.path.insert(0, str(REPO_ROOT / "scripts"))
import generate_validation_report as gvr  # noqa: E402


class GenerateValidationReportCuratedTest(unittest.TestCase):
    def run_report(
        self,
        csv_path: Path,
        manifest_path: Path,
        output_path: Path,
        summary_path: Path,
        figures_dir: Path,
        env=None,
    ) -> subprocess.CompletedProcess[str]:
        return self.run_command(
            [
                sys.executable,
                str(SCRIPT),
                "--scope",
                "curated",
                "--csv",
                str(csv_path),
                "--manifest",
                str(manifest_path),
                "--output",
                str(output_path),
                "--summary-json",
                str(summary_path),
                "--figures-dir",
                str(figures_dir),
            ],
            output_path,
            env=env,
        )

    def run_default_report(
        self,
        csv_path: Path,
        output_path: Path,
        summary_path: Path,
        figures_dir: Path,
        env=None,
    ) -> subprocess.CompletedProcess[str]:
        return self.run_command(
            [
                sys.executable,
                str(SCRIPT),
                "--csv",
                str(csv_path),
                "--output",
                str(output_path),
                "--summary-json",
                str(summary_path),
                "--figures-dir",
                str(figures_dir),
            ],
            output_path,
            env=env,
        )

    def run_default_report_without_csv(
        self,
        output_path: Path,
        summary_path: Path,
        figures_dir: Path,
        env=None,
    ) -> subprocess.CompletedProcess[str]:
        return self.run_command(
            [
                sys.executable,
                str(SCRIPT),
                "--output",
                str(output_path),
                "--summary-json",
                str(summary_path),
                "--figures-dir",
                str(figures_dir),
            ],
            output_path,
            env=env,
        )

    def run_command(
        self,
        command: list[str],
        output_path: Path,
        env=None,
    ) -> subprocess.CompletedProcess[str]:
        run_env = os.environ.copy() if env is None else env.copy()
        run_env.setdefault("MPLCONFIGDIR", str(output_path.parent / ".matplotlib"))
        return subprocess.run(
            command,
            text=True,
            capture_output=True,
            check=False,
            timeout=120,
            cwd=REPO_ROOT,
            env=run_env,
        )

    def write_csv(self, path: Path, kernels=("selected_kernel",), real_values=None) -> None:
        path.parent.mkdir(parents=True, exist_ok=True)
        if real_values is None:
            real_values = [1.0, 3.0, 4.0, 5.0, 6.0, 7.0]
        with path.open("w", newline="") as f:
            writer = csv.writer(f)
            writer.writerow(
                ["kernel_name", "problem_size", "real_ms", "sim_ms", "abs_error_ms", "rel_error"]
            )
            for kernel in kernels:
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


    def matplotlib_blocked_env(self, shim_parent: Path):
        shim_dir = shim_parent / "matplotlib_blocked"
        package_dir = shim_dir / "matplotlib"
        package_dir.mkdir(parents=True)
        (package_dir / "__init__.py").write_text(
            'raise ModuleNotFoundError("No module named matplotlib")\n'
        )
        env = os.environ.copy()
        existing = env.get("PYTHONPATH")
        env["PYTHONPATH"] = str(shim_dir) if not existing else f"{shim_dir}{os.pathsep}{existing}"
        return env

    def assert_no_trailing_whitespace(self, text: str) -> None:
        bad_lines = [
            str(line_no)
            for line_no, line in enumerate(text.splitlines(), start=1)
            if line.rstrip(" \t") != line
        ]
        self.assertFalse(
            bad_lines,
            "generated Markdown has trailing whitespace on line(s): " + ", ".join(bad_lines[:20]),
        )

    def write_manifest(self, path: Path, kernel: str = "selected_kernel") -> None:
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
                    "rationale": "Small deterministic fixture for curated report tests.",
                    "coverage_expectation": {
                        "policy": "tier2_flat_nonflat_v1",
                        "min_flat_points_if_hw_flat_exists": 1,
                        "min_non_flat_points_if_hw_non_flat_exists": 5,
                        "min_trend_points": 3,
                    },
                    "coverage_evidence": {
                        "hardware_points": 6,
                        "matched_points": 6,
                        "hardware_flat_points": 1,
                        "matched_flat_points": 1,
                        "hardware_non_flat_points": 5,
                        "matched_non_flat_points": 5,
                        "point_coverage_ready": True,
                        "trend_points": 6,
                    },
                }
            ],
        }
        path.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n")

    def write_partial_manifest(self, path: Path, kernel: str = "partial_kernel") -> None:
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
                    "rationale": "Partial deterministic fixture for curated report tests.",
                    "coverage_expectation": {
                        "policy": "tier2_flat_nonflat_v1",
                        "min_flat_points_if_hw_flat_exists": 1,
                        "min_non_flat_points_if_hw_non_flat_exists": 5,
                        "min_trend_points": 3,
                        "partial_coverage_allowed": True,
                        "partial_coverage_policy": "partial_current_data_v1",
                    },
                    "coverage_evidence": {
                        "hardware_points": 5,
                        "matched_points": 5,
                        "hardware_flat_points": 1,
                        "matched_flat_points": 1,
                        "hardware_non_flat_points": 4,
                        "matched_non_flat_points": 4,
                        "point_coverage_ready": False,
                        "coverage_status": "partial",
                        "partial_coverage_reason": "Fixture intentionally has too few non-flat points.",
                        "coverage_gaps": [
                            {
                                "field": "matched_non_flat_points",
                                "observed": 4,
                                "required": 5,
                            }
                        ],
                        "trend_points": 5,
                    },
                }
            ],
        }
        path.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n")

    def test_scope_csv_defaults_preserve_all_and_curated_contracts(self) -> None:
        self.assertEqual(gvr.default_csv_path("all"), gvr.SELECTED_RUN_COMPARISON_PATH)
        self.assertEqual(gvr.default_csv_path("curated"), gvr.CSV_PATH)
        self.assertEqual(gvr.default_output_path("all"), gvr.REPORT_PATH)
        self.assertEqual(gvr.default_output_path("curated"), gvr.CURATED_REPORT_PATH)
        self.assertIsNone(gvr.default_summary_path("all"))
        self.assertEqual(gvr.default_summary_path("curated"), gvr.CURATED_SUMMARY_PATH)
        self.assertEqual(
            gvr.resolve_csv_path("all", TOP_LEVEL_COMPARISON_REFERENCE),
            Path(TOP_LEVEL_COMPARISON_REFERENCE),
        )

    def test_all_scope_default_csv_regenerates_committed_selected_run_report(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            output_path = tmp_path / "validation_report.md"
            summary_path = tmp_path / "summary.json"
            figures_dir = tmp_path / "figures"

            result = self.run_default_report_without_csv(
                output_path,
                summary_path,
                figures_dir,
            )

            self.assertEqual(result.returncode, 0, msg=result.stderr)
            self.assertIn(SELECTED_RUN_COMPARISON_REFERENCE, result.stdout)
            report = output_path.read_text(encoding="utf-8")
            self.assertEqual(
                report,
                (REPO_ROOT / "docs" / "validation_report.md").read_text(encoding="utf-8"),
            )
            self.assertIn(
                f"**Data source:** `{SELECTED_RUN_COMPARISON_REFERENCE}`",
                report,
            )
            self.assertIn(f"- **Raw data:** `{SELECTED_RUN_COMPARISON_REFERENCE}`", report)
            self.assertNotIn(f"- **Raw data:** `{TOP_LEVEL_COMPARISON_REFERENCE}`", report)

            summary = json.loads(summary_path.read_text(encoding="utf-8"))
            self.assertEqual(summary["scope"], "all")
            self.assertEqual(summary["data_source"], SELECTED_RUN_COMPARISON_REFERENCE)
            self.assertEqual(summary["aggregate_metrics"]["total_points"], 579)
            self.assertEqual(summary["aggregate_metrics"]["flat_points"], 362)
            self.assertEqual(summary["aggregate_metrics"]["nonflat_points"], 217)
            self.assertTrue((figures_dir / "slowdown_bar_chart.png").is_file())
            self.assertTrue((figures_dir / "sim_vs_hw_scatter.png").is_file())
            self.assertTrue(
                any(path.endswith("2dconvolution_scaling.png") for path in summary["figure_files"])
            )
            self.assertFalse(
                any("curated_" in Path(path).name for path in summary["figure_files"])
            )

    def test_all_scope_explicit_csv_override_sets_report_summary_and_figures(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            csv_path = tmp_path / "benchmark-comparison" / "comparison_ci.csv"
            output_path = tmp_path / "validation_report.md"
            summary_path = tmp_path / "summary.json"
            figures_dir = tmp_path / "figures"
            self.write_csv(csv_path)

            result = self.run_default_report(csv_path, output_path, summary_path, figures_dir)

            self.assertEqual(result.returncode, 0, msg=result.stderr)
            report = output_path.read_text(encoding="utf-8")
            try:
                expected_source = csv_path.resolve().relative_to(REPO_ROOT).as_posix()
            except ValueError:
                expected_source = csv_path.as_posix()
            self.assertIn(f"**Data source:** `{expected_source}`", report)
            self.assertIn(f"- **Raw data:** `{expected_source}`", report)
            self.assertNotIn(SELECTED_RUN_COMPARISON_REFERENCE, report)

            summary = json.loads(summary_path.read_text(encoding="utf-8"))
            self.assertEqual(summary["scope"], "all")
            self.assertEqual(summary["data_source"], expected_source)
            self.assertTrue(
                any(path.endswith("selected_kernel_scaling.png") for path in summary["figure_files"])
            )
            self.assertFalse(
                any("curated_" in Path(path).name for path in summary["figure_files"])
            )
            self.assertTrue((figures_dir / "selected_kernel_scaling.png").is_file())

    def test_curated_scope_default_csv_keeps_current_top_level_outputs(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            output_path = tmp_path / "validation_report_curated.md"
            summary_path = tmp_path / "mi300a_evaluation_set_summary.json"
            figures_dir = tmp_path / "figures"

            result = self.run_command(
                [
                    sys.executable,
                    str(SCRIPT),
                    "--scope",
                    "curated",
                    "--output",
                    str(output_path),
                    "--summary-json",
                    str(summary_path),
                    "--figures-dir",
                    str(figures_dir),
                ],
                output_path,
            )

            self.assertEqual(result.returncode, 0, msg=result.stderr)
            report = output_path.read_text(encoding="utf-8")
            self.assertIn(f"**Data source:** `{TOP_LEVEL_COMPARISON_REFERENCE}`", report)
            self.assertIn(f"- **Raw data:** `{TOP_LEVEL_COMPARISON_REFERENCE}`", report)
            self.assertIn("MGPUSim MI300A Curated Validation Report", report)
            self.assertIn("Curated MI300A primary evaluation set", report)

            summary = json.loads(summary_path.read_text(encoding="utf-8"))
            self.assertEqual(summary["scope"], "curated")
            self.assertEqual(summary["data_source"], TOP_LEVEL_COMPARISON_REFERENCE)
            self.assertEqual(summary["report_path"], gvr.repo_relative(output_path))
            self.assertTrue(
                all("curated_" in Path(path).name for path in summary["figure_files"])
            )
            self.assertTrue((figures_dir / "curated_slowdown_bar_chart.png").is_file())

            kernels = {kernel["kernel_name"]: kernel for kernel in summary["kernels"]}
            l1_evidence = kernels["l1_cache_bw"]["evidence"]
            l2_evidence = kernels["l2_cache_bw"]["evidence"]
            self.assertEqual(l1_evidence["hardware_rows"], 16)
            self.assertEqual(l1_evidence["sim_completed_rows"], 16)
            self.assertEqual(l1_evidence["hardware_flat_rows"], 16)
            self.assertEqual(l1_evidence["hardware_non_flat_rows"], 0)
            self.assertAlmostEqual(
                l1_evidence["hardware_value_change_ratio"],
                1.148086522463,
            )
            self.assertAlmostEqual(l1_evidence["raw_avg_error"], 0.939625487055)
            self.assertFalse(l1_evidence["boosted_metrics_scored"])
            self.assertFalse(l1_evidence["gate"]["passed"])
            self.assertIn(
                "loaded_hardware_value_changing",
                l1_evidence["gate"]["reason"],
            )
            self.assertEqual(l2_evidence["hardware_rows"], 16)
            self.assertEqual(l2_evidence["sim_completed_rows"], 5)
            self.assertEqual(l2_evidence["hardware_flat_rows"], 16)
            self.assertEqual(l2_evidence["hardware_non_flat_rows"], 0)
            self.assertAlmostEqual(l2_evidence["raw_avg_error"], 0.894679076461)
            self.assertFalse(l2_evidence["shape"]["scaling_informative"])

    def test_curated_scope_filters_report_summary_and_figures_to_manifest(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            csv_path = tmp_path / "comparison_ci.csv"
            manifest_path = tmp_path / "mi300a_evaluation_set.json"
            output_path = tmp_path / "validation_report_curated.md"
            summary_path = tmp_path / "mi300a_evaluation_set_summary.json"
            figures_dir = tmp_path / "figures"
            self.write_csv(csv_path, kernels=("selected_kernel", "broad_diagnostic_kernel"))
            self.write_manifest(manifest_path)

            result = self.run_report(csv_path, manifest_path, output_path, summary_path, figures_dir)

            self.assertEqual(result.returncode, 0, msg=result.stderr)
            report = output_path.read_text()
            self.assertIn("unit-test-curated-set", report)
            self.assertIn("Selected benchmarks: **1**", report)
            self.assertIn("Broad-suite diagnostics policy", report)
            self.assertIn("selected_kernel", report)
            self.assertNotIn("broad_diagnostic_kernel", report)

            summary = json.loads(summary_path.read_text())
            self.assertEqual(summary["scope"], "curated")
            self.assertEqual(summary["set_id"], "unit-test-curated-set")
            self.assertEqual(summary["selected_count"], 1)
            self.assertEqual(summary["tier_counts"], {"2": 1})
            self.assertEqual(summary["category_counts"], {"unit_test_category": 1})
            self.assertEqual(
                [kernel["kernel_name"] for kernel in summary["kernels"]],
                ["selected_kernel"],
            )
            self.assertTrue(
                any(path.endswith("curated_selected_kernel_scaling.png") for path in summary["figure_files"])
            )
            self.assertFalse(
                any("broad_diagnostic_kernel" in path for path in summary["figure_files"])
            )
            self.assertTrue((figures_dir / "curated_selected_kernel_scaling.png").is_file())
            self.assertFalse((figures_dir / "curated_broad_diagnostic_kernel_scaling.png").exists())

    def test_curated_scope_reports_partial_coverage_without_failing(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            csv_path = tmp_path / "comparison_ci.csv"
            manifest_path = tmp_path / "mi300a_evaluation_set.json"
            output_path = tmp_path / "validation_report_curated.md"
            summary_path = tmp_path / "mi300a_evaluation_set_summary.json"
            figures_dir = tmp_path / "figures"
            self.write_csv(csv_path, kernels=("partial_kernel",), real_values=[1.0, 3.0, 4.0, 5.0, 6.0])
            self.write_partial_manifest(manifest_path)

            result = self.run_report(csv_path, manifest_path, output_path, summary_path, figures_dir)

            self.assertEqual(result.returncode, 0, msg=result.stderr)
            report = output_path.read_text(encoding="utf-8")
            self.assert_no_trailing_whitespace(report)
            self.assertIn("Coverage status: **partial**", report)
            self.assertIn("0/1 manifest-full coverage benchmark", report)
            self.assertIn("`partial_kernel`", report)
            self.assertIn("Curated partial entries", report)

            summary = json.loads(summary_path.read_text())
            point_status = summary["point_coverage_status"]
            self.assertEqual(point_status["status"], "partial")
            self.assertEqual(point_status["selected_with_passing_coverage"], 0)
            self.assertEqual(point_status["selected_with_partial_coverage"], 1)
            self.assertEqual(summary["coverage_status"]["non_compliant"], 1)
            self.assertEqual(summary["kernels"][0]["coverage"]["manifest_status"], "partial")
            self.assertEqual(
                summary["kernels"][0]["coverage"]["coverage_gaps"],
                [{"field": "matched_non_flat_points", "observed": 4, "required": 5}],
            )

    def test_default_scope_reports_regular_workflow_contract(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            csv_path = tmp_path / "comparison_ci.csv"
            output_path = tmp_path / "validation_report.md"
            summary_path = tmp_path / "summary.json"
            figures_dir = tmp_path / "figures"
            self.write_csv(csv_path)

            result = self.run_default_report(csv_path, output_path, summary_path, figures_dir)

            self.assertEqual(result.returncode, 0, msg=result.stderr)
            report = output_path.read_text(encoding="utf-8")
            self.assert_no_trailing_whitespace(report)
            self.assertIn("**Hardware reference:** AMD Instinct MI300A<br>", report)
            self.assertIn("**Points:** 6 total (1 flat, 5 non-flat)<br>", report)
            self.assertIn("Regular Workflow Contract", report)
            self.assertIn("19-benchmark Tier 1 / 308 plan entries", report)
            self.assertIn("63-benchmark Tier 2 / 1108 plan entries", report)
            self.assertIn("7200s per simulator invocation (`timeout_sec: 7200`)", report)
            self.assertIn(
                "360min/21600s per benchmark-tier job (`timeout-minutes: 360`)",
                report,
            )
            self.assertIn("`max-parallel: 14`", report)
            self.assertIn(
                "Validation -> Tier 1 -> Tier-1 summary -> Tier 2 -> Summary/report",
                report,
            )

    def test_default_scope_raw_data_reference_uses_selected_run_csv(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            csv_path = (
                tmp_path
                / "benchmark-comparison"
                / "selected-run-25619929396"
                / "comparison_ci.detailed.csv"
            )
            output_path = tmp_path / "validation_report.md"
            summary_path = tmp_path / "summary.json"
            figures_dir = tmp_path / "figures"
            self.write_csv(csv_path)

            result = self.run_default_report(
                csv_path,
                output_path,
                summary_path,
                figures_dir,
            )

            self.assertEqual(result.returncode, 0, msg=result.stderr)
            report = output_path.read_text(encoding="utf-8")
            try:
                expected_source = csv_path.resolve().relative_to(REPO_ROOT).as_posix()
            except ValueError:
                expected_source = csv_path.as_posix()
            self.assertIn(f"**Data source:** `{expected_source}`", report)
            self.assertIn(f"- **Raw data:** `{expected_source}`", report)
            self.assertNotIn(
                "- **Raw data:** `benchmark-comparison/comparison_ci.csv`",
                report,
            )

    def test_curated_scope_fails_loud_without_matplotlib(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            csv_path = tmp_path / "comparison_ci.csv"
            manifest_path = tmp_path / "mi300a_evaluation_set.json"
            output_path = tmp_path / "validation_report_curated.md"
            summary_path = tmp_path / "mi300a_evaluation_set_summary.json"
            figures_dir = tmp_path / "figures"
            self.write_csv(csv_path, kernels=("selected_kernel", "broad_diagnostic_kernel"))
            self.write_manifest(manifest_path)

            result = self.run_report(
                csv_path,
                manifest_path,
                output_path,
                summary_path,
                figures_dir,
                env=self.matplotlib_blocked_env(tmp_path),
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("matplotlib is required", result.stderr)
            self.assertIn("validation report figures", result.stderr)
            self.assertIn("No module named matplotlib", result.stderr)
            self.assertFalse(output_path.exists())
            self.assertFalse(summary_path.exists())
            self.assertFalse(figures_dir.exists())

    def test_default_report_fails_loud_without_matplotlib(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            csv_path = tmp_path / "comparison_ci.csv"
            output_path = tmp_path / "validation_report.md"
            summary_path = tmp_path / "summary.json"
            figures_dir = tmp_path / "figures"
            self.write_csv(csv_path)

            result = self.run_default_report(
                csv_path,
                output_path,
                summary_path,
                figures_dir,
                env=self.matplotlib_blocked_env(tmp_path),
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("matplotlib is required", result.stderr)
            self.assertIn("validation report figures", result.stderr)
            self.assertIn("No module named matplotlib", result.stderr)
            self.assertFalse(output_path.exists())
            self.assertFalse(summary_path.exists())
            self.assertFalse(figures_dir.exists())

    def test_curated_scope_rejects_malformed_manifest_without_matplotlib(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            csv_path = tmp_path / "comparison_ci.csv"
            manifest_path = tmp_path / "mi300a_evaluation_set.json"
            output_path = tmp_path / "validation_report_curated.md"
            summary_path = tmp_path / "summary.json"
            figures_dir = tmp_path / "figures"
            self.write_csv(csv_path)
            self.write_manifest(manifest_path)
            manifest = json.loads(manifest_path.read_text())
            del manifest["benchmarks"][0]["coverage_evidence"]
            manifest_path.write_text(json.dumps(manifest) + "\n")

            result = self.run_report(
                csv_path,
                manifest_path,
                output_path,
                summary_path,
                figures_dir,
                env=self.matplotlib_blocked_env(tmp_path),
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("malformed manifest", result.stderr)
            self.assertIn("coverage_evidence", result.stderr)
            self.assertNotIn("matplotlib is required", result.stderr)
            self.assertFalse(output_path.exists())
            self.assertFalse(summary_path.exists())

    def test_curated_scope_rejects_malformed_manifest(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            csv_path = tmp_path / "comparison_ci.csv"
            manifest_path = tmp_path / "mi300a_evaluation_set.json"
            output_path = tmp_path / "validation_report_curated.md"
            summary_path = tmp_path / "summary.json"
            figures_dir = tmp_path / "figures"
            self.write_csv(csv_path)
            self.write_manifest(manifest_path)
            manifest = json.loads(manifest_path.read_text())
            del manifest["benchmarks"][0]["coverage_evidence"]
            manifest_path.write_text(json.dumps(manifest) + "\n")

            result = self.run_report(csv_path, manifest_path, output_path, summary_path, figures_dir)

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("malformed manifest", result.stderr)
            self.assertIn("coverage_evidence", result.stderr)
            self.assertFalse(output_path.exists())
            self.assertFalse(summary_path.exists())

    def test_curated_scope_rejects_unknown_selected_kernel(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            csv_path = tmp_path / "comparison_ci.csv"
            manifest_path = tmp_path / "mi300a_evaluation_set.json"
            output_path = tmp_path / "validation_report_curated.md"
            summary_path = tmp_path / "summary.json"
            figures_dir = tmp_path / "figures"
            self.write_csv(csv_path, kernels=("known_kernel",))
            self.write_manifest(manifest_path, kernel="missing_kernel")

            result = self.run_report(csv_path, manifest_path, output_path, summary_path, figures_dir)

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("unknown selected kernel: missing_kernel", result.stderr)
            self.assertFalse(output_path.exists())
            self.assertFalse(summary_path.exists())


if __name__ == "__main__":
    unittest.main()
