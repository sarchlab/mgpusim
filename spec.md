# Project Spec: MI300A Timing Simulation Accuracy

## What do you want to build

Tune the MGPUSim timing simulator to accurately model AMD MI300A (gfx942/CDNA3) GPU performance. The simulator should produce kernel execution times that closely match real hardware measurements across all benchmarks in the `gpu_perf_scripts/` directory.

## How do you consider the project is success

- **Average error** across all benchmark/problem-size combinations: **< 20%**
- **Maximum error** for any single benchmark/problem-size: **< 50%**
- Error is measured as `|sim_time - real_time| / real_time` for each benchmark/problem-size pair
- Reference data: `gpu_perf_scripts/mi300a_120cu.csv` (120 CU — matches simulator's 120 CU config)
- **Skip large benchmarks** that cannot complete in simulation. Focus on smaller problem sizes that the simulator can handle within reasonable time.
- **Exclude kernel-launch-overhead-dominated sizes**: Problem sizes where real hardware time < ~0.01ms are dominated by launch overhead that the simulator does not model. Exclude these from accuracy evaluation.
- The human is flexible on evaluation methodology — we define which benchmark/size pairs to include.

## Constraints

- Create PRs in upstream (sarchlab/mgpusim) only, do not merge in upstream
- The human can run scripts on real MI300A hardware — create scripts and provide instructions via GitHub issue titled "HUMAN: [description]"
- Do not break existing tests or emulation correctness
- Timing model parameters to tune include: clock frequencies, cache sizes/associativity, DRAM latency/bandwidth parameters, pipeline widths, MSHR entries, memory bank interleaving, TLB configuration, etc.

## DRAM Model Evaluation (Human Request)

The Akita DRAM model (`github.com/sarchlab/akita/v4/mem/dram`) may not accurately model HBM3 memory used in MI300A. The human specifically requested:
- Create smaller programs to evaluate the HBM model
- Identify what the Akita team needs to change for accurate HBM modeling
- This is a separate deliverable alongside the timing accuracy work

## Resources

- MI300A real GPU measurements: `gpu_perf_scripts/mi300a.csv` (240 CU, 51 iterations per benchmark)
- MI300A 120 CU measurements: `gpu_perf_scripts/mi300a_120cu.csv` (120 CU, half CUs disabled, 5 iterations)
- 24 benchmarks with varying problem sizes
- Existing MI300A builder: `amd/samples/runner/timingconfig/mi300a/builder.go`
- Timing CU implementation: `amd/timing/cu/`
- Shader array configuration: `amd/samples/runner/timingconfig/shaderarray/builder.go`
- Comparison script: `gpu_perf_scripts/compare_sim_vs_real.py` (on branch `blake/benchmark-analysis-script`)
- Benchmark runner script: `gpu_perf_scripts/run_sim_benchmarks.sh` (on branch `casey/sim-benchmark-script`)

## Key Discrepancies Found (Research Phase)

Priority-ordered list of simulator vs real MI300A differences:

1. **DRAM Model**: Uses `idealmemcontroller` with fixed 100-cycle latency — no bandwidth modeling. A detailed HBM DRAM builder (`createDramControllerBuilder`) already exists in code but is unused.
2. **Wavefront Pool Size**: Simulator uses 10/SIMD, CDNA3 should be 8/SIMD (25% occupancy overestimate)
3. **VGPR Count**: Simulator uses 16384/SIMD, CDNA3 should be 32768/SIMD (half the real register file)
4. **SIMD Execution Latency**: May be 4 cycles in simulator vs 1 cycle for CDNA3
5. **L2 Cache Size**: 8 MB total vs real 24+ MB
6. **Clock Frequency**: 1500 MHz vs ~1900 MHz boost (~21% gap)
7. **L1V Bank Latency**: 60 cycles seems high (typical ~30-50 cycles)

## Notes

- The simulator creates 20 shader arrays × 6 CUs = 120 CUs, matching the 120 CU test data.
- Benchmarks are predominantly memory-bound (240 vs 120 CU scaling shows ~1.0x speedup for most).
- Peak observed MI300A bandwidth: ~3.6 TB/s (65-70% of 5.3 TB/s HBM3 spec).
- MMU page fault bug limits problem sizes in simulation (~16K-32K elements crash).
- Some benchmarks are extremely slow to simulate (bitonicsort, simpleconvolution).
