# Roadmap: Remove Scratchpad from Emulator

## Goal
Remove the scratchpad intermediary from the emulator's instruction execution pipeline. The ALU should read/write wavefront registers directly instead of going through a copy-in/execute/copy-out cycle via a flat byte buffer.

## Current State
- Project previously completed M1-M7 (22 CDNA3 benchmarks, GPU perf scripts)
- New task from human (issue #156): Remove scratchpad for performance improvement
- Scratchpad files: `scratchpad.go` (197 lines), `scratchpadpreparer.go` (640 lines)
- Both GCN3 ALU (`amd/emu/alu*.go`) and CDNA3 ALU (`amd/emu/cdna3/`) use scratchpad extensively
- GCN3 ALU: ~500+ `Scratchpad()` references across instruction implementations
- CDNA3 ALU: ~250+ `Scratchpad()` references
- Timing simulation (`amd/timing/`) may also reference scratchpad — needs investigation

## Milestones

### M1: Investigation & Performance Baseline
**Budget**: 3 cycles
**Status**: Not started
**Scope**: 
1. Evaluate the current scratchpad overhead with benchmarks (before measurement)
2. Investigate how the timing simulation (`amd/timing/`) uses scratchpad
3. Design the new interface: how ALU will read/write wavefront registers directly
4. Identify all code that references scratchpad (both emu and timing paths)
5. Plan the migration order (which instruction formats to convert first)

### M2: Implement direct wavefront access for scalar instructions
**Budget**: 6 cycles
**Status**: Not started
**Scope**: Convert SOP1, SOP2, SOPC, SOPK, SOPP, SMEM instruction formats to use direct wavefront register access instead of scratchpad. Both GCN3 and CDNA3 ALUs.

### M3: Implement direct wavefront access for vector instructions
**Budget**: 8 cycles
**Status**: Not started
**Scope**: Convert VOP1, VOP2, VOP3a, VOP3b, VOPC instruction formats. These are more complex due to 64-lane vector operations.

### M4: Implement direct wavefront access for memory/DS instructions
**Budget**: 6 cycles
**Status**: Not started
**Scope**: Convert FLAT, DS instruction formats. Handle the storage accessor integration.

### M5: Cleanup & Performance Measurement
**Budget**: 4 cycles
**Status**: Not started
**Scope**: Remove scratchpad code, measure performance improvement, update documentation.

## Lessons Learned (from previous project phase M1-M7)
- Smaller milestones = better (2-3 items each)
- Budget for unknowns — instruction format changes often reveal hidden dependencies
- Commit early, test often
- Integration must be continuous — merge bug fixes immediately
- Know when to defer vs. fix
