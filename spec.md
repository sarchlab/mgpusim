# Project Specification

## What do you want to build?

Improve MGPUSim emulation performance by removing the scratchpad intermediary pattern from instruction execution. The scratchpad is a 4096-byte buffer that copies register operands before ALU execution and copies results back afterward. Profiling shows **88.5% of emulation time is scratchpad overhead** (Prepare + Commit), with only 11.5% doing actual computation.

The replacement design uses a `ReadOperand`/`WriteOperand` interface on `InstEmuState`, allowing ALU implementations to read and write wavefront registers directly without the intermediate copy.

## How do you consider the project is successful?

1. **All instruction formats migrated**: SOP1, SOP2, SOPC, SOPK, SOPP, SMEM, VOP1, VOP2, VOP3A, VOP3B, VOPC, FLAT, DS — all use ReadOperand/WriteOperand instead of scratchpad
2. **Scratchpad removed**: The `Scratchpad()` method, scratchpad layout structs, and `ScratchpadPreparer` code are fully removed
3. **No regressions**: All CI checks pass (lint, unit tests, emulation tests, timing tests, determinism tests, multi-GPU tests)
4. **Measurable speedup**: Emulation benchmarks show significant performance improvement (target: 3-5x faster)
5. **Clean implementation**: Code is maintainable, follows project conventions, passes lint

## Constraints

- Must maintain backward compatibility with both GCN3 and CDNA3 architectures
- Must keep timing simulation working (timing wavefront uses CURegFileAccessor)
- Migrate incrementally — keep `Scratchpad()` until all formats are migrated, then remove
- Must pass all CI checks at each milestone
- Prefer small, verifiable milestones (one instruction format family per milestone)

## Resources

- Existing `ReadOperand`/`WriteOperand` interface already implemented (M2 complete)
- All scalar instructions (SOP1/SOP2/SOPC/SOPK/SOPP/SMEM) already migrated
- Devon's scratchpad usage map: full inventory of all 471+ call sites
- Morgan's performance baseline: detailed profiling data for before/after comparison
- Kai's design proposal: architecture for the replacement

## Notes

- Scalar instructions (M2) demonstrated the pattern works — same approach for vector/memory
- Vector instructions have 64 lanes, so ReadOperand/WriteOperand must handle per-lane access
- FLAT and DS instructions interact with memory (storageAccessor) — migration must preserve this
- Timing path has 5 non-ALU scratchpad consumers (coalescer, scalar unit) that need special handling
- The `clear()` call alone (zeroing 4096 bytes) accounts for 27.6% of instruction execution time
