// gesummv.cu -- CUDA benchmark for PolyBench GESUMMV
// y = alpha * A * x + beta * B * x
// A,B are N×N, x is N, y is N

#include "bench_common_cuda.h"

typedef float DATA_TYPE;

__global__ void gesummv_kernel(DATA_TYPE *A, DATA_TYPE *B, DATA_TYPE *x,
                               DATA_TYPE *y, DATA_TYPE alpha, DATA_TYPE beta,
                               int N) {
    int i = blockIdx.x * blockDim.x + threadIdx.x;

    if (i < N) {
        DATA_TYPE tmp1 = 0.0f;
        DATA_TYPE tmp2 = 0.0f;
        for (int j = 0; j < N; j++) {
            tmp1 += A[i * N + j] * x[j];
            tmp2 += B[i * N + j] * x[j];
        }
        y[i] = alpha * tmp1 + beta * tmp2;
    }
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    int N = parseIntParam(argc, argv, "--size", 256);
    DATA_TYPE alpha = 32412.0f, beta = 2123.0f;

    // Host allocations
    DATA_TYPE* h_A = (DATA_TYPE*)malloc(N * N * sizeof(DATA_TYPE));
    DATA_TYPE* h_B = (DATA_TYPE*)malloc(N * N * sizeof(DATA_TYPE));
    DATA_TYPE* h_x = (DATA_TYPE*)malloc(N * sizeof(DATA_TYPE));
    DATA_TYPE* h_y = (DATA_TYPE*)malloc(N * sizeof(DATA_TYPE));

    srand(42);
    for (int i = 0; i < N * N; i++) h_A[i] = (DATA_TYPE)(rand() % 100) / 10.0f;
    for (int i = 0; i < N * N; i++) h_B[i] = (DATA_TYPE)(rand() % 100) / 10.0f;
    for (int i = 0; i < N; i++) h_x[i] = (DATA_TYPE)(rand() % 100) / 10.0f;

    // Device allocations
    DATA_TYPE *d_A, *d_B, *d_x, *d_y;
    CUDA_CHECK(cudaMalloc(&d_A, N * N * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_B, N * N * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_x, N * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_y, N * sizeof(DATA_TYPE)));

    CUDA_CHECK(cudaMemcpy(d_A, h_A, N * N * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_B, h_B, N * N * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_x, h_x, N * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));

    int blockSize = 256;
    int gridSize = (N + blockSize - 1) / blockSize;

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", N);

    printCSVHeader();
    BenchResult r = runBenchmark("gesummv", problemSize, iterations, [&]() {
        gesummv_kernel<<<gridSize, blockSize>>>(d_A, d_B, d_x, d_y,
                                                alpha, beta, N);
    });
    printCSVRow(r);

    // Cleanup
    CUDA_CHECK(cudaFree(d_A));
    CUDA_CHECK(cudaFree(d_B));
    CUDA_CHECK(cudaFree(d_x));
    CUDA_CHECK(cudaFree(d_y));
    free(h_A); free(h_B); free(h_x); free(h_y);

    return 0;
}
