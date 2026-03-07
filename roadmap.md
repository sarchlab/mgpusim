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

### Baseline After M8 + Investigations (Cycle 319)
- **Harper's FLAT SAddr fix** (branch: harper/debug-flat-addr-corruption): Fixed readFlatAddr() mode detection using inst.Addr.RegCount instead of SAddr.IntValue. This fixed nbody and stencil2d crashes.
- **Blake's vectoradd investigation**: Confirmed "hang" was just slow simulation (~28-35s per 1M elements), not a deadlock. All sizes work with sufficient timeout.
- **Casey's benchmark survey** (12/16 pass): vectoradd, matrixmultiplication, stencil2d, nbody, atax, bicg, fft, fastwalshtransform, simpleconvolution, fir, relu, matrixtranspose all PASS at small sizes.
- **2 emulation bugs remain**: bitonicsort (wrong sort order), floydwarshall (wrong path results)
- **No more crash bugs** — all remaining issues are accuracy-related
- Harper's fix needs to be merged to main, then comprehensive accuracy measurement needed

### Key Data Points (from Casey's survey, small sizes only)
| Benchmark | Size | Sim kernel_time (ms) |
|-----------|------|---------------------|
| vectoradd | 1024 | 0.0913 |
| matrixmultiplication | 64x64x64 | 0.0250 |
| stencil2d | 64x64 | 0.0683 |
| stencil2d | 256x256 | 0.0707 |
| nbody | 256 | 0.2965 |
| atax | 64x64 | 0.0356 |
| bicg | 64x64 | 0.0370 |
| fft | 1MB | 0.0641 |
| fastwalshtransform | 1024 | 0.0594 |
| simpleconvolution | 128x128 mask3 | 0.0098 |
| fir | 1024 | 0.0105 |
| relu | 1024 | 0.0076 |
| matrixtranspose | 256 | 0.0144 |

## Planned Milestones

### M9: Merge Harper Fix + Comprehensive Accuracy Baseline + Initial Tuning (Budget: 8 cycles)
**Goal**: Merge the FLAT SAddr fix, collect accuracy data across ALL benchmarks at sizes matching mi300a.csv hardware data, and begin parameter tuning to reduce error.

Key tasks:
1. Merge Harper's FLAT SAddr fix (harper/debug-flat-addr-corruption) to main
2. Run `run_sim_benchmarks.sh` with expanded benchmark list covering all 12+ working benchmarks, at sizes matching mi300a.csv
3. Run `compare_sim_vs_real.py` to get per-benchmark error breakdown
4. Identify the top error contributors and begin tuning:
   - Memory system parameters (HBM3 bandwidth, cache latencies)
   - Kernel launch overhead (constantKernelOverhead, H2D/D2H)
   - ALU pipeline latencies
5. Target: establish quantified baseline and reduce average error below 50%

### M10: Memory System Modeling + Final Accuracy Push (Budget: 8 cycles)
- HBM3 bandwidth modeling improvements
- Cache hierarchy tuning (L1/L2 sizes, latencies, associativity)
- GPU-side command queueing (issue #286) if kernel launch overhead is still dominant
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
- Cycle estimates: M1-M4 ~20 cycles; M5 ~5; M6 ~8; M7 ~6; M8 ~2 cycles
