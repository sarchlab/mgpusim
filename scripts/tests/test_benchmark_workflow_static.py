"""Static regression checks for the benchmark summary/report workflow."""

from pathlib import Path
import re
import subprocess


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "benchmark.yml"
VALIDATION_REPORT = REPO_ROOT / "docs" / "validation_report.md"
WORKFLOW_TEXT = WORKFLOW.read_text(encoding="utf-8")


BARE_PIP_INSTALL = re.compile(r"(?m)^\s*(?:sudo\s+)?pip(?:3)?\s+install\b")
BARE_ALWAYS_IF = re.compile(
    r"(?m)^\s*if:\s*(?:\$\{\{\s*)?always\(\)\s*(?:\}\})?\s*$"
)


def workflow_step(name: str) -> str:
    """Return a named GitHub Actions step from benchmark.yml."""
    marker = f"      - name: {name}"
    lines = WORKFLOW_TEXT.splitlines()
    for index, line in enumerate(lines):
        if line == marker:
            end = len(lines)
            for next_index in range(index + 1, len(lines)):
                if lines[next_index].startswith("      - name: "):
                    end = next_index
                    break
            return "\n".join(lines[index:end])
    raise AssertionError(f"workflow step not found: {name}")


def upload_artifact_paths(step: str) -> list[str]:
    """Return path entries from an upload-artifact step."""
    lines = step.splitlines()
    for index, line in enumerate(lines):
        path_match = re.match(r"^(\s*)path:\s*(.*)$", line)
        if not path_match:
            continue
        indent, value = path_match.groups()
        if value and value != "|":
            return [value.strip()]

        paths = []
        child_indent = len(indent) + 2
        for child in lines[index + 1 :]:
            if not child.strip():
                continue
            if len(child) - len(child.lstrip(" ")) < child_indent:
                break
            paths.append(child.strip())
        return paths
    raise AssertionError("upload-artifact step has no path input")


def validation_report_figure_refs() -> list[str]:
    """Return repo-relative PNG refs linked from docs/validation_report.md."""
    report_text = VALIDATION_REPORT.read_text(encoding="utf-8")
    refs = []
    for match in re.findall(r"\]\(([^)\s]+?\.png)\)", report_text):
        if "://" in match or match.startswith("#"):
            continue
        path = Path(match)
        report_relative = path if path.parts[:1] == ("docs",) else Path("docs") / path
        refs.append(report_relative.as_posix())
    return list(dict.fromkeys(refs))


def artifact_path_covers(upload_path: str, repo_relative_path: str) -> bool:
    """Return whether an upload path entry would include a repo-relative path."""
    normalized_path = normalize_repo_path(repo_relative_path.replace("\\", "/"))
    normalized_pattern = normalize_repo_path(upload_path.strip().replace("\\", "/"))
    if normalized_path is None or normalized_pattern is None:
        return False
    return path_parts_match(normalized_path.split("/"), normalized_pattern.split("/"))


def normalize_repo_path(path: str) -> str | None:
    """Normalize a repo-relative path without allowing traversal above the root."""
    parts = []
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


def path_parts_match(path_parts: list[str], pattern_parts: list[str]) -> bool:
    """Match upload-artifact globs against path segments."""
    if not pattern_parts:
        return not path_parts
    if pattern_parts[0] == "**":
        if len(pattern_parts) == 1:
            return True
        return any(
            path_parts_match(path_parts[index:], pattern_parts[1:])
            for index in range(len(path_parts) + 1)
        )
    if not path_parts:
        return False
    return (
        glob_segment_matches(path_parts[0], pattern_parts[0])
        and path_parts_match(path_parts[1:], pattern_parts[1:])
    )


def glob_segment_matches(path_part: str, pattern_part: str) -> bool:
    """Match a glob pattern within one path segment only."""
    regex = "".join(
        "[^/]*"
        if char == "*"
        else "[^/]"
        if char == "?"
        else re.escape(char)
        for char in pattern_part
    )
    return re.fullmatch(regex, path_part) is not None


def test_artifact_path_covers_matches_direct_sibling_not_nested() -> None:
    """A single-star upload glob covers only one path segment."""
    upload_path = "docs/figures/*.png"

    assert artifact_path_covers(upload_path, "docs/figures/scaling.png")
    assert artifact_path_covers(upload_path, "docs/figures/slowdown_bar_chart.png")
    assert not artifact_path_covers(upload_path, "docs/figures/nested/scaling.png")


def test_artifact_path_covers_supports_recursive_double_star() -> None:
    """A double-star upload glob remains recursive across path segments."""
    upload_path = "docs/figures/**/*.png"

    assert artifact_path_covers(upload_path, "docs/figures/scaling.png")
    assert artifact_path_covers(upload_path, "docs/figures/nested/scaling.png")


def test_report_dependencies_use_python_module_pip_not_bare_pip() -> None:
    """Report deps must install through python3 -m pip and fail loudly."""
    step = workflow_step("Install Python report dependencies")

    assert not BARE_PIP_INSTALL.search(WORKFLOW_TEXT)
    assert "python3 -m pip --version" in step
    assert "python3 -m ensurepip --upgrade" in step
    assert "python3 -m pip install pandas matplotlib" in step
    assert (
        "::error::failed to install Python report dependencies with python3 -m pip"
        in step
    )
    assert "failed to import Python report dependencies after installation" in step


def test_accuracy_report_generation_builds_fresh_guarded_artifact_dir() -> None:
    """Generated report upload input must be staged from fresh outputs only."""
    step = workflow_step("Generate accuracy report")

    assert "id: accuracy_report" in step
    assert 'export ACCURACY_ARTIFACT_DIR="accuracy-report-artifact"' in step
    assert "ACCURACY_MARKER=\"$(mktemp)\"" in step
    assert "touch \"${ACCURACY_MARKER}\"" in step
    assert "rm -rf \"${ACCURACY_ARTIFACT_DIR}\"" in step
    assert "python3 scripts/generate_comparison_report.py" in step
    assert "accuracy_report.generated" in step
    assert "accuracy_report.md was not generated by Generate accuracy report" in step
    assert "accuracy_report.md was not generated by this step" in step
    assert "reported figure was not generated by this step" in step
    assert "accuracy report generated-output sentinel missing" in step
    assert 'echo "generated=true" >> "${GITHUB_OUTPUT}"' in step
    assert 'echo "artifact_dir=${ACCURACY_ARTIFACT_DIR}" >> "${GITHUB_OUTPUT}"' in step


def test_validation_report_upload_covers_all_linked_figures() -> None:
    """The validation-report artifact must be self-contained for linked PNGs."""
    step = workflow_step("Upload validation report artifacts")
    upload_paths = upload_artifact_paths(step)
    figure_refs = validation_report_figure_refs()

    assert len(figure_refs) > 2, "regression guard must cover more than the summary PNGs"
    missing_from_upload = [
        ref
        for ref in figure_refs
        if not any(artifact_path_covers(upload_path, ref) for upload_path in upload_paths)
    ]
    assert missing_from_upload == []


def test_accuracy_report_upload_requires_generated_sentinel() -> None:
    """The accuracy artifact must not be a bare always() upload of checked-in files."""
    step = workflow_step("Upload accuracy report")

    assert not BARE_ALWAYS_IF.search(step)
    assert "steps.accuracy_report.outputs.generated == 'true'" in step
    assert "path: ${{ steps.accuracy_report.outputs.artifact_dir }}/" in step
    assert "if-no-files-found: error" in step

    path_section = step.split("path:", 1)[1]
    assert "accuracy_report.md" not in path_section
    assert "docs/figures" not in path_section


CURRENT_MI300A_CONTRACT_GUIDANCE_FILES = (
    ".github/workflows/benchmark.yml",
    "benchmark-comparison/README.md",
    "docs/CONTRIBUTING.md",
    "docs/mi300a_evaluation_benchmark_set.md",
    "docs/validation_report.md",
)
STALE_CURRENT_CONTRACT_SNIPPETS = (
    "Current Replacement Workflow",
    "timeout_sec: 600",
    "timeout-minutes: 60",
    "timeout-minutes: 720",
    "720-min job budget",
    "matrix.timeout_sec || '3600'",
    "timeout_sec must be 3600s for the regular MI300A contract",
    "600s/size, 60min/job",
)
STALE_CONTRACT_ALLOWED_PREFIXES = (
    ".repo_tmp/",
    "benchmark-comparison/provenance/",
    "benchmark-comparison/selected-run-25619929396/",
    "scripts/fixtures/",
    "scripts/tests/",
    "vendor/",
)
STALE_CONTRACT_ALLOWED_FILES = {
    "scripts/validate_mi300a_regular_workflow_matrix.py",
}
HISTORICAL_CONTRACT_CLASSIFICATION_FILES = (
    "benchmark-comparison/selected-run-25619929396/README.md",
    "benchmark-comparison/provenance/README.md",
    "scripts/fixtures/mi300a_matrix_contract_boundary/README.md",
)


def test_current_mi300a_contract_guidance_uses_maintained_timeouts() -> None:
    """Current user-facing workflow text must state the maintained contract."""
    required_snippets = (
        "timeout_sec: 7200",
        "timeout-minutes: 360",
        "21600",
        "max-parallel: 14",
    )

    for rel_path in CURRENT_MI300A_CONTRACT_GUIDANCE_FILES:
        text = (REPO_ROOT / rel_path).read_text(encoding="utf-8")
        with_subtest_context = f"{rel_path} should describe current MI300A limits"
        for snippet in required_snippets:
            assert snippet in text, with_subtest_context
        for stale in STALE_CURRENT_CONTRACT_SNIPPETS:
            assert stale not in text, f"{rel_path} contains stale current-contract text: {stale}"

    evaluation_text = (REPO_ROOT / "docs/mi300a_evaluation_benchmark_set.md").read_text(
        encoding="utf-8"
    )
    current_section = _markdown_section(evaluation_text, "Current Maintained Regular Workflow")
    assert "timeout_sec: 7200" in current_section
    assert "timeout-minutes: 360" in current_section
    assert "21600 seconds" in current_section
    assert "strategy.max-parallel: 14" in current_section

    report_text = VALIDATION_REPORT.read_text(encoding="utf-8")
    report_contract = _markdown_section(report_text, "Regular Workflow Contract")
    assert "7200s per simulator invocation (`timeout_sec: 7200`)" in report_contract
    assert "360min/21600s per benchmark-tier job (`timeout-minutes: 360`)" in report_contract
    assert "`max-parallel: 14`" in report_contract


def test_no_unclassified_tracked_stale_current_mi300a_contract_snippets() -> None:
    """Static grep guard: stale current-contract snippets need an archive/negative-test home."""
    offenders: list[str] = []
    for rel_path in _git_tracked_files():
        if _stale_contract_path_is_allowed(rel_path):
            continue
        path = REPO_ROOT / rel_path
        if not path.is_file():
            continue
        text = path.read_text(encoding="utf-8", errors="ignore")
        for line_number, line in enumerate(text.splitlines(), start=1):
            for snippet in STALE_CURRENT_CONTRACT_SNIPPETS:
                if snippet in line:
                    offenders.append(f"{rel_path}:{line_number}: {snippet}")

    assert offenders == []


def test_historical_contract_artifacts_are_classified_and_allowlisted() -> None:
    """Raw selected-run/provenance/fixture bytes may keep old wording only as history."""
    expected_historical_snippets = {
        "benchmark-comparison/selected-run-25619929396/regular_workflow_validation_summary.md": "timeout-minutes: 720",
        "benchmark-comparison/provenance/mi300a_regular_benchmark_run_25266517426_launch_20260503T011826Z.md": "timeout-minutes: 720",
        "scripts/fixtures/mi300a_matrix_contract_boundary/archived_full_run_contract/benchmark.yml": "matrix.timeout_sec || '3600'",
        "scripts/tests/test_validate_mi300a_regular_workflow_matrix.py": "matrix.timeout_sec || '3600'",
        "scripts/validate_mi300a_regular_workflow_matrix.py": "timeout_sec must be 3600s for the regular MI300A contract",
    }
    for rel_path, snippet in expected_historical_snippets.items():
        assert _stale_contract_path_is_allowed(rel_path), rel_path
        assert snippet in (REPO_ROOT / rel_path).read_text(encoding="utf-8")

    for rel_path in HISTORICAL_CONTRACT_CLASSIFICATION_FILES:
        text = (REPO_ROOT / rel_path).read_text(encoding="utf-8")
        assert "current" in text.lower(), rel_path
        assert "historical" in text.lower() or "archive" in text.lower(), rel_path
        assert "7200" in text, rel_path
        assert "360" in text, rel_path
        assert "21600" in text, rel_path
        assert "14" in text, rel_path


def _markdown_section(markdown: str, heading: str) -> str:
    marker = f"## {heading}"
    start = markdown.find(marker)
    assert start >= 0, f"missing Markdown section: {heading}"
    next_heading = markdown.find("\n## ", start + len(marker))
    if next_heading < 0:
        return markdown[start:]
    return markdown[start:next_heading]


def _git_tracked_files() -> list[str]:
    result = subprocess.run(
        ["git", "ls-files"],
        cwd=REPO_ROOT,
        check=True,
        text=True,
        stdout=subprocess.PIPE,
    )
    return [line for line in result.stdout.splitlines() if line]


def _stale_contract_path_is_allowed(rel_path: str) -> bool:
    return rel_path in STALE_CONTRACT_ALLOWED_FILES or rel_path.startswith(
        STALE_CONTRACT_ALLOWED_PREFIXES
    )
