# Spec

## What do you want to build

Remove the scratchpad abstraction from the emulator to improve emulation performance. The scratchpad prepares operand data for ALU units and collects results, providing a clean interface but adding performance overhead. The alternative is to let the wavefront struct hold emulation register and LDS data, with the emulator ALU directly interfacing with the wavefront to read/write register data via ReadOperand/WriteOperand.

**Phase 2 (issue #200):** Fully remove the scratchpad prepare and commit functions from the emu package — not just make them no-ops, but delete the entire ScratchpadPreparer, Scratchpad type, and all related code. Also investigate and fix any unreasonable delays or performance bottlenecks in the emulation process.

## How do you consider the project is success

1. ✅ All instruction implementations use ReadOperand/WriteOperand instead of Scratchpad.
2. ✅ All emu Prepare/Commit functions are no-ops.
3. ✅ All tests pass.
4. ✅ Benchmarks show emulation performance improvement (2x for vector instructions, 13.5% end-to-end).
5. The emu ScratchpadPreparer, Scratchpad type, scratchpad layouts, and scratchpad field on emu Wavefront are fully removed.
6. The `Scratchpad()` method is removed from the `InstEmuState` interface (or kept only for timing).
7. The `executeInst` function in emu computeunit.go no longer calls Prepare/Commit.
8. Any additional performance bottlenecks in the emulation process are identified and fixed.
9. All tests continue to pass after full removal.

## Constraints
- Must not break existing GCN3 or CDNA3 emulation functionality.
- Must not break timing simulation (timing scratchpadpreparer still uses scratchpad for Flat/DS/SMEM coalescing).
- The `Scratchpad()` method may need to remain in the `InstEmuState` interface because timing's wavefront also implements it and timing still uses it. The approach should either: (a) keep `Scratchpad()` in the interface but remove it from emu Wavefront's usage, or (b) split the interface.

## Architecture Notes

### Timing mode scratchpad usage (MUST KEEP)
- `amd/timing/cu/defaultcoalescer.go` — reads `Scratchpad().AsFlat()` for EXEC, ADDR, DATA to generate memory transactions
- `amd/timing/cu/scalarunit.go` — reads `Scratchpad().AsSMEM()` for Base/Offset
- `amd/timing/cu/scratchpadpreparer.go` — prepareFlat, prepareSMEM, prepareDS still write to scratchpad; commitFlat, commitDS still read from scratchpad

### Emu mode scratchpad usage (CAN FULLY REMOVE)
- `amd/emu/scratchpadpreparer.go` — all Prepare/Commit are no-ops, plus a clear() that's wasted
- `amd/emu/computeunit.go` — calls Prepare/Commit on every instruction (wasted)
- `amd/emu/wavefront.go` — allocates 4096-byte scratchpad (wasted)
- `amd/emu/scratchpad.go` — type + layout definitions (only needed by timing now)
- `amd/emu/inst.go` — `Scratchpad()` in InstEmuState interface (needed by timing)

## Notes
- M1-M5 complete: All instruction types migrated to ReadOperand/WriteOperand.
- M6 complete: Benchmark results documented (2x speedup for vector Prepare/Commit, 13.5% end-to-end).
- M7 (issue #200): Full removal of emu scratchpad + performance bottleneck investigation.
