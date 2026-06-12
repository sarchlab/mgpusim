"""Static readiness checks for committed report artifacts."""

from __future__ import annotations

import csv
from dataclasses import dataclass
import hashlib
import json
import re
import struct
import subprocess
import unittest
from pathlib import Path
from urllib.parse import unquote


REPO_ROOT = Path(__file__).resolve().parents[2]
VALIDATION_REPORT = REPO_ROOT / "docs" / "validation_report.md"
CURATED_VALIDATION_REPORT = REPO_ROOT / "docs" / "validation_report_curated.md"
ACCURACY_REPORT = REPO_ROOT / "accuracy_report.md"
BENCHMARK_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "benchmark.yml"
SELECTED_RUN_DIR = REPO_ROOT / "benchmark-comparison" / "selected-run-25619929396"
SELECTED_COMPARISON_CSV = SELECTED_RUN_DIR / "comparison_ci.detailed.csv"
SELECTED_COMPARISON_CSV_REFERENCE = (
    "benchmark-comparison/selected-run-25619929396/comparison_ci.detailed.csv"
)
LEGACY_COMPARISON_CSV_REFERENCE = "benchmark-comparison/comparison_ci.csv"
CI_ARTIFACTS_PROVENANCE_MANIFEST = (
    REPO_ROOT / "ci_artifacts" / "provenance_manifest.json"
)
VALIDATION_REPORT_FIGURE_MANIFEST = (
    SELECTED_RUN_DIR / "validation-report-6905152413-sha256.tsv"
)
RAW_ZIP_SHA256 = SELECTED_RUN_DIR / "raw-zip-sha256.tsv"
REPORTS_WITH_COMMITTED_FIGURES = (
    VALIDATION_REPORT,
    CURATED_VALIDATION_REPORT,
    ACCURACY_REPORT,
)

EXPECTED_SELECTED_RUN_ID = "25619929396"
EXPECTED_SELECTED_HEAD_SHA = "a281789a7354b86a8a04b350c145c573d531f54d"
EXPECTED_VALIDATION_REPORT_ARTIFACT_ID = "6905152413"
EXPECTED_VALIDATION_REPORT_ZIP_SHA256 = (
    "6cf7ea08cc28be8f5751364fb583da905b9b8bdec15235a83f896fc8e6b67f5e"
)
EXPECTED_SELECTED_COMPARISON_SHA256 = (
    "867f125aa726ff3a06dff4b40d27008e897347da1001011cd5bd6207823a3a20"
)
EXPECTED_LEGACY_CI_ARTIFACTS_RUN_ID = "24481375286"
EXPECTED_LEGACY_CI_ARTIFACTS_REF_SHA = (
    "ba9bcc06f812b7d5d839d3d4bb43b8d8fd1e1932"
)
EXPECTED_VALIDATION_REPORT_ARTIFACT_FILES = {
    "docs/validation_report.md": {
        "sha256": "c743b5ec8ef4a0458edcdaa43b9dc0b46fef7cb51f4a71442d4328cd16cb165b",
        "size_bytes": "93990",
        "png_width": "n/a",
        "png_height": "n/a",
        # Raw selected-run artifact copy; docs/validation_report.md is the
        # current generated report and may include newer static contract text.
        "repo_paths": [
            "benchmark-comparison/selected-run-25619929396/validation_report.md",
        ],
    },
    "docs/figures/sim_vs_hw_scatter.png": {
        "sha256": "7b6f3011ba800d75680673a6d88550213b5920a1d4ac2f9a874834c825add345",
        "size_bytes": "190828",
        "png_width": "1172",
        "png_height": "1180",
        "repo_paths": ["docs/figures/sim_vs_hw_scatter.png"],
    },
    "docs/figures/slowdown_bar_chart.png": {
        "sha256": "ce24d0b08e44699a2e5bf57a6007782d35f7d1049baa8be9c0c4d1b1103fff2e",
        "size_bytes": "179659",
        "png_width": "1783",
        "png_height": "884",
        "repo_paths": ["docs/figures/slowdown_bar_chart.png"],
    },
}

_MARKDOWN_PNG_REF_RE = re.compile(
    r"!?\[[^\]]*\]\(\s*([^\s)]+?\.png)(?:\s+['\"][^)]*['\"])?\s*\)",
    re.IGNORECASE,
)
_HTML_PNG_REF_RE = re.compile(
    r"<img\b[^>]*\bsrc=['\"]([^'\"]+?\.png)['\"]",
    re.IGNORECASE,
)
_KNOWN_PLACEHOLDER_PNG = bytes.fromhex(
    "89504e470d0a1a0a0000000d4948445200000001000000010806000000"
    "1f15c4890000000b4944415478da636000020000050001e9fadcd800"
    "00000049454e44ae426082"
)


@dataclass(frozen=True)
class PngReferenceReadiness:
    refs: list[str]
    resolved_refs: dict[str, str]
    package_paths: dict[str, str]
    missing_from_head: list[str]
    missing_from_tracked_tree: list[str]
    missing_from_package: list[str]
    placeholder_pngs: list[str]


class CommittedReportReadinessTest(unittest.TestCase):
    def test_committed_report_png_references_exist_in_committed_tree(self) -> None:
        package_patterns = _validation_report_package_paths()
        validation_readiness = _report_png_reference_readiness(
            VALIDATION_REPORT,
            VALIDATION_REPORT.read_text(encoding="utf-8"),
            package_patterns,
        )

        self.assertIn("figures/slowdown_bar_chart.png", validation_readiness.refs)
        self.assertIn("figures/sim_vs_hw_scatter.png", validation_readiness.refs)
        self.assertTrue(
            any(ref.endswith("_scaling.png") for ref in validation_readiness.refs),
            "validation report should include per-benchmark scaling figures",
        )
        self.assertTrue(
            _path_matches_package_manifest(
                "docs/validation_report.md", package_patterns
            ),
            "docs/validation_report.md must be included in the validation-report "
            "artifact package manifest",
        )

        for report_path in REPORTS_WITH_COMMITTED_FIGURES:
            report_rel = report_path.relative_to(REPO_ROOT).as_posix()
            with self.subTest(report=report_rel):
                readiness = _report_png_reference_readiness(
                    report_path,
                    report_path.read_text(encoding="utf-8"),
                    package_patterns,
                )

                self.assertGreater(
                    len(readiness.refs),
                    0,
                    f"{report_rel} should reference PNG figures",
                )
                self.assertEqual(
                    readiness.missing_from_head,
                    [],
                    f"Every PNG referenced by {report_rel} must exist in HEAD; "
                    "ignored local generated files do not count as committed files",
                )
                self.assertEqual(
                    readiness.missing_from_tracked_tree,
                    [],
                    f"Every PNG referenced by {report_rel} must remain tracked "
                    "in the current git tree/index",
                )
                self.assertEqual(
                    readiness.missing_from_package,
                    [],
                    f"Every PNG referenced by {report_rel} must be covered by "
                    "the validation-report artifact package manifest",
                )
                self.assertEqual(
                    readiness.placeholder_pngs,
                    [],
                    f"No PNG referenced by {report_rel} may match the legacy "
                    "deterministic 1x1 placeholder bytes",
                )

    def test_accuracy_report_untracked_docs_figure_ref_fails_readiness(self) -> None:
        ref = "docs/figures/__readiness_untracked_regression__.png"
        temp_path = REPO_ROOT / ref
        self.assertFalse(_git_path_exists_in_head(ref), ref)
        self.assertFalse(_git_path_is_tracked(ref), ref)

        temp_path.write_bytes(b"untracked readiness regression png")
        try:
            readiness = _report_png_reference_readiness(
                ACCURACY_REPORT,
                f"![untracked regression]({ref})",
                ["docs/figures/*.png"],
            )
        finally:
            temp_path.unlink(missing_ok=True)

        self.assertEqual(readiness.refs, [ref])
        self.assertEqual(readiness.missing_from_head, [f"{ref} -> {ref}"])

    def test_validation_report_nested_docs_figure_ref_is_not_packaged(self) -> None:
        ref = "figures/nested/scaling.png"
        resolved_ref = "docs/figures/nested/scaling.png"
        package_patterns = ["docs/figures/*.png"]

        readiness = _report_png_reference_readiness(
            VALIDATION_REPORT,
            f"![nested regression]({ref})",
            package_patterns,
        )

        self.assertEqual(
            _repo_relative_reference_candidates(VALIDATION_REPORT, ref)[0],
            resolved_ref,
        )
        self.assertTrue(
            _path_matches_package_manifest(
                "docs/figures/scaling.png", package_patterns
            )
        )
        self.assertFalse(
            _path_matches_package_manifest(resolved_ref, package_patterns)
        )
        self.assertIn(
            f"{ref} -> {resolved_ref}", readiness.missing_from_package
        )

    def test_selected_validation_report_summary_pngs_match_extracted_manifest(
        self,
    ) -> None:
        manifest_header, manifest_rows = _read_validation_report_figure_manifest()

        self.assertEqual(manifest_header.get("run_id"), EXPECTED_SELECTED_RUN_ID)
        self.assertEqual(
            manifest_header.get("head_sha"), EXPECTED_SELECTED_HEAD_SHA
        )
        self.assertEqual(
            manifest_header.get("artifact_id"), EXPECTED_VALIDATION_REPORT_ARTIFACT_ID
        )
        self.assertEqual(manifest_header.get("artifact_name"), "validation-report")
        self.assertEqual(
            manifest_header.get("raw_zip_sha256"),
            EXPECTED_VALIDATION_REPORT_ZIP_SHA256,
        )
        self.assertEqual(
            manifest_header.get("comparison_ci_sha256"),
            EXPECTED_SELECTED_COMPARISON_SHA256,
        )
        self.assertIn(
            f"{EXPECTED_VALIDATION_REPORT_ZIP_SHA256}  "
            ".m57_run_25619929396_artifacts/raw/validation-report-6905152413.zip",
            RAW_ZIP_SHA256.read_text(encoding="utf-8"),
        )
        self.assertEqual(
            _sha256_file(SELECTED_COMPARISON_CSV),
            EXPECTED_SELECTED_COMPARISON_SHA256,
        )
        report_text = VALIDATION_REPORT.read_text(encoding="utf-8")
        self.assertIn(
            f"**Data source:** `{SELECTED_COMPARISON_CSV_REFERENCE}`",
            report_text,
        )
        self.assertIn(
            f"- **Raw data:** `{SELECTED_COMPARISON_CSV_REFERENCE}`",
            report_text,
        )
        self.assertNotIn(
            f"- **Raw data:** `{LEGACY_COMPARISON_CSV_REFERENCE}`",
            report_text,
            "selected-run validation report must not cite the legacy top-level "
            "comparison_ci.csv as its current raw-data reference",
        )

        duplicate_paths = _duplicate_manifest_paths(manifest_rows)
        self.assertEqual(duplicate_paths, [], "manifest paths must be unique")
        for row in manifest_rows:
            artifact_path = row.get("path", "")
            with self.subTest(manifest_dimensions=artifact_path):
                dimensions = (row.get("png_width"), row.get("png_height"))
                if artifact_path.endswith(".png"):
                    for dimension in dimensions:
                        self.assertIsNotNone(dimension)
                        assert dimension is not None
                        self.assertRegex(dimension, r"^[1-9][0-9]*$")
                else:
                    self.assertEqual(dimensions, ("n/a", "n/a"))
        rows_by_path = {row["path"]: row for row in manifest_rows}
        for artifact_path, expected in (
            EXPECTED_VALIDATION_REPORT_ARTIFACT_FILES.items()
        ):
            with self.subTest(artifact_path=artifact_path):
                row = rows_by_path.get(artifact_path)
                self.assertIsNotNone(row, f"missing manifest row for {artifact_path}")
                assert row is not None
                self.assertEqual(
                    row.get("artifact_id"), EXPECTED_VALIDATION_REPORT_ARTIFACT_ID
                )
                for field in ("sha256", "size_bytes", "png_width", "png_height"):
                    if field in expected:
                        self.assertEqual(row.get(field), expected[field])

                for repo_relative_path in expected["repo_paths"]:
                    repo_path = REPO_ROOT / repo_relative_path
                    self.assertTrue(repo_path.is_file(), repo_relative_path)
                    self.assertEqual(_sha256_file(repo_path), expected["sha256"])
                    self.assertEqual(
                        str(repo_path.stat().st_size), expected["size_bytes"]
                    )
                    if artifact_path.endswith(".png"):
                        self.assertEqual(
                            _png_dimensions(repo_path),
                            (int(expected["png_width"]), int(expected["png_height"])),
                        )

    def test_legacy_ci_artifacts_manifest_points_to_selected_run_provenance(
        self,
    ) -> None:
        manifest = json.loads(
            CI_ARTIFACTS_PROVENANCE_MANIFEST.read_text(encoding="utf-8")
        )
        selected_run_metadata = json.loads(
            (SELECTED_RUN_DIR / "run-view.json").read_text(encoding="utf-8")
        )

        self.assertEqual(manifest.get("run_id"), EXPECTED_LEGACY_CI_ARTIFACTS_RUN_ID)
        self.assertEqual(
            manifest.get("benchmark_ref_sha"), EXPECTED_LEGACY_CI_ARTIFACTS_REF_SHA
        )
        self.assertEqual(manifest.get("archive_status"), "legacy_archived")
        self.assertIs(manifest.get("active_for_m57_reports"), False)
        self.assertIn("not current M57 report provenance", manifest.get("context", ""))
        self.assertIn(
            "current M57 selected-run evidence", manifest.get("archive_notice", "")
        )

        superseded_by = manifest.get("superseded_by", {})
        self.assertEqual(superseded_by.get("run_id"), EXPECTED_SELECTED_RUN_ID)
        self.assertEqual(superseded_by.get("head_sha"), EXPECTED_SELECTED_HEAD_SHA)
        self.assertEqual(
            superseded_by.get("artifacts_dir"),
            "benchmark-comparison/selected-run-25619929396",
        )
        self.assertEqual(
            superseded_by.get("run_metadata"),
            "benchmark-comparison/selected-run-25619929396/run-view.json",
        )
        self.assertEqual(
            superseded_by.get("comparison_ci_csv"),
            "benchmark-comparison/selected-run-25619929396/"
            "comparison_ci.detailed.csv",
        )
        self.assertEqual(
            superseded_by.get("comparison_ci_sha256"),
            EXPECTED_SELECTED_COMPARISON_SHA256,
        )
        self.assertEqual(
            selected_run_metadata.get("databaseId"), int(EXPECTED_SELECTED_RUN_ID)
        )
        self.assertEqual(
            selected_run_metadata.get("headSha"), EXPECTED_SELECTED_HEAD_SHA
        )

    def test_tracked_docs_figures_pngs_are_referenced_by_committed_reports(self) -> None:
        tracked_figures = _git_ls_files("docs/figures/*.png")
        referenced_figures = _committed_report_figure_references()

        self.assertGreater(
            len(tracked_figures),
            0,
            "docs/figures should contain tracked PNG report figures",
        )
        self.assertGreater(
            len(referenced_figures),
            0,
            "committed reports should reference tracked docs/figures PNGs",
        )

        extra_tracked_figures = sorted(set(tracked_figures) - referenced_figures)
        self.assertEqual(
            extra_tracked_figures,
            [],
            "Every tracked docs/figures PNG is packaged by the validation-report "
            "wildcard, so each one must be referenced by a committed report. "
            "Unreferenced PNGs are stale wildcard payloads outside selected-run "
            "report generation.",
        )

    def test_accuracy_report_adjust_weights_selected_run_metrics(self) -> None:
        report_text = ACCURACY_REPORT.read_text(encoding="utf-8")
        row_match = re.search(
            r"^\| adjust_weights \| (?P<samples>\d+) \| "
            r"(?P<mean>[0-9.]+)% \| (?P<max>[0-9.]+)% \| "
            r"(?P<mean_sym>[0-9.]+)% \| (?P<max_sym>[0-9.]+)% \|",
            report_text,
            re.MULTILINE,
        )

        self.assertIsNotNone(
            row_match,
            "accuracy_report.md should include adjust_weights metrics row",
        )
        assert row_match is not None
        self.assertEqual(int(row_match.group("samples")), 11)
        self.assertLess(float(row_match.group("mean")), 20.0)
        self.assertIn("## Report-Time Simulator Calibration", report_text)
        self.assertIn("| adjust_weights | all sizes | 0.570694861247 |", report_text)
        self.assertIn("raw `sim_ms` values", report_text)

    def test_accuracy_report_mem_latency_chase_selected_run_target(self) -> None:
        report_text = ACCURACY_REPORT.read_text(encoding="utf-8")
        row_match = re.search(
            r"^\| mem_latency_chase \| (?P<samples>\d+) \| "
            r"(?P<mean>[0-9.]+)% \| (?P<max>[0-9.]+)% \| "
            r"(?P<mean_sym>[0-9.]+)% \| (?P<max_sym>[0-9.]+)% \|",
            report_text,
            re.MULTILINE,
        )

        self.assertIsNotNone(
            row_match,
            "accuracy_report.md should include mem_latency_chase metrics row",
        )
        assert row_match is not None
        self.assertEqual(int(row_match.group("samples")), 16)
        self.assertLess(float(row_match.group("mean")), 5.0)
        self.assertIn("## Report-Time Simulator Calibration", report_text)
        self.assertIn("| mem_latency_chase | small working set (≤8192) |", report_text)
        self.assertIn(
            "| mem_latency_chase | large DRAM-region working set (≥2097152) |",
            report_text,
        )
        self.assertIn("raw `sim_ms` values", report_text)

        with SELECTED_COMPARISON_CSV.open(newline="", encoding="utf-8") as fh:
            completed_rows = [
                row
                for row in csv.DictReader(fh)
                if row.get("kernel_name") == "mem_latency_chase"
                and row.get("real_ms", "").strip()
                and row.get("sim_ms", "").strip()
            ]
        self.assertEqual(len(completed_rows), 16)

        model = json.loads(
            (REPO_ROOT / "scripts" / "benchmark_tiers.json").read_text(
                encoding="utf-8"
            )
        )["per_benchmark_time_models"]["mem_latency_chase"]
        self.assertIn("run 25619929396", model.get("source", ""))
        self.assertEqual(len(model.get("regions", [])), 3)

    def test_accuracy_report_bfs_selected_run_scale_calibration(self) -> None:
        report_text = ACCURACY_REPORT.read_text(encoding="utf-8")
        row_match = re.search(
            r"^\| bfs \| (?P<samples>\d+) \| "
            r"(?P<mean>[0-9.]+)% \| (?P<max>[0-9.]+)% \| "
            r"(?P<mean_sym>[0-9.]+)% \| (?P<max_sym>[0-9.]+)% \|",
            report_text,
            re.MULTILINE,
        )

        self.assertIsNotNone(
            row_match,
            "accuracy_report.md should include bfs metrics row",
        )
        assert row_match is not None
        self.assertEqual(int(row_match.group("samples")), 12)
        self.assertLess(float(row_match.group("mean")), 20.0)
        self.assertLess(float(row_match.group("max")), 35.0)
        self.assertIn(f"- Run ID: `{EXPECTED_SELECTED_RUN_ID}`", report_text)
        self.assertIn(f"- Head SHA: `{EXPECTED_SELECTED_HEAD_SHA}`", report_text)
        self.assertIn("| bfs | all sizes | 133.490552877 | 0 |", report_text)

        model = json.loads(
            (REPO_ROOT / "scripts" / "benchmark_tiers.json").read_text(
                encoding="utf-8"
            )
        )["per_benchmark_time_models"]["bfs"]
        self.assertAlmostEqual(model["sim_scale"], 133.49055287727813)
        self.assertEqual(model["fixed_time_ms"], 0.0)
        source = model.get("source", "")
        for expected in (
            "12 completed BFS samples",
            SELECTED_COMPARISON_CSV_REFERENCE,
            f"selected run {EXPECTED_SELECTED_RUN_ID}",
            EXPECTED_SELECTED_HEAD_SHA,
            "raw sim_ms values",
            "selected-run identity",
        ):
            self.assertIn(expected, source)

        def bfs_rows(path: Path) -> list[tuple[str, str, str]]:
            with path.open(newline="", encoding="utf-8") as fh:
                return [
                    (
                        row.get("problem_size", ""),
                        row.get("real_ms", ""),
                        row.get("sim_ms", ""),
                    )
                    for row in csv.DictReader(fh)
                    if row.get("kernel_name") == "bfs"
                ]

        selected_rows = bfs_rows(SELECTED_COMPARISON_CSV)
        self.assertEqual(
            selected_rows,
            bfs_rows(REPO_ROOT / LEGACY_COMPARISON_CSV_REFERENCE),
        )
        completed = [row for row in selected_rows if row[1].strip() and row[2].strip()]
        self.assertEqual(len(completed), 12)

        scale = float(model["sim_scale"])
        raw_errors = []
        calibrated_errors = []
        for _, hw_ms, sim_ms in completed:
            hw = float(hw_ms)
            sim = float(sim_ms)
            raw_errors.append(abs(sim - hw) / hw)
            calibrated_errors.append(abs(scale * sim - hw) / hw)

        self.assertAlmostEqual(
            sum(raw_errors) / len(raw_errors) * 100,
            99.2866,
            places=4,
        )
        self.assertAlmostEqual(max(raw_errors) * 100, 99.5122, places=4)
        self.assertAlmostEqual(
            sum(calibrated_errors) / len(calibrated_errors) * 100,
            15.8856,
            places=4,
        )
        self.assertAlmostEqual(max(calibrated_errors) * 100, 34.8783, places=4)

    def test_accuracy_report_preserves_current_adjust_weights_known_limitation(self) -> None:
        report_text = ACCURACY_REPORT.read_text(encoding="utf-8")
        section = _extract_section(report_text, "## Known Limitations")

        self.assertIsNotNone(
            section,
            "accuracy_report.md should preserve Known Limitations",
        )
        assert section is not None
        self.assertIn("### adjust_weights (backprop benchmark)", section)
        self.assertIn("Historical calibration percentages", section)
        self.assertIn("Use the generated per-benchmark metrics table", section)

        stale_claims = [
            "13.51%",
            "5 samples",
            "meets the <20%",
            "fixed_time_ms=0.005",
            "26.00%",
        ]
        for stale_claim in stale_claims:
            self.assertNotIn(stale_claim, section)


def _report_png_reference_readiness(
    report_path: Path,
    report_text: str,
    package_patterns: list[str],
) -> PngReferenceReadiness:
    return _png_reference_readiness(
        report_path,
        _extract_png_references(report_text),
        package_patterns,
    )


def _png_reference_readiness(
    report_path: Path,
    refs: list[str],
    package_patterns: list[str],
) -> PngReferenceReadiness:
    resolved_refs: dict[str, str] = {}
    package_paths: dict[str, str] = {}
    missing_from_head: list[str] = []
    for ref in refs:
        candidates = _repo_relative_reference_candidates(report_path, ref)
        resolved = next(
            (path for path in candidates if _git_path_exists_in_head(path)),
            None,
        )
        if resolved is None:
            missing_from_head.append(_format_missing_ref(ref, candidates))
            if candidates:
                package_paths[ref] = candidates[0]
            continue
        resolved_refs[ref] = resolved
        package_paths[ref] = resolved

    missing_from_tracked_tree = [
        f"{ref} -> {path}"
        for ref, path in resolved_refs.items()
        if not _git_path_is_tracked(path)
    ]
    missing_from_package = [
        f"{ref} -> {path}"
        for ref, path in package_paths.items()
        if not _path_matches_package_manifest(path, package_patterns)
    ]
    return PngReferenceReadiness(
        refs=refs,
        resolved_refs=resolved_refs,
        package_paths=package_paths,
        missing_from_head=missing_from_head,
        missing_from_tracked_tree=missing_from_tracked_tree,
        missing_from_package=missing_from_package,
        placeholder_pngs=_known_placeholder_png_references(resolved_refs),
    )


def _read_validation_report_figure_manifest() -> tuple[
    dict[str, str], list[dict[str, str]]
]:
    header: dict[str, str] = {}
    table_lines: list[str] = []
    for line in VALIDATION_REPORT_FIGURE_MANIFEST.read_text(
        encoding="utf-8"
    ).splitlines():
        if not line:
            continue
        if line.startswith("#"):
            key_value = line[1:].strip().split(":", 1)
            if len(key_value) == 2:
                header[key_value[0].strip()] = key_value[1].strip()
            continue
        table_lines.append(line)

    rows = list(csv.DictReader(table_lines, delimiter="\t"))
    if not rows:
        raise AssertionError(
            f"{VALIDATION_REPORT_FIGURE_MANIFEST.relative_to(REPO_ROOT)} "
            "must contain selected validation-report artifact rows"
        )
    return header, rows


def _duplicate_manifest_paths(rows: list[dict[str, str]]) -> list[str]:
    seen: set[str] = set()
    duplicates: set[str] = set()
    for row in rows:
        path = row.get("path", "")
        if path in seen:
            duplicates.add(path)
        seen.add(path)
    return sorted(duplicates)


def _sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as file:
        for chunk in iter(lambda: file.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _png_dimensions(path: Path) -> tuple[int, int]:
    with path.open("rb") as file:
        header = file.read(24)
    if len(header) < 24 or not header.startswith(b"\x89PNG\r\n\x1a\n"):
        raise AssertionError(f"{path.relative_to(REPO_ROOT)} is not a PNG file")
    return struct.unpack(">II", header[16:24])


def _extract_png_references(markdown: str) -> list[str]:
    refs: list[str] = []
    seen: set[str] = set()
    for regex in (_MARKDOWN_PNG_REF_RE, _HTML_PNG_REF_RE):
        for match in regex.finditer(markdown):
            ref = unquote(match.group(1).split("#", 1)[0].split("?", 1)[0])
            if ref.startswith(("http://", "https://", "data:")):
                continue
            if ref not in seen:
                seen.add(ref)
                refs.append(ref)
    return refs


def _known_placeholder_png_references(resolved_refs: dict[str, str]) -> list[str]:
    offenders: list[str] = []
    for ref, repo_relative_path in resolved_refs.items():
        locations: list[str] = []
        if _git_blob_matches_known_placeholder(repo_relative_path):
            locations.append("HEAD")
        worktree_path = REPO_ROOT / repo_relative_path
        if (
            worktree_path.is_file()
            and worktree_path.read_bytes() == _KNOWN_PLACEHOLDER_PNG
        ):
            locations.append("working tree")
        if locations:
            offenders.append(
                f"{ref} -> {repo_relative_path} ({', '.join(locations)})"
            )
    return offenders


def _git_blob_matches_known_placeholder(repo_relative_path: str) -> bool:
    result = subprocess.run(
        ["git", "-C", str(REPO_ROOT), "show", f"HEAD:{repo_relative_path}"],
        check=False,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    return result.returncode == 0 and result.stdout == _KNOWN_PLACEHOLDER_PNG


def _committed_report_figure_references() -> set[str]:
    references: set[str] = set()
    for report_path in REPORTS_WITH_COMMITTED_FIGURES:
        report_text = report_path.read_text(encoding="utf-8")
        for ref in _extract_png_references(report_text):
            for candidate in _repo_relative_reference_candidates(report_path, ref):
                if candidate.startswith("docs/figures/"):
                    references.add(candidate)
                    break
    return references


def _repo_relative_reference_candidates(report_path: Path, ref: str) -> list[str]:
    report_dir = report_path.parent.relative_to(REPO_ROOT).as_posix()
    ref = ref.replace("\\", "/")
    if ref.startswith("/"):
        raw_candidates = [ref.lstrip("/")]
    else:
        raw_candidates = [f"{report_dir}/{ref}", ref]

    candidates: list[str] = []
    seen: set[str] = set()
    for raw_candidate in raw_candidates:
        normalized = _normalize_repo_path(raw_candidate)
        if normalized is None or normalized in seen:
            continue
        seen.add(normalized)
        candidates.append(normalized)
    return candidates


def _normalize_repo_path(path: str) -> str | None:
    parts: list[str] = []
    for part in path.split("/"):
        if part in ("", "."):
            continue
        if part == "..":
            if not parts:
                return None
            parts.pop()
            continue
        parts.append(part)
    if not parts:
        return None
    return "/".join(parts)


def _git_path_exists_in_head(repo_relative_path: str) -> bool:
    return (
        _run_git(["cat-file", "-e", f"HEAD:{repo_relative_path}"]).returncode == 0
    )


def _git_path_is_tracked(repo_relative_path: str) -> bool:
    return (
        _run_git(
            ["ls-files", "--error-unmatch", "--", repo_relative_path]
        ).returncode
        == 0
    )


def _git_ls_files(pattern: str) -> list[str]:
    result = _run_git(["ls-files", "--", pattern])
    if result.returncode != 0:
        raise AssertionError(f"git ls-files failed for {pattern}: {result.stderr}")
    return result.stdout.splitlines()


def _run_git(args: list[str]) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["git", "-C", str(REPO_ROOT), *args],
        check=False,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )


def _validation_report_package_paths() -> list[str]:
    workflow_text = BENCHMARK_WORKFLOW.read_text(encoding="utf-8")
    paths = _extract_upload_artifact_paths(
        workflow_text, "Upload validation report artifacts"
    )
    if not paths:
        raise AssertionError(
            "Could not find validation-report artifact package paths in "
            f"{BENCHMARK_WORKFLOW.relative_to(REPO_ROOT)}"
        )
    return paths


def _extract_upload_artifact_paths(workflow_text: str, step_name: str) -> list[str]:
    lines = workflow_text.splitlines()
    step_start = None
    step_indent = 0
    step_re = re.compile(r"^(\s*)-\s+name:\s*" + re.escape(step_name) + r"\s*$")
    for index, line in enumerate(lines):
        match = step_re.match(line)
        if match:
            step_start = index
            step_indent = len(match.group(1))
            break
    if step_start is None:
        return []

    step_lines: list[str] = []
    for line in lines[step_start + 1 :]:
        indent = len(line) - len(line.lstrip(" "))
        if line.lstrip().startswith("- name:") and indent <= step_indent:
            break
        step_lines.append(line)

    for index, line in enumerate(step_lines):
        match = re.match(r"^(\s*)path:\s*(.*)$", line)
        if not match:
            continue
        path_indent = len(match.group(1))
        value = match.group(2).strip()
        if value != "|":
            return [_strip_yaml_quotes(value)] if value else []

        paths: list[str] = []
        for block_line in step_lines[index + 1 :]:
            if not block_line.strip():
                continue
            block_indent = len(block_line) - len(block_line.lstrip(" "))
            if block_indent <= path_indent:
                break
            paths.append(_strip_yaml_quotes(block_line.strip()))
        return paths
    return []


def _strip_yaml_quotes(value: str) -> str:
    if len(value) >= 2 and value[0] == value[-1] and value[0] in "'\"":
        return value[1:-1]
    return value


def _path_matches_package_manifest(
    repo_relative_path: str, package_patterns: list[str]
) -> bool:
    return any(
        _path_matches_package_pattern(repo_relative_path, pattern)
        for pattern in package_patterns
    )


def _path_matches_package_pattern(repo_relative_path: str, pattern: str) -> bool:
    normalized_path = _normalize_repo_path(repo_relative_path.replace("\\", "/"))
    normalized_pattern = _normalize_repo_path(pattern.strip().replace("\\", "/"))
    if normalized_path is None or normalized_pattern is None:
        return False
    return _path_parts_match(
        normalized_path.split("/"), normalized_pattern.split("/")
    )


def _path_parts_match(path_parts: list[str], pattern_parts: list[str]) -> bool:
    if not pattern_parts:
        return not path_parts
    if pattern_parts[0] == "**":
        if len(pattern_parts) == 1:
            return True
        return any(
            _path_parts_match(path_parts[index:], pattern_parts[1:])
            for index in range(len(path_parts) + 1)
        )
    if not path_parts:
        return False
    return (
        _glob_segment_matches(path_parts[0], pattern_parts[0])
        and _path_parts_match(path_parts[1:], pattern_parts[1:])
    )


def _glob_segment_matches(path_part: str, pattern_part: str) -> bool:
    regex = "".join(
        "[^/]*"
        if char == "*"
        else "[^/]"
        if char == "?"
        else re.escape(char)
        for char in pattern_part
    )
    return re.fullmatch(regex, path_part) is not None


def _format_missing_ref(ref: str, candidates: list[str]) -> str:
    if not candidates:
        return f"{ref} -> <no valid repo-relative candidate>"
    return f"{ref} -> {', '.join(candidates)}"


def _extract_section(markdown: str, heading: str) -> str | None:
    pattern = re.compile(
        r"^" + re.escape(heading) + r"\b.*?(?=\n## |\Z)",
        re.MULTILINE | re.DOTALL,
    )
    match = pattern.search(markdown)
    if not match:
        return None
    return match.group(0).rstrip()


if __name__ == "__main__":
    unittest.main()
