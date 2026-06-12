# Proposed Replacement for Project `spec.md`

This document is the repository-tracked proposal for replacing the
human-owned project-root `spec.md`. It is intentionally a proposal only; do not
edit the project-root `spec.md` from automation.

## Project Goal

Update and operate the MI300A benchmark workflow so it exercises every runnable
benchmark/problem-size entry from the maintained MI300A matrix, records what
actually finishes under the accepted runtime contract, and reports accuracy and
coverage without overstating incomplete evidence.

## Maintained MI300A Regular Workflow Contract

The maintained regular workflow is `.github/workflows/benchmark.yml`. It has one
regular path:

```text
Validation -> Tier 1 -> Tier-1 summary -> Tier 2 -> Summary/report
```

The workflow is dispatched manually through `workflow_dispatch` with only the
optional `ref` input. Dispatching the workflow runs the regular path for the
selected ref; there is no maintained `run_mode` selector for discovery,
exploratory, or curated-primary modes.

The validation job must complete before any simulator jobs start. It validates
static workflow/manifest contracts and materializes the regular Tier 1 and Tier
2 matrices from the checked-in plan:

- Source plan: `benchmark-comparison/mi300a_problem_size_discovery_plan.json`
- Total regular problem-size entries: 1416
- Tier 1: 19 benchmark rows / 308 problem-size entries
- Tier 2: 63 benchmark rows / 1108 problem-size entries
- Total workflow matrix rows: 82

The bounded finishable-size manifest and generated finishable Tier views remain
checked in for auditability, but they do not prune the maintained regular
runtime matrix.

The accepted runtime and parallelism contract for both benchmark tiers is:

| Contract item | Required value |
| --- | ---: |
| Per simulator invocation timeout | 7200s (`timeout_sec: 7200`) |
| Per benchmark-tier job timeout | 360min / 21600s (`timeout-minutes: 360`) |
| Maximum parallel benchmark jobs | 14 (`strategy.max-parallel: 14`) |

Tier 1 must run before Tier 2. The Tier-1 summary is intentionally placed
between the two simulation tiers so early Tier 1 diagnostics are visible before
Tier 2 runs. It is informational rather than a finishability gate: Tier 2 may
still run after validation even when Tier 1 artifacts are partial or diagnostic.

## Required Workflow Evidence

A workflow update or acceptance claim must include enough evidence for reviewers
to identify what ran and what finished. At minimum, record:

- dispatch ref, run ID, run URL, head SHA, branch, and workflow name;
- terminal run status and conclusion;
- job outcomes for validation, Tier 1, Tier-1 summary, Tier 2, and summary;
- artifact identities used for simulation, comparison, regression, validation,
  and reporting outputs;
- expected regular row counts from the maintained matrix;
- observed CSV data-row counts for simulator, comparison, and regression
  artifacts; and
- clear labels for complete, partial, or insufficient evidence.

The historical selected-run report provenance used by `accuracy_report.md`
and the report tables remains run `25619929396`, head
`a281789a7354b86a8a04b350c145c573d531f54d`, workflow `MI300A Benchmark`,
status/conclusion `completed` / `failure`. Its regular artifact coverage label
is `partial_diagnostic_regular_evidence`, not complete regular evidence.

Current-main run `25710089668` is separate terminal artifacts-only
finishability/provenance evidence under the maintained workflow contract. Its
retained inventories record 85 jobs in `jobs.json`, 87 artifact inventory rows
in `artifacts.tsv`, and 340 raw ZIP file entries in
`raw-zip-entry-counts.tsv`. The artifacts-only finishability counts are
`pass=623`, `no-result=793`, `fail=0`, and `timeout=0`; absent planned rows stay
`no-result` unless a selected artifact contains an explicit per-size failure or
timeout marker. `selected-run-25710089668` is a curated retained subset, not a
full extraction of the run artifacts.

## Finishability Success Criterion

The project should claim success for the MI300A regular workflow only when the
evidence identifies which benchmark/problem-size entries finish within the
accepted runtime contract. Finishability is about observed completion under the
7200s per-simulator invocation contract and 360min benchmark-tier job cap; it is
not the same as accuracy, calibration quality, or terminal discovery provenance.

Complete regular artifact evidence requires all required upstream stages to
succeed and all required simulator, comparison, and regression CSV artifacts to
contain the expected 1416 data rows. If any stage fails, a CSV is missing, or row
counts do not match the expected matrix, report the result as partial diagnostic
or insufficient evidence. Do not fabricate missing outcomes, infer completion
from static budgets, or describe non-terminal rows as permanently unfinishable.

Historical report selected-run coverage is partial: run `25619929396` has the
expected 1416 comparison rows, but simulator and regression artifacts are
incomplete and both benchmark tiers concluded with failures. Use those
historical selected-run reports as diagnostic evidence only. The current-main
run `25710089668` terminal artifacts provide separate artifacts-only
finishability/provenance evidence, not a replacement accuracy/report-table
source unless a separate report-selection update is made.

## Handling Partial Coverage, Timeouts, and Static Budget Risk

Some per-benchmark matrix rows contain many problem-size invocations. Static
budget fields compare:

```text
planned_invocation_count * 7200s
```

against the 21600s benchmark-tier job cap. This identifies passability risk when
a row has more than three planned invocations, because the job can hit its
360-minute cap before every size runs if invocations consume their full timeout.
The risk fields are not simulator outcomes and must not be used to prune the
regular matrix.

When a workflow times out or uploads partial artifacts:

- keep all available artifacts for diagnosis;
- count completed rows only where the selected artifact has populated simulator
  data;
- label blank or missing `sim_ms` rows as `no-result` for that run;
- preserve explicit timeout and failure reasons in summaries;
- do not relabel absent rows as timeout or crash from job-level failure,
  cancellation, or missing log text alone; and
- avoid converting partial evidence into finishability or accuracy success.

Historical provenance files may contain older timeout wording from the run they
captured. Those strings document archived runs only and do not override the
current 7200s / 360min / max-parallel 14 contract.

## Reporting and Accuracy Expectations

Primary report locations are:

- `accuracy_report.md` for selected-run accuracy, finishability, calibration,
  and chart links;
- `docs/validation_report.md` for detailed MI300A validation metrics and report
  semantics;
- `benchmark-comparison/README.md` for workflow, matrix, artifact-coverage, and
  offline validation guidance;
- `benchmark-comparison/selected-run-25619929396/` for the historical
  selected-run accuracy/report-table artifact snapshot and coverage summaries;
  and
- `benchmark-comparison/selected-run-25710089668/` for separate current-main
  terminal artifacts-only finishability/provenance evidence.

Accuracy reports must preserve raw source data and provenance. Report-time
calibration may be shown in reports and charts, but it must not rewrite raw CSV
artifacts or hide missing rows. Tier 1 calibration benchmarks are reported for
transparency and are not part of the Tier 2 global accuracy rollup.

## L1/L2 Cache-Bandwidth Semantics

L1/L2 cache-bandwidth rows need explicit axis semantics because old and current
rows can mean different things.

Archived selected-run rows for `l1_cache_bw` and `l2_cache_bw` are fixed-work
cache-residency sweeps over `working_set_size`. In the selected run they are
true hardware-flat rows, so booster and trend/scaling metrics are diagnostic
only when the selected-run provenance gate passes. They must not be treated as
value-changing work-scaling evidence.

Current work-scaling metadata uses `num_elements` as the work-amount axis and
defines total read bytes as:

```text
num_elements * num_iterations * bytes_per_read
```

The repeat count is named `num_iterations` in HIP/CUDA workloads and
`num_repeats` in Go simulator runs. For L1, HIP/CUDA keeps the total
`working_set_size` fixed, while the Go simulator keeps each 256-thread read group
L1-resident and lets total input allocation grow with `num_elements`.

Fresh MI300A hardware rows for the redesigned work-scaling L1/L2 path have not
been collected on the local automation host because ROCm/HIP tooling is
unavailable there. The maintained hardware collector path now guards the axis so
future MI300A collection uses `--num_elements` for both cache-bandwidth
benchmarks before rows are accepted as work-scaled evidence.

## Current Calibration and Provenance Caveats

### BFS

BFS selected-run raw rows remain unchanged. The report currently applies a
scale-only report-time model (`sim_scale = 133.490552877`, `fixed_time_ms = 0`) to
12 completed selected-run samples from run `25619929396`. This reduces the
matched-row mean error in the report, but it is not a raw-data fix.

Important caveats remain:

- the hardware comparison rows combine Rodinia and Lonestar BFS rows under the
  same `kernel_name=bfs` identity, while the simulator/report mapping identifies
  one BFS implementation;
- the plan/input identity has documented degree/source-alignment concerns;
- fixed-time-only calibration was insufficient for BFS;
- six large selected-run BFS sizes have no selected simulator result and remain `no-result`; and
- the selected-run regular evidence is partial diagnostic evidence.

Do not claim BFS is fully resolved without preserving source identity, aligning
inputs, confirming the intended metric, and separately reporting large-size
finishability.

### adjust_weights

`adjust_weights` selected-run raw rows also remain unchanged. The report applies
an affine report-time model (`sim_scale = 0.570694861247`,
`fixed_time_ms = 0.005321986901`) fit to 11 completed selected-run samples. The
reported selected-run calibrated metrics are approximately 7.34% mean error and
25.80% max error for those completed samples.

Seven selected-run `adjust_weights` sizes still have `no-result` status. Accuracy claims
must therefore cite the completed-sample provenance and must not imply all 18
planned sizes completed.

### Tier 1 calibration reports

Tier 1 is the calibration/probe tier. Its rows are useful for diagnosing memory,
cache, branch, and bandwidth behavior, but Tier 1 calibration rows are reported
separately from the Tier 2 global accuracy rollup. Compute-bound Tier 1 probes
may be structurally too slow for the per-invocation timeout, and partial Tier 1
artifacts should be treated as diagnostic unless complete evidence exists.

## Required Static Checks Before Claiming a Spec or Workflow Update

For documentation-only spec proposals, do not change workflow behavior,
runtime behavior, raw benchmark artifacts, or selected-run artifact contents.
Run markdown/static hygiene checks appropriate to the changed documents and run:

```bash
git diff --check
```

For workflow/matrix/report changes, also run the relevant repo-only validators
and tests documented in `benchmark-comparison/README.md`, especially the regular
workflow matrix validator and materializer checks. These checks are static or
offline unless explicitly documented otherwise; they do not dispatch workflows,
run long simulator sweeps, or collect hardware artifacts.
