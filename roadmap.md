# Roadmap: MI300A Timing Simulation Accuracy

## Goal
Achieve <20% average error and <50% max error for MI300A timing simulation vs real hardware measurements (120 CU config).

## Evaluation Methodology
- Reference data: `gpu_perf_scripts/mi300a_120cu.csv`
- **Exclude sizes where real HW time < 0.01ms** (kernel launch overhead dominated)
- Error = |sim_time - real_time| / real_time

## Current Status (Cycle 253)
- M1.1 COMPLETE ✅ — DRAM controller, WfPoolSize, VGPR fixes. PR #251 merged.
- M1.2 COMPLETE ✅ — SimpleBankedMemory, CP fix, L2=32MB, freq=1800MHz
- M1.3 COMPLETE ✅ — DRAM parameter fix (depth=20, stageLatency=5). PR #253 open on upstream.

### Current Accuracy (meaningful sizes only, real HW > 0.01ms):
| Benchmark | Size | Sim (ms) | Real (ms) | Error |
|---|---|---|---|---|
| matmul | 128³ | 0.034 | 0.024 | +41% |
| matmul | 256³ | 0.089 | 0.040 | +120% |
| floydwarshall | 32 nodes | 0.083 | 0.156 | −47% |
| floydwarshall | 64 nodes | 0.168 | 0.302 | −44% |

**Average error: 63%** — Target: <20%
**Max error: 120%** — Target: <50%

### Key Blockers
1. **MMU page faults** at moderate sizes (vecadd ≥32K, stencil ≥1024², floyd ≥128 nodes)
2. **MatMul sim too slow** — 1.4-2.2× overestimate, gets worse at larger sizes
3. **FloydWarshall sim too fast** — consistently 1.8× underestimate
4. **L1V BankLatency=60** — likely too high (default is 20)

## Completed Milestones

### M1.1: Fix MI300A Timing Parameters (4 cycles) — COMPLETE ✅
- DRAM controller switch, WfPoolSize=8, VGPR=32768. PR #251 merged.

### M1.2: Switch to SimpleBankedMemory + Fix CP + L2/Freq (6 cycles) — COMPLETE ✅
- SimpleBankedMemory, CP wfPoolSizes/vRegCounts, L2=32MB, freq=1800MHz

### M1.3: Fix SimpleBankedMemory Parameters (4 cycles) — COMPLETE ✅
- BankPipelineDepth=20, StageLatency=5, TopPortBuf=64, PostPipelineBuf=4
- PR #253 open, GitHub issue #252 created for DRAM eval

## Current Phase: Investigation (Athena planning)
Investigating root causes of timing errors before defining M1.4.
- Issue #248: Investigate matmul +120% and floydwarshall -44% errors
- Harper investigating cache/timing parameters
- Emma investigating MMU page fault for potential workaround

## Upcoming Milestones

### M1.4: Parameter Tuning + MMU Fix (TBD cycles)
Pending investigation results. Expected areas:
- L1V BankLatency reduction (60 → 10-20?)
- MMU page fault workaround (enable autoPageAllocation?)
- L1V cache size/associativity tuning
- L2 latency tuning
- Need more benchmarks at larger sizes to validate

### M1.5: Final Accuracy Tuning (TBD cycles)
- Fine-tune based on M1.4 results
- Run comprehensive benchmark suite
- Target: <20% avg, <50% max

## Lessons Learned
- **M1 failure**: Don't give research tasks to execution teams. Do analysis in planning phase, give concrete code changes.
- **Tiny benchmarks mislead**: Testing at tiny sizes doesn't reveal real accuracy. Need sizes where real HW > 0.01ms.
- **DRAM model broken for HBM3**: Akita's detailed DRAM model has fundamental HBM3 issues. SimpleBankedMemory works better.
- **CP/CU parameter mismatch**: CP's view of CU resources must match actual CU configuration.
- **SimpleBankedMemory math**: bandwidth = controllers × banks × (1/stageLatency) × freq × 64B
- **Budget tracking**: M1.1=4cy, M1.2=6cy, M1.3=4cy. Investigation phase started cycle 253.
- **Opposite errors**: MatMul too slow, FloydWarshall too fast. Can't just uniformly adjust — need targeted parameter changes.
- **MMU page faults**: Crash at moderate sizes blocks testing. Akita MMU has `WithAutoPageAllocation(true)` as potential workaround.
