# Measured Benchmark Results

This directory stores **measured (real hardware)** benchmark timing results
used for calibrating the MGPUSim simulator.

## Purpose

Results collected on physical GPU hardware serve as the ground truth for
simulator calibration. By comparing simulated timings against these measured
baselines, we can tune simulator parameters and validate accuracy.

## File Naming Convention

```
<hostname>_<gpu_arch>_<date>.csv
```

**Examples:**
- `odyssey_gfx942_20260315.csv`
- `devbox_sm_80_20260301.csv`

## CSV Schema

Each CSV file should contain the following columns (matching the output of
`run_all.sh`):

| Column           | Type    | Description                                  |
|------------------|---------|----------------------------------------------|
| `run_id`         | string  | Unique identifier for the run                |
| `suite`          | string  | Benchmark suite (e.g., polybench, shoc)      |
| `benchmark`      | string  | Benchmark name within the suite              |
| `platform`       | string  | Platform used (`hip` or `cuda`)              |
| `problem_size`   | string  | Problem size / arguments used                |
| `iterations`     | integer | Number of iterations executed                |
| `total_avg_ms`   | float   | Average total execution time (ms)            |
| `total_min_ms`   | float   | Minimum total execution time (ms)            |
| `total_max_ms`   | float   | Maximum total execution time (ms)            |
| `total_stddev_ms`| float   | Standard deviation of execution time (ms)    |
| `status`         | string  | Result status (`pass`, `fail`, `skip`, `timeout`) |

**Header line:**
```
run_id,suite,benchmark,platform,problem_size,iterations,total_avg_ms,total_min_ms,total_max_ms,total_stddev_ms,status
```

## How to Generate

1. Build all benchmarks:
   ```bash
   cd workloads/
   make all
   ```

2. Run the benchmark harness:
   ```bash
   ./scripts/run_all.sh
   ```

3. Copy the resulting CSV file here with the proper naming convention:
   ```bash
   cp results/results_*.csv results/measured/<hostname>_<gpu_arch>_<date>.csv
   ```

## Commit Guidelines

- **One CSV file per machine/GPU combination.** Do not mix results from
  different hardware in the same file.
- **Update when the hardware or software environment changes** (e.g., new
  driver version, new ROCm/CUDA version, OS upgrade).
- **Include the corresponding `sysinfo_*.json`** in your commit message or
  PR description so reviewers can verify the hardware configuration.
- **Do not delete old results** — they serve as historical reference. If you
  need to replace a result, add a new file with an updated date.
