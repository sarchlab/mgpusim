// gemm.cpp -- SHOC GEMM benchmark (HIP)
// Dense matrix-matrix multiplication: C = alpha*A*B + beta*C
// Single precision, M=N=K=size.

#include "bench_common_hip.h"

// ---------------------------------------------------------------------------
// Kernel: Each thread computes one element of C
// ---------------------------------------------------------------------------
__global__ void gemm_kernel(const float* __restrict__ A,
                            const float* __restrict__ B,
                            float* __restrict__ C,
                            int M, int N, int K,
                            float alpha, float beta) {
    int row = blockIdx.y * blockDim.y + threadIdx.y;
    int col = blockIdx.x * blockDim.x + threadIdx.x;

    if (row < M && col < N) {
        float sum = 0.0f;
        for (int k = 0; k < K; ++k) {
            sum += A[row * K + k] * B[k * N + col];
        }
        C[row * N + col] = alpha * sum + beta * C[row * N + col];
    }
}

// ---------------------------------------------------------------------------
// CPU reference for verification
// ---------------------------------------------------------------------------
static void gemm_cpu(const float* A, const float* B, float* C,
                     int M, int N, int K, float alpha, float beta) {
    for (int i = 0; i < M; ++i) {
        for (int j = 0; j < N; ++j) {
            float sum = 0.0f;
            for (int k = 0; k < K; ++k) {
                sum += A[i * K + k] * B[k * N + j];
            }
            C[i * N + j] = alpha * sum + beta * C[i * N + j];
        }
    }
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------
int main(int argc, char** argv) {
    int size       = parseIntParam(argc, argv, "--size", 512);
    int iters      = parseIterations(argc, argv);

    int M = size, N = size, K = size;
    float alpha = 1.0f, beta = 0.0f;

    size_t bytesA = (size_t)M * K * sizeof(float);
    size_t bytesB = (size_t)K * N * sizeof(float);
    size_t bytesC = (size_t)M * N * sizeof(float);

    // Host allocations
    float* h_A   = (float*)malloc(bytesA);
    float* h_B   = (float*)malloc(bytesB);
    float* h_C   = (float*)malloc(bytesC);
    float* h_ref = (float*)malloc(bytesC);

    // Initialize
    for (int i = 0; i < M * K; ++i)
        h_A[i] = (float)(i % 1000) * 0.001f;
    for (int i = 0; i < K * N; ++i)
        h_B[i] = (float)((i + 37) % 1000) * 0.001f;
    for (int i = 0; i < M * N; ++i) {
        h_C[i]   = 0.0f;
        h_ref[i] = 0.0f;
    }

    // Device allocations
    float *d_A, *d_B, *d_C;
    HIP_CHECK(hipMalloc(&d_A, bytesA));
    HIP_CHECK(hipMalloc(&d_B, bytesB));
    HIP_CHECK(hipMalloc(&d_C, bytesC));

    HIP_CHECK(hipMemcpy(d_A, h_A, bytesA, hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_B, h_B, bytesB, hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_C, h_C, bytesC, hipMemcpyHostToDevice));

    dim3 block(16, 16);
    dim3 grid((N + 15) / 16, (M + 15) / 16);

    // Benchmark
    char size_str[64];
    snprintf(size_str, sizeof(size_str), "%d", size);

    printCSVHeader();

    BenchResult r = runBenchmark("gemm", size_str, iters, [&]() {
        hipLaunchKernelGGL(gemm_kernel, grid, block, 0, 0,
                           d_A, d_B, d_C, M, N, K, alpha, beta);
    });

    printCSVRow(r);

    // GFLOPS: 2*M*N*K / time
    double gflops = 2.0 * M * N * K / (r.avg_ms * 1e-3) / 1e9;
    fprintf(stderr, "Performance: %.2f GFLOPS\n", gflops);

    // Verify (only for small sizes)
    if (size <= 512) {
        HIP_CHECK(hipMemcpy(h_C, d_C, bytesC, hipMemcpyDeviceToHost));
        gemm_cpu(h_A, h_B, h_ref, M, N, K, alpha, beta);

        int errors = 0;
        for (int i = 0; i < M * N; ++i) {
            float diff = fabsf(h_C[i] - h_ref[i]);
            if (diff > 1e-3f * fabsf(h_ref[i]) + 1e-5f) {
                if (errors < 10) {
                    fprintf(stderr, "Mismatch at %d: got %f, expected %f\n",
                            i, h_C[i], h_ref[i]);
                }
                errors++;
            }
        }
        if (errors > 0) {
            fprintf(stderr, "FAIL: %d errors\n", errors);
        } else {
            fprintf(stderr, "PASS\n");
        }
    } else {
        fprintf(stderr, "Skipping verification for large size\n");
    }

    // Cleanup
    HIP_CHECK(hipFree(d_A));
    HIP_CHECK(hipFree(d_B));
    HIP_CHECK(hipFree(d_C));
    free(h_A);
    free(h_B);
    free(h_C);
    free(h_ref);

    return 0;
}
