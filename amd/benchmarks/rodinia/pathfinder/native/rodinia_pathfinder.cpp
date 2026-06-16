/*
 * HIP kernel for Rodinia PathFinder (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier2/rodinia_pathfinder.
 *
 * Dynamic-programming sweep that computes the minimum-cost path through a
 * 2D grid from top to bottom. One kernel launch processes one row, using
 * the previous row's costs (gpuSrc) to produce the current row (gpuDst).
 *
 * Uses a constant BLOCK_SIZE (not blockDim) for the block geometry so the
 * compiler emits no hidden ABI arguments.
 */
#include "hip/hip_runtime.h"
#include <climits>

#define BLOCK_SIZE 256

extern "C" __global__ void dynproc_kernel(
    const int* __restrict__ gpuWall,
    const int* __restrict__ gpuSrc,
    int* __restrict__       gpuDst,
    int cols,
    int t)
{
    int col = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    if (col >= cols) return;

    int left  = (col > 0)        ? gpuSrc[col - 1] : INT_MAX;
    int above = gpuSrc[col];
    int right = (col < cols - 1) ? gpuSrc[col + 1] : INT_MAX;

    int min3 = min(min(left, above), right);
    gpuDst[col] = gpuWall[(long long)t * cols + col] + min3;
}
