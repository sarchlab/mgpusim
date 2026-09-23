/*
 * HIP kernel for PolyBench 2D Convolution (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier2/polybench_2dconv.
 *
 * Applies a fixed 3x3 PolyBench coefficient stencil to an NI x NJ matrix A,
 * producing output B. One thread per output element.
 *
 * Uses a constant BLOCK_SIZE (not blockDim) for the block geometry so the
 * compiler emits no hidden ABI arguments (small, simple kernarg).
 */
#include "hip/hip_runtime.h"

#define BLOCK_SIZE 16

extern "C" __global__ void convolution2D_kernel(
    const float* __restrict__ A,
    float* __restrict__ B,
    int NI,
    int NJ)
{
    int j = (int)(blockIdx.x * BLOCK_SIZE + threadIdx.x);
    int i = (int)(blockIdx.y * BLOCK_SIZE + threadIdx.y);

    if (i < 1 || i >= NI - 1 || j < 1 || j >= NJ - 1)
        return;

    // PolyBench fixed coefficients
    const float c00 = 0.8f, c01 = 0.2f, c02 = 0.3f;
    const float c10 = 0.2f, c11 = 0.7f, c12 = 0.4f;
    const float c20 = 0.1f, c21 = 0.2f, c22 = 0.5f;

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
