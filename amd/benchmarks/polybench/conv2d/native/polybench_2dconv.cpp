// GPU Benchmarks v1.1, tier2/polybench_2dconv/hip/polybench_2dconv.hip.
#include <hip/hip_runtime.h>
#include <cmath>

extern "C" __global__ void convolution2D_kernel(const float* __restrict__ A,
                                     float*       __restrict__ B,
                                     int NI, int NJ) {
    int j = (int)(blockIdx.x * blockDim.x + threadIdx.x);
    int i = (int)(blockIdx.y * blockDim.y + threadIdx.y);

    if (i < 1 || i >= NI - 1 || j < 1 || j >= NJ - 1)
        return;

    // Upstream names enumerate the first coordinate vertically: row 0 below
    // corresponds to c11,c21,c31, not c11,c12,c13.
    const float c00 = 0.2f, c01 = 0.5f, c02 = -0.8f;
    const float c10 = -0.3f, c11 = 0.6f, c12 = -0.9f;
    const float c20 = 0.4f, c21 = 0.7f, c22 = 0.1f;

    B[i * NJ + j] =
        c00 * A[(i - 1) * NJ + (j - 1)] +
        c01 * A[(i - 1) * NJ +  j     ] +
        c02 * A[(i - 1) * NJ + (j + 1)] +
        c10 * A[ i      * NJ + (j - 1)] +
        c11 * A[ i      * NJ +  j     ] +
        c12 * A[ i      * NJ + (j + 1)] +
        c20 * A[(i + 1) * NJ + (j - 1)] +
        c21 * A[(i + 1) * NJ +  j     ] +
        c22 * A[(i + 1) * NJ + (j + 1)];
}
