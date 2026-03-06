# MI300A DRAM Fix Benchmark Results

**Branch:** `ares/mi300a-timing-fixes`  
**Commit:** `86383354` — [Finn] Fix SimpleBankedMemory DRAM parameters  
**Date:** 2026-03-05  
**Hardware reference:** `gpu_perf_scripts/mi300a_120cu.csv` (real MI300A, 120 CU)  
**Flags:** `-timing -arch cdna3 -gpu mi300a -disable-rtm`

## Summary

After Finn's DRAM parameter fix (BankPipelineDepth 1→20, StageLatency 100→5, TopPortBufferSize 64, PostPipelineBufferSize 4), benchmarks were run at multiple problem sizes and compared against real MI300A hardware measurements.

## Results

### Matrix Multiplication

| Problem Size | Sim kernel_time (s) | Sim (ms) | Real HW (ms) | Error (%) | Notes |
|---|---|---|---|---|---|
| 128×128×128 | 3.4351e-05 | 0.0344 | 0.0243 | +41.4% | Sim ~1.4x slower |
| **256×256×256** | **8.8547e-05** | **0.0886** | **0.0403** | **+119.7%** | Sim ~2.2x slower |

### Stencil2D

| Problem Size | Sim kernel_time (s) | Sim (ms) | Real HW (ms) | Error (%) | Notes |
|---|---|---|---|---|---|
| 64×64 | 4.2922e-05 | 0.0429 | 0.0059 | +627.5% | Sim ~7.3x slower |
| 128×128 | 4.3525e-05 | 0.0435 | 0.0062 | +601.7% | Sim ~7.0x slower |
| 256×256 | 4.4484e-05 | 0.0445 | 0.0065 | +584.4% | Sim ~6.8x slower |
| **512×512** | **4.5281e-05** | **0.0453** | **0.0063** | **+619.0%** | Sim ~7.2x slower |
| 1024×1024 | CRASH | — | 0.0111 | — | MMU page fault |

### Floyd-Warshall

| Problem Size | Sim kernel_time (s) | Sim (ms) | Real HW (ms) | Error (%) | Notes |
|---|---|---|---|---|---|
| 32 nodes | 8.2675e-05 | 0.0827 | 0.1563 | −47.1% | Sim ~1.9x faster |
| **64 nodes** | **1.67834e-04** | **0.1678** | **0.3024** | **−44.5%** | Sim ~1.8x faster |
| 128 nodes | CRASH | — | 0.6016 | — | MMU page fault |

### Vector Addition

| Problem Size | Sim kernel_time (s) | Sim (ms) | Real HW (ms) | Error (%) | Notes |
|---|---|---|---|---|---|
| 1,024 | 3.904e-06 | 0.0039 | 0.0043 | −9.3% | ✅ Close match |
| 4,096 | 3.963e-06 | 0.0040 | 0.0050 | −20.7% | Sim ~1.3x faster |
| 8,192 | 4.023e-06 | 0.0040 | 0.0056 | −28.1% | Sim ~1.4x faster |
| 16,384 | 2.398e-06 | 0.0024 | 0.0056 | −57.2% | Sim ~2.3x faster |
| 32,768 | CRASH | — | 0.0066 | — | MMU page fault |
| 65,536 | CRASH | — | 0.0048 | — | MMU page fault |
| **1,048,576** | **CRASH** | **—** | **0.0069** | **—** | MMU page fault |
| **16,777,216** | **CRASH** | **—** | **0.0516** | **—** | MMU page fault |

## Analysis

### Accuracy by Benchmark

| Benchmark | Best Error | Worst Error | Direction | Assessment |
|---|---|---|---|---|
| Matrix Multiplication | +41% (128³) | +120% (256³) | Sim too slow | Sim overpredicts by 1.4–2.2× |
| Stencil2D | +584% (256²) | +628% (64²) | Sim too slow | Sim overpredicts by ~7× |
| Floyd-Warshall | −44% (64n) | −47% (32n) | Sim too fast | Sim underpredicts by ~1.8× |
| Vector Addition | −9% (1k) | −57% (16k) | Sim too fast | Close at small sizes, diverges at larger |

### Key Observations

1. **DRAM fix improved sim realism** — previous benchmarks showed even larger discrepancies. The DRAM parameter changes (pipeline depth, stage latency, buffer sizes) moved timing closer to hardware.

2. **Stencil2D remains the largest outlier** (~7× slower than HW). This benchmark is memory-bandwidth-bound, suggesting the memory subsystem model may still be too conservative or the kernel dispatch overhead is too high relative to the small kernel execution time.

3. **Floyd-Warshall sim time scales correctly** — doubling nodes from 32→64 roughly doubles sim time (0.083ms → 0.168ms), matching the quadratic scaling seen in HW (0.156ms → 0.302ms). However, the absolute values are ~1.8× too fast.

4. **Vector addition close at small sizes** — at 1024 elements, error is only 9.3%. This suggests base kernel launch overhead is well-modeled, but memory transfer time for larger buffers is underpredicted.

5. **MMU page faults at larger sizes** — stencil2d ≥1024×1024, floydwarshall ≥128 nodes, vectoradd ≥32K elements all crash with `page not found` panics. This is a known pre-existing issue in the MMU/page table walker, not related to the DRAM fix.

### Comparison with Previous Results (Pre-DRAM Fix)

From issue #243 verification benchmarks (same branch, before Finn's DRAM fix commit):

| Benchmark | Pre-Fix Sim (ms) | Post-Fix Sim (ms) | Change |
|---|---|---|---|
| matmul 128³ | 0.0451 | 0.0344 | −23.8% (improved) |
| stencil2d 128×128 | 0.0445 | 0.0435 | −2.1% (slight improvement) |
| floydwarshall 32 nodes | 0.0826 | 0.0827 | ~0% (unchanged) |

The DRAM fix primarily affected compute-heavy benchmarks (matmul improved 24%) while memory-bandwidth-bound workloads (stencil2d) saw smaller improvements.

## Remaining Issues

1. **MMU page fault crashes** limit benchmarking to small problem sizes — the requested 1M/16M vectoradd sizes cannot be run.
2. **Stencil2D ~7× error** needs investigation — likely related to memory subsystem modeling or kernel dispatch overhead.
3. **No benchmarks at "meaningful" large sizes** possible due to MMU crashes — the largest successful runs are 256×256×256 (matmul), 512×512 (stencil2d), 64 nodes (floyd), and 16K elements (vectoradd).
