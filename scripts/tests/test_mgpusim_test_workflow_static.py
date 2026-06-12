"""Static regression checks for the MGPUSim test workflow."""

from pathlib import Path
import re


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "mgpusim_test.yml"
REPORT_SCRIPT = REPO_ROOT / "scripts" / "generate_comparison_report.py"
WORKFLOW_TEXT = WORKFLOW.read_text(encoding="utf-8")
REPORT_SCRIPT_TEXT = REPORT_SCRIPT.read_text(encoding="utf-8")

BARE_PIP_INSTALL = re.compile(
    r"(?m)^\s*(?:run:\s*)?(?:sudo\s+)?pip(?:3)?\s+install\b"
)
BARE_PYTEST = re.compile(r"(?m)^\s*(?:run:\s*)?pytest\b")
SETUP_PYTHON_3X = re.compile(
    r"uses:\s*actions/setup-python@v\d+[\s\S]{0,300}?"
    r"python-version:\s*['\"]?3\.x['\"]?",
    re.IGNORECASE,
)


def workflow_step(name: str) -> str:
    """Return a named GitHub Actions step from mgpusim_test.yml."""
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


def test_python_suite_uses_system_python3_with_pip_bootstrap() -> None:
    """Python suite must use system python3, bootstrap pip, and run pytest fully."""
    bootstrap_step = workflow_step("Bootstrap pip")
    install_step = workflow_step("Install Python test dependencies")
    pytest_step = workflow_step("Run pytest suite")

    assert "actions/setup-python" not in WORKFLOW_TEXT
    assert "python-version: '3.x'" not in WORKFLOW_TEXT
    assert 'python-version: "3.x"' not in WORKFLOW_TEXT
    assert not SETUP_PYTHON_3X.search(WORKFLOW_TEXT)

    assert not BARE_PIP_INSTALL.search(WORKFLOW_TEXT)
    assert not BARE_PYTEST.search(WORKFLOW_TEXT)

    assert "python3 --version" in bootstrap_step
    assert "if ! python3 -m pip --version; then" in bootstrap_step
    assert "python3 -m ensurepip --upgrade" in bootstrap_step
    assert "python3 -m pip --version" in bootstrap_step

    assert "python3 -m pip install --user --upgrade pytest pandas matplotlib" in install_step
    assert "Install pytest" not in WORKFLOW_TEXT
    assert "import pandas as pd" in REPORT_SCRIPT_TEXT
    assert "import matplotlib" in REPORT_SCRIPT_TEXT
    assert "pandas" in install_step
    assert "matplotlib" in install_step
    assert "PYTHONPATH: ." in pytest_step
    assert "run: python3 -m pytest scripts/tests/ -v" in pytest_step
