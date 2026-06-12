# MI300A regular benchmark run 25266517426 terminal observation — 2026-05-03T14:04:49Z

This is a bounded, read-only terminal evidence report for the associated pull request on branch `e1-m2-3-align-current-spec-mi300a-workflow-and-capture-t`. It records GitHub Actions run metadata, terminal job/tier outcomes, artifact/log availability, and per-plan-entry benchmark/problem-size adjudication from current-spec run 25266517426. This run used the current regular workflow budget contract: max-parallel `14`, per-simulator-invocation timeout `3600` seconds, and benchmark-tier timeout `43200` seconds / `720` minutes. The run is terminal with conclusion `failure`, so this is terminal failure evidence, not a benchmark-success or complete-finishability claim.

## Run identity

| Field | Value |
| --- | --- |
| Run database ID | `25266517426` |
| URL | `https://github.com/sarchlab/mgpusim-dev/actions/runs/25266517426` |
| Workflow | `MI300A Benchmark / .github/workflows/benchmark.yml` |
| Run name | `MI300A Benchmark` |
| Event | `workflow_dispatch` |
| Head branch / ref | `e1-m2-3-align-current-spec-mi300a-workflow-and-capture-t` |
| Head SHA | `0bfb4470e786181d3331f72bbb5b5bc38a66e396` |
| Created at | `2026-05-03T01:18:26Z` |
| Run updatedAt returned by GitHub | `2026-05-03T12:35:51Z` |
| Observed at | `2026-05-03T14:04:49Z` |
| Status | `completed` |
| Conclusion | `failure` |

## Terminal status

- Run status is `completed` and conclusion is `failure`.
- The terminal conclusion is `failure`: benchmark-tier jobs failed, the Tier-1 gate failed, validation succeeded, and the final summary job completed successfully.
- No job remained active at this observation; all `84` observed jobs were terminal.
- No benchmark-tier job exceeded the current `43200`s/`720`m benchmark-tier timeout budget based on job start/completion timestamps.
- The final `regular_artifact_coverage_summary.json` labels the evidence `partial_diagnostic_regular_evidence` and `complete_regular_evidence=false`.

## Job and tier outcomes

- Total jobs visible: `84`.
- Job status counts: `completed`=84.
- Job conclusion counts: `failure`=80, `success`=4.
- Validation: `1` success.
- Tier-1 summary/gate: `1` failure.
- Tier 1 benchmark jobs: `19`; `completed/failure`=18, `completed/success`=1.
- Tier 2 benchmark jobs: `62`; `completed/failure`=61, `completed/success`=1.
- Final summary job: `1` success.
- Full job table: `benchmark-comparison/provenance/mi300a_regular_benchmark_run_25266517426_job_summary_20260503T140449Z.csv`.

## Artifact availability

- Artifact API returned `85` records and expired artifacts: `0`.
- Artifact kinds: `benchmark-comparison`=1, `benchmark-results`=81, `comparison-detailed`=1, `validation-report`=1, `validation-summary`=1.
- `gh run download 25266517426 -D <tmp>/run25266517426-evidence/artifacts` succeeded; `81`/`81` expected `benchmark-results-*` artifacts were downloaded.
- Downloaded benchmark CSV files: `82`; benchmark CSV data rows: `559`; header-only benchmark CSVs: `8`.
- Header-only benchmark CSVs: `benchmark-results-atomic-throughput/results_atomic_throughput.csv`, `benchmark-results-dwt2d/results_dwt2d.csv`, `benchmark-results-fp32-fma/results_fp32_fma.csv`, `benchmark-results-fp64-fma/results_fp64_fma.csv`, `benchmark-results-gelu/results_gelu.csv`, `benchmark-results-int-mad/results_int_mad.csv`, `benchmark-results-parboil-sad/results_findminsad.csv`, `benchmark-results-sfun-sin/results_sfun_sin.csv`.
- Final summary artifacts are available: `benchmark-comparison`, `comparison-detailed`, and `validation-report`. The summary coverage artifact reports simulation rows `559` vs expected `1416`, regression rows `434` vs expected `1416`, and comparison rows `1416` vs expected `1416`.
- Full artifact table: `benchmark-comparison/provenance/mi300a_regular_benchmark_run_25266517426_artifact_summary_20260503T140449Z.csv`.

## Log availability and log summary

- `gh run view 25266517426 --log` succeeded and produced `66142` log lines (`7660449` bytes) in the temporary evidence directory.
- `gh run view --job 74114065495 --log` also succeeded for the completed final summary job and produced `2461` log lines (`400849` bytes); stderr was empty.
- Parsed run-log runtime events: finished CSV-emitting events `559`, 3600s timeout events `76`, crash events `3`.
- Timeout events were observed in `76` workflow-size invocations; crash events were observed for `scan 4194304 (exit 2)`, `parboil_histo 65536 (exit 1)`, and `dmr 65536 (exit 2)`.
- Seven finished CSV/log rows did not map to additional unique plan entries because they were duplicate emitted rows for already-mapped plan entries: five duplicate `backprop/layerforward` rows and two duplicate `parboil_sad/ComputeSAD` rows. They are counted in log/artifact summaries but not double-counted as unique plan rows.
- Log event table: `benchmark-comparison/provenance/mi300a_regular_benchmark_run_25266517426_log_event_summary_20260503T140449Z.csv`.

## Benchmark/problem-size row evidence

- Source plan: `benchmark-comparison/mi300a_problem_size_discovery_plan.json` with `1416` planned regular entries; the workflow runtime contract is max-parallel `14`, per simulator invocation `3600`s, benchmark-tier timeout `43200`s / `720`m.
- Known finished plan entries from downloaded benchmark CSVs plus `[OK]` log events: `552`.
- Known timed-out plan entries from run logs at the 3600s invocation budget: `80`.
- Known crashed plan entries from run logs: `3`.
- Unknown/not-attempted-or-unmatched plan entries: `781`. These entries have no mapped OK/TIMEOUT/CRASH event and no mapped downloaded CSV row; for failed benchmark jobs, later sizes generally stop after the first timeout/crash because the workflow loop breaks.
- Tier 1 plan-entry outcomes: finished `139`, timed out `18`, crashed `0`, unknown/not-attempted-or-unmatched `151`.
- Tier 2 plan-entry outcomes: finished `413`, timed out `62`, crashed `3`, unknown/not-attempted-or-unmatched `630`.
- Per-benchmark summary: `benchmark-comparison/provenance/mi300a_regular_benchmark_run_25266517426_benchmark_summary_20260503T140449Z.csv`.
- Per-plan-entry problem-size table: `benchmark-comparison/provenance/mi300a_regular_benchmark_run_25266517426_problem_size_rows_20260503T140449Z.csv`.

## Reconciliation check

Ad hoc Python generation/reconciliation over the downloaded current-run artifacts/logs exited `0` with:

```text
OK: problem_size_rows has 1416 unique plan rows
OK: problem row outcomes finished=552 timed_out_3600s=80 crashed=3 unknown=781 (total=1416)
OK: log_event_summary events finished=559 timed_out_3600s=76 crashed=3
OK: benchmark_summary expected_plan_entries total=1416, csv_data_rows total=559, unpaired duplicate csv/log rows=7
OK: benchmark_summary plan-entry projection total=1416 and every per-benchmark plan-entry count matches problem_size_rows
OK: no benchmark-tier job exceeded 43200s/720m budget; all parsed timeout events are 3600s invocation timeouts
OK: terminal markdown labels this completed/failure run as current-spec terminal failure evidence, not benchmark success or complete finishability
```

## Commands used

- `git fetch origin main e1-m2-3-align-current-spec-mi300a-workflow-and-capture-t`
- `git pull --ff-only origin e1-m2-3-align-current-spec-mi300a-workflow-and-capture-t`
- `gh run view 25266517426 --json databaseId,name,workflowName,event,headBranch,headSha,status,conclusion,createdAt,updatedAt,url,attempt,displayTitle`
- `gh api -X GET 'repos/sarchlab/mgpusim-dev/actions/runs/25266517426/jobs?per_page=100' --paginate`
- `gh api -X GET 'repos/sarchlab/mgpusim-dev/actions/runs/25266517426/artifacts?per_page=100' --paginate`
- `gh run download 25266517426 -D <tmp>/run25266517426-evidence/artifacts`
- `gh run view 25266517426 --log`
- `gh run view --job 74114065495 --log`
- `python3 scripts/generate_mi300a_run_provenance.py --run 25266517426 --artifacts <tmp>/run25266517426-evidence/artifacts` to create the repo-local provenance CSV/markdown files listed above and run the reconciliation assertions.

## Scope guard

- No workflow dispatch, rerun, cancellation, workflow/source-code edit, result merge, validation-report regeneration, simulator run, or benchmark rerun was performed.
- Historical run `25261564015` was not used as final evidence; all final evidence in this report comes from current-spec run `25266517426` and the checked-in current-spec plan/workflow on head SHA `0bfb4470e786181d3331f72bbb5b5bc38a66e396`.
- This update adds durable provenance/report files only; downloaded artifacts and raw logs remain in the uncommitted temporary evidence directory under `<tmp>/run25266517426-evidence/`.
