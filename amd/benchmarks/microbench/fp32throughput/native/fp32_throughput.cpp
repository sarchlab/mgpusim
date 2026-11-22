// Device source from GPU Benchmarks v1.1 (a8ca84f8), fp32_throughput.hip.
#include <hip/hip_runtime.h>

extern "C" __global__ void fp32_fma_kernel(float *out, int fmas_per_thread)
{
    /* Four independent accumulators expose instruction-level parallelism. */
    float a0 = 1.0f + static_cast<float>(threadIdx.x) * 0.001f;
    float a1 = a0 + 0.1f;
    float a2 = a0 + 0.2f;
    float a3 = a0 + 0.3f;

    /* Multiplier and addend — kept != 1/0 so the compiler cannot constant-fold
       the entire loop away. */
    const float mul = 1.0000001f;
    const float add = 0.0000001f;

    for (int i = 0; i < fmas_per_thread; i += 4) {
        a0 = fmaf(a0, mul, add);
        a1 = fmaf(a1, mul, add);
        a2 = fmaf(a2, mul, add);
        a3 = fmaf(a3, mul, add);
    }

    // One result per launched thread makes every accumulator chain observable.
    // This final store is included in the timed kernel, outside the FMA loop.
    const size_t tid = static_cast<size_t>(blockIdx.x) * blockDim.x + threadIdx.x;
    out[tid] = a0 + a1 + a2 + a3;
}
