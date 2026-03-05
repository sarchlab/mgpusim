# Roadmap

## M1: Compile first batch of benchmarks to gfx942 HSACO ✅
- Status: Complete
- Cycles: 3 estimated, 3 actual

## M2: Migrate scalar instructions from scratchpad to ReadOperand/WriteOperand ✅
- Status: Complete (PR #21 merged)
- Cycles: 8 estimated, ~12 actual (needed sub-milestones M2.1, M2.2)

## M3: Migrate vector instructions from scratchpad to ReadOperand/WriteOperand ❌ (deadline missed)
- Status: 95% complete — only `amd/emu/cdna3/vop3a.go` (57 Scratchpad calls) remains
- Cycles: 8 estimated, 8 used (deadline missed)
- Branch: `ares/m3-vector-migration`
- Completed: GCN3 VOP1/VOP2/VOP3A/VOP3B/VOPC, CDNA3 VOP1/VOP2/VOP3B/VOPC, timing no-ops
- Remaining: CDNA3 vop3a.go migration

### Lessons Learned
- Sam was assigned the CDNA3 vop3a.go migration 4 times but timed out each time trying to rewrite the entire 1600-line file in one shot.
- **Key takeaway**: Large file rewrites must be split across multiple workers or done incrementally. One agent cannot rewrite 1600 lines in a single cycle.
- The milestone scope was appropriate but the remaining work should have been split earlier when Sam first timed out.

## M3.1: Complete CDNA3 vop3a.go migration (next)
- Status: Pending
- Estimated cycles: 4
- Goal: Migrate remaining 57 Scratchpad() calls in `amd/emu/cdna3/vop3a.go`, build/test, open PR
- Strategy: Split the file into halves — two workers each handling ~28 functions. One adds helper functions and migrates comparison functions (lines 1-800), other migrates arithmetic/packed functions (lines 800-1600).

## M4: Migrate flat and DS instructions from scratchpad
- Status: Future
- Both GCN3 and CDNA3 flat.go and ds.go still use Scratchpad
- Also needs timing preparer updates

## M5: Performance benchmarking
- Status: Future
- Compare before/after emulation performance
- Validate that the scratchpad removal actually improves performance
