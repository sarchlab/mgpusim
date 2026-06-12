from __future__ import annotations

import json
import re
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOWS_DIR = REPO_ROOT / ".github" / "workflows"
RETIRED_WORKFLOW_PATH = WORKFLOWS_DIR / "mi300a_terminal_discovery.yml"
BENCHMARK_WORKFLOW_PATH = WORKFLOWS_DIR / "benchmark.yml"
README_PATH = REPO_ROOT / "benchmark-comparison" / "README.md"
OPERATOR_DOC_PATH = REPO_ROOT / "docs" / "mi300a_terminal_discovery_operator.md"
RECOVERY_OPERATOR_DOC_PATH = REPO_ROOT / "docs" / "terminal_recovery_operator_preflight.md"
RECOVERY_OPERATOR_PATH = REPO_ROOT / "scripts" / "mi300a_terminal_recovery_operator.py"
SHARD_MANIFEST_PATH = (
    REPO_ROOT
    / "benchmark-comparison"
    / "generated"
    / "mi300a_terminal_discovery_shard_manifest.json"
)
SHARD_MATRIX_DIR = REPO_ROOT / "benchmark-comparison" / "generated"


class MI300ATerminalDiscoveryWorkflowRetirementTest(unittest.TestCase):
    def setUp(self) -> None:
        self.benchmark_workflow_text = BENCHMARK_WORKFLOW_PATH.read_text(encoding="utf-8")
        self.workflow_files = sorted(WORKFLOWS_DIR.glob("*.y*ml"))
        self.workflow_text_by_name = {
            path.name: path.read_text(encoding="utf-8") for path in self.workflow_files
        }

    def test_visible_terminal_discovery_workflow_dispatch_surface_is_absent(self) -> None:
        self.assertFalse(
            RETIRED_WORKFLOW_PATH.exists(),
            msg="Retired terminal-discovery workflow must not remain in the visible Actions surface.",
        )
        self.assertNotIn("mi300a_terminal_discovery.yml", {path.name for path in self.workflow_files})

        forbidden_visible_workflow_snippets = [
            "name: MI300A Terminal Discovery",
            "validate-terminal-discovery",
            "terminal-discovery-shard-",
            "collect-terminal-discovery",
            "mi300a-terminal-discovery-plan-",
            "mi300a_terminal_discovery_shard_manifest.json",
            "validate_mi300a_terminal_discovery_shards.py",
            "collect_mi300a_terminal_discovery_provenance.py",
        ]
        for workflow_name, workflow_text in self.workflow_text_by_name.items():
            for snippet in forbidden_visible_workflow_snippets:
                self.assertNotIn(snippet, workflow_text, msg=f"{snippet!r} leaked into {workflow_name}")

    def test_benchmark_yml_is_only_maintained_mi300a_benchmark_entry_point(self) -> None:
        self.assertIn("name: MI300A Benchmark", self.benchmark_workflow_text)
        self.assertEqual(
            self._job_order(self.benchmark_workflow_text),
            ["validation", "benchmark-tier1", "tier1-summary", "benchmark-tier2", "summary"],
        )

        dispatch = self._extract_workflow_dispatch_block(self.benchmark_workflow_text)
        input_names = re.findall(r"^      ([A-Za-z0-9_-]+):\n", dispatch, flags=re.MULTILINE)
        self.assertEqual(input_names, ["ref"])
        self.assertNotIn("run_mode", dispatch)
        self.assertNotIn("github.event.inputs.run_mode", self.benchmark_workflow_text)

        for workflow_name, workflow_text in self.workflow_text_by_name.items():
            if workflow_name == "benchmark.yml":
                continue
            self.assertNotIn("MI300A Terminal Discovery", workflow_text)
            if "workflow_dispatch:" in workflow_text:
                self.assertNotRegex(
                    workflow_text,
                    r"(?i)mi300a.*benchmark|benchmark.*mi300a|terminal.*discovery|discovery.*terminal",
                    msg=f"{workflow_name} must not be a manual MI300A benchmark/discovery entry point",
                )

    def test_docs_describe_retired_dispatch_surface_and_approval_gate(self) -> None:
        readme = README_PATH.read_text(encoding="utf-8")
        operator_doc = OPERATOR_DOC_PATH.read_text(encoding="utf-8")

        for snippet in [
            "the visible `.github/workflows/mi300a_terminal_discovery.yml` Actions workflow is retired",
            "`.github/workflows/benchmark.yml` is the only maintained MI300A benchmark Actions entry point",
            "Validation -> Tier 1 -> Tier-1 summary -> Tier 2 -> Summary",
            "only the optional `ref` input",
            "do **not** approve, add, reintroduce, or dispatch terminal discovery",
            "recorded #181 approval plus reviewed-operator-path requirement",
            "do **not** resolve the finishability evidence",
        ]:
            self.assertIn(snippet, readme)

        for snippet in [
            "# MI300A Terminal Discovery Readiness Guide",
            "There is currently no visible `.github/workflows/mi300a_terminal_discovery.yml` workflow_dispatch surface to run.",
            "`.github/workflows/benchmark.yml` is the only maintained MI300A benchmark Actions entry point",
            "The next 3600-second terminal-discovery attempt has recorded human approval (#181)",
            "finishability evidence remains unresolved",
            "Do not dispatch, cancel, rerun, or reintroduce a terminal-discovery workflow from this document.",
        ]:
            self.assertIn(snippet, operator_doc)

        retired_dispatch_phrases = [
            "Workflow: `.github/workflows/mi300a_terminal_discovery.yml` (`MI300A Terminal Discovery`).",
            "Trigger: manual `workflow_dispatch` only.",
            "Runs to launch: one workflow run.",
            "Before dispatch",
        ]
        for snippet in retired_dispatch_phrases:
            self.assertNotIn(snippet, operator_doc)

    def test_docs_pin_m25_timeout_and_recovery_reproducibility_contract(self) -> None:
        readme = README_PATH.read_text(encoding="utf-8")
        operator_doc = OPERATOR_DOC_PATH.read_text(encoding="utf-8")

        shared_required_snippets = [
            "python3 scripts/validate_mi300a_terminal_discovery_timeout_contract.py",
            "--row-timeout-minutes <planned-row-timeout-minutes>",
            "--per-attempt-timeout-sec <planned-attempt-timeout-sec>",
            "--row-overhead-sec <planned-checkout-build-upload-overhead-sec>",
            "attempt_count * per_attempt_timeout_sec + row_overhead_sec",
            "python3 scripts/generate_mi300a_terminal_recovery_manifest.py --check",
            "25111815292",
            "940",
            "476 observed records",
        ]
        for document in [readme, operator_doc]:
            normalized_document = " ".join(document.split())
            for snippet in shared_required_snippets:
                self.assertIn(snippet, normalized_document)
            for forbidden in ["claim finishability evidence complete", "claim per-row-timeout evidence complete"]:
                self.assertNotIn(forbidden, normalized_document)

        normalized_readme = " ".join(readme.split())
        normalized_operator_doc = " ".join(operator_doc.split())
        self.assertIn("without rewriting it", normalized_readme)
        self.assertIn("without rewriting it", normalized_operator_doc)
        self.assertIn("78 workflow-benchmark recovery rows", normalized_readme)
        self.assertIn("byte-for-byte", normalized_operator_doc)
        self.assertIn(
            "does **not** approve, add, reintroduce, or dispatch terminal discovery",
            normalized_readme,
        )
        self.assertIn(
            "do not launch simulations, contact GitHub, dispatch Actions, collect new evidence, "
            "regenerate the finishable manifest, regenerate Tier views, or resolve finishability evidence",
            normalized_operator_doc,
        )

    def test_permanent_actions_surface_inventory_remains_unchanged(self) -> None:
        self.assertEqual(
            [path.name for path in self.workflow_files],
            ["benchmark.yml", "compile_hsaco.yml", "mgpusim_test.yml"],
        )
        maintained_surface = "\n".join(self.workflow_text_by_name.values())
        m28_recovery_snippets = [
            "mi300a_terminal_recovery_operator.py",
            "generate_mi300a_terminal_recovery_manifest.py",
            "generate_mi300a_terminal_recovery_dry_run_plan.py",
            "validate_mi300a_terminal_recovery_dry_run_plan.py",
            "mi300a_terminal_discovery_run_25111815292_recovery",
            "recovery_batch",
            "regenerate-provenance",
            "regenerate-finishable",
            "regenerate-tier",
        ]
        for snippet in m28_recovery_snippets:
            self.assertNotIn(snippet, maintained_surface)

        workflow_names = {
            workflow_name: self._workflow_display_name(workflow_text)
            for workflow_name, workflow_text in self.workflow_text_by_name.items()
        }
        self.assertEqual(
            workflow_names,
            {
                "benchmark.yml": "MI300A Benchmark",
                "compile_hsaco.yml": "Compile HSACO from kernel.hip files",
                "mgpusim_test.yml": "MGPUSim Test",
            },
        )

        dispatch_workflows = sorted(
            workflow_name
            for workflow_name, workflow_text in self.workflow_text_by_name.items()
            if "workflow_dispatch:" in workflow_text
        )
        self.assertEqual(dispatch_workflows, ["benchmark.yml", "compile_hsaco.yml"])
        for workflow_name in dispatch_workflows:
            dispatch = self._extract_workflow_dispatch_block(
                self.workflow_text_by_name[workflow_name]
            )
            input_names = re.findall(
                r"^      ([A-Za-z0-9_-]+):\n",
                dispatch,
                flags=re.MULTILINE,
            )
            self.assertEqual(
                input_names,
                ["ref"],
                msg=f"unexpected dispatch inputs in {workflow_name}",
            )
            self.assertNotIn("run_mode", dispatch)

        benchmark_jobs = self._job_order(self.benchmark_workflow_text)
        self.assertEqual(
            benchmark_jobs,
            ["validation", "benchmark-tier1", "tier1-summary", "benchmark-tier2", "summary"],
        )
        self.assertEqual(
            self._job_order(self.workflow_text_by_name["compile_hsaco.yml"]),
            ["compile"],
        )
        self.assertEqual(
            self._job_order(self.workflow_text_by_name["mgpusim_test.yml"]),
            [
                "unit_test",
                "deterministicity_test",
                "disassembler_test",
                "cdna3_emu_test",
                "cdna3_timing_test",
                "python_test",
            ],
        )

    def test_m28_operator_docs_pin_guardrails_without_actions_surface_change(self) -> None:
        operator_doc = RECOVERY_OPERATOR_DOC_PATH.read_text(encoding="utf-8")
        normalized = " ".join(operator_doc.split())

        required_snippets = [
            "scripts/mi300a_terminal_recovery_operator.py",
            "The terminal recovery operator maintains the read-only operator preflight/dry-run CLI",
            "Expected dry-run plan SHA-256",
            "Expected recovery manifest SHA-256",
            "branch-only and repository-local",
            "does not contact GitHub, dispatch/cancel/rerun Actions, run simulations",
            "regenerate provenance/finishable/Tier artifacts, or modify `.github/workflows`",
            "all 12 checked-in recovery batches",
            "940 plan indices",
            "476 prior observed entries",
            "incomplete_prior_evidence_only",
            "exact 1416-record provenance document",
            "max_parallel=14",
            "one attempt per matrix row",
            "per_attempt_timeout_sec=600",
            "row_timeout_sec=600",
            "action_timeout_sec=3600",
            "e31-m28-1-repair-recovery-operator-provenance-guard",
            "actual current branch and `HEAD` SHA",
            "optional `--expected-head-sha`",
            "`<run_id>` / `<issue_id>` / `<pr_id>`",
            "The CLI refuses",
            "stale or regenerated plan/manifest SHA-256 mismatches",
            "duplicate batch selections",
            "out-of-scope batch selections outside `1-12`",
            "Only after that validation may a separate reviewed decision determine",
        ]
        for snippet in required_snippets:
            self.assertIn(snippet, normalized)

        self.assertNotIn("gh workflow run", normalized)
        self.assertIn("Do not issue `gh run cancel`, `gh run rerun`, or another dispatch", normalized)
        self.assertIn("do not add a `run_mode` selector", normalized)
        self.assertIn("do not claim finishability evidence complete from recovery batches alone", normalized)
        self.assertNotRegex(normalized, r"\b[a-z]{3}-pr\s+1460\b")
        self.assertNotIn("issue 1462", normalized)
        self.assertNotIn("e31-m28-guarded-recovery-operator-path", normalized)

    def test_offline_terminal_discovery_shards_still_cover_1416_entries(self) -> None:
        manifest = json.loads(SHARD_MANIFEST_PATH.read_text(encoding="utf-8"))
        self.assertEqual(manifest["schema_name"], "mgpusim.mi300a_terminal_discovery_benchmark_shard_manifest")
        self.assertEqual(manifest["entry_count"], 1416)
        self.assertEqual(manifest["attempt_count"], 1416)
        self.assertEqual(manifest["benchmark_count"], 82)
        self.assertEqual(manifest["visible_matrix_row_count"], 82)
        self.assertEqual(manifest["shard_count"], 6)
        self.assertEqual(manifest["timeout_sec"], 3600)

        all_rows: list[dict[str, object]] = []
        all_attempts: list[dict[str, object]] = []
        for shard in manifest["shards"]:
            shard_index = int(shard["shard_index"])
            matrix_path = SHARD_MATRIX_DIR / f"mi300a_terminal_discovery_shard_{shard_index:02d}_matrix.json"
            matrix = json.loads(matrix_path.read_text(encoding="utf-8"))
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
                for key in ["matrix_row_id", "benchmark", "workflow_benchmark", "flag_template", "attempts"]:
                    self.assertIn(key, row)
                for attempt in row["attempts"]:
                    self.assertEqual(attempt["shard_index"], shard_index)
                    self.assertEqual(attempt["timeout_sec"], 3600)
                    self.assertEqual(len(str(attempt["sizes"]).split()), 1)
                    for key in ["plan_index", "benchmark", "hardware_kernel_name", "problem_size", "flag_template"]:
                        self.assertIn(key, attempt)
                all_attempts.extend(row["attempts"])
            all_rows.extend(matrix["include"])

        self.assertEqual(len(all_rows), 82)
        # Workflow benchmarks need not be physically contiguous in the source
        # plan (backprop_adjust_weights interleaved with backprop),
        # so attempts are emitted per benchmark in plan_index order; the
        # union across rows is therefore checked as a set.
        all_plan_indices = [attempt["plan_index"] for attempt in all_attempts]
        self.assertEqual(sorted(all_plan_indices), list(range(1, 1417)))
        hotspot_attempts = [a for a in all_attempts if a["plan_index"] == 474]
        self.assertEqual(len(hotspot_attempts), 1)
        hotspot = hotspot_attempts[0]
        self.assertEqual(hotspot["plan_index"], 474)
        self.assertEqual(hotspot["shard_index"], 2)
        self.assertEqual(hotspot["benchmark"], "hotspot")
        self.assertEqual(hotspot["hardware_kernel_name"], "hotspot")
        self.assertEqual(hotspot["problem_size"], "48")
        self.assertEqual(hotspot["sizes"], "48")

    def test_offline_terminal_provenance_tooling_remains_available_but_not_visible_actions(self) -> None:
        offline_tool_paths = [
            REPO_ROOT / "scripts" / "generate_mi300a_terminal_discovery_shards.py",
            REPO_ROOT / "scripts" / "validate_mi300a_terminal_discovery_shards.py",
            REPO_ROOT / "scripts" / "validate_mi300a_terminal_discovery_timeout_contract.py",
            REPO_ROOT / "scripts" / "collect_mi300a_terminal_discovery_provenance.py",
            REPO_ROOT / "scripts" / "validate_mi300a_terminal_discovery_provenance.py",
            REPO_ROOT / "scripts" / "generate_mi300a_terminal_recovery_manifest.py",
            REPO_ROOT / "scripts" / "generate_mi300a_terminal_recovery_dry_run_plan.py",
            REPO_ROOT / "scripts" / "validate_mi300a_terminal_recovery_dry_run_plan.py",
            RECOVERY_OPERATOR_PATH,
        ]
        for path in offline_tool_paths:
            self.assertTrue(path.exists(), msg=f"offline terminal-discovery tool missing: {path}")
            self.assertTrue(path.read_text(encoding="utf-8").startswith("#!/usr/bin/env python3"))

        visible_workflow_corpus = "\n".join(self.workflow_text_by_name.values())
        for path in offline_tool_paths:
            self.assertNotIn(f"python3 {path.relative_to(REPO_ROOT)}", visible_workflow_corpus)

    def _workflow_display_name(self, workflow_text: str) -> str:
        match = re.search(r"^name: (.+)$", workflow_text, flags=re.MULTILINE)
        self.assertIsNotNone(match, msg="workflow name missing")
        return match.group(1)

    def _job_order(self, workflow_text: str) -> list[str]:
        jobs_text = workflow_text.split("\njobs:\n", 1)[1]
        return re.findall(r"^  ([A-Za-z0-9_-]+):\n", jobs_text, flags=re.MULTILINE)

    def _extract_workflow_dispatch_block(self, workflow_text: str) -> str:
        pattern = re.compile(r"^  workflow_dispatch:\n(?P<body>.*?)(?=^jobs:)", re.MULTILINE | re.DOTALL)
        match = pattern.search(workflow_text)
        self.assertIsNotNone(match, msg="workflow_dispatch block missing")
        return match.group(0)


if __name__ == "__main__":
    unittest.main()
