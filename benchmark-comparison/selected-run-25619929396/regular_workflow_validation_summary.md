# MI300A Regular Workflow Matrix Validation

Plan: `benchmark-comparison/mi300a_problem_size_discovery_plan.json`

Regular runtime contract: benchmark-tier matrices run with `max-parallel: 14`; each individual simulator invocation is capped at `3600` seconds; each benchmark-tier job is capped at `timeout-minutes: 720` (43200 seconds).

| Workflow job | Matrix rows | Plan entries | Rows over 12h budget | Max invocations/row | Max row budget | Source plan index ranges |
|---|---:|---:|---:|---:|---:|---|
| benchmark-tier1 | 19 | 308 | 18 | 19 | 68400s | `1-308` |
| benchmark-tier2 | 63 | 1108 | 61 | 54 | 194400s | `309-1416` |

Total matrix rows: `82`.
Total covered plan entries: `1416`.

Runtime-budget/passability-risk visibility is informational only; it is not a hard pass/fail gate and is not a simulator outcome.

This validation is static/read-only and does not claim terminal finishability completion.
