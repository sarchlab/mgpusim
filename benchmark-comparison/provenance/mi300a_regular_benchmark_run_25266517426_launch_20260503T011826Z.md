# MI300A regular benchmark current-spec launch observation: run 25266517426

Observation time (UTC): 2026-05-03T01:18:50Z

This is current-spec evidence. The run was dispatched from the current-main-derived branch after commit `0bfb4470e786181d3331f72bbb5b5bc38a66e396`, which updates the maintained regular MI300A Benchmark workflow to `max-parallel: 14`, `timeout_sec: 3600` per simulator invocation, and `timeout-minutes: 720` (`43200` seconds / 12hr) for each benchmark-tier job.

Historical run `25261564015` is old-budget diagnostic context only (600s simulator invocation / 3600s job budgets) and is not used here as current-spec finishability evidence.

## Dispatch

- Command: `gh workflow run benchmark.yml --ref e1-m2-3-align-current-spec-mi300a-workflow-and-capture-t -f ref=e1-m2-3-align-current-spec-mi300a-workflow-and-capture-t`
- Workflow: `MI300A Benchmark` (`benchmark.yml`)
- Run ID: `25266517426`
- Run URL: https://github.com/sarchlab/mgpusim-dev/actions/runs/25266517426
- Event: `workflow_dispatch`
- Head branch/ref: `e1-m2-3-align-current-spec-mi300a-workflow-and-capture-t`
- Head SHA: `0bfb4470e786181d3331f72bbb5b5bc38a66e396`
- Created at: `2026-05-03T01:18:26Z`
- Status at launch observation: `queued`
- Conclusion at launch observation: ``
- Jobs visible at launch observation: `20`

## Guarded current-spec contract

- Regular plan source: `benchmark-comparison/mi300a_problem_size_discovery_plan.json`
- Expected regular plan rows: `1416`
- Maintained workflow dispatch surface: `workflow_dispatch` with only the `ref` input
- Benchmark tier parallelism: `14`
- Per-simulator invocation timeout: `3600` seconds
- Benchmark tier job timeout: `43200` seconds (`720` minutes)

Follow-up observations must wait until run 25266517426 becomes active/terminal or is conclusively blocked, then reconcile job/artifact/log/problem-size outcomes to all `1416` planned entries.
