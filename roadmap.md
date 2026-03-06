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

**M6** (merged, verified): VCCLO mask + MMU register corruption + FLAT SAddr fix
- Fixed VCCLO write mask inversion (wavefront.go:317)
- Fixed out-of-order memory response handling in computeunit.go
- Fixed FLAT SAddr mode in timing: add scalar base + signed offset
- vectoradd width>4096 now works, atax scaling verified
- M6 benchmark results: 26 data points, mean error ~196%

**M7** (merged, verified): Add gfx942 kernel support + fix emulation bugs
- Added gfx942 kernel support to nbody and matrixmultiplication benchmarks
- Fixed VOP2 SDWA decoder for AES benchmark (SGPR operand handling)
- Fixed VOP3P op_sel/op_sel_hi/neg decoding for packed instructions
- All 21 benchmarks pass CDNA3 emulation with -verify
- Timing benchmark results: 72.4% mean error on 9 data points
- matrixmultiplication shows excellent accuracy (8.5-20.7% at small sizes)
- Cycle estimate: budgeted 6, used 6

### Baseline After M7
- Mean symmetrical error: **72.4%** across 9 data points (down from 196% at M6)
- matrixmultiplication: 8.5-20.7% (small sizes) — compute pipeline well-calibrated
- vectoradd: 167-197% over-simulation — memory subsystem issues
- bitonicsort: 43.5% under-simulation
- stencil2d: 124% over-simulation
- nbody: crashes in timing mode (MMU page-not-found)
- Many benchmarks untested in timing mode (need CDNA3 kernel support or crash)

### Key Blocker: MMU Page-Not-Found in Timing Mode
The **single biggest blocker** is the timing-mode register corruption that causes MMU page-not-found panics. This prevents:
- Running nbody in timing mode at all
- Running vectoradd/stencil2d at larger sizes
- Getting enough data points for meaningful error metrics
- Testing benchmarks like floydwarshall, fir, simpleconvolution, etc. in timing mode

Root cause: Upper 32 bits of 64-bit FLAT addresses get corrupted to 0xFFFFFFFF in timing mode. The corruption originates in the scratchpad/register file management of the timing CU. See docs/mmu_page_not_found_investigation.md.

## Planned Milestones

### M8: Fix Timing-Mode Address Corruption (Budget: 8 cycles)
**Objective**: Fix the MMU page-not-found bug in timing mode so benchmarks can run at realistic sizes.
**Investigation** (Athena's team, pre-milestone):
- Harper: Investigate scratchpadpreparer.go register corruption for FLAT instructions
- Emma: Compare ISA debug traces between emulation and timing mode
- Blake: Systematic benchmark crash survey to understand scope
**Tasks** (for Ares):
- Fix the root cause of 64-bit FLAT address corruption in timing mode
- Verify vectoradd works at width >= 65536
- Verify nbody runs in timing mode
- Verify stencil2d works at larger sizes
- Run expanded benchmark suite with many more data points

### M9: Expand Benchmark Coverage + Memory System Tuning (Budget: 6-8 cycles)
- Add gfx942 kernel support to remaining benchmarks (fir, floydwarshall, kmeans, etc.)
- HBM3 bandwidth modeling improvements
- Cache hierarchy tuning
- Address systematic over-simulation for memory-bound workloads

### M10: Final Accuracy Push (Budget: 4-6 cycles)
- Fine-tune all parameters
- GPU-side command queueing (issue #286) if kernel launch overhead remains too high
- Target: avg <20%, max <50%

## Lessons Learned
- SIMD=32 was incorrect — always verify against ISA documentation
- Symmetrical error penalizes both over and underestimates more equally
- Small problem sizes are dominated by kernel launch overhead, not compute
- Development must stay in origin repo, not upstream
- Page-not-found crashes caused by corrupted 64-bit FLAT addresses in timing mode
- stencil2d constant timing is correct parallel GPU behavior, not a bug
- atax zero-work was caused by timing-mode SMEM/VCC corruption, not kernel arg layout
- Switch latency needed to be 15 (Infinity Fabric) not 140 (PCIe)
- s_nop infinite loop was root cause for ALL hanging benchmarks
- Kernel launch overhead was modeled wrong (CPU-side H2D delay vs GPU-side scheduler overhead)
- Cycle estimates: M1-M4 took ~20 cycles; M5 took ~5 cycles; M6 took ~8 cycles; M7 took ~6 cycles
- M5 reduced mean error from 341% to 120% — overhead tuning + bug fixes have massive impact
- **CRITICAL**: MMU crashes happen in EMULATION mode too (not just timing) — emulation bugs must be fixed FIRST
- Human's debugging suggestion (test emulation → compare disassembly → compare traces) is the correct systematic approach
- Always test emulation correctness before investigating timing accuracy
- matrixmultiplication accuracy at 8.5-20.7% shows compute pipeline is well-modeled — main issues are in memory subsystem and overhead
