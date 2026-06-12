#!/usr/bin/env python3
"""Validate the static MI300A finishability evidence matrix-contract boundary.

This command is intentionally static and read-only.  It does not query GitHub,
mutate workflows, download artifacts, dispatch/cancel/rerun Actions, run
simulators, or regenerate provenance/finishable/Tier outputs.  It combines the
existing timeout-contract and recovery-boundary validators to prove the
three matrix-contract cases that currently separate per-row-timeout evidence
from finishability evidence:

* the checked-in 940-entry recovery-only path has the requested 14/600/3600
  shape but remains non-completing for the finishability contract under the single-run source
  policy;
* the active full-run branch/run is a separate 6-way, 4320-minute-row,
  3600-second nested-attempt contract and is not per-row-timeout-compliant; and
* a literal full 1416-entry per-benchmark matrix with a 600-second row cap is
  rejected before dispatch by zero-overhead row-budget accounting.
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Mapping, Optional, Sequence

import validate_mi300a_recovery_evidence_readiness_boundary as recovery_boundary
import validate_mi300a_terminal_discovery_timeout_contract as timeout_contract
import validate_mi300a_terminal_recovery_dry_run_plan as recovery_validator


BOUNDARY_SCHEMA = "mgpusim.mi300a_finishability_matrix_contract_boundary"
SCHEMA_VERSION = 1

REQUESTED_278_MAX_PARALLEL = 14
REQUESTED_278_BENCHMARK_RUN_TIMEOUT_SEC = 600
REQUESTED_278_ROW_TIMEOUT_SEC = 600
REQUESTED_278_ACTION_TIMEOUT_SEC = 3600

RECOVERY_CASE = "checked_in_recovery_only_940_14_600_3600"
FULL_RUN_CASE = "archived_full_run_1416_separate_contract"
LITERAL_CASE = "literal_full_1416_per_benchmark_600s_row_cap"

FULL_RUN_RUN_ID = "25196153223"
FULL_RUN_WORKFLOW_PATH = ".github/workflows/benchmark.yml"
FULL_RUN_BRANCH = "archived-full-run-contract"
FULL_RUN_DISPATCH_SHA = "c60f56d19a23b43b020bb4f8e96391c20a2d2454"
FULL_RUN_OPERATOR_PATH = "scripts/archived_terminal_discovery_operator.py"
FULL_RUN_MANIFEST_PATH = "benchmark-comparison/generated/mi300a_terminal_discovery_shard_manifest.json"
FULL_RUN_SHARD_DIR = "benchmark-comparison/generated"
FULL_RUN_FIXTURE_ROOT = "scripts/fixtures/mi300a_matrix_contract_boundary/archived_full_run_contract"
FULL_RUN_FIXTURE_PATHS = {
    FULL_RUN_WORKFLOW_PATH: "benchmark.yml",
    FULL_RUN_OPERATOR_PATH: "archived_terminal_discovery_operator.py",
    FULL_RUN_MANIFEST_PATH: "mi300a_terminal_discovery_shard_manifest.json",
}
FULL_RUN_SOURCE_FIXTURE = "fixture"
FULL_RUN_SOURCE_GIT = "git"
FULL_RUN_ROW_TIMEOUT_MINUTES = 4320
FULL_RUN_ROW_TIMEOUT_SEC = FULL_RUN_ROW_TIMEOUT_MINUTES * 60
FULL_RUN_MAX_PARALLEL = 6
FULL_RUN_NESTED_ATTEMPT_TIMEOUT_SEC = 3600
FULL_RUN_ROW_OVERHEAD_SEC = 1800
FULL_RUN_EXPECTED_ATTEMPTS = 1416
FULL_RUN_EXPECTED_ROWS = 81
FULL_RUN_EXPECTED_SHARDS = 6
FULL_RUN_EXPECTED_MAX_ATTEMPTS_PER_ROW = 54
FULL_RUN_EXPECTED_MAX_REQUIRED_TIMEOUT_SEC = 196200

LITERAL_ROW_TIMEOUT_SEC = 600
LITERAL_PER_ATTEMPT_TIMEOUT_SEC = 600
LITERAL_ROW_OVERHEAD_SEC = 0
LITERAL_EXPECTED_VIOLATIONS = 81
LITERAL_EXPECTED_MAX_REQUIRED_TIMEOUT_SEC = 32400

NON_GOALS = [
    "no .github/workflows changes",
    "no workflow dispatch/cancel/rerun",
    "no artifact download or collection",
    "no simulator execution, long simulation, or calibration",
    "no provenance, finishable manifest, or Tier artifact regeneration",
    "no terminal finishability completion claim",
]


class MatrixContractBoundaryError(Exception):
    """Raised when the static matrix-contract boundary is not enforced."""


# ---------------------------------------------------------------------------
# Generic helpers
# ---------------------------------------------------------------------------


def repo_path(path_text: str) -> Path:
    return timeout_contract.repo_path(path_text)



def repo_relative(path: Path) -> str:
    return timeout_contract.repo_relative(path)



def require_equal(label: str, actual: Any, expected: Any) -> None:
    if actual != expected:
        raise MatrixContractBoundaryError(f"{label} must be {expected!r}, got {actual!r}")



def require_true(label: str, actual: Any) -> None:
    if actual is not True:
        raise MatrixContractBoundaryError(f"{label} must be true, got {actual!r}")



def require_false(label: str, actual: Any) -> None:
    if actual is not False:
        raise MatrixContractBoundaryError(f"{label} must be false, got {actual!r}")



def require_mapping(value: Any, label: str) -> Mapping[str, Any]:
    if not isinstance(value, Mapping):
        raise MatrixContractBoundaryError(f"{label} must be an object")
    return value



def require_sequence(value: Any, label: str) -> list[Any]:
    if not isinstance(value, list):
        raise MatrixContractBoundaryError(f"{label} must be a list")
    return value



def as_int(value: Any, label: str) -> int:
    try:
        return timeout_contract.as_int(value, label)
    except timeout_contract.TimeoutContractError as exc:
        raise MatrixContractBoundaryError(str(exc)) from exc



def git_show_text(ref: str, path: str) -> str:
    """Return a file from a local git object without fetching or contacting GitHub."""

    cmd = ["git", "show", f"{ref}:{path}"]
    result = subprocess.run(
        cmd,
        cwd=timeout_contract.generator.REPO_ROOT,
        text=True,
        capture_output=True,
        check=False,
        timeout=30,
    )
    if result.returncode != 0:
        message = result.stderr.strip() or result.stdout.strip() or "unknown git-show failure"
        raise MatrixContractBoundaryError(f"git show {ref}:{path} failed: {message}")
    return result.stdout



def git_show_json_object(ref: str, path: str) -> Mapping[str, Any]:
    text = git_show_text(ref, path)
    try:
        data = json.loads(text)
    except json.JSONDecodeError as exc:
        raise MatrixContractBoundaryError(f"git object {ref}:{path} is invalid JSON: {exc}") from exc
    if not isinstance(data, Mapping):
        raise MatrixContractBoundaryError(f"git object {ref}:{path} JSON root must be an object")
    return data



@dataclass(frozen=True)
class FullRunArtifactSource:
    """Deterministic source for the archived full-run contract inputs."""

    source_type: str
    ref: str = FULL_RUN_DISPATCH_SHA
    fixture_root: str = FULL_RUN_FIXTURE_ROOT

    @classmethod
    def fixture(cls) -> "FullRunArtifactSource":
        return cls(source_type=FULL_RUN_SOURCE_FIXTURE)

    @classmethod
    def git(cls, ref: str) -> "FullRunArtifactSource":
        return cls(source_type=FULL_RUN_SOURCE_GIT, ref=ref)

    def _relative_artifact_path(self, path: str) -> Path:
        artifact_path = Path(path)
        if artifact_path.is_absolute() or ".." in artifact_path.parts:
            raise MatrixContractBoundaryError(f"full-run artifact path must be repo-relative: {path!r}")
        return artifact_path

    def fixture_path(self, path: str) -> Path:
        artifact_path = self._relative_artifact_path(path)
        fixture_path_text = FULL_RUN_FIXTURE_PATHS.get(path)
        if fixture_path_text is None and path.startswith(f"{FULL_RUN_SHARD_DIR}/"):
            fixture_path_text = artifact_path.name
        if fixture_path_text is None:
            fixture_path_text = str(artifact_path)
        return repo_path(self.fixture_root) / fixture_path_text

    def label(self, path: str) -> str:
        if self.source_type == FULL_RUN_SOURCE_GIT:
            return f"{self.ref}:{path}"
        return repo_relative(self.fixture_path(path))

    def read_text(self, path: str) -> str:
        if self.source_type == FULL_RUN_SOURCE_GIT:
            return git_show_text(self.ref, path)
        if self.source_type != FULL_RUN_SOURCE_FIXTURE:
            raise MatrixContractBoundaryError(f"unknown full-run artifact source {self.source_type!r}")
        fixture_path = self.fixture_path(path)
        try:
            return fixture_path.read_text(encoding="utf-8")
        except FileNotFoundError as exc:
            raise MatrixContractBoundaryError(f"fixture {repo_relative(fixture_path)}: file not found") from exc
        except OSError as exc:
            raise MatrixContractBoundaryError(f"fixture {repo_relative(fixture_path)}: cannot read: {exc}") from exc

    def read_json_object(self, path: str) -> Mapping[str, Any]:
        text = self.read_text(path)
        label = self.label(path)
        try:
            data = json.loads(text)
        except json.JSONDecodeError as exc:
            raise MatrixContractBoundaryError(f"{label}: invalid JSON: {exc}") from exc
        if not isinstance(data, Mapping):
            raise MatrixContractBoundaryError(f"{label}: JSON root must be an object")
        return data

    def report_source(self) -> dict[str, Any]:
        if self.source_type == FULL_RUN_SOURCE_GIT:
            return {
                "source_type": "local_git_object",
                "ref": self.ref,
                "requires_local_git_object": True,
            }
        return {
            "source_type": "checked_in_fixture",
            "fixture_root": self.fixture_root,
            "captured_ref": FULL_RUN_DISPATCH_SHA,
            "requires_local_git_object": False,
        }

    def identity_source_text(self) -> str:
        if self.source_type == FULL_RUN_SOURCE_GIT:
            return "checked-in read-only monitoring snapshot plus local git object inspection; no live GitHub query"
        return (
            "checked-in deterministic archived full-run fixture captured from the dispatch SHA; "
            "no local git object or live GitHub query"
        )

    def enforced_by_text(self) -> str:
        if self.source_type == FULL_RUN_SOURCE_GIT:
            return "local git object inspection of the full-run branch/run workflow and operator contract"
        return "checked-in deterministic fixture of the full-run branch/run workflow and operator contract"



def default_full_run_source() -> FullRunArtifactSource:
    return FullRunArtifactSource.fixture()



def git_full_run_source(ref: str) -> FullRunArtifactSource:
    return FullRunArtifactSource.git(ref)



def coerce_full_run_source(source_or_ref: FullRunArtifactSource | str | None) -> FullRunArtifactSource:
    if source_or_ref is None or source_or_ref == FULL_RUN_DISPATCH_SHA:
        return default_full_run_source()
    if isinstance(source_or_ref, FullRunArtifactSource):
        return source_or_ref
    return git_full_run_source(source_or_ref)



def workflow_job_block(workflow_text: str, job_name: str) -> str:
    pattern = rf"^  {re.escape(job_name)}:\n(?P<body>.*?)(?=^  [A-Za-z0-9_-]+:\n|\Z)"
    match = re.search(pattern, workflow_text, flags=re.MULTILINE | re.DOTALL)
    if match is None:
        raise MatrixContractBoundaryError(f"workflow job {job_name!r} not found in full-run workflow")
    return match.group("body")



def first_int(pattern: str, text: str, label: str) -> int:
    match = re.search(pattern, text, flags=re.MULTILINE)
    if match is None:
        raise MatrixContractBoundaryError(f"{label} not found")
    return int(match.group(1))



def workflow_arg_int(text: str, arg_name: str, label: str) -> int:
    return first_int(rf"{re.escape(arg_name)}\s+([0-9]+)\b", text, label)



def operator_int_constant(operator_text: str, name: str) -> int:
    return first_int(rf"^{re.escape(name)}\s*=\s*([0-9]+)\s*$", operator_text, f"operator constant {name}")



def operator_str_constant(operator_text: str, name: str) -> str:
    match = re.search(rf"^{re.escape(name)}\s*=\s*['\"]([^'\"]+)['\"]\s*$", operator_text, flags=re.MULTILINE)
    if match is None:
        raise MatrixContractBoundaryError(f"operator constant {name} not found")
    return match.group(1)


# ---------------------------------------------------------------------------
# Reused timeout-contract validation over archived full-run artifacts
# ---------------------------------------------------------------------------


def matrix_path_from_full_run_shard(shard: Mapping[str, Any], shard_dir: str, label: str) -> str:
    matrix_path = timeout_contract.generator.str_or_none(shard.get("matrix_path"))
    if matrix_path is not None:
        return matrix_path
    shard_index = as_int(shard.get("shard_index"), f"{label}.shard_index")
    return f"{shard_dir.rstrip('/')}/{timeout_contract.generator.matrix_filename(shard_index)}"



matrix_path_from_git_shard = matrix_path_from_full_run_shard



def validate_full_run_timeout_contract(
    *,
    source: FullRunArtifactSource,
    manifest_path: str,
    shard_dir: str,
    per_attempt_timeout_sec: int,
    row_timeout_sec: int,
    row_overhead_sec: int,
    max_violations: int,
) -> dict[str, Any]:
    """Reuse timeout-contract row accounting against archived full-run matrix JSON."""

    manifest = source.read_json_object(manifest_path)
    require_equal(
        f"{source.label(manifest_path)}.schema_name",
        manifest.get("schema_name"),
        timeout_contract.generator.MANIFEST_SCHEMA,
    )
    rows: list[dict[str, Any]] = []
    shards = [
        require_mapping(shard, f"{source.label(manifest_path)}.shards[{offset}]")
        for offset, shard in enumerate(
            require_sequence(manifest.get("shards"), f"{source.label(manifest_path)}.shards")
        )
    ]
    for shard_offset, shard in enumerate(shards):
        shard_label = f"{source.label(manifest_path)}.shards[{shard_offset}]"
        shard_index = as_int(shard.get("shard_index"), f"{shard_label}.shard_index")
        matrix_path_text = matrix_path_from_full_run_shard(shard, shard_dir, shard_label)
        matrix = source.read_json_object(matrix_path_text)
        matrix_label = source.label(matrix_path_text)
        require_equal(
            f"{matrix_label}.shard_index",
            as_int(matrix.get("shard_index"), f"{matrix_label}.shard_index"),
            shard_index,
        )
        include = [
            require_mapping(row, f"{matrix_label}.include[{offset}]")
            for offset, row in enumerate(require_sequence(matrix.get("include"), f"{matrix_label}.include"))
        ]
        declared_rows = as_int(
            matrix.get("visible_matrix_row_count", matrix.get("benchmark_row_count")),
            f"{matrix_label}.visible_matrix_row_count",
        )
        require_equal(f"{matrix_label}.visible_matrix_row_count", declared_rows, len(include))
        matrix_path = repo_path(matrix_path_text)
        for row_offset, row in enumerate(include):
            try:
                rows.append(
                    timeout_contract.row_contract_record(
                        matrix_path=matrix_path,
                        matrix=matrix,
                        row=row,
                        row_offset=row_offset,
                        per_attempt_timeout_sec=per_attempt_timeout_sec,
                        row_timeout_sec=row_timeout_sec,
                        row_overhead_sec=row_overhead_sec,
                    )
                )
            except timeout_contract.TimeoutContractError as exc:
                raise MatrixContractBoundaryError(str(exc)) from exc

    try:
        return timeout_contract.summarize_contract_rows(
            manifest,
            rows,
            manifest_path=repo_path(manifest_path),
            shard_dir=repo_path(shard_dir),
            per_attempt_timeout_sec=per_attempt_timeout_sec,
            row_timeout_sec=row_timeout_sec,
            row_overhead_sec=row_overhead_sec,
            max_violations=max_violations,
        )
    except timeout_contract.TimeoutContractError as exc:
        raise MatrixContractBoundaryError(str(exc)) from exc



def validate_git_timeout_contract(
    *,
    ref: str,
    manifest_path: str,
    shard_dir: str,
    per_attempt_timeout_sec: int,
    row_timeout_sec: int,
    row_overhead_sec: int,
    max_violations: int,
) -> dict[str, Any]:
    """Compatibility wrapper for explicitly requested local-git validation."""

    return validate_full_run_timeout_contract(
        source=git_full_run_source(ref),
        manifest_path=manifest_path,
        shard_dir=shard_dir,
        per_attempt_timeout_sec=per_attempt_timeout_sec,
        row_timeout_sec=row_timeout_sec,
        row_overhead_sec=row_overhead_sec,
        max_violations=max_violations,
    )


# ---------------------------------------------------------------------------
# Case proofs
# ---------------------------------------------------------------------------


def requested_278_contract() -> dict[str, Any]:
    return {
        "max_parallel": REQUESTED_278_MAX_PARALLEL,
        "benchmark_run_timeout_sec": REQUESTED_278_BENCHMARK_RUN_TIMEOUT_SEC,
        "row_timeout_sec": REQUESTED_278_ROW_TIMEOUT_SEC,
        "action_timeout_sec": REQUESTED_278_ACTION_TIMEOUT_SEC,
        "description": "14-way parallelism, 600-second benchmark-row/run layer, 3600-second action/batch layer",
    }



def build_recovery_only_case(plan_path: Path, manifest_path: Path) -> dict[str, Any]:
    try:
        dry_run_report = recovery_validator.validate_recovery_dry_run_plan(plan_path, manifest_path)
        boundary_report = recovery_boundary.build_boundary_report(plan_path, manifest_path)
        plan = recovery_validator.load_json_object(plan_path)
    except (recovery_validator.ValidationError, recovery_boundary.ReadinessBoundaryError) as exc:
        raise MatrixContractBoundaryError(f"recovery-only boundary validation failed: {exc}") from exc

    require_equal("recovery dry-run validation_status", dry_run_report.get("validation_status"), "pass")
    coverage = require_mapping(dry_run_report.get("coverage"), "recovery dry-run coverage")
    contract = require_mapping(plan.get("operator_contract"), "recovery dry-run operator_contract")
    timeout_summary = require_mapping(plan.get("timeout_contract_summary"), "recovery dry-run timeout_contract_summary")

    require_true("recovery dry_run", plan.get("dry_run"))
    require_true("recovery recovery_only", plan.get("recovery_only"))
    require_equal(
        "recovery_entry_count",
        as_int(plan.get("recovery_entry_count"), "recovery_entry_count"),
        940,
    )
    require_equal(
        "dry_run_matrix_row_count",
        as_int(plan.get("dry_run_matrix_row_count"), "dry_run_matrix_row_count"),
        940,
    )
    require_equal(
        "prior_observed_plan_index_count",
        as_int(plan.get("prior_observed_plan_index_count"), "prior_observed_plan_index_count"),
        476,
    )
    require_equal(
        "operator_contract.max_parallel",
        as_int(contract.get("max_parallel"), "operator_contract.max_parallel"),
        REQUESTED_278_MAX_PARALLEL,
    )
    require_equal(
        "operator_contract.attempts_per_matrix_row",
        as_int(contract.get("attempts_per_matrix_row"), "operator_contract.attempts_per_matrix_row"),
        1,
    )
    require_equal(
        "operator_contract.per_attempt_timeout_sec",
        as_int(contract.get("per_attempt_timeout_sec"), "operator_contract.per_attempt_timeout_sec"),
        REQUESTED_278_ROW_TIMEOUT_SEC,
    )
    require_equal(
        "operator_contract.benchmark_run_timeout_sec",
        as_int(contract.get("benchmark_run_timeout_sec"), "operator_contract.benchmark_run_timeout_sec"),
        REQUESTED_278_BENCHMARK_RUN_TIMEOUT_SEC,
    )
    require_equal(
        "operator_contract.row_timeout_sec",
        as_int(contract.get("row_timeout_sec"), "operator_contract.row_timeout_sec"),
        REQUESTED_278_ROW_TIMEOUT_SEC,
    )
    require_equal(
        "operator_contract.action_timeout_sec",
        as_int(contract.get("action_timeout_sec"), "operator_contract.action_timeout_sec"),
        REQUESTED_278_ACTION_TIMEOUT_SEC,
    )
    require_equal(
        "recovery row violations",
        as_int(timeout_summary.get("row_violation_count"), "row_violation_count"),
        0,
    )
    require_equal(
        "recovery batch violations",
        as_int(timeout_summary.get("batch_violation_count"), "batch_violation_count"),
        0,
    )
    require_equal(
        "recovery max_batch_required_timeout_sec",
        as_int(timeout_summary.get("max_batch_required_timeout_sec"), "max_batch_required_timeout_sec"),
        REQUESTED_278_ACTION_TIMEOUT_SEC,
    )
    require_true("coverage.not_complete_terminal_provenance", coverage.get("not_complete_terminal_provenance"))

    decision = require_mapping(boundary_report.get("decision"), "recovery boundary decision")
    require_equal("recovery boundary status", decision.get("status"), recovery_boundary.BLOCKED_DECISION)
    for key in [
        "can_combine_476_prior_with_future_940_recovery_artifacts",
        "can_collect_terminal_provenance_from_mixed_union",
        "can_regenerate_finishable_manifest",
        "can_regenerate_tier_artifacts",
        "can_regenerate_terminal_provenance_for_completion",
        "can_claim_issue_101_complete",
    ]:
        require_false(f"recovery boundary decision.{key}", decision.get(key))

    return {
        "case": RECOVERY_CASE,
        "validation_status": "pass",
        "source": {
            "dry_run_plan": repo_relative(plan_path),
            "dry_run_plan_sha256": dry_run_report["plan_sha256"],
            "recovery_manifest": repo_relative(manifest_path),
            "recovery_manifest_sha256": dry_run_report["recovery_manifest_sha256"],
        },
        "shape": {
            "recovery_only": True,
            "entry_count": 940,
            "matrix_row_count": 940,
            "attempts_per_matrix_row": 1,
            "prior_observed_record_count": 476,
            "represented_plan_index_count_after_accounting_union": 1416,
            "max_parallel": REQUESTED_278_MAX_PARALLEL,
            "per_attempt_timeout_sec": REQUESTED_278_ROW_TIMEOUT_SEC,
            "benchmark_run_timeout_sec": REQUESTED_278_BENCHMARK_RUN_TIMEOUT_SEC,
            "row_timeout_sec": REQUESTED_278_ROW_TIMEOUT_SEC,
            "action_timeout_sec": REQUESTED_278_ACTION_TIMEOUT_SEC,
            "row_violation_count": 0,
            "batch_violation_count": 0,
            "max_batch_required_timeout_sec": REQUESTED_278_ACTION_TIMEOUT_SEC,
        },
        "is_278_shaped_recovery_path": True,
        "is_complete_1416_entry_single_run_terminal_provenance": False,
        "is_issue_101_completion_evidence": False,
        "can_regenerate_finishable_manifest": False,
        "can_regenerate_tier_artifacts": False,
        "m32_source_policy_decision": {
            "status": decision["status"],
            "can_claim_issue_101_complete": False,
            "can_combine_476_prior_with_future_940_recovery_artifacts": False,
            "reason": decision["reason"],
        },
    }



def build_full_run_contract_case(
    source_or_ref: FullRunArtifactSource | str | None = None,
    max_violations: int = timeout_contract.DEFAULT_MAX_VIOLATIONS,
) -> dict[str, Any]:
    source = coerce_full_run_source(source_or_ref)
    workflow_text = source.read_text(FULL_RUN_WORKFLOW_PATH)
    operator_text = source.read_text(FULL_RUN_OPERATOR_PATH)

    validation_job = workflow_job_block(workflow_text, "archived-full-run-validation")
    discovery_job = workflow_job_block(workflow_text, "archived-full-run-discovery")
    summary_job = workflow_job_block(workflow_text, "archived-full-run-summary")

    validation_row_timeout_minutes = workflow_arg_int(
        validation_job,
        "--row-timeout-minutes",
        "full-run validation --row-timeout-minutes",
    )
    validation_per_attempt_timeout_sec = workflow_arg_int(
        validation_job,
        "--per-attempt-timeout-sec",
        "full-run validation --per-attempt-timeout-sec",
    )
    validation_row_overhead_sec = workflow_arg_int(
        validation_job,
        "--row-overhead-sec",
        "full-run validation --row-overhead-sec",
    )
    discovery_timeout_minutes = first_int(
        r"^    timeout-minutes:\s*([0-9]+)\s*$",
        discovery_job,
        "full-run row timeout-minutes",
    )
    discovery_max_parallel = first_int(
        r"^      max-parallel:\s*([0-9]+)\s*$",
        discovery_job,
        "full-run max-parallel",
    )
    summary_timeout_minutes = first_int(
        r"^    timeout-minutes:\s*([0-9]+)\s*$",
        summary_job,
        "full-run summary timeout-minutes",
    )

    require_equal(
        "full-run validation row timeout minutes",
        validation_row_timeout_minutes,
        FULL_RUN_ROW_TIMEOUT_MINUTES,
    )
    require_equal(
        "full-run validation per-attempt timeout",
        validation_per_attempt_timeout_sec,
        FULL_RUN_NESTED_ATTEMPT_TIMEOUT_SEC,
    )
    require_equal(
        "full-run validation row overhead",
        validation_row_overhead_sec,
        FULL_RUN_ROW_OVERHEAD_SEC,
    )
    require_equal(
        "full-run discovery timeout minutes",
        discovery_timeout_minutes,
        FULL_RUN_ROW_TIMEOUT_MINUTES,
    )
    require_equal(
        "full-run discovery max-parallel",
        discovery_max_parallel,
        FULL_RUN_MAX_PARALLEL,
    )
    require_equal(
        "full-run operator EXPECTED_BRANCH",
        operator_str_constant(operator_text, "EXPECTED_BRANCH"),
        FULL_RUN_BRANCH,
    )
    require_equal(
        "full-run operator EXPECTED_ATTEMPTS",
        operator_int_constant(operator_text, "EXPECTED_ATTEMPTS"),
        FULL_RUN_EXPECTED_ATTEMPTS,
    )
    require_equal(
        "full-run operator EXPECTED_ROWS",
        operator_int_constant(operator_text, "EXPECTED_ROWS"),
        FULL_RUN_EXPECTED_ROWS,
    )
    require_equal(
        "full-run operator EXPECTED_SHARDS",
        operator_int_constant(operator_text, "EXPECTED_SHARDS"),
        FULL_RUN_EXPECTED_SHARDS,
    )
    require_equal(
        "full-run operator EXPECTED_TIMEOUT_SEC",
        operator_int_constant(operator_text, "EXPECTED_TIMEOUT_SEC"),
        FULL_RUN_NESTED_ATTEMPT_TIMEOUT_SEC,
    )
    if (
        FULL_RUN_BRANCH not in validation_job
        or FULL_RUN_BRANCH not in discovery_job
        or FULL_RUN_BRANCH not in summary_job
    ):
        raise MatrixContractBoundaryError(
            "full-run workflow jobs must all be guarded to the full-run branch"
        )

    timeout_report = validate_full_run_timeout_contract(
        source=source,
        manifest_path=FULL_RUN_MANIFEST_PATH,
        shard_dir=FULL_RUN_SHARD_DIR,
        per_attempt_timeout_sec=FULL_RUN_NESTED_ATTEMPT_TIMEOUT_SEC,
        row_timeout_sec=FULL_RUN_ROW_TIMEOUT_SEC,
        row_overhead_sec=FULL_RUN_ROW_OVERHEAD_SEC,
        max_violations=max_violations,
    )
    require_equal(
        "full-run timeout validation_status",
        timeout_report.get("validation_status"),
        "pass",
    )
    require_true("full-run timeout validated", timeout_report.get("validated"))
    require_equal(
        "full-run timeout row_count",
        timeout_report.get("row_count"),
        FULL_RUN_EXPECTED_ROWS,
    )
    require_equal(
        "full-run timeout attempt_count",
        timeout_report.get("attempt_count"),
        FULL_RUN_EXPECTED_ATTEMPTS,
    )
    require_equal(
        "full-run timeout shard_count",
        timeout_report.get("shard_count"),
        FULL_RUN_EXPECTED_SHARDS,
    )
    require_equal(
        "full-run timeout violation_count",
        timeout_report.get("violation_count"),
        0,
    )
    require_equal(
        "full-run timeout max_attempt_count",
        timeout_report.get("max_attempt_count"),
        FULL_RUN_EXPECTED_MAX_ATTEMPTS_PER_ROW,
    )
    require_equal(
        "full-run timeout max_required_timeout_sec",
        timeout_report.get("max_required_timeout_sec"),
        FULL_RUN_EXPECTED_MAX_REQUIRED_TIMEOUT_SEC,
    )

    mismatches = [
        {
            "field": "max_parallel",
            "requested_278": REQUESTED_278_MAX_PARALLEL,
            "full_run_value": discovery_max_parallel,
        },
        {
            "field": "row_timeout_sec",
            "requested_278": REQUESTED_278_ROW_TIMEOUT_SEC,
            "full_run_value": discovery_timeout_minutes * 60,
        },
        {
            "field": "nested_attempt_timeout_sec",
            "requested_278_row_layer_sec": REQUESTED_278_BENCHMARK_RUN_TIMEOUT_SEC,
            "full_run_value": FULL_RUN_NESTED_ATTEMPT_TIMEOUT_SEC,
        },
        {
            "field": "action_layer_timeout_sec",
            "requested_278": REQUESTED_278_ACTION_TIMEOUT_SEC,
            "full_run_row_timeout_sec": discovery_timeout_minutes * 60,
            "full_run_summary_timeout_sec": summary_timeout_minutes * 60,
        },
    ]

    return {
        "case": FULL_RUN_CASE,
        "validation_status": "pass",
        "run_identity": {
            "run_id": FULL_RUN_RUN_ID,
            "workflow_path": FULL_RUN_WORKFLOW_PATH,
            "head_branch": FULL_RUN_BRANCH,
            "head_sha": FULL_RUN_DISPATCH_SHA,
            "inspected_git_ref": source.ref,
            "artifact_source": source.report_source(),
            "identity_source": source.identity_source_text(),
        },
        "workflow_contract": {
            "job": "archived-full-run-discovery",
            "strategy_max_parallel": discovery_max_parallel,
            "row_timeout_minutes": discovery_timeout_minutes,
            "row_timeout_sec": discovery_timeout_minutes * 60,
            "validation_row_timeout_minutes": validation_row_timeout_minutes,
            "validation_per_attempt_timeout_sec": validation_per_attempt_timeout_sec,
            "validation_row_overhead_sec": validation_row_overhead_sec,
            "summary_timeout_minutes": summary_timeout_minutes,
        },
        "operator_contract": {
            "expected_branch": FULL_RUN_BRANCH,
            "expected_attempts": FULL_RUN_EXPECTED_ATTEMPTS,
            "expected_rows": FULL_RUN_EXPECTED_ROWS,
            "expected_shards": FULL_RUN_EXPECTED_SHARDS,
            "nested_attempt_timeout_sec": FULL_RUN_NESTED_ATTEMPT_TIMEOUT_SEC,
        },
        "full_matrix_timeout_contract": {
            "validation_status": timeout_report["validation_status"],
            "row_count": timeout_report["row_count"],
            "attempt_count": timeout_report["attempt_count"],
            "shard_count": timeout_report["shard_count"],
            "max_attempt_count": timeout_report["max_attempt_count"],
            "max_required_timeout_sec": timeout_report["max_required_timeout_sec"],
            "row_timeout_sec": timeout_report["contract"]["row_timeout_sec"],
            "per_attempt_timeout_sec": timeout_report["contract"]["per_attempt_timeout_sec"],
            "row_overhead_sec": timeout_report["contract"]["row_overhead_sec"],
            "violation_count": timeout_report["violation_count"],
            "tightest_row": timeout_report["tightest_row"],
        },
        "requested_278_contract": requested_278_contract(),
        "mismatches_with_requested_278_contract": mismatches,
        "is_full_1416_entry_branch_run": True,
        "is_issue_278_compliant": False,
        "must_not_label_issue_278_compliant": True,
        "separate_contract": "guarded full terminal evidence launch/readiness contract",
        "reason": (
            "The full-run branch/run uses six-way row parallelism, 4320-minute row jobs, "
            "and 3600-second nested attempts over the full 1416-entry matrix.  That is "
            "a different contract from the per-row-timeout 14-way / 600-second row-run / 3600-second "
            "action-layer request."
        ),
    }



def build_literal_600s_full_matrix_case(max_violations: int) -> dict[str, Any]:
    try:
        timeout_report = timeout_contract.validate_timeout_contract(
            timeout_contract.generator.DEFAULT_MANIFEST,
            timeout_contract.generator.DEFAULT_GENERATED_DIR,
            per_attempt_timeout_sec=LITERAL_PER_ATTEMPT_TIMEOUT_SEC,
            row_timeout_sec=LITERAL_ROW_TIMEOUT_SEC,
            row_overhead_sec=LITERAL_ROW_OVERHEAD_SEC,
            max_violations=max_violations,
        )
    except timeout_contract.TimeoutContractError as exc:
        raise MatrixContractBoundaryError(
            f"literal full-matrix timeout-contract validation failed: {exc}"
        ) from exc

    require_equal("literal timeout validation_status", timeout_report.get("validation_status"), "fail")
    require_false("literal timeout validated", timeout_report.get("validated"))
    require_equal("literal row_count", timeout_report.get("row_count"), 82)
    require_equal("literal attempt_count", timeout_report.get("attempt_count"), 1416)
    require_equal("literal shard_count", timeout_report.get("shard_count"), 6)
    require_equal(
        "literal violation_count",
        timeout_report.get("violation_count"),
        LITERAL_EXPECTED_VIOLATIONS,
    )
    require_equal(
        "literal max_required_timeout_sec",
        timeout_report.get("max_required_timeout_sec"),
        LITERAL_EXPECTED_MAX_REQUIRED_TIMEOUT_SEC,
    )
    require_equal(
        "literal row_timeout_sec",
        timeout_report["contract"].get("row_timeout_sec"),
        LITERAL_ROW_TIMEOUT_SEC,
    )
    require_equal(
        "literal per_attempt_timeout_sec",
        timeout_report["contract"].get("per_attempt_timeout_sec"),
        LITERAL_PER_ATTEMPT_TIMEOUT_SEC,
    )
    require_equal(
        "literal row_overhead_sec",
        timeout_report["contract"].get("row_overhead_sec"),
        LITERAL_ROW_OVERHEAD_SEC,
    )
    tightest = require_mapping(timeout_report.get("tightest_row"), "literal tightest_row")
    require_equal(
        "literal tightest attempt_count",
        tightest.get("attempt_count"),
        FULL_RUN_EXPECTED_MAX_ATTEMPTS_PER_ROW,
    )
    require_equal(
        "literal tightest required_timeout_sec",
        tightest.get("required_timeout_sec"),
        LITERAL_EXPECTED_MAX_REQUIRED_TIMEOUT_SEC,
    )

    return {
        "case": LITERAL_CASE,
        "validation_status": "pass",
        "candidate_contract": {
            "full_matrix_entry_count": 1416,
            "visible_benchmark_row_count": 82,
            "shard_count": 6,
            "row_timeout_sec": LITERAL_ROW_TIMEOUT_SEC,
            "per_attempt_timeout_sec": LITERAL_PER_ATTEMPT_TIMEOUT_SEC,
            "row_overhead_sec": LITERAL_ROW_OVERHEAD_SEC,
            "formula": timeout_contract.CONTRACT_FORMULA,
        },
        "pre_dispatch_rejected": True,
        "timeout_contract_validation_status": timeout_report["validation_status"],
        "violation_count": timeout_report["violation_count"],
        "violating_matrix_row_ids": timeout_report["violating_matrix_row_ids"],
        "violations_truncated": timeout_report["violations_truncated"],
        "violations": timeout_report["violations"],
        "zero_overhead_max_row_evidence": {
            "matrix_row_id": tightest["matrix_row_id"],
            "benchmark": tightest["benchmark"],
            "attempt_count": tightest["attempt_count"],
            "per_attempt_timeout_sec": tightest["per_attempt_timeout_sec"],
            "row_overhead_sec": tightest["row_overhead_sec"],
            "required_timeout_sec": tightest["required_timeout_sec"],
            "row_timeout_sec": tightest["row_timeout_sec"],
            "slack_sec": tightest["slack_sec"],
        },
        "must_not_dispatch": True,
        "reason": (
            "The compact full per-benchmark matrix has rows with multiple sequential attempts; "
            "with a 600-second row cap and zero overhead, 81 of 82 rows exceed the cap and "
            "the largest row requires 32,400 seconds."
        ),
    }



def build_boundary_report(
    *,
    recovery_plan_path: Path,
    recovery_manifest_path: Path,
    full_run_source: FullRunArtifactSource | str | None = None,
    full_run_ref: str | None = None,
    max_violations: int,
) -> dict[str, Any]:
    if full_run_ref is not None:
        if full_run_source is not None:
            raise MatrixContractBoundaryError("full_run_ref and full_run_source are mutually exclusive")
        full_run_source = full_run_ref
    source = coerce_full_run_source(full_run_source)
    recovery_case = build_recovery_only_case(recovery_plan_path, recovery_manifest_path)
    full_run_case = build_full_run_contract_case(source, max_violations=max_violations)
    literal_case = build_literal_600s_full_matrix_case(max_violations=max_violations)

    return {
        "schema_name": BOUNDARY_SCHEMA,
        "schema_version": SCHEMA_VERSION,
        "validation_status": "pass",
        "validated": True,
        "read_only": True,
        "pre_dispatch_static_validation": True,
        "requested_278_contract": requested_278_contract(),
        "inputs": {
            "recovery_dry_run_plan": repo_relative(recovery_plan_path),
            "recovery_manifest": repo_relative(recovery_manifest_path),
            "run_id": FULL_RUN_RUN_ID,
            "branch": FULL_RUN_BRANCH,
            "run_head_sha": FULL_RUN_DISPATCH_SHA,
            "inspected_git_ref": source.ref,
            "full_run_artifact_source": source.report_source(),
            "full_matrix_manifest": repo_relative(timeout_contract.generator.DEFAULT_MANIFEST),
            "full_matrix_shard_dir": repo_relative(timeout_contract.generator.DEFAULT_GENERATED_DIR),
        },
        "cases": [recovery_case, full_run_case, literal_case],
        "decision": {
            "status": "matrix_contract_boundary_enforced",
            "can_label_recovery_only_path_issue_101_complete": False,
            "can_regenerate_from_recovery_only_or_476_plus_940_union": False,
            "can_label_archived_full_run_issue_278_compliant": False,
            "can_dispatch_literal_full_1416_per_benchmark_600s_row_cap": False,
            "can_claim_issue_101_complete": False,
            "can_claim_issue_278_complete": False,
            "next_step": (
                "review/rescope a coherent matrix contract before any "
                "cancel/relaunch/collection/regeneration decision"
            ),
        },
        "enforced_by": [
            (
                "scripts/validate_mi300a_terminal_recovery_dry_run_plan.py "
                "940-entry recovery-only 14/600/3600 shape"
            ),
            (
                "scripts/validate_mi300a_recovery_evidence_readiness_boundary.py "
                "single-run source-policy block"
            ),
            (
                "scripts/validate_mi300a_terminal_discovery_timeout_contract.py "
                "pre-dispatch row timeout accounting"
            ),
            source.enforced_by_text(),
        ],
        "non_goals": NON_GOALS,
    }


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--recovery-dry-run-plan",
        default=str(recovery_validator.dry_run.DEFAULT_OUTPUT),
        help="Path to the checked-in 940-entry recovery dry-run plan JSON",
    )
    parser.add_argument(
        "--recovery-manifest",
        default=str(recovery_validator.dry_run.DEFAULT_RECOVERY_MANIFEST),
        help="Path to the checked-in recovery manifest JSON",
    )
    parser.add_argument(
        "--full-run-source",
        choices=[FULL_RUN_SOURCE_FIXTURE, FULL_RUN_SOURCE_GIT],
        default=FULL_RUN_SOURCE_FIXTURE,
        help=(
            "Source for archived full-run branch/run workflow, operator, and matrix inputs "
            "(default: checked-in deterministic fixture; git requires the local object)."
        ),
    )
    parser.add_argument(
        "--full-run-ref",
        default=FULL_RUN_DISPATCH_SHA,
        help=(
            "Dispatch ref/SHA recorded for the full-run branch/run. With --full-run-source git, "
            "this local git ref is inspected directly."
        ),
    )
    parser.add_argument(
        "--max-violations",
        type=timeout_contract.positive_int_arg,
        default=timeout_contract.DEFAULT_MAX_VIOLATIONS,
        help="Maximum violating full-matrix row records to include in nested reports",
    )
    return parser.parse_args(argv)



def full_run_source_from_args(args: argparse.Namespace) -> FullRunArtifactSource:
    if args.full_run_source == FULL_RUN_SOURCE_GIT:
        return git_full_run_source(args.full_run_ref)
    if args.full_run_ref != FULL_RUN_DISPATCH_SHA:
        raise MatrixContractBoundaryError(
            "--full-run-ref can only be changed with --full-run-source git; "
            f"the checked-in fixture captures {FULL_RUN_DISPATCH_SHA}"
        )
    return default_full_run_source()



def main(argv: Optional[Sequence[str]] = None) -> int:
    args = parse_args(argv)
    try:
        report = build_boundary_report(
            recovery_plan_path=recovery_validator.repo_path(args.recovery_dry_run_plan),
            recovery_manifest_path=recovery_validator.repo_path(args.recovery_manifest),
            full_run_source=full_run_source_from_args(args),
            max_violations=args.max_violations,
        )
    except MatrixContractBoundaryError as exc:
        print("FAIL: MI300A finishability evidence matrix-contract boundary is not enforced.", file=sys.stderr)
        print(f"  - {exc}", file=sys.stderr)
        return 1

    print(json.dumps(report, indent=2) + "\n", end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
