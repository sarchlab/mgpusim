# Recheck/provenance decision: terminal discovery run 25026018067

Observed at `2026-04-28T02:18:22Z` using only read-only GitHub Actions status and artifact inspection of existing run `25026018067`. No workflow was dispatched, rerun, cancelled, or launched, and no finishable manifest, tier view, or permanent workflow surface was regenerated or altered.

## Run state

`gh run view 25026018067 --json databaseId,status,conclusion,createdAt,startedAt,updatedAt,headBranch,headSha,workflowName,event,url,attempt,number,name,jobs` reported:

- run: `databaseId=25026018067`, `workflowName="MI300A Benchmark"`, `event="workflow_dispatch"`, `attempt=1`, `number=398`
- status/conclusion: `status="queued"`, `conclusion=""`
- timestamps: `createdAt=2026-04-27T23:57:09Z`, `startedAt=2026-04-27T23:57:09Z`, `updatedAt=2026-04-27T23:57:19Z`
- source: `headBranch=e26-m18-launch-per-benchmark-terminal-discovery-replacem`, `headSha=c3b6574fa0ef77f2b47e350b6a5a394b3e54625a`
- URL: <https://github.com/sarchlab/mgpusim-dev/actions/runs/25026018067>
- job accounting from the same response: 87 jobs total; 7 `completed`, 6 `in_progress`, and 74 `queued`; conclusions were 2 `success`, 5 `skipped`, and 80 blank/non-terminal.

The six jobs still reported as `in_progress` were `l2_cache_bw`, `global_bw_copy`, `mem_latency_chase`, `device_memory_read`, `shared_bw`, and `device_memory_write`, all with blank conclusions and no completed timestamp in the API response.

## Required plan contract

Reading the checked-in terminal-discovery plan and shard manifest (without regeneration) confirmed the required terminal-provenance shape remains:

- `benchmark-comparison/generated/mi300a_terminal_discovery_shard_manifest.json`: `entry_count=1416`, `runnable_unit_count=1416`, `benchmark_count=81`, `visible_matrix_row_count=81`, `timeout_sec=3600`
- `benchmark-comparison/mi300a_problem_size_discovery_plan.json`: 1416 entries, all with `timeout_sec=3600`
- `hotspot` problem size `48` remains `plan_index=474` with `timeout_sec=3600`

## Artifact state

`gh api repos/sarchlab/mgpusim-dev/actions/runs/25026018067/artifacts --paginate` reported exactly two non-expired artifacts:

1. `m18-terminal-discovery-outcomes-mi300a-terminal-discovery-benchmark-l1-cache-bw`, size `20601`, created/updated `2026-04-28T00:28:20Z`, expires `2026-05-28T00:28:20Z`
2. `m18-terminal-discovery-validation`, size `1872`, created/updated `2026-04-27T23:57:16Z`, expires `2026-05-27T23:57:16Z`

`gh run download 25026018067 --dir downloads/run25026018067/download` downloaded only those artifacts. The outcome artifact contained 16 standalone per-plan outcome JSON files plus one benchmark-level nested outcome JSON file for the same logical 16 records: `l1_cache_bw` plan indices `17-32`, all with `timeout_sec=3600` and `status="success"`. No artifact covers plan indices `1-16` or `33-1416`; hotspot/48 `plan_index=474` is absent.

Both the pre-collection outcome counter and compact provenance collector rejected the downloaded artifact tree deterministically:

```text
$ python3 scripts/m18_terminal_discovery_operator.py count-outcomes --outcomes-root downloads/run25026018067/download
FAIL: terminal provenance requires exactly one outcome per 1416-entry plan entry after source de-duplication (logical_observed=16, missing=['1-16', '33-1416'], extra=[])
```

```text
$ python3 scripts/collect_mi300a_terminal_discovery_provenance.py \
    --outcomes-root downloads/run25026018067/download \
    --output downloads/run25026018067/mi300a_terminal_discovery_provenance.json \
    --run-id 25026018067 \
    --run-url https://github.com/sarchlab/mgpusim-dev/actions/runs/25026018067 \
    --head-branch e26-m18-launch-per-benchmark-terminal-discovery-replacem \
    --head-sha c3b6574fa0ef77f2b47e350b6a5a394b3e54625a \
    --created-at 2026-04-27T23:57:09Z \
    --started-at 2026-04-27T23:57:09Z \
    --updated-at 2026-04-27T23:57:19Z \
    --workflow-name 'MI300A Benchmark' \
    --issue 262 \
    --dispatch-workflow .github/workflows/benchmark.yml \
    --collected-for read-only-salvageability-recheck
FAIL: MI300A terminal discovery provenance collection rejected artifacts.
  - terminal provenance requires exactly one outcome per 1416-entry plan entry after source de-duplication (logical_observed=16, missing=['1-16', '33-1416'], extra=[])
```

## Decision

Complete conflict-free 1416-logical-record terminal artifacts are not available for run `25026018067` at this observation. The current artifact set is incomplete and cannot produce acceptable terminal provenance or resolve #101. Do not regenerate finishable manifests or tier views from this state.

The next required decision is whether a human/operator wants continued read-only monitoring of this same queued/stale run, or whether to make an explicit operational decision about the stuck queued/in-progress Actions run. This note does not authorize any duplicate dispatch, rerun, cancellation, or permanent workflow-surface change.
