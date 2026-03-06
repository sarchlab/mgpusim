# MI300A Timing Simulation — Specification

## What do you want to build

An accurate timing simulation of the AMD MI300A GPU using MGPUSim. The simulator should match real MI300A hardware execution times across a suite of HIP benchmarks.

Real hardware measurements are available in `gpu_perf_scripts/mi300a.csv` (240 CU) and `gpu_perf_scripts/mi300a_120cu.csv` (120 CU, half CUs disabled). A comparison script (`gpu_perf_scripts/compare_sim_vs_real.py`) and a simulation runner (`gpu_perf_scripts/run_sim_benchmarks.sh`) are provided.

## How do you consider the project a success

- **Average symmetrical error < 20%** across all benchmarks
- **Maximum symmetrical error < 50%** for any single benchmark
- Error formula: `err = (sim - hw) / min(sim, hw)` (symmetrical, signed)

## Constraints

1. **SIMD width is 16 for CDNA3** — confirmed by ISA documentation (TRAP_STS.EXCP_CYCLE register). Do NOT set NumSinglePrecisionUnit to 32.
2. **Development in origin (dev repo) only** — do not merge PRs in the upstream repository (sarchlab/mgpusim). Create PRs in upstream for review but keep work in origin/sarchlab/mgpusim-dev.
3. **Error calculation uses symmetrical formula**: `(sim - hw) / min(sim, hw)`
4. If human help is needed to run something on real MI300A, create a script and instructions. Human will run and commit results.
5. The 120 CU data can be used for CU scaling analysis.

## Architecture Notes (CDNA3 / MI300A)

- **SIMD width**: 16 FP32 ALUs per SIMD (4 cycles per 64-thread wavefront)
- **Wavefront size**: 64 threads
- **CUs**: 240 (20 shader arrays × 6 CUs each) in simulator config; real hardware has 228 but we model 240
- **Frequency**: 1.8 GHz (1800 MHz)
- **L2 cache**: 32 MB
- **Memory**: HBM3, modeled as SimpleBankedMemory with 16 banks
- **VecMem pipeline**: inst=2, trans=4 stages (MI300A specific)

## Notes

- **GPU-side command queueing** (human issue #286): Real MI300A stores commands in a GPU-side stream so the GPU picks up tasks without CPU↔GPU round-trip communication. Consider implementing GPU-side queueing if kernel launch overhead is too high. Also, check and tune the fixed overhead values for kernel launching and memory copy operations.
- **Fixed overhead tuning**: The driver currently uses hardcoded `cyclesPerH2D=14500` and `cyclesPerD2H=8500` for memory copy middleware. These should be validated against real hardware behavior and tuned if necessary. Kernel launch dispatch also has overhead from the CPU→GPU→CPU message round-trip that should be examined.

## Resources

- CDNA3 ISA documentation: `docs/amd-instruct-mi300-cdna3-instruction-set-architecture.pdf`
- GCN3 ISA documentation: `docs/gcn3-instruction-set-architecture.pdf`
- HW benchmark data: `gpu_perf_scripts/mi300a.csv`, `gpu_perf_scripts/mi300a_120cu.csv`
- Benchmark comparison tool: `gpu_perf_scripts/compare_sim_vs_real.py`
- Simulation runner: `gpu_perf_scripts/run_sim_benchmarks.sh`
- M3 benchmark results: `docs/mi300a_m3_benchmark_results.md`
- M4 benchmark results: `docs/mi300a_m4_benchmark_results.md`
- MMU investigation: `docs/mmu_page_not_found_investigation.md`
