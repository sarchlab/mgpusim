/*
 * HIP kernel for the fp32_throughput microbenchmark (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier1/fp32_throughput.
 *
 * Each thread runs a chain of fused multiply-add (FMA) operations on
 * register-resident floats, using four independent accumulators (UNROLL=4)
 * to keep the FP32 pipelines busy. The kernel is memory-traffic free except
 * for one checksum write per thread so the work is not elided.
 *
 * The work-group size is passed as an explicit `threads_per_block` argument
 * (rather than read from blockDim.x) so the output index is correct for any
 * launch geometry. This keeps the kernel free of implicit/hidden ABI args --
 * the MGPUSim model does not populate the hidden group-size kernarg, so a
 * blockDim.x read would come back as 0 for multi-block launches.
 */
#include "hip/hip_runtime.h"

extern "C" __global__ void fp32_fma_kernel(float* out, int fmas_per_thread,
                                           int threads_per_block)
{
    int tid = blockIdx.x * threads_per_block + threadIdx.x;

    /* Four independent accumulators so the compiler can interleave FMAs and
       hide FP32 latency on the throughput path. */
    float a0 = 1.0f + static_cast<float>(threadIdx.x) * 0.001f;
    float a1 = a0 + 0.1f;
    float a2 = a0 + 0.2f;
    float a3 = a0 + 0.3f;

    const float mul = 1.0000001f;
    const float add = 0.0000001f;

    for (int i = 0; i < fmas_per_thread; i += 4) {
        a0 = fmaf(a0, mul, add);
        a1 = fmaf(a1, mul, add);
        a2 = fmaf(a2, mul, add);
        a3 = fmaf(a3, mul, add);
    }

    /* Every thread writes its checksum so the FMA work cannot be elided. */
    out[tid] = a0 + a1 + a2 + a3;
}
