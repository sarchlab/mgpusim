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
