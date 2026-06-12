import csv
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
CLI = REPO_ROOT / "scripts" / "derive_mi300a_regular_finishability_evidence.py"
SELECTED_RUN_257_ROOT = Path("benchmark-comparison") / "selected-run-25710089668"
SELECTED_RUN_258_ROOT = Path("benchmark-comparison") / "selected-run-25841274346"
MI300A_PLAN = Path("benchmark-comparison") / "mi300a_problem_size_discovery_plan.json"
SELECTED_RUN_257_STATUS_COUNTS = {
    "cancelled": 0,
    "fail": 0,
    "no-result": 793,
    "pass": 623,
    "timeout": 0,
}


class DeriveMi300ARegularFinishabilityEvidenceTest(unittest.TestCase):
    def write_json(self, path: Path, data) -> None:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(
            json.dumps(data, indent=2, sort_keys=True) + "\n",
            encoding="utf-8",
        )

    def write_csv(self, path: Path, rows, extra_columns=()) -> None:
        path.parent.mkdir(parents=True, exist_ok=True)
        with path.open("w", newline="", encoding="utf-8") as fh:
            writer = csv.writer(fh)
            writer.writerow([
                "kernel_name",
                "problem_size",
                "iterations",
                "avg_ms",
                "min_ms",
                "max_ms",
                *extra_columns,
            ])
            writer.writerows(rows)

    def make_fixture(
        self,
        root: Path,
        *,
        run_status="completed",
        non_terminal=False,
        omit_matrix_runtime_fields=False,
        job_conclusion="failure",
        step_conclusion="failure",
        result_rows=None,
        result_extra_columns=(),
    ) -> tuple[Path, Path]:
        artifact_root = root / "artifacts"
        plan = root / "plan.json"
        self.write_json(
            plan,
            {
                "schema_name": "fixture.plan",
                "entries": [
                    {
                        "plan_index": 1,
                        "workflow_job": "benchmark-tier1",
                        "workflow_benchmark": "fixture_bench",
                        "tier": 1,
                        "tier_name": "tier1",
                        "hardware_kernel_name": "fixture_kernel",
                        "hardware_problem_size": "small",
                        "problem_size": "small",
                        "size_label": "small",
                        "timeout_sec": 3600,
                    },
                    {
                        "plan_index": 2,
                        "workflow_job": "benchmark-tier1",
                        "workflow_benchmark": "fixture_bench",
                        "tier": 1,
                        "tier_name": "tier1",
                        "hardware_kernel_name": "fixture_kernel",
                        "hardware_problem_size": "large",
                        "problem_size": "large",
                        "size_label": "large",
                        "timeout_sec": 3600,
                    },
                ],
            },
        )
        self.write_json(
            artifact_root / "inventory" / "run-view.json",
            {
                "databaseId": 25576382818 if run_status == "completed" else 24959959195,
                "headSha": "7776c8db96b0acf19c86e1e8953fd29c0d9283af",
                "status": run_status,
                "conclusion": "failure" if run_status == "completed" else "",
                "non_terminal_snapshot": non_terminal,
                "url": "https://example.invalid/run",
                "workflowName": "MI300A Benchmark",
            },
        )
        self.write_json(
            artifact_root / "inventory" / "jobs.json",
            {
                "jobs": [
                    {
                        "name": "Tier 1 Bench: fixture_bench",
                        "databaseId": 123,
                        "status": "completed",
                        "conclusion": job_conclusion,
                        "startedAt": "2026-05-08T00:00:00Z",
                        "completedAt": "2026-05-08T01:00:00Z",
                        "url": "https://example.invalid/job/123",
                        "steps": [
                            {
                                "name": "Run benchmark sizes",
                                "number": 8,
                                "status": "completed",
                                "conclusion": step_conclusion,
                                "startedAt": "2026-05-08T00:00:10Z",
                                "completedAt": "2026-05-08T01:00:00Z",
                            }
                        ],
                    }
                ]
            },
        )
        matrix_row = {
            "benchmark": "fixture_bench",
            "artifact_id": "fixture-bench",
            "source_plan_index_ranges": ["1-2"],
            "plan_entries": 2,
            "size_count": 2,
            "planned_invocation_count": 2,
            "exceeds_benchmark_job_timeout": False,
        }
        if not omit_matrix_runtime_fields:
            matrix_row.update(
                {
                    "timeout_sec": 7200,
                    "per_invocation_timeout_sec": 7200,
                    "benchmark_job_timeout_sec": 21600,
                }
            )
        self.write_json(
            artifact_root
            / "extracted"
            / "validation-summary"
            / "regular_matrix_validation.json",
            {
                "matrix": {
                    "rows": {
                        "benchmark-tier1": [matrix_row],
                        "benchmark-tier2": [],
                    }
                }
            },
        )
        if result_rows is None:
            result_rows = [["fixture_kernel", "small", 1, "0.1", "0.1", "0.1"]]
        self.write_csv(
            artifact_root
            / "extracted"
            / "benchmark-results-fixture-bench"
            / "results_fixture_bench.csv",
            result_rows,
            extra_columns=result_extra_columns,
        )
        return artifact_root, plan

    def run_cli(self, *args: str):
        return subprocess.run(
            [sys.executable, str(CLI), *args],
            cwd=REPO_ROOT,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )

    def test_default_paths_accept_checked_in_collector_root_layout(self) -> None:
        result = self.run_cli(
            "--artifact-root",
            str(SELECTED_RUN_257_ROOT),
            "--expected-run-id",
            "25710089668",
            "--expected-head-sha",
            "6fdbbd1882f6a36c5e0846b9209a9a19c258b486",
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual(report["run"]["run_id"], "25710089668")
        self.assertTrue(report["run"]["terminal"])
        self.assertEqual(
            report["source"]["run_metadata"],
            str(SELECTED_RUN_257_ROOT / "run-view.json"),
        )
        self.assertEqual(
            report["source"]["jobs"],
            str(SELECTED_RUN_257_ROOT / "jobs.json"),
        )
        self.assertEqual(
            report["source"]["regular_matrix_report"],
            str(SELECTED_RUN_257_ROOT / "regular_matrix_validation.json"),
        )
        self.assertEqual(
            report["source"]["result_source_kind"],
            "merged_sim_results_ci_csv",
        )
        self.assertEqual(report["status_counts"], SELECTED_RUN_257_STATUS_COUNTS)

    def test_rederives_checked_in_257_evidence_from_selected_run_inputs(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            output_json = Path(tmp) / "mi300a_regular_finishability_evidence.json"
            output_md = Path(tmp) / "mi300a_regular_finishability_evidence.md"
            result = self.run_cli(
                "--artifact-root",
                str(SELECTED_RUN_257_ROOT),
                "--plan",
                str(MI300A_PLAN),
                "--expected-run-id",
                "25710089668",
                "--expected-head-sha",
                "6fdbbd1882f6a36c5e0846b9209a9a19c258b486",
                "--output-json",
                str(output_json),
                "--output-md",
                str(output_md),
            )

            self.assertEqual(result.returncode, 0, result.stderr)
            report = json.loads(output_json.read_text(encoding="utf-8"))
            checked_json = (
                REPO_ROOT
                / SELECTED_RUN_257_ROOT
                / "mi300a_regular_finishability_evidence.json"
            )
            checked_md = (
                REPO_ROOT
                / SELECTED_RUN_257_ROOT
                / "mi300a_regular_finishability_evidence.md"
            )
            checked_report = json.loads(checked_json.read_text(encoding="utf-8"))
            self.assertEqual(report["status_counts"], SELECTED_RUN_257_STATUS_COUNTS)
            self.assertEqual(report["status_counts"], checked_report["status_counts"])
            self.assertEqual(len(report["per_job_summary"]), 82)
            self.assertEqual(len(report["per_size_summary"]), 1416)
            self.assertEqual(
                report["source"]["result_source_paths"],
                [str(SELECTED_RUN_257_ROOT / "sim_results_ci.csv")],
            )
            self.assertEqual(
                output_json.read_text(encoding="utf-8"),
                checked_json.read_text(encoding="utf-8"),
            )
            self.assertEqual(
                output_md.read_text(encoding="utf-8"),
                checked_md.read_text(encoding="utf-8"),
            )

    def test_run_258_non_terminal_rejected_before_writing_evidence(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            output_dir = Path(tmp) / "evidence"
            output_json = output_dir / "mi300a_regular_finishability_evidence.json"
            output_md = output_dir / "mi300a_regular_finishability_evidence.md"
            result = self.run_cli(
                "--artifact-root",
                str(SELECTED_RUN_258_ROOT),
                "--expected-run-id",
                "25841274346",
                "--expected-head-sha",
                "e0ef96c57e042c636e2b94db8f294ff8a574bb78",
                "--output-json",
                str(output_json),
                "--output-md",
                str(output_md),
            )

            self.assertEqual(result.returncode, 2)
            self.assertFalse(output_json.exists())
            self.assertFalse(output_md.exists())
            self.assertFalse(output_json.parent.exists())

        self.assertEqual(result.stdout, "")
        self.assertIn("25841274346", result.stderr)
        self.assertIn("not terminal", result.stderr)
        self.assertIn("status=in_progress", result.stderr)
        self.assertNotIn("inventory/run-view.json", result.stderr)

    def test_derives_terminal_pass_and_no_result_statuses(self):
        with tempfile.TemporaryDirectory() as tmp:
            artifact_root, plan = self.make_fixture(Path(tmp))
            result = self.run_cli(
                "--artifact-root",
                str(artifact_root),
                "--plan",
                str(plan),
                "--expected-run-id",
                "25576382818",
                "--expected-head-sha",
                "7776c8db96b0acf19c86e1e8953fd29c0d9283af",
            )

        self.assertEqual(result.returncode, 0, result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual(
            report["schema_name"],
            "mgpusim.mi300a_regular_finishability_evidence",
        )
        self.assertEqual(report["run"]["run_id"], "25576382818")
        self.assertTrue(report["run"]["terminal"])
        self.assertEqual(
            report["status_counts"],
            {"cancelled": 0, "fail": 0, "no-result": 1, "pass": 1, "timeout": 0},
        )
        self.assertEqual(
            report["classification_counts"],
            {"cancelled": 0, "failure": 0, "no-result": 1, "pass": 1},
        )
        row = report["rows"][0]
        self.assertEqual(row["timeout_sec"], 7200)
        self.assertEqual(row["per_invocation_timeout_sec"], 7200)
        self.assertEqual(row["benchmark_job_timeout_sec"], 21600)
        self.assertEqual(row["terminal_classification"], "failure")
        self.assertEqual(row["evidence_classification"], "no-result")
        self.assertTrue(row["job_basis"]["job_terminal"])
        self.assertEqual(
            report["terminal"]["terminal_job_classification_counts"],
            {"cancelled": 0, "failure": 1, "no-result": 0, "pass": 0},
        )
        entries = row["entries"]
        self.assertEqual([entry["status"] for entry in entries], ["pass", "no-result"])
        self.assertEqual([entry["classification"] for entry in entries], ["pass", "no-result"])
        self.assertEqual(entries[0]["matched_result_row_count"], 1)
        self.assertEqual(entries[1]["matched_result_row_count"], 0)
        self.assertEqual(entries[1]["provenance"]["source"], "missing_result_row")
        self.assertIn("benchmark result CSV row emitted", entries[0]["basis"])
        self.assertIn("no matching result or simulator timing CSV row", entries[1]["basis"])
        self.assertEqual(report["per_job_summary"][0]["terminal_classification"], "failure")
        self.assertEqual(report["per_job_summary"][0]["evidence_classification"], "no-result")
        self.assertEqual(len(report["per_size_summary"]), 2)
        self.assertEqual(report["per_size_summary"][1]["classification"], "no-result")

    def test_markdown_includes_per_job_and_per_size_summaries(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            artifact_root, plan = self.make_fixture(Path(tmp))
            result = self.run_cli(
                "--artifact-root",
                str(artifact_root),
                "--plan",
                str(plan),
                "--expected-run-id",
                "25576382818",
                "--markdown-only",
            )

        self.assertEqual(result.returncode, 0, result.stderr)
        markdown = result.stdout
        self.assertIn("## Per-job summary", markdown)
        self.assertIn("## Per-size summary", markdown)
        self.assertIn("Terminal class", markdown)
        self.assertIn("Evidence class", markdown)
        self.assertIn("`failure`", markdown)
        self.assertIn("`no-result`", markdown)
        self.assertIn("missing per-size rows", markdown)

    def test_defaults_missing_matrix_runtime_fields_to_current_regular_contract(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            artifact_root, plan = self.make_fixture(
                Path(tmp),
                omit_matrix_runtime_fields=True,
            )
            result = self.run_cli(
                "--artifact-root",
                str(artifact_root),
                "--plan",
                str(plan),
                "--expected-run-id",
                "25576382818",
            )

        self.assertEqual(result.returncode, 0, result.stderr)
        report = json.loads(result.stdout)
        row = report["rows"][0]
        self.assertEqual(row["timeout_sec"], 7200)
        self.assertEqual(row["per_invocation_timeout_sec"], 7200)
        self.assertEqual(row["benchmark_job_timeout_sec"], 21600)

    def test_cancelled_job_does_not_relabel_missing_sizes(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            artifact_root, plan = self.make_fixture(
                Path(tmp),
                job_conclusion="cancelled",
                step_conclusion="cancelled",
                result_rows=[],
            )
            result = self.run_cli(
                "--artifact-root",
                str(artifact_root),
                "--plan",
                str(plan),
                "--expected-run-id",
                "25576382818",
            )

        self.assertEqual(result.returncode, 0, result.stderr)
        report = json.loads(result.stdout)
        row = report["rows"][0]
        self.assertEqual(row["terminal_classification"], "cancelled")
        self.assertEqual(row["evidence_classification"], "no-result")
        self.assertEqual(
            report["status_counts"],
            {"cancelled": 0, "fail": 0, "no-result": 2, "pass": 0, "timeout": 0},
        )
        self.assertEqual(
            report["classification_counts"],
            {"cancelled": 0, "failure": 0, "no-result": 2, "pass": 0},
        )
        self.assertEqual(
            [entry["status"] for entry in row["entries"]],
            ["no-result", "no-result"],
        )
        self.assertEqual(
            [entry["classification"] for entry in row["entries"]],
            ["no-result", "no-result"],
        )
        self.assertTrue(
            all(entry["provenance"]["source"] == "missing_result_row" for entry in row["entries"])
        )

    def test_explicit_per_size_markers_classify_failure_and_cancelled(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            artifact_root, plan = self.make_fixture(
                Path(tmp),
                job_conclusion="success",
                step_conclusion="success",
                result_extra_columns=("finishability_status",),
                result_rows=[
                    ["fixture_kernel", "small", 1, "0.1", "0.1", "0.1", "failed"],
                    ["fixture_kernel", "large", 1, "0.2", "0.2", "0.2", "cancelled"],
                ],
            )
            result = self.run_cli(
                "--artifact-root",
                str(artifact_root),
                "--plan",
                str(plan),
                "--expected-run-id",
                "25576382818",
            )

        self.assertEqual(result.returncode, 0, result.stderr)
        report = json.loads(result.stdout)
        row = report["rows"][0]
        self.assertEqual(row["terminal_classification"], "pass")
        self.assertEqual(row["evidence_classification"], "cancelled")
        self.assertEqual(
            report["status_counts"],
            {"cancelled": 1, "fail": 1, "no-result": 0, "pass": 0, "timeout": 0},
        )
        self.assertEqual(
            report["classification_counts"],
            {"cancelled": 1, "failure": 1, "no-result": 0, "pass": 0},
        )
        entries = row["entries"]
        self.assertEqual([entry["status"] for entry in entries], ["fail", "cancelled"])
        self.assertEqual([entry["classification"] for entry in entries], ["failure", "cancelled"])
        self.assertEqual(entries[0]["explicit_status_values"], ["failed"])
        self.assertEqual(entries[1]["explicit_status_values"], ["cancelled"])
        self.assertIn("explicit per-size failure marker", entries[0]["basis"])
        self.assertIn("explicit per-size cancelled marker", entries[1]["basis"])

    def test_rejects_non_terminal_run_24959959195(self):
        with tempfile.TemporaryDirectory() as tmp:
            artifact_root, plan = self.make_fixture(
                Path(tmp),
                run_status="queued",
                non_terminal=True,
            )
            result = self.run_cli(
                "--artifact-root",
                str(artifact_root),
                "--plan",
                str(plan),
                "--expected-run-id",
                "24959959195",
            )

        self.assertEqual(result.returncode, 2)
        self.assertIn("FAIL: MI300A regular finishability evidence rejected input", result.stderr)
        self.assertIn("24959959195", result.stderr)
        self.assertIn("not terminal", result.stderr)
        self.assertEqual(result.stdout, "")


if __name__ == "__main__":
    unittest.main()
