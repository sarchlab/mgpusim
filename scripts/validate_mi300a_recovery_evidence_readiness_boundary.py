#!/usr/bin/env python3
"""Validate the static MI300A finishability recovery-evidence readiness boundary.

This command is intentionally read-only.  It consumes the checked-in recovery
manifest and dry-run plan for cancelled run 25111815292, then verifies that the
current terminal-provenance schema still requires one successful single-run
source identity.  A passing result is therefore a *blocked* readiness decision:
the 476 observations from the cancelled run cannot be unioned with a future
940-entry recovery-only artifact set to complete the finishability evidence or
regenerate provenance, finishable manifests, or Tier views.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any, Mapping, Optional, Sequence

import collect_mi300a_terminal_discovery_provenance as collector
import generate_mi300a_finishable_size_manifest as finishable
import validate_mi300a_terminal_recovery_dry_run_plan as recovery_validator


BOUNDARY_SCHEMA = "mgpusim.mi300a_recovery_evidence_readiness_boundary"
SCHEMA_VERSION = 1

EXPECTED_SOURCE_POLICY_SCHEMA = "mgpusim.mi300a_terminal_source_policy"
EXPECTED_SOURCE_POLICY_NAME = "single_run_terminal_source_identity"
BLOCKED_DECISION = "blocked_under_current_reviewed_schema"
EXPECTED_SOURCE_RUN_ID = "25111815292"
EXPECTED_SOURCE_RUN_CONCLUSION = "cancelled"
EXPECTED_SOURCE_PLAN_COUNT = 1416
EXPECTED_RECOVERY_ONLY_COUNT = 940
EXPECTED_PRIOR_OBSERVED_COUNT = 476


class ReadinessBoundaryError(Exception):
    """Raised when the static recovery-evidence boundary is not enforced."""


# ---------------------------------------------------------------------------
# Path / JSON helpers
# ---------------------------------------------------------------------------


def repo_path(path_text: str) -> Path:
    return recovery_validator.repo_path(path_text)


def repo_relative(path: Path) -> str:
    return recovery_validator.repo_relative(path)


def as_bool(value: Any, label: str) -> bool:
    if not isinstance(value, bool):
        raise ReadinessBoundaryError(f"{label} must be boolean, got {value!r}")
    return value


def as_int(value: Any, label: str) -> int:
    try:
        return recovery_validator.integer(value, label)
    except recovery_validator.ValidationError as exc:
        raise ReadinessBoundaryError(str(exc)) from exc


def require_equal(label: str, actual: Any, expected: Any) -> None:
    if actual != expected:
        raise ReadinessBoundaryError(f"{label} must be {expected!r}, got {actual!r}")


def require_false(label: str, actual: Any) -> None:
    if as_bool(actual, label) is not False:
        raise ReadinessBoundaryError(f"{label} must be false under the current reviewed schema")


def require_true(label: str, actual: Any) -> None:
    if as_bool(actual, label) is not True:
        raise ReadinessBoundaryError(f"{label} must be true under the current reviewed schema")


# ---------------------------------------------------------------------------
# Source-policy validation
# ---------------------------------------------------------------------------


def placeholder_expected_source() -> collector.ExpectedSourceIdentity:
    """Build a stable identity only to ask the collector for its source policy."""

    return collector.ExpectedSourceIdentity(
        run_id=0,
        branch="single-run-placeholder",
        head_sha="0" * 40,
        workflow=".github/workflows/benchmark.yml",
        milestone="",
        collection_run_id="",
        issue=0,
    )


def current_collector_source_policy() -> Mapping[str, Any]:
    return collector.source_policy_report(
        expected=placeholder_expected_source(),
        raw_source_record_count=EXPECTED_SOURCE_PLAN_COUNT,
        logical_source_record_count=EXPECTED_SOURCE_PLAN_COUNT,
        raw_source_file_count=EXPECTED_SOURCE_PLAN_COUNT,
    )


def validate_source_policy_document(policy: Mapping[str, Any], label: str) -> dict[str, Any]:
    require_equal(f"{label}.schema_name", policy.get("schema_name"), EXPECTED_SOURCE_POLICY_SCHEMA)
    require_equal(f"{label}.policy", policy.get("policy"), EXPECTED_SOURCE_POLICY_NAME)
    require_true(f"{label}.single_run_identity_required", policy.get("single_run_identity_required"))
    require_false(f"{label}.reviewed_multi_source_schema", policy.get("reviewed_multi_source_schema"))
    require_false(f"{label}.multi_source_union_allowed", policy.get("multi_source_union_allowed"))
    raw_identity_count = as_int(
        policy.get("raw_source_identity_count"),
        f"{label}.raw_source_identity_count",
    )
    require_equal(f"{label}.raw_source_identity_count", raw_identity_count, 1)
    require_equal(
        f"{label}.logical_source_record_count",
        as_int(policy.get("logical_source_record_count"), f"{label}.logical_source_record_count"),
        EXPECTED_SOURCE_PLAN_COUNT,
    )
    expected_identity = policy.get("expected_source_identity")
    if not isinstance(expected_identity, Mapping):
        raise ReadinessBoundaryError(f"{label}.expected_source_identity must be an object")
    return {
        "schema_name": policy.get("schema_name"),
        "policy": policy.get("policy"),
        "single_run_identity_required": True,
        "reviewed_multi_source_schema": False,
        "multi_source_union_allowed": False,
        "raw_source_identity_count": 1,
        "logical_source_record_count": EXPECTED_SOURCE_PLAN_COUNT,
    }


def synthetic_finishable_provenance(
    *,
    policy_updates: Optional[Mapping[str, Any]] = None,
    run_conclusion: str = "success",
) -> dict[str, Any]:
    policy = dict(current_collector_source_policy())
    policy["expected_source_identity"] = {
        "run_id": 0,
        "branch": "single-run-placeholder",
        "head_sha": "0" * 40,
        "workflow": ".github/workflows/benchmark.yml",
        "milestone": "",
        "collection_run_id": "",
        "issue": 0,
    }
    if policy_updates:
        policy.update(policy_updates)
    return {
        "branch": "single-run-placeholder",
        "milestone": "",
        "collection_run_id": "",
        "issue": 0,
        "run": {
            "database_id": 0,
            "head_branch": "single-run-placeholder",
            "head_sha": "0" * 40,
            "conclusion": run_conclusion,
        },
        "dispatch": {
            "ref": "single-run-placeholder",
            "workflow": ".github/workflows/benchmark.yml",
        },
        "source_policy": policy,
    }


def assert_finishable_policy_rejects(
    *,
    case: str,
    expected_message: str,
    policy_updates: Optional[Mapping[str, Any]] = None,
    run_conclusion: str = "success",
) -> dict[str, Any]:
    provenance = synthetic_finishable_provenance(
        policy_updates=policy_updates,
        run_conclusion=run_conclusion,
    )
    try:
        finishable.validate_single_run_source_policy(
            provenance,
            EXPECTED_SOURCE_PLAN_COUNT,
            f"readiness_boundary.{case}",
        )
    except finishable.ManifestError as exc:
        message = str(exc)
        if expected_message not in message:
            raise ReadinessBoundaryError(
                f"finishable guard probe {case!r} rejected for the wrong reason: {message}"
            ) from exc
        return {"case": case, "rejected": True, "message_contains": expected_message}
    raise ReadinessBoundaryError(f"finishable guard probe {case!r} did not reject")


def validate_finishable_gate_contract() -> list[dict[str, Any]]:
    valid = synthetic_finishable_provenance()
    try:
        finishable.validate_single_run_source_policy(
            valid,
            EXPECTED_SOURCE_PLAN_COUNT,
            "readiness_boundary.valid_single_run_probe",
        )
    except finishable.ManifestError as exc:
        raise ReadinessBoundaryError(f"finishable valid single-run guard probe failed: {exc}") from exc

    return [
        assert_finishable_policy_rejects(
            case="reviewed_multi_source_schema_true",
            policy_updates={"reviewed_multi_source_schema": True},
            expected_message="reviewed_multi_source_schema must be false",
        ),
        assert_finishable_policy_rejects(
            case="multi_source_union_allowed_true",
            policy_updates={"multi_source_union_allowed": True},
            expected_message="multi_source_union_allowed must be false",
        ),
        assert_finishable_policy_rejects(
            case="cancelled_top_level_run",
            run_conclusion="cancelled",
            expected_message="run.conclusion must be 'success'",
        ),
    ]


def validate_current_source_policy() -> dict[str, Any]:
    policy_summary = validate_source_policy_document(
        current_collector_source_policy(),
        "collector.source_policy",
    )
    require_equal("finishable.SOURCE_POLICY_SCHEMA", finishable.SOURCE_POLICY_SCHEMA, EXPECTED_SOURCE_POLICY_SCHEMA)
    require_equal("finishable.SOURCE_POLICY_NAME", finishable.SOURCE_POLICY_NAME, EXPECTED_SOURCE_POLICY_NAME)
    return {
        "terminal_collector": policy_summary,
        "finishable_generator": {
            "schema_name": finishable.SOURCE_POLICY_SCHEMA,
            "policy": finishable.SOURCE_POLICY_NAME,
            "requires_single_run_source_policy": True,
            "requires_successful_top_level_run": True,
            "requires_exact_1416_terminal_accounting": True,
            "guard_probes": validate_finishable_gate_contract(),
        },
    }


# ---------------------------------------------------------------------------
# Boundary report construction
# ---------------------------------------------------------------------------


def validate_static_recovery_scope(static_report: Mapping[str, Any]) -> dict[str, Any]:
    require_equal("source_run_id", static_report.get("source_run_id"), EXPECTED_SOURCE_RUN_ID)
    require_equal("source_run_conclusion", static_report.get("source_run_conclusion"), EXPECTED_SOURCE_RUN_CONCLUSION)

    coverage = static_report.get("coverage")
    if not isinstance(coverage, Mapping):
        raise ReadinessBoundaryError("recovery dry-run validation report is missing coverage object")

    source_count = as_int(coverage.get("source_plan_index_count"), "coverage.source_plan_index_count")
    recovery_count = as_int(coverage.get("dry_run_plan_index_count"), "coverage.dry_run_plan_index_count")
    prior_count = as_int(coverage.get("prior_observed_plan_index_count"), "coverage.prior_observed_plan_index_count")
    union_count = as_int(
        coverage.get("represented_plan_index_count_after_union"),
        "coverage.represented_plan_index_count_after_union",
    )
    require_equal("coverage.source_plan_index_count", source_count, EXPECTED_SOURCE_PLAN_COUNT)
    require_equal("coverage.dry_run_plan_index_count", recovery_count, EXPECTED_RECOVERY_ONLY_COUNT)
    require_equal("coverage.prior_observed_plan_index_count", prior_count, EXPECTED_PRIOR_OBSERVED_COUNT)
    require_equal("coverage.represented_plan_index_count_after_union", union_count, EXPECTED_SOURCE_PLAN_COUNT)
    require_equal(
        "coverage.prior_observed_treatment",
        coverage.get("prior_observed_treatment"),
        "incomplete_prior_evidence_only",
    )
    require_true("coverage.not_complete_terminal_provenance", coverage.get("not_complete_terminal_provenance"))
    require_true(
        "coverage.prior_observed_excluded_from_recovery",
        coverage.get("prior_observed_excluded_from_recovery"),
    )

    return {
        "source_plan_index_count": source_count,
        "future_recovery_only_record_count": recovery_count,
        "prior_cancelled_run_observed_record_count": prior_count,
        "represented_plan_index_count_after_accounting_union": union_count,
        "accounting_union_covers_all_plan_indices": True,
        "accounting_union_is_completion_evidence": False,
        "recovery_only_artifacts_are_complete_terminal_collection": False,
        "prior_cancelled_run_observations_are_complete_terminal_provenance": False,
        "prior_observed_treatment": "incomplete_prior_evidence_only",
        "source_run_conclusion": EXPECTED_SOURCE_RUN_CONCLUSION,
    }


def boundary_decision() -> dict[str, Any]:
    return {
        "status": BLOCKED_DECISION,
        "can_combine_476_prior_with_future_940_recovery_artifacts": False,
        "can_collect_terminal_provenance_from_mixed_union": False,
        "can_regenerate_finishable_manifest": False,
        "can_regenerate_tier_artifacts": False,
        "can_regenerate_terminal_provenance_for_completion": False,
        "can_claim_issue_101_complete": False,
        "reason": (
            "The reviewed terminal-provenance schema is single-run only.  The 476 records observed in "
            "cancelled run 25111815292 and a future 940-entry recovery-only artifact set would have "
            "different run/branch/SHA/workflow/provenance identities, so their accounting union cannot be "
            "promoted to finishability-evidence completion or to finishable/Tier/provenance regeneration."
        ),
    }


def scenario_matrix() -> list[dict[str, Any]]:
    return [
        {
            "scenario": "future_940_recovery_only_artifact_set",
            "record_count": EXPECTED_RECOVERY_ONLY_COUNT,
            "decision": "reject_for_issue_101_completion",
            "reason": (
                "940 recovery records are missing the 476 prior plan entries and are not "
                "a full 1416-entry collection."
            ),
        },
        {
            "scenario": "prior_476_cancelled_run_observations_only",
            "record_count": EXPECTED_PRIOR_OBSERVED_COUNT,
            "decision": "reject_for_issue_101_completion",
            "reason": "the prior observations came from a cancelled run and remain incomplete_prior_evidence_only.",
        },
        {
            "scenario": "future_940_recovery_plus_prior_476_cancelled_run_union",
            "record_count_after_accounting_union": EXPECTED_SOURCE_PLAN_COUNT,
            "decision": "reject_mixed_source_union_under_current_schema",
            "reason": (
                "the current source policy allows exactly one run identity and no reviewed multi-source union; "
                "1416-by-accounting is not 1416-entry terminal provenance."
            ),
        },
        {
            "scenario": "fresh_full_1416_entry_single_run_terminal_collection",
            "record_count": EXPECTED_SOURCE_PLAN_COUNT,
            "decision": "eligible_for_future_validation_only",
            "reason": (
                "a fresh single-run collection would still need exact terminal outcome coverage, successful top-level "
                "run provenance, and finishable/Tier regeneration review before the finishability evidence could be completed."
            ),
        },
    ]


def unblockers() -> list[dict[str, Any]]:
    return [
        {
            "path": "future_reviewed_multi_source_provenance_schema",
            "currently_available": False,
            "requirements": [
                "reviewed schema defining per-source run/branch/SHA/workflow/source lineage",
                "explicit validation of 476 cancelled-run records and 940 recovery records without over-attribution",
                "finishable/Tier regeneration gates updated to consume that reviewed multi-source schema",
            ],
        },
        {
            "path": "fresh_full_1416_entry_terminal_collection",
            "currently_available": False,
            "requirements": [
                "one validated terminal outcome record for every source plan index 1-1416",
                "single requested/top-level run identity in every outcome record",
                "successful top-level run conclusion and validated terminal provenance before regeneration",
            ],
        },
    ]


def build_boundary_report(plan_path: Path, manifest_path: Path) -> dict[str, Any]:
    try:
        static_report = recovery_validator.validate_recovery_dry_run_plan(plan_path, manifest_path)
    except recovery_validator.ValidationError as exc:
        raise ReadinessBoundaryError(f"recovery dry-run validation failed: {exc}") from exc

    source_policy = validate_current_source_policy()
    recovery_scope = validate_static_recovery_scope(static_report)
    non_goals = list(static_report.get("non_goals", []))
    return {
        "schema_name": BOUNDARY_SCHEMA,
        "schema_version": SCHEMA_VERSION,
        "validation_status": "pass",
        "validated": True,
        "read_only": True,
        "pre_dispatch_static_validation": True,
        "inputs": {
            "dry_run_plan": repo_relative(plan_path),
            "dry_run_plan_sha256": static_report["plan_sha256"],
            "recovery_manifest": repo_relative(manifest_path),
            "recovery_manifest_sha256": static_report["recovery_manifest_sha256"],
            "source_run_id": static_report["source_run_id"],
            "source_run_conclusion": static_report["source_run_conclusion"],
        },
        "source_policy": source_policy,
        "recovery_scope": recovery_scope,
        "decision": boundary_decision(),
        "scenarios": scenario_matrix(),
        "unblockers": unblockers(),
        "enforced_by": [
            "scripts/collect_mi300a_terminal_discovery_provenance.py single_run_terminal_source_identity",
            "scripts/validate_mi300a_terminal_discovery_provenance.py exact 1416-entry terminal accounting",
            "scripts/generate_mi300a_finishable_size_manifest.py source-policy/success/complete-accounting gate",
            (
                "scripts/validate_mi300a_terminal_recovery_dry_run_plan.py "
                "recovery-only scope and prior-evidence exclusion"
            ),
        ],
        "non_goals": non_goals,
    }


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--dry-run-plan",
        default=str(recovery_validator.dry_run.DEFAULT_OUTPUT),
        help="Path to the checked-in recovery dry-run plan JSON",
    )
    parser.add_argument(
        "--recovery-manifest",
        default=str(recovery_validator.dry_run.DEFAULT_RECOVERY_MANIFEST),
        help="Path to the checked-in recovery manifest JSON",
    )
    return parser.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    args = parse_args(argv)
    try:
        report = build_boundary_report(
            repo_path(args.dry_run_plan),
            repo_path(args.recovery_manifest),
        )
    except ReadinessBoundaryError as exc:
        print("FAIL: MI300A recovery-evidence readiness boundary is not enforced.", file=sys.stderr)
        print(f"  - {exc}", file=sys.stderr)
        return 1

    print(json.dumps(report, indent=2) + "\n", end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
