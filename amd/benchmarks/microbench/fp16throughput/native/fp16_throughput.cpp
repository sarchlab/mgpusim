// Device source from GPU Benchmarks v1.1 (a8ca84f8), fp16_throughput.hip.
#include <hip/hip_runtime.h>
#include <hip/hip_fp16.h>

extern "C" __global__ void fp16_fma_kernel(__half2 *out, int fmas_per_thread)
{
    const float base = 1.0f + static_cast<float>(threadIdx.x) * 0.001f;
    __half2 a0 = __float2half2_rn(base);
    __half2 a1 = __float2half2_rn(base + 0.1f);
    __half2 a2 = __float2half2_rn(base + 0.2f);
    __half2 a3 = __float2half2_rn(base + 0.3f);

    // The multiplier must be a representable FP16 value != 1.0. 1.0000001f
    // rounds to exactly 1.0 in FP16 (the ULP near 1.0 is 2^-10 ~= 9.8e-4), so
    // a*mul+add would fold to `a` and the optimizer could delete the whole
    // loop. mul = 1 - 2^-10 and add = 2^-10 are both exactly representable and
    // make a <- mul*a + add a contraction toward the fixed point
    // add/(1-mul) = 1.0: the accumulators stay bounded for any sweep length
    // (no overflow to inf, no subnormal flush) while every iteration still
    // issues a genuine __hfma2.
    const __half2 mul = __float2half2_rn(0.9990234375f);  // 1 - 2^-10
    const __half2 add = __float2half2_rn(0.0009765625f);  // 2^-10

    for (int i = 0; i < fmas_per_thread; i += 4) {
        a0 = __hfma2(a0, mul, add);
        a1 = __hfma2(a1, mul, add);
        a2 = __hfma2(a2, mul, add);
        a3 = __hfma2(a3, mul, add);
    }

    // Store one checksum per global thread (not just thread 0) so every
    // launched thread's accumulator chain is observable. A single guarded
    // write would let the optimizer treat the other threads' recurrences as
    // dead and eliminate them, undercounting the work the grid reports. The
    // write is a single store after the timed loop, negligible next to the
    // per-thread FMA chain.
    const unsigned tid = blockIdx.x * blockDim.x + threadIdx.x;
    out[tid] = __hadd2(__hadd2(a0, a1), __hadd2(a2, a3));
}
