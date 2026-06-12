from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from typing import Any, Mapping


REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPT = REPO_ROOT / "scripts" / "generate_mi300a_terminal_discovery_shards.py"
PLAN = REPO_ROOT / "benchmark-comparison" / "mi300a_problem_size_discovery_plan.json"
TMP_ROOT = REPO_ROOT / ".repo_tmp" / "mi300a_terminal_discovery_shard_generation_tests"


class GenerateMI300ATerminalDiscoveryShardsTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        TMP_ROOT.mkdir(parents=True, exist_ok=True)

    def run_generator(self, tmp_path: Path) -> tuple[subprocess.CompletedProcess[str], Path, Path]:
        manifest = tmp_path / "manifest.json"
        shard_dir = tmp_path / "shards"
        result = subprocess.run(
            [
                sys.executable,
                str(SCRIPT),
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
        return result, manifest, shard_dir

    def load_json(self, path: Path) -> Mapping[str, Any]:
        with path.open("r", encoding="utf-8") as f:
            data = json.load(f)
        self.assertIsInstance(data, dict)
        return data

    def test_generation_covers_all_1416_attempts_in_per_benchmark_rows(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            result, manifest_path, shard_dir = self.run_generator(Path(tmpdir))
            self.assertEqual(result.returncode, 0, msg=result.stderr)
            self.assertIn("entries=1416 attempts=1416 benchmark_rows=82 shards=6", result.stdout)
            self.assertIn("max_attempts_per_shard=256", result.stdout)
            manifest = self.load_json(manifest_path)
            matrices = [self.load_json(shard_dir / f"mi300a_terminal_discovery_shard_{index:02d}_matrix.json") for index in range(1, 7)]

        self.assertEqual(manifest["schema_name"], "mgpusim.mi300a_terminal_discovery_benchmark_shard_manifest")
        self.assertEqual(manifest["schema_version"], 1)
        self.assertEqual(manifest["source_plan"]["artifact"], "benchmark-comparison/mi300a_problem_size_discovery_plan.json")
        self.assertEqual(manifest["entry_count"], 1416)
        self.assertEqual(manifest["attempt_count"], 1416)
        self.assertEqual(manifest["per_size_attempt_count"], 1416)
        self.assertEqual(manifest["runnable_unit_count"], 1416)
        self.assertEqual(manifest["benchmark_count"], 82)
        self.assertEqual(manifest["visible_matrix_row_count"], 82)
        self.assertEqual(manifest["timeout_sec"], 3600)
        self.assertEqual(manifest["max_attempts_per_shard"], 256)
        self.assertEqual(manifest["shard_count"], 6)
        self.assertEqual(manifest["shard_attempt_count_max"], 256)
        self.assertEqual(manifest["shard_benchmark_row_count_max"], 16)
        self.assertEqual(manifest["plan_index_ranges"], ["1-1416"])
        self.assertEqual(manifest["generation_policy"]["matrix_row_policy"], "one_matrix_include_entry_per_workflow_benchmark")
        self.assertEqual(
            {key: manifest["coverage"][key] for key in [
                "expected_plan_index_count",
                "represented_plan_index_count",
                "attempt_count",
                "per_size_attempt_count",
                "runnable_unit_count",
                "visible_matrix_row_count",
                "benchmark_count",
                "duplicate_plan_index_count",
                "missing_plan_index_count",
                "extra_plan_index_count",
            ]},
            {
                "expected_plan_index_count": 1416,
                "represented_plan_index_count": 1416,
                "attempt_count": 1416,
                "per_size_attempt_count": 1416,
                "runnable_unit_count": 1416,
                "visible_matrix_row_count": 82,
                "benchmark_count": 82,
                "duplicate_plan_index_count": 0,
                "missing_plan_index_count": 0,
                "extra_plan_index_count": 0,
            },
        )
        self.assertEqual(
            [shard["plan_index_ranges"] for shard in manifest["shards"]],
            [["1-256"], ["257-511"], ["512-765"], ["766-1020"], ["1021-1264"], ["1265-1416"]],
        )
        self.assertEqual([shard["attempt_count"] for shard in manifest["shards"]], [256, 255, 254, 255, 244, 152])
        self.assertEqual([shard["benchmark_row_count"] for shard in manifest["shards"]], [16, 15, 15, 15, 12, 9])

        all_rows: list[Mapping[str, Any]] = []
        all_attempts: list[Mapping[str, Any]] = []
        for shard_index, matrix in enumerate(matrices, start=1):
            self.assertEqual(matrix["schema_name"], "mgpusim.mi300a_terminal_discovery_benchmark_shard_matrix")
            self.assertEqual(matrix["shard_index"], shard_index)
            self.assertEqual(matrix["shard_count"], 6)
            self.assertEqual(matrix["max_attempts_per_shard"], 256)
            self.assertEqual(matrix["attempt_count"], manifest["shards"][shard_index - 1]["attempt_count"])
            self.assertEqual(matrix["benchmark_row_count"], manifest["shards"][shard_index - 1]["benchmark_row_count"])
            self.assertLessEqual(matrix["attempt_count"], 256)
            self.assertLessEqual(len(matrix["include"]), 256)
            self.assertEqual(matrix["timeout_sec"], 3600)
            self.assertEqual(matrix["matrix_row_policy"], "one_matrix_include_entry_per_workflow_benchmark")
            all_rows.extend(matrix["include"])
            for expected_row_offset, row in enumerate(matrix["include"], start=1):
                self.assertEqual(row["shard_index"], shard_index)
                self.assertEqual(row["shard_entry_index"], expected_row_offset)
                self.assertEqual(row["timeout_sec"], 3600)
                self.assertIn("attempts", row)
                self.assertEqual(row["attempt_count"], len(row["attempts"]))
                self.assertNotIn("plan_index", row, msg="visible matrix rows must be benchmark groups, not single problem sizes")
                all_attempts.extend(row["attempts"])
                for expected_attempt_offset, attempt in enumerate(row["attempts"], start=1):
                    self.assertEqual(attempt["shard_index"], shard_index)
                    self.assertEqual(attempt["benchmark_entry_index"], expected_row_offset)
                    self.assertEqual(attempt["benchmark_attempt_index"], expected_attempt_offset)
                    self.assertEqual(attempt["matrix_row_id"], row["matrix_row_id"])
                    self.assertEqual(attempt["timeout_sec"], 3600)
                    self.assertEqual(len(attempt["sizes"].split()), 1)
                    self.assertEqual(attempt["sizes"], attempt["size_token"])
                    self.assertEqual(attempt["size_label"], attempt["problem_size"])
                    self.assertEqual(attempt["runnable_unit_id"], f"mi300a-terminal-discovery-plan-{attempt['plan_index']:04d}")

        self.assertEqual(len(all_rows), 82)
        plan_indices = [attempt["plan_index"] for attempt in all_attempts]
        # Workflow benchmarks need not be physically contiguous in the
        # source plan (backprop_adjust_weights interleaved with
        # backprop), so attempts are emitted per benchmark in plan_index
        # order. Across-benchmark concatenation is therefore checked as a
        # set, not as a sequence.
        self.assertEqual(sorted(plan_indices), list(range(1, 1417)))
        self.assertEqual(len(plan_indices), 1416)
        self.assertEqual(len(set(plan_indices)), 1416)

        first_row = all_rows[0]
        self.assertEqual(first_row["benchmark"], "mem_latency_chase")
        self.assertEqual(first_row["attempt_count"], 16)
        self.assertEqual(first_row["job_name"], "Benchmark Discovery: mem_latency_chase (16 attempts)")
        first_attempt = first_row["attempts"][0]
        self.assertEqual(first_attempt["plan_index"], 1)
        self.assertEqual(first_attempt["benchmark"], "mem_latency_chase")
        self.assertEqual(first_attempt["plan_benchmark"], "mem_latency_chase")
        self.assertEqual(first_attempt["sizes"], "256")

        hotspot_48 = all_attempts[473]
        self.assertEqual(
            manifest["hotspot_48"],
            {
                "plan_index": 474,
                "shard_index": 2,
                "shard_id": "mi300a-terminal-discovery-shard-02",
                "shard_attempt_index": 218,
                "benchmark_entry_index": 14,
                "benchmark_attempt_index": 2,
                "matrix_path": str(manifest_path.parent / "shards" / "mi300a_terminal_discovery_shard_02_matrix.json").replace(str(REPO_ROOT) + "/", ""),
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
        self.assertEqual(hotspot_48["plan_index"], 474)
        self.assertEqual(hotspot_48["shard_index"], 2)
        self.assertEqual(hotspot_48["benchmark_entry_index"], 14)
        self.assertEqual(hotspot_48["benchmark_attempt_index"], 2)
        self.assertEqual(hotspot_48["benchmark"], "hotspot")
        self.assertEqual(hotspot_48["problem_size"], "48")
        self.assertEqual(hotspot_48["hardware_kernel_name"], "hotspot")
        self.assertEqual(hotspot_48["hardware_problem_size"], "48")
        self.assertEqual(hotspot_48["sizes"], "48")
        self.assertEqual(hotspot_48["timeout_sec"], 3600)

    def test_generation_is_byte_stable_when_repeated_to_same_paths(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            first_result, manifest_path, shard_dir = self.run_generator(tmp_path)
            self.assertEqual(first_result.returncode, 0, msg=first_result.stderr)
            first_texts = {manifest_path.name: manifest_path.read_text(encoding="utf-8")}
            first_texts.update(
                {
                    path.name: path.read_text(encoding="utf-8")
                    for path in sorted(shard_dir.glob("mi300a_terminal_discovery_shard_*_matrix.json"))
                }
            )

            second_result, _, _ = self.run_generator(tmp_path)
            self.assertEqual(second_result.returncode, 0, msg=second_result.stderr)
            second_texts = {manifest_path.name: manifest_path.read_text(encoding="utf-8")}
            second_texts.update(
                {
                    path.name: path.read_text(encoding="utf-8")
                    for path in sorted(shard_dir.glob("mi300a_terminal_discovery_shard_*_matrix.json"))
                }
            )

        self.assertEqual(first_result.stdout, second_result.stdout)
        self.assertEqual(first_texts, second_texts)

    def test_custom_max_attempts_keeps_shards_under_requested_limit(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            manifest = tmp_path / "manifest.json"
            shard_dir = tmp_path / "shards"
            result = subprocess.run(
                [
                    sys.executable,
                    str(SCRIPT),
                    "--plan",
                    str(PLAN),
                    "--manifest-output",
                    str(manifest),
                    "--shard-dir",
                    str(shard_dir),
                    "--max-attempts-per-shard",
                    "128",
                ],
                cwd=REPO_ROOT,
                text=True,
                capture_output=True,
                check=False,
                timeout=30,
            )
            self.assertEqual(result.returncode, 0, msg=result.stderr)
            doc = self.load_json(manifest)

        self.assertEqual(doc["max_attempts_per_shard"], 128)
        self.assertEqual(doc["shard_count"], 12)
        self.assertLessEqual(doc["shard_attempt_count_max"], 128)
        self.assertEqual(doc["attempt_count"], 1416)
        self.assertEqual(doc["visible_matrix_row_count"], 82)


if __name__ == "__main__":
    unittest.main()
