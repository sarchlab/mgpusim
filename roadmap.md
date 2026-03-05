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

## M9: Eliminate heap allocations in emulation hot path ✅
- Status: Complete (verified by Apollo, merged to main)
- Scope completed:
  1. ✅ ReadReg: Stack buffer `[8]byte` replaces `make([]byte, numBytes)`
  2. ✅ ReadOperand: Inlined register reads (VReg/SReg/SCC/VCC/EXEC/M0) return uint64 directly via binary.LittleEndian
  3. ✅ DS reads: Stack-allocated buffers outside lane loops
  4. ✅ flatAddr: Scalar base hoisted outside lane loop via flatPrecomputeScalarBase
- Estimated cycles: 6

## M10: Final evaluation and project completion ✅
- Status: Complete
- Final benchmarks: **2.07× end-to-end speedup** (gputensor: 2.86s → 1.38s)
- 0 heap allocations in all ReadOperand/WriteOperand hot paths
- Independent evaluator (Alex) confirmed project completeness
- Remaining optimizations (StorageAccessor.Read, logInst) deferred as diminishing returns

## PROJECT COMPLETE (Scratchpad Removal) ✅
- All 10 milestones achieved across ~222 cycles
- Human's request (issue #156) fully satisfied: scratchpad evaluated, removed, benchmarked

---

## M11: Fix CI lint failure (gocognit) — COMPLETE
- Code fix: extracted `readRegOperand` helper → lint passes
- Cycles: 3 (code was correct but CI retrigger took extra cycles)

## M11.1: Verify CI and merge PR #250 — FAILED (2/2 cycles)
- Status: FAILED — GCN3 emulation tests crash on PR #250
- Root cause discovered: **NOT a CI issue — real bug in ReadReg**
  - M9's heap allocation optimization changed `make([]byte, numBytes)` to `[8]byte` stack buffer
  - GCN3 flat_store_dwordx4 needs 16-byte reads → `buf[:16]` panics on `[8]byte`
  - Panic: `slice bounds out of range [:16] with length 8` at wavefront.go:246
  - Reproduced locally: AES benchmark crashes with same error

## M11.2: Fix ReadReg buffer overflow + merge PR #250 — NEXT
- Status: Defined (issue #226)
- Fix: Change `var buf [8]byte` to `var buf [32]byte` in `ReadReg` (wavefront.go:245)
- Then: push to both main and upstream/scratchpad-removal, verify CI, merge PR #250
- Estimated cycles: 2

### Lessons Learned
- Large file rewrites must be split across multiple workers
- GCN3 regression wasn't caught by unit tests — need integration-level testing
- Budget honestly: most milestones took ~50% more cycles than estimated
- Making functions no-ops first, then removing later, is a safe two-phase approach
- Iris's performance analysis was excellent — use dedicated analysis workers early
- Always check lint locally before pushing — the gocognit violation was introduced during optimization and not caught
- **Don't trust "CI passed on main" when the test matrix differs — upstream/main has different test coverage**
- **Stack buffer optimizations must account for maximum possible read size, not just common case**
- **Always reproduce failures locally before assigning to workers — saves cycles**
- **Previous Athena notes were WRONG about "code is identical" — the merge with upstream introduced new code paths that exposed the bug**
