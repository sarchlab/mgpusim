# GitHub Actions workflow hygiene

This document records maintained workflow-surface expectations and known GitHub Actions API retention behavior that can otherwise confuse provenance audits.

## Maintained tracked workflow surface

The repository-maintained workflow files under `.github/workflows` are expected to be exactly:

```text
.github/workflows/benchmark.yml
.github/workflows/compile_hsaco.yml
.github/workflows/mgpusim_test.yml
```

For provenance audits, treat the Git-tracked tree and the repository contents API for `.github/workflows?ref=main` as the source of truth for maintained workflow files. GitHub may also expose platform-managed dynamic workflows, such as Dependabot entries, through the Actions workflow API; those are not tracked workflow files in this repository.

## Historical `benchmark-tier-runner.yml` API record

Issue #2732 / milestone M82.2 triaged the stale GitHub Actions workflow record:

```text
id:    260768869
path:  .github/workflows/benchmark-tier-runner.yml
name:  .github/workflows/benchmark-tier-runner.yml
```

That workflow file is deleted from `main` and must not be reintroduced as part of routine hygiene. On 2026-05-13 the configured repository token was permitted to disable the retained Actions API record:

```bash
gh api --method PUT \
  repos/sarchlab/mgpusim-dev/actions/workflows/260768869/disable
```

Post-disable verification at 2026-05-13T22:52Z showed:

```text
260768869  .github/workflows/benchmark-tier-runner.yml  disabled_manually
242969677  .github/workflows/benchmark.yml              active
247895061  .github/workflows/compile_hsaco.yml          active
239626024  .github/workflows/mgpusim_test.yml           active
```

The deleted file still returned HTTP 404 through the contents API at `ref=main`, and the retained workflow record still had only one historical run:

```text
run id:      24403347434
branch:      leo/issue121-tier-gate
head sha:    2fb63e94491994c7177ebf8b53a71921b68ea9ab
event:       push
status:      completed
conclusion:  failure
created:     2026-04-14T14:01:50Z
```

Future provenance audits should ignore this retained historical record when all of the following remain true:

1. `.github/workflows/benchmark-tier-runner.yml` is absent from the tracked tree and from the contents API for `main`.
2. Workflow id `260768869` remains `disabled_manually` or is no longer returned by the Actions workflow API.
3. No new runs appear for workflow id `260768869` after the 2026-05-13 disable timestamp.

If any of those conditions changes, reopen workflow hygiene triage before treating the API record as benign.

No workflow files were added for this triage, and no workflow dispatch, rerun, cancellation, benchmark run, benchmark CSV/report regeneration, or raw benchmark evidence modification was performed.
