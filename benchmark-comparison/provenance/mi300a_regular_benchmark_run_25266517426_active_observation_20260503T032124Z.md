# MI300A regular benchmark current-spec active observation: run 25266517426

Observation time (UTC): 2026-05-03T03:21:24Z

This observation belongs to the current-spec run dispatched from branch `e1-m2-3-align-current-spec-mi300a-workflow-and-capture-t` at head SHA `0bfb4470e786181d3331f72bbb5b5bc38a66e396`.

Historical run `25261564015` remains old-budget diagnostic context only and is not current-spec finishability evidence.

## Run status

- Run URL: https://github.com/sarchlab/mgpusim-dev/actions/runs/25266517426
- GitHub run status: `in_progress`
- Conclusion: ``
- Created at: `2026-05-03T01:18:26Z`
- Updated at: `2026-05-03T02:18:45Z`
- Head branch: `e1-m2-3-align-current-spec-mi300a-workflow-and-capture-t`
- Head SHA: `0bfb4470e786181d3331f72bbb5b5bc38a66e396`
- Jobs observed: `20`
- Job status counts: `{'completed': 14, 'in_progress': 6}`
- Job conclusion counts: `{'<none>': 6, 'failure': 12, 'success': 2}`

## Active/blocked state

- Benchmark tier jobs in progress at observation: `6`
- Benchmark tier jobs queued at observation: `0`
- Benchmark tier jobs already concluded failure at observation: `12`
- Expected `strategy.max-parallel`: `14`
- The run is not terminal at this observation, so artifact/log/problem-size row reconciliation against the 1416 planned entries remains pending.
- This is an active-state observation only and makes no finishability claim.

## In-progress jobs

| Job | Status | Conclusion | Started at | Completed at |
|---|---|---|---|---|
| Tier 1 Bench: device_memory_write | in_progress |  | 2026-05-03T01:18:35Z |  |
| Tier 1 Bench: occupancy_fma | in_progress |  | 2026-05-03T01:56:45Z |  |
| Tier 1 Bench: mem_latency_chase | in_progress |  | 2026-05-03T01:18:35Z |  |
| Tier 1 Bench: triad | in_progress |  | 2026-05-03T02:18:43Z |  |
| Tier 1 Bench: branch_div_50pct | in_progress |  | 2026-05-03T02:18:43Z |  |
| Tier 1 Bench: reduction | in_progress |  | 2026-05-03T02:18:44Z |  |

## Failed jobs already observed

| Job | Status | Conclusion | Started at | Completed at |
|---|---|---|---|---|
| Tier 1 Bench: device_memory_read | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:20:18Z |
| Tier 1 Bench: bus_speed_readback | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:47:33Z |
| Tier 1 Bench: shared_bw | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:33:01Z |
| Tier 1 Bench: fp64_fma | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:18:42Z |
| Tier 1 Bench: max_flops | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T03:12:03Z |
| Tier 1 Bench: l2_cache_bw | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T03:00:31Z |
| Tier 1 Bench: int_mad | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:18:42Z |
| Tier 1 Bench: atomic_throughput | completed | failure | 2026-05-03T02:18:43Z | 2026-05-03T03:18:50Z |
| Tier 1 Bench: sfun_sin | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:18:43Z |
| Tier 1 Bench: global_bw_copy | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T03:09:00Z |
| Tier 1 Bench: fp32_fma | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:18:42Z |
| Tier 1 Bench: bus_speed_download | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:46:54Z |

## Complete job snapshot

CSV snapshot: `benchmark-comparison/provenance/mi300a_regular_benchmark_run_25266517426_job_summary_20260503T032124Z.csv`

| Job | Status | Conclusion | Started at | Completed at |
|---|---|---|---|---|
| Validation: full regular matrix | completed | success | 2026-05-03T01:18:28Z | 2026-05-03T01:18:33Z |
| Tier 1 Bench: device_memory_read | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:20:18Z |
| Tier 1 Bench: l1_cache_bw | completed | success | 2026-05-03T01:18:35Z | 2026-05-03T01:56:44Z |
| Tier 1 Bench: bus_speed_readback | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:47:33Z |
| Tier 1 Bench: device_memory_write | in_progress |  | 2026-05-03T01:18:35Z |  |
| Tier 1 Bench: shared_bw | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:33:01Z |
| Tier 1 Bench: fp64_fma | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:18:42Z |
| Tier 1 Bench: occupancy_fma | in_progress |  | 2026-05-03T01:56:45Z |  |
| Tier 1 Bench: mem_latency_chase | in_progress |  | 2026-05-03T01:18:35Z |  |
| Tier 1 Bench: max_flops | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T03:12:03Z |
| Tier 1 Bench: l2_cache_bw | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T03:00:31Z |
| Tier 1 Bench: int_mad | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:18:42Z |
| Tier 1 Bench: atomic_throughput | completed | failure | 2026-05-03T02:18:43Z | 2026-05-03T03:18:50Z |
| Tier 1 Bench: triad | in_progress |  | 2026-05-03T02:18:43Z |  |
| Tier 1 Bench: sfun_sin | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:18:43Z |
| Tier 1 Bench: global_bw_copy | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T03:09:00Z |
| Tier 1 Bench: fp32_fma | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:18:42Z |
| Tier 1 Bench: branch_div_50pct | in_progress |  | 2026-05-03T02:18:43Z |  |
| Tier 1 Bench: bus_speed_download | completed | failure | 2026-05-03T01:18:35Z | 2026-05-03T02:46:54Z |
| Tier 1 Bench: reduction | in_progress |  | 2026-05-03T02:18:44Z |  |
