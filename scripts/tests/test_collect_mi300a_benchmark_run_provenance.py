from __future__ import annotations

import importlib.util
import json
import tempfile
import unittest
import zipfile
from pathlib import Path
from unittest import mock


REPO_ROOT = Path(__file__).resolve().parents[2]
MODULE_PATH = REPO_ROOT / "scripts" / "collect_mi300a_benchmark_run_provenance.py"


def load_module():
    spec = importlib.util.spec_from_file_location(
        "collect_mi300a_benchmark_run_provenance",
        MODULE_PATH,
    )
    assert spec is not None
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


class CollectMI300ABenchmarkRunProvenanceTest(unittest.TestCase):
    def test_run_metadata_and_jobs_gh_commands_include_selected_repo(self) -> None:
        module = load_module()
        repo = "example/forked-mgpusim-dev"
        run_id = "25841274346"

        with mock.patch.object(module, "gh_json", return_value={}) as gh_json:
            module.collect_run_view(repo, run_id)

        self.assertEqual(
            gh_json.call_args.args[0],
            [
                "gh",
                "run",
                "view",
                run_id,
                "--repo",
                repo,
                "--json",
                module.RUN_VIEW_FIELDS,
            ],
        )

        with mock.patch.object(module, "gh_json", return_value={"jobs": []}) as gh_json:
            module.collect_jobs(repo, run_id)

        self.assertEqual(
            gh_json.call_args.args[0],
            [
                "gh",
                "run",
                "view",
                run_id,
                "--repo",
                repo,
                "--json",
                module.JOBS_FIELDS,
            ],
        )

    def test_main_uses_one_repo_for_run_jobs_and_artifacts(self) -> None:
        module = load_module()
        repo = "example/forked-mgpusim-dev"
        run_id = "25841274346"
        commands: list[list[str]] = []

        def fake_run_command(args, *, binary_stdout_path=None):
            self.assertIsNone(binary_stdout_path)
            command = list(args)
            commands.append(command)
            is_run_view = command[:3] == ["gh", "run", "view"]
            if is_run_view and command[-1] == module.RUN_VIEW_FIELDS:
                return json.dumps({"status": "completed", "conclusion": "success"})
            if is_run_view and command[-1] == module.JOBS_FIELDS:
                return json.dumps({"jobs": []})
            if command[:2] == ["gh", "api"]:
                self.assertEqual(
                    command[2],
                    f"repos/{repo}/actions/runs/{run_id}/artifacts",
                )
                return ""
            self.fail(f"unexpected command: {command}")

        with tempfile.TemporaryDirectory() as tmp:
            with mock.patch.object(module, "run_command", side_effect=fake_run_command):
                result = module.main(
                    [
                        "--run-id",
                        run_id,
                        "--repo",
                        repo,
                        "--output-dir",
                        tmp,
                        "--skip-raw-zips",
                    ]
                )

        self.assertEqual(result, 0)
        run_view_commands = [
            command for command in commands if command[:3] == ["gh", "run", "view"]
        ]
        self.assertEqual(len(run_view_commands), 2)
        for command in run_view_commands:
            self.assertIn("--repo", command)
            self.assertEqual(command[command.index("--repo") + 1], repo)
        self.assertTrue(any(command[:2] == ["gh", "api"] for command in commands))

    def test_non_terminal_readme_preserves_pending_state_without_overclaiming(self) -> None:
        module = load_module()
        with tempfile.TemporaryDirectory() as tmp:
            output_dir = Path(tmp)
            text = module.render_readme(
                run_id="25841274346",
                repo="sarchlab/mgpusim-dev",
                output_dir=output_dir,
                run_view={
                    "status": "queued",
                    "conclusion": "",
                    "workflowName": "MI300A Benchmark",
                    "event": "workflow_dispatch",
                    "headBranch": "main",
                    "headSha": "e0ef96c57e042c636e2b94db8f294ff8a574bb78",
                    "url": "https://github.com/sarchlab/mgpusim-dev/actions/runs/25841274346",
                },
                artifacts=[{"id": 1, "name": "validation-summary"}],
                job_summary={
                    "total_jobs": 2,
                    "status_counts": {"completed": 1, "in_progress": 1},
                    "conclusion_counts": {"success": 1, "empty": 1},
                },
                extracted_count=3,
                raw_zip_count=1,
                generated_at="2026-05-14T04:40:00Z",
                command="python3 scripts/collect_mi300a_benchmark_run_provenance.py --run-id 25841274346",
            )

        self.assertIn("Evidence state: `non_terminal_snapshot`", text)
        self.assertIn("This is **not terminal evidence**", text)
        self.assertIn("no clean pass, no-result, failure", text)
        self.assertIn("cancelled, or complete finishability conclusion", text)
        self.assertIn("Artifacts are provisional", text)
        self.assertNotIn("clean finishability pass", text.lower())

    def test_job_summary_counts_benchmark_statuses_and_run_steps(self) -> None:
        module = load_module()
        rows, summary = module.summarize_jobs(
            {
                "jobs": [
                    {
                        "databaseId": 1,
                        "name": "Validation: full regular matrix",
                        "status": "completed",
                        "conclusion": "success",
                        "startedAt": "2026-05-14T00:00:00Z",
                        "completedAt": "2026-05-14T00:00:05Z",
                        "steps": [],
                    },
                    {
                        "databaseId": 2,
                        "name": "Tier 1 Bench: l1_cache_bw",
                        "status": "completed",
                        "conclusion": "success",
                        "startedAt": "2026-05-14T00:00:10Z",
                        "completedAt": "2026-05-14T00:01:10Z",
                        "steps": [
                            {
                                "name": "Run benchmark sizes",
                                "status": "completed",
                                "conclusion": "success",
                            }
                        ],
                    },
                    {
                        "databaseId": 3,
                        "name": "Tier 1 Bench: shared_bw",
                        "status": "in_progress",
                        "conclusion": "",
                        "startedAt": "2026-05-14T00:00:10Z",
                        "completedAt": "0001-01-01T00:00:00Z",
                        "steps": [
                            {
                                "name": "Run benchmark sizes",
                                "status": "in_progress",
                                "conclusion": "",
                            }
                        ],
                    },
                ]
            }
        )

        by_name = {row["name"]: row for row in rows}
        self.assertEqual(summary["total_jobs"], 3)
        self.assertEqual(summary["benchmark_jobs"], 2)
        self.assertEqual(summary["status_counts"], {"completed": 2, "in_progress": 1})
        self.assertEqual(summary["benchmark_conclusion_or_status_counts"], {"in_progress": 1, "success": 1})
        self.assertEqual(by_name["Tier 1 Bench: l1_cache_bw"]["duration_sec"], 60)
        self.assertEqual(by_name["Tier 1 Bench: shared_bw"]["run_step_status"], "in_progress")

    def test_raw_zip_extraction_inventory_hashes_curated_files(self) -> None:
        module = load_module()
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            raw_dir = root / "raw-zips"
            raw_dir.mkdir()
            zip_path = raw_dir / "validation-summary-11.zip"
            with zipfile.ZipFile(zip_path, "w") as zf:
                zf.writestr("regular_matrix_validation.json", json.dumps({"status": "pass"}))
            rows = module.extract_artifacts(
                [{"id": 11, "name": "validation-summary"}],
                raw_dir,
                root / "extracted",
                "curated",
                {"validation-summary"},
            )

            self.assertEqual(len(rows), 1)
            self.assertEqual(rows[0]["artifact_id"], "11")
            self.assertEqual(
                rows[0]["relative_path"],
                "extracted/validation-summary/regular_matrix_validation.json",
            )
            self.assertEqual(len(rows[0]["sha256"]), 64)


if __name__ == "__main__":
    unittest.main()
