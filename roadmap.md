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
- **Key M9 changes on branch `ares/m9-accuracy-tuning`** (not yet merged to main):
  - SPU set to 32 (WRONG — must revert to 16)
  - L1V cache 32KB, bankLatency=5
  - L2 bankLatency=20 (was 50)
  - DRAM BankPipelineWidth=4, StageLatency=1
  - constantKernelLaunchOverhead=5400 with first/subsequent model (/4 divisor)
  - VecMem pipeline stages: inst=2, trans=4

### Root Cause Analysis (Quinn, issue #336)
Three root causes explain all high-error benchmarks:
1. **Memory bandwidth gap**: Sim saturates at ~250 GB/s, real MI300A achieves 1+ TB/s. Bottleneck is per-CU memory pipeline buffers (default 8), NOT the DRAM model.
2. **SPU=32 is incorrect**: Makes FWT compute 2x too fast. Must revert to 16.
3. **Kernel overhead /4 too aggressive**: FWT too fast, stencil2d dominated by per-kernel dispatch cost.

### DRAM Model Analysis (Alex, issue #335)
- `simplebankedmemory` is a latency model, NOT a bandwidth model — provides essentially unlimited bandwidth.
- The `directconnection` between L2 and DRAM has no bandwidth limit.
- Real bottleneck is CU-side: memPipelineBufferSize=8 limits concurrent outstanding requests per CU.
- DRAM parameters (width, stageLatency) barely affect results because CU can't feed enough requests.

## Planned Milestones

### M9.1: Fix SPU + Increase Memory Pipeline Throughput + Re-baseline (Budget: 6 cycles)
**Goal**: Apply the 3 critical corrections identified by root cause analysis and measure the impact.

**Code changes** (on a new branch from main, cherry-picking good M9 changes):
1. **Revert SPU to 16** (correctness — CDNA3 ISA mandates 16 FP32 ALUs per SIMD)
2. **Set memPipelineBufferSize to 32** for MI300A (increase per-CU memory throughput, currently defaults to 8)
3. **Keep kernel overhead /2** (not /4 — compromise between FWT and stencil2d)
4. **Keep good M9 parameter changes**: L1V=32KB bankLatency=5, L2 bankLatency=20, DRAM width=4 stageLatency=1, constantKernelLaunchOverhead=5400
5. Re-run all benchmarks and produce comparison report

**Expected impact**:
- FWT error: ~273% → ~50% (SPU revert + /2 overhead)
- vectoradd/relu at large sizes: significant improvement (more memory bandwidth)
- stencil2d: slight improvement (/2 vs /4 overhead)
- matrixmultiplication: may regress from 18% to ~35% (SPU revert) — acceptable, will need MFMA instructions later

**Acceptance criteria**: Produce comparison report with all 65+ data points. Average |error| should improve below 60%.

### M9.2: Targeted Kernel-Specific Tuning (Budget: 6 cycles)
- Based on M9.1 results, tune remaining high-error benchmarks
- Consider log2PageSize=21 (huge pages) for TLB-heavy workloads (FFT, stencil2d)
- Consider DRAM frequency tuning if memory bandwidth is still low
- Target: avg <40%

### M10: Memory System Modeling + Final Accuracy Push (Budget: 8 cycles)
- HBM3 bandwidth modeling improvements if needed
- GPU-side command queueing (issue #286) if kernel launch overhead is still dominant
- MFMA instruction support for matrixmultiplication accuracy
- Fine-tune all parameters toward target
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
- **FLAT SAddr mode detection**: Must use inst.Addr.RegCount (1=SAddr, 2=OFF) not SAddr.IntValue != 0x7F. GCN3 FLAT instructions have SAddr bits = 0 (reserved).
- The human watches the codebase closely — architectural decisions should be clean and principled
- "Hang" reports should be tested with much longer timeouts before being declared bugs — vectoradd was just slow, not stuck
- Cycle estimates: M1-M4 ~20 cycles; M5 ~5; M6 ~8; M7 ~6; M8 ~2; M9 ~8 (failed)
- **M9 lesson**: Pure parameter tuning hits diminishing returns. Root cause analysis BEFORE tuning is essential — identify architectural bottlenecks first, then tune parameters. M9 wasted cycles tuning DRAM parameters while the real bottleneck was per-CU memory pipeline buffers.
- **M9 lesson**: SPU=32 was re-introduced despite being reverted in M2/M3. Must enforce architectural constraints — never deviate from ISA-documented values for correctness parameters.
- **M9 lesson**: The DRAM model (`simplebankedmemory`) is a latency model, not a bandwidth model. The bandwidth bottleneck is in the per-CU memory pipeline (bufferSize=8). Increasing memPipelineBufferSize is the correct fix for memory bandwidth, not DRAM parameter tuning.
