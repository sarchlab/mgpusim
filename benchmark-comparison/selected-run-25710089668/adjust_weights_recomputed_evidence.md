# adjust_weights selected-run recomputed evidence

Inputs are read-only source files; raw comparison CSV rows are not rewritten.

## Provenance

- Comparison CSV: `benchmark-comparison/selected-run-25710089668/comparison_ci.detailed.csv` (sha256 `80031f376986ae24edf9cf18ee189ce4fe23570b98b1c9343db8c74b707167db`)
- Tier config: `scripts/benchmark_tiers.json` (sha256 `80b856d0639d2482002000ca56f6d415b31a2f716a7684c6ea968f2e1d205a8b`)
- Git-tracked input check: passed for `benchmark-comparison/selected-run-25710089668/comparison_ci.detailed.csv`, `scripts/benchmark_tiers.json`

## Summary

- Rows for kernel: **18**
- Completed rows with `sim_ms`: **12**
- `no-result` rows: **6**
- Boundary: completed through problem_size 524288; no-result starts at 1048576 and covers 6 larger planned sizes.
- Current affine model from tier config: `sim_scale=0.570694861247`, `fixed_time_ms=0.005321986901`.
- Affine source: Least-squares affine fit to 11 completed selected-run samples in benchmark-comparison/selected-run-25619929396/comparison_ci.detailed.csv; raw sim_ms values are unchanged.

| Model | sim_scale | fixed_time_ms | Mean error | Max error | Mean signed error | Mean abs error (ms) | Pearson real→sim | Spearman real→sim | Pearson size→abs error | Spearman size→abs error |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Raw | 1 | 0 | 59.29% | 92.68% | -35.28% | 0.008229 | 0.998920 | 0.830124 | -0.290790 | -0.797203 |
| Fixed-time-only | 1 | 0.005321986901 | 32.10% | 63.17% | 28.32% | 0.0078928224175 | 0.998920 | 0.830124 | 0.759394 | 0.776224 |
| Current affine | 0.570694861247 | 0.005321986901 | 7.14% | 25.80% | 0.53% | 0.000781396283791 | 0.998920 | 0.830124 | -0.278627 | -0.447552 |

## Feasibility decision

- Target for raw or fixed-time-only mean error: **<20.00%**.
- Current raw mean error: **59.29%**; configured fixed-time-only mean error: **32.10%**.
- Best non-negative fixed-time-only lower bound: **25.60%** mean error at `fixed_time_ms=0.00406` (max 59.71%).
- Raw signed-error shape: underpredicts 8 completed sizes through `32768` and overpredicts 4 completed sizes starting at `65536`.
- Positive real-to-sim trend preserved: `true`.
- Decision: `not_feasible_with_raw_or_fixed_time_only_model`. Raw adjust_weights timing underpredicts the small-size launch floor and overpredicts the large-size per-work slope; a single fixed-time offset cannot correct both regimes.
- Recommended limitation wording: Report adjust_weights as an affine report-time calibration. Raw sim_ms rows remain unchanged, and raw or fixed-time-only mean error is not below 20% on the retained selected-run evidence.

## Problem-size boundary

- Completed problem sizes: `256`, `512`, `1024`, `2048`, `4096`, `8192`, `16384`, `32768`, `65536`, `131072`, `262144`, `524288`
- No-result problem sizes: `1048576`, `1572864`, `2097152`, `2621440`, `3145728`, `4194304`
- Contiguous no-result tail: `true`

## Completed sample details

| problem_size | real_ms | raw_sim_ms | raw error | fixed-only sim_ms | fixed-only error | affine sim_ms | affine error |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 256 | 0.0076 | 0.000556 | 92.68% | 0.005877986901 | 22.66% | 0.00563929324385 | 25.80% |
| 512 | 0.0047 | 0.00064 | 86.38% | 0.005961986901 | 26.85% | 0.0056872316122 | 21.00% |
| 1024 | 0.0058 | 0.000739 | 87.26% | 0.006060986901 | 4.50% | 0.00574373040346 | 0.97% |
| 2048 | 0.0058 | 0.000927 | 84.02% | 0.006248986901 | 7.74% | 0.00585102103738 | 0.88% |
| 4096 | 0.0056 | 0.001336 | 76.14% | 0.006657986901 | 18.89% | 0.00608443523563 | 8.65% |
| 8192 | 0.0062 | 0.00216 | 65.16% | 0.007481986901 | 20.68% | 0.00655468780129 | 5.72% |
| 16384 | 0.0072 | 0.003847 | 46.57% | 0.009168986901 | 27.35% | 0.00751745003222 | 4.41% |
| 32768 | 0.0102 | 0.007224 | 29.18% | 0.012545986901 | 23.00% | 0.00944468657865 | 7.41% |
| 65536 | 0.0128 | 0.013989 | 9.29% | 0.019310986901 | 50.87% | 0.013305437315 | 3.95% |
| 131072 | 0.0208 | 0.027631 | 32.84% | 0.032952986901 | 58.43% | 0.0210908566121 | 1.40% |
| 262144 | 0.0364 | 0.054073 | 48.55% | 0.059394986901 | 63.17% | 0.0361811701332 | 0.60% |
| 524288 | 0.07 | 0.107384 | 53.41% | 0.112705986901 | 61.01% | 0.0666054838811 | 4.85% |
