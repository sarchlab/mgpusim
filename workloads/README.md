# GPU Workloads

GPU benchmark suites for characterizing GPU performance across a range of
compute patterns, memory access behaviors, and application domains.

## Directory Structure

```
workloads/
├── common/              # Shared headers (bench_common_cuda.h, bench_common_hip.h)
├── scripts/             # Build/run/comparison/validation automation
│   ├── build_all.sh
│   ├── run_all.sh
│   ├── collect_sysinfo.sh
│   ├── validate_outputs.py
│   ├── compare_sim_vs_real.py
│   ├── compare_regression.py
│   ├── schema.sql
│   └── fixtures/        # Minimal validator demo fixtures
├── reference/           # Canonical hardware/simulator reference CSVs
├── <suite>/             # Suite directories (polybench, rodinia, etc.)
│   ├── Makefile          # Suite-level build
│   └── <benchmark>/
│       ├── cuda/         # CUDA source + Makefile
│       ├── hip/          # HIP source + Makefile
│       └── params.json   # Parameter sweep definition
└── Makefile              # Top-level build orchestrator
```

## Prerequisites

- **HIP (ROCm):** `hipcc` must be on `PATH`. Tested with ROCm 6.x.
- **CUDA:** `nvcc` must be on `PATH`. Tested with CUDA 12.x.

The build system auto-detects the available platform. If both are installed,
HIP takes priority. You can override with `PLATFORM=cuda` or `PLATFORM=hip`.

## Build Instructions

Build everything (auto-detect platform):

```bash
make all
# or
./scripts/build_all.sh
```

Build a single suite:

```bash
make suite-polybench
```

Build a single benchmark within a suite:

```bash
cd polybench && make bench-atax
```

Force a specific platform:

```bash
make all PLATFORM=cuda
make all PLATFORM=hip
```

View detected build settings:

```bash
make info
```

## Run Instructions

Run all benchmarks:

```bash
./scripts/run_all.sh
```

Run a single suite:

```bash
./scripts/run_all.sh --suite polybench
```

Additional options:

| Option             | Description                          | Default        |
|--------------------|--------------------------------------|----------------|
| `--iterations N`   | Number of timing iterations          | 10             |
| `--timeout SECS`   | Per-benchmark timeout in seconds     | 120            |
| `--output DIR`     | Directory for result files           | `results/`     |
| `--db FILE`        | SQLite database for structured data  | `results/results.sqlite3` |

## Parameter System (`params.json`)

Each benchmark includes a `params.json` file that defines a parameter sweep for
systematic performance characterization.

### Structure

- **`scaling_parameter`** -- The primary dimension being swept (typically problem
  size). Contains 16–18 values spanning a wide range.
- **`non_scaling_parameters`** -- Configuration variants (e.g., tile size, block
  size, precision). Each has 3–5 values.

### Example (`polybench/atax/params.json`)

```json
{
  "benchmark": "atax",
  "suite": "polybench",
  "scaling_parameter": {
    "name": "N",
    "values": [64, 128, 256, 512, 768, 1024, 1536, 2048, 3072, 4096,
               5120, 6144, 7168, 8192, 10240, 12288]
  },
  "non_scaling_parameters": [
    {
      "name": "block_size",
      "values": [64, 128, 256]
    }
  ]
}
```

## Data Collection Schema

Results are stored in a SQLite database (created via `scripts/schema.sql`) and
also exported as CSV/JSON files under the run output directory.

### SQLite tables

| Table             | Description                                      |
|-------------------|--------------------------------------------------|
| `runs`            | System info (GPU model, driver, ROCm/CUDA version, timestamp) |
| `results`         | Per-benchmark timing (suite, benchmark, params, elapsed time)  |
| `kernel_timings`  | Per-kernel detail (one row per kernel measurement)             |

### CSV / JSON output files

`run_all.sh` writes three files for each run id `<RUN_ID>`:

- `results_<RUN_ID>.csv`
- `kernel_timings_<RUN_ID>.csv`
- `sysinfo_<RUN_ID>.json`

`results_<RUN_ID>.csv` schema:

```text
run_id,suite,benchmark,platform,problem_size,iterations,total_avg_ms,total_min_ms,total_max_ms,total_stddev_ms,status
```

- Every row has exactly 11 columns.
- `status` is always one of: `success`, `fail`, `timeout`, `skip`.
- Successful rows include aggregated totals from all kernels emitted by the benchmark invocation.

`kernel_timings_<RUN_ID>.csv` schema:

```text
run_id,suite,benchmark,platform,problem_size,kernel_name,iterations,avg_ms,min_ms,max_ms,stddev_ms
```

- This format matches the per-kernel CSV rows produced by benchmark binaries, with run/suite/benchmark/platform prepended by `run_all.sh`.

`sysinfo_<RUN_ID>.json` schema notes:

- Always emitted as JSON (even if partial system detection fails).
- Safe for `python json.load(...)` parsing.
- Multiline command outputs are JSON-escaped automatically.

### Output validation

Use `scripts/validate_outputs.py` to validate generated artifacts:

```bash
python3 ./scripts/validate_outputs.py \
  --results ./results/results_<RUN_ID>.csv \
  --kernel ./results/kernel_timings_<RUN_ID>.csv \
  --sysinfo ./results/sysinfo_<RUN_ID>.json
```

A fixture-based quick check is available:

```bash
python3 ./scripts/validate_outputs.py \
  --results ./scripts/fixtures/results_demo.csv \
  --kernel ./scripts/fixtures/kernel_timings_demo.csv \
  --sysinfo ./scripts/fixtures/sysinfo_demo.json
```

## Suites

| Suite          | Benchmarks | Description                                |
|----------------|------------|---------------------------------------------|
| `polybench`    | 15         | Linear algebra and stencil computations     |
| `rodinia`      | 16         | Heterogeneous computing benchmarks          |
| `shoc`         | 13         | Scalable heterogeneous computing benchmarks |
| `parboil`      | 10         | Throughput computing benchmarks             |
| `heteromark`   | 5          | Heterogeneous system benchmarks             |
| `lonestar`     | 5          | Irregular GPU algorithms                    |
| `microbench`   | 12         | GPU microarchitecture probes                |
| `ml_kernels`   | 7          | Machine learning kernel primitives          |
