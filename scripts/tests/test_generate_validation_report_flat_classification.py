from __future__ import annotations

import sys
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPTS_DIR = REPO_ROOT / "scripts"
if str(SCRIPTS_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPTS_DIR))

import generate_validation_report as gvr  # noqa: E402


class ValidationReportFlatClassificationTest(unittest.TestCase):
    def fixture_points(
        self,
        kernel: str = "l1_cache_bw",
        *,
        sizes=None,
        real=None,
        sim=None,
    ):
        sizes = sizes or [1024, 2048, 4096, 8192]
        real = real or [0.1200, 0.1210, 0.1220, 0.1230]
        sim = sim or [0.0070, 0.0071, 0.0072, 0.0073]
        hw_points = {
            kernel: [
                {"kernel": kernel, "size": str(size), "real": hw_ms}
                for size, hw_ms in zip(sizes, real)
            ]
        }
        matched_points = {
            kernel: [
                {
                    "kernel": kernel,
                    "size": str(size),
                    "real": hw_ms,
                    "sim": sim_ms,
                }
                for size, hw_ms, sim_ms in zip(sizes, real, sim)
            ]
        }
        return hw_points, matched_points

    def flat_provenance(self, **overrides):
        provenance = {
            "selected_run_id": "unit-test-run",
            "source": "unit-test-metadata",
        }
        provenance.update(overrides)
        return provenance

    def flat_metadata(
        self,
        kernel: str = "l1_cache_bw",
        *,
        hw_points=None,
        matched_points=None,
        **overrides,
    ):
        if hw_points is None or matched_points is None:
            hw_points, matched_points = self.fixture_points(kernel)
        hw_rows = hw_points[kernel]
        matched_rows = matched_points[kernel]
        summary = gvr.summarize_loaded_flat_region_rows(hw_rows, matched_rows)
        metadata = {
            "classification": "true_hardware_flat_region",
            "calibration_policy": "no_fixed_time_ms",
            "selected_run_id": "unit-test-run",
            "source": "unit-test-metadata",
            "scaling_axis": "working_set_size",
            "scaling_axis_role": "cache_residency_sweep",
            "work_amount_scaled_by_axis": False,
            "hardware_rows": summary["hardware_rows"],
            "sim_matched_rows": summary["sim_matched_rows"],
            "problem_size_min": summary["problem_size_min"],
            "problem_size_max": summary["problem_size_max"],
            "hardware_min_ms": summary["hardware_min_ms"],
            "hardware_max_ms": summary["hardware_max_ms"],
            "flat_threshold_ms": summary["flat_threshold_ms"],
            "sim_min_ms": summary["sim_min_ms"],
            "sim_max_ms": summary["sim_max_ms"],
            "reason": "fixture is a fixed-work cache-footprint sweep",
        }
        metadata.update(overrides)
        return {kernel: metadata}

    def work_metadata(self, kernel: str = "l1_cache_bw", **overrides):
        metadata = {
            "classification": "cache_bandwidth_work_sweep",
            "calibration_policy": "score_when_loaded_rows_value_changing",
            "source": f"workloads/microbench/{kernel}/params.json",
            "scaling_axis": "num_elements",
            "scaling_axis_role": "work_amount_sweep",
            "work_amount_scaled_by_axis": True,
            "total_read_bytes_formula": (
                "total_read_bytes = num_elements * num_iterations * bytes_per_read"
            ),
            "work_amount": {
                "total_read_bytes_formula": (
                    "total_read_bytes = num_elements * num_iterations * bytes_per_read"
                ),
                "bytes_per_read": 4,
                "default_num_iterations": 1000,
                "default_working_set_size": 8192,
                "default_total_read_bytes": 262144000,
                "repeat_count_parameter_names": {
                    "hip_cuda": "num_iterations",
                    "go_simulator": "num_repeats",
                },
            },
            "implementation_semantics": {
                "hip_cuda_workload": {
                    "working_set_model": "fixed_total_working_set_size",
                    "working_set_size_bytes": 8192,
                    "repeat_parameter": "num_iterations",
                },
                "go_simulator": {
                    "working_set_model": "per_256_thread_read_group_reuse",
                    "total_input_footprint_fixed": False,
                    "input_bytes_formula": (
                        "ceil(num_elements / 256) * 256 * bytes_per_read"
                    ),
                    "read_group_elements": 256,
                    "read_group_footprint_bytes": 1024,
                    "repeat_parameter": "num_repeats",
                },
            },
            "value_change_proof": {
                "minimum_matched_rows": 3,
                "minimum_distinct_problem_sizes": 3,
                "hardware_min_to_max_ratio_exclusive": 2.0,
            },
            "reason": "fixture work-scaling metadata",
        }
        metadata.update(overrides)
        return {kernel: metadata}

    def combined_l1_l2_metadata(self, kernel: str = "l1_cache_bw"):
        flat = self.flat_metadata(kernel)[kernel]
        work = self.work_metadata(kernel)[kernel]
        return {
            kernel: {
                "hardware_flat_region": flat,
                "work_scaling_region": work,
            }
        }

    def compute_fixture(
        self,
        metadata=None,
        kernel: str = "l1_cache_bw",
        *,
        hw_points=None,
        matched_points=None,
        report_provenance=None,
    ):
        if hw_points is None or matched_points is None:
            hw_points, matched_points = self.fixture_points(kernel)
        classified = gvr.classify_points(
            hw_points,
            matched_points,
            flat_region_metadata=metadata,
            report_provenance=report_provenance or self.flat_provenance(),
        )
        compliance = gvr.check_compliance(classified)
        metrics = gvr.compute_kernel_metrics(classified)
        aggregate = gvr.compute_aggregate_metrics(
            classified,
            metrics,
            compliance,
            tier_lookup={kernel: 1},
        )
        trends = gvr.compute_trend_metric(classified, tier_lookup={kernel: 1})
        return classified, compliance, metrics, aggregate, trends

    def classify_loaded_csv(self, csv_path: Path):
        all_rows, matched_by_kernel, hw_by_kernel = gvr.load_data(csv_path)
        self.assertGreater(len(all_rows), 0)
        return gvr.classify_points(
            hw_by_kernel,
            matched_by_kernel,
            flat_region_metadata=gvr.load_report_classification_metadata(),
            report_provenance=gvr.build_classification_provenance(csv_path),
        )

    def test_constant_work_hardware_flat_sweep_is_diagnostic_not_scored(self) -> None:
        classified, compliance, metrics, aggregate, trends = self.compute_fixture(
            metadata=self.flat_metadata()
        )
        metric = metrics["l1_cache_bw"]
        report_class = classified["l1_cache_bw"]["report_classification"]

        self.assertTrue(compliance["l1_cache_bw"]["compliant"])
        self.assertEqual(
            report_class["display_label"],
            "true HW-flat / constant-work cache-residency sweep",
        )
        self.assertTrue(report_class["metadata_gate"]["passed"])
        self.assertFalse(report_class["boosted_metrics_scored"])
        self.assertFalse(report_class["scaling_informative"])
        self.assertIsNone(metric["avg_err_adj"])
        self.assertIsNotNone(metric["avg_err_adj_diagnostic"])
        self.assertLess(metric["avg_err_adj_diagnostic"], 0.02)
        self.assertIsNone(metric["sf_err_adj"])
        self.assertIsNotNone(metric["sf_err_adj_diagnostic"])
        self.assertEqual(aggregate["boosted_unscored_kernels"], ["l1_cache_bw"])
        self.assertEqual(aggregate["noninformative_scaling_kernels"], ["l1_cache_bw"])
        self.assertEqual(trends, [])

    def test_report_surfaces_diagnostic_policy_for_l1_l2_flat_rows(self) -> None:
        classified, compliance, metrics, aggregate, trends = self.compute_fixture(
            metadata=self.flat_metadata()
        )

        report = gvr.generate_report(
            classified,
            compliance,
            metrics,
            aggregate,
            figures=[],
            trend_results=trends,
            report_context={"tier_lookup": {"l1_cache_bw": 1}},
        )

        self.assertIn("Hardware-Flat / Work-Scaling Sweep Classification", report)
        self.assertIn("Cache-Bandwidth Evidence Snapshot", report)
        self.assertIn("true HW-flat / constant-work cache-residency sweep", report)
        self.assertIn("diagnostic only; excluded from boosted/trend targets", report)
        self.assertIn("not scored (diag", report)
        self.assertIn("Raw avg/W", report)
        self.assertIn("passed archived flat gate; fixed-work diagnostic only", report)
        self.assertIn("true HW-flat (constant-work)", report)
        self.assertIn("Metric policy:** booster and trend/scaling are diagnostic only", report)

    def test_unannotated_flat_curve_keeps_default_booster_scoring(self) -> None:
        classified, _compliance, metrics, aggregate, trends = self.compute_fixture(
            metadata=None,
            kernel="synthetic_flat_kernel",
        )
        metric = metrics["synthetic_flat_kernel"]
        report_class = classified["synthetic_flat_kernel"]["report_classification"]

        self.assertEqual(report_class["display_label"], "auto hardware-flat region")
        self.assertTrue(report_class["boosted_metrics_scored"])
        self.assertTrue(report_class["scaling_informative"])
        self.assertIsNotNone(metric["avg_err_adj"])
        self.assertEqual(metric["avg_err_adj"], metric["avg_err_adj_diagnostic"])
        self.assertEqual(aggregate["boosted_unscored_kernels"], [])
        self.assertEqual(aggregate["noninformative_scaling_kernels"], [])
        self.assertEqual(len(trends), 1)

    def test_work_scaled_nonflat_rows_are_scored_and_trended(self) -> None:
        kernel = "l1_cache_bw"
        hw_points, matched_points = self.fixture_points(
            kernel,
            sizes=[1024, 2048, 4096, 8192, 16384],
            real=[1.0, 1.15, 2.25, 3.4, 4.8],
            sim=[0.8, 1.0, 2.0, 3.2, 4.5],
        )

        classified, _compliance, metrics, aggregate, trends = self.compute_fixture(
            metadata=self.work_metadata(kernel),
            kernel=kernel,
            hw_points=hw_points,
            matched_points=matched_points,
        )
        report_class = classified[kernel]["report_classification"]

        self.assertEqual(report_class["display_label"], "cache bandwidth work-scaling sweep")
        self.assertTrue(report_class["work_amount_scaled_by_axis"])
        self.assertEqual(
            report_class["total_read_bytes_formula"],
            "total_read_bytes = num_elements * num_iterations * bytes_per_read",
        )
        self.assertEqual(
            report_class["repeat_count_parameter_names"],
            {"hip_cuda": "num_iterations", "go_simulator": "num_repeats"},
        )
        simulator_semantics = report_class["implementation_semantics"]["go_simulator"]
        self.assertEqual(
            simulator_semantics["working_set_model"],
            "per_256_thread_read_group_reuse",
        )
        self.assertTrue(report_class["value_change_gate"]["passed"])
        self.assertTrue(report_class["value_change_proven"])
        self.assertEqual(report_class["metric_policy"], "scored")
        self.assertTrue(report_class["boosted_metrics_scored"])
        self.assertTrue(report_class["scaling_informative"])
        self.assertIsNotNone(metrics[kernel]["avg_err_adj"])
        self.assertEqual(aggregate["boosted_unscored_kernels"], [])
        self.assertEqual([entry["kernel"] for entry in trends], [kernel])

    def test_work_scaled_flat_rows_are_diagnostic_until_value_changing(self) -> None:
        kernel = "l1_cache_bw"

        classified, _compliance, metrics, aggregate, trends = self.compute_fixture(
            metadata=self.work_metadata(kernel),
            kernel=kernel,
        )
        report_class = classified[kernel]["report_classification"]

        self.assertEqual(
            report_class["display_label"],
            "cache bandwidth work-scaling sweep (pending value-change proof)",
        )
        self.assertTrue(report_class["work_amount_scaled_by_axis"])
        self.assertFalse(report_class["value_change_gate"]["passed"])
        self.assertIn(
            "loaded_hardware_value_changing",
            report_class["value_change_gate"]["reason"],
        )
        self.assertEqual(
            report_class["metric_policy"],
            "diagnostic_until_value_changing_rows",
        )
        self.assertFalse(report_class["boosted_metrics_scored"])
        self.assertFalse(report_class["scaling_informative"])
        self.assertIsNone(metrics[kernel]["avg_err_adj"])
        self.assertIsNotNone(metrics[kernel]["avg_err_adj_diagnostic"])
        self.assertEqual(aggregate["boosted_unscored_kernels"], [kernel])
        self.assertEqual(trends, [])

    def test_report_surfaces_l1_work_sweep_footprint_semantics(self) -> None:
        kernel = "l1_cache_bw"
        classified, compliance, metrics, aggregate, trends = self.compute_fixture(
            metadata=self.work_metadata(kernel),
            kernel=kernel,
        )

        report = gvr.generate_report(
            classified,
            compliance,
            metrics,
            aggregate,
            figures=[],
            trend_results=trends,
            report_context={"tier_lookup": {kernel: 1}},
        )

        self.assertIn("`num_elements` scales total reads", report)
        self.assertIn(
            "total_read_bytes = num_elements * num_iterations * bytes_per_read",
            report,
        )
        self.assertIn("HIP/CUDA default 262144000 B", report)
        self.assertIn("Go simulator `num_repeats`", report)
        self.assertIn("HIP/CUDA fixed working_set_size 8192 B", report)
        self.assertIn("Go simulator 256-thread read groups (1024 B each)", report)
        self.assertIn("Cache-Bandwidth Evidence Snapshot", report)
        self.assertIn("HW max/min", report)
        self.assertIn("value-change proof failed: loaded_hardware_value_changing", report)
        self.assertIn(
            "input bytes = ceil(num_elements / 256) * 256 * bytes_per_read",
            report,
        )

    def test_nonflat_loaded_rows_fall_back_to_normal_scored_classification(self) -> None:
        kernel = "l1_cache_bw"
        hw_points, matched_points = self.fixture_points(
            kernel,
            sizes=[1, 2, 3, 4, 5],
            real=[1.0, 1.2, 2.4, 3.1, 4.2],
            sim=[0.9, 1.1, 2.2, 3.0, 4.1],
        )
        metadata = self.flat_metadata(
            kernel,
            hw_points=hw_points,
            matched_points=matched_points,
        )

        classified, _compliance, metrics, aggregate, trends = self.compute_fixture(
            metadata=metadata,
            kernel=kernel,
            hw_points=hw_points,
            matched_points=matched_points,
        )
        report_class = classified[kernel]["report_classification"]

        self.assertEqual(report_class["display_label"], "threshold flat/non-flat regions")
        self.assertFalse(report_class["has_metadata"])
        self.assertFalse(report_class["true_hardware_flat"])
        self.assertFalse(report_class["metadata_gate"]["passed"])
        self.assertIn("loaded_hardware_rows_all_flat", report_class["metadata_gate"]["reason"])
        self.assertTrue(report_class["boosted_metrics_scored"])
        self.assertTrue(report_class["scaling_informative"])
        self.assertIsNotNone(metrics[kernel]["avg_err_adj"])
        self.assertEqual(aggregate["boosted_unscored_kernels"], [])
        self.assertEqual(aggregate["noninformative_scaling_kernels"], [])
        self.assertEqual([entry["kernel"] for entry in trends], [kernel])

    def test_selected_run_source_mismatch_falls_back_to_scored_flat_classification(self) -> None:
        kernel = "l1_cache_bw"
        metadata = self.flat_metadata(kernel)

        classified, _compliance, metrics, aggregate, trends = self.compute_fixture(
            metadata=metadata,
            kernel=kernel,
            report_provenance=self.flat_provenance(
                selected_run_id="different-run",
                source="different-source",
            ),
        )
        report_class = classified[kernel]["report_classification"]

        self.assertEqual(report_class["display_label"], "auto hardware-flat region")
        self.assertFalse(report_class["has_metadata"])
        self.assertFalse(report_class["metadata_gate"]["passed"])
        self.assertIn("selected_run_id", report_class["metadata_gate"]["reason"])
        self.assertIn("source", report_class["metadata_gate"]["reason"])
        self.assertTrue(report_class["boosted_metrics_scored"])
        self.assertIsNotNone(metrics[kernel]["avg_err_adj"])
        self.assertEqual(aggregate["boosted_unscored_kernels"], [])
        self.assertEqual([entry["kernel"] for entry in trends], [kernel])

    def test_production_top_level_csv_rejects_selected_run_flat_metadata(self) -> None:
        csv_path = REPO_ROOT / "benchmark-comparison" / "comparison_ci.csv"
        selected_source = (
            "benchmark-comparison/selected-run-25619929396/"
            "comparison_ci.detailed.csv"
        )

        classified = self.classify_loaded_csv(csv_path)

        for kernel in ("l1_cache_bw", "l2_cache_bw"):
            with self.subTest(kernel=kernel):
                report_class = classified[kernel]["report_classification"]
                archived_gate = report_class["archived_metadata_gate"]
                value_change_gate = report_class["value_change_gate"]
                source_check = next(
                    check
                    for check in archived_gate["checks"]
                    if check["name"] == "source/data_source"
                )

                self.assertEqual(
                    report_class["display_label"],
                    "cache bandwidth work-scaling sweep (pending value-change proof)",
                )
                self.assertTrue(report_class["has_metadata"])
                self.assertFalse(report_class["true_hardware_flat"])
                self.assertTrue(report_class["work_amount_scaled_by_axis"])
                self.assertEqual(
                    report_class["metric_policy"],
                    "diagnostic_until_value_changing_rows",
                )
                self.assertFalse(report_class["boosted_metrics_scored"])
                self.assertFalse(report_class["scaling_informative"])
                self.assertTrue(archived_gate["required"])
                self.assertFalse(archived_gate["passed"])
                self.assertIn("source/data_source", archived_gate["reason"])
                self.assertFalse(source_check["passed"])
                self.assertEqual(
                    source_check["actual"],
                    "benchmark-comparison/comparison_ci.csv",
                )
                self.assertEqual(source_check["expected"], selected_source)
                self.assertTrue(value_change_gate["required"])
                self.assertFalse(value_change_gate["passed"])
                self.assertIn("loaded_hardware_value_changing", value_change_gate["reason"])

    def test_selected_run_csv_keeps_same_source_flat_metadata(self) -> None:
        csv_path = (
            REPO_ROOT
            / "benchmark-comparison"
            / "selected-run-25619929396"
            / "comparison_ci.detailed.csv"
        )
        selected_source = (
            "benchmark-comparison/selected-run-25619929396/"
            "comparison_ci.detailed.csv"
        )

        classified = self.classify_loaded_csv(csv_path)

        for kernel in ("l1_cache_bw", "l2_cache_bw"):
            with self.subTest(kernel=kernel):
                report_class = classified[kernel]["report_classification"]
                metadata_gate = report_class["metadata_gate"]
                source_check = next(
                    check
                    for check in metadata_gate["checks"]
                    if check["name"] == "source/data_source"
                )

                self.assertEqual(
                    report_class["display_label"],
                    "true HW-flat / constant-work cache-residency sweep",
                )
                self.assertTrue(report_class["has_metadata"])
                self.assertTrue(report_class["true_hardware_flat"])
                self.assertEqual(
                    report_class["metric_policy"],
                    "diagnostic_only_constant_work_flat_region",
                )
                self.assertFalse(report_class["boosted_metrics_scored"])
                self.assertFalse(report_class["scaling_informative"])
                self.assertTrue(metadata_gate["passed"])
                self.assertTrue(source_check["passed"])
                self.assertEqual(source_check["actual"], selected_source)
                self.assertEqual(source_check["expected"], selected_source)

    def test_count_and_minmax_mismatches_fall_back_to_scored_flat_classification(self) -> None:
        kernel = "l1_cache_bw"
        mismatches = {
            "hardware_rows": 99,
            "sim_matched_rows": 99,
            "problem_size_min": 999,
            "problem_size_max": 999,
            "hardware_min_ms": 9.9,
            "hardware_max_ms": 9.9,
            "flat_threshold_ms": 9.9,
            "sim_min_ms": 9.9,
            "sim_max_ms": 9.9,
        }

        for field, value in mismatches.items():
            with self.subTest(field=field):
                metadata = self.flat_metadata(kernel, **{field: value})
                classified, _compliance, metrics, aggregate, _trends = self.compute_fixture(
                    metadata=metadata,
                    kernel=kernel,
                )
                report_class = classified[kernel]["report_classification"]

                self.assertEqual(
                    report_class["display_label"],
                    "auto hardware-flat region",
                )
                self.assertFalse(report_class["has_metadata"])
                self.assertFalse(report_class["metadata_gate"]["passed"])
                self.assertIn(field, report_class["metadata_gate"]["reason"])
                self.assertTrue(report_class["boosted_metrics_scored"])
                self.assertIsNotNone(metrics[kernel]["avg_err_adj"])
                self.assertEqual(aggregate["boosted_unscored_kernels"], [])


if __name__ == "__main__":
    unittest.main()
