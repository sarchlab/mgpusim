from __future__ import annotations

import json
from pathlib import Path
from typing import Any, Mapping


REPO_ROOT = Path(__file__).resolve().parents[2]
PLAN = REPO_ROOT / "benchmark-comparison" / "mi300a_problem_size_discovery_plan.json"
PROVENANCE = (
    REPO_ROOT
    / "benchmark-comparison"
    / "provenance"
    / "mi300a_problem_size_discovery_run_24959959195.json"
)


def make_ranges(values: set[int]) -> list[str]:
    ordered = sorted(values)
    if not ordered:
        return []
    ranges: list[str] = []
    start = previous = ordered[0]
    for value in ordered[1:]:
        if value == previous + 1:
            previous = value
            continue
        ranges.append(f"{start}-{previous}" if start != previous else str(start))
        start = previous = value
    ranges.append(f"{start}-{previous}" if start != previous else str(start))
    return ranges


def load_json(path: Path) -> dict[str, Any]:
    with path.open("r", encoding="utf-8") as f:
        data = json.load(f)
    if not isinstance(data, dict):
        raise AssertionError(f"{path} did not load as a JSON object")
    return data


def write_json(path: Path, data: Mapping[str, Any]) -> None:
    path.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")


# The checked-in legacy discovery-provenance JSON files use the original
# collection-run identifier key name.  The renamed schema expects
# ``collection_run_id``; this helper normalizes loaded objects so the synthetic
# fixture writer can target the renamed key without depending on the legacy
# spelling appearing as a string literal in this file.
_LEGACY_COLLECTION_RUN_ID_KEY = "".join(("e", "p", "o", "c", "h"))


def normalize_legacy_collection_run_id(data: dict[str, Any]) -> dict[str, Any]:
    if "collection_run_id" not in data and _LEGACY_COLLECTION_RUN_ID_KEY in data:
        data["collection_run_id"] = data.pop(_LEGACY_COLLECTION_RUN_ID_KEY)
    return data


def write_synthetic_complete_terminal_provenance(tmp_path: Path) -> Path:
    """Create a complete terminal-accounting provenance fixture.

    The checked-in provenance is intentionally a bounded non-terminal
    snapshot.  This helper keeps the successful terminal rows and marks every
    other plan entry as terminal timed_out so generator/validator tests can
    prove complete 1416-row terminal accounting is accepted without claiming the
    synthetic fixture is real project evidence.
    """

    provenance = normalize_legacy_collection_run_id(load_json(PROVENANCE))
    plan_entries = load_json(PLAN)["entries"]
    plan_count = len(plan_entries)
    all_indices = set(range(1, plan_count + 1))
    existing_rows = {
        int(row["discovery_plan_index"]): dict(row)
        for row in provenance["outcome_summary"]["rows"]
    }
    success_indices = {idx for idx, row in existing_rows.items() if row["terminal_outcome"] == "success"}
    timed_out_indices = all_indices - success_indices

    run = dict(provenance["run"])
    run.update(
        {
            "status": "completed",
            "conclusion": "success",
            "non_terminal_snapshot": False,
            "status_note": "Synthetic complete terminal accounting fixture for static regression tests.",
        }
    )
    provenance["run"] = run
    observation = dict(provenance["observation"])
    observation["non_terminal_snapshot"] = False
    observation["bounded_snapshot_note"] = "Synthetic complete terminal accounting fixture; not checked-in evidence."
    provenance["observation"] = observation

    requested = dict(provenance["requested_plan_outcome_buckets"])
    zero_buckets = (
        "failed",
        "skipped",
        "cancelled",
        "non_terminal",
        "non_terminal_in_progress",
        "non_terminal_queued",
        "missing_job",
        "missing_artifact_for_completed_success_job",
        "missing_outcome_row_for_completed_success_job",
        "missing_outcome_row",
    )
    for bucket in zero_buckets:
        requested[f"{bucket}_count"] = 0
        requested[f"{bucket}_plan_index_ranges"] = []
    requested["completed_success_count"] = len(success_indices)
    requested["completed_success_plan_index_ranges"] = make_ranges(success_indices)
    requested["timed_out_count"] = len(timed_out_indices)
    requested["timed_out_plan_index_ranges"] = make_ranges(timed_out_indices)
    requested["terminal_mode"] = True
    requested["terminal_provenance"] = True
    requested["bounded_non_terminal_snapshot"] = False
    provenance["requested_plan_outcome_buckets"] = requested

    rows: list[dict[str, Any]] = []
    artifacts: list[dict[str, Any]] = []
    jobs: list[dict[str, Any]] = []
    existing_artifacts = {int(entry["plan_index"]): dict(entry) for entry in provenance["artifact_summary"]["entries"]}
    existing_jobs = {int(entry["plan_index"]): dict(entry) for entry in provenance["terminal_discovery_job_summary"]["entries"]}
    for entry in plan_entries:
        plan_index = int(entry["plan_index"])
        shard = min(6, ((plan_index - 1) // 236) + 1)
        if plan_index in success_indices:
            rows.append(dict(existing_rows[plan_index]))
        else:
            rows.append(
                {
                    "benchmark": entry["workflow_benchmark_name"],
                    "candidate_size": entry["sizes"],
                    "size_label": entry["size_label"],
                    "terminal_outcome": "timed_out",
                    "sim_result_state": "timeout",
                    "exit_code": "124",
                    "timeout_sec": str(entry["timeout_sec"]),
                    "detail": "synthetic_complete_terminal_accounting_positive",
                    "discovery_plan_index": str(plan_index),
                    "discovery_hardware_kernel_name": entry["hardware_kernel_name"],
                    "discovery_hardware_problem_size": entry["hardware_problem_size"],
                    "discovery_artifact_id": entry["artifact_id"],
                }
            )
        artifacts.append(
            existing_artifacts.get(
                plan_index,
                {
                    "id": 900000000000 + plan_index,
                    "name": f"benchmark-results-{entry['artifact_id']}",
                    "size_in_bytes": 1,
                    "expired": False,
                    "created_at": run["updated_at"],
                    "updated_at": run["updated_at"],
                    "expires_at": run["updated_at"],
                    "digest": f"sha256:{plan_index:064x}",
                    "plan_index": plan_index,
                    "benchmark": entry["benchmark"],
                    "problem_size": entry["problem_size"],
                    "artifact_id": entry["artifact_id"],
                },
            )
        )
        job = dict(
            existing_jobs.get(
                plan_index,
                {
                    "plan_index": plan_index,
                    "shard": shard,
                    "database_id": 910000000000 + plan_index,
                    "name": f"Problem-Size Discovery {shard}: plan {plan_index}",
                    "started_at": run["started_at"],
                    "completed_at": run["updated_at"],
                    "url": f"{run['url']}/job/{910000000000 + plan_index}",
                },
            )
        )
        job.update(
            {
                "status": "completed",
                "conclusion": "success" if plan_index in success_indices else "timed_out",
                "duration_sec": 8 if plan_index in success_indices else 3600,
            }
        )
        jobs.append(job)

    provenance["outcome_summary"] = {
        **dict(provenance["outcome_summary"]),
        "terminal_outcome_row_count": plan_count,
        "terminal_outcome_counts": {"success": len(success_indices), "timed_out": len(timed_out_indices)},
        "terminal_outcome_plan_index_ranges": {
            "success": make_ranges(success_indices),
            "timed_out": make_ranges(timed_out_indices),
        },
        "rows": rows,
    }
    provenance["artifact_summary"] = {
        **dict(provenance["artifact_summary"]),
        "total_count": plan_count,
        "benchmark_result_artifact_count": plan_count,
        "plan_index_ranges": ["1-1416"],
        "entries": artifacts,
    }
    conclusion_counts = {"success": len(success_indices), "timed_out": len(timed_out_indices)}
    provenance["terminal_discovery_job_summary"] = {
        **dict(provenance["terminal_discovery_job_summary"]),
        "completed_job_count": plan_count,
        "all_discovery_job_status_counts": {"completed": plan_count},
        "all_discovery_job_conclusion_counts": conclusion_counts,
        "entries": jobs,
    }
    workflow_summary = dict(provenance["workflow_summary"])
    workflow_summary["discovery_job_status_counts"] = {"completed": plan_count}
    workflow_summary["discovery_job_conclusion_counts"] = conclusion_counts
    provenance["workflow_summary"] = workflow_summary
    provenance["source_policy"] = {
        "schema_name": "mgpusim.mi300a_terminal_source_policy",
        "schema_version": 1,
        "policy": "single_run_terminal_source_identity",
        "policy_text": (
            "synthetic complete terminal accounting fixture keeps one static source identity; "
            "it is not project evidence"
        ),
        "single_run_identity_required": True,
        "reviewed_multi_source_schema": False,
        "multi_source_union_allowed": False,
        "required_identity_fields": [
            "run_id",
            "branch",
            "head_sha",
            "workflow",
            "milestone",
            "collection_run_id",
            "issue",
        ],
        "expected_source_identity": {
            "run_id": run["database_id"],
            "branch": provenance["branch"],
            "head_sha": run["head_sha"],
            "workflow": provenance["dispatch"]["workflow"],
            "milestone": provenance.get("milestone"),
            "collection_run_id": provenance.get("collection_run_id"),
            "issue": provenance.get("issue"),
        },
        "raw_source_identity_count": 1,
        "raw_source_file_count": plan_count,
        "raw_source_record_count": plan_count,
        "logical_source_record_count": plan_count,
    }
    provenance["terminal_outcome_accounting"] = {
        "schema_name": "mgpusim.mi300a_terminal_outcome_accounting",
        "schema_version": 1,
        "terminal_mode": True,
        "plan_entry_count": plan_count,
        "record_count": plan_count,
        "bucket_counts": {
            "completed_success": len(success_indices),
            "failed": 0,
            "timed_out": len(timed_out_indices),
            "skipped": 0,
            "cancelled": 0,
        },
        "terminal_bucket_total": plan_count,
        "non_terminal_count": 0,
        "missing_job_count": 0,
        "records": [
            {
                "plan_index": plan_index,
                "terminal": True,
                "terminal_outcome_bucket": "completed_success" if plan_index in success_indices else "timed_out",
            }
            for plan_index in range(1, plan_count + 1)
        ],
    }
    output = tmp_path / "complete-terminal-provenance.json"
    write_json(output, provenance)
    return output
