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

**M8** (merged, verified): Remove timing-side scratchpad
- Deleted scratchpadpreparer.go, scratchpad.go from timing
- Rewrote defaultcoalescer.go to read from registers directly via wf.ReadOperand
- Rewrote scalarunit.go SMEM load to use ReadOperand for Base/Offset
- Removed scratchpadPreparer from all CU units (SIMD, Branch, Scalar, LDS, VecMem)
- All 84 CU tests pass; `go build ./...` clean
- vectoradd works to ~5000 width (previously hung at 4032)
- Cycle estimate: budgeted 8, used 2

### Baseline After M8
- Scratchpad removed — one class of corruption bugs eliminated
- vectoradd working range expanded (4032 → ~5000+) 
- nbody 256 particles still crashes with page-not-found in timing mode
- stencil2d 512x512 still crashes with page-not-found
- vectoradd hangs at width >= ~8192 (different from crash — possibly resource exhaustion)
- Still 0xFFFFFFFF corruption in upper 32 bits of FLAT addresses — source is NOT scratchpad
- Mean error still ~72.4% (need more data points)
- Key remaining suspects: VCC carry propagation bugs, register file access bugs, or instruction decoding issues

## Planned Milestones

### M9: Fix Remaining Timing-Mode Crashes (Budget: 6-8 cycles)
- Fix MMU page-not-found crashes (nbody, stencil2d) — root cause is still address corruption (0xFFFFFFFF upper 32 bits)
- Fix vectoradd hang at larger sizes (>=8192)
- Must fix at least: nbody, stencil2d, vectoradd at larger sizes
- Collect comprehensive timing accuracy data after fixes

### M10: Memory System Tuning + Final Accuracy Push (Budget: 6-8 cycles)
- HBM3 bandwidth modeling improvements
- Cache hierarchy tuning
- GPU-side command queueing (issue #286) if kernel launch overhead remains too high
- Fine-tune all parameters
- Target: avg <20%, max <50%

### M8 Investigation Results (Athena's team, cycle 312-314)
- **Emma** (trace comparison): Emu vs timing traces match at width=1024.
- **Harper** (scratchpad analysis): Found 2 bugs in scratchpad code (SAddr=0, VCCHI mask). Scratchpad then removed entirely per human directive.
- **Blake** (crash survey): vectoradd works to 3968 (hangs >=4032); nbody gfx942 crashes; stencil2d wrong results at 512.
- **Human directive** (issue #317): Remove ALL scratchpad-related code — done in M8.
- **Post-M8**: vectoradd boundary improved (4032→~5000), but page-not-found and hangs persist at larger sizes.

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
- **Scratchpad removal is both cleanup AND bug fix** — the scratchpad was a data-copying indirection layer. Since the ALU now reads/writes directly through InstEmuState (ReadOperand/WriteOperand), the scratchpad layer is redundant and introduced corruption bugs. Human confirmed it should be removed entirely.
- The human watches the codebase closely — architectural decisions should be clean and principled, not band-aid fixes
