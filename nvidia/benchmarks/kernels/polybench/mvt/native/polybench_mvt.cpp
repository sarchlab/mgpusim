/*
 * HIP kernels for PolyBench MVT (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier2/polybench_mvt.
 *
 * Computes:
 *   x1 = A   * y1  (mvt_kernel1: x1[i] = sum_j A[i,j] * y1[j])
 *   x2 = A^T * y2  (mvt_kernel2: x2[j] = sum_i A[i,j] * y2[i])
 * for an N x N matrix A and N-vectors x1, x2, y1, y2.
 *
 * Each kernel is 1D: one thread per output element. A constant BLOCK_SIZE
 * is used (not blockDim.x) so the compiler emits no hidden ABI arguments
 * (kernarg_segment_size = 28).
 */
#include "hip/hip_runtime.h"

#define BLOCK_SIZE 256

extern "C" __global__ void mvt_kernel1(
    const float* __restrict__ A,
    const float* __restrict__ y1,
    float* __restrict__ x1,
    int n)
{
    int i = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    if (i >= n) return;
    float sum = 0.0f;
    for (int j = 0; j < n; j++)
        sum += A[i * n + j] * y1[j];
    x1[i] = sum;
}

extern "C" __global__ void mvt_kernel2(
    const float* __restrict__ A,
    const float* __restrict__ y2,
    float* __restrict__ x2,
    int n)
{
    int j = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    if (j >= n) return;
    float sum = 0.0f;
    for (int i = 0; i < n; i++)
        sum += A[i * n + j] * y2[i];
    x2[j] = sum;
}
