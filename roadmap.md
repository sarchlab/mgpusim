# MI300A Timing Simulation — Roadmap

## Goal
Average symmetrical error < 20%, max < 50% across MI300A benchmarks.

## Current Status

### Completed Milestones

**M1** (merged): Basic MI300A timing config
- Frequency 1800MHz, 240 CUs, 32MB L2, SimpleBankedMemory DRAM
- wfPoolSize=8, vgprCount=32768
- L1V: bankLatency=20, MSHR=32
- L2: bankLatency=50, dirLatency=4

**M2** (merged): CU pipeline changes
- VecMem pipeline depth: inst=2, trans=4
- SIMD width confirmed as 16 (reverted incorrect change to 32)

**M3** (merged, PR #27): Correct baseline established
- Reverted SIMD width to 16
- Fixed nbody bug (numBodies calculation)
- Updated compare script to symmetrical error formula
- 33 benchmark data points across 8 benchmarks documented

### M3 Baseline Results (Current Accuracy)
| Benchmark | Avg |Error| | Direction | Key Issue |
|-----------|-------------|-----------|-----------|
| fir | 11.7% | too fast | Only 1 data point |
| relu | 21.9% | too fast | Small sizes only |
| nw | 27.2% | too slow | Error grows with size (4%→67%) |
| vectoradd | 46.2% | too fast | Sim misses HW overhead floor |
| matmul | 66.2% | too slow | Error grows with size (17%→119%) |
| floydwarshall | 75.4% | too fast | |
| nbody | 138.8% | too fast | Scaling correct, absolute ~2.5× off |
| stencil2d | 678.4% | too slow | Sim time constant ~0.048ms regardless of size |

**Overall: avg 155.6%, median 66.7%**

### Critical Blockers
1. **MMU page-not-found panics** crash at larger problem sizes (vectoradd ≥32K, matmul ≥640, etc.)
2. **stencil2d** has ~8× overhead — sim time constant regardless of problem size

## Planned Milestones

### M4: Investigate and Fix Stencil2D + MMU Crashes (NEXT)
- **Budget**: 8 cycles
- Investigate why stencil2d sim time is constant ~0.048ms (dispatch overhead?)
- Investigate MMU page-not-found panics blocking large sizes
- Fix stencil2d timing to match HW scaling behavior
- If MMU fix is feasible, implement it; otherwise document root cause
- **Acceptance**: stencil2d error < 100%, at least one benchmark runs at larger sizes

### M5: Improve Compute-Heavy Benchmarks (nbody, matmul, floydwarshall)
- Target nbody absolute timing (currently 2.5× too fast)
- Target matmul scaling issue (error grows with size)
- Investigate memory bandwidth modeling for compute-heavy workloads
- **Budget**: 8 cycles

### M6: Parameter Tuning and Final Optimization
- Fine-tune DRAM, cache, pipeline parameters
- Target: avg sym error < 20%, max < 50%
- **Budget**: 8 cycles

## Lessons Learned
- SIMD=32 was incorrect — always verify against ISA documentation
- Symmetrical error penalizes both over and underestimates more equally
- Small problem sizes are dominated by kernel launch overhead, not compute
- Development must stay in origin repo, not upstream
- Page-not-found crashes severely limit the range of testable problem sizes
- Stencil2d constant timing suggests simulator overhead (dispatch/launch) dominates at small sizes
