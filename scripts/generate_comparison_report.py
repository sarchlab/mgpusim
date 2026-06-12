#!/usr/bin/env python3
"""Generate per-benchmark accuracy comparison charts and an accuracy report.

Reads ``benchmark-comparison/comparison_ci.csv`` and, for each benchmark
with at least one row where both ``real_ms`` and ``sim_ms`` are populated:

1. Reads ``workloads/<suite>/<bench>/params.json`` (when available) to
   identify the scaling parameter and any non-scaling parameters.
2. Emits one PNG chart per unique combination of non-scaling-parameter
   values observed in the CSV, with the numeric scaling parameter value on
   the x-axis and execution time (ms) on the y-axis.  Two lines are drawn:
   Hardware (MI300A) and Simulator (MGPUSim).
3. Computes per-benchmark and global error metrics:

       err           = |sim - hw| / hw
       symmetric_err = |sim - hw| / min(sim, hw)

   ``sim`` is the raw ``sim_ms`` value unless a report-time affine calibration
   is configured in ``scripts/benchmark_tiers.json``.

The accuracy report is written as ``accuracy_report.md`` at the repo root
and uploaded as a CI artifact; it is committed to the repository.

Usage::

    python3 scripts/generate_comparison_report.py
"""

from __future__ import annotations

import argparse
import csv
import hashlib
import io
import json
import math
import os
import re
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, Iterable, List, Mapping, Optional, Sequence, Tuple

import matplotlib

matplotlib.use("Agg")  # non-interactive backend
import matplotlib.pyplot as plt
import pandas as pd

REPO_ROOT = Path(__file__).resolve().parents[1]
CSV_PATH = REPO_ROOT / "benchmark-comparison" / "comparison_ci.csv"
FIGURES_DIR = REPO_ROOT / "docs" / "figures"
REPORT_PATH = REPO_ROOT / "accuracy_report.md"
FIGURES_REL_DIR = "docs/figures"

# ---------------------------------------------------------------------------
# Kernel-name → workloads/<suite>/<benchmark> mapping
#
# Maps the ``kernel_name`` column in comparison_ci.csv to the workloads
# subdirectory that holds the benchmark's ``params.json``.  Sub-kernels
# sharing a parent benchmark point to the same params.json.
# ---------------------------------------------------------------------------
_KERNEL_TO_BENCHMARK: Dict[str, Tuple[str, str]] = {
    # polybench
    "2dconvolution": ("polybench", "2DConvolution"),
    "2mm": ("polybench", "2mm"),
    "3dconvolution": ("polybench", "3DConvolution"),
    "3mm": ("polybench", "3mm"),
    "atax": ("polybench", "atax"),
    "bicg": ("polybench", "bicg"),
    "correlation": ("polybench", "correlation"),
    "covariance": ("polybench", "covariance"),
    "fdtd2d": ("polybench", "fdtd2d"),
    "gemm": ("polybench", "gemm"),
    "gesummv": ("polybench", "gesummv"),
    "gramschmidt": ("polybench", "gramschmidt"),
    "mvt": ("polybench", "mvt"),
    "syr2k": ("polybench", "syr2k"),
    "syrk": ("polybench", "syrk"),
    # parboil
    "cutoff_potential": ("parboil", "cutcp"),
    "histogram": ("parboil", "histo"),
    "lbm_stream_collide": ("parboil", "lbm"),
    "gridding_kernel": ("parboil", "mri-gridding"),
    "computeq": ("parboil", "mri-q"),
    "computesad": ("parboil", "sad"),
    "findminsad": ("parboil", "sad"),
    "sgemm_tiled": ("parboil", "sgemm"),
    "spmv_csr": ("parboil", "spmv"),
    "stencil_kernel": ("parboil", "stencil"),
    "tpacf_dd": ("parboil", "tpacf"),
    "tpacf_dr": ("parboil", "tpacf"),
    "tpacf_rr": ("parboil", "tpacf"),
    # heteromark
    "bs": ("heteromark", "BS"),
    "ep": ("heteromark", "EP"),
    "ga": ("heteromark", "GA"),
    "hist": ("heteromark", "HIST"),
    "pagerank": ("heteromark", "PR"),
    # lonestar
    "bfs": ("lonestar", "bfs"),
    "bh": ("lonestar", "bh"),
    "dmr": ("lonestar", "dmr"),
    "sssp": ("lonestar", "sssp"),
    # rodinia
    "adjust_weights": ("rodinia", "backprop"),
    "layerforward": ("rodinia", "backprop"),
    "dwt2d": ("rodinia", "dwt2d"),
    "gaussian": ("rodinia", "gaussian"),
    "hotspot": ("rodinia", "hotspot"),
    "hotspot3d": ("rodinia", "hotspot3D"),
    "huffman": ("rodinia", "huffman"),
    "kmeans": ("rodinia", "kmeans"),
    "lavamd": ("rodinia", "lavaMD"),
    "lud": ("rodinia", "lud"),
    "nn": ("rodinia", "nn"),
    "nw": ("rodinia", "nw"),
    "particlefilter": ("rodinia", "particlefilter"),
    "pathfinder": ("rodinia", "pathfinder"),
    "srad": ("rodinia", "srad"),
    "streamcluster_pgain": ("rodinia", "streamcluster"),
    # shoc
    "busspeeddownload": ("shoc", "BusSpeedDownload"),
    "busspeedreadback": ("shoc", "BusSpeedReadback"),
    "devicememory_read": ("shoc", "DeviceMemory"),
    "devicememory_write": ("shoc", "DeviceMemory"),
    "maxflops": ("shoc", "MaxFlops"),
    "md": ("shoc", "MD"),
    "md5hash": ("shoc", "MD5Hash"),
    "qtc": ("shoc", "QTC"),
    "reduction": ("shoc", "Reduction"),
    "s3d": ("shoc", "S3D"),
    "scan": ("shoc", "Scan"),
    "sort": ("shoc", "Sort"),
    "triad": ("shoc", "Triad"),
    # microbench
    "atomic_throughput": ("microbench", "atomic_throughput"),
    "branch_div_50pct": ("microbench", "branch_divergence"),
    "fp32_fma": ("microbench", "fp32_throughput"),
    "fp64_fma": ("microbench", "fp64_throughput"),
    "gelu": ("microbench", "gelu"),
    "global_bw_copy": ("microbench", "global_bandwidth"),
    "int_mad": ("microbench", "int_throughput"),
    "l1_cache_bw": ("microbench", "l1_cache_bw"),
    "l2_cache_bw": ("microbench", "l2_cache_bw"),
    "mem_latency_chase": ("microbench", "mem_latency"),
    "occupancy_fma": ("microbench", "occupancy_sweep"),
    "sfun_sin": ("microbench", "sfun_throughput"),
    "shared_bw": ("microbench", "shared_bandwidth"),
    # ml_kernels
    "fused_swiglu": ("ml_kernels", "fused_swiglu"),
    "layernorm": ("ml_kernels", "layernorm"),
    "naive_attention": ("ml_kernels", "attention"),
    "rope": ("ml_kernels", "rope"),
    "softmax": ("ml_kernels", "softmax"),
    "tiled_gemm_16": ("ml_kernels", "gemm"),
}


# ---------------------------------------------------------------------------
# Tier classification
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class TimeCalibrationRegion:
    """One problem-size region for a report-time affine calibration."""

    sim_scale: float = 1.0
    fixed_time_ms: float = 0.0
    min_problem_size: Optional[float] = None
    max_problem_size: Optional[float] = None
    source: str = ""
    label: str = ""

    def adjusted_ms(self, sim_ms: float) -> float:
        """Return ``sim_scale * sim_ms + fixed_time_ms`` for this region."""
        return self.sim_scale * float(sim_ms) + self.fixed_time_ms

    def is_identity(self) -> bool:
        """Return True when this region leaves raw simulator time unchanged."""
        return self.sim_scale == 1.0 and self.fixed_time_ms == 0.0

    def matches(self, problem_size: object) -> bool:
        """Return True when *problem_size* falls inside this region."""
        numeric = _problem_size_numeric(problem_size)
        if numeric is None:
            return False
        if self.min_problem_size is not None and numeric < self.min_problem_size:
            return False
        if self.max_problem_size is not None and numeric > self.max_problem_size:
            return False
        return True


@dataclass(frozen=True)
class TimeCalibration:
    """Report-time affine calibration applied to raw simulator times.

    The committed comparison CSV remains the raw source of truth.  This model is
    used only while generating report metrics and calibrated chart lines.
    Optional regions let a benchmark use different affine coefficients across
    problem-size ranges without rewriting or filtering source rows.
    """

    sim_scale: float = 1.0
    fixed_time_ms: float = 0.0
    source: str = ""
    regions: Tuple[TimeCalibrationRegion, ...] = ()

    def adjusted_ms(self, sim_ms: float, problem_size: object = None) -> float:
        """Return the calibrated simulator time for *sim_ms* and *problem_size*."""
        region = self.region_for_problem_size(problem_size)
        if region is not None:
            return region.adjusted_ms(sim_ms)
        return self.sim_scale * float(sim_ms) + self.fixed_time_ms

    def region_for_problem_size(
        self, problem_size: object
    ) -> Optional[TimeCalibrationRegion]:
        """Return the first configured region matching *problem_size*, if any."""
        if problem_size is None:
            return None
        for region in self.regions:
            if region.matches(problem_size):
                return region
        return None

    def is_identity(self) -> bool:
        """Return True when the raw simulator time is unchanged everywhere."""
        if self.sim_scale != 1.0 or self.fixed_time_ms != 0.0:
            return False
        return all(region.is_identity() for region in self.regions)


IDENTITY_TIME_CALIBRATION = TimeCalibration()


def _load_tier_config_data(repo_root: Path) -> dict:
    """Return parsed ``scripts/benchmark_tiers.json`` or ``{}`` on failure."""
    path = Path(repo_root) / "scripts" / "benchmark_tiers.json"
    if not path.is_file():
        return {}
    try:
        with path.open(encoding="utf-8") as fh:
            data = json.load(fh)
    except (json.JSONDecodeError, OSError):
        return {}
    return data if isinstance(data, dict) else {}


def _parse_tier1_set(data: dict) -> frozenset:
    raw = data.get("tier1", []) or []
    if not isinstance(raw, list):
        return frozenset()
    return frozenset(str(kernel) for kernel in raw if str(kernel))


def _finite_float(value: object) -> Optional[float]:
    try:
        parsed = float(value)
    except (TypeError, ValueError):
        return None
    return parsed if math.isfinite(parsed) else None


def _problem_size_numeric(value: object) -> Optional[float]:
    """Return a numeric problem-size value for calibration-region matching."""
    if value is None:
        return None
    numeric = extract_scaling_value(str(value))
    if numeric != numeric:  # NaN
        return None
    return numeric


def _optional_finite_float(
    raw: Mapping[str, object],
    *keys: str,
) -> Tuple[bool, Optional[float]]:
    """Return ``(present, finite_value)`` for the first present key in *raw*."""
    for key in keys:
        if key in raw:
            return True, _finite_float(raw.get(key))
    return False, None


def _parse_time_calibration_regions(
    raw_model: Mapping[str, object],
    default_source: str,
) -> Tuple[TimeCalibrationRegion, ...]:
    """Parse optional per-size calibration regions from a config model."""
    regions_raw = raw_model.get("regions", ()) or ()
    if not isinstance(regions_raw, list):
        return ()

    regions: List[TimeCalibrationRegion] = []
    for raw_region in regions_raw:
        if not isinstance(raw_region, dict):
            continue
        sim_scale = _finite_float(raw_region.get("sim_scale", 1.0))
        fixed_time_ms = _finite_float(raw_region.get("fixed_time_ms", 0.0))
        if sim_scale is None or fixed_time_ms is None or sim_scale < 0:
            continue

        min_present, min_size = _optional_finite_float(
            raw_region, "min_problem_size", "min_size"
        )
        max_present, max_size = _optional_finite_float(
            raw_region, "max_problem_size", "max_size"
        )
        if (min_present and min_size is None) or (max_present and max_size is None):
            continue
        if min_size is not None and max_size is not None and min_size > max_size:
            continue

        source = str(raw_region.get("source") or default_source)
        label = str(raw_region.get("label") or "")
        regions.append(
            TimeCalibrationRegion(
                sim_scale=sim_scale,
                fixed_time_ms=fixed_time_ms,
                min_problem_size=min_size,
                max_problem_size=max_size,
                source=source,
                label=label,
            )
        )
    return tuple(regions)


def _parse_fixed_time_offsets(data: dict) -> Dict[str, float]:
    fixed_raw = data.get("per_benchmark_fixed_time_ms", {}) or {}
    if not isinstance(fixed_raw, dict):
        return {}

    fixed: Dict[str, float] = {}
    for kernel, value in fixed_raw.items():
        parsed = _finite_float(value)
        if parsed is None:
            continue
        fixed[str(kernel)] = parsed
    return fixed


def load_tier_config(
    repo_root: Path = REPO_ROOT,
) -> Tuple[frozenset, Dict[str, float]]:
    """Load tier-1 kernel set and legacy per-benchmark fixed-time offsets.

    Reads ``scripts/benchmark_tiers.json``.  Returns
    ``(tier1_set, fixed_time_ms_dict)`` and falls back to empty values when
    the file is missing or malformed so callers can keep computing reports.
    """
    data = _load_tier_config_data(repo_root)
    return _parse_tier1_set(data), _parse_fixed_time_offsets(data)


def load_time_calibrations(
    repo_root: Path = REPO_ROOT,
) -> Tuple[frozenset, Dict[str, TimeCalibration]]:
    """Load tier-1 kernels and report-time simulator calibration models.

    ``per_benchmark_fixed_time_ms`` entries are interpreted as legacy affine
    models with ``sim_scale=1``.  ``per_benchmark_time_models`` may override
    those entries with explicit ``sim_scale`` and ``fixed_time_ms`` values.
    """
    data = _load_tier_config_data(repo_root)
    calibrations = {
        kernel: TimeCalibration(
            sim_scale=1.0,
            fixed_time_ms=fixed_ms,
            source="per_benchmark_fixed_time_ms",
        )
        for kernel, fixed_ms in _parse_fixed_time_offsets(data).items()
    }

    models_raw = data.get("per_benchmark_time_models", {}) or {}
    if isinstance(models_raw, dict):
        for kernel, raw_model in models_raw.items():
            if not isinstance(raw_model, dict):
                continue
            sim_scale = _finite_float(raw_model.get("sim_scale", 1.0))
            fixed_time_ms = _finite_float(raw_model.get("fixed_time_ms", 0.0))
            if sim_scale is None or fixed_time_ms is None or sim_scale < 0:
                continue
            source = str(raw_model.get("source") or "per_benchmark_time_models")
            calibrations[str(kernel)] = TimeCalibration(
                sim_scale=sim_scale,
                fixed_time_ms=fixed_time_ms,
                source=source,
                regions=_parse_time_calibration_regions(raw_model, source),
            )

    return _parse_tier1_set(data), calibrations


def get_tier(kernel_name: str, tier1_set: frozenset) -> str:
    """Return ``'tier1'`` if *kernel_name* is in the calibration set, else ``'tier2'``."""
    return "tier1" if kernel_name in tier1_set else "tier2"


def _load_metadata_section(repo_root: Path, section: str) -> Dict[str, dict]:
    """Return one benchmark_tiers.json metadata section keyed by kernel."""
    data = _load_tier_config_data(repo_root)
    raw = data.get(section, {}) or {}
    if not isinstance(raw, dict):
        return {}
    return {
        str(kernel): dict(metadata)
        for kernel, metadata in raw.items()
        if str(kernel) and isinstance(metadata, dict)
    }


def _cache_bandwidth_report_metadata(repo_root: Path) -> Dict[str, dict]:
    """Return archived and current cache-bandwidth report metadata."""
    combined: Dict[str, dict] = {}
    flat_regions = _load_metadata_section(repo_root, "hardware_flat_regions")
    work_sweeps = _load_metadata_section(repo_root, "cache_bandwidth_work_sweeps")
    for kernel, metadata in flat_regions.items():
        combined.setdefault(kernel, {})["hardware_flat_region"] = metadata
    for kernel, metadata in work_sweeps.items():
        combined.setdefault(kernel, {})["work_scaling_region"] = metadata
    return combined


def _metadata_work_amount_scaled_value(metadata: Mapping[str, object]) -> Optional[bool]:
    value = metadata.get("work_amount_scaled_by_axis")
    if value is None:
        value = metadata.get("work_amount_scaled")
    return value if isinstance(value, bool) else None


def _metadata_work_amount(metadata: Mapping[str, object]) -> Mapping[str, object]:
    work_amount = metadata.get("work_amount")
    return work_amount if isinstance(work_amount, dict) else {}


def _metadata_total_read_bytes_formula(metadata: Mapping[str, object]) -> Optional[str]:
    work_amount = _metadata_work_amount(metadata)
    value = (
        metadata.get("total_read_bytes_formula")
        or work_amount.get("total_read_bytes_formula")
        or work_amount.get("formula")
    )
    return str(value) if value else None


def _metadata_default_total_read_bytes(metadata: Mapping[str, object]) -> Optional[object]:
    work_amount = _metadata_work_amount(metadata)
    return work_amount.get("default_total_read_bytes")


def _metadata_repeat_count_parameter_names(
    metadata: Mapping[str, object],
) -> Optional[Mapping[str, object]]:
    work_amount = _metadata_work_amount(metadata)
    names = work_amount.get("repeat_count_parameter_names")
    return names if isinstance(names, dict) else None


def _metadata_implementation_semantics(
    metadata: Mapping[str, object],
) -> Optional[Mapping[str, object]]:
    implementation = metadata.get("implementation_semantics")
    return implementation if isinstance(implementation, dict) else None


def _infer_selected_run_id_from_source(source: object) -> Optional[str]:
    match = re.search(
        r"(?:^|[/\\])selected-run-(\d+)(?:[/\\]|$)",
        str(source or ""),
    )
    return match.group(1) if match else None


def _finite_numeric_values(values: Iterable[object]) -> List[float]:
    out: List[float] = []
    for value in values:
        if not _has_value(value):
            continue
        try:
            parsed = float(value)
        except (TypeError, ValueError):
            continue
        if math.isfinite(parsed):
            out.append(parsed)
    return out


def _metadata_int(metadata: Mapping[str, object], key: str, default: int) -> int:
    try:
        return int(metadata.get(key, default))
    except (TypeError, ValueError):
        return default


def _proof_config(metadata: Mapping[str, object]) -> Mapping[str, object]:
    proof = metadata.get("value_change_proof")
    return proof if isinstance(proof, dict) else {}


def _proof_int(metadata: Mapping[str, object], key: str, default: int) -> int:
    proof = _proof_config(metadata)
    try:
        return int(proof.get(key, metadata.get(key, default)))
    except (TypeError, ValueError):
        return default


def _proof_float(metadata: Mapping[str, object], key: str, default: float) -> float:
    proof = _proof_config(metadata)
    try:
        parsed = float(proof.get(key, metadata.get(key, default)))
    except (TypeError, ValueError):
        return default
    return parsed if math.isfinite(parsed) else default


def _kernel_semantics_row_summary(df: pd.DataFrame, kernel: str) -> dict:
    rows = df[df["kernel_name"].astype(str) == kernel]
    real_values = _finite_numeric_values(rows["real_ms"].tolist())
    matched = rows[
        rows["real_ms"].apply(_has_value) & rows["sim_ms"].apply(_has_value)
    ]
    sim_values = _finite_numeric_values(matched["sim_ms"].tolist())
    problem_sizes = [str(value) for value in matched["problem_size"].tolist()]
    hw_min = min(real_values) if real_values else None
    hw_max = max(real_values) if real_values else None
    return {
        "hardware_rows": len(real_values),
        "matched_rows": len(sim_values),
        "distinct_problem_sizes": len(set(problem_sizes)),
        "hardware_min_ms": hw_min,
        "hardware_max_ms": hw_max,
        "hardware_value_change_ratio": (
            hw_max / hw_min if hw_min is not None and hw_min > 0 else None
        ),
        "all_hardware_rows_flat": (
            bool(real_values) and hw_min is not None
            and all(value <= 2 * hw_min for value in real_values)
        ),
    }


def _archived_flat_gate(
    metadata: Mapping[str, object],
    summary: Mapping[str, object],
    source_csv_label: str,
) -> dict:
    """Return archived fixed-work selected-run provenance gate details."""
    required = (
        metadata.get("classification") == "true_hardware_flat_region"
        and metadata.get("work_amount_scaled_by_axis") is False
    )
    checks = []

    def add(name: str, passed: bool, actual: object, expected: object) -> None:
        checks.append({
            "name": name,
            "passed": passed,
            "actual": actual,
            "expected": expected,
        })

    if not required:
        return {
            "required": False,
            "passed": True,
            "checks": checks,
            "reason": "not archived fixed-work metadata",
        }

    metadata_source = str(metadata.get("source") or "")
    add(
        "source/data_source",
        source_csv_label == metadata_source,
        source_csv_label,
        metadata_source,
    )
    metadata_run = str(metadata.get("selected_run_id") or "")
    loaded_run = _infer_selected_run_id_from_source(source_csv_label) or ""
    if metadata_run:
        add("selected_run_id", loaded_run == metadata_run, loaded_run, metadata_run)
    add(
        "loaded_hardware_rows_all_flat",
        bool(summary.get("all_hardware_rows_flat")),
        summary.get("all_hardware_rows_flat"),
        True,
    )
    for key in ("hardware_rows", "sim_matched_rows"):
        if key in metadata:
            summary_key = "matched_rows" if key == "sim_matched_rows" else key
            add(
                key,
                summary.get(summary_key) == _metadata_int(metadata, key, -1),
                summary.get(summary_key),
                metadata.get(key),
            )
    failed = [check["name"] for check in checks if not check["passed"]]
    return {
        "required": True,
        "passed": not failed,
        "checks": checks,
        "reason": (
            "OK" if not failed
            else "archived provenance gate failed: " + ", ".join(failed)
        ),
    }


def _work_scaling_gate(metadata: Mapping[str, object], summary: Mapping[str, object]) -> dict:
    """Return current work-scaling value-change proof gate details."""
    required = (
        metadata.get("classification") == "cache_bandwidth_work_sweep"
        and _metadata_work_amount_scaled_value(metadata) is True
    )
    checks = []

    def add(name: str, passed: bool, actual: object, expected: object) -> None:
        checks.append({
            "name": name,
            "passed": passed,
            "actual": actual,
            "expected": expected,
        })

    if not required:
        return {
            "required": False,
            "passed": True,
            "checks": checks,
            "reason": "not work-scaling metadata",
        }

    min_matched = _proof_int(metadata, "minimum_matched_rows", 3)
    min_sizes = _proof_int(metadata, "minimum_distinct_problem_sizes", min_matched)
    ratio_threshold = _proof_float(
        metadata,
        "hardware_min_to_max_ratio_exclusive",
        _proof_float(metadata, "hardware_value_change_min_ratio", 2.0),
    )
    ratio = summary.get("hardware_value_change_ratio")
    add(
        "work_amount_scaled_by_axis",
        _metadata_work_amount_scaled_value(metadata) is True,
        _metadata_work_amount_scaled_value(metadata),
        True,
    )
    add(
        "total_read_bytes_defined",
        bool(_metadata_total_read_bytes_formula(metadata)),
        _metadata_total_read_bytes_formula(metadata),
        "total_read_bytes formula",
    )
    add(
        "loaded_matched_rows_min",
        summary.get("matched_rows", 0) >= min_matched,
        summary.get("matched_rows"),
        f">={min_matched}",
    )
    add(
        "loaded_distinct_problem_sizes_min",
        summary.get("distinct_problem_sizes", 0) >= min_sizes,
        summary.get("distinct_problem_sizes"),
        f">={min_sizes}",
    )
    add(
        "loaded_hardware_value_changing",
        ratio is not None and ratio > ratio_threshold,
        ratio,
        f">{ratio_threshold:g}x min hardware time",
    )
    failed = [check["name"] for check in checks if not check["passed"]]
    return {
        "required": True,
        "passed": not failed,
        "checks": checks,
        "reason": "OK" if not failed else "value-change proof failed: " + ", ".join(failed),
    }


def evaluate_cache_bandwidth_report_semantics(
    df: pd.DataFrame,
    source_csv_label: str,
    repo_root: Path = REPO_ROOT,
) -> Dict[str, dict]:
    """Classify L1/L2 cache-bandwidth rows for comparison-report semantics."""
    metadata_by_kernel = _cache_bandwidth_report_metadata(repo_root)
    out: Dict[str, dict] = {}
    for kernel in sorted(metadata_by_kernel):
        metadata = metadata_by_kernel[kernel]
        summary = _kernel_semantics_row_summary(df, kernel)
        if summary["hardware_rows"] == 0 and summary["matched_rows"] == 0:
            continue
        archived = metadata.get("hardware_flat_region") or {}
        work = metadata.get("work_scaling_region") or {}
        archived_gate = (
            _archived_flat_gate(archived, summary, source_csv_label)
            if archived else {
                "required": False,
                "passed": True,
                "checks": [],
                "reason": "no archived metadata",
            }
        )
        if archived_gate.get("required") and archived_gate.get("passed"):
            active = archived
            report_class = "archived fixed-work cache-residency sweep"
            metric_policy = "diagnostic_only_archived_fixed_work"
            value_change_gate = None
        elif work:
            value_change_gate = _work_scaling_gate(work, summary)
            active = work
            if value_change_gate.get("passed"):
                report_class = "cache bandwidth work-scaling sweep"
                metric_policy = "scored_value_changing_work_sweep"
            else:
                report_class = "cache bandwidth work-scaling sweep (pending value-change proof)"
                metric_policy = "diagnostic_until_value_changing_rows"
        else:
            active = {}
            report_class = "default comparison metrics"
            metric_policy = "scored"
            value_change_gate = None

        out[kernel] = {
            "kernel": kernel,
            "report_class": report_class,
            "metric_policy": metric_policy,
            "work_amount_scaled_by_axis": _metadata_work_amount_scaled_value(active),
            "scaling_axis": active.get("scaling_axis"),
            "scaling_axis_role": active.get("scaling_axis_role"),
            "total_read_bytes_formula": _metadata_total_read_bytes_formula(active),
            "default_total_read_bytes": _metadata_default_total_read_bytes(active),
            "repeat_count_parameter_names": _metadata_repeat_count_parameter_names(active),
            "implementation_semantics": _metadata_implementation_semantics(active),
            "source": active.get("source"),
            "selected_run_id": active.get("selected_run_id"),
            "row_summary": summary,
            "archived_metadata_gate": archived_gate,
            "value_change_gate": value_change_gate,
        }
    return out


# ---------------------------------------------------------------------------
# Non-scaling parameter parsing
# ---------------------------------------------------------------------------


_NS_TOKEN_RE = re.compile(r"^([A-Za-z]+)(\d+)$")


def parse_nonscaling_values(
    problem_size: str,
    scaling_param: str,
    non_scaling_params: dict,
) -> Dict[str, str]:
    """Extract non-scaling parameter values from a ``problem_size`` token string.

    The ``problem_size`` is expected to be ``_``-separated tokens of the form
    ``<letters><digits>`` (e.g. ``M10240_N768_K768_tile16``).  This returns a
    dict ``{key: value_str}`` for every token whose key is present in
    *non_scaling_params* and is not the scaling parameter key.

    Returns an empty dict when *non_scaling_params* is falsy or no matching
    tokens are found.
    """
    if not non_scaling_params:
        return {}
    if not problem_size:
        return {}
    result: Dict[str, str] = {}
    for tok in str(problem_size).split("_"):
        m = _NS_TOKEN_RE.match(tok)
        if not m:
            continue
        key, value = m.group(1), m.group(2)
        if key == scaling_param:
            continue
        if key in non_scaling_params:
            result[key] = value
    return result


def load_benchmark_params(
    kernel_name: str, repo_root: Path = REPO_ROOT
) -> Optional[dict]:
    """Return parsed params.json for *kernel_name*, or ``None`` if unavailable.

    Looks up ``_KERNEL_TO_BENCHMARK`` and reads the corresponding
    ``workloads/<suite>/<bench>/params.json``.
    """
    entry = _KERNEL_TO_BENCHMARK.get(kernel_name.lower())
    if entry is None:
        return None
    suite, bench = entry
    params_path = repo_root / "workloads" / suite / bench / "params.json"
    if not params_path.is_file():
        return None
    try:
        with params_path.open(encoding="utf-8") as fh:
            return json.load(fh)
    except (json.JSONDecodeError, OSError):
        return None


def extract_scaling_value(
    problem_size: str,
    scaling_values: Optional[List] = None,
) -> float:
    """Extract the numeric scaling-parameter value from a problem_size string.

    Strategy (applied in order):

    1. Pure integer string → parse directly.
    2. ``NxM`` or ``NxMxP`` dimension format → use the leading dimension ``N``.
    3. When *scaling_values* is given: scan integers left-to-right and
       return the first that appears in the known scaling-value set.
    4. Fallback: return the first integer found in the string.

    Returns ``float('nan')`` when no integer can be extracted.
    """
    s = problem_size.strip()

    # Case 1: pure integer
    if re.fullmatch(r"\d+", s):
        return float(s)

    # Case 2: NxM... dimension string
    if re.fullmatch(r"\d+(?:[xX]\d+)+", s):
        return float(re.match(r"(\d+)", s).group(1))

    # Extract all integers from the string
    nums = [int(n) for n in re.findall(r"\d+", s)]
    if not nums:
        return float("nan")

    # Case 3: match against known scaling values
    if scaling_values:
        scaling_set = set(scaling_values)
        for n in nums:
            if n in scaling_set:
                return float(n)

    # Case 4: fallback to first integer
    return float(nums[0])


def compute_error_metrics(
    hw_vals: Sequence[float], sim_vals: Sequence[float]
) -> Dict[str, float]:
    """Compute relative and symmetric-relative error metrics.

    For every paired ``(hw, sim)`` sample where both values are strictly
    positive the function evaluates::

        err           = abs(sim - hw) / hw
        symmetric_err = abs(sim - hw) / min(sim, hw)

    Pairs where either value is ``None``/``NaN`` or non-positive are skipped.

    Returns a dict with ``mean_err``, ``max_err``, ``mean_sym_err`` and
    ``max_sym_err``.  Each metric is ``float('nan')`` when no usable pair
    is available.
    """
    if len(hw_vals) != len(sim_vals):
        raise ValueError(
            f"hw_vals and sim_vals must have equal length, "
            f"got {len(hw_vals)} and {len(sim_vals)}"
        )

    errs: List[float] = []
    sym_errs: List[float] = []
    for hw, sim in zip(hw_vals, sim_vals):
        if hw is None or sim is None:
            continue
        try:
            hw_f, sim_f = float(hw), float(sim)
        except (TypeError, ValueError):
            continue
        if hw_f != hw_f or sim_f != sim_f:  # NaN check
            continue
        if hw_f <= 0 or sim_f <= 0:
            continue
        diff = abs(sim_f - hw_f)
        errs.append(diff / hw_f)
        sym_errs.append(diff / min(sim_f, hw_f))

    nan = float("nan")
    if not errs:
        return {"mean_err": nan, "max_err": nan, "mean_sym_err": nan, "max_sym_err": nan}
    return {
        "mean_err": sum(errs) / len(errs),
        "max_err": max(errs),
        "mean_sym_err": sum(sym_errs) / len(sym_errs),
        "max_sym_err": max(sym_errs),
    }


# ---------------------------------------------------------------------------
# Run provenance and finishability evidence
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class ReportProvenance:
    """Selected GitHub Actions run identity rendered into the report."""

    run_id: Optional[str] = None
    head_sha: Optional[str] = None
    head_branch: Optional[str] = None
    workflow_name: Optional[str] = None
    status: Optional[str] = None
    conclusion: Optional[str] = None
    url: Optional[str] = None
    metadata_path: Optional[str] = None
    non_terminal_snapshot: bool = False

    def has_identity(self) -> bool:
        return bool(self.run_id or self.head_sha)

    def is_terminal(self) -> bool:
        return (
            (self.status or "").lower() == "completed"
            and not self.non_terminal_snapshot
        )


@dataclass(frozen=True)
class FinishabilitySummary:
    """Rendered per-tier finishability sections and their source description."""

    tier2_section: Optional[str]
    tier1_section: Optional[str]
    source_path: str
    source_note: str


_ENV_RUN_ID_KEYS = ("ACCURACY_SELECTED_RUN_ID", "GITHUB_RUN_ID")
_ENV_HEAD_SHA_KEYS = ("ACCURACY_SELECTED_HEAD_SHA", "GITHUB_SHA")
_ENV_METADATA_PATH_KEYS = (
    "ACCURACY_RUN_METADATA_JSON",
    "MI300A_SELECTED_RUN_METADATA_JSON",
)
_ENV_STATUS_KEYS = ("ACCURACY_SELECTED_RUN_STATUS",)
_ENV_CONCLUSION_KEYS = ("ACCURACY_SELECTED_RUN_CONCLUSION",)
_ENV_FINISHABILITY_PATH_KEYS = (
    "ACCURACY_FINISHABILITY_EVIDENCE_CSV",
    "MI300A_FINISHABILITY_EVIDENCE_CSV",
)

_SELECTED_FINISHABILITY_RUN_ID = "25619929396"
_SELECTED_FINISHABILITY_HEAD_SHA = "a281789a7354b86a8a04b350c145c573d531f54d"
_SELECTED_FINISHABILITY_STATUS = "completed"
_SELECTED_FINISHABILITY_PLAN_ROWS = 1416
_SELECTED_FINISHABILITY_ARTIFACT_NAME = "comparison-detailed"
_SELECTED_FINISHABILITY_ARTIFACT_ID = "6905152175"
_SELECTED_FINISHABILITY_ARTIFACT_SHA256 = (
    "867f125aa726ff3a06dff4b40d27008e897347da1001011cd5bd6207823a3a20"
)
_SELECTED_FINISHABILITY_METADATA_SHA256 = (
    "876d8992a9990108be248dc56575d28d4502d423bb22cd6ff76cac4f2d02ad70"
)


def _first_env(env: Mapping[str, str], keys: Sequence[str]) -> Optional[str]:
    for key in keys:
        value = env.get(key)
        if value:
            return value
    return None


def _clean_optional_text(value: object) -> Optional[str]:
    if value is None:
        return None
    text = str(value).strip()
    return text or None


def _metadata_value(raw: dict, *keys: str) -> Optional[str]:
    """Read a value from either flat run-view JSON or nested ``run`` JSON."""
    for key in keys:
        value = _clean_optional_text(raw.get(key))
        if value is not None:
            return value
    nested = raw.get("run")
    if isinstance(nested, dict):
        for key in keys:
            value = _clean_optional_text(nested.get(key))
            if value is not None:
                return value
    return None


def load_report_provenance(
    metadata_path: Optional[str] = None,
    env: Optional[Mapping[str, str]] = None,
    selected_run_id: Optional[str] = None,
    selected_head_sha: Optional[str] = None,
    selected_status: Optional[str] = None,
    selected_conclusion: Optional[str] = None,
) -> ReportProvenance:
    """Return selected-run provenance from a metadata JSON file and/or env.

    ``metadata_path`` may point to a GitHub run-view JSON (flat keys such as
    ``databaseId``/``headSha``) or to a collected provenance file with nested
    ``run.database_id``/``run.head_sha`` keys.  Explicit arguments and env vars
    are used as fallbacks when the metadata file is absent.
    """
    env = os.environ if env is None else env
    metadata_path = metadata_path or _first_env(env, _ENV_METADATA_PATH_KEYS)
    run_id = _clean_optional_text(selected_run_id) or _first_env(env, _ENV_RUN_ID_KEYS)
    head_sha = _clean_optional_text(selected_head_sha) or _first_env(env, _ENV_HEAD_SHA_KEYS)
    head_branch = _clean_optional_text(env.get("GITHUB_REF_NAME"))
    workflow_name = _clean_optional_text(env.get("GITHUB_WORKFLOW"))
    status = _clean_optional_text(selected_status) or _first_env(env, _ENV_STATUS_KEYS)
    conclusion = _clean_optional_text(selected_conclusion) or _first_env(env, _ENV_CONCLUSION_KEYS)
    url = None
    source_path = None
    non_terminal_snapshot = False

    if metadata_path:
        path = Path(metadata_path)
        try:
            raw = json.loads(path.read_text(encoding="utf-8"))
        except (json.JSONDecodeError, OSError) as exc:
            raise ValueError(f"failed to read run metadata {path}: {exc}") from exc
        if not isinstance(raw, dict):
            raise ValueError(f"run metadata {path} must be a JSON object")
        source_path = str(path)
        run_id = (
            _metadata_value(raw, "databaseId", "database_id", "run_id", "workflow_run_id")
            or run_id
        )
        head_sha = _metadata_value(raw, "headSha", "head_sha", "sha", "commit_sha") or head_sha
        head_branch = _metadata_value(raw, "headBranch", "head_branch", "branch") or head_branch
        workflow_name = (
            _metadata_value(raw, "workflowName", "workflow_name", "name")
            or workflow_name
        )
        status = _metadata_value(raw, "status") or status
        conclusion = _metadata_value(raw, "conclusion") or conclusion
        url = _metadata_value(raw, "url", "html_url")

        nested = raw.get("run")
        non_terminal_snapshot = bool(raw.get("non_terminal_snapshot"))
        if isinstance(nested, dict):
            non_terminal_snapshot = non_terminal_snapshot or bool(
                nested.get("non_terminal_snapshot")
            )
        if non_terminal_snapshot and (status or "").lower() != "completed":
            status = status or "non-terminal snapshot"

    return ReportProvenance(
        run_id=run_id,
        head_sha=head_sha,
        head_branch=head_branch,
        workflow_name=workflow_name,
        status=status,
        conclusion=conclusion,
        url=url,
        metadata_path=source_path,
        non_terminal_snapshot=non_terminal_snapshot,
    )


def _render_provenance_section(provenance: ReportProvenance) -> str:
    lines = ["## Selected CI Run Provenance", ""]
    if not provenance.has_identity():
        lines.append(
            "_Selected CI run provenance was not supplied. Set "
            "`ACCURACY_RUN_METADATA_JSON` or `ACCURACY_SELECTED_RUN_ID`/"
            "`ACCURACY_SELECTED_HEAD_SHA` when regenerating from selected CI artifacts._"
        )
        return "\n".join(lines)

    def add(label: str, value: Optional[str]) -> None:
        if value:
            lines.append(f"- {label}: `{value}`")

    add("Run ID", provenance.run_id)
    add("Head SHA", provenance.head_sha)
    add("Head branch", provenance.head_branch)
    add("Workflow", provenance.workflow_name)
    if provenance.status or provenance.conclusion:
        status = provenance.status or "unknown"
        conclusion = provenance.conclusion or "unknown"
        lines.append(f"- Run status/conclusion: `{status}` / `{conclusion}`")
    if provenance.url:
        lines.append(f"- Run URL: {provenance.url}")
    if provenance.metadata_path:
        lines.append(f"- Run metadata: `{provenance.metadata_path}`")
    return "\n".join(lines)


def _selected_finishability_dir(repo_root: Path) -> Path:
    return Path(repo_root) / "benchmark-comparison" / "selected-run-25619929396"


def _selected_finishability_metadata_path(repo_root: Path) -> Path:
    return _selected_finishability_dir(repo_root) / "run-view.json"


def _selected_finishability_artifact_path(repo_root: Path) -> Path:
    return _selected_finishability_dir(repo_root) / "comparison_ci.detailed.csv"


def _selected_finishability_artifact_inventory_path(repo_root: Path) -> Path:
    return _selected_finishability_dir(repo_root) / "artifacts.tsv"


def _selected_finishability_plan_path(repo_root: Path) -> Path:
    return (
        Path(repo_root)
        / "benchmark-comparison"
        / "mi300a_problem_size_discovery_plan.json"
    )


def _sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    try:
        with path.open("rb") as fh:
            for chunk in iter(lambda: fh.read(1024 * 1024), b""):
                digest.update(chunk)
    except OSError as exc:
        raise ValueError(f"failed to read {path}: {exc}") from exc
    return digest.hexdigest()


def _validate_selected_metadata_file(metadata_path: Path, repo_root: Path) -> None:
    """Require the supplied metadata to be the tracked selected run metadata."""
    selected_metadata = _selected_finishability_metadata_path(repo_root)
    try:
        raw = json.loads(metadata_path.read_text(encoding="utf-8"))
    except (json.JSONDecodeError, OSError) as exc:
        raise ValueError(
            f"failed to read selected run metadata {metadata_path}: {exc}"
        ) from exc
    if not isinstance(raw, dict):
        raise ValueError(
            f"selected run metadata {metadata_path} must be a JSON object"
        )

    run_id = _metadata_value(
        raw, "databaseId", "database_id", "run_id", "workflow_run_id"
    )
    head_sha = _metadata_value(raw, "headSha", "head_sha", "sha", "commit_sha")
    status = (_metadata_value(raw, "status") or "").lower()
    non_terminal_snapshot = bool(raw.get("non_terminal_snapshot"))
    nested = raw.get("run")
    if isinstance(nested, dict):
        non_terminal_snapshot = non_terminal_snapshot or bool(
            nested.get("non_terminal_snapshot")
        )

    if run_id != _SELECTED_FINISHABILITY_RUN_ID:
        raise ValueError(
            "finishability evidence metadata run "
            f"{run_id or '<missing>'} does not match selected terminal CI run "
            f"{_SELECTED_FINISHABILITY_RUN_ID}"
        )
    if (head_sha or "").lower() != _SELECTED_FINISHABILITY_HEAD_SHA:
        raise ValueError(
            "finishability evidence metadata head SHA "
            f"{head_sha or '<missing>'} does not match selected head SHA "
            f"{_SELECTED_FINISHABILITY_HEAD_SHA}"
        )
    if status != _SELECTED_FINISHABILITY_STATUS or non_terminal_snapshot:
        snapshot_note = "; non_terminal_snapshot=true" if non_terminal_snapshot else ""
        raise ValueError(
            "finishability evidence metadata rejected: selected run "
            f"{run_id} is not terminal (status={status or 'missing'}{snapshot_note})"
        )

    selected_sha = _sha256_file(selected_metadata)
    if selected_sha != _SELECTED_FINISHABILITY_METADATA_SHA256:
        raise ValueError(
            "checked-in selected run metadata changed: "
            f"{selected_metadata} has SHA-256 {selected_sha}, expected "
            f"{_SELECTED_FINISHABILITY_METADATA_SHA256}"
        )
    metadata_sha = _sha256_file(metadata_path)
    if metadata_sha != _SELECTED_FINISHABILITY_METADATA_SHA256:
        raise ValueError(
            "finishability evidence metadata is not the selected run metadata: "
            f"{metadata_path} has SHA-256 {metadata_sha}, expected "
            f"{_SELECTED_FINISHABILITY_METADATA_SHA256}"
        )


def _validate_selected_artifact_inventory(repo_root: Path) -> None:
    inventory_path = _selected_finishability_artifact_inventory_path(repo_root)
    try:
        with inventory_path.open(newline="", encoding="utf-8") as fh:
            rows = list(csv.DictReader(fh, delimiter="\t"))
    except OSError as exc:
        raise ValueError(
            f"failed to read selected artifact inventory {inventory_path}: {exc}"
        ) from exc

    for row in rows:
        if (
            row.get("id") == _SELECTED_FINISHABILITY_ARTIFACT_ID
            and row.get("name") == _SELECTED_FINISHABILITY_ARTIFACT_NAME
        ):
            if str(row.get("expired") or "").lower() == "true":
                raise ValueError(
                    "selected comparison-detailed artifact "
                    f"{_SELECTED_FINISHABILITY_ARTIFACT_ID} is marked expired"
                )
            return
    raise ValueError(
        "selected artifact inventory does not contain comparison-detailed "
        f"artifact {_SELECTED_FINISHABILITY_ARTIFACT_ID} for run "
        f"{_SELECTED_FINISHABILITY_RUN_ID}"
    )


def _validate_finishability_source(
    provenance: ReportProvenance,
    evidence_path: Path,
    repo_root: Path = REPO_ROOT,
) -> None:
    """Reject finishability inputs without selected terminal-run provenance."""
    if not provenance.run_id or not provenance.head_sha:
        raise ValueError(
            "finishability evidence requires selected run ID and head SHA; "
            f"evidence file {evidence_path} was supplied without complete provenance"
        )
    if not provenance.status:
        raise ValueError(
            "finishability evidence requires terminal run metadata with "
            f"status `completed`; no status was supplied for run {provenance.run_id}"
        )
    if not provenance.is_terminal():
        snapshot_note = (
            "; non_terminal_snapshot=true"
            if provenance.non_terminal_snapshot else ""
        )
        raise ValueError(
            "finishability evidence rejected: selected run "
            f"{provenance.run_id} is not terminal (status={provenance.status!r}"
            f"{snapshot_note}); evidence file {evidence_path} cannot be used"
        )
    if str(provenance.run_id) != _SELECTED_FINISHABILITY_RUN_ID:
        raise ValueError(
            "finishability evidence rejected: selected run "
            f"{provenance.run_id} does not match required terminal CI run "
            f"{_SELECTED_FINISHABILITY_RUN_ID}"
        )
    if (provenance.head_sha or "").lower() != _SELECTED_FINISHABILITY_HEAD_SHA:
        raise ValueError(
            "finishability evidence rejected: selected head SHA "
            f"{provenance.head_sha} does not match required head SHA "
            f"{_SELECTED_FINISHABILITY_HEAD_SHA}"
        )
    metadata_path = _clean_optional_text(provenance.metadata_path)
    if not metadata_path:
        raise ValueError(
            "finishability evidence requires selected run metadata JSON; "
            "run ID/head/status arguments alone are not sufficient"
        )
    _validate_selected_metadata_file(Path(metadata_path), repo_root)
    _validate_selected_artifact_inventory(repo_root)


def _require_comparison_columns(df: pd.DataFrame, source_label: str) -> pd.DataFrame:
    required = {"kernel_name", "problem_size", "real_ms", "sim_ms"}
    missing = required - set(df.columns)
    if missing:
        raise ValueError(f"CSV {source_label} is missing columns: {sorted(missing)}")
    return df


def _load_comparison_dataframe(csv_path: str) -> pd.DataFrame:
    return _require_comparison_columns(pd.read_csv(csv_path), csv_path)


def _load_comparison_dataframe_from_bytes(
    csv_bytes: bytes,
    source_label: str,
) -> pd.DataFrame:
    return _require_comparison_columns(pd.read_csv(io.BytesIO(csv_bytes)), source_label)


def _regular_plan_keys(
    repo_root: Path,
) -> Tuple[set[Tuple[str, str]], List[Tuple[str, str]]]:
    plan_path = _selected_finishability_plan_path(repo_root)
    try:
        raw = json.loads(plan_path.read_text(encoding="utf-8"))
    except (json.JSONDecodeError, OSError) as exc:
        raise ValueError(
            f"failed to read regular finishability plan {plan_path}: {exc}"
        ) from exc
    entries = raw.get("entries") if isinstance(raw, dict) else None
    if not isinstance(entries, list):
        raise ValueError(
            f"regular finishability plan {plan_path} must contain entries"
        )
    if len(entries) != _SELECTED_FINISHABILITY_PLAN_ROWS:
        raise ValueError(
            "regular finishability plan row count mismatch: "
            f"{len(entries)} entries, expected {_SELECTED_FINISHABILITY_PLAN_ROWS}"
        )

    keys_in_order: List[Tuple[str, str]] = []
    seen: set[Tuple[str, str]] = set()
    duplicates: List[Tuple[str, str]] = []
    for entry in entries:
        if not isinstance(entry, dict):
            raise ValueError(
                f"regular finishability plan {plan_path} has non-object entry"
            )
        kernel = _clean_optional_text(entry.get("hardware_kernel_name"))
        size = _clean_optional_text(entry.get("hardware_problem_size"))
        if kernel is None or size is None:
            raise ValueError(
                f"regular finishability plan {plan_path} has an entry without "
                "hardware kernel/problem-size keys"
            )
        key = (kernel, size)
        keys_in_order.append(key)
        if key in seen:
            duplicates.append(key)
        seen.add(key)
    if duplicates:
        raise ValueError(
            "regular finishability plan has duplicate rows: "
            f"{_format_plan_key_sample(duplicates)}"
        )
    if len(seen) != _SELECTED_FINISHABILITY_PLAN_ROWS:
        raise ValueError(
            "regular finishability plan unique row count mismatch: "
            f"{len(seen)} unique rows, expected {_SELECTED_FINISHABILITY_PLAN_ROWS}"
        )
    return seen, keys_in_order


def _format_plan_key_sample(keys: Sequence[Tuple[str, str]], limit: int = 5) -> str:
    sample = [f"{kernel}/{size}" for kernel, size in list(keys)[:limit]]
    suffix = "" if len(keys) <= limit else f", ... ({len(keys)} total)"
    return ", ".join(sample) + suffix


def _validate_finishability_regular_plan_rows(
    df: pd.DataFrame,
    evidence_path: Path,
    repo_root: Path,
) -> None:
    plan_keys, plan_keys_in_order = _regular_plan_keys(repo_root)
    plan_kernels = {kernel for kernel, _ in plan_keys}
    plan_order = {key: index for index, key in enumerate(plan_keys_in_order)}

    row_keys: List[Tuple[str, str]] = []
    row_key_counts: Dict[Tuple[str, str], int] = {}
    unknown_kernels: set[str] = set()
    malformed_rows = 0
    for _, row in df.iterrows():
        kernel = _clean_optional_text(row.get("kernel_name"))
        size = _clean_optional_text(row.get("problem_size"))
        if kernel is None or size is None:
            malformed_rows += 1
            continue
        if kernel not in plan_kernels:
            unknown_kernels.add(kernel)
        key = (kernel, size)
        row_keys.append(key)
        row_key_counts[key] = row_key_counts.get(key, 0) + 1

    row_key_set = set(row_keys)
    missing = sorted(plan_keys - row_key_set, key=lambda key: plan_order[key])
    extra = sorted(row_key_set - plan_keys)
    duplicates = sorted(key for key, count in row_key_counts.items() if count > 1)

    errors: List[str] = []
    if len(df.index) != _SELECTED_FINISHABILITY_PLAN_ROWS:
        errors.append(
            f"CSV has {len(df.index)} data rows; expected "
            f"{_SELECTED_FINISHABILITY_PLAN_ROWS} regular-plan rows"
        )
    if malformed_rows:
        errors.append(
            f"CSV has {malformed_rows} rows without kernel_name/problem_size values"
        )
    if unknown_kernels:
        errors.append(
            "unknown kernels not present in the selected regular plan: "
            f"{', '.join(sorted(unknown_kernels))}"
        )
    if duplicates:
        errors.append(
            "duplicate regular-plan rows in CSV: "
            f"{_format_plan_key_sample(duplicates)}"
        )
    if missing:
        errors.append(
            "missing regular-plan rows from CSV: "
            f"{_format_plan_key_sample(missing)}"
        )
    if extra:
        errors.append(
            "extra regular-plan rows not in selected plan: "
            f"{_format_plan_key_sample(extra)}"
        )
    if errors:
        raise ValueError(
            f"finishability evidence CSV {evidence_path} is not the selected "
            "1416-entry regular-plan evidence: "
            + "; ".join(errors)
        )


def _read_finishability_evidence_once(evidence_path: Path) -> Tuple[bytes, str]:
    try:
        evidence_bytes = evidence_path.read_bytes()
    except OSError as exc:
        raise ValueError(
            f"failed to read finishability evidence CSV {evidence_path}: {exc}"
        ) from exc
    evidence_sha = hashlib.sha256(evidence_bytes).hexdigest()
    return evidence_bytes, evidence_sha


def _finishability_selected_artifact_error(
    evidence_path: Path,
    evidence_sha: str,
    repo_root: Path,
) -> Optional[ValueError]:
    """Validate selected artifact identity for a pre-read evidence SHA."""
    selected_artifact = _selected_finishability_artifact_path(repo_root)
    selected_sha = _sha256_file(selected_artifact)
    if selected_sha != _SELECTED_FINISHABILITY_ARTIFACT_SHA256:
        raise ValueError(
            "checked-in selected comparison-detailed artifact changed: "
            f"{selected_artifact} has SHA-256 {selected_sha}, expected "
            f"{_SELECTED_FINISHABILITY_ARTIFACT_SHA256}"
        )
    if evidence_sha != _SELECTED_FINISHABILITY_ARTIFACT_SHA256:
        return ValueError(
            "finishability evidence CSV is not derived from selected "
            f"comparison-detailed artifact {_SELECTED_FINISHABILITY_ARTIFACT_ID}: "
            f"{evidence_path} has SHA-256 {evidence_sha}, expected "
            f"{_SELECTED_FINISHABILITY_ARTIFACT_SHA256}"
        )
    return None


def _matched_dataframe_from_full(df: pd.DataFrame) -> pd.DataFrame:
    """Return rows with both hardware and simulator timings as floats."""
    matched = df.dropna(subset=["real_ms", "sim_ms"]).copy()
    matched["real_ms"] = matched["real_ms"].astype(float)
    matched["sim_ms"] = matched["sim_ms"].astype(float)
    return matched


def _load_matched_dataframe(csv_path: str) -> pd.DataFrame:
    return _matched_dataframe_from_full(_load_comparison_dataframe(csv_path))


def _params_scaling_key(params: Optional[dict]) -> str:
    """Return the scaling-parameter key (e.g. ``'M'``) or empty string."""
    if not params:
        return ""
    sp = params.get("scaling_parameter") or {}
    return str(sp.get("key") or sp.get("name") or "")


def _params_scaling_values(params: Optional[dict]) -> Optional[List[float]]:
    """Return numeric scaling-parameter values from *params*, when available."""
    if not params:
        return None
    sp = params.get("scaling_parameter") or {}
    values = sp.get("values")
    if not isinstance(values, list):
        return None

    numeric_values: List[float] = []
    for value in values:
        try:
            numeric_values.append(float(value))
        except (TypeError, ValueError):
            continue
    return numeric_values or None


_SCALING_NAME_TOKEN_ALIASES: Dict[str, Tuple[str, ...]] = {
    "seq_len": ("S", "seq", "seqlen"),
    "sequence_length": ("S", "seq", "seqlen"),
    "num_rows": ("rows", "row", "R"),
    "num_elements": ("n", "N"),
}


def _params_scaling_token_keys(params: Optional[dict]) -> Tuple[str, ...]:
    """Return likely problem-size token prefixes for the scaling parameter.

    ``params.json`` files usually name the scaling parameter but do not always
    include the compact token used in ``problem_size`` strings. Prefer any
    explicit token/key fields and then add conservative aliases for formats used
    by committed comparison artifacts, such as ``seq_len`` -> ``S`` for
    ``B4_S128_H12_D64_blk256``.
    """
    if not params:
        return ()
    sp = params.get("scaling_parameter") or {}
    candidates: List[str] = []

    for field in ("key", "token", "prefix"):
        value = str(sp.get(field) or "").strip()
        if value:
            candidates.append(value)

    name = str(sp.get("name") or "").strip()
    if re.fullmatch(r"[A-Za-z]+", name):
        candidates.append(name)
    candidates.extend(_SCALING_NAME_TOKEN_ALIASES.get(name.lower(), ()))

    unique: List[str] = []
    seen: set[str] = set()
    for candidate in candidates:
        if candidate not in seen:
            unique.append(candidate)
            seen.add(candidate)
    return tuple(unique)


def _params_nonscaling_dict(params: Optional[dict]) -> dict:
    """Return the ``non_scaling_parameters`` dict from *params*, or ``{}``."""
    if not params:
        return {}
    return params.get("non_scaling_parameters") or {}


def _compute_nonscaling_groups(
    df: pd.DataFrame, params: Optional[dict]
) -> pd.DataFrame:
    """Add a ``_nonscaling_group`` column to *df* derived from problem_size.

    Each row's group is a tuple of ``(key, value)`` pairs sorted by key, or
    an empty tuple when no non-scaling parameters apply.
    """
    df = df.copy()
    scaling_key = _params_scaling_key(params)
    nsp = _params_nonscaling_dict(params)

    def _group(ps: object) -> tuple:
        values = parse_nonscaling_values(str(ps), scaling_key, nsp)
        return tuple(sorted(values.items()))

    df["_nonscaling_group"] = df["problem_size"].apply(_group)
    return df


def _chart_filename(kernel: str, combo: tuple) -> str:
    """Build the PNG filename for a (kernel, non-scaling combo) pair."""
    if not combo:
        return f"{kernel}_comparison.png"
    parts = "_".join(f"{k}{v}" for k, v in combo)
    return f"{kernel}_{parts}_comparison.png"


def _render_chart(
    kernel: str,
    group_df: pd.DataFrame,
    figures_dir: Path,
    params: Optional[dict],
    combo: tuple,
    time_calibration: TimeCalibration = IDENTITY_TIME_CALIBRATION,
) -> str:
    """Render a comparison PNG and return its filename."""
    figures_dir.mkdir(parents=True, exist_ok=True)

    scaling_name = "Scaling Parameter"
    scaling_values: Optional[List] = None
    if params is not None:
        sp = params.get("scaling_parameter", {})
        scaling_name = sp.get("name", scaling_name)
        scaling_values = sp.get("values")

    df = group_df.copy()
    df["_x"] = df["problem_size"].apply(
        lambda ps: extract_scaling_value(str(ps), scaling_values)
    )
    df = df.sort_values("_x").reset_index(drop=True)

    x = df["_x"].tolist()
    sim_vals = [
        time_calibration.adjusted_ms(row.sim_ms, row.problem_size)
        for row in df.itertuples(index=False)
    ]
    sim_label = "Simulator (MGPUSim)"
    if not time_calibration.is_identity():
        sim_label = "Simulator (MGPUSim, calibrated)"

    fig, ax = plt.subplots(figsize=(8, 5))
    ax.plot(
        x, df["real_ms"].tolist(),
        color="blue", linestyle="-", marker="o", markersize=5,
        label="Hardware (MI300A)",
    )
    ax.plot(
        x, sim_vals,
        color="red", linestyle="--", marker="s", markersize=5,
        label=sim_label,
    )
    ax.set_xlabel(scaling_name)
    ax.set_ylabel("Execution Time (ms)")

    title = kernel
    if combo:
        title = f"{kernel} ({', '.join(f'{k}={v}' for k, v in combo)})"
    ax.set_title(title)
    ax.legend()
    ax.grid(True, alpha=0.3)
    fig.tight_layout()

    filename = _chart_filename(kernel, combo)
    fig.savefig(figures_dir / filename, dpi=150, bbox_inches="tight")
    plt.close(fig)
    return filename


def _format_metric(value: float) -> str:
    if value != value:  # NaN
        return "n/a"
    return f"{value * 100:.2f}%"


def _format_calibration_float(value: float) -> str:
    return f"{value:.12g}"


def _escape_table_cell(value: str) -> str:
    return value.replace("\n", " ").replace("|", "\\|")


def _format_problem_size_bound(value: Optional[float]) -> str:
    if value is None:
        return ""
    if float(value).is_integer():
        return str(int(value))
    return _format_calibration_float(float(value))


def _format_calibration_region(region: Optional[TimeCalibrationRegion]) -> str:
    if region is None:
        return "all sizes"
    lower = _format_problem_size_bound(region.min_problem_size)
    upper = _format_problem_size_bound(region.max_problem_size)
    if lower and upper:
        span = lower if lower == upper else f"{lower}–{upper}"
    elif lower:
        span = f"≥{lower}"
    elif upper:
        span = f"≤{upper}"
    else:
        span = "all sizes"
    if region.label:
        return f"{region.label} ({span})" if span != "all sizes" else region.label
    return span


def _iter_time_calibration_rows(
    kernel: str,
    model: TimeCalibration,
) -> Iterable[Tuple[str, str, float, float, str]]:
    """Yield report rows for active global and per-region calibration models."""
    if model.regions:
        for region in model.regions:
            if region.is_identity():
                continue
            yield (
                kernel,
                _format_calibration_region(region),
                region.sim_scale,
                region.fixed_time_ms,
                region.source or model.source or "benchmark_tiers.json",
            )
        if model.sim_scale == 1.0 and model.fixed_time_ms == 0.0:
            return
        yield (
            kernel,
            "unmatched sizes",
            model.sim_scale,
            model.fixed_time_ms,
            model.source or "benchmark_tiers.json",
        )
        return

    if not model.is_identity():
        yield (
            kernel,
            "all sizes",
            model.sim_scale,
            model.fixed_time_ms,
            model.source or "benchmark_tiers.json",
        )


def _render_time_calibration_section(
    time_calibrations: Mapping[str, TimeCalibration],
) -> Optional[str]:
    active_rows: List[Tuple[str, str, float, float, str]] = []
    for kernel, model in sorted(time_calibrations.items()):
        active_rows.extend(_iter_time_calibration_rows(kernel, model))
    if not active_rows:
        return None

    lines = [
        "## Report-Time Simulator Calibration",
        "",
        "The comparison CSV keeps raw `sim_ms` values. Report metrics and "
        "calibrated chart lines apply these affine models without rewriting or "
        "filtering source rows. For region-aware rows, the problem-size region "
        "selects which affine model is applied:",
        "",
        "| Benchmark | Problem-size region | sim_scale | fixed_time_ms | Source |",
        "| --- | --- | ---: | ---: | --- |",
    ]
    for kernel, region, sim_scale, fixed_time_ms, source in active_rows:
        lines.append(
            "| {kernel} | {region} | {scale} | {fixed} | {source} |".format(
                kernel=_escape_table_cell(kernel),
                region=_escape_table_cell(region),
                scale=_format_calibration_float(sim_scale),
                fixed=_format_calibration_float(fixed_time_ms),
                source=_escape_table_cell(source),
            )
        )
    return "\n".join(lines)


def _format_semantics_bool(value: object) -> str:
    if value is True:
        return "yes"
    if value is False:
        return "no"
    return "unknown"


def _format_repeat_count_semantics(entry: Mapping[str, object]) -> str:
    names = entry.get("repeat_count_parameter_names")
    if not isinstance(names, dict):
        return ""
    aliases = []
    if names.get("hip_cuda"):
        aliases.append(f"HIP/CUDA `{names['hip_cuda']}`")
    if names.get("go_simulator"):
        aliases.append(f"Go simulator `{names['go_simulator']}`")
    return "repeat count " + ", ".join(aliases) if aliases else ""


def _format_total_read_bytes_semantics(entry: Mapping[str, object]) -> str:
    formula = entry.get("total_read_bytes_formula")
    default_bytes = entry.get("default_total_read_bytes")
    if not formula:
        return "—"
    pieces = [str(formula)]
    repeat_semantics = _format_repeat_count_semantics(entry)
    if default_bytes is not None:
        label = "HIP/CUDA default" if repeat_semantics else "default"
        pieces.append(f"{label} {default_bytes} B")
    if repeat_semantics:
        pieces.append(repeat_semantics)
    return "; ".join(pieces)


def _format_implementation_footprint_semantics(entry: Mapping[str, object]) -> str:
    implementation = entry.get("implementation_semantics")
    if not isinstance(implementation, dict):
        if entry.get("work_amount_scaled_by_axis") is False:
            return "archived fixed-work cache-residency footprint"
        return "—"

    parts = []
    hip = implementation.get("hip_cuda_workload")
    if isinstance(hip, dict):
        if hip.get("working_set_model") == "fixed_total_working_set_size":
            hip_text = "HIP/CUDA fixed working_set_size"
            if hip.get("working_set_size_bytes") is not None:
                hip_text += f" {hip['working_set_size_bytes']} B"
        else:
            hip_text = "HIP/CUDA " + str(hip.get("working_set_model") or "workload")
        if hip.get("repeat_parameter"):
            hip_text += f"; repeat {hip['repeat_parameter']}"
        parts.append(hip_text)

    simulator = implementation.get("go_simulator")
    if isinstance(simulator, dict):
        if simulator.get("working_set_model") == "per_256_thread_read_group_reuse":
            elems = simulator.get("read_group_elements")
            footprint = simulator.get("read_group_footprint_bytes")
            if elems and footprint:
                sim_text = f"Go simulator {elems}-thread read groups ({footprint} B each)"
            else:
                sim_text = "Go simulator per-read-group reuse"
        else:
            sim_text = "Go simulator " + str(
                simulator.get("working_set_model") or "workload"
            )
        if simulator.get("input_bytes_formula"):
            sim_text += f"; input bytes = {simulator['input_bytes_formula']}"
        if simulator.get("repeat_parameter"):
            sim_text += f"; repeat {simulator['repeat_parameter']}"
        parts.append(sim_text)

    return "; ".join(parts) if parts else "—"


def _format_semantics_policy(entry: Mapping[str, object]) -> str:
    policy = entry.get("metric_policy")
    if policy == "diagnostic_only_archived_fixed_work":
        return "diagnostic only under archived selected-run provenance gate"
    if policy == "diagnostic_until_value_changing_rows":
        return "diagnostic until loaded rows prove value-changing behavior"
    if policy == "scored_value_changing_work_sweep":
        return "scored; loaded rows prove value-changing work sweep"
    return str(policy or "scored")


def _format_value_change_proof(entry: Mapping[str, object]) -> str:
    gate = entry.get("value_change_gate")
    if isinstance(gate, dict) and gate.get("required"):
        return "passed" if gate.get("passed") else gate.get("reason", "failed")
    archived_gate = entry.get("archived_metadata_gate")
    if isinstance(archived_gate, dict) and archived_gate.get("required"):
        if archived_gate.get("passed"):
            return "archived provenance passed"
        return "archived provenance not active"
    return "not required"


def _render_cache_bandwidth_semantics_section(
    cache_bandwidth_semantics: Mapping[str, Mapping[str, object]],
) -> Optional[str]:
    if not cache_bandwidth_semantics:
        return None
    lines = [
        "## Cache Bandwidth Report Semantics",
        "",
        "L1/L2 cache-bandwidth rows have two possible report meanings. "
        "Archived selected-run fixed-work cache-residency rows are diagnostic-only "
        "only when their provenance gate passes. Current work-scaling rows mark "
        "`num_elements` as the work amount and define total read bytes; they are "
        "scored as value-changing evidence only after loaded rows prove non-flat "
        "hardware behavior. Current L1 metadata also distinguishes HIP/CUDA rows "
        "with fixed `working_set_size` from Go simulator rows whose per-256-thread "
        "read groups remain L1-resident while total input allocation grows with "
        "`num_elements`.",
        "",
        "| Benchmark | Report class | Work scaled? | Axis | Total read bytes | "
        "Implementation footprint | Value-change/provenance proof | "
        "Metric policy | Source |",
        "| --- | --- | ---: | --- | --- | --- | --- | --- | --- |",
    ]
    for kernel, entry in sorted(cache_bandwidth_semantics.items()):
        source = str(entry.get("source") or "metadata")
        selected_run_id = entry.get("selected_run_id")
        if selected_run_id:
            source = f"{source}; run {selected_run_id}"
        lines.append(
            "| {kernel} | {klass} | {work_scaled} | {axis} | {total_reads} | "
            "{footprint} | {proof} | {policy} | {source} |".format(
                kernel=_escape_table_cell(kernel),
                klass=_escape_table_cell(str(entry.get("report_class") or "unknown")),
                work_scaled=_format_semantics_bool(entry.get("work_amount_scaled_by_axis")),
                axis=_escape_table_cell(str(entry.get("scaling_axis") or "unknown")),
                total_reads=_escape_table_cell(_format_total_read_bytes_semantics(entry)),
                footprint=_escape_table_cell(
                    _format_implementation_footprint_semantics(entry)
                ),
                proof=_escape_table_cell(_format_value_change_proof(entry)),
                policy=_escape_table_cell(_format_semantics_policy(entry)),
                source=_escape_table_cell(source),
            )
        )
    return "\n".join(lines)


def _has_value(value: object) -> bool:
    if value is None:
        return False
    try:
        if pd.isna(value):
            return False
    except (TypeError, ValueError):
        pass
    return str(value).strip() != ""


def _extract_scaling_value_from_token(
    problem_size: str,
    token_keys: Sequence[str],
) -> float:
    """Extract a scaling value from a named problem-size token."""
    if not token_keys:
        return float("nan")
    exact_keys = set(token_keys)
    lower_keys = {key.lower() for key in token_keys}
    for token in str(problem_size).split("_"):
        match = _NS_TOKEN_RE.match(token)
        if not match:
            continue
        key, value = match.group(1), match.group(2)
        if key in exact_keys or key.lower() in lower_keys:
            return float(value)
    return float("nan")


def _problem_size_sort_key(
    value: str,
    params: Optional[dict] = None,
) -> Tuple[int, float, str]:
    numeric = _extract_scaling_value_from_token(
        str(value),
        _params_scaling_token_keys(params),
    )
    if numeric != numeric:  # NaN
        numeric = extract_scaling_value(str(value), _params_scaling_values(params))
    if numeric != numeric:  # NaN
        return (1, 0.0, str(value))
    return (0, numeric, str(value))


def _format_size_span(
    values: Sequence[str],
    params: Optional[dict] = None,
) -> str:
    unique = sorted(
        {str(v) for v in values},
        key=lambda value: _problem_size_sort_key(value, params),
    )
    if not unique:
        return "none"
    if len(unique) <= 4:
        return ", ".join(unique)
    return f"{unique[0]}–{unique[-1]}"


def _finishability_source_note(
    provenance: ReportProvenance,
    evidence_path: Path,
    source_label: Optional[str] = None,
) -> str:
    status = provenance.status or "unknown"
    conclusion = provenance.conclusion or "unknown"
    source_ref = source_label or f"`{evidence_path}`"
    return (
        f"Source: selected terminal run {provenance.run_id} "
        f"(head {provenance.head_sha}; status `{status}` / conclusion `{conclusion}`) "
        f"using {source_ref}. A size is counted as within the 7200s "
        "per-simulation invocation contract when its selected comparison row has "
        "a populated `sim_ms`; blank `sim_ms` rows are counted "
        "as not completed/no result for this run."
    )


def _render_finishability_omission_section() -> str:
    return "\n".join([
        "## Per-Size Finishability",
        "",
        "_Tier-1/Tier-2 per-size finishability tables are omitted because no "
        "selected terminal-run finishability evidence CSV was supplied. Pass "
        "`--finishability-evidence-csv` with selected run metadata, or "
        "`--require-finishability` to fail instead of omitting._",
    ])


def _emit_finishability_table(
    title: str,
    rows: List[Tuple[str, int, int, str, int]],
    source_note: str,
) -> str:
    lines = [
        title,
        "",
        "Problem-size completion under the CI workflow's 7200s "
        "per-simulation invocation contract.",
        "",
        source_note,
        "",
        "| Benchmark | Sizes within 7200s | Total sizes | Finishable size range | "
        "Missing/no-result sizes |",
        "| --- | ---: | ---: | --- | ---: |",
    ]
    if rows:
        for benchmark, finishable, total, span, missing in rows:
            lines.append(
                f"| {benchmark} | {finishable} | {total} | {span} | {missing} |"
            )
    else:
        lines.append("| _none_ | 0 | 0 | none | 0 |")
    return "\n".join(lines)


def build_finishability_summary(
    evidence_csv_path: str,
    tier1_set: frozenset,
    provenance: ReportProvenance,
    source_label: Optional[str] = None,
    repo_root: Path = REPO_ROOT,
) -> FinishabilitySummary:
    """Build Tier-1/Tier-2 finishability sections from selected CSV evidence."""
    evidence_path = Path(evidence_csv_path)
    _validate_finishability_source(provenance, evidence_path, repo_root)
    # Read/hash the evidence path once before pandas parses anything.  All
    # plan checks and rendered rows below come from this same immutable buffer.
    evidence_bytes, evidence_sha = _read_finishability_evidence_once(evidence_path)
    artifact_error = _finishability_selected_artifact_error(
        evidence_path,
        evidence_sha,
        repo_root,
    )
    df = _load_comparison_dataframe_from_bytes(evidence_bytes, str(evidence_path))
    _validate_finishability_regular_plan_rows(df, evidence_path, repo_root)
    if artifact_error is not None:
        raise artifact_error

    rows_by_tier: Dict[str, List[Tuple[str, int, int, str, int]]] = {
        "tier1": [],
        "tier2": [],
    }
    params_by_kernel: Dict[str, Optional[dict]] = {}
    for kernel in sorted(df["kernel_name"].dropna().astype(str).unique()):
        params_by_kernel[kernel] = load_benchmark_params(kernel, repo_root)
        kdf = df[df["kernel_name"].astype(str) == kernel].copy()
        by_size: Dict[str, bool] = {}
        for _, row in kdf.iterrows():
            size = str(row["problem_size"])
            by_size[size] = by_size.get(size, False) or _has_value(row.get("sim_ms"))
        total = len(by_size)
        finishable_sizes = [size for size, ok in by_size.items() if ok]
        finishable = len(finishable_sizes)
        missing = total - finishable
        tier = get_tier(kernel, tier1_set)
        span = _format_size_span(finishable_sizes, params_by_kernel[kernel])
        rows_by_tier[tier].append((kernel, finishable, total, span, missing))

    for tier_rows in rows_by_tier.values():
        tier_rows.sort(key=lambda item: (-item[1], item[0]))

    source_note = _finishability_source_note(provenance, evidence_path, source_label)
    return FinishabilitySummary(
        tier2_section=_emit_finishability_table(
            "## Tier-2 Per-Size Finishability",
            rows_by_tier["tier2"],
            source_note,
        ),
        tier1_section=_emit_finishability_table(
            "## Tier-1 Per-Size Finishability",
            rows_by_tier["tier1"],
            source_note,
        ),
        source_path=str(evidence_path),
        source_note=source_note,
    )


def _emit_metrics_table(
    lines: List[str],
    rows: List[Tuple[str, int, str, Dict[str, float]]],
    figures_rel_dir: str,
) -> None:
    """Append a per-benchmark metrics table to *lines*."""
    lines.append(
        "| Benchmark | Samples | Mean err | Max err"
        " | Mean sym err | Max sym err | Chart |"
    )
    lines.append("| --- | ---: | ---: | ---: | ---: | ---: | --- |")
    for label, n_samples, chart_name, metrics in rows:
        chart_link = f"[{chart_name}]({figures_rel_dir}/{chart_name})"
        lines.append(
            "| {k} | {n} | {me} | {xe} | {ms} | {xs} | {chart} |".format(
                k=label,
                n=n_samples,
                me=_format_metric(metrics["mean_err"]),
                xe=_format_metric(metrics["max_err"]),
                ms=_format_metric(metrics["mean_sym_err"]),
                xs=_format_metric(metrics["max_sym_err"]),
                chart=chart_link,
            )
        )
    lines.append("")


# Hand-maintained sections preserved across regeneration.  Finishability
# sections are intentionally excluded: they must be generated from explicit
# selected terminal-run evidence for the current report, never copied from a
# previously checked-in report.
PRESERVED_SECTIONS: Tuple[str, ...] = (
    "## Known Limitations",
)

ADJUST_WEIGHTS_LIMITATION_NOTE = (
    "### adjust_weights (backprop benchmark)\n\n"
    "`adjust_weights` is a short backprop sub-kernel where fixed launch or "
    "dispatch overhead can dominate total runtime. Historical calibration "
    "percentages for this kernel are run-specific and are not preserved across "
    "report regeneration. Use the generated per-benchmark metrics table above "
    "for the selected run's current sample count and error metrics."
)

_ADJUST_WEIGHTS_LIMITATION_RE = re.compile(
    r"^###\s+adjust_weights\b[^\n]*\n.*?(?=^###\s+|\Z)",
    re.IGNORECASE | re.MULTILINE | re.DOTALL,
)

_STALE_ADJUST_WEIGHTS_CLAIM_RE = re.compile(
    r"13\.51\s*%|\b5\s+samples?\b|meets\s+(?:the\s+)?<\s*20\s*%",
    re.IGNORECASE,
)


def _contains_stale_adjust_weights_claim(text: str) -> bool:
    return "adjust_weights" in text.lower() and bool(
        _STALE_ADJUST_WEIGHTS_CLAIM_RE.search(text)
    )


def sanitize_known_limitations_section(section: Optional[str]) -> Optional[str]:
    """Return a Known Limitations section safe to preserve in a fresh report.

    The section is hand-maintained, but historical ``adjust_weights`` narrative
    contained data-specific calibration claims that can contradict the selected
    run's generated metrics after a new report is built.  Normalize stale
    ``adjust_weights`` subsections to a data-agnostic note so stale sample
    counts, percentages, or pass/fail claims are not carried forward.
    """
    if not section:
        return section

    text = section.rstrip()
    changed = False

    def replace_stale_subsection(match: re.Match[str]) -> str:
        nonlocal changed
        subsection = match.group(0)
        if not _contains_stale_adjust_weights_claim(subsection):
            return subsection
        changed = True
        return ADJUST_WEIGHTS_LIMITATION_NOTE + "\n\n"

    normalized = _ADJUST_WEIGHTS_LIMITATION_RE.sub(replace_stale_subsection, text)
    if changed:
        return normalized.rstrip()

    if _contains_stale_adjust_weights_claim(text):
        paragraphs = re.split(r"\n\s*\n", text)
        kept = [
            paragraph
            for paragraph in paragraphs
            if not _contains_stale_adjust_weights_claim(paragraph)
        ]
        if kept and kept[0].startswith("## Known Limitations"):
            kept.insert(1, ADJUST_WEIGHTS_LIMITATION_NOTE)
            return "\n\n".join(kept).rstrip()
        return "## Known Limitations\n\n" + ADJUST_WEIGHTS_LIMITATION_NOTE

    return text


def extract_preserved_section(text: str, heading: str) -> Optional[str]:
    """Return the section starting at *heading* (heading included).

    The section runs from the heading line until the next ``## `` heading or
    end-of-text, with trailing whitespace stripped.  Returns ``None`` when the
    heading is not found.
    """
    if not text:
        return None
    pattern = re.compile(
        r"^" + re.escape(heading) + r"\b.*?(?=\n## |\Z)",
        re.MULTILINE | re.DOTALL,
    )
    match = pattern.search(text)
    if not match:
        return None
    return match.group(0).rstrip()


def extract_preserved_sections(text: str) -> Dict[str, str]:
    """Return a dict mapping preserved heading → captured section text."""
    if not text:
        return {}
    out: Dict[str, str] = {}
    for heading in PRESERVED_SECTIONS:
        section = extract_preserved_section(text, heading)
        if section is not None:
            out[heading] = section
    return out


def _append_section(lines: List[str], section: Optional[str]) -> None:
    """Append a preserved or generated section block followed by a blank separator."""
    if not section:
        return
    lines.append(section)
    lines.append("")


def _display_path(path: str | Path, repo_root: Path) -> str:
    """Return a stable report label for *path*, relative to *repo_root* when possible."""
    path_obj = Path(path)
    try:
        return path_obj.resolve().relative_to(Path(repo_root).resolve()).as_posix()
    except (OSError, ValueError):
        return str(path_obj)


def _build_report(
    per_kernel: List[Tuple[str, int, str, Dict[str, float], str]],
    tier2_metrics: Dict[str, float],
    tier2_samples: int,
    figures_rel_dir: str,
    source_csv_label: str,
    provenance: ReportProvenance,
    preserved: Optional[Dict[str, str]] = None,
    finishability: Optional[FinishabilitySummary] = None,
    time_calibrations: Optional[Mapping[str, TimeCalibration]] = None,
    cache_bandwidth_semantics: Optional[Mapping[str, Mapping[str, object]]] = None,
) -> str:
    """Render the accuracy report markdown.

    *per_kernel* entries are ``(label, samples, chart, metrics, tier)`` tuples
    where ``tier`` is ``'tier1'`` or ``'tier2'``.  Tier-1 entries are reported
    in a separate Calibration Set section and excluded from the global
    accuracy rollup, which covers tier-2 benchmarks only.

    *preserved* maps preserved section headings (see ``PRESERVED_SECTIONS``)
    to the verbatim block to re-emit.  Missing entries are silently skipped.
    """
    preserved = preserved or {}
    time_calibrations = time_calibrations or {}
    cache_bandwidth_semantics = cache_bandwidth_semantics or {}
    tier2_finishability = finishability.tier2_section if finishability else None
    tier1_finishability = finishability.tier1_section if finishability else None
    known_limitations = sanitize_known_limitations_section(
        preserved.get("## Known Limitations")
    )

    tier1_rows = [(l, n, c, m) for (l, n, c, m, t) in per_kernel if t == "tier1"]
    tier2_rows = [(l, n, c, m) for (l, n, c, m, t) in per_kernel if t == "tier2"]

    lines: List[str] = []
    lines.append("# Benchmark Accuracy Report\n")
    lines.append(
        "Comparison of MGPUSim simulated execution time against MI300A "
        "hardware measurements.\n"
    )
    lines.append(
        f"Source data: `{source_csv_label}` "
        "(only rows where both `real_ms` and `sim_ms` are populated).\n"
    )
    lines.append(_render_provenance_section(provenance))
    lines.append("")
    lines.append("Error definitions:\n")
    lines.append("- `err = |sim - hw| / hw`")
    lines.append("- `symmetric_err = |sim - hw| / min(sim, hw)`")
    lines.append(
        "- `sim` is raw `sim_ms` unless `scripts/benchmark_tiers.json` "
        "configures a report-time calibration."
    )
    lines.append(
        "- Calibrated metrics use the matching report-time affine model "
        "`sim = sim_scale * sim_ms + fixed_time_ms`; raw comparison rows "
        "remain unchanged."
    )
    lines.append("")

    calibration_section = _render_time_calibration_section(time_calibrations)
    _append_section(lines, calibration_section)
    _append_section(
        lines,
        _render_cache_bandwidth_semantics_section(cache_bandwidth_semantics),
    )

    if tier2_finishability:
        _append_section(lines, tier2_finishability)
    else:
        _append_section(lines, _render_finishability_omission_section())

    lines.append("## Tier-2 Accuracy (Global Summary)\n")
    lines.append(
        "Tier-1 calibration kernels are excluded from this rollup; see the "
        "Calibration Set section below.\n"
    )
    lines.append(f"- Matched tier-2 samples: **{tier2_samples}**")
    lines.append(f"- Tier-2 benchmarks compared: **{len(tier2_rows)}**")
    lines.append(f"- Mean err: **{_format_metric(tier2_metrics['mean_err'])}**")
    lines.append(f"- Max err: **{_format_metric(tier2_metrics['max_err'])}**")
    lines.append(
        f"- Mean symmetric err: **{_format_metric(tier2_metrics['mean_sym_err'])}**"
    )
    lines.append(
        f"- Max symmetric err: **{_format_metric(tier2_metrics['max_sym_err'])}**"
    )
    lines.append("")

    lines.append("## Calibration Set (Tier 1)\n")
    lines.append(
        "Per-benchmark errors for the tier-1 calibration set.  Reported for "
        "transparency and **not** included in the global accuracy rollup.\n"
    )
    if tier1_rows:
        _emit_metrics_table(lines, tier1_rows, figures_rel_dir)
    else:
        lines.append("_No tier-1 benchmarks present in this run._\n")

    _append_section(lines, tier1_finishability)

    lines.append("## Per-Benchmark Metrics (Tier 2)\n")
    if tier2_rows:
        _emit_metrics_table(lines, tier2_rows, figures_rel_dir)
    else:
        lines.append("_No tier-2 benchmarks present in this run._\n")

    _append_section(lines, known_limitations)

    text = "\n".join(lines)
    if not text.endswith("\n"):
        text += "\n"
    return text


# ---------------------------------------------------------------------------
# Public API
# ---------------------------------------------------------------------------


def generate_report(
    csv_path: str = str(CSV_PATH),
    figures_dir: str = str(FIGURES_DIR),
    report_path: str = str(REPORT_PATH),
    figures_rel_dir: str = FIGURES_REL_DIR,
    repo_root: Path = REPO_ROOT,
    run_metadata_path: Optional[str] = None,
    selected_run_id: Optional[str] = None,
    selected_head_sha: Optional[str] = None,
    selected_run_status: Optional[str] = None,
    selected_run_conclusion: Optional[str] = None,
    finishability_evidence_csv_path: Optional[str] = None,
    finishability_source_label: Optional[str] = None,
    require_finishability: bool = False,
    env: Optional[Mapping[str, str]] = None,
) -> Dict[str, object]:
    """Generate comparison charts and write the accuracy report.

    For each kernel in *csv_path* with at least one matched (hw + sim) row:

    * Loads ``workloads/<suite>/<bench>/params.json`` to identify the
      scaling parameter.  Uses it as the numeric x-axis for charts.
    * Emits one chart per unique combination of non-scaling parameter
      values observed as columns in the CSV.  When no such columns are
      present (the current default), one chart per kernel is produced.

    Returns a dict with:

    * ``per_kernel``: list of ``(kernel, n_samples, chart_name, metrics, tier)``
    * ``global``: global error metrics dict
    * ``charts``: list of generated chart filenames
    * ``report_path``: absolute path of the written report
    """
    figs_dir = Path(figures_dir)
    rpt_path = Path(report_path)

    env = os.environ if env is None else env
    tier1_set, time_calibrations = load_time_calibrations(repo_root)
    provenance = load_report_provenance(
        metadata_path=run_metadata_path,
        env=env,
        selected_run_id=selected_run_id,
        selected_head_sha=selected_head_sha,
        selected_status=selected_run_status,
        selected_conclusion=selected_run_conclusion,
    )
    finishability_path = finishability_evidence_csv_path or _first_env(
        env, _ENV_FINISHABILITY_PATH_KEYS
    )
    finishability = None
    if finishability_path:
        finishability = build_finishability_summary(
            finishability_path,
            tier1_set,
            provenance,
            source_label=finishability_source_label,
            repo_root=repo_root,
        )
    elif require_finishability:
        raise ValueError(
            "finishability evidence was required but no selected terminal-run "
            "CSV was supplied; set ACCURACY_FINISHABILITY_EVIDENCE_CSV or "
            "pass --finishability-evidence-csv"
        )
    full_df = _load_comparison_dataframe(str(csv_path))
    matched = _matched_dataframe_from_full(full_df)
    source_csv_label = _display_path(csv_path, repo_root)
    cache_bandwidth_semantics = evaluate_cache_bandwidth_report_semantics(
        full_df,
        source_csv_label,
        repo_root,
    )

    existing_text = ""
    if rpt_path.is_file():
        existing_text = rpt_path.read_text(encoding="utf-8")
    preserved = extract_preserved_sections(existing_text)

    if matched.empty:
        empty_metrics = compute_error_metrics([], [])
        report_text = _build_report(
            [],
            empty_metrics,
            0,
            figures_rel_dir,
            source_csv_label,
            provenance,
            preserved=preserved,
            finishability=finishability,
            time_calibrations=time_calibrations,
            cache_bandwidth_semantics=cache_bandwidth_semantics,
        )
        rpt_path.parent.mkdir(parents=True, exist_ok=True)
        rpt_path.write_text(report_text, encoding="utf-8")
        return {
            "per_kernel": [],
            "global": empty_metrics,
            "tier2": empty_metrics,
            "charts": [],
            "report_path": str(rpt_path),
            "provenance": provenance,
            "finishability": finishability,
            "time_calibrations": time_calibrations,
            "cache_bandwidth_semantics": cache_bandwidth_semantics,
        }

    kernels = sorted(matched["kernel_name"].unique())
    per_kernel: List[Tuple[str, int, str, Dict[str, float], str]] = []
    charts: List[str] = []
    tier2_real: List[float] = []
    tier2_adj_sim: List[float] = []

    for kernel in kernels:
        kdf = matched[matched["kernel_name"] == kernel].copy()
        params = load_benchmark_params(kernel, repo_root)
        kdf = _compute_nonscaling_groups(kdf, params)
        tier = get_tier(kernel, tier1_set)
        calibration = time_calibrations.get(kernel, IDENTITY_TIME_CALIBRATION)

        for combo, group_df in kdf.groupby("_nonscaling_group"):
            combo_tuple = tuple(combo) if combo else ()
            chart_name = _render_chart(
                kernel, group_df, figs_dir, params, combo_tuple, calibration
            )
            charts.append(chart_name)

            real_vals = group_df["real_ms"].tolist()
            adj_sim_vals = [
                calibration.adjusted_ms(row.sim_ms, row.problem_size)
                for row in group_df.itertuples(index=False)
            ]
            metrics = compute_error_metrics(real_vals, adj_sim_vals)

            label = kernel
            if combo_tuple:
                label = "{k} ({combo})".format(
                    k=kernel,
                    combo=", ".join(f"{k}={v}" for k, v in combo_tuple),
                )
            per_kernel.append((label, len(group_df), chart_name, metrics, tier))

            if tier == "tier2":
                tier2_real.extend(real_vals)
                tier2_adj_sim.extend(adj_sim_vals)

    tier2_metrics = compute_error_metrics(tier2_real, tier2_adj_sim)
    report_text = _build_report(
        per_kernel,
        tier2_metrics,
        len(tier2_real),
        figures_rel_dir,
        source_csv_label,
        provenance,
        preserved=preserved,
        finishability=finishability,
        time_calibrations=time_calibrations,
        cache_bandwidth_semantics=cache_bandwidth_semantics,
    )

    rpt_path.parent.mkdir(parents=True, exist_ok=True)
    rpt_path.write_text(report_text, encoding="utf-8")

    return {
        "per_kernel": per_kernel,
        "global": tier2_metrics,
        "tier2": tier2_metrics,
        "charts": charts,
        "report_path": str(rpt_path),
        "provenance": provenance,
        "finishability": finishability,
        "time_calibrations": time_calibrations,
        "cache_bandwidth_semantics": cache_bandwidth_semantics,
    }


def _parse_args(argv: Iterable[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--csv-path", default=str(CSV_PATH))
    parser.add_argument("--figures-dir", default=str(FIGURES_DIR))
    parser.add_argument("--report-path", default=str(REPORT_PATH))
    parser.add_argument("--figures-rel-dir", default=FIGURES_REL_DIR)
    parser.add_argument("--run-metadata-json", default=None)
    parser.add_argument("--selected-run-id", default=None)
    parser.add_argument("--selected-head-sha", default=None)
    parser.add_argument("--selected-run-status", default=None)
    parser.add_argument("--selected-run-conclusion", default=None)
    parser.add_argument("--finishability-evidence-csv", default=None)
    parser.add_argument("--finishability-source-label", default=None)
    parser.add_argument(
        "--require-finishability",
        action="store_true",
        help="fail if no selected terminal-run finishability evidence CSV is supplied",
    )
    return parser.parse_args(list(argv) if argv is not None else None)


def main(argv: Iterable[str] | None = None) -> int:
    args = _parse_args(argv)
    try:
        result = generate_report(
            csv_path=args.csv_path,
            figures_dir=args.figures_dir,
            report_path=args.report_path,
            figures_rel_dir=args.figures_rel_dir,
            run_metadata_path=args.run_metadata_json,
            selected_run_id=args.selected_run_id,
            selected_head_sha=args.selected_head_sha,
            selected_run_status=args.selected_run_status,
            selected_run_conclusion=args.selected_run_conclusion,
            finishability_evidence_csv_path=args.finishability_evidence_csv,
            finishability_source_label=args.finishability_source_label,
            require_finishability=args.require_finishability,
        )
    except ValueError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    print(
        f"Wrote {result['report_path']} "
        f"({len(result['charts'])} charts in {args.figures_dir})"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
