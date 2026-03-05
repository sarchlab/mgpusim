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

## M7: Fully remove emu scratchpad + performance optimization (NEXT)
- Status: Defining
- Goal: Per human issue #200 — fully remove scratchpad prepare/commit from emu package, investigate and fix additional performance bottlenecks
- Scope:
  1. Remove emu ScratchpadPreparer (entire file `amd/emu/scratchpadpreparer.go`)
  2. Remove Prepare/Commit calls from `executeInst` in `amd/emu/computeunit.go`
  3. Remove scratchpad field from emu Wavefront (save 4KB per wavefront)
  4. Move scratchpad.go layout types to timing package (only timing uses them)
  5. Handle InstEmuState interface: either keep Scratchpad() for timing compatibility or restructure
  6. Investigate other emulation performance bottlenecks (e.g., instruction decode, memory access patterns)
  7. Run benchmarks to verify improvement
- Estimated cycles: 8
- Risk: Interface changes may have wide-reaching effects on timing package

### Lessons Learned
- Large file rewrites must be split across multiple workers
- GCN3 regression wasn't caught by unit tests — need integration-level testing
- Budget honestly: most milestones took ~50% more cycles than estimated
- Making functions no-ops first, then removing later, is a safe two-phase approach
- Benchmarking should be done early — it builds confidence and provides data for decisions
