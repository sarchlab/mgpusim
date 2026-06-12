from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
VALIDATOR = REPO_ROOT / "scripts" / "validate_mi300a_regular_workflow_matrix.py"
PLAN = REPO_ROOT / "benchmark-comparison" / "mi300a_problem_size_discovery_plan.json"
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "benchmark.yml"
RUNTIME_BUDGET_FIELDS = {
    "expected_simulator_invocations",
    "planned_invocation_count",
    "per_invocation_timeout_sec",
    "row_timeout_budget_sec",
    "benchmark_job_timeout_sec",
    "row_timeout_budget_to_job_timeout_ratio",
    "exceeds_benchmark_job_timeout",
    "max_invocations_within_job_timeout",
}
RUNTIME_BUDGET_NOTICE = (
    "Runtime-budget/passability-risk visibility is informational only; it is not "
    "a hard pass/fail gate and is not a simulator outcome."
)


class ValidateMI300ARegularWorkflowMatrixTest(unittest.TestCase):
    def setUp(self) -> None:
        (REPO_ROOT / ".repo_tmp").mkdir(exist_ok=True)

    def test_validator_reports_full_plan_grouping_and_workflow_contract(self) -> None:
        with tempfile.TemporaryDirectory(dir=REPO_ROOT / ".repo_tmp") as tmp:
            report_path = Path(tmp) / "regular_workflow_contract_validation.json"
            result = self._run_validator("--report", str(report_path))

            self.assertEqual(result.returncode, 0, msg=result.stderr)
            stdout_report = json.loads(result.stdout)
            file_report = json.loads(report_path.read_text(encoding="utf-8"))
            self.assertEqual(file_report, stdout_report)

        self.assertEqual(stdout_report["schema_name"], "mgpusim.mi300a_regular_workflow_matrix_contract_validation")
        self.assertEqual(stdout_report["status"], "pass")
        self.assertEqual(stdout_report["runtime_budget_notice"], RUNTIME_BUDGET_NOTICE)
        self.assertIn("informational only", stdout_report["runtime_budget_notice"])
        self.assertIn("not a hard pass/fail gate", stdout_report["runtime_budget_notice"])
        self.assertIn("not a simulator outcome", stdout_report["runtime_budget_notice"])
        self.assertEqual(stdout_report["source_plan"], "benchmark-comparison/mi300a_problem_size_discovery_plan.json")
        self.assertEqual(stdout_report["plan_entry_count"], 1416)
        self.assertEqual(stdout_report["workflow_benchmark_count"], 82)

        matrix = stdout_report["matrix"]
        self.assertTrue(matrix["per_benchmark_grouping"])
        self.assertEqual(matrix["total_plan_entries"], 1416)
        self.assertEqual(matrix["total_matrix_rows"], 82)
        self.assertEqual(matrix["unique_benchmark_rows"], 82)
        self.assertEqual(matrix["unique_artifact_ids"], 82)
        self.assertEqual(matrix["jobs"]["benchmark-tier1"]["matrix_rows"], 19)
        self.assertEqual(matrix["jobs"]["benchmark-tier1"]["plan_entries"], 308)
        self.assertEqual(matrix["jobs"]["benchmark-tier1"]["source_plan_index_ranges"], ["1-308"])
        self.assertEqual(
            matrix["jobs"]["benchmark-tier1"]["passability_risk"],
            {
                "benchmark_job_timeout_sec": 21600,
                "informational_only": True,
                "max_invocations_within_job_timeout": 3,
                "max_planned_invocation_count": 19,
                "max_row_timeout_budget_sec": 136800,
                "max_row_timeout_budget_to_job_timeout_ratio": 6.333333,
                "per_invocation_timeout_sec": 7200,
                "planned_invocations": 308,
                "planned_invocations_over_job_timeout_capacity": 251,
                "rows_exceeding_benchmark_job_timeout": 19,
                "rows_within_benchmark_job_timeout": 0,
            },
        )
        self.assertEqual(matrix["jobs"]["benchmark-tier2"]["matrix_rows"], 63)
        self.assertEqual(matrix["jobs"]["benchmark-tier2"]["plan_entries"], 1108)
        self.assertEqual(matrix["jobs"]["benchmark-tier2"]["source_plan_index_ranges"], ["309-1416"])
        tier2_risk = matrix["jobs"]["benchmark-tier2"]["passability_risk"]
        self.assertEqual(tier2_risk["rows_exceeding_benchmark_job_timeout"], 62)
        self.assertEqual(tier2_risk["rows_within_benchmark_job_timeout"], 1)
        self.assertEqual(tier2_risk["max_planned_invocation_count"], 54)
        self.assertEqual(matrix["rows"]["benchmark-tier1"][0]["benchmark"], "mem_latency_chase")
        self.assertEqual(matrix["rows"]["benchmark-tier2"][-1]["benchmark"], "dmr")
        for row in matrix["rows"]["benchmark-tier1"] + matrix["rows"]["benchmark-tier2"]:
            self.assertLessEqual(RUNTIME_BUDGET_FIELDS, set(row))
            expected_budget = row["plan_entries"] * 7200
            self.assertEqual(row["timeout_sec"], 7200)
            self.assertEqual(row["size_count"], row["plan_entries"])
            self.assertEqual(row["expected_simulator_invocations"], row["plan_entries"])
            self.assertEqual(row["planned_invocation_count"], row["plan_entries"])
            self.assertEqual(row["per_invocation_timeout_sec"], 7200)
            self.assertEqual(row["row_timeout_budget_sec"], expected_budget)
            self.assertEqual(row["benchmark_job_timeout_sec"], 21600)
            self.assertAlmostEqual(
                row["row_timeout_budget_to_job_timeout_ratio"],
                round(expected_budget / 21600, 6),
            )
            self.assertEqual(row["max_invocations_within_job_timeout"], 3)
            self.assertEqual(row["exceeds_benchmark_job_timeout"], expected_budget > 21600)

        self.assertEqual(
            stdout_report["regular_runtime_contract"],
            {
                "benchmark_job_timeout_minutes": 360,
                "benchmark_job_timeout_sec": 21600,
                "max_invocations_within_job_timeout": 3,
                "per_simulator_timeout_sec": 7200,
                "row_budget_risk_is_informational": True,
                "strategy_max_parallel": 14,
            },
        )
        workflow = stdout_report["workflow"]
        self.assertEqual(workflow["workflow_events"], ["workflow_dispatch"])
        self.assertEqual(workflow["workflow_dispatch_inputs"], ["ref"])
        self.assertEqual(workflow["jobs"], ["validation", "benchmark-tier1", "tier1-summary", "benchmark-tier2", "summary"])
        self.assertTrue(workflow["tier2_runs_after_validation_for_full_coverage"])
        self.assertTrue(workflow["forbidden_surface_absent"])
        self.assertTrue(workflow["legacy_generated_matrix_sources_absent"])
        expected_tier_contract = {"strategy_max_parallel": 14, "timeout_minutes": 360, "timeout_sec": 21600}
        self.assertEqual(workflow["benchmark_tier_contract"]["benchmark-tier1"], expected_tier_contract)
        self.assertEqual(workflow["benchmark_tier_contract"]["benchmark-tier2"], expected_tier_contract)
        self.assertEqual(
            stdout_report["workflow_files"]["workflow_files"],
            ["benchmark.yml", "compile_hsaco.yml", "mgpusim_test.yml"],
        )
        self.assertEqual(stdout_report["workflow_files"]["terminal_discovery_workflow_files"], [])
        for snippet in [
            "no terminal-discovery workflow or benchmark dispatch mode",
            "no workflow dispatch/cancel/rerun",
            "no finishable/provenance/Tier regeneration",
            "no terminal finishability completion claim",
        ]:
            self.assertIn(snippet, stdout_report["non_goals"])

    def test_validator_rejects_incomplete_plan_coverage(self) -> None:
        with tempfile.TemporaryDirectory(dir=REPO_ROOT / ".repo_tmp") as tmp:
            plan = json.loads(PLAN.read_text(encoding="utf-8"))
            plan["entries"] = plan["entries"][:-1]
            plan_path = Path(tmp) / "plan_missing_one.json"
            plan_path.write_text(json.dumps(plan, sort_keys=True), encoding="utf-8")

            result = self._run_validator("--plan", str(plan_path))

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("plan.entries must contain 1416 entries", result.stderr)

    def test_validator_rejects_wrong_benchmark_parallelism(self) -> None:
        with tempfile.TemporaryDirectory(dir=REPO_ROOT / ".repo_tmp") as tmp:
            workflow_text = WORKFLOW.read_text(encoding="utf-8")
            # Structurally mutate the real YAML key while leaving the expected
            # snippet only in a comment to confirm the validator rejects
            # comment-only conformance.
            workflow_text = workflow_text.replace(
                "      max-parallel: 14",
                "      max-parallel: 6  # max-parallel: 14 (comment only)",
                1,
            )
            workflow_path = Path(tmp) / "benchmark.yml"
            workflow_path.write_text(workflow_text, encoding="utf-8")

            result = self._run_validator("--workflow", str(workflow_path))

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("max-parallel: 14", result.stderr)
        self.assertIn("benchmark-tier1", result.stderr)

    def test_validator_rejects_wrong_benchmark_job_budget(self) -> None:
        with tempfile.TemporaryDirectory(dir=REPO_ROOT / ".repo_tmp") as tmp:
            workflow_text = WORKFLOW.read_text(encoding="utf-8")
            # Structurally mutate the real YAML key while leaving the expected
            # snippet only in a comment to confirm the validator rejects
            # comment-only conformance.
            workflow_text = workflow_text.replace(
                "    timeout-minutes: 360",
                "    timeout-minutes: 60  # timeout-minutes: 360 (comment only)",
                1,
            )
            workflow_path = Path(tmp) / "benchmark.yml"
            workflow_path.write_text(workflow_text, encoding="utf-8")

            result = self._run_validator("--workflow", str(workflow_path))

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("timeout-minutes: 360", result.stderr)
        self.assertIn("benchmark-tier1", result.stderr)

    def test_validator_rejects_stale_720_minute_job_budget(self) -> None:
        with tempfile.TemporaryDirectory(dir=REPO_ROOT / ".repo_tmp") as tmp:
            workflow_text = WORKFLOW.read_text(encoding="utf-8")
            workflow_text = workflow_text.replace(
                "    timeout-minutes: 360",
                "    timeout-minutes: 720",
                1,
            )
            workflow_path = Path(tmp) / "benchmark.yml"
            workflow_path.write_text(workflow_text, encoding="utf-8")

            result = self._run_validator("--workflow", str(workflow_path))

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("timeout-minutes: 720", result.stderr)

    def test_validator_rejects_timeout_minutes_only_in_comment(self) -> None:
        with tempfile.TemporaryDirectory(dir=REPO_ROOT / ".repo_tmp") as tmp:
            workflow_text = WORKFLOW.read_text(encoding="utf-8")
            # Replace the real timeout-minutes for both benchmark tier jobs and
            # add a comment containing the expected snippet so the prior raw
            # substring check would erroneously accept the workflow.
            mutated = workflow_text.replace(
                "    timeout-minutes: 360",
                "    timeout-minutes: 60\n    # timeout-minutes: 360",
            )
            self.assertNotIn("    timeout-minutes: 360\n", mutated)
            self.assertIn("# timeout-minutes: 360", mutated)
            workflow_path = Path(tmp) / "benchmark.yml"
            workflow_path.write_text(mutated, encoding="utf-8")

            result = self._run_validator("--workflow", str(workflow_path))

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("timeout-minutes: 360", result.stderr)
        self.assertIn("benchmark-tier", result.stderr)

    def test_validator_rejects_max_parallel_only_in_comment(self) -> None:
        with tempfile.TemporaryDirectory(dir=REPO_ROOT / ".repo_tmp") as tmp:
            workflow_text = WORKFLOW.read_text(encoding="utf-8")
            mutated = workflow_text.replace(
                "      max-parallel: 14",
                "      max-parallel: 1\n      # max-parallel: 14",
            )
            self.assertNotIn("      max-parallel: 14\n", mutated)
            self.assertIn("# max-parallel: 14", mutated)
            workflow_path = Path(tmp) / "benchmark.yml"
            workflow_path.write_text(mutated, encoding="utf-8")

            result = self._run_validator("--workflow", str(workflow_path))

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("max-parallel: 14", result.stderr)
        self.assertIn("benchmark-tier", result.stderr)

    def test_validator_rejects_stale_simulator_invocation_budget(self) -> None:
        with tempfile.TemporaryDirectory(dir=REPO_ROOT / ".repo_tmp") as tmp:
            workflow_text = WORKFLOW.read_text(encoding="utf-8")
            workflow_text = workflow_text.replace("matrix.timeout_sec || '7200'", "matrix.timeout_sec || '3600'", 1)
            workflow_path = Path(tmp) / "benchmark.yml"
            workflow_path.write_text(workflow_text, encoding="utf-8")

            result = self._run_validator("--workflow", str(workflow_path))

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("forbidden snippet", result.stderr)
        self.assertIn("matrix.timeout_sec || '3600'", result.stderr)

    def test_validator_rejects_extra_dispatch_surface(self) -> None:
        with tempfile.TemporaryDirectory(dir=REPO_ROOT / ".repo_tmp") as tmp:
            workflow_text = WORKFLOW.read_text(encoding="utf-8") + "\n# forbidden regression\nrepository_dispatch:\n"
            workflow_path = Path(tmp) / "benchmark.yml"
            workflow_path.write_text(workflow_text, encoding="utf-8")

            result = self._run_validator("--workflow", str(workflow_path))

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("forbidden snippet", result.stderr)
        self.assertIn("repository_dispatch", result.stderr)


    def test_validator_rejects_extra_benchmark_workflow_trigger(self) -> None:
        with tempfile.TemporaryDirectory(dir=REPO_ROOT / ".repo_tmp") as tmp:
            workflow_text = WORKFLOW.read_text(encoding="utf-8")
            workflow_text = workflow_text.replace("on:\n  workflow_dispatch:", "on:\n  push:\n  workflow_dispatch:", 1)
            workflow_path = Path(tmp) / "benchmark.yml"
            workflow_path.write_text(workflow_text, encoding="utf-8")

            result = self._run_validator("--workflow", str(workflow_path))

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("benchmark workflow events must be ['workflow_dispatch']", result.stderr)
        self.assertIn("push", result.stderr)

    def test_validator_rejects_extra_workflow_file_surface(self) -> None:
        with tempfile.TemporaryDirectory(dir=REPO_ROOT / ".repo_tmp") as tmp:
            workflows_dir = Path(tmp) / ".github" / "workflows"
            workflows_dir.mkdir(parents=True)
            for name in ("benchmark.yml", "compile_hsaco.yml", "mgpusim_test.yml"):
                (workflows_dir / name).write_text((WORKFLOW.parent / name).read_text(encoding="utf-8"), encoding="utf-8")
            (workflows_dir / "mi300a_terminal_discovery.yaml").write_text("name: forbidden\non:\n  workflow_dispatch:\n", encoding="utf-8")

            result = self._run_validator(
                "--workflow",
                str(workflows_dir / "benchmark.yml"),
                "--workflows-dir",
                str(workflows_dir),
            )

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("forbidden terminal-discovery/recovery workflow files present", result.stderr)
        self.assertIn("mi300a_terminal_discovery.yaml", result.stderr)

    def test_validator_rejects_non_integer_plan_timeout_metadata(self) -> None:
        with tempfile.TemporaryDirectory(dir=REPO_ROOT / ".repo_tmp") as tmp:
            tmp_path = Path(tmp)
            stale_plan_path = tmp_path / "stale_plan.json"
            plan = json.loads(PLAN.read_text(encoding="utf-8"))
            plan["timeout_sec"] = "not-an-integer"
            for entry in plan["entries"]:
                entry["timeout_sec"] = "not-an-integer"
            stale_plan_path.write_text(json.dumps(plan), encoding="utf-8")

            result = self._run_validator("--plan", str(stale_plan_path))

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("timeout_sec", result.stderr)

    def _run_validator(self, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [sys.executable, str(VALIDATOR), *args],
            cwd=REPO_ROOT,
            capture_output=True,
            text=True,
            timeout=30,
            check=False,
        )


if __name__ == "__main__":
    unittest.main()
