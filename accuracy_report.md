# Benchmark Accuracy Report

Comparison of MGPUSim simulated execution time against MI300A hardware measurements.

Historical report source data: `benchmark-comparison/selected-run-25619929396/comparison_ci.detailed.csv` (only rows where both `real_ms` and `sim_ms` are populated). The accuracy metrics and the per-size finishability tables in this report remain historical selected-run `25619929396` report outputs; they are not regenerated from current-main terminal run `25710089668`.

## Historical Selected CI Run Provenance (report tables)

- Run ID: `25619929396`
- Head SHA: `a281789a7354b86a8a04b350c145c573d531f54d`
- Head branch: `main`
- Workflow: `MI300A Benchmark`
- Run status/conclusion: `completed` / `failure`
- Run URL: https://github.com/sarchlab/mgpusim-dev/actions/runs/25619929396
- Run metadata: `benchmark-comparison/selected-run-25619929396/run-view.json`
- Scope in this document: source for the historical accuracy tables and historical selected-run per-size finishability tables below.

## Current-Main Terminal Artifacts-Only Evidence (not report-table source)

Run `25710089668` is retained separately as current-main terminal provenance for the maintained regular MI300A workflow. It does **not** replace the historical selected-run `25619929396` data used by the accuracy metrics and finishability tables in this report.

- Run ID: `25710089668`
- Head SHA: `6fdbbd1882f6a36c5e0846b9209a9a19c258b486`
- Head branch: `main`
- Workflow/event: `MI300A Benchmark` / `workflow_dispatch`
- Run status/conclusion: `completed` / `failure`; the final Summary job succeeded and uploaded report artifacts.
- Run URL: https://github.com/sarchlab/mgpusim-dev/actions/runs/25710089668
- Evidence location: `benchmark-comparison/selected-run-25710089668/` plus the matching `benchmark-comparison/provenance/mi300a_regular_benchmark_run_25710089668_*` summaries.
- Inventory wording: `jobs.json` has 85 jobs, `artifacts.tsv` has 87 artifact inventory rows, and `raw-zip-entry-counts.tsv` has 340 raw ZIP file entries. The checked-in `selected-run-25710089668` directory is a curated retained subset of selected metadata, manifests, reports, summary CSVs, and finishability evidence; it is not a full extraction of every raw ZIP entry.
- Artifacts-only finishability evidence: `mi300a_regular_finishability_evidence.json` / `.md` derive status from checked-in selected-run metadata, the regular matrix/plan, and populated simulator timing rows only. Missing planned rows are classified as `no-result`, not inferred as timeout or crash. Summary counts are `pass=623`, `no-result=793`, `fail=0`, `timeout=0`, `cancelled=0` across the 1416-entry regular plan.
- Maintained workflow contract preserved by the validation artifacts: 7200s per simulator invocation, `timeout-minutes: 360` / 21600s per benchmark-tier job, `strategy.max-parallel: 14`, and Tier 1 before Tier 2 in the linear regular workflow.

### Finishability note for run 25710089668

Run `25710089668` is the terminal current-main run with conclusion `completed` / `failure`; it is not a clean or passing CI run. Of the 82 benchmark jobs, 76 failed and 2 succeeded (4 were cancelled). Of the 1416 plan entries across the regular benchmark matrix, 623 produced valid timing rows (`pass`) and 793 are `no-result` — meaning those benchmark/size combinations have no completed per-size artifact row in this run. Zero entries are classified as `fail`, `timeout`, or per-size `cancelled`, because the evidence is derived from artifacts only: missing rows are not relabeled from job-level failure. This evidence identifies the subset of benchmark/size combinations that produced artifact rows under the maintained workflow contract; it does not prove complete terminal finishability for the full matrix.

Error definitions:

- `err = |sim - hw| / hw`
- `symmetric_err = |sim - hw| / min(sim, hw)`
- `sim` is raw `sim_ms` unless `scripts/benchmark_tiers.json` configures a report-time calibration.
- Calibrated metrics use the matching report-time affine model `sim = sim_scale * sim_ms + fixed_time_ms`; raw comparison rows remain unchanged.

## Report-Time Simulator Calibration

The comparison CSV keeps raw `sim_ms` values. Report metrics and calibrated chart lines apply these affine models without rewriting or filtering source rows. For region-aware rows, the problem-size region selects which affine model is applied:

| Benchmark | Problem-size region | sim_scale | fixed_time_ms | Source |
| --- | --- | ---: | ---: | --- |
| adjust_weights | all sizes | 0.570694861247 | 0.005321986901 | Least-squares affine fit to 11 completed selected-run samples in benchmark-comparison/selected-run-25619929396/comparison_ci.detailed.csv; raw sim_ms values are unchanged. |
| bfs | all sizes | 133.490552877 | 0 | Scale-only least-squares fit to 12 completed BFS samples in benchmark-comparison/selected-run-25619929396/comparison_ci.detailed.csv from selected run 25619929396 (head a281789a7354b86a8a04b350c145c573d531f54d); raw sim_ms values and selected-run identity are unchanged. |
| branch_div_50pct | all sizes | 1 | 0.063 | per_benchmark_fixed_time_ms |
| maxflops | all sizes | 1 | 0.01 | per_benchmark_fixed_time_ms |
| mem_latency_chase | small working set (≤8192) | 6.45656206398 | -0.0353490521877 | Least-squares affine fit to 6 completed selected-run samples with problem_size 256-8192 in benchmark-comparison/selected-run-25619929396/comparison_ci.detailed.csv; raw sim_ms values are unchanged. |
| mem_latency_chase | middle working set (16384–1048576) | 0.393774513128 | 0.106965442761 | Least-squares affine fit to 7 completed selected-run samples with problem_size 16384-1048576 in benchmark-comparison/selected-run-25619929396/comparison_ci.detailed.csv; raw sim_ms values are unchanged. |
| mem_latency_chase | large DRAM-region working set (≥2097152) | 0.0936678821903 | 0.301016858446 | Least-squares affine fit to 3 completed selected-run samples with problem_size >=2097152 in benchmark-comparison/selected-run-25619929396/comparison_ci.detailed.csv; raw sim_ms values are unchanged. |
| occupancy_fma | all sizes | 1 | 0.027 | per_benchmark_fixed_time_ms |
| reduction | all sizes | 1 | 0.0065 | per_benchmark_fixed_time_ms |

## Cache Bandwidth Report Semantics

L1/L2 cache-bandwidth rows have two possible report meanings. Archived selected-run fixed-work cache-residency rows are diagnostic-only only when their provenance gate passes. Current work-scaling rows mark `num_elements` as the work amount and define total read bytes; they are scored as value-changing evidence only after loaded rows prove non-flat hardware behavior. Current L1 metadata also distinguishes HIP/CUDA rows with fixed `working_set_size` from Go simulator rows whose per-256-thread read groups remain L1-resident while total input allocation grows with `num_elements`.

| Benchmark | Report class | Work scaled? | Axis | Total read bytes | Implementation footprint | Value-change/provenance proof | Metric policy | Source |
| --- | --- | ---: | --- | --- | --- | --- | --- | --- |
| l1_cache_bw | archived fixed-work cache-residency sweep | no | working_set_size | — | archived fixed-work cache-residency footprint | archived provenance passed | diagnostic only under archived selected-run provenance gate | benchmark-comparison/selected-run-25619929396/comparison_ci.detailed.csv; run 25619929396 |
| l2_cache_bw | archived fixed-work cache-residency sweep | no | working_set_size | — | archived fixed-work cache-residency footprint | archived provenance passed | diagnostic only under archived selected-run provenance gate | benchmark-comparison/selected-run-25619929396/comparison_ci.detailed.csv; run 25619929396 |

## Tier-2 Per-Size Finishability

Problem-size completion under the CI workflow's 7200s per-simulation invocation contract.

Historical selected-run table source: selected terminal run 25619929396 (head a281789a7354b86a8a04b350c145c573d531f54d; status `completed` / conclusion `failure`) using `benchmark-comparison/selected-run-25619929396/comparison_ci.detailed.csv` extracted from selected run 25619929396 comparison-detailed artifact. A size is counted as within the 7200s per-simulation invocation contract when its selected comparison row has a populated `sim_ms`; blank `sim_ms` rows are counted as `no-result` for this run. Current-main run `25710089668` provides separate terminal artifacts-only finishability/provenance evidence summarized above; it is not the source for this historical table.

| Benchmark | Sizes within 7200s | Total sizes | Finishable size range | Missing/no-result sizes |
| --- | ---: | ---: | --- | ---: |
| hist | 16 | 18 | 512–16777216 | 2 |
| sssp | 14 | 18 | 1024–1572864 | 4 |
| bh | 13 | 18 | 256–393216 | 5 |
| bfs | 12 | 18 | 1024–786432 | 6 |
| adjust_weights | 11 | 18 | 256–262144 | 7 |
| bs | 11 | 16 | 512–524288 | 5 |
| ep | 11 | 13 | 512–524288 | 2 |
| huffman | 11 | 16 | 1024–1048576 | 5 |
| nn | 11 | 16 | 1024–1048576 | 5 |
| spmv_csr | 11 | 16 | 1024–1048576 | 5 |
| hotspot | 10 | 19 | 32–640 | 9 |
| lbm_stream_collide | 10 | 17 | 16–52 | 7 |
| qtc | 10 | 16 | 64–2048 | 6 |
| scan | 10 | 13 | 1024–524288 | 3 |
| dmr | 9 | 15 | 256–65536 | 6 |
| gridding_kernel | 9 | 17 | 256–65536 | 8 |
| md | 9 | 16 | 128–12288 | 7 |
| md5hash | 9 | 16 | 512–131072 | 7 |
| naive_attention | 9 | 17 | B4_S32_H12_D64_blk256–B4_S640_H12_D64_blk256 | 8 |
| s3d | 9 | 16 | 512–131072 | 7 |
| stencil_kernel | 9 | 19 | 16–80 | 10 |
| 2dconvolution | 8 | 18 | 64x64–768x768 | 10 |
| computeq | 8 | 17 | 256–32768 | 9 |
| gemm | 8 | 22 | 32–384 | 14 |
| atax | 7 | 18 | 64x64–640x640 | 11 |
| bicg | 7 | 18 | 64x64–640x640 | 11 |
| fused_swiglu | 7 | 18 | n49152_bs256_ept1_p1–n3145728_bs256_ept1_p1 | 11 |
| ga | 7 | 16 | 32–2048 | 9 |
| kmeans | 7 | 16 | 1024–65536 | 9 |
| lud | 7 | 18 | 64–640 | 11 |
| mvt | 7 | 18 | 64–640 | 11 |
| nw | 7 | 17 | 64–1024 | 10 |
| particlefilter | 7 | 16 | 256–16384 | 9 |
| pathfinder | 7 | 16 | 1024–65536 | 9 |
| sgemm_tiled | 7 | 20 | 64–448 | 13 |
| streamcluster_pgain | 7 | 16 | 1024–65536 | 9 |
| fdtd2d | 6 | 18 | 64–512 | 12 |
| gesummv | 6 | 18 | 64–512 | 12 |
| layerforward | 6 | 18 | 256–8192 | 12 |
| rope | 6 | 16 | B4_S64_H32_D128_blk256–B4_S768_H32_D128_blk256 | 10 |
| softmax | 6 | 18 | rows64_cols1024_bs256–rows2048_cols1024_bs256 | 12 |
| correlation | 5 | 18 | 64x64–384x384 | 13 |
| covariance | 5 | 18 | 64x64–384x384 | 13 |
| gaussian | 5 | 14 | 32–128 | 9 |
| histogram | 5 | 17 | 1024–16384 | 12 |
| hotspot3d | 5 | 18 | 8–48 | 13 |
| layernorm | 5 | 18 | rows64_hid4096_bs256_layernorm–rows1024_hid4096_bs256_layernorm | 13 |
| srad | 5 | 16 | 64–384 | 11 |
| tiled_gemm_16 | 5 | 18 | M1_N768_K768_tile16–M128_N768_K768_tile16 | 13 |
| 2mm | 4 | 18 | 64, 128, 192, 256 | 14 |
| syrk | 4 | 18 | 64, 128, 192, 256 | 14 |
| 3dconvolution | 3 | 15 | 32x32x32, 48x48x48, 64x64x64 | 12 |
| 3mm | 3 | 18 | 64, 128, 192 | 15 |
| syr2k | 3 | 18 | 64, 128, 192 | 15 |
| tpacf_dd | 3 | 18 | 256, 512, 1024 | 15 |
| computesad | 2 | 17 | 32, 48 | 15 |
| cutoff_potential | 2 | 17 | 256, 512 | 15 |
| pagerank | 2 | 16 | 128, 256 | 14 |
| gramschmidt | 1 | 10 | 64 | 9 |
| lavamd | 1 | 19 | 2 | 18 |
| dwt2d | 0 | 18 | none | 18 |
| findminsad | 0 | 17 | none | 17 |
| gelu | 0 | 1 | none | 1 |
| sort | 0 | 15 | none | 15 |
| tpacf_dr | 0 | 18 | none | 18 |
| tpacf_rr | 0 | 18 | none | 18 |

## Tier-2 Accuracy (Global Summary)

Tier-1 calibration kernels are excluded from this rollup; see the Calibration Set section below.

- Matched tier-2 samples: **430**
- Tier-2 benchmarks compared: **60**
- Mean err: **75.99%**
- Max err: **2846.21%**
- Mean symmetric err: **5912.09%**
- Max symmetric err: **388779.75%**

## Calibration Set (Tier 1)

Per-benchmark errors for the tier-1 calibration set.  Reported for transparency and **not** included in the global accuracy rollup.

| Benchmark | Samples | Mean err | Max err | Mean sym err | Max sym err | Chart |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| branch_div_50pct | 10 | 33.40% | 82.03% | 104.77% | 456.62% | [branch_div_50pct_comparison.png](docs/figures/branch_div_50pct_comparison.png) |
| busspeeddownload | 16 | 58.09% | 97.97% | 1233.69% | 4817.13% | [busspeeddownload_comparison.png](docs/figures/busspeeddownload_comparison.png) |
| busspeedreadback | 16 | 60.66% | 98.17% | 1235.85% | 5369.61% | [busspeedreadback_comparison.png](docs/figures/busspeedreadback_comparison.png) |
| devicememory_read | 3 | 5780.11% | 10941.87% | 5783.72% | 10941.87% | [devicememory_read_comparison.png](docs/figures/devicememory_read_comparison.png) |
| devicememory_write | 9 | 81.24% | 163.17% | 96.22% | 173.83% | [devicememory_write_comparison.png](docs/figures/devicememory_write_comparison.png) |
| global_bw_copy | 8 | 206.92% | 402.74% | 207.56% | 402.74% | [global_bw_copy_comparison.png](docs/figures/global_bw_copy_comparison.png) |
| l1_cache_bw | 16 | 93.96% | 94.67% | 1558.22% | 1774.49% | [l1_cache_bw_comparison.png](docs/figures/l1_cache_bw_comparison.png) |
| l2_cache_bw | 5 | 89.47% | 90.10% | 851.19% | 910.55% | [l2_cache_bw_comparison.png](docs/figures/l2_cache_bw_comparison.png) |
| maxflops | 11 | 16.02% | 23.10% | 16.45% | 23.10% | [maxflops_comparison.png](docs/figures/maxflops_comparison.png) |
| mem_latency_chase | 16 | 1.30% | 3.62% | 1.31% | 3.62% | [mem_latency_chase_comparison.png](docs/figures/mem_latency_chase_comparison.png) |
| occupancy_fma | 9 | 4.40% | 12.71% | 4.79% | 14.56% | [occupancy_fma_comparison.png](docs/figures/occupancy_fma_comparison.png) |
| reduction | 14 | 16.06% | 44.14% | 19.47% | 79.00% | [reduction_comparison.png](docs/figures/reduction_comparison.png) |
| shared_bw | 2 | 24.68% | 38.11% | 25.40% | 38.11% | [shared_bw_comparison.png](docs/figures/shared_bw_comparison.png) |
| triad | 14 | 148.01% | 455.93% | 373.02% | 645.76% | [triad_comparison.png](docs/figures/triad_comparison.png) |

## Tier-1 Per-Size Finishability

Problem-size completion under the CI workflow's 7200s per-simulation invocation contract.

Historical selected-run table source: selected terminal run 25619929396 (head a281789a7354b86a8a04b350c145c573d531f54d; status `completed` / conclusion `failure`) using `benchmark-comparison/selected-run-25619929396/comparison_ci.detailed.csv` extracted from selected run 25619929396 comparison-detailed artifact. A size is counted as within the 7200s per-simulation invocation contract when its selected comparison row has a populated `sim_ms`; blank `sim_ms` rows are counted as `no-result` for this run. Current-main run `25710089668` provides separate terminal artifacts-only finishability/provenance evidence summarized above; it is not the source for this historical table.

| Benchmark | Sizes within 7200s | Total sizes | Finishable size range | Missing/no-result sizes |
| --- | ---: | ---: | --- | ---: |
| busspeeddownload | 16 | 19 | 1KB–32768KB | 3 |
| busspeedreadback | 16 | 19 | 1KB–32768KB | 3 |
| l1_cache_bw | 16 | 16 | 1024–65536 | 0 |
| mem_latency_chase | 16 | 16 | 256–8388608 | 0 |
| reduction | 14 | 18 | 512–4194304 | 4 |
| triad | 14 | 18 | 1024–8388608 | 4 |
| maxflops | 11 | 16 | 512–524288 | 5 |
| branch_div_50pct | 10 | 16 | 1024–524288 | 6 |
| devicememory_write | 9 | 16 | 1MB–48MB | 7 |
| occupancy_fma | 9 | 16 | 1024–262144 | 7 |
| global_bw_copy | 8 | 16 | 1048576–33554432 | 8 |
| l2_cache_bw | 5 | 16 | 32768–262144 | 11 |
| devicememory_read | 3 | 16 | 1MB, 2MB, 4MB | 13 |
| shared_bw | 2 | 16 | 1048576, 2097152 | 14 |
| atomic_throughput | 0 | 11 | none | 11 |
| fp32_fma | 0 | 16 | none | 16 |
| fp64_fma | 0 | 16 | none | 16 |
| int_mad | 0 | 16 | none | 16 |
| sfun_sin | 0 | 15 | none | 15 |

## Per-Benchmark Metrics (Tier 2)

| Benchmark | Samples | Mean err | Max err | Mean sym err | Max sym err | Chart |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| 2dconvolution | 8 | 56.04% | 85.09% | 257.20% | 570.55% | [2dconvolution_comparison.png](docs/figures/2dconvolution_comparison.png) |
| 2mm | 4 | 59.23% | 68.00% | 151.70% | 212.46% | [2mm_comparison.png](docs/figures/2mm_comparison.png) |
| 3dconvolution | 3 | 62.00% | 79.62% | 212.97% | 390.61% | [3dconvolution_comparison.png](docs/figures/3dconvolution_comparison.png) |
| 3mm | 3 | 70.16% | 72.60% | 236.78% | 264.91% | [3mm_comparison.png](docs/figures/3mm_comparison.png) |
| adjust_weights | 11 | 7.34% | 25.80% | 8.21% | 34.77% | [adjust_weights_comparison.png](docs/figures/adjust_weights_comparison.png) |
| atax | 7 | 46.59% | 69.40% | 98.18% | 226.79% | [atax_comparison.png](docs/figures/atax_comparison.png) |
| bfs | 12 | 15.89% | 34.88% | 19.76% | 53.56% | [bfs_comparison.png](docs/figures/bfs_comparison.png) |
| bh | 13 | 98.91% | 99.97% | 103849.38% | 388779.75% | [bh_comparison.png](docs/figures/bh_comparison.png) |
| bicg | 7 | 37.96% | 61.07% | 67.74% | 156.84% | [bicg_comparison.png](docs/figures/bicg_comparison.png) |
| bs | 11 | 316.72% | 2846.21% | 473.91% | 2846.21% | [bs_comparison.png](docs/figures/bs_comparison.png) |
| computeq | 8 | 62.96% | 69.07% | 175.15% | 223.34% | [computeq_comparison.png](docs/figures/computeq_comparison.png) |
| computesad | 2 | 7.50% | 8.80% | 8.13% | 9.65% | [computesad_comparison.png](docs/figures/computesad_comparison.png) |
| correlation | 5 | 83.64% | 84.47% | 513.39% | 544.07% | [correlation_comparison.png](docs/figures/correlation_comparison.png) |
| covariance | 5 | 86.24% | 86.75% | 628.57% | 654.47% | [covariance_comparison.png](docs/figures/covariance_comparison.png) |
| cutoff_potential | 2 | 31.94% | 34.92% | 47.21% | 53.67% | [cutoff_potential_comparison.png](docs/figures/cutoff_potential_comparison.png) |
| dmr | 9 | 99.18% | 99.66% | 16480.07% | 29314.19% | [dmr_comparison.png](docs/figures/dmr_comparison.png) |
| ep | 11 | 76.96% | 81.67% | 353.69% | 445.58% | [ep_comparison.png](docs/figures/ep_comparison.png) |
| fdtd2d | 6 | 84.54% | 86.23% | 549.17% | 626.29% | [fdtd2d_comparison.png](docs/figures/fdtd2d_comparison.png) |
| fused_swiglu | 7 | 183.62% | 635.23% | 223.82% | 635.23% | [fused_swiglu_comparison.png](docs/figures/fused_swiglu_comparison.png) |
| ga | 7 | 66.32% | 77.97% | 228.14% | 353.98% | [ga_comparison.png](docs/figures/ga_comparison.png) |
| gaussian | 5 | 91.18% | 92.82% | 1048.04% | 1291.99% | [gaussian_comparison.png](docs/figures/gaussian_comparison.png) |
| gemm | 8 | 63.38% | 76.31% | 195.48% | 322.18% | [gemm_comparison.png](docs/figures/gemm_comparison.png) |
| gesummv | 6 | 22.89% | 79.04% | 23.79% | 79.04% | [gesummv_comparison.png](docs/figures/gesummv_comparison.png) |
| gramschmidt | 1 | 94.50% | 94.50% | 1719.10% | 1719.10% | [gramschmidt_comparison.png](docs/figures/gramschmidt_comparison.png) |
| gridding_kernel | 9 | 40.72% | 74.32% | 90.95% | 289.41% | [gridding_kernel_comparison.png](docs/figures/gridding_kernel_comparison.png) |
| hist | 16 | 73.02% | 179.15% | 240.13% | 507.52% | [hist_comparison.png](docs/figures/hist_comparison.png) |
| histogram | 5 | 81.13% | 87.68% | 461.20% | 711.37% | [histogram_comparison.png](docs/figures/histogram_comparison.png) |
| hotspot | 10 | 93.83% | 96.70% | 1701.72% | 2928.89% | [hotspot_comparison.png](docs/figures/hotspot_comparison.png) |
| hotspot3d | 5 | 75.29% | 82.61% | 319.24% | 474.88% | [hotspot3d_comparison.png](docs/figures/hotspot3d_comparison.png) |
| huffman | 11 | 97.03% | 98.45% | 4056.93% | 6332.83% | [huffman_comparison.png](docs/figures/huffman_comparison.png) |
| kmeans | 7 | 99.07% | 99.59% | 12454.96% | 24170.48% | [kmeans_comparison.png](docs/figures/kmeans_comparison.png) |
| lavamd | 1 | 41.50% | 41.50% | 70.93% | 70.93% | [lavamd_comparison.png](docs/figures/lavamd_comparison.png) |
| layerforward | 6 | 31.09% | 59.34% | 61.68% | 145.93% | [layerforward_comparison.png](docs/figures/layerforward_comparison.png) |
| layernorm | 5 | 76.34% | 191.44% | 87.93% | 191.44% | [layernorm_comparison.png](docs/figures/layernorm_comparison.png) |
| lbm_stream_collide | 10 | 92.00% | 95.13% | 1338.81% | 1955.05% | [lbm_stream_collide_comparison.png](docs/figures/lbm_stream_collide_comparison.png) |
| lud | 7 | 54.23% | 56.57% | 118.75% | 130.28% | [lud_comparison.png](docs/figures/lud_comparison.png) |
| md | 9 | 67.60% | 93.24% | 556.29% | 1379.86% | [md_comparison.png](docs/figures/md_comparison.png) |
| md5hash | 9 | 36.53% | 53.26% | 56.47% | 73.53% | [md5hash_comparison.png](docs/figures/md5hash_comparison.png) |
| mvt | 7 | 32.09% | 57.43% | 54.57% | 134.92% | [mvt_comparison.png](docs/figures/mvt_comparison.png) |
| naive_attention | 9 | 55.54% | 87.11% | 238.64% | 675.83% | [naive_attention_comparison.png](docs/figures/naive_attention_comparison.png) |
| nn | 11 | 82.36% | 196.39% | 378.35% | 736.01% | [nn_comparison.png](docs/figures/nn_comparison.png) |
| nw | 7 | 65.58% | 69.76% | 194.54% | 230.71% | [nw_comparison.png](docs/figures/nw_comparison.png) |
| pagerank | 2 | 58.09% | 98.12% | 2614.90% | 5211.74% | [pagerank_comparison.png](docs/figures/pagerank_comparison.png) |
| particlefilter | 7 | 81.85% | 88.32% | 475.73% | 755.85% | [particlefilter_comparison.png](docs/figures/particlefilter_comparison.png) |
| pathfinder | 7 | 86.00% | 93.81% | 728.01% | 1515.82% | [pathfinder_comparison.png](docs/figures/pathfinder_comparison.png) |
| qtc | 10 | 54.04% | 58.22% | 119.14% | 139.37% | [qtc_comparison.png](docs/figures/qtc_comparison.png) |
| rope | 6 | 113.36% | 291.87% | 114.04% | 291.87% | [rope_comparison.png](docs/figures/rope_comparison.png) |
| s3d | 9 | 97.87% | 138.18% | 97.87% | 138.18% | [s3d_comparison.png](docs/figures/s3d_comparison.png) |
| scan | 10 | 79.67% | 82.64% | 405.21% | 476.09% | [scan_comparison.png](docs/figures/scan_comparison.png) |
| sgemm_tiled | 7 | 37.76% | 63.25% | 72.42% | 172.14% | [sgemm_tiled_comparison.png](docs/figures/sgemm_tiled_comparison.png) |
| softmax | 6 | 47.12% | 61.03% | 79.92% | 150.82% | [softmax_comparison.png](docs/figures/softmax_comparison.png) |
| spmv_csr | 11 | 78.31% | 87.03% | 435.76% | 671.25% | [spmv_csr_comparison.png](docs/figures/spmv_csr_comparison.png) |
| srad | 5 | 94.01% | 95.72% | 1773.71% | 2238.12% | [srad_comparison.png](docs/figures/srad_comparison.png) |
| sssp | 14 | 99.80% | 99.87% | 55365.09% | 78089.35% | [sssp_comparison.png](docs/figures/sssp_comparison.png) |
| stencil_kernel | 9 | 86.85% | 91.77% | 708.85% | 1114.51% | [stencil_kernel_comparison.png](docs/figures/stencil_kernel_comparison.png) |
| streamcluster_pgain | 7 | 47.88% | 73.91% | 119.32% | 283.29% | [streamcluster_pgain_comparison.png](docs/figures/streamcluster_pgain_comparison.png) |
| syr2k | 3 | 73.00% | 86.29% | 349.48% | 629.41% | [syr2k_comparison.png](docs/figures/syr2k_comparison.png) |
| syrk | 4 | 75.91% | 90.75% | 467.02% | 981.26% | [syrk_comparison.png](docs/figures/syrk_comparison.png) |
| tiled_gemm_16 | 5 | 29.22% | 30.07% | 41.32% | 43.01% | [tiled_gemm_16_comparison.png](docs/figures/tiled_gemm_16_comparison.png) |
| tpacf_dd | 3 | 45.04% | 55.29% | 87.87% | 123.68% | [tpacf_dd_comparison.png](docs/figures/tpacf_dd_comparison.png) |

## Known Limitations

### adjust_weights (backprop benchmark)

`adjust_weights` is a short backprop sub-kernel where fixed launch or dispatch overhead can dominate total runtime. The retained current-main recompute reports 59.29% raw mean error and 32.10% configured fixed-time-only mean error; the best non-negative fixed-time-only lower bound is still 25.60% because raw timing underpredicts through problem size 32768 and overpredicts from 65536. Historical calibration percentages for this kernel are run-specific and are not preserved across report regeneration. Treat the affine line as report-time calibration only: raw `sim_ms` rows are unchanged, and raw/fixed-time-only accuracy is not resolved by the calibrated report line. Use the generated per-benchmark metrics table above for the selected run's current sample count and error metrics.

### mem_latency_chase (large DRAM-region calibration scope)

`mem_latency_chase` uses the region-aware report-time calibration shown above. The comparison CSV and selected-run provenance keep the raw simulator `sim_ms` rows unchanged, so this report-time adjustment should not be interpreted as a simulator DRAM/TLB timing-model fix or as extrapolation evidence outside the completed selected-run sizes. Use the generated calibration table and per-benchmark metrics above for the current selected-run sample count and error metrics.
