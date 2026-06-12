from __future__ import annotations

import json
import re
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from typing import Any


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOW_PATH = REPO_ROOT / ".github" / "workflows" / "benchmark.yml"
WORKFLOW_DOC_PATH = REPO_ROOT / "benchmark-comparison" / "README.md"
EVALUATION_SET_DOC_PATH = REPO_ROOT / "docs" / "mi300a_evaluation_benchmark_set.md"
MANIFEST_PATH = REPO_ROOT / "benchmark-comparison" / "mi300a_finishable_size_manifest.json"
TIER1_MATRIX_PATH = REPO_ROOT / "benchmark-comparison" / "generated" / "mi300a_finishable_tier1_matrix.json"
TIER2_MATRIX_PATH = REPO_ROOT / "benchmark-comparison" / "generated" / "mi300a_finishable_tier2_matrix.json"
PROVENANCE_PATH = (
    REPO_ROOT
    / "benchmark-comparison"
    / "provenance"
    / "mi300a_problem_size_discovery_run_24959959195.json"
)

REL_MANIFEST = "benchmark-comparison/mi300a_finishable_size_manifest.json"
REL_PLAN = "benchmark-comparison/mi300a_problem_size_discovery_plan.json"
REL_REGULAR_MATRIX_VALIDATOR = "scripts/validate_mi300a_regular_workflow_matrix.py"
REL_REGULAR_MATRIX_SCRIPT = "scripts/materialize_mi300a_regular_workflow_matrix.py"
REL_PROVENANCE = "benchmark-comparison/provenance/mi300a_problem_size_discovery_run_24959959195.json"
REL_EVALUATION_MANIFEST = "benchmark-comparison/mi300a_finishable_evaluation_set.json"
REL_EVALUATION_SUMMARY = "benchmark-comparison/mi300a_finishable_evaluation_set_summary.json"


class BenchmarkWorkflowLinearFinishableTest(unittest.TestCase):
    def setUp(self) -> None:
        (REPO_ROOT / ".repo_tmp").mkdir(exist_ok=True)
        self.workflow_text = WORKFLOW_PATH.read_text(encoding="utf-8")
        self.manifest = self._read_json(MANIFEST_PATH)
        self.tier1_matrix = self._read_json(TIER1_MATRIX_PATH)
        self.tier2_matrix = self._read_json(TIER2_MATRIX_PATH)

    def test_jobs_are_linear_validation_tier1_summary_tier2_summary(self) -> None:
        self.assertEqual(
            self._job_order(),
            ["validation", "benchmark-tier1", "tier1-summary", "benchmark-tier2", "summary"],
        )

        self.assertEqual(self._job_needs("validation"), [])
        self.assertEqual(self._job_needs("benchmark-tier1"), ["validation"])
        self.assertEqual(self._job_needs("tier1-summary"), ["validation", "benchmark-tier1"])
        self.assertEqual(self._job_needs("benchmark-tier2"), ["validation", "tier1-summary"])
        self.assertEqual(
            self._job_needs("summary"),
            ["validation", "benchmark-tier1", "tier1-summary", "benchmark-tier2"],
        )

        summary_check_job = self._extract_job("tier1-summary")
        tier2_job = self._extract_job("benchmark-tier2")
        summary_job = self._extract_job("summary")
        self.assertIn('name: "Tier-1 summary"', summary_check_job)
        self.assertIn('name: "Tier 2 Bench: ${{ matrix.benchmark }}"', tier2_job)
        self.assertIn('if: always()', summary_check_job)
        self.assertIn('if: always()', summary_job)
        self.assertIn('EXPECTED_TIER1_ROWS="${{ needs.validation.outputs.tier1_sizes }}"', summary_check_job)
        self.assertIn('Tier 1 artifacts contain ${DATA_ROWS} data rows; expected ${EXPECTED_TIER1_ROWS}', summary_check_job)
        self.assertIn('Available artifacts are partial/diagnostic only.', summary_check_job)
        self.assertIn('Tier 1 did not complete successfully; Tier 2 will still run after validation for full regular matrix coverage.', summary_check_job)
        self.assertLess(
            self.workflow_text.index("  benchmark-tier1:"),
            self.workflow_text.index("  tier1-summary:"),
        )
        self.assertLess(
            self.workflow_text.index("  tier1-summary:"),
            self.workflow_text.index("  benchmark-tier2:"),
        )

    def test_workflow_dispatch_has_no_legacy_run_mode_and_defaults_to_linear_path(self) -> None:
        on_block = self._extract_on_block()
        event_names = re.findall(r"^  ([A-Za-z0-9_-]+):\n", on_block, flags=re.MULTILINE)
        dispatch = self._extract_workflow_dispatch_block()
        input_names = re.findall(r"^      ([A-Za-z0-9_-]+):\n", dispatch, flags=re.MULTILINE)

        self.assertEqual(event_names, ["workflow_dispatch"])
        self.assertEqual(input_names, ["ref"])
        self.assertNotIn("run_mode", dispatch)
        self.assertNotIn("github.event.inputs.run_mode", self.workflow_text)
        self.assertNotIn("repository_dispatch", self.workflow_text)
        self.assertNotIn("workflow_call", self.workflow_text)

        jobs = set(self._job_order())
        removed_jobs = {
            "validate-curated-primary-plan",
            "benchmark-tier1-curated-primary",
            "benchmark-tier2-curated-primary",
            "benchmark-tier2-focused-near-miss-stencil-single",
            "validate-problem-size-discovery-plan",
            "compare",
            *{f"benchmark-problem-size-discovery-shard-{index}" for index in range(1, 7)},
        }
        self.assertTrue(removed_jobs.isdisjoint(jobs))

    def test_terminal_discovery_dispatch_surface_is_not_embedded_in_main_workflow(self) -> None:
        forbidden_terminal_discovery_references = [
            "mi300a_terminal_discovery.yml",
            "MI300A Terminal Discovery",
            "validate-terminal-discovery",
            "terminal-discovery-shard",
            "collect-terminal-discovery",
            "mi300a_terminal_discovery_shard_manifest.json",
            "mi300a_terminal_discovery_shard_01_matrix.json",
            "validate_mi300a_terminal_discovery_shards.py",
            "collect_mi300a_terminal_discovery_provenance.py",
            "mi300a-terminal-discovery-plan-",
        ]
        for snippet in forbidden_terminal_discovery_references:
            self.assertNotIn(snippet, self.workflow_text)

        self.assertEqual(
            re.findall(r"^  ([A-Za-z0-9_-]+):\n", self.workflow_text.split("\njobs:\n", 1)[1], flags=re.MULTILINE),
            ["validation", "benchmark-tier1", "tier1-summary", "benchmark-tier2", "summary"],
        )
        self.assertIn(REL_PLAN, self.workflow_text)
        self.assertIn(REL_REGULAR_MATRIX_VALIDATOR, self.workflow_text)
        self.assertIn(REL_REGULAR_MATRIX_SCRIPT, self.workflow_text)

    def test_docs_explain_linear_finishable_workflow_contract(self) -> None:
        doc = WORKFLOW_DOC_PATH.read_text(encoding="utf-8")

        expected_snippets = [
            "## MI300A linear regular workflow",
            "Validation -> Tier 1 -> Tier-1 summary -> Tier 2 -> Summary",
            "Default dispatch is `workflow_dispatch` with only the optional `ref` input.",
            "There is no `run_mode` selector",
            REL_PLAN,
            REL_MANIFEST,
            "materialize_mi300a_regular_workflow_matrix.py",
            "regular workflow covers all 1416 plan entries exactly once",
            "clean per-benchmark\nmatrix rows: 19 Tier 1 rows / 308 plan entries and 63 Tier 2 rows / 1108 plan",
            "not used to prune the maintained regular runtime\nmatrix",
            "workflow now restores the full 1416-entry regular plan matrix",
            "separate from the complete finishability-evidence\nrequirement",
            "terminal finishability completion",
            "validation normalizes every row to\n`timeout_sec: 7200`",
            "strategy.max-parallel: 14",
            "timeout-minutes: 360",
            "7200 seconds per individual simulator invocation",
            "21600 seconds per benchmark-tier job",
            "checks that Tier 1 artifacts contain the\nexpected regular Tier 1 data-row count",
            "warns when uploaded artifacts are\npartial/diagnostic only because an upstream stage failed",
            "successful Summary job after failed or partial benchmark tiers is\ntherefore report/upload completion only",
            "not clean MI300A Benchmark completion",
            "GitHub Actions discovery run `24959959195` observed at `2026-04-26T17:04:42Z`",
            "immutable observation-time / bounded non-terminal snapshot, not a final/current live finishability claim",
            "23 completed-success eligible entries, 20 timed-out exclusions, and 1373 non-terminal exclusions",
            "Legacy curated-primary, focused-near-miss-stencil-single, exploratory, and problem-size-discovery dispatch modes were removed",
            "only compatibility input retained is `ref`",
            "`benchmark-comparison`, `comparison-detailed`, and `validation-report` artifacts",
            "regular_artifact_coverage_summary.json` / `regular_artifact_coverage_summary.md`",
            "selected-run-25710089668/` — Terminal current-main MI300A completed/failure partial evidence archive",
            "does not claim a clean run or final completion",
            "selected-run-25841274346/` — Newer current-main MI300A non-terminal/provisional provenance snapshot",
            "collected while the run remained `in_progress` with empty conclusion",
            "not terminal evidence, not clean MI300A Benchmark completion",
            "not finishability evidence unless refreshed terminal evidence exists from a completed run that is recollected and validated",
            "rejects non-terminal snapshot sources such as runs `25841274346` and `24959959195`",
            "`schema_name`, `schema_version`, `evidence_label`,\n`complete_regular_evidence`",
            "308 Tier 1 rows plus 1108 Tier 2 rows, for 1416 total data rows",
            "`complete_regular_evidence` means every required upstream stage succeeded",
            "It is complete regular artifact coverage only; it does\n  not claim simulator accuracy",
            "`partial_diagnostic_regular_evidence` means the required CSV artifacts are\n  present",
            "`insufficient_regular_evidence` means a required CSV is missing",
            "not finishability/provenance completion",
        ]
        for snippet in expected_snippets:
            self.assertIn(snippet, doc)

        self._assert_text_in_order(
            doc,
            [
                "Validation -> Tier 1",
                "Tier-1 summary",
                "Tier 2",
                "Summary",
            ],
        )

    def test_curated_evaluation_set_doc_is_historical_not_live_workflow_guidance(self) -> None:
        doc = EVALUATION_SET_DOC_PATH.read_text(encoding="utf-8")

        for snippet in [
            "# Historical MI300A Curated Evaluation Benchmark Set",
            "superseded for live benchmark CI by the MI300A linear",
            "It is **not** the current\n`.github/workflows/benchmark.yml` operating guide.",
            "Validation -> Tier 1 -> Tier-1 summary -> Tier 2 -> Summary",
            "The current workflow dispatch accepts only the optional `ref` input",
            "materializes the maintained regular matrices",
            "directly from the\n1416-entry problem-size discovery plan",
            "covers all 1416 plan entries exactly once",
            "clean per-benchmark rows",
            "does not provide curated-only",
            "full-suite mode selectors",
            "## Historical/Offline Use Only",
            "## Current Maintained Regular Workflow",
            "Every simulator invocation is normalized to `timeout_sec: 7200`",
            "`timeout-minutes: 360` / 21600 seconds",
            "`strategy.max-parallel: 14`",
        ]:
            self.assertIn(snippet, doc)

        removed_live_guidance = [
            "run_mode=curated-primary",
            "enforce_curated_scorecard",
            "existing full-mode benchmark workflow matrix",
            "full-mode `benchmark-tier1` or `benchmark-tier2` matrix entry",
            "## Curated-primary Workflow Dispatch",
        ]
        for snippet in removed_live_guidance:
            self.assertNotIn(snippet, doc)

    def test_m7_evaluation_surface_is_documented_without_becoming_workflow_mode(self) -> None:
        readme = WORKFLOW_DOC_PATH.read_text(encoding="utf-8")
        curated_doc = EVALUATION_SET_DOC_PATH.read_text(encoding="utf-8")

        for snippet in [
            "## Bounded finishable evaluation surface",
            "mi300a-bounded-finishable-evaluation",
            "2026.04.26-run24959959195",
            "not a new workflow mode or dispatch selector",
            REL_EVALUATION_MANIFEST,
            REL_EVALUATION_SUMMARY,
            "includes all and only the 23 eligible completed-success\nentries",
            "preserving the 1416-entry discovery/finishable contract, the 20 timed-out source\nrows, and the 1373 non-terminal source rows",
        ]:
            self.assertIn(snippet, readme)

        for snippet in [
            "For post-cleanup MI300A tuning/reporting, use the current bounded finishable\nevaluation surface instead",
            REL_EVALUATION_MANIFEST,
            REL_EVALUATION_SUMMARY,
            "That surface contains the 23 completed-success\neligible entries from run `24959959195` observed at `2026-04-26T17:04:42Z`",
            "not a new `.github/workflows/benchmark.yml` mode",
        ]:
            self.assertIn(snippet, curated_doc)

        for workflow_only_matrix_source in [
            REL_MANIFEST,
        ]:
            self.assertIn(workflow_only_matrix_source, self.workflow_text)

        for evaluation_surface_reference in [
            REL_EVALUATION_MANIFEST,
            REL_EVALUATION_SUMMARY,
            "validate_mi300a_finishable_evaluation.py",
            "mi300a-bounded-finishable-evaluation",
        ]:
            self.assertNotIn(evaluation_surface_reference, self.workflow_text)

    def test_validation_job_sources_regular_matrix_from_checked_in_plan(self) -> None:
        validation_job = self._extract_job("validation")
        validator_step = self._extract_step("Validate full regular workflow matrix contract", validation_job)
        matrix_step = self._extract_step("Materialize full regular matrix from plan", validation_job)
        tier1_job = self._extract_job("benchmark-tier1")
        tier2_job = self._extract_job("benchmark-tier2")

        for required_text in [
            REL_REGULAR_MATRIX_VALIDATOR,
            "--report regular_workflow_contract_validation.json",
        ]:
            self.assertIn(required_text, validator_step)

        for required_text in [
            REL_REGULAR_MATRIX_SCRIPT,
            "--summary regular_workflow_validation_summary.md",
            "--report regular_matrix_validation.json",
        ]:
            self.assertIn(required_text, matrix_step)

        self.assertIn("tier1_matrix: ${{ steps.matrix-outputs.outputs.tier1_matrix }}", validation_job)
        self.assertIn("tier2_matrix: ${{ steps.matrix-outputs.outputs.tier2_matrix }}", validation_job)
        self.assertIn("matrix: ${{ fromJson(needs.validation.outputs.tier1_matrix) }}", tier1_job)
        self.assertIn("matrix: ${{ fromJson(needs.validation.outputs.tier2_matrix) }}", tier2_job)

        self._assert_materializer_covers_full_plan()

    def test_regular_mi300a_benchmark_tier_contract_is_static_and_bounded(self) -> None:
        validation_job = self._extract_job("validation")
        tier1_job = self._extract_job("benchmark-tier1")
        tier2_job = self._extract_job("benchmark-tier2")
        matrix_step = self._extract_step("Materialize full regular matrix from plan", validation_job)
        metadata_step = self._extract_step("Resolve benchmark metadata", tier1_job)
        run_step = self._extract_step("Run benchmark sizes", tier1_job)

        for tier_job in (tier1_job, tier2_job):
            self.assertIn("timeout-minutes: 360", tier_job)
            self.assertIn("max-parallel: 14", tier_job)

        validator_step = self._extract_step("Validate full regular workflow matrix contract", validation_job)
        self.assertIn(REL_REGULAR_MATRIX_VALIDATOR, validator_step)
        self.assertIn("regular_workflow_contract_validation.json", validator_step)
        self.assertIn(REL_REGULAR_MATRIX_SCRIPT, matrix_step)
        self.assertIn("regular_workflow_validation_summary.md", matrix_step)
        self.assertIn("regular_matrix_validation.json", matrix_step)
        self.assertIn("MATRIX_TIMEOUT_SEC: ${{ matrix.timeout_sec || '7200' }}", metadata_step)
        self.assertIn('"timeout_sec": env("MATRIX_TIMEOUT_SEC", "7200")', metadata_step)
        self.assertIn('int(metadata["timeout_sec"]) != 7200', metadata_step)
        self.assertIn("timeout_sec must be 7200s for the regular MI300A contract", metadata_step)
        self.assertIn('TIMEOUT_SEC="${RESOLVED_TIMEOUT_SEC}"', run_step)
        self.assertIn('timeout "${TIMEOUT_SEC}"', run_step)

    def test_materializer_script_emits_full_regular_plan_matrices(self) -> None:
        with tempfile.TemporaryDirectory(dir=REPO_ROOT / ".repo_tmp") as tmp:
            tmp_path = Path(tmp)
            github_output = tmp_path / "github_output.txt"
            github_output.touch()
            result = subprocess.run(
                [
                    sys.executable,
                    str(REPO_ROOT / REL_REGULAR_MATRIX_SCRIPT),
                    "--summary",
                    str(tmp_path / "regular_workflow_validation_summary.md"),
                    "--report",
                    str(tmp_path / "regular_matrix_validation.json"),
                    "--github-output",
                    str(github_output),
                ],
                cwd=REPO_ROOT,
                capture_output=True,
                text=True,
                timeout=30,
                check=False,
            )

            self.assertEqual(
                result.returncode,
                0,
                msg=f"materializer failed\nSTDOUT:\n{result.stdout}\nSTDERR:\n{result.stderr}",
            )
            outputs = dict(
                line.split("=", 1)
                for line in github_output.read_text(encoding="utf-8").splitlines()
                if line
            )
            self.assertEqual(outputs["tier1_count"], "19")
            self.assertEqual(outputs["tier2_count"], "63")
            self.assertEqual(outputs["tier1_sizes"], "308")
            self.assertEqual(outputs["tier2_sizes"], "1108")
            self.assertEqual(outputs["regular_matrix_rows"], "82")
            self.assertEqual(outputs["regular_plan_entries"], "1416")
            tier1 = json.loads(outputs["tier1_matrix"])
            tier2 = json.loads(outputs["tier2_matrix"])
            self.assertEqual(len(tier1["include"]), 19)
            self.assertEqual(len(tier2["include"]), 63)
            self.assertEqual(
                sum(row["expected_plan_entries"] for row in tier1["include"] + tier2["include"]),
                1416,
            )
            for row in tier1["include"] + tier2["include"]:
                expected_budget = row["expected_plan_entries"] * 7200
                self.assertEqual(row["timeout_sec"], 7200)
                self.assertEqual(row["expected_simulator_invocations"], row["expected_plan_entries"])
                self.assertEqual(row["per_invocation_timeout_sec"], 7200)
                self.assertEqual(row["row_timeout_budget_sec"], expected_budget)
                self.assertEqual(row["benchmark_job_timeout_sec"], 21600)
                self.assertEqual(row["max_invocations_within_job_timeout"], 3)
                self.assertEqual(row["exceeds_benchmark_job_timeout"], expected_budget > 21600)
                self.assertTrue((REPO_ROOT / "amd" / "samples" / row["benchmark"]).is_dir())
            report = json.loads((tmp_path / "regular_matrix_validation.json").read_text(encoding="utf-8"))
            self.assertEqual(report["matrix"]["total_matrix_rows"], 82)
            self.assertEqual(report["matrix"]["total_plan_entries"], 1416)
            self.assertEqual(
                report["regular_runtime_contract"],
                {
                    "strategy_max_parallel": 14,
                    "per_simulator_timeout_sec": 7200,
                    "benchmark_job_timeout_sec": 21600,
                    "benchmark_job_timeout_minutes": 360,
                    "max_invocations_within_job_timeout": 3,
                    "row_budget_risk_is_informational": True,
                },
            )
            summary = (tmp_path / "regular_workflow_validation_summary.md").read_text(encoding="utf-8")
            self.assertIn("| benchmark-tier1 | 19 | 308 | 19 | 19 | 136800s | `1-308` |", summary)
            self.assertIn("| benchmark-tier2 | 63 | 1108 | 62 | 54 | 388800s | `309-1416` |", summary)

    def test_artifact_names_and_paths_are_stable_for_linear_workflow(self) -> None:
        validation_job = self._extract_job("validation")
        tier1_job = self._extract_job("benchmark-tier1")
        summary_job = self._extract_job("summary")

        run_step = self._extract_step("Run benchmark sizes", tier1_job)
        self.assertIn("timeout \"${TIMEOUT_SEC}\"", run_step)
        self.assertIn("[ ${FAIL} -gt 0 ]", run_step)
        self.assertIn("regular benchmark job did not produce complete usable data", run_step)

        validation_upload = self._extract_step("Upload validation summary", validation_job)
        benchmark_upload = self._extract_step("Upload benchmark results", tier1_job)
        validation_download = self._extract_step("Download validation summary artifacts", summary_job)
        merged_upload = self._extract_step("Upload merged benchmark results", summary_job)
        comparison_upload = self._extract_step("Upload comparison CSV", summary_job)
        report_upload = self._extract_step("Upload validation report artifacts", summary_job)
        final_summary_step = self._extract_step("Generate final summary", summary_job)

        self._assert_step_contains_all(
            validation_upload,
            [
                "uses: actions/upload-artifact@v4",
                "name: validation-summary",
                "path: |",
                "regular_workflow_validation_summary.md",
                "regular_matrix_validation.json",
                "regular_workflow_contract_validation.json",
                "if-no-files-found: error",
                "retention-days: 30",
            ],
        )
        self._assert_step_contains_all(
            benchmark_upload,
            [
                "name: benchmark-results-${{ steps.benchmark-metadata.outputs.artifact_id || matrix.artifact_id || matrix.benchmark || 'unknown' }}",
                "results_${{ steps.benchmark-metadata.outputs.benchmark || matrix.benchmark || 'unknown' }}.csv",
                "results_findminsad.csv",
                "if-no-files-found: ignore",
                "retention-days: 30",
            ],
        )
        self._assert_step_contains_all(
            validation_download,
            [
                "uses: actions/download-artifact@v4",
                "continue-on-error: true",
                "name: validation-summary",
                "path: validation-artifacts/",
            ],
        )
        self._assert_step_contains_all(
            merged_upload,
            [
                "name: benchmark-comparison",
                "sim_results_ci.csv",
                "comparison_ci.csv",
                "regression_ci.csv",
                "regular_artifact_coverage_summary.json",
                "regular_artifact_coverage_summary.md",
                "retention-days: 90",
            ],
        )
        self._assert_step_contains_all(
            comparison_upload,
            [
                "name: comparison-detailed",
                "path: comparison_ci.csv",
                "retention-days: 90",
            ],
        )
        self._assert_step_contains_all(
            report_upload,
            [
                "name: validation-report",
                "docs/validation_report.md",
                "docs/figures/*.png",
                REL_MANIFEST,
                "benchmark-comparison/mi300a_finishable_size_summary.json",
                REL_PROVENANCE,
                "if-no-files-found: error",
                "retention-days: 90",
            ],
        )
        self.assertIn("- Artifacts:", final_summary_step)
        for artifact_name in ("benchmark-comparison", "comparison-detailed", "validation-report"):
            self.assertIn(artifact_name, final_summary_step)
        for diagnostic_snippet in (
            'EXPECTED_TIER1_ROWS="${{ needs.validation.outputs.tier1_sizes }}"',
            'EXPECTED_TIER2_ROWS="${{ needs.validation.outputs.tier2_sizes }}"',
            "- Expected complete simulation rows: ${EXPECTED_TOTAL_ROWS}",
            "SUMMARY_FAILED=0",
            "partial-coverage run",
            'if [ "${SUMMARY_FAILED}" -ne 0 ]; then',
            "regular_matrix_validation.json was not downloaded; using checked-in regular plan for partial/diagnostic coverage expectations.",
        ):
            self.assertIn(diagnostic_snippet, final_summary_step)
        for coverage_snippet in (
            "python3 scripts/generate_validation_report.py",
            "--regular-artifact-coverage",
            "--regular-matrix-report validation-artifacts/regular_matrix_validation.json",
            "--regular-sim-csv sim_results_ci.csv",
            "--regular-comparison-csv comparison_ci.csv",
            "--regular-regression-csv regression_ci.csv",
            "--regular-artifact-coverage-json regular_artifact_coverage_summary.json",
            "--regular-artifact-coverage-md regular_artifact_coverage_summary.md",
            '--regular-stage-result "validation=${{ needs.validation.result }}"',
            '--regular-stage-result "benchmark-tier1=${{ needs[\'benchmark-tier1\'].result }}"',
            '--regular-stage-result "tier1-summary=${{ needs[\'tier1-summary\'].result }}"',
            '--regular-stage-result "benchmark-tier2=${{ needs[\'benchmark-tier2\'].result }}"',
            'cat regular_artifact_coverage_summary.md >> "${GITHUB_STEP_SUMMARY}"',
        ):
            self.assertIn(coverage_snippet, final_summary_step)
        self._assert_text_in_order(
            final_summary_step,
            [
                "python3 scripts/generate_validation_report.py",
                'cat regular_artifact_coverage_summary.md >> "${GITHUB_STEP_SUMMARY}"',
                'if [ "${SUMMARY_FAILED}" -ne 0 ]; then',
            ],
        )

    def test_summary_report_dependency_install_uses_python_module_pip(self) -> None:
        summary_job = self._extract_job("summary")
        install_step = self._extract_step("Install Python report dependencies", summary_job)

        self.assertNotRegex(
            install_step,
            r"(?m)^\s*(?:if\s+!\s+)?pip\s+install\b",
        )
        for required_text in [
            'MISSING_REPORT_DEPS="$(python3 - <<\'PY\'',
            "if ! python3 -m pip --version >/dev/null 2>&1; then",
            "python3 -m ensurepip --upgrade",
            "python3 -m pip --version",
            "python3 -m pip install pandas matplotlib",
            "failed to install Python report dependencies with python3 -m pip",
            "Python report dependencies already importable",
            'importlib.import_module("pandas")',
            'matplotlib.use("Agg")',
            'importlib.import_module("matplotlib.pyplot")',
            "failed to import Python report dependencies after installation",
        ]:
            self.assertIn(required_text, install_step)

    def test_accuracy_report_upload_is_generated_output_guarded(self) -> None:
        summary_job = self._extract_job("summary")
        generate_step = self._extract_step("Generate accuracy report", summary_job)
        summary_step = self._extract_step("Write accuracy report to step summary", summary_job)
        upload_step = self._extract_step("Upload accuracy report", summary_job)

        for required_text in [
            "id: accuracy_report",
            'ACCURACY_ARTIFACT_DIR="accuracy-report-artifact"',
            'ACCURACY_MARKER="$(mktemp)"',
            'ACCURACY_SENTINEL="${ACCURACY_ARTIFACT_DIR}/accuracy_report.generated"',
            'rm -rf "${ACCURACY_ARTIFACT_DIR}"',
            'touch "${ACCURACY_MARKER}"',
            "python3 scripts/generate_comparison_report.py",
            "accuracy_report.md was not generated by Generate accuracy report",
            "accuracy_report.md was not generated by this step",
            "reported figure was not generated by this step",
            'sentinel = artifact_dir / "accuracy_report.generated"',
            'test -f "${ACCURACY_SENTINEL}"',
            'echo "generated=true" >> "${GITHUB_OUTPUT}"',
            'echo "artifact_dir=${ACCURACY_ARTIFACT_DIR}" >> "${GITHUB_OUTPUT}"',
        ]:
            self.assertIn(required_text, generate_step)

        self._assert_text_in_order(
            generate_step,
            [
                'touch "${ACCURACY_MARKER}"',
                "python3 scripts/generate_comparison_report.py",
                'sentinel = artifact_dir / "accuracy_report.generated"',
                'echo "generated=true" >> "${GITHUB_OUTPUT}"',
            ],
        )

        generated_guard = (
            "if: ${{ always() && steps.accuracy_report.outputs.generated == 'true' }}"
        )
        self.assertIn(generated_guard, summary_step)
        self.assertIn(generated_guard, upload_step)
        self._assert_step_contains_all(
            upload_step,
            [
                "uses: actions/upload-artifact@v4",
                "name: accuracy-report",
                "path: ${{ steps.accuracy_report.outputs.artifact_dir }}/",
                "if-no-files-found: error",
                "retention-days: 90",
            ],
        )
        self.assertNotIn("if: always()", upload_step)
        self.assertNotIn("accuracy_report.md\n            docs/figures/*.png", upload_step)

    def test_run_24959959195_bounded_snapshot_wording_is_preserved(self) -> None:
        validation_job = self._extract_job("validation")
        summary_job = self._extract_job("summary")
        self.assertIn("Reference run ID: GitHub Actions run 24959959195", self.workflow_text)
        self.assertIn("bounded non-terminal snapshot", self.workflow_text)
        self.assertIn("not used to prune the maintained", self.workflow_text)

        for matrix in (self.tier1_matrix, self.tier2_matrix):
            self.assertEqual(matrix["source_provenance"], REL_PROVENANCE)
            self.assertEqual(matrix["run"]["database_id"], 24959959195)
            self.assertTrue(matrix["run"]["non_terminal_snapshot"])
            self.assertTrue(matrix["snapshot_contract"]["bounded_snapshot"])
            self.assertTrue(matrix["snapshot_contract"]["non_terminal_snapshot"])
            self.assertIn(
                "immutable observation-time evidence snapshot only",
                matrix["snapshot_contract"]["claim_scope"],
            )

        self.assertEqual(self.manifest["run"]["database_id"], 24959959195)
        self.assertTrue(self.manifest["snapshot_contract"]["bounded_snapshot"])
        self.assertEqual(self._read_json(PROVENANCE_PATH)["run"]["database_id"], 24959959195)

    def test_m45_breadth_rationale_doc_is_removed(self) -> None:
        m45_doc = REPO_ROOT / "docs" / "m45_regular_tier1_breadth_rationale.md"
        self.assertFalse(
            m45_doc.exists(),
            msg=f"{m45_doc} should have been removed as a process doc",
        )

    def _assert_materializer_covers_full_plan(self) -> None:
        result = subprocess.run(
            [
                sys.executable,
                str(REPO_ROOT / REL_REGULAR_MATRIX_SCRIPT),
                "--summary",
                str(REPO_ROOT / ".repo_tmp" / "m50_test_summary.md"),
                "--report",
                str(REPO_ROOT / ".repo_tmp" / "m50_test_report.json"),
            ],
            cwd=REPO_ROOT,
            capture_output=True,
            text=True,
            timeout=30,
            check=False,
        )
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        report = self._read_json(REPO_ROOT / ".repo_tmp" / "m50_test_report.json")
        self.assertEqual(report["matrix"]["total_plan_entries"], 1416)
        self.assertEqual(report["matrix"]["total_matrix_rows"], 82)
        self.assertEqual(report["matrix"]["jobs"]["benchmark-tier1"]["matrix_rows"], 19)
        self.assertEqual(report["matrix"]["jobs"]["benchmark-tier2"]["matrix_rows"], 63)

    def _assert_matrix_matches_manifest(self, matrix: dict[str, Any], tier: int) -> None:
        self.assertEqual(matrix["schema_name"], "mgpusim.mi300a_finishable_tier_matrix_view")
        self.assertEqual(matrix["source_manifest"], REL_MANIFEST)
        self.assertEqual(matrix["source_provenance"], REL_PROVENANCE)
        self.assertEqual(matrix["tier"], tier)
        self.assertEqual(matrix["tier_name"], f"tier{tier}")
        self.assertTrue(matrix["eligible_only"])

        eligible = [
            entry
            for entry in self.manifest["entries"]
            if entry["tier"] == tier and entry["eligible_for_linear_workflow"]
        ]
        expected_pairs = [
            (entry["workflow_benchmark_name"], entry["size_token"])
            for entry in eligible
        ]
        matrix_pairs: list[tuple[str, str]] = []
        for row in matrix["include"]:
            self.assertEqual(row["tier"], f"tier{tier}")
            self.assertLessEqual(int(row["timeout_sec"]), 3600)
            self.assertIn("flag_template", row)
            self.assertIn("size_label_template", row)
            self.assertTrue((REPO_ROOT / "amd" / "samples" / row["benchmark"]).is_dir())
            matrix_pairs.extend((row["benchmark"], size) for size in row["sizes"].split())

        self.assertEqual(matrix_pairs, expected_pairs)
        self.assertEqual(matrix["eligible_entry_count"], len(expected_pairs))
        self.assertEqual(sum(1 for _benchmark, _size in matrix_pairs), matrix["eligible_entry_count"])

    def _assert_step_contains_all(self, step_text: str, snippets: list[str]) -> None:
        for snippet in snippets:
            self.assertIn(snippet, step_text)

    def _assert_text_in_order(self, text: str, snippets: list[str]) -> None:
        cursor = -1
        for snippet in snippets:
            position = text.find(snippet, cursor + 1)
            self.assertNotEqual(position, -1, msg=f"missing ordered snippet: {snippet}")
            cursor = position

    def _read_json(self, path: Path) -> dict[str, Any]:
        with path.open("r", encoding="utf-8") as f:
            data = json.load(f)
        self.assertIsInstance(data, dict)
        return data

    def _job_order(self) -> list[str]:
        jobs_text = self.workflow_text.split("\njobs:\n", 1)[1]
        return re.findall(r"^  ([A-Za-z0-9_-]+):\n", jobs_text, flags=re.MULTILINE)

    def _job_needs(self, job_name: str) -> list[str]:
        job = self._extract_job(job_name)
        match = re.search(r"^    needs: \[(?P<needs>[^\]]+)\]$", job, flags=re.MULTILINE)
        if not match:
            return []
        return [need.strip() for need in match.group("needs").split(",")]

    def _extract_on_block(self) -> str:
        pattern = re.compile(r"^on:\n(?P<body>.*?)(?=^jobs:)", re.MULTILINE | re.DOTALL)
        match = pattern.search(self.workflow_text)
        self.assertIsNotNone(match, msg="workflow event block missing")
        return match.group(0)

    def _extract_workflow_dispatch_block(self) -> str:
        pattern = re.compile(r"^  workflow_dispatch:\n(?P<body>.*?)(?=^jobs:)", re.MULTILINE | re.DOTALL)
        match = pattern.search(self.workflow_text)
        self.assertIsNotNone(match, msg="workflow_dispatch block missing")
        return match.group(0)

    def _extract_job(self, job_name: str) -> str:
        pattern = re.compile(
            rf"^  {re.escape(job_name)}:\n(?P<body>.*?)(?=^  [A-Za-z0-9_-]+:|\Z)",
            re.MULTILINE | re.DOTALL,
        )
        match = pattern.search(self.workflow_text)
        self.assertIsNotNone(match, msg=f"workflow job missing: {job_name}")
        return match.group(0)

    def _extract_step(self, step_name: str, job_text: str) -> str:
        pattern = re.compile(
            rf"^      - name: {re.escape(step_name)}\n(?P<body>.*?)(?=^      - name: |\Z)",
            re.MULTILINE | re.DOTALL,
        )
        match = pattern.search(job_text)
        self.assertIsNotNone(match, msg=f"workflow step missing: {step_name}")
        return match.group(0)



if __name__ == "__main__":
    unittest.main()
