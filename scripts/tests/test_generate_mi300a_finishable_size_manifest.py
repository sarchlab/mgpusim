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
SCRIPT = REPO_ROOT / "scripts" / "generate_mi300a_finishable_size_manifest.py"
PLAN = REPO_ROOT / "benchmark-comparison" / "mi300a_problem_size_discovery_plan.json"
PROVENANCE = (
    REPO_ROOT
    / "benchmark-comparison"
    / "provenance"
    / "mi300a_problem_size_discovery_run_24959959195.json"
)
TMP_ROOT = REPO_ROOT / ".repo_tmp" / "mi300a_finishable_size_manifest_tests"


class GenerateMI300AFinishableSizeManifestTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        TMP_ROOT.mkdir(parents=True, exist_ok=True)

    def run_manifest(
        self,
        tmp_path: Path,
        provenance_path: Path = PROVENANCE,
    ) -> tuple[subprocess.CompletedProcess[str], dict[str, Path]]:
        paths = {
            "manifest": tmp_path / "manifest.json",
            "tier1": tmp_path / "tier1.json",
            "tier2": tmp_path / "tier2.json",
            "summary": tmp_path / "summary.json",
        }
        result = subprocess.run(
            [
                sys.executable,
                str(SCRIPT),
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
        return result, paths

    def copy_provenance(self, tmp_path: Path) -> Path:
        copied = tmp_path / "provenance.json"
        shutil.copyfile(PROVENANCE, copied)
        return copied

    def load_json(self, path: Path) -> Mapping[str, Any]:
        with path.open("r", encoding="utf-8") as f:
            data = json.load(f)
        self.assertIsInstance(data, dict)
        return data

    def write_json(self, path: Path, data: Mapping[str, Any]) -> None:
        path.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")

    def test_real_manifest_generation_reconciles_all_plan_rows_once(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            result, paths = self.run_manifest(Path(tmpdir))

            self.assertEqual(result.returncode, 0, msg=result.stderr)
            self.assertIn("entries=1416 eligible=23 excluded=1393", result.stdout)
            manifest = self.load_json(paths["manifest"])
            summary = self.load_json(paths["summary"])
            tier1 = self.load_json(paths["tier1"])
            tier2 = self.load_json(paths["tier2"])

        self.assertEqual(manifest["schema_name"], "mgpusim.mi300a_finishable_size_manifest")
        self.assertEqual(manifest["entry_count"], 1416)
        entries = manifest["entries"]
        self.assertEqual(len(entries), 1416)
        plan_indices = [entry["plan_index"] for entry in entries]
        self.assertEqual(plan_indices, list(range(1, 1417)))
        self.assertEqual(len(set(plan_indices)), 1416)
        self.assertEqual(manifest["source_plan"]["entry_count"], 1416)
        self.assertEqual(manifest["source_plan"]["tier_counts"], {"1": 308, "2": 1108})
        self.assertFalse(manifest["eligibility_policy"]["hardware_timing_guesses_used"])
        self.assertEqual(manifest["run"]["database_id"], 24959959195)
        self.assertEqual(manifest["run"]["observed_at_utc"], "2026-04-26T17:04:42Z")
        self.assertTrue(manifest["snapshot_contract"]["non_terminal_snapshot"])
        self.assertTrue(manifest["snapshot_contract"]["bounded_snapshot"])
        self.assertNotIn("terminal_provenance", manifest["snapshot_contract"])
        self.assertIn("observation-time evidence snapshot", manifest["snapshot_contract"]["claim_scope"])
        self.assertEqual(summary["snapshot_contract"], manifest["snapshot_contract"])

        manifest_summary = manifest["summary"]
        self.assertEqual(summary["summary"], manifest_summary)
        self.assertEqual(manifest_summary["eligible_entry_count"], 23)
        self.assertEqual(manifest_summary["excluded_entry_count"], 1393)
        self.assertEqual(manifest_summary["eligible_plan_index_ranges"], ["1", "3", "6", "9", "237-238", "473-482", "720-722", "955-958"])
        self.assertEqual(
            manifest_summary["observed_outcome_bucket_counts"],
            {"completed_success": 23, "non_terminal": 1373, "timed_out": 20},
        )
        self.assertEqual(
            manifest_summary["observed_job_status_counts"],
            {"completed": 43, "in_progress": 20, "queued": 1353},
        )
        self.assertEqual(
            manifest_summary["requested_bucket_count_reconciliation"],
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
            manifest_summary["requested_diagnostic_bucket_plan_index_ranges"],
            {
                "missing_artifact_for_completed_success_job": [],
                "missing_outcome_row_for_completed_success_job": [],
                "missing_outcome_row": [
                    "2",
                    "4-5",
                    "7-8",
                    "10-236",
                    "240-241",
                    "247",
                    "250-472",
                    "487-719",
                    "723-944",
                    "946",
                    "949",
                    "959-1416",
                ],
            },
        )
        self.assertEqual(manifest_summary["tier_counts"]["1"]["eligible"], 6)
        self.assertEqual(manifest_summary["tier_counts"]["2"]["eligible"], 17)

        def entry(plan_index: int) -> Mapping[str, Any]:
            return entries[plan_index - 1]

        hotspot_48 = entry(474)
        self.assertEqual(hotspot_48["benchmark"], "hotspot")
        self.assertEqual(hotspot_48["problem_size"], "48")
        self.assertEqual(hotspot_48["observed_outcome_bucket"], "completed_success")
        self.assertEqual(hotspot_48["observed_outcome"], "success")
        self.assertTrue(hotspot_48["eligible_for_linear_workflow"])
        self.assertIsNone(hotspot_48["exclusion_reason"])
        self.assertEqual(hotspot_48["duration_sec"], 8)
        self.assertEqual(hotspot_48["discovery_job_database_id"], 73084795312)
        self.assertEqual(hotspot_48["artifact_database_id"], 6648375327)

        timed_out = entry(239)
        self.assertEqual(timed_out["benchmark"], "occupancy_fma")
        self.assertEqual(timed_out["observed_job_status"], "completed")
        self.assertEqual(timed_out["observed_job_conclusion"], "success")
        self.assertEqual(timed_out["observed_outcome_bucket"], "timed_out")
        self.assertEqual(timed_out["observed_outcome"], "timed_out")
        self.assertFalse(timed_out["eligible_for_linear_workflow"])
        self.assertEqual(timed_out["exclusion_reason"], "timed_out")
        self.assertGreaterEqual(timed_out["duration_sec"], 3600)
        self.assertEqual(timed_out["artifact_database_id"], 6648751404)

        tier1_success = entry(1)
        self.assertEqual(tier1_success["observed_job_status"], "completed")
        self.assertEqual(tier1_success["observed_outcome_bucket"], "completed_success")
        self.assertEqual(tier1_success["observed_outcome"], "success")
        self.assertTrue(tier1_success["eligible_for_linear_workflow"])
        self.assertIsNone(tier1_success["exclusion_reason"])
        self.assertIsNotNone(tier1_success["discovery_job_database_id"])

        queued = entry(2)
        self.assertEqual(queued["observed_job_status"], "queued")
        self.assertEqual(queued["observed_outcome_bucket"], "non_terminal")
        self.assertEqual(queued["observed_outcome"], "non_terminal")
        self.assertFalse(queued["eligible_for_linear_workflow"])
        self.assertEqual(queued["exclusion_reason"], "non_terminal_queued")
        self.assertIsNone(queued["discovery_job_database_id"])

        in_progress = entry(10)
        self.assertEqual(in_progress["observed_job_status"], "in_progress")
        self.assertEqual(in_progress["exclusion_reason"], "non_terminal_in_progress")

        self.assertFalse(entry(953)["eligible_for_linear_workflow"])
        self.assertEqual(entry(953)["observed_outcome_bucket"], "timed_out")
        self.assertEqual(entry(954)["observed_outcome_bucket"], "timed_out")

        self.assertEqual(tier1["schema_name"], "mgpusim.mi300a_finishable_tier_matrix_view")
        self.assertEqual(tier1["tier"], 1)
        self.assertEqual(tier1["eligible_entry_count"], 6)
        self.assertEqual(tier1["excluded_entry_count"], 302)
        self.assertEqual(tier1["eligible_plan_index_ranges"], ["1", "3", "6", "9", "237-238"])
        self.assertEqual([row["benchmark"] for row in tier1["include"]], ["mem_latency_chase", "occupancy_fma"])

        self.assertEqual(tier2["schema_name"], "mgpusim.mi300a_finishable_tier_matrix_view")
        self.assertEqual(tier2["tier"], 2)
        self.assertEqual(tier2["eligible_entry_count"], 17)
        self.assertEqual(tier2["eligible_plan_index_ranges"], ["473-482", "720-722", "955-958"])
        self.assertEqual([row["benchmark"] for row in tier2["include"]], ["hotspot", "gesummv", "srad"])
        self.assertEqual(tier2["include"][0]["finishable_plan_index_ranges"], ["473-482"])
        self.assertEqual(tier2["include"][1]["sizes"], "64 128 192")
        self.assertEqual(tier2["include"][2]["finishable_plan_index_ranges"], ["955-958"])

    def test_rejects_primary_bucket_coverage_gaps(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            provenance_path = self.copy_provenance(tmp_path)
            data = dict(self.load_json(provenance_path))
            requested = dict(data["requested_plan_outcome_buckets"])
            requested["non_terminal_count"] -= 1
            requested["non_terminal_plan_index_ranges"] = [
                "4-5",
                "7-8",
                "10-236",
                "240-241",
                "247",
                "250-472",
                "487-719",
                "723-944",
                "946",
                "949",
                "959-1416",
            ]
            requested["non_terminal_queued_count"] -= 1
            requested["non_terminal_queued_plan_index_ranges"] = [
                "4-5",
                "7-8",
                "11",
                "14-236",
                "251-472",
                "487-708",
                "723-944",
                "959-1416",
            ]
            requested["missing_outcome_row_count"] -= 1
            requested["missing_outcome_row_plan_index_ranges"] = requested["non_terminal_plan_index_ranges"]
            data["requested_plan_outcome_buckets"] = requested
            self.write_json(provenance_path, data)

            result, _ = self.run_manifest(tmp_path, provenance_path)

        self.assertEqual(result.returncode, 1)
        self.assertIn("primary outcome buckets must cover every plan entry exactly once", result.stderr)
        self.assertIn("missing=['2']", result.stderr)

    def test_rejects_non_terminal_run_without_bounded_snapshot_flag(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            provenance_path = self.copy_provenance(tmp_path)
            data = dict(self.load_json(provenance_path))
            run = dict(data["run"])
            self.assertEqual(run["status"], "queued")
            run["non_terminal_snapshot"] = False
            data["run"] = run
            self.write_json(provenance_path, data)

            result, _ = self.run_manifest(tmp_path, provenance_path)

        self.assertEqual(result.returncode, 1)
        self.assertIn("run.non_terminal_snapshot must be true when run.status is non-terminal", result.stderr)

    def test_rejects_relabelled_bounded_snapshot_as_completed_without_terminal_accounting(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            provenance_path = self.copy_provenance(tmp_path)
            data = dict(self.load_json(provenance_path))
            run = dict(data["run"])
            self.assertEqual(run["status"], "queued")
            self.assertTrue(run["non_terminal_snapshot"])
            run["status"] = "completed"
            run["conclusion"] = "success"
            run["non_terminal_snapshot"] = False
            data["run"] = run
            self.write_json(provenance_path, data)

            result, _ = self.run_manifest(tmp_path, provenance_path)

        self.assertEqual(result.returncode, 1)
        self.assertIn("terminal/non-bounded finishable evidence claims require complete terminal_outcome_accounting", result.stderr)
        self.assertIn("terminal_outcome_accounting is required", result.stderr)
        self.assertIn("non_terminal", result.stderr)
        self.assertIn("non_terminal_queued", result.stderr)

    def test_accepts_synthetic_complete_terminal_accounting_provenance(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            provenance_path = write_synthetic_complete_terminal_provenance(tmp_path)

            result, paths = self.run_manifest(tmp_path, provenance_path)

            self.assertEqual(result.returncode, 0, msg=result.stderr)
            manifest = self.load_json(paths["manifest"])
            summary = self.load_json(paths["summary"])

        self.assertFalse(manifest["run"]["non_terminal_snapshot"])
        self.assertFalse(manifest["snapshot_contract"]["non_terminal_snapshot"])
        self.assertFalse(manifest["snapshot_contract"]["bounded_snapshot"])
        self.assertTrue(manifest["snapshot_contract"]["terminal_provenance"])
        self.assertEqual(manifest["entry_count"], 1416)
        self.assertEqual(manifest["summary"]["eligible_entry_count"], 23)
        self.assertEqual(manifest["summary"]["excluded_entry_count"], 1393)
        self.assertEqual(
            manifest["summary"]["observed_outcome_bucket_counts"],
            {"completed_success": 23, "timed_out": 1393},
        )
        reconciliation = manifest["summary"]["requested_bucket_count_reconciliation"]
        self.assertEqual(reconciliation["terminal_bucket_total"], 1416)
        self.assertEqual(reconciliation["primary_bucket_counts"]["non_terminal"], 0)
        self.assertEqual(reconciliation["diagnostic_bucket_counts"]["missing_outcome_row"], 0)
        self.assertEqual(summary["snapshot_contract"], manifest["snapshot_contract"])

    def test_rejects_terminal_source_policy_top_level_conflicts_before_outputs(self) -> None:
        def set_path(data: dict[str, Any], path: tuple[str, ...], value: Any) -> None:
            target: dict[str, Any] = data
            for key in path[:-1]:
                nested = target[key]
                self.assertIsInstance(nested, dict)
                target = nested
            target[path[-1]] = value

        wrong_sha = "abcdef0123456789abcdef0123456789abcdef01"
        conflict_cases: tuple[tuple[str, tuple[str, ...], Any, str], ...] = (
            (
                "run_id",
                ("run", "database_id"),
                24959959196,
                "source_policy expected run_id conflicts",
            ),
            ("branch", ("branch",), "wrong-branch", "source_policy expected branch conflicts"),
            (
                "run_head_branch",
                ("run", "head_branch"),
                "wrong-branch",
                "source_policy expected branch conflicts",
            ),
            (
                "dispatch_ref",
                ("dispatch", "ref"),
                "wrong-ref",
                "source_policy expected branch conflicts",
            ),
            ("head_sha", ("run", "head_sha"), wrong_sha, "source_policy expected head_sha conflicts"),
            (
                "workflow",
                ("dispatch", "workflow"),
                ".github/workflows/wrong.yml",
                "source_policy expected workflow conflicts",
            ),
            ("milestone", ("milestone",), "run-conflict", "source_policy expected milestone conflicts"),
            ("collection_run_id", ("collection_run_id",), "run-conflict", "source_policy expected collection_run_id conflicts"),
            ("issue", ("issue",), 999, "source_policy expected issue conflicts"),
        )
        for case_name, path, value, expected_message in conflict_cases:
            with self.subTest(case=case_name):
                with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
                    tmp_path = Path(tmpdir)
                    provenance_path = write_synthetic_complete_terminal_provenance(tmp_path)
                    data = dict(self.load_json(provenance_path))
                    set_path(data, path, value)
                    self.write_json(provenance_path, data)

                    result, paths = self.run_manifest(tmp_path, provenance_path)
                    output_exists = {name: path.exists() for name, path in paths.items()}

                self.assertEqual(result.returncode, 1)
                self.assertIn(expected_message, result.stderr)
                self.assertTrue(all(not exists for exists in output_exists.values()), output_exists)

    def test_rejects_terminal_source_policy_missing_or_empty_head_sha_before_outputs(self) -> None:
        def remove_path(data: dict[str, Any], path: tuple[str, ...]) -> None:
            target: dict[str, Any] = data
            for key in path[:-1]:
                nested = target[key]
                self.assertIsInstance(nested, dict)
                target = nested
            target.pop(path[-1], None)

        def set_path(data: dict[str, Any], path: tuple[str, ...], value: Any) -> None:
            target: dict[str, Any] = data
            for key in path[:-1]:
                nested = target[key]
                self.assertIsInstance(nested, dict)
                target = nested
            target[path[-1]] = value

        cases: tuple[tuple[str, tuple[tuple[str, ...], ...], tuple[tuple[tuple[str, ...], Any], ...], str], ...] = (
            (
                "missing_expected_head_sha",
                (("source_policy", "expected_source_identity", "head_sha"),),
                (),
                "source_policy.expected_source_identity.head_sha must be a non-empty string",
            ),
            (
                "empty_expected_head_sha",
                (),
                ((("source_policy", "expected_source_identity", "head_sha"), ""),),
                "source_policy.expected_source_identity.head_sha must be a non-empty string",
            ),
            (
                "missing_run_head_sha",
                (("run", "head_sha"),),
                (),
                "run.head_sha must be a non-empty string",
            ),
            (
                "empty_run_head_sha",
                (),
                ((("run", "head_sha"), ""),),
                "run.head_sha must be a non-empty string",
            ),
            (
                "tampered_both_head_sha_removed",
                (("source_policy", "expected_source_identity", "head_sha"), ("run", "head_sha")),
                (),
                "source_policy.expected_source_identity.head_sha must be a non-empty string",
            ),
            (
                "tampered_both_head_sha_empty",
                (),
                ((("source_policy", "expected_source_identity", "head_sha"), ""), (("run", "head_sha"), "")),
                "source_policy.expected_source_identity.head_sha must be a non-empty string",
            ),
        )
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            valid_path = write_synthetic_complete_terminal_provenance(tmp_path)
            valid_provenance = dict(self.load_json(valid_path))

            for case_name, removals, updates, expected_message in cases:
                with self.subTest(case=case_name):
                    case_dir = tmp_path / case_name
                    case_dir.mkdir()
                    provenance_path = case_dir / "provenance.json"
                    tampered = json.loads(json.dumps(valid_provenance))
                    for path in removals:
                        remove_path(tampered, path)
                    for path, value in updates:
                        set_path(tampered, path, value)
                    self.write_json(provenance_path, tampered)

                    result, paths = self.run_manifest(case_dir, provenance_path)
                    output_exists = {name: path.exists() for name, path in paths.items()}

                    self.assertEqual(result.returncode, 1)
                    self.assertIn(expected_message, result.stderr)
                    self.assertTrue(all(not exists for exists in output_exists.values()), output_exists)

    def test_rejects_duplicate_terminal_outcome_rows(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            provenance_path = self.copy_provenance(tmp_path)
            data = dict(self.load_json(provenance_path))
            outcome_summary = dict(data["outcome_summary"])
            rows = list(outcome_summary["rows"])
            rows.append(dict(rows[0]))
            outcome_summary["rows"] = rows
            outcome_summary["terminal_outcome_row_count"] = len(rows)
            counts = dict(outcome_summary["terminal_outcome_counts"])
            counts[rows[0]["terminal_outcome"]] += 1
            outcome_summary["terminal_outcome_counts"] = counts
            data["outcome_summary"] = outcome_summary
            self.write_json(provenance_path, data)

            result, _ = self.run_manifest(tmp_path, provenance_path)

        self.assertEqual(result.returncode, 1)
        self.assertIn("duplicate terminal outcome row for discovery_plan_index", result.stderr)

    def test_rejects_missing_terminal_job_provenance(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            provenance_path = self.copy_provenance(tmp_path)
            data = dict(self.load_json(provenance_path))
            job_summary = dict(data["terminal_discovery_job_summary"])
            entries = [entry for entry in job_summary["entries"] if entry["plan_index"] != 474]
            job_summary["entries"] = entries
            job_summary["completed_job_count"] = len(entries)
            data["terminal_discovery_job_summary"] = job_summary
            self.write_json(provenance_path, data)

            result, _ = self.run_manifest(tmp_path, provenance_path)

        self.assertEqual(result.returncode, 1)
        self.assertIn("terminal requested plan entries are missing terminal job provenance", result.stderr)
        self.assertIn("474", result.stderr)

    def test_rejects_terminal_outcome_rows_that_do_not_match_declared_missing_ranges(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            provenance_path = self.copy_provenance(tmp_path)
            data = dict(self.load_json(provenance_path))
            outcome_summary = dict(data["outcome_summary"])
            rows = [row for row in outcome_summary["rows"] if row["discovery_plan_index"] != "720"]
            outcome_summary["rows"] = rows
            outcome_summary["terminal_outcome_row_count"] = len(rows)
            counts = dict(outcome_summary["terminal_outcome_counts"])
            counts["success"] -= 1
            outcome_summary["terminal_outcome_counts"] = counts
            outcome_summary["terminal_outcome_plan_index_ranges"] = {
                "success": ["1", "3", "6", "9", "237-238", "473-482", "721-722", "955-958"],
                "timed_out": ["239", "242-246", "248-249", "483-486", "945", "947-948", "950-954"],
            }
            data["outcome_summary"] = outcome_summary
            self.write_json(provenance_path, data)

            result, _ = self.run_manifest(tmp_path, provenance_path)

        self.assertEqual(result.returncode, 1)
        self.assertIn("terminal outcome rows must cover every terminal requested bucket", result.stderr)
        self.assertIn("720", result.stderr)


if __name__ == "__main__":
    unittest.main()
