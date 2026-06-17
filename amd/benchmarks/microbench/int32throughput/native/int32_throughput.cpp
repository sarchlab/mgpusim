/*
 * HIP kernel for the int32_throughput microbenchmark (gfx942 / CDNA3).
 * Extracted from sarchlab/gpu_benchmarks tier1/int32_throughput.
 *
 * Each thread runs a long chain of INT32 multiply-add operations
 * (a = a * mul + add) entirely in registers, then writes its accumulated
 * result to out[globalThreadId]. The dominant compute is the register-only
 * MAD chain, so the kernel still measures INT32 arithmetic throughput; the
 * single final store per thread makes the result fully verifiable on the
 * host (with int32 wraparound).
 *
 * A constant block size (BLOCK) is used instead of blockDim.x so the
 * compiler emits no hidden ABI arguments. The host launcher MUST use the
 * same block size.
 */
#include "hip/hip_runtime.h"

#define BLOCK 64

extern "C" __global__ void int32_mad_kernel(int *out, int mads_per_thread)
{
    int tid = blockIdx.x * BLOCK + threadIdx.x;

    int a0 = 1 + static_cast<int>(threadIdx.x);
    int a1 = a0 + 111;
    int a2 = a0 + 222;
    int a3 = a0 + 333;

    const int mul = 3;
    const int add = 1;

    for (int i = 0; i < mads_per_thread; i += 4) {
        a0 = a0 * mul + add;
        a1 = a1 * mul + add;
        a2 = a2 * mul + add;
        a3 = a3 * mul + add;
    }

    out[tid] = a0 + a1 + a2 + a3;
}
