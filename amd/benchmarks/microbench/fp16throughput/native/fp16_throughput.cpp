/*
 * HIP kernel for the fp16_throughput microbenchmark (gfx942 / CDNA3).
 * Extracted from sarchlab/gpu_benchmarks tier1/fp16_throughput.
 *
 * Each thread runs a long chain of packed half2 fused-multiply-add (FMA)
 * operations entirely in registers (no memory traffic in the loop). Only
 * thread (0,0) of block 0 writes the accumulated result, so the kernel is
 * verifiable from a single output element.
 *
 * The block is 1D and its size is not used inside the kernel (the kernel
 * only reads threadIdx.x / blockIdx.x), so no hidden ABI arguments are
 * required.
 */
#include "hip/hip_runtime.h"
#include <hip/hip_fp16.h>

extern "C" __global__ void fp16_fma_kernel(__half2 *out, int fmas_per_thread)
{
    const float base = 1.0f + static_cast<float>(threadIdx.x) * 0.001f;
    __half2 a0 = __float2half2_rn(base);
    __half2 a1 = __float2half2_rn(base + 0.1f);
    __half2 a2 = __float2half2_rn(base + 0.2f);
    __half2 a3 = __float2half2_rn(base + 0.3f);

    const __half2 mul = __float2half2_rn(1.0000001f);
    const __half2 add = __float2half2_rn(0.0000001f);

    for (int i = 0; i < fmas_per_thread; i += 4) {
        a0 = __hfma2(a0, mul, add);
        a1 = __hfma2(a1, mul, add);
        a2 = __hfma2(a2, mul, add);
        a3 = __hfma2(a3, mul, add);
    }

    if (blockIdx.x == 0 && threadIdx.x == 0) {
        *out = __hadd2(__hadd2(a0, a1), __hadd2(a2, a3));
    }
}
