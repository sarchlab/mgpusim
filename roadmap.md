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

**M8** (merged, verified): Remove timing-side scratchpad
- Deleted scratchpadpreparer.go, scratchpad.go from timing
- Rewrote defaultcoalescer.go to read from registers directly via wf.ReadOperand
- Rewrote scalarunit.go SMEM load to use ReadOperand for Base/Offset
- Removed scratchpadPreparer from all CU units (SIMD, Branch, Scalar, LDS, VecMem)
- All 84 CU tests pass; `go build ./...` clean
- vectoradd working range expanded (4032 → all sizes, just slow)
- Cycle estimate: budgeted 8, used 2

### M9: MISSED DEADLINE (budgeted 8, used 8)
- **Achieved**: Merged Harper's FLAT SAddr fix. Collected 65 data points across 10 benchmarks. Avg |error| = 79.6%, median = 31.6%, 66.2% within 50%.
- **Not achieved**: Target was avg < 50%, actual is 79.6%
- **What went wrong**: The team focused on parameter tuning (cache latencies, DRAM width/latency) but missed the fundamental architectural bottleneck: per-CU memory pipeline buffer size limits effective memory bandwidth to ~250 GB/s vs real MI300A's 1+ TB/s. Also introduced SPU=32 which is architecturally incorrect per CDNA3 ISA.

### M9.1: COMPLETE ✅ (budgeted 6, used 6) — PENDING MERGE
- **Branch**: `ares/m9.1-spu16-membuf32` — verified by Apollo, needs merge to main
- SPU=16 (reverted from 32), memPipelineBufferSize=32 (from 8)
- L1V=32KB/5cyc, L2=20cyc, DRAM BPW=4/SL=1, kernel overhead=5400(/2)
- stencil2d default iter=1, fft default passes=1 (matching HW measurement)
- **Results**: 65 data points, avg |error| = 58.2%, median 35.3%, 69.2% within 50%
- Per-kernel: matmul 4.8%, bicg 20.2%, matrixtranspose 34.5%, atax 40.4%, FWT 45.5%, fir 58.1%, stencil2d 61.8%, vectoradd 87.7%, fft 102%, relu 106.8%

## Active Human Issues & Priorities

1. **#344 — Simulation performance too slow**: Create GitHub Actions CI for parallel benchmarks. Simplify sim if needed. Workers should fire-and-check, not block.
2. **#346 — Host OOM**: Never run simulations on host machine. Use GitHub Actions.
3. **#343 — Evidence-based tuning**: Create microbenchmarks. Use documentation citations. Maintain mi300a_calibration.md.
4. **#286 — GPU-side command queueing**: Deferred, revisit when kernel launch overhead is dominant.

## Planned Milestones

### M10: Merge M9.1 + CI Infrastructure + Memory Bandwidth Fix (Budget: 8 cycles)
**Goal**: Merge M9.1 to main. Create GitHub Actions benchmark workflow. Fix the DRAM bandwidth modeling gap for streaming workloads.

**Deliverables**:
1. **Merge M9.1** to main via PR
2. **GitHub Actions benchmark workflow** — a workflow that runs all benchmarks in parallel jobs, collects CSVs, runs comparison script, and posts accuracy summary. Workers will trigger this workflow and check results later.
3. **Fix DRAM bandwidth** — Per Alex's analysis, change SimpleBankedMemory params: BankPipelineWidth 4→1, StageLatency 1→3. This reduces per-controller throughput to ~341 GB/s × 16 = ~5.5 TB/s (matching real MI300A 5.3 TB/s). Currently the DRAM model has unlimited bandwidth, but the real bottleneck is per-CU pipeline; with the DRAM fix, large streaming workloads should see improvement.
4. **Update mi300a_calibration.md** with evidence for DRAM parameter changes

**Expected impact**: vectoradd/relu large sizes should improve significantly. Target: avg <45%.

### M11: Targeted Accuracy Push (Budget: 8 cycles)
- Based on M10 results, target remaining high-error benchmarks
- Consider log2PageSize=21 (huge pages) for TLB-heavy workloads (FFT, stencil2d)
- Consider per-CU memory pipeline improvements
- Evidence-based tuning with microbenchmarks per human issue #343
- Target: avg <30%

### M12: MFMA Support + Final Accuracy (Budget: 10 cycles)
- Implement MFMA (matrix fused multiply-add) instructions for matrixmultiplication accuracy
- GPU-side command queueing if kernel launch overhead still dominant
- Final parameter tuning
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
- M5 reduced mean error from 341% to 120% — overhead tuning + bug fixes have massive impact
- **CRITICAL**: MMU crashes happen in EMULATION mode too (not just timing) — emulation bugs must be fixed FIRST
- Human's debugging suggestion (test emulation → compare disassembly → compare traces) is the correct systematic approach
- Always test emulation correctness before investigating timing accuracy
- matrixmultiplication accuracy at 8.5-20.7% shows compute pipeline is well-modeled — main issues are in memory subsystem and overhead
- **Scratchpad removal is both cleanup AND bug fix** — the scratchpad was a data-copying indirection layer
- **FLAT SAddr mode detection**: Must use inst.Addr.RegCount (1=SAddr, 2=OFF) not SAddr.IntValue != 0x7F
- The human watches the codebase closely — architectural decisions should be clean and principled
- **M9 lesson**: Pure parameter tuning hits diminishing returns. Root cause analysis BEFORE tuning is essential
- **M9 lesson**: SPU=32 was re-introduced despite being reverted in M2/M3. Must enforce architectural constraints
- **M9 lesson**: The DRAM model (`simplebankedmemory`) is a latency model, not a bandwidth model. The bandwidth bottleneck is in the per-CU memory pipeline (bufferSize=8)
- **M9.1 lesson**: stencil2d and fft defaults (iter=5, passes=2) didn't match real HW measurement methodology. Always verify benchmark settings match the reference data.
- **Operational lesson**: Human explicitly demands we stop running simulations on the host (OOM, issue #346) and use GitHub Actions instead. Must create CI workflows for benchmark evaluation.
- **Operational lesson**: Parameter tuning must be evidence-based (issue #343). Create microbenchmarks, cite documentation, document decisions in mi300a_calibration.md.
- **Cycle estimates**: M1-M4 ~20 cycles; M5 ~5; M6 ~8; M7 ~6; M8 ~2; M9 ~8 (failed); M9.1 ~6
