# MI300A benchmark provenance archive

This directory stores historical MI300A run observations, launch notes, and CSV
summaries exactly as captured for auditability. Files under this directory are
provenance artifacts, not current workflow-contract guidance.

Some archived run `25261564015` and `25266517426` artifacts intentionally mention
older regular-workflow limits such as 3600-second simulator invocations,
`timeout-minutes: 720`, or 720-minute job budgets. Those values describe the
captured run context only. The maintained current MI300A regular workflow
contract is defined in `.github/workflows/benchmark.yml` and
`benchmark-comparison/README.md`: 7200 seconds per simulator invocation,
`timeout-minutes: 360` / 21600 seconds per benchmark-tier job, and
`strategy.max-parallel: 14`.

Do not rewrite raw provenance captures just to update current-contract wording;
preserve their artifact identity unless a provenance replacement is explicitly
documented.
