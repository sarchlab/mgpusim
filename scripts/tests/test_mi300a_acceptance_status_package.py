"""Static checks for the MI300A acceptance status package."""

from __future__ import annotations

from collections import Counter
import json
import re
import subprocess
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
ACCEPTANCE_STATUS = REPO_ROOT / "docs" / "mi300a_acceptance_status.md"
ACCURACY_REPORT = REPO_ROOT / "accuracy_report.md"
WORKFLOWS_DIR = REPO_ROOT / ".github" / "workflows"
BENCHMARK_WORKFLOW = WORKFLOWS_DIR / "benchmark.yml"
DELETED_BENCHMARK_TIER_RUNNER_WORKFLOW = (
    REPO_ROOT / ".github" / "workflows" / "benchmark-tier-runner.yml"
)
WORKFLOW_HYGIENE = REPO_ROOT / "docs" / "github-actions-workflow-hygiene.md"
BACKPROP_BENCHMARK = (
    REPO_ROOT / "amd" / "benchmarks" / "rodinia" / "backprop" / "benchmark.go"
)
BACKPROP_BENCHMARK_TEST = (
    REPO_ROOT / "amd" / "benchmarks" / "rodinia" / "backprop" / "benchmark_test.go"
)
SELECTED_RUN_25710089668 = (
    REPO_ROOT / "benchmark-comparison" / "selected-run-25710089668"
)
SELECTED_RUN_25841274346 = (
    REPO_ROOT / "benchmark-comparison" / "selected-run-25841274346"
)
SELECTED_RUN_25619929396 = (
    REPO_ROOT / "benchmark-comparison" / "selected-run-25619929396"
)
RUN_25710089668_PROVENANCE = REPO_ROOT / "benchmark-comparison" / "provenance"


EXPECTED_CURRENT_RUN_ID = "25710089668"
EXPECTED_CURRENT_HEAD_SHA = "6fdbbd1882f6a36c5e0846b9209a9a19c258b486"
EXPECTED_NON_TERMINAL_RUN_ID = "25841274346"
EXPECTED_NON_TERMINAL_HEAD_SHA = "e0ef96c57e042c636e2b94db8f294ff8a574bb78"
EXPECTED_FRESH_RUN_ID = "25886484910"
EXPECTED_FRESH_RUN_HEAD_SHA = "77b08d4ceb2d55e4f02e31570b349d2857b497e3"
EXPECTED_LATEST_FRESH_RUN_ID = "25910086960"
EXPECTED_LATEST_FRESH_RUN_HEAD_SHA = "a3c64f59db9e62760468d6043b0c1cf7ab6b32ee"
EXPECTED_LATEST_CURRENT_MAIN_RUN_ID = "26213004542"
EXPECTED_LATEST_CURRENT_MAIN_RUN_NUMBER = "440"
EXPECTED_LATEST_CURRENT_MAIN_HEAD_SHA = "f42286c5e9de738f94b80733ebb0f0cea6123210"
EXPECTED_STALE_CURRENT_MAIN_RUN_ID = "26146663871"
EXPECTED_STALE_CURRENT_MAIN_RUN_NUMBER = "439"
EXPECTED_STALE_CURRENT_MAIN_HEAD_SHA = "f42286c5e9de738f94b80733ebb0f0cea6123210"
EXPECTED_PREVIOUS_CURRENT_MAIN_RUN_ID = "26083372148"
EXPECTED_PREVIOUS_CURRENT_MAIN_RUN_NUMBER = "438"
EXPECTED_PREVIOUS_CURRENT_MAIN_HEAD_SHA = "c12a2e12f3109d036c95be4e4171ac06a87ff74b"
EXPECTED_RUN435_ID = "25962281031"
EXPECTED_RUN435_NUMBER = "435"
EXPECTED_RUN435_HEAD_SHA = "59be524bb40eadfd8b7a44feb6c7e7dab699232b"
EXPECTED_POST_RUN435_MAIN_HEAD_SHA = "d46ecbf9ed3bd631118a591629d1a977f5d3fff6"
EXPECTED_CURRENT_SHA_RUN_ID = "25946106740"
EXPECTED_CURRENT_SHA_DISPATCHED_RUN_ID = "25946016596"
EXPECTED_CURRENT_SHA_RUN_HEAD_SHA = "0e47eda0a98049723b4a25ac8878a414d29e0e20"
EXPECTED_ACCURACY_RUN_ID = "25619929396"
EXPECTED_ACCURACY_HEAD_SHA = "a281789a7354b86a8a04b350c145c573d531f54d"
EXPECTED_TRACKED_WORKFLOW_FILENAMES = (
    "benchmark.yml",
    "compile_hsaco.yml",
    "mgpusim_test.yml",
)
EXPECTED_TRACKED_WORKFLOW_PATHS = tuple(
    f".github/workflows/{filename}"
    for filename in EXPECTED_TRACKED_WORKFLOW_FILENAMES
)
EXPECTED_JOB_NEEDS = {
    "validation": [],
    "benchmark-tier1": ["validation"],
    "tier1-summary": ["validation", "benchmark-tier1"],
    "benchmark-tier2": ["validation", "tier1-summary"],
    "summary": ["validation", "benchmark-tier1", "tier1-summary", "benchmark-tier2"],
}
EXPECTED_STATUS_COUNTS = {
    "cancelled": 0,
    "fail": 0,
    "no-result": 793,
    "pass": 623,
    "timeout": 0,
}
EXPECTED_TOPIC_ROLLUP_SNIPPETS = {
    "#125": (
        "Tier-1 calibration is partial",
        "Selected-run report evidence exists",
        "memory, compute, and L1/L2 caveats remain",
    ),
    "#268": (
        "Tier-1 benchmark analysis",
        "`scripts/run_tier1_hardware.sh`",
        "no new hardware collection in this package",
    ),
    "#552": (
        "`adjust_weights` uses report-time affine calibration",
        "`sim_scale=0.570694861247`",
        "`fixed_time_ms=0.005321986901`",
        "11 completed selected-run samples",
        "Raw `sim_ms` rows remain unchanged",
        "commit `eb093e13`",
        "`SkipLayerforward` skips the layerforward kernel",
        "hidden-output verification is skipped",
        "adjusted weights remain verified",
        "`amd/benchmarks/rodinia/backprop/benchmark_test.go`",
        "adjusted-weight mismatch regression test",
    ),
    "#1793": (
        "BFS uses report-time calibration",
        "scale `133.490552877`",
        "fixed time `0`",
        "12 completed samples",
        "15.89% mean error",
        "34.88% max error",
        "6 larger sizes are missing",
    ),
    "#1794": (
        "L1/L2 cache-bandwidth evidence remains diagnostic/insufficient",
        "L1 16 HW/16 sim",
        "max/min 1.148×",
        "L2 16 HW/5 sim",
        "max/min 1.009×",
        "raw 89.47%",
        "Fixed-offset boosters are diagnostic only",
        "value-changing evidence still requires non-flat loaded rows",
    ),
    "#1795": (
        "real human closed the increased-time workflow-contract request",
        "2026-05-18T21:06:48.384Z",
        "closed_by=human",
        "maintained workflow contract",
        "7200 seconds per simulator invocation",
        "`timeout-minutes: 360` / 21600 seconds",
        "`strategy.max-parallel: 14`",
        "Tier 1 before Tier 2",
        "Run `25841274346` is retained only as a non-terminal stale-run provenance snapshot",
        "not terminal finishability evidence",
        "stale deleted workflow API record",
        "`.github/workflows/benchmark-tier-runner.yml`",
        "workflow id `260768869`",
        "`docs/github-actions-workflow-hygiene.md`",
        "disabled manually on 2026-05-13",
        "absent from tracked `main`",
        "benign only while no new runs appear",
        "carried forward only for #1795",
        "does not accept partial finishability",
        "resolve #2894/#2893",
    ),
    "#1804": (
        "proposed replacement specification",
        "`docs/proposed_spec.md`",
        "project-root `spec.md` is not edited",
    ),
    "#2540": (
        "Acceptance scope remains undecided",
        "calibrated `adjust_weights` reporting",
        "raw `adjust_weights` accuracy",
    ),
    "#2541": (
        "Acceptance scope remains undecided",
        "diagnostic-only L1/L2 cache-bandwidth rows",
        "value-changing cache-bandwidth evidence",
    ),
}
EXPECTED_TOPIC_ROLLUP_REFERENCES = tuple(EXPECTED_TOPIC_ROLLUP_SNIPPETS)
FORBIDDEN_USER_FACING_PROCESS_LABEL_PATTERNS = {
    "milestone label": re.compile(r"\bM\d+(?:\.\d+)?\b"),
    "epoch label": re.compile(r"\bE\d+\b"),
    "tracker/process wording": re.compile(
        r"\b(?:tracker|intake|process|milestone|epoch|orchestrator|"
        r"TBC\s+PR|tbc-db|issue\s+board)\b",
        re.IGNORECASE,
    ),
    "agent/process label": re.compile(
        r"(?:\[[A-Z][A-Za-z0-9_-]{1,32}\]|"
        r"\b(?:agent(?![- ](?:attributable|provenance))|worker|"
        r"assignee|assigned\s+to|"
        r"Sam|Maya|Ares|Leo|Athena|Apollo|TheBotCompany|thebotcompany)\b)",
        re.IGNORECASE,
    ),
}
USER_FACING_ACCEPTANCE_PACKAGE_DOCS = (
    ACCEPTANCE_STATUS,
    SELECTED_RUN_25710089668 / "README.md",
    SELECTED_RUN_25841274346 / "README.md",
)


class MI300AAcceptanceStatusPackageTest(unittest.TestCase):
    def test_acceptance_status_preserves_maintained_workflow_contract(self) -> None:
        """The status package should match the maintained regular workflow contract."""

        contract = _read_json(
            SELECTED_RUN_25710089668 / "regular_workflow_contract_validation.json"
        )
        workflow_text = BENCHMARK_WORKFLOW.read_text(encoding="utf-8")
        status_text = ACCEPTANCE_STATUS.read_text(encoding="utf-8")
        contract_section = _markdown_section(
            status_text, "Maintained workflow contract"
        )

        self.assertEqual(contract["status"], "pass")
        self.assertEqual(
            contract["source_plan"],
            "benchmark-comparison/mi300a_problem_size_discovery_plan.json",
        )
        self.assertEqual(contract["workflow"]["workflow_events"], ["workflow_dispatch"])
        self.assertEqual(contract["workflow"]["workflow_dispatch_inputs"], ["ref"])
        self.assertEqual(contract["workflow"]["job_needs"], EXPECTED_JOB_NEEDS)
        self.assertEqual(contract["workflow_benchmark_count"], 82)
        self.assertEqual(contract["plan_entry_count"], 1416)
        self.assertEqual(
            contract["tier_counts"],
            {"benchmark-tier1": 308, "benchmark-tier2": 1108},
        )
        self.assertEqual(
            contract["regular_runtime_contract"],
            {
                "benchmark_job_timeout_minutes": 360,
                "benchmark_job_timeout_sec": 21600,
                "max_invocations_within_job_timeout": 3,
                "per_simulator_timeout_sec": 7200,
                "row_budget_risk_is_informational": True,
                "strategy_max_parallel": 14,
            },
        )
        self.assertEqual(
            {
                job: (
                    contract["matrix"]["jobs"][job]["matrix_rows"],
                    contract["matrix"]["jobs"][job]["plan_entries"],
                )
                for job in ("benchmark-tier1", "benchmark-tier2")
            },
            {"benchmark-tier1": (19, 308), "benchmark-tier2": (63, 1108)},
        )

        workflow_dispatch_block = _workflow_dispatch_block(workflow_text)
        self.assertIn("ref:", workflow_dispatch_block)
        self.assertNotIn("mode:", workflow_dispatch_block)
        self.assertNotIn("terminal", workflow_dispatch_block.lower())
        for job in ("benchmark-tier1", "benchmark-tier2"):
            job_block = _workflow_job_block(workflow_text, job)
            self.assertIn("timeout-minutes: 360", job_block)
            self.assertIn("max-parallel: 14", job_block)
        self.assertIn("matrix.timeout_sec || '7200'", workflow_text)

        tracked_workflow_paths = _git_tracked_files(".github/workflows")
        self.assertEqual(tracked_workflow_paths, EXPECTED_TRACKED_WORKFLOW_PATHS)
        self.assertEqual(
            tuple(Path(path).name for path in tracked_workflow_paths),
            EXPECTED_TRACKED_WORKFLOW_FILENAMES,
        )
        self.assertNotIn(
            ".github/workflows/benchmark-tier-runner.yml",
            tracked_workflow_paths,
        )
        self.assertFalse(DELETED_BENCHMARK_TIER_RUNNER_WORKFLOW.exists())

        for snippet in (
            "Validation -> Tier 1 -> Tier-1 summary -> Tier 2 -> Summary",
            "`workflow_dispatch` with only optional `ref`; no mode selector",
            "`benchmark-comparison/mi300a_problem_size_discovery_plan.json`",
            "82 benchmark rows / 1416 plan entries",
            "Tier 1: 19 rows / 308 entries; Tier 2: 63 rows / 1108 entries",
            "`timeout_sec: 7200` seconds",
            "`timeout-minutes: 360` / 21600 seconds",
            "`strategy.max-parallel: 14`",
        ):
            self.assertIn(snippet, contract_section)

    def test_current_run_finishability_is_artifacts_only_with_expected_counts(self) -> None:
        """Run 25710089668 should remain partial artifacts-only evidence."""

        evidence = _read_json(
            SELECTED_RUN_25710089668 / "mi300a_regular_finishability_evidence.json"
        )
        run_metadata = _read_json(SELECTED_RUN_25710089668 / "run-view.json")
        status_text = ACCEPTANCE_STATUS.read_text(encoding="utf-8")
        current_section = _markdown_section(
            status_text, "Retained stale finishability evidence"
        )
        selected_run_readme = (SELECTED_RUN_25710089668 / "README.md").read_text(
            encoding="utf-8"
        )
        provenance_observation = (
            RUN_25710089668_PROVENANCE
            / "mi300a_regular_benchmark_run_25710089668_terminal_observation_20260513T001602Z.md"
        ).read_text(encoding="utf-8")

        self.assertEqual(
            evidence["schema_name"],
            "mgpusim.mi300a_regular_finishability_evidence",
        )
        self.assertEqual(evidence["run"]["run_id"], EXPECTED_CURRENT_RUN_ID)
        self.assertTrue(evidence["run"]["terminal"])
        self.assertFalse(evidence["run"]["non_terminal_snapshot"])
        self.assertEqual(evidence["run"]["workflow_name"], "MI300A Benchmark")
        self.assertEqual(evidence["run"]["event"], "workflow_dispatch")
        self.assertEqual(evidence["run"]["head_branch"], "main")
        self.assertEqual(evidence["run"]["head_sha"], EXPECTED_CURRENT_HEAD_SHA)
        self.assertEqual(evidence["run"]["status"], "completed")
        self.assertEqual(evidence["run"]["conclusion"], "failure")
        self.assertEqual(
            evidence["matrix"],
            {
                "entries_by_workflow_job": {
                    "benchmark-tier1": 308,
                    "benchmark-tier2": 1108,
                },
                "regular_matrix_rows": 82,
                "regular_plan_entries": 1416,
                "rows_by_workflow_job": {
                    "benchmark-tier1": 19,
                    "benchmark-tier2": 63,
                },
            },
        )
        self.assertEqual(evidence["status_counts"], EXPECTED_STATUS_COUNTS)
        self.assertEqual(
            _status_counter_from_evidence(evidence),
            Counter({"pass": 623, "no-result": 793}),
        )
        self.assertEqual(sum(evidence["status_counts"].values()), 1416)
        self.assertEqual(
            set(evidence["classification_policy"]["status_values"]),
            {"pass", "fail", "timeout", "cancelled", "no-result"},
        )
        self.assertIn(
            "with artifacts-only evidence and no logs, missing rows are not "
            "relabeled as fail or timeout",
            evidence["classification_policy"]["no-result"],
        )
        self.assertIn(
            "reserved for explicit per-size timeout markers",
            evidence["classification_policy"]["timeout"],
        )
        self.assertIn(
            "reserved for explicit per-size non-timeout failure markers",
            evidence["classification_policy"]["fail"],
        )
        self.assertEqual(
            evidence["source"]["result_source_kind"],
            "merged_sim_results_ci_csv",
        )
        self.assertEqual(
            evidence["source"]["result_source_paths"],
            [
                "benchmark-comparison/selected-run-25710089668/"
                "sim_results_ci.csv"
            ],
        )

        self.assertEqual(run_metadata["databaseId"], int(EXPECTED_CURRENT_RUN_ID))
        self.assertEqual(run_metadata["workflowName"], "MI300A Benchmark")
        self.assertEqual(run_metadata["event"], "workflow_dispatch")
        self.assertEqual(run_metadata["headBranch"], "main")
        self.assertEqual(run_metadata["headSha"], EXPECTED_CURRENT_HEAD_SHA)
        self.assertEqual(run_metadata["status"], "completed")
        self.assertEqual(run_metadata["conclusion"], "failure")

        for snippet in (
            f"Run `{EXPECTED_CURRENT_RUN_ID}` is retained stale terminal, "
            "completed/failure partial artifacts-only evidence",
            f"https://github.com/sarchlab/mgpusim-dev/actions/runs/{EXPECTED_CURRENT_RUN_ID}",
            f"`main` / `{EXPECTED_CURRENT_HEAD_SHA}`",
            "`MI300A Benchmark` / `workflow_dispatch`",
            "`completed` / `failure`",
            "Artifacts-only finishability summary across the 1416-entry regular plan",
            "| pass | 623 |",
            "| no-result | 793 |",
            "| fail | 0 |",
            "| timeout | 0 |",
            "| cancelled | 0 |",
            "Missing rows are not relabeled as timeout or crash from job-level failure alone.",
            "partial finishability evidence, not a clean pass",
            "not clean benchmark completion",
            "A successful final `Summary` job in a completed/failure run means report/artifact upload completed",
            "does not turn partial or failing MI300A Benchmark CI into clean completion evidence",
        ):
            self.assertIn(snippet, current_section)

        self.assertIn("artifacts-only finishability evidence", selected_run_readme)
        self.assertIn(
            "Missing planned rows are classified as",
            selected_run_readme,
        )
        self.assertIn("`no-result`, not as timeout or crash", selected_run_readme)
        self.assertIn(
            "Finishability status counts: pass=623, no-result=793, "
            "fail=0, timeout=0, cancelled=0",
            selected_run_readme,
        )
        for snippet in (
            "Summary success is artifact/report upload\ncompletion only",
            "not a clean benchmark completion claim",
            "not final project\ncompletion evidence",
        ):
            self.assertIn(snippet, selected_run_readme)
        self.assertIn("partial artifacts-only evidence", provenance_observation)
        self.assertIn(
            "Status counts derived from populated simulator timing rows: "
            "pass=623, no-result=793, fail=0, timeout=0",
            provenance_observation,
        )

    def test_latest_retained_run_25841274346_is_non_terminal_snapshot(self) -> None:
        """Run 25841274346 must not be promoted to terminal finishability evidence."""

        run_metadata = _read_json(SELECTED_RUN_25841274346 / "run-view.json")
        status_text = ACCEPTANCE_STATUS.read_text(encoding="utf-8")
        snapshot_section = _markdown_section(
            status_text, "Retained non-terminal run snapshot"
        )
        selected_run_readme = (SELECTED_RUN_25841274346 / "README.md").read_text(
            encoding="utf-8"
        )
        job_summary_rows = _read_tsv(SELECTED_RUN_25841274346 / "jobs-summary.tsv")

        self.assertEqual(run_metadata["databaseId"], int(EXPECTED_NON_TERMINAL_RUN_ID))
        self.assertEqual(run_metadata["workflowName"], "MI300A Benchmark")
        self.assertEqual(run_metadata["event"], "workflow_dispatch")
        self.assertEqual(run_metadata["headBranch"], "main")
        self.assertEqual(run_metadata["headSha"], EXPECTED_NON_TERMINAL_HEAD_SHA)
        self.assertEqual(run_metadata["status"], "in_progress")
        self.assertEqual(run_metadata["conclusion"], "")
        self.assertTrue(run_metadata["provenance_collection"]["non_terminal_snapshot"])
        self.assertEqual(
            run_metadata["provenance_collection"]["collected_at_utc"],
            "2026-05-14T10:05:17Z",
        )

        self.assertEqual(
            Counter(row["status"] for row in job_summary_rows),
            Counter({"completed": 18, "in_progress": 2}),
        )
        self.assertEqual(
            Counter(row["conclusion"] or "empty" for row in job_summary_rows),
            Counter({"failure": 15, "success": 3, "empty": 2}),
        )

        for snippet in (
            f"Run `{EXPECTED_NON_TERMINAL_RUN_ID}` is retained only as the checked-in "
            "non-terminal/provisional provenance snapshot from a stale maintained `main` run",
            "checked-in package stays non-terminal/provisional unless a separate "
            "terminal package is committed and validated",
            f"https://github.com/sarchlab/mgpusim-dev/actions/runs/{EXPECTED_NON_TERMINAL_RUN_ID}",
            f"`main` / `{EXPECTED_NON_TERMINAL_HEAD_SHA}`",
            "`MI300A Benchmark` / `workflow_dispatch`",
            "`in_progress` / ``",
            "`2026-05-14T10:05:17Z`",
            "`18` completed jobs and `2` in-progress jobs",
            "job conclusion counts `failure=15`, `success=3`, and `empty=2`",
            "artifacts are provisional",
            "later read-only Actions recheck at `2026-05-14T20:28Z`",
            "no newer `main` `workflow_dispatch` `benchmark.yml` run",
            "live `main` was `404c8f2e09d6b59e7b15a84425437a886a5e2f16`",
            "remained `in_progress` with `conclusion=null`",
            "84 jobs, 71 completed, 13 in progress",
            "no final `benchmark-comparison`, `comparison-detailed`, `validation-report`, "
            "`accuracy-report`, or final `Summary` job evidence",
            "does not claim a clean pass",
            "clean benchmark completion",
            "complete finishability evidence",
            "Do not derive or claim terminal finishability from this checked-in package",
            "keep it non-terminal/provisional unless a separate terminal package "
            "is committed and validated",
            "re-run the provenance collector after the run reaches `completed`",
        ):
            self.assertIn(snippet, snapshot_section)

        for snippet in (
            "Evidence state: `non_terminal_snapshot`",
            "This is **not terminal evidence**",
            "no clean pass, no-result, failure",
            "complete finishability conclusion",
            "Artifacts are provisional",
            "checked-in snapshot stays non-terminal/provisional",
            "terminal package is committed and validated",
            "re-run the collector after",
        ):
            self.assertIn(snippet, selected_run_readme)

    def test_fresh_current_main_run_attempt_is_not_terminal_evidence(self) -> None:
        """The fresh maintained run attempts must not be promoted to clean evidence."""

        status_text = ACCEPTANCE_STATUS.read_text(encoding="utf-8")
        fresh_section = _markdown_section(status_text, "Fresh maintained run attempts")

        run_440_paragraph = next(
            paragraph
            for paragraph in _paragraphs_containing(
                fresh_section, f"#440 / `{EXPECTED_LATEST_CURRENT_MAIN_RUN_ID}`"
            )
            if "latest read-only adjudication boundary" in paragraph
        )
        run_439_paragraph = next(
            paragraph
            for paragraph in _paragraphs_containing(
                fresh_section, f"#439 / `{EXPECTED_STALE_CURRENT_MAIN_RUN_ID}`"
            )
            if "older/stale relative to #440" in paragraph
        )
        run_438_paragraph = next(
            paragraph
            for paragraph in _paragraphs_containing(
                fresh_section, f"#438 / `{EXPECTED_PREVIOUS_CURRENT_MAIN_RUN_ID}`"
            )
            if "older read-only adjudication boundary" in paragraph
        )
        latest_run_435_paragraph = next(
            paragraph
            for paragraph in _paragraphs_containing(
                fresh_section, f"#435 / `{EXPECTED_RUN435_ID}`"
            )
            if "2026-05-16T20:48Z" in paragraph
        )

        for snippet in (
            f"`origin/main` and remote `main` at "
            f"`{EXPECTED_LATEST_CURRENT_MAIN_HEAD_SHA}`",
            f"#440 / `{EXPECTED_LATEST_CURRENT_MAIN_RUN_ID}`",
            "created at `2026-05-21T07:51:41Z`",
            "updated at `2026-05-22T06:02:19Z`",
            f"from `main` / `{EXPECTED_LATEST_CURRENT_MAIN_HEAD_SHA}`",
            "latest-run query lists #440 above older same-head run #439, older "
            "stale run #438, and older maintained runs",
            "matches current `origin/main`",
            "top-level `status=completed` and `conclusion=failure`",
            "not completed/success clean evidence",
            "85 completed jobs total",
            "job conclusions are `success=5`, `failure=76`, and `cancelled=4`",
            "final `Summary: merge, compare, and report` job `77325134348` "
            "completed with `success`",
            "from `2026-05-22T06:01:53Z` to `2026-05-22T06:02:18Z`",
            "artifact API lists 87 artifacts",
            "`benchmark-comparison` (artifact `7153796957`, 36612 bytes)",
            "`comparison-detailed` (`7153797086`, 18209 bytes)",
            "`validation-report` (`7153797501`, 11128008 bytes)",
            "`accuracy-report` (`7153797745`, 3713274 bytes)",
            "`complete_regular_evidence=false`",
            "`partial_diagnostic_regular_evidence`",
            "upstream `benchmark-tier1` and `benchmark-tier2` failures",
            "`sim_results_ci.csv` with 632/1416 data rows",
            "`regression_ci.csv` with 517/1416 data rows",
            "Gate result: current-main terminal failure with final artifacts",
            "not completed/success and not complete regular evidence",
            "no clean-completion claim",
            "no merge/report regeneration from downloaded artifacts",
            "no terminal finishability acceptance is derived",
            "Run #440 is the latest current-main terminal failed final-artifact "
            "evidence only",
            "not clean terminal evidence",
            "does not resolve #2894 or #2893",
            "completed/success current-main run with `complete_regular_evidence`",
            "explicit real-human acceptance/closure",
        ):
            self.assertIn(snippet, run_440_paragraph)

        for forbidden in (
            "is completed/success clean evidence",
            "clean terminal maintained MI300A Benchmark run is collected",
            "resolves #2894",
            "resolves #2893",
        ):
            with self.subTest(run="440", forbidden=forbidden):
                self.assertNotIn(forbidden, run_440_paragraph)

        for snippet in (
            f"#439 / `{EXPECTED_STALE_CURRENT_MAIN_RUN_ID}`",
            "older/stale relative to #440",
            f"same head `main` / `{EXPECTED_STALE_CURRENT_MAIN_HEAD_SHA}`",
            "created at `2026-05-20T06:56:36Z`",
            "updated at `2026-05-21T04:05:11Z`",
            "top-level `status=completed` and `conclusion=failure`",
            "not completed/success clean evidence",
            "final `Summary: merge, compare, and report` job `77102101849` "
            "completed with `success`",
            "required final artifacts `benchmark-comparison`, `comparison-detailed`, "
            "`validation-report`, and `accuracy-report` are present",
            "`complete_regular_evidence=false`",
            "upstream `benchmark-tier1` and `benchmark-tier2` failures",
            "`sim_results_ci.csv` with 629/1416 data rows",
            "`regression_ci.csv` with 511/1416 data rows",
            "Gate result: older same-head terminal failure with final artifacts",
            "not completed/success and not complete regular evidence",
            "no clean-completion claim",
            "no merge/report regeneration from downloaded artifacts",
            "no terminal finishability acceptance is derived",
            "Run #439 is preserved only as older completed/failure final-artifact "
            "evidence stale relative to #440",
            "not current latest evidence",
            "not clean terminal evidence",
            "does not resolve #2894 or #2893",
        ):
            self.assertIn(snippet, run_439_paragraph)

        for forbidden in (
            "Run #439 is current-main terminal failed final-artifact evidence only",
            "Run #439 is the latest current-main terminal failed final-artifact "
            "evidence",
            "resolves #2894",
            "resolves #2893",
        ):
            with self.subTest(run="439", forbidden=forbidden):
                self.assertNotIn(forbidden, run_439_paragraph)

        for snippet in (
            f"`origin/main` and remote `main` at "
            f"`{EXPECTED_PREVIOUS_CURRENT_MAIN_HEAD_SHA}`",
            f"#438 / `{EXPECTED_PREVIOUS_CURRENT_MAIN_RUN_ID}`",
            "created at `2026-05-19T07:40:01Z`",
            "updated at `2026-05-20T05:32:39Z`",
            f"from `main` / `{EXPECTED_PREVIOUS_CURRENT_MAIN_HEAD_SHA}`",
            "latest-run query listed #438 above stale run #437 and older maintained runs",
            "matched that then-current `origin/main`",
            "top-level `status=completed` and `conclusion=failure`",
            "not completed/success clean evidence",
            "85 completed jobs total",
            "job conclusions were `success=5`, `failure=77`, and `cancelled=3`",
            "final `Summary: merge, compare, and report` job completed with `success`",
            "from `2026-05-20T05:32:13Z` to `2026-05-20T05:32:38Z`",
            "artifact API listed 87 artifacts",
            "required final artifacts `benchmark-comparison`, `comparison-detailed`, "
            "`validation-report`, and `accuracy-report`",
            "Gate result: then-current-main terminal failure with final artifacts",
            "not completed/success and no completed/success `complete_regular_evidence`",
            "no clean-completion claim",
            "no merge/report regeneration from downloaded artifacts",
            "no terminal finishability acceptance is derived",
            "Run #438 is now older/stale relative to #439 and #440",
            f"`{EXPECTED_LATEST_CURRENT_MAIN_HEAD_SHA}`",
            "not current latest evidence",
            "not clean terminal evidence",
            "does not resolve #2894 or #2893",
        ):
            self.assertIn(snippet, run_438_paragraph)

        for forbidden in (
            "Run #438 is current-main terminal failed final-artifact evidence only",
            "Run #438 is the latest current-main terminal failed final-artifact evidence",
            "resolves #2894",
            "resolves #2893",
        ):
            with self.subTest(run="438", forbidden=forbidden):
                self.assertNotIn(forbidden, run_438_paragraph)

        for snippet in (
            "2026-05-16T20:48Z",
            f"`main` / `{EXPECTED_RUN435_HEAD_SHA}`",
            f"current `origin/main` and remote `main` were "
            f"`{EXPECTED_POST_RUN435_MAIN_HEAD_SHA}`",
            "the run head was stale",
            "an ancestor of current `main` and `4` commits behind",
            "non-terminal with `status=queued` and `conclusion=null`",
            "20 jobs total: `11` completed, `4` in progress, and `5` queued",
            "job conclusions were `success=3`, `failure=8`, and `empty=9`",
            "No Summary/summary/compare/report-like job was present",
            "Eleven available artifacts existed (`validation-summary` plus "
            "`benchmark-results-*` artifacts)",
            "required final artifacts remained absent: `benchmark-comparison`, "
            "`comparison-detailed`, `validation-report`, and `accuracy-report`",
            "Gate result: stale, non-terminal, and missing final Summary/report artifacts",
        ):
            self.assertIn(snippet, latest_run_435_paragraph)

        for snippet in (
            "No artifacts were downloaded",
            "no terminal finishability/provenance was derived",
            "no dispatch/rerun/cancel was performed during those follow-up rechecks",
            "remains stale completed/failure final-artifact evidence only",
            "does not resolve the clean terminal evidence blocker",
            "follow-up status review",
            "queried the post-packet window across the rollup issue/comment records",
            "including #2893/#2894 and the records containing #125, #268, "
            "#552, #1793, #1794, #1795, #1804, #2540, and #2541",
            "latest status review repeated that check after the missed-deadline "
            "planning update",
            "2026-05-17T13:25:46Z",
            "further run #440 boundary review",
            "no actual real-human **ACCEPT**/**DECLINE** or "
            "**CLOSE**/**KEEP OPEN** decisions",
            "no explicit human **ACCEPT**/**DECLINE** responses for #2894, #125, "
            "#552, #1793, #1794, #2540, or #2541",
            "no human **CLOSE**/**KEEP OPEN** responses to the still-pending "
            "**CLOSE REQUEST** rows for #268 or #1804",
            "acceptance/scope items and the #268/#1804 closure requests therefore "
            "remain undecided/open at that pre-close checkpoint",
            "later #1795 human **CLOSE** decision is carried forward",
            "makes no final project-completion claim",
        ):
            self.assertIn(snippet, fresh_section)

    def test_run_classifications_are_consistent_across_sections(self) -> None:
        """Repeated run references must not conflict across status sections."""

        status_text = ACCEPTANCE_STATUS.read_text(encoding="utf-8")
        fresh_section = _markdown_section(status_text, "Fresh maintained run attempts")
        historical_section = _markdown_section(
            status_text, "Historical accuracy/report provenance"
        )

        run_258864_paragraphs = _paragraphs_containing(
            status_text, EXPECTED_FRESH_RUN_ID
        )
        self.assertGreaterEqual(len(run_258864_paragraphs), 2)
        self.assertIn("terminal `completed`/`failure`", fresh_section)
        self.assertIn(
            "final `Summary: merge, compare, and report` completed successfully",
            fresh_section,
        )
        self.assertIn(
            "86 artifacts were available including final `benchmark-comparison`, "
            "`comparison-detailed`, `validation-report`, and `accuracy-report` artifacts",
            fresh_section,
        )
        self.assertIn(
            "Use run `25886484910` only as stale completed/failure "
            "final-artifact evidence with final Summary/final report artifacts",
            historical_section,
        )
        self.assertIn(
            "not completed/success current-main evidence and not active/non-terminal "
            "evidence",
            historical_section,
        )
        self.assertIn(
            "Use run `25910086960` only as stale completed/failure "
            "final-artifact evidence with final Summary/final report artifacts",
            historical_section,
        )
        self.assertIn(
            "not completed/success current-main evidence and not "
            "active/non-terminal/provisional evidence",
            historical_section,
        )
        self.assertIn(
            "Use run `26213004542` only as the latest current-main terminal "
            "completed/failure final-artifact evidence with successful final "
            "Summary upload",
            historical_section,
        )
        self.assertIn(
            "final `benchmark-comparison`, `comparison-detailed`, `validation-report`, "
            "and `accuracy-report` artifacts",
            historical_section,
        )
        self.assertIn(
            f"it matches current `main` / "
            f"`{EXPECTED_LATEST_CURRENT_MAIN_HEAD_SHA}`",
            historical_section,
        )
        self.assertIn(
            "it is not completed/success clean evidence and "
            "`complete_regular_evidence=false` is recorded",
            historical_section,
        )
        self.assertIn(
            "Use run `26146663871` only as older same-head completed/failure "
            "final-artifact evidence with successful final Summary upload",
            historical_section,
        )
        self.assertIn(
            "required final artifacts present",
            historical_section,
        )
        self.assertIn(
            "stale relative to run `26213004542` at current `main` / "
            f"`{EXPECTED_LATEST_CURRENT_MAIN_HEAD_SHA}`",
            historical_section,
        )
        self.assertIn(
            "upstream benchmark-tier failures with 629/1416 simulation rows "
            "and 511/1416 regression rows",
            historical_section,
        )
        self.assertIn(
            "Use run `26083372148` only as older completed/failure "
            "final-artifact evidence with successful final Summary upload",
            historical_section,
        )
        self.assertIn(
            "it is stale relative to #439/#440 at current `main` / "
            f"`{EXPECTED_LATEST_CURRENT_MAIN_HEAD_SHA}`",
            historical_section,
        )
        self.assertIn("is not current latest evidence", historical_section)
        self.assertIn(
            "Use run `25993763757` only as older completed/failure "
            "final-artifact evidence with successful final Summary upload",
            historical_section,
        )
        self.assertIn(
            f"it is stale relative to current `main` / "
            f"`{EXPECTED_LATEST_CURRENT_MAIN_HEAD_SHA}`",
            historical_section,
        )
        self.assertIn(
            "partial diagnostic regular evidence with 618/1416 simulation rows "
            "and 500/1416 regression rows",
            historical_section,
        )
        self.assertIn(
            "Use stale run `25974165191` only as the previous current-main "
            "run attempt",
            historical_section,
        )
        self.assertIn(
            "must not be reused as clean evidence",
            historical_section,
        )
        self.assertIn(
            "Use run `25962281031` only as the previous queued/provisional stale "
            "fresh run attempt with `validation-summary` plus partial "
            "`benchmark-results-*` artifacts and no final Summary/report artifacts",
            historical_section,
        )
        self.assertIn(
            f"it is stale relative to current `main` / "
            f"`{EXPECTED_LATEST_CURRENT_MAIN_HEAD_SHA}`",
            historical_section,
        )
        self.assertIn(
            "and is not completed/success current-main evidence",
            historical_section,
        )
        self.assertIn(
            "Use run `25946016596` only as the recorded package-attributed earlier "
            "then-current-SHA launch",
            historical_section,
        )
        self.assertIn(
            "run `25946106740` only as the observed additional earlier then-current-SHA "
            "run with unknown/external provenance",
            historical_section,
        )
        self.assertIn(
            "those earlier runs are stale relative to "
            f"`{EXPECTED_LATEST_CURRENT_MAIN_HEAD_SHA}`",
            historical_section,
        )
        self.assertIn(
            "remain non-terminal/failed-partial/provisional attempts",
            historical_section,
        )

        for snippet in (
            f"`{EXPECTED_CURRENT_SHA_RUN_ID}`",
            f"`{EXPECTED_CURRENT_SHA_DISPATCHED_RUN_ID}`",
            f"`{EXPECTED_CURRENT_SHA_RUN_HEAD_SHA}`",
            f"`{EXPECTED_LATEST_CURRENT_MAIN_RUN_ID}`",
            f"`{EXPECTED_LATEST_CURRENT_MAIN_HEAD_SHA}`",
            f"`{EXPECTED_STALE_CURRENT_MAIN_RUN_ID}`",
            f"`{EXPECTED_STALE_CURRENT_MAIN_HEAD_SHA}`",
            "latest current-main terminal failed final-artifact evidence only",
            "Run #439 is preserved only as older completed/failure final-artifact "
            "evidence stale relative to #440",
            "Run #438 is now older/stale relative to #439 and #440",
            "Run #436 is now stale relative to #438, #439, and #440",
            f"`{EXPECTED_RUN435_ID}`",
            f"`{EXPECTED_RUN435_HEAD_SHA}`",
            "queued/provisional stale evidence only",
            "stale relative to later `main`",
            "remain non-terminal/failed-partial/provisional evidence only",
            "not clean terminal evidence",
        ):
            self.assertIn(snippet, fresh_section)

        forbidden_258864_classifications = (
            "Use run `25886484910` only as an earlier active/non-terminal",
            "Run `25886484910` remained active/non-terminal",
            "`25886484910` remained active/non-terminal",
            "`status=queued`",
            "`conclusion=null`",
            "final `Summary` job evidence and final `benchmark-comparison`",
            "final `benchmark-comparison`, `comparison-detailed`, `validation-report`, "
            "and `accuracy-report` artifacts were absent",
        )
        for paragraph in run_258864_paragraphs:
            for forbidden in forbidden_258864_classifications:
                with self.subTest(run=EXPECTED_FRESH_RUN_ID, forbidden=forbidden):
                    self.assertNotIn(forbidden, paragraph)

        run_259100_paragraphs = _paragraphs_containing(
            status_text, EXPECTED_LATEST_FRESH_RUN_ID
        )
        self.assertGreaterEqual(len(run_259100_paragraphs), 2)
        self.assertIn(
            "Run `25910086960` therefore remains stale completed/failure "
            "final-artifact evidence only",
            fresh_section,
        )
        forbidden_259100_classifications = (
            "Use run `25910086960` only as a stale non-terminal/provisional",
            "stale active/non-terminal/provisional evidence only",
            "with `status=queued` and `conclusion=null`",
            "final `Summary: merge, compare, and report` evidence was absent",
            "The final `benchmark-comparison`, `comparison-detailed`, "
            "`validation-report`, and `accuracy-report` artifacts were absent",
        )
        for paragraph in run_259100_paragraphs:
            for forbidden in forbidden_259100_classifications:
                with self.subTest(run=EXPECTED_LATEST_FRESH_RUN_ID, forbidden=forbidden):
                    self.assertNotIn(forbidden, paragraph)

        stale_current_main_conflicts = (
            "latest retained current-main terminal",
            "separate current-main artifacts-only finishability/provenance evidence",
            "current-main non-terminal/provisional provenance snapshot",
            "non-terminal current-main provenance snapshot",
            "current-main run `25710089668` is separate finishability evidence",
            "current-main run `25710089668`",
        )
        for forbidden in stale_current_main_conflicts:
            with self.subTest(forbidden=forbidden):
                self.assertNotIn(forbidden, status_text)

        for run_id in (EXPECTED_CURRENT_RUN_ID, EXPECTED_NON_TERMINAL_RUN_ID):
            joined = "\n\n".join(_paragraphs_containing(status_text, run_id))
            with self.subTest(run=run_id):
                self.assertIn("stale", joined)

    def test_duplicate_current_sha_dispatch_provenance_is_not_collapsed(self) -> None:
        """#434 must stay visible as an additional then-current-SHA run."""

        status_text = ACCEPTANCE_STATUS.read_text(encoding="utf-8")
        fresh_section = _markdown_section(status_text, "Fresh maintained run attempts")
        historical_section = _markdown_section(
            status_text, "Historical accuracy/report provenance"
        )

        self.assertEqual(status_text.count("agent-attributable"), 1)
        self.assertIn(
            "records exactly one agent-attributable maintained "
            "`.github/workflows/benchmark.yml` / `MI300A Benchmark` "
            "`workflow_dispatch` launch at that then-current SHA: #433 / "
            f"`{EXPECTED_CURRENT_SHA_DISPATCHED_RUN_ID}`",
            fresh_section,
        )
        self.assertIn(
            "observed additional maintained `.github/workflows/benchmark.yml` / "
            "`MI300A Benchmark` `workflow_dispatch` run, #434 / "
            f"`{EXPECTED_CURRENT_SHA_RUN_ID}`",
            fresh_section,
        )
        self.assertIn(
            "with unknown/external provenance unless later evidence proves otherwise",
            fresh_section,
        )
        self.assertIn(
            "GitHub metadata `actor`/`triggering_actor` value `syifan` is not "
            "agent provenance",
            fresh_section,
        )
        self.assertIn(
            "not an at-most-one/current-run story while #434 exists",
            fresh_section,
        )
        self.assertIn(
            "found two then-current-SHA maintained `main` `workflow_dispatch` runs",
            fresh_section,
        )
        self.assertIn(
            f"#435 / `{EXPECTED_RUN435_ID}`",
            fresh_section,
        )
        self.assertIn(
            f"Run #435's head stayed `main` / "
            f"`{EXPECTED_RUN435_HEAD_SHA}`",
            fresh_section,
        )
        self.assertIn(
            f"current `origin/main` and remote `main` were "
            f"`{EXPECTED_POST_RUN435_MAIN_HEAD_SHA}`",
            fresh_section,
        )
        self.assertIn(
            "run head was stale: an ancestor of current `main` and `4` commits behind",
            fresh_section,
        )
        self.assertIn(EXPECTED_RUN435_ID, historical_section)
        self.assertIn(EXPECTED_CURRENT_SHA_RUN_ID, historical_section)
        self.assertIn(EXPECTED_CURRENT_SHA_DISPATCHED_RUN_ID, historical_section)

        forbidden_collapsed_stories = (
            "no newer current-SHA maintained workflow_dispatch run",
            "only current-SHA maintained workflow_dispatch run",
            "at most one current-SHA",
            "at-most-one current-SHA",
            "#433 was the current-SHA run",
            "#434 is agent-attributable",
        )
        for forbidden in forbidden_collapsed_stories:
            with self.subTest(forbidden=forbidden):
                self.assertNotIn(forbidden, status_text)

    def test_selected_run_25619929396_remains_distinct_accuracy_source(self) -> None:
        """Accuracy/report provenance must not be silently rebound to run 25710089668."""

        status_text = ACCEPTANCE_STATUS.read_text(encoding="utf-8")
        accuracy_text = ACCURACY_REPORT.read_text(encoding="utf-8")
        historical_section = _markdown_section(
            status_text, "Historical accuracy/report provenance"
        )
        current_section = _markdown_section(
            status_text, "Retained stale finishability evidence"
        )
        selected_run_256_readme = (SELECTED_RUN_25619929396 / "README.md").read_text(
            encoding="utf-8"
        )
        selected_run_257_readme = (SELECTED_RUN_25710089668 / "README.md").read_text(
            encoding="utf-8"
        )

        self.assertIn(
            f"selected run `{EXPECTED_ACCURACY_RUN_ID}`, not from "
            f"retained stale finishability run `{EXPECTED_CURRENT_RUN_ID}`",
            historical_section,
        )
        self.assertIn(
            f"https://github.com/sarchlab/mgpusim-dev/actions/runs/{EXPECTED_ACCURACY_RUN_ID}",
            historical_section,
        )
        self.assertIn(f"`main` / `{EXPECTED_ACCURACY_HEAD_SHA}`", historical_section)
        self.assertIn(
            "`benchmark-comparison/selected-run-25619929396/"
            "comparison_ci.detailed.csv` rows with both `real_ms` "
            "and `sim_ms` populated",
            historical_section,
        )
        self.assertIn(
            "Use run `25710089668` only for the separate retained stale "
            "artifacts-only finishability/provenance evidence above",
            historical_section,
        )
        self.assertIn(
            "latest read-only refresh still found it completed/failure partial "
            "diagnostic evidence, not clean completion",
            historical_section,
        )
        self.assertIn(
            "latest refreshed Actions artifacts for that run were completed/failure "
            "partial diagnostic evidence, not a clean terminal package",
            historical_section,
        )
        self.assertNotIn("Accuracy source", current_section)
        self.assertNotIn(
            "comparison_ci.detailed.csv` rows with both `real_ms` "
            "and `sim_ms` populated",
            current_section,
        )
        self.assertIn(
            "Historical report source data: `benchmark-comparison/"
            "selected-run-25619929396/comparison_ci.detailed.csv`",
            accuracy_text,
        )
        self.assertIn(
            "not regenerated from current-main terminal run `25710089668`",
            accuracy_text,
        )
        self.assertIn(
            "Current-main run `25710089668` provides separate terminal "
            "artifacts-only finishability/provenance evidence",
            accuracy_text,
        )

        self.assertIn(
            f"`{EXPECTED_ACCURACY_RUN_ID}` on head `{EXPECTED_ACCURACY_HEAD_SHA}`",
            selected_run_256_readme,
        )
        self.assertIn("selected-run artifact bytes", selected_run_256_readme)
        self.assertIn(
            "Historical workflow-contract text in raw artifacts",
            selected_run_256_readme,
        )
        self.assertIn(
            "remain historical selected-run\nprovenance until a separate "
            "report-selection change replaces them",
            selected_run_257_readme,
        )

    def test_acceptance_status_includes_per_human_topic_rollup(self) -> None:
        """The status package should keep the benchmark acceptance topic rollup."""

        status_text = ACCEPTANCE_STATUS.read_text(encoding="utf-8")
        rollup = _markdown_section(status_text, "Benchmark acceptance topic rollup")
        table_rows = _markdown_table_rows(rollup)

        self.assertIn("benchmark-acceptance topics", rollup)
        self.assertIn("not an acceptance decision", rollup)
        self.assertEqual(
            [ref for ref, _summary in table_rows],
            list(EXPECTED_TOPIC_ROLLUP_REFERENCES),
        )

        for ref, expected_snippets in EXPECTED_TOPIC_ROLLUP_SNIPPETS.items():
            with self.subTest(ref=ref):
                row_text = dict(table_rows)[ref]
                for snippet in expected_snippets:
                    self.assertIn(snippet, row_text)

        for snippet in (
            "Use the selected-run files for the historical accuracy/report numbers",
            "Use run `25710089668` only for the separate retained stale "
            "artifacts-only finishability/provenance evidence above",
            "Use the checked-in run `25841274346` package only as the retained "
            "non-terminal stale provenance snapshot above",
            "latest current-main terminal failed final-artifact evidence, historical "
            "selected-run accuracy reporting, retained stale terminal artifacts-only "
            "finishability evidence, and the retained non-terminal stale-run "
            "provenance snapshot",
        ):
            self.assertIn(snippet, status_text)

    def test_acceptance_status_includes_final_human_decision_packet(
        self,
    ) -> None:
        """The packet should expose exact human choices without overclaiming."""

        status_text = ACCEPTANCE_STATUS.read_text(encoding="utf-8")
        selected_run_readme = (SELECTED_RUN_25841274346 / "README.md").read_text(
            encoding="utf-8"
        )
        packet = _markdown_section(status_text, "Final human decision packet")
        decision_rows = _markdown_table_rows_by_ref(packet)

        expected_order = [
            "#2894",
            "#125",
            "#552",
            "#1793",
            "#1794",
            "#2540",
            "#2541",
            "#268",
            "#1804",
        ]
        self.assertEqual(list(decision_rows), expected_order)
        self.assertNotIn("Human/HUMAN acceptance disposition checklist", status_text)

        for snippet in (
            "human-actionable decision surface",
            "records no acceptance by itself",
            "no row is preselected",
            "no human-owned issue is closed here",
            "no repository author self-accepts a caveat",
            "no final project-completion claim is made",
            "choose exactly one option per remaining row",
            "run `25710089668` is only completed/failure partial artifacts-only "
            "finishability evidence",
            "checked-in run `25841274346` package remains non-terminal/provisional",
            "unless a separate terminal package is committed and validated",
            "run `26213004542` is the latest current-main terminal "
            "completed/failure final-artifact evidence",
            "has `complete_regular_evidence=false`",
            "run `26146663871` is older same-head completed/failure "
            "final-artifact evidence stale relative to #440",
            "run `26083372148` is older completed/failure final-artifact "
            "evidence stale relative to #439/#440",
            "historical accuracy tables remain selected-run `25619929396` evidence",
            "only carried-forward post-planning human decision",
            "#1795 `CLOSE`",
            "closed by `human` at `2026-05-18T21:06:48.384Z`",
            "does not accept partial finishability or resolve #2894/#2893",
            "found no qualifying real-human **ACCEPT**/**DECLINE** decision",
            "run #440 boundary review",
            "latest decision recheck after the planning boundary carries forward "
            "exactly one qualifying real-human **CLOSE** decision",
            "No other real-human **CLOSE**/**KEEP OPEN** decision for #268 or #1804",
            "#2894 and #2893 remain open/unresolved absent completed/success "
            "`complete_regular_evidence` or actual real-human decisions",
            "without reclassifying partial/failing/non-terminal CI as clean completion",
            "the remaining closure-request rows remain requests rather than issue closures",
        ):
            self.assertIn(snippet, packet)

        accept_decline_refs = (
            "#2894",
            "#125",
            "#552",
            "#1793",
            "#1794",
            "#2540",
            "#2541",
        )
        for ref in accept_decline_refs:
            with self.subTest(ref=ref):
                self.assertIn("**ACCEPT**", decision_rows[ref])
                self.assertIn("**DECLINE**", decision_rows[ref])
                self.assertNotIn("**CLOSE REQUEST**", decision_rows[ref])

        closure_refs = ("#268", "#1804")
        for ref in closure_refs:
            with self.subTest(ref=ref):
                self.assertIn("**CLOSE REQUEST**", decision_rows[ref])
                self.assertIn("**KEEP OPEN**", decision_rows[ref])
                self.assertNotIn("**ACCEPT**", decision_rows[ref])
                self.assertNotIn("**DECLINE**", decision_rows[ref])

        expected_row_snippets = {
            "#2894": (
                "partial artifacts-only finishability",
                "require clean terminal maintained-run evidence",
                "No clean terminal maintained MI300A run is recorded",
                "Run `26213004542` is the latest current-main completed/failure "
                "final-artifact evidence",
                "run `26146663871` is older same-head completed/failure "
                "final-artifact evidence",
                "not completed/success clean evidence",
                "run `25841274346` is non-terminal/provisional snapshot evidence only",
                "without calling it a clean run",
                "clean terminal maintained MI300A Benchmark run is collected and validated",
            ),
            "#125": (
                "current partial Tier-1 calibration with documented caveats",
                "require more Tier-1 calibration/architecture work",
                "memory, compute, and L1/L2 caveats remain",
                "not a new accuracy-report source",
            ),
            "#552": (
                "calibrated `adjust_weights` reporting plus the completed "
                "verification-semantics fix",
                "require raw simulator accuracy/new measurements",
                "Commit `eb093e13` keeps adjusted-weight verification active",
                "raw `sim_ms` rows remain unchanged",
            ),
            "#1793": (
                "calibrated partial BFS evidence",
                "additional BFS tuning/collection for missing larger sizes",
                "12 completed selected-run samples",
                "6 larger sizes are missing",
            ),
            "#1794": (
                "diagnostic-only L1/L2 cache-bandwidth evidence",
                "value-changing loaded hardware/simulator rows above the >2× gate",
                "L1 16 HW/16 sim with max/min 1.148×",
                "L2 16 HW/5 sim with max/min 1.009×",
                "fixed-offset boosters remain diagnostic",
            ),
            "#2540": (
                "calibrated `adjust_weights` acceptance scope",
                "require raw `adjust_weights` accuracy resolution",
                "not changed raw `sim_ms` accuracy",
                "raw simulator accuracy fix or new raw measurements",
            ),
            "#2541": (
                "diagnostic-only L1/L2 acceptance scope",
                "require value-changing cache-bandwidth evidence",
                "diagnostic/hardware-flat",
                "not loaded non-flat value-changing cache-bandwidth evidence",
            ),
            "#268": (
                "close as technically resolved",
                "request specific new hardware probes",
                "`scripts/run_tier1_hardware.sh` are present",
                "no new hardware collection is included here",
            ),
            "#1804": (
                "close the suggested-spec request",
                "request adoption or edits",
                "`docs/proposed_spec.md` is the proposed replacement specification",
                "project-root `spec.md` is not edited",
                "adopt it into project-root `spec.md` or request edits",
            ),
        }
        for ref, snippets in expected_row_snippets.items():
            with self.subTest(ref=ref):
                for snippet in snippets:
                    self.assertIn(snippet, decision_rows[ref])

        for forbidden in (
            "acceptance is complete",
            "final project completion is achieved",
            "clean MI300A Benchmark completion is proven",
            "run `25841274346` is terminal evidence",
            "unless a future refreshed terminal collection proves otherwise",
            "unless refreshed terminal evidence exists",
        ):
            self.assertNotIn(forbidden.lower(), status_text.lower())
            self.assertNotIn(forbidden.lower(), selected_run_readme.lower())

    def test_1795_human_close_is_not_contradicted_by_pending_rows(self) -> None:
        """The carried-forward #1795 close must not conflict with pending rows."""

        status_text = ACCEPTANCE_STATUS.read_text(encoding="utf-8")
        fresh_section = _markdown_section(status_text, "Fresh maintained run attempts")
        rollup_rows = dict(
            _markdown_table_rows(
                _markdown_section(status_text, "Benchmark acceptance topic rollup")
            )
        )
        packet = _markdown_section(status_text, "Final human decision packet")
        decision_rows = _markdown_table_rows_by_ref(packet)

        self.assertIn("closed_by=human", rollup_rows["#1795"])
        self.assertIn("carried forward only for #1795", rollup_rows["#1795"])
        self.assertIn("only carried-forward post-planning human decision", packet)
        self.assertIn("#1795 `CLOSE`", packet)
        self.assertNotIn("#1795", decision_rows)

        pending_close_match = re.search(
            r"no human \*\*CLOSE\*\*/\*\*KEEP OPEN\*\* responses[^.]*\.",
            fresh_section,
        )
        self.assertIsNotNone(pending_close_match)
        pending_close_sentence = pending_close_match.group(0)
        self.assertIn("#268 or #1804", pending_close_sentence)
        self.assertNotIn("#1795", pending_close_sentence)

        self.assertNotRegex(
            status_text,
            r"no human \*\*CLOSE\*\*/\*\*KEEP OPEN\*\* responses[^.]*#1795",
        )
        self.assertNotRegex(
            status_text,
            r"#1795[^.]*\bremain(?:s|ed)? undecided/open\b",
        )

    def test_acceptance_status_keeps_adjust_weights_and_cache_caveats(self) -> None:
        """Known caveats should remain visible with the calibrated/raw distinction."""

        status_text = ACCEPTANCE_STATUS.read_text(encoding="utf-8")
        caveats = _markdown_section(status_text, "Remaining caveats")

        for snippet in (
            "`adjust_weights` has a report-time affine calibration",
            "`sim_scale=0.570694861247`, `fixed_time_ms=0.005321986901`",
            "fitted to 11 completed selected-run samples",
            "raw `sim_ms` rows remain unchanged",
            "59.29% raw mean error",
            "32.10% configured fixed-time-only mean error",
            "best non-negative fixed-time-only lower bound is 25.60%",
            "small-size underprediction and large-size overprediction",
            "should not be read as resolving raw `adjust_weights` accuracy",
            "L1/L2 cache-bandwidth rows remain provenance-sensitive",
            "Archived selected-run fixed-work cache-residency rows are diagnostic "
            "only when their provenance gate passes",
            "L1 16 hardware / 16 completed sim rows",
            "hardware max/min 1.148×",
            "L2 16 hardware / 5 completed sim rows",
            "hardware max/min 1.009×",
            "fixed-offset boosters and trend/shape metrics remain diagnostic",
            "Current work-scaling rows should count as value-changing evidence "
            "only after loaded rows demonstrate non-flat hardware behavior",
            "above the >2× value-change gate",
        ):
            self.assertIn(snippet, caveats)

    def test_acceptance_status_records_adjust_weights_verification_semantics(
        self,
    ) -> None:
        """The post-fix verification semantics should stay distinct from raw accuracy."""

        status_text = ACCEPTANCE_STATUS.read_text(encoding="utf-8")
        caveats = _markdown_section(status_text, "Remaining caveats")
        rollup_row = dict(
            _markdown_table_rows(
                _markdown_section(status_text, "Benchmark acceptance topic rollup")
            )
        )["#552"]
        benchmark_source = BACKPROP_BENCHMARK.read_text(encoding="utf-8")
        benchmark_test = BACKPROP_BENCHMARK_TEST.read_text(encoding="utf-8")

        for snippet in (
            "commit `eb093e13`",
            "`SkipLayerforward` now skips hidden-output verification",
            "still validates adjusted weights",
            "`benchmark_test.go` covers an adjusted-weight mismatch",
        ):
            self.assertIn(snippet, caveats)

        for snippet in (
            "commit `eb093e13`",
            "when `SkipLayerforward` skips the layerforward kernel",
            "hidden-output verification is skipped",
            "adjusted weights remain verified",
            "adjusted-weight mismatch regression test",
        ):
            self.assertIn(snippet, rollup_row)

        for snippet in (
            "The raw `sim_ms` rows remain unchanged",
            "should not be read as resolving raw `adjust_weights` accuracy",
            "This raw-accuracy caveat is separate",
        ):
            self.assertIn(snippet, caveats)

        verify_body = _active_go_function_body(
            benchmark_source,
            r"^func\s+\(b \*Benchmark\)\s+Verify\s*\(\s*\)",
            "Benchmark.Verify",
        )
        self.assertRegex(
            verify_body,
            r"(?s)b\.cpuBackprop\(\).*"
            r"if\s+!b\.SkipLayerforward\s*\{\s*"
            r"b\.verifyHiddenOutput\(\)\s*\}.*"
            r"b\.verifyAdjustedWeights\(\)",
        )

        skip_hidden_test_body = _active_go_function_body(
            benchmark_test,
            r"^func\s+TestVerifySkipsHiddenOutputWhenLayerforwardSkipped"
            r"\s*\(\s*t \*testing\.T\s*\)",
            "TestVerifySkipsHiddenOutputWhenLayerforwardSkipped",
        )
        self.assertRegex(
            skip_hidden_test_body,
            r"(?m)^\s*benchmark := newVerificationBenchmarkForTest\(4, 3\)\s*$",
        )
        self.assertRegex(
            skip_hidden_test_body,
            r"(?m)^\s*benchmark\.SkipLayerforward = true\s*$",
        )
        self.assertRegex(
            skip_hidden_test_body,
            r"(?m)^\s*benchmark\.hidden = nil\s*$",
        )
        self.assertRegex(
            skip_hidden_test_body,
            r"(?m)^\s*benchmark\.Verify\(\)\s*$",
        )
        self.assertNotIn("expectPanicContaining", skip_hidden_test_body)

        adjusted_weight_test_body = _active_go_function_body(
            benchmark_test,
            r"^func\s+TestVerifyChecksAdjustedWeightsWhenLayerforwardSkipped"
            r"\s*\(\s*t \*testing\.T\s*\)",
            "TestVerifyChecksAdjustedWeightsWhenLayerforwardSkipped",
        )
        self.assertRegex(
            adjusted_weight_test_body,
            r"(?m)^\s*benchmark := newVerificationBenchmarkForTest\(4, 3\)\s*$",
        )
        self.assertRegex(
            adjusted_weight_test_body,
            r"(?m)^\s*benchmark\.SkipLayerforward = true\s*$",
        )
        self.assertRegex(
            adjusted_weight_test_body,
            r"(?m)^\s*benchmark\.hidden = nil\s*$",
        )
        self.assertRegex(
            adjusted_weight_test_body,
            r"(?m)^\s*benchmark\.weights\[0\] \+= 1\s*$",
        )
        self.assertRegex(
            adjusted_weight_test_body,
            r"(?s)expectPanicContaining\s*\(\s*t,\s*func\(\)\s*"
            r"\{\s*benchmark\.Verify\(\)\s*\}\s*,\s*"
            r"\"adjusted-weight verification failed\"\s*\)",
        )

    def test_acceptance_status_references_deleted_workflow_api_triage(self) -> None:
        """The stale deleted workflow record should remain bounded by hygiene facts."""

        status_text = ACCEPTANCE_STATUS.read_text(encoding="utf-8")
        workflow_section = _markdown_section(status_text, "Maintained workflow contract")
        rollup_row = dict(
            _markdown_table_rows(
                _markdown_section(status_text, "Benchmark acceptance topic rollup")
            )
        )["#1795"]
        hygiene_text = WORKFLOW_HYGIENE.read_text(encoding="utf-8")

        for snippet in (
            ".github/workflows/benchmark-tier-runner.yml",
            "260768869",
            "disabled manually on 2026-05-13",
            "absent from tracked `main`",
            "benign only while no new runs appear",
        ):
            self.assertIn(snippet, workflow_section)
            self.assertIn(snippet, rollup_row)

        for snippet in (
            "id:    260768869",
            "path:  .github/workflows/benchmark-tier-runner.yml",
            "disabled_manually",
            "deleted file still returned HTTP 404 through the contents API",
            "No new runs appear for workflow id `260768869` after the 2026-05-13",
        ):
            self.assertIn(snippet, hygiene_text)

    def test_acceptance_package_user_docs_do_not_leak_process_labels(self) -> None:
        """User-facing package docs should not expose process labels."""

        offenders: list[str] = []
        for path in USER_FACING_ACCEPTANCE_PACKAGE_DOCS:
            text = path.read_text(encoding="utf-8")
            rel_path = path.relative_to(REPO_ROOT).as_posix()
            for line_number, line in enumerate(text.splitlines(), start=1):
                for label, pattern in FORBIDDEN_USER_FACING_PROCESS_LABEL_PATTERNS.items():
                    match = pattern.search(line)
                    if match is not None:
                        offenders.append(
                            f"{rel_path}:{line_number}: {label}: "
                            f"{match.group(0)!r}"
                        )

        self.assertEqual(offenders, [])


def _git_tracked_files(pathspec: str) -> tuple[str, ...]:
    result = subprocess.run(
        ["git", "ls-files", "--", pathspec],
        check=True,
        cwd=REPO_ROOT,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    return tuple(line for line in result.stdout.splitlines() if line)


def _read_json(path: Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def _read_tsv(path: Path) -> list[dict[str, str]]:
    lines = path.read_text(encoding="utf-8").splitlines()
    header = lines[0].split("\t")
    return [dict(zip(header, line.split("\t"), strict=True)) for line in lines[1:]]


def _markdown_section(markdown: str, heading: str) -> str:
    marker = f"## {heading}"
    start = markdown.find(marker)
    if start < 0:
        raise AssertionError(f"missing Markdown section: {heading}")
    next_heading = markdown.find("\n## ", start + len(marker))
    if next_heading < 0:
        return markdown[start:]
    return markdown[start:next_heading]


def _paragraphs_containing(markdown: str, needle: str) -> tuple[str, ...]:
    return tuple(
        paragraph
        for paragraph in re.split(r"\n\s*\n", markdown)
        if needle in paragraph
    )


def _markdown_table_rows(markdown: str) -> list[tuple[str, str]]:
    rows: list[tuple[str, str]] = []
    for line in markdown.splitlines():
        if not line.startswith("|"):
            continue
        cells = [cell.strip() for cell in line.strip().strip("|").split("|")]
        if len(cells) < 2 or not re.fullmatch(r"#\d+", cells[0]):
            continue
        rows.append((cells[0], cells[1]))
    return rows


def _markdown_table_rows_by_ref(markdown: str) -> dict[str, str]:
    rows: dict[str, str] = {}
    for line in markdown.splitlines():
        if not line.startswith("|"):
            continue
        cells = [cell.strip() for cell in line.strip().strip("|").split("|")]
        if len(cells) < 2 or not re.fullmatch(r"#\d+", cells[0]):
            continue
        rows[cells[0]] = " | ".join(cells[1:])
    return rows


def _active_go_function_body(source: str, signature_pattern: str, label: str) -> str:
    active_source = _strip_go_comments(source)
    match = re.search(signature_pattern + r"\s*\{", active_source, re.MULTILINE)
    if match is None:
        raise AssertionError(f"active Go function not found: {label}")

    opening_brace = active_source.find("{", match.start())
    if opening_brace < 0:
        raise AssertionError(f"active Go function has no body: {label}")

    depth = 0
    for index in range(opening_brace, len(active_source)):
        char = active_source[index]
        if char == "{":
            depth += 1
        elif char == "}":
            depth -= 1
            if depth == 0:
                return active_source[opening_brace + 1 : index]

    raise AssertionError(f"active Go function body is unterminated: {label}")


def _strip_go_comments(source: str) -> str:
    output: list[str] = []
    index = 0
    in_line_comment = False
    in_block_comment = False

    while index < len(source):
        char = source[index]
        next_char = source[index + 1] if index + 1 < len(source) else ""

        if in_line_comment:
            if char == "\n":
                in_line_comment = False
                output.append(char)
            index += 1
            continue

        if in_block_comment:
            if char == "*" and next_char == "/":
                in_block_comment = False
                index += 2
                continue
            if char == "\n":
                output.append(char)
            index += 1
            continue

        if char == "/" and next_char == "/":
            in_line_comment = True
            index += 2
            continue

        if char == "/" and next_char == "*":
            in_block_comment = True
            index += 2
            continue

        output.append(char)
        index += 1

    return "".join(output)


def _workflow_dispatch_block(workflow_text: str) -> str:
    start = workflow_text.find("  workflow_dispatch:")
    if start < 0:
        raise AssertionError("workflow_dispatch block not found")
    next_top_level = re.search(r"\n[a-zA-Z_-][^\n]*:\s*\n", workflow_text[start + 1 :])
    if next_top_level is None:
        return workflow_text[start:]
    return workflow_text[start : start + 1 + next_top_level.start()]


def _workflow_job_block(workflow_text: str, job_name: str) -> str:
    match = re.search(rf"(?m)^  {re.escape(job_name)}:\s*$", workflow_text)
    if match is None:
        raise AssertionError(f"workflow job not found: {job_name}")
    next_job = re.search(r"(?m)^  [A-Za-z0-9_-]+:\s*$", workflow_text[match.end() :])
    if next_job is None:
        return workflow_text[match.start() :]
    return workflow_text[match.start() : match.end() + next_job.start()]


def _status_counter_from_evidence(evidence: dict) -> Counter[str]:
    statuses: Counter[str] = Counter()
    for row in evidence["rows"]:
        for entry in row["entries"]:
            statuses[entry["status"]] += 1
    return statuses
