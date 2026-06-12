// bench_common_cuda.h -- Common timing/reporting header for CUDA GPU benchmarks
// Header-only. Include from each benchmark's .cu file.
//
// Provides: BenchResult, BenchmarkTimer, runBenchmark(), printCSVHeader(),
//           printCSVRow(), parseIterations(), parseIntParam()
//
// Identical interface to bench_common_hip.h -- benchmarks are interchangeable.

#ifndef BENCH_COMMON_CUDA_H
#define BENCH_COMMON_CUDA_H

#include <cuda_runtime.h>
#include <cstdio>
#include <cstdlib>
#include <cfloat>
#include <cstring>
#include <cmath>
#include <vector>

// ---------------------------------------------------------------------------
// Error-checking macro
// ---------------------------------------------------------------------------
#define CUDA_CHECK(cmd)                                                      \
    do {                                                                     \
        cudaError_t e = (cmd);                                               \
        if (e != cudaSuccess) {                                              \
            fprintf(stderr, "CUDA error %s at %s:%d\n",                     \
                    cudaGetErrorString(e), __FILE__, __LINE__);              \
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
    double      stddev_ms;
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
// Parse --<name> N from argv (returns defaultVal if not found)
// ---------------------------------------------------------------------------
inline int parseIntParam(int argc, char** argv, const char* name,
                         int defaultVal) {
    for (int i = 1; i < argc - 1; ++i) {
        if (strcmp(argv[i], name) == 0) {
            int v = atoi(argv[i + 1]);
            if (v > 0) return v;
            break;
        }
    }
    return defaultVal;
}

// ---------------------------------------------------------------------------
// CSV output helpers
// ---------------------------------------------------------------------------
inline void printCSVHeader() {
    printf("kernel_name,problem_size,iterations,avg_ms,min_ms,max_ms,"
           "stddev_ms\n");
}

inline void printCSVRow(const BenchResult& r) {
    printf("%s,%s,%d,%.4f,%.4f,%.4f,%.4f\n",
           r.kernel_name, r.problem_size, r.iterations,
           r.avg_ms, r.min_ms, r.max_ms, r.stddev_ms);
}

// ---------------------------------------------------------------------------
// BenchmarkTimer -- wraps cudaEvent-based timing
// ---------------------------------------------------------------------------
struct BenchmarkTimer {
    cudaEvent_t start, stop;

    BenchmarkTimer() {
        CUDA_CHECK(cudaEventCreate(&start));
        CUDA_CHECK(cudaEventCreate(&stop));
    }

    ~BenchmarkTimer() {
        (void)cudaEventDestroy(start);
        (void)cudaEventDestroy(stop);
    }

    void record_start(cudaStream_t stream = 0) {
        CUDA_CHECK(cudaEventRecord(start, stream));
    }

    void record_stop(cudaStream_t stream = 0) {
        CUDA_CHECK(cudaEventRecord(stop, stream));
        CUDA_CHECK(cudaEventSynchronize(stop));
    }

    float elapsed_ms() const {
        float ms = 0.0f;
        CUDA_CHECK(cudaEventElapsedTime(&ms, start, stop));
        return ms;
    }
};

// ---------------------------------------------------------------------------
// runBenchmark -- run a callable N+1 times (first = warmup), collect stats
//
// Usage:
//   BenchResult r = runBenchmark("myKernel", "1024", iters,
//                                [&]() { myKernel<<<grid,block>>>(...); });
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
    CUDA_CHECK(cudaDeviceSynchronize());

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

    double avg = sum / iterations;

    double variance = 0.0;
    for (int i = 0; i < iterations; ++i) {
        double diff = times[i] - avg;
        variance += diff * diff;
    }
    double stddev = (iterations > 1) ? sqrt(variance / (iterations - 1)) : 0.0;

    BenchResult r;
    r.kernel_name  = kernel_name;
    r.problem_size = problem_size;
    r.iterations   = iterations;
    r.avg_ms       = avg;
    r.min_ms       = mn;
    r.max_ms       = mx;
    r.stddev_ms    = stddev;
    return r;
}

#endif // BENCH_COMMON_CUDA_H
