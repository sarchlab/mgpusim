# MI300A Calibration Report

<!--
  TEMPLATE. The `{{...}}` tokens are filled in by
  gpu_perf_scripts/calibration/compare_to_ground_truth.py, which compares the
  MGPUSim simulated results (MI300A / CDNA3 timing config) against the
  real-hardware ground-truth summary CSV. Edit this template, not the generated
  copy. Error metric is ABSOLUTE error (sim - real, GFLOPS) -- swap in
  relative/MAPE later if desired.
-->

| | |
|---|---|
| **Generated** | {{GENERATED_AT}} |
| **Commit** | `{{GIT_SHA}}` |
| **Simulator** | `{{SIM_CONFIG}}` |
| **Ground truth** | `{{REF}}` — real MI300A (host `odyssey`, ROCm 7.2.4 / HIP 7.2), mean of 7 reps/point |

## Summary

| Benchmark | Metric | Points | MAE | Max \|err\| |
|-----------|--------|-------:|----:|----------:|
{{SUMMARY_ROWS}}

> Absolute error = `sim − real` (GFLOPS). **MAE** = mean absolute error over
> matched points; **Max |err|** = largest single-point absolute error.

---

## fp32_throughput — metric: GFLOPS

Single-precision FMA throughput. Simulated GFLOPS = `num_blocks × threads_per_block × fmas_per_thread × 2 / kernel_time`.
Each point is matched against ground-truth runs with the **same** `(num_blocks, threads_per_block = 256, fmas_per_thread)`.
"sim high" = simulator reports more GFLOPS than hardware (optimistic).

- **MAE:** {{FP32_MAE}} GFLOPS · **Max |err|:** {{FP32_MAXAE}} GFLOPS ({{FP32_MAXAE_AT}}) · **Points:** {{FP32_N}}

Per-configuration execution-time curves (sim vs real) are in the `figures/` artifact
(one PNG per `num_blocks` × `threads_per_block`, x = fmas, y = execution time).

| num_blocks | threads/block | fmas/thread | Real GFLOPS | Sim GFLOPS | Abs err (GFLOPS) | Note |
|-----------:|--------------:|------------:|------------:|-----------:|-----------------:|:-----|
{{FP32_ROWS}}

---

## cache_latency — metric: ns/access

Single-thread pointer-chasing latency probe. One work-item performs
`num_accesses` dependent loads over a randomly shuffled chain filling an array
of `array_bytes`; the per-access latency is `kernel_time / num_accesses`.
Sweeping `array_bytes` across the cache hierarchy exposes L1 / L2 / DRAM latency.

**Sim-only for now** — there is no committed cache_latency ground truth yet, so
only simulated latencies are shown. It runs as its own parallel CI job (kept
separate so a failure can't block fp32 reporting); once real-hardware latencies
land in the ground-truth CSV, an error column can be added here.

- **Sim points:** {{CACHE_N}}

| Array size | Level | Accesses | Sim ns/access | Sim kernel (ms) |
|-----------:|:------|---------:|--------------:|----------------:|
{{CACHE_ROWS}}

---

## Calibration knobs

If the errors above are large, tune these in
`amd/samples/runner/timingconfig/mi300a/builder.go`:

| Knob | Current default | Primarily affects |
|------|-----------------|-------------------|
| `freq` | 1.70 GHz | all compute throughput |
| `WithNumSinglePrecisionUnits` | 16 / CU | **fp32 GFLOPS** (peak) |
| CU count (`NumCUPerShaderArray × NumShaderArray`) | 6 × 20 = 120 | aggregate throughput |
| `ConstantKernelLaunchOverhead` | 5400 cyc | short-kernel time floor |

> Note: real-HW fp32 has a ~0.5 ms fixed time floor, so at small `fmas` the
> kernel is launch-overhead-bound, not compute-bound — the GFLOPS comparison is
> most meaningful in the throughput-bound regime (large `fmas`, high occupancy).

---

_Template: `gpu_perf_scripts/calibration/report_template.md` · numbers filled by CI._
