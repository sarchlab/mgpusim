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

### M4 Results (Current Accuracy — 27 data points, 8 benchmarks)
| Benchmark | Avg |Error| | Direction | Key Issue |
|-----------|-------------|-----------|-----------|
| matmul | 5.6% | ~neutral | ✅ Excellent |
| nw | 5.8% | ~neutral | ✅ Excellent |
| fir | 166.7% | too fast | Limited data (1 point) |
| stencil2d | 194.7% | too slow | Launch overhead |
| nbody | 210.0% | too slow | Overestimated |
| relu | 235.6% | too fast | Launch overhead dominated |
| vectoradd | 242.7% | too fast | Launch overhead dominated |
| floydwarshall | 442.4% | too fast | Small absolute times |

**Overall: avg 156.9% error, median 205.4%**

### Blake's Broader Baseline (51 data points, upstream/gfx942_emu branch)
- Mean absolute error: 75.7%, median: 71.5%
- Best: matmul/nw/vectoradd. Worst: kmeans, nbody, matmul at large sizes
- 7 benchmarks timeout (bfs, bitonicsort, conv2d, fft, fwt, im2col, matrixtranspose, simpleconvolution)
- bicg page fault, atax constant time

### Key Problems
1. **Fixed overhead mismatch**: relu/vectoradd/fir show sim too fast — sim doesn't model real kernel dispatch overhead floor (~4µs on real HW)
2. **Compute overestimate**: nbody/stencil2d show sim too slow — memory/compute timing too pessimistic
3. **Many benchmarks broken**: timeouts (7 benchmarks), page faults (bicg, pagerank), constant time (atax)
4. **MMU page faults** limit testable sizes for working benchmarks
5. **CPU-side queueing architecture** (human issue #286): sim uses CPU↔GPU round-trip per command; real HW uses GPU-side streams

## Active Milestones

### M5: Kernel Launch Overhead & Broader Benchmark Coverage (PLANNING)
- **Focus areas**:
  1. Investigate and tune kernel launch overhead (fixed overhead floor mismatch)
  2. Investigate GPU-side queueing vs CPU-side queueing (human issue #286)
  3. Investigate why 7+ benchmarks timeout/hang
  4. Run comprehensive benchmark suite on current main to establish accurate baseline
- **Status**: Under investigation — need data before defining concrete acceptance criteria

## Planned Milestones

### M6: Compute Accuracy (nbody, stencil2d, kmeans)
- Address overestimation in compute-heavy benchmarks
- Target nbody/stencil2d from ~200% to <50%

### M7: Memory Subsystem Tuning
- DRAM model accuracy (HBM3 parameters)
- Cache hierarchy tuning
- Target: avg <20%, max <50%

## Lessons Learned
- SIMD=32 was incorrect — always verify against ISA documentation
- Symmetrical error penalizes both over and underestimates more equally
- Small problem sizes are dominated by kernel launch overhead, not compute
- Development must stay in origin repo, not upstream
- Page-not-found crashes caused by corrupted 64-bit FLAT addresses in timing mode
- Stencil2d constant timing was caused by per-launch code re-allocation
- Switch latency needed to be 15 (Infinity Fabric) not 140 (PCIe)
- M4 accuracy was measured on only 27 points across limited size ranges — need broader coverage
- Many benchmarks that worked in emu mode timeout in timing mode — likely instruction dispatch issues
