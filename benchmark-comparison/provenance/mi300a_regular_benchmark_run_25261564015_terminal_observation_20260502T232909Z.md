# MI300A regular benchmark run 25261564015 terminal observation — 2026-05-02T23:29:09Z

This is a bounded, read-only terminal evidence report for the associated pull request. It records GitHub Actions run metadata, terminal job/tier outcomes, artifact/log availability, and per-plan-entry benchmark/problem-size adjudication from downloaded artifacts and logs for a completed/failure run. This is partial diagnostic evidence collected under the older 600s-per-invocation and 3600s-per-job budgets; it does not claim benchmark success, full regular-matrix finishability, or final success under the current 1hr-per-invocation / 12hr-per-job specification.

## Run identity

| Field | Value |
| --- | --- |
| Run database ID | `25261564015` |
| URL | `https://github.com/sarchlab/mgpusim-dev/actions/runs/25261564015` |
| Workflow | `MI300A Benchmark / .github/workflows/benchmark.yml` |
| Run name | `MI300A Benchmark` |
| Event | `workflow_dispatch` |
| Head branch / ref | `main` |
| Head SHA | `c002b5f4a39f44f13f19ecd7904727c96b1951d9` |
| Created at | `2026-05-02T20:50:04Z` |
| Run updatedAt returned by GitHub | `2026-05-02T22:52:19Z` |
| Observed at | `2026-05-02T23:29:09Z` |
| Status | `completed` |
| Conclusion | `failure` |

## Terminal status

- Run status is `completed` and conclusion is `failure`.
- The terminal conclusion is `failure`: 79 benchmark matrix jobs failed, the Tier-1 gate failed, and validation plus the final summary job completed successfully.
- No job remained active at this observation; no job exceeded this run's older workflow 3600s job budget based on job start/completion timestamps.
- This run is terminal but incomplete as a regular benchmark dataset: the final `regular_artifact_coverage_summary.json` labels the evidence `partial_diagnostic_regular_evidence` and `complete_regular_evidence=false`.
- The completed/failure outcome is therefore partial diagnostic evidence under the older 600s/3600s budgets, not a final success claim under the current 1hr/12hr specification.

## Job and tier outcomes

- Total jobs visible: `84`.
- Job status counts: `completed`=84.
- Job conclusion counts: `failure`=80, `success`=4.
- Validation: `1` success.
- Tier-1 summary/gate: `1` failure.
- Tier 1 benchmark jobs: `19`; `completed/failure`=18, `completed/success`=1.
- Tier 2 benchmark jobs: `62`; `completed/failure`=61, `completed/success`=1.
- Final summary job: `1` success.
- Full job table: `benchmark-comparison/provenance/mi300a_regular_benchmark_run_25261564015_job_summary_20260502T232909Z.csv`.

## Artifact availability

- Artifact API returned `85` records and none were expired.
- Artifact kinds: `benchmark-results-*`=81, `validation-summary`=1, `benchmark-comparison`=1, `comparison-detailed`=1, `validation-report`=1.
- `gh run download 25261564015 -D <temporary-evidence-dir>/artifacts` succeeded; 81/81 expected `benchmark-results-*` artifacts were downloaded.
- Downloaded benchmark CSV files: `81`; CSV data rows: `479`; header-only benchmark CSVs: `9`.
- Header-only benchmark artifacts: `benchmark-results-atomic-throughput`, `benchmark-results-dwt2d`, `benchmark-results-fp32-fma`, `benchmark-results-fp64-fma`, `benchmark-results-gelu`, `benchmark-results-int-mad`, `benchmark-results-parboil-cutcp`, `benchmark-results-parboil-sad`, `benchmark-results-sfun-sin`.
- Final summary artifacts are available: `benchmark-comparison`, `comparison-detailed`, and `validation-report`. The summary coverage artifact reports simulation rows `479` vs expected `1416`, regression rows `371` vs expected `1416`, and comparison rows `1416` vs expected `1416`.

## Log availability and log summary

- `gh run view 25261564015 --log` succeeded and produced `63311` log lines (`7328318` bytes) in the temporary evidence directory.
- `gh run view --job 74069637177 --log` also succeeded for a sample completed job (`Tier 1 Bench: shared_bw`); stderr was empty: `true`.
- Parsed run-log runtime events: finished CSV-emitting events `479` (`473` unique workflow benchmark/size labels), 600s timeout events `77`, crash events `2`, generic non-timeout/non-crash failed invocations `0`.
- The per-benchmark `log_*` columns count parsed log-event rows from `log_event_summary`; the `473` unique workflow benchmark/size labels are only a diagnostic distinct-label count.
- Timeout events were observed in `77` workflow-size labels; crash events were observed for `parboil_histo 65536 (exit 1)` and `dmr 65536 (exit 2)`.
- Log event table: `benchmark-comparison/provenance/mi300a_regular_benchmark_run_25261564015_log_event_summary_20260502T232909Z.csv`.

## Benchmark/problem-size row evidence

- Source plan: `benchmark-comparison/mi300a_problem_size_discovery_plan.json` with `1416` plan entries; the workflow runtime contract for this run was the older 600s per simulator invocation and 3600s per benchmark-tier job, not the current 1hr/12hr target specification.
- Known finished plan entries from downloaded benchmark CSVs plus `[OK]` log events: `479`.
- Known timed-out plan entries from run logs at the 600s invocation budget: `81`.
- Known crashed plan entries from run logs: `2`.
- Known generic failed/non-timeout/non-crash plan entries from logs: `0`.
- Unknown/not-attempted-after-prior-failure plan entries: `854`. These later entries have no OK/TIMEOUT/CRASH log event and no downloaded CSV row because each benchmark job loop breaks at the first timeout or crash.
- Tier 1 plan-entry outcomes: finished `127`, timed out `18`, crashed `0`, unknown/not-attempted `163`.
- Tier 2 plan-entry outcomes: finished `352`, timed out `63`, crashed `2`, unknown/not-attempted `691`.
- Per-benchmark summary: `benchmark-comparison/provenance/mi300a_regular_benchmark_run_25261564015_benchmark_summary_20260502T232909Z.csv`.
- Per-plan-entry problem-size table: `benchmark-comparison/provenance/mi300a_regular_benchmark_run_25261564015_problem_size_rows_20260502T232909Z.csv`.

## CSV integrity repair

- `parboil_tpacf` plan indices `1142`-`1144` preserve the three artifact-derived rows separately: `0.102327`, `0.104828`, and `0.102624` ms, tied respectively to finished log lines `51437`, `51441`, and `51445`.
- `benchmark_summary_20260502T232909Z.csv` now uses `log_finished_invocations` as a parsed finished-log-event count, matching `log_event_summary_20260502T232909Z.csv`; the distinct workflow benchmark/size-label count remains documented only as `473` in the log summary text above.
- The run remains completed/failure partial diagnostic evidence under the older 600s/3600s budgets, not final success evidence under the current 1hr/12hr specification.

Concise repo-local CSV/markdown reconciliation check (ad hoc Python over the committed provenance files) exited `0` with:

```text
OK: problem_size_rows has 1416 unique plan rows
OK: problem row outcomes finished=479 timed_out_600s=81 crashed=2 unknown=854 (total=1416)
OK: log_event_summary events finished=479 timed_out_600s=77 crashed=2
OK: benchmark_summary totals expected_plan_entries=1416 csv_data_rows=479 log_finished_invocations=479 log_timeout_invocations_600s=77 log_crash_invocations=2
OK: benchmark_summary plan-entry projection total=1416 (finished=479 timed_out_600s=81 crashed=2 unknown=854)
OK: every benchmark_summary log_* count matches log_event_summary and every plan_entries_* count matches problem_size_rows
OK: parboil_tpacf plan indices 1142-1144 preserve artifact averages/log lines: 1142=0.102327@51437, 1143=0.104828@51441, 1144=0.102624@51445
OK: terminal markdown labels this completed/failure run as partial diagnostic evidence under older 600s/3600s budgets, not current 1hr/12hr success
```

## Commands used

- `git fetch origin --prune && git pull --ff-only`
- `gh run view 25261564015 --json databaseId,name,workflowName,event,headBranch,headSha,status,conclusion,createdAt,updatedAt,url,jobs`
- `gh api -X GET 'repos/sarchlab/mgpusim-dev/actions/runs/25261564015/jobs?per_page=100' --paginate`
- `gh api -X GET 'repos/sarchlab/mgpusim-dev/actions/runs/25261564015/artifacts?per_page=100' --paginate`
- `gh run download 25261564015 -D <temporary-evidence-dir>/artifacts`
- `gh run view 25261564015 --log`
- `gh run view --job 74069637177 --log`
- `python3 - <<'PY' ... PY` repo-local CSV/markdown reconciliation check for the repair (output recorded above)

## Scope guard

- No workflow dispatch, rerun, cancellation, workflow/source-code edit, result merge, or validation-report regeneration was performed.
- This update adds durable provenance/report files only; downloaded artifacts and raw logs remain in an uncommitted temporary evidence directory.
