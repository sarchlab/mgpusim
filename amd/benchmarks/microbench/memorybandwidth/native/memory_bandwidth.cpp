/*
 * HIP kernel for the memory_bandwidth microbenchmark (gfx942 / CDNA3).
 * Ported from sarchlab/gpu_benchmarks tier1/memory_bandwidth.
 *
 * The original HIP benchmark measures bandwidth using host-side hipMemcpy in
 * three directions (H2D, D2H, D2D). The device-to-device (D2D) path is the
 * only one that performs work on the GPU; it streams every byte of a source
 * buffer into a destination buffer. We model that path with an explicit
 * element-wise copy kernel (dst[i] = src[i]), which is the on-device
 * equivalent of a device-to-device memcpy and produces a verifiable result.
 *
 * Uses a constant BLOCK_SIZE (not blockDim.x) for the block geometry so the
 * compiler emits no hidden ABI arguments. The host launches with a block
 * dimension equal to BLOCK_SIZE.
 */
#include "hip/hip_runtime.h"

#define BLOCK_SIZE 256

extern "C" __global__ void memcpy_d2d_kernel(
    const float* __restrict__ src,
    float* __restrict__ dst,
    int num_elements)
{
    int tid = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    if (tid >= num_elements) return;

    dst[tid] = src[tid];
}
