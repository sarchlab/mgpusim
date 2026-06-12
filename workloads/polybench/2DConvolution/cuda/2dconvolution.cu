// 2dconvolution.cu -- CUDA benchmark for PolyBench 2D Convolution
// Applies a fixed 3×3 filter to an NI×NJ matrix.

#include "bench_common_cuda.h"

typedef float DATA_TYPE;

__global__ void convolution2D_kernel(DATA_TYPE *A, DATA_TYPE *B, int NI, int NJ) {
    int j = blockIdx.x * blockDim.x + threadIdx.x;
    int i = blockIdx.y * blockDim.y + threadIdx.y;

    DATA_TYPE c11, c12, c13, c21, c22, c23, c31, c32, c33;
    c11 = 0.2f; c12 = 0.5f; c13 = 0.2f;
    c21 = 0.5f; c22 = 1.0f; c23 = 0.5f;
    c31 = 0.2f; c32 = 0.5f; c33 = 0.2f;

    if (i >= 1 && i < NI - 1 && j >= 1 && j < NJ - 1) {
        B[i * NJ + j] = c11 * A[(i - 1) * NJ + (j - 1)] +
                         c12 * A[(i - 1) * NJ + j] +
                         c13 * A[(i - 1) * NJ + (j + 1)] +
                         c21 * A[i * NJ + (j - 1)] +
                         c22 * A[i * NJ + j] +
                         c23 * A[i * NJ + (j + 1)] +
                         c31 * A[(i + 1) * NJ + (j - 1)] +
                         c32 * A[(i + 1) * NJ + j] +
                         c33 * A[(i + 1) * NJ + (j + 1)];
    }
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    int NI = parseIntParam(argc, argv, "--size", 256);
    int NJ = NI;

    size_t matSize = NI * NJ * sizeof(DATA_TYPE);

    // Host allocations
    DATA_TYPE* h_A = (DATA_TYPE*)malloc(matSize);
    srand(42);
    for (int i = 0; i < NI * NJ; i++)
        h_A[i] = (DATA_TYPE)(rand() % 100) / 10.0f;

    // Device allocations
    DATA_TYPE *d_A, *d_B;
    CUDA_CHECK(cudaMalloc(&d_A, matSize));
    CUDA_CHECK(cudaMalloc(&d_B, matSize));

    CUDA_CHECK(cudaMemcpy(d_A, h_A, matSize, cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemset(d_B, 0, matSize));

    dim3 block(16, 16);
    dim3 grid((NJ + block.x - 1) / block.x, (NI + block.y - 1) / block.y);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%dx%d", NI, NJ);

    printCSVHeader();
    BenchResult r = runBenchmark("2DConvolution", problemSize, iterations, [&]() {
        convolution2D_kernel<<<grid, block>>>(d_A, d_B, NI, NJ);
    });
    printCSVRow(r);

    // Cleanup
    CUDA_CHECK(cudaFree(d_A));
    CUDA_CHECK(cudaFree(d_B));
    free(h_A);

    return 0;
}
