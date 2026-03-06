# MI300A Timing Simulation — Roadmap

## Goal
Average symmetrical error < 20%, max < 50% across MI300A benchmarks.

## Current Status (Cycle 275)

### Work Done (on upstream/gfx942_emu and related branches)
- **M1 (merged in upstream PR #254)**: Basic MI300A timing config — frequency 1800MHz, 240 CUs, 32MB L2, SimpleBankedMemory DRAM, wfPoolSize=8, vgprCount=32768, L1V/L2 cache tuning
- **M2 (upstream PR #256, NOT merged)**: CU pipeline changes — SIMD width 32 (WRONG), VecMem pipeline depth reduction (inst=2, trans=4)

### Critical Findings
1. **SIMD width must be 16, not 32** — confirmed by CDNA3 ISA doc (TRAP_STS, LDS dispatch, 4-cycle forwarding). PR #256's SIMD change is incorrect.
2. **Error formula changed** to symmetrical: `(sim-hw)/min(sim,hw)` per human directive.
3. **All development must stay in origin** (dev repo), not upstream.

### Current Accuracy (from iris's baseline on ares/m2-cu-pipeline-clean, WITH SIMD=32)
- Excluding buggy benchmarks (stencil2d, nbody): avg |sym error| = ~45%, median ~26%
- stencil2d: ~7x overestimate (simulator bug)
- nbody: returns same time for all sizes (simulator bug)
- floydwarshall: sim ~2x too fast
- matmul: grows from +6% at small sizes to +119% at large sizes
- MMU page fault crashes limit coverage to small problem sizes

### When SIMD is corrected back to 16, compute times will roughly DOUBLE
This means benchmarks currently showing good accuracy may get worse. Need new baseline.

## Milestones

### M3: Revert SIMD to 16 + Establish Correct Baseline (NEXT)
- **Status**: Pending
- **Budget**: 6 cycles
- Revert SIMD width to 16 (keep VecMem pipeline depth changes)
- Update compare script to use symmetrical error formula  
- Run comprehensive benchmark baseline and document errors
- Fix nbody bug (same time for all sizes)
- All work on origin branches only
- **Acceptance**: Clean baseline report with correct SIMD=16 and sym error formula

### M4: Fix Stencil2D and Investigate Major Error Sources
- **Status**: Future
- Fix stencil2d ~7x overestimate
- Investigate and fix MMU page fault crashes for larger sizes
- Target: reduce avg error to ~30%

### M5: Fine-tune Parameters to Hit Target
- **Status**: Future  
- Tune DRAM, cache, pipeline parameters
- Target: avg sym error < 20%, max < 50%

## Lessons Learned
- Don't trust architectural assumptions without ISA documentation verification
- The SIMD=32 change showed "improvement" in some benchmarks but was based on wrong architecture understanding
- Always check CI on all architectures (GCN3 was broken initially by M2 changes)
- Development workflow: work in origin, create upstream PRs for review only
- Error formula matters — symmetrical error penalizes both over and underestimates more equally
