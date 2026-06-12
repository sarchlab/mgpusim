#include <hip/hip_runtime.h>

typedef float DATA_TYPE;

__global__ void convolution2D_kernel(DATA_TYPE *A, DATA_TYPE *B, int NI, int NJ) {
    int j = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int i = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

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
