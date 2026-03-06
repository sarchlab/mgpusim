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

**M4** (merged, verified): Kernel launch + interconnect fixes
- Cached kernel code object GPU addresses across launches
- Fixed memRangeOverlap adjacency bug (> instead of >=)
- Investigated MMU page-not-found panics (corrupted 64-bit FLAT addresses in timing mode)
- Reduced MI300A switch latency from 140→15 (Infinity Fabric, not PCIe)
- storageAccessor.Write bug fixed

### Baseline After M4 (Blake's comprehensive run, 66 data points)
| Benchmark | Avg |Error| | Direction | Status |
|-----------|-------------|-----------|--------|
| nw | 10.6% | ~neutral | ✅ Excellent |
| matmul | 23.9% | sim>real at large sizes | ✅ Good |
| spmv | 33.0% | sim>real | OK |
| nbody | 152.9% (1-iter) | sim<real | ⚠️ Too fast |
| fir | 164.2% | sim<real | ⚠️ Limited data |
| stencil2d | 194.5% | sim>real | ⚠️ Too slow |
| relu | 233.6% | sim<real | ⚠️ Launch overhead |
| vectoradd | N/A | sim<real | ⚠️ Launch overhead |
| kmeans | 337.3% | sim>real ~4x | ❌ Too slow |
| floydwarshall | 595.8% | sim<real | ❌ Too fast |
| atax | 2572% | constant time bug | ❌ Broken |
| 5 benchmarks HANG | bfs,bitonicsort,simpleconv,mattrans,fwt | ❌ s_nop bug |

**Overall: mean 341.1%, median 151.8%, 27% within 20%**

## Active Milestones

### M5: Fix s_nop hang + tune kernel launch overhead (IMPLEMENTING)
- **Cycles budget:** 6
- **Focus:**
  1. Fix s_nop infinite loop in scheduler.go (default case doesn't advance PC) — unlocks 5+ hanging benchmarks
  2. Reduce H2D/D2H middleware cycles for MI300A (unified memory, ~500/300 instead of 14500/8500)
  3. Set constantKernelOverhead to ~3600 cycles (~2µs GPU-side dispatch overhead)
  4. Run comprehensive benchmarks and document improvement
- **Acceptance criteria:**
  - bitonicsort and simpleconvolution complete in timing mode
  - At least 10 benchmarks produce results (vs 8 previously)
  - Mean error across all benchmarks < 341% (improvement over baseline)
- **Issue:** #290

## Planned Milestones

### M6: Compute/Memory Accuracy Tuning
- Address systematic errors: nbody (~2.5x too fast), kmeans (~4x too slow), floydwarshall (~6x too fast)
- Investigate atax constant-time bug
- Target: mean error < 100%

### M7: Memory Subsystem & Large Problem Sizes
- Fix MMU page faults blocking larger problem sizes
- DRAM model accuracy (HBM3 parameters)
- Cache hierarchy tuning
- Target: avg <50%, max <200%

### M8: Final Accuracy Push
- Fine-tune all parameters
- Target: avg <20%, max <50%

## Key Investigation Findings (Pre-M5)

### s_nop Bug (Harper, issue #289)
- `amd/timing/cu/scheduler.go` EvaluateInternalInst default case sets WfReady but doesn't advance PC
- s_nop loops forever → 5+ benchmarks hang
- Fix: use `s.cu.UpdatePCAndSetReady(executing)` instead of `executing.State = wavefront.WfReady`

### Kernel Launch Overhead (Emma, issue #288)
- H2D middleware delay = 29,000 cycles per launch (2 × 14,500) — dominates overhead
- MI300A has unified memory — H2D delay should be minimal
- constantKernelOverhead = 0 — should be ~2000-4000 cycles for GPU scheduler/cache warmup
- Real HW shows ~4µs minimum per kernel; sim has overhead in wrong place (CPU-side vs GPU-side)

### GPU-side Queueing (Human issue #286)
- Real HW: CPU writes AQL packet to ring buffer, GPU picks up autonomously
- Sim: CPU sends explicit messages, waits for round-trip per command
- Full implementation deferred; overhead tuning in M5 addresses the immediate symptom

## Lessons Learned
- SIMD=32 was incorrect — always verify against ISA documentation
- Symmetrical error penalizes both over and underestimates more equally
- Small problem sizes are dominated by kernel launch overhead, not compute
- Development must stay in origin repo, not upstream
- Page-not-found crashes caused by corrupted 64-bit FLAT addresses in timing mode
- Stencil2d constant timing was caused by per-launch code re-allocation
- Switch latency needed to be 15 (Infinity Fabric) not 140 (PCIe)
- M4 accuracy was measured on only 27 points; comprehensive run showed 66 points with worse mean
- s_nop infinite loop was root cause for ALL hanging benchmarks (compiler inserts s_nop for hazard avoidance)
- Kernel launch overhead is modeled in the wrong place (CPU-side H2D delay vs GPU-side scheduler overhead)
- Cycle estimates: M1-M4 took ~20 cycles total; budget M5 at 6 cycles for focused bug fix + tuning
