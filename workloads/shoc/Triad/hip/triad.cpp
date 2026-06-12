// triad.cpp -- SHOC Triad benchmark (HIP)
// Stream triad: a[i] = b[i] + scalar * c[i]
// Classic memory bandwidth benchmark.

#include "bench_common_hip.h"

// ---------------------------------------------------------------------------
// Kernel
// ---------------------------------------------------------------------------
__global__ void triad_kernel(float* __restrict__ a,
                             const float* __restrict__ b,
                             const float* __restrict__ c,
                             float scalar, int N) {
    int i = blockIdx.x * blockDim.x + threadIdx.x;
    if (i < N) {
        a[i] = b[i] + scalar * c[i];
    }
}

// ---------------------------------------------------------------------------
// CPU reference for verification
// ---------------------------------------------------------------------------
static void triad_cpu(float* a, const float* b, const float* c,
                      float scalar, int N) {
    for (int i = 0; i < N; ++i) {
        a[i] = b[i] + scalar * c[i];
    }
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------
int main(int argc, char** argv) {
    int N          = parseIntParam(argc, argv, "--array_size", 1048576);
    int block_size = parseIntParam(argc, argv, "--block_size", 256);
    int iters      = parseIterations(argc, argv);

    size_t bytes = (size_t)N * sizeof(float);
    float scalar = 1.75f;

    // Host allocations
    float* h_a = (float*)malloc(bytes);
    float* h_b = (float*)malloc(bytes);
    float* h_c = (float*)malloc(bytes);
    float* h_ref = (float*)malloc(bytes);

    // Initialize
    for (int i = 0; i < N; ++i) {
        h_b[i] = (float)(i % 1000) * 0.001f;
        h_c[i] = (float)((i + 37) % 1000) * 0.001f;
    }

    // Device allocations
    float *d_a, *d_b, *d_c;
    HIP_CHECK(hipMalloc(&d_a, bytes));
    HIP_CHECK(hipMalloc(&d_b, bytes));
    HIP_CHECK(hipMalloc(&d_c, bytes));

    HIP_CHECK(hipMemcpy(d_b, h_b, bytes, hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_c, h_c, bytes, hipMemcpyHostToDevice));

    int grid = (N + block_size - 1) / block_size;

    // Benchmark
    char size_str[64];
    snprintf(size_str, sizeof(size_str), "%d", N);

    printCSVHeader();

    BenchResult r = runBenchmark("triad", size_str, iters, [&]() {
        hipLaunchKernelGGL(triad_kernel, dim3(grid), dim3(block_size),
                           0, 0, d_a, d_b, d_c, scalar, N);
    });

    printCSVRow(r);

    // Bandwidth: 3 arrays * N * sizeof(float) / time
    double bandwidth_gb = 3.0 * N * sizeof(float) / (r.avg_ms * 1e-3) / 1e9;
    fprintf(stderr, "Effective bandwidth: %.2f GB/s\n", bandwidth_gb);

    // Verify
    HIP_CHECK(hipMemcpy(h_a, d_a, bytes, hipMemcpyDeviceToHost));
    triad_cpu(h_ref, h_b, h_c, scalar, N);

    int errors = 0;
    for (int i = 0; i < N; ++i) {
        float diff = fabsf(h_a[i] - h_ref[i]);
        if (diff > 1e-5f * fabsf(h_ref[i]) + 1e-6f) {
            if (errors < 10) {
                fprintf(stderr, "Mismatch at %d: got %f, expected %f\n",
                        i, h_a[i], h_ref[i]);
            }
            errors++;
        }
    }
    if (errors > 0) {
        fprintf(stderr, "FAIL: %d errors\n", errors);
    } else {
        fprintf(stderr, "PASS\n");
    }

    // Cleanup
    HIP_CHECK(hipFree(d_a));
    HIP_CHECK(hipFree(d_b));
    HIP_CHECK(hipFree(d_c));
    free(h_a);
    free(h_b);
    free(h_c);
    free(h_ref);

    return (errors > 0) ? 1 : 0;
}
