# Roadmap: MI300A Timing Simulation Accuracy

## Goal
Achieve <20% average error and <50% max error for MI300A timing simulation vs real hardware measurements (120 CU config).

## Status
- M1.1 COMPLETE and VERIFIED — DRAM controller, WfPoolSize, VGPR fixes applied
- PR #251 open on sarchlab/mgpusim (not merged)
- Baseline accuracy measured: ~80% average error across 50 comparisons (most at tiny sizes)
- Alex's DRAM analysis complete: Akita DRAM model has fundamental HBM3 limitations
- Human suggested simplebankedmemory as DRAM replacement (issue #240)
- CP still registers wrong wfPoolSizes (10 vs 8) and vRegCounts (16384 vs 32768) — scheduling mismatch

## Completed Milestones

### M1: Baseline Infrastructure (cycles: 2) — FAILED
Too broad for execution team. Lesson: give concrete code changes, not research tasks.

### M1.1: Fix MI300A Timing Parameters (cycles: 4) — COMPLETE ✅
- Switched DRAM from idealmemcontroller to HBM timing model
- Fixed WfPoolSize from 10 to 8 per SIMD (MI300A-specific)
- Fixed VGPR count from 16384 to 32768 per SIMD (MI300A-specific)
- Benchmark scripts cherry-picked (compare_sim_vs_real.py, run_sim_benchmarks.sh)
- PR #251 created on sarchlab/mgpusim
- Accuracy: ~80% average error (most benchmarks at tiny sizes)

## Current Milestone

### M1.2: Switch to SimpleBankedMemory + Fix CP Mismatch + L2/Freq Tuning (cycles: 6)
Concrete changes needed on `ares/mi300a-timing-fixes` branch:

1. **Replace DRAM with SimpleBankedMemory** (in `mi300a/builder.go`):
   - Remove `dram` import, add `simplebankedmemory` import
   - Replace `createDramControllerBuilder()` with a function that builds `simplebankedmemory.Comp`
   - Parameters to start with (tunable):
     - `Freq: 1 * sim.GHz` (or match GPU freq 1.5 GHz)
     - `NumBanks: 16` per controller (16 controllers × 16 banks = 256 total banks)
     - `BankPipelineWidth: 1` 
     - `BankPipelineDepth: 1`
     - `StageLatency: 100` (100 ns at 1 GHz = 100 cycles; represents DRAM access latency)
     - `Log2InterleaveSize: 6` (64-byte cache-line interleaving)
   - Note: each controller already has its own storage slice; simplebankedmemory takes a Storage pointer

2. **Fix CP wfPoolSizes/vRegCounts mismatch** (in `connectCPWithCUs()`):
   - Change `wfPoolSizes: []int{10, 10, 10, 10}` → `[]int{8, 8, 8, 8}`
   - Change `vRegCounts: []int{16384, 16384, 16384, 16384}` → `[]int{32768, 32768, 32768, 32768}`

3. **Increase L2 cache size**: Change `l2CacheSize` from `8 * mem.MB` to `32 * mem.MB` (MI300A has 32 MB L2)

4. **Adjust GPU clock frequency**: Consider changing from 1500 MHz to 1700-1900 MHz

5. **Run accuracy comparison** on at least 5 benchmarks that work (matmul, spmv, floydwarshall, kmeans, stencil2d) at available sizes

## Upcoming Milestones

### M1.3: Fine-Tuning Memory Latency & Bandwidth (cycles: 4) — PENDING
- Tune simplebankedmemory parameters (bank count, pipeline depth, latency) based on M1.2 results
- Tune L1V bank latency
- Target: <30% average error

### M1.4: Final Accuracy & DRAM Report (cycles: 4) — PENDING
- Address individual benchmark outliers
- Create GitHub issue for human with DRAM evaluation findings (from Alex's analysis)
- Target: <20% average, <50% max

## Lessons Learned
- **M1 failure**: Don't give research tasks to execution teams. Do analysis in planning phase, give concrete code changes.
- **Tiny benchmarks mislead**: Testing at 64-element sizes doesn't reveal real accuracy. Need larger problem sizes.
- **DRAM model is broken for HBM3**: Akita's detailed DRAM model has fundamental issues. SimpleBankedMemory is a better approach per human.
- **CP/CU parameter mismatch**: The CP's view of CU resources must match the actual CU configuration. The M1.1 fix updated SA builder but not CP registration.
- **Budget**: M1.1 used 4 cycles (estimated 4). Being more conservative with M1.2 at 6 cycles.
