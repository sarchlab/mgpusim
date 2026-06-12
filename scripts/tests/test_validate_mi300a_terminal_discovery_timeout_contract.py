from __future__ import annotations

import json
import subprocess
import sys
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
VALIDATOR = REPO_ROOT / "scripts" / "validate_mi300a_terminal_discovery_timeout_contract.py"


class ValidateMI300ATerminalDiscoveryTimeoutContractTest(unittest.TestCase):
    def run_guard(self, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [sys.executable, str(VALIDATOR), *args],
            cwd=REPO_ROOT,
            text=True,
            capture_output=True,
            check=False,
            timeout=30,
        )

    def test_m23_ten_minute_row_contract_fails_before_dispatch(self) -> None:
        result = self.run_guard(
            "--row-timeout-minutes",
            "10",
            "--per-attempt-timeout-sec",
            "600",
            "--row-overhead-sec",
            "120",
            "--max-violations",
            "5",
        )

        self.assertEqual(result.returncode, 1)
        self.assertIn("timeout-contract guard rejected the row budget", result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual(report["schema_name"], "mgpusim.mi300a_terminal_discovery_timeout_contract_validation")
        self.assertEqual(report["schema_version"], 1)
        self.assertEqual(report["validation_status"], "fail")
        self.assertFalse(report["validated"])
        self.assertTrue(report["dry_run"])
        self.assertEqual(
            report["contract"],
            {
                "formula": "required_timeout_sec = attempt_count * per_attempt_timeout_sec + row_overhead_sec",
                "row_timeout_sec": 600,
                "per_attempt_timeout_sec": 600,
                "row_overhead_sec": 120,
            },
        )
        self.assertEqual(report["row_count"], 82)
        self.assertEqual(report["attempt_count"], 1416)
        self.assertEqual(report["shard_count"], 6)
        self.assertEqual(report["source_attempt_timeout_sec_values"], [3600])
        self.assertEqual(report["max_attempt_count"], 54)
        self.assertEqual(report["max_required_timeout_sec"], 32520)
        self.assertEqual(report["min_slack_sec"], -31920)
        self.assertEqual(report["violation_count"], 82)
        self.assertTrue(report["violations_truncated"])
        self.assertEqual(len(report["violations"]), 5)
        self.assertEqual(
            report["violations"][0]["matrix_row_id"],
            "mi300a-terminal-discovery-benchmark-mem-latency-chase",
        )
        self.assertEqual(report["violations"][0]["required_timeout_sec"], 9720)
        self.assertEqual(report["violations"][0]["slack_sec"], -9120)
        self.assertEqual(
            report["tightest_row"]["matrix_row_id"],
            "mi300a-terminal-discovery-benchmark-parboil-tpacf",
        )
        self.assertEqual(report["tightest_row"]["attempt_count"], 54)
        self.assertEqual(report["tightest_row"]["required_timeout_sec"], 32520)
        self.assertEqual(report["tightest_row"]["slack_sec"], -31920)

    def test_sufficient_row_budget_passes_with_stable_json_report(self) -> None:
        result = self.run_guard(
            "--row-timeout-sec",
            "32520",
            "--per-attempt-timeout-sec",
            "600",
            "--row-overhead-sec",
            "120",
        )

        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertEqual(result.stderr, "")
        report = json.loads(result.stdout)
        self.assertEqual(report["validation_status"], "pass")
        self.assertTrue(report["validated"])
        self.assertEqual(report["violation_count"], 0)
        self.assertEqual(report["violating_matrix_row_ids"], [])
        self.assertEqual(report["violations"], [])
        self.assertFalse(report["violations_truncated"])
        self.assertEqual(report["row_count"], 82)
        self.assertEqual(report["attempt_count"], 1416)
        self.assertEqual(report["shard_count"], 6)
        self.assertEqual(report["max_attempt_count"], 54)
        self.assertEqual(report["max_required_timeout_sec"], 32520)
        self.assertEqual(report["min_slack_sec"], 0)
        self.assertEqual(
            report["tightest_row"]["matrix_row_id"],
            "mi300a-terminal-discovery-benchmark-parboil-tpacf",
        )
        self.assertEqual(report["tightest_row"]["fits_timeout_contract"], True)
        self.assertEqual(report["tightest_row"]["slack_sec"], 0)

    def test_row_timeout_minutes_are_converted_before_budget_check(self) -> None:
        result = self.run_guard(
            "--row-timeout-minutes",
            "540",
            "--per-attempt-timeout-sec",
            "600",
            "--row-overhead-sec",
            "0",
        )

        self.assertEqual(result.returncode, 0, msg=result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual(report["contract"]["row_timeout_sec"], 32400)
        self.assertEqual(report["contract"]["row_overhead_sec"], 0)
        self.assertEqual(report["validation_status"], "pass")
        self.assertEqual(
            report["tightest_row"]["matrix_row_id"],
            "mi300a-terminal-discovery-benchmark-parboil-tpacf",
        )
        self.assertEqual(report["tightest_row"]["slack_sec"], 0)

    def test_row_overhead_is_required_so_the_contract_is_explicit(self) -> None:
        result = self.run_guard(
            "--row-timeout-minutes",
            "10",
            "--per-attempt-timeout-sec",
            "600",
        )

        self.assertEqual(result.returncode, 2)
        self.assertEqual(result.stdout, "")
        self.assertIn("--row-overhead-sec", result.stderr)


if __name__ == "__main__":
    unittest.main()
