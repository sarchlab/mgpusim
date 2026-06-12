// 3dconvolution.cpp -- HIP benchmark for PolyBench 3D Convolution
// Applies a fixed 3×3×3 filter to an NI×NJ×NK volume.

#include "bench_common_hip.h"

typedef float DATA_TYPE;

__global__ void convolution3D_kernel(DATA_TYPE *A, DATA_TYPE *B,
                                     int NI, int NJ, int NK) {
    int k = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int j = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;
    int i = hipBlockIdx_z;

    if (i >= 1 && i < NI - 1 && j >= 1 && j < NJ - 1 && k >= 1 && k < NK - 1) {
        // Fixed 3x3x3 filter weights
        DATA_TYPE c11 = 2.0f,  c12 = 5.0f,  c13 = -8.0f;
        DATA_TYPE c21 = -3.0f, c22 = 6.0f,  c23 = -9.0f;
        DATA_TYPE c31 = 4.0f,  c32 = 7.0f,  c33 = 10.0f;

        int idx = i * NJ * NK + j * NK + k;

        B[idx] =
            c11 * A[(i - 1) * NJ * NK + (j - 1) * NK + (k - 1)] +
            c13 * A[(i - 1) * NJ * NK + (j - 1) * NK + (k + 1)] +
            c21 * A[(i - 1) * NJ * NK + j * NK + (k - 1)] +
            c23 * A[(i - 1) * NJ * NK + j * NK + (k + 1)] +
            c31 * A[(i - 1) * NJ * NK + (j + 1) * NK + (k - 1)] +
            c33 * A[(i - 1) * NJ * NK + (j + 1) * NK + (k + 1)] +
            c12 * A[(i - 1) * NJ * NK + (j - 1) * NK + k] +
            c22 * A[(i - 1) * NJ * NK + j * NK + k] +
            c32 * A[(i - 1) * NJ * NK + (j + 1) * NK + k] +

            c11 * A[i * NJ * NK + (j - 1) * NK + (k - 1)] +
            c13 * A[i * NJ * NK + (j - 1) * NK + (k + 1)] +
            c21 * A[i * NJ * NK + j * NK + (k - 1)] +
            c23 * A[i * NJ * NK + j * NK + (k + 1)] +
            c31 * A[i * NJ * NK + (j + 1) * NK + (k - 1)] +
            c33 * A[i * NJ * NK + (j + 1) * NK + (k + 1)] +
            c12 * A[i * NJ * NK + (j - 1) * NK + k] +
            c22 * A[i * NJ * NK + j * NK + k] +
            c32 * A[i * NJ * NK + (j + 1) * NK + k] +

            c11 * A[(i + 1) * NJ * NK + (j - 1) * NK + (k - 1)] +
            c13 * A[(i + 1) * NJ * NK + (j - 1) * NK + (k + 1)] +
            c21 * A[(i + 1) * NJ * NK + j * NK + (k - 1)] +
            c23 * A[(i + 1) * NJ * NK + j * NK + (k + 1)] +
            c31 * A[(i + 1) * NJ * NK + (j + 1) * NK + (k - 1)] +
            c33 * A[(i + 1) * NJ * NK + (j + 1) * NK + (k + 1)] +
            c12 * A[(i + 1) * NJ * NK + (j - 1) * NK + k] +
            c22 * A[(i + 1) * NJ * NK + j * NK + k] +
            c32 * A[(i + 1) * NJ * NK + (j + 1) * NK + k];
    }
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    int NI = parseIntParam(argc, argv, "--size", 64);
    int NJ = NI, NK = NI;

    size_t volSize = (size_t)NI * NJ * NK * sizeof(DATA_TYPE);

    // Host allocations
    DATA_TYPE* h_A = (DATA_TYPE*)malloc(volSize);
    srand(42);
    for (int i = 0; i < NI * NJ * NK; i++)
        h_A[i] = (DATA_TYPE)(rand() % 100) / 10.0f;

    // Device allocations
    DATA_TYPE *d_A, *d_B;
    HIP_CHECK(hipMalloc(&d_A, volSize));
    HIP_CHECK(hipMalloc(&d_B, volSize));

    HIP_CHECK(hipMemcpy(d_A, h_A, volSize, hipMemcpyHostToDevice));
    HIP_CHECK(hipMemset(d_B, 0, volSize));

    dim3 block(16, 16);
    dim3 grid((NK + block.x - 1) / block.x,
              (NJ + block.y - 1) / block.y,
              NI);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%dx%dx%d", NI, NJ, NK);

    printCSVHeader();
    BenchResult r = runBenchmark("3DConvolution", problemSize, iterations, [&]() {
        convolution3D_kernel<<<grid, block>>>(d_A, d_B, NI, NJ, NK);
    });
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_A));
    HIP_CHECK(hipFree(d_B));
    free(h_A);

    return 0;
}
