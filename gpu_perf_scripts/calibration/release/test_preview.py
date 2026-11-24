import json
import unittest
from pathlib import Path
from tempfile import TemporaryDirectory
from unittest.mock import patch

from preview import watch


class PreviewTest(unittest.TestCase):
    def test_live_report_keeps_paths_and_fetches_only_new_artifacts(self):
        shape = [64, 64, 1, 16, 16, 1]
        reference = {
            "series": [
                {
                    "benchmark": "example",
                    "sample": "example",
                    "axis": "size",
                    "parameters": {"block_size": 16 + i},
                    "cases": [
                        {
                            "case": i,
                            "x": 64,
                            "launches": [
                                ["kernel", *shape, 2000]
                                for _ in range(1 if i == 0 else 49)
                            ],
                        }
                    ],
                }
                for i in range(2)
            ]
        }
        plan = {
            "version": "test",
            "revision": "revision",
            "series": [
                {"id": i, "benchmark": "example", "sample": "example"} for i in range(2)
            ],
        }
        fixtures = {
            100: plan,
            **{
                101 + i: {
                    "started_at": "2026-11-24T16:00:00Z",
                    "cases": {
                        str(i): {
                            "status": "complete",
                            "launches": [
                                ["kernel", *shape, 1500]
                                for _ in range(1 if i == 0 else 49)
                            ],
                        }
                    },
                }
                for i in range(2)
            },
        }
        with TemporaryDirectory() as folder:
            root = Path(folder)
            template = root / "template"
            template.mkdir()
            html = """<main>
    <header class="report-header">Report</header>
</main>"""
            (template / "index.html").write_text(html)
            for name in ("report.css", "summary.js", "report.js"):
                (template / name).touch()
            output = root / "out"

            def inspect_live(_):
                data = json.loads((output / "data.json").read_text())
                self.assertEqual(data["progress"]["queued"], 1)
                self.assertEqual(data["progress"]["unsuccessful"], 0)
                self.assertTrue(data["report"]["live"])
                self.assertEqual(data["data"][0]["values"]["mgpusim"], 1500)
                page = json.loads((output / "example/part-2/data.json").read_text())
                self.assertEqual(page["data"], [])
                self.assertTrue(page["report"]["live"])

            runs = [
                json.dumps({"head_sha": "revision", "status": state}).encode()
                for state in ("in_progress", "completed")
            ]
            with (
                patch("preview.github", side_effect=runs),
                patch(
                    "preview.artifact_ids",
                    side_effect=[
                        {"calibration-plan": 100, "series-0": 101},
                        {"calibration-plan": 100, "series-0": 101, "series-1": 102},
                    ],
                ),
                patch(
                    "preview.artifact_json", side_effect=lambda _, i, __: fixtures[i]
                ) as download,
                patch("preview.time.sleep", side_effect=inspect_live),
            ):
                watch(reference, template, output, "sarchlab/mgpusim", 123, 1, None)
            self.assertEqual(download.call_count, 3)
            final = json.loads((output / "data.json").read_text())
            self.assertEqual(final["progress"]["completed"], 2)
            self.assertFalse(final["report"]["live"])
            self.assertEqual(len(final["data"]), 48)
            self.assertIn("48 of 50 available charts", final["report"]["description"])
            page = json.loads((output / "example/part-2/data.json").read_text())
            self.assertEqual(page["data"][0]["values"]["mgpusim"], 1500)
            self.assertFalse(page["report"]["live"])


if __name__ == "__main__":
    unittest.main()
