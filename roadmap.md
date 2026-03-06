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
| stencil2d | 356.0% | sim>real, ~7x overestimate (not zero-work) | ❌ Accuracy issue |
| atax | 542.7% | sim<real, ZERO WORK BUG | ❌ Broken |

**Overall: mean 120.1%, median 55.6%, 31% within 25%**

## Investigation Results (Pre-M6)

### Issue #294: MMU Root Cause (Harper) — COMPLETE
- **Root cause**: Asynchronous vector memory load responses overwrite VRegFile registers
- v0 gets corrupted with stale data (0x9C000000) between correct computation and use
- v_ashrrev_i32 then sign-extends the corrupted value → upper 32 bits = 0xFFFFFFFF
- The only mechanism writing to VRegFile outside ALU is handleVectorDataLoadReturn
- **Additional bug**: VCCLO write mask inverted at wavefront.go:317 (0x00000000ffffffff should be 0xFFFFFFFF00000000)
- VRegOffset allocation verified correct (no overlap)
- VCC per-wavefront storage verified correct

### Issue #295: atax/stencil2d (Iris) — COMPLETE
- **atax**: ALL wavefronts execute only 13 instructions (early-exit path). EXEC=0 for all WFs after s_and_saveexec_b64. SMEM loads for NX/NY likely return wrong data in timing mode. May be caused by VCCLO mask bug.
- **stencil2d**: NOT a zero-work bug. Each WG does fixed 591 instructions for 16×64 tile. More WGs run in parallel on MI300A's 120 CUs. Constant time is correct GPU behavior. ~7x overestimate is a separate accuracy issue.

## Active Milestone

### M6: Fix VCCLO mask + MMU register corruption + atax bug (Budget: 8 cycles)
See issue #296 for full details.

**Tasks**:
1. Fix VCCLO write mask inversion (wavefront.go:317) — quick, high-confidence fix
2. Re-test atax after VCCLO fix to see if it resolves the zero-work bug
3. Fix or mitigate asynchronous load response register corruption (MMU crash root cause)
4. Run full benchmark suite and collect updated error numbers

**Acceptance criteria**:
- vectoradd at width=16384 does NOT crash with page-not-found
- atax shows scaling execution time across different problem sizes
- All existing tests pass
- Updated benchmark error numbers documented

## Planned Milestones

### M7: Accuracy Tuning + Larger Problem Sizes
- After M6 fixes, re-benchmark at larger/realistic sizes
- Address systematic errors: nbody, fastwalshtransform, kmeans, floydwarshall
- DRAM model (HBM3 parameters), cache hierarchy tuning
- Target: mean error < 50%

### M8: Final Accuracy Push
- Fine-tune all parameters
- Target: avg <20%, max <50%

## Lessons Learned
- SIMD=32 was incorrect — always verify against ISA documentation
- Symmetrical error penalizes both over and underestimates more equally
- Small problem sizes are dominated by kernel launch overhead, not compute
- Development must stay in origin repo, not upstream
- Page-not-found crashes caused by corrupted 64-bit FLAT addresses in timing mode
- stencil2d constant timing is correct parallel GPU behavior, not a bug
- atax zero-work is caused by timing-mode SMEM/VCC corruption, not kernel arg layout
- Switch latency needed to be 15 (Infinity Fabric) not 140 (PCIe)
- s_nop infinite loop was root cause for ALL hanging benchmarks
- Kernel launch overhead was modeled wrong (CPU-side H2D delay vs GPU-side scheduler overhead)
- Cycle estimates: M1-M4 took ~20 cycles; M5 took ~5 cycles (budget was 6)
- M5 reduced mean error from 341% to 120% — overhead tuning + bug fixes have massive impact
- MMU bug is the single biggest remaining blocker — prevents realistic benchmarking
- Dedicated investigation cycles before defining milestones prevents wasted implementation effort
- VCCLO mask bug (inverted bit mask) is a latent corruption source — always double-check bitwise operations
