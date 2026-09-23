/*
 * HIP kernel for PolyBench SYR2K (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier2/polybench_syr2k.
 *
 * Symmetric rank-2k update: C = alpha*A*B^T + alpha*B*A^T + beta*C
 * A and B are [N x M] matrices, C is [N x N].
 * Tiled with shared memory (TILE_SIZE x TILE_SIZE), one thread per C element.
 *
 * Uses a constant TILE_SIZE (not blockDim) for the block geometry so the
 * compiler emits no hidden ABI arguments.
 */
#include "hip/hip_runtime.h"

#define TILE_SIZE 16

extern "C" __global__ void polybench_syr2k_kernel(
    const float* __restrict__ A,
    const float* __restrict__ B,
    float* __restrict__ C,
    int N, int M,
    float alpha, float beta)
{
    __shared__ float sA_row[TILE_SIZE][TILE_SIZE];
    __shared__ float sB_row[TILE_SIZE][TILE_SIZE];
    __shared__ float sA_col[TILE_SIZE][TILE_SIZE];
    __shared__ float sB_col[TILE_SIZE][TILE_SIZE];

    int row = blockIdx.y * TILE_SIZE + threadIdx.y;
    int col = blockIdx.x * TILE_SIZE + threadIdx.x;

    float sum = 0.0f;
    int num_tiles = (M + TILE_SIZE - 1) / TILE_SIZE;

    for (int t = 0; t < num_tiles; ++t) {
        int k = t * TILE_SIZE + threadIdx.x;

        // Load A and B rows for the row-block
        sA_row[threadIdx.y][threadIdx.x] = (row < N && k < M) ? A[row * M + k] : 0.0f;
        sB_row[threadIdx.y][threadIdx.x] = (row < N && k < M) ? B[row * M + k] : 0.0f;

        // Load A and B rows for the col-block
        int col_row = blockIdx.x * TILE_SIZE + threadIdx.y;
        int k2 = t * TILE_SIZE + threadIdx.x;
        sA_col[threadIdx.y][threadIdx.x] = (col_row < N && k2 < M) ? A[col_row * M + k2] : 0.0f;
        sB_col[threadIdx.y][threadIdx.x] = (col_row < N && k2 < M) ? B[col_row * M + k2] : 0.0f;

        __syncthreads();

        for (int kk = 0; kk < TILE_SIZE; ++kk) {
            // A*B^T: A[row][k]*B[col][k]  +  B*A^T: B[row][k]*A[col][k]
            sum += sA_row[threadIdx.y][kk] * sB_col[threadIdx.x][kk]
                 + sB_row[threadIdx.y][kk] * sA_col[threadIdx.x][kk];
        }

        __syncthreads();
    }

    if (row < N && col < N) {
        C[row * N + col] = alpha * sum + beta * C[row * N + col];
    }
}
