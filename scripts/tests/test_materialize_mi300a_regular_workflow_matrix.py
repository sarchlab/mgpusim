from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
MATERIALIZER = REPO_ROOT / "scripts" / "materialize_mi300a_regular_workflow_matrix.py"
PLAN = REPO_ROOT / "benchmark-comparison" / "mi300a_problem_size_discovery_plan.json"
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


class MaterializeMI300ARegularWorkflowMatrixTest(unittest.TestCase):
    def setUp(self) -> None:
        (REPO_ROOT / ".repo_tmp").mkdir(exist_ok=True)

    def test_matrix_rows_include_informational_runtime_budget_fields(self) -> None:
        with tempfile.TemporaryDirectory(dir=REPO_ROOT / ".repo_tmp") as tmp:
            tmp_path = Path(tmp)
            summary_path = tmp_path / "summary.md"
            report_path = tmp_path / "regular_matrix_validation.json"
            github_output_path = tmp_path / "github_output.txt"

            result = subprocess.run(
                [
                    sys.executable,
                    str(MATERIALIZER),
                    "--summary",
                    str(summary_path),
                    "--report",
                    str(report_path),
                    "--github-output",
                    str(github_output_path),
                ],
                cwd=REPO_ROOT,
                capture_output=True,
                text=True,
                timeout=30,
                check=False,
            )

            self.assertEqual(result.returncode, 0, msg=result.stderr)
            stdout_report = json.loads(result.stdout)
            file_report = json.loads(report_path.read_text(encoding="utf-8"))
            outputs = self._read_github_outputs(github_output_path)
            tier1_rows = json.loads(outputs["tier1_matrix"])["include"]
            tier2_rows = json.loads(outputs["tier2_matrix"])["include"]
            summary = summary_path.read_text(encoding="utf-8")

        self.assertEqual(file_report, stdout_report)
        self.assertEqual(stdout_report["runtime_budget_notice"], RUNTIME_BUDGET_NOTICE)
        self.assertIn("Rows over 6h budget", summary)
        self.assertIn(RUNTIME_BUDGET_NOTICE, summary)
        self.assertIn("not a hard pass/fail gate", summary)
        self.assertIn("not a simulator outcome", summary)
        self.assertEqual(outputs["tier1_count"], "19")
        self.assertEqual(outputs["tier2_count"], "63")
        self.assertEqual(outputs["tier1_sizes"], "308")
        self.assertEqual(outputs["tier2_sizes"], "1108")

        for rows in (tier1_rows, tier2_rows):
            for row in rows:
                invocation_count = int(row["expected_plan_entries"])
                self.assertRuntimeBudgetFields(row, invocation_count=invocation_count)

        first_tier1 = tier1_rows[0]
        self.assertEqual(first_tier1["benchmark"], "mem_latency_chase")
        self.assertEqual(first_tier1["expected_plan_entries"], 16)
        self.assertRuntimeBudgetFields(first_tier1, invocation_count=16)

        gelu = next(row for row in tier2_rows if row["benchmark"] == "gelu")
        self.assertEqual(gelu["expected_plan_entries"], 1)
        self.assertRuntimeBudgetFields(gelu, invocation_count=1)
        self.assertFalse(gelu["exceeds_benchmark_job_timeout"])

        tier1_report_row = stdout_report["matrix"]["rows"]["benchmark-tier1"][0]
        self.assertEqual(tier1_report_row["benchmark"], "mem_latency_chase")
        self.assertRuntimeBudgetFields(tier1_report_row, invocation_count=16, compact=True)
        tier2_report_row = next(
            row
            for row in stdout_report["matrix"]["rows"]["benchmark-tier2"]
            if row["benchmark"] == "gelu"
        )
        self.assertRuntimeBudgetFields(tier2_report_row, invocation_count=1, compact=True)
        tier1_risk = stdout_report["matrix"]["jobs"]["benchmark-tier1"]["passability_risk"]
        tier2_risk = stdout_report["matrix"]["jobs"]["benchmark-tier2"]["passability_risk"]
        self.assertEqual(tier1_risk["rows_exceeding_benchmark_job_timeout"], 19)
        self.assertEqual(tier2_risk["planned_invocations_over_job_timeout_capacity"], 921)

    def assertRuntimeBudgetFields(
        self,
        row: dict[str, object],
        *,
        invocation_count: int,
        compact: bool = False,
    ) -> None:
        self.assertLessEqual(RUNTIME_BUDGET_FIELDS, set(row))
        expected_budget = invocation_count * 7200
        if compact:
            self.assertEqual(row["plan_entries"], invocation_count)
        else:
            self.assertEqual(row["expected_plan_entries"], invocation_count)
        self.assertEqual(row["timeout_sec"], 7200)
        self.assertEqual(row["expected_simulator_invocations"], invocation_count)
        self.assertEqual(row["planned_invocation_count"], invocation_count)
        self.assertEqual(row["per_invocation_timeout_sec"], 7200)
        self.assertEqual(row["row_timeout_budget_sec"], expected_budget)
        self.assertEqual(row["benchmark_job_timeout_sec"], 21600)
        self.assertEqual(
            row["row_timeout_budget_to_job_timeout_ratio"],
            round(expected_budget / 21600, 6),
        )
        self.assertEqual(row["exceeds_benchmark_job_timeout"], expected_budget > 21600)
        self.assertEqual(row["max_invocations_within_job_timeout"], 3)

    def test_materializer_rejects_non_integer_plan_top_level_timeout(self) -> None:
        with tempfile.TemporaryDirectory(dir=REPO_ROOT / ".repo_tmp") as tmp:
            tmp_path = Path(tmp)
            stale_plan_path = tmp_path / "stale_plan.json"
            plan = json.loads(PLAN.read_text(encoding="utf-8"))
            plan["timeout_sec"] = "not-an-integer"
            stale_plan_path.write_text(json.dumps(plan), encoding="utf-8")

            result = subprocess.run(
                [
                    sys.executable,
                    str(MATERIALIZER),
                    "--plan",
                    str(stale_plan_path),
                    "--summary",
                    str(tmp_path / "summary.md"),
                    "--report",
                    str(tmp_path / "report.json"),
                ],
                cwd=REPO_ROOT,
                capture_output=True,
                text=True,
                timeout=30,
                check=False,
            )

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("plan.timeout_sec", result.stderr)

    def test_materializer_rejects_non_integer_plan_entry_timeout(self) -> None:
        with tempfile.TemporaryDirectory(dir=REPO_ROOT / ".repo_tmp") as tmp:
            tmp_path = Path(tmp)
            stale_plan_path = tmp_path / "stale_plan.json"
            plan = json.loads(PLAN.read_text(encoding="utf-8"))
            plan["entries"][0]["timeout_sec"] = "not-an-integer"
            stale_plan_path.write_text(json.dumps(plan), encoding="utf-8")

            result = subprocess.run(
                [
                    sys.executable,
                    str(MATERIALIZER),
                    "--plan",
                    str(stale_plan_path),
                    "--summary",
                    str(tmp_path / "summary.md"),
                    "--report",
                    str(tmp_path / "report.json"),
                ],
                cwd=REPO_ROOT,
                capture_output=True,
                text=True,
                timeout=30,
                check=False,
            )

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("plan.entries[0].timeout_sec", result.stderr)

    def _read_github_outputs(self, path: Path) -> dict[str, str]:
        outputs: dict[str, str] = {}
        for line in path.read_text(encoding="utf-8").splitlines():
            key, value = line.split("=", 1)
            outputs[key] = value
        return outputs


if __name__ == "__main__":
    unittest.main()
