#!/usr/bin/env python3
"""Generate and validate MI300A run provenance from committed CSV files.

Usage (from repo root):
    python3 scripts/generate_mi300a_run_provenance.py --run <RUN_ID>
    python3 scripts/generate_mi300a_run_provenance.py --run <RUN_ID> --artifacts <ARTIFACTS_DIR>

The script reads committed CSV provenance files from benchmark-comparison/provenance/
and runs reconciliation assertions, printing the same OK: lines that appear in
the terminal observation markdown under '## Reconciliation check'.

The --artifacts argument is optional. When supplied, the script can additionally
verify counts against downloaded artifact CSVs. Without it, all assertions are
run against the committed CSV files only.

Exit codes:
    0  all assertions passed
    1  one or more assertions failed
"""

import argparse
import csv
import pathlib
import sys


PROVENANCE_DIR = pathlib.Path("benchmark-comparison/provenance")
TERMINAL_MD_SUFFIX = "_terminal_observation_20260503T140449Z.md"
DEFAULT_INVOCATION_TIMEOUT_SEC = 7200
DEFAULT_BENCHMARK_JOB_TIMEOUT_SEC = 21600

# Expected run-specific constants (keyed by run ID).
RUN_CONSTANTS = {
    "25266517426": {
        "timestamp": "20260503T140449Z",
        "total_plan_rows": 1416,
        "finished": 552,
        "timed_out_3600s": 80,
        "invocation_timeout_sec": 3600,
        "benchmark_job_timeout_sec": 43200,
        "crashed": 3,
        "unknown": 781,
        "log_finished": 559,
        "log_timed_out": 76,
        "log_crashed": 3,
        "bs_plan_total": 1416,
        "bs_csv_total": 559,
        "bs_unpaired": 7,
        "terminal_md_suffix": "_terminal_observation_20260503T140449Z.md",
    },
}

OUTCOME_FINISHED = "known_finished_from_downloaded_benchmark_results_csv_and_run_log_ok"
OUTCOME_CRASHED = "known_crashed_from_run_log"


def invocation_timeout_sec(consts: dict) -> int:
    return int(consts.get("invocation_timeout_sec", DEFAULT_INVOCATION_TIMEOUT_SEC))


def benchmark_job_timeout_sec(consts: dict) -> int:
    return int(consts.get("benchmark_job_timeout_sec", DEFAULT_BENCHMARK_JOB_TIMEOUT_SEC))


def timeout_count_key(consts: dict) -> str:
    return f"timed_out_{invocation_timeout_sec(consts)}s"


def timeout_event_type(consts: dict) -> str:
    return str(consts.get("timeout_event_type", timeout_count_key(consts)))


def timeout_outcome_label(consts: dict) -> str:
    return str(
        consts.get(
            "timeout_outcome",
            f"known_timed_out_{invocation_timeout_sec(consts)}s_from_run_log",
        )
    )


def find_csv(run_id: str, kind: str) -> pathlib.Path:
    """Return the committed CSV path for the given run and data kind."""
    pattern = f"mi300a_regular_benchmark_run_{run_id}_{kind}_*.csv"
    matches = sorted(PROVENANCE_DIR.glob(pattern))
    # Prefer the terminal timestamp file if multiple exist.
    if not matches:
        raise FileNotFoundError(
            f"No committed CSV found for run {run_id} kind {kind} "
            f"under {PROVENANCE_DIR}"
        )
    return matches[-1]


def load_csv(path: pathlib.Path) -> list[dict]:
    with open(path, newline="") as fh:
        return list(csv.DictReader(fh))


def assert_ok(condition: bool, message: str) -> None:
    """Print OK: message if condition is True, else raise SystemExit(1)."""
    if condition:
        print(f"OK: {message}")
    else:
        print(f"FAIL: {message}", file=sys.stderr)
        sys.exit(1)


def check_problem_size_rows(run_id: str, consts: dict) -> None:
    """Assertion 1 & 2: unique plan rows and outcome counts."""
    path = find_csv(run_id, "problem_size_rows")
    rows = load_csv(path)

    total = len(rows)
    assert_ok(
        total == consts["total_plan_rows"],
        f"problem_size_rows has {total} unique plan rows",
    )

    timeout_outcome = timeout_outcome_label(consts)
    timeout_key = timeout_count_key(consts)
    finished = sum(1 for r in rows if r["row_observation"] == OUTCOME_FINISHED)
    timed_out = sum(1 for r in rows if r["row_observation"] == timeout_outcome)
    crashed = sum(1 for r in rows if r["row_observation"] == OUTCOME_CRASHED)
    unknown = total - finished - timed_out - crashed

    assert_ok(
        finished == consts["finished"]
        and timed_out == consts[timeout_key]
        and crashed == consts["crashed"]
        and unknown == consts["unknown"],
        (
            f"problem row outcomes "
            f"finished={finished} "
            f"{timeout_key}={timed_out} "
            f"crashed={crashed} "
            f"unknown={unknown} "
            f"(total={total})"
        ),
    )


def check_log_event_summary(run_id: str, consts: dict) -> None:
    """Assertion 3: log event counts."""
    path = find_csv(run_id, "log_event_summary")
    rows = load_csv(path)

    timeout_event = timeout_event_type(consts)
    finished = sum(1 for r in rows if r["event_type"] == "finished")
    timed_out = sum(1 for r in rows if r["event_type"] == timeout_event)
    crashed = sum(1 for r in rows if r["event_type"] == "crashed")

    assert_ok(
        finished == consts["log_finished"]
        and timed_out == consts["log_timed_out"]
        and crashed == consts["log_crashed"],
        (
            f"log_event_summary events "
            f"finished={finished} "
            f"{timeout_event}={timed_out} "
            f"crashed={crashed}"
        ),
    )


def check_benchmark_summary_totals(run_id: str, consts: dict) -> None:
    """Assertion 4: benchmark_summary totals."""
    path = find_csv(run_id, "benchmark_summary")
    rows = load_csv(path)

    plan_total = sum(int(r["expected_plan_entries"]) for r in rows)
    csv_total = sum(int(r["csv_data_rows"]) for r in rows)
    unpaired = sum(int(r["csv_rows_unpaired_to_unique_plan_entries"]) for r in rows)

    assert_ok(
        plan_total == consts["bs_plan_total"]
        and csv_total == consts["bs_csv_total"]
        and unpaired == consts["bs_unpaired"],
        (
            f"benchmark_summary "
            f"expected_plan_entries total={plan_total}, "
            f"csv_data_rows total={csv_total}, "
            f"unpaired duplicate csv/log rows={unpaired}"
        ),
    )


def check_benchmark_summary_projection(run_id: str, consts: dict) -> None:
    """Assertion 5: per-benchmark plan-entry count matches problem_size_rows."""
    bs_path = find_csv(run_id, "benchmark_summary")
    ps_path = find_csv(run_id, "problem_size_rows")

    bs_rows = load_csv(bs_path)
    ps_rows = load_csv(ps_path)

    ps_counts: dict[str, int] = {}
    for r in ps_rows:
        b = r["workflow_benchmark"]
        ps_counts[b] = ps_counts.get(b, 0) + 1

    plan_total = sum(int(r["expected_plan_entries"]) for r in bs_rows)
    all_match = all(
        int(r["expected_plan_entries"]) == ps_counts.get(r["workflow_benchmark"], 0)
        for r in bs_rows
    )

    assert_ok(
        plan_total == consts["bs_plan_total"] and all_match,
        (
            f"benchmark_summary plan-entry projection total={plan_total} "
            f"and every per-benchmark plan-entry count matches problem_size_rows"
        ),
    )


def check_budget_and_timeouts(run_id: str, consts: dict) -> None:
    """Assertion 6: no job exceeded the budget; all timeouts match the contract."""
    timeout_sec = invocation_timeout_sec(consts)
    budget_sec = benchmark_job_timeout_sec(consts)
    budget_min = budget_sec // 60
    exceeded_column = f"exceeded_{budget_sec}s_benchmark_tier_budget"
    bs_path = find_csv(run_id, "benchmark_summary")
    bs_rows = load_csv(bs_path)

    exceeded = [
        r for r in bs_rows
        if r.get(exceeded_column, "").lower() == "true"
    ]

    log_path = find_csv(run_id, "log_event_summary")
    log_rows = load_csv(log_path)
    timeout_event = timeout_event_type(consts)
    non_matching_timeout = [
        r for r in log_rows
        if r["event_type"] == timeout_event
        and r.get("timeout_sec", str(timeout_sec)) not in (str(timeout_sec), "")
    ]

    assert_ok(
        len(exceeded) == 0 and len(non_matching_timeout) == 0,
        (
            f"no benchmark-tier job exceeded {budget_sec}s/{budget_min}m budget; "
            f"all parsed timeout events are {timeout_sec}s invocation timeouts"
        ),
    )


def check_terminal_md_labels(run_id: str, consts: dict) -> None:
    """Assertion 7: terminal markdown labels the run as terminal failure evidence."""
    pattern = f"mi300a_regular_benchmark_run_{run_id}_terminal_observation_*.md"
    matches = sorted(PROVENANCE_DIR.glob(pattern))
    if not matches:
        print(
            f"FAIL: no terminal observation markdown found for run {run_id} "
            f"under {PROVENANCE_DIR}",
            file=sys.stderr,
        )
        sys.exit(1)

    md_path = matches[-1]
    text = md_path.read_text()

    is_failure_evidence = (
        "conclusion `failure`" in text
        and "terminal failure evidence" in text
        and "not benchmark success or complete finishability" in text.lower()
        or (
            "not a benchmark-success or complete-finishability claim" in text
            or "not benchmark success" in text
        )
    )

    assert_ok(
        is_failure_evidence,
        (
            "terminal markdown labels this completed/failure run as "
            "current-spec terminal failure evidence, not benchmark success "
            "or complete finishability"
        ),
    )


def run_reconciliation(run_id: str, artifacts_dir: str | None) -> None:
    """Run all reconciliation assertions for the given run ID."""
    if run_id not in RUN_CONSTANTS:
        print(
            f"ERROR: run {run_id} not recognised. "
            f"Known runs: {list(RUN_CONSTANTS)}",
            file=sys.stderr,
        )
        sys.exit(1)

    consts = RUN_CONSTANTS[run_id]

    check_problem_size_rows(run_id, consts)
    check_log_event_summary(run_id, consts)
    check_benchmark_summary_totals(run_id, consts)
    check_benchmark_summary_projection(run_id, consts)
    check_budget_and_timeouts(run_id, consts)
    check_terminal_md_labels(run_id, consts)

    if artifacts_dir:
        print(
            f"NOTE: --artifacts {artifacts_dir} supplied; "
            "artifact-level verification is not yet implemented in this script."
        )


def main() -> None:
    parser = argparse.ArgumentParser(
        description=__doc__,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument(
        "--run",
        required=True,
        metavar="RUN_ID",
        help="GitHub Actions run database ID (e.g. 25266517426)",
    )
    parser.add_argument(
        "--artifacts",
        metavar="ARTIFACTS_DIR",
        default=None,
        help=(
            "Optional path to downloaded artifact directory "
            "(e.g. <tmp>/run25266517426-evidence/artifacts). "
            "When omitted all assertions use committed CSV files only."
        ),
    )
    args = parser.parse_args()

    repo_root = pathlib.Path(__file__).parent.parent
    import os
    os.chdir(repo_root)

    run_reconciliation(args.run, args.artifacts)


if __name__ == "__main__":
    main()
