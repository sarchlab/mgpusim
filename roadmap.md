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

### M8: Remove Timing-Side Scratchpad Preparer (Budget: 8 cycles)
**Objective**: Per human directive (issue #317), remove ALL scratchpad-related code from the timing side. The scratchpad mechanism is a legacy data-copying layer that is now redundant (ALU reads/writes directly via InstEmuState) and is the suspected root cause of timing-mode register corruption bugs.

**What to remove/change**:
1. **Delete `amd/timing/cu/scratchpadpreparer.go`** — the ScratchpadPreparer interface and implementation
2. **Delete `amd/timing/wavefront/scratchpad.go`** — the Scratchpad type and all layout structs
3. **Update all CU units** to remove scratchpadPreparer dependency:
   - `simdunit.go`: Remove Prepare/Commit calls around alu.Run()
   - `branchunit.go`: Remove Prepare/Commit calls 
   - `scalarunit.go`: Remove Prepare call in read stage, Commit call in write stage
   - `ldsunit.go`: Remove Prepare/Commit calls
   - `vectormemoryunit.go`: Remove Prepare calls before coalescer
4. **Fix `defaultcoalescer.go`** — currently reads EXEC/ADDR/DATA from scratchpad. Must read directly from wavefront's register file using ReadOperand
5. **Fix `computeunit.go` handleVectorDataLoadReturn** — currently writes to scratchpad DST. Must write directly to register file
6. **Remove scratchpad field from wavefront.go** and the Scratchpad() method
7. **Update cubuilder.go** — remove scratchpadPreparer creation and passing

**Why this matters**: The scratchpad is the suspected cause of register corruption bugs (upper 32 bits of 64-bit FLAT addresses corrupted to 0xFFFFFFFF). Removing it eliminates an entire class of bugs and simplifies the codebase.

**Acceptance criteria**:
- No references to ScratchpadPreparer, Scratchpad(), or scratchpad layouts remain in timing code
- `go build ./...` passes
- `go test ./amd/...` passes
- vectoradd works in timing mode at various sizes with -verify
- matrixmultiplication works in timing mode with -verify

### M9: Run Full Benchmark Suite + Fix Timing Crashes (Budget: 6-8 cycles)
- Run all 19 benchmarks in timing mode at multiple sizes
- Fix remaining crashes/hangs (vectoradd hang >=4032, nbody crash, stencil2d wrong results)
- Collect comprehensive timing accuracy data
- Address systematic over-simulation for memory-bound workloads

### M10: Memory System Tuning + Final Accuracy Push (Budget: 4-6 cycles)
- HBM3 bandwidth modeling improvements
- Cache hierarchy tuning
- GPU-side command queueing (issue #286) if kernel launch overhead remains too high
- Fine-tune all parameters
- Target: avg <20%, max <50%

### M8 Investigation Results (Athena's team, cycle 312-313)
- **Emma** (trace comparison): Emu vs timing traces match at width=1024. Reported the MMU page-not-found bug may be fixed for vectoradd at previously-failing sizes.
- **Harper** (scratchpad analysis): Found 2 confirmed bugs — SAddr=0 mishandling in prepareFlat, and VCCHI mask inversion. Neither triggers for vectoradd specifically, but the scratchpad is a complex data-copying layer that introduces risk.
- **Blake** (crash survey): vectoradd works up to 3968, hangs >=4032. nbody gfx942 crashes immediately. stencil2d has wrong results at 512. Three distinct failure modes: MMU panic, hang/timeout, wrong results.
- **Human directive** (issue #317): Remove ALL scratchpad-related code from timing side — this supersedes the previous M8 plan of just fixing specific corruption bugs.

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
