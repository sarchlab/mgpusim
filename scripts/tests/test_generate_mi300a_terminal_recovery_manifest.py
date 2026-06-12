from __future__ import annotations

import hashlib
import json
import subprocess
import sys
import unittest
from pathlib import Path
from typing import Any, Mapping


REPO_ROOT = Path(__file__).resolve().parents[2]
PLAN = REPO_ROOT / "benchmark-comparison" / "mi300a_problem_size_discovery_plan.json"
CHECKED_IN_MANIFEST = (
    REPO_ROOT
    / "benchmark-comparison"
    / "generated"
    / "mi300a_terminal_discovery_run_25111815292_recovery_manifest.json"
)


class GenerateMI300ATerminalRecoveryManifestTest(unittest.TestCase):

    def load_json(self, path: Path) -> Mapping[str, Any]:
        with path.open("r", encoding="utf-8") as f:
            data = json.load(f)
        self.assertIsInstance(data, dict)
        return data

    def test_m24_coverage_doc_is_removed(self) -> None:
        m24_doc = REPO_ROOT / "docs" / "m24_incomplete_terminal_discovery_coverage.md"
        self.assertFalse(
            m24_doc.exists(),
            msg=f"{m24_doc} should have been removed as a process doc",
        )

    def test_checked_in_manifest_pins_reproducible_source_contract(self) -> None:
        manifest = self.load_json(CHECKED_IN_MANIFEST)
        plan_sha256 = hashlib.sha256(PLAN.read_bytes()).hexdigest()

        self.assertEqual(manifest["source_plan"]["sha256"], plan_sha256)
        self.assertEqual(
            manifest["source_evidence"]["m24_coverage_report"],
            "docs/m24_incomplete_terminal_discovery_coverage.md",
        )
        self.assertEqual(manifest["source_evidence"]["prior_attempt_timeout_sec"], 600)
        self.assertEqual(
            manifest["source_plan_lookup_fields"],
            [
                "problem_size",
                "sizes",
                "size_token",
                "size_label",
                "flag_template",
                "size_label_template",
                "timeout_sec",
                "artifact_id",
                "job_id",
                "extra_flags",
                "gomemlimit",
                "gogc",
            ],
        )
        self.assertEqual(
            manifest["generation_policy"]["non_goals"],
            [
                "no simulator execution",
                "no workflow dispatch/cancel/rerun",
                "no provenance or finishable manifest regeneration",
                "no terminal finishability completion claim",
            ],
        )
        self.assertEqual(
            manifest["generation_policy"]["row_policy"],
            "one recovery row per workflow benchmark that has at least one missing plan index",
        )

    def test_check_mode_accepts_checked_in_manifest_after_m24_doc_removal(self) -> None:
        before_stat = CHECKED_IN_MANIFEST.stat()
        before_text = CHECKED_IN_MANIFEST.read_text(encoding="utf-8")
        result = subprocess.run(
            [
                sys.executable,
                str(REPO_ROOT / "scripts" / "generate_mi300a_terminal_recovery_manifest.py"),
                "--check",
            ],
            cwd=REPO_ROOT,
            text=True,
            capture_output=True,
            check=False,
            timeout=30,
        )
        after_stat = CHECKED_IN_MANIFEST.stat()
        after_text = CHECKED_IN_MANIFEST.read_text(encoding="utf-8")

        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertIn("OK: recovery manifest is up to date", result.stdout)
        self.assertEqual(after_text, before_text)
        self.assertEqual(after_stat.st_mtime_ns, before_stat.st_mtime_ns)

    def test_contributing_md_has_required_sections(self) -> None:
        contributing = REPO_ROOT / "docs" / "CONTRIBUTING.md"
        text = contributing.read_text(encoding="utf-8")
        for expected in ["MI300A Benchmark Workflow", "Accuracy Report", "symmetric_err", "Workflow Contract"]:
            self.assertIn(expected, text, msg=f"docs/CONTRIBUTING.md missing: {expected!r}")

    def test_runner_configuration_has_max_parallel_14(self) -> None:
        runner_cfg = REPO_ROOT / "docs" / "runner_configuration.md"
        text = runner_cfg.read_text(encoding="utf-8")
        self.assertIn("max-parallel: 14", text, msg="docs/runner_configuration.md missing 'max-parallel: 14'")

    def test_validation_report_has_63_bench(self) -> None:
        validation = REPO_ROOT / "docs" / "validation_report.md"
        text = validation.read_text(encoding="utf-8")
        self.assertIn("63-bench", text, msg="docs/validation_report.md missing '63-bench'")


if __name__ == "__main__":
    unittest.main()
