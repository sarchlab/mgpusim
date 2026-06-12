from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from typing import Any


REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPT = REPO_ROOT / "scripts" / "validate_mi300a_discovery_provenance.py"
PLAN = REPO_ROOT / "benchmark-comparison" / "mi300a_problem_size_discovery_plan.json"
PROVENANCE_DIR = REPO_ROOT / "benchmark-comparison" / "provenance"
TMP_ROOT = REPO_ROOT / ".repo_tmp" / "mi300a_discovery_provenance_tests"
EXPECTED_BRANCH = "e5-m3-2-refresh-1416-entry-discovery-evidence"
EXPECTED_HEAD_SHA = "0123456789abcdef0123456789abcdef01234567"


class ValidateMI300ADiscoveryProvenanceTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        TMP_ROOT.mkdir(parents=True, exist_ok=True)

    def run_validator(self, *provenance_paths: Path, plan: Path = PLAN) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [
                sys.executable,
                str(SCRIPT),
                "--plan",
                str(plan),
                "--provenance",
                *[str(path) for path in provenance_paths],
            ],
            cwd=REPO_ROOT,
            text=True,
            capture_output=True,
            check=False,
            timeout=30,
        )

    def test_valid_m3_2_e5_provenance_passes_and_reports_hotspot_48_from_plan(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            provenance_path = Path(tmpdir) / "mi300a_problem_size_discovery_run_24999999999.json"
            provenance_path.write_text(json.dumps(self.valid_provenance(), indent=2) + "\n", encoding="utf-8")

            result = self.run_validator(provenance_path)

        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertEqual(result.stderr, "")
        report = json.loads(result.stdout)
        self.assertEqual(report["schema_name"], "mgpusim.mi300a_discovery_provenance_static_check")
        self.assertEqual(report["schema_version"], 1)
        self.assertEqual(report["expected"]["expected_entry_count"], 1416)
        self.assertEqual(report["expected"]["expected_represented_count"], 1416)
        self.assertEqual(report["expected"]["expected_missing_count"], 0)
        self.assertEqual(report["expected"]["expected_skipped_count"], 0)
        self.assertEqual(report["plan"]["entry_count"], 1416)
        self.assertEqual(report["plan"]["raw_represented"], 1416)
        self.assertEqual(report["plan"]["hotspot_48_plan_index"], 474)
        self.assertEqual(report["plan"]["hotspot_48_hardware_csv_line"], 654)
        self.assertEqual(report["plan"]["hotspot_48_sizes"], "48")
        self.assertEqual(report["provenance_count"], 1)
        self.assertEqual(report["provenance"][0]["run_id"], 24999999999)
        self.assertEqual(report["provenance"][0]["head_branch"], EXPECTED_BRANCH)

    def test_stale_e3_1415_entry_provenance_is_rejected_for_m3_2(self) -> None:
        stale = self.valid_provenance()
        stale.update({"milestone": "M3", "collection_run_id": "E3"})
        stale["run"].update(
            {
                "database_id": 24958024013,
                "head_branch": "e3-m3-add-3600s-problem-size-discovery-workflow",
                "head_sha": "367c996d45bf3aa6d08ff76a0095c02e8935f760",
                "url": "https://github.com/sarchlab/mgpusim-dev/actions/runs/24958024013",
                "status": "queued",
                "conclusion": "",
            }
        )
        stale["dispatch"].update(
            {
                "command": "gh workflow run benchmark.yml --ref e3-m3-add-3600s-problem-size-discovery-workflow -f ref=e3-m3-add-3600s-problem-size-discovery-workflow -f run_mode=problem-size-discovery",
                "selected_run_id": 24958024013,
                "dispatched_at_head_sha": "367c996d45bf3aa6d08ff76a0095c02e8935f760",
            }
        )
        stale["plan_summary"].update(
            {
                "entry_count": 1415,
                "last_plan_index": 1415,
                "tier_counts": {"1": 308, "2": 1107},
            }
        )
        stale["raw_mi300a_timing_coverage"].update(
            {
                "raw_unique_kernel_problem_size_count": 1416,
                "plan_represented_unique_kernel_problem_size_count": 1415,
                "missing_raw_measured_rows": 1,
                "skipped_exception_count": 0,
            }
        )

        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            provenance_path = Path(tmpdir) / "mi300a_problem_size_discovery_run_24958024013.json"
            provenance_path.write_text(json.dumps(stale, indent=2) + "\n", encoding="utf-8")

            result = self.run_validator(provenance_path)

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("FAIL: MI300A discovery provenance static check rejected input.", result.stderr)
        self.assertIn(".plan_summary.entry_count must be 1416, got 1415", result.stderr)
        self.assertIn(".plan_summary.last_plan_index must be 1416, got 1415", result.stderr)
        self.assertIn(".raw_mi300a_timing_coverage.represented must be 1416, got 1415", result.stderr)
        self.assertIn(".raw_mi300a_timing_coverage.missing must be 0, got 1", result.stderr)
        self.assertIn(f".run.head_branch must be '{EXPECTED_BRANCH}'", result.stderr)
        self.assertIn(".dispatch.command must include branch", result.stderr)

    def test_checked_in_m3_2_e5_provenance_files_validate_when_present(self) -> None:
        intended_paths = []
        for path in sorted(PROVENANCE_DIR.glob("mi300a_problem_size_discovery_run_*.json")):
            data = json.loads(path.read_text(encoding="utf-8"))
            run = data.get("run", {}) if isinstance(data.get("run"), dict) else {}
            if (
                data.get("milestone") == "M3.2"
                or data.get("collection_run_id") == "E5"
                or run.get("head_branch") == EXPECTED_BRANCH
            ):
                intended_paths.append(path)

        if not intended_paths:
            self.assertEqual(intended_paths, [])
            return

        result = self.run_validator(*intended_paths)
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual(report["provenance_count"], len(intended_paths))

    @staticmethod
    def valid_provenance() -> dict[str, Any]:
        return {
            "schema_version": 1,
            "purpose": "M3.2/E5 MI300A problem-size-discovery workflow_dispatch evidence snapshot.",
            "milestone": "M3.2",
            "collection_run_id": "E5",
            "run": {
                "database_id": 24999999999,
                "event": "workflow_dispatch",
                "head_branch": EXPECTED_BRANCH,
                "head_sha": EXPECTED_HEAD_SHA,
                "status": "completed",
                "conclusion": "success",
                "created_at": "2026-04-26T15:30:00Z",
                "updated_at": "2026-04-26T15:45:00Z",
                "url": "https://github.com/sarchlab/mgpusim-dev/actions/runs/24999999999",
                "workflow_name": "MI300A Benchmark",
            },
            "dispatch": {
                "command": "gh workflow run benchmark.yml --ref e5-m3-2-refresh-1416-entry-discovery-evidence -f ref=e5-m3-2-refresh-1416-entry-discovery-evidence -f run_mode=problem-size-discovery",
                "selected_run_id": 24999999999,
                "dispatched_at_head_sha": EXPECTED_HEAD_SHA,
            },
            "plan_summary": {
                "artifact": "benchmark-comparison/mi300a_problem_size_discovery_plan.json",
                "entry_count": 1416,
                "first_plan_index": 1,
                "last_plan_index": 1416,
                "tier_counts": {"1": 308, "2": 1108},
                "timeout_sec": 3600,
            },
            "raw_mi300a_timing_coverage": {
                "raw_unique_kernel_problem_size_count": 1416,
                "plan_represented_unique_kernel_problem_size_count": 1416,
                "missing_raw_measured_rows": 0,
                "skipped_exception_count": 0,
            },
            "parsed_discovery_outcomes": {
                "observed_plan_entry_count": 1416,
                "missing_plan_entry_count": 0,
                "requested_total_buckets": {
                    "completed_success": 1416,
                    "failed_crashed_missing_output_missing_kernel_time": 0,
                    "missing": 0,
                    "skipped": 0,
                    "timed_out": 0,
                },
            },
        }


if __name__ == "__main__":
    unittest.main()
