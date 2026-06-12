from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from typing import Any, Mapping


REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPTS_DIR = REPO_ROOT / "scripts"
if str(SCRIPTS_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPTS_DIR))

import validate_mi300a_recovery_evidence_readiness_boundary as boundary


SCRIPT = SCRIPTS_DIR / "validate_mi300a_recovery_evidence_readiness_boundary.py"
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
TMP_ROOT = REPO_ROOT / ".repo_tmp" / "mi300a_recovery_evidence_readiness_boundary_tests"


class ValidateMI300ARecoveryEvidenceReadinessBoundaryTest(unittest.TestCase):
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
            timeout=30,
        )

    def load_json(self, path: Path) -> dict[str, Any]:
        with path.open("r", encoding="utf-8") as f:
            data = json.load(f)
        self.assertIsInstance(data, dict)
        return data

    def write_json(self, path: Path, data: Mapping[str, Any]) -> None:
        path.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")

    def test_checked_in_boundary_passes_with_blocked_decision(self) -> None:
        result = self.run_validator()

        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertEqual(result.stderr, "")
        report = json.loads(result.stdout)
        self.assertEqual(report["schema_name"], "mgpusim.mi300a_recovery_evidence_readiness_boundary")
        self.assertEqual(report["schema_version"], 1)
        self.assertEqual(report["validation_status"], "pass")
        self.assertTrue(report["validated"])
        self.assertTrue(report["read_only"])
        self.assertTrue(report["pre_dispatch_static_validation"])

        inputs = report["inputs"]
        self.assertEqual(
            inputs["dry_run_plan"],
            "benchmark-comparison/generated/mi300a_terminal_discovery_run_25111815292_recovery_dry_run_plan.json",
        )
        self.assertEqual(
            inputs["recovery_manifest"],
            "benchmark-comparison/generated/mi300a_terminal_discovery_run_25111815292_recovery_manifest.json",
        )
        self.assertEqual(inputs["source_run_id"], "25111815292")
        self.assertEqual(inputs["source_run_conclusion"], "cancelled")

        collector_policy = report["source_policy"]["terminal_collector"]
        self.assertEqual(collector_policy["schema_name"], "mgpusim.mi300a_terminal_source_policy")
        self.assertEqual(collector_policy["policy"], "single_run_terminal_source_identity")
        self.assertTrue(collector_policy["single_run_identity_required"])
        self.assertFalse(collector_policy["reviewed_multi_source_schema"])
        self.assertFalse(collector_policy["multi_source_union_allowed"])
        self.assertEqual(collector_policy["raw_source_identity_count"], 1)
        self.assertEqual(collector_policy["logical_source_record_count"], 1416)
        finishable_policy = report["source_policy"]["finishable_generator"]
        self.assertTrue(finishable_policy["requires_single_run_source_policy"])
        self.assertTrue(finishable_policy["requires_successful_top_level_run"])
        self.assertTrue(finishable_policy["requires_exact_1416_terminal_accounting"])
        self.assertEqual(
            [probe["case"] for probe in finishable_policy["guard_probes"]],
            [
                "reviewed_multi_source_schema_true",
                "multi_source_union_allowed_true",
                "cancelled_top_level_run",
            ],
        )
        self.assertTrue(all(probe["rejected"] for probe in finishable_policy["guard_probes"]))

        scope = report["recovery_scope"]
        self.assertEqual(scope["source_plan_index_count"], 1416)
        self.assertEqual(scope["future_recovery_only_record_count"], 940)
        self.assertEqual(scope["prior_cancelled_run_observed_record_count"], 476)
        self.assertEqual(scope["represented_plan_index_count_after_accounting_union"], 1416)
        self.assertTrue(scope["accounting_union_covers_all_plan_indices"])
        self.assertFalse(scope["accounting_union_is_completion_evidence"])
        self.assertFalse(scope["recovery_only_artifacts_are_complete_terminal_collection"])
        self.assertFalse(scope["prior_cancelled_run_observations_are_complete_terminal_provenance"])
        self.assertEqual(scope["prior_observed_treatment"], "incomplete_prior_evidence_only")
        self.assertEqual(scope["source_run_conclusion"], "cancelled")

        decision = report["decision"]
        self.assertEqual(decision["status"], "blocked_under_current_reviewed_schema")
        self.assertFalse(decision["can_combine_476_prior_with_future_940_recovery_artifacts"])
        self.assertFalse(decision["can_collect_terminal_provenance_from_mixed_union"])
        self.assertFalse(decision["can_regenerate_finishable_manifest"])
        self.assertFalse(decision["can_regenerate_tier_artifacts"])
        self.assertFalse(decision["can_regenerate_terminal_provenance_for_completion"])
        self.assertFalse(decision["can_claim_issue_101_complete"])
        self.assertIn("single-run", decision["reason"])

        scenario_decisions = {item["scenario"]: item["decision"] for item in report["scenarios"]}
        self.assertEqual(
            scenario_decisions["future_940_recovery_plus_prior_476_cancelled_run_union"],
            "reject_mixed_source_union_under_current_schema",
        )
        self.assertEqual(
            scenario_decisions["future_940_recovery_only_artifact_set"],
            "reject_for_issue_101_completion",
        )
        self.assertTrue(all(not item["currently_available"] for item in report["unblockers"]))
        self.assertIn(
            "scripts/collect_mi300a_terminal_discovery_provenance.py single_run_terminal_source_identity",
            report["enforced_by"],
        )
        self.assertIn("no workflow dispatch/cancel/rerun", report["non_goals"])
        self.assertIn("no provenance, finishable manifest, or Tier artifact regeneration", report["non_goals"])
        self.assertIn("no terminal finishability completion claim", report["non_goals"])

    def test_source_policy_document_rejects_multi_source_schema_enablement(self) -> None:
        policy = dict(boundary.current_collector_source_policy())
        policy["reviewed_multi_source_schema"] = True

        with self.assertRaisesRegex(
            boundary.ReadinessBoundaryError,
            "reviewed_multi_source_schema must be false",
        ):
            boundary.validate_source_policy_document(policy, "test.source_policy")

    def test_source_policy_document_rejects_multi_source_union_enablement(self) -> None:
        policy = dict(boundary.current_collector_source_policy())
        policy["multi_source_union_allowed"] = True

        with self.assertRaisesRegex(
            boundary.ReadinessBoundaryError,
            "multi_source_union_allowed must be false",
        ):
            boundary.validate_source_policy_document(policy, "test.source_policy")

    def test_recovery_scope_rejects_completion_promotion(self) -> None:
        static_report = boundary.recovery_validator.validate_recovery_dry_run_plan(
            CHECKED_IN_PLAN,
            CHECKED_IN_MANIFEST,
        )
        tampered = json.loads(json.dumps(static_report))
        tampered["coverage"]["not_complete_terminal_provenance"] = False

        with self.assertRaisesRegex(
            boundary.ReadinessBoundaryError,
            "not_complete_terminal_provenance must be true",
        ):
            boundary.validate_static_recovery_scope(tampered)

    def test_cli_rejects_plan_without_no_completion_non_goal(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            plan_path = Path(tmpdir) / "completion_claim_recovery_dry_run_plan.json"
            plan = self.load_json(CHECKED_IN_PLAN)
            plan["generation_policy"]["non_goals"].remove("no terminal finishability completion claim")
            self.write_json(plan_path, plan)

            result = self.run_validator("--dry-run-plan", str(plan_path))

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("FAIL: MI300A recovery-evidence readiness boundary is not enforced.", result.stderr)
        self.assertIn("plan.generation_policy.non_goals missing", result.stderr)
        self.assertIn("no terminal finishability completion claim", result.stderr)
        self.assertNotIn("Traceback", result.stderr)


if __name__ == "__main__":
    unittest.main()
