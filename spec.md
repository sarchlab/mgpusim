# MI300A Timing Simulation Accuracy — Specification

## What do you want to build

An accurate timing simulation of the AMD MI300A GPU in MGPUSim. The simulator should faithfully model the CDNA3 architecture's timing behavior, including the compute pipeline, memory hierarchy, and DRAM subsystem.

## How do you consider the project is success

- **Average error** across all testable benchmarks is **less than 20%** using symmetrical error: `err = (sim - hw) / min(sim, hw)`
- **Maximum error** across all testable benchmarks is **less than 50%** using the same formula
- Hardware reference data is in `gpu_perf_scripts/mi300a.csv` and `gpu_perf_scripts/mi300a_120cu.csv`

## Constraints

1. **Error formula**: Use symmetrical error: `err = (sim - hw) / min(sim, hw)` (per issue #264)
2. **Development repo**: All PRs and development stay in origin (sarchlab/mgpusim-dev). Create PRs in upstream (sarchlab/mgpusim) but do NOT merge them. (per issue #266)
3. **No upstream merges**: Do not merge PRs into the upstream repository
4. **120 CU data**: A 120 CU version of hardware measurements is available (mi300a_120cu.csv) — use it if helpful
5. **Hardware access**: For any scripts that need to run on real MI300A hardware, create the script and provide instructions to the human

## Resources

- **Hardware reference data**: `gpu_perf_scripts/mi300a.csv` (full MI300A), `gpu_perf_scripts/mi300a_120cu.csv` (120 CU)
- **CDNA3 ISA docs**: `docs/amd-instinct-mi300-cdna3-instruction-set-architecture.pdf`
- **Benchmark scripts**: `gpu_perf_scripts/run_sim_benchmarks.sh`, `gpu_perf_scripts/compare_sim_vs_real.py`
- **Current timing code**: `amd/timing/cu/`, `amd/samples/runner/timingconfig/mi300a/`

## Notes

- Issue #262: Human questioned the source of "CDNA3 has 32 SP units per SIMD". This claim needs verification against official AMD documentation or the ISA spec before keeping it.
- The simulator currently has significant errors on several benchmarks (kmeans +460%, matmul 256³ +91.5%, floydwarshall -47.6%)
- Several benchmarks hit MMU page faults at larger problem sizes, limiting testable configurations
