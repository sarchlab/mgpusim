# Project Spec: MI300A Timing Simulation Accuracy

## What do you want to build

Tune the MGPUSim timing simulator to accurately model AMD MI300A (gfx942/CDNA3) GPU performance. The simulator should produce kernel execution times that closely match real hardware measurements across all benchmarks in the `gpu_perf_scripts/` directory.

## How do you consider the project is success

- **Average error** across all benchmark/problem-size combinations: **< 20%**
- **Maximum error** for any single benchmark/problem-size: **< 50%**
- Error is measured as `|sim_time - real_time| / real_time` for each benchmark/problem-size pair
- Reference data: `gpu_perf_scripts/mi300a.csv` (full 240 CU)
- 120 CU data available in `gpu_perf_scripts/mi300a_120cu.csv` for additional calibration

## Constraints

- Create PRs in upstream (sarchlab/mgpusim) only, do not merge in upstream
- The human can run scripts on real MI300A hardware — create scripts and provide instructions via GitHub issue titled "HUMAN: [description]"
- Do not break existing tests or emulation correctness
- Timing model parameters to tune include: clock frequencies, cache sizes/associativity, DRAM latency/bandwidth parameters, pipeline widths, MSHR entries, memory bank interleaving, TLB configuration, etc.

## Resources

- MI300A real GPU measurements: `gpu_perf_scripts/mi300a.csv` (240 CU, 51 iterations per benchmark)
- MI300A 120 CU measurements: `gpu_perf_scripts/mi300a_120cu.csv` (120 CU, half CUs disabled, 5 iterations)
- 22 benchmarks with varying problem sizes covering compute, memory, graph, signal processing, ML, and linear algebra workloads
- Existing MI300A builder: `amd/samples/runner/timingconfig/mi300a/builder.go`
- Timing CU implementation: `amd/timing/cu/`
- Shader array configuration: `amd/samples/runner/timingconfig/shaderarray/builder.go`

## Notes

- The current MI300A builder uses `idealmemcontroller` with hardcoded latency 100. There is also a `dram.Builder` function defined but unused (`createDramControllerBuilder`). Switching to realistic DRAM modeling may improve accuracy.
- MI300A has 120 CUs (20 shader arrays × 6 CUs each) in the simulator config. The real MI300A has 228 CUs (38 XCDs × 6 CUs). The 120 CU setting may correspond to a single GCD or a test configuration.
- The CU builder defaults to 1 GHz; MI300A builder sets 1.5 GHz.
- Very small problem sizes (< cache size) are dominated by kernel launch overhead which the simulator may not model.
