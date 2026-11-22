/*
 * HIP kernel for PolyBench 3D Convolution (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier2/polybench_3dconv.
 *
 * 3D convolution over an NxNxN volume with a small 3D filter
 * (filter_size x filter_size x filter_size). Each output point is the
 * weighted sum of its neighborhood, one thread per output element.
 *
 * Uses a constant BLOCK_SIZE (not blockDim) for the block geometry so the
 * compiler emits no hidden ABI arguments (smaller, simpler kernarg).
 */
#include "hip/hip_runtime.h"

#define BLOCK_SIZE 8

extern "C" __global__ void conv3d_kernel(
    const float* __restrict__ input,
    const float* __restrict__ filter,
    float* __restrict__ output,
    int N,
    int filter_size)
{
    int k = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    int j = blockIdx.y * BLOCK_SIZE + threadIdx.y;
    int i = blockIdx.z * BLOCK_SIZE + threadIdx.z;

    if (i >= N || j >= N || k >= N) return;

    int half = filter_size / 2;
    float sum = 0.0f;

    for (int fi = 0; fi < filter_size; ++fi) {
        int ii = i - half + fi;
        if (ii < 0 || ii >= N) continue;
        for (int fj = 0; fj < filter_size; ++fj) {
            int jj = j - half + fj;
            if (jj < 0 || jj >= N) continue;
            for (int fk = 0; fk < filter_size; ++fk) {
                int kk = k - half + fk;
                if (kk < 0 || kk >= N) continue;
                sum += input[((size_t)ii * N + jj) * N + kk]
                     * filter[((size_t)fi * filter_size + fj) * filter_size + fk];
            }
        }
    }

    output[((size_t)i * N + j) * N + k] = sum;
}
