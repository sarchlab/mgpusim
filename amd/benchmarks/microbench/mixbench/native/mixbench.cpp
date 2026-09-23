/*
 * HIP kernel for mixbench (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier1/mixbench.
 *
 * Parametric roofline microbenchmark: each thread loads one float, runs a
 * dependency chain of FP32 FMA operations, and stores the result back.
 *
 * Uses a constant BLOCK_SIZE (not blockDim.x) for the block geometry so the
 * compiler emits no hidden ABI arguments. The host launches with a block
 * dimension equal to BLOCK_SIZE.
 */
#include "hip/hip_runtime.h"

#define BLOCK_SIZE 256

extern "C" __global__ void mixbench_kernel(
    float* data,
    int num_elements,
    int num_fmas)
{
    int tid = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    if (tid >= num_elements) return;

    float val = data[tid];

    const float mul = 1.0000001f;
    const float add = 0.0000001f;

    for (int i = 0; i < num_fmas; ++i) {
        val = fmaf(val, mul, add);
    }

    data[tid] = val;
}
