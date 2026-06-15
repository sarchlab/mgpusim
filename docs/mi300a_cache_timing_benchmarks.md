# MI300A Cache Timing Fix Benchmark Results

**Branch:** `ares/mi300a-timing-fixes`  
**Commit:** `1e786a29` — [Finn] Fix cache timing: L1V bankLatency 60→20, MSHR 16→32, VecMem pipeline 60→10, L2 bankLatency=50 dirLatency=4  
**Date:** 2026-03-06  
**Hardware reference:** `gpu_perf_scripts/mi300a_120cu.csv` (real MI300A, 120 CU)  
**Flags:** `-timing -arch cdna3 -gpu mi300a -disable-rtm`

## Parameter Changes (5 total)

| # | Component | Parameter | Old Value | New Value | File |
|---|---|---|---|---|---|
| 1 | L1V Cache | BankLatency | 60 | 20 | `shaderarray/builder.go` |
| 2 | L1V Cache | NumMSHREntry | 16 | 32 | `shaderarray/builder.go` |
| 3 | VecMem Unit | Transaction Pipeline Latency | 60 | 10 | `cu/cubuilder.go` |
| 4 | L2 Cache | BankLatency | (default) | 50 | `mi300a/builder.go` |
| 5 | L2 Cache | DirectoryLatency | (default) | 4 | `mi300a/builder.go` |

**Rationale:** The L1V bank latency and VecMem pipeline latency were set too high (60 cycles each), causing excessive memory access overhead. The MSHR entry count was doubled (16→32) to allow more outstanding misses. L2 cache bank and directory latencies were explicitly set to prevent overestimation.

## Benchmark Results

### Comparison Table: Old (DRAM fix) vs New (cache timing fix) vs Real Hardware

| Benchmark | Old Sim (ms) | New Sim (ms) | Change | Real HW (ms) | Old Error | New Error | Improvement |
|---|---|---|---|---|---|---|---|
| matmul 128³ | 0.0344 | 0.0345 | +0.4% | 0.0243 | +41.4% | +41.9% | — |
| matmul 256³ | 0.0886 | 0.0884 | −0.2% | 0.0403 | +119.7% | +119.3% | +0.4pp |
| floydwarshall 32 | 0.0827 | 0.0856 | +3.6% | 0.1563 | −47.1% | −45.2% | +1.9pp |
| floydwarshall 64 | 0.1678 | 0.1830 | +9.0% | 0.3024 | −44.5% | −39.5% | +5.0pp |

*(pp = percentage points improvement in error; "Old Sim" values from `docs/mi300a_dram_fix_benchmarks.md`, commit `86383354`)*

### Detailed New Results

| Benchmark | Problem Size | Sim kernel_time (s) | Sim (ms) | Real HW (ms) | Error (%) |
|---|---|---|---|---|---|
| matmul | 128×128×128 | 3.4478e-05 | 0.0345 | 0.0243 | +41.9% |
| matmul | 256×256×256 | 8.8388e-05 | 0.0884 | 0.0403 | +119.3% |
| floydwarshall | 32 nodes | 8.5630e-05 | 0.0856 | 0.1563 | −45.2% |
| floydwarshall | 64 nodes | 1.8299e-04 | 0.1830 | 0.3024 | −39.5% |

## Analysis

### Impact by Benchmark Type

**Matrix Multiplication (compute-bound):** The cache timing changes had negligible impact (~0.2–0.4%) on matmul. This is expected — matmul at these sizes is dominated by ALU computation and the reduced cache latencies don't significantly affect the critical path. The error remains high (+42% to +119%) because the simulator overestimates compute time for this workload.

**Floyd-Warshall (memory-access-heavy):** The cache timing fix showed measurable improvement:
- 32 nodes: error improved from −47.1% to −45.2% (+1.9 percentage points)
- 64 nodes: error improved from −44.5% to −39.5% (+5.0 percentage points)

The increased simulation time (+3.6% to +9.0%) moves floyd-warshall closer to real hardware values (sim was previously too fast). The larger improvement at 64 nodes suggests the cache parameter changes have more impact at higher memory pressure.

### Error Direction Summary

| Benchmark | Error Direction | Meaning |
|---|---|---|
| matmul | Positive (+42% to +119%) | Simulator too slow (overpredicts time) |
| floydwarshall | Negative (−39% to −45%) | Simulator too fast (underpredicts time) |

### What the 5 Changes Do

1. **L1V BankLatency 60→20:** Reduces per-access latency to L1 vector cache. 60 cycles was far too pessimistic for a small 16KB SRAM cache. 20 cycles is more realistic for a single-bank L1.

2. **L1V MSHR 16→32:** Doubles the number of outstanding cache misses the L1V can track simultaneously. This allows more memory-level parallelism, which is important for memory-intensive workloads.

3. **VecMem Pipeline 60→10:** Reduces the vector memory unit's transaction pipeline latency from 60 to 10 stages. The original 60 was excessively conservative and added artificial stall cycles to every memory operation.

4. **L2 BankLatency=50:** Explicitly sets L2 cache bank access latency. Without this, the default may have been too high for the MI300A's 32MB L2.

5. **L2 DirectoryLatency=4:** Sets the L2 directory lookup latency to 4 cycles, ensuring tag checks are fast and don't bottleneck cache hit responses.

### Combined Effect of All Fixes (from baseline)

Comparing the cumulative effect of all timing fixes on this branch (DRAM fix + cache timing fix):

| Benchmark | Pre-Fix Sim (ms)¹ | Post-All-Fixes Sim (ms) | Total Change | Real HW (ms) |
|---|---|---|---|---|
| matmul 128³ | 0.0451 | 0.0345 | −23.5% | 0.0243 |
| floydwarshall 32 | 0.0826 | 0.0856 | +3.6% | 0.1563 |

¹ Pre-fix values from issue #243 verification benchmarks (before DRAM parameter fix)

## Remaining Accuracy Gaps

1. **matmul still +42% to +119% too slow** — the compute pipeline model may overestimate instruction latency or underestimate ILP
2. **floydwarshall still −39% to −45% too fast** — memory subsystem may undercount stall cycles for dependent memory accesses, or the kernel dispatch model may be too optimistic
3. **Stencil2D ~600% error** (from prior benchmarks) — not re-tested here but remains the largest known accuracy gap
4. **MMU page faults** prevent testing at larger, more realistic problem sizes

## Files Modified

- `amd/samples/runner/timingconfig/shaderarray/builder.go` — L1V BankLatency, NumMSHREntry
- `amd/timing/cu/cubuilder.go` — VecMem pipeline latency
- `amd/samples/runner/timingconfig/mi300a/builder.go` — L2 BankLatency, DirectoryLatency
