from __future__ import annotations

import contextlib
import csv
import importlib.util
import io
import tempfile
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
MODULE_PATH = REPO_ROOT / "scripts" / "generate_mi300a_run_provenance.py"


def load_module():
    spec = importlib.util.spec_from_file_location(
        "generate_mi300a_run_provenance",
        MODULE_PATH,
    )
    assert spec is not None
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


class GenerateMi300ARunProvenanceTest(unittest.TestCase):
    def write_csv(
        self,
        path: Path,
        fieldnames: list[str],
        rows: list[dict[str, object]],
    ) -> None:
        path.parent.mkdir(parents=True, exist_ok=True)
        with path.open("w", newline="", encoding="utf-8") as fh:
            writer = csv.DictWriter(fh, fieldnames=fieldnames)
            writer.writeheader()
            writer.writerows(rows)

    def write_default_contract_fixture(
        self,
        provenance_dir: Path,
        run_id: str,
    ) -> None:
        prefix = f"mi300a_regular_benchmark_run_{run_id}"
        timestamp = "20260511T000000Z"
        self.write_csv(
            provenance_dir / f"{prefix}_problem_size_rows_{timestamp}.csv",
            ["workflow_benchmark", "row_observation"],
            [
                {
                    "workflow_benchmark": "fixture_bench",
                    "row_observation": (
                        "known_finished_from_downloaded_benchmark_results_csv_and_run_log_ok"
                    ),
                },
                {
                    "workflow_benchmark": "fixture_bench",
                    "row_observation": "known_timed_out_7200s_from_run_log",
                },
            ],
        )
        self.write_csv(
            provenance_dir / f"{prefix}_log_event_summary_{timestamp}.csv",
            ["event_type", "timeout_sec"],
            [
                {"event_type": "finished", "timeout_sec": ""},
                {"event_type": "timed_out_7200s", "timeout_sec": "7200"},
            ],
        )
        self.write_csv(
            provenance_dir / f"{prefix}_benchmark_summary_{timestamp}.csv",
            [
                "workflow_benchmark",
                "expected_plan_entries",
                "csv_data_rows",
                "csv_rows_unpaired_to_unique_plan_entries",
                "exceeded_21600s_benchmark_tier_budget",
            ],
            [
                {
                    "workflow_benchmark": "fixture_bench",
                    "expected_plan_entries": "2",
                    "csv_data_rows": "1",
                    "csv_rows_unpaired_to_unique_plan_entries": "0",
                    "exceeded_21600s_benchmark_tier_budget": "false",
                }
            ],
        )
        terminal_md = provenance_dir / f"{prefix}_terminal_observation_{timestamp}.md"
        terminal_md.write_text(
            "Run conclusion `failure`; not a benchmark-success or "
            "complete-finishability claim.\n",
            encoding="utf-8",
        )

    def test_default_regular_timeout_contract_is_7200s_and_21600s(self) -> None:
        module = load_module()
        with tempfile.TemporaryDirectory() as tmp:
            provenance_dir = Path(tmp) / "benchmark-comparison" / "provenance"
            run_id = "999000111"
            self.write_default_contract_fixture(provenance_dir, run_id)
            old_dir = module.PROVENANCE_DIR
            old_constants = dict(module.RUN_CONSTANTS)
            module.PROVENANCE_DIR = provenance_dir
            module.RUN_CONSTANTS = {
                run_id: {
                    "total_plan_rows": 2,
                    "finished": 1,
                    "timed_out_7200s": 1,
                    "crashed": 0,
                    "unknown": 0,
                    "log_finished": 1,
                    "log_timed_out": 1,
                    "log_crashed": 0,
                    "bs_plan_total": 2,
                    "bs_csv_total": 1,
                    "bs_unpaired": 0,
                }
            }
            stdout = io.StringIO()
            try:
                with contextlib.redirect_stdout(stdout):
                    module.run_reconciliation(run_id, None)
            finally:
                module.PROVENANCE_DIR = old_dir
                module.RUN_CONSTANTS = old_constants

        text = stdout.getvalue()
        self.assertIn("timed_out_7200s=1", text)
        self.assertIn("no benchmark-tier job exceeded 21600s/360m budget", text)
        self.assertIn("7200s invocation timeouts", text)


if __name__ == "__main__":
    unittest.main()
