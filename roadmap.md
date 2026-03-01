# Roadmap: GFX942 (CDNA3) Kernel Emulation

## Goal
Support byte-level correct emulation of a wide range of gfx942 HIP kernels across benchmark suites: SHOC, PolyBench, Rodinia, Parboil, and others. Each benchmark needs:
1. HIP source code compiled to gfx942 HSACO (V5 code object)
2. Go reference implementation for result comparison
3. Byte-level correct emulated results
4. Acceptance tests runnable in GitHub Actions CI

## Current State (as of M3.1 PARTIAL merge)
- CDNA3 ALU emulator exists (~4000 lines in `amd/emu/cdna3/`)
- V5 HSACO loading works
- **10 of 12 attempted benchmarks pass** with `-arch=cdna3 -verify`:
  - **M1 (7)**: vectoradd, memcopy, matrixtranspose, floydwarshall, fastwalshtransform, fir, simpleconvolution
  - **M2 (4)**: bitonicsort, kmeans, atax, bicg (merged in PR #4)
  - **M3.1 (1)**: relu (merged in PR #5)
  - **Deferred (2)**: nbody (multi-workgroup LocalPtr complexity), aes (verification mismatch)
- Dual-arch pattern established: each benchmark embeds both GCN3 and gfx942 HSACOs
- Docker-based HIP compilation workflow established
- All benchmarks maintain GCN3 backward compatibility (12/12 GCN3 tests passing)
- CDNA3 kernarg struct layout pattern established: hidden args, proper padding, exact offset matching
- Code quality issues identified: debug logging and .orig file need cleanup (M3.2)

## Milestones

### M1: Compile existing benchmarks to gfx942 and verify emulation (first batch)
**Budget**: 8 cycles  
**Status**: ✅ COMPLETE (cycle 2)  
**Scope**: matrixtranspose, floydwarshall, fastwalshtransform, simpleconvolution, fir — all pass `-arch=cdna3 -verify` and maintain GCN3 compatibility.

### M2: Add gfx942 support to second batch of existing benchmarks
**Budget**: 8 cycles  
**Status**: ⚠️ PARTIAL (4/5 benchmarks working, 8/8 cycles used)  
**Scope**: atax, bicg, bitonicsort, matrixmultiplication, kmeans

**Result**: 4/5 benchmarks pass `-arch=cdna3 -verify` and merged to main via PR #4. matrixmultiplication deferred.

**Sub-milestones**:

#### M2.1.1: Fix CDNA3 FLAT SAddr and matrixmultiplication kernarg bugs
**Budget**: 2 cycles  
**Status**: ✅ COMPLETE
- Implemented FLAT SAddr (scalar base addressing) in scratchpadpreparer.go
- Fixed matrixmultiplication CDNA3KernelArgs struct layout
- Verified atax passes -arch=cdna3 -verify
- Regression tested M1 benchmarks

#### M2.1.2: Implement VOP3P packed instructions and merge M2 benchmarks
**Budget**: 2 cycles  
**Status**: ✅ COMPLETE
- Implemented VOP3P packed instructions (v_pk_mul_f32, v_pk_add_f32, v_pk_fma_f32) with OpSel handling
- FLAT SAddr fix also resolved bicg (was failing due to scalar base addressing, not VOP3P)
- **Result: 4/5 benchmarks work** (bitonicsort, kmeans, atax, bicg)
- matrixmultiplication still has value mismatches

#### M2.2: Clean up branch and merge 4 working benchmarks
**Budget**: 1 cycle  
**Status**: ✅ COMPLETE
- Removed 8 scratch files and 35+ build artifacts
- Updated .gitignore to prevent build artifacts
- Fixed lint violations (funlen, unconvert)
- Reverted incomplete matrixmultiplication CDNA3 changes to fix GCN3 CI
- **PR #4 merged**: bitonicsort, kmeans, atax, bicg all pass CI on both GCN3 and CDNA3

#### M2.3: Fix matrixmultiplication
**Status**: DEFERRED
**Reason**: Deep bug requiring more investigation than remaining budget allowed. Better ROI to move forward with new benchmarks.

### M3: Add gfx942 support to remaining existing benchmarks
**Budget**: 8 cycles (split into sub-milestones)  
**Status**: In progress  
**Scope**: bfs, fft, spmv, stencil2d, aes, pagerank, nbody, relu, nw

**Approach**: Based on M2 lessons, use smaller milestones (2-3 benchmarks each) for faster feedback.

#### M3.1: Add CDNA3 support for relu and nbody
**Budget**: 3 cycles  
**Status**: ⚠️ PARTIAL (1/2 benchmarks working, 3/3 cycles used)  
**Scope**: Get relu and nbody benchmarks working with -arch=cdna3 -verify

**Result**: relu passes `-arch=cdna3 -verify` and is complete. nbody deferred due to multi-workgroup LocalPos buffer allocation complexity.

**Cycle 1 Progress**:
- Leo: ✅ Compiled HIP to gfx942 HSACO (both benchmarks)
- Maya: ✅ Added dual-arch support (both benchmarks)
- Niko: ✅ Implemented missing opcodes (v_fma_f32, v_cmp_le_f32, v_cmp_class_f32, v_div_scale_f32, v_div_fmas_f32, v_div_fixup_f32, vcclo/vcchi fallback)
- **BLOCKED**: Both benchmarks fail with kernarg/memory issues:
  - nbody: "page not found in page table" during FLAT memory access
  - relu: runs but outputs all zeros
- Regression testing incomplete due to failures

**Cycle 2 Progress** (Morgan's investigation, issue #47):
- Root cause analysis completed for all failing benchmarks
- relu: Missing hidden kernel args in CDNA3KernelArgs struct (needs 280 bytes total, had 48)
- nbody: LocalPos field type mismatch (should be driver.Ptr, was driver.LocalPtr) + missing hidden args (needs 312 bytes, had 72)
- bicg: Has erroneous Padding field shifting hidden args +4 bytes (works with small inputs, fails with x=512, y=512)
- Detailed investigation report with exact struct layouts created

**Cycle 3 Progress**:
- Maya: ✅ Fixed relu CDNA3KernelArgs struct with correct 280-byte layout (commit 13143e63)
- Riley: ✅ Fixed bicg padding bug (commit 4486b140)
- Maya: Attempted nbody fix with correct struct layout (commits 87b8eaf1, 2b6b1cf4)
- **nbody decision**: Deferred due to multi-workgroup LocalPos buffer allocation complexity beyond M3.1 scope
- Maya: ✅ Reverted nbody to GCN3-only to maintain working state (commit 1f6fa2bc)
- Leo: ✅ Cleaned up debug code and leftover files (commit 00e99ccf)

**Deferred Work**:
- nbody CDNA3 support requires solving multi-workgroup local memory buffer allocation pattern where HIP converts `__local` to `__global` pointer. This is a deeper architectural issue that warrants separate investigation rather than blocking M3 progress.

#### M3.2: Code quality cleanup
**Budget**: 1 cycle  
**Status**: ✅ COMPLETE (cycle 92, PR #6 merged)
**Scope**: Remove technical debt that slipped through M3.1 merge

**Completed tasks**:
1. ✅ Removed debug logging from `amd/benchmarks/polybench/atax/benchmark.go:191-193` (Maya, commit dad81c45)
2. ✅ Removed .orig file `amd/insts/decodetable.go.orig` (Leo, commit 29dda4fd)
3. ✅ Added `*.orig` to .gitignore (Leo, commit 29dda4fd)
4. ✅ PR #6 merged to main (2026-03-01)

**Outcome**: All code quality issues resolved. Clean codebase ready for next milestone.

#### M3.3: Add CDNA3 support to stencil2d and pagerank
**Budget**: 2 cycles (extended to 3)
**Status**: ❌ FAILED (deadline missed, 3/3 cycles used, incomplete)
**Scope**: Add gfx942 CDNA3 emulation to stencil2d and pagerank benchmarks

**What Happened:**
- Cycle 1: Leo and Maya implemented dual-arch for both benchmarks. Pagerank ✅ works. Stencil2d ❌ page fault.
- Cycle 2: Leo tried LocalPtr → Ptr fix. Still page fault.
- Cycle 3: Leo rewrote kernel with __shared__ memory. Revealed missing opcode 510 (v_add_lshl_u32).

**Result**: Pagerank complete, stencil2d blocked on missing opcode.

**Root Cause Analysis (Athena investigation):**
1. **Branch isolation**: Opcode 510 was already implemented on `ares/cdna3-aes` branch but Ares started from stale main
2. **V2/V3 header bug**: `isV2V3Header()` in hsaco.go false-positives on V5 kernels, strips first 256 bytes of instructions
3. **Workflow dysfunction**: Multiple parallel branches (ares/cdna3-aes, ares/cdna3-benchmarks-m2) contain fixes that never merged to main

**Key Finding**: We have ~11-12 working benchmarks across all branches, but main only shows 10. The problem is integration velocity, not implementation capability.

**Lessons Learned:**
- **Branch debt accumulates**: 3+ unmerged feature branches created duplication of effort
- **Systemic bugs go undetected**: V2/V3 header bug is a time bomb affecting any V5 kernel
- **Starting from main is risky**: Main lacks recent fixes from parallel branches
- **Integration must be continuous**: Treating merge as milestone endpoint causes branch proliferation

**Deferred items**:
- nbody: multi-workgroup local memory allocation (architectural gap)
- matrixmultiplication: CDNA3 flat memory page table bug
- AES: non-deterministic output (investigation complete on ares/cdna3-aes, 6 bugs fixed but AES still broken)

#### M3.4: Branch Consolidation & Technical Debt Cleanup
**Budget**: 4 cycles  
**Status**: Not started  
**Scope**: Merge valuable feature branches, fix V2/V3 header bug, verify all benchmarks

**Rationale**: M3.3 failure revealed workflow dysfunction. Multiple branches contain fixes that should be in main:
- `ares/cdna3-aes`: 6 bug fixes + 2 opcodes (510, 164)
- `ares/cdna3-benchmarks-m2`: working stencil2d with __shared__ memory
- V2/V3 header detection bug is systemic, affects any V5 kernel

Merging these unlocks 11-12 working benchmarks (vs 10 on main) and prevents future duplication of effort.

**Tasks**:
1. Merge ares/cdna3-aes to main (issue #98)
2. Merge ares/cdna3-benchmarks-m2 to main (issue #99)
3. Fix V2/V3 header detection bug in hsaco.go (issue #100)
4. Comprehensive testing of all 13 benchmarks (issue #101)
5. Update roadmap to reflect actual state
6. Audit and close stale branches

**Success Criteria**:
- All valuable branches merged to main
- V2/V3 bug fixed without regressions
- 11-12 benchmarks passing on main (up from 10)
- CI green
- Clean branch list (≤3 active branches)
- Documentation reflects actual state

**Expected Outcome**:
- Stencil2d working (opcode 510 from ares/cdna3-aes + fix from ares/cdna3-benchmarks-m2)
- Pagerank working (already complete)
- All M1/M2/M3.1 benchmarks still working
- Clean foundation for adding remaining benchmarks efficiently

### M4: Add Parboil benchmarks (CUDA→HIP conversion)
**Budget**: 10 cycles  
**Status**: Not started  
**Scope**: Identify Parboil benchmarks, convert CUDA→HIP, compile to gfx942, write Go reference, get emulation passing.

### M5: Expand SHOC/PolyBench/Rodinia/additional coverage
**Budget**: 10 cycles  
**Status**: Not started  
**Scope**: Add benchmarks from these suites not already covered; find and integrate additional benchmark suites.

## Lessons Learned

### M1 (Cycles 1-2)
- **Dual-arch pattern**: embed both GCN3 and gfx942 HSACOs, load conditionally based on Arch field. Move loadProgram() to Run() so Arch is set before loading.
- **KernelArgs layout**: gfx942 V5 code objects use hidden kernel args (8 bytes each for X/Y/Z offsets). The struct must match exactly — use disassembly to verify argument offsets.
- **V5 code objects**: Different kernel descriptor layout than V4. Work-item IDs are packed in v0 for gfx942.
- **Opcode shifts**: GCN3 and gfx942 have different opcode numbers for the same instructions. Never run GCN3 HSACO through CDNA3 emulator.
- **File naming on macOS**: macOS is case-insensitive, so HIP source files can't have same basename as existing OpenCL files (even different case). Use `_hip` suffix to avoid collisions.
- **extern "C" kernels**: Use `extern "C" __global__` to prevent C++ name mangling, matching kernel names expected by Go code.

### M2 (Cycles 1-8, partial success)
- **Batch size matters**: M2 benchmarks were 7x more complex than M1 (0.5 vs 3.5 benchmarks/cycle). 5-benchmark batches too ambitious.
- **Hidden complexity**: Advanced instruction features (FLAT SAddr, VOP3P) discovered only when benchmarks fail. Need to expect unknowns.
- **Smaller batches = better**: 2-3 benchmarks per milestone provides faster feedback and lower risk.
- **FLAT SAddr addressing**: When `global_load v, vgpr, sgpr_pair` uses scalar base (SAddr != 0x7F), the address is `sgpr_pair + zero_extend(vgpr)`, not a 64-bit VGPR pair. Scratchpad preparer must handle this.
- **Kernel arg fragility**: Each benchmark needs struct layout verification against HSACO metadata. Misaligned fields (wrong size/offset) cause silent corruption or page faults.
- **Sequential discovery**: Fixing memory faults often reveals missing opcodes later in the kernel. Budget cycles for 'unknown unknowns' when bringing up new kernels.
- **Sunk cost fallacy danger**: After 4 cycles on M2, natural to keep investing. But 2 benchmarks with deep bugs are a trap.
- **Know when to pivot**: 4/5 working is good enough to ship. Better to learn from NEW benchmarks than stay stuck on old ones.
- **Code hygiene matters**: Branch accumulated 8 scratch files + 35 build artifacts. Must clean before merge or it becomes permanent debt.
- **CI checks must pass**: Code quality checks aren't optional - fix lint violations before merge.
- **Deadlines need finer slicing**: A 5-benchmark mixed milestone was too large; split by failure type (opcode gaps vs memory faults).

### M3.1 (Cycles 1-3, partial success: 1/2 benchmarks)
- **Recurring patterns are red flags**: Third occurrence of "all zeros" or "page not found" bugs (matrixmultiplication, relu, nbody). This is a systematic issue, not isolated bugs.
- **Stop and investigate**: When the same type of bug appears repeatedly, STOP adding benchmarks and find the root cause. Adding more failing benchmarks doesn't teach us anything new.
- **Successful opcode implementation ≠ working benchmark**: All M3.1 opcodes were implemented correctly, but benchmarks still fail. The problem is deeper (kernarg layout, memory addressing).
- **Need systematic debugging methodology**: Can't just keep trying benchmarks and hoping they work. Need proper investigation tools and process.
- **KernelArgs layout must match HSACO metadata exactly**: Hidden argument/padding mismatch causes hard-to-debug runtime errors. Morgan's investigation found exact root causes for relu, nbody, and bicg failures.
- **Root cause analysis pays off**: Dedicated investigation cycle (Morgan) provided clear struct layouts and fix patterns that enabled quick fixes for relu and bicg.
- **Know when to defer vs. fix**: relu and bicg were straightforward struct fixes. nbody requires multi-workgroup local memory allocation pattern that's beyond current scope.
- **Ship what works**: 1/2 working (relu) is better than 0/2 stuck in investigation. Defer nbody and move forward with other benchmarks.
- **Revert cleanly**: When deferring work, revert to a known-good state (GCN3-only) rather than leaving broken CDNA3 code in the repo.
- **LocalPtr vs Ptr semantics**: HIP's conversion of `__local` to `__global` means some benchmarks need different memory allocation patterns than OpenCL equivalents.
- **CI must validate milestone acceptance**: Relying only on local spot checks allows late regressions and misses human-requested acceptance automation.

### M3.3 (Cycles 1-3, failed: 1/2 benchmarks, deadline missed)
- **Branch isolation kills productivity**: Ares spent 3 cycles "discovering" opcode 510 was missing, when it was already implemented on ares/cdna3-aes. Starting from stale main means missing recent fixes.
- **Integration velocity < implementation velocity**: We can build features fast, but branches pile up because merging is treated as a milestone endpoint, not a continuous process.
- **Unmerged branches = duplicated effort**: Every day fixes sit on feature branches increases risk of rediscovering the same issues.
- **Systemic bugs need immediate investigation**: V2/V3 header detection bug is a time bomb that can hit any V5 kernel. Devon found it affects stencil2d because kernel bytes coincidentally match V2/V3 signature, causing emulator to strip first 256 bytes of real instructions.
- **Workarounds hide bugs**: Rewriting stencil2d to use __shared__ memory works around V2/V3 bug but doesn't fix it. Workarounds accumulate tech debt.
- **Branch audit reveals hidden value**: Kai's investigation of ares/cdna3-aes found 6 critical bug fixes (SOPK, VOPC VCC, SDWA) that benefit ALL benchmarks, not just AES. These should have been merged immediately.
- **Know when to stop adding features**: When workflow dysfunction blocks progress, FIX THE WORKFLOW before adding more features. We proved we can implement benchmarks (10 working!). Now prove we can integrate them efficiently.
- **Merge small, merge often**: Feature branch lifespan should be days, not weeks. Merge bug fixes immediately, don't wait for milestones.
