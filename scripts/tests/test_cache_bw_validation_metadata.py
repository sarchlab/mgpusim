from __future__ import annotations

import csv
import json
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
SELECTED_COMPARISON = (
    REPO_ROOT
    / "benchmark-comparison"
    / "selected-run-25619929396"
    / "comparison_ci.detailed.csv"
)
DISCOVERY_PLAN = REPO_ROOT / "benchmark-comparison" / "mi300a_problem_size_discovery_plan.json"
REGULAR_TIER1_MATRIX = (
    REPO_ROOT / "benchmark-comparison" / "generated" / "mi300a_regular_tier1_matrix.json"
)
TIER1_HARDWARE_RUNNER = REPO_ROOT / "scripts" / "run_tier1_hardware.sh"

CACHE_BW_BENCHMARKS = {
    "l1_cache_bw": {
        "default_num_elements": 65536,
        "default_working_set_size": 8192,
        "default_num_iterations": 1000,
        "problem_size_min": 1024,
        "problem_size_max": 65536,
        "hardware_rows": 16,
        "sim_matched_rows": 16,
    },
    "l2_cache_bw": {
        "default_num_elements": 262144,
        "default_working_set_size": 524288,
        "default_num_iterations": 500,
        "problem_size_min": 32768,
        "problem_size_max": 4194304,
        "hardware_rows": 16,
        "sim_matched_rows": 5,
    },
}


class CacheBandwidthValidationMetadataTest(unittest.TestCase):
    def test_params_mark_num_elements_as_work_scaling(self) -> None:
        for benchmark, expected in CACHE_BW_BENCHMARKS.items():
            with self.subTest(benchmark=benchmark):
                params = self._load_params(benchmark)

                self.assertEqual(params["benchmark"], benchmark)
                self.assertEqual(params["suite"], "microbench")
                self.assertEqual(params["scaling_parameter"]["name"], "num_elements")
                self.assertEqual(
                    params["scaling_parameter"]["scaling_role"],
                    "work_amount_sweep",
                )
                self.assertIs(params["scaling_parameter"]["work_amount_scaled"], True)

                work_model = params["work_model"]
                self.assertEqual(
                    work_model["default_num_elements"],
                    expected["default_num_elements"],
                )
                self.assertEqual(
                    work_model["default_working_set_size"],
                    expected["default_working_set_size"],
                )
                self.assertEqual(
                    work_model["default_num_iterations"],
                    expected["default_num_iterations"],
                )
                self.assertEqual(work_model["bytes_per_read"], 4)
                self.assertEqual(
                    work_model["total_read_bytes_formula"],
                    "total_read_bytes = num_elements * num_iterations * bytes_per_read",
                )
                self.assertEqual(
                    work_model["default_total_read_bytes"],
                    work_model["default_num_elements"]
                    * work_model["default_num_iterations"]
                    * work_model["bytes_per_read"],
                )
                self.assertEqual(
                    work_model["repeat_count_parameter_names"],
                    {
                        "hip_cuda": "num_iterations",
                        "go_simulator": "num_repeats",
                    },
                )

                semantics = params["validation_semantics"]
                self.assertEqual(semantics["classification"], "cache_bandwidth_work_sweep")
                self.assertIs(semantics["work_amount_scaled_by_axis"], True)
                self.assertIs(
                    semantics["flat_region_is_valid_when_residency_unchanged"],
                    False,
                )

    def test_l1_metadata_distinguishes_simulator_read_group_reuse(self) -> None:
        params = self._load_params("l1_cache_bw")
        work_model = params["work_model"]
        semantics = params["validation_semantics"]

        self.assertIn("HIP/CUDA rows use a fixed", params["measurement_intent"])
        self.assertIn("Go simulator rows use per-256-thread", params["measurement_intent"])
        self.assertEqual(
            work_model["simulator_input_bytes_formula"],
            "ceil(num_elements / 256) * 256 * bytes_per_read",
        )
        self.assertEqual(work_model["simulator_read_group_elements"], 256)
        self.assertEqual(work_model["simulator_read_group_footprint_bytes"], 1024)
        self.assertEqual(work_model["default_simulator_num_repeats"], 256)
        self.assertIs(semantics["hip_cuda_total_input_footprint_fixed"], True)
        self.assertIs(semantics["go_simulator_total_input_footprint_fixed"], False)
        self.assertEqual(
            semantics["go_simulator_working_set_model"],
            "per_256_thread_read_group_reuse",
        )
        self.assertIn(
            "not exposed by the current Go simulator sample",
            params["non_scaling_parameters"]["working_set_size"]["description"],
        )

    def test_regular_plan_l1_l2_flags_select_num_elements_axis(self) -> None:
        plan = json.loads(DISCOVERY_PLAN.read_text(encoding="utf-8"))
        entries = plan["entries"]

        for benchmark in CACHE_BW_BENCHMARKS:
            with self.subTest(benchmark=benchmark):
                params = self._load_params(benchmark)
                expected_values = [str(v) for v in params["scaling_parameter"]["values"]]
                rows = [row for row in entries if row["benchmark"] == benchmark]

                self.assertEqual([row["size_token"] for row in rows], expected_values)
                self.assertTrue(rows)
                for row in rows:
                    self.assertEqual(row["flag_template"], "-num_elements {size}")
                    self.assertEqual(row["size_label_template"], "{size}")

    def test_tier1_hardware_runner_l1_l2_flags_select_num_elements_axis(self) -> None:
        bench_table = self._tier1_hardware_bench_table()

        for benchmark in CACHE_BW_BENCHMARKS:
            with self.subTest(benchmark=benchmark):
                self.assertIn(benchmark, bench_table)
                row = bench_table[benchmark]
                self.assertEqual(row["workload_subdir"], f"microbench/{benchmark}")
                self.assertEqual(row["binary_name"], benchmark)
                self.assertEqual(row["flag_name"], "--num_elements")
                self.assertNotEqual(row["flag_name"], "--working_set_size")
                self.assertEqual(row["size_unit"], "raw")

    def test_tier1_hardware_runner_has_cache_bandwidth_axis_preflight(self) -> None:
        text = TIER1_HARDWARE_RUNNER.read_text(encoding="utf-8")

        self.assertIn("validate_cache_bw_axis_contract", text)
        self.assertIn("cache-bandwidth axis contract failed", text)
        self.assertIn("stale plan flag_template rows", text)
        self.assertIn("--num_elements", text)
        self.assertNotIn(
            "expected_runner_flags = {\n    \"l1_cache_bw\": \"--working_set_size\"",
            text,
        )

    def test_generated_regular_tier1_matrix_l1_l2_flags_select_num_elements_axis(self) -> None:
        matrix = json.loads(REGULAR_TIER1_MATRIX.read_text(encoding="utf-8"))
        rows = {row["benchmark"]: row for row in matrix["include"]}

        for benchmark in CACHE_BW_BENCHMARKS:
            with self.subTest(benchmark=benchmark):
                params = self._load_params(benchmark)
                row = rows[benchmark]
                expected_values = [str(v) for v in params["scaling_parameter"]["values"]]

                self.assertEqual(row["flag_template"], "-num_elements {size}")
                self.assertNotEqual(row["flag_template"], "-working_set_size {size}")
                self.assertEqual(row["size_label_template"], "{size}")
                self.assertEqual(row["source_size_tokens"], [expected_values[0]])
                expected_plan_index = 17 if benchmark == "l1_cache_bw" else 33
                self.assertEqual(row["source_plan_indices"], [expected_plan_index])

    def test_flat_region_manifest_disables_l1_l2_fixed_time_calibration(self) -> None:
        tiers = json.loads(
            (REPO_ROOT / "scripts" / "benchmark_tiers.json").read_text(
                encoding="utf-8"
            )
        )
        fixed_time = tiers.get("per_benchmark_fixed_time_ms", {})
        flat_regions = tiers.get("hardware_flat_regions", {})

        for benchmark, expected in CACHE_BW_BENCHMARKS.items():
            with self.subTest(benchmark=benchmark):
                self.assertNotIn(benchmark, fixed_time)
                region = flat_regions[benchmark]
                self.assertEqual(region["classification"], "true_hardware_flat_region")
                self.assertEqual(region["calibration_policy"], "no_fixed_time_ms")
                self.assertEqual(region["selected_run_id"], "25619929396")
                source = region["source"]
                self.assertNotIn("knowledge/", source)
                self.assertEqual(
                    source,
                    "benchmark-comparison/selected-run-25619929396/"
                    "comparison_ci.detailed.csv",
                )
                self.assertTrue((REPO_ROOT / source).is_file())
                self.assertEqual(region["scaling_axis"], "working_set_size")
                self.assertEqual(region["scaling_axis_role"], "cache_residency_sweep")
                self.assertIs(region["work_amount_scaled_by_axis"], False)
                self.assertEqual(region["hardware_rows"], expected["hardware_rows"])
                self.assertEqual(region["sim_matched_rows"], expected["sim_matched_rows"])
                self.assertEqual(
                    region["problem_size_min"],
                    expected["problem_size_min"],
                )
                self.assertEqual(
                    region["problem_size_max"],
                    expected["problem_size_max"],
                )

    def test_work_sweep_manifest_marks_new_axis_and_total_read_bytes(self) -> None:
        tiers = json.loads(
            (REPO_ROOT / "scripts" / "benchmark_tiers.json").read_text(
                encoding="utf-8"
            )
        )
        work_sweeps = tiers["cache_bandwidth_work_sweeps"]

        for benchmark, expected in CACHE_BW_BENCHMARKS.items():
            with self.subTest(benchmark=benchmark):
                region = work_sweeps[benchmark]
                params = self._load_params(benchmark)
                work_model = params["work_model"]

                self.assertEqual(region["classification"], "cache_bandwidth_work_sweep")
                self.assertEqual(region["scaling_axis"], "num_elements")
                self.assertEqual(region["scaling_axis_role"], "work_amount_sweep")
                self.assertIs(region["work_amount_scaled_by_axis"], True)
                self.assertIs(region["work_amount_scaled"], True)
                self.assertEqual(
                    region["total_read_bytes_formula"],
                    "total_read_bytes = num_elements * num_iterations * bytes_per_read",
                )
                self.assertEqual(
                    region["work_amount"]["total_read_bytes_formula"],
                    work_model["total_read_bytes_formula"],
                )
                self.assertEqual(
                    region["work_amount"]["default_total_read_bytes"],
                    work_model["default_total_read_bytes"],
                )
                self.assertEqual(
                    region["work_amount"]["repeat_count_parameter_names"],
                    work_model["repeat_count_parameter_names"],
                )
                if benchmark == "l1_cache_bw":
                    simulator = region["implementation_semantics"]["go_simulator"]
                    self.assertEqual(
                        simulator["working_set_model"],
                        "per_256_thread_read_group_reuse",
                    )
                    self.assertIs(simulator["total_input_footprint_fixed"], False)
                    self.assertEqual(
                        simulator["input_bytes_formula"],
                        work_model["simulator_input_bytes_formula"],
                    )
                    self.assertIn("total input allocation grows", region["reason"])
                self.assertEqual(
                    region["value_change_proof"]["minimum_matched_rows"],
                    3,
                )
                self.assertEqual(
                    region["value_change_proof"][
                        "hardware_min_to_max_ratio_exclusive"
                    ],
                    2.0,
                )

    def test_flat_region_manifest_matches_selected_run_rows(self) -> None:
        tiers = json.loads(
            (REPO_ROOT / "scripts" / "benchmark_tiers.json").read_text(
                encoding="utf-8"
            )
        )
        flat_regions = tiers["hardware_flat_regions"]
        rows_by_benchmark = self._selected_rows_by_benchmark()

        for benchmark in CACHE_BW_BENCHMARKS:
            with self.subTest(benchmark=benchmark):
                region = flat_regions[benchmark]
                rows = rows_by_benchmark[benchmark]
                real_ms = [float(row["real_ms"]) for row in rows]
                sim_ms = [float(row["sim_ms"]) for row in rows if row["sim_ms"].strip()]
                problem_sizes = [int(row["problem_size"]) for row in rows]
                flat_threshold = 2 * min(real_ms)

                self.assertEqual(len(rows), region["hardware_rows"])
                self.assertEqual(len(sim_ms), region["sim_matched_rows"])
                self.assertEqual(min(problem_sizes), region["problem_size_min"])
                self.assertEqual(max(problem_sizes), region["problem_size_max"])
                self.assertAlmostEqual(min(real_ms), region["hardware_min_ms"], places=6)
                self.assertAlmostEqual(max(real_ms), region["hardware_max_ms"], places=6)
                self.assertAlmostEqual(
                    round(flat_threshold, 6),
                    region["flat_threshold_ms"],
                    places=6,
                )
                self.assertTrue(
                    all(value <= flat_threshold for value in real_ms),
                    f"{benchmark} hardware rows should all be in the declared flat region",
                )
                self.assertAlmostEqual(min(sim_ms), region["sim_min_ms"], places=6)
                self.assertAlmostEqual(max(sim_ms), region["sim_max_ms"], places=6)

    def _load_params(self, benchmark: str) -> dict:
        path = REPO_ROOT / "workloads" / "microbench" / benchmark / "params.json"
        return json.loads(path.read_text(encoding="utf-8"))

    def _tier1_hardware_bench_table(self) -> dict[str, dict[str, str]]:
        rows: dict[str, dict[str, str]] = {}
        in_table = False
        for line in TIER1_HARDWARE_RUNNER.read_text(encoding="utf-8").splitlines():
            if "BENCH_TABLE <<'TABLE'" in line:
                in_table = True
                continue
            if in_table and line == "TABLE":
                break
            if not in_table or not line.strip():
                continue

            fields = line.split("|")
            self.assertEqual(len(fields), 5, f"malformed BENCH_TABLE row: {line}")
            plan_name, workload_subdir, binary_name, flag_name, size_unit = fields
            rows[plan_name] = {
                "workload_subdir": workload_subdir,
                "binary_name": binary_name,
                "flag_name": flag_name,
                "size_unit": size_unit,
            }

        self.assertTrue(rows, "could not parse BENCH_TABLE from run_tier1_hardware.sh")
        return rows

    def _selected_rows_by_benchmark(self) -> dict[str, list[dict[str, str]]]:
        rows_by_benchmark: dict[str, list[dict[str, str]]] = {
            benchmark: [] for benchmark in CACHE_BW_BENCHMARKS
        }
        with SELECTED_COMPARISON.open(newline="", encoding="utf-8") as fh:
            for row in csv.DictReader(fh):
                benchmark = row.get("kernel_name", "")
                if benchmark in rows_by_benchmark and row.get("real_ms", "").strip():
                    rows_by_benchmark[benchmark].append(row)
        return rows_by_benchmark


if __name__ == "__main__":
    unittest.main()
