"""Tests for ``scripts/generate_accuracy_summary.py``."""

from __future__ import annotations

import base64
import io
import os
import struct
import sys
import tempfile
import unittest
import zlib
from contextlib import redirect_stdout
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPTS_DIR = REPO_ROOT / "scripts"
if str(SCRIPTS_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPTS_DIR))

import generate_accuracy_summary as gas  # noqa: E402


def _minimal_png_bytes(*, width: int = 1, height: int = 1) -> bytes:
    """Return bytes for a tiny valid PNG (single transparent pixel)."""

    def chunk(tag: bytes, data: bytes) -> bytes:
        return (
            struct.pack(">I", len(data))
            + tag
            + data
            + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)
        )

    signature = b"\x89PNG\r\n\x1a\n"
    ihdr = struct.pack(">IIBBBBB", width, height, 8, 6, 0, 0, 0)
    raw = b"".join(b"\x00" + b"\x00\x00\x00\x00" * width for _ in range(height))
    idat = zlib.compress(raw)
    return signature + chunk(b"IHDR", ihdr) + chunk(b"IDAT", idat) + chunk(
        b"IEND", b""
    )


SAMPLE_REPORT = """\
# Benchmark Accuracy Report

Comparison of MGPUSim simulated runs against MI300A hardware.

## Tier-2 Accuracy

- Matched samples: **8**
- Mean err: **12.34%**

## Per-Benchmark Metrics

| Benchmark | Samples | Mean err | Chart |
| --- | ---: | ---: | --- |
| atax | 5 | 39.27% | [atax_comparison.png](docs/figures/atax_comparison.png) |
| bfs | 7 | 99.15% | [bfs_comparison.png](docs/figures/bfs_comparison.png) |
"""


class FindImageReferencesTest(unittest.TestCase):
    def test_extracts_png_links_in_order(self):
        refs = gas.find_image_references(SAMPLE_REPORT)
        self.assertEqual(
            refs,
            [
                "docs/figures/atax_comparison.png",
                "docs/figures/bfs_comparison.png",
            ],
        )

    def test_dedupes_repeated_references(self):
        text = "[a](docs/figures/x.png) and again [a](docs/figures/x.png)"
        self.assertEqual(
            gas.find_image_references(text), ["docs/figures/x.png"]
        )

    def test_ignores_non_png_links(self):
        text = "[csv](data/foo.csv) [img](docs/figures/y.png)"
        self.assertEqual(
            gas.find_image_references(text), ["docs/figures/y.png"]
        )


class BuildImageUrisTest(unittest.TestCase):
    def test_resolves_relative_to_repo_root(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            figures = root / "docs" / "figures"
            figures.mkdir(parents=True)
            png = figures / "atax_comparison.png"
            png.write_bytes(_minimal_png_bytes())

            uris = gas.build_image_uris(
                ["docs/figures/atax_comparison.png"],
                repo_root=root,
                figures_dir=figures,
            )
            self.assertIn("docs/figures/atax_comparison.png", uris)
            self.assertTrue(
                uris["docs/figures/atax_comparison.png"].startswith(
                    "data:image/png;base64,"
                )
            )

    def test_falls_back_to_figures_dir_basename(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            figures = root / "alt_figs"
            figures.mkdir(parents=True)
            (figures / "y.png").write_bytes(_minimal_png_bytes())

            uris = gas.build_image_uris(
                ["docs/figures/y.png"],
                repo_root=root,
                figures_dir=figures,
            )
            self.assertIn("docs/figures/y.png", uris)

    def test_skips_unresolvable_references(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            figures = root / "docs" / "figures"
            figures.mkdir(parents=True)
            uris = gas.build_image_uris(
                ["docs/figures/missing.png"],
                repo_root=root,
                figures_dir=figures,
            )
            self.assertEqual(uris, {})


class RenderSummaryMarkdownTest(unittest.TestCase):
    def _setup(self, tmp: Path) -> None:
        figures = tmp / "docs" / "figures"
        figures.mkdir(parents=True)
        for name in ("atax_comparison.png", "bfs_comparison.png"):
            (figures / name).write_bytes(_minimal_png_bytes())

    def test_replaces_png_links_with_data_uri_images(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self._setup(root)
            rendered, stats = gas.render_summary_markdown(
                SAMPLE_REPORT,
                repo_root=root,
                figures_dir=root / "docs" / "figures",
            )

        self.assertEqual(stats.embedded_images, 2)
        self.assertEqual(stats.referenced_images, 2)
        self.assertEqual(stats.missing_image_refs, [])
        self.assertIn("data:image/png;base64,", rendered)
        # The rendered summary must contain inline HTML <img> tags.
        self.assertIn("<img", rendered)
        # Headings/paragraphs from the original Markdown remain unchanged.
        self.assertIn("# Benchmark Accuracy Report", rendered)
        self.assertIn("**12.34%**", rendered)
        # Original PNG markdown link must not survive in raw form.
        self.assertNotIn("](docs/figures/atax_comparison.png)", rendered)

    def test_preserves_unresolved_links(self):
        rendered, stats = gas.render_summary_markdown(
            "# T\n\n[x](docs/figures/missing.png)\n",
            repo_root=Path("/nonexistent"),
            figures_dir=Path("/nonexistent"),
        )
        self.assertEqual(stats.embedded_images, 0)
        self.assertEqual(stats.missing_image_refs, ["docs/figures/missing.png"])
        # Unresolved references stay as-is so readers can still see the path.
        self.assertIn("[x](docs/figures/missing.png)", rendered)

    def test_rendered_image_decodes_to_original_png(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self._setup(root)
            rendered, _ = gas.render_summary_markdown(
                SAMPLE_REPORT,
                repo_root=root,
                figures_dir=root / "docs" / "figures",
            )
        expected = base64.b64encode(_minimal_png_bytes()).decode("ascii")
        self.assertIn(expected, rendered)


class GenerateTest(unittest.TestCase):
    def _setup_repo(self, tmp: Path) -> Path:
        figures = tmp / "docs" / "figures"
        figures.mkdir(parents=True)
        for name in ("atax_comparison.png", "bfs_comparison.png"):
            (figures / name).write_bytes(_minimal_png_bytes())
        report = tmp / "accuracy_report.md"
        report.write_text(SAMPLE_REPORT, encoding="utf-8")
        return report

    def test_appends_to_summary_file(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            report = self._setup_repo(tmp_path)
            summary = tmp_path / "summary.md"
            summary.write_text("preexisting line\n", encoding="utf-8")

            result = gas.generate(
                input_path=report,
                figures_dir=tmp_path / "docs" / "figures",
                summary_path=summary,
                repo_root=tmp_path,
            )

            text = summary.read_text(encoding="utf-8")
            self.assertTrue(text.startswith("preexisting line\n"))
            self.assertIn("# Benchmark Accuracy Report", text)
            self.assertIn("data:image/png;base64,", text)
            self.assertEqual(result.embedded_images, 2)
            self.assertEqual(result.referenced_images, 2)
            self.assertEqual(result.missing_image_refs, [])
            self.assertFalse(result.skipped)
            self.assertFalse(result.truncated)
            self.assertGreater(result.bytes_written, 0)

    def test_creates_summary_file_when_missing(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            report = self._setup_repo(tmp_path)
            summary = tmp_path / "nested" / "summary.md"

            gas.generate(
                input_path=report,
                figures_dir=tmp_path / "docs" / "figures",
                summary_path=summary,
                repo_root=tmp_path,
            )
            self.assertTrue(summary.is_file())
            self.assertIn("data:image/png;base64,", summary.read_text())

    def test_skips_when_input_missing(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            summary = tmp_path / "summary.md"

            result = gas.generate(
                input_path=tmp_path / "missing.md",
                figures_dir=tmp_path,
                summary_path=summary,
                repo_root=tmp_path,
            )

            self.assertTrue(result.skipped)
            self.assertFalse(summary.exists())
            self.assertIn("missing.md", result.skip_reason)

    def test_truncates_when_over_limit(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            report = self._setup_repo(tmp_path)
            summary = tmp_path / "summary.md"

            result = gas.generate(
                input_path=report,
                figures_dir=tmp_path / "docs" / "figures",
                summary_path=summary,
                repo_root=tmp_path,
                # Force truncation with a tiny budget.
                max_bytes=512,
            )

            text = summary.read_text(encoding="utf-8")
            self.assertTrue(result.truncated)
            self.assertIn("truncated", text.lower())
            self.assertLessEqual(len(text.encode("utf-8")), 512 + 64)


class MainTest(unittest.TestCase):
    def test_main_uses_summary_file_arg(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            (tmp_path / "docs" / "figures").mkdir(parents=True)
            (tmp_path / "docs" / "figures" / "atax_comparison.png").write_bytes(
                _minimal_png_bytes()
            )
            report = tmp_path / "accuracy_report.md"
            report.write_text(
                "# Title\n\n[atax](docs/figures/atax_comparison.png)\n",
                encoding="utf-8",
            )
            summary = tmp_path / "summary.md"

            exit_code = gas.main(
                [
                    "--input", str(report),
                    "--figures-dir", str(tmp_path / "docs" / "figures"),
                    "--summary-file", str(summary),
                    "--repo-root", str(tmp_path),
                ]
            )

            self.assertEqual(exit_code, 0)
            self.assertIn("data:image/png;base64,", summary.read_text())

    def test_main_uses_env_when_no_summary_arg(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            (tmp_path / "docs" / "figures").mkdir(parents=True)
            (tmp_path / "docs" / "figures" / "atax_comparison.png").write_bytes(
                _minimal_png_bytes()
            )
            report = tmp_path / "accuracy_report.md"
            report.write_text(
                "# Title\n\n[atax](docs/figures/atax_comparison.png)\n",
                encoding="utf-8",
            )
            summary = tmp_path / "summary.md"

            saved = os.environ.get("GITHUB_STEP_SUMMARY")
            os.environ["GITHUB_STEP_SUMMARY"] = str(summary)
            try:
                exit_code = gas.main(
                    [
                        "--input", str(report),
                        "--figures-dir", str(tmp_path / "docs" / "figures"),
                        "--repo-root", str(tmp_path),
                    ]
                )
            finally:
                if saved is None:
                    os.environ.pop("GITHUB_STEP_SUMMARY", None)
                else:
                    os.environ["GITHUB_STEP_SUMMARY"] = saved

            self.assertEqual(exit_code, 0)
            self.assertTrue(summary.is_file())
            self.assertIn("# Title", summary.read_text())

    def test_main_returns_zero_when_input_missing(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            summary = tmp_path / "summary.md"

            saved = os.environ.get("GITHUB_STEP_SUMMARY")
            os.environ["GITHUB_STEP_SUMMARY"] = str(summary)
            try:
                exit_code = gas.main(
                    [
                        "--input", str(tmp_path / "missing.md"),
                        "--figures-dir", str(tmp_path),
                        "--repo-root", str(tmp_path),
                    ]
                )
            finally:
                if saved is None:
                    os.environ.pop("GITHUB_STEP_SUMMARY", None)
                else:
                    os.environ["GITHUB_STEP_SUMMARY"] = saved

            self.assertEqual(exit_code, 0)
            self.assertFalse(summary.exists())

    def test_main_writes_to_stdout_when_no_summary_set(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            (tmp_path / "docs" / "figures").mkdir(parents=True)
            (tmp_path / "docs" / "figures" / "atax_comparison.png").write_bytes(
                _minimal_png_bytes()
            )
            report = tmp_path / "accuracy_report.md"
            report.write_text(
                "# Title\n\n[atax](docs/figures/atax_comparison.png)\n",
                encoding="utf-8",
            )

            saved = os.environ.pop("GITHUB_STEP_SUMMARY", None)
            buf = io.StringIO()
            try:
                with redirect_stdout(buf):
                    exit_code = gas.main(
                        [
                            "--input", str(report),
                            "--figures-dir", str(tmp_path / "docs" / "figures"),
                            "--repo-root", str(tmp_path),
                        ]
                    )
            finally:
                if saved is not None:
                    os.environ["GITHUB_STEP_SUMMARY"] = saved

            self.assertEqual(exit_code, 0)
            self.assertIn("data:image/png;base64,", buf.getvalue())


if __name__ == "__main__":
    unittest.main()
