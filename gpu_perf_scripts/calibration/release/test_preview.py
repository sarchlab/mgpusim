import json
import unittest
from pathlib import Path
from tempfile import TemporaryDirectory
from unittest.mock import patch

from build_site import render_site
from preview import watch


class PreviewTest(unittest.TestCase):
    def test_live_report_keeps_paths_and_fetches_only_new_artifacts(self):
        shape = [64, 64, 1, 16, 16, 1]
        reference = {
            "series": [
                {
                    "benchmark": "example" if i == 0 else "many_launches",
                    "sample": "example" if i == 0 else "many_launches",
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
                {"id": 0, "benchmark": "example", "sample": "example"},
                {"id": 1, "benchmark": "many_launches", "sample": "many_launches"},
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
                self.assertEqual(
                    {point["benchmark"] for point in data["data"]}, {"example"}
                )
                self.assertEqual(list(output.rglob("data.json")), [output / "data.json"])

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
            self.assertEqual(len(final["data"]), 50)
            self.assertEqual(set(final["benchmarks"]), {"example", "many_launches"})
            self.assertEqual(
                sum(point["benchmark"] == "many_launches" for point in final["data"]),
                49,
            )
            self.assertEqual(
                final["data"][-1]["non_scaling_params"]["launch"], "#49 kernel"
            )
            self.assertEqual((output / "index.html").read_text(), html)
            self.assertEqual(
                {file.name for file in output.iterdir()},
                {"index.html", "report.css", "summary.js", "report.js", "data.json"},
            )

    def test_report_copies_the_template_and_writes_only_data(self):
        with TemporaryDirectory() as folder:
            root = Path(folder)
            template = root / "template"
            template.mkdir()
            (template / "index.html").write_bytes(b"<!doctype html><main>Report</main>")
            (template / "report.js").write_bytes(b"// standard renderer")
            data = {"report": {"live": True}, "benchmarks": {}, "data": []}
            output = root / "report"
            render_site(data, template, output)
            for name in ("index.html", "report.js"):
                self.assertEqual(
                    (output / name).read_bytes(), (template / name).read_bytes()
                )
            self.assertEqual(json.loads((output / "data.json").read_text()), data)
            self.assertEqual(
                {file.name for file in output.iterdir()},
                {"index.html", "report.js", "data.json"},
            )


if __name__ == "__main__":
    unittest.main()
