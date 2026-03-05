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
- gputensor tests pass, all emu/cdna3 tests pass

## M6: Performance benchmarking and final cleanup (next)
- Status: Pending
- Goal: Benchmark emulation performance before vs. after scratchpad removal
- Compare gfx942_emu (after) against the pre-migration baseline (main branch, commit 6d36b99d)
- Use gputensor operator tests and/or emu unit tests as benchmark targets
- Write Go benchmark functions if needed
- Report results (expected: meaningful speedup from avoiding scratchpad copy overhead)
- Estimated cycles: 6

### Lessons Learned
- Large file rewrites must be split across multiple workers
- GCN3 regression wasn't caught by unit tests — need integration-level testing
- The emu scratchpadpreparer Prepare/Commit still gets called (just does nothing) — full removal could yield more gains
- Budget honestly: most milestones took ~50% more cycles than estimated
