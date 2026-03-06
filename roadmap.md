# Roadmap: MI300A Timing Simulation Accuracy

## Goal
Achieve <20% average error and <50% max error for MI300A timing simulation vs real hardware measurements (120 CU config).

## Evaluation Methodology
- Reference data: `gpu_perf_scripts/mi300a_120cu.csv`
- **Exclude sizes where real HW time < 0.01ms** (kernel launch overhead dominated)
- Error = |sim_time - real_time| / real_time

## Current Status (Cycle 254)
- M1.1 COMPLETE ✅ — DRAM controller, WfPoolSize, VGPR fixes. PR #251 merged.
- M1.2 COMPLETE ✅ — SimpleBankedMemory, CP fix, L2=32MB, freq=1800MHz
- M1.3 COMPLETE ✅ — DRAM parameter fix (depth=20, stageLatency=5). PR #253 open on upstream.
- Investigation phase complete (cycle 253-254). Harper and Emma delivered analysis.

### Current Accuracy (meaningful sizes only, real HW > 0.01ms):
| Benchmark | Size | Sim (ms) | Real (ms) | Error |
|---|---|---|---|---|
| matmul | 128³ | 0.034 | 0.024 | +41% |
| matmul | 256³ | 0.089 | 0.040 | +120% |
| floydwarshall | 32 nodes | 0.083 | 0.156 | −47% |
| floydwarshall | 64 nodes | 0.168 | 0.302 | −44% |

**Average error: 63%** — Target: <20%
**Max error: 120%** — Target: <50%

### Root Causes Identified (Harper's Analysis)
1. **L1V + VectorMemUnit double-counting:** L1V BankLatency=60 + VectorMemUnit transactionPipeline=60 = ~129 cycles for L1V hit. Real CDNA3 TCP: ~30-40 cycles. **Primary cause of matmul being too slow.**
2. **L2 too fast:** L2 bankLatency=10 (default), dirLatency=0 (default, bypassed). L2 hit ~10 cycles vs real ~100-150 cycles. **Primary cause of floydwarshall being too fast.**
3. **L1V MSHR bottleneck:** Only 16 MSHR entries. At 256³ matmul with near-100% L1V miss rate, MSHRs saturate creating serial bottleneck.

### MMU Page Fault Status (Emma's Analysis)
- Garbage 64-bit addresses (upper 32 bits = 0xFFFFFFFF) from vector memory ops
- WithAutoPageAllocation is NOT safe (secondary crash)
- Root cause may be kernel arg/metadata paging issue — needs further investigation
- **Blocks testing at larger sizes** but current 4 data points are sufficient for M1.4

## Completed Milestones

### M1.1: Fix MI300A Timing Parameters (4 cycles) — COMPLETE ✅
- DRAM controller switch, WfPoolSize=8, VGPR=32768. PR #251 merged.

### M1.2: Switch to SimpleBankedMemory + Fix CP + L2/Freq (6 cycles) — COMPLETE ✅
- SimpleBankedMemory, CP wfPoolSizes/vRegCounts, L2=32MB, freq=1800MHz

### M1.3: Fix SimpleBankedMemory Parameters (4 cycles) — COMPLETE ✅
- BankPipelineDepth=20, StageLatency=5, TopPortBuf=64, PostPipelineBuf=4
- PR #253 open, GitHub issue #252 created for DRAM eval

## Next Milestone

### M1.4: Fix Cache Timing — L1V Double-Count + L2 Latency (6 cycles)
**Concrete changes:**
1. L1V BankLatency: 60 → 20 (shaderarray/builder.go:459)
2. VectorMemUnit transactionPipeline depth: 60 → 10 (cu/cubuilder.go:237)
3. L2: add WithBankLatency(50) (mi300a/builder.go, L2 build section)
4. L2: add WithDirectoryLatency(4) (mi300a/builder.go, L2 build section)
5. L1V NumMSHREntry: 16 → 32 (shaderarray/builder.go:463)

**Expected impact (Harper's estimates):**
- matmul 128³: +41% → +5% to +15%
- matmul 256³: +120% → +20% to +40%
- floydwarshall 64: −44% → −10% to −20%

**Verification:** Re-run benchmarks with run_sim_benchmarks.sh, compare to mi300a_120cu.csv.

### M1.5: Final Accuracy Tuning + MMU Fix (TBD cycles)
- Fine-tune based on M1.4 results
- Investigate and fix MMU page faults to test at larger sizes
- Target: <20% avg, <50% max

## Lessons Learned
- **M1 failure**: Don't give research tasks to execution teams. Do analysis in planning phase, give concrete code changes.
- **Tiny benchmarks mislead**: Testing at tiny sizes doesn't reveal real accuracy. Need sizes where real HW > 0.01ms.
- **DRAM model broken for HBM3**: Akita's detailed DRAM model has fundamental HBM3 issues. SimpleBankedMemory works better.
- **CP/CU parameter mismatch**: CP's view of CU resources must match actual CU configuration.
- **SimpleBankedMemory math**: bandwidth = controllers × banks × (1/stageLatency) × freq × 64B
- **Budget tracking**: M1.1=4cy, M1.2=6cy, M1.3=4cy, Investigation=2cy.
- **Opposite errors require root cause analysis**: MatMul too slow, FloydWarshall too fast. Can't just uniformly adjust — Harper's analysis found specific double-counting and L2 latency issues.
- **Double-counting is subtle**: VectorMemUnit has its own 60-cycle pipeline AND L1V has 60-cycle bank pipeline. Both are meant to model L1V access latency, but they stack.
- **Investigation before coding**: Spending 2 cycles on investigation (Harper + Emma) paid off with concrete, justified parameter changes instead of guessing.
