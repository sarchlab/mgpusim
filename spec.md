# MI300A Timing Simulation — Specification

## What do you want to build

An accurate timing simulation of the AMD MI300A GPU using MGPUSim. The simulator should match real MI300A hardware execution times across a suite of HIP benchmarks.

Real hardware measurements are available in `gpu_perf_scripts/mi300a.csv` (240 CU) and `gpu_perf_scripts/mi300a_120cu.csv` (120 CU, half CUs disabled). A comparison script (`gpu_perf_scripts/compare_sim_vs_real.py`) and a simulation runner (`gpu_perf_scripts/run_sim_benchmarks.sh`) are provided.

## How do you consider the project a success

- **Primary metric: Linear regression slope accuracy** — For each benchmark, fit a linear regression of sim_time vs hw_time over large problem sizes (where compute dominates, GPU filled 2-3×). The slope should be close to 1.0 (±20%). Ignore small sizes where fixed overhead dominates.
- **Secondary metric: Average symmetrical error < 20%** across all benchmarks at large sizes
- **Maximum symmetrical error < 50%** for any single benchmark at large sizes
- Error formula: `err = (sim - hw) / min(sim, hw)` (symmetrical, signed)

### Accuracy Evaluation Method (human issue #434)
- **Remove all fixed latency parameters** (kernel launch overhead, memory copy overhead). These mask real modeling issues and don't improve simulation fidelity.
- **Use linear regression** to evaluate accuracy: for each benchmark, plot sim_time vs problem_size and hw_time vs problem_size. Compare the slopes at large sizes. The slopes represent the true computational throughput of the simulator vs hardware.
- **Ignore the starting region** where problem sizes are small and overhead dominates. Focus on cases where the kernel fills the GPU at least 2-3 times.

## Constraints

1. **SIMD width is 16 for CDNA3** — confirmed by ISA documentation (TRAP_STS.EXCP_CYCLE register). Do NOT set NumSinglePrecisionUnit to 32.
2. **Development in origin (dev repo) only** — do not merge PRs in the upstream repository (sarchlab/mgpusim). Create PRs in upstream for review but keep work in origin/sarchlab/mgpusim-dev.
3. **Error calculation uses symmetrical formula**: `(sim - hw) / min(sim, hw)`
4. If human help is needed to run something on real MI300A, create a script and instructions. Human will run and commit results.
5. The 120 CU data can be used for CU scaling analysis.
6. **Do NOT run simulations on the host machine** (human issue #346). Use GitHub Actions for all benchmark simulations. The host runs out of memory with large simulations.
7. **Evidence-based parameter tuning** (human issue #343): When a benchmark has large error, hypothesize a reason, then validate the hypothesis with a microbenchmark or published documentation BEFORE tuning. Document decisions in `mi300a_calibration.md`.
8. **Simulation performance matters** (human issue #344): The simulator is too slow for large problem sizes. Consider: (a) simplifying the simulation even at cost of detail, (b) running simulations in parallel (host has multiple cores, sim is deterministic), (c) using GitHub Actions workflows with parallel jobs for benchmark evaluation.
9. **CI runners** (human issue #444): Use shared runners (ubuntu-latest) for trivial jobs (compilation, lint). Use Marin group (self-hosted) for non-trivial benchmark jobs. The Marin runners are arm64 Fedora and currently need gcc installed for CGO.
10. **Remove all fixed latency** (human issue #434): Remove fixed kernel launch overhead and memory copy overhead. Use linear regression-based accuracy evaluation that ignores small sizes where overhead dominates.

## Architecture Notes (CDNA3 / MI300A)

- **SIMD width**: 16 FP32 ALUs per SIMD (4 cycles per 64-thread wavefront)
- **Wavefront size**: 64 threads
- **CUs**: 240 (20 shader arrays × 6 CUs each) in simulator config; real hardware has 228 but we model 240
- **Frequency**: 1.8 GHz (1800 MHz)
- **L2 cache**: 32 MB
- **Memory**: HBM3, modeled as SimpleBankedMemory with 16 banks
- **VecMem pipeline**: inst=2, trans=4 stages (MI300A specific)

## Notes

- **Remove scratchpad preparer from timing side** (human issue #317): DONE (M8). All scratchpad-related code removed from timing side. Coalescer reads directly from wavefront register file.
- **GPU-side command queueing** (human issue #286): Real MI300A stores commands in a GPU-side stream so the GPU picks up tasks without CPU↔GPU round-trip communication. Consider implementing this for accurate multi-kernel behavior.
- **Remove fixed latency** (human issue #434): All fixed overheads (kernel launch, memory copy) should be removed. They are not useful for simulation fidelity. Use linear regression-based evaluation instead.
- **Microbenchmarks for parameter tuning** (human issue #435): When tuning a parameter, write a microbenchmark in `gpu_perf_scripts/`. Create a script for the human to execute on real MI300A. If unable to progress, claim project failure so human can run microbenchmarks and restart.
- **Benchmark CI workflow** (human issue #344): Create GitHub Actions workflows for running benchmarks in parallel. Workers should start a workflow and check results later, not run simulations directly. Gather workloads in a top-level `workloads/` folder with a Go CLI for running and measuring them.
- **Microbenchmark infrastructure** (human issue #343): Create microbenchmarks for validating specific parameters (cache latency, memory bandwidth, kernel launch overhead). Scripts should be runnable on real MI300A by the human. Results should be committed to the repository.

## Resources

- CDNA3 ISA documentation: `docs/amd-instruct-mi300-cdna3-instruction-set-architecture.pdf`
- GCN3 ISA documentation: `docs/gcn3-instruction-set-architecture.pdf`
- HW benchmark data: `gpu_perf_scripts/mi300a.csv`, `gpu_perf_scripts/mi300a_120cu.csv`
- Benchmark comparison tool: `gpu_perf_scripts/compare_sim_vs_real.py`
- Simulation runner: `gpu_perf_scripts/run_sim_benchmarks.sh`
- M3 benchmark results: `docs/mi300a_m3_benchmark_results.md`
- M4 benchmark results: `docs/mi300a_m4_benchmark_results.md`
- MMU investigation: `docs/mmu_page_not_found_investigation.md`
