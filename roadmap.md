# MI300A Timing Accuracy — Roadmap

## Goal
Achieve average error <20% and max error <50% (symmetrical: `(sim-hw)/min(sim,hw)`) across MI300A benchmarks.

## Current State (Cycle 274)

### Completed Work (merged to `gfx942_emu` / main)
- CDNA3 emulation support (gfx942 ISA, VOP3P, etc.)
- DRAM controller switch (ideal → SimpleBankedMemory)
- Wavefront pool size fix (10→8)
- VGPR count fix (16384→32768)
- L1V cache timing (bankLatency 60→20, MSHR 16→32)
- L2 cache timing (bankLatency=50, dirLatency=4)
- VecMem pipeline latency reduction (60→10)

### Pending Work (PR #256 in upstream, not merged)
- SIMD width 32 for CDNA3 (NumSinglePrecisionUnit)
- VecMem instruction pipeline (6→2) and transaction pipeline (default 60, CDNA3 4)
- All CI passes on PR #256

### Known Benchmark Errors (from M2 CU fixes, 120CU hardware ref)
| Benchmark | Sim (ms) | HW (ms) | Error |
|---|---|---|---|
| matmul 128³ | 0.0282 | 0.0243 | +16.2% |
| matmul 256³ | 0.0772 | 0.0403 | +91.5% |
| kmeans 1024p/32f/5c | 0.0991 | 0.0177 | +460.0% |
| floydwarshall 64 | 0.1585 | 0.3024 | −47.6% |
| nw 128 | 0.1183 | 0.1285 | −8.0% |

**Note**: These errors use the old formula `(sim-hw)/hw`. Need to recalculate with symmetrical formula `(sim-hw)/min(sim,hw)`.

## Open Human Issues
- **#262**: Human asks about source of "CDNA3 has 32 SP units per SIMD" — needs verification
- **#264**: Use symmetrical error formula `(sim-hw)/min(sim,hw)` — need to update comparison scripts
- **#266**: Keep development in origin, don't merge in upstream

## Milestones

### M1: Infrastructure & Baseline [COMPLETED]
- Set up benchmark scripts and comparison tools
- DRAM, cache, wfPoolSize, VGPR fixes
- Established baseline error measurements

### M2: CU Compute Pipeline Fixes [IN PROGRESS — PR pending]
- SIMD width 32 for CDNA3
- VecMem pipeline depth reduction
- **Status**: PR #256 open in upstream, CI all green. Need to merge into dev branch.

### M3: Comprehensive Error Baseline (NEXT)
- Update error calculation to symmetrical formula (issue #264)
- Run ALL available benchmarks on current simulator (with M2 fixes)
- Identify which benchmarks work and which crash/timeout
- Establish complete error table as the baseline for further optimization
- Verify SIMD width=32 claim against AMD ISA documentation (issue #262)

### M4: Address Top Error Contributors
- Based on M3 baseline, identify and fix highest-error benchmarks
- Investigate kmeans extreme error (+460%)
- Investigate floydwarshall underestimation (-47.6%)
- Address matmul scaling issues (128³ OK at +16%, 256³ bad at +91%)

### M5: Fine-Tuning and Final Verification
- Optimize remaining parameters to bring avg <20%, max <50%
- Full benchmark suite verification

## Lessons Learned
- Making changes architecture-specific (GCN3 vs CDNA3) is critical — global changes break GCN3
- The VecMem transaction pipeline default must remain 60 for GCN3, only overridden for CDNA3
- Always verify CI on both GCN3 and CDNA3 test suites
- Small problem sizes can hide scaling errors (matmul 128³ at +16% but 256³ at +91%)
- The symmetrical error formula `(sim-hw)/min(sim,hw)` will produce LARGER numbers than the old formula for positive errors
