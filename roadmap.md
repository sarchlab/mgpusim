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

### Baseline After M6
Error was measured at ~196% mean across 26 data points. However, M6 focused on correctness fixes rather than accuracy tuning. Many benchmarks still crash (nbody at all sizes, stencil2d at large sizes).

**Critical Discovery**: nbody and matrixmultiplication crash in CDNA3 **EMULATION** mode too (not just timing). This means the CDNA3 instruction emulation itself is broken. The human (issue #299) correctly suggested testing emulation first.

## Active Phase: M7 — Add gfx942 Kernel Support + Fix Emulation

### Investigation Results (Completed)
Harper and Iris investigated the CDNA3 emulation crashes (issue #304). Key findings:

1. **Root cause**: nbody and matrixmultiplication only embed GCN3 (gfx803) kernel binaries. When run with `-arch cdna3`, the CDNA3 disassembler interprets `SAddr=0x00` as "use s[0:1]" (correct for CDNA3), but the GCN3 binary means "OFF" → garbage scalar base → page fault.
2. **Disassembler is correct**: MGPUSim's CDNA3 disassembler matches llvm-objdump for all tested gfx942 encodings (one cosmetic print issue, no functional bugs).
3. **18/21 benchmarks pass CDNA3 emulation** with `-verify`. Only nbody and matrixmultiplication crash (missing gfx942 kernels). aes fails verification (separate bug — wrong results with gfx942 kernel).
4. **gfx942 hsaco binaries already exist** in `native/` directories for both nbody and matrixmultiplication.

### M7: Add gfx942 Kernel Support + Fix Emulation Bugs (Budget: 6 cycles)
**Objective**: All benchmarks pass CDNA3 emulation with `-verify`, and timing-mode benchmarks run for nbody and matrixmultiplication.
**Tasks**:
1. Add gfx942 kernel support to nbody benchmark (copy hsaco, add CDNA3 kernel args struct with hidden fields, add `Arch` field, conditional loading, update sample main.go)
2. Add gfx942 kernel support to matrixmultiplication benchmark (same pattern)
3. Investigate and fix aes emulation bug (wrong results with gfx942 kernel, possibly related to `global_load_sbyte` with SAddr)
4. Verify all benchmarks pass CDNA3 emulation with `-verify`
5. Run timing benchmark suite and report updated error measurements

### M8: Emulation vs Timing Trace Comparison (Budget: 4-6 cycles)
- Use -debug-isa to dump instruction traces in both modes
- Compare register values, addresses, instruction sequences
- Find timing-specific correctness bugs

### M9: Memory System Accuracy Tuning (Budget: 6-8 cycles)
- HBM3 bandwidth modeling (current SimpleBankedMemory too slow at large sizes)
- Cache hierarchy tuning
- Address the systematic error where sim scales linearly with size while HW is nearly flat

### M10: Final Accuracy Push (Budget: 4-6 cycles)
- Fine-tune all parameters
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
- Cycle estimates: M1-M4 took ~20 cycles; M5 took ~5 cycles; M6 took ~8 cycles
- M5 reduced mean error from 341% to 120% — overhead tuning + bug fixes have massive impact
- **CRITICAL**: MMU crashes happen in EMULATION mode too (not just timing) — emulation bugs must be fixed FIRST
- Human's debugging suggestion (test emulation → compare disassembly → compare traces) is the correct systematic approach
- Always test emulation correctness before investigating timing accuracy
