from __future__ import annotations

import hashlib
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

import generate_mi300a_terminal_recovery_dry_run_plan as dry_run

SCRIPT = SCRIPTS_DIR / "generate_mi300a_terminal_recovery_dry_run_plan.py"
RECOVERY_MANIFEST = (
    REPO_ROOT
    / "benchmark-comparison"
    / "generated"
    / "mi300a_terminal_discovery_run_25111815292_recovery_manifest.json"
)
CHECKED_IN_PLAN = (
    REPO_ROOT
    / "benchmark-comparison"
    / "generated"
    / "mi300a_terminal_discovery_run_25111815292_recovery_dry_run_plan.json"
)
TMP_ROOT = REPO_ROOT / ".repo_tmp" / "mi300a_terminal_recovery_dry_run_plan_tests"
CONTRACT_CAP_FAILURE_CASES = (
    ("--max-parallel", "15", "max_parallel", 15, dry_run.DEFAULT_MAX_PARALLEL),
    (
        "--per-attempt-timeout-sec",
        "700",
        "per_attempt_timeout_sec",
        700,
        dry_run.DEFAULT_PER_ATTEMPT_TIMEOUT_SEC,
    ),
    ("--row-timeout-sec", "700", "row_timeout_sec", 700, dry_run.DEFAULT_ROW_TIMEOUT_SEC),
    ("--action-timeout-sec", "4200", "action_timeout_sec", 4200, dry_run.DEFAULT_ACTION_TIMEOUT_SEC),
)


def parse_ranges(ranges: list[str]) -> list[int]:
    values: list[int] = []
    for range_text in ranges:
        if "-" in range_text:
            start_text, end_text = range_text.split("-", 1)
            values.extend(range(int(start_text), int(end_text) + 1))
        else:
            values.append(int(range_text))
    return values


class GenerateMI300ATerminalRecoveryDryRunPlanTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        TMP_ROOT.mkdir(parents=True, exist_ok=True)

    def load_json(self, path: Path) -> Mapping[str, Any]:
        with path.open("r", encoding="utf-8") as f:
            data = json.load(f)
        self.assertIsInstance(data, dict)
        return data

    def run_generator(self, output: Path, *extra_args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [
                sys.executable,
                str(SCRIPT),
                "--recovery-manifest",
                str(RECOVERY_MANIFEST),
                "--output",
                str(output),
                *extra_args,
            ],
            cwd=REPO_ROOT,
            text=True,
            capture_output=True,
            check=False,
            timeout=30,
        )

    def test_generation_expands_940_missing_entries_into_single_attempt_batches(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            output = Path(tmpdir) / "recovery_dry_run_plan.json"
            result = self.run_generator(output)
            self.assertEqual(result.returncode, 0, msg=result.stderr)
            self.assertIn("attempts=940 rows=940 batches=12 max_parallel=14", result.stdout)
            plan = self.load_json(output)

        self.assertEqual(plan["schema_name"], "mgpusim.mi300a_terminal_discovery_recovery_dry_run_plan")
        self.assertEqual(plan["schema_version"], 1)
        self.assertTrue(plan["dry_run"])
        self.assertTrue(plan["recovery_only"])
        self.assertEqual(plan["recovery_entry_count"], 940)
        self.assertEqual(plan["recovery_attempt_count"], 940)
        self.assertEqual(plan["dry_run_matrix_row_count"], 940)
        self.assertEqual(plan["source_recovery_row_count"], 79)
        self.assertEqual(plan["recovery_benchmark_count"], 79)
        self.assertEqual(plan["recovery_batch_count"], 12)
        self.assertEqual(plan["prior_observed_plan_index_count"], 476)
        self.assertTrue(plan["prior_observed_excluded_from_recovery"])

        contract = plan["operator_contract"]
        self.assertEqual(contract["max_parallel"], 14)
        self.assertEqual(contract["attempts_per_matrix_row"], 1)
        self.assertEqual(contract["per_attempt_timeout_sec"], 600)
        self.assertEqual(contract["benchmark_run_timeout_sec"], 600)
        self.assertEqual(contract["row_timeout_sec"], 600)
        self.assertEqual(contract["row_overhead_sec"], 0)
        self.assertEqual(contract["row_required_timeout_sec"], 600)
        self.assertEqual(contract["action_timeout_sec"], 3600)
        self.assertEqual(contract["max_parallel_waves_per_batch"], 6)
        self.assertEqual(contract["max_rows_per_batch"], 84)
        self.assertEqual(contract["row_timeout_contract_status"], "pass")
        self.assertEqual(contract["action_timeout_contract_status"], "pass")

        summary = plan["timeout_contract_summary"]
        self.assertEqual(summary["row_violation_count"], 0)
        self.assertEqual(summary["batch_violation_count"], 0)
        self.assertEqual(summary["max_batch_required_timeout_sec"], 3600)
        self.assertEqual(summary["min_action_slack_sec"], 0)
        self.assertEqual(summary["max_row_required_timeout_sec"], 600)
        self.assertEqual(summary["min_row_slack_sec"], 0)

        batches = plan["batches"]
        self.assertEqual([batch["row_count"] for batch in batches], [84] * 11 + [16])
        self.assertEqual([batch["parallel_wave_count"] for batch in batches], [6] * 11 + [2])
        self.assertEqual([batch["batch_required_timeout_sec"] for batch in batches], [3600] * 11 + [1200])
        self.assertEqual([batch["action_slack_sec"] for batch in batches], [0] * 11 + [2400])
        for batch in batches:
            self.assertTrue(batch["dry_run_only"])
            self.assertTrue(batch["recovery_only"])
            self.assertTrue(batch["fits_action_timeout_contract"])
            self.assertLessEqual(batch["row_count"], contract["max_rows_per_batch"])
            self.assertLessEqual(batch["parallel_wave_count"], contract["max_parallel_waves_per_batch"])
            self.assertEqual(batch["attempt_count"], batch["row_count"])
            self.assertEqual(batch["per_size_attempt_count"], batch["row_count"])
            self.assertEqual(batch["per_attempt_timeout_sec"], 600)
            self.assertEqual(batch["row_timeout_sec"], 600)
            self.assertEqual(batch["row_required_timeout_sec"], 600)
            self.assertNotIn("rows", batch)
            self.assertEqual(
                len(parse_ranges(batch["plan_index_ranges"])),
                batch["row_count"],
            )

        template = plan["dry_run_matrix_row_template"]
        self.assertEqual(template["attempt_count"], 1)
        self.assertEqual(template["per_size_attempt_count"], 1)
        self.assertEqual(template["per_attempt_timeout_sec"], 600)
        self.assertEqual(template["row_timeout_sec"], 600)
        self.assertEqual(template["row_required_timeout_sec"], 600)
        self.assertTrue(template["dry_run_only"])
        self.assertTrue(template["recovery_only"])

        first_batch = batches[0]
        self.assertEqual(first_batch["dry_run_row_index_start"], 1)
        self.assertEqual(first_batch["dry_run_row_index_end"], 84)
        self.assertEqual(first_batch["plan_index_start"], 9)
        self.assertEqual(first_batch["plan_index_end"], 149)
        self.assertEqual(first_batch["first_matrix_row_id"], "mi300a-terminal-discovery-recovery-plan-0009")
        self.assertEqual(first_batch["last_matrix_row_id"], "mi300a-terminal-discovery-recovery-plan-0149")
        self.assertEqual(first_batch["first_runnable_unit_id"], "mi300a-terminal-discovery-plan-0009")
        self.assertEqual(first_batch["last_runnable_unit_id"], "mi300a-terminal-discovery-plan-0149")

        self.assertEqual(batches[1]["dry_run_row_index_start"], 85)
        self.assertEqual(batches[1]["recovery_batch_index"], 2)
        self.assertEqual(batches[-1]["plan_index_end"], 1401)
        self.assertEqual(batches[-1]["last_runnable_unit_id"], "mi300a-terminal-discovery-plan-1401")

    def test_recovery_coverage_matches_manifest_missing_scope_and_excludes_observed_rows(self) -> None:
        plan = self.load_json(CHECKED_IN_PLAN)
        manifest = self.load_json(RECOVERY_MANIFEST)
        dry_run_indices = [
            plan_index
            for batch in plan["batches"]
            for plan_index in parse_ranges(batch["plan_index_ranges"])
        ]
        missing_indices = parse_ranges(manifest["missing_plan_index_ranges"])
        observed_indices = parse_ranges(manifest["prior_incomplete_evidence"]["observed_plan_index_ranges"])

        self.assertEqual(dry_run_indices, missing_indices)
        self.assertEqual(len(dry_run_indices), 940)
        self.assertEqual(len(set(dry_run_indices)), 940)
        self.assertEqual(set(dry_run_indices).intersection(observed_indices), set())
        self.assertEqual(set(dry_run_indices).union(observed_indices), set(range(1, 1417)))
        self.assertEqual(plan["coverage"]["duplicate_dry_run_plan_index_count"], 0)
        self.assertEqual(plan["coverage"]["extra_dry_run_plan_index_count"], 0)
        self.assertEqual(plan["coverage"]["missing_dry_run_plan_index_count"], 0)
        self.assertEqual(plan["coverage"]["prior_observed_overlap_count"], 0)
        self.assertEqual(plan["coverage"]["represented_plan_index_count_after_union"], 1416)
        dry_run_benchmarks = {
            benchmark
            for batch in plan["batches"]
            for benchmark in batch["workflow_benchmarks"]
        }
        self.assertNotIn("gelu", dry_run_benchmarks)
        self.assertNotIn("sssp", dry_run_benchmarks)
        self.assertNotIn("dmr", dry_run_benchmarks)

    def test_checked_in_plan_is_current_without_rewriting(self) -> None:
        before_stat = CHECKED_IN_PLAN.stat()
        before_text = CHECKED_IN_PLAN.read_text(encoding="utf-8")
        result = self.run_generator(CHECKED_IN_PLAN, "--check")
        after_stat = CHECKED_IN_PLAN.stat()
        after_text = CHECKED_IN_PLAN.read_text(encoding="utf-8")

        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertIn("OK: recovery dry-run plan is up to date", result.stdout)
        self.assertEqual(after_text, before_text)
        self.assertEqual(after_stat.st_mtime_ns, before_stat.st_mtime_ns)

    def test_checked_in_plan_pins_reproducible_manifest_and_non_goals(self) -> None:
        plan = self.load_json(CHECKED_IN_PLAN)
        manifest_sha256 = hashlib.sha256(RECOVERY_MANIFEST.read_bytes()).hexdigest()

        self.assertEqual(
            plan["source_recovery_manifest"]["artifact"],
            "benchmark-comparison/generated/mi300a_terminal_discovery_run_25111815292_recovery_manifest.json",
        )
        self.assertEqual(plan["source_recovery_manifest"]["sha256"], manifest_sha256)
        self.assertEqual(plan["source_recovery_manifest"]["run_id"], "25111815292")
        self.assertEqual(plan["source_recovery_manifest"]["run_conclusion"], "cancelled")
        self.assertEqual(
            plan["generation_policy"]["non_goals"],
            [
                "no simulator execution or simulation campaign",
                "no workflow dispatch/cancel/rerun",
                "no calibration start or parameter sweep",
                "no provenance, finishable manifest, or Tier artifact regeneration",
                "no terminal finishability completion claim",
                "no permanent visible Actions surface change",
            ],
        )

    def test_check_mode_rejects_missing_output_without_creating_it(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            output = Path(tmpdir) / "missing_recovery_dry_run_plan.json"
            result = self.run_generator(output, "--check")
            self.assertFalse(output.exists())

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("output file is missing", result.stderr)

    def test_operator_budget_overrides_above_caps_fail_without_creating_output(self) -> None:
        for flag, value, field, int_value, cap in CONTRACT_CAP_FAILURE_CASES:
            with self.subTest(flag=flag):
                with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
                    output = Path(tmpdir) / "recovery_dry_run_plan.json"
                    result = self.run_generator(output, flag, value)
                    self.assertFalse(output.exists())

                self.assertEqual(result.returncode, 1)
                self.assertEqual(result.stdout, "")
                self.assertIn("ERROR:", result.stderr)
                self.assertIn("exceeds recovery dry-run operator contract cap", result.stderr)
                self.assertIn(f"{field}={int_value}", result.stderr)
                self.assertIn(f"{field}<={cap}", result.stderr)

    def test_programmatic_budget_values_above_caps_are_rejected(self) -> None:
        defaults = {
            "max_parallel": dry_run.DEFAULT_MAX_PARALLEL,
            "per_attempt_timeout_sec": dry_run.DEFAULT_PER_ATTEMPT_TIMEOUT_SEC,
            "row_timeout_sec": dry_run.DEFAULT_ROW_TIMEOUT_SEC,
            "action_timeout_sec": dry_run.DEFAULT_ACTION_TIMEOUT_SEC,
            "row_overhead_sec": dry_run.DEFAULT_ROW_OVERHEAD_SEC,
        }
        for _flag, _value, field, int_value, cap in CONTRACT_CAP_FAILURE_CASES:
            with self.subTest(field=field):
                invalid_contract = dict(defaults)
                invalid_contract[field] = int_value
                with self.assertRaisesRegex(
                    dry_run.RecoveryDryRunPlanError,
                    f"{field}={int_value}.*{field}<={cap}",
                ):
                    dry_run.validate_contract(**invalid_contract)
                with self.assertRaisesRegex(
                    dry_run.RecoveryDryRunPlanError,
                    f"{field}={int_value}.*{field}<={cap}",
                ):
                    dry_run.build_plan(RECOVERY_MANIFEST, **invalid_contract)

    def test_nonzero_row_overhead_rejects_exact_600_second_row_cap(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            output = Path(tmpdir) / "recovery_dry_run_plan.json"
            result = self.run_generator(output, "--row-overhead-sec", "1")
            self.assertFalse(output.exists())

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("single-attempt recovery rows do not fit the row timeout", result.stderr)

    def test_generation_is_byte_stable(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            output = Path(tmpdir) / "recovery_dry_run_plan.json"
            first = self.run_generator(output)
            self.assertEqual(first.returncode, 0, msg=first.stderr)
            first_text = output.read_text(encoding="utf-8")
            second = self.run_generator(output)
            self.assertEqual(second.returncode, 0, msg=second.stderr)
            second_text = output.read_text(encoding="utf-8")
        self.assertEqual(first.stdout, second.stdout)
        self.assertEqual(first_text, second_text)


if __name__ == "__main__":
    unittest.main()
