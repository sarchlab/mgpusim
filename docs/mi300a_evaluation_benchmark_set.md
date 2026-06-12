# Historical MI300A Curated Evaluation Benchmark Set

- **Manifest:** `benchmark-comparison/mi300a_evaluation_set.json`
- **Archived historical workflow plan:** `benchmark-comparison/archive/mi300a_curated_workflow_plan.json`
- **Set ID:** `mi300a-curated-primary`
- **Current manifest version:** `2026.04.26-m67.partial-current`
- **Status:** historical/offline reporting set, superseded for live benchmark CI by the MI300A linear
  regular workflow

This document records the curated MI300A benchmark set. It is retained as
historical tuning/evaluation rationale and as an offline data contract for the
checked-in curated manifest artifacts. It is **not** the current
`.github/workflows/benchmark.yml` operating guide.

Current benchmark workflow guidance lives in `benchmark-comparison/README.md`.
The live workflow is intentionally linear and regular-workflow based:

```text
Validation -> Tier 1 -> Tier-1 summary -> Tier 2 -> Summary
```

The current workflow dispatch accepts only the optional `ref` input, validates
the regular workflow/plan contract, materializes the maintained regular matrices
directly from the
1416-entry problem-size discovery plan
`benchmark-comparison/mi300a_problem_size_discovery_plan.json` (the checked-in
full measured problem-size plan), covers all 1416 plan entries exactly once in
clean per-benchmark rows, and does not provide curated-only or
full-suite mode selectors. Do not use the historical curated workflow-plan material below as
instructions for dispatching the current benchmark workflow.

For post-cleanup MI300A tuning/reporting, use the current bounded finishable
evaluation surface instead: `benchmark-comparison/mi300a_finishable_evaluation_set.json`
and `benchmark-comparison/mi300a_finishable_evaluation_set_summary.json`
(`mi300a-bounded-finishable-evaluation`, version
`2026.04.26-run24959959195`). That surface contains the 23 completed-success
eligible entries from run `24959959195` observed at `2026-04-26T17:04:42Z` across
`mem_latency_chase`, `occupancy_fma`, `hotspot`, `gesummv`, and `srad`. It is a
bounded observation-time snapshot for reporting, not a permanent finishability
claim and not a new `.github/workflows/benchmark.yml` mode.

## Current Curated Report Coverage Semantics

M67 keeps the historical curated membership but regenerates the manifest evidence
against the current checked-in `benchmark-comparison/comparison_ci.csv`. The
validator no longer treats weak current evidence as a full pass. Entries that do
not satisfy their full coverage expectation must be marked explicitly with
`coverage_status: "partial"`, `point_coverage_ready: false`,
`partial_coverage_policy: "partial_current_data_v1"`, and deterministic
`coverage_gaps`.

Current regenerated status:

| Status | Count | Notes |
|---|---:|---|
| Manifest-full coverage | 15 | Meets the entry's manifest expectation with current CSV counts. |
| Partial current-data coverage | 3 | Retained for historical/category continuity, but not counted as full coverage. |

Current aggregate coverage is 182/307 matched hardware points, 88/99 flat
matched points, 94/208 non-flat matched points, and 182 trend points. The partial
entries are:

| Kernel | Current gap | Reporting semantics |
|---|---|---|
| `fused_swiglu` | `matched_non_flat_points` 1/5 | Partial ML fused-activation evidence; not a full point-coverage pass. |
| `nn` | `matched_non_flat_points` 0/5 | Partial nearest-neighbor evidence; matched current rows are flat-only. |
| `rope` | `matched_non_flat_points` 4/5 | Partial ML positional-embedding evidence; one non-flat match short. |

`scripts/generate_validation_report.py --scope curated` may therefore complete
successfully while the generated report and summary show coverage status
`partial`. This is intentional: it preserves the historical set and current raw
provenance without overstating coverage.

## Historical Selection Criteria

The initial curated set was selected from the point-coverage-ready pool
identified in Issue #589:

`bh`, `busspeeddownload`, `busspeedreadback`, `devicememory_write`, `ep`,
`fused_swiglu`, `gelu`, `global_bw_copy`, `hist`, `hotspot`, `l1_cache_bw`,
`l2_cache_bw`, `lud`, `nn`, `nw`, `particlefilter`, `qtc`, `rope`,
`sgemm_tiled`.

A benchmark was selected when it met all of the following historical conditions:

1. **Point coverage was ready.** The then-current `comparison_ci.csv` had matched
   hardware and simulation points that satisfied the applicable coverage rule
   recorded in the manifest.
2. **Tier balance.** The set included Tier 1 system probes and Tier 2
   algorithm/application kernels.
3. **Workload diversity.** The set spanned memory-transfer probes, cache/global
   memory probes, dense linear algebra, stencil, dynamic programming, graph or
   irregular access, histogram/atomic-like aggregation, scientific kernels, and
   ML-oriented elementwise/fused kernels.
4. **Trend usability.** Selected entries had at least three matched points so the
   validator could report trend/scaling fields. Tier 1 probes whose hardware
   curve was all-flat were still included when they had enough matched points
   because they directly tested modeled system parameters.
5. **Determinism.** Manifest entries were sorted by `kernel_name`, and each entry
   carried explicit coverage expectations and evidence.

The resulting historical primary set contains **18 benchmarks**: **6 Tier 1
probes** and **12 Tier 2 kernels**.

## Historical Manifest Schema Summary

The manifest is a versioned JSON document. The top-level fields record the set
identity, data source, selection basis, shared coverage model, the full candidate
pool from Issue #589, any candidate-pool exclusions, and the selected benchmark
entries.

Each selected benchmark entry contains:

- `kernel_name`: exact name expected in `benchmark-comparison/comparison_ci.csv`
- `tier`: `1` for system-parameter probes or `2` for algorithm kernels
- `category`: deterministic category label for summary grouping
- `rationale`: human-readable reason for inclusion
- `coverage_expectation`: minimum coverage rule a validator should enforce
- `coverage_evidence`: evidence from the source `comparison_ci.csv`, including
  hardware points, matched points, flat/non-flat matched counts, trend-point
  availability, `coverage_status`, and `point_coverage_ready`
- Partial current-data entries additionally carry
  `partial_coverage_policy: "partial_current_data_v1"` in the expectation plus a
  non-empty `partial_coverage_reason` and `coverage_gaps` in the evidence. These
  entries are validator-accepted but are not counted as full point-coverage
  passes.

The coverage model used the validation-report definition of flat/non-flat
points:

```text
flat     := real_ms <= 2 * min(real_ms for the same kernel)
non-flat := real_ms >  2 * min(real_ms for the same kernel)
```

For Tier 2 kernels, the manifest expected at least one matched flat point when a
hardware flat region existed and at least five matched non-flat points when a
hardware non-flat region existed. For Tier 1 probes, the manifest recorded a
minimum of five matched points as the regime-coverage proxy.

## Included Historical Benchmarks

| Kernel | Tier | Category | Matched / HW Points | Matched Flat | Matched Non-flat | Historical reason for inclusion |
|---|---:|---|---:|---:|---:|---|
| `bh` | 2 | irregular_graph_tree_traversal | 7 / 18 | 1 / 3 | 6 / 15 | Irregular tree/graph behavior with enough non-flat points. |
| `busspeeddownload` | 1 | host_to_device_transfer_probe | 16 / 19 | 9 / 9 | 7 / 10 | Host-to-device transfer probe. |
| `busspeedreadback` | 1 | device_to_host_transfer_probe | 16 / 19 | 5 / 5 | 11 / 14 | Device-to-host readback probe. |
| `devicememory_write` | 1 | device_memory_write_probe | 9 / 16 | 4 / 4 | 5 / 12 | Device-memory write-throughput probe. |
| `ep` | 2 | embarrassingly_parallel_compute | 13 / 13 | 1 / 1 | 12 / 12 | Simple parallel algorithm kernel with complete coverage. |
| `fused_swiglu` | 2 | ml_fused_activation | 11 / 18 | 5 / 5 | 6 / 13 | Modern ML fused elementwise/activation behavior. |
| `global_bw_copy` | 1 | global_memory_copy_probe | 9 / 16 | 4 / 4 | 5 / 12 | Global memory copy bandwidth probe. |
| `hist` | 2 | histogram_atomic_memory | 16 / 18 | 8 / 8 | 8 / 10 | Histogram/aggregation memory behavior. |
| `hotspot` | 2 | stencil_thermal | 10 / 18 | 5 / 5 | 5 / 13 | 2D stencil/thermal solver. |
| `l1_cache_bw` | 1 | l1_cache_bandwidth_probe | 16 / 16 | 16 / 16 | 0 / 0 | L1 cache bandwidth parameter probe. |
| `l2_cache_bw` | 1 | l2_cache_bandwidth_probe | 5 / 16 | 5 / 16 | 0 / 0 | L2 cache bandwidth parameter probe. |
| `lud` | 2 | dense_linear_algebra_factorization | 7 / 18 | 1 / 1 | 6 / 17 | Dense tiled factorization with synchronization and reuse. |
| `nn` | 2 | nearest_neighbor_search | 16 / 16 | 11 / 11 | 5 / 5 | Search-style data access and branch behavior. |
| `nw` | 2 | dynamic_programming | 7 / 17 | 1 / 1 | 6 / 16 | Needleman-Wunsch dynamic programming/wavefront behavior. |
| `particlefilter` | 2 | monte_carlo_particle_tracking | 7 / 16 | 2 / 2 | 5 / 14 | Probabilistic particle-style application behavior. |
| `qtc` | 2 | scientific_compute | 9 / 16 | 2 / 2 | 7 / 14 | Scientific-computing kernel with usable non-flat trend evidence. |
| `rope` | 2 | ml_positional_embedding | 7 / 16 | 2 / 2 | 5 / 14 | ML positional-embedding/elementwise math behavior. |
| `sgemm_tiled` | 2 | tiled_dense_gemm | 8 / 20 | 3 / 3 | 5 / 17 | Dense matrix compute and memory-reuse anchor. |

## Historical Candidate-Pool Exclusion

| Kernel | Reason for excluding from the historical primary set |
|---|---|
| `gelu` | The data had only one hardware/simulation point. It was useful as an ML activation diagnostic, but one point could not support trend/scaling metrics. |

## Important Broad-Suite Kernels Not Selected Then

The following kernels remained important diagnostics, but they did not block the
historical curated primary set until coverage improved or deterministic blockers
were resolved:

| Kernel(s) | Historical reason to keep secondary |
|---|---|
| `correlation`, `covariance`, `dmr`, `stencil_kernel` | Near-miss/non-compliant kernels from then-recent milestones; useful diagnostics but missing the required non-flat coverage in the main-context report. |
| `atax`, `bicg`, `mvt`, `gaussian`, `layerforward`, `layernorm`, `md`, `softmax`, `spmv_csr`, `triad` | Representative algorithms, but matched non-flat coverage was below the broad validation rule. |
| `fp32_fma`, `fp64_fma`, `int_mad`, `sfun_sin`, `shared_bw`, `occupancy_fma`, `mem_latency_chase`, `maxflops` | Tier 1-style probes that remained valuable for root-cause analysis, but coverage was sparse, non-compliant, or not as balanced as the selected probe subset. |
| `naive_attention`, `streamcluster_pgain`, `computesad`, `findminsad`, `dwt2d`, `atomic_throughput` | No simulation coverage in the report; recovery targets rather than primary tuning gates. |

## Historical/Offline Use Only

- Treat `benchmark-comparison/mi300a_evaluation_set.json` as the historical
  source of truth for the curated tuning set, not as the live workflow
  matrix source.
- Historical curated validators may still read `comparison_ci.csv` and the
  manifest, fail on malformed entries or unknown kernel names, and fail if a
  non-partial selected benchmark drops below its recorded coverage expectation.
  Explicit `partial_current_data_v1` entries are allowed to validate only when
  their manifest counts match current CSV counts and their `coverage_gaps` match
  the current deficits.
- Historical summaries may report selected benchmark count, tier/category counts,
  `pass`/`partial` coverage status, and available accuracy/trend fields for this
  curated set.
- Broad-suite and curated-result analysis can remain useful for future tuning, but
  the current benchmark CI does not gate on this curated manifest unless a future update
  explicitly reintroduces such behavior.

## Historical Curated Accuracy Scorecard

The scorecard CLI can still turn a generated curated summary into a compact
offline coverage/accuracy/trend target check for tuning analysis:

```bash
python3 scripts/score_curated_validation.py
python3 scripts/score_curated_validation.py --format json
python3 scripts/score_curated_validation.py --enforce
```

By default the CLI reads
`benchmark-comparison/mi300a_evaluation_set_summary.json`, reports coverage
separately from Tier 1/Tier 2 accuracy and scaling/trend targets, and exits 0 in
diagnostic mode even when coverage is partial or accuracy targets fail.
`--enforce` returns a nonzero status when any curated coverage, accuracy, or trend
target fails. JSON output is deterministic and includes the source path,
aggregate target results, and ranked per-benchmark target gaps for tuning
prioritization.

The current benchmark workflow does not expose a curated scorecard gate or a
curated-only lane. If future work wants to restore such behavior, it must do so
explicitly instead of treating this historical document as a
live workflow contract.

## Historical Curated Workflow Plan Contract

The workflow-plan CLI and archived JSON
`benchmark-comparison/archive/mi300a_curated_workflow_plan.json` are retained to
make the old curated-lane selection and run parameters auditable:

```bash
python3 scripts/generate_mi300a_curated_workflow_plan.py
python3 scripts/generate_mi300a_curated_workflow_plan.py \
  --output benchmark-comparison/archive/mi300a_curated_workflow_plan.json
```

That contract was designed for the earlier benchmark workflow shape with inline
broad-suite Tier matrices. It recorded schema/version, manifest identity, source
paths, Tier-1-before-Tier-2 ordering, manifest kernel name, workflow benchmark
name, tier, category, sizes, flag templates, size-label templates, per-entry
timeouts, optional runtime fields (`extra_flags`, `gomemlimit`, `gogc`), and
explicit alias reasons for manifest/workflow name differences such as
`busspeeddownload` -> `bus_speed_download`, `busspeedreadback` ->
`bus_speed_readback`, `devicememory_write` -> `device_memory_write`, and
`sgemm_tiled` -> `parboil_sgemm`.

The generator is non-dispatching. It does not launch CI, merge artifacts, collect
a new baseline, retune simulator parameters, or promote broad-suite diagnostics
into live gates. Against the current linear regular workflow, the default
repository invocation validates and re-emits the archived curated plan; it does
not require removed legacy Tier matrices to be present in the live workflow. To
rebuild from an old branch or fixture that still has those historical matrices,
pass that file explicitly with `--workflow`.

## Current Maintained Regular Workflow

To run current MI300A benchmark CI, manually dispatch `.github/workflows/benchmark.yml`
with the default dispatch form and optionally provide `ref=<branch/tag/SHA>`.
The workflow validates the checked-in finishable-size manifest for audit
continuity, runs `scripts/validate_mi300a_regular_workflow_matrix.py` as a static
workflow/plan contract guard, materializes the maintained regular matrices
directly from the
1416-entry problem-size discovery plan
`benchmark-comparison/mi300a_problem_size_discovery_plan.json` (the checked-in
full measured problem-size plan), runs Tier 1 entries first, evaluates the
Tier-1 summary, then runs Tier 2 entries for
full regular matrix coverage and emits the final summary artifacts. The
maintained runtime matrix covers all 1416 plan entries exactly once in clean
per-benchmark rows: 19 Tier 1 rows / 308 entries and 63 Tier 2 rows / 1108
entries. Every simulator invocation is normalized to `timeout_sec: 7200`; each
benchmark-tier job remains bounded by `timeout-minutes: 360` / 21600 seconds and
`strategy.max-parallel: 14`, and the benchmark workflow keeps only the regular
`ref` dispatch input with no terminal-discovery or extra dispatch surface. These
7200-second per-simulator-invocation and 360-minute benchmark-tier job limits are
the maintained current contract; earlier 600-second, 3600-second, or 720-minute
wording in selected-run artifacts, provenance captures, or fixtures is
historical/archive context only.

This full regular matrix is not a complete finishability evidence set:
non-finishable, non-terminal, failed, or timed-out regular workflow rows remain
classification/runtime data, not terminal finishability evidence, and they do not
authorize terminal provenance collection or regeneration, finishable manifest
regeneration, Tier regeneration, local simulation/calibration, or finishability evidence
completion claims. This historical document also does not add dispatch, cancel,
rerun, terminal-discovery, `run_mode`, curated-only, or broad-suite recovery
guidance; those paths remain outside the live workflow unless a future update
reintroduces them with fresh evidence and tests.

## Updating the Historical Set

Changes to the historical curated set should update both the JSON manifest and
this rationale document in the same commit. Promotion of a secondary benchmark
should include evidence that it has enough matched points for its tier and that it
adds coverage not already represented by the set. Separately, changes to current
benchmark CI must update the regular matrix/finishable-view validation and linear
workflow documentation, not this historical curated-set contract alone.
