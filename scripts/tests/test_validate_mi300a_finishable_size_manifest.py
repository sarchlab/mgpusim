from __future__ import annotations

import json
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from typing import Any, Mapping

from scripts.tests.mi300a_finishable_terminal_fixture import write_synthetic_complete_terminal_provenance


REPO_ROOT = Path(__file__).resolve().parents[2]
GENERATOR = REPO_ROOT / "scripts" / "generate_mi300a_finishable_size_manifest.py"
VALIDATOR = REPO_ROOT / "scripts" / "validate_mi300a_finishable_size_manifest.py"
PLAN = REPO_ROOT / "benchmark-comparison" / "mi300a_problem_size_discovery_plan.json"
PROVENANCE = (
    REPO_ROOT
    / "benchmark-comparison"
    / "provenance"
    / "mi300a_problem_size_discovery_run_24959959195.json"
)
TMP_ROOT = REPO_ROOT / ".repo_tmp" / "mi300a_finishable_size_validator_tests"


class ValidateMI300AFinishableSizeManifestTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        TMP_ROOT.mkdir(parents=True, exist_ok=True)

    def load_json(self, path: Path) -> dict[str, Any]:
        with path.open("r", encoding="utf-8") as f:
            data = json.load(f)
        self.assertIsInstance(data, dict)
        return data

    def write_json(self, path: Path, data: Mapping[str, Any]) -> None:
        path.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")

    def output_paths(self, tmp_path: Path) -> dict[str, Path]:
        return {
            "manifest": tmp_path / "mi300a_finishable_size_manifest.json",
            "tier1": tmp_path / "mi300a_finishable_tier1_matrix.json",
            "tier2": tmp_path / "mi300a_finishable_tier2_matrix.json",
            "summary": tmp_path / "mi300a_finishable_size_summary.json",
        }

    def generate_fixture(self, tmp_path: Path) -> dict[str, Path]:
        paths = self.output_paths(tmp_path)
        result = subprocess.run(
            [
                sys.executable,
                str(GENERATOR),
                "--plan",
                str(PLAN),
                "--provenance",
                str(PROVENANCE),
                "--output",
                str(paths["manifest"]),
                "--tier1-output",
                str(paths["tier1"]),
                "--tier2-output",
                str(paths["tier2"]),
                "--summary-output",
                str(paths["summary"]),
            ],
            cwd=REPO_ROOT,
            text=True,
            capture_output=True,
            check=False,
            timeout=30,
        )
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        return paths

    def run_validator(
        self,
        paths: Mapping[str, Path] | None = None,
        provenance_path: Path = PROVENANCE,
    ) -> subprocess.CompletedProcess[str]:
        command = [
            sys.executable,
            str(VALIDATOR),
            "--plan",
            str(PLAN),
            "--provenance",
            str(provenance_path),
        ]
        if paths is not None:
            command.extend(
                [
                    "--manifest",
                    str(paths["manifest"]),
                    "--tier1",
                    str(paths["tier1"]),
                    "--tier2",
                    str(paths["tier2"]),
                    "--summary",
                    str(paths["summary"]),
                ]
            )
        return subprocess.run(
            command,
            cwd=REPO_ROOT,
            text=True,
            capture_output=True,
            check=False,
            timeout=30,
        )

    def copy_provenance(self, tmp_path: Path) -> Path:
        copied = tmp_path / "provenance.json"
        shutil.copyfile(PROVENANCE, copied)
        return copied

    def relabel_provenance_run_completed(self, provenance_path: Path) -> None:
        provenance = self.load_json(provenance_path)
        run = dict(provenance["run"])
        self.assertEqual(run["status"], "queued")
        self.assertTrue(run["non_terminal_snapshot"])
        run["status"] = "completed"
        run["conclusion"] = "success"
        run["non_terminal_snapshot"] = False
        provenance["run"] = run
        self.write_json(provenance_path, provenance)

    def relabel_generated_outputs_like_pre_fix_terminal_claim(self, paths: Mapping[str, Path]) -> None:
        manifest = self.load_json(paths["manifest"])
        source = manifest["snapshot_contract"].get("source")
        observed_at = manifest["run"].get("observed_at_utc")
        terminal_contract = {
            "observation_time_utc": observed_at,
            "non_terminal_snapshot": False,
            "bounded_snapshot": False,
            "terminal_provenance": False,
            "claim_scope": (
                "terminal discovery evidence regenerated from compact provenance; existing checked-in M4/M7 "
                "bounded snapshot artifacts remain historical unless explicitly regenerated from this terminal provenance"
            ),
            "source": source,
        }

        for key in ("manifest", "summary"):
            document = self.load_json(paths[key])
            run = dict(document["run"])
            run["status"] = "completed"
            run["conclusion"] = "success"
            run["non_terminal_snapshot"] = False
            document["run"] = run
            observation = dict(document.get("observation", {}))
            observation["non_terminal_snapshot"] = False
            document["observation"] = observation
            document["snapshot_contract"] = dict(terminal_contract)
            if key == "manifest":
                for entry in document["entries"]:
                    entry["run_status"] = "completed"
                    entry["run_conclusion"] = "success"
            self.write_json(paths[key], document)

        for key in ("tier1", "tier2"):
            document = self.load_json(paths[key])
            run = dict(document["run"])
            run["status"] = "completed"
            run["conclusion"] = "success"
            run["non_terminal_snapshot"] = False
            document["run"] = run
            document["snapshot_contract"] = dict(terminal_contract)
            self.write_json(paths[key], document)

    def plan_entry(self, plan_index: int) -> Mapping[str, Any]:
        plan = self.load_json(PLAN)
        return plan["entries"][plan_index - 1]

    def test_checked_in_manifest_validates_with_deterministic_json_report(self) -> None:
        result = self.run_validator()

        self.assertEqual(result.returncode, 0, msg=result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual(report["schema_name"], "mgpusim.mi300a_finishable_size_manifest_validation")
        self.assertEqual(report["schema_version"], 1)
        self.assertEqual(report["status"], "pass")
        self.assertEqual(report["plan_entry_count"], 1416)
        self.assertEqual(report["manifest_entry_count"], 1416)
        self.assertEqual(report["eligible_entry_count"], 23)
        self.assertEqual(report["excluded_entry_count"], 1393)
        self.assertEqual(report["eligible_plan_index_ranges"], ["1", "3", "6", "9", "237-238", "473-482", "720-722", "955-958"])
        self.assertEqual(
            report["observed_outcome_bucket_counts"],
            {"completed_success": 23, "non_terminal": 1373, "timed_out": 20},
        )
        self.assertEqual(
            report["hotspot_48"],
            {
                "benchmark": "hotspot",
                "duration_sec": 8,
                "eligible_for_linear_workflow": True,
                "observed_outcome": "success",
                "observed_outcome_bucket": "completed_success",
                "plan_index": 474,
                "problem_size": "48",
            },
        )
        self.assertEqual(report["tier_counts"]["1"]["eligible"], 6)
        self.assertEqual(report["tier_counts"]["2"]["eligible"], 17)
        self.assertEqual(report["provenance"]["terminal_outcome_row_count"], 43)
        self.assertEqual(report["provenance"]["artifact_entry_count"], 43)
        self.assertEqual(report["provenance"]["terminal_job_entry_count"], 43)
        self.assertEqual(
            report["provenance"]["requested_bucket_count_reconciliation"],
            {
                "primary_bucket_counts": {
                    "completed_success": 23,
                    "failed": 0,
                    "timed_out": 20,
                    "skipped": 0,
                    "cancelled": 0,
                    "non_terminal": 1373,
                    "missing_job": 0,
                },
                "primary_bucket_total": 1416,
                "terminal_bucket_counts": {
                    "completed_success": 23,
                    "failed": 0,
                    "timed_out": 20,
                    "skipped": 0,
                    "cancelled": 0,
                },
                "terminal_bucket_total": 43,
                "non_terminal_substatus_bucket_counts": {
                    "non_terminal_in_progress": 20,
                    "non_terminal_queued": 1353,
                },
                "non_terminal_substatus_bucket_total": 1373,
                "diagnostic_bucket_counts": {
                    "missing_artifact_for_completed_success_job": 0,
                    "missing_outcome_row_for_completed_success_job": 0,
                    "missing_outcome_row": 1373,
                },
                "diagnostic_bucket_total": 1373,
            },
        )
        self.assertEqual(
            report["source_paths"],
            {
                "problem_size_discovery_plan": "benchmark-comparison/mi300a_problem_size_discovery_plan.json",
                "discovery_provenance": "benchmark-comparison/provenance/mi300a_problem_size_discovery_run_24959959195.json",
                "finishable_manifest": "benchmark-comparison/mi300a_finishable_size_manifest.json",
                "tier1_view": "benchmark-comparison/generated/mi300a_finishable_tier1_matrix.json",
                "tier2_view": "benchmark-comparison/generated/mi300a_finishable_tier2_matrix.json",
                "summary": "benchmark-comparison/mi300a_finishable_size_summary.json",
            },
        )

    def test_accepts_synthetic_complete_terminal_accounting_manifest(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            provenance_path = write_synthetic_complete_terminal_provenance(tmp_path)
            paths = self.output_paths(tmp_path)
            generate = subprocess.run(
                [
                    sys.executable,
                    str(GENERATOR),
                    "--plan",
                    str(PLAN),
                    "--provenance",
                    str(provenance_path),
                    "--output",
                    str(paths["manifest"]),
                    "--tier1-output",
                    str(paths["tier1"]),
                    "--tier2-output",
                    str(paths["tier2"]),
                    "--summary-output",
                    str(paths["summary"]),
                ],
                cwd=REPO_ROOT,
                text=True,
                capture_output=True,
                check=False,
                timeout=30,
            )
            self.assertEqual(generate.returncode, 0, msg=generate.stderr)

            result = self.run_validator(paths, provenance_path)

        self.assertEqual(result.returncode, 0, msg=result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual(report["status"], "pass")
        self.assertEqual(report["provenance"]["terminal_outcome_row_count"], 1416)
        self.assertEqual(report["provenance"]["terminal_job_entry_count"], 1416)
        self.assertEqual(report["provenance"]["terminal_plan_index_ranges"], ["1-1416"])
        self.assertEqual(report["provenance"]["requested_bucket_count_reconciliation"]["terminal_bucket_total"], 1416)
        self.assertEqual(report["provenance"]["requested_bucket_count_reconciliation"]["primary_bucket_counts"]["non_terminal"], 0)

    def test_rejects_duplicate_or_missing_manifest_entries(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            paths = self.generate_fixture(tmp_path)
            manifest = self.load_json(paths["manifest"])
            manifest["entries"][1] = dict(manifest["entries"][0])
            self.write_json(paths["manifest"], manifest)

            result = self.run_validator(paths)

        self.assertEqual(result.returncode, 1)
        self.assertIn("manifest entries must cover exactly the 1416-entry plan once", result.stderr)
        self.assertIn("duplicates=['1']", result.stderr)
        self.assertIn("missing=['2']", result.stderr)

    def test_rejects_hotspot_48_manifest_tampering(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            paths = self.generate_fixture(tmp_path)
            manifest = self.load_json(paths["manifest"])
            hotspot = dict(manifest["entries"][473])
            self.assertEqual(hotspot["plan_index"], 474)
            hotspot["hardware_problem_size"] = "64"
            hotspot["problem_size"] = "64"
            manifest["entries"][473] = hotspot
            self.write_json(paths["manifest"], manifest)

            result = self.run_validator(paths)

        self.assertEqual(result.returncode, 1)
        self.assertIn("manifest must include exactly one hotspot/48 entry at plan_index 474", result.stderr)
        self.assertIn("found_plan_indices=[]", result.stderr)

    def test_rejects_stale_manifest_artifact_provenance_drift(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            paths = self.generate_fixture(tmp_path)
            manifest = self.load_json(paths["manifest"])
            hotspot = dict(manifest["entries"][473])
            self.assertEqual(hotspot["plan_index"], 474)
            hotspot["artifact_database_id"] = 123456789
            manifest["entries"][473] = hotspot
            self.write_json(paths["manifest"], manifest)

            result = self.run_validator(paths)

        self.assertEqual(result.returncode, 1)
        self.assertIn("differs from regenerated evidence", result.stderr)
        self.assertIn("artifact_database_id", result.stderr)

    def test_rejects_stale_tier_view_drift_from_manifest_and_provenance(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            paths = self.generate_fixture(tmp_path)
            tier2 = self.load_json(paths["tier2"])
            self.assertEqual(tier2["include"][0]["finishable_plan_index_ranges"], ["473-482"])
            tier2["include"][0]["finishable_plan_index_ranges"] = ["473", "475-482"]
            self.write_json(paths["tier2"], tier2)

            result = self.run_validator(paths)

        self.assertEqual(result.returncode, 1)
        self.assertIn("mi300a_finishable_tier2_matrix.json differs from regenerated evidence", result.stderr)
        self.assertIn("finishable_plan_index_ranges", result.stderr)

    def test_rejects_final_current_claim_for_non_terminal_snapshot(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            paths = self.generate_fixture(tmp_path)
            manifest = self.load_json(paths["manifest"])
            contract = dict(manifest["snapshot_contract"])
            self.assertTrue(contract["non_terminal_snapshot"])
            contract["claim_scope"] = "final/current finishable-size claims for the active workflow run"
            manifest["snapshot_contract"] = contract
            self.write_json(paths["manifest"], manifest)

            result = self.run_validator(paths)

        self.assertEqual(result.returncode, 1)
        self.assertIn("claim_scope for non-terminal snapshots", result.stderr)
        self.assertIn("not final/current finishable-size claims", result.stderr)

    def test_rejects_provenance_that_marks_non_terminal_run_as_final(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            paths = self.generate_fixture(tmp_path)
            provenance_path = self.copy_provenance(tmp_path)
            provenance = self.load_json(provenance_path)
            run = dict(provenance["run"])
            self.assertEqual(run["status"], "queued")
            run["non_terminal_snapshot"] = False
            provenance["run"] = run
            self.write_json(provenance_path, provenance)

            result = self.run_validator(paths, provenance_path)

        self.assertEqual(result.returncode, 1)
        self.assertIn("run.non_terminal_snapshot must be true when run.status is non-terminal", result.stderr)

    def test_rejects_relabelled_bounded_snapshot_even_with_pre_fix_matching_outputs(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            paths = self.generate_fixture(tmp_path)
            provenance_path = self.copy_provenance(tmp_path)
            self.relabel_provenance_run_completed(provenance_path)
            self.relabel_generated_outputs_like_pre_fix_terminal_claim(paths)

            result = self.run_validator(paths, provenance_path)

        self.assertEqual(result.returncode, 1)
        self.assertIn("terminal/non-bounded finishable evidence claims require complete terminal_outcome_accounting", result.stderr)
        self.assertIn("terminal_outcome_accounting is required", result.stderr)
        self.assertIn("non_terminal", result.stderr)
        self.assertIn("non_terminal_queued", result.stderr)

    def test_rejects_requested_bucket_count_drift_for_all_primary_and_diagnostic_buckets(self) -> None:
        count_keys = (
            "completed_success_count",
            "failed_count",
            "timed_out_count",
            "skipped_count",
            "cancelled_count",
            "non_terminal_count",
            "missing_job_count",
            "missing_artifact_for_completed_success_job_count",
            "missing_outcome_row_for_completed_success_job_count",
            "missing_outcome_row_count",
        )
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            paths = self.generate_fixture(tmp_path)
            for count_key in count_keys:
                with self.subTest(count_key=count_key):
                    case_path = tmp_path / count_key
                    case_path.mkdir()
                    provenance_path = self.copy_provenance(case_path)
                    provenance = self.load_json(provenance_path)
                    requested = dict(provenance["requested_plan_outcome_buckets"])
                    requested[count_key] += 1
                    provenance["requested_plan_outcome_buckets"] = requested
                    self.write_json(provenance_path, provenance)

                    result = self.run_validator(paths, provenance_path)

                    self.assertEqual(result.returncode, 1)
                    self.assertIn(count_key, result.stderr)
                    self.assertIn("must match range cardinality", result.stderr)

    def test_rejects_incomplete_terminal_outcome_data(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            paths = self.generate_fixture(tmp_path)
            provenance_path = self.copy_provenance(tmp_path)
            provenance = self.load_json(provenance_path)
            outcome_summary = dict(provenance["outcome_summary"])
            rows = [row for row in outcome_summary["rows"] if row["discovery_plan_index"] != "474"]
            outcome_summary["rows"] = rows
            outcome_summary["terminal_outcome_row_count"] = len(rows)
            counts = dict(outcome_summary["terminal_outcome_counts"])
            counts["success"] -= 1
            outcome_summary["terminal_outcome_counts"] = counts
            outcome_summary["terminal_outcome_plan_index_ranges"] = {
                "success": ["1", "3", "6", "9", "237-238", "473", "475-482", "720-722", "955-958"],
                "timed_out": outcome_summary["terminal_outcome_plan_index_ranges"]["timed_out"],
            }
            provenance["outcome_summary"] = outcome_summary
            self.write_json(provenance_path, provenance)

            result = self.run_validator(paths, provenance_path)

        self.assertEqual(result.returncode, 1)
        self.assertIn("terminal outcome rows must exactly match terminal requested buckets", result.stderr)
        self.assertIn("missing=['474']", result.stderr)

    def test_rejects_fabricated_outcome_row_for_non_terminal_plan(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            paths = self.generate_fixture(tmp_path)
            provenance_path = self.copy_provenance(tmp_path)
            provenance = self.load_json(provenance_path)
            plan2 = self.plan_entry(2)
            outcome_summary = dict(provenance["outcome_summary"])
            rows = list(outcome_summary["rows"])
            fabricated = dict(rows[0])
            fabricated.update(
                {
                    "benchmark": plan2["workflow_benchmark_name"],
                    "candidate_size": plan2["sizes"],
                    "size_label": plan2["size_label"],
                    "terminal_outcome": "success",
                    "sim_result_state": "sim_row_emitted",
                    "exit_code": "0",
                    "detail": "fabricated_non_terminal_result",
                    "discovery_plan_index": "2",
                    "discovery_hardware_kernel_name": plan2["hardware_kernel_name"],
                    "discovery_hardware_problem_size": plan2["hardware_problem_size"],
                    "discovery_artifact_id": plan2["artifact_id"],
                }
            )
            rows.append(fabricated)
            outcome_summary["rows"] = rows
            outcome_summary["terminal_outcome_row_count"] = len(rows)
            counts = dict(outcome_summary["terminal_outcome_counts"])
            counts["success"] += 1
            outcome_summary["terminal_outcome_counts"] = counts
            outcome_summary["terminal_outcome_plan_index_ranges"] = {
                "success": ["1-3", "6", "9", "237-238", "473-482", "720-722", "955-958"],
                "timed_out": outcome_summary["terminal_outcome_plan_index_ranges"]["timed_out"],
            }
            provenance["outcome_summary"] = outcome_summary
            self.write_json(provenance_path, provenance)

            result = self.run_validator(paths, provenance_path)

        self.assertEqual(result.returncode, 1)
        self.assertIn("terminal outcome rows must exactly match terminal requested buckets", result.stderr)
        self.assertIn("fabricated=['2']", result.stderr)

    def test_rejects_false_no_results_invented_guard(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            paths = self.generate_fixture(tmp_path)
            provenance_path = self.copy_provenance(tmp_path)
            provenance = self.load_json(provenance_path)
            requested = dict(provenance["requested_plan_outcome_buckets"])
            requested["no_results_invented_for_non_terminal_or_missing_rows"] = False
            provenance["requested_plan_outcome_buckets"] = requested
            self.write_json(provenance_path, provenance)

            result = self.run_validator(paths, provenance_path)

        self.assertEqual(result.returncode, 1)
        self.assertIn("no_results_invented_for_non_terminal_or_missing_rows must be true", result.stderr)


if __name__ == "__main__":
    unittest.main()
