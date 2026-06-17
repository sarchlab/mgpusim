/*
 * HIP kernel for PolyBench Jacobi 2D stencil (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks (tier2/polybench_jacobi2d).
 *
 * One time-step of the 2D Jacobi stencil on an NxN grid:
 *   B[i][j] = (A[i-1][j] + A[i+1][j] + A[i][j-1] + A[i][j+1] + A[i][j]) * 0.2
 * Only interior points (i=1..N-2, j=1..N-2) are updated; boundaries stay 0.
 *
 * Uses a constant BLOCK_DIM (not blockDim.x/y) for the block geometry so the
 * compiler emits no hidden ABI arguments (smaller, simpler kernarg).
 */
#include "hip/hip_runtime.h"

#define BLOCK_DIM 16

extern "C" __global__ void jacobi2d_kernel(
    const float* __restrict__ A,
    float* __restrict__ B,
    int N)
{
    int i = blockIdx.y * BLOCK_DIM + threadIdx.y + 1;  // interior row
    int j = blockIdx.x * BLOCK_DIM + threadIdx.x + 1;  // interior col
    if (i >= N - 1 || j >= N - 1) return;
    B[i * N + j] = (A[(i - 1) * N + j] + A[(i + 1) * N + j] +
                    A[i * N + (j - 1)] + A[i * N + (j + 1)] +
                    A[i * N + j]) * 0.2f;
}
