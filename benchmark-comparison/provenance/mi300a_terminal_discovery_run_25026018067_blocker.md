# Blocker/provenance note: terminal discovery run 25026018067

Observed at `2026-04-28T01:47:12Z` using read-only GitHub Actions status/artifact queries. No workflow was dispatched, rerun, or cancelled.

## Run state

`gh run view 25026018067 --json databaseId,status,conclusion,createdAt,startedAt,updatedAt,headBranch,headSha,workflowName,event,url,attempt,number,name,jobs` reported:

- run: `databaseId=25026018067`, `workflowName="MI300A Benchmark"`, `event="workflow_dispatch"`, `attempt=1`, `number=398`
- status/conclusion: `status="queued"`, `conclusion=""`
- timestamps: `createdAt=2026-04-27T23:57:09Z`, `startedAt=2026-04-27T23:57:09Z`, `updatedAt=2026-04-27T23:57:19Z`
- source: `headBranch=e26-m18-launch-per-benchmark-terminal-discovery-replacem`, `headSha=c3b6574fa0ef77f2b47e350b6a5a394b3e54625a`
- URL: <https://github.com/sarchlab/mgpusim-dev/actions/runs/25026018067>
- job accounting from the same response: 87 jobs total; 2 `completed/success`, 5 `completed/skipped`, 6 `in_progress`, and 74 `queued`.

## Artifact state

`gh api repos/sarchlab/mgpusim-dev/actions/runs/25026018067/artifacts --jq ...` reported exactly 2 non-expired artifacts:

1. `m18-terminal-discovery-validation`, size `1872`, created/updated `2026-04-27T23:57:16Z`
2. `m18-terminal-discovery-outcomes-mi300a-terminal-discovery-benchmark-l1-cache-bw`, size `20601`, created/updated `2026-04-28T00:28:20Z`

The checked-in terminal-discovery shard manifest requires `entry_count=1416`, `runnable_unit_count=1416`, `benchmark_count=81`, `visible_matrix_row_count=81`, `timeout_sec=3600`, and preserves `hotspot`/`48` at `plan_index=474`.

Downloading the two available artifacts yielded only 16 terminal outcome JSON files, all for `l1_cache_bw` plan indices `17-32`. A collection attempt with the merged collector contract failed deterministically:

```text
FAIL: MI300A terminal discovery provenance collection rejected artifacts.
  - terminal provenance requires exactly one outcome per 1416-entry plan entry after source de-duplication (logical_observed=16, missing=['1-16', '33-1416'], extra=[])
```

## Decision required

Run `25026018067` does not currently provide complete 1416-logical-record terminal provenance and cannot support resolving #101 from the available artifact set. Do not regenerate finishable manifests or Tier views from this incomplete state. The next required decision is whether to continue read-only monitoring of this same run until it reaches a terminal/completed artifact state, or have a human/operator decide how to handle the queued/in-progress Actions run; this note does not authorize a duplicate dispatch, rerun, cancellation, or permanent workflow-surface change.
