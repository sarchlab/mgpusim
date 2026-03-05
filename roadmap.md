# Roadmap: MI300A Timing Simulation Accuracy

## Goal
Achieve <20% average error and <50% max error for MI300A timing simulation vs real hardware measurements.

## Milestones

### M1: Baseline Measurement & Analysis (cycles: 3)
**Status: PENDING**
- Build automated benchmarking scripts that run key benchmarks on the simulator and compare results to `gpu_perf_scripts/mi300a.csv`
- Create a comparison script that calculates per-benchmark and overall error metrics
- Establish baseline accuracy numbers for all 22 benchmarks
- Identify which benchmarks have the worst accuracy and categorize root causes (compute-bound, memory-bound, launch overhead, etc.)

### M2: Parameter Tuning — Memory Subsystem (cycles: 4)
**Status: PENDING**
- Switch DRAM controller from `idealmemcontroller` to realistic `dram` model (the builder code already exists in `createDramControllerBuilder`)
- Tune HBM parameters (bandwidth, latency, ranks, banks) to match MI300A specs
- Calibrate L2 cache size, associativity, MSHR entries
- Calibrate memory interleaving settings
- Target: significant reduction in error for memory-bound benchmarks (memcopy, matrixtranspose, vectoradd large sizes)

### M3: Parameter Tuning — Compute Pipeline (cycles: 4)
**Status: PENDING**
- Tune CU frequency, SIMD count, wavefront pool sizes
- Calibrate instruction latencies if configurable
- Tune register file sizes to match MI300A specs
- Target: reduce error for compute-bound benchmarks (nbody, matmul, floydwarshall)

### M4: Fine-Tuning & Edge Cases (cycles: 4)
**Status: PENDING**
- Handle kernel launch overhead for very small problem sizes
- Fine-tune L1 cache and TLB parameters
- Cross-validate with 120 CU data
- Final pass to bring all benchmarks under 50% max error
- Target: <20% average, <50% max across all benchmarks

## Lessons Learned
(To be updated as milestones complete)
