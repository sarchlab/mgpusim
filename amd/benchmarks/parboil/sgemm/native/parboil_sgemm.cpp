/*
 * HIP kernel for Parboil SGEMM (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier2/parboil_sgemm.
 *
 * Tiled single-precision GEMM: C = alpha*A*B + beta*C for NxN square
 * matrices, one thread per C element with shared-memory tiles
 * (TILE x TILE).
 *
 * Uses a constant TILE (not blockDim) for the block geometry so the
 * compiler emits no hidden ABI arguments (kernarg_segment_size = 36).
 */
#include "hip/hip_runtime.h"

#define TILE 16

extern "C" __global__ void sgemm_kernel(
    const float* __restrict__ A,
    const float* __restrict__ B,
    float* __restrict__ C,
    int N,
    float alpha,
    float beta)
{
    __shared__ float tA[TILE][TILE];
    __shared__ float tB[TILE][TILE];

    int row = blockIdx.y * TILE + threadIdx.y;
    int col = blockIdx.x * TILE + threadIdx.x;

    float sum = 0.0f;
    int num_tiles = (N + TILE - 1) / TILE;

    for (int t = 0; t < num_tiles; ++t) {
        int aCol = t * TILE + threadIdx.x;
        int bRow = t * TILE + threadIdx.y;

        tA[threadIdx.y][threadIdx.x] = (row < N && aCol < N) ? A[row * N + aCol] : 0.0f;
        tB[threadIdx.y][threadIdx.x] = (bRow < N && col < N) ? B[bRow * N + col] : 0.0f;

        __syncthreads();

        for (int k = 0; k < TILE; ++k) {
            sum += tA[threadIdx.y][k] * tB[k][threadIdx.x];
        }

        __syncthreads();
    }

    if (row < N && col < N) {
        C[row * N + col] = alpha * sum + beta * C[row * N + col];
    }
}
