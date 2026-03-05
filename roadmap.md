# Roadmap: Scratchpad Removal for Emulation Performance

## Goal
Remove the scratchpad intermediary pattern from instruction execution in MGPUSim's emulator. Profiling shows 88.5% of emulation time is scratchpad overhead. Replacing it with direct ReadOperand/WriteOperand access should yield 3-5x speedup.

## Current State (post-M2)
- `ReadOperand`/`WriteOperand`/`ReadOperandBytes`/`WriteOperandBytes` interface added to `InstEmuState`
- `EXEC()`, `SetEXEC()`, `VCC()`, `SetVCC()`, `SCC()`, `SetSCC()`, `PC()`, `SetPC()` accessors added
- Implemented on both `emu.Wavefront` and `timing/wavefront.Wavefront` (via CURegFileAccessor)
- All scalar instructions (SOP1/SOP2/SOPC/SOPK/SOPP/SMEM) migrated in both GCN3 and CDNA3
- Scalar formats are no-ops in timing Prepare/Commit
- **Remaining**: ~330 functions across vector (VOP1/VOP2/VOP3A/VOP3B/VOPC) and memory (FLAT/DS) formats
- **Branch**: `gfx942_emu` (M2 merged via PR #21)

## Milestones

### M1: Investigation & Design
**Budget**: 2 cycles | **Actual**: 1 cycle | **Status**: ✅ COMPLETE
- Devon: Mapped all 471+ scratchpad call sites
- Morgan: Profiled overhead (88.5% of execution time)
- Kai: Designed ReadOperand/WriteOperand interface (Option C)

### M2: Add Interface + Migrate Scalar Instructions
**Budget**: 8 cycles | **Actual**: 3 cycles | **Status**: ✅ COMPLETE (PR #21 merged)
- Extended InstEmuState with ReadOperand/WriteOperand
- Implemented on emu.Wavefront (renamed VCC→vcc, SCC→scc, etc.)
- Implemented on timing.Wavefront via CURegFileAccessor
- Migrated 68 GCN3 + 82 CDNA3 scalar functions
- Verified by Apollo: all CI passing

### M3: Migrate Vector Instructions (VOP1, VOP2, VOP3A, VOP3B, VOPC)
**Budget**: 8 cycles | **Status**: NOT STARTED
**Scope**: Migrate ~267 functions in both GCN3 and CDNA3 ALUs from scratchpad to ReadOperand/WriteOperand.

Breakdown:
- GCN3: VOP1 (23), VOP2 (31), VOP3A (42), VOP3B (8), VOPC (32) = 136 functions
- CDNA3: VOP1 (26), VOP2 (35), VOP3A (54), VOP3B (9), VOPC (28) = 152 functions (~some are new, some shared)

Key differences from scalar migration:
- Vector instructions operate on 64 lanes — each function loops over lanes with EXEC mask
- ReadOperand/WriteOperand must be called per-lane (laneID parameter)
- SDWA (Sub-Dword Access) handling must be preserved
- VOP3 modifiers (abs, neg, clamp, omod) must be preserved
- Timing scratchpad preparer for vector formats must become no-ops (like scalar)

### M4: Migrate Memory Instructions (FLAT, DS) + Timing Non-ALU Consumers
**Budget**: 6 cycles | **Status**: NOT STARTED
**Scope**: 
- GCN3: FLAT (11), DS (9) = 20 functions
- CDNA3: FLAT (11), DS (11) = 22 functions
- Timing non-ALU consumers: coalescer, scalar unit (~5 sites)
- These instructions use `storageAccessor.Read/Write` — must preserve memory access patterns

### M5: Cleanup + Performance Benchmark
**Budget**: 4 cycles | **Status**: NOT STARTED
**Scope**:
- Remove `Scratchpad()` from InstEmuState interface
- Remove scratchpad layout structs (13 types in scratchpad.go)
- Remove ScratchpadPreparer implementations
- Remove scratchpad field from wavefront structs
- Run Morgan's performance benchmarks to measure improvement
- Clean up dead code, update tests

## Lessons Learned

### M1 (1 cycle)
- Independent investigation by multiple workers (Devon, Morgan, Kai) produced excellent results
- 88.5% overhead is massive — this is a high-value optimization

### M2 (3 cycles of 8 budget)
- Scalar migration was faster than expected — pattern is straightforward
- CURegFileAccessor bridge for timing wavefront was the hardest part
- BytesToUint64 panic fix needed for 32-bit register operands
- Making scalar Prepare/Commit no-ops in timing was clean
- Parallel workers (Riley, Maya, Leo, Sam, Niko) efficiently split across files
