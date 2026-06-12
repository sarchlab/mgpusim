# MI300A Terminal Discovery Readiness Guide

There is currently no visible `.github/workflows/mi300a_terminal_discovery.yml` workflow_dispatch surface to run. The separate terminal-discovery Actions workflow has been retired so that `.github/workflows/benchmark.yml` is the only maintained MI300A benchmark Actions entry point: `Validation -> Tier 1 -> Tier-1 summary -> Tier 2 -> Summary`, with only the optional `ref` dispatch input.

The next 3600-second terminal-discovery attempt has recorded human approval (#181), but it still requires a reviewed operator path and complete terminal evidence over the 1416-entry plan. finishability evidence remains unresolved until that complete terminal evidence is collected and validated or the human explicitly accepts a narrower scope. Do not dispatch, cancel, rerun, or reintroduce a terminal-discovery workflow from this document.

## Retired dispatch surface

- Retired visible workflow file: `.github/workflows/mi300a_terminal_discovery.yml`.
- Current maintained MI300A Actions entry point: `.github/workflows/benchmark.yml` only.
- Current benchmark workflow shape: `Validation -> Tier 1 -> Tier-1 summary -> Tier 2 -> Summary`.
- Current benchmark dispatch input: optional `ref` only; no `run_mode` and no terminal-discovery selector.
- Approval status: #181 approved the next terminal-discovery run, but only a reviewed temporary/operator path should use that approval; this guide does not authorize dispatch, rerun, cancellation, or reintroduction of a permanent terminal-discovery workflow.

## Offline readiness artifacts

The shard surface remains checked in for offline validation and future approved operator planning:

- `benchmark-comparison/generated/mi300a_terminal_discovery_shard_manifest.json`
- `benchmark-comparison/generated/mi300a_terminal_discovery_shard_01_matrix.json` through `benchmark-comparison/generated/mi300a_terminal_discovery_shard_06_matrix.json`

These files preserve the 1416-entry terminal-evidence contract while avoiding problem-size-level matrix rows: six contiguous workflow-benchmark shards, 81 visible benchmark `include` rows total, nested per-size attempts for every source plan entry, and `timeout_sec: 3600` on every attempt. The historical `hotspot`/`48` attempt must remain `plan_index: 474` in shard 2, hotspot benchmark row attempt 2.

The recovery manifest also tracks the read-only recovery scope for the incomplete replacement
run, and the recovery dry-run matrix is tracked derived from that manifest:

- `benchmark-comparison/generated/mi300a_terminal_discovery_run_25111815292_recovery_manifest.json`
- `benchmark-comparison/generated/mi300a_terminal_discovery_run_25111815292_recovery_dry_run_plan.json`

That recovery manifest is derived from
`benchmark-comparison/mi300a_problem_size_discovery_plan.json` and the missing
plan-index ranges in `docs/m24_incomplete_terminal_discovery_coverage.md`. It
contains only the 940 unobserved plan indices from run `25111815292`, grouped by
workflow benchmark with source-plan lookup fields for the runner metadata. The
The dry-run plan expands those 940 missing indices into one single-attempt matrix
row each, batches them under the per-row-timeout 14-way / 600-second row / 3600-second
action-layer contract, and leaves the 476 observed records from that run as
excluded incomplete prior evidence only. Those 476 records are recorded as
coverage context, not as complete terminal provenance, not as synthetic outcomes,
and not as evidence that finishability evidence or per-row-timeout evidence is complete.

## Offline checks

Run these commands from the repository root on the target ref. They are static
metadata checks over tracked files only.

Validate the checked-in readiness surface:

```bash
python3 scripts/validate_mi300a_terminal_discovery_shards.py
```

For any candidate operator row/action budget, also run the static timeout-contract guard with explicit values for the planned row timeout, effective per-attempt timeout, and non-simulator overhead:

```bash
python3 scripts/validate_mi300a_terminal_discovery_timeout_contract.py \
  --row-timeout-minutes <planned-row-timeout-minutes> \
  --per-attempt-timeout-sec <planned-attempt-timeout-sec> \
  --row-overhead-sec <planned-checkout-build-upload-overhead-sec>
```

The guard reads the checked-in shard manifest/matrix JSON and emits a JSON
summary to stdout. It uses `required_timeout_sec = attempt_count *
per_attempt_timeout_sec + row_overhead_sec` for each visible benchmark row and
exits non-zero if any row cannot fit. It is a dry-run metadata check only: it
does not run simulators, contact GitHub, dispatch Actions, collect evidence, or
regenerate finishable/Tier artifacts. The replacement run shape
(`--row-timeout-minutes 10 --per-attempt-timeout-sec 600` plus any positive
overhead) intentionally fails this guard because rows contain up to 54
sequential attempts; a future operator plan must choose values that satisfy this
formula before any human-approved recovery run is launched.

Check that the tracked run `25111815292` recovery manifest is exactly
reproducible without rewriting it:

```bash
python3 scripts/generate_mi300a_terminal_recovery_manifest.py --check
```

The recovery check derives the 940 missing plan indices from the checked-in
1416-entry plan and `docs/m24_incomplete_terminal_discovery_coverage.md`, then
compares the result byte-for-byte with
`benchmark-comparison/generated/mi300a_terminal_discovery_run_25111815292_recovery_manifest.json`.
The 476 observed records from the cancelled run remain incomplete prior
evidence only; the manifest is recovery scope, not terminal provenance and not a
synthetic outcome set. A future 940-entry recovery artifact set may not be
silently combined with those 476 cancelled-run observations under one top-level
run identity: the current terminal provenance path is single-source only, and it
must reject mixed run/branch/SHA/workflow/run-identity/issue/PR identities until
a reviewed multi-source provenance schema is explicitly added.

Check that the recovery-only dry-run matrix is reproducible from that recovery
manifest and still satisfies the pre-dispatch contract:

```bash
python3 scripts/generate_mi300a_terminal_recovery_dry_run_plan.py --check
python3 scripts/validate_mi300a_terminal_recovery_dry_run_plan.py
```

The dry-run validator covers exactly the 940 missing plan indices once, verifies
zero overlap with the 476 prior observed incomplete-evidence-only indices,
confirms the complete prior logical rows (`gelu`, `sssp`, `dmr`) remain excluded,
and checks the per-row-timeout budget contract: 14 maximum parallel rows, one 600-second
attempt per row, a 600-second row cap, a 3600-second action-layer cap, 12 static
batches, and no row/batch timeout violations. The guarded recovery operator
checklist (`docs/terminal_recovery_operator_preflight.md`) is the branch-only
preflight/monitoring/artifact-collection reference for any later human-approved
recovery attempt; it does not authorize dispatch or weaken the 1416-entry
terminal-evidence contract.

Validate the finishability recovery-evidence readiness boundary:

```bash
python3 scripts/validate_mi300a_recovery_evidence_readiness_boundary.py
```

A passing readiness-boundary report means the repository still enforces
`decision.status: "blocked_under_current_reviewed_schema"`: the 476 cancelled-run
observations may not be combined with a future 940-entry recovery-only artifact
set for finishability-evidence completion or for provenance/finishable/Tier regeneration until a
reviewed multi-source schema exists or a fresh full 1416-entry single-run
terminal collection validates.

Validate the finishability evidence matrix-contract boundary:

```bash
python3 scripts/validate_mi300a_matrix_contract_boundary.py
```

A passing matrix-contract report means the repository distinguishes three static
cases before any operational action: the checked-in 940-entry recovery-only path
has the 14/600/3600 shape but is still non-completing under the readiness boundary, active run
`25196153223` is a separate 6-way / 4320-minute-row / 3600-second
nested-attempt contract and must not be labeled per-row-timeout-compliant, and a literal
full 1416-entry per-benchmark matrix with a 600-second row cap is rejected before
dispatch with 80 row violations and 32,400-second zero-overhead max-row evidence.

Only regenerate the shard surface when intentionally refreshing the deterministic offline manifests from `benchmark-comparison/mi300a_problem_size_discovery_plan.json`:

```bash
python3 scripts/generate_mi300a_terminal_discovery_shards.py
python3 scripts/validate_mi300a_terminal_discovery_shards.py
```

Only run the recovery manifest generator without `--check` when intentionally
refreshing its static missing-index scope from the checked-in plan and incomplete-coverage report:

```bash
python3 scripts/generate_mi300a_terminal_recovery_manifest.py
python3 scripts/generate_mi300a_terminal_recovery_manifest.py --check
```

Those commands are offline/static metadata checks. They do not launch simulations,
contact GitHub, dispatch Actions, collect new evidence, regenerate the finishable
manifest, regenerate Tier views, or resolve finishability evidence. They also do not start
calibration or change the visible Actions surface: no terminal-discovery workflow
is restored, no `run_mode` selector is added to `.github/workflows/benchmark.yml`,
and the maintained MI300A Actions entry point remains the existing linear
finishable-size workflow with only the optional `ref` dispatch input.

## Future approved-run artifact contract

If a future human-approved operator path runs terminal discovery, it must still produce one terminal outcome record for every one of the 1416 plan entries even though the Actions matrix rows are grouped per benchmark. The preferred per-benchmark artifact shape is one artifact per visible benchmark row named `mi300a-terminal-discovery-benchmark-<workflow_benchmark>` containing `terminal_discovery_artifacts/outcomes_mi300a_terminal_discovery_benchmark_<workflow_benchmark>.json`. That JSON should use schema `mgpusim.mi300a_terminal_discovery_benchmark_outcomes` and carry an `outcomes` list whose entries are the same per-size `mgpusim.mi300a_terminal_discovery_outcome` records formerly uploaded one per plan entry. The collector also remains backward-compatible with legacy per-plan artifacts named `mi300a-terminal-discovery-plan-<plan_index>` containing:

- `outcome_mi300a_terminal_discovery_plan_<plan_index>.json`
- `results_mi300a_terminal_discovery_plan_<plan_index>.csv`
- `stdout_tail_mi300a_terminal_discovery_plan_<plan_index>.log`
- `build_mi300a_terminal_discovery_plan_<plan_index>.log`

Timeout, crash, missing SQLite, missing kernel-time, and build-failed statuses are terminal discovery outcomes when their outcome JSON record is present; they are not workflow-gating failures by themselves.

## Collection and validation after a future approved run

For a future approved run, download/unpack all per-benchmark artifacts (or legacy per-plan artifacts) under `terminal-discovery-artifacts/` and run:

```bash
python3 scripts/collect_mi300a_terminal_discovery_provenance.py \
  --outcomes-root terminal-discovery-artifacts \
  --output mi300a_terminal_discovery_provenance.json \
  --run-id <github-run-id> \
  --run-url <github-run-url> \
  --head-branch <head-branch-or-ref> \
  --head-sha <head-sha> \
  --actor <github-actor>

python3 scripts/validate_mi300a_terminal_discovery_provenance.py \
  --provenance mi300a_terminal_discovery_provenance.json
```

Terminal provenance is acceptable only when it accounts for every one of the 1416 plan entries exactly once: one terminal outcome row, one accounting record, one job entry, and one logical artifact entry per plan entry, whether the source files were 81 benchmark-level JSON artifacts, 1416 legacy per-plan JSON artifacts, or a mixed download containing both shapes. The source files must also satisfy the single-run source policy: every outcome JSON record must claim the requested/top-level run id, branch/ref, head SHA, workflow path, run-identity, issue, and PR. A 940-only recovery scope remains incomplete, a 476-only prior cancelled-run scope remains incomplete, and a 940+476 union from different source identities must fail instead of being over-attributed to either run. When both supported shapes are present for the same `plan_index` from the same source identity, the collector and the branch-only `m18_terminal_discovery_operator.py count-outcomes` gate validate every source record against the plan, reject duplicates that disagree on terminal/provenance-critical fields, and deterministically prefer the benchmark-level nested record over the standalone per-plan record in their logical 1416-record view. Accepted primary terminal buckets are `completed_success`, `failed`, `timed_out`, `skipped`, and `cancelled`; `non_terminal`, queued/in-progress, missing-job, missing-outcome, conflicting duplicate, source-identity conflict, completed non-success/cancelled run conclusion, or extra plan entries are collection/validation failures.

Run 25026018067 salvage note: this repair is intended to salvage the already-downloaded summary/provenance path without a new workflow dispatch when that artifact tree is otherwise complete and its duplicate nested/standalone records are conflict-free. The mixed duplicate artifact shape alone should no longer block collection or `count-outcomes`; missing plan indices, extra plan indices, non-terminal outcomes, timeout/metadata mismatches, or conflicting duplicate terminal/provenance-critical fields remain hard failures.

Finishable-manifest and Tier regeneration remain blocked unless either a future reviewed multi-source schema is added and validated, or a fresh full 1416-entry single-run collection validates under the source policy above. If such terminal provenance is accepted and the project intentionally wants to replace the current bounded snapshot, regenerate the finishable manifest and Tier views from that terminal provenance:

```bash
python3 scripts/generate_mi300a_finishable_size_manifest.py \
  --provenance mi300a_terminal_discovery_provenance.json
python3 scripts/validate_mi300a_finishable_size_manifest.py
```

Do not overwrite the checked-in bounded snapshot outputs unless that replacement is intentional, approved, and reviewed.

## What this is not

- This is not a dispatch guide for a currently visible terminal-discovery workflow.
- This is not a new mode in `.github/workflows/benchmark.yml`.
- This is not approval to add, rerun, cancel, or dispatch terminal discovery.
- This is not simulator execution or a simulation campaign.
- This is not calibration, a parameter sweep, or benchmark-data collection.
- This is not provenance, finishable-manifest, or Tier-artifact regeneration.
- This is not a reason to restore a permanent terminal-discovery Actions surface.
- This does not weaken the 1416-entry terminal-evidence contract.
- This does not turn the 476 prior observed incomplete-evidence-only entries into
  terminal outcomes.
- This does not resolve the finishability or per-row-timeout contract.
