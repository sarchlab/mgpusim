# M5 Benchmark Results: s_nop Fix + Overhead Tuning

**Branch:** `ares/m5-snop-and-tuning`
**Config:** `-timing -arch cdna3 -gpu mi300a -disable-rtm`
**Date:** 2026-03-06

## Changes in M5

1. **s_nop fix** — `scheduler.go`: Internal instructions (including `s_nop`) now call `UpdatePCAndSetReady()` instead of just setting state to `WfReady`. This prevents infinite loops on `s_nop` instructions.
2. **H2D/D2H tuning** — `builder.go`: `WithD2HCycles(8500→300)`, `WithH2DCycles(14500→500)`.
3. **Kernel overhead** — `dispatching/builder.go`: `constantKernelOverhead: 0→3600`.

## Summary

| Metric | Value |
|--------|-------|
| Benchmarks producing results | 13 |
| Total matched data points | 42 |
| Mean |relative error| | 120.1% |
| Median |relative error| | 55.6% |
| Within 10% error | 21.4% |
| Within 25% error | 31.0% |
| Within 50% error | 50.0% |

## Per-Benchmark Results

| Benchmark | Matched Points | Avg |Error| | Status |
|-----------|---------------|-------------|--------|
| matrixmultiplication | 5 | 15.6% | ✅ Excellent |
| nw | 4 | 5.8% | ✅ Excellent |
| spmv_csr_scalar | 2 | 9.6% | ✅ Excellent |
| matrixtranspose | 1 | 9.7% | ✅ Excellent (NEW) |
| bitonicsort | 3 | 23.2% | ✅ Good (NEW) |
| fir | 1 | 35.9% | ✅ Good |
| relu | 3 | 39.0% | ⚠️ Fair |
| vectoradd | 4 | 43.1% | ⚠️ Fair |
| floydwarshall | 2 | 67.2% | ⚠️ Over-estimated |
| kmeans | 2 | 77.8% | ⚠️ Over-estimated |
| fastwalshtransform | 4 | 111.2% | ❌ Large error (NEW) |
| nbody | 4 | 136.5% | ❌ Large error |
| stencil2d | 3 | 356.0% | ❌ Large error |
| atax | 4 | 542.7% | ❌ Very large error |

## Newly-Unlocked Benchmarks (s_nop fix)

| Benchmark | Status | Notes |
|-----------|--------|-------|
| **bitonicsort** | ✅ WORKS | Avg error 23.2% — excellent |
| **simpleconvolution** | ⚠️ PARTIAL | Works at 64x64, crashes at 128x128+ (MMU bug) |
| **matrixtranspose** | ⚠️ PARTIAL | Works at 64x64, crashes at 128x128+ (MMU bug) |
| **fastwalshtransform** | ✅ WORKS | Avg error 111.2% — sim too fast |
| **bfs** | ❌ CRASHES | MMU "page not found" panic at all tested sizes (256, 1024, 4096 nodes) |

## Critical Bug: MMU "page not found" Panic

Many benchmarks crash at moderate data sizes with:
```
panic: page not found
  at akita/v4/mem/vm/mmu/mmu.go:107 finalizePageWalk
```

This is a **pre-existing bug** (not introduced by M5 changes). It limits benchmarking to very small problem sizes for most benchmarks. The crash threshold varies by benchmark:
- vectoradd: works up to ~15000 elements, crashes at 16000
- simpleconvolution: crashes at 128x128
- matrixtranspose: crashes at 128x128
- bfs: crashes at all sizes
- floydwarshall: works at 32/64 nodes, crashes at 96+
- fir: works at 1024, crashes at 4096

## Comparison with M4

| Benchmark | M4 Avg Error | M5 Avg Error | Change |
|-----------|-------------|-------------|--------|
| matmul | 5.6% | 15.6% | Slightly worse (kernel overhead +3600) |
| nw | 5.8% | 5.8% | Same |
| stencil2d | 194.7% | 356.0% | Worse |
| nbody | 210.0% | 136.5% | Improved |
| floydwarshall | 442.4% | 67.2% | **Much improved** |
| relu | 235.6% | 39.0% | **Much improved** |
| vectoradd | 242.7% | 43.1% | **Much improved** |
| fir | 166.7% | 35.9% | **Much improved** |

Note: M4 was run on different (larger) data sizes. The M5 results are limited to small sizes due to the MMU bug, so these comparisons are approximate.

## Files

- `gpu_perf_scripts/sim_results_m5.csv` — Raw simulation results (44 data points)
- `gpu_perf_scripts/comparison_m5_detailed.csv` — Detailed comparison with reference
- `docs/mi300a_m5_benchmark_results.md` — This report
