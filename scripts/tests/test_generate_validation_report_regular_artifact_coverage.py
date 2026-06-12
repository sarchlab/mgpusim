from __future__ import annotations

import csv
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPT = REPO_ROOT / "scripts" / "generate_validation_report.py"


class GenerateValidationReportRegularArtifactCoverageTest(unittest.TestCase):
    def setUp(self) -> None:
        (REPO_ROOT / ".repo_tmp").mkdir(exist_ok=True)

    def run_regular_summary(
        self,
        tmp_path: Path,
        *,
        expected_rows: int = 5,
        sim_rows: int = 5,
        comparison_rows: int = 5,
        regression_rows: int = 5,
        stage_results: dict[str, str] | None = None,
        headerless_csvs: tuple[str, ...] = (),
        malformed_header_csvs: tuple[str, ...] = (),
        empty_csvs: tuple[str, ...] = (),
    ) -> subprocess.CompletedProcess[str]:
        matrix_report = tmp_path / "regular_matrix_validation.json"
        sim_csv = tmp_path / "sim_results_ci.csv"
        comparison_csv = tmp_path / "comparison_ci.csv"
        regression_csv = tmp_path / "regression_ci.csv"
        summary_json = tmp_path / "regular_artifact_coverage_summary.json"
        summary_md = tmp_path / "regular_artifact_coverage_summary.md"

        csv_paths = {
            "simulation": sim_csv,
            "comparison": comparison_csv,
            "regression": regression_csv,
        }
        csv_rows = {
            "simulation": sim_rows,
            "comparison": comparison_rows,
            "regression": regression_rows,
        }
        csv_headers = {
            "simulation": [
                "kernel_name",
                "problem_size",
                "iterations",
                "avg_ms",
                "min_ms",
                "max_ms",
            ],
            "comparison": [
                "kernel_name",
                "problem_size",
                "real_ms",
                "sim_ms",
                "abs_error_ms",
                "rel_error",
            ],
            "regression": [
                "kernel",
                "group",
                "label",
                "problem_size",
                "numeric_size",
                "hw_ms",
                "sim_ms",
            ],
        }

        self.write_matrix_report(matrix_report, tier1_rows=2, tier2_rows=expected_rows - 2)
        for artifact in ("simulation", "comparison", "regression"):
            if artifact in empty_csvs:
                csv_paths[artifact].write_text("", encoding="utf-8")
            elif artifact in malformed_header_csvs:
                self.write_malformed_header_csv(
                    csv_paths[artifact],
                    rows=csv_rows[artifact],
                    columns=len(csv_headers[artifact]),
                )
            elif artifact in headerless_csvs:
                self.write_headerless_csv(
                    csv_paths[artifact],
                    total_lines=expected_rows + 1,
                    columns=len(csv_headers[artifact]),
                )
            else:
                self.write_csv(
                    csv_paths[artifact],
                    csv_rows[artifact],
                    csv_headers[artifact],
                )

        stage_results = stage_results or {
            "validation": "success",
            "benchmark-tier1": "success",
            "tier1-summary": "success",
            "benchmark-tier2": "success",
        }
        cmd = [
            sys.executable,
            str(SCRIPT),
            "--regular-artifact-coverage",
            "--regular-matrix-report",
            str(matrix_report),
            "--regular-sim-csv",
            str(sim_csv),
            "--regular-comparison-csv",
            str(comparison_csv),
            "--regular-regression-csv",
            str(regression_csv),
            "--regular-coverage-json",
            str(summary_json),
            "--regular-coverage-md",
            str(summary_md),
        ]
        for stage, result in stage_results.items():
            cmd.extend(["--regular-stage-result", f"{stage}={result}"])
        return subprocess.run(
            cmd,
            cwd=REPO_ROOT,
            capture_output=True,
            text=True,
            timeout=30,
            check=False,
        )

    def test_regular_artifact_coverage_labels_complete_when_all_rows_and_stages_match(self) -> None:
        with tempfile.TemporaryDirectory(dir=REPO_ROOT / ".repo_tmp") as tmp:
            tmp_path = Path(tmp)
            result = self.run_regular_summary(tmp_path)

            self.assertEqual(result.returncode, 0, msg=result.stderr)
            summary_path = tmp_path / "regular_artifact_coverage_summary.json"
            markdown_path = tmp_path / "regular_artifact_coverage_summary.md"
            summary = json.loads(summary_path.read_text(encoding="utf-8"))
            stdout_summary = json.loads(result.stdout)
            markdown = markdown_path.read_text(encoding="utf-8")

        self.assertEqual(summary, stdout_summary)
        self.assertEqual(summary["schema_name"], "mgpusim.mi300a_regular_artifact_coverage_summary")
        self.assertEqual(summary["evidence_label"], "complete_regular_evidence")
        self.assertTrue(summary["complete_regular_evidence"])
        self.assertEqual(summary["expected"]["tier1_rows"], 2)
        self.assertEqual(summary["expected"]["tier2_rows"], 3)
        self.assertEqual(summary["expected"]["total_rows"], 5)
        self.assertEqual(summary["observed"]["simulation"]["data_rows"], 5)
        self.assertEqual(summary["observed"]["comparison"]["data_rows"], 5)
        self.assertEqual(summary["observed"]["regression"]["data_rows"], 5)
        self.assertEqual(summary["reasons"], [])
        self.assertIn("## Label interpretation", markdown)
        self.assertIn("`complete_regular_evidence`: every required upstream stage succeeded", markdown)
        self.assertIn("complete regular artifact coverage only", markdown)
        self.assertIn("not finishability/provenance completion", markdown)
        self.assertIn("`partial_diagnostic_regular_evidence`: required CSVs are present", markdown)
        self.assertIn("`insufficient_regular_evidence`: a required CSV is missing", markdown)
        self.assertIn("| benchmark-tier1 | 1 | 2 |", markdown)
        self.assertIn("| simulation | `", markdown)
        self.assertIn("`matches_expected`", markdown)

    def test_regular_artifact_coverage_rejects_headerless_data_only_csvs(self) -> None:
        for artifact in ("simulation", "comparison", "regression"):
            with self.subTest(artifact=artifact):
                with tempfile.TemporaryDirectory(dir=REPO_ROOT / ".repo_tmp") as tmp:
                    tmp_path = Path(tmp)
                    result = self.run_regular_summary(tmp_path, headerless_csvs=(artifact,))

                    self.assertEqual(result.returncode, 0, msg=result.stderr)
                    summary = json.loads(
                        (tmp_path / "regular_artifact_coverage_summary.json").read_text(
                            encoding="utf-8"
                        )
                    )

                record = summary["observed"][artifact]
                expected_rows = summary["expected"]["total_rows"]
                self.assertEqual(record["data_rows"], expected_rows)
                self.assertFalse(record["header_recognized"])
                self.assertFalse(record["matches_expected"])
                self.assertEqual(record["status"], "malformed_header")
                self.assertEqual(summary["evidence_label"], "insufficient_regular_evidence")
                self.assertFalse(summary["complete_regular_evidence"])
                self.assertTrue(
                    any(
                        artifact in reason
                        and "missing required header columns" in reason
                        for reason in summary["reasons"]
                    )
                )

    def test_regular_artifact_coverage_rejects_explicit_malformed_headers(self) -> None:
        for artifact in ("simulation", "comparison", "regression"):
            with self.subTest(artifact=artifact):
                with tempfile.TemporaryDirectory(dir=REPO_ROOT / ".repo_tmp") as tmp:
                    tmp_path = Path(tmp)
                    result = self.run_regular_summary(
                        tmp_path,
                        malformed_header_csvs=(artifact,),
                    )

                    self.assertEqual(result.returncode, 0, msg=result.stderr)
                    summary = json.loads(
                        (tmp_path / "regular_artifact_coverage_summary.json").read_text(
                            encoding="utf-8"
                        )
                    )

                record = summary["observed"][artifact]
                expected_rows = summary["expected"]["total_rows"]
                self.assertEqual(record["data_rows"], expected_rows)
                self.assertFalse(record["header_recognized"])
                self.assertFalse(record["matches_expected"])
                self.assertEqual(record["status"], "malformed_header")
                self.assertEqual(summary["evidence_label"], "insufficient_regular_evidence")
                self.assertFalse(summary["complete_regular_evidence"])
                self.assertTrue(record["missing_header_columns"])
                self.assertTrue(
                    any(
                        artifact in reason
                        and "header is not recognizable" in reason
                        and "missing required header columns" in reason
                        for reason in summary["reasons"]
                    )
                )

    def test_regular_artifact_coverage_rejects_empty_files(self) -> None:
        for artifact in ("simulation", "comparison", "regression"):
            with self.subTest(artifact=artifact):
                with tempfile.TemporaryDirectory(dir=REPO_ROOT / ".repo_tmp") as tmp:
                    tmp_path = Path(tmp)
                    result = self.run_regular_summary(tmp_path, empty_csvs=(artifact,))

                    self.assertEqual(result.returncode, 0, msg=result.stderr)
                    summary = json.loads(
                        (tmp_path / "regular_artifact_coverage_summary.json").read_text(
                            encoding="utf-8"
                        )
                    )

                record = summary["observed"][artifact]
                self.assertEqual(record["data_rows"], 0)
                self.assertEqual(record["header_columns"], [])
                self.assertFalse(record["header_recognized"])
                self.assertFalse(record["matches_expected"])
                self.assertEqual(record["status"], "missing_header")
                self.assertEqual(summary["evidence_label"], "insufficient_regular_evidence")
                self.assertFalse(summary["complete_regular_evidence"])
                self.assertIn(
                    f"{artifact} CSV {record['path']} is empty or missing a header",
                    summary["reasons"],
                )

    def test_regular_artifact_coverage_marks_row_mismatch_as_partial_diagnostic(self) -> None:
        with tempfile.TemporaryDirectory(dir=REPO_ROOT / ".repo_tmp") as tmp:
            tmp_path = Path(tmp)
            result = self.run_regular_summary(tmp_path, sim_rows=4)

            self.assertEqual(result.returncode, 0, msg=result.stderr)
            summary = json.loads((tmp_path / "regular_artifact_coverage_summary.json").read_text(encoding="utf-8"))
            markdown = (tmp_path / "regular_artifact_coverage_summary.md").read_text(encoding="utf-8")

        self.assertEqual(summary["evidence_label"], "partial_diagnostic_regular_evidence")
        self.assertFalse(summary["complete_regular_evidence"])
        self.assertEqual(summary["observed"]["simulation"]["status"], "row_count_mismatch")
        self.assertIn("simulation CSV", summary["reasons"][0])
        self.assertIn("has 4 data rows; expected 5", summary["reasons"][0])
        self.assertIn("`row_count_mismatch`", markdown)

    def test_regular_artifact_coverage_marks_empty_reference_fallbacks_insufficient(self) -> None:
        with tempfile.TemporaryDirectory(dir=REPO_ROOT / ".repo_tmp") as tmp:
            tmp_path = Path(tmp)
            result = self.run_regular_summary(
                tmp_path,
                comparison_rows=0,
                regression_rows=0,
            )

            self.assertEqual(result.returncode, 0, msg=result.stderr)
            summary = json.loads((tmp_path / "regular_artifact_coverage_summary.json").read_text(encoding="utf-8"))
            markdown = (tmp_path / "regular_artifact_coverage_summary.md").read_text(encoding="utf-8")

        self.assertEqual(summary["evidence_label"], "insufficient_regular_evidence")
        self.assertFalse(summary["complete_regular_evidence"])
        self.assertEqual(summary["observed"]["comparison"]["status"], "empty_reference_fallback_csv")
        self.assertEqual(summary["observed"]["regression"]["status"], "empty_reference_fallback_csv")
        self.assertTrue(any("empty reference fallback" in reason for reason in summary["reasons"]))
        self.assertIn("`empty_reference_fallback_csv`", markdown)

    def test_regular_artifact_coverage_treats_missing_stage_results_as_unknown(self) -> None:
        with tempfile.TemporaryDirectory(dir=REPO_ROOT / ".repo_tmp") as tmp:
            tmp_path = Path(tmp)
            result = self.run_regular_summary(
                tmp_path,
                stage_results={"validation": "success", "benchmark-tier1": "success"},
            )

            self.assertEqual(result.returncode, 0, msg=result.stderr)
            summary = json.loads((tmp_path / "regular_artifact_coverage_summary.json").read_text(encoding="utf-8"))

        self.assertEqual(summary["evidence_label"], "partial_diagnostic_regular_evidence")
        self.assertFalse(summary["complete_regular_evidence"])
        stage_results = {stage["stage"]: stage["result"] for stage in summary["upstream_stages"]}
        self.assertEqual(stage_results["tier1-summary"], "unknown")
        self.assertEqual(stage_results["benchmark-tier2"], "unknown")
        self.assertIn("upstream stage tier1-summary result is unknown", summary["reasons"])

    def write_matrix_report(self, path: Path, *, tier1_rows: int, tier2_rows: int) -> None:
        report = {
            "schema_name": "mgpusim.mi300a_regular_workflow_matrix_validation",
            "schema_version": 1,
            "status": "pass",
            "source_plan": "fixture/mi300a_problem_size_discovery_plan.json",
            "matrix": {
                "total_matrix_rows": 2,
                "total_plan_entries": tier1_rows + tier2_rows,
                "jobs": {
                    "benchmark-tier1": {"matrix_rows": 1, "plan_entries": tier1_rows},
                    "benchmark-tier2": {"matrix_rows": 1, "plan_entries": tier2_rows},
                },
            },
        }
        path.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")

    def write_csv(self, path: Path, rows: int, header: list[str]) -> None:
        with path.open("w", encoding="utf-8", newline="") as f:
            writer = csv.writer(f)
            writer.writerow(header)
            for index in range(rows):
                writer.writerow([f"value-{index}-{column}" for column in range(len(header))])

    def write_malformed_header_csv(self, path: Path, *, rows: int, columns: int) -> None:
        with path.open("w", encoding="utf-8", newline="") as f:
            writer = csv.writer(f)
            writer.writerow([f"unexpected_header_{column}" for column in range(columns)])
            for index in range(rows):
                writer.writerow([f"value-{index}-{column}" for column in range(columns)])

    def write_headerless_csv(self, path: Path, *, total_lines: int, columns: int) -> None:
        with path.open("w", encoding="utf-8", newline="") as f:
            writer = csv.writer(f)
            for index in range(total_lines):
                writer.writerow([f"data-{index}-{column}" for column in range(columns)])


if __name__ == "__main__":
    unittest.main()
