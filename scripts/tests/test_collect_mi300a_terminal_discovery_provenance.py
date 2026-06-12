from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from typing import Any, Mapping


REPO_ROOT = Path(__file__).resolve().parents[2]
PLAN = REPO_ROOT / "benchmark-comparison" / "mi300a_problem_size_discovery_plan.json"
RECOVERY_MANIFEST = REPO_ROOT / "benchmark-comparison" / "generated" / "mi300a_terminal_discovery_run_25111815292_recovery_manifest.json"
COLLECTOR = REPO_ROOT / "scripts" / "collect_mi300a_terminal_discovery_provenance.py"
TERMINAL_OPERATOR = REPO_ROOT / "scripts" / "terminal_discovery_operator.py"
VALIDATOR = REPO_ROOT / "scripts" / "validate_mi300a_terminal_discovery_provenance.py"
FINISHABLE_GENERATOR = REPO_ROOT / "scripts" / "generate_mi300a_finishable_size_manifest.py"
FINISHABLE_VALIDATOR = REPO_ROOT / "scripts" / "validate_mi300a_finishable_size_manifest.py"
TMP_ROOT = REPO_ROOT / ".repo_tmp" / "mi300a_terminal_provenance_tests"


class CollectMI300ATerminalDiscoveryProvenanceTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        TMP_ROOT.mkdir(parents=True, exist_ok=True)
        with PLAN.open("r", encoding="utf-8") as f:
            cls.plan = json.load(f)
        manifest = json.loads(
            (REPO_ROOT / "benchmark-comparison" / "generated" / "mi300a_terminal_discovery_shard_manifest.json").read_text(
                encoding="utf-8"
            )
        )
        cls.shard_by_plan = {}
        for shard in manifest["shards"]:
            shard_index = int(shard["shard_index"])
            for plan_range in shard["plan_index_ranges"]:
                if "-" in plan_range:
                    start, end = [int(part) for part in plan_range.split("-", 1)]
                else:
                    start = end = int(plan_range)
                for plan_index in range(start, end + 1):
                    cls.shard_by_plan[plan_index] = shard_index
        recovery = json.loads(RECOVERY_MANIFEST.read_text(encoding="utf-8"))
        cls.recovery_missing_indices = set(cls.parse_ranges(recovery["missing_plan_index_ranges"]))
        cls.recovery_prior_observed_indices = set(
            cls.parse_ranges(recovery["prior_incomplete_evidence"]["observed_plan_index_ranges"])
        )

    @staticmethod
    def parse_ranges(ranges: list[str]) -> list[int]:
        values: list[int] = []
        for range_text in ranges:
            if "-" in range_text:
                start, end = [int(part) for part in range_text.split("-", 1)]
            else:
                start = end = int(range_text)
            values.extend(range(start, end + 1))
        return values

    def outcome_record_for_entry(
        self, entry: Mapping[str, Any], *, override: Mapping[int, Mapping[str, Any]] | None = None
    ) -> dict[str, Any]:
        override = override or {}
        status_by_index = {
            1: "success",
            2: "timeout",
            3: "crash",
            4: "skipped",
            5: "cancelled",
            474: "success",
        }
        plan_index = int(entry["plan_index"])
        status = status_by_index.get(plan_index, "success")
        outcome = {
            "schema_name": "mgpusim.mi300a_terminal_discovery_outcome",
            "schema_version": 1,
            "terminal_discovery": True,
            "terminal": True,
            "run_id": 123456,
            "run_url": "https://example.test/actions/runs/123456",
            "branch": "e15-m9-add-terminal-mi300a-discovery-dispatch-surface",
            "head_sha": "0123456789abcdef0123456789abcdef01234567",
            "workflow": ".github/workflows/mi300a_terminal_discovery.yml",
            "milestone": "M9",
            "collection_run_id": "run-abc123",
            "issue": 115,
            "status": status,
            "started_at": "2026-04-26T00:00:00Z",
            "finished_at": "2026-04-26T00:00:05Z",
            "shard_index": self.shard_by_plan[plan_index],
            "plan_index": plan_index,
            "runnable_unit_id": f"mi300a-terminal-discovery-plan-{plan_index:04d}",
            "benchmark": entry["workflow_benchmark_name"],
            "hardware_kernel_name": entry["hardware_kernel_name"],
            "problem_size": entry["problem_size"],
            "sizes": entry["sizes"],
            "timeout_sec": 3600,
            "exit_code": 0 if status == "success" else (124 if status == "timeout" else 1),
            "kernel_time_ms": 1.0 if status == "success" else None,
            "flags": "",
            "gomemlimit": "4GiB",
            "gogc": "50",
            "artifacts": {},
        }
        if plan_index in override:
            outcome.update(override[plan_index])
        return outcome

    def write_outcome_fixture(
        self,
        root: Path,
        *,
        override: Mapping[int, Mapping[str, Any]] | None = None,
        plan_indices: set[int] | None = None,
    ) -> None:
        for entry in self.plan["entries"]:
            plan_index = int(entry["plan_index"])
            if plan_indices is not None and plan_index not in plan_indices:
                continue
            outcome = self.outcome_record_for_entry(entry, override=override)
            artifact_dir = root / f"mi300a-terminal-discovery-plan-{plan_index}"
            artifact_dir.mkdir(parents=True, exist_ok=True)
            (artifact_dir / f"outcome_mi300a_terminal_discovery_plan_{plan_index:04d}.json").write_text(
                json.dumps(outcome, indent=2) + "\n",
                encoding="utf-8",
            )

    def write_benchmark_outcome_fixture(
        self, root: Path, *, override: Mapping[int, Mapping[str, Any]] | None = None
    ) -> None:
        by_benchmark: dict[str, list[dict[str, Any]]] = {}
        for entry in self.plan["entries"]:
            outcome = self.outcome_record_for_entry(entry, override=override)
            benchmark = entry["workflow_benchmark_name"]
            by_benchmark.setdefault(benchmark, []).append(outcome)

        for benchmark, outcomes in by_benchmark.items():
            shard_index = self.shard_by_plan[int(outcomes[0]["plan_index"])]
            artifact_dir = root / f"mi300a-terminal-discovery-benchmark-{benchmark}"
            artifact_dir.mkdir(parents=True, exist_ok=True)
            benchmark_doc = {
                "schema_name": "mgpusim.mi300a_terminal_discovery_benchmark_outcomes",
                "schema_version": 1,
                "terminal_discovery": True,
                "terminal": True,
                "artifact_name": f"mi300a-terminal-discovery-benchmark-{benchmark}",
                "benchmark": benchmark,
                "workflow_benchmark_name": benchmark,
                "shard_index": shard_index,
                "timeout_sec": 3600,
                "per_size_outcome_count": len(outcomes),
                "per_size_plan_index_start": outcomes[0]["plan_index"],
                "per_size_plan_index_end": outcomes[-1]["plan_index"],
                "outcomes": outcomes,
            }
            (artifact_dir / f"outcomes_mi300a_terminal_discovery_benchmark_{benchmark}.json").write_text(
                json.dumps(benchmark_doc, indent=2) + "\n",
                encoding="utf-8",
            )

    def run_collect(
        self,
        outcomes_root: Path,
        output: Path,
        *,
        head_sha: str | None = "0123456789abcdef0123456789abcdef01234567",
    ) -> subprocess.CompletedProcess[str]:
        args = [
            sys.executable,
            str(COLLECTOR),
            "--outcomes-root",
            str(outcomes_root),
            "--output",
            str(output),
            "--run-id",
            "123456",
            "--run-url",
            "https://example.test/actions/runs/123456",
            "--head-branch",
            "e15-m9-add-terminal-mi300a-discovery-dispatch-surface",
            "--milestone",
            "M9",
            "--epoch",
            "run-abc123",
            "--issue",
            "115",
        ]
        if head_sha is not None:
            args.extend(["--head-sha", head_sha])
        args.extend(
            [
                "--created-at",
                "2026-04-26T00:00:00Z",
                "--updated-at",
                "2026-04-26T00:10:00Z",
                "--collected-at",
                "2026-04-26T00:10:00Z",
            ]
        )
        return subprocess.run(
            args,
            cwd=REPO_ROOT,
            text=True,
            capture_output=True,
            check=False,
            timeout=60,
        )

    def run_validate(self, provenance_path: Path) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [sys.executable, str(VALIDATOR), "--provenance", str(provenance_path)],
            cwd=REPO_ROOT,
            text=True,
            capture_output=True,
            check=False,
            timeout=60,
        )

    def run_count_outcomes(self, outcomes_root: Path) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [
                sys.executable,
                str(TERMINAL_OPERATOR),
                "count-outcomes",
                "--outcomes-root",
                str(outcomes_root),
            ],
            cwd=REPO_ROOT,
            text=True,
            capture_output=True,
            check=False,
            timeout=60,
        )

    def output_paths(self, tmp_path: Path) -> dict[str, Path]:
        return {
            "manifest": tmp_path / "mi300a_finishable_size_manifest.json",
            "tier1": tmp_path / "mi300a_finishable_tier1_matrix.json",
            "tier2": tmp_path / "mi300a_finishable_tier2_matrix.json",
            "summary": tmp_path / "mi300a_finishable_size_summary.json",
        }

    def test_collects_validates_and_regenerates_finishable_manifest_from_terminal_provenance(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            outcomes_root = tmp_path / "outcomes"
            self.write_outcome_fixture(outcomes_root)
            provenance_path = tmp_path / "terminal_provenance.json"

            collect = self.run_collect(outcomes_root, provenance_path)
            self.assertEqual(collect.returncode, 0, msg=collect.stderr)
            collect_report = json.loads(collect.stdout)
            provenance = json.loads(provenance_path.read_text(encoding="utf-8"))

            self.assertEqual(collect_report["status"], "pass")
            self.assertEqual(collect_report["terminal_outcome_row_count"], 1416)
            self.assertEqual(collect_report["terminal_outcome_accounting"]["record_count"], 1416)
            self.assertEqual(
                collect_report["requested_bucket_count_reconciliation"]["primary_bucket_counts"],
                {
                    "completed_success": 1412,
                    "failed": 1,
                    "timed_out": 1,
                    "skipped": 1,
                    "cancelled": 1,
                    "non_terminal": 0,
                    "missing_job": 0,
                },
            )
            self.assertFalse(provenance["run"]["non_terminal_snapshot"])
            self.assertEqual(
                provenance["source_policy"]["expected_source_identity"]["head_sha"],
                "0123456789abcdef0123456789abcdef01234567",
            )
            self.assertTrue(provenance["requested_plan_outcome_buckets"]["terminal_mode"])
            self.assertEqual(provenance["terminal_outcome_accounting"]["record_count"], 1416)
            self.assertEqual(len(provenance["terminal_outcome_accounting"]["records"]), 1416)

            validation = subprocess.run(
                [sys.executable, str(VALIDATOR), "--provenance", str(provenance_path)],
                cwd=REPO_ROOT,
                text=True,
                capture_output=True,
                check=False,
                timeout=60,
            )
            self.assertEqual(validation.returncode, 0, msg=validation.stderr)
            validation_report = json.loads(validation.stdout)
            self.assertEqual(validation_report["status"], "pass")
            self.assertEqual(validation_report["artifact_entry_count"], 1416)
            self.assertEqual(validation_report["terminal_job_entry_count"], 1416)

            paths = self.output_paths(tmp_path)
            generate = subprocess.run(
                [
                    sys.executable,
                    str(FINISHABLE_GENERATOR),
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
                timeout=60,
            )
            self.assertEqual(generate.returncode, 0, msg=generate.stderr)

            manifest = json.loads(paths["manifest"].read_text(encoding="utf-8"))
            summary = json.loads(paths["summary"].read_text(encoding="utf-8"))
            self.assertFalse(manifest["run"]["non_terminal_snapshot"])
            self.assertFalse(manifest["snapshot_contract"]["bounded_snapshot"])
            self.assertTrue(manifest["snapshot_contract"]["terminal_provenance"])
            self.assertIn("terminal discovery evidence", manifest["snapshot_contract"]["claim_scope"])
            self.assertIn("bounded snapshot artifacts remain historical", manifest["snapshot_contract"]["claim_scope"])
            self.assertEqual(summary["summary"]["eligible_entry_count"], 1412)
            self.assertEqual(summary["summary"]["excluded_entry_count"], 4)
            self.assertEqual(summary["summary"]["observed_outcome_bucket_counts"].get("non_terminal"), None)
            self.assertEqual(summary["summary"]["requested_bucket_count_reconciliation"]["terminal_bucket_total"], 1416)

            finishable_validation = subprocess.run(
                [
                    sys.executable,
                    str(FINISHABLE_VALIDATOR),
                    "--provenance",
                    str(provenance_path),
                    "--manifest",
                    str(paths["manifest"]),
                    "--tier1",
                    str(paths["tier1"]),
                    "--tier2",
                    str(paths["tier2"]),
                    "--summary",
                    str(paths["summary"]),
                ],
                cwd=REPO_ROOT,
                text=True,
                capture_output=True,
                check=False,
                timeout=60,
            )
            self.assertEqual(finishable_validation.returncode, 0, msg=finishable_validation.stderr)
            finishable_report = json.loads(finishable_validation.stdout)
            self.assertEqual(finishable_report["status"], "pass")
            self.assertEqual(finishable_report["provenance"]["terminal_outcome_row_count"], 1416)

    def test_collects_benchmark_level_nested_outcome_artifacts(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            outcomes_root = tmp_path / "benchmark-outcomes"
            self.write_benchmark_outcome_fixture(outcomes_root)
            provenance_path = tmp_path / "terminal_provenance.json"

            collect = self.run_collect(outcomes_root, provenance_path)
            self.assertEqual(collect.returncode, 0, msg=collect.stderr)
            collect_report = json.loads(collect.stdout)
            provenance = json.loads(provenance_path.read_text(encoding="utf-8"))

            self.assertEqual(collect_report["status"], "pass")
            self.assertEqual(collect_report["source_outcome_file_count"], 82)
            self.assertEqual(collect_report["source_outcome_record_count"], 1416)
            self.assertEqual(collect_report["terminal_outcome_row_count"], 1416)
            self.assertEqual(provenance["artifact_summary"]["source_file_count"], 82)
            self.assertEqual(provenance["artifact_summary"]["logical_terminal_outcome_artifact_count"], 1416)
            self.assertEqual(provenance["terminal_outcome_accounting"]["record_count"], 1416)
            hotspot_record = provenance["terminal_outcome_accounting"]["records"][473]
            self.assertEqual(hotspot_record["plan_index"], 474)
            self.assertIn(".outcomes[", hotspot_record["outcome_path"])

            validation = subprocess.run(
                [sys.executable, str(VALIDATOR), "--provenance", str(provenance_path)],
                cwd=REPO_ROOT,
                text=True,
                capture_output=True,
                check=False,
                timeout=60,
            )
            self.assertEqual(validation.returncode, 0, msg=validation.stderr)
            validation_report = json.loads(validation.stdout)
            self.assertEqual(validation_report["status"], "pass")
            self.assertEqual(validation_report["artifact_entry_count"], 1416)
            self.assertEqual(validation_report["terminal_job_entry_count"], 1416)

    def test_deduplicates_mixed_nested_and_standalone_outcome_shapes(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            outcomes_root = tmp_path / "mixed-outcomes"
            self.write_benchmark_outcome_fixture(outcomes_root / "benchmark")
            self.write_outcome_fixture(outcomes_root / "standalone")
            provenance_path = tmp_path / "terminal_provenance.json"

            collect = self.run_collect(outcomes_root, provenance_path)
            self.assertEqual(collect.returncode, 0, msg=collect.stderr)
            collect_report = json.loads(collect.stdout)
            provenance = json.loads(provenance_path.read_text(encoding="utf-8"))

            self.assertEqual(collect_report["status"], "pass")
            self.assertEqual(collect_report["source_outcome_file_count"], 1498)
            self.assertEqual(collect_report["raw_source_outcome_record_count"], 2832)
            self.assertEqual(collect_report["source_outcome_record_count"], 1416)
            self.assertEqual(collect_report["logical_source_outcome_record_count"], 1416)
            dedup = collect_report["outcome_source_deduplication"]
            self.assertIn("prefer benchmark-level nested records", dedup["policy"])
            self.assertEqual(dedup["preferred_shape"], "benchmark_nested")
            self.assertEqual(dedup["raw_source_record_count"], 2832)
            self.assertEqual(dedup["logical_record_count"], 1416)
            self.assertEqual(dedup["unique_plan_index_count"], 1416)
            self.assertEqual(dedup["raw_duplicate_plan_index_count"], 1416)
            self.assertEqual(dedup["deduplicated_source_record_count"], 1416)
            self.assertEqual(dedup["raw_shape_counts"], {"benchmark_nested": 1416, "standalone": 1416})
            self.assertEqual(dedup["selected_shape_counts"], {"benchmark_nested": 1416})
            self.assertEqual(dedup["discarded_shape_counts"], {"standalone": 1416})

            accounting = provenance["terminal_outcome_accounting"]
            coverage = accounting["coverage"]
            self.assertEqual(accounting["record_count"], 1416)
            self.assertEqual(len(accounting["records"]), 1416)
            self.assertEqual(coverage["record_count"], 1416)
            self.assertEqual(coverage["duplicate_plan_index_count"], 0)
            self.assertEqual(coverage["duplicate_plan_index_ranges"], [])
            self.assertEqual(coverage["missing_plan_index_count"], 0)
            self.assertEqual(coverage["extra_plan_index_count"], 0)
            plan_indices = [record["plan_index"] for record in accounting["records"]]
            self.assertEqual(plan_indices, list(range(1, 1417)))
            self.assertEqual(len(set(plan_indices)), 1416)
            self.assertTrue(all(record["timeout_sec"] == 3600 for record in accounting["records"]))
            expected_problem_size = {int(entry["plan_index"]): str(entry["problem_size"]) for entry in self.plan["entries"]}
            self.assertTrue(
                all(record["problem_size"] == expected_problem_size[record["plan_index"]] for record in accounting["records"])
            )
            hotspot_record = accounting["records"][473]
            self.assertEqual(hotspot_record["plan_index"], 474)
            self.assertEqual(hotspot_record["hardware_kernel_name"], "hotspot")
            self.assertEqual(hotspot_record["problem_size"], "48")
            self.assertEqual(hotspot_record["timeout_sec"], 3600)
            self.assertIn(".outcomes[", hotspot_record["outcome_path"])

            artifact_dedup = provenance["artifact_summary"]["outcome_source_deduplication"]
            self.assertEqual(artifact_dedup, dedup)
            self.assertEqual(provenance["artifact_summary"]["source_file_count"], 1498)
            self.assertEqual(provenance["artifact_summary"]["source_outcome_record_count"], 2832)
            self.assertEqual(provenance["artifact_summary"]["logical_terminal_outcome_artifact_count"], 1416)

            count = self.run_count_outcomes(outcomes_root)
            self.assertEqual(count.returncode, 0, msg=count.stderr)
            count_report = json.loads(count.stdout)
            self.assertTrue(count_report["complete"])
            self.assertEqual(count_report["record_count"], 1416)
            self.assertEqual(count_report["unique_plan_index_count"], 1416)
            self.assertEqual(count_report["duplicate_plan_index_count"], 0)
            self.assertEqual(count_report["duplicate_plan_indices"], [])
            self.assertEqual(count_report["raw_source_record_count"], 2832)
            self.assertEqual(count_report["raw_duplicate_plan_index_count"], 1416)
            self.assertEqual(count_report["timeout_sec"], 3600)
            self.assertTrue(count_report["plan_metadata_validated"])
            self.assertEqual(
                count_report["hotspot_48"],
                {
                    "covered": True,
                    "plan_index": 474,
                    "benchmark": "hotspot",
                    "problem_size": "48",
                    "timeout_sec": 3600,
                },
            )

    def test_rejects_conflicting_mixed_shape_duplicate_outcomes(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            outcomes_root = tmp_path / "mixed-conflict"
            self.write_benchmark_outcome_fixture(
                outcomes_root / "benchmark",
                override={474: {"status": "timeout", "detail": "conflicting timeout duplicate", "exit_code": 124}},
            )
            self.write_outcome_fixture(outcomes_root / "standalone")

            result = self.run_collect(outcomes_root, tmp_path / "terminal_provenance.json")

        self.assertEqual(result.returncode, 1)
        self.assertIn("conflicting duplicate terminal outcome records for plan_index 474", result.stderr)
        self.assertIn("terminal/provenance-critical fields", result.stderr)
        self.assertIn("status", result.stderr)

    def test_rejects_recovery_940_plus_prior_476_mixed_source_union(self) -> None:
        self.assertEqual(len(self.recovery_missing_indices), 940)
        self.assertEqual(len(self.recovery_prior_observed_indices), 476)
        prior_identity = {
            "run_id": 25111815292,
            "run_url": "https://github.com/sarchlab/mgpusim-dev/actions/runs/25111815292",
            "branch": "e31-m23-execute-278-terminal-discovery-relaunch",
            "head_sha": "8ff0b0e1674c6778c3c697ff516abb9b2e4694c1",
            "workflow": ".github/workflows/benchmark.yml",
            "milestone": "run-202604",
            "collection_run_id": "run-202604",
            "issue": 287,
        }
        prior_override = {plan_index: prior_identity for plan_index in self.recovery_prior_observed_indices}
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            outcomes_root = tmp_path / "mixed-recovery-prior"
            self.write_outcome_fixture(
                outcomes_root / "future-940-recovery",
                plan_indices=self.recovery_missing_indices,
            )
            self.write_outcome_fixture(
                outcomes_root / "prior-476-cancelled-run",
                override=prior_override,
                plan_indices=self.recovery_prior_observed_indices,
            )

            result = self.run_collect(outcomes_root, tmp_path / "terminal_provenance.json")

        self.assertEqual(result.returncode, 1)
        self.assertIn("source outcome metadata conflict for run_id", result.stderr)
        self.assertIn("single_run_terminal_source_identity", result.stderr)

    def test_rejects_source_outcome_identity_conflicts(self) -> None:
        conflict_cases = {
            "run_id": {"run_id": 654321},
            "branch": {"branch": "wrong-branch"},
            "head_sha": {"head_sha": "abcdef0123456789abcdef0123456789abcdef01"},
            "workflow": {"workflow": ".github/workflows/wrong.yml"},
            "milestone": {"milestone": "run-synthetic"},
            "issue": {"issue": 999},
        }
        for field, override in conflict_cases.items():
            with self.subTest(field=field):
                with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
                    tmp_path = Path(tmpdir)
                    outcomes_root = tmp_path / f"conflict-{field}"
                    self.write_outcome_fixture(outcomes_root, override={474: override})

                    result = self.run_collect(outcomes_root, tmp_path / "terminal_provenance.json")

                self.assertEqual(result.returncode, 1)
                self.assertIn(f"source outcome metadata conflict for {field}", result.stderr)

    def test_rejects_conflicting_non_empty_source_identity_aliases(self) -> None:
        wrong_sha = "abcdef0123456789abcdef0123456789abcdef01"
        conflict_cases = (
            ("run_database_id", "run_id", {"run_database_id": 654321}),
            ("workflow_run_id", "run_id", {"workflow_run_id": 654321}),
            ("head_branch", "branch", {"head_branch": "wrong-branch"}),
            ("ref", "branch", {"ref": "wrong-branch"}),
            ("sha", "head_sha", {"sha": wrong_sha}),
            ("commit_sha", "head_sha", {"commit_sha": wrong_sha}),
            ("dispatch_workflow", "workflow", {"dispatch_workflow": ".github/workflows/wrong.yml"}),
            ("issue_id", "issue", {"issue_id": 999}),
        )
        for alias, field, override in conflict_cases:
            with self.subTest(alias=alias):
                with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
                    tmp_path = Path(tmpdir)
                    outcomes_root = tmp_path / f"conflicting-alias-{alias}"
                    provenance_path = tmp_path / "terminal_provenance.json"
                    self.write_outcome_fixture(outcomes_root, override={474: override}, plan_indices={474})

                    result = self.run_collect(outcomes_root, provenance_path)
                    output_exists = provenance_path.exists()

                self.assertEqual(result.returncode, 1)
                self.assertIn(f"source outcome metadata conflict for {field}", result.stderr)
                self.assertIn(f"alias {alias}", result.stderr)
                self.assertFalse(output_exists)

    def test_rejects_conflicting_benchmark_container_identity_fields(self) -> None:
        wrong_sha = "abcdef0123456789abcdef0123456789abcdef01"
        conflict_cases = (
            ("run_id", "run_id", {"run_id": 654321}),
            ("run_database_id", "run_id", {"run_database_id": 654321}),
            ("workflow_run_id", "run_id", {"workflow_run_id": 654321}),
            ("branch", "branch", {"branch": "wrong-branch"}),
            ("head_branch", "branch", {"head_branch": "wrong-branch"}),
            ("ref", "branch", {"ref": "wrong-branch"}),
            ("head_sha", "head_sha", {"head_sha": wrong_sha}),
            ("sha", "head_sha", {"sha": wrong_sha}),
            ("commit_sha", "head_sha", {"commit_sha": wrong_sha}),
            ("workflow", "workflow", {"workflow": ".github/workflows/wrong.yml"}),
            ("dispatch_workflow", "workflow", {"dispatch_workflow": ".github/workflows/wrong.yml"}),
            ("milestone", "milestone", {"milestone": "run-synthetic"}),
            ("collection_run_id", "collection_run_id", {"collection_run_id": "run-synthetic"}),
            ("issue", "issue", {"issue": 999}),
            ("issue_id", "issue", {"issue_id": 999}),
        )
        for alias, field, override in conflict_cases:
            with self.subTest(alias=alias):
                with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
                    tmp_path = Path(tmpdir)
                    outcomes_root = tmp_path / f"conflicting-container-alias-{alias}"
                    provenance_path = tmp_path / "terminal_provenance.json"
                    self.write_benchmark_outcome_fixture(outcomes_root)
                    benchmark_doc_path = next(
                        outcomes_root.glob("*/outcomes_mi300a_terminal_discovery_benchmark_*.json")
                    )
                    benchmark_doc = json.loads(benchmark_doc_path.read_text(encoding="utf-8"))
                    benchmark_doc.update(override)
                    benchmark_doc_path.write_text(json.dumps(benchmark_doc, indent=2) + "\n", encoding="utf-8")

                    result = self.run_collect(outcomes_root, provenance_path)
                    output_exists = provenance_path.exists()

                self.assertEqual(result.returncode, 1)
                self.assertIn(f"source outcome metadata conflict for {field}", result.stderr)
                self.assertIn(f"alias {alias}", result.stderr)
                self.assertFalse(output_exists)

    def test_rejects_omitted_head_sha_before_conflicting_container_alias_can_write_provenance(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            outcomes_root = tmp_path / "omitted-head-sha-conflict"
            provenance_path = tmp_path / "terminal_provenance.json"
            self.write_benchmark_outcome_fixture(outcomes_root)
            benchmark_doc_path = next(
                outcomes_root.glob("*/outcomes_mi300a_terminal_discovery_benchmark_*.json")
            )
            benchmark_doc = json.loads(benchmark_doc_path.read_text(encoding="utf-8"))
            benchmark_doc["head_sha"] = "abcdef0123456789abcdef0123456789abcdef01"
            benchmark_doc_path.write_text(json.dumps(benchmark_doc, indent=2) + "\n", encoding="utf-8")

            result = self.run_collect(outcomes_root, provenance_path, head_sha=None)
            output_exists = provenance_path.exists()

        self.assertEqual(result.returncode, 1)
        self.assertIn("requested/top-level source identity missing head_sha", result.stderr)
        self.assertIn("single_run_terminal_source_identity", result.stderr)
        self.assertFalse(output_exists)

    def test_rejects_unrecognized_nested_terminal_discovery_container_before_record_overwrite(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            outcomes_root = tmp_path / "unrecognized-terminal-container"
            provenance_path = tmp_path / "terminal_provenance.json"
            self.write_benchmark_outcome_fixture(outcomes_root)
            benchmark_doc_path = next(
                outcomes_root.glob("*/outcomes_mi300a_terminal_discovery_benchmark_*.json")
            )
            benchmark_doc = json.loads(benchmark_doc_path.read_text(encoding="utf-8"))
            benchmark_doc["schema_name"] = "mgpusim.mi300a_terminal_discovery_unreviewed_container"
            benchmark_doc["run_id"] = 654321
            benchmark_doc_path.write_text(json.dumps(benchmark_doc, indent=2) + "\n", encoding="utf-8")

            result = self.run_collect(outcomes_root, provenance_path)
            output_exists = provenance_path.exists()

        self.assertEqual(result.returncode, 1)
        self.assertIn("unrecognized nested terminal_discovery container schema_name", result.stderr)
        self.assertIn("before inheriting per-size outcomes", result.stderr)
        self.assertFalse(output_exists)

    def test_container_run_id_conflict_blocks_downstream_finishable_outputs(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            outcomes_root = tmp_path / "container-run-id-conflict"
            provenance_path = tmp_path / "terminal_provenance.json"
            paths = self.output_paths(tmp_path)
            self.write_benchmark_outcome_fixture(outcomes_root)
            benchmark_doc_path = next(
                outcomes_root.glob("*/outcomes_mi300a_terminal_discovery_benchmark_*.json")
            )
            benchmark_doc = json.loads(benchmark_doc_path.read_text(encoding="utf-8"))
            benchmark_doc["run_id"] = 654321
            benchmark_doc_path.write_text(json.dumps(benchmark_doc, indent=2) + "\n", encoding="utf-8")

            collect = self.run_collect(outcomes_root, provenance_path)
            generate = subprocess.run(
                [
                    sys.executable,
                    str(FINISHABLE_GENERATOR),
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
                timeout=60,
            )
            downstream_output_exists = {name: path.exists() for name, path in paths.items()}
            provenance_exists = provenance_path.exists()

        self.assertEqual(collect.returncode, 1)
        self.assertIn("source outcome metadata conflict for run_id", collect.stderr)
        self.assertIn("alias run_id", collect.stderr)
        self.assertFalse(provenance_exists)
        self.assertEqual(generate.returncode, 1)
        self.assertIn("file not found", generate.stderr)
        self.assertFalse(any(downstream_output_exists.values()), downstream_output_exists)

    def test_validator_rejects_source_policy_top_level_conflict(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            outcomes_root = tmp_path / "outcomes"
            self.write_outcome_fixture(outcomes_root)
            provenance_path = tmp_path / "terminal_provenance.json"
            collect = self.run_collect(outcomes_root, provenance_path)
            self.assertEqual(collect.returncode, 0, msg=collect.stderr)
            provenance = json.loads(provenance_path.read_text(encoding="utf-8"))
            provenance["source_policy"]["expected_source_identity"]["issue"] = 999
            provenance_path.write_text(json.dumps(provenance, indent=2) + "\n", encoding="utf-8")

            validation = self.run_validate(provenance_path)

        self.assertEqual(validation.returncode, 1)
        self.assertIn("source_policy expected issue conflicts", validation.stderr)

    def test_validator_rejects_missing_or_empty_head_sha_in_source_policy_identity(self) -> None:
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
            outcomes_root = tmp_path / "outcomes"
            self.write_outcome_fixture(outcomes_root)
            valid_path = tmp_path / "terminal_provenance.json"
            collect = self.run_collect(outcomes_root, valid_path)
            self.assertEqual(collect.returncode, 0, msg=collect.stderr)
            valid_provenance = json.loads(valid_path.read_text(encoding="utf-8"))

            for case_name, removals, updates, expected_message in cases:
                with self.subTest(case=case_name):
                    tampered = json.loads(json.dumps(valid_provenance))
                    for path in removals:
                        remove_path(tampered, path)
                    for path, value in updates:
                        set_path(tampered, path, value)
                    tampered_path = tmp_path / f"{case_name}.json"
                    tampered_path.write_text(json.dumps(tampered, indent=2) + "\n", encoding="utf-8")

                    validation = self.run_validate(tampered_path)

                    self.assertEqual(validation.returncode, 1)
                    self.assertIn(expected_message, validation.stderr)

    def test_validator_rejects_completed_cancelled_run_conclusion(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            outcomes_root = tmp_path / "outcomes"
            self.write_outcome_fixture(outcomes_root)
            provenance_path = tmp_path / "terminal_provenance.json"
            collect = self.run_collect(outcomes_root, provenance_path)
            self.assertEqual(collect.returncode, 0, msg=collect.stderr)
            provenance = json.loads(provenance_path.read_text(encoding="utf-8"))
            provenance["run"]["conclusion"] = "cancelled"
            provenance_path.write_text(json.dumps(provenance, indent=2) + "\n", encoding="utf-8")

            validation = self.run_validate(provenance_path)

        self.assertEqual(validation.returncode, 1)
        self.assertIn("run.conclusion must be 'success'", validation.stderr)
        self.assertIn("non-success/cancelled", validation.stderr)

    def test_rejects_exact_940_only_and_476_only_scopes_as_incomplete(self) -> None:
        cases = (
            ("recovery-940-only", self.recovery_missing_indices, "logical_observed=940"),
            ("prior-476-only", self.recovery_prior_observed_indices, "logical_observed=476"),
        )
        for name, plan_indices, expected_message in cases:
            with self.subTest(name=name):
                with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
                    tmp_path = Path(tmpdir)
                    outcomes_root = tmp_path / name
                    self.write_outcome_fixture(outcomes_root, plan_indices=plan_indices)

                    result = self.run_collect(outcomes_root, tmp_path / "terminal_provenance.json")

                self.assertEqual(result.returncode, 1)
                self.assertIn("terminal provenance requires exactly one outcome per 1416-entry plan entry", result.stderr)
                self.assertIn(expected_message, result.stderr)

    def test_rejects_missing_terminal_outcome_artifact(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            outcomes_root = tmp_path / "outcomes"
            self.write_outcome_fixture(outcomes_root)
            missing = outcomes_root / "mi300a-terminal-discovery-plan-474" / "outcome_mi300a_terminal_discovery_plan_0474.json"
            missing.unlink()

            result = self.run_collect(outcomes_root, tmp_path / "terminal_provenance.json")

        self.assertEqual(result.returncode, 1)
        self.assertIn("terminal provenance requires exactly one outcome per 1416-entry plan entry", result.stderr)
        self.assertIn("missing=['474']", result.stderr)

    def test_rejects_non_terminal_status_in_terminal_mode(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            outcomes_root = tmp_path / "outcomes"
            self.write_outcome_fixture(outcomes_root, override={474: {"status": "in_progress"}})

            result = self.run_collect(outcomes_root, tmp_path / "terminal_provenance.json")

        self.assertEqual(result.returncode, 1)
        self.assertIn("is non-terminal", result.stderr)
        self.assertIn("completed/failed/timed_out/skipped/cancelled", result.stderr)


if __name__ == "__main__":
    unittest.main()
