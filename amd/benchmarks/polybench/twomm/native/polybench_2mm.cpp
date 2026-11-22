// GPU Benchmarks v1.1, tier2/polybench_2mm/hip/polybench_2mm.hip.
#include <hip/hip_runtime.h>
#include <cmath>

#define TILE_SIZE 16

template<bool Accumulate>
__global__ void mm_kernel(const float* __restrict__ P,
                          const float* __restrict__ Q,
                          float* __restrict__ Out,
                          int N, float alpha, float beta) {
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
        // The temporary has no initial value: stage 1 must never read it.
        float value = alpha * sum;
        if (Accumulate) value += beta * Out[row * N + col];
        Out[row * N + col] = value;
    }
}

template __global__ void mm_kernel<false>(const float*, const float*, float*, int, float, float);
template __global__ void mm_kernel<true>(const float*, const float*, float*, int, float, float);
