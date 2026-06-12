from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock
from typing import Any, Mapping


REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPTS_DIR = REPO_ROOT / "scripts"
if str(SCRIPTS_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPTS_DIR))

import validate_mi300a_matrix_contract_boundary as boundary


SCRIPT = SCRIPTS_DIR / "validate_mi300a_matrix_contract_boundary.py"
CHECKED_IN_PLAN = (
    REPO_ROOT
    / "benchmark-comparison"
    / "generated"
    / "mi300a_terminal_discovery_run_25111815292_recovery_dry_run_plan.json"
)
MATRIX_CONTRACT_DOC = REPO_ROOT / "docs" / "m36_matrix_contract_boundary.md"
TMP_ROOT = REPO_ROOT / ".repo_tmp" / "mi300a_matrix_contract_boundary_tests"


class ValidateMI300AMatrixContractBoundaryTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        TMP_ROOT.mkdir(parents=True, exist_ok=True)

    def run_validator(self, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [sys.executable, str(SCRIPT), *args],
            cwd=REPO_ROOT,
            text=True,
            capture_output=True,
            check=False,
            timeout=60,
        )

    def load_json(self, path: Path) -> dict[str, Any]:
        with path.open("r", encoding="utf-8") as f:
            data = json.load(f)
        self.assertIsInstance(data, dict)
        return data

    def write_json(self, path: Path, data: Mapping[str, Any]) -> None:
        path.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")

    def cases_by_name(self, report: Mapping[str, Any]) -> dict[str, Mapping[str, Any]]:
        cases = report["cases"]
        self.assertIsInstance(cases, list)
        return {str(case["case"]): case for case in cases}

    def test_cli_proves_all_three_static_boundary_cases(self) -> None:
        result = self.run_validator("--max-violations", "5")

        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertEqual(result.stderr, "")
        report = json.loads(result.stdout)
        self.assertEqual(report["schema_name"], "mgpusim.mi300a_finishability_matrix_contract_boundary")
        self.assertEqual(report["schema_version"], 1)
        self.assertEqual(report["validation_status"], "pass")
        self.assertTrue(report["validated"])
        self.assertTrue(report["read_only"])
        self.assertTrue(report["pre_dispatch_static_validation"])
        self.assertEqual(report["inputs"]["full_run_artifact_source"]["source_type"], "checked_in_fixture")
        self.assertFalse(report["inputs"]["full_run_artifact_source"]["requires_local_git_object"])
        self.assertEqual(
            report["requested_278_contract"],
            {
                "max_parallel": 14,
                "benchmark_run_timeout_sec": 600,
                "row_timeout_sec": 600,
                "action_timeout_sec": 3600,
                "description": "14-way parallelism, 600-second benchmark-row/run layer, 3600-second action/batch layer",
            },
        )

        cases = self.cases_by_name(report)
        self.assertEqual(
            sorted(cases),
            [
                "archived_full_run_1416_separate_contract",
                "checked_in_recovery_only_940_14_600_3600",
                "literal_full_1416_per_benchmark_600s_row_cap",
            ],
        )

        recovery = cases["checked_in_recovery_only_940_14_600_3600"]
        self.assertEqual(recovery["validation_status"], "pass")
        self.assertTrue(recovery["is_278_shaped_recovery_path"])
        self.assertFalse(recovery["is_complete_1416_entry_single_run_terminal_provenance"])
        self.assertFalse(recovery["is_issue_101_completion_evidence"])
        self.assertFalse(recovery["can_regenerate_finishable_manifest"])
        self.assertFalse(recovery["can_regenerate_tier_artifacts"])
        self.assertEqual(
            recovery["shape"],
            {
                "recovery_only": True,
                "entry_count": 940,
                "matrix_row_count": 940,
                "attempts_per_matrix_row": 1,
                "prior_observed_record_count": 476,
                "represented_plan_index_count_after_accounting_union": 1416,
                "max_parallel": 14,
                "per_attempt_timeout_sec": 600,
                "benchmark_run_timeout_sec": 600,
                "row_timeout_sec": 600,
                "action_timeout_sec": 3600,
                "row_violation_count": 0,
                "batch_violation_count": 0,
                "max_batch_required_timeout_sec": 3600,
            },
        )
        self.assertEqual(
            recovery["m32_source_policy_decision"]["status"],
            "blocked_under_current_reviewed_schema",
        )
        self.assertFalse(recovery["m32_source_policy_decision"]["can_claim_issue_101_complete"])
        self.assertFalse(
            recovery["m32_source_policy_decision"][
                "can_combine_476_prior_with_future_940_recovery_artifacts"
            ]
        )

        full_run = cases["archived_full_run_1416_separate_contract"]
        self.assertEqual(full_run["validation_status"], "pass")
        self.assertEqual(full_run["run_identity"]["run_id"], "25196153223")
        self.assertEqual(
            full_run["run_identity"]["head_branch"],
            "archived-full-run-contract",
        )
        self.assertEqual(
            full_run["run_identity"]["head_sha"],
            "c60f56d19a23b43b020bb4f8e96391c20a2d2454",
        )
        self.assertEqual(
            full_run["run_identity"]["artifact_source"]["source_type"],
            "checked_in_fixture",
        )
        self.assertFalse(full_run["run_identity"]["artifact_source"]["requires_local_git_object"])
        self.assertEqual(full_run["workflow_contract"]["strategy_max_parallel"], 6)
        self.assertEqual(full_run["workflow_contract"]["row_timeout_minutes"], 4320)
        self.assertEqual(full_run["workflow_contract"]["row_timeout_sec"], 259200)
        self.assertEqual(full_run["workflow_contract"]["validation_per_attempt_timeout_sec"], 3600)
        self.assertEqual(full_run["workflow_contract"]["validation_row_overhead_sec"], 1800)
        self.assertEqual(full_run["operator_contract"]["expected_attempts"], 1416)
        self.assertEqual(full_run["operator_contract"]["expected_rows"], 81)
        self.assertEqual(full_run["operator_contract"]["expected_shards"], 6)
        self.assertEqual(full_run["operator_contract"]["nested_attempt_timeout_sec"], 3600)
        self.assertEqual(full_run["full_matrix_timeout_contract"]["violation_count"], 0)
        self.assertEqual(full_run["full_matrix_timeout_contract"]["max_attempt_count"], 54)
        self.assertEqual(full_run["full_matrix_timeout_contract"]["max_required_timeout_sec"], 196200)
        self.assertFalse(full_run["is_issue_278_compliant"])
        self.assertTrue(full_run["must_not_label_issue_278_compliant"])
        mismatch_fields = {item["field"] for item in full_run["mismatches_with_requested_278_contract"]}
        self.assertEqual(
            mismatch_fields,
            {"max_parallel", "row_timeout_sec", "nested_attempt_timeout_sec", "action_layer_timeout_sec"},
        )

        literal = cases["literal_full_1416_per_benchmark_600s_row_cap"]
        self.assertEqual(literal["validation_status"], "pass")
        self.assertTrue(literal["pre_dispatch_rejected"])
        self.assertTrue(literal["must_not_dispatch"])
        self.assertEqual(literal["timeout_contract_validation_status"], "fail")
        self.assertEqual(literal["candidate_contract"]["full_matrix_entry_count"], 1416)
        self.assertEqual(literal["candidate_contract"]["visible_benchmark_row_count"], 82)
        self.assertEqual(literal["candidate_contract"]["row_timeout_sec"], 600)
        self.assertEqual(literal["candidate_contract"]["per_attempt_timeout_sec"], 600)
        self.assertEqual(literal["candidate_contract"]["row_overhead_sec"], 0)
        self.assertEqual(literal["violation_count"], 81)
        self.assertTrue(literal["violations_truncated"])
        self.assertEqual(len(literal["violations"]), 5)
        self.assertEqual(
            literal["zero_overhead_max_row_evidence"],
            {
                "matrix_row_id": "mi300a-terminal-discovery-benchmark-parboil-tpacf",
                "benchmark": "parboil_tpacf",
                "attempt_count": 54,
                "per_attempt_timeout_sec": 600,
                "row_overhead_sec": 0,
                "required_timeout_sec": 32400,
                "row_timeout_sec": 600,
                "slack_sec": -31800,
            },
        )

        decision = report["decision"]
        self.assertEqual(decision["status"], "matrix_contract_boundary_enforced")
        self.assertFalse(decision["can_label_recovery_only_path_issue_101_complete"])
        self.assertFalse(decision["can_regenerate_from_recovery_only_or_476_plus_940_union"])
        self.assertFalse(decision["can_label_archived_full_run_issue_278_compliant"])
        self.assertFalse(decision["can_dispatch_literal_full_1416_per_benchmark_600s_row_cap"])
        self.assertFalse(decision["can_claim_issue_101_complete"])
        self.assertFalse(decision["can_claim_issue_278_complete"])
        self.assertIn("no workflow dispatch/cancel/rerun", report["non_goals"])
        self.assertIn("no artifact download or collection", report["non_goals"])
        self.assertIn("no terminal finishability completion claim", report["non_goals"])


    def test_matrix_contract_doc_is_removed(self) -> None:
        self.assertFalse(
            MATRIX_CONTRACT_DOC.exists(),
            msg=f"{MATRIX_CONTRACT_DOC} should have been removed as a process doc",
        )

    def test_literal_case_keeps_total_violation_count_when_rows_are_truncated(self) -> None:
        result = self.run_validator("--max-violations", "2")

        self.assertEqual(result.returncode, 0, msg=result.stderr)
        report = json.loads(result.stdout)
        literal = self.cases_by_name(report)["literal_full_1416_per_benchmark_600s_row_cap"]
        self.assertEqual(literal["violation_count"], 81)
        self.assertTrue(literal["violations_truncated"])
        self.assertEqual(len(literal["violations"]), 2)
        self.assertEqual(literal["zero_overhead_max_row_evidence"]["required_timeout_sec"], 32400)

    def test_cli_fails_cleanly_when_explicit_full_run_git_ref_is_missing(self) -> None:
        result = self.run_validator(
            "--full-run-source",
            "git",
            "--full-run-ref",
            "missing-full-run-ref-for-boundary-test",
        )

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("FAIL: MI300A finishability evidence matrix-contract boundary is not enforced.", result.stderr)
        self.assertIn(
            "git show missing-full-run-ref-for-boundary-test:.github/workflows/benchmark.yml failed",
            result.stderr,
        )
        self.assertNotIn("Traceback", result.stderr)

    def test_default_full_run_fixture_covers_shallow_checkout_missing_git_object(self) -> None:
        with mock.patch.object(
            boundary,
            "git_show_text",
            side_effect=AssertionError("default source must not inspect git objects"),
        ):
            full_run = boundary.build_full_run_contract_case(max_violations=3)
            legacy_ref_full_run = boundary.build_full_run_contract_case(
                boundary.FULL_RUN_DISPATCH_SHA,
                max_violations=3,
            )

        self.assertEqual(full_run["validation_status"], "pass")
        self.assertEqual(legacy_ref_full_run["validation_status"], "pass")
        self.assertEqual(full_run["run_identity"]["artifact_source"]["source_type"], "checked_in_fixture")
        self.assertFalse(full_run["run_identity"]["artifact_source"]["requires_local_git_object"])
        self.assertEqual(full_run["full_matrix_timeout_contract"]["row_count"], 81)
        self.assertEqual(full_run["full_matrix_timeout_contract"]["attempt_count"], 1416)

    def test_cli_fails_cleanly_if_recovery_plan_promotes_completion(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            plan_path = Path(tmpdir) / "completion_claim_recovery_dry_run_plan.json"
            plan = self.load_json(CHECKED_IN_PLAN)
            plan["coverage"]["not_complete_terminal_provenance"] = False
            self.write_json(plan_path, plan)

            result = self.run_validator("--recovery-dry-run-plan", str(plan_path))

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("FAIL: MI300A finishability evidence matrix-contract boundary is not enforced.", result.stderr)
        self.assertIn("not_complete_terminal_provenance", result.stderr)
        self.assertNotIn("Traceback", result.stderr)

    def test_direct_full_run_case_uses_checked_in_fixture_static_timeout_accounting(self) -> None:
        full_run = boundary.build_full_run_contract_case(max_violations=3)

        self.assertEqual(full_run["run_identity"]["artifact_source"]["source_type"], "checked_in_fixture")
        self.assertEqual(full_run["workflow_contract"]["strategy_max_parallel"], 6)
        self.assertEqual(full_run["workflow_contract"]["row_timeout_minutes"], 4320)
        self.assertEqual(full_run["operator_contract"]["nested_attempt_timeout_sec"], 3600)
        self.assertEqual(full_run["full_matrix_timeout_contract"]["row_count"], 81)
        self.assertEqual(full_run["full_matrix_timeout_contract"]["attempt_count"], 1416)
        self.assertEqual(full_run["full_matrix_timeout_contract"]["violation_count"], 0)
        self.assertFalse(full_run["is_issue_278_compliant"])


if __name__ == "__main__":
    unittest.main()
