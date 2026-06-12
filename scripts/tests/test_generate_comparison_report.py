"""Tests for scripts/generate_comparison_report.py."""

from __future__ import annotations

import csv
import errno
import io
import json
import math
import os
import sys
import tempfile
import threading
import time
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPTS_DIR = REPO_ROOT / "scripts"
if str(SCRIPTS_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPTS_DIR))

import generate_comparison_report as gcr  # noqa: E402


class ComputeErrorMetricsTest(unittest.TestCase):
    def test_known_values(self):
        # hw=1.0, sim=1.5  -> err=0.5,  sym=abs(0.5)/min(1.5,1.0)=0.5
        # hw=2.0, sim=1.0  -> err=0.5,  sym=abs(-1.0)/min(1.0,2.0)=1.0
        metrics = gcr.compute_error_metrics([1.0, 2.0], [1.5, 1.0])
        self.assertAlmostEqual(metrics["mean_err"], 0.5)
        self.assertAlmostEqual(metrics["max_err"], 0.5)
        self.assertAlmostEqual(metrics["mean_sym_err"], 0.75)
        self.assertAlmostEqual(metrics["max_sym_err"], 1.0)

    def test_zero_error_when_equal(self):
        metrics = gcr.compute_error_metrics([1.0, 2.0, 3.0], [1.0, 2.0, 3.0])
        self.assertEqual(metrics["mean_err"], 0.0)
        self.assertEqual(metrics["max_err"], 0.0)
        self.assertEqual(metrics["mean_sym_err"], 0.0)
        self.assertEqual(metrics["max_sym_err"], 0.0)

    def test_empty_input_returns_nan(self):
        metrics = gcr.compute_error_metrics([], [])
        for key in ("mean_err", "max_err", "mean_sym_err", "max_sym_err"):
            self.assertTrue(math.isnan(metrics[key]), f"{key} should be nan")

    def test_skips_nan_and_nonpositive_pairs(self):
        # Only the (1.0, 2.0) pair is valid -> err=1.0, sym=1.0
        hw = [1.0, float("nan"), 0.0, 4.0]
        sim = [2.0, 1.0, 1.0, 0.0]
        metrics = gcr.compute_error_metrics(hw, sim)
        self.assertAlmostEqual(metrics["mean_err"], 1.0)
        self.assertAlmostEqual(metrics["max_err"], 1.0)
        self.assertAlmostEqual(metrics["mean_sym_err"], 1.0)
        self.assertAlmostEqual(metrics["max_sym_err"], 1.0)

    def test_skips_none_pairs(self):
        metrics = gcr.compute_error_metrics([None, 1.0], [1.0, None])
        self.assertTrue(math.isnan(metrics["mean_err"]))
        self.assertTrue(math.isnan(metrics["max_sym_err"]))

    def test_length_mismatch_raises(self):
        with self.assertRaises(ValueError):
            gcr.compute_error_metrics([1.0, 2.0], [1.0])

    def test_max_picks_largest(self):
        # hw=1, sim=2 -> err=1.0, sym=1.0
        # hw=10, sim=11 -> err=0.1, sym=0.1
        metrics = gcr.compute_error_metrics([1.0, 10.0], [2.0, 11.0])
        self.assertAlmostEqual(metrics["max_err"], 1.0)
        self.assertAlmostEqual(metrics["max_sym_err"], 1.0)
        self.assertAlmostEqual(metrics["mean_err"], 0.55)


class ExtractScalingValueTest(unittest.TestCase):
    """Tests for extract_scaling_value()."""

    def test_pure_integer(self):
        self.assertEqual(gcr.extract_scaling_value("1024"), 1024.0)
        self.assertEqual(gcr.extract_scaling_value("64"), 64.0)

    def test_nxn_dimension(self):
        self.assertEqual(gcr.extract_scaling_value("128x128"), 128.0)
        self.assertEqual(gcr.extract_scaling_value("256x256"), 256.0)

    def test_nxnxn_dimension(self):
        self.assertEqual(gcr.extract_scaling_value("64x64x64"), 64.0)

    def test_known_scaling_values_hint(self):
        # "rows1024_hid4096_bs256" → 1024 is the first match in values
        values = [64, 128, 256, 512, 1024, 2048, 4096]
        result = gcr.extract_scaling_value("rows1024_hid4096_bs256_layernorm", values)
        self.assertEqual(result, 1024.0)

    def test_prefixed_n_format(self):
        # "n1572864_bs256_ept1_p1" → first matching value
        values = [49152, 196608, 786432, 1572864, 3145728]
        result = gcr.extract_scaling_value("n1572864_bs256_ept1_p1", values)
        self.assertEqual(result, 1572864.0)

    def test_m_prefix_format(self):
        # "M128_N768_K768_tile16" → M=128 is the scaling param
        values = [1, 8, 32, 64, 128, 256, 512, 1024]
        result = gcr.extract_scaling_value("M128_N768_K768_tile16", values)
        self.assertEqual(result, 128.0)

    def test_fallback_first_integer(self):
        # No scaling values hint → use first integer found
        result = gcr.extract_scaling_value("n16777216_bs256_ept1_gelu")
        self.assertEqual(result, 16777216.0)

    def test_no_integers_returns_nan(self):
        result = gcr.extract_scaling_value("nointegers")
        self.assertTrue(math.isnan(result))


class FormatSizeSpanTest(unittest.TestCase):
    """Tests for finishability problem-size range rendering."""

    def test_b4_seq_len_span_uses_s_token_not_batch_constant(self):
        params = {
            "scaling_parameter": {
                "name": "seq_len",
                "values": [32, 64, 96, 128, 768],
            },
        }
        values = [
            "B4_S128_H12_D64_blk256",
            "B4_S32_H12_D64_blk256",
            "B4_S64_H12_D64_blk256",
            "B4_S768_H12_D64_blk256",
            "B4_S96_H12_D64_blk256",
        ]

        self.assertEqual(
            gcr._format_size_span(values, params),
            "B4_S32_H12_D64_blk256–B4_S768_H12_D64_blk256",
        )

    def test_span_falls_back_to_known_scaling_values(self):
        params = {
            "scaling_parameter": {
                "name": "custom_size",
                "values": [16, 32, 64, 128, 256],
            },
        }
        values = [
            "batch4_size16_mode1",
            "batch4_size32_mode1",
            "batch4_size64_mode1",
            "batch4_size128_mode1",
            "batch4_size256_mode1",
        ]

        self.assertEqual(
            gcr._format_size_span(values, params),
            "batch4_size16_mode1–batch4_size256_mode1",
        )


class LoadBenchmarkParamsTest(unittest.TestCase):
    """Tests for load_benchmark_params()."""

    def test_known_kernel_loads_params(self):
        # polybench/2DConvolution has a params.json in the live repo
        params = gcr.load_benchmark_params("2dconvolution", REPO_ROOT)
        if params is not None:  # may be None in minimal test envs
            self.assertIn("scaling_parameter", params)
            self.assertEqual(params["scaling_parameter"]["name"], "size")

    def test_unknown_kernel_returns_none(self):
        result = gcr.load_benchmark_params("nonexistent_kernel_xyz", REPO_ROOT)
        self.assertIsNone(result)

    def test_missing_params_file_returns_none(self):
        with tempfile.TemporaryDirectory() as tmp:
            result = gcr.load_benchmark_params("2dconvolution", Path(tmp))
            self.assertIsNone(result)

    def test_params_with_fake_repo(self):
        """Verify that a manually created params.json is read correctly."""
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            params_dir = tmp_path / "workloads" / "polybench" / "2DConvolution"
            params_dir.mkdir(parents=True)
            params_data = {
                "benchmark": "2DConvolution",
                "suite": "polybench",
                "scaling_parameter": {
                    "name": "size",
                    "values": [64, 128, 256],
                },
                "non_scaling_parameters": {
                    "block_size": {"values": [8, 16, 32]},
                },
            }
            (params_dir / "params.json").write_text(
                json.dumps(params_data), encoding="utf-8"
            )
            result = gcr.load_benchmark_params("2dconvolution", tmp_path)
            self.assertIsNotNone(result)
            self.assertEqual(result["scaling_parameter"]["name"], "size")


class ReportPipelineTest(unittest.TestCase):
    def _write_csv(self, path: Path, rows, extra_cols=None):
        header = ["kernel_name", "problem_size", "real_ms", "sim_ms",
                  "abs_error_ms", "rel_error"]
        if extra_cols:
            header += extra_cols
        with path.open("w", newline="") as fh:
            writer = csv.writer(fh)
            writer.writerow(header)
            for row in rows:
                writer.writerow(row)

    def _write_cache_bw_tiers(self, repo_root: Path, run_id: str = "123") -> None:
        scripts_dir = repo_root / "scripts"
        scripts_dir.mkdir(exist_ok=True)
        selected_source = (
            f"benchmark-comparison/selected-run-{run_id}/comparison_ci.detailed.csv"
        )
        tiers = {
            "tier1": ["l1_cache_bw", "l2_cache_bw"],
            "hardware_flat_regions": {
                "l1_cache_bw": {
                    "classification": "true_hardware_flat_region",
                    "selected_run_id": run_id,
                    "source": selected_source,
                    "scaling_axis": "working_set_size",
                    "scaling_axis_role": "cache_residency_sweep",
                    "work_amount_scaled_by_axis": False,
                    "hardware_rows": 3,
                    "sim_matched_rows": 3,
                }
            },
            "cache_bandwidth_work_sweeps": {
                "l1_cache_bw": {
                    "classification": "cache_bandwidth_work_sweep",
                    "source": "workloads/microbench/l1_cache_bw/params.json",
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
                }
            },
        }
        (scripts_dir / "benchmark_tiers.json").write_text(
            json.dumps(tiers), encoding="utf-8"
        )

    def test_generate_report_pipeline_with_mock_data(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            csv_path = tmp_path / "comparison_ci.csv"
            figures_dir = tmp_path / "figures"
            report_path = tmp_path / "accuracy_report.md"

            self._write_csv(
                csv_path,
                [
                    # kernel_a: two matched rows — plain integer problem sizes
                    ["kernel_a", "128", 1.0, 2.0, 1.0, 1.0],
                    ["kernel_a", "256", 4.0, 5.0, 1.0, 0.25],
                    # kernel_b: one matched, one HW-only (must be skipped)
                    ["kernel_b", "64x64", 2.0, 3.0, 1.0, 0.5],
                    ["kernel_b", "128x128", 8.0, "", "", ""],
                    # kernel_c: only sim_ms (must be skipped entirely)
                    ["kernel_c", "1024", "", 0.5, "", ""],
                ],
            )

            result = gcr.generate_report(
                csv_path=str(csv_path),
                figures_dir=str(figures_dir),
                report_path=str(report_path),
                figures_rel_dir="figures",
                repo_root=tmp_path,  # no params.json present → graceful fallback
            )

            # kernel_c had no matched rows so it must be excluded.
            kernels = [entry[0] for entry in result["per_kernel"]]
            self.assertEqual(kernels, ["kernel_a", "kernel_b"])

            # Charts created with the expected naming convention.
            self.assertIn("kernel_a_comparison.png", result["charts"])
            self.assertIn("kernel_b_comparison.png", result["charts"])
            for chart in result["charts"]:
                self.assertTrue((figures_dir / chart).is_file(),
                                f"missing chart {chart}")

            # Report exists and references the charts + tier-2 sections.
            self.assertTrue(report_path.is_file())
            text = report_path.read_text(encoding="utf-8")
            self.assertIn("# Benchmark Accuracy Report", text)
            self.assertIn("## Tier-2 Accuracy", text)
            self.assertIn("## Per-Benchmark Metrics (Tier 2)", text)
            self.assertIn("## Calibration Set (Tier 1)", text)
            self.assertIn("kernel_a_comparison.png", text)
            self.assertIn("kernel_b_comparison.png", text)
            self.assertNotIn("kernel_c", text)

            # Spot-check kernel_a metrics: err = [1.0, 0.25] -> mean 0.625
            metrics_a = dict(result["per_kernel"][0][3])
            self.assertAlmostEqual(metrics_a["mean_err"], 0.625)
            self.assertAlmostEqual(metrics_a["max_err"], 1.0)
            # sym: pair1 abs(1)/min(1,2)=1.0; pair2 abs(1)/min(4,5)=0.25
            self.assertAlmostEqual(metrics_a["max_sym_err"], 1.0)
            self.assertAlmostEqual(metrics_a["mean_sym_err"], 0.625)

            # Global metrics aggregate all 3 matched samples.
            global_metrics = result["global"]
            self.assertFalse(math.isnan(global_metrics["mean_err"]))
            self.assertGreater(global_metrics["max_err"], 0.0)

    def test_affine_time_model_calibrates_metrics_without_hiding_rows(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            scripts_dir = tmp_path / "scripts"
            scripts_dir.mkdir()
            (scripts_dir / "benchmark_tiers.json").write_text(
                json.dumps({
                    "per_benchmark_time_models": {
                        "adjust_weights": {
                            "sim_scale": 2.0,
                            "fixed_time_ms": 1.0,
                            "source": "unit-test affine fit",
                        }
                    }
                }),
                encoding="utf-8",
            )

            csv_path = tmp_path / "comparison_ci.csv"
            self._write_csv(
                csv_path,
                [
                    ["adjust_weights", "1", 5.0, 2.0, 3.0, -0.6],
                    ["adjust_weights", "2", 7.0, 3.0, 4.0, -0.57],
                    ["adjust_weights", "3", 9.0, "", "", ""],
                ],
            )

            result = gcr.generate_report(
                csv_path=str(csv_path),
                figures_dir=str(tmp_path / "figures"),
                report_path=str(tmp_path / "accuracy_report.md"),
                figures_rel_dir="figures",
                repo_root=tmp_path,
                env={},
            )

            self.assertEqual(len(result["per_kernel"]), 1)
            label, samples, _, metrics, _ = result["per_kernel"][0]
            self.assertEqual(label, "adjust_weights")
            self.assertEqual(samples, 2)
            self.assertAlmostEqual(metrics["mean_err"], 0.0)
            self.assertAlmostEqual(metrics["max_sym_err"], 0.0)

            text = (tmp_path / "accuracy_report.md").read_text(encoding="utf-8")
            self.assertIn("## Report-Time Simulator Calibration", text)
            self.assertIn("raw `sim_ms` values", text)
            self.assertIn("| adjust_weights | 2 | 0.00%", text)
            self.assertIn("unit-test affine fit", text)

    def test_region_time_model_calibrates_by_problem_size(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            scripts_dir = tmp_path / "scripts"
            scripts_dir.mkdir()
            (scripts_dir / "benchmark_tiers.json").write_text(
                json.dumps({
                    "per_benchmark_time_models": {
                        "mem_latency_chase": {
                            "regions": [
                                {
                                    "label": "small",
                                    "max_problem_size": 10,
                                    "sim_scale": 2.0,
                                    "fixed_time_ms": 1.0,
                                    "source": "small selected-run fit",
                                },
                                {
                                    "label": "large",
                                    "min_problem_size": 11,
                                    "sim_scale": 3.0,
                                    "fixed_time_ms": -1.0,
                                    "source": "large selected-run fit",
                                },
                            ]
                        }
                    }
                }),
                encoding="utf-8",
            )

            csv_path = tmp_path / "comparison_ci.csv"
            self._write_csv(
                csv_path,
                [
                    ["mem_latency_chase", "8", 5.0, 2.0, 3.0, -0.6],
                    ["mem_latency_chase", "16", 5.0, 2.0, 3.0, -0.6],
                    ["mem_latency_chase", "32", 9.0, "", "", ""],
                ],
            )

            result = gcr.generate_report(
                csv_path=str(csv_path),
                figures_dir=str(tmp_path / "figures"),
                report_path=str(tmp_path / "accuracy_report.md"),
                figures_rel_dir="figures",
                repo_root=tmp_path,
                env={},
            )

            self.assertEqual(len(result["per_kernel"]), 1)
            label, samples, _, metrics, _ = result["per_kernel"][0]
            self.assertEqual(label, "mem_latency_chase")
            self.assertEqual(samples, 2)
            self.assertAlmostEqual(metrics["mean_err"], 0.0)
            self.assertAlmostEqual(metrics["max_sym_err"], 0.0)

            text = (tmp_path / "accuracy_report.md").read_text(encoding="utf-8")
            self.assertIn("raw `sim_ms` values", text)
            self.assertIn("| mem_latency_chase | small (≤10) | 2", text)
            self.assertIn("| mem_latency_chase | large (≥11) | 3", text)
            self.assertIn("small selected-run fit", text)
            self.assertIn("large selected-run fit", text)

    def test_cache_bandwidth_flat_work_sweep_is_diagnostic_pending_value_change(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            self._write_cache_bw_tiers(tmp_path)
            csv_path = tmp_path / "comparison_ci.csv"
            self._write_csv(
                csv_path,
                [
                    ["l1_cache_bw", "1024", 0.120, 0.010, 0.110, -0.92],
                    ["l1_cache_bw", "2048", 0.121, 0.011, 0.110, -0.91],
                    ["l1_cache_bw", "4096", 0.122, 0.012, 0.110, -0.90],
                ],
            )

            result = gcr.generate_report(
                csv_path=str(csv_path),
                figures_dir=str(tmp_path / "figures"),
                report_path=str(tmp_path / "accuracy_report.md"),
                figures_rel_dir="figures",
                repo_root=tmp_path,
                env={},
            )

            semantic = result["cache_bandwidth_semantics"]["l1_cache_bw"]
            self.assertEqual(
                semantic["metric_policy"],
                "diagnostic_until_value_changing_rows",
            )
            self.assertIs(semantic["work_amount_scaled_by_axis"], True)
            self.assertIn(
                "loaded_hardware_value_changing",
                semantic["value_change_gate"]["reason"],
            )
            text = (tmp_path / "accuracy_report.md").read_text(encoding="utf-8")
            self.assertIn("## Cache Bandwidth Report Semantics", text)
            self.assertIn("pending value-change proof", text)
            self.assertIn(
                "total_read_bytes = num_elements * num_iterations * bytes_per_read",
                text,
            )
            self.assertIn("HIP/CUDA default 262144000 B", text)
            self.assertIn("Go simulator `num_repeats`", text)
            self.assertIn("HIP/CUDA fixed working_set_size 8192 B", text)
            self.assertIn("Go simulator 256-thread read groups (1024 B each)", text)
            self.assertIn(
                "input bytes = ceil(num_elements / 256) * 256 * bytes_per_read",
                text,
            )

    def test_cache_bandwidth_nonflat_work_sweep_is_scored_value_changing(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            self._write_cache_bw_tiers(tmp_path)
            csv_path = tmp_path / "comparison_ci.csv"
            self._write_csv(
                csv_path,
                [
                    ["l1_cache_bw", "1024", 1.0, 0.9, 0.1, -0.1],
                    ["l1_cache_bw", "2048", 1.3, 1.1, 0.2, -0.15],
                    ["l1_cache_bw", "4096", 2.5, 2.2, 0.3, -0.12],
                    ["l1_cache_bw", "8192", 4.2, 4.0, 0.2, -0.05],
                ],
            )

            result = gcr.generate_report(
                csv_path=str(csv_path),
                figures_dir=str(tmp_path / "figures"),
                report_path=str(tmp_path / "accuracy_report.md"),
                figures_rel_dir="figures",
                repo_root=tmp_path,
                env={},
            )

            semantic = result["cache_bandwidth_semantics"]["l1_cache_bw"]
            self.assertEqual(
                semantic["metric_policy"],
                "scored_value_changing_work_sweep",
            )
            self.assertTrue(semantic["value_change_gate"]["passed"])
            self.assertEqual(
                semantic["repeat_count_parameter_names"]["go_simulator"],
                "num_repeats",
            )
            self.assertEqual(
                semantic["implementation_semantics"]["go_simulator"]["input_bytes_formula"],
                "ceil(num_elements / 256) * 256 * bytes_per_read",
            )
            text = (tmp_path / "accuracy_report.md").read_text(encoding="utf-8")
            self.assertIn("scored; loaded rows prove value-changing work sweep", text)

    def test_cache_bandwidth_archived_selected_run_remains_diagnostic_only(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            self._write_cache_bw_tiers(tmp_path, run_id="123")
            csv_path = (
                tmp_path
                / "benchmark-comparison"
                / "selected-run-123"
                / "comparison_ci.detailed.csv"
            )
            csv_path.parent.mkdir(parents=True)
            self._write_csv(
                csv_path,
                [
                    ["l1_cache_bw", "1024", 0.120, 0.010, 0.110, -0.92],
                    ["l1_cache_bw", "2048", 0.121, 0.011, 0.110, -0.91],
                    ["l1_cache_bw", "4096", 0.122, 0.012, 0.110, -0.90],
                ],
            )

            result = gcr.generate_report(
                csv_path=str(csv_path),
                figures_dir=str(tmp_path / "figures"),
                report_path=str(tmp_path / "accuracy_report.md"),
                figures_rel_dir="figures",
                repo_root=tmp_path,
                env={},
            )

            semantic = result["cache_bandwidth_semantics"]["l1_cache_bw"]
            self.assertEqual(
                semantic["metric_policy"],
                "diagnostic_only_archived_fixed_work",
            )
            self.assertIs(semantic["work_amount_scaled_by_axis"], False)
            self.assertTrue(semantic["archived_metadata_gate"]["passed"])
            text = (tmp_path / "accuracy_report.md").read_text(encoding="utf-8")
            self.assertIn("archived fixed-work cache-residency sweep", text)
            self.assertIn("diagnostic only under archived selected-run provenance gate", text)

    def test_generate_report_handles_no_matched_rows(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            csv_path = tmp_path / "comparison_ci.csv"
            figures_dir = tmp_path / "figures"
            report_path = tmp_path / "accuracy_report.md"

            self._write_csv(
                csv_path,
                [
                    ["kernel_a", "128", 1.0, "", "", ""],
                    ["kernel_b", "256", "", 1.0, "", ""],
                ],
            )

            result = gcr.generate_report(
                csv_path=str(csv_path),
                figures_dir=str(figures_dir),
                report_path=str(report_path),
                figures_rel_dir="figures",
                repo_root=tmp_path,
            )

            self.assertEqual(result["per_kernel"], [])
            self.assertEqual(result["charts"], [])
            self.assertTrue(report_path.is_file())
            text = report_path.read_text(encoding="utf-8")
            self.assertIn("Matched tier-2 samples: **0**", text)

    def test_params_json_drives_numeric_xaxis(self):
        """When params.json is present, the scaling parameter is used as x-axis."""
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            # Create a minimal params.json for "mykernel"
            # Mapping: mykernel → polybench/mykernel (we add it via monkeypatching
            # but for simplicity just create the directory tree the code expects)
            params_dir = tmp_path / "workloads" / "polybench" / "2DConvolution"
            params_dir.mkdir(parents=True)
            params_data = {
                "benchmark": "2DConvolution",
                "suite": "polybench",
                "scaling_parameter": {"name": "size", "values": [64, 128, 256]},
                "non_scaling_parameters": {},
            }
            (params_dir / "params.json").write_text(
                json.dumps(params_data), encoding="utf-8"
            )

            csv_path = tmp_path / "comparison_ci.csv"
            self._write_csv(
                csv_path,
                [
                    ["2dconvolution", "64x64", 1.0, 1.5, 0.5, 0.5],
                    ["2dconvolution", "128x128", 2.0, 2.5, 0.5, 0.25],
                    ["2dconvolution", "256x256", 4.0, 5.0, 1.0, 0.25],
                ],
            )

            result = gcr.generate_report(
                csv_path=str(csv_path),
                figures_dir=str(tmp_path / "figures"),
                report_path=str(tmp_path / "report.md"),
                figures_rel_dir="figures",
                repo_root=tmp_path,
            )

            self.assertEqual(len(result["per_kernel"]), 1)
            self.assertEqual(result["per_kernel"][0][0], "2dconvolution")
            self.assertIn("2dconvolution_comparison.png", result["charts"])

    def test_single_nonscaling_combo_one_chart(self):
        """All rows share the same non-scaling values → exactly one chart."""
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            params_dir = tmp_path / "workloads" / "ml_kernels" / "gemm"
            params_dir.mkdir(parents=True)
            params_data = {
                "benchmark": "gemm",
                "suite": "ml_kernels",
                "scaling_parameter": {"name": "M", "values": [128, 256, 512]},
                "non_scaling_parameters": {
                    "N": {"values": [768]},
                    "K": {"values": [768]},
                    "tile": {"values": [16]},
                },
            }
            (params_dir / "params.json").write_text(
                json.dumps(params_data), encoding="utf-8"
            )

            csv_path = tmp_path / "comparison_ci.csv"
            self._write_csv(
                csv_path,
                [
                    ["tiled_gemm_16", "M128_N768_K768_tile16", 1.0, 1.5, 0.5, 0.5],
                    ["tiled_gemm_16", "M256_N768_K768_tile16", 2.0, 2.5, 0.5, 0.25],
                    ["tiled_gemm_16", "M512_N768_K768_tile16", 4.0, 5.0, 1.0, 0.25],
                ],
            )

            result = gcr.generate_report(
                csv_path=str(csv_path),
                figures_dir=str(tmp_path / "figures"),
                report_path=str(tmp_path / "report.md"),
                figures_rel_dir="figures",
                repo_root=tmp_path,
            )

            self.assertEqual(len(result["charts"]), 1)
            self.assertEqual(len(result["per_kernel"]), 1)
            chart = result["charts"][0]
            self.assertIn("N768", chart)
            self.assertIn("K768", chart)
            self.assertIn("tile16", chart)

    def test_multiple_nonscaling_combos_split_charts(self):
        """Different non-scaling-value combos produce one chart per combo."""
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            params_dir = tmp_path / "workloads" / "ml_kernels" / "gemm"
            params_dir.mkdir(parents=True)
            params_data = {
                "benchmark": "gemm",
                "suite": "ml_kernels",
                "scaling_parameter": {"name": "M", "values": [128, 256]},
                "non_scaling_parameters": {
                    "N": {"values": [768, 4096]},
                    "K": {"values": [768, 4096]},
                    "tile": {"values": [16, 32]},
                },
            }
            (params_dir / "params.json").write_text(
                json.dumps(params_data), encoding="utf-8"
            )

            csv_path = tmp_path / "comparison_ci.csv"
            self._write_csv(
                csv_path,
                [
                    ["tiled_gemm_16", "M128_N768_K768_tile16", 1.0, 1.5, 0.5, 0.5],
                    ["tiled_gemm_16", "M256_N768_K768_tile16", 2.0, 2.5, 0.5, 0.25],
                    ["tiled_gemm_16", "M128_N4096_K4096_tile32", 3.0, 3.6, 0.6, 0.2],
                    ["tiled_gemm_16", "M256_N4096_K4096_tile32", 6.0, 7.0, 1.0, 0.17],
                ],
            )

            result = gcr.generate_report(
                csv_path=str(csv_path),
                figures_dir=str(tmp_path / "figures"),
                report_path=str(tmp_path / "report.md"),
                figures_rel_dir="figures",
                repo_root=tmp_path,
            )

            # Two distinct (N, K, tile) combos → two charts.
            self.assertEqual(len(result["charts"]), 2)
            self.assertEqual(len(result["per_kernel"]), 2)
            charts = set(result["charts"])
            self.assertTrue(any("N768" in c and "tile16" in c for c in charts))
            self.assertTrue(any("N4096" in c and "tile32" in c for c in charts))


class ParseNonscalingValuesTest(unittest.TestCase):
    """Tests for parse_nonscaling_values()."""

    def test_multi_param_problem_size(self):
        nsp = {"N": {}, "K": {}, "tile": {}}
        result = gcr.parse_nonscaling_values("M10240_N768_K768_tile16", "M", nsp)
        self.assertEqual(result, {"N": "768", "K": "768", "tile": "16"})

    def test_excludes_scaling_param(self):
        nsp = {"N": {}, "K": {}}
        result = gcr.parse_nonscaling_values("M128_N768_K768", "M", nsp)
        self.assertNotIn("M", result)
        self.assertEqual(result, {"N": "768", "K": "768"})

    def test_unknown_keys_ignored(self):
        nsp = {"N": {}}
        result = gcr.parse_nonscaling_values("M128_N768_K768_tile16", "M", nsp)
        self.assertEqual(result, {"N": "768"})

    def test_empty_nonscaling_returns_empty(self):
        self.assertEqual(
            gcr.parse_nonscaling_values("M128_N768", "M", {}),
            {},
        )

    def test_unparseable_tokens_skipped(self):
        nsp = {"K": {}}
        # "64x64" doesn't match the <letters><digits> token pattern.
        result = gcr.parse_nonscaling_values("64x64", "size", nsp)
        self.assertEqual(result, {})

    def test_empty_problem_size_returns_empty(self):
        self.assertEqual(
            gcr.parse_nonscaling_values("", "M", {"N": {}}),
            {},
        )


class SectionPreservationTest(unittest.TestCase):
    """Only allowed hand-maintained sections survive report regeneration."""

    _CSV_HEADER = ["kernel_name", "problem_size", "real_ms", "sim_ms",
                   "abs_error_ms", "rel_error"]

    _MINIMAL_REPORT = (
        "# Benchmark Accuracy Report\n"
        "\n"
        "Comparison of MGPUSim simulated execution time against MI300A "
        "hardware measurements.\n"
        "\n"
        "Source data: stub.\n"
        "\n"
        "Error definitions:\n"
        "\n"
        "- `err = |sim - hw| / hw`\n"
        "\n"
        "## Tier-2 Per-Size Finishability\n"
        "\n"
        "Source: Run 24959959195 non-terminal stale content. SENTINEL_TIER2.\n"
        "\n"
        "| Benchmark | Sizes within 7200s |\n"
        "|-----------|-----------|\n"
        "| stub | 1 |\n"
        "\n"
        "## Tier-2 Accuracy (Global Summary)\n"
        "\n"
        "stub.\n"
        "\n"
        "## Calibration Set (Tier 1)\n"
        "\n"
        "stub.\n"
        "\n"
        "## Tier-1 Per-Size Finishability\n"
        "\n"
        "Source: Run 24959959195 non-terminal stale content. SENTINEL_TIER1.\n"
        "\n"
        "## Per-Benchmark Metrics (Tier 2)\n"
        "\n"
        "stub.\n"
        "\n"
        "## Known Limitations\n"
        "\n"
        "Hand-maintained limitations text. SENTINEL_LIMITATIONS.\n"
    )

    def _write_csv(self, path: Path, rows):
        with path.open("w", newline="") as fh:
            writer = csv.writer(fh)
            writer.writerow(self._CSV_HEADER)
            for row in rows:
                writer.writerow(row)

    def _generate(self, tmp_path: Path) -> str:
        csv_path = tmp_path / "comparison_ci.csv"
        self._write_csv(
            csv_path,
            [
                ["kernel_a", "128", 1.0, 2.0, 1.0, 1.0],
                ["kernel_a", "256", 4.0, 5.0, 1.0, 0.25],
            ],
        )
        report_path = tmp_path / "accuracy_report.md"
        report_path.write_text(self._MINIMAL_REPORT, encoding="utf-8")

        gcr.generate_report(
            csv_path=str(csv_path),
            figures_dir=str(tmp_path / "figures"),
            report_path=str(report_path),
            figures_rel_dir="figures",
            repo_root=tmp_path,
            env={},
        )
        return report_path.read_text(encoding="utf-8")

    def test_extract_preserved_section_returns_none_when_missing(self):
        self.assertIsNone(
            gcr.extract_preserved_section("no headings here", "## Foo")
        )
        self.assertIsNone(gcr.extract_preserved_section("", "## Foo"))

    def test_extract_preserved_section_stops_at_next_heading(self):
        text = (
            "## Tier-2 Per-Size Finishability\n"
            "\n"
            "body line\n"
            "\n"
            "## Next Section\n"
            "\n"
            "should not appear\n"
        )
        section = gcr.extract_preserved_section(
            text, "## Tier-2 Per-Size Finishability"
        )
        self.assertIsNotNone(section)
        self.assertIn("body line", section)
        self.assertNotIn("Next Section", section)
        self.assertNotIn("should not appear", section)

    def test_regeneration_preserves_known_limitations_only(self):
        with tempfile.TemporaryDirectory() as tmp:
            text = self._generate(Path(tmp))
            self.assertIn("## Known Limitations", text)
            self.assertIn("SENTINEL_LIMITATIONS", text)
            self.assertIn("## Per-Size Finishability", text)
            self.assertIn("tables are omitted", text)
            self.assertNotIn("## Tier-2 Per-Size Finishability", text)
            self.assertNotIn("SENTINEL_TIER2", text)
            self.assertNotIn("## Tier-1 Per-Size Finishability", text)
            self.assertNotIn("SENTINEL_TIER1", text)
            self.assertNotIn("24959959195", text)

    def test_repeated_regeneration_is_idempotent(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            first = self._generate(tmp_path)
            # Run a second time without re-writing the seed report.
            csv_path = tmp_path / "comparison_ci.csv"
            report_path = tmp_path / "accuracy_report.md"
            gcr.generate_report(
                csv_path=str(csv_path),
                figures_dir=str(tmp_path / "figures"),
                report_path=str(report_path),
                figures_rel_dir="figures",
                repo_root=tmp_path,
                env={},
            )
            second = report_path.read_text(encoding="utf-8")
            self.assertEqual(first, second)
            # Known limitations remain, but stale finishability does not reappear.
            self.assertIn("## Per-Size Finishability", second)
            self.assertNotIn("SENTINEL_TIER2", second)
            self.assertNotIn("SENTINEL_TIER1", second)
            self.assertIn("SENTINEL_LIMITATIONS", second)

    def test_stale_adjust_weights_known_limitations_are_normalized(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            csv_path = tmp_path / "comparison_ci.csv"
            rows = []
            for size in range(1, 12):
                err = 0.6229 if size == 11 else 0.22668
                rows.append([
                    "adjust_weights", str(size), 100.0, 100.0 * (1.0 + err),
                    100.0 * err, err,
                ])
            self._write_csv(csv_path, rows)

            report_path = tmp_path / "accuracy_report.md"
            report_path.write_text(
                "## Known Limitations\n"
                "\n"
                "### adjust_weights (backprop benchmark) — calibrated\n"
                "\n"
                "With `fixed_time_ms=0.005` calibration, `adjust_weights` "
                "now achieves **13.51% mean error**, which meets the <20% "
                "target from issue #552.\n"
                "\n"
                "**Mean error: 13.51% (5 samples). Max error: 26.00%.**\n"
                "\n"
                "### Other limitation\n"
                "\n"
                "OTHER_LIMITATION_SENTINEL.\n",
                encoding="utf-8",
            )

            gcr.generate_report(
                csv_path=str(csv_path),
                figures_dir=str(tmp_path / "figures"),
                report_path=str(report_path),
                figures_rel_dir="figures",
                repo_root=tmp_path,
                env={},
            )

            text = report_path.read_text(encoding="utf-8")
            self.assertIn("| adjust_weights | 11 | 26.27% | 62.29%", text)
            self.assertIn("## Known Limitations", text)
            self.assertIn("OTHER_LIMITATION_SENTINEL", text)
            self.assertIn("Use the generated per-benchmark metrics table", text)
            self.assertNotIn("13.51%", text)
            self.assertNotIn("5 samples", text)
            self.assertNotIn("meets the <20%", text)
            self.assertNotIn("26.00%", text)


class ReportProvenanceAndFinishabilityTest(unittest.TestCase):
    RUN_ID = "25619929396"
    HEAD_SHA = "a281789a7354b86a8a04b350c145c573d531f54d"
    SELECTED_METADATA = (
        REPO_ROOT / "benchmark-comparison" / "selected-run-25619929396" / "run-view.json"
    )
    SELECTED_ARTIFACT = (
        REPO_ROOT
        / "benchmark-comparison"
        / "selected-run-25619929396"
        / "comparison_ci.detailed.csv"
    )
    SELECTED_COMPARISON = REPO_ROOT / "benchmark-comparison" / "selected-run-25619929396" / "comparison_ci.detailed.csv"

    def _write_csv(self, path: Path, rows):
        with path.open("w", newline="") as fh:
            writer = csv.writer(fh)
            writer.writerow([
                "kernel_name", "problem_size", "real_ms", "sim_ms",
                "abs_error_ms", "rel_error",
            ])
            for row in rows:
                writer.writerow(row)

    def _write_minimal_inputs(self, tmp_path: Path) -> Path:
        csv_path = tmp_path / "comparison_ci.csv"
        self._write_csv(
            csv_path,
            [
                ["tier1_kernel", "1", 1.0, 2.0, 1.0, 1.0],
                ["tier1_kernel", "2", 1.0, "", "", ""],
                ["tier2_kernel", "1", 1.0, "", "", ""],
                ["tier2_kernel", "2", 1.0, 3.0, 2.0, 2.0],
            ],
        )
        scripts_dir = tmp_path / "scripts"
        scripts_dir.mkdir()
        (scripts_dir / "benchmark_tiers.json").write_text(
            json.dumps({"tier1": ["tier1_kernel"]}),
            encoding="utf-8",
        )
        return csv_path

    def _selected_provenance(
        self,
        metadata_path: Path | None = None,
    ) -> gcr.ReportProvenance:
        metadata_path = self.SELECTED_METADATA if metadata_path is None else metadata_path
        return gcr.ReportProvenance(
            run_id=self.RUN_ID,
            head_sha=self.HEAD_SHA,
            head_branch="main",
            workflow_name="MI300A Benchmark",
            status="completed",
            conclusion="failure",
            url="https://github.com/sarchlab/mgpusim-dev/actions/runs/25619929396",
            metadata_path=str(metadata_path),
        )

    def _build_selected_finishability(
        self,
        evidence_path: Path,
        metadata_path: Path | None = None,
    ):
        tier1_set, _ = gcr.load_tier_config(REPO_ROOT)
        return gcr.build_finishability_summary(
            str(evidence_path),
            tier1_set,
            self._selected_provenance(metadata_path),
            repo_root=REPO_ROOT,
        )

    def _read_selected_evidence_rows(self):
        with self.SELECTED_ARTIFACT.open(newline="", encoding="utf-8") as fh:
            reader = csv.DictReader(fh)
            fieldnames = list(reader.fieldnames or [])
            rows = list(reader)
        return fieldnames, rows

    def _write_evidence_rows(self, path: Path, rows, fieldnames=None) -> None:
        if fieldnames is None:
            fieldnames, _ = self._read_selected_evidence_rows()
        with path.open("w", newline="", encoding="utf-8") as fh:
            writer = csv.DictWriter(fh, fieldnames=fieldnames)
            writer.writeheader()
            writer.writerows(rows)

    def _evidence_rows_to_bytes(self, rows, fieldnames) -> bytes:
        buf = io.StringIO()
        writer = csv.DictWriter(buf, fieldnames=fieldnames)
        writer.writeheader()
        writer.writerows(rows)
        return buf.getvalue().encode("utf-8")

    def _write_metadata(self, path: Path, **updates) -> Path:
        data = json.loads(self.SELECTED_METADATA.read_text(encoding="utf-8"))
        data.update(updates)
        path.write_text(json.dumps(data, sort_keys=True), encoding="utf-8")
        return path

    def test_generate_report_renders_selected_run_provenance(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            csv_path = self._write_minimal_inputs(tmp_path)
            report_path = tmp_path / "accuracy_report.md"
            report_path.write_text(
                "# stale\n\n"
                "## Tier-2 Per-Size Finishability\n\n"
                "Source: Run 24959959195 should not survive.\n\n"
                "## Known Limitations\n\nSENTINEL_LIMITATIONS\n",
                encoding="utf-8",
            )

            gcr.generate_report(
                csv_path=str(csv_path),
                figures_dir=str(tmp_path / "figures"),
                report_path=str(report_path),
                figures_rel_dir="figures",
                repo_root=tmp_path,
                selected_run_id=self.RUN_ID,
                selected_head_sha=self.HEAD_SHA,
                selected_run_status="completed",
                selected_run_conclusion="failure",
                env={},
            )

            text = report_path.read_text(encoding="utf-8")
            self.assertIn("## Selected CI Run Provenance", text)
            self.assertIn(f"- Run ID: `{self.RUN_ID}`", text)
            self.assertIn(f"- Head SHA: `{self.HEAD_SHA}`", text)
            self.assertIn("- Run status/conclusion: `completed` / `failure`", text)
            self.assertIn("SENTINEL_LIMITATIONS", text)
            self.assertIn("tables are omitted", text)
            self.assertNotIn("24959959195", text)
            self.assertNotIn("Source: Run", text)

    def test_finishability_sections_generated_from_selected_run_artifact(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            csv_path = self._write_minimal_inputs(tmp_path)
            report_path = tmp_path / "accuracy_report.md"

            result = gcr.generate_report(
                csv_path=str(csv_path),
                figures_dir=str(tmp_path / "figures"),
                report_path=str(report_path),
                figures_rel_dir="figures",
                repo_root=REPO_ROOT,
                run_metadata_path=str(self.SELECTED_METADATA),
                finishability_evidence_csv_path=str(self.SELECTED_COMPARISON),
                finishability_source_label=(
                    "`benchmark-comparison/selected-run-25619929396/"
                    "comparison_ci.detailed.csv` extracted from selected "
                    "run 25619929396 comparison-detailed artifact"
                ),
                env={},
            )

            text = report_path.read_text(encoding="utf-8")
            self.assertIsNotNone(result["finishability"])
            self.assertIn("## Tier-2 Per-Size Finishability", text)
            self.assertIn("## Tier-1 Per-Size Finishability", text)
            self.assertNotIn("tables are omitted", text)
            self.assertIn(f"Source: selected terminal run {self.RUN_ID}", text)
            self.assertIn(f"head {self.HEAD_SHA}", text)
            self.assertIn("`benchmark-comparison/selected-run-25619929396/", text)
            self.assertIn("7200s per-simulation invocation contract", text)
            self.assertIn("Sizes within 7200s", text)
            self.assertNotIn("1" + "hr", text)
            self.assertNotIn("1 " + "hour", text.lower())
            self.assertIn("| hist | 16 | 18 |", text)
            self.assertIn("| mem_latency_chase | 16 | 16 |", text)
            self.assertIn(
                "| naive_attention | 9 | 17 | "
                "B4_S32_H12_D64_blk256–B4_S640_H12_D64_blk256 | 8 |",
                text,
            )
            self.assertIn(
                "| rope | 6 | 16 | "
                "B4_S64_H32_D128_blk256–B4_S768_H32_D128_blk256 | 10 |",
                text,
            )

    def test_finishability_rejects_selected_identity_without_metadata(self):
        tier1_set, _ = gcr.load_tier_config(REPO_ROOT)
        provenance = gcr.ReportProvenance(
            run_id=self.RUN_ID,
            head_sha=self.HEAD_SHA,
            status="completed",
            conclusion="failure",
        )
        with self.assertRaisesRegex(ValueError, "run ID/head/status arguments alone"):
            gcr.build_finishability_summary(
                str(self.SELECTED_COMPARISON),
                tier1_set,
                provenance,
                repo_root=REPO_ROOT,
            )

    def test_finishability_rejects_fake_one_row_csv(self):
        with tempfile.TemporaryDirectory() as tmp:
            fake_csv = Path(tmp) / "fake.csv"
            self._write_csv(
                fake_csv,
                [["2dconvolution", "1024x1024", 0.0076, "", "", ""]],
            )
            with self.assertRaisesRegex(
                ValueError,
                "CSV has 1 data rows.*missing regular-plan rows",
            ):
                self._build_selected_finishability(fake_csv)

    def test_finishability_rejects_mismatched_run_metadata(self):
        with tempfile.TemporaryDirectory() as tmp:
            metadata_path = self._write_metadata(
                Path(tmp) / "run-view.json",
                databaseId=999,
            )
            with self.assertRaisesRegex(ValueError, "metadata run 999.*25619929396"):
                self._build_selected_finishability(
                    self.SELECTED_COMPARISON,
                    metadata_path=metadata_path,
                )

    def test_finishability_rejects_mismatched_head_metadata(self):
        with tempfile.TemporaryDirectory() as tmp:
            metadata_path = self._write_metadata(
                Path(tmp) / "run-view.json",
                headSha="deadbeef",
            )
            with self.assertRaisesRegex(ValueError, "metadata head SHA deadbeef"):
                self._build_selected_finishability(
                    self.SELECTED_COMPARISON,
                    metadata_path=metadata_path,
                )

    def test_finishability_rejects_unknown_kernels(self):
        with tempfile.TemporaryDirectory() as tmp:
            fieldnames, rows = self._read_selected_evidence_rows()
            rows.append({
                "kernel_name": "fake_kernel",
                "problem_size": "1",
                "real_ms": "1.0",
                "sim_ms": "1.0",
                "abs_error_ms": "0.0",
                "rel_error": "0.0",
            })
            evidence_path = Path(tmp) / "unknown.csv"
            self._write_evidence_rows(evidence_path, rows, fieldnames)

            with self.assertRaisesRegex(ValueError, "unknown kernels.*fake_kernel"):
                self._build_selected_finishability(evidence_path)

    def test_finishability_rejects_missing_regular_plan_rows(self):
        with tempfile.TemporaryDirectory() as tmp:
            fieldnames, rows = self._read_selected_evidence_rows()
            evidence_path = Path(tmp) / "missing.csv"
            self._write_evidence_rows(evidence_path, rows[:-1], fieldnames)

            with self.assertRaisesRegex(
                ValueError,
                "CSV has 1415 data rows.*missing regular-plan rows",
            ):
                self._build_selected_finishability(evidence_path)

    def test_finishability_rejects_extra_regular_plan_rows(self):
        with tempfile.TemporaryDirectory() as tmp:
            fieldnames, rows = self._read_selected_evidence_rows()
            rows.append({
                "kernel_name": "2dconvolution",
                "problem_size": "not_in_plan",
                "real_ms": "1.0",
                "sim_ms": "1.0",
                "abs_error_ms": "0.0",
                "rel_error": "0.0",
            })
            evidence_path = Path(tmp) / "extra.csv"
            self._write_evidence_rows(evidence_path, rows, fieldnames)

            with self.assertRaisesRegex(
                ValueError,
                "extra regular-plan rows.*2dconvolution/not_in_plan",
            ):
                self._build_selected_finishability(evidence_path)

    def test_finishability_rejects_evidence_not_derived_from_selected_artifact(self):
        with tempfile.TemporaryDirectory() as tmp:
            fieldnames, rows = self._read_selected_evidence_rows()
            rows[0] = dict(rows[0])
            rows[0]["sim_ms"] = "123.456"
            evidence_path = Path(tmp) / "not-selected-artifact.csv"
            self._write_evidence_rows(evidence_path, rows, fieldnames)

            with self.assertRaisesRegex(
                ValueError,
                "not derived from selected comparison-detailed artifact",
            ):
                self._build_selected_finishability(evidence_path)

    def test_finishability_rejects_fifo_toctou_before_later_selected_hash(self):
        if not hasattr(os, "mkfifo"):
            self.skipTest("FIFO paths are not supported on this platform")

        with tempfile.TemporaryDirectory() as tmp:
            fieldnames, rows = self._read_selected_evidence_rows()
            malicious_rows = [dict(row) for row in rows]
            for row in malicious_rows:
                if not str(row.get("sim_ms") or "").strip():
                    row["sim_ms"] = "1.0"
            malicious_bytes = self._evidence_rows_to_bytes(
                malicious_rows,
                fieldnames,
            )
            real_bytes = self.SELECTED_ARTIFACT.read_bytes()
            fifo = Path(tmp) / "finishability-evidence.csv"
            try:
                os.mkfifo(fifo)
            except OSError as exc:  # pragma: no cover - platform/filesystem guard
                self.skipTest(f"FIFO paths are not supported here: {exc}")

            first_done = threading.Event()
            stop_second = threading.Event()
            writer_errors = []

            def write_first_bytes() -> None:
                try:
                    with open(fifo, "wb") as fh:
                        fh.write(malicious_bytes)
                except OSError as exc:
                    writer_errors.append(f"first writer failed: {exc}")
                finally:
                    first_done.set()

            def write_later_selected_bytes() -> None:
                if not first_done.wait(5):
                    writer_errors.append("first writer did not finish")
                    return
                time.sleep(0.1)
                deadline = time.monotonic() + 5
                while not stop_second.is_set() and time.monotonic() < deadline:
                    try:
                        fd = os.open(fifo, os.O_WRONLY | os.O_NONBLOCK)
                    except OSError as exc:
                        if exc.errno == errno.ENXIO:
                            time.sleep(0.01)
                            continue
                        writer_errors.append(f"second writer open failed: {exc}")
                        return
                    try:
                        with os.fdopen(fd, "wb") as fh:
                            fh.write(real_bytes)
                        return
                    except BrokenPipeError:
                        return
                    except OSError as exc:
                        writer_errors.append(f"second writer failed: {exc}")
                        return
                if not stop_second.is_set():
                    writer_errors.append("second writer did not observe a later reader")

            first_thread = threading.Thread(target=write_first_bytes, daemon=True)
            second_thread = threading.Thread(
                target=write_later_selected_bytes,
                daemon=True,
            )
            first_thread.start()
            second_thread.start()

            try:
                try:
                    summary = self._build_selected_finishability(fifo)
                except ValueError as exc:
                    self.assertIn(
                        "not derived from selected comparison-detailed artifact",
                        str(exc),
                    )
                else:
                    rendered = "\n".join(
                        section
                        for section in (summary.tier1_section, summary.tier2_section)
                        if section
                    )
                    self.assertIn(
                        "| hist | 16 | 18 | 512–16777216 | 2 |",
                        rendered,
                    )
                    self.assertNotIn(
                        "| hist | 18 | 18 | 512–67108864 | 0 |",
                        rendered,
                    )
            finally:
                stop_second.set()
                first_thread.join(timeout=5)
                second_thread.join(timeout=5)

            self.assertFalse(first_thread.is_alive(), "first FIFO writer hung")
            self.assertFalse(second_thread.is_alive(), "second FIFO writer hung")
            self.assertEqual(writer_errors, [])

    def test_non_terminal_finishability_evidence_is_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            csv_path = self._write_minimal_inputs(tmp_path)
            metadata_path = tmp_path / "run-view.json"
            metadata_path.write_text(
                json.dumps({
                    "databaseId": 24959959195,
                    "headSha": "d1105930b0272b8f8f552abfc2490744ef6fcf2f",
                    "status": "queued",
                    "conclusion": "",
                    "non_terminal_snapshot": True,
                }),
                encoding="utf-8",
            )

            with self.assertRaisesRegex(ValueError, "24959959195.*not terminal"):
                gcr.generate_report(
                    csv_path=str(csv_path),
                    figures_dir=str(tmp_path / "figures"),
                    report_path=str(tmp_path / "accuracy_report.md"),
                    figures_rel_dir="figures",
                    repo_root=tmp_path,
                    run_metadata_path=str(metadata_path),
                    finishability_evidence_csv_path=str(csv_path),
                    env={},
                )

    def test_require_finishability_without_evidence_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            csv_path = self._write_minimal_inputs(tmp_path)
            with self.assertRaisesRegex(ValueError, "finishability evidence was required"):
                gcr.generate_report(
                    csv_path=str(csv_path),
                    figures_dir=str(tmp_path / "figures"),
                    report_path=str(tmp_path / "accuracy_report.md"),
                    figures_rel_dir="figures",
                    repo_root=tmp_path,
                    selected_run_id=self.RUN_ID,
                    selected_head_sha=self.HEAD_SHA,
                    env={},
                    require_finishability=True,
                )


class TierConfigTest(unittest.TestCase):
    """Tests for load_tier_config() and get_tier()."""

    def test_missing_file_returns_empty(self):
        with tempfile.TemporaryDirectory() as tmp:
            tier1, fixed = gcr.load_tier_config(Path(tmp))
            self.assertEqual(tier1, frozenset())
            self.assertEqual(fixed, {})

    def test_loads_known_tier_config(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            (tmp_path / "scripts").mkdir()
            (tmp_path / "scripts" / "benchmark_tiers.json").write_text(
                json.dumps({
                    "tier1": ["fp32_fma", "triad"],
                    "per_benchmark_fixed_time_ms": {"fp32_fma": 0.1},
                }),
                encoding="utf-8",
            )
            tier1, fixed = gcr.load_tier_config(tmp_path)
            self.assertIn("fp32_fma", tier1)
            self.assertIn("triad", tier1)
            self.assertAlmostEqual(fixed["fp32_fma"], 0.1)

    def test_loads_time_calibration_models(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            (tmp_path / "scripts").mkdir()
            (tmp_path / "scripts" / "benchmark_tiers.json").write_text(
                json.dumps({
                    "tier1": ["fp32_fma"],
                    "per_benchmark_fixed_time_ms": {"fp32_fma": 0.1},
                    "per_benchmark_time_models": {
                        "adjust_weights": {
                            "sim_scale": 0.5,
                            "fixed_time_ms": 0.2,
                            "source": "affine unit-test model",
                        }
                    },
                }),
                encoding="utf-8",
            )

            tier1, calibrations = gcr.load_time_calibrations(tmp_path)
            self.assertIn("fp32_fma", tier1)
            self.assertAlmostEqual(calibrations["fp32_fma"].adjusted_ms(1.0), 1.1)
            self.assertEqual(
                calibrations["fp32_fma"].source,
                "per_benchmark_fixed_time_ms",
            )
            self.assertAlmostEqual(
                calibrations["adjust_weights"].adjusted_ms(4.0), 2.2
            )
            self.assertEqual(
                calibrations["adjust_weights"].source,
                "affine unit-test model",
            )

    def test_loads_region_time_calibration_models(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            (tmp_path / "scripts").mkdir()
            (tmp_path / "scripts" / "benchmark_tiers.json").write_text(
                json.dumps({
                    "per_benchmark_time_models": {
                        "mem_latency_chase": {
                            "source": "selected-run piecewise fit",
                            "regions": [
                                {
                                    "max_problem_size": 10,
                                    "sim_scale": 2.0,
                                    "fixed_time_ms": 1.0,
                                    "source": "small region",
                                },
                                {
                                    "min_problem_size": 11,
                                    "sim_scale": 3.0,
                                    "fixed_time_ms": -1.0,
                                    "source": "large region",
                                },
                            ],
                        }
                    }
                }),
                encoding="utf-8",
            )

            _, calibrations = gcr.load_time_calibrations(tmp_path)
            model = calibrations["mem_latency_chase"]
            self.assertFalse(model.is_identity())
            self.assertEqual(len(model.regions), 2)
            self.assertAlmostEqual(model.adjusted_ms(2.0, "8"), 5.0)
            self.assertAlmostEqual(model.adjusted_ms(2.0, "16"), 5.0)
            self.assertAlmostEqual(model.adjusted_ms(2.0), 2.0)
            self.assertEqual(model.regions[0].source, "small region")

    def test_get_tier_classifies_correctly(self):
        tier1 = frozenset({"fp32_fma"})
        self.assertEqual(gcr.get_tier("fp32_fma", tier1), "tier1")
        self.assertEqual(gcr.get_tier("layernorm", tier1), "tier2")


if __name__ == "__main__":
    unittest.main()
