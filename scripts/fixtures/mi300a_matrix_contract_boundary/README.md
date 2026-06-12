# MI300A matrix-contract boundary fixtures

`archived_full_run_contract/` is a checked-in copy of the archived full-run contract inputs used by
`scripts/validate_mi300a_matrix_contract_boundary.py` for run `25196153223` on branch
`archived-full-run-contract` at dispatch SHA
`c60f56d19a23b43b020bb4f8e96391c20a2d2454`.

The fixture intentionally preserves the historical 81-row / 1416-attempt full-run workflow,
operator, manifest, and shard matrix data so the validator does not depend on a local
historical git object in shallow checkouts. Its archived workflow may contain old
600-second / 60-minute regular-workflow wording. Treat those values as fixture input
bytes for the historical boundary validator, not as current MI300A regular workflow
contract guidance. The maintained current regular contract is 7200 seconds per
simulator invocation, `timeout-minutes: 360` / 21600 seconds per benchmark-tier job,
and `strategy.max-parallel: 14` in `.github/workflows/benchmark.yml`. Do not replace
this fixture with the current `benchmark-comparison/generated` terminal-discovery
matrix unless the matrix-boundary contract is intentionally rebaselined.

The fixture stores the archived `.github/workflows/benchmark.yml` as `benchmark.yml`,
the archived terminal-discovery helper as
`archived_terminal_discovery_operator.py`, and the archived generated manifest/shards by their
original basenames. This keeps fixture data out of executable workflow paths while
preserving the historical artifact contents.
