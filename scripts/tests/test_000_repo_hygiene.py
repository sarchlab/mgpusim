"""Bootstrap repository hygiene for the standard unittest discovery run.

The milestone verifier runs::

    python3 -m unittest discover -s scripts/tests -v

from the repository root and then checks both normal untracked files and
ignored files.  Python bytecode caches and a few older tests' repo-local
``.repo_tmp`` scratch directory are implementation details of the test run,
not deliverables, so the discovery suite must prevent or clean them itself.
"""

import sys

# Apply before unittest discovery imports the rest of the local test modules.
# Discovery loads test files in sorted filename order, so this bootstrap module
# runs first for the standard ``scripts/tests`` command.
sys.dont_write_bytecode = True

import atexit
import shutil
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]


def _remove_path(path: Path) -> None:
    if path.is_dir():
        shutil.rmtree(path, ignore_errors=True)
    else:
        try:
            path.unlink()
        except FileNotFoundError:
            pass


def _cleanup_repo_local_runtime_artifacts() -> None:
    """Remove ignored runtime artifacts produced by local test execution."""

    _remove_path(REPO_ROOT / ".repo_tmp")

    for search_root in (
        REPO_ROOT,
        REPO_ROOT / "benchmark-comparison",
        REPO_ROOT / "scripts",
    ):
        if not search_root.exists():
            continue
        for cache_dir in search_root.rglob("__pycache__"):
            _remove_path(cache_dir)

    artifact_root = REPO_ROOT / "benchmark-comparison"
    if artifact_root.exists():
        for artifact_dump in artifact_root.glob("ci_artifacts_*"):
            _remove_path(artifact_dump)


atexit.register(_cleanup_repo_local_runtime_artifacts)


class RepoHygieneBootstrapTest(unittest.TestCase):
    def test_repo_hygiene_cleanup_is_registered(self) -> None:
        self.assertTrue(sys.dont_write_bytecode)
