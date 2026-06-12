from __future__ import annotations

import csv
import json
import math
import re
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from typing import Iterable, Mapping, Optional, Tuple


REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPT = REPO_ROOT / "scripts" / "generate_mi300a_problem_size_discovery_plan.py"
COMPARISON = REPO_ROOT / "benchmark-comparison" / "comparison_ci.csv"
RAW_KERNEL_TIMINGS = REPO_ROOT / "workloads" / "results" / "kernel_timings_20260317-075319-odyssey.csv"
RAW_RESULTS = REPO_ROOT / "workloads" / "results" / "results_20260317-075319-odyssey.csv"
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "benchmark.yml"
CHECKED_IN_PLAN = REPO_ROOT / "benchmark-comparison" / "mi300a_problem_size_discovery_plan.json"
MERGE_SCRIPT = REPO_ROOT / "benchmark-comparison" / "merge_ci_results.py"
TMP_ROOT = REPO_ROOT / ".repo_tmp" / "mi300a_problem_size_discovery_plan_tests"
SAFE_ID_RE = re.compile(r"^[a-z0-9][a-z0-9-]*$")


class GenerateMI300AProblemSizeDiscoveryPlanTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        TMP_ROOT.mkdir(parents=True, exist_ok=True)

    def run_plan(
        self,
        comparison_path: Path,
        workflow_path: Path,
        *extra_args: str,
        raw_kernel_timings_path: Optional[Path] = None,
        raw_results_path: Optional[Path] = None,
    ) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [
                sys.executable,
                str(SCRIPT),
                "--comparison",
                str(comparison_path),
                "--raw-kernel-timings",
                str(raw_kernel_timings_path or RAW_KERNEL_TIMINGS),
                "--raw-results",
                str(raw_results_path or RAW_RESULTS),
                "--workflow",
                str(workflow_path),
                "--merge-script",
                str(MERGE_SCRIPT),
                *extra_args,
            ],
            cwd=REPO_ROOT,
            text=True,
            capture_output=True,
            check=False,
            timeout=30,
        )

    def test_generator_wording_matches_regular_workflow_default(self) -> None:
        source_text = SCRIPT.read_text(encoding="utf-8")
        self.assertIn("maintained linear regular-workflow-only path", source_text)
        self.assertNotIn("linear finishable-size path", source_text)
        self.assertNotIn("linear finishable-size workflow", source_text)

        help_result = subprocess.run(
            [sys.executable, str(SCRIPT), "--help"],
            cwd=REPO_ROOT,
            text=True,
            capture_output=True,
            check=False,
            timeout=30,
        )
        self.assertEqual(help_result.returncode, 0, msg=help_result.stderr)
        self.assertIn("maintained linear regular-workflow-only path", help_result.stdout)
        self.assertIn("maintained linear regular workflow", help_result.stdout)
        self.assertNotIn("linear finishable-size workflow", help_result.stdout)

    def test_real_plan_generation_maps_all_hardware_rows(self) -> None:
        result = self.run_plan(COMPARISON, WORKFLOW)

        self.assertEqual(result.returncode, 0, msg=result.stderr)
        plan = json.loads(result.stdout)
        self.assertEqual(plan["schema_name"], "mgpusim.mi300a_problem_size_discovery_plan")
        self.assertEqual(plan["schema_version"], 1)
        self.assertEqual(
            plan["source_paths"],
            {
                "comparison_csv": "benchmark-comparison/comparison_ci.csv",
                "raw_kernel_timings_csv": "workloads/results/kernel_timings_20260317-075319-odyssey.csv",
                "raw_results_csv": "workloads/results/results_20260317-075319-odyssey.csv",
                "workflow": ".github/workflows/benchmark.yml",
                "merge_mapping": "benchmark-comparison/merge_ci_results.py",
            },
        )
        self.assertEqual(plan["workflow_jobs"], ["benchmark-tier1", "benchmark-tier2"])
        workflow_text = WORKFLOW.read_text(encoding="utf-8")
        self.assertIn("Default path is intentionally linear and regular-workflow only", workflow_text)
        self.assertNotRegex(workflow_text, r"(?m)^\s+- benchmark:\s")
        self.assertNotIn("run_mode", workflow_text)
        self.assertEqual(
            plan["selection_policy"],
            "comparison_ci_rows_with_nonempty_real_ms_plus_missing_raw_kernel_timings_"
            "mapped_to_full_mode_workflow_template",
        )
        self.assertEqual(plan["timeout_sec"], 3600)
        self.assertEqual(plan["entry_count"], 1416)
        self.assertEqual(len(plan["entries"]), 1416)
        self.assertEqual(plan["hardware_row_count"], 1416)
        self.assertEqual(plan["hardware_kernel_count"], 85)
        self.assertEqual(plan["workflow_benchmark_count"], 82)
        self.assertEqual(plan["tier_counts"], {"1": 308, "2": 1108})
        self.assertEqual(
            plan["size_token_source_counts"],
            {"inferred_from_size_label_template": 166, "workflow_matrix_sizes": 1250},
        )
        self.assertEqual(plan["validation"]["unmapped_hardware_rows"], 0)
        self.assertEqual(plan["validation"]["missing_raw_measured_rows"], 0)

        first = plan["entries"][0]
        self.assertEqual(first["benchmark"], "mem_latency_chase")
        self.assertEqual(first["problem_size"], "256")
        self.assertEqual(first["workflow_benchmark"], "mem_latency_chase")
        self.assertEqual(first["sizes"], "256")
        self.assertEqual(first["timeout_sec"], 3600)

        inferred_atax = next(
            entry
            for entry in plan["entries"]
            if entry["benchmark"] == "atax" and entry["problem_size"] == "1280x1280"
        )
        self.assertEqual(inferred_atax["workflow_benchmark"], "atax")
        self.assertEqual(inferred_atax["sizes"], "1280")
        self.assertEqual(inferred_atax["size_token_source"], "inferred_from_size_label_template")
        self.assertEqual(inferred_atax["timeout_sec"], 3600)

        findminsad = next(entry for entry in plan["entries"] if entry["benchmark"] == "findminsad")
        self.assertEqual(findminsad["workflow_benchmark"], "parboil_sad")
        self.assertEqual(findminsad["kernel_to_workflow_mapping_source"], "plan_override")
        self.assertIn("secondary per-kernel", findminsad["kernel_to_workflow_mapping_reason"])

        hotspot_48 = next(
            entry
            for entry in plan["entries"]
            if entry["hardware_kernel_name"] == "hotspot" and entry["hardware_problem_size"] == "48"
        )
        self.assertEqual(hotspot_48["workflow_benchmark"], "hotspot")
        self.assertEqual(hotspot_48["sizes"], "48")
        self.assertEqual(hotspot_48["timeout_sec"], 3600)
        self.assertEqual(hotspot_48["hardware_csv_line"], 654)
        self.assertEqual(hotspot_48["size_token_source"], "inferred_from_size_label_template")

        coverage = plan["raw_mi300a_timing_coverage"]
        self.assertEqual(coverage["raw_kernel_timing_measured_row_count"], 1446)
        self.assertEqual(coverage["raw_unique_kernel_problem_size_count"], 1416)
        self.assertEqual(coverage["comparison_measured_row_count"], 1416)
        self.assertEqual(coverage["supplemental_raw_kernel_timing_rows_added"], 0)
        self.assertEqual(coverage["plan_represented_unique_kernel_problem_size_count"], 1416)
        self.assertEqual(coverage["missing_raw_measured_rows"], 0)
        self.assertEqual(coverage["skipped_exception_count"], 0)
        self.assertEqual(coverage["missing_result_success_for_same_benchmark_kernel_rows"], 0)
        self.assertEqual(plan["skipped_exception_records"], [])

    def test_checked_in_plan_is_deterministic(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            output_a = tmp_path / "plan_a.json"
            output_b = tmp_path / "plan_b.json"

            first = self.run_plan(COMPARISON, WORKFLOW, "--output", str(output_a))
            second = self.run_plan(COMPARISON, WORKFLOW, "--output", str(output_b))

            self.assertEqual(first.returncode, 0, msg=first.stderr)
            self.assertEqual(second.returncode, 0, msg=second.stderr)
            self.assertEqual(first.stdout, second.stdout)
            self.assertEqual(output_a.read_text(encoding="utf-8"), output_b.read_text(encoding="utf-8"))
            self.assertEqual(first.stdout, output_a.read_text(encoding="utf-8"))
            self.assertEqual(CHECKED_IN_PLAN.read_text(encoding="utf-8"), output_a.read_text(encoding="utf-8"))

    def test_checked_in_plan_reconciles_tracked_raw_mi300a_timings(self) -> None:
        plan = json.loads(CHECKED_IN_PLAN.read_text(encoding="utf-8"))
        comparison_lines = {}
        with COMPARISON.open(newline="", encoding="utf-8") as f:
            reader = csv.DictReader(f)
            for line_no, row in enumerate(reader, start=2):
                real_ms = (row.get("real_ms") or "").strip()
                if not real_ms:
                    continue
                key = (
                    (row.get("kernel_name") or "").strip().lower(),
                    (row.get("problem_size") or "").strip(),
                )
                comparison_lines.setdefault(key, line_no)

        raw_pairs = set()
        raw_lines = {}
        with RAW_KERNEL_TIMINGS.open(newline="", encoding="utf-8") as f:
            reader = csv.DictReader(f)
            for line_no, row in enumerate(reader, start=2):
                avg_ms = (row.get("avg_ms") or "").strip()
                if not avg_ms:
                    continue
                parsed = float(avg_ms)
                if not math.isfinite(parsed) or parsed < 0:
                    continue
                key = (
                    (row.get("kernel_name") or "").strip().lower(),
                    (row.get("problem_size") or "").strip(),
                )
                raw_pairs.add(key)
                raw_lines.setdefault(key, line_no)

        plan_pairs = {
            (entry["hardware_kernel_name"].lower(), entry["hardware_problem_size"])
            for entry in plan["entries"]
        }

        self.assertEqual(len(raw_pairs), 1416)
        self.assertEqual(raw_pairs - plan_pairs, set())
        self.assertIn(("hotspot", "48"), plan_pairs)
        hotspot_48 = next(
            entry
            for entry in plan["entries"]
            if entry["hardware_kernel_name"].lower() == "hotspot" and entry["hardware_problem_size"] == "48"
        )
        self.assertEqual(
            hotspot_48["hardware_csv_line"],
            comparison_lines.get(("hotspot", "48"), raw_lines[("hotspot", "48")]),
        )
        self.assertEqual(hotspot_48["sizes"], "48")
        self.assertEqual(hotspot_48["timeout_sec"], 3600)

        results_success_pairs = set()
        with RAW_RESULTS.open(newline="", encoding="utf-8") as f:
            reader = csv.DictReader(f)
            for row in reader:
                if (row.get("status") or "").strip().lower() != "success":
                    continue
                results_success_pairs.add(
                    ((row.get("benchmark") or "").strip().lower(), (row.get("problem_size") or "").strip())
                )
        self.assertIn(("hotspot", "48"), results_success_pairs)
        self.assertEqual(plan["raw_mi300a_timing_coverage"]["missing_raw_measured_rows"], 0)
        self.assertEqual(
            plan["raw_mi300a_timing_coverage"]["missing_result_success_for_same_benchmark_kernel_rows"],
            0,
        )

    def test_mapping_failures_and_duplicate_job_ids_are_reported(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            workflow_path = tmp_path / "workflow.yml"
            self.write_workflow(
                workflow_path,
                tier1_entries=[
                    self.workflow_entry(
                        "alpha", sizes="64", flag_template="-n {size}", size_label_template="{size}x{size}"
                    )
                ],
                tier2_entries=[],
            )

            missing_csv = tmp_path / "missing.csv"
            missing_rows = [("missing_kernel", "64", "0.100000")]
            self.write_comparison(missing_csv, missing_rows)
            missing_raw_kernel, missing_raw_results = self.write_raw_inputs(tmp_path, "missing", missing_rows)
            missing = self.run_plan(
                missing_csv,
                workflow_path,
                raw_kernel_timings_path=missing_raw_kernel,
                raw_results_path=missing_raw_results,
            )
            self.assertEqual(missing.returncode, 1)
            self.assertIn("problem-size discovery mapping failed", missing.stderr)
            self.assertIn("missing_kernel", missing.stderr)
            self.assertIn("absent from full-mode jobs", missing.stderr)

            bad_label_csv = tmp_path / "bad_label.csv"
            bad_label_rows = [("alpha", "64x128", "0.100000")]
            self.write_comparison(bad_label_csv, bad_label_rows)
            bad_label_raw_kernel, bad_label_raw_results = self.write_raw_inputs(tmp_path, "bad_label", bad_label_rows)
            bad_label = self.run_plan(
                bad_label_csv,
                workflow_path,
                raw_kernel_timings_path=bad_label_raw_kernel,
                raw_results_path=bad_label_raw_results,
            )
            self.assertEqual(bad_label.returncode, 1)
            self.assertIn("no runnable size token", bad_label.stderr)
            self.assertIn("does not match size_label_template", bad_label.stderr)

            duplicate_csv = tmp_path / "duplicate.csv"
            duplicate_rows = [
                ("alpha", "64x64", "0.100000"),
                ("alpha", "64x64", "0.100000"),
            ]
            self.write_comparison(duplicate_csv, duplicate_rows)
            duplicate_raw_kernel, duplicate_raw_results = self.write_raw_inputs(tmp_path, "duplicate", duplicate_rows)
            duplicate = self.run_plan(
                duplicate_csv,
                workflow_path,
                raw_kernel_timings_path=duplicate_raw_kernel,
                raw_results_path=duplicate_raw_results,
            )
            self.assertEqual(duplicate.returncode, 1)
            self.assertIn("problem-size discovery plan validation failed", duplicate.stderr)
            self.assertIn("duplicate job_id", duplicate.stderr)

    def test_entries_enforce_3600_timeout_and_single_size_token(self) -> None:
        plan = json.loads(CHECKED_IN_PLAN.read_text(encoding="utf-8"))
        job_ids = set()
        artifact_ids = set()
        entries = plan["entries"]
        self.assertEqual(plan["entry_count"], len(entries))

        for expected_index, entry in enumerate(entries, start=1):
            with self.subTest(plan_index=expected_index):
                self.assertEqual(entry["plan_index"], expected_index)
                self.assertEqual(entry["timeout_sec"], 3600)
                self.assertIsInstance(entry["sizes"], str)
                self.assertEqual(entry["sizes"], entry["size_token"])
                self.assertEqual(len(entry["sizes"].split()), 1)
                self.assertEqual(entry["size_label"], entry["problem_size"])
                self.assertRegex(entry["job_id"], SAFE_ID_RE)
                self.assertRegex(entry["artifact_id"], SAFE_ID_RE)
                self.assertNotIn(entry["job_id"], job_ids)
                self.assertNotIn(entry["artifact_id"], artifact_ids)
                self.assertIn(entry["tier"], {1, 2})
                self.assertIn(entry["workflow_job"], {"benchmark-tier1", "benchmark-tier2"})
                self.assertTrue(entry["flag_template"])
                self.assertTrue(entry["size_label_template"])
                job_ids.add(entry["job_id"])
                artifact_ids.add(entry["artifact_id"])

    def test_fixture_generation_can_infer_single_size_token_from_label_template(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            workflow_path = tmp_path / "workflow.yml"
            comparison_path = tmp_path / "comparison.csv"
            self.write_workflow(
                workflow_path,
                tier1_entries=[
                    self.workflow_entry(
                        "alpha", sizes="64", flag_template="-n {size}", size_label_template="{size}x{size}"
                    )
                ],
                tier2_entries=[],
            )
            fixture_rows = [("alpha", "128x128", "0.200000")]
            self.write_comparison(comparison_path, fixture_rows)
            raw_kernel_path, raw_results_path = self.write_raw_inputs(tmp_path, "alpha", fixture_rows)

            result = self.run_plan(
                comparison_path,
                workflow_path,
                raw_kernel_timings_path=raw_kernel_path,
                raw_results_path=raw_results_path,
            )

            self.assertEqual(result.returncode, 0, msg=result.stderr)
            plan = json.loads(result.stdout)
            self.assertEqual(plan["entry_count"], 1)
            entry = plan["entries"][0]
            self.assertEqual(entry["benchmark"], "alpha")
            self.assertEqual(entry["problem_size"], "128x128")
            self.assertEqual(entry["workflow_benchmark"], "alpha")
            self.assertEqual(entry["sizes"], "128")
            self.assertEqual(entry["size_token_source"], "inferred_from_size_label_template")
            self.assertEqual(entry["timeout_sec"], 3600)

    @staticmethod
    def workflow_entry(
        benchmark: str,
        *,
        tier: str = "tier1",
        sizes: str = "1",
        flag_template: str = "-n {size}",
        size_label_template: str = "{size}",
        extra: Optional[Mapping[str, str]] = None,
    ) -> Mapping[str, str]:
        entry = {
            "benchmark": benchmark,
            "tier": tier,
            "sizes": sizes,
            "flag_template": flag_template,
            "size_label_template": size_label_template,
        }
        if extra:
            entry.update(extra)
        return entry

    @staticmethod
    def write_workflow(
        path: Path,
        *,
        tier1_entries: Iterable[Mapping[str, str]],
        tier2_entries: Iterable[Mapping[str, str]],
    ) -> None:
        def render_entries(entries: Iterable[Mapping[str, str]]) -> str:
            lines = []
            for entry in entries:
                lines.append(f"          - benchmark: {entry['benchmark']}")
                for key in ("tier", "sizes", "flag_template", "size_label_template"):
                    lines.append(f"            {key}: \"{entry[key]}\"")
                for key, value in entry.items():
                    if key not in {"benchmark", "tier", "sizes", "flag_template", "size_label_template"}:
                        lines.append(f"            {key}: \"{value}\"")
            return "\n".join(lines)

        path.write_text(
            "\n".join(
                [
                    "jobs:",
                    "  benchmark-tier1:",
                    "    strategy:",
                    "      matrix:",
                    "        include:",
                    render_entries(tier1_entries),
                    "  benchmark-tier2:",
                    "    strategy:",
                    "      matrix:",
                    "        include:",
                    render_entries(tier2_entries),
                    "",
                ]
            ),
            encoding="utf-8",
        )

    @staticmethod
    def write_raw_inputs(
        tmp_path: Path,
        stem: str,
        rows: Iterable[Tuple[str, str, str]],
    ) -> Tuple[Path, Path]:
        materialized_rows = list(rows)
        kernel_path = tmp_path / f"{stem}_kernel_timings.csv"
        results_path = tmp_path / f"{stem}_results.csv"
        with kernel_path.open("w", encoding="utf-8", newline="") as f:
            f.write(
                "run_id,suite,benchmark,platform,problem_size,kernel_name,iterations,"
                "avg_ms,min_ms,max_ms,stddev_ms\n"
            )
            for kernel_name, problem_size, real_ms in materialized_rows:
                f.write(
                    "fixture,fixture,{benchmark},hip,{problem_size},{kernel_name},10,"
                    "{real_ms},{real_ms},{real_ms},0.0000\n".format(
                        benchmark=kernel_name,
                        problem_size=problem_size,
                        kernel_name=kernel_name,
                        real_ms=real_ms,
                    )
                )
        with results_path.open("w", encoding="utf-8", newline="") as f:
            f.write(
                "run_id,suite,benchmark,platform,problem_size,iterations,total_avg_ms,"
                "total_min_ms,total_max_ms,total_stddev_ms,status\n"
            )
            for kernel_name, problem_size, real_ms in materialized_rows:
                f.write(
                    "fixture,fixture,{benchmark},hip,{problem_size},10,"
                    "{real_ms},{real_ms},{real_ms},0.0000,success\n".format(
                        benchmark=kernel_name,
                        problem_size=problem_size,
                        real_ms=real_ms,
                    )
                )
        return kernel_path, results_path

    @staticmethod
    def write_comparison(path: Path, rows: Iterable[Tuple[str, str, str]]) -> None:
        with path.open("w", encoding="utf-8", newline="") as f:
            f.write("kernel_name,problem_size,real_ms,sim_ms,abs_error_ms,rel_error\n")
            for kernel_name, problem_size, real_ms in rows:
                f.write(f"{kernel_name},{problem_size},{real_ms},,,\n")


if __name__ == "__main__":
    unittest.main()
