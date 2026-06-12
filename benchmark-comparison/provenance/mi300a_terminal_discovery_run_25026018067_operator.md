# Operator decision: terminal discovery run 25026018067

Observed and acted at `2026-04-28T02:50-02:55Z` for issue #269. This note records the final current-state recheck of replacement run `25026018067`, the cancellation result for that run only, and the single replacement dispatch. No simulations were run locally, no finishable manifest or Tier view was regenerated, and the permanent `origin/main` workflow surface remains clean; the replacement workflow path exists only on the temporary operator branch named below.

## Final recheck of run 25026018067

`gh run view 25026018067 --json databaseId,status,conclusion,createdAt,startedAt,updatedAt,headBranch,headSha,workflowName,event,url,attempt,number,name,jobs` reported before cancellation:

- run: `databaseId=25026018067`, `workflowName="MI300A Benchmark"`, `event="workflow_dispatch"`, `attempt=1`, `number=398`
- source: `headBranch=e26-m18-launch-per-benchmark-terminal-discovery-replacem`, `headSha=c3b6574fa0ef77f2b47e350b6a5a394b3e54625a`
- URL: <https://github.com/sarchlab/mgpusim-dev/actions/runs/25026018067>
- status/conclusion: `status="queued"`, `conclusion=""`, `updatedAt=2026-04-27T23:57:19Z`
- job accounting: 87 jobs total; 7 `completed`, 6 `in_progress`, 74 `queued`; conclusions were 2 `success`, 5 `skipped`, and 80 blank/non-terminal.
- in-progress terminal-discovery jobs: `l2_cache_bw`, `global_bw_copy`, `mem_latency_chase`, `device_memory_read`, `shared_bw`, and `device_memory_write`.

The artifact API initially showed only two non-expired artifacts (`m18-terminal-discovery-validation` and `m18-terminal-discovery-outcomes-mi300a-terminal-discovery-benchmark-l1-cache-bw`). Downloading that artifact tree and running the branch outcome counter rejected it:

```text
$ python3 scripts/m18_terminal_discovery_operator.py count-outcomes --outcomes-root downloads/run25026018067_download
FAIL: terminal provenance requires exactly one outcome per 1416-entry plan entry after source de-duplication (logical_observed=16, missing=['1-16', '33-1416'], extra=[])
```

The downloaded outcome records represented `l1_cache_bw` plan indices `17-32`: 16 nested benchmark-level records plus 16 standalone per-plan records for the same 16 logical plan entries, all `status="success"`; hotspot/48 `plan_index=474` was absent.

## Cancellation of run 25026018067

Because the final recheck was non-terminal and incomplete, I cancelled only run `25026018067`:

```text
$ gh run cancel 25026018067
✓ Request to cancel workflow 25026018067 submitted.
```

Polling after the cancellation request reached terminal state:

- final run status/conclusion: `status="completed"`, `conclusion="cancelled"`, `updatedAt=2026-04-28T02:53:18Z`
- final job accounting: 88 jobs total, all `completed`; conclusions were 80 `cancelled`, 5 `skipped`, 2 `success`, and 1 `failure`.
- the failure was the post-cancel provenance summary job, which could not collect complete coverage from cancelled/incomplete artifacts.

After cancellation, the artifact API reported eight non-expired artifacts: the validation artifact plus seven partial benchmark outcome artifacts (`mem_latency_chase`, `l1_cache_bw`, `l2_cache_bw`, `shared_bw`, `global_bw_copy`, `device_memory_read`, and `device_memory_write`). The full post-cancel download remained incomplete:

```text
$ python3 scripts/m18_terminal_discovery_operator.py count-outcomes --outcomes-root downloads/run25026018067_download_after_cancel
FAIL: terminal provenance requires exactly one outcome per 1416-entry plan entry after source de-duplication (logical_observed=63, missing=['13-16', '39-48', '53-64', '75-80', '85-96', '108-1416'], extra=[])
```

Raw post-cancel outcome files contained 79 records before source de-duplication: 16 nested plus 63 standalone, 63 unique plan indices in range `1-107`, with raw status counts `success=71` and `timeout=8`. That is not complete, conflict-free 1416-logical-record terminal provenance and cannot resolve #101.

## Required terminal contract preserved

Read-only/static checks of the checked-in shard/plan contract still show the intended replacement shape:

- `benchmark-comparison/generated/mi300a_terminal_discovery_shard_manifest.json`: `entry_count=1416`, `runnable_unit_count=1416`, `benchmark_count=81`, `visible_matrix_row_count=81`, `timeout_sec=3600`.
- `benchmark-comparison/mi300a_problem_size_discovery_plan.json`: 1416 entries, all `timeout_sec=3600`.
- hotspot problem size `48` remains `plan_index=474` with `timeout_sec=3600`.
- `python3 scripts/validate_mi300a_terminal_discovery_shards.py` passed on the operator branch before dispatch.
- `python3 scripts/m18_terminal_discovery_operator.py matrix` materialized 81 visible benchmark rows and 1416 nested per-size attempts.

## Single replacement dispatch

I dispatched exactly one replacement per-benchmark terminal-discovery run after the cancellation request, using a temporary operator branch/path rather than changing the permanent default workflow surface.

- replacement run id: `25031355226`
- replacement run URL: <https://github.com/sarchlab/mgpusim-dev/actions/runs/25031355226>
- workflow: `MI300A Benchmark` / `.github/workflows/benchmark.yml`
- event/ref input: `workflow_dispatch`, `ref=<operator>/m21-terminal-discovery-operator`
- branch: `<operator>/m21-terminal-discovery-operator`
- branch SHA: `d5e41dfeeaf7c68042c394b3092d43c05a4bbb0c`
- run number/attempt: `number=399`, `attempt=1`
- created: `2026-04-28T02:55:15Z`
- initial state observed after dispatch: `status="queued"`, `conclusion=""`, 87 jobs total; the validation job had completed successfully, 5 permanent finishable-size jobs were skipped by the branch gate, 6 terminal-discovery benchmark jobs were already `in_progress`, and 75 jobs were still `queued`.
- initial artifact state: one non-expired validation artifact, `m21-terminal-discovery-validation`, created `2026-04-28T02:55:21Z`.

Expected artifacts are 81 benchmark-level outcome artifacts named `m21-terminal-discovery-outcomes-*`, each containing nested terminal outcome records for its benchmark row, plus `m21-terminal-discovery-provenance` only after exact 1416-record outcome coverage passes. Complete provenance remains acceptable only when the collector/validator accounts for every one of the 1416 plan entries exactly once, preserves original `plan_index`/`problem_size` metadata, keeps every attempt at `timeout_sec=3600`, includes hotspot/48 as `plan_index=474`, and reports only terminal outcome buckets. Until that complete validated provenance exists, #101 remains open and no finishable manifest or Tier view should be regenerated from this evidence.
