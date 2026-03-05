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

## M4: Migrate flat and DS instructions from scratchpad ✅
- Status: Complete (merged to gfx942_emu)
- Cycles: 6 estimated, ~6 actual

## M5: Fix GCN3 gputensor regression from M3 ✅
- Status: Complete
- Fix: Made all VOP Prepare/Commit functions no-ops in emu scratchpadpreparer

## M6: Performance benchmarking ✅
- Status: Complete
- Results: 2x speedup for vector Prepare/Commit, 13.5% end-to-end

## M7: Remove emu scratchpadpreparer.go ✅
- Status: Complete (scratchpadpreparer.go deleted, Prepare/Commit calls removed)

## M8: Delete scratchpad.go from emu + add instruction decode cache ✅
- Status: Complete and verified by Apollo
- Scratchpad type moved to timing/wavefront, emu/scratchpad.go deleted
- instCache map[uint64]*insts.Inst added to emu ComputeUnit

## M9: Eliminate heap allocations in emulation hot path (NEXT)
- Goal: Fix remaining performance bottlenecks identified by Iris
- Scope:
  1. **ReadReg**: Eliminate `make([]byte, numBytes)` allocation — read directly from register files and return uint64 or use fixed-size stack buffer
  2. **ReadOperand padding**: Eliminate `make([]byte, 8)` on line 99 of wavefront.go — use `[8]byte` stack buffer
  3. **DS reads**: Move `make([]byte, N)` outside lane loops in aluds.go and cdna3/ds.go — use stack-allocated `[16]byte`
  4. **flatAddr**: Hoist scalar base read outside the lane loop in alu_flat.go and cdna3/flat.go
  5. All tests pass
  6. Benchmark showing allocation reduction
- Estimated cycles: 6

## Future (after M9)
- StorageAccessor.Read buffer reuse
- logInst hook short-circuit
- End-to-end benchmark comparison with original baseline

### Lessons Learned
- Large file rewrites must be split across multiple workers
- GCN3 regression wasn't caught by unit tests — need integration-level testing
- Budget honestly: most milestones took ~50% more cycles than estimated
- Making functions no-ops first, then removing later, is a safe two-phase approach
- Iris's performance analysis was excellent — use dedicated analysis workers early
