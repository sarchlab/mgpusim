# Benchmark Comparison Framework

## Overview

This directory contains data and scripts for comparing MGPUSim simulator timing against MI300A hardware measurements.

## How Benchmarks Are Parameterized

### Scaling vs Non-Scaling Parameters

Each benchmark has parameters that control problem size. These fall into two categories:

1. **Scaling parameter**: The primary parameter that varies across measurements (e.g., matrix dimension N, array length, grid size). This is the parameter swept in both hardware and simulator runs.

2. **Non-scaling parameters**: Secondary parameters held constant at their default values (e.g., number of iterations, sparsity, mask size, number of bins).

### Important: Only Scaling Parameters Vary

Both the hardware measurement script (`workloads/scripts/run_all.sh`) and the simulator workflow (`.github/workflows/benchmark.yml`) follow the same protocol:

- **Only the scaling parameter is varied** across problem sizes.
- **Non-scaling parameters are fixed** at their default values.
- There is **no mixing of different non-scaling parameter values** within a single benchmark's dataset.

For example, for `hotspot`:
- Scaling parameter: `grid_size` (varied: 32, 48, 64, ..., 4096)
- Non-scaling parameter: `num_iterations` (fixed at 10)

For compound benchmarks like `kmeans`:
- Both `points` and `features` vary together as part of the scaling sweep.
- But `clusters` (5) and `max-iter` (1) are fixed non-scaling parameters.

### Non-Monotonic Hardware Data

Some hardware measurements show non-monotonic behavior (e.g., hotspot: 512→640 drops from 0.36ms to 0.19ms). This is **genuine hardware behavior**, not an artifact of parameter mixing. Possible causes include:

- Cache effects and memory hierarchy transitions
- GPU frequency scaling / thermal throttling
- Hardware-level scheduling differences
- Measurement variance at small timescales

The simulator may not reproduce these non-monotonic effects since it uses a deterministic timing model.

## Key Files

- `selected-run-25619929396/comparison_ci.detailed.csv` — Current selected point-by-point report comparison (sim vs HW) from run `25619929396`; `comparison_ci.csv` is retained as a legacy compatibility copy. The selected-run directory itself preserves raw historical artifact identity, so embedded old timeout-contract wording in its summaries is archive context, not current workflow guidance.
- `selected-run-25710089668/` — Terminal current-main MI300A completed/failure partial evidence archive for workflow-dispatch run `25710089668` on `main@6fdbbd1882f6a36c5e0846b9209a9a19c258b486`. It records the fetched artifact inventory, raw-zip hashes, validation-summary contract pass artifacts, and artifacts-only finishability evidence (`pass=623`, `no-result=793`, `fail=0`, `timeout=0`, `cancelled=0`) regenerated from the checked-in selected-run metadata, matrix, plan, and merged `sim_results_ci.csv` timing rows. This archive does not replace the selected accuracy report input above and does not claim a clean run or final completion.
- `selected-run-25841274346/` — Newer current-main MI300A non-terminal/provisional provenance snapshot for workflow-dispatch run `25841274346` on `main@e0ef96c57e042c636e2b94db8f294ff8a574bb78`, collected while the run remained `in_progress` with empty conclusion. It records provisional artifacts and job state only; it is not terminal evidence, not clean MI300A Benchmark completion, and not finishability evidence unless refreshed terminal evidence exists from a completed run that is recollected and validated.
- `mi300a_problem_size_discovery_plan.json` — Checked-in full measured MI300A problem-size plan. The maintained regular workflow uses all 1416 entries as its regular runtime matrix, grouped into per-benchmark rows, while the same file remains the archival contract for discovery provenance and finishable-manifest validation.
- `generated/mi300a_terminal_discovery_shard_manifest.json` and `generated/mi300a_terminal_discovery_shard_01_matrix.json` … `generated/mi300a_terminal_discovery_shard_06_matrix.json` — Deterministic terminal-discovery readiness manifests retained for offline validation. They shard all 1416 per-size attempts into six benchmark-grouped matrix manifests: every visible `include` row is one workflow benchmark with nested per-size attempts, and every attempt preserves `timeout_sec: 3600`; they are not a visible Actions dispatch surface.
- `../docs/mi300a_terminal_discovery_operator.md` — Retired/readiness guide for the terminal-discovery tooling, the #181 approval gate, future artifact expectations, and provenance validation.
- `mi300a_finishable_size_manifest.json` — bounded finishable-size manifest derived from discovery outcome evidence.
- `../scripts/validate_mi300a_regular_workflow_matrix.py` — Static/read-only guard for the maintained full regular workflow matrix. It verifies `.github/workflows/benchmark.yml` still materializes all 1416 discovery-plan entries into clean per-benchmark rows, keeps `max-parallel: 14`, per-simulator `timeout_sec: 7200`, and `timeout-minutes: 360` / 21600-second benchmark-tier job limits, and has no terminal-discovery or extra dispatch surface.
- `../scripts/materialize_mi300a_regular_workflow_matrix.py` — Runtime materializer consumed by `.github/workflows/benchmark.yml`. It reads the checked-in 1416-entry discovery plan and emits clean per-benchmark Tier 1/Tier 2 matrix rows for the maintained regular workflow, normalizing every simulator invocation to `timeout_sec: 7200`.
- `../scripts/derive_mi300a_regular_finishability_evidence.py` — Offline selected-run evidence CLI. Given downloaded artifacts or a curated selected-run root for one terminal regular run, it validates terminal run metadata, rejects non-terminal snapshot sources such as runs `25841274346` and `24959959195`, and emits deterministic JSON/Markdown per-size pass/no-result evidence joined to the regular matrix and job outcomes.
- `provenance/` — Historical run-observation archive. Its raw launch notes, observation Markdown, and CSV summaries may retain old 3600-second, 720-minute, or other captured-run timeout wording; those strings document past runs only and do not override the maintained 7200-second / 360-minute / max-parallel 14 regular workflow contract.
- `generated/mi300a_regular_tier1_matrix.json` — Retired/offline generated
  Tier 1 audit artifact. Its `runtime_contract` intentionally preserves the
  historical 600-second per-simulator and 3600-second benchmark-job values with
  `max_parallel: 14` for auditability only; it is not the maintained regular
  MI300A workflow contract or a pruning source. The maintained contract comes
  from `.github/workflows/benchmark.yml` plus
  `scripts/materialize_mi300a_regular_workflow_matrix.py`: 7200 seconds per
  simulator, `timeout-minutes: 360` / 21600 seconds per benchmark-tier job, and
  max-parallel 14.
- `generated/mi300a_finishable_tier1_matrix.json` and `generated/mi300a_finishable_tier2_matrix.json` — Checked-in eligible matrix views derived from the bounded finishable-size manifest. They remain checked in for finishable-manifest/evaluation validation and auditability, but the maintained regular workflow no longer uses them to prune runtime coverage.
- `mi300a_finishable_evaluation_set.json` and `mi300a_finishable_evaluation_set_summary.json` — Current bounded finishable MI300A evaluation surface derived from the finishable-size manifest and Tier views. It includes all 23 eligible completed-success problem sizes and carries the bounded-snapshot disclaimer for the 20 timed-out and 1373 non-terminal source rows.
- `benchmark_tiers.md` — Benchmark tier classification (Tier 1/2/3 by sim time)
- `selected-run-25619929396/sim_results_ci.csv` — Current selected merged simulator results from run `25619929396`; `sim_results_ci.csv` is retained as a legacy compatibility copy.
- `regular_artifact_coverage_summary.json` / `regular_artifact_coverage_summary.md` — Deterministic Summary-job artifacts that label regular artifact coverage by comparing expected regular rows with observed simulation/comparison/regression CSV rows. They are regular-workflow coverage evidence only, not finishability/provenance completion.

## MI300A linear regular workflow

`.github/workflows/benchmark.yml` now has one default MI300A path:

```text
Validation -> Tier 1 -> Tier-1 summary -> Tier 2 -> Summary
```

Default dispatch is `workflow_dispatch` with only the optional `ref` input. There is no `run_mode` selector:
dispatching the workflow runs the linear regular path,
using the requested ref when supplied or the dispatch ref otherwise.

The validation job runs first and must pass before any simulator job starts. It
validates `benchmark-comparison/mi300a_finishable_size_manifest.json` for audit
continuity, runs `python3 scripts/validate_mi300a_regular_workflow_matrix.py` as
a static workflow/plan contract guard, then runs
`python3 scripts/materialize_mi300a_regular_workflow_matrix.py` to materialize
the maintained regular matrices directly from
`benchmark-comparison/mi300a_problem_size_discovery_plan.json`, the checked-in
full measured MI300A plan. The maintained
regular workflow covers all 1416 plan entries exactly once and uses that checked-in
full measured plan as the regular runtime matrix in clean per-benchmark
matrix rows: 19 Tier 1 rows / 308 plan entries and 63 Tier 2 rows / 1108 plan
entries. The
bounded finishable manifest and generated finishable Tier views remain checked in
for auditability, but they are not used to prune the maintained regular runtime
matrix and are not regenerated by this path.

Before emitting the dynamic Tier matrices, validation normalizes every row to
`timeout_sec: 7200`, so every individual simulator invocation is capped at 7200
seconds. Matrix rows may contain more than three source plan entries; the Actions
job layer remains bounded by `timeout-minutes: 360` / 21600 seconds and any partial
artifacts are diagnostic regular-workflow data, not finishability evidence.

The maintained regular MI300A benchmark-tier contract is static and visible in
`.github/workflows/benchmark.yml`: both `benchmark-tier1` and `benchmark-tier2`
use `strategy.max-parallel: 14`, `timeout-minutes: 360` for the job layer (21600
seconds), and the shared runner step executes each simulator invocation through
`timeout "${TIMEOUT_SEC}"` after the metadata guard has required
`timeout_sec == 7200`. This keeps the regular workflow contract at 14-way
benchmark parallelism, 7200 seconds per individual simulator invocation, and
21600 seconds per benchmark-tier job without adding a terminal-discovery flow or
dispatch mode.

### Interpreting row-budget/passability-risk fields

The regular matrix materializer and validator expose runtime-budget fields on
each emitted per-benchmark row and in the validation summaries.
`planned_invocation_count` / `expected_simulator_invocations` is the number of
planned simulator invocations in that row. The static worst-case row budget is
that count multiplied by `per_invocation_timeout_sec`: currently
`invocation_count * 7200s`, stored as `row_timeout_budget_sec`, then compared
with the `benchmark_job_timeout_sec` / 21600-second Actions job cap and reported
as `row_timeout_budget_to_job_timeout_ratio` plus
`exceeds_benchmark_job_timeout`.

Interpret this as static worst-case runtime-budget/passability-risk visibility:
compare `invocation_count * 7200s` against the 21600s job cap. Rows with at most
three planned invocations fit the cap if every invocation consumes its full
7200-second timeout and no other overhead is charged; rows above three show a
budget-over-cap risk that the benchmark-tier job can hit the 360-minute cap
before finishing all sizes. This accounting is not a simulator result or
evidence that any invocation actually ran for 7200 seconds.
It is not a hard pass/fail blocker by itself. It does not prune the maintained
regular matrix or prove whether a row will pass in a live Actions run.

The evidence boundary is intentionally narrow. Boundaries in shorthand:

- no workflow dispatch/cancel/rerun;
- no terminal-discovery mode restoration;
- no long local simulator sweep or calibration;
- no finishable/provenance/Tier regeneration.

Reading these fields, materializing the matrix, or running the static validator
also does not collect terminal-discovery evidence or claim terminal finishability completion.

Current maintained regular workflow matrix coverage is:

| Workflow job | Matrix source | Matrix rows | Expected data rows | Source plan-index ranges |
|---|---|---:|---:|---|
| benchmark-tier1 | `mi300a_problem_size_discovery_plan.json` | 19 | 308 | `1-308` |
| benchmark-tier2 | `mi300a_problem_size_discovery_plan.json` | 63 | 1108 | `309-1416` |

The maintained
workflow now restores the full 1416-entry regular plan matrix. This regular
matrix is still separate from the complete finishability-evidence
requirement: non-terminal or excluded source rows stay regular workflow data only
and do not authorize terminal-provenance collection or regeneration, finishable
manifest regeneration, Tier regeneration, terminal finishability completion, local
simulation/calibration, or any workflow cancel/rerun action.

The Tier-1 summary is intentionally between the two simulation tiers. It
summarizes downloaded Tier 1 outputs and checks that Tier 1 artifacts contain the
expected regular Tier 1 data-row count. It is informational rather than a
finishability gate: Tier 2 still runs after validation for full regular matrix
coverage even if Tier 1 produced only partial/diagnostic artifacts. The final
Summary job runs with `if: always()`, merges available benchmark artifacts, runs
comparison/report generation when possible, records the
expected complete row count, and warns when uploaded artifacts are
partial/diagnostic only because an upstream stage failed. It uploads
`benchmark-comparison`, `comparison-detailed`, and `validation-report` artifacts
for diagnosis without treating partial artifacts as successful completion
evidence. A successful Summary job after failed or partial benchmark tiers is
therefore report/upload completion only, not clean MI300A Benchmark completion.

### Regular artifact coverage summary

The final Summary job also emits a deterministic repo-only regular artifact
coverage summary as `regular_artifact_coverage_summary.json` and
`regular_artifact_coverage_summary.md`, then appends the Markdown version to the
Actions step summary. The summary does not dispatch workflows, run simulators,
download extra artifacts, or fabricate missing outcomes. It only compares
expected regular row counts with observed rows in already-present CSV artifacts.

The JSON schema records `schema_name`, `schema_version`, `evidence_label`,
`complete_regular_evidence`, `expected`, `upstream_stages`, `observed`, and
`reasons`. Expected rows come from downloaded `regular_matrix_validation.json`
when available; otherwise they are materialized from the checked-in
`mi300a_problem_size_discovery_plan.json`. The maintained regular workflow
expects 308 Tier 1 rows plus 1108 Tier 2 rows, for 1416 total data rows. Observed
coverage is counted from `sim_results_ci.csv`, `comparison_ci.csv`, and
`regression_ci.csv` after headers are excluded. The comparison/regression CSVs
can be empty fallback files when no reference CSV is present; those fallbacks are
insufficient evidence, not successful comparisons.

Evidence labels are narrow regular-artifact coverage labels:

- `complete_regular_evidence` means every required upstream stage succeeded and
  all three required CSV artifacts have observed data-row counts matching the
  expected regular total. It is complete regular artifact coverage only; it does
  not claim simulator accuracy, terminal finishability, terminal provenance, or
  terminal finishability completion.
- `partial_diagnostic_regular_evidence` means the required CSV artifacts are
  present, but at least one upstream stage is non-success/unknown or a non-empty
  observed row count differs from the expected row count. Treat the artifacts as
  diagnostic regular evidence only.
- `insufficient_regular_evidence` means a required CSV is missing, lacks a
  header, is empty, or comparison/regression used an empty reference fallback.
  There is not enough observed artifact evidence to claim regular coverage.

### Bounded finishable-size snapshot contract

The manifest and Tier views come from GitHub Actions discovery run `24959959195` observed at `2026-04-26T17:04:42Z`. This is an immutable observation-time / bounded non-terminal snapshot, not a final/current live finishability claim. The snapshot covers all 1416 discovery-plan entries exactly once and records 23 completed-success eligible entries, 20 timed-out exclusions, and 1373 non-terminal exclusions at the observation time. Do not describe non-terminal exclusions as permanent or current unless new terminal/current evidence is collected and validated.

### Legacy modes

Legacy curated-primary, focused-near-miss-stencil-single, exploratory, and problem-size-discovery dispatch modes were removed from the default benchmark workflow rather than retained as parallel modes. The only compatibility input retained is `ref`, so callers can still dispatch a branch, tag, or SHA. Historical discovery/manifest/provenance files and offline generator/validator scripts remain checked in for auditability and regeneration, but they are not alternate default workflow modes.

### Repo-only/offline static workflow checks

Run the following checks from the repository root when reviewing the MI300A
workflow documentation and checked-in metadata. They are bounded,
offline/repo-only/static checks: they inspect tracked workflow files, manifests,
shard plans, and report inputs without dispatching GitHub Actions, contacting
GitHub, running simulator benchmarks, or regenerating evidence/manifests/data.
The `validate_mi300a_regular_tier1_matrix.py` check is an offline
audit/static-validation check for the retired generated Tier 1 JSON only; the
maintained regular workflow contract check is
`validate_mi300a_regular_workflow_matrix.py` and the 7200-second /
21600-second / max-parallel 14 contract described above.

```bash
python3 -m unittest \
  scripts.tests.test_benchmark_workflow_linear_finishable \
  scripts.tests.test_mi300a_terminal_discovery_workflow \
  scripts.tests.test_validate_mi300a_terminal_discovery_timeout_contract \
  scripts.tests.test_generate_mi300a_terminal_recovery_manifest
python3 scripts/validate_mi300a_finishable_size_manifest.py
python3 scripts/validate_mi300a_regular_workflow_matrix.py \
  --report /tmp/regular_workflow_contract_validation.json
python3 scripts/materialize_mi300a_regular_workflow_matrix.py \
  --summary /tmp/regular_workflow_validation_summary.md \
  --report /tmp/regular_matrix_validation.json
python3 scripts/validate_mi300a_regular_tier1_matrix.py
python3 scripts/validate_mi300a_finishable_evaluation.py
python3 scripts/validate_mi300a_terminal_discovery_shards.py
python3 scripts/generate_mi300a_terminal_recovery_manifest.py --check
```

For a downloaded terminal regular-run artifact bundle, derive selected-run
finishability evidence without contacting GitHub:

```bash
python3 scripts/derive_mi300a_regular_finishability_evidence.py \
  --artifact-root <downloaded-terminal-run-artifacts> \
  --expected-run-id <terminal-run-id> \
  --expected-head-sha <terminal-head-sha> \
  --output-json /tmp/regular_finishability_evidence.json
```

The command rejects non-terminal snapshot metadata (for example, maintained
workflow run `25841274346` or discovery run `24959959195`) as current
finishability input.

The timeout-contract guard is also static, but it requires explicit planned
operator values rather than a repository-default budget:

```bash
python3 scripts/validate_mi300a_terminal_discovery_timeout_contract.py \
  --row-timeout-minutes <planned-row-timeout-minutes> \
  --per-attempt-timeout-sec <planned-attempt-timeout-sec> \
  --row-overhead-sec <planned-checkout-build-upload-overhead-sec>
```

These checks only prove the local static contracts still line up. They do **not** approve, add, reintroduce, or dispatch terminal discovery; do **not** replace the recorded #181 approval plus reviewed-operator-path requirement for the next 3600-second terminal-discovery attempt; do **not** run local simulation or calibration; do **not** regenerate the finishable manifest, Tier matrices, provenance, or benchmark data; and do **not** resolve the finishability evidence or per-row-timeout contract. The updated workflow/readiness tests also prove the visible `.github/workflows/mi300a_terminal_discovery.yml` Actions workflow is retired and that `.github/workflows/benchmark.yml` is the only maintained MI300A benchmark Actions entry point, with exactly `Validation -> Tier 1 -> Tier-1 summary -> Tier 2 -> Summary` and only the optional `ref` input.

## Scripts

- `workloads/scripts/compare_sim_vs_real.py` — Point comparison script
- `workloads/scripts/compare_regression.py` — Regression-based comparison
- `workloads/scripts/run_all.sh` — HW measurement script
- `scripts/ci_artifacts_provenance.py` — Legacy `ci_artifacts` archive manifest writer/checker
- `scripts/generate_mi300a_problem_size_discovery_plan.py` — Archival/offline validator-emitter for `mi300a_problem_size_discovery_plan.json`. On the current linear workflow it validates and re-emits the checked-in 1416-entry plan; with an explicit legacy/fixture workflow it can still rebuild and validate entries, including mapping failures, duplicate job/artifact ids, and the fixed 3600s timeout contract.
- `scripts/generate_mi300a_terminal_discovery_shards.py` — Generates the terminal discovery shard manifest plus six per-shard matrix JSON files from the checked-in 1416-entry plan, keeping each shard at <=256 per-size attempts while grouping visible matrix rows one per workflow benchmark.
- `scripts/validate_mi300a_terminal_discovery_shards.py` — Static benchmark-shard validator that re-derives the manifest/matrices, rejects omissions/duplicates/tampering, proves every nested per-size attempt is preserved exactly once, and asserts hotspot/48 at plan_index 474 with 3600s timeouts.
- `scripts/validate_mi300a_terminal_discovery_timeout_contract.py` — Static/dry-run timeout-contract guard for a candidate terminal-discovery operator budget. It rejects any visible benchmark row whose `attempt_count * per_attempt_timeout_sec + row_overhead_sec` cannot fit under the row/action timeout, without dispatching Actions or running simulators.
- `scripts/generate_mi300a_terminal_recovery_manifest.py` — Builds or `--check`s the read-only run `25111815292` recovery manifest from the checked-in 1416-entry plan and incomplete-coverage report. The manifest contains only the 940 unobserved plan indices grouped into recovery rows; it does not synthesize outcomes or complete the finishability evidence.
- `scripts/generate_mi300a_terminal_recovery_dry_run_plan.py` — Builds or
  `--check`s the recovery-only dry-run matrix from the recovery manifest.
  It expands the 940 missing plan indices into one single-attempt row each,
  excludes the 476 prior observed incomplete-evidence-only entries, partitions
  rows into 12 static batches, and preserves the per-row-timeout 14-way / 600-second row /
  3600-second action-layer budget without changing the visible Actions surface.
- `scripts/validate_mi300a_terminal_recovery_dry_run_plan.py` — Read-only
  pre-dispatch validator for the checked-in dry-run plan. It re-derives the
  plan from the recovery manifest, rejects duplicate/missing/extra recovery indices or
  overlap with prior observed records, verifies complete prior rows stay out of
  recovery, verifies the per-row-timeout budget contract, and reports the plan's non-goals.
- `scripts/validate_mi300a_recovery_evidence_readiness_boundary.py` — Read-only
  recovery-evidence readiness-boundary validator for finishability recovery evidence. It verifies that the 476
  cancelled-run observations plus a future 940-entry recovery-only artifact set
  remain blocked under the current single-run source policy and cannot drive
  provenance/finishable/Tier regeneration unless a reviewed multi-source schema
  or a fresh full 1416-entry terminal collection exists.
- `scripts/collect_mi300a_terminal_discovery_provenance.py` — Terminal outcome collector that merges legacy per-entry terminal discovery outcome JSON artifacts and/or benchmark-level JSON artifacts with nested per-size outcome records into compact provenance with one terminal outcome accounting record per 1416-entry plan entry. If a download contains both shapes for a plan entry, it validates both records, rejects terminal/provenance-critical conflicts, and prefers the benchmark-level nested representation in its logical 1416-record view.
- `scripts/validate_mi300a_terminal_discovery_provenance.py` — Static terminal provenance validator that requires completed/failed/timed_out/skipped/cancelled coverage for all 1416 plan entries and rejects non-terminal/missing buckets in terminal mode.
- `scripts/validate_mi300a_discovery_provenance.py` — Static discovery provenance guard that rejects stale 1415-entry discovery evidence and verifies the refreshed evidence references the 1416-entry plan coverage contract.
- `scripts/generate_mi300a_finishable_size_manifest.py` — Generates the bounded finishable-size manifest plus Tier 1/Tier 2 matrix views from compact discovery outcome provenance.
- `scripts/validate_mi300a_finishable_size_manifest.py` — Static finishable-size manifest guard that re-derives the finishable manifest from the 1416-entry plan/provenance, rejects duplicate/missing/fabricated outcome rows, and proves `hotspot`/`48` remains represented.
- `scripts/validate_mi300a_regular_workflow_matrix.py` — Static/read-only guard for the maintained full regular workflow matrix and `.github/workflows/benchmark.yml` surface. It verifies exact 1416-entry plan coverage, 82 per-benchmark matrix rows, `max-parallel: 14`, per-simulator `timeout_sec: 7200`, 21600-second / 360-minute benchmark-tier job limits, and no terminal-discovery/extra dispatch surface.
- `scripts/materialize_mi300a_regular_workflow_matrix.py` — Static/read-only runtime matrix materializer for the maintained regular workflow. It consumes `mi300a_problem_size_discovery_plan.json`, validates exact 1416-entry plan coverage, and emits 19 Tier 1 rows / 308 entries plus 63 Tier 2 rows / 1108 entries without running simulators, contacting GitHub, or regenerating finishable/provenance/Tier outputs.
- `scripts/validate_mi300a_regular_tier1_matrix.py` — Historical broadened regular Tier 1 breadth-matrix guard that validates checked-in plan/manifest source references, at least 12 rows from at least 10 Tier 1 benchmark families, unique artifact identifiers, and per-row 600s/3600s budget accounting without running simulators or regenerating evidence.
- `scripts/validate_mi300a_finishable_evaluation.py` — Static bounded finishable evaluation validator/summary CLI for the current bounded finishable evaluation set. It derives `mi300a_finishable_evaluation_set.json` and `mi300a_finishable_evaluation_set_summary.json` from the finishable manifest plus Tier views, validates exact all-and-only eligible coverage, and prints deterministic JSON or text summaries.
- `scripts/remerge_ci_artifacts.py` — Legacy guardrail-first merge + report regeneration workflow

## MI300A 3600s problem-size discovery plan (archival/offline)

The current `.github/workflows/benchmark.yml` is the linear regular path documented above; it intentionally does not include a problem-size-discovery dispatch mode or inline legacy full-mode matrices. The discovery plan is now also the maintained regular runtime matrix source: validation materializes all 1416 entries into per-benchmark rows while applying the 14-way / 7200-second / 21600-second regular contract. The plan remains a checked-in archival contract for the discovery provenance guard, the finishable-size manifest validator, and historical Tier matrix validators.

Validate and re-emit the checked-in discovery plan without dispatching simulations:

```bash
python3 scripts/generate_mi300a_problem_size_discovery_plan.py \
  --output benchmark-comparison/mi300a_problem_size_discovery_plan.json
```

With the current linear workflow, the generator detects that legacy full-mode matrix entries are absent, then validates and re-emits the archived plan against the tracked MI300A timing CSVs. The output stays byte-for-byte compatible with `benchmark-comparison/mi300a_problem_size_discovery_plan.json` and preserves the existing `workflow_job`/Tier/job metadata as historical provenance fields only. They are not instructions to add legacy jobs back to the workflow.

For offline fixture or historical-branch use, passing `--workflow <legacy benchmark.yml>` still rebuilds entries from a legacy full-mode matrix. Each rebuilt entry carries a single runner-loop `sizes` token, `flag_template`, `size_label_template`, Tier/job metadata, job/artifact-safe ids, and `timeout_sec: 3600`; the command fails if mapping, duplicate-id, timeout, or raw MI300A timing reconciliation contracts are violated.

## Terminal discovery shard readiness surface (offline; Actions dispatcher retired)

For the retired/readiness guide (approval gate, no-dispatch status, future shard/run
expectations, expected artifacts, and provenance collection), see
`docs/mi300a_terminal_discovery_operator.md`.

The terminal-discovery shard surface is generated offline from
`benchmark-comparison/mi300a_problem_size_discovery_plan.json`. It is not a mode in
`.github/workflows/benchmark.yml`, and the visible
`.github/workflows/mi300a_terminal_discovery.yml` Actions workflow is retired.
`.github/workflows/benchmark.yml` is the only maintained MI300A benchmark Actions
entry point: `Validation -> Tier 1 -> Tier-1 summary -> Tier 2 -> Summary`
with only the optional `ref` input. The next 3600-second terminal-discovery
attempt has recorded human approval (#181), but still requires a reviewed
operator path before anything is dispatched. The checked-in offline artifacts are:

- `benchmark-comparison/generated/mi300a_terminal_discovery_shard_manifest.json`
- `benchmark-comparison/generated/mi300a_terminal_discovery_shard_01_matrix.json` through
  `benchmark-comparison/generated/mi300a_terminal_discovery_shard_06_matrix.json`
- `benchmark-comparison/generated/mi300a_terminal_discovery_run_25111815292_recovery_manifest.json`
- `benchmark-comparison/generated/mi300a_terminal_discovery_run_25111815292_recovery_dry_run_plan.json`

Regenerate the shard manifest/matrices deterministically only when intentionally
refreshing the offline readiness manifests from the checked-in 1416-entry plan:

```bash
python3 scripts/generate_mi300a_terminal_discovery_shards.py
```

Validate the checked-in readiness surface with:

```bash
python3 scripts/validate_mi300a_terminal_discovery_shards.py
```

Validate any proposed operator row/action timeout contract with explicit planned values:

```bash
python3 scripts/validate_mi300a_terminal_discovery_timeout_contract.py \
  --row-timeout-minutes <planned-row-timeout-minutes> \
  --per-attempt-timeout-sec <planned-attempt-timeout-sec> \
  --row-overhead-sec <planned-checkout-build-upload-overhead-sec>
```

This timeout guard is static/dry-run only. It reads the tracked shard manifest and per-shard matrices, reports the row budget as JSON, and fails non-zero when a visible benchmark row cannot fit `attempt_count * per_attempt_timeout_sec + row_overhead_sec` under the row timeout budget. Contradictory operator shapes are rejected before any approved recovery run is launched, without dispatching Actions or running simulators.

The run `25111815292` recovery manifest is checked in as a static artifact at
`benchmark-comparison/generated/mi300a_terminal_discovery_run_25111815292_recovery_manifest.json`
and does not need to be regenerated. Historically it was reproducible without
rewriting it via:

```bash
python3 scripts/generate_mi300a_terminal_recovery_manifest.py --check
```

That generator derived the manifest from the checked-in 1416-entry plan plus a
now-removed incomplete-terminal-discovery coverage report, so the `--check`
mode requires that removed coverage doc and is no longer runnable. The
committed manifest contains the 940 plan indices that were not observed in
cancelled run `25111815292`, grouped into 78 workflow-benchmark recovery rows
with source-plan lookup metadata. The 476 observed records from that run
remain incomplete prior evidence only; they are not synthetic terminal
outcomes, not validated complete terminal provenance, and not evidence that
finishability evidence or per-row-timeout evidence is complete.

A recovery-only dry-run matrix is derived from that recovery manifest. Reproduce
and validate the dry-run plan with:

```bash
python3 scripts/generate_mi300a_terminal_recovery_dry_run_plan.py --check
python3 scripts/validate_mi300a_terminal_recovery_dry_run_plan.py
python3 scripts/validate_mi300a_recovery_evidence_readiness_boundary.py
python3 scripts/validate_mi300a_matrix_contract_boundary.py
```

The dry-run validator proves that the 940 missing plan indices are each covered
exactly once, the 476 prior observed records remain excluded as
`incomplete_prior_evidence_only`, and the combined dry-run/prior accounting still
represents all 1416 source plan indices. The readiness-boundary validator
then makes the policy decision explicit: under the current single-run source
schema, that 476+940 accounting union is still blocked for finishability-evidence completion and
for provenance/finishable/Tier regeneration. It also verifies the per-row-timeout
pre-dispatch budget contract: 14-way maximum parallelism, one 600-second attempt
per matrix row, a 600-second row cap, a 3600-second action-layer cap, 12 static
recovery batches, and no row or batch timeout violations. The matrix-contract
boundary validator preserves the per-row-timeout/finishability evidence distinction across three static cases:
the 940-entry recovery-only path has the 14/600/3600 shape but remains
non-completing under the readiness boundary, active run `25196153223` is a separate 6-way /
4320-minute-row / 3600-second nested-attempt contract, and a literal full
1416-entry per-benchmark matrix with a 600-second row cap is rejected before
dispatch with 80 row violations and 32,400-second max-row evidence. The
guarded operator checklist in `docs/terminal_recovery_operator_preflight.md`
documents the branch-only preflight, safe dispatch/monitoring envelope, artifact collection
boundary, and exact run/branch/SHA/provenance fields to record for any later
human-approved recovery attempt. This is a documentation and metadata artifact
only: it does not add or restore a terminal-discovery workflow, does not change
`.github/workflows/benchmark.yml`, does not dispatch/cancel/rerun Actions, does
not run simulators, does not start calibration, does not regenerate
provenance/finishable/Tier artifacts, and does not claim terminal finishability completion.

The generator/validator keep the discovery plan as the only source of truth,
partition all 1416 per-size attempts into six contiguous workflow-benchmark
shards (each <= the default 256 attempts), and require every visible matrix
`include` row to represent one workflow benchmark. Each row carries a nested
`attempts` list with the original per-size `plan_index`, problem-size, hardware
kernel/problem-size, runner flags/templates, stable `runnable_unit_id`, and
`timeout_sec: 3600` metadata needed for terminal outcome accounting. The
validator also asserts the historically important `hotspot`/`48` attempt remains
exactly at `plan_index: 474` (shard 2, hotspot benchmark row attempt 2) and exits
non-zero on omitted, duplicated, reformatted, or tampered manifests.

After a future human-approved terminal attempt finishes, collect the downloaded
benchmark-level artifacts with nested per-size outcome records (or legacy
per-entry outcome artifacts) into deterministic provenance and validate the
terminal coverage with:

```bash
python3 scripts/collect_mi300a_terminal_discovery_provenance.py \
  --outcomes-root terminal-discovery-artifacts \
  --output mi300a_terminal_discovery_provenance.json \
  --run-id <github-run-id>
python3 scripts/validate_mi300a_terminal_discovery_provenance.py \
  --provenance mi300a_terminal_discovery_provenance.json
```

Terminal provenance must contain one accounting record, one outcome row, one
logical artifact entry, and one terminal job entry for every one of the 1416
discovery plan entries. Those logical entries may come from 81 benchmark-level
source JSON files or from the older 1416 per-plan source JSON files. The only
accepted terminal primary buckets are
`completed_success`, `failed`, `timed_out`, `skipped`, and `cancelled`; terminal
mode rejects any `non_terminal`, queued/in-progress, missing-job, or
missing-outcome bucket. A validated terminal provenance JSON can be passed to
`scripts/generate_mi300a_finishable_size_manifest.py` to regenerate finishable
manifest/Tier views from terminal evidence. This does not change the current
checked-in bounded snapshot files: run `24959959195` remains historical
bounded non-terminal evidence until those outputs are intentionally regenerated
from a validated terminal provenance file. This readiness section does **not**
approve, add, reintroduce, or dispatch terminal discovery and does **not**
resolve the finishability or per-row-timeout contract.

## Discovery provenance static check

Validate a compact refreshed discovery provenance JSON before using it as
discovery evidence:

```bash
python3 scripts/validate_mi300a_discovery_provenance.py \
  --provenance benchmark-comparison/provenance/mi300a_problem_size_discovery_run_<run_id>.json
```

The guard requires the `e5-m3-2-refresh-1416-entry-discovery-evidence` branch, full run/head metadata,
`plan_summary.entry_count: 1416`, raw MI300A timing coverage represented as
1416 unique kernel/problem-size pairs with zero missing and zero skipped rows,
and a derivable `hotspot`/`48` entry from
`benchmark-comparison/mi300a_problem_size_discovery_plan.json`. Passing a stale
1415-entry provenance file to this command exits non-zero.

## Finishable-size manifest static check

Validate that the checked-in finishable-size manifest and Tier 1/Tier 2 views
are exactly re-derivable from the 1416-entry discovery plan plus compact outcome
provenance:

```bash
python3 scripts/validate_mi300a_finishable_size_manifest.py
```

The validator is offline/deterministic. It rejects duplicate or missing manifest
plan entries, omitted/tampered `hotspot`/`48`, eligible rows without completed
success outcome/job/artifact evidence, incomplete terminal outcome data, and
invented terminal outcomes for non-terminal or missing discovery jobs. Passing
prints a stable JSON report with entry counts, eligible ranges, Tier 1/Tier 2
counts, and provenance row/job/artifact counts.

## Maintained regular workflow matrix static check

Validate the maintained regular workflow matrix and workflow surface from the
checked-in 1416-entry plan without dispatching or running simulators:

```bash
python3 scripts/validate_mi300a_regular_workflow_matrix.py \
  --report regular_workflow_contract_validation.json
python3 scripts/materialize_mi300a_regular_workflow_matrix.py \
  --summary regular_workflow_validation_summary.md \
  --report regular_matrix_validation.json
```

The validator and materializer are offline/deterministic and read-only. They
verify 19 Tier 1 per-benchmark rows covering 308 plan entries and 63 Tier 2
per-benchmark rows covering 1108 plan entries, for exact 1416-entry coverage.
Every row is normalized to `timeout_sec: 7200`; the workflow job layer remains
`timeout-minutes: 360` / 21600 seconds and `strategy.max-parallel: 14`; and the
benchmark workflow keeps only the regular `ref` dispatch input with no
terminal-discovery or extra dispatch surface. Passing does not run simulators,
query GitHub, dispatch/cancel/rerun Actions, inspect/download/collect artifacts,
regenerate provenance, regenerate the finishable manifest, regenerate Tier
artifacts, or claim terminal finishability completion.

The historical broadened regular Tier 1 breadth/probe matrix can still be validated
for auditability:

```bash
python3 scripts/validate_mi300a_regular_tier1_matrix.py
```

## Bounded finishable evaluation surface

The current post-cleanup MI300A tuning/reporting surface is the bounded
finishable evaluation set (`mi300a-bounded-finishable-evaluation`, version
`2026.04.26-run24959959195`). It is intentionally a reporting surface,
not a new workflow mode or dispatch selector:

| Tier | Benchmarks | Entries | Plan-index ranges | Problem sizes |
|---|---|---:|---|---|
| Tier 1 | `mem_latency_chase`, `occupancy_fma` | 6 | `1`, `3`, `6`, `9`, `237-238` | `256`, `1024`, `8192`, `65536`, `131072`, `262144` |
| Tier 2 | `hotspot`, `gesummv`, `srad` | 17 | `473-482`, `720-722`, `955-958` | `hotspot`: `32`-`640`; `gesummv`: `64`-`192`; `srad`: `64`-`256` |

The checked-in surface files are
`benchmark-comparison/mi300a_finishable_evaluation_set.json` and
`benchmark-comparison/mi300a_finishable_evaluation_set_summary.json`.

Validate or summarize it offline with:

```bash
python3 scripts/validate_mi300a_finishable_evaluation.py
python3 scripts/validate_mi300a_finishable_evaluation.py --summary-format text
```

The command is offline/deterministic and consumes only
`mi300a_finishable_size_manifest.json` plus the checked-in Tier 1/Tier 2 matrix
views. It validates `mi300a_finishable_evaluation_set.json` and
`mi300a_finishable_evaluation_set_summary.json` against those sources, proving
that the evaluation set includes all and only the 23 eligible completed-success
entries (`1`, `3`, `6`, `9`, `237-238`, `473-482`, `720-722`, `955-958`) while
preserving the 1416-entry discovery/finishable contract, the 20 timed-out source
rows, and the 1373 non-terminal source rows as bounded snapshot evidence. It does
not dispatch CI or run simulations. Use `--update` only when intentionally
regenerating the two evaluation-surface JSON files from the accepted checked-in
sources.

## Legacy `ci_artifacts` archive provenance

### Archived manifest

`ci_artifacts/provenance_manifest.json` is retained only as an archive marker for
legacy run `24481375286` / benchmark ref
`ba9bcc06f812b7d5d839d3d4bb43b8d8fd1e1932`. It is **not** the selected accuracy report
provenance, and reviewers should not use it as selected-run evidence for
`accuracy_report.md` or `docs/validation_report.md`.

Current Selected accuracy report provenance lives under:

- `benchmark-comparison/selected-run-25619929396/`
- `benchmark-comparison/selected-run-25619929396/run-view.json`
- run `25619929396`, head `a281789a7354b86a8a04b350c145c573d531f54d`
- selected comparison SHA256
  `867f125aa726ff3a06dff4b40d27008e897347da1001011cd5bd6207823a3a20`

The legacy manifest keeps the historical fields below plus explicit archive
metadata (`archive_status: legacy_archived`, `active_for_m57_reports: false`, and
`superseded_by`) so stale `ci_artifacts` evidence is not confused with current
Selected-run accuracy report evidence:

- `run_id`
- `benchmark_ref_sha`
- `context`
- `generated_at_utc`
- `fingerprint` (deterministic SHA256 over sorted file-hash listing)

### Legacy re-merge guardrail commands

The `ci_artifacts` helper scripts remain in the repository for historical
archive/re-merge checks only. Do not use these commands for the selected accuracy report
provenance unless the accepted selected run is intentionally replaced and the
manifest/archive policy is updated in the same change.

```bash
python3 scripts/ci_artifacts_provenance.py check \
  --artifacts-dir ci_artifacts \
  --manifest ci_artifacts/provenance_manifest.json \
  --expected-run-id 24481375286 \
  --expected-benchmark-ref-sha ba9bcc06f812b7d5d839d3d4bb43b8d8fd1e1932
```

If run id / benchmark ref SHA / fingerprint does not match, the script exits
non-zero and blocks that legacy archive workflow.

For a historical deterministic re-merge of the archived artifact set, run the
guardrail + merge + validation report regeneration in one command:

```bash
python3 scripts/remerge_ci_artifacts.py \
  --artifacts-dir ci_artifacts \
  --manifest ci_artifacts/provenance_manifest.json \
  --comparison benchmark-comparison/comparison_ci.csv \
  --output benchmark-comparison/comparison_ci.csv \
  --expected-run-id 24481375286 \
  --expected-benchmark-ref-sha ba9bcc06f812b7d5d839d3d4bb43b8d8fd1e1932
```

Use `--clear` only if you intentionally want a single-run-only CSV.

Repro hash check:

```bash
shasum -a 256 benchmark-comparison/comparison_ci.csv
```
