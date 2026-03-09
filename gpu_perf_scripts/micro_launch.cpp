// micro_launch.cpp — Kernel launch overhead microbenchmark for AMD MI300A
//
// Measures per-kernel-launch overhead in three modes:
//   1. Batch async  — launch N empty kernels, then synchronize once
//   2. Sync-per-launch — launch + hipDeviceSynchronize each time
//   3. Small kernel  — 1-thread kernel with minimal work, sync each time
//
// Build:  hipcc -O2 micro_launch.cpp -o micro_launch
// Run:    ./micro_launch [--iterations N] [--launches L]
//
// Output: CSV with per-launch time in microseconds.

#include "bench_common.h"
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <chrono>

// ---------------------------------------------------------------------------
// Kernels
// ---------------------------------------------------------------------------

__global__ void empty_kernel() {
    // Intentionally empty — measures pure launch overhead
}

__global__ void small_kernel(float* out) {
    // Minimal work kernel — 1 thread writes one value
    if (threadIdx.x == 0 && blockIdx.x == 0) {
        out[0] = 1.0f;
    }
}

// ---------------------------------------------------------------------------
// Wall-clock helper (hipEvents don't capture host-side launch overhead well
// for async batches, so we use steady_clock for the batch-async test)
// ---------------------------------------------------------------------------
static double wall_time_ms() {
    auto now = std::chrono::steady_clock::now();
    auto dur = now.time_since_epoch();
    return std::chrono::duration<double, std::milli>(dur).count();
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------
int main(int argc, char** argv) {
    int iters   = parseIterations(argc, argv);
    int launches = parseIntParam(argc, argv, "--launches", 10000);

    // Small buffer for the small_kernel test
    float* d_out = nullptr;
    HIP_CHECK(hipMalloc(&d_out, sizeof(float)));
    HIP_CHECK(hipMemset(d_out, 0, sizeof(float)));
    HIP_CHECK(hipDeviceSynchronize());

    printf("# Kernel Launch Overhead Microbenchmark (MI300A)\n");
    printf("# Launches per measurement: %d\n", launches);
    printf("# Measurement iterations: %d\n", iters);
    printf("#\n");
    printf("test,launches,iterations,total_avg_ms,per_launch_avg_us\n");

    // -----------------------------------------------------------------------
    // Test 1: Batch async — launch N empty kernels, then sync once
    // -----------------------------------------------------------------------
    {
        double total_sum = 0.0;
        double total_min = 1e30;
        double total_max = 0.0;

        // Warmup
        for (int l = 0; l < launches; l++) {
            hipLaunchKernelGGL(empty_kernel, dim3(1), dim3(1), 0, 0);
        }
        HIP_CHECK(hipDeviceSynchronize());

        for (int it = 0; it < iters; it++) {
            double t0 = wall_time_ms();
            for (int l = 0; l < launches; l++) {
                hipLaunchKernelGGL(empty_kernel, dim3(1), dim3(1), 0, 0);
            }
            HIP_CHECK(hipDeviceSynchronize());
            double t1 = wall_time_ms();
            double elapsed = t1 - t0;
            total_sum += elapsed;
            if (elapsed < total_min) total_min = elapsed;
            if (elapsed > total_max) total_max = elapsed;
        }

        double avg_total_ms = total_sum / iters;
        double per_launch_us = (avg_total_ms / launches) * 1000.0;

        printf("batch_async,%d,%d,%.4f,%.4f\n",
               launches, iters, avg_total_ms, per_launch_us);
    }

    // -----------------------------------------------------------------------
    // Test 2: Sync-per-launch — empty kernel + hipDeviceSynchronize each time
    // -----------------------------------------------------------------------
    {
        double total_sum = 0.0;
        double total_min = 1e30;
        double total_max = 0.0;

        // Warmup
        for (int l = 0; l < 100; l++) {
            hipLaunchKernelGGL(empty_kernel, dim3(1), dim3(1), 0, 0);
            HIP_CHECK(hipDeviceSynchronize());
        }

        for (int it = 0; it < iters; it++) {
            double t0 = wall_time_ms();
            for (int l = 0; l < launches; l++) {
                hipLaunchKernelGGL(empty_kernel, dim3(1), dim3(1), 0, 0);
                HIP_CHECK(hipDeviceSynchronize());
            }
            double t1 = wall_time_ms();
            double elapsed = t1 - t0;
            total_sum += elapsed;
            if (elapsed < total_min) total_min = elapsed;
            if (elapsed > total_max) total_max = elapsed;
        }

        double avg_total_ms = total_sum / iters;
        double per_launch_us = (avg_total_ms / launches) * 1000.0;

        printf("sync_per_launch,%d,%d,%.4f,%.4f\n",
               launches, iters, avg_total_ms, per_launch_us);
    }

    // -----------------------------------------------------------------------
    // Test 3: Small kernel (1 thread) with sync per launch
    // -----------------------------------------------------------------------
    {
        double total_sum = 0.0;
        double total_min = 1e30;
        double total_max = 0.0;

        // Warmup
        for (int l = 0; l < 100; l++) {
            hipLaunchKernelGGL(small_kernel, dim3(1), dim3(1), 0, 0, d_out);
            HIP_CHECK(hipDeviceSynchronize());
        }

        for (int it = 0; it < iters; it++) {
            double t0 = wall_time_ms();
            for (int l = 0; l < launches; l++) {
                hipLaunchKernelGGL(small_kernel, dim3(1), dim3(1), 0, 0, d_out);
                HIP_CHECK(hipDeviceSynchronize());
            }
            double t1 = wall_time_ms();
            double elapsed = t1 - t0;
            total_sum += elapsed;
            if (elapsed < total_min) total_min = elapsed;
            if (elapsed > total_max) total_max = elapsed;
        }

        double avg_total_ms = total_sum / iters;
        double per_launch_us = (avg_total_ms / launches) * 1000.0;

        printf("small_kernel_sync,%d,%d,%.4f,%.4f\n",
               launches, iters, avg_total_ms, per_launch_us);
    }

    HIP_CHECK(hipFree(d_out));

    printf("#\n");
    printf("# Done.\n");
    return 0;
}
