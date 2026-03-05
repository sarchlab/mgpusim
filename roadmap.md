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
- **Known regression**: gputensor test fails (GCN3 flat_load gets vAddr=0x0 from VGPRs written by migrated VOP instructions)

### Lessons Learned
- Large file rewrites must be split across multiple workers
- GCN3 regression wasn't caught by unit tests — need integration-level testing

## M4: Migrate flat and DS instructions from scratchpad ✅
- Status: Complete (merged to gfx942_emu, commit 07d9ccee)
- Cycles: 6 estimated, ~6 actual
- All flat/DS migrated for both GCN3 and CDNA3
- Timing flat/DS preparer restored (timing coalescer depends on scratchpad)

## M5: Fix GCN3 gputensor regression from M3 (next)
- Status: Pending
- The gputensor benchmark panics with "page not found: vAddr=0x0" in GCN3 flat_load
- Root cause: some GCN3 VOP instruction(s) migrated in M3 produce incorrect VGPR results (addresses end up as 0x0)
- Need to: (1) identify which VOP instruction writes the address VGPR incorrectly, (2) fix the ReadOperand/WriteOperand migration, (3) verify gputensor passes

## M6: Performance benchmarking
- Status: Future
- Compare before/after emulation performance
- Validate that the scratchpad removal actually improves performance
