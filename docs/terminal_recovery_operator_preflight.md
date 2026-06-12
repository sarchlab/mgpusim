# Terminal recovery operator preflight

The terminal recovery operator maintains the read-only operator preflight/dry-run CLI for the checked-in MI300A
terminal-discovery recovery dry-run plan for cancelled run `25111815292`.

- CLI: `scripts/mi300a_terminal_recovery_operator.py`
- Dry-run plan:
  `benchmark-comparison/generated/mi300a_terminal_discovery_run_25111815292_recovery_dry_run_plan.json`
- Recovery manifest:
  `benchmark-comparison/generated/mi300a_terminal_discovery_run_25111815292_recovery_manifest.json`
- Expected dry-run plan SHA-256:
  `88760e9b71bd83e82637352670788632efe207c4c10338d4d78f5ae6b1bccec4`
- Expected recovery manifest SHA-256:
  `d09d26cae96127b10a564bd3376ccbcba8b2433e9e5cfb6ee6c35fb7f7004913`

The command is intentionally branch-only and repository-local. It does not contact
GitHub, dispatch/cancel/rerun Actions, run simulations, start calibration,
regenerate provenance/finishable/Tier artifacts, or modify `.github/workflows`.

## Commands

Run the full preflight over all 12 checked-in recovery batches:

```bash
python3 scripts/mi300a_terminal_recovery_operator.py preflight
```

`preflight` is also the default command:

```bash
python3 scripts/mi300a_terminal_recovery_operator.py
```

Emit the same read-only report in dry-run mode for an explicit batch selection:

```bash
python3 scripts/mi300a_terminal_recovery_operator.py dry-run --batches 1,12
```

Batch selectors accept comma-separated indices and ranges. The operator sorts
valid explicit selections into recovery-batch order and rejects duplicate or
out-of-scope selections before emitting a report.

## Bounded operator path

This document is an operator-path checklist, not a dispatch authorization.  The
only checked-in recovery scope is the recovery dry-run plan for the 940 plan indices
that were missing from cancelled run `25111815292`.  The path is bounded to the
12 checked-in recovery batches in that plan and to the branch
`e31-m28-1-repair-recovery-operator-provenance-guard`; it must not be
generalized to other plan indices, other runs, regenerated matrices, or a terminal finishability completion claim
without a separately reviewed decision and complete terminal evidence.

### 1. Preflight

Before any later human-approved recovery action, run these repository-local
checks on the exact operator branch/SHA that would be used:

```bash
git fetch origin e31-m28-1-repair-recovery-operator-provenance-guard
git checkout e31-m28-1-repair-recovery-operator-provenance-guard
git pull --ff-only origin e31-m28-1-repair-recovery-operator-provenance-guard

python3 scripts/generate_mi300a_terminal_recovery_manifest.py --check
python3 scripts/generate_mi300a_terminal_recovery_dry_run_plan.py --check
python3 scripts/validate_mi300a_terminal_recovery_dry_run_plan.py
python3 scripts/mi300a_terminal_recovery_operator.py preflight --all-batches \
  --expected-head-sha "$(git rev-parse --verify HEAD)"
```

For a subset, replace `--all-batches` with an explicit `--batches` selection such
as `--batches 1-3,12`.  The operator report must be saved with the operator log
for that later run.  A passing report must show `status: "pass"`,
`validated: true`, `read_only: true`, `dispatch_performed: false`,
`simulator_execution_performed: false`, `artifact_regeneration_performed: false`,
and `workflow_modification_performed: false`.

The operator also verifies git provenance before reporting `status: "pass"`: it
reads the actual current branch and `HEAD` SHA from the checkout, rejects any
branch other than `e31-m28-1-repair-recovery-operator-provenance-guard`,
optionally rejects a mismatched `--expected-head-sha`, and records the actual
branch/SHA in the JSON report.

### 2. Safe dispatch and monitoring envelope

No dispatch is approved by this documentation.  If a later human-approved
cycle authorizes a branch-only recovery attempt, keep it inside this envelope:

1. Dispatch only from a reviewed temporary/operator branch whose preflight report
   was produced from the same branch/SHA.  Do not add a permanent
   terminal-discovery workflow, do not add a `run_mode` selector to
   `.github/workflows/benchmark.yml`, and do not change the maintained permanent
   MI300A Actions surface.
2. Target only recovery batch indices `1` through `12` from the checked-in dry-run
   plan.  Do not dispatch plan indices outside the 940 missing-index scope, do
   not split/rebatch rows differently, and do not include the 476 prior observed
   entries as recovery work.
3. Preserve the checked-in contract exactly: `max_parallel=14`, one attempt per
   matrix row, `per_attempt_timeout_sec=600`, `row_timeout_sec=600`, and
   `action_timeout_sec=3600`.  Do not use lower or higher timeout/parallelism
   overrides that no longer match the checked-in batch schedule.
4. Monitor only with read-only run/job/artifact inspection commands (for example,
   `gh run view`, `gh run list`, and `gh api .../actions/runs/.../artifacts`).
   Do not issue `gh run cancel`, `gh run rerun`, or another dispatch to repair an
   in-flight run from this path.  If the run diverges from the approved branch,
   SHA, batch selection, or contract, stop the operator path and file a new tracking issue
   instead of completing evidence.
5. Treat row failures, timeouts, skipped rows, and cancellations as evidence only
   when the expected terminal outcome JSON exists and validates against the source
   plan.  Missing outcome JSON, non-terminal queued/in-progress state, duplicate
   conflicting outcome records, or extra plan indices are collection failures.

### 3. Artifact collection boundary

After a later approved recovery run finishes, artifact handling is still bounded
and evidence-only:

1. Download the run artifacts into an untracked collection directory named by run
   id and batch selection, for example
   `terminal-discovery-artifacts/run_<run_id>_batches_<selection>/`.
2. Inventory artifact names, source JSON counts, outcome-record counts, run/job
   conclusions, and the selected batch plan-index ranges before running any
   collector.  Keep the inventory with the operator log.
3. Collect terminal provenance only when the artifact roots together contain a
   complete 1416-entry terminal collection that validates as an exact 1416-record
   provenance document.  The 940-entry recovery-only artifact set is not
   sufficient by itself, and under the current reviewed single-run source schema
   it may not be combined with the 476 records observed in cancelled run
   `25111815292`. Those 476 records remain `incomplete_prior_evidence_only`
   until either a future reviewed multi-source provenance schema is added or a
   fresh full 1416-entry single-run terminal collection validates.
4. Do not regenerate `mi300a_terminal_discovery_provenance.json`, finishable-size
   manifests, Tier views, validation reports, or calibration artifacts from a
   partial recovery download, and do not claim finishability evidence complete from recovery
   batches alone.  The allowed terminal buckets for a complete collection remain
   `completed_success`, `failed`, `timed_out`, `skipped`, and `cancelled`;
   `non_terminal`, missing, extra, or conflicting rows fail collection.

When, and only when, a complete 1416-entry artifact root exists, provenance
collection must record the exact fields below and then be validated:

```bash
python3 scripts/collect_mi300a_terminal_discovery_provenance.py \
  --outcomes-root terminal-discovery-artifacts/run_<run_id>_complete_1416 \
  --output mi300a_terminal_discovery_provenance.json \
  --run-id <github-run-database-id> \
  --run-url <github-run-url> \
  --head-branch <approved-operator-branch> \
  --head-sha <approved-operator-head-sha> \
  --created-at <run-created-at-utc> \
  --started-at <run-started-at-utc> \
  --updated-at <run-updated-at-utc> \
  --collected-at <collection-observation-utc> \
  --workflow-name <workflow-name> \
  --actor <github-actor> \
  --issue <issue_id> \
  --pr-id <pr_id> \
  --dispatch-workflow <reviewed-branch-only-workflow-path>

python3 scripts/validate_mi300a_terminal_discovery_provenance.py \
  --provenance mi300a_terminal_discovery_provenance.json
```

## Guarded contract

A passing report proves the operator input remains bounded by the
static recovery contract:

| Field | Required value |
|---|---:|
| Recovery batches in checked-in plan | 12 |
| Missing recovery plan indices | 940 |
| Prior observed entries | 476, treated as `incomplete_prior_evidence_only` |
| Max parallel recovery rows | 14 |
| Attempts per matrix row | 1 |
| Per-attempt timeout cap | 600 seconds |
| Row cap | 600 seconds |
| Action-layer cap per batch | 3600 seconds |

The CLI refuses:

- stale or regenerated plan/manifest SHA-256 mismatches;
- canonical dry-run plan mismatches detected by
  `validate_mi300a_terminal_recovery_dry_run_plan.py`;
- `--max-parallel`, `--per-attempt-timeout-sec`, `--row-timeout-sec`, or
  `--action-timeout-sec` values above the checked-in caps;
- lower contract overrides that no longer match the checked-in batch schedule;
- duplicate batch selections, including overlapping ranges; and
- out-of-scope batch selections outside `1-12`.

## Fields that must be recorded

Keep the JSON field names below exact in the saved operator/preflight report and
in any later complete terminal provenance file.

### Preflight report fields

| JSON path | Required value / source |
|---|---|
| `schema_name` | `mgpusim.mi300a_terminal_discovery_recovery_operator_preflight` |
| `schema_version` | `1` |
| `run_id` / `issue_id` / `pr_id` | `<run_id>` / `<issue_id>` / `<pr_id>` |
| `operator_branch` / `current_branch` | actual current git branch, required to equal `e31-m28-1-repair-recovery-operator-provenance-guard` |
| `operator_head_sha` / `current_head_sha` | actual current git `HEAD` SHA read during preflight |
| `expected_operator_head_sha` | optional `--expected-head-sha` value; if supplied it must match actual `HEAD` |
| `mode` | `preflight` or `dry-run` |
| `status`, `validated`, `read_only` | `pass`, `true`, `true` |
| `dispatch_performed` | `false` |
| `simulator_execution_performed` | `false` |
| `artifact_regeneration_performed` | `false` |
| `workflow_modification_performed` | `false` |
| `plan.artifact` | `benchmark-comparison/generated/mi300a_terminal_discovery_run_25111815292_recovery_dry_run_plan.json` |
| `plan.sha256` and `plan.expected_sha256` | `88760e9b71bd83e82637352670788632efe207c4c10338d4d78f5ae6b1bccec4` |
| `recovery_manifest.artifact` | `benchmark-comparison/generated/mi300a_terminal_discovery_run_25111815292_recovery_manifest.json` |
| `recovery_manifest.sha256` and `recovery_manifest.expected_sha256` | `d09d26cae96127b10a564bd3376ccbcba8b2433e9e5cfb6ee6c35fb7f7004913` |
| `source_run.run_id` | `25111815292` |
| `source_run.run_conclusion` | `cancelled` |
| `source_run.head_branch` | `e31-m23-execute-278-terminal-discovery-relaunch` |
| `source_run.head_sha` | `8ff0b0e1674c6778c3c697ff516abb9b2e4694c1` |
| `coverage.source_plan_index_count` | `1416` |
| `coverage.missing_plan_index_count` | `940` |
| `coverage.prior_observed_plan_index_count` | `476` |
| `coverage.prior_observed_treatment` | `incomplete_prior_evidence_only` |
| `coverage.recovery_batch_count` | `12` |
| `operator_contract.max_parallel` / `per_attempt_timeout_sec` / `row_timeout_sec` / `action_timeout_sec` | `14` / `600` / `600` / `3600` |
| `batch_selection.selected_batch_indices` | subset of checked-in batch indices `1`-`12` only |
| `selected_batches[].plan_index_ranges` | only ranges from the checked-in 940-entry missing scope |
| `guardrails.forbidden_side_effects` | includes workflow dispatch/cancel/rerun, simulator execution, calibration, provenance regeneration, finishable/Tier regeneration, and workflow-file modification |
| `non_goals` | includes no simulator execution, workflow dispatch/cancel/rerun, calibration, provenance/finishable/Tier regeneration, terminal finishability completion claim, or permanent Actions surface change |

The preflight JSON records the active tracker boundary directly: issue <issue_id>
and PR <pr_id>. Do not substitute older global/local tracker numbers in
saved operator logs or future provenance.

### Complete provenance fields

When a later approved collection really contains all 1416 terminal records, the
validated provenance JSON must preserve these run/branch/SHA/provenance fields:

| JSON path | Required value / source |
|---|---|
| `schema_name` | `mgpusim.mi300a_problem_size_discovery_provenance` |
| `purpose` | `terminal_mi300a_problem_size_discovery_provenance` |
| `issue` / `pr` | `<issue_id>` / `<pr_id>` |
| `branch` | the approved branch/ref passed as `--head-branch` |
| `run.database_id` | GitHub Actions run database id, not run number |
| `run.url` | GitHub Actions run URL |
| `run.workflow_name` / `run.name` / `run.display_title` | workflow name observed for the approved run |
| `run.event` | `workflow_dispatch` |
| `run.head_branch` | approved branch/ref actually dispatched |
| `run.head_sha` | exact head SHA actually dispatched |
| `run.status` / `run.conclusion` | `completed` / `success` in the accepted complete-provenance document; preserve per-row outcomes and job conclusions in `workflow_summary`/`terminal_jobs` |
| `run.created_at`, `run.started_at`, `run.updated_at` | run timestamps from Actions |
| `run.observed_at_utc` | collection observation timestamp |
| `dispatch.workflow` | reviewed branch-only workflow path used for the approved run |
| `dispatch.ref` | same approved branch/ref as `run.head_branch` |
| `dispatch.actor` | GitHub actor that dispatched the approved run |
| `plan_summary.artifact` / `plan_summary.entry_count` | `benchmark-comparison/mi300a_problem_size_discovery_plan.json` / `1416` |
| `artifact_summary.source_file_count` and `source_outcome_record_count` | exact downloaded source JSON and record counts |
| `artifact_summary.logical_terminal_outcome_artifact_count` | `1416` logical artifacts after source de-duplication |
| `outcome_summary.terminal_outcome_row_count` | `1416` |
| `terminal_outcome_accounting.accounting_record_count` | `1416` |

Do not treat a provenance JSON as usable until
`scripts/validate_mi300a_terminal_discovery_provenance.py` accepts it with exact
1416-entry coverage and zero missing, extra, non-terminal, or conflicting rows.
Only after that validation may a separate reviewed decision determine whether to
regenerate finishable-size or Tier artifacts.


## Verification

Focused tests for the operator CLI:

```bash
python3 -m pytest scripts/tests/test_mi300a_terminal_recovery_operator.py
```

Broader static recovery checks:

```bash
python3 scripts/generate_mi300a_terminal_recovery_manifest.py --check
python3 scripts/generate_mi300a_terminal_recovery_dry_run_plan.py --check
python3 scripts/validate_mi300a_terminal_recovery_dry_run_plan.py
python3 -m pytest \
  scripts/tests/test_generate_mi300a_terminal_recovery_manifest.py \
  scripts/tests/test_generate_mi300a_terminal_recovery_dry_run_plan.py \
  scripts/tests/test_validate_mi300a_terminal_recovery_dry_run_plan.py \
  scripts/tests/test_mi300a_terminal_recovery_operator.py
```
