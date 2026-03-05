# Roadmap: Remove Scratchpad from Emulator

## Goal
Remove the scratchpad intermediary from the emulator's instruction execution pipeline. The ALU should read/write wavefront registers directly instead of going through a copy-in/execute/copy-out cycle via a flat byte buffer.

## Current State
- M1 (Investigation) is **COMPLETE** (1 cycle used, 3 budgeted)
- Key findings:
  - **88.5% of emulation time is scratchpad overhead** (only 11.5% is actual ALU computation)
  - 532 total `Scratchpad()` call sites across the codebase
  - GCN3 ALU: ~220 calls across 13 files; CDNA3 ALU: ~251 calls across 13 files
  - Emu scratchpad preparer: 640 lines; Timing scratchpad preparer: 703 lines
  - **Design chosen**: Replace `Scratchpad()` with `ReadOperand()`/`WriteOperand()` on `InstEmuState` interface (Kai's Option C)
  - **Timing path has non-ALU scratchpad consumers**: `defaultcoalescer.go` and `scalarunit.go` read scratchpad directly
  - **Field naming conflicts**: `emu.Wavefront` has fields `VCC`, `SCC`, `PC`, `Exec` that conflict with proposed interface method names — fields must be renamed to unexported

## Design Summary (from Kai's proposal)
The new `InstEmuState` interface adds:
- `ReadOperand(operand, laneID) uint64` — resolves any operand type (reg, int, float, literal)
- `WriteOperand(operand, laneID, value)` — writes to register operand
- `ReadOperandBytes(operand, laneID, byteCount) []byte` — for multi-dword (SMEM, Flat, DS)
- `WriteOperandBytes(operand, laneID, data)` — for multi-dword writes
- `EXEC()/SetEXEC()`, `VCC()/SetVCC()`, `SCC()/SetSCC()`, `PC()/SetPC()` — special register access

Both `emu.Wavefront` and `timing.Wavefront` implement this interface. The ALU code changes from `sp := state.Scratchpad().AsVOP2(); sp.SRC0[i]` to `state.ReadOperand(inst.Src0, i)`.

## Milestones

### M1: Investigation & Performance Baseline ✅ COMPLETE
**Budget**: 3 cycles | **Actual**: 1 cycle
**Outcome**: Full scope mapped, design chosen, performance baseline established.

### M2: Add New Interface + Migrate Scalar Instructions
**Budget**: 8 cycles
**Status**: NEXT
**Scope**:
1. Add `ReadOperand`/`WriteOperand`/`ReadOperandBytes`/`WriteOperandBytes` + special register accessors to `InstEmuState` interface
2. Implement on `emu.Wavefront` (rename conflicting fields to unexported)
3. Implement on `timing/wavefront.Wavefront` (add `RegFileAccessor` for CU register file access)
4. Migrate SOP1, SOP2, SOPC, SOPK, SOPP instruction implementations in GCN3 ALU
5. Migrate SOP1, SOP2, SOPC, SOPK, SOPP instruction implementations in CDNA3 ALU
6. Migrate SMEM instruction implementations (both arches)
7. Update corresponding unit tests
8. All CI checks pass, all benchmarks still correct (`-verify`)

**Why 8 cycles**: Interface changes affect foundational types used everywhere. The field renaming in `emu.Wavefront` will ripple through many files. Plus ~100 instruction implementations + tests across 2 arches.

### M3: Migrate Vector Instructions
**Budget**: 10 cycles
**Status**: Not started
**Scope**: Convert VOP1, VOP2, VOP3A, VOP3B, VOPC in both GCN3 and CDNA3 ALUs. ~350 instruction implementations. Mechanically straightforward but high volume.

### M4: Migrate Memory Instructions + Timing Path
**Budget**: 8 cycles
**Status**: Not started
**Scope**: Convert FLAT, DS instructions. Handle timing path's non-ALU scratchpad consumers (coalescer, scalar unit). Add staging buffer for timing memory operations.

### M5: Cleanup + Performance Benchmark
**Budget**: 4 cycles
**Status**: Not started
**Scope**: Remove scratchpad code, measure before/after performance, update documentation.

## Lessons Learned
- M1 investigation completed in 1 cycle (budgeted 3) — good use of parallel workers
- Smaller milestones = better (2-3 items each)
- Budget for unknowns — instruction format changes often reveal hidden dependencies
- The investigation revealed that timing path changes are more complex than expected (non-ALU consumers)
- Field naming conflicts in Go require careful planning (fields vs methods with same name)
