# MI300A regular benchmark run 25261564015 active observation — 2026-05-02T22:39:21Z

This is a bounded, read-only evidence report for the associated pull request. It records the current GitHub Actions observation and partial artifacts only. It is **not** a terminal completion, finishability, merge-results, or benchmark-success claim.

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
| Run updatedAt returned by GitHub | `2026-05-02T22:28:44Z` |
| Observed at | `2026-05-02T22:39:21Z` |
| Status | `in_progress` |
| Conclusion | `(blank / none)` |

## Terminal/blocker status

- Current run status is `in_progress` with blank conclusion; the run was still active at `2026-05-02T22:39:21Z`.
- `6` jobs were still `in_progress`: `Tier 2 Bench: hist`, `Tier 2 Bench: fused_swiglu`, `Tier 2 Bench: rope`, `Tier 2 Bench: tiled_gemm_16`, `Tier 2 Bench: naive_attention`, `Tier 2 Bench: bh`.
- Full run logs and individual completed-job logs were probed, but GitHub CLI returned: `run 25261564015 is still in progress; logs will be available when it is complete`.
- Blocker: terminal logs are unavailable while the run is active, so per-problem-size failed/timed-out/crashed rows cannot be adjudicated from logs in this observation.
- Next action: after the run reaches a terminal status, re-run the same read-only queries, download final artifacts and logs, then classify failed/timed-out/crashed problem-size rows from logs. Do not claim final finishability from this active snapshot.

## Job outcomes

- Total jobs visible: `83`.
- Job status counts: `completed`=77, `in_progress`=6.
- Job conclusion counts: `failure`=74, `none`=6, `success`=3.
- Validation jobs: `1`; summary/gate jobs: `1`.
- Tier 1 benchmark jobs: `19`; `completed/failure`=18, `completed/success`=1.
- Tier 2 benchmark jobs: `62`; `completed/failure`=55, `completed/success`=1, `in_progress/none`=6.
- Completed failure jobs are job-level failures only in this report (`74` jobs). Per-row failure causes are unknown until logs are available.

Full job table: `benchmark-comparison/provenance/mi300a_regular_benchmark_run_25261564015_job_summary_20260502T223921Z.csv`.

## Artifact availability

- Artifact API returned `76` records: `75` `benchmark-results-*` artifacts and `1` non-benchmark artifact(s): `validation-summary`.
- `gh run download` succeeded for currently available artifacts into a temporary evidence directory (not committed).
- Downloaded benchmark CSV files: `75` of 81 expected regular benchmark rows; `66` had data rows, `9` were header-only, and `6` expected benchmark artifacts were absent.
- Header-only benchmark artifacts: `benchmark-results-atomic-throughput`, `benchmark-results-dwt2d`, `benchmark-results-fp32-fma`, `benchmark-results-fp64-fma`, `benchmark-results-gelu`, `benchmark-results-int-mad`, `benchmark-results-parboil-cutcp`, `benchmark-results-parboil-sad`, `benchmark-results-sfun-sin`.
- Missing expected benchmark artifacts: `benchmark-results-bh`, `benchmark-results-fused-swiglu`, `benchmark-results-hist`, `benchmark-results-naive-attention`, `benchmark-results-rope`, `benchmark-results-tiled-gemm-16`.
- Available validation-summary files: `regular_matrix_validation.json`, `regular_workflow_contract_validation.json`, and `regular_workflow_validation_summary.md`. These validate the static regular matrix/contract; they are not terminal benchmark summaries.

## Benchmark/problem-size row evidence

- Known finished rows from downloaded `benchmark-results-*` CSV artifacts: `434` rows. These rows emitted `kernel_name,problem_size,iterations,avg_ms,min_ms,max_ms` and are listed in `benchmark-comparison/provenance/mi300a_regular_benchmark_run_25261564015_problem_size_rows_20260502T223921Z.csv` with `row_observation=known_finished_from_downloaded_benchmark_results_csv`.
- Known failed/timed-out/crashed problem-size rows from logs: `0` in this observation, because GitHub logs are not available while the run remains active.
- Unknown/unadjudicated plan rows: `1020` strict plan entries are listed in `benchmark-comparison/provenance/mi300a_regular_benchmark_run_25261564015_problem_size_rows_20260502T223921Z.csv` with `row_observation=unknown_no_matching_downloaded_csv_row_and_logs_unavailable`. This conservative strict match requires workflow benchmark + hardware kernel + hardware problem size to appear in the downloaded CSV; emitted CSV kernel names that differ from plan names are kept as known-finished CSV rows but do not reduce the strict unknown count.
- Per-benchmark artifact/job/row summary: `benchmark-comparison/provenance/mi300a_regular_benchmark_run_25261564015_benchmark_summary_20260502T223921Z.csv`.

## Commands used

- `git fetch origin --prune && git pull --ff-only  # pull failed because branch has no upstream; origin/main was already c002b5f4 and this branch is based on it`
- `gh run view 25261564015 --json databaseId,name,workflowName,event,headBranch,headSha,status,conclusion,createdAt,updatedAt,url,jobs`
- `gh api -X GET 'repos/sarchlab/mgpusim-dev/actions/runs/25261564015/jobs?per_page=100' --paginate`
- `gh api -X GET 'repos/sarchlab/mgpusim-dev/actions/runs/25261564015/artifacts?per_page=100' --paginate`
- `gh run download 25261564015 -D <temporary-evidence-dir>`
- `gh run view 25261564015 --log`
- `gh run view --job 74069637177 --log`

## Scope guard

- No workflow dispatch, rerun, cancellation, workflow/source-code edit, result merge, or validation-report regeneration was performed.
- This commit adds durable provenance/report files only.
