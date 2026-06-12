// gemm.cpp -- HIP benchmark for PolyBench GEMM
// C = alpha * A * B + beta * C
// A is NI×NK, B is NK×NJ, C is NI×NJ

#include "bench_common_hip.h"

typedef float DATA_TYPE;

__global__ void gemm_kernel(DATA_TYPE *A, DATA_TYPE *B, DATA_TYPE *C,
                            DATA_TYPE alpha, DATA_TYPE beta,
                            int NI, int NJ, int NK) {
    int j = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int i = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

    if (i < NI && j < NJ) {
        DATA_TYPE sum = 0.0f;
        for (int k = 0; k < NK; k++) {
            sum += A[i * NK + k] * B[k * NJ + j];
        }
        C[i * NJ + j] = alpha * sum + beta * C[i * NJ + j];
    }
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    int N = parseIntParam(argc, argv, "--size", 256);
    int NI = N, NJ = N, NK = N;
    DATA_TYPE alpha = 32412.0f, beta = 2123.0f;

    // Host allocations
    DATA_TYPE* h_A = (DATA_TYPE*)malloc(NI * NK * sizeof(DATA_TYPE));
    DATA_TYPE* h_B = (DATA_TYPE*)malloc(NK * NJ * sizeof(DATA_TYPE));
    DATA_TYPE* h_C = (DATA_TYPE*)malloc(NI * NJ * sizeof(DATA_TYPE));

    srand(42);
    for (int i = 0; i < NI * NK; i++) h_A[i] = (DATA_TYPE)(rand() % 100) / 10.0f;
    for (int i = 0; i < NK * NJ; i++) h_B[i] = (DATA_TYPE)(rand() % 100) / 10.0f;
    for (int i = 0; i < NI * NJ; i++) h_C[i] = (DATA_TYPE)(rand() % 100) / 10.0f;

    // Device allocations
    DATA_TYPE *d_A, *d_B, *d_C;
    HIP_CHECK(hipMalloc(&d_A, NI * NK * sizeof(DATA_TYPE)));
    HIP_CHECK(hipMalloc(&d_B, NK * NJ * sizeof(DATA_TYPE)));
    HIP_CHECK(hipMalloc(&d_C, NI * NJ * sizeof(DATA_TYPE)));

    HIP_CHECK(hipMemcpy(d_A, h_A, NI * NK * sizeof(DATA_TYPE), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_B, h_B, NK * NJ * sizeof(DATA_TYPE), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_C, h_C, NI * NJ * sizeof(DATA_TYPE), hipMemcpyHostToDevice));

    dim3 block(16, 16);
    dim3 grid((NJ + block.x - 1) / block.x, (NI + block.y - 1) / block.y);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", N);

    printCSVHeader();
    BenchResult r = runBenchmark("gemm", problemSize, iterations, [&]() {
        gemm_kernel<<<grid, block>>>(d_A, d_B, d_C, alpha, beta, NI, NJ, NK);
    });
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_A));
    HIP_CHECK(hipFree(d_B));
    HIP_CHECK(hipFree(d_C));
    free(h_A); free(h_B); free(h_C);

    return 0;
}
