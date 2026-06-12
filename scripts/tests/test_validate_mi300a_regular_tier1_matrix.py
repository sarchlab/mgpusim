"""Tests for the retired generated MI300A regular Tier-1 audit validator."""

from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from typing import Any


REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPTS_DIR = REPO_ROOT / "scripts"
if str(SCRIPTS_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPTS_DIR))

import validate_mi300a_regular_tier1_matrix as validator  # noqa: E402


class ValidateMI300ARetiredGeneratedRegularTier1AuditMatrixTest(unittest.TestCase):
    """Validate audit-only behavior for the retired generated 600s/3600s artifact."""

    def setUp(self) -> None:
        self.matrix_path = REPO_ROOT / "benchmark-comparison" / "generated" / "mi300a_regular_tier1_matrix.json"
        self.matrix = self._read_json(self.matrix_path)
        self.plan_by_index = {
            entry["plan_index"]: entry for entry in self._read_json(validator.DEFAULT_PLAN)["entries"]
        }
        self.manifest_by_index = {
            entry["plan_index"]: entry for entry in self._read_json(validator.DEFAULT_MANIFEST)["entries"]
        }

    def test_checked_in_retired_generated_audit_matrix_passes_with_source_report(self) -> None:
        report = validator.validate_retired_generated_regular_tier1_matrix(
            matrix_path=self.matrix_path,
            manifest_path=validator.DEFAULT_MANIFEST,
            plan_path=validator.DEFAULT_PLAN,
        )

        self.assertEqual(report["schema_name"], validator.VALIDATION_SCHEMA)
        self.assertEqual(report["status"], "pass")
        self.assertEqual(report["validation_scope"]["artifact_role"], "retired_generated_audit_only")
        self.assertTrue(report["validation_scope"]["not_maintained_regular_workflow_matrix"])
        self.assertEqual(
            report["source_paths"]["retired_generated_regular_tier1_matrix"],
            "benchmark-comparison/generated/mi300a_regular_tier1_matrix.json",
        )
        self.assertEqual(
            report["retired_artifact_lifecycle"]["status"],
            validator.RETIRED_ARTIFACT_LIFECYCLE_STATUS,
        )
        self.assertTrue(report["retired_artifact_lifecycle"]["retired_generated_audit_only"])
        self.assertEqual(
            report["retired_artifact_lifecycle"]["preserved_runtime_contract"],
            "historical_600s_per_simulator_3600s_job",
        )
        self.assertEqual(
            report["maintained_regular_workflow_contract_reference"],
            {
                "benchmark_job_timeout_minutes": 360,
                "benchmark_job_timeout_sec": 21600,
                "max_parallel": 14,
                "per_simulator_timeout_sec": 7200,
            },
        )
        self.assertEqual(report["retired_generated_matrix"]["row_count"], 12)
        self.assertEqual(report["retired_generated_matrix"]["entry_count"], 12)
        self.assertGreaterEqual(report["retired_generated_matrix"]["distinct_benchmark_family_count"], 10)
        self.assertEqual(report["retired_generated_matrix"]["artifact_id_count"], 12)
        self.assertEqual(report["retired_generated_runtime_contract"]["max_parallel"], 14)
        self.assertEqual(report["retired_generated_runtime_contract"]["per_simulator_timeout_sec"], 600)
        self.assertEqual(report["retired_generated_runtime_contract"]["benchmark_job_timeout_sec"], 3600)
        self.assertTrue(report["retired_row_budget_validation"]["all_rows_within_budget"])
        self.assertEqual(report["retired_row_budget_validation"]["max_row_budget_sec"], 600)
        self.assertEqual(
            report["retired_generated_matrix"]["source_plan_index_ranges"],
            ["1", "17", "33", "49", "65", "81", "97", "113", "132", "151", "167", "183"],
        )
        self.assertEqual(report["source_reference"]["plan_entry_count"], 1416)
        self.assertEqual(report["source_reference"]["manifest_entry_count"], 1416)
        self.assertEqual(report["source_reference"]["tier1_source_entry_count"], 308)
        axis_contract = report["cache_bandwidth_axis_contract"]
        self.assertEqual(axis_contract["status"], "pass")
        for benchmark, expected_range in {
            "l1_cache_bw": ["17"],
            "l2_cache_bw": ["33"],
        }.items():
            with self.subTest(benchmark=benchmark):
                contract = axis_contract["benchmarks"][benchmark]
                self.assertEqual(contract["scaling_axis"], "num_elements")
                self.assertEqual(contract["flag_template"], "-num_elements {size}")
                self.assertEqual(contract["size_label_template"], "{size}")
                self.assertEqual(contract["source_plan_entry_count"], 16)
                self.assertEqual(contract["matrix_row_count"], 1)
                self.assertEqual(
                    contract["matrix_source_plan_index_ranges"],
                    expected_range,
                )
        self.assertIn("no workflow dispatch/cancel/rerun", report["non_goals"])
        self.assertIn("no terminal finishability completion claim", report["non_goals"])

    def test_cli_emits_stable_json_report(self) -> None:
        result = subprocess.run(
            [sys.executable, str(SCRIPTS_DIR / "validate_mi300a_regular_tier1_matrix.py")],
            cwd=REPO_ROOT,
            text=True,
            capture_output=True,
            check=False,
            timeout=30,
        )

        self.assertEqual(result.returncode, 0, msg=f"STDOUT:\n{result.stdout}\nSTDERR:\n{result.stderr}")
        report = json.loads(result.stdout)
        self.assertEqual(report["schema_name"], validator.VALIDATION_SCHEMA)
        self.assertEqual(report["validation_scope"]["artifact_role"], "retired_generated_audit_only")
        self.assertEqual(report["retired_generated_matrix"]["row_count"], 12)
        self.assertEqual(report["retired_generated_runtime_contract"]["per_simulator_timeout_sec"], 600)
        self.assertEqual(report["maintained_regular_workflow_contract_reference"]["per_simulator_timeout_sec"], 7200)
        self.assertEqual(result.stderr, "")

    def test_rejects_retired_runtime_contract_drift_from_14_600_3600(self) -> None:
        drift_cases = [
            ("max_parallel", 6, "retired generated.*max_parallel"),
            ("per_simulator_timeout_sec", 3600, "retired generated.*per_simulator_timeout_sec"),
            ("benchmark_job_timeout_sec", 600, "retired generated.*benchmark_job_timeout_sec"),
        ]
        for field, value, expected_message in drift_cases:
            with self.subTest(field=field):
                matrix = self._copy_matrix()
                matrix["runtime_contract"][field] = value

                with self.assertRaisesRegex(validator.ValidationError, expected_message):
                    self._validate_temp_matrix(matrix)

    def test_rejects_maintained_workflow_wording_on_retired_600s_3600s_artifact(self) -> None:
        cases = [
            (
                ("selection_policy", "scope"),
                "regular Tier 1 breadth/probe matrix for the maintained MI300A benchmark workflow",
                "matrix.selection_policy.scope",
            ),
            (
                ("selection_policy", "non_finishable_claim"),
                "non-eligible or non-terminal source rows are included only as regular workflow "
                "probes and are not promoted to finishable evidence",
                "matrix.selection_policy.non_finishable_claim",
            ),
            (
                ("artifact_lifecycle", "preserved_runtime_contract_note"),
                "runtime_contract intentionally preserves the historical 600s per-simulator and "
                "3600s benchmark-job values captured by this retired artifact for the maintained "
                "regular workflow matrix.",
                "matrix.artifact_lifecycle.preserved_runtime_contract_note",
            ),
        ]
        for path, confusing_text, expected_label in cases:
            with self.subTest(path=".".join(path)):
                matrix = self._copy_matrix()
                parent, key = path
                matrix[parent][key] = confusing_text

                with self.assertRaisesRegex(
                    validator.ValidationError,
                    f"{expected_label}.*confusing maintained-workflow wording",
                ):
                    self._validate_temp_matrix(matrix)

    def test_rejects_too_few_retired_audit_rows(self) -> None:
        matrix = self._copy_matrix()
        matrix["include"] = matrix["include"][:11]
        matrix["row_count"] = 11
        matrix["entry_count"] = 11

        with self.assertRaisesRegex(validator.ValidationError, "at least 12"):
            self._validate_temp_matrix(matrix)

    def test_rejects_too_few_distinct_retired_audit_families(self) -> None:
        matrix = self._copy_matrix()
        replacement_plan_indices = [2, 3, 4]
        for row_offset, plan_index in zip(range(9, 12), replacement_plan_indices):
            self._retarget_single_size_row(
                matrix["include"][row_offset],
                plan_index=plan_index,
                artifact_id=f"mem_latency_chase_extra_{plan_index}",
            )
        matrix["distinct_benchmark_family_count"] = 9
        matrix["benchmark_families"] = sorted({row["benchmark_family"] for row in matrix["include"]})
        matrix["source_plan_index_ranges"] = validator.make_ranges(
            index for row in matrix["include"] for index in row["source_plan_indices"]
        )

        with self.assertRaisesRegex(
            validator.ValidationError,
            "at least 10 distinct audit benchmark families",
        ):
            self._validate_temp_matrix(matrix)

    def test_rejects_top_level_source_path_mismatch(self) -> None:
        matrix = self._copy_matrix()
        matrix["source_plan"] = "benchmark-comparison/generated/not-the-source-plan.json"

        with self.assertRaisesRegex(validator.ValidationError, "matrix.source_plan"):
            self._validate_temp_matrix(matrix)

    def test_rejects_source_reference_mismatch(self) -> None:
        matrix = self._copy_matrix()
        matrix["include"][0]["source_plan_indices"] = [2]

        with self.assertRaisesRegex(validator.ValidationError, "size_token"):
            self._validate_temp_matrix(matrix)

    def test_rejects_cache_bandwidth_plan_and_matrix_stale_axis(self) -> None:
        matrix = self._copy_matrix()
        plan = self._read_json(validator.DEFAULT_PLAN)
        manifest = self._read_json(validator.DEFAULT_MANIFEST)

        for document in (plan, manifest):
            for entry in document["entries"]:
                if entry["benchmark"] == "l1_cache_bw":
                    entry["flag_template"] = "-working_set_size {size}"
        for row in matrix["include"]:
            if row["benchmark"] == "l1_cache_bw":
                row["flag_template"] = "-working_set_size {size}"

        with self.assertRaisesRegex(
            validator.ValidationError,
            r"plan\.entries\[17\]\.flag_template",
        ):
            self._validate_temp_documents(matrix, plan=plan, manifest=manifest)

    def test_rejects_row_budget_overflow(self) -> None:
        matrix = self._copy_matrix()
        first = matrix["include"][0]
        first["sizes"] = "256 512 1024 2048 4096 8192 16384"
        first["expected_simulator_invocations"] = 7
        first["row_timeout_budget_sec"] = 4200
        first["source_plan_indices"] = [1, 2, 3, 4, 5, 6, 7]
        first["source_size_tokens"] = ["256", "512", "1024", "2048", "4096", "8192", "16384"]
        first["source_observed_outcome_buckets"] = [
            "completed_success",
            "non_terminal",
            "completed_success",
            "non_terminal",
            "non_terminal",
            "completed_success",
            "non_terminal",
        ]
        first["source_eligibility_statuses"] = [
            "eligible",
            "excluded",
            "eligible",
            "excluded",
            "excluded",
            "eligible",
            "excluded",
        ]

        with self.assertRaisesRegex(
            validator.ValidationError,
            "exceeds retired generated audit benchmark job timeout budget",
        ):
            self._validate_temp_matrix(matrix)

    def test_rejects_duplicate_artifact_ids(self) -> None:
        matrix = self._copy_matrix()
        matrix["include"][1]["artifact_id"] = matrix["include"][0]["artifact_id"]

        with self.assertRaisesRegex(validator.ValidationError, "duplicates another matrix row"):
            self._validate_temp_matrix(matrix)

    def _retarget_single_size_row(self, row: dict[str, Any], *, plan_index: int, artifact_id: str) -> None:
        plan_entry = self.plan_by_index[plan_index]
        manifest_entry = self.manifest_by_index[plan_index]
        self.assertEqual(plan_entry["tier"], validator.EXPECTED_TIER)
        self.assertEqual(plan_entry["tier_name"], validator.EXPECTED_TIER_NAME)
        self.assertEqual(plan_entry["workflow_job"], "benchmark-tier1")

        row.update(
            {
                "benchmark": plan_entry["workflow_benchmark_name"],
                "benchmark_family": plan_entry["benchmark"],
                "sizes": str(plan_entry["sizes"]),
                "flag_template": plan_entry["flag_template"],
                "size_label_template": plan_entry["size_label_template"],
                "artifact_id": artifact_id,
                "source_plan_indices": [plan_index],
                "source_size_tokens": [str(plan_entry["size_token"])],
                "source_observed_outcome_buckets": [manifest_entry["observed_outcome_bucket"]],
                "source_eligibility_statuses": [manifest_entry["eligibility_status"]],
            }
        )
        for key in ("extra_flags", "gomemlimit", "gogc"):
            if key in plan_entry:
                row[key] = plan_entry[key]
            else:
                row.pop(key, None)

    def _validate_temp_matrix(self, matrix: dict[str, Any]) -> dict[str, Any]:
        with tempfile.TemporaryDirectory() as tmp:
            matrix_path = Path(tmp) / "retired_generated_regular_tier1.json"
            matrix_path.write_text(json.dumps(matrix, indent=2) + "\n", encoding="utf-8")
            return validator.validate_retired_generated_regular_tier1_matrix(
                matrix_path=matrix_path,
                manifest_path=validator.DEFAULT_MANIFEST,
                plan_path=validator.DEFAULT_PLAN,
            )

    def _validate_temp_documents(
        self,
        matrix: dict[str, Any],
        *,
        plan: dict[str, Any] | None = None,
        manifest: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            plan_path = validator.DEFAULT_PLAN
            manifest_path = validator.DEFAULT_MANIFEST

            if plan is not None:
                plan_path = tmp_path / "mi300a_problem_size_discovery_plan.json"
                plan_path.write_text(
                    json.dumps(plan, indent=2) + "\n",
                    encoding="utf-8",
                )
                matrix["source_plan"] = validator.repo_relative(plan_path)
            if manifest is not None:
                manifest_path = tmp_path / "mi300a_finishable_size_manifest.json"
                manifest["source_paths"]["problem_size_discovery_plan"] = (
                    validator.repo_relative(plan_path)
                )
                manifest_path.write_text(
                    json.dumps(manifest, indent=2) + "\n",
                    encoding="utf-8",
                )
                matrix["source_manifest"] = validator.repo_relative(manifest_path)

            matrix_path = tmp_path / "retired_generated_regular_tier1.json"
            matrix_path.write_text(json.dumps(matrix, indent=2) + "\n", encoding="utf-8")
            return validator.validate_retired_generated_regular_tier1_matrix(
                matrix_path=matrix_path,
                manifest_path=manifest_path,
                plan_path=plan_path,
            )

    def _copy_matrix(self) -> dict[str, Any]:
        return json.loads(json.dumps(self.matrix))

    def _read_json(self, path: Path) -> dict[str, Any]:
        with path.open("r", encoding="utf-8") as f:
            data = json.load(f)
        self.assertIsInstance(data, dict)
        return data


if __name__ == "__main__":
    unittest.main()
