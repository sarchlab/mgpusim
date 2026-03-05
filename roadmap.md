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

## M11: Fix CI lint failure (gocognit) — FAILED (3/3 cycles used)
- Status: Code fix complete, but CI verification incomplete
- Issue: `ReadOperand` cognitive complexity 52 → fixed by extracting `readRegOperand` helper
- Code is on main and passes all CI (run 22722972753 — Lint success)
- PR #250 CI was stale (ran on pre-fix commit e10826ba, not the fixed code)
- Apollo flagged funlen violation but main CI passed — false positive from local check
- Estimated cycles: 2, Actual cycles: 3 (failed due to CI retrigger issues, not code)

## M11.1: Verify CI passes on PR #250 and merge — NEXT
- Status: Planning
- The code fix is already on main and scratchpad-removal (identical `wavefront.go`)
- New CI run triggered (run 22733965555 on SHA 7f7d5a7b)
- Need: Wait for CI to pass, verify all checks green, merge PR #250
- Estimated cycles: 1

### Lessons Learned (continued)
- Large file rewrites must be split across multiple workers
- GCN3 regression wasn't caught by unit tests — need integration-level testing
- Budget honestly: most milestones took ~50% more cycles than estimated
- Making functions no-ops first, then removing later, is a safe two-phase approach
- Iris's performance analysis was excellent — use dedicated analysis workers early
- Always check lint locally before pushing — the gocognit violation was introduced during optimization and not caught
- **NEW: Don't trust local lint checks over CI results — CI on main passed, local check was wrong about funlen**
- **NEW: GitHub Actions sometimes doesn't trigger on push — always verify CI runs were created for the right SHA**
- **NEW: When code is on main and passing CI, the PR branch just needs a retrigger, not more code changes**
