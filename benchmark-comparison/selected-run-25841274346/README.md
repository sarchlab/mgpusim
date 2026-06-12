# MI300A Benchmark run 25841274346 provenance collection

This directory is a reproducible provenance collection for the maintained
`MI300A Benchmark` GitHub Actions workflow. It records the run metadata, job
metadata, artifact inventory, raw artifact ZIP fingerprints, extracted artifact
inventory, per-job summary, and selected-file SHA256 inventory.

## Source run

- Repository: `sarchlab/mgpusim-dev`
- Run ID: `25841274346`
- Workflow/event: `MI300A Benchmark` / `workflow_dispatch`
- Head branch/SHA: `main` / `e0ef96c57e042c636e2b94db8f294ff8a574bb78`
- Status/conclusion at collection: `in_progress` / ``
- Evidence state: `non_terminal_snapshot`
- Run URL: https://github.com/sarchlab/mgpusim-dev/actions/runs/25841274346
- Collected at: `2026-05-14T10:05:17Z`

## Pending/non-terminal state

This is **not terminal evidence**. The workflow had not reached
`status=completed` at collection time, so no clean pass, no-result, failure,
cancelled, or complete finishability conclusion is claimed from this snapshot.

Pending state recorded here:
- Run status is `in_progress` and conclusion is ``.
- Job status counts: `{'completed': 18, 'in_progress': 2}`.
- Job conclusion counts: `{'empty': 2, 'failure': 15, 'success': 3}`.
- Artifacts are provisional and may increase until the run is terminal.
- This checked-in snapshot stays non-terminal/provisional unless a separate
  terminal package is committed and validated; re-run the collector after the
  run reaches `completed` before deriving or claiming terminal finishability
  evidence.

## Inventory files

- `run-view.json`: raw `gh run view` metadata, annotated with collection state.
- `jobs.json`: raw `gh run view --json jobs` metadata.
- `jobs-summary.tsv`: `20` per-job summary rows.
- `artifacts.tsv`: `18` Actions artifact inventory rows.
- `raw-zip-sha256.tsv`: `18` raw artifact ZIP SHA256 rows.
- `raw-zip-entry-counts.tsv`: raw ZIP member inventory/count rows.
- `artifact-file-inventory.tsv`: `20` extracted artifact file rows.
- `selected-file-sha256.tsv`: SHA256 inventory for tracked curated files in this directory.

## Maintained workflow contract snapshot

- Validation summary status: `pass`
- Matrix rows / plan entries: Tier 1 `19` / `308`, Tier 2 `63` / `1108`
- Runtime contract: per-invocation `7200` seconds, benchmark job `21600` seconds, max-parallel `14`

## Reproduction command

Run from the repository root with GitHub CLI authentication:

```bash
scripts/collect_mi300a_benchmark_run_provenance.py --run-id 25841274346 --repo sarchlab/mgpusim-dev --output-dir benchmark-comparison/selected-run-25841274346
```

Raw ZIP payloads are written under `raw-zips/` for hashing/extraction. They
are intentionally excluded from the selected-file SHA256 inventory; retain or
delete them according to local storage policy after committing the TSV
inventories and curated extracted files needed for review.
