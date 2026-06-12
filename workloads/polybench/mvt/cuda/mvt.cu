// mvt.cu -- CUDA benchmark for PolyBench MVT
// x1 = x1 + A * y_1
// x2 = x2 + A^T * y_2
// Two kernels, each with N threads

#include "bench_common_cuda.h"

typedef float DATA_TYPE;

__global__ void mvt_kernel1(DATA_TYPE *A, DATA_TYPE *x1, DATA_TYPE *y1,
                            int N) {
    int i = blockIdx.x * blockDim.x + threadIdx.x;

    if (i < N) {
        DATA_TYPE sum = 0.0f;
        for (int j = 0; j < N; j++) {
            sum += A[i * N + j] * y1[j];
        }
        x1[i] += sum;
    }
}

__global__ void mvt_kernel2(DATA_TYPE *A, DATA_TYPE *x2, DATA_TYPE *y2,
                            int N) {
    int i = blockIdx.x * blockDim.x + threadIdx.x;

    if (i < N) {
        DATA_TYPE sum = 0.0f;
        for (int j = 0; j < N; j++) {
            sum += A[j * N + i] * y2[j];
        }
        x2[i] += sum;
    }
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    int N = parseIntParam(argc, argv, "--size", 256);

    // Host allocations
    DATA_TYPE* h_A  = (DATA_TYPE*)malloc(N * N * sizeof(DATA_TYPE));
    DATA_TYPE* h_x1 = (DATA_TYPE*)malloc(N * sizeof(DATA_TYPE));
    DATA_TYPE* h_x2 = (DATA_TYPE*)malloc(N * sizeof(DATA_TYPE));
    DATA_TYPE* h_y1 = (DATA_TYPE*)malloc(N * sizeof(DATA_TYPE));
    DATA_TYPE* h_y2 = (DATA_TYPE*)malloc(N * sizeof(DATA_TYPE));

    srand(42);
    for (int i = 0; i < N * N; i++) h_A[i]  = (DATA_TYPE)(rand() % 100) / 10.0f;
    for (int i = 0; i < N; i++) h_x1[i] = (DATA_TYPE)(rand() % 100) / 10.0f;
    for (int i = 0; i < N; i++) h_x2[i] = (DATA_TYPE)(rand() % 100) / 10.0f;
    for (int i = 0; i < N; i++) h_y1[i] = (DATA_TYPE)(rand() % 100) / 10.0f;
    for (int i = 0; i < N; i++) h_y2[i] = (DATA_TYPE)(rand() % 100) / 10.0f;

    // Device allocations
    DATA_TYPE *d_A, *d_x1, *d_x2, *d_y1, *d_y2;
    CUDA_CHECK(cudaMalloc(&d_A,  N * N * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_x1, N * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_x2, N * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_y1, N * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_y2, N * sizeof(DATA_TYPE)));

    CUDA_CHECK(cudaMemcpy(d_A,  h_A,  N * N * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_x1, h_x1, N * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_x2, h_x2, N * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_y1, h_y1, N * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_y2, h_y2, N * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));

    int blockSize = 256;
    int gridSize = (N + blockSize - 1) / blockSize;

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", N);

    printCSVHeader();
    BenchResult r = runBenchmark("mvt", problemSize, iterations, [&]() {
        mvt_kernel1<<<gridSize, blockSize>>>(d_A, d_x1, d_y1, N);
        mvt_kernel2<<<gridSize, blockSize>>>(d_A, d_x2, d_y2, N);
    });
    printCSVRow(r);

    // Cleanup
    CUDA_CHECK(cudaFree(d_A));
    CUDA_CHECK(cudaFree(d_x1));
    CUDA_CHECK(cudaFree(d_x2));
    CUDA_CHECK(cudaFree(d_y1));
    CUDA_CHECK(cudaFree(d_y2));
    free(h_A); free(h_x1); free(h_x2); free(h_y1); free(h_y2);

    return 0;
}
