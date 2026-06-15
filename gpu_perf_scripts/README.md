# GPU Performance Benchmark Scripts

Standalone HIP C++ micro-benchmarks for measuring GPU kernel performance on
AMD MI300A (gfx942). Each benchmark is a single `.cpp` file that includes
`bench_common.h` for timing and CSV reporting.

## Prerequisites

| Requirement | Minimum version |
|-------------|----------------|
| ROCm        | 5.x or later   |
| `hipcc`     | Included with ROCm |
| Target GPU  | gfx942 (AMD MI300A) |

Make sure `hipcc` is on your `PATH` (usually `/opt/rocm/bin/hipcc`).

## Benchmarks (22 total)

| # | Name | Category |
|---|------|----------|
| 1 | vectoradd | Basic |
| 2 | memcopy | Memory |
| 3 | matrixtranspose | Memory |
| 4 | floydwarshall | Graph |
| 5 | fastwalshtransform | Signal |
| 6 | fir | Signal |
| 7 | simpleconvolution | Signal |
| 8 | bitonicsort | Sorting |
| 9 | kmeans | ML |
| 10 | atax | Linear Algebra |
| 11 | bicg | Linear Algebra |
| 12 | relu | ML |
| 13 | pagerank | Graph |
| 14 | stencil2d | Stencil |
| 15 | bfs | Graph |
| 16 | nw | Dynamic Prog. |
| 17 | fft | Signal |
| 18 | spmv | Sparse |
| 19 | matrixmultiplication | Linear Algebra |
| 20 | nbody | Simulation |
| 21 | conv2d | ML |
| 22 | im2col | ML |

## Quick Start

```bash
# Build all benchmarks
chmod +x build_all.sh run_all.sh
./build_all.sh

# Run all benchmarks (default: 10 iterations each)
./run_all.sh

# Run with custom iteration count
./run_all.sh --iterations 50

# Write results to a different file
./run_all.sh --output my_results.csv
```

### Build a single benchmark

```bash
./build_all.sh vectoradd
```

### Override compiler or architecture

```bash
HIPCC=/opt/rocm/bin/hipcc GPU_ARCH=gfx90a ./build_all.sh
```

## Output Format

Both individual benchmarks and `run_all.sh` produce CSV output:

```
kernel_name,problem_size,iterations,avg_ms,min_ms,max_ms
vectoradd,N=1048576,10,0.1234,0.1100,0.1400
```

`run_all.sh` collects all results into `results.csv` (or the file specified
with `--output`).

## Writing a New Benchmark

1. Create `<name>.cpp` in this directory.
2. Include `bench_common.h`.
3. Use the provided helpers:

```cpp
#include "bench_common.h"

__global__ void myKernel(float* data, int N) { /* ... */ }

int main(int argc, char** argv) {
    int iters = parseIterations(argc, argv);

    // ... allocate memory, set up data ...

    BenchResult r = runBenchmark("myKernel", "N=1048576", iters, [&]() {
        hipLaunchKernelGGL(myKernel, grid, block, 0, 0, d_data, N);
    });

    printCSVHeader();
    printCSVRow(r);

    // ... clean up ...
    return 0;
}
```

4. Add the benchmark name to the `BENCHMARKS` array in `build_all.sh` and
   `run_all.sh`.

## bench_common.h API

| Symbol | Description |
|--------|-------------|
| `HIP_CHECK(cmd)` | Macro — abort on HIP error with file/line info |
| `BenchResult` | Struct — holds kernel name, problem size, iterations, avg/min/max ms |
| `BenchmarkTimer` | Struct — RAII wrapper around `hipEvent` start/stop |
| `parseIterations(argc, argv)` | Parse `--iterations N` from CLI (default 10) |
| `printCSVHeader()` | Print CSV column header to stdout |
| `printCSVRow(r)` | Print one `BenchResult` as a CSV row |
| `runBenchmark(name, size, iters, func)` | Template — warmup + timed iterations, returns `BenchResult` |

## HIP Microbenchmarks

Two standalone microbenchmarks for measuring low-level GPU performance
characteristics on MI300A. These are **not** application benchmarks — they
target raw hardware capabilities to help validate simulator parameters.

> **⚠️ These must be run on real MI300A hardware by a human.**
> They cannot be run in CI or on machines without a gfx942 GPU.

### Microbenchmark 1: Memory Bandwidth (`micro_membw.cpp`)

Measures effective global memory bandwidth via three streaming patterns:

| Pattern | What it measures |
|---------|-----------------|
| Stream Read | Sum all elements — forces DRAM reads |
| Stream Write | Fill all elements — pure write bandwidth |
| Stream Copy | Copy A→B — combined read+write bandwidth |

Tests array sizes: 256 MB, 512 MB, 1 GB. Reports GB/s for each.

#### Build & Run

```bash
hipcc -O2 micro_membw.cpp -o micro_membw
./micro_membw                    # default: 10 iterations
./micro_membw --iterations 20   # custom iteration count
```

#### Expected Output

```
# Memory Bandwidth Microbenchmark (MI300A)
# Iterations per test: 10
#
operation,array_size,iterations,avg_ms,min_ms,max_ms,avg_gbps
stream_read,256MB,10,0.1234,0.1100,0.1400,2068.41
stream_write,256MB,10,0.0987,0.0900,0.1100,2593.72
stream_copy,256MB,10,0.1456,0.1300,0.1600,3516.48
...
```

### Microbenchmark 2: Kernel Launch Overhead (`micro_launch.cpp`)

Measures per-kernel-launch overhead in three modes:

| Mode | Description |
|------|-------------|
| `batch_async` | Launch N empty kernels, sync once at end |
| `sync_per_launch` | Empty kernel + `hipDeviceSynchronize` each time |
| `small_kernel_sync` | 1-thread kernel with minimal work + sync each time |

Default: 10 000 launches per measurement.

#### Build & Run

```bash
hipcc -O2 micro_launch.cpp -o micro_launch
./micro_launch                              # default: 10000 launches, 10 iters
./micro_launch --iterations 20 --launches 5000
```

#### Expected Output

```
# Kernel Launch Overhead Microbenchmark (MI300A)
# Launches per measurement: 10000
# Measurement iterations: 10
#
test,launches,iterations,total_avg_ms,per_launch_avg_us
batch_async,10000,10,45.1234,4.5123
sync_per_launch,10000,10,123.4567,12.3457
small_kernel_sync,10000,10,125.6789,12.5679
```
