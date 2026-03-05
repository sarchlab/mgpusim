# Spec

## What do you want to build

Remove the scratchpad abstraction from the emulator to improve emulation performance. The scratchpad prepares operand data for ALU units and collects results, providing a clean interface but adding performance overhead. The alternative is to let the wavefront struct hold emulation register and LDS data, with the emulator ALU directly interfacing with the wavefront to read/write register data via ReadOperand/WriteOperand.

**Phase 2 (issue #200):** Fully remove the scratchpad prepare and commit functions from the emu package — not just make them no-ops, but delete the entire ScratchpadPreparer, Scratchpad type, and all related code. Also investigate and fix any unreasonable delays or performance bottlenecks in the emulation process.

**Phase 3 (issue #203):** Implement a decoded instruction table/cache indexed by PC. Instead of re-decoding instructions every time they're fetched, cache decoded `*Inst` objects by their PC address and return cached results on subsequent lookups. GPU programs don't self-modify, so this is safe and eliminates decode overhead (especially in loops).

## How do you consider the project is success

1. ✅ All instruction implementations use ReadOperand/WriteOperand instead of Scratchpad.
2. ✅ All emu Prepare/Commit functions are no-ops.
3. ✅ All tests pass.
4. ✅ Benchmarks show emulation performance improvement (2x for vector instructions, 13.5% end-to-end).
5. The emu `scratchpad.go` file is deleted. Scratchpad type and layout structs are moved to the timing package (only timing uses them).
6. The `Scratchpad()` method is removed from the `InstEmuState` interface (or restructured so emu doesn't need it).
7. The `executeInst` function in emu computeunit.go no longer calls Prepare/Commit. ✅ (Already done)
8. A decoded instruction cache/table indexed by PC is implemented in the emu decoder, avoiding repeated decode of the same instruction.
9. All tests continue to pass after all changes.

## Constraints
- Must not break existing GCN3 or CDNA3 emulation functionality.
- Must not break timing simulation (timing scratchpadpreparer still uses scratchpad for Flat/DS/SMEM coalescing).
- The `Scratchpad()` method may need to remain in the timing wavefront's implementation. The approach should move the Scratchpad type to the timing package or a shared location accessible by timing.
- GPU programs do not self-modify, so caching decoded instructions by PC is safe.

## Architecture Notes

### Timing mode scratchpad usage (MUST KEEP)
- `amd/timing/cu/defaultcoalescer.go` — reads `Scratchpad().AsFlat()` for EXEC, ADDR, DATA to generate memory transactions
- `amd/timing/cu/scalarunit.go` — reads `Scratchpad().AsSMEM()` for Base/Offset
- `amd/timing/cu/scratchpadpreparer.go` — prepareFlat, prepareSMEM, prepareDS still write to scratchpad; commitFlat, commitDS still read from scratchpad
- `amd/timing/wavefront/wavefront.go` — has Scratchpad field and method

### Emu mode scratchpad usage (BEING REMOVED)
- `amd/emu/scratchpadpreparer.go` — ✅ DELETED
- `amd/emu/computeunit.go` — ✅ Prepare/Commit calls removed
- `amd/emu/wavefront.go` — ✅ scratchpad field removed, `Scratchpad()` returns nil
- `amd/emu/scratchpad.go` — type + layout definitions still here (needed by timing via import)
- `amd/emu/inst.go` — `Scratchpad()` in InstEmuState interface (timing wavefront implements it)

### Instruction decode path (for cache optimization)
- `amd/emu/computeunit.go:runWfUntilBarrier` — fetches 8 bytes, calls `cu.decoder.Decode(instBuf)` every iteration
- `amd/emu/decoder.go` — `Decoder` interface with `Decode(buf []byte) (*insts.Inst, error)`
- `amd/insts/disassembler.go` — actual decode logic, allocates `new(Inst)` + multiple `new(Operand)` per call
- Cache should map PC → decoded *Inst, check cache before calling Decode

## Notes
- M1-M6 complete: All instruction types migrated to ReadOperand/WriteOperand. Benchmarked.
- M7 partial: scratchpadpreparer.go deleted. Still need to delete scratchpad.go (move types to timing).
- M8 (issue #203): Decoded instruction table indexed by PC.
