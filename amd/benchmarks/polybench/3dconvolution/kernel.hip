#include <hip/hip_runtime.h>

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
