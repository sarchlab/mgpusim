/*
 * HIP kernel for PolyBench 2MM (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier2/polybench_2mm.
 *
 * 2MM computes two chained matrix multiplications:
 *   D = alpha*A*B + beta*D
 *   E = alpha*C*D + beta*E
 * for NxN square matrices. Both multiplications use the same tiled
 * shared-memory GEMM kernel (mm_kernel), launched twice from the host.
 *
 * Uses a constant TILE_SIZE (not blockDim) for the block geometry so the
 * compiler emits no hidden ABI arguments (kernarg_segment_size = 36).
 */
#include "hip/hip_runtime.h"

#define TILE_SIZE 16

extern "C" __global__ void mm_kernel(
    const float* __restrict__ P,
    const float* __restrict__ Q,
    float* __restrict__ Out,
    int N,
    float alpha,
    float beta)
{
    __shared__ float sP[TILE_SIZE][TILE_SIZE];
    __shared__ float sQ[TILE_SIZE][TILE_SIZE];

    int row = blockIdx.y * TILE_SIZE + threadIdx.y;
    int col = blockIdx.x * TILE_SIZE + threadIdx.x;

    float sum = 0.0f;
    int num_tiles = (N + TILE_SIZE - 1) / TILE_SIZE;

    for (int t = 0; t < num_tiles; ++t) {
        int pCol = t * TILE_SIZE + threadIdx.x;
        int qRow = t * TILE_SIZE + threadIdx.y;

        sP[threadIdx.y][threadIdx.x] = (row < N && pCol < N) ? P[row * N + pCol] : 0.0f;
        sQ[threadIdx.y][threadIdx.x] = (qRow < N && col < N) ? Q[qRow * N + col] : 0.0f;

        __syncthreads();

        for (int k = 0; k < TILE_SIZE; ++k) {
            sum += sP[threadIdx.y][k] * sQ[k][threadIdx.x];
        }

        __syncthreads();
    }

    if (row < N && col < N) {
        Out[row * N + col] = alpha * sum + beta * Out[row * N + col];
    }
}
