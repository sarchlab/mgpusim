from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPT = REPO_ROOT / "scripts" / "generate_mi300a_curated_workflow_plan.py"
MANIFEST = REPO_ROOT / "benchmark-comparison" / "mi300a_evaluation_set.json"
CURRENT_LINEAR_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "benchmark.yml"
CHECKED_IN_PLAN = (
    REPO_ROOT / "benchmark-comparison" / "archive" / "mi300a_curated_workflow_plan.json"
)
TMP_ROOT = REPO_ROOT / ".repo_tmp" / "mi300a_curated_workflow_plan_tests"


class GenerateMI300ACuratedWorkflowPlanTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        TMP_ROOT.mkdir(parents=True, exist_ok=True)

    def run_plan(
        self,
        manifest_path: Path,
        workflow_path: Path,
        *extra_args: str,
    ) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [
                sys.executable,
                str(SCRIPT),
                "--manifest",
                str(manifest_path),
                "--workflow",
                str(workflow_path),
                *extra_args,
            ],
            cwd=REPO_ROOT,
            text=True,
            capture_output=True,
            check=False,
            timeout=30,
        )

    def test_current_linear_workflow_reemits_archived_plan_with_18_entries_and_tier_order(self) -> None:
        result = self.run_plan(MANIFEST, CURRENT_LINEAR_WORKFLOW)

        self.assertEqual(result.returncode, 0, msg=result.stderr)
        plan = json.loads(result.stdout)
        current_manifest = json.loads(MANIFEST.read_text(encoding="utf-8"))

        self.assertEqual(plan["schema_name"], "mgpusim.mi300a_curated_workflow_plan")
        self.assertEqual(plan["schema_version"], 1)
        self.assertEqual(
            plan["manifest"],
            {
                "schema_name": current_manifest["schema_name"],
                "schema_version": current_manifest["schema_version"],
                "set_id": current_manifest["set_id"],
                "set_version": current_manifest["set_version"],
            },
        )
        self.assertEqual(
            plan["source_paths"],
            {
                "manifest": "benchmark-comparison/mi300a_evaluation_set.json",
                "workflow": ".github/workflows/benchmark.yml",
            },
        )
        self.assertEqual(plan["workflow_jobs"], ["benchmark-tier1", "benchmark-tier2"])
        self.assertEqual(plan["ordering_policy"], "tier1_then_tier2_preserve_manifest_order")
        self.assertEqual(plan["entry_count"], 18)
        self.assertEqual(plan["tier_counts"], {"1": 6, "2": 12})

        expected_order = [
            "busspeeddownload",
            "busspeedreadback",
            "devicememory_write",
            "global_bw_copy",
            "l1_cache_bw",
            "l2_cache_bw",
            "bh",
            "ep",
            "fused_swiglu",
            "hist",
            "hotspot",
            "lud",
            "nn",
            "nw",
            "particlefilter",
            "qtc",
            "rope",
            "sgemm_tiled",
        ]
        entries = plan["benchmarks"]
        self.assertEqual([entry["manifest_kernel_name"] for entry in entries], expected_order)
        self.assertEqual([entry["tier"] for entry in entries[:6]], [1] * 6)
        self.assertEqual([entry["tier"] for entry in entries[6:]], [2] * 12)

        for plan_index, entry in enumerate(entries, start=1):
            self.assertEqual(entry["plan_index"], plan_index)
            self.assertIsInstance(entry["manifest_index"], int)
            self.assertIn(entry["workflow_job"], {"benchmark-tier1", "benchmark-tier2"})
            self.assertIsInstance(entry["workflow_line"], int)
            self.assertIn(entry["tier"], {1, 2})
            self.assertIsInstance(entry["category"], str)
            self.assertTrue(entry["category"])
            self.assertIsInstance(entry["sizes"], list)
            self.assertGreater(len(entry["sizes"]), 0)
            self.assertEqual(entry["size_count"], len(entry["sizes"]))
            self.assertIn("{size}", entry["flag_template"])
            self.assertIsInstance(entry["size_label_template"], str)
            if "timeout_sec" in entry:
                self.assertIsInstance(entry["timeout_sec"], int)
                self.assertGreater(entry["timeout_sec"], 0)

    def test_aliases_are_explicit_for_manifest_workflow_name_differences(self) -> None:
        result = self.run_plan(MANIFEST, CURRENT_LINEAR_WORKFLOW)
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        plan = json.loads(result.stdout)

        aliases = {entry["manifest_kernel_name"]: entry for entry in plan["aliases"]}
        expected = {
            "busspeeddownload": "bus_speed_download",
            "busspeedreadback": "bus_speed_readback",
            "devicememory_write": "device_memory_write",
            "sgemm_tiled": "parboil_sgemm",
        }
        self.assertEqual(set(aliases), set(expected))
        self.assertEqual(plan["alias_count"], len(expected))
        for manifest_kernel, workflow_benchmark in expected.items():
            with self.subTest(manifest_kernel=manifest_kernel):
                self.assertEqual(aliases[manifest_kernel]["workflow_benchmark_name"], workflow_benchmark)
                self.assertIn(manifest_kernel, aliases[manifest_kernel]["alias_reason"])
                self.assertTrue(aliases[manifest_kernel]["alias_reason"])

                plan_entry = next(
                    entry
                    for entry in plan["benchmarks"]
                    if entry["manifest_kernel_name"] == manifest_kernel
                )
                self.assertEqual(plan_entry["workflow_benchmark_name"], workflow_benchmark)
                self.assertEqual(plan_entry["alias_reason"], aliases[manifest_kernel]["alias_reason"])

        identity_entry = next(
            entry for entry in plan["benchmarks"] if entry["manifest_kernel_name"] == "global_bw_copy"
        )
        self.assertEqual(identity_entry["workflow_benchmark_name"], "global_bw_copy")
        self.assertIsNone(identity_entry["alias_reason"])

    def test_optional_runtime_fields_are_preserved_for_curated_dispatch(self) -> None:
        result = self.run_plan(MANIFEST, CURRENT_LINEAR_WORKFLOW)
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        plan = json.loads(result.stdout)
        entries = {entry["workflow_benchmark_name"]: entry for entry in plan["benchmarks"]}

        for benchmark in ("bh", "particlefilter"):
            with self.subTest(benchmark=benchmark):
                self.assertEqual(entries[benchmark]["extra_flags"], "-magic-memory-copy")
                self.assertEqual(entries[benchmark]["gomemlimit"], "6GiB")
                self.assertEqual(entries[benchmark]["gogc"], "off")

        self.assertEqual(entries["rope"]["gomemlimit"], "6GiB")
        self.assertEqual(entries["rope"]["gogc"], "off")
        self.assertNotIn("extra_flags", entries["rope"])
        self.assertEqual(entries["l1_cache_bw"]["timeout_sec"], 7200)
        self.assertEqual(entries["hotspot"]["timeout_sec"], 7200)
        self.assertEqual(entries["nw"]["timeout_sec"], 3600)
        self.assertNotIn("extra_flags", entries["global_bw_copy"])
        self.assertNotIn("gomemlimit", entries["global_bw_copy"])
        self.assertNotIn("gogc", entries["global_bw_copy"])

    def test_archived_fallback_json_output_is_deterministic_and_matches_checked_in_plan(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)
            output_a = tmp_path / "plan_a.json"
            output_b = tmp_path / "plan_b.json"

            first = self.run_plan(MANIFEST, CURRENT_LINEAR_WORKFLOW, "--output", str(output_a))
            second = self.run_plan(MANIFEST, CURRENT_LINEAR_WORKFLOW, "--output", str(output_b))

            self.assertEqual(first.returncode, 0, msg=first.stderr)
            self.assertEqual(second.returncode, 0, msg=second.stderr)
            self.assertEqual(first.stdout, second.stdout)
            self.assertEqual(output_a.read_text(encoding="utf-8"), output_b.read_text(encoding="utf-8"))
            self.assertEqual(first.stdout, output_a.read_text(encoding="utf-8"))
            self.assertEqual(CHECKED_IN_PLAN.read_text(encoding="utf-8"), output_a.read_text(encoding="utf-8"))

    def test_negative_validation_failures_are_reported(self) -> None:
        with tempfile.TemporaryDirectory(dir=TMP_ROOT) as tmpdir:
            tmp_path = Path(tmpdir)

            workflow_path = tmp_path / "workflow.yml"
            self.write_workflow(
                workflow_path,
                tier1_entries=[self.workflow_entry("alpha_probe", tier="tier1")],
                tier2_entries=[self.workflow_entry("beta_kernel", tier="tier2")],
            )

            missing_manifest = tmp_path / "missing_manifest.json"
            self.write_manifest(
                missing_manifest,
                [
                    self.manifest_entry("alpha_probe", tier=1),
                    self.manifest_entry("missing_kernel", tier=2),
                ],
            )
            missing = self.run_plan(missing_manifest, workflow_path)
            self.assertEqual(missing.returncode, 1)
            self.assertIn("missing_kernel", missing.stderr)
            self.assertIn("no supported alias", missing.stderr)

            duplicate_manifest = tmp_path / "duplicate_manifest.json"
            self.write_manifest(
                duplicate_manifest,
                [
                    self.manifest_entry("alpha_probe", tier=1),
                    self.manifest_entry("alpha_probe", tier=1),
                ],
            )
            duplicate = self.run_plan(duplicate_manifest, workflow_path)
            self.assertEqual(duplicate.returncode, 1)
            self.assertIn("duplicate selected kernel_name", duplicate.stderr)

            unsupported_alias_manifest = tmp_path / "unsupported_alias_manifest.json"
            unsupported_alias_workflow = tmp_path / "unsupported_alias_workflow.yml"
            self.write_manifest(
                unsupported_alias_manifest,
                [self.manifest_entry("foobar", tier=1)],
            )
            self.write_workflow(
                unsupported_alias_workflow,
                tier1_entries=[self.workflow_entry("foo_bar", tier="tier1")],
                tier2_entries=[],
            )
            unsupported_alias = self.run_plan(
                unsupported_alias_manifest,
                unsupported_alias_workflow,
            )
            self.assertEqual(unsupported_alias.returncode, 1)
            self.assertIn("normalized workflow candidate", unsupported_alias.stderr)
            self.assertIn("explicit supported alias", unsupported_alias.stderr)

            tier_mismatch_manifest = tmp_path / "tier_mismatch_manifest.json"
            self.write_manifest(
                tier_mismatch_manifest,
                [self.manifest_entry("alpha_probe", tier=2)],
            )
            tier_mismatch = self.run_plan(tier_mismatch_manifest, workflow_path)
            self.assertEqual(tier_mismatch.returncode, 1)
            self.assertIn("tier mismatch", tier_mismatch.stderr)

            duplicate_workflow = tmp_path / "duplicate_workflow.yml"
            self.write_workflow(
                duplicate_workflow,
                tier1_entries=[
                    self.workflow_entry("alpha_probe", tier="tier1"),
                    self.workflow_entry("alpha_probe", tier="tier1", sizes="3 4"),
                ],
                tier2_entries=[],
            )
            duplicate_workflow_manifest = tier_mismatch_manifest.with_name(
                "duplicate_workflow_manifest.json"
            )
            self.write_manifest(
                duplicate_workflow_manifest,
                [self.manifest_entry("alpha_probe", tier=1)],
            )
            duplicate_workflow_result = self.run_plan(
                duplicate_workflow_manifest,
                duplicate_workflow,
            )
            self.assertEqual(duplicate_workflow_result.returncode, 1)
            self.assertIn("duplicate workflow benchmark", duplicate_workflow_result.stderr)

    def manifest_entry(self, kernel_name: str, *, tier: int, category: str | None = None) -> dict:
        return {
            "kernel_name": kernel_name,
            "tier": tier,
            "category": category or f"tier{tier}_fixture",
        }

    def workflow_entry(
        self,
        benchmark: str,
        *,
        tier: str,
        sizes: str = "1 2",
        flag_template: str = "-n {size}",
        size_label_template: str = "{size}",
        timeout_sec: int | None = None,
    ) -> dict:
        entry = {
            "benchmark": benchmark,
            "tier": tier,
            "sizes": sizes,
            "flag_template": flag_template,
            "size_label_template": size_label_template,
        }
        if timeout_sec is not None:
            entry["timeout_sec"] = timeout_sec
        return entry

    def write_manifest(self, path: Path, entries: list[dict]) -> None:
        manifest = {
            "schema_name": "mgpusim.mi300a_evaluation_set",
            "schema_version": 1,
            "set_id": "fixture-curated-primary",
            "set_version": "unit-test",
            "benchmarks": entries,
        }
        path.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")

    def write_workflow(
        self,
        path: Path,
        *,
        tier1_entries: list[dict],
        tier2_entries: list[dict],
    ) -> None:
        def render_entries(entries: list[dict]) -> str:
            lines: list[str] = []
            for entry in entries:
                lines.append(f"          - benchmark: {entry['benchmark']}")
                lines.append(f"            tier: {entry['tier']}")
                lines.append(f"            sizes: \"{entry['sizes']}\"")
                lines.append(f"            flag_template: \"{entry['flag_template']}\"")
                lines.append(f"            size_label_template: \"{entry['size_label_template']}\"")
                if "timeout_sec" in entry:
                    lines.append(f"            timeout_sec: {entry['timeout_sec']}")
            return "\n".join(lines)

        lines = [
            "name: Fixture Benchmark",
            "jobs:",
            "  benchmark-tier1:",
            "    strategy:",
            "      matrix:",
            "        include:",
        ]
        lines.extend(render_entries(tier1_entries).splitlines())
        lines.extend(
            [
                "  benchmark-tier2:",
                "    strategy:",
                "      matrix:",
                "        include:",
            ]
        )
        lines.extend(render_entries(tier2_entries).splitlines())
        lines.extend(["  compare:", "    runs-on: self-hosted"])
        path.write_text("\n".join(lines) + "\n", encoding="utf-8")


if __name__ == "__main__":
    unittest.main()
