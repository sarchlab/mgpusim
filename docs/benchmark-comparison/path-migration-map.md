# Benchmark Path Migration Map

This document records the benchmark path cleanup.

## Before → After

| Legacy path | Maintained path | Notes |
|---|---|---|
| `gpu_perf_scripts/compare_sim_vs_real.py` | `workloads/scripts/compare_sim_vs_real.py` | Active simulator-vs-hardware comparison script used by CI. |
| `gpu_perf_scripts/compare_regression.py` | `workloads/scripts/compare_regression.py` | Active regression/slope comparison script used by CI. |
| `gpu_perf_scripts/mi300a.csv` | ~~`workloads/reference/mi300a.csv`~~ (REMOVED) | Old MI300A hardware reference; superseded by `workloads/results/kernel_timings_20260317-075319-odyssey.csv`. |
| `gpu_perf_scripts/mi300a_120cu.csv` | ~~`workloads/reference/mi300a_120cu.csv`~~ (REMOVED) | Old 120 CU reference dataset; removed with mi300a.csv. |
| `gpu_perf_scripts/sim_results_m5.csv` | `workloads/reference/sim_results_m5.csv` | Historical simulator output retained as legacy benchmark artifact. |
| `gpu_perf_scripts/comparison_m5_detailed.csv` | `workloads/reference/comparison_m5_detailed.csv` | Historical detailed comparison retained as legacy benchmark artifact. |
| tracked `workspace/devon/*.csv` | untracked + ignored via `.gitignore` (`/workspace/devon/**`) | Removes accidental user workspace artifacts from git history moving forward. |
| remaining files under `gpu_perf_scripts/` | removed from repository | Legacy folder retired; active workflows now read from `workloads/`. |

## Updated consumers

- `.github/workflows/benchmark.yml`
- `benchmark-comparison/run_conv2d_timing_attempts.py`
- Docs that referenced legacy data paths (`accuracy_analysis.md`, `simulator_infrastructure_analysis.md`, `docs/mi300a_*`)

## Non-goal

- No simulator functional source changes were made as part of this migration.
