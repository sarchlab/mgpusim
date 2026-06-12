// syr2k.cpp -- HIP benchmark for PolyBench SYR2K
// C = alpha*A*B^T + alpha*B*A^T + beta*C
// A is N×M, B is N×M, C is N×N (symmetric, only lower triangle computed)

#include "bench_common_hip.h"

typedef float DATA_TYPE;

__global__ void syr2k_kernel(DATA_TYPE *A, DATA_TYPE *B, DATA_TYPE *C,
                             DATA_TYPE alpha, DATA_TYPE beta,
                             int N, int M) {
    int j = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int i = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

    if (i < N && j <= i) {
        C[i * N + j] *= beta;
        for (int k = 0; k < M; k++) {
            C[i * N + j] += alpha * A[i * M + k] * B[j * M + k] +
                            alpha * B[i * M + k] * A[j * M + k];
        }
    }
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    int N = parseIntParam(argc, argv, "--size", 256);
    int M = parseIntParam(argc, argv, "--msize", 256);
    DATA_TYPE alpha = 32412.0f, beta = 2123.0f;

    // Host allocations
    DATA_TYPE* h_A = (DATA_TYPE*)malloc(N * M * sizeof(DATA_TYPE));
    DATA_TYPE* h_B = (DATA_TYPE*)malloc(N * M * sizeof(DATA_TYPE));
    DATA_TYPE* h_C = (DATA_TYPE*)malloc(N * N * sizeof(DATA_TYPE));

    srand(42);
    for (int i = 0; i < N * M; i++) h_A[i] = (DATA_TYPE)(rand() % 100) / 10.0f;
    for (int i = 0; i < N * M; i++) h_B[i] = (DATA_TYPE)(rand() % 100) / 10.0f;
    for (int i = 0; i < N * N; i++) h_C[i] = (DATA_TYPE)(rand() % 100) / 10.0f;

    // Device allocations
    DATA_TYPE *d_A, *d_B, *d_C;
    HIP_CHECK(hipMalloc(&d_A, N * M * sizeof(DATA_TYPE)));
    HIP_CHECK(hipMalloc(&d_B, N * M * sizeof(DATA_TYPE)));
    HIP_CHECK(hipMalloc(&d_C, N * N * sizeof(DATA_TYPE)));

    HIP_CHECK(hipMemcpy(d_A, h_A, N * M * sizeof(DATA_TYPE), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_B, h_B, N * M * sizeof(DATA_TYPE), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_C, h_C, N * N * sizeof(DATA_TYPE), hipMemcpyHostToDevice));

    dim3 block(16, 16);
    dim3 grid((N + block.x - 1) / block.x, (N + block.y - 1) / block.y);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", N);

    printCSVHeader();
    BenchResult r = runBenchmark("syr2k", problemSize, iterations, [&]() {
        syr2k_kernel<<<grid, block>>>(d_A, d_B, d_C, alpha, beta, N, M);
    });
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_A));
    HIP_CHECK(hipFree(d_B));
    HIP_CHECK(hipFree(d_C));
    free(h_A); free(h_B); free(h_C);

    return 0;
}
