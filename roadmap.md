# Roadmap: MI300A Timing Simulation Accuracy

## Goal
Achieve <20% average error and <50% max error for MI300A timing simulation vs real hardware measurements (120 CU config).

## Status
- M1.1 COMPLETE and VERIFIED — DRAM controller, WfPoolSize, VGPR fixes applied. PR #251 merged.
- M1.2 code changes applied on `ares/mi300a-timing-fixes` (not yet in upstream):
  - Switched to SimpleBankedMemory, fixed CP wfPoolSizes/vRegCounts, increased L2 to 32MB, changed freq to 1800 MHz
  - BUT SimpleBankedMemory parameters are wrong: stageLatency=100, depth=1 gives only ~164 GB/s — **22x too low** for MI300A's ~3600 GB/s observed bandwidth
- Accuracy at small sizes: ~280% average error (5 benchmarks, tiny problem sizes)
- Small sizes are dominated by kernel launch overhead and sim overhead — not meaningful for accuracy evaluation

## Completed Milestones

### M1: Baseline Infrastructure (cycles: 2) — FAILED
Too broad for execution team. Lesson: give concrete code changes, not research tasks.

### M1.1: Fix MI300A Timing Parameters (cycles: 4) — COMPLETE ✅
- Switched DRAM from idealmemcontroller to HBM timing model
- Fixed WfPoolSize from 10 to 8 per SIMD (MI300A-specific)
- Fixed VGPR count from 16384 to 32768 per SIMD (MI300A-specific)
- Benchmark scripts cherry-picked (compare_sim_vs_real.py, run_sim_benchmarks.sh)
- PR #251 created and merged on sarchlab/mgpusim

### M1.2: Switch to SimpleBankedMemory + Fix CP + L2/Freq (cycles: 6) — IN VERIFICATION
- Code changes applied on branch but memory parameters are inadequate
- Needs parameter fix before PR creation

## Current Milestone

### M1.3: Fix SimpleBankedMemory Parameters + Run Accuracy Benchmarks (cycles: 4)
The SimpleBankedMemory parameters from M1.2 are critically wrong. The current config gives ~164 GB/s bandwidth when MI300A has ~3600 GB/s observed. This is the primary accuracy bottleneck.

**Required changes:**
1. Fix SimpleBankedMemory parameters in `mi300a/builder.go`:
   - `BankPipelineDepth`: 1 → 20 (models ~100ns HBM3 access latency)
   - `StageLatency`: 100 → 5 (yields ~3277 GB/s with 256 total banks)
   - `TopPortBufferSize`: 16 (default) → 64 (prevent port congestion)
   - `PostPipelineBufSize`: 1 (default) → 4 (prevent pipeline stalls)
   - Keep: Freq=1GHz, NumBanks=16, BankPipelineWidth=1, Log2InterleaveSize=6
   - Math: 16 controllers × 16 banks × (1/5 items/cycle) × 1GHz × 64 bytes = 3277 GB/s

2. Run benchmarks at meaningful sizes (where real HW time > 0.01ms):
   - matrixmultiplication: 256×256, 512×512
   - stencil2d: 512×512, 1024×1024
   - floydwarshall: 64, 128, 256 nodes
   - vectoradd: 1M, 4M, 16M elements
   - spmv: various larger sizes

3. Create PR on upstream with all M1.2+M1.3 changes

4. Create GitHub issue on sarchlab/mgpusim titled "HUMAN: Akita DRAM Model Evaluation for HBM3" with Alex's analysis findings

## Upcoming Milestones

### M1.4: Fine-Tuning Based on Accuracy Results (cycles: 6) — PENDING
- Tune SimpleBankedMemory parameters (stage latency, bank count) based on M1.3 benchmark results
- Tune L1V bank latency if needed
- Address individual benchmark outliers
- Target: <20% average, <50% max error

## Lessons Learned
- **M1 failure**: Don't give research tasks to execution teams. Do analysis in planning phase, give concrete code changes.
- **Tiny benchmarks mislead**: Testing at 64-element sizes doesn't reveal real accuracy. Need larger problem sizes where real hardware > 0.01ms.
- **DRAM model is broken for HBM3**: Akita's detailed DRAM model has fundamental issues. SimpleBankedMemory is a better approach per human.
- **CP/CU parameter mismatch**: The CP's view of CU resources must match the actual CU configuration.
- **SimpleBankedMemory math matters**: With depth=1 + stageLatency=100, each bank processes 1 req per 100 cycles. With 256 banks at 1GHz × 64B = only 164 GB/s. Need depth=20 + stageLatency=5 for correct bandwidth (~3277 GB/s) and latency (~100ns).
- **Budget**: M1.1 used 4 cycles (estimated 4). M1.2 used 6 cycles but with wrong parameters. Being more conservative going forward.
