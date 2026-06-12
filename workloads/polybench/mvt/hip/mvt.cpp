// mvt.cpp -- HIP benchmark for PolyBench MVT
// x1 = x1 + A * y_1
// x2 = x2 + A^T * y_2
// Two kernels, each with N threads

#include "bench_common_hip.h"

typedef float DATA_TYPE;

__global__ void mvt_kernel1(DATA_TYPE *A, DATA_TYPE *x1, DATA_TYPE *y1,
                            int N) {
    int i = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;

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
    int i = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;

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
    HIP_CHECK(hipMalloc(&d_A,  N * N * sizeof(DATA_TYPE)));
    HIP_CHECK(hipMalloc(&d_x1, N * sizeof(DATA_TYPE)));
    HIP_CHECK(hipMalloc(&d_x2, N * sizeof(DATA_TYPE)));
    HIP_CHECK(hipMalloc(&d_y1, N * sizeof(DATA_TYPE)));
    HIP_CHECK(hipMalloc(&d_y2, N * sizeof(DATA_TYPE)));

    HIP_CHECK(hipMemcpy(d_A,  h_A,  N * N * sizeof(DATA_TYPE), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_x1, h_x1, N * sizeof(DATA_TYPE), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_x2, h_x2, N * sizeof(DATA_TYPE), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_y1, h_y1, N * sizeof(DATA_TYPE), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_y2, h_y2, N * sizeof(DATA_TYPE), hipMemcpyHostToDevice));

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
    HIP_CHECK(hipFree(d_A));
    HIP_CHECK(hipFree(d_x1));
    HIP_CHECK(hipFree(d_x2));
    HIP_CHECK(hipFree(d_y1));
    HIP_CHECK(hipFree(d_y2));
    free(h_A); free(h_x1); free(h_x2); free(h_y1); free(h_y2);

    return 0;
}
