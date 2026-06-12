# MI300A regular benchmark current-spec active/queued observation: run 25266517426

Observation time (UTC): 2026-05-03T05:28:44Z

This observation belongs to the current-spec run dispatched from branch `e1-m2-3-align-current-spec-mi300a-workflow-and-capture-t` at head SHA `0bfb4470e786181d3331f72bbb5b5bc38a66e396`.

Historical run `25261564015` remains old-budget diagnostic context only and is not current-spec finishability evidence.

## Run status

- Run URL: `https://github.com/sarchlab/mgpusim-dev/actions/runs/25266517426`
- GitHub run status: `queued`
- Conclusion: ``
- Created at: `2026-05-03T01:18:26Z`
- Updated at: `2026-05-03T03:43:55Z`
- Head branch: `e1-m2-3-align-current-spec-mi300a-workflow-and-capture-t`
- Head SHA: `0bfb4470e786181d3331f72bbb5b5bc38a66e396`
- Jobs observed: `83`
- Job status counts: `{'completed': 33, 'in_progress': 14, 'queued': 36}`
- Job conclusion counts: `{'success': 2, 'failure': 31, '<none>': 50}`

## Active/queued state

- Run API status is `queued` while the jobs API reports `14` in-progress jobs and `36` queued jobs.
- Overall completed jobs at observation: `33`
- Overall failed jobs already observed: `31`
- Tier 1 benchmark jobs: `19` total; status counts `{'completed': 19}`; conclusion counts `{'failure': 18, 'success': 1}`
- Tier 2 benchmark jobs: `62` total; status counts `{'completed': 12, 'in_progress': 14, 'queued': 36}`; conclusion counts `{'failure': 12, '<none>': 50}`
- Tier-1 summary/gate jobs: `1` total; status counts `{'completed': 1}`; conclusion counts `{'failure': 1}`
- Validation jobs: `1` total; status counts `{'completed': 1}`; conclusion counts `{'success': 1}`
- Expected `strategy.max-parallel`: `14`
- Current regular MI300A budget contract: per simulator invocation `3600` seconds; benchmark-tier job layer `43200` seconds (`timeout-minutes: 720`).
- The run is not terminal at this observation, so no artifact/log/benchmark-summary/problem-size row reconciliation against the 1416 planned entries was performed.
- This is an active/queued-state observation only and makes no finishability claim.

## In-progress jobs

| Job | Status | Conclusion | Started at | Completed at |
|---|---|---|---|---|
| Tier 2 Bench: nn | in_progress |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: parboil_sgemm | in_progress |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: bicg | in_progress |  | 2026-05-03T04:43:50Z |  |
| Tier 2 Bench: pagerank | in_progress |  | 2026-05-03T04:47:24Z |  |
| Tier 2 Bench: nw | in_progress |  | 2026-05-03T04:45:52Z |  |
| Tier 2 Bench: covariance | in_progress |  | 2026-05-03T04:59:26Z |  |
| Tier 2 Bench: 3mm | in_progress |  | 2026-05-03T04:52:24Z |  |
| Tier 2 Bench: bitonicsort | in_progress |  | 2026-05-03T04:44:03Z |  |
| Tier 2 Bench: kmeans | in_progress |  | 2026-05-03T04:47:56Z |  |
| Tier 2 Bench: fdtd2d | in_progress |  | 2026-05-03T05:03:21Z |  |
| Tier 2 Bench: bfs | in_progress |  | 2026-05-03T04:50:15Z |  |
| Tier 2 Bench: correlation | in_progress |  | 2026-05-03T04:53:44Z |  |
| Tier 2 Bench: gesummv | in_progress |  | 2026-05-03T05:07:16Z |  |
| Tier 2 Bench: gramschmidt | in_progress |  | 2026-05-03T05:19:05Z |  |

## Queued jobs

| Job | Status | Conclusion | Started at | Completed at |
|---|---|---|---|---|
| Tier 2 Bench: mvt | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: hotspot3D | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: backprop | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: syr2k | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: syrk | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: gaussian | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: huffman | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: lavaMD | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: lud | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: particlefilter | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: pathfinder | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: srad | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: hist | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: ga | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: streamcluster | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: parboil_histo | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: parboil_cutcp | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: parboil_lbm | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: parboil_mri_gridding | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: parboil_sad | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: parboil_tpacf | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: md5hash | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: parboil_stencil | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: qtc | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: gelu | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: softmax | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: layernorm | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: fused_swiglu | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: md_lj | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: s3d | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: rope | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: naive_attention | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: tiled_gemm_16 | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: sssp | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: bh | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: dmr | queued |  | 2026-05-03T03:43:56Z |  |

## Failed jobs already observed

| Job | Status | Conclusion | Started at | Completed at |
|---|---|---|---|---|
| Tier 1 Bench: device_memory_read | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:20:18Z |
| Tier 1 Bench: bus_speed_readback | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:47:33Z |
| Tier 1 Bench: device_memory_write | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T03:26:13Z |
| Tier 1 Bench: shared_bw | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:33:01Z |
| Tier 1 Bench: fp64_fma | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:18:42Z |
| Tier 1 Bench: occupancy_fma | completed | failure | 2026-05-03T01:56:45Z | 2026-05-03T03:32:49Z |
| Tier 1 Bench: mem_latency_chase | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T03:35:37Z |
| Tier 1 Bench: max_flops | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T03:12:03Z |
| Tier 1 Bench: l2_cache_bw | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T03:00:31Z |
| Tier 1 Bench: int_mad | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:18:42Z |
| Tier 1 Bench: atomic_throughput | completed | failure | 2026-05-03T02:18:43Z | 2026-05-03T03:18:50Z |
| Tier 1 Bench: triad | completed | failure | 2026-05-03T02:18:43Z | 2026-05-03T03:33:31Z |
| Tier 1 Bench: sfun_sin | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:18:43Z |
| Tier 1 Bench: global_bw_copy | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T03:09:00Z |
| Tier 1 Bench: fp32_fma | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:18:42Z |
| Tier 1 Bench: branch_div_50pct | completed | failure | 2026-05-03T02:18:43Z | 2026-05-03T03:33:58Z |
| Tier 1 Bench: bus_speed_download | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:46:54Z |
| Tier 1 Bench: reduction | completed | failure | 2026-05-03T02:18:44Z | 2026-05-03T03:43:49Z |
| Tier-1 summary/gate | completed | failure | 2026-05-03T03:43:51Z | 2026-05-03T03:43:54Z |
| Tier 2 Bench: gemm | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:59:25Z |
| Tier 2 Bench: 3dconvolution | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:45:51Z |
| Tier 2 Bench: parboil_spmv | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:50:14Z |
| Tier 2 Bench: 2dconvolution | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:47:55Z |
| Tier 2 Bench: scan | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:43:49Z |
| Tier 2 Bench: 2mm | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:53:43Z |
| Tier 2 Bench: parboil_mri_q | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T05:07:14Z |
| Tier 2 Bench: bs | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:47:22Z |
| Tier 2 Bench: atax | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T05:03:19Z |
| Tier 2 Bench: dwt2d | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:44:02Z |
| Tier 2 Bench: ep | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:52:23Z |
| Tier 2 Bench: hotspot | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T05:19:04Z |

## Complete job snapshot

CSV snapshot: `benchmark-comparison/provenance/mi300a_regular_benchmark_run_25266517426_job_summary_20260503T052844Z.csv`

| Job | Status | Conclusion | Started at | Completed at |
|---|---|---|---|---|
| Validation: full regular matrix | completed | success | 2026-05-03T01:18:28Z | 2026-05-03T01:18:33Z |
| Tier 1 Bench: device_memory_read | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:20:18Z |
| Tier 1 Bench: l1_cache_bw | completed | success | 2026-05-03T01:18:35Z | 2026-05-03T01:56:44Z |
| Tier 1 Bench: bus_speed_readback | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:47:33Z |
| Tier 1 Bench: device_memory_write | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T03:26:13Z |
| Tier 1 Bench: shared_bw | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:33:01Z |
| Tier 1 Bench: fp64_fma | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:18:42Z |
| Tier 1 Bench: occupancy_fma | completed | failure | 2026-05-03T01:56:45Z | 2026-05-03T03:32:49Z |
| Tier 1 Bench: mem_latency_chase | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T03:35:37Z |
| Tier 1 Bench: max_flops | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T03:12:03Z |
| Tier 1 Bench: l2_cache_bw | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T03:00:31Z |
| Tier 1 Bench: int_mad | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:18:42Z |
| Tier 1 Bench: atomic_throughput | completed | failure | 2026-05-03T02:18:43Z | 2026-05-03T03:18:50Z |
| Tier 1 Bench: triad | completed | failure | 2026-05-03T02:18:43Z | 2026-05-03T03:33:31Z |
| Tier 1 Bench: sfun_sin | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:18:43Z |
| Tier 1 Bench: global_bw_copy | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T03:09:00Z |
| Tier 1 Bench: fp32_fma | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:18:42Z |
| Tier 1 Bench: branch_div_50pct | completed | failure | 2026-05-03T02:18:43Z | 2026-05-03T03:33:58Z |
| Tier 1 Bench: bus_speed_download | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:46:54Z |
| Tier 1 Bench: reduction | completed | failure | 2026-05-03T02:18:44Z | 2026-05-03T03:43:49Z |
| Tier-1 summary/gate | completed | failure | 2026-05-03T03:43:51Z | 2026-05-03T03:43:54Z |
| Tier 2 Bench: gemm | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:59:25Z |
| Tier 2 Bench: 3dconvolution | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:45:51Z |
| Tier 2 Bench: parboil_spmv | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:50:14Z |
| Tier 2 Bench: nn | in_progress |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: 2dconvolution | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:47:55Z |
| Tier 2 Bench: scan | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:43:49Z |
| Tier 2 Bench: 2mm | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:53:43Z |
| Tier 2 Bench: parboil_mri_q | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T05:07:14Z |
| Tier 2 Bench: bs | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:47:22Z |
| Tier 2 Bench: parboil_sgemm | in_progress |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: atax | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T05:03:19Z |
| Tier 2 Bench: bicg | in_progress |  | 2026-05-03T04:43:50Z |  |
| Tier 2 Bench: dwt2d | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:44:02Z |
| Tier 2 Bench: pagerank | in_progress |  | 2026-05-03T04:47:24Z |  |
| Tier 2 Bench: nw | in_progress |  | 2026-05-03T04:45:52Z |  |
| Tier 2 Bench: ep | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T04:52:23Z |
| Tier 2 Bench: covariance | in_progress |  | 2026-05-03T04:59:26Z |  |
| Tier 2 Bench: 3mm | in_progress |  | 2026-05-03T04:52:24Z |  |
| Tier 2 Bench: hotspot | completed | failure | 2026-05-03T03:43:56Z | 2026-05-03T05:19:04Z |
| Tier 2 Bench: bitonicsort | in_progress |  | 2026-05-03T04:44:03Z |  |
| Tier 2 Bench: kmeans | in_progress |  | 2026-05-03T04:47:56Z |  |
| Tier 2 Bench: fdtd2d | in_progress |  | 2026-05-03T05:03:21Z |  |
| Tier 2 Bench: bfs | in_progress |  | 2026-05-03T04:50:15Z |  |
| Tier 2 Bench: correlation | in_progress |  | 2026-05-03T04:53:44Z |  |
| Tier 2 Bench: gesummv | in_progress |  | 2026-05-03T05:07:16Z |  |
| Tier 2 Bench: mvt | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: gramschmidt | in_progress |  | 2026-05-03T05:19:05Z |  |
| Tier 2 Bench: hotspot3D | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: backprop | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: syr2k | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: syrk | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: gaussian | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: huffman | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: lavaMD | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: lud | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: particlefilter | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: pathfinder | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: srad | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: hist | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: ga | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: streamcluster | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: parboil_histo | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: parboil_cutcp | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: parboil_lbm | queued |  | 2026-05-03T03:43:55Z |  |
| Tier 2 Bench: parboil_mri_gridding | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: parboil_sad | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: parboil_tpacf | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: md5hash | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: parboil_stencil | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: qtc | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: gelu | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: softmax | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: layernorm | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: fused_swiglu | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: md_lj | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: s3d | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: rope | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: naive_attention | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: tiled_gemm_16 | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: sssp | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: bh | queued |  | 2026-05-03T03:43:56Z |  |
| Tier 2 Bench: dmr | queued |  | 2026-05-03T03:43:56Z |  |

## Observation commands

- `git fetch origin main e1-m2-3-align-current-spec-mi300a-workflow-and-capture-t`
- `git pull --ff-only origin main`
- `gh run view 25266517426 --json status,conclusion,createdAt,updatedAt,headSha,headBranch,name,displayTitle,event,url,databaseId,attempt,workflowName`
- `gh api repos/sarchlab/mgpusim-dev/actions/runs/25266517426/jobs --paginate`
