# MI300A regular benchmark current-spec active observation: run 25266517426

Observation time (UTC): 2026-05-03T01:19:11Z

This observation belongs to the current-spec run dispatched from branch `e1-m2-3-align-current-spec-mi300a-workflow-and-capture-t` at head SHA `0bfb4470e786181d3331f72bbb5b5bc38a66e396`.

Historical run `25261564015` remains old-budget diagnostic context only and is not current-spec finishability evidence.

## Run status

- Run URL: https://github.com/sarchlab/mgpusim-dev/actions/runs/25266517426
- GitHub run status: `queued`
- Conclusion: ``
- Created at: `2026-05-03T01:18:26Z`
- Updated at: `2026-05-03T01:18:34Z`
- Jobs observed: `20`
- Job status counts: `{'completed': 1, 'in_progress': 14, 'queued': 5}`

## Active max-parallel guard

- Benchmark tier jobs in progress at observation: `14`
- Expected `strategy.max-parallel`: `14`
- Observation: the active Tier 1 fan-out did not exceed the maintained `max-parallel: 14` guard.

## Job snapshot

| Job | Status | Conclusion | Started at | Completed at |
|---|---|---|---|---|
| Validation: full regular matrix | completed | success | 2026-05-03T01:18:28Z | 2026-05-03T01:18:33Z |
| Tier 1 Bench: device_memory_read | in_progress |  | 2026-05-03T01:18:35Z | 0001-01-01T00:00:00Z |
| Tier 1 Bench: l1_cache_bw | in_progress |  | 2026-05-03T01:18:35Z | 0001-01-01T00:00:00Z |
| Tier 1 Bench: bus_speed_readback | in_progress |  | 2026-05-03T01:18:35Z | 0001-01-01T00:00:00Z |
| Tier 1 Bench: device_memory_write | in_progress |  | 2026-05-03T01:18:35Z | 0001-01-01T00:00:00Z |
| Tier 1 Bench: shared_bw | in_progress |  | 2026-05-03T01:18:35Z | 0001-01-01T00:00:00Z |
| Tier 1 Bench: fp64_fma | in_progress |  | 2026-05-03T01:18:35Z | 0001-01-01T00:00:00Z |
| Tier 1 Bench: occupancy_fma | queued |  | 2026-05-03T01:18:34Z | 0001-01-01T00:00:00Z |
| Tier 1 Bench: mem_latency_chase | in_progress |  | 2026-05-03T01:18:35Z | 0001-01-01T00:00:00Z |
| Tier 1 Bench: max_flops | in_progress |  | 2026-05-03T01:18:35Z | 0001-01-01T00:00:00Z |
| Tier 1 Bench: l2_cache_bw | in_progress |  | 2026-05-03T01:18:35Z | 0001-01-01T00:00:00Z |
| Tier 1 Bench: int_mad | in_progress |  | 2026-05-03T01:18:35Z | 0001-01-01T00:00:00Z |
| Tier 1 Bench: atomic_throughput | queued |  | 2026-05-03T01:18:34Z | 0001-01-01T00:00:00Z |
| Tier 1 Bench: triad | queued |  | 2026-05-03T01:18:34Z | 0001-01-01T00:00:00Z |
| Tier 1 Bench: sfun_sin | in_progress |  | 2026-05-03T01:18:35Z | 0001-01-01T00:00:00Z |
| Tier 1 Bench: global_bw_copy | in_progress |  | 2026-05-03T01:18:35Z | 0001-01-01T00:00:00Z |
| Tier 1 Bench: fp32_fma | in_progress |  | 2026-05-03T01:18:35Z | 0001-01-01T00:00:00Z |
| Tier 1 Bench: branch_div_50pct | queued |  | 2026-05-03T01:18:34Z | 0001-01-01T00:00:00Z |
| Tier 1 Bench: bus_speed_download | in_progress |  | 2026-05-03T01:18:35Z | 0001-01-01T00:00:00Z |
| Tier 1 Bench: reduction | queued |  | 2026-05-03T01:18:34Z | 0001-01-01T00:00:00Z |

Terminal/artifact/log/problem-size reconciliation is still pending until the run reaches a terminal state or is conclusively blocked.
