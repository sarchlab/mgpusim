/*
 * HIP kernel for PolyBench GEMM (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier2/polybench_gemm.
 *
 * Dense matrix multiply: C = alpha*A*B + beta*C for NxN square matrices.
 * Tiled with shared memory (TILE_SIZE x TILE_SIZE), one thread per C element.
 *
 * Uses a constant TILE_SIZE (not blockDim) for the block geometry so the
 * compiler emits no hidden ABI arguments (kernarg_segment_size = 36).
 */
#include "hip/hip_runtime.h"

#define TILE_SIZE 16

extern "C" __global__ void polybench_gemm_kernel(
    const float* __restrict__ A,
    const float* __restrict__ B,
    float* __restrict__ C,
    int N,
    float alpha,
    float beta)
{
    __shared__ float sA[TILE_SIZE][TILE_SIZE];
    __shared__ float sB[TILE_SIZE][TILE_SIZE];

    int row = blockIdx.y * TILE_SIZE + threadIdx.y;
    int col = blockIdx.x * TILE_SIZE + threadIdx.x;

    float sum = 0.0f;
    int num_tiles = (N + TILE_SIZE - 1) / TILE_SIZE;

    for (int t = 0; t < num_tiles; ++t) {
        int aCol = t * TILE_SIZE + threadIdx.x;
        int bRow = t * TILE_SIZE + threadIdx.y;

        sA[threadIdx.y][threadIdx.x] = (row < N && aCol < N) ? A[row * N + aCol] : 0.0f;
        sB[threadIdx.y][threadIdx.x] = (bRow < N && col < N) ? B[bRow * N + col] : 0.0f;

        __syncthreads();

        for (int k = 0; k < TILE_SIZE; ++k) {
            sum += sA[threadIdx.y][k] * sB[k][threadIdx.x];
        }

        __syncthreads();
    }

    if (row < N && col < N) {
        C[row * N + col] = alpha * sum + beta * C[row * N + col];
    }
}
