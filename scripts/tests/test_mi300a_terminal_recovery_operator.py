from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from contextlib import redirect_stderr, redirect_stdout
from io import StringIO
from pathlib import Path
from unittest import mock
from typing import Any, Mapping


REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPTS_DIR = REPO_ROOT / "scripts"
if str(SCRIPTS_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPTS_DIR))

import mi300a_terminal_recovery_operator as operator


SCRIPT = SCRIPTS_DIR / "mi300a_terminal_recovery_operator.py"
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
TMP_ROOT = REPO_ROOT / ".repo_tmp" / "mi300a_terminal_recovery_operator_tests"
CONTRACT_CAP_FAILURE_CASES = (
    ("--max-parallel", "15", "max_parallel", 15, 14),
    ("--per-attempt-timeout-sec", "601", "per_attempt_timeout_sec", 601, 600),
    ("--row-timeout-sec", "601", "row_timeout_sec", 601, 600),
    ("--action-timeout-sec", "3601", "action_timeout_sec", 3601, 3600),
)
REGENERATION_COMMAND_REFUSAL_CASES = (
    "collect-provenance",
    "regenerate-provenance",
    "regenerate-finishable",
    "regenerate-tier",
)
EXPECTED_FORBIDDEN_SIDE_EFFECTS = [
    "workflow_dispatch",
    "workflow_cancel",
    "workflow_rerun",
    "simulator_execution",
    "calibration_start",
    "provenance_regeneration",
    "finishable_manifest_regeneration",
    "tier_artifact_regeneration",
    "github_workflow_file_modification",
]


class MI300ATerminalRecoveryOperatorTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        TMP_ROOT.mkdir(parents=True, exist_ok=True)

    def run_operator(self, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [sys.executable, str(SCRIPT), *args],
            cwd=REPO_ROOT,
            text=True,
            capture_output=True,
            check=False,
            timeout=30,
        )

    def run_operator_in_process(self, *args: str) -> tuple[int, str, str]:
        stdout = StringIO()
        stderr = StringIO()
        with redirect_stdout(stdout), redirect_stderr(stderr):
            returncode = operator.main(list(args))
        return returncode, stdout.getvalue(), stderr.getvalue()

    def git_output(self, *args: str) -> str:
        result = subprocess.run(
            ["git", "-C", str(REPO_ROOT), *args],
            text=True,
            capture_output=True,
            check=False,
            timeout=10,
        )
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        value = result.stdout.strip()
        self.assertTrue(value)
        return value

    def current_git_branch(self) -> str:
        return self.git_output("branch", "--show-current")

    def current_git_head_sha(self) -> str:
        return self.git_output("rev-parse", "--verify", "HEAD").lower()

    def load_json(self, path: Path) -> dict[str, Any]:
        with path.open("r", encoding="utf-8") as f:
            data = json.load(f)
        self.assertIsInstance(data, dict)
        return data

    def write_json(self, path: Path, data: Mapping[str, Any], *, indent: int = 2) -> None:
        path.write_text(json.dumps(data, indent=indent) + "\n", encoding="utf-8")

    def test_default_preflight_proves_full_bounded_recovery_plan(self) -> None:
        result = self.run_operator()

        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertEqual(result.stderr, "")
        report = json.loads(result.stdout)
        current_branch = self.current_git_branch()
        current_head_sha = self.current_git_head_sha()
        self.assertEqual(
            report["schema_name"],
            "mgpusim.mi300a_terminal_discovery_recovery_operator_preflight",
        )
        self.assertEqual(report["schema_version"], 1)
        self.assertIsNone(report["expected_operator_branch"])
        self.assertEqual(report["operator_branch"], current_branch)
        self.assertEqual(report["current_branch"], current_branch)
        self.assertEqual(report["operator_head_sha"], current_head_sha)
        self.assertEqual(report["current_head_sha"], current_head_sha)
        self.assertIsNone(report["expected_operator_head_sha"])
        self.assertIsNone(report["operator_git"]["expected_operator_branch"])
        self.assertEqual(report["operator_git"]["current_branch"], current_branch)
        self.assertEqual(report["operator_git"]["operator_head_sha"], current_head_sha)
        self.assertEqual(report["mode"], "preflight")
        self.assertEqual(report["status"], "pass")
        self.assertTrue(report["validated"])
        self.assertTrue(report["preflight"])
        self.assertTrue(report["dry_run"])
        self.assertTrue(report["read_only"])
        self.assertFalse(report["dispatch_performed"])
        self.assertFalse(report["simulator_execution_performed"])
        self.assertFalse(report["artifact_regeneration_performed"])
        self.assertFalse(report["workflow_modification_performed"])

        self.assertEqual(report["plan"]["sha256"], operator.EXPECTED_DRY_RUN_PLAN_SHA256)
        self.assertEqual(report["plan"]["expected_sha256"], operator.EXPECTED_DRY_RUN_PLAN_SHA256)
        self.assertTrue(report["plan"]["canonical_matches_checked_in"])
        self.assertTrue(report["plan"]["rederived_matches_checked_in"])
        self.assertEqual(report["recovery_manifest"]["sha256"], operator.EXPECTED_RECOVERY_MANIFEST_SHA256)
        self.assertEqual(report["source_run"]["run_id"], "25111815292")
        self.assertEqual(report["source_run"]["run_conclusion"], "cancelled")

        coverage = report["coverage"]
        self.assertEqual(coverage["source_plan_index_count"], 1416)
        self.assertEqual(coverage["missing_plan_index_count"], 940)
        self.assertEqual(coverage["dry_run_plan_index_count"], 940)
        self.assertEqual(coverage["dry_run_unique_plan_index_count"], 940)
        self.assertEqual(coverage["prior_observed_plan_index_count"], 476)
        self.assertEqual(coverage["prior_observed_unique_plan_index_count"], 476)
        self.assertEqual(
            coverage["missing_plan_index_count"] + coverage["prior_observed_plan_index_count"],
            coverage["source_plan_index_count"],
        )
        self.assertEqual(coverage["prior_observed_treatment"], "incomplete_prior_evidence_only")
        self.assertTrue(coverage["prior_observed_excluded_from_recovery"])
        self.assertTrue(coverage["not_complete_terminal_provenance"])
        self.assertEqual(coverage["duplicate_dry_run_plan_index_count"], 0)
        self.assertEqual(coverage["extra_dry_run_plan_index_count"], 0)
        self.assertEqual(coverage["missing_dry_run_plan_index_count"], 0)
        self.assertEqual(coverage["prior_observed_overlap_count"], 0)
        self.assertEqual(coverage["represented_plan_index_count_after_union"], 1416)
        self.assertEqual(coverage["recovery_batch_count"], 12)
        self.assertTrue(coverage["dry_run_matches_manifest_missing_scope"])
        self.assertEqual(
            [row["benchmark"] for row in coverage["complete_prior_rows_excluded_from_recovery"]],
            ["gelu", "sssp", "dmr"],
        )

        contract = report["operator_contract"]
        self.assertEqual(contract["max_parallel"], 14)
        self.assertEqual(contract["attempts_per_matrix_row"], 1)
        self.assertEqual(contract["per_attempt_timeout_sec"], 600)
        self.assertEqual(contract["benchmark_run_timeout_sec"], 600)
        self.assertEqual(contract["row_timeout_sec"], 600)
        self.assertEqual(contract["row_cap_sec"], 600)
        self.assertEqual(contract["row_required_timeout_sec"], 600)
        self.assertEqual(contract["action_timeout_sec"], 3600)
        self.assertEqual(contract["action_cap_sec"], 3600)
        self.assertEqual(contract["max_rows_per_batch"], 84)
        self.assertEqual(contract["row_violation_count"], 0)
        self.assertEqual(contract["batch_violation_count"], 0)
        self.assertEqual(contract["recovery_batch_count"], 12)
        self.assertEqual(contract["batch_row_counts"], [84] * 11 + [16])
        self.assertEqual(contract["batch_required_timeout_sec"], [3600] * 11 + [1200])
        self.assertTrue(contract["all_batches_fit_action_timeout"])
        self.assertTrue(contract["all_batches_max_parallel_at_most_14"])

        selection = report["batch_selection"]
        self.assertEqual(selection["requested"], ["all"])
        self.assertEqual(selection["selected_batch_indices"], list(range(1, 13)))
        self.assertEqual(selection["selected_batch_count"], 12)
        self.assertEqual(selection["available_batch_count"], 12)
        self.assertTrue(selection["all_batches_selected"])
        self.assertEqual(selection["selected_row_count"], 940)
        self.assertEqual(selection["selected_attempt_count"], 940)
        self.assertEqual(selection["selected_plan_index_count"], 940)
        self.assertEqual(selection["selected_unique_plan_index_count"], 940)
        self.assertEqual(selection["duplicate_selected_plan_index_count"], 0)
        self.assertEqual(selection["selected_plan_index_ranges"][0], "9-16")
        self.assertEqual(selection["selected_plan_index_ranges"][-1], "1393-1401")

        selected_batches = report["selected_batches"]
        self.assertEqual(len(selected_batches), 12)
        self.assertEqual([batch["row_count"] for batch in selected_batches], [84] * 11 + [16])
        for batch in selected_batches:
            self.assertTrue(batch["dry_run_only"])
            self.assertTrue(batch["recovery_only"])
            self.assertEqual(batch["attempt_count"], batch["row_count"])
            self.assertEqual(batch["per_size_attempt_count"], batch["row_count"])
            self.assertEqual(batch["max_parallel"], 14)
            self.assertEqual(batch["per_attempt_timeout_sec"], 600)
            self.assertEqual(batch["row_timeout_sec"], 600)
            self.assertEqual(batch["action_timeout_sec"], 3600)
            self.assertFalse(batch["would_dispatch_workflow"])
            self.assertFalse(batch["would_run_simulator"])
        self.assertEqual(
            report["guardrails"]["forbidden_side_effects"],
            EXPECTED_FORBIDDEN_SIDE_EFFECTS,
        )
        self.assertTrue(report["guardrails"]["actual_operator_branch_verified"])
        self.assertTrue(report["guardrails"]["actual_operator_head_sha_recorded"])
        self.assertFalse(report["guardrails"]["expected_operator_head_sha_verified"])
        self.assertIn("no workflow dispatch/cancel/rerun", report["non_goals"])
        self.assertIn(
            "no provenance, finishable manifest, or Tier artifact regeneration",
            report["non_goals"],
        )

    def test_accepts_current_expected_operator_branch_and_head_sha(self) -> None:
        current_branch = self.current_git_branch()
        current_head_sha = self.current_git_head_sha()
        returncode, stdout, stderr = self.run_operator_in_process(
            "preflight",
            "--expected-branch",
            current_branch,
            "--expected-head-sha",
            current_head_sha,
        )

        self.assertEqual(returncode, 0, msg=stderr)
        self.assertEqual(stderr, "")
        report = json.loads(stdout)
        self.assertEqual(report["operator_branch"], current_branch)
        self.assertEqual(report["expected_operator_branch"], current_branch)
        self.assertEqual(report["operator_head_sha"], current_head_sha)
        self.assertEqual(report["expected_operator_head_sha"], current_head_sha)
        self.assertEqual(
            report["operator_git"]["expected_operator_branch"],
            current_branch,
        )
        self.assertTrue(report["guardrails"]["expected_operator_head_sha_verified"])

    def test_rejects_configured_wrong_operator_branch_cleanly(self) -> None:
        current_branch = self.current_git_branch()
        wrong_branch = f"{current_branch}-wrong"
        with mock.patch.object(operator, "EXPECTED_OPERATOR_BRANCH", wrong_branch):
            returncode, stdout, stderr = self.run_operator_in_process("preflight")

        self.assertEqual(returncode, 1)
        self.assertEqual(stdout, "")
        self.assertIn("FAIL: MI300A recovery operator preflight rejected", stderr)
        self.assertIn("operator branch mismatch", stderr)
        self.assertIn("configured expectation", stderr)
        self.assertIn(f"expected {wrong_branch}", stderr)
        self.assertIn(f"got {current_branch}", stderr)
        self.assertNotIn("Traceback", stderr)

    def test_rejects_supplied_wrong_operator_branch_cleanly(self) -> None:
        current_branch = self.current_git_branch()
        wrong_branch = f"{current_branch}-wrong"
        returncode, stdout, stderr = self.run_operator_in_process(
            "preflight",
            "--expected-branch",
            wrong_branch,
        )

        self.assertEqual(returncode, 1)
        self.assertEqual(stdout, "")
        self.assertIn("FAIL: MI300A recovery operator preflight rejected", stderr)
        self.assertIn("operator branch mismatch", stderr)
        self.assertIn("supplied expectation", stderr)
        self.assertIn(f"expected {wrong_branch}", stderr)
        self.assertIn(f"got {current_branch}", stderr)
        self.assertNotIn("Traceback", stderr)

    def test_rejects_wrong_expected_operator_head_sha_cleanly(self) -> None:
        current_head_sha = self.current_git_head_sha()
        wrong_head_sha = "0" * 40 if current_head_sha != "0" * 40 else "1" * 40
        returncode, stdout, stderr = self.run_operator_in_process(
            "preflight",
            "--expected-head-sha",
            wrong_head_sha,
        )

        self.assertEqual(returncode, 1)
        self.assertEqual(stdout, "")
        self.assertIn("FAIL: MI300A recovery operator preflight rejected", stderr)
        self.assertIn("operator HEAD SHA mismatch", stderr)
        self.assertIn(f"expected {wrong_head_sha}", stderr)
        self.assertIn(f"got {current_head_sha}", stderr)
        self.assertNotIn("Traceback", stderr)

    def test_dry_run_mode_can_select_a_deterministic_batch_subset(self) -> None:
        result = self.run_operator("dry-run", "--batches", "12,1")

        self.assertEqual(result.returncode, 0, msg=result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual(report["mode"], "dry-run")
        selection = report["batch_selection"]
        self.assertEqual(selection["requested"], ["12,1"])
        self.assertEqual(selection["selected_batch_indices"], [1, 12])
        self.assertEqual(selection["selected_batch_count"], 2)
        self.assertFalse(selection["all_batches_selected"])
        self.assertEqual(selection["selected_row_count"], 100)
        self.assertEqual(selection["selected_attempt_count"], 100)
        self.assertEqual(selection["selected_plan_index_count"], 100)
        self.assertEqual(selection["selected_plan_index_ranges"][0], "9-16")
        self.assertEqual(selection["selected_plan_index_ranges"][-1], "1393-1401")
        self.assertEqual([batch["recovery_batch_index"] for batch in report["selected_batches"]], [1, 12])
        self.assertEqual(report["selected_batches"][0]["row_count"], 84)
        self.assertEqual(report["selected_batches"][1]["row_count"], 16)

    def test_rejects_duplicate_batch_selections(self) -> None:
        result = self.run_operator("preflight", "--batch", "2", "--batches", "1-2")

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("FAIL: MI300A recovery operator preflight rejected", result.stderr)
        self.assertIn("duplicate recovery batch selections: 2", result.stderr)

    def test_rejects_out_of_scope_batch_selections(self) -> None:
        for batch_value in ("0", "13"):
            with self.subTest(batch=batch_value):
                result = self.run_operator("preflight", "--batch", batch_value)

                self.assertEqual(result.returncode, 1)
                self.assertEqual(result.stdout, "")
                self.assertIn(f"out-of-scope recovery batch selections: {batch_value}", result.stderr)
                self.assertIn("valid batch range is 1-12", result.stderr)

    def test_rejects_over_cap_operator_overrides(self) -> None:
        for flag, value, field, int_value, cap in CONTRACT_CAP_FAILURE_CASES:
            with self.subTest(flag=flag):
                result = self.run_operator("preflight", flag, value)

                self.assertEqual(result.returncode, 1)
                self.assertEqual(result.stdout, "")
                self.assertIn("exceeds bounded recovery operator contract cap", result.stderr)
                self.assertIn(f"{field}={int_value}", result.stderr)
                self.assertIn(f"{field}<={cap}", result.stderr)

    def test_rejects_lower_contract_override_that_no_longer_matches_checked_in_plan(self) -> None:
        result = self.run_operator("preflight", "--max-parallel", "13")

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("does not match checked-in recovery dry-run plan contract", result.stderr)
        self.assertIn("max_parallel=14", result.stderr)

    def test_refuses_premature_regeneration_commands(self) -> None:
        for command in REGENERATION_COMMAND_REFUSAL_CASES:
            with self.subTest(command=command):
                result = self.run_operator(command)

                self.assertEqual(result.returncode, 2)
                self.assertEqual(result.stdout, "")
                self.assertIn("invalid choice", result.stderr)
                self.assertIn(command, result.stderr)
                self.assertIn("preflight", result.stderr)
                self.assertIn("dry-run", result.stderr)

    def test_rejects_noncanonical_plan_rendering_before_operator_selection(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            plan_path = Path(tmpdir) / "noncanonical_recovery_dry_run_plan.json"
            self.write_json(plan_path, self.load_json(CHECKED_IN_PLAN), indent=4)
            result = self.run_operator("preflight", "--dry-run-plan", str(plan_path))

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("canonical static recovery dry-run plan", result.stderr)

    def test_rejects_stale_or_regenerated_plan_sha_programmatically(self) -> None:
        original_sha = operator.EXPECTED_DRY_RUN_PLAN_SHA256
        try:
            operator.EXPECTED_DRY_RUN_PLAN_SHA256 = "0" * 64
            with self.assertRaisesRegex(
                operator.OperatorPreflightError,
                "stale or regenerated recovery dry-run plan sha256 mismatch",
            ):
                operator.build_operator_report(
                    mode="preflight",
                    plan_path=CHECKED_IN_PLAN,
                    manifest_path=CHECKED_IN_MANIFEST,
                    batch_specs=[],
                    requested_contract=operator.CONTRACT_CAPS,
                )
        finally:
            operator.EXPECTED_DRY_RUN_PLAN_SHA256 = original_sha

    def test_rejects_stale_or_regenerated_manifest_sha_programmatically(self) -> None:
        original_sha = operator.EXPECTED_RECOVERY_MANIFEST_SHA256
        try:
            operator.EXPECTED_RECOVERY_MANIFEST_SHA256 = "f" * 64
            with self.assertRaisesRegex(
                operator.OperatorPreflightError,
                "stale or regenerated recovery manifest sha256 mismatch",
            ):
                operator.build_operator_report(
                    mode="preflight",
                    plan_path=CHECKED_IN_PLAN,
                    manifest_path=CHECKED_IN_MANIFEST,
                    batch_specs=[],
                    requested_contract=operator.CONTRACT_CAPS,
                )
        finally:
            operator.EXPECTED_RECOVERY_MANIFEST_SHA256 = original_sha


if __name__ == "__main__":
    unittest.main()
