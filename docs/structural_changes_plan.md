# Structural Changes Plan for MGPUSim MI300A Timing Accuracy

**Date:** 2026-04-01
**Status:** Proposal
**Context:** [docs/validation_report.md](validation_report.md) — 30 kernels, 198 matched points, overall WMAPE 100%, application-only WMAPE 69.1%

---

## 1. Overview

MGPUSim's timing simulation currently uses a **flat memory topology** with
uniform-latency `directconnection` links between all components. The real AMD
MI300A has **8 XCDs (Accelerator Complex Dies)**, each containing a subset of
CUs, L1 caches, and a portion of the L2 cache, connected via an inter-XCD
crossbar/fabric with non-uniform latencies. This architectural mismatch is the
single largest contributor to WMAPE: 13 of 30 benchmarked kernels run "too fast"
in simulation because cross-die latency, DRAM controller contention, and memory
bandwidth limits are undermodeled.

This document proposes **concrete structural changes** to close the gap between
simulated and real MI300A timing, ordered by expected accuracy improvement per
unit of effort.

---

## 2. Top Issues by Impact

The table below ranks accuracy issues by their estimated contribution to overall
WMAPE, derived from the validation report's per-kernel error data.

| # | Issue | Affected Kernels | WMAPE Contribution | Sim/Real Direction |
|---|-------|-----------------|-------------------|-------------------|
| 1 | **Flat topology — no XCD partitioning** | atomic_throughput, fp32_fma, fp64_fma, mem_latency_chase, busspeedreadback, l2_cache_bw, hotspot, hotspot3d, syrk, syr2k, 3mm, spmv_csr, srad (13 kernels) | ~60–70% of total error weight | Too fast |
| 2 | **Compute throughput undercount (SIMD pipeline width)** | fp32_fma (ratio 0.048), fp64_fma (ratio 0.063), ep, gemm, 2mm, lud | ~10–15% | Too fast |
| 3 | **DRAM model lacks HBM3 bank-group/channel contention** | triad (ratio 2.23, scaling WMAPE 461%), busspeedreadback, l2_cache_bw | ~8–10% | Mixed |
| 4 | **Kernel launch / fixed overhead mismatch** | adjust_weights (ratio 71×), bs (small sizes), gesummv | ~7–8% (dominated by adjust_weights) | Too slow |
| 5 | **Atomic operation latency model** | atomic_throughput (ratio 0.000) | ~3–5% | Too fast |
| 6 | **L2 cache coherence and bandwidth throttling** | spmv_csr, hotspot, ep | ~2–3% | Too fast |

---

## 3. Proposed Changes

### 3.1 XCD-Aware Topology (Priority 1)

**Problem:** All 120 CUs (20 shader arrays × 6 CUs) share a single flat
`directconnection` to L2 caches. Real MI300A groups CUs into 8 XCDs, each
with its own L1→L2 path and an inter-XCD fabric for cross-die traffic.

**Current code path:**
- `mi300a/builder.go` → `connectL1ToL2()` creates one `directconnection`
  ("L1ToL2") that connects **all** SA L1 cache bottom ports to **all** L2
  cache top ports.
- `directconnection` delivers messages in the same cycle (zero latency
  beyond port scheduling). No topology-aware routing.

**Proposed change:**

1. **Partition shader arrays into XCDs.** Add an `numXCDs` parameter
   (default 8 for MI300A, with 2–3 SAs per XCD). Each XCD gets its own
   intra-XCD `directconnection` linking its SAs' L1 caches to a subset of
   L2 cache banks.

2. **Add inter-XCD `latencyconn`** (already implemented in
   `amd/timing/latencyconn/`). Connect XCD-local L2 slices to remote L2
   slices via `latencyconn.Comp` with configurable latency (estimated
   50–100 cycles at 1.75 GHz based on MI300A die-to-die latency of ~30–60 ns).

3. **Update `l1AddressMapper`** to route requests to XCD-local L2 banks
   first, falling back to remote XCDs for addresses hashed to other
   banks. This requires a new `XCDInterleavedAddressPortMapper` that
   checks locality before routing.

**Files to modify:**
- `amd/samples/runner/timingconfig/mi300a/builder.go` — partition SAs,
  create per-XCD connections, add inter-XCD latency connections
- New file: `amd/timing/mem/xcdaddressmapper.go` — XCD-aware address
  routing
- `amd/samples/runner/timingconfig/shaderarray/builder.go` — add XCD ID
  to shader array metadata

**Effort:** **L** (Large) — 3–5 days. Requires careful topology wiring and
extensive testing across all benchmarks.

**Expected impact:** ~15–25% WMAPE reduction. The 13 "too fast" kernels
should see sim/real ratios increase from 0.0–0.5 toward 0.5–1.0.

---

### 3.2 SIMD Pipeline Width and Compute Throughput (Priority 2)

**Problem:** `SIMDUnit.AcceptWave()` computes `cycleLeft = 64 / NumSinglePrecisionUnits`.
With `NumSinglePrecisionUnits = 16`, each wavefront takes 4 cycles. Real CDNA3
CUs have **16 FP32 ALUs per SIMD but 4 SIMDs per CU**, yielding 64 FP32 ops/cycle
per CU. However, the simulator's SIMD pipeline is **non-pipelined by default**
(only one wave executes at a time per SIMD unless `scoreboardEnabled` is true).
The scoreboard/pipeline mode is already implemented but the pipeline capacity
(`pipelineCapacity`) defaults to 0, limiting throughput.

Additionally, FP64 throughput on CDNA3 is **1:1 with FP32** (MI300A has
full-rate FP64), but the simulator does not differentiate FP64 instruction
latency from FP32, potentially over- or under-counting FP64-heavy kernels.

**Proposed changes:**

1. **Set SIMD pipeline capacity.** In `cubuilder.go`, when
   `scoreboardEnabled` is true, set `pipelineCapacity` to 4 (matching the
   4-cycle wavefront execution latency, allowing 4 waves in-flight per
   SIMD for full throughput).

2. **Add FP64 instruction latency differentiation.** In `simdunit.go`
   `AcceptWave()`, check the instruction opcode category. For FP64
   instructions on CDNA3, use `cycleLeft = 64 / numDoublePrecisionUnits`
   where `numDoublePrecisionUnits = 16` (MI300A has 1:1 FP64:FP32 rate).
   For non-CDNA3 configs, FP64 should be half-rate (8 units).

3. **Integer/special function latency.** Transcendental operations
   (sin, cos, exp, log, rcp) execute on the SALU or a dedicated SFU at
   ¼ rate. Add a `numTranscendentalUnits` parameter defaulting to 4.

**Files to modify:**
- `amd/timing/cu/cubuilder.go` — set `pipelineCapacity` on SIMD units
- `amd/timing/cu/simdunit.go` — instruction-category-aware latency in
  `AcceptWave()`
- `amd/timing/cu/scheduler.go` — ensure scoreboard correctly tracks
  multi-cycle instructions

**Effort:** **M** (Medium) — 2–3 days. Core logic is straightforward;
testing requires verifying cycle counts against known instruction mixes.

**Expected impact:** ~10–15% WMAPE reduction. fp32_fma, fp64_fma should
jump from ratio 0.05–0.06 to closer to 0.5–1.0. Dense compute kernels
(gemm, 2mm, 3mm, syrk, syr2k) should also improve.

---

### 3.3 HBM3 DRAM Controller Model (Priority 3)

**Problem:** The current DRAM model (`simplebankedmemory`) uses 16 banks
with fixed 5-stage pipelines and a simple row-buffer miss penalty (52
cycles). Real MI300A has 8 HBM3 stacks with 128 banks per stack, complex
bank-group interleaving, and channel-level bandwidth limits (~5.2 TB/s
aggregate). The simplified model:
- Under-penalizes bank conflicts (only 16 banks vs 1024+ real banks)
- Lacks per-channel bandwidth limits (real HBM3 channels saturate at
  ~40 GB/s each)
- Has no pseudo-channel or bank-group timing constraints

**Proposed changes:**

1. **Increase DRAM bank count.** Change `numMemoryBank` from 16 to 64
   (or 128) to better represent HBM3 bank-level parallelism and
   conflict rates.

2. **Add per-channel bandwidth cap.** Implement a `maxBytesPerCycle`
   limit in `simplebankedmemory` that throttles how many bytes can be
   served per cycle across all banks, modeling the aggregate HBM3
   bandwidth wall. At 1 GHz DRAM freq with 128-byte cache lines:
   `maxBytesPerCycle = 5200 / 1.0 = 5200 bytes/cycle` (5.2 TB/s).

3. **Bank-group conflict penalty.** Add a configurable bank-group
   conflict stall (e.g., 2 extra cycles) when consecutive accesses hit
   the same bank group. This penalizes streaming patterns that stride
   across bank groups poorly.

**Files to modify:**
- `amd/timing/mem/simplebankedmemory/` — add bandwidth cap, bank-group
  conflict logic
- `amd/samples/runner/timingconfig/mi300a/builder.go` — update DRAM
  parameters

**Effort:** **M** (Medium) — 2–3 days.

**Expected impact:** ~5–10% WMAPE reduction. Memory-bandwidth-bound
kernels (triad, busspeedreadback, l2_cache_bw) should improve. The triad
kernel's scaling-region WMAPE (460.9%) should decrease significantly with
proper bandwidth throttling.

---

### 3.4 Kernel Launch Overhead Calibration (Priority 4)

**Problem:** Some kernels (adjust_weights, bs, gesummv) show the simulator
running much *slower* than hardware at small sizes. The `CommandProcessor`
has configurable `constantKernelLaunchOverhead` and
`subsequentKernelLaunchOverhead`, both currently set to **0 cycles**
(`mi300a/builder.go:363-364`). However, the workgroup dispatch mechanism
introduces implicit overhead through the CP→CU dispatch pipeline.

For `adjust_weights` (ratio 71×, WMAPE 7475%), the issue is likely
**per-wavefront dispatch overhead** in the simulator: each WG dispatch
involves message passing through the CP, which serializes at simulation
granularity rather than pipelining as real hardware does.

**Proposed changes:**

1. **Audit dispatch pipeline throughput.** Profile how many cycles the
   simulator takes to dispatch N workgroups and compare against MI300A
   hardware dispatch rate (~1 WG per CU per ~10 cycles). Identify
   serialization bottlenecks in `dispatching/dispatcher.go`.

2. **Implement batch dispatching.** Allow the CP to dispatch multiple WGs
   per cycle (up to the number of available CUs) rather than one at a
   time. Real MI300A ACE (Asynchronous Compute Engine) can dispatch
   multiple WGs simultaneously.

3. **Add WG dispatch pipeline width parameter.** Add `wgDispatchWidth`
   to the CP builder, defaulting to 8 (matching 8 XCDs that can accept
   WGs in parallel).

**Files to modify:**
- `amd/timing/cp/internal/dispatching/` — batch dispatch logic
- `amd/timing/cp/builder.go` — add `wgDispatchWidth` parameter
- `amd/timing/cp/commandprocessor.go` — CP tick loop changes

**Effort:** **M** (Medium) — 2–3 days.

**Expected impact:** ~5–8% WMAPE reduction. Primarily fixes adjust_weights
(7475% → <100%) and small-size behavior for bs and gesummv. Does not affect
large-size accuracy.

---

### 3.5 Atomic Operation Modeling (Priority 5)

**Problem:** `atomic_throughput` has sim/real ratio of 0.000 (WMAPE 100%).
The simulator executes `flat_atomic` operations via
`VectorMemoryUnit.executeFlatAtomic()` which builds memory transactions
and sends them through the cache hierarchy. However, there's no model for:
- L2 cache atomic arbitration latency (real hardware serializes atomics
  at the L2 bank level)
- Cross-XCD atomic coherence traffic
- Read-modify-write pipeline stalls

**Proposed changes:**

1. **Add atomic serialization penalty at L2.** When an atomic op arrives
   at an L2 bank, add a configurable serialization delay (e.g., 16–32
   cycles) representing the read-modify-write cycle.

2. **Global atomic counter.** Track concurrent atomics to the same cache
   line and apply contention-based stalls (each additional concurrent
   atomic doubles the latency).

3. **Tag atomic transactions in coalescer.** The coalescer should
   recognize atomic ops and not coalesce them across lanes (each lane's
   atomic is independent), reflecting real hardware behavior.

**Files to modify:**
- `amd/timing/cu/vectormemoryunit.go` — tag atomic transactions
- `amd/timing/cu/defaultcoalescer.go` — skip coalescing for atomics
- L2 writeback cache (in akita) — add atomic penalty (may require
  upstream change or wrapper)

**Effort:** **M** (Medium) — 2–3 days. L2-side changes may need akita
library modification.

**Expected impact:** ~3–5% WMAPE reduction. Fixes atomic_throughput from
0.000 ratio. May also improve accuracy for kernels using atomics
(histogram, reduction).

---

### 3.6 L2 Cache Bandwidth and Coherence (Priority 6)

**Problem:** The L2 cache is configured with `NumReqPerCycle = 16` and
512 MSHR entries, which is generous but lacks:
- Per-bank request queuing limits (real L2 banks have finite input
  queues)
- Directory-based coherence traffic modeling
- Writeback traffic to DRAM under high eviction rates

**Proposed changes:**

1. **Reduce L2 NumReqPerCycle per bank.** Currently 16 requests per cycle
   across the entire L2 cache. For 16 L2 banks, this is 1 req/bank/cycle,
   which may be correct. Verify against MI300A specs (typically 2–4
   reqs/bank/cycle for L2).

2. **Add L2 eviction backpressure.** When eviction buffer is full, stall
   incoming requests. This models contention under write-heavy workloads.

3. **L2 bank conflict penalty.** Add stall cycles when multiple requests
   target the same L2 bank in the same cycle.

**Files to modify:**
- L2 configuration in `mi300a/builder.go`
- Possibly `akita/mem/cache/writeback/` (upstream)

**Effort:** **S** (Small) — 1–2 days for parameter tuning; **M** for
structural changes to writeback cache.

**Expected impact:** ~2–3% WMAPE reduction. Affects spmv_csr, hotspot,
ep, and other kernels with irregular access patterns.

---

## 4. Effort Summary

| Change | T-Shirt Size | Estimated Days | Priority |
|--------|-------------|---------------|----------|
| 3.1 XCD topology | L | 3–5 | P1 |
| 3.2 SIMD pipeline/compute throughput | M | 2–3 | P2 |
| 3.3 HBM3 DRAM model | M | 2–3 | P3 |
| 3.4 Kernel launch overhead | M | 2–3 | P4 |
| 3.5 Atomic operation modeling | M | 2–3 | P5 |
| 3.6 L2 cache tuning | S–M | 1–3 | P6 |
| **Total** | | **13–20 days** | |

---

## 5. Priority Rationale

```
                    High Impact
                        │
     ┌──────────────────┼──────────────────┐
     │                  │                  │
     │  P1: XCD Topo    │                  │
     │  (60-70% error)  │                  │
     │                  │                  │
     │  P2: SIMD Width  │                  │
     │  (10-15% error)  │                  │
     │──────────────────┼──────────────────│
     │                  │                  │
     │  P3: HBM3 DRAM   │  P4: Launch OH   │
     │  (5-10% error)   │  (5-8% error)    │
     │                  │                  │
     │  P5: Atomics     │  P6: L2 Tuning   │
     │  (3-5% error)    │  (2-3% error)    │
     │                  │                  │
     └──────────────────┼──────────────────┘
                        │
                    Low Impact
     Low Effort ────────┼──────── High Effort
```

**P1 (XCD)** is the clear top priority: it's the root cause of the
dominant "too fast" error pattern across 13 kernels and addresses the
single largest structural gap. While it's also the largest effort, the
`latencyconn` infrastructure already exists.

**P2 (SIMD)** is second because compute throughput errors affect
many kernels and the fix is well-scoped — pipeline capacity and
per-opcode latency tables.

**P3–P6** are roughly equal in effort but declining in impact.
P3 (DRAM) is preferred over P4 (launch overhead) because DRAM
fixes improve scaling-region accuracy, which matters more than
overhead-region accuracy for real-world predictions.

---

## 6. Dependencies

```
P1: XCD Topology ──→ P5: Atomics (cross-XCD atomic traffic requires
│                     XCD topology to model correctly)
│
├──→ P3: HBM3 DRAM (XCD-to-DRAM routing affects bank assignment)
│
└──→ P6: L2 Tuning (per-XCD L2 slices change bank conflict patterns)

P2: SIMD Pipeline (independent — can be done in parallel with P1)

P4: Launch Overhead (independent — can be done in parallel with P1)
```

- **P1 → P5:** Atomic contention modeling needs XCD partitioning to
  simulate cross-die coherence. Implementing atomics before XCD
  topology would only capture intra-die contention.
- **P1 → P3:** The XCD topology determines which DRAM controllers serve
  which XCDs. Bank assignment and channel routing depend on the
  topology.
- **P1 → P6:** L2 cache partitioning across XCDs changes the effective
  cache size per compute group and the eviction/conflict patterns.
- **P2 and P4 are independent** and can proceed in parallel with P1.

---

## 7. Success Criteria

After implementing all changes, the target metrics are:

| Metric | Current | Target |
|--------|---------|--------|
| Overall WMAPE | 100.0% | ≤ 40% |
| Application-only WMAPE | 69.1% | ≤ 25% |
| Per-kernel Spearman ρ (avg) | 0.79 | ≥ 0.85 |
| Kernels with ρ ≥ 0.9 | 21/27 | ≥ 24/27 |
| "Too fast" kernels (ratio < 0.5) | 13 | ≤ 4 |
| "Too slow" kernels (ratio > 1.5) | 4 | ≤ 2 |

Individual change targets:

- **After P1 (XCD):** "Too fast" kernels reduced from 13 to ≤7; overall
  WMAPE ≤ 75%.
- **After P1+P2:** Application-only WMAPE ≤ 45%; fp32_fma/fp64_fma ratios
  > 0.3.
- **After P1+P2+P3:** triad scaling-region WMAPE < 100%;
  busspeedreadback ratio > 0.5.
- **After all (P1–P6):** Overall WMAPE ≤ 40%; ≤ 6 kernels outside
  the 0.5–1.5 ratio band.

---

## 8. Implementation Notes

### 8.1 Existing Infrastructure

The following components already exist and should be leveraged:

- **`latencyconn.Comp`** (`amd/timing/latencyconn/`): A connection with
  configurable cycle-based latency. Use this for inter-XCD links.
- **`simplebankedmemory`** (`amd/timing/mem/simplebankedmemory/`):
  Already has row-buffer miss modeling (`rowMissDelay`). Extend with
  bandwidth cap and bank-group conflict logic.
- **Scoreboard/pipeline mode** in `SIMDUnit`: Already implemented behind
  `scoreboardEnabled` flag. Just needs capacity tuning.
- **CP overhead parameters**: `constantKernelLaunchOverhead`,
  `subsequentKernelLaunchOverhead`, `constantKernelOverhead` are wired
  through the builder; just need calibration.

### 8.2 Testing Strategy

Each change should be validated by:

1. **Unit tests** for new components (XCD mapper, bandwidth cap)
2. **Micro-benchmark regression**: fp32_fma, fp64_fma,
   atomic_throughput, busspeedreadback, mem_latency_chase, triad
3. **Application benchmark regression**: gemm, 3mm, hotspot, spmv_csr,
   bs, atax, bicg
4. **Full CI matrix run** after each priority level (P1, then P1+P2,
   etc.) to regenerate `comparison_ci.csv` and update
   `validation_report.md`

### 8.3 Risk Mitigation

- **P1 (XCD) is complex.** Start with a simplified 2-level topology
  (intra-XCD = 0 cycles, inter-XCD = 80 cycles) and iterate. Don't try
  to model the full mesh/crossbar immediately.
- **Akita upstream changes.** P5 (atomics) and P6 (L2) may require
  changes in the akita memory library. If so, create PRs there first
  and vendor-sync before implementing in mgpusim.
- **Parameter sensitivity.** Use the 13 well-calibrated kernels (ratio
  0.5–1.5) as regression guards. Any change that pushes these kernels
  outside the band indicates over-tuning.
