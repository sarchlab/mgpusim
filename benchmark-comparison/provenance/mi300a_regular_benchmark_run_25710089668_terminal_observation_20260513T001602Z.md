# MI300A regular benchmark run 25710089668 terminal observation (20260513T001602Z)

Terminal artifact collection for the current-main MI300A regular workflow evidence run.

## Source run

- Command: `gh run view 25710089668 --json databaseId,status,conclusion,headBranch,headSha,event,displayTitle,name,workflowName,createdAt,updatedAt,url,jobs,attempt,startedAt,workflowDatabaseId,number`
- Run status/conclusion: `completed` / `failure`
- Workflow/event: `MI300A Benchmark` / `workflow_dispatch`
- Head branch/SHA: `main` / `6fdbbd1882f6a36c5e0846b9209a9a19c258b486`
- URL: https://github.com/sarchlab/mgpusim-dev/actions/runs/25710089668
- Job count: `85`
- Artifact count from Actions API: `87`

## Fetch and inventory commands

- `gh api repos/sarchlab/mgpusim-dev/actions/runs/25710089668/artifacts --paginate`
- `gh run download 25710089668 --dir <run_workdir>/artifacts`
- Raw artifact zips fetched with `gh api repos/sarchlab/mgpusim-dev/actions/artifacts/<artifact_id>/zip` and recorded in `benchmark-comparison/selected-run-25710089668/raw-zip-sha256.tsv`.

## Maintained workflow contract verification

Fetched validation-summary artifacts preserve `status: pass` for:

- `regular_workflow_contract_validation.json`
- `regular_matrix_validation.json`

The maintained regular workflow contract is 82 benchmark rows / 1416 plan entries (Tier 1: 19 rows / 308 entries; Tier 2: 63 rows / 1108 entries), 7200 seconds per simulator invocation, 21600 seconds (`timeout-minutes: 360`) per benchmark-tier job, and `strategy.max-parallel: 14`.

## Artifacts-only finishability derivation

Command:

```bash
python3 scripts/derive_mi300a_regular_finishability_evidence.py \
  --artifact-root <run_workdir> \
  --expected-run-id 25710089668 \
  --expected-head-sha 6fdbbd1882f6a36c5e0846b9209a9a19c258b486 \
  --output-json <run_workdir>/mi300a_regular_finishability_evidence.json \
  --output-md <run_workdir>/mi300a_regular_finishability_evidence.md
```

Result:

- Regular matrix rows / plan entries: `82` / `1416`
- Benchmark jobs: `82` (cancelled=4, failure=76, success=2)
- Status counts derived from populated simulator timing rows: pass=623, no-result=793, fail=0, timeout=0

`no-result` rows were not relabeled as timeout/crash from job-level failure alone. This is partial artifacts-only evidence from a completed/failure CI run, not a clean pass and not proof of complete terminal finishability for all 1416 plan entries.

## Tracked artifacts added

- `benchmark-comparison/selected-run-25710089668/`
- `benchmark-comparison/provenance/mi300a_regular_benchmark_run_25710089668_artifact_summary_20260513T001602Z.csv`
- `benchmark-comparison/provenance/mi300a_regular_benchmark_run_25710089668_job_summary_20260513T001602Z.csv`
- `benchmark-comparison/provenance/mi300a_regular_benchmark_run_25710089668_finishability_row_summary_20260513T001602Z.csv`
- `benchmark-comparison/provenance/mi300a_regular_benchmark_run_25710089668_terminal_observation_20260513T001602Z.md`
