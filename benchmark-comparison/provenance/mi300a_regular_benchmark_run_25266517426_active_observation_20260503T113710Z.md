# MI300A regular benchmark current-spec active observation: run 25266517426

Observation time (UTC): 2026-05-03T11:37:10Z

This observation belongs to the current-spec run dispatched from branch `e1-m2-3-align-current-spec-mi300a-workflow-and-capture-t` at head SHA `0bfb4470e786181d3331f72bbb5b5bc38a66e396`.

Historical run `25261564015` remains old-budget diagnostic context only and is not current-spec finishability evidence.

## Run status

- Run URL: `https://github.com/sarchlab/mgpusim-dev/actions/runs/25266517426`
- GitHub run status: `in_progress`
- Conclusion: ``
- Created at: `2026-05-03T01:18:26Z`
- Updated at: `2026-05-03T09:52:36Z`
- Head branch: `e1-m2-3-align-current-spec-mi300a-workflow-and-capture-t`
- Head SHA: `0bfb4470e786181d3331f72bbb5b5bc38a66e396`
- Jobs observed: `83`
- Job status counts: `{'completed': 81, 'in_progress': 2}`
- Job conclusion counts: `{'<none>': 2, 'failure': 78, 'success': 3}`

## Active state and blocker evidence

- Run API status is `in_progress` and conclusion is ``; this is non-terminal.
- Jobs API reports `2` in-progress jobs and `0` queued jobs, so artifact/log/problem-size reconciliation is blocked by non-terminal execution state.
- Overall completed jobs at observation: `81`
- Overall failed jobs already observed: `78`
- Validation jobs: `1` total; status counts `{'completed': 1}`; conclusion counts `{'success': 1}`
- Tier 1 Bench jobs: `19` total; status counts `{'completed': 19}`; conclusion counts `{'failure': 18, 'success': 1}`
- Tier-1 summary/gate jobs: `1` total; status counts `{'completed': 1}`; conclusion counts `{'failure': 1}`
- Tier 2 Bench jobs: `62` total; status counts `{'completed': 60, 'in_progress': 2}`; conclusion counts `{'<none>': 2, 'failure': 59, 'success': 1}`
- Expected `strategy.max-parallel`: `14`.
- Current regular MI300A budget contract: per simulator invocation `3600` seconds; benchmark-tier job layer `43200` seconds (`timeout-minutes: 720`).
- The run is not terminal at this observation, so no artifact/log/benchmark-summary/problem-size row reconciliation against the 1416 planned regular entries was performed.
- This is an active-state observation only and makes no finishability claim.
- I did not dispatch, rerun, cancel, or modify the workflow/source.

## In-progress jobs

| Job | Status | Conclusion | Started at | Completed at |
|---|---|---|---|---|
| Tier 2 Bench: s3d | in_progress |  | 2026-05-03T09:23:27Z |  |
| Tier 2 Bench: bh | in_progress |  | 2026-05-03T09:47:08Z |  |

## Queued jobs

| Job | Status | Conclusion | Started at | Completed at |
|---|---|---|---|---|
| `<none>` |  |  |  |  |

## Failed jobs already observed

| Job | Status | Conclusion | Started at | Completed at |
|---|---|---|---|---|
| Tier 1 Bench: device_memory_read | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:20:18Z |
| Tier 1 Bench: bus_speed_readback | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:47:33Z |
| Tier 1 Bench: device_memory_write | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T03:26:13Z |
| Tier 1 Bench: shared_bw | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:33:01Z |
| Tier 1 Bench: fp64_fma | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:18:42Z |
| Tier 1 Bench: mem_latency_chase | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T03:35:37Z |
| Tier 1 Bench: max_flops | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T03:12:03Z |
| Tier 1 Bench: l2_cache_bw | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T03:00:31Z |
| Tier 1 Bench: int_mad | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:18:42Z |
| Tier 1 Bench: sfun_sin | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:18:43Z |
| Tier 1 Bench: global_bw_copy | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T03:09:00Z |
| Tier 1 Bench: fp32_fma | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:18:42Z |
| Tier 1 Bench: bus_speed_download | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:46:54Z |
| Tier 1 Bench: occupancy_fma | completed | failure | 2026-05-03T01:56:45Z | 2026-05-03T03:32:49Z |
| Tier 1 Bench: atomic_throughput | completed | failure | 2026-05-03T02:18:43Z | 2026-05-03T03:18:50Z |
| Tier 1 Bench: triad | completed | failure | 2026-05-03T02:18:43Z | 2026-05-03T03:33:31Z |
| Tier 1 Bench: branch_div_50pct | completed | failure | 2026-05-03T02:18:43Z | 2026-05-03T03:33:58Z |
| Tier 1 Bench: reduction | completed | failure | 2026-05-03T02:18:44Z | 2026-05-03T03:43:49Z |
| Tier-1 summary/gate | completed | failure | 2026-05-03T03:43:51Z | 2026-05-03T03:43:54Z |
| Tier 2 Bench: gemm | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:59:25Z |
| Tier 2 Bench: 3dconvolution | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:45:51Z |
| Tier 2 Bench: parboil_spmv | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:50:14Z |
| Tier 2 Bench: nn | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T05:46:57Z |
| Tier 2 Bench: 2dconvolution | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:47:55Z |
| Tier 2 Bench: scan | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:43:49Z |
| Tier 2 Bench: 2mm | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:53:43Z |
| Tier 2 Bench: parboil_mri_q | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T05:07:14Z |
| Tier 2 Bench: bs | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:47:22Z |
| Tier 2 Bench: parboil_sgemm | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T06:18:29Z |
| Tier 2 Bench: atax | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T05:03:19Z |
| Tier 2 Bench: dwt2d | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:44:02Z |
| Tier 2 Bench: ep | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:52:23Z |
| Tier 2 Bench: hotspot | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T05:19:04Z |
| Tier 2 Bench: bicg | completed | failure | 2026-05-03T04:43:50Z | 2026-05-03T06:54:55Z |
| Tier 2 Bench: bitonicsort | completed | failure | 2026-05-03T04:44:03Z | 2026-05-03T06:16:02Z |
| Tier 2 Bench: nw | completed | failure | 2026-05-03T04:45:52Z | 2026-05-03T06:00:01Z |
| Tier 2 Bench: pagerank | completed | failure | 2026-05-03T04:47:24Z | 2026-05-03T05:48:15Z |
| Tier 2 Bench: kmeans | completed | failure | 2026-05-03T04:47:56Z | 2026-05-03T06:21:42Z |
| Tier 2 Bench: bfs | completed | failure | 2026-05-03T04:50:15Z | 2026-05-03T06:53:55Z |
| Tier 2 Bench: 3mm | completed | failure | 2026-05-03T04:52:24Z | 2026-05-03T06:16:43Z |
| Tier 2 Bench: correlation | completed | failure | 2026-05-03T04:53:44Z | 2026-05-03T06:46:07Z |
| Tier 2 Bench: covariance | completed | failure | 2026-05-03T04:59:26Z | 2026-05-03T06:54:20Z |
| Tier 2 Bench: fdtd2d | completed | failure | 2026-05-03T05:03:21Z | 2026-05-03T06:19:34Z |
| Tier 2 Bench: gesummv | completed | failure | 2026-05-03T05:07:16Z | 2026-05-03T06:23:43Z |
| Tier 2 Bench: gramschmidt | completed | failure | 2026-05-03T05:19:05Z | 2026-05-03T06:23:11Z |
| Tier 2 Bench: mvt | completed | failure | 2026-05-03T05:46:58Z | 2026-05-03T07:28:19Z |
| Tier 2 Bench: syr2k | completed | failure | 2026-05-03T05:48:16Z | 2026-05-03T07:17:07Z |
| Tier 2 Bench: syrk | completed | failure | 2026-05-03T06:00:03Z | 2026-05-03T07:43:11Z |
| Tier 2 Bench: backprop | completed | failure | 2026-05-03T06:16:03Z | 2026-05-03T08:04:25Z |
| Tier 2 Bench: gaussian | completed | failure | 2026-05-03T06:16:44Z | 2026-05-03T07:25:26Z |
| Tier 2 Bench: hotspot3D | completed | failure | 2026-05-03T06:18:30Z | 2026-05-03T07:48:08Z |
| Tier 2 Bench: huffman | completed | failure | 2026-05-03T06:19:35Z | 2026-05-03T08:20:47Z |
| Tier 2 Bench: lavaMD | completed | failure | 2026-05-03T06:21:43Z | 2026-05-03T07:24:21Z |
| Tier 2 Bench: lud | completed | failure | 2026-05-03T06:23:13Z | 2026-05-03T07:58:34Z |
| Tier 2 Bench: particlefilter | completed | failure | 2026-05-03T06:23:44Z | 2026-05-03T08:07:11Z |
| Tier 2 Bench: pathfinder | completed | failure | 2026-05-03T06:46:09Z | 2026-05-03T07:59:08Z |
| Tier 2 Bench: srad | completed | failure | 2026-05-03T06:53:56Z | 2026-05-03T08:02:54Z |
| Tier 2 Bench: streamcluster | completed | failure | 2026-05-03T06:54:21Z | 2026-05-03T09:31:09Z |
| Tier 2 Bench: ga | completed | failure | 2026-05-03T06:54:56Z | 2026-05-03T08:37:03Z |
| Tier 2 Bench: hist | completed | failure | 2026-05-03T07:17:09Z | 2026-05-03T09:26:18Z |
| Tier 2 Bench: parboil_cutcp | completed | failure | 2026-05-03T07:24:23Z | 2026-05-03T09:24:27Z |
| Tier 2 Bench: parboil_histo | completed | failure | 2026-05-03T07:25:28Z | 2026-05-03T07:27:36Z |
| Tier 2 Bench: parboil_lbm | completed | failure | 2026-05-03T07:27:37Z | 2026-05-03T10:00:09Z |
| Tier 2 Bench: parboil_mri_gridding | completed | failure | 2026-05-03T07:28:20Z | 2026-05-03T09:52:34Z |
| Tier 2 Bench: parboil_sad | completed | failure | 2026-05-03T07:43:12Z | 2026-05-03T10:47:18Z |
| Tier 2 Bench: parboil_stencil | completed | failure | 2026-05-03T07:48:09Z | 2026-05-03T09:24:56Z |
| Tier 2 Bench: parboil_tpacf | completed | failure | 2026-05-03T07:58:35Z | 2026-05-03T10:04:43Z |
| Tier 2 Bench: md5hash | completed | failure | 2026-05-03T07:59:10Z | 2026-05-03T09:54:02Z |
| Tier 2 Bench: qtc | completed | failure | 2026-05-03T08:02:56Z | 2026-05-03T10:19:47Z |
| Tier 2 Bench: gelu | completed | failure | 2026-05-03T08:04:26Z | 2026-05-03T09:04:32Z |
| Tier 2 Bench: softmax | completed | failure | 2026-05-03T08:08:02Z | 2026-05-03T09:23:26Z |
| Tier 2 Bench: layernorm | completed | failure | 2026-05-03T08:20:48Z | 2026-05-03T09:57:56Z |
| Tier 2 Bench: fused_swiglu | completed | failure | 2026-05-03T08:37:05Z | 2026-05-03T10:30:53Z |
| Tier 2 Bench: md_lj | completed | failure | 2026-05-03T09:04:33Z | 2026-05-03T11:19:18Z |
| Tier 2 Bench: rope | completed | failure | 2026-05-03T09:24:28Z | 2026-05-03T11:11:02Z |
| Tier 2 Bench: naive_attention | completed | failure | 2026-05-03T09:24:57Z | 2026-05-03T11:20:28Z |
| Tier 2 Bench: tiled_gemm_16 | completed | failure | 2026-05-03T09:26:20Z | 2026-05-03T10:50:19Z |
| Tier 2 Bench: dmr | completed | failure | 2026-05-03T09:52:35Z | 2026-05-03T09:53:23Z |

## Successful jobs already observed

| Job | Status | Conclusion | Started at | Completed at |
|---|---|---|---|---|
| Validation: full regular matrix | completed | success | 2026-05-03T01:18:28Z | 2026-05-03T01:18:33Z |
| Tier 1 Bench: l1_cache_bw | completed | success | 2026-05-03T01:18:35Z | 2026-05-03T01:56:44Z |
| Tier 2 Bench: sssp | completed | success | 2026-05-03T09:31:10Z | 2026-05-03T09:47:06Z |

## Full job summary artifact

- CSV: `benchmark-comparison/provenance/mi300a_regular_benchmark_run_25266517426_job_summary_20260503T113710Z.csv`
- CSV row count excluding header: `83`

## Commands used

- `git fetch origin main e1-m2-3-align-current-spec-mi300a-workflow-and-capture-t`
- `git pull --ff-only origin e1-m2-3-align-current-spec-mi300a-workflow-and-capture-t`
- `gh run view 25266517426 --json status,conclusion,createdAt,updatedAt,headSha,headBranch,name,displayTitle,event,url,databaseId,attempt,workflowName`
- `gh api repos/sarchlab/mgpusim-dev/actions/runs/25266517426/jobs --paginate`
- `python3 provenance generation script -> benchmark-comparison/provenance/mi300a_regular_benchmark_run_25266517426_active_observation_20260503T113710Z.md and benchmark-comparison/provenance/mi300a_regular_benchmark_run_25266517426_job_summary_20260503T113710Z.csv`
