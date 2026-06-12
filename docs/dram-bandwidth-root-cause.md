# Root Cause Analysis: Memory-Bandwidth-Bound Benchmark Outliers

## 1. Executive Summary

Six memory-bandwidth-bound benchmarks (adjust_weights, devicememory_read, global_bw_copy, gelu, nn, triad) account for 45.5% of total simulation error, with the simulator running 3–77× slower than real MI300A hardware. The root cause is a combination of insufficient L1V cache miss-level parallelism (only 16 MSHRs per CU), an inflated DRAM row-miss penalty (52 cycles vs. HBM3's ~14 cycles), and excessive L2-to-DRAM interconnect latency (20 cycles vs. ~5 cycles for on-interposer HBM3). Together, these bottlenecks limit effective simulated memory bandwidth to roughly 1.5–2 TB/s, far below MI300A's ~5.3 TB/s HBM3 capability.

## 2. Affected Benchmarks

| Kernel | Problem Size | Real (ms) | Sim (ms) | Error (%) | Slowdown |
|--------|-------------|-----------|----------|-----------|----------|
| adjust_weights | 4096 | 0.0056 | 0.4303 | 7585% | 76.8× |
| adjust_weights | 2048 | 0.0058 | 0.2115 | 3547% | 36.5× |
| adjust_weights | 1024 | 0.0058 | 0.0783 | 1250% | 13.5× |
| devicememory_read | 2MB | 0.0054 | 0.2553 | 4627% | 47.3× |
| devicememory_read | 1MB | 0.0055 | 0.0674 | 1125% | 12.2× |
| nn | 2097152 | 0.0092 | 0.1238 | 1246% | 13.5× |
| nn | 1048576 | 0.0067 | 0.0558 | 733% | 8.3× |
| global_bw_copy | 4194304 | 0.0063 | 0.0551 | 774% | 8.7× |
| global_bw_copy | 2097152 | 0.0052 | 0.0292 | 461% | 5.6× |
| gelu | n16777216 | 0.0393 | 0.2102 | 435% | 5.3× |
| triad | 4194304 | 0.0160 | 0.0809 | 406% | 5.1× |
| triad | 2097152 | 0.0103 | 0.0421 | 309% | 4.1× |

## 3. Root Cause Analysis

### 3a. L1V Cache MSHRs — Primary Bottleneck

**Configuration (before fix):**
- 16 MSHRs per L1V cache
- 32 max concurrent transactions
- 16-entry top/bottom port buffers
- 120 CUs total (20 shader arrays × 6 CUs)

**Analysis:**
The L1V cache MSHR count directly limits memory-level parallelism (MLP) per compute unit. With only 16 MSHRs, each CU can have at most 16 outstanding cache misses. At a typical round-trip latency of ~119 cycles (L1→L2→DRAM→L2→L1), the maximum bandwidth per CU is:

```
16 misses × 64 bytes / 119 cycles × 1.75 GHz ≈ 15 GB/s per CU
```

Total system bandwidth: 120 CUs × 15 GB/s = **~1.8 TB/s**

This is only 34% of MI300A's actual ~5.3 TB/s HBM3 bandwidth. The small port buffers (16 entries) further restricted throughput by creating backpressure on high-traffic CUs.

**Real hardware:** CDNA3 vector caches support 64+ outstanding misses per CU.

### 3b. DRAM Row-Miss Penalty — Secondary Bottleneck

**Configuration (before fix):**
- Row miss delay: 52 cycles at 1 GHz
- Row buffer size: 2 KB (2^11 bytes)
- 16 DRAM controllers × 16 banks each

**Analysis:**
The 52-cycle row-miss penalty corresponds to approximately tRCD + tRP = 52 ns, which is characteristic of DDR5 DIMM memory, not HBM3. HBM3 has significantly lower timing parameters:
- tRCD ≈ 14 ns (row-to-column delay)
- tRP ≈ 14 ns (row precharge)
- Combined open-page miss: ~28 ns; closed-page: ~14 ns

For streaming workloads with sequential access patterns, the row buffer hit rate is high (~97% for 64-byte accesses into 2 KB row buffers), so the row-miss penalty has moderate per-access impact. However, at the start of each new row and during random access patterns, the 3.7× inflated penalty accumulates significantly.

### 3c. L2-to-DRAM Interconnect Latency — Tertiary Bottleneck

**Configuration (before fix):**
- Latency: 20 cycles at 1.75 GHz = 11.4 ns one-way
- Round-trip contribution: ~40 cycles (22.8 ns)

**Analysis:**
On MI300A, the HBM3 stacks are connected to the compute dies via an on-interposer silicon bridge with very short physical distance. The realistic latency is approximately 2–3 ns one-way (5 cycles at 1.75 GHz). The 20-cycle configuration added an unnecessary ~34 cycles to each memory round-trip, reducing effective bandwidth by approximately 29% at 16 outstanding misses per CU.

### 3d. Small Problem Sizes Amplify the Error

Several of the worst outliers use small problem sizes (e.g., adjust_weights 4096 = only 4096 elements). With 120 CUs, this creates only ~64 wavefronts, meaning most CUs are idle or have insufficient work to hide memory latency. The simulator's overhead for kernel launch, dispatch, and wavefront scheduling becomes proportionally large relative to the actual computation, amplifying percentage errors.

## 4. Changes Implemented

The following physically-motivated changes were made:

| Parameter | Before | After | Justification |
|-----------|--------|-------|---------------|
| DRAM row miss delay | 52 cycles | 14 cycles | HBM3 tRCD ≈ 14 ns |
| L2-to-DRAM latency | 20 cycles | 5 cycles | On-interposer HBM3 ~3 ns |
| L1V MSHRs | 16 | 64 | CDNA3 hardware capability |
| L1V max concurrent trans | 32 | 128 | Match increased MSHR count |
| L1V port buffer size | 16 | 64 | Remove per-CU backpressure |

**Files modified:**
- `amd/samples/runner/timingconfig/mi300a/builder.go` — DRAM and interconnect parameters
- `amd/samples/runner/timingconfig/shaderarray/builder.go` — L1V cache parameters

## 5. Expected Impact

### Bandwidth Improvement

With 64 MSHRs and reduced round-trip latency (~79 cycles vs. ~119 cycles):

```
64 misses × 64 bytes / 79 cycles × 1.75 GHz ≈ 91 GB/s per CU
Total: 120 CUs × 91 GB/s ≈ 10.9 TB/s (theoretical max)
```

In practice, L2 cache bandwidth, DRAM controller queuing, and interconnect contention will limit the effective bandwidth to below this theoretical maximum. The target is to bring simulated bandwidth within 2× of MI300A's ~5.3 TB/s, which should reduce the worst-case errors from 7585% to under 2000%.

### Risk Assessment

- **Gate benchmarks (atax, bicg, tiled_gemm_16):** These are compute-bound or cache-sensitive workloads. The L1V MSHR and DRAM changes primarily affect memory-bandwidth-bound kernels. Gate benchmarks should not regress significantly because their performance is dominated by ALU throughput and cache hit rates, not DRAM bandwidth.
- **Over-correction risk:** The increased MLP could make some memory-bound workloads simulate faster than real hardware. This is acceptable at this stage — the prior 50–75× slowdown was far more problematic than a potential 2× speedup.
