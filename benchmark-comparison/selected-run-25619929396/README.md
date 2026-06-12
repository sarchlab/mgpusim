# Selected MI300A Benchmark run 25619929396 artifacts

Tracked copy of selected terminal-run artifacts used to bind the
comparison, validation, and accuracy reports.

Source: curated selected artifacts from MI300A Benchmark run
`25619929396` on head `a281789a7354b86a8a04b350c145c573d531f54d`.
Run status/conclusion: `completed` / `failure`; the final Summary job succeeded
and uploaded the report artifacts.

These files preserve the detailed comparison, merged simulation, regression,
regular artifact coverage, validation report byte manifest, and provenance
metadata inputs selected for report regeneration.

## Final artifact identities

| Artifact | ID | Raw zip SHA256 |
| --- | ---: | --- |
| `accuracy-report` | 6905152547 | `054944f015fe5dfec34fbfb1e03e13b3832a9bec24b21b2f4ba65f8d9c820aaf` |
| `validation-report` | 6905152413 | `6cf7ea08cc28be8f5751364fb583da905b9b8bdec15235a83f896fc8e6b67f5e` |
| `comparison-detailed` | 6905152175 | `22bedf7da325b1b6287f2f21d696481033ff6a9f4aa3c7f3b8926fca45b4d421` |
| `benchmark-comparison` | 6905152122 | `9256a494a783d4c50187d6df60efd19c63dbde13a1266c5434b43dcde771c420` |

`comparison_ci.detailed.csv` is the extracted `comparison-detailed` artifact
CSV. Its SHA256 is
`867f125aa726ff3a06dff4b40d27008e897347da1001011cd5bd6207823a3a20`.

## Validation-report figure byte provenance

The selected validation-report artifact is `validation-report` artifact
`6905152413` from the same run/head. The raw artifact zip digest is recorded in
`raw-zip-sha256.tsv` as
`6cf7ea08cc28be8f5751364fb583da905b9b8bdec15235a83f896fc8e6b67f5e`.
`validation-report-6905152413-sha256.tsv` records the extracted file hashes from
that artifact, including the two summary PNGs committed under `docs/figures/`:

- `docs/figures/sim_vs_hw_scatter.png` sha256
  `7b6f3011ba800d75680673a6d88550213b5920a1d4ac2f9a874834c825add345`,
  1172x1180 px.
- `docs/figures/slowdown_bar_chart.png` sha256
  `ce24d0b08e44699a2e5bf57a6007782d35f7d1049baa8be9c0c4d1b1103fff2e`,
  1783x884 px.

Those PNG bytes are selected-run artifact bytes, not a promise that current local
regeneration with a different plotting stack is byte-identical. Keep the
selected-run hashes above when auditing report provenance unless the
selected run itself is replaced.

## Historical workflow-contract text in raw artifacts

The Markdown and JSON files in this directory preserve selected run
`25619929396` artifact identity. Embedded validation summaries may mention the
runtime contract that existed when that run was captured, including older
3600-second simulator-invocation wording or `timeout-minutes: 720` /
720-minute job-budget text. Those strings are historical selected-run artifact
content only and are not current MI300A workflow guidance.

For current dispatch guidance, use `.github/workflows/benchmark.yml` and the
maintained workflow section in `benchmark-comparison/README.md`: 7200 seconds per
simulator invocation, `timeout-minutes: 360` / 21600 seconds per benchmark-tier
job, and `strategy.max-parallel: 14`. Do not rewrite the selected-run raw
reports merely to refresh contract wording; replace the selected run only with a
separately documented provenance update.
