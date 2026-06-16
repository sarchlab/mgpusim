/*
 * HIP kernel for the fp32_throughput microbenchmark (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier1/fp32_throughput.
 *
 * Each thread runs a chain of fused multiply-add (FMA) operations on
 * register-resident floats, using four independent accumulators (UNROLL=4)
 * to keep the FP32 pipelines busy. The kernel is memory-traffic free except
 * for a single checksum write from thread (0,0) so the work is not elided.
 *
 * The kernel references only threadIdx.x / blockIdx.x (no blockDim), so the
 * compiler emits no hidden ABI arguments.
 */
#include "hip/hip_runtime.h"

/* Block size is compile-time known (256) so the kernel needs no blockDim and
   emits no hidden ABI arguments. */
#define THREADS_PER_BLOCK 256

extern "C" __global__ void fp32_fma_kernel(float* out, int fmas_per_thread)
{
    int tid = blockIdx.x * THREADS_PER_BLOCK + threadIdx.x;

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
