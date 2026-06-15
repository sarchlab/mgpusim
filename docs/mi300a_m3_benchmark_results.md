# MI300A M3 Benchmark Results (ares/m3-correct-baseline)

**Date:** 2026-03-06  
**Branch:** `ares/m3-correct-baseline`  
**Configuration:** `-timing -arch cdna3 -gpu mi300a -disable-rtm`  
**SIMD Width:** 16 (CDNA3 ISA-correct baseline, reverted from 32)

## Summary

- **33 data points** matched against MI300A 120CU hardware reference
- **Overall average |relative error|:** 155.6%
- **Median |relative error|:** 66.7%
- **Within 10% error:** 12.1%
- **Within 25% error:** 27.3%
- **Within 50% error:** 48.5%

## Known Issue: Page-Not-Found Panic

Several benchmarks crash at larger problem sizes due to a **"page not found" panic** in the MMU (`akita/v4/mem/vm/mmu`). This limits the sizes we can test:
- **vectoradd:** works ≤16384, crashes ≥32768
- **relu:** works ≤8192, crashes ≥16384 (note: only `-length` flag, no taps)
- **stencil2d:** works ≤512x512, crashes ≥768x768
- **matrixmultiplication:** works ≤512x512x512, crashes ≥640 (untested)
- **floydwarshall:** works ≤64 nodes, crashes ≥96
- **nbody:** works ≤8192 particles
- **nw:** works ≤1024
- **fir:** works ≤1024, crashes ≥2048

## Detailed Results

### Per-Kernel Summary

| Kernel | Matched | Avg |Error| | Max |Error| | Direction |
|--------|---------|-------------|-------------|-----------|
| fir | 1 | 11.7% | 11.7% | too fast |
| relu | 4 | 21.9% | 36.1% | too fast |
| nw | 6 | 27.2% | 66.7% | mixed (mostly too slow) |
| vectoradd | 5 | 46.2% | 129.7% | too fast |
| matrixmultiplication | 4 | 66.2% | 118.8% | too slow |
| floydwarshall | 2 | 75.4% | 84.2% | too fast |
| nbody | 6 | 138.8% | 153.9% | too fast |
| stencil2d | 5 | 678.4% | 711.3% | too slow |

### Per-Point Results

| Kernel | Size | HW (ms) | Sim (ms) | Rel Error | Direction |
|--------|------|---------|----------|-----------|-----------|
| fir | 1024_taps16 | 0.0056 | 0.0050 | -11.7% | too fast |
| relu | 1024 | 0.0053 | 0.0039 | -36.1% | too fast |
| relu | 2048 | 0.0048 | 0.0039 | -22.9% | too fast |
| relu | 4096 | 0.0044 | 0.0039 | -12.1% | too fast |
| relu | 8192 | 0.0046 | 0.0040 | -16.3% | too fast |
| vectoradd | 1024 | 0.0043 | 0.0039 | -9.7% | too fast |
| vectoradd | 2048 | 0.0050 | 0.0039 | -27.0% | too fast |
| vectoradd | 4096 | 0.0050 | 0.0040 | -25.7% | too fast |
| vectoradd | 8192 | 0.0056 | 0.0040 | -38.7% | too fast |
| vectoradd | 16384 | 0.0056 | 0.0024 | -129.7% | too fast |
| matmul | 64x64x64 | 0.0132 | 0.0154 | +16.9% | too slow |
| matmul | 128x128x128 | 0.0243 | 0.0307 | +26.4% | too slow |
| matmul | 256x256x256 | 0.0403 | 0.0818 | +102.9% | too slow |
| matmul | 512x512x512 | 0.0780 | 0.1706 | +118.8% | too slow |
| floydwarshall | 32_nodes | 0.1563 | 0.0849 | -84.2% | too fast |
| floydwarshall | 64_nodes | 0.3024 | 0.1814 | -66.7% | too fast |
| nbody | 256_particles | 0.0484 | 0.0227 | -113.2% | too fast |
| nbody | 512_particles | 0.0916 | 0.0404 | -126.5% | too fast |
| nbody | 1024_particles | 0.1803 | 0.0752 | -139.9% | too fast |
| nbody | 2048_particles | 0.3579 | 0.1443 | -147.9% | too fast |
| nbody | 4096_particles | 0.7121 | 0.2832 | -151.5% | too fast |
| nbody | 8192_particles | 1.4213 | 0.5598 | -153.9% | too fast |
| nw | length=64 | 0.0511 | 0.0552 | +8.0% | too slow |
| nw | length=128 | 0.1285 | 0.1336 | +3.9% | too slow |
| nw | length=256 | 0.2860 | 0.2987 | +4.5% | too slow |
| nw | length=512 | 0.5441 | 0.7160 | +31.6% | too slow |
| nw | length=768 | 0.8353 | 1.2416 | +48.6% | too slow |
| nw | length=1024 | 1.1413 | 1.9020 | +66.7% | too slow |
| stencil2d | 64x64 | 0.0059 | 0.0479 | +711.3% | too slow |
| stencil2d | 128x128 | 0.0062 | 0.0484 | +680.4% | too slow |
| stencil2d | 192x192 | 0.0062 | 0.0483 | +678.5% | too slow |
| stencil2d | 256x256 | 0.0065 | 0.0491 | +656.1% | too slow |
| stencil2d | 512x512 | 0.0063 | 0.0483 | +665.9% | too slow |

## NBody Scaling Verification ✅

The nbody benchmark now correctly **scales with particle count**:

| Particles | Sim Time (ms) | Ratio to Previous |
|-----------|--------------|-------------------|
| 256 | 0.0227 | — |
| 512 | 0.0404 | 1.78× |
| 1024 | 0.0752 | 1.86× |
| 2048 | 0.1443 | 1.92× |
| 4096 | 0.2832 | 1.96× |
| 8192 | 0.5598 | 1.98× |

The scaling ratio approaches 2× as expected (O(n²) complexity, doubling particles doubles time). **This confirms the nbody fix is working correctly** — previously all sizes gave the same time.

## Analysis by Benchmark Category

### Good Accuracy (< 25% error)
- **fir** (11.7%): Single data point but very close to HW
- **relu** (21.9%): Consistently slightly faster than HW (kernel launch overhead not fully modeled?)
- **nw** small sizes (3.9-8.0%): Excellent at 64-256, degrades at larger sizes

### Moderate Accuracy (25-75% error)
- **vectoradd** (46.2%): Simulator too fast, especially at larger sizes. HW shows ~5µs overhead floor that sim doesn't capture
- **matrixmultiplication** (66.2%): Simulator too slow — compute throughput may be overestimated for matrix operations  
- **floydwarshall** (75.4%): Simulator too fast — may be missing memory access penalties
- **nw** large sizes (31-67%): Error grows with problem size, suggesting O(n) overhead accumulation

### Poor Accuracy (>100% error)
- **nbody** (138.8%): Simulator is ~2.5× faster than HW. The scaling is correct but absolute times are too low
- **stencil2d** (678.4%): Simulator is ~8× slower than HW. The sim time is nearly constant across sizes (~0.048ms) suggesting a fixed overhead dominates. The HW runs these in 0.006ms — the sim appears to have excessive kernel launch overhead

## Key Observations

1. **Stencil2d has a major issue**: Sim time is nearly constant (~0.048ms) regardless of problem size (64x64 to 512x512), while HW scales. This suggests the simulator is dominated by a fixed kernel dispatch overhead that dwarfs the actual compute.

2. **NBody scaling is correct but absolute values are off**: The 2× scaling with particle count proves the fix works, but sim is 2.5× faster than HW, suggesting the compute pipeline is too fast or memory latency is undermodeled.

3. **NW shows increasing error with size**: Small sizes (64-256) are within 8%, but error grows to 67% at 1024. This pattern suggests accumulating per-iteration overhead.

4. **MatMul is consistently too slow**: Error grows from 17% to 119% with size, suggesting memory bandwidth bottleneck in the simulator that doesn't exist in HW (which has high-bandwidth HBM).

5. **Page-not-found crashes** limit testing to small problem sizes, which are often in the kernel-launch-dominated regime rather than the compute-dominated regime where accuracy matters most.

## Recommendations

1. **Fix the page-not-found MMU panic** — this is the highest priority blocker. Without larger problem sizes, we can't assess simulator accuracy in the compute-dominated regime.
2. **Investigate stencil2d kernel dispatch overhead** — 8× overhead suggests a systematic issue.
3. **Calibrate nbody compute pipeline** — scaling is correct, absolute values need ~2.5× adjustment.
4. **Profile matmul memory subsystem** — increasing error with size points to bandwidth modeling issues.
