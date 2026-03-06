# MI300A Timing Simulation — Roadmap

## Goal
Average symmetrical error < 20%, max < 50% across MI300A benchmarks.

## Current Status

### Completed Milestones

**M1** (merged): Basic MI300A timing config
- Frequency 1800MHz, 240 CUs, 32MB L2, SimpleBankedMemory DRAM
- wfPoolSize=8, vgprCount=32768
- L1V: bankLatency=20, MSHR=32
- L2: bankLatency=50, dirLatency=4

**M2** (merged): CU pipeline changes
- VecMem pipeline depth: inst=2, trans=4
- SIMD width confirmed as 16 (reverted incorrect change to 32)

**M3** (merged, PR #27): Correct baseline established
- Reverted SIMD width to 16
- Fixed nbody bug (numBodies calculation)
- Updated compare script to symmetrical error formula
- 33 benchmark data points across 8 benchmarks documented

**M4** (merged, verified): Kernel launch + interconnect fixes
- Cached kernel code object GPU addresses across launches
- Fixed memRangeOverlap adjacency bug (> instead of >=)
- Investigated MMU page-not-found panics (corrupted 64-bit FLAT addresses in timing mode)
- Reduced MI300A switch latency from 140→15 (Infinity Fabric, not PCIe)
- storageAccessor.Write bug fixed

**M5** (merged, verified): s_nop fix + kernel overhead tuning
- Fixed s_nop infinite loop in scheduler.go (default case now advances PC)
- Reduced H2D 14500→500, D2H 8500→300 for MI300A (unified memory)
- Set constantKernelOverhead to 3600 (~2µs GPU-side dispatch)
- Unlocked 4 new benchmarks: bitonicsort, matrixtranspose, fastwalshtransform, simpleconvolution(partial)
- 13 benchmarks now produce results (vs 8 before)
- Mean error: 120.1%, median: 55.6%

### Baseline After M5 (42 matched data points, 13 benchmarks)
| Benchmark | Avg |Error| | Direction | Status |
|-----------|-------------|-----------|--------|
| nw | 5.8% | ~neutral | ✅ Excellent |
| spmv | 9.6% | ~neutral | ✅ Excellent |
| matrixtranspose | 9.7% | sim<real | ✅ Excellent (NEW) |
| matmul | 15.6% | sim>real | ✅ Good |
| bitonicsort | 23.2% | sim<real | ✅ Good (NEW) |
| fir | 35.9% | sim<real | ⚠️ Fair |
| relu | 39.0% | sim<real | ⚠️ Fair |
| vectoradd | 43.1% | sim<real | ⚠️ Fair |
| floydwarshall | 67.2% | sim<real | ⚠️ Over-estimated |
| kmeans | 77.8% | sim>real | ⚠️ Over-estimated |
| fastwalshtransform | 111.2% | sim<real | ❌ Sim too fast (NEW) |
| nbody | 136.5% | sim<real | ❌ Sim too fast |
| stencil2d | 356.0% | sim>real, CONSTANT TIME BUG | ❌ Broken |
| atax | 542.7% | sim<real, CONSTANT TIME BUG | ❌ Broken |

**Overall: mean 120.1%, median 55.6%, 31% within 25%**

## Critical Blockers

### 1. MMU Page-Not-Found Bug (BLOCKER)
- Corrupted 64-bit FLAT addresses (upper 32 bits = 0xFFFFFFFF) in timing mode
- Limits all benchmarks to very small problem sizes
- Root cause: register corruption in timing CU (VRegOffset, VCC, or scratchpad issues)
- See docs/mmu_page_not_found_investigation.md
- **Impact**: Cannot benchmark at realistic sizes → error measurements unreliable

### 2. Constant-Time Bugs (atax, stencil2d)
- atax: ~5.656ms regardless of 48x48 to 128x128 (should scale)
- stencil2d: ~28ms regardless of 64x64 to 256x256 (should scale)
- Likely: kernels executing zero useful work, or kernel argument layout mismatch

## Active Investigation (Pre-M6)

### Issue #294: MMU root cause investigation (Harper)
- Deep dive into timing CU register handling
- VRegOffset assignment, VCC propagation, scratchpad prepare/commit

### Issue #295: atax/stencil2d constant-time bug (Iris)
- Kernel argument layout analysis
- CDNA3 kernel binary verification
- Emu vs timing comparison

## Planned Milestones

### M6: Fix constant-time bugs + MMU crash fix
- **Depends on**: Investigation results from #294 and #295
- Fix atax constant simulation time
- Fix stencil2d constant simulation time
- Fix or mitigate MMU page-not-found crashes to enable larger problem sizes
- Run benchmarks at larger sizes
- Target: mean error < 80%, at least 15 benchmarks producing results

### M7: Compute/Memory Accuracy Tuning
- Address systematic errors: nbody (~1.4x too fast), fastwalshtransform (~1.1-1.9x too fast)
- kmeans (~0.8x too slow), floydwarshall (~0.6x too slow)
- DRAM model accuracy (HBM3 parameters)
- Target: mean error < 50%

### M8: Final Accuracy Push
- Fine-tune all parameters
- Cache hierarchy tuning
- Memory bandwidth model
- Target: avg <20%, max <50%

## Lessons Learned
- SIMD=32 was incorrect — always verify against ISA documentation
- Symmetrical error penalizes both over and underestimates more equally
- Small problem sizes are dominated by kernel launch overhead, not compute
- Development must stay in origin repo, not upstream
- Page-not-found crashes caused by corrupted 64-bit FLAT addresses in timing mode
- Stencil2d and atax show constant timing → kernel execution likely zero (constant overhead only)
- Switch latency needed to be 15 (Infinity Fabric) not 140 (PCIe)
- s_nop infinite loop was root cause for ALL hanging benchmarks
- Kernel launch overhead was modeled wrong (CPU-side H2D delay vs GPU-side scheduler overhead)
- Cycle estimates: M1-M4 took ~20 cycles; M5 took ~5 cycles (budget was 6)
- M5 reduced mean error from 341% to 120% — overhead tuning + bug fixes have massive impact
- MMU bug is the single biggest remaining blocker — prevents realistic benchmarking
