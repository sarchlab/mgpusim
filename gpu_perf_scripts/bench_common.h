// bench_common.h — Common timing/reporting header for GPU performance benchmarks
// Header-only. Include from each benchmark's .cpp file.

#ifndef BENCH_COMMON_H
#define BENCH_COMMON_H

#include "hip/hip_runtime.h"
#include <cstdio>
#include <cstdlib>
#include <cfloat>
#include <cstring>
#include <algorithm>
#include <cmath>
#include <vector>

// ---------------------------------------------------------------------------
// Error-checking macro
// ---------------------------------------------------------------------------
#define HIP_CHECK(cmd)                                                       \
    do {                                                                     \
        hipError_t e = (cmd);                                                \
        if (e != hipSuccess) {                                               \
            fprintf(stderr, "HIP error %s at %s:%d\n",                      \
                    hipGetErrorString(e), __FILE__, __LINE__);               \
            exit(1);                                                         \
        }                                                                    \
    } while (0)

// ---------------------------------------------------------------------------
// Result record
// ---------------------------------------------------------------------------
struct BenchResult {
    const char* kernel_name;
    const char* problem_size;
    int         iterations;
    double      avg_ms;
    double      min_ms;
    double      max_ms;
};

// ---------------------------------------------------------------------------
// Parse --iterations N from argv (default: 10)
// ---------------------------------------------------------------------------
inline int parseIterations(int argc, char** argv) {
    int iters = 10;
    for (int i = 1; i < argc - 1; ++i) {
        if (strcmp(argv[i], "--iterations") == 0) {
            iters = atoi(argv[i + 1]);
            if (iters < 1) iters = 1;
            break;
        }
    }
    return iters;
}

// ---------------------------------------------------------------------------
// CSV output helpers
// ---------------------------------------------------------------------------
inline void printCSVHeader() {
    printf("kernel_name,problem_size,iterations,avg_ms,min_ms,max_ms\n");
}

inline void printCSVRow(const BenchResult& r) {
    printf("%s,%s,%d,%.4f,%.4f,%.4f\n",
           r.kernel_name, r.problem_size, r.iterations,
           r.avg_ms, r.min_ms, r.max_ms);
}

// ---------------------------------------------------------------------------
// BenchmarkTimer — wraps hipEvent-based timing
// ---------------------------------------------------------------------------
struct BenchmarkTimer {
    hipEvent_t start, stop;

    BenchmarkTimer() {
        HIP_CHECK(hipEventCreate(&start));
        HIP_CHECK(hipEventCreate(&stop));
    }

    ~BenchmarkTimer() {
        hipEventDestroy(start);
        hipEventDestroy(stop);
    }

    void record_start(hipStream_t stream = 0) {
        HIP_CHECK(hipEventRecord(start, stream));
    }

    void record_stop(hipStream_t stream = 0) {
        HIP_CHECK(hipEventRecord(stop, stream));
        HIP_CHECK(hipEventSynchronize(stop));
    }

    // Returns elapsed time in milliseconds between start and stop.
    float elapsed_ms() const {
        float ms = 0.0f;
        HIP_CHECK(hipEventElapsedTime(&ms, start, stop));
        return ms;
    }
};

// ---------------------------------------------------------------------------
// runBenchmark — run a callable N+1 times (first = warmup), collect stats
//
// Usage:
//   BenchResult r = runBenchmark("myKernel", "1024x1024", iters,
//                                [&]() { myKernelLaunch(); });
//   printCSVRow(r);
// ---------------------------------------------------------------------------
template <typename Func>
inline BenchResult runBenchmark(const char* kernel_name,
                                const char* problem_size,
                                int         iterations,
                                Func        func) {
    BenchmarkTimer timer;

    // Warmup (not measured)
    func();
    HIP_CHECK(hipDeviceSynchronize());

    std::vector<double> times(iterations);

    for (int i = 0; i < iterations; ++i) {
        timer.record_start();
        func();
        timer.record_stop();
        times[i] = static_cast<double>(timer.elapsed_ms());
    }

    double sum = 0.0;
    double mn  = DBL_MAX;
    double mx  = 0.0;
    for (int i = 0; i < iterations; ++i) {
        sum += times[i];
        if (times[i] < mn) mn = times[i];
        if (times[i] > mx) mx = times[i];
    }

    BenchResult r;
    r.kernel_name  = kernel_name;
    r.problem_size = problem_size;
    r.iterations   = iterations;
    r.avg_ms       = sum / iterations;
    r.min_ms       = mn;
    r.max_ms       = mx;
    return r;
}

#endif // BENCH_COMMON_H
