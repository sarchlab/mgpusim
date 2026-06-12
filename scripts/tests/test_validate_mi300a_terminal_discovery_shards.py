from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from typing import Any, Mapping


REPO_ROOT = Path(__file__).resolve().parents[2]
GENERATOR = REPO_ROOT / "scripts" / "generate_mi300a_terminal_discovery_shards.py"
VALIDATOR = REPO_ROOT / "scripts" / "validate_mi300a_terminal_discovery_shards.py"
PLAN = REPO_ROOT / "benchmark-comparison" / "mi300a_problem_size_discovery_plan.json"
TMP_ROOT = REPO_ROOT / ".repo_tmp" / "mi300a_terminal_discovery_shard_validation_tests"


class ValidateMI300ATerminalDiscoveryShardsTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        TMP_ROOT.mkdir(parents=True, exist_ok=True)

    def generate_surface(self, tmp_path: Path) -> tuple[Path, Path]:
        manifest = tmp_path / "manifest.json"
        shard_dir = tmp_path / "shards"
        result = subprocess.run(
            [
                sys.executable,
                str(GENERATOR),
                "--plan",
                str(PLAN),
                "--manifest-output",
                str(manifest),
                "--shard-dir",
                str(shard_dir),
            ],
            cwd=REPO_ROOT,
            text=True,
            capture_output=True,
            check=False,
            timeout=30,
        )
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        return manifest, shard_dir

    def run_validator(self, manifest: Path, shard_dir: Path) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [
                sys.executable,
                str(VALIDATOR),
                "--plan",
                str(PLAN),
                "--manifest",
                str(manifest),
                "--shard-dir",
                str(shard_dir),
            ],
            cwd=REPO_ROOT,
            text=True,
            capture_output=True,
            check=False,
            timeout=30,
        )

    def load_json(self, path: Path) -> Mapping[str, Any]:
        with path.open("r", encoding="utf-8") as f:
            data = json.load(f)
        self.assertIsInstance(data, dict)
        return data

    def write_json(self, path: Path, data: Mapping[str, Any]) -> None:
        path.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")

    def test_checked_in_surface_validates_with_stable_json_report(self) -> None:
        result = subprocess.run(
            [sys.executable, str(VALIDATOR)],
            cwd=REPO_ROOT,
            text=True,
            capture_output=True,
            check=False,
            timeout=30,
        )

        self.assertEqual(result.returncode, 0, msg=result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual(report["schema_name"], "mgpusim.mi300a_terminal_discovery_benchmark_shard_validation")
        self.assertTrue(report["validated"])
        self.assertEqual(report["manifest"], "benchmark-comparison/generated/mi300a_terminal_discovery_shard_manifest.json")
        self.assertEqual(report["source_plan"], "benchmark-comparison/mi300a_problem_size_discovery_plan.json")
        self.assertEqual(report["entry_count"], 1416)
        self.assertEqual(report["attempt_count"], 1416)
        self.assertEqual(report["per_size_attempt_count"], 1416)
        self.assertEqual(report["runnable_unit_count"], 1416)
        self.assertEqual(report["benchmark_count"], 82)
        self.assertEqual(report["visible_matrix_row_count"], 82)
        self.assertEqual(report["problem_size_level_row_count"], 0)
        self.assertEqual(report["timeout_sec"], 3600)
        self.assertEqual(report["max_attempts_per_shard"], 256)
        self.assertEqual(report["shard_count"], 6)
        self.assertEqual(report["shard_attempt_count_max"], 256)
        self.assertEqual(report["shard_benchmark_row_count_max"], 16)
        self.assertEqual(report["shard_attempt_counts"], {"1": 256, "2": 255, "3": 254, "4": 255, "5": 244, "6": 152})
        self.assertEqual(report["shard_benchmark_row_counts"], {"1": 16, "2": 15, "3": 15, "4": 15, "5": 12, "6": 9})
        self.assertEqual(report["shard_plan_index_ranges"]["2"], ["257-511"])
        self.assertEqual(
            report["hotspot_48"],
            {
                "plan_index": 474,
                "shard_index": 2,
                "shard_id": "mi300a-terminal-discovery-shard-02",
                "shard_attempt_index": 218,
                "benchmark_entry_index": 14,
                "benchmark_attempt_index": 2,
                "matrix_path": "benchmark-comparison/generated/mi300a_terminal_discovery_shard_02_matrix.json",
                "matrix_row_id": "mi300a-terminal-discovery-benchmark-hotspot",
                "runnable_unit_id": "mi300a-terminal-discovery-plan-0474",
                "benchmark": "hotspot",
                "plan_benchmark": "hotspot",
                "problem_size": "48",
                "hardware_kernel_name": "hotspot",
                "hardware_problem_size": "48",
                "sizes": "48",
                "timeout_sec": 3600,
            },
        )

    def test_checked_in_matrices_cover_source_plan_attempts_exactly_once(self) -> None:
        plan = self.load_json(PLAN)
        plan_entries = plan["entries"]
        plan_by_index = {entry["plan_index"]: entry for entry in plan_entries}
        self.assertEqual(len(plan_entries), 1416)
        self.assertEqual(sorted(plan_by_index), list(range(1, 1417)))

        observed_attempts: list[Mapping[str, Any]] = []
        observed_rows: list[Mapping[str, Any]] = []
        manifest = self.load_json(REPO_ROOT / "benchmark-comparison" / "generated" / "mi300a_terminal_discovery_shard_manifest.json")
        for shard in manifest["shards"]:
            shard_index = shard["shard_index"]
            matrix = self.load_json(REPO_ROOT / shard["matrix_path"])
            self.assertEqual(matrix["shard_index"], shard_index)
            self.assertEqual(matrix["timeout_sec"], 3600)
            self.assertEqual(matrix["attempt_count"], shard["attempt_count"])
            self.assertEqual(matrix["benchmark_row_count"], shard["benchmark_row_count"])
            self.assertLessEqual(matrix["attempt_count"], 256)
            self.assertLessEqual(len(matrix["include"]), 256)
            for row in matrix["include"]:
                self.assertEqual(row["shard_index"], shard_index)
                self.assertEqual(row["timeout_sec"], 3600)
                self.assertEqual(row["attempt_count"], len(row["attempts"]))
                self.assertNotIn("plan_index", row)
                observed_rows.append(row)
                for attempt in row["attempts"]:
                    self.assertEqual(attempt["shard_index"], shard_index)
                    self.assertEqual(attempt["timeout_sec"], 3600)
                    self.assertLessEqual(attempt["timeout_sec"], 3600)
                    source_entry = plan_by_index[attempt["plan_index"]]
                    self.assertEqual(attempt["hardware_kernel_name"], source_entry["hardware_kernel_name"])
                    self.assertEqual(attempt["hardware_problem_size"], source_entry["hardware_problem_size"])
                    self.assertEqual(attempt["sizes"], source_entry["sizes"])
                    observed_attempts.append(attempt)

        observed_indices = [attempt["plan_index"] for attempt in observed_attempts]
        self.assertEqual(len(observed_rows), 82)
        self.assertEqual(len(observed_indices), 1416)
        # Workflow benchmarks need not be physically contiguous in the source
        # plan; attempts are emitted per benchmark in plan_index order, so
        # across-benchmark concatenation is checked as a set.
        self.assertEqual(sorted(observed_indices), list(range(1, 1417)))
        self.assertEqual(len(set(observed_indices)), 1416)
        hotspot_attempts = [
            attempt
            for attempt in observed_attempts
            if attempt["hardware_kernel_name"].lower() == "hotspot"
            and attempt["hardware_problem_size"] == "48"
        ]
        self.assertEqual(len(hotspot_attempts), 1)
        hotspot = hotspot_attempts[0]
        self.assertEqual(hotspot, observed_attempts[473])
        self.assertEqual(hotspot["plan_index"], 474)
        self.assertEqual(hotspot["shard_index"], 2)
        self.assertEqual(hotspot["benchmark_entry_index"], 14)
        self.assertEqual(hotspot["benchmark_attempt_index"], 2)
        self.assertEqual(hotspot["benchmark"], "hotspot")
        self.assertEqual(hotspot["problem_size"], "48")
        self.assertEqual(hotspot["sizes"], "48")

    def test_rejects_missing_per_size_attempt(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            manifest, shard_dir = self.generate_surface(Path(tmpdir))
            matrix_path = shard_dir / "mi300a_terminal_discovery_shard_01_matrix.json"
            matrix = dict(self.load_json(matrix_path))
            include = [dict(row) for row in matrix["include"]]
            removed = None
            for row in include:
                attempts = [dict(attempt) for attempt in row["attempts"]]
                for index, attempt in enumerate(attempts):
                    if attempt["plan_index"] == 100:
                        removed = attempts.pop(index)
                        row["attempts"] = attempts
                        row["attempt_count"] = len(attempts)
                        row["per_size_attempt_count"] = len(attempts)
                        break
                if removed is not None:
                    break
            self.assertIsNotNone(removed)
            self.assertEqual(removed["plan_index"], 100)
            matrix["include"] = include
            matrix["attempt_count"] = matrix["attempt_count"] - 1
            matrix["per_size_attempt_count"] = matrix["per_size_attempt_count"] - 1
            matrix["entry_count"] = matrix["entry_count"] - 1
            self.write_json(matrix_path, matrix)

            result = self.run_validator(manifest, shard_dir)

        self.assertEqual(result.returncode, 1)
        self.assertIn("rejected artifacts", result.stderr)
        self.assertIn("attempt_count must be", result.stderr)

    def test_rejects_hotspot_48_tampering(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            manifest, shard_dir = self.generate_surface(Path(tmpdir))
            matrix_path = shard_dir / "mi300a_terminal_discovery_shard_02_matrix.json"
            matrix = dict(self.load_json(matrix_path))
            include = [dict(row) for row in matrix["include"]]
            hotspot_row = include[13]
            self.assertEqual(hotspot_row["benchmark"], "hotspot")
            attempts = [dict(attempt) for attempt in hotspot_row["attempts"]]
            hotspot = dict(attempts[1])
            self.assertEqual(hotspot["plan_index"], 474)
            hotspot["hardware_problem_size"] = "49"
            attempts[1] = hotspot
            hotspot_row["attempts"] = attempts
            include[13] = hotspot_row
            matrix["include"] = include
            self.write_json(matrix_path, matrix)

            result = self.run_validator(manifest, shard_dir)

        self.assertEqual(result.returncode, 1)
        self.assertIn("rejected artifacts", result.stderr)
        self.assertIn("hardware_problem_size must match source plan", result.stderr)
        self.assertIn("'49' != '48'", result.stderr)

    def test_rejects_problem_size_level_row_metadata(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            manifest, shard_dir = self.generate_surface(Path(tmpdir))
            matrix_path = shard_dir / "mi300a_terminal_discovery_shard_01_matrix.json"
            matrix = dict(self.load_json(matrix_path))
            include = [dict(row) for row in matrix["include"]]
            first_row = dict(include[0])
            first_attempt = first_row["attempts"][0]
            first_row["plan_index"] = first_attempt["plan_index"]
            include[0] = first_row
            matrix["include"] = include
            self.write_json(matrix_path, matrix)

            result = self.run_validator(manifest, shard_dir)

        self.assertEqual(result.returncode, 1)
        self.assertIn("rejected artifacts", result.stderr)
        self.assertIn("not a problem-size-level row", result.stderr)
        self.assertIn("forbidden per-size keys=['plan_index']", result.stderr)

    def test_rejects_extra_generated_shard_matrix_file(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            manifest, shard_dir = self.generate_surface(Path(tmpdir))
            source_path = shard_dir / "mi300a_terminal_discovery_shard_01_matrix.json"
            extra_path = shard_dir / "mi300a_terminal_discovery_shard_99_matrix.json"
            extra_path.write_text(source_path.read_text(encoding="utf-8"), encoding="utf-8")

            result = self.run_validator(manifest, shard_dir)

        self.assertEqual(result.returncode, 1)
        self.assertIn("rejected artifacts", result.stderr)
        self.assertIn("must contain exactly the regenerated benchmark shard matrix files", result.stderr)
        self.assertIn("mi300a_terminal_discovery_shard_99_matrix.json", result.stderr)

    def test_rejects_timeout_tampering(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            manifest, shard_dir = self.generate_surface(Path(tmpdir))
            matrix_path = shard_dir / "mi300a_terminal_discovery_shard_01_matrix.json"
            matrix = dict(self.load_json(matrix_path))
            include = [dict(row) for row in matrix["include"]]
            first_row = dict(include[0])
            attempts = [dict(attempt) for attempt in first_row["attempts"]]
            first = dict(attempts[0])
            self.assertEqual(first["plan_index"], 1)
            first["timeout_sec"] = 7200
            attempts[0] = first
            first_row["attempts"] = attempts
            include[0] = first_row
            matrix["include"] = include
            self.write_json(matrix_path, matrix)

            result = self.run_validator(manifest, shard_dir)

        self.assertEqual(result.returncode, 1)
        self.assertIn("rejected artifacts", result.stderr)
        self.assertIn("timeout_sec must be 3600", result.stderr)
        self.assertIn("got 7200", result.stderr)


if __name__ == "__main__":
    unittest.main()
