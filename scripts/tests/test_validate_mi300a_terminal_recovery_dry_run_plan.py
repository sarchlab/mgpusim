from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from typing import Any, Mapping


REPO_ROOT = Path(__file__).resolve().parents[2]
VALIDATOR = REPO_ROOT / "scripts" / "validate_mi300a_terminal_recovery_dry_run_plan.py"
CHECKED_IN_PLAN = (
    REPO_ROOT
    / "benchmark-comparison"
    / "generated"
    / "mi300a_terminal_discovery_run_25111815292_recovery_dry_run_plan.json"
)
CHECKED_IN_MANIFEST = (
    REPO_ROOT
    / "benchmark-comparison"
    / "generated"
    / "mi300a_terminal_discovery_run_25111815292_recovery_manifest.json"
)
TMP_ROOT = REPO_ROOT / ".repo_tmp" / "mi300a_terminal_recovery_dry_run_validation_tests"


class ValidateMI300ATerminalRecoveryDryRunPlanTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        TMP_ROOT.mkdir(parents=True, exist_ok=True)

    def run_validator(self, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [sys.executable, str(VALIDATOR), *args],
            cwd=REPO_ROOT,
            text=True,
            capture_output=True,
            check=False,
            timeout=30,
        )

    def load_json(self, path: Path) -> dict[str, Any]:
        with path.open("r", encoding="utf-8") as f:
            data = json.load(f)
        self.assertIsInstance(data, dict)
        return data

    def write_json(self, path: Path, data: Mapping[str, Any]) -> None:
        path.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")

    def test_checked_in_plan_passes_with_stable_json_report(self) -> None:
        result = self.run_validator()

        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertEqual(result.stderr, "")
        report = json.loads(result.stdout)
        self.assertEqual(
            report["schema_name"],
            "mgpusim.mi300a_terminal_discovery_recovery_dry_run_validation",
        )
        self.assertEqual(report["schema_version"], 1)
        self.assertEqual(report["validation_status"], "pass")
        self.assertTrue(report["validated"])
        self.assertTrue(report["dry_run"])
        self.assertTrue(report["pre_dispatch_static_validation"])
        self.assertEqual(
            report["plan"],
            "benchmark-comparison/generated/mi300a_terminal_discovery_run_25111815292_recovery_dry_run_plan.json",
        )
        self.assertEqual(
            report["recovery_manifest"],
            "benchmark-comparison/generated/mi300a_terminal_discovery_run_25111815292_recovery_manifest.json",
        )
        self.assertEqual(report["source_run_id"], "25111815292")
        self.assertEqual(report["source_run_conclusion"], "cancelled")
        self.assertTrue(report["rederived_plan_matches_checked_in"])
        self.assertTrue(report["canonical_plan_matches_checked_in"])

        coverage = report["coverage"]
        self.assertEqual(coverage["source_plan_index_count"], 1416)
        self.assertEqual(coverage["missing_plan_index_count"], 940)
        self.assertEqual(coverage["dry_run_plan_index_count"], 940)
        self.assertEqual(coverage["dry_run_unique_plan_index_count"], 940)
        self.assertEqual(coverage["prior_observed_plan_index_count"], 476)
        self.assertEqual(coverage["prior_observed_unique_plan_index_count"], 476)
        self.assertEqual(coverage["duplicate_dry_run_plan_index_count"], 0)
        self.assertEqual(coverage["extra_dry_run_plan_index_count"], 0)
        self.assertEqual(coverage["missing_dry_run_plan_index_count"], 0)
        self.assertEqual(coverage["prior_observed_overlap_count"], 0)
        self.assertEqual(coverage["represented_plan_index_count_after_union"], 1416)
        self.assertTrue(coverage["dry_run_matches_manifest_missing_scope"])
        self.assertTrue(coverage["prior_observed_excluded_from_recovery"])
        self.assertEqual(coverage["prior_observed_treatment"], "incomplete_prior_evidence_only")
        self.assertTrue(coverage["not_complete_terminal_provenance"])
        self.assertEqual(coverage["dry_run_plan_index_ranges"][0], "9-16")
        self.assertEqual(coverage["dry_run_plan_index_ranges"][-1], "1393-1401")
        self.assertEqual(coverage["prior_observed_plan_index_ranges"][0], "1-8")
        self.assertEqual(coverage["prior_observed_plan_index_ranges"][-1], "1402-1416")
        self.assertEqual(
            [row["benchmark"] for row in coverage["complete_prior_rows_excluded_from_recovery"]],
            ["gelu", "sssp", "dmr"],
        )

        budget = report["budget_contract"]
        self.assertEqual(budget["max_parallel"], 14)
        self.assertEqual(budget["attempts_per_matrix_row"], 1)
        self.assertEqual(budget["per_attempt_timeout_sec"], 600)
        self.assertEqual(budget["benchmark_run_timeout_sec"], 600)
        self.assertEqual(budget["row_timeout_sec"], 600)
        self.assertEqual(budget["row_required_timeout_sec"], 600)
        self.assertEqual(budget["action_timeout_sec"], 3600)
        self.assertEqual(budget["max_parallel_waves_per_batch"], 6)
        self.assertEqual(budget["max_rows_per_batch"], 84)
        self.assertEqual(budget["row_violation_count"], 0)
        self.assertEqual(budget["batch_violation_count"], 0)
        self.assertEqual(budget["max_batch_required_timeout_sec"], 3600)
        self.assertEqual(budget["min_action_slack_sec"], 0)
        self.assertEqual(budget["recovery_batch_count"], 12)
        self.assertEqual(budget["batch_row_counts"], [84] * 11 + [16])
        self.assertEqual(budget["batch_required_timeout_sec"], [3600] * 11 + [1200])
        self.assertTrue(budget["all_batches_fit_action_timeout"])
        self.assertTrue(budget["all_batches_max_parallel_at_most_14"])
        self.assertIn("no workflow dispatch/cancel/rerun", report["non_goals"])
        self.assertIn(
            "no provenance, finishable manifest, or Tier artifact regeneration",
            report["non_goals"],
        )

    def test_rejects_empty_batch_ranges_without_traceback(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            plan_path = Path(tmpdir) / "empty_batch_recovery_dry_run_plan.json"
            plan = self.load_json(CHECKED_IN_PLAN)
            first_batch = plan["batches"][0]
            first_batch["plan_index_ranges"] = []
            first_batch["row_count"] = 0
            first_batch["attempt_count"] = 0
            first_batch["per_size_attempt_count"] = 0
            self.write_json(plan_path, plan)
            result = self.run_validator("--dry-run-plan", str(plan_path))

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("FAIL: MI300A recovery dry-run static validation rejected the plan.", result.stderr)
        self.assertIn("plan.batches[0].plan_index_ranges", result.stderr)
        self.assertIn("at least one recovery plan index", result.stderr)
        self.assertNotIn("Traceback", result.stderr)
        self.assertNotIn("IndexError", result.stderr)

    def test_rejects_impossible_zero_row_batch_without_traceback(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            plan_path = Path(tmpdir) / "zero_row_recovery_dry_run_plan.json"
            plan = self.load_json(CHECKED_IN_PLAN)
            first_batch = plan["batches"][0]
            first_batch["row_count"] = 0
            first_batch["attempt_count"] = 0
            first_batch["per_size_attempt_count"] = 0
            self.write_json(plan_path, plan)
            result = self.run_validator("--dry-run-plan", str(plan_path))

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("FAIL: MI300A recovery dry-run static validation rejected the plan.", result.stderr)
        self.assertIn("plan.batches[0].row_count must be 84, got 0", result.stderr)
        self.assertNotIn("Traceback", result.stderr)
        self.assertNotIn("IndexError", result.stderr)

    def test_rejects_duplicate_dry_run_plan_index(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            plan_path = Path(tmpdir) / "duplicate_recovery_dry_run_plan.json"
            plan = self.load_json(CHECKED_IN_PLAN)
            plan["batches"][0]["plan_index_ranges"].append("9")
            self.write_json(plan_path, plan)
            result = self.run_validator("--dry-run-plan", str(plan_path))

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("duplicates plan_index 9", result.stderr)

    def test_rejects_prior_observed_overlap_as_recovery_scope(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            plan_path = Path(tmpdir) / "prior_overlap_recovery_dry_run_plan.json"
            plan = self.load_json(CHECKED_IN_PLAN)
            plan["coverage"]["dry_run_plan_index_ranges"][0] = "1-8"
            plan["batches"][0]["plan_index_ranges"][0] = "1-8"
            plan["batches"][0]["plan_index_start"] = 1
            self.write_json(plan_path, plan)
            result = self.run_validator("--dry-run-plan", str(plan_path))

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("overlaps prior observed incomplete evidence", result.stderr)
        self.assertIn("1-8", result.stderr)

    def test_rejects_budget_contract_regression_before_dispatch(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            plan_path = Path(tmpdir) / "unsafe_budget_recovery_dry_run_plan.json"
            plan = self.load_json(CHECKED_IN_PLAN)
            plan["operator_contract"]["max_parallel"] = 15
            self.write_json(plan_path, plan)
            result = self.run_validator("--dry-run-plan", str(plan_path))

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("plan.operator_contract.max_parallel must be 14", result.stderr)

    def test_rejects_prior_observed_completion_claim(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            plan_path = Path(tmpdir) / "prior_completion_claim_recovery_dry_run_plan.json"
            plan = self.load_json(CHECKED_IN_PLAN)
            plan["prior_observed_treatment"] = "complete_terminal_provenance"
            self.write_json(plan_path, plan)
            result = self.run_validator("--dry-run-plan", str(plan_path))

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("plan.prior_observed_treatment", result.stderr)
        self.assertIn("incomplete_prior_evidence_only", result.stderr)


if __name__ == "__main__":
    unittest.main()
