/*
 * HIP kernels for PolyBench Correlation (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier2/polybench_correlation.
 *
 * Computes a correlation matrix from an MxN data matrix in four steps:
 *   1. mean_kernel        - column means
 *   2. stddev_kernel      - column standard deviations
 *   3. normalize_kernel   - normalize each element
 *   4. correlation_kernel - tiled (TILE_SIZE x TILE_SIZE) matmul of
 *                           normalized^T * normalized
 *
 * The 1D kernels use a CONSTANT block size (BLOCK_SIZE) and the tiled
 * kernel a CONSTANT TILE_SIZE instead of blockDim.x/y so the compiler
 * emits no hidden ABI arguments.
 */
#include "hip/hip_runtime.h"
#include <math.h>

#define BLOCK_SIZE 256
#define TILE_SIZE 16

extern "C" __global__ void mean_kernel(
    const float* __restrict__ data,
    float* __restrict__ mean,
    int M,
    int N)
{
    int j = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    if (j >= N) return;

    float sum = 0.0f;
    for (int i = 0; i < M; ++i) {
        sum += data[i * N + j];
    }
    mean[j] = sum / (float)M;
}

extern "C" __global__ void stddev_kernel(
    const float* __restrict__ data,
    const float* __restrict__ mean,
    float* __restrict__ stddev,
    int M,
    int N)
{
    int j = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    if (j >= N) return;

    float m = mean[j];
    float sum = 0.0f;
    for (int i = 0; i < M; ++i) {
        float diff = data[i * N + j] - m;
        sum += diff * diff;
    }
    float s = sqrtf(sum / (float)M);
    stddev[j] = (s < 1e-12f) ? 1.0f : s;
}

extern "C" __global__ void normalize_kernel(
    float* __restrict__ data,
    const float* __restrict__ mean,
    const float* __restrict__ stddev,
    int M,
    int N)
{
    int idx = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    if (idx >= M * N) return;

    int j = idx % N;
    float sqrt_m = sqrtf((float)M);
    data[idx] = (data[idx] - mean[j]) / (sqrt_m * stddev[j]);
}

extern "C" __global__ void correlation_kernel(
    const float* __restrict__ data,
    float* __restrict__ corr,
    int M,
    int N)
{
    __shared__ float sA[TILE_SIZE][TILE_SIZE];
    __shared__ float sB[TILE_SIZE][TILE_SIZE];

    int row = blockIdx.y * TILE_SIZE + threadIdx.y;
    int col = blockIdx.x * TILE_SIZE + threadIdx.x;

    float sum = 0.0f;
    int num_tiles = (M + TILE_SIZE - 1) / TILE_SIZE;

    for (int t = 0; t < num_tiles; ++t) {
        int k_a = t * TILE_SIZE + threadIdx.x;
        int k_b = t * TILE_SIZE + threadIdx.y;

        sA[threadIdx.y][threadIdx.x] = (row < N && k_a < M) ? data[k_a * N + row] : 0.0f;
        sB[threadIdx.y][threadIdx.x] = (k_b < M && col < N) ? data[k_b * N + col] : 0.0f;

        __syncthreads();

        for (int k = 0; k < TILE_SIZE; ++k) {
            sum += sA[threadIdx.y][k] * sB[k][threadIdx.x];
        }

        __syncthreads();
    }

    if (row < N && col < N) {
        if (row == col)
            corr[row * N + col] = 1.0f;
        else
            corr[row * N + col] = sum;
    }
}
