# MI300A regular benchmark launch provenance — run 25261564015

This document is a bounded launch-provenance record for issue #79. It records the guarded dispatch of the maintained regular MI300A benchmark workflow from the then-current `main` commit only. It is not a completion, finishability, Tier, artifact, or benchmark-results claim.

## Boundaries observed

- No workflow source changes were made for this launch.
- No new dispatch mode, workflow, `repository_dispatch`, or `workflow_call` surface was added or used.
- No rerun or cancellation was requested.
- No workflow logs or artifacts were downloaded or inspected.
- No terminal-discovery restoration, simulator calibration, local sweep, finishable regeneration, or provenance completion claim is included here.

## Current-main verification

- Repository fetch/pull command: `git fetch origin main --prune && git checkout main && git pull --ff-only origin main`.
- Verified local `main` and `origin/main` both resolved to `c002b5f4a39f44f13f19ecd7904727c96b1951d9` immediately before dispatch.
- Dispatch target: `.github/workflows/benchmark.yml` on `ref=main` with workflow input `ref=main`.

## Pre-dispatch workflow-surface and matrix validation

Static checks confirmed the maintained workflow surface and regular matrix contract before dispatch:

- `.github/workflows/benchmark.yml` exposes `workflow_dispatch` with exactly one input, `ref`.
- The `on:` surface contains no `repository_dispatch` and no `workflow_call`.
- Both benchmark tiers carry `strategy.max-parallel: 14`.
- Both benchmark tier jobs carry `timeout-minutes: 60`.
- Benchmark metadata defaults `timeout_sec` to `600`, rejects non-600 values, and invokes the simulator through `timeout "${TIMEOUT_SEC}"`.

Repo-local validation commands completed successfully:

```text
python3 scripts/validate_mi300a_regular_workflow_matrix.py --report ../<workspace>/regular_workflow_contract_validation.json
python3 scripts/materialize_mi300a_regular_workflow_matrix.py --summary ../<workspace>/regular_workflow_validation_summary.md --report ../<workspace>/regular_matrix_validation.json
python3 scripts/validate_mi300a_finishable_size_manifest.py
```

Validation summary from the generated reports:

| Report | Status | Total plan entries | Total matrix rows | Tier 1 rows / entries | Tier 2 rows / entries |
| --- | --- | ---: | ---: | ---: | ---: |
| `regular_workflow_contract_validation.json` | pass | 1416 | 81 | 19 / 308 | 62 / 1108 |
| `regular_matrix_validation.json` | pass | 1416 | 81 | 19 / 308 | 62 / 1108 |

## Duplicate-active-run guard

Before dispatch, recent `benchmark.yml` `workflow_dispatch` runs on `main` were checked with `gh run list --workflow benchmark.yml --event workflow_dispatch --branch main --limit 20 ...`.

- No active `workflow_dispatch` run for `.github/workflows/benchmark.yml` on `main` existed before dispatch.
- The most recent `main` runs were completed failures on older SHAs, including:
  - `25257043354`, created `2026-05-02T16:58:46Z`, `completed/failure`, `headSha=90dbb04f9b9e021dd2d1d6ca98010f1387fc0a57`.
  - `25248489660`, created `2026-05-02T09:04:12Z`, `completed/failure`, `headSha=b95e63cad2ae6dcd1cd45addd3b722f7f6da171e`.
  - `25242755508`, created `2026-05-02T03:31:26Z`, `completed/failure`, `headSha=590c41dfe39842f809e8ed9e77b9d34b9fe5cab7`.
  - `25237442755`, created `2026-05-01T23:23:14Z`, `completed/failure`, `headSha=c4652c8f1ae3fb289b829ec84ad17c4e92c943ca`.
- A broader active-status check found one non-equivalent queued workflow-dispatch run on an archived full-run contract branch (`25196153223`, `headSha=c60f56d19a23b43b020bb4f8e96391c20a2d2454`), not on `main` and not for current-main commit `c002b5f4a39f44f13f19ecd7904727c96b1951d9`.

The duplicate-active-run guard was therefore clear for the required current-main regular benchmark dispatch.

## Dispatch command and accepted run

Dispatch was submitted exactly once:

```text
gh workflow run .github/workflows/benchmark.yml --ref main -f ref=main
```

Timestamp evidence from the command wrapper:

```text
before_dispatch_utc=2026-05-02T20:50:03Z
main_sha=c002b5f4a39f44f13f19ecd7904727c96b1951d9
after_dispatch_utc=2026-05-02T20:50:04Z
```

Accepted run metadata observed after dispatch:

| Field | Value |
| --- | --- |
| Run database ID | `25261564015` |
| URL | <https://github.com/sarchlab/mgpusim-dev/actions/runs/25261564015> |
| Workflow | `MI300A Benchmark` / `.github/workflows/benchmark.yml` |
| Event | `workflow_dispatch` |
| Display title | `MI300A Benchmark` |
| Head branch | `main` |
| Head SHA | `c002b5f4a39f44f13f19ecd7904727c96b1951d9` |
| Created at | `2026-05-02T20:50:04Z` |
| Initial observed run status | `queued` after initial validation/tier-1 job materialization began |
| Initial observed conclusion | none |

## Initial job snapshot

Initial snapshot source: `gh run view 25261564015 --json databaseId,status,conclusion,createdAt,updatedAt,url,headBranch,headSha,event,displayTitle,workflowName,jobs`.

Snapshot observation time was immediately after dispatch discovery. The run-level `updatedAt` value returned by GitHub was `2026-05-02T20:50:13Z`.

Initial job counts:

| Job status / conclusion | Count |
| --- | ---: |
| `completed/success` | 1 |
| `in_progress/none` | 14 |
| `queued/none` | 5 |
| Total jobs visible | 20 |

Visible jobs at the initial snapshot:

| Job ID | Status | Conclusion | Job name | Started at | Completed at |
| ---: | --- | --- | --- | --- | --- |
| 74069630492 | completed | success | Validation: full regular matrix | 2026-05-02T20:50:07Z | 2026-05-02T20:50:12Z |
| 74069637177 | in_progress |  | Tier 1 Bench: shared_bw | 2026-05-02T20:50:14Z | 0001-01-01T00:00:00Z |
| 74069637179 | in_progress |  | Tier 1 Bench: global_bw_copy | 2026-05-02T20:50:14Z | 0001-01-01T00:00:00Z |
| 74069637180 | in_progress |  | Tier 1 Bench: l2_cache_bw | 2026-05-02T20:50:14Z | 0001-01-01T00:00:00Z |
| 74069637181 | in_progress |  | Tier 1 Bench: mem_latency_chase | 2026-05-02T20:50:14Z | 0001-01-01T00:00:00Z |
| 74069637184 | in_progress |  | Tier 1 Bench: device_memory_read | 2026-05-02T20:50:14Z | 0001-01-01T00:00:00Z |
| 74069637192 | in_progress |  | Tier 1 Bench: l1_cache_bw | 2026-05-02T20:50:14Z | 0001-01-01T00:00:00Z |
| 74069637202 | in_progress |  | Tier 1 Bench: bus_speed_readback | 2026-05-02T20:50:14Z | 0001-01-01T00:00:00Z |
| 74069637205 | in_progress |  | Tier 1 Bench: max_flops | 2026-05-02T20:50:14Z | 0001-01-01T00:00:00Z |
| 74069637206 | in_progress |  | Tier 1 Bench: bus_speed_download | 2026-05-02T20:50:14Z | 0001-01-01T00:00:00Z |
| 74069637207 | in_progress |  | Tier 1 Bench: fp64_fma | 2026-05-02T20:50:14Z | 0001-01-01T00:00:00Z |
| 74069637213 | queued |  | Tier 1 Bench: atomic_throughput | 2026-05-02T20:50:13Z | 0001-01-01T00:00:00Z |
| 74069637214 | queued |  | Tier 1 Bench: occupancy_fma | 2026-05-02T20:50:13Z | 0001-01-01T00:00:00Z |
| 74069637215 | in_progress |  | Tier 1 Bench: fp32_fma | 2026-05-02T20:50:14Z | 0001-01-01T00:00:00Z |
| 74069637216 | in_progress |  | Tier 1 Bench: int_mad | 2026-05-02T20:50:14Z | 0001-01-01T00:00:00Z |
| 74069637218 | in_progress |  | Tier 1 Bench: sfun_sin | 2026-05-02T20:50:14Z | 0001-01-01T00:00:00Z |
| 74069637219 | in_progress |  | Tier 1 Bench: device_memory_write | 2026-05-02T20:50:14Z | 0001-01-01T00:00:00Z |
| 74069637220 | queued |  | Tier 1 Bench: branch_div_50pct | 2026-05-02T20:50:13Z | 0001-01-01T00:00:00Z |
| 74069637221 | queued |  | Tier 1 Bench: triad | 2026-05-02T20:50:13Z | 0001-01-01T00:00:00Z |
| 74069637230 | queued |  | Tier 1 Bench: reduction | 2026-05-02T20:50:13Z | 0001-01-01T00:00:00Z |

This initial snapshot only proves launch acceptance and early job materialization. It intentionally does not assert final benchmark success or completeness.
