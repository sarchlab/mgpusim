from __future__ import annotations

import binascii
import os
import struct
import sys
import tempfile
import unittest
import zlib
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(REPO_ROOT / "scripts"))
import generate_validation_report as gvr  # noqa: E402


class FakeFigure:
    def __init__(self, payload: bytes):
        self.payload = payload
        self.savefig_kwargs = None

    def savefig(self, output_path: Path, **kwargs) -> None:
        self.savefig_kwargs = kwargs
        Path(output_path).write_bytes(self.payload)


class GenerateValidationReportFigureIdempotencyTest(unittest.TestCase):
    def test_save_figure_preserves_existing_png_when_pixels_match(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            output_path = tmp_path / "figure.png"
            existing = _rgba_png(
                [(255, 0, 0, 255), (0, 0, 255, 255)],
                software="Matplotlib version3.10.9",
                compresslevel=9,
            )
            candidate = _rgba_png(
                [(255, 0, 0, 255), (0, 0, 255, 255)],
                software="Matplotlib version3.10.8",
                compresslevel=0,
            )
            self.assertNotEqual(existing, candidate)
            output_path.write_bytes(existing)

            figure = FakeFigure(candidate)
            written = gvr.save_figure_if_materially_changed(
                figure, output_path, dpi=150, bbox_inches="tight"
            )

            self.assertFalse(written)
            self.assertEqual(output_path.read_bytes(), existing)
            self.assertEqual(figure.savefig_kwargs["dpi"], 150)
            self.assertEqual(list(tmp_path.glob(".figure.png.*.png")), [])

    def test_save_figure_preserves_existing_png_for_renderer_roundoff(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = Path(tmpdir) / "figure.png"
            existing = _rgba_png([(127, 127, 127, 255)], software="old")
            candidate = _rgba_png([(127, 128, 127, 255)], software="new")
            output_path.write_bytes(existing)

            written = gvr.save_figure_if_materially_changed(
                FakeFigure(candidate), output_path, dpi=150
            )

            self.assertFalse(written)
            self.assertEqual(output_path.read_bytes(), existing)

    def test_save_figure_replaces_existing_png_when_pixels_change(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = Path(tmpdir) / "figure.png"
            existing = _rgba_png([(255, 0, 0, 255)], software="old")
            candidate = _rgba_png([(0, 255, 0, 255)], software="new")
            output_path.write_bytes(existing)

            written = gvr.save_figure_if_materially_changed(
                FakeFigure(candidate), output_path, dpi=150
            )

            self.assertTrue(written)
            self.assertEqual(output_path.read_bytes(), candidate)

    def test_save_figure_writes_new_output_path(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = Path(tmpdir) / "nested" / "figure.png"
            candidate = _rgba_png([(0, 255, 0, 255)], software="new")

            written = gvr.save_figure_if_materially_changed(
                FakeFigure(candidate), output_path, dpi=150
            )

            self.assertTrue(written)
            self.assertEqual(output_path.read_bytes(), candidate)

    def test_generate_figures_keeps_existing_png_when_rendering_matches(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            old_mplconfigdir = os.environ.get("MPLCONFIGDIR")
            os.environ["MPLCONFIGDIR"] = str(tmp_path / ".matplotlib")
            try:
                figures_dir = tmp_path / "figures"
                rows = _figure_rows(sim_scale=1.1)
                classified = _classified(rows)

                generated = gvr.generate_figures(
                    rows, classified, figures_dir=figures_dir
                )
                output_path = figures_dir / "unit_kernel_scaling.png"
                self.assertEqual(
                    generated, [("unit_kernel", "unit_kernel_scaling.png")]
                )
                self.assertTrue(output_path.is_file())

                equivalent_container = _recontainer_png(
                    output_path.read_bytes(), software="Matplotlib version3.10.9"
                )
                self.assertNotEqual(equivalent_container, output_path.read_bytes())
                output_path.write_bytes(equivalent_container)

                generated = gvr.generate_figures(
                    rows, classified, figures_dir=figures_dir
                )

                self.assertEqual(
                    generated, [("unit_kernel", "unit_kernel_scaling.png")]
                )
                self.assertEqual(output_path.read_bytes(), equivalent_container)

                changed_rows = _figure_rows(sim_scale=2.0)
                gvr.generate_figures(
                    changed_rows, _classified(changed_rows), figures_dir=figures_dir
                )

                self.assertNotEqual(output_path.read_bytes(), equivalent_container)
            finally:
                if old_mplconfigdir is None:
                    os.environ.pop("MPLCONFIGDIR", None)
                else:
                    os.environ["MPLCONFIGDIR"] = old_mplconfigdir

    def test_write_text_if_changed_reports_noop_for_identical_content(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = Path(tmpdir) / "report.md"
            output_path.write_text("stable report\n", encoding="utf-8")

            self.assertFalse(
                gvr.write_text_if_changed(output_path, "stable report\n")
            )
            self.assertTrue(
                gvr.write_text_if_changed(output_path, "updated report\n")
            )
            self.assertEqual(
                output_path.read_text(encoding="utf-8"), "updated report\n"
            )


def _rgba_png(pixels, *, software: str, compresslevel: int = 6) -> bytes:
    width = len(pixels)
    height = 1
    ihdr = struct.pack(">IIBBBBB", width, height, 8, 6, 0, 0, 0)
    raw = b"\x00" + b"".join(bytes(pixel) for pixel in pixels)
    chunks = [
        _png_chunk(b"IHDR", ihdr),
        _png_chunk(b"tEXt", f"Software\0{software}".encode("latin1")),
        _png_chunk(b"IDAT", zlib.compress(raw, level=compresslevel)),
        _png_chunk(b"IEND", b""),
    ]
    return gvr.PNG_SIGNATURE + b"".join(chunks)


def _png_chunk(chunk_type: bytes, data: bytes) -> bytes:
    checksum = binascii.crc32(chunk_type + data) & 0xFFFFFFFF
    return (
        struct.pack(">I", len(data))
        + chunk_type
        + data
        + struct.pack(">I", checksum)
    )


def _figure_rows(sim_scale: float):
    rows = []
    for idx, real_ms in enumerate([1.0, 2.0, 3.0, 4.0], start=1):
        rows.append(
            {
                "kernel_name": "unit_kernel",
                "problem_size": str(idx),
                "real_ms": f"{real_ms:.6f}",
                "sim_ms": f"{real_ms * sim_scale:.6f}",
            }
        )
    return rows


def _classified(rows):
    points = [
        {
            "size": row["problem_size"],
            "real": float(row["real_ms"]),
            "sim": float(row["sim_ms"]),
        }
        for row in rows
    ]
    return {
        "unit_kernel": {
            "all": points,
            "threshold": 1.5,
            "all_flat": False,
            "no_flat": False,
            "report_classification": {"scaling_informative": True},
        }
    }


def _recontainer_png(png_bytes: bytes, *, software: str) -> bytes:
    chunks = _read_png_chunks(png_bytes)
    ihdr = _single_chunk(chunks, b"IHDR")
    raw_scanlines = zlib.decompress(
        b"".join(data for chunk_type, data in chunks if chunk_type == b"IDAT")
    )
    rebuilt = [
        _png_chunk(b"IHDR", ihdr),
        _png_chunk(b"tEXt", f"Software\0{software}".encode("latin1")),
    ]
    for chunk_type, data in chunks:
        if chunk_type == b"pHYs":
            rebuilt.append(_png_chunk(chunk_type, data))
    rebuilt.extend(
        [
            _png_chunk(b"IDAT", zlib.compress(raw_scanlines, level=0)),
            _png_chunk(b"IEND", b""),
        ]
    )
    return gvr.PNG_SIGNATURE + b"".join(rebuilt)


def _single_chunk(chunks, chunk_type: bytes) -> bytes:
    matches = [
        data for observed_type, data in chunks if observed_type == chunk_type
    ]
    if len(matches) != 1:
        raise AssertionError(f"expected one {chunk_type!r} chunk, got {len(matches)}")
    return matches[0]


def _read_png_chunks(png_bytes: bytes):
    if not png_bytes.startswith(gvr.PNG_SIGNATURE):
        raise AssertionError("not a PNG")
    chunks = []
    pos = len(gvr.PNG_SIGNATURE)
    while pos < len(png_bytes):
        length = struct.unpack(">I", png_bytes[pos:pos + 4])[0]
        chunk_type = png_bytes[pos + 4:pos + 8]
        chunk_start = pos + 8
        chunk_end = chunk_start + length
        chunks.append((chunk_type, png_bytes[chunk_start:chunk_end]))
        pos = chunk_end + 4
        if chunk_type == b"IEND":
            break
    return chunks


if __name__ == "__main__":
    unittest.main()
