# MI300A Timing Model Calibration Report

## Overview

This document records the calibration decisions for the MI300A timing model in MGPUSim.
Each parameter is documented with its source, rationale, and impact on accuracy.

**Branch:** `ares/m10-dram-fix-and-ci` (builds on M9.1)  
**Date:** 2026-03-07 (M9.1), 2026-03-08 (M10 DRAM fix)  

---

## Compute Unit Parameters

### NumSinglePrecisionUnits = 16

**Source:** AMD CDNA3 ISA Reference (GFX942)

Each CDNA3 Compute Unit contains 4 SIMDs, and each SIMD has **16 FP32 ALUs**. This is confirmed by:
- AMD CDNA3 ISA documentation: each SIMD unit processes one 64-wide wavefront using 16 FP32 ALUs over 4 cycles
- AMD Instinct MI300 series white paper: CDNA3 architecture maintains 16 FP32 ALUs per SIMD

**Impact:** Setting SPU=32 (as was done in M9) caused compute-bound benchmarks like FastWalshTransform to run ~2x too fast. SPU=16 corrects this. MatrixMultiplication error increased slightly (from ~2% to ~5%) but remains excellent.

### VecMemInstPipelineStages = 2

**Source:** Estimated based on CDNA3 CU pipeline depth.

The vector memory instruction pipeline processes memory load/store instructions. 2 stages allows one memory instruction to be issued every 2 cycles per SIMD.

### VecMemTransPipelineStages = 4

**Source:** Estimated. The transaction pipeline handles address generation and memory request formation.

---

## Cache Parameters

### L1 Vector Cache: 32 KB, Bank Latency = 5 cycles

**Source:** AMD Instinct MI300A specifications.

- L1 vector cache per CU: 32 KB (confirmed by AMD documentation)
- Bank latency of 5 cycles at 1.8 GHz ≈ 2.8 ns access time, consistent with published L1 cache latencies for CDNA3

### L2 Cache: 32 MB total, Bank Latency = 20 cycles

**Source:** AMD Instinct MI300A specifications (32 MB L2 per XCD).

- L2 bank latency of 20 cycles at 1.8 GHz ≈ 11.1 ns
- Previous value of 50 cycles was too conservative; 20 cycles better matches published L2 access latencies
- Organized as 16 banks with 16-way set associativity

---

## Memory Pipeline

### MemPipelineBufferSize = 32

**Source:** Calibrated parameter. Default was 8.

This controls the buffer size on connections between CU, ROB, Address Translator, and L1V cache. It limits how many concurrent memory transactions each CU can have in-flight.

**Rationale:** With bufSize=8, each CU can only have 8 outstanding memory requests, severely limiting throughput for streaming workloads. AMD CDNA3 CUs are designed to have many more in-flight memory requests to hide latency. 32 was chosen to better match observed memory bandwidth utilization on real hardware.

**Impact:** Improves accuracy for memory-bandwidth-sensitive benchmarks (vectoradd, relu) at medium sizes, though large sizes still show significant error due to other bottlenecks.

---

## DRAM Configuration

### M10 Fix: BankPipelineWidth = 1, BankPipelineDepth = 10, StageLatency = 3

**Source:** Calibrated to match MI300A HBM3 specifications. Verified by source code analysis of SimpleBankedMemory (issue #352, Iris's analysis).

**Previous values (M9.1):** BPW=4, depth=20, SL=1 → 65.5 TB/s simulated (12× too fast)

**New values (M10):** BPW=1, depth=10, SL=3 → 5.46 TB/s simulated (matches 5.3 TB/s real)

#### Pipeline Throughput Analysis (SimpleBankedMemory source code)

The `Pipeline` in `pipelining/pipeline.go` has `[width][numStage]` slots. Steady-state throughput per lane is `1/cyclePerStage` requests/cycle. Per-bank throughput is `width/cyclePerStage`.

| Metric | M9.1 (old) | M10 (new) | Real MI300A |
|--------|------------|-----------|-------------|
| Per-bank throughput | 4/1 = 4 req/cycle | 1/3 = 0.333 req/cycle | — |
| Per-controller BW (16 banks × 64B) | 4,096 GB/s | 341 GB/s | ~331 GB/s |
| Total system BW (16 controllers) | 65.5 TB/s | 5.46 TB/s | 5.3 TB/s |
| Access latency (depth × SL) | 20 ns | 30 ns | 30–40 ns |

#### Evidence

1. **AMD MI300A HBM3 bandwidth:** 5.3 TB/s aggregate (8 stacks × 8-Hi HBM3, per AMD Instinct MI300 Series datasheet)
2. **HBM3 access latency:** ~30-40 ns (per JEDEC HBM3 specification and published measurements)
3. **Source code verification:** `SimpleBankedMemory.finalizeBanks()` drains `postPipelineBuf` entries, each producing a 64-byte response. Pipeline accept rate = `width` items per `cyclePerStage` cycles (confirmed by `Pipeline.CanAccept()` and `Pipeline.Accept()` in `pipelining/pipeline.go`)
4. **Per-controller calculation:** 16 banks × (1/3 req/cycle) × 64 bytes/req × 1 GHz = 341 GB/s. 16 controllers × 341 GB/s = 5,461 GB/s ≈ 5.46 TB/s

#### Impact

The previous config provided essentially unlimited DRAM bandwidth (65 TB/s), meaning memory-bound workloads were bottlenecked only by CU-side pipeline limits. This was incorrect for streaming workloads (vectoradd, relu) where DRAM bandwidth should be the limiting factor at high occupancy. The fix correctly introduces DRAM as a bandwidth constraint matching real hardware.

---

## Kernel Launch Overhead

### constantKernelLaunchOverhead = 5400 cycles (all kernels, no /2 for subsequent)

**Source:** Calibrated against real hardware measurements (Harper's FWT analysis, M13).

- All kernel launches: 5400 cycles at 1.8 GHz = 3.0 μs
- Real MI300A kernel launch overhead is estimated at 2-5 μs

**Rationale:** FWT requires ~5 μs per kernel launch on real MI300A hardware, but was only getting ~2.6 μs due to the `/2` halving applied to subsequent kernels. Harper's analysis shows that subsequent kernels need the same overhead as the first kernel launch on MI300A — the `/2` was an assumption that subsequent launches are cheaper, which doesn't match real hardware data. The halving has been removed so all kernels (first and subsequent) use the full 5400-cycle overhead.

**History:** Previously, subsequent kernels used `constantKernelLaunchOverhead / 2` (2700 cycles = 1.5 μs). This was removed in M13 based on FWT calibration data.

### constantKernelOverhead = 1800 cycles (MI300A), default 3600 cycles

**Source:** Calibrated against real MI300A hardware measurements.

- Default (dispatching builder): 3600 cycles (2.0 μs at 1.8 GHz)
- MI300A override: 1800 cycles (~1.0 μs at 1.8 GHz)

**Rationale:** Real MI300A post-completion overhead is ~1 μs based on HW measurement analysis. The default of 3600 cycles was too conservative, adding excessive overhead especially for multi-kernel benchmarks. The MI300A builder now explicitly sets this to 1800 via `WithConstantKernelOverhead(1800)` on the CP builder, which passes it through to the dispatching builder.

**Known issue:** For multi-kernel benchmarks like stencil2d (5 iterations), kernel overhead adds up and may not reflect real hardware behavior. Reducing from 3600 to 1800 cycles helps but total overhead per stencil2d run may still be significant.

---

## Accuracy Summary (M9.1)

| Benchmark | Points | Avg |Error| | Notes |
|-----------|--------|-------------|-------|
| matrixmultiplication | 4 | 4.8% | Excellent — compute-bound, well-modeled |
| bicg | 9 | 20.2% | Good — regular memory access pattern |
| matrixtranspose | 5 | 34.5% | Acceptable — moderate kernel overhead |
| atax | 9 | 40.4% | Acceptable — sim is "too fast" (under-models real overhead) |
| fastwalshtransform | 4 | 45.5% | Acceptable at small sizes; 97% error at 65536 |
| fir | 5 | 58.1% | Marginal — large sizes affected by memory throughput limits |
| vectoradd | 10 | 87.7% | Poor — large sizes hit DRAM bandwidth limit |
| relu | 9 | 106.8% | Poor — same DRAM bandwidth bottleneck as vectoradd |
| fft1D_512 | 3 | 218.2% | Poor — sim 3x too slow, likely multi-kernel overhead |
| stencil2d | 7 | 439.3% | Very poor — 5x too slow, multi-kernel + LDS overhead |

**v1 (65 points):** avg |error| = 104.3%, median = 35.3%, within 50% = 64.6%
**v2 (65 points, corrected stencil2d/fft):** avg |error| = **58.2%**, median = **35.3%**, within 50% = **69.2%**

v2 changes: stencil2d re-run with `-iter 1` (was 5), fft re-run with `-passes 1` (was 2).
- stencil2d avg error: 439% → 62%
- fft avg error: 218% → 102%

---

## Known Accuracy Gaps and Root Causes

### 1. Streaming Workload Bandwidth (vectoradd, relu at large sizes)
- **Root cause:** CU memory instruction issue rate limits effective bandwidth
- **Why:** Even with memPipelineBufferSize=32, the CU vector memory pipeline can only issue 1 memory instruction per 2 cycles per SIMD. For streaming workloads with pure memory operations, this is the bottleneck.
- **Fix needed:** Model wider memory instruction issue or investigate CU pipeline occupancy

### 2. FFT Still ~2x Too Slow
- **Root cause:** Even with -passes 1, fft1D_512 shows ~102% avg error. The butterfly memory pattern and multiple-workgroup dispatch overhead are overestimated.
- **Fix needed:** Profile FFT kernel execution to identify if it's memory latency or dispatch overhead

### 3. DRAM Bandwidth Gap — FIXED in M10
- **Root cause:** Simulated DRAM bandwidth was 65.5 TB/s (12× too fast)
- **Fix (M10):** Changed BPW=4→1, depth=20→10, SL=1→3. Now 5.46 TB/s (matches 5.3 TB/s real)
- **Status:** Fixed. See DRAM Configuration section above for full analysis.

---

## Future Work

1. **Microbenchmark validation:** Create targeted microbenchmarks to measure specific parameters (L1/L2 latency, DRAM bandwidth, kernel launch overhead) on real MI300A hardware
2. ~~**DRAM bandwidth model:** Replace simple banked memory with HBM3-aware model~~ — **DONE in M10** (calibrated existing model to 5.46 TB/s)
3. **CU memory throughput:** Investigate why CU memory instruction issue rate is the bottleneck
4. **Multi-kernel overhead:** Profile real hardware kernel launch sequences to calibrate overlap model
5. **CI-based benchmarking:** Use `.github/workflows/benchmark.yml` for all future benchmark runs (added in M10)
