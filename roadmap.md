# Roadmap

## M1: Compile first batch of benchmarks to gfx942 HSACO ✅
- Status: Complete
- Cycles: 3 estimated, 3 actual

## M2: Migrate scalar instructions from scratchpad to ReadOperand/WriteOperand ✅
- Status: Complete (PR #21 merged)
- Cycles: 8 estimated, ~12 actual (needed sub-milestones M2.1, M2.2)

## M3: Migrate vector instructions from scratchpad to ReadOperand/WriteOperand ✅
- Status: Complete (merged to gfx942_emu)
- Cycles: 8 estimated + 4 for M3.1 = ~12 actual
- All VOP1/VOP2/VOP3A/VOP3B/VOPC migrated for both GCN3 and CDNA3

## M4: Migrate flat and DS instructions from scratchpad ✅
- Status: Complete (merged to gfx942_emu, commit 07d9ccee)
- Cycles: 6 estimated, ~6 actual

## M5: Fix GCN3 gputensor regression from M3 ✅
- Status: Complete (commit ebc4e7eb on gfx942_emu)
- Root cause: emu scratchpadpreparer Commit functions were overwriting correct register values with stale scratchpad data
- Fix: Made all VOP Prepare/Commit functions no-ops in emu scratchpadpreparer

## M6: Performance benchmarking ✅
- Status: Complete (commit e8767713 on gfx942_emu)
- Benchmark results: 2x speedup for vector instruction Prepare/Commit, 13.5% end-to-end
- Microbenchmarks and end-to-end results documented in docs/benchmark_results.md
- Zero heap allocations achieved in Prepare/Commit path

## M7: Remove emu scratchpadpreparer.go ✅ (Partial)
- Status: Partial — scratchpadpreparer.go deleted, Prepare/Commit calls removed from computeunit.go
- Remaining: scratchpad.go still in amd/emu/ (types needed by timing)

## M8: Delete scratchpad.go from emu + add instruction decode cache (NEXT)
- Goal: Complete issue #200 (move scratchpad types to timing, delete emu/scratchpad.go) AND implement issue #203 (decoded instruction table indexed by PC)
- Scope:
  1. Move Scratchpad type + all layout structs from `amd/emu/scratchpad.go` to `amd/timing/cu/` (or a shared sub-package)
  2. Update all timing imports to use the new location
  3. Remove `Scratchpad()` from `InstEmuState` interface in `amd/emu/inst.go` (or split interface)
  4. Delete `amd/emu/scratchpad.go`
  5. Clean up test mocks that reference Scratchpad
  6. Implement decoded instruction cache: map[uint64]*insts.Inst in the decoder or compute unit
  7. In `runWfUntilBarrier`, check cache by PC before calling Decode
  8. All tests pass
- Estimated cycles: 6

### Lessons Learned
- Large file rewrites must be split across multiple workers
- GCN3 regression wasn't caught by unit tests — need integration-level testing
- Budget honestly: most milestones took ~50% more cycles than estimated
- Making functions no-ops first, then removing later, is a safe two-phase approach
- Benchmarking should be done early — it builds confidence and provides data for decisions
- scratchpadpreparer.go was successfully deleted in M7 with no issues — the remaining scratchpad.go deletion is blocked by timing imports
