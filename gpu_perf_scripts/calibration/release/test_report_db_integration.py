"""Workflow writes against a real calibration-report database.

Skipped unless CALIBRATION_REPORT_TEST_DSN (the cr_ingest login) and
CALIBRATION_REPORT_TEST_ADMIN_DSN (a role that can read schema cr) point at a
database with the sarchlab/calibration-report migrations applied.
"""

import json
import os
import tempfile
import unittest
import uuid
from argparse import Namespace
from pathlib import Path
from unittest.mock import patch

import psycopg

import report_db
from workflow import finalize, plan, sweep

INGEST = os.environ.get("CALIBRATION_REPORT_TEST_DSN")
ADMIN = os.environ.get("CALIBRATION_REPORT_TEST_ADMIN_DSN")
SHAPE = [64, 64, 1, 16, 16, 1]
CFD_NAMES = [
    "copy_variables_kernel",
    "cfd_device::compute_step_factor_kernel",
    *("cfd_device::compute_flux_kernel", "cfd_device::rk_update_kernel") * 3,
] * 10


def example(block_size):
    return {
        "benchmark": "example",
        "sample": "example",
        "axis": "size",
        "parameters": {"block_size": block_size},
        "cases": [
            {"case": block_size * 10 + i, "x": i * 64, "launches": [["kernel(float*)", *SHAPE, 2000]]}
            for i in range(1, 4)
        ],
    }


REFERENCE = {
    "series": [
        example(16),
        example(32),
        {
            "benchmark": "altis_cfd",
            "sample": "altis_cfd",
            "axis": "size",
            "parameters": {"block_size": 256},
            "cases": [
                {"case": 900 + i, "x": i * 500,
                 "launches": [[f"{name}(int)", *SHAPE, 1000 + j] for j, name in enumerate(CFD_NAMES)]}
                for i in range(1, 3)
            ],
        },
    ]
}
COMPLETE = {"status": "complete", "launches": [["kernel", *SHAPE, 1500]]}
CFD_COMPLETE = {
    "status": "complete",
    "launches": [[name.removeprefix("cfd_device::"), *SHAPE, 900] for name in CFD_NAMES],
}


@unittest.skipUnless(INGEST and ADMIN, "needs CALIBRATION_REPORT_TEST_DSN and _ADMIN_DSN")
class DatabaseTest(unittest.TestCase):
    def setUp(self):
        patcher = patch.dict(os.environ, {"CALIBRATION_REPORT_DSN": INGEST})
        patcher.start()
        self.addCleanup(patcher.stop)
        folder = tempfile.TemporaryDirectory()
        self.addCleanup(folder.cleanup)
        self.folder = Path(folder.name)

    def query(self, sql, *params):
        with psycopg.connect(ADMIN) as connection:
            return connection.execute(sql, params).fetchall()

    def plan(self):
        tag = f"test-{uuid.uuid4().hex[:12]}"
        with patch("builtins.print") as output:
            plan(REFERENCE, Namespace(benchmark=None, tag=tag, run_id="1", version=tag,
                                      revision="sha"))
        lines = dict(c.args[0].split("=", 1) for c in output.call_args_list)
        self.assertEqual(lines["run_key"], f"mgpusim-mi300x-{tag}")
        self.assertEqual([row["id"] for row in json.loads(lines["series"])], [0, 1, 2])
        return lines

    def sweep(self, key, series, results):
        output = self.folder / f"series-{series}"
        with patch("workflow.run_case", side_effect=results):
            sweep(REFERENCE, Namespace(series=series, binary=Path("sample"), output=output,
                                       run_key=key, version=key, revision="sha"))
        for file in output.glob("runs"):
            file.rmdir()

    def finalize(self, key, started_at, result):
        finalize(REFERENCE, Namespace(artifacts=self.folder, benchmark=None, run_key=key,
                                      started_at=started_at, sweep_result=result))

    def run_state(self, key):
        ((state, progress),) = self.query(
            "SELECT state, progress FROM cr.runs WHERE key = %s", key)
        return state, progress

    def test_live_run(self):
        lines = self.plan()
        key = lines["run_key"]
        state, progress = self.run_state(key)
        self.assertEqual((state, progress["total"], progress["queued"]), ("running", 8, 8))
        self.assertEqual(
            self.query("""SELECT b.key, array_agg(p.key ORDER BY p.ordinal) FROM cr.benchmarks b
                          JOIN cr.runs r ON r.id = b.run_id JOIN cr.params p ON p.benchmark_id = b.id
                          WHERE r.key = %s GROUP BY b.key ORDER BY b.key""", key),
            [("altis_cfd", ["size", "launch", "block_size", "launch_group"]),
             ("example", ["size", "launch", "block_size"])],
        )

        timeout = {"status": "timeout", "reason": "Series budget expired during this configuration."}
        failed = {"status": "failed", "reason": "panic: " + "é" * 600}
        mismatch = {"status": "complete", "launches": [["other", *SHAPE, 1]]}
        self.sweep(key, 0, [COMPLETE, mismatch, timeout])
        self.sweep(key, 1, [COMPLETE, failed, COMPLETE])
        self.sweep(key, 2, [CFD_COMPLETE, {"status": "complete", "launches": []}])
        self.assertEqual(self.run_state(key)[1], progress)  # Sweeps never write progress.
        self.finalize(key, lines["started_at"], "success")

        state, progress = self.run_state(key)
        self.assertEqual(state, "completed")
        self.assertEqual(
            {k: progress[k] for k in ("started_at", "total", "completed", "unsuccessful")},
            {"started_at": lines["started_at"], "total": 8, "completed": 4, "unsuccessful": 4},
        )
        rows = self.query(
            """SELECT b.key, count(DISTINCT pl.id), count(DISTINCT pt.id),
                      count(o.value) FILTER (WHERE s.key = 'mgpusim'),
                      array_agg(DISTINCT o.state) FILTER (WHERE o.state IS NOT NULL)
               FROM cr.runs r JOIN cr.benchmarks b ON b.run_id = r.id
               JOIN cr.plots pl ON pl.benchmark_id = b.id
               JOIN cr.points pt ON pt.plot_id = pl.id
               JOIN cr.observations o ON o.point_id = pt.id
               JOIN cr.series s ON s.id = o.series_id
               WHERE r.key = %s GROUP BY b.key ORDER BY b.key""", key)
        self.assertEqual(rows, [
            ("altis_cfd", 80, 160, 80, ["unsupported"]),
            ("example", 2, 6, 3, ["failed", "timeout", "unsupported"]),
        ])
        labels = self.query(
            """SELECT DISTINCT pl.params ->> 'launch_group' FROM cr.plots pl
               JOIN cr.runs r ON r.id = pl.run_id JOIN cr.benchmarks b ON b.id = pl.benchmark_id
               WHERE r.key = %s AND b.key = 'altis_cfd'""", key)
        self.assertEqual(len(labels), 9)
        # The long failure reason is stored truncated and kept whole in the artifact.
        ((reason,),) = self.query(
            """SELECT o.reason FROM cr.observations o JOIN cr.points pt ON pt.id = o.point_id
               JOIN cr.plots pl ON pl.id = pt.plot_id JOIN cr.runs r ON r.id = pl.run_id
               WHERE r.key = %s AND o.state = 'failed'""", key)
        self.assertLessEqual(len(reason.encode()), 1000)
        self.assertTrue(failed["reason"].startswith(reason[:-1]))
        saved = json.loads((self.folder / "series-1" / "results.json").read_text())
        self.assertIn(failed["reason"], [case.get("reason") for case in saved["cases"].values()])

    def test_rerun_reopens_a_failed_run(self):
        lines = self.plan()
        key = lines["run_key"]
        self.sweep(key, 0, [COMPLETE] * 3)
        self.finalize(key, lines["started_at"], "failure")
        self.assertEqual(self.run_state(key)[0], "failed")

        # Re-running a failed series job reopens the run before writing.
        self.sweep(key, 1, [COMPLETE] * 3)
        self.assertEqual(self.run_state(key)[0], "running")
        self.sweep(key, 2, [CFD_COMPLETE] * 2)
        self.finalize(key, lines["started_at"], "success")
        state, progress = self.run_state(key)
        self.assertEqual((state, progress["completed"]), ("completed", 8))

        # A completed run is terminal and the error is not retried.
        with self.assertRaises(psycopg.errors.ObjectNotInPrerequisiteState):
            self.sweep(key, 0, [COMPLETE] * 3)

    def test_a_stuck_statement_is_cancelled_as_retryable(self):
        with (
            patch.object(report_db, "STATEMENT_TIMEOUT", "SET LOCAL statement_timeout = '50ms'"),
            patch.object(report_db, "RETRY_SECONDS", 0),
            self.assertRaises(psycopg.errors.QueryCanceled) as caught,
        ):
            report_db.call("SELECT pg_sleep(5)")
        self.assertTrue(report_db.retryable(caught.exception))


if __name__ == "__main__":
    unittest.main()
