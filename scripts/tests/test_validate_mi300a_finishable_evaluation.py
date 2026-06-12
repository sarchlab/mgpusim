from __future__ import annotations

import json
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from typing import Any, Mapping


REPO_ROOT = Path(__file__).resolve().parents[2]
VALIDATOR = REPO_ROOT / "scripts" / "validate_mi300a_finishable_evaluation.py"
FINISHABLE_MANIFEST = REPO_ROOT / "benchmark-comparison" / "mi300a_finishable_size_manifest.json"
TIER2_VIEW = REPO_ROOT / "benchmark-comparison" / "generated" / "mi300a_finishable_tier2_matrix.json"
EVALUATION_MANIFEST = REPO_ROOT / "benchmark-comparison" / "mi300a_finishable_evaluation_set.json"
EVALUATION_SUMMARY = REPO_ROOT / "benchmark-comparison" / "mi300a_finishable_evaluation_set_summary.json"
TMP_ROOT = REPO_ROOT / ".repo_tmp" / "mi300a_finishable_evaluation_validator_tests"


class ValidateMI300AFinishableEvaluationTest(unittest.TestCase):
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

    def run_validator(self, *extra_args: str | Path) -> subprocess.CompletedProcess[str]:
        command = [sys.executable, str(VALIDATOR), *(str(arg) for arg in extra_args)]
        return subprocess.run(
            command,
            cwd=REPO_ROOT,
            text=True,
            capture_output=True,
            check=False,
            timeout=30,
        )

    def generate_fixture(self, tmp_path: Path) -> tuple[Path, Path]:
        evaluation_path = tmp_path / "mi300a_finishable_evaluation_set.json"
        summary_path = tmp_path / "mi300a_finishable_evaluation_set_summary.json"
        result = self.run_validator(
            "--update",
            "--evaluation-manifest",
            evaluation_path,
            "--summary",
            summary_path,
        )
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertTrue(evaluation_path.is_file())
        self.assertTrue(summary_path.is_file())
        return evaluation_path, summary_path

    def test_checked_in_evaluation_manifest_validates_with_json_summary(self) -> None:
        result = self.run_validator()

        self.assertEqual(result.returncode, 0, msg=result.stderr)
        summary = json.loads(result.stdout)
        checked_in_summary = self.load_json(EVALUATION_SUMMARY)
        self.assertEqual(summary, checked_in_summary)
        self.assertEqual(summary["schema_name"], "mgpusim.mi300a_finishable_evaluation_summary")
        self.assertEqual(summary["validation_status"], "pass")
        self.assertEqual(summary["entry_count"], 23)
        self.assertEqual(summary["benchmark_count"], 5)
        self.assertEqual(summary["source_contracts"]["discovery_plan_entry_count"], 1416)
        self.assertEqual(summary["source_contracts"]["finishable_manifest_entry_count"], 1416)
        self.assertEqual(summary["source_contracts"]["eligible_entry_count"], 23)
        self.assertEqual(summary["source_contracts"]["timed_out_entry_count"], 20)
        self.assertEqual(summary["source_contracts"]["non_terminal_entry_count"], 1373)
        self.assertEqual(summary["tier_counts"]["tier1"]["entry_count"], 6)
        self.assertEqual(summary["tier_counts"]["tier2"]["entry_count"], 17)
        self.assertEqual(
            [item["benchmark"] for item in summary["benchmark_summaries"]],
            ["mem_latency_chase", "occupancy_fma", "hotspot", "gesummv", "srad"],
        )

        manifest = self.load_json(EVALUATION_MANIFEST)
        self.assertEqual(manifest["schema_name"], "mgpusim.mi300a_finishable_evaluation_set")
        self.assertEqual(manifest["entry_count"], 23)
        self.assertEqual(manifest["selection"]["excluded_eligible_entries"], [])
        self.assertEqual(
            [entry["plan_index"] for entry in manifest["entries"]],
            [1, 3, 6, 9, 237, 238, 473, 474, 475, 476, 477, 478, 479, 480, 481, 482, 720, 721, 722, 955, 956, 957, 958],
        )
        hotspot_48 = manifest["entries"][7]
        self.assertEqual(hotspot_48["plan_index"], 474)
        self.assertEqual(hotspot_48["benchmark"], "hotspot")
        self.assertEqual(hotspot_48["kernel"], "hotspot")
        self.assertEqual(hotspot_48["problem_size"], "48")
        self.assertEqual(hotspot_48["hardware_real_ms"], "0.090800")
        self.assertEqual(hotspot_48["observed"]["duration_sec"], 8)
        self.assertEqual(hotspot_48["observed"]["outcome"], "success")
        self.assertEqual(hotspot_48["source_ci"]["run_id"], 24959959195)
        self.assertIn("not final/current finishable-size claims", manifest["snapshot_contract"]["claim_scope"])

    def test_text_summary_is_stable(self) -> None:
        result = self.run_validator("--summary-format", "text")

        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertEqual(
            result.stdout,
            "MI300A finishable evaluation set validation: pass\n"
            "set=mi300a-bounded-finishable-evaluation version=2026.04.26-run24959959195\n"
            "source_run=24959959195 observed_at=2026-04-26T17:04:42Z bounded_snapshot=True\n"
            "contract: discovery_entries=1416 finishable_entries=1416 eligible=23 timed_out=20 non_terminal=1373\n"
            "entries=23 benchmarks=5\n"
            "tier1: entries=6 benchmarks=2 matrix_rows=2 ranges=1,3,6,9,237-238\n"
            "tier2: entries=17 benchmarks=3 matrix_rows=3 ranges=473-482,720-722,955-958\n"
            "This evaluation surface includes only completed-success entries from the bounded 3600s discovery snapshot. "
            "Timed-out and non-terminal source rows remain visible only in source-contract counts and must not be described "
            "as permanent unfinishable claims without new terminal evidence.\n",
        )

    def test_update_writes_reproducible_temp_manifest_and_summary(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            evaluation_path, summary_path = self.generate_fixture(tmp_path)
            fixture_manifest = self.load_json(evaluation_path)
            checked_in_manifest = self.load_json(EVALUATION_MANIFEST)
            fixture_summary = self.load_json(summary_path)

        self.assertEqual(fixture_manifest, checked_in_manifest)
        self.assertEqual(fixture_summary["schema_name"], "mgpusim.mi300a_finishable_evaluation_summary")
        self.assertEqual(fixture_summary["entry_count"], 23)
        self.assertEqual(fixture_summary["source_evaluation_manifest"], str(evaluation_path.relative_to(REPO_ROOT)))

    def test_rejects_missing_eligible_entry_from_evaluation_manifest(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            evaluation_path, summary_path = self.generate_fixture(tmp_path)
            manifest = self.load_json(evaluation_path)
            manifest["entries"] = manifest["entries"][:-1]
            manifest["entry_count"] = len(manifest["entries"])
            self.write_json(evaluation_path, manifest)

            result = self.run_validator("--evaluation-manifest", evaluation_path, "--summary", summary_path)

        self.assertEqual(result.returncode, 1)
        self.assertIn("entries must include all and only eligible finishable entries", result.stderr)
        self.assertIn("missing=['958']", result.stderr)

    def test_rejects_extra_noneligible_entry_in_evaluation_manifest(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            evaluation_path, summary_path = self.generate_fixture(tmp_path)
            manifest = self.load_json(evaluation_path)
            extra_entry = dict(manifest["entries"][0])
            extra_entry["plan_index"] = 2
            manifest["entries"].append(extra_entry)
            manifest["entry_count"] = len(manifest["entries"])
            self.write_json(evaluation_path, manifest)

            result = self.run_validator("--evaluation-manifest", evaluation_path, "--summary", summary_path)

        self.assertEqual(result.returncode, 1)
        self.assertIn("entries must include all and only eligible finishable entries", result.stderr)
        self.assertIn("extra=['2']", result.stderr)

    def test_rejects_tier_view_drift_from_source_finishable_manifest(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            tier2_path = tmp_path / "mi300a_finishable_tier2_matrix.json"
            shutil.copyfile(TIER2_VIEW, tier2_path)
            tier2 = self.load_json(tier2_path)
            tier2["include"][0]["finishable_plan_index_ranges"] = ["473", "475-482"]
            tier2["include"][0]["sizes"] = "32 64 96 128 192 256 384 512 640"
            self.write_json(tier2_path, tier2)

            result = self.run_validator("--tier2", tier2_path)

        self.assertEqual(result.returncode, 1)
        self.assertIn("include rows must cover exactly eligible tier 2 indices", result.stderr)
        self.assertIn("missing=['474']", result.stderr)

    def test_rejects_source_finishable_count_contract_drift(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            finishable_path = tmp_path / "mi300a_finishable_size_manifest.json"
            shutil.copyfile(FINISHABLE_MANIFEST, finishable_path)
            manifest = self.load_json(finishable_path)
            summary = dict(manifest["summary"])
            observed = dict(summary["observed_outcome_bucket_counts"])
            observed["timed_out"] = 19
            summary["observed_outcome_bucket_counts"] = observed
            manifest["summary"] = summary
            self.write_json(finishable_path, manifest)

            result = self.run_validator("--finishable-manifest", finishable_path)

        self.assertEqual(result.returncode, 1)
        self.assertIn("observed_outcome_bucket_counts must preserve", result.stderr)
        self.assertIn("20 timed_out", result.stderr)


if __name__ == "__main__":
    unittest.main()
